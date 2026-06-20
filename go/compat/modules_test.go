package compat_test

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/signalbreak-labs/cambium/go/compat"
	upstream "github.com/signalbreak-labs/cambium/go/internal/yangparse/upstream/yang"
)

func TestCompatModuleExportedFieldShape(t *testing.T) {
	moduleType := reflect.TypeOf(compat.Module{})
	var got []string
	for i := 0; i < moduleType.NumField(); i++ {
		field := moduleType.Field(i)
		if field.PkgPath == "" {
			got = append(got, field.Name)
		}
	}
	want := []string{
		"Name",
		"Source",
		"Parent",
		"Extensions",
		"Anydata",
		"Anyxml",
		"Augment",
		"BelongsTo",
		"Choice",
		"Contact",
		"Container",
		"Description",
		"Deviation",
		"Extension",
		"Feature",
		"Grouping",
		"Identity",
		"Import",
		"Include",
		"Leaf",
		"LeafList",
		"List",
		"Namespace",
		"Notification",
		"Organization",
		"Prefix",
		"Reference",
		"Revision",
		"RPC",
		"Typedef",
		"Uses",
		"YangVersion",
		"Modules",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Module exported fields = %v, want %v", got, want)
	}
}

func TestModulesAddPathTracksGoyangEmptySegments(t *testing.T) {
	compatModules := compat.NewModules()
	upstreamModules := upstream.NewModules()
	for _, path := range []string{"", ":alpha", "alpha:", "beta:alpha"} {
		compatModules.AddPath(path)
		upstreamModules.AddPath(path)
	}
	if !reflect.DeepEqual(compatModules.Path, upstreamModules.Path) {
		t.Fatalf("AddPath Path = %v, want goyang %v", compatModules.Path, upstreamModules.Path)
	}
}

func TestModulesFacadeParseProcessGetModule(t *testing.T) {
	source := `module compat-modules-demo {
    namespace "urn:compat-modules-demo";
    prefix cmd;

    container z-top {
        leaf zed { type string; }
    }
    container a-top {
        leaf alpha { type string; }
    }
}
`
	ms := compat.NewModules()
	if ms == nil {
		t.Fatal("NewModules returned nil")
	}
	ms.ParseOptions.DeviateOptions = compat.DeviateOptions{IgnoreDeviateNotSupported: true}
	if err := ms.Parse(source, "compat-modules-demo.yang"); err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if got := ms.Modules["compat-modules-demo"]; got == nil || got.Name != "compat-modules-demo" {
		t.Fatalf("Modules entry = %#v, want compat-modules-demo", got)
	}
	if errs := ms.Process(); len(errs) != 0 {
		t.Fatalf("Process errors = %v, want none", errs)
	}
	entry, errs := ms.GetModule("compat-modules-demo")
	if len(errs) != 0 {
		t.Fatalf("GetModule errors = %v, want none", errs)
	}
	if entry == nil {
		t.Fatal("GetModule returned nil entry")
	}
	if got, want := childNames(entry.Children()), []string{"z-top", "a-top"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("entry children = %v, want %v", got, want)
	}
	if got := entry.Modules(); got != ms {
		t.Fatalf("entry Modules() = %p, want %p", got, ms)
	}
	var moduleNode compat.Node = ms.Modules["compat-modules-demo"]
	fromNode := compat.ToEntry(moduleNode)
	if fromNode != entry {
		t.Fatalf("ToEntry(module node) = %p, want cached entry %p", fromNode, entry)
	}
	if got, want := childNames(fromNode.Children()), []string{"z-top", "a-top"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("ToEntry(module node) children = %v, want %v", got, want)
	}
	ms.ClearEntryCache()
	freshFromNode := compat.ToEntry(moduleNode)
	if freshFromNode == fromNode {
		t.Fatalf("ToEntry(module node) after ClearEntryCache = %p, want fresh entry", freshFromNode)
	}
	if got, want := childNames(freshFromNode.Children()), []string{"z-top", "a-top"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("fresh ToEntry(module node) children = %v, want %v", got, want)
	}
	if got := entry.Lookup("z-top").Modules(); got != ms {
		t.Fatalf("child Modules() = %p, want %p", got, ms)
	}
	if processed, skipped := entry.Augment(false); processed != 0 || skipped != 0 {
		t.Fatalf("Augment(false) = (%d,%d), want (0,0)", processed, skipped)
	}
	if errs := entry.ApplyDeviate(compat.DeviateOptions{IgnoreDeviateNotSupported: true}); len(errs) != 0 {
		t.Fatalf("ApplyDeviate errors = %v, want none", errs)
	}
	entry.FixChoice()
}

func TestModulesProcessDeviateOptionsRetainsNotSupportedLikeGoyang(t *testing.T) {
	target := `module compat-deviate-option-target {
    yang-version 1.1;
    namespace "urn:compat-deviate-option-target";
    prefix cdot;

    container top {
        leaf before { type string; }
        leaf value { type string; }
        leaf after { type string; }
    }
}
`
	deviator := `module compat-deviate-option-source {
    yang-version 1.1;
    namespace "urn:compat-deviate-option-source";
    prefix cdos;

    import compat-deviate-option-target {
        prefix cdot;
    }

    deviation "/cdot:top/cdot:value" {
        deviate not-supported;
    }
}
`

	compatModules := compat.NewModules()
	compatModules.ParseOptions.DeviateOptions = compat.DeviateOptions{IgnoreDeviateNotSupported: true}
	if err := compatModules.Parse(target, "compat-deviate-option-target.yang"); err != nil {
		t.Fatalf("compat target Parse: %v", err)
	}
	if err := compatModules.Parse(deviator, "compat-deviate-option-source.yang"); err != nil {
		t.Fatalf("compat deviator Parse: %v", err)
	}
	compatRoot, compatErrs := compatModules.GetModule("compat-deviate-option-target")
	if len(compatErrs) != 0 {
		t.Fatalf("compat GetModule errors = %v", compatErrs)
	}

	upstreamModules := upstream.NewModules()
	upstreamModules.ParseOptions.DeviateOptions = upstream.DeviateOptions{IgnoreDeviateNotSupported: true}
	if err := upstreamModules.Parse(target, "compat-deviate-option-target.yang"); err != nil {
		t.Fatalf("upstream target Parse: %v", err)
	}
	if err := upstreamModules.Parse(deviator, "compat-deviate-option-source.yang"); err != nil {
		t.Fatalf("upstream deviator Parse: %v", err)
	}
	upstreamRoot, upstreamErrs := upstreamModules.GetModule("compat-deviate-option-target")
	if len(upstreamErrs) != 0 {
		t.Fatalf("upstream GetModule errors = %v", upstreamErrs)
	}

	compatTop := compatRoot.Find("/cdot:top")
	upstreamTop := upstreamRoot.Find("/cdot:top")
	if compatTop == nil || upstreamTop == nil {
		t.Fatalf("top entries = (%#v,%#v), want both non-nil", compatTop, upstreamTop)
	}
	if (compatTop.Lookup("value") == nil) != (upstreamTop.Dir["value"] == nil) {
		t.Fatalf("value retained = %v, want goyang %v", compatTop.Lookup("value") != nil, upstreamTop.Dir["value"] != nil)
	}
	if got, want := childNames(compatTop.Children()), []string{"before", "value", "after"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("top children = %v, want declaration order %v", got, want)
	}
}

func TestManualSubmoduleGetPrefixMatchesGoyang(t *testing.T) {
	tests := []struct {
		name     string
		compat   *compat.Module
		upstream *upstream.Module
	}{
		{
			name: "belongs-to prefix",
			compat: &compat.Module{
				Name: "compat-manual-submodule",
				BelongsTo: &compat.BelongsTo{
					Name:   "compat-manual-owner",
					Prefix: &compat.ASTValue{Name: "cmo"},
				},
			},
			upstream: &upstream.Module{
				Name: "compat-manual-submodule",
				BelongsTo: &upstream.BelongsTo{
					Name:   "compat-manual-owner",
					Prefix: &upstream.Value{Name: "cmo"},
				},
			},
		},
		{
			name: "missing belongs-to prefix ignores module prefix",
			compat: &compat.Module{
				Name:      "compat-manual-submodule",
				BelongsTo: &compat.BelongsTo{Name: "compat-manual-owner"},
				Prefix:    &compat.Value{Name: "wrong"},
			},
			upstream: &upstream.Module{
				Name:      "compat-manual-submodule",
				BelongsTo: &upstream.BelongsTo{Name: "compat-manual-owner"},
				Prefix:    &upstream.Value{Name: "wrong"},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got, want := tt.compat.GetPrefix(), tt.upstream.GetPrefix(); got != want {
				t.Fatalf("manual submodule GetPrefix() = %q, want goyang %q", got, want)
			}
		})
	}
}

func TestParsedSubmodulePrefixFieldMatchesGoyang(t *testing.T) {
	source := `submodule compat-parsed-submodule-prefix {
    belongs-to compat-parsed-owner {
        prefix cpo;
    }
}
`
	compatModules := compat.NewModules()
	if err := compatModules.Parse(source, "compat-parsed-submodule-prefix.yang"); err != nil {
		t.Fatalf("compat Parse: %v", err)
	}
	upstreamModules := upstream.NewModules()
	if err := upstreamModules.Parse(source, "compat-parsed-submodule-prefix.yang"); err != nil {
		t.Fatalf("upstream Parse: %v", err)
	}

	compatSubmodule := compatModules.SubModules["compat-parsed-submodule-prefix"]
	upstreamSubmodule := upstreamModules.SubModules["compat-parsed-submodule-prefix"]
	if compatSubmodule == nil || upstreamSubmodule == nil {
		t.Fatalf("SubModules = (%#v,%#v), want both non-nil", compatSubmodule, upstreamSubmodule)
	}
	if (compatSubmodule.Prefix == nil) != (upstreamSubmodule.Prefix == nil) {
		t.Fatalf("submodule Prefix nil = %v, want goyang %v", compatSubmodule.Prefix == nil, upstreamSubmodule.Prefix == nil)
	}
	if compatSubmodule.Prefix != nil && compatSubmodule.Prefix.Name != upstreamSubmodule.Prefix.Name {
		t.Fatalf("submodule Prefix.Name = %q, want goyang %q", compatSubmodule.Prefix.Name, upstreamSubmodule.Prefix.Name)
	}
	if got, want := compatSubmodule.GetPrefix(), upstreamSubmodule.GetPrefix(); got != want {
		t.Fatalf("submodule GetPrefix() = %q, want goyang %q", got, want)
	}
}

func TestModulesParseDuplicateModuleMatchesGoyang(t *testing.T) {
	source := `module compat-duplicate-module {
    namespace "urn:compat-duplicate-module";
    prefix cdm;
}
`
	compatModules := compat.NewModules()
	if err := compatModules.Parse(source, "compat-duplicate-module-a.yang"); err != nil {
		t.Fatalf("compat first Parse: %v", err)
	}
	compatErr := compatModules.Parse(source, "compat-duplicate-module-b.yang")

	upstreamModules := upstream.NewModules()
	if err := upstreamModules.Parse(source, "compat-duplicate-module-a.yang"); err != nil {
		t.Fatalf("upstream first Parse: %v", err)
	}
	upstreamErr := upstreamModules.Parse(source, "compat-duplicate-module-b.yang")

	if (compatErr == nil) != (upstreamErr == nil) {
		t.Fatalf("duplicate Parse error nil = %v, want goyang %v", compatErr == nil, upstreamErr == nil)
	}
	if compatErr != nil && compatErr.Error() != upstreamErr.Error() {
		t.Fatalf("duplicate Parse error = %q, want goyang %q", compatErr, upstreamErr)
	}
}

func TestModulesParseTopLevelNonModuleMatchesGoyang(t *testing.T) {
	source := `container compat-top-level-container {
    leaf value { type string; }
}
`
	compatErr := compat.NewModules().Parse(source, "compat-top-level-container.yang")
	upstreamErr := upstream.NewModules().Parse(source, "compat-top-level-container.yang")

	if (compatErr == nil) != (upstreamErr == nil) {
		t.Fatalf("top-level non-module Parse error nil = %v, want goyang %v", compatErr == nil, upstreamErr == nil)
	}
	if compatErr != nil && compatErr.Error() != upstreamErr.Error() {
		t.Fatalf("top-level non-module Parse error = %q, want goyang %q", compatErr, upstreamErr)
	}
}

func TestModulesProcessEmptyNamespaceMatchesGoyang(t *testing.T) {
	source := `module compat-empty-namespace {
    namespace "";
    prefix cen;

    leaf value { type string; }
}
`
	compatModules := compat.NewModules()
	if err := compatModules.Parse(source, "compat-empty-namespace.yang"); err != nil {
		t.Fatalf("compat Parse: %v", err)
	}
	upstreamModules := upstream.NewModules()
	if err := upstreamModules.Parse(source, "compat-empty-namespace.yang"); err != nil {
		t.Fatalf("upstream Parse: %v", err)
	}

	compatErrs := compatModules.Process()
	upstreamErrs := upstreamModules.Process()
	if len(compatErrs) != len(upstreamErrs) {
		t.Fatalf("Process errors = %v, want goyang %v", compatErrs, upstreamErrs)
	}

	compatModule, compatErr := compatModules.FindModuleByNamespace("")
	upstreamModule, upstreamErr := upstreamModules.FindModuleByNamespace("")
	if (compatErr == nil) != (upstreamErr == nil) {
		t.Fatalf("FindModuleByNamespace(\"\") errors = (%v,%v), want goyang parity", compatErr, upstreamErr)
	}
	if (compatModule == nil) != (upstreamModule == nil) {
		t.Fatalf("FindModuleByNamespace(\"\") modules = (%#v,%#v), want goyang parity", compatModule, upstreamModule)
	}
	if compatModule != nil && upstreamModule != nil && compatModule.Name != upstreamModule.Name {
		t.Fatalf("FindModuleByNamespace(\"\").Name = %q, want goyang %q", compatModule.Name, upstreamModule.Name)
	}
}

func TestModulesProcessStoreUsesMatchesGoyang(t *testing.T) {
	source := `module compat-store-uses {
    namespace "urn:compat-store-uses";
    prefix csu;

    grouping common {
        leaf grouped { type string; }
    }

    container top {
        uses common;
    }
}
`
	compatModules := compat.NewModules()
	compatModules.ParseOptions.StoreUses = true
	if err := compatModules.Parse(source, "compat-store-uses.yang"); err != nil {
		t.Fatalf("compat Parse: %v", err)
	}
	compatRoot, compatErrs := compatModules.GetModule("compat-store-uses")
	if len(compatErrs) != 0 {
		t.Fatalf("compat GetModule errors: %v", compatErrs)
	}

	upstreamModules := upstream.NewModules()
	upstreamModules.ParseOptions.StoreUses = true
	if err := upstreamModules.Parse(source, "compat-store-uses.yang"); err != nil {
		t.Fatalf("upstream Parse: %v", err)
	}
	upstreamRoot, upstreamErrs := upstreamModules.GetModule("compat-store-uses")
	if len(upstreamErrs) != 0 {
		t.Fatalf("upstream GetModule errors: %v", upstreamErrs)
	}

	compatTop := compatRoot.Lookup("top")
	upstreamTop := upstreamRoot.Dir["top"]
	if compatTop == nil || upstreamTop == nil {
		t.Fatalf("top entries = (%#v,%#v), want both non-nil", compatTop, upstreamTop)
	}
	if got, want := len(compatTop.Uses), len(upstreamTop.Uses); got != want {
		t.Fatalf("top Uses len = %d, want goyang %d", got, want)
	}
	if len(compatTop.Uses) != 0 {
		if got, want := compatTop.Uses[0].Uses.Name, upstreamTop.Uses[0].Uses.Name; got != want {
			t.Fatalf("top Uses[0].Uses.Name = %q, want goyang %q", got, want)
		}
		if got, want := compatTop.Uses[0].Grouping.Name, upstreamTop.Uses[0].Grouping.Name; got != want {
			t.Fatalf("top Uses[0].Grouping.Name = %q, want goyang %q", got, want)
		}
	}
}

func TestModulesProcessMissingReferenceMatchesGoyang(t *testing.T) {
	tests := []struct {
		name       string
		sourceName string
		source     string
	}{
		{
			name:       "import",
			sourceName: "compat-missing-import.yang",
			source: `module compat-missing-import {
    namespace "urn:compat-missing-import";
    prefix cmi;

    import compat-missing-target {
        prefix cmt;
    }
}
`,
		},
		{
			name:       "include",
			sourceName: "compat-missing-include.yang",
			source: `module compat-missing-include {
    namespace "urn:compat-missing-include";
    prefix cmi;

    include compat-missing-submodule;
}
`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			compatModules := compat.NewModules()
			if err := compatModules.Parse(tt.source, tt.sourceName); err != nil {
				t.Fatalf("compat Parse: %v", err)
			}
			upstreamModules := upstream.NewModules()
			if err := upstreamModules.Parse(tt.source, tt.sourceName); err != nil {
				t.Fatalf("upstream Parse: %v", err)
			}

			compatErrs := compatModules.Process()
			upstreamErrs := upstreamModules.Process()
			if len(compatErrs) != len(upstreamErrs) {
				t.Fatalf("Process errors = %v, want goyang %v", compatErrs, upstreamErrs)
			}
			if len(compatErrs) == 0 {
				t.Fatal("Process errors empty, test fixture did not exercise missing reference")
			}
			if got, want := compatErrs[0].Error(), upstreamErrs[0].Error(); got != want {
				t.Fatalf("Process error = %q, want goyang %q", got, want)
			}
		})
	}
}

func TestModulesGetModuleWrongFileModuleNameMatchesGoyang(t *testing.T) {
	dir := t.TempDir()
	source := `module compat-wrong-file-actual {
    namespace "urn:compat-wrong-file-actual";
    prefix cwfa;

    import compat-wrong-file-missing { prefix cwfm; }
}
`
	if err := os.WriteFile(filepath.Join(dir, "compat-wrong-file-request.yang"), []byte(source), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	compatModules := compat.NewModules()
	compatModules.AddPath(dir)
	_, compatErrs := compatModules.GetModule("compat-wrong-file-request")

	upstreamModules := upstream.NewModules()
	upstreamModules.AddPath(dir)
	_, upstreamErrs := upstreamModules.GetModule("compat-wrong-file-request")

	if len(compatErrs) != len(upstreamErrs) {
		t.Fatalf("GetModule errors = %v, want goyang %v", compatErrs, upstreamErrs)
	}
	if len(compatErrs) == 0 {
		t.Fatal("GetModule errors empty, test fixture did not exercise missing requested module")
	}
	if got, want := compatErrs[0].Error(), upstreamErrs[0].Error(); got != want {
		t.Fatalf("GetModule error = %q, want goyang %q", got, want)
	}
	if !strings.Contains(compatErrs[0].Error(), "module not found: compat-wrong-file-request") {
		t.Fatalf("GetModule error = %q, want module not found", compatErrs[0])
	}
}

func TestModulesGetModuleEmptyNameMatchesGoyang(t *testing.T) {
	_, compatErrs := compat.NewModules().GetModule("")
	_, upstreamErrs := upstream.NewModules().GetModule("")

	if len(compatErrs) != len(upstreamErrs) {
		t.Fatalf("GetModule errors = %v, want goyang %v", compatErrs, upstreamErrs)
	}
	if len(compatErrs) == 0 {
		t.Fatal("GetModule errors empty, test fixture did not exercise empty module name")
	}
	if got, want := compatErrs[0].Error(), upstreamErrs[0].Error(); got != want {
		t.Fatalf("GetModule error = %q, want goyang %q", got, want)
	}
}

func TestModulesFacadeModuleMetadata(t *testing.T) {
	mainSource := `module compat-metadata-demo {
    yang-version 1.1;
    namespace "urn:compat-metadata-demo";
    prefix cmd;
    import compat-metadata-import {
        prefix cmi;
        revision-date 2026-01-01;
        description "Import description.";
        reference "Import reference.";
    }
    include compat-metadata-sub {
        revision-date 2026-01-03;
    }
    organization "Signalbreak Labs";
    contact "ops@example.test";
    description "Metadata module.";
    reference "Metadata reference.";
    revision 2026-01-02 {
        description "Current revision.";
        reference "Revision reference.";
    }
    extension local-ext {
        argument value;
    }
    feature fast-mode {
        description "Fast mode.";
    }
    identity local-id;
    typedef local-type { type string; }
    grouping local-group {
        leaf local-group-leaf { type string; }
    }
    anydata opaque {
        description "Opaque anydata.";
    }
    anyxml legacy {
        description "Legacy anyxml.";
    }
    leaf top-leaf {
        type string;
        default "leaf-default";
        units "widgets";
    }
    leaf-list top-leaf-list {
        type string;
        default "blue";
        default "green";
        min-elements 0;
        max-elements 3;
        ordered-by user;
    }
    list top-list {
        key "name";
        min-elements 1;
        max-elements 2;
        ordered-by user;
        leaf name { type string; }
    }
    choice top-choice {
        case one {
            leaf selected { type string; }
        }
    }
    augment "/top" {
        leaf augmented { type string; }
    }
    deviation "/top/local" {
        deviate add {
            default "fallback";
        }
    }
    container top {
        uses local-group {
            description "Local group use.";
            reference "Local group reference.";
            refine local-group-leaf {
                default "refined";
                description "Refined grouping leaf.";
            }
        }
        uses sub-group;
        leaf imported { type cmi:imported-type; }
        leaf local { type local-type; }
    }
    rpc reset {
        input {
            leaf request { type string; }
        }
        output {
            leaf response { type string; }
        }
    }
    notification alarm {
        leaf severity { type string; }
    }
}
`
	importSource := `module compat-metadata-import {
    namespace "urn:compat-metadata-import";
    prefix cmi;
    revision 2026-01-01;
    typedef imported-type { type string; }
}
`
	subSource := `submodule compat-metadata-sub {
    belongs-to compat-metadata-demo { prefix cmd; }
    revision 2026-01-03;
    grouping sub-group {
        leaf sub-leaf { type string; }
    }
}
`

	dir := t.TempDir()
	writeYANG(t, filepath.Join(dir, "compat-metadata-demo.yang"), mainSource)
	writeYANG(t, filepath.Join(dir, "compat-metadata-import@2026-01-01.yang"), importSource)
	writeYANG(t, filepath.Join(dir, "compat-metadata-sub@2026-01-03.yang"), subSource)

	ms := compat.NewModules()
	ms.AddPath(dir)
	if err := ms.Read("compat-metadata-demo"); err != nil {
		t.Fatalf("Read: %v", err)
	}

	mod := ms.Modules["compat-metadata-demo"]
	if mod == nil {
		t.Fatal("module record not found after Parse")
	}
	assertModuleMetadata(t, mod)
	assertModuleNodeShape(t, mod, "module")
	assertModuleTopLevelAST(t, mod)
	if mod.Modules != ms {
		t.Fatalf("module Modules = %p, want %p", mod.Modules, ms)
	}
	if len(mod.Import) != 1 || mod.Import[0].Name != "compat-metadata-import" {
		t.Fatalf("module Import = %#v, want compat-metadata-import", mod.Import)
	}
	if got := mod.Import[0].Prefix.Name; got != "cmi" {
		t.Fatalf("import Prefix = %q, want cmi", got)
	}
	if got := mod.Import[0].RevisionDate.Name; got != "2026-01-01" {
		t.Fatalf("import RevisionDate = %q, want 2026-01-01", got)
	}
	if got := mod.Import[0].Description.Name; got != "Import description." {
		t.Fatalf("import Description = %q, want Import description.", got)
	}
	if got := mod.Import[0].Reference.Name; got != "Import reference." {
		t.Fatalf("import Reference = %q, want Import reference.", got)
	}
	assertImportNodeShape(t, mod.Import[0], "compat-metadata-demo.yang:5:5", mod)
	if len(mod.Include) != 1 || mod.Include[0].Name != "compat-metadata-sub" {
		t.Fatalf("module Include = %#v, want compat-metadata-sub", mod.Include)
	}
	if got := mod.Include[0].RevisionDate.Name; got != "2026-01-03" {
		t.Fatalf("include RevisionDate = %q, want 2026-01-03", got)
	}
	assertIncludeNodeShape(t, mod.Include[0], "compat-metadata-demo.yang:11:5", mod)

	if errs := ms.Process(); len(errs) != 0 {
		t.Fatalf("Process errors = %v, want none", errs)
	}
	assertModuleMetadata(t, ms.Modules["compat-metadata-demo"])
	assertModuleNodeShape(t, ms.Modules["compat-metadata-demo"], "module")
	assertModuleTopLevelAST(t, ms.Modules["compat-metadata-demo"])
}

func TestPackageGetModuleReadsSourceFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "compat-file-demo.yang")
	source := `module compat-file-demo {
    namespace "urn:compat-file-demo";
    prefix cfd;

    container beta { leaf value { type string; } }
}
`
	if err := os.WriteFile(path, []byte(source), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	entry, errs := compat.GetModule("compat-file-demo", path)
	if len(errs) != 0 {
		t.Fatalf("GetModule errors = %v, want none", errs)
	}
	if entry == nil || entry.Lookup("beta") == nil {
		t.Fatalf("GetModule entry = %#v, want beta child", entry)
	}
}

func assertModuleMetadata(t *testing.T, mod *compat.Module) {
	t.Helper()
	if mod.YangVersion == nil || mod.YangVersion.Name != "1.1" {
		t.Fatalf("YangVersion = %#v, want 1.1", mod.YangVersion)
	}
	if mod.Organization == nil || mod.Organization.Name != "Signalbreak Labs" {
		t.Fatalf("Organization = %#v, want Signalbreak Labs", mod.Organization)
	}
	if mod.Contact == nil || mod.Contact.Name != "ops@example.test" {
		t.Fatalf("Contact = %#v, want ops@example.test", mod.Contact)
	}
	if mod.Description == nil || mod.Description.Name != "Metadata module." {
		t.Fatalf("Description = %#v, want Metadata module.", mod.Description)
	}
	if mod.Reference == nil || mod.Reference.Name != "Metadata reference." {
		t.Fatalf("Reference = %#v, want Metadata reference.", mod.Reference)
	}
	if len(mod.Revision) != 1 || mod.Revision[0].Name != "2026-01-02" {
		t.Fatalf("Revision = %#v, want 2026-01-02", mod.Revision)
	}
	if got := mod.Revision[0].Description.Name; got != "Current revision." {
		t.Fatalf("revision Description = %q, want Current revision.", got)
	}
	if got := mod.Revision[0].Reference.Name; got != "Revision reference." {
		t.Fatalf("revision Reference = %q, want Revision reference.", got)
	}
}

func assertModuleNodeShape(t *testing.T, mod *compat.Module, wantKind string) {
	t.Helper()
	var node compat.Node = mod
	if got := node.Kind(); got != wantKind {
		t.Fatalf("module Kind = %q, want %q", got, wantKind)
	}
	if got := node.NName(); got != "compat-metadata-demo" {
		t.Fatalf("module NName = %q, want compat-metadata-demo", got)
	}
	if node.ParentNode() != nil {
		t.Fatalf("module ParentNode = %#v, want nil", node.ParentNode())
	}
	if mod.Source == nil || mod.Source.Keyword != "module" || mod.Source.Argument != "compat-metadata-demo" {
		t.Fatalf("module Source = %#v, want module statement", mod.Source)
	}
	if node.Statement() != mod.Source {
		t.Fatal("module Statement() does not return Source")
	}
	if got := compat.Source(node); !strings.HasSuffix(got, "compat-metadata-demo.yang:1:1") {
		t.Fatalf("module Source location = %q, want compat-metadata-demo.yang:1:1 suffix", got)
	}
	if got := node.Exts(); len(got) != 0 {
		t.Fatalf("module Exts = %#v, want empty", got)
	}
	if got := mod.Groupings(); len(got) != 1 || got[0].Name != "local-group" {
		t.Fatalf("module Groupings = %#v, want local-group", got)
	}
	if got := mod.Typedefs(); len(got) != 1 || got[0].Name != "local-type" {
		t.Fatalf("module Typedefs = %#v, want local-type", got)
	}
	moduleIdentities := mod.Identity
	if len(moduleIdentities) != 1 || moduleIdentities[0].Name != "local-id" {
		t.Fatalf("Module.Identity = %#v, want local-id compat identities", moduleIdentities)
	}
	var identityNode compat.Node = moduleIdentities[0]
	if identityNode.Kind() != "identity" || identityNode.NName() != "local-id" {
		t.Fatalf("Module.Identity[0] node = %s/%s, want identity/local-id", identityNode.Kind(), identityNode.NName())
	}
	if got := mod.Identities(); len(got) != 1 || got[0].Name != "local-id" {
		t.Fatalf("module Identities = %#v, want local-id", got)
	}
	assertScalarNodeShape(t, "YangVersion", mod.YangVersion, "yang-version", "1.1", "compat-metadata-demo.yang:2:5", mod)
	assertScalarNodeShape(t, "Namespace", mod.Namespace, "namespace", "urn:compat-metadata-demo", "compat-metadata-demo.yang:3:5", mod)
	assertScalarNodeShape(t, "Prefix", mod.Prefix, "prefix", "cmd", "compat-metadata-demo.yang:4:5", mod)
	assertScalarNodeShape(t, "Organization", mod.Organization, "organization", "Signalbreak Labs", "compat-metadata-demo.yang:14:5", mod)
	assertScalarNodeShape(t, "Contact", mod.Contact, "contact", "ops@example.test", "compat-metadata-demo.yang:15:5", mod)
	assertScalarNodeShape(t, "Description", mod.Description, "description", "Metadata module.", "compat-metadata-demo.yang:16:5", mod)
	assertScalarNodeShape(t, "Reference", mod.Reference, "reference", "Metadata reference.", "compat-metadata-demo.yang:17:5", mod)
	if len(mod.Revision) != 1 {
		t.Fatalf("module Revision = %#v, want one revision", mod.Revision)
	}
	assertRevisionNodeShape(t, mod.Revision[0], "compat-metadata-demo.yang:18:5", mod)
	assertScalarNodeShape(t, "Revision.Description", mod.Revision[0].Description, "description", "Current revision.", "compat-metadata-demo.yang:19:9", mod.Revision[0])
	assertScalarNodeShape(t, "Revision.Reference", mod.Revision[0].Reference, "reference", "Revision reference.", "compat-metadata-demo.yang:20:9", mod.Revision[0])
}

func assertImportNodeShape(t *testing.T, imp *compat.Import, sourceSuffix string, parent *compat.Module) {
	t.Helper()
	if imp == nil {
		t.Fatal("import node is nil")
	}
	if got := imp.Kind(); got != "import" {
		t.Fatalf("import Kind = %q, want import", got)
	}
	if got := imp.NName(); got != imp.Name {
		t.Fatalf("import NName = %q, want %q", got, imp.Name)
	}
	if got := imp.ParentNode(); got != parent {
		t.Fatalf("import ParentNode = %#v, want parent module", got)
	}
	if imp.Statement() == nil || imp.Statement().Keyword != "import" || imp.Statement().Argument != imp.Name {
		t.Fatalf("import Statement = %#v, want import statement", imp.Statement())
	}
	if got := compat.Source(imp); !strings.HasSuffix(got, sourceSuffix) {
		t.Fatalf("import Source = %q, want suffix %q", got, sourceSuffix)
	}
}

func assertIncludeNodeShape(t *testing.T, inc *compat.Include, sourceSuffix string, parent *compat.Module) {
	t.Helper()
	if inc == nil {
		t.Fatal("include node is nil")
	}
	if got := inc.Kind(); got != "include" {
		t.Fatalf("include Kind = %q, want include", got)
	}
	if got := inc.NName(); got != inc.Name {
		t.Fatalf("include NName = %q, want %q", got, inc.Name)
	}
	if got := inc.ParentNode(); got != parent {
		t.Fatalf("include ParentNode = %#v, want parent module", got)
	}
	if inc.Statement() == nil || inc.Statement().Keyword != "include" || inc.Statement().Argument != inc.Name {
		t.Fatalf("include Statement = %#v, want include statement", inc.Statement())
	}
	if got := compat.Source(inc); !strings.HasSuffix(got, sourceSuffix) {
		t.Fatalf("include Source = %q, want suffix %q", got, sourceSuffix)
	}
}

func assertScalarNodeShape(t *testing.T, label string, node compat.Node, keyword, name, sourceSuffix string, parent compat.Node) {
	t.Helper()
	if node == nil {
		t.Fatalf("%s node is nil", label)
	}
	if got := node.Kind(); got != "string" {
		t.Fatalf("%s Kind = %q, want string", label, got)
	}
	if got := node.NName(); got != name {
		t.Fatalf("%s NName = %q, want %q", label, got, name)
	}
	if got := node.ParentNode(); got != parent {
		t.Fatalf("%s ParentNode = %#v, want parent node", label, got)
	}
	if node.Statement() == nil || node.Statement().Keyword != keyword || node.Statement().Argument != name {
		t.Fatalf("%s Statement = %#v, want %s statement", label, node.Statement(), keyword)
	}
	if got := compat.Source(node); !strings.HasSuffix(got, sourceSuffix) {
		t.Fatalf("%s Source = %q, want suffix %q", label, got, sourceSuffix)
	}
	if got := node.Exts(); len(got) != 0 {
		t.Fatalf("%s Exts = %#v, want empty", label, got)
	}
}

func assertRevisionNodeShape(t *testing.T, rev *compat.Revision, sourceSuffix string, parent *compat.Module) {
	t.Helper()
	if rev == nil {
		t.Fatal("revision node is nil")
	}
	if got := rev.Kind(); got != "revision" {
		t.Fatalf("revision Kind = %q, want revision", got)
	}
	if got := rev.NName(); got != rev.Name {
		t.Fatalf("revision NName = %q, want %q", got, rev.Name)
	}
	if got := rev.ParentNode(); got != parent {
		t.Fatalf("revision ParentNode = %#v, want parent module", got)
	}
	if rev.Statement() == nil || rev.Statement().Keyword != "revision" || rev.Statement().Argument != rev.Name {
		t.Fatalf("revision Statement = %#v, want revision statement", rev.Statement())
	}
	if got := compat.Source(rev); !strings.HasSuffix(got, sourceSuffix) {
		t.Fatalf("revision Source = %q, want suffix %q", got, sourceSuffix)
	}
	if got := rev.Exts(); len(got) != 0 {
		t.Fatalf("revision Exts = %#v, want empty", got)
	}
}

func assertASTNodeShape(t *testing.T, label string, node compat.Node, kind, name, sourceSuffix string, parent compat.Node) {
	t.Helper()
	if node == nil {
		t.Fatalf("%s node is nil", label)
	}
	if got := node.Kind(); got != kind {
		t.Fatalf("%s Kind = %q, want %q", label, got, kind)
	}
	if got := node.NName(); got != name {
		t.Fatalf("%s NName = %q, want %q", label, got, name)
	}
	if got := node.ParentNode(); got != parent {
		t.Fatalf("%s ParentNode = %#v, want parent node", label, got)
	}
	if node.Statement() == nil || node.Statement().Keyword != kind || node.Statement().Argument != name {
		t.Fatalf("%s Statement = %#v, want %s statement", label, node.Statement(), kind)
	}
	if got := compat.Source(node); !strings.HasSuffix(got, sourceSuffix) {
		t.Fatalf("%s Source = %q, want suffix %q", label, got, sourceSuffix)
	}
	if got := node.Exts(); len(got) != 0 {
		t.Fatalf("%s Exts = %#v, want empty", label, got)
	}
}

func assertTypeNodeShape(t *testing.T, label string, typ *compat.Type, name, sourceSuffix string, parent compat.Node) {
	t.Helper()
	if typ == nil {
		t.Fatalf("%s node is nil", label)
	}
	if got := typ.Kind(); got != "type" {
		t.Fatalf("%s Kind = %q, want type", label, got)
	}
	if got := typ.NName(); got != name {
		t.Fatalf("%s NName = %q, want %q", label, got, name)
	}
	if got := typ.ParentNode(); got != parent {
		t.Fatalf("%s ParentNode = %#v, want parent node", label, got)
	}
	if typ.Statement() == nil || typ.Statement().Keyword != "type" || typ.Statement().Argument != name {
		t.Fatalf("%s Statement = %#v, want type statement", label, typ.Statement())
	}
	if got := compat.Source(typ); !strings.HasSuffix(got, sourceSuffix) {
		t.Fatalf("%s Source = %q, want suffix %q", label, got, sourceSuffix)
	}
	if got := typ.Exts(); len(got) != 0 {
		t.Fatalf("%s Exts = %#v, want empty", label, got)
	}
}

func assertModuleTopLevelAST(t *testing.T, mod *compat.Module) {
	t.Helper()
	if len(mod.Extension) != 1 || mod.Extension[0].Name != "local-ext" || mod.Extension[0].Argument == nil || mod.Extension[0].Argument.Name != "value" {
		t.Fatalf("module Extension = %#v, want local-ext argument value", mod.Extension)
	}
	if len(mod.Feature) != 1 || mod.Feature[0].Name != "fast-mode" || mod.Feature[0].Description == nil || mod.Feature[0].Description.Name != "Fast mode." {
		t.Fatalf("module Feature = %#v, want fast-mode", mod.Feature)
	}
	if len(mod.Typedef) != 1 || mod.Typedef[0].Name != "local-type" {
		t.Fatalf("module Typedef = %#v, want local-type", mod.Typedef)
	}
	assertASTNodeShape(t, "Typedef", mod.Typedef[0], "typedef", "local-type", "compat-metadata-demo.yang:29:5", mod)
	assertTypeNodeShape(t, "Typedef.Type", mod.Typedef[0].Type, "string", "compat-metadata-demo.yang:29:26", mod.Typedef[0])
	if len(mod.Grouping) != 1 || mod.Grouping[0].Name != "local-group" {
		t.Fatalf("module Grouping = %#v, want local-group", mod.Grouping)
	}
	assertASTNodeShape(t, "Grouping", mod.Grouping[0], "grouping", "local-group", "compat-metadata-demo.yang:30:5", mod)
	if got, want := len(mod.Grouping[0].Leaf), 1; got != want {
		t.Fatalf("Grouping.Leaf len = %d, want %d", got, want)
	}
	assertTypeNodeShape(t, "Grouping.Leaf[0].Type", mod.Grouping[0].Leaf[0].Type, "string", "compat-metadata-demo.yang:31:33", mod.Grouping[0].Leaf[0])
	if len(mod.Anydata) != 1 || mod.Anydata[0].Name != "opaque" {
		t.Fatalf("module Anydata = %#v, want opaque", mod.Anydata)
	}
	assertASTNodeShape(t, "Anydata", mod.Anydata[0], "anydata", "opaque", "compat-metadata-demo.yang:33:5", mod)
	assertScalarNodeShape(t, "Anydata.Description", mod.Anydata[0].Description, "description", "Opaque anydata.", "compat-metadata-demo.yang:34:9", mod.Anydata[0])
	if len(mod.Anyxml) != 1 || mod.Anyxml[0].Name != "legacy" {
		t.Fatalf("module Anyxml = %#v, want legacy", mod.Anyxml)
	}
	assertASTNodeShape(t, "Anyxml", mod.Anyxml[0], "anyxml", "legacy", "compat-metadata-demo.yang:36:5", mod)
	assertScalarNodeShape(t, "Anyxml.Description", mod.Anyxml[0].Description, "description", "Legacy anyxml.", "compat-metadata-demo.yang:37:9", mod.Anyxml[0])
	if len(mod.Leaf) != 1 || mod.Leaf[0].Name != "top-leaf" {
		t.Fatalf("module Leaf = %#v, want top-leaf", mod.Leaf)
	}
	assertTypeNodeShape(t, "Leaf.Type", mod.Leaf[0].Type, "string", "compat-metadata-demo.yang:40:9", mod.Leaf[0])
	assertScalarNodeShape(t, "Leaf.Default", mod.Leaf[0].Default, "default", "leaf-default", "compat-metadata-demo.yang:41:9", mod.Leaf[0])
	assertScalarNodeShape(t, "Leaf.Units", mod.Leaf[0].Units, "units", "widgets", "compat-metadata-demo.yang:42:9", mod.Leaf[0])
	if len(mod.LeafList) != 1 || mod.LeafList[0].Name != "top-leaf-list" {
		t.Fatalf("module LeafList = %#v, want top-leaf-list", mod.LeafList)
	}
	assertTypeNodeShape(t, "LeafList.Type", mod.LeafList[0].Type, "string", "compat-metadata-demo.yang:45:9", mod.LeafList[0])
	if got, want := len(mod.LeafList[0].Default), 2; got != want {
		t.Fatalf("LeafList.Default len = %d, want %d", got, want)
	}
	assertScalarNodeShape(t, "LeafList.Default[0]", mod.LeafList[0].Default[0], "default", "blue", "compat-metadata-demo.yang:46:9", mod.LeafList[0])
	assertScalarNodeShape(t, "LeafList.Default[1]", mod.LeafList[0].Default[1], "default", "green", "compat-metadata-demo.yang:47:9", mod.LeafList[0])
	assertScalarNodeShape(t, "LeafList.MinElements", mod.LeafList[0].MinElements, "min-elements", "0", "compat-metadata-demo.yang:48:9", mod.LeafList[0])
	assertScalarNodeShape(t, "LeafList.MaxElements", mod.LeafList[0].MaxElements, "max-elements", "3", "compat-metadata-demo.yang:49:9", mod.LeafList[0])
	assertScalarNodeShape(t, "LeafList.OrderedBy", mod.LeafList[0].OrderedBy, "ordered-by", "user", "compat-metadata-demo.yang:50:9", mod.LeafList[0])
	if len(mod.List) != 1 || mod.List[0].Name != "top-list" || mod.List[0].Key == nil || mod.List[0].Key.Name != "name" {
		t.Fatalf("module List = %#v, want top-list key name", mod.List)
	}
	assertScalarNodeShape(t, "List.Key", mod.List[0].Key, "key", "name", "compat-metadata-demo.yang:53:9", mod.List[0])
	assertScalarNodeShape(t, "List.MinElements", mod.List[0].MinElements, "min-elements", "1", "compat-metadata-demo.yang:54:9", mod.List[0])
	assertScalarNodeShape(t, "List.MaxElements", mod.List[0].MaxElements, "max-elements", "2", "compat-metadata-demo.yang:55:9", mod.List[0])
	assertScalarNodeShape(t, "List.OrderedBy", mod.List[0].OrderedBy, "ordered-by", "user", "compat-metadata-demo.yang:56:9", mod.List[0])
	if got, want := len(mod.List[0].Leaf), 1; got != want {
		t.Fatalf("List.Leaf len = %d, want %d", got, want)
	}
	assertTypeNodeShape(t, "List.Leaf[0].Type", mod.List[0].Leaf[0].Type, "string", "compat-metadata-demo.yang:57:21", mod.List[0].Leaf[0])
	if len(mod.Choice) != 1 || mod.Choice[0].Name != "top-choice" {
		t.Fatalf("module Choice = %#v, want top-choice", mod.Choice)
	}
	assertASTNodeShape(t, "Choice", mod.Choice[0], "choice", "top-choice", "compat-metadata-demo.yang:59:5", mod)
	if got, want := len(mod.Choice[0].Case), 1; got != want {
		t.Fatalf("Choice.Case len = %d, want %d", got, want)
	}
	assertASTNodeShape(t, "Choice.Case[0]", mod.Choice[0].Case[0], "case", "one", "compat-metadata-demo.yang:60:9", mod.Choice[0])
	if got, want := len(mod.Choice[0].Case[0].Leaf), 1; got != want {
		t.Fatalf("Choice.Case[0].Leaf len = %d, want %d", got, want)
	}
	assertTypeNodeShape(t, "Choice.Case[0].Leaf[0].Type", mod.Choice[0].Case[0].Leaf[0].Type, "string", "compat-metadata-demo.yang:61:29", mod.Choice[0].Case[0].Leaf[0])
	if len(mod.Augment) != 1 || mod.Augment[0].Name != "/top" {
		t.Fatalf("module Augment = %#v, want /top", mod.Augment)
	}
	assertASTNodeShape(t, "Augment", mod.Augment[0], "augment", "/top", "compat-metadata-demo.yang:64:5", mod)
	if got, want := len(mod.Augment[0].Leaf), 1; got != want {
		t.Fatalf("Augment.Leaf len = %d, want %d", got, want)
	}
	assertTypeNodeShape(t, "Augment.Leaf[0].Type", mod.Augment[0].Leaf[0].Type, "string", "compat-metadata-demo.yang:65:26", mod.Augment[0].Leaf[0])
	if len(mod.Deviation) != 1 || mod.Deviation[0].Name != "/top/local" || len(mod.Deviation[0].Deviate) != 1 || mod.Deviation[0].Deviate[0].Name != "add" {
		t.Fatalf("module Deviation = %#v, want /top/local add", mod.Deviation)
	}
	if len(mod.Container) != 1 || mod.Container[0].Name != "top" {
		t.Fatalf("module Container = %#v, want top", mod.Container)
	}
	assertASTNodeShape(t, "Container", mod.Container[0], "container", "top", "compat-metadata-demo.yang:72:5", mod)
	if got, want := len(mod.Container[0].Uses), 2; got != want {
		t.Fatalf("Container.Uses len = %d, want %d", got, want)
	}
	assertASTNodeShape(t, "Container.Uses[0]", mod.Container[0].Uses[0], "uses", "local-group", "compat-metadata-demo.yang:73:9", mod.Container[0])
	assertScalarNodeShape(t, "Container.Uses[0].Description", mod.Container[0].Uses[0].Description, "description", "Local group use.", "compat-metadata-demo.yang:74:13", mod.Container[0].Uses[0])
	assertScalarNodeShape(t, "Container.Uses[0].Reference", mod.Container[0].Uses[0].Reference, "reference", "Local group reference.", "compat-metadata-demo.yang:75:13", mod.Container[0].Uses[0])
	if got, want := len(mod.Container[0].Uses[0].Refine), 1; got != want {
		t.Fatalf("Container.Uses[0].Refine len = %d, want %d", got, want)
	}
	assertASTNodeShape(t, "Container.Uses[0].Refine[0]", mod.Container[0].Uses[0].Refine[0], "refine", "local-group-leaf", "compat-metadata-demo.yang:76:13", mod.Container[0].Uses[0])
	assertScalarNodeShape(t, "Container.Uses[0].Refine[0].Default", mod.Container[0].Uses[0].Refine[0].Default, "default", "refined", "compat-metadata-demo.yang:77:17", mod.Container[0].Uses[0].Refine[0])
	assertScalarNodeShape(t, "Container.Uses[0].Refine[0].Description", mod.Container[0].Uses[0].Refine[0].Description, "description", "Refined grouping leaf.", "compat-metadata-demo.yang:78:17", mod.Container[0].Uses[0].Refine[0])
	assertASTNodeShape(t, "Container.Uses[1]", mod.Container[0].Uses[1], "uses", "sub-group", "compat-metadata-demo.yang:81:9", mod.Container[0])
	if got, want := len(mod.Container[0].Leaf), 2; got != want {
		t.Fatalf("Container.Leaf len = %d, want %d", got, want)
	}
	assertTypeNodeShape(t, "Container.Leaf[0].Type", mod.Container[0].Leaf[0].Type, "cmi:imported-type", "compat-metadata-demo.yang:82:25", mod.Container[0].Leaf[0])
	assertTypeNodeShape(t, "Container.Leaf[1].Type", mod.Container[0].Leaf[1].Type, "local-type", "compat-metadata-demo.yang:83:22", mod.Container[0].Leaf[1])
	if len(mod.RPC) != 1 || mod.RPC[0].Name != "reset" || mod.RPC[0].Input == nil || mod.RPC[0].Output == nil {
		t.Fatalf("module RPC = %#v, want reset input/output", mod.RPC)
	}
	assertASTNodeShape(t, "RPC.Input", mod.RPC[0].Input, "input", "", "compat-metadata-demo.yang:86:9", mod.RPC[0])
	if got, want := len(mod.RPC[0].Input.Leaf), 1; got != want {
		t.Fatalf("RPC.Input.Leaf len = %d, want %d", got, want)
	}
	assertTypeNodeShape(t, "RPC.Input.Leaf[0].Type", mod.RPC[0].Input.Leaf[0].Type, "string", "compat-metadata-demo.yang:87:28", mod.RPC[0].Input.Leaf[0])
	assertASTNodeShape(t, "RPC.Output", mod.RPC[0].Output, "output", "", "compat-metadata-demo.yang:89:9", mod.RPC[0])
	if got, want := len(mod.RPC[0].Output.Leaf), 1; got != want {
		t.Fatalf("RPC.Output.Leaf len = %d, want %d", got, want)
	}
	assertTypeNodeShape(t, "RPC.Output.Leaf[0].Type", mod.RPC[0].Output.Leaf[0].Type, "string", "compat-metadata-demo.yang:90:29", mod.RPC[0].Output.Leaf[0])
	if len(mod.Notification) != 1 || mod.Notification[0].Name != "alarm" {
		t.Fatalf("module Notification = %#v, want alarm", mod.Notification)
	}
	assertASTNodeShape(t, "Notification", mod.Notification[0], "notification", "alarm", "compat-metadata-demo.yang:93:5", mod)
	if got, want := len(mod.Notification[0].Leaf), 1; got != want {
		t.Fatalf("Notification.Leaf len = %d, want %d", got, want)
	}
	assertTypeNodeShape(t, "Notification.Leaf[0].Type", mod.Notification[0].Leaf[0].Type, "string", "compat-metadata-demo.yang:94:25", mod.Notification[0].Leaf[0])
}

func TestModulesFacadeSearchPathAndNamespaceLookup(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "compat-search-demo.yang")
	source := `module compat-search-demo {
    namespace "urn:compat-search-demo";
    prefix csd;

    container z-top { leaf zed { type string; } }
    container a-top { leaf alpha { type string; } }
}
`
	if err := os.WriteFile(path, []byte(source), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	ms := compat.NewModules()
	ms.AddPath(dir+string(os.PathListSeparator)+dir, dir)
	if got, want := ms.Path, []string{dir}; !reflect.DeepEqual(got, want) {
		t.Fatalf("Path = %v, want duplicate-suppressed %v", got, want)
	}

	entry, errs := ms.GetModule("compat-search-demo")
	if len(errs) != 0 {
		t.Fatalf("GetModule errors = %v, want none", errs)
	}
	if got, want := childNames(entry.Children()), []string{"z-top", "a-top"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("entry children = %v, want %v", got, want)
	}

	mod, err := ms.FindModuleByNamespace("urn:compat-search-demo")
	if err != nil {
		t.Fatalf("FindModuleByNamespace: %v", err)
	}
	if mod == nil || mod.Name != "compat-search-demo" {
		t.Fatalf("FindModuleByNamespace module = %#v, want compat-search-demo", mod)
	}
	if mod.Namespace == nil || mod.Namespace.Name != "urn:compat-search-demo" {
		t.Fatalf("module Namespace = %#v, want urn:compat-search-demo", mod.Namespace)
	}
	if mod.Prefix == nil || mod.Prefix.Name != "csd" {
		t.Fatalf("module Prefix = %#v, want csd", mod.Prefix)
	}
	if fromNode := compat.ToEntry(mod); fromNode != entry {
		t.Fatalf("ToEntry(FindModuleByNamespace module) = %p, want cached entry %p", fromNode, entry)
	}
	if _, err := ms.FindModuleByNamespace("urn:missing"); err == nil {
		t.Fatal("FindModuleByNamespace missing namespace returned nil error")
	}
}

func TestModulesFindModuleByNamespaceMatchesGoyangBeforeProcess(t *testing.T) {
	source := `module compat-namespace-before-process {
    namespace "urn:compat-namespace-before-process";
    prefix cnbp;

    import compat-namespace-missing-import {
        prefix cnmi;
    }

    container top {
        leaf value { type string; }
    }
}
`

	compatModules := compat.NewModules()
	if err := compatModules.Parse(source, "compat-namespace-before-process.yang"); err != nil {
		t.Fatalf("compat Parse: %v", err)
	}
	upstreamModules := upstream.NewModules()
	if err := upstreamModules.Parse(source, "compat-namespace-before-process.yang"); err != nil {
		t.Fatalf("upstream Parse: %v", err)
	}

	compatModule, compatErr := compatModules.FindModuleByNamespace("urn:compat-namespace-before-process")
	upstreamModule, upstreamErr := upstreamModules.FindModuleByNamespace("urn:compat-namespace-before-process")
	if compatErr != nil || upstreamErr != nil {
		t.Fatalf("FindModuleByNamespace errors = (%v,%v), want both nil", compatErr, upstreamErr)
	}
	if compatModule == nil || upstreamModule == nil {
		t.Fatalf("FindModuleByNamespace modules = (%#v,%#v), want both non-nil", compatModule, upstreamModule)
	}
	if compatModule.Name != upstreamModule.Name {
		t.Fatalf("FindModuleByNamespace Name = %q, want goyang %q", compatModule.Name, upstreamModule.Name)
	}

	padded := " urn:compat-namespace-before-process "
	compatPadded, compatPaddedErr := compatModules.FindModuleByNamespace(padded)
	upstreamPadded, upstreamPaddedErr := upstreamModules.FindModuleByNamespace(padded)
	if (compatPaddedErr == nil) != (upstreamPaddedErr == nil) {
		t.Fatalf("FindModuleByNamespace(%q) errors = (%v,%v), want goyang parity", padded, compatPaddedErr, upstreamPaddedErr)
	}
	if (compatPadded == nil) != (upstreamPadded == nil) {
		t.Fatalf("FindModuleByNamespace(%q) modules = (%#v,%#v), want goyang parity", padded, compatPadded, upstreamPadded)
	}

	missingNamespace := "urn:compat-namespace-not-loaded"
	compatMissing, compatMissingErr := compatModules.FindModuleByNamespace(missingNamespace)
	upstreamMissing, upstreamMissingErr := upstreamModules.FindModuleByNamespace(missingNamespace)
	if (compatMissingErr == nil) != (upstreamMissingErr == nil) {
		t.Fatalf("FindModuleByNamespace(%q) errors = (%v,%v), want goyang parity", missingNamespace, compatMissingErr, upstreamMissingErr)
	}
	if (compatMissing == nil) != (upstreamMissing == nil) {
		t.Fatalf("FindModuleByNamespace(%q) modules = (%#v,%#v), want goyang parity", missingNamespace, compatMissing, upstreamMissing)
	}
	if compatMissingErr != nil && upstreamMissingErr != nil && compatMissingErr.Error() != upstreamMissingErr.Error() {
		t.Fatalf("FindModuleByNamespace(%q) error = %q, want goyang %q", missingNamespace, compatMissingErr.Error(), upstreamMissingErr.Error())
	}
}

func TestModulesFacadeRecursiveSearchPath(t *testing.T) {
	root := t.TempDir()
	nested := filepath.Join(root, "nested", "yang")
	if err := os.MkdirAll(nested, 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	path := filepath.Join(nested, "compat-recursive-demo.yang")
	source := `module compat-recursive-demo {
    namespace "urn:compat-recursive-demo";
    prefix crd;

    container found { leaf value { type string; } }
}
`
	if err := os.WriteFile(path, []byte(source), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	ms := compat.NewModules()
	ms.AddPath(filepath.Join(root, "..."))
	entry, errs := ms.GetModule("compat-recursive-demo")
	if len(errs) != 0 {
		t.Fatalf("GetModule recursive errors = %v, want none", errs)
	}
	if entry == nil || entry.Lookup("found") == nil {
		t.Fatalf("GetModule recursive entry = %#v, want found child", entry)
	}
}

func TestModulesFindModuleImportIncludeAndRevision(t *testing.T) {
	dir := t.TempDir()
	writeYANG(t, filepath.Join(dir, "compat-import-target@2024-01-01.yang"), `module compat-import-target {
    namespace "urn:compat-import-target:old";
    prefix cit;
    revision 2024-01-01;
}
`)
	writeYANG(t, filepath.Join(dir, "compat-import-target@2025-01-01.yang"), `module compat-import-target {
    namespace "urn:compat-import-target:new";
    prefix cit;
    revision 2025-01-01;
}
`)
	writeYANG(t, filepath.Join(dir, "compat-sub-target@2024-02-03.yang"), `submodule compat-sub-target {
    belongs-to compat-owner { prefix co; }
    revision 2024-02-03;
}
`)

	ms := compat.NewModules()
	ms.AddPath(dir)

	latest := ms.FindModule(&compat.Import{Name: "compat-import-target"})
	if latest == nil {
		t.Fatal("FindModule(import latest) returned nil")
	}
	if got, want := latest.Name, "compat-import-target"; got != want {
		t.Fatalf("latest Name = %q, want %q", got, want)
	}
	if got, want := latest.Current(), "2025-01-01"; got != want {
		t.Fatalf("latest Current() = %q, want %q", got, want)
	}
	if got, want := latest.FullName(), "compat-import-target@2025-01-01"; got != want {
		t.Fatalf("latest FullName() = %q, want %q", got, want)
	}
	if got, want := latest.GetPrefix(), "cit"; got != want {
		t.Fatalf("latest GetPrefix() = %q, want %q", got, want)
	}
	if ms.Modules[latest.FullName()] != latest {
		t.Fatalf("Modules[%q] does not point at latest record", latest.FullName())
	}

	old := ms.FindModule(&compat.Import{
		Name:         "compat-import-target",
		RevisionDate: &compat.ASTValue{Name: "2024-01-01"},
	})
	if old == nil {
		t.Fatal("FindModule(import revision-date) returned nil")
	}
	if got, want := old.Current(), "2024-01-01"; got != want {
		t.Fatalf("old Current() = %q, want %q", got, want)
	}
	if old == latest {
		t.Fatal("revision-date import returned latest module")
	}
	if ms.Modules["compat-import-target"] != latest {
		t.Fatal("bare module key should keep latest revision")
	}

	sub := ms.FindModule(&compat.Include{
		Name:         "compat-sub-target",
		RevisionDate: &compat.ASTValue{Name: "2024-02-03"},
	})
	if sub == nil {
		t.Fatal("FindModule(include revision-date) returned nil")
	}
	if got, want := sub.FullName(), "compat-sub-target@2024-02-03"; got != want {
		t.Fatalf("submodule FullName() = %q, want %q", got, want)
	}
	if ms.SubModules[sub.FullName()] != sub {
		t.Fatalf("SubModules[%q] does not point at submodule record", sub.FullName())
	}

	if got := ms.FindModule(&compat.Leaf{Name: "not-import-or-include"}); got != nil {
		t.Fatalf("FindModule(non import/include) = %#v, want nil", got)
	}
}

func TestModulesFindModuleRevisionFallbackMatchesGoyang(t *testing.T) {
	dir := t.TempDir()
	writeYANG(t, filepath.Join(dir, "compat-revision-fallback.yang"), `module compat-revision-fallback {
    namespace "urn:compat-revision-fallback";
    prefix crf;
    revision 2025-01-01;
}
`)
	writeYANG(t, filepath.Join(dir, "compat-sub-fallback.yang"), `submodule compat-sub-fallback {
    belongs-to compat-revision-owner { prefix cro; }
    revision 2025-02-03;
}
`)

	compatModules := compat.NewModules()
	compatModules.AddPath(dir)
	upstreamModules := upstream.NewModules()
	upstreamModules.AddPath(dir)

	compatImport := compatModules.FindModule(&compat.Import{
		Name:         "compat-revision-fallback",
		RevisionDate: &compat.ASTValue{Name: "2024-01-01"},
	})
	upstreamImport := upstreamModules.FindModule(&upstream.Import{
		Name:         "compat-revision-fallback",
		RevisionDate: &upstream.Value{Name: "2024-01-01"},
	})
	assertFindModuleRevisionFallback(t, "import", compatImport, upstreamImport)

	compatInclude := compatModules.FindModule(&compat.Include{
		Name:         "compat-sub-fallback",
		RevisionDate: &compat.ASTValue{Name: "2024-02-03"},
	})
	upstreamInclude := upstreamModules.FindModule(&upstream.Include{
		Name:         "compat-sub-fallback",
		RevisionDate: &upstream.Value{Name: "2024-02-03"},
	})
	assertFindModuleRevisionFallback(t, "include", compatInclude, upstreamInclude)
}

func TestModulesFacadeListActionMetadata(t *testing.T) {
	source := `module compat-list-action-demo {
    yang-version 1.1;
    namespace "urn:compat-list-action-demo";
    prefix clad;

    feature maintenance;

    list item {
        key "name";
        unique "tag";
        must "string-length(name) > 0" {
            error-message "name required";
        }
        leaf name { type string; }
        leaf tag { type string; }
        action reset {
            description "Reset one item.";
            if-feature maintenance;
            typedef reset-token { type string; }
            grouping reset-input {
                leaf reason { type string; }
            }
            input {
                uses reset-input;
            }
            output {
                leaf result { type string; }
            }
        }
        notification item-event {
            leaf severity { type string; }
        }
    }
}
`
	ms := compat.NewModules()
	if err := ms.Parse(source, "compat-list-action-demo.yang"); err != nil {
		t.Fatalf("Parse: %v", err)
	}
	mod := ms.Modules["compat-list-action-demo"]
	if mod == nil {
		t.Fatal("module record not found after Parse")
	}
	if got, want := len(mod.List), 1; got != want {
		t.Fatalf("Module.List len = %d, want %d", got, want)
	}
	list := mod.List[0]
	assertASTNodeShape(t, "List", list, "list", "item", "compat-list-action-demo.yang:8:5", mod)
	if got, want := len(list.Unique), 1; got != want {
		t.Fatalf("List.Unique len = %d, want %d", got, want)
	}
	assertScalarNodeShape(t, "List.Unique[0]", list.Unique[0], "unique", "tag", "compat-list-action-demo.yang:10:9", list)
	if got, want := len(list.Must), 1; got != want {
		t.Fatalf("List.Must len = %d, want %d", got, want)
	}
	assertASTNodeShape(t, "List.Must[0]", list.Must[0], "must", "string-length(name) > 0", "compat-list-action-demo.yang:11:9", list)
	assertScalarNodeShape(t, "List.Must[0].ErrorMessage", list.Must[0].ErrorMessage, "error-message", "name required", "compat-list-action-demo.yang:12:13", list.Must[0])
	if got, want := len(list.Action), 1; got != want {
		t.Fatalf("List.Action len = %d, want %d", got, want)
	}
	action := list.Action[0]
	assertASTNodeShape(t, "List.Action[0]", action, "action", "reset", "compat-list-action-demo.yang:16:9", list)
	assertScalarNodeShape(t, "List.Action[0].Description", action.Description, "description", "Reset one item.", "compat-list-action-demo.yang:17:13", action)
	if got, want := len(action.IfFeature), 1; got != want {
		t.Fatalf("List.Action[0].IfFeature len = %d, want %d", got, want)
	}
	assertScalarNodeShape(t, "List.Action[0].IfFeature[0]", action.IfFeature[0], "if-feature", "maintenance", "compat-list-action-demo.yang:18:13", action)
	if got, want := len(action.Typedef), 1; got != want {
		t.Fatalf("List.Action[0].Typedef len = %d, want %d", got, want)
	}
	assertASTNodeShape(t, "List.Action[0].Typedef[0]", action.Typedef[0], "typedef", "reset-token", "compat-list-action-demo.yang:19:13", action)
	assertTypeNodeShape(t, "List.Action[0].Typedef[0].Type", action.Typedef[0].Type, "string", "compat-list-action-demo.yang:19:35", action.Typedef[0])
	if got, want := len(action.Grouping), 1; got != want {
		t.Fatalf("List.Action[0].Grouping len = %d, want %d", got, want)
	}
	assertASTNodeShape(t, "List.Action[0].Grouping[0]", action.Grouping[0], "grouping", "reset-input", "compat-list-action-demo.yang:20:13", action)
	if got, want := len(action.Grouping[0].Leaf), 1; got != want {
		t.Fatalf("List.Action[0].Grouping[0].Leaf len = %d, want %d", got, want)
	}
	assertTypeNodeShape(t, "List.Action[0].Grouping[0].Leaf[0].Type", action.Grouping[0].Leaf[0].Type, "string", "compat-list-action-demo.yang:21:31", action.Grouping[0].Leaf[0])
	assertASTNodeShape(t, "List.Action[0].Input", action.Input, "input", "", "compat-list-action-demo.yang:23:13", action)
	if got, want := len(action.Input.Uses), 1; got != want {
		t.Fatalf("List.Action[0].Input.Uses len = %d, want %d", got, want)
	}
	assertASTNodeShape(t, "List.Action[0].Input.Uses[0]", action.Input.Uses[0], "uses", "reset-input", "compat-list-action-demo.yang:24:17", action.Input)
	assertASTNodeShape(t, "List.Action[0].Output", action.Output, "output", "", "compat-list-action-demo.yang:26:13", action)
	if got, want := len(action.Output.Leaf), 1; got != want {
		t.Fatalf("List.Action[0].Output.Leaf len = %d, want %d", got, want)
	}
	assertTypeNodeShape(t, "List.Action[0].Output.Leaf[0].Type", action.Output.Leaf[0].Type, "string", "compat-list-action-demo.yang:27:31", action.Output.Leaf[0])
	if got, want := len(list.Notification), 1; got != want {
		t.Fatalf("List.Notification len = %d, want %d", got, want)
	}
	assertASTNodeShape(t, "List.Notification[0]", list.Notification[0], "notification", "item-event", "compat-list-action-demo.yang:30:9", list)
	if got, want := len(list.Notification[0].Leaf), 1; got != want {
		t.Fatalf("List.Notification[0].Leaf len = %d, want %d", got, want)
	}
	assertTypeNodeShape(t, "List.Notification[0].Leaf[0].Type", list.Notification[0].Leaf[0].Type, "string", "compat-list-action-demo.yang:31:29", list.Notification[0].Leaf[0])
}

func TestModulesFacadeDefinitionMetadataSources(t *testing.T) {
	source := `module compat-definition-metadata {
    yang-version 1.1;
    namespace "urn:compat-definition-metadata";
    prefix cdm;

    extension local-ext {
        argument value {
            yin-element true;
        }
        description "Extension description.";
        reference "Extension reference.";
        status current;
    }

    feature other-mode;
    feature fast-mode {
        if-feature other-mode;
        description "Fast mode.";
        reference "Feature reference.";
        status current;
    }

    deviation "/target" {
        description "Deviation description.";
        reference "Deviation reference.";
        deviate replace {
            type string;
            default "fallback";
            unique "id";
            must "id" {
                error-message "id required";
            }
            units "widgets";
        }
    }

    container target {
        leaf id { type string; }
    }
}
`
	subSource := `submodule compat-definition-sub {
    belongs-to compat-definition-metadata {
        prefix cdm;
    }
}
`
	ms := compat.NewModules()
	if err := ms.Parse(source, "compat-definition-metadata.yang"); err != nil {
		t.Fatalf("Parse module: %v", err)
	}
	if err := ms.Parse(subSource, "compat-definition-sub.yang"); err != nil {
		t.Fatalf("Parse submodule: %v", err)
	}
	mod := ms.Modules["compat-definition-metadata"]
	if mod == nil {
		t.Fatal("module record not found after Parse")
	}
	if got, want := len(mod.Extension), 1; got != want {
		t.Fatalf("Module.Extension len = %d, want %d", got, want)
	}
	ext := mod.Extension[0]
	assertASTNodeShape(t, "Extension", ext, "extension", "local-ext", "compat-definition-metadata.yang:6:5", mod)
	assertASTNodeShape(t, "Extension.Argument", ext.Argument, "argument", "value", "compat-definition-metadata.yang:7:9", ext)
	assertScalarNodeShape(t, "Extension.Argument.YinElement", ext.Argument.YinElement, "yin-element", "true", "compat-definition-metadata.yang:8:13", ext.Argument)
	assertScalarNodeShape(t, "Extension.Description", ext.Description, "description", "Extension description.", "compat-definition-metadata.yang:10:9", ext)
	assertScalarNodeShape(t, "Extension.Reference", ext.Reference, "reference", "Extension reference.", "compat-definition-metadata.yang:11:9", ext)
	assertScalarNodeShape(t, "Extension.Status", ext.Status, "status", "current", "compat-definition-metadata.yang:12:9", ext)

	if got, want := len(mod.Feature), 2; got != want {
		t.Fatalf("Module.Feature len = %d, want %d", got, want)
	}
	feature := mod.Feature[1]
	assertASTNodeShape(t, "Feature", feature, "feature", "fast-mode", "compat-definition-metadata.yang:16:5", mod)
	if got, want := len(feature.IfFeature), 1; got != want {
		t.Fatalf("Feature.IfFeature len = %d, want %d", got, want)
	}
	assertScalarNodeShape(t, "Feature.IfFeature[0]", feature.IfFeature[0], "if-feature", "other-mode", "compat-definition-metadata.yang:17:9", feature)
	assertScalarNodeShape(t, "Feature.Description", feature.Description, "description", "Fast mode.", "compat-definition-metadata.yang:18:9", feature)
	assertScalarNodeShape(t, "Feature.Reference", feature.Reference, "reference", "Feature reference.", "compat-definition-metadata.yang:19:9", feature)
	assertScalarNodeShape(t, "Feature.Status", feature.Status, "status", "current", "compat-definition-metadata.yang:20:9", feature)

	if got, want := len(mod.Deviation), 1; got != want {
		t.Fatalf("Module.Deviation len = %d, want %d", got, want)
	}
	deviation := mod.Deviation[0]
	assertASTNodeShape(t, "Deviation", deviation, "deviation", "/target", "compat-definition-metadata.yang:23:5", mod)
	assertScalarNodeShape(t, "Deviation.Description", deviation.Description, "description", "Deviation description.", "compat-definition-metadata.yang:24:9", deviation)
	assertScalarNodeShape(t, "Deviation.Reference", deviation.Reference, "reference", "Deviation reference.", "compat-definition-metadata.yang:25:9", deviation)
	if got, want := len(deviation.Deviate), 1; got != want {
		t.Fatalf("Deviation.Deviate len = %d, want %d", got, want)
	}
	deviate := deviation.Deviate[0]
	assertASTNodeShape(t, "Deviation.Deviate[0]", deviate, "deviate", "replace", "compat-definition-metadata.yang:26:9", deviation)
	assertTypeNodeShape(t, "Deviation.Deviate[0].Type", deviate.Type, "string", "compat-definition-metadata.yang:27:13", deviate)
	assertScalarNodeShape(t, "Deviation.Deviate[0].Default", deviate.Default, "default", "fallback", "compat-definition-metadata.yang:28:13", deviate)
	if got, want := len(deviate.Unique), 1; got != want {
		t.Fatalf("Deviation.Deviate[0].Unique len = %d, want %d", got, want)
	}
	assertScalarNodeShape(t, "Deviation.Deviate[0].Unique[0]", deviate.Unique[0], "unique", "id", "compat-definition-metadata.yang:29:13", deviate)
	if got, want := len(deviate.Must), 1; got != want {
		t.Fatalf("Deviation.Deviate[0].Must len = %d, want %d", got, want)
	}
	assertASTNodeShape(t, "Deviation.Deviate[0].Must[0]", deviate.Must[0], "must", "id", "compat-definition-metadata.yang:30:13", deviate)
	assertScalarNodeShape(t, "Deviation.Deviate[0].Must[0].ErrorMessage", deviate.Must[0].ErrorMessage, "error-message", "id required", "compat-definition-metadata.yang:31:17", deviate.Must[0])
	assertScalarNodeShape(t, "Deviation.Deviate[0].Units", deviate.Units, "units", "widgets", "compat-definition-metadata.yang:33:13", deviate)

	sub := ms.SubModules["compat-definition-sub"]
	if sub == nil || sub.BelongsTo == nil {
		t.Fatalf("submodule BelongsTo = %#v, want compat-definition-metadata", sub)
	}
	assertASTNodeShape(t, "BelongsTo", sub.BelongsTo, "belongs-to", "compat-definition-metadata", "compat-definition-sub.yang:2:5", sub)
	assertScalarNodeShape(t, "BelongsTo.Prefix", sub.BelongsTo.Prefix, "prefix", "cdm", "compat-definition-sub.yang:3:9", sub.BelongsTo)
}

func TestModulesFacadeConstraintChildMetadata(t *testing.T) {
	source := `module compat-node-constraints {
    yang-version 1.1;
    namespace "urn:compat-node-constraints";
    prefix cnc;

    feature gate;

    anydata payload {
        if-feature gate;
        must "ready" {
            error-message "ready required";
        }
    }

    anyxml legacy {
        if-feature gate;
        must "legacy" {
            error-message "legacy required";
        }
    }

    leaf scalar {
        type string;
        must ". != ''" {
            error-message "scalar required";
        }
    }

    leaf-list tags {
        type string;
        if-feature gate;
        must ". != ''" {
            error-message "tag required";
        }
    }
}
`
	ms := compat.NewModules()
	if err := ms.Parse(source, "compat-node-constraints.yang"); err != nil {
		t.Fatalf("Parse: %v", err)
	}
	mod := ms.Modules["compat-node-constraints"]
	if mod == nil {
		t.Fatal("module record not found after Parse")
	}

	if got, want := len(mod.Anydata), 1; got != want {
		t.Fatalf("Module.Anydata len = %d, want %d", got, want)
	}
	anydata := mod.Anydata[0]
	if got, want := len(anydata.IfFeature), 1; got != want {
		t.Fatalf("Anydata.IfFeature len = %d, want %d", got, want)
	}
	assertScalarNodeShape(t, "Anydata.IfFeature[0]", anydata.IfFeature[0], "if-feature", "gate", "compat-node-constraints.yang:9:9", anydata)
	if got, want := len(anydata.Must), 1; got != want {
		t.Fatalf("Anydata.Must len = %d, want %d", got, want)
	}
	assertASTNodeShape(t, "Anydata.Must[0]", anydata.Must[0], "must", "ready", "compat-node-constraints.yang:10:9", anydata)
	assertScalarNodeShape(t, "Anydata.Must[0].ErrorMessage", anydata.Must[0].ErrorMessage, "error-message", "ready required", "compat-node-constraints.yang:11:13", anydata.Must[0])

	if got, want := len(mod.Anyxml), 1; got != want {
		t.Fatalf("Module.Anyxml len = %d, want %d", got, want)
	}
	anyxml := mod.Anyxml[0]
	if got, want := len(anyxml.IfFeature), 1; got != want {
		t.Fatalf("Anyxml.IfFeature len = %d, want %d", got, want)
	}
	assertScalarNodeShape(t, "Anyxml.IfFeature[0]", anyxml.IfFeature[0], "if-feature", "gate", "compat-node-constraints.yang:16:9", anyxml)
	if got, want := len(anyxml.Must), 1; got != want {
		t.Fatalf("Anyxml.Must len = %d, want %d", got, want)
	}
	assertASTNodeShape(t, "Anyxml.Must[0]", anyxml.Must[0], "must", "legacy", "compat-node-constraints.yang:17:9", anyxml)
	assertScalarNodeShape(t, "Anyxml.Must[0].ErrorMessage", anyxml.Must[0].ErrorMessage, "error-message", "legacy required", "compat-node-constraints.yang:18:13", anyxml.Must[0])

	if got, want := len(mod.Leaf), 1; got != want {
		t.Fatalf("Module.Leaf len = %d, want %d", got, want)
	}
	leaf := mod.Leaf[0]
	if got, want := len(leaf.Must), 1; got != want {
		t.Fatalf("Leaf.Must len = %d, want %d", got, want)
	}
	assertASTNodeShape(t, "Leaf.Must[0]", leaf.Must[0], "must", ". != ''", "compat-node-constraints.yang:24:9", leaf)
	assertScalarNodeShape(t, "Leaf.Must[0].ErrorMessage", leaf.Must[0].ErrorMessage, "error-message", "scalar required", "compat-node-constraints.yang:25:13", leaf.Must[0])

	if got, want := len(mod.LeafList), 1; got != want {
		t.Fatalf("Module.LeafList len = %d, want %d", got, want)
	}
	leafList := mod.LeafList[0]
	if got, want := len(leafList.IfFeature), 1; got != want {
		t.Fatalf("LeafList.IfFeature len = %d, want %d", got, want)
	}
	assertScalarNodeShape(t, "LeafList.IfFeature[0]", leafList.IfFeature[0], "if-feature", "gate", "compat-node-constraints.yang:31:9", leafList)
	if got, want := len(leafList.Must), 1; got != want {
		t.Fatalf("LeafList.Must len = %d, want %d", got, want)
	}
	assertASTNodeShape(t, "LeafList.Must[0]", leafList.Must[0], "must", ". != ''", "compat-node-constraints.yang:32:9", leafList)
	assertScalarNodeShape(t, "LeafList.Must[0].ErrorMessage", leafList.Must[0].ErrorMessage, "error-message", "tag required", "compat-node-constraints.yang:33:13", leafList.Must[0])
}

func TestModulesFacadeIdentityMetadataSources(t *testing.T) {
	source := `module compat-identity-metadata {
    namespace "urn:compat-identity-metadata";
    prefix cim;

    feature gate;

    identity base-id;
    identity local-id {
        base base-id;
        if-feature gate;
        description "Local identity.";
        reference "Identity reference.";
        status current;
    }
}
`
	ms := compat.NewModules()
	if err := ms.Parse(source, "compat-identity-metadata.yang"); err != nil {
		t.Fatalf("Parse: %v", err)
	}
	mod := ms.Modules["compat-identity-metadata"]
	if mod == nil {
		t.Fatal("module record not found after Parse")
	}
	if got, want := len(mod.Identity), 2; got != want {
		t.Fatalf("Module.Identity len = %d, want %d", got, want)
	}
	identity := mod.Identity[1]
	assertASTNodeShape(t, "Identity", identity, "identity", "local-id", "compat-identity-metadata.yang:8:5", mod)
	if got, want := len(identity.Base), 1; got != want {
		t.Fatalf("Identity.Base len = %d, want %d", got, want)
	}
	assertScalarNodeShape(t, "Identity.Base[0]", identity.Base[0], "base", "base-id", "compat-identity-metadata.yang:9:9", identity)
	if got, want := len(identity.IfFeature), 1; got != want {
		t.Fatalf("Identity.IfFeature len = %d, want %d", got, want)
	}
	assertScalarNodeShape(t, "Identity.IfFeature[0]", identity.IfFeature[0], "if-feature", "gate", "compat-identity-metadata.yang:10:9", identity)
	assertScalarNodeShape(t, "Identity.Description", identity.Description, "description", "Local identity.", "compat-identity-metadata.yang:11:9", identity)
	assertScalarNodeShape(t, "Identity.Reference", identity.Reference, "reference", "Identity reference.", "compat-identity-metadata.yang:12:9", identity)
	assertScalarNodeShape(t, "Identity.Status", identity.Status, "status", "current", "compat-identity-metadata.yang:13:9", identity)
}

func assertFindModuleRevisionFallback(t *testing.T, label string, got *compat.Module, want *upstream.Module) {
	t.Helper()
	if got == nil || want == nil {
		t.Fatalf("%s FindModule fallback = (%#v,%#v), want both non-nil", label, got, want)
	}
	if got.Name != want.Name {
		t.Fatalf("%s fallback Name = %q, want goyang %q", label, got.Name, want.Name)
	}
	if got.Current() != want.Current() {
		t.Fatalf("%s fallback Current() = %q, want goyang %q", label, got.Current(), want.Current())
	}
	if got.FullName() != want.FullName() {
		t.Fatalf("%s fallback FullName() = %q, want goyang %q", label, got.FullName(), want.FullName())
	}
}

func writeYANG(t *testing.T, path, source string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(source), 0o600); err != nil {
		t.Fatalf("WriteFile(%s): %v", path, err)
	}
}
