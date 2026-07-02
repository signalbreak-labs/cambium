// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

package compat_test

import (
	"testing"

	"github.com/signalbreak-labs/cambium/go/compat"
)

const benchmarkCompatSource = `module compat-modules-demo {
    namespace "urn:compat-modules-demo";
    prefix cmd;

    container z-top {
        leaf zed { type string; }
    }
    container a-top {
        leaf alpha { type string; }
    }
}
`

func BenchmarkCompatToEntry(b *testing.B) {
	ms := compat.NewModules()
	ms.ParseOptions.DeviateOptions = compat.DeviateOptions{IgnoreDeviateNotSupported: true}
	if err := ms.Parse(benchmarkCompatSource, "compat-modules-demo.yang"); err != nil {
		b.Fatalf("Parse: %v", err)
	}
	if errs := ms.Process(); len(errs) != 0 {
		b.Fatalf("Process errors: %v", errs)
	}
	module := ms.Modules["compat-modules-demo"]
	if module == nil {
		b.Fatal("parsed module not found")
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ms.ClearEntryCache()
		if entry := compat.ToEntry(module); entry == nil {
			b.Fatal("ToEntry returned nil")
		}
	}
}
