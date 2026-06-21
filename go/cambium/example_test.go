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
