// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

package compat

import (
	"errors"
	"sort"

	upstream "github.com/signalbreak-labs/cambium/go/internal/yangparse/upstream/yang"
)

const (
	// MaxInt64 corresponds to the maximum value of a signed int64.
	MaxInt64 = 1<<63 - 1
	// MinInt64 corresponds to the minimum value of a signed int64.
	MinInt64 = -1 << 63
	// MinDecimal64 and MaxDecimal64 are the decimal64 numeric limits.
	MinDecimal64 float64 = -922337203685477580.8
	MaxDecimal64 float64 = 922337203685477580.7
	// AbsMinInt64 is the absolute value of MinInt64.
	AbsMinInt64 = 1 << 63
	// MaxFractionDigits is the maximum number of decimal64 fraction digits.
	MaxFractionDigits uint8 = 18
)

var (
	Int8Range  = fromUpstreamRange(upstream.Int8Range)
	Int16Range = fromUpstreamRange(upstream.Int16Range)
	Int32Range = fromUpstreamRange(upstream.Int32Range)
	Int64Range = fromUpstreamRange(upstream.Int64Range)

	Uint8Range  = fromUpstreamRange(upstream.Uint8Range)
	Uint16Range = fromUpstreamRange(upstream.Uint16Range)
	Uint32Range = fromUpstreamRange(upstream.Uint32Range)
	Uint64Range = fromUpstreamRange(upstream.Uint64Range)
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
	if i < 0 {
		return Number{Negative: true, Value: uint64(-i)}
	}
	return Number{Value: uint64(i)}
}

// FromUint creates a Number from a uint64.
func FromUint(i uint64) Number {
	return Number{Value: i}
}

// FromFloat creates a decimal Number from f using goyang decimal64 behavior.
func FromFloat(f float64) Number {
	return fromUpstreamNumber(upstream.FromFloat(f))
}

// ParseInt parses s as a goyang-compatible integer Number.
func ParseInt(s string) (Number, error) {
	n, err := upstream.ParseInt(s)
	return fromUpstreamNumber(n), err
}

// ParseDecimal parses s as a goyang-compatible decimal64 Number.
func ParseDecimal(s string, fracDigRequired uint8) (Number, error) {
	n, err := upstream.ParseDecimal(s, fracDigRequired)
	return fromUpstreamNumber(n), err
}

// ParseRangesInt parses an integer range expression.
func ParseRangesInt(s string) (YangRange, error) {
	r, err := upstream.ParseRangesInt(s)
	return fromUpstreamRange(r), err
}

// ParseRangesDecimal parses a decimal64 range expression.
func ParseRangesDecimal(s string, fracDigRequired uint8) (YangRange, error) {
	r, err := upstream.ParseRangesDecimal(s, fracDigRequired)
	return fromUpstreamRange(r), err
}

// Frac returns the fractional part of f.
func Frac(f float64) float64 {
	return upstream.Frac(f)
}

func fromUpstreamNumber(n upstream.Number) Number {
	return Number{
		Value:          n.Value,
		FractionDigits: n.FractionDigits,
		Negative:       n.Negative,
	}
}

func fromUpstreamRange(r upstream.YangRange) YangRange {
	out := make(YangRange, 0, len(r))
	for _, part := range r {
		out = append(out, YRange{
			Min: fromUpstreamNumber(part.Min),
			Max: fromUpstreamNumber(part.Max),
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
