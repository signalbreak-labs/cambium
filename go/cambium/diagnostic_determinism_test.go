// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

package cambium_test

import (
	"strings"
	"testing"

	"github.com/signalbreak-labs/cambium/go/cambium"
)

// TestUnknownEnabledFeatureErrorIsDeterministic guards against map iteration
// reaching a diagnostic: with several unknown features across modules, Build
// reports the first module in load order and, within it, the first unknown
// feature by name, every time.
func TestUnknownEnabledFeatureErrorIsDeterministic(t *testing.T) {
	var first string
	for i := 0; i < 50; i++ {
		builder, err := cambium.NewContextBuilder(cambium.ContextFlags{DisableSearchdirCwd: true})
		if err != nil {
			t.Fatal(err)
		}
		// Load order (feat-b, then feat-a) differs from name order on purpose.
		for _, source := range []string{
			`module feat-b { namespace "urn:feat-b"; prefix b; feature known; }`,
			`module feat-a { namespace "urn:feat-a"; prefix a; feature known; }`,
		} {
			if err := builder.LoadModuleStr(source); err != nil {
				t.Fatalf("LoadModuleStr: %v", err)
			}
		}
		if err := builder.SetFeatures("feat-a", []string{"a4", "a3", "a2", "a1"}); err != nil {
			t.Fatal(err)
		}
		if err := builder.SetFeatures("feat-b", []string{"known", "u4", "u2", "u3", "u1"}); err != nil {
			t.Fatal(err)
		}
		_, err = builder.Build()
		if err == nil {
			t.Fatal("Build accepted unknown enabled features")
		}
		if i == 0 {
			first = err.Error()
			if !strings.Contains(first, `unknown feature "u1" for module "feat-b"`) {
				t.Fatalf("Build error = %q, want the first unknown feature of the first loaded module", first)
			}
			continue
		}
		if got := err.Error(); got != first {
			t.Fatalf("Build error changed between runs:\n first: %s\n   now: %s", first, got)
		}
	}
}
