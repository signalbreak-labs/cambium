// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

package cambium_test

import (
	"strings"
	"testing"

	"github.com/signalbreak-labs/cambium/go/cambium"
)

// An augment into another module must not add mandatory nodes (RFC 7950
// §7.17; RFC 6020 §7.15: "to the target node"). A mandatory node is a leaf,
// choice, anydata or anyxml with "mandatory true", a list or leaf-list with
// min-elements above zero, or a non-presence container with a mandatory child
// (RFC 7950 §3): mandatory descendants of a presence container, a list, or a
// new case do not make the augment's own nodes mandatory. ietf-ip@2014-06-16
// (RFC 7277) relies on this: its interface augment holds a mandatory choice
// inside a list inside a presence container. Verdicts match libyang v5.4.9.
func TestCrossModuleAugmentMandatoryNodesPerRFCDefinition(t *testing.T) {
	const base = `module cambium-augment-mandatory-base {
    namespace "urn:cambium:augment-mandatory-base";
    prefix camb;
    container top { leaf mode { type string; } choice ch { leaf one { type string; } } }
}`
	cases := []struct {
		name    string
		target  string // "" means /base:top
		body    string
		wantErr string // "" means the augment is accepted
	}{
		{
			name: "mandatory choice in list in presence container",
			body: `container ipv4 {
            presence "enables";
            list address {
                key ip;
                leaf ip { type string; }
                choice subnet { mandatory true; leaf prefix-length { type uint8; } leaf netmask { type string; } }
            }
        }`,
		},
		{
			name: "mandatory leaf in list",
			body: `list entry { key id; leaf id { type string; } leaf req { type string; mandatory true; } }`,
		},
		{
			name:   "mandatory leaf in a new case of a choice",
			target: "/base:top/base:ch",
			body:   `case extra { leaf req { type string; mandatory true; } }`,
		},
		{
			name: "mandatory leaf in presence container in non-presence container",
			body: `container outer { container inner { presence "p"; leaf req { type string; mandatory true; } } }`,
		},
		{
			name:    "mandatory leaf in non-presence container",
			body:    `container outer { leaf req { type string; mandatory true; } }`,
			wantErr: `adds mandatory config node "outer"`,
		},
		{
			name:    "list with min-elements",
			body:    `list entry { key id; min-elements 1; leaf id { type string; } }`,
			wantErr: `adds mandatory config node "entry"`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			target := tc.target
			if target == "" {
				target = "/base:top"
			}
			augment := `module cambium-augment-mandatory-source {
    namespace "urn:cambium:augment-mandatory-source";
    prefix cams;
    import cambium-augment-mandatory-base { prefix base; }
    augment "` + target + `" {
        ` + tc.body + `
    }
}`
			builder, err := cambium.NewContextBuilder(cambium.ContextFlags{})
			if err != nil {
				t.Fatal(err)
			}
			for _, src := range []string{base, augment} {
				if err := builder.LoadModuleStr(src); err != nil {
					t.Fatalf("LoadModuleStr: %v", err)
				}
			}
			ctx, err := builder.Build()
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("Build rejected an augment without mandatory nodes: %v", err)
				}
				ctx.Close()
				return
			}
			if err == nil {
				ctx.Close()
				t.Fatal("Build accepted an augment adding a mandatory node")
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("Build error = %q, want to contain %q", err.Error(), tc.wantErr)
			}
		})
	}
}
