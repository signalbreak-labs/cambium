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

// TestUserOrderedListMove exercises the positional mutation API (not just
// parse-time order preservation): parse entries c,a,b, move b before c, and
// confirm serialization reflects the new caller-controlled order b,c,a.
func TestUserOrderedListMove(t *testing.T) {
	conf := findConformance(t)
	moduleDir := filepath.Join(conf, "fixtures", "ordered-user", "module")
	input, err := os.ReadFile(filepath.Join(conf, "fixtures", "ordered-user", "input.xml"))
	if err != nil {
		t.Fatal(err)
	}

	ctx, err := cambium.NewContext()
	if err != nil {
		t.Fatal(err)
	}
	defer ctx.Close()
	if err := ctx.SetSearchPath(moduleDir); err != nil {
		t.Fatal(err)
	}
	if err := ctx.LoadModule("ordered-user-demo"); err != nil {
		t.Fatal(err)
	}

	tree, err := ctx.Parse(cambium.FormatXML, cambium.ParseModeDataOnly, input)
	if err != nil {
		t.Fatal(err)
	}
	defer tree.Close()

	list, err := tree.UserOrderedListAt("/ordered-user-demo:config/entry[name='c']")
	if err != nil {
		t.Fatalf("UserOrderedListAt: %v", err)
	}
	// indices: 0=c, 1=a, 2=b. Move b before c -> b, c, a.
	if err := list.MoveBefore(2, 0); err != nil {
		t.Fatalf("MoveBefore: %v", err)
	}

	out, err := tree.Serialize(cambium.FormatXML, cambium.DefaultSerializeFlags())
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	bi, ci, ai := strings.Index(s, "<name>b"), strings.Index(s, "<name>c"), strings.Index(s, "<name>a")
	if bi < 0 || ci < 0 || ai < 0 || bi >= ci || ci >= ai {
		t.Fatalf("positional move failed; want b<c<a, got:\n%s", s)
	}
}

// TestUserOrderedListHeterogeneousParent exercises the schema-name filter in
// nth(): the user-ordered list's parent container also holds a foreign `marker`
// leaf, which sorts ahead of the entries in the data tree. An unfiltered index
// walk would address `marker` at index 0 and shift every entry index by one,
// inserting/moving relative to the wrong node. The filter must index only the
// list's own entries.
func TestUserOrderedListHeterogeneousParent(t *testing.T) {
	dir := filepath.Join("..", "..", "target", "tests", "ordered-hetero", "modules")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	const modName = "hetero-ordered"
	yang := `module hetero-ordered {
    namespace "urn:cambium:hetero-ordered";
    prefix ho;
    yang-version 1.1;
    revision 2026-06-14;
    container c {
        leaf marker { type string; }
        list entry {
            key name;
            ordered-by user;
            leaf name { type string; }
        }
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

	input := []byte(`<c xmlns="urn:cambium:hetero-ordered">
  <marker>M</marker>
  <entry><name>a</name></entry>
  <entry><name>b</name></entry>
  <entry><name>c</name></entry>
</c>`)
	tree, err := ctx.Parse(cambium.FormatXML, cambium.ParseModeDataOnly, input)
	if err != nil {
		t.Fatal(err)
	}
	defer tree.Close()

	list, err := tree.UserOrderedListAt("/hetero-ordered:c/entry[name='a']")
	if err != nil {
		t.Fatalf("UserOrderedListAt: %v", err)
	}
	// Filtered indices: 0=a, 1=b, 2=c. Move c before a -> entry order c, a, b.
	if err := list.MoveBefore(2, 0); err != nil {
		t.Fatalf("MoveBefore: %v", err)
	}

	out, err := tree.Serialize(cambium.FormatXML, cambium.DefaultSerializeFlags())
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	ai, bi, ci := strings.Index(s, "<name>a"), strings.Index(s, "<name>b"), strings.Index(s, "<name>c")
	if ai < 0 || bi < 0 || ci < 0 || ci >= ai || ai >= bi {
		t.Fatalf("filtered positional move failed; want entry order c<a<b, got:\n%s", s)
	}
	if !strings.Contains(s, "<marker>M") {
		t.Fatalf("foreign marker sibling lost:\n%s", s)
	}
}

// TestUserOrderedListAtRejectsSystemOrdered asserts that asking for a positional
// handle on a system-ordered list fails with E0005 (the I1 mechanism). libyang
// only partially guards this — lyd_insert_child silently succeeds on a
// system-ordered list — so Cambium must reject the target at the API boundary,
// mirroring the Rust oracle (rust/cambium-core/tests/user_ordered_read.rs).
func TestUserOrderedListAtRejectsSystemOrdered(t *testing.T) {
	dir := filepath.Join("..", "..", "target", "tests", "ordered-system-reject", "modules")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	const modName = "system-reject"
	yang := `module system-reject {
    namespace "urn:cambium:system-reject";
    prefix sr;
    yang-version 1.1;
    revision 2026-06-14;
    container c {
        list entry {
            key name;
            leaf name { type string; }
        }
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

	input := []byte(`<c xmlns="urn:cambium:system-reject"><entry><name>a</name></entry></c>`)
	tree, err := ctx.Parse(cambium.FormatXML, cambium.ParseModeDataOnly, input)
	if err != nil {
		t.Fatal(err)
	}
	defer tree.Close()

	_, err = tree.UserOrderedListAt("/system-reject:c/entry[name='a']")
	if err == nil {
		t.Fatal("UserOrderedListAt on a system-ordered list must error, got nil")
	}
	var ce *cambium.Error
	if !errors.As(err, &ce) || ce.RuleCode() != cambium.RuleCodeOrderedList {
		t.Fatalf("want RuleCodeOrderedList (E0005), got %v", err)
	}
}
