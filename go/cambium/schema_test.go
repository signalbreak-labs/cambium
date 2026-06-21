// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

package cambium_test

import (
	"path/filepath"
	"runtime"
	"testing"

	"github.com/signalbreak-labs/cambium/go/cambium"
)

func schemaFixtureDir(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Join(file, "..", "..", "..", "conformance", "fixtures", "scrambled-children")
}

func TestSchemaTreeChildrenInDeclarationOrder(t *testing.T) {
	ctx, err := cambium.NewContext()
	if err != nil {
		t.Fatalf("new context: %v", err)
	}
	defer ctx.Close()

	if err := ctx.SetSearchPath(filepath.Join(schemaFixtureDir(t), "module")); err != nil {
		t.Fatalf("set search path: %v", err)
	}
	if err := ctx.LoadModule("order-demo"); err != nil {
		t.Fatalf("load module: %v", err)
	}

	tree, err := ctx.SchemaTree("order-demo")
	if err != nil {
		t.Fatalf("schema tree: %v", err)
	}

	top := tree.Find([]string{"top"})
	if top == nil {
		t.Fatal("top container not found")
	}
	if top.Kind() != cambium.SchemaNodeKindContainer {
		t.Fatalf("top kind = %v, want container", top.Kind())
	}

	var got []string
	for _, c := range top.Children() {
		got = append(got, c.Name())
	}
	want := []string{"z", "m", "a"}
	if len(got) != len(want) {
		t.Fatalf("children = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("children = %v, want %v", got, want)
		}
	}
}

func TestSchemaTreePreOrder(t *testing.T) {
	ctx, err := cambium.NewContext()
	if err != nil {
		t.Fatalf("new context: %v", err)
	}
	defer ctx.Close()

	if err := ctx.SetSearchPath(filepath.Join(schemaFixtureDir(t), "module")); err != nil {
		t.Fatalf("set search path: %v", err)
	}
	if err := ctx.LoadModule("order-demo"); err != nil {
		t.Fatalf("load module: %v", err)
	}

	tree, err := ctx.SchemaTree("order-demo")
	if err != nil {
		t.Fatalf("schema tree: %v", err)
	}

	var got []string
	tree.PreOrder(func(n *cambium.SchemaNode) bool {
		got = append(got, n.Name())
		return true
	})
	want := []string{"", "top", "z", "m", "a"}
	if len(got) != len(want) {
		t.Fatalf("preorder = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("preorder = %v, want %v", got, want)
		}
	}
}

func TestSchemaTreeZeroValueMethodsDoNotPanic(t *testing.T) {
	assertNoPanic := func(name string, fn func()) {
		t.Helper()
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("%s panicked: %v", name, r)
			}
		}()
		fn()
	}

	var nilTree *cambium.SchemaTree
	assertNoPanic("nil Find", func() {
		if got := nilTree.Find([]string{"top"}); got != nil {
			t.Fatalf("nil Find returned %v, want nil", got)
		}
	})
	assertNoPanic("nil PreOrder", func() {
		called := false
		nilTree.PreOrder(func(*cambium.SchemaNode) bool {
			called = true
			return true
		})
		if called {
			t.Fatal("nil PreOrder called visitor")
		}
	})

	var zero cambium.SchemaTree
	assertNoPanic("zero Find", func() {
		if got := zero.Find([]string{"top"}); got != nil {
			t.Fatalf("zero Find returned %v, want nil", got)
		}
	})
	assertNoPanic("zero PreOrder", func() {
		called := false
		zero.PreOrder(func(*cambium.SchemaNode) bool {
			called = true
			return true
		})
		if called {
			t.Fatal("zero PreOrder called visitor")
		}
	})
}
