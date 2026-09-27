// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

package cambium_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/signalbreak-labs/cambium/go/cambium"
	"github.com/signalbreak-labs/cambium/go/internal/confmanifest"
)

// TestSchemaIRTableOrderMatchesConformanceGoldens gates the v2 node table
// against the shared schema-ir corpus: for every golden path, the table's
// Children, DataChildren and ListKeys views must list exactly the golden
// order, and ConfigOnly must keep the remaining children in that order.
func TestSchemaIRTableOrderMatchesConformanceGoldens(t *testing.T) {
	root := conformanceRoot(t)
	cases, err := confmanifest.Load(filepath.Join(root, "manifest.toml"))
	if err != nil {
		t.Fatal(err)
	}
	ran := 0
	for _, c := range cases {
		if c.EffectiveTier() != confmanifest.TierSchemaIR {
			continue
		}
		ran++
		t.Run(c.Name, func(t *testing.T) {
			raw, err := os.ReadFile(filepath.Join(root, c.ExpectedIR))
			if err != nil {
				t.Fatal(err)
			}
			var expected schemaIRExpected
			if err := json.Unmarshal(raw, &expected); err != nil {
				t.Fatal(err)
			}
			builder, err := cambium.NewContextBuilder(cambium.ContextFlags{DisableSearchdirCwd: true})
			if err != nil {
				t.Fatal(err)
			}
			if err := builder.SearchPath(filepath.Join(root, c.Module)); err != nil {
				t.Fatal(err)
			}
			load := expected.Load
			if len(load) == 0 {
				load = []string{expected.Module}
			}
			for _, module := range load {
				if err := builder.LoadModule(module, nil, expected.Features[module]); err != nil {
					t.Fatalf("LoadModule(%s): %v", module, err)
				}
			}
			for module, features := range expected.Features {
				if err := builder.SetFeatures(module, features); err != nil {
					t.Fatal(err)
				}
			}
			ctx, err := builder.Build()
			if err != nil {
				t.Fatalf("Build: %v", err)
			}
			defer ctx.Close()
			mod, err := ctx.Schema(expected.Module)
			if err != nil {
				t.Fatal(err)
			}
			table := ctx.SchemaIRTable()
			if len(table.Errors) != 0 {
				t.Fatalf("table errors: %v", table.Errors)
			}
			byPath := make(map[string]cambium.SchemaIRTableNode, len(table.Nodes))
			for _, node := range table.Nodes {
				byPath[node.QualifiedPath] = node
			}
			view := func(ids []cambium.SchemaIRNodeID) []string {
				var out []string
				for _, id := range ids {
					out = append(out, table.Nodes[id].Name)
				}
				return out
			}
			lookup := func(path string) cambium.SchemaIRTableNode {
				node, ok := byPath[schemaIRNode(t, mod, path).QualifiedPath()]
				if !ok {
					t.Fatalf("%s missing from table", path)
				}
				return node
			}
			moduleChildren := func(name string) []cambium.SchemaIRNodeID {
				for _, m := range table.Modules {
					if m.Name == name {
						return m.Children
					}
				}
				t.Fatalf("module %s missing from table", name)
				return nil
			}
			for path, want := range expected.Children {
				ref := schemaIRNode(t, mod, path)
				if ref.Kind() == cambium.SchemaNodeKindModule {
					assertStringSlices(t, path+" table module children", view(moduleChildren(ref.Module().Name())), want)
				} else {
					assertStringSlices(t, path+" table children", view(lookup(path).Children), want)
				}
				var all, config []string
				for child := range ref.Children().Iter() {
					all = append(all, child.Name())
				}
				for child := range ref.Children().ConfigOnly().Iter() {
					config = append(config, child.Name())
				}
				if !isOrderedSubsequence(config, all) {
					t.Fatalf("%s ConfigOnly %v is not an ordered subsequence of %v", path, config, all)
				}
			}
			// The table has no module-level data view; module roots are
			// gated by the legacy fixture runner.
			for path, want := range expected.DataChildrenFlatten {
				if schemaIRNode(t, mod, path).Kind() == cambium.SchemaNodeKindModule {
					continue
				}
				assertStringSlices(t, path+" table data children", view(lookup(path).DataChildren), want)
			}
			for path, want := range expected.Keys {
				assertStringSlices(t, path+" table keys", view(lookup(path).ListKeys), want)
			}
		})
	}
	if ran == 0 {
		t.Fatal("no schema-ir fixtures ran")
	}
}

func isOrderedSubsequence(sub, all []string) bool {
	i := 0
	for _, name := range all {
		if i < len(sub) && sub[i] == name {
			i++
		}
	}
	return i == len(sub)
}
