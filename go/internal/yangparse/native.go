// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

// Clean-room RFC 7950 (YANG 1.1) statement tokenizer and recursive statement
// builder. Implemented from RFC 7950 sections 6.1 (lexical structure / quoting),
// 6.2 (identifiers), 6.3 (statements), and 14 (grammar). It emits an ordered
// Statement tree; semantic resolution (typedefs, augment, deviate, grouping
// expansion, ordering) is performed by the cambium package on top of this.

package yangparse

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// parser is a single-pass scanner over UTF-8 source. Callers must validate input
// bounds and source characters (checkInputBounds) before constructing one, so the
// scanner may assume valid UTF-8 and legal YANG source characters.
type parser struct {
	src  string
	name string
	pos  int // byte offset
	line int // 1-based
	col  int // 1-based rune column within the current line (for diagnostics)
	tcol int // 1-based tab-expanded column (tabs advance to the next multiple of 8)
}

// parseDocument scans the whole input into ordered top-level statements.
func parseDocument(input, name string) ([]*Statement, error) {
	p := &parser{src: input, name: name, line: 1, col: 1, tcol: 1}
	var stmts []*Statement
	for {
		if err := p.skipSeparators(); err != nil {
			return nil, err
		}
		if p.atEOF() {
			break
		}
		if p.peek() == '}' {
			return nil, p.errf(p.line, p.col, "unexpected '}'")
		}
		st, err := p.parseStatement()
		if err != nil {
			return nil, err
		}
		stmts = append(stmts, st)
	}
	return stmts, nil
}

func (p *parser) atEOF() bool { return p.pos >= len(p.src) }

// peek returns the current byte, or 0 at EOF. YANG structural punctuation and
// string/comment delimiters are all ASCII, so byte-level peeking is sufficient
// for control flow; rune decoding happens in advance.
func (p *parser) peek() byte {
	if p.pos >= len(p.src) {
		return 0
	}
	return p.src[p.pos]
}

func (p *parser) peek2() byte {
	if p.pos+1 >= len(p.src) {
		return 0
	}
	return p.src[p.pos+1]
}

// nextTabStop returns the 1-based column after a tab at 1-based column c, treating
// a tab as advancing to the next multiple-of-8 tab stop (RFC 7950 6.1.3).
func nextTabStop(c int) int { return c + 8 - ((c - 1) % 8) }

// advance consumes one rune and updates line/col/tcol. A bare LF, a bare CR, and a
// CRLF pair each count as exactly one line break for position tracking.
func (p *parser) advance() rune {
	r, sz := utf8.DecodeRuneInString(p.src[p.pos:])
	p.pos += sz
	switch r {
	case '\n':
		p.line++
		p.col = 1
		p.tcol = 1
	case '\r':
		if p.pos < len(p.src) && p.src[p.pos] == '\n' {
			p.pos++
		}
		p.line++
		p.col = 1
		p.tcol = 1
	case '\t':
		p.col++
		p.tcol = nextTabStop(p.tcol)
	default:
		p.col++
		p.tcol++
	}
	return r
}

// skipSeparators consumes runs of whitespace, line comments (//), and block
// comments (/* */). Comment delimiters are recognized only here, never inside a
// quoted string scan.
func (p *parser) skipSeparators() error {
	for !p.atEOF() {
		c := p.peek()
		switch {
		case c == ' ' || c == '\t' || c == '\r' || c == '\n':
			p.advance()
		case c == '/' && p.peek2() == '/':
			p.advance()
			p.advance()
			for !p.atEOF() && p.peek() != '\n' && p.peek() != '\r' {
				p.advance()
			}
		case c == '/' && p.peek2() == '*':
			startLine, startCol := p.line, p.col
			p.advance()
			p.advance()
			for {
				if p.atEOF() {
					return p.errf(startLine, startCol, "unterminated block comment")
				}
				if p.peek() == '*' && p.peek2() == '/' {
					p.advance()
					p.advance()
					break
				}
				p.advance()
			}
		default:
			return nil
		}
	}
	return nil
}

// parseStatement parses one statement and its block, recursively.
func (p *parser) parseStatement() (*Statement, error) {
	kwLine, kwCol := p.line, p.col
	keyword, err := p.scanKeyword()
	if err != nil {
		return nil, err
	}
	st := &Statement{Keyword: keyword, file: p.name, line: kwLine, col: kwCol}

	if err := p.skipSeparators(); err != nil {
		return nil, err
	}

	// An argument is present iff the next significant token is neither ';' nor '{'.
	if c := p.peek(); !p.atEOF() && c != ';' && c != '{' {
		arg, err := p.scanArgument()
		if err != nil {
			return nil, err
		}
		st.Argument = arg
		st.HasArgument = true
		if err := p.skipSeparators(); err != nil {
			return nil, err
		}
	}

	switch p.peek() {
	case ';':
		p.advance()
	case '{':
		p.advance()
		for {
			if err := p.skipSeparators(); err != nil {
				return nil, err
			}
			if p.atEOF() {
				return nil, p.errf(kwLine, kwCol, "unterminated block, expected '}'")
			}
			if p.peek() == '}' {
				p.advance()
				break
			}
			child, err := p.parseStatement()
			if err != nil {
				return nil, err
			}
			st.statements = append(st.statements, child)
		}
	default:
		return nil, p.errf(p.line, p.col, "expected ';' or '{' after statement %q", keyword)
	}
	return st, nil
}

// scanKeyword reads a bareword keyword (possibly prefix:identifier). Keywords are
// never quoted in conformant input.
func (p *parser) scanKeyword() (string, error) {
	startLine, startCol := p.line, p.col
	start := p.pos
	for !p.atEOF() {
		c := p.peek()
		if isSeparatorByte(c) || c == ';' || c == '{' || c == '}' {
			break
		}
		if c == '/' && (p.peek2() == '/' || p.peek2() == '*') {
			break
		}
		if c == '"' || c == '\'' {
			break
		}
		p.advance()
	}
	if p.pos == start {
		return "", p.errf(startLine, startCol, "expected statement keyword")
	}
	return p.src[start:p.pos], nil
}

// scanArgument reads one string, then resolves YANG '+' string concatenation.
// Per RFC 7950 6.1.3 concatenation joins QUOTED strings only; an unquoted operand
// does not participate, so a '+' after an unquoted argument is left for the caller
// (which will surface the expected ';'/'{' error, matching the reference parser).
func (p *parser) scanArgument() (string, error) {
	value, quoted, err := p.scanString()
	if err != nil {
		return "", err
	}
	if !quoted {
		return value, nil
	}
	for {
		mark := *p
		if err := p.skipSeparators(); err != nil {
			return "", err
		}
		if p.peek() != '+' {
			*p = mark // not a concatenation; leave separators for the caller
			return value, nil
		}
		p.advance() // consume '+'
		if err := p.skipSeparators(); err != nil {
			return "", err
		}
		next, nextQuoted, err := p.scanString()
		if err != nil {
			return "", err
		}
		if !nextQuoted {
			return "", p.errf(p.line, p.col, "expected a quoted string after '+' concatenation operator")
		}
		value += next
	}
}

// scanString reads one string operand and reports whether it was quoted.
func (p *parser) scanString() (value string, quoted bool, err error) {
	switch p.peek() {
	case '"':
		s, err := p.scanDoubleQuoted()
		return s, true, err
	case '\'':
		s, err := p.scanSingleQuoted()
		return s, true, err
	default:
		s, err := p.scanUnquoted()
		return s, false, err
	}
}

// scanUnquoted reads a maximal unquoted run. It terminates at whitespace, the
// structural punctuation ;{} , quote characters, or a comment opener. A quote
// or standalone comment-close "*/" inside an unquoted run fails closed.
func (p *parser) scanUnquoted() (string, error) {
	startLine, startCol := p.line, p.col
	start := p.pos
	for !p.atEOF() {
		c := p.peek()
		if isSeparatorByte(c) || c == ';' || c == '{' || c == '}' {
			break
		}
		if c == '/' && (p.peek2() == '/' || p.peek2() == '*') {
			break
		}
		if c == '"' || c == '\'' {
			return "", p.errf(p.line, p.col, "unquoted argument contains quote character %q", c)
		}
		if c == '*' && p.peek2() == '/' {
			return "", p.errf(p.line, p.col, "unquoted argument contains comment-close sequence '*/'")
		}
		p.advance()
	}
	if p.pos == start {
		return "", p.errf(startLine, startCol, "expected argument")
	}
	return p.src[start:p.pos], nil
}

// scanSingleQuoted reads a single-quoted string: every character is literal, no
// escapes, no trimming. The first closing quote ends it.
func (p *parser) scanSingleQuoted() (string, error) {
	startLine, startCol := p.line, p.col
	p.advance() // opening '
	start := p.pos
	for !p.atEOF() {
		if p.peek() == '\'' {
			value := p.src[start:p.pos]
			p.advance() // closing '
			return value, nil
		}
		p.advance()
	}
	return "", p.errf(startLine, startCol, "unterminated single-quoted string")
}

// scanDoubleQuoted reads a double-quoted string, applying RFC 7950 6.1.3 escape
// processing and whitespace trimming. Only the four escapes \n \t \" \\ are valid;
// any other escape fails closed. Only a line feed (LF) is a line break for the
// trimming algorithm; a bare carriage return is ordinary content (matching the
// reference parser). Per-line trailing whitespace is stripped before each line
// break, and continuation-line leading whitespace is stripped up to the
// tab-expanded column of the opening quote.
func (p *parser) scanDoubleQuoted() (string, error) {
	startLine, startCol := p.line, p.col
	quoteCol := p.tcol
	p.advance() // opening "

	var texts []string
	var endings []string
	var cur strings.Builder
	for {
		if p.atEOF() {
			return "", p.errf(startLine, startCol, "unterminated double-quoted string")
		}
		switch p.peek() {
		case '"':
			p.advance()
			texts = append(texts, cur.String())
			return trimDoubleQuoted(texts, endings, quoteCol), nil
		case '\\':
			p.advance() // backslash
			if p.atEOF() {
				return "", p.errf(startLine, startCol, "unterminated double-quoted string")
			}
			switch e := p.peek(); e {
			case 'n':
				cur.WriteByte('\n')
			case 't':
				cur.WriteByte('\t')
			case '"':
				cur.WriteByte('"')
			case '\\':
				cur.WriteByte('\\')
			default:
				return "", p.errf(p.line, p.col, "invalid escape sequence: \\%c", rune(e))
			}
			p.advance()
		case '\n':
			p.advance()
			texts = append(texts, cur.String())
			endings = append(endings, "\n")
			cur.Reset()
		default:
			// Ordinary rune. Consume exactly one rune WITHOUT the CRLF coalescing
			// that advance() performs: a bare CR is ordinary content, and the LF of
			// a CRLF must remain to be seen as the line break on the next iteration.
			r, sz := utf8.DecodeRuneInString(p.src[p.pos:])
			p.pos += sz
			cur.WriteRune(r)
			if r == '\t' {
				p.tcol = nextTabStop(p.tcol)
			} else {
				p.tcol++
			}
			p.col++
		}
	}
}

// trimDoubleQuoted applies the RFC 7950 6.1.3 whitespace rules across the source
// lines of a double-quoted string, preserving each original line-break sequence.
func trimDoubleQuoted(texts, endings []string, quoteCol int) string {
	var b strings.Builder
	for i, line := range texts {
		if i < len(texts)-1 {
			// Followed by a line break: strip trailing whitespace.
			line = strings.TrimRight(line, " \t")
		}
		if i > 0 {
			// Continuation line: strip leading indentation up to the quote column.
			line = stripLeadingColumns(line, quoteCol)
		}
		b.WriteString(line)
		if i < len(endings) {
			b.WriteString(endings[i])
		}
	}
	return b.String()
}

// stripLeadingColumns removes leading spaces/tabs spanning up to indent visual
// columns (a tab advances to the next multiple of 8), or until the first
// non-whitespace character. A whitespace character that would carry the column
// past indent is left in place (RFC 7950 6.1.3: strip up to and including the
// quote column, never beyond it).
func stripLeadingColumns(s string, indent int) string {
	cols := 0
	i := 0
	for i < len(s) {
		switch s[i] {
		case ' ':
			if cols+1 > indent {
				return s[i:]
			}
			cols++
		case '\t':
			next := cols + (8 - cols%8)
			if next > indent {
				return s[i:]
			}
			cols = next
		default:
			return s[i:]
		}
		i++
	}
	return s[i:]
}

func isSeparatorByte(c byte) bool {
	return c == ' ' || c == '\t' || c == '\r' || c == '\n'
}

func (p *parser) errf(line, col int, format string, args ...any) error {
	loc := (&Statement{file: p.name, line: line, col: col}).Location()
	return fmt.Errorf("%s: %s", loc, fmt.Sprintf(format, args...))
}
