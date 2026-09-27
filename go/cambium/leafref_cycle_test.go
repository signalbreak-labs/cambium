// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

package cambium_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/signalbreak-labs/cambium/go/cambium"
)

// assertLeafrefCycleError checks that Build rejected a leafref cycle as an
// invalid schema whose message lists the cycle's path chain.
func assertLeafrefCycleError(t *testing.T, err error, want string) {
	t.Helper()
	if err == nil {
		t.Fatalf("Build succeeded, want leafref cycle error %q", want)
	}
	if !strings.Contains(err.Error(), want) {
		t.Fatalf("error = %v, want %q", err, want)
	}
	var lerr *cambium.LeafrefResolutionError
	if !errors.As(err, &lerr) || lerr.Reason != cambium.LeafrefFailureCycle {
		t.Fatalf("error = %v, want a LeafrefResolutionError with reason %q", err, cambium.LeafrefFailureCycle)
	}
	diag := cambium.DiagnosticFromError(err)
	if diag.Kind != cambium.DiagnosticSemanticSchemaError || diag.Code != cambium.RuleCodeContext {
		t.Fatalf("diagnostic kind/code = %q/%q, want %q/%q (%v)", diag.Kind, diag.Code, cambium.DiagnosticSemanticSchemaError, cambium.RuleCodeContext, err)
	}
	if diag.Source.Line == 0 || diag.Path != lerr.Node.Path() || diag.Module != lerr.Node.Module().Name() {
		t.Fatalf("diagnostic = %#v, want the cycle node's module, path, and source", diag)
	}
}

func TestBuildRejectsLeafrefSelfReference(t *testing.T) {
	_, err := buildModeContext(t, cambium.ValidationStrict, nil, `module self-ref {
  yang-version 1.1; namespace "urn:self-ref"; prefix s;
  leaf a { type leafref { path "/s:a"; } }
}`)
	assertLeafrefCycleError(t, err, "leafref chain from /self-ref/a contains a cycle at /self-ref/a: /self-ref/a -> /self-ref/a")
}

func TestBuildRejectsLeafrefTwoCycle(t *testing.T) {
	// entry leads into the cycle without being part of it; the report names
	// the node the walk started from and the cycle itself.
	_, err := buildModeContext(t, cambium.ValidationStrict, nil, `module two-cycle {
  yang-version 1.1; namespace "urn:two-cycle"; prefix t;
  leaf plain { type string; }
  container c {
    leaf entry { type leafref { path "../a"; } }
    leaf a { type leafref { path "/t:c/t:b"; } }
    leaf-list b { type leafref { path "../a"; } }
  }
}`)
	assertLeafrefCycleError(t, err, "leafref chain from /two-cycle/c/entry contains a cycle at /two-cycle/c/a: /two-cycle/c/a -> /two-cycle/c/b -> /two-cycle/c/a")
	// The other cycle member is a related location.
	if related := cambium.DiagnosticFromError(err).Related; len(related) != 1 || related[0].Line != 7 {
		t.Fatalf("related = %#v, want leaf-list b", related)
	}
}

// A three-module cycle without an import cycle: the hub module augments
// leafrefs into both imported modules' trees.
const (
	cycleModA = `module cyc-a {
  yang-version 1.1; namespace "urn:cyc-a"; prefix a;
  container ca;
}`
	cycleModB = `module cyc-b {
  yang-version 1.1; namespace "urn:cyc-b"; prefix b;
  container cb;
}`
	cycleModHub = `module cyc-hub {
  yang-version 1.1; namespace "urn:cyc-hub"; prefix h;
  import cyc-a { prefix a; }
  import cyc-b { prefix b; }
  augment "/a:ca" { leaf x { type leafref { path "/b:cb/h:y"; } } }
  augment "/b:cb" { leaf y { type leafref { path "/h:z"; } } }
  leaf z { type leafref { path "/a:ca/h:x"; } }
}`
)

func TestBuildRejectsLeafrefCycleAcrossModules(t *testing.T) {
	_, err := buildModeContext(t, cambium.ValidationStrict, nil, cycleModA, cycleModB, cycleModHub)
	assertLeafrefCycleError(t, err, "leafref chain from /cyc-a/ca/x contains a cycle at /cyc-a/ca/x: /cyc-a/ca/x -> /cyc-b/cb/y -> /cyc-hub/z -> /cyc-a/ca/x")
}

func TestBuildRejectsLeafrefUnionMemberCycle(t *testing.T) {
	_, err := buildModeContext(t, cambium.ValidationStrict, nil, `module union-cycle {
  yang-version 1.1; namespace "urn:union-cycle"; prefix u;
  typedef ref-b { type leafref { path "/u:b"; } }
  leaf a { type union { type int8; type union { type boolean; type ref-b; } } }
  leaf b { type leafref { path "/u:a"; } }
}`)
	assertLeafrefCycleError(t, err, "leafref chain from /union-cycle/a contains a cycle at /union-cycle/a: /union-cycle/a -> /union-cycle/b -> /union-cycle/a")
}

func TestBuildAcceptsAcyclicLeafrefChains(t *testing.T) {
	// A chain, a diamond onto a shared target, and a union member pointing at
	// a leafref chain are not cycles.
	ctx, err := buildModeContext(t, cambium.ValidationStrict, nil, `module acyclic {
  yang-version 1.1; namespace "urn:acyclic"; prefix ac;
  leaf end { type uint16; }
  leaf mid { type leafref { path "/ac:end"; } }
  leaf first { type leafref { path "/ac:mid"; } }
  leaf other { type leafref { path "/ac:mid"; } }
  leaf mixed { type union { type leafref { path "/ac:first"; } type leafref { path "/ac:other"; } } }
}`)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if warnings := ctx.LoadReport().Warnings; len(warnings) != 0 {
		t.Fatalf("warnings = %#v, want none", warnings)
	}
	mod, err := ctx.Schema("acyclic")
	if err != nil {
		t.Fatalf("Schema: %v", err)
	}
	chain, err := cambium.ResolveLeafrefChain(schemaNodeAt(t, mod, "/ac:first"))
	if err != nil {
		t.Fatalf("ResolveLeafrefChain: %v", err)
	}
	if chain.Target.Name() != "end" || len(chain.Trace) != 2 {
		t.Fatalf("chain = %d hops to %s", len(chain.Trace), chain.Target.Name())
	}
}

func TestLeafrefCycleReportFollowsLoadOrder(t *testing.T) {
	// The walk visits modules in load order and nodes in schema order, so
	// the reported cycle is a function of the load order alone.
	cases := []struct {
		name    string
		sources []string
		want    string
	}{
		{"a first", []string{cycleModA, cycleModB, cycleModHub}, "leafref chain from /cyc-a/ca/x contains a cycle at /cyc-a/ca/x: /cyc-a/ca/x -> /cyc-b/cb/y -> /cyc-hub/z -> /cyc-a/ca/x"},
		{"b first", []string{cycleModB, cycleModA, cycleModHub}, "leafref chain from /cyc-b/cb/y contains a cycle at /cyc-b/cb/y: /cyc-b/cb/y -> /cyc-hub/z -> /cyc-a/ca/x -> /cyc-b/cb/y"},
	}
	for _, tc := range cases {
		for i := 0; i < 10; i++ {
			_, err := buildModeContext(t, cambium.ValidationStrict, nil, tc.sources...)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("%s run %d: error = %v, want %q", tc.name, i, err, tc.want)
			}
		}
	}

	// Two independent cycles: the one reached first in schema order wins.
	const twoCycles = `module two-cycles {
  yang-version 1.1; namespace "urn:two-cycles"; prefix tc;
  leaf p { type leafref { path "/tc:q"; } }
  leaf x { type leafref { path "/tc:y"; } }
  leaf y { type leafref { path "/tc:x"; } }
  leaf q { type leafref { path "/tc:p"; } }
}`
	for i := 0; i < 10; i++ {
		_, err := buildModeContext(t, cambium.ValidationStrict, nil, twoCycles)
		assertLeafrefCycleError(t, err, "leafref chain from /two-cycles/p contains a cycle at /two-cycles/p: /two-cycles/p -> /two-cycles/q -> /two-cycles/p")
	}
}

func TestVendorModeWarnsOnLeafrefCycle(t *testing.T) {
	ctx, err := buildModeContext(t, cambium.ValidationVendorCompatible, nil, `module vendor-cycle {
  yang-version 1.1; namespace "urn:vendor-cycle"; prefix v;
  leaf a { type leafref { path "/v:b"; } }
  leaf b { type leafref { path "/v:a"; } }
  leaf s { type leafref { path "/v:s"; } }
}`)
	if err != nil {
		t.Fatalf("vendor Build: %v", err)
	}
	var messages []string
	for _, w := range ctx.LoadReport().Warnings {
		if w.Kind != cambium.DiagnosticSemanticSchemaError || w.Code != cambium.RuleCodeContext || w.Module != "vendor-cycle" || w.Source.Line == 0 {
			t.Fatalf("warning = %#v", w)
		}
		messages = append(messages, w.Message)
	}
	want := []string{
		"leafref chain from /vendor-cycle/a contains a cycle at /vendor-cycle/a: /vendor-cycle/a -> /vendor-cycle/b -> /vendor-cycle/a; allowed in vendor-compatible mode",
		"leafref chain from /vendor-cycle/s contains a cycle at /vendor-cycle/s: /vendor-cycle/s -> /vendor-cycle/s; allowed in vendor-compatible mode",
	}
	if strings.Join(messages, "\n") != strings.Join(want, "\n") {
		t.Fatalf("warnings =\n%s\nwant\n%s", strings.Join(messages, "\n"), strings.Join(want, "\n"))
	}
	if len(ctx.LoadReport().OmittedContent()) != 0 {
		t.Fatal("a leafref cycle warning must not report omitted content")
	}

	// The chain helper keeps its runtime guard for schemas built this way.
	mod, err := ctx.Schema("vendor-cycle")
	if err != nil {
		t.Fatalf("Schema: %v", err)
	}
	_, err = cambium.ResolveLeafrefChain(schemaNodeAt(t, mod, "/v:a"))
	var lerr *cambium.LeafrefResolutionError
	if !errors.As(err, &lerr) || lerr.Reason != cambium.LeafrefFailureCycle {
		t.Fatalf("ResolveLeafrefChain error = %v, want cycle", err)
	}
}
