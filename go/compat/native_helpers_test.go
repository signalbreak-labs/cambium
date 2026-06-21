// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

package compat_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCompatSmallHelpersDoNotDelegateToUpstream(t *testing.T) {
	source := readCompatSource(t, "ast.go")
	for _, forbidden := range []string{
		"upstream.Source(",
		"upstream.NodePath(",
		"upstream.FindModuleByPrefix(",
		"upstream.Parse(",
		"upstream/indent",
		"BaseTypedefs = upstream.BaseTypedefs",
		"case *upstream.Value:",
		"type ErrorNode = upstream.ErrorNode",
		"type Typedefer = upstream.Typedefer",
		"type ASTValue = upstream.Value",
		"type ASTModule = upstream.Module",
		"type ASTIdentity = upstream.Identity",
		"type Type = upstream.Type",
	} {
		if strings.Contains(source, forbidden) {
			t.Fatalf("compat ast.go still delegates helper %q to upstream", forbidden)
		}
	}
}

func readCompatSource(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(repoRoot(t), "go", "compat", name))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
