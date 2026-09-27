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

// TestLeafRefUnprefixedPathInForeignGroupingUsesCurrentModule: RFC 7950
// section 6.4.1 puts unprefixed names in a leafref path in the module of the
// current node, which is where the grouping is used or the typedef referenced,
// not the module that wrote the path.
func TestLeafRefUnprefixedPathInForeignGroupingUsesCurrentModule(t *testing.T) {
	defs := `module lr-defs {
        namespace "urn:lr-defs"; prefix defs;
        typedef inst-ref {
            type leafref { path "/instances/instance/name"; }
        }
        grouping overlay {
            leaf absolute-ref { type leafref { path "/instances/instance/name"; } }
            leaf typed-ref { type inst-ref; }
            leaf parent-ref { type leafref { path "../../name"; } }
        }
    }`
	user := `module lr-user {
        namespace "urn:lr-user"; prefix user;
        import lr-defs { prefix defs; }
        container instances {
            list instance {
                key name;
                leaf name { type string; }
                container overlay { uses defs:overlay; }
            }
        }
    }`
	mod := loadMultiModSrc(t, "lr-user", defs, user)
	valid := `{"lr-user:instances":{"instance":[{"name":"blue","overlay":` +
		`{"absolute-ref":"blue","typed-ref":"blue","parent-ref":"blue"}}]}}`
	if err := validateOne(t, mod, valid); err != nil {
		t.Fatalf("leafrefs to existing instances should be valid: %v", err)
	}
	for _, leaf := range []string{"absolute-ref", "typed-ref", "parent-ref"} {
		in := `{"lr-user:instances":{"instance":[{"name":"blue","overlay":{"` + leaf + `":"red"}}]}}`
		err := validateOne(t, mod, in)
		if err == nil || !strings.Contains(err.Error(), "no matching instance") {
			t.Fatalf("%s=red has no instance, expected violation, got %v", leaf, err)
		}
	}
}
