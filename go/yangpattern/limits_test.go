// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs
package yangpattern

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestConstructorDepthAndAccounting(t *testing.T) {
	_, err := CompileWithLimits(strings.Repeat("(", 20)+"a"+strings.Repeat(")", 20), CompileLimits{Depth: 10})
	checkPhase(t, err, "compile")
	for _, limits := range []CompileLimits{{Nodes: 2}, {Bytes: 1}, {Work: 1}, {Digits: 1}, {Ranges: 1}, {Edges: 1}} {
		_, err := CompileWithLimits(`([a-zA-Z]{12}|bc)`, limits)
		checkPhase(t, err, "compile")
	}
	p, err := Compile(`(ab|ba){3}`)
	if err != nil {
		t.Fatal(err)
	}
	for _, limits := range []RestoreLimits{{Bytes: 1}, {Work: 1}, {Depth: 1}, {Edges: 1}, {Digits: 1}, {Ranges: 1}} {
		_, err := RestoreWithLimits(p.Export(), limits)
		checkPhase(t, err, "restore")
	}
	for _, limits := range []MatchLimits{{Bytes: 1}, {Work: 1, NativeInstructions: 1}, {Transitions: 1, NativeInstructions: 1}, {NativeSourceBytes: 1, Nodes: 2}} {
		_, err := p.MatchStringWithLimits("abbaab", limits)
		checkPhase(t, err, "match")
	}
}
func TestDeepNullableAndArbitraryProducts(t *testing.T) {
	source := "a"
	for range 100 {
		source = "(" + source + ")?"
	}
	source = "(" + source + "){999999999999999999999999999999}"
	p, err := Compile(source)
	if err != nil {
		t.Fatal(err)
	}
	if ok, err := p.MatchString(strings.Repeat("a", 4000)); err != nil || !ok {
		t.Fatalf("nested nullable = %v,%v", ok, err)
	}
	p, err = Compile(`((a{99999999999999999999}){99999999999999999999}){99999999999999999999}`)
	if err != nil {
		t.Fatal(err)
	}
	if ok, err := p.MatchString("a"); err != nil || ok {
		t.Fatalf("huge product = %v,%v", ok, err)
	}
	d := p.Export()
	n := d.Nodes[d.Root]
	if n.Min != "999999999999999999970000000000000000000299999999999999999999" {
		t.Fatalf("exact product %s", n.Min)
	}
}
func TestMalformedPortableAllNodes(t *testing.T) {
	base := ProgramData{Version: 1, Root: 0, Nodes: []NodeData{{Op: OpEpsilon}}}
	invalid := []NodeData{
		{Op: OpClass, Ranges: []RuneRange{{'z', 'a'}}},
		{Op: OpClass, Ranges: []RuneRange{{0xD800, 0xDFFF}}},
		{Op: OpClass, Ranges: []RuneRange{{'a', 'c'}, {'b', 'd'}}},
		{Op: OpClass, Ranges: []RuneRange{{-1, 2}}},
		{Op: OpClass, Ranges: []RuneRange{{1, 0x110000}}},
		{Op: OpClass},
		{Op: OpRepeat, Children: []uint32{0}, Min: "0", Max: "1"},
		{Op: OpRepeat, Children: []uint32{1}, Min: "0", Max: "1"},
		{Op: OpRepeat, Children: []uint32{4}, Min: "0", Max: "1"},
		{Op: OpEpsilon, Min: "0"},
		{Op: OpAlternate, Children: []uint32{0}},
	}
	for _, n := range invalid {
		d := base
		d.Nodes = append(d.Nodes, n)
		if _, err := Restore(d); err == nil {
			t.Errorf("accepted unreachable malformed node %+v", n)
		} else if _, ok := errors.AsType[*ConfigurationError](err); !ok {
			t.Fatalf("wrong error %v", err)
		}
	}
}
func TestStrictXSDGrammar(t *testing.T) {
	for _, source := range []string{`a{`, `a{1`, `a{1,`, `a{,2}`, `a{b}`, `}`, `]`, `[a-[b]c]`, `a{1,2,3}`} {
		if _, err := Compile(source); err == nil {
			t.Errorf("accepted %q", source)
		}
	}
}

func TestAlphabetPartitionAndPerCallArena(t *testing.T) {
	p, err := Compile(`(.*b*){1001}`)
	if err != nil {
		t.Fatal(err)
	}
	before := p.Export()
	for iteration := range 4 {
		a, state := p.specialize(100000, MatchLimits{}.defaults())
		for i := range 100000 {
			r := rune(0x1000 + i + iteration)
			if r >= 0xD800 {
				r += 0x800
			}
			state = a.derive(state, a.symbol(r))
			if a.b.err != nil {
				t.Fatal(a.b.err)
			}
		}
		if !a.nodes[state].nullable {
			t.Fatal("stream rejected")
		}
		if len(a.transitions) > 100 || len(a.nodes) > 100 {
			t.Fatalf("per-rune cache instead of alphabet partition: nodes=%d transitions=%d", len(a.nodes), len(a.transitions))
		}
	}
	if !reflect.DeepEqual(before, p.Export()) {
		t.Fatal("match state escaped into program")
	}
}

func TestSaturatedAccountingCannotAuthorizeOverflow(t *testing.T) {
	unlimited := ^uint64(0)
	err := nativeEligible(1, unlimited, unlimited, MatchLimits{Bytes: unlimited, NativeInstructions: unlimited, NativeSourceBytes: unlimited, NativeWork: unlimited})
	checkPhase(t, err, "match")
	b := ledger{phase: "compile", stage: "test", byteLimit: unlimited, workLimit: unlimited, bytes: unlimited - 1}
	if b.charge(4, 1) {
		t.Fatal("saturated storage ledger authorized allocation")
	}
	checkPhase(t, b.err, "compile")
}
