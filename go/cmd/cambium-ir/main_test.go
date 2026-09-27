// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
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

func TestRunExportsResolvedRestrictionBounds(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ir-bounds.yang"), []byte(`module ir-bounds {
  yang-version 1.1; namespace "urn:ir-bounds"; prefix irb;
  typedef pct { type uint8 { range "1..100"; } }
  leaf level { type pct { range "min..10 | 20..max"; } }
  leaf name { type string { length "1..max"; } }
  leaf dec { type decimal64 { fraction-digits 2; range "min..0"; } }
  leaf plain { type string; }
}`), 0o644); err != nil {
		t.Fatal(err)
	}
	// min/max resolve against the restricted type; 64-bit and decimal64
	// values are canonical strings so no JSON number precision is lost.
	want := map[string]exportType{
		"level": {Base: "uint8", Range: []exportBound{{Min: "1", Max: "10"}, {Min: "20", Max: "100"}}},
		"name":  {Base: "string", Length: []exportBound{{Min: "1", Max: "18446744073709551615"}}},
		"dec":   {Base: "decimal64", Range: []exportBound{{Min: "-92233720368547758.08", Max: "0.00"}}},
		"plain": {Base: "string"},
	}
	check := func(format, name string, got *exportType) {
		t.Helper()
		if got == nil {
			t.Fatalf("%s %s: no type", format, name)
		}
		if fmt.Sprintf("%+v", *got) != fmt.Sprintf("%+v", want[name]) {
			t.Fatalf("%s %s: type = %+v, want %+v", format, name, *got, want[name])
		}
	}

	var stdout, stderr bytes.Buffer
	if code := run([]string{"-search", dir, "ir-bounds"}, &stdout, &stderr); code != 0 {
		t.Fatalf("v1 exit = %d, stderr = %s", code, stderr.String())
	}
	var compact bytes.Buffer
	if err := json.Compact(&compact, stdout.Bytes()); err != nil {
		t.Fatalf("compact v1: %v", err)
	}
	if !bytes.Contains(compact.Bytes(), []byte(`"length":[{"min":"1","max":"18446744073709551615"}]`)) {
		t.Fatalf("v1 JSON lacks string-encoded length bounds:\n%s", stdout.String())
	}
	var doc exportIR
	if err := json.Unmarshal(stdout.Bytes(), &doc); err != nil {
		t.Fatalf("decode v1: %v", err)
	}
	for _, node := range doc.Modules[0].Children {
		check("v1", node.Name, node.Type)
	}

	stdout.Reset()
	if code := run([]string{"-search", dir, "-format", "v2", "ir-bounds"}, &stdout, &stderr); code != 0 {
		t.Fatalf("v2 exit = %d, stderr = %s", code, stderr.String())
	}
	var table exportTable
	if err := json.Unmarshal(stdout.Bytes(), &table); err != nil {
		t.Fatalf("decode v2: %v", err)
	}
	if len(table.Nodes) != len(want) {
		t.Fatalf("v2 nodes = %d, want %d", len(table.Nodes), len(want))
	}
	for _, node := range table.Nodes {
		check("v2", node.Name, node.Type)
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

func writeChainModule(t *testing.T, dir string, depth int) {
	t.Helper()
	var b bytes.Buffer
	b.WriteString("module chain {\n  yang-version 1.1; namespace \"urn:chain\"; prefix c;\n")
	for i := 0; i < depth; i++ {
		fmt.Fprintf(&b, "container c%d {\n", i)
	}
	b.WriteString("leaf end { type string; }\n")
	for i := 0; i < depth; i++ {
		b.WriteString("}\n")
	}
	b.WriteString("}\n")
	if err := os.WriteFile(filepath.Join(dir, "chain.yang"), b.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestRunV2OutputIsLinearInDepth(t *testing.T) {
	var prev int
	for _, depth := range []int{4, 8, 12, 16} {
		dir := t.TempDir()
		writeChainModule(t, dir, depth)
		var stdout, stderr bytes.Buffer
		if code := run([]string{"-search", dir, "-format", "v2", "chain"}, &stdout, &stderr); code != 0 {
			t.Fatalf("depth %d: exit %d: %s", depth, code, stderr.String())
		}
		var doc exportTable
		if err := json.Unmarshal(stdout.Bytes(), &doc); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if doc.Version != cambium.SchemaIRTableVersion || len(doc.Nodes) != depth+1 {
			t.Fatalf("depth %d: version %q, nodes %d", depth, doc.Version, len(doc.Nodes))
		}
		last := doc.Nodes[len(doc.Nodes)-1]
		if last.Name != "end" || last.Parent != depth-1 || last.Type == nil || last.Type.Base != "string" {
			t.Fatalf("depth %d: last node = %+v", depth, last)
		}
		// Output grows by the per-node record plus longer paths: well under
		// doubling per four levels, unlike the nested v1 projection.
		if prev != 0 && stdout.Len() > prev*3 {
			t.Fatalf("depth %d: %d bytes after %d at depth-4", depth, stdout.Len(), prev)
		}
		t.Logf("depth %d: v2 JSON %d bytes", depth, stdout.Len())
		prev = stdout.Len()
	}
}

func TestRunV1RefusesOversizedProjection(t *testing.T) {
	dir := t.TempDir()
	writeChainModule(t, dir, 40)
	var stdout, stderr bytes.Buffer
	if code := run([]string{"-search", dir, "chain"}, &stdout, &stderr); code != 1 {
		t.Fatalf("exit = %d, want 1; stderr %s", code, stderr.String())
	}
	if stdout.Len() != 0 || !bytes.Contains(stderr.Bytes(), []byte(cambium.SchemaIRTableVersion)) {
		t.Fatalf("stdout %d bytes, stderr %q", stdout.Len(), stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	writeChainModule(t, dir, 4)
	if code := run([]string{"-search", dir, "-max-records", "31", "chain"}, &stdout, &stderr); code != 0 {
		t.Fatalf("depth 4 at exact limit: exit %d: %s", code, stderr.String())
	}
	if code := run([]string{"-search", dir, "-format", "v3", "chain"}, &stdout, &stderr); code != 2 {
		t.Fatalf("bad format exit = %d, want 2", code)
	}
}
