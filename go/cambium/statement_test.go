// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

package cambium_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/signalbreak-labs/cambium/go/cambium"
)

func TestParseStatementsNativeAPI(t *testing.T) {
	const source = `module cambium-native-statements {
  namespace "urn:cambium:native-statements";
  prefix cns;
  x:flag;

  container top {
    leaf z { type string; }
    leaf m { type string; default ""; }
    leaf a { type string; }
  }
}`

	stmts, err := cambium.ParseStatements(source, "native-statements.yang")
	if err != nil {
		t.Fatalf("ParseStatements: %v", err)
	}
	if len(stmts) != 1 {
		t.Fatalf("top-level statements = %d, want 1", len(stmts))
	}
	mod := stmts[0]
	if got := mod.Keyword(); got != "module" {
		t.Fatalf("module Keyword = %q, want module", got)
	}
	if got, ok := mod.Argument(); !ok || got != "cambium-native-statements" {
		t.Fatalf("module Argument = (%q,%v), want cambium-native-statements,true", got, ok)
	}

	flag := findNativeStatement(mod, "x:flag", "")
	if !flag.IsValid() {
		t.Fatal("x:flag statement not found")
	}
	if got, ok := flag.Argument(); ok || got != "" {
		t.Fatalf("x:flag Argument = (%q,%v), want empty,false", got, ok)
	}

	top := findNativeStatement(mod, "container", "top")
	if !top.IsValid() {
		t.Fatal("container top not found")
	}
	got := statementChildLabels(top)
	want := []string{"leaf:z", "leaf:m", "leaf:a"}
	if len(got) != len(want) {
		t.Fatalf("container children = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("container children = %v, want %v", got, want)
		}
	}

	def := findNativeStatement(top, "default", "")
	if !def.IsValid() {
		t.Fatal("default statement not found")
	}
	if got, ok := def.Argument(); !ok || got != "" {
		t.Fatalf("default Argument = (%q,%v), want empty,true", got, ok)
	}

	if got := def.Location(); got == "" || got == "unknown" {
		t.Fatalf("default Location = %q, want source location", got)
	}
}

func TestReadStatementsNativeAPI(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "native-read.yang")
	const source = `module native-read {
  namespace "urn:native-read";
  prefix nr;
}`
	if err := os.WriteFile(path, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}

	stmts, err := cambium.ReadStatements(path)
	if err != nil {
		t.Fatalf("ReadStatements: %v", err)
	}
	if len(stmts) != 1 {
		t.Fatalf("statements = %d, want 1", len(stmts))
	}
	if got, ok := stmts[0].Argument(); !ok || got != "native-read" {
		t.Fatalf("module Argument = (%q,%v), want native-read,true", got, ok)
	}
}

func TestStatementZeroValue(t *testing.T) {
	var st cambium.Statement
	if st.IsValid() {
		t.Fatal("zero Statement IsValid = true, want false")
	}
	if got := st.Keyword(); got != "" {
		t.Fatalf("zero Keyword = %q, want empty", got)
	}
	if got, ok := st.Argument(); ok || got != "" {
		t.Fatalf("zero Argument = (%q,%v), want empty,false", got, ok)
	}
	if got := st.SubStatements(); got != nil {
		t.Fatalf("zero SubStatements = %v, want nil", got)
	}
	if got := st.Location(); got != "unknown" {
		t.Fatalf("zero Location = %q, want unknown", got)
	}
}

func findNativeStatement(root cambium.Statement, keyword, arg string) cambium.Statement {
	for _, child := range root.SubStatements() {
		childArg, childHasArg := child.Argument()
		if child.Keyword() == keyword && (!childHasArg && arg == "" || childArg == arg) {
			return child
		}
		if found := findNativeStatement(child, keyword, arg); found.IsValid() {
			return found
		}
	}
	return cambium.Statement{}
}

func statementChildLabels(st cambium.Statement) []string {
	var out []string
	for _, child := range st.SubStatements() {
		arg, _ := child.Argument()
		out = append(out, child.Keyword()+":"+arg)
	}
	return out
}
