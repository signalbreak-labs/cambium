// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

package datatree_test

import (
	"strings"
	"testing"
)

const mwSchema = `module mw {
    namespace "urn:mw"; prefix mw;
    leaf kind { type string; }
    leaf speed {
        when "../kind = 'fast'";
        must ". <= 100";
        type uint16;
    }
}`

func TestMustWhenSatisfied(t *testing.T) {
	mod := loadModSrc(t, mwSchema, "mw")
	if err := validateOne(t, mod, `{"mw:kind":"fast","mw:speed":50}`); err != nil {
		t.Fatalf("when true + must true should be valid: %v", err)
	}
	// boundary: 100 <= 100
	if err := validateOne(t, mod, `{"mw:kind":"fast","mw:speed":100}`); err != nil {
		t.Fatalf("speed=100 should satisfy must '. <= 100': %v", err)
	}
}

func TestWhenViolation(t *testing.T) {
	mod := loadModSrc(t, mwSchema, "mw")
	err := validateOne(t, mod, `{"mw:kind":"slow","mw:speed":50}`)
	if err == nil || !strings.Contains(err.Error(), "when condition") {
		t.Fatalf("speed present but when false, expected when violation, got %v", err)
	}
}

func TestMustViolation(t *testing.T) {
	mod := loadModSrc(t, mwSchema, "mw")
	err := validateOne(t, mod, `{"mw:kind":"fast","mw:speed":200}`)
	if err == nil || !strings.Contains(err.Error(), "must condition") {
		t.Fatalf("speed=200 violates must '. <= 100', expected must violation, got %v", err)
	}
}

const mwCountSchema = `module mwc {
    namespace "urn:mwc"; prefix mwc;
    list srv { key id; leaf id { type string; } }
    leaf onlyone { type string; must "count(/mwc:srv) <= 2"; }
}`

func TestMustCountAbsolutePath(t *testing.T) {
	mod := loadModSrc(t, mwCountSchema, "mwc")
	if err := validateOne(t, mod, `{"mwc:srv":[{"id":"a"},{"id":"b"}],"mwc:onlyone":"x"}`); err != nil {
		t.Fatalf("2 servers satisfies count<=2: %v", err)
	}
	err := validateOne(t, mod, `{"mwc:srv":[{"id":"a"},{"id":"b"},{"id":"c"}],"mwc:onlyone":"x"}`)
	if err == nil || !strings.Contains(err.Error(), "must condition") {
		t.Fatalf("3 servers violates count<=2, expected must violation, got %v", err)
	}
}

// TestMustUnsupportedFunctionSkipped guards the safety rule: a must using a
// function the engine does not implement (re-match) is SKIPPED, not reported,
// even though the value would fail it.
func TestMustUnsupportedFunctionSkipped(t *testing.T) {
	mod := loadModSrc(t, `module mwu {
        namespace "urn:mwu"; prefix mwu;
        leaf x { must "re-match(., '[0-9]+')"; type string; }
    }`, "mwu")
	if err := validateOne(t, mod, `{"mwu:x":"abc"}`); err != nil {
		if strings.Contains(err.Error(), "must") {
			t.Fatalf("unsupported must function must be skipped, not reported: %v", err)
		}
	}
}
