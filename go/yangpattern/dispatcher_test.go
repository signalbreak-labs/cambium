// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs
package yangpattern

import (
	"errors"
	"fmt"
	"math/rand/v2"
	"regexp"
	"runtime"
	"strings"
	"testing"
	"time"
)

func deterministicAB(n int) string {
	b := make([]byte, n)
	x := uint64(0x9e3779b97f4a7c15)
	for i := range b {
		x ^= x << 13
		x ^= x >> 7
		x ^= x << 17
		b[i] = 'a' + byte(x&1)
	}
	return string(b)
}
func TestDispatchPolicies(t *testing.T) {
	p, err := Compile(`(ab|ba){3}`)
	if err != nil {
		t.Fatal(err)
	}
	value := "abbaab"
	if got, route, err := p.match(value, MatchLimits{}); err != nil || !got || route != "native" {
		t.Fatalf("warm route: %v,%s,%v", got, route, err)
	}
	if got, route, err := p.match(value, MatchLimits{NativeWork: 1}); err != nil || !got || route != "derivative" {
		t.Fatalf("strict cached route: %v,%s,%v", got, route, err)
	}
	_, route, err := p.match(value, MatchLimits{Nodes: 2, NativeWork: 1})
	checkPhase(t, err, "match")
	if route == "native" {
		t.Fatal("cache bypassed policy")
	}
	p, err = Compile(`[a-z]{100000}`)
	if err != nil {
		t.Fatal(err)
	}
	if got, route, err := p.match(strings.Repeat("a", 100000), MatchLimits{NativeWork: 1}); err != nil || !got || route != "direct" {
		t.Fatalf("direct route: %v,%s,%v", got, route, err)
	}
	checkPhase(t, nativeEligible(1, 2, 0, MatchLimits{NativeSourceBytes: 10, NativeInstructions: 10, NativeWork: 1}), "match")
	checkPhase(t, nativeEligible(1, ^uint64(0), ^uint64(0), MatchLimits{NativeSourceBytes: 10, NativeInstructions: ^uint64(0), NativeWork: 1}), "match")
}
func TestDefaultResourceWitness(t *testing.T) {
	for _, count := range []int{1000, 1001} {
		p, err := Compile(fmt.Sprintf(`[ab]*a[ab]{%d}`, count))
		if err != nil {
			t.Fatal(err)
		}
		value := deterministicAB(1000000)
		if p.native != nil && nativeEligible(uint64(len(p.nativeSource)), p.nativeInstructions, 1000000, MatchLimits{}.defaults()) == nil {
			t.Fatal("ordinary native path eligible above work allowance")
		}
		_, route, err := p.match(value, MatchLimits{})
		checkPhase(t, err, "match")
		if route == "native" {
			t.Fatal("ordinary native limit bypass")
		}
		t.Logf("count=%d route=%s error=%v", count, route, err)
	}
}
func TestAcceptanceMeasurements(t *testing.T) {
	if testing.Short() {
		t.Skip("engineering measurements")
	}
	nested := "a"
	for range 100 {
		nested = "(" + nested + ")?"
	}
	nested = "(" + nested + "){1001}"
	cases := []struct {
		name, source, value string
		want                bool
		route               string
	}{
		{"dense100k", `(a*b*){1001}`, strings.Repeat("a", 100000), true, "derivative"},
		{"dense100kNegative", `(a*b*){1001}`, strings.Repeat("a", 100000) + "c", false, "derivative"},
		{"denseAlternating100k", `(a*b*){1001}`, strings.Repeat("ab", 50000), false, "derivative"},
		{"exact1001", `a{1001}`, strings.Repeat("a", 1001), true, "direct"},
		{"range1024", `a{1,1024}`, strings.Repeat("a", 1024), true, "direct"},
		{"hundredNullableGroups", nested, strings.Repeat("a", 1001), true, "direct"},
		{"dense1m", `(a*b*){1001}`, strings.Repeat("a", 1000000), true, "derivative"},
		{"ambiguous1500", `(a|aa){1001}`, strings.Repeat("a", 1500), true, "derivative"},
		{"ambiguous4000", `(a|aa){2001}`, strings.Repeat("a", 4000), true, "derivative"},
		{"ambiguousNegative", `(a|aa){1001}`, strings.Repeat("a", 2003), false, "derivative"},
		{"nested1020", `((a|bc){20}){51}`, strings.Repeat("a", 1020), true, "derivative"},
		{"nullable1001", `(a?){1001}`, strings.Repeat("a", 1001), true, "direct"},
		{"nestedNullable2000", `((a|ab){0,1001}){1001}`, strings.Repeat("ab", 1000), true, "derivative"},
		{"suffix100k", `[ab]*a[ab]{1001}`, deterministicAB(100000), false, "fallback"},
		{"class100000", `[a-z]{100000}`, strings.Repeat("a", 100000), true, "direct"},
		{"hugeShort", `(ab){999999999999999999999999999999999999}`, "ab", false, "length"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p, err := Compile(tc.source)
			if err != nil {
				t.Fatal(err)
			}
			want := tc.want
			if tc.name == "suffix100k" {
				want = tc.value[len(tc.value)-1002] == 'a'
			}
			for repetition := range 5 {
				runtime.GC()
				var before, after runtime.MemStats
				runtime.ReadMemStats(&before)
				start := time.Now()
				got, route, err := p.match(tc.value, MatchLimits{})
				elapsed := time.Since(start)
				runtime.ReadMemStats(&after)
				if err != nil || got != want || route != tc.route {
					t.Fatalf("result=%v route=%s error=%v, want %v/%s", got, route, err, want, tc.route)
				}
				t.Logf("repetition=%d elapsed=%s allocated=%d route=%s", repetition+1, elapsed, after.TotalAlloc-before.TotalAlloc, route)
			}
		})
	}
}

func TestRandomCommonSubsetOracle(t *testing.T) {
	random := rand.New(rand.NewPCG(17, 23))
	var expression func(int) string
	expression = func(depth int) string {
		if depth == 0 {
			return []string{"a", "b", "[ab]", ""}[random.IntN(4)]
		}
		switch random.IntN(5) {
		case 0:
			return "(" + expression(depth-1) + "|" + expression(depth-1) + ")"
		case 1:
			return "(" + expression(depth-1) + expression(depth-1) + ")"
		case 2:
			return "(" + expression(depth-1) + "){0,3}"
		case 3:
			return "(" + expression(depth-1) + "){2}"
		default:
			return "(" + expression(depth-1) + ")*"
		}
	}
	for trial := range 500 {
		source := expression(4)
		oracle, err := regexp.Compile("^(?:" + source + ")$")
		if err != nil {
			t.Fatal(err)
		}
		p, err := Compile(source)
		if err != nil {
			t.Fatalf("trial %d %s: %v", trial, source, err)
		}
		for n := 0; n <= 6; n++ {
			for mask := range 1 << n {
				b := make([]byte, n)
				for i := range b {
					b[i] = 'a' + byte(mask>>i&1)
				}
				value := string(b)
				want := oracle.MatchString(value)
				got, err := p.MatchStringWithLimits(value, MatchLimits{NativeWork: 1})
				if err != nil || got != want {
					t.Fatalf("trial %d %s on %q = %v,%v want %v", trial, source, value, got, err, want)
				}
			}
		}
	}
}

func FuzzCompileAndRestore(f *testing.F) {
	for _, s := range []string{`(a*b*){1001}`, `a{0001}`, `[a-z-[aeiou]]`, `\p{IsGreek}`, `(){99999999999999999999}`, `(`} {
		f.Add(s, "ab")
	}
	f.Fuzz(func(t *testing.T, source, value string) {
		if len(source) > 4096 || len(value) > 4096 {
			t.Skip()
		}
		p, err := Compile(source)
		if err != nil {
			return
		}
		q, err := Restore(p.Export())
		if err != nil {
			t.Fatal(err)
		}
		a, ea := p.MatchString(value)
		b, eb := q.MatchString(value)
		if (ea == nil) != (eb == nil) || a != b {
			t.Fatalf("roundtrip differs: %v,%v vs %v,%v", a, ea, b, eb)
		}
	})
}
func FuzzPortableValidation(f *testing.F) {
	f.Add(uint8(OpRepeat), uint32(0), "0", "2", int32('a'), int32('z'))
	f.Fuzz(func(t *testing.T, op uint8, child uint32, lo, hi string, low, high int32) {
		if len(lo)+len(hi) > 4096 {
			t.Skip()
		}
		d := ProgramData{Version: 1, Root: 1, Nodes: []NodeData{{Op: OpClass, Ranges: []RuneRange{{low, high}}}, {Op: Op(op), Children: []uint32{child}, Min: lo, Max: hi}}}
		p, err := Restore(d)
		if err != nil {
			return
		}
		_, err = p.MatchString("a")
		if err != nil {
			if _, ok := errors.AsType[*ResourceError](err); !ok {
				t.Fatal(err)
			}
		}
	})
}

func TestSpecializationRefusalStillFallsBack(t *testing.T) {
	p, err := Compile(`(ab){1001}`)
	if err != nil {
		t.Fatal(err)
	}
	got, route, err := p.match(strings.Repeat("ab", 1001), MatchLimits{Nodes: 2})
	if err != nil || !got || route != "fallback" {
		t.Fatalf("result=%v route=%s error=%v", got, route, err)
	}
}

func FuzzEvaluatorOracle(f *testing.F) {
	f.Add([]byte{1, 2, 3, 4, 0, 2, 1}, "abba")
	f.Add([]byte{4, 4, 4, 4, 4}, "")
	f.Fuzz(func(t *testing.T, data []byte, value string) {
		if len(data) == 0 || len(data) > 128 || len(value) > 64 {
			t.Skip()
		}
		offset := 0
		next := func() byte { v := data[offset%len(data)]; offset++; return v }
		var build func(int) string
		build = func(depth int) string {
			if depth == 0 {
				return []string{"a", "b", "[ab]", ""}[next()%4]
			}
			switch next() % 5 {
			case 0:
				return "(" + build(depth-1) + "|" + build(depth-1) + ")"
			case 1:
				return "(" + build(depth-1) + build(depth-1) + ")"
			case 2:
				return "(" + build(depth-1) + "){0,3}"
			case 3:
				return "(" + build(depth-1) + "){2}"
			default:
				return "(" + build(depth-1) + ")*"
			}
		}
		source := build(3)
		oracle, err := regexp.Compile("^(?:" + source + ")$")
		if err != nil {
			t.Fatal(err)
		}
		p, err := Compile(source)
		if err != nil {
			t.Fatal(err)
		}
		got, err := p.MatchStringWithLimits(value, MatchLimits{NativeWork: 1})
		if err != nil || got != oracle.MatchString(value) {
			t.Fatalf("%s on %q = %v,%v; want %v", source, value, got, err, oracle.MatchString(value))
		}
	})
}
