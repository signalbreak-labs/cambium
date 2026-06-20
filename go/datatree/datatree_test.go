package datatree_test

import (
	"strings"
	"testing"

	"github.com/signalbreak-labs/cambium/go/cambium"
	"github.com/signalbreak-labs/cambium/go/datatree"
)

// dtSchema declares children in a deliberately non-alphabetical, key-last order
// so the round-trip proves the output is driven by schema declaration order
// (I2) with list keys emitted first (I3): container c is z, m, a; list item
// declares leaf name before its key leaf id.
const dtSchema = `module dt {
    namespace "urn:dt";
    prefix dt;

    container c {
        leaf z { type string; }
        leaf m { type int32; }
        leaf a { type boolean; }
    }

    leaf-list tags { type string; }

    list item {
        key "id";
        leaf name { type string; }
        leaf id { type string; }
    }
}`

func loadDT(t *testing.T) cambium.Module {
	t.Helper()
	b, err := cambium.NewContextBuilder(cambium.ContextFlags{})
	if err != nil {
		t.Fatal(err)
	}
	if err := b.LoadModuleStr(dtSchema); err != nil {
		t.Fatalf("LoadModuleStr: %v", err)
	}
	ctx, err := b.Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	t.Cleanup(func() { ctx.Close() })
	mod, err := ctx.Schema("dt")
	if err != nil {
		t.Fatalf("Schema: %v", err)
	}
	return mod
}

func TestRoundTripOrdersBySchema(t *testing.T) {
	mod := loadDT(t)
	// Input members are scrambled at every level: top-level item/tags/c (schema
	// is c/tags/item); container a/z/m (schema z/m/a); entry name/id (key id).
	in := `{"dt:item":[{"name":"x","id":"1"}],"dt:tags":["t1","t2"],"dt:c":{"a":true,"z":"hi","m":7}}`
	tree, err := datatree.Parse(mod, datatree.FormatJSONIETF, []byte(in))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	out, err := tree.Serialize(datatree.FormatJSONIETF)
	if err != nil {
		t.Fatalf("Serialize: %v", err)
	}
	want := `{"dt:c":{"z":"hi","m":7,"a":true},"dt:tags":["t1","t2"],"dt:item":[{"id":"1","name":"x"}]}`
	if got := string(out); got != want {
		t.Fatalf("serialized order mismatch:\n got: %s\nwant: %s", got, want)
	}
}

func TestRoundTripStable(t *testing.T) {
	mod := loadDT(t)
	in := `{"dt:c":{"z":"hi","m":7,"a":true},"dt:tags":["t1","t2"],"dt:item":[{"id":"1","name":"x"}]}`
	first, err := datatree.Parse(mod, datatree.FormatJSONIETF, []byte(in))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	out1, err := first.Serialize(datatree.FormatJSONIETF)
	if err != nil {
		t.Fatalf("Serialize: %v", err)
	}
	// Re-parsing the output and serializing again must be byte-identical.
	second, err := datatree.Parse(mod, datatree.FormatJSONIETF, out1)
	if err != nil {
		t.Fatalf("re-Parse: %v", err)
	}
	out2, err := second.Serialize(datatree.FormatJSONIETF)
	if err != nil {
		t.Fatalf("re-Serialize: %v", err)
	}
	if string(out1) != string(out2) {
		t.Fatalf("round-trip not stable:\n out1: %s\n out2: %s", out1, out2)
	}
}

func TestUnknownMemberRejected(t *testing.T) {
	mod := loadDT(t)
	in := `{"dt:c":{"z":"hi"},"dt:bogus":1}`
	_, err := datatree.Parse(mod, datatree.FormatJSONIETF, []byte(in))
	if err == nil || !strings.Contains(err.Error(), "unknown member") {
		t.Fatalf("expected unknown-member error, got %v", err)
	}
}
