// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

package cambium_test

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/signalbreak-labs/cambium/go/cambium"
)

// S4: loading, features, deviations and completeness. Each test varies one
// policy at a time.

func writeYANG(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, source := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(source), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

type loadSpec struct {
	search   []string
	roots    []string
	features map[string][]string
	policy   cambium.DeviationPolicy
	mode     cambium.ValidationMode
}

func buildFromDisk(t *testing.T, spec loadSpec) (*cambium.Context, error) {
	t.Helper()
	builder, err := cambium.NewContextBuilder(cambium.ContextFlags{DisableSearchdirCwd: true})
	if err != nil {
		t.Fatal(err)
	}
	for _, dir := range spec.search {
		if err := builder.SearchPath(dir); err != nil {
			t.Fatal(err)
		}
	}
	if err := builder.SetValidationMode(spec.mode); err != nil {
		t.Fatal(err)
	}
	if err := builder.SetDeviationPolicy(spec.policy); err != nil {
		t.Fatal(err)
	}
	for _, root := range spec.roots {
		name, rev, pinned := strings.Cut(root, "@")
		var revision *string
		if pinned {
			revision = &rev
		}
		if err := builder.LoadModule(name, revision, spec.features[name]); err != nil {
			return nil, err
		}
	}
	for module, names := range spec.features {
		if err := builder.SetFeatures(module, names); err != nil {
			return nil, err
		}
	}
	ctx, err := builder.Build()
	if err == nil {
		t.Cleanup(ctx.Close)
	}
	return ctx, err
}

func loadInfoNames(infos []cambium.ModuleLoadInfo) []string {
	var out []string
	for _, info := range infos {
		name := info.Name
		if info.Revision != "" {
			name += "@" + info.Revision
		}
		out = append(out, name)
	}
	return out
}

const s4DepOld = `module dep {
  yang-version 1.1; namespace "urn:s4:dep"; prefix dep;
  revision 2024-01-01;
  typedef id { type uint8; }
}`

const s4DepNew = `module dep {
  yang-version 1.1; namespace "urn:s4:dep"; prefix dep;
  revision 2025-01-01;
  revision 2024-01-01;
  typedef id { type string; }
}`

func TestS4OrderedSearchPathsAndRevisionSelection(t *testing.T) {
	root := t.TempDir()
	first := filepath.Join(root, "first")
	second := filepath.Join(root, "second")
	writeYANG(t, first, map[string]string{"dep@2024-01-01.yang": s4DepOld})
	writeYANG(t, second, map[string]string{"dep@2025-01-01.yang": s4DepNew, "user.yang": `module user {
  yang-version 1.1; namespace "urn:s4:user"; prefix u;
  import dep { prefix dep; revision-date 2024-01-01; }
  leaf v { type dep:id; }
}`, "latest-user.yang": `module latest-user {
  yang-version 1.1; namespace "urn:s4:latest-user"; prefix lu;
  import dep { prefix dep; }
  leaf v { type dep:id; }
}`})

	// Revision-pinned import selects the exact dependency revision and its content.
	ctx, err := buildFromDisk(t, loadSpec{search: []string{first, second}, roots: []string{"user"}})
	if err != nil {
		t.Fatalf("Build pinned: %v", err)
	}
	user, _ := ctx.Schema("user")
	if base := leafType(t, user, "/u:v").Base(); base != cambium.BaseTypeUint8 {
		t.Fatalf("pinned import resolved typedef base %v, want uint8 from dep@2024-01-01", base)
	}
	report := ctx.LoadReport()
	if got, want := loadInfoNames(report.RequestedModules), []string{"user"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("requested = %v, want %v", got, want)
	}
	if got, want := loadInfoNames(report.TransitiveImports), []string{"dep@2024-01-01"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("transitive = %v, want %v", got, want)
	}
	if report.TransitiveImports[0].Implemented {
		t.Fatal("an import-only dependency is reported as implemented")
	}
	var implemented []string
	for _, m := range ctx.Modules() {
		implemented = append(implemented, m.Name())
	}
	if !reflect.DeepEqual(implemented, []string{"user"}) {
		t.Fatalf("Modules() = %v, want implemented modules only", implemented)
	}

	// An unpinned import resolves in search-path order with latest-revision
	// preference; both files are candidates.
	ctx, err = buildFromDisk(t, loadSpec{search: []string{second, first}, roots: []string{"latest-user"}})
	if err != nil {
		t.Fatalf("Build unpinned: %v", err)
	}
	lu, _ := ctx.Schema("latest-user")
	if base := leafType(t, lu, "/lu:v").Base(); base != cambium.BaseTypeString {
		t.Fatalf("unpinned import base %v, want string from dep@2025-01-01", base)
	}

	// Loading the same explicit root twice is idempotent.
	builder, _ := cambium.NewContextBuilder(cambium.ContextFlags{DisableSearchdirCwd: true})
	_ = builder.SearchPath(first)
	_ = builder.SearchPath(second)
	for i := 0; i < 2; i++ {
		if err := builder.LoadModule("user", nil, nil); err != nil {
			t.Fatalf("repeat LoadModule %d: %v", i, err)
		}
	}
	ctx, err = builder.Build()
	if err != nil {
		t.Fatal(err)
	}
	defer ctx.Close()
	if got := loadInfoNames(ctx.LoadReport().RequestedModules); !reflect.DeepEqual(got, []string{"user"}) {
		t.Fatalf("repeated root requested = %v", got)
	}
}

func TestS4MissingImportAndRevisionConflict(t *testing.T) {
	root := t.TempDir()
	writeYANG(t, root, map[string]string{
		"dep@2024-01-01.yang": s4DepOld,
		"dep@2025-01-01.yang": s4DepNew,
		"orphan.yang": `module orphan {
  yang-version 1.1; namespace "urn:s4:orphan"; prefix o;
  import absent { prefix a; }
}`,
		"old-user.yang": `module old-user {
  yang-version 1.1; namespace "urn:s4:old-user"; prefix ou;
  import dep { prefix dep; revision-date 2024-01-01; }
}`,
		"new-user.yang": `module new-user {
  yang-version 1.1; namespace "urn:s4:new-user"; prefix nu;
  import dep { prefix dep; revision-date 2025-01-01; }
  leaf v { type dep:id; }
}`,
	})
	_, err := buildFromDisk(t, loadSpec{search: []string{root}, roots: []string{"orphan"}})
	if err == nil {
		t.Fatal("missing import succeeded")
	}
	if diag := cambium.DiagnosticFromError(err); diag.Kind != cambium.DiagnosticMissingModule {
		t.Fatalf("missing import kind = %q (%v)", diag.Kind, err)
	}
	// Two importers pinning different revisions of one dependency: import-only
	// revisions may coexist, each importer sees its own pinned content.
	ctx, err := buildFromDisk(t, loadSpec{search: []string{root}, roots: []string{"old-user", "new-user"}})
	if err != nil {
		t.Fatalf("coexisting import revisions: %v", err)
	}
	nu, _ := ctx.Schema("new-user")
	if base := leafType(t, nu, "/nu:v").Base(); base != cambium.BaseTypeString {
		t.Fatalf("new-user sees dep base %v, want string", base)
	}
	if got := loadInfoNames(ctx.LoadReport().TransitiveImports); !reflect.DeepEqual(got, []string{"dep@2024-01-01", "dep@2025-01-01"}) {
		t.Fatalf("transitive = %v", got)
	}
	// A Cambium context may hold two explicitly requested revisions of one
	// module (schema tooling such as diffs relies on it). This differs from a
	// server, which implements at most one revision; callers that need that
	// shape detect it from the requested roots.
	ctx, err = buildFromDisk(t, loadSpec{search: []string{root}, roots: []string{"dep@2024-01-01", "dep@2025-01-01"}})
	if err != nil {
		t.Fatalf("two requested revisions: %v", err)
	}
	if got := loadInfoNames(ctx.LoadReport().RequestedModules); !reflect.DeepEqual(got, []string{"dep@2024-01-01", "dep@2025-01-01"}) {
		t.Fatalf("requested = %v", got)
	}
	oldDep, err := ctx.SchemaRevision("dep", "2024-01-01")
	if err != nil {
		t.Fatal(err)
	}
	if name, _ := oldDep.Revision(); name != "2024-01-01" {
		t.Fatalf("SchemaRevision returned %q", name)
	}
}

func TestS4ExcludedRootMayStillBeImported(t *testing.T) {
	root := t.TempDir()
	writeYANG(t, root, map[string]string{
		"dep@2025-01-01.yang": s4DepNew,
		"user.yang": `module user {
  yang-version 1.1; namespace "urn:s4:user"; prefix u;
  import dep { prefix dep; }
  leaf v { type dep:id; }
}`,
	})
	// dep is not requested as a root, yet user can import it.
	ctx, err := buildFromDisk(t, loadSpec{search: []string{root}, roots: []string{"user"}})
	if err != nil {
		t.Fatal(err)
	}
	report := ctx.LoadReport()
	if got := loadInfoNames(report.RequestedModules); !reflect.DeepEqual(got, []string{"user"}) {
		t.Fatalf("requested = %v", got)
	}
	if got := loadInfoNames(report.TransitiveImports); !reflect.DeepEqual(got, []string{"dep@2025-01-01"}) {
		t.Fatalf("transitive = %v", got)
	}
	if _, ok := ctx.GetModule("dep", nil); !ok {
		t.Fatal("dependency not available by GetModule")
	}
}

const s4Features = `module feat {
  yang-version 1.1; namespace "urn:s4:feat"; prefix f;
  import feat-lib { prefix fl; }
  feature a;
  feature b { if-feature a; }
  feature c;
  identity base-id;
  identity gated-id { base base-id; if-feature c; }
  container top {
    leaf always { type string; }
    leaf only-a { if-feature a; type string; }
    leaf only-b { if-feature b; type string; }
    leaf not-a { if-feature "not a"; type string; }
    leaf a-or-c { if-feature "a or c"; type string; }
    leaf a-and-c { if-feature "a and c"; type string; }
    leaf lib-gated { if-feature fl:remote; type string; }
    uses g { if-feature c; }
  }
  grouping g { leaf from-uses { type string; } }
  augment "/f:top" { if-feature a; leaf aug-a { type string; } }
}`

const s4FeatureLib = `module feat-lib {
  yang-version 1.1; namespace "urn:s4:feat-lib"; prefix fl;
  feature remote;
}`

func TestS4FeaturePolicyMatrix(t *testing.T) {
	cases := []struct {
		name     string
		features map[string][]string
		want     string
		ids      []string
	}{
		{"disabled", nil, "always,not-a", []string{}},
		{"a", map[string][]string{"feat": {"a"}}, "always,only-a,a-or-c,aug-a", []string{}},
		{"a+b", map[string][]string{"feat": {"a", "b"}}, "always,only-a,only-b,a-or-c,aug-a", []string{}},
		{"c", map[string][]string{"feat": {"c"}}, "always,not-a,a-or-c,from-uses", []string{"gated-id"}},
		{"a+c", map[string][]string{"feat": {"a", "c"}}, "always,only-a,a-or-c,a-and-c,from-uses,aug-a", []string{"gated-id"}},
		{"imported", map[string][]string{"feat-lib": {"remote"}}, "always,not-a,lib-gated", []string{}},
	}
	for _, tc := range cases {
		ctx, err := buildPolicyContextWithFeatures(t, tc.features, s4FeatureLib, s4Features)
		if err != nil {
			t.Fatalf("%s: Build: %v", tc.name, err)
		}
		mod, _ := ctx.Schema("feat")
		if got := names(schemaNodeAt(t, mod, "/f:top").Children()); got != tc.want {
			t.Errorf("%s: top children = %s, want %s", tc.name, got, tc.want)
		}
		base, _ := mod.Identity("base-id")
		ids := []string{}
		for _, id := range base.DerivedClosure() {
			ids = append(ids, id.Name())
		}
		if !reflect.DeepEqual(ids, tc.ids) {
			t.Errorf("%s: derived identities = %v, want %v", tc.name, ids, tc.ids)
		}
	}
	// Enabling b without a leaves b effectively disabled by its own
	// if-feature; the load report says so rather than hiding it.
	ctx, err := buildPolicyContextWithFeatures(t, map[string][]string{"feat": {"b"}}, s4FeatureLib, s4Features)
	if err != nil {
		t.Fatalf("b without a: %v", err)
	}
	mod, _ := ctx.Schema("feat")
	if got := names(schemaNodeAt(t, mod, "/f:top").Children()); got != "always,not-a" {
		t.Errorf("b without a: children = %s", got)
	}
	report := ctx.LoadReport()
	if !diagnosticContains(report.Warnings, "feat", `feature "b"`) {
		t.Errorf("b without a: warnings = %#v, want effectively-disabled warning", report.Warnings)
	}
	for _, sel := range report.EnabledFeatures {
		if sel.Feature == "b" {
			t.Error("b reported as enabled")
		}
	}
	// There is no wildcard: "*" is rejected, not treated as all features.
	builder, _ := cambium.NewContextBuilder(cambium.ContextFlags{DisableSearchdirCwd: true})
	if err := builder.SetFeatures("feat", []string{"*"}); err == nil {
		t.Error(`SetFeatures("*") succeeded`)
	}
}

func buildPolicyContextWithFeatures(t *testing.T, features map[string][]string, sources ...string) (*cambium.Context, error) {
	t.Helper()
	builder, err := cambium.NewContextBuilder(cambium.ContextFlags{DisableSearchdirCwd: true})
	if err != nil {
		t.Fatal(err)
	}
	for _, source := range sources {
		if err := builder.LoadModuleStr(source); err != nil {
			return nil, err
		}
	}
	for module, names := range features {
		if err := builder.SetFeatures(module, names); err != nil {
			return nil, err
		}
	}
	ctx, err := builder.Build()
	if err == nil {
		t.Cleanup(ctx.Close)
	}
	return ctx, err
}

const s4DevTarget = `module dt {
  yang-version 1.1; namespace "urn:s4:dt"; prefix dt;
  container top {
    leaf first { type string; }
    leaf typed { type uint8; }
    leaf defaulted { type string; default "d"; }
    leaf cfg { type string; }
    leaf-list many { type string; max-elements 3; }
    leaf checked { type string; must "true()"; }
    list rows { key k; unique "a"; leaf k { type string; } leaf a { type string; } leaf b { type string; } }
    leaf removed { type string; }
    leaf last { type string; }
  }
}`

const s4Dev = `module dv {
  yang-version 1.1; namespace "urn:s4:dv"; prefix dv;
  import dt { prefix dt; }
  deviation "/dt:top/dt:typed" { deviate replace { type string; } }
  deviation "/dt:top/dt:defaulted" { deviate delete { default "d"; } }
  deviation "/dt:top/dt:cfg" { deviate add { config false; } }
  deviation "/dt:top/dt:many" { deviate replace { max-elements 10; } deviate add { min-elements 1; } }
  deviation "/dt:top/dt:checked" { deviate delete { must "true()"; } deviate add { must "string-length(.) > 0"; } }
  deviation "/dt:top/dt:rows" { deviate delete { unique "a"; } deviate add { unique "b"; } }
  deviation "/dt:top/dt:removed" { description "not on this platform"; deviate not-supported; }
}`

func TestS4DeviationEffects(t *testing.T) {
	ctx, err := buildPolicyContext(t, cambium.DeviationPolicy{}, s4DevTarget, s4Dev)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	mod, _ := ctx.Schema("dt")
	// Unaffected sibling order survives the removal.
	if got, want := names(schemaNodeAt(t, mod, "/dt:top").Children()), "first,typed,defaulted,cfg,many,checked,rows,last"; got != want {
		t.Fatalf("children = %s, want %s", got, want)
	}
	if base := leafType(t, mod, "/dt:top/typed").Base(); base != cambium.BaseTypeString {
		t.Errorf("replaced type = %v", base)
	}
	if _, ok := schemaNodeAt(t, mod, "/dt:top/defaulted").DefaultValue(); ok {
		t.Error("deleted default still present")
	}
	if schemaNodeAt(t, mod, "/dt:top/cfg").Config() != cambium.ConfigRo {
		t.Error("added config false not effective")
	}
	many := schemaNodeAt(t, mod, "/dt:top/many")
	if mx, ok := many.MaxElements(); !ok || mx != 10 {
		t.Errorf("max-elements = %d,%v", mx, ok)
	}
	if mn, ok := many.MinElements(); !ok || mn != 1 {
		t.Errorf("min-elements = %d,%v", mn, ok)
	}
	if musts := schemaNodeAt(t, mod, "/dt:top/checked").Musts(); len(musts) != 1 || musts[0].Expression() != "string-length(.) > 0" {
		t.Errorf("musts = %+v", musts)
	}
	uniques := schemaNodeAt(t, mod, "/dt:top/rows").UniqueConstraints()
	if len(uniques) != 1 || uniques[0].Leafs()[0].Name() != "b" {
		t.Errorf("uniques = %+v", uniques)
	}

	// Reports keep every effect, including the removed node's.
	dv, _ := ctx.Schema("dv")
	var effects []string
	for _, d := range dv.Deviations() {
		effects = append(effects, d.Type()+":"+d.Property()+"="+d.NewValue()+"@"+d.TargetPath())
	}
	want := []string{
		"replace:type=string@/dt:top/dt:typed",
		"delete:default=d@/dt:top/dt:defaulted",
		"add:config=false@/dt:top/dt:cfg",
		"replace:max-elements=10@/dt:top/dt:many",
		"add:min-elements=1@/dt:top/dt:many",
		"delete:must=true()@/dt:top/dt:checked",
		"add:must=string-length(.) > 0@/dt:top/dt:checked",
		"delete:unique=a@/dt:top/dt:rows",
		"add:unique=b@/dt:top/dt:rows",
		"not-supported:=@/dt:top/dt:removed",
	}
	if !reflect.DeepEqual(effects, want) {
		t.Fatalf("deviation effects =\n%v\nwant\n%v", effects, want)
	}
	removed := dv.Deviations()[len(want)-1]
	if desc, ok := removed.Description(); !ok || desc != "not on this platform" || removed.SourceModule() != "dv" || removed.SourceLocation().Line == 0 {
		t.Fatalf("removed deviation metadata = %+v", removed)
	}
	if got := loadInfoNames(ctx.LoadReport().DeviationModules); !reflect.DeepEqual(got, []string{"dv"}) {
		t.Fatalf("deviation modules = %v", got)
	}
	// Surviving nodes carry their own provenance.
	if prov := schemaNodeAt(t, mod, "/dt:top/many").DeviationProvenance(); len(prov) != 2 {
		t.Fatalf("many provenance = %d entries", len(prov))
	}
}

func TestS4InvalidDeviations(t *testing.T) {
	cases := map[string]string{
		"missing target":    `deviation "/dt:top/dt:nope" { deviate not-supported; }`,
		"add existing":      `deviation "/dt:top/dt:defaulted" { deviate add { default "x"; } }`,
		"delete absent":     `deviation "/dt:top/dt:first" { deviate delete { default "x"; } }`,
		"bad property":      `deviation "/dt:top/dt:first" { deviate replace { presence "x"; } }`,
		"unknown deviate":   `deviation "/dt:top/dt:first" { deviate remove; }`,
		"replace no target": `deviation "/dt:top/dt:first" { deviate replace { units "x"; } }`,
	}
	for name, body := range cases {
		source := `module bad-dev { yang-version 1.1; namespace "urn:s4:bad"; prefix bd; import dt { prefix dt; } ` + body + ` }`
		if _, err := buildPolicyContext(t, cambium.DeviationPolicy{}, s4DevTarget, source); err == nil {
			t.Errorf("%s: Build succeeded", name)
		}
	}
}
