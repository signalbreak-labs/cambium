// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

package compat

import (
	"github.com/signalbreak-labs/cambium/go/internal/yangparse"
)

func compatStatementsFromNative(in []*yangparse.Statement) []*Statement {
	if len(in) == 0 {
		return nil
	}
	out := make([]*Statement, 0, len(in))
	for _, stmt := range in {
		if converted := compatStatementFromNative(stmt); converted != nil {
			out = append(out, converted)
		}
	}
	return out
}

func compatStatementFromNative(in *yangparse.Statement) *Statement {
	if in == nil {
		return nil
	}
	file, line, col := in.Position()
	return &Statement{
		Keyword:     in.Keyword,
		HasArgument: in.HasArgument,
		Argument:    in.Argument,
		statements:  compatStatementsFromNative(in.SubStatements()),
		file:        file,
		line:        line,
		col:         col,
	}
}
