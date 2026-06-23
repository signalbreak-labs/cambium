// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

package cambium_test

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/signalbreak-labs/cambium/go/cambium"
)

func loadDownstreamContext(t *testing.T, sources ...string) *cambium.Context {
	t.Helper()
	builder, err := cambium.NewContextBuilder(cambium.ContextFlags{})
	if err != nil {
		t.Fatalf("NewContextBuilder: %v", err)
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

func childNamesForProfile(children cambium.SchemaChildren) []string {
	var out []string
	for child := range children.Iter() {
		out = append(out, child.Name())
	}
	return out
}

func TestTraversalProfilesUseSchemaTermsAndStableOrder(t *testing.T) {
	source := `module downstream-traversal {
    yang-version 1.1;
    namespace "urn:downstream-traversal";
    prefix dt;

    container top {
        list iface {
            key "name vrf";
            leaf description { type string; }
            leaf name { type string; }
            choice address-family {
                case v4 {
                    leaf ipv4 { type string; }
                }
                case v6 {
                    leaf ipv6 { type string; }
                }
            }
            leaf vrf { type string; }
            leaf tail { type string; }
        }
    }
}`
	ctx := loadDownstreamContext(t, source)
	mod, err := ctx.Schema("downstream-traversal")
	if err != nil {
		t.Fatalf("Schema: %v", err)
	}
	iface := schemaNodeAt(t, mod, "/dt:top/iface")

	if got, want := childNamesForProfile(iface.Traverse(cambium.TraversalStructuralChildren)), []string{"description", "name", "address-family", "vrf", "tail"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("structural children = %v, want %v", got, want)
	}
	if got, want := childNamesForProfile(iface.Traverse(cambium.TraversalDataChildren)), []string{"description", "name", "ipv4", "ipv6", "vrf", "tail"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("data children = %v, want %v", got, want)
	}
	if got, want := childNamesForProfile(iface.Traverse(cambium.TraversalListEntryOrder)), []string{"name", "vrf", "description", "ipv4", "ipv6", "tail"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("list-entry order = %v, want %v", got, want)
	}
	if got, want := childNamesForProfile(iface.Traverse(cambium.TraversalSerializationOrder)), []string{"name", "vrf", "description", "ipv4", "ipv6", "tail"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("serialization order = %v, want %v", got, want)
	}
}

func TestSchemaIRV1IncludesOrderedNodesAndProvenance(t *testing.T) {
	source := `module downstream-ir {
    namespace "urn:downstream-ir";
    prefix di;

    grouping reusable {
        leaf grouped { type string; }
    }

    container top {
        leaf before { type string; }
        uses reusable;
        leaf after {
            config false;
            type uint16;
            default "42";
        }
    }
}`
	ctx := loadDownstreamContext(t, source)

	ir := ctx.SchemaIR()
	if got, want := ir.Version, cambium.SchemaIRVersion; got != want {
		t.Fatalf("SchemaIR version = %q, want %q", got, want)
	}
	if len(ir.Modules) != 1 {
		t.Fatalf("SchemaIR modules = %d, want 1", len(ir.Modules))
	}
	module := ir.Modules[0]
	if module.Name != "downstream-ir" || module.Namespace != "urn:downstream-ir" {
		t.Fatalf("module projection = %#v", module)
	}
	if got, want := module.Source.File, "<memory>"; got != want {
		t.Fatalf("module source file = %q, want %q", got, want)
	}
	if got, want := schemaIRNodeNames(module.Children), []string{"top"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("module children = %v, want %v", got, want)
	}

	top := module.Children[0]
	if got, want := schemaIRNodeNames(top.Children), []string{"before", "grouped", "after"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("top children = %v, want %v", got, want)
	}
	after := top.Children[2]
	if after.LocalPath != "/top/after" || after.QualifiedPath != "/downstream-ir:top/downstream-ir:after" {
		t.Fatalf("after paths = local %q qualified %q", after.LocalPath, after.QualifiedPath)
	}
	if got, want := after.NamespaceQualifiedPath, "/{urn:downstream-ir}top/{urn:downstream-ir}after"; got != want {
		t.Fatalf("after namespace path = %q, want %q", got, want)
	}
	if got, want := after.Ref.NamespaceQualifiedPath(), "/{urn:downstream-ir}top/{urn:downstream-ir}after"; got != want {
		t.Fatalf("after ref namespace path = %q, want %q", got, want)
	}
	if after.Config != cambium.ConfigRo {
		t.Fatalf("after Config = %v, want ConfigRo", after.Config)
	}
	if got, want := defaultValues(after.Defaults), []string{"42"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("after defaults = %v, want %v", got, want)
	}
	if after.Provenance.DefiningModule != "downstream-ir" || after.Provenance.InstantiatingModule != "downstream-ir" {
		t.Fatalf("after provenance = %#v", after.Provenance)
	}
	grouped := top.Children[1]
	if grouped.Provenance.Grouping == "" {
		t.Fatalf("grouped provenance = %#v, want grouping origin", grouped.Provenance)
	}
}

func TestSchemaIRIncludesLoadedImportModules(t *testing.T) {
	imported := `module downstream-ir-import {
    namespace "urn:downstream-ir-import";
    prefix dii;

    typedef shared { type string; }
}`
	source := `module downstream-ir-import-user {
    namespace "urn:downstream-ir-import-user";
    prefix diu;

    import downstream-ir-import { prefix dii; }

    leaf value { type dii:shared; }
}`
	dir := t.TempDir()
	files := map[string]string{
		"downstream-ir-import.yang":      imported,
		"downstream-ir-import-user.yang": source,
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatalf("WriteFile %s: %v", name, err)
		}
	}
	builder, err := cambium.NewContextBuilder(cambium.ContextFlags{})
	if err != nil {
		t.Fatalf("NewContextBuilder: %v", err)
	}
	if err := builder.SearchPath(dir); err != nil {
		t.Fatalf("SearchPath: %v", err)
	}
	if err := builder.LoadModuleFromPath(filepath.Join(dir, "downstream-ir-import-user.yang")); err != nil {
		t.Fatalf("LoadModuleFromPath: %v", err)
	}
	ctx, err := builder.Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	t.Cleanup(func() { ctx.Close() })

	ir := ctx.SchemaIR()
	if got, want := schemaIRModuleNames(ir.Modules), []string{"downstream-ir-import-user", "downstream-ir-import"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("SchemaIR modules = %v, want %v", got, want)
	}
	if ir.Modules[1].Implemented {
		t.Fatalf("import module Implemented = true, want false")
	}
}

func schemaIRModuleNames(modules []cambium.SchemaIRModule) []string {
	out := make([]string, len(modules))
	for i, module := range modules {
		out[i] = module.Name
	}
	return out
}

func schemaIRNodeNames(nodes []cambium.SchemaIRNode) []string {
	out := make([]string, len(nodes))
	for i, node := range nodes {
		out[i] = node.Name
	}
	return out
}

func defaultValues(values []cambium.DefaultValue) []string {
	out := make([]string, len(values))
	for i, value := range values {
		out[i] = value.Value()
	}
	return out
}

func TestLoadReportExposesRequestedImportsIncludesAndFeatures(t *testing.T) {
	base := `module downstream-load-base {
    namespace "urn:downstream-load-base";
    prefix dlb;

    include downstream-load-sub;
    import downstream-load-import { prefix dli; }
    feature beta;

    leaf base-leaf {
        if-feature beta;
        type dli:shared;
    }
}`
	sub := `submodule downstream-load-sub {
    belongs-to downstream-load-base { prefix dlb; }
    leaf sub-leaf { type string; }
}`
	imp := `module downstream-load-import {
    namespace "urn:downstream-load-import";
    prefix dli;

    typedef shared { type string; }
}`
	builder, err := cambium.NewContextBuilder(cambium.ContextFlags{})
	if err != nil {
		t.Fatalf("NewContextBuilder: %v", err)
	}
	if err := builder.SetFeatures("downstream-load-base", []string{"beta"}); err != nil {
		t.Fatalf("SetFeatures: %v", err)
	}
	dir := t.TempDir()
	files := map[string]string{
		"downstream-load-base.yang":   base,
		"downstream-load-sub.yang":    sub,
		"downstream-load-import.yang": imp,
	}
	for name, source := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(source), 0o644); err != nil {
			t.Fatalf("WriteFile %s: %v", name, err)
		}
	}
	if err := builder.SearchPath(dir); err != nil {
		t.Fatalf("SearchPath: %v", err)
	}
	if err := builder.LoadModuleFromPath(filepath.Join(dir, "downstream-load-base.yang")); err != nil {
		t.Fatalf("LoadModuleFromPath: %v", err)
	}
	ctx, err := builder.Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	t.Cleanup(func() { ctx.Close() })

	report := ctx.LoadReport()
	if got, want := moduleInfoNames(report.RequestedModules), []string{"downstream-load-base"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("requested modules = %v, want %v", got, want)
	}
	if got, want := moduleInfoNames(report.TransitiveImports), []string{"downstream-load-import"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("transitive imports = %v, want %v", got, want)
	}
	if len(report.IncludedSubmodules) != 1 || report.IncludedSubmodules[0].Name != "downstream-load-sub" {
		t.Fatalf("included submodules = %#v", report.IncludedSubmodules)
	}
	if len(report.EnabledFeatures) != 1 || report.EnabledFeatures[0].Module != "downstream-load-base" || report.EnabledFeatures[0].Feature != "beta" {
		t.Fatalf("enabled features = %#v", report.EnabledFeatures)
	}
	if len(report.SourceFiles) == 0 {
		t.Fatal("load report source files = empty")
	}
}

func moduleInfoNames(modules []cambium.ModuleLoadInfo) []string {
	out := make([]string, len(modules))
	for i, mod := range modules {
		out[i] = mod.Name
	}
	return out
}

func TestLeafrefResolutionTraceAndIdentityClosure(t *testing.T) {
	source := `module downstream-resolution {
    namespace "urn:downstream-resolution";
    prefix dr;

    identity transport;
    identity tcp { base transport; }
    identity tls { base tcp; }

    leaf target { type string; }
    leaf ref-one {
        type leafref { path "../target"; }
    }
    leaf ref-two {
        type leafref { path "../ref-one"; }
    }
}`
	ctx := loadDownstreamContext(t, source)
	mod, err := ctx.Schema("downstream-resolution")
	if err != nil {
		t.Fatalf("Schema: %v", err)
	}
	refTwo := schemaNodeAt(t, mod, "/dr:ref-two")
	resolution, err := cambium.ResolveLeafrefChain(refTwo)
	if err != nil {
		t.Fatalf("ResolveLeafrefChain: %v", err)
	}
	if got, want := resolution.Target.Path(), "/downstream-resolution/target"; got != want {
		t.Fatalf("leafref target = %q, want %q", got, want)
	}
	if got, want := leafrefTracePaths(resolution.Trace), []string{"/ref-two", "/ref-one"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("leafref trace = %v, want %v", got, want)
	}

	target := schemaNodeAt(t, mod, "/dr:target")
	_, err = cambium.ResolveLeafref(target)
	var leafrefErr *cambium.LeafrefResolutionError
	if !errors.As(err, &leafrefErr) {
		t.Fatalf("ResolveLeafref non-leafref error = %T, want *LeafrefResolutionError", err)
	}
	if leafrefErr.Reason != cambium.LeafrefFailureNotLeafref {
		t.Fatalf("leafref error reason = %s, want %s", leafrefErr.Reason, cambium.LeafrefFailureNotLeafref)
	}

	transport, ok := mod.Identity("transport")
	if !ok {
		t.Fatal("Identity transport not found")
	}
	if got, want := identityNames(transport.DerivedClosure()), []string{"tcp", "tls"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("identity derived closure = %v, want %v", got, want)
	}
}

func leafrefTracePaths(trace []cambium.LeafrefTraceStep) []string {
	out := make([]string, len(trace))
	for i, step := range trace {
		out[i] = step.From.LocalPath()
	}
	return out
}

func identityNames(identities []cambium.Identity) []string {
	out := make([]string, len(identities))
	for i, identity := range identities {
		out[i] = identity.Name()
	}
	return out
}

func TestDiagnosticFromErrorIsStructured(t *testing.T) {
	builder, err := cambium.NewContextBuilder(cambium.ContextFlags{})
	if err != nil {
		t.Fatalf("NewContextBuilder: %v", err)
	}
	err = builder.LoadModuleStr(`module "bad diag" {
    namespace "urn:bad";
    prefix bd;
}`)
	if err == nil {
		t.Fatal("LoadModuleStr accepted invalid module name")
	}
	diag := cambium.DiagnosticFromError(err)
	if diag.Code != cambium.RuleCodeContext {
		t.Fatalf("diagnostic code = %s, want %s", diag.Code, cambium.RuleCodeContext)
	}
	if diag.Kind != cambium.DiagnosticInvalidIdentifier {
		t.Fatalf("diagnostic kind = %s, want %s", diag.Kind, cambium.DiagnosticInvalidIdentifier)
	}
	if !strings.Contains(diag.Message, "invalid identifier") {
		t.Fatalf("diagnostic message = %q, want invalid identifier", diag.Message)
	}
}

func TestDiagnosticIncludesRelatedSourceLocations(t *testing.T) {
	builder, err := cambium.NewContextBuilder(cambium.ContextFlags{})
	if err != nil {
		t.Fatalf("NewContextBuilder: %v", err)
	}
	err = builder.LoadModuleStr(`module downstream-diag-related {
    namespace "urn:downstream-diag-related";
    prefix ddr;

    typedef reused { type string; }
    typedef reused { type uint16; }
}`)
	if err == nil {
		_, err = builder.Build()
	}
	if err == nil {
		t.Fatal("duplicate typedef was accepted")
	}

	var diagErr *cambium.DiagnosticError
	if !errors.As(err, &diagErr) {
		t.Fatalf("duplicate typedef error = %T, want *DiagnosticError", err)
	}
	diag := cambium.DiagnosticFromError(err)
	if diag.Kind != cambium.DiagnosticSemanticSchemaError {
		t.Fatalf("diagnostic kind = %s, want %s", diag.Kind, cambium.DiagnosticSemanticSchemaError)
	}
	if diag.Source.Line == 0 {
		t.Fatalf("diagnostic source = %#v, want structured line", diag.Source)
	}
	if len(diag.Related) != 1 {
		t.Fatalf("diagnostic related = %#v, want 1 location", diag.Related)
	}
	if diag.Related[0].Line == 0 || diag.Related[0].Line >= diag.Source.Line {
		t.Fatalf("diagnostic source/related = %#v / %#v, want previous related location", diag.Source, diag.Related[0])
	}
}

func TestSchemaDiffReportsGenericSchemaChanges(t *testing.T) {
	oldSource := `module downstream-diff {
    namespace "urn:downstream-diff";
    prefix dd;

    container top {
        list item {
            key "name";
            leaf name { type string; }
            leaf old-only { type string; }
            leaf mode {
                type string;
                default "auto";
            }
            leaf constrained {
                mandatory true;
                type string;
            }
        }
    }
}`
	newSource := `module downstream-diff {
    namespace "urn:downstream-diff";
    prefix dd;

    container top {
        list item {
            key "name id";
            leaf name { type string; }
            leaf id { type uint16; }
            leaf mode {
                config false;
                type uint16;
                default "10";
            }
            leaf constrained {
                type string;
            }
            leaf new-only { type boolean; }
        }
    }
}`
	oldCtx := loadDownstreamContext(t, oldSource)
	newCtx := loadDownstreamContext(t, newSource)
	oldMod, err := oldCtx.Schema("downstream-diff")
	if err != nil {
		t.Fatalf("old Schema: %v", err)
	}
	newMod, err := newCtx.Schema("downstream-diff")
	if err != nil {
		t.Fatalf("new Schema: %v", err)
	}

	diff, err := cambium.DiffModules(oldMod, newMod)
	if err != nil {
		t.Fatalf("DiffModules: %v", err)
	}
	if diff.IsEmpty() {
		t.Fatal("DiffModules returned empty diff")
	}
	if got, want := diff.Version, cambium.SchemaDiffVersion; got != want {
		t.Fatalf("SchemaDiff version = %q, want %q", got, want)
	}
	if got := len(diff.ByKind(cambium.SchemaDiffNodeAdded)); got != 2 {
		t.Fatalf("node_added count = %d, want 2", got)
	}
	got := schemaDiffChangeKeys(diff.Changes)
	want := []string{
		"config_changed /top/item/mode",
		"constraint_changed /top/item/constrained",
		"default_changed /top/item/mode",
		"key_changed /top/item",
		"node_added /top/item/id",
		"node_added /top/item/new-only",
		"node_removed /top/item/old-only",
		"type_changed /top/item/mode",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("schema diff changes = %v, want %v", got, want)
	}
}

func TestSchemaDiffReportsAugmentProvenanceChanges(t *testing.T) {
	base := `module downstream-diff-aug-base {
    yang-version 1.1;
    namespace "urn:downstream-diff-aug-base";
    prefix ddab;

    container top;
}`
	oldAugment := `module downstream-diff-aug-old {
    yang-version 1.1;
    namespace "urn:downstream-diff-aug-old";
    prefix dda-old;

    import downstream-diff-aug-base { prefix ddab; }

    augment "/ddab:top" {
        leaf vendor-state { type string; }
    }
}`
	newAugment := `module downstream-diff-aug-new {
    yang-version 1.1;
    namespace "urn:downstream-diff-aug-new";
    prefix dda-new;

    import downstream-diff-aug-base { prefix ddab; }

    augment "/ddab:top" {
        leaf vendor-state { type string; }
    }
}`
	oldCtx := loadDownstreamContext(t, base, oldAugment)
	newCtx := loadDownstreamContext(t, base, newAugment)
	oldMod, err := oldCtx.Schema("downstream-diff-aug-base")
	if err != nil {
		t.Fatalf("old Schema: %v", err)
	}
	newMod, err := newCtx.Schema("downstream-diff-aug-base")
	if err != nil {
		t.Fatalf("new Schema: %v", err)
	}

	diff, err := cambium.DiffModules(oldMod, newMod)
	if err != nil {
		t.Fatalf("DiffModules: %v", err)
	}
	changes := diff.ByKind(cambium.SchemaDiffAugmentChanged)
	if len(changes) != 1 {
		t.Fatalf("augment changes = %#v, want 1", changes)
	}
	change := changes[0]
	if change.Path != "/top/vendor-state" {
		t.Fatalf("augment change path = %q, want /top/vendor-state", change.Path)
	}
	if change.OldValue != "downstream-diff-aug-old" || change.NewValue != "downstream-diff-aug-new" {
		t.Fatalf("augment change values = %q -> %q", change.OldValue, change.NewValue)
	}
	if got, want := change.NamespaceQualifiedPath, "/{urn:downstream-diff-aug-base}top/{urn:downstream-diff-aug-new}vendor-state"; got != want {
		t.Fatalf("augment namespace path = %q, want %q", got, want)
	}
}

func TestSchemaDiffDisambiguatesSameLocalAugmentSiblings(t *testing.T) {
	base := `module downstream-diff-dup-base {
    yang-version 1.1;
    namespace "urn:downstream-diff-dup-base";
    prefix dddb;

    container top;
}`
	left := `module downstream-diff-dup-left {
    yang-version 1.1;
    namespace "urn:downstream-diff-dup-left";
    prefix dddl;

    import downstream-diff-dup-base { prefix dddb; }

    augment "/dddb:top" {
        leaf state { type string; }
    }
}`
	oldRight := `module downstream-diff-dup-right {
    yang-version 1.1;
    namespace "urn:downstream-diff-dup-right";
    prefix dddr;

    import downstream-diff-dup-base { prefix dddb; }

    augment "/dddb:top" {
        leaf state { type string; }
    }
}`
	newRight := `module downstream-diff-dup-right {
    yang-version 1.1;
    namespace "urn:downstream-diff-dup-right";
    prefix dddr;

    import downstream-diff-dup-base { prefix dddb; }

    augment "/dddb:top" {
        leaf state { type uint16; }
    }
}`
	oldCtx := loadDownstreamContext(t, base, left, oldRight)
	newCtx := loadDownstreamContext(t, base, left, newRight)
	oldMod, err := oldCtx.Schema("downstream-diff-dup-base")
	if err != nil {
		t.Fatalf("old Schema: %v", err)
	}
	newMod, err := newCtx.Schema("downstream-diff-dup-base")
	if err != nil {
		t.Fatalf("new Schema: %v", err)
	}

	diff, err := cambium.DiffModules(oldMod, newMod)
	if err != nil {
		t.Fatalf("DiffModules: %v", err)
	}
	changes := diff.ByKind(cambium.SchemaDiffTypeChanged)
	if len(changes) != 1 {
		t.Fatalf("type changes = %#v, want 1", changes)
	}
	change := changes[0]
	if change.OldNode.Module().Name() != "downstream-diff-dup-right" || change.NewNode.Module().Name() != "downstream-diff-dup-right" {
		t.Fatalf("type change modules = %q -> %q, want downstream-diff-dup-right", change.OldNode.Module().Name(), change.NewNode.Module().Name())
	}
	if got, want := change.QualifiedPath, "/downstream-diff-dup-base:top/downstream-diff-dup-right:state"; got != want {
		t.Fatalf("type change qualified path = %q, want %q", got, want)
	}
	if got, want := change.NamespaceQualifiedPath, "/{urn:downstream-diff-dup-base}top/{urn:downstream-diff-dup-right}state"; got != want {
		t.Fatalf("type change namespace path = %q, want %q", got, want)
	}
	if got := schemaDiffChangeKeys(diff.Changes); !reflect.DeepEqual(got, []string{"type_changed /top/state"}) {
		t.Fatalf("schema diff changes = %v, want only right state type change", got)
	}
}

func TestSchemaDiffReportsDeviationEffects(t *testing.T) {
	base := `module downstream-diff-dev-base {
    yang-version 1.1;
    namespace "urn:downstream-diff-dev-base";
    prefix ddb;

    leaf value {
        type string;
        default "base";
    }
}`
	deviator := `module downstream-diff-dev-old {
    yang-version 1.1;
    namespace "urn:downstream-diff-dev-old";
    prefix ddo;

    import downstream-diff-dev-base { prefix ddb; }

    deviation "/ddb:value" {
        deviate replace {
            default "old";
        }
    }
}`
	oldCtx := loadDownstreamContext(t, base, deviator)
	newCtx := loadDownstreamContext(t, base)
	oldMod, err := oldCtx.Schema("downstream-diff-dev-base")
	if err != nil {
		t.Fatalf("old Schema: %v", err)
	}
	newMod, err := newCtx.Schema("downstream-diff-dev-base")
	if err != nil {
		t.Fatalf("new Schema: %v", err)
	}

	diff, err := cambium.DiffModules(oldMod, newMod)
	if err != nil {
		t.Fatalf("DiffModules: %v", err)
	}
	got := schemaDiffChangeKeys(diff.Changes)
	want := []string{
		"default_changed /value",
		"deviation_changed /value",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("schema diff changes = %v, want %v", got, want)
	}
}

func TestSchemaDiffReportsDeviationTypeEffects(t *testing.T) {
	base := `module downstream-diff-dev-type-base {
    yang-version 1.1;
    namespace "urn:downstream-diff-dev-type-base";
    prefix ddtb;

    leaf value {
        type string;
    }
}`
	deviator := `module downstream-diff-dev-type-source {
    yang-version 1.1;
    namespace "urn:downstream-diff-dev-type-source";
    prefix ddts;

    import downstream-diff-dev-type-base { prefix ddtb; }

    deviation "/ddtb:value" {
        deviate replace {
            type uint16;
        }
    }
}`
	oldCtx := loadDownstreamContext(t, base)
	newCtx := loadDownstreamContext(t, base, deviator)
	oldMod, err := oldCtx.Schema("downstream-diff-dev-type-base")
	if err != nil {
		t.Fatalf("old Schema: %v", err)
	}
	newMod, err := newCtx.Schema("downstream-diff-dev-type-base")
	if err != nil {
		t.Fatalf("new Schema: %v", err)
	}

	diff, err := cambium.DiffModules(oldMod, newMod)
	if err != nil {
		t.Fatalf("DiffModules: %v", err)
	}
	got := schemaDiffChangeKeys(diff.Changes)
	want := []string{
		"deviation_changed /value",
		"type_changed /value",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("schema diff changes = %v, want %v", got, want)
	}
}

func schemaDiffChangeKeys(changes []cambium.SchemaDiffChange) []string {
	out := make([]string, len(changes))
	for i, change := range changes {
		out[i] = string(change.Kind) + " " + change.Path
	}
	sort.Strings(out)
	return out
}
