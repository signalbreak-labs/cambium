// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

package cambium_test

import (
	"slices"
	"testing"

	"github.com/signalbreak-labs/cambium/go/cambium"
)

func TestEffectivePresenceAndUniqueExpressions(t *testing.T) {
	b, err := cambium.NewContextBuilder(cambium.ContextFlags{DisableSearchdirCwd: true})
	if err != nil {
		t.Fatal(err)
	}
	if err := b.LoadModuleStr(`module metadata {
  yang-version 1.1; namespace "urn:metadata"; prefix m;
  grouping fields {
    container selected {
      presence "declared presence";
      list item {
        key id;
        unique "detail/name   kind";
        unique obsolete;
        leaf id { type string; }
        container detail { leaf name { type string; } }
        leaf kind { type string; }
        leaf obsolete { type string; }
        leaf extra { type string; }
      }
    }
  }
  container top { uses fields { refine selected { presence "refined presence"; } } }
  container original { uses fields; }
  container empty { presence ""; }
  container plain;
  deviation "/m:top/m:selected/m:item" {
    deviate delete { unique obsolete; }
    deviate add { unique extra; }
  }
}`); err != nil {
		t.Fatal(err)
	}
	ctx, err := b.Build()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(ctx.Close)
	mod, err := ctx.Schema("metadata")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		path string
		want string
		ok   bool
	}{
		{"/m:top/m:selected", "refined presence", true},
		{"/m:original/m:selected", "declared presence", true},
		{"/m:empty", "", true},
		{"/m:plain", "", false},
	} {
		node := schemaNodeAt(t, mod, tc.path)
		if got, ok := node.Presence(); got != tc.want || ok != tc.ok || node.IsPresenceContainer() != tc.ok {
			t.Errorf("%s.Presence() = %q, %v, want %q, %v", tc.path, got, ok, tc.want, tc.ok)
		}
	}
	for _, tc := range []struct {
		path string
		want []string
	}{
		{"/m:top/m:selected/m:item", []string{"detail/name   kind", "extra"}},
		{"/m:original/m:selected/m:item", []string{"detail/name   kind", "obsolete"}},
	} {
		var got []string
		for _, unique := range schemaNodeAt(t, mod, tc.path).UniqueConstraints() {
			got = append(got, unique.Expression())
			if len(unique.Leafs()) == 0 {
				t.Errorf("%s: unique %q lost resolved leaves", tc.path, unique.Expression())
			}
		}
		if !slices.Equal(got, tc.want) {
			t.Errorf("%s unique expressions = %q, want %q", tc.path, got, tc.want)
		}
	}
	if got, ok := (cambium.SchemaNodeRef{}).Presence(); got != "" || ok {
		t.Errorf("zero node Presence() = %q, %v", got, ok)
	}
	if got := (cambium.UniqueConstraint{}).Expression(); got != "" {
		t.Errorf("zero unique Expression() = %q", got)
	}
}
