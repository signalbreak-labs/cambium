// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

package datatree_test

import (
	"strings"
	"testing"

	"github.com/signalbreak-labs/cambium/go/cambium"
	"github.com/signalbreak-labs/cambium/go/datatree"
)

// dtAnySchema declares anydata and anyxml at top level and nested in a container,
// interleaved with an ordinary leaf, so round-trip output proves schema order is
// preserved and opaque payloads survive as compact JSON. yang-version 1.1 is required by
// anydata (anyxml is YANG 1.0, but the module mixes both).
const dtAnySchema = `module dta {
    namespace "urn:dta";
    prefix dta;
    yang-version 1.1;

    anydata payload;
    anyxml raw;
    container c {
        anydata meta;
    }
    leaf after { type string; }
}`

func loadDTAny(t *testing.T) cambium.Module {
	t.Helper()
	b, err := cambium.NewContextBuilder(cambium.ContextFlags{})
	if err != nil {
		t.Fatal(err)
	}
	if err := b.LoadModuleStr(dtAnySchema); err != nil {
		t.Fatalf("LoadModuleStr: %v", err)
	}
	ctx, err := b.Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	t.Cleanup(func() { ctx.Close() })
	mod, err := ctx.Schema("dta")
	if err != nil {
		t.Fatalf("Schema: %v", err)
	}
	return mod
}

func TestAnyDataAnyXMLJSONRoundTrip(t *testing.T) {
	mod := loadDTAny(t)
	// Input members in non-schema order; output must be schema declaration order
	// (payload, raw, after) with opaque content preserved as compact JSON.
	in := `{"dta:raw":"hello","dta:payload":{"x":1,"y":[2,3]},"dta:after":"z"}`
	tree, err := datatree.Parse(mod, datatree.FormatJSONIETF, []byte(in))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	out, err := tree.Serialize(datatree.FormatJSONIETF)
	if err != nil {
		t.Fatalf("Serialize: %v", err)
	}
	want := `{"dta:payload":{"x":1,"y":[2,3]},"dta:raw":"hello","dta:after":"z"}`
	if got := string(out); got != want {
		t.Fatalf("round-trip mismatch:\n got: %s\nwant: %s", got, want)
	}
	// Round-trip is stable (parse(out) re-serializes identically).
	tree2, err := datatree.Parse(mod, datatree.FormatJSONIETF, out)
	if err != nil {
		t.Fatalf("re-Parse: %v", err)
	}
	out2, err := tree2.Serialize(datatree.FormatJSONIETF)
	if err != nil {
		t.Fatalf("re-Serialize: %v", err)
	}
	if string(out2) != want {
		t.Fatalf("not stable:\n out2: %s\nwant: %s", out2, want)
	}
}

func TestAnyDataNestedInContainer(t *testing.T) {
	mod := loadDTAny(t)
	in := `{"dta:c":{"meta":{"k":"v","n":42}}}`
	tree, err := datatree.Parse(mod, datatree.FormatJSONIETF, []byte(in))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	out, err := tree.Serialize(datatree.FormatJSONIETF)
	if err != nil {
		t.Fatalf("Serialize: %v", err)
	}
	want := `{"dta:c":{"meta":{"k":"v","n":42}}}`
	if got := string(out); got != want {
		t.Fatalf("nested mismatch:\n got: %s\nwant: %s", got, want)
	}
}

func TestAnyDataCrossFormatSerializeError(t *testing.T) {
	mod := loadDTAny(t)
	tree, err := datatree.Parse(mod, datatree.FormatJSONIETF, []byte(`{"dta:payload":{"x":1}}`))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	_, err = tree.Serialize(datatree.FormatXML)
	if err == nil {
		t.Fatal("expected cross-format error serializing JSON-parsed anydata to XML, got nil")
	}
	if !strings.Contains(err.Error(), "cross-format") {
		t.Fatalf("error = %v, want cross-format message", err)
	}
}

func TestAnyDataXMLParseUnsupported(t *testing.T) {
	mod := loadDTAny(t)
	_, err := datatree.Parse(mod, datatree.FormatXML, []byte(`<payload xmlns="urn:dta"><x>1</x></payload>`))
	if err == nil {
		t.Fatal("expected error parsing XML anydata (opaque XML unsupported), got nil")
	}
	if !strings.Contains(err.Error(), "anydata") && !strings.Contains(err.Error(), "anyxml") {
		t.Fatalf("error = %v, want anydata/anyxml unsupported message", err)
	}
}

func TestAnyDataMandatoryValidation(t *testing.T) {
	const sch = `module dtm {
    namespace "urn:dtm";
    prefix dtm;
    yang-version 1.1;
    container top {
        anydata payload { mandatory true; }
    }
}`
	b, err := cambium.NewContextBuilder(cambium.ContextFlags{})
	if err != nil {
		t.Fatal(err)
	}
	if err := b.LoadModuleStr(sch); err != nil {
		t.Fatalf("LoadModuleStr: %v", err)
	}
	ctx, err := b.Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	t.Cleanup(func() { ctx.Close() })
	mod, err := ctx.Schema("dtm")
	if err != nil {
		t.Fatalf("Schema: %v", err)
	}

	// Present mandatory anydata validates clean (no inner schema check).
	present, err := datatree.Parse(mod, datatree.FormatJSONIETF, []byte(`{"dtm:top":{"payload":{"anything":[1,2]}}}`))
	if err != nil {
		t.Fatalf("Parse present: %v", err)
	}
	if err := present.Validate(); err != nil {
		t.Fatalf("Validate(present) = %v, want nil", err)
	}

	// Absent mandatory anydata is a violation.
	absent, err := datatree.Parse(mod, datatree.FormatJSONIETF, []byte(`{"dtm:top":{}}`))
	if err != nil {
		t.Fatalf("Parse absent: %v", err)
	}
	if err := absent.Validate(); err == nil {
		t.Fatal("Validate(absent) = nil, want missing-mandatory violation")
	}
}

func TestAnyDataNodeAccessors(t *testing.T) {
	mod := loadDTAny(t)
	tree, err := datatree.Parse(mod, datatree.FormatJSONIETF, []byte(`{"dta:payload": { "x" : 1 }, "dta:raw": true}`))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	var payload, raw datatree.Node
	for _, nd := range tree.RootNodes() {
		switch nd.Name() {
		case "payload":
			payload = nd
		case "raw":
			raw = nd
		}
	}
	if !payload.IsAnyData() || payload.IsAnyXML() {
		t.Fatalf("payload: IsAnyData=%v IsAnyXML=%v, want true,false", payload.IsAnyData(), payload.IsAnyXML())
	}
	if v, ok := payload.AnyValue(); !ok || v != `{"x":1}` {
		t.Fatalf("payload AnyValue = (%q,%v), want `{\"x\":1}`,true", v, ok)
	}
	if !raw.IsAnyXML() || raw.IsAnyData() {
		t.Fatalf("raw: IsAnyXML=%v IsAnyData=%v, want true,false", raw.IsAnyXML(), raw.IsAnyData())
	}
	if v, ok := raw.AnyValue(); !ok || v != `true` {
		t.Fatalf("raw AnyValue = (%q,%v), want `true`,true", v, ok)
	}
}

// TestAnydataXPathPresenceInWhenConstraint verifies that XPath presence checks
// on anydata nodes work correctly in when/must constraints. This specifically
// tests the claim that opaque xnodes created in buildXNodes are "visible to XPath
// for existence and for when/must on the node itself".
func TestAnydataXPathPresenceInWhenConstraint(t *testing.T) {
	const schWithWhen = `module awc {
		namespace "urn:awc";
		prefix awc;
		yang-version 1.1;

		anydata payload;
		leaf mustexist {
			when "../payload";
			type string;
		}
	}`

	b, err := cambium.NewContextBuilder(cambium.ContextFlags{})
	if err != nil {
		t.Fatal(err)
	}
	if err := b.LoadModuleStr(schWithWhen); err != nil {
		t.Fatalf("LoadModuleStr: %v", err)
	}
	ctx, err := b.Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	t.Cleanup(func() { ctx.Close() })
	mod, err := ctx.Schema("awc")
	if err != nil {
		t.Fatalf("Schema: %v", err)
	}

	// Case 1: payload present, mustexist present (when="../payload" evaluates true).
	// This exercises buildXNodes: the opaque anydata xnode must be present in the
	// XPath tree for the sibling reference "../payload" to resolve.
	tree, err := datatree.Parse(mod, datatree.FormatJSONIETF,
		[]byte(`{"awc:payload":{"x":1},"awc:mustexist":"yes"}`))
	if err != nil {
		t.Fatalf("Parse with payload present: %v", err)
	}
	if err := tree.Validate(); err != nil {
		t.Fatalf("Validate with payload present: %v", err)
	}

	// Case 2: payload absent, mustexist present (when="../payload" evaluates false)
	// Expected: validation error (when condition not satisfied)
	tree2, err := datatree.Parse(mod, datatree.FormatJSONIETF,
		[]byte(`{"awc:mustexist":"yes"}`))
	if err != nil {
		t.Fatalf("Parse with payload absent: %v", err)
	}
	err = tree2.Validate()
	if err == nil {
		t.Fatal("Expected validation error for when condition on absent anydata, got nil")
	}
	if !strings.Contains(err.Error(), "when condition") {
		t.Fatalf("Expected 'when condition' error, got: %v", err)
	}
}

// TestAnyDataInListEntryKeysFirst proves opaque anydata survives as compact JSON
// inside a list entry AND that keys-first ordering (I3) holds even though the anydata
// child is declared before the key in the schema.
func TestAnyDataInListEntryKeysFirst(t *testing.T) {
	const sch = `module dtl {
    namespace "urn:dtl";
    prefix dtl;
    yang-version 1.1;
    list entry {
        key "id";
        anydata content;
        leaf id { type string; }
    }
}`
	b, err := cambium.NewContextBuilder(cambium.ContextFlags{})
	if err != nil {
		t.Fatal(err)
	}
	if err := b.LoadModuleStr(sch); err != nil {
		t.Fatalf("LoadModuleStr: %v", err)
	}
	ctx, err := b.Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	t.Cleanup(func() { ctx.Close() })
	mod, err := ctx.Schema("dtl")
	if err != nil {
		t.Fatalf("Schema: %v", err)
	}

	// content is declared before id, but id is the key; output must emit id first
	// (I3), then the opaque content as compact JSON, for every entry.
	in := `{"dtl:entry":[{"content":{"a":1},"id":"k1"},{"id":"k2","content":[9,8]}]}`
	tree, err := datatree.Parse(mod, datatree.FormatJSONIETF, []byte(in))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	out, err := tree.Serialize(datatree.FormatJSONIETF)
	if err != nil {
		t.Fatalf("Serialize: %v", err)
	}
	want := `{"dtl:entry":[{"id":"k1","content":{"a":1}},{"id":"k2","content":[9,8]}]}`
	if got := string(out); got != want {
		t.Fatalf("list keys-first mismatch:\n got: %s\nwant: %s", got, want)
	}
	// Round-trip stable.
	tree2, err := datatree.Parse(mod, datatree.FormatJSONIETF, out)
	if err != nil {
		t.Fatalf("re-Parse: %v", err)
	}
	out2, err := tree2.Serialize(datatree.FormatJSONIETF)
	if err != nil {
		t.Fatalf("re-Serialize: %v", err)
	}
	if string(out2) != want {
		t.Fatalf("list round-trip not stable:\n out2: %s\nwant: %s", out2, want)
	}
}

// TestAnyXMLMandatoryValidation mirrors the anydata mandatory test for anyxml, so
// both opaque kinds are proven to honor presence/mandatory identically.
func TestAnyXMLMandatoryValidation(t *testing.T) {
	const sch = `module dtx {
    namespace "urn:dtx";
    prefix dtx;
    yang-version 1.1;
    container top {
        anyxml blob { mandatory true; }
    }
}`
	b, err := cambium.NewContextBuilder(cambium.ContextFlags{})
	if err != nil {
		t.Fatal(err)
	}
	if err := b.LoadModuleStr(sch); err != nil {
		t.Fatalf("LoadModuleStr: %v", err)
	}
	ctx, err := b.Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	t.Cleanup(func() { ctx.Close() })
	mod, err := ctx.Schema("dtx")
	if err != nil {
		t.Fatalf("Schema: %v", err)
	}

	present, err := datatree.Parse(mod, datatree.FormatJSONIETF, []byte(`{"dtx:top":{"blob":{"any":true}}}`))
	if err != nil {
		t.Fatalf("Parse present: %v", err)
	}
	if err := present.Validate(); err != nil {
		t.Fatalf("Validate(present anyxml) = %v, want nil", err)
	}

	absent, err := datatree.Parse(mod, datatree.FormatJSONIETF, []byte(`{"dtx:top":{}}`))
	if err != nil {
		t.Fatalf("Parse absent: %v", err)
	}
	if err := absent.Validate(); err == nil {
		t.Fatal("Validate(absent anyxml) = nil, want missing-mandatory violation")
	}
}

// TestAnyValueFalseOnNonAny guards the AnyValue() discriminant: leaves and
// containers must report (\"\", false), never a false-positive opaque value.
func TestAnyValueFalseOnNonAny(t *testing.T) {
	mod := loadDTAny(t)
	tree, err := datatree.Parse(mod, datatree.FormatJSONIETF, []byte(`{"dta:payload":{"x":1},"dta:c":{"meta":{"k":1}},"dta:after":"hi"}`))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	for _, nd := range tree.RootNodes() {
		_, ok := nd.AnyValue()
		switch nd.Name() {
		case "payload": // anydata
			if !ok {
				t.Fatalf("payload AnyValue ok=false, want true")
			}
		case "after": // leaf
			if ok {
				t.Fatalf("leaf %q AnyValue ok=true, want false", nd.Name())
			}
		case "c": // container
			if ok {
				t.Fatalf("container %q AnyValue ok=true, want false", nd.Name())
			}
		}
	}
}

// TestFindDoesNotDescendAnyData proves Find resolves an anydata node but refuses
// to descend through it (opaque content is not addressable by path).
func TestFindDoesNotDescendAnyData(t *testing.T) {
	mod := loadDTAny(t)
	tree, err := datatree.Parse(mod, datatree.FormatJSONIETF, []byte(`{"dta:payload":{"x":1}}`))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	nd, ok := tree.Find("/payload")
	if !ok {
		t.Fatal("Find(/payload) = false, want the anydata node")
	}
	if !nd.IsAnyData() {
		t.Fatalf("Find(/payload) kind: IsAnyData=false, want true")
	}
	if _, ok := tree.Find("/payload/x"); ok {
		t.Fatal("Find(/payload/x) = true, want false (must not descend into opaque anydata)")
	}
}
