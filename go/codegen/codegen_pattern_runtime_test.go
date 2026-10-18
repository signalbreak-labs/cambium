// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

package codegen_test

import (
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"strconv"
	"strings"
	"testing"

	"github.com/signalbreak-labs/cambium/go/cambium"
	"github.com/signalbreak-labs/cambium/go/codegen"
	"github.com/signalbreak-labs/cambium/go/yangpattern"
)

func generatePatternModule(t *testing.T, name, content string) string {
	t.Helper()
	builder, err := cambium.NewContextBuilder(cambium.ContextFlags{DisableSearchdirCwd: true})
	if err != nil {
		t.Fatal(err)
	}
	if err := builder.LoadModuleStr(`module ` + name + ` { yang-version 1.1; namespace "urn:` + name + `"; prefix p; ` + content + ` }`); err != nil {
		t.Fatal(err)
	}
	ctx, err := builder.Build()
	if err != nil {
		t.Fatal(err)
	}
	defer ctx.Close()
	src, err := codegen.GenerateGo(ctx, name)
	if err != nil {
		t.Fatal(err)
	}
	return src
}
func portablePatternCases() []struct {
	name, pattern, yes, no string
	inverted               bool
} {
	return []struct {
		name, pattern, yes, no string
		inverted               bool
	}{
		{"large", `a{1001}`, strings.Repeat("a", 1001), strings.Repeat("a", 1000), false},
		{"countedrange", `a{1001,1024}`, strings.Repeat("a", 1024), strings.Repeat("a", 1025), false},
		{"nested", `(a{11}){100}`, strings.Repeat("a", 1100), strings.Repeat("a", 1099), false},
		{"zeroone", `a{01}`, "a", "aa", false},
		{"zero", `a{00}`, "", "a", false},
		{"zerorange", `a{0001,0002}`, "aa", "aaa", false},
		{"unbounded", `a{0002,}`, "aaa", "a", false},
		{"zeronested", `(a{0011}){0100}`, strings.Repeat("a", 1100), "a", false},
		{"escaped", `a\{01\}`, "a{01}", "a", false},
		{"nullable", `(a?){1001}`, "", "b", false},
		{"empty", `[a-[a]]`, "a", "", true},
		{"unicode", `\p{L}{1001}`, strings.Repeat("Ω", 1001), strings.Repeat("1", 1001), false},
		{"subtract", `[a-z-[aeiou]]{01}`, "b", "a", false},
		{"anchors", `^a$`, "^a$", "a", false},
		{"invert", `a{1001}`, "b", strings.Repeat("a", 1001), true},
	}
}

func TestGeneratedPortablePatternSemantics(t *testing.T) {
	cases := portablePatternCases()
	var schema, body strings.Builder
	body.WriteString("func TestPortablePatterns(t *testing.T) {\n")
	for _, c := range cases {
		modifier := ""
		if c.inverted {
			modifier = "modifier invert-match;"
		}
		fmt.Fprintf(&schema, "leaf %s { type string { pattern '%s' { %s } } }\n", c.name, c.pattern, modifier)
		field := strings.ToUpper(c.name[:1]) + c.name[1:]
		fmt.Fprintf(&body, "t.Run(%q,func(t *testing.T){ good := Portable{ %s: ptr(%q) }; if err := good.Validate(); err != nil { t.Fatal(err) }; if _,err := FromJSONIETF([]byte(%q)); err != nil { t.Fatal(err) };", c.name, field, c.yes, `{"portable:`+c.name+`":`+strconv.Quote(c.yes)+`}`)
		if c.name != "empty" {
			fmt.Fprintf(&body, "bad := Portable{ %s: ptr(%q) }; if bad.Validate() == nil { t.Fatal(\"accepted invalid value\") }; if _,err := FromJSONIETF([]byte(%q)); err == nil { t.Fatal(\"parsed invalid value\") };", field, c.no, `{"portable:`+c.name+`":`+strconv.Quote(c.no)+`}`)
		}
		body.WriteString("})\n")
	}
	body.WriteString("}\n")
	runGeneratedGoTest(t, generatePatternModule(t, "portable", schema.String()), body.String())
}

func TestGeneratedPortablePatternDefaults(t *testing.T) {
	value := strings.Repeat("a", 1001)
	schema := `typedef counted { type string { pattern 'a{01001}'; } default '` + value + `'; }
 leaf inherited { type counted; }
 leaf target { type counted; }
 leaf reference { type leafref { path '../target'; } default '` + value + `'; }
 leaf nested { type union { type union { type string { pattern 'a{1001}'; } type uint16; } type boolean; } default '` + value + `'; }
 leaf-list ordered { type union { type string { pattern 'a{1001}'; } type uint16; } default '` + value + `'; default '2'; }
 leaf-list inserted { ordered-by user; type union { type string { pattern 'a{1001}'; } type uint16; } default '` + value + `'; default '2'; }`
	src := generatePatternModule(t, "defaults", schema)
	expectedJSON := fmt.Sprintf("{\n  \"defaults:inherited\": %s,\n  \"defaults:target\": %s,\n  \"defaults:reference\": %s,\n  \"defaults:nested\": %s,\n  \"defaults:ordered\": [\n    2,\n    %s\n  ],\n  \"defaults:inserted\": [\n    %s,\n    2\n  ]\n}\n", strconv.Quote(value), strconv.Quote(value), strconv.Quote(value), strconv.Quote(value), strconv.Quote(value), strconv.Quote(value))
	var expectedXML strings.Builder
	for _, item := range []struct{ name, value string }{{"inherited", value}, {"target", value}, {"reference", value}, {"nested", value}, {"ordered", "2"}, {"ordered", value}, {"inserted", value}, {"inserted", "2"}} {
		fmt.Fprintf(&expectedXML, "<%s xmlns=\"urn:defaults\">%s</%s>\n", item.name, item.value, item.name)
	}
	body := fmt.Sprintf(`func TestDefaults(t *testing.T) {
 n := Defaults{}
 got := n.ToJSONIETFWithDefaults(WithDefaultsAll)
 if got != %q { t.Fatalf("defaults JSON mismatch: %%s",got) }
 parsed,err := FromJSONIETF([]byte(got)); if err != nil { t.Fatal(err) }
 if gotXML := parsed.ToXML(); gotXML != %q { t.Fatalf("defaults XML mismatch: %%s",gotXML) }
 if gotTrim := parsed.ToJSONIETFWithDefaults(WithDefaultsTrim); gotTrim != "{\n}\n" { t.Fatalf("trim: %%s",gotTrim) }
}`, expectedJSON, expectedXML.String())
	runGeneratedGoTest(t, src, body)
}

func TestGeneratedPortablePatternResourceBoundaries(t *testing.T) {
	schema := `typedef bounded { type string { pattern '[ab]*a[ab]{1001}'; } }
 leaf direct { type bounded; }
 leaf native { type string { pattern '[ab]*a[ab]{1000}'; } }
 leaf inverted { type string { pattern '[ab]*a[ab]{1001}' { modifier invert-match; } } }
 leaf length { type string { length '1..max'; pattern '[ab]*a[ab]{1001}'; } }
 leaf reference { type leafref { path '../direct'; require-instance false; } }
 leaf-list values { type bounded; }
 list entries { key id; leaf id { type string; } leaf value { type bounded; } }
 leaf ordered { type union { type bounded; type string; } }
 leaf nested { type union { type union { type bounded; type uint8; } type string; } }
 leaf length-union { type union { type string { length '1..max'; pattern '[ab]*a[ab]{1001}'; } type string; } }
 leaf early { type union { type string; type bounded; } }
 leaf short { type string { pattern 'x'; pattern '[ab]*a[ab]{1001}'; } }
 leaf union-reference { type leafref { path '../ordered'; require-instance false; } }
 choice selected { case first { leaf chosen { type bounded; } leaf-list chosen-values { type bounded; } leaf chosen-length { type string { length '1..max'; pattern '[ab]*a[ab]{1001}'; } } } case second { leaf other { type string; } } }
 rpc run { input { leaf value { type bounded; } } output { leaf value { type bounded; } } }
 notification notice { leaf value { type bounded; } }`
	src := generatePatternModule(t, "bounded", schema)
	helpers := `
func testWitness() string {
 b := make([]byte,1000000); x := uint64(0x9e3779b97f4a7c15)
 for i := range b { x ^= x << 13; x ^= x >> 7; x ^= x << 17; b[i] = 'a'+byte(x&1) }
 return string(b)
}
func testResource(err error, context string) bool {
 resource, ok := errors.AsType[*ResourceError](err)
 return ok && resource.Phase == "match" && strings.Contains(err.Error(), context)
}
func testConfiguration(err error) bool { _,ok := errors.AsType[*ConfigurationError](err); return ok }
func testOrdinary(err error) bool { _,ok := errors.AsType[*ValidationError](err); return ok }
`
	// Helpers live alongside generated imports; the existing isolated test/vet
	// harness and its deadlines remain unchanged.
	body := `func TestResourceBoundaries(t *testing.T) {
 v := testWitness()
 length,err := NewBoundedLengthLength(v); if err != nil { t.Fatal(err) }
 unionLength,err := NewBoundedLengthUnionUnionStringLength(v); if err != nil { t.Fatal(err) }
 chosenLength,err := NewBoundedChosenLengthLength(v); if err != nil { t.Fatal(err) }
 direct := []struct{name string; node *Bounded}{
  {"direct",&Bounded{Direct:ptr(v)}},
  {"native",&Bounded{Native:ptr(v)}},
  {"inverted",&Bounded{Inverted:ptr(v)}},
  {"length",&Bounded{Length:&length}},
  {"reference",&Bounded{Reference:ptr(v)}},
  {"values",&Bounded{Values:[]string{v}}},
  {"entries",&Bounded{Entries:[]BoundedEntriesEntry{{Id:"key",Value:ptr(v)}}}},
  {"ordered",&Bounded{Ordered:BoundedOrderedUnionBounded(v)}},
  {"nested",&Bounded{Nested:BoundedNestedUnionUnion{Value:BoundedNestedUnionUnionUnionBounded(v)}}},
  {"length-union",&Bounded{LengthUnion:BoundedLengthUnionUnionString(unionLength)}},
  {"chosen",&Bounded{Chosen:ptr(v)}},
 {"chosen-values",&Bounded{ChosenValues:[]string{v}}},
 {"chosen-length",&Bounded{ChosenLength:&chosenLength}},
 {"union-reference",&Bounded{UnionReference:BoundedUnionReferenceUnionBounded(v)}},
 }
 for _,tt := range direct { t.Run("validate/"+tt.name,func(t *testing.T){ if err := tt.node.Validate(); !testResource(err,tt.name) { t.Fatalf("lost typed match error: %v",err) } }) }
 for _,key := range []string{"direct","native","inverted","length","reference","ordered","nested","length-union","chosen","chosen-length","union-reference"} {
  t.Run("parse/"+key,func(t *testing.T){ _,err := FromJSONIETF([]byte("{\"bounded:"+key+"\":\""+v+"\"}")); if !testResource(err,key) { t.Fatalf("lost typed match error: %v",err) } })
 }
 for _,tt := range []struct{key,doc string}{
  {"values","{\"bounded:values\":[\""+v+"\"]}"},
 {"chosen-values","{\"bounded:chosen-values\":[\""+v+"\"]}"},
  {"value","{\"bounded:entries\":[{\"id\":\"key\",\"value\":\""+v+"\"}]}"},
 } { _,err := FromJSONIETF([]byte(tt.doc)); if !testResource(err,tt.key) { t.Fatalf("collection parse: %v",err) } }
 early := Bounded{Early:BoundedEarlyUnionString(v)}; if err := early.Validate(); err != nil { t.Fatal(err) }
 if _,err := FromJSONIETF([]byte("{\"bounded:early\":\""+v+"\"}")); err != nil { t.Fatal(err) }
 short := Bounded{Short:ptr(v)}; if err:=short.Validate(); !testOrdinary(err) {t.Fatalf("early pattern mismatch evaluated later value: %v",err)}
 bad := Bounded{Direct:ptr("x")}; if err := bad.Validate(); !testOrdinary(err) || err.Error() != "/bounded/direct: pattern violation" { t.Fatalf("ordinary error changed: %v",err) }
 for _,tt := range []struct{name string; validate func()error; parse func([]byte)error}{
  {"run",func()error{return (&BoundedRunRPC{Value:ptr(v)}).Validate()},func(b []byte)error{_,err:=FromBoundedRunRPCJSONIETF(b);return err}},
  {"run",func()error{return (&BoundedRunRPCOutput{Value:ptr(v)}).Validate()},func(b []byte)error{_,err:=FromBoundedRunRPCOutputJSONIETF(b);return err}},
  {"notice",func()error{return (&BoundedNoticeNotification{Value:ptr(v)}).Validate()},func(b []byte)error{_,err:=FromBoundedNoticeNotificationJSONIETF(b);return err}},
 } {
  if err := tt.validate(); !testResource(err,"value") { t.Fatalf("operation validate: %v",err) }
  if err := tt.parse([]byte("{\"bounded:"+tt.name+"\":{\"value\":\""+v+"\"}}")); !testResource(err,"value") { t.Fatalf("operation parse: %v",err) }
 }
}
func TestRestoreBeforeValueBranches(t *testing.T) {
 valid := ProgramData{Version:FormatVersion,Nodes:[]NodeData{{Op:OpClass,Ranges:[]RuneRange{{Lo:'a',Hi:'a'}}}}}
 invalid := ProgramData{Version:FormatVersion,Nodes:[]NodeData{{Op:OpClass,Ranges:[]RuneRange{{Lo:'z',Hi:'a'}}}}}
 for _,value := range []string{"a","x"} { for _,inverted := range []bool{false,true} {
  err := cambiumValidateStringPatterns(value,[]cambiumStringPattern{{expr:"a",data:valid,inverted:inverted},{expr:"invalid",data:invalid}})
  if !testConfiguration(err) || !cambiumPatternOperational(err) { t.Fatalf("malformed later metadata hidden: %v",err) }
 } }
 oversized:=valid;oversized.Source=strings.Repeat("x",16<<20)
 if err:=cambiumValidateStringPatterns("a",[]cambiumStringPattern{{data:oversized,inverted:true}});err==nil{t.Fatal("restore resource failure hidden")}else if resource,ok:=errors.AsType[*ResourceError](err);!ok||resource.Phase!="restore"{t.Fatalf("restore phase: %v",err)}
 valid.Nodes = append(valid.Nodes,NodeData{Op:OpRepeat,Children:[]uint32{77},Min:"1",Max:"1"})
 if _,err := Restore(valid); !testConfiguration(err) { t.Fatalf("unreachable malformed node: %v",err) }
}
`
	runGeneratedGoTest(t, src, helpers+body, `"errors"`, `"strings"`)
}

func TestGeneratedPortableRuntimeIdentityAndCollisions(t *testing.T) {
	for _, tt := range []struct{ module, root string }{
		{"program", "Program2"}, {"program-data", "ProgramData2"}, {"node-data", "NodeData2"},
		{"rune-range", "RuneRange2"}, {"resource-error", "ResourceError2"}, {"configuration-error", "ConfigurationError2"},
		{"compile-limits", "CompileLimits2"}, {"restore-limits", "RestoreLimits2"}, {"match-limits", "MatchLimits2"},
		{"restore", "Restore2"}, {"op", "Op2"}, {"format-version", "FormatVersion2"},
	} {
		t.Run(tt.module, func(t *testing.T) {
			plain := generatePatternModule(t, tt.module, `leaf value { type string; }`)
			src := generatePatternModule(t, tt.module, `leaf value { type string { pattern 'a{01}'; } }`)
			declaration := "type " + tt.root + " struct"
			if !strings.Contains(plain, declaration) || !strings.Contains(src, declaration) {
				t.Fatal("helper activation renamed root")
			}
			assertGeneratedRuntimeIdentity(t, src)
			runGeneratedGoTest(t, src, fmt.Sprintf(`func TestCollision(t *testing.T) { n := %s{Value:ptr("a")};if err:=n.Validate();err!=nil { t.Fatal(err) };if _,err:=FromJSONIETF([]byte(%q));err!=nil {t.Fatal(err)} }`, tt.root, `{"`+tt.module+`:value":"a"}`))
		})
	}
	// The root's derived schema type family collides with a runtime error type.
	src := generatePatternModule(t, "resource", `container error { leaf value { type string {pattern 'a{01}';} } }`)
	runGeneratedGoTest(t, src, `func TestFamilyCollision(t *testing.T){n:=Resource{Error:ResourceError2{Value:ptr("a")}};if err:=n.Validate();err!=nil{t.Fatal(err)}}`)
}

func assertGeneratedRuntimeIdentity(t *testing.T, generated string) {
	t.Helper()
	declarations := func(source string) map[string]int {
		fs := token.NewFileSet()
		file, err := parser.ParseFile(fs, "runtime.go", source, 0)
		if err != nil {
			t.Fatal(err)
		}
		result := make(map[string]int)
		for _, decl := range file.Decls {
			if gen, ok := decl.(*ast.GenDecl); ok && gen.Tok == token.IMPORT {
				continue
			}
			var b strings.Builder
			if err := format.Node(&b, fs, decl); err != nil {
				t.Fatal(err)
			}
			result[b.String()]++
		}
		return result
	}
	actual := declarations(generated)
	sources := yangpattern.RuntimeSources()
	if len(sources) != 5 {
		t.Fatalf("runtime inventory: %d", len(sources))
	}
	for _, source := range sources {
		if !strings.Contains(generated, "// Shared yangpattern runtime: "+source.Name+"\n// SPDX-License-Identifier: Apache-2.0\n// Copyright 2026 signalbreak-labs") {
			t.Fatalf("attribution lost: %s", source.Name)
		}
		for decl, count := range declarations(string(source.Data)) {
			if actual[decl] != count {
				t.Fatalf("declaration changed/duplicated/omitted in %s:\n%s", source.Name, decl)
			}
		}
	}
	if strings.Contains(generated, "func Compile(") || strings.Contains(generated, `"github.com/signalbreak-labs/cambium`) {
		t.Fatal("generated source compiler/module dependency")
	}
}

func TestGeneratedPortableUnionMetadataBeforeBranches(t *testing.T) {
	src := generatePatternModule(t, "metadata", `leaf value { type union { type string; type union { type string { pattern 'a'; } type uint8; } } }`)
	// Fault-inject portable metadata in the generated artifact, never in the
	// immutable schema. No value match of the later arm should be necessary.
	if !strings.Contains(src, "Version: 1,") {
		t.Fatal("portable metadata missing")
	}
	src = strings.ReplaceAll(src, "Version: 1,", "Version: 99,")
	runGeneratedGoTest(t, src, `func TestMalformedLaterUnion(t *testing.T){
 n:=Metadata{Value:MetadataValueUnionString("early")}
 for _,err:=range []error{n.Validate(),func()error{_,err:=FromJSONIETF([]byte("{\"metadata:value\":\"early\"}"));return err}()} {
  if _,ok:=errors.AsType[*ConfigurationError](err);!ok{t.Fatalf("early success hid malformed later metadata: %v",err)}
 }
}`, `"errors"`)
}

func TestGeneratedPortablePatternDefaultMatrix(t *testing.T) {
	var schema, json, xml strings.Builder
	json.WriteString("{\n")
	for i, c := range portablePatternCases() {
		modifier := ""
		if c.inverted {
			modifier = "modifier invert-match;"
		}
		typ := fmt.Sprintf("type string { pattern '%s' { %s } }", c.pattern, modifier)
		fmt.Fprintf(&schema, "leaf %s { %s default '%s'; }\n", c.name, typ, c.yes)
		if i > 0 {
			json.WriteString(",\n")
		}
		fmt.Fprintf(&json, "  \"matrix:%s\": %s", c.name, strconv.Quote(c.yes))
		fmt.Fprintf(&xml, "<%s xmlns=\"urn:matrix\">%s</%s>\n", c.name, c.yes, c.name)
		if c.name == "empty" {
			continue
		} // The inverted empty language accepts every string.
		t.Run("reject-default/"+c.name, func(t *testing.T) {
			builder, err := cambium.NewContextBuilder(cambium.ContextFlags{DisableSearchdirCwd: true})
			if err != nil {
				t.Fatal(err)
			}
			if err := builder.LoadModuleStr(`module reject { yang-version 1.1; namespace "urn:reject"; prefix p; leaf value { ` + typ + ` default '` + c.no + `'; } }`); err != nil {
				t.Fatal(err)
			}
			ctx, err := builder.Build()
			if ctx != nil {
				defer ctx.Close()
			}
			if err == nil {
				t.Fatal("invalid default accepted by Build")
			}
		})
	}
	json.WriteString("\n}\n")
	src := generatePatternModule(t, "matrix", schema.String())
	body := fmt.Sprintf(`func TestDefaultMatrix(t *testing.T){
 n:=Matrix{};got:=n.ToJSONIETFWithDefaults(WithDefaultsAll);if got!=%q{t.Fatalf("JSON: %%s",got)}
 parsed,err:=FromJSONIETF([]byte(got));if err!=nil{t.Fatal(err)}
 if got:=parsed.ToXML();got!=%q{t.Fatalf("XML: %%s",got)}
}`, json.String(), xml.String())
	runGeneratedGoTest(t, src, body)
}

func TestGeneratedPortableIdentityDefaultReachability(t *testing.T) {
	b := make([]byte, 1000000)
	x := uint64(0x9e3779b97f4a7c15)
	for i := range b {
		x ^= x << 13
		x ^= x >> 7
		x ^= x << 17
		b[i] = 'a' + byte(x&1)
	}
	value := string(b)
	src := generatePatternModule(t, "identity-probe", `identity root; identity `+value+` { base root; }
 leaf value { type union { type string {pattern '[ab]*a[ab]{1001}';} type identityref {base root;} } default 'p:`+value+`'; }`)
	// Build and GenerateGo see qualified lexical text. The emitted JSON default
	// is an unqualified local identity, which changes the ordered parse trial.
	body := `func TestIdentityDefaultReachability(t *testing.T){
 b:=make([]byte,1000000);x:=uint64(0x9e3779b97f4a7c15)
 for i:=range b{x^=x<<13;x^=x>>7;x^=x<<17;b[i]='a'+byte(x&1)}
 n:=IdentityProbe{};got:=n.ToJSONIETFWithDefaults(WithDefaultsAll)
 want:="{\n  \"identity-probe:value\": \""+string(b)+"\"\n}\n"
 if got!=want{t.Fatal("identity default spelling changed")}
 _,err:=FromJSONIETF([]byte(got));resource,ok:=errors.AsType[*ResourceError](err)
 if !ok||resource.Phase!="match"{t.Fatalf("canonical identity parse lost resource error: %v",err)}
}`
	runGeneratedGoTest(t, src, body, `"errors"`)
}

func TestGeneratedPortableDirectMetadataBeforeValueChecks(t *testing.T) {
	const schema = `
 typedef counted { type string { length '1..max'; pattern 'a'; } }
 leaf direct { type counted; }
 leaf plain { type string { pattern 'a'; } }
 leaf reference { type leafref { path '../direct'; require-instance false; } }
 leaf-list values { type string { pattern 'a'; } }
 leaf-list user-values { ordered-by user; type string { pattern 'a'; } }
 leaf-list references { type leafref { path '../plain'; require-instance false; } }
 choice selected {
  case first {
   leaf chosen { type counted; }
   leaf chosen-plain { type string { pattern 'a'; } }
   leaf-list chosen-values { type string { pattern 'a'; } }
  }
  case second { leaf other { type string; } }
 }
 container required { presence present; leaf value { mandatory true; type counted; } }
 `
	src := generatePatternModule(t, "metadata-direct", schema)
	if !strings.Contains(src, "Version: 1,") {
		t.Fatal("portable metadata missing")
	}
	malformed := strings.ReplaceAll(src, "Version: 1,", "Version: 99,")
	// Corrupt only portable metadata in the generated artifact. Zero-valued public
	// wrappers make the prior length-error path reachable without schema mutation.
	runGeneratedGoTest(t, malformed, `
func TestMetadataBeforeValueChecks(t *testing.T) {
 direct := MetadataDirectDirectLength{}
 chosen := MetadataDirectChosenLength{}
 reference := MetadataDirectReferenceLength{}
 required := MetadataDirectRequiredValueLength{}
 invalidUTF8 := string([]byte{0xff})
 for _,tt:=range []struct{name string; n *MetadataDirect}{
  {"direct",&MetadataDirect{Direct:&direct}},
  {"chosen",&MetadataDirect{Chosen:&chosen}},
  {"reference",&MetadataDirect{Reference:&reference}},
  {"value",&MetadataDirect{Required:&MetadataDirectRequired{Value:required}}},
  {"plain",&MetadataDirect{Plain:ptr(invalidUTF8)}},
  {"chosen-plain",&MetadataDirect{ChosenPlain:ptr(invalidUTF8)}},
  {"values",&MetadataDirect{Values:[]string{invalidUTF8}}},
 {"user-values",&MetadataDirect{UserValues:NewUserOrderedVec([]string{invalidUTF8})}},
  {"references",&MetadataDirect{References:[]string{invalidUTF8}}},
  {"chosen-values",&MetadataDirect{ChosenValues:[]string{invalidUTF8}}},
 } { t.Run("validate/"+tt.name,func(t *testing.T){err:=tt.n.Validate();if _,ok:=errors.AsType[*ConfigurationError](err);!ok||!strings.Contains(err.Error(),tt.name){t.Fatalf("metadata hidden by value check: %v",err)}}) }
 for _,tt:=range []struct{name,doc string}{
  {"direct","{\"metadata-direct:direct\":\"\"}"},
  {"chosen","{\"metadata-direct:chosen\":\"\"}"},
  {"reference","{\"metadata-direct:reference\":\"\"}"},
  {"value","{\"metadata-direct:required\":{\"value\":\"\"}}"},
  {"plain","{\"metadata-direct:plain\":23}"},
  {"chosen-plain","{\"metadata-direct:chosen-plain\":23}"},
  {"values","{\"metadata-direct:values\":[23]}"},
 {"user-values","{\"metadata-direct:user-values\":[23]}"},
  {"references","{\"metadata-direct:references\":[23]}"},
  {"chosen-values","{\"metadata-direct:chosen-values\":[23]}"},
 } { t.Run("parse/"+tt.name,func(t *testing.T){_,err:=FromJSONIETF([]byte(tt.doc));if _,ok:=errors.AsType[*ConfigurationError](err);!ok||!strings.Contains(err.Error(),tt.name){t.Fatalf("metadata hidden by value check: %v",err)}}) }
 // Absent optional values and empty collections do not activate their metadata.
 absent:=MetadataDirect{}
 if err:=absent.Validate();err!=nil{t.Fatalf("absent values: %v",err)}
 for _,doc:=range []string{"{}","{\"metadata-direct:values\":[]}","{\"metadata-direct:user-values\":[]}","{\"metadata-direct:chosen-values\":[]}","{\"metadata-direct:other\":\"selected\"}"}{if _,err:=FromJSONIETF([]byte(doc));err!=nil{t.Fatalf("absence/other case: %v",err)}}
}
`, `"errors"`, `"strings"`)
	runGeneratedGoTest(t, src, `
func TestValidMetadataKeepsOrdinaryDiagnostics(t *testing.T) {
 direct:=MetadataDirectDirectLength{};chosen:=MetadataDirectChosenLength{}
 for _,tt:=range []struct{n *MetadataDirect;want string}{
  {&MetadataDirect{Direct:&direct},"/metadata-direct/direct: length 0 out of bounds for MetadataDirectDirectLength"},
  {&MetadataDirect{Chosen:&chosen},"/metadata-direct/selected/first/chosen: length 0 out of bounds for MetadataDirectChosenLength"},
  {&MetadataDirect{Plain:ptr("b")},"/metadata-direct/plain: pattern violation"},
 {&MetadataDirect{Plain:ptr(string([]byte{0xff}))},"/metadata-direct/plain: string value is not valid UTF-8"},
 {&MetadataDirect{ChosenPlain:ptr(string([]byte{0xff}))},"/metadata-direct/selected/first/chosen-plain: string value is not valid UTF-8"},
 {&MetadataDirect{Values:[]string{string([]byte{0xff})}},"/metadata-direct/values: string value is not valid UTF-8"},
 }{err:=tt.n.Validate();if _,ok:=errors.AsType[*ValidationError](err);!ok||err.Error()!=tt.want{t.Fatalf("ordinary diagnostic changed: %v",err)}}
 for _,name:=range []string{"direct","chosen"}{_,err:=FromJSONIETF([]byte("{\"metadata-direct:"+name+"\":\"\"}"));if err==nil||!strings.Contains(err.Error(),"length 0 out of bounds"){t.Fatalf("ordinary parse length diagnostic: %v",err)}}
 if _,err:=FromJSONIETF([]byte("{\"metadata-direct:direct\":\"a\"}"));err!=nil{t.Fatal(err)}
}
`, `"errors"`, `"strings"`)
}
