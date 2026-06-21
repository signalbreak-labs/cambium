// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

package datatree_test

import (
	"strings"
	"testing"
)

// Module name == prefix so the JSON_IETF module-name qualifier in the
// instance-identifier value resolves through the import-prefix resolver.
const iiSchema = `module p {
    namespace "urn:p"; prefix p;
    list item { key id; leaf id { type string; } }
    leaf ptr { type instance-identifier; }
}`

func TestInstanceIdentifierExists(t *testing.T) {
	mod := loadModSrc(t, iiSchema, "p")
	if err := validateOne(t, mod, `{"p:item":[{"id":"a"}],"p:ptr":"/p:item[p:id='a']"}`); err != nil {
		t.Fatalf("instance-identifier points at an existing entry, should be valid: %v", err)
	}
}

func TestInstanceIdentifierMissing(t *testing.T) {
	mod := loadModSrc(t, iiSchema, "p")
	err := validateOne(t, mod, `{"p:item":[{"id":"a"}],"p:ptr":"/p:item[p:id='zzz']"}`)
	if err == nil || !strings.Contains(err.Error(), "non-existent instance") {
		t.Fatalf("instance-identifier points at a missing entry, expected violation, got %v", err)
	}
}
