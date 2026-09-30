// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

package cambium_test

import (
	"fmt"
	"strings"
	"testing"
)

func TestIdentityrefDefaultsRequireAllBases(t *testing.T) {
	for _, form := range []struct {
		name, definition string
	}{
		{"leaf", `leaf value { type identityref { base root-a; base root-b; } default %s; }`},
		{"leaf-list", `leaf-list value { type combined; default %s; }`},
		{"typedef", `typedef with-default { type combined; default %s; } leaf value { type with-default; }`},
		{"union", `leaf value { type union { type combined; type uint8; } default %s; }`},
		{"nested union", `leaf value { type union { type union { type combined; type uint8; } type boolean; } default %s; }`},
		{"leafref", `leaf target { type combined; } leaf value { type leafref { path "../target"; } default %s; }`},
	} {
		for _, value := range []struct {
			name  string
			valid bool
		}{
			{"only-a", false}, {"only-b", false}, {"both", true}, {"grandchild", true},
			{"root-a", false}, {"unrelated", false}, {"i:both", true},
		} {
			t.Run(form.name+"/"+value.name, func(t *testing.T) {
				b := retentionBuilder(t, false)
				source := `module identity-defaults {
  yang-version 1.1; namespace "urn:identity-defaults"; prefix i;
  identity root-a; identity root-b; identity unrelated;
  identity only-a { base root-a; } identity only-b { base root-b; }
  identity both { base root-a; base root-b; }
  identity grandchild { base both; }
  typedef combined { type identityref { base root-a; base root-b; } }
` + fmt.Sprintf(form.definition, value.name) + `}`
				if err := b.LoadModuleStr(source); err != nil {
					t.Fatal(err)
				}
				ctx, err := b.Build()
				if ctx != nil {
					t.Cleanup(ctx.Close)
				}
				if (err == nil) != value.valid {
					t.Errorf("Build default %s = %v, want valid=%v", value.name, err, value.valid)
				} else if err != nil && !strings.Contains(err.Error(), "default") {
					t.Errorf("unexpected rejection: %v", err)
				}
			})
		}
	}
}

func TestIdentityrefDefaultsExcludeEveryBaseItself(t *testing.T) {
	for _, bases := range []string{"base root;", "base root; base child;"} {
		for _, value := range []string{"root", "child", "grandchild"} {
			b := retentionBuilder(t, false)
			source := fmt.Sprintf(`module identity-derived-bases {
  yang-version 1.1; namespace "urn:identity-derived-bases"; prefix i;
  identity root; identity child { base root; } identity grandchild { base child; }
  leaf value { type identityref { %s } default %s; }
}`, bases, value)
			if err := b.LoadModuleStr(source); err != nil {
				t.Fatal(err)
			}
			ctx, err := b.Build()
			if ctx != nil {
				t.Cleanup(ctx.Close)
			}
			valid := value == "grandchild" || value == "child" && bases == "base root;"
			if (err == nil) != valid {
				t.Errorf("bases %q default %q: Build = %v, want valid=%v", bases, value, err, valid)
			}
		}
	}
}
