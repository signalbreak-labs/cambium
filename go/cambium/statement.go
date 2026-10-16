// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

package cambium

import (
	"fmt"

	"github.com/signalbreak-labs/cambium/go/internal/yangparse"
)

// Statement is a read-only handle to one parsed YANG statement.
//
// It is Cambium's native raw statement surface: useful for lightweight source
// inspection, extension-aware tooling, and migration code that needs a generic
// YANG statement tree without importing the goyang-shaped compat package.
// Semantic schema resolution lives on Context/Module/SchemaNodeRef.
type Statement struct{ stmt *yangparse.Statement }

// StatementParseOptions selects raw statement parsing behavior. The zero
// value is strict RFC 7950 parsing.
type StatementParseOptions struct {
	// ValidationMode mirrors the schema-loading modes: ValidationStrict
	// (the default) fails closed on vendor source defects, while
	// ValidationVendorCompatible preserves unknown backslash escapes in
	// double-quoted pattern arguments as regex escapes and reports each
	// one in the returned diagnostics.
	ValidationMode ValidationMode
}

// ParseStatements parses one YANG source document into ordered top-level
// statements using Cambium's native RFC 7950 parser. Parsing is strict;
// use ParseStatementsWithOptions for vendor-compatible parsing.
func ParseStatements(input, name string) ([]Statement, error) {
	stmts, _, err := ParseStatementsWithOptions(input, name, StatementParseOptions{})
	return stmts, err
}

// ParseStatementsWithOptions is ParseStatements with explicit options. The
// returned diagnostics report every vendor-compatible relaxation applied
// while parsing, in source order; they are empty in strict mode.
func ParseStatementsWithOptions(input, name string, opts StatementParseOptions) ([]Statement, []Diagnostic, error) {
	if !validValidationMode(opts.ValidationMode) {
		return nil, nil, wrap("parse statements", fmt.Errorf("invalid validation mode %d", opts.ValidationMode))
	}
	stmts, warnings, err := yangparse.ParseWithOptions(input, name, parseOptionsForMode(opts.ValidationMode))
	if err != nil {
		return nil, nil, err
	}
	module := ""
	if len(stmts) == 1 && (stmts[0].Keyword == "module" || stmts[0].Keyword == "submodule") {
		module = stmts[0].Argument
	}
	return publicStatements(stmts), patternEscapeDiagnostics(module, warnings), nil
}

// ReadStatements reads path and parses it into ordered top-level statements.
// Parsing is strict; use ReadStatementsWithOptions for vendor-compatible
// parsing.
func ReadStatements(path string) ([]Statement, error) {
	stmts, _, err := ReadStatementsWithOptions(path, StatementParseOptions{})
	return stmts, err
}

// ReadStatementsWithOptions is ReadStatements with explicit options; see
// ParseStatementsWithOptions for the returned diagnostics.
func ReadStatementsWithOptions(path string, opts StatementParseOptions) ([]Statement, []Diagnostic, error) {
	input, err := yangparse.ReadFile(path)
	if err != nil {
		return nil, nil, err
	}
	return ParseStatementsWithOptions(input, path, opts)
}

// IsValid reports whether s refers to a parsed statement.
func (s Statement) IsValid() bool { return s.stmt != nil }

// Keyword returns the verbatim statement keyword, including any extension
// prefix. The zero value returns "".
func (s Statement) Keyword() string {
	if s.stmt == nil {
		return ""
	}
	return s.stmt.Keyword
}

// Argument returns the statement argument, if one was present in source. An
// explicitly empty argument returns ("", true); an absent argument returns
// ("", false).
func (s Statement) Argument() (string, bool) {
	if s.stmt == nil || !s.stmt.HasArgument {
		return "", false
	}
	return s.stmt.Argument, true
}

// SubStatements returns direct child statements in source declaration order.
func (s Statement) SubStatements() []Statement {
	if s.stmt == nil {
		return nil
	}
	return publicStatements(s.stmt.SubStatements())
}

// Location returns a human-readable source location for diagnostics.
func (s Statement) Location() string {
	if s.stmt == nil {
		return "unknown"
	}
	return s.stmt.Location()
}

func publicStatements(stmts []*yangparse.Statement) []Statement {
	if len(stmts) == 0 {
		return nil
	}
	out := make([]Statement, len(stmts))
	for i, st := range stmts {
		out[i] = Statement{stmt: st}
	}
	return out
}
