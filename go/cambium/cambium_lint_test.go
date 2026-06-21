// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

package cambium_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/signalbreak-labs/cambium/go/cambium"
)

// writeLintDeps writes two importable dependency modules (dep-a, dep-b) into a
// fresh search dir and returns the dir. Each defines a typedef so an importer
// can reference its prefix.
func writeLintDeps(t *testing.T) string {
	t.Helper()
	dir := schemaIntrospectionModuleDir(t)
	writeModuleFile(t, filepath.Join(dir, "dep-a.yang"), []byte(`module dep-a {
    namespace "urn:dep-a";
    prefix a;
    typedef some-type { type string; }
}`))
	writeModuleFile(t, filepath.Join(dir, "dep-b.yang"), []byte(`module dep-b {
    namespace "urn:dep-b";
    prefix b;
    typedef other-type { type uint8; }
}`))
	return dir
}

func lintModule(t *testing.T, dir, source string) []cambium.LintFinding {
	t.Helper()
	builder, err := cambium.NewContextBuilder(cambium.ContextFlags{})
	if err != nil {
		t.Fatal(err)
	}
	if err := builder.SearchPath(dir); err != nil {
		t.Fatalf("SearchPath: %v", err)
	}
	if err := builder.LoadModuleStr(source); err != nil {
		t.Fatalf("LoadModuleStr: %v", err)
	}
	ctx, err := builder.Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	t.Cleanup(func() { ctx.Close() })
	return ctx.Lint()
}

func TestLintReportsUnusedImport(t *testing.T) {
	dir := writeLintDeps(t)
	// lintex imports both deps but references only dep-a (prefix a).
	source := `module lintex {
    namespace "urn:lintex";
    prefix lx;
    import dep-a { prefix a; }
    import dep-b { prefix b; }
    leaf x { type a:some-type; }
}`
	findings := lintModule(t, dir, source)
	if len(findings) != 1 {
		t.Fatalf("expected exactly 1 finding, got %d: %+v", len(findings), findings)
	}
	f := findings[0]
	if f.Module != "lintex" {
		t.Errorf("finding.Module = %q, want lintex", f.Module)
	}
	if f.Code != cambium.RuleCodeContext {
		t.Errorf("finding.Code = %q, want %q", f.Code, cambium.RuleCodeContext)
	}
	if !strings.Contains(f.Message, "dep-b") || !strings.Contains(f.Message, `"b"`) {
		t.Errorf("finding.Message = %q, want it to name dep-b and prefix b", f.Message)
	}
}

func TestLintNoFindingsWhenAllImportsUsed(t *testing.T) {
	dir := writeLintDeps(t)
	// References both prefixes — a directly as a type, b inside a leafref path.
	source := `module lintex2 {
    namespace "urn:lintex2";
    prefix lx;
    import dep-a { prefix a; }
    import dep-b { prefix b; }
    leaf x { type a:some-type; }
    leaf y { type b:other-type; }
}`
	if findings := lintModule(t, dir, source); len(findings) != 0 {
		t.Fatalf("expected no findings when every import is used, got %d: %+v", len(findings), findings)
	}
}

// TestLintPrefixInXPathCountsAsUse guards the over-detection contract: a prefix
// used only inside a must-expression XPath string must still count as used, so
// the import is not falsely reported.
func TestLintPrefixInXPathCountsAsUse(t *testing.T) {
	dir := writeLintDeps(t)
	source := `module lintex3 {
    namespace "urn:lintex3";
    prefix lx;
    import dep-a { prefix a; }
    import dep-b { prefix b; }
    leaf x { type a:some-type; }
    container c {
        must "/lx:x = 'ok' or count(/b:anything) > 0";
        leaf z { type string; }
    }
}`
	for _, f := range lintModule(t, dir, source) {
		if strings.Contains(f.Message, "dep-b") {
			t.Fatalf("prefix b is used in an XPath must-expression; should not be reported unused: %q", f.Message)
		}
	}
}
