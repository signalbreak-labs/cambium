// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

package cambium_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/signalbreak-labs/cambium/go/cambium"
)

func buildModeContext(t *testing.T, mode cambium.ValidationMode, features map[string][]string, sources ...string) (*cambium.Context, error) {
	t.Helper()
	builder, err := cambium.NewContextBuilder(cambium.ContextFlags{DisableSearchdirCwd: true})
	if err != nil {
		t.Fatalf("NewContextBuilder: %v", err)
	}
	if err := builder.SetValidationMode(mode); err != nil {
		t.Fatalf("SetValidationMode: %v", err)
	}
	for _, source := range sources {
		if err := builder.LoadModuleStr(source); err != nil {
			t.Fatalf("LoadModuleStr: %v", err)
		}
	}
	for module, names := range features {
		if err := builder.SetFeatures(module, names); err != nil {
			t.Fatalf("SetFeatures: %v", err)
		}
	}
	ctx, err := builder.Build()
	if err == nil {
		t.Cleanup(ctx.Close)
	}
	return ctx, err
}

const completenessTypo = `module typo {
  yang-version 1.1; namespace "urn:typo"; prefix t;
  container real;
  augment "/t:typo" { leaf lost { type string; } }
}`

func TestAugmentTypoFailsInEveryValidationMode(t *testing.T) {
	for _, mode := range []cambium.ValidationMode{cambium.ValidationStrict, cambium.ValidationVendorCompatible} {
		_, err := buildModeContext(t, mode, nil, completenessTypo)
		if err == nil {
			t.Fatalf("mode %d: Build succeeded with unresolved augment target, want error", mode)
		}
		diag := cambium.DiagnosticFromError(err)
		if diag.Kind != cambium.DiagnosticUnresolvedPath {
			t.Fatalf("mode %d: diagnostic kind = %q, want %q (%v)", mode, diag.Kind, cambium.DiagnosticUnresolvedPath, err)
		}
		if !strings.Contains(err.Error(), `augment "/t:typo" target not found`) {
			t.Fatalf("mode %d: error = %v", mode, err)
		}
		if strings.Contains(err.Error(), "feature") {
			t.Fatalf("mode %d: typo reported as feature exclusion: %v", mode, err)
		}
	}
}

const completenessGatedTarget = `module gated {
  yang-version 1.1; namespace "urn:gated"; prefix g;
  feature adv;
  container root {
    leaf first { type string; }
    container opt { if-feature adv; }
    choice pick {
      case a { leaf a1 { type string; } }
      leaf b1 { if-feature adv; type string; }
    }
    uses extra { if-feature adv; }
  }
  grouping extra { container grouped; }
}`

const completenessGatedUser = `module gated-user {
  yang-version 1.1; namespace "urn:gated-user"; prefix gu;
  import gated { prefix g; }
  augment "/g:root/g:opt" { leaf added { type string; } }
  augment "/g:root/g:pick/g:b1" { leaf case-added { type string; } }
  augment "/g:root/g:grouped" { leaf via-uses { type string; } }
}`

func TestAugmentFeatureExcludedTargetIsDistinguished(t *testing.T) {
	// Strict mode rejects, but names the feature exclusion rather than a
	// generic missing target.
	_, err := buildModeContext(t, cambium.ValidationStrict, nil, completenessGatedTarget, completenessGatedUser)
	if err == nil {
		t.Fatal("strict Build succeeded, want feature-excluded target error")
	}
	if !strings.Contains(err.Error(), "excluded by feature policy") {
		t.Fatalf("strict error = %v, want feature exclusion detail", err)
	}

	// Vendor mode skips each augment, with a warning that is also reported as
	// omitted schema content.
	ctx, err := buildModeContext(t, cambium.ValidationVendorCompatible, nil, completenessGatedTarget, completenessGatedUser)
	if err != nil {
		t.Fatalf("vendor Build: %v", err)
	}
	report := ctx.LoadReport()
	omitted := report.OmittedContent()
	if len(omitted) != 3 {
		t.Fatalf("omitted content = %#v, want 3 skipped augments", omitted)
	}
	for _, diag := range omitted {
		if diag.Kind != cambium.DiagnosticOmittedSchemaContent || diag.Module != "gated-user" || diag.Source.Line == 0 {
			t.Fatalf("omitted diagnostic = %#v", diag)
		}
		if !strings.Contains(diag.Message, "excluded by feature policy") {
			t.Fatalf("omitted message = %q", diag.Message)
		}
	}
	found := 0
	for _, w := range report.Warnings {
		if w.Kind == cambium.DiagnosticOmittedSchemaContent {
			found++
		}
	}
	if found != 3 {
		t.Fatalf("warnings carry %d omitted-content diagnostics, want 3", found)
	}

	// Enabling the feature makes every target present and the schema complete.
	ctx, err = buildModeContext(t, cambium.ValidationStrict, map[string][]string{"gated": {"adv"}}, completenessGatedTarget, completenessGatedUser)
	if err != nil {
		t.Fatalf("strict Build with adv: %v", err)
	}
	mod, _ := ctx.Schema("gated")
	root := schemaNodeAt(t, mod, "/g:root")
	if got, want := childNamesForProfile(root.Children()), []string{"first", "opt", "pick", "grouped"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("root children = %v, want %v", got, want)
	}
	for _, path := range []string{"/g:root/g:opt/gu:added", "/g:root/g:pick/g:b1/gu:case-added", "/g:root/g:grouped/gu:via-uses"} {
		if _, err := mod.FindPath(path); err != nil {
			t.Fatalf("FindPath(%s): %v", path, err)
		}
	}
	if got := ctx.LoadReport().OmittedContent(); len(got) != 0 {
		t.Fatalf("omitted content with feature enabled = %#v", got)
	}
}

func TestAugmentMissingDependencyTargetFailsInVendorMode(t *testing.T) {
	// The target module is present, but the target path names a node that
	// was never declared: vendor mode must not treat this as feature policy.
	const target = `module present {
  yang-version 1.1; namespace "urn:present"; prefix p;
  feature adv;
  container root { container opt { if-feature adv; } }
}`
	const user = `module present-user {
  yang-version 1.1; namespace "urn:present-user"; prefix pu;
  import present { prefix p; }
  augment "/p:root/p:opt/p:deeper" { leaf x { type string; } }
  augment "/p:root/p:missing" { leaf y { type string; } }
}`
	_, err := buildModeContext(t, cambium.ValidationVendorCompatible, nil, target, user)
	if err == nil {
		t.Fatal("vendor Build succeeded, want unresolved augment error")
	}
	if !strings.Contains(err.Error(), `augment "/p:root/p:missing" target not found`) {
		t.Fatalf("error = %v, want missing target for /p:root/p:missing", err)
	}
}

func TestDeviationFeatureExcludedTargetIsDistinguished(t *testing.T) {
	const target = `module dg {
  yang-version 1.1; namespace "urn:dg"; prefix dg;
  feature adv;
  container root {
    leaf keep { type string; }
    leaf gated { if-feature adv; type string; default "a"; }
  }
}`
	const deviator = `module dg-dev {
  yang-version 1.1; namespace "urn:dg-dev"; prefix dd;
  import dg { prefix dg; }
  deviation "/dg:root/dg:gated" { deviate replace { default "b"; } }
}`
	_, err := buildModeContext(t, cambium.ValidationStrict, nil, target, deviator)
	if err == nil || !strings.Contains(err.Error(), "excluded by feature policy") {
		t.Fatalf("strict Build error = %v, want feature exclusion detail", err)
	}
	ctx, err := buildModeContext(t, cambium.ValidationVendorCompatible, nil, target, deviator)
	if err != nil {
		t.Fatalf("vendor Build: %v", err)
	}
	report := ctx.LoadReport()
	if len(report.OmittedContent()) != 0 {
		t.Fatalf("skipped deviation reported as omitted content: %#v", report.OmittedContent())
	}
	if !diagnosticContains(report.Warnings, "dg-dev", "deviation") {
		t.Fatalf("warnings = %#v, want skipped deviation warning", report.Warnings)
	}
	ctx, err = buildModeContext(t, cambium.ValidationStrict, map[string][]string{"dg": {"adv"}}, target, deviator)
	if err != nil {
		t.Fatalf("strict Build with adv: %v", err)
	}
	mod, _ := ctx.Schema("dg")
	if got, ok := schemaNodeAt(t, mod, "/dg:root/gated").DefaultValue(); !ok || got != "b" {
		t.Fatalf("deviated default = %q,%v, want b", got, ok)
	}
	// A deviation typo still fails in vendor mode.
	const typo = `module dg-typo {
  yang-version 1.1; namespace "urn:dg-typo"; prefix dt;
  import dg { prefix dg; }
  deviation "/dg:root/dg:gatd" { deviate not-supported; }
}`
	if _, err := buildModeContext(t, cambium.ValidationVendorCompatible, nil, target, typo); err == nil {
		t.Fatal("vendor Build with deviation typo succeeded, want error")
	}
}

func TestFeatureExclusionMatchesQualifiedStep(t *testing.T) {
	// The feature-gated node belongs to module qa. A path step naming the same
	// local name under another module's prefix, or under a prefix that does
	// not resolve, is a typo, not a feature exclusion.
	const owner = `module qa {
  yang-version 1.1; namespace "urn:qa"; prefix qa;
  feature adv;
  container top { container x { if-feature adv; } }
  container gated { if-feature adv; }
}`
	const other = `module qb {
  yang-version 1.1; namespace "urn:qb"; prefix qb;
  container unrelated;
}`
	const wrongModule = `module qc {
  yang-version 1.1; namespace "urn:qc"; prefix qc;
  import qa { prefix qa; }
  import qb { prefix qb; }
  augment "/qa:top/qb:x" { leaf lost { type string; } }
}`
	const wrongTopModule = `module qd {
  yang-version 1.1; namespace "urn:qd"; prefix qd;
  import qb { prefix qb; }
  augment "/qb:gated" { leaf lost { type string; } }
}`
	const unknownPrefix = `module qe {
  yang-version 1.1; namespace "urn:qe"; prefix qe;
  feature adv;
  container gated { if-feature adv; }
  augment "/zz:gated" { leaf lost { type string; } }
}`
	const nestedUnknownPrefix = `module qf {
  yang-version 1.1; namespace "urn:qf"; prefix qf;
  import qa { prefix qa; }
  augment "/qa:top/zz:x" { leaf lost { type string; } }
}`
	cases := []struct {
		name    string
		sources []string
	}{
		{"nested step under another module", []string{owner, other, wrongModule}},
		{"nested step under an unresolvable prefix", []string{owner, nestedUnknownPrefix}},
		{"top-level step under another module", []string{owner, other, wrongTopModule}},
		{"unresolvable prefix", []string{unknownPrefix}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for _, mode := range []cambium.ValidationMode{cambium.ValidationStrict, cambium.ValidationVendorCompatible} {
				_, err := buildModeContext(t, mode, nil, tc.sources...)
				if err == nil {
					t.Fatalf("mode %d: Build succeeded, want unresolved target error", mode)
				}
				if strings.Contains(err.Error(), "excluded by feature policy") {
					t.Fatalf("mode %d: typo reported as feature exclusion: %v", mode, err)
				}
			}
		})
	}
}

func TestFeatureExclusionCoversDisabledAugmentContent(t *testing.T) {
	// A node declared inside a feature-disabled augment is excluded by the
	// feature set, so an augment or deviation that targets it gets the same
	// treatment as a target excluded by its own if-feature.
	const base = `module da {
  yang-version 1.1; namespace "urn:da"; prefix da;
  container top { leaf keep { type string; } }
}`
	const vendor = `module dv {
  yang-version 1.1; namespace "urn:dv"; prefix dv;
  import da { prefix da; }
  feature ext;
  augment "/da:top" { if-feature ext; container vc { leaf q { type string; } } }
  augment "/da:top/dv:vc" { leaf z { type string; } }
}`
	const deviator = `module dd {
  yang-version 1.1; namespace "urn:dd"; prefix dd;
  import da { prefix da; }
  import dv { prefix dv; }
  deviation "/da:top/dv:vc/dv:q" { deviate not-supported; }
}`
	_, err := buildModeContext(t, cambium.ValidationStrict, nil, base, vendor, deviator)
	if err == nil || !strings.Contains(err.Error(), "excluded by feature policy") {
		t.Fatalf("strict Build error = %v, want feature exclusion detail", err)
	}
	ctx, err := buildModeContext(t, cambium.ValidationVendorCompatible, nil, base, vendor, deviator)
	if err != nil {
		t.Fatalf("vendor Build: %v", err)
	}
	report := ctx.LoadReport()
	omitted := report.OmittedContent()
	if len(omitted) != 1 || omitted[0].Module != "dv" || !strings.Contains(omitted[0].Message, `"/da:top/dv:vc"`) {
		t.Fatalf("omitted content = %#v, want the nested dv augment", omitted)
	}
	if !diagnosticContains(report.Warnings, "dd", "deviation") {
		t.Fatalf("warnings = %#v, want skipped deviation warning", report.Warnings)
	}
	mod, _ := ctx.Schema("da")
	if got, want := childNamesForProfile(schemaNodeAt(t, mod, "/da:top").Children()), []string{"keep"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("top children = %v, want %v", got, want)
	}

	// With the feature enabled every target exists, in declaration order.
	ctx, err = buildModeContext(t, cambium.ValidationStrict, map[string][]string{"dv": {"ext"}}, base, vendor)
	if err != nil {
		t.Fatalf("strict Build with ext: %v", err)
	}
	mod, _ = ctx.Schema("da")
	if got, want := childNamesForProfile(schemaNodeAt(t, mod, "/da:top/dv:vc").Children()), []string{"q", "z"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("vc children = %v, want %v", got, want)
	}

	// A typo into the disabled augment's target is still a typo.
	const typo = `module dt {
  yang-version 1.1; namespace "urn:dt"; prefix dt;
  import da { prefix da; }
  import dv { prefix dv; }
  augment "/da:top/dv:vcc" { leaf z { type string; } }
}`
	if _, err := buildModeContext(t, cambium.ValidationVendorCompatible, nil, base, vendor, typo); err == nil || strings.Contains(err.Error(), "feature") {
		t.Fatalf("vendor Build with typo error = %v, want plain unresolved target", err)
	}
}

func TestFeatureExclusionCoversDisabledUsesAugmentContent(t *testing.T) {
	const base = `module ua {
  yang-version 1.1; namespace "urn:ua"; prefix ua;
  feature ext;
  grouping g { container gc { leaf keep { type string; } } }
  container top { uses g { augment "gc" { if-feature ext; container inner; } } }
}`
	const deviator = `module ud {
  yang-version 1.1; namespace "urn:ud"; prefix ud;
  import ua { prefix ua; }
  deviation "/ua:top/ua:gc/ua:inner" { deviate not-supported; }
}`
	_, err := buildModeContext(t, cambium.ValidationStrict, nil, base, deviator)
	if err == nil || !strings.Contains(err.Error(), "excluded by feature policy") {
		t.Fatalf("strict Build error = %v, want feature exclusion detail", err)
	}
	ctx, err := buildModeContext(t, cambium.ValidationVendorCompatible, nil, base, deviator)
	if err != nil {
		t.Fatalf("vendor Build: %v", err)
	}
	if !diagnosticContains(ctx.LoadReport().Warnings, "ud", "deviation") {
		t.Fatalf("warnings = %#v, want skipped deviation warning", ctx.LoadReport().Warnings)
	}
}

func TestDisabledAugmentAddsNoLoadWarnings(t *testing.T) {
	// The disabled augment's path only resolves through the vendor local-name
	// fallback. It contributes nothing, so it must not add a fallback warning.
	const base = `module wa {
  yang-version 1.1; namespace "urn:wa"; prefix wa;
  container top { container inner; }
}`
	const vendor = `module wv {
  yang-version 1.1; namespace "urn:wv"; prefix wv;
  import wa { prefix wa; }
  feature ext;
  augment "/wa:top/wv:inner" { if-feature ext; leaf z { type string; } }
}`
	ctx, err := buildModeContext(t, cambium.ValidationVendorCompatible, nil, base, vendor)
	if err != nil {
		t.Fatalf("vendor Build: %v", err)
	}
	if warnings := ctx.LoadReport().Warnings; len(warnings) != 0 {
		t.Fatalf("warnings = %#v, want none for a disabled augment", warnings)
	}
}
