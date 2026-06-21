// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

package cambium_test

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/signalbreak-labs/cambium/go/cambium"
)

// deviationTestDir returns the temporary module directory used by deviation tests.
func deviationTestDir(t *testing.T) string {
	t.Helper()
	dir := filepath.Join("..", "..", "target", "tests", "deviation", "modules")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestDeviationDeleteRemovesOneDefault(t *testing.T) {
	t.Helper()
	dir := deviationTestDir(t)
	writeModuleFile(t, filepath.Join(dir, "cambium-deviate-delete-default-target.yang"), []byte(`module cambium-deviate-delete-default-target {
    namespace "urn:cambium:deviate-delete-default-target";
    prefix cdddt;
    yang-version 1.1;

    leaf value {
        type string;
        default "a";
        default "b";
    }
}
`))
	writeModuleFile(t, filepath.Join(dir, "cambium-deviate-delete-default-source.yang"), []byte(`module cambium-deviate-delete-default-source {
    namespace "urn:cambium:deviate-delete-default-source";
    prefix cddds;
    yang-version 1.1;

    import cambium-deviate-delete-default-target {
        prefix tgt;
    }

    deviation "/tgt:value" {
        deviate delete {
            default "a";
        }
    }
}
`))

	ctx, err := cambium.NewContext()
	if err != nil {
		t.Fatal(err)
	}
	defer ctx.Close()
	if err := ctx.SetSearchPath(dir); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"cambium-deviate-delete-default-target", "cambium-deviate-delete-default-source"} {
		if err := ctx.LoadModule(name); err != nil {
			t.Fatalf("LoadModule %s: %v", name, err)
		}
	}

	mod, err := ctx.Schema("cambium-deviate-delete-default-target")
	if err != nil {
		t.Fatalf("Schema: %v", err)
	}
	leaf, err := mod.FindPath("/cdddt:value")
	if err != nil {
		t.Fatalf("FindPath value: %v", err)
	}
	got := leaf.DefaultValues()
	want := []string{"b"}
	if len(got) != len(want) || (len(got) > 0 && got[0] != want[0]) {
		t.Fatalf("DefaultValues() = %v, want %v", got, want)
	}
}

func TestDeviationDeleteRemovesOneMust(t *testing.T) {
	t.Helper()
	dir := deviationTestDir(t)
	writeModuleFile(t, filepath.Join(dir, "cambium-deviate-delete-must-target.yang"), []byte(`module cambium-deviate-delete-must-target {
    namespace "urn:cambium:deviate-delete-must-target";
    prefix cddmt;
    yang-version 1.1;

    leaf value {
        type string;
        must "../value = 'a'";
        must "../value = 'b'";
    }
}
`))
	writeModuleFile(t, filepath.Join(dir, "cambium-deviate-delete-must-source.yang"), []byte(`module cambium-deviate-delete-must-source {
    namespace "urn:cambium:deviate-delete-must-source";
    prefix cddms;
    yang-version 1.1;

    import cambium-deviate-delete-must-target {
        prefix tgt;
    }

    deviation "/tgt:value" {
        deviate delete {
            must "../value = 'a'";
        }
    }
}
`))

	ctx, err := cambium.NewContext()
	if err != nil {
		t.Fatal(err)
	}
	defer ctx.Close()
	if err := ctx.SetSearchPath(dir); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"cambium-deviate-delete-must-target", "cambium-deviate-delete-must-source"} {
		if err := ctx.LoadModule(name); err != nil {
			t.Fatalf("LoadModule %s: %v", name, err)
		}
	}

	mod, err := ctx.Schema("cambium-deviate-delete-must-target")
	if err != nil {
		t.Fatalf("Schema: %v", err)
	}
	leaf, err := mod.FindPath("/cddmt:value")
	if err != nil {
		t.Fatalf("FindPath value: %v", err)
	}
	musts := leaf.Musts()
	if len(musts) != 1 || musts[0].Expression() != "../value = 'b'" {
		t.Fatalf("Musts() = %v, want one must with expression ../value = 'b'", musts)
	}
}

func TestDeviationDeleteRemovesOneUnique(t *testing.T) {
	t.Helper()
	dir := deviationTestDir(t)
	writeModuleFile(t, filepath.Join(dir, "cambium-deviate-delete-unique-target.yang"), []byte(`module cambium-deviate-delete-unique-target {
    namespace "urn:cambium:deviate-delete-unique-target";
    prefix cddut;
    yang-version 1.1;

    list item {
        key "name";
        leaf name { type string; }
        leaf color { type string; }
        leaf size { type string; }
        unique "name color";
        unique "name size";
    }
}
`))
	writeModuleFile(t, filepath.Join(dir, "cambium-deviate-delete-unique-source.yang"), []byte(`module cambium-deviate-delete-unique-source {
    namespace "urn:cambium:deviate-delete-unique-source";
    prefix cddus;
    yang-version 1.1;

    import cambium-deviate-delete-unique-target {
        prefix tgt;
    }

    deviation "/tgt:item" {
        deviate delete {
            unique "name color";
        }
    }
}
`))

	ctx, err := cambium.NewContext()
	if err != nil {
		t.Fatal(err)
	}
	defer ctx.Close()
	if err := ctx.SetSearchPath(dir); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"cambium-deviate-delete-unique-target", "cambium-deviate-delete-unique-source"} {
		if err := ctx.LoadModule(name); err != nil {
			t.Fatalf("LoadModule %s: %v", name, err)
		}
	}

	mod, err := ctx.Schema("cambium-deviate-delete-unique-target")
	if err != nil {
		t.Fatalf("Schema: %v", err)
	}
	listNode, err := mod.FindPath("/cddut:item")
	if err != nil {
		t.Fatalf("FindPath item: %v", err)
	}
	uniques := listNode.UniqueConstraints()
	if len(uniques) != 1 {
		t.Fatalf("UniqueConstraints() len = %d, want 1", len(uniques))
	}
	leafs := uniques[0].Leafs()
	want := []string{"name", "size"}
	if len(leafs) != len(want) {
		t.Fatalf("remaining unique leafs = %v, want %v", leafNames(leafs), want)
	}
	for i, name := range want {
		if leafs[i].Name() != name {
			t.Fatalf("remaining unique leafs = %v, want %v", leafNames(leafs), want)
		}
	}
}

func TestDeviationReplaceTypeUsesSourceModuleTypedef(t *testing.T) {
	t.Helper()
	dir := deviationTestDir(t)
	writeModuleFile(t, filepath.Join(dir, "cambium-deviate-replace-type-target.yang"), []byte(`module cambium-deviate-replace-type-target {
    namespace "urn:cambium:deviate-replace-type-target";
    prefix cdrtt;
    yang-version 1.1;

    leaf value {
        type string;
    }
}
`))
	writeModuleFile(t, filepath.Join(dir, "cambium-deviate-replace-type-source.yang"), []byte(`module cambium-deviate-replace-type-source {
    namespace "urn:cambium:deviate-replace-type-source";
    prefix cdrts;
    yang-version 1.1;

    import cambium-deviate-replace-type-target {
        prefix tgt;
    }

    typedef my-type {
        type string {
            pattern "[a-z]+";
        }
    }

    deviation "/tgt:value" {
        deviate replace {
            type my-type;
        }
    }
}
`))

	ctx, err := cambium.NewContext()
	if err != nil {
		t.Fatal(err)
	}
	defer ctx.Close()
	if err := ctx.SetSearchPath(dir); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"cambium-deviate-replace-type-target", "cambium-deviate-replace-type-source"} {
		if err := ctx.LoadModule(name); err != nil {
			t.Fatalf("LoadModule %s: %v", name, err)
		}
	}

	mod, err := ctx.Schema("cambium-deviate-replace-type-target")
	if err != nil {
		t.Fatalf("Schema: %v", err)
	}
	leaf, err := mod.FindPath("/cdrtt:value")
	if err != nil {
		t.Fatalf("FindPath value: %v", err)
	}
	assertPattern(t, leaf, "[a-z]+")
}

func TestDeviationReplaceTypeUsesPrefixedTargetTypedef(t *testing.T) {
	t.Helper()
	dir := deviationTestDir(t)
	writeModuleFile(t, filepath.Join(dir, "cambium-deviate-replace-type-cross-target.yang"), []byte(`module cambium-deviate-replace-type-cross-target {
    namespace "urn:cambium:deviate-replace-type-cross-target";
    prefix cdrttc;
    yang-version 1.1;

    typedef target-type {
        type string {
            pattern "[0-9]+";
        }
    }

    leaf value {
        type string;
    }
}
`))
	writeModuleFile(t, filepath.Join(dir, "cambium-deviate-replace-type-cross-source.yang"), []byte(`module cambium-deviate-replace-type-cross-source {
    namespace "urn:cambium:deviate-replace-type-cross-source";
    prefix cdrttcs;
    yang-version 1.1;

    import cambium-deviate-replace-type-cross-target {
        prefix tgt;
    }

    deviation "/tgt:value" {
        deviate replace {
            type tgt:target-type;
        }
    }
}
`))

	ctx, err := cambium.NewContext()
	if err != nil {
		t.Fatal(err)
	}
	defer ctx.Close()
	if err := ctx.SetSearchPath(dir); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"cambium-deviate-replace-type-cross-target", "cambium-deviate-replace-type-cross-source"} {
		if err := ctx.LoadModule(name); err != nil {
			t.Fatalf("LoadModule %s: %v", name, err)
		}
	}

	mod, err := ctx.Schema("cambium-deviate-replace-type-cross-target")
	if err != nil {
		t.Fatalf("Schema: %v", err)
	}
	leaf, err := mod.FindPath("/cdrttc:value")
	if err != nil {
		t.Fatalf("FindPath value: %v", err)
	}
	assertPattern(t, leaf, "[0-9]+")
}

func TestDeviationNotSupportedRemovesTopLevelNode(t *testing.T) {
	t.Helper()
	dir := deviationTestDir(t)
	writeModuleFile(t, filepath.Join(dir, "cambium-deviate-not-supported-target.yang"), []byte(`module cambium-deviate-not-supported-target {
    namespace "urn:cambium:deviate-not-supported-target";
    prefix cdnst;
    yang-version 1.1;

    leaf foo { type string; }
    leaf bar { type string; }
}
`))
	writeModuleFile(t, filepath.Join(dir, "cambium-deviate-not-supported-source.yang"), []byte(`module cambium-deviate-not-supported-source {
    namespace "urn:cambium:deviate-not-supported-source";
    prefix cdnss;
    yang-version 1.1;

    import cambium-deviate-not-supported-target {
        prefix tgt;
    }

    deviation "/tgt:foo" {
        deviate not-supported;
    }
}
`))

	ctx, err := cambium.NewContext()
	if err != nil {
		t.Fatal(err)
	}
	defer ctx.Close()
	if err := ctx.SetSearchPath(dir); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"cambium-deviate-not-supported-target", "cambium-deviate-not-supported-source"} {
		if err := ctx.LoadModule(name); err != nil {
			t.Fatalf("LoadModule %s: %v", name, err)
		}
	}

	mod, err := ctx.Schema("cambium-deviate-not-supported-target")
	if err != nil {
		t.Fatalf("Schema: %v", err)
	}

	got := schemaChildNames(mod.TopLevel())
	want := []string{"bar"}
	if len(got) != len(want) || got[0] != want[0] {
		t.Fatalf("TopLevel() = %v, want %v", got, want)
	}
	if _, err := mod.FindPath("/cdnst:foo"); err == nil {
		t.Fatal("FindPath /cdnst:foo should fail after not-supported deviation")
	}
	if _, err := mod.FindPath("/cdnst:bar"); err != nil {
		t.Fatalf("FindPath /cdnst:bar: %v", err)
	}
}

func TestDeviationAddOnGroupingUseDoesNotAliasOtherUses(t *testing.T) {
	t.Helper()
	dir := deviationTestDir(t)
	writeModuleFile(t, filepath.Join(dir, "cambium-deviate-grouping-isolation-target.yang"), []byte(`module cambium-deviate-grouping-isolation-target {
    namespace "urn:cambium:deviate-grouping-isolation-target";
    prefix cdgit;
    yang-version 1.1;

    grouping common {
        list item {
            key "name";
            leaf name { type string; }
        }
        leaf value {
            type string;
        }
    }

    container c1 {
        uses common;
    }

    container c2 {
        uses common;
    }
}
`))
	writeModuleFile(t, filepath.Join(dir, "cambium-deviate-grouping-isolation-source.yang"), []byte(`module cambium-deviate-grouping-isolation-source {
    namespace "urn:cambium:deviate-grouping-isolation-source";
    prefix cdgis;
    yang-version 1.1;

    import cambium-deviate-grouping-isolation-target {
        prefix tgt;
    }

    deviation "/tgt:c1/tgt:item" {
        deviate add {
            min-elements 1;
            max-elements 5;
        }
    }

    deviation "/tgt:c2/tgt:value" {
        deviate add {
            default "only-c2";
        }
    }
}
`))

	ctx, err := cambium.NewContext()
	if err != nil {
		t.Fatal(err)
	}
	defer ctx.Close()
	if err := ctx.SetSearchPath(dir); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"cambium-deviate-grouping-isolation-target", "cambium-deviate-grouping-isolation-source"} {
		if err := ctx.LoadModule(name); err != nil {
			t.Fatalf("LoadModule %s: %v", name, err)
		}
	}

	mod, err := ctx.Schema("cambium-deviate-grouping-isolation-target")
	if err != nil {
		t.Fatalf("Schema: %v", err)
	}
	c1Item := mustFindPath(t, mod, "/cdgit:c1/cdgit:item")
	c2Item := mustFindPath(t, mod, "/cdgit:c2/cdgit:item")
	c1Value := mustFindPath(t, mod, "/cdgit:c1/cdgit:value")
	c2Value := mustFindPath(t, mod, "/cdgit:c2/cdgit:value")

	if got, ok := c1Item.MinElements(); !ok || got != 1 {
		t.Fatalf("c1 item MinElements() = %d, %v, want 1, true", got, ok)
	}
	if got, ok := c1Item.MaxElements(); !ok || got != 5 {
		t.Fatalf("c1 item MaxElements() = %d, %v, want 5, true", got, ok)
	}
	if got, ok := c2Item.MinElements(); ok {
		t.Fatalf("c2 item MinElements() = %d, true, want unset", got)
	}
	if got, ok := c2Item.MaxElements(); ok {
		t.Fatalf("c2 item MaxElements() = %d, true, want unset", got)
	}
	if got := c1Value.DefaultValues(); len(got) != 0 {
		t.Fatalf("c1 value DefaultValues() = %v, want empty", got)
	}
	if got, want := c2Value.DefaultValues(), []string{"only-c2"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("c2 value DefaultValues() = %v, want %v", got, want)
	}
	if got, want := deviationProperties(c1Item.DeviationProvenance()), []string{"min-elements", "max-elements"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("c1 item deviation properties = %v, want %v", got, want)
	}
	if got := len(c2Item.DeviationProvenance()); got != 0 {
		t.Fatalf("c2 item DeviationProvenance() len = %d, want 0", got)
	}
	if got := len(c1Value.DeviationProvenance()); got != 0 {
		t.Fatalf("c1 value DeviationProvenance() len = %d, want 0", got)
	}
	if got, want := deviationProperties(c2Value.DeviationProvenance()), []string{"default"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("c2 value deviation properties = %v, want %v", got, want)
	}
}

func mustFindPath(t *testing.T, mod cambium.Module, path string) cambium.SchemaNodeRef {
	t.Helper()
	node, err := mod.FindPath(path)
	if err != nil {
		t.Fatalf("FindPath %s: %v", path, err)
	}
	return node
}

func deviationProperties(devs []cambium.Deviation) []string {
	out := make([]string, len(devs))
	for i, dev := range devs {
		out[i] = dev.Property()
	}
	return out
}

func assertPattern(t *testing.T, leaf cambium.SchemaNodeRef, wantRegex string) {
	t.Helper()
	info, ok := leaf.LeafType()
	if !ok {
		t.Fatal("expected leaf type")
	}
	rs, ok := info.Resolved().(cambium.ResolvedString)
	if !ok {
		t.Fatalf("expected resolved string, got %T", info.Resolved())
	}
	for _, p := range rs.Patterns {
		if p.Regex() == wantRegex {
			return
		}
	}
	var got []string
	for _, p := range rs.Patterns {
		got = append(got, p.Regex())
	}
	t.Fatalf("patterns = %v, want %q", got, wantRegex)
}

func leafNames(refs []cambium.SchemaNodeRef) []string {
	out := make([]string, len(refs))
	for i, r := range refs {
		out[i] = r.Name()
	}
	return out
}
