// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

package cambium_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/signalbreak-labs/cambium/go/cambium"
)

func TestLoadReportSurfacesRebuildFailure(t *testing.T) {
	dir := t.TempDir()
	writeModuleFile(t, filepath.Join(dir, "load-report-rebuild.yang"), []byte(`module load-report-rebuild {
    namespace "urn:load-report-rebuild";
    prefix lrr;

    feature declared;

    container top {
        leaf value { type string; }
    }
}`))

	ctx, err := cambium.NewContext()
	if err != nil {
		t.Fatal(err)
	}
	defer ctx.Close()
	if err := ctx.SetSearchPath(dir); err != nil {
		t.Fatalf("SetSearchPath: %v", err)
	}
	if err := ctx.LoadModule("load-report-rebuild"); err != nil {
		t.Fatalf("LoadModule: %v", err)
	}
	if _, err := ctx.Schema("load-report-rebuild"); err != nil {
		t.Fatalf("Schema: %v", err)
	}
	if err := ctx.SetFeatures("load-report-rebuild", []string{"no-such-feature"}); err != nil {
		t.Fatalf("SetFeatures: %v", err)
	}

	report := ctx.LoadReport()
	for _, warning := range report.Warnings {
		if strings.Contains(warning.Message, "schema rebuild") &&
			strings.Contains(warning.Message, `unknown feature "no-such-feature"`) {
			return
		}
	}
	t.Fatalf("LoadReport warnings = %#v, want schema rebuild warning", report.Warnings)
}
