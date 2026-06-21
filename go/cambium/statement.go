// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

package cambium

import "github.com/signalbreak-labs/cambium/go/internal/yangparse"

// Statement is a read-only handle to one parsed YANG statement.
//
// It is Cambium's native raw statement surface: useful for lightweight source
// inspection, extension-aware tooling, and migration code that needs a generic
// YANG statement tree without importing the goyang-shaped compat package.
// Semantic schema resolution lives on Context/Module/SchemaNodeRef.
type Statement struct{ stmt *yangparse.Statement }

// ParseStatements parses one YANG source document into ordered top-level
// statements using Cambium's native RFC 7950 parser.
func ParseStatements(input, name string) ([]Statement, error) {
	stmts, err := yangparse.Parse(input, name)
	if err != nil {
		return nil, err
	}
	return publicStatements(stmts), nil
}

// ReadStatements reads path and parses it into ordered top-level statements.
func ReadStatements(path string) ([]Statement, error) {
	input, err := yangparse.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return ParseStatements(input, path)
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
