//go:build cgo

package libyangbackend_test

import (
	"testing"

	cambium "github.com/signalbreak-labs/cambium/go/libyangbackend"
)

func TestDiffApplyRoundTrip(t *testing.T) {
	ctx, baseTree := loadCRUDContext(t)
	defer ctx.Close()
	defer baseTree.Close()
	buildBase(t, baseTree)

	afterTree := ctx.NewData()
	defer afterTree.Close()
	buildAfter(t, afterTree)

	diff, err := baseTree.Diff(afterTree, cambium.DiffOpts{})
	if err != nil {
		t.Fatalf("Diff: %v", err)
	}
	defer diff.Close()

	if err := baseTree.DiffApply(diff); err != nil {
		t.Fatalf("DiffApply: %v", err)
	}

	expectedXML, err := afterTree.Serialize(cambium.FormatXML, cambium.DefaultSerializeFlags())
	if err != nil {
		t.Fatalf("Serialize after XML: %v", err)
	}
	expectedJSON, err := afterTree.Serialize(cambium.FormatJSON, cambium.DefaultSerializeFlags())
	if err != nil {
		t.Fatalf("Serialize after JSON: %v", err)
	}

	gotXML, err := baseTree.Serialize(cambium.FormatXML, cambium.DefaultSerializeFlags())
	if err != nil {
		t.Fatalf("Serialize base XML: %v", err)
	}
	gotJSON, err := baseTree.Serialize(cambium.FormatJSON, cambium.DefaultSerializeFlags())
	if err != nil {
		t.Fatalf("Serialize base JSON: %v", err)
	}

	if string(gotXML) != string(expectedXML) {
		t.Fatalf("XML mismatch after apply:\n%s\nexpected:\n%s", gotXML, expectedXML)
	}
	if string(gotJSON) != string(expectedJSON) {
		t.Fatalf("JSON mismatch after apply:\n%s\nexpected:\n%s", gotJSON, expectedJSON)
	}
}
