// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

package compat_test

import (
	"slices"
	"testing"

	"github.com/signalbreak-labs/cambium/go/compat"
)

func TestNativeProjectionDoesNotRestoreInapplicableTypeDefaults(t *testing.T) {
	ctx := nativeProjectionContext(t, false, map[string]string{
		"defaults": `module defaults {
  yang-version 1.1; namespace "urn:defaults"; prefix d;
  typedef label { type string; default fallback; }
  leaf required { type label; mandatory true; }
  leaf-list required-list { type label; min-elements 1; }
  list rows { key "inherited explicit";
    leaf inherited { type label; }
    leaf explicit { type label; default explicit; mandatory true; }
  }
  leaf optional { type label; }
}`,
		"legacy": `module legacy {
  namespace "urn:legacy"; prefix l;
  typedef label { type string; default fallback; }
  leaf-list values { type label; }
}`,
	}, "defaults", "legacy")
	roots, err := compat.FromContext(ctx)
	if err != nil {
		t.Fatal(err)
	}
	mod, _ := ctx.Schema("defaults")
	for _, root := range []*compat.Entry{projectionRoot(t, roots, "defaults"), compat.FromModule(mod)} {
		for _, entry := range []*compat.Entry{
			root.Lookup("required"), root.Lookup("required-list"),
			root.Lookup("rows").Lookup("inherited"), root.Lookup("rows").Lookup("explicit"),
		} {
			assertNoEffectiveDefault(t, entry)
		}
		if got := root.Lookup("optional").DefaultValues(); !slices.Equal(got, []string{"fallback"}) {
			t.Errorf("optional default lost: %v", got)
		}
	}
	assertNoEffectiveDefault(t, projectionRoot(t, roots, "legacy").Lookup("values"))
	ast := &compat.Entry{Kind: compat.LeafEntry, Node: &compat.Leaf{Name: "ast"}, Type: &compat.YangType{HasDefault: true, Default: "fallback"}}
	if got := ast.DefaultValues(); !slices.Equal(got, []string{"fallback"}) {
		t.Errorf("AST-only type-default fallback changed: %v", got)
	}
}

func assertNoEffectiveDefault(t *testing.T, entry *compat.Entry) {
	t.Helper()
	if len(entry.Default) != 0 || len(entry.DefaultValues()) != 0 {
		t.Errorf("%s restored inapplicable default: field=%v values=%v", entry.Path(), entry.Default, entry.DefaultValues())
	}
	if value, ok := entry.SingleDefaultValue(); ok {
		t.Errorf("%s has inapplicable single default %q", entry.Path(), value)
	}
	if !entry.Type.HasDefault || entry.Type.Default != "fallback" {
		t.Errorf("%s lost type-default provenance: %+v", entry.Path(), entry.Type)
	}
}
