// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

//go:build cgo

package libyang

import (
	"os"
	"strings"
	"testing"
)

// Build-gate smoke test: ly_ctx_new must return a non-nil context, proving the
// vendored static libyang + PCRE2 are compiled and linked.
func TestContextSmoke(t *testing.T) {
	ctx, err := NewContext()
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}
	if ctx == nil || ctx.ctx == nil {
		t.Fatal("NewContext returned a nil context")
	}
	ctx.Close()
}

// NewData creates an empty tree that still carries its context so later
// path-based mutations can resolve schema nodes.
func TestRawDataTreeNewDataCarriesContext(t *testing.T) {
	ctx, err := NewContext()
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}
	defer ctx.Close()

	tree := ctx.NewData()
	if tree == nil {
		t.Fatal("NewData returned nil")
	}
	if tree.tree != nil {
		t.Fatalf("NewData tree pointer = %p, want nil", tree.tree)
	}
	if tree.owner != ctx {
		t.Fatal("NewData owner mismatch")
	}
	if tree.ctx != ctx.ctx {
		t.Fatal("NewData ctx pointer mismatch")
	}

	// Close on an empty tree must be safe (the finalizer guards nil tree).
	tree.Close()
	if tree.tree != nil {
		t.Fatal("Close did not nil the tree pointer")
	}
}

// Merge and Diff across two different contexts must be rejected at the raw layer
// (matching the Rust adapter); passing two ly_ctx to libyang is undefined
// behavior. Empty NewData trees keep the red case benign (no node deref).
func TestMergeDiffRejectCrossContext(t *testing.T) {
	c1, err := NewContext()
	if err != nil {
		t.Fatalf("NewContext c1: %v", err)
	}
	defer c1.Close()
	c2, err := NewContext()
	if err != nil {
		t.Fatalf("NewContext c2: %v", err)
	}
	defer c2.Close()

	t1 := c1.NewData()
	t2 := c2.NewData()

	if err := t1.Merge(t2); err == nil {
		t.Fatal("Merge across contexts must return an error")
	}
	if _, err := t1.Diff(t2, false); err == nil {
		t.Fatal("Diff across contexts must return an error")
	}
}

func TestNodeValueStrDirectUsesLydGetValue(t *testing.T) {
	src, err := os.ReadFile("libyang.go")
	if err != nil {
		t.Fatal(err)
	}
	body := functionSource(t, string(src), "func nodeValueStrDirect")
	if !strings.Contains(body, "cam_lyd_get_value") {
		t.Fatal("nodeValueStrDirect must read values through lyd_get_value")
	}
	if strings.Contains(body, "._canonical") {
		t.Fatal("nodeValueStrDirect must not read lyd_value._canonical directly")
	}
}

func TestMetaValueStrUsesLydGetMetaValue(t *testing.T) {
	src, err := os.ReadFile("libyang.go")
	if err != nil {
		t.Fatal(err)
	}
	body := functionSource(t, string(src), "func (d *RawDataDiff) metaValueStr")
	if !strings.Contains(body, "cam_lyd_get_meta_value") {
		t.Fatal("metaValueStr must read metadata through lyd_get_meta_value")
	}
	if strings.Contains(body, "._canonical") {
		t.Fatal("metaValueStr must not read lyd_value._canonical directly")
	}
}

func functionSource(t *testing.T, src, signature string) string {
	t.Helper()
	start := strings.Index(src, signature)
	if start < 0 {
		t.Fatalf("%s not found", signature)
	}
	open := strings.Index(src[start:], "{")
	if open < 0 {
		t.Fatalf("%s has no body", signature)
	}
	open += start
	depth := 0
	for idx := open; idx < len(src); idx++ {
		switch src[idx] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return src[open : idx+1]
			}
		}
	}
	t.Fatalf("%s body did not terminate", signature)
	return ""
}