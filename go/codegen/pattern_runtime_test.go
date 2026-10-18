// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

package codegen

import (
	"bytes"
	"errors"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"slices"
	"strings"
	"testing"

	"github.com/signalbreak-labs/cambium/go/cambium"
	"github.com/signalbreak-labs/cambium/go/yangpattern"
)

func TestPatternRuntimeInventoryRendering(t *testing.T) {
	inventory := yangpattern.RuntimeSources()
	if len(inventory) != 5 {
		t.Fatalf("inventory changed: %d files", len(inventory))
	}
	runtime, err := readPatternRuntime(inventory)
	if err != nil {
		t.Fatal(err)
	}
	generated := normalizedDeclarations(t, "package generated\n"+runtime.declarations)
	count := 0
	for _, source := range inventory {
		if !strings.Contains(runtime.declarations, "// Shared yangpattern runtime: "+source.Name+"\n// SPDX-License-Identifier: Apache-2.0\n// Copyright 2026 signalbreak-labs") {
			t.Fatalf("missing attribution for %s", source.Name)
		}
		for _, decl := range normalizedDeclarations(t, string(source.Data)) {
			if generated[count] != decl {
				t.Fatalf("declaration %d in %s changed", count, source.Name)
			}
			count++
		}
	}
	if count != len(generated) {
		t.Fatalf("runtime has %d extra declarations", len(generated)-count)
	}
}

// Parse independently without comments, then format each entire declaration.
// This checks every type, constant, variable, function and method body, including
// unexported declarations, without sharing the renderer's slicing algorithm.
func normalizedDeclarations(t *testing.T, source string) []string {
	t.Helper()
	fs := token.NewFileSet()
	file, err := parser.ParseFile(fs, "source.go", source, 0)
	if err != nil {
		t.Fatal(err)
	}
	var result []string
	for _, decl := range file.Decls {
		if gen, ok := decl.(*ast.GenDecl); ok && gen.Tok == token.IMPORT {
			continue
		}
		var b bytes.Buffer
		if err := format.Node(&b, fs, decl); err != nil {
			t.Fatal(err)
		}
		result = append(result, b.String())
	}
	return result
}

func TestPatternRuntimeInvalidSourceIsFallible(t *testing.T) {
	_, err := readPatternRuntime([]yangpattern.SourceFile{{Name: "broken.go", Data: []byte("package bad\nfunc broken(")}})
	if err == nil || !strings.Contains(err.Error(), "broken.go") {
		t.Fatalf("parse error missing: %v", err)
	}
	g := &goEmitter{}
	g.recordEmitError(err)
	if g.emitErr != err {
		t.Fatal("emitter discarded parse cause")
	}
}

func TestPatternDefaultOperationalErrors(t *testing.T) {
	b := make([]byte, 1000000)
	x := uint64(0x9e3779b97f4a7c15)
	for i := range b {
		x ^= x << 13
		x ^= x >> 7
		x ^= x << 17
		b[i] = 'a' + byte(x&1)
	}
	builder, err := cambium.NewContextBuilder(cambium.ContextFlags{DisableSearchdirCwd: true})
	if err != nil {
		t.Fatal(err)
	}
	if err := builder.LoadModuleStr(`module defaults-probe { yang-version 1.1; namespace "urn:defaults-probe"; prefix p;
 leaf donor-z { type string; default 'z'; }
 leaf donor-a { type string; default 'a'; }
 leaf donor { type string; default '` + string(b) + `'; }
 leaf target { type string { pattern '[ab]*a[ab]{1001}'; } }
 leaf nested { type union { type union { type string { pattern '[ab]*a[ab]{1001}'; } type boolean; } type string; } }
 leaf reference { type leafref { path '../target'; require-instance false; } }
 leaf inverted { type string { pattern '[ab]*a[ab]{1001}' { modifier invert-match; } } }
 leaf-list ordered { type union { type string { pattern '[ab]*a[ab]{1001}'; } type string; } }
}`); err != nil {
		t.Fatal(err)
	}
	ctx, err := builder.Build()
	if err != nil {
		t.Fatal(err)
	}
	defer ctx.Close()
	mod, err := ctx.Schema("defaults-probe")
	if err != nil {
		t.Fatal(err)
	}
	// The unrestricted donor supplies a public immutable DefaultValue. Passing
	// that DTO to a different type exercises helpers without mutating a schema.
	var def, defZ, defA cambium.DefaultValue
	for n := range mod.TopLevel().Iter() {
		switch n.Name() {
		case "donor":
			def, _ = n.DefaultEntry()
		case "donor-z":
			defZ, _ = n.DefaultEntry()
		case "donor-a":
			defA, _ = n.DefaultEntry()
		}
	}
	check := func(err error) {
		t.Helper()
		r, ok := errors.AsType[*yangpattern.ResourceError](err)
		if !ok || r.Phase != "match" {
			t.Fatalf("lost operational error: %v", err)
		}
	}
	for n := range mod.TopLevel().Iter() {
		if strings.HasPrefix(n.Name(), "donor") {
			continue
		}
		t.Run(n.Name(), func(t *testing.T) {
			info, _ := n.LeafType()
			g := &goEmitter{module: mod, moduleName: mod.Name()}
			g.initPlanningState()
			_, _, err := g.typeDefaultJSONLiteralExpr(info, def, mod)
			check(err)
			_, _, err = g.defaultTextForType(info, def, mod)
			check(err)
			f := fieldInfo{node: n, isUnion: n.Name() == "nested" || n.Name() == "ordered"}
			_, err = g.defaultValueSortKey(info, def, f)
			check(err)
			_ = g.defaultJSONLiteralExpr(f, def)
			check(g.emitErr)
			if n.Name() == "ordered" {
				// Two short values successfully select the later string arm. Their
				// input order is deliberately reversed before the final failing key.
				values := []cambium.DefaultValue{defZ, defA, def}
				before := slices.Clone(values)
				for i, value := range values[:2] {
					key, err := g.defaultValueSortKey(info, value, f)
					if err != nil || key.text != before[i].Value() {
						t.Fatalf("short key %d: %q, %v", i, key.text, err)
					}
				}
				_, err = g.orderedDefaultValues(values, f)
				check(err)
				if !slices.Equal(values, before) {
					t.Fatal("input default sequence mutated after key failure")
				}
				// Complementary successful helper control also preserves its input.
				short := slices.Clone(before[:2])
				ordered, err := g.orderedDefaultValues(short, f)
				if err != nil {
					t.Fatal(err)
				}
				if !slices.Equal(ordered, []cambium.DefaultValue{defA, defZ}) || !slices.Equal(short, before[:2]) {
					t.Fatal("successful default order/input preservation")
				}
			}
		})
	}
}
