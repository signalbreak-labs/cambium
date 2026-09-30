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

func retentionBuilder(t *testing.T, retain bool) *cambium.ContextBuilder {
	t.Helper()
	b, err := cambium.NewContextBuilder(cambium.ContextFlags{DisableSearchdirCwd: true})
	if err != nil {
		t.Fatal(err)
	}
	if err := b.SetFeaturePolicy(cambium.FeaturePolicy{RetainAll: retain}); err != nil {
		t.Fatal(err)
	}
	return b
}

func TestFeatureRetentionPreservesComplementaryDeclarations(t *testing.T) {
	for _, retain := range []bool{false, true} {
		b := retentionBuilder(t, retain)
		for _, src := range []string{s4FeatureLib, s4Features} {
			if err := b.LoadModuleStr(src); err != nil {
				t.Fatal(err)
			}
		}
		ctx, err := b.Build()
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(ctx.Close)
		mod, err := ctx.Schema("feat")
		if err != nil {
			t.Fatal(err)
		}
		want := "always,not-a"
		if retain {
			want = "always,only-a,only-b,not-a,a-or-c,a-and-c,lib-gated,from-uses,aug-a"
		}
		if got := names(schemaNodeAt(t, mod, "/f:top").Children()); got != want {
			t.Errorf("RetainAll=%v: children = %s, want %s", retain, got, want)
		}
		if retain {
			for _, path := range []string{"/f:top/f:only-a", "/f:top/f:not-a", "/f:top/f:lib-gated", "/f:top/f:from-uses", "/f:top/f:aug-a"} {
				if len(schemaNodeAt(t, mod, path).IfFeatures()) == 0 {
					t.Errorf("%s lost its feature expression", path)
				}
			}
			base, _ := mod.Identity("base-id")
			if got := base.DerivedClosure(); len(got) != 1 || got[0].Name() != "gated-id" {
				t.Errorf("retained identities = %v", got)
			}
		}
		// Retention changes declaration visibility, not the evaluated feature
		// selection or a claim about device capabilities.
		if enabled, known := mod.FeatureValue("a"); enabled || !known {
			t.Errorf("feature a = %v,%v", enabled, known)
		}
		report := ctx.LoadReport()
		if report.FeaturePolicy.RetainAll != retain || len(report.EnabledFeatures) != 0 || len(report.DisabledFeatures) != 4 || len(report.Warnings) != 0 {
			t.Errorf("RetainAll=%v: report = %+v", retain, report)
		}
	}
}

func TestFeatureRetentionIncludesImportsAndSubmodules(t *testing.T) {
	dir := t.TempDir()
	files := map[string]string{
		"lib.yang": `module lib { yang-version 1.1; namespace "urn:lib"; prefix l; feature remote; }`,
		"part.yang": `submodule part {
  yang-version 1.1; belongs-to root { prefix r; }
  feature local;
  container included { if-feature local; leaf value { type string; } }
}`,
		"root.yang": `module root {
  yang-version 1.1; namespace "urn:root"; prefix r;
  import lib { prefix l; } include part;
  leaf remote { if-feature l:remote; type string; }
  leaf fallback { if-feature "not l:remote"; type string; }
}`,
	}
	for name, src := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(src), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	b := retentionBuilder(t, true)
	if err := b.SearchPath(dir); err != nil {
		t.Fatal(err)
	}
	if err := b.LoadModuleFromPath(filepath.Join(dir, "root.yang")); err != nil {
		t.Fatal(err)
	}
	ctx, err := b.Build()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(ctx.Close)
	mod, err := ctx.Schema("root")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"included", "remote", "fallback"} {
		if _, ok := mod.TopLevel().Lookup(name); !ok {
			t.Errorf("lost %s", name)
		}
	}
	report := ctx.LoadReport()
	if len(report.TransitiveImports) != 1 || len(report.IncludedSubmodules) != 1 || len(report.DisabledFeatures) != 2 {
		t.Fatalf("load report = %+v", report)
	}
}

func TestFeatureRetentionKeepsTypeValuesAndRefines(t *testing.T) {
	b := retentionBuilder(t, true)
	if err := b.LoadModuleStr(`module values {
  yang-version 1.1; namespace "urn:values"; prefix v;
  feature f;
  grouping g { leaf refined { type string; } }
  container top {
    leaf enums { type enumeration {
      enum yes { if-feature f; } enum no { if-feature "not f"; }
    } }
    leaf bits { type bits {
      bit yes { if-feature f; } bit no { if-feature "not f"; }
    } }
    uses g { refine refined { if-feature f; } }
  }
}`); err != nil {
		t.Fatal(err)
	}
	ctx, err := b.Build()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(ctx.Close)
	mod, err := ctx.Schema("values")
	if err != nil {
		t.Fatal(err)
	}
	enums, _ := schemaNodeAt(t, mod, "/v:top/v:enums").LeafType()
	var gotEnums []string
	for _, value := range enums.Resolved().(cambium.ResolvedEnumeration).Values() {
		gotEnums = append(gotEnums, value.Name())
	}
	bits, _ := schemaNodeAt(t, mod, "/v:top/v:bits").LeafType()
	var gotBits []string
	for _, value := range bits.Resolved().(cambium.ResolvedBits).Values() {
		gotBits = append(gotBits, value.Name())
	}
	for _, got := range [][]string{gotEnums, gotBits} {
		if !reflect.DeepEqual(got, []string{"yes", "no"}) {
			t.Errorf("retained values = %v", got)
		}
	}
	if got := schemaNodeAt(t, mod, "/v:top/v:refined").IfFeatures(); !reflect.DeepEqual(got, []string{"f"}) {
		t.Errorf("refined feature metadata = %v", got)
	}
}

func TestFeatureRetentionWithExplicitFeatureSelection(t *testing.T) {
	b := retentionBuilder(t, true)
	if err := b.SetFeatures("feat", []string{"a", "b", "c"}); err != nil {
		t.Fatal(err)
	}
	for _, src := range []string{s4FeatureLib, s4Features} {
		if err := b.LoadModuleStr(src); err != nil {
			t.Fatal(err)
		}
	}
	ctx, err := b.Build()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(ctx.Close)
	mod, err := ctx.Schema("feat")
	if err != nil {
		t.Fatal(err)
	}
	if got := names(schemaNodeAt(t, mod, "/f:top").Children()); got != "always,only-a,only-b,not-a,a-or-c,a-and-c,lib-gated,from-uses,aug-a" {
		t.Errorf("selected features changed retention: %s", got)
	}
	if enabled, known := mod.FeatureValue("a"); !enabled || !known {
		t.Errorf("selected feature a = %v,%v", enabled, known)
	}
	if report := ctx.LoadReport(); len(report.EnabledFeatures) != 3 || len(report.DisabledFeatures) != 1 || len(report.Warnings) != 0 {
		t.Errorf("feature report = %+v", report)
	}
}

func TestFeatureRetentionAppliesGatedDeviationsBeforeLeafrefs(t *testing.T) {
	b := retentionBuilder(t, true)
	if err := b.SetDeviationPolicy(cambium.DeviationPolicy{IgnoreNotSupported: true}); err != nil {
		t.Fatal(err)
	}
	if err := b.LoadModuleStr(`module amended {
  yang-version 1.1; namespace "urn:amended"; prefix a;
  feature f;
  leaf target { if-feature f; type string; }
  leaf ref { type leafref { path "/a:target"; } }
  deviation "/a:target" { if-feature f; deviate not-supported; }
  deviation "/a:target" { if-feature "not f"; deviate add { units "widgets"; } }
}`); err != nil {
		t.Fatal(err)
	}
	ctx, err := b.Build()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(ctx.Close)
	mod, err := ctx.Schema("amended")
	if err != nil {
		t.Fatal(err)
	}
	resolution, err := cambium.ResolveLeafref(schemaNodeAt(t, mod, "/a:ref"))
	if err != nil {
		t.Fatal(err)
	}
	if units, ok := resolution.Target.Units(); !ok || units != "widgets" {
		t.Errorf("target units = %q,%v", units, ok)
	}
	if report := ctx.LoadReport(); len(report.IgnoredDeviations) != 1 || !report.FeaturePolicy.RetainAll || len(report.OmittedContent()) != 0 {
		t.Errorf("deviation report = %+v", report)
	}
}

func TestFeatureRetentionStillRejectsInvalidExpressions(t *testing.T) {
	for _, tc := range []struct{ name, source, want string }{
		{"unknown", `feature f; leaf x { if-feature missing; type string; }`, "if-feature"},
		{"malformed", `feature f; leaf x { if-feature "f and"; type string; }`, "if-feature"},
		{"cycle", `feature a { if-feature b; } feature b { if-feature a; } leaf x { if-feature a; type string; }`, "if-feature"},
		{"conditional key", `feature f; list x { key name; leaf name { if-feature f; type string; } }`, "if-feature"},
		{"conditional enum default", `feature f; leaf x { type enumeration { enum yes { if-feature f; } } default yes; }`, "if-feature"},
		{"conditional bit default", `feature f; leaf x { type bits { bit yes { if-feature f; } } default yes; }`, "if-feature"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b := retentionBuilder(t, true)
			if err := b.LoadModuleStr(`module invalid { yang-version 1.1; namespace "urn:invalid"; prefix i; ` + tc.source + ` }`); err != nil {
				t.Fatal(err)
			}
			ctx, err := b.Build()
			if err == nil || ctx != nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Build = %v,%v; want %s error", ctx, err, tc.want)
			}
		})
	}
}

func TestFeatureRetentionCanResetBeforeBuildAndFreezes(t *testing.T) {
	b := retentionBuilder(t, true)
	if err := b.LoadModuleStr(s4FeatureLib); err != nil {
		t.Fatal(err)
	}
	if err := b.LoadModuleStr(s4Features); err != nil {
		t.Fatal(err)
	}
	if err := b.SetFeaturePolicy(cambium.FeaturePolicy{}); err != nil {
		t.Fatal(err)
	}
	copyBeforeBuild := *b
	ctx, err := b.Build()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(ctx.Close)
	mod, err := ctx.Schema("feat")
	if err != nil {
		t.Fatal(err)
	}
	if got := names(schemaNodeAt(t, mod, "/f:top").Children()); got != "always,not-a" {
		t.Errorf("reset policy children = %s", got)
	}
	if err := b.SetFeaturePolicy(cambium.FeaturePolicy{RetainAll: true}); err == nil {
		t.Error("frozen builder accepted feature policy mutation")
	}
	if err := copyBeforeBuild.SetFeaturePolicy(cambium.FeaturePolicy{RetainAll: true}); err == nil {
		t.Error("copied builder changed a frozen context's feature policy")
	}
	if ctx.LoadReport().FeaturePolicy.RetainAll {
		t.Error("frozen context policy changed")
	}
	var nilBuilder *cambium.ContextBuilder
	if err := nilBuilder.SetFeaturePolicy(cambium.FeaturePolicy{}); err == nil {
		t.Error("nil builder accepted feature policy")
	}
}
