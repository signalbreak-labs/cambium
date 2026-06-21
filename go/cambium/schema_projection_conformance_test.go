package cambium_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/signalbreak-labs/cambium/go/cambium"
	"github.com/signalbreak-labs/cambium/go/internal/confmanifest"
)

func TestProjectionConformanceManifestFixtures(t *testing.T) {
	root := conformanceRoot(t)
	cases, err := confmanifest.Load(filepath.Join(root, "manifest.toml"))
	if err != nil {
		t.Fatalf("Load manifest: %v", err)
	}

	totalModules := 0
	totalRoots := 0
	totalSelections := 0
	for _, c := range cases {
		t.Run(c.Name, func(t *testing.T) {
			ctx := loadProjectionConformanceContext(t, root, c)
			modules := ctx.Modules()
			if len(modules) == 0 {
				t.Fatalf("%s loaded no implemented modules in the pure schema context", c.Name)
			}
			for _, mod := range modules {
				roots, selections := assertProjectionConformanceModule(t, mod)
				totalModules++
				totalRoots += roots
				totalSelections += selections
			}
		})
	}

	if totalModules == 0 || totalRoots == 0 || totalSelections == 0 {
		t.Fatalf("projection conformance tested modules=%d roots=%d selections=%d, want non-zero coverage", totalModules, totalRoots, totalSelections)
	}
}

func loadProjectionConformanceContext(t *testing.T, root string, c confmanifest.Case) *cambium.Context {
	t.Helper()

	builder, err := cambium.NewContextBuilder(cambium.ContextFlags{})
	if err != nil {
		t.Fatal(err)
	}

	moduleDir := filepath.Join(root, c.Module)
	if err := builder.SearchPath(moduleDir); err != nil {
		t.Fatalf("SetSearchPath(%s): %v", moduleDir, err)
	}

	var load []string
	features := map[string][]string{}
	if c.EffectiveTier() == confmanifest.TierSchemaIR {
		raw, err := os.ReadFile(filepath.Join(root, c.ExpectedIR))
		if err != nil {
			t.Fatal(err)
		}
		var expected schemaIRExpected
		if err := json.Unmarshal(raw, &expected); err != nil {
			t.Fatal(err)
		}
		load = expected.Load
		if len(load) == 0 {
			load = []string{expected.Module}
		}
		features = expected.Features
	} else {
		load = projectionConformanceBackendModules(t, c, moduleDir)
	}

	for _, module := range load {
		if err := builder.LoadModule(module, nil, features[module]); err != nil {
			t.Fatalf("LoadModule(%s): %v", module, err)
		}
	}
	ctx, err := builder.Build()
	if err != nil {
		t.Fatalf("Build context: %v", err)
	}
	t.Cleanup(func() { ctx.Close() })
	return ctx
}

func projectionConformanceBackendModules(t *testing.T, c confmanifest.Case, moduleDir string) []string {
	t.Helper()
	if c.Name == "ietf-interfaces" && c.Module == "corpus/ietf-interfaces" {
		return []string{"ietf-interfaces"}
	}
	load, err := projectionConformanceModuleNames(moduleDir)
	if err != nil {
		t.Fatal(err)
	}
	return load
}

func projectionConformanceModuleNames(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var names []string
	for _, entry := range entries {
		if filepath.Ext(entry.Name()) != ".yang" {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		if projectionConformanceIsSubmodule(path) {
			continue
		}
		name := strings.TrimSuffix(entry.Name(), ".yang")
		if at := strings.IndexByte(name, '@'); at >= 0 {
			name = name[:at]
		}
		names = append(names, name)
	}
	sort.Strings(names)
	return names, nil
}

func projectionConformanceIsSubmodule(path string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	return strings.HasPrefix(strings.TrimSpace(string(data)), "submodule ")
}

func assertProjectionConformanceModule(t *testing.T, mod cambium.Module) (roots, selections int) {
	t.Helper()

	opts := cambium.DefaultProjectionOptions()
	opts.IncludeMandatory = true
	opts.IncludeDefaults = true

	for top := range mod.TopLevel().Iter() {
		if !top.IsDataNode() {
			continue
		}
		roots++
		subtree, err := cambium.ProjectSubtree(top, opts)
		if err != nil {
			t.Fatalf("%s ProjectSubtree(%s): %v", mod.Name(), top.Path(), err)
		}
		assertProjectionConformanceFullSubtree(t, subtree, opts, true)
	}

	for _, node := range projectionConformanceDataNodes(mod) {
		selections++
		path := projectionConformanceSchemaPath(node)
		projection, err := cambium.ProjectSchemaPaths(mod, []string{path}, cambium.ProjectionOptions{
			FlattenChoices:     true,
			IncludeListKeys:    true,
			ListKeysFirst:      true,
			IncludeDescendants: false,
		})
		if err != nil {
			t.Fatalf("%s ProjectSchemaPaths(%s): %v", mod.Name(), path, err)
		}
		assertProjectionConformanceSelection(t, projection, node)
	}

	return roots, selections
}

func assertProjectionConformanceFullSubtree(t *testing.T, node cambium.ProjectedNode, opts cambium.ProjectionOptions, selected bool) {
	t.Helper()

	if selected != node.Role.Has(cambium.ProjectionRoleSelected) {
		t.Fatalf("%s selected role = %v, want %v", node.Node.Path(), node.Role.Has(cambium.ProjectionRoleSelected), selected)
	}
	if node.Node.IsListKey() && !node.Role.Has(cambium.ProjectionRoleKey) {
		t.Fatalf("%s is a list key but projection role lacks key", node.Node.Path())
	}
	if got, want := node.Role.Has(cambium.ProjectionRoleMandatory), projectionConformanceMandatoryNode(node.Node); got != want {
		t.Fatalf("%s mandatory role = %v, want %v", node.Node.Path(), got, want)
	}
	if got, want := node.Role.Has(cambium.ProjectionRoleDefault), projectionConformanceDefaultNode(node.Node); got != want {
		t.Fatalf("%s default role = %v, want %v", node.Node.Path(), got, want)
	}

	wantChildren := projectionConformanceExpectedChildren(node.Node, opts)
	if len(node.Children) != len(wantChildren) {
		t.Fatalf("%s children len = %d, want %d\ngot:  %v\nwant: %v", node.Node.Path(), len(node.Children), len(wantChildren), projectedNodePaths(node.Children), schemaNodePaths(wantChildren))
	}
	for i, want := range wantChildren {
		if node.Children[i].Node != want {
			t.Fatalf("%s child[%d] = %s, want %s\ngot:  %v\nwant: %v", node.Node.Path(), i, node.Children[i].Node.Path(), want.Path(), projectedNodePaths(node.Children), schemaNodePaths(wantChildren))
		}
		assertProjectionConformanceFullSubtree(t, node.Children[i], opts, false)
	}
}

func projectionConformanceExpectedChildren(node cambium.SchemaNodeRef, opts cambium.ProjectionOptions) []cambium.SchemaNodeRef {
	emitted := map[cambium.SchemaNodeRef]bool{}
	var out []cambium.SchemaNodeRef
	appendNode := func(child cambium.SchemaNodeRef) {
		if emitted[child] {
			return
		}
		emitted[child] = true
		out = append(out, child)
	}
	if opts.ListKeysFirst && node.IsList() {
		for key := range node.ListKeys().Iter() {
			appendNode(key)
		}
	}
	for child := range node.DataChildren(opts.FlattenChoices).Iter() {
		appendNode(child)
	}
	return out
}

func assertProjectionConformanceSelection(t *testing.T, projection cambium.Projection, selected cambium.SchemaNodeRef) {
	t.Helper()

	var selectedCount int
	var sequence []cambium.SchemaNodeRef
	projection.WalkPreOrder(func(node cambium.ProjectedNode) bool {
		sequence = append(sequence, node.Node)
		if node.Role.Has(cambium.ProjectionRoleSelected) {
			selectedCount++
			if node.Node != selected {
				t.Fatalf("selected node = %s, want %s", node.Node.Path(), selected.Path())
			}
		}
		return true
	})
	if selectedCount != 1 {
		t.Fatalf("%s selected role count = %d, want 1 in %v", selected.Path(), selectedCount, schemaNodePaths(sequence))
	}

	wantSpine := append(selected.DataAncestors(), selected)
	if !projectionConformanceContainsOrderedSubsequence(sequence, wantSpine) {
		t.Fatalf("%s projection sequence does not contain data spine\ngot:  %v\nwant: %v", selected.Path(), schemaNodePaths(sequence), schemaNodePaths(wantSpine))
	}
}

func projectionConformanceDataNodes(mod cambium.Module) []cambium.SchemaNodeRef {
	var out []cambium.SchemaNodeRef
	var walk func(cambium.SchemaNodeRef)
	walk = func(node cambium.SchemaNodeRef) {
		if node.IsDataNode() {
			out = append(out, node)
		}
		for child := range node.Children().Iter() {
			walk(child)
		}
	}
	for top := range mod.TopLevel().Iter() {
		walk(top)
	}
	return out
}

func projectionConformanceContainsOrderedSubsequence(got, want []cambium.SchemaNodeRef) bool {
	if len(want) == 0 {
		return true
	}
	i := 0
	for _, node := range got {
		if node == want[i] {
			i++
			if i == len(want) {
				return true
			}
		}
	}
	return false
}

func projectionConformanceSchemaPath(node cambium.SchemaNodeRef) string {
	refs := append(node.Ancestors(), node)
	parts := make([]string, 0, len(refs))
	for _, ref := range refs {
		prefix := ref.Module().Prefix()
		if prefix == "" {
			prefix = ref.Module().Name()
		}
		parts = append(parts, prefix+":"+ref.Name())
	}
	return "/" + strings.Join(parts, "/")
}

func projectionConformanceMandatoryNode(node cambium.SchemaNodeRef) bool {
	if node.IsMandatory() {
		return true
	}
	if node.IsList() || node.IsLeafList() {
		if min, ok := node.MinElements(); ok && min > 0 {
			return true
		}
	}
	return false
}

func projectionConformanceDefaultNode(node cambium.SchemaNodeRef) bool {
	_, ok := node.DefaultEntry()
	return ok
}

func projectedNodePaths(nodes []cambium.ProjectedNode) []string {
	out := make([]string, len(nodes))
	for i, node := range nodes {
		out[i] = node.Node.Path()
	}
	return out
}

func schemaNodePaths(nodes []cambium.SchemaNodeRef) []string {
	out := make([]string, len(nodes))
	for i, node := range nodes {
		out[i] = node.Path()
	}
	return out
}
