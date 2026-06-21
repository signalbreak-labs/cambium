// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

//go:build cgo

package libyangbackend_test

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	cambium "github.com/signalbreak-labs/cambium/go/libyangbackend"
)

// loadOrderDemo returns a parsed data tree from the scrambled-children fixture.
func loadOrderDemo(t *testing.T) (*cambium.Context, *cambium.DataTree) {
	t.Helper()
	conf := findConformance(t)
	moduleDir := filepath.Join(conf, "fixtures", "scrambled-children", "module")
	input, err := os.ReadFile(filepath.Join(conf, "fixtures", "scrambled-children", "input.xml"))
	if err != nil {
		t.Fatal(err)
	}
	ctx, err := cambium.NewContext()
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}
	if err := ctx.SetSearchPath(moduleDir); err != nil {
		ctx.Close()
		t.Fatalf("SetSearchPath: %v", err)
	}
	if err := ctx.LoadModule("order-demo"); err != nil {
		ctx.Close()
		t.Fatalf("LoadModule: %v", err)
	}
	tree, err := ctx.Parse(cambium.FormatXML, cambium.ParseModeDataOnly, input)
	if err != nil {
		ctx.Close()
		t.Fatalf("Parse: %v", err)
	}
	return ctx, tree
}

func TestContextNewDataEmptyTree(t *testing.T) {
	ctx, err := cambium.NewContext()
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}
	defer ctx.Close()

	tree := ctx.NewData()
	if tree == nil {
		t.Fatal("NewData returned nil")
	}
	defer tree.Close()

	// An empty tree has no root nodes.
	roots, err := tree.RootNodes()
	if err != nil {
		t.Fatalf("RootNodes: %v", err)
	}
	if !roots.IsEmpty() {
		t.Fatalf("RootNodes len = %d, want 0", roots.Len())
	}
}

func TestDataTreeGetTryGetExists(t *testing.T) {
	ctx, tree := loadOrderDemo(t)
	defer ctx.Close()
	defer tree.Close()

	// Get returns an existing node.
	root := "/order-demo:top"
	node, err := tree.Get(root)
	if err != nil {
		t.Fatalf("Get(%s): %v", root, err)
	}
	if name, _ := node.Name(); name != "top" {
		t.Fatalf("Get node name = %q, want top", name)
	}

	// Get on a missing path returns E0006.
	_, err = tree.Get("/order-demo:top/no-such")
	if err == nil {
		t.Fatal("Get on missing path should error")
	}
	if e, ok := err.(*cambium.Error); !ok || e.RuleCode() != cambium.RuleCodeDataPath {
		t.Fatalf("Get missing path code = %v, want E0006", err)
	}

	// TryGet returns the node when present.
	if n, ok := tree.TryGet(root); !ok || n == (cambium.NodeRef{}) {
		t.Fatal("TryGet existing path should return ok=true")
	}

	// TryGet on a missing path returns ok=false with no error.
	if _, ok := tree.TryGet("/order-demo:top/no-such"); ok {
		t.Fatal("TryGet missing path should return ok=false")
	}

	// Exists reports presence correctly.
	if !tree.Exists(root) {
		t.Fatalf("Exists(%s) should be true", root)
	}
	if tree.Exists("/order-demo:top/no-such") {
		t.Fatal("Exists on missing path should be false")
	}
}

func TestDataTreeRootNodesAndChildrenOrder(t *testing.T) {
	ctx, tree := loadOrderDemo(t)
	defer ctx.Close()
	defer tree.Close()

	roots, err := tree.RootNodes()
	if err != nil {
		t.Fatalf("RootNodes: %v", err)
	}
	if roots.Len() != 1 {
		t.Fatalf("RootNodes len = %d, want 1", roots.Len())
	}
	root, _ := roots.Get(0)
	if name, _ := root.Name(); name != "top" {
		t.Fatalf("root name = %q, want top", name)
	}

	children, err := root.Children()
	if err != nil {
		t.Fatalf("Children: %v", err)
	}
	want := []string{"z", "m", "a"}
	got := make([]string, children.Len())
	for i := 0; i < children.Len(); i++ {
		c, _ := children.Get(i)
		got[i], _ = c.Name()
	}
	if !slices.Equal(got, want) {
		t.Fatalf("children order = %v, want %v", got, want)
	}
}

func TestDataTreeSelectDocumentOrder(t *testing.T) {
	ctx, tree := loadOrderDemo(t)
	defer ctx.Close()
	defer tree.Close()

	set, err := tree.Select("//order-demo:top/*")
	if err != nil {
		t.Fatalf("Select: %v", err)
	}
	if set.Len() != 3 {
		t.Fatalf("Select len = %d, want 3", set.Len())
	}
	want := []string{"/order-demo:top/z", "/order-demo:top/m", "/order-demo:top/a"}
	for i := 0; i < set.Len(); i++ {
		node, _ := set.Get(i)
		path, _ := node.Path()
		if path != want[i] {
			t.Fatalf("Select[%d] path = %q, want %q", i, path, want[i])
		}
	}

	// Empty selection is not an error.
	empty, err := tree.Select("//order-demo:nothing")
	if err != nil {
		t.Fatalf("Select empty: %v", err)
	}
	if !empty.IsEmpty() {
		t.Fatal("Select empty should be empty")
	}
}

func TestNodeRefSchemaBridge(t *testing.T) {
	ctx, tree := loadOrderDemo(t)
	defer ctx.Close()
	defer tree.Close()

	node, err := tree.Get("/order-demo:top/order-demo:z")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	schema, err := node.Schema()
	if err != nil {
		t.Fatalf("Schema: %v", err)
	}
	if schema.Name() != "z" {
		t.Fatalf("schema name = %q, want z", schema.Name())
	}
	if schema.Kind() != cambium.SchemaNodeKindLeaf {
		t.Fatalf("schema kind = %v, want Leaf", schema.Kind())
	}
	info, ok := schema.LeafType()
	if !ok {
		t.Fatal("LeafType should be available")
	}
	if info.Base() != cambium.BaseTypeString {
		t.Fatalf("leaf base type = %v, want string", info.Base())
	}
}

func TestValueTypedLeaves(t *testing.T) {
	dir := t.TempDir()
	module := filepath.Join(dir, "typed-demo.yang")
	if err := os.WriteFile(module, []byte(`module typed-demo {
  namespace "urn:td";
  prefix td;
  container c {
    leaf i8 { type int8; }
    leaf u32 { type uint32; }
    leaf dec { type decimal64 { fraction-digits 2; } }
    leaf b { type boolean; }
    leaf e { type enumeration { enum one; enum two; } }
    leaf bits { type bits { bit alpha; bit beta; } }
    leaf bin { type binary; }
    leaf empty { type empty; }
  }
}`), 0o644); err != nil {
		t.Fatal(err)
	}
	data := []byte(`<c xmlns="urn:td">
  <i8>-5</i8>
  <u32>42</u32>
  <dec>-3.50</dec>
  <b>true</b>
  <e>two</e>
  <bits>alpha beta</bits>
  <bin>SGVsbG8=</bin>
  <empty/>
</c>`)

	ctx, err := cambium.NewContext()
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}
	defer ctx.Close()
	if err := ctx.SetSearchPath(dir); err != nil {
		t.Fatalf("SetSearchPath: %v", err)
	}
	if err := ctx.LoadModule("typed-demo"); err != nil {
		t.Fatalf("LoadModule: %v", err)
	}
	tree, err := ctx.Parse(cambium.FormatXML, cambium.ParseModeDataOnly, data)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	defer tree.Close()

	mustValue := func(path string) cambium.Value {
		t.Helper()
		node, err := tree.Get(path)
		if err != nil {
			t.Fatalf("Get(%s): %v", path, err)
		}
		v, ok, err := node.Value()
		if err != nil {
			t.Fatalf("Value(%s): %v", path, err)
		}
		if !ok {
			t.Fatalf("Value(%s) returned ok=false", path)
		}
		return v
	}

	if v := mustValue("/typed-demo:c/i8"); v.Kind() != cambium.ValueInt8 {
		t.Fatalf("i8 kind = %v", v.Kind())
	} else if i, ok := v.Int8(); !ok || i != -5 {
		t.Fatalf("i8 = %d", i)
	}

	if v := mustValue("/typed-demo:c/u32"); v.Kind() != cambium.ValueUint32 {
		t.Fatalf("u32 kind = %v", v.Kind())
	} else if i, ok := v.Uint32(); !ok || i != 42 {
		t.Fatalf("u32 = %d", i)
	}

	if v := mustValue("/typed-demo:c/dec"); v.Kind() != cambium.ValueDecimal64 {
		t.Fatalf("dec kind = %v", v.Kind())
	} else if d, ok := v.Decimal64(); !ok {
		t.Fatal("Decimal64 accessor failed")
	} else if d.FractionDigits() != 2 || d.String() != "-3.5" || d.Raw() != -350 {
		t.Fatalf("decimal64 = %s raw=%d fd=%d", d.String(), d.Raw(), d.FractionDigits())
	}

	if v := mustValue("/typed-demo:c/b"); v.Kind() != cambium.ValueBool {
		t.Fatalf("b kind = %v", v.Kind())
	} else if b, ok := v.Bool(); !ok || !b {
		t.Fatalf("b = %v", b)
	}

	if v := mustValue("/typed-demo:c/e"); v.Kind() != cambium.ValueEnum {
		t.Fatalf("e kind = %v", v.Kind())
	} else if s, ok := v.Enum(); !ok || s != "two" {
		t.Fatalf("e = %q", s)
	}

	if v := mustValue("/typed-demo:c/bits"); v.Kind() != cambium.ValueBits {
		t.Fatalf("bits kind = %v", v.Kind())
	} else if bits, ok := v.Bits(); !ok || !slices.Equal(bits, []string{"alpha", "beta"}) {
		t.Fatalf("bits = %v", bits)
	}

	if v := mustValue("/typed-demo:c/bin"); v.Kind() != cambium.ValueBinary {
		t.Fatalf("bin kind = %v", v.Kind())
	} else if b, ok := v.Binary(); !ok || string(b) != "Hello" {
		t.Fatalf("bin = %q", string(b))
	}

	if v := mustValue("/typed-demo:c/empty"); v.Kind() != cambium.ValueEmpty || !v.Empty() {
		t.Fatalf("empty kind = %v empty=%v", v.Kind(), v.Empty())
	}

	// Non-term node returns ok=false with no error.
	cnode, err := tree.Get("/typed-demo:c")
	if err != nil {
		t.Fatalf("Get c: %v", err)
	}
	if _, ok, err := cnode.Value(); err != nil || ok {
		t.Fatalf("Value on container should be ok=false, err=nil; got ok=%v err=%v", ok, err)
	}
}

func TestDecimal64Canonical(t *testing.T) {
	cases := []struct {
		raw  int64
		fd   uint8
		want string
	}{
		{12345, 3, "12.345"},
		{-12345, 3, "-12.345"},
		{1000, 3, "1.0"},
		{-1000, 3, "-1.0"},
		{0, 2, "0.0"},
		{5, 1, "0.5"},
		{-5, 1, "-0.5"},
	}
	for _, tc := range cases {
		d := cambium.NewDecimal64(tc.raw, tc.fd)
		if got := d.String(); got != tc.want {
			t.Errorf("Decimal64(%d,%d).String() = %q, want %q", tc.raw, tc.fd, got, tc.want)
		}
	}
}

func TestNodeRefSiblings(t *testing.T) {
	ctx, tree := loadOrderDemo(t)
	defer ctx.Close()
	defer tree.Close()

	z, err := tree.Get("/order-demo:top/order-demo:z")
	if err != nil {
		t.Fatalf("Get z leaf: %v", err)
	}
	sibs, err := z.Siblings()
	if err != nil {
		t.Fatalf("Siblings: %v", err)
	}
	want := []string{"z", "m", "a"}
	got := make([]string, sibs.Len())
	for i := 0; i < sibs.Len(); i++ {
		c, _ := sibs.Get(i)
		got[i], _ = c.Name()
	}
	if !slices.Equal(got, want) {
		t.Fatalf("siblings names = %v, want %v", got, want)
	}
}

func TestNodeRefValueStrAndIsDefault(t *testing.T) {
	dir := t.TempDir()
	module := filepath.Join(dir, "default-demo.yang")
	if err := os.WriteFile(module, []byte(`module default-demo {
  namespace "urn:dd";
  prefix dd;
  container c { leaf x { type string; default "hello"; } }
}`), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx, err := cambium.NewContext()
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}
	defer ctx.Close()
	if err := ctx.SetSearchPath(dir); err != nil {
		t.Fatalf("SetSearchPath: %v", err)
	}
	if err := ctx.LoadModule("default-demo"); err != nil {
		t.Fatalf("LoadModule: %v", err)
	}
	// Parse with defaults materialized.
	tree, err := ctx.Parse(cambium.FormatXML, cambium.ParseMode{}, []byte(`<c xmlns="urn:dd"/>`))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	defer tree.Close()

	node, err := tree.Get("/default-demo:c/x")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	s, ok, err := node.ValueStr()
	if err != nil || !ok || s != "hello" {
		t.Fatalf("ValueStr = %q ok=%v err=%v", s, ok, err)
	}
	def, err := node.IsDefault()
	if err != nil || !def {
		t.Fatalf("IsDefault = %v err=%v", def, err)
	}
}

func TestNodeRefPathReturnsStablePath(t *testing.T) {
	ctx, tree := loadOrderDemo(t)
	defer ctx.Close()
	defer tree.Close()

	node, err := tree.Get("/order-demo:top/order-demo:z")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	path, err := node.Path()
	if err != nil {
		t.Fatalf("Path: %v", err)
	}
	want := "/order-demo:top/order-demo:z"
	if path != want {
		t.Fatalf("Path = %q, want %q", path, want)
	}
}
