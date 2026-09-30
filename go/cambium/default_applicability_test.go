// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

package cambium_test

import (
	"strings"
	"testing"

	"github.com/signalbreak-labs/cambium/go/cambium"
)

func TestInapplicableDefaultsDoNotBecomeEffective(t *testing.T) {
	// RFC 7950 sections 7.6.1, 7.7.2 and 7.8.2 distinguish a valid default
	// declaration from a default that is applicable to a particular node.
	for _, tc := range []struct {
		name, body, path string
		mandatory        bool
	}{
		{"mandatory", `leaf value { type label; mandatory true; }`, "/d:value", true},
		{"key-inherited", `list rows { key value; leaf value { type label; } }`, "/d:rows/value", false},
		{"key-explicit", `list rows { key value; leaf value { type label; default explicit; mandatory true; } }`, "/d:rows/value", false},
		{"leaf-list", `leaf-list value { type label; min-elements 1; }`, "/d:value", false},
		{"refined-mandatory", `grouping g { leaf value { type label; } } uses g { refine value { mandatory true; } }`, "/d:value", true},
		{"deviated-min", `leaf-list value { type label; } deviation "/d:value" { deviate add { min-elements 1; } }`, "/d:value", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, err := buildPolicyContext(t, cambium.DeviationPolicy{}, `module defaults {
  yang-version 1.1; namespace "urn:defaults"; prefix d;
  typedef label { type string; default fallback; }
  leaf optional { type label; }
  leaf-list optional-list { type label; }
`+tc.body+`}`)
			if err != nil {
				t.Fatalf("valid inapplicable default rejected: %v", err)
			}
			mod, _ := ctx.Schema("defaults")
			node := schemaNodeAt(t, mod, tc.path)
			if got := node.DefaultValues(); len(got) != 0 {
				t.Errorf("inapplicable effective defaults = %v", got)
			}
			if node.IsMandatory() != tc.mandatory {
				t.Errorf("mandatory = %v, want %v", node.IsMandatory(), tc.mandatory)
			}
			for _, path := range []string{"/d:optional", "/d:optional-list"} {
				if got, ok := schemaNodeAt(t, mod, path).DefaultEntry(); !ok || got.Value() != "fallback" || got.Origin() != cambium.DefaultOriginTypedef {
					t.Errorf("%s lost applicable inherited default: %+v, %v", path, got, ok)
				}
			}
		})
	}
}

func TestInapplicableDefaultsStillValidateDeclarations(t *testing.T) {
	for _, tc := range []struct{ name, body, message string }{
		{"explicit-mandatory", `leaf value { type string; mandatory true; default invalid; }`, "mandatory leaf"},
		{"explicit-min", `leaf-list value { type string; min-elements 1; default invalid; }`, "with min-elements"},
		{"invalid-key-value", `list rows { key value; leaf value { type uint8; default 999; } }`, `default "999" is not valid`},
		{"invalid-inherited-value", `typedef label { type uint8; default 999; } leaf value { type label; mandatory true; }`, `default "999" is not valid`},
		{"invalid-inherited-restriction", `typedef label { type uint8; default 10; } leaf value { type label { range "1..5"; } mandatory true; }`, `default "10" is not valid`},
		{"duplicate-key-default", `list rows { key value; leaf value { type string; default a; default b; } }`, "multiple default"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := buildPolicyContext(t, cambium.DeviationPolicy{}, `module defaults {
  yang-version 1.1; namespace "urn:defaults"; prefix d; `+tc.body+` }`)
			if err == nil || !strings.Contains(err.Error(), tc.message) {
				t.Errorf("invalid declaration error = %v, want %q", err, tc.message)
			}
		})
	}
}
