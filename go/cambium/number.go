// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

package cambium

import (
	"errors"
	"fmt"
	"math"
	"math/big"
	"sort"
	"strconv"
	"strings"
)

const (
	// MaxInt64 corresponds to the maximum value of a signed int64.
	MaxInt64 = 1<<63 - 1
	// MinInt64 corresponds to the minimum value of a signed int64.
	MinInt64 = -1 << 63
	// MinDecimal64 is the minimum decimal64 numeric limit.
	MinDecimal64 float64 = -922337203685477580.8
	// MaxDecimal64 is the maximum decimal64 numeric limit.
	MaxDecimal64 float64 = 922337203685477580.7
	// AbsMinInt64 is the absolute value of MinInt64.
	AbsMinInt64 = 1 << 63
	// MaxEnum is the maximum YANG enumeration value.
	MaxEnum = 1<<31 - 1
	// MinEnum is the minimum YANG enumeration value.
	MinEnum = -1 << 31
	// MaxBitfieldSize is the maximum number of bits in a bitfield.
	MaxBitfieldSize = 1 << 32
	// MaxFractionDigits is the maximum decimal64 fraction-digits value.
	MaxFractionDigits uint8 = 18

	decimalPad18 = "000000000000000000"
)

// Int8Range, Int16Range, Int32Range, Int64Range, Uint8Range, Uint16Range,
// Uint32Range, and Uint64Range are the inclusive value ranges of the
// corresponding YANG integer base types.
var (
	Int8Range  = mustParseRangesInt("-128..127")
	Int16Range = mustParseRangesInt("-32768..32767")
	Int32Range = mustParseRangesInt("-2147483648..2147483647")
	Int64Range = mustParseRangesInt("-9223372036854775808..9223372036854775807")

	Uint8Range  = mustParseRangesInt("0..255")
	Uint16Range = mustParseRangesInt("0..65535")
	Uint32Range = mustParseRangesInt("0..4294967295")
	Uint64Range = mustParseRangesInt("0..18446744073709551615")
)

// Number is a YANG integer or decimal64 value.
type Number struct {
	Value          uint64
	FractionDigits uint8
	Negative       bool
}

// IsDecimal reports whether n is a decimal64 value.
func (n Number) IsDecimal() bool { return n.FractionDigits != 0 }

// String returns n in YANG lexical notation.
func (n Number) String() string {
	out := strconv.FormatUint(n.Value, 10)
	if n.IsDecimal() {
		fd := int(n.FractionDigits)
		if len(out) <= fd {
			out = strings.Repeat("0", fd-len(out)+1) + out
		}
		split := len(out) - fd
		out = out[:split] + "." + out[split:]
	}
	if n.Negative {
		out = "-" + out
	}
	return out
}

// Int returns n as an int64 if it is an integer in range.
func (n Number) Int() (int64, error) {
	if n.IsDecimal() {
		return 0, fmt.Errorf("called Int() on decimal64 value")
	}
	const maxInt64 = uint64(1<<63 - 1)
	const minInt64Abs = uint64(1 << 63)
	switch {
	case n.Negative && n.Value == minInt64Abs:
		return -1 << 63, nil
	case n.Negative && n.Value <= maxInt64:
		return -int64(n.Value), nil //nolint:gosec // n.Value is bounded by maxInt64 above.
	case !n.Negative && n.Value <= maxInt64:
		return int64(n.Value), nil //nolint:gosec // n.Value is bounded by maxInt64 above.
	default:
		return 0, fmt.Errorf("signed integer overflow")
	}
}

// Trunc returns the whole part of abs(n).
func (n Number) Trunc() uint64 {
	return n.Value / pow10Number(n.FractionDigits)
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

// Less reports whether n is less than m.
func (n Number) Less(m Number) bool {
	return n.scaledBigInt().Cmp(m.scaledBigInt()) < 0
}

// Equal reports whether n and m have the same numeric value.
func (n Number) Equal(m Number) bool {
	return n.scaledBigInt().Cmp(m.scaledBigInt()) == 0
}

func (n Number) scaledBigInt() *big.Int {
	out := new(big.Int).SetUint64(n.Value)
	scale := 18 - int64(n.FractionDigits)
	if scale > 0 {
		mul := new(big.Int).Exp(big.NewInt(10), big.NewInt(scale), nil)
		out.Mul(out, mul)
	}
	if n.Negative {
		out.Neg(out)
	}
	return out
}

// YRange is one inclusive range of consecutive numbers.
type YRange struct {
	Min Number
	Max Number
}

// Valid reports whether r has a min less than or equal to max.
func (r YRange) Valid() bool {
	return !r.Max.Less(r.Min)
}

// String returns r in YANG range notation.
func (r YRange) String() string {
	if r.Min.Equal(r.Max) {
		return r.Min.String()
	}
	return r.Min.String() + ".." + r.Max.String()
}

// Equal reports whether r and s have equal bounds.
func (r YRange) Equal(s YRange) bool {
	return r.Min.Equal(s.Min) && r.Max.Equal(s.Max)
}

// YangRange is a set of non-overlapping YANG ranges.
type YangRange []YRange

// String returns the ranges in YANG notation separated by '|'.
func (r YangRange) String() string {
	parts := make([]string, 0, len(r))
	for _, part := range r {
		parts = append(parts, part.String())
	}
	return strings.Join(parts, "|")
}

func (r YangRange) Len() int      { return len(r) }
func (r YangRange) Swap(i, j int) { r[i], r[j] = r[j], r[i] }
func (r YangRange) Less(i, j int) bool {
	switch {
	case r[i].Min.Less(r[j].Min):
		return true
	case r[j].Min.Less(r[i].Min):
		return false
	default:
		return r[i].Max.Less(r[j].Max)
	}
}

// Sort sorts r by min, then max.
func (r YangRange) Sort() { sort.Sort(r) }

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

// Equal reports whether r and q contain the same ordered ranges.
func (r YangRange) Equal(q YangRange) bool {
	if len(r) != len(q) {
		return false
	}
	for i := range r {
		if !r[i].Equal(q[i]) {
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

// FromFloat creates a decimal Number from f using goyang-compatible decimal64
// behavior.
func FromFloat(f float64) Number {
	if f > MaxDecimal64 {
		return Number{Value: FromInt(MaxInt64).Value, FractionDigits: 1}
	}
	if f < MinDecimal64 {
		return Number{Negative: true, Value: FromInt(MaxInt64).Value, FractionDigits: 1}
	}

	fracDigits := uint8(1)
	f *= 10.0
	for ; Frac(f) != 0.0 && fracDigits <= MaxFractionDigits; fracDigits++ {
		f *= 10.0
	}
	negative := false
	if f < 0 {
		negative = true
		f = -f
	}
	return Number{Negative: negative, Value: uint64(f), FractionDigits: fracDigits}
}

// ParseInt parses s as a YANG integer Number.
func ParseInt(s string) (Number, error) {
	s = strings.TrimSpace(s)
	var n Number
	switch s {
	case "":
		return n, errors.New("converting empty string to number")
	case "+", "-":
		return n, errors.New("sign with no value")
	}

	unsigned := s
	switch s[0] {
	case '+':
		unsigned = s[1:]
	case '-':
		n.Negative = true
		unsigned = s[1:]
	}

	var err error
	n.Value, err = strconv.ParseUint(unsigned, 0, 64)
	return n, err
}

// ParseDecimal parses s as a YANG decimal64 Number.
func ParseDecimal(s string, fracDigRequired uint8) (Number, error) {
	s = strings.TrimSpace(s)
	switch s {
	case "":
		return Number{}, errors.New("converting empty string to number")
	case "+", "-":
		return Number{}, errors.New("sign with no value")
	}
	return decimalValueFromString(s, fracDigRequired)
}

func decimalValueFromString(s string, fracDigRequired uint8) (Number, error) {
	if fracDigRequired > MaxFractionDigits || fracDigRequired < 1 {
		return Number{}, fmt.Errorf("invalid number of fraction digits %d > max of %d, minimum 1", fracDigRequired, MaxFractionDigits)
	}

	raw := s
	dot := strings.Index(s, ".")
	var fracDigits uint8
	if dot >= 0 {
		n := len(s) - 1 - dot
		if n > int(MaxFractionDigits) {
			return Number{}, fmt.Errorf("%s has too much precision, expect <= %d fractional digits", s, fracDigRequired)
		}
		fracDigits = uint8(n) //nolint:gosec // n is in [0, MaxFractionDigits]: dot is a valid index and the upper bound is checked above
		s = s[:dot] + s[dot+1:]
	}
	if fracDigits > fracDigRequired {
		return Number{}, fmt.Errorf("%s has too much precision, expect <= %d fractional digits", s, fracDigRequired)
	}
	s += decimalPad18[:fracDigRequired-fracDigits]

	v, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return Number{}, fmt.Errorf("%s is not a valid decimal number: %w", raw, err)
	}
	negative := v < 0
	var mag uint64
	if negative {
		// Negate in the unsigned domain so math.MinInt64 does not overflow.
		mag = uint64(-(v + 1)) + 1 //nolint:gosec // v is negative, so -(v + 1) is non-negative and in range.
	} else {
		mag = uint64(v)
	}
	return Number{Value: mag, FractionDigits: fracDigRequired, Negative: negative}, nil
}

// ParseRangesInt parses an integer range expression.
func ParseRangesInt(s string) (YangRange, error) {
	return YangRange{}.parseChildRanges(s, false, 0)
}

// ParseRangesDecimal parses a decimal64 range expression.
func ParseRangesDecimal(s string, fracDigRequired uint8) (YangRange, error) {
	return YangRange{}.parseChildRanges(s, true, fracDigRequired)
}

func (r YangRange) parseChildRanges(s string, decimal bool, fracDigRequired uint8) (YangRange, error) {
	parseNumber := func(raw string) (Number, error) {
		switch {
		case raw == "max":
			if len(r) == 0 {
				return Number{}, errors.New("cannot resolve 'max' keyword using an empty YangRange parent object")
			}
			hi := r[len(r)-1].Max
			hi.FractionDigits = fracDigRequired
			return hi, nil
		case raw == "min":
			if len(r) == 0 {
				return Number{}, errors.New("cannot resolve 'min' keyword using an empty YangRange parent object")
			}
			lo := r[0].Min
			lo.FractionDigits = fracDigRequired
			return lo, nil
		case decimal:
			return ParseDecimal(raw, fracDigRequired)
		default:
			return ParseInt(raw)
		}
	}

	parts := strings.Split(s, "|")
	out := make(YangRange, len(parts))
	for i, part := range parts {
		bounds := strings.Split(part, "..")
		lo, err := parseNumber(strings.TrimSpace(bounds[0]))
		if err != nil {
			return nil, err
		}
		hi := lo
		switch len(bounds) {
		case 1:
		case 2:
			hi, err = parseNumber(strings.TrimSpace(bounds[1]))
			if err != nil {
				return nil, err
			}
		default:
			return nil, fmt.Errorf("too many '..' in %s", part)
		}
		if hi.Less(lo) {
			return nil, fmt.Errorf("range boundaries out of order (%s less than %s): %s", hi, lo, part)
		}
		out[i] = YRange{Min: lo, Max: hi}
	}
	out.Sort()
	out = coalesceRanges(out)
	if !r.Contains(out) {
		return nil, fmt.Errorf("%v not within %v", s, r)
	}
	if err := out.Validate(); err != nil {
		return nil, err
	}
	return out, nil
}

func coalesceRanges(r YangRange) YangRange {
	if len(r) < 2 {
		return r
	}
	out := make(YangRange, len(r))
	i := 0
	out[i] = r[0]
	for _, next := range r[1:] {
		if out[i].Max.addQuantum(1).Less(next.Min) {
			i++
			out[i] = next
		} else if out[i].Max.Less(next.Max) {
			out[i].Max = next.Max
		}
	}
	return out[:i+1]
}

func mustParseRangesInt(s string) YangRange {
	r, err := ParseRangesInt(s)
	if err != nil {
		panic(err)
	}
	return r
}

// Frac returns the fractional part of f.
func Frac(f float64) float64 {
	return f - math.Trunc(f)
}

func pow10Number(e uint8) uint64 {
	out := uint64(1)
	for ; e > 0; e-- {
		out *= 10
	}
	return out
}
