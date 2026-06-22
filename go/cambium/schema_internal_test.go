// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

package cambium

import "testing"

// TestIfFeatureExpressionOperatorSubstrings verifies that single feature names
// containing operator substrings (e.g. "android", "notable", "orange") are
// still parsed as feature references, not as boolean operators.
func TestIfFeatureExpressionOperatorSubstrings(t *testing.T) {
	mod := &moduleData{
		ctx: &Context{enabledFeatures: map[string]map[string]struct{}{
			"demo": {"android": {}, "notable": {}, "orange": {}},
		}},
		name:       "demo",
		featureMap: make(map[string]*featureData),
	}
	for _, name := range []string{"android", "notable", "orange", "andromeda", "missing"} {
		mod.featureMap[name] = &featureData{name: name, module: mod}
	}
	cases := []struct {
		expr string
		want bool
	}{
		{"android", true},
		{"notable", true},
		{"orange", true},
		{"andromeda", false},
		{"notable and orange", true},
		{"android and missing", false},
		{"not missing", true},
		{"not(android)", false},
		{"android or missing", true},
	}

	for _, tc := range cases {
		got, ok := mod.evalIfFeatureExpr(tc.expr)
		if !ok {
			t.Fatalf("evalIfFeatureExpr(%q) returned parse failure", tc.expr)
		}
		if got != tc.want {
			t.Errorf("evalIfFeatureExpr(%q) = %v, want %v", tc.expr, got, tc.want)
		}
	}
}

func TestIfFeatureExpressionRejectsInvalidSyntax(t *testing.T) {
	mod := &moduleData{ctx: &Context{}, name: "demo"}
	for _, expr := range []string{"", "  ", "alpha and", "(alpha", "alpha)", "alpha beta", "alpha && beta"} {
		if got, ok := mod.evalIfFeatureExpr(expr); ok {
			t.Errorf("evalIfFeatureExpr(%q) = (%v,true), want parse failure", expr, got)
		}
	}
}

func TestReferencedPrefixesSkipsQuotesAxesAndDuplicates(t *testing.T) {
	got := referencedPrefixes(`/if:interfaces/if:interface[if:name = "ianaift:ethernetCsmacd"] and ancestor::node and ex:enabled`)
	want := []string{"if", "ex"}
	if len(got) != len(want) {
		t.Fatalf("referencedPrefixes len = %d (%v), want %d (%v)", len(got), got, len(want), want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("referencedPrefixes[%d] = %q, want %q (all: %v)", i, got[i], want[i], got)
		}
	}
}
