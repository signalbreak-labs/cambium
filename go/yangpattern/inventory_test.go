// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs
package yangpattern

import (
	"bytes"
	"go/parser"
	"go/token"
	"os"
	"path"
	"strconv"
	"strings"
	"testing"
)

func TestRuntimeInventory(t *testing.T) {
	sources := RuntimeSources()
	if len(sources) == 0 {
		t.Fatal("empty runtime inventory")
	}
	seen := map[string]bool{}
	for _, source := range sources {
		if path.Base(source.Name) != source.Name || !strings.HasPrefix(source.Name, "runtime_") || seen[source.Name] {
			t.Fatalf("invalid inventory name %q", source.Name)
		}
		seen[source.Name] = true
		disk, err := os.ReadFile(source.Name)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(disk, source.Data) {
			t.Fatalf("inventory differs from %s", source.Name)
		}
		if !bytes.Contains(source.Data, []byte("SPDX-License-Identifier: Apache-2.0")) || !bytes.Contains(source.Data, []byte("Copyright 2026 signalbreak-labs")) {
			t.Fatal("missing attribution")
		}
		file, err := parser.ParseFile(token.NewFileSet(), source.Name, source.Data, parser.ImportsOnly)
		if err != nil {
			t.Fatal(err)
		}
		if file.Name.Name != "yangpattern" {
			t.Fatal("package changed")
		}
		for _, imp := range file.Imports {
			p, err := strconv.Unquote(imp.Path.Value)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(strings.Split(p, "/")[0], ".") {
				t.Errorf("nonstdlib import %q", p)
			}
		}
		source.Data[0] = '!'
	}
	for _, s := range RuntimeSources() {
		if s.Data[0] == '!' {
			t.Fatal("inventory mutation leaked")
		}
	}
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), "runtime_") && strings.HasSuffix(entry.Name(), ".go") && !strings.HasSuffix(entry.Name(), "_test.go") && !seen[entry.Name()] {
			t.Fatalf("runtime file not inventoried: %s", entry.Name())
		}
	}
}
