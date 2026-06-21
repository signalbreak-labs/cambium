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
