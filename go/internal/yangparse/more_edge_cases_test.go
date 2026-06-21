// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

package yangparse

import (
	"testing"

	upstream "github.com/signalbreak-labs/cambium/go/internal/yangparse/upstream/yang"
)

// TestUnquotedConcatenationRejected: per RFC 7950 6.1.3 the '+' operator joins
// quoted strings only; an unquoted argument followed by '+' is a syntax error
// (matching the reference parser, which the corpus differential pins behavior to).
func TestUnquotedConcatenationRejected(t *testing.T) {
	src := "module m { namespace \"u\"; prefix p;\n" +
		"  leaf l { type string; description foo + bar; }\n" +
		"}\n"
	if _, err := Parse(src, "concat.yang"); err == nil {
		t.Fatal("Parse accepted '+' concatenation of unquoted operands")
	}
	// The reference parser also rejects it; equivalence holds (both decline).
	if _, err := upstream.Parse(src, "concat.yang"); err == nil {
		t.Fatal("oracle assumption broken: upstream accepted unquoted '+' concatenation")
	}
}

// TestQuotedAfterPlusRequired: a '+' between a quoted string and an unquoted
// operand is rejected.
func TestQuotedAfterPlusRequired(t *testing.T) {
	src := "module m { namespace \"u\"; prefix p;\n" +
		"  leaf l { type string; default \"a\" + b; }\n" +
		"}\n"
	if _, err := Parse(src, "concat2.yang"); err == nil {
		t.Fatal("Parse accepted quoted '+' unquoted concatenation")
	}
}

// TestTabsAndCRMatchUpstream pins the tab-expanded indentation trimming and
// bare-CR handling (the cases the all-space corpus does not exercise) to the
// goyang oracle. Tabs are literal "\t" in these inputs.
func TestTabsAndCRMatchUpstream(t *testing.T) {
	const bom = "\ufeff"
	inputs := []string{
		// Tab before the opening quote + space continuation indentation.
		"module m {\n\tnamespace \"u\";\n\tprefix p;\n\tdescription \"a\n\t            b\";\n}\n",
		// Tab-only continuation indentation under a space-indented quote.
		"module m {\n  namespace \"u\"; prefix p;\n  description \"a\n\t\tb\";\n}\n",
		// Bare CR (not CRLF) inside a double-quoted string: ordinary content.
		"module m {\n namespace \"u\"; prefix p;\n description \"a  \r b\";\n}\n",
		// CRLF line endings throughout, with a multi-line quoted description.
		"module m {\r\n namespace \"u\";\r\n prefix p;\r\n description \"l1\r\n l2\";\r\n}\r\n",
		// Zero separators around structural punctuation and quotes.
		"module m{namespace\"u\";prefix p;container c{leaf x{type string;}}}\n",
		// Comment-only and empty inputs (both should yield no statements).
		"// only a comment\n/* and a block */\n",
		"",
		// Leading UTF-8 BOM before the module keyword.
		bom + "module m { namespace \"u\"; prefix p; }\n",
		// Bare (unquoted) numeric argument.
		"module m { namespace \"u\"; prefix p; revision 2020 { description x; } }\n",
	}
	for i, src := range inputs {
		native, nerr := Parse(src, "edge.yang")
		up, uerr := upstream.Parse(src, "edge.yang")
		if (nerr == nil) != (uerr == nil) {
			t.Errorf("case %d accept/reject mismatch: native err=%v upstream err=%v\nsource:\n%q", i, nerr, uerr, src)
			continue
		}
		if nerr != nil {
			continue
		}
		if err := compareStatementLists(native, up); err != nil {
			t.Errorf("case %d diverged: %v\nsource:\n%q", i, err, src)
		}
	}
}
