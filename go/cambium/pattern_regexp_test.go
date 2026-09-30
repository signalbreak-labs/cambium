// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

package cambium_test

import (
	"regexp"
	"testing"

	"github.com/signalbreak-labs/cambium/go/cambium"
)

func TestPatternGoRegexpPreservesXSDSemantics(t *testing.T) {
	for _, tc := range []struct {
		name, expression string
		matches, rejects []string
	}{
		{"literal anchors", `^foo$`, []string{"^foo$"}, []string{"foo", "x^foo$", "^foo$\n"}},
		{"alternation", `foo|bar`, []string{"foo", "bar"}, []string{"foobar", "xfoo", "barx"}},
		{"unicode digits", `\d+`, []string{"123", "١"}, []string{"", "a", "١a"}},
		{"xml names", `\i\c*`, []string{"name", "名", "a-1"}, []string{"1name", "a b"}},
		{"unicode block", `\p{IsGreek}+`, []string{"Ω", "αβ"}, []string{"abc", "Ω1"}},
		{"class subtraction", `[a-z-[aeiou]]+`, []string{"bcdf", "xyz"}, []string{"a", "boy"}},
		{"dot", `.`, []string{"a", "\t", "Ω"}, []string{"", "ab", "\r", "\n"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mod := s3Module(t, `module pattern-regexp {
  yang-version 1.1; namespace "urn:pattern-regexp"; prefix p;
  leaf value { type string { pattern '`+tc.expression+`'; } }
}`)
			pattern := leafType(t, mod, "/p:value").Resolved().(cambium.ResolvedString).Patterns[0]
			expression, err := pattern.GoRegexp()
			if err != nil {
				t.Fatal(err)
			}
			compiled, err := regexp.Compile(expression)
			if err != nil {
				t.Fatalf("returned expression %q does not compile: %v", expression, err)
			}
			for _, value := range tc.matches {
				if !compiled.MatchString(value) {
					t.Errorf("translated %q does not match %q", tc.expression, value)
				}
			}
			for _, value := range tc.rejects {
				if compiled.MatchString(value) {
					t.Errorf("translated %q unexpectedly matches %q", tc.expression, value)
				}
			}
			if pattern.Regex() != tc.expression || pattern.IsInverted() {
				t.Errorf("translation changed pattern metadata: %+v", pattern)
			}
		})
	}
}

func TestPatternGoRegexpKeepsInversionSeparate(t *testing.T) {
	mod := s3Module(t, `module pattern-regexp {
  yang-version 1.1; namespace "urn:pattern-regexp"; prefix p;
  typedef filtered { type string { pattern 'denied' { modifier invert-match; } } }
  leaf value { type union { type filtered; type uint8; } }
}`)
	members := leafType(t, mod, "/p:value").Resolved().(cambium.ResolvedUnion).Members()
	pattern := members[0].Resolved().(cambium.ResolvedString).Patterns[0]
	expression, err := pattern.GoRegexp()
	if err != nil {
		t.Fatal(err)
	}
	compiled := regexp.MustCompile(expression)
	if !pattern.IsInverted() || !compiled.MatchString("denied") || compiled.MatchString("allowed") {
		t.Errorf("inversion must remain separate: expression=%q inverted=%v", expression, pattern.IsInverted())
	}
}

func TestPatternGoRegexpRejectsUnsupportedNativeSyntax(t *testing.T) {
	mod := s3Module(t, `module pattern-regexp {
  yang-version 1.1; namespace "urn:pattern-regexp"; prefix p;
  leaf value { type string { pattern 'a{1001}'; } }
}`)
	pattern := leafType(t, mod, "/p:value").Resolved().(cambium.ResolvedString).Patterns[0]
	if expression, err := pattern.GoRegexp(); err == nil || expression != "" {
		t.Errorf("GoRegexp = %q, %v; want empty expression and unsupported native syntax error", expression, err)
	}
	if pattern.Regex() != "a{1001}" {
		t.Errorf("raw pattern = %q, want a{1001}", pattern.Regex())
	}
}
