// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

package cambium_test

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/signalbreak-labs/cambium/go/cambium"
)

// buildPolicyContext builds a cwd-independent context from in-memory sources
// with an explicit deviation policy. It returns the Build error unchanged.
func buildPolicyContext(t *testing.T, policy cambium.DeviationPolicy, sources ...string) (*cambium.Context, error) {
	t.Helper()
	builder, err := cambium.NewContextBuilder(cambium.ContextFlags{DisableSearchdirCwd: true})
	if err != nil {
		t.Fatalf("NewContextBuilder: %v", err)
	}
	if err := builder.SetDeviationPolicy(policy); err != nil {
		t.Fatalf("SetDeviationPolicy: %v", err)
	}
	for _, source := range sources {
		if err := builder.LoadModuleStr(source); err != nil {
			t.Fatalf("LoadModuleStr: %v", err)
		}
	}
	ctx, err := builder.Build()
	if err == nil {
		t.Cleanup(ctx.Close)
	}
	return ctx, err
}

const deviationPolicyLocal = `module d {
  yang-version 1.1; namespace "urn:d"; prefix d;
  leaf target { type string; }
  leaf ref { type leafref { path "/d:target"; } }
  deviation "/d:target" {
    description "vendor drops target";
    deviate not-supported;
  }
}`

func TestDeviationPolicyDefaultAppliesNotSupported(t *testing.T) {
	_, err := buildPolicyContext(t, cambium.DeviationPolicy{}, deviationPolicyLocal)
	if err == nil {
		t.Fatal("Build succeeded, want dangling leafref after not-supported")
	}
	var leafrefErr *cambium.LeafrefResolutionError
	if !errors.As(err, &leafrefErr) && !strings.Contains(err.Error(), "target not found") {
		t.Fatalf("Build error = %v, want leafref target not found", err)
	}
}

func TestDeviationPolicyIgnoreNotSupportedBeforeValidation(t *testing.T) {
	ctx, err := buildPolicyContext(t, cambium.DeviationPolicy{IgnoreNotSupported: true}, deviationPolicyLocal)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	mod, err := ctx.Schema("d")
	if err != nil {
		t.Fatal(err)
	}
	if got, want := childNamesForProfile(mod.Children()), []string{"target", "ref"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("children = %v, want %v", got, want)
	}
	ref := schemaNodeAt(t, mod, "/d:ref")
	res, err := cambium.ResolveLeafref(ref)
	if err != nil {
		t.Fatalf("ResolveLeafref: %v", err)
	}
	if res.Target.Name() != "target" {
		t.Fatalf("leafref target = %q, want target", res.Target.Name())
	}

	// The ignored deviation is still reported, marked as not applied, on
	// the module and on the surviving node.
	devs := mod.Deviations()
	if len(devs) != 1 || devs[0].Type() != "not-supported" || devs[0].Applied() {
		t.Fatalf("module deviations = %+v, want one unapplied not-supported", devs)
	}
	if desc, ok := devs[0].Description(); !ok || desc != "vendor drops target" {
		t.Fatalf("deviation description = %q,%v", desc, ok)
	}
	if loc := devs[0].SourceLocation(); loc.Line != 7 {
		t.Fatalf("deviation source line = %d, want 7, the deviate statement (%+v)", loc.Line, loc)
	}
	target := schemaNodeAt(t, mod, "/d:target")
	prov := target.DeviationProvenance()
	if len(prov) != 1 || prov[0].Applied() {
		t.Fatalf("target provenance = %+v, want one unapplied deviation", prov)
	}
	report := ctx.LoadReport()
	if report.DeviationPolicy != (cambium.DeviationPolicy{IgnoreNotSupported: true}) {
		t.Fatalf("report policy = %+v", report.DeviationPolicy)
	}
	if len(report.IgnoredDeviations) != 1 || report.IgnoredDeviations[0].TargetPath() != "/d:target" {
		t.Fatalf("report ignored deviations = %+v", report.IgnoredDeviations)
	}
}

func TestDeviationPolicyIgnoreNotSupportedKeepsOtherOperations(t *testing.T) {
	const target = `module tgt {
  yang-version 1.1; namespace "urn:tgt"; prefix t;
  container top {
    leaf before { type string; }
    list entry {
      key name;
      leaf name { type string; }
      leaf value { type string; default "x"; }
    }
    leaf ref { type leafref { path "../entry/name"; } }
    leaf after { type string; }
  }
}`
	const deviator = `module dev {
  yang-version 1.1; namespace "urn:dev"; prefix dv;
  import tgt { prefix t; }
  deviation "/t:top/t:entry" { deviate not-supported; }
  deviation "/t:top/t:entry/t:value" { deviate replace { default "y"; } }
  deviation "/t:top/t:after" { deviate add { must "true()"; } }
}`
	ctx, err := buildPolicyContext(t, cambium.DeviationPolicy{IgnoreNotSupported: true}, target, deviator)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	mod, _ := ctx.Schema("tgt")
	top := schemaNodeAt(t, mod, "/t:top")
	if got, want := childNamesForProfile(top.Children()), []string{"before", "entry", "ref", "after"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("children = %v, want %v", got, want)
	}
	entry := schemaNodeAt(t, mod, "/t:top/entry")
	if got := entry.KeyNames(); !reflect.DeepEqual(got, []string{"name"}) {
		t.Fatalf("keys = %v", got)
	}
	value := schemaNodeAt(t, mod, "/t:top/entry/value")
	if got, ok := value.DefaultValue(); !ok || got != "y" {
		t.Fatalf("value default = %q,%v, want y", got, ok)
	}
	after := schemaNodeAt(t, mod, "/t:top/after")
	if got := len(after.Musts()); got != 1 {
		t.Fatalf("after musts = %d, want 1", got)
	}

	// Loading the same modules with the default policy removes the list, so
	// the leafref is dangling and the build fails.
	if _, err := buildPolicyContext(t, cambium.DeviationPolicy{}, target, deviator); err == nil {
		t.Fatal("default policy Build succeeded, want error")
	}
	// Omitting the deviation module is a distinct policy from ignoring
	// not-supported: nothing is deviated at all.
	ctx, err = buildPolicyContext(t, cambium.DeviationPolicy{}, target)
	if err != nil {
		t.Fatalf("Build without deviation module: %v", err)
	}
	mod, _ = ctx.Schema("tgt")
	value = schemaNodeAt(t, mod, "/t:top/entry/value")
	if got, ok := value.DefaultValue(); !ok || got != "x" {
		t.Fatalf("undeviated default = %q,%v, want x", got, ok)
	}
	if len(ctx.LoadReport().DeviationModules) != 0 {
		t.Fatal("report lists a deviation module that was not loaded")
	}
}

func TestDeviationPolicyAppliedRemovalIsReportedAfterNodeIsGone(t *testing.T) {
	const source = `module rm {
  yang-version 1.1; namespace "urn:rm"; prefix rm;
  container top {
    leaf keep { type string; }
    leaf gone { type string; }
    leaf tail { type string; }
  }
  deviation "/rm:top/rm:gone" { deviate not-supported; }
}`
	ctx, err := buildPolicyContext(t, cambium.DeviationPolicy{}, source)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	mod, _ := ctx.Schema("rm")
	top := schemaNodeAt(t, mod, "/rm:top")
	if got, want := childNamesForProfile(top.Children()), []string{"keep", "tail"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("children = %v, want %v", got, want)
	}
	devs := mod.Deviations()
	if len(devs) != 1 || !devs[0].Applied() || devs[0].TargetPath() != "/rm:top/rm:gone" || devs[0].SourceModule() != "rm" {
		t.Fatalf("deviations = %+v, want one applied removal of /rm:top/rm:gone", devs)
	}
	if len(ctx.LoadReport().IgnoredDeviations) != 0 {
		t.Fatal("default policy reported ignored deviations")
	}
}
