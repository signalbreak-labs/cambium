// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

package cambium_test

import (
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/signalbreak-labs/cambium/go/cambium"
)

// buildDirContext loads the named modules from dir in the given LoadModule
// order, so a test controls the context's module-load order.
func buildDirContext(t *testing.T, mode cambium.ValidationMode, features map[string][]string, dir string, names ...string) (*cambium.Context, error) {
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
			t.Fatalf("LoadModule %s: %v", name, err)
		}
	}
	for module, enabled := range features {
		if err := builder.SetFeatures(module, enabled); err != nil {
			t.Fatalf("SetFeatures %s: %v", module, err)
		}
	}
	ctx, err := builder.Build()
	if err == nil {
		t.Cleanup(ctx.Close)
	}
	return ctx, err
}

func writeYANGModules(t *testing.T, modules map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, source := range modules {
		if err := os.WriteFile(filepath.Join(dir, name+".yang"), []byte(source), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// permutations returns every ordering of names, in a fixed order.
func permutations(names []string) [][]string {
	if len(names) <= 1 {
		return [][]string{append([]string(nil), names...)}
	}
	var out [][]string
	for i := range names {
		rest := make([]string, 0, len(names)-1)
		rest = append(rest, names[:i]...)
		rest = append(rest, names[i+1:]...)
		for _, tail := range permutations(rest) {
			out = append(out, append([]string{names[i]}, tail...))
		}
	}
	return out
}

func assertChildNames(t *testing.T, mod cambium.Module, path string, want ...string) {
	t.Helper()
	if got := childNamesForProfile(schemaNodeAt(t, mod, path).Children()); !reflect.DeepEqual(got, want) {
		t.Fatalf("%s children = %v, want %v", path, got, want)
	}
}

func TestAugmentOfAugmentWithinModuleIsSourceOrderIndependent(t *testing.T) {
	const header = `module aoa {
  yang-version 1.1; namespace "urn:aoa"; prefix aoa;
  container c { leaf base { type string; } }
`
	const base = `  augment "/aoa:c" { container d { leaf own { type string; } } leaf after { type string; } }
`
	const nested = `  augment "/aoa:c/aoa:d" { leaf x { type string; } }
`
	for _, tc := range []struct{ name, source string }{
		{"base first", header + base + nested + "}"},
		{"nested first", header + nested + base + "}"},
	} {
		for _, mode := range []cambium.ValidationMode{cambium.ValidationStrict, cambium.ValidationVendorCompatible} {
			ctx, err := buildModeContext(t, mode, nil, tc.source)
			if err != nil {
				t.Fatalf("%s mode %d: Build: %v", tc.name, mode, err)
			}
			mod, _ := ctx.Schema("aoa")
			assertChildNames(t, mod, "/aoa:c", "base", "d", "after")
			assertChildNames(t, mod, "/aoa:c/aoa:d", "own", "x")
			if warnings := ctx.LoadReport().Warnings; len(warnings) != 0 {
				t.Fatalf("%s mode %d: warnings = %#v, want none", tc.name, mode, warnings)
			}
		}
	}

	// An augment whose target no augment creates is still reported, after
	// the chain declared before it has resolved.
	dangling := header + nested + `  augment "/aoa:c/aoa:zz" { leaf lost { type string; } }
` + base + "}"
	for _, mode := range []cambium.ValidationMode{cambium.ValidationStrict, cambium.ValidationVendorCompatible} {
		_, err := buildModeContext(t, mode, nil, dangling)
		if err == nil || !strings.Contains(err.Error(), `augment "/aoa:c/aoa:zz" target not found`) {
			t.Fatalf("mode %d: Build error = %v, want unresolved /aoa:c/aoa:zz", mode, err)
		}
		if strings.Contains(err.Error(), "aoa:d") || strings.Contains(err.Error(), "feature") {
			t.Fatalf("mode %d: Build error = %v, want only the dangling augment", mode, err)
		}
	}
}

func TestAugmentOfAugmentAcrossModulesIsLoadOrderIndependent(t *testing.T) {
	cases := []struct {
		name    string
		modules map[string]string
		module  string
		want    map[string][]string
	}{
		{
			name: "target module augments itself",
			modules: map[string]string{
				"xa": `module xa { yang-version 1.1; namespace "urn:xa"; prefix xa; import xb { prefix xb; }
  augment "/xb:c/xb:d" { leaf x { type string; } } }`,
				"xb": `module xb { yang-version 1.1; namespace "urn:xb"; prefix xb;
  container c; augment "/xb:c" { container d { leaf own { type string; } } } }`,
			},
			module: "xb",
			want: map[string][]string{
				"/xb:c":      {"d"},
				"/xb:c/xb:d": {"own", "x"},
			},
		},
		{
			name: "third module creates the target",
			modules: map[string]string{
				"ya": `module ya { namespace "urn:ya"; prefix ya; import yb { prefix yb; } import yc { prefix yc; }
  augment "/yb:top/yc:d" { leaf x { type string; } } }`,
				"yb": `module yb { namespace "urn:yb"; prefix yb; container top { leaf base { type string; } } }`,
				"yc": `module yc { namespace "urn:yc"; prefix yc; import yb { prefix yb; }
  augment "/yb:top" { container d { leaf own { type string; } } } }`,
			},
			module: "yb",
			want: map[string][]string{
				"/yb:top":      {"base", "d"},
				"/yb:top/yc:d": {"own", "x"},
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := writeYANGModules(t, tc.modules)
			for _, order := range permutations(slices.Sorted(maps.Keys(tc.modules))) {
				ctx, err := buildDirContext(t, cambium.ValidationStrict, nil, dir, order...)
				if err != nil {
					t.Fatalf("load order %v: Build: %v", order, err)
				}
				mod, _ := ctx.Schema(tc.module)
				for _, path := range slices.Sorted(maps.Keys(tc.want)) {
					assertChildNames(t, mod, path, tc.want[path]...)
				}
			}
		})
	}
}

func TestAugmentsOfAugmentCreatedTargetFollowLoadOrder(t *testing.T) {
	// ox and oz both augment a container that om's augment creates. Their
	// contributions follow module-load order (spec §1.1 rule 3) even when one
	// of them is loaded before the module that creates the target.
	dir := writeYANGModules(t, map[string]string{
		"ob": `module ob { yang-version 1.1; namespace "urn:ob"; prefix ob; container top; }`,
		"om": `module om { yang-version 1.1; namespace "urn:om"; prefix om; import ob { prefix ob; }
  augment "/ob:top" { container t { leaf own { type string; } } } }`,
		"ox": `module ox { yang-version 1.1; namespace "urn:ox"; prefix ox; import ob { prefix ob; } import om { prefix om; }
  augment "/ob:top/om:t" { leaf from-x { type string; } } }`,
		"oz": `module oz { yang-version 1.1; namespace "urn:oz"; prefix oz; import ob { prefix ob; } import om { prefix om; }
  augment "/ob:top/om:t" { leaf from-z { type string; } } }`,
	})
	cases := []struct {
		order       []string
		children    []string
		augmentedBy []string
	}{
		{[]string{"ob", "om", "ox", "oz"}, []string{"own", "from-x", "from-z"}, []string{"om", "ox", "oz"}},
		{[]string{"ox", "om", "oz"}, []string{"own", "from-x", "from-z"}, []string{"ox", "om", "oz"}},
		{[]string{"oz", "om", "ox"}, []string{"own", "from-z", "from-x"}, []string{"oz", "om", "ox"}},
		{[]string{"ox", "oz"}, []string{"own", "from-x", "from-z"}, []string{"ox", "om", "oz"}},
	}
	for _, tc := range cases {
		for _, mode := range []cambium.ValidationMode{cambium.ValidationStrict, cambium.ValidationVendorCompatible} {
			ctx, err := buildDirContext(t, mode, nil, dir, tc.order...)
			if err != nil {
				t.Fatalf("load order %v mode %d: Build: %v", tc.order, mode, err)
			}
			mod, _ := ctx.Schema("ob")
			assertChildNames(t, mod, "/ob:top/om:t", tc.children...)
			if got := mod.AugmentedBy(); !reflect.DeepEqual(got, tc.augmentedBy) {
				t.Fatalf("load order %v mode %d: AugmentedBy = %v, want %v", tc.order, mode, got, tc.augmentedBy)
			}
		}
	}
}

func TestAugmentOfAugmentPrefersExactTargetOverVendorFallback(t *testing.T) {
	// When pw's augment is first tried, pm's d already exists but pz's d does
	// not. The exact path names pz's d, so the vendor local-name fallback must
	// not claim pm's d before pz's augment has run.
	dir := writeYANGModules(t, map[string]string{
		"pb": `module pb { yang-version 1.1; namespace "urn:pb"; prefix pb; container top; }`,
		"pm": `module pm { yang-version 1.1; namespace "urn:pm"; prefix pm; import pb { prefix pb; }
  augment "/pb:top" { container d; } }`,
		"pw": `module pw { yang-version 1.1; namespace "urn:pw"; prefix pw; import pb { prefix pb; } import pz { prefix pz; }
  augment "/pb:top/pz:d" { leaf x { type string; } } }`,
		"pz": `module pz { yang-version 1.1; namespace "urn:pz"; prefix pz; import pb { prefix pb; }
  augment "/pb:top" { container d; } }`,
	})
	ctx, err := buildDirContext(t, cambium.ValidationVendorCompatible, nil, dir, "pm", "pw")
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	mod, _ := ctx.Schema("pb")
	assertChildNames(t, mod, "/pb:top/pm:d")
	assertChildNames(t, mod, "/pb:top/pz:d", "x")
	if warnings := ctx.LoadReport().Warnings; len(warnings) != 0 {
		t.Fatalf("warnings = %#v, want none", warnings)
	}
}

func TestAugmentOfAugmentFeatureExclusionIsOrderIndependent(t *testing.T) {
	// fv's disabled augment targets a container that fv's later augment
	// creates; fd augments into the disabled augment's content. fd's target is
	// excluded by feature policy, not missing, whatever the declaration order.
	const base = `module fa {
  yang-version 1.1; namespace "urn:fa"; prefix fa;
  container top { leaf keep { type string; } }
}`
	const vendor = `module fv {
  yang-version 1.1; namespace "urn:fv"; prefix fv;
  import fa { prefix fa; }
  feature ext;
  augment "/fa:top/fv:vc" { if-feature ext; container gated { leaf q { type string; } } }
  augment "/fa:top" { container vc; }
}`
	const user = `module fd {
  yang-version 1.1; namespace "urn:fd"; prefix fd;
  import fa { prefix fa; }
  import fv { prefix fv; }
  augment "/fa:top/fv:vc/fv:gated" { leaf z { type string; } }
}`
	_, err := buildModeContext(t, cambium.ValidationStrict, nil, base, vendor, user)
	if err == nil || !strings.Contains(err.Error(), `augment "/fa:top/fv:vc/fv:gated" target not found`) || !strings.Contains(err.Error(), "excluded by feature policy") {
		t.Fatalf("strict Build error = %v, want feature exclusion detail", err)
	}

	ctx, err := buildModeContext(t, cambium.ValidationVendorCompatible, nil, base, vendor, user)
	if err != nil {
		t.Fatalf("vendor Build: %v", err)
	}
	omitted := ctx.LoadReport().OmittedContent()
	if len(omitted) != 1 || omitted[0].Module != "fd" || !strings.Contains(omitted[0].Message, "excluded by feature policy") {
		t.Fatalf("omitted content = %#v, want the fd augment", omitted)
	}
	mod, _ := ctx.Schema("fa")
	assertChildNames(t, mod, "/fa:top/fv:vc")

	ctx, err = buildModeContext(t, cambium.ValidationStrict, map[string][]string{"fv": {"ext"}}, base, vendor, user)
	if err != nil {
		t.Fatalf("strict Build with ext: %v", err)
	}
	mod, _ = ctx.Schema("fa")
	assertChildNames(t, mod, "/fa:top", "keep", "vc")
	assertChildNames(t, mod, "/fa:top/fv:vc", "gated")
	assertChildNames(t, mod, "/fa:top/fv:vc/fv:gated", "q", "z")
}

const refineIfFeatureModule = `module rf {
  yang-version 1.1; namespace "urn:rf"; prefix rf;
  feature f;
  grouping g {
    container d {
      leaf x { type string; }
      leaf keep { type string; }
    }
    container e { leaf inner { type string; } }
    leaf tail { type string; }
  }
  container c {
    uses g {
      refine "d/x" { if-feature f; default abc; description refined; }
      refine e { if-feature f; }
      refine tail { if-feature "not f"; default t; }
    }
  }
}`

func TestRefineIfFeatureGatesTarget(t *testing.T) {
	// RFC 7950 §7.13.2: a refine's if-feature statements are added to the
	// target node; they do not gate the refine's other properties.
	for _, mode := range []cambium.ValidationMode{cambium.ValidationStrict, cambium.ValidationVendorCompatible} {
		ctx, err := buildModeContext(t, mode, nil, refineIfFeatureModule)
		if err != nil {
			t.Fatalf("mode %d: Build: %v", mode, err)
		}
		mod, _ := ctx.Schema("rf")
		assertChildNames(t, mod, "/rf:c", "d", "tail")
		assertChildNames(t, mod, "/rf:c/rf:d", "keep")
		if _, err := mod.FindPath("/rf:c/rf:d/rf:x"); err == nil {
			t.Fatalf("mode %d: refined-out x is present with feature f disabled", mode)
		}
		tail := schemaNodeAt(t, mod, "/rf:c/rf:tail")
		if got, ok := tail.DefaultValue(); !ok || got != "t" {
			t.Fatalf("mode %d: tail default = %q,%v, want t", mode, got, ok)
		}
		if got, want := tail.IfFeatures(), []string{"not f"}; !reflect.DeepEqual(got, want) {
			t.Fatalf("mode %d: tail IfFeatures = %v, want %v", mode, got, want)
		}
	}

	ctx, err := buildModeContext(t, cambium.ValidationStrict, map[string][]string{"rf": {"f"}}, refineIfFeatureModule)
	if err != nil {
		t.Fatalf("Build with f: %v", err)
	}
	mod, _ := ctx.Schema("rf")
	assertChildNames(t, mod, "/rf:c", "d", "e")
	assertChildNames(t, mod, "/rf:c/rf:d", "x", "keep")
	x := schemaNodeAt(t, mod, "/rf:c/rf:d/rf:x")
	if got, ok := x.DefaultValue(); !ok || got != "abc" {
		t.Fatalf("x default = %q,%v, want abc", got, ok)
	}
	if got, ok := x.Description(); !ok || got != "refined" {
		t.Fatalf("x description = %q,%v, want refined", got, ok)
	}
	if got, want := x.IfFeatures(), []string{"f"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("x IfFeatures = %v, want %v", got, want)
	}
	if got, want := schemaNodeAt(t, mod, "/rf:c/rf:e").IfFeatures(), []string{"f"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("e IfFeatures = %v, want %v", got, want)
	}
}

func TestRefineIfFeatureExclusionIsTrackedForAugmentAndDeviation(t *testing.T) {
	const user = `module rfu {
  yang-version 1.1; namespace "urn:rfu"; prefix rfu;
  import rf { prefix rf; }
  augment "/rf:c/rf:e" { leaf added { type string; } }
  deviation "/rf:c/rf:e/rf:inner" { deviate not-supported; }
}`
	_, err := buildModeContext(t, cambium.ValidationStrict, nil, refineIfFeatureModule, user)
	if err == nil || !strings.Contains(err.Error(), "excluded by feature policy") {
		t.Fatalf("strict Build error = %v, want feature exclusion detail", err)
	}

	ctx, err := buildModeContext(t, cambium.ValidationVendorCompatible, nil, refineIfFeatureModule, user)
	if err != nil {
		t.Fatalf("vendor Build: %v", err)
	}
	report := ctx.LoadReport()
	if omitted := report.OmittedContent(); len(omitted) != 1 || omitted[0].Module != "rfu" || !strings.Contains(omitted[0].Message, `"/rf:c/rf:e"`) {
		t.Fatalf("omitted content = %#v, want the rfu augment", omitted)
	}
	if !diagnosticContains(report.Warnings, "rfu", "deviation") {
		t.Fatalf("warnings = %#v, want skipped deviation warning", report.Warnings)
	}

	ctx, err = buildModeContext(t, cambium.ValidationStrict, map[string][]string{"rf": {"f"}}, refineIfFeatureModule, user)
	if err != nil {
		t.Fatalf("strict Build with f: %v", err)
	}
	mod, _ := ctx.Schema("rf")
	assertChildNames(t, mod, "/rf:c/rf:e", "added")
}

func TestRefineOfFeatureExcludedGroupingNodeIsNotAnError(t *testing.T) {
	// The refine names a node the grouping declares; the feature set only
	// removes it from the effective schema, so there is nothing to refine.
	for _, refine := range []string{
		`refine x { default abc; }`,
		`refine x { if-feature f; default abc; }`,
		`refine "d/y" { default abc; }`,
	} {
		source := `module rg {
  yang-version 1.1; namespace "urn:rg"; prefix rg;
  feature f;
  grouping g {
    leaf x { if-feature f; type string; }
    container d { leaf y { if-feature f; type string; } }
    leaf keep { type string; }
  }
  container c { uses g { ` + refine + ` } }
}`
		for _, mode := range []cambium.ValidationMode{cambium.ValidationStrict, cambium.ValidationVendorCompatible} {
			ctx, err := buildModeContext(t, mode, nil, source)
			if err != nil {
				t.Fatalf("%s mode %d: Build: %v", refine, mode, err)
			}
			mod, _ := ctx.Schema("rg")
			assertChildNames(t, mod, "/rg:c", "d", "keep")
			if warnings := ctx.LoadReport().Warnings; len(warnings) != 0 {
				t.Fatalf("%s mode %d: warnings = %#v, want none", refine, mode, warnings)
			}
		}
		ctx, err := buildModeContext(t, cambium.ValidationStrict, map[string][]string{"rg": {"f"}}, source)
		if err != nil {
			t.Fatalf("%s: Build with f: %v", refine, err)
		}
		mod, _ := ctx.Schema("rg")
		assertChildNames(t, mod, "/rg:c", "x", "d", "keep")
	}

	// A refine whose path names no grouping node is still an error.
	const typo = `module rt {
  yang-version 1.1; namespace "urn:rt"; prefix rt;
  feature f;
  grouping g { leaf x { if-feature f; type string; } }
  container c { uses g { refine xx { if-feature f; default abc; } } }
}`
	for _, mode := range []cambium.ValidationMode{cambium.ValidationStrict, cambium.ValidationVendorCompatible} {
		if _, err := buildModeContext(t, mode, nil, typo); err == nil || !strings.Contains(err.Error(), `refine "xx" target not found`) {
			t.Fatalf("mode %d: Build error = %v, want refine target not found", mode, err)
		}
	}
}
