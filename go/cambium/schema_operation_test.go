// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

package cambium_test

import (
	"testing"

	"github.com/signalbreak-labs/cambium/go/cambium"
)

func TestAugmentsResolveImplicitOperationIO(t *testing.T) {
	base := `module operations {
  yang-version 1.1; namespace "urn:operations"; prefix op;
  rpc prepare;
  container service { action reset; }
}`
	extension := `module metadata {
  yang-version 1.1; namespace "urn:metadata"; prefix meta;
  import operations { prefix op; }
  augment "/op:prepare/op:input" { leaf label { type string; mandatory true; } }
  augment "/op:prepare/op:output" { leaf result { type string; } }
  augment "/op:service/op:reset/op:input" { leaf label { type string; } }
  augment "/op:service/op:reset/op:output" { leaf result { type string; } }
}`
	for _, mode := range bothValidationModes {
		ctx, err := buildModeContext(t, mode, nil, base, extension)
		if err != nil {
			t.Fatalf("mode %d: Build: %v", mode, err)
		}
		module, err := ctx.Schema("operations")
		if err != nil {
			t.Fatal(err)
		}
		for _, path := range []string{"/op:prepare", "/op:service/op:reset"} {
			operation, err := module.FindPath(path)
			if err != nil {
				t.Fatal(err)
			}
			assertChildNames(t, module, path, "input", "output")
			input, ok := operation.Input()
			if !ok || input.Module().Name() != "operations" || input.Statement().IsValid() {
				t.Fatalf("%s: expected implicit input owned by operations", path)
			}
			output, ok := operation.Output()
			if !ok || output.Module().Name() != "operations" || output.Statement().IsValid() {
				t.Fatalf("%s: expected implicit output owned by operations", path)
			}
			assertChildNames(t, module, path+"/input", "label")
			assertChildNames(t, module, path+"/output", "result")
			label, ok := input.Children().LookupQualified("metadata", "label")
			if !ok || label.Kind() != cambium.SchemaNodeKindLeaf {
				t.Fatalf("%s: missing augmented metadata:label", path)
			}
		}
		if warnings := ctx.LoadReport().Warnings; len(warnings) != 0 {
			t.Fatalf("mode %d: unexpected warnings: %v", mode, warnings)
		}
	}
}

func TestOperationIOPreservesExplicitChildren(t *testing.T) {
	ctx, err := buildModeContext(t, cambium.ValidationStrict, nil, `module operations {
  yang-version 1.1; namespace "urn:operations"; prefix op;
  rpc no-parameters;
  rpc input-only { input { leaf value { type string; } } }
  rpc output-only { output { leaf value { type string; } } }
  rpc both-explicit {
    output { leaf result { type string; } }
    input { leaf argument { type string; } }
  }
}`)
	if err != nil {
		t.Fatal(err)
	}
	module, err := ctx.Schema("operations")
	if err != nil {
		t.Fatal(err)
	}
	assertChildNames(t, module, "/op:both-explicit", "output", "input")
	for _, name := range []string{"no-parameters", "input-only", "output-only"} {
		path := "/op:" + name
		assertChildNames(t, module, path, "input", "output")
		for _, kind := range []string{"input", "output"} {
			node, err := module.FindPath(path + "/" + kind)
			if err != nil {
				t.Fatal(err)
			}
			explicit := name == kind+"-only"
			if node.Statement().IsValid() != explicit {
				t.Errorf("%s/%s: explicit statement = %v, want %v", path, kind, node.Statement().IsValid(), explicit)
			}
			if explicit {
				assertChildNames(t, module, path+"/"+kind, "value")
			} else if !node.Children().IsEmpty() {
				t.Errorf("%s/%s: implicit IO has unexpected children", path, kind)
			}
		}
	}
}

func TestImplicitOperationIOCountsAgainstSchemaBudget(t *testing.T) {
	for _, limit := range []uint64{2, 3} {
		builder, err := cambium.NewContextBuilder(cambium.ContextFlags{})
		if err != nil {
			t.Fatal(err)
		}
		if err := builder.SetMaxSchemaNodes(limit); err != nil {
			t.Fatal(err)
		}
		if err := builder.LoadModuleStr(`module operations {
  namespace "urn:operations"; prefix op; rpc prepare;
}`); err != nil {
			t.Fatal(err)
		}
		ctx, err := builder.Build()
		if err == nil {
			t.Cleanup(ctx.Close)
		}
		if limit == 2 {
			if err == nil || cambium.DiagnosticFromError(err).Kind != cambium.DiagnosticResourceLimit {
				t.Fatalf("budget 2: got %v, want resource-limit error for RPC plus implicit IO", err)
			}
		} else if err != nil {
			t.Fatalf("budget 3: %v", err)
		}
	}
}

func TestImplicitActionIOPreservesChoiceAncestry(t *testing.T) {
	ctx, err := buildModeContext(t, cambium.ValidationStrict, nil, `module operations {
  yang-version 1.1; namespace "urn:operations"; prefix op;
  choice mode {
    case selected {
      container service {
        action implicit;
        action explicit { input { leaf value { type string; } } }
      }
    }
  }
  augment "/op:mode/op:selected/op:service/op:implicit/op:input" {
    leaf value { type string; }
  }
}`)
	if err != nil {
		t.Fatal(err)
	}
	module, err := ctx.Schema("operations")
	if err != nil {
		t.Fatal(err)
	}
	for _, action := range []string{"implicit", "explicit"} {
		for _, suffix := range []string{"/input", "/output", "/input/value"} {
			path := "/op:mode/selected/service/" + action + suffix
			node, err := module.FindPath(path)
			if err != nil {
				t.Fatal(err)
			}
			if !node.IsChoiceDescendant() {
				t.Errorf("%s: lost choice ancestry", path)
			}
		}
	}
}
