// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

package codegen_test

import "testing"

// RFC 7950 section 9.10.2: an identityref value must be derived from its base;
// the base identity itself is not a valid value. RFC 7951 section 6.8: an
// identity from the leaf's own module may be written with or without the
// module prefix, and is emitted without it.
func TestGeneratedGoIdentityrefExcludesBaseIdentity(t *testing.T) {
	const ids = `module idref-base-ids {
    yang-version 1.1;
    namespace "urn:idref-base-ids";
    prefix i;
    identity base;
    identity child { base base; }
    identity gc { base child; }
    identity lonely;
}`
	const mod = `module idref-base {
    yang-version 1.1;
    namespace "urn:idref-base";
    prefix m;
    import idref-base-ids { prefix i; }
    identity local { base i:base; }
    leaf id { type identityref { base i:base; } }
    leaf-list ids { type identityref { base i:child; } }
    leaf u { type union { type identityref { base i:base; } type int8; } }
    leaf lonely-ref { type identityref { base i:lonely; } }
}`
	src := generateInlineNamesModule(t, "idref-base", ids, mod)

	testBody := `
func TestGeneratedIdentityrefExcludesBaseIdentity(t *testing.T) {
	for _, tc := range []struct {
		in, out string
	}{
		{` + "`" + `{"idref-base:id":"idref-base-ids:base"}` + "`" + `, ""},
		{` + "`" + `{"idref-base:id":"idref-base-ids:child"}` + "`" + `, "{\n  \"idref-base:id\": \"idref-base-ids:child\"\n}\n"},
		{` + "`" + `{"idref-base:id":"idref-base-ids:gc"}` + "`" + `, "{\n  \"idref-base:id\": \"idref-base-ids:gc\"\n}\n"},
		{` + "`" + `{"idref-base:id":"local"}` + "`" + `, "{\n  \"idref-base:id\": \"local\"\n}\n"},
		{` + "`" + `{"idref-base:id":"idref-base:local"}` + "`" + `, "{\n  \"idref-base:id\": \"local\"\n}\n"},
		{` + "`" + `{"idref-base:ids":["idref-base-ids:child"]}` + "`" + `, ""},
		{` + "`" + `{"idref-base:ids":["idref-base-ids:gc"]}` + "`" + `, "{\n  \"idref-base:ids\": [\n    \"idref-base-ids:gc\"\n  ]\n}\n"},
		{` + "`" + `{"idref-base:u":"idref-base-ids:base"}` + "`" + `, ""},
		{` + "`" + `{"idref-base:u":"idref-base:local"}` + "`" + `, "{\n  \"idref-base:u\": \"local\"\n}\n"},
		{` + "`" + `{"idref-base:lonely-ref":"idref-base-ids:lonely"}` + "`" + `, ""},
	} {
		doc, err := FromJSONIETF([]byte(tc.in))
		if tc.out == "" {
			if err == nil {
				t.Errorf("FromJSONIETF(%s) accepted a value not derived from the base", tc.in)
			}
			continue
		}
		if err != nil {
			t.Errorf("FromJSONIETF(%s): %v", tc.in, err)
			continue
		}
		if got := doc.ToJSONIETF(); got != tc.out {
			t.Errorf("FromJSONIETF(%s).ToJSONIETF():\n got: %q\nwant: %q", tc.in, got, tc.out)
		}
	}
}
`
	runGeneratedGoTest(t, src, testBody)
}

// An identityref leaf augmented into another module's tree qualifies foreign
// identities relative to the leaf's own module (RFC 7951 section 6.8), no
// matter which module the code is generated for.
func TestGeneratedGoIdentityrefQualifiesAgainstLeafModule(t *testing.T) {
	const a = `module idref-leaf-a {
    yang-version 1.1;
    namespace "urn:idref-leaf-a";
    prefix a;
    identity base;
    identity one { base base; }
    container top { leaf own { type string; } }
}`
	const b = `module idref-leaf-b {
    yang-version 1.1;
    namespace "urn:idref-leaf-b";
    prefix b;
    import idref-leaf-a { prefix a; }
    augment "/a:top" { leaf idr { type identityref { base a:base; } } }
}`
	for _, module := range []string{"idref-leaf-a", "idref-leaf-b"} {
		t.Run(module, func(t *testing.T) {
			src := generateInlineNamesModule(t, module, a, b)
			testBody := `
func TestGeneratedIdentityrefQualifiesAgainstLeafModule(t *testing.T) {
	doc, err := FromJSONIETF([]byte(` + "`" + `{"idref-leaf-a:top":{"idref-leaf-b:idr":"idref-leaf-a:one"}}` + "`" + `))
	if err != nil {
		t.Fatalf("FromJSONIETF rejected the qualified foreign identity: %v", err)
	}
	if got, want := doc.ToJSONIETF(), "{\n  \"idref-leaf-a:top\": {\n    \"idref-leaf-b:idr\": \"idref-leaf-a:one\"\n  }\n}\n"; got != want {
		t.Fatalf("JSON mismatch:\n got: %q\nwant: %q", got, want)
	}
	if got, want := doc.ToXML(), "<top xmlns=\"urn:idref-leaf-a\">\n  <idr xmlns=\"urn:idref-leaf-b\" xmlns:a=\"urn:idref-leaf-a\">a:one</idr>\n</top>\n"; got != want {
		t.Fatalf("XML mismatch:\n got: %q\nwant: %q", got, want)
	}
	if _, err := FromJSONIETF([]byte(` + "`" + `{"idref-leaf-a:top":{"idref-leaf-b:idr":"one"}}` + "`" + `)); err == nil {
		t.Fatal("FromJSONIETF accepted an unqualified identity from another module")
	}
}
`
			runGeneratedGoTest(t, src, testBody)
		})
	}
}

const anydataCodegenModule = `module anydata-content-codegen {
    yang-version 1.1;
    namespace "urn:anydata-content-codegen";
    prefix acc;
    container top {
        anydata ad;
        anyxml ax;
        leaf u1 { type string; }
    }
}`

// Validate must reject anydata/anyxml content that is not a well-formed XML
// fragment or JSON value, so raw content cannot inject sibling markup.
func TestGeneratedGoAnydataValidateRejectsMalformedContent(t *testing.T) {
	src := generateInlineNamesModule(t, "anydata-content-codegen", anydataCodegenModule)
	testBody := `
func TestGeneratedAnydataValidateRejectsMalformedContent(t *testing.T) {
	for _, tc := range []struct {
		xml, json string
		ok        bool
	}{
		{"", "", true},
		{"<custom>value</custom>", "{\"custom\":\"value\"}", true},
		{"<a>1</a>\n<b xmlns=\"urn:x\"><c/></b>", "{\n  \"a\": 1\n}", true},
		{"<!-- note --><a>&amp;&#65;</a>", "[1, 2]", true},
		{"</ad><inject xmlns=\"x\"/><ad>", "{}", false},
		{"<cambium-anydata/></cambium-anydata><x/><cambium-anydata>", "{}", false},
		{"<a>", "{}", false},
		{"<a>&undefined;</a>", "{}", false},
		{"<?xml version=\"1.0\"?><a/>", "{}", false},
		{"<!DOCTYPE a><a/>", "{}", false},
		{"<a>\x00</a>", "{}", false},
		{"<a/>", "1}, \"anydata-content-codegen:u1\": \"evil", false},
		{"<a/>", "{\"a\":1} {\"b\":2}", false},
	} {
		value := NewAnyData(tc.xml, tc.json)
		for _, doc := range []*AnydataContentCodegen{
			{Top: AnydataContentCodegenTop{Ad: &value}},
			{Top: AnydataContentCodegenTop{Ax: &value}},
		} {
			err := doc.Validate()
			if tc.ok && err != nil {
				t.Errorf("Validate(%q, %q) rejected well-formed content: %v", tc.xml, tc.json, err)
			}
			if !tc.ok && err == nil {
				t.Errorf("Validate(%q, %q) accepted malformed content", tc.xml, tc.json)
			}
		}
	}
}
`
	runGeneratedGoTest(t, src, testBody)
}

// anydata parsed from JSON_IETF has no XML form; the checked XML serializer
// reports that instead of emitting an empty element, and a deeply nested value
// re-serializes to JSON in size linear in its input.
func TestGeneratedGoAnydataFromJSONIsNotSilentlyEmptyXML(t *testing.T) {
	src := generateInlineNamesModule(t, "anydata-content-codegen", anydataCodegenModule)
	testBody := `
func TestGeneratedAnydataFromJSONIsNotSilentlyEmptyXML(t *testing.T) {
	doc, err := FromJSONIETF([]byte(` + "`" + `{"anydata-content-codegen:top":{"ad":{"k":{"x":1}}}}` + "`" + `))
	if err != nil {
		t.Fatalf("FromJSONIETF: %v", err)
	}
	if got, want := doc.ToJSONIETF(), "{\n  \"anydata-content-codegen:top\": {\n    \"ad\": {\n      \"k\": {\n        \"x\": 1\n      }\n    }\n  }\n}\n"; got != want {
		t.Fatalf("JSON mismatch:\n got: %q\nwant: %q", got, want)
	}
	if out, err := doc.ToXMLChecked(); err == nil {
		t.Fatalf("ToXMLChecked serialized JSON-only anydata to XML: %q", out)
	} else if got, want := err.Error(), "/anydata-content-codegen/top/ad: anydata/anyxml value has no XML form (it was parsed from JSON_IETF); cross-format anydata/anyxml serialization is unsupported"; got != want {
		t.Fatalf("ToXMLChecked error = %q, want %q", got, want)
	}
	if _, err := doc.Top.ToXMLChecked(); err == nil {
		t.Fatal("container ToXMLChecked serialized JSON-only anydata to XML")
	}

	both := NewAnyData("<k>v</k>", "{\"k\":\"v\"}")
	built := &AnydataContentCodegen{Top: AnydataContentCodegenTop{Ad: &both}}
	got, err := built.ToXMLChecked()
	if err != nil {
		t.Fatalf("ToXMLChecked: %v", err)
	}
	if want := built.ToXML(); got != want {
		t.Fatalf("ToXMLChecked = %q, want ToXML output %q", got, want)
	}

	const depth = 3000
	in := []byte(` + "`" + `{"anydata-content-codegen:top":{"ad":` + "`" + `)
	for i := 0; i < depth; i++ {
		in = append(in, ` + "`" + `{"a":` + "`" + `...)
	}
	in = append(in, '1')
	for i := 0; i < depth; i++ {
		in = append(in, '}')
	}
	in = append(in, "}}"...)
	deep, err := FromJSONIETF(in)
	if err != nil {
		t.Fatalf("FromJSONIETF deep: %v", err)
	}
	if out := deep.ToJSONIETF(); len(out) > 10*len(in) {
		t.Fatalf("deep anydata JSON grew from %d to %d bytes", len(in), len(out))
	}
}
`
	runGeneratedGoTest(t, src, testBody)
}
