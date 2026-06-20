package xsdregex

import (
	"regexp"
	"testing"
)

func TestNativePatternXMLSchemaBlocksCompileAndMatch(t *testing.T) {
	for name, ranges := range unicodeBlockRanges {
		if len(ranges) == 0 {
			t.Fatalf("%s has no ranges", name)
		}
		expr := `\p{Is` + name + `}`
		re, err := regexp.Compile("^(?:" + NativePattern(expr) + ")$")
		if err != nil {
			t.Fatalf("%s native regexp compile: %v", name, err)
		}
		sample := string(ranges[0].lo)
		if !re.MatchString(sample) {
			t.Fatalf("%s did not match first block code point U+%04X", name, ranges[0].lo)
		}
		if ranges[0].lo > 0 {
			before := string(ranges[0].lo - 1)
			if re.MatchString(before) {
				t.Fatalf("%s matched code point before block U+%04X", name, ranges[0].lo-1)
			}
		}

		complement := `\P{Is` + name + `}`
		complementRE, err := regexp.Compile("^(?:" + NativePattern(complement) + ")$")
		if err != nil {
			t.Fatalf("%s complement native regexp compile: %v", name, err)
		}
		if complementRE.MatchString(sample) {
			t.Fatalf("%s complement matched first block code point U+%04X", name, ranges[0].lo)
		}
	}
}

func TestNativePatternXMLSchemaMultiCharacterEscapes(t *testing.T) {
	tests := []struct {
		name    string
		expr    string
		valid   string
		invalid string
	}{
		{name: "digit", expr: `\d`, valid: "\u0661", invalid: "A"},
		{name: "not digit", expr: `\D`, valid: "A", invalid: "\u0661"},
		{name: "space", expr: `\s`, valid: "\t", invalid: "\f"},
		{name: "not space", expr: `\S`, valid: "\f", invalid: "\t"},
		{name: "word", expr: `\w`, valid: "$", invalid: "-"},
		{name: "not word", expr: `\W`, valid: "-", invalid: "$"},
		{name: "name start", expr: `\i`, valid: "_", invalid: "1"},
		{name: "not name start", expr: `\I`, valid: "1", invalid: "_"},
		{name: "name char", expr: `\c`, valid: "-", invalid: " "},
		{name: "not name char", expr: `\C`, valid: " ", invalid: "-"},
		{name: "wildcard", expr: `.`, valid: "x", invalid: "\r"},
		{name: "literal anchors", expr: `^foo$`, valid: "^foo$", invalid: "foo"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			re, err := regexp.Compile("^(?:" + NativePattern(tt.expr) + ")$")
			if err != nil {
				t.Fatalf("native regexp compile: %v", err)
			}
			if !re.MatchString(tt.valid) {
				t.Fatalf("%s did not match valid value %q", tt.expr, tt.valid)
			}
			if re.MatchString(tt.invalid) {
				t.Fatalf("%s matched invalid value %q", tt.expr, tt.invalid)
			}
		})
	}
}

func TestNativePatternXMLSchemaClassSubtractionRanges(t *testing.T) {
	tests := []struct {
		name     string
		expr     string
		valid    string
		invalids []string
	}{
		{
			name:     "unicode block minus category",
			expr:     `[\p{IsGreek}-[\p{Lu}]]+`,
			valid:    "\u03c9",
			invalids: []string{"\u03a9", "a"},
		},
		{
			name:     "name characters minus digits",
			expr:     `[\c-[\d]]+`,
			valid:    "name-",
			invalids: []string{"1"},
		},
		{
			name:     "negated subtractor",
			expr:     `[a-z-[^aeiou]]+`,
			valid:    "aeiou",
			invalids: []string{"b", "abc"},
		},
		{
			name:     "nested subtraction",
			expr:     `[a-z-[a-m-[aeiou]]]+`,
			valid:    "aeiounz",
			invalids: []string{"b", "af"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if unsupported := UnsupportedNativeSyntax(tt.expr); unsupported != "" {
				t.Fatalf("UnsupportedNativeSyntax(%q) = %q", tt.expr, unsupported)
			}
			re, err := regexp.Compile("^(?:" + NativePattern(tt.expr) + ")$")
			if err != nil {
				t.Fatalf("native regexp compile: %v", err)
			}
			if !re.MatchString(tt.valid) {
				t.Fatalf("%s did not match valid value %q", tt.expr, tt.valid)
			}
			for _, invalid := range tt.invalids {
				if re.MatchString(invalid) {
					t.Fatalf("%s matched invalid value %q", tt.expr, invalid)
				}
			}
		})
	}
}

func TestNativePatternXMLSchemaEmptyClassSubtraction(t *testing.T) {
	const expr = `[a-[a]]`
	if unsupported := UnsupportedNativeSyntax(expr); unsupported != "" {
		t.Fatalf("UnsupportedNativeSyntax(%q) = %q", expr, unsupported)
	}
	re, err := regexp.Compile("^(?:" + NativePattern(expr) + ")$")
	if err != nil {
		t.Fatalf("native regexp compile: %v", err)
	}
	for _, value := range []string{"", "a", "b"} {
		if re.MatchString(value) {
			t.Fatalf("%s matched %q, want no matches", expr, value)
		}
	}
}
