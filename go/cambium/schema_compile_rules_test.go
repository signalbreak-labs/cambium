// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

package cambium_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/signalbreak-labs/cambium/go/cambium"
)

var bothValidationModes = []cambium.ValidationMode{cambium.ValidationStrict, cambium.ValidationVendorCompatible}

// assertCompileRuleRejected checks that err is a context (CAMBIUM_E0001)
// schema error of the wanted diagnostic kind whose message contains want.
func assertCompileRuleRejected(t *testing.T, err error, kind cambium.DiagnosticKind, want string) {
	t.Helper()
	if err == nil {
		t.Fatalf("accepted, want error containing %q", want)
	}
	var ce *cambium.Error
	if !errors.As(err, &ce) || ce.RuleCode() != cambium.RuleCodeContext {
		t.Fatalf("error = %v, want RuleCodeContext", err)
	}
	if !strings.Contains(err.Error(), want) {
		t.Fatalf("error = %q, want it to contain %q", err.Error(), want)
	}
	if got := cambium.DiagnosticFromError(err).Kind; got != kind {
		t.Fatalf("diagnostic kind = %q, want %q (%v)", got, kind, err)
	}
}

// compileRuleModule wraps body in a YANG 1.1 module "cr".
func compileRuleModule(body string) string {
	return "module cr {\n  yang-version 1.1; namespace \"urn:cr\"; prefix cr;\n" + body + "\n}"
}

// compileRuleModule10 wraps body in a YANG 1.0 module "cr".
func compileRuleModule10(body string) string {
	return "module cr {\n  namespace \"urn:cr\"; prefix cr;\n" + body + "\n}"
}

func TestDuplicateIdentifiersThroughChoiceAndCaseRejected(t *testing.T) {
	// RFC 7950 §6.2.1, §7.9.2: choice and case are transparent to the data
	// node identifier namespace, which is scoped to the closest ancestor that
	// is neither a choice nor a case.
	cases := []struct {
		name, body, want string
	}{
		{"leaf and shorthand case", `leaf x { type string; } choice c { leaf x { type string; } }`, `duplicate schema child "x"`},
		{"across cases", `choice c { case a { leaf x { type string; } } case b { leaf x { type string; } } }`, `duplicate schema child "x"`},
		{"nested choice", `container p { leaf x { type string; } choice c { case a { choice d { leaf x { type string; } } } } }`, `duplicate schema child "x"`},
		{"choice name and nested leaf", `container p { choice x { leaf a { type string; } } choice c { leaf x { type string; } } }`, `duplicate schema child "x"`},
		{"uses into case", `grouping g { leaf x { type string; } } container p { leaf x { type string; } choice c { case a { uses g; } } }`, `duplicate schema child "x"`},
		{"augment into case", `container p { leaf x { type string; } choice c { case a { leaf y { type string; } } } } augment "/cr:p/cr:c/cr:a" { leaf x { type string; } }`, `duplicate schema child "x"`},
		{"grouping body", `grouping g { leaf x { type string; } choice c { leaf x { type string; } } }`, `duplicate schema child "x"`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for _, mode := range bothValidationModes {
				_, err := buildModeContext(t, mode, nil, compileRuleModule(tc.body))
				assertCompileRuleRejected(t, err, cambium.DiagnosticSemanticSchemaError, tc.want)
			}
		})
	}

	valid := []struct {
		name, body string
	}{
		{"distinct names", `leaf x { type string; } choice c { case a { leaf y { type string; } } case b { leaf z { type string; } } }`},
		{"case name matches parent leaf", `leaf a { type string; } choice c { case a { leaf y { type string; } } }`},
		{"same name in nested container", `leaf x { type string; } choice c { case a { container k { leaf x { type string; } } } }`},
	}
	for _, tc := range valid {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := buildModeContext(t, cambium.ValidationStrict, nil, compileRuleModule(tc.body)); err != nil {
				t.Fatalf("Build: %v", err)
			}
		})
	}

	// Identifiers are qualified by module: another module may augment the
	// same local name into a case.
	other := `module cro {
  yang-version 1.1; namespace "urn:cro"; prefix cro;
  import cr { prefix cr; }
  augment "/cr:p/cr:c/cr:a" { leaf x { type string; } }
}`
	ctx, err := buildModeContext(t, cambium.ValidationStrict, nil, compileRuleModule(`container p { leaf x { type string; } choice c { case a { leaf y { type string; } } } }`), other)
	if err != nil {
		t.Fatalf("cross-module Build: %v", err)
	}
	mod, _ := ctx.Schema("cr")
	assertChildNames(t, mod, "/cr:p/cr:c/cr:a", "y", "x")
}

// importOnlyModules are io-app, which only imports io-base and io-ext, and
// io-ext, which augments and deviates io-base.
var importOnlyModules = map[string]string{
	"io-app": `module io-app { yang-version 1.1; namespace "urn:io-app"; prefix app;
  import io-base { prefix b; } import io-ext { prefix e; }
  leaf use { type e:name; } }`,
	"io-base": `module io-base { yang-version 1.1; namespace "urn:io-base"; prefix b;
  container top { leaf x { type string; } leaf y { type string; } } }`,
	"io-ext": `module io-ext { yang-version 1.1; namespace "urn:io-ext"; prefix e;
  import io-base { prefix b; }
  typedef name { type string; }
  augment "/b:top" { leaf added { type string; } }
  deviation "/b:top/b:x" { deviate not-supported; } }`,
}

func TestImportOnlyModuleContributesNoAugmentsOrDeviations(t *testing.T) {
	// RFC 7950 §5.6.5: augments and deviations belong to the module that
	// declares them, and only an implemented module contributes them. A
	// module that is only imported provides its reusable definitions.
	dir := writeYANGModules(t, importOnlyModules)
	for _, mode := range bothValidationModes {
		ctx, err := buildDirContext(t, mode, nil, dir, "io-app")
		if err != nil {
			t.Fatalf("mode %d: Build: %v", mode, err)
		}
		base, _ := ctx.Schema("io-base")
		assertChildNames(t, base, "/b:top", "x", "y")
		if got := base.AugmentedBy(); len(got) != 0 {
			t.Fatalf("mode %d: AugmentedBy = %v, want none", mode, got)
		}
		ext, ok := ctx.GetModule("io-ext", nil)
		if !ok || ext.IsImplemented() {
			t.Fatalf("mode %d: io-ext loaded=%v implemented=%v, want import-only", mode, ok, ok && ext.IsImplemented())
		}
		report := ctx.LoadReport()
		if got := loadInfoNames(report.DeviationModules); len(got) != 0 {
			t.Fatalf("mode %d: DeviationModules = %v, want none", mode, got)
		}
		if len(report.Warnings) != 0 {
			t.Fatalf("mode %d: warnings = %#v, want none", mode, report.Warnings)
		}
	}

	// Loading the same module explicitly implements it, so its augment and
	// deviation apply.
	ctx, err := buildDirContext(t, cambium.ValidationStrict, nil, dir, "io-app", "io-ext")
	if err != nil {
		t.Fatalf("explicit Build: %v", err)
	}
	base, _ := ctx.Schema("io-base")
	assertChildNames(t, base, "/b:top", "y", "added")
	if got := loadInfoNames(ctx.LoadReport().DeviationModules); len(got) != 1 || got[0] != "io-ext" {
		t.Fatalf("DeviationModules = %v, want [io-ext]", got)
	}
	if !base.IsImplemented() {
		t.Fatal("augmented io-base is not implemented")
	}
}

func TestImportOnlyModuleAmendmentsAreNotResolved(t *testing.T) {
	// An import-only module's augments and deviations are not part of the
	// schema, so their targets are not resolved.
	dir := writeYANGModules(t, map[string]string{
		"io-app": `module io-app { yang-version 1.1; namespace "urn:io-app"; prefix app;
  import io-stale { prefix s; } leaf use { type s:name; } }`,
		"io-stale": `module io-stale { yang-version 1.1; namespace "urn:io-stale"; prefix s;
  typedef name { type string; }
  container top;
  augment "/s:gone" { leaf lost { type string; } }
  deviation "/s:top/s:gone" { deviate not-supported; } }`,
	})
	if _, err := buildDirContext(t, cambium.ValidationStrict, nil, dir, "io-app"); err != nil {
		t.Fatalf("Build: %v", err)
	}
	if _, err := buildDirContext(t, cambium.ValidationStrict, nil, dir, "io-app", "io-stale"); err == nil || !strings.Contains(err.Error(), `augment "/s:gone" target not found`) {
		t.Fatalf("implemented io-stale Build error = %v, want unresolved augment", err)
	}
}

func TestLeafrefPathImplementsModulesItNames(t *testing.T) {
	// RFC 7950 §5.6.5: a module whose nodes an implemented module's leafref
	// path uses is implemented, together with its augments.
	dir := writeYANGModules(t, map[string]string{
		"lp-app": `module lp-app { yang-version 1.1; namespace "urn:lp-app"; prefix app;
  import lp-base { prefix b; } import lp-ext { prefix e; }
  leaf ref { type leafref { path "/b:top/e:added"; } } }`,
		"lp-base": `module lp-base { yang-version 1.1; namespace "urn:lp-base"; prefix b; container top; }`,
		"lp-ext": `module lp-ext { yang-version 1.1; namespace "urn:lp-ext"; prefix e;
  import lp-base { prefix b; }
  augment "/b:top" { leaf added { type string; } } }`,
	})
	for _, mode := range bothValidationModes {
		ctx, err := buildDirContext(t, mode, nil, dir, "lp-app")
		if err != nil {
			t.Fatalf("mode %d: Build: %v", mode, err)
		}
		base, _ := ctx.Schema("lp-base")
		assertChildNames(t, base, "/b:top", "added")
		if ext, ok := ctx.GetModule("lp-ext", nil); !ok || !ext.IsImplemented() {
			t.Fatalf("mode %d: lp-ext is not implemented", mode)
		}
		if warnings := ctx.LoadReport().Warnings; len(warnings) != 0 {
			t.Fatalf("mode %d: warnings = %#v, want none", mode, warnings)
		}
	}
}

// loadDirModules loads names from dir in mode and builds the context. It
// returns the first LoadModule or Build error.
func loadDirModules(t *testing.T, mode cambium.ValidationMode, dir string, names ...string) (*cambium.Context, error) {
	t.Helper()
	builder, err := cambium.NewContextBuilder(cambium.ContextFlags{DisableSearchdirCwd: true})
	if err != nil {
		t.Fatalf("NewContextBuilder: %v", err)
	}
	if err := builder.SetValidationMode(mode); err != nil {
		t.Fatalf("SetValidationMode: %v", err)
	}
	if err := builder.SearchPath(dir); err != nil {
		t.Fatalf("SearchPath: %v", err)
	}
	for _, name := range names {
		if err := builder.LoadModule(name, nil, nil); err != nil {
			return nil, err
		}
	}
	ctx, err := builder.Build()
	if err == nil {
		t.Cleanup(ctx.Close)
	}
	return ctx, err
}

// assertVendorCompatibleWarning checks that the vendor-compatible context
// loaded and reported one warning containing fragment, for module unless
// module is empty.
func assertVendorCompatibleWarning(t *testing.T, ctx *cambium.Context, err error, module, fragment string) {
	t.Helper()
	if err != nil {
		t.Fatalf("vendor-compatible load: %v", err)
	}
	var matched []cambium.Diagnostic
	for _, warning := range ctx.LoadReport().Warnings {
		if strings.Contains(warning.Message, fragment) && (module == "" || warning.Module == module) {
			matched = append(matched, warning)
		}
	}
	if len(matched) != 1 {
		t.Fatalf("warnings = %#v, want one containing %q for module %q", ctx.LoadReport().Warnings, fragment, module)
	}
	if matched[0].Kind != cambium.DiagnosticSemanticSchemaError || matched[0].Code != cambium.RuleCodeContext {
		t.Fatalf("warning = %#v, want a semantic context diagnostic", matched[0])
	}
}

func TestImportAndIncludeCyclesRejected(t *testing.T) {
	// RFC 7950 §7.1.5: there MUST NOT be any circular chains of imports.
	// Circular include chains are rejected the same way.
	cases := []struct {
		name    string
		modules map[string]string
		module  string
		want    string
	}{
		{
			name: "two modules",
			modules: map[string]string{
				"cy-a": `module cy-a { namespace "urn:cy-a"; prefix a; import cy-b { prefix b; } }`,
				"cy-b": `module cy-b { namespace "urn:cy-b"; prefix b; import cy-a { prefix a; } }`,
			},
			module: "cy-a",
			want:   `import cycle cy-a -> cy-b -> cy-a`,
		},
		{
			name: "three modules",
			modules: map[string]string{
				"cy-a": `module cy-a { namespace "urn:cy-a"; prefix a; import cy-b { prefix b; } }`,
				"cy-b": `module cy-b { namespace "urn:cy-b"; prefix b; import cy-c { prefix c; } }`,
				"cy-c": `module cy-c { namespace "urn:cy-c"; prefix c; import cy-a { prefix a; } }`,
			},
			module: "cy-b",
			want:   `import cycle cy-b -> cy-c -> cy-a -> cy-b`,
		},
		{
			name: "through a submodule import",
			modules: map[string]string{
				"cy-a":   `module cy-a { namespace "urn:cy-a"; prefix a; include cy-a-s; }`,
				"cy-a-s": `submodule cy-a-s { belongs-to cy-a { prefix a; } import cy-b { prefix b; } }`,
				"cy-b":   `module cy-b { namespace "urn:cy-b"; prefix b; import cy-a { prefix a; } }`,
			},
			module: "cy-a",
			want:   `import cycle cy-a -> cy-b -> cy-a`,
		},
		{
			name: "two submodules",
			modules: map[string]string{
				"cy-a":  `module cy-a { namespace "urn:cy-a"; prefix a; include cy-s1; }`,
				"cy-s1": `submodule cy-s1 { belongs-to cy-a { prefix a; } include cy-s2; }`,
				"cy-s2": `submodule cy-s2 { belongs-to cy-a { prefix a; } include cy-s1; }`,
			},
			module: "cy-a",
			want:   `include cycle cy-s1 -> cy-s2 -> cy-s1`,
		},
		{
			name: "submodule includes itself",
			modules: map[string]string{
				"cy-a":  `module cy-a { namespace "urn:cy-a"; prefix a; include cy-s1; }`,
				"cy-s1": `submodule cy-s1 { belongs-to cy-a { prefix a; } include cy-s1; }`,
			},
			module: "cy-a",
			want:   `include cycle cy-s1 -> cy-s1`,
		},
		{
			name: "YANG 1.1 submodules",
			modules: map[string]string{
				"cy-a":  `module cy-a { yang-version 1.1; namespace "urn:cy-a"; prefix a; include cy-s1; include cy-s2; }`,
				"cy-s1": `submodule cy-s1 { yang-version 1.1; belongs-to cy-a { prefix a; } include cy-s2; }`,
				"cy-s2": `submodule cy-s2 { yang-version 1.1; belongs-to cy-a { prefix a; } include cy-s1; }`,
			},
			module: "cy-a",
			want:   `include cycle cy-s1 -> cy-s2 -> cy-s1`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := writeYANGModules(t, tc.modules)
			_, err := loadDirModules(t, cambium.ValidationStrict, dir, tc.module)
			assertCompileRuleRejected(t, err, cambium.DiagnosticSemanticSchemaError, tc.want)
			ctx, err := loadDirModules(t, cambium.ValidationVendorCompatible, dir, tc.module)
			assertVendorCompatibleWarning(t, ctx, err, "", tc.want)
		})
	}

	// Importing a module that imports a third module twice is no cycle.
	dir := writeYANGModules(t, map[string]string{
		"dg-a": `module dg-a { namespace "urn:dg-a"; prefix a; import dg-b { prefix b; } import dg-c { prefix c; } }`,
		"dg-b": `module dg-b { namespace "urn:dg-b"; prefix b; import dg-c { prefix c; } }`,
		"dg-c": `module dg-c { namespace "urn:dg-c"; prefix c; }`,
	})
	if _, err := loadDirModules(t, cambium.ValidationStrict, dir, "dg-a", "dg-b", "dg-c"); err != nil {
		t.Fatalf("diamond imports: %v", err)
	}
}

func TestYangVersionCoexistenceRulesEnforced(t *testing.T) {
	// RFC 7950 §12: a YANG 1.1 module MUST NOT include a YANG 1 submodule, a
	// YANG 1 module MUST NOT include a YANG 1.1 submodule, and a YANG 1 module
	// or submodule MUST NOT import a YANG 1.1 module by revision.
	cases := []struct {
		name    string
		modules map[string]string
		want    string
	}{
		{
			name: "1.1 module includes 1 submodule",
			modules: map[string]string{
				"yv-a": `module yv-a { yang-version 1.1; namespace "urn:yv-a"; prefix a; include yv-s; }`,
				"yv-s": `submodule yv-s { belongs-to yv-a { prefix a; } leaf x { type string; } }`,
			},
			want: `YANG version 1.1 module "yv-a" must not include YANG version 1 submodule "yv-s"`,
		},
		{
			name: "1 module includes 1.1 submodule",
			modules: map[string]string{
				"yv-a": `module yv-a { namespace "urn:yv-a"; prefix a; include yv-s; }`,
				"yv-s": `submodule yv-s { yang-version 1.1; belongs-to yv-a { prefix a; } leaf x { type string; } }`,
			},
			want: `YANG version 1 module "yv-a" must not include YANG version 1.1 submodule "yv-s"`,
		},
		{
			name: "1 module imports 1.1 module by revision",
			modules: map[string]string{
				"yv-a": `module yv-a { namespace "urn:yv-a"; prefix a; import yv-b { prefix b; revision-date 2020-01-01; } }`,
				"yv-b": `module yv-b { yang-version 1.1; namespace "urn:yv-b"; prefix b; revision 2020-01-01; }`,
			},
			want: `YANG version 1 module "yv-a" must not import YANG version 1.1 module "yv-b" by revision`,
		},
		{
			name: "1 submodule imports 1.1 module by revision",
			modules: map[string]string{
				"yv-a": `module yv-a { namespace "urn:yv-a"; prefix a; include yv-s; }`,
				"yv-s": `submodule yv-s { belongs-to yv-a { prefix a; } import yv-b { prefix b; revision-date 2020-01-01; } }`,
				"yv-b": `module yv-b { yang-version 1.1; namespace "urn:yv-b"; prefix b; revision 2020-01-01; }`,
			},
			want: `YANG version 1 submodule "yv-s" must not import YANG version 1.1 module "yv-b" by revision`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := writeYANGModules(t, tc.modules)
			_, err := loadDirModules(t, cambium.ValidationStrict, dir, "yv-a")
			assertCompileRuleRejected(t, err, cambium.DiagnosticSemanticSchemaError, tc.want)
			ctx, err := loadDirModules(t, cambium.ValidationVendorCompatible, dir, "yv-a")
			assertVendorCompatibleWarning(t, ctx, err, "yv-a", tc.want)
		})
	}

	valid := map[string]map[string]string{
		"1 module imports 1.1 module without revision": {
			"yv-a": `module yv-a { namespace "urn:yv-a"; prefix a; import yv-b { prefix b; } leaf x { type b:t; } }`,
			"yv-b": `module yv-b { yang-version 1.1; namespace "urn:yv-b"; prefix b; revision 2020-01-01; typedef t { type string; } }`,
		},
		"1.1 module imports 1 module by revision": {
			"yv-a": `module yv-a { yang-version 1.1; namespace "urn:yv-a"; prefix a; import yv-b { prefix b; revision-date 2020-01-01; } }`,
			"yv-b": `module yv-b { namespace "urn:yv-b"; prefix b; revision 2020-01-01; }`,
		},
		"matching submodule versions": {
			"yv-a": `module yv-a { yang-version 1.1; namespace "urn:yv-a"; prefix a; include yv-s; }`,
			"yv-s": `submodule yv-s { yang-version 1.1; belongs-to yv-a { prefix a; } leaf x { type string; } }`,
		},
	}
	for name, modules := range valid {
		dir := writeYANGModules(t, modules)
		if _, err := loadDirModules(t, cambium.ValidationStrict, dir, "yv-a"); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
	}
}

func TestInvalidLeafrefPathPredicatesRejected(t *testing.T) {
	// RFC 7950 §9.9.2, §14 (path-predicate): a leafref predicate constrains a
	// key of the list it follows to the value of a leaf reached by a
	// current()/.. path.
	const tree = `list l { key "k j"; leaf k { type string; } leaf j { type string; } leaf v { type string; } }
  list nk { config false; leaf v { type string; } }
  container c { leaf x { type string; } }
  leaf r { type string; }
  leaf s { type string; }
`
	cases := []struct {
		name, path, want string
	}{
		{"unknown key", `/cr:l[cr:nope = current()/../cr:r]/cr:v`, `"cr:nope" is not a key of list "l"`},
		{"non-key leaf", `/cr:l[cr:v = current()/../cr:r]/cr:v`, `"cr:v" is not a key of list "l"`},
		{"duplicate key", `/cr:l[cr:k = current()/../cr:r][cr:k = current()/../cr:s]/cr:v`, `duplicate key "cr:k"`},
		{"right side resolves to nothing", `/cr:l[cr:k = current()/../cr:gone]/cr:v`, `"current()/../cr:gone" does not resolve to a leaf`},
		{"right side is a container", `/cr:l[cr:k = current()/../cr:c]/cr:v`, `"current()/../cr:c" does not resolve to a leaf`},
		{"right side above the root", `/cr:l[cr:k = current()/../../../cr:r]/cr:v`, `"current()/../../../cr:r" does not resolve to a leaf`},
		{"literal right side", `/cr:l[cr:k = 'a']/cr:v`, `right-hand side "'a'" must be a current()/.. path`},
		{"right side without parent step", `/cr:l[cr:k = current()/cr:r]/cr:v`, `right-hand side "current()/cr:r" must be a current()/.. path`},
		{"predicate on container", `/cr:c[cr:x = current()/../cr:r]/cr:x`, `predicate on container "c", which is not a list`},
		{"predicate on keyless list", `/cr:nk[cr:v = current()/../cr:r]/cr:v`, `predicate on list "nk", which has no keys`},
		{"relative path", `../cr:l[cr:nope = current()/../cr:r]/cr:v`, `"cr:nope" is not a key of list "l"`},
		{"malformed predicate", `/cr:l[cr:k]/cr:v`, `invalid predicate "[cr:k]"`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			source := compileRuleModule(tree + `leaf ref { config false; type leafref { path "` + tc.path + `"; } }`)
			for _, mode := range bothValidationModes {
				_, err := buildModeContext(t, mode, nil, source)
				assertCompileRuleRejected(t, err, cambium.DiagnosticSemanticSchemaError, tc.want)
			}
		})
	}

	valid := []string{
		`leaf ref { type leafref { path "/cr:l[cr:k = current()/../cr:r][cr:j = current()/../cr:s]/cr:v"; } }`,
		`leaf ref { type leafref { path "/cr:l[k = current()/../r]/v"; } }`,
		`leaf ref { type leafref { path "/cr:l[ cr:k = current() / .. / cr:r ]/cr:v"; } }`,
		`leaf ref { type leafref { path "../cr:l[cr:k = current()/../cr:r]/cr:v"; } }`,
		`container p { leaf ref { type leafref { path "/cr:l[cr:k = current()/../../cr:r]/cr:v"; } } }`,
		`container p { leaf name { type string; } leaf ref { type leafref { path "/cr:l[cr:k = current()/../name]/cr:v"; } } }`,
		`typedef lr { type leafref { path "/cr:l[cr:k = current()/../cr:r]/cr:v"; } } leaf ref { type lr; }`,
	}
	for _, body := range valid {
		if _, err := buildModeContext(t, cambium.ValidationStrict, nil, compileRuleModule(tree+body)); err != nil {
			t.Fatalf("%s: Build: %v", body, err)
		}
	}
}

func TestDeviateAddExistingConfigOrMandatoryRejected(t *testing.T) {
	// RFC 7950 §7.20.3.2: a property that can appear only once MUST NOT
	// exist in the target of "deviate add". An explicit config or mandatory
	// statement exists; a config value inherited from an ancestor does not.
	cases := []struct {
		name, body, want string
	}{
		{"explicit config", `container c { config true; } deviation "/cr:c" { deviate add { config false; } }`, `deviate add config for "c" already exists`},
		{"explicit config false", `container c { leaf x { type string; config false; } } deviation "/cr:c/cr:x" { deviate add { config false; } }`, `deviate add config for "x" already exists`},
		{"refined config", `grouping g { leaf x { type string; } } container c { uses g { refine x { config false; } } } deviation "/cr:c/cr:x" { deviate add { config false; } }`, `deviate add config for "x" already exists`},
		{"explicit mandatory", `leaf x { type string; mandatory false; } deviation "/cr:x" { deviate add { mandatory true; } }`, `deviate add mandatory for "x" already exists`},
		{"mandatory added twice", `leaf x { type string; } deviation "/cr:x" { deviate add { mandatory true; } } deviation "/cr:x" { deviate add { mandatory false; } }`, `deviate add mandatory for "x" already exists`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for _, mode := range bothValidationModes {
				_, err := buildModeContext(t, mode, nil, compileRuleModule(tc.body))
				assertCompileRuleRejected(t, err, cambium.DiagnosticInvalidDeviation, tc.want)
			}
		})
	}

	ctx, err := buildModeContext(t, cambium.ValidationStrict, nil, compileRuleModule(`
  container s { config false; leaf x { type string; } }
  container c { leaf y { type string; } leaf z { type string; } }
  deviation "/cr:s/cr:x" { deviate add { config false; } }
  deviation "/cr:c/cr:y" { deviate add { config false; } }
  deviation "/cr:c/cr:z" { deviate add { mandatory true; } }`))
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	mod, _ := ctx.Schema("cr")
	if got := schemaNodeAt(t, mod, "/cr:c/cr:y").Config(); got != cambium.ConfigRo {
		t.Fatalf("y config = %v, want read-only", got)
	}
	if !schemaNodeAt(t, mod, "/cr:c/cr:z").IsMandatory() {
		t.Fatal("z is not mandatory")
	}
}

func TestChoiceDefaultCaseMandatoryNodeRejected(t *testing.T) {
	// RFC 7950 §7.9.3: there MUST NOT be any mandatory nodes (§3) directly
	// under the default case.
	cases := []struct {
		name, body, want string
	}{
		{"mandatory leaf", `choice c { default a; case a { leaf a { type string; mandatory true; } } leaf b { type string; } }`, `choice "c" default case "a" must not contain mandatory node "a"`},
		{"shorthand case", `choice c { default a; leaf a { type string; mandatory true; } leaf b { type string; } }`, `choice "c" default case "a" must not contain mandatory node "a"`},
		{"non-presence container", `choice c { default a; case a { container k { leaf x { type string; mandatory true; } } } leaf b { type string; } }`, `must not contain mandatory node "k"`},
		{"leaf-list with min-elements", `choice c { default a; case a { leaf-list x { type string; min-elements 1; } } leaf b { type string; } }`, `must not contain mandatory node "x"`},
		{"mandatory choice", `choice c { default a; case a { choice d { mandatory true; leaf x { type string; } leaf y { type string; } } } leaf b { type string; } }`, `must not contain mandatory node "d"`},
		{"config false", `container s { config false; choice c { default a; case a { leaf x { type string; mandatory true; } } leaf b { type string; } } }`, `must not contain mandatory node "x"`},
		{"augmented", `choice c { default a; case a { leaf y { type string; } } leaf b { type string; } } augment "/cr:c/cr:a" { leaf m { type string; mandatory true; } }`, `must not contain mandatory node "m"`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for _, mode := range bothValidationModes {
				_, err := buildModeContext(t, mode, nil, compileRuleModule(tc.body))
				assertCompileRuleRejected(t, err, cambium.DiagnosticSemanticSchemaError, tc.want)
			}
		})
	}

	for _, body := range []string{
		`choice c { default a; case a { leaf a { type string; } } case b { leaf b { type string; mandatory true; } } }`,
		`choice c { default a; case a { container k { presence p; leaf x { type string; mandatory true; } } } leaf b { type string; } }`,
		`choice c { default a; case a { choice d { leaf x { type string; mandatory true; } leaf y { type string; } } } leaf b { type string; } }`,
		`choice c { default a; case a { list l { key k; leaf k { type string; } } } leaf b { type string; } }`,
	} {
		if _, err := buildModeContext(t, cambium.ValidationStrict, nil, compileRuleModule(body)); err != nil {
			t.Fatalf("%s: Build: %v", body, err)
		}
	}
}

func TestStatusReferencesWithinModuleEnforced(t *testing.T) {
	// RFC 7950 §7.21.2: within one module, a current definition MUST NOT
	// reference a deprecated or obsolete definition, and a deprecated
	// definition MUST NOT reference an obsolete one. A definition without a
	// status statement takes the status of its closest ancestor that has one.
	cases := []struct {
		name, body, want string
	}{
		{"leaf uses deprecated typedef", `typedef t { type string; status deprecated; } leaf x { type t; }`, `current leaf "x" must not reference deprecated typedef "t"`},
		{"union member", `typedef t { type string; status deprecated; } leaf x { type union { type t; type int8; } }`, `current leaf "x" must not reference deprecated typedef "t"`},
		{"typedef chain", `typedef t { type string; status deprecated; } typedef u { type t; } leaf x { type u; }`, `current typedef "u" must not reference deprecated typedef "t"`},
		{"deprecated leaf uses obsolete typedef", `typedef t { type string; status obsolete; } leaf x { type t; status deprecated; }`, `deprecated leaf "x" must not reference obsolete typedef "t"`},
		{"uses obsolete grouping", `grouping g { status obsolete; leaf y { type string; } } container c { uses g; }`, `current uses "g" must not reference obsolete grouping "g"`},
		{"identity base", `identity b { status deprecated; } identity i { base b; }`, `current identity "i" must not reference deprecated identity "b"`},
		{"identityref base", `identity b { status obsolete; } leaf x { type identityref { base b; } }`, `current leaf "x" must not reference obsolete identity "b"`},
		{"if-feature", `feature f { status deprecated; } leaf x { if-feature f; type string; }`, `current leaf "x" must not reference deprecated feature "f"`},
		{"feature if-feature expression", `feature f { status obsolete; } feature g; feature h { if-feature "g and not f"; }`, `current feature "h" must not reference obsolete feature "f"`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := buildModeContext(t, cambium.ValidationStrict, nil, compileRuleModule(tc.body))
			assertCompileRuleRejected(t, err, cambium.DiagnosticSemanticSchemaError, tc.want)
			ctx, err := buildModeContext(t, cambium.ValidationVendorCompatible, nil, compileRuleModule(tc.body))
			assertVendorCompatibleWarning(t, ctx, err, "cr", tc.want)
		})
	}

	for _, body := range []string{
		`typedef t { type string; status deprecated; } leaf x { type t; status deprecated; }`,
		`typedef t { type string; status deprecated; } leaf x { type t; status obsolete; }`,
		`typedef t { type string; status obsolete; } leaf x { type t; status obsolete; }`,
		`typedef t { type string; status deprecated; } container c { status deprecated; leaf x { type t; } }`,
		`grouping g { status obsolete; leaf y { type string; } } container d { status obsolete; uses g; }`,
		`grouping g { status deprecated; leaf y { type string; } } container d { uses g { status deprecated; } }`,
		`feature f { status deprecated; } leaf x { if-feature f; type string; status deprecated; }`,
	} {
		ctx, err := buildModeContext(t, cambium.ValidationStrict, nil, compileRuleModule(body))
		if err != nil {
			t.Fatalf("%s: Build: %v", body, err)
		}
		if warnings := ctx.LoadReport().Warnings; len(warnings) != 0 {
			t.Fatalf("%s: warnings = %#v, want none", body, warnings)
		}
	}

	// The rule is per module: another module's deprecated definitions may be
	// referenced.
	other := `module cro { yang-version 1.1; namespace "urn:cro"; prefix cro;
  typedef t { type string; status deprecated; }
  grouping g { status obsolete; leaf y { type string; } } }`
	if _, err := buildModeContext(t, cambium.ValidationStrict, nil, other, compileRuleModule(`import cro { prefix o; } leaf x { type o:t; } container c { uses o:g; }`)); err != nil {
		t.Fatalf("cross-module Build: %v", err)
	}
}

func TestStatusIsNotInheritedBySchemaNodes(t *testing.T) {
	// RFC 7950 §7.21.2 defines no status inheritance: a node without a status
	// statement reports the default, current.
	ctx, err := buildModeContext(t, cambium.ValidationStrict, nil, compileRuleModule(`container c { status deprecated; leaf x { type string; } }`))
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	mod, _ := ctx.Schema("cr")
	if got := schemaNodeAt(t, mod, "/cr:c/cr:x").Status(); got != cambium.StatusCurrent {
		t.Fatalf("x status = %v, want current", got)
	}
}

func TestYang10DerivedEnumerationAndBitsRestrictionRejected(t *testing.T) {
	// RFC 7950 §1.1, §9.6.4, §9.7.4: restricting a derived enumeration or bits
	// type is new in YANG 1.1.
	cases := []struct {
		name, body, want string
	}{
		{"enumeration", `typedef t { type enumeration { enum a; enum b; } } leaf x { type t { enum a; } }`, `enum restriction of a derived type requires yang-version 1.1`},
		{"bits", `typedef t { type bits { bit a; bit b; } } leaf x { type t { bit a; } }`, `bit restriction of a derived type requires yang-version 1.1`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for _, mode := range bothValidationModes {
				_, err := buildModeContext(t, mode, nil, compileRuleModule10(tc.body))
				assertCompileRuleRejected(t, err, cambium.DiagnosticSemanticSchemaError, tc.want)
			}
			if _, err := buildModeContext(t, cambium.ValidationStrict, nil, compileRuleModule(tc.body)); err != nil {
				t.Fatalf("YANG 1.1 Build: %v", err)
			}
		})
	}
}

func TestEnumNameWhitespaceRejected(t *testing.T) {
	// RFC 7950 §9.6.4: an enum name MUST NOT be zero-length and MUST NOT have
	// leading or trailing whitespace.
	cases := map[string]string{
		"leading space":   `leaf x { type enumeration { enum " a"; } }`,
		"trailing space":  `leaf x { type enumeration { enum "a "; } }`,
		"trailing tab":    `leaf x { type enumeration { enum "a\t"; } }`,
		"zero length":     `leaf x { type enumeration { enum ""; } }`,
		"restriction":     `typedef t { type enumeration { enum a; } } leaf x { type t { enum " a"; } }`,
		"no-break space":  "leaf x { type enumeration { enum \"a \"; } }",
		"leading newline": `leaf x { type enumeration { enum "\na"; } }`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			for _, mode := range bothValidationModes {
				_, err := buildModeContext(t, mode, nil, compileRuleModule(body))
				assertCompileRuleRejected(t, err, cambium.DiagnosticSemanticSchemaError, "invalid enum name")
			}
		})
	}
	if _, err := buildModeContext(t, cambium.ValidationStrict, nil, compileRuleModule(`leaf x { type enumeration { enum "Admin Down"; enum "a\tb"; } }`)); err != nil {
		t.Fatalf("inner whitespace Build: %v", err)
	}
}

func TestIdentityrefDefaultMustBeDerivedFromBase(t *testing.T) {
	// RFC 7950 §9.10.2: an identityref value is an identity derived from the
	// base identity; the base identity itself is not a valid value.
	for name, body := range map[string]string{
		"base":          `identity b; identity c { base b; } leaf x { type identityref { base b; } default b; }`,
		"prefixed base": `identity b; identity c { base b; } leaf x { type identityref { base b; } default cr:b; }`,
		"typedef":       `identity b; identity c { base b; } typedef t { type identityref { base b; } default b; } leaf x { type t; }`,
		"leaf-list":     `identity b; identity c { base b; } leaf-list x { type identityref { base b; } default c; default b; }`,
	} {
		t.Run(name, func(t *testing.T) {
			for _, mode := range bothValidationModes {
				_, err := buildModeContext(t, mode, nil, compileRuleModule(body))
				if err == nil || !strings.Contains(err.Error(), `default "`) || !strings.Contains(err.Error(), `is not valid for identityref`) {
					t.Fatalf("mode %d: Build error = %v, want invalid identityref default", mode, err)
				}
			}
		})
	}
	for _, body := range []string{
		`identity b; identity c { base b; } leaf x { type identityref { base b; } default c; }`,
		`identity b; identity c { base b; } identity d { base c; } leaf x { type identityref { base b; } default d; }`,
	} {
		if _, err := buildModeContext(t, cambium.ValidationStrict, nil, compileRuleModule(body)); err != nil {
			t.Fatalf("%s: Build: %v", body, err)
		}
	}
}
