// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

package datatree_test

import (
	"strings"
	"testing"
)

const lrAbsSchema = `module lr {
    namespace "urn:lr"; prefix lr;
    list user { key name; leaf name { type string; } }
    leaf admin { type leafref { path "/lr:user/lr:name"; } }
}`

func TestLeafRefAbsoluteInstance(t *testing.T) {
	mod := loadModSrc(t, lrAbsSchema, "lr")
	if err := validateOne(t, mod, `{"lr:user":[{"name":"alice"},{"name":"bob"}],"lr:admin":"alice"}`); err != nil {
		t.Fatalf("admin=alice exists, should be valid: %v", err)
	}
	err := validateOne(t, mod, `{"lr:user":[{"name":"alice"}],"lr:admin":"carol"}`)
	if err == nil || !strings.Contains(err.Error(), "no matching instance") {
		t.Fatalf("admin=carol absent, expected leafref violation, got %v", err)
	}
}

const lrRelSchema = `module lr2 {
    namespace "urn:lr2"; prefix lr2;
    container c {
        list user { key name; leaf name { type string; } }
        leaf admin { type leafref { path "../user/name"; } }
    }
}`

func TestLeafRefRelativeInstance(t *testing.T) {
	mod := loadModSrc(t, lrRelSchema, "lr2")
	if err := validateOne(t, mod, `{"lr2:c":{"user":[{"name":"x"}],"admin":"x"}}`); err != nil {
		t.Fatalf("relative leafref admin=x exists, should be valid: %v", err)
	}
	err := validateOne(t, mod, `{"lr2:c":{"user":[{"name":"x"}],"admin":"y"}}`)
	if err == nil || !strings.Contains(err.Error(), "no matching instance") {
		t.Fatalf("relative leafref admin=y absent, expected violation, got %v", err)
	}
}

// TestLeafRefPredicateSkipped guards the safety rule: a path with a predicate is
// unsupported, so the instance check is SKIPPED — never reported as a violation,
// even when the value would not match.
func TestLeafRefPredicateSkipped(t *testing.T) {
	mod := loadModSrc(t, `module lr3 {
        namespace "urn:lr3"; prefix lr3;
        list user { key name; leaf name { type string; } }
        leaf admin { type leafref { path "/lr3:user[lr3:name=current()]/lr3:name"; } }
    }`, "lr3")
	// admin=zzz does not exist, but the predicate path is unsupported -> skipped.
	if err := validateOne(t, mod, `{"lr3:user":[{"name":"a"}],"lr3:admin":"zzz"}`); err != nil {
		if strings.Contains(err.Error(), "leafref") {
			t.Fatalf("unsupported leafref path must be skipped, not reported: %v", err)
		}
	}
}
