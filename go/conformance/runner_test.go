//go:build cgo

package conformance

import (
	"bytes"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/signalbreak-labs/cambium/go/internal/confmanifest"
	cambium "github.com/signalbreak-labs/cambium/go/libyangbackend"
)

// TestScrambledChildren is the headline byte-parity gate: the Go runner must
// reproduce the golden output for scrambled-children exactly (the case where a
// map-based toolkit like goyang would alphabetize). Written red-first.
func TestScrambledChildren(t *testing.T) {
	dir, err := FindConformanceDir()
	if err != nil {
		t.Fatal(err)
	}
	cases, err := LoadManifest(dir + "/manifest.toml")
	if err != nil {
		t.Fatalf("LoadManifest: %v", err)
	}
	var target *Case
	for i := range cases {
		if cases[i].Name == "scrambled-children" {
			target = &cases[i]
			break
		}
	}
	if target == nil {
		t.Fatal("scrambled-children case not found in manifest")
	}
	if err := RunCase(dir, *target); err != nil {
		t.Fatalf("scrambled-children failed byte parity: %v", err)
	}
}

// TestRunSkipsSchemaIRCases asserts that Run ignores schema-ir tier entries
// and still passes a selected backend-data case.
func TestRunSkipsSchemaIRCases(t *testing.T) {
	dir, err := FindConformanceDir()
	if err != nil {
		t.Fatal(err)
	}

	passed, failed, err := Run(dir, []string{"scrambled-children", "schema-cross-kind-order"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !slices.Contains(passed, "scrambled-children") {
		t.Errorf("scrambled-children not in passed list: %v", passed)
	}
	if slices.Contains(passed, "schema-cross-kind-order") {
		t.Errorf("schema-cross-kind-order should be skipped, not passed: %v", passed)
	}
	for _, f := range failed {
		t.Errorf("unexpected failure: %s", f)
	}
}

// TestRunCaseRejectsSchemaIR asserts that calling RunCase directly with a
// schema-ir case returns a clear error instead of crashing on missing input.
func TestRunCaseRejectsSchemaIR(t *testing.T) {
	dir, err := FindConformanceDir()
	if err != nil {
		t.Fatal(err)
	}
	c := confmanifest.Case{
		Name:       "schema-cross-kind-order",
		Tier:       confmanifest.TierSchemaIR,
		Module:     "fixtures/schema-cross-kind-order/module",
		ExpectedIR: "fixtures/schema-cross-kind-order/expected-ir.json",
	}
	if err := RunCase(dir, c); err == nil {
		t.Fatal("RunCase(schema-ir) succeeded, want error")
	} else if err.Error() != `RunCase cannot execute schema-ir case "schema-cross-kind-order"` {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunYanglintOracleBuildsExpectedCommand(t *testing.T) {
	dir := t.TempDir()
	moduleDir := filepath.Join(dir, "module")
	if err := os.MkdirAll(moduleDir, 0o755); err != nil {
		t.Fatal(err)
	}
	modulePath := filepath.Join(moduleDir, "demo@2026-06-18.yang")
	if err := os.WriteFile(modulePath, []byte(`module demo { namespace "urn:demo"; prefix d; revision 2026-06-18; }`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(moduleDir, "demo-sub.yang"), []byte(`submodule demo-sub { belongs-to demo { prefix d; } }`), 0o644); err != nil {
		t.Fatal(err)
	}
	inputPath := filepath.Join(dir, "input.xml")
	if err := os.WriteFile(inputPath, []byte(`<demo/>`), 0o644); err != nil {
		t.Fatal(err)
	}

	argsPath := filepath.Join(dir, "args.txt")
	t.Setenv("ARGS_FILE", argsPath)
	fakeYanglint := filepath.Join(dir, "yanglint")
	script := `#!/bin/sh
: > "$ARGS_FILE"
for arg in "$@"; do
  printf '%s\n' "$arg" >> "$ARGS_FILE"
done
printf '<ok/>'
`
	if err := os.WriteFile(fakeYanglint, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	got, err := runYanglintOracle(fakeYanglint, moduleDir, inputPath, cambium.FormatJSONIETF, cambium.WithDefaultsAllTagged, "notification")
	if err != nil {
		t.Fatalf("runYanglintOracle: %v", err)
	}
	if string(got) != "<ok/>" {
		t.Fatalf("oracle stdout = %q, want <ok/>", got)
	}
	argsRaw, err := os.ReadFile(argsPath)
	if err != nil {
		t.Fatalf("read args: %v", err)
	}
	gotArgs := strings.Split(strings.TrimSuffix(string(argsRaw), "\n"), "\n")
	wantArgs := []string{
		"-X",
		"-p",
		moduleDir,
		"-d",
		"all-tagged",
		"-t",
		"notif",
		"-f",
		"json",
		"-F",
		"demo:",
		modulePath,
		inputPath,
	}
	if strings.Join(gotArgs, "\n") != strings.Join(wantArgs, "\n") {
		t.Fatalf("yanglint args = %#v, want %#v", gotArgs, wantArgs)
	}
}

func TestParseFormatSupportsLYB(t *testing.T) {
	got, err := parseFormat("lyb")
	if err != nil {
		t.Fatalf("parseFormat(lyb): %v", err)
	}
	if got != cambium.FormatLYB {
		t.Fatalf("parseFormat(lyb) = %v, want FormatLYB", got)
	}
}

func TestFormatBytesForComparePreservesLYB(t *testing.T) {
	lyb := []byte{'l', 'y', 'b', 0, '\n', ' '}
	if got := formatBytesForCompare(cambium.FormatLYB, lyb); !bytes.Equal(got, lyb) {
		t.Fatalf("FormatLYB comparison bytes = %v, want exact %v", got, lyb)
	}

	xml := []byte("<ok/>\n")
	if got := formatBytesForCompare(cambium.FormatXML, xml); string(got) != "<ok/>" {
		t.Fatalf("FormatXML comparison bytes = %q, want text-normalized <ok/>", got)
	}
}

// TestEnabledCorpus runs every fixture the scaffold currently enables and
// reports which pass, including operation documents routed through OpType.
func TestEnabledCorpus(t *testing.T) {
	dir, err := FindConformanceDir()
	if err != nil {
		t.Fatal(err)
	}
	enabled := []string{"scrambled-children", "keys-first", "ordered-user", "rpc-order", "system-list-canonical", "ietf-interfaces"}
	passed, failed, err := Run(dir, enabled)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("passed: %v", passed)
	for _, f := range failed {
		t.Errorf("FAIL %s", f)
	}
}
