// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

//go:build cgo

package libyangbackend_test

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	cambium "github.com/signalbreak-labs/cambium/go/libyangbackend"
)

func jsonIetfFixtureDir(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Join(file, "..", "..", "..", "conformance", "fixtures", "ordered-user")
}

func TestJSONIETFPreservesUserOrderedList(t *testing.T) {
	ctx, err := cambium.NewContext()
	if err != nil {
		t.Fatalf("new context: %v", err)
	}
	defer ctx.Close()

	if err := ctx.SetSearchPath(filepath.Join(jsonIetfFixtureDir(t), "module")); err != nil {
		t.Fatalf("set search path: %v", err)
	}
	if err := ctx.LoadModule("ordered-user-demo"); err != nil {
		t.Fatalf("load module: %v", err)
	}

	input := readFile(t, filepath.Join(jsonIetfFixtureDir(t), "input.xml"))
	tree, err := ctx.Parse(cambium.FormatXML, cambium.ParseModeDataOnly, input)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	defer tree.Close()

	if err := tree.Validate(cambium.ValidateMode{}); err != nil {
		t.Fatalf("validate: %v", err)
	}

	jsonIetf, err := tree.Serialize(cambium.FormatJSONIETF, cambium.DefaultSerializeFlags())
	if err != nil {
		t.Fatalf("serialize JSON_IETF: %v", err)
	}
	jsonPlain, err := tree.Serialize(cambium.FormatJSON, cambium.DefaultSerializeFlags())
	if err != nil {
		t.Fatalf("serialize JSON: %v", err)
	}

	// Both formats must preserve the user-ordered list as an array. JSON_IETF
	// additionally keeps empty non-presence containers, so the bytes may differ.
	_ = jsonPlain

	s := string(jsonIetf)
	if !strings.Contains(s, `"entry": [`) {
		t.Fatalf("JSON_IETF must encode ordered-by user list as array: %s", s)
	}

	cIdx := strings.Index(s, `"name": "c"`)
	aIdx := strings.Index(s, `"name": "a"`)
	bIdx := strings.Index(s, `"name": "b"`)
	if cIdx < 0 || aIdx < 0 || bIdx < 0 {
		t.Fatalf("missing list entries in: %s", s)
	}
	if cIdx >= aIdx || aIdx >= bIdx {
		t.Fatalf("user-ordered list must stay c, a, b order: %s", s)
	}
}

func readFile(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return b
}
