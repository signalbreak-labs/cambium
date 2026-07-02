// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

package compat

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/signalbreak-labs/cambium/go/cambium"
)

const orderedSyncFixture = `module compat-ordered-sync {
  yang-version 1.1;
  namespace "urn:compat-ordered-sync";
  prefix cos;

  grouping nested {
    leaf grouped-a { type string; }
  }

  grouping common {
    leaf grouped-z { type string; }
    uses nested;
  }

  container top {
    leaf before { type string; }
    uses common;
    choice pick {
      leaf direct { type string; }
    }
    leaf deviated { type string; }
    leaf after { type string; }
  }

  augment "/cos:top" {
    leaf augmented { type string; }
  }

  deviation "/cos:top/cos:deviated" {
    deviate not-supported;
  }
}
`

func TestSupportedEntryBuildersKeepDirChildrenOrdered(t *testing.T) {
	t.Run("raw ToEntry with augment deviation uses", func(t *testing.T) {
		ms := NewModules()
		if err := ms.Parse(orderedSyncFixture, "compat-ordered-sync.yang"); err != nil {
			t.Fatalf("Parse: %v", err)
		}
		root := ToEntry(ms.Modules["compat-ordered-sync"])
		assertNoDirOnlyChildren(t, root)

		if processed, skipped := root.Augment(false); processed != 1 || skipped != 0 {
			t.Fatalf("Augment(false) = (%d,%d), want (1,0)", processed, skipped)
		}
		root.FixChoice()
		if errs := root.ApplyDeviate(); len(errs) != 0 {
			t.Fatalf("ApplyDeviate errors = %v, want none", errs)
		}
		assertNoDirOnlyChildren(t, root)
	})

	t.Run("processed native module with augment deviation uses", func(t *testing.T) {
		builder, err := cambium.NewContextBuilder(cambium.ContextFlags{})
		if err != nil {
			t.Fatalf("NewContextBuilder: %v", err)
		}
		if err := builder.LoadModuleStr(orderedSyncFixture); err != nil {
			t.Fatalf("LoadModuleStr: %v", err)
		}
		ctx, err := builder.Build()
		if err != nil {
			t.Fatalf("Build: %v", err)
		}
		defer ctx.Close()

		mod, err := ctx.Schema("compat-ordered-sync")
		if err != nil {
			t.Fatalf("Schema: %v", err)
		}
		root := FromModule(mod)
		assertNoDirOnlyChildren(t, root)
	})
}

func TestOrderedEntryChildPairsManualDirFallbackRemainsAlphabetical(t *testing.T) {
	root := &Entry{
		Name: "manual",
		Dir: map[string]*Entry{
			"z-last":  {Name: "z-last"},
			"a-first": {Name: "a-first"},
		},
	}
	got := make([]string, 0, len(root.Dir))
	for _, pair := range orderedEntryChildPairs(root) {
		got = append(got, pair.key)
	}
	if want := []string{"a-first", "z-last"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("orderedEntryChildPairs manual fallback = %v, want %v", got, want)
	}
}

func assertNoDirOnlyChildren(t *testing.T, root *Entry) {
	t.Helper()
	seen := map[*Entry]bool{}
	var walk func(*Entry, string)
	walk = func(entry *Entry, path string) {
		t.Helper()
		if entry == nil || seen[entry] {
			return
		}
		seen[entry] = true
		orderedKeys := map[string]bool{}
		for _, child := range entry.ordered {
			key, ok := entry.keyForChild(child)
			if !ok {
				t.Fatalf("%s: ordered child %#v is not present in Dir", path, child)
			}
			orderedKeys[key] = true
		}
		for key, child := range entry.Dir {
			if !orderedKeys[key] {
				t.Fatalf("%s: Dir child %q is missing from ordered children", path, key)
			}
			walk(child, path+"/"+key)
		}
		for i, augment := range entry.Augments {
			walk(augment, fmt.Sprintf("%s/augment[%d]", path, i))
		}
		for i, augment := range entry.Augmented {
			walk(augment, fmt.Sprintf("%s/augmented[%d]", path, i))
		}
		for i, augment := range entry.AugmentedBy {
			walk(augment, fmt.Sprintf("%s/augmented-by[%d]", path, i))
		}
		for i, deviation := range entry.Deviations {
			if deviation != nil {
				walk(deviation.Entry, fmt.Sprintf("%s/deviation[%d]", path, i))
			}
		}
		for kind, entries := range entry.Deviate {
			for i, deviate := range entries {
				walk(deviate, fmt.Sprintf("%s/deviate[%s:%d]", path, kind, i))
			}
		}
	}
	walk(root, root.Name)
}
