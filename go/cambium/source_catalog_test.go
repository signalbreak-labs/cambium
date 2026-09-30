// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

package cambium_test

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/signalbreak-labs/cambium/go/cambium"
)

func catalogSource(t *testing.T, dir, filename, source string) string {
	t.Helper()
	path := filepath.Join(dir, filename)
	if err := os.WriteFile(path, []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func catalogBuilder(t *testing.T) *cambium.ContextBuilder {
	t.Helper()
	b, err := cambium.NewContextBuilder(cambium.ContextFlags{DisableSearchdirCwd: true})
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestSourceCatalogResolvesDeclaredNamesBeforeExplicitRootLoads(t *testing.T) {
	dir := t.TempDir()
	consumer := catalogSource(t, dir, "00-consumer.yang", `module consumer {
  yang-version 1.1; namespace "urn:consumer"; prefix c;
  import library { prefix l; }
  leaf value { type l:text; }
}`)
	library := catalogSource(t, dir, "99-schema.yang", `module library {
  yang-version 1.1; namespace "urn:library"; prefix l;
  typedef text { type string; }
}`)
	unused := catalogSource(t, dir, "unused-alias.yang", `module unused {
  yang-version 1.1; namespace "urn:unused"; prefix u;
  leaf omitted { if-feature undeclared; type string; }
}`)
	b := catalogBuilder(t)
	if err := b.RegisterSourcePaths(consumer, library, unused); err != nil {
		t.Fatal(err)
	}
	if err := b.LoadModuleFromPath(consumer); err != nil {
		t.Fatalf("consumer could not discover declared library name: %v", err)
	}
	ctx, err := b.Build()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(ctx.Close)
	report := ctx.LoadReport()
	if len(report.RequestedModules) != 1 || report.RequestedModules[0].Name != "consumer" || len(report.TransitiveImports) != 1 || report.TransitiveImports[0].Name != "library" || report.TransitiveImports[0].Implemented {
		t.Fatalf("registration changed requested/implemented modules: %+v", report)
	}
	if _, ok := ctx.GetModule("unused", nil); ok {
		t.Fatal("registering a source loaded or compiled an unused module")
	}
}

func TestSourceCatalogPinsRevisionsAndSubmodulesWithSearchPathFallback(t *testing.T) {
	dir := t.TempDir()
	old := catalogSource(t, dir, "old-alias.yang", `module library {
  yang-version 1.1; namespace "urn:library"; prefix l;
  revision 2024-01-01; typedef text { type string; }
}`)
	latest := catalogSource(t, dir, "latest-alias.yang", `module library {
  yang-version 1.1; namespace "urn:library"; prefix l;
  revision 2025-01-01; typedef text { type uint8; }
}`)
	part := catalogSource(t, dir, "part-alias.yang", `submodule part {
  yang-version 1.1; belongs-to consumer { prefix c; }
  revision 2024-01-01; leaf included { type string; }
}`)
	newPart := catalogSource(t, dir, "part-new-alias.yang", `submodule part {
  yang-version 1.1; belongs-to consumer { prefix c; }
  revision 2025-01-01; leaf wrong-revision { type string; }
}`)
	consumer := catalogSource(t, dir, "00-consumer.yang", `module consumer {
  yang-version 1.1; namespace "urn:consumer"; prefix c;
  import library { prefix l; revision-date 2024-01-01; }
  import fallback { prefix f; }
  include part { revision-date 2024-01-01; }
  leaf value { type l:text; }
  leaf other { type f:text; }
}`)
	catalogSource(t, dir, "fallback.yang", `module fallback { namespace "urn:fallback"; prefix f; typedef text { type string; } }`)
	b := catalogBuilder(t)
	if err := b.RegisterSourcePaths(latest, newPart, consumer, old, part); err != nil {
		t.Fatal(err)
	}
	if err := b.LoadModuleFromPath(consumer); err != nil {
		t.Fatal(err)
	}
	if err := b.LoadModule("library", nil, nil); err != nil {
		t.Fatal(err)
	}
	ctx, err := b.Build()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(ctx.Close)
	mod, _ := ctx.Schema("consumer")
	if got := childNamesForProfile(mod.Children()); !slices.Equal(got, []string{"included", "value", "other"}) {
		t.Errorf("selected submodule children = %v", got)
	}
	typ, _ := schemaNodeAt(t, mod, "/c:value").LeafType()
	if typ.Base() != cambium.BaseTypeString {
		t.Error("revision-date import selected newer registered source")
	}
	lib, _ := ctx.Schema("library")
	if revision, _ := lib.Revision(); revision != "2025-01-01" {
		t.Errorf("unpinned catalog selection = %q", revision)
	}
}

func TestSourceCatalogRegistrationIsAtomicAndFrozen(t *testing.T) {
	dir := t.TempDir()
	good := catalogSource(t, dir, "alias.yang", `module library { namespace "urn:library"; prefix l; }`)
	bad := catalogSource(t, dir, "invalid.yang", `module invalid {`)
	b := catalogBuilder(t)
	if err := b.RegisterSourcePaths(good, bad); err == nil {
		t.Fatal("registration accepted malformed source")
	}
	if err := b.LoadModule("library", nil, nil); err == nil {
		t.Fatal("failed registration leaked an earlier catalog entry")
	}
	if err := b.RegisterSourcePaths(good, good); err != nil {
		t.Fatalf("same-path registration should be idempotent: %v", err)
	}
	duplicate := catalogSource(t, dir, "conflict.yang", `module library { namespace "urn:different"; prefix l; }`)
	if err := b.RegisterSourcePaths(duplicate); err == nil {
		t.Fatal("registration accepted conflicting declared identity")
	}
	if err := b.LoadModule("library", nil, nil); err != nil {
		t.Fatal(err)
	}
	copyOfBuilder := *b
	ctx, err := b.Build()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(ctx.Close)
	mod, _ := ctx.Schema("library")
	if mod.Namespace() != "urn:library" {
		t.Fatal("failed registration replaced existing catalog entry")
	}
	if err := copyOfBuilder.RegisterSourcePaths(good); err == nil {
		t.Fatal("copied builder registered source after Build")
	}
}

func TestSourceCatalogUnpinnedImportsSelectNewestAfterPinnedImport(t *testing.T) {
	dir := t.TempDir()
	old := catalogSource(t, dir, "old.yang", `module library { yang-version 1.1; namespace "urn:library"; prefix l; revision 2024-01-01; typedef text { type string; } }`)
	latest := catalogSource(t, dir, "latest.yang", `module library { yang-version 1.1; namespace "urn:library"; prefix l; revision 2025-01-01; typedef text { type uint8; } }`)
	pinned := catalogSource(t, dir, "pinned-alias.yang", `module pinned { yang-version 1.1; namespace "urn:pinned"; prefix p; import library { prefix l; revision-date 2024-01-01; } leaf value { type l:text; } }`)
	floating := catalogSource(t, dir, "floating-alias.yang", `module floating { yang-version 1.1; namespace "urn:floating"; prefix f; import library { prefix l; } leaf value { type l:text; } }`)
	b := catalogBuilder(t)
	if err := b.RegisterSourcePaths(old, latest, pinned, floating); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"pinned", "floating"} {
		if err := b.LoadModule(name, nil, nil); err != nil {
			t.Fatal(err)
		}
	}
	ctx, err := b.Build()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(ctx.Close)
	for _, tc := range []struct {
		name, path, revision string
		base                 cambium.BaseType
	}{
		{"pinned", "/p:value", "2024-01-01", cambium.BaseTypeString},
		{"floating", "/f:value", "2025-01-01", cambium.BaseTypeUint8},
	} {
		mod, _ := ctx.Schema(tc.name)
		lib, _ := mod.ResolvePrefix("l")
		revision, _ := lib.Revision()
		typ, _ := schemaNodeAt(t, mod, tc.path).LeafType()
		if revision != tc.revision || typ.Base() != tc.base {
			t.Errorf("%s uses revision %q / %s, want %q / %s", tc.name, revision, typ.Base(), tc.revision, tc.base)
		}
	}
}

func TestCopiedBuilderCannotMutateOrBuildFrozenContext(t *testing.T) {
	b := catalogBuilder(t)
	copyOfBuilder := *b
	ctx, err := b.Build()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(ctx.Close)
	if err := copyOfBuilder.SetValidationMode(cambium.ValidationVendorCompatible); err == nil {
		t.Error("copied builder changed frozen validation mode")
	}
	if _, err := copyOfBuilder.Build(); err == nil {
		t.Error("copied builder built already-frozen context")
	}
}
