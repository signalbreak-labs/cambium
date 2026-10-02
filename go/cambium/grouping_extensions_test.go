// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

package cambium_test

import (
	"path/filepath"
	"testing"

	"github.com/signalbreak-labs/cambium/go/cambium"
)

func TestImportedGroupingExtensionsUseDeclarationScope(t *testing.T) {
	for _, tc := range []struct {
		name    string
		imports string
	}{
		{name: "no_extension_import"},
		{name: "different_alias", imports: `import annotations { prefix other; }`},
		{name: "shadowed_alias", imports: `import unrelated { prefix ann; }`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			writeModuleFile(t, filepath.Join(dir, "annotations.yang"), []byte(`module annotations {
  yang-version 1.1; namespace "urn:annotations"; prefix ann;
  extension description-local { argument text; }
}`))
			writeModuleFile(t, filepath.Join(dir, "unrelated.yang"), []byte(`module unrelated {
  yang-version 1.1; namespace "urn:unrelated"; prefix u;
  extension description-local { argument text; }
}`))
			writeModuleFile(t, filepath.Join(dir, "library.yang"), []byte(`module library {
  yang-version 1.1; namespace "urn:library"; prefix lib;
  import annotations { prefix ann; }
  grouping parameters {
    leaf command {
      type string;
      ann:description-local "명령어 설명";
    }
  }
}`))
			writeModuleFile(t, filepath.Join(dir, "wrapper.yang"), []byte(`module wrapper {
  yang-version 1.1; namespace "urn:wrapper"; prefix w;
  import library { prefix lib; }
  grouping parameters { uses lib:parameters; }
}`))
			consumer := `module consumer {
  yang-version 1.1; namespace "urn:consumer"; prefix c;
  import wrapper { prefix w; }
  ` + tc.imports + `
  container settings {
    uses w:parameters;
    action execute { input { uses w:parameters; } }
  }
  rpc execute {
    input { uses w:parameters; }
    output { uses w:parameters; }
  }
  notification event { uses w:parameters; }
}`
			builder, err := cambium.NewContextBuilder(cambium.ContextFlags{DisableSearchdirCwd: true})
			if err != nil {
				t.Fatal(err)
			}
			if err := builder.SearchPath(dir); err != nil {
				t.Fatal(err)
			}
			if err := builder.LoadModuleStr(consumer); err != nil {
				t.Fatal(err)
			}
			ctx, err := builder.Build()
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(ctx.Close)
			module, err := ctx.Schema("consumer")
			if err != nil {
				t.Fatal(err)
			}
			for _, path := range []string{
				"/c:settings/command",
				"/c:settings/execute/input/command",
				"/c:execute/input/command",
				"/c:execute/output/command",
				"/c:event/command",
			} {
				node, err := module.FindPath(path)
				if err != nil {
					t.Fatal(err)
				}
				extensions := node.Extensions()
				if len(extensions) != 1 {
					t.Fatalf("%s: got %d extensions, want 1", path, len(extensions))
				}
				ext := extensions[0]
				if ext.ModuleName() != "annotations" || ext.Name() != "description-local" {
					t.Errorf("%s: extension = %s:%s, want annotations:description-local", path, ext.ModuleName(), ext.Name())
				}
				if argument, ok := ext.Argument(); !ok || argument != "명령어 설명" {
					t.Errorf("%s: argument = %q, %v; want Korean description", path, argument, ok)
				}
				if got := node.Module().Name(); got != "consumer" {
					t.Errorf("%s: node module = %s, want consumer", path, got)
				}
			}
		})
	}
}
