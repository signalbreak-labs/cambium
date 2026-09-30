// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

package compat_test

import (
	"fmt"
	"testing"

	"github.com/signalbreak-labs/cambium/go/compat"
)

func TestNativeDecimalRangesPreserveIntrinsicAndRestrictedBounds(t *testing.T) {
	for _, tc := range []struct {
		fd             int
		min, max, zero string
	}{
		{2, "-92233720368547758.08", "92233720368547758.07", "0.00"},
		{6, "-9223372036854.775808", "9223372036854.775807", "0.000000"},
		{18, "-9.223372036854775808", "9.223372036854775807", "0.000000000000000000"},
	} {
		t.Run(fmt.Sprint(tc.fd), func(t *testing.T) {
			ctx := nativeProjectionContext(t, false, map[string]string{
				"decimal": fmt.Sprintf(`module decimal {
  yang-version 1.1; namespace "urn:decimal"; prefix d;
  typedef inherited { type decimal64 { fraction-digits %d; } }
  typedef restricted { type decimal64 { fraction-digits 6; range "0.000001 .. 7.123456"; } }
  leaf plain { type decimal64 { fraction-digits %d; } }
  leaf alias { type inherited; }
  leaf narrowed { type restricted; }
  leaf explicit { type decimal64 { fraction-digits 2; range "-12.34 .. 56.78"; } }
  leaf lower { type decimal64 { fraction-digits %d; range "min .. 0"; } }
  leaf upper { type decimal64 { fraction-digits %d; range "0 .. max"; } }
  leaf ref { type leafref { path "../plain"; } }
  leaf union { type union { type inherited; type restricted; } }
}`, tc.fd, tc.fd, tc.fd, tc.fd),
			}, "decimal")
			roots, err := compat.FromContext(ctx)
			if err != nil {
				t.Fatal(err)
			}
			root := projectionRoot(t, roots, "decimal")
			assertDecimalRange(t, root.Lookup("plain").Type, tc.fd, tc.min, tc.max)
			assertDecimalRange(t, root.Lookup("alias").Type, tc.fd, tc.min, tc.max)
			assertDecimalRange(t, root.Lookup("narrowed").Type, 6, "0.000001", "7.123456")
			assertDecimalRange(t, root.Lookup("explicit").Type, 2, "-12.34", "56.78")
			assertDecimalRange(t, root.Lookup("lower").Type, tc.fd, tc.min, tc.zero)
			assertDecimalRange(t, root.Lookup("upper").Type, tc.fd, tc.zero, tc.max)
			target, err := root.Lookup("ref").ResolveLeafref()
			if err != nil {
				t.Fatal(err)
			}
			assertDecimalRange(t, target.Type, tc.fd, tc.min, tc.max)
			members := root.Lookup("union").Type.Type
			if len(members) != 2 {
				t.Fatalf("union members = %d, want 2", len(members))
			}
			assertDecimalRange(t, members[0], tc.fd, tc.min, tc.max)
			assertDecimalRange(t, members[1], 6, "0.000001", "7.123456")
		})
	}
}

func assertDecimalRange(t *testing.T, typ *compat.YangType, fd int, lower, upper string) {
	t.Helper()
	if typ == nil || typ.FractionDigits != fd || len(typ.Range) != 1 {
		t.Errorf("decimal type = %+v, want fraction-digits %d and one range", typ, fd)
		return
	}
	if got := typ.Range[0]; got.Min.String() != lower || got.Max.String() != upper {
		t.Errorf("decimal bounds = %s .. %s, want %s .. %s", got.Min, got.Max, lower, upper)
	}
}
