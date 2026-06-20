//go:build cgo

package libyangbackend_test

import (
	"os"
	"path/filepath"
	"testing"

	cambium "github.com/signalbreak-labs/cambium/go/libyangbackend"
)

func TestDuplicateDeepCopyIndependent(t *testing.T) {
	ctx, original := loadCRUDContext(t)
	defer ctx.Close()
	defer original.Close()

	if _, err := original.NewPath("/cambium-data-crud-demo:top", nil, cambium.NewPathOpts{}); err != nil {
		t.Fatalf("NewPath top: %v", err)
	}
	counter := "1"
	if _, err := original.NewPath("/cambium-data-crud-demo:top/counter", &counter, cambium.NewPathOpts{}); err != nil {
		t.Fatalf("NewPath counter: %v", err)
	}
	if _, err := original.NewPath("/cambium-data-crud-demo:top/nested", nil, cambium.NewPathOpts{}); err != nil {
		t.Fatalf("NewPath nested: %v", err)
	}
	name := "inner"
	if _, err := original.NewPath("/cambium-data-crud-demo:top/nested/name", &name, cambium.NewPathOpts{}); err != nil {
		t.Fatalf("NewPath nested/name: %v", err)
	}

	copy, err := original.Duplicate()
	if err != nil {
		t.Fatalf("Duplicate: %v", err)
	}
	defer copy.Close()

	// Mutate copy; original must stay unchanged.
	if _, err := copy.SetValue("/cambium-data-crud-demo:top/counter", "99"); err != nil {
		t.Fatalf("SetValue copy counter: %v", err)
	}
	if err := copy.RemovePath("/cambium-data-crud-demo:top/nested"); err != nil {
		t.Fatalf("RemovePath copy nested: %v", err)
	}
	if _, err := copy.NewPath("/cambium-data-crud-demo:top/item[id='x']", nil, cambium.NewPathOpts{}); err != nil {
		t.Fatalf("NewPath copy item: %v", err)
	}

	node, err := original.Get("/cambium-data-crud-demo:top/counter")
	if err != nil {
		t.Fatalf("Get original counter: %v", err)
	}
	v, ok, err := node.Value()
	if err != nil || !ok {
		t.Fatalf("Value original counter: ok=%v err=%v", ok, err)
	}
	if u, ok := v.Uint64(); !ok || u != 1 {
		t.Fatalf("original counter = %v, want 1", u)
	}
	if !original.Exists("/cambium-data-crud-demo:top/nested") {
		t.Fatal("original nested should survive")
	}
	if original.Exists("/cambium-data-crud-demo:top/item[id='x']") {
		t.Fatal("original should not contain copy's new item")
	}

	// Mutate original; copy must stay unchanged.
	if _, err := original.SetValue("/cambium-data-crud-demo:top/counter", "42"); err != nil {
		t.Fatalf("SetValue original counter: %v", err)
	}
	copyNode, err := copy.Get("/cambium-data-crud-demo:top/counter")
	if err != nil {
		t.Fatalf("Get copy counter: %v", err)
	}
	copyV, ok, err := copyNode.Value()
	if err != nil || !ok {
		t.Fatalf("Value copy counter: ok=%v err=%v", ok, err)
	}
	if u, ok := copyV.Uint64(); !ok || u != 99 {
		t.Fatalf("copy counter = %v, want 99", u)
	}
	if copy.Exists("/cambium-data-crud-demo:top/nested") {
		t.Fatal("copy nested should be removed")
	}
	if !copy.Exists("/cambium-data-crud-demo:top/item[id='x']") {
		t.Fatal("copy item should survive")
	}
}

func TestDuplicatePreservesUserOrder(t *testing.T) {
	conf := findConformance(t)
	moduleDir := filepath.Join(conf, "fixtures", "ordered-user", "module")
	input, err := os.ReadFile(filepath.Join(conf, "fixtures", "ordered-user", "input.xml"))
	if err != nil {
		t.Fatal(err)
	}

	ctx, err := cambium.NewContext()
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}
	if err := ctx.SetSearchPath(moduleDir); err != nil {
		t.Fatalf("SetSearchPath: %v", err)
	}
	if err := ctx.LoadModule("ordered-user-demo"); err != nil {
		t.Fatalf("LoadModule: %v", err)
	}
	defer ctx.Close()

	original, err := ctx.Parse(cambium.FormatXML, cambium.ParseModeDataOnly, input)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	defer original.Close()

	copy, err := original.Duplicate()
	if err != nil {
		t.Fatalf("Duplicate: %v", err)
	}
	defer copy.Close()

	origJSON, err := original.Serialize(cambium.FormatJSON, cambium.DefaultSerializeFlags())
	if err != nil {
		t.Fatalf("Serialize original: %v", err)
	}
	copyJSON, err := copy.Serialize(cambium.FormatJSON, cambium.DefaultSerializeFlags())
	if err != nil {
		t.Fatalf("Serialize copy: %v", err)
	}
	if string(origJSON) != string(copyJSON) {
		t.Fatalf("user order diverged:\noriginal: %s\ncopy: %s", origJSON, copyJSON)
	}
}

func TestDuplicateFreedOnce(t *testing.T) {
	ctx, tree := loadCRUDContext(t)
	defer ctx.Close()
	defer tree.Close()

	if _, err := tree.NewPath("/cambium-data-crud-demo:top", nil, cambium.NewPathOpts{}); err != nil {
		t.Fatalf("NewPath top: %v", err)
	}
	counter := "1"
	if _, err := tree.NewPath("/cambium-data-crud-demo:top/counter", &counter, cambium.NewPathOpts{}); err != nil {
		t.Fatalf("NewPath counter: %v", err)
	}

	for i := 0; i < 100; i++ {
		copy, err := tree.Duplicate()
		if err != nil {
			t.Fatalf("Duplicate iteration %d: %v", i, err)
		}
		copy.Close()
	}

	if !tree.Exists("/cambium-data-crud-demo:top/counter") {
		t.Fatal("source tree corrupted after duplicate loop")
	}
}
