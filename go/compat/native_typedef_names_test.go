// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

package compat_test

import (
	"testing"

	"github.com/signalbreak-labs/cambium/go/compat"
)

func TestNativeProjectionUnionLeafrefTypedefNames(t *testing.T) {
	ctx := nativeProjectionContext(t, false, map[string]string{
		"aliases": `module aliases {
  yang-version 1.1; namespace "urn:aliases"; prefix a;
  typedef port-union { type union { type uint16; type string; } }
  typedef outer-union { type port-union; }
  typedef target-ref { type leafref { path "/a:target"; require-instance false; } }
  typedef outer-ref { type target-ref; }
  leaf target { type string; }
  leaf direct-union { type union { type uint16; type string; } }
  leaf aliased-union { type outer-union; }
  leaf-list union-list { type port-union; }
  leaf direct-ref { type leafref { path "/a:target"; } }
  leaf aliased-ref { type outer-ref; }
  leaf-list ref-list { type target-ref; }
  leaf nested { type union { type outer-union; type outer-ref; } }
}`,
	}, "aliases")
	roots, err := compat.FromContext(ctx)
	if err != nil {
		t.Fatal(err)
	}
	mod, _ := ctx.Schema("aliases")
	for _, root := range []*compat.Entry{projectionRoot(t, roots, "aliases"), compat.FromModule(mod)} {
		for _, tc := range []struct {
			leaf, name, base string
			kind             compat.TypeKind
		}{
			{"direct-union", "union", "", compat.Yunion},
			{"aliased-union", "outer-union", "port-union", compat.Yunion},
			{"union-list", "port-union", "union", compat.Yunion},
			{"direct-ref", "leafref", "", compat.Yleafref},
			{"aliased-ref", "outer-ref", "target-ref", compat.Yleafref},
			{"ref-list", "target-ref", "leafref", compat.Yleafref},
		} {
			typ := root.Lookup(tc.leaf).Type
			if typ.Name != tc.name || typ.Kind != tc.kind {
				t.Errorf("%s type = %s (%v), want %s (%v)", tc.leaf, typ.Name, typ.Kind, tc.name, tc.kind)
			}
			if tc.base != "" && (typ.Base == nil || typ.Base.Name != tc.base) {
				t.Errorf("%s base = %+v, want %s", tc.leaf, typ.Base, tc.base)
			}
		}
		members := root.Lookup("nested").Type.Type
		for i, want := range []string{"outer-union", "outer-ref"} {
			if members[i].Name != want {
				t.Errorf("member %d name = %q, want %q", i, members[i].Name, want)
			}
		}
		if typ := members[0]; typ.Kind != compat.Yunion || len(typ.Type) != 2 {
			t.Errorf("union member lost resolved types: %+v", typ)
		}
		if typ := members[1]; typ.Kind != compat.Yleafref || typ.Path != "/a:target" || !typ.OptionalInstance {
			t.Errorf("leafref member lost reference metadata: %+v", typ)
		}
	}
}
