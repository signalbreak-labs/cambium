// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

package cambium_test

import (
	"fmt"

	"github.com/signalbreak-labs/cambium/go/cambium"
)

// Example loads a module and walks a container's children. They are returned in
// effective schema declaration order — z, m, a — not the alphabetical order a
// map would yield. This is invariant I2.
func Example() {
	const src = `module order-demo {
  namespace "urn:order-demo";
  prefix od;
  container top {
    leaf z { type string; }
    leaf m { type string; }
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

	mod, err := ctx.Schema("order-demo")
	if err != nil {
		panic(err)
	}
	top, ok := mod.TopLevel().Lookup("top")
	if !ok {
		panic("container top not found")
	}
	for child := range top.Children().Iter() {
		fmt.Println(child.Name())
	}
	// Output:
	// z
	// m
	// a
}

// ExampleParseStatements parses raw YANG into Cambium's native ordered statement
// tree without using the goyang-shaped compat package.
func ExampleParseStatements() {
	const src = `module statement-demo {
  namespace "urn:statement-demo";
  prefix sd;
  container top {
    leaf z { type string; }
    leaf m { type string; }
    leaf a { type string; }
  }
}`
	stmts, err := cambium.ParseStatements(src, "statement-demo.yang")
	if err != nil {
		panic(err)
	}
	mod := stmts[0]
	fmt.Println(mod.Keyword())
	fmt.Println(mustArgument(mod))
	for _, child := range mod.SubStatements() {
		arg, _ := child.Argument()
		fmt.Println(child.Keyword(), arg)
	}
	// Output:
	// module
	// statement-demo
	// namespace urn:statement-demo
	// prefix sd
	// container top
}

func mustArgument(st cambium.Statement) string {
	arg, ok := st.Argument()
	if !ok {
		panic("missing argument")
	}
	return arg
}
