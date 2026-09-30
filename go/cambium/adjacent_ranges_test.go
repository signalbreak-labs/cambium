// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

package cambium_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/signalbreak-labs/cambium/go/cambium"
)

func TestDerivedRestrictionsAllowExactlyAdjacentSpans(t *testing.T) {
	for _, tc := range []struct {
		name, typ, keyword, spans, valid, invalid string
	}{
		{"length", "string", "length", "1..2 | 3..4", "abc", "abcde"},
		{"integer", "int64", "range", "-2..-1 | 0 | 1..2", "0", "3"},
		{"unsigned-limit", "uint64", "range", "18446744073709551614 | 18446744073709551615", "18446744073709551615", "18446744073709551613"},
		{"decimal", "decimal64 { fraction-digits 2; %s }", "range", "-0.02..-0.01 | 0 | 0.01..0.02", "0.01", "0.03"},
		{"decimal-limit", "decimal64 { fraction-digits 18; %s }", "range", "9.223372036854775806 | 9.223372036854775807", "9.223372036854775807", "9.223372036854775805"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			restriction := fmt.Sprintf(`%s %q;`, tc.keyword, tc.spans)
			typ := tc.typ + " { " + restriction + " }"
			if strings.Contains(tc.typ, "%s") {
				typ = fmt.Sprintf(tc.typ, restriction)
			}
			for _, value := range []string{tc.valid, tc.invalid} {
				source := fmt.Sprintf(`module adjacent {
  yang-version 1.1; namespace "urn:adjacent"; prefix a;
  typedef base { type %s }
  leaf value { type base { %s "min..max"; } default %q; }
}`, typ, tc.keyword, value)
				_, err := buildPolicyContext(t, cambium.DeviationPolicy{}, source)
				if value == tc.valid && err != nil {
					t.Errorf("adjacent parent spans rejected valid derived restriction: %v", err)
				}
				if value == tc.invalid && (err == nil || !strings.Contains(err.Error(), "default")) {
					t.Errorf("out-of-bounds default error = %v, want default validation", err)
				}
			}
		})
	}
}

func TestDerivedRestrictionsCannotBridgeGaps(t *testing.T) {
	for _, typ := range []string{
		`string { length "1..2 | 4..5"; }`,
		`int64 { range "-2..-1 | 1..2"; }`,
		`decimal64 { fraction-digits 2; range "-0.02..-0.01 | 0.01..0.02"; }`,
	} {
		keyword := "range"
		if strings.HasPrefix(typ, "string") {
			keyword = "length"
		}
		source := fmt.Sprintf(`module gap {
  yang-version 1.1; namespace "urn:gap"; prefix g;
  typedef base { type %s }
  leaf value { type base { %s "min..max"; } }
}`, typ, keyword)
		_, err := buildPolicyContext(t, cambium.DeviationPolicy{}, source)
		if err == nil || !strings.Contains(err.Error(), "is not within the base restriction") {
			t.Errorf("gap in %s: error = %v, want subset violation", typ, err)
		}
	}
}
