// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

package cambium_test

import (
	"sync"
	"testing"

	"github.com/signalbreak-labs/cambium/go/cambium"
)

// S5: frozen-context concurrent reads, defensive copies and builder lifetime
// for the surfaces this work touched.

func TestS5FrozenContextConcurrentConsumerReads(t *testing.T) {
	ctx, err := buildPolicyContext(t, cambium.DeviationPolicy{IgnoreNotSupported: true}, s4DevTarget, s4Dev)
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			mod, err := ctx.Schema("dt")
			if err != nil {
				t.Error(err)
				return
			}
			top, err := mod.FindPath("/dt:top")
			if err != nil {
				t.Error(err)
				return
			}
			if got := names(top.Traverse(cambium.TraversalListEntryOrder).ConfigOnly()); got == "" {
				t.Error("empty traversal")
			}
			_ = ctx.LoadReport()
			_ = ctx.SchemaIRTable()
			_ = ctx.SchemaIRStats()
			if _, err := cambium.ProjectSchemaPaths(mod, []string{"/dt:top/rows"}, cambium.DefaultProjectionOptions()); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
}

func TestS5ReturnedMetadataIsCopied(t *testing.T) {
	ctx, err := buildPolicyContext(t, cambium.DeviationPolicy{IgnoreNotSupported: true}, s4DevTarget, s4Dev)
	if err != nil {
		t.Fatal(err)
	}
	dv, _ := ctx.Schema("dv")
	devs := dv.Deviations()
	devs[0] = cambium.Deviation{}
	if dv.Deviations()[0].TargetPath() == "" {
		t.Fatal("Module.Deviations shares its backing array")
	}
	report := ctx.LoadReport()
	report.IgnoredDeviations[0] = cambium.Deviation{}
	if ctx.LoadReport().IgnoredDeviations[0].TargetPath() == "" {
		t.Fatal("LoadReport.IgnoredDeviations shares state")
	}
	mod, _ := ctx.Schema("dt")
	rows := schemaNodeAt(t, mod, "/dt:top/rows")
	keys := rows.KeyNames()
	keys[0] = "mutated"
	if rows.KeyNames()[0] != "k" {
		t.Fatal("KeyNames shares state")
	}
	defaults := schemaNodeAt(t, mod, "/dt:top/defaulted").DefaultEntries()
	if len(defaults) != 0 {
		t.Fatalf("defaults after delete deviation = %v", defaults)
	}
}

func TestS5BuilderAndClosedContextBehavior(t *testing.T) {
	builder, err := cambium.NewContextBuilder(cambium.ContextFlags{DisableSearchdirCwd: true})
	if err != nil {
		t.Fatal(err)
	}
	if err := builder.LoadModuleStr(s4DevTarget); err != nil {
		t.Fatal(err)
	}
	ctx, err := builder.Build()
	if err != nil {
		t.Fatal(err)
	}
	// The builder is consumed by Build.
	if err := builder.SetDeviationPolicy(cambium.DeviationPolicy{IgnoreNotSupported: true}); err == nil {
		t.Fatal("SetDeviationPolicy after Build succeeded")
	}
	if _, err := builder.Build(); err == nil {
		t.Fatal("second Build succeeded")
	}
	ctx.Close()
	ctx.Close() // idempotent
	if table := ctx.SchemaIRTable(); len(table.Nodes) != 0 || table.Version != cambium.SchemaIRTableVersion {
		t.Fatalf("closed context table = %+v", table)
	}
	if stats := ctx.SchemaIRStats(); stats != (cambium.SchemaIRStats{}) {
		t.Fatalf("closed context stats = %+v", stats)
	}
	if _, err := ctx.Schema("dt"); err == nil {
		t.Fatal("Schema on closed context succeeded")
	}

	// A failed build returns its error and shares no state with a later,
	// independent builder.
	failing, _ := cambium.NewContextBuilder(cambium.ContextFlags{DisableSearchdirCwd: true})
	_ = failing.LoadModuleStr(completenessTypo)
	if _, err := failing.Build(); err == nil {
		t.Fatal("typo build succeeded")
	}
	fresh, err := buildPolicyContext(t, cambium.DeviationPolicy{}, s4DevTarget)
	if err != nil {
		t.Fatal(err)
	}
	if got := len(fresh.LoadReport().Warnings); got != 0 {
		t.Fatalf("fresh context inherited %d warnings", got)
	}
}
