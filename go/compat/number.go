// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

package compat

import (
	"errors"
	"sort"

	"github.com/signalbreak-labs/cambium/go/cambium"
)

const (
	// MaxInt64 corresponds to the maximum value of a signed int64.
	MaxInt64 = cambium.MaxInt64
	// MinInt64 corresponds to the minimum value of a signed int64.
	MinInt64 = cambium.MinInt64
	// MinDecimal64 is the smallest representable decimal64 value.
	MinDecimal64 float64 = cambium.MinDecimal64
	// MaxDecimal64 is the largest representable decimal64 value.
	MaxDecimal64 float64 = cambium.MaxDecimal64
	// AbsMinInt64 is the absolute value of MinInt64.
	AbsMinInt64 = cambium.AbsMinInt64
	// MaxFractionDigits is the maximum number of decimal64 fraction digits.
	MaxFractionDigits uint8 = cambium.MaxFractionDigits
)

// Range tables for the YANG built-in integer types.
var (
	Int8Range  = fromCambiumRange(cambium.Int8Range)
	Int16Range = fromCambiumRange(cambium.Int16Range)
	Int32Range = fromCambiumRange(cambium.Int32Range)
	Int64Range = fromCambiumRange(cambium.Int64Range)

	Uint8Range  = fromCambiumRange(cambium.Uint8Range)
	Uint16Range = fromCambiumRange(cambium.Uint16Range)
	Uint32Range = fromCambiumRange(cambium.Uint32Range)
	Uint64Range = fromCambiumRange(cambium.Uint64Range)
)

// Trunc returns the whole part of abs(n).
func (n Number) Trunc() uint64 {
	return n.Value / pow10Compat(n.FractionDigits)
}

func (n Number) addQuantum(i uint64) Number {
	if n.Negative {
		if n.Value <= i {
			n.Value = i - n.Value
			n.Negative = false
			return n
		}
		n.Value -= i
		return n
	}
	n.Value += i
	return n
}

// Validate returns an error when r is unsorted, invalid, or overlapping.
func (r YangRange) Validate() error {
	if !sort.IsSorted(r) {
		return errors.New("range not sorted")
	}
	if len(r) == 0 {
		return nil
	}
	prev := r[0]
	if !prev.Valid() {
		return errors.New("invalid number")
	}
	for _, next := range r[1:] {
		if !next.Valid() {
			return errors.New("invalid number")
		}
		if !prev.Max.Less(next.Min) {
			return errors.New("overlapping ranges")
		}
		prev = next
	}
	return nil
}

// Contains reports whether every value permitted by s is also permitted by r.
func (r YangRange) Contains(s YangRange) bool {
	if len(r) == 0 || len(s) == 0 {
		return true
	}
	ri := 0
	for _, ss := range s {
		for r[ri].Max.Less(ss.Min) {
			ri++
			if ri == len(r) {
				return false
			}
		}
		if ss.Min.Less(r[ri].Min) || r[ri].Max.Less(ss.Max) {
			return false
		}
	}
	return true
}

// FromInt creates a Number from an int64.
func FromInt(i int64) Number {
	return fromCambiumNumber(cambium.FromInt(i))
}

// FromUint creates a Number from a uint64.
func FromUint(i uint64) Number {
	return fromCambiumNumber(cambium.FromUint(i))
}

// FromFloat creates a decimal Number from f using goyang decimal64 behavior.
func FromFloat(f float64) Number {
	return fromCambiumNumber(cambium.FromFloat(f))
}

// ParseInt parses s as a goyang-compatible integer Number.
func ParseInt(s string) (Number, error) {
	n, err := cambium.ParseInt(s)
	return fromCambiumNumber(n), err
}

// ParseDecimal parses s as a goyang-compatible decimal64 Number.
func ParseDecimal(s string, fracDigRequired uint8) (Number, error) {
	n, err := cambium.ParseDecimal(s, fracDigRequired)
	return fromCambiumNumber(n), err
}

// ParseRangesInt parses an integer range expression.
func ParseRangesInt(s string) (YangRange, error) {
	r, err := cambium.ParseRangesInt(s)
	return fromCambiumRange(r), err
}

// ParseRangesDecimal parses a decimal64 range expression.
func ParseRangesDecimal(s string, fracDigRequired uint8) (YangRange, error) {
	r, err := cambium.ParseRangesDecimal(s, fracDigRequired)
	return fromCambiumRange(r), err
}

// Frac returns the fractional part of f.
func Frac(f float64) float64 {
	return cambium.Frac(f)
}

func fromCambiumNumber(n cambium.Number) Number {
	return Number{
		Value:          n.Value,
		FractionDigits: n.FractionDigits,
		Negative:       n.Negative,
	}
}

func fromCambiumRange(r cambium.YangRange) YangRange {
	out := make(YangRange, 0, len(r))
	for _, part := range r {
		out = append(out, YRange{
			Min: fromCambiumNumber(part.Min),
			Max: fromCambiumNumber(part.Max),
		})
	}
	return out
}

func pow10Compat(e uint8) uint64 {
	out := uint64(1)
	for ; e > 0; e-- {
		out *= 10
	}
	return out
}
