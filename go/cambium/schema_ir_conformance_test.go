package cambium_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"

	"github.com/signalbreak-labs/cambium/go/cambium"
	"github.com/signalbreak-labs/cambium/go/internal/confmanifest"
)

type schemaIRExpected struct {
	Module              string                         `json:"module"`
	Load                []string                       `json:"load"`
	Children            map[string][]string            `json:"children"`
	DataChildrenFlatten map[string][]string            `json:"dataChildrenFlatten"`
	Keys                map[string][]string            `json:"keys"`
	Imports             map[string][]schemaIRImport    `json:"imports"`
	Prefixes            map[string]map[string]string   `json:"prefixes"`
	IdentityDerived     map[string][]string            `json:"identityDerived"`
	Leafrefs            map[string]schemaIRLeafrefWant `json:"leafrefs"`
}

type schemaIRImport struct {
	Prefix   string `json:"prefix"`
	Name     string `json:"name"`
	Revision string `json:"revision,omitempty"`
}

type schemaIRLeafrefWant struct {
	Path   string `json:"path"`
	Target string `json:"target"`
}

func TestSchemaIRManifestTier(t *testing.T) {
	want := map[string]struct{}{
		"schema-cross-kind-order":  {},
		"schema-uses-site-order":   {},
		"schema-augment-order":     {},
		"schema-choice-case-order": {},
		"schema-import-prefix":     {},
		"schema-identity-derived":  {},
		"schema-leafref-path":      {},
	}

	cases, err := confmanifest.Load(filepath.Join(conformanceRoot(t), "manifest.toml"))
	if err != nil {
		t.Fatalf("Load manifest: %v", err)
	}

	found := make(map[string]struct{})
	for _, c := range cases {
		if c.EffectiveTier() == confmanifest.TierSchemaIR {
			found[c.Name] = struct{}{}
		}
	}

	var missing, extra []string
	for name := range want {
		if _, ok := found[name]; !ok {
			missing = append(missing, name)
		}
	}
	for name := range found {
		if _, ok := want[name]; !ok {
			extra = append(extra, name)
		}
	}

	if len(missing) > 0 || len(extra) > 0 {
		t.Errorf("schema-ir tier mismatch\nmissing: %v\nextra:   %v\nfound:   %v", missing, extra, mapKeys(found))
	}
}

func mapKeys(m map[string]struct{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

func TestSchemaIRConformanceFixtures(t *testing.T) {
	cases, err := confmanifest.Load(filepath.Join(conformanceRoot(t), "manifest.toml"))
	if err != nil {
		t.Fatalf("Load manifest: %v", err)
	}

	for _, c := range cases {
		if c.EffectiveTier() != confmanifest.TierSchemaIR {
			continue
		}
		t.Run(c.Name, func(t *testing.T) {
			runSchemaIRFixture(t, c)
		})
	}
}

func TestSchemaIRManifestMatchesExpectedIRFiles(t *testing.T) {
	root := conformanceRoot(t)
	cases, err := confmanifest.Load(filepath.Join(root, "manifest.toml"))
	if err != nil {
		t.Fatalf("Load manifest: %v", err)
	}
	manifest := map[string]struct{}{}
	for _, c := range cases {
		if c.EffectiveTier() == confmanifest.TierSchemaIR {
			manifest[filepath.ToSlash(c.ExpectedIR)] = struct{}{}
		}
	}

	files := map[string]struct{}{}
	fixtures := filepath.Join(root, "fixtures")
	if err := filepath.WalkDir(fixtures, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || filepath.Base(path) != "expected-ir.json" {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		files[filepath.ToSlash(rel)] = struct{}{}
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	var missing, extra []string
	for file := range files {
		if _, ok := manifest[file]; !ok {
			missing = append(missing, file)
		}
	}
	for file := range manifest {
		if _, ok := files[file]; !ok {
			extra = append(extra, file)
		}
	}
	sort.Strings(missing)
	sort.Strings(extra)
	if len(missing) > 0 || len(extra) > 0 {
		t.Fatalf("schema-ir manifest/files mismatch\nmissing manifest entries for files: %v\nmanifest entries without files: %v", missing, extra)
	}
}

func runSchemaIRFixture(t *testing.T, c confmanifest.Case) {
	t.Helper()
	root := conformanceRoot(t)
	raw, err := os.ReadFile(filepath.Join(root, c.ExpectedIR))
	if err != nil {
		t.Fatal(err)
	}
	var expected schemaIRExpected
	if err := json.Unmarshal(raw, &expected); err != nil {
		t.Fatal(err)
	}
	ctx, err := cambium.NewContext()
	if err != nil {
		t.Fatal(err)
	}
	defer ctx.Close()
	moduleDir := filepath.Join(root, c.Module)
	if err := ctx.SetSearchPath(moduleDir); err != nil {
		t.Fatal(err)
	}
	load := expected.Load
	if len(load) == 0 {
		load = []string{expected.Module}
	}
	for _, module := range load {
		if err := ctx.LoadModule(module); err != nil {
			t.Fatalf("LoadModule(%s): %v", module, err)
		}
	}
	mod, err := ctx.Schema(expected.Module)
	if err != nil {
		t.Fatalf("Schema(%s): %v", expected.Module, err)
	}

	for path, want := range expected.Children {
		node := schemaIRNode(t, mod, path)
		assertStringSlices(t, path+" children", schemaChildNames(node.Children()), want)
	}
	for path, want := range expected.DataChildrenFlatten {
		node := schemaIRNode(t, mod, path)
		assertStringSlices(t, path+" flat data children", schemaChildNames(node.DataChildren(true)), want)
	}
	for path, want := range expected.Keys {
		node := schemaIRNode(t, mod, path)
		assertStringSlices(t, path+" keys", node.KeyNames(), want)
	}
	for moduleName, want := range expected.Imports {
		gotMod, err := ctx.Schema(moduleName)
		if err != nil {
			t.Fatalf("Schema(%s): %v", moduleName, err)
		}
		got := gotMod.Imports()
		if len(got) != len(want) {
			t.Fatalf("%s imports = %+v, want %+v", moduleName, got, want)
		}
		for i := range want {
			if got[i].Prefix != want[i].Prefix || got[i].Name != want[i].Name || got[i].Revision != want[i].Revision {
				t.Fatalf("%s imports = %+v, want %+v", moduleName, got, want)
			}
		}
	}
	for moduleName, checks := range expected.Prefixes {
		gotMod, err := ctx.Schema(moduleName)
		if err != nil {
			t.Fatalf("Schema(%s): %v", moduleName, err)
		}
		for prefix, want := range checks {
			resolved, ok := gotMod.ResolvePrefix(prefix)
			if !ok || resolved.Name() != want {
				t.Fatalf("%s ResolvePrefix(%q) = (%q,%v), want %q,true", moduleName, prefix, resolved.Name(), ok, want)
			}
		}
	}
	for key, want := range expected.IdentityDerived {
		moduleName, identName, ok := strings.Cut(key, ":")
		if !ok {
			t.Fatalf("identity key %q must be module:identity", key)
		}
		gotMod, err := ctx.Schema(moduleName)
		if err != nil {
			t.Fatalf("Schema(%s): %v", moduleName, err)
		}
		var found cambium.Identity
		for id := range gotMod.Identities() {
			if id.Name() == identName {
				found = id
				break
			}
		}
		if found.Name() == "" {
			t.Fatalf("identity %s not found", key)
		}
		assertStringSlices(t, key+" derived", namesOf(found.Derived()), want)
	}
	for path, want := range expected.Leafrefs {
		node := schemaIRNode(t, mod, path)
		info, ok := node.LeafType()
		if !ok {
			t.Fatalf("%s has no leaf type", path)
		}
		lr, ok := info.Resolved().(cambium.ResolvedLeafRef)
		if !ok {
			t.Fatalf("%s type = %T, want ResolvedLeafRef", path, info.Resolved())
		}
		gotPath, ok := lr.Path()
		if !ok || gotPath != want.Path {
			t.Fatalf("%s leafref path = (%q,%v), want %q,true", path, gotPath, ok, want.Path)
		}
		target, ok := lr.Target()
		if !ok || target.Path() != want.Target {
			t.Fatalf("%s leafref target = (%q,%v), want %q,true", path, target.Path(), ok, want.Target)
		}
	}
}

func schemaIRNode(t *testing.T, mod cambium.Module, path string) cambium.SchemaNodeRef {
	t.Helper()
	node, err := mod.FindPath(path)
	if err != nil {
		t.Fatalf("FindPath(%s): %v", path, err)
	}
	return node
}

func assertStringSlices(t *testing.T, label string, got, want []string) {
	t.Helper()
	if strings.Join(got, "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("%s = %v, want %v", label, got, want)
	}
}

func conformanceRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", "conformance"))
}

func schemaIRCaseByName(t *testing.T, name string) confmanifest.Case {
	t.Helper()
	cases, err := confmanifest.Load(filepath.Join(conformanceRoot(t), "manifest.toml"))
	if err != nil {
		t.Fatalf("Load manifest: %v", err)
	}
	for _, c := range cases {
		if c.Name == name {
			return c
		}
	}
	t.Fatalf("schema-ir fixture %q not found in manifest", name)
	return confmanifest.Case{}
}

func runSchemaIRFixtureByName(t *testing.T, name string) {
	t.Helper()
	runSchemaIRFixture(t, schemaIRCaseByName(t, name))
}

func TestSchemaChildrenPreserveCrossKindDeclarationOrder(t *testing.T) {
	runSchemaIRFixtureByName(t, "schema-cross-kind-order")
}

func TestGroupingUsesExpansionOrder(t *testing.T) {
	runSchemaIRFixtureByName(t, "schema-uses-site-order")
}

func TestAugmentPlacementPureGoGolden(t *testing.T) {
	runSchemaIRFixtureByName(t, "schema-augment-order")
}

func TestChoiceCaseOrder(t *testing.T) {
	runSchemaIRFixtureByName(t, "schema-choice-case-order")
}
