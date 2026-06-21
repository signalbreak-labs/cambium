// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

package codegen

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestRegenerateStaticTemplates captures the exact bytes of each fully-static
// helper block into go/codegen/templates/*.tmpl. It is a one-shot generator,
// run only when CAMBIUM_REGEN_TEMPLATES is set, used to migrate the helpers to
// //go:embed without changing a single output byte. Kept in-tree so the embed
// files can be regenerated verbatim if a helper ever changes.
func TestRegenerateStaticTemplates(t *testing.T) {
	if os.Getenv("CAMBIUM_REGEN_TEMPLATES") == "" {
		t.Skip("set CAMBIUM_REGEN_TEMPLATES=1 to regenerate codegen/templates/*.tmpl")
	}
	// Only fully-static helpers (no internal g-dependent branches) are captured.
	// emitJSONParseHelper is intentionally excluded: it has a conditional
	// `if g.emittedDecimal64` block, so it stays as code (see templates.go).
	blocks := map[string]func(*goEmitter, *strings.Builder){
		"binaryparse":        (*goEmitter).emitBinaryParseHelper,
		"patternvalidate":    (*goEmitter).emitPatternValidateHelper,
		"bitsparse":          (*goEmitter).emitBitsParseHelper,
		"decimal64":          (*goEmitter).emitDecimal64Helper,
		"userorderedvec":     (*goEmitter).emitUserOrderedVecHelper,
		"instanceidentifier": (*goEmitter).emitInstanceIdentifierHelper,
		"anydata":            (*goEmitter).emitAnyDataHelper,
		"cambiumstructiface": (*goEmitter).emitCambiumStructInterface,
	}
	if err := os.MkdirAll("templates", 0o755); err != nil {
		t.Fatal(err)
	}
	for name, fn := range blocks {
		var b strings.Builder
		fn(&goEmitter{}, &b)
		path := filepath.Join("templates", name+".tmpl")
		if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
		t.Logf("wrote %s (%d bytes)", path, b.Len())
	}
}

func TestEmitJSONParseScalarUnsupportedKindFailsGeneration(t *testing.T) {
	var out strings.Builder
	g := &goEmitter{}
	ok := g.emitJSONParseScalarValue("value", "raw", "key", fieldInfo{
		goType:   "unsupportedType",
		jsonKind: "UnsupportedKind",
	}, &out, "\t")
	if ok {
		t.Fatal("emitJSONParseScalarValue accepted unsupported scalar kind")
	}
	if g.emitErr == nil {
		t.Fatal("emitJSONParseScalarValue did not record generation error")
	}
	if got, want := g.emitErr.Error(), `unsupported JSON_IETF parser for <unknown>: unknown JSON kind "UnsupportedKind"`; got != want {
		t.Fatalf("emitErr = %q, want %q", got, want)
	}
	if strings.Contains(out.String(), "not supported by generated parser") {
		t.Fatalf("emitted runtime unsupported parser fallback:\n%s", out.String())
	}
}

func TestEmitJSONParseUnionWithoutTypeFailsGeneration(t *testing.T) {
	var out strings.Builder
	g := &goEmitter{}
	ok := g.emitJSONParseScalarValue("value", "raw", "key", fieldInfo{
		goType:  "badUnion",
		isUnion: true,
	}, &out, "\t")
	if ok {
		t.Fatal("emitJSONParseScalarValue accepted union without type info")
	}
	if g.emitErr == nil {
		t.Fatal("emitJSONParseScalarValue did not record generation error")
	}
	if got, want := g.emitErr.Error(), "unsupported JSON_IETF parser for <unknown>: missing union type info"; got != want {
		t.Fatalf("emitErr = %q, want %q", got, want)
	}
	if strings.Contains(out.String(), "not supported by generated parser") {
		t.Fatalf("emitted runtime unsupported parser fallback:\n%s", out.String())
	}
}
