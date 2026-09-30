// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

package compat_test

import (
	"testing"

	"github.com/signalbreak-labs/cambium/go/cambium"
	"github.com/signalbreak-labs/cambium/go/compat"
)

func TestNativeProjectionForwardLeafrefRealtypeAcrossModules(t *testing.T) {
	ctx := nativeProjectionContext(t, false, map[string]string{
		"source": `module source {
  yang-version 1.1; namespace "urn:source"; prefix s;
  import target { prefix t; }
  leaf ref { type leafref { path "/t:middle"; } }
}`,
		"target": `module target {
  yang-version 1.1; namespace "urn:target"; prefix t;
  identity first; identity second;
  identity shared { base first; base second; }
  leaf middle { type leafref { path "../value"; } }
  leaf value { type identityref { base first; base second; } }
}`,
	}, "source")
	roots, err := compat.FromContext(ctx)
	if err != nil {
		t.Fatal(err)
	}
	source := projectionRoot(t, roots, "source")
	target := projectionRoot(t, roots, "target")
	current := source.Lookup("ref")
	for _, want := range []*compat.Entry{target.Lookup("middle"), target.Lookup("value")} {
		resolved, err := current.ResolveLeafref()
		if err != nil || resolved != want {
			t.Fatalf("projected leafref = %v, %v, want %v", resolved, err, want)
		}
		current = resolved
	}
	if bases := current.Type.IdentityBases; len(bases) != 2 || bases[0].Name != "first" || bases[1].Name != "second" {
		t.Fatalf("projected identity bases = %v, want first, second", bases)
	}
	native, ok := source.Lookup("ref").NativeSchemaNode()
	if !ok {
		t.Fatal("projection lost native source node")
	}
	info, ok := native.LeafType()
	if !ok {
		t.Fatal("projection lost native source type")
	}
	for _, want := range []string{"/target/middle", "/target/value"} {
		ref, ok := info.Resolved().(cambium.ResolvedLeafRef)
		if !ok {
			t.Fatalf("native projected type = %T, want leafref", info.Resolved())
		}
		if node, ok := ref.Target(); !ok || node.Path() != want {
			t.Fatalf("native target = %q, %v, want %s", node.Path(), ok, want)
		}
		realtype, ok := ref.Realtype()
		if !ok || realtype == nil {
			t.Fatalf("native projected leafref to %s lost Realtype", want)
		}
		info = *realtype
	}
	identity, ok := info.Resolved().(cambium.ResolvedIdentityRef)
	if !ok || len(identity.Bases()) != 2 || identity.Bases()[0].Name() != "first" || identity.Bases()[1].Name() != "second" {
		t.Fatalf("native projected terminal type = %+v, want both identity bases", info.Resolved())
	}
}
