// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

package cambium_test

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/signalbreak-labs/cambium/go/cambium"
)

// S1: traversal, grouping expansion, keys and config filtering.

const s1Walk = `module contract-walk {
  yang-version 1.1;
  namespace "urn:example:contract-walk";
  prefix w;

  grouping pair {
    leaf g-first { type string; }
    leaf g-last { type string; }
  }

  container root {
    leaf z { type string; }
    uses pair;
    list item {
      key "realm name";
      ordered-by user;
      leaf payload { type string; }
      leaf name { type string; }
      choice mode {
        case automatic { leaf auto { type boolean; } }
        case explicit { leaf manual { type string; } }
      }
      leaf realm { type string; }
      leaf state { config false; type uint32; }
      leaf tail { type string; }
    }
    leaf a { type string; }
  }
}`

func names(children cambium.SchemaChildren) string {
	var out []string
	for child := range children.Iter() {
		out = append(out, child.Name())
	}
	return strings.Join(out, ",")
}

func TestS1TraversalViews(t *testing.T) {
	ctx, err := buildPolicyContext(t, cambium.DeviationPolicy{}, s1Walk)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	mod, _ := ctx.Schema("contract-walk")
	root := schemaNodeAt(t, mod, "/w:root")
	item := schemaNodeAt(t, mod, "/w:root/item")

	cases := []struct {
		view string
		got  string
		want string
	}{
		{"root.Children()", names(root.Children()), "z,g-first,g-last,item,a"},
		{"item.Children()", names(item.Children()), "payload,name,mode,realm,state,tail"},
		{"item.DataChildren(true)", names(item.DataChildren(true)), "payload,name,auto,manual,realm,state,tail"},
		{"item.ListKeys()", names(item.ListKeys()), "realm,name"},
		{"item.Traverse(TraversalListEntryOrder)", names(item.Traverse(cambium.TraversalListEntryOrder)), "realm,name,payload,auto,manual,state,tail"},
		{"list-entry order, config only", names(item.Traverse(cambium.TraversalListEntryOrder).ConfigOnly()), "realm,name,payload,auto,manual,tail"},
		// Neither the structural nor the flattened view filters state on its own.
		{"item.Traverse(TraversalDataChildren)", names(item.Traverse(cambium.TraversalDataChildren)), "payload,name,auto,manual,realm,state,tail"},
	}
	for _, tc := range cases {
		if tc.got != tc.want {
			t.Errorf("%s = %s, want %s", tc.view, tc.got, tc.want)
		}
	}
	// Key-first child order within one entry is independent of the order
	// of list instances, which ordered-by user governs.
	if item.OrderedBy() != cambium.OrderedByUser {
		t.Fatalf("item ordered-by = %v, want user", item.OrderedBy())
	}
	state := schemaNodeAt(t, mod, "/w:root/item/state")
	if state.Config() != cambium.ConfigRo || state.RepresentsConfigurationData() {
		t.Fatal("state should be effective config false")
	}
	// A grouping-expanded node records its grouping origin.
	gFirst := schemaNodeAt(t, mod, "/w:root/g-first")
	if origin, ok := gFirst.GroupingOrigin(); !ok || origin != "pair" {
		t.Fatalf("g-first grouping origin = %q,%v", origin, ok)
	}
}

const s1Parent = `module walk-ext {
  yang-version 1.1;
  namespace "urn:example:walk-ext";
  prefix x;
  include walk-ext-sub;

  grouping inner {
    leaf i-one { type string; }
    container i-box { leaf deep { type string; } }
  }
  grouping outer {
    leaf o-first { type string; }
    uses inner {
      refine "i-box" { presence "refined presence"; }
      augment "i-box" { leaf added-by-uses { type string; } }
    }
    leaf o-last { type string; default "d"; }
  }

  container top {
    container np { leaf np-leaf { type string; } }
    container p { presence "enables p"; }
    uses outer;
    leaf-list tags { type string; ordered-by user; }
    choice shape {
      leaf implicit { type string; }
      case square { leaf side { type uint8; } }
    }
    list outer-list {
      key "id";
      leaf id { type string; }
      leaf info { type string; }
      list inner-list {
        key "k1 k2";
        leaf note { type string; }
        leaf k2 { type string; }
        leaf k1 { type string; }
        leaf required { type string; mandatory true; }
        leaf defaulted { type string; default "x"; }
        leaf counter { config false; type uint32; }
        container nested-state { config false; leaf s { type string; } }
      }
    }
    uses from-sub;
  }
}`

const s1Sub = `submodule walk-ext-sub {
  yang-version 1.1;
  belongs-to walk-ext { prefix x; }
  grouping from-sub { leaf sub-leaf { type string; } }
  container sub-top { leaf st { type string; } }
}`

func loadS1Extended(t *testing.T) cambium.Module {
	t.Helper()
	dir := t.TempDir()
	for name, source := range map[string]string{"walk-ext.yang": s1Parent, "walk-ext-sub.yang": s1Sub} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(source), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	builder, err := cambium.NewContextBuilder(cambium.ContextFlags{DisableSearchdirCwd: true})
	if err != nil {
		t.Fatal(err)
	}
	if err := builder.SearchPath(dir); err != nil {
		t.Fatal(err)
	}
	if err := builder.LoadModule("walk-ext", nil, nil); err != nil {
		t.Fatalf("LoadModule: %v", err)
	}
	ctx, err := builder.Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	t.Cleanup(ctx.Close)
	report := ctx.LoadReport()
	if len(report.IncludedSubmodules) != 1 || report.IncludedSubmodules[0].Name != "walk-ext-sub" || report.IncludedSubmodules[0].Parent != "walk-ext" {
		t.Fatalf("included submodules = %+v", report.IncludedSubmodules)
	}
	mod, err := ctx.Schema("walk-ext")
	if err != nil {
		t.Fatal(err)
	}
	return mod
}

func TestS1ExtendedExpansionOrder(t *testing.T) {
	mod := loadS1Extended(t)
	// Submodule content is spliced at its include statement, which precedes
	// top in the parent module.
	if got, want := names(mod.Children()), "sub-top,top"; got != want {
		t.Fatalf("module children = %s, want %s", got, want)
	}
	top := schemaNodeAt(t, mod, "/x:top")
	if got, want := names(top.Children()), "np,p,o-first,i-one,i-box,o-last,tags,shape,outer-list,sub-leaf"; got != want {
		t.Fatalf("top children = %s, want %s", got, want)
	}
	box := schemaNodeAt(t, mod, "/x:top/i-box")
	if got, want := names(box.Children()), "deep,added-by-uses"; got != want {
		t.Fatalf("i-box children = %s, want %s", got, want)
	}
	if !box.IsPresenceContainer() || schemaNodeAt(t, mod, "/x:top/np").IsPresenceContainer() || !schemaNodeAt(t, mod, "/x:top/p").IsPresenceContainer() {
		t.Fatal("presence facts wrong")
	}
	shape := schemaNodeAt(t, mod, "/x:top/shape")
	if got, want := names(shape.Children()), "implicit,square"; got != want {
		t.Fatalf("choice cases = %s, want %s", got, want)
	}
	implicitCase, _ := shape.Children().Get(0)
	if !implicitCase.IsCase() {
		t.Fatalf("implicit case kind = %v", implicitCase.Kind())
	}
	if got, want := names(top.DataChildren(true)), "np,p,o-first,i-one,i-box,o-last,tags,implicit,side,outer-list,sub-leaf"; got != want {
		t.Fatalf("top data children = %s, want %s", got, want)
	}
	tags := schemaNodeAt(t, mod, "/x:top/tags")
	if !tags.IsLeafList() || tags.OrderedBy() != cambium.OrderedByUser {
		t.Fatal("tags should be an ordered-by user leaf-list")
	}
	inner := schemaNodeAt(t, mod, "/x:top/outer-list/inner-list")
	if got, want := names(inner.Traverse(cambium.TraversalListEntryOrder)), "k1,k2,note,required,defaulted,counter,nested-state"; got != want {
		t.Fatalf("inner-list entry order = %s, want %s", got, want)
	}
	if got, want := names(inner.Traverse(cambium.TraversalListEntryOrder).ConfigOnly()), "k1,k2,note,required,defaulted"; got != want {
		t.Fatalf("inner-list config entry order = %s, want %s", got, want)
	}
	nestedLeaf := schemaNodeAt(t, mod, "/x:top/outer-list/inner-list/nested-state/s")
	if nestedLeaf.Config() != cambium.ConfigRo {
		t.Fatal("inherited config false not effective")
	}
}

func projectedNames(p cambium.Projection) string {
	var out []string
	p.WalkPreOrder(func(n cambium.ProjectedNode) bool {
		out = append(out, n.Node.Name())
		return true
	})
	return strings.Join(out, ",")
}

func TestS1ProjectionContract(t *testing.T) {
	mod := loadS1Extended(t)
	const innerPath = "/x:top/outer-list/inner-list"

	// Selecting a nested list keeps its ancestors and every list key, and
	// orders keys first.
	p, err := cambium.ProjectSchemaPaths(mod, []string{innerPath}, cambium.DefaultProjectionOptions())
	if err != nil {
		t.Fatalf("ProjectSchemaPaths: %v", err)
	}
	if got, want := projectedNames(p), "top,outer-list,id,inner-list,k1,k2,note,required,defaulted,counter,nested-state,s"; got != want {
		t.Fatalf("default projection = %s, want %s", got, want)
	}
	var roles []cambium.ProjectionRole
	p.WalkPreOrder(func(n cambium.ProjectedNode) bool {
		if n.Node.Name() == "id" || n.Node.Name() == "top" {
			roles = append(roles, n.Role)
		}
		return true
	})
	if !roles[0].Has(cambium.ProjectionRoleAncestor) || !roles[1].Has(cambium.ProjectionRoleKey) {
		t.Fatalf("roles = %v, want ancestor then key", roles)
	}

	// Explicit effective-config filtering; the default options do not filter.
	opts := cambium.DefaultProjectionOptions()
	opts.ConfigOnly = true
	p, err = cambium.ProjectSchemaPaths(mod, []string{innerPath}, opts)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := projectedNames(p), "top,outer-list,id,inner-list,k1,k2,note,required,defaulted"; got != want {
		t.Fatalf("config-only projection = %s, want %s", got, want)
	}

	// Ignore wins over allow; keys stay protected.
	opts = cambium.DefaultProjectionOptions()
	opts.AllowRelativePaths = []string{"note", "defaulted", "k1"}
	opts.IgnoreRelativePaths = []string{"note", "k2"}
	p, err = cambium.ProjectSchemaPaths(mod, []string{innerPath}, opts)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := projectedNames(p), "top,outer-list,id,inner-list,k1,k2,defaulted"; got != want {
		t.Fatalf("allow/ignore projection = %s, want %s", got, want)
	}

	// Protected mandatory survives an ignore filter.
	opts.ProtectMandatory = true
	opts.IgnoreRelativePaths = append(opts.IgnoreRelativePaths, "required")
	p, err = cambium.ProjectSchemaPaths(mod, []string{innerPath}, opts)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := projectedNames(p), "top,outer-list,id,inner-list,k1,k2,required,defaulted"; got != want {
		t.Fatalf("protected mandatory projection = %s, want %s", got, want)
	}

	// Mandatory/default inclusion without full descendants.
	opts = cambium.ProjectionOptions{IncludeListKeys: true, ListKeysFirst: true, FlattenChoices: true, IncludeMandatory: true, IncludeDefaults: true}
	p, err = cambium.ProjectSchemaPaths(mod, []string{innerPath}, opts)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := projectedNames(p), "top,outer-list,id,inner-list,k1,k2,required,defaulted"; got != want {
		t.Fatalf("mandatory/default projection = %s, want %s", got, want)
	}

	// Multiple selections merge in schema order, not input order.
	p, err = cambium.ProjectSchemaPaths(mod, []string{"/x:top/tags", "/x:top/np"}, cambium.DefaultProjectionOptions())
	if err != nil {
		t.Fatal(err)
	}
	if got, want := projectedNames(p), "top,np,np-leaf,tags"; got != want {
		t.Fatalf("multi-selection = %s, want %s", got, want)
	}

	// Choice handling is deliberate: structural when not flattening.
	// Selections must be data nodes, so a choice is reached through a filter.
	if _, err := cambium.ProjectSchemaPaths(mod, []string{"/x:top/shape"}, cambium.DefaultProjectionOptions()); err == nil {
		t.Fatal("selecting a choice succeeded, want not-a-data-node error")
	}
	opts = cambium.DefaultProjectionOptions()
	opts.FlattenChoices = false
	opts.AllowRelativePaths = []string{"shape"}
	p, err = cambium.ProjectSchemaPaths(mod, []string{"/x:top"}, opts)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := projectedNames(p), "top,shape,implicit,implicit,square,side"; got != want {
		t.Fatalf("structural choice projection = %s, want %s", got, want)
	}

	// Absence differs from an empty projection: an unknown path is an
	// error; a config-false selection under ConfigOnly is a valid empty result.
	if _, err := cambium.ProjectSchemaPaths(mod, []string{"/x:top/nope"}, cambium.DefaultProjectionOptions()); err == nil {
		t.Fatal("unknown selection path succeeded")
	}
	opts = cambium.DefaultProjectionOptions()
	opts.ConfigOnly = true
	p, err = cambium.ProjectSchemaPaths(mod, []string{innerPath + "/counter"}, opts)
	if err != nil {
		t.Fatalf("config-false selection: %v", err)
	}
	if len(p.Roots) != 0 {
		t.Fatalf("config-false selection under ConfigOnly = %s, want empty", projectedNames(p))
	}

	// Invalid filters are errors unless explicitly ignored.
	opts = cambium.DefaultProjectionOptions()
	opts.AllowRelativePaths = []string{"no-such-leaf"}
	if _, err := cambium.ProjectSchemaPaths(mod, []string{innerPath}, opts); err == nil || !strings.Contains(err.Error(), "no-such-leaf") {
		t.Fatalf("invalid filter error = %v", err)
	}
	opts.IgnoreInvalidFilters = true
	if _, err := cambium.ProjectSchemaPaths(mod, []string{innerPath}, opts); err != nil {
		t.Fatalf("ignored invalid filter: %v", err)
	}
}

func TestS1ProjectedChildrenMatchReflectOrder(t *testing.T) {
	mod := loadS1Extended(t)
	top := schemaNodeAt(t, mod, "/x:top")
	projected, err := cambium.ProjectSubtree(top, cambium.DefaultProjectionOptions())
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, child := range projected.Children {
		got = append(got, child.Node.Name())
	}
	var want []string
	for child := range top.Traverse(cambium.TraversalDataChildren).Iter() {
		want = append(want, child.Name())
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("projected children %v differ from data children %v", got, want)
	}
}
