// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

package compat

// ErrorNode is a goyang-style AST node that carries an error.
type ErrorNode struct {
	Parent Node `yang:"Parent,nomerge"`

	Error error
}

// Kind reports the node kind, always "error".
func (ErrorNode) Kind() string { return "error" }

// ParentNode returns the enclosing AST node.
func (s *ErrorNode) ParentNode() Node { return s.Parent }

// NName returns "error".
func (s *ErrorNode) NName() string { return "error" }

// Statement returns an empty placeholder statement.
func (s *ErrorNode) Statement() *Statement { return &Statement{} }

// Exts returns nil; an error node carries no extensions.
func (s *ErrorNode) Exts() []*Statement { return nil }
