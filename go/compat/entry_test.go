// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

package compat_test

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/signalbreak-labs/cambium/go/cambium"
	"github.com/signalbreak-labs/cambium/go/compat"
	upstream "github.com/signalbreak-labs/cambium/go/internal/yangparse/upstream/yang"
)

func TestCompatNumberLiteralShape(t *testing.T) {
	number := compat.Number{1, 0, false}
	if got := number.String(); got != "1" {
		t.Fatalf("Number{1,0,false}.String() = %q, want 1", got)
	}

	decimal := compat.Number{120, 2, false}
	if got := decimal.String(); got != "1.20" {
		t.Fatalf("Number{120,2,false}.String() = %q, want 1.20", got)
	}
}

func TestCompatListAttrLiteralShape(t *testing.T) {
	attr := compat.ListAttr{0, ^uint64(0), nil, false}
	if attr.MinElements != 0 || attr.MaxElements != ^uint64(0) || attr.OrderedBy != nil || attr.OrderedByUser {
		t.Fatalf("ListAttr literal = %#v, want goyang field order", attr)
	}
}

func TestCompatIdentityLiteralShape(t *testing.T) {
	source := &compat.Statement{Keyword: "identity", Argument: "transport", HasArgument: true}
	parent := &compat.Value{Name: "parent"}
	exts := []*compat.Statement{{Keyword: "ext:marker"}}
	base := []*compat.Value{{Name: "base"}}
	desc := &compat.Value{Name: "description"}
	ifFeature := []*compat.Value{{Name: "enabled"}}
	ref := &compat.Value{Name: "reference"}
	status := &compat.Value{Name: "current"}
	values := []*compat.Identity{{Name: "tcp"}}

	identity := compat.Identity{"transport", source, parent, exts, base, desc, ifFeature, ref, status, values}
	if identity.Name != "transport" || identity.Source != source || identity.Parent != parent || len(identity.Extensions) != 1 || len(identity.Base) != 1 || identity.Description != desc || len(identity.IfFeature) != 1 || identity.Reference != ref || identity.Status != status || len(identity.Values) != 1 {
		t.Fatalf("Identity literal = %#v, want goyang field order", identity)
	}
}

func TestCompatEntryExportedFieldShape(t *testing.T) {
	entryType := reflect.TypeOf(compat.Entry{})
	var got []string
	for i := 0; i < entryType.NumField(); i++ {
		field := entryType.Field(i)
		if field.PkgPath == "" {
			got = append(got, field.Name)
		}
	}
	want := []string{
		"Parent",
		"Node",
		"Name",
		"Description",
		"Default",
		"Units",
		"Errors",
		"Kind",
		"Config",
		"Prefix",
		"Mandatory",
		"Dir",
		"Key",
		"Type",
		"Exts",
		"ListAttr",
		"RPC",
		"Identities",
		"Augments",
		"Augmented",
		"Deviations",
		"Deviate",
		"Uses",
		"Extra",
		"Annotation",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Entry exported fields = %v, want %v", got, want)
	}
}

func TestToEntryNilMatchesGoyangErrorNode(t *testing.T) {
	compatEntry := compat.ToEntry(nil)
	upstreamEntry := upstream.ToEntry(nil)

	if len(compatEntry.Errors) != len(upstreamEntry.Errors) || len(compatEntry.Errors) != 1 {
		t.Fatalf("ToEntry(nil) error len = %d, want goyang %d", len(compatEntry.Errors), len(upstreamEntry.Errors))
	}
	if got, want := compatEntry.Errors[0].Error(), upstreamEntry.Errors[0].Error(); got != want {
		t.Fatalf("ToEntry(nil) error = %q, want goyang %q", got, want)
	}
	if compatEntry.Node == nil || upstreamEntry.Node == nil {
		t.Fatalf("ToEntry(nil) Node nil = %v, want goyang nil %v", compatEntry.Node == nil, upstreamEntry.Node == nil)
	}
	if got, want := compatEntry.Node.Kind(), upstreamEntry.Node.Kind(); got != want {
		t.Fatalf("ToEntry(nil) Node.Kind = %q, want goyang %q", got, want)
	}
	if got, want := compatEntry.Node.NName(), upstreamEntry.Node.NName(); got != want {
		t.Fatalf("ToEntry(nil) Node.NName = %q, want goyang %q", got, want)
	}
}

func TestToEntryStatementPreservesOrderAndTypedefs(t *testing.T) {
	source := `module compat-statement-entry {
    namespace "urn:compat-statement-entry";
    prefix cse;

    typedef local-text {
        type string;
        default "fallback";
    }

    grouping common {
        leaf grouped {
            type local-text;
        }
    }

    container top {
        leaf before { type string; }
        uses common;
        leaf after {
            type local-text;
        }
    }
}
`
	stmts, err := compat.Parse(source, "compat-statement-entry.yang")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(stmts) != 1 {
		t.Fatalf("Parse returned %d statements, want 1", len(stmts))
	}

	root := compat.ToEntry(stmts[0])
	if errs := root.GetErrors(); len(errs) != 0 {
		t.Fatalf("ToEntry(statement) errors = %v", errs)
	}
	top := root.Lookup("top")
	if top == nil {
		t.Fatal("ToEntry(statement) top = nil")
	}
	if got, want := childNames(top.Children()), []string{"before", "grouped", "after"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("ToEntry(statement) children = %v, want %v", got, want)
	}
	for _, name := range []string{"grouped", "after"} {
		entry := top.Lookup(name)
		if entry == nil || entry.Type == nil {
			t.Fatalf("%s entry/type = %#v", name, entry)
		}
		if got := entry.Type.Name; got != "local-text" {
			t.Fatalf("%s Type.Name = %q, want local-text", name, got)
		}
		if entry.Type.Base == nil || entry.Type.Base.Name != "string" {
			t.Fatalf("%s Type.Base = %#v, want string", name, entry.Type.Base)
		}
		if got, ok := entry.SingleDefaultValue(); !ok || got != "fallback" {
			t.Fatalf("%s SingleDefaultValue = (%q,%v), want fallback,true", name, got, ok)
		}
	}
}

func TestToEntryPreservesDeclarationOrder(t *testing.T) {
	source := `module compat-order-demo {
    yang-version 1.1;
    namespace "urn:compat-order-demo";
    prefix cod;

    identity transport;
    identity tcp { base transport; }
    identity udp { base transport; }
    extension marker {
        argument value;
    }

    container z-top {
        description "Top container.";
        config false;
        leaf z-child { type string; }
        leaf a-child {
            cod:marker "tracked";
            type string;
            default "fallback";
            units "widgets";
        }
        leaf m-child { type string; }
        leaf required {
            mandatory true;
            type uint32;
        }
        leaf ref-child {
            type leafref {
                path "../a-child";
                require-instance false;
            }
        }
        leaf union-child {
            type union {
                type string;
                type uint32;
            }
        }
        leaf ranged-int {
            type int32 {
                range "1..10 | 20..max";
            }
        }
        leaf ranged-decimal {
            type decimal64 {
                fraction-digits 2;
                range "0..100.00";
            }
        }
        leaf ranged-decimal-open {
            type decimal64 {
                fraction-digits 2;
                range "min..1.00";
            }
        }
        leaf bounded-string {
            type string {
                length "2..8";
                pattern "[a-z]+";
                pattern "[0-9]+";
            }
        }
        leaf open-string {
            type string {
                length "min..8";
            }
        }
        leaf bounded-binary {
            type binary {
                length "4..16";
            }
        }
        leaf identity-child {
            type identityref {
                base transport;
            }
        }
        leaf enum-child {
            type enumeration {
                enum down { value 2; }
                enum up { value 5; }
            }
        }
        leaf bits-child {
            type bits {
                bit read { position 0; }
                bit write { position 3; }
            }
        }
        leaf conditional {
            when "../a-child = 'fallback'" {
                description "Only shown for fallback.";
                reference "compat when";
            }
            type string;
        }
        list item {
            key "id";
            min-elements 1;
            max-elements 3;
            ordered-by user;
            leaf id { type string; }
        }
    }

    rpc do-reset {
        input {
            leaf zeta { type string; }
            leaf alpha { type string; }
        }
        output {
            leaf result { type string; }
        }
    }

    container a-top {
        leaf value { type string; }
    }

    notification alarm {
        leaf severity { type string; }
    }
}
`
	builder, err := cambium.NewContextBuilder(cambium.ContextFlags{})
	if err != nil {
		t.Fatalf("NewContextBuilder: %v", err)
	}
	if err := builder.LoadModuleStr(source); err != nil {
		t.Fatalf("LoadModuleStr: %v", err)
	}
	ctx, err := builder.Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	defer ctx.Close()

	mod, err := ctx.Schema("compat-order-demo")
	if err != nil {
		t.Fatalf("Schema: %v", err)
	}
	root := compat.FromModule(mod)
	if root == nil {
		t.Fatal("ToEntry returned nil")
	}
	if got := root.GetErrors(); len(got) != 0 {
		t.Fatalf("root GetErrors = %v, want empty", got)
	}
	root.Annotation["owner"] = "compat-test"
	if got := root.Annotation["owner"]; got != "compat-test" {
		t.Fatalf("root Annotation owner = %v, want compat-test", got)
	}
	if got, want := identityNames(root.Identities), []string{"transport", "tcp", "udp"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("root Identities = %v, want %v", got, want)
	}
	if root.Identities[0].GetValue("tcp") == nil {
		t.Fatal("transport identity GetValue(tcp) = nil")
	}
	if got, err := root.InstantiatingModule(); err != nil || got != "compat-order-demo" {
		t.Fatalf("root InstantiatingModule = (%q,%v), want compat-order-demo,nil", got, err)
	}
	rootNode := root.Node
	if rootNode == nil || rootNode.Kind() != "module" || rootNode.NName() != "compat-order-demo" {
		t.Fatalf("root Node = %#v, want module compat-order-demo", rootNode)
	}
	defaultAttr := compat.NewDefaultListAttr()
	if defaultAttr.MinElements != 0 || defaultAttr.MaxElements != ^uint64(0) {
		t.Fatalf("NewDefaultListAttr = %#v, want min 0 max MaxUint64", defaultAttr)
	}
	if got, want := childNames(root.Children()), []string{"z-top", "do-reset", "a-top", "alarm"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("root children = %v, want %v", got, want)
	}
	if got, want := childNames(root.Lookup("z-top").Children()), []string{"z-child", "a-child", "m-child", "required", "ref-child", "union-child", "ranged-int", "ranged-decimal", "ranged-decimal-open", "bounded-string", "open-string", "bounded-binary", "identity-child", "enum-child", "bits-child", "conditional", "item"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("z-top children = %v, want %v", got, want)
	}
	top := root.Lookup("z-top")
	if got := top.Description; got != "Top container." {
		t.Fatalf("z-top Description = %q, want Top container.", got)
	}
	if got := top.Config; got != compat.TSFalse {
		t.Fatalf("z-top Config = %v, want TSFalse", got)
	}
	if !top.ReadOnly() {
		t.Fatal("z-top ReadOnly() = false, want true")
	}
	if ns := top.Namespace(); ns == nil || ns.Name != "urn:compat-order-demo" {
		t.Fatalf("z-top Namespace() = %#v, want urn:compat-order-demo", ns)
	}
	if got, err := top.InstantiatingModule(); err != nil || got != "compat-order-demo" {
		t.Fatalf("z-top InstantiatingModule = (%q,%v), want compat-order-demo,nil", got, err)
	}
	aChild := top.Lookup("a-child")
	if got, want := aChild.Default, []string{"fallback"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("a-child Default = %v, want %v", got, want)
	}
	if got := aChild.Units; got != "widgets" {
		t.Fatalf("a-child Units = %q, want widgets", got)
	}
	aChildNode := aChild.Node
	if aChildNode == nil || aChildNode.Kind() != "leaf" || aChildNode.NName() != "a-child" {
		t.Fatalf("a-child Node = %#v, want leaf a-child", aChildNode)
	}
	if aChild.Type == nil || aChild.Type.Name != "string" {
		t.Fatalf("a-child Type = %#v, want string", aChild.Type)
	}
	if got := aChild.Type.Kind; got != compat.Ystring {
		t.Fatalf("a-child Type.Kind = %v, want Ystring", got)
	}
	if len(aChild.Exts) != 1 {
		t.Fatalf("a-child Exts len = %d, want 1", len(aChild.Exts))
	}
	matches, err := compat.MatchingEntryExtensions(aChild, "compat-order-demo", "marker")
	if err != nil {
		t.Fatalf("MatchingEntryExtensions: %v", err)
	}
	if len(matches) != 1 || matches[0].Argument != "tracked" {
		t.Fatalf("MatchingEntryExtensions = %#v, want tracked marker", matches)
	}
	if matches[0].Keyword != "compat-order-demo:marker" {
		t.Fatalf("MatchingEntryExtensions keyword = %q, want compat-order-demo:marker", matches[0].Keyword)
	}
	if got := aChild.Type.Default; got != "" {
		t.Fatalf("a-child Type.Default = %q, want empty for explicit leaf default", got)
	}
	if aChild.Type.HasDefault {
		t.Fatal("a-child Type.HasDefault = true, want false for explicit leaf default")
	}
	if got := aChild.Type.Units; got != "widgets" {
		t.Fatalf("a-child Type.Units = %q, want widgets", got)
	}
	required := top.Lookup("required")
	if got := required.Mandatory; got != compat.TSTrue {
		t.Fatalf("required Mandatory = %v, want TSTrue", got)
	}
	if required.Type == nil || required.Type.Name != "uint32" {
		t.Fatalf("required Type = %#v, want uint32", required.Type)
	}
	if got := required.Type.Kind; got != compat.Yuint32 {
		t.Fatalf("required Type.Kind = %v, want Yuint32", got)
	}
	refChild := top.Lookup("ref-child")
	if refChild.Type == nil || refChild.Type.Kind != compat.Yleafref {
		t.Fatalf("ref-child Type = %#v, want leafref", refChild.Type)
	}
	if got := refChild.Type.Path; got != "../a-child" {
		t.Fatalf("ref-child Type.Path = %q, want ../a-child", got)
	}
	if !refChild.Type.OptionalInstance {
		t.Fatal("ref-child OptionalInstance = false, want true")
	}
	unionChild := top.Lookup("union-child")
	if unionChild.Type == nil || unionChild.Type.Kind != compat.Yunion {
		t.Fatalf("union-child Type = %#v, want union", unionChild.Type)
	}
	if got := len(unionChild.Type.Type); got != 2 {
		t.Fatalf("union-child member count = %d, want 2", got)
	}
	if unionChild.Type.Type[0].Kind != compat.Ystring || unionChild.Type.Type[1].Kind != compat.Yuint32 {
		t.Fatalf("union-child member kinds = %#v, want string,uint32", unionChild.Type.Type)
	}
	rangedInt := top.Lookup("ranged-int")
	if rangedInt.Type == nil || rangedInt.Type.Kind != compat.Yint32 {
		t.Fatalf("ranged-int Type = %#v, want int32", rangedInt.Type)
	}
	if got := rangedInt.Type.Range.String(); got != "1..10|20..2147483647" {
		t.Fatalf("ranged-int Range = %q, want 1..10|20..2147483647", got)
	}
	rangedDecimal := top.Lookup("ranged-decimal")
	if rangedDecimal.Type == nil || rangedDecimal.Type.Kind != compat.Ydecimal64 {
		t.Fatalf("ranged-decimal Type = %#v, want decimal64", rangedDecimal.Type)
	}
	if got := rangedDecimal.Type.FractionDigits; got != 2 {
		t.Fatalf("ranged-decimal FractionDigits = %d, want 2", got)
	}
	if got := rangedDecimal.Type.Range.String(); got != "0.00..100.00" {
		t.Fatalf("ranged-decimal Range = %q, want 0.00..100.00", got)
	}
	rangedDecimalOpen := top.Lookup("ranged-decimal-open")
	if rangedDecimalOpen.Type == nil || rangedDecimalOpen.Type.Kind != compat.Ydecimal64 {
		t.Fatalf("ranged-decimal-open Type = %#v, want decimal64", rangedDecimalOpen.Type)
	}
	if got := rangedDecimalOpen.Type.Range.String(); got != "-92233720368547758.08..1.00" {
		t.Fatalf("ranged-decimal-open Range = %q, want -92233720368547758.08..1.00", got)
	}
	boundedString := top.Lookup("bounded-string")
	if boundedString.Type == nil || boundedString.Type.Kind != compat.Ystring {
		t.Fatalf("bounded-string Type = %#v, want string", boundedString.Type)
	}
	if got := boundedString.Type.Length.String(); got != "2..8" {
		t.Fatalf("bounded-string Length = %q, want 2..8", got)
	}
	if got, want := boundedString.Type.Pattern, []string{"[a-z]+", "[0-9]+"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("bounded-string Pattern = %v, want %v", got, want)
	}
	if !boundedString.Type.Equal(&compat.YangType{
		Name:    "ignored-name",
		Kind:    compat.Ystring,
		Length:  boundedString.Type.Length,
		Pattern: []string{"[a-z]+", "[0-9]+"},
	}) {
		t.Fatal("bounded-string Type.Equal returned false for equivalent string type")
	}
	if boundedString.Type.Equal(&compat.YangType{
		Kind:    compat.Ystring,
		Length:  boundedString.Type.Length,
		Pattern: []string{"[a-z]+"},
	}) {
		t.Fatal("bounded-string Type.Equal returned true for different patterns")
	}
	openString := top.Lookup("open-string")
	if openString.Type == nil || openString.Type.Kind != compat.Ystring {
		t.Fatalf("open-string Type = %#v, want string", openString.Type)
	}
	if got := openString.Type.Length.String(); got != "0..8" {
		t.Fatalf("open-string Length = %q, want 0..8", got)
	}
	boundedBinary := top.Lookup("bounded-binary")
	if boundedBinary.Type == nil || boundedBinary.Type.Kind != compat.Ybinary {
		t.Fatalf("bounded-binary Type = %#v, want binary", boundedBinary.Type)
	}
	if got := boundedBinary.Type.Length.String(); got != "4..16" {
		t.Fatalf("bounded-binary Length = %q, want 4..16", got)
	}
	identityChild := top.Lookup("identity-child")
	if identityChild.Type == nil || identityChild.Type.Kind != compat.Yidentityref {
		t.Fatalf("identity-child Type = %#v, want identityref", identityChild.Type)
	}
	if identityChild.Type.IdentityBase == nil {
		t.Fatal("identity-child Type.IdentityBase = nil")
	}
	if got := len(identityChild.Type.IdentityBases); got != 1 {
		t.Fatalf("identity-child Type.IdentityBases len = %d, want 1", got)
	}
	if got := identityChild.Type.IdentityBase.Name; got != "transport" {
		t.Fatalf("identity base name = %q, want transport", got)
	}
	if got := identityChild.Type.IdentityBase.PrefixedName(); got != "cod:transport" {
		t.Fatalf("identity base prefixed name = %q, want cod:transport", got)
	}
	if !identityChild.Type.IdentityBase.IsDefined("tcp") {
		t.Fatal("identity base IsDefined(tcp) = false, want true")
	}
	if value := identityChild.Type.IdentityBase.GetValue("udp"); value == nil || value.Name != "udp" {
		t.Fatalf("identity base GetValue(udp) = %#v, want udp", value)
	}
	if !identityChild.Type.Equal(&compat.YangType{
		Kind:          compat.Yidentityref,
		IdentityBase:  identityChild.Type.IdentityBase,
		IdentityBases: identityChild.Type.IdentityBases,
	}) {
		t.Fatal("identity-child Type.Equal returned false for equivalent identityref type")
	}
	enumChild := top.Lookup("enum-child")
	if enumChild.Type == nil || enumChild.Type.Kind != compat.Yenum {
		t.Fatalf("enum-child Type = %#v, want enumeration", enumChild.Type)
	}
	if enumChild.Type.Enum == nil {
		t.Fatal("enum-child Type.Enum = nil")
	}
	if got := enumChild.Type.Enum.Value("down"); got != 2 {
		t.Fatalf("enum down value = %d, want 2", got)
	}
	if got := enumChild.Type.Enum.Name(5); got != "up" {
		t.Fatalf("enum value 5 name = %q, want up", got)
	}
	if !enumChild.Type.Enum.IsDefined("up") {
		t.Fatal("enum up IsDefined = false, want true")
	}
	if got, want := enumChild.Type.Enum.Names(), []string{"down", "up"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("enum Names = %v, want %v", got, want)
	}
	if got, want := enumChild.Type.Enum.Values(), []int64{2, 5}; !reflect.DeepEqual(got, want) {
		t.Fatalf("enum Values = %v, want %v", got, want)
	}
	if !enumChild.Type.Equal(&compat.YangType{Kind: compat.Yenum, Enum: enumChild.Type.Enum}) {
		t.Fatal("enum-child Type.Equal returned false for equivalent enum type")
	}
	bitsChild := top.Lookup("bits-child")
	if bitsChild.Type == nil || bitsChild.Type.Kind != compat.Ybits {
		t.Fatalf("bits-child Type = %#v, want bits", bitsChild.Type)
	}
	if bitsChild.Type.Bit == nil {
		t.Fatal("bits-child Type.Bit = nil")
	}
	if got := bitsChild.Type.Bit.Value("write"); got != 3 {
		t.Fatalf("bit write position = %d, want 3", got)
	}
	if got := bitsChild.Type.Bit.Name(0); got != "read" {
		t.Fatalf("bit position 0 name = %q, want read", got)
	}
	otherBit := compat.NewBitfield()
	if err := otherBit.Set("read", 0); err != nil {
		t.Fatalf("otherBit.Set(read): %v", err)
	}
	if bitsChild.Type.Equal(&compat.YangType{Kind: compat.Ybits, Bit: otherBit}) {
		t.Fatal("bits-child Type.Equal returned true for different bits")
	}
	if !unionChild.Type.Equal(&compat.YangType{
		Kind: compat.Yunion,
		Type: []*compat.YangType{
			{Kind: compat.Ystring},
			{Kind: compat.Yuint32, Range: compat.Uint32Range},
		},
	}) {
		t.Fatal("union-child Type.Equal returned false for equivalent union members")
	}
	var nilType *compat.YangType
	if !nilType.Equal(nil) {
		t.Fatal("nil YangType Equal(nil) = false, want true")
	}
	if nilType.Equal(bitsChild.Type) {
		t.Fatal("nil YangType Equal(non-nil) = true, want false")
	}
	conditional := top.Lookup("conditional")
	if got, ok := conditional.GetWhenXPath(); !ok || got != "../a-child = 'fallback'" {
		t.Fatalf("conditional GetWhenXPath = (%q,%v), want ../a-child = 'fallback',true", got, ok)
	}
	if got, ok := aChild.GetWhenXPath(); ok || got != "" {
		t.Fatalf("a-child GetWhenXPath = (%q,%v), want empty,false", got, ok)
	}
	item := top.Lookup("item")
	if got := item.Key; got != "id" {
		t.Fatalf("item Key = %q, want id", got)
	}
	if item.ListAttr == nil {
		t.Fatal("item ListAttr = nil")
	}
	if got := item.ListAttr.MinElements; got != 1 {
		t.Fatalf("item MinElements = %d, want 1", got)
	}
	if got := item.ListAttr.MaxElements; got != 3 {
		t.Fatalf("item MaxElements = %d, want 3", got)
	}
	if !item.ListAttr.OrderedByUser {
		t.Fatal("item OrderedByUser = false, want true")
	}
	if item.ListAttr.OrderedBy == nil || item.ListAttr.OrderedBy.Name != "user" {
		t.Fatalf("item OrderedBy = %#v, want user", item.ListAttr.OrderedBy)
	}
	rpc := root.Lookup("do-reset")
	if rpc == nil || rpc.RPC == nil || rpc.RPC.Input == nil || rpc.RPC.Output == nil {
		t.Fatalf("RPC projection missing input/output: %#v", rpc)
	}
	if got, want := childNames(rpc.RPC.Input.Children()), []string{"zeta", "alpha"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("rpc input children = %v, want %v", got, want)
	}
	if got := root.Dir["a-top"]; got == nil || !got.IsContainer() {
		t.Fatalf("Dir lookup for a-top = %#v, want container entry", got)
	}
	if got := root.Find("z-top/../a-top/value"); got == nil || got.Name != "value" || !got.IsLeaf() {
		t.Fatalf("relative Find value = %#v, want leaf value", got)
	}
	if got := root.Find("/cod:z-top/cod:a-child"); got == nil || got.Name != "a-child" || !got.IsLeaf() {
		t.Fatalf("prefixed absolute Find a-child = %#v, want leaf a-child", got)
	}
	if got := root.Find("/compat-order-demo/do-reset/input/alpha"); got == nil || got.Name != "alpha" || !got.IsLeaf() {
		t.Fatalf("module-root absolute Find rpc input alpha = %#v, want leaf alpha", got)
	}
	if got := root.Find("z-top/no-such-child"); got != nil {
		t.Fatalf("Find missing child = %#v, want nil", got)
	}
	var printed bytes.Buffer
	top.Print(&printed)
	assertOrderedSubstrings(t, printed.String(), []string{"z-child", "a-child", "m-child", "required", "ref-child", "union-child", "ranged-int", "ranged-decimal", "ranged-decimal-open", "bounded-string", "open-string", "bounded-binary", "identity-child", "enum-child", "bits-child", "conditional", "item"})
}

func TestCompatLoaderPathMatchesGoyang(t *testing.T) {
	const source = `module compat-loader-path {
    namespace "urn:compat-loader-path";
    prefix clp;

    container top {
        container nested {
            leaf value { type string; }
        }
    }
}
`
	ms := compat.NewModules()
	if err := ms.Parse(source, "compat-loader-path.yang"); err != nil {
		t.Fatalf("compat Parse: %v", err)
	}
	compatRoot, errs := ms.GetModule("compat-loader-path")
	if len(errs) != 0 {
		t.Fatalf("compat GetModule errors: %v", errs)
	}
	upstreamRoot := mustUpstreamEntry(t, "compat-loader-path", source)

	for _, path := range []string{".", "top", "top/nested", "top/nested/value"} {
		compatEntry := compatRoot.Find(path)
		upstreamEntry := upstreamRoot.Find(path)
		if compatEntry == nil || upstreamEntry == nil {
			t.Fatalf("Find(%q) = (%#v,%#v), want both non-nil", path, compatEntry, upstreamEntry)
		}
		if got, want := compatEntry.Path(), upstreamEntry.Path(); got != want {
			t.Fatalf("Find(%q).Path() = %q, want goyang %q", path, got, want)
		}
	}
}

func TestMatchingEntryExtensionsResolvesImportedPrefixLikeGoyang(t *testing.T) {
	defSource := `module compat-entry-ext-def {
    namespace "urn:compat-entry-ext-def";
    prefix ceed;

    extension marker {
        argument value;
    }
}
`
	userSource := `module compat-entry-ext-user {
    namespace "urn:compat-entry-ext-user";
    prefix ceeu;

    import compat-entry-ext-def {
        prefix ceed;
    }

    container top {
        leaf tagged {
            ceed:marker "external";
            type string;
        }
    }
}
`

	compatModules := compat.NewModules()
	for name, source := range map[string]string{
		"compat-entry-ext-def.yang":  defSource,
		"compat-entry-ext-user.yang": userSource,
	} {
		if err := compatModules.Parse(source, name); err != nil {
			t.Fatalf("compat Parse(%s): %v", name, err)
		}
	}
	compatRoot, compatErrs := compatModules.GetModule("compat-entry-ext-user")
	if len(compatErrs) != 0 {
		t.Fatalf("compat GetModule errors: %v", compatErrs)
	}
	compatTagged := compatRoot.Lookup("top").Lookup("tagged")
	compatMatches, compatErr := compat.MatchingEntryExtensions(compatTagged, "compat-entry-ext-def", "marker")

	upstreamModules := upstream.NewModules()
	for name, source := range map[string]string{
		"compat-entry-ext-def.yang":  defSource,
		"compat-entry-ext-user.yang": userSource,
	} {
		if err := upstreamModules.Parse(source, name); err != nil {
			t.Fatalf("upstream Parse(%s): %v", name, err)
		}
	}
	upstreamRoot, upstreamErrs := upstreamModules.GetModule("compat-entry-ext-user")
	if len(upstreamErrs) != 0 {
		t.Fatalf("upstream GetModule errors: %v", upstreamErrs)
	}
	upstreamTagged := upstreamRoot.Dir["top"].Dir["tagged"]
	upstreamMatches, upstreamErr := upstream.MatchingEntryExtensions(upstreamTagged, "compat-entry-ext-def", "marker")

	if compatErr != nil || upstreamErr != nil {
		t.Fatalf("MatchingEntryExtensions errors = (%v,%v), want both nil", compatErr, upstreamErr)
	}
	if len(compatMatches) != len(upstreamMatches) || len(compatMatches) != 1 {
		t.Fatalf("MatchingEntryExtensions = (%#v,%#v), want one match on both", compatMatches, upstreamMatches)
	}
	if compatMatches[0].Argument != upstreamMatches[0].Argument {
		t.Fatalf("MatchingEntryExtensions argument = %q, want goyang %q", compatMatches[0].Argument, upstreamMatches[0].Argument)
	}
}

func TestCompatEntryFindImportedAbsolutePrefixMatchesGoyang(t *testing.T) {
	importedSource := `module compat-entry-find-imported {
    namespace "urn:compat-entry-find-imported";
    prefix cefi;

    container remote {
        leaf value { type string; }
    }
}
`
	mainSource := `module compat-entry-find-main {
    namespace "urn:compat-entry-find-main";
    prefix cefm;

    import compat-entry-find-imported {
        prefix cefi;
    }

    container local {
        leaf value { type string; }
    }
}
`
	sources := map[string]string{
		"compat-entry-find-imported.yang": importedSource,
		"compat-entry-find-main.yang":     mainSource,
	}

	compatModules := compat.NewModules()
	for name, source := range sources {
		if err := compatModules.Parse(source, name); err != nil {
			t.Fatalf("compat Parse(%s): %v", name, err)
		}
	}
	compatRoot, compatErrs := compatModules.GetModule("compat-entry-find-main")
	if len(compatErrs) != 0 {
		t.Fatalf("compat GetModule errors: %v", compatErrs)
	}

	upstreamModules := upstream.NewModules()
	for name, source := range sources {
		if err := upstreamModules.Parse(source, name); err != nil {
			t.Fatalf("upstream Parse(%s): %v", name, err)
		}
	}
	upstreamRoot, upstreamErrs := upstreamModules.GetModule("compat-entry-find-main")
	if len(upstreamErrs) != 0 {
		t.Fatalf("upstream GetModule errors: %v", upstreamErrs)
	}

	path := "/cefi:remote/cefi:value"
	compatFound := compatRoot.Find(path)
	upstreamFound := upstreamRoot.Find(path)
	if upstreamFound == nil {
		t.Fatalf("upstream Find(%q) = nil, expected imported leaf", path)
	}
	if compatFound == nil {
		t.Fatalf("compat Find(%q) = nil, want goyang %s", path, upstreamFound.Name)
	}
	if compatFound.Name != upstreamFound.Name || compatFound.Kind.String() != upstreamFound.Kind.String() {
		t.Fatalf("compat Find(%q) = %s/%s, want goyang %s/%s", path, compatFound.Name, compatFound.Kind, upstreamFound.Name, upstreamFound.Kind)
	}
}

func TestCompatEntryFindSearchPathImportedAbsolutePrefixMatchesGoyang(t *testing.T) {
	dir := t.TempDir()
	importedPath := filepath.Join(dir, "compat-entry-find-search-imported.yang")
	importedSource := `module compat-entry-find-search-imported {
    namespace "urn:compat-entry-find-search-imported";
    prefix cefsi;

    container remote {
        leaf value { type string; }
    }
}
`
	if err := os.WriteFile(importedPath, []byte(importedSource), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	mainSource := `module compat-entry-find-search-main {
    namespace "urn:compat-entry-find-search-main";
    prefix cefsm;

    import compat-entry-find-search-imported {
        prefix cefsi;
    }

    container local {
        leaf value { type string; }
    }
}
`

	compatModules := compat.NewModules()
	compatModules.AddPath(dir)
	if err := compatModules.Parse(mainSource, "compat-entry-find-search-main.yang"); err != nil {
		t.Fatalf("compat Parse: %v", err)
	}
	compatRoot, compatErrs := compatModules.GetModule("compat-entry-find-search-main")
	if len(compatErrs) != 0 {
		t.Fatalf("compat GetModule errors: %v", compatErrs)
	}

	upstreamModules := upstream.NewModules()
	upstreamModules.AddPath(dir)
	if err := upstreamModules.Parse(mainSource, "compat-entry-find-search-main.yang"); err != nil {
		t.Fatalf("upstream Parse: %v", err)
	}
	upstreamRoot, upstreamErrs := upstreamModules.GetModule("compat-entry-find-search-main")
	if len(upstreamErrs) != 0 {
		t.Fatalf("upstream GetModule errors: %v", upstreamErrs)
	}

	path := "/cefsi:remote/cefsi:value"
	compatFound := compatRoot.Find(path)
	upstreamFound := upstreamRoot.Find(path)
	if upstreamFound == nil {
		t.Fatalf("upstream Find(%q) = nil, expected imported leaf", path)
	}
	if compatFound == nil {
		t.Fatalf("compat Find(%q) = nil, want goyang %s", path, upstreamFound.Name)
	}
	if compatFound.Name != upstreamFound.Name || compatFound.Kind.String() != upstreamFound.Kind.String() {
		t.Fatalf("compat Find(%q) = %s/%s, want goyang %s/%s", path, compatFound.Name, compatFound.Kind, upstreamFound.Name, upstreamFound.Kind)
	}
}

func TestCompatEntryFindUnknownAbsolutePrefixErrorMatchesGoyang(t *testing.T) {
	source := `module compat-entry-find-unknown-prefix {
    namespace "urn:compat-entry-find-unknown-prefix";
    prefix cefup;

    container top {
        leaf value { type string; }
    }
}
`
	compatRoot := mustCompatEntry(t, "compat-entry-find-unknown-prefix", source)
	upstreamRoot := mustUpstreamEntry(t, "compat-entry-find-unknown-prefix", source)

	path := "/missing:top/missing:value"
	if got := compatRoot.Find(path); got != nil {
		t.Fatalf("compat Find(%q) = %#v, want nil", path, got)
	}
	if got := upstreamRoot.Find(path); got != nil {
		t.Fatalf("upstream Find(%q) = %#v, test fixture expected nil", path, got)
	}

	compatErrs := compatTestErrorStrings(compatRoot.GetErrors())
	upstreamErrs := compatTestErrorStrings(upstreamRoot.GetErrors())
	if !reflect.DeepEqual(compatErrs, upstreamErrs) {
		t.Fatalf("Find unknown prefix errors = %v, want goyang %v", compatErrs, upstreamErrs)
	}

	compatRoot.Errors = nil
	upstreamRoot.Errors = nil
	compatChild := compatRoot.Lookup("top")
	upstreamChild := upstreamRoot.Dir["top"]
	if got := compatChild.Find(path); got != nil {
		t.Fatalf("compat child Find(%q) = %#v, want nil", path, got)
	}
	if got := upstreamChild.Find(path); got != nil {
		t.Fatalf("upstream child Find(%q) = %#v, test fixture expected nil", path, got)
	}
	if len(compatChild.Errors) != len(upstreamChild.Errors) {
		t.Fatalf("child Find attached errors to child = %v, want goyang %v", compatChild.Errors, upstreamChild.Errors)
	}
	compatErrs = compatTestErrorStrings(compatRoot.GetErrors())
	upstreamErrs = compatTestErrorStrings(upstreamRoot.GetErrors())
	if !reflect.DeepEqual(compatErrs, upstreamErrs) {
		t.Fatalf("child Find unknown prefix errors = %v, want goyang %v", compatErrs, upstreamErrs)
	}
}

func TestCompatEntryNamespaceUnresolvedMatchesGoyang(t *testing.T) {
	compatEntry := &compat.Entry{Name: "detached"}
	upstreamEntry := &upstream.Entry{Name: "detached"}

	compatNamespace := compatEntry.Namespace()
	upstreamNamespace := upstreamEntry.Namespace()
	if (compatNamespace == nil) != (upstreamNamespace == nil) {
		t.Fatalf("Namespace nil = %v, want goyang %v", compatNamespace == nil, upstreamNamespace == nil)
	}
	if compatNamespace != nil && compatNamespace.Name != upstreamNamespace.Name {
		t.Fatalf("Namespace Name = %q, want goyang %q", compatNamespace.Name, upstreamNamespace.Name)
	}
}

func TestCompatEntryFindSyntheticRPCIOEntriesMatchesGoyang(t *testing.T) {
	source := `module compat-entry-find-rpc-io {
    namespace "urn:compat-entry-find-rpc-io";
    prefix cefri;

    rpc ping;
}
`
	compatRoot := mustCompatEntry(t, "compat-entry-find-rpc-io", source)
	upstreamRoot := mustUpstreamEntry(t, "compat-entry-find-rpc-io", source)

	for _, path := range []string{"ping/input", "ping/output"} {
		t.Run(path, func(t *testing.T) {
			compatFound := compatRoot.Find(path)
			upstreamFound := upstreamRoot.Find(path)
			if upstreamFound == nil {
				t.Fatalf("upstream Find(%q) = nil, expected synthetic operation entry", path)
			}
			if compatFound == nil {
				t.Fatalf("compat Find(%q) = nil, want goyang %s", path, upstreamFound.Name)
			}
			if compatFound.Name != upstreamFound.Name || compatFound.Kind.String() != upstreamFound.Kind.String() {
				t.Fatalf("compat Find(%q) = %s/%s, want goyang %s/%s", path, compatFound.Name, compatFound.Kind, upstreamFound.Name, upstreamFound.Kind)
			}
		})
	}
}

func childNames(entries []*compat.Entry) []string {
	out := make([]string, 0, len(entries))
	for _, entry := range entries {
		out = append(out, entry.Name)
	}
	return out
}

func compatTestErrorStrings(errs []error) []string {
	out := make([]string, 0, len(errs))
	for _, err := range errs {
		out = append(out, err.Error())
	}
	return out
}

func identityNames(identities []*compat.Identity) []string {
	out := make([]string, 0, len(identities))
	for _, identity := range identities {
		out = append(out, identity.Name)
	}
	return out
}

func assertOrderedSubstrings(t *testing.T, text string, parts []string) {
	t.Helper()
	offset := 0
	for _, part := range parts {
		idx := strings.Index(text[offset:], part)
		if idx < 0 {
			t.Fatalf("printed entry missing %q after offset %d:\n%s", part, offset, text)
		}
		offset += idx + len(part)
	}
}

func TestCompatStringersMatchGoyangSpellings(t *testing.T) {
	triStates := []struct {
		value compat.TriState
		want  string
	}{
		{compat.TSUnset, "unset"},
		{compat.TSTrue, "true"},
		{compat.TSFalse, "false"},
		{compat.TriState(99), "ts-99"},
	}
	for _, tc := range triStates {
		if got := tc.value.String(); got != tc.want {
			t.Fatalf("TriState(%d).String() = %q, want %q", tc.value, got, tc.want)
		}
	}

	entryKinds := []struct {
		value compat.EntryKind
		want  string
	}{
		{compat.LeafEntry, "Leaf"},
		{compat.DirectoryEntry, "Directory"},
		{compat.AnyDataEntry, "AnyData"},
		{compat.AnyXMLEntry, "AnyXML"},
		{compat.CaseEntry, "Case"},
		{compat.ChoiceEntry, "Choice"},
		{compat.InputEntry, "Input"},
		{compat.NotificationEntry, "Notification"},
		{compat.OutputEntry, "Output"},
		{compat.DeviateEntry, "Deviate"},
		{compat.EntryKind(99), "unknown-entry-99"},
	}
	for _, tc := range entryKinds {
		if got := tc.value.String(); got != tc.want {
			t.Fatalf("EntryKind(%d).String() = %q, want %q", tc.value, got, tc.want)
		}
	}

	typeKinds := []struct {
		value compat.TypeKind
		want  string
	}{
		{compat.Ynone, "none"},
		{compat.Yint8, "int8"},
		{compat.Yuint32, "uint32"},
		{compat.Yleafref, "leafref"},
		{compat.Ystring, "string"},
		{compat.Yunion, "union"},
		{compat.TypeKind(99), "unknown-type-99"},
	}
	for _, tc := range typeKinds {
		if got := tc.value.String(); got != tc.want {
			t.Fatalf("TypeKind(%d).String() = %q, want %q", tc.value, got, tc.want)
		}
	}
}

func TestCompatKindMappingsMatchGoyang(t *testing.T) {
	gotEntryKinds := map[uint]string{}
	for kind, name := range compat.EntryKindToName {
		gotEntryKinds[uint(kind)] = name
	}
	wantEntryKinds := map[uint]string{}
	for kind, name := range upstream.EntryKindToName {
		wantEntryKinds[uint(kind)] = name
	}
	if !reflect.DeepEqual(gotEntryKinds, wantEntryKinds) {
		t.Fatalf("EntryKindToName = %v, want goyang %v", gotEntryKinds, wantEntryKinds)
	}

	gotTypeKinds := map[uint]string{}
	for kind, name := range compat.TypeKindToName {
		gotTypeKinds[uint(kind)] = name
	}
	wantTypeKinds := map[uint]string{}
	for kind, name := range upstream.TypeKindToName {
		wantTypeKinds[uint(kind)] = name
	}
	if !reflect.DeepEqual(gotTypeKinds, wantTypeKinds) {
		t.Fatalf("TypeKindToName = %v, want goyang %v", gotTypeKinds, wantTypeKinds)
	}

	gotTypeNames := map[string]uint{}
	for name, kind := range compat.TypeKindFromName {
		gotTypeNames[name] = uint(kind)
	}
	wantTypeNames := map[string]uint{}
	for name, kind := range upstream.TypeKindFromName {
		wantTypeNames[name] = uint(kind)
	}
	if !reflect.DeepEqual(gotTypeNames, wantTypeNames) {
		t.Fatalf("TypeKindFromName = %v, want goyang %v", gotTypeNames, wantTypeNames)
	}
}

func TestCompatScalarConstantsMatchGoyang(t *testing.T) {
	gotTriStates := map[string]int{
		"TSUnset": int(compat.TSUnset),
		"TSTrue":  int(compat.TSTrue),
		"TSFalse": int(compat.TSFalse),
	}
	wantTriStates := map[string]int{
		"TSUnset": int(upstream.TSUnset),
		"TSTrue":  int(upstream.TSTrue),
		"TSFalse": int(upstream.TSFalse),
	}
	if !reflect.DeepEqual(gotTriStates, wantTriStates) {
		t.Fatalf("TriState constants = %v, want goyang %v", gotTriStates, wantTriStates)
	}

	gotDeviations := map[string]int{
		"DeviationUnset":        int(compat.DeviationUnset),
		"DeviationNotSupported": int(compat.DeviationNotSupported),
		"DeviationAdd":          int(compat.DeviationAdd),
		"DeviationReplace":      int(compat.DeviationReplace),
		"DeviationDelete":       int(compat.DeviationDelete),
	}
	wantDeviations := map[string]int{
		"DeviationUnset":        int(upstream.DeviationUnset),
		"DeviationNotSupported": int(upstream.DeviationNotSupported),
		"DeviationAdd":          int(upstream.DeviationAdd),
		"DeviationReplace":      int(upstream.DeviationReplace),
		"DeviationDelete":       int(upstream.DeviationDelete),
	}
	if !reflect.DeepEqual(gotDeviations, wantDeviations) {
		t.Fatalf("deviation constants = %v, want goyang %v", gotDeviations, wantDeviations)
	}

	gotNumericLimits := map[string]any{
		"MaxInt64":          compat.MaxInt64,
		"MinInt64":          compat.MinInt64,
		"MinDecimal64":      compat.MinDecimal64,
		"MaxDecimal64":      compat.MaxDecimal64,
		"AbsMinInt64":       uint64(compat.AbsMinInt64),
		"MaxFractionDigits": compat.MaxFractionDigits,
		"MaxEnum":           compat.MaxEnum,
		"MinEnum":           compat.MinEnum,
		"MaxBitfieldSize":   compat.MaxBitfieldSize,
	}
	wantNumericLimits := map[string]any{
		"MaxInt64":          upstream.MaxInt64,
		"MinInt64":          upstream.MinInt64,
		"MinDecimal64":      upstream.MinDecimal64,
		"MaxDecimal64":      upstream.MaxDecimal64,
		"AbsMinInt64":       uint64(upstream.AbsMinInt64),
		"MaxFractionDigits": upstream.MaxFractionDigits,
		"MaxEnum":           upstream.MaxEnum,
		"MinEnum":           upstream.MinEnum,
		"MaxBitfieldSize":   upstream.MaxBitfieldSize,
	}
	if !reflect.DeepEqual(gotNumericLimits, wantNumericLimits) {
		t.Fatalf("numeric constants = %v, want goyang %v", gotNumericLimits, wantNumericLimits)
	}

	gotRanges := map[string]string{
		"Int8Range":   compat.Int8Range.String(),
		"Int16Range":  compat.Int16Range.String(),
		"Int32Range":  compat.Int32Range.String(),
		"Int64Range":  compat.Int64Range.String(),
		"Uint8Range":  compat.Uint8Range.String(),
		"Uint16Range": compat.Uint16Range.String(),
		"Uint32Range": compat.Uint32Range.String(),
		"Uint64Range": compat.Uint64Range.String(),
	}
	wantRanges := map[string]string{
		"Int8Range":   upstream.Int8Range.String(),
		"Int16Range":  upstream.Int16Range.String(),
		"Int32Range":  upstream.Int32Range.String(),
		"Int64Range":  upstream.Int64Range.String(),
		"Uint8Range":  upstream.Uint8Range.String(),
		"Uint16Range": upstream.Uint16Range.String(),
		"Uint32Range": upstream.Uint32Range.String(),
		"Uint64Range": upstream.Uint64Range.String(),
	}
	if !reflect.DeepEqual(gotRanges, wantRanges) {
		t.Fatalf("built-in ranges = %v, want goyang %v", gotRanges, wantRanges)
	}
}

func TestCompatNumberAndRangeHelpers(t *testing.T) {
	neg := compat.FromInt(-42)
	if got := neg.String(); got != "-42" {
		t.Fatalf("FromInt(-42).String() = %q, want -42", got)
	}
	if got := neg.Trunc(); got != 42 {
		t.Fatalf("FromInt(-42).Trunc() = %d, want 42", got)
	}
	if got, err := neg.Int(); err != nil || got != -42 {
		t.Fatalf("FromInt(-42).Int() = (%d,%v), want -42,nil", got, err)
	}
	min := compat.FromInt(compat.MinInt64)
	if got := min.String(); got != "-9223372036854775808" {
		t.Fatalf("FromInt(MinInt64).String() = %q, want -9223372036854775808", got)
	}
	if got, err := min.Int(); err != nil || got != compat.MinInt64 {
		t.Fatalf("FromInt(MinInt64).Int() = (%d,%v), want MinInt64,nil", got, err)
	}
	if _, err := (compat.Number{Value: uint64(compat.MaxInt64) + 1}).Int(); err == nil {
		t.Fatal("Number.Int accepted positive overflow")
	}
	if _, err := (compat.Number{Negative: true, Value: uint64(compat.AbsMinInt64) + 1}).Int(); err == nil {
		t.Fatal("Number.Int accepted negative overflow")
	}
	if _, err := (compat.Number{Value: 10, FractionDigits: 1}).Int(); err == nil {
		t.Fatal("Number.Int accepted decimal value")
	}
	if got := compat.FromUint(10).String(); got != "10" {
		t.Fatalf("FromUint(10).String() = %q, want 10", got)
	}
	if got := compat.FromFloat(1.25).String(); got != "1.25" {
		t.Fatalf("FromFloat(1.25).String() = %q, want 1.25", got)
	}
	if got := compat.Frac(3.25); got != 0.25 {
		t.Fatalf("Frac(3.25) = %v, want 0.25", got)
	}

	parsed, err := compat.ParseInt("+0x10")
	if err != nil {
		t.Fatalf("ParseInt: %v", err)
	}
	if got, err := parsed.Int(); err != nil || got != 16 {
		t.Fatalf("ParseInt(+0x10).Int() = (%d,%v), want 16,nil", got, err)
	}
	decimal, err := compat.ParseDecimal("1.20", 2)
	if err != nil {
		t.Fatalf("ParseDecimal: %v", err)
	}
	if got := decimal.String(); got != "1.20" {
		t.Fatalf("ParseDecimal(1.20).String() = %q, want 1.20", got)
	}
	if got := decimal.Trunc(); got != 1 {
		t.Fatalf("ParseDecimal(1.20).Trunc() = %d, want 1", got)
	}
	if _, err := compat.ParseDecimal("1.234", 2); err == nil {
		t.Fatal("ParseDecimal accepted excess precision")
	}

	ranges, err := compat.ParseRangesInt("1..5 | 6..10 | 20")
	if err != nil {
		t.Fatalf("ParseRangesInt: %v", err)
	}
	if got := ranges.String(); got != "1..10|20" {
		t.Fatalf("ParseRangesInt String = %q, want 1..10|20", got)
	}
	if err := ranges.Validate(); err != nil {
		t.Fatalf("ranges Validate: %v", err)
	}
	subset, err := compat.ParseRangesInt("3..5")
	if err != nil {
		t.Fatalf("ParseRangesInt subset: %v", err)
	}
	if !ranges.Contains(subset) {
		t.Fatalf("%s Contains(%s) = false, want true", ranges, subset)
	}
	decimalRange, err := compat.ParseRangesDecimal("1.20..3.40", 2)
	if err != nil {
		t.Fatalf("ParseRangesDecimal: %v", err)
	}
	if got := decimalRange.String(); got != "1.20..3.40" {
		t.Fatalf("ParseRangesDecimal String = %q, want 1.20..3.40", got)
	}
	if _, err := compat.ParseRangesInt("10..1"); err == nil {
		t.Fatal("ParseRangesInt accepted inverted range")
	}
	if !compat.Int8Range.Contains(compat.YangRange{{Min: compat.FromInt(-1), Max: compat.FromInt(1)}}) {
		t.Fatal("Int8Range does not contain -1..1")
	}
	if !compat.Uint8Range.Contains(compat.YangRange{{Min: compat.FromUint(0), Max: compat.FromUint(255)}}) {
		t.Fatal("Uint8Range does not contain 0..255")
	}
	if compat.MaxFractionDigits != 18 || compat.MaxEnum != 1<<31-1 || compat.MinEnum != -1<<31 {
		t.Fatal("numeric exported constants do not match goyang values")
	}
}

func TestCompatEnumTypeHelpersMatchGoyang(t *testing.T) {
	compatEnum := compat.NewEnumType()
	upstreamEnum := upstream.NewEnumType()
	assertEnumSetMatchesGoyang(t, compatEnum, upstreamEnum, "first", 1)
	assertEnumSetMatchesGoyang(t, compatEnum, upstreamEnum, "second", 3)
	assertEnumSetNextMatchesGoyang(t, compatEnum, upstreamEnum, "third")
	assertEnumSetMatchesGoyang(t, compatEnum, upstreamEnum, "first", 7)
	assertEnumSetMatchesGoyang(t, compatEnum, upstreamEnum, "collide", 1)
	assertEnumSetMatchesGoyang(t, compatEnum, upstreamEnum, "below-min", int64(compat.MinEnum)-1)
	assertEnumSetMatchesGoyang(t, compatEnum, upstreamEnum, "above-max", int64(compat.MaxEnum)+1)
	assertEnumStateMatchesGoyang(t, compatEnum, upstreamEnum)
	assertEnumMapsAreDefensiveCopies(t, compatEnum, "first", 1)

	compatBits := compat.NewBitfield()
	upstreamBits := upstream.NewBitfield()
	assertEnumSetMatchesGoyang(t, compatBits, upstreamBits, "read", 0)
	assertEnumSetMatchesGoyang(t, compatBits, upstreamBits, "also-read", 0)
	assertEnumSetNextMatchesGoyang(t, compatBits, upstreamBits, "write")
	assertEnumSetMatchesGoyang(t, compatBits, upstreamBits, "negative", -1)
	assertEnumSetMatchesGoyang(t, compatBits, upstreamBits, "too-large", int64(compat.MaxBitfieldSize))
	assertEnumStateMatchesGoyang(t, compatBits, upstreamBits)
	assertEnumMapsAreDefensiveCopies(t, compatBits, "read", 0)
}

func assertEnumSetMatchesGoyang(t *testing.T, got *compat.EnumType, want *upstream.EnumType, name string, value int64) {
	t.Helper()
	gotErr := got.Set(name, value)
	wantErr := want.Set(name, value)
	assertErrorStringMatches(t, "Set("+name+")", gotErr, wantErr)
}

func assertEnumSetNextMatchesGoyang(t *testing.T, got *compat.EnumType, want *upstream.EnumType, name string) {
	t.Helper()
	gotErr := got.SetNext(name)
	wantErr := want.SetNext(name)
	assertErrorStringMatches(t, "SetNext("+name+")", gotErr, wantErr)
}

func assertEnumStateMatchesGoyang(t *testing.T, got *compat.EnumType, want *upstream.EnumType) {
	t.Helper()
	if !reflect.DeepEqual(got.Names(), want.Names()) {
		t.Fatalf("Names() = %v, want goyang %v", got.Names(), want.Names())
	}
	if !reflect.DeepEqual(got.Values(), want.Values()) {
		t.Fatalf("Values() = %v, want goyang %v", got.Values(), want.Values())
	}
	if !reflect.DeepEqual(got.NameMap(), want.NameMap()) {
		t.Fatalf("NameMap() = %v, want goyang %v", got.NameMap(), want.NameMap())
	}
	if !reflect.DeepEqual(got.ValueMap(), want.ValueMap()) {
		t.Fatalf("ValueMap() = %v, want goyang %v", got.ValueMap(), want.ValueMap())
	}
	for _, name := range got.Names() {
		if got.IsDefined(name) != want.IsDefined(name) {
			t.Fatalf("IsDefined(%q) = %v, want goyang %v", name, got.IsDefined(name), want.IsDefined(name))
		}
		if got.Value(name) != want.Value(name) {
			t.Fatalf("Value(%q) = %d, want goyang %d", name, got.Value(name), want.Value(name))
		}
	}
	for _, value := range got.Values() {
		if got.Name(value) != want.Name(value) {
			t.Fatalf("Name(%d) = %q, want goyang %q", value, got.Name(value), want.Name(value))
		}
	}
}

func assertEnumMapsAreDefensiveCopies(t *testing.T, enum *compat.EnumType, name string, value int64) {
	t.Helper()
	names := enum.NameMap()
	values := enum.ValueMap()
	names[name] = value + 99
	values[value] = "mutated"
	if got := enum.Value(name); got != value {
		t.Fatalf("NameMap mutation changed Value(%q) to %d, want %d", name, got, value)
	}
	if got := enum.Name(value); got == "mutated" {
		t.Fatalf("ValueMap mutation changed Name(%d)", value)
	}
}

func assertErrorStringMatches(t *testing.T, label string, got, want error) {
	t.Helper()
	gotText, wantText := "", ""
	if got != nil {
		gotText = got.Error()
	}
	if want != nil {
		wantText = want.Error()
	}
	if gotText != wantText {
		t.Fatalf("%s error = %q, want goyang %q", label, gotText, wantText)
	}
}

func TestCompatYangRangeValidateAdjacentMatchesGoyang(t *testing.T) {
	adjacent := compat.YangRange{
		{Min: compat.FromInt(1), Max: compat.FromInt(1)},
		{Min: compat.FromInt(2), Max: compat.FromInt(2)},
	}
	if err := adjacent.Validate(); err != nil {
		t.Fatalf("Validate adjacent ranges = %v, want nil like goyang", err)
	}
}

func TestCompatYangRangeValidateRejectsInvalidRangesAsSafetyNet(t *testing.T) {
	tests := []struct {
		name   string
		ranges compat.YangRange
		want   string
	}{
		{
			name: "inclusive endpoint overlap",
			ranges: compat.YangRange{
				{Min: compat.FromInt(1), Max: compat.FromInt(2)},
				{Min: compat.FromInt(2), Max: compat.FromInt(3)},
			},
			want: "overlapping ranges",
		},
		{
			name: "later invalid range",
			ranges: compat.YangRange{
				{Min: compat.FromInt(1), Max: compat.FromInt(2)},
				{Min: compat.FromInt(5), Max: compat.FromInt(4)},
			},
			want: "invalid number",
		},
		{
			name: "non-first overlap",
			ranges: compat.YangRange{
				{Min: compat.FromInt(1), Max: compat.FromInt(2)},
				{Min: compat.FromInt(3), Max: compat.FromInt(5)},
				{Min: compat.FromInt(4), Max: compat.FromInt(6)},
			},
			want: "overlapping ranges",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.ranges.Validate(); err == nil || err.Error() != tt.want {
				t.Fatalf("Validate(%s) = %v, want %q", tt.ranges, err, tt.want)
			}
		})
	}
}

func TestCompatBuiltinTypedefsAndYangTypeShape(t *testing.T) {
	gotKeys := map[string]bool{}
	for name := range compat.BaseTypedefs {
		gotKeys[name] = true
	}
	wantKeys := map[string]bool{}
	for name := range upstream.BaseTypedefs {
		wantKeys[name] = true
	}
	if !reflect.DeepEqual(gotKeys, wantKeys) {
		t.Fatalf("BaseTypedefs keys = %v, want goyang %v", gotKeys, wantKeys)
	}

	td := compat.BaseTypedefs["string"]
	if td == nil {
		t.Fatal("BaseTypedefs[string] = nil")
	}
	if td.Name != "string" || td.Type == nil || td.Type.Name != "string" {
		t.Fatalf("BaseTypedefs[string] = %#v, want typedef/type named string", td)
	}
	for name, want := range upstream.BaseTypedefs {
		got := compat.BaseTypedefs[name]
		if got == nil {
			t.Fatalf("BaseTypedefs[%s] = nil, want goyang typedef", name)
		}
		if got.Name != want.Name || got.Type == nil || want.Type == nil || got.Type.Name != want.Type.Name {
			t.Fatalf("BaseTypedefs[%s] = %#v, want goyang %#v", name, got, want)
		}
		if got.YangType == nil || want.YangType == nil {
			t.Fatalf("BaseTypedefs[%s].YangType = (%#v,%#v), want both non-nil", name, got.YangType, want.YangType)
		}
		if got.YangType.Name != want.YangType.Name ||
			got.YangType.Kind != compat.TypeKind(want.YangType.Kind) ||
			got.YangType.Range.String() != want.YangType.Range.String() {
			t.Fatalf("BaseTypedefs[%s].YangType = %#v, want goyang %#v", name, got.YangType, want.YangType)
		}
		if got.YangType.Root != got.YangType {
			t.Fatalf("BaseTypedefs[%s].YangType.Root = %#v, want self", name, got.YangType.Root)
		}
	}

	typ := &compat.YangType{Name: "local-string", Kind: compat.Ystring}
	typ.Base = &compat.Type{Name: "string"}
	typ.Root = &compat.YangType{Name: "string", Kind: compat.Ystring}
	if typ.Base == nil || typ.Base.Name != "string" {
		t.Fatalf("YangType.Base = %#v, want string", typ.Base)
	}
	if typ.Root == nil || typ.Root.Name != "string" || typ.Root.Kind != compat.Ystring {
		t.Fatalf("YangType.Root = %#v, want string root", typ.Root)
	}
	if !typ.Equal(&compat.YangType{Name: "local-string", Kind: compat.Ystring}) {
		t.Fatal("YangType.Equal should ignore Base and Root, matching goyang")
	}
}

func TestCompatTypedefProjectionSetsBaseAndRoot(t *testing.T) {
	source := `module compat-typedef-demo {
    namespace "urn:compat-typedef-demo";
    prefix ctd;

    typedef local-string {
        type string;
    }

    leaf value {
        type local-string;
    }
}
`
	builder, err := cambium.NewContextBuilder(cambium.ContextFlags{})
	if err != nil {
		t.Fatalf("NewContextBuilder: %v", err)
	}
	if err := builder.LoadModuleStr(source); err != nil {
		t.Fatalf("LoadModuleStr: %v", err)
	}
	ctx, err := builder.Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	defer ctx.Close()

	mod, err := ctx.Schema("compat-typedef-demo")
	if err != nil {
		t.Fatalf("Schema: %v", err)
	}
	entry := compat.FromModule(mod).Lookup("value")
	if entry == nil || entry.Type == nil {
		t.Fatalf("value entry/type = %#v", entry)
	}
	if entry.Type.Name != "local-string" || entry.Type.Kind != compat.Ystring {
		t.Fatalf("value type = %#v, want local-string string", entry.Type)
	}
	if entry.Type.Base == nil || entry.Type.Base.Name != "string" {
		t.Fatalf("value type Base = %#v, want string", entry.Type.Base)
	}
	if entry.Type.Root == nil || entry.Type.Root.Name != "string" || entry.Type.Root.Kind != compat.Ystring {
		t.Fatalf("value type Root = %#v, want string root", entry.Type.Root)
	}
}

func TestCompatEntryAugmentDeviationUsesShape(t *testing.T) {
	entry := &compat.Entry{Name: "root"}
	entry.Augments = []*compat.Entry{{Name: "local-augment"}}
	entry.Augmented = []*compat.Entry{{Name: "merged-augment"}}
	entry.Deviations = []*compat.DeviatedEntry{{
		Type:         compat.DeviationAdd,
		DeviatedPath: "/root/value",
		Entry:        &compat.Entry{Name: "deviated-value"},
	}}
	entry.Uses = []*compat.UsesStmt{{
		Uses:     &compat.Uses{Name: "common"},
		Grouping: &compat.Entry{Name: "common-grouping"},
	}}

	if got := entry.Augments[0].Name; got != "local-augment" {
		t.Fatalf("Entry.Augments[0].Name = %q, want local-augment", got)
	}
	if got := entry.Augmented[0].Name; got != "merged-augment" {
		t.Fatalf("Entry.Augmented[0].Name = %q, want merged-augment", got)
	}
	if got := entry.Deviations[0].Type; got != compat.DeviationAdd {
		t.Fatalf("Deviation type = %v, want add", got)
	}
	if got := entry.Deviations[0].Type.String(); got != "add" {
		t.Fatalf("DeviationAdd.String() = %q, want add", got)
	}
	if got := compat.DeviationNotSupported.String(); got != "not-supported" {
		t.Fatalf("DeviationNotSupported.String() = %q, want not-supported", got)
	}
	if got := entry.Deviations[0].DeviatedPath; got != "/root/value" {
		t.Fatalf("DeviatedPath = %q, want /root/value", got)
	}
	if got := entry.Deviations[0].Name; got != "deviated-value" {
		t.Fatalf("Deviated entry name = %q, want deviated-value", got)
	}
	if got := entry.Uses[0].Uses.Name; got != "common" {
		t.Fatalf("UsesStmt Uses.Name = %q, want common", got)
	}
	if got := entry.Uses[0].Grouping.Name; got != "common-grouping" {
		t.Fatalf("UsesStmt Grouping.Name = %q, want common-grouping", got)
	}
	if len(entry.Deviate) != 0 {
		t.Fatalf("Entry.Deviate len = %d, want 0", len(entry.Deviate))
	}
}

func TestCompatIdentityImplementsNodeShape(t *testing.T) {
	parent := &compat.Value{Name: "parent"}
	identity := &compat.Identity{
		Name:   "transport",
		Source: &compat.Statement{Keyword: "identity", Argument: "transport", HasArgument: true},
		Parent: parent,
		Extensions: []*compat.Statement{
			{Keyword: "ext:marker", Argument: "ok", HasArgument: true},
		},
	}
	var node compat.Node = identity
	if got := node.Kind(); got != "identity" {
		t.Fatalf("Identity Kind = %q, want identity", got)
	}
	if got := node.NName(); got != "transport" {
		t.Fatalf("Identity NName = %q, want transport", got)
	}
	if got := node.ParentNode(); got != parent {
		t.Fatalf("Identity ParentNode = %#v, want parent", got)
	}
	if got := node.Statement(); got != identity.Source {
		t.Fatalf("Identity Statement = %#v, want source", got)
	}
	if got := node.Exts(); len(got) != 1 || got[0].Argument != "ok" {
		t.Fatalf("Identity Exts = %#v, want marker", got)
	}
}
