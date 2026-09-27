// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

package cambium_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/signalbreak-labs/cambium/go/cambium"
)

// numericBounds renders each bound's resolved MinNumber/MaxNumber, or "?" when
// a bound is not resolved.
func numericBounds(rs []cambium.RangeBound) []bound {
	var out []bound
	for _, r := range rs {
		lo, loOK := r.MinNumber()
		hi, hiOK := r.MaxNumber()
		b := bound{"?", "?"}
		if loOK {
			b.min = lo.String()
		}
		if hiOK {
			b.max = hi.String()
		}
		out = append(out, b)
	}
	return out
}

type lengthBound struct{ min, max uint64 }

func lengthBounds(t *testing.T, rs []cambium.RangeBound) []lengthBound {
	t.Helper()
	var out []lengthBound
	for _, r := range rs {
		lo, loOK := r.MinLength()
		hi, hiOK := r.MaxLength()
		if !loOK || !hiOK {
			t.Fatalf("length bound %s..%s is not resolved", r.Min(), r.Max())
		}
		out = append(out, lengthBound{lo, hi})
	}
	return out
}

func TestRangeBoundNumericValues(t *testing.T) {
	mod := s3Module(t, `module num-bounds {
  yang-version 1.1; namespace "urn:num-bounds"; prefix nb;
  leaf i64 { type int64 { range "min..-5 | 5..max"; } }
  leaf u8 { type uint8 { range "min..max"; } }
  leaf u64 { type uint64 { range "0 | 10..max"; } }
  leaf dec { type decimal64 { fraction-digits 3; range "min..-1.5 | 0..max"; } }
  leaf dec18 { type decimal64 { fraction-digits 18; range "-9.223372036854775808..9.223372036854775807"; } }
  leaf str { type string { length "0..3 | 7 | 10..max"; } }
  leaf bin { type binary { length "min..16"; } }
}`)
	cases := []struct {
		path    string
		lexical []bound
		numeric []bound
	}{
		{"/nb:i64", []bound{{"-9223372036854775808", "-5"}, {"5", "9223372036854775807"}}, []bound{{"-9223372036854775808", "-5"}, {"5", "9223372036854775807"}}},
		{"/nb:u8", []bound{{"0", "255"}}, []bound{{"0", "255"}}},
		{"/nb:u64", []bound{{"0", "0"}, {"10", "18446744073709551615"}}, []bound{{"0", "0"}, {"10", "18446744073709551615"}}},
		// decimal64 min/max are the fraction-digits-scaled int64 limits.
		{"/nb:dec", []bound{{"min", "-1.5"}, {"0.000", "max"}}, []bound{{"-9223372036854775.808", "-1.500"}, {"0.000", "9223372036854775.807"}}},
		{"/nb:dec18", []bound{{"-9.223372036854775808", "9.223372036854775807"}}, []bound{{"-9.223372036854775808", "9.223372036854775807"}}},
	}
	for _, tc := range cases {
		var got []cambium.RangeBound
		switch r := leafType(t, mod, tc.path).Resolved().(type) {
		case cambium.ResolvedInt:
			got = r.Range
		case cambium.ResolvedDecimal64:
			got = r.Range
		default:
			t.Fatalf("%s: resolved = %#v", tc.path, r)
		}
		// Lexical bounds are unchanged; the numeric accessors resolve min/max.
		if lexical := bounds(got); !reflect.DeepEqual(lexical, tc.lexical) {
			t.Errorf("%s: lexical = %v, want %v", tc.path, lexical, tc.lexical)
		}
		if numeric := numericBounds(got); !reflect.DeepEqual(numeric, tc.numeric) {
			t.Errorf("%s: numeric = %v, want %v", tc.path, numeric, tc.numeric)
		}
		for _, r := range got {
			if _, ok := r.MinLength(); ok {
				t.Errorf("%s: MinLength reported a range bound", tc.path)
			}
		}
	}

	dec := leafType(t, mod, "/nb:dec").Resolved().(cambium.ResolvedDecimal64)
	if lo, _ := dec.Range[0].MinNumber(); lo.FractionDigits != 3 || !lo.Negative || lo.Value != 1<<63 {
		t.Fatalf("decimal64 min = %#v", lo)
	}

	str := leafType(t, mod, "/nb:str").Resolved().(cambium.ResolvedString)
	if got, want := bounds(str.Length), []bound{{"0", "3"}, {"7", "7"}, {"10", "max"}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("string lexical length = %v, want %v", got, want)
	}
	if got, want := lengthBounds(t, str.Length), []lengthBound{{0, 3}, {7, 7}, {10, 18446744073709551615}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("string length = %v, want %v", got, want)
	}
	if got, want := numericBounds(str.Length), []bound{{"0", "3"}, {"7", "7"}, {"10", "18446744073709551615"}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("string length numbers = %v, want %v", got, want)
	}
	bin := leafType(t, mod, "/nb:bin").Resolved().(cambium.ResolvedBinary)
	if got, want := lengthBounds(t, bin.Length), []lengthBound{{0, 16}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("binary length = %v, want %v", got, want)
	}

	var zero cambium.RangeBound
	if _, ok := zero.MinNumber(); ok {
		t.Fatal("zero RangeBound MinNumber reported a value")
	}
	if _, ok := zero.MaxLength(); ok {
		t.Fatal("zero RangeBound MaxLength reported a value")
	}
}

func TestDerivedRestrictionResolvesMinMaxAgainstBase(t *testing.T) {
	// RFC 7950 sections 9.2.4 and 9.4.4: min and max stand for the limits of
	// the type being restricted, not of the built-in type.
	mod := s3Module(t, `module derived-bounds {
  yang-version 1.1; namespace "urn:derived-bounds"; prefix db;
  typedef pct { type int32 { range "1..100"; } }
  typedef name { type string { length "5..10"; } }
  typedef open-name { type string { length "min..10"; } }
  typedef temp { type decimal64 { fraction-digits 2; range "-1.5..20"; } }
  typedef blob { type binary { length "2..64"; } }
  leaf low { type pct { range "min..50 | 60..max"; } }
  leaf nm { type name { length "min..8"; } }
  leaf open { type open-name { length "min..8"; } }
  leaf tmp { type temp { range "min..3 | 5..max"; } }
  leaf bl { type blob { length "4..max"; } }
}`)
	low := leafType(t, mod, "/db:low").Resolved().(cambium.ResolvedInt)
	if got, want := bounds(low.Range), []bound{{"1", "50"}, {"60", "100"}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("int32 lexical = %v, want %v", got, want)
	}
	if got, want := numericBounds(low.Range), []bound{{"1", "50"}, {"60", "100"}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("int32 numeric = %v, want %v", got, want)
	}
	nm := leafType(t, mod, "/db:nm").Resolved().(cambium.ResolvedString)
	if got, want := bounds(nm.Length), []bound{{"5", "8"}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("string lexical = %v, want %v", got, want)
	}
	if got, want := lengthBounds(t, nm.Length), []lengthBound{{5, 8}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("string length = %v, want %v", got, want)
	}
	// A min inherited from a built-in min stays the lexical keyword.
	open := leafType(t, mod, "/db:open").Resolved().(cambium.ResolvedString)
	if got, want := bounds(open.Length), []bound{{"min", "8"}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("open lexical = %v, want %v", got, want)
	}
	if got, want := lengthBounds(t, open.Length), []lengthBound{{0, 8}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("open length = %v, want %v", got, want)
	}
	tmp := leafType(t, mod, "/db:tmp").Resolved().(cambium.ResolvedDecimal64)
	if got, want := numericBounds(tmp.Range), []bound{{"-1.50", "3.00"}, {"5.00", "20.00"}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("decimal64 numeric = %v, want %v", got, want)
	}
	bl := leafType(t, mod, "/db:bl").Resolved().(cambium.ResolvedBinary)
	if got, want := lengthBounds(t, bl.Length), []lengthBound{{4, 64}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("binary length = %v, want %v", got, want)
	}

	// Resolution does not loosen the subset rule.
	for _, tc := range []struct{ name, typ string }{
		{"explicit lower bound below base", `type pct { range "0..50"; }`},
		{"min..max across a base gap", `type gap { range "min..max"; }`},
		{"length below base", `type name { length "4..max"; }`},
	} {
		_, err := buildPolicyContext(t, cambium.DeviationPolicy{}, `module derived-neg {
  yang-version 1.1; namespace "urn:derived-neg"; prefix dn;
  typedef pct { type int32 { range "1..100"; } }
  typedef gap { type int32 { range "1..5 | 10..20"; } }
  typedef name { type string { length "5..10"; } }
  leaf x { `+tc.typ+` }
}`)
		if err == nil || !strings.Contains(err.Error(), "is not within the base restriction") {
			t.Errorf("%s: Build error = %v, want subset violation", tc.name, err)
		}
	}
}

func TestDecimal64RangeBoundOutsideValueSpaceFails(t *testing.T) {
	_, err := buildPolicyContext(t, cambium.DeviationPolicy{}, `module dec-overflow {
  yang-version 1.1; namespace "urn:dec-overflow"; prefix do;
  leaf d { type decimal64 { fraction-digits 1; range "0..99999999999999999999"; } }
}`)
	if err == nil || !strings.Contains(err.Error(), `invalid decimal64 range bound "99999999999999999999"`) {
		t.Fatalf("Build error = %v, want out-of-range decimal64 bound", err)
	}
}
