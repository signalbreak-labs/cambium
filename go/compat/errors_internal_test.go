// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

package compat

import (
	"errors"
	"reflect"
	"testing"

	upstream "github.com/signalbreak-labs/cambium/go/internal/yangparse/upstream/yang"
)

func TestGetErrorsSortsAndDeduplicatesLikeGoyang(t *testing.T) {
	errB := errors.New("b.yang:20:1: later")
	errA := errors.New("a.yang:2:1: earlier")
	errADuplicate := errors.New("a.yang:2:1: earlier")

	compatRoot := &Entry{
		Name:   "root",
		Kind:   DirectoryEntry,
		Errors: []error{errB},
		Dir:    map[string]*Entry{},
	}
	compatChild := &Entry{Name: "child", Kind: LeafEntry, Errors: []error{errA, errADuplicate}, Parent: compatRoot}
	compatRoot.add(compatChild)

	upstreamRoot := &upstream.Entry{
		Name:   "root",
		Kind:   upstream.DirectoryEntry,
		Errors: []error{errB},
		Dir:    map[string]*upstream.Entry{},
	}
	upstreamRoot.Dir["child"] = &upstream.Entry{Name: "child", Kind: upstream.LeafEntry, Errors: []error{errA, errADuplicate}, Parent: upstreamRoot}

	got := errorStrings(compatRoot.GetErrors())
	want := errorStrings(upstreamRoot.GetErrors())
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("GetErrors() = %v, want goyang %v", got, want)
	}
}

func errorStrings(errs []error) []string {
	out := make([]string, 0, len(errs))
	for _, err := range errs {
		out = append(out, err.Error())
	}
	return out
}
