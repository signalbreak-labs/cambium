// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

package confmanifest

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadSharedManifest(t *testing.T) {
	// Walk up to the workspace root from this internal package.
	root := filepath.Join("..", "..", "..")
	cases, err := Load(filepath.Join(root, "conformance", "manifest.toml"))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	var schemaIR, backend, reject int
	for _, c := range cases {
		if c.EffectiveExpect() == ExpectReject {
			reject++
		}
		switch c.EffectiveTier() {
		case TierSchemaIR:
			schemaIR++
			if c.Module == "" {
				t.Errorf("schema-ir case %q missing module", c.Name)
			}
			if c.ExpectedIR == "" {
				t.Errorf("schema-ir case %q missing expected-ir", c.Name)
			}
			if c.Input != "" {
				t.Errorf("schema-ir case %q must not have input", c.Name)
			}
			if c.InputFormat != "" {
				t.Errorf("schema-ir case %q must not have input-format", c.Name)
			}
			if len(c.Expected) != 0 {
				t.Errorf("schema-ir case %q must not have expected map", c.Name)
			}
		case TierBackendData:
			backend++
		default:
			t.Errorf("unknown tier %q for case %q", c.Tier, c.Name)
		}
	}

	if schemaIR != 13 {
		t.Errorf("schema-ir cases = %d, want 13", schemaIR)
	}
	if backend == 0 {
		t.Error("no backend-data cases found")
	}
	if reject == 0 {
		t.Error(`no expect = "reject" cases found; validation verdicts are never compared`)
	}
}

func TestSharedManifestReferencesExistingFiles(t *testing.T) {
	root := filepath.Join("..", "..", "..", "conformance")
	cases, err := Load(filepath.Join(root, "manifest.toml"))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	seen := map[string]bool{}
	for _, c := range cases {
		if c.Name == "" {
			t.Fatal("manifest contains unnamed case")
		}
		if seen[c.Name] {
			t.Fatalf("manifest contains duplicate case name %q", c.Name)
		}
		seen[c.Name] = true

		assertPathExists(t, root, c.Name, "module", c.Module)
		assertModuleDirContainsYANG(t, root, c.Name, c.Module)
		switch c.EffectiveTier() {
		case TierSchemaIR:
			assertPathExists(t, root, c.Name, "expected-ir", c.ExpectedIR)
		case TierBackendData:
			assertPathExists(t, root, c.Name, "input", c.Input)
			if c.InputFormat == "" {
				t.Fatalf("case %q has no input-format", c.Name)
			}
			if c.EffectiveExpect() == ExpectReject {
				// Load refuses expected outputs on a reject case.
				continue
			}
			if len(c.Expected) == 0 {
				t.Fatalf("case %q has no expected outputs", c.Name)
			}
			for format, rel := range c.Expected {
				if format == "gnmi-json-ietf" && c.GNMIPath == "" {
					t.Fatalf("case %q has gnmi-json-ietf output but no gnmi-path", c.Name)
				}
				assertPathExists(t, root, c.Name, "expected "+format, rel)
			}
		default:
			t.Fatalf("case %q has unsupported tier %q", c.Name, c.Tier)
		}
	}
}

func assertModuleDirContainsYANG(t *testing.T, root, caseName, rel string) {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join(root, rel))
	if err != nil {
		t.Fatalf("case %q module path %q is not readable: %v", caseName, rel, err)
	}
	for _, entry := range entries {
		if !entry.IsDir() && filepath.Ext(entry.Name()) == ".yang" {
			return
		}
	}
	t.Fatalf("case %q module path %q contains no .yang files", caseName, rel)
}

func assertPathExists(t *testing.T, root, caseName, field, rel string) {
	t.Helper()
	if rel == "" {
		t.Fatalf("case %q missing %s path", caseName, field)
	}
	if filepath.IsAbs(rel) {
		t.Fatalf("case %q %s path %q must be relative", caseName, field, rel)
	}
	if _, err := os.Stat(filepath.Join(root, rel)); err != nil {
		t.Fatalf("case %q %s path %q does not exist: %v", caseName, field, rel, err)
	}
}

func TestEffectiveTierDefaultsToBackendData(t *testing.T) {
	c := Case{}
	if got := c.EffectiveTier(); got != TierBackendData {
		t.Errorf("EffectiveTier() = %q, want %q", got, TierBackendData)
	}
}

func TestLoadParsesDataTreeFlag(t *testing.T) {
	manifest := `[[case]]
name = "datatree-opt-in"
module = "fixtures/datatree-opt-in/module"
input = "fixtures/datatree-opt-in/input.xml"
input-format = "xml"
datatree = true
[case.expected]
xml = "golden/datatree-opt-in/output.xml"
`
	tmp, err := os.CreateTemp("", "confmanifest-datatree-*.toml")
	if err != nil {
		t.Fatalf("CreateTemp: %v", err)
	}
	defer func() { _ = os.Remove(tmp.Name()) }()

	if _, err := tmp.WriteString(manifest); err != nil {
		t.Fatalf("WriteString: %v", err)
	}
	_ = tmp.Close()

	cases, err := Load(tmp.Name())
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(cases) != 1 {
		t.Fatalf("cases = %d, want 1", len(cases))
	}
	if !cases[0].DataTree {
		t.Fatalf("DataTree = false, want true")
	}
}

func TestLoadParsesGNMIPath(t *testing.T) {
	manifest := `[[case]]
name = "gnmi"
module = "fixtures/gnmi/module"
input = "fixtures/gnmi/input.xml"
input-format = "xml"
gnmi-path = "/gnmi:top/rule"
[case.expected]
gnmi-json-ietf = "golden/gnmi/output.gnmi.json"
`
	tmp, err := os.CreateTemp("", "confmanifest-gnmi-*.toml")
	if err != nil {
		t.Fatalf("CreateTemp: %v", err)
	}
	defer func() { _ = os.Remove(tmp.Name()) }()

	if _, err := tmp.WriteString(manifest); err != nil {
		t.Fatalf("WriteString: %v", err)
	}
	_ = tmp.Close()

	cases, err := Load(tmp.Name())
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(cases) != 1 {
		t.Fatalf("cases = %d, want 1", len(cases))
	}
	if cases[0].GNMIPath != "/gnmi:top/rule" {
		t.Fatalf("GNMIPath = %q", cases[0].GNMIPath)
	}
}

func TestLoadRejectsInvalidTier(t *testing.T) {
	manifest := `[[case]]
name = "bad-tier"
tier = "schema-irx"
module = "foo.yang"
`
	tmp, err := os.CreateTemp("", "confmanifest-invalid-tier-*.toml")
	if err != nil {
		t.Fatalf("CreateTemp: %v", err)
	}
	defer func() { _ = os.Remove(tmp.Name()) }()

	if _, err := tmp.WriteString(manifest); err != nil {
		t.Fatalf("WriteString: %v", err)
	}
	_ = tmp.Close()

	_, err = Load(tmp.Name())
	if err == nil {
		t.Fatal("Load returned no error for invalid tier")
	}
	msg := err.Error()
	if !strings.Contains(msg, "bad-tier") {
		t.Errorf("error %q does not contain case name %q", msg, "bad-tier")
	}
	if !strings.Contains(msg, "schema-irx") {
		t.Errorf("error %q does not contain tier value %q", msg, "schema-irx")
	}
}

func writeTempManifest(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "manifest.toml")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	return path
}

func TestEffectiveExpectDefaultsToAccept(t *testing.T) {
	if got := (Case{}).EffectiveExpect(); got != ExpectAccept {
		t.Errorf("EffectiveExpect() = %q, want %q", got, ExpectAccept)
	}
}

func TestLoadParsesExpectReject(t *testing.T) {
	cases, err := Load(writeTempManifest(t, `[[case]]
name = "accept-me"
expect = "accept"
module = "fixtures/accept-me/module"
input = "fixtures/accept-me/input.json"
input-format = "json_ietf"
[case.expected]
json_ietf = "golden/accept-me/output.json_ietf"

[[case]]
name = "reject-me"
expect = "reject"
module = "fixtures/reject-me/module"
input = "fixtures/reject-me/input.json"
input-format = "json_ietf"
oracle = true
datatree = true
`))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(cases) != 2 {
		t.Fatalf("cases = %d, want 2", len(cases))
	}
	if got := cases[0].EffectiveExpect(); got != ExpectAccept {
		t.Errorf("%s: EffectiveExpect() = %q, want %q", cases[0].Name, got, ExpectAccept)
	}
	reject := cases[1]
	if reject.Expect != ExpectReject || reject.EffectiveExpect() != ExpectReject {
		t.Errorf("%s: Expect = %q, want %q", reject.Name, reject.Expect, ExpectReject)
	}
	if reject.EffectiveTier() != TierBackendData || !reject.DataTree || !reject.Oracle {
		t.Errorf("%s: tier/datatree/oracle = %q/%v/%v, want backend-data/true/true",
			reject.Name, reject.EffectiveTier(), reject.DataTree, reject.Oracle)
	}
	if len(reject.Expected) != 0 {
		t.Errorf("%s: Expected = %v, want none", reject.Name, reject.Expected)
	}
}

func TestLoadRejectsInvalidExpect(t *testing.T) {
	_, err := Load(writeTempManifest(t, `[[case]]
name = "bad-expect"
expect = "fail"
module = "fixtures/bad-expect/module"
input = "fixtures/bad-expect/input.json"
input-format = "json_ietf"
`))
	if err == nil {
		t.Fatal("Load returned no error for invalid expect")
	}
	for _, want := range []string{"bad-expect", `"fail"`} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not contain %s", err, want)
		}
	}
}

// A reject case is a data document with no outputs: fields that only make
// sense for serialized output (or that the verdict does not cover yet) are a
// manifest error, not something a runner silently ignores.
func TestLoadRejectsMalformedRejectCases(t *testing.T) {
	const head = `[[case]]
name = "bad-reject"
expect = "reject"
module = "fixtures/bad-reject/module"
input = "fixtures/bad-reject/input.json"
input-format = "json_ietf"
`
	for _, tc := range []struct {
		name, extra, want string
	}{
		{"expected outputs", "[case.expected]\njson_ietf = \"golden/bad-reject/output.json_ietf\"\n", "expected output"},
		{"schema-ir tier", "tier = \"schema-ir\"\n", "tier"},
		{"op-type", "op-type = \"rpc\"\n", "op-type"},
		{"serialize-defaults", "serialize-defaults = \"trim\"\n", "serialize-defaults"},
		{"gnmi-path", "gnmi-path = \"/m:top\"\n", "gnmi-path"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Load(writeTempManifest(t, head+tc.extra))
			if err == nil {
				t.Fatal("Load returned no error")
			}
			for _, want := range []string{"bad-reject", tc.want} {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error %q does not contain %q", err, want)
				}
			}
		})
	}
}

// requiredInvariants are the ordering invariants that MUST each have at least
// one passing conformance fixture.
var requiredInvariants = []string{"I1", "I2", "I3", "I4", "I5", "I6"}

// G-08: the I1-I6 -> fixture mapping must be machine-checkable, not prose-only.
// Cases carry an `invariants` tag in manifest.toml; this fails if a required
// invariant loses its last fixture, or if tagging is dropped entirely.
func TestInvariantsCoverAllRequired(t *testing.T) {
	cases, err := Load(filepath.Join("..", "..", "..", "conformance", "manifest.toml"))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	seen := map[string]string{} // invariant -> first fixture that tags it
	tagged := 0
	for _, c := range cases {
		if len(c.Invariants) > 0 {
			tagged++
		}
		for _, inv := range c.Invariants {
			if _, ok := seen[inv]; !ok {
				seen[inv] = c.Name
			}
		}
	}

	if tagged == 0 {
		t.Fatal("no manifest case carries an `invariants` tag; the I1-I6 -> fixture mapping is not machine-checkable")
	}
	for _, inv := range requiredInvariants {
		if _, ok := seen[inv]; !ok {
			t.Errorf("ordering invariant %s has no tagged conformance fixture; tag one with `invariants` in conformance/manifest.toml", inv)
		}
	}
}
