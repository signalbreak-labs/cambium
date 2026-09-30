// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

package compat_test

import (
	"testing"

	"github.com/signalbreak-labs/cambium/go/compat"
)

func TestNativeProjectionCoalescesExactlyAdjacentRanges(t *testing.T) {
	ctx := nativeProjectionContext(t, false, map[string]string{
		"ranges": `module ranges {
  yang-version 1.1; namespace "urn:ranges"; prefix r;
  typedef label { type string { length "1..2 | 3..4"; } }
  leaf text { type label; }
  leaf singletons { type string { length "1 | 2"; } }
  leaf binary { type binary { length "1..2 | 3..4"; } }
  leaf gap { type string { length "1..2 | 4..5"; } }
  leaf signed { type int64 { range "-2..-1 | 0 | 1..2"; } }
  leaf unsigned { type uint64 { range "18446744073709551614 | 18446744073709551615"; } }
  leaf decimal { type decimal64 { fraction-digits 2; range "-0.02..-0.01 | 0 | 0.01..0.02"; } }
  leaf precise { type decimal64 { fraction-digits 18; range "9.223372036854775806 | 9.223372036854775807"; } }
  leaf decimal-gap { type decimal64 { fraction-digits 2; range "-0.02..-0.01 | 0.01..0.02"; } }
  leaf union { type union { type label; type int16 { range "1 | 2"; } } }
}`,
	}, "ranges")
	roots, err := compat.FromContext(ctx)
	if err != nil {
		t.Fatal(err)
	}
	root := projectionRoot(t, roots, "ranges")
	for name, want := range map[string]string{
		"text": "1..4", "singletons": "1..2", "binary": "1..4", "gap": "1..2|4..5",
		"signed": "-2..2", "unsigned": "18446744073709551614..18446744073709551615",
		"decimal": "-0.02..0.02", "precise": "9.223372036854775806..9.223372036854775807",
		"decimal-gap": "-0.02..-0.01|0.01..0.02",
	} {
		typ := root.Lookup(name).Type
		got := typ.Range
		if typ.Kind == compat.Ystring || typ.Kind == compat.Ybinary {
			got = typ.Length
		}
		if got.String() != want {
			t.Errorf("%s bounds = %s, want %s", name, got, want)
		}
	}
	members := root.Lookup("union").Type.Type
	if len(members) != 2 || members[0].Length.String() != "1..4" || members[1].Range.String() != "1..2" {
		t.Fatalf("union bounds were not preserved: %+v", members)
	}
}
