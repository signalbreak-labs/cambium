// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

package cambium_test

import (
	"testing"

	"github.com/signalbreak-labs/cambium/go/cambium"
)

const benchmarkSchemaIRSource = `module downstream-ir {
    namespace "urn:downstream-ir";
    prefix di;

    grouping reusable {
        leaf grouped { type string; }
    }

    container top {
        leaf before { type string; }
        uses reusable;
        leaf after {
            config false;
            type uint16;
            default "42";
        }
    }
}`

func BenchmarkBuildSchemaIR(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		builder, err := cambium.NewContextBuilder(cambium.ContextFlags{})
		if err != nil {
			b.Fatal(err)
		}
		if err := builder.LoadModuleStr(benchmarkSchemaIRSource); err != nil {
			b.Fatalf("LoadModuleStr: %v", err)
		}
		ctx, err := builder.Build()
		if err != nil {
			b.Fatalf("Build: %v", err)
		}
		_ = ctx.SchemaIR()
		ctx.Close()
	}
}
