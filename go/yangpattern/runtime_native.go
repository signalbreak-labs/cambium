// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

package yangpattern

import (
	"fmt"
	"regexp"
	"regexp/syntax"
	"strings"
)

type nativeSize struct{ source, instructions uint64 }

func (p *Program) nativeExpression(expand bool, l MatchLimits) (source string, instructions uint64, err error) {
	tableBytes := mulSat(uint64(len(p.data.Nodes)), 64)
	if err := limitError("match", "native", "bytes", l.Bytes, tableBytes); err != nil {
		return "", 0, err
	}
	var work uint64
	sizes := make([]nativeSize, len(p.data.Nodes))
	seen := make([]bool, len(sizes))
	var measure func(uint32) (nativeSize, error)
	measure = func(id uint32) (nativeSize, error) {
		if seen[id] {
			return sizes[id], nil
		}
		work++
		if err := limitError("match", "native", "work", l.Work, work); err != nil {
			return nativeSize{}, err
		}
		n := p.data.Nodes[id]
		s := nativeSize{8, 1}
		switch n.Op {
		case OpEmpty:
			s.source = 26
		case OpEpsilon:
			s.instructions = 0
		case OpClass:
			s.source = addSat(2, mulSat(uint64(len(n.Ranges)), 24))
		case OpAlternate, OpConcat:
			a, err := measure(n.Children[0])
			if err != nil {
				return s, err
			}
			b, err := measure(n.Children[1])
			if err != nil {
				return s, err
			}
			s.source = addSat(8, addSat(a.source, b.source))
			s.instructions = addSat(a.instructions, b.instructions)
			if n.Op == OpAlternate {
				s.instructions++
			}
		case OpRepeat:
			child, err := measure(n.Children[0])
			if err != nil {
				return s, err
			}
			lo, hi := p.bounds[id].lower, p.bounds[id].upper
			copies := hi
			if n.Unbounded {
				copies = addSat(lo, 1)
			}
			if !expand && (lo > 1000 || (!n.Unbounded && hi > 1000)) {
				return s, &ConfigurationError{"count cannot be represented by native regexp syntax"}
			}
			splits := hi - lo
			if n.Unbounded {
				splits = 1
			}
			s.instructions = addSat(mulSat(child.instructions, copies), splits)
			s.source = addSat(child.source, 32)
			if expand {
				s.source = mulSat(addSat(child.source, 8), copies)
			}
		}
		if err := limitError("match", "native", "source bytes", l.NativeSourceBytes, addSat(s.source, 8)); err != nil {
			return s, err
		}
		if err := limitError("match", "native", "instructions", l.NativeInstructions, addSat(s.instructions, 4)); err != nil {
			return s, err
		}
		seen[id] = true
		sizes[id] = s
		return s, nil
	}
	size, err := measure(p.data.Root)
	if err != nil {
		return "", 0, err
	}
	// Account native parser/compiler expansion separately from residual arenas.
	if err := limitError("match", "native", "bytes", l.Bytes, addSat(tableBytes, addSat(mulSat(size.source, 8), mulSat(size.instructions, 256)))); err != nil {
		return "", 0, err
	}
	if err := limitError("match", "native", "work", l.Work, addSat(work, addSat(size.source, size.instructions))); err != nil {
		return "", 0, err
	}
	var out strings.Builder
	if size.source > uint64(int(^uint(0)>>1))-8 {
		return "", 0, &ResourceError{"match", "native", "source address space", uint64(int(^uint(0)>>1)) - 8, size.source}
	}
	out.Grow(int(size.source + 8)) // #nosec G115 -- checked against address space above.
	out.WriteString("^(?:")
	var emit func(uint32)
	emit = func(id uint32) {
		n := p.data.Nodes[id]
		switch n.Op {
		case OpEmpty:
			out.WriteString(`[^\x{0}-\x{10FFFF}]`)
		case OpEpsilon:
			out.WriteString("(?:)")
		case OpClass:
			out.WriteByte('[')
			for _, r := range n.Ranges {
				fmt.Fprintf(&out, `\x{%X}`, r.Lo)
				if r.Hi != r.Lo {
					fmt.Fprintf(&out, `-\x{%X}`, r.Hi)
				}
			}
			out.WriteByte(']')
		case OpAlternate, OpConcat:
			out.WriteString("(?:")
			emit(n.Children[0])
			if n.Op == OpAlternate {
				out.WriteByte('|')
			}
			emit(n.Children[1])
			out.WriteByte(')')
		case OpRepeat:
			writeChild := func() { out.WriteString("(?:"); emit(n.Children[0]); out.WriteByte(')') }
			if !expand {
				writeChild()
				out.WriteByte('{')
				out.WriteString(n.Min)
				if n.Unbounded || n.Min != n.Max {
					out.WriteByte(',')
					out.WriteString(n.Max)
				}
				out.WriteByte('}')
				break
			}
			lo, hi := p.bounds[id].lower, p.bounds[id].upper
			for range lo {
				writeChild()
			}
			if n.Unbounded {
				writeChild()
				out.WriteByte('*')
			} else {
				for i := lo; i < hi; i++ {
					writeChild()
					out.WriteByte('?')
				}
			}
		}
	}
	emit(p.data.Root)
	out.WriteString(")$")
	return out.String(), size.instructions + 4, nil
}
func (p *Program) compileNative(source string) (*regexp.Regexp, error) {
	re, err := regexp.Compile(source)
	if err != nil {
		return nil, fmt.Errorf("native pattern: %w", err)
	}
	return re, nil
}
func nativeInstructionCount(source string) (uint64, error) {
	parsed, err := syntax.Parse(source, syntax.Perl)
	if err != nil {
		return 0, err
	}
	prog, err := syntax.Compile(parsed.Simplify())
	if err != nil {
		return 0, err
	}
	return uint64(len(prog.Inst)), nil
}
func (p *Program) prepareNative(bytes, work uint64) {
	if bytes == 0 || work == 0 {
		return
	}
	l := MatchLimits{}.defaults()
	l.Work = min(l.Work, work)
	l.Bytes = min(l.Bytes, bytes)
	l.NativeSourceBytes = min(l.NativeSourceBytes, work)
	source, _, err := p.nativeExpression(false, l)
	if err != nil {
		return
	}
	instructions, err := nativeInstructionCount(source)
	if err != nil || instructions > l.NativeInstructions {
		return
	}
	re, err := p.compileNative(source)
	if err != nil {
		return
	}
	p.native, p.nativeSource, p.nativeInstructions = re, source, instructions
}
func nativeEligible(sourceBytes, instructions, runes uint64, l MatchLimits) error {
	l = l.defaults()
	if err := limitError("match", "native", "bytes", l.Bytes, addSat(mulSat(instructions, 128), mulSat(sourceBytes, 4))); err != nil {
		return err
	}
	for _, v := range []struct {
		name          string
		limit, actual uint64
	}{{"source bytes", l.NativeSourceBytes, sourceBytes}, {"instructions", l.NativeInstructions, instructions}, {"instruction*rune work", l.NativeWork, mulSat(instructions, addSat(runes, 1))}} {
		if err := limitError("match", "native", v.name, v.limit, v.actual); err != nil {
			return err
		}
	}
	return nil
}
