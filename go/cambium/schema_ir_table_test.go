// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

package cambium_test

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/signalbreak-labs/cambium/go/cambium"
)

// nestedChainModule returns a module with depth nested containers ending in
// one string leaf.
func nestedChainModule(depth int) string {
	var b strings.Builder
	b.WriteString("module chain {\n  yang-version 1.1; namespace \"urn:chain\"; prefix c;\n")
	for i := 0; i < depth; i++ {
		fmt.Fprintf(&b, "container c%d {\n", i)
	}
	b.WriteString("leaf end { type string; }\n")
	for i := 0; i < depth; i++ {
		b.WriteString("}\n")
	}
	b.WriteString("}\n")
	return b.String()
}

// fanoutModule returns a module with width lists, each keyed by two leaves and
// carrying a choice with two cases, so the data and key views differ from the
// structural view.
func fanoutModule(width int) string {
	var b strings.Builder
	b.WriteString("module fan {\n  yang-version 1.1; namespace \"urn:fan\"; prefix f;\n  container root {\n")
	for i := 0; i < width; i++ {
		fmt.Fprintf(&b, `list l%d { key "k2 k1"; leaf v { type string; } leaf k1 { type string; } leaf k2 { type uint8; }
  choice ch { case a { container ca { leaf x { type string; } } } case b { leaf y { type string; } } } }
`, i)
	}
	b.WriteString("  }\n}\n")
	return b.String()
}

func TestSchemaIRTableIsLinearInNodes(t *testing.T) {
	for _, depth := range []int{4, 8, 12, 20, 40} {
		ctx := loadDownstreamContext(t, nestedChainModule(depth))
		table := ctx.SchemaIRTable()
		if table.Version != cambium.SchemaIRTableVersion {
			t.Fatalf("version = %q", table.Version)
		}
		if len(table.Errors) != 0 {
			t.Fatalf("errors = %#v", table.Errors)
		}
		stats := ctx.SchemaIRStats()
		unique := depth + 1
		if len(table.Nodes) != unique || stats.Nodes != unique {
			t.Fatalf("depth %d: table nodes = %d, stats nodes = %d, want %d", depth, len(table.Nodes), stats.Nodes, unique)
		}
		// Each container references its child from Children and DataChildren.
		if want := 2 * depth; stats.Relationships != want {
			t.Fatalf("depth %d: relationships = %d, want %d", depth, stats.Relationships, want)
		}
		// The legacy nested v1 projection doubles per level.
		if want := uint64(1)<<(depth+1) - 1; stats.V1Records != want {
			t.Fatalf("depth %d: v1 records = %d, want %d", depth, stats.V1Records, want)
		}
		// Path bytes grow with depth (absolute paths), not with the number of views.
		if stats.PathBytes <= 0 || stats.PathBytes > unique*3*(depth+1)*48 {
			t.Fatalf("depth %d: path bytes = %d out of bounds", depth, stats.PathBytes)
		}
	}
}

func TestSchemaIRTableFanoutViewsReferenceCanonicalNodes(t *testing.T) {
	const width = 50
	ctx := loadDownstreamContext(t, fanoutModule(width))
	table := ctx.SchemaIRTable()
	// root + width*(list + v,k1,k2 + choice + 2 cases + ca + x + y)
	if want := 1 + width*10; len(table.Nodes) != want {
		t.Fatalf("table nodes = %d, want %d", len(table.Nodes), want)
	}
	for i, node := range table.Nodes {
		if int(node.ID) != i {
			t.Fatalf("node %d has ID %d", i, node.ID)
		}
	}
	mod := table.Modules[0]
	if len(mod.Children) != 1 {
		t.Fatalf("module children = %v", mod.Children)
	}
	root := table.Nodes[mod.Children[0]]
	if root.Parent != cambium.SchemaIRNoParent {
		t.Fatalf("root parent = %d", root.Parent)
	}
	list := table.Nodes[root.Children[0]]
	names := func(ids []cambium.SchemaIRNodeID) []string {
		var out []string
		for _, id := range ids {
			out = append(out, table.Nodes[id].Name)
		}
		return out
	}
	if got, want := names(list.Children), []string{"v", "k1", "k2", "ch"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("list children = %v, want %v", got, want)
	}
	if got, want := names(list.DataChildren), []string{"v", "k1", "k2", "ca", "y"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("list data children = %v, want %v", got, want)
	}
	if got, want := names(list.ListKeys), []string{"k2", "k1"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("list keys = %v, want %v", got, want)
	}
	// Alternate views point at the same canonical records.
	if list.ListKeys[0] != list.Children[2] || list.DataChildren[3] == list.Children[3] {
		t.Fatalf("views do not share canonical node IDs: %+v", list)
	}
	for _, id := range list.Children {
		if table.Nodes[id].Parent != list.ID {
			t.Fatalf("child %s parent = %d, want %d", table.Nodes[id].Name, table.Nodes[id].Parent, list.ID)
		}
	}
	ca := table.Nodes[list.DataChildren[3]]
	if ca.Parent == list.ID {
		t.Fatal("ca's structural parent should be its case, not the list")
	}
}

func TestSchemaIRTableMatchesLegacyProjectionFacts(t *testing.T) {
	ctx := loadDownstreamContext(t, fanoutModule(2))
	legacy := ctx.SchemaIR()
	table := ctx.SchemaIRTable()
	var walk func(nodes []cambium.SchemaIRNode, ids []cambium.SchemaIRNodeID)
	walk = func(nodes []cambium.SchemaIRNode, ids []cambium.SchemaIRNodeID) {
		if len(nodes) != len(ids) {
			t.Fatalf("view length %d vs %d", len(nodes), len(ids))
		}
		for i, legacyNode := range nodes {
			node := table.Nodes[ids[i]]
			if node.QualifiedPath != legacyNode.QualifiedPath || node.LocalPath != legacyNode.LocalPath ||
				node.NamespaceQualifiedPath != legacyNode.NamespaceQualifiedPath || node.Kind != legacyNode.Kind ||
				!reflect.DeepEqual(node.KeyNames, legacyNode.KeyNames) || node.Config != legacyNode.Config {
				t.Fatalf("table node %+v differs from legacy %+v", node, legacyNode)
			}
			walk(legacyNode.Children, node.Children)
			walk(legacyNode.DataChildren, node.DataChildren)
			walk(legacyNode.ListKeys, node.ListKeys)
		}
	}
	for i, mod := range legacy.Modules {
		walk(mod.Children, table.Modules[i].Children)
	}
}

func TestSchemaIRWithLimitFailsWithoutMaterializing(t *testing.T) {
	// 2^41-1 legacy records would exhaust memory; the limit check must run
	// before any materialization.
	ctx := loadDownstreamContext(t, nestedChainModule(40))
	_, err := ctx.SchemaIRWithLimit(1 << 16)
	if err == nil {
		t.Fatal("SchemaIRWithLimit succeeded, want resource limit error")
	}
	var cerr *cambium.Error
	if !errors.As(err, &cerr) {
		t.Fatalf("error %T is not *cambium.Error", err)
	}
	if diag := cambium.DiagnosticFromError(err); diag.Kind != cambium.DiagnosticResourceLimit {
		t.Fatalf("diagnostic kind = %q, want %q", diag.Kind, cambium.DiagnosticResourceLimit)
	}
	small := loadDownstreamContext(t, nestedChainModule(4))
	ir, err := small.SchemaIRWithLimit(31)
	if err != nil {
		t.Fatalf("SchemaIRWithLimit at exact size: %v", err)
	}
	if !reflect.DeepEqual(ir, small.SchemaIR()) {
		t.Fatal("limited projection differs from SchemaIR()")
	}
	if _, err := small.SchemaIRWithLimit(30); err == nil {
		t.Fatal("SchemaIRWithLimit below size succeeded")
	}
}

func TestSchemaIRTableConcurrentReadsOnFrozenContext(t *testing.T) {
	ctx := loadDownstreamContext(t, fanoutModule(10))
	want := ctx.SchemaIRTable()
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			got := ctx.SchemaIRTable()
			if len(got.Nodes) != len(want.Nodes) {
				t.Errorf("concurrent table nodes = %d, want %d", len(got.Nodes), len(want.Nodes))
			}
		}()
	}
	wg.Wait()
	// Snapshots are values: mutating one does not change another.
	want.Nodes[0].KeyNames = append(want.Nodes[0].KeyNames, "mutated")
	want.Nodes[0].Children[0] = 999
	again := ctx.SchemaIRTable()
	if len(again.Nodes[0].KeyNames) != 0 || again.Nodes[0].Children[0] == 999 {
		t.Fatal("SchemaIRTable snapshot shares mutable state")
	}
}

func BenchmarkSchemaIRTableDepth(b *testing.B) {
	for _, depth := range []int{8, 12, 16} {
		source := nestedChainModule(depth)
		builder, _ := cambium.NewContextBuilder(cambium.ContextFlags{DisableSearchdirCwd: true})
		if err := builder.LoadModuleStr(source); err != nil {
			b.Fatal(err)
		}
		ctx, err := builder.Build()
		if err != nil {
			b.Fatal(err)
		}
		b.Run(fmt.Sprintf("table/depth=%d", depth), func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				_ = ctx.SchemaIRTable()
			}
		})
		b.Run(fmt.Sprintf("v1/depth=%d", depth), func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				_ = ctx.SchemaIR()
			}
		})
		ctx.Close()
	}
}
