// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

package datatree_test

import (
	"strings"
	"testing"

	"github.com/signalbreak-labs/cambium/go/datatree"
)

func parseDT(t *testing.T) *datatree.Tree {
	t.Helper()
	mod := loadDT(t)
	in := `{"dt:c":{"z":"hi","m":7,"a":true},"dt:tags":["t1","t2"],"dt:item":[{"id":"1","name":"x"}]}`
	tree, err := datatree.Parse(mod, datatree.FormatJSONIETF, []byte(in))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	return tree
}

func TestRootNodesOrder(t *testing.T) {
	tree := parseDT(t)
	roots := tree.RootNodes()
	var names []string
	for _, n := range roots {
		names = append(names, n.Name())
	}
	if got := strings.Join(names, ","); got != "c,tags,item" {
		t.Fatalf("root order = %s, want c,tags,item", got)
	}
}

func TestNavLeafAndContainer(t *testing.T) {
	tree := parseDT(t)

	z, ok := tree.Find("/c/z")
	if !ok || !z.IsLeaf() {
		t.Fatalf("Find /c/z: ok=%v leaf=%v", ok, z.IsLeaf())
	}
	if v, _ := z.LeafValue(); v != `"hi"` {
		t.Fatalf("/c/z value = %s, want \"hi\"", v)
	}
	// leading slash optional
	m, ok := tree.Find("c/m")
	if !ok {
		t.Fatal("Find c/m failed")
	}
	if v, _ := m.LeafValue(); v != "7" {
		t.Fatalf("/c/m value = %s, want 7", v)
	}

	c, ok := tree.Find("/c")
	if !ok || !c.IsContainer() {
		t.Fatalf("Find /c: ok=%v container=%v", ok, c.IsContainer())
	}
	var kids []string
	for _, k := range c.Children() {
		kids = append(kids, k.Name())
	}
	if got := strings.Join(kids, ","); got != "z,m,a" {
		t.Fatalf("/c children = %s, want z,m,a", got)
	}
}

func TestNavLeafListAndEntries(t *testing.T) {
	tree := parseDT(t)

	tags, ok := tree.Find("/tags")
	if !ok || !tags.IsLeafList() {
		t.Fatalf("Find /tags: ok=%v leaflist=%v", ok, tags.IsLeafList())
	}
	if got := strings.Join(tags.LeafListValues(), ","); got != `"t1","t2"` {
		t.Fatalf("/tags values = %s", got)
	}

	item, ok := tree.Find("/item")
	if !ok || !item.IsList() {
		t.Fatalf("Find /item: ok=%v list=%v", ok, item.IsList())
	}
	entries := item.Entries()
	if len(entries) != 1 {
		t.Fatalf("item entries = %d, want 1", len(entries))
	}
	var fields []string
	for _, f := range entries[0] {
		fields = append(fields, f.Name())
	}
	if got := strings.Join(fields, ","); got != "id,name" {
		t.Fatalf("entry fields = %s, want id,name (keys first)", got)
	}
}

func TestNavMisses(t *testing.T) {
	tree := parseDT(t)
	if _, ok := tree.Find("/nope"); ok {
		t.Fatal("Find /nope should miss")
	}
	if _, ok := tree.Find("/c/z/extra"); ok {
		t.Fatal("Find past a leaf should miss")
	}
	if _, ok := tree.Find("/item/x"); ok {
		t.Fatal("Find should not descend into a list")
	}
	if _, ok := tree.Find(""); ok {
		t.Fatal("Find empty path should miss")
	}
}
