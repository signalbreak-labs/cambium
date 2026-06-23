// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

package cambium

import "github.com/signalbreak-labs/cambium/go/internal/yangparse"

// SourceLocation is a structured source position. Text preserves Cambium's
// established human-readable location string; File, Line, and Column expose the
// parser's raw position when available.
type SourceLocation struct {
	File   string
	Line   int
	Column int
	Text   string
}

func sourceLocation(st *yangparse.Statement) SourceLocation {
	if st == nil {
		return SourceLocation{Text: "unknown"}
	}
	file, line, col := st.Position()
	return SourceLocation{
		File:   file,
		Line:   line,
		Column: col,
		Text:   st.Location(),
	}
}

// SourceLocation returns the module statement's structured source location.
func (m Module) SourceLocation() SourceLocation {
	if m.mod == nil {
		return SourceLocation{Text: "unknown"}
	}
	return sourceLocation(m.mod.stmt)
}

// SourceLocation returns the schema node statement's structured source location.
func (n SchemaNodeRef) SourceLocation() SourceLocation {
	if n.node == nil {
		return SourceLocation{Text: "unknown"}
	}
	return sourceLocation(n.node.stmt)
}

// SourceLocation returns the identity statement's structured source location.
func (i Identity) SourceLocation() SourceLocation {
	if i.id == nil {
		return SourceLocation{Text: "unknown"}
	}
	return sourceLocation(i.id.stmt)
}
