// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

package cambium_test

import (
	"path/filepath"
	"testing"

	"github.com/signalbreak-labs/cambium/go/cambium"
	upstream "github.com/signalbreak-labs/cambium/go/internal/yangparse/upstream/yang"
)

func TestModuleResolvePrefixMatchesGoyang(t *testing.T) {
	root := t.TempDir()
	importedPath := filepath.Join(root, "native-prefix-imported.yang")
	userPath := filepath.Join(root, "native-prefix-user.yang")
	const importedSource = `module native-prefix-imported {
  namespace "urn:native-prefix-imported";
  prefix npi;
}`
	const userSource = `module native-prefix-user {
  namespace "urn:native-prefix-user";
  prefix npu;

  import native-prefix-imported {
    prefix imported;
  }

  container top {
    leaf value { type string; }
  }
}`
	writeTestFile(t, importedPath, importedSource)
	writeTestFile(t, userPath, userSource)

	builder, err := cambium.NewContextBuilder(cambium.ContextFlags{})
	if err != nil {
		t.Fatalf("NewContextBuilder: %v", err)
	}
	if err := builder.SearchPath(root); err != nil {
		t.Fatalf("SearchPath: %v", err)
	}
	if err := builder.LoadModuleFromPath(userPath); err != nil {
		t.Fatalf("LoadModuleFromPath: %v", err)
	}
	ctx, err := builder.Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	nativeMod, err := ctx.Schema("native-prefix-user")
	if err != nil {
		t.Fatalf("Schema: %v", err)
	}

	goyangRoot := parseGoyangEntrySet(t, "native-prefix-user", map[string]string{
		importedPath: importedSource,
		userPath:     userSource,
	})
	goyangImported := upstream.FindModuleByPrefix(goyangRoot.Node, "imported")
	if goyangImported == nil {
		t.Fatal("goyang FindModuleByPrefix(imported) = nil")
	}

	nativeImported, ok := nativeMod.ResolvePrefix("imported")
	if !ok {
		t.Fatal("native ResolvePrefix(imported) = false")
	}
	if got, want := nativeImported.Name(), goyangImported.Name; got != want {
		t.Fatalf("ResolvePrefix(imported).Name = %q, want goyang %q", got, want)
	}

	nativeSelf, ok := nativeMod.ResolvePrefix("npu")
	if !ok {
		t.Fatal("native ResolvePrefix(npu) = false")
	}
	goyangSelf := upstream.FindModuleByPrefix(goyangRoot.Node, "npu")
	if goyangSelf == nil {
		t.Fatal("goyang FindModuleByPrefix(npu) = nil")
	}
	if got, want := nativeSelf.Name(), goyangSelf.Name; got != want {
		t.Fatalf("ResolvePrefix(npu).Name = %q, want goyang %q", got, want)
	}

	if _, ok := nativeMod.ResolvePrefix("missing"); ok {
		t.Fatal("ResolvePrefix(missing) = true, want false")
	}
	if got := upstream.FindModuleByPrefix(goyangRoot.Node, "missing"); got != nil {
		t.Fatalf("goyang FindModuleByPrefix(missing) = %#v, want nil", got)
	}
}
