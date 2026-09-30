// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

package datatree_test

import (
	"fmt"
	"testing"
)

func TestIdentityrefRequiresAllBases(t *testing.T) {
	mod := loadModSrc(t, `module identities {
  yang-version 1.1; namespace "urn:identities"; prefix i;
  identity root-a; identity root-b; identity unrelated;
  identity only-a { base root-a; } identity only-b { base root-b; }
  identity both { base root-a; base root-b; }
  identity grandchild { base both; }
  typedef combined { type identityref { base root-a; base root-b; } }
  leaf direct { type identityref { base root-a; base root-b; } }
  leaf alias { type combined; }
  leaf-list values { type combined; }
  leaf unioned { type union { type combined; type uint8; } }
  leaf narrowed { type identityref { base root-a; base only-a; } }
}`, "identities")
	for _, leaf := range []string{"direct", "alias", "values", "unioned"} {
		for _, tc := range []struct {
			value string
			valid bool
		}{
			{"only-a", false}, {"only-b", false}, {"root-a", false}, {"unrelated", false},
			{"both", true}, {"grandchild", true}, {"identities:both", true},
		} {
			value := fmt.Sprintf("%q", tc.value)
			if leaf == "values" {
				value = "[" + value + "]"
			}
			input := fmt.Sprintf(`{"identities:%s":%s}`, leaf, value)
			if err := validateOne(t, mod, input); (err == nil) != tc.valid {
				t.Errorf("Validate(%s) = %v, want valid=%v", input, err, tc.valid)
			}
		}
	}
	if err := validateOne(t, mod, `{"identities:narrowed":"only-a"}`); err == nil {
		t.Error("accepted one required base as its own strict descendant")
	}
	if err := validateOne(t, mod, `{"identities:unioned":7}`); err != nil {
		t.Fatal(err)
	}
}
