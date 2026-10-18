// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

package codegen_test

import (
	"strings"
	"testing"

	"github.com/signalbreak-labs/cambium/go/cambium"
	"github.com/signalbreak-labs/cambium/go/codegen"
)

const restrictedLeafListSchema = `module restricted-lists {
 yang-version 1.1; namespace "urn:restricted-lists"; prefix r;
 typedef label { type string { length '2..4'; pattern '[a-z]+'; } }
 leaf-list labels { type label; default ba; default ab; }
 leaf-list user-labels { ordered-by user; type label; default ba; default ab; }
 leaf-list lengths { type string { length '2..4'; } }
 leaf-list user-lengths { ordered-by user; type string { length '2..4'; } }
 leaf-list references { type leafref { path '../target'; require-instance false; } }
 leaf-list signed { type int8 { range '-10..10'; } default 2; default -2; }
 leaf-list user-signed { ordered-by user; type int8 { range '-10..10'; } }
 leaf-list unsigned { type uint16 { range '1..1000'; } }
 leaf-list user-unsigned { ordered-by user; type uint16 { range '1..1000'; } }
 leaf-list wide { type int64 { range 'min..-1 | 1..max'; } }
 leaf-list huge { type uint64 { range '1..max'; } }
 leaf-list upper { type uint64 { range '9223372036854775808..18446744073709551615'; } }
 leaf-list decimals { type decimal64 { fraction-digits 2; range '-10..10'; } }
 leaf-list user-decimals { ordered-by user; type decimal64 { fraction-digits 2; range '-10..10'; } }
 leaf-list plain { type string; }
 leaf-list flags { type boolean; }
 leaf-list small { type int8; }
 leaf-list plain-wide { type int64; }
 leaf target { type label; }
 leaf upper-leaf { type uint64 { range '9223372036854775808..max'; } }
 choice selected {
  case first {
   leaf-list chosen { type label; }
   leaf-list user-chosen { ordered-by user; type string { length '2..4'; } }
  }
  case second { leaf other { type string; } }
 }
 container nested {
  leaf-list labels { type label; }
  leaf-list ports { ordered-by user; type uint16 { range '1..1000'; } }
 }
 list row {
  key id; unique tag;
  leaf tag { type label; }
  leaf id { type uint8 { range '1..10'; } }
  leaf-list labels { type label; }
 }
}`

func TestGeneratedRestrictedLeafListScalars(t *testing.T) {
	// Compile ordinary public Build/GenerateGo output before testing any fault
	// injection. This used to fail with invalid conversions to struct wrappers.
	src := generateInlineNamesModule(t, "restricted-lists", restrictedLeafListSchema)
	runGeneratedGoTest(t, src, `
func TestRestrictedLeafListRoundTrip(t *testing.T) {
 n,err:=FromJSONIETF([]byte("{\"restricted-lists:labels\":[\"ba\",\"ab\"]}"))
 if err!=nil { t.Fatal(err) }
 if err:=n.Validate();err!=nil { t.Fatal(err) }
 if len(n.Labels)!=2 || n.Labels[0].String()!="ba" || n.Labels[1].String()!="ab" { t.Fatalf("parsed values: %v",n.Labels) }
 want:="{\n  \"restricted-lists:labels\": [\n    \"ab\",\n    \"ba\"\n  ]\n}\n"
 if got:=n.ToJSONIETF();got!=want { t.Fatalf("JSON: %s",got) }
 if got:=n.ToXML();got!="<labels xmlns=\"urn:restricted-lists\">ab</labels>\n<labels xmlns=\"urn:restricted-lists\">ba</labels>\n" { t.Fatalf("XML: %s",got) }
}
`+restrictedLeafListSemantics, `"strings"`, `"errors"`)
}

// Each case checks an ordinary parse, typed validation, and complete JSON/XML
// output with distinct values so sorting and preservation cannot pass by chance.
const restrictedLeafListSemantics = `
func TestScalarCollectionEncodings(t *testing.T) {
 for _,tt:=range []struct{name,input string; json,xml []string}{
  {"labels","[\"ba\",\"ab\"]",[]string{"\"ab\"","\"ba\""},[]string{"ab","ba"}},
  {"user-labels","[\"ba\",\"ab\"]",[]string{"\"ba\"","\"ab\""},[]string{"ba","ab"}},
  {"lengths","[\"éa\",\"ab\"]",[]string{"\"ab\"","\"éa\""},[]string{"ab","éa"}},
  {"user-lengths","[\"éa\",\"ab\"]",[]string{"\"éa\"","\"ab\""},[]string{"éa","ab"}},
  {"references","[\"ba\",\"ab\"]",[]string{"\"ab\"","\"ba\""},[]string{"ab","ba"}},
  {"signed","[10,-10,2]",[]string{"-10","2","10"},[]string{"-10","2","10"}},
  {"user-signed","[10,-10,2]",[]string{"10","-10","2"},[]string{"10","-10","2"}},
  {"unsigned","[1000,2,10]",[]string{"2","10","1000"},[]string{"2","10","1000"}},
  {"user-unsigned","[1000,2,10]",[]string{"1000","2","10"},[]string{"1000","2","10"}},
  {"wide","[\"9223372036854775807\",\"-9223372036854775808\",\"2\"]",[]string{"\"-9223372036854775808\"","\"2\"","\"9223372036854775807\""},[]string{"-9223372036854775808","2","9223372036854775807"}},
  {"huge","[\"18446744073709551615\",\"2\",\"10\"]",[]string{"\"2\"","\"10\"","\"18446744073709551615\""},[]string{"2","10","18446744073709551615"}},
  {"upper","[\"18446744073709551615\",\"9223372036854775808\"]",[]string{"\"9223372036854775808\"","\"18446744073709551615\""},[]string{"9223372036854775808","18446744073709551615"}},
  {"decimals","[\"2.50\",\"-1.00\",\"0.25\"]",[]string{"\"-1.0\"","\"0.25\"","\"2.5\""},[]string{"-1.0","0.25","2.5"}},
  {"user-decimals","[\"2.50\",\"-1.00\"]",[]string{"\"2.5\"","\"-1.0\""},[]string{"2.5","-1.0"}},
  {"plain","[\"ba\",\"ab\"]",[]string{"\"ab\"","\"ba\""},[]string{"ab","ba"}},
  {"flags","[true,false]",[]string{"false","true"},[]string{"false","true"}},
  {"small","[127,-128,2]",[]string{"-128","2","127"},[]string{"-128","2","127"}},
  {"plain-wide","[\"9223372036854775807\",\"-9223372036854775808\"]",[]string{"\"-9223372036854775808\"","\"9223372036854775807\""},[]string{"-9223372036854775808","9223372036854775807"}},
  {"chosen","[\"ba\",\"ab\"]",[]string{"\"ab\"","\"ba\""},[]string{"ab","ba"}},
  {"user-chosen","[\"ba\",\"ab\"]",[]string{"\"ba\"","\"ab\""},[]string{"ba","ab"}},
 } { t.Run(tt.name,func(t *testing.T){
  n,err:=FromJSONIETF([]byte("{\"restricted-lists:"+tt.name+"\":"+tt.input+"}"));if err!=nil{t.Fatal(err)}
  if err:=n.Validate();err!=nil{t.Fatal(err)}
  want:="{\n  \"restricted-lists:"+tt.name+"\": [\n    "+strings.Join(tt.json,",\n    ")+"\n  ]\n}\n"
  if got:=n.ToJSONIETF();got!=want{t.Fatalf("JSON got %q want %q",got,want)}
  var xml strings.Builder
  for _,value:=range tt.xml{xml.WriteString("<"+tt.name+" xmlns=\"urn:restricted-lists\">"+value+"</"+tt.name+">\n")}
  if got:=n.ToXML();got!=xml.String(){t.Fatalf("XML got %q want %q",got,xml.String())}
  again,err:=FromJSONIETF([]byte(want));if err!=nil{t.Fatal(err)}
  if again.ToJSONIETF()!=want || again.ToXML()!=xml.String(){t.Fatal("round trip changed canonical values/order")}
 }) }
}

func TestRestrictedLeafListValuesAndRejections(t *testing.T) {
 n,err:=FromJSONIETF([]byte("{\"restricted-lists:wide\":[\"-9223372036854775808\",\"9223372036854775807\"],\"restricted-lists:huge\":[\"18446744073709551615\"],\"restricted-lists:user-signed\":[10,-10]}"));if err!=nil{t.Fatal(err)}
 first,ok:=n.UserSigned.Get(0);if !ok||first.Get()!=10||n.Wide[0].Get()!=-9223372036854775808||n.Wide[1].Get()!=9223372036854775807||n.Huge[0].Get()!=18446744073709551615{t.Fatal("typed exact values changed")}
 for _,tt:=range []struct{name,value,diagnostic string}{
  {"labels","[\"a\"]","length 1 out of bounds"},
  {"user-labels","[\"abcde\"]","length 5 out of bounds"},
  {"labels","[\"AB\"]","pattern violation"},
  {"lengths","[\"a\"]","length 1 out of bounds"},
  {"user-lengths","[\"abcde\"]","length 5 out of bounds"},
  {"references","[\"a\"]","length 1 out of bounds"},
  {"references","[\"AB\"]","pattern violation"},
  {"signed","[-11]","out of range"}, {"user-signed","[11]","out of range"},
  {"signed","[128]",""}, {"signed","[-129]",""},
  {"unsigned","[0]","out of range"}, {"user-unsigned","[1001]","out of range"},
  {"unsigned","[65536]",""}, {"unsigned","[-1]",""},
  {"wide","[\"0\"]","out of range"}, {"wide","[\"9223372036854775808\"]",""},
  {"wide","[\"-9223372036854775809\"]",""}, {"wide","[1]",""},
  {"huge","[\"0\"]","out of range"}, {"huge","[\"18446744073709551616\"]",""},
  {"huge","[\"-1\"]",""}, {"huge","[1]",""},
  {"upper","[\"9223372036854775807\"]","out of range"},
  {"decimals","[\"10.01\"]",""}, {"user-decimals","[\"0.001\"]",""},
  {"chosen","[\"a\"]","length 1 out of bounds"}, {"user-chosen","[\"a\"]","length 1 out of bounds"},
  {"labels","[\"ab\",\"ab\"]","duplicate"},
 } {t.Run(tt.name+tt.value,func(t *testing.T){_,err:=FromJSONIETF([]byte("{\"restricted-lists:"+tt.name+"\":"+tt.value+"}"));if err==nil||!strings.Contains(err.Error(),tt.diagnostic){t.Fatalf("accepted invalid value/lost diagnostic: %v",err)}})}
 for _,value:=range []string{"a","abcde"}{if _,err:=NewRestrictedListsLabelsLength(value);err==nil{t.Fatalf("constructor accepted %q",value)}}
 if _,err:=NewRestrictedListsSignedRange(11);err==nil{t.Fatal("signed constructor accepted invalid value")}
 if _,err:=NewRestrictedListsUnsignedRange(0);err==nil{t.Fatal("unsigned constructor accepted invalid value")}
 if got:=DefaultRestrictedListsUpperRange().Get();got!=9223372036854775808{t.Fatalf("upper-half default truncated: %d",got)}
 if _,err:=NewRestrictedListsUpperRange(9223372036854775807);err==nil{t.Fatal("upper-half constructor accepted below minimum")}
 direct,err:=FromJSONIETF([]byte("{\"restricted-lists:upper-leaf\":\"18446744073709551615\"}"));if err!=nil{t.Fatal(err)}
 if direct.UpperLeaf==nil||direct.UpperLeaf.Get()!=18446744073709551615{t.Fatal("direct uint64 wrapper truncated")}
 if got:=direct.ToJSONIETF();got!="{\n  \"restricted-lists:upper-leaf\": \"18446744073709551615\"\n}\n"{t.Fatalf("direct uint64 JSON: %s",got)}
 for _,tt:=range []struct{n *RestrictedLists;path string}{
  {&RestrictedLists{Labels:[]RestrictedListsLabelsLength{{}}},"/restricted-lists/labels"},
  {&RestrictedLists{UserLabels:NewUserOrderedVec([]RestrictedListsUserLabelsLength{{}})},"/restricted-lists/user-labels"},
  {&RestrictedLists{Unsigned:[]RestrictedListsUnsignedRange{{}}},"/restricted-lists/unsigned"},
  {&RestrictedLists{Upper:[]RestrictedListsUpperRange{{}}},"/restricted-lists/upper"},
  {&RestrictedLists{Chosen:[]RestrictedListsChosenLength{{}}},"/restricted-lists/selected/first/chosen"},
 } {err:=tt.n.Validate();if _,ok:=errors.AsType[*ValidationError](err);!ok||!strings.Contains(err.Error(),tt.path){t.Fatalf("invalid zero wrapper: %v",err)}}
 badPattern,err:=NewRestrictedListsLabelsLength("AB");if err!=nil{t.Fatal(err)}
 if err:=(&RestrictedLists{Labels:[]RestrictedListsLabelsLength{badPattern}}).Validate();err==nil||!strings.Contains(err.Error(),"pattern violation"){t.Fatalf("pattern skipped: %v",err)}
}

func TestRestrictedCollectionsInRecordsAndDefaults(t *testing.T) {
 n,err:=FromJSONIETF([]byte("{\"restricted-lists:nested\":{\"labels\":[\"ba\",\"ab\"],\"ports\":[1000,2]},\"restricted-lists:row\":[{\"tag\":\"bb\",\"id\":10,\"labels\":[\"ba\",\"ab\"]},{\"tag\":\"aa\",\"id\":2,\"labels\":[\"dc\",\"cd\"]}]}"));if err!=nil{t.Fatal(err)}
 want:="{\n  \"restricted-lists:nested\": {\n    \"labels\": [\n      \"ab\",\n      \"ba\"\n    ],\n    \"ports\": [\n      1000,\n      2\n    ]\n  },\n  \"restricted-lists:row\": [\n    {\n      \"id\": 2,\n      \"tag\": \"aa\",\n      \"labels\": [\n        \"cd\",\n        \"dc\"\n      ]\n    },\n    {\n      \"id\": 10,\n      \"tag\": \"bb\",\n      \"labels\": [\n        \"ab\",\n        \"ba\"\n      ]\n    }\n  ]\n}\n"
 if got:=n.ToJSONIETF();got!=want{t.Fatalf("nested/list JSON: %s",got)}
 wantXML:="<nested xmlns=\"urn:restricted-lists\">\n  <labels>ab</labels>\n  <labels>ba</labels>\n  <ports>1000</ports>\n  <ports>2</ports>\n</nested>\n<row xmlns=\"urn:restricted-lists\">\n  <id>2</id>\n  <tag>aa</tag>\n  <labels>cd</labels>\n  <labels>dc</labels>\n</row>\n<row xmlns=\"urn:restricted-lists\">\n  <id>10</id>\n  <tag>bb</tag>\n  <labels>ab</labels>\n  <labels>ba</labels>\n</row>\n"
 if got:=n.ToXML();got!=wantXML{t.Fatalf("nested/list XML: %q",got)}
 for _,doc:=range []string{
 "{\"restricted-lists:nested\":{\"labels\":[\"a\"]}}",
 "{\"restricted-lists:nested\":{\"ports\":[0]}}",
 "{\"restricted-lists:row\":[{\"id\":0}]}",
 "{\"restricted-lists:row\":[{\"id\":1,\"tag\":\"aa\"},{\"id\":2,\"tag\":\"aa\"}]}",
 "{\"restricted-lists:chosen\":[\"ab\"],\"restricted-lists:other\":\"selected\"}",
 "{\"restricted-lists:target\":\"a\"}",
 } {if _,err:=FromJSONIETF([]byte(doc));err==nil{t.Fatalf("accepted invalid placement/control: %s",doc)}}
 empty:=RestrictedLists{}
 defaults:=empty.ToJSONIETFWithDefaults(WithDefaultsAll)
 wantDefaults:="{\n  \"restricted-lists:labels\": [\n    \"ab\",\n    \"ba\"\n  ],\n  \"restricted-lists:user-labels\": [\n    \"ba\",\n    \"ab\"\n  ],\n  \"restricted-lists:signed\": [\n    -2,\n    2\n  ]\n}\n"
 if defaults!=wantDefaults{t.Fatalf("defaults: %s",defaults)}
 parsed,err:=FromJSONIETF([]byte(defaults));if err!=nil{t.Fatal(err)}
 if got:=parsed.ToJSONIETFWithDefaults(WithDefaultsTrim);got!="{\n}\n"{t.Fatalf("trim: %s",got)}
 if got:=empty.ToJSONIETFWithDefaults(WithDefaultsAllTagged);got!=wantDefaults{t.Fatalf("tagged defaults: %s",got)}
 reordered,err:=FromJSONIETF([]byte("{\"restricted-lists:labels\":[\"ba\",\"ab\"],\"restricted-lists:user-labels\":[\"ab\",\"ba\"],\"restricted-lists:signed\":[2,-2]}"));if err!=nil{t.Fatal(err)}
 if got:=reordered.ToJSONIETFWithDefaults(WithDefaultsTrim);got!="{\n  \"restricted-lists:user-labels\": [\n    \"ab\",\n    \"ba\"\n  ]\n}\n"{t.Fatalf("trim ordering policy: %s",got)}
}
`

func TestGeneratedLengthLeafListMetadataBeforeValues(t *testing.T) {
	const schema = `typedef bounded { type string { length '2..4'; pattern 'a+'; } }
 leaf-list values { type bounded; }
 leaf-list user-values { ordered-by user; type bounded; }
 choice selected { case first {
  leaf-list chosen { type bounded; }
  leaf-list user-chosen { ordered-by user; type bounded; }
 } case second { leaf other { type string; } } }
 container nested { leaf-list values { type bounded; } }`
	src := generatePatternModule(t, "metadata-lists", schema)
	if !strings.Contains(src, "Version: 1,") {
		t.Fatal("portable pattern metadata missing")
	}
	malformed := strings.ReplaceAll(src, "Version: 1,", "Version: 99,")
	runGeneratedGoTest(t, malformed, `
func TestMetadataPrecedesInvalidLength(t *testing.T) {
 for _,tt:=range []struct{name,path string;n *MetadataLists}{
  {"values","/metadata-lists/values",&MetadataLists{Values:[]MetadataListsValuesLength{{}}}},
  {"user-values","/metadata-lists/user-values",&MetadataLists{UserValues:NewUserOrderedVec([]MetadataListsUserValuesLength{{}})}},
  {"chosen","/metadata-lists/selected/first/chosen",&MetadataLists{Chosen:[]MetadataListsChosenLength{{}}}},
  {"user-chosen","/metadata-lists/selected/first/user-chosen",&MetadataLists{UserChosen:NewUserOrderedVec([]MetadataListsUserChosenLength{{}})}},
  {"nested","/metadata-lists/nested/values",&MetadataLists{Nested:MetadataListsNested{Values:[]MetadataListsNestedValuesLength{{}}}}},
 } { t.Run("validate/"+tt.name,func(t *testing.T){
  err:=tt.n.Validate();configuration,ok:=errors.AsType[*ConfigurationError](err)
  if !ok||configuration==nil||!strings.Contains(err.Error(),tt.path)||strings.Contains(err.Error(),"length 0"){t.Fatalf("metadata hidden: %v",err)}
 }) }
 for _,name:=range []string{"values","user-values","chosen","user-chosen"}{
  for _,value:=range []string{"\"\"","23"}{
   _,err:=FromJSONIETF([]byte("{\"metadata-lists:"+name+"\":["+value+"]}"))
   if _,ok:=errors.AsType[*ConfigurationError](err);!ok||!strings.Contains(err.Error(),"metadata-lists:"+name){t.Fatalf("parse metadata context lost: %v",err)}
  }
 }
 _,err:=FromJSONIETF([]byte("{\"metadata-lists:nested\":{\"values\":[\"\"]}}"))
 if _,ok:=errors.AsType[*ConfigurationError](err);!ok||!strings.Contains(err.Error(),"values"){t.Fatalf("nested parse metadata lost: %v",err)}
 // Missing and empty collections never restore or match their programs.
 if err:=(&MetadataLists{}).Validate();err!=nil{t.Fatal(err)}
 for _,doc:=range []string{"{}","{\"metadata-lists:values\":[]}","{\"metadata-lists:user-values\":[]}","{\"metadata-lists:chosen\":[]}","{\"metadata-lists:user-chosen\":[]}","{\"metadata-lists:other\":\"selected\"}"}{if _,err:=FromJSONIETF([]byte(doc));err!=nil{t.Fatalf("absent/empty activated metadata: %v",err)}}
 // Document/container/array shape errors precede per-item metadata checks.
 for _,doc:=range []string{"[]","{\"metadata-lists:nested\":23}","{\"metadata-lists:values\":23}"}{
  _,err:=FromJSONIETF([]byte(doc));if err==nil{t.Fatal("accepted malformed shape")};if _,ok:=errors.AsType[*ConfigurationError](err);ok{t.Fatalf("metadata preempted shape error: %v",err)}
 }
}
`, `"errors"`, `"strings"`)
	runGeneratedGoTest(t, src, `
func TestValidMetadataKeepsLengthAndPatternDiagnostics(t *testing.T) {
 for _,tt:=range []struct{path,typ string;n *MetadataLists}{
  {"/metadata-lists/values","MetadataListsValuesLength",&MetadataLists{Values:[]MetadataListsValuesLength{{}}}},
  {"/metadata-lists/user-values","MetadataListsUserValuesLength",&MetadataLists{UserValues:NewUserOrderedVec([]MetadataListsUserValuesLength{{}})}},
  {"/metadata-lists/selected/first/chosen","MetadataListsChosenLength",&MetadataLists{Chosen:[]MetadataListsChosenLength{{}}}},
  {"/metadata-lists/selected/first/user-chosen","MetadataListsUserChosenLength",&MetadataLists{UserChosen:NewUserOrderedVec([]MetadataListsUserChosenLength{{}})}},
 } {
  err:=tt.n.Validate();if _,ok:=errors.AsType[*ValidationError](err);!ok||err.Error()!=tt.path+": length 0 out of bounds for "+tt.typ{t.Fatalf("ordinary length diagnostic changed: %v",err)}
 }
 for _,name:=range []string{"values","user-values","chosen","user-chosen"}{
  _,err:=FromJSONIETF([]byte("{\"metadata-lists:"+name+"\":[\"\"]}"));if err==nil||!strings.Contains(err.Error(),"length 0 out of bounds"){t.Fatalf("parse length diagnostic: %v",err)}
  _,err=FromJSONIETF([]byte("{\"metadata-lists:"+name+"\":[\"bb\"]}"));if err==nil||!strings.Contains(err.Error(),"pattern violation"){t.Fatalf("parse pattern diagnostic: %v",err)}
  if _,err=FromJSONIETF([]byte("{\"metadata-lists:"+name+"\":[\"aa\"]}"));err!=nil{t.Fatal(err)}
 }
 value,err:=NewMetadataListsValuesLength("bb");if err!=nil{t.Fatal(err)}
 err=(&MetadataLists{Values:[]MetadataListsValuesLength{value}}).Validate()
 if _,ok:=errors.AsType[*ValidationError](err);!ok||err.Error()!="/metadata-lists/values: pattern violation"{t.Fatalf("ordinary pattern diagnostic: %v",err)}
}
`, `"errors"`, `"strings"`)
}

func TestPlanRestrictedLeafListPreservesFieldClassification(t *testing.T) {
	builder, err := cambium.NewContextBuilder(cambium.ContextFlags{DisableSearchdirCwd: true})
	if err != nil {
		t.Fatal(err)
	}
	if err := builder.LoadModuleStr(restrictedLeafListSchema); err != nil {
		t.Fatal(err)
	}
	ctx, err := builder.Build()
	if err != nil {
		t.Fatal(err)
	}
	defer ctx.Close()
	plan, err := codegen.Plan(ctx, "restricted-lists")
	if err != nil {
		t.Fatal(err)
	}
	if plan.Version != codegen.PlanVersion {
		t.Fatalf("plan version: %q", plan.Version)
	}
	for _, record := range plan.Records {
		for _, field := range record.Fields {
			switch field.Node.Kind() {
			case cambium.SchemaNodeKindLeafList, cambium.SchemaNodeKindList, cambium.SchemaNodeKindContainer:
				if field.Type.IsNewtype {
					t.Fatalf("collection/record became scalar newtype: %s", field.Path)
				}
			case cambium.SchemaNodeKindLeaf:
				if field.WireName != "other" && !field.Type.IsNewtype {
					t.Fatalf("scalar wrapper classification lost: %s", field.Path)
				}
			}
		}
	}
	for _, tt := range []struct {
		wire, goType, jsonKind string
		base                   cambium.BaseType
	}{
		{"labels", "[]RestrictedListsLabelsLength", "String", cambium.BaseTypeString},
		{"user-labels", "UserOrderedVec[RestrictedListsUserLabelsLength]", "String", cambium.BaseTypeString},
		{"signed", "[]RestrictedListsSignedRange", "BareNumber", cambium.BaseTypeInt8},
		{"huge", "[]RestrictedListsHugeRange", "QuotedNumber", cambium.BaseTypeUint64},
		{"decimals", "[]Decimal64", "QuotedNumber", cambium.BaseTypeDecimal64},
		{"plain", "[]string", "String", cambium.BaseTypeString},
	} {
		var found bool
		for _, field := range plan.Records[0].Fields {
			if field.WireName != tt.wire {
				continue
			}
			found = true
			if field.Type.GoType != tt.goType || field.Type.JSONKind != tt.jsonKind || field.Type.Base != tt.base || field.Type.Resolved == nil || field.Optional || field.Type.IsNewtype {
				t.Fatalf("collection plan changed for %s: %#v", tt.wire, field)
			}
			if tt.wire == "labels" {
				resolved, ok := field.Type.Resolved.(cambium.ResolvedString)
				if !ok || len(resolved.Length) != 1 || len(resolved.Patterns) != 1 {
					t.Fatalf("element restrictions lost: %#v", field.Type.Resolved)
				}
			}
		}
		if !found {
			t.Fatalf("missing planned field %s", tt.wire)
		}
	}
}
