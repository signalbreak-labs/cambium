// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

package compat_test

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/signalbreak-labs/cambium/go/compat"
	upstream "github.com/signalbreak-labs/cambium/go/internal/yangparse/upstream/yang"
)

func TestParserHelpersExposeGoyangStatementShape(t *testing.T) {
	source := `module compat-parse-demo {
    namespace "urn:compat-parse-demo";
    prefix cpd;

    container top {
        leaf value { type string; }
    }
}
`
	stmts, err := compat.Parse(source, "compat-parse-demo.yang")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(stmts) != 1 {
		t.Fatalf("Parse returned %d statements, want 1", len(stmts))
	}
	module := stmts[0]
	if got := module.Kind(); got != "module" {
		t.Fatalf("Kind = %q, want module", got)
	}
	if got := module.NName(); got != "compat-parse-demo" {
		t.Fatalf("NName = %q, want compat-parse-demo", got)
	}
	if got, ok := module.Arg(); !ok || got != "compat-parse-demo" {
		t.Fatalf("Arg = (%q,%v), want compat-parse-demo,true", got, ok)
	}
	if got := compat.Source(module); got != "compat-parse-demo.yang:1:1" {
		t.Fatalf("Source = %q, want compat-parse-demo.yang:1:1", got)
	}
	if got := compat.NodePath(module); got != "/compat-parse-demo" {
		t.Fatalf("NodePath = %q, want /compat-parse-demo", got)
	}
	if got := compat.CamelCase("ietf-interfaces.foo"); got != "IETFInterfacesFoo" {
		t.Fatalf("CamelCase = %q, want IETFInterfacesFoo", got)
	}

	var names []string
	for _, stmt := range module.SubStatements() {
		names = append(names, stmt.Kind()+":"+stmt.NName())
	}
	want := []string{"namespace:urn:compat-parse-demo", "prefix:cpd", "container:top"}
	if !reflect.DeepEqual(names, want) {
		t.Fatalf("SubStatements = %v, want %v", names, want)
	}

	var out bytes.Buffer
	if err := module.Write(&out, ""); err != nil {
		t.Fatalf("Statement.Write: %v", err)
	}
	if got := out.String(); !strings.Contains(got, `module "compat-parse-demo"`) ||
		!strings.Contains(got, `container "top"`) {
		t.Fatalf("Statement.Write output = %q, want module and container", got)
	}
}

func TestCompatParseMatchesGoyangEdgeCases(t *testing.T) {
	const bom = "\ufeff"
	tests := []struct {
		name   string
		source string
	}{
		{
			name:   "escaped double quoted argument",
			source: "module m { namespace \"u\"; prefix p; leaf l { type string; description \"a\\nb\\tc\\\"d\\\\e\"; } }\n",
		},
		{
			name:   "mixed quoted concatenation",
			source: "module m { namespace \"u\"; prefix p; leaf l { type string; description \"x\" + 'y' + \"z\"; } }\n",
		},
		{
			name:   "tabs in multiline quote indentation",
			source: "module m {\n\tnamespace \"u\";\n\tprefix p;\n\tdescription \"a\n\t            b\";\n}\n",
		},
		{
			name:   "bare CR in quoted argument",
			source: "module m {\n namespace \"u\"; prefix p;\n description \"a  \r b\";\n}\n",
		},
		{
			name:   "CRLF line endings",
			source: "module m {\r\n namespace \"u\";\r\n prefix p;\r\n description \"l1\r\n l2\";\r\n}\r\n",
		},
		{
			name:   "empty and comment only input",
			source: "// only a comment\n/* and a block */\n",
		},
		{
			name:   "leading BOM",
			source: bom + "module m { namespace \"u\"; prefix p; }\n",
		},
		{
			name:   "unquoted concatenation rejected",
			source: "module m { namespace \"u\"; prefix p; leaf l { type string; description foo + bar; } }\n",
		},
		{
			name:   "quoted plus unquoted rejected",
			source: "module m { namespace \"u\"; prefix p; leaf l { type string; default \"a\" + b; } }\n",
		},
		{
			name:   "hash comment rejected",
			source: "module m { namespace \"u\"; prefix p; # not a YANG comment\n}\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, gotErr := compat.Parse(tt.source, tt.name+".yang")
			want, wantErr := upstream.Parse(tt.source, tt.name+".yang")
			if (gotErr == nil) != (wantErr == nil) {
				t.Fatalf("Parse accept/reject mismatch: compat err=%v goyang err=%v", gotErr, wantErr)
			}
			if gotErr != nil {
				return
			}
			if err := compareCompatStatementsToGoyang(got, want); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func compareCompatStatementsToGoyang(got []*compat.Statement, want []*upstream.Statement) error {
	if len(got) != len(want) {
		return fmt.Errorf("statement count compat=%d goyang=%d", len(got), len(want))
	}
	for i := range got {
		if err := compareCompatStatementToGoyang(got[i], want[i], fmt.Sprintf("statements[%d]", i)); err != nil {
			return err
		}
	}
	return nil
}

func compareCompatStatementToGoyang(got *compat.Statement, want *upstream.Statement, path string) error {
	switch {
	case got == nil && want == nil:
		return nil
	case got == nil || want == nil:
		return fmt.Errorf("%s nil mismatch compat=%v goyang=%v", path, got == nil, want == nil)
	case got.Keyword != want.Keyword:
		return fmt.Errorf("%s keyword compat=%q goyang=%q", path, got.Keyword, want.Keyword)
	case got.HasArgument != want.HasArgument:
		return fmt.Errorf("%s has-argument compat=%v goyang=%v", path, got.HasArgument, want.HasArgument)
	case got.Argument != want.Argument:
		return fmt.Errorf("%s argument compat=%q goyang=%q", path, got.Argument, want.Argument)
	case got.Location() != want.Location():
		return fmt.Errorf("%s location compat=%q goyang=%q", path, got.Location(), want.Location())
	}
	gotChildren := got.SubStatements()
	wantChildren := want.SubStatements()
	if len(gotChildren) != len(wantChildren) {
		return fmt.Errorf("%s child count compat=%d goyang=%d", path, len(gotChildren), len(wantChildren))
	}
	for i := range gotChildren {
		if err := compareCompatStatementToGoyang(gotChildren[i], wantChildren[i], fmt.Sprintf("%s[%d]", path, i)); err != nil {
			return err
		}
	}
	return nil
}

func TestPathsWithModules(t *testing.T) {
	root := t.TempDir()
	first := filepath.Join(root, "first")
	second := filepath.Join(root, "nested", "second")
	for _, dir := range []string{first, second} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatalf("MkdirAll %s: %v", dir, err)
		}
	}
	if err := os.WriteFile(filepath.Join(first, "a.yang"), []byte("module a { namespace \"urn:a\"; prefix a; }"), 0o600); err != nil {
		t.Fatalf("WriteFile first: %v", err)
	}
	if err := os.WriteFile(filepath.Join(second, "b.yang"), []byte("module b { namespace \"urn:b\"; prefix b; }"), 0o600); err != nil {
		t.Fatalf("WriteFile second: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "ignored.txt"), []byte("not yang"), 0o600); err != nil {
		t.Fatalf("WriteFile ignored: %v", err)
	}

	paths, err := compat.PathsWithModules(root)
	if err != nil {
		t.Fatalf("PathsWithModules: %v", err)
	}
	sort.Strings(paths)
	want := []string{first, second}
	sort.Strings(want)
	if !reflect.DeepEqual(paths, want) {
		t.Fatalf("PathsWithModules = %v, want %v", paths, want)
	}
}

func TestValueImplementsGoyangNodeShape(t *testing.T) {
	stmts, err := compat.Parse(`module compat-value-demo { namespace "urn:compat-value-demo"; prefix cvd; }`, "compat-value-demo.yang")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	parent := &compat.Value{Name: "parent"}
	literal := compat.Value{"literal", stmts[0], parent, []*compat.Statement{{Keyword: "ext:literal"}}, &compat.Value{Name: "literal description"}}
	if literal.Name != "literal" || literal.Source != stmts[0] || literal.Parent != parent || len(literal.Extensions) != 1 || literal.Description.Name != "literal description" {
		t.Fatalf("Value literal = %#v, want goyang field order", literal)
	}
	value := &compat.Value{
		Name:        "cvd",
		Source:      stmts[0],
		Extensions:  []*compat.Statement{{Keyword: "ext:marker", Argument: "value", HasArgument: true}},
		Description: &compat.Value{Name: "local prefix"},
	}

	var node compat.Node = value
	if got := node.Kind(); got != "string" {
		t.Fatalf("Value Kind = %q, want string", got)
	}
	if got := node.NName(); got != "cvd" {
		t.Fatalf("Value NName = %q, want cvd", got)
	}
	if got := compat.Source(node); got != "compat-value-demo.yang:1:1" {
		t.Fatalf("Value Source = %q, want compat-value-demo.yang:1:1", got)
	}
	if got := node.Statement(); got != stmts[0] {
		t.Fatalf("Value Statement = %#v, want parsed module statement", got)
	}
	if got := node.Exts(); len(got) != 1 || got[0].Keyword != "ext:marker" {
		t.Fatalf("Value Exts = %#v, want ext:marker", got)
	}
	if value.Description == nil || value.Description.Name != "local prefix" {
		t.Fatalf("Value Description = %#v, want local prefix", value.Description)
	}
}

func TestMatchingExtensionsAcceptsCompatModuleNodes(t *testing.T) {
	mod := &compat.Module{
		Name:   "compat-match-demo",
		Prefix: &compat.Value{Name: "cmd"},
		Extensions: []*compat.Statement{
			{Keyword: "cmd:module-marker", Argument: "module", HasArgument: true},
		},
	}
	child := &compat.Value{
		Name:   "child",
		Parent: mod,
		Extensions: []*compat.Statement{
			{Keyword: "cmd:child-marker", Argument: "child", HasArgument: true},
		},
	}

	moduleMatches, err := compat.MatchingExtensions(mod, "compat-match-demo", "module-marker")
	if err != nil {
		t.Fatalf("MatchingExtensions(module): %v", err)
	}
	if len(moduleMatches) != 1 || moduleMatches[0].Argument != "module" {
		t.Fatalf("MatchingExtensions(module) = %#v, want module marker", moduleMatches)
	}

	childMatches, err := compat.MatchingExtensions(child, "compat-match-demo", "child-marker")
	if err != nil {
		t.Fatalf("MatchingExtensions(child): %v", err)
	}
	if len(childMatches) != 1 || childMatches[0].Argument != "child" {
		t.Fatalf("MatchingExtensions(child) = %#v, want child marker", childMatches)
	}
	if got := compat.FindModuleByPrefix(child, "missing"); got != nil {
		t.Fatalf("FindModuleByPrefix(child, missing) = %#v, want nil", got)
	}
}

func TestMatchingExtensionsUnprefixedUnknownMatchesGoyang(t *testing.T) {
	source := `module compat-bare-extension-demo {
    namespace "urn:compat-bare-extension-demo";
    prefix cbed;

    container top {
        leaf value { type string; }
    }
}
`

	compatModules := compat.NewModules()
	if err := compatModules.Parse(source, "compat-bare-extension-demo.yang"); err != nil {
		t.Fatalf("compat Parse: %v", err)
	}
	compatTop := compat.ChildNode(compatModules.Modules["compat-bare-extension-demo"], "top").(*compat.Container)
	compatTop.Extensions = []*compat.Statement{{Keyword: "bare-marker"}}
	_, compatErr := compat.MatchingExtensions(compatTop, "compat-bare-extension-demo", "marker")

	upstreamModules := upstream.NewModules()
	if err := upstreamModules.Parse(source, "compat-bare-extension-demo.yang"); err != nil {
		t.Fatalf("upstream Parse: %v", err)
	}
	upstreamTop := upstream.ChildNode(upstreamModules.Modules["compat-bare-extension-demo"], "top").(*upstream.Container)
	upstreamTop.Extensions = []*upstream.Statement{{Keyword: "bare-marker"}}
	_, upstreamErr := upstream.MatchingExtensions(upstreamTop, "compat-bare-extension-demo", "marker")

	if compatErr == nil || upstreamErr == nil {
		t.Fatalf("MatchingExtensions errors = (%v,%v), want both non-nil", compatErr, upstreamErr)
	}
	if compatErr.Error() != upstreamErr.Error() {
		t.Fatalf("MatchingExtensions error = %q, want goyang %q", compatErr, upstreamErr)
	}

	compatEntry := compat.ToEntry(compatModules.Modules["compat-bare-extension-demo"]).Lookup("top")
	compatEntry.Exts = []*compat.Statement{{Keyword: "bare-marker"}}
	_, compatEntryErr := compat.MatchingEntryExtensions(compatEntry, "compat-bare-extension-demo", "marker")

	upstreamEntry := upstream.ToEntry(upstreamModules.Modules["compat-bare-extension-demo"]).Dir["top"]
	upstreamEntry.Exts = []*upstream.Statement{{Keyword: "bare-marker"}}
	_, upstreamEntryErr := upstream.MatchingEntryExtensions(upstreamEntry, "compat-bare-extension-demo", "marker")

	if compatEntryErr == nil || upstreamEntryErr == nil {
		t.Fatalf("MatchingEntryExtensions errors = (%v,%v), want both non-nil", compatEntryErr, upstreamEntryErr)
	}
	if compatEntryErr.Error() != upstreamEntryErr.Error() {
		t.Fatalf("MatchingEntryExtensions error = %q, want goyang %q", compatEntryErr, upstreamEntryErr)
	}
}

func TestFindModuleByPrefixResolvesImportedASTModuleLikeGoyang(t *testing.T) {
	importSource := `module compat-prefix-import {
    namespace "urn:compat-prefix-import";
    prefix cpi;
}
`
	mainSource := `module compat-prefix-main {
    namespace "urn:compat-prefix-main";
    prefix cpm;

    import compat-prefix-import {
        prefix cpi;
    }

    container top {
        leaf value { type string; }
    }
}
`
	upstreamModules := upstream.NewModules()
	for name, source := range map[string]string{
		"compat-prefix-import.yang": importSource,
		"compat-prefix-main.yang":   mainSource,
	} {
		if err := upstreamModules.Parse(source, name); err != nil {
			t.Fatalf("upstream Parse(%s): %v", name, err)
		}
	}
	if errs := upstreamModules.Process(); len(errs) != 0 {
		t.Fatalf("upstream Process: %v", errs)
	}
	upstreamTop := upstream.ChildNode(upstreamModules.Modules["compat-prefix-main"], "top")
	if upstreamTop == nil {
		t.Fatal("upstream ChildNode(top) returned nil")
	}

	got := compat.FindModuleByPrefix(upstreamTop, "cpi")
	want := upstream.FindModuleByPrefix(upstreamTop, "cpi")
	if got == nil || want == nil {
		t.Fatalf("FindModuleByPrefix(imported AST prefix) = (%#v,%#v), want both non-nil", got, want)
	}
	if got.Name != want.Name {
		t.Fatalf("FindModuleByPrefix(imported AST prefix).Name = %q, want goyang %q", got.Name, want.Name)
	}
}

func TestFindModuleByPrefixFromSubmoduleMatchesGoyang(t *testing.T) {
	parentSource := `module compat-prefix-owner {
    namespace "urn:compat-prefix-owner";
    prefix cpo;

    include compat-prefix-sub;
}
`
	submoduleSource := `submodule compat-prefix-sub {
    belongs-to compat-prefix-owner {
        prefix cpo;
    }

    container from-sub {
        leaf value { type string; }
    }
}
`
	compatModules := compat.NewModules()
	upstreamModules := upstream.NewModules()
	for name, source := range map[string]string{
		"compat-prefix-owner.yang": parentSource,
		"compat-prefix-sub.yang":   submoduleSource,
	} {
		if err := compatModules.Parse(source, name); err != nil {
			t.Fatalf("compat Parse(%s): %v", name, err)
		}
		if err := upstreamModules.Parse(source, name); err != nil {
			t.Fatalf("upstream Parse(%s): %v", name, err)
		}
	}
	if errs := compatModules.Process(); len(errs) != 0 {
		t.Fatalf("compat Process: %v", errs)
	}
	if errs := upstreamModules.Process(); len(errs) != 0 {
		t.Fatalf("upstream Process: %v", errs)
	}

	compatSub := compatModules.SubModules["compat-prefix-sub"]
	upstreamSub := upstreamModules.SubModules["compat-prefix-sub"]
	if compatSub == nil || upstreamSub == nil {
		t.Fatalf("SubModules = (%#v,%#v), want both non-nil", compatSub, upstreamSub)
	}
	compatContainer := compat.ChildNode(compatSub, "from-sub")
	upstreamContainer := upstream.ChildNode(upstreamSub, "from-sub")
	if compatContainer == nil || upstreamContainer == nil {
		t.Fatalf("ChildNode(from-sub) = (%#v,%#v), want both non-nil", compatContainer, upstreamContainer)
	}

	got := compat.FindModuleByPrefix(compatContainer, "cpo")
	want := upstream.FindModuleByPrefix(upstreamContainer, "cpo")
	if got == nil || want == nil {
		t.Fatalf("FindModuleByPrefix(submodule local prefix) = (%#v,%#v), want both non-nil", got, want)
	}
	if got.Name != want.Name || got.Kind() != want.Kind() {
		t.Fatalf("FindModuleByPrefix(submodule local prefix) = %s:%s, want goyang %s:%s", got.Kind(), got.Name, want.Kind(), want.Name)
	}

	got = compat.FindModuleByPrefix(compatContainer, "compat-prefix-owner")
	want = upstream.FindModuleByPrefix(upstreamContainer, "compat-prefix-owner")
	if (got == nil) != (want == nil) {
		t.Fatalf("FindModuleByPrefix(submodule owner name) nil = %v, want goyang %v", got == nil, want == nil)
	}
	if got != nil && (got.Name != want.Name || got.Kind() != want.Kind()) {
		t.Fatalf("FindModuleByPrefix(submodule owner name) = %s:%s, want goyang %s:%s", got.Kind(), got.Name, want.Kind(), want.Name)
	}
}

func TestFindModuleByPrefixDoesNotTreatModuleNameAsPrefix(t *testing.T) {
	source := `module compat-prefix-name {
    namespace "urn:compat-prefix-name";
    prefix cpn;

    container top {
        leaf value { type string; }
    }
}
`
	compatModules := compat.NewModules()
	if err := compatModules.Parse(source, "compat-prefix-name.yang"); err != nil {
		t.Fatalf("compat Parse: %v", err)
	}
	upstreamModules := upstream.NewModules()
	if err := upstreamModules.Parse(source, "compat-prefix-name.yang"); err != nil {
		t.Fatalf("upstream Parse: %v", err)
	}

	compatTop := compat.ChildNode(compatModules.Modules["compat-prefix-name"], "top")
	upstreamTop := upstream.ChildNode(upstreamModules.Modules["compat-prefix-name"], "top")
	if compatTop == nil || upstreamTop == nil {
		t.Fatalf("ChildNode(top) = (%#v,%#v), want both non-nil", compatTop, upstreamTop)
	}

	got := compat.FindModuleByPrefix(compatTop, "compat-prefix-name")
	want := upstream.FindModuleByPrefix(upstreamTop, "compat-prefix-name")
	if (got == nil) != (want == nil) {
		t.Fatalf("FindModuleByPrefix(module name) nil = %v, want goyang %v", got == nil, want == nil)
	}
}

func TestASTHelpersTraverseCompatModuleFacadeLikeGoyang(t *testing.T) {
	source := `module compat-ast-helper-demo {
    namespace "urn:compat-ast-helper-demo";
    prefix cahd;

    extension marker {
        argument value;
    }

    grouping shared {
        leaf grouped { type string; }
    }

    container top {
        cahd:marker "top-marker";
        leaf alpha { type string; }
        uses shared;
    }
}
`
	compatModules := compat.NewModules()
	if err := compatModules.Parse(source, "compat-ast-helper-demo.yang"); err != nil {
		t.Fatalf("compat Parse: %v", err)
	}
	compatModule := compatModules.Modules["compat-ast-helper-demo"]
	if compatModule == nil {
		t.Fatal("compat module missing")
	}

	upstreamModules := upstream.NewModules()
	if err := upstreamModules.Parse(source, "compat-ast-helper-demo.yang"); err != nil {
		t.Fatalf("upstream Parse: %v", err)
	}
	upstreamModule := upstreamModules.Modules["compat-ast-helper-demo"]
	if upstreamModule == nil {
		t.Fatal("upstream module missing")
	}

	assertNodeMatch := func(label string, got compat.Node, want upstream.Node) {
		t.Helper()
		if got == nil || want == nil {
			t.Fatalf("%s = (%#v,%#v), want both non-nil", label, got, want)
		}
		if got.Kind() != want.Kind() || got.NName() != want.NName() {
			t.Fatalf("%s = %s:%s, want goyang %s:%s", label, got.Kind(), got.NName(), want.Kind(), want.NName())
		}
	}

	assertNodeMatch("ChildNode(top)", compat.ChildNode(compatModule, "top"), upstream.ChildNode(upstreamModule, "top"))
	assertNodeMatch("FindNode(relative)", mustFindCompatNode(t, compatModule, "top/alpha"), mustFindUpstreamNode(t, upstreamModule, "top/alpha"))
	assertNodeMatch("FindNode(absolute)", mustFindCompatNode(t, compatModule, "/cahd:top/cahd:alpha"), mustFindUpstreamNode(t, upstreamModule, "/cahd:top/cahd:alpha"))
	assertNodeMatch("FindNode(uses)", mustFindCompatNode(t, compatModule, "top/grouped"), mustFindUpstreamNode(t, upstreamModule, "top/grouped"))

	top := compat.ChildNode(compatModule, "top")
	if got, want := compat.NodePath(top), upstream.NodePath(upstream.ChildNode(upstreamModule, "top")); got != want {
		t.Fatalf("NodePath(top) = %q, want goyang %q", got, want)
	}
	matches, err := compat.MatchingExtensions(top, "compat-ast-helper-demo", "marker")
	if err != nil {
		t.Fatalf("MatchingExtensions(top): %v", err)
	}
	upstreamMatches, err := upstream.MatchingExtensions(upstream.ChildNode(upstreamModule, "top"), "compat-ast-helper-demo", "marker")
	if err != nil {
		t.Fatalf("upstream MatchingExtensions(top): %v", err)
	}
	if len(matches) != len(upstreamMatches) || len(matches) != 1 || matches[0].Argument != upstreamMatches[0].Argument {
		t.Fatalf("MatchingExtensions(top) = %#v, want goyang %#v", matches, upstreamMatches)
	}
}

func TestFindNodeSourceBackedASTFallbackUsesOrderedStatements(t *testing.T) {
	source := `module compat-source-backed-find {
    namespace "urn:compat-source-backed-find";
    prefix csbf;

    grouping shared {
        leaf grouped { type string; }
    }

    container top {
        leaf before { type string; }
        uses shared;
        leaf after { type string; }
    }
}
`
	stmts, err := compat.Parse(source, "compat-source-backed-find.yang")
	if err != nil {
		t.Fatalf("compat Parse: %v", err)
	}
	module := &compat.ASTModule{
		Name:   "compat-source-backed-find",
		Prefix: &compat.ASTValue{Name: "csbf"},
		Source: stmts[0],
	}

	top := mustFindCompatNode(t, module, "top")
	if top.Kind() != "container" || top.NName() != "top" {
		t.Fatalf("FindNode(top) = %s:%s, want container:top", top.Kind(), top.NName())
	}
	grouped := mustFindCompatNode(t, module, "top/grouped")
	if grouped.Kind() != "leaf" || grouped.NName() != "grouped" {
		t.Fatalf("FindNode(top/grouped) = %s:%s, want leaf:grouped", grouped.Kind(), grouped.NName())
	}
	absolute := mustFindCompatNode(t, module, "/csbf:top/csbf:after")
	if absolute.Kind() != "leaf" || absolute.NName() != "after" {
		t.Fatalf("FindNode(/csbf:top/csbf:after) = %s:%s, want leaf:after", absolute.Kind(), absolute.NName())
	}
}

func TestChildNodeSourceBackedASTFallbackCoversStatementKinds(t *testing.T) {
	source := `module compat-source-backed-kinds {
    yang-version 1.1;
    namespace "urn:compat-source-backed-kinds";
    prefix csbk;

    extension marker;
    feature gate;
    identity base;
    typedef local-id { type string; }
    grouping common {
        leaf grouped { type string; }
    }
    leaf scalar { type string; }
    leaf-list tags { type string; }
    anydata payload;
    anyxml legacy;
    choice mode {
        case automatic {
            leaf enabled { type boolean; }
        }
    }
    list item {
        key "name";
        leaf name { type string; }
    }
    container top {
        uses common;
        action reset {
            input {
                leaf force { type boolean; }
            }
            output {
                leaf status { type string; }
            }
        }
        notification changed {
            leaf reason { type string; }
        }
    }
    rpc sync {
        input {
            leaf request { type string; }
        }
        output {
            leaf response { type string; }
        }
    }
    notification event {
        leaf message { type string; }
    }
    augment "/csbk:top" {
        leaf augmented { type string; }
    }
    deviation "/csbk:scalar" {
        deviate add {
            default "x";
        }
    }
}
`
	stmts, err := compat.Parse(source, "compat-source-backed-kinds.yang")
	if err != nil {
		t.Fatalf("compat Parse: %v", err)
	}
	module := &compat.ASTModule{
		Name:   "compat-source-backed-kinds",
		Prefix: &compat.ASTValue{Name: "csbk"},
		Source: stmts[0],
	}

	for _, tc := range []struct {
		name string
		kind string
	}{
		{name: "marker", kind: "extension"},
		{name: "gate", kind: "feature"},
		{name: "base", kind: "identity"},
		{name: "local-id", kind: "typedef"},
		{name: "common", kind: "grouping"},
		{name: "scalar", kind: "leaf"},
		{name: "tags", kind: "leaf-list"},
		{name: "payload", kind: "anydata"},
		{name: "legacy", kind: "anyxml"},
		{name: "mode", kind: "choice"},
		{name: "item", kind: "list"},
		{name: "top", kind: "container"},
		{name: "sync", kind: "rpc"},
		{name: "event", kind: "notification"},
		{name: "/csbk:top", kind: "augment"},
		{name: "/csbk:scalar", kind: "deviation"},
	} {
		got := compat.ChildNode(module, tc.name)
		if got == nil {
			t.Fatalf("ChildNode(module, %q) = nil, want %s", tc.name, tc.kind)
		}
		if got.Kind() != tc.kind || got.NName() != tc.name {
			t.Fatalf("ChildNode(module, %q) = %s:%s, want %s:%s", tc.name, got.Kind(), got.NName(), tc.kind, tc.name)
		}
		if got.Statement() == nil || got.ParentNode() != module {
			t.Fatalf("ChildNode(module, %q) metadata = statement:%v parent:%#v, want source-backed parent module", tc.name, got.Statement() != nil, got.ParentNode())
		}
	}

	top := compat.ChildNode(module, "top")
	if top == nil {
		t.Fatal("ChildNode(module, top) = nil")
	}
	for _, tc := range []struct {
		name string
		kind string
	}{
		{name: "grouped", kind: "leaf"},
		{name: "reset", kind: "action"},
		{name: "changed", kind: "notification"},
	} {
		got := compat.ChildNode(top, tc.name)
		if got == nil {
			t.Fatalf("ChildNode(top, %q) = nil, want %s", tc.name, tc.kind)
		}
		if got.Kind() != tc.kind || got.NName() != tc.name {
			t.Fatalf("ChildNode(top, %q) = %s:%s, want %s:%s", tc.name, got.Kind(), got.NName(), tc.kind, tc.name)
		}
	}

	action := compat.ChildNode(top, "reset")
	actionNode, ok := action.(*compat.Action)
	if !ok {
		t.Fatalf("ChildNode(top, reset) = %T, want *compat.Action", action)
	}
	input := actionNode.Input
	output := actionNode.Output
	if input == nil || input.Kind() != "input" || input.NName() != "" || output == nil || output.Kind() != "output" || output.NName() != "" {
		t.Fatalf("action IO = (%#v,%#v), want goyang-shaped input/output with empty NName", input, output)
	}
	if force := compat.ChildNode(input, "force"); force == nil || force.Kind() != "leaf" {
		t.Fatalf("ChildNode(action input, force) = %#v, want leaf", force)
	}
	if status := compat.ChildNode(output, "status"); status == nil || status.Kind() != "leaf" {
		t.Fatalf("ChildNode(action output, status) = %#v, want leaf", status)
	}
	rpc := compat.ChildNode(module, "sync")
	rpcNode, ok := rpc.(*compat.RPC)
	if !ok {
		t.Fatalf("ChildNode(module, sync) = %T, want *compat.RPC", rpc)
	}
	rpcInput := rpcNode.Input
	rpcOutput := rpcNode.Output
	if rpcInput == nil || rpcInput.Kind() != "input" || rpcInput.NName() != "" || rpcOutput == nil || rpcOutput.Kind() != "output" || rpcOutput.NName() != "" {
		t.Fatalf("rpc IO = (%#v,%#v), want goyang-shaped input/output with empty NName", rpcInput, rpcOutput)
	}
	if request := compat.ChildNode(rpcInput, "request"); request == nil || request.Kind() != "leaf" {
		t.Fatalf("ChildNode(rpc input, request) = %#v, want leaf", request)
	}
	if response := compat.ChildNode(rpcOutput, "response"); response == nil || response.Kind() != "leaf" {
		t.Fatalf("ChildNode(rpc output, response) = %#v, want leaf", response)
	}
	deviation := compat.ChildNode(module, "/csbk:scalar")
	deviate := compat.ChildNode(deviation, "add")
	if deviate == nil || deviate.Kind() != "deviate" {
		t.Fatalf("ChildNode(deviation, add) = %#v, want deviate", deviate)
	}
}

func TestSourceBackedOperationPayloadCoversStatementKinds(t *testing.T) {
	source := `module compat-source-backed-operation-payload {
    yang-version 1.1;
    namespace "urn:compat-source-backed-operation-payload";
    prefix csop;

    grouping op-common {
        leaf common-leaf { type string; }
    }

    container top {
        action run {
            input {
                anydata action-in-anydata;
                anyxml action-in-anyxml;
                choice action-in-choice {
                    case selected {
                        leaf action-in-choice-leaf { type string; }
                    }
                }
                container action-in-container {
                    leaf nested { type string; }
                }
                grouping action-in-group {
                    leaf grouped { type string; }
                }
                leaf action-in-leaf { type string; }
                leaf-list action-in-leaf-list { type string; }
                list action-in-list {
                    key "name";
                    leaf name { type string; }
                }
                typedef action-in-typedef { type string; }
                uses op-common;
            }
            output {
                anydata action-out-anydata;
                anyxml action-out-anyxml;
                choice action-out-choice {
                    case selected {
                        leaf action-out-choice-leaf { type string; }
                    }
                }
                container action-out-container {
                    leaf nested { type string; }
                }
                grouping action-out-group {
                    leaf grouped { type string; }
                }
                leaf action-out-leaf { type string; }
                leaf-list action-out-leaf-list { type string; }
                list action-out-list {
                    key "name";
                    leaf name { type string; }
                }
                typedef action-out-typedef { type string; }
                uses op-common;
            }
        }
    }

    rpc sync {
        input {
            anydata rpc-in-anydata;
            anyxml rpc-in-anyxml;
            choice rpc-in-choice {
                case selected {
                    leaf rpc-in-choice-leaf { type string; }
                }
            }
            container rpc-in-container {
                leaf nested { type string; }
            }
            grouping rpc-in-group {
                leaf grouped { type string; }
            }
            leaf rpc-in-leaf { type string; }
            leaf-list rpc-in-leaf-list { type string; }
            list rpc-in-list {
                key "name";
                leaf name { type string; }
            }
            typedef rpc-in-typedef { type string; }
            uses op-common;
        }
        output {
            anydata rpc-out-anydata;
            anyxml rpc-out-anyxml;
            choice rpc-out-choice {
                case selected {
                    leaf rpc-out-choice-leaf { type string; }
                }
            }
            container rpc-out-container {
                leaf nested { type string; }
            }
            grouping rpc-out-group {
                leaf grouped { type string; }
            }
            leaf rpc-out-leaf { type string; }
            leaf-list rpc-out-leaf-list { type string; }
            list rpc-out-list {
                key "name";
                leaf name { type string; }
            }
            typedef rpc-out-typedef { type string; }
            uses op-common;
        }
    }
}
`
	stmts, err := compat.Parse(source, "compat-source-backed-operation-payload.yang")
	if err != nil {
		t.Fatalf("compat Parse: %v", err)
	}
	module := &compat.ASTModule{
		Name:   "compat-source-backed-operation-payload",
		Prefix: &compat.ASTValue{Name: "csop"},
		Source: stmts[0],
	}

	top := compat.ChildNode(module, "top")
	if top == nil {
		t.Fatal("ChildNode(module, top) = nil")
	}
	actionNode, ok := compat.ChildNode(top, "run").(*compat.Action)
	if !ok {
		t.Fatalf("ChildNode(top, run) = %T, want *compat.Action", compat.ChildNode(top, "run"))
	}
	assertOperationPayloadChildren(t, "action input", actionNode.Input, "action-in")
	assertOperationPayloadChildren(t, "action output", actionNode.Output, "action-out")

	rpcNode, ok := compat.ChildNode(module, "sync").(*compat.RPC)
	if !ok {
		t.Fatalf("ChildNode(module, sync) = %T, want *compat.RPC", compat.ChildNode(module, "sync"))
	}
	assertOperationPayloadChildren(t, "rpc input", rpcNode.Input, "rpc-in")
	assertOperationPayloadChildren(t, "rpc output", rpcNode.Output, "rpc-out")
}

func assertOperationPayloadChildren(t *testing.T, label string, payload compat.Node, prefix string) {
	t.Helper()
	if payload == nil {
		t.Fatalf("%s payload = nil", label)
	}
	for _, tc := range []struct {
		name string
		kind string
	}{
		{name: prefix + "-anydata", kind: "anydata"},
		{name: prefix + "-anyxml", kind: "anyxml"},
		{name: prefix + "-choice", kind: "choice"},
		{name: prefix + "-container", kind: "container"},
		{name: prefix + "-group", kind: "grouping"},
		{name: prefix + "-leaf", kind: "leaf"},
		{name: prefix + "-leaf-list", kind: "leaf-list"},
		{name: prefix + "-list", kind: "list"},
		{name: prefix + "-typedef", kind: "typedef"},
	} {
		got := compat.ChildNode(payload, tc.name)
		if got == nil {
			t.Fatalf("%s ChildNode(%q) = nil, want %s", label, tc.name, tc.kind)
		}
		if got.Kind() != tc.kind || got.NName() != tc.name {
			t.Fatalf("%s ChildNode(%q) = %s:%s, want %s:%s", label, tc.name, got.Kind(), got.NName(), tc.kind, tc.name)
		}
		if got.Statement() == nil || got.ParentNode() != payload {
			t.Fatalf("%s ChildNode(%q) metadata = statement:%v parent:%#v, want source-backed payload child", label, tc.name, got.Statement() != nil, got.ParentNode())
		}
	}
	grouped := compat.ChildNode(payload, "common-leaf")
	if grouped == nil {
		t.Fatalf("%s ChildNode(common-leaf) = nil, want leaf from uses grouping", label)
	}
	if grouped.Kind() != "leaf" || grouped.NName() != "common-leaf" {
		t.Fatalf("%s ChildNode(common-leaf) = %s:%s, want leaf:common-leaf", label, grouped.Kind(), grouped.NName())
	}
}

func TestPrintNodeCompatModuleMatchesGoyang(t *testing.T) {
	source := `module compat-print-node-demo {
    namespace "urn:compat-print-node-demo";
    prefix cpnd;

    identity transport {
        description "Transport identity.";
    }

    container top {
        leaf alpha { type string; }
    }
}
`
	compatModules := compat.NewModules()
	if err := compatModules.Parse(source, "compat-print-node-demo.yang"); err != nil {
		t.Fatalf("compat Parse: %v", err)
	}
	upstreamModules := upstream.NewModules()
	if err := upstreamModules.Parse(source, "compat-print-node-demo.yang"); err != nil {
		t.Fatalf("upstream Parse: %v", err)
	}

	var got, want bytes.Buffer
	compat.PrintNode(&got, compatModules.Modules["compat-print-node-demo"])
	upstream.PrintNode(&want, upstreamModules.Modules["compat-print-node-demo"])
	if got.String() != want.String() {
		t.Fatalf("PrintNode compat module =\n%s\nwant goyang\n%s", got.String(), want.String())
	}
}

func TestFindGroupingTraversesCompatImportsAndIncludesLikeGoyang(t *testing.T) {
	mainSource := `module compat-grouping-main {
    namespace "urn:compat-grouping-main";
    prefix cgm;

    import compat-grouping-import {
        prefix cgi;
    }
    include compat-grouping-sub;

    container top {
        uses cgi:imported-group;
        uses included-group;
    }
}
`
	importSource := `module compat-grouping-import {
    namespace "urn:compat-grouping-import";
    prefix cgi;

    grouping imported-group {
        leaf imported-leaf { type string; }
    }
}
`
	submoduleSource := `submodule compat-grouping-sub {
    belongs-to compat-grouping-main { prefix cgm; }

    grouping included-group {
        leaf included-leaf { type string; }
    }
}
`

	dir := t.TempDir()
	writeYANG(t, filepath.Join(dir, "compat-grouping-main.yang"), mainSource)
	writeYANG(t, filepath.Join(dir, "compat-grouping-import.yang"), importSource)
	writeYANG(t, filepath.Join(dir, "compat-grouping-sub.yang"), submoduleSource)

	compatModules := compat.NewModules()
	compatModules.AddPath(dir)
	if err := compatModules.Read("compat-grouping-main"); err != nil {
		t.Fatalf("compat Read: %v", err)
	}
	if errs := compatModules.Process(); len(errs) != 0 {
		t.Fatalf("compat Process: %v", errs)
	}
	compatModule := compatModules.Modules["compat-grouping-main"]
	if compatModule == nil {
		t.Fatal("compat module missing")
	}
	compatTop := compat.ChildNode(compatModule, "top")
	if compatTop == nil {
		t.Fatal("compat ChildNode(top) returned nil")
	}

	upstreamModules := upstream.NewModules()
	upstreamModules.AddPath(dir)
	if err := upstreamModules.Read("compat-grouping-main"); err != nil {
		t.Fatalf("upstream Read: %v", err)
	}
	if errs := upstreamModules.Process(); len(errs) != 0 {
		t.Fatalf("upstream Process: %v", errs)
	}
	upstreamModule := upstreamModules.Modules["compat-grouping-main"]
	if upstreamModule == nil {
		t.Fatal("upstream module missing")
	}
	upstreamTop := upstream.ChildNode(upstreamModule, "top")
	if upstreamTop == nil {
		t.Fatal("upstream ChildNode(top) returned nil")
	}

	assertGroupingMatch := func(label, name string) {
		t.Helper()
		got := compat.FindGrouping(compatTop, name, map[string]bool{})
		want := upstream.FindGrouping(upstreamTop, name, map[string]bool{})
		if got == nil || want == nil {
			t.Fatalf("%s FindGrouping(%q) = (%#v,%#v), want both non-nil", label, name, got, want)
		}
		if got.Name != want.Name {
			t.Fatalf("%s FindGrouping(%q).Name = %q, want goyang %q", label, name, got.Name, want.Name)
		}
	}

	assertGroupingMatch("import", "cgi:imported-group")
	assertGroupingMatch("include", "included-group")
}

func TestProcessInMemoryParsedImportsAndIncludesLikeGoyang(t *testing.T) {
	sources := []struct {
		name string
		data string
	}{
		{
			name: "compat-inmemory-main.yang",
			data: `module compat-inmemory-main {
    namespace "urn:compat-inmemory-main";
    prefix cim;

    import compat-inmemory-import {
        prefix cii;
    }
    include compat-inmemory-sub;

    container top {
        uses cii:imported-group;
        uses included-group;
    }
}
`,
		},
		{
			name: "compat-inmemory-import.yang",
			data: `module compat-inmemory-import {
    namespace "urn:compat-inmemory-import";
    prefix cii;

    grouping imported-group {
        leaf imported-leaf { type string; }
    }
}
`,
		},
		{
			name: "compat-inmemory-sub.yang",
			data: `submodule compat-inmemory-sub {
    belongs-to compat-inmemory-main { prefix cim; }

    grouping included-group {
        leaf included-leaf { type string; }
    }
}
`,
		},
	}

	compatModules := compat.NewModules()
	upstreamModules := upstream.NewModules()
	for _, source := range sources {
		if err := compatModules.Parse(source.data, source.name); err != nil {
			t.Fatalf("compat Parse(%s): %v", source.name, err)
		}
		if err := upstreamModules.Parse(source.data, source.name); err != nil {
			t.Fatalf("upstream Parse(%s): %v", source.name, err)
		}
	}
	if errs := upstreamModules.Process(); len(errs) != 0 {
		t.Fatalf("upstream Process: %v", errs)
	}
	if errs := compatModules.Process(); len(errs) != 0 {
		t.Fatalf("compat Process: %v", errs)
	}

	compatTop := compat.ChildNode(compatModules.Modules["compat-inmemory-main"], "top")
	upstreamTop := upstream.ChildNode(upstreamModules.Modules["compat-inmemory-main"], "top")
	for _, name := range []string{"cii:imported-group", "included-group"} {
		got := compat.FindGrouping(compatTop, name, map[string]bool{})
		want := upstream.FindGrouping(upstreamTop, name, map[string]bool{})
		if got == nil || want == nil {
			t.Fatalf("FindGrouping(%q) = (%#v,%#v), want both non-nil", name, got, want)
		}
		if got.Name != want.Name {
			t.Fatalf("FindGrouping(%q).Name = %q, want goyang %q", name, got.Name, want.Name)
		}
	}
}

func TestToEntryASTModuleStoreUsesMatchesGoyang(t *testing.T) {
	source := `module compat-ast-store-uses {
    namespace "urn:compat-ast-store-uses";
    prefix casu;

    grouping common {
        leaf grouped { type string; }
    }

    container top {
        uses common;
    }
}
`
	upstreamModules := upstream.NewModules()
	upstreamModules.ParseOptions.StoreUses = true
	if err := upstreamModules.Parse(source, "compat-ast-store-uses.yang"); err != nil {
		t.Fatalf("upstream Parse: %v", err)
	}
	if errs := upstreamModules.Process(); len(errs) != 0 {
		t.Fatalf("upstream Process: %v", errs)
	}
	rawModule := upstreamModules.Modules["compat-ast-store-uses"]
	compatRoot := compat.ToEntry(rawModule)
	upstreamRoot := upstream.ToEntry(rawModule)

	compatTop := compatRoot.Find("top")
	upstreamTop := upstreamRoot.Find("top")
	if compatTop == nil || upstreamTop == nil {
		t.Fatalf("Find(top) = (%#v,%#v), want both non-nil", compatTop, upstreamTop)
	}
	if got, want := len(compatTop.Uses), len(upstreamTop.Uses); got != want {
		t.Fatalf("ToEntry(upstream module) top Uses len = %d, want goyang %d", got, want)
	}
	if len(compatTop.Uses) != 0 {
		if got, want := compatTop.Uses[0].Uses.Name, upstreamTop.Uses[0].Uses.Name; got != want {
			t.Fatalf("ToEntry(upstream module) top Uses[0].Uses.Name = %q, want goyang %q", got, want)
		}
		if got, want := compatTop.Uses[0].Grouping.Name, upstreamTop.Uses[0].Grouping.Name; got != want {
			t.Fatalf("ToEntry(upstream module) top Uses[0].Grouping.Name = %q, want goyang %q", got, want)
		}
	}
}

func TestToEntryAcceptsASTModuleAndPreservesSourceOrder(t *testing.T) {
	source := `module compat-to-entry-ast {
    namespace "urn:compat-to-entry-ast";
    prefix ctea;

    container z-top {
        leaf zed { type string; }
    }
    leaf a-leaf {
        type uint32;
        default "7";
    }
    container m-top {
        leaf middle { type string; }
    }
}
`
	stmts, err := compat.Parse(source, "compat-to-entry-ast.yang")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(stmts) != 1 {
		t.Fatalf("Parse returned %d statements, want 1", len(stmts))
	}
	module := &compat.ASTModule{
		Name:      "compat-to-entry-ast",
		Source:    stmts[0],
		Namespace: &compat.ASTValue{Name: "urn:compat-to-entry-ast"},
		Prefix:    &compat.ASTValue{Name: "ctea"},
	}
	entry := compat.ToEntry(module)
	if entry == nil {
		t.Fatal("ToEntry(ASTModule) returned nil")
	}
	if got := entry.Name; got != "compat-to-entry-ast" {
		t.Fatalf("entry Name = %q, want compat-to-entry-ast", got)
	}
	if got, want := childNames(entry.Children()), []string{"z-top", "a-leaf", "m-top"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("AST ToEntry children = %v, want %v", got, want)
	}
	aLeaf := entry.Lookup("a-leaf")
	if aLeaf == nil || !aLeaf.IsLeaf() || aLeaf.Type == nil || aLeaf.Type.Kind != compat.Yuint32 {
		t.Fatalf("a-leaf entry = %#v, want uint32 leaf", aLeaf)
	}
	if got, ok := aLeaf.SingleDefaultValue(); !ok || got != "7" {
		t.Fatalf("a-leaf SingleDefaultValue = (%q,%v), want 7,true", got, ok)
	}
}

func TestToEntryASTModuleExpandsUsesLikeGoyang(t *testing.T) {
	source := `module compat-to-entry-uses {
    namespace "urn:compat-to-entry-uses";
    prefix cteu;

    grouping common {
        leaf zed { type string; }
        leaf alpha { type string; }
    }

    container top {
        leaf before { type string; }
        uses common;
        leaf after { type string; }
    }
}
`
	compatModules := compat.NewModules()
	if err := compatModules.Parse(source, "compat-to-entry-uses.yang"); err != nil {
		t.Fatalf("compat Parse: %v", err)
	}
	compatModule := compatModules.Modules["compat-to-entry-uses"]
	if compatModule == nil {
		t.Fatal("compat module not recorded")
	}
	compatEntry := compat.ToEntry(compatModule)

	upstreamModules := upstream.NewModules()
	if err := upstreamModules.Parse(source, "compat-to-entry-uses.yang"); err != nil {
		t.Fatalf("upstream Parse: %v", err)
	}
	upstreamModule := upstreamModules.Modules["compat-to-entry-uses"]
	if upstreamModule == nil {
		t.Fatal("upstream module not recorded")
	}
	upstreamEntry := upstream.ToEntry(upstreamModule)

	for _, path := range []string{"top/zed", "top/alpha"} {
		t.Run(path, func(t *testing.T) {
			upstreamFound := upstreamEntry.Find(path)
			if upstreamFound == nil {
				t.Fatalf("upstream ToEntry Find(%q) = nil, expected grouping child", path)
			}
			compatFound := compatEntry.Find(path)
			if compatFound == nil {
				t.Fatalf("compat ToEntry Find(%q) = nil, want goyang %s", path, upstreamFound.Name)
			}
			if compatFound.Name != upstreamFound.Name || compatFound.Kind.String() != upstreamFound.Kind.String() {
				t.Fatalf("compat ToEntry Find(%q) = %s/%s, want goyang %s/%s", path, compatFound.Name, compatFound.Kind, upstreamFound.Name, upstreamFound.Kind)
			}
			if got, want := valueName(compatFound.Namespace()), upstreamValueName(upstreamFound.Namespace()); got != want {
				t.Fatalf("compat ToEntry Find(%q).Namespace = %q, want goyang %q", path, got, want)
			}
			if got, want := compatFound.Path(), upstreamFound.Path(); got != want {
				t.Fatalf("compat ToEntry Find(%q).Path = %q, want goyang %q", path, got, want)
			}
			if compatFound.Modules() == nil {
				t.Fatalf("compat ToEntry Find(%q).Modules = nil, want goyang module set", path)
			}
			if got, err := compatFound.InstantiatingModule(); err != nil || got != "compat-to-entry-uses" {
				t.Fatalf("compat ToEntry Find(%q).InstantiatingModule = (%q,%v), want compat-to-entry-uses,nil", path, got, err)
			}
		})
	}
}

func TestToEntryASTModuleExpandsImportedAndIncludedUsesLikeGoyang(t *testing.T) {
	mainSource := `module compat-to-entry-foreign-uses {
    namespace "urn:compat-to-entry-foreign-uses";
    prefix ctefu;

    import compat-to-entry-foreign-import {
        prefix ctefi;
    }
    include compat-to-entry-foreign-sub;

    container top {
        leaf before { type string; }
        uses ctefi:imported-group;
        uses included-group;
        leaf after { type string; }
    }
}
`
	importSource := `module compat-to-entry-foreign-import {
    namespace "urn:compat-to-entry-foreign-import";
    prefix ctefi;

    grouping imported-nested {
        leaf imported-nested-leaf { type string; }
    }
    grouping imported-group {
        leaf imported-leaf { type string; }
        uses imported-nested;
    }
}
`
	submoduleSource := `submodule compat-to-entry-foreign-sub {
    belongs-to compat-to-entry-foreign-uses {
        prefix ctefu;
    }

    grouping included-nested {
        leaf included-nested-leaf { type string; }
    }
    grouping included-group {
        leaf included-leaf { type string; }
        uses included-nested;
    }
}
`

	dir := t.TempDir()
	writeYANG(t, filepath.Join(dir, "compat-to-entry-foreign-uses.yang"), mainSource)
	writeYANG(t, filepath.Join(dir, "compat-to-entry-foreign-import.yang"), importSource)
	writeYANG(t, filepath.Join(dir, "compat-to-entry-foreign-sub.yang"), submoduleSource)

	upstreamModules := upstream.NewModules()
	upstreamModules.AddPath(dir)
	if err := upstreamModules.Read("compat-to-entry-foreign-uses"); err != nil {
		t.Fatalf("upstream Read: %v", err)
	}
	if errs := upstreamModules.Process(); len(errs) != 0 {
		t.Fatalf("upstream Process: %v", errs)
	}
	rawModule := upstreamModules.Modules["compat-to-entry-foreign-uses"]
	compatEntry := compat.ToEntry(rawModule)
	if errs := compatEntry.GetErrors(); len(errs) != 0 {
		t.Fatalf("compat ToEntry errors = %v", errs)
	}
	upstreamEntry := upstream.ToEntry(rawModule)

	for _, path := range []string{
		"top/imported-leaf",
		"top/imported-nested-leaf",
		"top/included-leaf",
		"top/included-nested-leaf",
	} {
		t.Run(path, func(t *testing.T) {
			upstreamFound := upstreamEntry.Find(path)
			if upstreamFound == nil {
				t.Fatalf("upstream ToEntry Find(%q) = nil, expected grouping child", path)
			}
			compatFound := compatEntry.Find(path)
			if compatFound == nil {
				t.Fatalf("compat ToEntry Find(%q) = nil, want goyang %s", path, upstreamFound.Name)
			}
			if compatFound.Name != upstreamFound.Name || compatFound.Kind.String() != upstreamFound.Kind.String() {
				t.Fatalf("compat ToEntry Find(%q) = %s/%s, want goyang %s/%s", path, compatFound.Name, compatFound.Kind, upstreamFound.Name, upstreamFound.Kind)
			}
		})
	}

	top := compatEntry.Find("top")
	if top == nil {
		t.Fatal("compat ToEntry Find(top) = nil")
	}
	wantOrder := []string{"before", "imported-leaf", "imported-nested-leaf", "included-leaf", "included-nested-leaf", "after"}
	if got := childNames(top.Children()); !reflect.DeepEqual(got, wantOrder) {
		t.Fatalf("compat ToEntry top children = %v, want %v", got, wantOrder)
	}
}

func TestToEntryASTFindImportedAbsolutePrefixMatchesGoyang(t *testing.T) {
	importedSource := `module compat-to-entry-find-imported {
    namespace "urn:compat-to-entry-find-imported";
    prefix ctefi;

    container remote {
        leaf value { type string; }
    }
}
`
	mainSource := `module compat-to-entry-find-main {
    namespace "urn:compat-to-entry-find-main";
    prefix ctefm;

    import compat-to-entry-find-imported {
        prefix ctefi;
    }

    container local {
        leaf value { type string; }
    }
}
`
	compatModules := compat.NewModules()
	upstreamModules := upstream.NewModules()
	for name, source := range map[string]string{
		"compat-to-entry-find-imported.yang": importedSource,
		"compat-to-entry-find-main.yang":     mainSource,
	} {
		if err := compatModules.Parse(source, name); err != nil {
			t.Fatalf("compat Parse(%s): %v", name, err)
		}
		if err := upstreamModules.Parse(source, name); err != nil {
			t.Fatalf("upstream Parse(%s): %v", name, err)
		}
	}
	if errs := upstreamModules.Process(); len(errs) != 0 {
		t.Fatalf("upstream Process: %v", errs)
	}
	compatModule := compatModules.Modules["compat-to-entry-find-main"]
	upstreamModule := upstreamModules.Modules["compat-to-entry-find-main"]
	if compatModule == nil || upstreamModule == nil {
		t.Fatalf("main module = (%#v,%#v), want both non-nil", compatModule, upstreamModule)
	}
	compatEntry := compat.ToEntry(compatModule)
	upstreamEntry := upstream.ToEntry(upstreamModule)
	path := "/ctefi:remote/ctefi:value"
	compatFound := compatEntry.Find(path)
	upstreamFound := upstreamEntry.Find(path)
	if upstreamFound == nil {
		t.Fatalf("upstream ToEntry Find(%q) = nil, expected imported leaf", path)
	}
	if compatFound == nil {
		t.Fatalf("compat ToEntry Find(%q) = nil, want goyang %s", path, upstreamFound.Name)
	}
	if compatFound.Name != upstreamFound.Name || compatFound.Kind.String() != upstreamFound.Kind.String() {
		t.Fatalf("compat ToEntry Find(%q) = %s/%s, want goyang %s/%s", path, compatFound.Name, compatFound.Kind, upstreamFound.Name, upstreamFound.Kind)
	}
	compatRawEntry := compat.ToEntry(upstreamModule)
	compatRawFound := compatRawEntry.Find(path)
	if compatRawFound == nil {
		t.Fatalf("compat ToEntry(upstream module) Find(%q) = nil, want goyang %s", path, upstreamFound.Name)
	}
	if compatRawFound.Name != upstreamFound.Name || compatRawFound.Kind.String() != upstreamFound.Kind.String() {
		t.Fatalf("compat ToEntry(upstream module) Find(%q) = %s/%s, want goyang %s/%s", path, compatRawFound.Name, compatRawFound.Kind, upstreamFound.Name, upstreamFound.Kind)
	}
	compatRawModules := compatRawFound.Modules()
	if compatRawModules == nil {
		t.Fatalf("compat ToEntry(upstream module) Find(%q).Modules = nil, want goyang module set", path)
	}
	if imported, err := compatRawModules.FindModuleByNamespace("urn:compat-to-entry-find-imported"); err != nil || imported.Name != "compat-to-entry-find-imported" {
		t.Fatalf("compat ToEntry(upstream module) imported namespace lookup = (%#v,%v), want compat-to-entry-find-imported,nil", imported, err)
	}
}

func TestToEntryASTSubmoduleNamespaceMatchesGoyang(t *testing.T) {
	mainSource := `module compat-to-entry-submodule-main {
    namespace "urn:compat-to-entry-submodule-main";
    prefix ctesm;

    include compat-to-entry-submodule-child;
}
`
	submoduleSource := `submodule compat-to-entry-submodule-child {
    belongs-to compat-to-entry-submodule-main {
        prefix ctesm;
    }

    container from-submodule {
        leaf value { type string; }
    }
}
`
	dir := t.TempDir()
	writeYANG(t, filepath.Join(dir, "compat-to-entry-submodule-main.yang"), mainSource)
	writeYANG(t, filepath.Join(dir, "compat-to-entry-submodule-child.yang"), submoduleSource)

	upstreamModules := upstream.NewModules()
	upstreamModules.AddPath(dir)
	if err := upstreamModules.Read("compat-to-entry-submodule-main"); err != nil {
		t.Fatalf("upstream Read: %v", err)
	}
	if errs := upstreamModules.Process(); len(errs) != 0 {
		t.Fatalf("upstream Process: %v", errs)
	}
	rawSubmodule := upstreamModules.SubModules["compat-to-entry-submodule-child"]
	if rawSubmodule == nil {
		t.Fatal("upstream submodule not recorded")
	}
	compatEntry := compat.ToEntry(rawSubmodule)
	upstreamEntry := upstream.ToEntry(rawSubmodule)
	compatFound := compatEntry.Find("from-submodule/value")
	upstreamFound := upstreamEntry.Find("from-submodule/value")
	if compatFound == nil || upstreamFound == nil {
		t.Fatalf("Find(from-submodule/value) = (%#v,%#v), want both non-nil", compatFound, upstreamFound)
	}
	if got, want := valueName(compatFound.Namespace()), upstreamValueName(upstreamFound.Namespace()); got != want {
		t.Fatalf("submodule leaf Namespace = %q, want goyang %q", got, want)
	}
	compatModules := compatFound.Modules()
	if compatModules == nil {
		t.Fatal("submodule leaf Modules = nil, want goyang module set")
	}
	if parent, err := compatModules.FindModuleByNamespace("urn:compat-to-entry-submodule-main"); err != nil || parent.Name != "compat-to-entry-submodule-main" {
		t.Fatalf("submodule parent namespace lookup = (%#v,%v), want compat-to-entry-submodule-main,nil", parent, err)
	}
	gotModule, gotErr := compatFound.InstantiatingModule()
	wantModule, wantErr := upstreamFound.InstantiatingModule()
	if (gotErr != nil) != (wantErr != nil) || gotModule != wantModule {
		t.Fatalf("submodule leaf InstantiatingModule = (%q,%v), want goyang (%q,%v)", gotModule, gotErr, wantModule, wantErr)
	}
}

func TestMatchingEntryExtensionsSubmodulePrefixUsesOwnerModule(t *testing.T) {
	stmts, err := compat.Parse(`submodule compat-submodule-extension-child {
    belongs-to compat-submodule-extension-owner {
        prefix cse;
    }

    leaf tagged {
        cse:marker "submodule";
        type string;
    }
}
`, "compat-submodule-extension-child.yang")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	submodule := &compat.ASTModule{
		Name:      "compat-submodule-extension-child",
		BelongsTo: &compat.BelongsTo{Name: "compat-submodule-extension-owner", Prefix: &compat.ASTValue{Name: "cse"}},
		Source:    stmts[0],
	}
	tagged := compat.ToEntry(submodule).Find("tagged")
	if tagged == nil {
		t.Fatal("Find(tagged) = nil")
	}

	matches, err := compat.MatchingEntryExtensions(tagged, "compat-submodule-extension-owner", "marker")
	if err != nil {
		t.Fatalf("MatchingEntryExtensions(owner): %v", err)
	}
	if len(matches) != 1 || matches[0].Argument != "submodule" {
		t.Fatalf("MatchingEntryExtensions(owner) = %#v, want submodule marker", matches)
	}
	submoduleMatches, err := compat.MatchingEntryExtensions(tagged, "compat-submodule-extension-child", "marker")
	if err != nil {
		t.Fatalf("MatchingEntryExtensions(submodule): %v", err)
	}
	if len(submoduleMatches) != 0 {
		t.Fatalf("MatchingEntryExtensions(submodule) = %#v, want no matches", submoduleMatches)
	}
}

func TestToEntryASTMatchingEntryExtensionsMatchesGoyang(t *testing.T) {
	defSource := `module compat-to-entry-ext-def {
    namespace "urn:compat-to-entry-ext-def";
    prefix cteed;

    extension marker {
        argument value;
    }
}
`
	userSource := `module compat-to-entry-ext-user {
    namespace "urn:compat-to-entry-ext-user";
    prefix cteeu;

    import compat-to-entry-ext-def {
        prefix cteed;
    }

    container top {
        leaf tagged {
            cteed:marker "external";
            type string;
        }
    }
}
`
	upstreamModules := upstream.NewModules()
	for name, source := range map[string]string{
		"compat-to-entry-ext-def.yang":  defSource,
		"compat-to-entry-ext-user.yang": userSource,
	} {
		if err := upstreamModules.Parse(source, name); err != nil {
			t.Fatalf("upstream Parse(%s): %v", name, err)
		}
	}
	if errs := upstreamModules.Process(); len(errs) != 0 {
		t.Fatalf("upstream Process: %v", errs)
	}
	rawModule := upstreamModules.Modules["compat-to-entry-ext-user"]
	compatTagged := compat.ToEntry(rawModule).Find("top/tagged")
	upstreamTagged := upstream.ToEntry(rawModule).Find("top/tagged")
	if compatTagged == nil || upstreamTagged == nil {
		t.Fatalf("Find(top/tagged) = (%#v,%#v), want both non-nil", compatTagged, upstreamTagged)
	}
	compatMatches, compatErr := compat.MatchingEntryExtensions(compatTagged, "compat-to-entry-ext-def", "marker")
	upstreamMatches, upstreamErr := upstream.MatchingEntryExtensions(upstreamTagged, "compat-to-entry-ext-def", "marker")
	if compatErr != nil || upstreamErr != nil {
		t.Fatalf("MatchingEntryExtensions errors = (%v,%v), want both nil", compatErr, upstreamErr)
	}
	if len(compatMatches) != len(upstreamMatches) || len(compatMatches) != 1 {
		t.Fatalf("MatchingEntryExtensions = (%#v,%#v), want one match on both", compatMatches, upstreamMatches)
	}
	if compatMatches[0].Argument != upstreamMatches[0].Argument {
		t.Fatalf("MatchingEntryExtensions argument = %q, want goyang %q", compatMatches[0].Argument, upstreamMatches[0].Argument)
	}
}

func TestToEntryASTModulePropagatesUsesMetadataLikeGoyang(t *testing.T) {
	source := `module compat-to-entry-uses-metadata {
    namespace "urn:compat-to-entry-uses-metadata";
    prefix cteum;

    feature gate;

    grouping common {
        leaf grouped { type string; }
    }

    container top {
        uses common {
            if-feature gate;
            when "../enabled = 'true'";
            reference "use reference";
            status deprecated;
        }
    }
}
`
	upstreamModules := upstream.NewModules()
	if err := upstreamModules.Parse(source, "compat-to-entry-uses-metadata.yang"); err != nil {
		t.Fatalf("upstream Parse: %v", err)
	}
	if errs := upstreamModules.Process(); len(errs) != 0 {
		t.Fatalf("upstream Process: %v", errs)
	}
	rawModule := upstreamModules.Modules["compat-to-entry-uses-metadata"]
	compatLeaf := compat.ToEntry(rawModule).Find("top/grouped")
	upstreamLeaf := upstream.ToEntry(rawModule).Find("top/grouped")
	if compatLeaf == nil || upstreamLeaf == nil {
		t.Fatalf("Find(top/grouped) = (%#v,%#v), want both non-nil", compatLeaf, upstreamLeaf)
	}

	for _, key := range []string{"if-feature", "when", "reference", "status"} {
		got := extraNodeNames(compatLeaf.Extra, key)
		want := extraNodeNames(upstreamLeaf.Extra, key)
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("Extra[%q] = %v, want goyang %v", key, got, want)
		}
	}
}

func TestToEntryUsesNodeMatchesGoyang(t *testing.T) {
	source := `module compat-to-entry-uses-node {
    namespace "urn:compat-to-entry-uses-node";
    prefix cteun;

    grouping common {
        leaf grouped { type string; }
    }

    container top {
        uses common {
            when "../enabled = 'true'";
        }
    }
}
`
	upstreamModules := upstream.NewModules()
	if err := upstreamModules.Parse(source, "compat-to-entry-uses-node.yang"); err != nil {
		t.Fatalf("upstream Parse: %v", err)
	}
	if errs := upstreamModules.Process(); len(errs) != 0 {
		t.Fatalf("upstream Process: %v", errs)
	}
	uses := upstreamModules.Modules["compat-to-entry-uses-node"].Container[0].Uses[0]
	compatEntry := compat.ToEntry(uses)
	upstreamEntry := upstream.ToEntry(uses)
	if compatEntry == nil || upstreamEntry == nil {
		t.Fatalf("ToEntry(uses) = (%#v,%#v), want both non-nil", compatEntry, upstreamEntry)
	}
	if compatEntry.Name != upstreamEntry.Name || compatEntry.Kind.String() != upstreamEntry.Kind.String() {
		t.Fatalf("ToEntry(uses) = %s/%s, want goyang %s/%s", compatEntry.Name, compatEntry.Kind, upstreamEntry.Name, upstreamEntry.Kind)
	}
	compatLeaf := compatEntry.Find("grouped")
	upstreamLeaf := upstreamEntry.Find("grouped")
	if compatLeaf == nil || upstreamLeaf == nil {
		t.Fatalf("ToEntry(uses).Find(grouped) = (%#v,%#v), want both non-nil", compatLeaf, upstreamLeaf)
	}
	if got, want := extraNodeNames(compatEntry.Extra, "when"), extraNodeNames(upstreamEntry.Extra, "when"); !reflect.DeepEqual(got, want) {
		t.Fatalf("ToEntry(uses) Extra[when] = %v, want goyang %v", got, want)
	}
}

func TestToEntryGroupingNodeMatchesGoyang(t *testing.T) {
	source := `module compat-to-entry-grouping-node {
    namespace "urn:compat-to-entry-grouping-node";
    prefix ctegn;

    grouping nested {
        leaf nested-leaf { type string; }
    }
    grouping common {
        leaf grouped { type string; }
        uses nested;
    }
}
`
	upstreamModules := upstream.NewModules()
	if err := upstreamModules.Parse(source, "compat-to-entry-grouping-node.yang"); err != nil {
		t.Fatalf("upstream Parse: %v", err)
	}
	if errs := upstreamModules.Process(); len(errs) != 0 {
		t.Fatalf("upstream Process: %v", errs)
	}
	grouping := upstreamModules.Modules["compat-to-entry-grouping-node"].Grouping[1]
	compatEntry := compat.ToEntry(grouping)
	upstreamEntry := upstream.ToEntry(grouping)
	if compatEntry == nil || upstreamEntry == nil {
		t.Fatalf("ToEntry(grouping) = (%#v,%#v), want both non-nil", compatEntry, upstreamEntry)
	}
	if compatEntry.Name != upstreamEntry.Name || compatEntry.Kind.String() != upstreamEntry.Kind.String() {
		t.Fatalf("ToEntry(grouping) = %s/%s, want goyang %s/%s", compatEntry.Name, compatEntry.Kind, upstreamEntry.Name, upstreamEntry.Kind)
	}
	for _, path := range []string{"grouped", "nested-leaf"} {
		if compatEntry.Find(path) == nil || upstreamEntry.Find(path) == nil {
			t.Fatalf("ToEntry(grouping).Find(%q) = (%#v,%#v), want both non-nil", path, compatEntry.Find(path), upstreamEntry.Find(path))
		}
	}
	if got, want := childNames(compatEntry.Children()), []string{"grouped", "nested-leaf"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("ToEntry(grouping) children = %v, want %v", got, want)
	}
}

func TestToEntryASTModuleRecordsAugmentsAndDeviationsLikeGoyang(t *testing.T) {
	source := `module compat-to-entry-augment-deviation {
    namespace "urn:compat-to-entry-augment-deviation";
    prefix ctead;

    container top {
        leaf base {
            type string;
        }
    }

    augment "/top" {
        leaf added {
            type string;
        }
    }

    deviation "/top/base" {
        deviate add {
            default "fallback";
            units "widgets";
        }
    }
}
`
	upstreamModules := upstream.NewModules()
	if err := upstreamModules.Parse(source, "compat-to-entry-augment-deviation.yang"); err != nil {
		t.Fatalf("upstream Parse: %v", err)
	}
	rawModule := upstreamModules.Modules["compat-to-entry-augment-deviation"]
	compatEntry := compat.ToEntry(rawModule)
	upstreamEntry := upstream.ToEntry(rawModule)

	if got, want := len(compatEntry.Augments), len(upstreamEntry.Augments); got != want {
		t.Fatalf("ToEntry(module).Augments len = %d, want goyang %d", got, want)
	}
	if len(upstreamEntry.Augments) == 0 {
		t.Fatal("upstream ToEntry(module).Augments len = 0, test fixture does not exercise augment parity")
	}
	if compatEntry.Augments[0].Name != upstreamEntry.Augments[0].Name {
		t.Fatalf("ToEntry(module).Augments[0].Name = %q, want goyang %q", compatEntry.Augments[0].Name, upstreamEntry.Augments[0].Name)
	}
	if compatEntry.Augments[0].Find("added") == nil || upstreamEntry.Augments[0].Find("added") == nil {
		t.Fatalf("ToEntry(module).Augments[0].Find(added) = (%#v,%#v), want both non-nil", compatEntry.Augments[0].Find("added"), upstreamEntry.Augments[0].Find("added"))
	}

	if got, want := len(compatEntry.Deviations), len(upstreamEntry.Deviations); got != want {
		t.Fatalf("ToEntry(module).Deviations len = %d, want goyang %d", got, want)
	}
	if len(upstreamEntry.Deviations) == 0 {
		t.Fatal("upstream ToEntry(module).Deviations len = 0, test fixture does not exercise deviation parity")
	}
	if got, want := compatEntry.Deviations[0].DeviatedPath, upstreamEntry.Deviations[0].DeviatedPath; got != want {
		t.Fatalf("ToEntry(module).Deviations[0].DeviatedPath = %q, want goyang %q", got, want)
	}

	compatDeviation := compat.ToEntry(rawModule.Deviation[0])
	upstreamDeviation := upstream.ToEntry(rawModule.Deviation[0])
	if compatDeviation == nil || upstreamDeviation == nil {
		t.Fatalf("ToEntry(deviation) = (%#v,%#v), want both non-nil", compatDeviation, upstreamDeviation)
	}
	if got, want := compatDeviation.Name, upstreamDeviation.Name; got != want {
		t.Fatalf("ToEntry(deviation).Name = %q, want goyang %q", got, want)
	}
	compatDeviate := compatDeviation.Deviate[compat.DeviationAdd][0]
	upstreamDeviate := upstreamDeviation.Deviate[upstream.DeviationAdd][0]
	if got, want := compatDeviate.Kind.String(), upstreamDeviate.Kind.String(); got != want {
		t.Fatalf("ToEntry(deviate).Kind = %q, want goyang %q", got, want)
	}
	if got, want := compatDeviate.DefaultValues(), upstreamDeviate.Default; !reflect.DeepEqual(got, want) {
		t.Fatalf("ToEntry(deviate).Default = %v, want goyang %v", got, want)
	}
	if got, want := compatDeviate.Units, upstreamDeviate.Units; got != want {
		t.Fatalf("ToEntry(deviate).Units = %q, want goyang %q", got, want)
	}

	compatAugment := compat.ToEntry(rawModule.Augment[0])
	upstreamAugment := upstream.ToEntry(rawModule.Augment[0])
	if compatAugment == nil || upstreamAugment == nil {
		t.Fatalf("ToEntry(augment) = (%#v,%#v), want both non-nil", compatAugment, upstreamAugment)
	}
	if got, want := childNames(compatAugment.Children()), []string{"added"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("ToEntry(augment) children = %v, want %v", got, want)
	}
}

func TestToEntryDirectRPCAndModuleRPCIOMatchesGoyang(t *testing.T) {
	source := `module compat-to-entry-rpc-direct {
    namespace "urn:compat-to-entry-rpc-direct";
    prefix cterd;

    rpc ping;
    rpc reset {
        input {
            leaf target { type string; }
        }
        output {
            leaf ok { type boolean; }
        }
    }
}
`
	upstreamModules := upstream.NewModules()
	if err := upstreamModules.Parse(source, "compat-to-entry-rpc-direct.yang"); err != nil {
		t.Fatalf("upstream Parse: %v", err)
	}
	rawModule := upstreamModules.Modules["compat-to-entry-rpc-direct"]

	compatPing := compat.ToEntry(rawModule.RPC[0])
	upstreamPing := upstream.ToEntry(rawModule.RPC[0])
	if (compatPing.RPC == nil) != (upstreamPing.RPC == nil) {
		t.Fatalf("direct ToEntry(empty rpc).RPC nil = %v, want goyang %v", compatPing.RPC == nil, upstreamPing.RPC == nil)
	}

	compatRoot := compat.ToEntry(rawModule)
	upstreamRoot := upstream.ToEntry(rawModule)
	if (compatRoot.Find("ping/input") == nil) != (upstreamRoot.Find("ping/input") == nil) {
		t.Fatalf("module ToEntry Find(ping/input) nil = %v, want goyang %v", compatRoot.Find("ping/input") == nil, upstreamRoot.Find("ping/input") == nil)
	}
	compatReset := compat.ToEntry(rawModule.RPC[1])
	upstreamReset := upstream.ToEntry(rawModule.RPC[1])
	if (compatReset.RPC == nil) != (upstreamReset.RPC == nil) {
		t.Fatalf("direct ToEntry(rpc with io).RPC nil = %v, want goyang %v", compatReset.RPC == nil, upstreamReset.RPC == nil)
	}
	if compatReset.RPC == nil || compatReset.RPC.Input == nil || compatReset.RPC.Output == nil {
		t.Fatalf("direct ToEntry(rpc with io).RPC = %#v, want input/output", compatReset.RPC)
	}
}

func TestToEntryASTMetadataExtrasAndChoiceDefaultLikeGoyang(t *testing.T) {
	source := `module compat-to-entry-extra {
    namespace "urn:compat-to-entry-extra";
    prefix ctee;

    feature gate;

    container top {
        presence "enable top";
        if-feature gate;
        when "../enabled = 'true'";
        must "count(child) > 0";
        reference "top reference";
        status deprecated;

        list item {
            key "name";
            unique "serial";
            leaf name { type string; }
            leaf serial { type string; }
        }

        choice mode {
            default automatic;
            case automatic {
                leaf auto { type string; }
            }
            case manual {
                leaf manual { type string; }
            }
        }
    }
}
`
	upstreamModules := upstream.NewModules()
	if err := upstreamModules.Parse(source, "compat-to-entry-extra.yang"); err != nil {
		t.Fatalf("upstream Parse: %v", err)
	}
	rawModule := upstreamModules.Modules["compat-to-entry-extra"]
	compatEntry := compat.ToEntry(rawModule)
	upstreamEntry := upstream.ToEntry(rawModule)

	compatTop := compatEntry.Find("top")
	upstreamTop := upstreamEntry.Find("top")
	if compatTop == nil || upstreamTop == nil {
		t.Fatalf("Find(top) = (%#v,%#v), want both non-nil", compatTop, upstreamTop)
	}
	for _, key := range []string{"if-feature", "must", "presence", "reference", "status", "when"} {
		got := extraNodeNames(compatTop.Extra, key)
		want := extraNodeNames(upstreamTop.Extra, key)
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("top Extra[%q] = %v, want goyang %v", key, got, want)
		}
	}
	gotWhen, gotOK := compatTop.GetWhenXPath()
	wantWhen, wantOK := upstreamTop.GetWhenXPath()
	if gotWhen != wantWhen || gotOK != wantOK {
		t.Fatalf("top GetWhenXPath = (%q,%v), want goyang (%q,%v)", gotWhen, gotOK, wantWhen, wantOK)
	}

	compatList := compatEntry.Find("top/item")
	upstreamList := upstreamEntry.Find("top/item")
	if compatList == nil || upstreamList == nil {
		t.Fatalf("Find(top/item) = (%#v,%#v), want both non-nil", compatList, upstreamList)
	}
	if got, want := extraNodeNames(compatList.Extra, "unique"), extraNodeNames(upstreamList.Extra, "unique"); !reflect.DeepEqual(got, want) {
		t.Fatalf("list Extra[unique] = %v, want goyang %v", got, want)
	}

	compatChoice := compatEntry.Find("top/mode")
	upstreamChoice := upstreamEntry.Find("top/mode")
	if compatChoice == nil || upstreamChoice == nil {
		t.Fatalf("Find(top/mode) = (%#v,%#v), want both non-nil", compatChoice, upstreamChoice)
	}
	if got, want := compatChoice.DefaultValues(), upstreamChoice.Default; !reflect.DeepEqual(got, want) {
		t.Fatalf("choice defaults = %v, want goyang %v", got, want)
	}
}

func TestToEntryAnydataAnyxmlDirectoryShapeMatchesGoyang(t *testing.T) {
	source := `module compat-to-entry-any {
    yang-version 1.1;
    namespace "urn:compat-to-entry-any";
    prefix ctea;

    anydata payload;
    anyxml markup;
}
`
	upstreamModules := upstream.NewModules()
	if err := upstreamModules.Parse(source, "compat-to-entry-any.yang"); err != nil {
		t.Fatalf("upstream Parse: %v", err)
	}
	rawModule := upstreamModules.Modules["compat-to-entry-any"]
	for _, tt := range []struct {
		name string
		node upstream.Node
	}{
		{name: "anydata", node: rawModule.Anydata[0]},
		{name: "anyxml", node: rawModule.Anyxml[0]},
	} {
		t.Run(tt.name, func(t *testing.T) {
			compatEntry := compat.ToEntry(tt.node)
			upstreamEntry := upstream.ToEntry(tt.node)
			if compatEntry == nil || upstreamEntry == nil {
				t.Fatalf("ToEntry(%s) = (%#v,%#v), want both non-nil", tt.name, compatEntry, upstreamEntry)
			}
			if compatEntry.Kind.String() != upstreamEntry.Kind.String() {
				t.Fatalf("ToEntry(%s).Kind = %q, want goyang %q", tt.name, compatEntry.Kind, upstreamEntry.Kind)
			}
			if compatEntry.IsDir() != upstreamEntry.IsDir() {
				t.Fatalf("ToEntry(%s).IsDir = %v, want goyang %v", tt.name, compatEntry.IsDir(), upstreamEntry.IsDir())
			}
		})
	}
}

func TestToEntryASTMalformedScalarsReportErrorsLikeGoyang(t *testing.T) {
	source := `module compat-to-entry-invalid-scalars {
    namespace "urn:compat-to-entry-invalid-scalars";
    prefix cteis;

    container top {
        config maybe;

        leaf required {
            type string;
            mandatory maybe;
        }

        list bad-list {
            key "name";
            min-elements "bogus";
            max-elements 0;
            leaf name { type string; }
        }
    }
}
`
	upstreamModules := upstream.NewModules()
	if err := upstreamModules.Parse(source, "compat-to-entry-invalid-scalars.yang"); err != nil {
		t.Fatalf("upstream Parse: %v", err)
	}
	rawModule := upstreamModules.Modules["compat-to-entry-invalid-scalars"]
	compatErrors := compat.ToEntry(rawModule).GetErrors()
	upstreamErrors := upstream.ToEntry(rawModule).GetErrors()
	if len(upstreamErrors) == 0 {
		t.Fatal("upstream ToEntry errors = empty, test fixture does not exercise malformed scalar parity")
	}
	if len(compatErrors) != len(upstreamErrors) {
		t.Fatalf("compat ToEntry errors len = %d, want goyang %d\ncompat: %v\ngoyang: %v", len(compatErrors), len(upstreamErrors), compatErrors, upstreamErrors)
	}
}

func TestToEntryASTTypedefChainResolvesLikeGoyang(t *testing.T) {
	source := `module compat-to-entry-typedef-chain {
    namespace "urn:compat-to-entry-typedef-chain";
    prefix ctetc;

    typedef inner {
        type uint8;
        default "7";
        units "widgets";
    }
    typedef middle {
        type inner;
    }
    typedef outer {
        type middle;
    }

    container top {
        leaf value {
            type outer;
        }
    }
}
`
	upstreamModules := upstream.NewModules()
	if err := upstreamModules.Parse(source, "compat-to-entry-typedef-chain.yang"); err != nil {
		t.Fatalf("upstream Parse: %v", err)
	}
	rawModule := upstreamModules.Modules["compat-to-entry-typedef-chain"]
	compatLeaf := compat.ToEntry(rawModule).Find("top/value")
	upstreamLeaf := upstream.ToEntry(rawModule).Find("top/value")
	if compatLeaf == nil || upstreamLeaf == nil {
		t.Fatalf("Find(top/value) = (%#v,%#v), want both non-nil", compatLeaf, upstreamLeaf)
	}
	if got, want := compatTypeBehaviorSummary(compatLeaf.Type), upstreamTypeBehaviorSummary(upstreamLeaf.Type); !reflect.DeepEqual(got, want) {
		t.Fatalf("raw AST typedef type summary = %#v, want goyang %#v", got, want)
	}
	if got, want := compatYangTypeBaseChain(compatLeaf.Type), upstreamYangTypeBaseChain(upstreamLeaf.Type); !reflect.DeepEqual(got, want) {
		t.Fatalf("raw AST typedef Base chain = %v, want goyang %v", got, want)
	}
}

func TestToEntryASTImportedTypedefResolvesLikeGoyang(t *testing.T) {
	importSource := `module compat-to-entry-typedef-imported {
    namespace "urn:compat-to-entry-typedef-imported";
    prefix cteti;

    typedef imported-id {
        type uint16;
        default "42";
        units "widgets";
    }
}
`
	mainSource := `module compat-to-entry-typedef-import-main {
    namespace "urn:compat-to-entry-typedef-import-main";
    prefix ctetm;

    import compat-to-entry-typedef-imported {
        prefix cteti;
    }

    leaf value {
        type cteti:imported-id;
    }
}
`
	upstreamModules := upstream.NewModules()
	for name, source := range map[string]string{
		"compat-to-entry-typedef-imported.yang":    importSource,
		"compat-to-entry-typedef-import-main.yang": mainSource,
	} {
		if err := upstreamModules.Parse(source, name); err != nil {
			t.Fatalf("upstream Parse(%s): %v", name, err)
		}
	}
	if errs := upstreamModules.Process(); len(errs) != 0 {
		t.Fatalf("upstream Process: %v", errs)
	}
	rawModule := upstreamModules.Modules["compat-to-entry-typedef-import-main"]
	compatLeaf := compat.ToEntry(rawModule).Find("value")
	upstreamLeaf := upstream.ToEntry(rawModule).Find("value")
	if compatLeaf == nil || upstreamLeaf == nil {
		t.Fatalf("Find(value) = (%#v,%#v), want both non-nil", compatLeaf, upstreamLeaf)
	}
	if got, want := compatTypeBehaviorSummary(compatLeaf.Type), upstreamTypeBehaviorSummary(upstreamLeaf.Type); !reflect.DeepEqual(got, want) {
		t.Fatalf("raw AST imported typedef type summary = %#v, want goyang %#v", got, want)
	}
	if got, want := compatYangTypeBaseChain(compatLeaf.Type), upstreamYangTypeBaseChain(upstreamLeaf.Type); !reflect.DeepEqual(got, want) {
		t.Fatalf("raw AST imported typedef Base chain = %v, want goyang %v", got, want)
	}
}

func TestToEntryCompatASTImportedTypedefResolvesLikeGoyang(t *testing.T) {
	importSource := `module compat-ast-typedef-imported {
    namespace "urn:compat-ast-typedef-imported";
    prefix cati;

    typedef imported-id {
        type uint16;
        default "42";
        units "widgets";
    }
}
`
	mainSource := `module compat-ast-typedef-import-main {
    namespace "urn:compat-ast-typedef-import-main";
    prefix catm;

    import compat-ast-typedef-imported {
        prefix cati;
    }

    leaf value {
        type cati:imported-id;
    }
}
`
	compatModules := compat.NewModules()
	upstreamModules := upstream.NewModules()
	for name, source := range map[string]string{
		"compat-ast-typedef-imported.yang":    importSource,
		"compat-ast-typedef-import-main.yang": mainSource,
	} {
		if err := compatModules.Parse(source, name); err != nil {
			t.Fatalf("compat Parse(%s): %v", name, err)
		}
		if err := upstreamModules.Parse(source, name); err != nil {
			t.Fatalf("upstream Parse(%s): %v", name, err)
		}
	}
	if errs := compatModules.Process(); len(errs) != 0 {
		t.Fatalf("compat Process: %v", errs)
	}
	if errs := upstreamModules.Process(); len(errs) != 0 {
		t.Fatalf("upstream Process: %v", errs)
	}

	compatLeaf := compat.ToEntry(compatModules.Modules["compat-ast-typedef-import-main"]).Find("value")
	upstreamLeaf := upstream.ToEntry(upstreamModules.Modules["compat-ast-typedef-import-main"]).Find("value")
	if compatLeaf == nil || upstreamLeaf == nil {
		t.Fatalf("Find(value) = (%#v,%#v), want both non-nil", compatLeaf, upstreamLeaf)
	}
	if got, want := compatTypeBehaviorSummary(compatLeaf.Type), upstreamTypeBehaviorSummary(upstreamLeaf.Type); !reflect.DeepEqual(got, want) {
		t.Fatalf("compat AST imported typedef type summary = %#v, want goyang %#v", got, want)
	}
	if got, want := compatYangTypeBaseChain(compatLeaf.Type), upstreamYangTypeBaseChain(upstreamLeaf.Type); !reflect.DeepEqual(got, want) {
		t.Fatalf("compat AST imported typedef Base chain = %v, want goyang %v", got, want)
	}
}

func TestToEntryCompatASTIncludedTypedefResolvesLikeGoyang(t *testing.T) {
	parentSource := `module compat-ast-typedef-include-main {
    namespace "urn:compat-ast-typedef-include-main";
    prefix catim;

    include compat-ast-typedef-include-sub;

    leaf value {
        type included-id;
    }
}
`
	submoduleSource := `submodule compat-ast-typedef-include-sub {
    belongs-to compat-ast-typedef-include-main {
        prefix catim;
    }

    typedef included-id {
        type uint32;
        default "99";
        units "widgets";
    }
}
`
	compatModules := compat.NewModules()
	upstreamModules := upstream.NewModules()
	for name, source := range map[string]string{
		"compat-ast-typedef-include-main.yang": parentSource,
		"compat-ast-typedef-include-sub.yang":  submoduleSource,
	} {
		if err := compatModules.Parse(source, name); err != nil {
			t.Fatalf("compat Parse(%s): %v", name, err)
		}
		if err := upstreamModules.Parse(source, name); err != nil {
			t.Fatalf("upstream Parse(%s): %v", name, err)
		}
	}
	if errs := compatModules.Process(); len(errs) != 0 {
		t.Fatalf("compat Process: %v", errs)
	}
	if errs := upstreamModules.Process(); len(errs) != 0 {
		t.Fatalf("upstream Process: %v", errs)
	}

	compatLeaf := compat.ToEntry(compatModules.Modules["compat-ast-typedef-include-main"]).Find("value")
	upstreamLeaf := upstream.ToEntry(upstreamModules.Modules["compat-ast-typedef-include-main"]).Find("value")
	if compatLeaf == nil || upstreamLeaf == nil {
		t.Fatalf("Find(value) = (%#v,%#v), want both non-nil", compatLeaf, upstreamLeaf)
	}
	if got, want := compatTypeBehaviorSummary(compatLeaf.Type), upstreamTypeBehaviorSummary(upstreamLeaf.Type); !reflect.DeepEqual(got, want) {
		t.Fatalf("compat AST included typedef type summary = %#v, want goyang %#v", got, want)
	}
	if got, want := compatYangTypeBaseChain(compatLeaf.Type), upstreamYangTypeBaseChain(upstreamLeaf.Type); !reflect.DeepEqual(got, want) {
		t.Fatalf("compat AST included typedef Base chain = %v, want goyang %v", got, want)
	}
}

func TestToEntryCompatASTImportedIdentityRefResolvesLikeGoyang(t *testing.T) {
	importSource := `module compat-ast-identity-imported {
    namespace "urn:compat-ast-identity-imported";
    prefix caii;

    identity base;
    identity derived {
        base base;
    }
}
`
	mainSource := `module compat-ast-identity-import-main {
    namespace "urn:compat-ast-identity-import-main";
    prefix caim;

    import compat-ast-identity-imported {
        prefix caii;
    }

    leaf value {
        type identityref {
            base caii:base;
        }
    }
}
`
	compatModules := compat.NewModules()
	upstreamModules := upstream.NewModules()
	for name, source := range map[string]string{
		"compat-ast-identity-imported.yang":    importSource,
		"compat-ast-identity-import-main.yang": mainSource,
	} {
		if err := compatModules.Parse(source, name); err != nil {
			t.Fatalf("compat Parse(%s): %v", name, err)
		}
		if err := upstreamModules.Parse(source, name); err != nil {
			t.Fatalf("upstream Parse(%s): %v", name, err)
		}
	}
	if errs := compatModules.Process(); len(errs) != 0 {
		t.Fatalf("compat Process: %v", errs)
	}
	if errs := upstreamModules.Process(); len(errs) != 0 {
		t.Fatalf("upstream Process: %v", errs)
	}

	compatLeaf := compat.ToEntry(compatModules.Modules["compat-ast-identity-import-main"]).Find("value")
	upstreamLeaf := upstream.ToEntry(upstreamModules.Modules["compat-ast-identity-import-main"]).Find("value")
	if compatLeaf == nil || upstreamLeaf == nil {
		t.Fatalf("Find(value) = (%#v,%#v), want both non-nil", compatLeaf, upstreamLeaf)
	}
	if got, want := compatTypeBehaviorSummary(compatLeaf.Type), upstreamTypeBehaviorSummary(upstreamLeaf.Type); !reflect.DeepEqual(got, want) {
		t.Fatalf("compat AST imported identityref type summary = %#v, want goyang %#v", got, want)
	}
	if compatLeaf.Type.IdentityBase == nil || upstreamLeaf.Type.IdentityBase == nil {
		t.Fatalf("IdentityBase = (%#v,%#v), want both non-nil", compatLeaf.Type.IdentityBase, upstreamLeaf.Type.IdentityBase)
	}
	if got, want := compatLeaf.Type.IdentityBase.PrefixedName(), upstreamLeaf.Type.IdentityBase.PrefixedName(); got != want {
		t.Fatalf("IdentityBase.PrefixedName = %q, want goyang %q", got, want)
	}
	if got, want := compatLeaf.Type.IdentityBase.IsDefined("derived"), upstreamLeaf.Type.IdentityBase.IsDefined("derived"); got != want {
		t.Fatalf("IdentityBase.IsDefined(derived) = %v, want goyang %v", got, want)
	}
	compatDerived := compatLeaf.Type.IdentityBase.GetValue("derived")
	upstreamDerived := upstreamLeaf.Type.IdentityBase.GetValue("derived")
	if (compatDerived == nil) != (upstreamDerived == nil) {
		t.Fatalf("IdentityBase.GetValue(derived) nil = %v, want goyang %v", compatDerived == nil, upstreamDerived == nil)
	}
	if compatDerived != nil && (compatDerived.Name != upstreamDerived.Name || compatDerived.PrefixedName() != upstreamDerived.PrefixedName()) {
		t.Fatalf("IdentityBase.GetValue(derived) = %s/%s, want goyang %s/%s", compatDerived.Name, compatDerived.PrefixedName(), upstreamDerived.Name, upstreamDerived.PrefixedName())
	}
}

func TestToEntryASTRichTypeMetadataMatchesGoyang(t *testing.T) {
	source := `module compat-to-entry-rich-types {
    yang-version 1.1;
    namespace "urn:compat-to-entry-rich-types";
    prefix ctert;

    identity transport;
    identity tcp {
        base transport;
    }

    container top {
        leaf ranged-int {
            type int32 {
                range "1..10 | 20..max";
            }
        }
        leaf ranged-decimal {
            type decimal64 {
                fraction-digits 2;
                range "0.00..10.00";
            }
        }
        leaf bounded-string {
            type string {
                length "2..8";
                pattern "[a-z]+";
                pattern "[0-9]+";
            }
        }
        leaf bounded-binary {
            type binary {
                length "4..16";
            }
        }
        leaf enum-child {
            type enumeration {
                enum down { value 2; }
                enum up { value 5; }
            }
        }
        leaf bits-child {
            type bits {
                bit read { position 0; }
                bit write { position 3; }
            }
        }
        leaf identity-child {
            type identityref {
                base transport;
            }
        }
    }
}
`
	upstreamModules := upstream.NewModules()
	if err := upstreamModules.Parse(source, "compat-to-entry-rich-types.yang"); err != nil {
		t.Fatalf("upstream Parse: %v", err)
	}
	if errs := upstreamModules.Process(); len(errs) != 0 {
		t.Fatalf("upstream Process: %v", errs)
	}
	rawModule := upstreamModules.Modules["compat-to-entry-rich-types"]
	compatRoot := compat.ToEntry(rawModule)
	upstreamRoot := upstream.ToEntry(rawModule)
	for _, path := range []string{
		"top/ranged-int",
		"top/ranged-decimal",
		"top/bounded-string",
		"top/bounded-binary",
		"top/enum-child",
		"top/bits-child",
		"top/identity-child",
	} {
		compatLeaf := compatRoot.Find(path)
		upstreamLeaf := upstreamRoot.Find(path)
		if compatLeaf == nil || upstreamLeaf == nil {
			t.Fatalf("Find(%s) = (%#v,%#v), want both non-nil", path, compatLeaf, upstreamLeaf)
		}
		if got, want := compatTypeBehaviorSummary(compatLeaf.Type), upstreamTypeBehaviorSummary(upstreamLeaf.Type); !reflect.DeepEqual(got, want) {
			t.Fatalf("%s raw AST type summary = %#v, want goyang %#v", path, got, want)
		}
	}
}

func TestToEntryASTModuleCyclicUsesFailsClosed(t *testing.T) {
	source := `module compat-to-entry-cyclic-uses {
    namespace "urn:compat-to-entry-cyclic-uses";
    prefix ctecu;

    grouping one {
        uses two;
    }
    grouping two {
        uses one;
    }

    container top {
        uses one;
    }
}
`
	compatModules := compat.NewModules()
	if err := compatModules.Parse(source, "compat-to-entry-cyclic-uses.yang"); err != nil {
		t.Fatalf("compat Parse: %v", err)
	}
	module := compatModules.Modules["compat-to-entry-cyclic-uses"]
	if module == nil {
		t.Fatal("compat module not recorded")
	}
	entry := compat.ToEntry(module)
	if entry == nil {
		t.Fatal("ToEntry returned nil")
	}
	if got := entry.GetErrors(); len(got) == 0 {
		t.Fatal("ToEntry cyclic uses errors = empty, want fail-closed diagnostic")
	}
}

func extraNodeNames(extra map[string][]interface{}, key string) []string {
	var names []string
	for _, item := range extra[key] {
		if node, ok := item.(interface{ NName() string }); ok {
			names = append(names, node.NName())
			continue
		}
		names = append(names, "")
	}
	return names
}

func mustFindCompatNode(t *testing.T, node compat.Node, path string) compat.Node {
	t.Helper()
	found, err := compat.FindNode(node, path)
	if err != nil {
		t.Fatalf("compat FindNode(%q): %v", path, err)
	}
	if found == nil {
		t.Fatalf("compat FindNode(%q) returned nil", path)
	}
	return found
}

func mustFindUpstreamNode(t *testing.T, node upstream.Node, path string) upstream.Node {
	t.Helper()
	found, err := upstream.FindNode(node, path)
	if err != nil {
		t.Fatalf("upstream FindNode(%q): %v", path, err)
	}
	if found == nil {
		t.Fatalf("upstream FindNode(%q) returned nil", path)
	}
	return found
}

func TestASTAliasesAndHelpers(t *testing.T) {
	mod := &compat.ASTModule{
		Name:   "compat-ast-demo",
		Prefix: &compat.ASTValue{Name: "cad"},
		Grouping: []*compat.Grouping{
			{Name: "shared"},
		},
	}
	leaf := &compat.Leaf{
		Name:   "value",
		Parent: mod,
		Extensions: []*compat.Statement{
			{Keyword: "cad:marker", Argument: "enabled", HasArgument: true},
		},
	}

	var node compat.Node = leaf
	root := compat.RootNode(node)
	if root == nil || root.Name != "compat-ast-demo" || root.GetPrefix() != "cad" {
		t.Fatalf("RootNode = %#v, want compat module compat-ast-demo with prefix cad", root)
	}
	prefixed := compat.FindModuleByPrefix(node, "cad")
	if prefixed == nil || prefixed.Name != "compat-ast-demo" || prefixed.GetPrefix() != "cad" {
		t.Fatalf("FindModuleByPrefix = %#v, want compat module compat-ast-demo with prefix cad", prefixed)
	}
	if got := compat.FindGrouping(node, "shared", map[string]bool{}); got == nil || got.Name != "shared" {
		t.Fatalf("FindGrouping = %#v, want shared grouping", got)
	}
	matches, err := compat.MatchingExtensions(node, "compat-ast-demo", "marker")
	if err != nil {
		t.Fatalf("MatchingExtensions: %v", err)
	}
	if len(matches) != 1 || matches[0].Argument != "enabled" {
		t.Fatalf("MatchingExtensions = %#v, want enabled marker", matches)
	}
	identity := &compat.ASTIdentity{Name: "transport", Parent: mod}
	if got := identity.Kind(); got != "identity" {
		t.Fatalf("ASTIdentity Kind = %q, want identity", got)
	}
	if got := compat.RootNode(identity); got == nil || got.Name != "compat-ast-demo" {
		t.Fatalf("ASTIdentity RootNode = %#v, want compat module", got)
	}
	var _ compat.Typedefer = mod
	var _ compat.Typedefer = root
}

func TestASTModuleIdentityCloneParentsValueChildren(t *testing.T) {
	mod := &compat.ASTModule{
		Name:   "compat-ast-identity",
		Prefix: &compat.ASTValue{Name: "cai"},
	}
	mod.Identity = []*compat.ASTIdentity{{
		Name:        "local-id",
		Parent:      mod,
		Base:        []*compat.ASTValue{{Name: "base-id", Description: &compat.ASTValue{Name: "Base description."}}},
		Description: &compat.ASTValue{Name: "Local identity."},
		IfFeature:   []*compat.ASTValue{{Name: "gate"}},
		Reference:   &compat.ASTValue{Name: "Identity reference."},
		Status:      &compat.ASTValue{Name: "current"},
	}}
	leaf := &compat.Leaf{Name: "value", Parent: mod}

	root := compat.RootNode(leaf)
	if root == nil {
		t.Fatal("RootNode returned nil")
	}
	if got, want := len(root.Identity), 1; got != want {
		t.Fatalf("RootNode Identity len = %d, want %d", got, want)
	}
	identity := root.Identity[0]
	if got := identity.ParentNode(); got != root {
		t.Fatalf("Identity ParentNode = %#v, want root module", got)
	}
	if got := identity.Description.ParentNode(); got != identity {
		t.Fatalf("Identity.Description ParentNode = %#v, want cloned identity", got)
	}
	if got := identity.Reference.ParentNode(); got != identity {
		t.Fatalf("Identity.Reference ParentNode = %#v, want cloned identity", got)
	}
	if got := identity.Status.ParentNode(); got != identity {
		t.Fatalf("Identity.Status ParentNode = %#v, want cloned identity", got)
	}
	if got, want := len(identity.Base), 1; got != want {
		t.Fatalf("Identity.Base len = %d, want %d", got, want)
	}
	if got := identity.Base[0].ParentNode(); got != identity {
		t.Fatalf("Identity.Base[0] ParentNode = %#v, want cloned identity", got)
	}
	if got := identity.Base[0].Description.ParentNode(); got != identity.Base[0] {
		t.Fatalf("Identity.Base[0].Description ParentNode = %#v, want cloned base value", got)
	}
	if got, want := len(identity.IfFeature), 1; got != want {
		t.Fatalf("Identity.IfFeature len = %d, want %d", got, want)
	}
	if got := identity.IfFeature[0].ParentNode(); got != identity {
		t.Fatalf("Identity.IfFeature[0] ParentNode = %#v, want cloned identity", got)
	}
}

func TestASTModuleDefinitionCloneParentsValueChildren(t *testing.T) {
	mod := &compat.ASTModule{
		Name:   "compat-ast-definitions",
		Prefix: &compat.ASTValue{Name: "cad"},
	}
	mod.Typedef = []*compat.Typedef{{
		Name:        "local-type",
		Default:     &compat.ASTValue{Name: "fallback", Description: &compat.ASTValue{Name: "Default description."}},
		Description: &compat.ASTValue{Name: "Typedef description."},
		Reference:   &compat.ASTValue{Name: "Typedef reference."},
		Status:      &compat.ASTValue{Name: "current"},
		Type:        &compat.Type{Name: "string"},
		Units:       &compat.ASTValue{Name: "widgets"},
	}}
	mod.Grouping = []*compat.Grouping{{
		Name:        "shared",
		Description: &compat.ASTValue{Name: "Grouping description."},
		Leaf: []*compat.Leaf{{
			Name: "shared-leaf",
			Type: &compat.Type{Name: "string"},
		}},
		Reference: &compat.ASTValue{Name: "Grouping reference."},
		Status:    &compat.ASTValue{Name: "current"},
	}}
	leaf := &compat.Leaf{Name: "value", Parent: mod}

	root := compat.RootNode(leaf)
	if root == nil {
		t.Fatal("RootNode returned nil")
	}
	if got, want := len(root.Typedef), 1; got != want {
		t.Fatalf("RootNode Typedef len = %d, want %d", got, want)
	}
	typedef := root.Typedef[0]
	if got := typedef.ParentNode(); got != root {
		t.Fatalf("Typedef ParentNode = %#v, want root module", got)
	}
	if typedef.Type == nil {
		t.Fatal("Typedef.Type = nil, want cloned type")
	}
	if got := typedef.Type.ParentNode(); got != typedef {
		t.Fatalf("Typedef.Type ParentNode = %#v, want cloned typedef", got)
	}
	if got := typedef.Default.ParentNode(); got != typedef {
		t.Fatalf("Typedef.Default ParentNode = %#v, want cloned typedef", got)
	}
	if got := typedef.Description.ParentNode(); got != typedef {
		t.Fatalf("Typedef.Description ParentNode = %#v, want cloned typedef", got)
	}
	if got := typedef.Reference.ParentNode(); got != typedef {
		t.Fatalf("Typedef.Reference ParentNode = %#v, want cloned typedef", got)
	}
	if got := typedef.Status.ParentNode(); got != typedef {
		t.Fatalf("Typedef.Status ParentNode = %#v, want cloned typedef", got)
	}
	if got := typedef.Units.ParentNode(); got != typedef {
		t.Fatalf("Typedef.Units ParentNode = %#v, want cloned typedef", got)
	}

	if got, want := len(root.Grouping), 1; got != want {
		t.Fatalf("RootNode Grouping len = %d, want %d", got, want)
	}
	grouping := root.Grouping[0]
	if got := grouping.ParentNode(); got != root {
		t.Fatalf("Grouping ParentNode = %#v, want root module", got)
	}
	if got := grouping.Description.ParentNode(); got != grouping {
		t.Fatalf("Grouping.Description ParentNode = %#v, want cloned grouping", got)
	}
	if got, want := len(grouping.Leaf), 1; got != want {
		t.Fatalf("Grouping.Leaf len = %d, want %d", got, want)
	}
	groupingLeaf := grouping.Leaf[0]
	if got := groupingLeaf.ParentNode(); got != grouping {
		t.Fatalf("Grouping.Leaf[0] ParentNode = %#v, want cloned grouping", got)
	}
	if groupingLeaf.Type == nil {
		t.Fatal("Grouping.Leaf[0].Type = nil, want cloned type")
	}
	if got := groupingLeaf.Type.ParentNode(); got != groupingLeaf {
		t.Fatalf("Grouping.Leaf[0].Type ParentNode = %#v, want cloned leaf", got)
	}
	if got := grouping.Reference.ParentNode(); got != grouping {
		t.Fatalf("Grouping.Reference ParentNode = %#v, want cloned grouping", got)
	}
	if got := grouping.Status.ParentNode(); got != grouping {
		t.Fatalf("Grouping.Status ParentNode = %#v, want cloned grouping", got)
	}
}

func TestASTModuleLeafCloneParentsValueChildren(t *testing.T) {
	mod := &compat.ASTModule{
		Name:   "compat-ast-leaf",
		Prefix: &compat.ASTValue{Name: "cal"},
	}
	mod.Leaf = []*compat.Leaf{{
		Name:        "value",
		Default:     &compat.ASTValue{Name: "fallback", Description: &compat.ASTValue{Name: "Default description."}},
		Description: &compat.ASTValue{Name: "Leaf description."},
		IfFeature:   []*compat.ASTValue{{Name: "gate"}},
		Mandatory:   &compat.ASTValue{Name: "true"},
		Must: []*compat.Must{{
			Name:         ". != ''",
			ErrorMessage: &compat.ASTValue{Name: "value required"},
		}},
		Reference: &compat.ASTValue{Name: "Leaf reference."},
		Status:    &compat.ASTValue{Name: "current"},
		Type:      &compat.Type{Name: "string"},
		Units:     &compat.ASTValue{Name: "widgets"},
		When:      &compat.ASTValue{Name: "../enabled"},
	}}
	leafNode := &compat.Leaf{Name: "probe", Parent: mod}

	root := compat.RootNode(leafNode)
	if root == nil {
		t.Fatal("RootNode returned nil")
	}
	if got, want := len(root.Leaf), 1; got != want {
		t.Fatalf("RootNode Leaf len = %d, want %d", got, want)
	}
	leaf := root.Leaf[0]
	if got := leaf.ParentNode(); got != root {
		t.Fatalf("Leaf ParentNode = %#v, want root module", got)
	}
	if got := leaf.Default.ParentNode(); got != leaf {
		t.Fatalf("Leaf.Default ParentNode = %#v, want cloned leaf", got)
	}
	if got := leaf.Default.Description.ParentNode(); got != leaf.Default {
		t.Fatalf("Leaf.Default.Description ParentNode = %#v, want cloned default value", got)
	}
	if got := leaf.Description.ParentNode(); got != leaf {
		t.Fatalf("Leaf.Description ParentNode = %#v, want cloned leaf", got)
	}
	if got, want := len(leaf.IfFeature), 1; got != want {
		t.Fatalf("Leaf.IfFeature len = %d, want %d", got, want)
	}
	if got := leaf.IfFeature[0].ParentNode(); got != leaf {
		t.Fatalf("Leaf.IfFeature[0] ParentNode = %#v, want cloned leaf", got)
	}
	if got := leaf.Mandatory.ParentNode(); got != leaf {
		t.Fatalf("Leaf.Mandatory ParentNode = %#v, want cloned leaf", got)
	}
	if got, want := len(leaf.Must), 1; got != want {
		t.Fatalf("Leaf.Must len = %d, want %d", got, want)
	}
	if got := leaf.Must[0].ParentNode(); got != leaf {
		t.Fatalf("Leaf.Must[0] ParentNode = %#v, want cloned leaf", got)
	}
	if got := leaf.Must[0].ErrorMessage.ParentNode(); got != leaf.Must[0] {
		t.Fatalf("Leaf.Must[0].ErrorMessage ParentNode = %#v, want cloned must", got)
	}
	if got := leaf.Reference.ParentNode(); got != leaf {
		t.Fatalf("Leaf.Reference ParentNode = %#v, want cloned leaf", got)
	}
	if got := leaf.Status.ParentNode(); got != leaf {
		t.Fatalf("Leaf.Status ParentNode = %#v, want cloned leaf", got)
	}
	if leaf.Type == nil {
		t.Fatal("Leaf.Type = nil, want cloned type")
	}
	if got := leaf.Type.ParentNode(); got != leaf {
		t.Fatalf("Leaf.Type ParentNode = %#v, want cloned leaf", got)
	}
	if got := leaf.Units.ParentNode(); got != leaf {
		t.Fatalf("Leaf.Units ParentNode = %#v, want cloned leaf", got)
	}
	if got := leaf.When.ParentNode(); got != leaf {
		t.Fatalf("Leaf.When ParentNode = %#v, want cloned leaf", got)
	}
}

func TestASTModuleLeafListCloneParentsValueChildren(t *testing.T) {
	mod := &compat.ASTModule{
		Name:   "compat-ast-leaf-list",
		Prefix: &compat.ASTValue{Name: "call"},
	}
	mod.LeafList = []*compat.LeafList{{
		Name:        "tags",
		Config:      &compat.ASTValue{Name: "true"},
		Default:     []*compat.ASTValue{{Name: "blue"}, {Name: "green"}},
		Description: &compat.ASTValue{Name: "Leaf-list description."},
		IfFeature:   []*compat.ASTValue{{Name: "gate"}},
		MaxElements: &compat.ASTValue{Name: "3"},
		MinElements: &compat.ASTValue{Name: "1"},
		Must: []*compat.Must{{
			Name:         ". != ''",
			ErrorMessage: &compat.ASTValue{Name: "tag required"},
		}},
		OrderedBy: &compat.ASTValue{Name: "user"},
		Reference: &compat.ASTValue{Name: "Leaf-list reference."},
		Status:    &compat.ASTValue{Name: "current"},
		Type:      &compat.Type{Name: "string"},
		Units:     &compat.ASTValue{Name: "labels"},
		When:      &compat.ASTValue{Name: "../enabled"},
	}}
	leafNode := &compat.Leaf{Name: "probe", Parent: mod}

	root := compat.RootNode(leafNode)
	if root == nil {
		t.Fatal("RootNode returned nil")
	}
	if got, want := len(root.LeafList), 1; got != want {
		t.Fatalf("RootNode LeafList len = %d, want %d", got, want)
	}
	leafList := root.LeafList[0]
	if got := leafList.ParentNode(); got != root {
		t.Fatalf("LeafList ParentNode = %#v, want root module", got)
	}
	if got := leafList.Config.ParentNode(); got != leafList {
		t.Fatalf("LeafList.Config ParentNode = %#v, want cloned leaf-list", got)
	}
	if got, want := len(leafList.Default), 2; got != want {
		t.Fatalf("LeafList.Default len = %d, want %d", got, want)
	}
	if got := leafList.Default[0].ParentNode(); got != leafList {
		t.Fatalf("LeafList.Default[0] ParentNode = %#v, want cloned leaf-list", got)
	}
	if got := leafList.Description.ParentNode(); got != leafList {
		t.Fatalf("LeafList.Description ParentNode = %#v, want cloned leaf-list", got)
	}
	if got, want := len(leafList.IfFeature), 1; got != want {
		t.Fatalf("LeafList.IfFeature len = %d, want %d", got, want)
	}
	if got := leafList.IfFeature[0].ParentNode(); got != leafList {
		t.Fatalf("LeafList.IfFeature[0] ParentNode = %#v, want cloned leaf-list", got)
	}
	if got := leafList.MaxElements.ParentNode(); got != leafList {
		t.Fatalf("LeafList.MaxElements ParentNode = %#v, want cloned leaf-list", got)
	}
	if got := leafList.MinElements.ParentNode(); got != leafList {
		t.Fatalf("LeafList.MinElements ParentNode = %#v, want cloned leaf-list", got)
	}
	if got, want := len(leafList.Must), 1; got != want {
		t.Fatalf("LeafList.Must len = %d, want %d", got, want)
	}
	if got := leafList.Must[0].ParentNode(); got != leafList {
		t.Fatalf("LeafList.Must[0] ParentNode = %#v, want cloned leaf-list", got)
	}
	if got := leafList.OrderedBy.ParentNode(); got != leafList {
		t.Fatalf("LeafList.OrderedBy ParentNode = %#v, want cloned leaf-list", got)
	}
	if got := leafList.Reference.ParentNode(); got != leafList {
		t.Fatalf("LeafList.Reference ParentNode = %#v, want cloned leaf-list", got)
	}
	if got := leafList.Status.ParentNode(); got != leafList {
		t.Fatalf("LeafList.Status ParentNode = %#v, want cloned leaf-list", got)
	}
	if leafList.Type == nil {
		t.Fatal("LeafList.Type = nil, want cloned type")
	}
	if got := leafList.Type.ParentNode(); got != leafList {
		t.Fatalf("LeafList.Type ParentNode = %#v, want cloned leaf-list", got)
	}
	if got := leafList.Units.ParentNode(); got != leafList {
		t.Fatalf("LeafList.Units ParentNode = %#v, want cloned leaf-list", got)
	}
	if got := leafList.When.ParentNode(); got != leafList {
		t.Fatalf("LeafList.When ParentNode = %#v, want cloned leaf-list", got)
	}
}

func TestASTModuleListCloneParentsValueChildren(t *testing.T) {
	mod := &compat.ASTModule{
		Name:   "compat-ast-list",
		Prefix: &compat.ASTValue{Name: "cal"},
	}
	mod.List = []*compat.List{{
		Name:        "item",
		Anydata:     []*compat.AnyData{{Name: "payload"}},
		Anyxml:      []*compat.AnyXML{{Name: "legacy"}},
		Config:      &compat.ASTValue{Name: "true"},
		Container:   []*compat.Container{{Name: "nested"}},
		Description: &compat.ASTValue{Name: "List description."},
		IfFeature:   []*compat.ASTValue{{Name: "gate"}},
		Key:         &compat.ASTValue{Name: "id"},
		Leaf: []*compat.Leaf{{
			Name: "id",
			Type: &compat.Type{Name: "string"},
		}},
		MaxElements: &compat.ASTValue{Name: "3"},
		MinElements: &compat.ASTValue{Name: "1"},
		Must: []*compat.Must{{
			Name:         "id",
			ErrorMessage: &compat.ASTValue{Name: "id required"},
		}},
		OrderedBy: &compat.ASTValue{Name: "user"},
		Reference: &compat.ASTValue{Name: "List reference."},
		Status:    &compat.ASTValue{Name: "current"},
		Unique:    []*compat.ASTValue{{Name: "id"}},
		When:      &compat.ASTValue{Name: "../enabled"},
	}}
	leafNode := &compat.Leaf{Name: "probe", Parent: mod}

	root := compat.RootNode(leafNode)
	if root == nil {
		t.Fatal("RootNode returned nil")
	}
	if got, want := len(root.List), 1; got != want {
		t.Fatalf("RootNode List len = %d, want %d", got, want)
	}
	list := root.List[0]
	if got := list.ParentNode(); got != root {
		t.Fatalf("List ParentNode = %#v, want root module", got)
	}
	if got, want := len(list.Anydata), 1; got != want {
		t.Fatalf("List.Anydata len = %d, want %d", got, want)
	}
	if got := list.Anydata[0].ParentNode(); got != list {
		t.Fatalf("List.Anydata[0] ParentNode = %#v, want cloned list", got)
	}
	if got, want := len(list.Anyxml), 1; got != want {
		t.Fatalf("List.Anyxml len = %d, want %d", got, want)
	}
	if got := list.Anyxml[0].ParentNode(); got != list {
		t.Fatalf("List.Anyxml[0] ParentNode = %#v, want cloned list", got)
	}
	if got := list.Config.ParentNode(); got != list {
		t.Fatalf("List.Config ParentNode = %#v, want cloned list", got)
	}
	if got, want := len(list.Container), 1; got != want {
		t.Fatalf("List.Container len = %d, want %d", got, want)
	}
	if got := list.Container[0].ParentNode(); got != list {
		t.Fatalf("List.Container[0] ParentNode = %#v, want cloned list", got)
	}
	if got := list.Description.ParentNode(); got != list {
		t.Fatalf("List.Description ParentNode = %#v, want cloned list", got)
	}
	if got, want := len(list.IfFeature), 1; got != want {
		t.Fatalf("List.IfFeature len = %d, want %d", got, want)
	}
	if got := list.IfFeature[0].ParentNode(); got != list {
		t.Fatalf("List.IfFeature[0] ParentNode = %#v, want cloned list", got)
	}
	if got := list.Key.ParentNode(); got != list {
		t.Fatalf("List.Key ParentNode = %#v, want cloned list", got)
	}
	if got, want := len(list.Leaf), 1; got != want {
		t.Fatalf("List.Leaf len = %d, want %d", got, want)
	}
	if got := list.Leaf[0].ParentNode(); got != list {
		t.Fatalf("List.Leaf[0] ParentNode = %#v, want cloned list", got)
	}
	if list.Leaf[0].Type == nil {
		t.Fatal("List.Leaf[0].Type = nil, want cloned type")
	}
	if got := list.Leaf[0].Type.ParentNode(); got != list.Leaf[0] {
		t.Fatalf("List.Leaf[0].Type ParentNode = %#v, want cloned leaf", got)
	}
	if got := list.MaxElements.ParentNode(); got != list {
		t.Fatalf("List.MaxElements ParentNode = %#v, want cloned list", got)
	}
	if got := list.MinElements.ParentNode(); got != list {
		t.Fatalf("List.MinElements ParentNode = %#v, want cloned list", got)
	}
	if got, want := len(list.Must), 1; got != want {
		t.Fatalf("List.Must len = %d, want %d", got, want)
	}
	if got := list.Must[0].ParentNode(); got != list {
		t.Fatalf("List.Must[0] ParentNode = %#v, want cloned list", got)
	}
	if got := list.OrderedBy.ParentNode(); got != list {
		t.Fatalf("List.OrderedBy ParentNode = %#v, want cloned list", got)
	}
	if got := list.Reference.ParentNode(); got != list {
		t.Fatalf("List.Reference ParentNode = %#v, want cloned list", got)
	}
	if got := list.Status.ParentNode(); got != list {
		t.Fatalf("List.Status ParentNode = %#v, want cloned list", got)
	}
	if got, want := len(list.Unique), 1; got != want {
		t.Fatalf("List.Unique len = %d, want %d", got, want)
	}
	if got := list.Unique[0].ParentNode(); got != list {
		t.Fatalf("List.Unique[0] ParentNode = %#v, want cloned list", got)
	}
	if got := list.When.ParentNode(); got != list {
		t.Fatalf("List.When ParentNode = %#v, want cloned list", got)
	}
}

func TestASTModuleGroupingCloneParentsValueChildren(t *testing.T) {
	mod := &compat.ASTModule{
		Name:   "compat-ast-grouping",
		Prefix: &compat.ASTValue{Name: "cag"},
	}
	mod.Grouping = []*compat.Grouping{{
		Name:        "grp",
		Anydata:     []*compat.AnyData{{Name: "payload"}},
		Anyxml:      []*compat.AnyXML{{Name: "legacy"}},
		Description: &compat.ASTValue{Name: "Grouping description."},
		Reference:   &compat.ASTValue{Name: "Grouping reference."},
		Status:      &compat.ASTValue{Name: "current"},
		Action: []*compat.Action{{
			Name: "reset",
			Input: &compat.Input{
				Name: "input",
				Leaf: []*compat.Leaf{{
					Name: "request",
					Type: &compat.Type{Name: "string"},
				}},
			},
		}},
		Choice: []*compat.Choice{{
			Name: "pick",
			Leaf: []*compat.Leaf{{
				Name: "choice-leaf",
				Type: &compat.Type{Name: "string"},
			}},
		}},
		Container: []*compat.Container{{Name: "nested"}},
		Grouping:  []*compat.Grouping{{Name: "nested-group"}},
		Leaf: []*compat.Leaf{{
			Name: "value",
			Type: &compat.Type{Name: "string"},
		}},
		LeafList: []*compat.LeafList{{
			Name: "tags",
			Type: &compat.Type{Name: "string"},
		}},
		List: []*compat.List{{
			Name: "item",
			Leaf: []*compat.Leaf{{
				Name: "id",
				Type: &compat.Type{Name: "string"},
			}},
		}},
		Notification: []*compat.Notification{{
			Name: "changed",
			Leaf: []*compat.Leaf{{
				Name: "message",
				Type: &compat.Type{Name: "string"},
			}},
		}},
		Typedef: []*compat.Typedef{{
			Name: "name-type",
			Type: &compat.Type{Name: "string"},
		}},
		Uses: []*compat.Uses{{Name: "other-group"}},
	}}
	leafNode := &compat.Leaf{Name: "probe", Parent: mod}

	root := compat.RootNode(leafNode)
	if root == nil {
		t.Fatal("RootNode returned nil")
	}
	if got, want := len(root.Grouping), 1; got != want {
		t.Fatalf("RootNode Grouping len = %d, want %d", got, want)
	}
	grouping := root.Grouping[0]
	if got := grouping.ParentNode(); got != root {
		t.Fatalf("Grouping ParentNode = %#v, want root module", got)
	}
	if got, want := len(grouping.Anydata), 1; got != want {
		t.Fatalf("Grouping.Anydata len = %d, want %d", got, want)
	}
	if got := grouping.Anydata[0].ParentNode(); got != grouping {
		t.Fatalf("Grouping.Anydata[0] ParentNode = %#v, want cloned grouping", got)
	}
	if got, want := len(grouping.Anyxml), 1; got != want {
		t.Fatalf("Grouping.Anyxml len = %d, want %d", got, want)
	}
	if got := grouping.Anyxml[0].ParentNode(); got != grouping {
		t.Fatalf("Grouping.Anyxml[0] ParentNode = %#v, want cloned grouping", got)
	}
	if got, want := len(grouping.Action), 1; got != want {
		t.Fatalf("Grouping.Action len = %d, want %d", got, want)
	}
	if got := grouping.Action[0].ParentNode(); got != grouping {
		t.Fatalf("Grouping.Action[0] ParentNode = %#v, want cloned grouping", got)
	}
	if got := grouping.Action[0].Input.ParentNode(); got != grouping.Action[0] {
		t.Fatalf("Grouping.Action[0].Input ParentNode = %#v, want cloned action", got)
	}
	if got, want := len(grouping.Choice), 1; got != want {
		t.Fatalf("Grouping.Choice len = %d, want %d", got, want)
	}
	if got := grouping.Choice[0].ParentNode(); got != grouping {
		t.Fatalf("Grouping.Choice[0] ParentNode = %#v, want cloned grouping", got)
	}
	if got, want := len(grouping.Container), 1; got != want {
		t.Fatalf("Grouping.Container len = %d, want %d", got, want)
	}
	if got := grouping.Container[0].ParentNode(); got != grouping {
		t.Fatalf("Grouping.Container[0] ParentNode = %#v, want cloned grouping", got)
	}
	if got := grouping.Description.ParentNode(); got != grouping {
		t.Fatalf("Grouping.Description ParentNode = %#v, want cloned grouping", got)
	}
	if got, want := len(grouping.Grouping), 1; got != want {
		t.Fatalf("Grouping.Grouping len = %d, want %d", got, want)
	}
	if got := grouping.Grouping[0].ParentNode(); got != grouping {
		t.Fatalf("Grouping.Grouping[0] ParentNode = %#v, want cloned grouping", got)
	}
	if got, want := len(grouping.Leaf), 1; got != want {
		t.Fatalf("Grouping.Leaf len = %d, want %d", got, want)
	}
	if got := grouping.Leaf[0].ParentNode(); got != grouping {
		t.Fatalf("Grouping.Leaf[0] ParentNode = %#v, want cloned grouping", got)
	}
	if got := grouping.Leaf[0].Type.ParentNode(); got != grouping.Leaf[0] {
		t.Fatalf("Grouping.Leaf[0].Type ParentNode = %#v, want cloned leaf", got)
	}
	if got, want := len(grouping.LeafList), 1; got != want {
		t.Fatalf("Grouping.LeafList len = %d, want %d", got, want)
	}
	if got := grouping.LeafList[0].ParentNode(); got != grouping {
		t.Fatalf("Grouping.LeafList[0] ParentNode = %#v, want cloned grouping", got)
	}
	if got, want := len(grouping.List), 1; got != want {
		t.Fatalf("Grouping.List len = %d, want %d", got, want)
	}
	if got := grouping.List[0].ParentNode(); got != grouping {
		t.Fatalf("Grouping.List[0] ParentNode = %#v, want cloned grouping", got)
	}
	if got, want := len(grouping.Notification), 1; got != want {
		t.Fatalf("Grouping.Notification len = %d, want %d", got, want)
	}
	if got := grouping.Notification[0].ParentNode(); got != grouping {
		t.Fatalf("Grouping.Notification[0] ParentNode = %#v, want cloned grouping", got)
	}
	if got := grouping.Reference.ParentNode(); got != grouping {
		t.Fatalf("Grouping.Reference ParentNode = %#v, want cloned grouping", got)
	}
	if got := grouping.Status.ParentNode(); got != grouping {
		t.Fatalf("Grouping.Status ParentNode = %#v, want cloned grouping", got)
	}
	if got, want := len(grouping.Typedef), 1; got != want {
		t.Fatalf("Grouping.Typedef len = %d, want %d", got, want)
	}
	if got := grouping.Typedef[0].ParentNode(); got != grouping {
		t.Fatalf("Grouping.Typedef[0] ParentNode = %#v, want cloned grouping", got)
	}
	if got, want := len(grouping.Uses), 1; got != want {
		t.Fatalf("Grouping.Uses len = %d, want %d", got, want)
	}
	if got := grouping.Uses[0].ParentNode(); got != grouping {
		t.Fatalf("Grouping.Uses[0] ParentNode = %#v, want cloned grouping", got)
	}
}

func TestASTModuleAnyDataAnyXMLCloneParentsValueChildren(t *testing.T) {
	mod := &compat.ASTModule{
		Name:   "compat-ast-opaque",
		Prefix: &compat.ASTValue{Name: "cao"},
	}
	mod.Anydata = []*compat.AnyData{{
		Name:        "payload",
		Config:      &compat.ASTValue{Name: "true"},
		Description: &compat.ASTValue{Name: "Anydata description."},
		IfFeature:   []*compat.ASTValue{{Name: "gate"}},
		Mandatory:   &compat.ASTValue{Name: "true"},
		Must: []*compat.Must{{
			Name:         "ready",
			ErrorMessage: &compat.ASTValue{Name: "ready required"},
		}},
		Reference: &compat.ASTValue{Name: "Anydata reference."},
		Status:    &compat.ASTValue{Name: "current"},
		When:      &compat.ASTValue{Name: "../enabled"},
	}}
	mod.Anyxml = []*compat.AnyXML{{
		Name:        "legacy",
		Config:      &compat.ASTValue{Name: "false"},
		Description: &compat.ASTValue{Name: "Anyxml description."},
		IfFeature:   []*compat.ASTValue{{Name: "gate"}},
		Mandatory:   &compat.ASTValue{Name: "true"},
		Must: []*compat.Must{{
			Name:         "legacy",
			ErrorMessage: &compat.ASTValue{Name: "legacy required"},
		}},
		Reference: &compat.ASTValue{Name: "Anyxml reference."},
		Status:    &compat.ASTValue{Name: "current"},
		When:      &compat.ASTValue{Name: "../enabled"},
	}}
	leafNode := &compat.Leaf{Name: "probe", Parent: mod}

	root := compat.RootNode(leafNode)
	if root == nil {
		t.Fatal("RootNode returned nil")
	}
	if got, want := len(root.Anydata), 1; got != want {
		t.Fatalf("RootNode Anydata len = %d, want %d", got, want)
	}
	anydata := root.Anydata[0]
	if got := anydata.ParentNode(); got != root {
		t.Fatalf("Anydata ParentNode = %#v, want root module", got)
	}
	if got := anydata.Config.ParentNode(); got != anydata {
		t.Fatalf("Anydata.Config ParentNode = %#v, want cloned anydata", got)
	}
	if got := anydata.Description.ParentNode(); got != anydata {
		t.Fatalf("Anydata.Description ParentNode = %#v, want cloned anydata", got)
	}
	if got, want := len(anydata.IfFeature), 1; got != want {
		t.Fatalf("Anydata.IfFeature len = %d, want %d", got, want)
	}
	if got := anydata.IfFeature[0].ParentNode(); got != anydata {
		t.Fatalf("Anydata.IfFeature[0] ParentNode = %#v, want cloned anydata", got)
	}
	if got := anydata.Mandatory.ParentNode(); got != anydata {
		t.Fatalf("Anydata.Mandatory ParentNode = %#v, want cloned anydata", got)
	}
	if got, want := len(anydata.Must), 1; got != want {
		t.Fatalf("Anydata.Must len = %d, want %d", got, want)
	}
	if got := anydata.Must[0].ParentNode(); got != anydata {
		t.Fatalf("Anydata.Must[0] ParentNode = %#v, want cloned anydata", got)
	}
	if got := anydata.Reference.ParentNode(); got != anydata {
		t.Fatalf("Anydata.Reference ParentNode = %#v, want cloned anydata", got)
	}
	if got := anydata.Status.ParentNode(); got != anydata {
		t.Fatalf("Anydata.Status ParentNode = %#v, want cloned anydata", got)
	}
	if got := anydata.When.ParentNode(); got != anydata {
		t.Fatalf("Anydata.When ParentNode = %#v, want cloned anydata", got)
	}

	if got, want := len(root.Anyxml), 1; got != want {
		t.Fatalf("RootNode Anyxml len = %d, want %d", got, want)
	}
	anyxml := root.Anyxml[0]
	if got := anyxml.ParentNode(); got != root {
		t.Fatalf("Anyxml ParentNode = %#v, want root module", got)
	}
	if got := anyxml.Config.ParentNode(); got != anyxml {
		t.Fatalf("Anyxml.Config ParentNode = %#v, want cloned anyxml", got)
	}
	if got := anyxml.Description.ParentNode(); got != anyxml {
		t.Fatalf("Anyxml.Description ParentNode = %#v, want cloned anyxml", got)
	}
	if got, want := len(anyxml.IfFeature), 1; got != want {
		t.Fatalf("Anyxml.IfFeature len = %d, want %d", got, want)
	}
	if got := anyxml.IfFeature[0].ParentNode(); got != anyxml {
		t.Fatalf("Anyxml.IfFeature[0] ParentNode = %#v, want cloned anyxml", got)
	}
	if got := anyxml.Mandatory.ParentNode(); got != anyxml {
		t.Fatalf("Anyxml.Mandatory ParentNode = %#v, want cloned anyxml", got)
	}
	if got, want := len(anyxml.Must), 1; got != want {
		t.Fatalf("Anyxml.Must len = %d, want %d", got, want)
	}
	if got := anyxml.Must[0].ParentNode(); got != anyxml {
		t.Fatalf("Anyxml.Must[0] ParentNode = %#v, want cloned anyxml", got)
	}
	if got := anyxml.Reference.ParentNode(); got != anyxml {
		t.Fatalf("Anyxml.Reference ParentNode = %#v, want cloned anyxml", got)
	}
	if got := anyxml.Status.ParentNode(); got != anyxml {
		t.Fatalf("Anyxml.Status ParentNode = %#v, want cloned anyxml", got)
	}
	if got := anyxml.When.ParentNode(); got != anyxml {
		t.Fatalf("Anyxml.When ParentNode = %#v, want cloned anyxml", got)
	}
}

func TestASTModuleContainerCloneParentsValueChildren(t *testing.T) {
	mod := &compat.ASTModule{
		Name:   "compat-ast-container",
		Prefix: &compat.ASTValue{Name: "cac"},
	}
	mod.Container = []*compat.Container{{
		Name:        "top",
		Config:      &compat.ASTValue{Name: "true"},
		Description: &compat.ASTValue{Name: "Container description."},
		IfFeature:   []*compat.ASTValue{{Name: "gate"}},
		Leaf: []*compat.Leaf{{
			Name: "value",
			Type: &compat.Type{Name: "string"},
		}},
		Must: []*compat.Must{{
			Name:         "value",
			ErrorMessage: &compat.ASTValue{Name: "value required"},
		}},
		Presence:  &compat.ASTValue{Name: "enabled"},
		Reference: &compat.ASTValue{Name: "Container reference."},
		Status:    &compat.ASTValue{Name: "current"},
		When:      &compat.ASTValue{Name: "../enabled"},
	}}
	leafNode := &compat.Leaf{Name: "probe", Parent: mod}

	root := compat.RootNode(leafNode)
	if root == nil {
		t.Fatal("RootNode returned nil")
	}
	if got, want := len(root.Container), 1; got != want {
		t.Fatalf("RootNode Container len = %d, want %d", got, want)
	}
	container := root.Container[0]
	if got := container.ParentNode(); got != root {
		t.Fatalf("Container ParentNode = %#v, want root module", got)
	}
	if got := container.Config.ParentNode(); got != container {
		t.Fatalf("Container.Config ParentNode = %#v, want cloned container", got)
	}
	if got := container.Description.ParentNode(); got != container {
		t.Fatalf("Container.Description ParentNode = %#v, want cloned container", got)
	}
	if got, want := len(container.IfFeature), 1; got != want {
		t.Fatalf("Container.IfFeature len = %d, want %d", got, want)
	}
	if got := container.IfFeature[0].ParentNode(); got != container {
		t.Fatalf("Container.IfFeature[0] ParentNode = %#v, want cloned container", got)
	}
	if got, want := len(container.Leaf), 1; got != want {
		t.Fatalf("Container.Leaf len = %d, want %d", got, want)
	}
	if got := container.Leaf[0].ParentNode(); got != container {
		t.Fatalf("Container.Leaf[0] ParentNode = %#v, want cloned container", got)
	}
	if container.Leaf[0].Type == nil {
		t.Fatal("Container.Leaf[0].Type = nil, want cloned type")
	}
	if got := container.Leaf[0].Type.ParentNode(); got != container.Leaf[0] {
		t.Fatalf("Container.Leaf[0].Type ParentNode = %#v, want cloned leaf", got)
	}
	if got, want := len(container.Must), 1; got != want {
		t.Fatalf("Container.Must len = %d, want %d", got, want)
	}
	if got := container.Must[0].ParentNode(); got != container {
		t.Fatalf("Container.Must[0] ParentNode = %#v, want cloned container", got)
	}
	if got := container.Must[0].ErrorMessage.ParentNode(); got != container.Must[0] {
		t.Fatalf("Container.Must[0].ErrorMessage ParentNode = %#v, want cloned must", got)
	}
	if got := container.Presence.ParentNode(); got != container {
		t.Fatalf("Container.Presence ParentNode = %#v, want cloned container", got)
	}
	if got := container.Reference.ParentNode(); got != container {
		t.Fatalf("Container.Reference ParentNode = %#v, want cloned container", got)
	}
	if got := container.Status.ParentNode(); got != container {
		t.Fatalf("Container.Status ParentNode = %#v, want cloned container", got)
	}
	if got := container.When.ParentNode(); got != container {
		t.Fatalf("Container.When ParentNode = %#v, want cloned container", got)
	}
}

func TestASTModuleContainerListOperationChildrenCloneParents(t *testing.T) {
	mod := &compat.ASTModule{
		Name:   "compat-ast-ops",
		Prefix: &compat.ASTValue{Name: "cao"},
	}
	mod.Container = []*compat.Container{{
		Name: "top",
		Action: []*compat.Action{{
			Name:        "reset",
			Description: &compat.ASTValue{Name: "Action description."},
			IfFeature:   []*compat.ASTValue{{Name: "action-gate"}},
			Reference:   &compat.ASTValue{Name: "Action reference."},
			Status:      &compat.ASTValue{Name: "current"},
			Input: &compat.Input{
				Name: "input",
				Leaf: []*compat.Leaf{{
					Name: "request",
					Type: &compat.Type{Name: "string"},
				}},
			},
			Output: &compat.Output{
				Name: "output",
				Leaf: []*compat.Leaf{{
					Name: "result",
					Type: &compat.Type{Name: "string"},
				}},
			},
		}},
		Notification: []*compat.Notification{{
			Name: "changed",
			Leaf: []*compat.Leaf{{
				Name: "message",
				Type: &compat.Type{Name: "string"},
			}},
		}},
		List: []*compat.List{{
			Name: "item",
			Action: []*compat.Action{{
				Name: "touch",
				Input: &compat.Input{
					Name: "input",
					Leaf: []*compat.Leaf{{
						Name: "target",
						Type: &compat.Type{Name: "string"},
					}},
				},
			}},
			Notification: []*compat.Notification{{
				Name: "item-changed",
				Leaf: []*compat.Leaf{{
					Name: "detail",
					Type: &compat.Type{Name: "string"},
				}},
			}},
		}},
	}}
	leafNode := &compat.Leaf{Name: "probe", Parent: mod}

	root := compat.RootNode(leafNode)
	if root == nil {
		t.Fatal("RootNode returned nil")
	}
	if got, want := len(root.Container), 1; got != want {
		t.Fatalf("RootNode Container len = %d, want %d", got, want)
	}
	container := root.Container[0]
	if got, want := len(container.Action), 1; got != want {
		t.Fatalf("Container.Action len = %d, want %d", got, want)
	}
	action := container.Action[0]
	if got := action.ParentNode(); got != container {
		t.Fatalf("Container.Action[0] ParentNode = %#v, want cloned container", got)
	}
	if got := action.Description.ParentNode(); got != action {
		t.Fatalf("Container.Action[0].Description ParentNode = %#v, want cloned action", got)
	}
	if got := action.IfFeature[0].ParentNode(); got != action {
		t.Fatalf("Container.Action[0].IfFeature[0] ParentNode = %#v, want cloned action", got)
	}
	if got := action.Reference.ParentNode(); got != action {
		t.Fatalf("Container.Action[0].Reference ParentNode = %#v, want cloned action", got)
	}
	if got := action.Status.ParentNode(); got != action {
		t.Fatalf("Container.Action[0].Status ParentNode = %#v, want cloned action", got)
	}
	if got := action.Input.ParentNode(); got != action {
		t.Fatalf("Container.Action[0].Input ParentNode = %#v, want cloned action", got)
	}
	if got := action.Input.Leaf[0].ParentNode(); got != action.Input {
		t.Fatalf("Container.Action[0].Input.Leaf[0] ParentNode = %#v, want cloned input", got)
	}
	if got := action.Input.Leaf[0].Type.ParentNode(); got != action.Input.Leaf[0] {
		t.Fatalf("Container.Action[0].Input.Leaf[0].Type ParentNode = %#v, want cloned leaf", got)
	}
	if got := action.Output.ParentNode(); got != action {
		t.Fatalf("Container.Action[0].Output ParentNode = %#v, want cloned action", got)
	}
	if got := action.Output.Leaf[0].ParentNode(); got != action.Output {
		t.Fatalf("Container.Action[0].Output.Leaf[0] ParentNode = %#v, want cloned output", got)
	}
	if got := action.Output.Leaf[0].Type.ParentNode(); got != action.Output.Leaf[0] {
		t.Fatalf("Container.Action[0].Output.Leaf[0].Type ParentNode = %#v, want cloned leaf", got)
	}
	if got, want := len(container.Notification), 1; got != want {
		t.Fatalf("Container.Notification len = %d, want %d", got, want)
	}
	notification := container.Notification[0]
	if got := notification.ParentNode(); got != container {
		t.Fatalf("Container.Notification[0] ParentNode = %#v, want cloned container", got)
	}
	if got := notification.Leaf[0].ParentNode(); got != notification {
		t.Fatalf("Container.Notification[0].Leaf[0] ParentNode = %#v, want cloned notification", got)
	}
	if got := notification.Leaf[0].Type.ParentNode(); got != notification.Leaf[0] {
		t.Fatalf("Container.Notification[0].Leaf[0].Type ParentNode = %#v, want cloned leaf", got)
	}
	if got, want := len(container.List), 1; got != want {
		t.Fatalf("Container.List len = %d, want %d", got, want)
	}
	list := container.List[0]
	if got, want := len(list.Action), 1; got != want {
		t.Fatalf("Container.List[0].Action len = %d, want %d", got, want)
	}
	listAction := list.Action[0]
	if got := listAction.ParentNode(); got != list {
		t.Fatalf("Container.List[0].Action[0] ParentNode = %#v, want cloned list", got)
	}
	if got := listAction.Input.ParentNode(); got != listAction {
		t.Fatalf("Container.List[0].Action[0].Input ParentNode = %#v, want cloned action", got)
	}
	if got := listAction.Input.Leaf[0].ParentNode(); got != listAction.Input {
		t.Fatalf("Container.List[0].Action[0].Input.Leaf[0] ParentNode = %#v, want cloned input", got)
	}
	if got := listAction.Input.Leaf[0].Type.ParentNode(); got != listAction.Input.Leaf[0] {
		t.Fatalf("Container.List[0].Action[0].Input.Leaf[0].Type ParentNode = %#v, want cloned leaf", got)
	}
	if got, want := len(list.Notification), 1; got != want {
		t.Fatalf("Container.List[0].Notification len = %d, want %d", got, want)
	}
	listNotification := list.Notification[0]
	if got := listNotification.ParentNode(); got != list {
		t.Fatalf("Container.List[0].Notification[0] ParentNode = %#v, want cloned list", got)
	}
	if got := listNotification.Leaf[0].ParentNode(); got != listNotification {
		t.Fatalf("Container.List[0].Notification[0].Leaf[0] ParentNode = %#v, want cloned notification", got)
	}
	if got := listNotification.Leaf[0].Type.ParentNode(); got != listNotification.Leaf[0] {
		t.Fatalf("Container.List[0].Notification[0].Leaf[0].Type ParentNode = %#v, want cloned leaf", got)
	}
}

func TestASTModuleChoiceCaseCloneParentsValueChildren(t *testing.T) {
	mod := &compat.ASTModule{
		Name:   "compat-ast-choice",
		Prefix: &compat.ASTValue{Name: "cac"},
	}
	mod.Choice = []*compat.Choice{{
		Name:        "select",
		Config:      &compat.ASTValue{Name: "true"},
		Default:     &compat.ASTValue{Name: "one"},
		Description: &compat.ASTValue{Name: "Choice description."},
		IfFeature:   []*compat.ASTValue{{Name: "gate"}},
		Leaf: []*compat.Leaf{{
			Name: "direct",
			Type: &compat.Type{Name: "string"},
		}},
		Mandatory: &compat.ASTValue{Name: "true"},
		Reference: &compat.ASTValue{Name: "Choice reference."},
		Status:    &compat.ASTValue{Name: "current"},
		When:      &compat.ASTValue{Name: "../enabled"},
		Case: []*compat.Case{{
			Name:        "one",
			Description: &compat.ASTValue{Name: "Case description."},
			IfFeature:   []*compat.ASTValue{{Name: "gate"}},
			Leaf: []*compat.Leaf{{
				Name: "case-leaf",
				Type: &compat.Type{Name: "string"},
			}},
			Reference: &compat.ASTValue{Name: "Case reference."},
			Status:    &compat.ASTValue{Name: "current"},
			When:      &compat.ASTValue{Name: "../enabled"},
		}},
	}}
	leafNode := &compat.Leaf{Name: "probe", Parent: mod}

	root := compat.RootNode(leafNode)
	if root == nil {
		t.Fatal("RootNode returned nil")
	}
	if got, want := len(root.Choice), 1; got != want {
		t.Fatalf("RootNode Choice len = %d, want %d", got, want)
	}
	choice := root.Choice[0]
	if got := choice.ParentNode(); got != root {
		t.Fatalf("Choice ParentNode = %#v, want root module", got)
	}
	if got := choice.Config.ParentNode(); got != choice {
		t.Fatalf("Choice.Config ParentNode = %#v, want cloned choice", got)
	}
	if got := choice.Default.ParentNode(); got != choice {
		t.Fatalf("Choice.Default ParentNode = %#v, want cloned choice", got)
	}
	if got := choice.Description.ParentNode(); got != choice {
		t.Fatalf("Choice.Description ParentNode = %#v, want cloned choice", got)
	}
	if got, want := len(choice.IfFeature), 1; got != want {
		t.Fatalf("Choice.IfFeature len = %d, want %d", got, want)
	}
	if got := choice.IfFeature[0].ParentNode(); got != choice {
		t.Fatalf("Choice.IfFeature[0] ParentNode = %#v, want cloned choice", got)
	}
	if got, want := len(choice.Leaf), 1; got != want {
		t.Fatalf("Choice.Leaf len = %d, want %d", got, want)
	}
	if got := choice.Leaf[0].ParentNode(); got != choice {
		t.Fatalf("Choice.Leaf[0] ParentNode = %#v, want cloned choice", got)
	}
	if got := choice.Leaf[0].Type.ParentNode(); got != choice.Leaf[0] {
		t.Fatalf("Choice.Leaf[0].Type ParentNode = %#v, want cloned leaf", got)
	}
	if got := choice.Mandatory.ParentNode(); got != choice {
		t.Fatalf("Choice.Mandatory ParentNode = %#v, want cloned choice", got)
	}
	if got := choice.Reference.ParentNode(); got != choice {
		t.Fatalf("Choice.Reference ParentNode = %#v, want cloned choice", got)
	}
	if got := choice.Status.ParentNode(); got != choice {
		t.Fatalf("Choice.Status ParentNode = %#v, want cloned choice", got)
	}
	if got := choice.When.ParentNode(); got != choice {
		t.Fatalf("Choice.When ParentNode = %#v, want cloned choice", got)
	}
	if got, want := len(choice.Case), 1; got != want {
		t.Fatalf("Choice.Case len = %d, want %d", got, want)
	}
	cas := choice.Case[0]
	if got := cas.ParentNode(); got != choice {
		t.Fatalf("Choice.Case[0] ParentNode = %#v, want cloned choice", got)
	}
	if got := cas.Description.ParentNode(); got != cas {
		t.Fatalf("Choice.Case[0].Description ParentNode = %#v, want cloned case", got)
	}
	if got, want := len(cas.IfFeature), 1; got != want {
		t.Fatalf("Choice.Case[0].IfFeature len = %d, want %d", got, want)
	}
	if got := cas.IfFeature[0].ParentNode(); got != cas {
		t.Fatalf("Choice.Case[0].IfFeature[0] ParentNode = %#v, want cloned case", got)
	}
	if got, want := len(cas.Leaf), 1; got != want {
		t.Fatalf("Choice.Case[0].Leaf len = %d, want %d", got, want)
	}
	if got := cas.Leaf[0].ParentNode(); got != cas {
		t.Fatalf("Choice.Case[0].Leaf[0] ParentNode = %#v, want cloned case", got)
	}
	if got := cas.Leaf[0].Type.ParentNode(); got != cas.Leaf[0] {
		t.Fatalf("Choice.Case[0].Leaf[0].Type ParentNode = %#v, want cloned leaf", got)
	}
	if got := cas.Reference.ParentNode(); got != cas {
		t.Fatalf("Choice.Case[0].Reference ParentNode = %#v, want cloned case", got)
	}
	if got := cas.Status.ParentNode(); got != cas {
		t.Fatalf("Choice.Case[0].Status ParentNode = %#v, want cloned case", got)
	}
	if got := cas.When.ParentNode(); got != cas {
		t.Fatalf("Choice.Case[0].When ParentNode = %#v, want cloned case", got)
	}
}

func TestASTModuleUsesCloneParentsValueChildren(t *testing.T) {
	mod := &compat.ASTModule{
		Name:   "compat-ast-uses",
		Prefix: &compat.ASTValue{Name: "cau"},
	}
	mod.Uses = []*compat.Uses{{
		Name:        "grp",
		Description: &compat.ASTValue{Name: "Uses description."},
		IfFeature:   []*compat.ASTValue{{Name: "gate"}},
		Reference:   &compat.ASTValue{Name: "Uses reference."},
		Status:      &compat.ASTValue{Name: "current"},
		When:        &compat.ASTValue{Name: "../enabled"},
		Refine: []*compat.Refine{{
			Name:        "target",
			Config:      &compat.ASTValue{Name: "false"},
			Default:     &compat.ASTValue{Name: "fallback"},
			Description: &compat.ASTValue{Name: "Refine description."},
			IfFeature:   []*compat.ASTValue{{Name: "refine-gate"}},
			Mandatory:   &compat.ASTValue{Name: "true"},
			MaxElements: &compat.ASTValue{Name: "4"},
			MinElements: &compat.ASTValue{Name: "1"},
			Must: []*compat.Must{{
				Name:         "../enabled = 'true'",
				ErrorMessage: &compat.ASTValue{Name: "must fail"},
			}},
			Presence:  &compat.ASTValue{Name: "present"},
			Reference: &compat.ASTValue{Name: "Refine reference."},
		}},
		Augment: &compat.Augment{
			Name:        "target",
			Description: &compat.ASTValue{Name: "Augment description."},
			IfFeature:   []*compat.ASTValue{{Name: "augment-gate"}},
			Leaf: []*compat.Leaf{{
				Name: "added",
				Type: &compat.Type{Name: "string"},
			}},
			Reference: &compat.ASTValue{Name: "Augment reference."},
			Status:    &compat.ASTValue{Name: "current"},
			When:      &compat.ASTValue{Name: "../enabled"},
		},
	}}
	leafNode := &compat.Leaf{Name: "probe", Parent: mod}

	root := compat.RootNode(leafNode)
	if root == nil {
		t.Fatal("RootNode returned nil")
	}
	if got, want := len(root.Uses), 1; got != want {
		t.Fatalf("RootNode Uses len = %d, want %d", got, want)
	}
	uses := root.Uses[0]
	if got := uses.ParentNode(); got != root {
		t.Fatalf("Uses ParentNode = %#v, want root module", got)
	}
	if got := uses.Description.ParentNode(); got != uses {
		t.Fatalf("Uses.Description ParentNode = %#v, want cloned uses", got)
	}
	if got, want := len(uses.IfFeature), 1; got != want {
		t.Fatalf("Uses.IfFeature len = %d, want %d", got, want)
	}
	if got := uses.IfFeature[0].ParentNode(); got != uses {
		t.Fatalf("Uses.IfFeature[0] ParentNode = %#v, want cloned uses", got)
	}
	if got := uses.Reference.ParentNode(); got != uses {
		t.Fatalf("Uses.Reference ParentNode = %#v, want cloned uses", got)
	}
	if got := uses.Status.ParentNode(); got != uses {
		t.Fatalf("Uses.Status ParentNode = %#v, want cloned uses", got)
	}
	if got := uses.When.ParentNode(); got != uses {
		t.Fatalf("Uses.When ParentNode = %#v, want cloned uses", got)
	}
	if got, want := len(uses.Refine), 1; got != want {
		t.Fatalf("Uses.Refine len = %d, want %d", got, want)
	}
	refine := uses.Refine[0]
	if got := refine.ParentNode(); got != uses {
		t.Fatalf("Uses.Refine[0] ParentNode = %#v, want cloned uses", got)
	}
	if got := refine.Config.ParentNode(); got != refine {
		t.Fatalf("Uses.Refine[0].Config ParentNode = %#v, want cloned refine", got)
	}
	if got := refine.Default.ParentNode(); got != refine {
		t.Fatalf("Uses.Refine[0].Default ParentNode = %#v, want cloned refine", got)
	}
	if got := refine.Description.ParentNode(); got != refine {
		t.Fatalf("Uses.Refine[0].Description ParentNode = %#v, want cloned refine", got)
	}
	if got, want := len(refine.IfFeature), 1; got != want {
		t.Fatalf("Uses.Refine[0].IfFeature len = %d, want %d", got, want)
	}
	if got := refine.IfFeature[0].ParentNode(); got != refine {
		t.Fatalf("Uses.Refine[0].IfFeature[0] ParentNode = %#v, want cloned refine", got)
	}
	if got := refine.Mandatory.ParentNode(); got != refine {
		t.Fatalf("Uses.Refine[0].Mandatory ParentNode = %#v, want cloned refine", got)
	}
	if got := refine.MaxElements.ParentNode(); got != refine {
		t.Fatalf("Uses.Refine[0].MaxElements ParentNode = %#v, want cloned refine", got)
	}
	if got := refine.MinElements.ParentNode(); got != refine {
		t.Fatalf("Uses.Refine[0].MinElements ParentNode = %#v, want cloned refine", got)
	}
	if got, want := len(refine.Must), 1; got != want {
		t.Fatalf("Uses.Refine[0].Must len = %d, want %d", got, want)
	}
	if got := refine.Must[0].ParentNode(); got != refine {
		t.Fatalf("Uses.Refine[0].Must[0] ParentNode = %#v, want cloned refine", got)
	}
	if got := refine.Must[0].ErrorMessage.ParentNode(); got != refine.Must[0] {
		t.Fatalf("Uses.Refine[0].Must[0].ErrorMessage ParentNode = %#v, want cloned must", got)
	}
	if got := refine.Presence.ParentNode(); got != refine {
		t.Fatalf("Uses.Refine[0].Presence ParentNode = %#v, want cloned refine", got)
	}
	if got := refine.Reference.ParentNode(); got != refine {
		t.Fatalf("Uses.Refine[0].Reference ParentNode = %#v, want cloned refine", got)
	}
	augment := uses.Augment
	if augment == nil {
		t.Fatal("Uses.Augment = nil, want cloned augment")
	}
	if got := augment.ParentNode(); got != uses {
		t.Fatalf("Uses.Augment ParentNode = %#v, want cloned uses", got)
	}
	if got := augment.Description.ParentNode(); got != augment {
		t.Fatalf("Uses.Augment.Description ParentNode = %#v, want cloned augment", got)
	}
	if got, want := len(augment.IfFeature), 1; got != want {
		t.Fatalf("Uses.Augment.IfFeature len = %d, want %d", got, want)
	}
	if got := augment.IfFeature[0].ParentNode(); got != augment {
		t.Fatalf("Uses.Augment.IfFeature[0] ParentNode = %#v, want cloned augment", got)
	}
	if got, want := len(augment.Leaf), 1; got != want {
		t.Fatalf("Uses.Augment.Leaf len = %d, want %d", got, want)
	}
	if got := augment.Leaf[0].ParentNode(); got != augment {
		t.Fatalf("Uses.Augment.Leaf[0] ParentNode = %#v, want cloned augment", got)
	}
	if got := augment.Leaf[0].Type.ParentNode(); got != augment.Leaf[0] {
		t.Fatalf("Uses.Augment.Leaf[0].Type ParentNode = %#v, want cloned leaf", got)
	}
	if got := augment.Reference.ParentNode(); got != augment {
		t.Fatalf("Uses.Augment.Reference ParentNode = %#v, want cloned augment", got)
	}
	if got := augment.Status.ParentNode(); got != augment {
		t.Fatalf("Uses.Augment.Status ParentNode = %#v, want cloned augment", got)
	}
	if got := augment.When.ParentNode(); got != augment {
		t.Fatalf("Uses.Augment.When ParentNode = %#v, want cloned augment", got)
	}
}

func TestASTModuleAugmentCloneParentsValueChildren(t *testing.T) {
	mod := &compat.ASTModule{
		Name:   "compat-ast-augment",
		Prefix: &compat.ASTValue{Name: "caa"},
	}
	mod.Augment = []*compat.Augment{{
		Name:        "/caa:top",
		Description: &compat.ASTValue{Name: "Augment description."},
		IfFeature:   []*compat.ASTValue{{Name: "gate"}},
		Leaf: []*compat.Leaf{{
			Name: "added",
			Type: &compat.Type{Name: "string"},
		}},
		Reference: &compat.ASTValue{Name: "Augment reference."},
		Status:    &compat.ASTValue{Name: "current"},
		When:      &compat.ASTValue{Name: "../enabled"},
		Choice: []*compat.Choice{{
			Name: "select",
			Case: []*compat.Case{{
				Name: "one",
				Leaf: []*compat.Leaf{{
					Name: "case-leaf",
					Type: &compat.Type{Name: "string"},
				}},
			}},
		}},
	}}
	leafNode := &compat.Leaf{Name: "probe", Parent: mod}

	root := compat.RootNode(leafNode)
	if root == nil {
		t.Fatal("RootNode returned nil")
	}
	if got, want := len(root.Augment), 1; got != want {
		t.Fatalf("RootNode Augment len = %d, want %d", got, want)
	}
	augment := root.Augment[0]
	if got := augment.ParentNode(); got != root {
		t.Fatalf("Augment ParentNode = %#v, want root module", got)
	}
	if got := augment.Description.ParentNode(); got != augment {
		t.Fatalf("Augment.Description ParentNode = %#v, want cloned augment", got)
	}
	if got, want := len(augment.IfFeature), 1; got != want {
		t.Fatalf("Augment.IfFeature len = %d, want %d", got, want)
	}
	if got := augment.IfFeature[0].ParentNode(); got != augment {
		t.Fatalf("Augment.IfFeature[0] ParentNode = %#v, want cloned augment", got)
	}
	if got, want := len(augment.Leaf), 1; got != want {
		t.Fatalf("Augment.Leaf len = %d, want %d", got, want)
	}
	if got := augment.Leaf[0].ParentNode(); got != augment {
		t.Fatalf("Augment.Leaf[0] ParentNode = %#v, want cloned augment", got)
	}
	if got := augment.Leaf[0].Type.ParentNode(); got != augment.Leaf[0] {
		t.Fatalf("Augment.Leaf[0].Type ParentNode = %#v, want cloned leaf", got)
	}
	if got := augment.Reference.ParentNode(); got != augment {
		t.Fatalf("Augment.Reference ParentNode = %#v, want cloned augment", got)
	}
	if got := augment.Status.ParentNode(); got != augment {
		t.Fatalf("Augment.Status ParentNode = %#v, want cloned augment", got)
	}
	if got := augment.When.ParentNode(); got != augment {
		t.Fatalf("Augment.When ParentNode = %#v, want cloned augment", got)
	}
	if got, want := len(augment.Choice), 1; got != want {
		t.Fatalf("Augment.Choice len = %d, want %d", got, want)
	}
	choice := augment.Choice[0]
	if got := choice.ParentNode(); got != augment {
		t.Fatalf("Augment.Choice[0] ParentNode = %#v, want cloned augment", got)
	}
	if got, want := len(choice.Case), 1; got != want {
		t.Fatalf("Augment.Choice[0].Case len = %d, want %d", got, want)
	}
	cas := choice.Case[0]
	if got := cas.ParentNode(); got != choice {
		t.Fatalf("Augment.Choice[0].Case[0] ParentNode = %#v, want cloned choice", got)
	}
	if got, want := len(cas.Leaf), 1; got != want {
		t.Fatalf("Augment.Choice[0].Case[0].Leaf len = %d, want %d", got, want)
	}
	if got := cas.Leaf[0].ParentNode(); got != cas {
		t.Fatalf("Augment.Choice[0].Case[0].Leaf[0] ParentNode = %#v, want cloned case", got)
	}
	if got := cas.Leaf[0].Type.ParentNode(); got != cas.Leaf[0] {
		t.Fatalf("Augment.Choice[0].Case[0].Leaf[0].Type ParentNode = %#v, want cloned leaf", got)
	}
}

func TestASTModuleTopLevelMetadataAndOperationsCloneParents(t *testing.T) {
	mod := &compat.ASTModule{
		Name:   "compat-ast-top-level",
		Prefix: &compat.ASTValue{Name: "cat"},
	}
	mod.Feature = []*compat.Feature{{
		Name:        "gate",
		Description: &compat.ASTValue{Name: "Feature description."},
		IfFeature:   []*compat.ASTValue{{Name: "base-gate"}},
		Reference:   &compat.ASTValue{Name: "Feature reference."},
		Status:      &compat.ASTValue{Name: "current"},
	}}
	mod.Extension = []*compat.Extension{{
		Name:        "ext",
		Description: &compat.ASTValue{Name: "Extension description."},
		Reference:   &compat.ASTValue{Name: "Extension reference."},
		Status:      &compat.ASTValue{Name: "current"},
		Argument: &compat.Argument{
			Name:       "arg",
			YinElement: &compat.ASTValue{Name: "true"},
		},
	}}
	mod.Deviation = []*compat.Deviation{{
		Name:        "/cat:top",
		Description: &compat.ASTValue{Name: "Deviation description."},
		Reference:   &compat.ASTValue{Name: "Deviation reference."},
		Deviate: []*compat.Deviate{{
			Name:      "replace",
			Config:    &compat.ASTValue{Name: "false"},
			Default:   &compat.ASTValue{Name: "fallback"},
			Mandatory: &compat.ASTValue{Name: "true"},
			Must: []*compat.Must{{
				Name:         "../enabled = 'true'",
				ErrorMessage: &compat.ASTValue{Name: "must fail"},
			}},
			Type:   &compat.Type{Name: "string"},
			Unique: []*compat.ASTValue{{Name: "id"}},
			Units:  &compat.ASTValue{Name: "widgets"},
		}},
	}}
	mod.RPC = []*compat.RPC{{
		Name:        "reset",
		Description: &compat.ASTValue{Name: "RPC description."},
		IfFeature:   []*compat.ASTValue{{Name: "rpc-gate"}},
		Reference:   &compat.ASTValue{Name: "RPC reference."},
		Status:      &compat.ASTValue{Name: "current"},
		Grouping: []*compat.Grouping{{
			Name: "rpc-group",
			Leaf: []*compat.Leaf{{
				Name: "group-leaf",
				Type: &compat.Type{Name: "string"},
			}},
		}},
		Typedef: []*compat.Typedef{{
			Name: "rpc-type",
			Type: &compat.Type{Name: "string"},
		}},
		Input: &compat.Input{
			Name: "input",
			Leaf: []*compat.Leaf{{
				Name: "request",
				Type: &compat.Type{Name: "string"},
			}},
		},
		Output: &compat.Output{
			Name: "output",
			Leaf: []*compat.Leaf{{
				Name: "result",
				Type: &compat.Type{Name: "string"},
			}},
		},
	}}
	mod.Notification = []*compat.Notification{{
		Name:        "event",
		Description: &compat.ASTValue{Name: "Notification description."},
		IfFeature:   []*compat.ASTValue{{Name: "notification-gate"}},
		Reference:   &compat.ASTValue{Name: "Notification reference."},
		Status:      &compat.ASTValue{Name: "current"},
		Leaf: []*compat.Leaf{{
			Name: "message",
			Type: &compat.Type{Name: "string"},
		}},
		Choice: []*compat.Choice{{
			Name: "pick",
			Leaf: []*compat.Leaf{{
				Name: "choice-leaf",
				Type: &compat.Type{Name: "string"},
			}},
		}},
		Uses: []*compat.Uses{{Name: "notif-group"}},
	}}
	leafNode := &compat.Leaf{Name: "probe", Parent: mod}

	root := compat.RootNode(leafNode)
	if root == nil {
		t.Fatal("RootNode returned nil")
	}

	if got, want := len(root.Feature), 1; got != want {
		t.Fatalf("RootNode Feature len = %d, want %d", got, want)
	}
	feature := root.Feature[0]
	if got := feature.ParentNode(); got != root {
		t.Fatalf("Feature ParentNode = %#v, want root module", got)
	}
	if got := feature.Description.ParentNode(); got != feature {
		t.Fatalf("Feature.Description ParentNode = %#v, want cloned feature", got)
	}
	if got := feature.IfFeature[0].ParentNode(); got != feature {
		t.Fatalf("Feature.IfFeature[0] ParentNode = %#v, want cloned feature", got)
	}
	if got := feature.Reference.ParentNode(); got != feature {
		t.Fatalf("Feature.Reference ParentNode = %#v, want cloned feature", got)
	}
	if got := feature.Status.ParentNode(); got != feature {
		t.Fatalf("Feature.Status ParentNode = %#v, want cloned feature", got)
	}

	if got, want := len(root.Extension), 1; got != want {
		t.Fatalf("RootNode Extension len = %d, want %d", got, want)
	}
	extension := root.Extension[0]
	if got := extension.ParentNode(); got != root {
		t.Fatalf("Extension ParentNode = %#v, want root module", got)
	}
	if got := extension.Argument.ParentNode(); got != extension {
		t.Fatalf("Extension.Argument ParentNode = %#v, want cloned extension", got)
	}
	if got := extension.Argument.YinElement.ParentNode(); got != extension.Argument {
		t.Fatalf("Extension.Argument.YinElement ParentNode = %#v, want cloned argument", got)
	}
	if got := extension.Description.ParentNode(); got != extension {
		t.Fatalf("Extension.Description ParentNode = %#v, want cloned extension", got)
	}
	if got := extension.Reference.ParentNode(); got != extension {
		t.Fatalf("Extension.Reference ParentNode = %#v, want cloned extension", got)
	}
	if got := extension.Status.ParentNode(); got != extension {
		t.Fatalf("Extension.Status ParentNode = %#v, want cloned extension", got)
	}

	if got, want := len(root.Deviation), 1; got != want {
		t.Fatalf("RootNode Deviation len = %d, want %d", got, want)
	}
	deviation := root.Deviation[0]
	if got := deviation.ParentNode(); got != root {
		t.Fatalf("Deviation ParentNode = %#v, want root module", got)
	}
	if got := deviation.Description.ParentNode(); got != deviation {
		t.Fatalf("Deviation.Description ParentNode = %#v, want cloned deviation", got)
	}
	if got := deviation.Reference.ParentNode(); got != deviation {
		t.Fatalf("Deviation.Reference ParentNode = %#v, want cloned deviation", got)
	}
	deviate := deviation.Deviate[0]
	if got := deviate.ParentNode(); got != deviation {
		t.Fatalf("Deviation.Deviate[0] ParentNode = %#v, want cloned deviation", got)
	}
	if got := deviate.Config.ParentNode(); got != deviate {
		t.Fatalf("Deviation.Deviate[0].Config ParentNode = %#v, want cloned deviate", got)
	}
	if got := deviate.Default.ParentNode(); got != deviate {
		t.Fatalf("Deviation.Deviate[0].Default ParentNode = %#v, want cloned deviate", got)
	}
	if got := deviate.Mandatory.ParentNode(); got != deviate {
		t.Fatalf("Deviation.Deviate[0].Mandatory ParentNode = %#v, want cloned deviate", got)
	}
	if got := deviate.Must[0].ParentNode(); got != deviate {
		t.Fatalf("Deviation.Deviate[0].Must[0] ParentNode = %#v, want cloned deviate", got)
	}
	if got := deviate.Type.ParentNode(); got != deviate {
		t.Fatalf("Deviation.Deviate[0].Type ParentNode = %#v, want cloned deviate", got)
	}
	if got := deviate.Unique[0].ParentNode(); got != deviate {
		t.Fatalf("Deviation.Deviate[0].Unique[0] ParentNode = %#v, want cloned deviate", got)
	}
	if got := deviate.Units.ParentNode(); got != deviate {
		t.Fatalf("Deviation.Deviate[0].Units ParentNode = %#v, want cloned deviate", got)
	}

	if got, want := len(root.RPC), 1; got != want {
		t.Fatalf("RootNode RPC len = %d, want %d", got, want)
	}
	rpc := root.RPC[0]
	if got := rpc.ParentNode(); got != root {
		t.Fatalf("RPC ParentNode = %#v, want root module", got)
	}
	if got := rpc.Description.ParentNode(); got != rpc {
		t.Fatalf("RPC.Description ParentNode = %#v, want cloned rpc", got)
	}
	if got := rpc.IfFeature[0].ParentNode(); got != rpc {
		t.Fatalf("RPC.IfFeature[0] ParentNode = %#v, want cloned rpc", got)
	}
	if got := rpc.Reference.ParentNode(); got != rpc {
		t.Fatalf("RPC.Reference ParentNode = %#v, want cloned rpc", got)
	}
	if got := rpc.Status.ParentNode(); got != rpc {
		t.Fatalf("RPC.Status ParentNode = %#v, want cloned rpc", got)
	}
	if got := rpc.Grouping[0].ParentNode(); got != rpc {
		t.Fatalf("RPC.Grouping[0] ParentNode = %#v, want cloned rpc", got)
	}
	if got := rpc.Typedef[0].ParentNode(); got != rpc {
		t.Fatalf("RPC.Typedef[0] ParentNode = %#v, want cloned rpc", got)
	}
	if got := rpc.Input.ParentNode(); got != rpc {
		t.Fatalf("RPC.Input ParentNode = %#v, want cloned rpc", got)
	}
	if got := rpc.Input.Leaf[0].ParentNode(); got != rpc.Input {
		t.Fatalf("RPC.Input.Leaf[0] ParentNode = %#v, want cloned input", got)
	}
	if got := rpc.Input.Leaf[0].Type.ParentNode(); got != rpc.Input.Leaf[0] {
		t.Fatalf("RPC.Input.Leaf[0].Type ParentNode = %#v, want cloned leaf", got)
	}
	if got := rpc.Output.ParentNode(); got != rpc {
		t.Fatalf("RPC.Output ParentNode = %#v, want cloned rpc", got)
	}
	if got := rpc.Output.Leaf[0].ParentNode(); got != rpc.Output {
		t.Fatalf("RPC.Output.Leaf[0] ParentNode = %#v, want cloned output", got)
	}
	if got := rpc.Output.Leaf[0].Type.ParentNode(); got != rpc.Output.Leaf[0] {
		t.Fatalf("RPC.Output.Leaf[0].Type ParentNode = %#v, want cloned leaf", got)
	}

	if got, want := len(root.Notification), 1; got != want {
		t.Fatalf("RootNode Notification len = %d, want %d", got, want)
	}
	notification := root.Notification[0]
	if got := notification.ParentNode(); got != root {
		t.Fatalf("Notification ParentNode = %#v, want root module", got)
	}
	if got := notification.Description.ParentNode(); got != notification {
		t.Fatalf("Notification.Description ParentNode = %#v, want cloned notification", got)
	}
	if got := notification.IfFeature[0].ParentNode(); got != notification {
		t.Fatalf("Notification.IfFeature[0] ParentNode = %#v, want cloned notification", got)
	}
	if got := notification.Reference.ParentNode(); got != notification {
		t.Fatalf("Notification.Reference ParentNode = %#v, want cloned notification", got)
	}
	if got := notification.Status.ParentNode(); got != notification {
		t.Fatalf("Notification.Status ParentNode = %#v, want cloned notification", got)
	}
	if got := notification.Leaf[0].ParentNode(); got != notification {
		t.Fatalf("Notification.Leaf[0] ParentNode = %#v, want cloned notification", got)
	}
	if got := notification.Leaf[0].Type.ParentNode(); got != notification.Leaf[0] {
		t.Fatalf("Notification.Leaf[0].Type ParentNode = %#v, want cloned leaf", got)
	}
	if got := notification.Choice[0].ParentNode(); got != notification {
		t.Fatalf("Notification.Choice[0] ParentNode = %#v, want cloned notification", got)
	}
	if got := notification.Uses[0].ParentNode(); got != notification {
		t.Fatalf("Notification.Uses[0] ParentNode = %#v, want cloned notification", got)
	}
}
