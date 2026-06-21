// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

//go:build !cgo

package conformance

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/signalbreak-labs/cambium/go/internal/confmanifest"
)

func TestNoCGOConformanceManifestDeclaresSupportedTiers(t *testing.T) {
	dir := findConformanceDirNoCGO(t)
	cases, err := confmanifest.Load(filepath.Join(dir, "manifest.toml"))
	if err != nil {
		t.Fatalf("Load manifest: %v", err)
	}

	var schemaIR, backendData int
	for _, c := range cases {
		switch c.EffectiveTier() {
		case confmanifest.TierSchemaIR:
			schemaIR++
			if c.ExpectedIR == "" {
				t.Fatalf("schema-ir case %q missing expected-ir", c.Name)
			}
			if c.Input != "" || c.InputFormat != "" || len(c.Expected) != 0 {
				t.Fatalf("schema-ir case %q contains backend-data fields", c.Name)
			}
		case confmanifest.TierBackendData:
			backendData++
			if c.Input == "" || c.InputFormat == "" || len(c.Expected) == 0 {
				t.Fatalf("backend-data case %q missing input, input-format, or expected outputs", c.Name)
			}
		default:
			t.Fatalf("case %q has unsupported tier %q", c.Name, c.Tier)
		}
	}
	if schemaIR == 0 {
		t.Fatal("manifest has no schema-ir cases for the cgo-free schema tier")
	}
	if backendData == 0 {
		t.Fatal("manifest has no backend-data cases for the optional cgo runner")
	}
}

func findConformanceDirNoCGO(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		candidate := filepath.Join(dir, "conformance", "manifest.toml")
		if _, err := os.Stat(candidate); err == nil {
			return filepath.Join(dir, "conformance")
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("could not locate conformance/manifest.toml above %s", dir)
		}
		dir = parent
	}
}
