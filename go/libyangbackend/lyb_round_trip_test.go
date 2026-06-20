//go:build cgo

package libyangbackend_test

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	cambium "github.com/signalbreak-labs/cambium/go/libyangbackend"
)

func TestSerializeLYBRoundTrip(t *testing.T) {
	conf := findConformance(t)
	moduleDir := filepath.Join(conf, "fixtures", "ordered-user", "module")
	inputJSON, err := os.ReadFile(filepath.Join(conf, "golden", "ordered-user", "output.json"))
	if err != nil {
		t.Fatal(err)
	}

	ctx, err := cambium.NewContext()
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}
	defer ctx.Close()
	if err := ctx.SetSearchPath(moduleDir); err != nil {
		t.Fatalf("SetSearchPath: %v", err)
	}
	if err := ctx.LoadModule("ordered-user-demo"); err != nil {
		t.Fatalf("LoadModule: %v", err)
	}

	tree, err := ctx.Parse(cambium.FormatJSON, cambium.ParseModeDataOnly, inputJSON)
	if err != nil {
		t.Fatalf("Parse JSON: %v", err)
	}
	defer tree.Close()

	lyb, err := tree.Serialize(cambium.FormatLYB, cambium.DefaultSerializeFlags())
	if err != nil {
		t.Fatalf("Serialize LYB: %v", err)
	}
	if len(lyb) == 0 {
		t.Fatal("Serialize LYB returned empty output")
	}

	round, err := ctx.Parse(cambium.FormatLYB, cambium.ParseModeDataOnly, lyb)
	if err != nil {
		t.Fatalf("Parse LYB: %v", err)
	}
	defer round.Close()

	roundJSON, err := round.Serialize(cambium.FormatJSON, cambium.DefaultSerializeFlags())
	if err != nil {
		t.Fatalf("Serialize JSON: %v", err)
	}
	if !bytes.Equal(bytes.TrimSpace(roundJSON), bytes.TrimSpace(inputJSON)) {
		t.Fatalf("JSON round trip mismatch\nwant:\n%s\n\ngot:\n%s", inputJSON, roundJSON)
	}

	expectedXML, err := os.ReadFile(filepath.Join(conf, "golden", "ordered-user", "output.xml"))
	if err != nil {
		t.Fatal(err)
	}
	roundXML, err := round.Serialize(cambium.FormatXML, cambium.DefaultSerializeFlags())
	if err != nil {
		t.Fatalf("Serialize XML: %v", err)
	}
	if !bytes.Equal(bytes.TrimSpace(roundXML), bytes.TrimSpace(expectedXML)) {
		t.Fatalf("XML round trip mismatch\nwant:\n%s\n\ngot:\n%s", expectedXML, roundXML)
	}
}
