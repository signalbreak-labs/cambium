// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

//go:build cgo

package libyangbackend_test

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	cambium "github.com/signalbreak-labs/cambium/go/libyangbackend"
)

// loadDataReadContext creates the cambium-data-read-demo module used by the
// Rust data_read.rs oracle.
func loadDataReadContext(t *testing.T) *cambium.Context {
	t.Helper()
	dir := t.TempDir()
	module := filepath.Join(dir, "cambium-data-read-demo.yang")
	src := `module cambium-data-read-demo {
  namespace "urn:cambium:data-read";
  prefix cdr;
  yang-version 1.1;
  revision 2026-06-14;

  container top {
    leaf rw-flag {
      type boolean;
      default "true";
    }
    leaf ro-counter {
      config false;
      type uint64;
    }
    leaf deprecated-leaf {
      status deprecated;
      type string;
    }
    leaf mandatory-leaf {
      mandatory "true";
      type string;
    }
    leaf all-builtins {
      type int64;
    }
    leaf dec64 {
      type decimal64 { fraction-digits 4; }
    }
    leaf status-enum {
      type enumeration {
        enum up { value 1; }
        enum down { value 2; }
        enum unknown { value 0; }
      }
    }
  }
}`
	if err := os.WriteFile(module, []byte(src), 0644); err != nil {
		t.Fatal(err)
	}
	ctx, err := cambium.NewContext()
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}
	if err := ctx.SetSearchPath(dir); err != nil {
		ctx.Close()
		t.Fatalf("SetSearchPath: %v", err)
	}
	if err := ctx.LoadModule("cambium-data-read-demo"); err != nil {
		ctx.Close()
		t.Fatalf("LoadModule: %v", err)
	}
	return ctx
}

// parseDataRead parses the deliberately scrambled demo input.
func parseDataRead(t *testing.T, ctx *cambium.Context) *cambium.DataTree {
	t.Helper()
	xml := `<top xmlns="urn:cambium:data-read">
  <mandatory-leaf>required</mandatory-leaf>
  <rw-flag>true</rw-flag>
  <status-enum>up</status-enum>
  <dec64>12.3400</dec64>
  <all-builtins>-7</all-builtins>
</top>`
	tree, err := ctx.Parse(cambium.FormatXML, cambium.ParseModeDataOnly, []byte(xml))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	return tree
}

func TestGetValueTyped(t *testing.T) {
	ctx := loadDataReadContext(t)
	defer ctx.Close()
	tree := parseDataRead(t, ctx)
	defer tree.Close()

	rw, err := tree.Get("/cambium-data-read-demo:top/rw-flag")
	if err != nil {
		t.Fatalf("Get rw-flag: %v", err)
	}
	v, ok, err := rw.Value()
	if err != nil || !ok {
		t.Fatalf("Value rw-flag: ok=%v err=%v", ok, err)
	}
	if b, ok := v.Bool(); !ok || b != true {
		t.Fatalf("rw-flag = %v", b)
	}

	all, err := tree.Get("/cambium-data-read-demo:top/all-builtins")
	if err != nil {
		t.Fatalf("Get all-builtins: %v", err)
	}
	v, ok, err = all.Value()
	if err != nil || !ok {
		t.Fatalf("Value all-builtins: ok=%v err=%v", ok, err)
	}
	if i, ok := v.Int64(); !ok || i != -7 {
		t.Fatalf("all-builtins = %d", i)
	}

	dec, err := tree.Get("/cambium-data-read-demo:top/dec64")
	if err != nil {
		t.Fatalf("Get dec64: %v", err)
	}
	v, ok, err = dec.Value()
	if err != nil || !ok {
		t.Fatalf("Value dec64: ok=%v err=%v", ok, err)
	}
	d, ok := v.Decimal64()
	if !ok || d.Raw() != 123400 || d.FractionDigits() != 4 {
		t.Fatalf("dec64 = %v", d)
	}

	enm, err := tree.Get("/cambium-data-read-demo:top/status-enum")
	if err != nil {
		t.Fatalf("Get status-enum: %v", err)
	}
	v, ok, err = enm.Value()
	if err != nil || !ok {
		t.Fatalf("Value status-enum: ok=%v err=%v", ok, err)
	}
	if s, ok := v.Enum(); !ok || s != "up" {
		t.Fatalf("status-enum = %q", s)
	}
}

func TestGetValueStrCanonical(t *testing.T) {
	ctx := loadDataReadContext(t)
	defer ctx.Close()
	tree := parseDataRead(t, ctx)
	defer tree.Close()

	mustStr := func(path, want string) {
		t.Helper()
		node, err := tree.Get(path)
		if err != nil {
			t.Fatalf("Get %s: %v", path, err)
		}
		s, ok, err := node.ValueStr()
		if err != nil || !ok || s != want {
			t.Fatalf("ValueStr(%s) = %q ok=%v err=%v, want %q", path, s, ok, err, want)
		}
	}

	mustStr("/cambium-data-read-demo:top/rw-flag", "true")
	mustStr("/cambium-data-read-demo:top/all-builtins", "-7")
	mustStr("/cambium-data-read-demo:top/dec64", "12.34")
}

func TestGetTryGetExists(t *testing.T) {
	ctx := loadDataReadContext(t)
	defer ctx.Close()
	tree := parseDataRead(t, ctx)
	defer tree.Close()

	if !tree.Exists("/cambium-data-read-demo:top/rw-flag") {
		t.Fatal("Exists rw-flag should be true")
	}
	if _, ok := tree.TryGet("/cambium-data-read-demo:top/rw-flag"); !ok {
		t.Fatal("TryGet rw-flag should be ok=true")
	}
	if tree.Exists("/cambium-data-read-demo:top/no-such") {
		t.Fatal("Exists no-such should be false")
	}
	if _, ok := tree.TryGet("/cambium-data-read-demo:top/no-such"); ok {
		t.Fatal("TryGet no-such should be ok=false")
	}

	_, err := tree.Get("/cambium-data-read-demo:top/no-such")
	if err == nil {
		t.Fatal("Get no-such should error")
	}
	if e, ok := err.(*cambium.Error); !ok || e.RuleCode() != cambium.RuleCodeDataPath {
		t.Fatalf("Get no-such code = %v, want E0006", err)
	}
}

func TestSelectDocumentOrder(t *testing.T) {
	ctx := loadDataReadContext(t)
	defer ctx.Close()
	tree := parseDataRead(t, ctx)
	defer tree.Close()

	set, err := tree.Select("/cambium-data-read-demo:top/*")
	if err != nil {
		t.Fatalf("Select: %v", err)
	}
	want := []string{"rw-flag", "mandatory-leaf", "all-builtins", "dec64", "status-enum"}
	got := make([]string, set.Len())
	for i := 0; i < set.Len(); i++ {
		node, _ := set.Get(i)
		got[i], _ = node.Name()
	}
	if !slices.Equal(got, want) {
		t.Fatalf("select order = %v, want %v", got, want)
	}
}

func TestChildrenOneFFIWalkOrdered(t *testing.T) {
	ctx := loadDataReadContext(t)
	defer ctx.Close()
	tree := parseDataRead(t, ctx)
	defer tree.Close()

	top, err := tree.Get("/cambium-data-read-demo:top")
	if err != nil {
		t.Fatalf("Get top: %v", err)
	}
	children, err := top.Children()
	if err != nil {
		t.Fatalf("Children: %v", err)
	}
	want := []string{"rw-flag", "mandatory-leaf", "all-builtins", "dec64", "status-enum"}
	got := make([]string, children.Len())
	for i := 0; i < children.Len(); i++ {
		node, _ := children.Get(i)
		got[i], _ = node.Name()
	}
	if !slices.Equal(got, want) {
		t.Fatalf("children order = %v, want %v", got, want)
	}
}

func TestNodeSchemaBridge(t *testing.T) {
	ctx := loadDataReadContext(t)
	defer ctx.Close()
	tree := parseDataRead(t, ctx)
	defer tree.Close()

	rw, err := tree.Get("/cambium-data-read-demo:top/rw-flag")
	if err != nil {
		t.Fatalf("Get rw-flag: %v", err)
	}
	schema, err := rw.Schema()
	if err != nil {
		t.Fatalf("Schema rw-flag: %v", err)
	}
	if schema.Name() != "rw-flag" {
		t.Fatalf("schema name = %q", schema.Name())
	}
	info, ok := schema.LeafType()
	if !ok || info.Base() != cambium.BaseTypeBoolean {
		t.Fatalf("rw-flag base = %v, want boolean", info.Base())
	}

	all, err := tree.Get("/cambium-data-read-demo:top/all-builtins")
	if err != nil {
		t.Fatalf("Get all-builtins: %v", err)
	}
	schema, err = all.Schema()
	if err != nil {
		t.Fatalf("Schema all-builtins: %v", err)
	}
	info, ok = schema.LeafType()
	if !ok || info.Base() != cambium.BaseTypeInt64 {
		t.Fatalf("all-builtins base = %v, want int64", info.Base())
	}
}
