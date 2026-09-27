// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

//go:build cgo

package conformance

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/signalbreak-labs/cambium/go/internal/confmanifest"
)

// verdictModule has a mandatory leaf and a must over a leaf whose default
// value only libyang instantiates, so datatree and libyang can be made to
// disagree on purpose.
const verdictModule = `module verdict {
  yang-version 1.1;
  namespace "urn:verdict";
  prefix v;

  container c {
    leaf name { type string; mandatory true; }
    leaf d { type string; default "x"; }
    leaf eq { type string; must "../d = 'x'"; }
    leaf ne { type string; must "not(../d = 'x')"; }
  }
}
`

// writeVerdictCase lays out a one-case conformance directory for input and
// returns it with its manifest entry.
func writeVerdictCase(t *testing.T, expect confmanifest.Expect, input string) (dir string, c Case) {
	t.Helper()
	dir = t.TempDir()
	moduleDir := filepath.Join(dir, "fixtures", "verdict", "module")
	if err := os.MkdirAll(moduleDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(moduleDir, "verdict.yang"), []byte(verdictModule), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "fixtures", "verdict", "input.json"), []byte(input), 0o644); err != nil {
		t.Fatal(err)
	}
	c = Case{
		Name:        "verdict",
		Expect:      expect,
		Module:      "fixtures/verdict/module",
		Input:       "fixtures/verdict/input.json",
		InputFormat: "json_ietf",
		DataTree:    true,
		Expected:    map[string]string{},
	}
	if expect != confmanifest.ExpectReject {
		c.Expected["json_ietf"] = "golden/verdict/output.json_ietf"
	}
	return dir, c
}

func wantErrContaining(t *testing.T, err error, want string) {
	t.Helper()
	if err == nil {
		t.Fatalf("got nil error, want one containing %q", want)
	}
	if !strings.Contains(err.Error(), want) {
		t.Fatalf("error %q does not contain %q", err, want)
	}
}

// A reject case passes the libyang lane only when libyang's validating parse
// refuses the document.
func TestRunCaseRejectCase(t *testing.T) {
	t.Setenv("CAMBIUM_YANGLINT", "")
	dir, c := writeVerdictCase(t, confmanifest.ExpectReject, `{"verdict:c":{"eq":"a"}}`)
	if err := RunCase(dir, c); err != nil {
		t.Fatalf("RunCase(missing mandatory leaf) = %v, want the rejection to pass", err)
	}

	dir, c = writeVerdictCase(t, confmanifest.ExpectReject, `{"verdict:c":{"name":"n"}}`)
	wantErrContaining(t, RunCase(dir, c), "libyang: "+msgAcceptedReject)
}

// Unknown data is an error for the verdict (a strict parse), as for yanglint.
func TestRunCaseRejectCaseParsesStrictly(t *testing.T) {
	t.Setenv("CAMBIUM_YANGLINT", "")
	dir, c := writeVerdictCase(t, confmanifest.ExpectReject, `{"verdict:c":{"name":"n","bogus":"b"}}`)
	if err := RunCase(dir, c); err != nil {
		t.Fatalf("RunCase(unknown leaf) = %v, want the rejection to pass", err)
	}
}

// A harness failure is never a rejection: a module that does not load or an
// input that is missing fails the case instead of passing it.
func TestRunCaseRejectCaseHarnessFailures(t *testing.T) {
	t.Setenv("CAMBIUM_YANGLINT", "")
	dir, c := writeVerdictCase(t, confmanifest.ExpectReject, `{"verdict:c":{"eq":"a"}}`)
	c.Input = "fixtures/verdict/missing.json"
	wantErrContaining(t, RunCase(dir, c), "read input")

	dir, c = writeVerdictCase(t, confmanifest.ExpectReject, `{"verdict:c":{"eq":"a"}}`)
	broken := filepath.Join(dir, c.Module, "verdict.yang")
	if err := os.WriteFile(broken, []byte("module verdict {"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := RunCase(dir, c); err == nil || strings.Contains(err.Error(), "accepted") {
		t.Fatalf("RunCase(broken module) = %v, want a harness failure", err)
	}
}

// An empty document cannot show a rejection: the backend reports an error for
// any document without data, valid or not.
func TestRunCaseRejectCaseNeedsData(t *testing.T) {
	t.Setenv("CAMBIUM_YANGLINT", "")
	for _, input := range []string{`{}`, " \n"} {
		dir, c := writeVerdictCase(t, confmanifest.ExpectReject, input)
		wantErrContaining(t, RunCase(dir, c), "needs at least one top-level data node")
	}
}

// With CAMBIUM_YANGLINT set, an oracle reject case must also be refused by
// yanglint; a harness failure of the oracle is not a rejection.
func TestRunCaseRejectCaseConsultsYanglintOracle(t *testing.T) {
	dir, c := writeVerdictCase(t, confmanifest.ExpectReject, `{"verdict:c":{"eq":"a"}}`)
	c.Oracle = true
	fake := func(t *testing.T, script string) {
		t.Helper()
		path := filepath.Join(t.TempDir(), "yanglint")
		if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
			t.Fatal(err)
		}
		t.Setenv("CAMBIUM_YANGLINT", path)
	}

	fake(t, "#!/bin/sh\necho 'libyang[0]: Mandatory node \"name\" instance does not exist.' >&2\nexit 1\n")
	if err := RunCase(dir, c); err != nil {
		t.Fatalf("RunCase with a rejecting oracle = %v, want nil", err)
	}

	fake(t, "#!/bin/sh\nexit 0\n")
	wantErrContaining(t, RunCase(dir, c), "yanglint oracle: "+msgAcceptedReject)

	t.Setenv("CAMBIUM_YANGLINT", filepath.Join(t.TempDir(), "no-such-yanglint"))
	wantErrContaining(t, RunCase(dir, c), "resolve yanglint")
}

// The differential lane compares verdicts: a flagged reject case needs both
// engines to refuse the document.
func TestRunDataTreeDifferentialCaseRejectCase(t *testing.T) {
	dir, c := writeVerdictCase(t, confmanifest.ExpectReject, `{"verdict:c":{"eq":"a"}}`)
	if err := RunDataTreeDifferentialCase(dir, c); err != nil {
		t.Fatalf("both engines reject: %v", err)
	}

	dir, c = writeVerdictCase(t, confmanifest.ExpectReject, `{"verdict:c":{"name":"n"}}`)
	wantErrContaining(t, RunDataTreeDifferentialCase(dir, c), "libyang: "+msgAcceptedReject)

	// libyang sees the default d = "x", so the must on ne fails; datatree's
	// must evaluation does not see defaults yet.
	dir, c = writeVerdictCase(t, confmanifest.ExpectReject, `{"verdict:c":{"name":"n","ne":"a"}}`)
	wantErrContaining(t, RunDataTreeDifferentialCase(dir, c), "datatree: "+msgAcceptedReject)
}

// An accept case in the differential lane must pass datatree validation, not
// just parse and serialize.
func TestRunDataTreeDifferentialCaseAcceptNeedsDataTreeValidation(t *testing.T) {
	dir, c := writeVerdictCase(t, confmanifest.ExpectAccept, `{"verdict:c":{"name":"n","eq":"a"}}`)
	wantErrContaining(t, RunDataTreeDifferentialCase(dir, c), "datatree: "+msgRejectedAccept)

	dir, c = writeVerdictCase(t, confmanifest.ExpectAccept, `{"verdict:c":{"name":"n"}}`)
	if err := RunDataTreeDifferentialCase(dir, c); err != nil {
		t.Fatalf("valid document: %v", err)
	}
}
