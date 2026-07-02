// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

package cambium_test

import (
	"fmt"
	"sync"
	"testing"

	"github.com/signalbreak-labs/cambium/go/cambium"
)

func TestBuiltContextConcurrentReadAccess(t *testing.T) {
	const source = `module concurrent-read {
  namespace "urn:cambium:concurrent-read";
  prefix cr;

  container top {
    leaf name { type string; }
    list item {
      key "id";
      leaf id { type string; }
      leaf value { type uint32; }
    }
  }
}`

	builder, err := cambium.NewContextBuilder(cambium.ContextFlags{})
	if err != nil {
		t.Fatalf("NewContextBuilder: %v", err)
	}
	if err := builder.LoadModuleStr(source); err != nil {
		t.Fatalf("LoadModuleStr: %v", err)
	}
	ctx, err := builder.Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	defer ctx.Close()

	const goroutines = 8
	const iterations = 50
	errs := make(chan error, goroutines*iterations)
	var wg sync.WaitGroup
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				mod, err := ctx.Schema("concurrent-read")
				if err != nil {
					errs <- fmt.Errorf("Schema: %w", err)
					return
				}
				if mod.Name() != "concurrent-read" {
					errs <- fmt.Errorf("module name = %q, want concurrent-read", mod.Name())
					return
				}
				if got := len(ctx.Modules()); got != 1 {
					errs <- fmt.Errorf("Modules length = %d, want 1", got)
					return
				}
				if got := len(ctx.SchemaIR().Modules); got != 1 {
					errs <- fmt.Errorf("SchemaIR modules = %d, want 1", got)
					return
				}
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
}
