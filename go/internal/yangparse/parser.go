// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

// Package yangparse is Cambium's internal pure-Go YANG parser adapter.
//
// It deliberately hides the vendored upstream parser package from public
// Cambium code so the default Go API owns its parser boundary.
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

// Parse parses one YANG module or submodule source into ordered statements using
// Cambium's own cgo-free RFC 7950 parser (see native.go). The Statement type is
// defined in statement.go.
func Parse(input, name string) (stmts []*Statement, err error) {
	if err := checkInputBounds(input, name); err != nil {
		return nil, err
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			stmts = nil
			err = fmt.Errorf("%s: YANG parser panic: %v", displayName(name), recovered)
		}
	}()
	return parseDocument(input, name)
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
// characters, statement nesting depth) without parsing. It is exported so the
// goyang-compat layer can apply identical pre-parse safety to its upstream-backed
// parse path while keeping this package free of any upstream dependency.
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
