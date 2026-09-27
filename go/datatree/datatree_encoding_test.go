// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

package datatree_test

import (
	"strings"
	"testing"

	"github.com/signalbreak-labs/cambium/go/datatree"
)

// Leaf values are held in their canonical form (RFC 7950 §9.1), as libyang
// stores them, whichever format and lexical form they were parsed from.
const canonicalValueSchema = `module cv {
    yang-version 1.1;
    namespace "urn:cv"; prefix cv;
    identity base;
    identity one { base base; }
    container top {
        leaf d2 { type decimal64 { fraction-digits 2; } }
        leaf bb { type bits { bit hi { position 9; } bit lo { position 0; } bit mid { position 3; } } }
        leaf i32 { type int32; }
        leaf i64 { type int64; }
        leaf u64 { type uint64; }
        leaf id { type identityref { base base; } }
        leaf un { type union { type decimal64 { fraction-digits 2; } type string; } }
        leaf lr { type leafref { path "../d2"; } }
        leaf-list dl { ordered-by user; type decimal64 { fraction-digits 3; } }
        leaf dd { type decimal64 { fraction-digits 2; } default "2.50"; }
    }
}`

func TestCanonicalLeafValuesFromJSON(t *testing.T) {
	mod := loadModSrc(t, canonicalValueSchema, "cv")
	cases := []struct {
		name, leaf, in, want, wantXML string
	}{
		{"decimal64 trailing zero", "d2", `"45.50"`, `"45.5"`, "45.5"},
		{"decimal64 integer", "d2", `"1"`, `"1.0"`, "1.0"},
		{"decimal64 zero", "d2", `"0"`, `"0.0"`, "0.0"},
		{"decimal64 negative zero", "d2", `"-0.00"`, `"0.0"`, "0.0"},
		{"decimal64 plus sign", "d2", `"+2.5"`, `"2.5"`, "2.5"},
		{"decimal64 leading zeros", "d2", `"007.10"`, `"7.1"`, "7.1"},
		{"decimal64 small negative", "d2", `"-0.05"`, `"-0.05"`, "-0.05"},
		{"decimal64 too many fraction digits left for Validate", "d2", `"1.234"`, `"1.234"`, "1.234"},
		{"bits in position order", "bb", `"hi  mid lo"`, `"lo mid hi"`, "lo mid hi"},
		{"bits unknown bit left for Validate", "bb", `"lo nope"`, `"lo nope"`, "lo nope"},
		{"int32 negative zero", "i32", `-0`, `0`, "0"},
		{"int64 sign and zeros", "i64", `"+007"`, `"7"`, "7"},
		{"uint64 leading zeros", "u64", `"0005"`, `"5"`, "5"},
		{"identityref own module qualifier dropped", "id", `"cv:one"`, `"one"`, "one"},
		{"union decimal64 member", "un", `"3.10"`, `"3.1"`, "3.1"},
		{"union string member", "un", `"abc"`, `"abc"`, "abc"},
		{"leafref to decimal64", "lr", `"2.50"`, `"2.5"`, "2.5"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			j, x := serializeBoth(t, mod, datatree.FormatJSONIETF, `{"cv:top":{"`+tc.leaf+`":`+tc.in+`}}`)
			if want := `{"cv:top":{"` + tc.leaf + `":` + tc.want + `}}`; j != want {
				t.Fatalf("JSON:\n got: %s\nwant: %s", j, want)
			}
			if want := `<top xmlns="urn:cv"><` + tc.leaf + `>` + tc.wantXML + `</` + tc.leaf + `></top>`; x != want {
				t.Fatalf("XML:\n got: %s\nwant: %s", x, want)
			}
		})
	}
}

func TestCanonicalLeafValuesFromXML(t *testing.T) {
	mod := loadModSrc(t, canonicalValueSchema, "cv")
	in := `<top xmlns="urn:cv"><d2>-1.50</d2><bb>mid hi lo</bb><i64>+0042</i64>` +
		`<un>0.10</un><dl>2.500</dl><dl>1</dl></top>`
	j, x := serializeBoth(t, mod, datatree.FormatXML, in)
	if want := `{"cv:top":{"d2":"-1.5","bb":"lo mid hi","i64":"42","un":"0.1","dl":["2.5","1.0"]}}`; j != want {
		t.Fatalf("JSON:\n got: %s\nwant: %s", j, want)
	}
	if want := `<top xmlns="urn:cv"><d2>-1.5</d2><bb>lo mid hi</bb><i64>42</i64><un>0.1</un><dl>2.5</dl><dl>1.0</dl></top>`; x != want {
		t.Fatalf("XML:\n got: %s\nwant: %s", x, want)
	}
}

// XML identityref prefixes are document bindings (RFC 7950 §9.10.3): they
// resolve against the xmlns declarations in scope on the element, never the
// schema's import prefixes, and a foreign identity is serialized with its
// module's prefix declared on the element (as libyang prints it).
const idrefBase = `module idb {
    namespace "urn:idb"; prefix tifb;
    identity component-class;
    identity cpu { base component-class; }
}`

const idrefMain = `module idm {
    namespace "urn:idm"; prefix tifmp;
    import idb { prefix foreign; }
    identity local { base foreign:component-class; }
    leaf component { type identityref { base foreign:component-class; } }
    leaf-list comps { type identityref { base foreign:component-class; } }
    container c { leaf r { type identityref { base foreign:component-class; } } }
    leaf un { type union { type int8; type identityref { base foreign:component-class; } } }
}`

func TestXMLIdentityRefResolvesInScopeNamespaces(t *testing.T) {
	mod := loadMultiModSrc(t, "idm", idrefBase, idrefMain)
	cases := []struct {
		name, in, wantJSON, wantXML string
	}{
		{
			"document prefix differs from schema prefixes",
			`<component xmlns="urn:idm" xmlns:zz="urn:idb">zz:cpu</component>`,
			`{"idm:component":"idb:cpu"}`,
			`<component xmlns="urn:idm" xmlns:tifb="urn:idb">tifb:cpu</component>`,
		},
		{
			"prefix declared on an ancestor",
			`<c xmlns="urn:idm" xmlns:q="urn:idb"><r>q:cpu</r></c>`,
			`{"idm:c":{"r":"idb:cpu"}}`,
			`<c xmlns="urn:idm"><r xmlns:tifb="urn:idb">tifb:cpu</r></c>`,
		},
		{
			"unprefixed value is in the default namespace",
			`<x:component xmlns:x="urn:idm" xmlns="urn:idb">cpu</x:component>`,
			`{"idm:component":"idb:cpu"}`,
			`<component xmlns="urn:idm" xmlns:tifb="urn:idb">tifb:cpu</component>`,
		},
		{
			"own-module identity stays bare",
			`<component xmlns="urn:idm">local</component>`,
			`{"idm:component":"local"}`,
			`<component xmlns="urn:idm">local</component>`,
		},
		{
			"leaf-list values each declare their prefix",
			`<comps xmlns="urn:idm">local</comps><comps xmlns="urn:idm" xmlns:f="urn:idb">f:cpu</comps>`,
			`{"idm:comps":["idb:cpu","local"]}`,
			`<comps xmlns="urn:idm" xmlns:tifb="urn:idb">tifb:cpu</comps><comps xmlns="urn:idm">local</comps>`,
		},
		{
			"union identityref member",
			`<un xmlns="urn:idm" xmlns:f="urn:idb">f:cpu</un>`,
			`{"idm:un":"idb:cpu"}`,
			`<un xmlns="urn:idm" xmlns:tifb="urn:idb">tifb:cpu</un>`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			j, x := serializeBoth(t, mod, datatree.FormatXML, tc.in)
			if j != tc.wantJSON {
				t.Fatalf("JSON:\n got: %s\nwant: %s", j, tc.wantJSON)
			}
			if x != tc.wantXML {
				t.Fatalf("XML:\n got: %s\nwant: %s", x, tc.wantXML)
			}
			tree, err := datatree.Parse(mod, datatree.FormatXML, []byte(tc.in))
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			if err := tree.Validate(); err != nil {
				t.Fatalf("Validate: %v", err)
			}
		})
	}
}

func TestXMLIdentityRefIgnoresUndeclaredSchemaPrefix(t *testing.T) {
	mod := loadMultiModSrc(t, "idm", idrefBase, idrefMain)
	// "foreign" is idm's import prefix, but the document never binds it.
	tree, err := datatree.Parse(mod, datatree.FormatXML, []byte(`<component xmlns="urn:idm">foreign:cpu</component>`))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if err := tree.Validate(); err == nil {
		t.Fatal("an identityref prefix with no xmlns binding in scope must not resolve")
	}
}

func TestIdentityRefJSONToXMLDeclaresForeignPrefix(t *testing.T) {
	mod := loadMultiModSrc(t, "idm", idrefBase, idrefMain)
	_, x := serializeBoth(t, mod, datatree.FormatJSONIETF, `{"idm:component":"idb:cpu","idm:c":{"r":"local"}}`)
	want := `<component xmlns="urn:idm" xmlns:tifb="urn:idb">tifb:cpu</component><c xmlns="urn:idm"><r>local</r></c>`
	if x != want {
		t.Fatalf("XML:\n got: %s\nwant: %s", x, want)
	}
}

// instance-identifier values are paths whose qualifiers depend on the format:
// XML prefixes every node with an xmlns-bound prefix, JSON_IETF names the
// module on the first node and wherever it changes (RFC 7951 §6.11). Values are
// converted at the format boundary and kept in canonical JSON_IETF form.
const iidSchema = `module iid {
    yang-version 1.1;
    namespace "urn:iid"; prefix ip;
    list node { key id; leaf id { type string; } }
    list vlan {
        key "id name";
        leaf id { type uint16; }
        leaf name { type string; }
        list member { key port; leaf port { type string; } }
    }
    leaf-list tag { type string; }
    container top { leaf s { type string; } }
    leaf ref { type instance-identifier; }
    leaf ref2 { type instance-identifier { require-instance false; } }
    leaf un { type union { type int8; type instance-identifier { require-instance false; } } }
}`

const iidAug = `module iida {
    yang-version 1.1;
    namespace "urn:iida"; prefix ia;
    import iid { prefix b; }
    augment /b:top {
        leaf extra { type string; }
        leaf ptr { type instance-identifier { require-instance false; } }
        leaf ptr2 { type instance-identifier; }
    }
}`

func TestInstanceIdentifierXMLToJSON(t *testing.T) {
	mod := loadMultiModSrc(t, "iid", iidSchema, iidAug)
	cases := []struct {
		name, in, wantJSON, wantXML string
	}{
		{
			"document prefixes become module names",
			`<ref2 xmlns="urn:iid" xmlns:t="urn:iid">/t:node[t:id='x']</ref2>`,
			`{"iid:ref2":"/iid:node[id='x']"}`,
			`<ref2 xmlns="urn:iid" xmlns:ip="urn:iid">/ip:node[ip:id='x']</ref2>`,
		},
		{
			"multi-key and nested list, key value canonical",
			`<ref2 xmlns="urn:iid" xmlns:t="urn:iid">/t:vlan[t:id='0100'][t:name='mgmt']/t:member[t:port='eth0']</ref2>`,
			`{"iid:ref2":"/iid:vlan[id='100'][name='mgmt']/member[port='eth0']"}`,
			`<ref2 xmlns="urn:iid" xmlns:ip="urn:iid">/ip:vlan[ip:id='100'][ip:name='mgmt']/ip:member[ip:port='eth0']</ref2>`,
		},
		{
			"module changes along the path",
			`<ref2 xmlns="urn:iid" xmlns:x="urn:iid" xmlns:y="urn:iida">/x:top/y:extra</ref2>`,
			`{"iid:ref2":"/iid:top/iida:extra"}`,
			`<ref2 xmlns="urn:iid" xmlns:ip="urn:iid" xmlns:ia="urn:iida">/ip:top/ia:extra</ref2>`,
		},
		{
			"leaf-list predicate and whitespace",
			`<ref2 xmlns="urn:iid" xmlns:t="urn:iid">/t:tag[ . = 'blue' ]</ref2>`,
			`{"iid:ref2":"/iid:tag[.='blue']"}`,
			`<ref2 xmlns="urn:iid" xmlns:ip="urn:iid">/ip:tag[.='blue']</ref2>`,
		},
		{
			"union member",
			`<un xmlns="urn:iid" xmlns:t="urn:iid">/t:top/t:s</un>`,
			`{"iid:un":"/iid:top/s"}`,
			`<un xmlns="urn:iid" xmlns:ip="urn:iid">/ip:top/ip:s</un>`,
		},
		{
			"augmented leaf, prefix bound on the leaf",
			`<top xmlns="urn:iid"><ptr xmlns="urn:iida" xmlns:q="urn:iid">/q:node[q:id='x']</ptr></top>`,
			`{"iid:top":{"iida:ptr":"/iid:node[id='x']"}}`,
			`<top xmlns="urn:iid"><ptr xmlns="urn:iida" xmlns:ip="urn:iid">/ip:node[ip:id='x']</ptr></top>`,
		},
		{
			"unresolvable path kept as written",
			`<ref2 xmlns="urn:iid" xmlns:t="urn:iid">/t:nope</ref2>`,
			`{"iid:ref2":"/t:nope"}`,
			`<ref2 xmlns="urn:iid">/t:nope</ref2>`,
		},
		{
			"undeclared prefix kept as written",
			`<ref2 xmlns="urn:iid">/zz:node[zz:id='x']</ref2>`,
			`{"iid:ref2":"/zz:node[zz:id='x']"}`,
			`<ref2 xmlns="urn:iid">/zz:node[zz:id='x']</ref2>`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			j, x := serializeBoth(t, mod, datatree.FormatXML, tc.in)
			if j != tc.wantJSON {
				t.Fatalf("JSON:\n got: %s\nwant: %s", j, tc.wantJSON)
			}
			if xmlQuotes.Replace(x) != tc.wantXML {
				t.Fatalf("XML:\n got: %s\nwant: %s", x, tc.wantXML)
			}
		})
	}
}

// xmlQuotes undoes the serializer's quote escaping in element text, which is
// not what these tests are about.
var xmlQuotes = strings.NewReplacer("&apos;", "'", "&quot;", `"`)

func TestInstanceIdentifierJSONCanonicalAndToXML(t *testing.T) {
	mod := loadMultiModSrc(t, "iid", iidSchema, iidAug)
	cases := []struct{ in, wantJSON, wantXML string }{
		{
			`{"iid:ref2":"/iid:vlan[iid:id=\"7\"][ name = 'a' ]/iid:member[port='p']"}`,
			`{"iid:ref2":"/iid:vlan[id='7'][name='a']/member[port='p']"}`,
			`<ref2 xmlns="urn:iid" xmlns:ip="urn:iid">/ip:vlan[ip:id='7'][ip:name='a']/ip:member[ip:port='p']</ref2>`,
		},
		{
			`{"iid:ref2":"/iid:node[id=\"it's\"]"}`,
			`{"iid:ref2":"/iid:node[id=\"it's\"]"}`,
			`<ref2 xmlns="urn:iid" xmlns:ip="urn:iid">/ip:node[ip:id="it's"]</ref2>`,
		},
		{
			`{"iid:ref2":"/iid:top/iida:extra"}`,
			`{"iid:ref2":"/iid:top/iida:extra"}`,
			`<ref2 xmlns="urn:iid" xmlns:ip="urn:iid" xmlns:ia="urn:iida">/ip:top/ia:extra</ref2>`,
		},
	}
	for _, tc := range cases {
		j, x := serializeBoth(t, mod, datatree.FormatJSONIETF, tc.in)
		if j != tc.wantJSON {
			t.Fatalf("JSON:\n got: %s\nwant: %s", j, tc.wantJSON)
		}
		if xmlQuotes.Replace(x) != tc.wantXML {
			t.Fatalf("XML:\n got: %s\nwant: %s", x, tc.wantXML)
		}
		// And the XML form parses back to the same canonical JSON.
		back, _ := serializeBoth(t, mod, datatree.FormatXML, x)
		if back != tc.wantJSON {
			t.Fatalf("XML round trip:\n got: %s\nwant: %s", back, tc.wantJSON)
		}
	}
}

func TestInstanceIdentifierRequireInstanceAcrossFormats(t *testing.T) {
	mod := loadMultiModSrc(t, "iid", iidSchema, iidAug)
	// XML: document prefixes that are not schema prefixes still resolve.
	ok := `<node xmlns="urn:iid"><id>x</id></node><ref xmlns="urn:iid" xmlns:t="urn:iid">/t:node[t:id='x']</ref>`
	missing := `<node xmlns="urn:iid"><id>x</id></node><ref xmlns="urn:iid" xmlns:t="urn:iid">/t:node[t:id='zzz']</ref>`
	for _, tc := range []struct {
		in   string
		want bool
	}{{ok, true}, {missing, false}} {
		tree, err := datatree.Parse(mod, datatree.FormatXML, []byte(tc.in))
		if err != nil {
			t.Fatalf("Parse: %v", err)
		}
		if err := tree.Validate(); (err == nil) != tc.want {
			t.Fatalf("Validate(%s) = %v, want valid=%v", tc.in, err, tc.want)
		}
	}
	// JSON: a module-name qualifier for a module the leaf's module does not
	// import by that name still resolves.
	if err := validateOne(t, mod, `{"iid:top":{"iida:extra":"e","iida:ptr2":"/iid:top/iida:extra"}}`); err != nil {
		t.Fatalf("existing cross-module target should be valid: %v", err)
	}
	err := validateOne(t, mod, `{"iid:top":{"iida:ptr2":"/iid:top/iida:extra"}}`)
	if err == nil || !strings.Contains(err.Error(), "non-existent instance") {
		t.Fatalf("missing cross-module target should be reported, got %v", err)
	}
}

// Element text escapes only what XML requires (&, <, and > for readability),
// as libyang prints it, so quotes inside values (instance-identifier
// predicates, strings) are emitted byte-for-byte like the backend.
func TestXMLTextEscapesLikeLibyang(t *testing.T) {
	mod := loadModSrc(t, `module esc { namespace "urn:esc"; prefix esc; leaf s { type string; } }`, "esc")
	j, x := serializeBoth(t, mod, datatree.FormatJSONIETF, `{"esc:s":"say \"hi\", it's <b> & c"}`)
	if want := `<s xmlns="urn:esc">say "hi", it's &lt;b&gt; &amp; c</s>`; x != want {
		t.Fatalf("XML:\n got: %s\nwant: %s", x, want)
	}
	back, _ := serializeBoth(t, mod, datatree.FormatXML, x)
	if back != j {
		t.Fatalf("XML round trip:\n got: %s\nwant: %s", back, j)
	}
}

// Overlong numeric text is rejected by length before any math/big parsing
// (a multi-megabyte digit string used to cost seconds of CPU), and the
// violation does not echo the digits back.
func TestOverlongNumericValuesRejectedByLength(t *testing.T) {
	mod := loadModSrc(t, canonicalValueSchema, "cv")
	digits := strings.Repeat("9", 200000)
	cases := []struct {
		name string
		f    datatree.Format
		in   string
	}{
		{"int32 JSON", datatree.FormatJSONIETF, `{"cv:top":{"i32":` + digits + `}}`},
		{"int64 JSON", datatree.FormatJSONIETF, `{"cv:top":{"i64":"` + digits + `"}}`},
		{"int32 XML", datatree.FormatXML, `<top xmlns="urn:cv"><i32>` + digits + `</i32></top>`},
		{"uint64 XML", datatree.FormatXML, `<top xmlns="urn:cv"><u64>` + digits + `</u64></top>`},
		{"decimal64 JSON", datatree.FormatJSONIETF, `{"cv:top":{"d2":"` + digits + `.5"}}`},
		{"decimal64 XML", datatree.FormatXML, `<top xmlns="urn:cv"><d2>` + digits + `.5</d2></top>`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tree, err := datatree.Parse(mod, tc.f, []byte(tc.in))
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			err = tree.Validate()
			if err == nil {
				t.Fatal("overlong numeric value must be invalid")
			}
			msg := err.Error()
			if !strings.Contains(msg, "longer than 256 characters") {
				t.Fatalf("want a length violation, got %.200s", msg)
			}
			if len(msg) > 1000 {
				t.Fatalf("violation echoes the value (%d bytes)", len(msg))
			}
		})
	}
	// A leading-zero-padded but otherwise small value under the bound is fine.
	if err := validateOne(t, mod, `{"cv:top":{"i64":"`+strings.Repeat("0", 200)+`7"}}`); err != nil {
		t.Fatalf("zero-padded int64 within the bound must be valid: %v", err)
	}
}

func TestCanonicalDefaultValue(t *testing.T) {
	mod := loadModSrc(t, canonicalValueSchema, "cv")
	tree, err := datatree.Parse(mod, datatree.FormatJSONIETF, []byte(`{"cv:top":{}}`))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	tree.ApplyDefaults()
	out, err := tree.Serialize(datatree.FormatJSONIETF)
	if err != nil {
		t.Fatalf("Serialize: %v", err)
	}
	if want := `{"cv:top":{"dd":"2.5"}}`; string(out) != want {
		t.Fatalf("default not canonical:\n got: %s\nwant: %s", out, want)
	}
}
