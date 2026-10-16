// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

package compat_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/signalbreak-labs/cambium/go/compat"
)

// Vendor modules write regex escapes such as \* directly in double-quoted
// pattern arguments. The compat default stays strict; the existing
// IgnoreSubmoduleCircularDependencies option, documented as selecting
// vendor-compatible loading, also selects pattern-escape preservation.
func TestModulesParsePatternEscapeFollowsVendorCompatibleOption(t *testing.T) {
	source := `module compat-wild-escape {
    namespace "urn:compat-wild-escape";
    prefix cwe;
    leaf name {
        type string {
            pattern "\*";
        }
    }
}
`
	if err := compat.NewModules().Parse(source, "compat-wild-escape.yang"); err == nil {
		t.Fatal("Parse accepted pattern escape without vendor-compatible option")
	} else if !strings.Contains(err.Error(), `invalid escape sequence: \*`) {
		t.Fatalf("Parse error = %q, want invalid escape sequence", err)
	}

	ms := compat.NewModules()
	ms.ParseOptions.IgnoreSubmoduleCircularDependencies = true
	if err := ms.Parse(source, "compat-wild-escape.yang"); err != nil {
		t.Fatalf("Parse with vendor-compatible option: %v", err)
	}
	if errs := ms.Process(); len(errs) != 0 {
		t.Fatalf("Process errors = %v, want none", errs)
	}
	entry, errs := ms.GetModule("compat-wild-escape")
	if len(errs) != 0 {
		t.Fatalf("GetModule errors = %v, want none", errs)
	}
	if entry == nil {
		t.Fatal("GetModule returned nil entry")
	}
	// The preserved backslash must survive compat staging and the native
	// re-parse: the projected leaf type still carries the regex \*.
	leaf := entry.Dir["name"]
	if leaf == nil || leaf.Type == nil {
		t.Fatalf("entry = %+v, want a typed leaf name", entry)
	}
	if len(leaf.Type.Pattern) != 1 || leaf.Type.Pattern[0] != `\*` {
		t.Fatalf("leaf pattern = %q, want [\\*]", leaf.Type.Pattern)
	}
	// In-memory sources are staged by re-rendering statements with safe
	// quoting, so the native re-parse sees conformant text and stays
	// silent. The same source loaded from a file is parsed as written,
	// so the relaxation is reported in the load report exactly once.
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "compat-wild-escape.yang"), []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}
	fileModules := compat.NewModules()
	fileModules.ParseOptions.IgnoreSubmoduleCircularDependencies = true
	fileModules.AddPath(dir)
	if err := fileModules.Read("compat-wild-escape"); err != nil {
		t.Fatalf("Read with vendor-compatible option: %v", err)
	}
	if errs := fileModules.Process(); len(errs) != 0 {
		t.Fatalf("Process file-backed errors = %v, want none", errs)
	}
	escapeWarnings := 0
	for _, warning := range fileModules.LoadReport().Warnings {
		if strings.Contains(warning.Message, `invalid escape sequence: \*`) {
			escapeWarnings++
		}
	}
	if escapeWarnings != 1 {
		t.Fatalf("LoadReport escape warnings = %d, want 1 (warnings %+v)", escapeWarnings, fileModules.LoadReport().Warnings)
	}
}
