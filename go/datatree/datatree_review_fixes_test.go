package datatree_test

import (
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
