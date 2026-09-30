// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

package codegen_test

import "testing"

func TestGeneratedGoIdentityrefRequiresAllBases(t *testing.T) {
	src := generateInlineNamesModule(t, "identities", `module identities {
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
}`)
	runGeneratedGoTest(t, src, `
func TestIdentityrefIntersection(t *testing.T) {
  for _, leaf := range []string{"direct", "alias", "values", "unioned"} {
    for _, tc := range []struct { value string; valid bool }{
      {"only-a", false}, {"only-b", false}, {"root-a", false}, {"unrelated", false},
      {"both", true}, {"grandchild", true}, {"identities:both", true},
    } {
      value := "\"" + tc.value + "\""
      if leaf == "values" { value = "[" + value + "]" }
      input := "{\"identities:" + leaf + "\":" + value + "}"
      doc, err := FromJSONIETF([]byte(input))
      if err == nil { err = doc.Validate() }
      if (err == nil) != tc.valid { t.Errorf("input %s: %v, want valid=%v", input, err, tc.valid) }
    }
  }
  if _, err := FromJSONIETF([]byte("{\"identities:narrowed\":\"only-a\"}")); err == nil {
    t.Error("accepted one of the required bases as its own strict descendant")
  }
  if _, err := FromJSONIETF([]byte("{\"identities:unioned\":7}")); err != nil { t.Fatal(err) }
}
`)
}
