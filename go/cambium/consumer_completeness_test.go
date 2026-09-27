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
