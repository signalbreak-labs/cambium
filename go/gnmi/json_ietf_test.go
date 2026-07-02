// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

//go:build cgo

package gnmi_test

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/signalbreak-labs/cambium/go/gnmi"
	backend "github.com/signalbreak-labs/cambium/go/libyangbackend"
)

func TestJSONIETFAtomicUpdateCarriesUserOrderedListAsOneValue(t *testing.T) {
	_, tree := loadGNMIOrderedAtomic(t)

	update, err := gnmi.JSONIETFAtomicUpdate(tree, "/gnmi-ordered-atomic:top/rule", backend.DefaultSerializeFlags())
	if err != nil {
		t.Fatalf("JSONIETFAtomicUpdate: %v", err)
	}
	if update.Path != "/gnmi-ordered-atomic:top/rule" {
		t.Fatalf("Path = %q", update.Path)
	}
	if update.Encoding != gnmi.EncodingJSONIETF {
		t.Fatalf("Encoding = %q, want %q", update.Encoding, gnmi.EncodingJSONIETF)
	}

	var rules []struct {
		Name   string `json:"name"`
		Action string `json:"action"`
	}
	if err := json.Unmarshal(update.Value, &rules); err != nil {
		t.Fatalf("atomic value is not one JSON array: %v\n%s", err, update.Value)
	}
	got := make([]string, len(rules))
	for i, rule := range rules {
		got[i] = rule.Name + ":" + rule.Action
	}
	want := []string{"c:drop", "a:accept", "b:reject"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("ordered-by user value = %v, want %v", got, want)
		}
	}
}

func TestJSONIETFAtomicUpdateRejectsPredicatedPath(t *testing.T) {
	_, tree := loadGNMIOrderedAtomic(t)

	update, err := gnmi.JSONIETFAtomicUpdate(tree, "/gnmi-ordered-atomic:top/rule[name='a']", backend.DefaultSerializeFlags())
	if err == nil {
		t.Fatalf("JSONIETFAtomicUpdate predicated path returned value %s, want error", update.Value)
	}
	var backendErr *backend.Error
	if !errors.As(err, &backendErr) || backendErr.RuleCode() != backend.RuleCodeDataPath {
		t.Fatalf("predicated path error = %v, want RuleCodeDataPath", err)
	}
	if !strings.Contains(err.Error(), "predicates are not supported; pass the list path for an atomic I6 update") {
		t.Fatalf("predicated path error = %v, want unsupported predicate guidance", err)
	}
}

func loadGNMIOrderedAtomic(t *testing.T) (*backend.Context, *backend.DataTree) {
	t.Helper()
	conf := findConformance(t)
	moduleDir := filepath.Join(conf, "fixtures", "gnmi-ordered-atomic", "module")
	inputPath := filepath.Join(conf, "fixtures", "gnmi-ordered-atomic", "input.xml")

	ctx, err := backend.NewContext()
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}
	t.Cleanup(ctx.Close)
	if err := ctx.SetSearchPath(moduleDir); err != nil {
		t.Fatalf("SetSearchPath: %v", err)
	}
	if err := ctx.LoadModule("gnmi-ordered-atomic"); err != nil {
		t.Fatalf("LoadModule: %v", err)
	}
	input, err := os.ReadFile(inputPath)
	if err != nil {
		t.Fatalf("read input: %v", err)
	}
	tree, err := ctx.Parse(backend.FormatXML, backend.ParseModeDataOnly, input)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	t.Cleanup(tree.Close)
	return ctx, tree
}

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
