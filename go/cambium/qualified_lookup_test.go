// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

package cambium_test

import (
	"testing"

	"github.com/signalbreak-labs/cambium/go/cambium"
)

func TestSchemaChildrenLookupQualifiedDisambiguatesAugments(t *testing.T) {
	const targetSource = `module qualified-target {
  namespace "urn:qualified-target";
  prefix qt;

  container top;
}`
	const leftSource = `module qualified-left {
  namespace "urn:qualified-left";
  prefix ql;

  import qualified-target { prefix qt; }

  augment "/qt:top" {
    leaf state { type string; }
  }
}`
	const rightSource = `module qualified-right {
  namespace "urn:qualified-right";
  prefix qr;

  import qualified-target { prefix qt; }

  augment "/qt:top" {
    leaf state { type boolean; }
  }
}`

	builder, err := cambium.NewContextBuilder(cambium.ContextFlags{})
	if err != nil {
		t.Fatalf("NewContextBuilder: %v", err)
	}
	for _, source := range []string{targetSource, leftSource, rightSource} {
		if err := builder.LoadModuleStr(source); err != nil {
			t.Fatalf("LoadModuleStr: %v", err)
		}
	}
	ctx, err := builder.Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	target, err := ctx.Schema("qualified-target")
	if err != nil {
		t.Fatalf("Schema(qualified-target): %v", err)
	}
	top, ok := target.TopLevel().Lookup("top")
	if !ok {
		t.Fatal("top not found")
	}

	children := top.Children()
	states := children.LookupAll("state")
	if states.Len() != 2 {
		t.Fatalf("LookupAll(state).Len() = %d, want 2", states.Len())
	}
	firstState, ok := states.Get(0)
	if !ok || firstState.Module().Name() != "qualified-left" {
		t.Fatalf("LookupAll(state)[0] = (%q,%v), want qualified-left", firstState.Module().Name(), ok)
	}
	secondState, ok := states.Get(1)
	if !ok || secondState.Module().Name() != "qualified-right" {
		t.Fatalf("LookupAll(state)[1] = (%q,%v), want qualified-right", secondState.Module().Name(), ok)
	}
	if states.QualifiedNames()[0] != (cambium.QualifiedName{Module: "qualified-left", Prefix: "ql", Namespace: "urn:qualified-left", Name: "state"}) {
		t.Fatalf("LookupAll(state).QualifiedNames()[0] = %#v", states.QualifiedNames()[0])
	}
	if children.LookupAll("missing").Len() != 0 {
		t.Fatal("LookupAll(missing) returned matches")
	}

	left, ok := children.LookupQualified("qualified-left", "state")
	if !ok {
		t.Fatal("LookupQualified(qualified-left, state) not found")
	}
	right, ok := children.LookupQualified("qualified-right", "state")
	if !ok {
		t.Fatal("LookupQualified(qualified-right, state) not found")
	}
	if left.Name() != "state" || right.Name() != "state" {
		t.Fatalf("qualified lookups = %q/%q, want state/state", left.Name(), right.Name())
	}
	if left.Module().Name() != "qualified-left" || right.Module().Name() != "qualified-right" {
		t.Fatalf("qualified lookup modules = %q/%q, want qualified-left/qualified-right", left.Module().Name(), right.Module().Name())
	}
	if left.QualifiedName() != (cambium.QualifiedName{Module: "qualified-left", Prefix: "ql", Namespace: "urn:qualified-left", Name: "state"}) {
		t.Fatalf("left QualifiedName = %#v", left.QualifiedName())
	}
	if right.QualifiedName() != (cambium.QualifiedName{Module: "qualified-right", Prefix: "qr", Namespace: "urn:qualified-right", Name: "state"}) {
		t.Fatalf("right QualifiedName = %#v", right.QualifiedName())
	}

	leftByQName, ok := children.LookupQName(left.QualifiedName())
	if !ok || leftByQName.Module().Name() != "qualified-left" {
		t.Fatalf("LookupQName(left) = (%#v,%v), want qualified-left", leftByQName, ok)
	}
	rightByNamespace, ok := children.LookupQName(cambium.QualifiedName{Namespace: "urn:qualified-right", Name: "state"})
	if !ok || rightByNamespace.Module().Name() != "qualified-right" {
		t.Fatalf("LookupQName(namespace right) = (%#v,%v), want qualified-right", rightByNamespace, ok)
	}
	if _, ok := children.LookupQualified("qualified-missing", "state"); ok {
		t.Fatal("LookupQualified(qualified-missing, state) succeeded")
	}

	if got, want := left.QualifiedPath(), "/qualified-target:top/qualified-left:state"; got != want {
		t.Fatalf("left QualifiedPath = %q, want %q", got, want)
	}
	if got, want := right.QualifiedPath(), "/qualified-target:top/qualified-right:state"; got != want {
		t.Fatalf("right QualifiedPath = %q, want %q", got, want)
	}
	if left.QualifiedPath() == right.QualifiedPath() {
		t.Fatalf("qualified paths collide: %q", left.QualifiedPath())
	}

	leftFromTarget, err := target.FindPath(left.QualifiedPath())
	if err != nil {
		t.Fatalf("target FindPath(left qualified path): %v", err)
	}
	if leftFromTarget.QualifiedName() != left.QualifiedName() {
		t.Fatalf("target FindPath(left qualified path) = %#v, want %#v", leftFromTarget.QualifiedName(), left.QualifiedName())
	}
	leftFromOwnModule, err := left.Module().FindPath(left.QualifiedPath())
	if err != nil {
		t.Fatalf("left module FindPath(left qualified path): %v", err)
	}
	if leftFromOwnModule.QualifiedName() != left.QualifiedName() {
		t.Fatalf("left module FindPath(left qualified path) = %#v, want %#v", leftFromOwnModule.QualifiedName(), left.QualifiedName())
	}
}
