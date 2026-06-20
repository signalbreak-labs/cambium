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

	var schemaIR, backend int
	for _, c := range cases {
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

	if schemaIR != 7 {
		t.Errorf("schema-ir cases = %d, want 7", schemaIR)
	}
	if backend == 0 {
		t.Error("no backend-data cases found")
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
			if len(c.Expected) == 0 {
				t.Fatalf("case %q has no expected outputs", c.Name)
			}
			for format, rel := range c.Expected {
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
