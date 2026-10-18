// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

package xsdregex

// AtomRanges resolves one complete XSD atom using the same character semantics
// as NativePattern. Callers must bound source size and nested class depth before
// calling. Unicode expansion has at most 8192 intervals per escape; callers can
// reserve that conservative allowance before translation.
func AtomRanges(source []rune) ([][2]rune, bool) {
	var ranges []runeRange
	var ok bool
	if len(source) == 0 {
		return nil, false
	}
	switch source[0] {
	case '[':
		var end int
		ranges, end, ok = parseClassExprRanges(source, 0)
		ok = ok && end == len(source)-1
	case '\\':
		var atom classAtom
		atom, ok = parseClassEscapeAtom(source, 0)
		ok = ok && atom.next == len(source)
		ranges = atom.ranges
	case '.':
		ranges = []runeRange{{0, 9}, {11, 12}, {14, 0x10FFFF}}
		ok = len(source) == 1
	default:
		ranges = []runeRange{{source[0], source[0]}}
		ok = len(source) == 1
	}
	if !ok {
		return nil, false
	}
	out := make([][2]rune, 0, len(ranges)+1)
	for _, r := range ranges {
		// The portable alphabet contains Unicode scalar values only.
		if r.lo < 0xD800 {
			out = append(out, [2]rune{r.lo, min(r.hi, 0xD7FF)})
		}
		if r.hi > 0xDFFF {
			out = append(out, [2]rune{max(r.lo, 0xE000), r.hi})
		}
	}
	return out, true
}
