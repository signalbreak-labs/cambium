// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

package codegen_test

import (
	"path/filepath"
	"testing"

	"github.com/signalbreak-labs/cambium/go/cambium"
	"github.com/signalbreak-labs/cambium/go/codegen"
)

// generateInlineNamesModule builds a context from in-memory YANG sources (in
// the given load order) and generates Go for module.
func generateInlineNamesModule(t *testing.T, module string, sources ...string) string {
	t.Helper()
	return generateInlineNamesModuleWithSearchPath(t, "", module, sources...)
}

// generateInlineNamesModuleWithSearchPath is generateInlineNamesModule with a
// search path for imports that are not given inline.
func generateInlineNamesModuleWithSearchPath(t *testing.T, searchPath, module string, sources ...string) string {
	t.Helper()
	builder, err := cambium.NewContextBuilder(cambium.ContextFlags{})
	if err != nil {
		t.Fatal(err)
	}
	if searchPath != "" {
		if err := builder.SearchPath(searchPath); err != nil {
			t.Fatalf("SearchPath: %v", err)
		}
	}
	for _, source := range sources {
		if err := builder.LoadModuleStr(source); err != nil {
			t.Fatalf("LoadModuleStr: %v", err)
		}
	}
	ctx, err := builder.Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	defer ctx.Close()
	src, err := codegen.GenerateGo(ctx, module)
	if err != nil {
		t.Fatalf("generate %s: %v\n%s", module, err, src)
	}
	return src
}

// Distinct leaves whose flattened names collide (c/x vs top-level c-x) must
// not share one generated restriction type: each keeps its own bounds/values.
func TestGeneratedGoDistinctLeavesKeepDistinctRestrictionTypes(t *testing.T) {
	const source = `module names-restriction-merge {
    yang-version 1.1;
    namespace "urn:names-restriction-merge";
    prefix nrm;

    container c {
        leaf x { type uint8 { range "1..5"; } }
        leaf e { type enumeration { enum red; enum green; } }
        leaf s { type string { length "1..2"; } }
        leaf b { type bits { bit left; bit right; } }
        leaf u { type union { type int8 { range "1..3"; } type enumeration { enum lo; } } }
    }
    leaf c-x { type uint8 { range "1..9"; } }
    leaf c-e { type enumeration { enum up; enum down; } }
    leaf c-s { type string { length "1..5"; } }
    leaf c-b { type bits { bit top; bit bottom; } }
    leaf c-u { type union { type int8 { range "7..9"; } type enumeration { enum hi; } } }
}`
	src := generateInlineNamesModule(t, "names-restriction-merge", source)

	testBody := `
func TestGeneratedDistinctLeavesKeepDistinctRestrictionTypes(t *testing.T) {
	for _, tc := range []struct {
		in string
		ok bool
	}{
		{` + "`" + `{"names-restriction-merge:c-x":7}` + "`" + `, true},
		{` + "`" + `{"names-restriction-merge:c":{"x":7}}` + "`" + `, false},
		{` + "`" + `{"names-restriction-merge:c":{"x":5}}` + "`" + `, true},
		{` + "`" + `{"names-restriction-merge:c":{"e":"red"}}` + "`" + `, true},
		{` + "`" + `{"names-restriction-merge:c":{"e":"up"}}` + "`" + `, false},
		{` + "`" + `{"names-restriction-merge:c-e":"up"}` + "`" + `, true},
		{` + "`" + `{"names-restriction-merge:c-e":"red"}` + "`" + `, false},
		{` + "`" + `{"names-restriction-merge:c":{"s":"ab"}}` + "`" + `, true},
		{` + "`" + `{"names-restriction-merge:c":{"s":"abcd"}}` + "`" + `, false},
		{` + "`" + `{"names-restriction-merge:c-s":"abcd"}` + "`" + `, true},
		{` + "`" + `{"names-restriction-merge:c":{"b":"left"}}` + "`" + `, true},
		{` + "`" + `{"names-restriction-merge:c":{"b":"top"}}` + "`" + `, false},
		{` + "`" + `{"names-restriction-merge:c-b":"top"}` + "`" + `, true},
		{` + "`" + `{"names-restriction-merge:c-b":"left"}` + "`" + `, false},
		{` + "`" + `{"names-restriction-merge:c":{"u":2}}` + "`" + `, true},
		{` + "`" + `{"names-restriction-merge:c":{"u":8}}` + "`" + `, false},
		{` + "`" + `{"names-restriction-merge:c":{"u":"lo"}}` + "`" + `, true},
		{` + "`" + `{"names-restriction-merge:c":{"u":"hi"}}` + "`" + `, false},
		{` + "`" + `{"names-restriction-merge:c-u":8}` + "`" + `, true},
		{` + "`" + `{"names-restriction-merge:c-u":"hi"}` + "`" + `, true},
		{` + "`" + `{"names-restriction-merge:c-u":"lo"}` + "`" + `, false},
	} {
		_, err := FromJSONIETF([]byte(tc.in))
		if tc.ok && err != nil {
			t.Errorf("FromJSONIETF(%s) rejected valid input: %v", tc.in, err)
		}
		if !tc.ok && err == nil {
			t.Errorf("FromJSONIETF(%s) accepted invalid input", tc.in)
		}
	}
}
`
	runGeneratedGoTest(t, src, testBody)
}

// Generated type names that collide across nesting levels, with the
// per-struct helper names (Entry/Enum/Range/FieldOrder suffixes), or with
// enum/union member identifiers must still compile.
func TestGeneratedGoCrossLevelIdentifierCollisionsCompile(t *testing.T) {
	const source = `module names-cross-level {
    yang-version 1.1;
    namespace "urn:names-cross-level";
    prefix ncl;

    container a-b { container c { leaf v { type string; } } }
    container a { container b-c { leaf w { type int8; } } }
    list foo { key k; leaf k { type string; } }
    container foo-entry { leaf z { type string; } }
    leaf bar { type enumeration { enum x; } }
    container bar-enum { leaf y { type string; } }
    leaf q { type uint8 { range "1..5"; } }
    container q-range { leaf r { type string; } }
    container p { leaf s { type string; } }
    container p-field-order { leaf t { type string; } }
    leaf e2 { type enumeration { enum red; } }
    container e2-enum-red { leaf f { type string; } }
    leaf u { type union { type int8; type string; } }
    container u-union-int8 { leaf g { type string; } }
    container go-r-p-c { leaf h { type string; } }
    rpc go { input { leaf i { type string; } } }
}`
	src := generateInlineNamesModule(t, "names-cross-level", source)

	testBody := `
func TestGeneratedCrossLevelIdentifierCollisionsCompile(t *testing.T) {
	in := ` + "`" + `{"names-cross-level:a-b":{"c":{"v":"one"}},"names-cross-level:a":{"b-c":{"w":2}},"names-cross-level:foo":[{"k":"key"}],"names-cross-level:foo-entry":{"z":"z"},"names-cross-level:bar":"x","names-cross-level:bar-enum":{"y":"y"},"names-cross-level:q":3,"names-cross-level:q-range":{"r":"r"},"names-cross-level:p":{"s":"s"},"names-cross-level:p-field-order":{"t":"t"},"names-cross-level:e2":"red","names-cross-level:e2-enum-red":{"f":"f"},"names-cross-level:u":5,"names-cross-level:u-union-int8":{"g":"g"},"names-cross-level:go-r-p-c":{"h":"h"}}` + "`" + `
	doc, err := FromJSONIETF([]byte(in))
	if err != nil {
		t.Fatalf("FromJSONIETF: %v", err)
	}
	for _, want := range []string{"<c>\n", "<v>one</v>", "<w>2</w>", "<k>key</k>", "<z>z</z>", "<y>y</y>", "<q xmlns=\"urn:names-cross-level\">3</q>", "<r>r</r>", "<s>s</s>", "<t>t</t>", "<f>f</f>", "<g>g</g>", "<h>h</h>"} {
		if got := doc.ToXML(); !namesContains(got, want) {
			t.Fatalf("ToXML missing %q:\n%s", want, got)
		}
	}
	// The container go-r-p-c claims NamesCrossLevelGoRPC first, so the RPC
	// document is suffixed deterministically.
	rpc, err := FromNamesCrossLevelGoRPC2JSONIETF([]byte(` + "`" + `{"names-cross-level:go":{"i":"in"}}` + "`" + `))
	if err != nil {
		t.Fatalf("FromNamesCrossLevelGoRPC2JSONIETF: %v", err)
	}
	if got, want := rpc.ToXML(), "<go xmlns=\"urn:names-cross-level\">\n  <i>in</i>\n</go>\n"; got != want {
		t.Fatalf("RPC XML mismatch:\n got: %q\nwant: %q", got, want)
	}
}
`
	runGeneratedGoTest(t, src, testBody+namesContainsHelper)
}

// namesContainsHelper is appended to generated test bodies that need a
// substring check; the generated test file only imports "testing".
const namesContainsHelper = `
func namesContains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
`

// Generated names that collide with the fixed runtime helper identifiers the
// generator always reserves (CambiumStruct, WithDefaultsMode, AnyData, ...)
// must be disambiguated instead of redeclaring the helper.
func TestGeneratedGoHelperIdentifierCollisionsCompile(t *testing.T) {
	for _, tc := range []struct {
		module, source, body string
	}{
		{
			module: "cambium",
			source: `module cambium {
    namespace "urn:cambium-struct";
    prefix cs;
    container struct { leaf a { type string; } }
}`,
			body: `
func TestGeneratedHelperCollisionCambium(t *testing.T) {
	var doc CambiumStruct = &Cambium{Struct_: CambiumStruct2{A: ptr("x")}}
	if got, want := doc.ToXML(), "<struct xmlns=\"urn:cambium-struct\">\n  <a>x</a>\n</struct>\n"; got != want {
		t.Fatalf("XML mismatch:\n got: %q\nwant: %q", got, want)
	}
}
`,
		},
		{
			module: "with",
			source: `module with {
    namespace "urn:with";
    prefix w;
    container defaults-mode { leaf a { type string; default "d"; } }
}`,
			body: `
func TestGeneratedHelperCollisionWith(t *testing.T) {
	var mode WithDefaultsMode = WithDefaultsAll
	doc := &With{DefaultsMode: WithDefaultsMode2{}}
	if got, want := doc.ToJSONIETFWithDefaults(mode), "{\n  \"with:defaults-mode\": {\n    \"a\": \"d\"\n  }\n}\n"; got != want {
		t.Fatalf("JSON mismatch:\n got: %q\nwant: %q", got, want)
	}
}
`,
		},
		{
			module: "any",
			source: `module any {
    yang-version 1.1;
    namespace "urn:any";
    prefix any;
    container data { leaf a { type string; } }
    anydata blob;
}`,
			body: `
func TestGeneratedHelperCollisionAny(t *testing.T) {
	blob := NewAnyData("<k>v</k>", "{\"k\":\"v\"}")
	doc := &Any{Data: AnyData2{A: ptr("x")}, Blob: &blob}
	if got, want := doc.ToXML(), "<data xmlns=\"urn:any\">\n  <a>x</a>\n</data>\n<blob xmlns=\"urn:any\">\n  <k>v</k>\n</blob>\n"; got != want {
		t.Fatalf("XML mismatch:\n got: %q\nwant: %q", got, want)
	}
}
`,
		},
	} {
		t.Run(tc.module, func(t *testing.T) {
			src := generateInlineNamesModule(t, tc.module, tc.source)
			runGeneratedGoTest(t, src, tc.body)
		})
	}
}

// Same-named siblings contributed by different modules must not produce a
// duplicate metadata switch case, and their RFC 7952 metadata must not share a
// CambiumMetadata key: the foreign sibling is keyed by its qualified name.
func TestGeneratedGoCrossModuleSameNameSiblingsCompile(t *testing.T) {
	const base = `module names-sibling-a {
    yang-version 1.1;
    namespace "urn:names-sibling-a";
    prefix a;
    import ietf-yang-metadata { prefix md; }
    md:annotation tag { type string; }
    container top {
        leaf first { type string; }
        leaf last { type string; }
    }
}`
	const augmenting = `module names-sibling-b {
    yang-version 1.1;
    namespace "urn:names-sibling-b";
    prefix b;
    import names-sibling-a { prefix a; }
    augment "/a:top" {
        leaf first { type string; }
    }
}`
	src := generateInlineNamesModuleWithSearchPath(t, filepath.Join(schemaFixtureDir(t, "metadata-annotation-rfc7952"), "module"), "names-sibling-a", base, augmenting)

	testBody := `
func TestGeneratedCrossModuleSameNameSiblingsCompile(t *testing.T) {
	in := "{\n  \"names-sibling-a:top\": {\n    \"first\": \"a\",\n    \"@first\": {\n      \"names-sibling-a:tag\": \"own\"\n    },\n    \"last\": \"l\",\n    \"names-sibling-b:first\": \"b\",\n    \"@names-sibling-b:first\": {\n      \"names-sibling-a:tag\": \"foreign\"\n    }\n  }\n}\n"
	doc, err := FromJSONIETF([]byte(in))
	if err != nil {
		t.Fatalf("FromJSONIETF: %v", err)
	}
	if *doc.Top.First != "a" || *doc.Top.First2 != "b" {
		t.Fatalf("parsed siblings = %q, %q; want a, b", *doc.Top.First, *doc.Top.First2)
	}
	if got := doc.Top.CambiumMetadata["first"]; len(got) != 1 || got[0].Value != "own" {
		t.Fatalf("metadata for first = %+v, want own", got)
	}
	if got := doc.Top.CambiumMetadata["names-sibling-b:first"]; len(got) != 1 || got[0].Value != "foreign" {
		t.Fatalf("metadata for names-sibling-b:first = %+v, want foreign", got)
	}
	if err := doc.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if got := doc.ToJSONIETF(); got != in {
		t.Fatalf("JSON round trip mismatch:\n got: %q\nwant: %q", got, in)
	}
	want := "<top xmlns=\"urn:names-sibling-a\">\n  <first xmlns:a=\"urn:names-sibling-a\" a:tag=\"own\">a</first>\n  <last>l</last>\n  <first xmlns=\"urn:names-sibling-b\" xmlns:a=\"urn:names-sibling-a\" a:tag=\"foreign\">b</first>\n</top>\n"
	if got := doc.ToXML(); got != want {
		t.Fatalf("XML mismatch:\n got: %q\nwant: %q", got, want)
	}
}
`
	runGeneratedGoTest(t, src, testBody)
}

// When two other modules augment the same local name into a container, which
// one keeps the plain field name and metadata key is decided by module name,
// not by the order the augmenting modules were loaded in.
func TestGeneratedGoForeignSameNameSiblingsIndependentOfLoadOrder(t *testing.T) {
	const base = `module names-foreign-a {
    yang-version 1.1;
    namespace "urn:names-foreign-a";
    prefix a;
    import ietf-yang-metadata { prefix md; }
    md:annotation tag { type string; }
    container top { leaf own { type string; } }
}`
	augment := func(module string) string {
		return `module ` + module + ` {
    yang-version 1.1;
    namespace "urn:` + module + `";
    prefix p;
    import names-foreign-a { prefix a; }
    augment "/a:top" { leaf x { type string; } }
}`
	}
	b, c := augment("names-foreign-b"), augment("names-foreign-c")
	testBody := `
func TestGeneratedForeignSameNameSiblingsIndependentOfLoadOrder(t *testing.T) {
	in := ` + "`" + `{"names-foreign-a:top":{"names-foreign-c:x":"c","@names-foreign-c:x":{"names-foreign-a:tag":"from-c"},"names-foreign-b:x":"b","@names-foreign-b:x":{"names-foreign-a:tag":"from-b"}}}` + "`" + `
	doc, err := FromJSONIETF([]byte(in))
	if err != nil {
		t.Fatalf("FromJSONIETF: %v", err)
	}
	if *doc.Top.X != "b" || *doc.Top.X2 != "c" {
		t.Fatalf("fields X, X2 = %q, %q; want b, c", *doc.Top.X, *doc.Top.X2)
	}
	if got := doc.Top.CambiumMetadata["x"]; len(got) != 1 || got[0].Value != "from-b" {
		t.Fatalf("metadata for x = %+v, want from-b", got)
	}
	if got := doc.Top.CambiumMetadata["names-foreign-c:x"]; len(got) != 1 || got[0].Value != "from-c" {
		t.Fatalf("metadata for names-foreign-c:x = %+v, want from-c", got)
	}
	if err := doc.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
}
`
	searchPath := filepath.Join(schemaFixtureDir(t, "metadata-annotation-rfc7952"), "module")
	for _, order := range [][]string{{base, b, c}, {base, c, b}} {
		src := generateInlineNamesModuleWithSearchPath(t, searchPath, "names-foreign-a", order...)
		runGeneratedGoTest(t, src, testBody)
	}
}

// The document root holds top-level nodes of imported modules too, so the same
// rule applies there: the generated module's node keeps the plain metadata key
// and the imported one is keyed by qualified name, in every serializer.
func TestGeneratedGoRootLevelSameNameSiblingsKeepMetadata(t *testing.T) {
	const own = `module names-root-a {
    yang-version 1.1;
    namespace "urn:names-root-a";
    prefix a;
    import ietf-yang-metadata { prefix md; }
    import names-root-b { prefix b; }
    md:annotation tag { type string; }
    leaf first { type string; }
}`
	const imported = `module names-root-b {
    yang-version 1.1;
    namespace "urn:names-root-b";
    prefix b;
    leaf first { type string; }
}`
	testBody := `
func TestGeneratedRootLevelSameNameSiblingsKeepMetadata(t *testing.T) {
	in := "{\n  \"names-root-a:first\": \"a\",\n  \"@names-root-a:first\": {\n    \"names-root-a:tag\": \"own\"\n  },\n  \"names-root-b:first\": \"b\",\n  \"@names-root-b:first\": {\n    \"names-root-a:tag\": \"foreign\"\n  }\n}\n"
	doc, err := FromJSONIETF([]byte(in))
	if err != nil {
		t.Fatalf("FromJSONIETF: %v", err)
	}
	if got := doc.CambiumMetadata["first"]; len(got) != 1 || got[0].Value != "own" {
		t.Fatalf("metadata for first = %+v, want own", got)
	}
	if got := doc.CambiumMetadata["names-root-b:first"]; len(got) != 1 || got[0].Value != "foreign" {
		t.Fatalf("metadata for names-root-b:first = %+v, want foreign", got)
	}
	if got := doc.ToJSONIETF(); got != in {
		t.Fatalf("JSON round trip mismatch:\n got: %q\nwant: %q", got, in)
	}
	want := "<first xmlns=\"urn:names-root-a\" xmlns:a=\"urn:names-root-a\" a:tag=\"own\">a</first>\n<first xmlns=\"urn:names-root-b\" xmlns:a=\"urn:names-root-a\" a:tag=\"foreign\">b</first>\n"
	if got := doc.ToXML(); got != want {
		t.Fatalf("XML mismatch:\n got: %q\nwant: %q", got, want)
	}
}
`
	src := generateInlineNamesModuleWithSearchPath(t, filepath.Join(schemaFixtureDir(t, "metadata-annotation-rfc7952"), "module"), "names-root-a", imported, own)
	runGeneratedGoTest(t, src, testBody)
}

// Enum names are arbitrary strings; every one must become a valid, unique,
// deterministic Go identifier.
func TestGeneratedGoEnumNamesThatAreNotGoIdentifiersCompile(t *testing.T) {
	const source = `module names-enum-chars {
    yang-version 1.1;
    namespace "urn:names-enum-chars";
    prefix nec;
    leaf e {
        type enumeration {
            enum "a/b";
            enum "c d";
            enum "10GigE";
            enum "ü";
            enum "a.b";
            enum "a-b";
            enum "A_b";
            enum "x+y";
            enum "é";
            enum "$";
            enum "@";
        }
    }
}`
	src := generateInlineNamesModule(t, "names-enum-chars", source)

	testBody := `
func TestGeneratedEnumNamesThatAreNotGoIdentifiersCompile(t *testing.T) {
	names := []string{"a/b", "c d", "10GigE", "ü", "a.b", "a-b", "A_b", "x+y", "é", "$", "@"}
	consts := []NamesEnumCharsEEnum{NamesEnumCharsEEnumAB, NamesEnumCharsEEnumCD, NamesEnumCharsEEnumValue10GigE, NamesEnumCharsEEnumü, NamesEnumCharsEEnumAB2, NamesEnumCharsEEnumAB3, NamesEnumCharsEEnumAB4, NamesEnumCharsEEnumXY, NamesEnumCharsEEnumE, NamesEnumCharsEEnumNode, NamesEnumCharsEEnumNode2}
	for i, name := range names {
		parsed, ok := ParseNamesEnumCharsEEnum(name)
		if !ok || parsed != consts[i] {
			t.Fatalf("Parse(%q) = %v, %v; want %v", name, parsed, ok, consts[i])
		}
		if got := consts[i].AsName(); got != name {
			t.Fatalf("AsName() = %q, want %q", got, name)
		}
	}
}
`
	runGeneratedGoTest(t, src, testBody)
}

// Colliding names are resolved by schema identity, not by module load order:
// generating the same schema with the augmenting modules loaded in either
// order yields the same identifier for every record and field.
func TestPlanNamesIndependentOfModuleLoadOrder(t *testing.T) {
	const base = `module names-order-base {
    yang-version 1.1;
    namespace "urn:names-order-base";
    prefix base;
    container top { leaf own { type string; } }
}`
	augment := func(name string) string {
		return `module ` + name + ` {
    yang-version 1.1;
    namespace "urn:` + name + `";
    prefix ` + name + `;
    import names-order-base { prefix base; }
    augment "/base:top" {
        container x { leaf v { type string; } }
        leaf first { type enumeration { enum ` + name + `; } }
    }
}`
	}
	names := func(sources ...string) map[string]string {
		builder, err := cambium.NewContextBuilder(cambium.ContextFlags{})
		if err != nil {
			t.Fatal(err)
		}
		for _, source := range sources {
			if err := builder.LoadModuleStr(source); err != nil {
				t.Fatalf("LoadModuleStr: %v", err)
			}
		}
		ctx, err := builder.Build()
		if err != nil {
			t.Fatalf("Build: %v", err)
		}
		defer ctx.Close()
		plan, err := codegen.Plan(ctx, "names-order-base")
		if err != nil {
			t.Fatalf("Plan: %v", err)
		}
		out := make(map[string]string)
		for _, record := range plan.Records {
			out["record "+record.QualifiedPath] = record.Name
			for _, field := range record.Fields {
				out["field "+field.QualifiedPath] = field.Identifier + " " + field.Type.GoType
			}
		}
		return out
	}
	forward := names(base, augment("names-order-m1"), augment("names-order-m2"))
	reverse := names(base, augment("names-order-m2"), augment("names-order-m1"))
	if len(forward) != len(reverse) {
		t.Fatalf("plans differ in size: %d vs %d", len(forward), len(reverse))
	}
	for key, want := range forward {
		if got := reverse[key]; got != want {
			t.Errorf("%s: reverse load order named it %q, forward %q", key, got, want)
		}
	}
}
