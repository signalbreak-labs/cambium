// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

//go:build cgo

package main

import (
	"path/filepath"
	"testing"

	"github.com/signalbreak-labs/cambium/go/conformance"
	"github.com/signalbreak-labs/cambium/go/internal/confmanifest"
)

func TestEnabledNamesExistInManifest(t *testing.T) {
	dir, err := conformance.FindConformanceDir()
	if err != nil {
		t.Fatalf("FindConformanceDir: %v", err)
	}
	cases, err := confmanifest.Load(filepath.Join(dir, "manifest.toml"))
	if err != nil {
		t.Fatalf("Load manifest: %v", err)
	}
	present := make(map[string]struct{}, len(cases))
	for _, c := range cases {
		present[c.Name] = struct{}{}
	}
	for _, name := range enabled {
		if _, ok := present[name]; !ok {
			t.Errorf("enabled name %q is not in conformance/manifest.toml", name)
		}
	}
}
