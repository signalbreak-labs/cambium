package codegen_test

import (
	"fmt"
	"strings"

	"github.com/signalbreak-labs/cambium/go/cambium"
	"github.com/signalbreak-labs/cambium/go/codegen"
)

// ExampleGenerateGo generates typed Go structs for a module. The generated source
// carries a per-struct field-order manifest (keys first, then schema declaration
// order) that the emitted serializers walk, so output stays order-correct.
func ExampleGenerateGo() {
	const src = `module shapes {
  namespace "urn:shapes";
  prefix s;
  container box {
    leaf width { type uint16; }
    leaf height { type uint16; }
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

	out, err := codegen.GenerateGo(ctx, "shapes")
	if err != nil {
		panic(err)
	}

	// The generated source is deterministic, gofmt-clean Go that includes a
	// field-order manifest.
	fmt.Println(strings.Contains(out, "FieldOrder"))
	// Output: true
}
