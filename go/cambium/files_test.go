// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

package cambium_test

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/signalbreak-labs/cambium/go/cambium"
	upstream "github.com/signalbreak-labs/cambium/go/internal/yangparse/upstream/yang"
)

func TestPathsWithModulesMatchesGoyang(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "root.yang"), "module root { namespace \"urn:root\"; prefix r; }")
	writeTestFile(t, filepath.Join(root, "root-too.yang"), "module root-too { namespace \"urn:root-too\"; prefix rt; }")
	writeTestFile(t, filepath.Join(root, "nested", "one.yang"), "module one { namespace \"urn:one\"; prefix o; }")
	writeTestFile(t, filepath.Join(root, "nested", "deeper", "two.yang"), "module two { namespace \"urn:two\"; prefix t; }")
	writeTestFile(t, filepath.Join(root, "ignored", "readme.txt"), "not yang")

	got, err := cambium.PathsWithModules(root)
	if err != nil {
		t.Fatalf("PathsWithModules: %v", err)
	}
	want, err := upstream.PathsWithModules(root)
	if err != nil {
		t.Fatalf("goyang PathsWithModules: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("PathsWithModules = %v, want goyang %v", got, want)
	}
}

func TestPathsWithModulesMatchesGoyangErrors(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing")

	got, gotErr := cambium.PathsWithModules(missing)
	want, wantErr := upstream.PathsWithModules(missing)
	if (gotErr == nil) != (wantErr == nil) {
		t.Fatalf("error nil = %v, want goyang %v (got paths %v want paths %v)", gotErr == nil, wantErr == nil, got, want)
	}
	if gotErr == nil && !reflect.DeepEqual(got, want) {
		t.Fatalf("PathsWithModules = %v, want goyang %v", got, want)
	}
}

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
