// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

package cambium_test

import (
	"fmt"
	"testing"

	"github.com/signalbreak-labs/cambium/go/cambium"
)

func buildRefinementContext(t *testing.T, ignore bool, source string) (*cambium.Context, error) {
	t.Helper()
	b := catalogBuilder(t)
	if err := b.SetRefinementPolicy(cambium.RefinementPolicy{IgnoreAll: ignore}); err != nil {
		t.Fatal(err)
	}
	if err := b.LoadModuleStr(source); err != nil {
		t.Fatal(err)
	}
	ctx, err := b.Build()
	if err == nil {
		t.Cleanup(ctx.Close)
	}
	return ctx, err
}

func TestRefinementPolicyIgnoreAllPreservesGroupingDeclarations(t *testing.T) {
	const source = `module refined {
  yang-version 1.1; namespace "urn:refined"; prefix r;
  feature optional;
  grouping fields {
    container inner {
      presence original; description original; reference original;
      must "true()";
      leaf defaulted { type string; default before; }
      leaf required { type string; }
      leaf gated { type string; }
      list items { key id; min-elements 1; max-elements 8; leaf id { type string; } }
    }
  }
  container top {
    uses fields {
      refine inner { presence refined; description refined; reference refined; config false; must "count(items) > 0"; }
      refine inner/defaulted { default after; }
      refine inner/required { mandatory true; }
      refine inner/gated { if-feature optional; }
      refine inner/items { min-elements 2; max-elements 4; }
    }
  }
}`
	for _, ignore := range []bool{false, true} {
		ctx, err := buildRefinementContext(t, ignore, source)
		if err != nil {
			t.Fatal(err)
		}
		mod, _ := ctx.Schema("refined")
		inner := schemaNodeAt(t, mod, "/r:top/r:inner")
		wantText, wantDefault := "refined", "after"
		wantMusts, wantMin, wantMax := 2, uint32(2), uint32(4)
		wantConfig := cambium.ConfigRo
		if ignore {
			wantText, wantDefault = "original", "before"
			wantMusts, wantMin, wantMax = 1, 1, 8
			wantConfig = cambium.ConfigRw
		}
		for _, got := range []string{optionalText(inner.Presence()), optionalText(inner.Description()), optionalText(inner.Reference())} {
			if got != wantText {
				t.Errorf("ignore=%v: refined metadata = %q, want %q", ignore, got, wantText)
			}
		}
		if inner.Config() != wantConfig || len(inner.Musts()) != wantMusts {
			t.Errorf("ignore=%v: config/musts = %v, %v", ignore, inner.Config(), inner.Musts())
		}
		if got := optionalText(schemaNodeAt(t, mod, "/r:top/r:inner/r:defaulted").DefaultValue()); got != wantDefault {
			t.Errorf("ignore=%v: default = %q", ignore, got)
		}
		if schemaNodeAt(t, mod, "/r:top/r:inner/r:required").IsMandatory() == ignore {
			t.Errorf("ignore=%v: mandatory refinement not honored", ignore)
		}
		if _, ok := inner.Children().Lookup("gated"); ok != ignore {
			t.Errorf("ignore=%v: refine if-feature visibility = %v", ignore, ok)
		}
		items := schemaNodeAt(t, mod, "/r:top/r:inner/r:items")
		minimum, _ := items.MinElements()
		maximum, _ := items.MaxElements()
		if minimum != wantMin || maximum != wantMax {
			t.Errorf("ignore=%v: cardinality = %d..%d", ignore, minimum, maximum)
		}
		if ctx.LoadReport().RefinementPolicy.IgnoreAll != ignore {
			t.Error("load report lost refinement policy")
		}
	}
}

func optionalText(value string, _ bool) string { return value }

func TestRefinementPolicyIgnoreAllValidatesShapeWithoutRequiringTargets(t *testing.T) {
	for _, tc := range []struct {
		refine string
		valid  bool
	}{
		{`refine absent { mandatory true; }`, true},
		{`refine /absolute { mandatory true; }`, false},
		{`refine absent { default one; default two; }`, false},
		{`refine absent { mandatory invalid; }`, false},
		{`refine absent { max-elements 0; }`, false},
		{`refine absent { must "true()" { description one; description two; } }`, false},
	} {
		source := fmt.Sprintf(`module shape { yang-version 1.1; namespace "urn:shape"; prefix s;
  grouping fields { leaf value { type string; } }
  container top { uses fields { %s } }
}`, tc.refine)
		if _, err := buildRefinementContext(t, true, source); (err == nil) != tc.valid {
			t.Errorf("%s: Build error = %v, want valid=%v", tc.refine, err, tc.valid)
		}
	}
}

func TestRefinementPolicyCopiedBuilderCannotMutateFrozenContext(t *testing.T) {
	b := catalogBuilder(t)
	copyOfBuilder := *b
	ctx, err := b.Build()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(ctx.Close)
	if err := copyOfBuilder.SetRefinementPolicy(cambium.RefinementPolicy{IgnoreAll: true}); err == nil {
		t.Fatal("copied builder changed refinement policy after Build")
	}
	if ctx.LoadReport().RefinementPolicy.IgnoreAll {
		t.Fatal("frozen refinement policy changed")
	}
}
