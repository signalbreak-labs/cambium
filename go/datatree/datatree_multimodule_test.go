// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

package datatree_test

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/signalbreak-labs/cambium/go/cambium"
	"github.com/signalbreak-labs/cambium/go/datatree"
)

// Documents whose top-level nodes come from several modules. Every expected
// output and verdict below is what libyang v5.4.9 gives for the same modules
// and data (lyd_parse_data, lyd_print_mem, lyd_new_implicit_all and
// lyd_validate_all).

// loadModules builds one context from srcs and returns its implemented
// modules, looked up by name in the order names lists them.
func loadModules(t *testing.T, srcs []string, names ...string) []cambium.Module {
	t.Helper()
	b, err := cambium.NewContextBuilder(cambium.ContextFlags{})
	if err != nil {
		t.Fatal(err)
	}
	for _, src := range srcs {
		if err := b.LoadModuleStr(src); err != nil {
			t.Fatalf("LoadModuleStr: %v", err)
		}
	}
	ctx, err := b.Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	t.Cleanup(func() { ctx.Close() })
	mods := make([]cambium.Module, 0, len(names))
	for _, name := range names {
		mod, err := ctx.Schema(name)
		if err != nil {
			t.Fatalf("Schema(%s): %v", name, err)
		}
		mods = append(mods, mod)
	}
	return mods
}

func parseModules(t *testing.T, mods []cambium.Module, f datatree.Format, in string) *datatree.Tree {
	t.Helper()
	tree, err := datatree.ParseModules(mods, f, []byte(in))
	if err != nil {
		t.Fatalf("ParseModules(%s): %v", in, err)
	}
	return tree
}

func serializeTree(t *testing.T, tree *datatree.Tree) (jsonOut, xmlOut string) {
	t.Helper()
	j, err := tree.Serialize(datatree.FormatJSONIETF)
	if err != nil {
		t.Fatalf("Serialize JSON: %v", err)
	}
	x, err := tree.Serialize(datatree.FormatXML)
	if err != nil {
		t.Fatalf("Serialize XML: %v", err)
	}
	return string(j), string(x)
}

// Module names, namespaces, prefixes and load order all disagree, so only
// libyang's rule can produce the expected order: top-level siblings are
// grouped by module, modules in bytewise name order ("Bm" < "aa-last" <
// "mm" < "mm-x" < "mm.x" < "mm_x" < "zz-first"), and each module's nodes in
// its schema order.
var topOrderModules = []string{
	`module zz-first { namespace "urn:aaa"; prefix aa;
  leaf z1 { type string; }
  container zc { leaf x { type string; } }
  leaf z2 { type string; default zd; }
}`,
	`module aa-last { namespace "urn:zzz"; prefix zz;
  leaf a1 { type string; }
  leaf a2 { type string; }
}`,
	`module mm { namespace "urn:mmm"; prefix b;
  leaf-list m1 { type int32; }
  list ml { key k; leaf k { type string; } }
}`,
	`module Bm { namespace "urn:bbb"; prefix bm;
  leaf bdef { type string; default bdv; }
  leaf b1 { type string; }
}`,
	`module mm-x { namespace "urn:mmx"; prefix mx; leaf x1 { type string; } }`,
	`module mm_x { namespace "urn:mmu"; prefix mu; leaf u1 { type string; } }`,
	`module mm.x { namespace "urn:mmd"; prefix md; leaf d1 { type string; } }`,
}

var topOrderNames = []string{"zz-first", "aa-last", "mm", "Bm", "mm-x", "mm_x", "mm.x"}

const (
	topOrderJSONIn = `{"aa-last:a2":"A2","zz-first:z2":"Z2","mm:m1":[3,1],"mm-x:x1":"X","mm_x:u1":"U","mm.x:d1":"D",` +
		`"Bm:b1":"B","mm:ml":[{"k":"q"}],"zz-first:zc":{"x":"c"},"aa-last:a1":"A1","zz-first:z1":"Z1"}`
	topOrderXMLIn = `<a2 xmlns="urn:zzz">A2</a2>
<z2 xmlns="urn:aaa">Z2</z2>
<m1 xmlns="urn:mmm">3</m1>
<x1 xmlns="urn:mmx">X</x1>
<u1 xmlns="urn:mmu">U</u1>
<d1 xmlns="urn:mmd">D</d1>
<b1 xmlns="urn:bbb">B</b1>
<ml xmlns="urn:mmm"><k>q</k></ml>
<zc xmlns="urn:aaa"><x>c</x></zc>
<m1 xmlns="urn:mmm">1</m1>
<a1 xmlns="urn:zzz">A1</a1>
<z1 xmlns="urn:aaa">Z1</z1>
`
	topOrderJSONWant = `{"Bm:b1":"B","aa-last:a1":"A1","aa-last:a2":"A2","mm:m1":[1,3],"mm:ml":[{"k":"q"}],` +
		`"mm-x:x1":"X","mm.x:d1":"D","mm_x:u1":"U","zz-first:z1":"Z1","zz-first:zc":{"x":"c"},"zz-first:z2":"Z2"}`
	topOrderXMLWant = `<b1 xmlns="urn:bbb">B</b1><a1 xmlns="urn:zzz">A1</a1><a2 xmlns="urn:zzz">A2</a2>` +
		`<m1 xmlns="urn:mmm">1</m1><m1 xmlns="urn:mmm">3</m1><ml xmlns="urn:mmm"><k>q</k></ml>` +
		`<x1 xmlns="urn:mmx">X</x1><d1 xmlns="urn:mmd">D</d1><u1 xmlns="urn:mmu">U</u1>` +
		`<z1 xmlns="urn:aaa">Z1</z1><zc xmlns="urn:aaa"><x>c</x></zc><z2 xmlns="urn:aaa">Z2</z2>`
)

func TestParseModulesTopLevelOrderMatchesLibyang(t *testing.T) {
	mods := loadModules(t, topOrderModules, topOrderNames...)
	reversed := slices.Clone(mods)
	slices.Reverse(reversed)
	for _, order := range [][]cambium.Module{mods, reversed} {
		for _, in := range []struct {
			name string
			f    datatree.Format
			data string
		}{
			{"json", datatree.FormatJSONIETF, topOrderJSONIn},
			{"xml", datatree.FormatXML, topOrderXMLIn},
		} {
			t.Run(in.name, func(t *testing.T) {
				tree := parseModules(t, order, in.f, in.data)
				gotJSON, gotXML := serializeTree(t, tree)
				if gotJSON != topOrderJSONWant {
					t.Errorf("JSON order:\n got %s\nwant %s", gotJSON, topOrderJSONWant)
				}
				if gotXML != topOrderXMLWant {
					t.Errorf("XML order:\n got %s\nwant %s", gotXML, topOrderXMLWant)
				}
				var roots []string
				for _, n := range tree.RootNodes() {
					roots = append(roots, n.Module()+":"+n.Name())
				}
				want := []string{"Bm:b1", "aa-last:a1", "aa-last:a2", "mm:m1", "mm:ml", "mm-x:x1", "mm.x:d1", "mm_x:u1", "zz-first:z1", "zz-first:zc", "zz-first:z2"}
				if !slices.Equal(roots, want) {
					t.Errorf("RootNodes = %v, want %v", roots, want)
				}
			})
		}
	}
}

// Top-level nodes of different modules that share a local name stay distinct:
// they are matched by module name (JSON_IETF) or namespace (XML).
func TestParseModulesSameLocalNameInTwoModules(t *testing.T) {
	mods := loadModules(t, []string{
		`module dup-b { namespace "urn:dup-b"; prefix b; leaf x { type string; } container c { leaf v { type int8; } } }`,
		`module dup-a { namespace "urn:dup-a"; prefix a; leaf x { type int8; } container c { leaf v { type string; } } }`,
	}, "dup-b", "dup-a")
	const (
		jsonWant = `{"dup-a:x":1,"dup-a:c":{"v":"s"},"dup-b:x":"one","dup-b:c":{"v":2}}`
		xmlWant  = `<x xmlns="urn:dup-a">1</x><c xmlns="urn:dup-a"><v>s</v></c><x xmlns="urn:dup-b">one</x><c xmlns="urn:dup-b"><v>2</v></c>`
	)
	for _, in := range []struct {
		name string
		f    datatree.Format
		data string
	}{
		{"json", datatree.FormatJSONIETF, `{"dup-b:c":{"v":2},"dup-a:c":{"v":"s"},"dup-b:x":"one","dup-a:x":1}`},
		{"xml", datatree.FormatXML, `<c xmlns="urn:dup-b"><v>2</v></c><x xmlns="urn:dup-b">one</x><c xmlns="urn:dup-a"><v>s</v></c><x xmlns="urn:dup-a">1</x>`},
	} {
		t.Run(in.name, func(t *testing.T) {
			tree := parseModules(t, mods, in.f, in.data)
			gotJSON, gotXML := serializeTree(t, tree)
			if gotJSON != jsonWant {
				t.Errorf("JSON:\n got %s\nwant %s", gotJSON, jsonWant)
			}
			if gotXML != xmlWant {
				t.Errorf("XML:\n got %s\nwant %s", gotXML, xmlWant)
			}
			if err := tree.Validate(); err != nil {
				t.Errorf("Validate: %v", err)
			}
		})
	}
}

// mmb holds the targets; mma refers to them from its own top-level nodes
// (leafref, must, when) and augments one of them; mmz adds a mandatory
// top-level leaf that no document below carries.
var crossModuleSrcs = []string{
	`module mmb {
    yang-version 1.1; namespace "urn:mmb"; prefix mmb;
    list interface { key name; leaf name { type string; } leaf mtu { type uint16; } }
    leaf mode { type string; }
    container sys { leaf hostname { type string; default "base-host"; } }
}`,
	`module mma {
    yang-version 1.1; namespace "urn:mma"; prefix mma;
    import mmb { prefix b; }
    leaf bound-if { type leafref { path "/b:interface/b:name"; } }
    leaf-list watched { type leafref { path "/b:interface/b:name"; } }
    leaf checked { type string; must "/b:mode = 'strict'"; }
    leaf gated { type string; when "/b:mode = 'on'"; }
    leaf gated-default { type string; default "gd"; when "/b:mode = 'on'"; }
    leaf plain-default { type string; default "pd"; }
    augment "/b:sys" { leaf domain { type string; default "example.net"; } }
}`,
	`module mmz {
    yang-version 1.1; namespace "urn:mmz"; prefix mmz;
    leaf required { type string; mandatory true; }
}`,
}

func TestParseModulesCrossModuleRoundTrip(t *testing.T) {
	mods := loadModules(t, crossModuleSrcs, "mmb", "mma")
	const (
		jsonIn   = `{"mmb:interface":[{"mtu":1500,"name":"eth0"}],"mma:bound-if":"eth0","mmb:sys":{"mma:domain":"example.org"}}`
		xmlIn    = `<interface xmlns="urn:mmb"><mtu>1500</mtu><name>eth0</name></interface><bound-if xmlns="urn:mma">eth0</bound-if><sys xmlns="urn:mmb"><domain xmlns="urn:mma">example.org</domain></sys>`
		jsonWant = `{"mma:bound-if":"eth0","mmb:interface":[{"name":"eth0","mtu":1500}],"mmb:sys":{"mma:domain":"example.org"}}`
		xmlWant  = `<bound-if xmlns="urn:mma">eth0</bound-if><interface xmlns="urn:mmb"><name>eth0</name><mtu>1500</mtu></interface><sys xmlns="urn:mmb"><domain xmlns="urn:mma">example.org</domain></sys>`
	)
	for _, in := range []struct {
		name string
		f    datatree.Format
		data string
	}{
		{"json", datatree.FormatJSONIETF, jsonIn},
		{"xml", datatree.FormatXML, xmlIn},
		{"json-reparse", datatree.FormatJSONIETF, jsonWant},
		{"xml-reparse", datatree.FormatXML, xmlWant},
	} {
		t.Run(in.name, func(t *testing.T) {
			tree := parseModules(t, mods, in.f, in.data)
			gotJSON, gotXML := serializeTree(t, tree)
			if gotJSON != jsonWant {
				t.Errorf("JSON:\n got %s\nwant %s", gotJSON, jsonWant)
			}
			if gotXML != xmlWant {
				t.Errorf("XML:\n got %s\nwant %s", gotXML, xmlWant)
			}
			if err := tree.Validate(); err != nil {
				t.Errorf("Validate: %v", err)
			}
		})
	}
}

func TestParseModulesCrossModuleValidation(t *testing.T) {
	two := loadModules(t, crossModuleSrcs, "mmb", "mma")
	three := loadModules(t, crossModuleSrcs, "mmb", "mma", "mmz")
	cases := []struct {
		name string
		mods []cambium.Module
		json string
		xml  string
		want []string // violation substrings; nil means valid
	}{
		{
			name: "leafref target in other module",
			mods: two,
			json: `{"mma:bound-if":"eth0","mmb:interface":[{"name":"eth0"}]}`,
			xml:  `<bound-if xmlns="urn:mma">eth0</bound-if><interface xmlns="urn:mmb"><name>eth0</name></interface>`,
		},
		{
			name: "leafref target missing in other module",
			mods: two,
			json: `{"mma:bound-if":"eth9","mmb:interface":[{"name":"eth0"}]}`,
			xml:  `<bound-if xmlns="urn:mma">eth9</bound-if><interface xmlns="urn:mmb"><name>eth0</name></interface>`,
			want: []string{`/bound-if: leafref value "eth9" has no matching instance`},
		},
		{
			name: "leaf-list leafref targets in other module",
			mods: two,
			json: `{"mma:watched":["eth0","eth1"],"mmb:interface":[{"name":"eth0"},{"name":"eth1"}]}`,
			xml:  `<watched xmlns="urn:mma">eth0</watched><watched xmlns="urn:mma">eth1</watched><interface xmlns="urn:mmb"><name>eth0</name></interface><interface xmlns="urn:mmb"><name>eth1</name></interface>`,
		},
		{
			name: "leaf-list leafref target missing in other module",
			mods: two,
			json: `{"mma:watched":["eth0","eth7"],"mmb:interface":[{"name":"eth0"}]}`,
			xml:  `<watched xmlns="urn:mma">eth0</watched><watched xmlns="urn:mma">eth7</watched><interface xmlns="urn:mmb"><name>eth0</name></interface>`,
			want: []string{`/watched[1]: leafref value "eth7" has no matching instance`},
		},
		{
			name: "must over other module holds",
			mods: two,
			json: `{"mma:checked":"x","mmb:mode":"strict"}`,
			xml:  `<checked xmlns="urn:mma">x</checked><mode xmlns="urn:mmb">strict</mode>`,
		},
		{
			name: "must over other module fails",
			mods: two,
			json: `{"mma:checked":"x","mmb:mode":"loose"}`,
			xml:  `<checked xmlns="urn:mma">x</checked><mode xmlns="urn:mmb">loose</mode>`,
			want: []string{`/checked: must condition "/b:mode = 'strict'" is not satisfied`},
		},
		{
			name: "must over absent node of other module fails",
			mods: two,
			json: `{"mma:checked":"x"}`,
			xml:  `<checked xmlns="urn:mma">x</checked>`,
			want: []string{`/checked: must condition "/b:mode = 'strict'" is not satisfied`},
		},
		{
			name: "when over other module holds",
			mods: two,
			json: `{"mma:gated":"x","mmb:mode":"on"}`,
			xml:  `<gated xmlns="urn:mma">x</gated><mode xmlns="urn:mmb">on</mode>`,
		},
		{
			name: "when over other module fails",
			mods: two,
			json: `{"mma:gated":"x","mmb:mode":"off"}`,
			xml:  `<gated xmlns="urn:mma">x</gated><mode xmlns="urn:mmb">off</mode>`,
			want: []string{`/gated: when condition "/b:mode = 'on'" is not satisfied`},
		},
		{
			name: "mandatory node of a bound module without data",
			mods: three,
			json: `{"mma:bound-if":"eth0","mmb:interface":[{"name":"eth0"}]}`,
			xml:  `<bound-if xmlns="urn:mma">eth0</bound-if><interface xmlns="urn:mmb"><name>eth0</name></interface>`,
			want: []string{"missing mandatory node /required"},
		},
	}
	for _, tc := range cases {
		for _, in := range []struct {
			name string
			f    datatree.Format
			data string
		}{
			{"json", datatree.FormatJSONIETF, tc.json},
			{"xml", datatree.FormatXML, tc.xml},
		} {
			t.Run(tc.name+"/"+in.name, func(t *testing.T) {
				err := parseModules(t, tc.mods, in.f, in.data).Validate()
				if len(tc.want) == 0 {
					if err != nil {
						t.Fatalf("expected valid data, got %v", err)
					}
					return
				}
				if err == nil {
					t.Fatalf("expected violations %q, got nil", tc.want)
				}
				for _, w := range tc.want {
					if !strings.Contains(err.Error(), w) {
						t.Errorf("violation %q not found in: %v", w, err)
					}
				}
				var ve *datatree.ValidationError
				if !errors.As(err, &ve) || len(ve.Violations) != len(tc.want) {
					t.Errorf("got %v, want %d violations", err, len(tc.want))
				}
			})
		}
	}
}

// ApplyDefaults fills the defaults of every bound module, including one with
// no data in the document, in libyang's top-level order; a default whose when
// refers to another module's data follows that data.
func TestParseModulesApplyDefaultsAcrossModules(t *testing.T) {
	mods := loadModules(t, crossModuleSrcs, "mmb", "mma")
	cases := []struct {
		name     string
		json     string
		xml      string
		jsonWant string
		xmlWant  string
	}{
		{
			name:     "when true",
			json:     `{"mmb:sys":{},"mmb:mode":"on"}`,
			xml:      `<sys xmlns="urn:mmb"/><mode xmlns="urn:mmb">on</mode>`,
			jsonWant: `{"mma:gated-default":"gd","mma:plain-default":"pd","mmb:mode":"on","mmb:sys":{"hostname":"base-host","mma:domain":"example.net"}}`,
			xmlWant: `<gated-default xmlns="urn:mma">gd</gated-default><plain-default xmlns="urn:mma">pd</plain-default>` +
				`<mode xmlns="urn:mmb">on</mode><sys xmlns="urn:mmb"><hostname>base-host</hostname><domain xmlns="urn:mma">example.net</domain></sys>`,
		},
		{
			name:     "when false",
			json:     `{"mmb:mode":"off","mmb:interface":[{"name":"eth0"}]}`,
			xml:      `<mode xmlns="urn:mmb">off</mode><interface xmlns="urn:mmb"><name>eth0</name></interface>`,
			jsonWant: `{"mma:plain-default":"pd","mmb:interface":[{"name":"eth0"}],"mmb:mode":"off"}`,
			xmlWant:  `<plain-default xmlns="urn:mma">pd</plain-default><interface xmlns="urn:mmb"><name>eth0</name></interface><mode xmlns="urn:mmb">off</mode>`,
		},
	}
	for _, tc := range cases {
		for _, in := range []struct {
			name string
			f    datatree.Format
			data string
		}{
			{"json", datatree.FormatJSONIETF, tc.json},
			{"xml", datatree.FormatXML, tc.xml},
		} {
			t.Run(tc.name+"/"+in.name, func(t *testing.T) {
				tree := parseModules(t, mods, in.f, in.data)
				tree.ApplyDefaults()
				gotJSON, gotXML := serializeTree(t, tree)
				if gotJSON != tc.jsonWant {
					t.Errorf("JSON:\n got %s\nwant %s", gotJSON, tc.jsonWant)
				}
				if gotXML != tc.xmlWant {
					t.Errorf("XML:\n got %s\nwant %s", gotXML, tc.xmlWant)
				}
				tree.ApplyDefaults() // idempotent
				if again, _ := serializeTree(t, tree); again != tc.jsonWant {
					t.Errorf("second ApplyDefaults changed the tree:\n got %s\nwant %s", again, tc.jsonWant)
				}
			})
		}
	}
}

// Only the bound modules' top-level nodes are accepted, and Parse keeps
// binding exactly one module.
func TestParseModulesRejectsUnboundModules(t *testing.T) {
	mods := loadModules(t, crossModuleSrcs, "mmb", "mma")
	mmb, mma := mods[0], mods[1]
	for _, tc := range []struct {
		name string
		f    datatree.Format
		data string
		want string
	}{
		{"json", datatree.FormatJSONIETF, `{"mmb:mode":"on","mma:gated":"x"}`, `unknown member "mma:gated"`},
		{"xml", datatree.FormatXML, `<mode xmlns="urn:mmb">on</mode><gated xmlns="urn:mma">x</gated>`, `unknown XML element "gated" in namespace "urn:mma"`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := datatree.ParseModules([]cambium.Module{mmb}, tc.f, []byte(tc.data)); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("ParseModules(mmb) error = %v, want %q", err, tc.want)
			}
			if _, err := datatree.Parse(mmb, tc.f, []byte(tc.data)); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("Parse(mmb) error = %v, want %q", err, tc.want)
			}
			if _, err := datatree.ParseModules([]cambium.Module{mmb, mma}, tc.f, []byte(tc.data)); err != nil {
				t.Errorf("ParseModules(mmb, mma): %v", err)
			}
		})
	}
}

func TestParseModulesArguments(t *testing.T) {
	mods := loadModules(t, crossModuleSrcs, "mmb", "mma")
	for _, tc := range []struct {
		name string
		mods []cambium.Module
		want string
	}{
		{"none", nil, "no modules"},
		{"duplicate", []cambium.Module{mods[0], mods[1], mods[0]}, `module "mmb" given more than once`},
		{"zero module", []cambium.Module{mods[0], {}}, "invalid module"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := datatree.ParseModules(tc.mods, datatree.FormatJSONIETF, []byte(`{}`))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("ParseModules error = %v, want %q", err, tc.want)
			}
		})
	}
}
