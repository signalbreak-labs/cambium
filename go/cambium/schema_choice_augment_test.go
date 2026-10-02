// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

package cambium_test

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/signalbreak-labs/cambium/go/cambium"
)

// An augment of a choice accepts the same shorthand cases as a choice
// declaration (RFC 7950 sections 7.9.2 and 7.17). Every shorthand contributes
// a case plus its data node, even though payload traversal skips the case.
func TestChoiceAugmentMaterializesShorthandCases(t *testing.T) {
	const base = `module selection {
  yang-version 1.1; namespace "urn:selection"; prefix sel;
  grouping alternatives {
    choice target { case original { leaf first { type string; } } }
  }
  rpc operate { input { uses alternatives; } }
}`
	const additions = `
    leaf scalar {
      if-feature extra;
      type string;
      description "Augmented scalar.";
      meta:mark "scalar-source";
    }
    case explicit {
      meta:mark "case-source";
      leaf explicit-value { type string; }
    }
    container object { leaf value { type string; } }
    list entries { key name; leaf name { type string; } }
    leaf-list values { type string; }
    anyxml xml-content;
    anydata data-content;
    choice nested { case inner { leaf inner-value { type string; } } }
`
	const extensionHeader = `module metadata {
  yang-version 1.1; namespace "urn:metadata"; prefix meta;
  import selection { prefix sel; }
  extension mark { argument value; }
  feature extra;
`
	for _, scope := range []struct {
		name, source, module, path string
		config                     cambium.Config
	}{
		{
			name:   "module augment",
			source: extensionHeader + `augment "/sel:operate/sel:input/sel:target" {` + additions + `}}`,
			module: "selection", path: "/sel:operate/sel:input/sel:target", config: cambium.ConfigRw,
		},
		{
			name: "uses augment",
			source: extensionHeader + `container state { config false;
    uses sel:alternatives { augment target {` + additions + `} }
  }}`,
			module: "metadata", path: "/meta:state/meta:target", config: cambium.ConfigRo,
		},
	} {
		for _, mode := range bothValidationModes {
			t.Run(fmt.Sprintf("%s/%d", scope.name, mode), func(t *testing.T) {
				ctx, err := buildModeContext(t, mode, map[string][]string{"metadata": {"extra"}}, base, scope.source)
				if err != nil {
					t.Fatal(err)
				}
				mod, err := ctx.Schema(scope.module)
				if err != nil {
					t.Fatal(err)
				}
				choice := schemaNodeAt(t, mod, scope.path)
				assertChildNames(t, mod, scope.path, "original", "scalar", "explicit", "object", "entries", "values", "xml-content", "data-content", "nested")
				for _, want := range []struct {
					name string
					kind cambium.SchemaNodeKind
				}{
					{"scalar", cambium.SchemaNodeKindLeaf},
					{"object", cambium.SchemaNodeKindContainer},
					{"entries", cambium.SchemaNodeKindList},
					{"values", cambium.SchemaNodeKindLeafList},
					{"xml-content", cambium.SchemaNodeKindAnyXML},
					{"data-content", cambium.SchemaNodeKindAnyData},
					{"nested", cambium.SchemaNodeKindChoice},
				} {
					branch := childByName(t, choice.Children(), want.name)
					if branch.Kind() != cambium.SchemaNodeKindCase || branch.Statement().IsValid() {
						t.Fatalf("%s: kind = %v, explicit = %v; want implicit case", want.name, branch.Kind(), branch.Statement().IsValid())
					}
					if branch.Children().Len() != 1 {
						t.Fatalf("%s: case children = %d, want 1", want.name, branch.Children().Len())
					}
					child := childByName(t, branch.Children(), want.name)
					if child.Kind() != want.kind || !child.Statement().IsValid() {
						t.Fatalf("%s: data kind = %v, explicit = %v; want %v with source", want.name, child.Kind(), child.Statement().IsValid(), want.kind)
					}
					for _, node := range []cambium.SchemaNodeRef{branch, child} {
						if node.Module().Name() != "metadata" || !node.IsChoiceDescendant() || node.Config() != scope.config {
							t.Errorf("%s: module = %q, choice descendant = %v, config = %v", node.Path(), node.Module().Name(), node.IsChoiceDescendant(), node.Config())
						}
					}
					if got, ok := child.Parent(); !ok || got != branch {
						t.Errorf("%s: data parent is not its implicit case", want.name)
					}
					if resolved := schemaNodeAt(t, mod, scope.path+"/metadata:"+want.name+"/metadata:"+want.name); resolved != child {
						t.Errorf("%s: schema path did not resolve to data child", want.name)
					}
				}
				explicit := childByName(t, choice.Children(), "explicit")
				if explicit.Kind() != cambium.SchemaNodeKindCase || !explicit.Statement().IsValid() {
					t.Fatalf("explicit case was wrapped or lost its source: %v", explicit.Kind())
				}
				if exts := explicit.MatchingExtensions("metadata", "mark"); len(exts) != 1 {
					t.Fatalf("explicit case extensions = %v, want source marker", exts)
				}
				scalarCase := childByName(t, choice.Children(), "scalar")
				scalar := childByName(t, scalarCase.Children(), "scalar")
				if scalarCase.Extensions() != nil {
					t.Fatal("data extension was copied onto implicit case")
				}
				if exts := scalar.MatchingExtensions("metadata", "mark"); len(exts) != 1 {
					t.Fatalf("scalar extensions = %v, want declaration-scope source marker", exts)
				}
				if got, ok := scalar.Description(); !ok || got != "Augmented scalar." {
					t.Errorf("scalar description = (%q, %v)", got, ok)
				}
				for _, node := range []cambium.SchemaNodeRef{scalarCase, scalar} {
					if got := node.IfFeatures(); !reflect.DeepEqual(got, []string{"extra"}) {
						t.Errorf("%s: if-features = %v, want [extra]", node.Path(), got)
					}
				}
				wantData := []string{"first", "scalar", "explicit-value", "object", "entries", "values", "xml-content", "data-content", "inner-value"}
				if got := schemaChildNames(choice.DataChildren(true)); !reflect.DeepEqual(got, wantData) {
					t.Errorf("payload order = %v, want %v", got, wantData)
				}
			})
		}
	}
}
