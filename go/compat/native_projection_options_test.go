// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

package compat_test

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/signalbreak-labs/cambium/go/cambium"
	"github.com/signalbreak-labs/cambium/go/compat"
)

func TestNativeProjectionExcludesModulesBeforeNameCollisions(t *testing.T) {
	sources := map[string]string{
		"base": `module base {
  yang-version 1.1; namespace "urn:base"; prefix b;
  container top { leaf before { type string; } leaf after { type string; } }
}`,
		"alpha": `module alpha {
  yang-version 1.1; namespace "urn:alpha"; prefix a;
  import base { prefix b; }
  augment "/b:top" {
    leaf state {
      type uint16 { range "1..20"; } default 7; units ticks;
      description "Selected state";
      must ". > 0" { error-message "State must be positive"; }
    }
    leaf retained { type string; }
  }
}`,
		"beta": `module beta {
  yang-version 1.1; namespace "urn:beta"; prefix x;
  import base { prefix b; }
  augment "/b:top" { leaf state { type boolean; default false; } }
}`,
	}
	for _, order := range [][]string{{"alpha", "beta"}, {"beta", "alpha"}} {
		t.Run(strings.Join(order, "-"), func(t *testing.T) {
			ctx := nativeProjectionContext(t, false, sources, order...)
			for _, options := range []compat.ContextProjectionOptions{{}, {ExcludedModules: []string{"missing", "x"}}} {
				if roots, err := compat.FromContextWithOptions(ctx, options); roots != nil || err == nil || !strings.Contains(err.Error(), "sibling name collision") {
					t.Fatalf("included collision = %v, %v, want no roots and collision error", roots, err)
				}
			}
			base, err := ctx.Schema("base")
			if err != nil {
				t.Fatal(err)
			}
			nativeTop, _ := base.Children().Lookup("top")
			alpha, _ := nativeTop.Children().LookupQualified("alpha", "state")
			beta, _ := nativeTop.Children().LookupQualified("beta", "state")
			roots, err := compat.FromContextWithOptions(ctx, compat.ContextProjectionOptions{ExcludedModules: []string{"beta", "beta", "missing"}})
			if err != nil {
				t.Fatal(err)
			}
			if got := childNames(roots); !slices.Equal(got, []string{"alpha", "base"}) {
				t.Fatalf("projected modules = %v, want alpha then base", got)
			}
			top := projectionRoot(t, roots, "base").Lookup("top")
			if got := childNames(top.Children()); !slices.Equal(got, []string{"before", "after", "state", "retained"}) {
				t.Fatalf("filtered children = %v", got)
			}
			state := top.Lookup("state")
			if state == nil || state.Parent != top || state.Type == nil || state.Type.Kind != compat.Yuint16 || state.Type.Range.String() != "1..20" {
				t.Fatalf("retained state lost parent or type: %+v", state)
			}
			if !slices.Equal(state.Default, []string{"7"}) || state.Units != "ticks" || state.Description != "Selected state" {
				t.Fatalf("retained state lost metadata: %+v", state)
			}
			if len(state.Extra["must"]) != 1 {
				t.Fatalf("retained must constraints = %v", state.Extra["must"])
			}
			must, ok := state.Extra["must"][0].(*compat.Must)
			if !ok || must.Name != ". > 0" || valueName(must.ErrorMessage) != "State must be positive" {
				t.Fatalf("retained must constraint = %+v", must)
			}
			if node, ok := state.NativeSchemaNode(); !ok || node != alpha {
				t.Fatal("retained state lost its native node identity")
			}
			if module, ok := state.NativeModule(); !ok || module != alpha.Module() {
				t.Fatal("retained state lost its native module identity")
			}
			if nodes := nativeTop.Children().LookupAll("state"); nodes.Len() != 2 {
				t.Fatalf("native context lost colliding nodes: %d", nodes.Len())
			}
			if node, ok := nativeTop.Children().LookupQualified("beta", "state"); !ok || node != beta {
				t.Fatal("excluded state changed in the native context")
			}
			if roots, err := compat.FromContext(ctx); roots != nil || err == nil || !strings.Contains(err.Error(), "sibling name collision") {
				t.Fatalf("fresh unfiltered projection = %v, %v, want original collision", roots, err)
			}
		})
	}
}

func TestNativeProjectionExcludesModuleRootBeforeInternalCollisions(t *testing.T) {
	ctx := nativeProjectionContext(t, false, map[string]string{
		"base": `module base {
  yang-version 1.1; namespace "urn:base"; prefix b;
  container top { leaf state { type string; } }
}`,
		"extension": `module extension {
  yang-version 1.1; namespace "urn:extension"; prefix e;
  import base { prefix b; }
  leaf local { type string; }
  augment "/b:top" { leaf state { type string; } }
}`,
	}, "extension")
	if roots, err := compat.FromContext(ctx); roots != nil || err == nil {
		t.Fatalf("unfiltered projection = %v, %v, want collision", roots, err)
	}
	roots, err := compat.FromContextWithOptions(ctx, compat.ContextProjectionOptions{ExcludedModules: []string{"base"}})
	if err != nil {
		t.Fatal(err)
	}
	if got := childNames(roots); !slices.Equal(got, []string{"extension"}) {
		t.Fatalf("roots = %v, want extension", got)
	}
	if got := childNames(roots[0].Children()); !slices.Equal(got, []string{"local"}) {
		t.Fatalf("remaining root children = %v, want local", got)
	}
}

func TestNativeProjectionExcludesModulesBeforeFlattenedChoiceCollisions(t *testing.T) {
	ctx := nativeProjectionContext(t, false, map[string]string{
		"base": `module base {
  yang-version 1.1; namespace "urn:base"; prefix b;
  container top { leaf state { type string; } }
}`,
		"extension": `module extension {
  yang-version 1.1; namespace "urn:extension"; prefix e;
  import base { prefix b; }
  augment "/b:top" { choice selection { case added { leaf state { type boolean; } } } }
}`,
	}, "extension")
	if roots, err := compat.FromContext(ctx); roots != nil || err == nil {
		t.Fatalf("unfiltered projection = %v, %v, want flattened collision", roots, err)
	}
	roots, err := compat.FromContextWithOptions(ctx, compat.ContextProjectionOptions{ExcludedModules: []string{"extension"}})
	if err != nil {
		t.Fatal(err)
	}
	top := projectionRoot(t, roots, "base").Lookup("top")
	if got := childNames(top.Children()); !slices.Equal(got, []string{"state"}) || top.Lookup("state").Type.Kind != compat.Ystring {
		t.Fatalf("filtered choice children = %v, want original state", got)
	}
}

func TestNativeProjectionExcludedSourceRetainsInstantiatedDefinitions(t *testing.T) {
	ctx := nativeProjectionContext(t, false, map[string]string{
		"library": `module library {
  yang-version 1.1; namespace "urn:library"; prefix l;
  typedef label { type string { length "1..10"; pattern "[a-z]+"; } default ready; }
  grouping fields { leaf grouped { type label; } }
  identity kind;
  identity member { base kind; }
}`,
		"consumer": `module consumer {
  yang-version 1.1; namespace "urn:consumer"; prefix c;
  import library { prefix l; }
  container top { leaf before { type string; } uses l:fields; leaf after { type l:label; } }
  leaf selected { type identityref { base l:kind; } }
}`,
	}, "consumer")
	roots, err := compat.FromContextWithOptions(ctx, compat.ContextProjectionOptions{ExcludedModules: []string{"library"}})
	if err != nil {
		t.Fatal(err)
	}
	if got := childNames(roots); !slices.Equal(got, []string{"consumer"}) {
		t.Fatalf("roots = %v, want consumer", got)
	}
	root := roots[0]
	top := root.Lookup("top")
	if got := childNames(top.Children()); !slices.Equal(got, []string{"before", "grouped", "after"}) {
		t.Fatalf("instantiated children = %v", got)
	}
	for _, name := range []string{"grouped", "after"} {
		entry := top.Lookup(name)
		if entry.Type == nil || entry.Type.Name != "label" || entry.Type.Kind != compat.Ystring || entry.Type.Length.String() != "1..10" || !slices.Equal(entry.Type.Pattern, []string{"[a-z]+"}) || !slices.Equal(entry.Default, []string{"ready"}) {
			t.Fatalf("%s lost imported type metadata: %+v", name, entry)
		}
	}
	grouped, ok := top.Lookup("grouped").NativeSchemaNode()
	if !ok || grouped.Module().Name() != "consumer" || grouped.SourceModule().Name() != "library" {
		t.Fatal("grouping source exclusion removed or changed the effective owner")
	}
	identity := root.Lookup("selected").Type.IdentityBase
	if identity == nil || identity.Name != "kind" || len(identity.Values) != 1 || identity.Values[0].Name != "member" {
		t.Fatalf("imported identity closure = %+v", identity)
	}
	if roots, err := compat.FromContext(ctx); err != nil || !slices.Equal(childNames(roots), []string{"consumer", "library"}) {
		t.Fatalf("unfiltered roots after exclusion = %v, %v", childNames(roots), err)
	}
}

func TestNativeProjectionExcludedSubtreeLeafrefsKeepNativeResolution(t *testing.T) {
	ctx := nativeProjectionContext(t, false, map[string]string{
		"base": `module base {
  yang-version 1.1; namespace "urn:base"; prefix b;
  container top { leaf value { type uint16; } }
}`,
		"extension": `module extension {
  yang-version 1.1; namespace "urn:extension"; prefix e;
  import base { prefix b; }
  leaf target { type uint16; }
  augment "/b:top" { container branch { leaf value { type uint16; } } }
}`,
		"consumer": `module consumer {
  yang-version 1.1; namespace "urn:consumer"; prefix c;
  import base { prefix b; }
  import extension { prefix e; }
  augment "/b:top/e:branch" { leaf descendant { type uint16; } }
  leaf live-ref { type leafref { path "/b:top/b:value"; } }
  leaf chain { type leafref { path "../live-ref"; } }
  leaf root-ref { type leafref { path "/e:target"; } }
  leaf node-ref { type leafref { path "/b:top/e:branch/e:value"; } }
  leaf descendant-ref { type leafref { path "/b:top/e:branch/c:descendant"; } }
}`,
	}, "consumer")
	roots, err := compat.FromContextWithOptions(ctx, compat.ContextProjectionOptions{ExcludedModules: []string{"extension"}})
	if err != nil {
		t.Fatal(err)
	}
	if got := childNames(roots); !slices.Equal(got, []string{"consumer", "base"}) {
		t.Fatalf("roots = %v, want consumer then base", got)
	}
	consumer := projectionRoot(t, roots, "consumer")
	top := projectionRoot(t, roots, "base").Lookup("top")
	if got := childNames(top.Children()); !slices.Equal(got, []string{"value"}) {
		t.Fatalf("children of filtered parent = %v, want value", got)
	}
	current := consumer.Lookup("chain")
	for _, want := range []*compat.Entry{consumer.Lookup("live-ref"), top.Lookup("value")} {
		target, err := current.ResolveLeafref()
		if err != nil || target != want {
			t.Fatalf("included leafref = %v, %v, want shared target %s", target, err, want.Path())
		}
		current = target
	}
	for _, name := range []string{"root-ref", "node-ref", "descendant-ref"} {
		entry := consumer.Lookup(name)
		node, ok := entry.NativeSchemaNode()
		if !ok {
			t.Fatalf("%s lost native schema handle", name)
		}
		native, err := cambium.ResolveLeafref(node)
		if err != nil || native.Realtype == nil || native.Realtype.Base() != cambium.BaseTypeUint16 {
			t.Fatalf("%s native resolution = %+v, %v", name, native, err)
		}
		if name == "descendant-ref" && native.Target.Module().Name() != "consumer" {
			t.Fatalf("descendant owner = %s, want included consumer", native.Target.Module().Name())
		}
		target, err := entry.ResolveLeafref()
		if target != nil || err == nil || !strings.Contains(err.Error(), "outside the FromContext projection") || !strings.Contains(err.Error(), native.Target.QualifiedPath()) {
			t.Fatalf("%s excluded target = %v, %v, want descriptive outside-projection error", name, target, err)
		}
	}
	if _, err := top.Lookup("value").ResolveLeafref(); err == nil {
		t.Fatal("non-leafref resolved successfully")
	} else if detail, ok := errors.AsType[*cambium.LeafrefResolutionError](err); !ok || detail.Reason != cambium.LeafrefFailureNotLeafref {
		t.Fatalf("native non-leafref error changed: %v", err)
	}
	full, err := compat.FromContext(ctx)
	if err != nil {
		t.Fatal(err)
	}
	fullConsumer := projectionRoot(t, full, "consumer")
	if _, err := fullConsumer.Lookup("descendant-ref").ResolveLeafref(); err != nil {
		t.Fatalf("fresh full projection lost excluded descendants: %v", err)
	}
}

func TestNativeProjectionOptionsRequireLiveFrozenContext(t *testing.T) {
	mutable, err := cambium.NewContext()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(mutable.Close)
	closed := nativeProjectionContext(t, false, nil)
	closed.Close()
	for _, ctx := range []*cambium.Context{nil, mutable, closed} {
		roots, err := compat.FromContextWithOptions(ctx, compat.ContextProjectionOptions{ExcludedModules: []string{"unused"}})
		if roots != nil || err == nil || !strings.Contains(err.Error(), "live frozen native context") {
			t.Fatalf("invalid context projection = %v, %v, want lifecycle error", roots, err)
		}
	}
}

func TestNativeProjectionOptionsPreserveUnresolvedLeafrefErrors(t *testing.T) {
	ctx := nativeProjectionContext(t, false, map[string]string{
		"library": `module library {
  yang-version 1.1; namespace "urn:library"; prefix l;
  leaf unresolved { type leafref { path "/l:missing"; } }
}`,
		"consumer": `module consumer {
  yang-version 1.1; namespace "urn:consumer"; prefix c;
  import library { prefix l; }
}`,
	}, "consumer")
	// An import-only module can expose an unresolved leafref without being
	// implemented. Projection options must preserve its native resolution error.
	roots, err := compat.FromContextWithOptions(ctx, compat.ContextProjectionOptions{ExcludedModules: []string{"consumer"}})
	if err != nil {
		t.Fatal(err)
	}
	library := projectionRoot(t, roots, "library")
	if module, ok := library.NativeModule(); !ok || module.IsImplemented() {
		t.Fatal("library should remain import-only")
	}
	entry := library.Lookup("unresolved")
	target, err := entry.ResolveLeafref()
	detail, ok := errors.AsType[*cambium.LeafrefResolutionError](err)
	if target != nil || !ok || detail.Reason != cambium.LeafrefFailureUnresolvedTarget || detail.Path != "/l:missing" {
		t.Fatalf("unresolved leafref = %v, %v, want native unresolved-target error", target, err)
	}
	if roots, err := compat.FromContextWithOptions(ctx, compat.ContextProjectionOptions{ExcludedModules: []string{"consumer", "library"}}); len(roots) != 0 || err != nil {
		t.Fatalf("excluding all roots = %v, %v, want an empty projection", roots, err)
	}
}
