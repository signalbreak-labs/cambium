// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

package compat

import (
	"reflect"
	"testing"
)

func TestAugmentMatchesGoyangAndMaintainsOrder(t *testing.T) {
	const source = `module compat-augment-apply {
  yang-version 1.1;
  namespace "urn:compat-augment-apply";
  prefix caa;

  container top {
    leaf before { type string; }
  }

  augment "/caa:top" {
    leaf added { type string; }
    container nested {
      leaf child { type string; }
    }
  }
}
`
	compatRoot, upstreamRoot := rawCompatAndUpstreamEntries(t, "compat-augment-apply", source)

	compatProcessed, compatSkipped := compatRoot.Augment(false)
	upstreamProcessed, upstreamSkipped := upstreamRoot.Augment(false)
	if compatProcessed != upstreamProcessed || compatSkipped != upstreamSkipped {
		t.Fatalf("Augment(false) = (%d,%d), want goyang (%d,%d)", compatProcessed, compatSkipped, upstreamProcessed, upstreamSkipped)
	}
	compatTop := compatRoot.Find("/caa:top")
	upstreamTop := upstreamRoot.Find("/caa:top")
	if compatTop == nil || upstreamTop == nil {
		t.Fatalf("fixture top entries = (%#v,%#v), want both non-nil", compatTop, upstreamTop)
	}
	for _, name := range []string{"before", "added", "nested"} {
		if (compatTop.Lookup(name) == nil) != (upstreamTop.Dir[name] == nil) {
			t.Fatalf("child %q present = %v, want goyang %v", name, compatTop.Lookup(name) != nil, upstreamTop.Dir[name] != nil)
		}
	}
	if len(compatRoot.Augments) != len(upstreamRoot.Augments) {
		t.Fatalf("remaining augments = %d, want goyang %d", len(compatRoot.Augments), len(upstreamRoot.Augments))
	}
	if len(compatTop.Augmented) != len(upstreamTop.Augmented) {
		t.Fatalf("Augmented len = %d, want goyang %d", len(compatTop.Augmented), len(upstreamTop.Augmented))
	}
	if len(compatTop.AugmentedBy) != len(compatTop.Augmented) {
		t.Fatalf("AugmentedBy len = %d, want Augmented len %d", len(compatTop.AugmentedBy), len(compatTop.Augmented))
	}
	if len(compatTop.AugmentedBy) > 0 && compatTop.AugmentedBy[0].Name != compatTop.Augmented[0].Name {
		t.Fatalf("AugmentedBy[0] = %q, want Augmented[0] %q", compatTop.AugmentedBy[0].Name, compatTop.Augmented[0].Name)
	}
	var orderedNames []string
	for _, child := range compatTop.Children() {
		orderedNames = append(orderedNames, child.Name)
	}
	if !reflect.DeepEqual(orderedNames, []string{"before", "added", "nested"}) {
		t.Fatalf("ordered children after augment = %v, want [before added nested]", orderedNames)
	}
}
