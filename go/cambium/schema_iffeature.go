// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

package cambium

import (
	"fmt"
	"strings"

	"github.com/signalbreak-labs/cambium/go/internal/yangparse"
)

func (m *moduleData) ifFeaturePlacementRequiresYang11(st *yangparse.Statement) bool {
	if st == nil || st.Keyword != "if-feature" {
		return false
	}
	parent := m.statementParents[st]
	if parent == nil {
		return false
	}
	switch parent.Keyword {
	case "bit", "enum", "identity", "refine":
		return true
	default:
		return false
	}
}

func ifFeatureExprRequiresYang11(expr string) bool {
	if _, ok := singleIfFeatureRefArg(expr, false); ok {
		return false
	}
	tokens, ok := tokenizeIfFeatureExpr(expr)
	if !ok {
		return false
	}
	return len(tokens) != 1 || tokens[0].kind != ifFeatureTokenIdent
}

func prependIfFeatures(roots []*schemaNodeData, features []string) {
	if len(features) == 0 {
		return
	}
	var walk func(*schemaNodeData)
	walk = func(n *schemaNodeData) {
		if n == nil {
			return
		}
		prefix := append([]string(nil), features...)
		n.ifFeatures = append(prefix, n.ifFeatures...)
		for _, child := range n.children {
			walk(child)
		}
	}
	for _, root := range roots {
		walk(root)
	}
}

func ifFeatureArgs(st *yangparse.Statement) []string {
	children := direct(st, "if-feature")
	if len(children) == 0 {
		return nil
	}
	out := make([]string, len(children))
	for i, child := range children {
		out[i] = child.Argument
	}
	return out
}

func (m *moduleData) validateIfFeatureExpressions() {
	if m == nil || m.schemaErr != nil {
		return
	}
	for _, top := range m.sourceTopStatements() {
		walkStatements(top, func(st *yangparse.Statement) {
			if m.schemaErr != nil {
				return
			}
			for _, iff := range direct(st, "if-feature") {
				if !m.validateIfFeatureExprFrom(iff.Argument, iff) {
					m.recordSchemaError(fmt.Errorf("invalid or unresolved if-feature expression %q at %s", iff.Argument, iff.Location()))
					return
				}
			}
		})
	}
}

func (m *moduleData) evalIfFeatureExpr(expr string) (bool, bool) {
	return m.evalIfFeatureExprFrom(expr, nil)
}

func (m *moduleData) evalIfFeatureExprFrom(expr string, from *yangparse.Statement) (bool, bool) {
	return m.evalIfFeatureExprSeen(expr, from, make(map[string]bool))
}

func (m *moduleData) evalIfFeatureExprSeen(expr string, from *yangparse.Statement, resolving map[string]bool) (bool, bool) {
	if ref, ok := m.yang10SingleIfFeatureRef(expr, from); ok {
		return m.featureEnabledSeen(ref, from, resolving)
	}
	tokens, ok := tokenizeIfFeatureExpr(expr)
	if !ok {
		return false, false
	}
	parser := ifFeatureParser{mod: m, from: from, tokens: tokens, resolving: resolving}
	value, ok := parser.parseOr()
	if !ok || parser.pos != len(tokens) {
		return false, false
	}
	return value, true
}

func (m *moduleData) validateIfFeatureExprFrom(expr string, from *yangparse.Statement) bool {
	if ref, ok := m.yang10SingleIfFeatureRef(expr, from); ok {
		return m.validateFeatureRefSeen(ref, from, make(map[string]bool))
	}
	tokens, ok := tokenizeIfFeatureExpr(expr)
	if !ok {
		return false
	}
	parser := ifFeatureParser{mod: m, from: from, tokens: tokens, resolving: make(map[string]bool), validateOnly: true}
	_, ok = parser.parseOr()
	return ok && parser.pos == len(tokens)
}

type ifFeatureTokenKind int

type ifFeatureToken struct {
	kind ifFeatureTokenKind
	text string
}

func tokenizeIfFeatureExpr(expr string) ([]ifFeatureToken, bool) {
	expr = yangTrimSpace(expr)
	if expr == "" {
		return nil, false
	}
	var tokens []ifFeatureToken
	for i := 0; i < len(expr); {
		ch := expr[i]
		switch ch {
		case ' ', '\t', '\r', '\n':
			i++
		case '(':
			tokens = append(tokens, ifFeatureToken{kind: ifFeatureTokenLParen})
			i++
		case ')':
			tokens = append(tokens, ifFeatureToken{kind: ifFeatureTokenRParen})
			i++
		default:
			start := i
			for i < len(expr) && !isIfFeatureExprSeparator(expr[i]) {
				i++
			}
			word := expr[start:i]
			switch word {
			case "not":
				tokens = append(tokens, ifFeatureToken{kind: ifFeatureTokenNot})
			case "and":
				tokens = append(tokens, ifFeatureToken{kind: ifFeatureTokenAnd})
			case "or":
				tokens = append(tokens, ifFeatureToken{kind: ifFeatureTokenOr})
			default:
				if !validIfFeatureRef(word) {
					return nil, false
				}
				tokens = append(tokens, ifFeatureToken{kind: ifFeatureTokenIdent, text: word})
			}
		}
	}
	if len(tokens) == 0 {
		return nil, false
	}
	return tokens, true
}

func singleIfFeatureRefArg(expr string, allowXMLPrefix bool) (string, bool) {
	ref := yangTrimSpace(expr)
	if !validYangIdentifierRef(ref, allowXMLPrefix) {
		return "", false
	}
	return ref, true
}

func isIfFeatureExprSeparator(ch byte) bool {
	return ch == ' ' || ch == '\t' || ch == '\r' || ch == '\n' || ch == '(' || ch == ')'
}

func validIfFeatureRef(ref string) bool {
	if ref == "" || strings.Count(ref, ":") > 1 {
		return false
	}
	parts := strings.Split(ref, ":")
	for _, part := range parts {
		if part == "" {
			return false
		}
		for i := 0; i < len(part); i++ {
			ch := part[i]
			if ch >= 'a' && ch <= 'z' || ch >= 'A' && ch <= 'Z' || ch >= '0' && ch <= '9' || ch == '_' || ch == '-' || ch == '.' {
				continue
			}
			return false
		}
	}
	return true
}

type ifFeatureParser struct {
	mod          *moduleData
	from         *yangparse.Statement
	tokens       []ifFeatureToken
	pos          int
	resolving    map[string]bool
	validateOnly bool
}

func (m *moduleData) validateIfFeatureExprSeen(expr string, from *yangparse.Statement, resolving map[string]bool) bool {
	tokens, ok := tokenizeIfFeatureExpr(expr)
	if !ok {
		return false
	}
	parser := ifFeatureParser{mod: m, from: from, tokens: tokens, resolving: resolving, validateOnly: true}
	_, ok = parser.parseOr()
	return ok && parser.pos == len(tokens)
}
