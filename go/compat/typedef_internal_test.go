package compat

import (
	"testing"

	upstream "github.com/signalbreak-labs/cambium/go/internal/yangparse/upstream/yang"
)

func TestRawASTIncludedTypedefFallback(t *testing.T) {
	parentStatements, err := Parse(`module compat-raw-typedef-include-main {
    namespace "urn:compat-raw-typedef-include-main";
    prefix crtim;

    include compat-raw-typedef-include-sub;

    leaf value {
        type included-id;
    }
}
`, "compat-raw-typedef-include-main.yang")
	if err != nil {
		t.Fatalf("Parse parent: %v", err)
	}
	submoduleStatements, err := Parse(`submodule compat-raw-typedef-include-sub {
    belongs-to compat-raw-typedef-include-main {
        prefix crtim;
    }

    typedef included-id {
        type uint32;
        default "99";
        units "widgets";
    }
}
`, "compat-raw-typedef-include-sub.yang")
	if err != nil {
		t.Fatalf("Parse submodule: %v", err)
	}

	parent := &ASTModule{
		Name:   "compat-raw-typedef-include-main",
		Prefix: &ASTValue{Name: "crtim"},
		Source: parentStatements[0],
	}
	submodule := &ASTModule{
		Name:      "compat-raw-typedef-include-sub",
		BelongsTo: &BelongsTo{Name: "compat-raw-typedef-include-main", Prefix: &ASTValue{Name: "crtim"}},
		Source:    submoduleStatements[0],
	}
	parent.Include = []*Include{{
		Name:   "compat-raw-typedef-include-sub",
		Parent: parent,
		Module: submodule,
	}}

	leaf := ToEntry(parent).Find("value")
	if leaf == nil {
		t.Fatal("Find(value) = nil")
	}
	if leaf.Type == nil {
		t.Fatal("value Type = nil")
	}
	if leaf.Type.Kind != Yuint32 {
		t.Fatalf("value Type.Kind = %s, want uint32", leaf.Type.Kind)
	}
	if leaf.Type.Base == nil || leaf.Type.Base.Name != "uint32" {
		t.Fatalf("value Type.Base = %#v, want uint32 base", leaf.Type.Base)
	}
	if !leaf.Type.HasDefault || leaf.Type.Default != "99" {
		t.Fatalf("value Type default = (%q,%v), want 99,true", leaf.Type.Default, leaf.Type.HasDefault)
	}
	if got := leaf.Type.Units; got != "widgets" {
		t.Fatalf("Type.Units = %q, want widgets", got)
	}
}

func TestRawASTImportedIdentityRefFallbackMatchesGoyang(t *testing.T) {
	const importedSource = `module compat-raw-identity-imported {
    namespace "urn:compat-raw-identity-imported";
    prefix crii;

    identity base;
    identity derived {
        base base;
    }
}
`
	const parentSource = `module compat-raw-identity-main {
    namespace "urn:compat-raw-identity-main";
    prefix crim;

    import compat-raw-identity-imported {
        prefix crii;
    }

    leaf value {
        type identityref {
            base crii:base;
        }
    }
}
`
	importedStatements, err := Parse(importedSource, "compat-raw-identity-imported.yang")
	if err != nil {
		t.Fatalf("Parse imported: %v", err)
	}
	parentStatements, err := Parse(parentSource, "compat-raw-identity-main.yang")
	if err != nil {
		t.Fatalf("Parse parent: %v", err)
	}
	upstreamModules := upstream.NewModules()
	for name, source := range map[string]string{
		"compat-raw-identity-imported.yang": importedSource,
		"compat-raw-identity-main.yang":     parentSource,
	} {
		if err := upstreamModules.Parse(source, name); err != nil {
			t.Fatalf("upstream Parse(%s): %v", name, err)
		}
	}
	if errs := upstreamModules.Process(); len(errs) != 0 {
		t.Fatalf("upstream Process: %v", errs)
	}

	imported := &ASTModule{
		Name:   "compat-raw-identity-imported",
		Prefix: &ASTValue{Name: "crii"},
		Source: importedStatements[0],
	}
	parent := &ASTModule{
		Name:   "compat-raw-identity-main",
		Prefix: &ASTValue{Name: "crim"},
		Source: parentStatements[0],
	}
	parent.Import = []*Import{{
		Name:   "compat-raw-identity-imported",
		Parent: parent,
		Prefix: &ASTValue{Name: "crii"},
		Module: imported,
	}}

	compatLeaf := ToEntry(parent).Find("value")
	upstreamLeaf := upstream.ToEntry(upstreamModules.Modules["compat-raw-identity-main"]).Find("value")
	if compatLeaf == nil || upstreamLeaf == nil {
		t.Fatalf("Find(value) = (%#v,%#v), want both non-nil", compatLeaf, upstreamLeaf)
	}
	if compatLeaf.Type == nil || upstreamLeaf.Type == nil {
		t.Fatalf("Type = (%#v,%#v), want both non-nil", compatLeaf.Type, upstreamLeaf.Type)
	}
	if compatLeaf.Type.Kind != TypeKind(upstreamLeaf.Type.Kind) {
		t.Fatalf("Type.Kind = %s, want goyang %s", compatLeaf.Type.Kind, upstreamLeaf.Type.Kind)
	}
	if compatLeaf.Type.IdentityBase == nil || upstreamLeaf.Type.IdentityBase == nil {
		t.Fatalf("IdentityBase = (%#v,%#v), want both non-nil", compatLeaf.Type.IdentityBase, upstreamLeaf.Type.IdentityBase)
	}
	if got, want := compatLeaf.Type.IdentityBase.Name, upstreamLeaf.Type.IdentityBase.Name; got != want {
		t.Fatalf("IdentityBase.Name = %q, want goyang %q", got, want)
	}
	if got, want := compatLeaf.Type.IdentityBase.PrefixedName(), upstreamLeaf.Type.IdentityBase.PrefixedName(); got != want {
		t.Fatalf("IdentityBase.PrefixedName = %q, want goyang %q", got, want)
	}
	if got, want := compatLeaf.Type.IdentityBase.IsDefined("derived"), upstreamLeaf.Type.IdentityBase.IsDefined("derived"); got != want {
		t.Fatalf("IdentityBase.IsDefined(derived) = %v, want goyang %v", got, want)
	}
}
