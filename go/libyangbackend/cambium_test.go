// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

//go:build cgo

package libyangbackend_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	cambium "github.com/signalbreak-labs/cambium/go/libyangbackend"
)

// findConformance walks up from the test's working directory to the shared
// conformance corpus.
func findConformance(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "conformance", "manifest.toml")); err == nil {
			return filepath.Join(dir, "conformance")
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("could not locate conformance/manifest.toml")
		}
		dir = parent
	}
}

// TestParseSerializeOrder is the safe-API smoke test: load order-demo, parse a
// document whose children are scrambled, and confirm Cambium serializes them in
// schema declaration order (z, then m, then a) — the core value proposition.
func TestParseSerializeOrder(t *testing.T) {
	conf := findConformance(t)
	moduleDir := filepath.Join(conf, "fixtures", "scrambled-children", "module")
	input, err := os.ReadFile(filepath.Join(conf, "fixtures", "scrambled-children", "input.xml"))
	if err != nil {
		t.Fatal(err)
	}

	ctx, err := cambium.NewContext()
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}
	defer ctx.Close()
	if err := ctx.SetSearchPath(moduleDir); err != nil {
		t.Fatalf("SetSearchPath: %v", err)
	}
	if err := ctx.LoadModule("order-demo"); err != nil {
		t.Fatalf("LoadModule: %v", err)
	}

	tree, err := ctx.Parse(cambium.FormatXML, cambium.ParseModeDataOnly, input)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	defer tree.Close()

	out, err := tree.Serialize(cambium.FormatXML, cambium.DefaultSerializeFlags())
	if err != nil {
		t.Fatalf("Serialize: %v", err)
	}

	s := string(out)
	zi, mi, ai := strings.Index(s, "<z>"), strings.Index(s, "<m>"), strings.Index(s, "<a>")
	if zi < 0 || mi < 0 || ai < 0 {
		t.Fatalf("missing elements in output:\n%s", s)
	}
	if zi >= mi || mi >= ai {
		t.Fatalf("children not in schema declaration order (want z<m<a):\n%s", s)
	}
}
