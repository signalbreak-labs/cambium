// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

//go:build cgo

package libyangbackend_test

import (
	"fmt"
	"sync"
	"testing"

	cambium "github.com/signalbreak-labs/cambium/go/libyangbackend"
)

func TestContextSchemaAccessConcurrent(t *testing.T) {
	ctx, _ := loadValidationContext(t)
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
				mod, err := ctx.Schema("cambium-validation-demo")
				if err != nil {
					errs <- fmt.Errorf("Schema: %w", err)
					return
				}
				if mod.Name() != "cambium-validation-demo" {
					errs <- fmt.Errorf("module name = %q, want cambium-validation-demo", mod.Name())
					return
				}
				if !modulePresent(ctx.Modules(), "cambium-validation-demo") {
					errs <- fmt.Errorf("Modules missing cambium-validation-demo")
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

func modulePresent(mods []cambium.Module, name string) bool {
	for _, mod := range mods {
		if mod.Name() == name {
			return true
		}
	}
	return false
}

func TestContextParseValidateConcurrent(t *testing.T) {
	ctx, _ := loadValidationContext(t)
	defer ctx.Close()

	const goroutines = 8
	const iterations = 25
	errs := make(chan error, goroutines*iterations)
	var wg sync.WaitGroup
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				tree, err := ctx.Parse(
					cambium.FormatXML,
					cambium.ParseMode{ParseOnly: true},
					[]byte(`<top xmlns="urn:cambium:validation"><name>open</name><c><x>1</x></c><z>ok</z></top>`),
				)
				if err != nil {
					errs <- fmt.Errorf("Parse: %w", err)
					return
				}
				if err := tree.Validate(cambium.ValidateMode{Present: true}); err != nil {
					tree.Close()
					errs <- fmt.Errorf("Validate: %w", err)
					return
				}
				if _, err := tree.Serialize(cambium.FormatXML, cambium.DefaultSerializeFlags()); err != nil {
					tree.Close()
					errs <- fmt.Errorf("Serialize: %w", err)
					return
				}
				tree.Close()
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
}
