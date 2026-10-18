// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs
package yangpattern

import (
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"
)

func TestCountsAndXSDAtoms(t *testing.T) {
	for _, tc := range []struct {
		source, value string
		want          bool
	}{
		{`a{0001}`, "a", true}, {`a{0001}`, "a{0001}", false}, {`a{0000}`, "", true}, {`a{00,0000}`, "", true},
		{`a{01,01024}`, strings.Repeat("a", 1024), true}, {`a{01001}`, strings.Repeat("a", 1001), true},
		{`a{0001,}`, "aaa", true}, {`(a{0002}){0002}`, "aaaa", true},
		{`a{999999999999999999999999999999999999}`, "a", false},
		{`(a?){999999999999999999999999999999999999}`, "aaa", true},
		{`(){999999999999999999999999999999999999}`, "", true},
		{`[a-[a]]{0}`, "", true}, {`[a-[a]]{1}`, "", false},
		{`(a|aa){1001}`, strings.Repeat("a", 1500), true}, {`(a|aa){1001}`, strings.Repeat("a", 2003), false},
		{`((a|bc){20}){51}`, strings.Repeat("a", 1020), true},
		{`((a|ab){0,1001}){1001}`, strings.Repeat("ab", 1000), true},
		{`(a{2,3}){2,3}`, strings.Repeat("a", 7), true}, {`(a{3}){2,3}`, strings.Repeat("a", 7), false},
		{`\{01001\}`, "{01001}", true}, {`\p{L}{0001}`, "Ω", true},
		{`\p{IsGreek}+`, "Ωα", true}, {`\i\c*`, "名-1", true}, {`\i\c*`, "1name", false},
		{`\d`, "١", true}, {`\s`, "\f", false}, {`\w`, "$", true}, {`\W`, "-", true},
		{`[a-z-[a-m-[aeiou]]]+`, "aeiounz", true}, {`[a-z-[a-m-[aeiou]]]+`, "b", false},
		{`^foo$`, "^foo$", true}, {`^foo$`, "foo", false}, {`.`, "\r", false}, {`.`, "\n", false}, {`.`, "Ω", true},
		{`a|`, "", true}, {``, "", true}, {`a`, "a\n", false},
	} {
		t.Run(tc.source+"/"+string(rune(len(tc.value))), func(t *testing.T) {
			p, err := Compile(tc.source)
			if err != nil {
				t.Fatal(err)
			}
			got, err := p.MatchString(tc.value)
			if err != nil || got != tc.want {
				t.Fatalf("match(%q,len=%d)=%v,%v want %v", tc.source, len(tc.value), got, err, tc.want)
			}
			q, err := Restore(p.Export())
			if err != nil {
				t.Fatal(err)
			}
			got, err = q.MatchString(tc.value)
			if err != nil || got != tc.want {
				t.Fatalf("restored match=%v,%v", got, err)
			}
		})
	}
}

func TestInvalidSyntax(t *testing.T) {
	for _, s := range []string{`[`, `(`, `)`, `a**`, `*a`, `a{2,1}`, `\q`, `\p{NoSuch}`, `[z-a]`, `[]`, `a{1,2}?`, `(?=a)`} {
		if _, err := Compile(s); err == nil {
			t.Errorf("Compile(%q) succeeded", s)
		}
	}
}

func TestPortableValidationAndIsolation(t *testing.T) {
	p, err := Compile(`[a-z]{0002,0003}`)
	if err != nil {
		t.Fatal(err)
	}
	original := p.Export()
	q, err := Restore(original)
	if err != nil {
		t.Fatal(err)
	}
	for i := range original.Nodes {
		original.Nodes[i].Ranges = nil
		original.Nodes[i].Children = nil
		original.Nodes[i].Min = "999"
	}
	if ok, err := q.MatchString("ab"); err != nil || !ok {
		t.Fatalf("copy changed: %v,%v", ok, err)
	}
	if !reflect.DeepEqual(p.Export(), q.Export()) {
		t.Fatal("round trip changed metadata")
	}
	for _, change := range []func(*ProgramData){
		func(d *ProgramData) { d.Version++ }, func(d *ProgramData) { d.Root = 999999 },
		func(d *ProgramData) { d.Nodes[d.Root].Children = []uint32{d.Root} },
		func(d *ProgramData) { d.Nodes[d.Root].Min = "01" },
		func(d *ProgramData) { d.Nodes[d.Root].Max = "1" },
		func(d *ProgramData) { d.Nodes[0].Op = Op(255) },
	} {
		d := p.Export()
		change(&d)
		if _, err := Restore(d); err == nil {
			t.Errorf("accepted malformed program: %+v", d)
		}
	}
}

func TestPhaseLimits(t *testing.T) {
	_, err := CompileWithLimits(`(ab|ba){5}`, CompileLimits{SourceBytes: 1})
	checkPhase(t, err, "compile")
	p, err := Compile(`(ab|ba){5}`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = RestoreWithLimits(p.Export(), RestoreLimits{Nodes: 1})
	checkPhase(t, err, "restore")
	_, err = p.MatchStringWithLimits("ababababab", MatchLimits{Nodes: 1, NativeInstructions: 1})
	checkPhase(t, err, "match")
}
func checkPhase(t *testing.T, err error, phase string) {
	t.Helper()
	e, ok := errors.AsType[*ResourceError](err)
	if !ok || e.Phase != phase || e.Observed <= e.Allowance {
		t.Fatalf("want %s resource error, got %#v", phase, err)
	}
}

func TestConcurrentImmutable(t *testing.T) {
	p, err := Compile(`(a|aa){1001}`)
	if err != nil {
		t.Fatal(err)
	}
	before := p.Export()
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			for range 5 {
				ok, err := p.MatchString(strings.Repeat("a", 1500))
				if err != nil || !ok {
					t.Errorf("match %v,%v", ok, err)
				}
			}
		})
	}
	wg.Wait()
	if !reflect.DeepEqual(before, p.Export()) {
		t.Fatal("matching mutated metadata")
	}
}

func TestUnassignedAndSurrogateCategories(t *testing.T) {
	for _, source := range []string{`\p{Cn}`, `\p{C}`, `[\p{Cn}]`} {
		p, err := Compile(source)
		if err != nil {
			t.Fatalf("%s: %v", source, err)
		}
		ok, err := p.MatchString("\u0378")
		if err != nil || !ok {
			t.Fatalf("%s does not match unassigned scalar: %v,%v", source, ok, err)
		}
	}
	p, err := Compile(`\p{Cs}`)
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range []string{"", "a", "�"} {
		if ok, err := p.MatchString(v); err != nil || ok {
			t.Fatalf("surrogate category matched %q: %v,%v", v, ok, err)
		}
	}
}
