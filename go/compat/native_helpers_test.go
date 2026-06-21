// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

package compat_test

import (
	"os"
	"os/exec"
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
		"type Node = upstream.Node",
		"type Statement = upstream.Statement",
	} {
		if strings.Contains(source, forbidden) {
			t.Fatalf("compat ast.go still delegates helper %q to upstream", forbidden)
		}
	}
}

func TestCompatHasNoVendoredGoyangDependency(t *testing.T) {
	cmd := exec.Command("go", "list", "-deps", "./compat")
	cmd.Dir = filepath.Join(repoRoot(t), "go")
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go list compat dependency closure failed:\n%s", out)
	}
	const vendoredGoyang = "github.com/signalbreak-labs/cambium/go/internal/yangparse/upstream"
	for _, dep := range strings.Fields(string(out)) {
		if strings.HasPrefix(dep, vendoredGoyang) {
			t.Fatalf("compat depends on vendored goyang package %q; production compat must use Cambium-native code\n%s", dep, out)
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
