// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

//go:build cgo

package libyangbackend_test

import (
	"testing"

	core "github.com/signalbreak-labs/cambium/go/cambium"
	backend "github.com/signalbreak-labs/cambium/go/libyangbackend"
)

// TestSchemaNodeKindUnifiedWithCore guards the single-source-of-truth invariant
// for the schema-kind taxonomy: the backend's SchemaNodeKind must be a type
// alias of the pure-Go core's, so the two tiers can never drift (see
// docs/adr/0001-unify-schemanodekind.md). The cross-package assignment below
// only compiles when the two are the SAME type, not merely two int enums with
// equal values.
func TestSchemaNodeKindUnifiedWithCore(t *testing.T) {
	// The field below is typed core.SchemaNodeKind but populated with
	// backend.SchemaNodeKind* constants: that only compiles when the backend
	// type is an alias of the core type, so this slice IS the compile-time
	// type-identity guard. Reintroducing a distinct backend type breaks the build.
	cases := []struct {
		got  core.SchemaNodeKind
		name string
	}{
		{backend.SchemaNodeKindModule, "module"},
		{backend.SchemaNodeKindContainer, "container"},
		{backend.SchemaNodeKindLeaf, "leaf"},
		{backend.SchemaNodeKindLeafList, "leaf-list"},
		{backend.SchemaNodeKindList, "list"},
		{backend.SchemaNodeKindChoice, "choice"},
		{backend.SchemaNodeKindCase, "case"},
		{backend.SchemaNodeKindAnyData, "anydata"},
		{backend.SchemaNodeKindRPC, "rpc"},
		{backend.SchemaNodeKindAction, "action"},
		{backend.SchemaNodeKindInput, "input"},
		{backend.SchemaNodeKindOutput, "output"},
		{backend.SchemaNodeKindNotification, "notification"},
		{backend.SchemaNodeKindAnyXML, "anyxml"},
		{backend.SchemaNodeKindUnknown, "unknown"},
	}
	for _, c := range cases {
		// Backend constant must equal its core counterpart and stringify via the
		// core's single String() implementation.
		if c.got.String() != c.name {
			t.Fatalf("%s: String() = %q, want %q", c.name, c.got.String(), c.name)
		}
	}
}
