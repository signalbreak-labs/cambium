// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

package cambium_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/signalbreak-labs/cambium/go/cambium"
)

func TestDiffContextsFollowsContextLoadOrder(t *testing.T) {
	const beta = `module beta {
  yang-version 1.1; namespace "urn:beta"; prefix b;
  revision 2026-01-01;
  container b;
}`
	const alphaOld = `module alpha {
  yang-version 1.1; namespace "urn:alpha"; prefix a;
  container top { leaf x { type string; } }
}`
	const alphaNew = `module alpha {
  yang-version 1.1; namespace "urn:alpha"; prefix a;
  container top { leaf x { type int32; } leaf y { type string; } }
}`
	const delta = `module delta {
  yang-version 1.1; namespace "urn:delta"; prefix d;
  container d;
}`
	oldCtx := loadDownstreamContext(t, beta, alphaOld)
	newCtx := loadDownstreamContext(t, delta, alphaNew)

	diff, err := cambium.DiffContexts(oldCtx, newCtx)
	if err != nil {
		t.Fatalf("DiffContexts: %v", err)
	}
	if diff.Version != cambium.SchemaDiffVersion {
		t.Fatalf("version = %q", diff.Version)
	}
	oldAlpha, _ := oldCtx.Schema("alpha")
	newAlpha, _ := newCtx.Schema("alpha")
	alphaDiff, err := cambium.DiffModules(oldAlpha, newAlpha)
	if err != nil {
		t.Fatalf("DiffModules: %v", err)
	}
	if len(alphaDiff.Changes) == 0 {
		t.Fatal("alpha diff is empty")
	}

	// Old modules in old load order (beta removed, then alpha's changes),
	// then modules only in the new context in new load order.
	var got []string
	for _, change := range diff.Changes {
		got = append(got, string(change.Kind)+" "+change.Path)
	}
	want := []string{string(cambium.SchemaDiffNodeRemoved) + " /beta@2026-01-01"}
	for _, change := range alphaDiff.Changes {
		want = append(want, string(change.Kind)+" "+change.Path)
	}
	want = append(want, string(cambium.SchemaDiffNodeAdded)+" /delta")
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("changes = %v, want %v", got, want)
	}
	removed := diff.Changes[0]
	if removed.OldValue != "present" || removed.NewValue != "" || removed.QualifiedPath != "/beta@2026-01-01" {
		t.Fatalf("removed module change = %#v", removed)
	}
}

func TestDiffContextsScalesWithUniqueNodes(t *testing.T) {
	// A v1 projection of this schema would hold 2^41-1 records. The context
	// diff must enumerate modules without building it.
	oldCtx := loadDownstreamContext(t, nestedChainModule(40))
	newCtx := loadDownstreamContext(t, strings.Replace(nestedChainModule(40), "leaf end { type string; }", "leaf end { type uint8; }", 1))
	diff, err := cambium.DiffContexts(oldCtx, newCtx)
	if err != nil {
		t.Fatalf("DiffContexts: %v", err)
	}
	if len(diff.Changes) != 1 || !strings.HasSuffix(diff.Changes[0].Path, "/c39/end") {
		t.Fatalf("changes = %#v, want one type change on the leaf", diff.Changes)
	}
}
