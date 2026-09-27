// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

package compat_test

import (
	"testing"

	"github.com/signalbreak-labs/cambium/go/compat"
)

// TestFindModuleByNamespaceAmbiguityErrorUsesLoadOrder guards against map
// iteration reaching the error text: when several recorded modules share a
// namespace, the error names the first two in load order, every time.
func TestFindModuleByNamespaceAmbiguityErrorUsesLoadOrder(t *testing.T) {
	const want = "namespace urn:compat-shared-ns matches two or more modules (compat-ns-y, compat-ns-x)"
	for i := 0; i < 50; i++ {
		ms := compat.NewModules()
		// Load order (y, x, w) differs from name order on purpose.
		for _, name := range []string{"compat-ns-y", "compat-ns-x", "compat-ns-w"} {
			source := "module " + name + " { namespace \"urn:compat-shared-ns\"; prefix p; }"
			if err := ms.Parse(source, name+".yang"); err != nil {
				t.Fatalf("Parse %s: %v", name, err)
			}
		}
		_, err := ms.FindModuleByNamespace("urn:compat-shared-ns")
		if err == nil {
			t.Fatal("FindModuleByNamespace accepted an ambiguous namespace")
		}
		if err.Error() != want {
			t.Fatalf("run %d: error = %q, want %q", i, err, want)
		}
	}
}

// TestFindModuleByNamespaceAmbiguityErrorForDirectRecordsIsSorted covers
// records placed in the public Modules map directly, which have no load order:
// they are reported by map key.
func TestFindModuleByNamespaceAmbiguityErrorForDirectRecordsIsSorted(t *testing.T) {
	const want = "namespace urn:direct matches two or more modules (direct-a, direct-b)"
	for i := 0; i < 50; i++ {
		ms := &compat.Modules{Modules: map[string]*compat.Module{}}
		for _, name := range []string{"direct-c", "direct-b", "direct-a"} {
			ms.Modules[name] = &compat.Module{Name: name, Namespace: &compat.Value{Name: "urn:direct"}}
		}
		_, err := ms.FindModuleByNamespace("urn:direct")
		if err == nil || err.Error() != want {
			t.Fatalf("run %d: error = %v, want %q", i, err, want)
		}
	}
}
