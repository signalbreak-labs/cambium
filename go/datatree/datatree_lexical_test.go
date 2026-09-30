// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

package datatree_test

import (
	"encoding/json/v2"
	"fmt"
	"strings"
	"testing"

	"github.com/signalbreak-labs/cambium/go/datatree"
)

func TestBinaryPreservesDocumentedPEMLineBreaks(t *testing.T) {
	mod := loadModSrc(t, `module pem { namespace urn:pem; prefix p; leaf value { type binary; } }`, "pem")
	for _, value := range []string{strings.Repeat("A", 64) + "\n", strings.Repeat("A", 64) + "\nAA=="} {
		input, err := json.Marshal(map[string]string{"pem:value": value})
		if err != nil {
			t.Fatal(err)
		}
		tree, err := datatree.Parse(mod, datatree.FormatJSONIETF, input)
		if err != nil {
			t.Fatal(err)
		}
		if err := tree.Validate(); err != nil {
			t.Fatalf("documented PEM form rejected: %v", err)
		}
	}
}

const lexicalSchema = `module lexical {
  yang-version 1.1; namespace "urn:lexical"; prefix l;
  typedef flags { type bits { bit red; bit blue; } }
  typedef octets { type binary { length "0..4"; } }
  leaf flags { type flags; }
  leaf flags-ref { type leafref { path "/l:flags"; require-instance false; } }
  leaf flags-union { type union { type flags; type boolean; } }
  leaf flags-text { type union { type flags; type string; } }
  leaf-list flags-list { type flags; }
  leaf octets { type octets; }
  leaf octets-ref { type leafref { path "/l:octets"; require-instance false; } }
  leaf octets-union { type union { type octets; type boolean; } }
  leaf-list octets-list { type octets; }
  leaf-list octets-text { type union { type octets; type string; } }
}`

func TestBitsRejectNonSpaceSeparatorsBeforeCanonicalization(t *testing.T) {
	mod := loadModSrc(t, lexicalSchema, "lexical")
	for _, separator := range []string{"\t", "\n", "\r", "\v", "\u00a0", "\u2003"} {
		for _, value := range []string{"blue" + separator + "red", separator} {
			for _, leaf := range []string{"flags", "flags-ref", "flags-union", "flags-list"} {
				t.Run(fmt.Sprintf("%s/%q", leaf, value), func(t *testing.T) {
					var data any = value
					if leaf == "flags-list" {
						data = []string{value}
					}
					input, err := json.Marshal(map[string]any{"lexical:" + leaf: data})
					if err != nil {
						t.Fatal(err)
					}
					tree, err := datatree.Parse(mod, datatree.FormatJSONIETF, input)
					if err != nil {
						t.Fatal(err)
					}
					output, err := tree.Serialize(datatree.FormatJSONIETF)
					if err != nil {
						t.Fatal(err)
					}
					if string(output) != string(input) {
						t.Errorf("invalid bits were rewritten: %s -> %s", input, output)
					}
					if err := tree.Validate(); err == nil {
						t.Errorf("Validate accepted non-space separator in %s", input)
					}
				})
			}
		}
	}
}

func TestBinaryRejectsLineBreaks(t *testing.T) {
	mod := loadModSrc(t, lexicalSchema, "lexical")
	for _, value := range []string{"Y\nQ==", "YQ==\r", "\r\n", "Y\r\nQ=="} {
		for _, leaf := range []string{"octets", "octets-ref", "octets-union", "octets-list"} {
			t.Run(fmt.Sprintf("%s/%q", leaf, value), func(t *testing.T) {
				var data any = value
				if leaf == "octets-list" {
					data = []string{value}
				}
				input, err := json.Marshal(map[string]any{"lexical:" + leaf: data})
				if err != nil {
					t.Fatal(err)
				}
				tree, err := datatree.Parse(mod, datatree.FormatJSONIETF, input)
				if err != nil {
					t.Fatal(err)
				}
				if err := tree.Validate(); err == nil {
					t.Errorf("Validate accepted non-alphabet base64 characters in %s", input)
				}
			})
		}
	}
}

func TestBitsBinaryLexicalRulesPreserveStringUnionAlternatives(t *testing.T) {
	mod := loadModSrc(t, lexicalSchema, "lexical")
	input := `{"lexical:flags-text":"blue\tred","lexical:octets-text":["AA==","YQ==\n"]}`
	tree, err := datatree.Parse(mod, datatree.FormatJSONIETF, []byte(input))
	if err != nil {
		t.Fatal(err)
	}
	if err := tree.Validate(); err != nil {
		t.Fatal(err)
	}
	output, err := tree.Serialize(datatree.FormatJSONIETF)
	if err != nil {
		t.Fatal(err)
	}
	// The string member keeps its lexical value and sorts before the earlier
	// binary member under the existing union ordering contract.
	want := `{"lexical:flags-text":"blue\tred","lexical:octets-text":["YQ==\n","AA=="]}`
	if string(output) != want {
		t.Errorf("Serialize = %s, want %s", output, want)
	}
}

func TestBitsBinaryValidLexicalForms(t *testing.T) {
	mod := loadModSrc(t, lexicalSchema, "lexical")
	for _, tc := range []struct{ input, want string }{
		{`{"lexical:flags":"  blue  red "}`, `{"lexical:flags":"red blue"}`},
		{`{"lexical:flags":""}`, `{"lexical:flags":""}`},
		{`{"lexical:octets":""}`, `{"lexical:octets":""}`},
		{`{"lexical:octets":"YQ=="}`, `{"lexical:octets":"YQ=="}`},
	} {
		tree, err := datatree.Parse(mod, datatree.FormatJSONIETF, []byte(tc.input))
		if err != nil {
			t.Fatal(err)
		}
		if err := tree.Validate(); err != nil {
			t.Fatalf("Validate(%s): %v", tc.input, err)
		}
		output, err := tree.Serialize(datatree.FormatJSONIETF)
		if err != nil {
			t.Fatal(err)
		}
		if string(output) != tc.want {
			t.Errorf("Serialize = %s, want %s", output, tc.want)
		}
	}
}

func TestBitsBinaryRejectInvalidXMLLexicalForms(t *testing.T) {
	mod := loadModSrc(t, lexicalSchema, "lexical")
	for _, input := range []string{
		"<flags xmlns=\"urn:lexical\">blue\tred</flags>",
		"<octets xmlns=\"urn:lexical\">YQ==\n</octets>",
	} {
		tree, err := datatree.Parse(mod, datatree.FormatXML, []byte(input))
		if err != nil {
			t.Fatal(err)
		}
		if err := tree.Validate(); err == nil {
			t.Errorf("Validate accepted invalid XML lexical value: %q", input)
		}
	}
}
