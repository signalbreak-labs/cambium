// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

//go:build cgo

package libyangbackend_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	cambium "github.com/signalbreak-labs/cambium/go/libyangbackend"
)

func TestSerializeDefaultBytesUnchanged(t *testing.T) {
	conf := findConformance(t)
	moduleDir := filepath.Join(conf, "fixtures", "ordered-user", "module")
	input, err := os.ReadFile(filepath.Join(conf, "fixtures", "ordered-user", "input.xml"))
	if err != nil {
		t.Fatal(err)
	}

	ctx, err := cambium.NewContext()
	if err != nil {
		t.Fatal(err)
	}
	defer ctx.Close()
	if err := ctx.SetSearchPath(moduleDir); err != nil {
		t.Fatal(err)
	}
	if err := ctx.LoadModule("ordered-user-demo"); err != nil {
		t.Fatal(err)
	}

	tree, err := ctx.Parse(cambium.FormatXML, cambium.ParseModeDataOnly, input)
	if err != nil {
		t.Fatal(err)
	}
	defer tree.Close()

	goldenXML, err := os.ReadFile(filepath.Join(conf, "golden", "ordered-user", "output.xml"))
	if err != nil {
		t.Fatal(err)
	}
	goldenJSON, err := os.ReadFile(filepath.Join(conf, "golden", "ordered-user", "output.json"))
	if err != nil {
		t.Fatal(err)
	}

	actualXML, err := tree.Serialize(cambium.FormatXML, cambium.DefaultSerializeFlags())
	if err != nil {
		t.Fatal(err)
	}
	actualJSON, err := tree.Serialize(cambium.FormatJSON, cambium.DefaultSerializeFlags())
	if err != nil {
		t.Fatal(err)
	}

	if string(normalize(goldenXML)) != string(normalize(actualXML)) {
		t.Fatalf("XML default flags changed golden bytes\n--- want ---\n%s\n--- got ---\n%s", goldenXML, actualXML)
	}
	if string(normalize(goldenJSON)) != string(normalize(actualJSON)) {
		t.Fatalf("JSON default flags changed golden bytes\n--- want ---\n%s\n--- got ---\n%s", goldenJSON, actualJSON)
	}
}

func TestSerializeWithDefaultsAll(t *testing.T) {
	ctx, tree := loadCRUDContext(t)
	defer ctx.Close()
	defer tree.Close()

	if _, err := tree.NewPath("/cambium-data-crud-demo:top", nil, cambium.NewPathOpts{}); err != nil {
		t.Fatalf("NewPath top: %v", err)
	}
	if err := tree.AddDefaults(cambium.ImplicitOpts{}); err != nil {
		t.Fatalf("AddDefaults: %v", err)
	}

	flags := cambium.SerializeFlags{Siblings: true, Shrink: true, WithDefaults: cambium.WithDefaultsAll}
	goldenXML := `<top xmlns="urn:cambium:data-crud"><enabled>true</enabled></top>`
	goldenJSON := `{"cambium-data-crud-demo:top":{"enabled":true}}`

	xml, err := tree.Serialize(cambium.FormatXML, flags)
	if err != nil {
		t.Fatalf("Serialize XML: %v", err)
	}
	json, err := tree.Serialize(cambium.FormatJSON, flags)
	if err != nil {
		t.Fatalf("Serialize JSON: %v", err)
	}

	if got := string(normalize(xml)); got != goldenXML {
		t.Fatalf("XML with defaults all mismatch\nwant: %s\ngot:  %s", goldenXML, got)
	}
	if got := string(normalize(json)); got != goldenJSON {
		t.Fatalf("JSON with defaults all mismatch\nwant: %s\ngot:  %s", goldenJSON, got)
	}
}

func TestSerializeKeepEmptyContainer(t *testing.T) {
	ctx, tree := loadCRUDContext(t)
	defer ctx.Close()
	defer tree.Close()

	if _, err := tree.NewPath("/cambium-data-crud-demo:top", nil, cambium.NewPathOpts{}); err != nil {
		t.Fatalf("NewPath top: %v", err)
	}

	flags := cambium.DefaultSerializeFlags()
	flags.Shrink = true
	flags.KeepEmptyContainers = true
	goldenXML := `<top xmlns="urn:cambium:data-crud"/>`
	goldenJSON := `{"cambium-data-crud-demo:top":{}}`

	xml, err := tree.Serialize(cambium.FormatXML, flags)
	if err != nil {
		t.Fatalf("Serialize XML: %v", err)
	}
	json, err := tree.Serialize(cambium.FormatJSON, flags)
	if err != nil {
		t.Fatalf("Serialize JSON: %v", err)
	}

	if got := string(normalize(xml)); got != goldenXML {
		t.Fatalf("XML keep-empty mismatch\nwant: %s\ngot:  %s", goldenXML, got)
	}
	if got := string(normalize(json)); got != goldenJSON {
		t.Fatalf("JSON keep-empty mismatch\nwant: %s\ngot:  %s", goldenJSON, got)
	}
}

func TestSerializeShrinkRemovesWhitespace(t *testing.T) {
	ctx, tree := loadCRUDContext(t)
	defer ctx.Close()
	defer tree.Close()

	if _, err := tree.NewPath("/cambium-data-crud-demo:top", nil, cambium.NewPathOpts{}); err != nil {
		t.Fatalf("NewPath top: %v", err)
	}
	counter := "5"
	if _, err := tree.NewPath("/cambium-data-crud-demo:top/counter", &counter, cambium.NewPathOpts{}); err != nil {
		t.Fatalf("NewPath counter: %v", err)
	}

	flags := cambium.SerializeFlags{Siblings: true, Shrink: true}
	goldenXML := `<top xmlns="urn:cambium:data-crud"><counter>5</counter></top>`
	goldenJSON := `{"cambium-data-crud-demo:top":{"counter":"5"}}`

	xml, err := tree.Serialize(cambium.FormatXML, flags)
	if err != nil {
		t.Fatalf("Serialize XML: %v", err)
	}
	json, err := tree.Serialize(cambium.FormatJSON, flags)
	if err != nil {
		t.Fatalf("Serialize JSON: %v", err)
	}

	if got := string(normalize(xml)); got != goldenXML {
		t.Fatalf("XML shrink mismatch\nwant: %s\ngot:  %s", goldenXML, got)
	}
	if got := string(normalize(json)); got != goldenJSON {
		t.Fatalf("JSON shrink mismatch\nwant: %s\ngot:  %s", goldenJSON, got)
	}
	if strings.Contains(string(xml), "\n") {
		t.Fatalf("shrunk XML contains newline:\n%s", xml)
	}
}

func normalize(b []byte) []byte {
	return []byte(strings.TrimRight(string(b), " \t\r\n\v\f"))
}
