// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

package cambium_test

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/signalbreak-labs/cambium/go/cambium"
	"github.com/signalbreak-labs/cambium/go/yangpattern"
)

// Catch raw/native count semantics and ordered union probes hiding failures.
func buildPatternDefaults(t *testing.T, body string) (*cambium.Context, error) {
	t.Helper()
	b, err := cambium.NewContextBuilder(cambium.ContextFlags{DisableSearchdirCwd: true})
	if err != nil {
		t.Fatal(err)
	}
	if err := b.LoadModuleStr(`module pattern-defaults { yang-version 1.1; namespace "urn:pattern-defaults"; prefix p; ` + body + ` }`); err != nil {
		t.Fatalf("LoadModuleStr: %.300s", err)
	}
	ctx, err := b.Build()
	if ctx != nil {
		t.Cleanup(ctx.Close)
	}
	return ctx, err
}
func patternStringType(source string, inverted bool) string {
	modifier := ""
	if inverted {
		modifier = "modifier invert-match;"
	}
	return fmt.Sprintf("type string { pattern '%s' { %s } }", source, modifier)
}
func assertPatternDefaultContextError(t *testing.T, err error) {
	t.Helper()
	op, ok := errors.AsType[*cambium.Error](err)
	if !ok || op.Code != cambium.RuleCodeContext {
		t.Fatalf("context error classification: %.300s", err)
	}
}
func TestSchemaPatternDefaultLanguages(t *testing.T) {
	for _, tc := range []struct {
		name, pattern string
		good, bad     []string
	}{
		{"exact1001", `[a-z]{1001}`, []string{strings.Repeat("a", 1001)}, []string{strings.Repeat("a", 1000), strings.Repeat("a", 1002), strings.Repeat("a", 1000) + "0"}},
		{"range1024", `[a-z]{1,1024}`, []string{"a", strings.Repeat("z", 1024)}, []string{"", strings.Repeat("a", 1025), "0"}},
		{"nestedCounts", `((a|bc){20}){51}`, []string{strings.Repeat("a", 1020), strings.Repeat("bc", 1020)}, []string{strings.Repeat("a", 1019), strings.Repeat("a", 1021), strings.Repeat("a", 1019) + "d"}},
		{"leadingOne", `a{0001}`, []string{"a"}, []string{"", "aa", "a{0001}"}},
		{"leadingZero", `a{0000}`, []string{""}, []string{"a", "a{0000}"}},
		{"leadingZeroRange", `a{00,0000}`, []string{""}, []string{"a", "a{00,0000}"}},
		{"leadingRange", `a{01,01024}`, []string{"a", strings.Repeat("a", 1024)}, []string{"", strings.Repeat("a", 1025)}},
		{"leadingLarge", `a{01001}`, []string{strings.Repeat("a", 1001)}, []string{strings.Repeat("a", 1000), strings.Repeat("a", 1002), "a{01001}"}},
		{"leadingUnbounded", `a{0001,}`, []string{"a", "aaa"}, []string{"", "ab", "a{0001,}"}},
		{"nestedLeading", `(a{0002}){00501}`, []string{strings.Repeat("a", 1002)}, []string{strings.Repeat("a", 1001), strings.Repeat("a", 1003)}},
		{"escapedBraces", `a\{0001\}`, []string{"a{0001}"}, []string{"a", "aa"}},
		{"unicodeCategory", `\p{L}{01001}`, []string{strings.Repeat("λ", 1001)}, []string{strings.Repeat("λ", 1000), strings.Repeat("λ", 1000) + "1"}},
		{"unicodeSubtraction", `[\p{L}-[a-z]]{0002}`, []string{"λΩ", "漢字"}, []string{"λa", "λ", "λΩπ"}},
		{"nullable", `(a?){01001}`, []string{"", "a", strings.Repeat("a", 1001)}, []string{strings.Repeat("a", 1002), "b"}},
		{"zeroEmptyLanguage", `([a-[a]]){0}`, []string{""}, []string{"a", "b"}},
		{"emptyLanguage", `[a-[a]]`, nil, []string{"", "a", "b"}},
		{"emptyGroup", `(){1001}`, []string{""}, []string{"a"}},
		{"literalAnchors", `^a$`, []string{"^a$"}, []string{"a", "x^a$", "^a$x"}},
		{"postRangeDash", `[a-b-c]{0002}`, []string{"--", "ac", "bb"}, []string{"ad", "a", "---"}},
		{"posix", `[[:digit:]]{0002}`, []string{"00", "19"}, []string{"1", "123", "١٢", "aa"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, inverted := range []bool{false, true} {
				for _, group := range []struct {
					values  []string
					matches bool
				}{{tc.good, true}, {tc.bad, false}} {
					for i, value := range group.values {
						t.Run(fmt.Sprintf("invert=%v/match=%v/%d", inverted, group.matches, i), func(t *testing.T) {
							ctx, err := buildPatternDefaults(t, fmt.Sprintf("leaf value { %s default '%s'; }", patternStringType(tc.pattern, inverted), value))
							if group.matches != inverted {
								if err != nil {
									t.Fatalf("valid default rejected: %.300s", err)
								}
								mod, err := ctx.Schema("pattern-defaults")
								if err != nil {
									t.Fatal(err)
								}
								node := schemaNodeAt(t, mod, "/p:value")
								if got, ok := node.DefaultValue(); !ok || got != value {
									t.Fatalf("default changed: present=%v length=%d want=%d", ok, len(got), len(value))
								}
								info, ok := node.LeafType()
								if !ok {
									t.Fatal("missing leaf type")
								}
								resolved := info.Resolved().(cambium.ResolvedString)
								if resolved.Patterns[0].Regex() != tc.pattern || resolved.Patterns[0].IsInverted() != inverted {
									t.Fatal("pattern source/inversion changed")
								}
							} else {
								if err == nil {
									t.Fatal("invalid default accepted")
								}
								assertPatternDefaultContextError(t, err)
								if _, ok := errors.AsType[*yangpattern.ResourceError](err); ok {
									t.Fatal("ordinary mismatch became resource error")
								}
								if !strings.Contains(err.Error(), "does not satisfy pattern") {
									t.Fatalf("mismatch diagnostic: %.300s", err)
								}
							}
						})
					}
				}
			}
		})
	}
}
func TestSchemaPatternDefaultConsumers(t *testing.T) {
	for _, tc := range []struct {
		name string
		body func(string) string
	}{
		{"typedefIntersection", func(v string) string {
			return fmt.Sprintf("typedef base { %s default '%s'; } typedef derived { type base { pattern '(a?){01001}'; } } leaf value { type derived; }", patternStringType(`[a-z]{1,1024}`, false), v)
		}},
		{"unusedTypedef", func(v string) string {
			return fmt.Sprintf("typedef unused { %s default '%s'; }", patternStringType(`(a?){01001}`, false), v)
		}},
		{"leafList", func(v string) string {
			return fmt.Sprintf("leaf-list value { %s default ''; default '%s'; }", patternStringType(`(a?){01001}`, false), v)
		}},
		{"leafref", func(v string) string {
			return fmt.Sprintf("leaf target { %s } leaf value { type leafref { path '/p:target'; } default '%s'; }", patternStringType(`(a?){01001}`, false), v)
		}},
		{"leafrefChain", func(v string) string {
			return fmt.Sprintf("leaf target { %s } leaf middle { type leafref { path '/p:target'; } } leaf value { type leafref { path '/p:middle'; } default '%s'; }", patternStringType(`(a?){01001}`, false), v)
		}},
		{"nestedUnion", func(v string) string {
			return fmt.Sprintf("leaf value { type union { type uint8; type union { %s type boolean; } } default '%s'; }", patternStringType(`(a?){01001}`, false), v)
		}},
		{"invertedUnion", func(v string) string {
			return fmt.Sprintf("leaf value { type union { %s type boolean; } default '%s'; }", patternStringType(`b+`, true), v)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, valid := range []bool{true, false} {
				value := strings.Repeat("a", 1001)
				if !valid {
					value += "a"
					if tc.name == "invertedUnion" {
						value = "bbb"
					}
				}
				_, err := buildPatternDefaults(t, tc.body(value))
				if (err == nil) != valid {
					t.Fatalf("accepted=%v want=%v: %.300s", err == nil, valid, err)
				}
				if err != nil {
					assertPatternDefaultContextError(t, err)
				}
			}
		})
	}
}
func schemaDefaultDeterministicAB(n int) string {
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
func TestSchemaPatternDefaultOperationalErrors(t *testing.T) {
	value := schemaDefaultDeterministicAB(1000000)
	for _, tc := range []struct {
		name string
		body func(string) string
	}{
		{"native1000", func(v string) string {
			return fmt.Sprintf("leaf value { %s default '%s'; }", patternStringType(`[ab]*a[ab]{1000}`, false), v)
		}},
		{"oversize1001", func(v string) string {
			return fmt.Sprintf("leaf value { %s default '%s'; }", patternStringType(`[ab]*a[ab]{1001}`, false), v)
		}},
		{"inversion", func(v string) string {
			return fmt.Sprintf("leaf value { %s default '%s'; }", patternStringType(`[ab]*a[ab]{1000}`, true), v)
		}},
		{"unionNextArmTrap", func(v string) string {
			return fmt.Sprintf("leaf value { type union { %s type string; } default '%s'; }", patternStringType(`[ab]*a[ab]{1001}`, false), v)
		}},
		{"nestedUnionTrap", func(v string) string {
			return fmt.Sprintf("leaf value { type union { type uint8; type union { %s type string; } type string; } default '%s'; }", patternStringType(`[ab]*a[ab]{1000}`, true), v)
		}},
		{"leafList", func(v string) string {
			return fmt.Sprintf("leaf-list value { %s default '%s'; }", patternStringType(`[ab]*a[ab]{1000}`, false), v)
		}},
		{"typedefInheritance", func(v string) string {
			return fmt.Sprintf("typedef bounded { %s default '%s'; } leaf value { type bounded; }", patternStringType(`[ab]*a[ab]{1000}`, false), v)
		}},
		{"unusedTypedef", func(v string) string {
			return fmt.Sprintf("typedef value { %s default '%s'; }", patternStringType(`[ab]*a[ab]{1001}`, false), v)
		}},
		{"leafref", func(v string) string {
			return fmt.Sprintf("leaf target { %s } leaf value { type leafref { path '/p:target'; } default '%s'; }", patternStringType(`[ab]*a[ab]{1000}`, false), v)
		}},
		{"unionLeafrefTrap", func(v string) string {
			return fmt.Sprintf("leaf target { %s } leaf value { type union { type leafref { path '/p:target'; } type string; } default '%s'; }", patternStringType(`[ab]*a[ab]{1001}`, false), v)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := buildPatternDefaults(t, tc.body(value))
			resource, ok := errors.AsType[*yangpattern.ResourceError](err)
			if !ok || resource.Phase != "match" {
				t.Fatalf("lost match resource cause: %.300s", err)
			}
			assertPatternDefaultContextError(t, err)
			context := "/pattern-defaults/value"
			if tc.name == "unusedTypedef" {
				context = `typedef "value"`
			}
			if !strings.Contains(err.Error(), context) || !strings.Contains(err.Error(), "[ab]*a[ab]") {
				t.Fatalf("missing default/pattern context: %.300s", err)
			}
			if strings.Contains(err.Error(), "does not satisfy pattern") || strings.Contains(err.Error(), "is not valid for union") {
				t.Fatal("operational failure described as ordinary invalid default")
			}
		})
	}
}
func TestSchemaPatternMetadataValidationAndShortCircuit(t *testing.T) {
	for _, tc := range []struct{ name, body, phase string }{
		{"laterStringPattern", `leaf value { type string { pattern 'z'; pattern '['; } default a; }`, ""},
		{"laterStringPatternAfterMatch", `leaf value { type string { pattern 'a'; pattern '['; } default a; }`, ""},
		{"laterUnionArm", `leaf value { type union { type string; type union { type uint8; type string { pattern '['; } } } default a; }`, ""},
		{"noDefaultSyntax", `leaf value { type string { pattern '['; } }`, ""},
		{"compileResource", `leaf value { ` + patternStringType(strings.Repeat("(", 257)+"a"+strings.Repeat(")", 257), false) + ` }`, "compile"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := buildPatternDefaults(t, tc.body)
			assertPatternDefaultContextError(t, err)
			if tc.phase == "" {
				if _, ok := errors.AsType[*yangpattern.ConfigurationError](err); !ok {
					t.Fatalf("lost configuration cause: %.300s", err)
				}
			} else {
				resource, ok := errors.AsType[*yangpattern.ResourceError](err)
				if !ok || resource.Phase != tc.phase {
					t.Fatalf("lost compile resource cause: %.300s", err)
				}
				if strings.Contains(err.Error(), "invalid pattern") {
					t.Fatal("resource refusal labeled invalid pattern syntax")
				}
			}
		})
	}
	t.Run("laterUnionPatternMetadata", func(t *testing.T) {
		_, err := buildPatternDefaults(t, `leaf value { type union { type string; type string { pattern 'a' { description one; description two; } } } default a; }`)
		assertPatternDefaultContextError(t, err)
		if !strings.Contains(err.Error(), "description") {
			t.Fatalf("metadata diagnostic: %.300s", err)
		}
	})
	for _, body := range []string{
		`leaf value { type string { pattern '[ab]*a[ab]{1000}'; } }`,
		`leaf value { type union { type string; type string { pattern '[ab]*a[ab]{1001}'; } } default '` + schemaDefaultDeterministicAB(1000000) + `'; }`,
		`leaf value { type union { type string { length '0'; pattern '[ab]*a[ab]{1000}'; } type string; } default '` + schemaDefaultDeterministicAB(1000000) + `'; }`,
	} {
		if _, err := buildPatternDefaults(t, body); err != nil {
			t.Fatalf("legitimate short circuit rejected: %.300s", err)
		}
	}
}
