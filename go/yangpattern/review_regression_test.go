// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

package yangpattern

import (
	"errors"
	"regexp"
	"strings"
	"testing"
)

func TestRawDashGrammar(t *testing.T) {
	for _, source := range []string{`[--a]`, `[^--a]`, `[a--b]`, `[---]`, `[a-z--.]`, `[z-a]`, `[a-\d]`, `[\d-a]`, `[a-z-\d-a]`} {
		for _, suffix := range []string{"", "{0002}"} {
			_, err := Compile(source + suffix)
			if _, ok := errors.AsType[*ConfigurationError](err); !ok {
				t.Errorf("Compile(%q) error=%v, want ConfigurationError", source+suffix, err)
			}
		}
	}
	for _, tc := range []struct {
		source         string
		accept, reject []string
	}{
		{`[-ac]`, []string{"-", "a", "c"}, []string{"b", "Z"}},
		{`[ac-]`, []string{"-", "a", "c"}, []string{"b", "Z"}},
		{`[a-b\-c]`, []string{"-", "a", "b", "c"}, []string{"d", "Z"}},
		{`[\--a]`, []string{"-", "Z", "a"}, []string{"+", "b"}},
		{`[--]`, []string{"-"}, []string{"a"}},
		{`[-ac-[a]]`, []string{"-", "c"}, []string{"a", "b"}},
		{`[ac--[a]]`, []string{"-", "c"}, []string{"a", "b"}},
		{`[\--a-[A-Z]]`, []string{"-", "a"}, []string{"A", "Z"}},
		{`[a-[:digit:]]`, []string{"a"}, []string{"d", "-"}},
		{`[a-[:alnum:]]`, nil, []string{"a", ":"}},
		{`[a-z-[a-m-[aeiou]]]`, []string{"a", "n"}, []string{"b", "-"}},
	} {
		p, err := Compile(tc.source)
		if err != nil {
			t.Errorf("Compile(%q): %v", tc.source, err)
			continue
		}
		for _, value := range tc.accept {
			ok, err := p.MatchString(value)
			if err != nil || !ok {
				t.Errorf("%s rejects %q: %v,%v", tc.source, value, ok, err)
			}
		}
		for _, value := range tc.reject {
			ok, err := p.MatchString(value)
			if err != nil || ok {
				t.Errorf("%s accepts %q: %v,%v", tc.source, value, ok, err)
			}
		}
	}
}

// The post-range dash is Cambium compatibility syntax: it is a standalone
// literal, not a second range operator or a raw range endpoint.
func TestPostRangeDashCompatibility(t *testing.T) {
	for _, tc := range []struct {
		source         string
		accept, reject []string
	}{
		{`[a-b-c]`, []string{"a", "b", "c", "-"}, []string{"", "d", "Z", ".", "0", "ab"}},
		{`[^a-b-c]`, []string{"d", "Z", ".", "0", "Ω"}, []string{"a", "b", "c", "-", "ab"}},
		{`[a-z0-9-\.]+`, []string{"example.com", "host-1.example", "-", ".", "0"}, []string{"", "A", "bad_value", "Ω"}},
		{`[a-b-c]{0002}`, []string{"--", "ac", "b-"}, []string{"", "a", "aaa", "aZ"}},
		{`[^a-b-c]{0002}`, []string{"ΩZ", ".."}, []string{"aZ", "-Z", "Z", "ZZZ"}},
		{`[a-z-\d]`, []string{"-", "a", "z", "0", "١"}, []string{"A", ".", "_"}},
		{`[a-b-c-e]`, []string{"a", "b", "-", "c", "d", "e"}, []string{"f", ".", "Z"}},
		{`[a-z-[aeiou]]`, []string{"b", "n", "z"}, []string{"-", "a", "e", "Ω"}},
		{`[a-z--[aeiou]]`, []string{"-", "b", "n", "z"}, []string{"a", "e", "Ω"}},
		{`[a-b-c-[b]]`, []string{"a", "c", "-"}, []string{"b", "d", "Z"}},
		{`[^a-b-c-[Z]]`, []string{"d", ".", "Ω"}, []string{"a", "b", "c", "-", "Z"}},
		{`[a-b-c-[b]]{2}`, []string{"a-", "cc"}, []string{"ab", "-b", "c"}},
		{`[a-z-\d-[a-m-[aeiou]]]`, []string{"-", "0", "١", "a", "n"}, []string{"b", "m", "Z"}},
		{`[a-z-[:digit:]]`, []string{"a", "b", "z"}, []string{":", "d", "i", "g", "t", "-", "0"}},
	} {
		t.Run(tc.source, func(t *testing.T) {
			compiled, err := Compile(tc.source)
			if err != nil {
				t.Fatal(err)
			}
			restored, err := Restore(compiled.Export())
			if err != nil {
				t.Fatal(err)
			}
			for _, p := range []*Program{compiled, restored} {
				native, err := p.GoRegexp()
				if err != nil {
					t.Fatal(err)
				}
				r, err := regexp.Compile(native)
				if err != nil {
					t.Fatal(err)
				}
				for _, group := range []struct {
					values []string
					want   bool
				}{{tc.accept, true}, {tc.reject, false}} {
					for _, value := range group.values {
						got, err := p.MatchString(value)
						if err != nil || got != group.want {
							t.Errorf("MatchString(%q)=%v,%v want %v", value, got, err, group.want)
						}
						if got := r.MatchString(value); got != group.want {
							t.Errorf("GoRegexp match(%q)=%v want %v", value, got, group.want)
						}
					}
				}
			}
		})
	}
}

func TestPostRangeDashDispatch(t *testing.T) {
	for _, tc := range []struct {
		source, accept, reject, route string
		limits                        MatchLimits
	}{
		{`[a-b-c]+`, "a-bc", "a-Z", "direct", MatchLimits{Work: 1, NativeWork: 1}},
		{`[a-b-c]x`, "-x", "Zx", "native", MatchLimits{}},
		{`[a-b-c]x`, "-x", "Zx", "derivative", MatchLimits{NativeWork: 1}},
		{`([a-b-c]x){1001}`, strings.Repeat("-x", 1001), strings.Repeat("-x", 1000) + "Zx", "fallback", MatchLimits{Nodes: 2}},
	} {
		t.Run(tc.route, func(t *testing.T) {
			compiled, err := Compile(tc.source)
			if err != nil {
				t.Fatal(err)
			}
			restored, err := Restore(compiled.Export())
			if err != nil {
				t.Fatal(err)
			}
			for _, p := range []*Program{compiled, restored} {
				for _, value := range []string{tc.accept, tc.reject} {
					want := value == tc.accept
					got, route, err := p.match(value, tc.limits)
					if err != nil || got != want || route != tc.route {
						t.Errorf("match=%v,%s,%v want %v/%s", got, route, err, want, tc.route)
					}
				}
			}
		})
	}
}

func TestPOSIXCompatibilityClasses(t *testing.T) {
	names := []string{"alnum", "alpha", "ascii", "blank", "cntrl", "digit", "graph", "lower", "print", "punct", "space", "upper", "word", "xdigit"}
	for _, name := range names {
		for _, negation := range []string{"", "^"} {
			source := "[[:" + negation + name + ":]]"
			p, err := Compile(source)
			if err != nil {
				t.Errorf("Compile(%q): %v", source, err)
				continue
			}
			oracle := regexp.MustCompile("^(?:" + source + ")$")
			for _, r := range append(asciiRunes(), 'Ω', '١', '名', '�') {
				value := string(r)
				got, err := p.MatchString(value)
				if err != nil || got != oracle.MatchString(value) {
					t.Errorf("%s on U+%04X=%v,%v", source, r, got, err)
				}
			}
		}
	}
	for _, tc := range []struct {
		source, value string
		want          bool
	}{
		{`[[:alnum:]_]*`, "abc_123", true}, {`[[:alnum:]_]*`, "abc-", false}, {`[[:alnum:]_]*`, "Ω", false},
		{`[[:alnum:]_]{01001}`, strings.Repeat("a", 1001), true},
		{`[-[:alnum:]]`, "-", true}, {`[-[:alnum:]]`, "Z", true}, {`[-[:alnum:]]`, "_", false},
		{`[[:alnum:]-[[:digit:]]]`, "a", true}, {`[[:alnum:]-[[:digit:]]]`, "1", false},
	} {
		p, err := Compile(tc.source)
		if err != nil {
			t.Errorf("Compile(%q): %v", tc.source, err)
			continue
		}
		got, err := p.MatchString(tc.value)
		if err != nil || got != tc.want {
			t.Errorf("%s on %q=%v,%v want %v", tc.source, tc.value, got, err, tc.want)
		}
	}
	for _, source := range []string{`[[:unknown:]]`, `[[:alnum:]]x]`, `[[:alnum]_]`} {
		_, err := Compile(source)
		if _, ok := errors.AsType[*ConfigurationError](err); !ok {
			t.Errorf("Compile(%q) error=%v, want ConfigurationError", source, err)
		}
	}
}
func asciiRunes() []rune {
	out := make([]rune, 128)
	for i := range out {
		out[i] = rune(i)
	}
	return out
}

func TestLongDecimalConstructorPolicy(t *testing.T) {
	large := strings.Repeat("9", 10000)
	for _, source := range []string{"a{0," + large + "}", "(ab|ba){0," + large + "}", "a{" + large + ",}"} {
		p, err := Compile(source)
		if err != nil {
			t.Fatal(err)
		}
		// Bound validation and numeric-cache construction are independent scans.
		// A policy funding only the original validation scan must not fund both.
		digits := uint64(0)
		for _, n := range p.Export().Nodes {
			digits += uint64(len(n.Min) + len(n.Max))
		}
		for _, work := range []uint64{digits + 100, 2*digits + 100} {
			_, err = RestoreWithLimits(p.Export(), RestoreLimits{Work: work})
			if e, ok := errors.AsType[*ResourceError](err); !ok || e.Phase != "restore" || e.Limit != "work" || e.Observed <= e.Allowance {
				t.Errorf("RestoreWithLimits(%s...,Work:%d)=%v; want charged restore work error", source[:min(len(source), 12)], work, err)
			}
		}
		_, err = CompileWithLimits(source, CompileLimits{Work: 50000})
		checkPhase(t, err, "compile")
	}
}

func TestLongDecimalMatchPolicies(t *testing.T) {
	upper := strings.Repeat("9", 10000)
	for _, tc := range []struct {
		source, value string
		limits        MatchLimits
		route         string
		want          bool
	}{
		{"a{0," + upper + "}", strings.Repeat("a", 100000), MatchLimits{Work: 1, NativeWork: 1}, "direct", true},
		{"(ab|ba){0," + upper + "}", "abbaab", MatchLimits{Work: 1000, NativeWork: 1}, "derivative", true},
		{"(ab|ba){0," + upper + "}", "abbaab", MatchLimits{Work: 1000, Nodes: 2}, "fallback", true},
		{"a{0," + upper + "}", "b", MatchLimits{Work: 1, NativeWork: 1}, "direct", false},
		{"a{" + upper + ",}", "a", MatchLimits{Work: 1, NativeWork: 1}, "length", false},
		{"(ab|ba){" + upper + ",}", "abbaab", MatchLimits{Work: 1, NativeWork: 1}, "length", false},
	} {
		compiled, err := Compile(tc.source)
		if err != nil {
			t.Fatal(err)
		}
		restored, err := RestoreWithLimits(compiled.Export(), RestoreLimits{Work: 50000})
		if err != nil {
			t.Fatal(err)
		}
		for _, p := range []*Program{compiled, restored} {
			got, route, err := p.match(tc.value, tc.limits)
			if err != nil || got != tc.want || route != tc.route {
				t.Errorf("match route=%s result=%v error=%v; want %v/%s", route, got, err, tc.want, tc.route)
			}
			if tc.route == "derivative" || tc.route == "fallback" {
				_, err = p.MatchStringWithLimits(tc.value, MatchLimits{Work: 1, NativeWork: 1})
				checkPhase(t, err, "match")
			}
		}
	}
	// A candidate-independent cache must not be part of portable metadata: the
	// restored program must derive it from the actual supplied canonical bounds.
	p, err := Compile("a{0," + upper + "}")
	if err != nil {
		t.Fatal(err)
	}
	data := p.Export()
	data.Nodes[data.Root].Max = "1"
	q, err := Restore(data)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := q.MatchStringWithLimits("aa", MatchLimits{Work: 1, NativeWork: 1}); err != nil || got {
		t.Errorf("restored changed upper bound match=%v,%v", got, err)
	}
}
