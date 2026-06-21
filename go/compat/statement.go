// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

package compat

import (
	"fmt"
	"io"
	"strings"
)

// Statement is a generic parsed YANG statement.
type Statement struct {
	Keyword     string
	HasArgument bool
	Argument    string
	statements  []*Statement

	file string
	line int
	col  int
}

// NName returns the statement argument.
func (s *Statement) NName() string {
	if s == nil {
		return ""
	}
	return s.Argument
}

// Kind returns the statement keyword.
func (s *Statement) Kind() string {
	if s == nil {
		return ""
	}
	return s.Keyword
}

// Statement returns s itself.
func (s *Statement) Statement() *Statement { return s }

// ParentNode returns nil; a statement does not track its parent.
func (s *Statement) ParentNode() Node { return nil }

// Exts returns nil; a statement carries no separate extension list.
func (s *Statement) Exts() []*Statement { return nil }

// Arg returns the optional argument to s. It returns false if s has no argument.
func (s *Statement) Arg() (string, bool) {
	if s == nil {
		return "", false
	}
	return s.Argument, s.HasArgument
}

// SubStatements returns the direct child statements found in s.
func (s *Statement) SubStatements() []*Statement {
	if s == nil {
		return nil
	}
	return s.statements
}

// Location returns the location in the source where s was defined.
func (s *Statement) Location() string {
	if s == nil {
		return "unknown"
	}
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

// Write writes the tree in s to w, each line indented by indent.
func (s *Statement) Write(w io.Writer, indent string) error {
	if s == nil {
		return nil
	}
	if s.Keyword == "" {
		for _, child := range s.statements {
			if err := child.Write(w, indent); err != nil {
				return err
			}
		}
		return nil
	}

	parts := []string{fmt.Sprintf("%s%s", indent, s.Keyword)}
	if s.HasArgument {
		args := strings.Split(s.Argument, "\n")
		if len(args) == 1 {
			parts = append(parts, fmt.Sprintf(" %q", s.Argument))
		} else {
			parts = append(parts, ` "`, args[0], "\n")
			padding := fmt.Sprintf("%*s", len(s.Keyword)+1, "")
			for i, part := range args[1:] {
				quoted := fmt.Sprintf("%q", part)
				quoted = quoted[1 : len(quoted)-1]
				parts = append(parts, indent, " ", padding, quoted)
				if i == len(args[1:])-1 {
					parts = append(parts, `"`)
				} else {
					parts = append(parts, "\n")
				}
			}
		}
	}

	if len(s.statements) == 0 {
		_, err := fmt.Fprintf(w, "%s;\n", strings.Join(parts, ""))
		return err
	}
	if _, err := fmt.Fprintf(w, "%s {\n", strings.Join(parts, "")); err != nil {
		return err
	}
	for _, child := range s.statements {
		if err := child.Write(w, indent+"\t"); err != nil {
			return err
		}
	}
	_, err := fmt.Fprintf(w, "%s}\n", indent)
	return err
}
