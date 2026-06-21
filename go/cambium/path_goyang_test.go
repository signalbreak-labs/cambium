// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

package cambium_test

import (
	"errors"
	"testing"

	"github.com/signalbreak-labs/cambium/go/cambium"
	upstream "github.com/signalbreak-labs/cambium/go/internal/yangparse/upstream/yang"
)

func TestNativeSchemaNodeFindPathMatchesGoyang(t *testing.T) {
	const source = `module native-path-demo {
  namespace "urn:native-path-demo";
  prefix npd;

  container top {
    container nested {
      leaf value { type string; }
      leaf sibling { type string; }
    }
    leaf top-leaf { type string; }
    choice mode {
      case auto {
        leaf auto-enabled { type boolean; }
      }
    }
  }
}`

	builder, err := cambium.NewContextBuilder(cambium.ContextFlags{})
	if err != nil {
		t.Fatalf("NewContextBuilder: %v", err)
	}
	if err := builder.LoadModuleStr(source); err != nil {
		t.Fatalf("LoadModuleStr: %v", err)
	}
	ctx, err := builder.Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	mod, err := ctx.Schema("native-path-demo")
	if err != nil {
		t.Fatalf("Schema: %v", err)
	}
	nativeTop, ok := mod.TopLevel().Lookup("top")
	if !ok {
		t.Fatal("native top not found")
	}
	nativeNested, ok := nativeTop.Children().Lookup("nested")
	if !ok {
		t.Fatal("native nested not found")
	}
	nativeValue, ok := nativeNested.Children().Lookup("value")
	if !ok {
		t.Fatal("native value not found")
	}

	goyangRoot := parseGoyangEntrySet(t, "native-path-demo", map[string]string{
		"native-path-demo.yang": source,
	})
	goyangTop := goyangRoot.Dir["top"]
	goyangNested := goyangTop.Dir["nested"]
	goyangValue := goyangNested.Dir["value"]

	if got, want := nativeValue.Path(), upstream.NodePath(goyangValue.Node); got != want {
		t.Fatalf("Path = %q, want goyang NodePath %q", got, want)
	}

	tests := []struct {
		name       string
		start      cambium.SchemaNodeRef
		goyang     upstream.Node
		path       string
		wantNative string
	}{
		{name: "empty", start: nativeValue, goyang: goyangValue.Node, path: "", wantNative: nativeValue.Path()},
		{name: "parent", start: nativeValue, goyang: goyangValue.Node, path: "..", wantNative: nativeNested.Path()},
		{name: "sibling", start: nativeValue, goyang: goyangValue.Node, path: "../sibling", wantNative: "/native-path-demo/top/nested/sibling"},
		{name: "relative child", start: nativeTop, goyang: goyangTop.Node, path: "nested/value", wantNative: nativeValue.Path()},
		{name: "absolute", start: nativeValue, goyang: goyangValue.Node, path: "/npd:top/npd:nested/npd:sibling", wantNative: "/native-path-demo/top/nested/sibling"},
		{name: "choice case", start: nativeTop, goyang: goyangTop.Node, path: "mode/auto/auto-enabled", wantNative: "/native-path-demo/top/mode/auto/auto-enabled"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.start.FindPath(tt.path)
			if err != nil {
				t.Fatalf("FindPath(%q): %v", tt.path, err)
			}
			if got.Path() != tt.wantNative {
				t.Fatalf("FindPath(%q).Path = %q, want %q", tt.path, got.Path(), tt.wantNative)
			}

			goyangFound, err := upstream.FindNode(tt.goyang, tt.path)
			if err != nil {
				t.Fatalf("goyang FindNode(%q): %v", tt.path, err)
			}
			if got.Path() != upstream.NodePath(goyangFound) {
				t.Fatalf("FindPath(%q).Path = %q, want goyang %q", tt.path, got.Path(), upstream.NodePath(goyangFound))
			}
		})
	}
}

func TestNativeSchemaNodeFindPathErrors(t *testing.T) {
	const source = `module native-path-errors {
  namespace "urn:native-path-errors";
  prefix npe;
  container top {
    leaf value { type string; }
  }
}`

	builder, err := cambium.NewContextBuilder(cambium.ContextFlags{})
	if err != nil {
		t.Fatalf("NewContextBuilder: %v", err)
	}
	if err := builder.LoadModuleStr(source); err != nil {
		t.Fatalf("LoadModuleStr: %v", err)
	}
	ctx, err := builder.Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	mod, err := ctx.Schema("native-path-errors")
	if err != nil {
		t.Fatalf("Schema: %v", err)
	}
	top, ok := mod.TopLevel().Lookup("top")
	if !ok {
		t.Fatal("top not found")
	}
	value, ok := top.Children().Lookup("value")
	if !ok {
		t.Fatal("value not found")
	}

	for _, path := range []string{"/", "missing", "missing/", "../../.."} {
		if _, err := value.FindPath(path); err == nil {
			t.Fatalf("FindPath(%q) succeeded, want error", path)
		}
	}
}

func TestSchemaFindPathReturnsStructuredPathError(t *testing.T) {
	const source = `module native-path-structured-errors {
  namespace "urn:native-path-structured-errors";
  prefix npse;
  container top {
    leaf value { type string; }
  }
}`

	builder, err := cambium.NewContextBuilder(cambium.ContextFlags{})
	if err != nil {
		t.Fatalf("NewContextBuilder: %v", err)
	}
	if err := builder.LoadModuleStr(source); err != nil {
		t.Fatalf("LoadModuleStr: %v", err)
	}
	ctx, err := builder.Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	mod, err := ctx.Schema("native-path-structured-errors")
	if err != nil {
		t.Fatalf("Schema: %v", err)
	}
	top, ok := mod.TopLevel().Lookup("top")
	if !ok {
		t.Fatal("top not found")
	}
	value, ok := top.Children().Lookup("value")
	if !ok {
		t.Fatal("value not found")
	}

	_, err = mod.FindPath("/npse:missing")
	assertSchemaPathError(t, err, cambium.SchemaPathErrorNotFound, "/npse:missing", "npse:missing", "/native-path-structured-errors")

	_, err = value.FindPath("missing")
	assertSchemaPathError(t, err, cambium.SchemaPathErrorNotFound, "missing", "missing", value.Path())

	_, err = value.FindPath("/")
	assertSchemaPathError(t, err, cambium.SchemaPathErrorInvalid, "/", "/", value.Path())

	_, err = top.FindPath("../..")
	assertSchemaPathError(t, err, cambium.SchemaPathErrorNoParent, "../..", "..", "/native-path-structured-errors")
}

func assertSchemaPathError(t *testing.T, err error, kind cambium.SchemaPathErrorKind, path, segment, from string) {
	t.Helper()
	if err == nil {
		t.Fatal("FindPath succeeded, want error")
	}
	var ce *cambium.Error
	if !errors.As(err, &ce) || ce.RuleCode() != cambium.RuleCodeContext {
		t.Fatalf("FindPath error = %v, want RuleCodeContext", err)
	}
	var pe *cambium.SchemaPathError
	if !errors.As(err, &pe) {
		t.Fatalf("FindPath error = %v, want SchemaPathError", err)
	}
	if pe.Kind != kind || pe.Path != path || pe.Segment != segment || pe.From != from {
		t.Fatalf("SchemaPathError = {Kind:%q Path:%q Segment:%q From:%q}, want {%q %q %q %q}",
			pe.Kind, pe.Path, pe.Segment, pe.From, kind, path, segment, from)
	}
}
