// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

package yangparse

import (
	"testing"

	upstream "github.com/signalbreak-labs/cambium/go/internal/yangparse/upstream/yang"
)

// find returns the first statement with the given keyword anywhere in the tree.
func find(stmts []*Statement, keyword string) *Statement {
	for _, s := range stmts {
		if s.Keyword == keyword {
			return s
		}
		if got := find(s.SubStatements(), keyword); got != nil {
			return got
		}
	}
	return nil
}

func mustParse(t *testing.T, src string) []*Statement {
	t.Helper()
	stmts, err := Parse(src, "edge.yang")
	if err != nil {
		t.Fatalf("Parse: %v\nsource:\n%s", err, src)
	}
	return stmts
}

func TestDoubleQuotedEscapeSequences(t *testing.T) {
	src := "module m {\n" +
		"  namespace \"u\"; prefix p;\n" +
		"  leaf l {\n" +
		"    type string { pattern \"a\\tb\"; }\n" +
		"    description \"line1\\nline2 q\\\" bs\\\\\";\n" +
		"  }\n" +
		"}\n"
	stmts := mustParse(t, src)

	if got, want := find(stmts, "pattern").Argument, "a\tb"; got != want {
		t.Errorf("pattern escape: got %q want %q", got, want)
	}
	if got, want := find(stmts, "description").Argument, "line1\nline2 q\" bs\\"; got != want {
		t.Errorf("description escapes: got %q want %q", got, want)
	}
}

func TestStringConcatenationMixedQuotes(t *testing.T) {
	src := "module m {\n" +
		"  namespace \"u\"; prefix p;\n" +
		"  leaf l { type string; description \"foo\" + 'bar' + \"baz\"; }\n" +
		"}\n"
	stmts := mustParse(t, src)
	if got, want := find(stmts, "description").Argument, "foobarbaz"; got != want {
		t.Errorf("concatenation: got %q want %q", got, want)
	}
}

func TestConcatenationAcrossLinesWithComment(t *testing.T) {
	src := "module m {\n" +
		"  namespace \"u\"; prefix p;\n" +
		"  leaf l { type string; description \"a\" /* c */ +\n    \"b\"; }\n" +
		"}\n"
	stmts := mustParse(t, src)
	if got, want := find(stmts, "description").Argument, "ab"; got != want {
		t.Errorf("multiline concat with comment: got %q want %q", got, want)
	}
}

func TestSingleQuotedIsLiteral(t *testing.T) {
	src := "module m {\n" +
		"  namespace \"u\"; prefix p;\n" +
		"  leaf l { type string { pattern '\\d+\\.[0-9]'; } }\n" +
		"}\n"
	stmts := mustParse(t, src)
	if got, want := find(stmts, "pattern").Argument, `\d+\.[0-9]`; got != want {
		t.Errorf("single-quoted literal: got %q want %q", got, want)
	}
}

func TestEmptyVersusAbsentArgument(t *testing.T) {
	src := "module m {\n" +
		"  namespace \"u\"; prefix p;\n" +
		"  rpc r { input { leaf a { type string; default \"\"; } } }\n" +
		"}\n"
	stmts := mustParse(t, src)

	def := find(stmts, "default")
	if !def.HasArgument || def.Argument != "" {
		t.Errorf("empty argument: HasArgument=%v Argument=%q, want true/\"\"", def.HasArgument, def.Argument)
	}
	in := find(stmts, "input")
	if in.HasArgument {
		t.Errorf("no-argument statement: HasArgument=%v, want false", in.HasArgument)
	}
}

func TestUnterminatedConstructsFailClosed(t *testing.T) {
	cases := map[string]string{
		"double-quote":  "module m { description \"unterminated",
		"single-quote":  "module m { description 'unterminated",
		"block-comment": "module m { /* unterminated",
		"block":         "module m { leaf l { type string;",
	}
	for name, src := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := Parse(src, name+".yang"); err == nil {
				t.Fatalf("Parse accepted unterminated %s", name)
			}
		})
	}
}

// TestEdgeCasesMatchUpstream feeds synthetic valid inputs (escapes, concatenation,
// CRLF, empty args) through both parsers; where both accept, trees must match.
// This extends the corpus differential to lexer paths the corpus does not cover.
func TestEdgeCasesMatchUpstream(t *testing.T) {
	inputs := []string{
		"module m { namespace \"u\"; prefix p; leaf l { type string; description \"a\\nb\\tc\\\"d\\\\e\"; } }\n",
		"module m { namespace \"u\"; prefix p; leaf l { type string; description \"x\" + 'y' + \"z\"; } }\n",
		"module m {\r\n namespace \"u\";\r\n prefix p;\r\n description \"l1\r\n l2\";\r\n}\r\n",
		"module m { namespace \"u\"; prefix p; leaf l { type string; default \"\"; } }\n",
		"module m { namespace \"u\"; prefix p; rpc r { input { leaf a { type string; } } } }\n",
	}
	for i, src := range inputs {
		native, nerr := Parse(src, "edge.yang")
		up, uerr := upstream.Parse(src, "edge.yang")
		if nerr != nil || uerr != nil {
			t.Fatalf("case %d: native err=%v upstream err=%v", i, nerr, uerr)
		}
		if err := compareStatementLists(native, up); err != nil {
			t.Errorf("case %d diverged: %v\nsource:\n%s", i, err, src)
		}
	}
}
