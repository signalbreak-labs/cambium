// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

//go:build cgo

package libyangbackend_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	cambium "github.com/signalbreak-labs/cambium/go/libyangbackend"
)

// TestRemovePathRejectsListKey: libyang's lyd_free_tree silently refuses to free
// a list key (void return), so RemovePath on a key leaf must surface an error
// (E0006), not a silent no-op success. Remove the list entry, not its key.
func TestRemovePathRejectsListKey(t *testing.T) {
	ctx, tree := loadCRUDContext(t)
	defer ctx.Close()

	// Create a list entry so the key leaf is addressable.
	v := "1"
	if _, err := tree.NewPath("/cambium-data-crud-demo:top/item[id='a']/value", &v, cambium.NewPathOpts{}); err != nil {
		t.Fatalf("NewPath: %v", err)
	}
	keyPath := "/cambium-data-crud-demo:top/item[id='a']/id"
	if !tree.Exists(keyPath) {
		t.Fatalf("precondition: key %q should exist", keyPath)
	}

	err := tree.RemovePath(keyPath)
	if err == nil {
		t.Fatal("RemovePath on a list key must error, got nil (silent no-op)")
	}
	var ce *cambium.Error
	if !errors.As(err, &ce) || ce.RuleCode() != cambium.RuleCodeDataPath {
		t.Fatalf("want RuleCodeDataPath (E0006), got %v", err)
	}
	if !tree.Exists(keyPath) {
		t.Fatal("key was removed despite the returned error")
	}
}

// loadCRUDContext creates a temporary module that matches the Rust
// data_crud.rs demo exactly so the same paths and observables can be asserted.
func loadCRUDContext(t *testing.T) (*cambium.Context, *cambium.DataTree) {
	t.Helper()
	dir := t.TempDir()
	module := filepath.Join(dir, "cambium-data-crud-demo.yang")
	const source = `module cambium-data-crud-demo {
    namespace "urn:cambium:data-crud";
    prefix cdc;
    yang-version 1.1;
    revision 2026-06-14;

    container top {
        leaf enabled {
            type boolean;
            default "true";
        }
        leaf counter {
            type uint64;
        }
        container nested {
            leaf name {
                type string;
            }
        }
        list item {
            key "id";
            leaf id {
                type string;
            }
            leaf value {
                type uint64;
            }
        }
        leaf-list tags {
            type string;
        }
    }
}
`
	if err := os.WriteFile(module, []byte(source), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	ctx, err := cambium.NewContext()
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}
	if err := ctx.SetSearchPath(dir); err != nil {
		ctx.Close()
		t.Fatalf("SetSearchPath: %v", err)
	}
	if err := ctx.LoadModule("cambium-data-crud-demo"); err != nil {
		ctx.Close()
		t.Fatalf("LoadModule: %v", err)
	}

	tree := ctx.NewData()
	return ctx, tree
}

func TestNewPathThenSerializeDeclarationOrder(t *testing.T) {
	ctx, tree := loadCRUDContext(t)
	defer ctx.Close()
	defer tree.Close()

	if _, err := tree.NewPath("/cambium-data-crud-demo:top", nil, cambium.NewPathOpts{}); err != nil {
		t.Fatalf("NewPath top: %v", err)
	}
	// Insert in reverse declaration order: nested, counter, enabled.
	name := "n"
	if _, err := tree.NewPath("/cambium-data-crud-demo:top/nested/name", &name, cambium.NewPathOpts{}); err != nil {
		t.Fatalf("NewPath nested/name: %v", err)
	}
	counter := "7"
	if _, err := tree.NewPath("/cambium-data-crud-demo:top/counter", &counter, cambium.NewPathOpts{}); err != nil {
		t.Fatalf("NewPath counter: %v", err)
	}
	enabled := "false"
	if _, err := tree.NewPath("/cambium-data-crud-demo:top/enabled", &enabled, cambium.NewPathOpts{}); err != nil {
		t.Fatalf("NewPath enabled: %v", err)
	}

	xml, err := tree.Serialize(cambium.FormatXML, cambium.DefaultSerializeFlags())
	if err != nil {
		t.Fatalf("Serialize XML: %v", err)
	}
	s := string(xml)
	xe := strings.Index(s, "<enabled")
	xc := strings.Index(s, "<counter")
	xn := strings.Index(s, "<nested")
	if xe < 0 || xc < 0 || xn < 0 {
		t.Fatalf("missing elements in XML:\n%s", s)
	}
	if xe >= xc || xc >= xn {
		t.Fatalf("XML must be schema declaration order enabled<counter<nested, got:\n%s", s)
	}

	json, err := tree.Serialize(cambium.FormatJSON, cambium.DefaultSerializeFlags())
	if err != nil {
		t.Fatalf("Serialize JSON: %v", err)
	}
	js := string(json)
	je := strings.Index(js, "\"enabled\"")
	jc := strings.Index(js, "\"counter\"")
	jn := strings.Index(js, "\"nested\"")
	if je < 0 || jc < 0 || jn < 0 {
		t.Fatalf("missing elements in JSON:\n%s", js)
	}
	if je >= jc || jc >= jn {
		t.Fatalf("JSON must be schema declaration order, got:\n%s", js)
	}
}

func TestNewPathCreatesNodes(t *testing.T) {
	ctx, tree := loadCRUDContext(t)
	defer ctx.Close()
	defer tree.Close()

	if _, err := tree.NewPath("/cambium-data-crud-demo:top", nil, cambium.NewPathOpts{}); err != nil {
		t.Fatalf("NewPath top: %v", err)
	}
	counter := "42"
	addr, err := tree.NewPath("/cambium-data-crud-demo:top/counter", &counter, cambium.NewPathOpts{})
	if err != nil {
		t.Fatalf("NewPath counter: %v", err)
	}
	if got, want := addr.Path(), "/cambium-data-crud-demo:top/counter"; got != want {
		t.Fatalf("NodeAddr.Path() = %q, want %q", got, want)
	}

	node, err := tree.Get("/cambium-data-crud-demo:top/counter")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	v, ok, err := node.Value()
	if err != nil || !ok {
		t.Fatalf("Value: ok=%v err=%v", ok, err)
	}
	if v.Kind() != cambium.ValueUint64 {
		t.Fatalf("Value kind = %v, want Uint64", v.Kind())
	}
	if u, ok := v.Uint64(); !ok || u != 42 {
		t.Fatalf("Value = %v, want 42", u)
	}
}

func TestNewPathUpdatesExistingLeaf(t *testing.T) {
	ctx, tree := loadCRUDContext(t)
	defer ctx.Close()
	defer tree.Close()

	if _, err := tree.NewPath("/cambium-data-crud-demo:top", nil, cambium.NewPathOpts{}); err != nil {
		t.Fatalf("NewPath top: %v", err)
	}
	five := "5"
	if _, err := tree.NewPath("/cambium-data-crud-demo:top/counter", &five, cambium.NewPathOpts{}); err != nil {
		t.Fatalf("NewPath counter 5: %v", err)
	}
	seven := "7"
	if _, err := tree.NewPath("/cambium-data-crud-demo:top/counter", &seven, cambium.NewPathOpts{Update: true}); err != nil {
		t.Fatalf("NewPath counter 7: %v", err)
	}

	node, err := tree.Get("/cambium-data-crud-demo:top/counter")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	v, ok, err := node.Value()
	if err != nil || !ok {
		t.Fatalf("Value: ok=%v err=%v", ok, err)
	}
	if u, ok := v.Uint64(); !ok || u != 7 {
		t.Fatalf("Value = %v, want 7", u)
	}
}

func TestNewPathListEntry(t *testing.T) {
	ctx, tree := loadCRUDContext(t)
	defer ctx.Close()
	defer tree.Close()

	if _, err := tree.NewPath("/cambium-data-crud-demo:top", nil, cambium.NewPathOpts{}); err != nil {
		t.Fatalf("NewPath top: %v", err)
	}
	if _, err := tree.NewPath("/cambium-data-crud-demo:top/item[id='a']", nil, cambium.NewPathOpts{}); err != nil {
		t.Fatalf("NewPath item: %v", err)
	}
	value := "10"
	if _, err := tree.NewPath("/cambium-data-crud-demo:top/item[id='a']/value", &value, cambium.NewPathOpts{}); err != nil {
		t.Fatalf("NewPath item value: %v", err)
	}

	item, err := tree.Get("/cambium-data-crud-demo:top/item[id='a']")
	if err != nil {
		t.Fatalf("Get item: %v", err)
	}
	children, err := item.Children()
	if err != nil {
		t.Fatalf("Children: %v", err)
	}
	if children.Len() != 2 {
		t.Fatalf("item children = %d, want 2", children.Len())
	}

	node, err := tree.Get("/cambium-data-crud-demo:top/item[id='a']/value")
	if err != nil {
		t.Fatalf("Get value: %v", err)
	}
	v, ok, err := node.Value()
	if err != nil || !ok {
		t.Fatalf("Value: ok=%v err=%v", ok, err)
	}
	if u, ok := v.Uint64(); !ok || u != 10 {
		t.Fatalf("Value = %v, want 10", u)
	}
}

func TestSetValueReportsChange(t *testing.T) {
	ctx, tree := loadCRUDContext(t)
	defer ctx.Close()
	defer tree.Close()

	if _, err := tree.NewPath("/cambium-data-crud-demo:top", nil, cambium.NewPathOpts{}); err != nil {
		t.Fatalf("NewPath top: %v", err)
	}
	three := "3"
	if _, err := tree.NewPath("/cambium-data-crud-demo:top/counter", &three, cambium.NewPathOpts{}); err != nil {
		t.Fatalf("NewPath counter 3: %v", err)
	}

	changed, err := tree.SetValue("/cambium-data-crud-demo:top/counter", "4")
	if err != nil {
		t.Fatalf("SetValue 4: %v", err)
	}
	if !changed {
		t.Fatal("SetValue 4 should report changed=true")
	}

	changed, err = tree.SetValue("/cambium-data-crud-demo:top/counter", "4")
	if err != nil {
		t.Fatalf("SetValue 4 again: %v", err)
	}
	if changed {
		t.Fatal("SetValue 4 again should report changed=false")
	}

	node, err := tree.Get("/cambium-data-crud-demo:top/counter")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	v, ok, err := node.Value()
	if err != nil || !ok {
		t.Fatalf("Value: ok=%v err=%v", ok, err)
	}
	if u, ok := v.Uint64(); !ok || u != 4 {
		t.Fatalf("Value = %v, want 4", u)
	}
}

func TestRemovePathRemovesSubtree(t *testing.T) {
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
	if _, err := tree.NewPath("/cambium-data-crud-demo:top/nested", nil, cambium.NewPathOpts{}); err != nil {
		t.Fatalf("NewPath nested: %v", err)
	}
	name := "inner"
	if _, err := tree.NewPath("/cambium-data-crud-demo:top/nested/name", &name, cambium.NewPathOpts{}); err != nil {
		t.Fatalf("NewPath nested/name: %v", err)
	}

	if err := tree.RemovePath("/cambium-data-crud-demo:top/nested"); err != nil {
		t.Fatalf("RemovePath nested: %v", err)
	}
	if tree.Exists("/cambium-data-crud-demo:top/nested") {
		t.Fatal("nested should be removed")
	}
	if tree.Exists("/cambium-data-crud-demo:top/nested/name") {
		t.Fatal("nested/name should be removed")
	}
	if !tree.Exists("/cambium-data-crud-demo:top/counter") {
		t.Fatal("counter should survive")
	}
}

func TestUnlinkPathDetachesSubtree(t *testing.T) {
	ctx, tree := loadCRUDContext(t)
	defer ctx.Close()
	defer tree.Close()

	if _, err := tree.NewPath("/cambium-data-crud-demo:top", nil, cambium.NewPathOpts{}); err != nil {
		t.Fatalf("NewPath top: %v", err)
	}
	counter := "9"
	if _, err := tree.NewPath("/cambium-data-crud-demo:top/counter", &counter, cambium.NewPathOpts{}); err != nil {
		t.Fatalf("NewPath counter: %v", err)
	}
	if _, err := tree.NewPath("/cambium-data-crud-demo:top/nested", nil, cambium.NewPathOpts{}); err != nil {
		t.Fatalf("NewPath nested: %v", err)
	}
	name := "detached"
	if _, err := tree.NewPath("/cambium-data-crud-demo:top/nested/name", &name, cambium.NewPathOpts{}); err != nil {
		t.Fatalf("NewPath nested/name: %v", err)
	}

	detached, err := tree.UnlinkPath("/cambium-data-crud-demo:top/nested")
	if err != nil {
		t.Fatalf("UnlinkPath nested: %v", err)
	}
	defer detached.Close()

	if tree.Exists("/cambium-data-crud-demo:top/nested") {
		t.Fatal("nested should be gone from source tree")
	}

	xml, err := detached.Serialize(cambium.FormatXML, cambium.DefaultSerializeFlags())
	if err != nil {
		t.Fatalf("Serialize detached: %v", err)
	}
	s := string(xml)
	if !strings.Contains(s, "<nested") {
		t.Fatalf("detached XML missing nested:\n%s", s)
	}
	if !strings.Contains(s, "<name>detached</name>") {
		t.Fatalf("detached XML missing name:\n%s", s)
	}
}

func TestAddDefaultsAddsDefaultLeaves(t *testing.T) {
	ctx, tree := loadCRUDContext(t)
	defer ctx.Close()
	defer tree.Close()

	if _, err := tree.NewPath("/cambium-data-crud-demo:top", nil, cambium.NewPathOpts{}); err != nil {
		t.Fatalf("NewPath top: %v", err)
	}
	if tree.Exists("/cambium-data-crud-demo:top/enabled") {
		t.Fatal("enabled should not exist before AddDefaults")
	}

	if err := tree.AddDefaults(cambium.ImplicitOpts{}); err != nil {
		t.Fatalf("AddDefaults: %v", err)
	}
	node, err := tree.Get("/cambium-data-crud-demo:top/enabled")
	if err != nil {
		t.Fatalf("Get enabled: %v", err)
	}
	v, ok, err := node.Value()
	if err != nil || !ok {
		t.Fatalf("Value: ok=%v err=%v", ok, err)
	}
	if b, ok := v.Bool(); !ok || !b {
		t.Fatalf("Value = %v, want true", b)
	}
	def, err := node.IsDefault()
	if err != nil || !def {
		t.Fatalf("IsDefault = %v err=%v", def, err)
	}
}

func TestValidateAfterMutationPasses(t *testing.T) {
	ctx, tree := loadCRUDContext(t)
	defer ctx.Close()
	defer tree.Close()

	if _, err := tree.NewPath("/cambium-data-crud-demo:top", nil, cambium.NewPathOpts{}); err != nil {
		t.Fatalf("NewPath top: %v", err)
	}
	counter := "99"
	if _, err := tree.NewPath("/cambium-data-crud-demo:top/counter", &counter, cambium.NewPathOpts{}); err != nil {
		t.Fatalf("NewPath counter: %v", err)
	}
	if err := tree.Validate(cambium.ValidateMode{Present: true}); err != nil {
		t.Fatalf("Validate: %v", err)
	}
}

func TestNewPathInvalidPathReturnsDataPathRuleCode(t *testing.T) {
	ctx, tree := loadCRUDContext(t)
	defer ctx.Close()
	defer tree.Close()

	value := "x"
	_, err := tree.NewPath("/cambium-data-crud-demo:top/no-such-leaf", &value, cambium.NewPathOpts{})
	if err == nil {
		t.Fatal("NewPath invalid path should error")
	}
	e, ok := err.(*cambium.Error)
	if !ok || e.RuleCode() != cambium.RuleCodeDataPath {
		t.Fatalf("error = %v, want E0006", err)
	}
}
