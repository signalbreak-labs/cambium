package cambium_test

import (
	"strings"
	"testing"

	"github.com/signalbreak-labs/cambium/go/cambium"
)

// dumpFixture exercises declaration order deliberately: container c declares
// leaves in the order z, m, a (reverse-alphabetical) so the golden proves the
// renderer preserves schema declaration order rather than sorting; the
// enumeration values red, green, blue prove value order in the types view; the
// list annotates its key and the config-false leaf renders read-only.
const dumpFixture = `module ex {
    namespace "urn:ex";
    prefix ex;

    container c {
        leaf z { type string; }
        leaf m { type uint8; }
        leaf a { type boolean; }
    }

    list l {
        key "id";
        leaf id { type string; }
        leaf val { config false; type string; }
    }

    leaf-list tags { type string; }

    leaf color {
        type enumeration {
            enum red;
            enum green;
            enum blue;
        }
    }
}`

func loadDumpFixture(t *testing.T) cambium.Module {
	t.Helper()
	builder, err := cambium.NewContextBuilder(cambium.ContextFlags{})
	if err != nil {
		t.Fatal(err)
	}
	if err := builder.LoadModuleStr(dumpFixture); err != nil {
		t.Fatalf("LoadModuleStr: %v", err)
	}
	ctx, err := builder.Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	t.Cleanup(func() { ctx.Close() })
	mod, err := ctx.Schema("ex")
	if err != nil {
		t.Fatalf("Schema: %v", err)
	}
	return mod
}

func TestWriteTreeGolden(t *testing.T) {
	mod := loadDumpFixture(t)
	var sb strings.Builder
	if err := cambium.WriteTree(&sb, mod); err != nil {
		t.Fatalf("WriteTree: %v", err)
	}
	want := `module: ex
  container c [rw]
    leaf z [rw] : string
    leaf m [rw] : uint8
    leaf a [rw] : boolean
  list l [rw] [key: id]
    leaf id [rw] : string
    leaf val [ro] : string
  leaf-list tags [rw] : string
  leaf color [rw] : enumeration
`
	if got := sb.String(); got != want {
		t.Fatalf("WriteTree output mismatch:\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

func TestWriteTypesGolden(t *testing.T) {
	mod := loadDumpFixture(t)
	var sb strings.Builder
	if err := cambium.WriteTypes(&sb, mod); err != nil {
		t.Fatalf("WriteTypes: %v", err)
	}
	want := `types: ex
  /ex/c/z : string
  /ex/c/m : uint8
  /ex/c/a : boolean
  /ex/l/id : string
  /ex/l/val : string
  /ex/tags : string
  /ex/color : enumeration {red=0, green=1, blue=2}
`
	if got := sb.String(); got != want {
		t.Fatalf("WriteTypes output mismatch:\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

// TestWriteTreePreservesDeclarationOrder is the explicit order guard: the
// rendered children of container c must be z, m, a (declaration order), never
// the alphabetized a, m, z.
func TestWriteTreePreservesDeclarationOrder(t *testing.T) {
	mod := loadDumpFixture(t)
	var sb strings.Builder
	if err := cambium.WriteTree(&sb, mod); err != nil {
		t.Fatalf("WriteTree: %v", err)
	}
	out := sb.String()
	zi, mi, ai := strings.Index(out, "leaf z "), strings.Index(out, "leaf m "), strings.Index(out, "leaf a ")
	if zi < 0 || mi < 0 || ai < 0 {
		t.Fatalf("expected leaves z, m, a in output:\n%s", out)
	}
	if !(zi < mi && mi < ai) {
		t.Fatalf("leaves not in declaration order z<m<a (got positions z=%d m=%d a=%d):\n%s", zi, mi, ai, out)
	}
}
