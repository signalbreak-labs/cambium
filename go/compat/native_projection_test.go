// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

package compat_test

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/signalbreak-labs/cambium/go/cambium"
	"github.com/signalbreak-labs/cambium/go/compat"
)

func nativeProjectionContext(t *testing.T, retain bool, sources map[string]string, requested ...string) *cambium.Context {
	t.Helper()
	dir := t.TempDir()
	for name, source := range sources {
		if err := os.WriteFile(filepath.Join(dir, name+".yang"), []byte(source), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	b, err := cambium.NewContextBuilder(cambium.ContextFlags{DisableSearchdirCwd: true})
	if err != nil {
		t.Fatal(err)
	}
	if err := b.SetFeaturePolicy(cambium.FeaturePolicy{RetainAll: retain}); err != nil {
		t.Fatal(err)
	}
	if err := b.SearchPath(dir); err != nil {
		t.Fatal(err)
	}
	for _, name := range requested {
		if err := b.LoadModule(name, nil, nil); err != nil {
			t.Fatal(err)
		}
	}
	ctx, err := b.Build()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(ctx.Close)
	return ctx
}

func projectionRoot(t *testing.T, roots []*compat.Entry, name string) *compat.Entry {
	t.Helper()
	for _, root := range roots {
		if root.Name == name {
			return root
		}
	}
	t.Fatalf("module %q missing from projections %v", name, childNames(roots))
	return nil
}

func TestNativeProjectionContextPreservesOwnershipAndLeafrefChain(t *testing.T) {
	ctx := nativeProjectionContext(t, false, map[string]string{
		"base": `module base {
  yang-version 1.1; namespace "urn:base"; prefix b;
  container top {
    leaf z { type string; }
    leaf mid { type leafref { path "../z"; } }
  }
}`,
		"library": `module library {
  yang-version 1.1; namespace "urn:library"; prefix l;
  grouping fields { leaf grouped { type string; } }
}`,
		"consumer": `module consumer {
  yang-version 1.1; namespace "urn:consumer"; prefix c;
  import base { prefix b; }
  import library { prefix l; }
  container local { uses l:fields; choice mode { leaf shorthand { type string; } } }
  augment "/b:top" {
    leaf ref { type leafref { path "../b:mid"; } }
    leaf chained { type leafref { path "../c:ref"; } }
  }
  leaf external { type leafref { path "/b:top/c:chained"; } }
}`,
	}, "consumer")
	roots, err := compat.FromContext(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got := childNames(roots); !slices.Equal(got, []string{"consumer", "base", "library"}) {
		t.Fatalf("projected modules = %v, want requested module then all imports once", got)
	}
	base := projectionRoot(t, roots, "base")
	consumer := projectionRoot(t, roots, "consumer")
	library := projectionRoot(t, roots, "library")
	if module, ok := library.NativeModule(); !ok || module.IsImplemented() {
		t.Fatalf("import-only native module = %v, %v; want unimplemented library", module, ok)
	}
	if _, ok := base.NativeSchemaNode(); ok {
		t.Fatal("synthetic module root has a schema node")
	}
	baseModule, err := ctx.Schema("base")
	if err != nil {
		t.Fatal(err)
	}
	if module, ok := base.NativeModule(); !ok || module != baseModule {
		t.Fatal("module projection lost native module identity")
	}
	top := base.Lookup("top")
	if got := childNames(top.Children()); !slices.Equal(got, []string{"z", "mid", "ref", "chained"}) {
		t.Fatalf("augmented children = %v", got)
	}
	ref := top.Lookup("ref")
	native, ok := ref.NativeSchemaNode()
	if !ok || native.Module().Name() != "consumer" || native.SourceModule().Name() != "consumer" {
		t.Fatal("augmented node lost its native defining/source module")
	}
	if module, ok := ref.NativeModule(); !ok || module != native.Module() {
		t.Fatal("augmented entry module does not match native owner")
	}
	grouped, ok := consumer.Lookup("local").Lookup("grouped").NativeSchemaNode()
	if !ok || grouped.SourceModule().Name() != "library" || grouped.InstantiatingModule().Name() != "consumer" {
		t.Fatal("grouping projection lost source or instantiating module")
	}
	implicit, ok := consumer.Lookup("local").Lookup("mode").Lookup("shorthand").NativeSchemaNode()
	if !ok || !implicit.IsCase() || implicit.Statement().IsValid() {
		t.Fatal("native implicit case must retain its schema handle without inventing a source statement")
	}
	current := consumer.Lookup("external")
	for _, want := range []*compat.Entry{top.Lookup("chained"), ref, top.Lookup("mid"), top.Lookup("z")} {
		target, err := current.ResolveLeafref()
		if err != nil || target != want {
			t.Fatalf("%s.ResolveLeafref() = %p, %v; want existing projected %s (%p)", current.Path(), target, err, want.Path(), want)
		}
		current = target
	}
	if _, err := current.ResolveLeafref(); err == nil {
		t.Fatal("non-leafref resolution succeeded")
	} else if detail, ok := errors.AsType[*cambium.LeafrefResolutionError](err); !ok || detail.Reason != cambium.LeafrefFailureNotLeafref {
		t.Fatalf("non-leafref error = %v, want native structured error", err)
	}
	standalone := compat.FromModule(baseModule).Lookup("top").Lookup("mid")
	if _, err := standalone.ResolveLeafref(); err == nil || !strings.Contains(err.Error(), "FromContext") {
		t.Fatalf("standalone leafref error = %v, want explicit context requirement", err)
	}
}

func TestNativeProjectionNilAndSyntheticHandles(t *testing.T) {
	for _, entry := range []*compat.Entry{nil, {}, {Name: "synthetic"}, compat.FromModule(cambium.Module{})} {
		if node, ok := entry.NativeSchemaNode(); ok || node != (cambium.SchemaNodeRef{}) {
			t.Fatalf("NativeSchemaNode() = %v, %v for synthetic entry", node, ok)
		}
		if module, ok := entry.NativeModule(); ok || module != (cambium.Module{}) {
			t.Fatalf("NativeModule() = %v, %v for synthetic entry", module, ok)
		}
		if target, err := entry.ResolveLeafref(); target != nil || err == nil {
			t.Fatalf("synthetic ResolveLeafref() = %v, %v, want nil and error", target, err)
		}
	}
	if roots, err := compat.FromContext(nil); roots != nil || err == nil {
		t.Fatalf("FromContext(nil) = %v, %v, want nil and error", roots, err)
	}
}

func TestNativeProjectionRequiresLiveFrozenContext(t *testing.T) {
	mutable, err := cambium.NewContext()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(mutable.Close)
	if roots, err := compat.FromContext(mutable); roots != nil || err == nil {
		t.Fatalf("mutable context projection = %v, %v, want rejection without compilation", roots, err)
	}
	ctx := nativeProjectionContext(t, false, nil)
	if !ctx.IsFrozen() {
		t.Fatal("built context is not frozen")
	}
	ctx.Close()
	if ctx.IsFrozen() || mutable.IsFrozen() || (*cambium.Context)(nil).IsFrozen() {
		t.Fatal("nil, closed, or mutable context reports a live frozen state")
	}
	if roots, err := compat.FromContext(ctx); roots != nil || err == nil {
		t.Fatalf("closed context projection = %v, %v, want rejection", roots, err)
	}
}

func TestNativeProjectionRejectsLocalNameCollisions(t *testing.T) {
	ctx := nativeProjectionContext(t, false, map[string]string{
		"base": `module base { yang-version 1.1; namespace "urn:base"; prefix b;
  container top { leaf state { type string; } }
}`,
		"augmenter": `module augmenter { yang-version 1.1; namespace "urn:augmenter"; prefix a;
  import base { prefix b; }
  augment "/b:top" { leaf state { type string; } }
}`,
	}, "augmenter")
	roots, err := compat.FromContext(ctx)
	if roots != nil || err == nil {
		t.Fatalf("FromContext with colliding names = %v, %v, want no lossy projection", roots, err)
	}
	for _, detail := range []string{"state", "base", "augmenter", "top"} {
		if !strings.Contains(err.Error(), detail) {
			t.Fatalf("collision error %q does not identify %q", err, detail)
		}
	}
}

func TestNativeProjectionRejectsFlattenedChoiceCollisions(t *testing.T) {
	ctx := nativeProjectionContext(t, false, map[string]string{
		"base": `module base { yang-version 1.1; namespace "urn:base"; prefix b;
  container top { choice selection { case original { leaf state { type string; } } } }
}`,
		"augmenter": `module augmenter { yang-version 1.1; namespace "urn:augmenter"; prefix a;
  import base { prefix b; }
  augment "/b:top" { leaf state { type string; } }
}`,
	}, "augmenter")
	if roots, err := compat.FromContext(ctx); roots != nil || err == nil {
		t.Fatalf("FromContext with flattened data-name collision = %v, %v, want no lossy projection", roots, err)
	}
}

func TestNativeProjectionEffectiveConstraintMetadata(t *testing.T) {
	ctx := nativeProjectionContext(t, false, map[string]string{
		"constraints": `module constraints {
  yang-version 1.1; namespace "urn:constraints"; prefix c;
  extension note { argument text; }
  grouping fields {
    container selected {
      presence "original presence";
      must "true()";
      c:note "grouped extension";
      list item {
        key id; unique "details/name   kind"; unique obsolete;
        leaf id { type string; }
        container details { leaf name { type string; } }
        leaf kind { type string; }
        leaf obsolete { type string; }
        leaf extra { type string; }
      }
    }
  }
  container top {
    leaf enabled { type boolean; }
    uses fields {
      when "enabled = 'true'";
      refine selected {
        presence "refined presence";
        must "count(item) > 0" {
          description "At least one item"; reference "refined rule";
          error-message "An item is required"; error-app-tag "missing-item";
        }
      }
    }
  }
  container empty { presence ""; }
  container plain;
  augment "/c:top/c:selected" {
    when "../enabled = 'true'";
    leaf augmented { type string; c:note "augmented extension"; must ". != ''"; }
  }
  deviation "/c:top/c:selected" {
    deviate delete { must "true()"; }
    deviate add { must "count(item) < 4" {
      description "At most three items"; reference "deviated rule";
      error-message "Too many items"; error-app-tag "excess-items";
    } }
  }
  deviation "/c:top/c:selected/c:item" {
    deviate delete { unique obsolete; }
    deviate add { unique extra; }
  }
}`,
	}, "constraints")
	roots, err := compat.FromContext(ctx)
	if err != nil {
		t.Fatal(err)
	}
	module, err := ctx.Schema("constraints")
	if err != nil {
		t.Fatal(err)
	}
	for _, root := range []*compat.Entry{projectionRoot(t, roots, "constraints"), compat.FromModule(module)} {
		selected := root.Lookup("top").Lookup("selected")
		if got := nativeExtraValues(t, selected, "presence"); !slices.Equal(got, []string{"refined presence"}) {
			t.Errorf("effective presence = %q", got)
		}
		if got := nativeExtraValues(t, root.Lookup("empty"), "presence"); !slices.Equal(got, []string{""}) {
			t.Errorf("empty presence = %q, want present empty string", got)
		}
		if got := nativeExtraValues(t, root.Lookup("plain"), "presence"); len(got) != 0 {
			t.Errorf("plain container presence = %q", got)
		}
		if got := nativeExtraValues(t, selected.Lookup("item"), "unique"); !slices.Equal(got, []string{"details/name   kind", "extra"}) {
			t.Errorf("effective unique expressions = %q", got)
		}
		wantMusts := []struct{ expression, description, reference, message, tag string }{
			{"count(item) > 0", "At least one item", "refined rule", "An item is required", "missing-item"},
			{"count(item) < 4", "At most three items", "deviated rule", "Too many items", "excess-items"},
		}
		if got := len(selected.Extra["must"]); got != len(wantMusts) {
			t.Errorf("effective must count = %d, want %d", got, len(wantMusts))
		} else {
			for i, want := range wantMusts {
				must, ok := selected.Extra["must"][i].(*compat.Must)
				if !ok || must == nil {
					t.Fatalf("must %d = %T, want *compat.Must", i, selected.Extra["must"][i])
				}
				if must.Name != want.expression || valueName(must.Description) != want.description || valueName(must.Reference) != want.reference || valueName(must.ErrorMessage) != want.message || valueName(must.ErrorAppTag) != want.tag {
					t.Errorf("effective must %d = %+v, want %+v", i, must, want)
				}
			}
		}
		if when, ok := selected.GetWhenXPath(); !ok || when != "enabled = 'true'" {
			t.Errorf("grouping effective when = %q, %v", when, ok)
		}
		augmented := selected.Lookup("augmented")
		if when, ok := augmented.GetWhenXPath(); !ok || when != "../enabled = 'true'" {
			t.Errorf("augment effective when = %q, %v", when, ok)
		}
		if got := len(augmented.Extra["must"]); got != 1 {
			t.Errorf("augmented must count = %d, want 1", got)
		}
		for _, entry := range []*compat.Entry{selected, augmented} {
			if len(entry.Exts) != 1 || entry.Exts[0].Keyword != "constraints:note" {
				t.Errorf("%s lost native extension metadata: %v", entry.Path(), entry.Exts)
			}
		}
		if got := childNames(selected.Children()); !slices.Equal(got, []string{"item", "augmented"}) {
			t.Errorf("effective children = %v", got)
		}
	}
}

func nativeExtraValues(t *testing.T, entry *compat.Entry, keyword string) []string {
	t.Helper()
	var names []string
	for _, extra := range entry.Extra[keyword] {
		value, ok := extra.(*compat.Value)
		if !ok || value == nil {
			t.Fatalf("%s Extra[%q] = %T, want *compat.Value", entry.Path(), keyword, extra)
		}
		names = append(names, value.Name)
	}
	return names
}

func TestNativeProjectionRetainsFeaturesAndTransitiveIdentityValues(t *testing.T) {
	for _, retain := range []bool{false, true} {
		ctx := nativeProjectionContext(t, retain, map[string]string{
			"identities": `module identities {
  yang-version 1.1; namespace "urn:identities"; prefix i;
  feature optional;
  identity root;
  identity zebra { base root; }
  identity alpha { base zebra; if-feature optional; }
  leaf chosen { type identityref { base root; } }
  container top {
    leaf positive { if-feature optional; type string; }
    leaf negative { if-feature "not optional"; type string; }
    leaf enums { type enumeration {
      enum zulu { if-feature optional; }
      enum alpha { if-feature "not optional"; }
    } }
    leaf bits { type bits {
      bit zulu { if-feature optional; }
      bit alpha { if-feature "not optional"; }
    } }
  }
}`,
			"derived": `module derived {
  yang-version 1.1; namespace "urn:derived"; prefix d;
  import identities { prefix i; }
  identity middle { base i:zebra; }
  identity aardvark { base middle; }
}`,
		}, "identities", "derived")
		roots, err := compat.FromContext(ctx)
		if err != nil {
			t.Fatal(err)
		}
		root := projectionRoot(t, roots, "identities")
		wantChildren := []string{"negative", "enums", "bits"}
		wantValues := []string{"aardvark", "middle", "zebra"}
		wantNames := []string{"alpha"}
		if retain {
			wantChildren = []string{"positive", "negative", "enums", "bits"}
			wantValues = []string{"aardvark", "alpha", "middle", "zebra"}
			wantNames = []string{"alpha", "zulu"}
		}
		top := root.Lookup("top")
		if got := childNames(top.Children()); !slices.Equal(got, wantChildren) {
			t.Errorf("retain=%v: children = %v, want %v", retain, got, wantChildren)
		}
		for _, values := range [][]string{top.Lookup("enums").Type.Enum.Names(), top.Lookup("bits").Type.Bit.Names()} {
			if !slices.Equal(values, wantNames) {
				t.Errorf("retain=%v: enum/bit Names = %v, want %v", retain, values, wantNames)
			}
		}
		base := root.Lookup("chosen").Type.IdentityBase
		if got := identityNames(base.Values); !slices.Equal(got, wantValues) {
			t.Errorf("retain=%v: identity Values = %v, want lexical transitive %v", retain, got, wantValues)
		}
		if derived := base.GetValue("aardvark"); derived == nil || derived.PrefixedName() != "d:aardvark" {
			t.Error("transitive identity lost defining module")
		}
		if retain {
			node, ok := top.Lookup("positive").NativeSchemaNode()
			if !ok || !slices.Equal(node.IfFeatures(), []string{"optional"}) {
				t.Error("retained declaration lost native feature metadata")
			}
		}
	}
}
