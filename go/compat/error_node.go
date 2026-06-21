// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

package compat

// ErrorNode is a goyang-style AST node that carries an error.
type ErrorNode struct {
	Parent Node `yang:"Parent,nomerge"`

	Error error
}

func (ErrorNode) Kind() string             { return "error" }
func (s *ErrorNode) ParentNode() Node      { return s.Parent }
func (s *ErrorNode) NName() string         { return "error" }
func (s *ErrorNode) Statement() *Statement { return &Statement{} }
func (s *ErrorNode) Exts() []*Statement    { return nil }
