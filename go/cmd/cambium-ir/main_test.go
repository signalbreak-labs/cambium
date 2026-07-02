// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/signalbreak-labs/cambium/go/cambium"
)

func TestRunEmitsSchemaIRJSON(t *testing.T) {
	dir := t.TempDir()
	modulePath := filepath.Join(dir, "ir-demo.yang")
	if err := os.WriteFile(modulePath, []byte(`module ir-demo {
  namespace "urn:ir-demo";
  prefix ird;

  container top {
    leaf z { type string; }
    list items {
      key "id";
      leaf value { type uint32; }
      leaf id { type string; }
    }
  }
}`), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	if code := run([]string{"-search", dir, "ir-demo"}, &stdout, &stderr); code != 0 {
		t.Fatalf("run exit = %d, stderr = %s", code, stderr.String())
	}

	var doc exportIR
	if err := json.Unmarshal(stdout.Bytes(), &doc); err != nil {
		t.Fatalf("decode SchemaIR JSON: %v\n%s", err, stdout.String())
	}
	if doc.Version != cambium.SchemaIRVersion {
		t.Fatalf("version = %q, want %q", doc.Version, cambium.SchemaIRVersion)
	}
	if len(doc.Errors) != 0 {
		t.Fatalf("errors = %#v, want none", doc.Errors)
	}
	if len(doc.Modules) != 1 {
		t.Fatalf("modules = %d, want 1", len(doc.Modules))
	}
	mod := doc.Modules[0]
	if mod.Name != "ir-demo" || mod.Namespace != "urn:ir-demo" || mod.Prefix != "ird" {
		t.Fatalf("module = %#v", mod)
	}
	if got := exportNodeNames(mod.Children); !stringSlicesEqual(got, []string{"top"}) {
		t.Fatalf("module children = %v, want top", got)
	}
	top := mod.Children[0]
	if got := exportNodeNames(top.Children); !stringSlicesEqual(got, []string{"z", "items"}) {
		t.Fatalf("top children = %v, want z, items", got)
	}
	items := top.Children[1]
	if items.Kind != "list" {
		t.Fatalf("items kind = %q, want list", items.Kind)
	}
	if !stringSlicesEqual(items.KeyNames, []string{"id"}) {
		t.Fatalf("items key names = %v, want id", items.KeyNames)
	}
	if got := exportNodeNames(items.Children); !stringSlicesEqual(got, []string{"value", "id"}) {
		t.Fatalf("items children = %v, want schema declaration order value, id", got)
	}
	if got := exportNodeNames(items.ListKeys); !stringSlicesEqual(got, []string{"id"}) {
		t.Fatalf("items list keys = %v, want id", got)
	}
}

func TestRunRequiresModule(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run(nil, &stdout, &stderr); code == 0 {
		t.Fatal("run without modules succeeded")
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty", stdout.String())
	}
	if !bytes.Contains(stderr.Bytes(), []byte("module")) {
		t.Fatalf("stderr = %q, want module error", stderr.String())
	}
}

func exportNodeNames(nodes []exportNode) []string {
	out := make([]string, 0, len(nodes))
	for _, node := range nodes {
		out = append(out, node.Name)
	}
	return out
}

func stringSlicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
