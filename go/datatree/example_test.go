// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

package datatree_test

import (
	"fmt"

	"github.com/signalbreak-labs/cambium/go/cambium"
	"github.com/signalbreak-labs/cambium/go/datatree"
)

// Example parses generic instance data against a schema and reads it back, with
// no libyang and no cgo. The input members arrive out of schema order; the data
// tree normalizes them to schema declaration order (z before a) on the way out.
//
// datatree is experimental: its API and value representation will change.
func Example() {
	const src = `module dt {
  namespace "urn:dt";
  prefix dt;
  container c {
    leaf z { type string; }
    leaf a { type string; }
  }
}`
	b, err := cambium.NewContextBuilder(cambium.ContextFlags{})
	if err != nil {
		panic(err)
	}
	if err := b.LoadModuleStr(src); err != nil {
		panic(err)
	}
	ctx, err := b.Build()
	if err != nil {
		panic(err)
	}
	mod, err := ctx.Schema("dt")
	if err != nil {
		panic(err)
	}

	tree, err := datatree.Parse(mod, datatree.FormatJSONIETF, []byte(`{"dt:c":{"a":"1","z":"2"}}`))
	if err != nil {
		panic(err)
	}

	roots := tree.RootNodes()
	for _, child := range roots[0].Children() {
		fmt.Println(child.Name())
	}
	// Output:
	// z
	// a
}

// ExampleParseModules parses a document whose top-level nodes come from two
// modules, validates the leafref that crosses them, and prints the tree as
// XML. Top-level nodes of different modules come out grouped by module, in
// module-name order ("ex-base" before "ex-ref"), as libyang orders them.
func ExampleParseModules() {
	const base = `module ex-base {
  namespace "urn:ex-base";
  prefix b;
  list interface { key name; leaf name { type string; } }
}`
	const ref = `module ex-ref {
  namespace "urn:ex-ref";
  prefix r;
  import ex-base { prefix b; }
  leaf bound-if { type leafref { path "/b:interface/b:name"; } }
}`
	b, err := cambium.NewContextBuilder(cambium.ContextFlags{})
	if err != nil {
		panic(err)
	}
	for _, src := range []string{base, ref} {
		if err := b.LoadModuleStr(src); err != nil {
			panic(err)
		}
	}
	ctx, err := b.Build()
	if err != nil {
		panic(err)
	}

	// Bind every implemented module, as a libyang context does.
	tree, err := datatree.ParseModules(ctx.Modules(), datatree.FormatJSONIETF,
		[]byte(`{"ex-ref:bound-if":"eth0","ex-base:interface":[{"name":"eth0"}]}`))
	if err != nil {
		panic(err)
	}
	if err := tree.Validate(); err != nil {
		panic(err)
	}
	out, err := tree.Serialize(datatree.FormatXML)
	if err != nil {
		panic(err)
	}
	fmt.Println(string(out))
	// Output:
	// <interface xmlns="urn:ex-base"><name>eth0</name></interface><bound-if xmlns="urn:ex-ref">eth0</bound-if>
}
