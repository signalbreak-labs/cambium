// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

package cambium_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/signalbreak-labs/cambium/go/cambium"
	upstream "github.com/signalbreak-labs/cambium/go/internal/yangparse/upstream/yang"
)

func TestNativeSchemaLocationsMatchGoyangSource(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "native-source-location.yang")
	const source = `module native-source-location {
  namespace "urn:native-source-location";
  prefix nsl;

  container top {
    leaf value { type string; }
  }
}`
	writeTestFile(t, path, source)

	builder, err := cambium.NewContextBuilder(cambium.ContextFlags{})
	if err != nil {
		t.Fatalf("NewContextBuilder: %v", err)
	}
	if err := builder.LoadModuleFromPath(path); err != nil {
		t.Fatalf("LoadModuleFromPath: %v", err)
	}
	ctx, err := builder.Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	mod, err := ctx.Schema("native-source-location")
	if err != nil {
		t.Fatalf("Schema: %v", err)
	}
	top, ok := mod.TopLevel().Lookup("top")
	if !ok {
		t.Fatal("top container not found")
	}
	value, ok := top.Children().Lookup("value")
	if !ok {
		t.Fatal("value leaf not found")
	}

	goyangRoot := parseGoyangEntry(t, path, source, "native-source-location")
	goyangTop := goyangRoot.Dir["top"]
	if goyangTop == nil {
		t.Fatal("goyang top container not found")
	}
	goyangValue := goyangTop.Dir["value"]
	if goyangValue == nil {
		t.Fatal("goyang value leaf not found")
	}

	if got, want := mod.Location(), upstream.Source(goyangRoot.Node); got != want {
		t.Fatalf("Module.Location = %q, want goyang %q", got, want)
	}
	if got, want := top.Location(), upstream.Source(goyangTop.Node); got != want {
		t.Fatalf("container Location = %q, want goyang %q", got, want)
	}
	if got, want := value.Location(), upstream.Source(goyangValue.Node); got != want {
		t.Fatalf("leaf Location = %q, want goyang %q", got, want)
	}
}

func TestNativeSchemaLocationZeroValues(t *testing.T) {
	if got := (cambium.Module{}).Location(); got != "unknown" {
		t.Fatalf("zero Module Location = %q, want unknown", got)
	}
	if got := (cambium.SchemaNodeRef{}).Location(); got != "unknown" {
		t.Fatalf("zero SchemaNodeRef Location = %q, want unknown", got)
	}
}

func parseGoyangEntry(t *testing.T, path, source, name string) *upstream.Entry {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read source: %v", err)
	}
	if string(data) != source {
		t.Fatal("test fixture readback changed")
	}
	modules := upstream.NewModules()
	if err := modules.Parse(source, path); err != nil {
		t.Fatalf("goyang Parse: %v", err)
	}
	entry, errs := modules.GetModule(name)
	if len(errs) != 0 {
		t.Fatalf("goyang GetModule errors: %v", errs)
	}
	return entry
}
