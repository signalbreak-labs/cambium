// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

package datatree_test

import (
	"testing"

	"github.com/signalbreak-labs/cambium/go/datatree"
)

func TestEmptyDerivedEnumBitsValidation(t *testing.T) {
	mod := loadModSrc(t, `module filtered {
  yang-version 1.1; namespace "urn:filtered"; prefix f; feature available;
  typedef enum-base { type enumeration { enum allowed; enum forbidden; } }
  typedef bit-base { type bits { bit allowed; bit forbidden; } }
  leaf e { type enum-base { enum allowed { if-feature available; } } }
  leaf b { type bit-base { bit allowed { if-feature available; } } }
  leaf u { type union { type enum-base { enum allowed { if-feature available; } } type uint8; } }
}`, "filtered")
	for _, tc := range []struct {
		input string
		valid bool
	}{
		{`{"filtered:e":"allowed"}`, false}, {`{"filtered:e":"forbidden"}`, false},
		{`{"filtered:b":"allowed"}`, false}, {`{"filtered:b":"forbidden"}`, false},
		{`{"filtered:u":"allowed"}`, false}, {`{"filtered:u":"forbidden"}`, false},
		{`{}`, true}, {`{"filtered:b":""}`, true}, {`{"filtered:u":7}`, true},
	} {
		tree, err := datatree.Parse(mod, datatree.FormatJSONIETF, []byte(tc.input))
		if err == nil {
			err = tree.Validate()
		}
		if (err == nil) != tc.valid {
			t.Errorf("Validate(%s) = %v, want valid=%v", tc.input, err, tc.valid)
		}
	}
}
