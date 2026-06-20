package codegen

import (
	"strings"
	"testing"
)

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
