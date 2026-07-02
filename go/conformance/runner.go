// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

//go:build cgo

// Package conformance runs Cambium's shared /conformance corpus. It reads
// manifest.toml, parses each fixture through the libyang backend, and asserts
// byte-for-byte equality with the golden outputs (after trailing-whitespace
// normalization).
package conformance

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	core "github.com/signalbreak-labs/cambium/go/cambium"
	"github.com/signalbreak-labs/cambium/go/datatree"
	"github.com/signalbreak-labs/cambium/go/gnmi"
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
	outputs, err := backendCaseOutputs(conformanceDir, c)
	if err != nil {
		return err
	}
	for _, actual := range outputs {
		goldenPath := filepath.Join(conformanceDir, c.Expected[actual.name])
		expected, err := os.ReadFile(goldenPath)
		if err != nil {
			return fmt.Errorf("read golden %s: %w", goldenPath, err)
		}
		if !bytes.Equal(formatBytesForCompare(actual.format, expected), formatBytesForCompare(actual.format, actual.data)) {
			return fmt.Errorf(
				"%s output does not match golden %s\n--- expected ---\n%s\n--- actual ---\n%s",
				actual.name, goldenPath, snippet(expected), snippet(actual.data),
			)
		}
		if c.Oracle {
			yanglint := strings.TrimSpace(os.Getenv("CAMBIUM_YANGLINT"))
			if yanglint != "" {
				oracle, err := runYanglintOracle(yanglint, filepath.Join(conformanceDir, c.Module), filepath.Join(conformanceDir, c.Input), actual.format, actual.flags.WithDefaults, c.OpType)
				if err != nil {
					return err
				}
				if !bytes.Equal(formatBytesForCompare(actual.format, oracle), formatBytesForCompare(actual.format, actual.data)) {
					return fmt.Errorf(
						"%s output differs from yanglint oracle\n--- yanglint ---\n%s\n--- cambium ---\n%s",
						actual.name, snippet(oracle), snippet(actual.data),
					)
				}
			}
		}
	}
	return nil
}

type caseOutput struct {
	name   string
	format backend.Format
	flags  backend.SerializeFlags
	data   []byte
}

func backendCaseOutputs(conformanceDir string, c Case) ([]caseOutput, error) {
	if c.Input == "" {
		return nil, fmt.Errorf("case %q has no input", c.Name)
	}
	if c.InputFormat == "" {
		return nil, fmt.Errorf("case %q has no input-format", c.Name)
	}

	moduleDir := filepath.Join(conformanceDir, c.Module)
	inputPath := filepath.Join(conformanceDir, c.Input)

	ctx, err := backend.NewContext()
	if err != nil {
		return nil, err
	}
	defer ctx.Close()
	if err := ctx.SetSearchPath(moduleDir); err != nil {
		return nil, err
	}
	if err := loadModulesInDir(ctx, moduleDir); err != nil {
		return nil, err
	}

	input, err := os.ReadFile(inputPath)
	if err != nil {
		return nil, fmt.Errorf("read input: %w", err)
	}
	inFmt, err := parseFormat(c.InputFormat)
	if err != nil {
		return nil, err
	}

	var tree *backend.DataTree
	if c.OpType != "" {
		op, err := parseOpType(c.OpType)
		if err != nil {
			return nil, err
		}
		tree, err = ctx.ParseOp(inFmt, op, input)
		if err != nil {
			return nil, err
		}
	} else {
		tree, err = ctx.Parse(inFmt, backend.ParseModeDataOnly, input)
		if err != nil {
			return nil, err
		}
	}
	defer tree.Close()

	formats := make([]string, 0, len(c.Expected))
	for f := range c.Expected {
		formats = append(formats, f)
	}
	sort.Strings(formats)

	outputs := make([]caseOutput, 0, len(formats))
	for _, fmtName := range formats {
		if fmtName == "gnmi-json-ietf" {
			if c.GNMIPath == "" {
				return nil, fmt.Errorf("case %q has gnmi-json-ietf output but no gnmi-path", c.Name)
			}
			flags := backend.DefaultSerializeFlags()
			update, err := gnmi.JSONIETFAtomicUpdate(tree, c.GNMIPath, flags)
			if err != nil {
				return nil, err
			}
			actual, err := json.MarshalIndent(update, "", "  ")
			if err != nil {
				return nil, fmt.Errorf("marshal gnmi update: %w", err)
			}
			actual = append(actual, '\n')
			outputs = append(outputs, caseOutput{name: fmtName, format: backend.FormatJSONIETF, flags: flags, data: actual})
			continue
		}
		outFmt, err := parseFormat(fmtName)
		if err != nil {
			return nil, err
		}
		flags := backend.DefaultSerializeFlags()
		if c.SerializeDefaults != "" {
			flags.WithDefaults, err = parseWithDefaults(c.SerializeDefaults)
			if err != nil {
				return nil, err
			}
		}
		actual, err := tree.Serialize(outFmt, flags)
		if err != nil {
			return nil, err
		}
		outputs = append(outputs, caseOutput{name: fmtName, format: outFmt, flags: flags, data: actual})
	}
	return outputs, nil
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

// RunDataTreeDifferential executes datatree-opted backend-data cases through
// both the libyang backend and the pure-Go datatree, comparing normalized
// serialized outputs. Cases that are selected but not opted in are reported as
// skipped.
func RunDataTreeDifferential(conformanceDir string, only []string) (passed, skipped, failed []string, err error) {
	cases, err := LoadManifest(filepath.Join(conformanceDir, "manifest.toml"))
	if err != nil {
		return nil, nil, nil, err
	}
	enabled := map[string]bool{}
	for _, n := range only {
		enabled[n] = true
	}
	for _, c := range cases {
		if len(only) > 0 && !enabled[c.Name] {
			continue
		}
		if c.EffectiveTier() != confmanifest.TierBackendData || !c.DataTree {
			if len(only) > 0 {
				skipped = append(skipped, c.Name)
			}
			continue
		}
		if e := RunDataTreeDifferentialCase(conformanceDir, c); e != nil {
			failed = append(failed, fmt.Sprintf("%s: %v", c.Name, e))
		} else {
			passed = append(passed, c.Name)
		}
	}
	return passed, skipped, failed, nil
}

// RunDataTreeDifferentialCase compares one datatree-opted backend-data fixture
// against the libyang backend.
func RunDataTreeDifferentialCase(conformanceDir string, c Case) error {
	if c.EffectiveTier() == confmanifest.TierSchemaIR {
		return fmt.Errorf("RunDataTreeDifferentialCase cannot execute schema-ir case %q", c.Name)
	}
	if !c.DataTree {
		return fmt.Errorf("case %q is not marked datatree=true", c.Name)
	}
	backendOutputs, err := backendCaseOutputs(conformanceDir, c)
	if err != nil {
		return fmt.Errorf("backend: %w", err)
	}
	dataTreeOutputs, err := dataTreeCaseOutputs(conformanceDir, c)
	if err != nil {
		return fmt.Errorf("datatree: %w", err)
	}
	dataByName := make(map[string]caseOutput, len(dataTreeOutputs))
	for _, out := range dataTreeOutputs {
		dataByName[out.name] = out
	}
	for _, want := range backendOutputs {
		got, ok := dataByName[want.name]
		if !ok {
			return fmt.Errorf("%s output missing from datatree", want.name)
		}
		if !bytes.Equal(formatBytesForDifferential(want.format, want.data), formatBytesForDifferential(want.format, got.data)) {
			return fmt.Errorf(
				"%s output differs between backend and datatree\n--- backend ---\n%s\n--- datatree ---\n%s",
				want.name, snippet(want.data), snippet(got.data),
			)
		}
	}
	return nil
}

func dataTreeCaseOutputs(conformanceDir string, c Case) ([]caseOutput, error) {
	if c.OpType != "" {
		return nil, fmt.Errorf("operation documents are not supported")
	}
	if c.SerializeDefaults != "" {
		return nil, fmt.Errorf("with-defaults serialization is not supported")
	}
	if c.Input == "" {
		return nil, fmt.Errorf("case %q has no input", c.Name)
	}
	if c.InputFormat == "" {
		return nil, fmt.Errorf("case %q has no input-format", c.Name)
	}

	moduleDir := filepath.Join(conformanceDir, c.Module)
	inputPath := filepath.Join(conformanceDir, c.Input)
	ctx, err := core.NewContext()
	if err != nil {
		return nil, err
	}
	defer ctx.Close()
	if err := ctx.SetSearchPath(moduleDir); err != nil {
		return nil, err
	}
	if err := loadModulesInDirPure(ctx, moduleDir); err != nil {
		return nil, err
	}

	input, err := os.ReadFile(inputPath)
	if err != nil {
		return nil, fmt.Errorf("read input: %w", err)
	}
	inFmt, err := parseDataTreeFormat(c.InputFormat)
	if err != nil {
		return nil, err
	}
	mod, err := dataTreeModuleForInput(ctx, c.InputFormat, input)
	if err != nil {
		return nil, err
	}
	tree, err := datatree.Parse(mod, inFmt, input)
	if err != nil {
		return nil, err
	}

	formats := make([]string, 0, len(c.Expected))
	for f := range c.Expected {
		formats = append(formats, f)
	}
	sort.Strings(formats)

	outputs := make([]caseOutput, 0, len(formats))
	for _, fmtName := range formats {
		dtFmt, backendFmt, err := parseDataTreeOutputFormat(fmtName)
		if err != nil {
			return nil, err
		}
		data, err := tree.Serialize(dtFmt)
		if err != nil {
			return nil, err
		}
		outputs = append(outputs, caseOutput{name: fmtName, format: backendFmt, data: data})
	}
	return outputs, nil
}

func loadModulesInDirPure(ctx *core.Context, dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("read module dir: %w", err)
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if filepath.Ext(e.Name()) != ".yang" {
			continue
		}
		if isSubmodule(filepath.Join(dir, e.Name())) {
			continue
		}
		names = append(names, e.Name())
	}
	sort.Strings(names)
	for _, n := range names {
		stem := strings.TrimSuffix(n, ".yang")
		if at := strings.IndexByte(stem, '@'); at >= 0 {
			stem = stem[:at]
		}
		if err := ctx.LoadModule(stem); err != nil {
			return err
		}
	}
	return nil
}

func dataTreeModuleForInput(ctx *core.Context, format string, input []byte) (core.Module, error) {
	switch strings.ToLower(format) {
	case "xml":
		dec := xml.NewDecoder(bytes.NewReader(input))
		for {
			tok, err := dec.Token()
			if errors.Is(err, io.EOF) {
				break
			}
			if err != nil {
				return core.Module{}, err
			}
			if start, ok := tok.(xml.StartElement); ok {
				if mod, ok := ctx.FindModuleByNamespace(start.Name.Space); ok {
					return mod, nil
				}
				return core.Module{}, fmt.Errorf("no module loaded for namespace %q", start.Name.Space)
			}
		}
	case "json", "json-ietf", "json_ietf":
		var raw map[string]json.RawMessage
		if err := json.Unmarshal(input, &raw); err != nil {
			return core.Module{}, err
		}
		names := make([]string, 0, len(raw))
		for name := range raw {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			if i := strings.IndexByte(name, ':'); i > 0 {
				if mod, err := ctx.Schema(name[:i]); err == nil {
					return mod, nil
				}
			}
		}
	}
	mods := ctx.Modules()
	if len(mods) == 1 {
		return mods[0], nil
	}
	return core.Module{}, fmt.Errorf("cannot infer datatree module from %s input", format)
}

func parseDataTreeFormat(s string) (datatree.Format, error) {
	switch strings.ToLower(s) {
	case "xml":
		return datatree.FormatXML, nil
	case "json", "json-ietf", "json_ietf":
		return datatree.FormatJSONIETF, nil
	default:
		return 0, fmt.Errorf("unsupported datatree input format: %s", s)
	}
}

func parseDataTreeOutputFormat(s string) (dtFormat datatree.Format, backendFormat backend.Format, err error) {
	switch strings.ToLower(s) {
	case "xml":
		return datatree.FormatXML, backend.FormatXML, nil
	case "json", "json-ietf", "json_ietf":
		backendFmt, err := parseFormat(s)
		return datatree.FormatJSONIETF, backendFmt, err
	default:
		return 0, 0, fmt.Errorf("unsupported datatree output format: %s", s)
	}
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
	// yanglint is the operator-supplied oracle path from CAMBIUM_YANGLINT (CI/test
	// harness only). Resolve it to an executable so a missing or non-runnable path
	// fails here rather than as an opaque exec error.
	bin, err := exec.LookPath(yanglint)
	if err != nil {
		return nil, fmt.Errorf("resolve yanglint %q: %w", yanglint, err)
	}
	cmd := exec.Command(bin) //nolint:gosec // resolved trusted oracle path from CAMBIUM_YANGLINT
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
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
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

func formatBytesForDifferential(format backend.Format, b []byte) []byte {
	b = normalize(b)
	switch format {
	case backend.FormatJSON, backend.FormatJSONIETF:
		var buf bytes.Buffer
		if err := json.Compact(&buf, b); err == nil {
			return buf.Bytes()
		}
	case backend.FormatXML:
		return compactXMLWhitespace(b)
	}
	return b
}

func compactXMLWhitespace(b []byte) []byte {
	dec := xml.NewDecoder(bytes.NewReader(b))
	var out bytes.Buffer
	enc := xml.NewEncoder(&out)
	for {
		tok, err := dec.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return b
		}
		if chars, ok := tok.(xml.CharData); ok && strings.TrimSpace(string(chars)) == "" {
			continue
		}
		if err := enc.EncodeToken(tok); err != nil {
			return b
		}
	}
	if err := enc.Flush(); err != nil {
		return b
	}
	return out.Bytes()
}

// normalize strips trailing ASCII whitespace, matching the conformance contract.
func normalize(b []byte) []byte {
	return bytes.TrimRight(b, " \t\r\n\v\f")
}

func snippet(b []byte) string {
	if len(b) > 512 {
		b = b[:512]
	}
	return string(b)
}
