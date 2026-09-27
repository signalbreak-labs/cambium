// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

package compat_test

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/signalbreak-labs/cambium/go/compat"
)

// Consumer-policy regressions for the compat bridge: deviation options must not
// change augment order or feature visibility, and ignoring deviate
// not-supported must take effect before schema validation.

const policyBase = `module base {
  yang-version 1.1; namespace "urn:base"; prefix b;
  container top { leaf own { type string; } }
}
`

const policyLeft = `module left {
  yang-version 1.1; namespace "urn:left"; prefix l;
  import base { prefix b; }
  augment "/b:top" { leaf z { type string; } }
}
`

const policyRight = `module right {
  yang-version 1.1; namespace "urn:right"; prefix r;
  import base { prefix b; }
  augment "/b:top" { leaf m { type string; } }
}
`

func policyModules(ignoreNotSupported bool) *compat.Modules {
	ms := compat.NewModules()
	ms.ParseOptions = compat.Options{
		StoreUses:                           true,
		IgnoreSubmoduleCircularDependencies: true,
		DeviateOptions: compat.DeviateOptions{
			IgnoreDeviateNotSupported: ignoreNotSupported,
		},
	}
	return ms
}

func parseAndProcess(t *testing.T, ms *compat.Modules, sources ...string) {
	t.Helper()
	for i, source := range sources {
		if err := ms.Parse(source, "policy-"+string(rune('a'+i))+".yang"); err != nil {
			t.Fatalf("Parse source %d: %v", i, err)
		}
	}
	if errs := ms.Process(); len(errs) != 0 {
		t.Fatalf("Process errors = %v, want none", errs)
	}
}

func TestCompatAugmentOrderIndependentOfDeviateOption(t *testing.T) {
	cases := []struct {
		name    string
		sources []string
		want    []string
	}{
		{"base-left-right", []string{policyBase, policyLeft, policyRight}, []string{"own", "z", "m"}},
		{"base-right-left", []string{policyBase, policyRight, policyLeft}, []string{"own", "m", "z"}},
	}
	for _, tc := range cases {
		for _, ignore := range []bool{false, true} {
			// Map iteration order is randomized per range statement, so repeated
			// fresh module sets exercise the nondeterministic path.
			for i := 0; i < 40; i++ {
				ms := policyModules(ignore)
				parseAndProcess(t, ms, tc.sources...)
				top := compat.ToEntry(ms.Modules["base"]).Dir["top"]
				if top == nil {
					t.Fatalf("%s ignore=%v: top missing", tc.name, ignore)
				}
				if got := childNames(top.Children()); !reflect.DeepEqual(got, tc.want) {
					t.Fatalf("%s ignore=%v run %d: top children = %v, want %v", tc.name, ignore, i, got, tc.want)
				}
			}
		}
	}
}

func TestCompatAugmentOrderFileLoadsIndependentOfDeviateOption(t *testing.T) {
	dir := t.TempDir()
	for name, source := range map[string]string{"base.yang": policyBase, "left.yang": policyLeft, "right.yang": policyRight} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(source), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for _, ignore := range []bool{false, true} {
		for i := 0; i < 20; i++ {
			ms := policyModules(ignore)
			ms.AddPath(dir)
			for _, name := range []string{"left", "right"} {
				if err := ms.Read(name); err != nil {
					t.Fatalf("Read %s: %v", name, err)
				}
			}
			if errs := ms.Process(); len(errs) != 0 {
				t.Fatalf("Process: %v", errs)
			}
			entry, errs := ms.GetModule("base")
			if len(errs) != 0 {
				t.Fatalf("GetModule: %v", errs)
			}
			if got, want := childNames(entry.Dir["top"].Children()), []string{"own", "z", "m"}; !reflect.DeepEqual(got, want) {
				t.Fatalf("ignore=%v run %d: children = %v, want %v", ignore, i, got, want)
			}
		}
	}
}

func TestCompatFeatureVisibilityIndependentOfDeviateOption(t *testing.T) {
	const source = `module f {
  yang-version 1.1; namespace "urn:f"; prefix f;
  feature opt;
  feature other;
  container top {
    leaf a { type string; }
    leaf b { if-feature opt; type string; }
    leaf c { if-feature "not opt"; type string; }
    leaf d { if-feature "opt or other"; type string; }
    leaf e { if-feature "opt and other"; type string; }
  }
}
`
	var results [][]string
	for _, ignore := range []bool{false, true} {
		ms := policyModules(ignore)
		parseAndProcess(t, ms, source)
		entry, errs := ms.GetModule("f")
		if len(errs) != 0 {
			t.Fatalf("GetModule: %v", errs)
		}
		results = append(results, childNames(entry.Dir["top"].Children()))
	}
	// compat projects Cambium's effective schema with no features enabled,
	// the native default, whatever the deviation option.
	want := []string{"a", "c"}
	for i, got := range results {
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("ignore=%v: children = %v, want %v", i == 1, got, want)
		}
	}
}

const policyIgnoredTarget = `module d {
  yang-version 1.1; namespace "urn:d"; prefix d;
  leaf target { type string; }
  leaf ref { type leafref { path "/d:target"; } }
  deviation "/d:target" { deviate not-supported; }
}
`

func TestCompatIgnoredNotSupportedKeepsReferenceValid(t *testing.T) {
	ms := policyModules(true)
	parseAndProcess(t, ms, policyIgnoredTarget)
	entry, errs := ms.GetModule("d")
	if len(errs) != 0 {
		t.Fatalf("GetModule: %v", errs)
	}
	if got, want := childNames(entry.Children()), []string{"target", "ref"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("children = %v, want %v", got, want)
	}
}

func TestCompatAppliedNotSupportedStillRejectsDanglingReference(t *testing.T) {
	ms := policyModules(false)
	if err := ms.Parse(policyIgnoredTarget, "d.yang"); err != nil {
		t.Fatalf("Parse: %v", err)
	}
	errs := ms.Process()
	if len(errs) == 0 {
		t.Fatal("Process succeeded, want dangling leafref error")
	}
	if !strings.Contains(errs[0].Error(), "target not found") {
		t.Fatalf("Process error = %v, want leafref target not found", errs[0])
	}
}

func TestCompatIgnoredNotSupportedFromImportedDeviationModule(t *testing.T) {
	const target = `module tgt {
  yang-version 1.1; namespace "urn:tgt"; prefix t;
  container top {
    leaf before { type string; }
    list entry {
      key name;
      leaf name { type string; }
      leaf value { type string; default "x"; }
    }
    leaf ref { type leafref { path "../entry/name"; } }
    leaf after { type string; }
  }
}
`
	const deviator = `module dev {
  yang-version 1.1; namespace "urn:dev"; prefix dv;
  import tgt { prefix t; }
  deviation "/t:top/t:entry" { deviate not-supported; }
  deviation "/t:top/t:entry/t:value" { deviate replace { default "y"; } }
}
`
	for _, ignore := range []bool{false, true} {
		ms := policyModules(ignore)
		for i, source := range []string{target, deviator} {
			if err := ms.Parse(source, []string{"tgt.yang", "dev.yang"}[i]); err != nil {
				t.Fatalf("Parse: %v", err)
			}
		}
		errs := ms.Process()
		if !ignore {
			if len(errs) == 0 {
				t.Fatal("applied not-supported: Process succeeded, want dangling leafref error")
			}
			continue
		}
		if len(errs) != 0 {
			t.Fatalf("ignored not-supported: Process errors = %v", errs)
		}
		entry, errs := ms.GetModule("tgt")
		if len(errs) != 0 {
			t.Fatalf("GetModule: %v", errs)
		}
		top := entry.Dir["top"]
		if got, want := childNames(top.Children()), []string{"before", "entry", "ref", "after"}; !reflect.DeepEqual(got, want) {
			t.Fatalf("children = %v, want %v", got, want)
		}
		list := top.Dir["entry"]
		if got := list.Key; got != "name" {
			t.Fatalf("list key = %q, want name", got)
		}
		// Other deviate operations still apply.
		if got := list.Dir["value"].Default; !reflect.DeepEqual(got, []string{"y"}) {
			t.Fatalf("value default = %v, want [y]", got)
		}
	}
}

// IgnoreSubmoduleCircularDependencies keeps goyang's contract: a circular
// submodule include is an error by default (RFC 7950 forbids it) and loads
// only when the flag is set, which selects Cambium's vendor-compatible
// loading and reports the cycle as a LoadReport warning.
func TestCompatSubmoduleCircularDependencyFlag(t *testing.T) {
	dir := t.TempDir()
	files := map[string]string{
		"circ.yang": `module circ {
  yang-version 1.1; namespace "urn:circ"; prefix c;
  include circ-a; include circ-b;
  container top { uses from-a; uses from-b; }
}`,
		"circ-a.yang": `submodule circ-a {
  yang-version 1.1; belongs-to circ { prefix c; }
  include circ-b;
  grouping from-a { leaf a { type string; } }
}`,
		"circ-b.yang": `submodule circ-b {
  yang-version 1.1; belongs-to circ { prefix c; }
  include circ-a;
  grouping from-b { leaf b { type string; } }
}`,
	}
	for name, source := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(source), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	process := func(flag bool) (*compat.Modules, []error) {
		ms := compat.NewModules()
		ms.ParseOptions.IgnoreSubmoduleCircularDependencies = flag
		ms.AddPath(dir)
		if err := ms.Read("circ"); err != nil {
			t.Fatalf("Read: %v", err)
		}
		errs := ms.Process()
		return ms, errs
	}

	if _, errs := process(false); len(errs) == 0 || !strings.Contains(errs[0].Error(), "include cycle circ-a -> circ-b -> circ-a") {
		t.Fatalf("Process without flag errors = %v, want include cycle error", errs)
	}

	ms, errs := process(true)
	if len(errs) != 0 {
		t.Fatalf("Process with flag errors = %v, want none", errs)
	}
	entry, errs := ms.GetModule("circ")
	if len(errs) != 0 {
		t.Fatalf("GetModule: %v", errs)
	}
	// Circular submodule includes load in uses order.
	if got := strings.Join(childNames(entry.Dir["top"].Children()), ","); got != "a,b" {
		t.Fatalf("circular submodule include children = %q, want %q", got, "a,b")
	}
	var cycleWarnings int
	for _, warning := range ms.LoadReport().Warnings {
		if strings.Contains(warning.Message, "include cycle circ-a -> circ-b -> circ-a") {
			cycleWarnings++
		}
	}
	if cycleWarnings != 1 {
		t.Fatalf("include cycle warnings = %d, want 1 in %+v", cycleWarnings, ms.LoadReport().Warnings)
	}
}
