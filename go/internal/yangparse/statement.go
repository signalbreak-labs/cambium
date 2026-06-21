// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

package yangparse

import "fmt"

// Statement is one parsed YANG statement with ordered substatements.
//
// It is Cambium-owned: the default schema/codegen tier carries its own YANG
// front-end and does not depend on any external parser. Statements are produced
// only by Parse and are treated as read-only by the core; their pointer identity
// is stable for the lifetime of a parse (the core uses *Statement as a map key).
type Statement struct {
	// Keyword is the verbatim statement keyword, including any extension prefix
	// (e.g. "leaf", "type", or "md:annotation"). Never normalized.
	Keyword string
	// HasArgument reports whether an argument token was present in source. When
	// false, Argument is "". When true, Argument holds the (possibly empty) value.
	HasArgument bool
	// Argument is the statement argument with quotes removed and YANG string
	// concatenation ('+') already resolved. Multi-line values carry embedded '\n'.
	Argument string

	// statements are the direct children in effective declaration (source) order.
	// Never sorted or map-derived; order is load-bearing for invariants I1-I6.
	statements []*Statement

	file string
	line int // 1-based
	col  int // 1-based
}

// SubStatements returns the direct child statements in effective declaration
// (source) order. The returned slice and its element pointers are stable across
// calls. Terminal statements return nil.
func (s *Statement) SubStatements() []*Statement { return s.statements }

// Location returns a human-readable source location for diagnostics. It is never
// parsed; the format matches Cambium's established error wording.
func (s *Statement) Location() string {
	switch {
	case s.file == "" && s.line == 0:
		return "unknown"
	case s.file == "":
		return fmt.Sprintf("line %d:%d", s.line, s.col)
	case s.line == 0:
		return s.file
	default:
		return fmt.Sprintf("%s:%d:%d", s.file, s.line, s.col)
	}
}

// Position returns the raw source position components used by Location.
func (s *Statement) Position() (file string, line, col int) {
	if s == nil {
		return "", 0, 0
	}
	return s.file, s.line, s.col
}
