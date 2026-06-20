//go:build cgo

package libyangbackend_test

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	cambium "github.com/signalbreak-labs/cambium/go/libyangbackend"
)

// loadFixture parses the named conformance fixture (module directory + input.xml).
func loadFixture(t *testing.T, name string) (*cambium.Context, *cambium.DataTree) {
	t.Helper()
	conf := findConformance(t)
	moduleDir := filepath.Join(conf, "fixtures", name, "module")
	input, err := os.ReadFile(filepath.Join(conf, "fixtures", name, "input.xml"))
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
	modName := ""
	switch name {
	case "ordered-user":
		modName = "ordered-user-demo"
	case "system-list-canonical":
		modName = "system-list-demo"
	default:
		ctx.Close()
		t.Fatalf("unknown fixture %q", name)
	}
	if err := ctx.LoadModule(modName); err != nil {
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

func TestUserOrderedViewRead(t *testing.T) {
	ctx, tree := loadFixture(t, "ordered-user")
	defer ctx.Close()
	defer tree.Close()

	node, err := tree.Get("/ordered-user-demo:config/entry[name='a']")
	if err != nil {
		t.Fatalf("Get entry a: %v", err)
	}

	view, ok, err := node.AsUserOrdered()
	if err != nil {
		t.Fatalf("AsUserOrdered: %v", err)
	}
	if !ok {
		t.Fatal("AsUserOrdered should return ok=true for ordered-by user list")
	}

	n, err := view.Len()
	if err != nil {
		t.Fatalf("Len: %v", err)
	}
	if n != 3 {
		t.Fatalf("Len = %d, want 3", n)
	}

	entries, err := view.Iter()
	if err != nil {
		t.Fatalf("Iter: %v", err)
	}
	var names []string
	for _, e := range entries {
		nameNode, err := e.Children()
		if err != nil {
			t.Fatalf("entry Children: %v", err)
		}
		for i := 0; i < nameNode.Len(); i++ {
			child, _ := nameNode.Get(i)
			if n, _ := child.Name(); n == "name" {
				s, _, _ := child.ValueStr()
				names = append(names, s)
				break
			}
		}
	}
	want := []string{"c", "a", "b"}
	if !slices.Equal(names, want) {
		t.Fatalf("insertion order = %v, want %v", names, want)
	}

	idx, ok, err := view.FindByKey([][2]string{{"name", "a"}})
	if err != nil {
		t.Fatalf("FindByKey: %v", err)
	}
	if !ok || idx != 1 {
		t.Fatalf("FindByKey(a) = (%d, %v), want (1, true)", idx, ok)
	}

	if _, ok, _ := view.FindByKey([][2]string{{"name", "missing"}}); ok {
		t.Fatal("FindByKey missing should return ok=false")
	}
}

func TestAsUserOrderedSystemListReturnsFalse(t *testing.T) {
	ctx, tree := loadFixture(t, "system-list-canonical")
	defer ctx.Close()
	defer tree.Close()

	node, err := tree.Get("/system-list-demo:config/server[name='any']")
	if err != nil {
		// The fixture may not contain this key; use xpath to get the first entry.
		set, err2 := tree.Select("/system-list-demo:config/server[1]")
		if err2 != nil {
			t.Fatalf("Select server: %v", err2)
		}
		if set.IsEmpty() {
			t.Fatal("no system list entries found")
		}
		node, _ = set.Get(0)
	}

	_, ok, err := node.AsUserOrdered()
	if err != nil {
		t.Fatalf("AsUserOrdered: %v", err)
	}
	if ok {
		t.Fatal("AsUserOrdered should return ok=false for system-ordered list")
	}
}
