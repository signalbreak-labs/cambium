// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

package cambium_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/signalbreak-labs/cambium/go/cambium"
	upstream "github.com/signalbreak-labs/cambium/go/internal/yangparse/upstream/yang"
)

func TestNativeNumbersMatchGoyang(t *testing.T) {
	intInputs := []string{
		"0",
		"+0x10",
		"077",
		"-9223372036854775808",
		"18446744073709551615",
		"",
		"+",
		"not-number",
	}
	for _, input := range intInputs {
		t.Run("ParseInt/"+input, func(t *testing.T) {
			got, gotErr := cambium.ParseInt(input)
			want, wantErr := upstream.ParseInt(input)
			assertNumberResultMatchesGoyang(t, "ParseInt", got, gotErr, want, wantErr)
		})
	}

	decimalInputs := []struct {
		value          string
		fractionDigits uint8
	}{
		{value: "1.20", fractionDigits: 2},
		{value: "-0.01", fractionDigits: 2},
		{value: "+3.4", fractionDigits: 3},
		{value: "1.234", fractionDigits: 2},
		{value: "", fractionDigits: 2},
		{value: "1.0", fractionDigits: 0},
		{value: "1.0", fractionDigits: cambium.MaxFractionDigits + 1},
	}
	for _, input := range decimalInputs {
		t.Run(fmt.Sprintf("ParseDecimal/%s/%d", input.value, input.fractionDigits), func(t *testing.T) {
			got, gotErr := cambium.ParseDecimal(input.value, input.fractionDigits)
			want, wantErr := upstream.ParseDecimal(input.value, input.fractionDigits)
			assertNumberResultMatchesGoyang(t, "ParseDecimal", got, gotErr, want, wantErr)
		})
	}

	floatInputs := []float64{
		0,
		1.25,
		-1.25,
		cambium.MaxDecimal64 * 2,
		cambium.MinDecimal64 * 2,
	}
	for _, input := range floatInputs {
		t.Run(fmt.Sprintf("FromFloat/%g", input), func(t *testing.T) {
			got := cambium.FromFloat(input)
			want := upstream.FromFloat(input)
			assertNumberMatchesGoyang(t, "FromFloat", got, want)
		})
	}

	if got, want := cambium.Frac(3.25), upstream.Frac(3.25); got != want {
		t.Fatalf("Frac(3.25) = %v, want goyang %v", got, want)
	}
}

func TestNativeRangesMatchGoyang(t *testing.T) {
	intInputs := []string{
		"1..5 | 6..10 | 20",
		"20 | 1..5 | 6..10",
		"-5..-1 | 0",
		"10..1",
		"1..2..3",
		"bad",
	}
	for _, input := range intInputs {
		t.Run("ParseRangesInt/"+input, func(t *testing.T) {
			got, gotErr := cambium.ParseRangesInt(input)
			want, wantErr := upstream.ParseRangesInt(input)
			assertRangeResultMatchesGoyang(t, "ParseRangesInt", got, gotErr, want, wantErr)
		})
	}

	decimalInputs := []struct {
		value          string
		fractionDigits uint8
	}{
		{value: "1.20..3.40", fractionDigits: 2},
		{value: "-1.00..-0.50 | 0.00", fractionDigits: 2},
		{value: "1.234..2.000", fractionDigits: 2},
		{value: "3.00..1.00", fractionDigits: 2},
	}
	for _, input := range decimalInputs {
		t.Run(fmt.Sprintf("ParseRangesDecimal/%s/%d", input.value, input.fractionDigits), func(t *testing.T) {
			got, gotErr := cambium.ParseRangesDecimal(input.value, input.fractionDigits)
			want, wantErr := upstream.ParseRangesDecimal(input.value, input.fractionDigits)
			assertRangeResultMatchesGoyang(t, "ParseRangesDecimal", got, gotErr, want, wantErr)
		})
	}
}

func TestNativeBuiltinRangesMatchGoyang(t *testing.T) {
	tests := []struct {
		name string
		got  cambium.YangRange
		want upstream.YangRange
	}{
		{name: "Int8Range", got: cambium.Int8Range, want: upstream.Int8Range},
		{name: "Int16Range", got: cambium.Int16Range, want: upstream.Int16Range},
		{name: "Int32Range", got: cambium.Int32Range, want: upstream.Int32Range},
		{name: "Int64Range", got: cambium.Int64Range, want: upstream.Int64Range},
		{name: "Uint8Range", got: cambium.Uint8Range, want: upstream.Uint8Range},
		{name: "Uint16Range", got: cambium.Uint16Range, want: upstream.Uint16Range},
		{name: "Uint32Range", got: cambium.Uint32Range, want: upstream.Uint32Range},
		{name: "Uint64Range", got: cambium.Uint64Range, want: upstream.Uint64Range},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertRangeMatchesGoyang(t, tt.name, tt.got, tt.want)
		})
	}
}

func TestNativeYangRangeSafety(t *testing.T) {
	adjacent := cambium.YangRange{
		{Min: cambium.FromInt(1), Max: cambium.FromInt(1)},
		{Min: cambium.FromInt(2), Max: cambium.FromInt(2)},
	}
	if err := adjacent.Validate(); err != nil {
		t.Fatalf("Validate adjacent ranges = %v, want nil", err)
	}

	overlap := cambium.YangRange{
		{Min: cambium.FromInt(1), Max: cambium.FromInt(2)},
		{Min: cambium.FromInt(2), Max: cambium.FromInt(3)},
	}
	if err := overlap.Validate(); err == nil || err.Error() != "overlapping ranges" {
		t.Fatalf("Validate overlap = %v, want overlapping ranges", err)
	}

	if !cambium.Int8Range.Contains(cambium.YangRange{{Min: cambium.FromInt(-1), Max: cambium.FromInt(1)}}) {
		t.Fatal("Int8Range does not contain -1..1")
	}
	if cambium.Int8Range.Contains(cambium.YangRange{{Min: cambium.FromInt(-200), Max: cambium.FromInt(1)}}) {
		t.Fatal("Int8Range contains -200..1")
	}
}

func assertNumberResultMatchesGoyang(t *testing.T, label string, got cambium.Number, gotErr error, want upstream.Number, wantErr error) {
	t.Helper()
	if (gotErr == nil) != (wantErr == nil) {
		t.Fatalf("%s error nil = %v, want goyang %v (got %v want %v)", label, gotErr == nil, wantErr == nil, gotErr, wantErr)
	}
	if gotErr != nil {
		return
	}
	assertNumberMatchesGoyang(t, label, got, want)
}

func assertNumberMatchesGoyang(t *testing.T, label string, got cambium.Number, want upstream.Number) {
	t.Helper()
	if got.Value != want.Value || got.FractionDigits != want.FractionDigits || got.Negative != want.Negative {
		t.Fatalf("%s = %#v, want goyang %#v", label, got, want)
	}
	if got.String() != want.String() {
		t.Fatalf("%s String = %q, want goyang %q", label, got.String(), want.String())
	}
	if got.IsDecimal() != want.IsDecimal() {
		t.Fatalf("%s IsDecimal = %v, want goyang %v", label, got.IsDecimal(), want.IsDecimal())
	}
}

func assertRangeResultMatchesGoyang(t *testing.T, label string, got cambium.YangRange, gotErr error, want upstream.YangRange, wantErr error) {
	t.Helper()
	if (gotErr == nil) != (wantErr == nil) {
		t.Fatalf("%s error nil = %v, want goyang %v (got %v want %v)", label, gotErr == nil, wantErr == nil, gotErr, wantErr)
	}
	if gotErr != nil {
		return
	}
	assertRangeMatchesGoyang(t, label, got, want)
}

func assertRangeMatchesGoyang(t *testing.T, label string, got cambium.YangRange, want upstream.YangRange) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s len = %d, want goyang %d", label, len(got), len(want))
	}
	for i := range got {
		assertNumberMatchesGoyang(t, fmt.Sprintf("%s[%d].Min", label, i), got[i].Min, want[i].Min)
		assertNumberMatchesGoyang(t, fmt.Sprintf("%s[%d].Max", label, i), got[i].Max, want[i].Max)
	}
	wantString := upstreamRangeString(want)
	if got.String() != wantString {
		t.Fatalf("%s String = %q, want goyang %q", label, got.String(), wantString)
	}
}

func upstreamRangeString(r upstream.YangRange) string {
	parts := make([]string, 0, len(r))
	for _, part := range r {
		parts = append(parts, part.String())
	}
	return strings.Join(parts, "|")
}
