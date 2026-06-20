//go:build cgo

package libyangbackend

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCodeForOpMergeUsesValidateRuleCode(t *testing.T) {
	if got := codeForOp("merge"); got != RuleCodeValidate {
		t.Fatalf("codeForOp(%q) = %s, want %s", "merge", got, RuleCodeValidate)
	}
}

func TestNodeRefStaleAfterDiffApply(t *testing.T) {
	ctx, tree := loadBackendCRUDContext(t)
	defer ctx.Close()
	defer tree.Close()
	buildBackendBase(t, tree)

	node, err := tree.Get("/cambium-data-crud-demo:top/counter")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}

	after := ctx.NewData()
	defer after.Close()
	buildBackendAfter(t, after)

	diff, err := tree.Diff(after, DiffOpts{})
	if err != nil {
		t.Fatalf("Diff: %v", err)
	}
	defer diff.Close()

	if err := tree.DiffApply(diff); err != nil {
		t.Fatalf("DiffApply: %v", err)
	}
	assertBackendStale(t, node)
}

func TestNodeRefStaleAfterMerge(t *testing.T) {
	ctx, target := loadBackendCRUDContext(t)
	defer ctx.Close()
	defer target.Close()

	if _, err := target.NewPath("/cambium-data-crud-demo:top", nil, NewPathOpts{}); err != nil {
		t.Fatalf("NewPath target top: %v", err)
	}
	counter := "1"
	if _, err := target.NewPath("/cambium-data-crud-demo:top/counter", &counter, NewPathOpts{}); err != nil {
		t.Fatalf("NewPath target counter: %v", err)
	}
	node, err := target.Get("/cambium-data-crud-demo:top")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}

	source := ctx.NewData()
	defer source.Close()
	if _, err := source.NewPath("/cambium-data-crud-demo:top", nil, NewPathOpts{}); err != nil {
		t.Fatalf("NewPath source top: %v", err)
	}
	name := "merged"
	if _, err := source.NewPath("/cambium-data-crud-demo:top/nested/name", &name, NewPathOpts{}); err != nil {
		t.Fatalf("NewPath source nested/name: %v", err)
	}

	if err := target.Merge(source, MergeOpts{}); err != nil {
		t.Fatalf("Merge: %v", err)
	}
	assertBackendStale(t, node)
}

func loadBackendCRUDContext(t *testing.T) (*Context, *DataTree) {
	t.Helper()
	dir := t.TempDir()
	module := filepath.Join(dir, "cambium-data-crud-demo.yang")
	const source = `module cambium-data-crud-demo {
    namespace "urn:cambium:data-crud";
    prefix cdc;
    yang-version 1.1;
    revision 2026-06-14;

    container top {
        leaf enabled {
            type boolean;
            default "true";
        }
        leaf counter {
            type uint64;
        }
        container nested {
            leaf name {
                type string;
            }
        }
        list item {
            key "id";
            leaf id {
                type string;
            }
            leaf value {
                type uint64;
            }
        }
        leaf-list tags {
            type string;
        }
    }
}
`
	if err := os.WriteFile(module, []byte(source), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	ctx, err := NewContext()
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}
	if err := ctx.SetSearchPath(dir); err != nil {
		ctx.Close()
		t.Fatalf("SetSearchPath: %v", err)
	}
	if err := ctx.LoadModule("cambium-data-crud-demo"); err != nil {
		ctx.Close()
		t.Fatalf("LoadModule: %v", err)
	}
	return ctx, ctx.NewData()
}

func buildBackendBase(t *testing.T, tree *DataTree) {
	t.Helper()
	if _, err := tree.NewPath("/cambium-data-crud-demo:top", nil, NewPathOpts{}); err != nil {
		t.Fatalf("NewPath top: %v", err)
	}
	counter := "1"
	if _, err := tree.NewPath("/cambium-data-crud-demo:top/counter", &counter, NewPathOpts{}); err != nil {
		t.Fatalf("NewPath counter: %v", err)
	}
	if _, err := tree.NewPath("/cambium-data-crud-demo:top/nested", nil, NewPathOpts{}); err != nil {
		t.Fatalf("NewPath nested: %v", err)
	}
	name := "x"
	if _, err := tree.NewPath("/cambium-data-crud-demo:top/nested/name", &name, NewPathOpts{}); err != nil {
		t.Fatalf("NewPath nested/name: %v", err)
	}
	if _, err := tree.NewPath("/cambium-data-crud-demo:top/item[id='a']", nil, NewPathOpts{}); err != nil {
		t.Fatalf("NewPath item a: %v", err)
	}
	value := "10"
	if _, err := tree.NewPath("/cambium-data-crud-demo:top/item[id='a']/value", &value, NewPathOpts{}); err != nil {
		t.Fatalf("NewPath item a value: %v", err)
	}
	tag := "red"
	if _, err := tree.NewPath("/cambium-data-crud-demo:top/tags", &tag, NewPathOpts{}); err != nil {
		t.Fatalf("NewPath tags: %v", err)
	}
}

func buildBackendAfter(t *testing.T, tree *DataTree) {
	t.Helper()
	if _, err := tree.NewPath("/cambium-data-crud-demo:top", nil, NewPathOpts{}); err != nil {
		t.Fatalf("NewPath top: %v", err)
	}
	counter := "2"
	if _, err := tree.NewPath("/cambium-data-crud-demo:top/counter", &counter, NewPathOpts{}); err != nil {
		t.Fatalf("NewPath counter: %v", err)
	}
	if _, err := tree.NewPath("/cambium-data-crud-demo:top/item[id='b']", nil, NewPathOpts{}); err != nil {
		t.Fatalf("NewPath item b: %v", err)
	}
	value := "20"
	if _, err := tree.NewPath("/cambium-data-crud-demo:top/item[id='b']/value", &value, NewPathOpts{}); err != nil {
		t.Fatalf("NewPath item b value: %v", err)
	}
}

func assertBackendStale(t *testing.T, node NodeRef) {
	t.Helper()
	_, _, err := node.ValueStr()
	if err == nil {
		t.Fatal("ValueStr on stale NodeRef should error")
	}
	e, ok := err.(*Error)
	if !ok {
		t.Fatalf("stale error type = %T, want *Error", err)
	}
	if e.RuleCode() != RuleCodeStale {
		t.Fatalf("stale error code = %v, want %v", e.RuleCode(), RuleCodeStale)
	}
}
