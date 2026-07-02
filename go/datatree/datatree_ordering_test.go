// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

package datatree_test

import (
	"strings"
	"testing"

	"github.com/signalbreak-labs/cambium/go/cambium"
	"github.com/signalbreak-labs/cambium/go/datatree"
)

const dtUserOrderedSchema = `module dtuser {
    namespace "urn:dtuser";
    prefix dtuser;

    list item {
        key "id";
        ordered-by user;
        leaf name { type string; }
        leaf id { type string; }
    }
}`

func loadDTUserOrdered(t *testing.T) cambium.Module {
	t.Helper()
	b, err := cambium.NewContextBuilder(cambium.ContextFlags{})
	if err != nil {
		t.Fatal(err)
	}
	if err := b.LoadModuleStr(dtUserOrderedSchema); err != nil {
		t.Fatalf("LoadModuleStr: %v", err)
	}
	ctx, err := b.Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	t.Cleanup(func() { ctx.Close() })
	mod, err := ctx.Schema("dtuser")
	if err != nil {
		t.Fatalf("Schema: %v", err)
	}
	return mod
}

func TestUserOrderedListRoundTripPreservesInsertionOrder(t *testing.T) {
	mod := loadDTUserOrdered(t)
	in := `{"dtuser:item":[{"id":"b","name":"B"},{"id":"a","name":"A"},{"id":"c","name":"C"}]}`
	tree, err := datatree.Parse(mod, datatree.FormatJSONIETF, []byte(in))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	out, err := tree.Serialize(datatree.FormatJSONIETF)
	if err != nil {
		t.Fatalf("Serialize: %v", err)
	}
	s := string(out)
	bi := strings.Index(s, `"id":"b"`)
	ai := strings.Index(s, `"id":"a"`)
	ci := strings.Index(s, `"id":"c"`)
	if bi < 0 || ai < 0 || ci < 0 || bi >= ai || ai >= ci {
		t.Fatalf("ordered-by user list order mismatch; want b,a,c, got %s", s)
	}

	second, err := datatree.Parse(mod, datatree.FormatJSONIETF, out)
	if err != nil {
		t.Fatalf("re-Parse: %v", err)
	}
	out2, err := second.Serialize(datatree.FormatJSONIETF)
	if err != nil {
		t.Fatalf("re-Serialize: %v", err)
	}
	if string(out2) != s {
		t.Fatalf("round-trip not stable:\n out1: %s\n out2: %s", s, out2)
	}
}
