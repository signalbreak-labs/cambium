// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

package cambium_test

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/signalbreak-labs/cambium/go/cambium"
)

// doublingGroupingModule returns a module in which grouping g<i> uses g<i-1>
// under two containers, so "uses g<depth>" expands to about 3*2^depth schema
// nodes from a source that grows only linearly with depth.
func doublingGroupingModule(depth int) string {
	var b strings.Builder
	b.WriteString("module doubling {\n  yang-version 1.1; namespace \"urn:doubling\"; prefix d;\n")
	b.WriteString("  grouping g0 { leaf l { type string; } }\n")
	for i := 1; i <= depth; i++ {
		fmt.Fprintf(&b, "  grouping g%d { container x { uses g%d; } container y { uses g%d; } }\n", i, i-1, i-1)
	}
	fmt.Fprintf(&b, "  container top { uses g%d; }\n}\n", depth)
	return b.String()
}

func buildWithSchemaNodeLimit(t *testing.T, limit uint64, sources ...string) (*cambium.Context, error) {
	t.Helper()
	builder, err := cambium.NewContextBuilder(cambium.ContextFlags{DisableSearchdirCwd: true})
	if err != nil {
		t.Fatal(err)
	}
	if err := builder.SetMaxSchemaNodes(limit); err != nil {
		t.Fatalf("SetMaxSchemaNodes(%d): %v", limit, err)
	}
	for _, source := range sources {
		if err := builder.LoadModuleStr(source); err != nil {
			t.Fatalf("LoadModuleStr: %v", err)
		}
	}
	ctx, err := builder.Build()
	if ctx != nil {
		t.Cleanup(ctx.Close)
	}
	return ctx, err
}

func requireSchemaNodeLimitError(t *testing.T, err error, limit uint64) {
	t.Helper()
	if err == nil {
		t.Fatalf("Build succeeded, want a schema node limit (%d) error", limit)
	}
	var cerr *cambium.Error
	if !errors.As(err, &cerr) {
		t.Fatalf("error %T is not *cambium.Error: %v", err, err)
	}
	diag := cambium.DiagnosticFromError(err)
	if diag.Kind != cambium.DiagnosticResourceLimit {
		t.Fatalf("diagnostic kind = %q, want %q (%v)", diag.Kind, cambium.DiagnosticResourceLimit, err)
	}
	for _, want := range []string{fmt.Sprintf("limit of %d schema nodes", limit), "SetMaxSchemaNodes"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error %q does not mention %q", err, want)
		}
	}
}

// TestBuildSchemaNodeLimitCountsInstantiatedNodes pins the unit of the limit:
// each schema node Build instantiates counts once.
func TestBuildSchemaNodeLimitCountsInstantiatedNodes(t *testing.T) {
	// container c, leaf a, leaf b, leaf d: four schema nodes.
	source := `module budget-flat {
  namespace "urn:budget-flat"; prefix bf;
  container c { leaf a { type string; } leaf b { type string; } }
  leaf d { type string; }
}`
	if _, err := buildWithSchemaNodeLimit(t, 4, source); err != nil {
		t.Fatalf("Build at exactly the node count: %v", err)
	}
	_, err := buildWithSchemaNodeLimit(t, 3, source)
	requireSchemaNodeLimitError(t, err, 3)
}

// TestBuildSchemaNodeLimitStopsGroupingExpansionEarly proves the limit is
// checked while nodes are created: an expansion of about 3*2^40 nodes fails
// fast instead of exhausting memory.
func TestBuildSchemaNodeLimitStopsGroupingExpansionEarly(t *testing.T) {
	start := time.Now()
	_, err := buildWithSchemaNodeLimit(t, 1000, doublingGroupingModule(40))
	requireSchemaNodeLimitError(t, err, 1000)
	if elapsed := time.Since(start); elapsed > 10*time.Second {
		t.Fatalf("Build took %v to hit a 1000-node limit", elapsed)
	}
}

func TestBuildSchemaNodeLimitAdmitsSmallExpansions(t *testing.T) {
	ctx, err := buildWithSchemaNodeLimit(t, 1000, doublingGroupingModule(4))
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	mod, err := ctx.Schema("doubling")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := mod.FindPath("/d:top/d:x/d:y/d:x/d:y/d:l"); err != nil {
		t.Fatalf("expanded leaf missing: %v", err)
	}
}

func TestSetMaxSchemaNodesZeroRestoresDefault(t *testing.T) {
	builder, err := cambium.NewContextBuilder(cambium.ContextFlags{DisableSearchdirCwd: true})
	if err != nil {
		t.Fatal(err)
	}
	if err := builder.SetMaxSchemaNodes(1); err != nil {
		t.Fatal(err)
	}
	if err := builder.SetMaxSchemaNodes(0); err != nil {
		t.Fatal(err)
	}
	if err := builder.LoadModuleStr(doublingGroupingModule(6)); err != nil {
		t.Fatal(err)
	}
	ctx, err := builder.Build()
	if err != nil {
		t.Fatalf("Build with the default limit: %v", err)
	}
	ctx.Close()
	if err := builder.SetMaxSchemaNodes(10); err == nil {
		t.Fatal("SetMaxSchemaNodes accepted after Build")
	}
	// The documented default (spec/api.md, CHANGELOG) leaves room for the
	// largest vendor schemas measured (about 3.3 million instantiations).
	if cambium.DefaultMaxSchemaNodes != 8_388_608 {
		t.Fatalf("DefaultMaxSchemaNodes = %d, want the documented 8,388,608", cambium.DefaultMaxSchemaNodes)
	}
}
