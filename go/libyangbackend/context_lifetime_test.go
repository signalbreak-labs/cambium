// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

//go:build cgo

package libyangbackend_test

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	cambium "github.com/signalbreak-labs/cambium/go/libyangbackend"
)

func lifetimeContext(t *testing.T) *cambium.Context {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "lifeclose.yang"), []byte(`module lifeclose {
  namespace "urn:lifeclose";
  prefix lc;
  revision 2026-06-14;
  container top { leaf x { type string; } }
}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx, err := cambium.NewContext()
	if err != nil {
		t.Fatal(err)
	}
	if err := ctx.SetSearchPath(dir); err != nil {
		ctx.Close()
		t.Fatal(err)
	}
	if err := ctx.LoadModule("lifeclose"); err != nil {
		ctx.Close()
		t.Fatal(err)
	}
	return ctx
}

const lifetimeDoc = `<top xmlns="urn:lifeclose"><x>1</x></top>`

// TestContextCloseBeforeTreeFreed guards a use-after-free: a data tree
// references the context's schema/dictionary, so closing the context while a
// tree is still alive must NOT crash when the tree is later freed. The context
// refcounts its trees and defers ly_ctx_destroy until the last is freed. This
// is the explicit-Close path; the GC-finalizer path shares the same free/release
// code and is exercised under GC pressure by the rest of the suite.
func TestContextCloseBeforeTreeFreed(t *testing.T) {
	ctx := lifetimeContext(t)
	tree, err := ctx.Parse(cambium.FormatXML, cambium.ParseModeDataOnly, []byte(lifetimeDoc))
	if err != nil {
		t.Fatal(err)
	}

	// Close the context BEFORE the tree. Pre-fix this destroyed the context
	// immediately, so freeing the tree below faulted (lyd_free_all over freed
	// schema/dictionary). The fix defers ly_ctx_destroy until the tree is freed.
	ctx.Close()
	tree.Close() // must not crash; frees the tree, then destroys the context
}

// TestContextCloseBeforeTreeFinalized covers the same hazard via the GC
// finalizer rather than an explicit tree.Close().
func TestContextCloseBeforeTreeFinalized(t *testing.T) {
	ctx := lifetimeContext(t)
	tree, err := ctx.Parse(cambium.FormatXML, cambium.ParseModeDataOnly, []byte(lifetimeDoc))
	if err != nil {
		t.Fatal(err)
	}

	// Keep the tree alive THROUGH the context Close (so the close-before-free
	// ordering holds), then let it become collectable. After KeepAlive the tree
	// is unreachable, so the GC runs its finalizer (lyd_free_all).
	ctx.Close()
	runtime.KeepAlive(tree)
	for i := 0; i < 10; i++ {
		runtime.GC()
	}
	// Reaching here without a SIGSEGV is the assertion.
}
