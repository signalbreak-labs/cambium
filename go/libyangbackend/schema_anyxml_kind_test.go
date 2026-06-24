// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

//go:build cgo

package libyangbackend_test

import (
	"path/filepath"
	"testing"

	cambium "github.com/signalbreak-labs/cambium/go/libyangbackend"
)

// TestSchemaAnyDataAnyXMLDistinctKinds verifies the libyang backend tier
// classifies anydata and anyxml as distinct schema kinds (anydata: RFC 7950
// section 7.10, YANG 1.1; anyxml: section 7.11, YANG 1.0) rather
// than collapsing both into SchemaNodeKindAnyData. This exercises the
// backend-only classification path
// lysAnyXml -> schemaKindName "anyxml" -> kindFromRaw -> SchemaNodeKindAnyXML,
// which the data-tier serialization corpus does not assert on directly.
func TestSchemaAnyDataAnyXMLDistinctKinds(t *testing.T) {
	conf := findConformance(t)
	moduleDir := filepath.Join(conf, "fixtures", "schema-anydata-anyxml-keyword", "module")

	ctx, err := cambium.NewContext()
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}
	defer ctx.Close()
	if err := ctx.SetSearchPath(moduleDir); err != nil {
		t.Fatalf("SetSearchPath: %v", err)
	}
	if err := ctx.LoadModule("schema-anydata-anyxml-keyword"); err != nil {
		t.Fatalf("LoadModule: %v", err)
	}
	mod, err := ctx.Schema("schema-anydata-anyxml-keyword")
	if err != nil {
		t.Fatalf("Schema: %v", err)
	}

	payload, err := mod.FindPath("/schema-anydata-anyxml-keyword:root/payload")
	if err != nil {
		t.Fatalf("FindPath payload: %v", err)
	}
	if payload.Kind() != cambium.SchemaNodeKindAnyData {
		t.Fatalf("payload (anydata) kind = %v, want AnyData", payload.Kind())
	}
	if got := payload.Kind().String(); got != "anydata" {
		t.Fatalf("payload kind string = %q, want anydata", got)
	}
	if !payload.IsAnyData() || payload.IsAnyXML() {
		t.Fatalf("payload predicates: IsAnyData=%v IsAnyXML=%v, want true,false", payload.IsAnyData(), payload.IsAnyXML())
	}

	raw, err := mod.FindPath("/schema-anydata-anyxml-keyword:root/raw-data")
	if err != nil {
		t.Fatalf("FindPath raw-data: %v", err)
	}
	if raw.Kind() != cambium.SchemaNodeKindAnyXML {
		t.Fatalf("raw-data (anyxml) kind = %v, want AnyXML", raw.Kind())
	}
	if got := raw.Kind().String(); got != "anyxml" {
		t.Fatalf("raw-data kind string = %q, want anyxml", got)
	}
	if !raw.IsAnyXML() || raw.IsAnyData() {
		t.Fatalf("raw-data predicates: IsAnyXML=%v IsAnyData=%v, want true,false", raw.IsAnyXML(), raw.IsAnyData())
	}
}
