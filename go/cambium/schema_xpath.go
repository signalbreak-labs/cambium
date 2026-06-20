package cambium

import (
	"fmt"
	"strings"

	"github.com/signalbreak-labs/cambium/go/internal/yangparse"
)

func (m *moduleData) validateXPathExpressionPrefixes(kind string, st *yangparse.Statement) error {
	if m == nil || st == nil {
		return nil
	}
	for _, prefix := range referencedPrefixes(st.Argument) {
		if m.resolveSourceQNameModule(prefix+":_") == nil {
			return fmt.Errorf("unknown prefix %q in %s expression %q at %s", prefix, kind, st.Argument, st.Location())
		}
	}
	return nil
}

func validateXPathExpressionStatement(keyword string, st *yangparse.Statement) error {
	if strings.TrimSpace(st.Argument) == "" {
		return fmt.Errorf("%s expression is empty at %s", keyword, st.Location())
	}
	if !structurallyValidXPathExpression(st.Argument) {
		return fmt.Errorf("invalid %s expression %q at %s", keyword, st.Argument, st.Location())
	}
	return nil
}

func structurallyValidXPathExpression(expr string) bool {
	brackets := 0
	parens := 0
	var quote rune
	for _, r := range expr {
		if quote != 0 {
			if r == quote {
				quote = 0
			}
			continue
		}
		switch r {
		case '\'', '"':
			quote = r
		case '[':
			brackets++
		case ']':
			if brackets == 0 {
				return false
			}
			brackets--
		case '(':
			parens++
		case ')':
			if parens == 0 {
				return false
			}
			parens--
		}
	}
	return quote == 0 && brackets == 0 && parens == 0 && lexicallyValidXPathExpression(expr)
}

func lexicallyValidXPathExpression(expr string) bool {
	return xpathAxesAreKnown(expr) && xpathFunctionsHaveKnownArity(expr) && xpathBinaryOperatorsHaveOperands(expr)
}

func xpathAxesAreKnown(expr string) bool {
	for i := 0; i < len(expr); i++ {
		if expr[i] == '\'' || expr[i] == '"' {
			next, ok := skipXPathQuoted(expr, i)
			if !ok {
				return false
			}
			i = next
			continue
		}
		if i+1 >= len(expr) || expr[i] != ':' || expr[i+1] != ':' {
			continue
		}
		start := i - 1
		for start >= 0 && isXPathNameChar(expr[start]) {
			start--
		}
		axis := expr[start+1 : i]
		if axis == "" {
			return false
		}
		if _, ok := xpathKnownAxes[axis]; !ok {
			return false
		}
		i++
	}
	return true
}

type xpathFunctionArity struct {
	min int
	max int
}

func xpathFunctionsHaveKnownArity(expr string) bool {
	for i := 0; i < len(expr); {
		if expr[i] == '\'' || expr[i] == '"' {
			next, ok := skipXPathQuoted(expr, i)
			if !ok {
				return false
			}
			i = next + 1
			continue
		}
		if !isXPathNameStart(expr[i]) {
			i++
			continue
		}
		start := i
		i++
		for i < len(expr) && isXPathNameChar(expr[i]) {
			i++
		}
		name := expr[start:i]
		j := i
		for j < len(expr) && isYANGSpace(expr[j]) {
			j++
		}
		if j >= len(expr) || expr[j] != '(' {
			continue
		}
		arity, known := xpathFunctionArities[name]
		if !known {
			return false
		}
		end, ok := matchingXPathParen(expr, j)
		if !ok {
			return false
		}
		count, ok := countXPathFunctionArgs(expr[j+1 : end])
		if !ok {
			return false
		}
		if count < arity.min {
			return false
		}
		if arity.max != xpathVariadic && count > arity.max {
			return false
		}
	}
	return true
}

func countXPathFunctionArgs(args string) (int, bool) {
	if strings.TrimSpace(args) == "" {
		return 0, true
	}
	count := 1
	parens := 0
	brackets := 0
	segmentHasContent := false
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case '\'', '"':
			next, ok := skipXPathQuoted(args, i)
			if !ok {
				return 0, false
			}
			segmentHasContent = true
			i = next
		case '(':
			parens++
			segmentHasContent = true
		case ')':
			if parens == 0 {
				return 0, false
			}
			parens--
			segmentHasContent = true
		case '[':
			brackets++
			segmentHasContent = true
		case ']':
			if brackets == 0 {
				return 0, false
			}
			brackets--
			segmentHasContent = true
		case ',':
			if parens != 0 || brackets != 0 {
				segmentHasContent = true
				continue
			}
			if !segmentHasContent {
				return 0, false
			}
			count++
			segmentHasContent = false
		default:
			if !isYANGSpace(args[i]) {
				segmentHasContent = true
			}
		}
	}
	if parens != 0 || brackets != 0 || !segmentHasContent {
		return 0, false
	}
	return count, true
}

type xpathSyntaxTokenKind int

type xpathSyntaxToken struct {
	kind xpathSyntaxTokenKind
}

func xpathBinaryOperatorsHaveOperands(expr string) bool {
	tokens, ok := xpathSyntaxTokens(expr)
	if !ok || len(tokens) == 0 {
		return false
	}
	expectOperand := true
	seenOperand := false
	for _, token := range tokens {
		switch token.kind {
		case xpathSyntaxOperand:
			expectOperand = false
			seenOperand = true
		case xpathSyntaxOperator:
			if expectOperand {
				return false
			}
			expectOperand = true
		}
	}
	return seenOperand && !expectOperand
}

func xpathSyntaxTokens(expr string) ([]xpathSyntaxToken, bool) {
	var tokens []xpathSyntaxToken
	for i := 0; i < len(expr); {
		ch := expr[i]
		switch {
		case isYANGSpace(ch):
			i++
		case ch == '\'' || ch == '"':
			next, ok := skipXPathQuoted(expr, i)
			if !ok {
				return nil, false
			}
			tokens = append(tokens, xpathSyntaxToken{kind: xpathSyntaxOperand})
			i = next + 1
		case ch == '(' || ch == ')' || ch == '[' || ch == ']' || ch == ',':
			i++
		case isXPathSymbolicOperatorStart(ch):
			if ch == '!' && (i+1 >= len(expr) || expr[i+1] != '=') {
				return nil, false
			}
			tokens = append(tokens, xpathSyntaxToken{kind: xpathSyntaxOperator})
			if i+1 < len(expr) && (ch == '!' || ch == '<' || ch == '>') && expr[i+1] == '=' {
				i += 2
			} else {
				i++
			}
		case isXPathNameStart(ch):
			start := i
			i++
			for i < len(expr) && isXPathNameChar(expr[i]) {
				i++
			}
			if isXPathWordOperator(expr[start:i]) {
				tokens = append(tokens, xpathSyntaxToken{kind: xpathSyntaxOperator})
			} else {
				tokens = append(tokens, xpathSyntaxToken{kind: xpathSyntaxOperand})
			}
		default:
			start := i
			for i < len(expr) && !isYANGSpace(expr[i]) && expr[i] != '\'' && expr[i] != '"' &&
				expr[i] != '(' && expr[i] != ')' && expr[i] != '[' && expr[i] != ']' &&
				expr[i] != ',' && !isXPathSymbolicOperatorStart(expr[i]) {
				i++
			}
			if i == start {
				return nil, false
			}
			tokens = append(tokens, xpathSyntaxToken{kind: xpathSyntaxOperand})
		}
	}
	return tokens, true
}

func skipXPathQuoted(expr string, start int) (int, bool) {
	quote := expr[start]
	for i := start + 1; i < len(expr); i++ {
		if expr[i] == quote {
			return i, true
		}
	}
	return 0, false
}

func matchingXPathParen(expr string, start int) (int, bool) {
	depth := 0
	for i := start; i < len(expr); i++ {
		switch expr[i] {
		case '\'', '"':
			next, ok := skipXPathQuoted(expr, i)
			if !ok {
				return 0, false
			}
			i = next
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return i, true
			}
			if depth < 0 {
				return 0, false
			}
		}
	}
	return 0, false
}

func isXPathSymbolicOperatorStart(ch byte) bool {
	switch ch {
	case '=', '<', '>', '!', '+', '|':
		return true
	default:
		return false
	}
}

func isXPathWordOperator(word string) bool {
	switch word {
	case "and", "or", "div", "mod":
		return true
	default:
		return false
	}
}

func isXPathNameStart(ch byte) bool {
	return ch >= 'A' && ch <= 'Z' || ch >= 'a' && ch <= 'z' || ch == '_'
}

func isXPathNameChar(ch byte) bool {
	return isXPathNameStart(ch) || ch >= '0' && ch <= '9' || ch == '-' || ch == '.' || ch == ':'
}
