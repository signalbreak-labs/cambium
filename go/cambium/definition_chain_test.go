// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

package cambium_test

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/signalbreak-labs/cambium/go/cambium"
)

// definitionChainModule returns a module holding a chain of n+1 definitions of
// kind, each defined in terms of the previous one, plus one leaf using the
// last typedef or gated by the last feature.
func definitionChainModule(kind string, n int) string {
	var b strings.Builder
	b.WriteString("module chain {\n  yang-version 1.1; namespace \"urn:chain\"; prefix c;\n")
	switch kind {
	case "typedef":
		b.WriteString("  typedef t0 { type string; }\n")
		for i := 1; i <= n; i++ {
			fmt.Fprintf(&b, "  typedef t%d { type t%d; }\n", i, i-1)
		}
		fmt.Fprintf(&b, "  leaf x { type t%d; }\n", n)
	case "feature":
		b.WriteString("  feature f0;\n")
		for i := 1; i <= n; i++ {
			fmt.Fprintf(&b, "  feature f%d { if-feature f%d; }\n", i, i-1)
		}
		fmt.Fprintf(&b, "  leaf x { if-feature f%d; type string; }\n", n)
	case "identity":
		b.WriteString("  identity i0;\n")
		for i := 1; i <= n; i++ {
			fmt.Fprintf(&b, "  identity i%d { base i%d; }\n", i, i-1)
		}
	}
	b.WriteString("}\n")
	return b.String()
}

func buildDefinitionChain(tb testing.TB, kind string, n int) (*cambium.Context, time.Duration) {
	tb.Helper()
	builder, err := cambium.NewContextBuilder(cambium.ContextFlags{DisableSearchdirCwd: true})
	if err != nil {
		tb.Fatal(err)
	}
	if err := builder.LoadModuleStr(definitionChainModule(kind, n)); err != nil {
		tb.Fatalf("LoadModuleStr: %v", err)
	}
	if kind == "feature" {
		features := make([]string, 0, n+1)
		for i := 0; i <= n; i++ {
			features = append(features, fmt.Sprintf("f%d", i))
		}
		if err := builder.SetFeatures("chain", features); err != nil {
			tb.Fatal(err)
		}
	}
	start := time.Now()
	ctx, err := builder.Build()
	elapsed := time.Since(start)
	if err != nil {
		tb.Fatalf("Build %s chain of %d: %v", kind, n, err)
	}
	tb.Cleanup(ctx.Close)
	return ctx, elapsed
}

// TestLongDefinitionChainsBuildQuickly guards against cubic resolution of
// typedef, if-feature, and identity chains. Before the fix these took about
// 70 s (2000 typedefs), 70 s (4000 enabled features), and 20-30 s (4000
// identities); now typedef and feature chains take about 0.1 s and the
// identity chain about 1 s, as identity chains stay quadratic: every ancestor
// lists all its derived identities. The bound is generous on purpose for
// loaded CI runners, and wider still under the race detector.
func TestLongDefinitionChainsBuildQuickly(t *testing.T) {
	bound := 10 * time.Second
	if raceDetectorEnabled {
		bound *= 3
	}
	for _, tc := range []struct {
		kind string
		n    int
	}{{"typedef", 2000}, {"feature", 4000}, {"identity", 4000}} {
		kind, n := tc.kind, tc.n
		t.Run(kind, func(t *testing.T) {
			ctx, elapsed := buildDefinitionChain(t, kind, n)
			if elapsed > bound {
				t.Fatalf("Build of a %d-long %s chain took %v (bound %v)", n, kind, elapsed, bound)
			}
			mod, err := ctx.Schema("chain")
			if err != nil {
				t.Fatal(err)
			}
			switch kind {
			case "typedef":
				x, err := mod.FindPath("/c:x")
				if err != nil {
					t.Fatal(err)
				}
				info, ok := x.LeafType()
				if !ok {
					t.Fatal("leaf x has no type")
				}
				chain := info.TypedefChain()
				if len(chain) != n+1 || chain[0] != fmt.Sprintf("t%d", n) || chain[n] != "t0" {
					t.Fatalf("TypedefChain has %d names from %q to %q, want t%d..t0", len(chain), chain[0], chain[len(chain)-1], n)
				}
				if info.Base() != cambium.BaseTypeString {
					t.Fatalf("base = %v, want string", info.Base())
				}
			case "feature":
				if _, ok := mod.Children().Lookup("x"); !ok {
					t.Fatal("leaf x gated by the enabled feature chain is missing")
				}
			case "identity":
				root, ok := mod.Identity("i0")
				if !ok {
					t.Fatal("identity i0 missing")
				}
				derived := root.Derived()
				if len(derived) != n || derived[0].Name() != "i1" || derived[n-1].Name() != fmt.Sprintf("i%d", n) {
					t.Fatalf("i0 has %d derived identities, want i1..i%d in order", len(derived), n)
				}
			}
		})
	}
}

// TestIdentityDiamondDerivedListsOncePerAncestor pins Derived() for shared
// ancestors: through one base whose ancestry is a diamond, and through several
// bases that are declared after the identity deriving from them.
func TestIdentityDiamondDerivedListsOncePerAncestor(t *testing.T) {
	builder, err := cambium.NewContextBuilder(cambium.ContextFlags{DisableSearchdirCwd: true})
	if err != nil {
		t.Fatal(err)
	}
	source := `module diamond {
  yang-version 1.1; namespace "urn:diamond"; prefix d;
  identity y { base p; base q; }
  identity e;
  identity c { base e; }
  identity f { base e; }
  identity b { base c; base f; }
  identity x { base b; }
  identity p { base r; }
  identity q { base r; base e; }
  identity r { base e; }
}`
	if err := builder.LoadModuleStr(source); err != nil {
		t.Fatal(err)
	}
	ctx, err := builder.Build()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(ctx.Close)
	mod, err := ctx.Schema("diamond")
	if err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string][]string{
		"e": {"r", "p", "y", "q", "c", "f", "b", "x"},
		"r": {"p", "y", "q"},
		"c": {"b", "x"},
		"f": {"b", "x"},
		"b": {"x"},
		"p": {"y"},
		"q": {"y"},
	} {
		id, ok := mod.Identity(name)
		if !ok {
			t.Fatalf("identity %s missing", name)
		}
		var got []string
		for _, derived := range id.Derived() {
			got = append(got, derived.Name())
		}
		if strings.Join(got, ",") != strings.Join(want, ",") {
			t.Errorf("%s derived = %v, want %v", name, got, want)
		}
	}
}

func BenchmarkBuildDefinitionChain(b *testing.B) {
	for _, kind := range []string{"typedef", "feature", "identity"} {
		for _, n := range []int{500, 1000, 2000} {
			b.Run(fmt.Sprintf("%s/n=%d", kind, n), func(b *testing.B) {
				for i := 0; i < b.N; i++ {
					buildDefinitionChain(b, kind, n)
				}
			})
		}
	}
}
