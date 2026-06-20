//go:build cgo

// Package conformance runs Cambium's shared /conformance corpus. It reads
// manifest.toml, parses each fixture through the libyang backend, and asserts
// byte-for-byte equality with the golden outputs (after trailing-whitespace
// normalization).
package conformance

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/signalbreak-labs/cambium/go/internal/confmanifest"
	backend "github.com/signalbreak-labs/cambium/go/libyangbackend"
)

// Case is one entry in manifest.toml. Kept as an alias so existing callers
// continue to work while the parser lives in the cgo-free internal package.
type Case = confmanifest.Case

// FindConformanceDir walks up from the current directory to locate the shared
// conformance directory (the one containing manifest.toml).
func FindConformanceDir() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		candidate := filepath.Join(dir, "conformance", "manifest.toml")
		if _, err := os.Stat(candidate); err == nil {
			return filepath.Join(dir, "conformance"), nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("could not locate conformance/manifest.toml above %s", dir)
		}
		dir = parent
	}
}

// LoadManifest delegates to the shared confmanifest parser.
func LoadManifest(path string) ([]Case, error) {
	return confmanifest.Load(path)
}

// RunCase loads the case's modules, parses its input, and asserts every
// expected format matches the golden bytes.
func RunCase(conformanceDir string, c Case) error {
	if c.EffectiveTier() == confmanifest.TierSchemaIR {
		return fmt.Errorf("RunCase cannot execute schema-ir case %q", c.Name)
	}
	if c.Input == "" {
		return fmt.Errorf("case %q has no input", c.Name)
	}
	if c.InputFormat == "" {
		return fmt.Errorf("case %q has no input-format", c.Name)
	}

	moduleDir := filepath.Join(conformanceDir, c.Module)
	inputPath := filepath.Join(conformanceDir, c.Input)

	ctx, err := backend.NewContext()
	if err != nil {
		return err
	}
	defer ctx.Close()
	if err := ctx.SetSearchPath(moduleDir); err != nil {
		return err
	}
	if err := loadModulesInDir(ctx, moduleDir); err != nil {
		return err
	}

	input, err := os.ReadFile(inputPath)
	if err != nil {
		return fmt.Errorf("read input: %w", err)
	}
	inFmt, err := parseFormat(c.InputFormat)
	if err != nil {
		return err
	}

	var tree *backend.DataTree
	if c.OpType != "" {
		op, err := parseOpType(c.OpType)
		if err != nil {
			return err
		}
		tree, err = ctx.ParseOp(inFmt, op, input)
		if err != nil {
			return err
		}
	} else {
		tree, err = ctx.Parse(inFmt, backend.ParseModeDataOnly, input)
		if err != nil {
			return err
		}
	}
	defer tree.Close()

	formats := make([]string, 0, len(c.Expected))
	for f := range c.Expected {
		formats = append(formats, f)
	}
	sort.Strings(formats)

	for _, fmtName := range formats {
		goldenPath := filepath.Join(conformanceDir, c.Expected[fmtName])
		expected, err := os.ReadFile(goldenPath)
		if err != nil {
			return fmt.Errorf("read golden %s: %w", goldenPath, err)
		}
		outFmt, err := parseFormat(fmtName)
		if err != nil {
			return err
		}
		flags := backend.DefaultSerializeFlags()
		if c.SerializeDefaults != "" {
			flags.WithDefaults, err = parseWithDefaults(c.SerializeDefaults)
			if err != nil {
				return err
			}
		}
		actual, err := tree.Serialize(outFmt, flags)
		if err != nil {
			return err
		}
		if !bytes.Equal(formatBytesForCompare(outFmt, expected), formatBytesForCompare(outFmt, actual)) {
			return fmt.Errorf(
				"%s output does not match golden %s\n--- expected ---\n%s\n--- actual ---\n%s",
				fmtName, goldenPath, snippet(expected), snippet(actual),
			)
		}
		if c.Oracle {
			yanglint := strings.TrimSpace(os.Getenv("CAMBIUM_YANGLINT"))
			if yanglint != "" {
				oracle, err := runYanglintOracle(yanglint, moduleDir, inputPath, outFmt, flags.WithDefaults, c.OpType)
				if err != nil {
					return err
				}
				if !bytes.Equal(formatBytesForCompare(outFmt, oracle), formatBytesForCompare(outFmt, actual)) {
					return fmt.Errorf(
						"%s output differs from yanglint oracle\n--- yanglint ---\n%s\n--- cambium ---\n%s",
						fmtName, snippet(oracle), snippet(actual),
					)
				}
			}
		}
	}
	return nil
}

// Run executes the named cases (or all, if only is empty) and returns the
// passing and failing case names.
func Run(conformanceDir string, only []string) (passed, failed []string, err error) {
	cases, err := LoadManifest(filepath.Join(conformanceDir, "manifest.toml"))
	if err != nil {
		return nil, nil, err
	}
	enabled := map[string]bool{}
	for _, n := range only {
		enabled[n] = true
	}
	for _, c := range cases {
		if len(only) > 0 && !enabled[c.Name] {
			continue
		}
		if c.EffectiveTier() == confmanifest.TierSchemaIR {
			continue
		}
		if e := RunCase(conformanceDir, c); e != nil {
			failed = append(failed, fmt.Sprintf("%s: %v", c.Name, e))
		} else {
			passed = append(passed, c.Name)
		}
	}
	return passed, failed, nil
}

func loadModulesInDir(ctx *backend.Context, dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("read module dir: %w", err)
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if filepath.Ext(e.Name()) != ".yang" {
			continue
		}
		// Skip submodule files; they are resolved via include from their main module.
		if isSubmodule(filepath.Join(dir, e.Name())) {
			continue
		}
		names = append(names, e.Name())
	}
	sort.Strings(names)
	for _, n := range names {
		stem := strings.TrimSuffix(n, ".yang")
		// Strip a revision suffix such as ietf-inet-types@2025-12-22.
		if at := strings.IndexByte(stem, '@'); at >= 0 {
			stem = stem[:at]
		}
		if err := ctx.LoadModule(stem); err != nil {
			return err
		}
	}
	return nil
}

func isSubmodule(path string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	return strings.HasPrefix(strings.TrimSpace(string(data)), "submodule ")
}

func runYanglintOracle(yanglint, moduleDir, inputPath string, format backend.Format, wd backend.WithDefaults, opType string) ([]byte, error) {
	schemas, err := oracleSchemaPaths(moduleDir)
	if err != nil {
		return nil, err
	}
	formatArg, err := yanglintFormatArg(format)
	if err != nil {
		return nil, err
	}
	cmd := exec.Command(yanglint)
	cmd.Args = append(cmd.Args, "-X", "-p", moduleDir)
	if wdArg := yanglintWithDefaultsArg(wd); wdArg != "" {
		cmd.Args = append(cmd.Args, "-d", wdArg)
	}
	if opType != "" {
		yt, err := yanglintOpTypeArg(opType)
		if err != nil {
			return nil, err
		}
		cmd.Args = append(cmd.Args, "-t", yt)
	}
	cmd.Args = append(cmd.Args, "-f", formatArg)
	for _, schema := range schemas {
		cmd.Args = append(cmd.Args, "-F", moduleNameForYANGPath(schema)+":")
	}
	cmd.Args = append(cmd.Args, schemas...)
	cmd.Args = append(cmd.Args, inputPath)

	out, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return nil, fmt.Errorf("yanglint failed: %s", strings.TrimSpace(string(exitErr.Stderr)))
		}
		return nil, fmt.Errorf("yanglint: %w", err)
	}
	return out, nil
}

func oracleSchemaPaths(moduleDir string) ([]string, error) {
	entries, err := os.ReadDir(moduleDir)
	if err != nil {
		return nil, fmt.Errorf("read module dir: %w", err)
	}
	schemas := make([]string, 0, len(entries))
	for _, entry := range entries {
		if filepath.Ext(entry.Name()) != ".yang" {
			continue
		}
		path := filepath.Join(moduleDir, entry.Name())
		if isSubmodule(path) {
			continue
		}
		schemas = append(schemas, path)
	}
	sort.Strings(schemas)
	return schemas, nil
}

func yanglintFormatArg(format backend.Format) (string, error) {
	switch format {
	case backend.FormatXML:
		return "xml", nil
	case backend.FormatJSON, backend.FormatJSONIETF:
		return "json", nil
	default:
		return "", fmt.Errorf("unsupported oracle format: %v", format)
	}
}

func yanglintWithDefaultsArg(wd backend.WithDefaults) string {
	switch wd {
	case backend.WithDefaultsTrim:
		return "trim"
	case backend.WithDefaultsAll:
		return "all"
	case backend.WithDefaultsAllTagged:
		return "all-tagged"
	default:
		return ""
	}
}

func yanglintOpTypeArg(opType string) (string, error) {
	switch strings.ToLower(opType) {
	case "rpc":
		return "rpc", nil
	case "reply":
		return "reply", nil
	case "notification":
		return "notif", nil
	default:
		return "", fmt.Errorf("unknown op-type %q for yanglint oracle", opType)
	}
}

func moduleNameForYANGPath(path string) string {
	stem := strings.TrimSuffix(filepath.Base(path), ".yang")
	if at := strings.IndexByte(stem, '@'); at >= 0 {
		return stem[:at]
	}
	return stem
}

func parseFormat(s string) (backend.Format, error) {
	switch strings.ToLower(s) {
	case "xml":
		return backend.FormatXML, nil
	case "json":
		return backend.FormatJSON, nil
	case "json-ietf", "json_ietf":
		return backend.FormatJSONIETF, nil
	case "lyb":
		return backend.FormatLYB, nil
	default:
		return 0, fmt.Errorf("unknown format: %s", s)
	}
}

func parseWithDefaults(s string) (backend.WithDefaults, error) {
	switch strings.ToLower(s) {
	case "explicit":
		return backend.WithDefaultsExplicit, nil
	case "trim":
		return backend.WithDefaultsTrim, nil
	case "all", "report-all":
		return backend.WithDefaultsAll, nil
	case "all-tagged", "report-all-tagged":
		return backend.WithDefaultsAllTagged, nil
	default:
		return 0, fmt.Errorf("unknown serialize-defaults: %s", s)
	}
}

func parseOpType(s string) (backend.OpType, error) {
	switch strings.ToLower(s) {
	case "rpc":
		return backend.OpTypeRPC, nil
	case "notification":
		return backend.OpTypeNotification, nil
	case "reply":
		return backend.OpTypeReply, nil
	default:
		return 0, fmt.Errorf("unknown op-type: %s", s)
	}
}

func formatBytesForCompare(format backend.Format, b []byte) []byte {
	if format == backend.FormatLYB {
		return b
	}
	return normalize(b)
}

// normalize strips trailing ASCII whitespace, matching the Rust runner.
func normalize(b []byte) []byte {
	return bytes.TrimRight(b, " \t\r\n\v\f")
}

func snippet(b []byte) string {
	if len(b) > 512 {
		b = b[:512]
	}
	return string(b)
}
