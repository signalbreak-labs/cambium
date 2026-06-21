// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

//go:build cgo

package libyangbackend_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	cambium "github.com/signalbreak-labs/cambium/go/libyangbackend"
)

func writeParseModeModule(t *testing.T) string {
	t.Helper()
	dir := filepath.Join("..", "..", "target", "tests", "parse-mode", "modules")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "parse-mode-demo.yang")
	yang := `module parse-mode-demo {
    namespace "urn:parse-mode";
    prefix pmd;
    yang-version 1.1;
    revision 2026-06-14;

    leaf config-value {
        type string;
    }
    leaf state-value {
        config false;
        type string;
    }
}
`
	if err := os.WriteFile(path, []byte(yang), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func parseModeContext(t *testing.T) *cambium.Context {
	t.Helper()
	dir := writeParseModeModule(t)
	ctx, err := cambium.NewContext()
	if err != nil {
		t.Fatal(err)
	}
	if err := ctx.SetSearchPath(dir); err != nil {
		t.Fatal(err)
	}
	if err := ctx.LoadModule("parse-mode-demo"); err != nil {
		t.Fatal(err)
	}
	return ctx
}

func TestParseModeStrictRejectsUnknown(t *testing.T) {
	ctx := parseModeContext(t)
	defer ctx.Close()

	xml := []byte(`<config-value xmlns="urn:parse-mode">cfg</config-value>
<unknown xmlns="urn:parse-mode">x</unknown>`)
	_, err := ctx.Parse(cambium.FormatXML, cambium.ParseMode{Strict: true}, xml)
	if err == nil {
		t.Fatal("expected strict parse to reject unknown node")
	}
	var ce *cambium.Error
	if !errors.As(err, &ce) || ce.RuleCode() != cambium.RuleCodeParse {
		t.Fatalf("want E0002 parse error, got %v", err)
	}
}

func TestParseModeStrictAllowsKnownState(t *testing.T) {
	ctx := parseModeContext(t)
	defer ctx.Close()

	xml := []byte(`<config-value xmlns="urn:parse-mode">cfg</config-value>
<state-value xmlns="urn:parse-mode">st</state-value>`)
	tree, err := ctx.Parse(cambium.FormatXML, cambium.ParseMode{Strict: true}, xml)
	if err != nil {
		t.Fatalf("strict parse failed: %v", err)
	}
	defer tree.Close()
	// Full data-tree navigation is Slice 3; use serialization as an existence oracle.
	out, err := tree.Serialize(cambium.FormatXML, cambium.DefaultSerializeFlags())
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	if !contains(s, "config-value") || !contains(s, "state-value") {
		t.Fatalf("known state data was dropped: %s", s)
	}
}

func TestParseModeNoStateForbidsState(t *testing.T) {
	ctx := parseModeContext(t)
	defer ctx.Close()

	xml := []byte(`<config-value xmlns="urn:parse-mode">cfg</config-value>
<state-value xmlns="urn:parse-mode">st</state-value>`)
	_, err := ctx.Parse(cambium.FormatXML, cambium.ParseMode{NoState: true}, xml)
	if err == nil {
		t.Fatal("expected no_state parse to reject state data")
	}
	var ce *cambium.Error
	if !errors.As(err, &ce) || ce.RuleCode() != cambium.RuleCodeParse {
		t.Fatalf("want E0002 parse error, got %v", err)
	}
}

func TestParseModeComposeStrictNoState(t *testing.T) {
	ctx := parseModeContext(t)
	defer ctx.Close()

	xml := []byte(`<config-value xmlns="urn:parse-mode">cfg</config-value>
<state-value xmlns="urn:parse-mode">st</state-value>`)
	_, err := ctx.Parse(cambium.FormatXML, cambium.ParseMode{Strict: true, NoState: true}, xml)
	if err == nil {
		t.Fatal("expected strict+no_state parse to reject state data")
	}
	var ce *cambium.Error
	if !errors.As(err, &ce) || ce.RuleCode() != cambium.RuleCodeParse {
		t.Fatalf("want E0002 parse error, got %v", err)
	}
}

func TestParseModeStrictOpaqueRejected(t *testing.T) {
	ctx := parseModeContext(t)
	defer ctx.Close()

	xml := []byte(`<config-value xmlns="urn:parse-mode">cfg</config-value>`)
	_, err := ctx.Parse(cambium.FormatXML, cambium.ParseMode{Strict: true, Opaque: true}, xml)
	if err == nil {
		t.Fatal("expected strict+opaque combination to be rejected before parsing")
	}
	var ce *cambium.Error
	if !errors.As(err, &ce) || ce.RuleCode() != cambium.RuleCodeParse {
		t.Fatalf("want E0002 parse error, got %v", err)
	}
	if !contains(err.Error(), "mutually exclusive") {
		t.Fatalf("error should mention mutually exclusive, got %v", err)
	}
}

func contains(s, substr string) bool {
	return strings.Contains(s, substr)
}
