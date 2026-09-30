// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

package codegen_test

import "testing"

func TestGeneratedGoEmptyDerivedEnumBits(t *testing.T) {
	src := generateInlineNamesModule(t, "filtered", `module filtered {
  yang-version 1.1; namespace "urn:filtered"; prefix f; feature available;
  typedef enum-base { type enumeration { enum allowed; enum forbidden; } }
  typedef bit-base { type bits { bit allowed; bit forbidden; } }
  leaf e { type enum-base { enum allowed { if-feature available; } } }
  leaf b { type bit-base { bit allowed { if-feature available; } } }
  leaf u { type union { type enum-base { enum allowed { if-feature available; } } type uint8; } }
}`)
	runGeneratedGoTest(t, src, `
func TestEmptyDerivedEnumBits(t *testing.T) {
  for _, in := range []string{
    "{\"filtered:e\":\"allowed\"}", "{\"filtered:e\":\"forbidden\"}",
    "{\"filtered:b\":\"allowed\"}", "{\"filtered:b\":\"forbidden\"}",
    "{\"filtered:u\":\"allowed\"}", "{\"filtered:u\":\"forbidden\"}",
  } {
    if _, err := FromJSONIETF([]byte(in)); err == nil {
      t.Errorf("accepted disabled/forbidden value: %s", in)
    }
  }
  for _, in := range []string{"{}", "{\"filtered:b\":\"\"}", "{\"filtered:u\":7}"} {
    doc, err := FromJSONIETF([]byte(in))
    if err != nil { t.Fatalf("valid document %s: %v", in, err) }
    if err := doc.Validate(); err != nil { t.Errorf("valid document %s: %v", in, err) }
  }
  doc := &Filtered{E: new(FilteredEEnum(0))}
  if err := doc.Validate(); err == nil { t.Error("empty enumeration accepted numeric zero") }
}
`)
}
