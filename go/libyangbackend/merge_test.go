//go:build cgo

package libyangbackend_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	cambium "github.com/signalbreak-labs/cambium/go/libyangbackend"
)

func TestMergePreservesUserOrder(t *testing.T) {
	conf := findConformance(t)
	moduleDir := filepath.Join(conf, "fixtures", "ordered-user", "module")
	input, err := os.ReadFile(filepath.Join(conf, "fixtures", "ordered-user", "input.xml"))
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
	source, err := ctx.Parse(cambium.FormatXML, cambium.ParseModeDataOnly, input)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	defer source.Close()

	target := ctx.NewData()
	defer target.Close()
	if _, err := target.NewPath("/ordered-user-demo:config", nil, cambium.NewPathOpts{}); err != nil {
		t.Fatalf("NewPath config: %v", err)
	}
	if err := target.Merge(source, cambium.MergeOpts{}); err != nil {
		t.Fatalf("Merge: %v", err)
	}

	expected, err := os.ReadFile(filepath.Join(conf, "golden", "ordered-user", "output.json"))
	if err != nil {
		t.Fatal(err)
	}
	got, err := target.Serialize(cambium.FormatJSON, cambium.DefaultSerializeFlags())
	if err != nil {
		t.Fatalf("Serialize: %v", err)
	}
	if string(got) != string(expected) {
		t.Fatalf("merged output mismatch:\n%s\nexpected:\n%s", got, expected)
	}
}

func TestMergeConflictErrors(t *testing.T) {
	ctx, target := loadCRUDContext(t)
	defer ctx.Close()
	defer target.Close()

	if _, err := target.NewPath("/cambium-data-crud-demo:top", nil, cambium.NewPathOpts{}); err != nil {
		t.Fatalf("NewPath top: %v", err)
	}
	counter := "1"
	if _, err := target.NewPath("/cambium-data-crud-demo:top/counter", &counter, cambium.NewPathOpts{}); err != nil {
		t.Fatalf("NewPath counter: %v", err)
	}

	source := ctx.NewData()
	defer source.Close()
	if _, err := source.NewPath("/cambium-data-crud-demo:top", nil, cambium.NewPathOpts{}); err != nil {
		t.Fatalf("NewPath source top: %v", err)
	}
	sourceCounter := "2"
	if _, err := source.NewPath("/cambium-data-crud-demo:top/counter", &sourceCounter, cambium.NewPathOpts{}); err != nil {
		t.Fatalf("NewPath source counter: %v", err)
	}

	err := target.Merge(source, cambium.MergeOpts{})
	if err == nil {
		t.Fatal("expected merge conflict error")
	}
	var ce *cambium.Error
	if !errors.As(err, &ce) || ce.RuleCode() != cambium.RuleCodeValidate {
		t.Fatalf("want RuleCodeValidate (E0003), got %v", err)
	}

	node, err := target.Get("/cambium-data-crud-demo:top/counter")
	if err != nil {
		t.Fatalf("Get counter: %v", err)
	}
	v, ok, err := node.Value()
	if err != nil || !ok {
		t.Fatalf("Value counter: ok=%v err=%v", ok, err)
	}
	if u, ok := v.Uint64(); !ok || u != 1 {
		t.Fatalf("target counter = %v, want 1 (unmutated)", u)
	}
}
