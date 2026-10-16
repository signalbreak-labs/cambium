// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

package cambium_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/signalbreak-labs/cambium/go/cambium"
)

const wildEscapeSource = `module wild-escape {
    namespace "urn:wild-escape";
    prefix we;
    leaf name {
        type string {
            pattern "\*";
        }
    }
}
`

func patternStatementArgument(t *testing.T, stmts []cambium.Statement) string {
	t.Helper()
	var found string
	var walk func([]cambium.Statement)
	walk = func(list []cambium.Statement) {
		for _, stmt := range list {
			if stmt.Keyword() == "pattern" {
				arg, ok := stmt.Argument()
				if !ok {
					t.Fatal("pattern statement has no argument")
				}
				found = arg
			}
			walk(stmt.SubStatements())
		}
	}
	walk(stmts)
	if found == "" {
		t.Fatal("no pattern statement found")
	}
	return found
}

// Raw statement inspection is strict by default; the WithOptions variants
// apply the same vendor-compatible pattern-escape preservation as schema
// loading and return the relaxation as diagnostics.
func TestParseStatementsWithOptionsPatternEscapes(t *testing.T) {
	if _, err := cambium.ParseStatements(wildEscapeSource, "wild-escape.yang"); err == nil {
		t.Fatal("ParseStatements accepted pattern escape in strict mode")
	} else if !strings.Contains(err.Error(), `invalid escape sequence: \*`) {
		t.Fatalf("ParseStatements error = %q, want invalid escape sequence", err)
	}

	stmts, diagnostics, err := cambium.ParseStatementsWithOptions(wildEscapeSource, "wild-escape.yang", cambium.StatementParseOptions{
		ValidationMode: cambium.ValidationVendorCompatible,
	})
	if err != nil {
		t.Fatalf("ParseStatementsWithOptions: %v", err)
	}
	if got := patternStatementArgument(t, stmts); got != `\*` {
		t.Fatalf("pattern argument = %q, want \\*", got)
	}
	if len(diagnostics) != 1 {
		t.Fatalf("diagnostics = %+v, want 1", diagnostics)
	}
	diag := diagnostics[0]
	if !strings.Contains(diag.Message, `invalid escape sequence: \*`) {
		t.Fatalf("diagnostic message = %q, want invalid escape sequence", diag.Message)
	}
	if diag.Module != "wild-escape" || diag.Source.Line != 6 {
		t.Fatalf("diagnostic = %+v, want module wild-escape at line 6", diag)
	}

	if _, _, err := cambium.ParseStatementsWithOptions(wildEscapeSource, "wild-escape.yang", cambium.StatementParseOptions{
		ValidationMode: cambium.ValidationMode(99),
	}); err == nil {
		t.Fatal("ParseStatementsWithOptions accepted an invalid validation mode")
	}
}

func TestReadStatementsWithOptionsPatternEscapes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "wild-escape.yang")
	if err := os.WriteFile(path, []byte(wildEscapeSource), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := cambium.ReadStatements(path); err == nil {
		t.Fatal("ReadStatements accepted pattern escape in strict mode")
	}
	stmts, diagnostics, err := cambium.ReadStatementsWithOptions(path, cambium.StatementParseOptions{
		ValidationMode: cambium.ValidationVendorCompatible,
	})
	if err != nil {
		t.Fatalf("ReadStatementsWithOptions: %v", err)
	}
	if got := patternStatementArgument(t, stmts); got != `\*` {
		t.Fatalf("pattern argument = %q, want \\*", got)
	}
	if len(diagnostics) != 1 || diagnostics[0].Source.File != path {
		t.Fatalf("diagnostics = %+v, want 1 for %s", diagnostics, path)
	}
}
