// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

package compat_test

import (
	"slices"
	"testing"

	"github.com/signalbreak-labs/cambium/go/compat"
)

func TestNativeProjectionEmptyDerivedEnumBits(t *testing.T) {
	for _, retain := range []bool{false, true} {
		ctx := nativeProjectionContext(t, retain, map[string]string{
			"filtered": `module filtered {
  yang-version 1.1; namespace "urn:filtered"; prefix f; feature available;
  typedef enum-base { type enumeration { enum allowed; enum forbidden; } }
  typedef bit-base { type bits { bit allowed; bit forbidden; } }
  leaf e { type enum-base { enum allowed { if-feature available; } } }
  leaf b { type bit-base { bit allowed { if-feature available; } } }
  leaf u { type union { type enum-base { enum allowed { if-feature available; } } type uint8; } }
}`,
		}, "filtered")
		roots, err := compat.FromContext(ctx)
		if err != nil {
			t.Fatal(err)
		}
		root := projectionRoot(t, roots, "filtered")
		var want []string
		if retain {
			want = []string{"allowed"}
		}
		for _, values := range []*compat.EnumType{root.Lookup("e").Type.Enum, root.Lookup("b").Type.Bit, root.Lookup("u").Type.Type[0].Enum} {
			if values == nil || !slices.Equal(values.Names(), want) {
				t.Errorf("RetainAll=%v: mapping = %+v, want names %v", retain, values, want)
			}
		}
	}
}
