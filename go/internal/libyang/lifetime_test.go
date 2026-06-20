//go:build cgo

package libyang

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// TestTreeSurvivesContextGC exercises the critical lifetime invariant: a data
// tree must remain usable even if the Go *RawContext that created it is
// garbage-collected before the tree. Before the owner fix this is latent UAF:
// the context finalizer can run first, destroying ly_ctx, and then Serialize or
// Validate dereferences the freed context. After the fix the tree holds the
// owning *RawContext, so the GC cannot collect it.
func TestTreeSurvivesContextGC(t *testing.T) {
	conf := filepath.Join("..", "..", "..", "conformance")
	moduleDir := filepath.Join(conf, "fixtures", "scrambled-children", "module")
	input, err := os.ReadFile(filepath.Join(conf, "fixtures", "scrambled-children", "input.xml"))
	if err != nil {
		t.Fatal(err)
	}

	var trees []*RawDataTree
	for i := 0; i < 50; i++ {
		trees = append(trees, parseDetachedTree(t, moduleDir, input))
	}

	// Force collection of any unreachable RawContext wrappers. If RawDataTree
	// does not keep its owner alive, the context finalizers run now and the next
	// Serialize call is UB.
	runtime.GC()
	runtime.GC()

	for _, tree := range trees {
		_, err := tree.Serialize(FormatXML, PrintWithSiblings)
		if err != nil {
			t.Fatalf("serialize after GC: %v", err)
		}
	}
}

func TestUserOrderedListSurvivesTreeGC(t *testing.T) {
	conf := filepath.Join("..", "..", "..", "conformance")
	moduleDir := filepath.Join(conf, "fixtures", "ordered-user", "module")
	input, err := os.ReadFile(filepath.Join(conf, "fixtures", "ordered-user", "input.xml"))
	if err != nil {
		t.Fatal(err)
	}

	lists := make([]*RawUserOrderedList, 50)
	for i := range lists {
		lists[i] = parseDetachedList(t, moduleDir, input)
	}

	runtime.GC()
	runtime.GC()

	for _, list := range lists {
		if err := list.MoveBefore(2, 0); err != nil {
			t.Fatalf("move after GC: %v", err)
		}
	}
}

func TestInteriorNulRejected(t *testing.T) {
	ctx, err := NewContext()
	if err != nil {
		t.Fatal(err)
	}
	defer ctx.Close()

	payload := []byte("<top>\x00</top>")
	if _, err := ctx.ParseData(FormatXML, ParseOnly, payload); err == nil {
		t.Fatal("expected error for interior NUL, got nil")
	}
}

// parseDetachedTree returns a tree while deliberately allowing its RawContext
// wrapper to become unreachable.
func parseDetachedTree(t *testing.T, moduleDir string, input []byte) *RawDataTree {
	t.Helper()
	ctx, err := NewContext()
	if err != nil {
		t.Fatal(err)
	}
	if err := ctx.SetSearchPath(moduleDir); err != nil {
		t.Fatal(err)
	}
	if err := ctx.LoadModule("order-demo"); err != nil {
		t.Fatal(err)
	}
	tree, err := ctx.ParseData(FormatXML, ParseOnly, input)
	if err != nil {
		t.Fatal(err)
	}
	// Drop the Go reference to the context. The only remaining reference must
	// be through the tree's owner field after the fix.
	ctx = nil
	runtime.GC()
	return tree
}

func parseDetachedList(t *testing.T, moduleDir string, input []byte) *RawUserOrderedList {
	t.Helper()
	ctx, err := NewContext()
	if err != nil {
		t.Fatal(err)
	}
	if err := ctx.SetSearchPath(moduleDir); err != nil {
		t.Fatal(err)
	}
	if err := ctx.LoadModule("ordered-user-demo"); err != nil {
		t.Fatal(err)
	}
	tree, err := ctx.ParseData(FormatXML, ParseOnly, input)
	if err != nil {
		t.Fatal(err)
	}
	list, err := tree.UserOrderedListAt("/ordered-user-demo:config/entry[name='c']")
	if err != nil {
		t.Fatal(err)
	}
	// Drop references to both context and tree. The list must keep the tree
	// alive after the fix.
	tree = nil
	ctx = nil
	runtime.GC()
	return list
}
