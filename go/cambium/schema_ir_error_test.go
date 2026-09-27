// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

package cambium_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/signalbreak-labs/cambium/go/cambium"
)

func TestSchemaIRSurfacesRebuildFailure(t *testing.T) {
	dir := t.TempDir()
	writeModuleFile(t, filepath.Join(dir, "schema-ir-rebuild.yang"), []byte(`module schema-ir-rebuild {
    namespace "urn:schema-ir-rebuild";
    prefix sir;

    feature declared;

    container top {
        leaf value { type string; }
    }
}`))

	ctx, err := cambium.NewContext()
	if err != nil {
		t.Fatal(err)
	}
	defer ctx.Close()
	if err := ctx.SetSearchPath(dir); err != nil {
		t.Fatalf("SetSearchPath: %v", err)
	}
	if err := ctx.LoadModule("schema-ir-rebuild"); err != nil {
		t.Fatalf("LoadModule: %v", err)
	}
	if _, err := ctx.Schema("schema-ir-rebuild"); err != nil {
		t.Fatalf("Schema: %v", err)
	}
	if err := ctx.SetFeatures("schema-ir-rebuild", []string{"no-such-feature"}); err != nil {
		t.Fatalf("SetFeatures: %v", err)
	}

	ir := ctx.SchemaIR()
	if len(ir.Errors) != 1 {
		t.Fatalf("SchemaIR errors = %#v, want one rebuild diagnostic", ir.Errors)
	}
	got := ir.Errors[0]
	if got.Code != cambium.RuleCodeContext {
		t.Fatalf("SchemaIR error code = %s, want %s", got.Code, cambium.RuleCodeContext)
	}
	if !strings.Contains(got.Message, "schema rebuild") ||
		!strings.Contains(got.Message, `unknown feature "no-such-feature"`) {
		t.Fatalf("SchemaIR error message = %q, want schema rebuild unknown feature", got.Message)
	}

	// The v2 table and the limited v1 projection carry the same diagnostic.
	table := ctx.SchemaIRTable()
	if table.Version != cambium.SchemaIRTableVersion || len(table.Errors) != 1 || table.Errors[0].Message != got.Message {
		t.Fatalf("SchemaIRTable = version %q errors %#v, want the rebuild diagnostic", table.Version, table.Errors)
	}
	limited, err := ctx.SchemaIRWithLimit(1 << 20)
	if err != nil {
		t.Fatalf("SchemaIRWithLimit: %v", err)
	}
	if len(limited.Errors) != 1 || limited.Errors[0].Message != got.Message {
		t.Fatalf("SchemaIRWithLimit errors = %#v, want the rebuild diagnostic", limited.Errors)
	}
}
