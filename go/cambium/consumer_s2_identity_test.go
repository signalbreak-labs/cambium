// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

package cambium_test

import (
	"reflect"
	"testing"

	"github.com/signalbreak-labs/cambium/go/cambium"
)

// S2: qualified identity and source context.

const s2Base = `module s2-base {
  yang-version 1.1; namespace "urn:s2:base"; prefix b;
  import s2-lib { prefix lib; }
  identity base-kind;
  typedef ref-to-own { type leafref { path "/b:top/b:own"; } }
  container top {
    leaf own { type string; }
    uses lib:shared;
    leaf own-ref { type ref-to-own; }
    leaf kind { type identityref { base lib:root-kind; } }
  }
}`

const s2Lib = `module s2-lib {
  yang-version 1.1; namespace "urn:s2:lib"; prefix lib;
  extension label { argument text; }
  identity root-kind;
  identity lib-child { base root-kind; }
  grouping shared {
    leaf from-lib { type string; lib:label "shared-leaf"; }
  }
}`

const s2Left = `module s2-left {
  yang-version 1.1; namespace "urn:s2:left"; prefix l;
  import s2-base { prefix b; }
  import s2-lib { prefix lib; }
  identity left-kind { base lib:root-kind; }
  identity left-grandchild { base left-kind; }
  augment "/b:top" {
    leaf status { type string; }
  }
}`

const s2Right = `module s2-right {
  yang-version 1.1; namespace "urn:s2:right"; prefix r;
  import s2-base { prefix b; }
  augment "/b:top" {
    leaf status { type uint8; }
  }
}`

func TestS2QualifiedIdentityAcrossAugments(t *testing.T) {
	ctx, err := buildPolicyContext(t, cambium.DeviationPolicy{}, s2Lib, s2Base, s2Left, s2Right)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	mod, _ := ctx.Schema("s2-base")
	top := schemaNodeAt(t, mod, "/b:top")
	if got, want := names(top.Children()), "own,from-lib,own-ref,kind,status,status"; got != want {
		t.Fatalf("top children = %s, want %s", got, want)
	}
	all := top.Children().LookupAll("status")
	if all.Len() != 2 {
		t.Fatalf("LookupAll(status) = %d", all.Len())
	}
	// Local lookup returns the first match in schema order, which is
	// deterministic; qualified lookup is the unambiguous form.
	first, _ := top.Children().Lookup("status")
	if first.QualifiedName().Module != "s2-left" {
		t.Fatalf("Lookup(status) module = %q, want s2-left", first.QualifiedName().Module)
	}
	left, ok := top.Children().LookupQualified("s2-left", "status")
	if !ok {
		t.Fatal("LookupQualified(s2-left,status) failed")
	}
	right, ok := top.Children().LookupQualified("s2-right", "status")
	if !ok {
		t.Fatal("LookupQualified(s2-right,status) failed")
	}
	if lt, _ := left.LeafType(); lt.Base() != cambium.BaseTypeString {
		t.Fatalf("left status base = %v", lt.Base())
	}
	if rt, _ := right.LeafType(); rt.Base() != cambium.BaseTypeUint8 {
		t.Fatalf("right status base = %v", rt.Base())
	}
	if right2, ok := top.Children().LookupQualifiedName(cambium.QualifiedName{Namespace: "urn:s2:right", Name: "status"}); !ok || right2.Namespace() != "urn:s2:right" {
		t.Fatal("namespace-qualified lookup failed")
	}

	// Path forms.
	pathCases := []struct{ got, want string }{
		{left.LocalPath(), "/top/status"},
		{left.QualifiedPath(), "/s2-base:top/s2-left:status"},
		{left.NamespaceQualifiedPath(), "/{urn:s2:base}top/{urn:s2:left}status"},
		{right.NamespaceQualifiedPath(), "/{urn:s2:base}top/{urn:s2:right}status"},
	}
	for _, tc := range pathCases {
		if tc.got != tc.want {
			t.Errorf("path = %q, want %q", tc.got, tc.want)
		}
	}
	// Caller-supplied paths are outside any YANG source context, so FindPath
	// accepts the module name or prefix of any loaded module for each step.
	for _, path := range []string{"/b:top/r:status", "/s2-base:top/s2-right:status"} {
		found, err := mod.FindPath(path)
		if err != nil || found.QualifiedPath() != right.QualifiedPath() {
			t.Fatalf("FindPath(%s) = %s, %v", path, found.QualifiedPath(), err)
		}
	}

	// Provenance: augmenting module defines the node in its own namespace;
	// the instantiation happens in the base tree.
	if left.Module().Name() != "s2-left" || left.Namespace() != "urn:s2:left" {
		t.Fatalf("left defining module = %q ns %q", left.Module().Name(), left.Namespace())
	}
	if got := left.InstantiatingModule().Name(); got != "s2-base" {
		t.Fatalf("left instantiating module = %q, want s2-base", got)
	}
	if got := names(cambium.SchemaChildren{}); got != "" {
		t.Fatal("empty children should have no names")
	}
	var ancestors []string
	for _, a := range left.DataAncestors() {
		ancestors = append(ancestors, a.Name())
	}
	if !reflect.DeepEqual(ancestors, []string{"top"}) {
		t.Fatalf("data ancestors = %v", ancestors)
	}
}

func TestS2ImportedGroupingSourceContext(t *testing.T) {
	ctx, err := buildPolicyContext(t, cambium.DeviationPolicy{}, s2Lib, s2Base, s2Left, s2Right)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	mod, _ := ctx.Schema("s2-base")
	fromLib := schemaNodeAt(t, mod, "/b:top/from-lib")
	// A node instantiated from an imported grouping takes the namespace of
	// the module where the uses appears; the source module is the grouping's.
	if fromLib.Namespace() != "urn:s2:base" || fromLib.Module().Name() != "s2-base" {
		t.Fatalf("from-lib effective module = %q ns %q", fromLib.Module().Name(), fromLib.Namespace())
	}
	if got := fromLib.SourceModule().Name(); got != "s2-lib" {
		t.Fatalf("from-lib source module = %q, want s2-lib", got)
	}
	if origin, ok := fromLib.GroupingOrigin(); !ok || origin != "shared" {
		t.Fatalf("grouping origin = %q,%v", origin, ok)
	}
	if loc := fromLib.SourceLocation(); loc.Line != 7 {
		t.Fatalf("from-lib source line = %d, want 7 in s2-lib (%+v)", loc.Line, loc)
	}
	// The extension prefix is interpreted in the grouping's module context.
	ext, ok := fromLib.Extension("label")
	if !ok || ext.ModuleName() != "s2-lib" {
		t.Fatalf("extension = %+v,%v", ext, ok)
	}
	if arg, ok := ext.Argument(); !ok || arg != "shared-leaf" {
		t.Fatalf("extension argument = %q,%v", arg, ok)
	}
	if got := fromLib.MatchingExtensions("s2-lib", "label"); len(got) != 1 {
		t.Fatalf("MatchingExtensions = %d", len(got))
	}

	// Typedef-defined leafref resolves in the typedef's module context.
	ownRef := schemaNodeAt(t, mod, "/b:top/own-ref")
	typ, _ := ownRef.LeafType()
	if name, ok := typ.TypedefName(); !ok || name != "ref-to-own" || typ.Base() != cambium.BaseTypeLeafRef {
		t.Fatalf("own-ref type = %v %q", typ.Base(), name)
	}
	res, err := cambium.ResolveLeafref(ownRef)
	if err != nil {
		t.Fatalf("ResolveLeafref: %v", err)
	}
	if res.Target.QualifiedPath() != "/s2-base:top/s2-base:own" {
		t.Fatalf("leafref target = %s", res.Target.QualifiedPath())
	}
}

func TestS2IdentityHierarchyAcrossModules(t *testing.T) {
	ctx, err := buildPolicyContext(t, cambium.DeviationPolicy{}, s2Lib, s2Base, s2Left, s2Right)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	lib, _ := ctx.Schema("s2-lib")
	root, ok := lib.Identity("root-kind")
	if !ok {
		t.Fatal("root-kind missing")
	}
	var closure []string
	for _, id := range root.DerivedClosure() {
		closure = append(closure, id.Module().Name()+":"+id.Name())
	}
	want := []string{"s2-lib:lib-child", "s2-left:left-kind", "s2-left:left-grandchild"}
	if !reflect.DeepEqual(closure, want) {
		t.Fatalf("derived closure = %v, want %v", closure, want)
	}
	base, _ := ctx.Schema("s2-base")
	kind := schemaNodeAt(t, base, "/b:top/kind")
	typ, _ := kind.LeafType()
	ref, ok := typ.Resolved().(cambium.ResolvedIdentityRef)
	if !ok || len(ref.Bases()) != 1 || ref.Bases()[0].Name() != "root-kind" || ref.Bases()[0].Module().Name() != "s2-lib" {
		t.Fatalf("identityref bases = %#v", typ.Resolved())
	}
}
