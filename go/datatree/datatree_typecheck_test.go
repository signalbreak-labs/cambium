package datatree_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/signalbreak-labs/cambium/go/cambium"
	"github.com/signalbreak-labs/cambium/go/datatree"
)

const tcSchema = `module tc {
    namespace "urn:tc";
    prefix tc;

    leaf s   { type string { length "2..4"; pattern "[a-z]+"; } }
    leaf n   { type int32 { range "1..100"; } }
    leaf big { type uint64; }
    leaf d   { type decimal64 { fraction-digits 2; range "0.0 .. 9.99"; } }
    leaf b   { type boolean; }
    leaf e   { type empty; }
    leaf col { type enumeration { enum red; enum green; } }
    leaf fl  { type bits { bit a; bit b; } }
    leaf bin { type binary { length "1..3"; } }
    leaf u   { type union { type uint8; type enumeration { enum auto; } } }

    leaf-list ll { type enumeration { enum on; enum off; } }
}`

func loadTC(t *testing.T) cambium.Module {
	t.Helper()
	b, err := cambium.NewContextBuilder(cambium.ContextFlags{})
	if err != nil {
		t.Fatal(err)
	}
	if err := b.LoadModuleStr(tcSchema); err != nil {
		t.Fatalf("LoadModuleStr: %v", err)
	}
	ctx, err := b.Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	t.Cleanup(func() { ctx.Close() })
	mod, err := ctx.Schema("tc")
	if err != nil {
		t.Fatalf("Schema: %v", err)
	}
	return mod
}

func TestValueTypeChecks(t *testing.T) {
	mod := loadTC(t)
	cases := []struct {
		leaf  string
		value string // raw JSON
		want  string // "" => valid; else substring expected in the violation
	}{
		// string: length 2..4, pattern [a-z]+
		{"s", `"abc"`, ""},
		{"s", `"x"`, "length"},
		{"s", `"abcde"`, "length"},
		{"s", `"ABC"`, "pattern"},
		// int32 range 1..100
		{"n", `50`, ""},
		{"n", `0`, "range"},
		{"n", `200`, "range"},
		// uint64 (JSON string), implicit width
		{"big", `"123"`, ""},
		{"big", `"18446744073709551615"`, ""},
		{"big", `"18446744073709551616"`, "base type range"},
		// decimal64 fraction-digits 2, range 0..9.99
		{"d", `"1.50"`, ""},
		{"d", `"1.555"`, "fraction digits"},
		{"d", `"10.0"`, "range"},
		// boolean / empty
		{"b", `true`, ""},
		{"b", `"yes"`, "true or false"},
		{"e", `[null]`, ""},
		{"e", `"x"`, "[null]"},
		// enumeration / bits membership
		{"col", `"red"`, ""},
		{"col", `"blue"`, "enum"},
		{"fl", `"a b"`, ""},
		{"fl", `"a a"`, "duplicate bit"},
		{"fl", `"a c"`, "not a defined bit"},
		// binary base64 + length 1..3
		{"bin", `"YWI="`, ""}, // "ab" => 2 bytes
		{"bin", `""`, "length"},
		// union: uint8 or enum{auto}
		{"u", `5`, ""},
		{"u", `"auto"`, ""},
		{"u", `"999"`, "union member"},
	}
	for _, tc := range cases {
		t.Run(fmt.Sprintf("%s=%s", tc.leaf, tc.value), func(t *testing.T) {
			in := fmt.Sprintf(`{"tc:%s":%s}`, tc.leaf, tc.value)
			tree, err := datatree.Parse(mod, datatree.FormatJSONIETF, []byte(in))
			if err != nil {
				t.Fatalf("Parse(%s): %v", in, err)
			}
			err = tree.Validate()
			if tc.want == "" {
				if err != nil {
					t.Fatalf("expected valid, got %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("expected violation containing %q, got %v", tc.want, err)
			}
		})
	}
}

func TestValueTypeChecksLeafList(t *testing.T) {
	mod := loadTC(t)
	// Second element is not a defined enum value.
	in := `{"tc:ll":["on","bad"]}`
	tree, err := datatree.Parse(mod, datatree.FormatJSONIETF, []byte(in))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	err = tree.Validate()
	if err == nil || !strings.Contains(err.Error(), "/ll[1]") || !strings.Contains(err.Error(), "enum") {
		t.Fatalf("expected enum violation at /ll[1], got %v", err)
	}
}
