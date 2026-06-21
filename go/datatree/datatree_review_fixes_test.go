package datatree_test

import (
	"strings"
	"testing"

	"github.com/signalbreak-labs/cambium/go/cambium"
	"github.com/signalbreak-labs/cambium/go/datatree"
)

func loadModSrc(t *testing.T, src, name string) cambium.Module {
	t.Helper()
	b, err := cambium.NewContextBuilder(cambium.ContextFlags{})
	if err != nil {
		t.Fatal(err)
	}
	if err := b.LoadModuleStr(src); err != nil {
		t.Fatalf("LoadModuleStr: %v", err)
	}
	ctx, err := b.Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	t.Cleanup(func() { ctx.Close() })
	mod, err := ctx.Schema(name)
	if err != nil {
		t.Fatalf("Schema: %v", err)
	}
	return mod
}

func validateOne(t *testing.T, mod cambium.Module, in string) error {
	t.Helper()
	tree, err := datatree.Parse(mod, datatree.FormatJSONIETF, []byte(in))
	if err != nil {
		t.Fatalf("Parse(%s): %v", in, err)
	}
	return tree.Validate()
}

func loadMultiModSrc(t *testing.T, name string, srcs ...string) cambium.Module {
	t.Helper()
	b, err := cambium.NewContextBuilder(cambium.ContextFlags{})
	if err != nil {
		t.Fatal(err)
	}
	for _, src := range srcs {
		if err := b.LoadModuleStr(src); err != nil {
			t.Fatalf("LoadModuleStr: %v", err)
		}
	}
	ctx, err := b.Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	t.Cleanup(func() { ctx.Close() })
	mod, err := ctx.Schema(name)
	if err != nil {
		t.Fatalf("Schema: %v", err)
	}
	return mod
}

// Finding 1: length "1..max" / "min..N" must not false-reject (the "min"/"max"
// keywords mean unbounded, not the literal strings).
func TestLengthMinMaxKeywords(t *testing.T) {
	mod := loadModSrc(t, `module ml {
        namespace "urn:ml"; prefix ml;
        leaf s { type string { length "1..max"; } }
        leaf t { type string { length "min..4"; } }
    }`, "ml")
	if err := validateOne(t, mod, `{"ml:s":"hello"}`); err != nil {
		t.Fatalf(`length "1..max" should accept "hello": %v`, err)
	}
	if err := validateOne(t, mod, `{"ml:t":"ab"}`); err != nil {
		t.Fatalf(`length "min..4" should accept "ab": %v`, err)
	}
	if err := validateOne(t, mod, `{"ml:t":"abcde"}`); err == nil {
		t.Fatalf(`length "min..4" should reject "abcde"`)
	}
}

// Finding 2: decimal64 range "0..max" must not false-reject.
func TestDecimalMaxKeyword(t *testing.T) {
	mod := loadModSrc(t, `module md {
        namespace "urn:md"; prefix md;
        leaf d { type decimal64 { fraction-digits 2; range "0..max"; } }
    }`, "md")
	if err := validateOne(t, mod, `{"md:d":"1.50"}`); err != nil {
		t.Fatalf(`range "0..max" should accept 1.50: %v`, err)
	}
	if err := validateOne(t, mod, `{"md:d":"-1.0"}`); err == nil {
		t.Fatalf(`range "0..max" should reject -1.0`)
	}
}

// Finding 3: decimal64 must reject lexical forms big.Rat would accept but YANG
// does not.
func TestDecimalLexicalRejected(t *testing.T) {
	mod := loadModSrc(t, `module md2 {
        namespace "urn:md2"; prefix md2;
        leaf d { type decimal64 { fraction-digits 3; } }
    }`, "md2")
	for _, bad := range []string{`"0x10"`, `"1.5e1"`, `"1/2"`, `"1_000"`} {
		if err := validateOne(t, mod, `{"md2:d":`+bad+`}`); err == nil {
			t.Fatalf("decimal64 should reject %s", bad)
		}
	}
	if err := validateOne(t, mod, `{"md2:d":"3.14"}`); err != nil {
		t.Fatalf("decimal64 should accept 3.14: %v", err)
	}
}

// Finding 4: a leaf inside a top-level choice must be reachable (parse, find,
// validate), matching nested-level choice flattening.
func TestTopLevelChoiceFlattened(t *testing.T) {
	mod := loadModSrc(t, `module mc {
	        namespace "urn:mc"; prefix mc;
	        choice ch {
            leaf a { type string; }
            leaf b { type string; }
        }
    }`, "mc")
	tree, err := datatree.Parse(mod, datatree.FormatJSONIETF, []byte(`{"mc:a":"x"}`))
	if err != nil {
		t.Fatalf("leaf in top-level choice should parse: %v", err)
	}
	n, ok := tree.Find("/a")
	if !ok || !n.IsLeaf() {
		t.Fatalf("Find /a in top-level choice: ok=%v", ok)
	}
	if err := tree.Validate(); err != nil {
		t.Fatalf("validate top-level choice tree: %v", err)
	}
}

func TestJSONRejectsNullAndDuplicateStructuralValues(t *testing.T) {
	mod := loadDT(t)
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"root null", `null`, "expected JSON object"},
		{"container null", `{"dt:c":null}`, "expected JSON object"},
		{"leaf-list null", `{"dt:tags":null}`, "expected JSON array"},
		{"list null", `{"dt:item":null}`, "expected JSON array"},
		{"list entry null", `{"dt:item":[null]}`, "expected JSON object"},
		{"duplicate member", `{"dt:c":{},"dt:c":{"z":"last"}}`, "duplicate member"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := datatree.Parse(mod, datatree.FormatJSONIETF, []byte(tc.in))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("expected parse error containing %q, got %v", tc.want, err)
			}
		})
	}
}

func TestJSONIETFIntegerEncodingShapes(t *testing.T) {
	mod := loadTC(t)
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"int32 must be number", `{"tc:n":"50"}`, "JSON number"},
		{"uint64 must be string", `{"tc:big":123}`, "JSON string"},
		{"union numeric member must be number", `{"tc:u":"5"}`, "union member"},
		{"decimal64 rejects surrounding whitespace", `{"tc:d":" 1.50 "}`, "valid decimal64"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tree, err := datatree.Parse(mod, datatree.FormatJSONIETF, []byte(tc.in))
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			err = tree.Validate()
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("expected validation error containing %q, got %v", tc.want, err)
			}
		})
	}
}

func TestXMLRejectsNamespaceAndStructuralViolations(t *testing.T) {
	mod := loadDT(t)
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"wrong namespace", `<c xmlns="urn:wrong"><z>hi</z></c>`, "unknown XML element"},
		{"leaf has child", `<c xmlns="urn:dt"><z><bad/></z></c>`, "leaf"},
		{"container has text", `<c xmlns="urn:dt">junk<z>hi</z></c>`, "non-whitespace text"},
		{"duplicate singleton", `<c xmlns="urn:dt"><z>one</z><z>two</z></c>`, "duplicate XML element"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := datatree.Parse(mod, datatree.FormatXML, []byte(tc.in))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("expected parse error containing %q, got %v", tc.want, err)
			}
		})
	}
}

func TestXMLPreservesStringWhitespace(t *testing.T) {
	mod := loadDT(t)
	tree, err := datatree.Parse(mod, datatree.FormatXML, []byte(`<c xmlns="urn:dt"><z>  hi  </z></c>`))
	if err != nil {
		t.Fatalf("Parse XML: %v", err)
	}
	out, err := tree.Serialize(datatree.FormatJSONIETF)
	if err != nil {
		t.Fatalf("Serialize JSON: %v", err)
	}
	want := `{"dt:c":{"z":"  hi  "}}`
	if got := string(out); got != want {
		t.Fatalf("XML string whitespace was not preserved:\n got: %s\nwant: %s", got, want)
	}
}

func TestIdentityRefBareValueMustBeSameModule(t *testing.T) {
	base := `module idbase {
	    namespace "urn:idbase"; prefix ib;
	    identity base;
	    leaf ref { type identityref { base base; } }
	}`
	ext := `module idext {
	    namespace "urn:idext"; prefix ie;
	    import idbase { prefix ib; }
	    identity foreign { base ib:base; }
	}`
	mod := loadMultiModSrc(t, "idbase", base, ext)
	if err := validateOne(t, mod, `{"idbase:ref":"idext:foreign"}`); err != nil {
		t.Fatalf("qualified derived identity should be valid: %v", err)
	}
	if err := validateOne(t, mod, `{"idbase:ref":"foreign"}`); err == nil {
		t.Fatalf("bare cross-module identity should be rejected")
	}
}

func TestLeafListDuplicateValuesRejected(t *testing.T) {
	mod := loadDV(t)
	err := validateInput(t, mod, `{"dv:req":"x","dv:tags":["a","a"]}`)
	if err == nil || !strings.Contains(err.Error(), "duplicate leaf-list value") {
		t.Fatalf("expected duplicate leaf-list value violation, got %v", err)
	}
}
