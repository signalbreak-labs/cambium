//go:build cgo

package libyangbackend_test

import (
	"os"
	"path/filepath"
	"testing"

	cambium "github.com/signalbreak-labs/cambium/go/libyangbackend"
)

// TestOpaqueNodesExcludedFromChildren asserts that opaque / schema-less nodes
// (produced only under Opaque parse mode) do not surface in Children() /
// RootNodes(), matching the Rust adapter which gates the walk on the node having
// a schema name. Without the gate, identical input + API yields a different node
// set across the two SDKs — and an opaque NodeRef cannot round-trip through
// lyd_find_path, so its accessors would all error.
func TestOpaqueNodesExcludedFromChildren(t *testing.T) {
	dir := filepath.Join("..", "..", "target", "tests", "opaque-exclude", "modules")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	const modName = "opaque-demo"
	yang := `module opaque-demo {
    namespace "urn:cambium:opaque-demo";
    prefix od;
    yang-version 1.1;
    revision 2026-06-14;
    container top {
        leaf known { type string; }
    }
}
`
	if err := os.WriteFile(filepath.Join(dir, modName+".yang"), []byte(yang), 0o644); err != nil {
		t.Fatal(err)
	}

	ctx, err := cambium.NewContext()
	if err != nil {
		t.Fatal(err)
	}
	defer ctx.Close()
	if err := ctx.SetSearchPath(dir); err != nil {
		t.Fatal(err)
	}
	if err := ctx.LoadModule(modName); err != nil {
		t.Fatal(err)
	}

	// <mystery> is not in top's schema. Under ParseOnly+Opaque, validation (which
	// would reject the unknown child) is skipped and libyang keeps it as an
	// opaque, schema-less child of top.
	input := []byte(`<top xmlns="urn:cambium:opaque-demo"><known>x</known><mystery>?</mystery></top>`)
	tree, err := ctx.Parse(cambium.FormatXML, cambium.ParseMode{Opaque: true, ParseOnly: true}, input)
	if err != nil {
		t.Fatal(err)
	}
	defer tree.Close()

	top, err := tree.Get("/opaque-demo:top")
	if err != nil {
		t.Fatal(err)
	}
	children, err := top.Children()
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, c := range children.Iter() {
		name, err := c.Name()
		if err != nil {
			t.Fatal(err)
		}
		names = append(names, name)
	}
	// The opaque "mystery" child must be excluded (Rust gates on a schema name);
	// only the schema-known "known" leaf remains.
	if len(names) != 1 || names[0] != "known" {
		t.Fatalf("Children(top) = %v, want [known] (opaque <mystery> must be excluded)", names)
	}
}
