// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

package yang

// CambiumInternalStatement constructs an upstream-shaped Statement from
// Cambium's native parser output. It is exported only because compat lives in a
// different package; it is not part of the goyang-compatible public surface.
func CambiumInternalStatement(keyword string, hasArgument bool, argument string, children []*Statement, file string, line, col int) *Statement {
	return &Statement{
		Keyword:     keyword,
		HasArgument: hasArgument,
		Argument:    argument,
		statements:  append([]*Statement(nil), children...),
		file:        file,
		line:        line,
		col:         col,
	}
}
