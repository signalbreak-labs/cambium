// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

package datatree_test

import (
	"testing"

	"github.com/signalbreak-labs/cambium/go/cambium"
	"github.com/signalbreak-labs/cambium/go/datatree"
)

func benchmarkDTModule(b *testing.B) cambium.Module {
	b.Helper()
	builder, err := cambium.NewContextBuilder(cambium.ContextFlags{})
	if err != nil {
		b.Fatal(err)
	}
	if err := builder.LoadModuleStr(dtSchema); err != nil {
		b.Fatalf("LoadModuleStr: %v", err)
	}
	ctx, err := builder.Build()
	if err != nil {
		b.Fatalf("Build: %v", err)
	}
	b.Cleanup(func() { ctx.Close() })
	mod, err := ctx.Schema("dt")
	if err != nil {
		b.Fatalf("Schema: %v", err)
	}
	return mod
}

func BenchmarkDatatreeRoundTrip(b *testing.B) {
	mod := benchmarkDTModule(b)
	in := []byte(`{"dt:c":{"z":"hi","m":7,"a":true},"dt:tags":["t1","t2"],"dt:item":[{"id":"1","name":"x"}]}`)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tree, err := datatree.Parse(mod, datatree.FormatJSONIETF, in)
		if err != nil {
			b.Fatalf("Parse: %v", err)
		}
		if _, err := tree.Serialize(datatree.FormatJSONIETF); err != nil {
			b.Fatalf("Serialize: %v", err)
		}
	}
}
