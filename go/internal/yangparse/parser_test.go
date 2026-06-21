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
