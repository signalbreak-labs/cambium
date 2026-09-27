// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

package cambium_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/signalbreak-labs/cambium/go/cambium"
)

// Example_schemaConsumer is the integration recipe from the downstream schema
// consumer guide: explicit loading policy, completeness checks, ordered
// config-only traversal, list keys, qualified identity, defaults and resolved
// types, all from public native handles.
func Example_schemaConsumer() {
	dir, _ := os.MkdirTemp("", "cambium-example")
	defer os.RemoveAll(dir)
	_ = os.WriteFile(filepath.Join(dir, "shop.yang"), []byte(`module shop {
  yang-version 1.1; namespace "urn:example:shop"; prefix s;
  feature discounts;
  typedef price { type decimal64 { fraction-digits 2; range "0..max"; } default "0.00"; }
  container shop {
    list item {
      key "sku";
      leaf title { type string; }
      leaf sku { type string; }
      leaf cost { type price; }
      leaf discount { if-feature discounts; type uint8 { range "0..90"; } }
      leaf sold { config false; type uint32; }
    }
  }
}`), 0o644)
	_ = os.WriteFile(filepath.Join(dir, "shop-dev.yang"), []byte(`module shop-dev {
  yang-version 1.1; namespace "urn:example:shop-dev"; prefix sd;
  import shop { prefix s; }
  deviation "/s:shop/s:item/s:title" { deviate not-supported; }
}`), 0o644)

	// 1. Explicit, cwd-independent policies.
	builder, err := cambium.NewContextBuilder(cambium.ContextFlags{DisableSearchdirCwd: true})
	if err != nil {
		panic(err)
	}
	_ = builder.SearchPath(dir)
	_ = builder.SetValidationMode(cambium.ValidationStrict)
	_ = builder.SetDeviationPolicy(cambium.DeviationPolicy{IgnoreNotSupported: true})
	for _, root := range []string{"shop", "shop-dev"} {
		if err := builder.LoadModule(root, nil, nil); err != nil {
			panic(err)
		}
	}
	if err := builder.SetFeatures("shop", []string{"discounts"}); err != nil {
		panic(err)
	}
	ctx, err := builder.Build()
	if err != nil {
		diag := cambium.DiagnosticFromError(err)
		fmt.Println("build failed:", diag.Kind, diag.Source.Text)
		return
	}
	// 2. Close only after every handle read has finished.
	defer ctx.Close()

	// 3. Completeness: reject omitted content; decide separately on warnings.
	report := ctx.LoadReport()
	if len(report.OmittedContent()) != 0 {
		panic("incomplete schema")
	}
	fmt.Println("warnings:", len(report.Warnings), "ignored deviations:", len(report.IgnoredDeviations))

	// 4. Ordered config-only list-entry traversal with keys first.
	mod, _ := ctx.Schema("shop")
	item, _ := mod.FindPath("/s:shop/item")
	fmt.Println("keys:", strings.Join(item.KeyNames(), ","))
	for leaf := range item.Traverse(cambium.TraversalListEntryOrder).ConfigOnly().Iter() {
		typ, _ := leaf.LeafType()
		line := fmt.Sprintf("%s %s %v", leaf.QualifiedPath(), leaf.NamespaceQualifiedPath(), typ.Base())
		if name, ok := typ.TypedefName(); ok {
			line += " typedef=" + name
		}
		if def, ok := leaf.DefaultEntry(); ok {
			line += fmt.Sprintf(" default=%q(%v)", def.Value(), def.Origin())
		}
		if r, ok := typ.Resolved().(cambium.ResolvedInt); ok && len(r.Range) > 0 {
			line += " range=" + r.Range[0].Min() + ".." + r.Range[0].Max()
		}
		fmt.Println(line)
	}
	// Output:
	// warnings: 0 ignored deviations: 1
	// keys: sku
	// /shop:shop/shop:item/shop:sku /{urn:example:shop}shop/{urn:example:shop}item/{urn:example:shop}sku string
	// /shop:shop/shop:item/shop:title /{urn:example:shop}shop/{urn:example:shop}item/{urn:example:shop}title string
	// /shop:shop/shop:item/shop:cost /{urn:example:shop}shop/{urn:example:shop}item/{urn:example:shop}cost decimal64 typedef=price default="0.00"(typedef)
	// /shop:shop/shop:item/shop:discount /{urn:example:shop}shop/{urn:example:shop}item/{urn:example:shop}discount uint8 range=0..90
}
