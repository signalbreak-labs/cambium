// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

package cambium_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/signalbreak-labs/cambium/go/cambium"
)

func TestVendorYANGEnumArgumentsAreYangStrings(t *testing.T) {
	source := `module enum-string-values {
    namespace "urn:test:enum-string-values";
    prefix esv;

    leaf cipher {
        type enumeration {
            enum "3des";
            enum "60/60";
            enum "Admin Down";
            enum "?";
        }
    }
}`
	builder, err := cambium.NewContextBuilder(cambium.ContextFlags{})
	if err != nil {
		t.Fatal(err)
	}
	if err := builder.LoadModuleStr(source); err != nil {
		t.Fatalf("LoadModuleStr: %v", err)
	}
	ctx, err := builder.Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	t.Cleanup(func() { ctx.Close() })

	mod, err := ctx.Schema("enum-string-values")
	if err != nil {
		t.Fatalf("Schema: %v", err)
	}
	resolved := resolvedTypeFor[cambium.ResolvedEnumeration](t, mod, "/esv:cipher")
	var got []string
	for _, value := range resolved.Values() {
		got = append(got, value.Name())
	}
	want := []string{"3des", "60/60", "Admin Down", "?"}
	if strings.Join(got, "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("enum values = %v, want %v", got, want)
	}
}

func TestVendorYANGBitArgumentsRemainYangIdentifiers(t *testing.T) {
	source := `module bit-identifier-values {
    namespace "urn:test:bit-identifier-values";
    prefix biv;

    leaf flags {
        type bits {
            bit "not an identifier";
        }
    }
}`
	builder, err := cambium.NewContextBuilder(cambium.ContextFlags{})
	if err != nil {
		t.Fatal(err)
	}
	if err := builder.LoadModuleStr(source); err == nil {
		if _, err := builder.Build(); err == nil {
			t.Fatal("Build accepted bit argument that is not a YANG identifier")
		} else if !strings.Contains(err.Error(), "invalid identifier") {
			t.Fatalf("Build error = %v, want invalid identifier", err)
		}
	} else if !strings.Contains(err.Error(), "invalid identifier") {
		t.Fatalf("LoadModuleStr error = %v, want invalid identifier", err)
	}
}

func TestVendorYANGXSDPatternLargeRepeatPreserved(t *testing.T) {
	source := `module large-repeat-pattern {
    namespace "urn:test:large-repeat-pattern";
    prefix lrp;

    leaf token {
        type string {
            pattern "[a-fA-F0-9]{4096}";
        }
    }
}`
	builder, err := cambium.NewContextBuilder(cambium.ContextFlags{})
	if err != nil {
		t.Fatal(err)
	}
	if err := builder.LoadModuleStr(source); err != nil {
		t.Fatalf("LoadModuleStr: %v", err)
	}
	ctx, err := builder.Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	t.Cleanup(func() { ctx.Close() })

	mod, err := ctx.Schema("large-repeat-pattern")
	if err != nil {
		t.Fatalf("Schema: %v", err)
	}
	resolved := resolvedTypeFor[cambium.ResolvedString](t, mod, "/lrp:token")
	if got := len(resolved.Patterns); got != 1 {
		t.Fatalf("patterns = %d, want 1", got)
	}
	if got, want := resolved.Patterns[0].Regex(), "[a-fA-F0-9]{4096}"; got != want {
		t.Fatalf("pattern = %q, want %q", got, want)
	}
}

func TestVendorYANGStrictRejectsOutOfOrderRevisions(t *testing.T) {
	source := `module out-of-order-revisions {
    namespace "urn:test:out-of-order-revisions";
    prefix oor;

    revision 2023-01-01;
    revision 2025-01-01;

    container top;
}`
	builder, err := cambium.NewContextBuilder(cambium.ContextFlags{})
	if err != nil {
		t.Fatal(err)
	}
	if err := builder.LoadModuleStr(source); err == nil {
		t.Fatal("LoadModuleStr accepted out-of-order revisions in strict mode")
	} else if !strings.Contains(err.Error(), "out of order") {
		t.Fatalf("LoadModuleStr error = %v, want out-of-order error", err)
	}
}

func TestVendorYANGVendorCompatibleLoadsOutOfOrderRevisionsWithWarning(t *testing.T) {
	source := `module out-of-order-revisions {
    namespace "urn:test:out-of-order-revisions";
    prefix oor;

    revision 2023-01-01;
    revision 2025-01-01;

    container top;
}`
	builder, err := cambium.NewContextBuilder(cambium.ContextFlags{})
	if err != nil {
		t.Fatal(err)
	}
	if err := builder.SetValidationMode(cambium.ValidationVendorCompatible); err != nil {
		t.Fatalf("SetValidationMode: %v", err)
	}
	if err := builder.LoadModuleStr(source); err != nil {
		t.Fatalf("LoadModuleStr: %v", err)
	}
	ctx, err := builder.Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	t.Cleanup(func() { ctx.Close() })

	mod, err := ctx.Schema("out-of-order-revisions")
	if err != nil {
		t.Fatalf("Schema: %v", err)
	}
	if got, ok := mod.Revision(); !ok || got != "2025-01-01" {
		t.Fatalf("Revision = (%q,%v), want 2025-01-01,true", got, ok)
	}
	if !diagnosticContains(ctx.LoadReport().Warnings, "out-of-order-revisions", "out of order") {
		t.Fatalf("warnings = %#v, want out-of-order revision warning", ctx.LoadReport().Warnings)
	}
}

func TestVendorYANGStrictRejectsDuplicateEquivalentImports(t *testing.T) {
	base := `module duplicate-import-base {
    namespace "urn:duplicate-import-base";
    prefix dib;

    typedef identifier {
        type string;
    }
}`
	user := `module duplicate-import-user {
    namespace "urn:duplicate-import-user";
    prefix diu;

    import duplicate-import-base {
        prefix dib;
    }
    import duplicate-import-base {
        prefix dib;
    }

    container root {
        leaf id {
            type dib:identifier;
        }
    }
}`
	builder, err := cambium.NewContextBuilder(cambium.ContextFlags{})
	if err != nil {
		t.Fatal(err)
	}
	if err := builder.LoadModuleStr(base); err != nil {
		t.Fatalf("LoadModuleStr(base): %v", err)
	}
	err = builder.LoadModuleStr(user)
	if err == nil {
		t.Fatal("LoadModuleStr accepted duplicate equivalent imports in strict mode")
	}
	if !strings.Contains(err.Error(), `duplicate import prefix "dib"`) {
		t.Fatalf("LoadModuleStr error = %v, want duplicate import prefix", err)
	}
}

func TestVendorYANGVendorCompatibleAllowsDuplicateEquivalentImports(t *testing.T) {
	base := `module duplicate-import-base {
    namespace "urn:duplicate-import-base";
    prefix dib;

    typedef identifier {
        type string;
    }
}`
	user := `module duplicate-import-user {
    namespace "urn:duplicate-import-user";
    prefix diu;

    import duplicate-import-base {
        prefix dib;
    }
    import duplicate-import-base {
        prefix dib;
    }

    container root {
        leaf id {
            type dib:identifier;
        }
    }
}`
	ctx := buildVendorYANGModules(t, cambium.ValidationVendorCompatible, base, user)
	mod, err := ctx.Schema("duplicate-import-user")
	if err != nil {
		t.Fatalf("Schema: %v", err)
	}
	imports := mod.Imports()
	if got, want := len(imports), 1; got != want {
		t.Fatalf("Imports length = %d, want %d: %#v", got, want, imports)
	}
	if imports[0].Name != "duplicate-import-base" || imports[0].Prefix != "dib" || imports[0].Revision != "" {
		t.Fatalf("Imports[0] = %#v, want duplicate-import-base/dib without revision", imports[0])
	}
	leaf, err := mod.FindPath("/diu:root/diu:id")
	if err != nil {
		t.Fatalf("FindPath: %v", err)
	}
	typ, ok := leaf.LeafType()
	if !ok {
		t.Fatal("LeafType missing")
	}
	if got, want := typ.Base(), cambium.BaseTypeString; got != want {
		t.Fatalf("LeafType().Base() = %s, want %s", got, want)
	}
	if !diagnosticContains(ctx.LoadReport().Warnings, "duplicate-import-user", "duplicate import") {
		t.Fatalf("warnings = %#v, want duplicate import warning", ctx.LoadReport().Warnings)
	}
}

func TestVendorYANGVendorCompatibleAllowsDuplicateEquivalentRevisionedImports(t *testing.T) {
	base := `module duplicate-import-revisioned-equivalent-base {
    namespace "urn:duplicate-import-revisioned-equivalent-base";
    prefix direb;

    revision 2024-01-01;

    typedef identifier {
        type string;
    }
}`
	user := `module duplicate-import-revisioned-equivalent-user {
    namespace "urn:duplicate-import-revisioned-equivalent-user";
    prefix direu;

    import duplicate-import-revisioned-equivalent-base {
        prefix direb;
        revision-date 2024-01-01;
    }
    import duplicate-import-revisioned-equivalent-base {
        prefix direb;
        revision-date 2024-01-01;
    }

    leaf id {
        type direb:identifier;
    }
}`
	ctx := buildVendorYANGModules(t, cambium.ValidationVendorCompatible, base, user)
	mod, err := ctx.Schema("duplicate-import-revisioned-equivalent-user")
	if err != nil {
		t.Fatalf("Schema: %v", err)
	}
	imports := mod.Imports()
	if got, want := len(imports), 1; got != want {
		t.Fatalf("Imports length = %d, want %d: %#v", got, want, imports)
	}
	if imports[0].Name != "duplicate-import-revisioned-equivalent-base" || imports[0].Prefix != "direb" || imports[0].Revision != "2024-01-01" {
		t.Fatalf("Imports[0] = %#v, want duplicate-import-revisioned-equivalent-base/direb@2024-01-01", imports[0])
	}
	if _, err := mod.FindPath("/direu:id"); err != nil {
		t.Fatalf("FindPath: %v", err)
	}
	if !diagnosticContains(ctx.LoadReport().Warnings, "duplicate-import-revisioned-equivalent-user", "duplicate import") {
		t.Fatalf("warnings = %#v, want duplicate import warning", ctx.LoadReport().Warnings)
	}
}

func TestVendorYANGVendorCompatibleRejectsDuplicateImportPrefixConflicts(t *testing.T) {
	base2024 := `module duplicate-import-revisioned-base {
    namespace "urn:duplicate-import-revisioned-base";
    prefix dirb;

    revision 2024-01-01;

    typedef identifier {
        type string;
    }
}`
	base2025 := `module duplicate-import-revisioned-base {
    namespace "urn:duplicate-import-revisioned-base";
    prefix dirb;

    revision 2025-01-01;

    typedef identifier {
        type string;
    }
}`
	user := `module duplicate-import-conflict-user {
    namespace "urn:duplicate-import-conflict-user";
    prefix dicu;

    import duplicate-import-revisioned-base {
        prefix dirb;
        revision-date 2024-01-01;
    }
    import duplicate-import-revisioned-base {
        prefix dirb;
        revision-date 2025-01-01;
    }

    container root;
}`
	builder, err := cambium.NewContextBuilder(cambium.ContextFlags{})
	if err != nil {
		t.Fatal(err)
	}
	if err := builder.SetValidationMode(cambium.ValidationVendorCompatible); err != nil {
		t.Fatalf("SetValidationMode: %v", err)
	}
	for _, source := range []string{base2024, base2025} {
		if err := builder.LoadModuleStr(source); err != nil {
			t.Fatalf("LoadModuleStr(base): %v", err)
		}
	}
	err = builder.LoadModuleStr(user)
	if err == nil {
		t.Fatal("LoadModuleStr accepted duplicate import prefix with conflicting revisions")
	}
	if !strings.Contains(err.Error(), `duplicate import prefix "dirb"`) {
		t.Fatalf("LoadModuleStr error = %v, want duplicate import prefix", err)
	}

	other := `module duplicate-import-other-base {
    namespace "urn:duplicate-import-other-base";
    prefix diob;
}`
	differentModuleUser := `module duplicate-import-different-user {
    namespace "urn:duplicate-import-different-user";
    prefix didu;

    import duplicate-import-revisioned-base {
        prefix shared;
        revision-date 2024-01-01;
    }
    import duplicate-import-other-base {
        prefix shared;
    }

    container root;
}`
	builder, err = cambium.NewContextBuilder(cambium.ContextFlags{})
	if err != nil {
		t.Fatal(err)
	}
	if err := builder.SetValidationMode(cambium.ValidationVendorCompatible); err != nil {
		t.Fatalf("SetValidationMode: %v", err)
	}
	for _, source := range []string{base2024, other} {
		if err := builder.LoadModuleStr(source); err != nil {
			t.Fatalf("LoadModuleStr(target): %v", err)
		}
	}
	err = builder.LoadModuleStr(differentModuleUser)
	if err == nil {
		t.Fatal("LoadModuleStr accepted duplicate import prefix for different modules")
	}
	if !strings.Contains(err.Error(), `duplicate import prefix "shared"`) {
		t.Fatalf("LoadModuleStr error = %v, want duplicate import prefix", err)
	}
}

func TestVendorYANGStrictRejectsTopLevelExtensionBeforeRevision(t *testing.T) {
	metadata := `module metadata-extension {
    namespace "urn:test:metadata-extension";
    prefix me;

    extension version {
        argument value;
    }
}`
	source := `module extension-before-revision {
    namespace "urn:test:extension-before-revision";
    prefix ebr;

    import metadata-extension {
        prefix me;
    }

    me:version "1.0.0";

    revision 2025-01-01;
    revision 2023-01-01;

    container top;
}`
	builder, err := cambium.NewContextBuilder(cambium.ContextFlags{})
	if err != nil {
		t.Fatal(err)
	}
	if err := builder.LoadModuleStr(metadata); err != nil {
		t.Fatalf("LoadModuleStr metadata: %v", err)
	}
	if err := builder.LoadModuleStr(source); err == nil {
		t.Fatal("LoadModuleStr accepted top-level extension before revision in strict mode")
	} else if !strings.Contains(err.Error(), `revision "2025-01-01" is out of order`) {
		t.Fatalf("LoadModuleStr error = %v, want revision placement error", err)
	}
}

func TestVendorYANGVendorCompatibleAllowsTopLevelExtensionBeforeRevision(t *testing.T) {
	metadata := `module metadata-extension {
    namespace "urn:test:metadata-extension";
    prefix me;

    extension version {
        argument value;
    }
}`
	source := `module extension-before-revision {
    namespace "urn:test:extension-before-revision";
    prefix ebr;

    import metadata-extension {
        prefix me;
    }

    me:version "1.0.0";

    revision 2025-01-01;
    revision 2023-01-01;

    container top;
}`
	ctx := buildVendorYANGModules(t, cambium.ValidationVendorCompatible, metadata, source)
	mod, err := ctx.Schema("extension-before-revision")
	if err != nil {
		t.Fatalf("Schema: %v", err)
	}
	if got, ok := mod.Revision(); !ok || got != "2025-01-01" {
		t.Fatalf("Revision = (%q,%v), want 2025-01-01,true", got, ok)
	}
}

func TestVendorYANGVendorCompatibleExplicitTransitiveLoadIsIdempotent(t *testing.T) {
	dir := t.TempDir()
	shared := `module shared-types {
    namespace "urn:test:shared-types";
    prefix st;

    typedef name {
        type string;
    }
}`
	user := `module imports-shared {
    namespace "urn:test:imports-shared";
    prefix is;

    import shared-types {
        prefix st;
    }

    leaf name {
        type st:name;
    }
}`
	sharedPath := filepath.Join(dir, "shared-types.yang")
	writeModuleFile(t, sharedPath, []byte(shared))
	writeModuleFile(t, filepath.Join(dir, "imports-shared.yang"), []byte(user))

	builder, err := cambium.NewContextBuilder(cambium.ContextFlags{})
	if err != nil {
		t.Fatal(err)
	}
	if err := builder.SetValidationMode(cambium.ValidationVendorCompatible); err != nil {
		t.Fatalf("SetValidationMode: %v", err)
	}
	if err := builder.SearchPath(dir); err != nil {
		t.Fatalf("SearchPath: %v", err)
	}
	if err := builder.LoadModule("imports-shared", nil, nil); err != nil {
		t.Fatalf("LoadModule imports-shared: %v", err)
	}
	if err := builder.LoadModuleFromPath(sharedPath); err != nil {
		t.Fatalf("LoadModuleFromPath shared-types: %v", err)
	}
	ctx, err := builder.Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	t.Cleanup(func() { ctx.Close() })

	report := ctx.LoadReport()
	if len(report.RequestedModules) != 2 {
		t.Fatalf("requested modules = %d, want 2 (%#v)", len(report.RequestedModules), report.RequestedModules)
	}
}

func TestVendorYANGEquivalentBareSymlinkAndRevisionedLoadIsIdempotent(t *testing.T) {
	dir := t.TempDir()
	shared := `module shared-types {
    namespace "urn:test:shared-types";
    prefix st;

    revision 2024-01-01;

    typedef name {
        type string;
    }
}`
	user := `module imports-shared {
    namespace "urn:test:imports-shared";
    prefix is;

    import shared-types {
        prefix st;
    }

    revision 2024-01-02;

    leaf name {
        type st:name;
    }
}`
	sharedRevisionedPath := filepath.Join(dir, "shared-types@2024-01-01.yang")
	writeModuleFile(t, sharedRevisionedPath, []byte(shared))
	writeModuleFile(t, filepath.Join(dir, "imports-shared@2024-01-02.yang"), []byte(user))
	if err := os.Symlink("shared-types@2024-01-01.yang", filepath.Join(dir, "shared-types.yang")); err != nil {
		t.Fatalf("Symlink: %v", err)
	}

	builder, err := cambium.NewContextBuilder(cambium.ContextFlags{AllImplemented: true})
	if err != nil {
		t.Fatal(err)
	}
	if err := builder.SetValidationMode(cambium.ValidationVendorCompatible); err != nil {
		t.Fatalf("SetValidationMode: %v", err)
	}
	if err := builder.SearchPath(dir); err != nil {
		t.Fatalf("SearchPath: %v", err)
	}
	if err := builder.LoadModuleFromPath(filepath.Join(dir, "imports-shared@2024-01-02.yang")); err != nil {
		t.Fatalf("LoadModuleFromPath imports-shared: %v", err)
	}
	if err := builder.LoadModuleFromPath(sharedRevisionedPath); err != nil {
		t.Fatalf("LoadModuleFromPath shared-types: %v", err)
	}
	ctx, err := builder.Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	t.Cleanup(func() { ctx.Close() })

	if !moduleRequested(ctx.LoadReport().RequestedModules, "shared-types") {
		t.Fatalf("requested modules = %#v, want shared-types requested", ctx.LoadReport().RequestedModules)
	}
}

func TestVendorYANGConflictingSameRevisionSourcesStillFail(t *testing.T) {
	dir := t.TempDir()
	first := `module conflicting-source {
    namespace "urn:test:conflicting-source";
    prefix cs;

    revision 2024-01-01;

    leaf first {
        type string;
    }
}`
	second := `module conflicting-source {
    namespace "urn:test:conflicting-source";
    prefix cs;

    revision 2024-01-01;

    leaf second {
        type string;
    }
}`
	firstPath := filepath.Join(dir, "conflicting-source.yang")
	secondPath := filepath.Join(dir, "conflicting-source@2024-01-01.yang")
	writeModuleFile(t, firstPath, []byte(first))
	writeModuleFile(t, secondPath, []byte(second))

	builder, err := cambium.NewContextBuilder(cambium.ContextFlags{})
	if err != nil {
		t.Fatal(err)
	}
	if err := builder.SetValidationMode(cambium.ValidationVendorCompatible); err != nil {
		t.Fatalf("SetValidationMode: %v", err)
	}
	if err := builder.LoadModuleFromPath(firstPath); err != nil {
		t.Fatalf("LoadModuleFromPath first: %v", err)
	}
	err = builder.LoadModuleFromPath(secondPath)
	if err == nil {
		t.Fatal("LoadModuleFromPath accepted conflicting same module revision source")
	}
	if !strings.Contains(err.Error(), `module "conflicting-source" revision "2024-01-01" already loaded`) {
		t.Fatalf("LoadModuleFromPath error = %v, want duplicate module identity", err)
	}
}

func TestVendorYANGVendorCompatibleAllowsMandatoryConfigAugmentWithWarning(t *testing.T) {
	base := `module augment-base {
    namespace "urn:test:augment-base";
    prefix b;

    container top;
}`
	augment := `module augment-vendor {
    namespace "urn:test:augment-vendor";
    prefix v;

    import augment-base {
        prefix b;
    }

    augment "/b:top" {
        container required {
            leaf name {
                mandatory true;
                type string;
            }
        }
    }
}`
	ctx := buildVendorYANGModules(t, cambium.ValidationVendorCompatible, base, augment)
	mod, err := ctx.Schema("augment-base")
	if err != nil {
		t.Fatalf("Schema: %v", err)
	}
	if _, err := mod.FindPath("/b:top/v:required/v:name"); err != nil {
		t.Fatalf("FindPath augmented mandatory leaf: %v", err)
	}
	if !diagnosticContains(ctx.LoadReport().Warnings, "augment-vendor", "mandatory config") {
		t.Fatalf("warnings = %#v, want mandatory augment warning", ctx.LoadReport().Warnings)
	}
}

func TestVendorYANGStrictRejectsDirectSubmoduleLoad(t *testing.T) {
	dir, childPath := writeParentChildSubmoduleFixture(t)
	builder, err := cambium.NewContextBuilder(cambium.ContextFlags{})
	if err != nil {
		t.Fatal(err)
	}
	if err := builder.SearchPath(dir); err != nil {
		t.Fatalf("SearchPath: %v", err)
	}
	if err := builder.LoadModuleFromPath(childPath); err == nil {
		t.Fatal("LoadModuleFromPath accepted direct submodule in strict mode")
	} else if !strings.Contains(err.Error(), "direct submodule") {
		t.Fatalf("LoadModuleFromPath error = %v, want direct submodule", err)
	}
}

func TestVendorYANGVendorCompatibleDirectSubmoduleLoadResolvesParent(t *testing.T) {
	dir, childPath := writeParentChildSubmoduleFixture(t)
	builder, err := cambium.NewContextBuilder(cambium.ContextFlags{})
	if err != nil {
		t.Fatal(err)
	}
	if err := builder.SetValidationMode(cambium.ValidationVendorCompatible); err != nil {
		t.Fatalf("SetValidationMode: %v", err)
	}
	if err := builder.SearchPath(dir); err != nil {
		t.Fatalf("SearchPath: %v", err)
	}
	if err := builder.LoadModuleFromPath(childPath); err != nil {
		t.Fatalf("LoadModuleFromPath child submodule: %v", err)
	}
	ctx, err := builder.Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	t.Cleanup(func() { ctx.Close() })

	mod, err := ctx.Schema("parent-module")
	if err != nil {
		t.Fatalf("Schema parent-module: %v", err)
	}
	if _, err := mod.FindPath("/p:from-child"); err != nil {
		t.Fatalf("FindPath submodule child: %v", err)
	}
	if !moduleRequested(ctx.LoadReport().RequestedModules, "parent-module") {
		t.Fatalf("requested modules = %#v, want parent-module requested", ctx.LoadReport().RequestedModules)
	}
	if !diagnosticContains(ctx.LoadReport().Warnings, "parent-module", "direct submodule") {
		t.Fatalf("warnings = %#v, want direct submodule warning", ctx.LoadReport().Warnings)
	}
}

func TestVendorYANGIncludedSubmoduleListChildrenAndKeysSurvive(t *testing.T) {
	parent := `module parent-routes {
    namespace "urn:test:parent-routes";
    prefix p;

    include child-static-routes;

    container root {
        uses routes;
    }
}`
	child := `submodule child-static-routes {
    belongs-to parent-routes {
        prefix p;
    }

    grouping routes {
        list interface-next-hop {
            key "ip-address";
            leaf ip-address {
                type string;
            }
            uses route-options;
        }
    }

    grouping route-options {
        leaf multicast {
            type empty;
        }
    }
}`
	dir := t.TempDir()
	writeModuleFile(t, filepath.Join(dir, "parent-routes.yang"), []byte(parent))
	writeModuleFile(t, filepath.Join(dir, "child-static-routes.yang"), []byte(child))

	ctx := buildVendorYANGModulesFromDir(t, cambium.ValidationStrict, dir, "parent-routes")
	mod, err := ctx.Schema("parent-routes")
	if err != nil {
		t.Fatalf("Schema: %v", err)
	}
	list := schemaNodeAt(t, mod, "/p:root/interface-next-hop")
	if got := strings.Join(list.KeyNames(), ","); got != "ip-address" {
		t.Fatalf("key names = %q, want ip-address", got)
	}
	if _, err := list.FindPath("ip-address"); err != nil {
		t.Fatalf("FindPath key leaf: %v", err)
	}
	if _, err := list.FindPath("multicast"); err != nil {
		t.Fatalf("FindPath uses child: %v", err)
	}
}

func TestVendorYANGRefineDuringListBuildDoesNotResolveKeysEarly(t *testing.T) {
	source := `module refine-during-list-build {
    namespace "urn:test:refine-during-list-build";
    prefix rdlb;

    grouping opts {
        leaf multicast {
            type empty;
        }
    }

    grouping routes {
        list next-hop {
            key "ip-address";
            leaf ip-address {
                type string;
            }
            uses opts {
                refine "multicast" {
                    mandatory true;
                }
            }
        }
    }

    container root {
        uses routes;
    }
}`
	ctx := buildVendorYANGModules(t, cambium.ValidationStrict, source)
	mod, err := ctx.Schema("refine-during-list-build")
	if err != nil {
		t.Fatalf("Schema: %v", err)
	}
	list := schemaNodeAt(t, mod, "/rdlb:root/next-hop")
	if got := strings.Join(list.KeyNames(), ","); got != "ip-address" {
		t.Fatalf("key names = %q, want ip-address", got)
	}
	multicast := childByName(t, list.Children(), "multicast")
	if !multicast.IsMandatory() {
		t.Fatal("refined multicast leaf should be mandatory")
	}
}

func TestVendorYANGVendorCompatibleSkipsFeatureDisabledAugmentTarget(t *testing.T) {
	target := `module feature-augment-target {
    namespace "urn:test:feature-augment-target";
    prefix fat;

    feature advanced;

    container root {
        container gated {
            if-feature advanced;
        }
    }
}`
	user := `module feature-augment-user {
    namespace "urn:test:feature-augment-user";
    prefix fau;

    import feature-augment-target {
        prefix fat;
    }

    augment "/fat:root/fat:gated" {
        leaf added {
            type string;
        }
    }
}`
	ctx := buildVendorYANGModules(t, cambium.ValidationVendorCompatible, target, user)
	targetMod, err := ctx.Schema("feature-augment-target")
	if err != nil {
		t.Fatalf("Schema disabled target: %v", err)
	}
	root := schemaNodeAt(t, targetMod, "/fat:root")
	if _, ok := root.Children().Lookup("gated"); ok {
		t.Fatal("feature-disabled target should be absent")
	}
	if !diagnosticContains(ctx.LoadReport().Warnings, "feature-augment-user", "augment") {
		t.Fatalf("warnings = %#v, want skipped augment warning", ctx.LoadReport().Warnings)
	}

	enabledCtx := buildVendorYANGModulesWithFeatures(t, cambium.ValidationVendorCompatible, map[string][]string{
		"feature-augment-target": {"advanced"},
	}, target, user)
	enabledTarget, err := enabledCtx.Schema("feature-augment-target")
	if err != nil {
		t.Fatalf("Schema enabled target: %v", err)
	}
	if _, err := enabledTarget.FindPath("/fat:root/fat:gated/fau:added"); err != nil {
		t.Fatalf("FindPath enabled augment: %v", err)
	}
}

func TestVendorYANGVendorCompatibleConfigFalseMandatoryTypedefDefault(t *testing.T) {
	source := `module inherited-default-state {
    namespace "urn:test:inherited-default-state";
    prefix ids;

    typedef zero-based-counter32 {
        type uint32;
        default "0";
    }

    container state {
        config false;
        leaf denied-operations {
            type zero-based-counter32;
            mandatory true;
        }
    }
}`
	ctx := buildVendorYANGModules(t, cambium.ValidationVendorCompatible, source)
	mod, err := ctx.Schema("inherited-default-state")
	if err != nil {
		t.Fatalf("Schema: %v", err)
	}
	leaf := schemaNodeAt(t, mod, "/ids:state/denied-operations")
	if got, ok := leaf.DefaultValue(); !ok || got != "0" {
		t.Fatalf("DefaultValue = (%q,%v), want 0,true", got, ok)
	}
	if !diagnosticContains(ctx.LoadReport().Warnings, "inherited-default-state", "mandatory leaf") {
		t.Fatalf("warnings = %#v, want mandatory default warning", ctx.LoadReport().Warnings)
	}
}

func TestVendorYANGVendorCompatibleDeviationUsesRootPrefixForAugmentedDescendants(t *testing.T) {
	root := `module deviation-augment-root {
    namespace "urn:test:deviation-augment-root";
    prefix r;

    container configuration;
}`
	child := `module deviation-augment-child {
    namespace "urn:test:deviation-augment-child";
    prefix c;

    import deviation-augment-root {
        prefix r;
    }

    augment "/r:configuration" {
        container chassis {
            leaf craft-lockout {
                type empty;
            }
        }
    }
}`
	vendor := `module deviation-augment-vendor {
    namespace "urn:test:deviation-augment-vendor";
    prefix v;

    import deviation-augment-root {
        prefix r;
    }
    import deviation-augment-child {
        prefix c;
    }

    deviation "/r:configuration/r:chassis/r:craft-lockout" {
        deviate not-supported;
    }
}`
	ctx := buildVendorYANGModules(t, cambium.ValidationVendorCompatible, root, child, vendor)
	rootMod, err := ctx.Schema("deviation-augment-root")
	if err != nil {
		t.Fatalf("Schema: %v", err)
	}
	if _, err := rootMod.FindPath("/r:configuration/c:chassis/c:craft-lockout"); err == nil {
		t.Fatal("craft-lockout should be removed by not-supported deviation")
	}
	if !diagnosticContains(ctx.LoadReport().Warnings, "deviation-augment-vendor", "local-name") {
		t.Fatalf("warnings = %#v, want local-name fallback warning", ctx.LoadReport().Warnings)
	}
}

func TestVendorYANGVendorCompatibleLeafrefUnprefixedAbsoluteFallback(t *testing.T) {
	target := `module leafref-local-target {
    namespace "urn:test:leafref-local-target";
    prefix tgt;

    container network-instances {
        list network-instance {
            key "name";
            leaf name {
                type string;
            }
        }
    }
}`
	grouping := `module leafref-local-grouping {
    namespace "urn:test:leafref-local-grouping";
    prefix grp;

    grouping overlay-config {
        leaf overlay-endpoint-network-instance {
            type leafref {
                path "/network-instances/network-instance/name";
            }
        }
    }
}`
	user := `module leafref-local-user {
    namespace "urn:test:leafref-local-user";
    prefix user;

    import leafref-local-target {
        prefix tgt;
    }
    import leafref-local-grouping {
        prefix grp;
    }

    container top {
        uses grp:overlay-config;
    }
}`
	ctx := buildVendorYANGModules(t, cambium.ValidationVendorCompatible, target, grouping, user)
	mod, err := ctx.Schema("leafref-local-user")
	if err != nil {
		t.Fatalf("Schema: %v", err)
	}
	lr := resolvedTypeFor[cambium.ResolvedLeafRef](t, mod, "/user:top/overlay-endpoint-network-instance")
	targetNode, ok := lr.Target()
	if !ok {
		t.Fatal("leafref target unresolved")
	}
	if got, want := targetNode.Path(), "/leafref-local-target/network-instances/network-instance/name"; got != want {
		t.Fatalf("leafref target = %q, want %q", got, want)
	}
	if !diagnosticContains(ctx.LoadReport().Warnings, "leafref-local-grouping", "unprefixed") {
		t.Fatalf("warnings = %#v, want unprefixed leafref fallback warning", ctx.LoadReport().Warnings)
	}
}

func TestVendorYANGTypedefLeafrefUsesTypedefDefiningModule(t *testing.T) {
	base := `module typedef-leafref-base {
    namespace "urn:test:typedef-leafref-base";
    prefix base;

    container network-instances {
        list network-instance {
            key "name";
            leaf name {
                type leafref {
                    path "../config/name";
                }
            }
            container config {
                leaf name {
                    type string;
                }
            }
        }
    }

    typedef network-instance-ref {
        type leafref {
            path "/base:network-instances/base:network-instance/base:config/base:name";
        }
    }
}`
	user := `module typedef-leafref-user {
    namespace "urn:test:typedef-leafref-user";
    prefix user;

    import typedef-leafref-base {
        prefix base;
    }

    leaf lookup-network-instance {
        type base:network-instance-ref;
    }
}`
	ctx := buildVendorYANGModules(t, cambium.ValidationStrict, base, user)
	mod, err := ctx.Schema("typedef-leafref-user")
	if err != nil {
		t.Fatalf("Schema: %v", err)
	}
	lr := resolvedTypeFor[cambium.ResolvedLeafRef](t, mod, "/user:lookup-network-instance")
	target, ok := lr.Target()
	if !ok {
		t.Fatal("typedef leafref target unresolved")
	}
	if got, want := target.Path(), "/typedef-leafref-base/network-instances/network-instance/config/name"; got != want {
		t.Fatalf("typedef leafref target = %q, want %q", got, want)
	}
}

func TestVendorYANGEffectivePatternsReachKeysAndTypedefLeaves(t *testing.T) {
	source := `module effective-pattern-values {
    namespace "urn:test:effective-pattern-values";
    prefix epv;

    typedef admin-state {
        type string {
            pattern "(act)|(pre)";
        }
    }

    typedef ipv4-address {
        type string {
            pattern "([0-9]|[1-9][0-9]|1[0-9]{2}|2[0-4][0-9]|25[0-5])(\\.([0-9]|[1-9][0-9]|1[0-9]{2}|2[0-4][0-9]|25[0-5])){3}";
        }
    }

    container interfaces {
        list interface {
            key "active name";

            leaf active {
                type admin-state;
            }

            leaf name {
                type string;
            }

            leaf router-id {
                type ipv4-address;
            }
        }
    }
}`
	ctx := buildVendorYANGModules(t, cambium.ValidationStrict, source)
	mod, err := ctx.Schema("effective-pattern-values")
	if err != nil {
		t.Fatalf("Schema: %v", err)
	}
	active := resolvedTypeFor[cambium.ResolvedString](t, mod, "/epv:interfaces/interface/active")
	if got, want := patternStrings(active.Patterns), []string{"(act)|(pre)"}; strings.Join(got, "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("active patterns = %v, want %v", got, want)
	}
	routerID := resolvedTypeFor[cambium.ResolvedString](t, mod, "/epv:interfaces/interface/router-id")
	if got := patternStrings(routerID.Patterns); len(got) != 1 || !strings.Contains(got[0], `25[0-5]`) {
		t.Fatalf("router-id patterns = %v, want inherited IPv4 pattern", got)
	}
	list := schemaNodeAt(t, mod, "/epv:interfaces/interface")
	key := childByName(t, list.ListKeys(), "active")
	keyType, ok := key.LeafType()
	if !ok {
		t.Fatal("active key should have leaf type")
	}
	keyString, ok := keyType.Resolved().(cambium.ResolvedString)
	if !ok {
		t.Fatalf("active key resolved type = %T, want ResolvedString", keyType.Resolved())
	}
	if got := patternStrings(keyString.Patterns); len(got) != 1 || got[0] != "(act)|(pre)" {
		t.Fatalf("active key patterns = %v, want inherited admin pattern", got)
	}
}

func TestVendorYANGXSDPatternContractPreservesRawDollarAndAlternation(t *testing.T) {
	source := `module xsd-pattern-contract {
    namespace "urn:test:xsd-pattern-contract";
    prefix xpc;

    typedef admin-state {
        type string {
            pattern "(act)|(pre)";
        }
    }

    typedef crypt-hash {
        type string {
            pattern "$0$.*|$1$[a-zA-Z0-9./]{1,8}$[a-zA-Z0-9./]{22}";
        }
    }

    leaf state {
        type admin-state;
    }

    leaf password {
        type crypt-hash;
    }
}`
	ctx := buildVendorYANGModules(t, cambium.ValidationStrict, source)
	mod, err := ctx.Schema("xsd-pattern-contract")
	if err != nil {
		t.Fatalf("Schema: %v", err)
	}
	state := resolvedTypeFor[cambium.ResolvedString](t, mod, "/xpc:state")
	if got := patternStrings(state.Patterns); len(got) != 1 || got[0] != "(act)|(pre)" {
		t.Fatalf("state patterns = %v, want raw alternation", got)
	}
	password := resolvedTypeFor[cambium.ResolvedString](t, mod, "/xpc:password")
	want := "$0$.*|$1$[a-zA-Z0-9./]{1,8}$[a-zA-Z0-9./]{22}"
	if got := patternStrings(password.Patterns); len(got) != 1 || got[0] != want {
		t.Fatalf("password patterns = %v, want %q", got, want)
	}
}

func TestVendorYANGBroadPatternListKeyMetadataPreserved(t *testing.T) {
	source := `module broad-pattern-list-key {
    namespace "urn:test:broad-pattern-list-key";
    prefix bplk;

    typedef vendor-string {
        type string {
            pattern "[\\w\\-\\.:,_@#%$\\+=\\| ;]+";
        }
    }

    container interfaces {
        list interface {
            key "owner";

            leaf owner {
                type vendor-string;
            }

            leaf mtu {
                type uint32 {
                    range "64..65535";
                }
                mandatory true;
            }
        }
    }
}`
	ctx := buildVendorYANGModules(t, cambium.ValidationStrict, source)
	mod, err := ctx.Schema("broad-pattern-list-key")
	if err != nil {
		t.Fatalf("Schema: %v", err)
	}
	owner := resolvedTypeFor[cambium.ResolvedString](t, mod, "/bplk:interfaces/interface/owner")
	want := `[\w\-\.:,_@#%$\+=\| ;]+`
	if got := patternStrings(owner.Patterns); len(got) != 1 || got[0] != want {
		t.Fatalf("owner patterns = %v, want %q", got, want)
	}
	list := schemaNodeAt(t, mod, "/bplk:interfaces/interface")
	key := childByName(t, list.ListKeys(), "owner")
	keyType, ok := key.LeafType()
	if !ok {
		t.Fatal("owner key should have leaf type")
	}
	keyString, ok := keyType.Resolved().(cambium.ResolvedString)
	if !ok {
		t.Fatalf("owner key resolved type = %T, want ResolvedString", keyType.Resolved())
	}
	if got := patternStrings(keyString.Patterns); len(got) != 1 || got[0] != want {
		t.Fatalf("owner key patterns = %v, want %q", got, want)
	}
}

func buildVendorYANGModules(t *testing.T, mode cambium.ValidationMode, sources ...string) *cambium.Context {
	t.Helper()
	return buildVendorYANGModulesWithFeatures(t, mode, nil, sources...)
}

func buildVendorYANGModulesWithFeatures(t *testing.T, mode cambium.ValidationMode, features map[string][]string, sources ...string) *cambium.Context {
	t.Helper()
	builder, err := cambium.NewContextBuilder(cambium.ContextFlags{})
	if err != nil {
		t.Fatal(err)
	}
	if err := builder.SetValidationMode(mode); err != nil {
		t.Fatalf("SetValidationMode: %v", err)
	}
	for module, enabled := range features {
		if err := builder.SetFeatures(module, enabled); err != nil {
			t.Fatalf("SetFeatures %s: %v", module, err)
		}
	}
	for _, source := range sources {
		if err := builder.LoadModuleStr(source); err != nil {
			t.Fatalf("LoadModuleStr: %v", err)
		}
	}
	ctx, err := builder.Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	t.Cleanup(func() { ctx.Close() })
	return ctx
}

func buildVendorYANGModulesFromDir(t *testing.T, mode cambium.ValidationMode, dir string, names ...string) *cambium.Context {
	t.Helper()
	builder, err := cambium.NewContextBuilder(cambium.ContextFlags{})
	if err != nil {
		t.Fatal(err)
	}
	if err := builder.SetValidationMode(mode); err != nil {
		t.Fatalf("SetValidationMode: %v", err)
	}
	if err := builder.SearchPath(dir); err != nil {
		t.Fatalf("SearchPath: %v", err)
	}
	for _, name := range names {
		if err := builder.LoadModule(name, nil, nil); err != nil {
			t.Fatalf("LoadModule %s: %v", name, err)
		}
	}
	ctx, err := builder.Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	t.Cleanup(func() { ctx.Close() })
	return ctx
}

func writeParentChildSubmoduleFixture(t *testing.T) (dir, childPath string) {
	t.Helper()
	dir = t.TempDir()
	parent := `module parent-module {
    namespace "urn:test:parent-module";
    prefix p;

    include child-submodule;
}`
	child := `submodule child-submodule {
    belongs-to parent-module {
        prefix p;
    }

    container from-child;
}`
	writeModuleFile(t, filepath.Join(dir, "parent-module.yang"), []byte(parent))
	childPath = filepath.Join(dir, "child-submodule.yang")
	writeModuleFile(t, childPath, []byte(child))
	return dir, childPath
}

func moduleRequested(modules []cambium.ModuleLoadInfo, name string) bool {
	for _, module := range modules {
		if module.Name == name && module.Requested {
			return true
		}
	}
	return false
}

func patternStrings(patterns []cambium.Pattern) []string {
	out := make([]string, 0, len(patterns))
	for _, pattern := range patterns {
		out = append(out, pattern.Regex())
	}
	return out
}

func diagnosticContains(warnings []cambium.Diagnostic, module, fragment string) bool {
	for _, warning := range warnings {
		if warning.Module == module && strings.Contains(warning.Message, fragment) {
			return true
		}
	}
	return false
}
