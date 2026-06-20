//go:build cgo

package libyangbackend_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	cambium "github.com/signalbreak-labs/cambium/go/libyangbackend"
)

func buildBase(t *testing.T, tree *cambium.DataTree) {
	t.Helper()
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
	name := "x"
	if _, err := tree.NewPath("/cambium-data-crud-demo:top/nested/name", &name, cambium.NewPathOpts{}); err != nil {
		t.Fatalf("NewPath nested/name: %v", err)
	}
	if _, err := tree.NewPath("/cambium-data-crud-demo:top/item[id='a']", nil, cambium.NewPathOpts{}); err != nil {
		t.Fatalf("NewPath item a: %v", err)
	}
	value := "10"
	if _, err := tree.NewPath("/cambium-data-crud-demo:top/item[id='a']/value", &value, cambium.NewPathOpts{}); err != nil {
		t.Fatalf("NewPath item a value: %v", err)
	}
	tag := "red"
	if _, err := tree.NewPath("/cambium-data-crud-demo:top/tags", &tag, cambium.NewPathOpts{}); err != nil {
		t.Fatalf("NewPath tags: %v", err)
	}
}

func buildAfter(t *testing.T, tree *cambium.DataTree) {
	t.Helper()
	if _, err := tree.NewPath("/cambium-data-crud-demo:top", nil, cambium.NewPathOpts{}); err != nil {
		t.Fatalf("NewPath top: %v", err)
	}
	counter := "2"
	if _, err := tree.NewPath("/cambium-data-crud-demo:top/counter", &counter, cambium.NewPathOpts{}); err != nil {
		t.Fatalf("NewPath counter: %v", err)
	}
	if _, err := tree.NewPath("/cambium-data-crud-demo:top/item[id='b']", nil, cambium.NewPathOpts{}); err != nil {
		t.Fatalf("NewPath item b: %v", err)
	}
	value := "20"
	if _, err := tree.NewPath("/cambium-data-crud-demo:top/item[id='b']/value", &value, cambium.NewPathOpts{}); err != nil {
		t.Fatalf("NewPath item b value: %v", err)
	}
}

func TestDiffEmptyWhenEqual(t *testing.T) {
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
	defer ctx.Close()
	if err := ctx.SetSearchPath(moduleDir); err != nil {
		t.Fatalf("SetSearchPath: %v", err)
	}
	if err := ctx.LoadModule("ordered-user-demo"); err != nil {
		t.Fatalf("LoadModule: %v", err)
	}
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

	diff, err := original.Diff(copy, cambium.DiffOpts{})
	if err != nil {
		t.Fatalf("Diff: %v", err)
	}
	defer diff.Close()

	if !diff.IsEmpty() {
		t.Fatalf("expected empty diff, got %d edits", len(diff.Edits()))
	}
	if len(diff.Edits()) != 0 {
		t.Fatalf("expected no edits, got %v", diff.Edits())
	}
}

func TestDiffCreateDeleteReplace(t *testing.T) {
	ctx, baseTree := loadCRUDContext(t)
	defer ctx.Close()
	defer baseTree.Close()
	buildBase(t, baseTree)

	afterTree := ctx.NewData()
	defer afterTree.Close()
	buildAfter(t, afterTree)

	diff, err := baseTree.Diff(afterTree, cambium.DiffOpts{})
	if err != nil {
		t.Fatalf("Diff: %v", err)
	}
	defer diff.Close()

	if diff.IsEmpty() {
		t.Fatal("expected non-empty diff")
	}

	find := func(path string) (cambium.DiffEdit, bool) {
		for _, e := range diff.Edits() {
			if e.Path() == path {
				return e, true
			}
		}
		return cambium.DiffEdit{}, false
	}

	counter, ok := find("/cambium-data-crud-demo:top/counter")
	if !ok {
		t.Fatal("missing counter edit")
	}
	if counter.Op() != cambium.DiffOpReplace {
		t.Fatalf("counter op = %v, want Replace", counter.Op())
	}
	if v, ok := counter.Value(); !ok || v != "2" {
		t.Fatalf("counter value = %q ok=%v, want 2", v, ok)
	}

	itemB, ok := find("/cambium-data-crud-demo:top/item[id='b']")
	if !ok {
		t.Fatal("missing item b edit")
	}
	if itemB.Op() != cambium.DiffOpCreate {
		t.Fatalf("item b op = %v, want Create", itemB.Op())
	}

	tagRed, ok := find("/cambium-data-crud-demo:top/tags[.='red']")
	if !ok {
		t.Fatal("missing tag red edit")
	}
	if tagRed.Op() != cambium.DiffOpDelete {
		t.Fatalf("tag red op = %v, want Delete", tagRed.Op())
	}

	nested, ok := find("/cambium-data-crud-demo:top/nested")
	if !ok {
		t.Fatal("missing nested edit")
	}
	if nested.Op() != cambium.DiffOpDelete {
		t.Fatalf("nested op = %v, want Delete", nested.Op())
	}

	for _, e := range diff.Edits() {
		if e.Op() == cambium.DiffOpNone {
			t.Fatalf("unexpected None edit at %s", e.Path())
		}
	}
}

func TestDataDiffSerializeLYB(t *testing.T) {
	ctx, baseTree := loadCRUDContext(t)
	defer ctx.Close()
	defer baseTree.Close()
	buildBase(t, baseTree)

	afterTree := ctx.NewData()
	defer afterTree.Close()
	buildAfter(t, afterTree)

	diff, err := baseTree.Diff(afterTree, cambium.DiffOpts{})
	if err != nil {
		t.Fatalf("Diff: %v", err)
	}
	defer diff.Close()

	lyb, err := diff.Serialize(cambium.FormatLYB)
	if err != nil {
		t.Fatalf("Serialize LYB: %v", err)
	}
	if len(lyb) == 0 {
		t.Fatal("Serialize LYB returned empty output")
	}

	xml, err := diff.Serialize(cambium.FormatXML)
	if err != nil {
		t.Fatalf("Serialize XML: %v", err)
	}
	if bytes.Equal(bytes.TrimSpace(lyb), bytes.TrimSpace(xml)) {
		t.Fatal("LYB output unexpectedly matched XML output")
	}
}

func TestDiffOrderedByUserAtomic(t *testing.T) {
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
	defer ctx.Close()
	if err := ctx.SetSearchPath(moduleDir); err != nil {
		t.Fatalf("SetSearchPath: %v", err)
	}
	if err := ctx.LoadModule("ordered-user-demo"); err != nil {
		t.Fatalf("LoadModule: %v", err)
	}
	base, err := ctx.Parse(cambium.FormatXML, cambium.ParseModeDataOnly, input)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	defer base.Close()

	after, err := base.Duplicate()
	if err != nil {
		t.Fatalf("Duplicate: %v", err)
	}
	defer after.Close()

	list, err := after.UserOrderedListAt("/ordered-user-demo:config/entry[name='c']")
	if err != nil {
		t.Fatalf("UserOrderedListAt: %v", err)
	}
	// indices: c=0, a=1, b=2. Move a (1) before c (0) -> a, c, b.
	if err := list.MoveBefore(1, 0); err != nil {
		t.Fatalf("MoveBefore: %v", err)
	}

	diff, err := base.Diff(after, cambium.DiffOpts{})
	if err != nil {
		t.Fatalf("Diff: %v", err)
	}
	defer diff.Close()

	edits := diff.Edits()
	if len(edits) != 1 {
		t.Fatalf("expected exactly one atomic edit, got %d: %v", len(edits), edits)
	}
	if !edits[0].IsUserOrdered() {
		t.Fatal("expected the atomic edit to be flagged user-ordered")
	}

	for _, e := range edits {
		if e.Op() == cambium.DiffOpReplace && strings.Contains(e.Path(), "/value") {
			t.Fatalf("unexpected scalar leaf replace: %s", e.Path())
		}
	}

	if _, err := diff.Serialize(cambium.FormatXML); err != nil {
		t.Fatalf("Serialize XML: %v", err)
	}
	if _, err := diff.Serialize(cambium.FormatJSON); err != nil {
		t.Fatalf("Serialize JSON: %v", err)
	}
}

func TestDataDiffFreedOnce(t *testing.T) {
	ctx, baseTree := loadCRUDContext(t)
	defer ctx.Close()
	defer baseTree.Close()
	buildBase(t, baseTree)

	afterTree := ctx.NewData()
	defer afterTree.Close()
	buildAfter(t, afterTree)

	for i := 0; i < 100; i++ {
		diff, err := baseTree.Diff(afterTree, cambium.DiffOpts{})
		if err != nil {
			t.Fatalf("Diff iteration %d: %v", i, err)
		}
		diff.Close()
	}
}
