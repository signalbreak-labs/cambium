// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

package compat

import "testing"

func TestDeviateApplicationOrderDeterministic(t *testing.T) {
	type result struct {
		def      string
		errCount int
	}
	var want result
	for i := range 30 {
		root, target := deviateOrderFixture()
		errs := root.ApplyDeviate()
		got := result{errCount: len(errs)}
		if len(target.Default) != 0 {
			got.def = target.Default[0]
		}
		if i == 0 {
			want = got
			continue
		}
		if got != want {
			t.Fatalf("iteration %d result = %#v, want %#v", i, got, want)
		}
	}
}

func deviateOrderFixture() (root, target *Entry) {
	root = &Entry{
		Name: "root",
		Kind: DirectoryEntry,
		Dir:  make(map[string]*Entry),
	}
	target = &Entry{
		Name:   "value",
		Kind:   LeafEntry,
		Parent: root,
	}
	root.Dir[target.Name] = target
	root.Deviations = []*DeviatedEntry{{
		DeviatedPath: "/root/value",
		Entry: &Entry{
			Deviate: map[deviationType][]*Entry{
				DeviationAdd:     {{Default: []string{"add"}}},
				DeviationReplace: {{Default: []string{"replace"}}},
			},
		},
	}}
	return root, target
}
