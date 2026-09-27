// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

package datatree_test

import (
	"testing"

	"github.com/signalbreak-labs/cambium/go/cambium"
	"github.com/signalbreak-labs/cambium/go/datatree"
)

// serializeBoth parses in (in the given format) and returns the JSON_IETF and
// XML serializations, failing the test on any error.
func serializeBoth(t *testing.T, mod cambium.Module, f datatree.Format, in string) (jsonOut, xmlOut string) {
	t.Helper()
	tree, err := datatree.Parse(mod, f, []byte(in))
	if err != nil {
		t.Fatalf("Parse(%s): %v", in, err)
	}
	j, err := tree.Serialize(datatree.FormatJSONIETF)
	if err != nil {
		t.Fatalf("Serialize JSON: %v", err)
	}
	x, err := tree.Serialize(datatree.FormatXML)
	if err != nil {
		t.Fatalf("Serialize XML: %v", err)
	}
	return string(j), string(x)
}

// A list key is identified by its qualified (module, name) identity, so a leaf
// augmented into the list from another module that happens to share a key's
// local name is neither sorted into the key group (I3) nor read as the key.
const keyNameCollisionBase = `module kc {
    yang-version 1.1;
    namespace "urn:kc"; prefix kc;
    list l {
        key name;
        leaf other { type string; }
        leaf name { type string; }
    }
}`

const keyNameCollisionAug = `module kc2 {
    yang-version 1.1;
    namespace "urn:kc2"; prefix kc2;
    import kc { prefix kc; }
    augment /kc:l { leaf name { type string; } }
}`

func TestKeysFirstUsesQualifiedKeyIdentity(t *testing.T) {
	mod := loadMultiModSrc(t, "kc", keyNameCollisionBase, keyNameCollisionAug)
	in := `{"kc:l":[{"kc2:name":"x","other":"o","name":"1"}]}`
	j, x := serializeBoth(t, mod, datatree.FormatJSONIETF, in)
	if want := `{"kc:l":[{"name":"1","other":"o","kc2:name":"x"}]}`; j != want {
		t.Fatalf("JSON keys-first mismatch:\n got: %s\nwant: %s", j, want)
	}
	if want := `<l xmlns="urn:kc"><name>1</name><other>o</other><name xmlns="urn:kc2">x</name></l>`; x != want {
		t.Fatalf("XML keys-first mismatch:\n got: %s\nwant: %s", x, want)
	}
}

func TestListKeyUniquenessUsesQualifiedKeyIdentity(t *testing.T) {
	mod := loadMultiModSrc(t, "kc", keyNameCollisionBase, keyNameCollisionAug)
	in := `{"kc:l":[{"name":"1","other":"o","kc2:name":"same"},{"name":"2","other":"o","kc2:name":"same"}]}`
	if err := validateOne(t, mod, in); err != nil {
		t.Fatalf("distinct keys with an equal same-named augmented leaf must be valid, got %v", err)
	}
	dup := `{"kc:l":[{"name":"1","kc2:name":"a"},{"name":"1","kc2:name":"b"}]}`
	if err := validateOne(t, mod, dup); err == nil {
		t.Fatal("equal keys must be reported as a duplicate key")
	}
	missing := `{"kc:l":[{"other":"o","kc2:name":"a"}]}`
	if err := validateOne(t, mod, missing); err == nil {
		t.Fatal("an entry whose only 'name' is the augmented leaf is missing its key")
	}
}

// systemOrderedSchema covers every built-in type libyang knows how to order for
// ordered-by system lists and leaf-lists (RFC 7950 §7.7.7, invariant I2): the
// canonical order is by value per type, never by raw text.
const systemOrderedSchema = `module so {
    yang-version 1.1;
    namespace "urn:so"; prefix so;
    identity base;
    identity zeta { base base; }
    identity alpha { base base; }
    container top {
        leaf-list u16 { type uint16; }
        leaf-list s { type string; }
        leaf-list i8 { type int8; }
        leaf-list i64 { type int64; }
        leaf-list u64 { type uint64; }
        leaf-list d { type decimal64 { fraction-digits 2; } }
        leaf-list e { type enumeration { enum z { value 1; } enum a { value 2; } enum m { value -5; } } }
        leaf-list b { type boolean; }
        leaf-list bin { type binary; }
        leaf-list bits { type bits { bit hi { position 9; } bit lo { position 0; } bit mid { position 3; } } }
        leaf-list id { type identityref { base base; } }
        leaf-list un { type union { type int8; type string; } }
        leaf-list lr { type leafref { path "../u16"; } }
        leaf-list usr { ordered-by user; type uint16; }
        list vlan { key "id"; leaf name { type string; } leaf id { type uint16; } }
        list pair { key "a b"; leaf a { type string; } leaf b { type int32; } }
        list usrl { key k; ordered-by user; leaf k { type uint8; } }
    }
    container state {
        config false;
        leaf-list v { type uint16; }
        list st { key k; leaf k { type uint8; } }
    }
}`

const systemOrderedForeign = `module so2 {
    yang-version 1.1;
    namespace "urn:so2"; prefix so2;
    import so { prefix so; }
    identity beta { base so:base; }
}`

func TestSystemOrderedLeafListsAndListsAreCanonicalJSON(t *testing.T) {
	mod := loadMultiModSrc(t, "so", systemOrderedSchema, systemOrderedForeign)
	in := `{"so:top":{` +
		`"u16":[300,20,1000,3],` +
		`"s":["b","B","a","aa"],` +
		`"i8":[5,-3,0],` +
		`"i64":["10","-20","3"],` +
		`"u64":["18446744073709551615","2","10"],` +
		`"d":["10.5","9.25","-1.0"],` +
		`"e":["a","z","m"],` +
		`"b":[true,false],` +
		`"bin":["AAAA","/w==","AA=="],` +
		`"bits":["mid","lo hi","lo","hi"],` +
		`"id":["zeta","so2:beta","alpha"],` +
		`"un":["x",5,"a",-1],` +
		`"lr":[20,3],` +
		`"usr":[30,10,20],` +
		`"vlan":[{"id":300,"name":"c"},{"id":20,"name":"a"},{"id":1000,"name":"b"}],` +
		`"pair":[{"a":"x","b":10},{"a":"a","b":2},{"a":"x","b":9}],` +
		`"usrl":[{"k":3},{"k":1},{"k":2}]` +
		`},"so:state":{"v":[30,10,20],"st":[{"k":3},{"k":1},{"k":2}]}}`
	// Per libyang's type plugins: numbers numerically, strings and identity
	// names bytewise, enums by assigned value, binary by decoded length then
	// bytes, bits by position bitmap, and a union's later member types first.
	want := `{"so:top":{` +
		`"u16":[3,20,300,1000],` +
		`"s":["B","a","aa","b"],` +
		`"i8":[-3,0,5],` +
		`"i64":["-20","3","10"],` +
		`"u64":["2","10","18446744073709551615"],` +
		`"d":["-1.0","9.25","10.5"],` +
		`"e":["m","z","a"],` +
		`"b":[false,true],` +
		`"bin":["AA==","/w==","AAAA"],` +
		`"bits":["hi","lo","lo hi","mid"],` +
		`"id":["alpha","so2:beta","zeta"],` +
		`"un":["a","x",-1,5],` +
		`"lr":[3,20],` +
		`"usr":[30,10,20],` +
		`"vlan":[{"id":20,"name":"a"},{"id":300,"name":"c"},{"id":1000,"name":"b"}],` +
		`"pair":[{"a":"a","b":2},{"a":"x","b":9},{"a":"x","b":10}],` +
		`"usrl":[{"k":3},{"k":1},{"k":2}]` +
		`},"so:state":{"v":[30,10,20],"st":[{"k":3},{"k":1},{"k":2}]}}`
	j, _ := serializeBoth(t, mod, datatree.FormatJSONIETF, in)
	if j != want {
		t.Fatalf("ordered-by system canonical order mismatch:\n got: %s\nwant: %s", j, want)
	}
}

func TestSystemOrderedCanonicalFromXMLInput(t *testing.T) {
	mod := loadMultiModSrc(t, "so", systemOrderedSchema, systemOrderedForeign)
	// Entries/values interleave with other elements and arrive out of order.
	in := `<top xmlns="urn:so">` +
		`<vlan><id>300</id><name>c</name></vlan>` +
		`<u16>300</u16><usr>30</usr><u16>20</u16>` +
		`<vlan><name>a</name><id>20</id></vlan>` +
		`<usr>10</usr><u16>1000</u16><usr>20</usr>` +
		`<vlan><id>1000</id><name>b</name></vlan>` +
		`</top>`
	wantXML := `<top xmlns="urn:so">` +
		`<u16>20</u16><u16>300</u16><u16>1000</u16>` +
		`<usr>30</usr><usr>10</usr><usr>20</usr>` +
		`<vlan><id>20</id><name>a</name></vlan>` +
		`<vlan><id>300</id><name>c</name></vlan>` +
		`<vlan><id>1000</id><name>b</name></vlan>` +
		`</top>`
	wantJSON := `{"so:top":{"u16":[20,300,1000],"usr":[30,10,20],` +
		`"vlan":[{"id":20,"name":"a"},{"id":300,"name":"c"},{"id":1000,"name":"b"}]}}`
	j, x := serializeBoth(t, mod, datatree.FormatXML, in)
	if x != wantXML {
		t.Fatalf("XML canonical order mismatch:\n got: %s\nwant: %s", x, wantXML)
	}
	if j != wantJSON {
		t.Fatalf("JSON canonical order mismatch:\n got: %s\nwant: %s", j, wantJSON)
	}
}

func TestSystemOrderedNavigationSeesCanonicalOrder(t *testing.T) {
	mod := loadMultiModSrc(t, "so", systemOrderedSchema, systemOrderedForeign)
	tree, err := datatree.Parse(mod, datatree.FormatJSONIETF, []byte(`{"so:top":{"u16":[30,10,20]}}`))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	got := tree.RootNodes()[0].Children()[0].LeafListValues()
	if len(got) != 3 || got[0] != "10" || got[1] != "20" || got[2] != "30" {
		t.Fatalf("LeafListValues = %v, want canonical [10 20 30]", got)
	}
}
