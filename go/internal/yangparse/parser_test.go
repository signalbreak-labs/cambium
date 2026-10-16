// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

package yangparse

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseRejectsExcessiveStatementDepth(t *testing.T) {
	var b strings.Builder
	b.WriteString(`module too-deep { namespace "urn:too-deep"; prefix td;`)
	for i := 0; i < maxStatementDepth+1; i++ {
		b.WriteString(" container c")
		b.WriteString("x")
		b.WriteString(" {")
	}
	for i := 0; i < maxStatementDepth+1; i++ {
		b.WriteByte('}')
	}
	b.WriteByte('}')

	_, err := Parse(b.String(), "too-deep.yang")
	if err == nil {
		t.Fatal("Parse accepted input above the maximum statement depth")
	}
	if got := err.Error(); !strings.Contains(got, "nesting depth exceeds maximum") {
		t.Fatalf("Parse error = %q, want nesting-depth error", got)
	}
}

func TestParseRejectsExcessiveDepthAfterCRTerminatedLineComment(t *testing.T) {
	var b strings.Builder
	b.WriteString("module too-deep-cr-comment { namespace \"urn:too-deep-cr-comment\"; prefix td; // comment\r")
	for i := 0; i < maxStatementDepth+1; i++ {
		b.WriteString(" container c")
		b.WriteString("x")
		b.WriteString(" {")
	}
	for i := 0; i < maxStatementDepth+1; i++ {
		b.WriteByte('}')
	}
	b.WriteByte('}')

	_, err := Parse(b.String(), "too-deep-cr-comment.yang")
	if err == nil {
		t.Fatal("Parse accepted input above the maximum statement depth after a CR-terminated line comment")
	}
	if got := err.Error(); !strings.Contains(got, "nesting depth exceeds maximum") {
		t.Fatalf("Parse error = %q, want nesting-depth error", got)
	}
}

func TestParseDepthIgnoresQuotedBraces(t *testing.T) {
	source := `module quoted-braces {
  namespace "urn:quoted-braces";
  prefix qb;
  description "` + strings.Repeat("{", maxStatementDepth+1) + `";
  container top {
    leaf name { type string; }
  }
}
`
	stmts, err := Parse(source, "quoted-braces.yang")
	if err != nil {
		t.Fatalf("Parse quoted braces: %v", err)
	}
	if got, want := len(stmts), 1; got != want {
		t.Fatalf("statements = %d, want %d", got, want)
	}
}

func TestParseRejectsOversizedInput(t *testing.T) {
	_, err := Parse(strings.Repeat(" ", maxInputBytes+1), "oversized.yang")
	if err == nil {
		t.Fatal("Parse accepted oversized input")
	}
	if got := err.Error(); !strings.Contains(got, "exceeds maximum") {
		t.Fatalf("Parse error = %q, want size-bound error", got)
	}
}

func TestParseDoubleQuotedEscapedApostrophe(t *testing.T) {
	source := `module escaped-apostrophe {
  namespace "urn:test:escaped-apostrophe";
  prefix ea;

  description "Vendor text can\'t always avoid this escape.";

  container top;
}
`
	stmts, err := Parse(source, "escaped-apostrophe.yang")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	desc := find(stmts, "description")
	if desc == nil {
		t.Fatal("description not found")
	}
	if got, want := desc.Argument, "Vendor text can't always avoid this escape."; got != want {
		t.Fatalf("description = %q, want %q", got, want)
	}
}

func TestParseRejectsInvalidUTF8(t *testing.T) {
	source := `module bad-utf8 {
  namespace "urn:bad-utf8";
  prefix bu;
  description "` + string([]byte{0xff}) + `";
}
`

	_, err := Parse(source, "bad-utf8.yang")
	if err == nil {
		t.Fatal("Parse accepted invalid UTF-8")
	}
	if got := err.Error(); !strings.Contains(got, "invalid UTF-8") {
		t.Fatalf("Parse error = %q, want invalid UTF-8 error", got)
	}
}

func TestParseRejectsIllegalSourceCharacters(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    string
	}{
		{
			name:    "nul",
			content: "\x00",
			want:    "U+0000",
		},
		{
			name:    "c0-control",
			content: "\x01",
			want:    "U+0001",
		},
		{
			name:    "noncharacter",
			content: "\ufdd0",
			want:    "U+FDD0",
		},
		{
			name:    "plane-noncharacter",
			content: "\U0010ffff",
			want:    "U+10FFFF",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			source := `module bad-source-character {
  namespace "urn:bad-source-character";
  prefix bsc;
  description "` + tt.content + `";
}
`

			_, err := Parse(source, tt.name+".yang")
			if err == nil {
				t.Fatal("Parse accepted illegal YANG source character")
			}
			got := err.Error()
			if !strings.Contains(got, "illegal YANG source character") || !strings.Contains(got, tt.want) {
				t.Fatalf("Parse error = %q, want illegal source character %s", got, tt.want)
			}
		})
	}
}

func TestParseAllowsLegalSourceCharacters(t *testing.T) {
	source := "module legal-source-characters {\r\n" +
		"\tnamespace \"urn:legal-source-characters\";\r\n" +
		"\tprefix lsc;\r\n" +
		"\tdescription \"unicode " + string(rune(0x2603)) + " and allowed whitespace\";\r\n" +
		"}\r\n"

	stmts, err := Parse(source, "legal-source-characters.yang")
	if err != nil {
		t.Fatalf("Parse legal source characters: %v", err)
	}
	if got, want := len(stmts), 1; got != want {
		t.Fatalf("statements = %d, want %d", got, want)
	}
}

func TestParseTerminatesUnquotedArgumentBeforeAdjacentComment(t *testing.T) {
	source := `module comment-after-unquoted {
  namespace urn:comment-after-unquoted/* adjacent block comment */;
  prefix cau// adjacent line comment
  ;
  leaf value { type string; }
}
`

	stmts, err := Parse(source, "comment-after-unquoted.yang")
	if err != nil {
		t.Fatalf("Parse adjacent comments: %v", err)
	}
	if got, want := len(stmts), 1; got != want {
		t.Fatalf("statements = %d, want %d", got, want)
	}
	mod := stmts[0]
	if got, want := mod.Keyword, "module"; got != want {
		t.Fatalf("top keyword = %q, want %q", got, want)
	}
	if got, want := mod.Argument, "comment-after-unquoted"; got != want {
		t.Fatalf("module argument = %q, want %q", got, want)
	}

	var gotNamespace, gotPrefix string
	for _, sub := range mod.SubStatements() {
		switch sub.Keyword {
		case "namespace":
			gotNamespace = sub.Argument
		case "prefix":
			gotPrefix = sub.Argument
		}
	}
	if got, want := gotNamespace, "urn:comment-after-unquoted"; got != want {
		t.Fatalf("namespace argument = %q, want %q", got, want)
	}
	if got, want := gotPrefix, "cau"; got != want {
		t.Fatalf("prefix argument = %q, want %q", got, want)
	}
}

func TestParseRejectsStandaloneCommentCloseInUnquotedArgument(t *testing.T) {
	source := `module bad-unquoted-comment-close {
  namespace "urn:bad-unquoted-comment-close";
  prefix bucc;
  description invalid*/text;
}
`

	_, err := Parse(source, "bad-unquoted-comment-close.yang")
	if err == nil {
		t.Fatal("Parse accepted standalone comment-close sequence in unquoted argument")
	}
	if got := err.Error(); !strings.Contains(got, "*/") {
		t.Fatalf("Parse error = %q, want to mention comment-close sequence", got)
	}
}

func TestParseRejectsQuotesInUnquotedArgument(t *testing.T) {
	tests := map[string]string{
		"single-quote": `module bad-unquoted-single-quote {
  namespace "urn:bad-unquoted-single-quote";
  prefix busq;
  description invalid'value;
}
`,
		"double-quote": `module bad-unquoted-double-quote {
  namespace "urn:bad-unquoted-double-quote";
  prefix budq;
  description invalid"value;
}
`,
	}
	for name, source := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := Parse(source, name+".yang")
			if err == nil {
				t.Fatal("Parse accepted quote character in unquoted argument")
			}
			if got := err.Error(); !strings.Contains(got, "quote") {
				t.Fatalf("Parse error = %q, want quote-specific error", got)
			}
		})
	}
}

func TestParseRejectsUnknownDoubleQuotedPatternEscape(t *testing.T) {
	source := `module bad-pattern-escape {
  namespace "urn:bad-pattern-escape";
  prefix bpe;
  leaf value {
    type string {
      pattern "\p";
    }
  }
}
`

	_, err := Parse(source, "bad-pattern-escape.yang")
	if err == nil {
		t.Fatal("Parse accepted unknown double-quoted pattern escape")
	}
	if got := err.Error(); !strings.Contains(got, `invalid escape sequence: \p`) {
		t.Fatalf("Parse error = %q, want invalid escape sequence", got)
	}
}

func patternArguments(stmts []*Statement) []string {
	var args []string
	var walk func([]*Statement)
	walk = func(list []*Statement) {
		for _, st := range list {
			if st.Keyword == "pattern" {
				args = append(args, st.Argument)
			}
			walk(st.SubStatements())
		}
	}
	walk(stmts)
	return args
}

// Real-world vendor modules write regex escapes such as \* directly in
// double-quoted pattern arguments. AllowPatternEscapes preserves the
// backslash so the regex keeps its meaning, and reports each occurrence.
func TestParseWithOptionsPreservesPatternEscapes(t *testing.T) {
	source := `module wild-escape {
  namespace "urn:wild-escape";
  prefix we;
  leaf star {
    type string {
      pattern "\*";
    }
  }
  leaf dotted {
    type string {
      pattern "\d+\.\d+";
    }
  }
  leaf joined {
    type string {
      pattern "x" + "\*";
    }
  }
  leaf escaped {
    type string {
      pattern "\\*";
    }
  }
}
`
	stmts, warnings, err := ParseWithOptions(source, "wild-escape.yang", ParseOptions{AllowPatternEscapes: true})
	if err != nil {
		t.Fatalf("ParseWithOptions: %v", err)
	}
	want := []string{`\*`, `\d+\.\d+`, `x\*`, `\*`}
	got := patternArguments(stmts)
	if len(got) != len(want) {
		t.Fatalf("pattern arguments = %q, want %q", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("pattern arguments = %q, want %q", got, want)
		}
	}
	// One warning per preserved escape (\d+\.\d+ has three); the properly
	// escaped "\\*" is silent.
	if len(warnings) != 5 {
		t.Fatalf("warnings = %+v, want 5", warnings)
	}
	if !strings.Contains(warnings[0].Message, `invalid escape sequence: \*`) {
		t.Fatalf("first warning = %q, want invalid escape sequence", warnings[0].Message)
	}
	if warnings[0].File != "wild-escape.yang" || warnings[0].Line != 6 {
		t.Fatalf("first warning location = %s:%d:%d, want wild-escape.yang:6", warnings[0].File, warnings[0].Line, warnings[0].Column)
	}
}

// AllowPatternEscapes is scoped to pattern arguments: unknown escapes in
// any other double-quoted string remain errors.
func TestParseWithOptionsStillRejectsNonPatternEscapes(t *testing.T) {
	source := `module bad-description-escape {
  namespace "urn:bad-description-escape";
  prefix bde;
  leaf value {
    description "\q";
    type string;
  }
}
`
	_, _, err := ParseWithOptions(source, "bad-description-escape.yang", ParseOptions{AllowPatternEscapes: true})
	if err == nil {
		t.Fatal("ParseWithOptions accepted unknown double-quoted description escape")
	}
	if got := err.Error(); !strings.Contains(got, `invalid escape sequence: \q`) {
		t.Fatalf("ParseWithOptions error = %q, want invalid escape sequence", got)
	}
}

func statementArgs(stmts []*Statement, keyword string) []string {
	var args []string
	var walk func([]*Statement)
	walk = func(list []*Statement) {
		for _, st := range list {
			if st.Keyword == keyword {
				args = append(args, st.Argument)
			}
			walk(st.SubStatements())
		}
	}
	walk(stmts)
	return args
}

func escapeMatrixModule(stmt string) string {
	return "module escape-matrix {\n" +
		"  namespace \"urn:escape-matrix\";\n" +
		"  prefix em;\n" +
		"  leaf value {\n" +
		"    type string {\n" +
		"      " + stmt + "\n" +
		"    }\n" +
		"  }\n" +
		"}\n"
}

// TestParsePatternEscapeMatrix pins the RFC 7950 6.1.3 / XSD regex boundary:
// YANG defines only \n, \t, \" and \\ in double-quoted strings ("The
// backslash MUST NOT be followed by any other character"), while a pattern
// argument is an XSD regular expression whose own escapes (\d, \*, \p{...},
// ...) need the backslash to survive. Strict parsing rejects the collision;
// AllowPatternEscapes preserves it inside pattern arguments only.
func TestParsePatternEscapeMatrix(t *testing.T) {
	vendor := ParseOptions{AllowPatternEscapes: true}

	// Every XSD single/multi character escape that YANG does not define is
	// preserved verbatim in a pattern argument, with one warning each.
	preserved := []string{
		`\*`, `\.`, `\+`, `\?`, `\(`, `\)`, `\{`, `\}`, `\[`, `\]`, `\^`, `\-`, `\|`, `\r`,
		`\d`, `\D`, `\s`, `\S`, `\w`, `\W`, `\p{L}`, `\P{L}`,
	}
	for _, esc := range preserved {
		source := escapeMatrixModule(`pattern "` + esc + `";`)
		stmts, warnings, err := ParseWithOptions(source, "escape-matrix.yang", vendor)
		if err != nil {
			t.Fatalf("%s: ParseWithOptions(vendor): %v", esc, err)
		}
		if got := statementArgs(stmts, "pattern"); len(got) != 1 || got[0] != esc {
			t.Fatalf("%s: pattern argument = %q, want [%q]", esc, got, esc)
		}
		if len(warnings) != 1 {
			t.Fatalf("%s: warnings = %+v, want 1", esc, warnings)
		}
		if _, _, err := ParseWithOptions(source, "escape-matrix.yang", ParseOptions{}); err == nil {
			t.Fatalf("%s: strict ParseWithOptions accepted escape", esc)
		}
	}

	// An escape that is not valid XSD either is still preserved at parse
	// level (schema compilation validates the regex separately), matching
	// goyang's pattern handling.
	stmts, warnings, err := ParseWithOptions(escapeMatrixModule(`pattern "\q";`), "escape-matrix.yang", vendor)
	if err != nil {
		t.Fatalf(`\q: ParseWithOptions(vendor): %v`, err)
	}
	if got := statementArgs(stmts, "pattern"); len(got) != 1 || got[0] != `\q` || len(warnings) != 1 {
		t.Fatalf(`\q: args = %q warnings = %+v, want [\q] and 1 warning`, got, warnings)
	}

	// YANG-defined escapes are substituted in every mode, without warnings,
	// including the long-standing \' leniency for vendor files.
	defined := []struct{ source, want string }{
		{`pattern "\n";`, "\n"},
		{`pattern "\t";`, "\t"},
		{`pattern "\\";`, `\`},
		{`pattern "\"";`, `"`},
		{`pattern "\'";`, `'`},
		{`pattern '\*';`, `\*`},
		{`pattern \*;`, `\*`},
	}
	for _, tc := range defined {
		for _, opts := range []ParseOptions{{}, vendor} {
			stmts, warnings, err := ParseWithOptions(escapeMatrixModule(tc.source), "escape-matrix.yang", opts)
			if err != nil {
				t.Fatalf("%s (opts %+v): %v", tc.source, opts, err)
			}
			if got := statementArgs(stmts, "pattern"); len(got) != 1 || got[0] != tc.want {
				t.Fatalf("%s (opts %+v): pattern argument = %q, want [%q]", tc.source, opts, got, tc.want)
			}
			if len(warnings) != 0 {
				t.Fatalf("%s (opts %+v): warnings = %+v, want none", tc.source, opts, warnings)
			}
		}
	}

	// The relaxation is keyword-scoped: a prefixed extension statement is
	// not a pattern statement.
	if _, _, err := ParseWithOptions(escapeMatrixModule(`em:posix-pattern "\d";`), "escape-matrix.yang", vendor); err == nil {
		t.Fatal("vendor parsing accepted escape in prefixed extension argument")
	}

	// \r outside a pattern argument stays an error in every mode.
	if _, _, err := ParseWithOptions(escapeMatrixModule("description \"a\\rb\";\n      pattern \"x\";"), "escape-matrix.yang", vendor); err == nil {
		t.Fatal("vendor parsing accepted \\r in description")
	}
}

// TestParsePatternEscapeShapes covers concatenation, multiline strings,
// modifiers, and warning order across multiple pattern statements.
func TestParsePatternEscapeShapes(t *testing.T) {
	vendor := ParseOptions{AllowPatternEscapes: true}
	source := `module escape-shapes {
  namespace "urn:escape-shapes";
  prefix es;
  leaf joined {
    type string {
      pattern '\d' + "\." + "\d+";
    }
  }
  leaf multiline {
    type string {
      pattern "abc
\*";
    }
  }
  leaf inverted {
    type string {
      pattern "\*" {
        modifier invert-match;
      }
      pattern "\d+";
    }
  }
}
`
	stmts, warnings, err := ParseWithOptions(source, "escape-shapes.yang", vendor)
	if err != nil {
		t.Fatalf("ParseWithOptions: %v", err)
	}
	want := []string{`\d\.\d+`, "abc\n\\*", `\*`, `\d+`}
	got := statementArgs(stmts, "pattern")
	if len(got) != len(want) {
		t.Fatalf("pattern arguments = %q, want %q", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("pattern arguments = %q, want %q", got, want)
		}
	}
	// Joined contributes \. and \d, multiline \*, inverted \* and \d.
	if len(warnings) != 5 {
		t.Fatalf("warnings = %+v, want 5", warnings)
	}
	if warnings[2].Line != 12 {
		t.Fatalf("multiline warning = %+v, want line 12", warnings[2])
	}
}

// TestParsePatternBackslashNewline pins the pathological backslash at end
// of line inside a double-quoted pattern argument: vendor parsing keeps
// both characters (schema regex validation rejects the result separately),
// strict parsing rejects it as an undefined escape.
func TestParsePatternBackslashNewline(t *testing.T) {
	source := escapeMatrixModule("pattern \"a\\\nb\";")
	stmts, warnings, err := ParseWithOptions(source, "escape-matrix.yang", ParseOptions{AllowPatternEscapes: true})
	if err != nil {
		t.Fatalf("ParseWithOptions(vendor): %v", err)
	}
	if got := statementArgs(stmts, "pattern"); len(got) != 1 || got[0] != "a\\\nb" {
		t.Fatalf("pattern argument = %q, want a, backslash, newline, b", got)
	}
	if len(warnings) != 1 {
		t.Fatalf("warnings = %+v, want 1", warnings)
	}
	if _, _, err := ParseWithOptions(source, "escape-matrix.yang", ParseOptions{}); err == nil {
		t.Fatal("strict ParseWithOptions accepted backslash-newline")
	}
}

func TestReadFileRejectsOversizedFileBeforeRead(t *testing.T) {
	path := filepath.Join(t.TempDir(), "oversized.yang")
	file, err := os.Create(path)
	if err != nil {
		t.Fatalf("create oversized file: %v", err)
	}
	if err := file.Truncate(maxInputBytes + 1); err != nil {
		_ = file.Close()
		t.Fatalf("truncate oversized file: %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("close oversized file: %v", err)
	}

	_, err = ReadFile(path)
	if err == nil {
		t.Fatal("ReadFile accepted oversized file")
	}
	if got := err.Error(); !strings.Contains(got, "exceeds maximum") {
		t.Fatalf("ReadFile error = %q, want size-bound error", got)
	}
}
