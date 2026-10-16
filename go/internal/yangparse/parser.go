// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

// Package yangparse is Cambium's internal pure-Go YANG parser adapter.
//
// It deliberately hides parser implementation details from public Cambium code
// so the default Go API owns its parser boundary.
package yangparse

import (
	"fmt"
	"os"
	"strings"
	"unicode/utf8"
)

const (
	maxInputBytes     = 64 << 20
	maxStatementDepth = 10000
)

// ParseOptions selects optional parser relaxations. The zero value is the
// strict RFC 7950 behavior.
type ParseOptions struct {
	// AllowPatternEscapes preserves unknown backslash escapes (for example
	// \*) in double-quoted pattern statement arguments instead of rejecting
	// them. Real-world vendor modules write regex escapes that way even
	// though RFC 7950 6.1.3 only defines \n, \t, \" and \\ in double-quoted
	// strings. Each preserved escape is reported as a Warning. Escapes in
	// any other statement argument remain errors.
	AllowPatternEscapes bool
}

// Warning is a non-fatal parse diagnostic recorded while a ParseOptions
// relaxation repaired vendor source.
type Warning struct {
	File    string
	Line    int
	Column  int
	Message string
}

// Location returns the warning source location in Statement.Location format.
func (w Warning) Location() string {
	return (&Statement{file: w.File, line: w.Line, col: w.Column}).Location()
}

// Parse parses one YANG module or submodule source into ordered statements using
// Cambium's own cgo-free RFC 7950 parser (see native.go). The Statement type is
// defined in statement.go.
func Parse(input, name string) ([]*Statement, error) {
	stmts, _, err := ParseWithOptions(input, name, ParseOptions{})
	return stmts, err
}

// ParseWithOptions is Parse with explicit ParseOptions. It also returns the
// warnings recorded by any enabled relaxation, in source order.
func ParseWithOptions(input, name string, opts ParseOptions) (stmts []*Statement, warnings []Warning, err error) {
	if err := checkInputBounds(input, name); err != nil {
		return nil, nil, err
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			stmts = nil
			warnings = nil
			err = fmt.Errorf("%s: YANG parser panic: %v", displayName(name), recovered)
		}
	}()
	return parseDocument(input, name, opts)
}

// ReadFile reads a YANG source file after checking parser input bounds that can
// be known before allocating the full file contents.
func ReadFile(path string) (string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	if info.Size() > maxInputBytes {
		return "", fmt.Errorf("%s: YANG input is %d bytes, exceeds maximum %d bytes", displayName(path), info.Size(), maxInputBytes)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

// CheckInputBounds validates parser input limits (size, UTF-8, legal source
// characters, statement nesting depth) without parsing.
func CheckInputBounds(input, name string) error {
	return checkInputBounds(input, name)
}

func checkInputBounds(input, name string) error {
	if len(input) > maxInputBytes {
		return fmt.Errorf("%s: YANG input is %d bytes, exceeds maximum %d bytes", displayName(name), len(input), maxInputBytes)
	}
	if err := checkYangSourceCharacters(input, name); err != nil {
		return err
	}
	depth := 0
	for i := 0; i < len(input); {
		switch input[i] {
		case '/':
			if next := skipComment(input, i); next > i {
				i = next
				continue
			}
		case '\'', '"':
			i = skipQuoted(input, i)
			continue
		case '{':
			depth++
			if depth > maxStatementDepth {
				return fmt.Errorf("%s: YANG statement nesting depth exceeds maximum %d", displayName(name), maxStatementDepth)
			}
		case '}':
			if depth > 0 {
				depth--
			}
		}
		i++
	}
	return nil
}

func checkYangSourceCharacters(input, name string) error {
	for i := 0; i < len(input); {
		r, width := utf8.DecodeRuneInString(input[i:])
		if r == utf8.RuneError && width == 1 {
			return fmt.Errorf("%s: YANG input contains invalid UTF-8 at byte %d", displayName(name), i)
		}
		if !isLegalYangSourceCharacter(r) {
			return fmt.Errorf("%s: illegal YANG source character U+%04X at byte %d", displayName(name), r, i)
		}
		i += width
	}
	return nil
}

func isLegalYangSourceCharacter(r rune) bool {
	if r == '\t' || r == '\n' || r == '\r' {
		return true
	}
	if r < 0x20 {
		return false
	}
	if r >= 0xD800 && r <= 0xDFFF {
		return false
	}
	if r >= 0xFDD0 && r <= 0xFDEF {
		return false
	}
	if r&0xFFFF == 0xFFFE || r&0xFFFF == 0xFFFF {
		return false
	}
	return true
}

func skipComment(input string, start int) int {
	switch {
	case start+1 < len(input) && input[start] == '/' && input[start+1] == '/':
		return skipLine(input, start+2)
	case start+1 < len(input) && input[start] == '/' && input[start+1] == '*':
		if end := strings.Index(input[start+2:], "*/"); end >= 0 {
			return start + 2 + end + len("*/")
		}
	}
	return start
}

func skipLine(input string, start int) int {
	for start < len(input) && input[start] != '\n' && input[start] != '\r' {
		start++
	}
	return start
}

func skipQuoted(input string, start int) int {
	quote := input[start]
	i := start + 1
	for i < len(input) {
		if quote == '"' && input[i] == '\\' {
			i += 2
			continue
		}
		if input[i] == quote {
			return i + 1
		}
		i++
	}
	return len(input)
}

func displayName(name string) string {
	if name == "" {
		return "<input>"
	}
	return name
}
