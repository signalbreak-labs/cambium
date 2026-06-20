//go:build cgo

package libyangbackend_test

import (
	"testing"

	cambium "github.com/signalbreak-labs/cambium/go/libyangbackend"
)

func TestNodeRefStaleAfterValidate(t *testing.T) {
	ctx, tree := loadOrderDemo(t)
	defer ctx.Close()
	defer tree.Close()

	node, err := tree.Get("/order-demo:top/order-demo:z")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}

	if err := tree.Validate(cambium.ValidateMode{}); err != nil {
		t.Fatalf("Validate: %v", err)
	}

	_, _, err = node.ValueStr()
	if err == nil {
		t.Fatal("ValueStr on stale NodeRef should error")
	}
	e, ok := err.(*cambium.Error)
	if !ok {
		t.Fatalf("stale error type = %T, want *cambium.Error", err)
	}
	if e.RuleCode() != cambium.RuleCodeStale {
		t.Fatalf("stale error code = %v, want E0007", e.RuleCode())
	}
}

func TestNodeRefStaleAfterTreeClose(t *testing.T) {
	ctx, tree := loadOrderDemo(t)
	defer ctx.Close()

	node, err := tree.Get("/order-demo:top/order-demo:z")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	tree.Close()

	_, err = node.Path()
	if err == nil {
		t.Fatal("Path on NodeRef after tree close should error")
	}
	if e, ok := err.(*cambium.Error); !ok || e.RuleCode() != cambium.RuleCodeStale {
		t.Fatalf("error = %v, want E0007", err)
	}
}

func assertStale(t *testing.T, node cambium.NodeRef) {
	t.Helper()
	_, _, err := node.ValueStr()
	if err == nil {
		t.Fatal("ValueStr on stale NodeRef should error")
	}
	e, ok := err.(*cambium.Error)
	if !ok {
		t.Fatalf("stale error type = %T, want *cambium.Error", err)
	}
	if e.RuleCode() != cambium.RuleCodeStale {
		t.Fatalf("stale error code = %v, want E0007", e.RuleCode())
	}
}

func TestNodeRefStaleAfterNewPath(t *testing.T) {
	ctx, tree := loadCRUDContext(t)
	defer ctx.Close()
	defer tree.Close()

	if _, err := tree.NewPath("/cambium-data-crud-demo:top", nil, cambium.NewPathOpts{}); err != nil {
		t.Fatalf("NewPath top: %v", err)
	}
	counter := "1"
	if _, err := tree.NewPath("/cambium-data-crud-demo:top/counter", &counter, cambium.NewPathOpts{}); err != nil {
		t.Fatalf("NewPath counter: %v", err)
	}
	node, err := tree.Get("/cambium-data-crud-demo:top/counter")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}

	counter2 := "2"
	if _, err := tree.NewPath("/cambium-data-crud-demo:top/counter", &counter2, cambium.NewPathOpts{Update: true}); err != nil {
		t.Fatalf("NewPath counter update: %v", err)
	}
	assertStale(t, node)
}

func TestNodeRefStaleAfterSetValue(t *testing.T) {
	ctx, tree := loadCRUDContext(t)
	defer ctx.Close()
	defer tree.Close()

	if _, err := tree.NewPath("/cambium-data-crud-demo:top", nil, cambium.NewPathOpts{}); err != nil {
		t.Fatalf("NewPath top: %v", err)
	}
	counter := "1"
	if _, err := tree.NewPath("/cambium-data-crud-demo:top/counter", &counter, cambium.NewPathOpts{}); err != nil {
		t.Fatalf("NewPath counter: %v", err)
	}
	node, err := tree.Get("/cambium-data-crud-demo:top/counter")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}

	if _, err := tree.SetValue("/cambium-data-crud-demo:top/counter", "2"); err != nil {
		t.Fatalf("SetValue: %v", err)
	}
	assertStale(t, node)
}

func TestNodeRefStaleAfterRemovePath(t *testing.T) {
	ctx, tree := loadCRUDContext(t)
	defer ctx.Close()
	defer tree.Close()

	if _, err := tree.NewPath("/cambium-data-crud-demo:top", nil, cambium.NewPathOpts{}); err != nil {
		t.Fatalf("NewPath top: %v", err)
	}
	counter := "1"
	if _, err := tree.NewPath("/cambium-data-crud-demo:top/counter", &counter, cambium.NewPathOpts{}); err != nil {
		t.Fatalf("NewPath counter: %v", err)
	}
	node, err := tree.Get("/cambium-data-crud-demo:top/counter")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}

	if err := tree.RemovePath("/cambium-data-crud-demo:top/counter"); err != nil {
		t.Fatalf("RemovePath: %v", err)
	}
	assertStale(t, node)
}

func TestNodeRefStaleAfterUnlinkPath(t *testing.T) {
	ctx, tree := loadCRUDContext(t)
	defer ctx.Close()
	defer tree.Close()

	if _, err := tree.NewPath("/cambium-data-crud-demo:top", nil, cambium.NewPathOpts{}); err != nil {
		t.Fatalf("NewPath top: %v", err)
	}
	if _, err := tree.NewPath("/cambium-data-crud-demo:top/nested", nil, cambium.NewPathOpts{}); err != nil {
		t.Fatalf("NewPath nested: %v", err)
	}
	node, err := tree.Get("/cambium-data-crud-demo:top/nested")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}

	detached, err := tree.UnlinkPath("/cambium-data-crud-demo:top/nested")
	if err != nil {
		t.Fatalf("UnlinkPath: %v", err)
	}
	defer detached.Close()
	assertStale(t, node)
}

func TestNodeRefStaleAfterAddDefaults(t *testing.T) {
	ctx, tree := loadCRUDContext(t)
	defer ctx.Close()
	defer tree.Close()

	if _, err := tree.NewPath("/cambium-data-crud-demo:top", nil, cambium.NewPathOpts{}); err != nil {
		t.Fatalf("NewPath top: %v", err)
	}
	node, err := tree.Get("/cambium-data-crud-demo:top")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}

	if err := tree.AddDefaults(cambium.ImplicitOpts{}); err != nil {
		t.Fatalf("AddDefaults: %v", err)
	}
	assertStale(t, node)
}
