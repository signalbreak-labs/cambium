// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

package cambium_test

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/signalbreak-labs/cambium/go/cambium"
)

func writeModuleFile(t *testing.T, path string, content []byte) {
	t.Helper()
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Remove(path) })
}

func schemaIntrospectionModuleDir(t *testing.T) string {
	t.Helper()
	base := filepath.Join("..", "..", "target", "tests", "schema-introspection", "modules")
	if err := os.MkdirAll(base, 0o755); err != nil {
		t.Fatal(err)
	}
	prefix := strings.NewReplacer("/", "_", " ", "_").Replace(t.Name()) + "-"
	dir, err := os.MkdirTemp(base, prefix)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return dir
}

func TestScopedTypedefAndGroupingResolution(t *testing.T) {
	source := `module cambium-scoped-definitions {
    namespace "urn:cambium:scoped-definitions";
    prefix csd;

    container alpha {
        typedef local-value {
            type uint8;
        }
        grouping local-group {
            leaf alpha-grouped {
                type string;
            }
        }
        leaf value {
            type local-value;
        }
        uses local-group;
    }

    container beta {
        typedef local-value {
            type string;
        }
        grouping local-group {
            leaf beta-grouped {
                type string;
            }
        }
        leaf value {
            type local-value;
        }
        uses local-group;
    }
}`
	builder, err := cambium.NewContextBuilder(cambium.ContextFlags{})
	if err != nil {
		t.Fatal(err)
	}
	if err := builder.LoadModuleStr(source); err != nil {
		t.Fatalf("LoadModuleStr: %v", err)
	}
	ctx, err := builder.Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	t.Cleanup(func() { ctx.Close() })
	mod, err := ctx.Schema("cambium-scoped-definitions")
	if err != nil {
		t.Fatalf("Schema: %v", err)
	}

	alpha := schemaNodeAt(t, mod, "/csd:alpha/value")
	alphaInfo, ok := alpha.LeafType()
	if !ok {
		t.Fatal("alpha/value should be a leaf")
	}
	if got, want := alphaInfo.Base(), cambium.BaseTypeUint8; got != want {
		t.Fatalf("alpha/value base = %s, want %s", got, want)
	}
	beta := schemaNodeAt(t, mod, "/csd:beta/value")
	betaInfo, ok := beta.LeafType()
	if !ok {
		t.Fatal("beta/value should be a leaf")
	}
	if got, want := betaInfo.Base(), cambium.BaseTypeString; got != want {
		t.Fatalf("beta/value base = %s, want %s", got, want)
	}

	alphaNode := schemaNodeAt(t, mod, "/csd:alpha")
	if got := strings.Join(schemaChildNames(alphaNode.Children()), ","); got != "value,alpha-grouped" {
		t.Fatalf("alpha children = %s, want value,alpha-grouped", got)
	}
	betaNode := schemaNodeAt(t, mod, "/csd:beta")
	if got := strings.Join(schemaChildNames(betaNode.Children()), ","); got != "value,beta-grouped" {
		t.Fatalf("beta children = %s, want value,beta-grouped", got)
	}
}

func TestUsesAugmentAppliesToUseInstanceOnly(t *testing.T) {
	source := `module cambium-uses-augment {
    namespace "urn:cambium:uses-augment";
    prefix cua;

    grouping reusable {
        container settings {
            leaf name { type string; }
        }
    }

    container augmented {
        uses reusable {
            augment settings {
                leaf enabled { type boolean; }
            }
        }
    }

    container plain {
        uses reusable;
    }
}`
	builder, err := cambium.NewContextBuilder(cambium.ContextFlags{})
	if err != nil {
		t.Fatal(err)
	}
	if err := builder.LoadModuleStr(source); err != nil {
		t.Fatalf("LoadModuleStr: %v", err)
	}
	ctx, err := builder.Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	t.Cleanup(func() { ctx.Close() })
	mod, err := ctx.Schema("cambium-uses-augment")
	if err != nil {
		t.Fatalf("Schema: %v", err)
	}

	augmentedSettings := schemaNodeAt(t, mod, "/cua:augmented/settings")
	if got := strings.Join(schemaChildNames(augmentedSettings.Children()), ","); got != "name,enabled" {
		t.Fatalf("augmented settings children = %s, want name,enabled", got)
	}
	enabled := schemaNodeAt(t, mod, "/cua:augmented/settings/enabled")
	info, ok := enabled.LeafType()
	if !ok {
		t.Fatal("augmented enabled should be a leaf")
	}
	if got, want := info.Base(), cambium.BaseTypeBoolean; got != want {
		t.Fatalf("enabled base = %s, want %s", got, want)
	}

	plainSettings := schemaNodeAt(t, mod, "/cua:plain/settings")
	if got := strings.Join(schemaChildNames(plainSettings.Children()), ","); got != "name" {
		t.Fatalf("plain settings children = %s, want name", got)
	}
	if _, err := mod.FindPath("/cua:plain/settings/enabled"); err == nil {
		t.Fatal("plain use unexpectedly contains uses-augment leaf")
	}
}

func TestUsesRefineResolvesPrefixedTarget(t *testing.T) {
	source := `module cambium-uses-refine-prefixed {
    namespace "urn:cambium:uses-refine-prefixed";
    prefix curp;

    grouping reusable {
        leaf value { type string; }
    }

    container top {
        uses reusable {
            refine curp:value {
                mandatory true;
            }
        }
    }
}`
	builder, err := cambium.NewContextBuilder(cambium.ContextFlags{})
	if err != nil {
		t.Fatal(err)
	}
	if err := builder.LoadModuleStr(source); err != nil {
		t.Fatalf("LoadModuleStr: %v", err)
	}
	ctx, err := builder.Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	t.Cleanup(func() { ctx.Close() })

	mod, err := ctx.Schema("cambium-uses-refine-prefixed")
	if err != nil {
		t.Fatalf("Schema: %v", err)
	}
	value := schemaNodeAt(t, mod, "/curp:top/value")
	if !value.IsMandatory() {
		t.Fatal("prefixed refine target did not apply mandatory true")
	}
}

func TestUsesWhenAppliesToExpandedNodes(t *testing.T) {
	source := `module cambium-uses-when {
    namespace "urn:cambium:uses-when";
    prefix cuw;

    grouping reusable {
        container settings {
            leaf name { type string; }
        }
        leaf flag { type boolean; }
    }

    container conditional {
        leaf enabled { type boolean; }
        uses reusable {
            when "../enabled = 'true'" {
                description "Only present when enabled";
                reference "uses when";
            }
        }
    }
}`
	builder, err := cambium.NewContextBuilder(cambium.ContextFlags{})
	if err != nil {
		t.Fatal(err)
	}
	if err := builder.LoadModuleStr(source); err != nil {
		t.Fatalf("LoadModuleStr: %v", err)
	}
	ctx, err := builder.Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	t.Cleanup(func() { ctx.Close() })

	mod, err := ctx.Schema("cambium-uses-when")
	if err != nil {
		t.Fatalf("Schema: %v", err)
	}
	conditional := schemaNodeAt(t, mod, "/cuw:conditional")
	for _, childName := range []string{"settings", "flag"} {
		child := childByName(t, conditional.Children(), childName)
		whens := child.Whens()
		if len(whens) != 1 {
			t.Fatalf("%s whens = %d, want 1", childName, len(whens))
		}
		if got, want := whens[0].Expression(), "../enabled = 'true'"; got != want {
			t.Fatalf("%s when = %q, want %q", childName, got, want)
		}
		if got, ok := whens[0].Description(); !ok || got != "Only present when enabled" {
			t.Fatalf("%s when description = (%q, %v), want (Only present when enabled, true)", childName, got, ok)
		}
		if got, ok := whens[0].Reference(); !ok || got != "uses when" {
			t.Fatalf("%s when reference = (%q, %v), want (uses when, true)", childName, got, ok)
		}
	}
}

func TestDuplicateDefinitionsReturnContextRuleCode(t *testing.T) {
	cases := []struct {
		name    string
		source  string
		message string
	}{
		{
			name: "typedef",
			source: `module cambium-duplicate-typedef {
    namespace "urn:cambium:duplicate-typedef";
    prefix cdt;
    typedef dup { type string; }
    typedef dup { type uint8; }
}`,
			message: `duplicate typedef "dup"`,
		},
		{
			name: "grouping",
			source: `module cambium-duplicate-grouping {
    namespace "urn:cambium:duplicate-grouping";
    prefix cdg;
    grouping dup { leaf a { type string; } }
    grouping dup { leaf b { type string; } }
}`,
			message: `duplicate grouping "dup"`,
		},
		{
			name: "identity",
			source: `module cambium-duplicate-identity {
    namespace "urn:cambium:duplicate-identity";
    prefix cdi;
    identity dup;
    identity dup;
}`,
			message: `duplicate identity "dup"`,
		},
		{
			name: "feature",
			source: `module cambium-duplicate-feature {
    namespace "urn:cambium:duplicate-feature";
    prefix cdf;
    feature dup;
    feature dup;
}`,
			message: `duplicate feature "dup"`,
		},
		{
			name: "extension",
			source: `module cambium-duplicate-extension {
    namespace "urn:cambium:duplicate-extension";
    prefix cde;
    extension dup;
    extension dup;
}`,
			message: `duplicate extension "dup"`,
		},
		{
			name: "scoped typedef collision",
			source: `module cambium-scoped-typedef-collision {
    namespace "urn:cambium:scoped-typedef-collision";
    prefix cstc;
    typedef dup { type string; }
    container top {
        typedef dup { type uint8; }
        leaf value { type dup; }
    }
}`,
			message: `duplicate typedef "dup"`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			builder, err := cambium.NewContextBuilder(cambium.ContextFlags{})
			if err != nil {
				t.Fatal(err)
			}
			if err := builder.LoadModuleStr(tc.source); err != nil {
				t.Fatalf("LoadModuleStr: %v", err)
			}
			_, err = builder.Build()
			if err == nil {
				t.Fatal("Build accepted duplicate definition")
			}
			var ce *cambium.Error
			if !errors.As(err, &ce) || ce.RuleCode() != cambium.RuleCodeContext {
				t.Fatalf("Build error = %v, want RuleCodeContext", err)
			}
			if !strings.Contains(err.Error(), tc.message) {
				t.Fatalf("Build error = %q, want to contain %q", err.Error(), tc.message)
			}
		})
	}
}

func TestInvalidScopedDefinitionPlacementReturnsContextRuleCode(t *testing.T) {
	cases := []struct {
		name    string
		source  string
		message string
	}{
		{
			name: "typedef under leaf",
			source: `module cambium-typedef-under-leaf {
    namespace "urn:cambium:typedef-under-leaf";
    prefix ctul;

    leaf value {
        typedef local {
            type string;
        }
        type string;
    }
}`,
			message: `typedef "local" is not valid under leaf "value"`,
		},
		{
			name: "grouping under leaf-list",
			source: `module cambium-grouping-under-leaf-list {
    namespace "urn:cambium:grouping-under-leaf-list";
    prefix cgull;

    leaf-list value {
        grouping local {
            leaf nested {
                type string;
            }
        }
        type string;
    }
}`,
			message: `grouping "local" is not valid under leaf-list "value"`,
		},
		{
			name: "typedef under type",
			source: `module cambium-typedef-under-type {
    namespace "urn:cambium:typedef-under-type";
    prefix ctut;

    leaf value {
        type string {
            typedef local {
                type string;
            }
        }
    }
}`,
			message: `typedef "local" is not valid under type "string"`,
		},
		{
			name: "grouping under choice",
			source: `module cambium-grouping-under-choice {
    namespace "urn:cambium:grouping-under-choice";
    prefix cguc;

    choice selector {
        grouping local {
            leaf nested {
                type string;
            }
        }
        leaf value {
            type string;
        }
    }
}`,
			message: `grouping "local" is not valid under choice "selector"`,
		},
		{
			name: "typedef under augment",
			source: `module cambium-typedef-under-augment {
    namespace "urn:cambium:typedef-under-augment";
    prefix ctua;

    container target;
    augment "/ctua:target" {
        typedef local {
            type string;
        }
        leaf added {
            type string;
        }
    }
}`,
			message: `typedef "local" is not valid under augment "/ctua:target"`,
		},
		{
			name: "augment under container",
			source: `module cambium-augment-under-container {
    namespace "urn:cambium:augment-under-container";
    prefix cauc;

    container top {
        augment "/cauc:top" {
            leaf added {
                type string;
            }
        }
    }
}`,
			message: `augment "/cauc:top" is not valid under container "top"`,
		},
		{
			name: "augment invalid child",
			source: `module cambium-augment-invalid-child {
    namespace "urn:cambium:augment-invalid-child";
    prefix caic;

    container target;
    augment "/caic:target" {
        default "bad";
        leaf added {
            type string;
        }
    }
}`,
			message: `default "bad" is not valid under augment "/caic:target"`,
		},
		{
			name: "refine under container",
			source: `module cambium-refine-under-container {
    namespace "urn:cambium:refine-under-container";
    prefix cruc;

    container top {
        refine value {
            mandatory true;
        }
        leaf value {
            type string;
        }
    }
}`,
			message: `refine "value" is not valid under container "top"`,
		},
		{
			name: "refine invalid child",
			source: `module cambium-refine-invalid-child {
    namespace "urn:cambium:refine-invalid-child";
    prefix cric;

    grouping shared {
        leaf value {
            type string;
        }
    }

    container top {
        uses shared {
            refine value {
                type string;
            }
        }
    }
}`,
			message: `type "string" is not valid under refine "value"`,
		},
		{
			name: "refine invalid metadata child",
			source: `module cambium-refine-invalid-metadata-child {
    namespace "urn:cambium:refine-invalid-metadata-child";
    prefix crimc;

    grouping shared {
        leaf value {
            type string;
        }
    }

    container top {
        uses shared {
            refine value {
                status deprecated;
            }
        }
    }
}`,
			message: `status "deprecated" is not valid under refine "value"`,
		},
		{
			name: "refine if-feature invalid sibling",
			source: `module cambium-refine-if-feature-invalid-sibling {
    namespace "urn:cambium:refine-if-feature-invalid-sibling";
    prefix crifis;
    feature advanced;

    grouping shared {
        leaf value {
            type string;
        }
    }

    container top {
        uses shared {
            refine value {
                if-feature advanced;
                type string;
            }
        }
    }
}`,
			message: `type "string" is not valid under refine "value"`,
		},
		{
			name: "uses invalid child",
			source: `module cambium-uses-invalid-child {
    namespace "urn:cambium:uses-invalid-child";
    prefix cuic;

    grouping shared {
        leaf value {
            type string;
        }
    }

    container top {
        uses shared {
            default "bad";
        }
    }
}`,
			message: `default "bad" is not valid under uses "shared"`,
		},
		{
			name: "uses augment invalid child",
			source: `module cambium-uses-augment-invalid-child {
    namespace "urn:cambium:uses-augment-invalid-child";
    prefix cuaic;

    grouping shared {
        container settings {
            leaf name {
                type string;
            }
        }
    }

    container top {
        uses shared {
            augment settings {
                default "bad";
                leaf enabled {
                    type boolean;
                }
            }
        }
    }
}`,
			message: `default "bad" is not valid under augment "settings"`,
		},
		{
			name: "uses if-feature invalid sibling",
			source: `module cambium-uses-if-feature-invalid-sibling {
    namespace "urn:cambium:uses-if-feature-invalid-sibling";
    prefix cuifis;
    feature advanced;

    grouping shared {
        leaf value {
            type string;
        }
    }

    container top {
        uses shared {
            if-feature advanced;
            default "bad";
        }
    }
}`,
			message: `default "bad" is not valid under uses "shared"`,
		},
		{
			name: "deviate under container",
			source: `module cambium-deviate-under-container {
    namespace "urn:cambium:deviate-under-container";
    prefix cduc;

    container top {
        deviate add {
            config false;
        }
        leaf value {
            type string;
        }
    }
}`,
			message: `deviate "add" is not valid under container "top"`,
		},
		{
			name: "revision-date under container",
			source: `module cambium-revision-date-under-container {
    namespace "urn:cambium:revision-date-under-container";
    prefix crduc;

    container top {
        revision-date 2026-06-18;
        leaf value {
            type string;
        }
    }
}`,
			message: `revision-date "2026-06-18" is not valid under container "top"`,
		},
		{
			name: "argument under container",
			source: `module cambium-argument-under-container {
    namespace "urn:cambium:argument-under-container";
    prefix cauc;

    container top {
        argument value;
        leaf value {
            type string;
        }
    }
}`,
			message: `argument "value" is not valid under container "top"`,
		},
		{
			name: "yin-element under extension",
			source: `module cambium-yin-element-under-extension {
    namespace "urn:cambium:yin-element-under-extension";
    prefix cyeue;

    extension marker {
        yin-element true;
    }

    leaf value {
        type string;
    }
}`,
			message: `yin-element "true" is not valid under extension "marker"`,
		},
		{
			name: "value under enumeration type",
			source: `module cambium-value-under-enumeration-type {
    namespace "urn:cambium:value-under-enumeration-type";
    prefix cvuet;

    leaf value {
        type enumeration {
            value 1;
            enum one;
        }
    }
}`,
			message: `value "1" is not valid under type "enumeration"`,
		},
		{
			name: "position under bits type",
			source: `module cambium-position-under-bits-type {
    namespace "urn:cambium:position-under-bits-type";
    prefix cpubt;

    leaf value {
        type bits {
            position 1;
            bit one;
        }
    }
}`,
			message: `position "1" is not valid under type "bits"`,
		},
		{
			name: "error-message under container",
			source: `module cambium-error-message-under-container {
    namespace "urn:cambium:error-message-under-container";
    prefix cemuc;

    container top {
        error-message "bad";
        leaf value {
            type string;
        }
    }
}`,
			message: `error-message "bad" is not valid under container "top"`,
		},
		{
			name: "modifier under type",
			source: `module cambium-modifier-under-type {
    namespace "urn:cambium:modifier-under-type";
    prefix cmut;

    leaf value {
        type string {
            modifier invert-match;
        }
    }
}`,
			message: `modifier "invert-match" is not valid under type "string"`,
		},
		{
			name: "prefix under container",
			source: `module cambium-prefix-under-container {
    namespace "urn:cambium:prefix-under-container";
    prefix cpuc;

    container top {
        prefix nested;
        leaf value {
            type string;
        }
    }
}`,
			message: `prefix "nested" is not valid under container "top"`,
		},
		{
			name: "base under container",
			source: `module cambium-base-under-container {
    namespace "urn:cambium:base-under-container";
    prefix cbuc;

    identity root;

    container top {
        base root;
        leaf value {
            type string;
        }
    }
}`,
			message: `base "root" is not valid under container "top"`,
		},
		{
			name: "path under leaf",
			source: `module cambium-path-under-leaf {
    namespace "urn:cambium:path-under-leaf";
    prefix cpul;

    leaf value {
        type string;
        path "../target";
    }
}`,
			message: `path "../target" is not valid under leaf "value"`,
		},
		{
			name: "range under container",
			source: `module cambium-range-under-container {
    namespace "urn:cambium:range-under-container";
    prefix cruc;

    container top {
        range "1..10";
        leaf value {
            type uint8;
        }
    }
}`,
			message: `range "1..10" is not valid under container "top"`,
		},
		{
			name: "description under type",
			source: `module cambium-description-under-type {
    namespace "urn:cambium:description-under-type";
    prefix cdut;

    leaf value {
        type string {
            description "bad";
        }
    }
}`,
			message: `description "bad" is not valid under type "string"`,
		},
		{
			name: "status under type",
			source: `module cambium-status-under-type {
    namespace "urn:cambium:status-under-type";
    prefix csut;

    leaf value {
        type string {
            status deprecated;
        }
    }
}`,
			message: `status "deprecated" is not valid under type "string"`,
		},
		{
			name: "module under container",
			source: `module cambium-module-under-container {
    namespace "urn:cambium:module-under-container";
    prefix cmuc;

    container top {
        module nested;
        leaf value {
            type string;
        }
    }
}`,
			message: `module "nested" is not valid under container "top"`,
		},
		{
			name: "submodule under container",
			source: `module cambium-submodule-under-container {
    namespace "urn:cambium:submodule-under-container";
    prefix csuc;

    container top {
        submodule nested;
        leaf value {
            type string;
        }
    }
}`,
			message: `submodule "nested" is not valid under container "top"`,
		},
		{
			name: "type under grouping",
			source: `module cambium-type-under-grouping {
    namespace "urn:cambium:type-under-grouping";
    prefix ctug;

    grouping shared {
        type string;
        leaf value {
            type string;
        }
    }
}`,
			message: `type "string" is not valid under grouping "shared"`,
		},
		{
			name: "grouping invalid child",
			source: `module cambium-grouping-invalid-child {
    namespace "urn:cambium:grouping-invalid-child";
    prefix cgic;

    grouping shared {
        default "bad";
        leaf value {
            type string;
        }
    }
}`,
			message: `default "bad" is not valid under grouping "shared"`,
		},
		{
			name: "type under pattern",
			source: `module cambium-type-under-pattern {
    namespace "urn:cambium:type-under-pattern";
    prefix ctup;

    leaf value {
        type string {
            pattern "[a-z]+" {
                type uint8;
            }
        }
    }
}`,
			message: `type "uint8" is not valid under pattern "[a-z]+"`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			builder, err := cambium.NewContextBuilder(cambium.ContextFlags{})
			if err != nil {
				t.Fatal(err)
			}
			if err := builder.LoadModuleStr(tc.source); err != nil {
				t.Fatalf("LoadModuleStr: %v", err)
			}
			_, err = builder.Build()
			if err == nil {
				t.Fatal("Build accepted invalid scoped definition placement")
			}
			var ce *cambium.Error
			if !errors.As(err, &ce) || ce.RuleCode() != cambium.RuleCodeContext {
				t.Fatalf("Build error = %v, want RuleCodeContext", err)
			}
			if !strings.Contains(err.Error(), tc.message) {
				t.Fatalf("Build error = %q, want to contain %q", err.Error(), tc.message)
			}
		})
	}
}

func TestInvalidTopLevelDefinitionPlacementReturnsContextRuleCode(t *testing.T) {
	cases := []struct {
		name    string
		source  string
		message string
	}{
		{
			name: "identity under container",
			source: `module cambium-identity-under-container {
    namespace "urn:cambium:identity-under-container";
    prefix ciuc;

    container top {
        identity local;
    }
}`,
			message: `identity "local" is not valid under container "top"`,
		},
		{
			name: "feature under grouping",
			source: `module cambium-feature-under-grouping {
    namespace "urn:cambium:feature-under-grouping";
    prefix cfug;

    grouping shared {
        feature local;
        leaf value {
            type string;
        }
    }
}`,
			message: `feature "local" is not valid under grouping "shared"`,
		},
		{
			name: "extension under list",
			source: `module cambium-extension-under-list {
    namespace "urn:cambium:extension-under-list";
    prefix ceul;

    list item {
        key "name";
        leaf name {
            type string;
        }
        extension local;
    }
}`,
			message: `extension "local" is not valid under list "item"`,
		},
		{
			name: "identity invalid child",
			source: `module cambium-identity-invalid-child {
    namespace "urn:cambium:identity-invalid-child";
    prefix ciic;

    identity root {
        default "bad";
    }
}`,
			message: `default "bad" is not valid under identity "root"`,
		},
		{
			name: "feature invalid child",
			source: `module cambium-feature-invalid-child {
    namespace "urn:cambium:feature-invalid-child";
    prefix cfic;

    feature enabled {
        default "bad";
    }
}`,
			message: `default "bad" is not valid under feature "enabled"`,
		},
		{
			name: "extension invalid child",
			source: `module cambium-extension-invalid-child {
    namespace "urn:cambium:extension-invalid-child";
    prefix ceic;

    extension marker {
        default "bad";
    }
}`,
			message: `default "bad" is not valid under extension "marker"`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			builder, err := cambium.NewContextBuilder(cambium.ContextFlags{})
			if err != nil {
				t.Fatal(err)
			}
			if err := builder.LoadModuleStr(tc.source); err != nil {
				t.Fatalf("LoadModuleStr: %v", err)
			}
			_, err = builder.Build()
			if err == nil {
				t.Fatal("Build accepted invalid top-level definition placement")
			}
			var ce *cambium.Error
			if !errors.As(err, &ce) || ce.RuleCode() != cambium.RuleCodeContext {
				t.Fatalf("Build error = %v, want RuleCodeContext", err)
			}
			if !strings.Contains(err.Error(), tc.message) {
				t.Fatalf("Build error = %q, want to contain %q", err.Error(), tc.message)
			}
		})
	}
}

func TestInvalidModuleBodyStatementPlacementReturnsContextRuleCode(t *testing.T) {
	cases := []struct {
		name    string
		source  string
		message string
	}{
		{
			name: "import under container",
			source: `module cambium-import-under-container {
    namespace "urn:cambium:import-under-container";
    prefix ciuc;

    container top {
        import external-target {
            prefix ext;
        }
    }
}`,
			message: `import "external-target" is not valid under container "top"`,
		},
		{
			name: "revision under grouping",
			source: `module cambium-revision-under-grouping {
    namespace "urn:cambium:revision-under-grouping";
    prefix crug;

    grouping shared {
        revision 2026-06-18;
        leaf value {
            type string;
        }
    }
}`,
			message: `revision "2026-06-18" is not valid under grouping "shared"`,
		},
		{
			name: "deviation under list",
			source: `module cambium-deviation-under-list {
    namespace "urn:cambium:deviation-under-list";
    prefix cdul;

    list item {
        key "name";
        leaf name {
            type string;
        }
        deviation "/cdul:item/cdul:name" {
            deviate not-supported;
        }
    }
}`,
			message: `deviation "/cdul:item/cdul:name" is not valid under list "item"`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			builder, err := cambium.NewContextBuilder(cambium.ContextFlags{})
			if err != nil {
				t.Fatal(err)
			}
			if err := builder.LoadModuleStr(tc.source); err != nil {
				t.Fatalf("LoadModuleStr: %v", err)
			}
			_, err = builder.Build()
			if err == nil {
				t.Fatal("Build accepted invalid module-body statement placement")
			}
			var ce *cambium.Error
			if !errors.As(err, &ce) || ce.RuleCode() != cambium.RuleCodeContext {
				t.Fatalf("Build error = %v, want RuleCodeContext", err)
			}
			if !strings.Contains(err.Error(), tc.message) {
				t.Fatalf("Build error = %q, want to contain %q", err.Error(), tc.message)
			}
		})
	}
}

func TestUnknownNestedStatementsReturnContextRuleCode(t *testing.T) {
	cases := []struct {
		name    string
		source  string
		message string
	}{
		{
			name: "unknown under container",
			source: `module cambium-unknown-under-container {
    namespace "urn:cambium:unknown-under-container";
    prefix cuuc;
    container top {
        mystery "x";
        leaf value { type string; }
    }
}`,
			message: `unknown statement "mystery"`,
		},
		{
			name: "unknown under type",
			source: `module cambium-unknown-under-type {
    namespace "urn:cambium:unknown-under-type";
    prefix cuut;
    leaf value {
        type string {
            mystery "x";
        }
    }
}`,
			message: `unknown statement "mystery"`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			builder, err := cambium.NewContextBuilder(cambium.ContextFlags{})
			if err != nil {
				t.Fatal(err)
			}
			if err := builder.LoadModuleStr(tc.source); err != nil {
				t.Fatalf("LoadModuleStr: %v", err)
			}
			_, err = builder.Build()
			if err == nil {
				t.Fatal("Build accepted unknown nested statement")
			}
			var ce *cambium.Error
			if !errors.As(err, &ce) || ce.RuleCode() != cambium.RuleCodeContext {
				t.Fatalf("Build error = %v, want RuleCodeContext", err)
			}
			if !strings.Contains(err.Error(), tc.message) {
				t.Fatalf("Build error = %q, want to contain %q", err.Error(), tc.message)
			}
		})
	}
}

func TestInvalidStatementArgumentCardinalityReturnsContextRuleCode(t *testing.T) {
	cases := []struct {
		name    string
		source  string
		message string
	}{
		{
			name: "leaf missing identifier argument",
			source: `module cambium-leaf-missing-argument {
    namespace "urn:cambium:leaf-missing-argument";
    prefix clma;
    leaf;
}`,
			message: `leaf statement requires an identifier argument`,
		},
		{
			name: "type missing identifier-ref argument",
			source: `module cambium-type-missing-argument {
    namespace "urn:cambium:type-missing-argument";
    prefix ctma;
    leaf value {
        type;
    }
}`,
			message: `type statement requires an identifier-ref argument`,
		},
		{
			name: "input unexpected argument",
			source: `module cambium-input-unexpected-argument {
    namespace "urn:cambium:input-unexpected-argument";
    prefix ciua;
    rpc reset {
        input bad {
            leaf target { type string; }
        }
    }
}`,
			message: `input statement must not have an argument`,
		},
		{
			name: "refine missing target argument",
			source: `module cambium-refine-missing-argument {
    namespace "urn:cambium:refine-missing-argument";
    prefix crma;

    grouping reusable {
        leaf value { type string; }
    }

    container top {
        uses reusable {
            refine {
                mandatory true;
            }
        }
    }
}`,
			message: `refine statement requires a schema-nodeid argument`,
		},
		{
			name: "deviate missing operation argument",
			source: `module cambium-deviate-missing-argument {
    namespace "urn:cambium:deviate-missing-argument";
    prefix cdma;

    container top {
        leaf value {
            type string;
        }
    }

    deviation "/cdma:top/cdma:value" {
        deviate;
    }
}`,
			message: `deviate statement requires an operation argument`,
		},
		{
			name: "description missing string argument",
			source: `module cambium-description-missing-argument {
    namespace "urn:cambium:description-missing-argument";
    prefix cdma;
    description;
}`,
			message: `description statement requires an argument`,
		},
		{
			name: "presence missing string argument",
			source: `module cambium-presence-missing-argument {
    namespace "urn:cambium:presence-missing-argument";
    prefix cpma;
    container top {
        presence;
    }
}`,
			message: `presence statement requires an argument`,
		},
		{
			name: "must missing xpath argument",
			source: `module cambium-must-missing-argument {
    namespace "urn:cambium:must-missing-argument";
    prefix cmma;
    container top {
        must;
    }
}`,
			message: `must statement requires an argument`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			builder, err := cambium.NewContextBuilder(cambium.ContextFlags{})
			if err != nil {
				t.Fatal(err)
			}
			if err := builder.LoadModuleStr(tc.source); err != nil {
				t.Fatalf("LoadModuleStr: %v", err)
			}
			_, err = builder.Build()
			if err == nil {
				t.Fatal("Build accepted invalid statement argument cardinality")
			}
			var ce *cambium.Error
			if !errors.As(err, &ce) || ce.RuleCode() != cambium.RuleCodeContext {
				t.Fatalf("Build error = %v, want RuleCodeContext", err)
			}
			if !strings.Contains(err.Error(), tc.message) {
				t.Fatalf("Build error = %q, want to contain %q", err.Error(), tc.message)
			}
		})
	}
}

func TestInvalidFeatureMetadataReturnsContextRuleCode(t *testing.T) {
	cases := []struct {
		name    string
		source  string
		message string
	}{
		{
			name: "duplicate description",
			source: `module cambium-feature-duplicate-description {
    namespace "urn:cambium:feature-duplicate-description";
    prefix cfdd;

    feature advanced {
        description "one";
        description "two";
    }
}`,
			message: `feature "advanced" has multiple description statements`,
		},
		{
			name: "duplicate reference",
			source: `module cambium-feature-duplicate-reference {
    namespace "urn:cambium:feature-duplicate-reference";
    prefix cfdr;

    feature advanced {
        reference "one";
        reference "two";
    }
}`,
			message: `feature "advanced" has multiple reference statements`,
		},
		{
			name: "invalid status",
			source: `module cambium-feature-invalid-status {
    namespace "urn:cambium:feature-invalid-status";
    prefix cfis;

    feature advanced {
        status future;
    }
}`,
			message: `invalid status "future"`,
		},
		{
			name: "duplicate status",
			source: `module cambium-feature-duplicate-status {
    namespace "urn:cambium:feature-duplicate-status";
    prefix cfds;

    feature advanced {
        status current;
        status deprecated;
    }
}`,
			message: `feature "advanced" has multiple status statements`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			builder, err := cambium.NewContextBuilder(cambium.ContextFlags{})
			if err != nil {
				t.Fatal(err)
			}
			if err := builder.LoadModuleStr(tc.source); err != nil {
				t.Fatalf("LoadModuleStr: %v", err)
			}
			_, err = builder.Build()
			if err == nil {
				t.Fatal("Build accepted invalid feature metadata")
			}
			var ce *cambium.Error
			if !errors.As(err, &ce) || ce.RuleCode() != cambium.RuleCodeContext {
				t.Fatalf("Build error = %v, want RuleCodeContext", err)
			}
			if !strings.Contains(err.Error(), tc.message) {
				t.Fatalf("Build error = %q, want to contain %q", err.Error(), tc.message)
			}
		})
	}
}

func TestInvalidDefinitionMetadataReturnsContextRuleCode(t *testing.T) {
	cases := []struct {
		name    string
		source  string
		message string
	}{
		{
			name: "typedef duplicate description",
			source: `module cambium-typedef-duplicate-description {
    namespace "urn:cambium:typedef-duplicate-description";
    prefix ctdd;

    typedef text {
        type string;
        description "one";
        description "two";
    }
}`,
			message: `typedef "text" has multiple description statements`,
		},
		{
			name: "grouping duplicate reference",
			source: `module cambium-grouping-duplicate-reference {
    namespace "urn:cambium:grouping-duplicate-reference";
    prefix cgdr;

    grouping shared {
        reference "one";
        reference "two";
        leaf value { type string; }
    }
}`,
			message: `grouping "shared" has multiple reference statements`,
		},
		{
			name: "identity duplicate description",
			source: `module cambium-identity-duplicate-description {
    namespace "urn:cambium:identity-duplicate-description";
    prefix cidd;

    identity root {
        description "one";
        description "two";
    }
}`,
			message: `identity "root" has multiple description statements`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			builder, err := cambium.NewContextBuilder(cambium.ContextFlags{})
			if err != nil {
				t.Fatal(err)
			}
			if err := builder.LoadModuleStr(tc.source); err != nil {
				t.Fatalf("LoadModuleStr: %v", err)
			}
			_, err = builder.Build()
			if err == nil {
				t.Fatal("Build accepted invalid definition metadata")
			}
			var ce *cambium.Error
			if !errors.As(err, &ce) || ce.RuleCode() != cambium.RuleCodeContext {
				t.Fatalf("Build error = %v, want RuleCodeContext", err)
			}
			if !strings.Contains(err.Error(), tc.message) {
				t.Fatalf("Build error = %q, want to contain %q", err.Error(), tc.message)
			}
		})
	}
}

func TestInvalidExtensionDefinitionsReturnContextRuleCode(t *testing.T) {
	cases := []struct {
		name    string
		source  string
		message string
	}{
		{
			name: "duplicate argument",
			source: `module cambium-extension-duplicate-argument {
    namespace "urn:cambium:extension-duplicate-argument";
    prefix ceda;

    extension marker {
        argument one;
        argument two;
    }
}`,
			message: `extension "marker" has multiple argument statements`,
		},
		{
			name: "duplicate yin-element",
			source: `module cambium-extension-duplicate-yin-element {
    namespace "urn:cambium:extension-duplicate-yin-element";
    prefix cedy;

    extension marker {
        argument value {
            yin-element true;
            yin-element false;
        }
    }
}`,
			message: `argument "value" has multiple yin-element statements`,
		},
		{
			name: "invalid yin-element",
			source: `module cambium-extension-invalid-yin-element {
    namespace "urn:cambium:extension-invalid-yin-element";
    prefix ceiy;

    extension marker {
        argument value {
            yin-element maybe;
        }
    }
}`,
			message: `invalid yin-element "maybe"`,
		},
		{
			name: "invalid argument child",
			source: `module cambium-extension-invalid-argument-child {
    namespace "urn:cambium:extension-invalid-argument-child";
    prefix ceiac;

    extension marker {
        argument value {
            description "not valid here";
        }
    }
}`,
			message: `description "not valid here" is not valid under argument "value"`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			builder, err := cambium.NewContextBuilder(cambium.ContextFlags{})
			if err != nil {
				t.Fatal(err)
			}
			if err := builder.LoadModuleStr(tc.source); err != nil {
				t.Fatalf("LoadModuleStr: %v", err)
			}
			_, err = builder.Build()
			if err == nil {
				t.Fatal("Build accepted invalid extension definition")
			}
			var ce *cambium.Error
			if !errors.As(err, &ce) || ce.RuleCode() != cambium.RuleCodeContext {
				t.Fatalf("Build error = %v, want RuleCodeContext", err)
			}
			if !strings.Contains(err.Error(), tc.message) {
				t.Fatalf("Build error = %q, want to contain %q", err.Error(), tc.message)
			}
		})
	}
}

func TestInvalidExtensionInstancesReturnContextRuleCode(t *testing.T) {
	cases := []struct {
		name    string
		source  string
		message string
	}{
		{
			name: "unknown extension on schema node",
			source: `module cambium-unknown-extension-instance {
    namespace "urn:cambium:unknown-extension-instance";
    prefix cuei;

    container top {
        missing:marker true;
    }
}`,
			message: `unknown extension "missing:marker"`,
		},
		{
			name: "unknown extension at module top level",
			source: `module cambium-unknown-top-extension-instance {
    namespace "urn:cambium:unknown-top-extension-instance";
    prefix cutei;

    missing:marker true;
    leaf value { type string; }
}`,
			message: `unknown extension "missing:marker"`,
		},
		{
			name: "unknown extension under type",
			source: `module cambium-unknown-type-extension-instance {
    namespace "urn:cambium:unknown-type-extension-instance";
    prefix cutxi;

    leaf value {
        type string {
            missing:marker true;
        }
    }
}`,
			message: `unknown extension "missing:marker"`,
		},
		{
			name: "extension missing required argument",
			source: `module cambium-extension-missing-required-argument {
    namespace "urn:cambium:extension-missing-required-argument";
    prefix cemra;

    extension marker {
        argument value;
    }

    leaf value {
        cemra:marker;
        type string;
    }
}`,
			message: `extension "cemra:marker" requires an argument`,
		},
		{
			name: "extension unexpected argument",
			source: `module cambium-extension-unexpected-argument {
    namespace "urn:cambium:extension-unexpected-argument";
    prefix ceua;

    extension marker;

    leaf value {
        ceua:marker "bad";
        type string;
    }
}`,
			message: `extension "ceua:marker" does not accept an argument`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			builder, err := cambium.NewContextBuilder(cambium.ContextFlags{})
			if err != nil {
				t.Fatal(err)
			}
			if err := builder.LoadModuleStr(tc.source); err != nil {
				t.Fatalf("LoadModuleStr: %v", err)
			}
			_, err = builder.Build()
			if err == nil {
				t.Fatal("Build accepted unknown extension instance")
			}
			var ce *cambium.Error
			if !errors.As(err, &ce) || ce.RuleCode() != cambium.RuleCodeContext {
				t.Fatalf("Build error = %v, want RuleCodeContext", err)
			}
			if !strings.Contains(err.Error(), tc.message) {
				t.Fatalf("Build error = %q, want to contain %q", err.Error(), tc.message)
			}
		})
	}
}

func TestUnresolvedSchemaReferencesReturnContextRuleCode(t *testing.T) {
	cases := []struct {
		name    string
		source  string
		message string
	}{
		{
			name: "unknown typedef",
			source: `module cambium-unknown-typedef {
    namespace "urn:cambium:unknown-typedef";
    prefix cut;
    leaf value { type missing-type; }
}`,
			message: `unknown type "missing-type"`,
		},
		{
			name: "unknown grouping",
			source: `module cambium-unknown-grouping {
    namespace "urn:cambium:unknown-grouping";
    prefix cug;
    container top { uses missing-group; }
}`,
			message: `unknown grouping "missing-group"`,
		},
		{
			name: "grouping uses cycle",
			source: `module cambium-grouping-uses-cycle {
    namespace "urn:cambium:grouping-uses-cycle";
    prefix cguc;

    grouping recursive {
        uses recursive;
    }

    container top {
        uses recursive;
    }
}`,
			message: `grouping cycle involving "recursive"`,
		},
		{
			name: "unknown refine target",
			source: `module cambium-unknown-refine-target {
    namespace "urn:cambium:unknown-refine-target";
    prefix curt;

    grouping reusable {
        leaf value { type string; }
    }

    container top {
        uses reusable {
            refine missing {
                mandatory true;
            }
        }
    }
}`,
			message: `refine "missing" target not found`,
		},
		{
			name: "unknown uses augment prefixed target",
			source: `module cambium-unknown-uses-augment-prefixed-target {
    namespace "urn:cambium:unknown-uses-augment-prefixed-target";
    prefix cuuapt;

    grouping reusable {
        container settings {
            leaf name { type string; }
        }
    }

    container top {
        uses reusable {
            augment missing:settings {
                leaf enabled { type boolean; }
            }
        }
    }
}`,
			message: `uses augment "missing:settings" target not found`,
		},
		{
			name: "uses augment target with surrounding whitespace",
			source: `module cambium-uses-augment-target-whitespace {
    namespace "urn:cambium:uses-augment-target-whitespace";
    prefix cuatw;

    grouping reusable {
        container settings {
            leaf name { type string; }
        }
    }

    container top {
        uses reusable {
            augment " settings " {
                leaf enabled { type boolean; }
            }
        }
    }
}`,
			message: `invalid descendant schema-nodeid " settings " for augment`,
		},
		{
			name: "unknown identityref base",
			source: `module cambium-unknown-identityref-base {
    namespace "urn:cambium:unknown-identityref-base";
    prefix cuib;
    leaf value {
        type identityref {
            base missing-base;
        }
    }
}`,
			message: `unknown identity base "missing-base"`,
		},
		{
			name: "unknown identity base",
			source: `module cambium-unknown-identity-base {
    namespace "urn:cambium:unknown-identity-base";
    prefix cuibd;
    identity child {
        base missing-base;
    }
}`,
			message: `unknown identity base "missing-base"`,
		},
		{
			name: "duplicate identity base",
			source: `module cambium-duplicate-identity-base {
    namespace "urn:cambium:duplicate-identity-base";
    prefix cdib;
    identity root;
    identity child {
        base root;
        base root;
    }
}`,
			message: `identity "child" has duplicate base "root"`,
		},
		{
			name: "typedef cycle",
			source: `module cambium-typedef-cycle {
    namespace "urn:cambium:typedef-cycle";
    prefix ctc;
    typedef a { type b; }
    typedef b { type a; }
    leaf value { type a; }
}`,
			message: `typedef cycle involving`,
		},
		{
			name: "identity cycle",
			source: `module cambium-identity-cycle {
    namespace "urn:cambium:identity-cycle";
    prefix cic;
    identity a { base b; }
    identity b { base a; }
}`,
			message: `identity cycle involving`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			builder, err := cambium.NewContextBuilder(cambium.ContextFlags{})
			if err != nil {
				t.Fatal(err)
			}
			if err := builder.LoadModuleStr(tc.source); err != nil {
				t.Fatalf("LoadModuleStr: %v", err)
			}
			_, err = builder.Build()
			if err == nil {
				t.Fatal("Build accepted unresolved schema reference")
			}
			var ce *cambium.Error
			if !errors.As(err, &ce) || ce.RuleCode() != cambium.RuleCodeContext {
				t.Fatalf("Build error = %v, want RuleCodeContext", err)
			}
			if !strings.Contains(err.Error(), tc.message) {
				t.Fatalf("Build error = %q, want to contain %q", err.Error(), tc.message)
			}
		})
	}
}

func TestUnresolvedAugmentDeviationTargetsReturnContextRuleCode(t *testing.T) {
	target := `module cambium-unresolved-target {
    namespace "urn:cambium:unresolved-target";
    prefix cut;

    container top {
        leaf value { type string; }
    }
}`
	cases := []struct {
		name    string
		source  string
		message string
	}{
		{
			name: "augment target",
			source: `module cambium-unresolved-augment {
    namespace "urn:cambium:unresolved-augment";
    prefix cua;

    import cambium-unresolved-target {
        prefix target;
    }

    augment "/target:top/target:missing" {
        leaf added { type string; }
    }
}`,
			message: `augment "/target:top/target:missing" target not found`,
		},
		{
			name: "deviation target",
			source: `module cambium-unresolved-deviation {
    namespace "urn:cambium:unresolved-deviation";
    prefix cud;

    import cambium-unresolved-target {
        prefix target;
    }

    deviation "/target:top/target:missing" {
        deviate not-supported;
    }
}`,
			message: `deviation "/target:top/target:missing" target not found`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			builder, err := cambium.NewContextBuilder(cambium.ContextFlags{})
			if err != nil {
				t.Fatal(err)
			}
			if err := builder.LoadModuleStr(target); err != nil {
				t.Fatalf("Load target: %v", err)
			}
			if err := builder.LoadModuleStr(tc.source); err != nil {
				t.Fatalf("Load source: %v", err)
			}
			_, err = builder.Build()
			if err == nil {
				t.Fatal("Build accepted unresolved target path")
			}
			var ce *cambium.Error
			if !errors.As(err, &ce) || ce.RuleCode() != cambium.RuleCodeContext {
				t.Fatalf("Build error = %v, want RuleCodeContext", err)
			}
			if !strings.Contains(err.Error(), tc.message) {
				t.Fatalf("Build error = %q, want to contain %q", err.Error(), tc.message)
			}
		})
	}
}

func TestModuleNamePrefixRejectedInSourceQNames(t *testing.T) {
	cases := []struct {
		name    string
		source  string
		message string
	}{
		{
			name: "identityref base",
			source: `module cambium-module-name-prefix-identity-base {
    namespace "urn:cambium:module-name-prefix-identity-base";
    prefix cmnpib;

    identity base;
    leaf value {
        type identityref {
            base cambium-module-name-prefix-identity-base:base;
        }
    }
}`,
			message: `unknown identity base "cambium-module-name-prefix-identity-base:base"`,
		},
		{
			name: "identityref default",
			source: `module cambium-module-name-prefix-identity-default {
    namespace "urn:cambium:module-name-prefix-identity-default";
    prefix cmnpid;

    identity base;
    identity child {
        base base;
    }
    leaf value {
        type identityref {
            base base;
        }
        default cambium-module-name-prefix-identity-default:child;
    }
}`,
			message: `default "cambium-module-name-prefix-identity-default:child" is not valid for identityref leaf "value"`,
		},
		{
			name: "extension instance",
			source: `module cambium-module-name-prefix-extension {
    namespace "urn:cambium:module-name-prefix-extension";
    prefix cmnpe;

    extension marker {
        argument value;
    }
    cambium-module-name-prefix-extension:marker "bad";
}`,
			message: `unknown extension "cambium-module-name-prefix-extension:marker"`,
		},
		{
			name: "leafref path",
			source: `module cambium-module-name-prefix-leafref {
    namespace "urn:cambium:module-name-prefix-leafref";
    prefix cmnpl;

    container top {
        leaf value { type string; }
    }
    leaf ref {
        type leafref {
            path "/cambium-module-name-prefix-leafref:top/cambium-module-name-prefix-leafref:value";
        }
    }
}`,
			message: `unknown prefix "cambium-module-name-prefix-leafref" in leafref path "/cambium-module-name-prefix-leafref:top/cambium-module-name-prefix-leafref:value"`,
		},
		{
			name: "leafref prefix path segment",
			source: `module cambium-source-prefix-segment-leafref {
    namespace "urn:cambium:source-prefix-segment-leafref";
    prefix cspsl;

    container top {
        leaf value { type string; }
    }
    leaf ref {
        type leafref {
            path "/cspsl/top/value";
        }
    }
}`,
			message: `leafref "ref" path "/cspsl/top/value" target not found`,
		},
		{
			name: "leafref predicate unknown prefix",
			source: `module cambium-leafref-predicate-unknown-prefix {
    namespace "urn:cambium:leafref-predicate-unknown-prefix";
    prefix clpup;

    leaf selected { type string; }
    list item {
        key id;
        leaf id { type string; }
        leaf value { type string; }
    }
    leaf ref {
        type leafref {
            path "/item[missing:id = current()/../selected]/value";
        }
    }
}`,
			message: `unknown prefix "missing" in leafref path "/item[missing:id = current()/../selected]/value"`,
		},
		{
			name: "leafref predicate module-name prefix",
			source: `module cambium-leafref-predicate-module-name {
    namespace "urn:cambium:leafref-predicate-module-name";
    prefix clpmnp;

    leaf selected { type string; }
    list item {
        key id;
        leaf id { type string; }
        leaf value { type string; }
    }
    leaf ref {
        type leafref {
            path "/item[cambium-leafref-predicate-module-name:id = current()/../selected]/value";
        }
    }
}`,
			message: `unknown prefix "cambium-leafref-predicate-module-name" in leafref path "/item[cambium-leafref-predicate-module-name:id = current()/../selected]/value"`,
		},
		{
			name: "augment target",
			source: `module cambium-module-name-prefix-augment {
    namespace "urn:cambium:module-name-prefix-augment";
    prefix cmnpa;

    container top {
        leaf before { type string; }
    }
    augment "/cambium-module-name-prefix-augment:top" {
        leaf added { type string; }
    }
}`,
			message: `augment "/cambium-module-name-prefix-augment:top" target not found`,
		},
		{
			name: "augment prefix path segment",
			source: `module cambium-source-prefix-segment-augment {
    namespace "urn:cambium:source-prefix-segment-augment";
    prefix cspsa;

    container top {
        leaf before { type string; }
    }
    augment "/cspsa/top" {
        leaf added { type string; }
    }
}`,
			message: `augment "/cspsa/top" target not found`,
		},
		{
			name: "deviation prefix path segment",
			source: `module cambium-source-prefix-segment-deviation {
    namespace "urn:cambium:source-prefix-segment-deviation";
    prefix cspsd;

    container top {
        leaf value { type string; }
    }
    deviation "/cspsd/top/value" {
        deviate add {
            default "x";
        }
    }
}`,
			message: `deviation "/cspsd/top/value" target not found`,
		},
		{
			name: "must unknown xpath prefix",
			source: `module cambium-xpath-unknown-prefix-must {
    namespace "urn:cambium:xpath-unknown-prefix-must";
    prefix cxupm;

    container top {
        must "/missing:top = 'x'";
        leaf value { type string; }
    }
}`,
			message: `unknown prefix "missing" in must expression "/missing:top = 'x'"`,
		},
		{
			name: "when unknown xpath prefix",
			source: `module cambium-xpath-unknown-prefix-when {
    namespace "urn:cambium:xpath-unknown-prefix-when";
    prefix cxupw;

    leaf value {
        when "/missing:enabled = 'true'";
        type string;
    }
    leaf enabled { type boolean; }
}`,
			message: `unknown prefix "missing" in when expression "/missing:enabled = 'true'"`,
		},
		{
			name: "must module-name xpath prefix",
			source: `module cambium-xpath-module-prefix-must {
    namespace "urn:cambium:xpath-module-prefix-must";
    prefix cxmpm;

    container top {
        must "/cambium-xpath-module-prefix-must:top/cambium-xpath-module-prefix-must:value = 'x'";
        leaf value { type string; }
    }
}`,
			message: `unknown prefix "cambium-xpath-module-prefix-must" in must expression "/cambium-xpath-module-prefix-must:top/cambium-xpath-module-prefix-must:value = 'x'"`,
		},
		{
			name: "when module-name xpath prefix",
			source: `module cambium-xpath-module-prefix-when {
    namespace "urn:cambium:xpath-module-prefix-when";
    prefix cxmpw;

    leaf value {
        when "/cambium-xpath-module-prefix-when:enabled = 'true'";
        type string;
    }
    leaf enabled { type boolean; }
}`,
			message: `unknown prefix "cambium-xpath-module-prefix-when" in when expression "/cambium-xpath-module-prefix-when:enabled = 'true'"`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			builder, err := cambium.NewContextBuilder(cambium.ContextFlags{})
			if err != nil {
				t.Fatal(err)
			}
			if err := builder.LoadModuleStr(tc.source); err != nil {
				t.Fatalf("LoadModuleStr: %v", err)
			}
			_, err = builder.Build()
			if err == nil {
				t.Fatal("Build accepted module-name prefix in source QName")
			}
			var ce *cambium.Error
			if !errors.As(err, &ce) || ce.RuleCode() != cambium.RuleCodeContext {
				t.Fatalf("Build error = %v, want RuleCodeContext", err)
			}
			if !strings.Contains(err.Error(), tc.message) {
				t.Fatalf("Build error = %q, want to contain %q", err.Error(), tc.message)
			}
		})
	}
}

func TestInvalidTopLevelSchemaNodeIDReturnsContextRuleCode(t *testing.T) {
	target := `module cambium-invalid-schema-nodeid-target {
    namespace "urn:cambium:invalid-schema-nodeid-target";
    prefix cisnt;

    container top {
        leaf value { type string; }
    }
}`
	cases := []struct {
		name    string
		source  string
		message string
	}{
		{
			name: "augment missing leading slash",
			source: `module cambium-invalid-augment-schema-nodeid {
    namespace "urn:cambium:invalid-augment-schema-nodeid";
    prefix ciasn;

    import cambium-invalid-schema-nodeid-target {
        prefix target;
    }

    augment "target:top" {
        leaf added { type string; }
    }
}`,
			message: `invalid absolute schema-nodeid "target:top" for augment`,
		},
		{
			name: "deviation missing leading slash",
			source: `module cambium-invalid-deviation-schema-nodeid {
    namespace "urn:cambium:invalid-deviation-schema-nodeid";
    prefix cidsn;

    import cambium-invalid-schema-nodeid-target {
        prefix target;
    }

    deviation "target:top" {
        deviate not-supported;
    }
}`,
			message: `invalid absolute schema-nodeid "target:top" for deviation`,
		},
		{
			name: "augment trailing slash",
			source: `module cambium-invalid-augment-schema-nodeid-trailing-slash {
    namespace "urn:cambium:invalid-augment-schema-nodeid-trailing-slash";
    prefix ciasts;

    import cambium-invalid-schema-nodeid-target {
        prefix target;
    }

    augment "/target:top/" {
        leaf added { type string; }
    }
}`,
			message: `invalid absolute schema-nodeid "/target:top/" for augment`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			builder, err := cambium.NewContextBuilder(cambium.ContextFlags{})
			if err != nil {
				t.Fatal(err)
			}
			if err := builder.LoadModuleStr(target); err != nil {
				t.Fatalf("Load target: %v", err)
			}
			if err := builder.LoadModuleStr(tc.source); err != nil {
				t.Fatalf("Load source: %v", err)
			}
			_, err = builder.Build()
			if err == nil {
				t.Fatal("Build accepted invalid schema-nodeid")
			}
			var ce *cambium.Error
			if !errors.As(err, &ce) || ce.RuleCode() != cambium.RuleCodeContext {
				t.Fatalf("Build error = %v, want RuleCodeContext", err)
			}
			if !strings.Contains(err.Error(), tc.message) {
				t.Fatalf("Build error = %q, want to contain %q", err.Error(), tc.message)
			}
		})
	}
}

func TestInvalidDeviationTypeReturnsContextRuleCode(t *testing.T) {
	target := `module cambium-invalid-deviation-target {
    namespace "urn:cambium:invalid-deviation-target";
    prefix cidt;

    container top {
        leaf value { type string; }
    }
}`
	cases := []struct {
		name    string
		source  string
		message string
	}{
		{
			name: "unsupported deviate",
			source: `module cambium-invalid-deviation-source {
    namespace "urn:cambium:invalid-deviation-source";
    prefix cids;

    import cambium-invalid-deviation-target {
        prefix target;
    }

    deviation "/target:top/target:value" {
        deviate unsupported;
    }
}`,
			message: `unsupported deviation type "unsupported"`,
		},
		{
			name: "missing deviate",
			source: `module cambium-missing-deviate-source {
    namespace "urn:cambium:missing-deviate-source";
    prefix cmds;

    import cambium-invalid-deviation-target {
        prefix target;
    }

    deviation "/target:top/target:value";
}`,
			message: `deviation "/target:top/target:value" has no deviate statements`,
		},
		{
			name: "replace type on container",
			source: `module cambium-deviation-type-on-container {
    namespace "urn:cambium:deviation-type-on-container";
    prefix cdtoc;

    import cambium-invalid-deviation-target {
        prefix target;
    }

    deviation "/target:top" {
        deviate replace {
            type string;
        }
    }
}`,
			message: `type at`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			builder, err := cambium.NewContextBuilder(cambium.ContextFlags{})
			if err != nil {
				t.Fatal(err)
			}
			if err := builder.LoadModuleStr(target); err != nil {
				t.Fatalf("Load target: %v", err)
			}
			if err := builder.LoadModuleStr(tc.source); err != nil {
				t.Fatalf("Load source: %v", err)
			}
			_, err = builder.Build()
			if err == nil {
				t.Fatal("Build accepted invalid deviation type")
			}
			var ce *cambium.Error
			if !errors.As(err, &ce) || ce.RuleCode() != cambium.RuleCodeContext {
				t.Fatalf("Build error = %v, want RuleCodeContext", err)
			}
			if !strings.Contains(err.Error(), tc.message) {
				t.Fatalf("Build error = %q, want to contain %q", err.Error(), tc.message)
			}
		})
	}
}

func TestInvalidDeviationMetadataReturnsContextRuleCode(t *testing.T) {
	target := `module cambium-deviation-metadata-target {
    namespace "urn:cambium:deviation-metadata-target";
    prefix cdmt;

    container top;
}`
	cases := []struct {
		name    string
		source  string
		message string
	}{
		{
			name: "duplicate description",
			source: `module cambium-deviation-duplicate-description {
    namespace "urn:cambium:deviation-duplicate-description";
    prefix cddd;

    import cambium-deviation-metadata-target {
        prefix target;
    }

    deviation "/target:top" {
        description "one";
        description "two";
        deviate not-supported;
    }
}`,
			message: `deviation "/target:top" has multiple description statements`,
		},
		{
			name: "duplicate reference",
			source: `module cambium-deviation-duplicate-reference {
    namespace "urn:cambium:deviation-duplicate-reference";
    prefix cddr;

    import cambium-deviation-metadata-target {
        prefix target;
    }

    deviation "/target:top" {
        reference "one";
        reference "two";
        deviate not-supported;
    }
}`,
			message: `deviation "/target:top" has multiple reference statements`,
		},
		{
			name: "invalid child",
			source: `module cambium-deviation-invalid-child {
    namespace "urn:cambium:deviation-invalid-child";
    prefix cdic;

    import cambium-deviation-metadata-target {
        prefix target;
    }

    deviation "/target:top" {
        default "bad";
        deviate not-supported;
    }
}`,
			message: `default "bad" is not valid under deviation "/target:top"`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			builder, err := cambium.NewContextBuilder(cambium.ContextFlags{})
			if err != nil {
				t.Fatal(err)
			}
			if err := builder.LoadModuleStr(target); err != nil {
				t.Fatalf("Load target: %v", err)
			}
			if err := builder.LoadModuleStr(tc.source); err != nil {
				t.Fatalf("Load source: %v", err)
			}
			_, err = builder.Build()
			if err == nil {
				t.Fatal("Build accepted invalid deviation metadata")
			}
			var ce *cambium.Error
			if !errors.As(err, &ce) || ce.RuleCode() != cambium.RuleCodeContext {
				t.Fatalf("Build error = %v, want RuleCodeContext", err)
			}
			if !strings.Contains(err.Error(), tc.message) {
				t.Fatalf("Build error = %q, want to contain %q", err.Error(), tc.message)
			}
		})
	}
}

func TestInvalidDeviateStatementChildrenReturnContextRuleCode(t *testing.T) {
	target := `module cambium-deviate-child-target {
    namespace "urn:cambium:deviate-child-target";
    prefix cdct;

    leaf value {
        type string;
        default "old";
    }
}`
	cases := []struct {
		name    string
		source  string
		message string
	}{
		{
			name: "not-supported child",
			source: `module cambium-deviate-not-supported-child {
    namespace "urn:cambium:deviate-not-supported-child";
    prefix cdnsc;

    import cambium-deviate-child-target {
        prefix target;
    }

    deviation "/target:value" {
        deviate not-supported {
            default "bad";
        }
    }
}`,
			message: `default "bad" is not valid under deviate "not-supported"`,
		},
		{
			name: "add invalid child",
			source: `module cambium-deviate-add-invalid-child {
    namespace "urn:cambium:deviate-add-invalid-child";
    prefix cdaic;

    import cambium-deviate-child-target {
        prefix target;
    }

    feature advanced;

    deviation "/target:value" {
        deviate add {
            if-feature advanced;
        }
    }
}`,
			message: `if-feature "advanced" is not valid under deviate "add"`,
		},
		{
			name: "delete type child",
			source: `module cambium-deviate-delete-type-child {
    namespace "urn:cambium:deviate-delete-type-child";
    prefix cddtc;

    import cambium-deviate-child-target {
        prefix target;
    }

    deviation "/target:value" {
        deviate delete {
            type string;
        }
    }
}`,
			message: `type "string" is not valid under deviate "delete"`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			builder, err := cambium.NewContextBuilder(cambium.ContextFlags{})
			if err != nil {
				t.Fatal(err)
			}
			if err := builder.LoadModuleStr(target); err != nil {
				t.Fatalf("Load target: %v", err)
			}
			if err := builder.LoadModuleStr(tc.source); err != nil {
				t.Fatalf("Load source: %v", err)
			}
			_, err = builder.Build()
			if err == nil {
				t.Fatal("Build accepted invalid deviate child")
			}
			var ce *cambium.Error
			if !errors.As(err, &ce) || ce.RuleCode() != cambium.RuleCodeContext {
				t.Fatalf("Build error = %v, want RuleCodeContext", err)
			}
			if !strings.Contains(err.Error(), tc.message) {
				t.Fatalf("Build error = %q, want to contain %q", err.Error(), tc.message)
			}
		})
	}
}

func TestInvalidDeviationAddUnitsReturnsContextRuleCode(t *testing.T) {
	target := `module cambium-deviation-add-units-target {
    namespace "urn:cambium:deviation-add-units-target";
    prefix cdaut;

    leaf value {
        type string;
        units "meters";
    }
}`
	source := `module cambium-deviation-add-units-source {
    namespace "urn:cambium:deviation-add-units-source";
    prefix cdaus;

    import cambium-deviation-add-units-target {
        prefix target;
    }

    deviation "/target:value" {
        deviate add {
            units "seconds";
        }
    }
}`
	builder, err := cambium.NewContextBuilder(cambium.ContextFlags{})
	if err != nil {
		t.Fatal(err)
	}
	if err := builder.LoadModuleStr(target); err != nil {
		t.Fatalf("Load target: %v", err)
	}
	if err := builder.LoadModuleStr(source); err != nil {
		t.Fatalf("Load source: %v", err)
	}
	_, err = builder.Build()
	if err == nil {
		t.Fatal("Build accepted deviate add units on a target with existing units")
	}
	var ce *cambium.Error
	if !errors.As(err, &ce) || ce.RuleCode() != cambium.RuleCodeContext {
		t.Fatalf("Build error = %v, want RuleCodeContext", err)
	}
	if !strings.Contains(err.Error(), `deviate add units for "value" already exists`) {
		t.Fatalf("Build error = %q, want duplicate units message", err.Error())
	}
}

func TestInvalidDeviationUnitsPlacementReturnsContextRuleCode(t *testing.T) {
	target := `module cambium-deviation-units-placement-target {
    namespace "urn:cambium:deviation-units-placement-target";
    prefix cdupt;

    container top;
}`
	source := `module cambium-deviation-units-placement-source {
    namespace "urn:cambium:deviation-units-placement-source";
    prefix cdups;

    import cambium-deviation-units-placement-target {
        prefix target;
    }

    deviation "/target:top" {
        deviate add {
            units "meters";
        }
    }
}`
	builder, err := cambium.NewContextBuilder(cambium.ContextFlags{})
	if err != nil {
		t.Fatal(err)
	}
	if err := builder.LoadModuleStr(target); err != nil {
		t.Fatalf("Load target: %v", err)
	}
	if err := builder.LoadModuleStr(source); err != nil {
		t.Fatalf("Load source: %v", err)
	}
	_, err = builder.Build()
	if err == nil {
		t.Fatal("Build accepted deviate units on a container")
	}
	var ce *cambium.Error
	if !errors.As(err, &ce) || ce.RuleCode() != cambium.RuleCodeContext {
		t.Fatalf("Build error = %v, want RuleCodeContext", err)
	}
	if !strings.Contains(err.Error(), `units at`) {
		t.Fatalf("Build error = %q, want units placement message", err.Error())
	}
}

func TestInvalidDeviationReplaceUnitsReturnsContextRuleCode(t *testing.T) {
	target := `module cambium-deviation-replace-units-target {
    namespace "urn:cambium:deviation-replace-units-target";
    prefix cdrut;

    leaf value {
        type string;
    }
}`
	source := `module cambium-deviation-replace-units-source {
    namespace "urn:cambium:deviation-replace-units-source";
    prefix cdrus;

    import cambium-deviation-replace-units-target {
        prefix target;
    }

    deviation "/target:value" {
        deviate replace {
            units "seconds";
        }
    }
}`
	builder, err := cambium.NewContextBuilder(cambium.ContextFlags{})
	if err != nil {
		t.Fatal(err)
	}
	if err := builder.LoadModuleStr(target); err != nil {
		t.Fatalf("Load target: %v", err)
	}
	if err := builder.LoadModuleStr(source); err != nil {
		t.Fatalf("Load source: %v", err)
	}
	_, err = builder.Build()
	if err == nil {
		t.Fatal("Build accepted deviate replace units on a target without units")
	}
	var ce *cambium.Error
	if !errors.As(err, &ce) || ce.RuleCode() != cambium.RuleCodeContext {
		t.Fatalf("Build error = %v, want RuleCodeContext", err)
	}
	if !strings.Contains(err.Error(), `deviate replace units for "value" has no existing units`) {
		t.Fatalf("Build error = %q, want missing units message", err.Error())
	}
}

func TestInvalidDeviationDeleteUnitsReturnsContextRuleCode(t *testing.T) {
	cases := []struct {
		name    string
		target  string
		source  string
		message string
	}{
		{
			name: "missing units",
			target: `module cambium-deviation-delete-missing-units-target {
    namespace "urn:cambium:deviation-delete-missing-units-target";
    prefix cddmut;

    leaf value {
        type string;
    }
}`,
			source: `module cambium-deviation-delete-missing-units-source {
    namespace "urn:cambium:deviation-delete-missing-units-source";
    prefix cddmus;

    import cambium-deviation-delete-missing-units-target {
        prefix target;
    }

    deviation "/target:value" {
        deviate delete {
            units "seconds";
        }
    }
}`,
			message: `deviate delete units "seconds" for "value" does not exist`,
		},
		{
			name: "mismatched units",
			target: `module cambium-deviation-delete-mismatched-units-target {
    namespace "urn:cambium:deviation-delete-mismatched-units-target";
    prefix cddmut;

    leaf value {
        type string;
        units "meters";
    }
}`,
			source: `module cambium-deviation-delete-mismatched-units-source {
    namespace "urn:cambium:deviation-delete-mismatched-units-source";
    prefix cddmus;

    import cambium-deviation-delete-mismatched-units-target {
        prefix target;
    }

    deviation "/target:value" {
        deviate delete {
            units "seconds";
        }
    }
}`,
			message: `deviate delete units "seconds" for "value" does not exist`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			builder, err := cambium.NewContextBuilder(cambium.ContextFlags{})
			if err != nil {
				t.Fatal(err)
			}
			if err := builder.LoadModuleStr(tc.target); err != nil {
				t.Fatalf("Load target: %v", err)
			}
			if err := builder.LoadModuleStr(tc.source); err != nil {
				t.Fatalf("Load source: %v", err)
			}
			_, err = builder.Build()
			if err == nil {
				t.Fatal("Build accepted invalid deviation units delete")
			}
			var ce *cambium.Error
			if !errors.As(err, &ce) || ce.RuleCode() != cambium.RuleCodeContext {
				t.Fatalf("Build error = %v, want RuleCodeContext", err)
			}
			if !strings.Contains(err.Error(), tc.message) {
				t.Fatalf("Build error = %q, want to contain %q", err.Error(), tc.message)
			}
		})
	}
}

func TestInvalidDeviationDefaultOperationsReturnContextRuleCode(t *testing.T) {
	cases := []struct {
		name    string
		target  string
		source  string
		message string
	}{
		{
			name: "add duplicate default",
			target: `module cambium-deviation-add-default-target {
    namespace "urn:cambium:deviation-add-default-target";
    prefix cdadt;

    leaf-list values {
        type string;
        default "old";
    }
}`,
			source: `module cambium-deviation-add-default-source {
    namespace "urn:cambium:deviation-add-default-source";
    prefix cdads;

    import cambium-deviation-add-default-target {
        prefix target;
    }

    deviation "/target:values" {
        deviate add {
            default "old";
        }
    }
}`,
			message: `deviate add default "old" for "values" already exists`,
		},
		{
			name: "replace missing default",
			target: `module cambium-deviation-replace-default-target {
    namespace "urn:cambium:deviation-replace-default-target";
    prefix cdrdt;

    leaf value {
        type string;
    }
}`,
			source: `module cambium-deviation-replace-default-source {
    namespace "urn:cambium:deviation-replace-default-source";
    prefix cdrds;

    import cambium-deviation-replace-default-target {
        prefix target;
    }

    deviation "/target:value" {
        deviate replace {
            default "new";
        }
    }
}`,
			message: `deviate replace default for "value" has no existing default`,
		},
		{
			name: "delete missing default",
			target: `module cambium-deviation-delete-default-target {
    namespace "urn:cambium:deviation-delete-default-target";
    prefix cdddt;

    leaf value {
        type string;
        default "old";
    }
}`,
			source: `module cambium-deviation-delete-default-source {
    namespace "urn:cambium:deviation-delete-default-source";
    prefix cddds;

    import cambium-deviation-delete-default-target {
        prefix target;
    }

    deviation "/target:value" {
        deviate delete {
            default "missing";
        }
    }
}`,
			message: `deviate delete default "missing" for "value" does not exist`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			builder, err := cambium.NewContextBuilder(cambium.ContextFlags{})
			if err != nil {
				t.Fatal(err)
			}
			if err := builder.LoadModuleStr(tc.target); err != nil {
				t.Fatalf("Load target: %v", err)
			}
			if err := builder.LoadModuleStr(tc.source); err != nil {
				t.Fatalf("Load source: %v", err)
			}
			_, err = builder.Build()
			if err == nil {
				t.Fatal("Build accepted invalid deviation default operation")
			}
			var ce *cambium.Error
			if !errors.As(err, &ce) || ce.RuleCode() != cambium.RuleCodeContext {
				t.Fatalf("Build error = %v, want RuleCodeContext", err)
			}
			if !strings.Contains(err.Error(), tc.message) {
				t.Fatalf("Build error = %q, want to contain %q", err.Error(), tc.message)
			}
		})
	}
}

func TestInvalidDeviationCardinalityOperationsReturnContextRuleCode(t *testing.T) {
	cases := []struct {
		name    string
		target  string
		source  string
		message string
	}{
		{
			name: "add existing min-elements",
			target: `module cambium-deviation-add-min-elements-target {
    namespace "urn:cambium:deviation-add-min-elements-target";
    prefix cdamet;

    leaf-list values {
        type string;
        min-elements 1;
    }
}`,
			source: `module cambium-deviation-add-min-elements-source {
    namespace "urn:cambium:deviation-add-min-elements-source";
    prefix cdames;

    import cambium-deviation-add-min-elements-target {
        prefix target;
    }

    deviation "/target:values" {
        deviate add {
            min-elements 2;
        }
    }
}`,
			message: `deviate add min-elements for "values" already exists`,
		},
		{
			name: "add existing max-elements",
			target: `module cambium-deviation-add-max-elements-target {
    namespace "urn:cambium:deviation-add-max-elements-target";
    prefix cdamet;

    leaf-list values {
        type string;
        max-elements 3;
    }
}`,
			source: `module cambium-deviation-add-max-elements-source {
    namespace "urn:cambium:deviation-add-max-elements-source";
    prefix cdames;

    import cambium-deviation-add-max-elements-target {
        prefix target;
    }

    deviation "/target:values" {
        deviate add {
            max-elements 4;
        }
    }
}`,
			message: `deviate add max-elements for "values" already exists`,
		},
		{
			name: "add existing unbounded max-elements",
			target: `module cambium-deviation-add-unbounded-max-elements-target {
    namespace "urn:cambium:deviation-add-unbounded-max-elements-target";
    prefix cdaumet;

    leaf-list values {
        type string;
        max-elements unbounded;
    }
}`,
			source: `module cambium-deviation-add-unbounded-max-elements-source {
    namespace "urn:cambium:deviation-add-unbounded-max-elements-source";
    prefix cdaumes;

    import cambium-deviation-add-unbounded-max-elements-target {
        prefix target;
    }

    deviation "/target:values" {
        deviate add {
            max-elements 4;
        }
    }
}`,
			message: `deviate add max-elements for "values" already exists`,
		},
		{
			name: "replace missing min-elements",
			target: `module cambium-deviation-replace-min-elements-target {
    namespace "urn:cambium:deviation-replace-min-elements-target";
    prefix cdrmet;

    leaf-list values {
        type string;
    }
}`,
			source: `module cambium-deviation-replace-min-elements-source {
    namespace "urn:cambium:deviation-replace-min-elements-source";
    prefix cdrmes;

    import cambium-deviation-replace-min-elements-target {
        prefix target;
    }

    deviation "/target:values" {
        deviate replace {
            min-elements 1;
        }
    }
}`,
			message: `deviate replace min-elements for "values" has no existing min-elements`,
		},
		{
			name: "replace missing max-elements",
			target: `module cambium-deviation-replace-max-elements-target {
    namespace "urn:cambium:deviation-replace-max-elements-target";
    prefix cdrmet;

    leaf-list values {
        type string;
    }
}`,
			source: `module cambium-deviation-replace-max-elements-source {
    namespace "urn:cambium:deviation-replace-max-elements-source";
    prefix cdrmes;

    import cambium-deviation-replace-max-elements-target {
        prefix target;
    }

    deviation "/target:values" {
        deviate replace {
            max-elements 8;
        }
    }
}`,
			message: `deviate replace max-elements for "values" has no existing max-elements`,
		},
		{
			name: "delete missing min-elements",
			target: `module cambium-deviation-delete-min-elements-target {
    namespace "urn:cambium:deviation-delete-min-elements-target";
    prefix cddmet;

    leaf-list values {
        type string;
    }
}`,
			source: `module cambium-deviation-delete-min-elements-source {
    namespace "urn:cambium:deviation-delete-min-elements-source";
    prefix cddmes;

    import cambium-deviation-delete-min-elements-target {
        prefix target;
    }

    deviation "/target:values" {
        deviate delete {
            min-elements 1;
        }
    }
}`,
			message: `deviate delete min-elements for "values" does not exist`,
		},
		{
			name: "delete mismatched min-elements",
			target: `module cambium-deviation-delete-mismatched-min-elements-target {
    namespace "urn:cambium:deviation-delete-mismatched-min-elements-target";
    prefix cddmmet;

    leaf-list values {
        type string;
        min-elements 2;
    }
}`,
			source: `module cambium-deviation-delete-mismatched-min-elements-source {
    namespace "urn:cambium:deviation-delete-mismatched-min-elements-source";
    prefix cddmmes;

    import cambium-deviation-delete-mismatched-min-elements-target {
        prefix target;
    }

    deviation "/target:values" {
        deviate delete {
            min-elements 1;
        }
    }
}`,
			message: `deviate delete min-elements 1 for "values" does not exist`,
		},
		{
			name: "delete missing max-elements",
			target: `module cambium-deviation-delete-max-elements-target {
    namespace "urn:cambium:deviation-delete-max-elements-target";
    prefix cddmet;

    leaf-list values {
        type string;
    }
}`,
			source: `module cambium-deviation-delete-max-elements-source {
    namespace "urn:cambium:deviation-delete-max-elements-source";
    prefix cddmes;

    import cambium-deviation-delete-max-elements-target {
        prefix target;
    }

    deviation "/target:values" {
        deviate delete {
            max-elements 8;
        }
    }
}`,
			message: `deviate delete max-elements for "values" does not exist`,
		},
		{
			name: "delete mismatched max-elements",
			target: `module cambium-deviation-delete-mismatched-max-elements-target {
    namespace "urn:cambium:deviation-delete-mismatched-max-elements-target";
    prefix cddmmet;

    leaf-list values {
        type string;
        max-elements 4;
    }
}`,
			source: `module cambium-deviation-delete-mismatched-max-elements-source {
    namespace "urn:cambium:deviation-delete-mismatched-max-elements-source";
    prefix cddmmes;

    import cambium-deviation-delete-mismatched-max-elements-target {
        prefix target;
    }

    deviation "/target:values" {
        deviate delete {
            max-elements 8;
        }
    }
}`,
			message: `deviate delete max-elements 8 for "values" does not exist`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			builder, err := cambium.NewContextBuilder(cambium.ContextFlags{})
			if err != nil {
				t.Fatal(err)
			}
			if err := builder.LoadModuleStr(tc.target); err != nil {
				t.Fatalf("Load target: %v", err)
			}
			if err := builder.LoadModuleStr(tc.source); err != nil {
				t.Fatalf("Load source: %v", err)
			}
			_, err = builder.Build()
			if err == nil {
				t.Fatal("Build accepted invalid deviation cardinality operation")
			}
			var ce *cambium.Error
			if !errors.As(err, &ce) || ce.RuleCode() != cambium.RuleCodeContext {
				t.Fatalf("Build error = %v, want RuleCodeContext", err)
			}
			if !strings.Contains(err.Error(), tc.message) {
				t.Fatalf("Build error = %q, want to contain %q", err.Error(), tc.message)
			}
		})
	}
}

func TestInvalidDeviationReplaceConstraintOperationsReturnContextRuleCode(t *testing.T) {
	cases := []struct {
		name    string
		target  string
		source  string
		message string
	}{
		{
			name: "replace missing must",
			target: `module cambium-deviation-replace-must-target {
    namespace "urn:cambium:deviation-replace-must-target";
    prefix cdrmt;

    container top;
}`,
			source: `module cambium-deviation-replace-must-source {
    namespace "urn:cambium:deviation-replace-must-source";
    prefix cdrms;

    import cambium-deviation-replace-must-target {
        prefix target;
    }

    deviation "/target:top" {
        deviate replace {
            must "true()";
        }
    }
}`,
			message: `deviate replace must for "top" has no existing must`,
		},
		{
			name: "replace missing unique",
			target: `module cambium-deviation-replace-unique-target {
    namespace "urn:cambium:deviation-replace-unique-target";
    prefix cdrut;

    list item {
        key "id";
        leaf id { type string; }
        leaf code { type string; }
    }
}`,
			source: `module cambium-deviation-replace-unique-source {
    namespace "urn:cambium:deviation-replace-unique-source";
    prefix cdrus;

    import cambium-deviation-replace-unique-target {
        prefix target;
    }

    deviation "/target:item" {
        deviate replace {
            unique "code";
        }
    }
}`,
			message: `deviate replace unique for "item" has no existing unique`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			builder, err := cambium.NewContextBuilder(cambium.ContextFlags{})
			if err != nil {
				t.Fatal(err)
			}
			if err := builder.LoadModuleStr(tc.target); err != nil {
				t.Fatalf("Load target: %v", err)
			}
			if err := builder.LoadModuleStr(tc.source); err != nil {
				t.Fatalf("Load source: %v", err)
			}
			_, err = builder.Build()
			if err == nil {
				t.Fatal("Build accepted invalid deviation constraint replace")
			}
			var ce *cambium.Error
			if !errors.As(err, &ce) || ce.RuleCode() != cambium.RuleCodeContext {
				t.Fatalf("Build error = %v, want RuleCodeContext", err)
			}
			if !strings.Contains(err.Error(), tc.message) {
				t.Fatalf("Build error = %q, want to contain %q", err.Error(), tc.message)
			}
		})
	}
}

func TestInvalidDeviationDeleteConstraintOperationsReturnContextRuleCode(t *testing.T) {
	cases := []struct {
		name    string
		target  string
		source  string
		message string
	}{
		{
			name: "delete missing must",
			target: `module cambium-deviation-delete-must-target {
    namespace "urn:cambium:deviation-delete-must-target";
    prefix cddmt;

    container top {
        must "true()";
    }
}`,
			source: `module cambium-deviation-delete-must-source {
    namespace "urn:cambium:deviation-delete-must-source";
    prefix cddms;

    import cambium-deviation-delete-must-target {
        prefix target;
    }

    deviation "/target:top" {
        deviate delete {
            must "false()";
        }
    }
}`,
			message: `deviate delete must "false()" for "top" does not exist`,
		},
		{
			name: "delete missing unique",
			target: `module cambium-deviation-delete-unique-target {
    namespace "urn:cambium:deviation-delete-unique-target";
    prefix cddut;

    list item {
        key "id";
        unique "code";
        leaf id { type string; }
        leaf code { type string; }
        leaf alt { type string; }
    }
}`,
			source: `module cambium-deviation-delete-unique-source {
    namespace "urn:cambium:deviation-delete-unique-source";
    prefix cddus;

    import cambium-deviation-delete-unique-target {
        prefix target;
    }

    deviation "/target:item" {
        deviate delete {
            unique "alt";
        }
    }
}`,
			message: `deviate delete unique "alt" for "item" does not exist`,
		},
		{
			name: "delete unique leading whitespace",
			target: `module cambium-deviation-delete-unique-leading-target {
    namespace "urn:cambium:deviation-delete-unique-leading-target";
    prefix cddult;

    list item {
        key "id";
        unique "code";
        leaf id { type string; }
        leaf code { type string; }
    }
}`,
			source: `module cambium-deviation-delete-unique-leading-source {
    namespace "urn:cambium:deviation-delete-unique-leading-source";
    prefix cdduls;

    import cambium-deviation-delete-unique-leading-target {
        prefix target;
    }

    deviation "/target:item" {
        deviate delete {
            unique " code";
        }
    }
}`,
			message: `deviate delete unique " code" has invalid identifier list`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			builder, err := cambium.NewContextBuilder(cambium.ContextFlags{})
			if err != nil {
				t.Fatal(err)
			}
			if err := builder.LoadModuleStr(tc.target); err != nil {
				t.Fatalf("Load target: %v", err)
			}
			if err := builder.LoadModuleStr(tc.source); err != nil {
				t.Fatalf("Load source: %v", err)
			}
			_, err = builder.Build()
			if err == nil {
				t.Fatal("Build accepted invalid deviation constraint delete")
			}
			var ce *cambium.Error
			if !errors.As(err, &ce) || ce.RuleCode() != cambium.RuleCodeContext {
				t.Fatalf("Build error = %v, want RuleCodeContext", err)
			}
			if !strings.Contains(err.Error(), tc.message) {
				t.Fatalf("Build error = %q, want to contain %q", err.Error(), tc.message)
			}
		})
	}
}

func TestInvalidListConstraintsReturnContextRuleCode(t *testing.T) {
	cases := []struct {
		name     string
		source   string
		features map[string][]string
		message  string
	}{
		{
			name: "missing key leaf",
			source: `module cambium-list-missing-key {
    namespace "urn:cambium:list-missing-key";
    prefix clmk;
    list item {
        key "id";
        leaf name { type string; }
    }
}`,
			message: `key "id" does not reference a child leaf`,
		},
		{
			name: "empty key statement",
			source: `module cambium-list-empty-key {
    namespace "urn:cambium:list-empty-key";
    prefix clek;
    list item {
        key "";
        leaf id { type string; }
    }
}`,
			message: `list "item" key statement is empty`,
		},
		{
			name: "key unicode whitespace",
			source: `module cambium-key-unicode-whitespace {
    namespace "urn:cambium:key-unicode-whitespace";
    prefix ckuw;
    list item {
        key "a` + "\u2003" + `b";
        leaf a { type string; }
        leaf b { type string; }
    }
}`,
			message: `key "a\u2003b" does not reference a child leaf`,
		},
		{
			name: "key leading whitespace",
			source: `module cambium-key-leading-whitespace {
    namespace "urn:cambium:key-leading-whitespace";
    prefix cklw;
    list item {
        key " id";
        leaf id { type string; }
    }
}`,
			message: `list "item" key statement has invalid identifier list " id"`,
		},
		{
			name: "key trailing whitespace",
			source: `module cambium-key-trailing-whitespace {
    namespace "urn:cambium:key-trailing-whitespace";
    prefix cktw;
    list item {
        key "id ";
        leaf id { type string; }
    }
}`,
			message: `list "item" key statement has invalid identifier list "id "`,
		},
		{
			name: "config true list missing key",
			source: `module cambium-config-true-list-missing-key {
    namespace "urn:cambium:config-true-list-missing-key";
    prefix ctlmk;
    list item {
        leaf id { type string; }
    }
}`,
			message: `config true list "item" must define a key`,
		},
		{
			name: "key on container",
			source: `module cambium-key-on-container {
    namespace "urn:cambium:key-on-container";
    prefix ckoc;
    container item {
        key "id";
        leaf id { type string; }
    }
}`,
			message: `key at`,
		},
		{
			name: "duplicate key leaf",
			source: `module cambium-list-duplicate-key {
    namespace "urn:cambium:list-duplicate-key";
    prefix cldk;
    list item {
        key "id id";
        leaf id { type string; }
    }
}`,
			message: `duplicate key "id"`,
		},
		{
			name: "empty key leaf type",
			source: `module cambium-list-empty-key-type {
    namespace "urn:cambium:list-empty-key-type";
    prefix clekt;
    list item {
        key "id";
        leaf id { type empty; }
    }
}`,
			message: `key leaf "id" with type empty requires yang-version 1.1`,
		},
		{
			name: "key leaf if-feature",
			source: `module cambium-list-key-if-feature {
    namespace "urn:cambium:list-key-if-feature";
    prefix clkif;
    yang-version 1.1;

    feature keyed;

    list item {
        key "id";
        leaf id {
            if-feature keyed;
            type string;
        }
    }
}`,
			message: `key leaf "id" cannot have if-feature statements`,
		},
		{
			name: "grouping key leaf if-feature",
			source: `module cambium-list-grouping-key-if-feature {
    namespace "urn:cambium:list-grouping-key-if-feature";
    prefix clgkif;
    yang-version 1.1;

    feature keyed;

    grouping keyed-fields {
        leaf id {
            if-feature keyed;
            type string;
        }
    }

    list item {
        key "id";
        uses keyed-fields;
    }
}`,
			features: map[string][]string{"cambium-list-grouping-key-if-feature": {"keyed"}},
			message:  `key leaf "id" cannot have if-feature statements`,
		},
		{
			name: "key leaf when",
			source: `module cambium-list-key-when {
    namespace "urn:cambium:list-key-when";
    prefix clkw;

    list item {
        key "id";
        leaf id {
            when "../enabled = 'true'";
            type string;
        }
        leaf enabled {
            type boolean;
        }
    }
}`,
			message: `key leaf "id" cannot have when statements`,
		},
		{
			name: "grouping key leaf uses when",
			source: `module cambium-list-grouping-key-uses-when {
    namespace "urn:cambium:list-grouping-key-uses-when";
    prefix clgkuw;

    grouping keyed-fields {
        leaf id {
            type string;
        }
    }

    list item {
        key "id";
        uses keyed-fields {
            when "../enabled = 'true'";
        }
        leaf enabled {
            type boolean;
        }
    }
}`,
			message: `key leaf "id" cannot have when statements`,
		},
		{
			name: "key leaf config false under config true list",
			source: `module cambium-list-key-config-false {
    namespace "urn:cambium:list-key-config-false";
    prefix clkcf;

    list item {
        key "id";
        leaf id {
            config false;
            type string;
        }
    }
}`,
			message: `key leaf "id" config must match list "item"`,
		},
		{
			name: "refine key leaf config false under config true list",
			source: `module cambium-list-refine-key-config-false {
    namespace "urn:cambium:list-refine-key-config-false";
    prefix clrkcf;

    grouping keyed-list {
        list item {
            key "id";
            leaf id { type string; }
        }
    }

    container top {
        uses keyed-list {
            refine item/id {
                config false;
            }
        }
    }
}`,
			message: `key leaf "id" config must match list "item"`,
		},
		{
			name: "key leaf config true under config false list",
			source: `module cambium-list-key-config-true {
    namespace "urn:cambium:list-key-config-true";
    prefix clkct;

    list item {
        config false;
        key "id";
        leaf id {
            config true;
            type string;
        }
    }
}`,
			message: `config true is not valid under config false ancestor "item"`,
		},
		{
			name: "empty unique leaf type",
			source: `module cambium-list-empty-unique-type {
    namespace "urn:cambium:list-empty-unique-type";
    prefix cleut;
    list item {
        key "id";
        unique "flag";
        leaf id { type string; }
        leaf flag { type empty; }
    }
}`,
			message: `unique leaf "flag" cannot have type empty`,
		},
		{
			name: "mixed config unique leaves",
			source: `module cambium-list-mixed-config-unique {
    namespace "urn:cambium:list-mixed-config-unique";
    prefix clmcu;
    list item {
        key "id";
        unique "rw state";
        leaf id { type string; }
        leaf rw { type string; }
        leaf state {
            config false;
            type string;
        }
    }
}`,
			message: `unique constraint "rw state" mixes config and state leaves`,
		},
		{
			name: "unused grouping empty unique leaf type",
			source: `module cambium-unused-grouping-empty-unique-type {
    namespace "urn:cambium:unused-grouping-empty-unique-type";
    prefix cugut;
    grouping invalid {
        list item {
            key "id";
            unique "flag";
            leaf id { type string; }
            leaf flag { type empty; }
        }
    }
    leaf ok { type string; }
}`,
			message: `unique leaf "flag" cannot have type empty`,
		},
		{
			name: "key references container",
			source: `module cambium-list-key-container {
    namespace "urn:cambium:list-key-container";
    prefix clkc;
    list item {
        key "id";
        container id { leaf value { type string; } }
    }
}`,
			message: `key "id" does not reference a child leaf`,
		},
		{
			name: "missing unique leaf",
			source: `module cambium-list-missing-unique {
    namespace "urn:cambium:list-missing-unique";
    prefix clmu;
    list item {
        key "id";
        unique "missing";
        leaf id { type string; }
    }
}`,
			message: `unique path "missing" does not reference a descendant leaf`,
		},
		{
			name: "unique references nested list leaf",
			source: `module cambium-list-unique-nested-list {
    namespace "urn:cambium:list-unique-nested-list";
    prefix clunl;
    list item {
        key "id";
        unique "nested/inner/code";
        leaf id { type string; }
        container nested {
            list inner {
                key "name";
                leaf name { type string; }
                leaf code { type string; }
            }
        }
    }
}`,
			message: `unique path "nested/inner/code" refers to a leaf in nested list "inner"`,
		},
		{
			name: "empty unique statement",
			source: `module cambium-list-empty-unique {
    namespace "urn:cambium:list-empty-unique";
    prefix cleu;
    list item {
        key "id";
        unique "";
        leaf id { type string; }
    }
}`,
			message: `list "item" unique statement is empty`,
		},
		{
			name: "unique unicode whitespace",
			source: `module cambium-unique-unicode-whitespace {
    namespace "urn:cambium:unique-unicode-whitespace";
    prefix cuuw;
    list item {
        key "id";
        unique "code` + "\u2003" + `tag";
        leaf id { type string; }
        leaf code { type string; }
        leaf tag { type string; }
    }
}`,
			message: `unique path "code\u2003tag" does not reference a descendant leaf`,
		},
		{
			name: "unique leading whitespace",
			source: `module cambium-unique-leading-whitespace {
    namespace "urn:cambium:unique-leading-whitespace";
    prefix culw;
    list item {
        key "id";
        unique " code";
        leaf id { type string; }
        leaf code { type string; }
    }
}`,
			message: `list "item" unique statement has invalid identifier list " code"`,
		},
		{
			name: "unique trailing whitespace",
			source: `module cambium-unique-trailing-whitespace {
    namespace "urn:cambium:unique-trailing-whitespace";
    prefix cutw;
    list item {
        key "id";
        unique "code ";
        leaf id { type string; }
        leaf code { type string; }
    }
}`,
			message: `list "item" unique statement has invalid identifier list "code "`,
		},
		{
			name: "unique on container",
			source: `module cambium-unique-on-container {
    namespace "urn:cambium:unique-on-container";
    prefix cuoc;
    container item {
        unique "id";
        leaf id { type string; }
    }
}`,
			message: `unique at`,
		},
		{
			name: "duplicate unique leaf",
			source: `module cambium-list-duplicate-unique {
    namespace "urn:cambium:list-duplicate-unique";
    prefix cldu;
    list item {
        key "id";
        unique "code code";
        leaf id { type string; }
        leaf code { type string; }
    }
}`,
			message: `unique constraint has duplicate leaf "code"`,
		},
		{
			name: "duplicate unique constraint",
			source: `module cambium-list-duplicate-unique-constraint {
    namespace "urn:cambium:list-duplicate-unique-constraint";
    prefix clduc;
    list item {
        key "id";
        unique "code";
        unique "code";
        leaf id { type string; }
        leaf code { type string; }
    }
}`,
			message: `duplicate unique constraint "code"`,
		},
		{
			name: "unique references container",
			source: `module cambium-list-unique-container {
    namespace "urn:cambium:list-unique-container";
    prefix cluc;
    list item {
        key "id";
        unique "nested";
        leaf id { type string; }
        container nested { leaf code { type string; } }
    }
}`,
			message: `unique path "nested" does not reference a descendant leaf`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			builder, err := cambium.NewContextBuilder(cambium.ContextFlags{})
			if err != nil {
				t.Fatal(err)
			}
			if err := builder.LoadModuleStr(tc.source); err != nil {
				t.Fatalf("LoadModuleStr: %v", err)
			}
			for module, features := range tc.features {
				if err := builder.SetFeatures(module, features); err != nil {
					t.Fatalf("SetFeatures(%s): %v", module, err)
				}
			}
			_, err = builder.Build()
			if err == nil {
				t.Fatal("Build accepted invalid list constraint")
			}
			var ce *cambium.Error
			if !errors.As(err, &ce) || ce.RuleCode() != cambium.RuleCodeContext {
				t.Fatalf("Build error = %v, want RuleCodeContext", err)
			}
			if !strings.Contains(err.Error(), tc.message) {
				t.Fatalf("Build error = %q, want to contain %q", err.Error(), tc.message)
			}
		})
	}
}

func TestInvalidCardinalityConstraintsReturnContextRuleCode(t *testing.T) {
	cases := []struct {
		name    string
		source  string
		message string
	}{
		{
			name: "invalid min-elements",
			source: `module cambium-invalid-min-elements {
    namespace "urn:cambium:invalid-min-elements";
    prefix cime;

    leaf-list servers {
        min-elements many;
        type string;
    }
}`,
			message: `invalid min-elements "many"`,
		},
		{
			name: "min-elements leading zero",
			source: `module cambium-min-elements-leading-zero {
    namespace "urn:cambium:min-elements-leading-zero";
    prefix cmelz;

    leaf-list servers {
        min-elements 01;
        type string;
    }
}`,
			message: `invalid min-elements "01"`,
		},
		{
			name: "invalid max-elements",
			source: `module cambium-invalid-max-elements {
    namespace "urn:cambium:invalid-max-elements";
    prefix cimax;

    leaf-list servers {
        max-elements many;
        type string;
    }
}`,
			message: `invalid max-elements "many"`,
		},
		{
			name: "max-elements leading zero",
			source: `module cambium-max-elements-leading-zero {
    namespace "urn:cambium:max-elements-leading-zero";
    prefix cmxlz;

    leaf-list servers {
        max-elements 01;
        type string;
    }
}`,
			message: `invalid max-elements "01"`,
		},
		{
			name: "zero max-elements",
			source: `module cambium-zero-max-elements {
    namespace "urn:cambium:zero-max-elements";
    prefix czmaxe;

    leaf-list servers {
        max-elements 0;
        type string;
    }
}`,
			message: `invalid max-elements "0"`,
		},
		{
			name: "min exceeds max",
			source: `module cambium-min-exceeds-max {
    namespace "urn:cambium:min-exceeds-max";
    prefix cmem;

    leaf-list servers {
        min-elements 3;
        max-elements 2;
        type string;
    }
}`,
			message: `min-elements 3 exceeds max-elements 2`,
		},
		{
			name: "refine min exceeds max",
			source: `module cambium-refine-min-exceeds-max {
    namespace "urn:cambium:refine-min-exceeds-max";
    prefix crmem;

    grouping server-group {
        leaf-list servers {
            type string;
        }
    }

    container top {
        uses server-group {
            refine servers {
                min-elements 3;
                max-elements 2;
            }
        }
    }
}`,
			message: `min-elements 3 exceeds max-elements 2`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			builder, err := cambium.NewContextBuilder(cambium.ContextFlags{})
			if err != nil {
				t.Fatal(err)
			}
			if err := builder.LoadModuleStr(tc.source); err != nil {
				t.Fatalf("LoadModuleStr: %v", err)
			}
			_, err = builder.Build()
			if err == nil {
				t.Fatal("Build accepted invalid cardinality constraint")
			}
			var ce *cambium.Error
			if !errors.As(err, &ce) || ce.RuleCode() != cambium.RuleCodeContext {
				t.Fatalf("Build error = %v, want RuleCodeContext", err)
			}
			if !strings.Contains(err.Error(), tc.message) {
				t.Fatalf("Build error = %q, want to contain %q", err.Error(), tc.message)
			}
		})
	}
}

func TestInvalidSchemaMetadataValuesReturnContextRuleCode(t *testing.T) {
	target := `module cambium-invalid-metadata-target {
    namespace "urn:cambium:invalid-metadata-target";
    prefix cimt;

    container top {
        leaf value { type string; }
    }
}`
	cases := []struct {
		name    string
		sources []string
		message string
	}{
		{
			name: "invalid status",
			sources: []string{`module cambium-invalid-status {
    namespace "urn:cambium:invalid-status";
    prefix cis;

    leaf value {
        status future;
        type string;
    }
}`},
			message: `invalid status "future"`,
		},
		{
			name: "invalid config",
			sources: []string{`module cambium-invalid-config {
    namespace "urn:cambium:invalid-config";
    prefix cicfg;

    container top {
        config maybe;
    }
}`},
			message: `invalid config "maybe"`,
		},
		{
			name: "config on rpc",
			sources: []string{`module cambium-config-rpc {
    namespace "urn:cambium:config-rpc";
    prefix ccr;

    rpc reset {
        config false;
    }
}`},
			message: `config "false" is not valid under rpc "reset"`,
		},
		{
			name: "config true under config false container",
			sources: []string{`module cambium-config-true-under-false-container {
    namespace "urn:cambium:config-true-under-false-container";
    prefix cctufc;

    container state {
        config false;
        leaf value {
            config true;
            type string;
        }
    }
}`},
			message: `config true is not valid under config false ancestor "state"`,
		},
		{
			name: "grouping config true under config false container",
			sources: []string{`module cambium-grouping-config-true-under-false {
    namespace "urn:cambium:grouping-config-true-under-false";
    prefix cgctuf;

    grouping writable {
        leaf value {
            config true;
            type string;
        }
    }

    container state {
        config false;
        uses writable;
    }
}`},
			message: `config true is not valid under config false ancestor "state"`,
		},
		{
			name: "anydata requires yang 1.1",
			sources: []string{`module cambium-anydata-yang10 {
    namespace "urn:cambium:anydata-yang10";
    prefix cayt;
    anydata payload;
}`},
			message: `anydata "payload" requires yang-version 1.1`,
		},
		{
			name: "action requires yang 1.1",
			sources: []string{`module cambium-action-yang10 {
    namespace "urn:cambium:action-yang10";
    prefix cayt;
    container top {
        action reset;
    }
}`},
			message: `action "reset" requires yang-version 1.1`,
		},
		{
			name: "nested notification requires yang 1.1",
			sources: []string{`module cambium-nested-notification-yang10 {
    namespace "urn:cambium:nested-notification-yang10";
    prefix cnnyt;
    container top {
        notification changed;
    }
}`},
			message: `notification "changed" requires yang-version 1.1 when nested under container "top"`,
		},
		{
			name: "if-feature expression requires yang 1.1",
			sources: []string{`module cambium-if-feature-expression-yang10 {
    namespace "urn:cambium:if-feature-expression-yang10";
    prefix cifeyt;

    feature alpha;
    feature beta;

    container top {
        if-feature "alpha and beta";
    }
}`},
			message: `if-feature expression "alpha and beta" requires yang-version 1.1`,
		},
		{
			name: "enum if-feature requires yang 1.1",
			sources: []string{`module cambium-enum-if-feature-yang10 {
    namespace "urn:cambium:enum-if-feature-yang10";
    prefix ceifyt;

    feature advanced;

    leaf value {
        type enumeration {
            enum basic;
            enum advanced {
                if-feature advanced;
            }
        }
    }
}`},
			message: `if-feature "advanced" under enum "advanced" requires yang-version 1.1`,
		},
		{
			name: "bit if-feature requires yang 1.1",
			sources: []string{`module cambium-bit-if-feature-yang10 {
    namespace "urn:cambium:bit-if-feature-yang10";
    prefix cbifyt;

    feature advanced;

    leaf value {
        type bits {
            bit read;
            bit write {
                if-feature advanced;
            }
        }
    }
}`},
			message: `if-feature "advanced" under bit "write" requires yang-version 1.1`,
		},
		{
			name: "identity if-feature requires yang 1.1",
			sources: []string{`module cambium-identity-if-feature-yang10 {
    namespace "urn:cambium:identity-if-feature-yang10";
    prefix ciifyt;

    feature advanced;

    identity base-id;
    identity advanced-id {
        base base-id;
        if-feature advanced;
    }
}`},
			message: `if-feature "advanced" under identity "advanced-id" requires yang-version 1.1`,
		},
		{
			name: "refine if-feature requires yang 1.1",
			sources: []string{`module cambium-refine-if-feature-yang10 {
    namespace "urn:cambium:refine-if-feature-yang10";
    prefix crifyt;

    feature advanced;

    grouping reusable {
        leaf value { type string; }
    }

    container top {
        uses reusable {
            refine value {
                if-feature advanced;
            }
        }
    }
}`},
			message: `if-feature "advanced" under refine "value" requires yang-version 1.1`,
		},
		{
			name: "pattern modifier requires yang 1.1",
			sources: []string{`module cambium-pattern-modifier-yang10 {
    namespace "urn:cambium:pattern-modifier-yang10";
    prefix cpmyt;

    leaf value {
        type string {
            pattern "[a-z]+" {
                modifier invert-match;
            }
        }
    }
}`},
			message: `pattern modifier "invert-match" requires yang-version 1.1`,
		},
		{
			name: "identity multiple base requires yang 1.1",
			sources: []string{`module cambium-identity-multi-base-yang10 {
    namespace "urn:cambium:identity-multi-base-yang10";
    prefix cimbyt;

    identity base-a;
    identity base-b;
    identity child {
        base base-a;
        base base-b;
    }
}`},
			message: `identity "child" with multiple base statements requires yang-version 1.1`,
		},
		{
			name: "identityref multiple base requires yang 1.1",
			sources: []string{`module cambium-identityref-multi-base-yang10 {
    namespace "urn:cambium:identityref-multi-base-yang10";
    prefix cirmbyt;

    identity base-a;
    identity base-b;

    leaf value {
        type identityref {
            base base-a;
            base base-b;
        }
    }
}`},
			message: `identityref type with multiple base statements requires yang-version 1.1`,
		},
		{
			name: "invalid mandatory",
			sources: []string{`module cambium-invalid-mandatory {
    namespace "urn:cambium:invalid-mandatory";
    prefix cim;

    leaf value {
        mandatory maybe;
        type string;
    }
}`},
			message: `invalid mandatory "maybe"`,
		},
		{
			name: "mandatory on container",
			sources: []string{`module cambium-mandatory-container {
    namespace "urn:cambium:mandatory-container";
    prefix cmc;

    container top {
        mandatory true;
    }
}`},
			message: `mandatory at`,
		},
		{
			name: "invalid ordered-by",
			sources: []string{`module cambium-invalid-ordered-by {
    namespace "urn:cambium:invalid-ordered-by";
    prefix ciob;

    leaf-list values {
        ordered-by random;
        type string;
    }
}`},
			message: `invalid ordered-by "random"`,
		},
		{
			name: "duplicate status",
			sources: []string{`module cambium-duplicate-status {
    namespace "urn:cambium:duplicate-status";
    prefix cds;

    leaf value {
        status current;
        status deprecated;
        type string;
    }
}`},
			message: `schema node "value" has multiple status statements`,
		},
		{
			name: "status on input",
			sources: []string{`module cambium-status-input {
    namespace "urn:cambium:status-input";
    prefix csi;

    rpc reset {
        input {
            status deprecated;
            leaf id { type string; }
        }
    }
}`},
			message: `status "deprecated" is not valid under input`,
		},
		{
			name: "duplicate config",
			sources: []string{`module cambium-duplicate-config {
    namespace "urn:cambium:duplicate-config";
    prefix cdc;

    container top {
        config true;
        config false;
    }
}`},
			message: `schema node "top" has multiple config statements`,
		},
		{
			name: "duplicate mandatory",
			sources: []string{`module cambium-duplicate-mandatory {
    namespace "urn:cambium:duplicate-mandatory";
    prefix cdm;

    leaf value {
        mandatory true;
        mandatory false;
        type string;
    }
}`},
			message: `schema node "value" has multiple mandatory statements`,
		},
		{
			name: "duplicate ordered-by",
			sources: []string{`module cambium-duplicate-ordered-by {
    namespace "urn:cambium:duplicate-ordered-by";
    prefix cdob;

    leaf-list values {
        ordered-by system;
        ordered-by user;
        type string;
    }
}`},
			message: `schema node "values" has multiple ordered-by statements`,
		},
		{
			name: "duplicate presence",
			sources: []string{`module cambium-duplicate-presence {
    namespace "urn:cambium:duplicate-presence";
    prefix cdp;

    container top {
        presence "one";
        presence "two";
    }
}`},
			message: `schema node "top" has multiple presence statements`,
		},
		{
			name: "duplicate key statement",
			sources: []string{`module cambium-duplicate-key-statement {
    namespace "urn:cambium:duplicate-key-statement";
    prefix cdks;

    list item {
        key "id";
        key "other";
        leaf id { type string; }
        leaf other { type string; }
    }
}`},
			message: `schema node "item" has multiple key statements`,
		},
		{
			name: "duplicate when statement",
			sources: []string{`module cambium-duplicate-when-statement {
    namespace "urn:cambium:duplicate-when-statement";
    prefix cdws;

    container top {
        leaf enabled { type boolean; }
        leaf value {
            when "../enabled = 'true'";
            when "../enabled = 'false'";
            type string;
        }
    }
}`},
			message: `schema node "value" has multiple when statements`,
		},
		{
			name: "when empty xpath",
			sources: []string{`module cambium-when-empty-xpath {
    namespace "urn:cambium:when-empty-xpath";
    prefix cwex;

    leaf value {
        when "";
        type string;
    }
}`},
			message: `when expression is empty`,
		},
		{
			name: "when extra closing paren",
			sources: []string{`module cambium-when-extra-closing-paren {
    namespace "urn:cambium:when-extra-closing-paren";
    prefix cwecp;

    leaf enabled { type boolean; }
    leaf value {
        when "../enabled = 'true')";
        type string;
    }
}`},
			message: `invalid when expression`,
		},
		{
			name: "when unclosed quote",
			sources: []string{`module cambium-when-unclosed-quote {
    namespace "urn:cambium:when-unclosed-quote";
    prefix cwuq;

    leaf enabled { type boolean; }
    leaf value {
        when "../enabled = 'true";
        type string;
    }
}`},
			message: `invalid when expression`,
		},
		{
			name: "when leading operator",
			sources: []string{`module cambium-when-leading-operator {
    namespace "urn:cambium:when-leading-operator";
    prefix cwlo;

    leaf enabled { type boolean; }
    leaf value {
        when "and ../enabled";
        type string;
    }
}`},
			message: `invalid when expression`,
		},
		{
			name: "when double operator",
			sources: []string{`module cambium-when-double-operator {
    namespace "urn:cambium:when-double-operator";
    prefix cwdo;

    leaf enabled { type boolean; }
    leaf ready { type boolean; }
    leaf value {
        when "../enabled and or ../ready";
        type string;
    }
}`},
			message: `invalid when expression`,
		},
		{
			name: "when contains missing argument",
			sources: []string{`module cambium-when-contains-missing-argument {
    namespace "urn:cambium:when-contains-missing-argument";
    prefix cwcma;

    leaf value {
        when "contains('abc')";
        type string;
    }
}`},
			message: `invalid when expression`,
		},
		{
			name: "when true too many arguments",
			sources: []string{`module cambium-when-true-too-many-arguments {
    namespace "urn:cambium:when-true-too-many-arguments";
    prefix cwttma;

    leaf enabled { type boolean; }
    leaf value {
        when "true(../enabled)";
        type string;
    }
}`},
			message: `invalid when expression`,
		},
		{
			name: "when on rpc",
			sources: []string{`module cambium-when-rpc {
    namespace "urn:cambium:when-rpc";
    prefix cwr;

    rpc reset {
        when "true()";
    }
}`},
			message: `when "true()" is not valid under rpc "reset"`,
		},
		{
			name: "input under container",
			sources: []string{`module cambium-input-under-container {
    namespace "urn:cambium:input-under-container";
    prefix ciuc;

    container top {
        input {
            leaf value { type string; }
        }
    }
}`},
			message: `input at`,
		},
		{
			name: "action at module top level",
			sources: []string{`module cambium-top-level-action {
    namespace "urn:cambium:top-level-action";
    prefix ctla;

    action reset;
}`},
			message: `action at`,
		},
		{
			name: "input at module top level",
			sources: []string{`module cambium-top-level-input {
    namespace "urn:cambium:top-level-input";
    prefix ctli;

    input {
        leaf value { type string; }
    }
}`},
			message: `input at`,
		},
		{
			name: "output at module top level",
			sources: []string{`module cambium-top-level-output {
    namespace "urn:cambium:top-level-output";
    prefix ctlo;

    output {
        leaf value { type string; }
    }
}`},
			message: `output at`,
		},
		{
			name: "type at module top level",
			sources: []string{`module cambium-top-level-type {
    namespace "urn:cambium:top-level-type";
    prefix ctlt;

    type string;
}`},
			message: `type at`,
		},
		{
			name: "duplicate rpc input",
			sources: []string{`module cambium-duplicate-rpc-input {
    namespace "urn:cambium:duplicate-rpc-input";
    prefix cdri;

    rpc reset {
        input {
            leaf id { type string; }
        }
        input {
            leaf name { type string; }
        }
    }
}`},
			message: `rpc "reset" has multiple input statements`,
		},
		{
			name: "duplicate action output",
			sources: []string{`module cambium-duplicate-action-output {
    namespace "urn:cambium:duplicate-action-output";
    prefix cdao;

    container top {
        action reset {
            output {
                leaf id { type string; }
            }
            output {
                leaf name { type string; }
            }
        }
    }
}`},
			message: `action "reset" has multiple output statements`,
		},
		{
			name: "if-feature under rpc input",
			sources: []string{`module cambium-if-feature-under-input {
    namespace "urn:cambium:if-feature-under-input";
    prefix cifi;

    feature advanced;

    rpc reset {
        input {
            if-feature advanced;
            leaf id { type string; }
        }
    }
}`},
			message: `if-feature "advanced" is not valid under input`,
		},
		{
			name: "if-feature under action output",
			sources: []string{`module cambium-if-feature-under-output {
    namespace "urn:cambium:if-feature-under-output";
    prefix cifo;

    feature advanced;

    container top {
        action reset {
            output {
                if-feature advanced;
                leaf result { type string; }
            }
        }
    }
}`},
			message: `if-feature "advanced" is not valid under output`,
		},
		{
			name: "action under leaf",
			sources: []string{`module cambium-action-under-leaf {
    namespace "urn:cambium:action-under-leaf";
    prefix caul;

    leaf value {
        type string;
        action reset;
    }
}`},
			message: `action at`,
		},
		{
			name: "action under rpc input",
			sources: []string{`module cambium-action-under-rpc-input {
    namespace "urn:cambium:action-under-rpc-input";
    prefix cauri;

    rpc reset {
        input {
            action nested;
        }
    }
}`},
			message: `action "nested" is not valid under input`,
		},
		{
			name: "grouping action under rpc",
			sources: []string{`module cambium-grouping-action-under-rpc {
    namespace "urn:cambium:grouping-action-under-rpc";
    prefix cgaur;

    grouping reusable {
        container payload {
            action nested;
        }
    }

    rpc reset {
        input {
            uses reusable;
        }
    }
}`},
			message: `action "nested" cannot have rpc ancestor "reset"`,
		},
		{
			name: "action under keyless list",
			sources: []string{`module cambium-action-under-keyless-list {
    namespace "urn:cambium:action-under-keyless-list";
    prefix caukl;

    list item {
        config false;
        leaf name { type string; }
        action reset;
    }
}`},
			message: `action "reset" cannot have keyless list ancestor "item"`,
		},
		{
			name: "rpc under container",
			sources: []string{`module cambium-rpc-under-container {
    namespace "urn:cambium:rpc-under-container";
    prefix cruc;

    container top {
        rpc reset;
    }
}`},
			message: `rpc at`,
		},
		{
			name: "case under container",
			sources: []string{`module cambium-case-under-container {
    namespace "urn:cambium:case-under-container";
    prefix ccuc;

    container top {
        case active {
            leaf value { type string; }
        }
    }
}`},
			message: `case at`,
		},
		{
			name: "notification under leaf",
			sources: []string{`module cambium-notification-under-leaf {
    namespace "urn:cambium:notification-under-leaf";
    prefix cnul;

    leaf value {
        type string;
        notification changed;
    }
}`},
			message: `notification at`,
		},
		{
			name: "notification under keyless list ancestor",
			sources: []string{`module cambium-notification-under-keyless-list {
    namespace "urn:cambium:notification-under-keyless-list";
    prefix cnukl;

    list item {
        config false;
        container state {
            notification changed;
        }
    }
}`},
			message: `notification "changed" cannot have keyless list ancestor "item"`,
		},
		{
			name: "grouping notification under rpc",
			sources: []string{`module cambium-grouping-notification-under-rpc {
    namespace "urn:cambium:grouping-notification-under-rpc";
    prefix cgnur;

    grouping reusable {
        container payload {
            notification changed;
        }
    }

    rpc reset {
        input {
            uses reusable;
        }
    }
}`},
			message: `notification "changed" cannot have rpc ancestor "reset"`,
		},
		{
			name: "container under leaf",
			sources: []string{`module cambium-container-under-leaf {
    namespace "urn:cambium:container-under-leaf";
    prefix ccul;

    leaf value {
        type string;
        container nested;
    }
}`},
			message: `container at`,
		},
		{
			name: "leaf directly under rpc",
			sources: []string{`module cambium-leaf-under-rpc {
    namespace "urn:cambium:leaf-under-rpc";
    prefix clur;

    rpc reset {
        leaf value {
            type string;
        }
    }
}`},
			message: `leaf "value" is not valid under rpc "reset"`,
		},
		{
			name: "must directly under rpc",
			sources: []string{`module cambium-must-under-rpc {
    namespace "urn:cambium:must-under-rpc";
    prefix cmur;

    rpc reset {
        must "true()";
    }
}`},
			message: `must "true()" is not valid under rpc "reset"`,
		},
		{
			name: "must directly under action",
			sources: []string{`module cambium-must-under-action {
    namespace "urn:cambium:must-under-action";
    prefix cmua;

    container top {
        action reset {
            must "true()";
        }
    }
}`},
			message: `must "true()" is not valid under action "reset"`,
		},
		{
			name: "must directly under notification",
			sources: []string{`module cambium-must-under-notification {
    namespace "urn:cambium:must-under-notification";
    prefix cmun;

    notification changed {
        must "true()";
    }
}`},
			message: `must "true()" under notification "changed" requires yang-version 1.1`,
		},
		{
			name: "must under rpc input requires yang 1.1",
			sources: []string{`module cambium-must-under-rpc-input {
    namespace "urn:cambium:must-under-rpc-input";
    prefix cmuri;

    rpc reset {
        input {
            must "true()";
            leaf id {
                type string;
            }
        }
    }
}`},
			message: `must "true()" under input requires yang-version 1.1`,
		},
		{
			name: "must under rpc output requires yang 1.1",
			sources: []string{`module cambium-must-under-rpc-output {
    namespace "urn:cambium:must-under-rpc-output";
    prefix cmuro;

    rpc reset {
        output {
            must "true()";
            leaf result {
                type string;
            }
        }
    }
}`},
			message: `must "true()" under output requires yang-version 1.1`,
		},
		{
			name: "uses under leaf",
			sources: []string{`module cambium-uses-under-leaf {
    namespace "urn:cambium:uses-under-leaf";
    prefix cuul;

    grouping empty;
    leaf value {
        type string;
        uses empty;
    }
}`},
			message: `uses at`,
		},
		{
			name: "uses directly under rpc",
			sources: []string{`module cambium-uses-under-rpc {
    namespace "urn:cambium:uses-under-rpc";
    prefix cuur;

    grouping empty;
    rpc reset {
        uses empty;
    }
}`},
			message: `uses "empty" is not valid under rpc "reset"`,
		},
		{
			name: "uses directly under choice",
			sources: []string{`module cambium-uses-under-choice {
    namespace "urn:cambium:uses-under-choice";
    prefix cuuc;

    grouping empty;
    choice selector {
        uses empty;
    }
}`},
			message: `uses at`,
		},
		{
			name: "uses duplicate when",
			sources: []string{`module cambium-uses-duplicate-when {
    namespace "urn:cambium:uses-duplicate-when";
    prefix cudw;

    grouping reusable {
        leaf value { type string; }
    }

    container top {
        leaf enabled { type boolean; }
        uses reusable {
            when "../enabled = 'true'";
            when "../enabled = 'false'";
        }
    }
}`},
			message: `uses "reusable" has multiple when statements`,
		},
		{
			name: "choice directly under choice",
			sources: []string{`module cambium-choice-under-choice {
    namespace "urn:cambium:choice-under-choice";
    prefix ccuc;

    container top {
        choice selector {
            choice nested {
                leaf value { type string; }
            }
        }
    }
}`},
			message: `choice "nested" under choice "selector" requires yang-version 1.1`,
		},
		{
			name: "must directly under case",
			sources: []string{`module cambium-must-under-case {
    namespace "urn:cambium:must-under-case";
    prefix cmuc;

    container top {
        choice selector {
            case active {
                must "true()";
                leaf value { type string; }
            }
        }
    }
}`},
			message: `must "true()" is not valid under case "active"`,
		},
		{
			name: "must empty xpath",
			sources: []string{`module cambium-must-empty-xpath {
    namespace "urn:cambium:must-empty-xpath";
    prefix cmex;

    container top {
        must "";
        leaf value { type string; }
    }
}`},
			message: `must expression is empty`,
		},
		{
			name: "must unclosed paren",
			sources: []string{`module cambium-must-unclosed-paren {
    namespace "urn:cambium:must-unclosed-paren";
    prefix cmup;

    container top {
        must "count(../value";
        leaf value { type string; }
    }
}`},
			message: `invalid must expression`,
		},
		{
			name: "must unclosed predicate",
			sources: []string{`module cambium-must-unclosed-predicate {
    namespace "urn:cambium:must-unclosed-predicate";
    prefix cmupred;

    container top {
        must "../value[";
        leaf value { type string; }
    }
}`},
			message: `invalid must expression`,
		},
		{
			name: "must trailing operator",
			sources: []string{`module cambium-must-trailing-operator {
    namespace "urn:cambium:must-trailing-operator";
    prefix cmto;

    container top {
        must "../value =";
        leaf value { type string; }
    }
}`},
			message: `invalid must expression`,
		},
		{
			name: "must empty function arg",
			sources: []string{`module cambium-must-empty-function-arg {
    namespace "urn:cambium:must-empty-function-arg";
    prefix cmefa;

    container top {
        must "count() > 0";
        leaf value { type string; }
    }
}`},
			message: `invalid must expression`,
		},
		{
			name: "must invalid axis",
			sources: []string{`module cambium-must-invalid-axis {
    namespace "urn:cambium:must-invalid-axis";
    prefix cmia;

    container top {
        must "bad-axis::value";
        leaf value { type string; }
    }
}`},
			message: `invalid must expression`,
		},
		{
			name: "must unknown function",
			sources: []string{`module cambium-must-unknown-function {
    namespace "urn:cambium:must-unknown-function";
    prefix cmuf;

    container top {
        must "unknown-fn(../value)";
        leaf value { type string; }
    }
}`},
			message: `invalid must expression`,
		},
		{
			name: "must count too many arguments",
			sources: []string{`module cambium-must-count-too-many-arguments {
    namespace "urn:cambium:must-count-too-many-arguments";
    prefix cmctma;

    container top {
        must "count(../a, ../b) > 0";
        leaf a { type string; }
        leaf b { type string; }
    }
}`},
			message: `invalid must expression`,
		},
		{
			name: "must string length too many arguments",
			sources: []string{`module cambium-must-string-length-too-many-arguments {
    namespace "urn:cambium:must-string-length-too-many-arguments";
    prefix cmslma;

    container top {
        must "string-length('abc', 'def') > 0";
        leaf value { type string; }
    }
}`},
			message: `invalid must expression`,
		},
		{
			name: "duplicate must error message",
			sources: []string{`module cambium-duplicate-must-error-message {
    namespace "urn:cambium:duplicate-must-error-message";
    prefix cdmem;

    container top {
        must "true()" {
            error-message "one";
            error-message "two";
        }
    }
}`},
			message: `must "true()" has multiple error-message statements`,
		},
		{
			name: "must invalid child",
			sources: []string{`module cambium-must-invalid-child {
    namespace "urn:cambium:must-invalid-child";
    prefix cmic;

    container top {
        must "true()" {
            default "bad";
        }
    }
}`},
			message: `default "bad" is not valid under must "true()"`,
		},
		{
			name: "must on choice",
			sources: []string{`module cambium-must-choice {
    namespace "urn:cambium:must-choice";
    prefix cmc;

    container top {
        choice mode {
            must "true()";
            leaf a { type string; }
        }
    }
}`},
			message: `must "true()" is not valid under choice "mode"`,
		},
		{
			name: "duplicate when description",
			sources: []string{`module cambium-duplicate-when-description {
    namespace "urn:cambium:duplicate-when-description";
    prefix cdwd;

    container top {
        leaf enabled { type boolean; }
        leaf value {
            when "../enabled = 'true'" {
                description "one";
                description "two";
            }
            type string;
        }
    }
}`},
			message: `when "../enabled = 'true'" has multiple description statements`,
		},
		{
			name: "when invalid child",
			sources: []string{`module cambium-when-invalid-child {
    namespace "urn:cambium:when-invalid-child";
    prefix cwic;

    container top {
        leaf enabled { type boolean; }
        leaf value {
            when "../enabled = 'true'" {
                error-message "bad";
            }
            type string;
        }
    }
}`},
			message: `error-message "bad" is not valid under when "../enabled = 'true'"`,
		},
		{
			name: "duplicate description",
			sources: []string{`module cambium-duplicate-description {
    namespace "urn:cambium:duplicate-description";
    prefix cdd;

    leaf value {
        description "one";
        description "two";
        type string;
    }
}`},
			message: `schema node "value" has multiple description statements`,
		},
		{
			name: "description on input",
			sources: []string{`module cambium-description-input {
    namespace "urn:cambium:description-input";
    prefix cdi;

    rpc reset {
        input {
            description "bad";
            leaf id { type string; }
        }
    }
}`},
			message: `description "bad" is not valid under input`,
		},
		{
			name: "duplicate reference",
			sources: []string{`module cambium-duplicate-reference {
    namespace "urn:cambium:duplicate-reference";
    prefix cdr;

    leaf value {
        reference "one";
        reference "two";
        type string;
    }
}`},
			message: `schema node "value" has multiple reference statements`,
		},
		{
			name: "reference on input",
			sources: []string{`module cambium-reference-input {
    namespace "urn:cambium:reference-input";
    prefix cri;

    rpc reset {
        input {
            reference "bad";
            leaf id { type string; }
        }
    }
}`},
			message: `reference "bad" is not valid under input`,
		},
		{
			name: "duplicate units",
			sources: []string{`module cambium-duplicate-units {
    namespace "urn:cambium:duplicate-units";
    prefix cdu;

    leaf value {
        units "one";
        units "two";
        type string;
    }
}`},
			message: `schema node "value" has multiple units statements`,
		},
		{
			name: "units on container",
			sources: []string{`module cambium-units-on-container {
    namespace "urn:cambium:units-on-container";
    prefix cuoc;

    container top {
        units "meters";
    }
}`},
			message: `units at`,
		},
		{
			name: "presence on leaf",
			sources: []string{`module cambium-presence-leaf {
    namespace "urn:cambium:presence-leaf";
    prefix cpl;

    leaf value {
        type string;
        presence "bad";
    }
}`},
			message: `presence at`,
		},
		{
			name: "invalid refine mandatory",
			sources: []string{`module cambium-invalid-refine-mandatory {
    namespace "urn:cambium:invalid-refine-mandatory";
    prefix cirm;

    grouping reusable {
        leaf value { type string; }
    }

    container top {
        uses reusable {
            refine value {
                mandatory maybe;
            }
        }
    }
}`},
			message: `invalid mandatory "maybe"`,
		},
		{
			name: "duplicate refine mandatory",
			sources: []string{`module cambium-duplicate-refine-mandatory {
    namespace "urn:cambium:duplicate-refine-mandatory";
    prefix cdrm;

    grouping reusable {
        leaf value { type string; }
    }

    container top {
        uses reusable {
            refine value {
                mandatory true;
                mandatory false;
            }
        }
    }
}`},
			message: `schema node "value" has multiple mandatory statements`,
		},
		{
			name: "refine mandatory on container",
			sources: []string{`module cambium-refine-mandatory-container {
    namespace "urn:cambium:refine-mandatory-container";
    prefix crmc;

    grouping reusable {
        container nested;
    }

    container top {
        uses reusable {
            refine nested {
                mandatory true;
            }
        }
    }
}`},
			message: `mandatory at`,
		},
		{
			name: "duplicate refine config",
			sources: []string{`module cambium-duplicate-refine-config {
    namespace "urn:cambium:duplicate-refine-config";
    prefix cdrc;

    grouping reusable {
        container nested;
    }

    container top {
        uses reusable {
            refine nested {
                config true;
                config false;
            }
        }
    }
}`},
			message: `schema node "nested" has multiple config statements`,
		},
		{
			name: "duplicate refine presence",
			sources: []string{`module cambium-duplicate-refine-presence {
    namespace "urn:cambium:duplicate-refine-presence";
    prefix cdrp;

    grouping reusable {
        container nested;
    }

    container top {
        uses reusable {
            refine nested {
                presence "one";
                presence "two";
            }
        }
    }
}`},
			message: `schema node "nested" has multiple presence statements`,
		},
		{
			name: "refine presence on leaf",
			sources: []string{`module cambium-refine-presence-leaf {
    namespace "urn:cambium:refine-presence-leaf";
    prefix crpl;

    grouping reusable {
        leaf value { type string; }
    }

    container top {
        uses reusable {
            refine value {
                presence "bad";
            }
        }
    }
}`},
			message: `presence at`,
		},
		{
			name: "refine must on choice",
			sources: []string{`module cambium-refine-must-choice {
    namespace "urn:cambium:refine-must-choice";
    prefix crmc;

    grouping reusable {
        choice mode {
            leaf a { type string; }
        }
    }

    container top {
        uses reusable {
            refine mode {
                must "true()";
            }
        }
    }
}`},
			message: `must at`,
		},
		{
			name: "invalid deviation config",
			sources: []string{target, `module cambium-invalid-metadata-source {
    namespace "urn:cambium:invalid-metadata-source";
    prefix cims;

    import cambium-invalid-metadata-target {
        prefix target;
    }

    deviation "/target:top" {
        deviate replace {
            config maybe;
        }
    }
}`},
			message: `invalid config "maybe"`,
		},
		{
			name: "deviation config on rpc",
			sources: []string{`module cambium-deviation-config-rpc-target {
    namespace "urn:cambium:deviation-config-rpc-target";
    prefix cdcrt;

    rpc reset;
}`, `module cambium-deviation-config-rpc-source {
    namespace "urn:cambium:deviation-config-rpc-source";
    prefix cdcrs;

    import cambium-deviation-config-rpc-target {
        prefix target;
    }

    deviation "/target:reset" {
        deviate replace {
            config false;
        }
    }
}`},
			message: `config at`,
		},
		{
			name: "deviation mandatory on container",
			sources: []string{target, `module cambium-deviation-mandatory-container {
    namespace "urn:cambium:deviation-mandatory-container";
    prefix cdmc;

    import cambium-invalid-metadata-target {
        prefix target;
    }

    deviation "/target:top" {
        deviate replace {
            mandatory true;
        }
    }
}`},
			message: `mandatory at`,
		},
		{
			name: "deviation must on choice",
			sources: []string{`module cambium-deviation-must-choice-target {
    namespace "urn:cambium:deviation-must-choice-target";
    prefix cdmct;

    container top {
        choice mode {
            leaf a { type string; }
        }
    }
}`, `module cambium-deviation-must-choice-source {
    namespace "urn:cambium:deviation-must-choice-source";
    prefix cdmcs;

    import cambium-deviation-must-choice-target {
        prefix target;
    }

    deviation "/target:top/target:mode" {
        deviate add {
            must "true()";
        }
    }
}`},
			message: `must at`,
		},
		{
			name: "duplicate deviation must error message",
			sources: []string{target, `module cambium-duplicate-deviation-must-error-message {
    namespace "urn:cambium:duplicate-deviation-must-error-message";
    prefix cddmem;

    import cambium-invalid-metadata-target {
        prefix target;
    }

    deviation "/target:top" {
        deviate add {
            must "true()" {
                error-message "one";
                error-message "two";
            }
        }
    }
}`},
			message: `must "true()" has multiple error-message statements`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			builder, err := cambium.NewContextBuilder(cambium.ContextFlags{})
			if err != nil {
				t.Fatal(err)
			}
			for _, source := range tc.sources {
				if err := builder.LoadModuleStr(source); err != nil {
					t.Fatalf("LoadModuleStr: %v", err)
				}
			}
			_, err = builder.Build()
			if err == nil {
				t.Fatal("Build accepted invalid schema metadata")
			}
			var ce *cambium.Error
			if !errors.As(err, &ce) || ce.RuleCode() != cambium.RuleCodeContext {
				t.Fatalf("Build error = %v, want RuleCodeContext", err)
			}
			if !strings.Contains(err.Error(), tc.message) {
				t.Fatalf("Build error = %q, want to contain %q", err.Error(), tc.message)
			}
		})
	}
}

func TestInvalidDefaultStatementsReturnContextRuleCode(t *testing.T) {
	cases := []struct {
		name     string
		source   string
		features map[string][]string
		message  string
	}{
		{
			name: "leaf multiple defaults",
			source: `module cambium-leaf-multiple-defaults {
    namespace "urn:cambium:leaf-multiple-defaults";
    prefix clmd;

    leaf value {
        type string;
        default "a";
        default "b";
    }
}`,
			message: `leaf "value" has multiple default statements`,
		},
		{
			name: "mandatory leaf default",
			source: `module cambium-mandatory-leaf-default {
    namespace "urn:cambium:mandatory-leaf-default";
    prefix cmld;

    leaf value {
        mandatory true;
        type string;
        default "a";
    }
}`,
			message: `mandatory leaf "value" cannot have a default`,
		},
		{
			name: "invalid boolean default",
			source: `module cambium-invalid-boolean-default {
    namespace "urn:cambium:invalid-boolean-default";
    prefix cibd;

    leaf value {
        type boolean;
        default maybe;
    }
}`,
			message: `default "maybe" is not valid for boolean leaf "value"`,
		},
		{
			name: "invalid uint8 default",
			source: `module cambium-invalid-uint8-default {
    namespace "urn:cambium:invalid-uint8-default";
    prefix ciud;

    leaf value {
        type uint8;
        default 256;
    }
}`,
			message: `default "256" is not valid for uint8 leaf "value"`,
		},
		{
			name: "invalid decimal64 default",
			source: `module cambium-invalid-decimal64-default {
    namespace "urn:cambium:invalid-decimal64-default";
    prefix cidd;

    leaf value {
        type decimal64 {
            fraction-digits 1;
        }
        default 1.23;
    }
}`,
			message: `default "1.23" is not valid for decimal64 leaf "value"`,
		},
		{
			name: "invalid enum default",
			source: `module cambium-invalid-enum-default {
    namespace "urn:cambium:invalid-enum-default";
    prefix cied;

    leaf value {
        type enumeration {
            enum up;
        }
        default down;
    }
}`,
			message: `default "down" is not valid for enumeration leaf "value"`,
		},
		{
			name: "feature gated enum default",
			source: `module cambium-feature-gated-enum-default {
    namespace "urn:cambium:feature-gated-enum-default";
    prefix cfged;
    yang-version 1.1;

    feature advanced;

    leaf color {
        type enumeration {
            enum blue {
                if-feature advanced;
            }
            enum red;
        }
        default blue;
    }
}`,
			features: map[string][]string{"cambium-feature-gated-enum-default": {"advanced"}},
			message:  `default "blue" references enum "blue" marked with if-feature`,
		},
		{
			name: "feature gated enum leaf-list default",
			source: `module cambium-feature-gated-enum-leaf-list-default {
    namespace "urn:cambium:feature-gated-enum-leaf-list-default";
    prefix cfgelld;
    yang-version 1.1;

    feature advanced;

    leaf-list colors {
        type enumeration {
            enum blue {
                if-feature advanced;
            }
            enum red;
        }
        default blue;
    }
}`,
			features: map[string][]string{"cambium-feature-gated-enum-leaf-list-default": {"advanced"}},
			message:  `default "blue" references enum "blue" marked with if-feature`,
		},
		{
			name: "invalid bits default",
			source: `module cambium-invalid-bits-default {
    namespace "urn:cambium:invalid-bits-default";
    prefix cibd;

    leaf value {
        type bits {
            bit read;
        }
        default "read write";
    }
}`,
			message: `default "read write" is not valid for bits leaf "value"`,
		},
		{
			name: "invalid bits default unicode whitespace",
			source: `module cambium-invalid-bits-default-unicode-whitespace {
    namespace "urn:cambium:invalid-bits-default-unicode-whitespace";
    prefix cibduw;

    leaf value {
        type bits {
            bit read;
            bit write;
        }
        default "read` + "\u2003" + `write";
    }
}`,
			message: `default "read\u2003write" is not valid for bits leaf "value"`,
		},
		{
			name: "feature gated bits default",
			source: `module cambium-feature-gated-bits-default {
    namespace "urn:cambium:feature-gated-bits-default";
    prefix cfgbd;
    yang-version 1.1;

    feature advanced;

    leaf flags {
        type bits {
            bit read {
                if-feature advanced;
            }
            bit write;
        }
        default "read";
    }
}`,
			features: map[string][]string{"cambium-feature-gated-bits-default": {"advanced"}},
			message:  `default "read" references bit "read" marked with if-feature`,
		},
		{
			name: "feature gated bits leaf-list default",
			source: `module cambium-feature-gated-bits-leaf-list-default {
    namespace "urn:cambium:feature-gated-bits-leaf-list-default";
    prefix cfgblld;
    yang-version 1.1;

    feature advanced;

    leaf-list flags {
        type bits {
            bit read {
                if-feature advanced;
            }
            bit write;
        }
        default "read";
    }
}`,
			features: map[string][]string{"cambium-feature-gated-bits-leaf-list-default": {"advanced"}},
			message:  `default "read" references bit "read" marked with if-feature`,
		},
		{
			name: "invalid string length default",
			source: `module cambium-invalid-string-length-default {
    namespace "urn:cambium:invalid-string-length-default";
    prefix cisld;

    leaf value {
        type string {
            length "2";
        }
        default "a";
    }
}`,
			message: `default "a" does not satisfy length for string leaf "value"`,
		},
		{
			name: "invalid string pattern default",
			source: `module cambium-invalid-string-pattern-default {
    namespace "urn:cambium:invalid-string-pattern-default";
    prefix cispd;

    leaf value {
        type string {
            pattern "a+";
        }
        default "bbb";
    }
}`,
			message: `default "bbb" does not satisfy pattern for string leaf "value"`,
		},
		{
			name: "invalid binary default",
			source: `module cambium-invalid-binary-default {
    namespace "urn:cambium:invalid-binary-default";
    prefix cibd;

    leaf value {
        type binary;
        default "@";
    }
}`,
			message: `default "@" is not valid for binary leaf "value"`,
		},
		{
			name: "invalid binary default newline",
			source: `module cambium-invalid-binary-default-newline {
    namespace "urn:cambium:invalid-binary-default-newline";
    prefix cibdn;

    leaf value {
        type binary;
        default "AQID` + "\n" + `BAUG";
    }
}`,
			message: `default "AQID\nBAUG" is not valid for binary leaf "value"`,
		},
		{
			name: "invalid identityref default",
			source: `module cambium-invalid-identityref-default {
    namespace "urn:cambium:invalid-identityref-default";
    prefix ciid;

    identity root;
    identity other;

    leaf value {
        type identityref {
            base root;
        }
        default other;
    }
}`,
			message: `default "other" is not valid for identityref leaf "value"`,
		},
		{
			name: "empty default",
			source: `module cambium-empty-default {
    namespace "urn:cambium:empty-default";
    prefix ced;

    leaf value {
        type empty;
        default "";
    }
}`,
			message: `empty leaf "value" cannot have a default`,
		},
		{
			name: "invalid union default",
			source: `module cambium-invalid-union-default {
    namespace "urn:cambium:invalid-union-default";
    prefix ciud;

    leaf value {
        type union {
            type boolean;
            type uint8;
        }
        default maybe;
    }
}`,
			message: `default "maybe" is not valid for union leaf "value"`,
		},
		{
			name: "invalid leafref default",
			source: `module cambium-invalid-leafref-default {
    namespace "urn:cambium:invalid-leafref-default";
    prefix cild;

    leaf target {
        type uint8;
    }

    leaf ref {
        type leafref {
            path "/target";
        }
        default 999;
    }
}`,
			message: `default "999" is not valid for uint8 leaf "ref"`,
		},
		{
			name: "key leaf default",
			source: `module cambium-key-leaf-default {
    namespace "urn:cambium:key-leaf-default";
    prefix ckld;

    list item {
        key "id";
        leaf id {
            type string;
            default "a";
        }
    }
}`,
			message: `key leaf "id" cannot have a default`,
		},
		{
			name: "leaf-list duplicate defaults",
			source: `module cambium-leaf-list-duplicate-defaults {
    namespace "urn:cambium:leaf-list-duplicate-defaults";
    prefix clldd;

    leaf-list values {
        type string;
        default "a";
        default "a";
    }
}`,
			message: `leaf-list "values" has duplicate default "a"`,
		},
		{
			name: "leaf-list default requires yang 1.1",
			source: `module cambium-leaf-list-default-yang10 {
    namespace "urn:cambium:leaf-list-default-yang10";
    prefix clldyt;

    leaf-list values {
        type string;
        default "a";
    }
}`,
			message: `leaf-list "values" default statements require yang-version 1.1`,
		},
		{
			name: "leaf-list min-elements default",
			source: `module cambium-leaf-list-min-elements-default {
    namespace "urn:cambium:leaf-list-min-elements-default";
    prefix cllmed;

    leaf-list values {
        type string;
        min-elements 1;
        default "a";
    }
}`,
			message: `leaf-list "values" with min-elements cannot have defaults`,
		},
		{
			name: "container default",
			source: `module cambium-container-default {
    namespace "urn:cambium:container-default";
    prefix ccd;

    container top {
        default "bad";
    }
}`,
			message: `container "top" cannot have default statements`,
		},
		{
			name: "typedef multiple defaults",
			source: `module cambium-typedef-multiple-defaults {
    namespace "urn:cambium:typedef-multiple-defaults";
    prefix ctmd;

    typedef name {
        type string;
        default "a";
        default "b";
    }

    leaf value { type name; }
}`,
			message: `typedef "name" has multiple default statements`,
		},
		{
			name: "unused typedef multiple defaults",
			source: `module cambium-unused-typedef-multiple-defaults {
    namespace "urn:cambium:unused-typedef-multiple-defaults";
    prefix cutmd;

    typedef unused {
        type string;
        default "a";
        default "b";
    }

    leaf value { type string; }
}`,
			message: `typedef "unused" has multiple default statements`,
		},
		{
			name: "unused typedef invalid default value",
			source: `module cambium-unused-typedef-invalid-default {
    namespace "urn:cambium:unused-typedef-invalid-default";
    prefix cutid;

    typedef flag {
        type boolean;
        default "maybe";
    }

    leaf value { type string; }
}`,
			message: `default "maybe" is not valid for boolean typedef "flag"`,
		},
		{
			name: "unused grouping leaf multiple defaults",
			source: `module cambium-unused-grouping-leaf-multiple-defaults {
    namespace "urn:cambium:unused-grouping-leaf-multiple-defaults";
    prefix cuglmd;

    grouping invalid {
        leaf value {
            type string;
            default "a";
            default "b";
        }
    }

    leaf ok { type string; }
}`,
			message: `leaf "value" has multiple default statements`,
		},
		{
			name: "refine multiple defaults",
			source: `module cambium-refine-multiple-defaults {
    namespace "urn:cambium:refine-multiple-defaults";
    prefix crmd;

    grouping reusable {
        leaf value { type string; }
    }

    container top {
        uses reusable {
            refine value {
                default "a";
                default "b";
            }
        }
    }
}`,
			message: `refine "value" has multiple default statements`,
		},
		{
			name: "choice default missing case",
			source: `module cambium-choice-default-missing-case {
    namespace "urn:cambium:choice-default-missing-case";
    prefix ccdmc;

    container top {
        choice mode {
            default missing;
            case active {
                leaf enabled { type string; }
            }
        }
    }
}`,
			message: `choice "mode" default "missing" does not reference a case`,
		},
		{
			name: "mandatory choice default",
			source: `module cambium-mandatory-choice-default {
    namespace "urn:cambium:mandatory-choice-default";
    prefix cmcd;

    container top {
        choice mode {
            mandatory true;
            default active;
            case active {
                leaf enabled { type string; }
            }
        }
    }
}`,
			message: `mandatory choice "mode" cannot have a default`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			builder, err := cambium.NewContextBuilder(cambium.ContextFlags{})
			if err != nil {
				t.Fatal(err)
			}
			for module, features := range tc.features {
				if err := builder.SetFeatures(module, features); err != nil {
					t.Fatalf("SetFeatures(%s): %v", module, err)
				}
			}
			if err := builder.LoadModuleStr(tc.source); err != nil {
				t.Fatalf("LoadModuleStr: %v", err)
			}
			_, err = builder.Build()
			if err == nil {
				t.Fatal("Build accepted invalid default statement")
			}
			var ce *cambium.Error
			if !errors.As(err, &ce) || ce.RuleCode() != cambium.RuleCodeContext {
				t.Fatalf("Build error = %v, want RuleCodeContext", err)
			}
			if !strings.Contains(err.Error(), tc.message) {
				t.Fatalf("Build error = %q, want to contain %q", err.Error(), tc.message)
			}
		})
	}
}

func TestEmptyTypeEdgeFixtureRuleCode(t *testing.T) {
	dir := filepath.Join(conformanceRoot(t), "fixtures", "types-empty-edge-illegality", "module")
	invalid := []string{"empty-default-illegal", "empty-leaflist-1_0-illegal"}
	for _, module := range invalid {
		t.Run(module, func(t *testing.T) {
			ctx, err := cambium.NewContext()
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { ctx.Close() })
			if err := ctx.SetSearchPath(dir); err != nil {
				t.Fatalf("SetSearchPath: %v", err)
			}
			err = ctx.LoadModule(module)
			if err == nil {
				_, err = ctx.Schema(module)
			}
			if err == nil {
				t.Fatalf("Schema accepted invalid empty-type fixture %q", module)
			}
			var ce *cambium.Error
			if !errors.As(err, &ce) || ce.RuleCode() != cambium.RuleCodeContext {
				t.Fatalf("%s error = %v, want RuleCodeContext", module, err)
			}
		})
	}

	t.Run("empty-leaflist-1_1-legal", func(t *testing.T) {
		ctx, err := cambium.NewContext()
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { ctx.Close() })
		if err := ctx.SetSearchPath(dir); err != nil {
			t.Fatalf("SetSearchPath: %v", err)
		}
		if err := ctx.LoadModule("empty-leaflist-1_1-legal"); err != nil {
			t.Fatalf("LoadModule: %v", err)
		}
		mod, err := ctx.Schema("empty-leaflist-1_1-legal")
		if err != nil {
			t.Fatalf("Schema: %v", err)
		}
		flags := schemaNodeAt(t, mod, "/el1l:flags")
		info, ok := flags.LeafType()
		if !ok {
			t.Fatal("flags should be a leaf-list with type info")
		}
		if got := info.Base(); got != cambium.BaseTypeEmpty {
			t.Fatalf("flags base type = %s, want empty", got)
		}
	})
}

func TestUnsignedIntegerDefaultsAcceptLeadingPlus(t *testing.T) {
	const source = `module cambium-uint-default-leading-plus {
    namespace "urn:cambium:uint-default-leading-plus";
    prefix cudlp;
    yang-version 1.1;

    leaf value {
        type uint8;
        default "+1";
    }

    leaf-list values {
        type uint16;
        default "+2";
    }
}`
	builder, err := cambium.NewContextBuilder(cambium.ContextFlags{})
	if err != nil {
		t.Fatal(err)
	}
	if err := builder.LoadModuleStr(source); err != nil {
		t.Fatalf("LoadModuleStr: %v", err)
	}
	ctx, err := builder.Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	defer ctx.Close()

	mod, err := ctx.Schema("cambium-uint-default-leading-plus")
	if err != nil {
		t.Fatalf("Schema: %v", err)
	}
	value := schemaNodeAt(t, mod, "/cudlp:value")
	if got, ok := value.DefaultValue(); !ok || got != "+1" {
		t.Fatalf("value.DefaultValue() = (%q,%v), want +1,true", got, ok)
	}
	values := schemaNodeAt(t, mod, "/cudlp:values")
	if got := values.DefaultValues(); len(got) != 1 || got[0] != "+2" {
		t.Fatalf("values.DefaultValues() = %v, want [+2]", got)
	}
}

func TestRefineDefaultPreservesEmptyString(t *testing.T) {
	source := `module cambium-refine-empty-default {
    namespace "urn:cambium:refine-empty-default";
    prefix cred;

    grouping reusable {
        leaf value { type string; }
    }

    container top {
        uses reusable {
            refine value {
                default "";
            }
        }
    }
}`
	builder, err := cambium.NewContextBuilder(cambium.ContextFlags{})
	if err != nil {
		t.Fatal(err)
	}
	if err := builder.LoadModuleStr(source); err != nil {
		t.Fatalf("LoadModuleStr: %v", err)
	}
	ctx, err := builder.Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	t.Cleanup(ctx.Close)
	mod, err := ctx.Schema("cambium-refine-empty-default")
	if err != nil {
		t.Fatalf("Schema: %v", err)
	}
	value := schemaNodeAt(t, mod, "/cred:top/cred:value")
	if got, ok := value.DefaultValue(); !ok || got != "" {
		t.Fatalf("DefaultValue = (%q,%v), want empty string,true", got, ok)
	}
}

func TestRefineTextMetadataAppliesToEffectiveNode(t *testing.T) {
	source := `module cambium-refine-text-metadata {
    namespace "urn:cambium:refine-text-metadata";
    prefix crtm;

    grouping reusable {
        leaf value {
            type string;
            description "Original description.";
            reference "original-ref";
        }
    }

    container top {
        uses reusable {
            refine value {
                description "Refined description.";
                reference "refined-ref";
            }
        }
    }
}`
	builder, err := cambium.NewContextBuilder(cambium.ContextFlags{})
	if err != nil {
		t.Fatal(err)
	}
	if err := builder.LoadModuleStr(source); err != nil {
		t.Fatalf("LoadModuleStr: %v", err)
	}
	ctx, err := builder.Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	t.Cleanup(ctx.Close)
	mod, err := ctx.Schema("cambium-refine-text-metadata")
	if err != nil {
		t.Fatalf("Schema: %v", err)
	}
	value := schemaNodeAt(t, mod, "/crtm:top/crtm:value")
	if got, ok := value.Description(); !ok || got != "Refined description." {
		t.Fatalf("Description = (%q,%v), want refined description,true", got, ok)
	}
	if got, ok := value.Reference(); !ok || got != "refined-ref" {
		t.Fatalf("Reference = (%q,%v), want refined-ref,true", got, ok)
	}
}

func TestDuplicateSchemaChildrenReturnContextRuleCode(t *testing.T) {
	cases := []struct {
		name   string
		source string
	}{
		{
			name: "top-level leaf",
			source: `module cambium-duplicate-top-child {
    namespace "urn:cambium:duplicate-top-child";
    prefix cdtc;
    leaf value { type string; }
    leaf value { type string; }
}`,
		},
		{
			name: "container child",
			source: `module cambium-duplicate-nested-child {
    namespace "urn:cambium:duplicate-nested-child";
    prefix cdnc;
    container top {
        leaf value { type string; }
        container value { leaf nested { type string; } }
    }
}`,
		},
		{
			name: "unused grouping child",
			source: `module cambium-duplicate-unused-grouping-child {
    namespace "urn:cambium:duplicate-unused-grouping-child";
    prefix cdugc;
    grouping invalid {
        leaf value { type string; }
        leaf value { type string; }
    }
    leaf ok { type string; }
}`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			builder, err := cambium.NewContextBuilder(cambium.ContextFlags{})
			if err != nil {
				t.Fatal(err)
			}
			if err := builder.LoadModuleStr(tc.source); err != nil {
				t.Fatalf("LoadModuleStr: %v", err)
			}
			_, err = builder.Build()
			if err == nil {
				t.Fatal("Build accepted duplicate schema child")
			}
			var ce *cambium.Error
			if !errors.As(err, &ce) || ce.RuleCode() != cambium.RuleCodeContext {
				t.Fatalf("Build error = %v, want RuleCodeContext", err)
			}
			if !strings.Contains(err.Error(), `duplicate schema child "value"`) {
				t.Fatalf("Build error = %q, want duplicate schema child", err.Error())
			}
		})
	}
}

func TestInvalidImportPrefixesReturnContextRuleCode(t *testing.T) {
	t.Run("missing prefix", func(t *testing.T) {
		builder, err := cambium.NewContextBuilder(cambium.ContextFlags{})
		if err != nil {
			t.Fatal(err)
		}
		source := `module cambium-import-missing-prefix {
    namespace "urn:cambium:import-missing-prefix";
    prefix cimp;
    import cambium-import-target;
}`
		err = builder.LoadModuleStr(source)
		if err == nil {
			t.Fatal("LoadModuleStr accepted import without prefix")
		}
		var ce *cambium.Error
		if !errors.As(err, &ce) || ce.RuleCode() != cambium.RuleCodeContext {
			t.Fatalf("LoadModuleStr error = %v, want RuleCodeContext", err)
		}
		if !strings.Contains(err.Error(), `has no prefix`) {
			t.Fatalf("LoadModuleStr error = %q, want missing-prefix message", err.Error())
		}
	})

	t.Run("duplicate prefix in one import", func(t *testing.T) {
		builder, err := cambium.NewContextBuilder(cambium.ContextFlags{})
		if err != nil {
			t.Fatal(err)
		}
		source := `module cambium-import-duplicate-prefix-child {
    namespace "urn:cambium:import-duplicate-prefix-child";
    prefix cidpc;
    import cambium-import-duplicate-prefix-child-target {
        prefix one;
        prefix two;
    }
}`
		err = builder.LoadModuleStr(source)
		if err == nil {
			t.Fatal("LoadModuleStr accepted import with duplicate prefix children")
		}
		var ce *cambium.Error
		if !errors.As(err, &ce) || ce.RuleCode() != cambium.RuleCodeContext {
			t.Fatalf("LoadModuleStr error = %v, want RuleCodeContext", err)
		}
		if !strings.Contains(err.Error(), `duplicate prefix in import "cambium-import-duplicate-prefix-child-target"`) {
			t.Fatalf("LoadModuleStr error = %q, want duplicate-prefix-child message", err.Error())
		}
	})

	t.Run("duplicate prefix", func(t *testing.T) {
		builder, err := cambium.NewContextBuilder(cambium.ContextFlags{})
		if err != nil {
			t.Fatal(err)
		}
		targetA := `module cambium-import-dup-a {
    namespace "urn:cambium:import-dup-a";
    prefix cida;
}`
		if err := builder.LoadModuleStr(targetA); err != nil {
			t.Fatalf("Load target A: %v", err)
		}
		targetB := `module cambium-import-dup-b {
    namespace "urn:cambium:import-dup-b";
    prefix cidb;
}`
		if err := builder.LoadModuleStr(targetB); err != nil {
			t.Fatalf("Load target B: %v", err)
		}
		source := `module cambium-import-duplicate-prefix {
    namespace "urn:cambium:import-duplicate-prefix";
    prefix cidp;
    import cambium-import-dup-a { prefix dup; }
    import cambium-import-dup-b { prefix dup; }
}`
		err = builder.LoadModuleStr(source)
		if err == nil {
			t.Fatal("LoadModuleStr accepted duplicate import prefix")
		}
		var ce *cambium.Error
		if !errors.As(err, &ce) || ce.RuleCode() != cambium.RuleCodeContext {
			t.Fatalf("LoadModuleStr error = %v, want RuleCodeContext", err)
		}
		if !strings.Contains(err.Error(), `duplicate import prefix "dup"`) {
			t.Fatalf("LoadModuleStr error = %q, want duplicate-prefix message", err.Error())
		}
	})

	t.Run("prefix collides with module prefix", func(t *testing.T) {
		builder, err := cambium.NewContextBuilder(cambium.ContextFlags{})
		if err != nil {
			t.Fatal(err)
		}
		target := `module cambium-import-prefix-collision-target {
    namespace "urn:cambium:import-prefix-collision-target";
    prefix ctpct;
}`
		if err := builder.LoadModuleStr(target); err != nil {
			t.Fatalf("Load target: %v", err)
		}
		source := `module cambium-import-prefix-collision-user {
    namespace "urn:cambium:import-prefix-collision-user";
    prefix cipcu;
    import cambium-import-prefix-collision-target { prefix cipcu; }
}`
		err = builder.LoadModuleStr(source)
		if err == nil {
			t.Fatal("LoadModuleStr accepted import prefix colliding with module prefix")
		}
		var ce *cambium.Error
		if !errors.As(err, &ce) || ce.RuleCode() != cambium.RuleCodeContext {
			t.Fatalf("LoadModuleStr error = %v, want RuleCodeContext", err)
		}
		if !strings.Contains(err.Error(), `import prefix "cipcu" in module "cambium-import-prefix-collision-user" collides with module prefix`) {
			t.Fatalf("LoadModuleStr error = %q, want module-prefix collision message", err.Error())
		}
	})

	t.Run("prefix collides with module name", func(t *testing.T) {
		builder, err := cambium.NewContextBuilder(cambium.ContextFlags{})
		if err != nil {
			t.Fatal(err)
		}
		target := `module cambium-import-name-collision-target {
    namespace "urn:cambium:import-name-collision-target";
    prefix ctinct;
}`
		if err := builder.LoadModuleStr(target); err != nil {
			t.Fatalf("Load target: %v", err)
		}
		source := `module cambium-import-name-collision-user {
    namespace "urn:cambium:import-name-collision-user";
    prefix cincu;
    import cambium-import-name-collision-target { prefix cambium-import-name-collision-user; }
}`
		err = builder.LoadModuleStr(source)
		if err == nil {
			t.Fatal("LoadModuleStr accepted import prefix colliding with module name")
		}
		var ce *cambium.Error
		if !errors.As(err, &ce) || ce.RuleCode() != cambium.RuleCodeContext {
			t.Fatalf("LoadModuleStr error = %v, want RuleCodeContext", err)
		}
		if !strings.Contains(err.Error(), `import prefix "cambium-import-name-collision-user" in module "cambium-import-name-collision-user" collides with module name`) {
			t.Fatalf("LoadModuleStr error = %q, want module-name collision message", err.Error())
		}
	})

	t.Run("self import", func(t *testing.T) {
		builder, err := cambium.NewContextBuilder(cambium.ContextFlags{})
		if err != nil {
			t.Fatal(err)
		}
		source := `module cambium-import-self {
    namespace "urn:cambium:import-self";
    prefix cis;
    import cambium-import-self { prefix self; }
}`
		err = builder.LoadModuleStr(source)
		if err == nil {
			t.Fatal("LoadModuleStr accepted self import")
		}
		var ce *cambium.Error
		if !errors.As(err, &ce) || ce.RuleCode() != cambium.RuleCodeContext {
			t.Fatalf("LoadModuleStr error = %v, want RuleCodeContext", err)
		}
		if !strings.Contains(err.Error(), `module "cambium-import-self" imports itself`) {
			t.Fatalf("LoadModuleStr error = %q, want self-import message", err.Error())
		}
	})
}

func TestSubmoduleImportPrefixesAreLexicallyScoped(t *testing.T) {
	t.Run("parent and submodule may reuse same import prefix", func(t *testing.T) {
		dir := schemaIntrospectionModuleDir(t)
		writeModuleFile(t, filepath.Join(dir, "cisco-semver.yang"), []byte(`module cisco-semver {
    namespace "urn:cisco:semver";
    prefix semver;
}`))
		writeModuleFile(t, filepath.Join(dir, "cambium-import-scope-cisco-parent.yang"), []byte(`module cambium-import-scope-cisco-parent {
    namespace "urn:cambium:import-scope-cisco-parent";
    prefix ciscop;
    import cisco-semver { prefix semver; }
    include cambium-import-scope-cisco-part;
}`))
		writeModuleFile(t, filepath.Join(dir, "cambium-import-scope-cisco-part.yang"), []byte(`submodule cambium-import-scope-cisco-part {
    belongs-to cambium-import-scope-cisco-parent {
        prefix ciscop;
    }
    import cisco-semver { prefix semver; }
    leaf from-part { type string; }
}`))
		ctx, err := cambium.NewContext()
		if err != nil {
			t.Fatal(err)
		}
		defer ctx.Close()
		if err := ctx.SetSearchPath(dir); err != nil {
			t.Fatal(err)
		}
		if err := ctx.LoadModule("cambium-import-scope-cisco-parent"); err != nil {
			t.Fatalf("LoadModule: %v", err)
		}
		if _, err := ctx.Schema("cambium-import-scope-cisco-parent"); err != nil {
			t.Fatalf("Schema: %v", err)
		}
	})

	t.Run("duplicate prefix inside one submodule is rejected", func(t *testing.T) {
		dir := schemaIntrospectionModuleDir(t)
		writeModuleFile(t, filepath.Join(dir, "cambium-import-scope-dup-a.yang"), []byte(`module cambium-import-scope-dup-a {
    namespace "urn:cambium:import-scope-dup-a";
    prefix cisda;
}`))
		writeModuleFile(t, filepath.Join(dir, "cambium-import-scope-dup-b.yang"), []byte(`module cambium-import-scope-dup-b {
    namespace "urn:cambium:import-scope-dup-b";
    prefix cisdb;
}`))
		writeModuleFile(t, filepath.Join(dir, "cambium-import-scope-dup-parent.yang"), []byte(`module cambium-import-scope-dup-parent {
    namespace "urn:cambium:import-scope-dup-parent";
    prefix cisdp;
    include cambium-import-scope-dup-part;
}`))
		writeModuleFile(t, filepath.Join(dir, "cambium-import-scope-dup-part.yang"), []byte(`submodule cambium-import-scope-dup-part {
    belongs-to cambium-import-scope-dup-parent {
        prefix cisdp;
    }
    import cambium-import-scope-dup-a { prefix dup; }
    import cambium-import-scope-dup-b { prefix dup; }
}`))
		ctx, err := cambium.NewContext()
		if err != nil {
			t.Fatal(err)
		}
		defer ctx.Close()
		if err := ctx.SetSearchPath(dir); err != nil {
			t.Fatal(err)
		}
		err = ctx.LoadModule("cambium-import-scope-dup-parent")
		if err == nil {
			t.Fatal("LoadModule accepted duplicate import prefix inside one submodule")
		}
		var ce *cambium.Error
		if !errors.As(err, &ce) || ce.RuleCode() != cambium.RuleCodeContext {
			t.Fatalf("LoadModule error = %v, want RuleCodeContext", err)
		}
		if !strings.Contains(err.Error(), `duplicate import prefix "dup" in submodule "cambium-import-scope-dup-part"`) {
			t.Fatalf("LoadModule error = %q, want submodule duplicate-prefix message", err.Error())
		}
	})

	t.Run("submodule prefix shadows parent import for local type references", func(t *testing.T) {
		dir := schemaIntrospectionModuleDir(t)
		writeModuleFile(t, filepath.Join(dir, "cambium-import-scope-a.yang"), []byte(`module cambium-import-scope-a {
    namespace "urn:cambium:import-scope-a";
    prefix cisa;
    typedef selected {
        type string;
    }
}`))
		writeModuleFile(t, filepath.Join(dir, "cambium-import-scope-b.yang"), []byte(`module cambium-import-scope-b {
    namespace "urn:cambium:import-scope-b";
    prefix cisb;
    typedef selected {
        type uint32;
    }
}`))
		writeModuleFile(t, filepath.Join(dir, "cambium-import-scope-shadow-parent.yang"), []byte(`module cambium-import-scope-shadow-parent {
    namespace "urn:cambium:import-scope-shadow-parent";
    prefix cissp;
    import cambium-import-scope-a { prefix p; }
    include cambium-import-scope-shadow-part;

    leaf parent-selected {
        type p:selected;
    }
}`))
		writeModuleFile(t, filepath.Join(dir, "cambium-import-scope-shadow-part.yang"), []byte(`submodule cambium-import-scope-shadow-part {
    belongs-to cambium-import-scope-shadow-parent {
        prefix cissp;
    }
    import cambium-import-scope-b { prefix p; }

    leaf submodule-selected {
        type p:selected;
    }
}`))
		ctx, err := cambium.NewContext()
		if err != nil {
			t.Fatal(err)
		}
		defer ctx.Close()
		if err := ctx.SetSearchPath(dir); err != nil {
			t.Fatal(err)
		}
		if err := ctx.LoadModule("cambium-import-scope-shadow-parent"); err != nil {
			t.Fatalf("LoadModule: %v", err)
		}
		mod, err := ctx.Schema("cambium-import-scope-shadow-parent")
		if err != nil {
			t.Fatalf("Schema: %v", err)
		}
		parent := schemaNodeAt(t, mod, "/cissp:parent-selected")
		parentType, ok := parent.LeafType()
		if !ok {
			t.Fatal("parent-selected should have a leaf type")
		}
		if got, want := parentType.Base(), cambium.BaseTypeString; got != want {
			t.Fatalf("parent-selected base = %s, want %s", got, want)
		}
		submodule := schemaNodeAt(t, mod, "/cissp:submodule-selected")
		submoduleType, ok := submodule.LeafType()
		if !ok {
			t.Fatal("submodule-selected should have a leaf type")
		}
		if got, want := submoduleType.Base(), cambium.BaseTypeUint32; got != want {
			t.Fatalf("submodule-selected base = %s, want %s", got, want)
		}
	})
}

func TestSubmoduleLexicalPrefixScopeAppliesToSchemaReferences(t *testing.T) {
	dir := schemaIntrospectionModuleDir(t)
	writeModuleFile(t, filepath.Join(dir, "cambium-import-scope-reference-a.yang"), []byte(`module cambium-import-scope-reference-a {
    yang-version 1.1;
    namespace "urn:cambium:import-scope-reference-a";
    prefix cisra;

    feature gate;
    extension marker;
    typedef selected {
        type string;
    }
    identity base;
    container top {
        leaf value {
            type string;
        }
    }
}`))
	writeModuleFile(t, filepath.Join(dir, "cambium-import-scope-reference-b.yang"), []byte(`module cambium-import-scope-reference-b {
    yang-version 1.1;
    namespace "urn:cambium:import-scope-reference-b";
    prefix cisrb;

    feature gate;
    extension marker {
        argument label;
    }
    typedef selected {
        type uint32;
    }
    identity base;
    container top {
        leaf value {
            type uint32;
        }
    }
}`))
	writeModuleFile(t, filepath.Join(dir, "cambium-import-scope-reference-parent.yang"), []byte(`module cambium-import-scope-reference-parent {
    yang-version 1.1;
    namespace "urn:cambium:import-scope-reference-parent";
    prefix cisrp;

    import cambium-import-scope-reference-a { prefix p; }
    include cambium-import-scope-reference-part;

    p:marker;

    identity parent-derived {
        base p:base;
    }

    leaf parent-gated {
        if-feature p:gate;
        type string;
    }
    leaf parent-selected {
        type p:selected;
    }
    leaf parent-idref {
        type identityref {
            base p:base;
        }
    }
}`))
	writeModuleFile(t, filepath.Join(dir, "cambium-import-scope-reference-part.yang"), []byte(`submodule cambium-import-scope-reference-part {
    yang-version 1.1;
    belongs-to cambium-import-scope-reference-parent {
        prefix cisrp;
    }

    import cambium-import-scope-reference-b { prefix p; }
    import cambium-import-scope-reference-b { prefix q; }

    p:marker "submodule";

    identity submodule-derived {
        base p:base;
    }

    leaf submodule-gated {
        if-feature p:gate;
        type string;
    }
    leaf submodule-selected {
        type p:selected;
    }
    leaf submodule-idref {
        type identityref {
            base p:base;
        }
    }
    leaf submodule-leafref {
        type leafref {
            path "/p:top/p:value";
            require-instance false;
        }
    }
    leaf submodule-must {
        must "../q:top or not(../q:top)";
        type string;
    }
    container submodule-when {
        when "/q:top";
        leaf value {
            type string;
        }
    }
    augment "/p:top" {
        leaf augmented-from-submodule {
            type string;
        }
    }
    deviation "/p:top/p:value" {
        deviate replace {
            config false;
        }
    }
}`))

	ctx, err := cambium.NewContext()
	if err != nil {
		t.Fatal(err)
	}
	defer ctx.Close()
	if err := ctx.SetSearchPath(dir); err != nil {
		t.Fatal(err)
	}
	if err := ctx.SetFeatures("cambium-import-scope-reference-b", []string{"gate"}); err != nil {
		t.Fatalf("SetFeatures: %v", err)
	}
	if err := ctx.LoadModule("cambium-import-scope-reference-parent"); err != nil {
		t.Fatalf("LoadModule: %v", err)
	}

	parentMod, err := ctx.Schema("cambium-import-scope-reference-parent")
	if err != nil {
		t.Fatalf("Schema parent: %v", err)
	}
	aMod, err := ctx.Schema("cambium-import-scope-reference-a")
	if err != nil {
		t.Fatalf("Schema A: %v", err)
	}
	bMod, err := ctx.Schema("cambium-import-scope-reference-b")
	if err != nil {
		t.Fatalf("Schema B: %v", err)
	}

	assertLeafBase := func(path string, want cambium.BaseType) {
		t.Helper()
		node := schemaNodeAt(t, parentMod, path)
		info, ok := node.LeafType()
		if !ok {
			t.Fatalf("%s should have a leaf type", path)
		}
		if got := info.Base(); got != want {
			t.Fatalf("%s base = %s, want %s", path, got, want)
		}
	}
	assertLeafBase("/cisrp:parent-selected", cambium.BaseTypeString)
	assertLeafBase("/cisrp:submodule-selected", cambium.BaseTypeUint32)

	if _, err := parentMod.FindPath("/cisrp:parent-gated"); err == nil {
		t.Fatal("parent-gated should be excluded because parent p:gate resolves to disabled module A feature")
	}
	if _, err := parentMod.FindPath("/cisrp:submodule-gated"); err != nil {
		t.Fatalf("submodule-gated should be included because submodule p:gate resolves to enabled module B feature: %v", err)
	}

	identityBaseModule := func(name string) string {
		t.Helper()
		for id := range parentMod.Identities() {
			if id.Name() != name {
				continue
			}
			bases := id.Bases()
			if len(bases) != 1 {
				t.Fatalf("%s bases = %d, want 1", name, len(bases))
			}
			return bases[0].Module().Name()
		}
		t.Fatalf("identity %s not found", name)
		return ""
	}
	if got, want := identityBaseModule("parent-derived"), "cambium-import-scope-reference-a"; got != want {
		t.Fatalf("parent-derived base module = %q, want %q", got, want)
	}
	if got, want := identityBaseModule("submodule-derived"), "cambium-import-scope-reference-b"; got != want {
		t.Fatalf("submodule-derived base module = %q, want %q", got, want)
	}

	assertIdentityRefBase := func(path, want string) {
		t.Helper()
		node := schemaNodeAt(t, parentMod, path)
		info, ok := node.LeafType()
		if !ok {
			t.Fatalf("%s should have a leaf type", path)
		}
		idref, ok := info.Resolved().(cambium.ResolvedIdentityRef)
		if !ok {
			t.Fatalf("%s resolved type = %T, want ResolvedIdentityRef", path, info.Resolved())
		}
		bases := idref.Bases()
		if len(bases) != 1 {
			t.Fatalf("%s identityref bases = %d, want 1", path, len(bases))
		}
		if got := bases[0].Module().Name(); got != want {
			t.Fatalf("%s identityref base module = %q, want %q", path, got, want)
		}
	}
	assertIdentityRefBase("/cisrp:parent-idref", "cambium-import-scope-reference-a")
	assertIdentityRefBase("/cisrp:submodule-idref", "cambium-import-scope-reference-b")

	leafrefNode := schemaNodeAt(t, parentMod, "/cisrp:submodule-leafref")
	leafrefInfo, ok := leafrefNode.LeafType()
	if !ok {
		t.Fatal("submodule-leafref should have a leaf type")
	}
	leafref, ok := leafrefInfo.Resolved().(cambium.ResolvedLeafRef)
	if !ok {
		t.Fatalf("submodule-leafref resolved type = %T, want ResolvedLeafRef", leafrefInfo.Resolved())
	}
	target, ok := leafref.Target()
	if !ok {
		t.Fatal("submodule-leafref target was not resolved")
	}
	if got, want := target.Module().Name(), "cambium-import-scope-reference-b"; got != want {
		t.Fatalf("submodule-leafref target module = %q, want %q", got, want)
	}

	if musts := schemaNodeAt(t, parentMod, "/cisrp:submodule-must").Musts(); len(musts) != 1 {
		t.Fatalf("submodule-must musts = %d, want 1", len(musts))
	}
	if whens := schemaNodeAt(t, parentMod, "/cisrp:submodule-when").Whens(); len(whens) != 1 {
		t.Fatalf("submodule-when whens = %d, want 1", len(whens))
	}

	exts := parentMod.Extensions()
	extByModule := make(map[string]int)
	for _, ext := range exts {
		if ext.Name() == "marker" {
			extByModule[ext.ModuleName()]++
		}
	}
	if got := extByModule["cambium-import-scope-reference-a"]; got != 1 {
		t.Fatalf("module A top-level marker extensions = %d, want 1 (all: %#v)", got, exts)
	}
	if got := extByModule["cambium-import-scope-reference-b"]; got != 1 {
		t.Fatalf("module B top-level marker extensions = %d, want 1 (all: %#v)", got, exts)
	}

	if _, err := aMod.FindPath("/cisra:top/cisrp:augmented-from-submodule"); err == nil {
		t.Fatal("augment from submodule should not land on module A")
	}
	augmented, err := bMod.FindPath("/cisrb:top/cisrp:augmented-from-submodule")
	if err != nil {
		t.Fatalf("augment from submodule should land on module B: %v", err)
	}
	if got, want := augmented.Module().Name(), "cambium-import-scope-reference-parent"; got != want {
		t.Fatalf("augmented child module = %q, want %q", got, want)
	}
	aValue := schemaNodeAt(t, aMod, "/cisra:top/cisra:value")
	if got, want := aValue.Config(), cambium.ConfigRw; got != want {
		t.Fatalf("module A value Config() = %v, want %v", got, want)
	}
	bValue := schemaNodeAt(t, bMod, "/cisrb:top/cisrb:value")
	if got, want := bValue.Config(), cambium.ConfigRo; got != want {
		t.Fatalf("module B value Config() = %v, want %v", got, want)
	}
}

func TestImportNonTransitiveRuleCode(t *testing.T) {
	dir := filepath.Join(conformanceRoot(t), "fixtures", "linkage-import-non-transitive", "module")

	t.Run("transitive dependency does not leak symbols", func(t *testing.T) {
		ctx, err := cambium.NewContext()
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { ctx.Close() })
		if err := ctx.SetSearchPath(dir); err != nil {
			t.Fatalf("SetSearchPath: %v", err)
		}
		err = ctx.LoadModule("linkage-import-non-transitive-a")
		if err != nil {
			t.Fatalf("LoadModule linkage-import-non-transitive-a: %v", err)
		}
		_, err = ctx.Schema("linkage-import-non-transitive-a")
		if err == nil {
			t.Fatal("Schema accepted transitive import prefix use")
		}
		var ce *cambium.Error
		if !errors.As(err, &ce) || ce.RuleCode() != cambium.RuleCodeContext {
			t.Fatalf("Schema error = %v, want RuleCodeContext", err)
		}
	})

	t.Run("direct import still resolves", func(t *testing.T) {
		ctx, err := cambium.NewContext()
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { ctx.Close() })
		if err := ctx.SetSearchPath(dir); err != nil {
			t.Fatalf("SetSearchPath: %v", err)
		}
		if err := ctx.LoadModule("linkage-import-non-transitive-b"); err != nil {
			t.Fatalf("LoadModule linkage-import-non-transitive-b: %v", err)
		}
		if _, err := ctx.Schema("linkage-import-non-transitive-b"); err != nil {
			t.Fatalf("Schema linkage-import-non-transitive-b: %v", err)
		}
	})
}

func TestInvalidDependencyRevisionDatesReturnContextRuleCode(t *testing.T) {
	cases := []struct {
		name    string
		source  string
		message string
	}{
		{
			name: "import invalid revision-date",
			source: `module cambium-import-invalid-revision-date {
    namespace "urn:cambium:import-invalid-revision-date";
    prefix ciird;
    import cambium-import-invalid-revision-date-target {
        prefix target;
        revision-date 2024-02-31;
    }
}`,
			message: `invalid revision-date "2024-02-31"`,
		},
		{
			name: "import duplicate revision-date",
			source: `module cambium-import-duplicate-revision-date {
    namespace "urn:cambium:import-duplicate-revision-date";
    prefix cidrd;
    import cambium-import-duplicate-revision-date-target {
        prefix target;
        revision-date 2024-01-01;
        revision-date 2025-01-01;
    }
}`,
			message: `duplicate revision-date in import "cambium-import-duplicate-revision-date-target"`,
		},
		{
			name: "import duplicate description",
			source: `module cambium-import-duplicate-description {
    namespace "urn:cambium:import-duplicate-description";
    prefix cidd;
    import cambium-import-duplicate-description-target {
        prefix target;
        description "one";
        description "two";
    }
}`,
			message: `duplicate description in import "cambium-import-duplicate-description-target"`,
		},
		{
			name: "import invalid child",
			source: `module cambium-import-invalid-child {
    namespace "urn:cambium:import-invalid-child";
    prefix ciic;
    import cambium-import-invalid-child-target {
        prefix target;
        default "bad";
    }
}`,
			message: `default "bad" is not valid under import "cambium-import-invalid-child-target"`,
		},
		{
			name: "include invalid revision-date",
			source: `module cambium-include-invalid-revision-date {
    namespace "urn:cambium:include-invalid-revision-date";
    prefix ciird;
    include child {
        revision-date 2024-02-31;
    }
}`,
			message: `invalid revision-date "2024-02-31"`,
		},
		{
			name: "include duplicate revision-date",
			source: `module cambium-include-duplicate-revision-date {
    namespace "urn:cambium:include-duplicate-revision-date";
    prefix cidrd;
    include child {
        revision-date 2024-01-01;
        revision-date 2025-01-01;
    }
}`,
			message: `duplicate revision-date in include "child"`,
		},
		{
			name: "include duplicate reference",
			source: `module cambium-include-duplicate-reference {
    namespace "urn:cambium:include-duplicate-reference";
    prefix cidr;
    include child {
        reference "one";
        reference "two";
    }
}`,
			message: `duplicate reference in include "child"`,
		},
		{
			name: "include invalid child",
			source: `module cambium-include-invalid-child {
    namespace "urn:cambium:include-invalid-child";
    prefix ciic;
    include child {
        default "bad";
    }
}`,
			message: `default "bad" is not valid under include "child"`,
		},
		{
			name: "submodule include invalid revision-date",
			source: `submodule cambium-submodule-include-invalid-revision-date {
    belongs-to cambium-submodule-include-invalid-revision-date-parent {
        prefix parent;
    }
    include child {
        revision-date 2024-02-31;
    }
}`,
			message: `invalid revision-date "2024-02-31"`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			builder, err := cambium.NewContextBuilder(cambium.ContextFlags{})
			if err != nil {
				t.Fatal(err)
			}
			err = builder.LoadModuleStr(tc.source)
			if err == nil {
				t.Fatal("LoadModuleStr accepted invalid dependency metadata")
			}
			var ce *cambium.Error
			if !errors.As(err, &ce) || ce.RuleCode() != cambium.RuleCodeContext {
				t.Fatalf("LoadModuleStr error = %v, want RuleCodeContext", err)
			}
			if !strings.Contains(err.Error(), tc.message) {
				t.Fatalf("LoadModuleStr error = %q, want to contain %q", err.Error(), tc.message)
			}
		})
	}
}

func TestDuplicateIncludeRejected(t *testing.T) {
	dir := schemaIntrospectionModuleDir(t)
	module := `module cambium-duplicate-include {
    namespace "urn:cambium:duplicate-include";
    prefix cdi;
    include cambium-duplicate-include-part;
    include cambium-duplicate-include-part;
}`
	submodule := `submodule cambium-duplicate-include-part {
    belongs-to cambium-duplicate-include {
        prefix cdi;
    }
    leaf value { type string; }
}`
	writeModuleFile(t, filepath.Join(dir, "cambium-duplicate-include.yang"), []byte(module))
	writeModuleFile(t, filepath.Join(dir, "cambium-duplicate-include-part.yang"), []byte(submodule))

	ctx, err := cambium.NewContext()
	if err != nil {
		t.Fatal(err)
	}
	defer ctx.Close()
	if err := ctx.SetSearchPath(dir); err != nil {
		t.Fatal(err)
	}
	err = ctx.LoadModule("cambium-duplicate-include")
	if err == nil {
		t.Fatal("LoadModule accepted duplicate include")
	}
	var ce *cambium.Error
	if !errors.As(err, &ce) || ce.RuleCode() != cambium.RuleCodeContext {
		t.Fatalf("LoadModule error = %v, want RuleCodeContext", err)
	}
	if !strings.Contains(err.Error(), `duplicate include "cambium-duplicate-include-part"`) {
		t.Fatalf("LoadModule error = %q, want duplicate-include message", err.Error())
	}
}

func TestInvalidModuleMetadataReturnsContextRuleCode(t *testing.T) {
	cases := []struct {
		name    string
		source  string
		message string
	}{
		{
			name: "missing namespace",
			source: `module cambium-missing-namespace {
    prefix cmn;
}`,
			message: `module "cambium-missing-namespace" has no namespace`,
		},
		{
			name: "missing prefix",
			source: `module cambium-missing-prefix {
    namespace "urn:cambium:missing-prefix";
}`,
			message: `module "cambium-missing-prefix" has no prefix`,
		},
		{
			name: "duplicate namespace statement",
			source: `module cambium-duplicate-namespace-statement {
    namespace "urn:cambium:duplicate-namespace-statement-a";
    namespace "urn:cambium:duplicate-namespace-statement-b";
    prefix cdns;
}`,
			message: `duplicate namespace in module "cambium-duplicate-namespace-statement"`,
		},
		{
			name: "namespace invalid child",
			source: `module cambium-namespace-invalid-child {
    namespace "urn:cambium:namespace-invalid-child" {
        description "bad";
    }
    prefix cnic;
}`,
			message: `description "bad" is not valid under namespace "urn:cambium:namespace-invalid-child"`,
		},
		{
			name: "duplicate prefix statement",
			source: `module cambium-duplicate-prefix-statement {
    namespace "urn:cambium:duplicate-prefix-statement";
    prefix cdps;
    prefix cdps2;
}`,
			message: `duplicate prefix in module "cambium-duplicate-prefix-statement"`,
		},
		{
			name: "prefix invalid child",
			source: `module cambium-prefix-invalid-child {
    namespace "urn:cambium:prefix-invalid-child";
    prefix cpic {
        description "bad";
    }
}`,
			message: `description "bad" is not valid under prefix "cpic"`,
		},
		{
			name: "contact invalid child",
			source: `module cambium-contact-invalid-child {
    namespace "urn:cambium:contact-invalid-child";
    prefix ccic;
    contact "ops" {
        default "bad";
    }
}`,
			message: `default "bad" is not valid under contact "ops"`,
		},
		{
			name: "duplicate contact",
			source: `module cambium-duplicate-contact {
    namespace "urn:cambium:duplicate-contact";
    prefix cdc;
    contact "ops-a";
    contact "ops-b";
}`,
			message: `duplicate contact in module "cambium-duplicate-contact"`,
		},
		{
			name: "duplicate organization",
			source: `module cambium-duplicate-organization {
    namespace "urn:cambium:duplicate-organization";
    prefix cdo;
    organization "team-a";
    organization "team-b";
}`,
			message: `duplicate organization in module "cambium-duplicate-organization"`,
		},
		{
			name: "duplicate description",
			source: `module cambium-duplicate-description {
    namespace "urn:cambium:duplicate-description";
    prefix cdd;
    description "one";
    description "two";
}`,
			message: `duplicate description in module "cambium-duplicate-description"`,
		},
		{
			name: "duplicate reference",
			source: `module cambium-duplicate-reference {
    namespace "urn:cambium:duplicate-reference";
    prefix cdrf;
    reference "one";
    reference "two";
}`,
			message: `duplicate reference in module "cambium-duplicate-reference"`,
		},
		{
			name: "description invalid child",
			source: `module cambium-description-invalid-child {
    namespace "urn:cambium:description-invalid-child";
    prefix cdic;
    description "module text" {
        default "bad";
    }
}`,
			message: `default "bad" is not valid under description "module text"`,
		},
		{
			name: "invalid yang-version",
			source: `module cambium-invalid-yang-version {
    yang-version 2;
    namespace "urn:cambium:invalid-yang-version";
    prefix ciyv;
}`,
			message: `invalid yang-version "2" in module "cambium-invalid-yang-version"`,
		},
		{
			name: "duplicate yang-version",
			source: `module cambium-duplicate-yang-version {
    yang-version 1;
    yang-version 1.1;
    namespace "urn:cambium:duplicate-yang-version";
    prefix cdyv;
}`,
			message: `duplicate yang-version in module "cambium-duplicate-yang-version"`,
		},
		{
			name: "yang-version invalid child",
			source: `module cambium-yang-version-invalid-child {
    yang-version 1 {
        description "bad";
    }
    namespace "urn:cambium:yang-version-invalid-child";
    prefix cyvic;
}`,
			message: `description "bad" is not valid under yang-version "1"`,
		},
		{
			name: "duplicate namespace",
			source: `module cambium-duplicate-namespace-a {
    namespace "urn:cambium:duplicate-namespace";
    prefix cdna;
}
---
module cambium-duplicate-namespace-b {
    namespace "urn:cambium:duplicate-namespace";
    prefix cdnb;
}`,
			message: `namespace "urn:cambium:duplicate-namespace" already belongs to module "cambium-duplicate-namespace-a"`,
		},
		{
			name: "invalid revision",
			source: `module cambium-invalid-revision {
    namespace "urn:cambium:invalid-revision";
    prefix cir;
    revision 2024-02-31;
}`,
			message: `invalid revision "2024-02-31"`,
		},
		{
			name: "namespace after body",
			source: `module cambium-namespace-after-body {
    prefix cnab;
    leaf value { type string; }
    namespace "urn:cambium:namespace-after-body";
}`,
			message: `namespace "urn:cambium:namespace-after-body" is out of order in module "cambium-namespace-after-body"`,
		},
		{
			name: "import after body",
			source: `module cambium-import-after-body {
    namespace "urn:cambium:import-after-body";
    prefix ciab;
    leaf value { type string; }
    import cambium-import-after-body-target { prefix target; }
}`,
			message: `import "cambium-import-after-body-target" is out of order in module "cambium-import-after-body"`,
		},
		{
			name: "include after body",
			source: `module cambium-include-after-body {
    namespace "urn:cambium:include-after-body";
    prefix ciab;
    leaf value { type string; }
    include cambium-include-after-body-part;
}`,
			message: `include "cambium-include-after-body-part" is out of order in module "cambium-include-after-body"`,
		},
		{
			name: "duplicate revision",
			source: `module cambium-duplicate-revision {
    namespace "urn:cambium:duplicate-revision";
    prefix cdr;
    revision 2024-01-01;
    revision 2024-01-01;
}`,
			message: `duplicate revision "2024-01-01"`,
		},
		{
			name: "revision duplicate description",
			source: `module cambium-revision-duplicate-description {
    namespace "urn:cambium:revision-duplicate-description";
    prefix crdd;
    revision 2024-01-01 {
        description "one";
        description "two";
    }
}`,
			message: `duplicate description in revision "2024-01-01"`,
		},
		{
			name: "revision invalid child",
			source: `module cambium-revision-invalid-child {
    namespace "urn:cambium:revision-invalid-child";
    prefix cric;
    revision 2024-01-01 {
        default "bad";
    }
}`,
			message: `default "bad" is not valid under revision "2024-01-01"`,
		},
		{
			name: "module belongs-to",
			source: `module cambium-module-belongs-to {
    namespace "urn:cambium:module-belongs-to";
    prefix cmbt;
    belongs-to cambium-parent {
        prefix parent;
    }
}`,
			message: `belongs-to "cambium-parent" is not valid in module "cambium-module-belongs-to"`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			builder, err := cambium.NewContextBuilder(cambium.ContextFlags{})
			if err != nil {
				t.Fatal(err)
			}
			var loadErr error
			for _, source := range strings.Split(tc.source, "\n---\n") {
				loadErr = builder.LoadModuleStr(source)
				if loadErr != nil {
					break
				}
			}
			err = loadErr
			if err == nil {
				t.Fatal("LoadModuleStr accepted invalid module metadata")
			}
			var ce *cambium.Error
			if !errors.As(err, &ce) || ce.RuleCode() != cambium.RuleCodeContext {
				t.Fatalf("LoadModuleStr error = %v, want RuleCodeContext", err)
			}
			if !strings.Contains(err.Error(), tc.message) {
				t.Fatalf("LoadModuleStr error = %q, want to contain %q", err.Error(), tc.message)
			}
		})
	}
}

func TestYang11StatementsAcceptedWithYangVersion11(t *testing.T) {
	const source = `module cambium-yang11-statements-valid {
    yang-version 1.1;
    namespace "urn:cambium:yang11-statements-valid";
    prefix cysv;

    anydata payload;

    rpc probe {
        input {
            must "true()" {
                error-message "probe input must be valid";
            }
            leaf id { type string; }
        }
        output {
            must "../id != ''";
            leaf id { type string; }
        }
    }

    notification changed {
        must "count(*) >= 0";
        leaf id { type string; }
    }

    container top {
        action reset {
            input {
                must "true()";
                leaf id { type string; }
            }
        }
    }
}`
	builder, err := cambium.NewContextBuilder(cambium.ContextFlags{})
	if err != nil {
		t.Fatal(err)
	}
	if err := builder.LoadModuleStr(source); err != nil {
		t.Fatalf("LoadModuleStr: %v", err)
	}
	ctx, err := builder.Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	defer ctx.Close()
	mod, err := ctx.Schema("cambium-yang11-statements-valid")
	if err != nil {
		t.Fatalf("Schema: %v", err)
	}
	if _, err := mod.FindPath("/cysv:payload"); err != nil {
		t.Fatalf("FindPath payload: %v", err)
	}
	rpcs := mod.RPCs()
	if rpcs.Len() != 1 {
		t.Fatalf("RPCs() len = %d, want 1", rpcs.Len())
	}
	rpc, _ := rpcs.Get(0)
	rpcInput, ok := rpc.Input()
	if !ok {
		t.Fatal("probe Input() returned false")
	}
	rpcInputMusts := rpcInput.Musts()
	if len(rpcInputMusts) != 1 {
		t.Fatalf("probe input musts = %d, want 1", len(rpcInputMusts))
	}
	if rpcInputMusts[0].Expression() != "true()" {
		t.Fatalf("probe input must = %q, want true()", rpcInputMusts[0].Expression())
	}
	if msg, ok := rpcInputMusts[0].ErrorMessage(); !ok || msg != "probe input must be valid" {
		t.Fatalf("probe input must error-message = (%q, %v), want (probe input must be valid, true)", msg, ok)
	}
	rpcOutput, ok := rpc.Output()
	if !ok {
		t.Fatal("probe Output() returned false")
	}
	rpcOutputMusts := rpcOutput.Musts()
	if len(rpcOutputMusts) != 1 {
		t.Fatalf("probe output musts = %d, want 1", len(rpcOutputMusts))
	}
	if rpcOutputMusts[0].Expression() != "../id != ''" {
		t.Fatalf("probe output must = %q, want ../id != ''", rpcOutputMusts[0].Expression())
	}
	notifs := mod.Notifications()
	if notifs.Len() != 1 {
		t.Fatalf("Notifications() len = %d, want 1", notifs.Len())
	}
	notif, _ := notifs.Get(0)
	notifMusts := notif.Musts()
	if len(notifMusts) != 1 {
		t.Fatalf("changed musts = %d, want 1", len(notifMusts))
	}
	if notifMusts[0].Expression() != "count(*) >= 0" {
		t.Fatalf("changed must = %q, want count(*) >= 0", notifMusts[0].Expression())
	}
	top, err := mod.FindPath("/cysv:top")
	if err != nil {
		t.Fatalf("FindPath top: %v", err)
	}
	for child := range top.Children().Iter() {
		if child.Name() == "reset" && child.Kind() == cambium.SchemaNodeKindAction {
			resetInput, ok := child.Input()
			if !ok {
				t.Fatal("reset Input() returned false")
			}
			if len(resetInput.Musts()) != 1 {
				t.Fatalf("reset input musts = %d, want 1", len(resetInput.Musts()))
			}
			return
		}
	}
	t.Fatalf("top children = %v, want action reset", schemaChildNames(top.Children()))
}

func TestSchemaNodeReportsSourceYangVersion(t *testing.T) {
	const defaultSource = `module cambium-version-default {
    namespace "urn:cambium:version-default";
    prefix cvd;

    leaf name { type string; }
}`
	const yang11Source = `module cambium-version-yang11 {
    yang-version 1.1;
    namespace "urn:cambium:version-yang11";
    prefix cvy;

    leaf name { type string; }
}`
	builder, err := cambium.NewContextBuilder(cambium.ContextFlags{})
	if err != nil {
		t.Fatal(err)
	}
	if err := builder.LoadModuleStr(defaultSource); err != nil {
		t.Fatalf("LoadModuleStr default: %v", err)
	}
	if err := builder.LoadModuleStr(yang11Source); err != nil {
		t.Fatalf("LoadModuleStr yang11: %v", err)
	}
	ctx, err := builder.Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	defer ctx.Close()

	defaultMod, err := ctx.Schema("cambium-version-default")
	if err != nil {
		t.Fatalf("Schema default: %v", err)
	}
	if got, want := defaultMod.YangVersion(), "1"; got != want {
		t.Fatalf("default module YangVersion = %q, want %q", got, want)
	}
	defaultName, err := defaultMod.FindPath("/cvd:name")
	if err != nil {
		t.Fatalf("FindPath default name: %v", err)
	}
	if got, want := defaultName.YangVersion(), "1"; got != want {
		t.Fatalf("default node YangVersion = %q, want %q", got, want)
	}

	yang11Mod, err := ctx.Schema("cambium-version-yang11")
	if err != nil {
		t.Fatalf("Schema yang11: %v", err)
	}
	if got, want := yang11Mod.YangVersion(), "1.1"; got != want {
		t.Fatalf("yang11 module YangVersion = %q, want %q", got, want)
	}
	yang11Name, err := yang11Mod.FindPath("/cvy:name")
	if err != nil {
		t.Fatalf("FindPath yang11 name: %v", err)
	}
	if got, want := yang11Name.YangVersion(), "1.1"; got != want {
		t.Fatalf("yang11 node YangVersion = %q, want %q", got, want)
	}
}

func TestYang11ChoiceUnderChoiceShorthandCase(t *testing.T) {
	const source = `module cambium-yang11-choice-under-choice {
    yang-version 1.1;
    namespace "urn:cambium:yang11-choice-under-choice";
    prefix cycuc;

    container top {
        choice selector {
            leaf direct { type string; }
            choice nested {
                leaf value { type string; }
            }
            leaf after { type string; }
        }
    }
}`
	builder, err := cambium.NewContextBuilder(cambium.ContextFlags{})
	if err != nil {
		t.Fatal(err)
	}
	if err := builder.LoadModuleStr(source); err != nil {
		t.Fatalf("LoadModuleStr: %v", err)
	}
	ctx, err := builder.Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	defer ctx.Close()
	mod, err := ctx.Schema("cambium-yang11-choice-under-choice")
	if err != nil {
		t.Fatalf("Schema: %v", err)
	}
	top, err := mod.FindPath("/cycuc:top")
	if err != nil {
		t.Fatalf("FindPath top: %v", err)
	}
	selector := childByName(t, top.Children(), "selector")
	if selector.Kind() != cambium.SchemaNodeKindChoice {
		t.Fatalf("selector kind = %v, want Choice", selector.Kind())
	}
	if got := schemaChildNames(selector.Children()); strings.Join(got, ",") != "direct,nested,after" {
		t.Fatalf("selector children = %v, want [direct nested after]", got)
	}
	nestedCase := childByName(t, selector.Children(), "nested")
	if nestedCase.Kind() != cambium.SchemaNodeKindCase {
		t.Fatalf("nested shorthand case kind = %v, want Case", nestedCase.Kind())
	}
	nestedChoice := childByName(t, nestedCase.Children(), "nested")
	if nestedChoice.Kind() != cambium.SchemaNodeKindChoice {
		t.Fatalf("nested choice kind = %v, want Choice", nestedChoice.Kind())
	}
	if got := schemaChildNames(nestedChoice.Children()); strings.Join(got, ",") != "value" {
		t.Fatalf("nested choice children = %v, want [value]", got)
	}
}

func TestInvalidSubmoduleMetadataReturnsContextRuleCode(t *testing.T) {
	cases := []struct {
		name    string
		source  string
		message string
	}{
		{
			name: "missing belongs-to prefix",
			source: `submodule cambium-submodule-missing-prefix {
    belongs-to cambium-submodule-parent;
}`,
			message: `submodule belongs-to "cambium-submodule-parent" has no prefix`,
		},
		{
			name: "invalid revision",
			source: `submodule cambium-submodule-invalid-revision {
    belongs-to cambium-submodule-parent {
        prefix csp;
    }
    revision 2024-02-31;
}`,
			message: `invalid revision "2024-02-31"`,
		},
		{
			name: "revision duplicate reference",
			source: `submodule cambium-submodule-revision-duplicate-reference {
    belongs-to cambium-submodule-parent {
        prefix csp;
    }
    revision 2024-01-01 {
        reference "one";
        reference "two";
    }
}`,
			message: `duplicate reference in revision "2024-01-01"`,
		},
		{
			name: "revision invalid child",
			source: `submodule cambium-submodule-revision-invalid-child {
    belongs-to cambium-submodule-parent {
        prefix csp;
    }
    revision 2024-01-01 {
        default "bad";
    }
}`,
			message: `default "bad" is not valid under revision "2024-01-01"`,
		},
		{
			name: "invalid yang-version",
			source: `submodule cambium-submodule-invalid-yang-version {
    yang-version 2;
    belongs-to cambium-submodule-parent {
        prefix csp;
    }
}`,
			message: `invalid yang-version "2" in submodule "cambium-submodule-invalid-yang-version"`,
		},
		{
			name: "duplicate yang-version",
			source: `submodule cambium-submodule-duplicate-yang-version {
    yang-version 1;
    yang-version 1.1;
    belongs-to cambium-submodule-parent {
        prefix csp;
    }
}`,
			message: `duplicate yang-version in submodule "cambium-submodule-duplicate-yang-version"`,
		},
		{
			name: "yang-version invalid child",
			source: `submodule cambium-submodule-yang-version-invalid-child {
    yang-version 1 {
        description "bad";
    }
    belongs-to cambium-submodule-parent {
        prefix csp;
    }
}`,
			message: `description "bad" is not valid under yang-version "1"`,
		},
		{
			name: "duplicate belongs-to",
			source: `submodule cambium-submodule-duplicate-belongs-to {
    belongs-to cambium-submodule-parent {
        prefix csp;
    }
    belongs-to cambium-submodule-parent-alt {
        prefix cspa;
    }
}`,
			message: `duplicate belongs-to in submodule "cambium-submodule-duplicate-belongs-to"`,
		},
		{
			name: "duplicate belongs-to prefix",
			source: `submodule cambium-submodule-duplicate-belongs-to-prefix {
    belongs-to cambium-submodule-parent {
        prefix csp;
        prefix csp2;
    }
}`,
			message: `duplicate prefix in belongs-to "cambium-submodule-parent"`,
		},
		{
			name: "belongs-to prefix invalid child",
			source: `submodule cambium-submodule-belongs-to-prefix-invalid-child {
    belongs-to cambium-submodule-parent {
        prefix csp {
            description "bad";
        }
    }
}`,
			message: `description "bad" is not valid under prefix "csp"`,
		},
		{
			name: "belongs-to invalid child",
			source: `submodule cambium-submodule-belongs-to-invalid-child {
    belongs-to cambium-submodule-parent {
        prefix csp;
        default "bad";
    }
}`,
			message: `default "bad" is not valid under belongs-to "cambium-submodule-parent"`,
		},
		{
			name: "submodule namespace",
			source: `submodule cambium-submodule-namespace {
    namespace "urn:cambium:submodule-namespace";
    belongs-to cambium-submodule-parent {
        prefix csp;
    }
}`,
			message: `namespace "urn:cambium:submodule-namespace" is not valid in submodule "cambium-submodule-namespace"`,
		},
		{
			name: "submodule prefix",
			source: `submodule cambium-submodule-prefix {
    prefix csp;
    belongs-to cambium-submodule-parent {
        prefix parent;
    }
}`,
			message: `prefix "csp" is not valid in submodule "cambium-submodule-prefix"`,
		},
		{
			name: "duplicate contact",
			source: `submodule cambium-submodule-duplicate-contact {
    belongs-to cambium-submodule-parent {
        prefix csp;
    }
    contact "ops-a";
    contact "ops-b";
}`,
			message: `duplicate contact in submodule "cambium-submodule-duplicate-contact"`,
		},
		{
			name: "duplicate organization",
			source: `submodule cambium-submodule-duplicate-organization {
    belongs-to cambium-submodule-parent {
        prefix csp;
    }
    organization "team-a";
    organization "team-b";
}`,
			message: `duplicate organization in submodule "cambium-submodule-duplicate-organization"`,
		},
		{
			name: "duplicate description",
			source: `submodule cambium-submodule-duplicate-description {
    belongs-to cambium-submodule-parent {
        prefix csp;
    }
    description "one";
    description "two";
}`,
			message: `duplicate description in submodule "cambium-submodule-duplicate-description"`,
		},
		{
			name: "duplicate reference",
			source: `submodule cambium-submodule-duplicate-reference {
    belongs-to cambium-submodule-parent {
        prefix csp;
    }
    reference "one";
    reference "two";
}`,
			message: `duplicate reference in submodule "cambium-submodule-duplicate-reference"`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			builder, err := cambium.NewContextBuilder(cambium.ContextFlags{})
			if err != nil {
				t.Fatal(err)
			}
			err = builder.LoadModuleStr(tc.source)
			if err == nil {
				t.Fatal("LoadModuleStr accepted invalid submodule metadata")
			}
			var ce *cambium.Error
			if !errors.As(err, &ce) || ce.RuleCode() != cambium.RuleCodeContext {
				t.Fatalf("LoadModuleStr error = %v, want RuleCodeContext", err)
			}
			if !strings.Contains(err.Error(), tc.message) {
				t.Fatalf("LoadModuleStr error = %q, want to contain %q", err.Error(), tc.message)
			}
		})
	}
}

func TestInvalidIdentifierArgumentsReturnContextRuleCode(t *testing.T) {
	cases := []struct {
		name    string
		source  string
		message string
	}{
		{
			name: "module name contains space",
			source: `module "bad name" {
    namespace "urn:cambium:bad-name";
    prefix cbn;
    leaf value { type string; }
}`,
			message: `invalid identifier "bad name" for module`,
		},
		{
			name: "module prefix contains space",
			source: `module cambium-invalid-prefix-name {
    namespace "urn:cambium:invalid-prefix-name";
    prefix "bad prefix";
    leaf value { type string; }
}`,
			message: `invalid identifier "bad prefix" for prefix`,
		},
		{
			name: "leaf name contains space",
			source: `module cambium-invalid-leaf-name {
    namespace "urn:cambium:invalid-leaf-name";
    prefix ciln;
    leaf "bad name" { type string; }
}`,
			message: `invalid identifier "bad name" for leaf`,
		},
		{
			name: "enum name starts with digit",
			source: `module cambium-invalid-enum-name {
    namespace "urn:cambium:invalid-enum-name";
    prefix cien;
    leaf value {
        type enumeration {
            enum 9bad;
        }
    }
}`,
			message: `invalid identifier "9bad" for enum`,
		},
		{
			name: "yang 1 module name starts with xml",
			source: `module xml-invalid-yang1-name {
    namespace "urn:cambium:xml-invalid-yang1-name";
    prefix xiy;
    leaf value { type string; }
}`,
			message: `invalid identifier "xml-invalid-yang1-name" for module`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			builder, err := cambium.NewContextBuilder(cambium.ContextFlags{})
			if err != nil {
				t.Fatal(err)
			}
			err = builder.LoadModuleStr(tc.source)
			if err == nil {
				_, err = builder.Build()
			}
			if err == nil {
				t.Fatal("context accepted invalid identifier")
			}
			var ce *cambium.Error
			if !errors.As(err, &ce) || ce.RuleCode() != cambium.RuleCodeContext {
				t.Fatalf("context error = %v, want RuleCodeContext", err)
			}
			if !strings.Contains(err.Error(), tc.message) {
				t.Fatalf("context error = %q, want to contain %q", err.Error(), tc.message)
			}
		})
	}
}

func TestYang11XMLPrefixedIdentifiersAccepted(t *testing.T) {
	const source = `module xml-valid-yang11 {
    yang-version 1.1;
    namespace "urn:cambium:xml-valid-yang11";
    prefix xmlp;

    feature xmlFeature;

    typedef xmlType {
        type string;
    }

    identity xmlBase;
    identity xmlChild {
        base xmlBase;
    }

    grouping xmlGroup {
        leaf xmlGrouped {
            type xmlType;
        }
    }

    leaf xmlLeaf {
        type xmlType;
    }

    list xmlList {
        key "xmlKey";
        leaf xmlKey {
            type string;
        }
        uses xmlGroup;
    }
}`
	builder, err := cambium.NewContextBuilder(cambium.ContextFlags{})
	if err != nil {
		t.Fatal(err)
	}
	if err := builder.LoadModuleStr(source); err != nil {
		t.Fatalf("LoadModuleStr: %v", err)
	}
	ctx, err := builder.Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	defer ctx.Close()
	mod, err := ctx.Schema("xml-valid-yang11")
	if err != nil {
		t.Fatalf("Schema: %v", err)
	}
	if got := mod.Prefix(); got != "xmlp" {
		t.Fatalf("Prefix = %q, want xmlp", got)
	}
	if _, ok := mod.TopLevel().Lookup("xmlLeaf"); !ok {
		t.Fatal("xmlLeaf not found")
	}
	xmlList, ok := mod.TopLevel().Lookup("xmlList")
	if !ok {
		t.Fatal("xmlList not found")
	}
	if _, ok := xmlList.Children().Lookup("xmlGrouped"); !ok {
		t.Fatal("xmlGrouped from uses not found")
	}
	keys := xmlList.ListKeys()
	if keys.Len() != 1 {
		t.Fatalf("xmlList keys = %d, want 1", keys.Len())
	}
	key, _ := keys.Get(0)
	if key.Name() != "xmlKey" {
		t.Fatalf("key name = %q, want xmlKey", key.Name())
	}
	if got := identityByName(t, mod, "xmlChild").Name(); got != "xmlChild" {
		t.Fatalf("identity name = %q, want xmlChild", got)
	}
}

func TestInvalidDeviationUniqueConstraintsReturnContextRuleCode(t *testing.T) {
	target := `module cambium-deviation-unique-target {
    namespace "urn:cambium:deviation-unique-target";
    prefix cdut;

    list item {
        key "id";
        unique "code";
        leaf id { type string; }
        leaf code { type string; }
        leaf tag { type string; }
    }

    container top {
        leaf id { type string; }
    }
}`
	cases := []struct {
		name    string
		source  string
		message string
	}{
		{
			name: "empty deviation unique",
			source: `module cambium-deviation-empty-unique {
    namespace "urn:cambium:deviation-empty-unique";
    prefix cdeu;

    import cambium-deviation-unique-target {
        prefix target;
    }

    deviation "/target:item" {
        deviate add {
            unique "";
        }
    }
}`,
			message: `list "item" unique statement is empty`,
		},
		{
			name: "duplicate deviation unique",
			source: `module cambium-deviation-duplicate-unique {
    namespace "urn:cambium:deviation-duplicate-unique";
    prefix cddu;

    import cambium-deviation-unique-target {
        prefix target;
    }

    deviation "/target:item" {
        deviate add {
            unique "code";
        }
    }
}`,
			message: `duplicate unique constraint "code"`,
		},
		{
			name: "deviation unique unicode whitespace",
			source: `module cambium-deviation-unique-unicode-whitespace {
    namespace "urn:cambium:deviation-unique-unicode-whitespace";
    prefix cduuw;

    import cambium-deviation-unique-target {
        prefix target;
    }

    deviation "/target:item" {
        deviate add {
            unique "code` + "\u2003" + `tag";
        }
    }
}`,
			message: `unique path "code\u2003tag" does not reference a descendant leaf`,
		},
		{
			name: "deviation unique leading whitespace",
			source: `module cambium-deviation-unique-leading-whitespace {
    namespace "urn:cambium:deviation-unique-leading-whitespace";
    prefix cdulw;

    import cambium-deviation-unique-target {
        prefix target;
    }

    deviation "/target:item" {
        deviate add {
            unique " code";
        }
    }
}`,
			message: `list "item" unique statement has invalid identifier list " code"`,
		},
		{
			name: "deviation unique on non-list",
			source: `module cambium-deviation-unique-on-container {
    namespace "urn:cambium:deviation-unique-on-container";
    prefix cduoc;

    import cambium-deviation-unique-target {
        prefix target;
    }

    deviation "/target:top" {
        deviate add {
            unique "id";
        }
    }
}`,
			message: `unique at`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			builder, err := cambium.NewContextBuilder(cambium.ContextFlags{})
			if err != nil {
				t.Fatal(err)
			}
			if err := builder.LoadModuleStr(target); err != nil {
				t.Fatalf("Load target: %v", err)
			}
			if err := builder.LoadModuleStr(tc.source); err != nil {
				t.Fatalf("Load source: %v", err)
			}
			_, err = builder.Build()
			if err == nil {
				t.Fatal("Build accepted invalid deviation unique metadata")
			}
			var ce *cambium.Error
			if !errors.As(err, &ce) || ce.RuleCode() != cambium.RuleCodeContext {
				t.Fatalf("Build error = %v, want RuleCodeContext", err)
			}
			if !strings.Contains(err.Error(), tc.message) {
				t.Fatalf("Build error = %q, want to contain %q", err.Error(), tc.message)
			}
		})
	}
}

func TestListUniqueDescendantPathResolution(t *testing.T) {
	source := `module cambium-list-unique-descendant {
    namespace "urn:cambium:list-unique-descendant";
    prefix clud;
    list item {
        key "id";
        unique "nested/code";
        leaf id { type string; }
        container nested {
            leaf code { type string; }
        }
    }
}`
	builder, err := cambium.NewContextBuilder(cambium.ContextFlags{})
	if err != nil {
		t.Fatal(err)
	}
	if err := builder.LoadModuleStr(source); err != nil {
		t.Fatalf("LoadModuleStr: %v", err)
	}
	ctx, err := builder.Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	t.Cleanup(ctx.Close)
	mod, err := ctx.Schema("cambium-list-unique-descendant")
	if err != nil {
		t.Fatalf("Schema: %v", err)
	}
	list := schemaNodeAt(t, mod, "/clud:item")
	uniques := list.UniqueConstraints()
	if got, want := len(uniques), 1; got != want {
		t.Fatalf("UniqueConstraints len = %d, want %d", got, want)
	}
	leafs := uniques[0].Leafs()
	if got, want := len(leafs), 1; got != want {
		t.Fatalf("Unique leaf len = %d, want %d", got, want)
	}
	if got, want := leafs[0].Path(), "/cambium-list-unique-descendant/item/nested/code"; got != want {
		t.Fatalf("unique leaf path = %q, want %q", got, want)
	}
}

func TestListUniquePrefixedDescendantPathResolution(t *testing.T) {
	source := `module cambium-list-unique-prefixed-descendant {
    namespace "urn:cambium:list-unique-prefixed-descendant";
    prefix clupd;
    list item {
        key "id";
        unique "clupd:nested/clupd:code";
        leaf id { type string; }
        container nested {
            leaf code { type string; }
        }
    }
}`
	builder, err := cambium.NewContextBuilder(cambium.ContextFlags{})
	if err != nil {
		t.Fatal(err)
	}
	if err := builder.LoadModuleStr(source); err != nil {
		t.Fatalf("LoadModuleStr: %v", err)
	}
	ctx, err := builder.Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	t.Cleanup(ctx.Close)
	mod, err := ctx.Schema("cambium-list-unique-prefixed-descendant")
	if err != nil {
		t.Fatalf("Schema: %v", err)
	}
	list := schemaNodeAt(t, mod, "/clupd:item")
	uniques := list.UniqueConstraints()
	if got, want := len(uniques), 1; got != want {
		t.Fatalf("UniqueConstraints len = %d, want %d", got, want)
	}
	leafs := uniques[0].Leafs()
	if got, want := len(leafs), 1; got != want {
		t.Fatalf("Unique leaf len = %d, want %d", got, want)
	}
	if got, want := leafs[0].Path(), "/cambium-list-unique-prefixed-descendant/item/nested/code"; got != want {
		t.Fatalf("unique leaf path = %q, want %q", got, want)
	}
}

func TestInvalidTypeDefinitionsReturnContextRuleCode(t *testing.T) {
	cases := []struct {
		name    string
		source  string
		message string
	}{
		{
			name: "leaf missing type",
			source: `module cambium-leaf-missing-type {
    namespace "urn:cambium:leaf-missing-type";
    prefix clmt;
    leaf value;
}`,
			message: `leaf "value" has no type`,
		},
		{
			name: "leaf duplicate type",
			source: `module cambium-leaf-duplicate-type {
    namespace "urn:cambium:leaf-duplicate-type";
    prefix cldt;
    leaf value {
        type string;
        type uint8;
    }
}`,
			message: `leaf "value" has multiple type statements`,
		},
		{
			name: "leaf-list missing type",
			source: `module cambium-leaflist-missing-type {
    namespace "urn:cambium:leaflist-missing-type";
    prefix cllmt;
    leaf-list value;
}`,
			message: `leaf-list "value" has no type`,
		},
		{
			name: "unused grouping leaf missing type",
			source: `module cambium-unused-grouping-leaf-missing-type {
    namespace "urn:cambium:unused-grouping-leaf-missing-type";
    prefix cuglmt;
    grouping invalid {
        leaf value;
    }
    leaf ok { type string; }
}`,
			message: `leaf "value" has no type`,
		},
		{
			name: "container type statement",
			source: `module cambium-container-type {
    namespace "urn:cambium:container-type";
    prefix cct;
    container top {
        type string;
    }
}`,
			message: `type at`,
		},
		{
			name: "typedef duplicate type",
			source: `module cambium-typedef-duplicate-type {
    namespace "urn:cambium:typedef-duplicate-type";
    prefix ctdt;
    typedef dup {
        type string;
        type uint8;
    }
    leaf value { type dup; }
}`,
			message: `duplicate type in typedef "dup"`,
		},
		{
			name: "typedef invalid child",
			source: `module cambium-typedef-invalid-child {
    namespace "urn:cambium:typedef-invalid-child";
    prefix ctic;
    feature advanced;
    typedef alias {
        type string;
        if-feature advanced;
    }
    leaf value { type alias; }
}`,
			message: `if-feature "advanced" is not valid under typedef "alias"`,
		},
		{
			name: "unused typedef missing type",
			source: `module cambium-unused-typedef-missing-type {
    namespace "urn:cambium:unused-typedef-missing-type";
    prefix cutmt;
    typedef unused;
    leaf value { type string; }
}`,
			message: `typedef "unused" has no type`,
		},
		{
			name: "unused typedef duplicate type",
			source: `module cambium-unused-typedef-duplicate-type {
    namespace "urn:cambium:unused-typedef-duplicate-type";
    prefix cutdt;
    typedef unused {
        type string;
        type uint8;
    }
    leaf value { type string; }
}`,
			message: `duplicate type in typedef "unused"`,
		},
		{
			name: "unused typedef unknown type",
			source: `module cambium-unused-typedef-unknown-type {
    namespace "urn:cambium:unused-typedef-unknown-type";
    prefix cutut;
    typedef unused {
        type missing;
    }
    leaf value { type string; }
}`,
			message: `unknown type "missing"`,
		},
		{
			name: "unknown prefix builtin-looking type",
			source: `module cambium-type-unknown-prefix-builtin {
    namespace "urn:cambium:type-unknown-prefix-builtin";
    prefix ctupb;
    leaf value { type missing:string; }
}`,
			message: `unknown type "missing:string"`,
		},
		{
			name: "decimal64 missing fraction digits",
			source: `module cambium-decimal64-missing-fd {
    namespace "urn:cambium:decimal64-missing-fd";
    prefix cdmfd;
    leaf value { type decimal64; }
}`,
			message: `must have exactly one fraction-digits statement`,
		},
		{
			name: "decimal64 zero fraction digits",
			source: `module cambium-decimal64-zero-fd {
    namespace "urn:cambium:decimal64-zero-fd";
    prefix cdzfd;
    leaf value { type decimal64 { fraction-digits 0; } }
}`,
			message: `fraction-digits 0 outside 1..18`,
		},
		{
			name: "decimal64 high fraction digits",
			source: `module cambium-decimal64-high-fd {
    namespace "urn:cambium:decimal64-high-fd";
    prefix cdhfd;
    leaf value { type decimal64 { fraction-digits 19; } }
}`,
			message: `fraction-digits 19 outside 1..18`,
		},
		{
			name: "decimal64 leading zero fraction digits",
			source: `module cambium-decimal64-leading-zero-fd {
    namespace "urn:cambium:decimal64-leading-zero-fd";
    prefix cdlzfd;
    leaf value { type decimal64 { fraction-digits 02; } }
}`,
			message: `invalid fraction-digits "02"`,
		},
		{
			name: "fraction digits on string",
			source: `module cambium-fraction-digits-on-string {
    namespace "urn:cambium:fraction-digits-on-string";
    prefix cfdos;
    leaf value { type string { fraction-digits 2; } }
}`,
			message: `fraction-digits is not valid for type string`,
		},
		{
			name: "identityref missing base",
			source: `module cambium-identityref-missing-base {
    namespace "urn:cambium:identityref-missing-base";
    prefix cimb;
    leaf value { type identityref; }
}`,
			message: `identityref type`,
		},
		{
			name: "identityref duplicate base",
			source: `module cambium-identityref-duplicate-base {
    namespace "urn:cambium:identityref-duplicate-base";
    prefix cidb;
    identity root;
    leaf value {
        type identityref {
            base root;
            base root;
        }
    }
}`,
			message: `identityref type has duplicate base "root"`,
		},
		{
			name: "leafref missing path",
			source: `module cambium-leafref-missing-path {
    namespace "urn:cambium:leafref-missing-path";
    prefix clmp;
    leaf value { type leafref; }
}`,
			message: `must define exactly one non-empty path`,
		},
		{
			name: "leafref padded path",
			source: `module cambium-leafref-padded-path {
    namespace "urn:cambium:leafref-padded-path";
    prefix clpp;
    leaf target { type string; }
    leaf value {
        type leafref {
            path " /target ";
        }
    }
}`,
			message: `leafref path`,
		},
		{
			name: "leafref dot path",
			source: `module cambium-leafref-dot-path {
    namespace "urn:cambium:leafref-dot-path";
    prefix cldp;
    leaf target { type string; }
    leaf value {
        type leafref {
            path ".";
        }
    }
}`,
			message: `invalid leafref path "."`,
		},
		{
			name: "leafref unclosed predicate",
			source: `module cambium-leafref-unclosed-predicate {
    namespace "urn:cambium:leafref-unclosed-predicate";
    prefix clup;
    leaf target { type string; }
    leaf value {
        type leafref {
            path "/target[";
        }
    }
}`,
			message: `invalid leafref path "/target["`,
		},
		{
			name: "leafref unclosed predicate quote",
			source: `module cambium-leafref-unclosed-predicate-quote {
    namespace "urn:cambium:leafref-unclosed-predicate-quote";
    prefix clupq;
    leaf target { type string; }
    leaf value {
        type leafref {
            path "/target[name='x]";
        }
    }
}`,
			message: `invalid leafref path "/target[name='x]"`,
		},
		{
			name: "leafref invalid require-instance",
			source: `module cambium-leafref-invalid-require-instance {
    namespace "urn:cambium:leafref-invalid-require-instance";
    prefix cliri;
    leaf target { type string; }
    leaf value {
        type leafref {
            path /target;
            require-instance maybe;
        }
    }
}`,
			message: `invalid require-instance "maybe"`,
		},
		{
			name: "leafref require-instance requires yang 1.1",
			source: `module cambium-leafref-require-instance-yang10 {
    namespace "urn:cambium:leafref-require-instance-yang10";
    prefix clrityt;
    leaf target { type string; }
    leaf value {
        type leafref {
            path /target;
            require-instance false;
        }
    }
}`,
			message: `leafref type require-instance statement requires yang-version 1.1`,
		},
		{
			name: "leafref config target state require-instance",
			source: `module cambium-leafref-config-target-state {
    namespace "urn:cambium:leafref-config-target-state";
    prefix clcts;
    container top {
        leaf state-id {
            config false;
            type string;
        }
        leaf ref {
            type leafref {
                path "../state-id";
            }
        }
    }
}`,
			message: `leafref "ref" with require-instance true cannot target config false leaf "state-id"`,
		},
		{
			name: "instance-identifier invalid require-instance",
			source: `module cambium-instance-id-invalid-require-instance {
    namespace "urn:cambium:instance-id-invalid-require-instance";
    prefix ciiiri;
    leaf value {
        type instance-identifier {
            require-instance maybe;
        }
    }
}`,
			message: `invalid require-instance "maybe"`,
		},
		{
			name: "union missing member",
			source: `module cambium-union-missing-member {
    namespace "urn:cambium:union-missing-member";
    prefix cumm;
    leaf value { type union; }
}`,
			message: `union type`,
		},
		{
			name: "union leafref member requires yang 1.1",
			source: `module cambium-union-leafref-yang10 {
    namespace "urn:cambium:union-leafref-yang10";
    prefix culryt;
    leaf target { type string; }
    leaf value {
        type union {
            type leafref {
                path /target;
            }
            type string;
        }
    }
}`,
			message: `union member type "leafref" requires yang-version 1.1`,
		},
		{
			name: "union empty member requires yang 1.1",
			source: `module cambium-union-empty-yang10 {
    namespace "urn:cambium:union-empty-yang10";
    prefix cueyt;
    leaf value {
        type union {
            type empty;
            type string;
        }
    }
}`,
			message: `union member type "empty" requires yang-version 1.1`,
		},
		{
			name: "enumeration missing enum",
			source: `module cambium-enum-missing-value {
    namespace "urn:cambium:enum-missing-value";
    prefix cemv;
    leaf value { type enumeration; }
}`,
			message: `must define at least one enum`,
		},
		{
			name: "bits missing bit",
			source: `module cambium-bits-missing-value {
    namespace "urn:cambium:bits-missing-value";
    prefix cbmv;
    leaf value { type bits; }
}`,
			message: `must define at least one bit`,
		},
		{
			name: "duplicate enum name",
			source: `module cambium-enum-duplicate-name {
    namespace "urn:cambium:enum-duplicate-name";
    prefix cedn;
    leaf value { type enumeration { enum same; enum same; } }
}`,
			message: `duplicate enum name "same"`,
		},
		{
			name: "duplicate enum value",
			source: `module cambium-enum-duplicate-value {
    namespace "urn:cambium:enum-duplicate-value";
    prefix cedv;
    leaf value { type enumeration { enum one { value 1; } enum other { value 1; } } }
}`,
			message: `duplicate enum value 1`,
		},
		{
			name: "invalid enum value",
			source: `module cambium-enum-invalid-value {
    namespace "urn:cambium:enum-invalid-value";
    prefix ceiv;
    leaf value { type enumeration { enum too-high { value 2147483648; } } }
}`,
			message: `invalid value "2147483648"`,
		},
		{
			name: "enum positive plus value",
			source: `module cambium-enum-positive-plus-value {
    namespace "urn:cambium:enum-positive-plus-value";
    prefix ceppv;
    leaf value { type enumeration { enum plus { value +1; } } }
}`,
			message: `invalid value "+1"`,
		},
		{
			name: "enum positive leading zero value",
			source: `module cambium-enum-positive-leading-zero-value {
    namespace "urn:cambium:enum-positive-leading-zero-value";
    prefix ceplzv;
    leaf value { type enumeration { enum leading { value 01; } } }
}`,
			message: `invalid value "01"`,
		},
		{
			name: "enum duplicate value statement",
			source: `module cambium-enum-duplicate-value-statement {
    namespace "urn:cambium:enum-duplicate-value-statement";
    prefix cedvs;
    leaf value {
        type enumeration {
            enum one {
                value 1;
                value 2;
            }
        }
    }
}`,
			message: `enum "one" has multiple value statements`,
		},
		{
			name: "enum invalid status",
			source: `module cambium-enum-invalid-status {
    namespace "urn:cambium:enum-invalid-status";
    prefix ceis;
    leaf value {
        type enumeration {
            enum one {
                status future;
            }
        }
    }
}`,
			message: `invalid status "future"`,
		},
		{
			name: "enum duplicate description",
			source: `module cambium-enum-duplicate-description {
    namespace "urn:cambium:enum-duplicate-description";
    prefix cedd;
    leaf value {
        type enumeration {
            enum one {
                description "one";
                description "two";
            }
        }
    }
}`,
			message: `enum "one" has multiple description statements`,
		},
		{
			name: "enum invalid child",
			source: `module cambium-enum-invalid-child {
    namespace "urn:cambium:enum-invalid-child";
    prefix ceic;
    leaf value {
        type enumeration {
            enum one {
                type string;
            }
        }
    }
}`,
			message: `type "string" is not valid under enum "one"`,
		},
		{
			name: "duplicate bit name",
			source: `module cambium-bits-duplicate-name {
    namespace "urn:cambium:bits-duplicate-name";
    prefix cbdn;
    leaf value { type bits { bit same; bit same; } }
}`,
			message: `duplicate bit name "same"`,
		},
		{
			name: "duplicate bit position",
			source: `module cambium-bits-duplicate-position {
    namespace "urn:cambium:bits-duplicate-position";
    prefix cbdp;
    leaf value { type bits { bit one { position 1; } bit other { position 1; } } }
}`,
			message: `duplicate bit position 1`,
		},
		{
			name: "invalid bit position",
			source: `module cambium-bits-invalid-position {
    namespace "urn:cambium:bits-invalid-position";
    prefix cbip;
    leaf value { type bits { bit bad { position -1; } } }
}`,
			message: `invalid position "-1"`,
		},
		{
			name: "bit leading zero position",
			source: `module cambium-bits-leading-zero-position {
    namespace "urn:cambium:bits-leading-zero-position";
    prefix cblzp;
    leaf value { type bits { bit bad { position 01; } } }
}`,
			message: `invalid position "01"`,
		},
		{
			name: "bit duplicate position statement",
			source: `module cambium-bit-duplicate-position-statement {
    namespace "urn:cambium:bit-duplicate-position-statement";
    prefix cbdps;
    leaf value {
        type bits {
            bit one {
                position 1;
                position 2;
            }
        }
    }
}`,
			message: `bit "one" has multiple position statements`,
		},
		{
			name: "bit duplicate reference",
			source: `module cambium-bit-duplicate-reference {
    namespace "urn:cambium:bit-duplicate-reference";
    prefix cbdr;
    leaf value {
        type bits {
            bit one {
                reference "one";
                reference "two";
            }
        }
    }
}`,
			message: `bit "one" has multiple reference statements`,
		},
		{
			name: "bit invalid child",
			source: `module cambium-bit-invalid-child {
    namespace "urn:cambium:bit-invalid-child";
    prefix cbic;
    leaf value {
        type bits {
            bit one {
                default "one";
            }
        }
    }
}`,
			message: `default "one" is not valid under bit "one"`,
		},
		{
			name: "uint8 range out of bounds",
			source: `module cambium-range-out-of-bounds {
    namespace "urn:cambium:range-out-of-bounds";
    prefix croob;
    leaf value { type uint8 { range "0..256"; } }
}`,
			message: `invalid range bound "256" for uint8`,
		},
		{
			name: "derived range widens base",
			source: `module cambium-derived-range-widens-base {
    namespace "urn:cambium:derived-range-widens-base";
    prefix cdrwb;
    typedef narrow {
        type uint8 {
            range "10..20";
        }
    }
    leaf value {
        type narrow {
            range "0..30";
        }
    }
}`,
			message: `range restriction "0..30" is not within the base restriction`,
		},
		{
			name: "range on string",
			source: `module cambium-range-on-string {
    namespace "urn:cambium:range-on-string";
    prefix cros;
    leaf value { type string { range "1..10"; } }
}`,
			message: `range is not valid for type string`,
		},
		{
			name: "length on uint8",
			source: `module cambium-length-on-uint8 {
    namespace "urn:cambium:length-on-uint8";
    prefix clou;
    leaf value { type uint8 { length "1"; } }
}`,
			message: `length is not valid for type uint8`,
		},
		{
			name: "malformed range segment",
			source: `module cambium-range-malformed {
    namespace "urn:cambium:range-malformed";
    prefix crm;
    leaf value { type int32 { range "1..2..3"; } }
}`,
			message: `malformed segment`,
		},
		{
			name: "range unicode whitespace",
			source: `module cambium-range-unicode-whitespace {
    namespace "urn:cambium:range-unicode-whitespace";
    prefix cruw;
    leaf value { type int32 { range "1` + "\u2003" + `..` + "\u2003" + `10"; } }
}`,
			message: `invalid range bound`,
		},
		{
			name: "reversed range segment",
			source: `module cambium-range-reversed {
    namespace "urn:cambium:range-reversed";
    prefix crr;
    leaf value { type int32 { range "10..1"; } }
}`,
			message: `range segment "10..1" has lower bound greater than upper bound`,
		},
		{
			name: "unordered range segment",
			source: `module cambium-range-unordered {
    namespace "urn:cambium:range-unordered";
    prefix cru;
    leaf value { type int32 { range "10..20|1..5"; } }
}`,
			message: `range expression "10..20|1..5" has overlapping or unordered segment "1..5"`,
		},
		{
			name: "missing range bound",
			source: `module cambium-range-missing-bound {
    namespace "urn:cambium:range-missing-bound";
    prefix crmb;
    leaf value { type int32 { range "1.."; } }
}`,
			message: `missing bound`,
		},
		{
			name: "negative length",
			source: `module cambium-length-negative {
    namespace "urn:cambium:length-negative";
    prefix cln;
    leaf value { type string { length "-1"; } }
}`,
			message: `invalid length bound "-1"`,
		},
		{
			name: "length unicode whitespace",
			source: `module cambium-length-unicode-whitespace {
    namespace "urn:cambium:length-unicode-whitespace";
    prefix cluw;
    leaf value { type string { length "1` + "\u2003" + `..` + "\u2003" + `10"; } }
}`,
			message: `invalid length bound`,
		},
		{
			name: "reversed length segment",
			source: `module cambium-length-reversed {
    namespace "urn:cambium:length-reversed";
    prefix clr;
    leaf value { type string { length "10..1"; } }
}`,
			message: `length segment "10..1" has lower bound greater than upper bound`,
		},
		{
			name: "overlapping length segment",
			source: `module cambium-length-overlap {
    namespace "urn:cambium:length-overlap";
    prefix clo;
    leaf value { type string { length "1..10|5..20"; } }
}`,
			message: `length expression "1..10|5..20" has overlapping or unordered segment "5..20"`,
		},
		{
			name: "decimal64 range exponent",
			source: `module cambium-decimal64-range-exponent {
    namespace "urn:cambium:decimal64-range-exponent";
    prefix cdre;
    leaf value { type decimal64 { fraction-digits 2; range "1e2"; } }
}`,
			message: `invalid decimal64 range bound "1e2"`,
		},
		{
			name: "decimal64 range missing integer digits",
			source: `module cambium-decimal64-range-missing-integer-digits {
    namespace "urn:cambium:decimal64-range-missing-integer-digits";
    prefix cdrmid;
    leaf value { type decimal64 { fraction-digits 2; range ".1..2.0"; } }
}`,
			message: `invalid decimal64 range bound ".1"`,
		},
		{
			name: "multiple length statements",
			source: `module cambium-length-multiple {
    namespace "urn:cambium:length-multiple";
    prefix clm;
    leaf value { type string { length "1"; length "2"; } }
}`,
			message: `multiple length statements`,
		},
		{
			name: "range duplicate error-message",
			source: `module cambium-range-duplicate-error-message {
    namespace "urn:cambium:range-duplicate-error-message";
    prefix crdem;
    leaf value {
        type uint8 {
            range "1..10" {
                error-message "one";
                error-message "two";
            }
        }
    }
}`,
			message: `range "1..10" has multiple error-message statements`,
		},
		{
			name: "length duplicate reference",
			source: `module cambium-length-duplicate-reference {
    namespace "urn:cambium:length-duplicate-reference";
    prefix cldr;
    leaf value {
        type string {
            length "1..10" {
                reference "one";
                reference "two";
            }
        }
    }
}`,
			message: `length "1..10" has multiple reference statements`,
		},
		{
			name: "range invalid child",
			source: `module cambium-range-invalid-child {
    namespace "urn:cambium:range-invalid-child";
    prefix cric;
    leaf value {
        type uint8 {
            range "1..10" {
                default "1";
            }
        }
    }
}`,
			message: `default "1" is not valid under range "1..10"`,
		},
		{
			name: "length invalid child",
			source: `module cambium-length-invalid-child {
    namespace "urn:cambium:length-invalid-child";
    prefix clic;
    leaf value {
        type string {
            length "1..10" {
                modifier invert-match;
            }
        }
    }
}`,
			message: `modifier "invert-match" is not valid under length "1..10"`,
		},
		{
			name: "pattern invalid modifier",
			source: `module cambium-pattern-invalid-modifier {
    namespace "urn:cambium:pattern-invalid-modifier";
    prefix cpim;
    leaf value {
        type string {
            pattern "[a-z]+" {
                modifier other;
            }
        }
    }
}`,
			message: `invalid pattern modifier "other"`,
		},
		{
			name: "pattern duplicate modifier",
			source: `module cambium-pattern-duplicate-modifier {
    namespace "urn:cambium:pattern-duplicate-modifier";
    prefix cpdm;
    leaf value {
        type string {
            pattern "[a-z]+" {
                modifier invert-match;
                modifier invert-match;
            }
        }
    }
}`,
			message: `pattern "[a-z]+" has multiple modifier statements`,
		},
		{
			name: "pattern duplicate error-app-tag",
			source: `module cambium-pattern-duplicate-error-app-tag {
    namespace "urn:cambium:pattern-duplicate-error-app-tag";
    prefix cpdeat;
    leaf value {
        type string {
            pattern "[a-z]+" {
                error-app-tag "one";
                error-app-tag "two";
            }
        }
    }
}`,
			message: `pattern "[a-z]+" has multiple error-app-tag statements`,
		},
		{
			name: "pattern duplicate error-message",
			source: `module cambium-pattern-duplicate-error-message {
    namespace "urn:cambium:pattern-duplicate-error-message";
    prefix cpdem;
    leaf value {
        type string {
            pattern "[a-z]+" {
                error-message "one";
                error-message "two";
            }
        }
    }
}`,
			message: `pattern "[a-z]+" has multiple error-message statements`,
		},
		{
			name: "pattern invalid child",
			source: `module cambium-pattern-invalid-child {
    namespace "urn:cambium:pattern-invalid-child";
    prefix cpic;
    leaf value {
        type string {
            pattern "[a-z]+" {
                default "abc";
            }
        }
    }
}`,
			message: `default "abc" is not valid under pattern "[a-z]+"`,
		},
		{
			name: "pattern invalid regexp",
			source: `module cambium-pattern-invalid-regexp {
    namespace "urn:cambium:pattern-invalid-regexp";
    prefix cpir;
    leaf value {
        type string {
            pattern "[";
        }
    }
}`,
			message: `invalid pattern "["`,
		},
		{
			name: "pattern unknown escape",
			source: `module cambium-pattern-unknown-escape {
    namespace "urn:cambium:pattern-unknown-escape";
    prefix cpue;
    leaf value {
        type string {
            pattern "\\q";
        }
    }
}`,
			message: `invalid pattern "\\q"`,
		},
		{
			name: "pattern unknown category",
			source: `module cambium-pattern-unknown-category {
    namespace "urn:cambium:pattern-unknown-category";
    prefix cpuc;
    leaf value {
        type string {
            pattern "\\p{DefinitelyNotACategory}";
        }
    }
}`,
			message: `invalid pattern "\\p{DefinitelyNotACategory}"`,
		},
		{
			name: "pattern unknown unicode block",
			source: `module cambium-pattern-unknown-unicode-block {
    namespace "urn:cambium:pattern-unknown-unicode-block";
    prefix cpuub;
    leaf value {
        type string {
            pattern "\\p{IsDefinitelyNotACategory}";
        }
    }
}`,
			message: `invalid pattern "\\p{IsDefinitelyNotACategory}"`,
		},
		{
			name: "pattern empty character class",
			source: `module cambium-pattern-empty-character-class {
    namespace "urn:cambium:pattern-empty-character-class";
    prefix cpecc;
    leaf value {
        type string {
            pattern "[]";
        }
    }
}`,
			message: `invalid pattern "[]"`,
		},
		{
			name: "pattern repeated quantifier",
			source: `module cambium-pattern-repeated-quantifier {
    namespace "urn:cambium:pattern-repeated-quantifier";
    prefix cprq;
    leaf value {
        type string {
            pattern "a**";
        }
    }
}`,
			message: `invalid pattern "a**"`,
		},
		{
			name: "pattern reversed character range",
			source: `module cambium-pattern-reversed-character-range {
    namespace "urn:cambium:pattern-reversed-character-range";
    prefix cprcr;
    leaf value {
        type string {
            pattern "[z-a]";
        }
    }
}`,
			message: `invalid pattern "[z-a]"`,
		},
		{
			name: "pattern reversed quantifier range",
			source: `module cambium-pattern-reversed-quantifier-range {
    namespace "urn:cambium:pattern-reversed-quantifier-range";
    prefix cprqr;
    leaf value {
        type string {
            pattern "a{2,1}";
        }
    }
}`,
			message: `invalid pattern "a{2,1}"`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			builder, err := cambium.NewContextBuilder(cambium.ContextFlags{})
			if err != nil {
				t.Fatal(err)
			}
			if err := builder.LoadModuleStr(tc.source); err != nil {
				t.Fatalf("LoadModuleStr: %v", err)
			}
			_, err = builder.Build()
			if err == nil {
				t.Fatal("Build accepted invalid type definition")
			}
			var ce *cambium.Error
			if !errors.As(err, &ce) || ce.RuleCode() != cambium.RuleCodeContext {
				t.Fatalf("Build error = %v, want RuleCodeContext", err)
			}
			if !strings.Contains(err.Error(), tc.message) {
				t.Fatalf("Build error = %q, want to contain %q", err.Error(), tc.message)
			}
		})
	}
}

func TestImportedPrefixBuiltinLookingTypeReturnsContextRuleCode(t *testing.T) {
	builder, err := cambium.NewContextBuilder(cambium.ContextFlags{})
	if err != nil {
		t.Fatal(err)
	}
	target := `module cambium-type-prefix-target {
    namespace "urn:cambium:type-prefix-target";
    prefix ctpt;
    leaf target { type string; }
}`
	if err := builder.LoadModuleStr(target); err != nil {
		t.Fatalf("LoadModuleStr target: %v", err)
	}
	user := `module cambium-type-imported-prefix-builtin {
    namespace "urn:cambium:type-imported-prefix-builtin";
    prefix ctipb;
    import cambium-type-prefix-target { prefix ctpt; }
    leaf value { type ctpt:string; }
}`
	if err := builder.LoadModuleStr(user); err != nil {
		t.Fatalf("LoadModuleStr user: %v", err)
	}
	_, err = builder.Build()
	if err == nil {
		t.Fatal("Build accepted imported prefixed builtin-looking type")
	}
	var ce *cambium.Error
	if !errors.As(err, &ce) || ce.RuleCode() != cambium.RuleCodeContext {
		t.Fatalf("Build error = %v, want RuleCodeContext", err)
	}
	if !strings.Contains(err.Error(), `unknown type "ctpt:string"`) {
		t.Fatalf("Build error = %q, want unknown type ctpt:string", err.Error())
	}
}

const introspectionModuleName = "cambium-introspection-demo"

func introspectionContext(t *testing.T) (ctx *cambium.Context, cleanup func()) {
	t.Helper()
	dir := schemaIntrospectionModuleDir(t)
	path := filepath.Join(dir, introspectionModuleName+".yang")
	yang := `module cambium-introspection-demo {
    namespace "urn:cambium:introspection";
    prefix cid;
    yang-version 1.1;
    revision 2026-06-13;

    identity base-id;
    identity mid-id {
        base base-id;
    }
    identity leaf-id {
        base mid-id;
    }

    typedef small-number {
        type uint16;
        units "widgets";
        default "7";
        status deprecated;
        description "Small number typedef.";
        reference "small-number-ref";
    }
    typedef plain-text {
        type string;
    }

    extension camelcase-name {
        argument value {
            yin-element true;
        }
        status deprecated;
        description "Camelcase extension.";
        reference "camel-ref";
    }
    extension flag-foo {
    }

    container top {
        description "Top-level ordered container.";
        leaf rw-flag {
            cid:flag-foo;
            cid:camelcase-name "fooBar";
            type boolean;
            default "true";
        }
        leaf-list multi-defaults {
            type string;
            default "alpha";
            default "beta";
            default "gamma";
        }
        leaf ro-counter {
            config false;
            type uint64;
        }
        leaf deprecated-leaf {
            status deprecated;
            type string;
        }
        leaf mandatory-leaf {
            mandatory "true";
            type string;
            must "../rw-flag = 'true'" {
                error-message "rw-flag must be true";
                error-app-tag "must-rw-flag";
            }
        }
        container presence-box {
            presence "Explicit presence container.";
            when "../rw-flag = 'true'" {
                description "Only present when rw-flag is true";
                reference "RFC 6020 when statement";
            }
            leaf inner {
                type string;
                units "packets";
            }
        }
        leaf-list typed-leaf-list {
            ordered-by user;
            type string;
            min-elements 1;
            max-elements 10;
        }
        list keyed-list {
            key "name color";
            unique "name extra";
            leaf name {
                type string;
            }
            leaf color {
                type string;
            }
            leaf extra {
                type string;
            }
        }
        leaf all-builtins {
            type int64;
        }
        leaf dec64 {
            type decimal64 {
                fraction-digits 4;
                range "0..100";
            }
        }
        leaf status-enum {
            type enumeration {
                enum up { value 1; }
                enum down { value 2; }
                enum unknown { value 0; }
            }
        }
        leaf flags-bits {
            type bits {
                bit read;
                bit write;
                bit execute;
            }
        }
        leaf uni {
            type union {
                type string;
                type int32;
            }
        }
        leaf ranged-int {
            type int32 {
                range "1..10|20..max" {
                    error-app-tag "range-tag";
                    reference "range-ref";
                }
            }
        }
        leaf ranged-dec64 {
            type decimal64 {
                fraction-digits 2;
                range "0..100";
            }
        }
        leaf constrained-string {
            type string {
                length "1..255" {
                    error-message "length failed";
                    description "String length bounds.";
                }
                pattern "[a-zA-Z0-9_-]+" {
                    error-message "pattern failed";
                    error-app-tag "my-tag";
                    description "Allowed identifier characters.";
                    reference "pattern-ref";
                }
                pattern "^foo.*" {
                    modifier invert-match;
                }
            }
        }
        leaf idref {
            type identityref {
                base base-id;
            }
        }
        leaf ref-to-name {
            type leafref {
                path "/cid:top/keyed-list/name";
            }
        }
        choice preference {
            case primary {
                leaf primary-name {
                    type string;
                }
            }
            case secondary {
                leaf secondary-name {
                    type string;
                }
            }
        }
        anyxml raw-data;
        action reset {
            input {
                leaf in-val {
                    type string;
                }
            }
            output {
                leaf out-val {
                    type string;
                }
            }
        }
    }

    rpc reboot {
        input {
            leaf delay {
                type uint32;
            }
        }
        output {
            leaf result {
                type string;
            }
        }
    }

    notification event {
        leaf severity {
            type string;
        }
    }
}
`
	writeModuleFile(t, path, []byte(yang))

	ctx, err := cambium.NewContext()
	if err != nil {
		t.Fatal(err)
	}
	if err := ctx.SetSearchPath(dir); err != nil {
		t.Fatal(err)
	}
	if err := ctx.LoadModule(introspectionModuleName); err != nil {
		t.Fatal(err)
	}
	return ctx, func() { ctx.Close() }
}

func topNode(t *testing.T, ctx *cambium.Context) cambium.SchemaNodeRef {
	t.Helper()
	mod, err := ctx.Schema(introspectionModuleName)
	if err != nil {
		t.Fatalf("Schema: %v", err)
	}
	top, err := mod.FindPath("/cambium-introspection-demo:top")
	if err != nil {
		t.Fatalf("FindPath top: %v", err)
	}
	return top
}

func childByName(t *testing.T, children cambium.SchemaChildren, name string) cambium.SchemaNodeRef {
	t.Helper()
	for i := 0; i < children.Len(); i++ {
		c, _ := children.Get(i)
		if c.Name() == name {
			return c
		}
	}
	t.Fatalf("child %q not found", name)
	return cambium.SchemaNodeRef{}
}

func schemaChildNames(children cambium.SchemaChildren) []string {
	out := make([]string, children.Len())
	for i := 0; i < children.Len(); i++ {
		c, _ := children.Get(i)
		out[i] = c.Name()
	}
	return out
}

func TestModuleMetadata(t *testing.T) {
	ctx, cleanup := introspectionContext(t)
	defer cleanup()

	mod, err := ctx.Schema(introspectionModuleName)
	if err != nil {
		t.Fatalf("Schema: %v", err)
	}
	if got, want := mod.Name(), introspectionModuleName; got != want {
		t.Fatalf("name = %q, want %q", got, want)
	}
	if got, want := mod.Namespace(), "urn:cambium:introspection"; got != want {
		t.Fatalf("namespace = %q, want %q", got, want)
	}
	if got, want := mod.Prefix(), "cid"; got != want {
		t.Fatalf("prefix = %q, want %q", got, want)
	}
	rev, ok := mod.Revision()
	if !ok || rev != "2026-06-13" {
		t.Fatalf("revision = %q, ok=%v, want 2026-06-13", rev, ok)
	}
	if !mod.IsImplemented() {
		t.Fatal("expected module to be implemented")
	}

	byNamespace, err := ctx.Schema("urn:cambium:introspection")
	if err != nil {
		t.Fatalf("Schema namespace lookup: %v", err)
	}
	if got := byNamespace.Name(); got != introspectionModuleName {
		t.Fatalf("namespace lookup module = %q, want %q", got, introspectionModuleName)
	}
}

func TestChildrenDeclarationOrder(t *testing.T) {
	ctx, cleanup := introspectionContext(t)
	defer cleanup()

	mod, err := ctx.Schema(introspectionModuleName)
	if err != nil {
		t.Fatal(err)
	}
	top := topNode(t, ctx)

	topChildren := top.Children()
	want := []string{
		"rw-flag",
		"multi-defaults",
		"ro-counter",
		"deprecated-leaf",
		"mandatory-leaf",
		"presence-box",
		"typed-leaf-list",
		"keyed-list",
		"all-builtins",
		"dec64",
		"status-enum",
		"flags-bits",
		"uni",
		"ranged-int",
		"ranged-dec64",
		"constrained-string",
		"idref",
		"ref-to-name",
		"preference",
		"raw-data",
		"reset",
	}
	if topChildren.Len() != len(want) {
		t.Fatalf("top has %d children, want %d", topChildren.Len(), len(want))
	}
	for i, name := range want {
		c, _ := topChildren.Get(i)
		if c.Name() != name {
			t.Fatalf("child[%d] = %q, want %q", i, c.Name(), name)
		}
	}

	// Top-level of module should be just "top".
	modTop := mod.TopLevel()
	if modTop.Len() != 1 {
		t.Fatalf("module top-level has %d children, want 1", modTop.Len())
	}
	c, _ := modTop.Get(0)
	if c.Name() != "top" {
		t.Fatalf("module top-level[0] = %q, want top", c.Name())
	}
}

func TestChildrenCrossClassOrderIsGrouped(t *testing.T) {
	dir := schemaIntrospectionModuleDir(t)
	module := `module cambium-cross-class-order {
    namespace "urn:cambium:cross-class-order";
    prefix cco;
    yang-version 1.1;
    revision 2026-06-16;

    container top {
        leaf before {
            type string;
        }
        action operate {
            input {
                leaf arg {
                    type string;
                }
            }
        }
        leaf after {
            type string;
        }
        notification event {
            leaf severity {
                type string;
            }
        }
        leaf tail {
            type string;
        }
    }
}
`
	writeModuleFile(t, filepath.Join(dir, "cambium-cross-class-order.yang"), []byte(module))

	ctx, err := cambium.NewContext()
	if err != nil {
		t.Fatal(err)
	}
	defer ctx.Close()
	if err := ctx.SetSearchPath(dir); err != nil {
		t.Fatal(err)
	}
	if err := ctx.LoadModule("cambium-cross-class-order"); err != nil {
		t.Fatalf("LoadModule: %v", err)
	}
	mod, err := ctx.Schema("cambium-cross-class-order")
	if err != nil {
		t.Fatalf("Schema: %v", err)
	}
	top, err := mod.FindPath("/cco:top")
	if err != nil {
		t.Fatalf("FindPath top: %v", err)
	}

	got := schemaChildNames(top.Children())
	want := []string{"before", "operate", "after", "event", "tail"}
	if len(got) != len(want) {
		t.Fatalf("Children() = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("Children() = %v, want %v", got, want)
		}
	}

	gotData := schemaChildNames(top.DataChildren(false))
	wantData := []string{"before", "after", "tail"}
	if len(gotData) != len(wantData) {
		t.Fatalf("DataChildren(false) = %v, want %v", gotData, wantData)
	}
	for i := range wantData {
		if gotData[i] != wantData[i] {
			t.Fatalf("DataChildren(false) = %v, want %v", gotData, wantData)
		}
	}
}

func TestSchemaNodeConfigStatusMandatory(t *testing.T) {
	ctx, cleanup := introspectionContext(t)
	defer cleanup()

	top := topNode(t, ctx)
	children := top.Children()

	rw := childByName(t, children, "rw-flag")
	if rw.Config() != cambium.ConfigRw {
		t.Fatalf("rw-flag config = %v, want Rw", rw.Config())
	}
	if rw.Status() != cambium.StatusCurrent {
		t.Fatalf("rw-flag status = %v, want Current", rw.Status())
	}
	if rw.IsMandatory() {
		t.Fatal("rw-flag should not be mandatory")
	}

	ro := childByName(t, children, "ro-counter")
	if ro.Config() != cambium.ConfigRo {
		t.Fatalf("ro-counter config = %v, want Ro", ro.Config())
	}

	dep := childByName(t, children, "deprecated-leaf")
	if dep.Status() != cambium.StatusDeprecated {
		t.Fatalf("deprecated-leaf status = %v, want Deprecated", dep.Status())
	}

	mand := childByName(t, children, "mandatory-leaf")
	if !mand.IsMandatory() {
		t.Fatal("mandatory-leaf should be mandatory")
	}

	pc := childByName(t, children, "presence-box")
	if !pc.IsPresenceContainer() {
		t.Fatal("presence-box should be a presence container")
	}
	inner := childByName(t, pc.Children(), "inner")
	units, ok := inner.Units()
	if !ok || units != "packets" {
		t.Fatalf("inner units = %q, ok=%v, want packets", units, ok)
	}

	ll := childByName(t, children, "typed-leaf-list")
	if ll.OrderedBy() != cambium.OrderedByUser {
		t.Fatalf("typed-leaf-list ordered-by = %v, want User", ll.OrderedBy())
	}
	minElems, ok := ll.MinElements()
	if !ok || minElems != 1 {
		t.Fatalf("typed-leaf-list min = %d, ok=%v, want 1", minElems, ok)
	}
	maxElems, ok := ll.MaxElements()
	if !ok || maxElems != 10 {
		t.Fatalf("typed-leaf-list max = %d, ok=%v, want 10", maxElems, ok)
	}

	// Regression: libyang sets LYS_ORDBY_SYSTEM (0x80) on every system-ordered
	// (the default) list/leaf-list in the compiled tree, and that bit aliases
	// LYS_PRESENCE (0x80). The presence read must be gated to containers, so a
	// system-ordered keyed-list must NOT report as a presence container.
	kl := childByName(t, children, "keyed-list")
	if kl.IsPresenceContainer() {
		t.Fatal("keyed-list is system-ordered, not a presence container (0x80 ORDBY_SYSTEM aliases LYS_PRESENCE)")
	}
}

func TestConfigFalseInheritedByDescendants(t *testing.T) {
	t.Helper()
	dir := schemaIntrospectionModuleDir(t)
	module := `module cambium-config-inheritance {
    namespace "urn:cambium:config-inheritance";
    prefix cci;
    yang-version 1.1;

    container system {
        leaf hostname {
            type string;
        }
        container state {
            config false;
            leaf uptime {
                type uint64;
            }
            container counters {
                leaf packets {
                    type uint64;
                }
            }
        }
    }
}
`
	writeModuleFile(t, filepath.Join(dir, "cambium-config-inheritance.yang"), []byte(module))

	ctx, err := cambium.NewContext()
	if err != nil {
		t.Fatal(err)
	}
	defer ctx.Close()
	if err := ctx.SetSearchPath(dir); err != nil {
		t.Fatal(err)
	}
	if err := ctx.LoadModule("cambium-config-inheritance"); err != nil {
		t.Fatalf("LoadModule: %v", err)
	}
	mod, err := ctx.Schema("cambium-config-inheritance")
	if err != nil {
		t.Fatalf("Schema: %v", err)
	}

	system, err := mod.FindPath("/cci:system")
	if err != nil {
		t.Fatalf("FindPath system: %v", err)
	}
	hostname, err := mod.FindPath("/cci:system/cci:hostname")
	if err != nil {
		t.Fatalf("FindPath hostname: %v", err)
	}
	state, err := mod.FindPath("/cci:system/cci:state")
	if err != nil {
		t.Fatalf("FindPath state: %v", err)
	}
	uptime, err := mod.FindPath("/cci:system/cci:state/cci:uptime")
	if err != nil {
		t.Fatalf("FindPath uptime: %v", err)
	}
	counters, err := mod.FindPath("/cci:system/cci:state/cci:counters")
	if err != nil {
		t.Fatalf("FindPath counters: %v", err)
	}
	packets, err := mod.FindPath("/cci:system/cci:state/cci:counters/cci:packets")
	if err != nil {
		t.Fatalf("FindPath packets: %v", err)
	}

	for name, node := range map[string]cambium.SchemaNodeRef{
		"system":   system,
		"hostname": hostname,
	} {
		if node.Config() != cambium.ConfigRw || node.ReadOnly() {
			t.Fatalf("%s config = %v/readOnly=%v, want ConfigRw/false", name, node.Config(), node.ReadOnly())
		}
	}
	for name, node := range map[string]cambium.SchemaNodeRef{
		"state":    state,
		"uptime":   uptime,
		"counters": counters,
		"packets":  packets,
	} {
		if node.Config() != cambium.ConfigRo || !node.ReadOnly() {
			t.Fatalf("%s config = %v/readOnly=%v, want ConfigRo/true", name, node.Config(), node.ReadOnly())
		}
	}
}

func TestRefineConfigFalseInheritedByDescendants(t *testing.T) {
	t.Helper()
	dir := schemaIntrospectionModuleDir(t)
	module := `module cambium-refine-config-inheritance {
    namespace "urn:cambium:refine-config-inheritance";
    prefix crci;
    yang-version 1.1;

    grouping state-group {
        container state {
            leaf uptime {
                type uint64;
            }
            container counters {
                leaf packets {
                    type uint64;
                }
            }
        }
    }

    container system {
        uses state-group {
            refine state {
                config false;
            }
        }
    }
}
`
	writeModuleFile(t, filepath.Join(dir, "cambium-refine-config-inheritance.yang"), []byte(module))

	ctx, err := cambium.NewContext()
	if err != nil {
		t.Fatal(err)
	}
	defer ctx.Close()
	if err := ctx.SetSearchPath(dir); err != nil {
		t.Fatal(err)
	}
	if err := ctx.LoadModule("cambium-refine-config-inheritance"); err != nil {
		t.Fatalf("LoadModule: %v", err)
	}
	mod, err := ctx.Schema("cambium-refine-config-inheritance")
	if err != nil {
		t.Fatalf("Schema: %v", err)
	}

	system, err := mod.FindPath("/crci:system")
	if err != nil {
		t.Fatalf("FindPath system: %v", err)
	}
	if system.Config() != cambium.ConfigRw || system.ReadOnly() {
		t.Fatalf("system config = %v/readOnly=%v, want ConfigRw/false", system.Config(), system.ReadOnly())
	}

	for _, path := range []string{
		"/crci:system/crci:state",
		"/crci:system/crci:state/crci:uptime",
		"/crci:system/crci:state/crci:counters",
		"/crci:system/crci:state/crci:counters/crci:packets",
	} {
		node, err := mod.FindPath(path)
		if err != nil {
			t.Fatalf("FindPath %s: %v", path, err)
		}
		if node.Config() != cambium.ConfigRo || !node.ReadOnly() {
			t.Fatalf("%s config = %v/readOnly=%v, want ConfigRo/true", path, node.Config(), node.ReadOnly())
		}
	}
}

func TestRefineConfigFalseAllowsKeylessDescendantList(t *testing.T) {
	t.Helper()
	dir := schemaIntrospectionModuleDir(t)
	module := `module cambium-refine-config-keyless-list {
    namespace "urn:cambium:refine-config-keyless-list";
    prefix crckl;
    yang-version 1.1;

    grouping state-group {
        container state {
            list event {
                leaf seq {
                    type uint32;
                }
                leaf message {
                    type string;
                }
            }
        }
    }

    container system {
        uses state-group {
            refine state {
                config false;
            }
        }
    }
}
`
	writeModuleFile(t, filepath.Join(dir, "cambium-refine-config-keyless-list.yang"), []byte(module))

	ctx, err := cambium.NewContext()
	if err != nil {
		t.Fatal(err)
	}
	defer ctx.Close()
	if err := ctx.SetSearchPath(dir); err != nil {
		t.Fatal(err)
	}
	if err := ctx.LoadModule("cambium-refine-config-keyless-list"); err != nil {
		t.Fatalf("LoadModule: %v", err)
	}
	mod, err := ctx.Schema("cambium-refine-config-keyless-list")
	if err != nil {
		t.Fatalf("Schema: %v", err)
	}
	event, err := mod.FindPath("/crckl:system/crckl:state/crckl:event")
	if err != nil {
		t.Fatalf("FindPath event: %v", err)
	}
	if event.Config() != cambium.ConfigRo || !event.ReadOnly() {
		t.Fatalf("event config = %v/readOnly=%v, want ConfigRo/true", event.Config(), event.ReadOnly())
	}
	if keys := event.ListKeys(); keys.Len() != 0 {
		t.Fatalf("event ListKeys len = %d, want 0", keys.Len())
	}
}

func TestRefineConfigFalseRefreshesUniqueConfigClassification(t *testing.T) {
	t.Helper()
	dir := schemaIntrospectionModuleDir(t)
	module := `module cambium-refine-config-unique-state {
    namespace "urn:cambium:refine-config-unique-state";
    prefix crcus;
    yang-version 1.1;

    grouping state-group {
        container state {
            list item {
                key "id";
                unique "name counter";
                leaf id {
                    type string;
                }
                leaf name {
                    type string;
                }
                leaf counter {
                    config false;
                    type uint64;
                }
            }
        }
    }

    container system {
        uses state-group {
            refine state {
                config false;
            }
        }
    }
}
`
	writeModuleFile(t, filepath.Join(dir, "cambium-refine-config-unique-state.yang"), []byte(module))

	ctx, err := cambium.NewContext()
	if err != nil {
		t.Fatal(err)
	}
	defer ctx.Close()
	if err := ctx.SetSearchPath(dir); err != nil {
		t.Fatal(err)
	}
	if err := ctx.LoadModule("cambium-refine-config-unique-state"); err != nil {
		t.Fatalf("LoadModule: %v", err)
	}
	mod, err := ctx.Schema("cambium-refine-config-unique-state")
	if err != nil {
		t.Fatalf("Schema: %v", err)
	}
	item, err := mod.FindPath("/crcus:system/crcus:state/crcus:item")
	if err != nil {
		t.Fatalf("FindPath item: %v", err)
	}
	if item.Config() != cambium.ConfigRo || !item.ReadOnly() {
		t.Fatalf("item config = %v/readOnly=%v, want ConfigRo/true", item.Config(), item.ReadOnly())
	}
	ucs := item.UniqueConstraints()
	if len(ucs) != 1 {
		t.Fatalf("unique constraints = %d, want 1", len(ucs))
	}
	leafs := ucs[0].Leafs()
	want := []string{"name", "counter"}
	if len(leafs) != len(want) {
		t.Fatalf("unique leafs = %d, want %d", len(leafs), len(want))
	}
	for i, name := range want {
		if leafs[i].Name() != name {
			t.Fatalf("unique leaf[%d] = %q, want %q", i, leafs[i].Name(), name)
		}
		if leafs[i].Config() != cambium.ConfigRo {
			t.Fatalf("unique leaf[%d] config = %v, want ConfigRo", i, leafs[i].Config())
		}
	}
}

func TestRefineConfigFalseRefreshesKeyConfigClassification(t *testing.T) {
	t.Helper()
	dir := schemaIntrospectionModuleDir(t)
	module := `module cambium-refine-config-key-state {
    namespace "urn:cambium:refine-config-key-state";
    prefix crcks;
    yang-version 1.1;

    grouping state-group {
        container state {
            list item {
                key "id";
                leaf id {
                    config false;
                    type string;
                }
                leaf value {
                    type string;
                }
            }
        }
    }

    container system {
        uses state-group {
            refine state {
                config false;
            }
        }
    }
}
`
	writeModuleFile(t, filepath.Join(dir, "cambium-refine-config-key-state.yang"), []byte(module))

	ctx, err := cambium.NewContext()
	if err != nil {
		t.Fatal(err)
	}
	defer ctx.Close()
	if err := ctx.SetSearchPath(dir); err != nil {
		t.Fatal(err)
	}
	if err := ctx.LoadModule("cambium-refine-config-key-state"); err != nil {
		t.Fatalf("LoadModule: %v", err)
	}
	mod, err := ctx.Schema("cambium-refine-config-key-state")
	if err != nil {
		t.Fatalf("Schema: %v", err)
	}
	item, err := mod.FindPath("/crcks:system/crcks:state/crcks:item")
	if err != nil {
		t.Fatalf("FindPath item: %v", err)
	}
	if item.Config() != cambium.ConfigRo || !item.ReadOnly() {
		t.Fatalf("item config = %v/readOnly=%v, want ConfigRo/true", item.Config(), item.ReadOnly())
	}
	keys := item.ListKeys()
	if keys.Len() != 1 {
		t.Fatalf("item ListKeys len = %d, want 1", keys.Len())
	}
	key, ok := keys.Get(0)
	if !ok {
		t.Fatal("item key[0] missing")
	}
	if key.Name() != "id" || key.Config() != cambium.ConfigRo {
		t.Fatalf("item key = %q/config %v, want id/ConfigRo", key.Name(), key.Config())
	}
}

func TestUsesIfFeatureOnKeyedGroupingDoesNotInvalidateKey(t *testing.T) {
	t.Helper()
	dir := schemaIntrospectionModuleDir(t)
	module := `module cambium-uses-if-feature-keyed-list {
    namespace "urn:cambium:uses-if-feature-keyed-list";
    prefix cuifkl;
    yang-version 1.1;

    feature advanced;

    grouping keyed-group {
        list item {
            key "id";
            leaf id {
                type string;
            }
            leaf value {
                type string;
            }
        }
    }

    container top {
        uses keyed-group {
            if-feature advanced;
        }
    }
}
`
	writeModuleFile(t, filepath.Join(dir, "cambium-uses-if-feature-keyed-list.yang"), []byte(module))

	ctx, err := cambium.NewContext()
	if err != nil {
		t.Fatal(err)
	}
	defer ctx.Close()
	if err := ctx.SetSearchPath(dir); err != nil {
		t.Fatal(err)
	}
	if err := ctx.SetFeatures("cambium-uses-if-feature-keyed-list", []string{"advanced"}); err != nil {
		t.Fatal(err)
	}
	if err := ctx.LoadModule("cambium-uses-if-feature-keyed-list"); err != nil {
		t.Fatalf("LoadModule: %v", err)
	}
	mod, err := ctx.Schema("cambium-uses-if-feature-keyed-list")
	if err != nil {
		t.Fatalf("Schema: %v", err)
	}
	item, err := mod.FindPath("/cuifkl:top/cuifkl:item")
	if err != nil {
		t.Fatalf("FindPath item: %v", err)
	}
	keys := item.ListKeys()
	if keys.Len() != 1 {
		t.Fatalf("item ListKeys len = %d, want 1", keys.Len())
	}
	key, ok := keys.Get(0)
	if !ok {
		t.Fatal("item key[0] missing")
	}
	if key.Name() != "id" {
		t.Fatalf("item key[0] = %q, want id", key.Name())
	}
	if got, want := key.IfFeatures(), []string{"advanced"}; len(got) != len(want) || got[0] != want[0] {
		t.Fatalf("key IfFeatures = %v, want %v", got, want)
	}
}

func TestListKeysStatementOrder(t *testing.T) {
	ctx, cleanup := introspectionContext(t)
	defer cleanup()

	top := topNode(t, ctx)
	list := childByName(t, top.Children(), "keyed-list")
	keys := list.ListKeys()
	want := []string{"name", "color"}
	if keys.Len() != len(want) {
		t.Fatalf("list has %d keys, want %d", keys.Len(), len(want))
	}
	for i, name := range want {
		k, _ := keys.Get(i)
		if k.Name() != name {
			t.Fatalf("key[%d] = %q, want %q", i, k.Name(), name)
		}
	}
}

func TestYang11EmptyListKeyAccepted(t *testing.T) {
	const source = `module cambium-yang11-empty-list-key {
    yang-version 1.1;
    namespace "urn:cambium:yang11-empty-list-key";
    prefix cyelk;

    list item {
        key "id";
        leaf id { type empty; }
        leaf label { type string; }
    }
}`
	builder, err := cambium.NewContextBuilder(cambium.ContextFlags{})
	if err != nil {
		t.Fatal(err)
	}
	if err := builder.LoadModuleStr(source); err != nil {
		t.Fatalf("LoadModuleStr: %v", err)
	}
	ctx, err := builder.Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	defer ctx.Close()
	mod, err := ctx.Schema("cambium-yang11-empty-list-key")
	if err != nil {
		t.Fatalf("Schema: %v", err)
	}
	item, err := mod.FindPath("/cyelk:item")
	if err != nil {
		t.Fatalf("FindPath item: %v", err)
	}
	keys := item.ListKeys()
	if keys.Len() != 1 {
		t.Fatalf("item keys = %d, want 1", keys.Len())
	}
	id, _ := keys.Get(0)
	if id.Name() != "id" {
		t.Fatalf("key name = %q, want id", id.Name())
	}
	if !id.IsListKey() {
		t.Fatal("id should report IsListKey")
	}
	info, ok := id.LeafType()
	if !ok {
		t.Fatal("id LeafType returned false")
	}
	if _, ok := info.Resolved().(cambium.ResolvedEmpty); !ok {
		t.Fatalf("id resolved type = %T, want ResolvedEmpty", info.Resolved())
	}
}

func TestSchemaNodeKindAnyxml(t *testing.T) {
	ctx, cleanup := introspectionContext(t)
	defer cleanup()

	top := topNode(t, ctx)
	// anyxml and anydata are distinct YANG statements (RFC 7950 section 7.11
	// anyxml, section 7.10 anydata) and map to distinct schema kinds; raw-data
	// is anyxml.
	// Regression guard: anyxml must NOT collapse back into SchemaNodeKindAnyData.
	anyx := childByName(t, top.Children(), "raw-data")
	if anyx.Kind() != cambium.SchemaNodeKindAnyXML {
		t.Fatalf("raw-data (anyxml) kind = %v, want AnyXML", anyx.Kind())
	}
	if !anyx.IsAnyXML() || anyx.IsAnyData() {
		t.Fatalf("raw-data predicates: IsAnyXML=%v IsAnyData=%v, want true,false", anyx.IsAnyXML(), anyx.IsAnyData())
	}
	if got := anyx.Kind().String(); got != "anyxml" {
		t.Fatalf("raw-data kind string = %q, want anyxml", got)
	}
	if got := anyx.Statement().Keyword(); got != "anyxml" {
		t.Fatalf("raw-data statement keyword = %q, want anyxml", got)
	}
}

func TestSchemaNodeKindAnyDataAnyXMLDistinct(t *testing.T) {
	dir := t.TempDir()
	const name = "cambium-anydata-anyxml-kinds"
	yang := `module cambium-anydata-anyxml-kinds {
    namespace "urn:cambium:anydata-anyxml-kinds";
    prefix caak;
    yang-version 1.1;
    container top {
        anydata payload;
        anyxml raw-data;
    }
}`
	writeModuleFile(t, filepath.Join(dir, name+".yang"), []byte(yang))

	ctx, err := cambium.NewContext()
	if err != nil {
		t.Fatal(err)
	}
	defer ctx.Close()
	if err := ctx.SetSearchPath(dir); err != nil {
		t.Fatal(err)
	}
	if err := ctx.LoadModule(name); err != nil {
		t.Fatal(err)
	}
	mod, err := ctx.Schema(name)
	if err != nil {
		t.Fatalf("Schema: %v", err)
	}
	top := childByName(t, mod.TopLevel(), "top")

	payload := childByName(t, top.Children(), "payload")
	if payload.Kind() != cambium.SchemaNodeKindAnyData {
		t.Fatalf("payload kind = %v, want AnyData", payload.Kind())
	}
	if !payload.IsAnyData() || payload.IsAnyXML() {
		t.Fatalf("payload predicates: IsAnyData=%v IsAnyXML=%v, want true,false", payload.IsAnyData(), payload.IsAnyXML())
	}
	if got := payload.Kind().String(); got != "anydata" {
		t.Fatalf("payload kind string = %q, want anydata", got)
	}
	if got := payload.Statement().Keyword(); got != "anydata" {
		t.Fatalf("payload statement keyword = %q, want anydata", got)
	}

	raw := childByName(t, top.Children(), "raw-data")
	if raw.Kind() != cambium.SchemaNodeKindAnyXML {
		t.Fatalf("raw-data kind = %v, want AnyXML", raw.Kind())
	}
	if !raw.IsAnyXML() || raw.IsAnyData() {
		t.Fatalf("raw-data predicates: IsAnyXML=%v IsAnyData=%v, want true,false", raw.IsAnyXML(), raw.IsAnyData())
	}
	if got := raw.Kind().String(); got != "anyxml" {
		t.Fatalf("raw-data kind string = %q, want anyxml", got)
	}
	if got := raw.Statement().Keyword(); got != "anyxml" {
		t.Fatalf("raw-data statement keyword = %q, want anyxml", got)
	}
}

func TestIdentityDerivedClosure(t *testing.T) {
	ctx, cleanup := introspectionContext(t)
	defer cleanup()

	mod, err := ctx.Schema(introspectionModuleName)
	if err != nil {
		t.Fatal(err)
	}
	var base cambium.Identity
	var mid cambium.Identity
	var leaf cambium.Identity
	for id := range mod.Identities() {
		switch id.Name() {
		case "base-id":
			base = id
		case "mid-id":
			mid = id
		case "leaf-id":
			leaf = id
		}
	}
	if base.Name() == "" {
		t.Fatal("base-id identity not found")
	}
	if mid.Name() == "" {
		t.Fatal("mid-id identity not found")
	}
	if leaf.Name() == "" {
		t.Fatal("leaf-id identity not found")
	}

	midBases := mid.Bases()
	if len(midBases) != 1 {
		t.Fatalf("mid-id bases = %d, want 1", len(midBases))
	}
	if got, want := midBases[0].Name(), "base-id"; got != want {
		t.Fatalf("mid-id base = %q, want %q", got, want)
	}
	leafBases := leaf.Bases()
	if len(leafBases) != 1 {
		t.Fatalf("leaf-id bases = %d, want 1", len(leafBases))
	}
	if got, want := leafBases[0].Name(), "mid-id"; got != want {
		t.Fatalf("leaf-id base = %q, want %q", got, want)
	}

	derived := base.Derived()
	want := []string{"mid-id", "leaf-id"}
	if len(derived) != len(want) {
		t.Fatalf("derived = %v, want %v", namesOf(derived), want)
	}
	for i, name := range want {
		if derived[i].Name() != name {
			t.Fatalf("derived[%d] = %q, want %q", i, derived[i].Name(), name)
		}
	}
}

func TestIdentityMetadataAccessors(t *testing.T) {
	source := `module cambium-identity-metadata {
    namespace "urn:cambium:identity-metadata";
    prefix cim;

    identity base {
        description "Base identity.";
        reference "Base reference.";
        status deprecated;
    }

    identity child {
        base base;
    }
}`

	builder, err := cambium.NewContextBuilder(cambium.ContextFlags{})
	if err != nil {
		t.Fatalf("NewContextBuilder: %v", err)
	}
	if err := builder.LoadModuleStr(source); err != nil {
		t.Fatalf("LoadModuleStr: %v", err)
	}
	ctx, err := builder.Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	defer ctx.Close()

	mod, err := ctx.Schema("cambium-identity-metadata")
	if err != nil {
		t.Fatalf("Schema: %v", err)
	}
	base := identityByName(t, mod, "base")
	if got, ok := base.Description(); !ok || got != "Base identity." {
		t.Fatalf("Description = (%q,%v), want Base identity.,true", got, ok)
	}
	if got, ok := base.Reference(); !ok || got != "Base reference." {
		t.Fatalf("Reference = (%q,%v), want Base reference.,true", got, ok)
	}
	if got := base.Status(); got != cambium.StatusDeprecated {
		t.Fatalf("Status = %v, want StatusDeprecated", got)
	}

	child := identityByName(t, mod, "child")
	if got, ok := child.Description(); ok || got != "" {
		t.Fatalf("child Description = (%q,%v), want empty,false", got, ok)
	}
	if got, ok := child.Reference(); ok || got != "" {
		t.Fatalf("child Reference = (%q,%v), want empty,false", got, ok)
	}
	if got := child.Status(); got != cambium.StatusCurrent {
		t.Fatalf("child Status = %v, want StatusCurrent", got)
	}
}

func TestIdentityBasesFromSubmodule(t *testing.T) {
	dir := schemaIntrospectionModuleDir(t)

	extModule := `module cambium-submodule-identity-ext {
    namespace "urn:cambium:submodule-identity-ext";
    prefix csie;
    yang-version 1.1;
    revision 2026-06-16;

    identity external-base;
}
`
	writeModuleFile(t, filepath.Join(dir, "cambium-submodule-identity-ext.yang"), []byte(extModule))
	module := `module cambium-submodule-identity {
    namespace "urn:cambium:submodule-identity";
    prefix csi;
    yang-version 1.1;
    include cambium-submodule-identity-part;
    revision 2026-06-16;
}
`
	writeModuleFile(t, filepath.Join(dir, "cambium-submodule-identity.yang"), []byte(module))
	submodule := `submodule cambium-submodule-identity-part {
    yang-version 1.1;
    belongs-to cambium-submodule-identity {
        prefix csi;
    }
    import cambium-submodule-identity-ext {
        prefix ext;
    }
    revision 2026-06-16;

    identity base-sub-id;
    identity derived-sub-id {
        base base-sub-id;
    }
    identity imported-derived-sub-id {
        base ext:external-base;
    }
}
`
	writeModuleFile(t, filepath.Join(dir, "cambium-submodule-identity-part.yang"), []byte(submodule))

	ctx, err := cambium.NewContext()
	if err != nil {
		t.Fatal(err)
	}
	defer ctx.Close()
	if err := ctx.SetSearchPath(dir); err != nil {
		t.Fatal(err)
	}
	if err := ctx.LoadModule("cambium-submodule-identity"); err != nil {
		t.Fatalf("LoadModule: %v", err)
	}
	mod, err := ctx.Schema("cambium-submodule-identity")
	if err != nil {
		t.Fatalf("Schema: %v", err)
	}
	imports := mod.Imports()
	if len(imports) != 1 {
		t.Fatalf("Imports() len = %d, want 1", len(imports))
	}
	if got, want := imports[0].Name, "cambium-submodule-identity-ext"; got != want {
		t.Fatalf("Imports()[0].Name = %q, want %q", got, want)
	}
	if got, want := imports[0].Prefix, "ext"; got != want {
		t.Fatalf("Imports()[0].Prefix = %q, want %q", got, want)
	}
	if resolved, ok := mod.ResolvePrefix("ext"); !ok || resolved.Name() != "cambium-submodule-identity-ext" {
		t.Fatalf("ResolvePrefix(ext) = (%q,%v), want cambium-submodule-identity-ext,true", resolved.Name(), ok)
	}

	var derived cambium.Identity
	var importedDerived cambium.Identity
	for id := range mod.Identities() {
		switch id.Name() {
		case "derived-sub-id":
			derived = id
		case "imported-derived-sub-id":
			importedDerived = id
		}
	}
	if derived.Name() == "" {
		t.Fatal("derived-sub-id identity not found")
	}
	if importedDerived.Name() == "" {
		t.Fatal("imported-derived-sub-id identity not found")
	}
	bases := derived.Bases()
	if len(bases) != 1 {
		t.Fatalf("derived-sub-id bases = %d, want 1", len(bases))
	}
	if got, want := bases[0].Name(), "base-sub-id"; got != want {
		t.Fatalf("derived-sub-id base = %q, want %q", got, want)
	}

	importedBases := importedDerived.Bases()
	if len(importedBases) != 1 {
		t.Fatalf("imported-derived-sub-id bases = %d, want 1", len(importedBases))
	}
	if got, want := importedBases[0].Name(), "external-base"; got != want {
		t.Fatalf("imported-derived-sub-id base = %q, want %q", got, want)
	}
	if got, want := importedBases[0].Module().Name(), "cambium-submodule-identity-ext"; got != want {
		t.Fatalf("imported-derived-sub-id base module = %q, want %q", got, want)
	}
}

func TestIdentityDerivedClosureIsFullyTransitive(t *testing.T) {
	t.Helper()
	dir := schemaIntrospectionModuleDir(t)
	module := `module cambium-identity-transitive {
    namespace "urn:cambium:identity-transitive";
    prefix cit;
    yang-version 1.1;

    identity root;
    identity level-one {
        base root;
    }
    identity level-two {
        base level-one;
    }
    identity level-three {
        base level-two;
    }
}
`
	writeModuleFile(t, filepath.Join(dir, "cambium-identity-transitive.yang"), []byte(module))

	ctx, err := cambium.NewContext()
	if err != nil {
		t.Fatal(err)
	}
	defer ctx.Close()
	if err := ctx.SetSearchPath(dir); err != nil {
		t.Fatal(err)
	}
	if err := ctx.LoadModule("cambium-identity-transitive"); err != nil {
		t.Fatalf("LoadModule: %v", err)
	}
	mod, err := ctx.Schema("cambium-identity-transitive")
	if err != nil {
		t.Fatalf("Schema: %v", err)
	}

	var root cambium.Identity
	for id := range mod.Identities() {
		if id.Name() == "root" {
			root = id
			break
		}
	}
	if root.Name() == "" {
		t.Fatal("root identity not found")
	}
	got := namesOf(root.Derived())
	want := []string{"level-one", "level-two", "level-three"}
	if len(got) != len(want) {
		t.Fatalf("root derived = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("root derived = %v, want %v", got, want)
		}
	}
}

func TestIdentityCrossModuleDerivationConformance(t *testing.T) {
	mod := loadConformanceSchema(t, "identity-cross-module-derivation")
	baseMod, ok := mod.ResolvePrefix("base")
	if !ok {
		t.Fatal("ResolvePrefix(base) returned false")
	}
	base := identityByName(t, baseMod, "transport-protocol")
	assertStringSlices(t, "transport-protocol derived", namesOf(base.Derived()), []string{"tcp", "udp"})

	leaf := schemaNodeAt(t, mod, "/proto")
	info, ok := leaf.LeafType()
	if !ok {
		t.Fatal("expected proto leaf type")
	}
	idref, ok := info.Resolved().(cambium.ResolvedIdentityRef)
	if !ok {
		t.Fatalf("expected identityref, got %T", info.Resolved())
	}
	bases := idref.Bases()
	if len(bases) != 1 {
		t.Fatalf("proto bases = %d, want 1", len(bases))
	}
	if got, want := bases[0].Name(), "transport-protocol"; got != want {
		t.Fatalf("proto base = %q, want %q", got, want)
	}
	if got, want := bases[0].Module().Name(), "identity-cross-module-derivation-base"; got != want {
		t.Fatalf("proto base module = %q, want %q", got, want)
	}
}

func TestIdentityMultiBaseCrossModuleConformance(t *testing.T) {
	mod := loadConformanceSchema(t, "identity-multi-base-cross-module")
	roleMod, ok := mod.ResolvePrefix("a")
	if !ok {
		t.Fatal("ResolvePrefix(a) returned false")
	}
	hardwareMod, ok := mod.ResolvePrefix("b")
	if !ok {
		t.Fatal("ResolvePrefix(b) returned false")
	}
	assertStringSlices(t, "interface-role derived", namesOf(identityByName(t, roleMod, "interface-role").Derived()), []string{"high-speed-interface"})
	assertStringSlices(t, "hardware-feature derived", namesOf(identityByName(t, hardwareMod, "hardware-feature").Derived()), []string{"high-speed-interface"})

	highSpeed := identityByName(t, mod, "high-speed-interface")
	bases := highSpeed.Bases()
	if len(bases) != 2 {
		t.Fatalf("high-speed-interface bases = %d, want 2", len(bases))
	}
	if got, want := bases[0].Module().Name()+":"+bases[0].Name(), "identity-multi-base-cross-module-a:interface-role"; got != want {
		t.Fatalf("high-speed-interface base[0] = %q, want %q", got, want)
	}
	if got, want := bases[1].Module().Name()+":"+bases[1].Name(), "identity-multi-base-cross-module-b:hardware-feature"; got != want {
		t.Fatalf("high-speed-interface base[1] = %q, want %q", got, want)
	}

	leaf := schemaNodeAt(t, mod, "/type")
	info, ok := leaf.LeafType()
	if !ok {
		t.Fatal("expected type leaf type")
	}
	idref, ok := info.Resolved().(cambium.ResolvedIdentityRef)
	if !ok {
		t.Fatalf("expected identityref, got %T", info.Resolved())
	}
	refBases := idref.Bases()
	if len(refBases) != 1 {
		t.Fatalf("type bases = %d, want 1", len(refBases))
	}
	if got, want := refBases[0].Module().Name()+":"+refBases[0].Name(), "identity-multi-base-cross-module-a:interface-role"; got != want {
		t.Fatalf("type base = %q, want %q", got, want)
	}
}

func namesOf(ids []cambium.Identity) []string {
	out := make([]string, len(ids))
	for i, id := range ids {
		out[i] = id.Name()
	}
	return out
}

func identityByName(t *testing.T, mod cambium.Module, name string) cambium.Identity {
	t.Helper()
	for id := range mod.Identities() {
		if id.Name() == name {
			return id
		}
	}
	t.Fatalf("identity %s:%s not found", mod.Name(), name)
	return cambium.Identity{}
}

func TestBaseTypeString(t *testing.T) {
	cases := []struct {
		base cambium.BaseType
		want string
	}{
		{cambium.BaseTypeString, "string"},
		{cambium.BaseTypeBoolean, "boolean"},
		{cambium.BaseTypeInt8, "int8"},
		{cambium.BaseTypeInt16, "int16"},
		{cambium.BaseTypeInt32, "int32"},
		{cambium.BaseTypeInt64, "int64"},
		{cambium.BaseTypeUint8, "uint8"},
		{cambium.BaseTypeUint16, "uint16"},
		{cambium.BaseTypeUint32, "uint32"},
		{cambium.BaseTypeUint64, "uint64"},
		{cambium.BaseTypeDecimal64, "decimal64"},
		{cambium.BaseTypeEmpty, "empty"},
		{cambium.BaseTypeBinary, "binary"},
		{cambium.BaseTypeBits, "bits"},
		{cambium.BaseTypeEnumeration, "enumeration"},
		{cambium.BaseTypeIdentityRef, "identityref"},
		{cambium.BaseTypeInstanceIdentifier, "instance-identifier"},
		{cambium.BaseTypeLeafRef, "leafref"},
		{cambium.BaseTypeUnion, "union"},
		{cambium.BaseTypeUnknown, "unknown"},
	}
	for _, tc := range cases {
		if got := tc.base.String(); got != tc.want {
			t.Fatalf("%v.String() = %q, want %q", tc.base, got, tc.want)
		}
	}
}

func TestKeyNamesStatementOrder(t *testing.T) {
	ctx, cleanup := introspectionContext(t)
	defer cleanup()

	top := topNode(t, ctx)
	list := childByName(t, top.Children(), "keyed-list")
	got := list.KeyNames()
	want := []string{"name", "color"}
	if len(got) != len(want) {
		t.Fatalf("KeyNames() = %v, want %v", got, want)
	}
	for i, name := range want {
		if got[i] != name {
			t.Fatalf("KeyNames()[%d] = %q, want %q", i, got[i], name)
		}
	}
}

func TestLeafTypeBaseKindAllBuiltins(t *testing.T) {
	ctx, cleanup := introspectionContext(t)
	defer cleanup()

	top := topNode(t, ctx)
	children := top.Children()

	rw := childByName(t, children, "rw-flag")
	info, ok := rw.LeafType()
	if !ok || info.Base() != cambium.BaseTypeBoolean {
		t.Fatalf("rw-flag base = %v, ok=%v, want Boolean", info.Base(), ok)
	}

	ro := childByName(t, children, "ro-counter")
	info, ok = ro.LeafType()
	if !ok || info.Base() != cambium.BaseTypeUint64 {
		t.Fatalf("ro-counter base = %v, want Uint64", info.Base())
	}

	all := childByName(t, children, "all-builtins")
	info, ok = all.LeafType()
	if !ok || info.Base() != cambium.BaseTypeInt64 {
		t.Fatalf("all-builtins base = %v, want Int64", info.Base())
	}

	pc := childByName(t, children, "presence-box")
	inner := childByName(t, pc.Children(), "inner")
	info, ok = inner.LeafType()
	if !ok || info.Base() != cambium.BaseTypeString {
		t.Fatalf("inner base = %v, want String", info.Base())
	}
}

func TestLeafTypeTypedefChain(t *testing.T) {
	builder, err := cambium.NewContextBuilder(cambium.ContextFlags{})
	if err != nil {
		t.Fatal(err)
	}
	source := `module cambium-typedef-chain {
    namespace "urn:cambium:typedef-chain";
    prefix ctc;

    typedef inner {
        type uint8;
    }
    typedef middle {
        type inner;
    }
    typedef outer {
        type middle;
    }

    leaf value {
        type outer;
    }
}
`
	if err := builder.LoadModuleStr(source); err != nil {
		t.Fatalf("LoadModuleStr: %v", err)
	}
	ctx, err := builder.Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	defer ctx.Close()

	mod, err := ctx.Schema("cambium-typedef-chain")
	if err != nil {
		t.Fatalf("Schema: %v", err)
	}
	value, err := mod.FindPath("/ctc:value")
	if err != nil {
		t.Fatalf("FindPath value: %v", err)
	}
	info, ok := value.LeafType()
	if !ok {
		t.Fatal("expected leaf type")
	}
	if got := info.Base(); got != cambium.BaseTypeUint8 {
		t.Fatalf("Base = %v, want uint8", got)
	}
	if got, ok := info.TypedefName(); !ok || got != "outer" {
		t.Fatalf("TypedefName = (%q,%v), want (outer,true)", got, ok)
	}
	gotChain := info.TypedefChain()
	wantChain := []string{"outer", "middle", "inner"}
	if strings.Join(gotChain, ",") != strings.Join(wantChain, ",") {
		t.Fatalf("TypedefChain = %v, want %v", gotChain, wantChain)
	}
}

func TestLeafTypeDecimal64FractionDigits(t *testing.T) {
	ctx, cleanup := introspectionContext(t)
	defer cleanup()

	top := topNode(t, ctx)
	dec := childByName(t, top.Children(), "dec64")
	info, ok := dec.LeafType()
	if !ok {
		t.Fatal("expected leaf type")
	}
	r, ok := info.Resolved().(cambium.ResolvedDecimal64)
	if !ok {
		t.Fatalf("expected decimal64, got %T", info.Resolved())
	}
	if got := r.FractionDigits().Value(); got != 4 {
		t.Fatalf("fraction-digits = %d, want 4", got)
	}
}

func TestLeafTypeEnumValuesOrdered(t *testing.T) {
	ctx, cleanup := introspectionContext(t)
	defer cleanup()

	top := topNode(t, ctx)
	enm := childByName(t, top.Children(), "status-enum")
	info, ok := enm.LeafType()
	if !ok {
		t.Fatal("expected leaf type")
	}
	r, ok := info.Resolved().(cambium.ResolvedEnumeration)
	if !ok {
		t.Fatalf("expected enumeration, got %T", info.Resolved())
	}
	vals := r.Values()
	if len(vals) != 3 {
		t.Fatalf("enum values = %d, want 3", len(vals))
	}
	names := []string{vals[0].Name(), vals[1].Name(), vals[2].Name()}
	values := []int64{vals[0].Value(), vals[1].Value(), vals[2].Value()}
	wantNames := []string{"up", "down", "unknown"}
	wantValues := []int64{1, 2, 0}
	for i := range wantNames {
		if names[i] != wantNames[i] || values[i] != wantValues[i] {
			t.Fatalf("enum[%d] = (%q, %d), want (%q, %d)", i, names[i], values[i], wantNames[i], wantValues[i])
		}
	}
}

func TestLeafTypeBitsPositionsOrdered(t *testing.T) {
	ctx, cleanup := introspectionContext(t)
	defer cleanup()

	top := topNode(t, ctx)
	bits := childByName(t, top.Children(), "flags-bits")
	info, ok := bits.LeafType()
	if !ok {
		t.Fatal("expected leaf type")
	}
	r, ok := info.Resolved().(cambium.ResolvedBits)
	if !ok {
		t.Fatalf("expected bits, got %T", info.Resolved())
	}
	vals := r.Values()
	if len(vals) != 3 {
		t.Fatalf("bits values = %d, want 3", len(vals))
	}
	names := []string{vals[0].Name(), vals[1].Name(), vals[2].Name()}
	want := []string{"read", "write", "execute"}
	for i := range want {
		if names[i] != want[i] {
			t.Fatalf("bits[%d] = %q, want %q", i, names[i], want[i])
		}
	}
}

func TestResolvedTypeSlicesAreDefensiveCopies(t *testing.T) {
	source := `module cambium-resolved-copy {
    namespace "urn:cambium:resolved-copy";
    prefix crc;
    yang-version 1.1;

    container top {
        leaf ranged-int {
            type int32 {
                range "1..10";
            }
        }
        leaf ranged-dec64 {
            type decimal64 {
                fraction-digits 2;
                range "0..100";
            }
        }
        leaf bounded-binary {
            type binary {
                length "1..16";
            }
        }
        leaf constrained-string {
            type string {
                length "2..32";
                pattern "[a-z]+";
            }
        }
        leaf enum-leaf {
            type enumeration {
                enum first;
                enum second;
            }
        }
        leaf bits-leaf {
            type bits {
                bit read;
                bit write;
            }
        }
    }
}`
	builder, err := cambium.NewContextBuilder(cambium.ContextFlags{})
	if err != nil {
		t.Fatal(err)
	}
	if err := builder.LoadModuleStr(source); err != nil {
		t.Fatalf("LoadModuleStr: %v", err)
	}
	ctx, err := builder.Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	t.Cleanup(func() { ctx.Close() })
	mod, err := ctx.Schema("cambium-resolved-copy")
	if err != nil {
		t.Fatalf("Schema: %v", err)
	}

	intRange := resolvedTypeFor[cambium.ResolvedInt](t, mod, "/crc:top/crc:ranged-int")
	intRange.Range[0] = cambium.RangeBound{}
	intRange = resolvedTypeFor[cambium.ResolvedInt](t, mod, "/crc:top/crc:ranged-int")
	if got, want := intRange.Range[0].Min(), "1"; got != want {
		t.Fatalf("int range lower bound after caller mutation = %q, want %q", got, want)
	}

	decRange := resolvedTypeFor[cambium.ResolvedDecimal64](t, mod, "/crc:top/crc:ranged-dec64")
	decRange.Range[0] = cambium.RangeBound{}
	decRange = resolvedTypeFor[cambium.ResolvedDecimal64](t, mod, "/crc:top/crc:ranged-dec64")
	if got, want := decRange.Range[0].Max(), "100.00"; got != want {
		t.Fatalf("decimal64 range upper bound after caller mutation = %q, want %q", got, want)
	}

	binary := resolvedTypeFor[cambium.ResolvedBinary](t, mod, "/crc:top/crc:bounded-binary")
	binary.Length[0] = cambium.RangeBound{}
	binary = resolvedTypeFor[cambium.ResolvedBinary](t, mod, "/crc:top/crc:bounded-binary")
	if got, want := binary.Length[0].Max(), "16"; got != want {
		t.Fatalf("binary length upper bound after caller mutation = %q, want %q", got, want)
	}

	str := resolvedTypeFor[cambium.ResolvedString](t, mod, "/crc:top/crc:constrained-string")
	str.Length[0] = cambium.RangeBound{}
	str.Patterns[0] = cambium.Pattern{}
	str = resolvedTypeFor[cambium.ResolvedString](t, mod, "/crc:top/crc:constrained-string")
	if got, want := str.Length[0].Min(), "2"; got != want {
		t.Fatalf("string length lower bound after caller mutation = %q, want %q", got, want)
	}
	if got, want := str.Patterns[0].Regex(), "[a-z]+"; got != want {
		t.Fatalf("string pattern after caller mutation = %q, want %q", got, want)
	}

	enum := resolvedTypeFor[cambium.ResolvedEnumeration](t, mod, "/crc:top/crc:enum-leaf")
	enumValues := enum.Values()
	enumValues[0] = cambium.EnumValue{}
	enum = resolvedTypeFor[cambium.ResolvedEnumeration](t, mod, "/crc:top/crc:enum-leaf")
	if got, want := enum.Values()[0].Name(), "first"; got != want {
		t.Fatalf("enum value after caller mutation = %q, want %q", got, want)
	}

	bits := resolvedTypeFor[cambium.ResolvedBits](t, mod, "/crc:top/crc:bits-leaf")
	bitValues := bits.Values()
	bitValues[0] = cambium.EnumValue{}
	bits = resolvedTypeFor[cambium.ResolvedBits](t, mod, "/crc:top/crc:bits-leaf")
	if got, want := bits.Values()[0].Name(), "read"; got != want {
		t.Fatalf("bit value after caller mutation = %q, want %q", got, want)
	}
}

func resolvedTypeFor[T cambium.ResolvedType](t *testing.T, mod cambium.Module, path string) T {
	t.Helper()
	node := schemaNodeAt(t, mod, path)
	info, ok := node.LeafType()
	if !ok {
		t.Fatalf("%s has no leaf type", path)
	}
	resolved, ok := info.Resolved().(T)
	if !ok {
		t.Fatalf("%s resolved type = %T", path, info.Resolved())
	}
	return resolved
}

func TestEnumBitValueMetadataAccessors(t *testing.T) {
	source := `module cambium-enum-bit-metadata {
    namespace "urn:cambium:enum-bit-metadata";
    prefix cebm;

    leaf state {
        type enumeration {
            enum up {
                value 1;
                description "Up state.";
                reference "Up reference.";
                status deprecated;
            }
            enum down {
                value 2;
            }
        }
    }

    leaf flags {
        type bits {
            bit read {
                position 0;
                description "Read permission.";
                reference "Read reference.";
                status obsolete;
            }
            bit write {
                position 1;
            }
        }
    }
}`

	builder, err := cambium.NewContextBuilder(cambium.ContextFlags{})
	if err != nil {
		t.Fatalf("NewContextBuilder: %v", err)
	}
	if err := builder.LoadModuleStr(source); err != nil {
		t.Fatalf("LoadModuleStr: %v", err)
	}
	ctx, err := builder.Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	defer ctx.Close()

	mod, err := ctx.Schema("cambium-enum-bit-metadata")
	if err != nil {
		t.Fatalf("Schema: %v", err)
	}
	state, err := mod.FindPath("/cebm:state")
	if err != nil {
		t.Fatalf("Find state: %v", err)
	}
	stateInfo, ok := state.LeafType()
	if !ok {
		t.Fatal("state leaf type missing")
	}
	enumType, ok := stateInfo.Resolved().(cambium.ResolvedEnumeration)
	if !ok {
		t.Fatalf("expected enumeration, got %T", stateInfo.Resolved())
	}
	enumValues := enumType.Values()
	if got, ok := enumValues[0].Description(); !ok || got != "Up state." {
		t.Fatalf("enum description = (%q,%v), want Up state.,true", got, ok)
	}
	if got, ok := enumValues[0].Reference(); !ok || got != "Up reference." {
		t.Fatalf("enum reference = (%q,%v), want Up reference.,true", got, ok)
	}
	if got := enumValues[0].Status(); got != cambium.StatusDeprecated {
		t.Fatalf("enum status = %v, want StatusDeprecated", got)
	}
	if got, ok := enumValues[1].Description(); ok || got != "" {
		t.Fatalf("plain enum description = (%q,%v), want empty,false", got, ok)
	}
	if got, ok := enumValues[1].Reference(); ok || got != "" {
		t.Fatalf("plain enum reference = (%q,%v), want empty,false", got, ok)
	}
	if got := enumValues[1].Status(); got != cambium.StatusCurrent {
		t.Fatalf("plain enum status = %v, want StatusCurrent", got)
	}

	flags, err := mod.FindPath("/cebm:flags")
	if err != nil {
		t.Fatalf("Find flags: %v", err)
	}
	flagsInfo, ok := flags.LeafType()
	if !ok {
		t.Fatal("flags leaf type missing")
	}
	bitsType, ok := flagsInfo.Resolved().(cambium.ResolvedBits)
	if !ok {
		t.Fatalf("expected bits, got %T", flagsInfo.Resolved())
	}
	bitValues := bitsType.Values()
	if got, ok := bitValues[0].Description(); !ok || got != "Read permission." {
		t.Fatalf("bit description = (%q,%v), want Read permission.,true", got, ok)
	}
	if got, ok := bitValues[0].Reference(); !ok || got != "Read reference." {
		t.Fatalf("bit reference = (%q,%v), want Read reference.,true", got, ok)
	}
	if got := bitValues[0].Status(); got != cambium.StatusObsolete {
		t.Fatalf("bit status = %v, want StatusObsolete", got)
	}
	if got, ok := bitValues[1].Description(); ok || got != "" {
		t.Fatalf("plain bit description = (%q,%v), want empty,false", got, ok)
	}
	if got, ok := bitValues[1].Reference(); ok || got != "" {
		t.Fatalf("plain bit reference = (%q,%v), want empty,false", got, ok)
	}
	if got := bitValues[1].Status(); got != cambium.StatusCurrent {
		t.Fatalf("plain bit status = %v, want StatusCurrent", got)
	}
}

func TestDerivedEnumBitsRestrictionsNarrowValues(t *testing.T) {
	source := `module cambium-derived-enum-bits-restrict {
    namespace "urn:cambium:derived-enum-bits-restrict";
    prefix cdebr;

    typedef base-state {
        type enumeration {
            enum up { value 1; }
            enum down { value 2; }
            enum unknown { value 0; }
        }
    }

    typedef base-flags {
        type bits {
            bit read { position 0; }
            bit write { position 2; }
            bit execute { position 7; }
        }
    }

    container top {
        leaf state {
            type base-state {
                enum down;
            }
        }
        leaf flags {
            type base-flags {
                bit execute;
            }
        }
    }
}`
	builder, err := cambium.NewContextBuilder(cambium.ContextFlags{})
	if err != nil {
		t.Fatal(err)
	}
	if err := builder.LoadModuleStr(source); err != nil {
		t.Fatalf("LoadModuleStr: %v", err)
	}
	ctx, err := builder.Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	defer ctx.Close()
	mod, err := ctx.Schema("cambium-derived-enum-bits-restrict")
	if err != nil {
		t.Fatalf("Schema: %v", err)
	}
	state, err := mod.FindPath("/cdebr:top/cdebr:state")
	if err != nil {
		t.Fatalf("Find state: %v", err)
	}
	stateInfo, ok := state.LeafType()
	if !ok {
		t.Fatal("expected state leaf type")
	}
	stateEnum, ok := stateInfo.Resolved().(cambium.ResolvedEnumeration)
	if !ok {
		t.Fatalf("expected state enumeration, got %T", stateInfo.Resolved())
	}
	stateVals := stateEnum.Values()
	if len(stateVals) != 1 || stateVals[0].Name() != "down" || stateVals[0].Value() != 2 {
		t.Fatalf("state enum values = %v, want [down=2]", enumValuesForLog(stateVals))
	}

	flags, err := mod.FindPath("/cdebr:top/cdebr:flags")
	if err != nil {
		t.Fatalf("Find flags: %v", err)
	}
	flagsInfo, ok := flags.LeafType()
	if !ok {
		t.Fatal("expected flags leaf type")
	}
	flagsBits, ok := flagsInfo.Resolved().(cambium.ResolvedBits)
	if !ok {
		t.Fatalf("expected flags bits, got %T", flagsInfo.Resolved())
	}
	bitVals := flagsBits.Values()
	if len(bitVals) != 1 || bitVals[0].Name() != "execute" || bitVals[0].Value() != 7 {
		t.Fatalf("bit values = %v, want [execute=7]", enumValuesForLog(bitVals))
	}
}

func enumValuesForLog(values []cambium.EnumValue) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		out = append(out, fmt.Sprintf("%s=%d", value.Name(), value.Value()))
	}
	return out
}

func TestDerivedIdentityRefRestrictionNarrowsBases(t *testing.T) {
	source := `module cambium-derived-identityref-restrict {
    namespace "urn:cambium:derived-identityref-restrict";
    prefix cdir;

    identity root;
    identity child {
        base root;
    }

    typedef base-ref {
        type identityref {
            base root;
        }
    }

    leaf value {
        type base-ref {
            base child;
        }
    }
}`
	builder, err := cambium.NewContextBuilder(cambium.ContextFlags{})
	if err != nil {
		t.Fatal(err)
	}
	if err := builder.LoadModuleStr(source); err != nil {
		t.Fatalf("LoadModuleStr: %v", err)
	}
	ctx, err := builder.Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	defer ctx.Close()
	mod, err := ctx.Schema("cambium-derived-identityref-restrict")
	if err != nil {
		t.Fatalf("Schema: %v", err)
	}
	leaf, err := mod.FindPath("/cdir:value")
	if err != nil {
		t.Fatalf("Find value: %v", err)
	}
	info, ok := leaf.LeafType()
	if !ok {
		t.Fatal("expected leaf type")
	}
	idref, ok := info.Resolved().(cambium.ResolvedIdentityRef)
	if !ok {
		t.Fatalf("expected identityref, got %T", info.Resolved())
	}
	bases := idref.Bases()
	if len(bases) != 1 || bases[0].Name() != "child" {
		t.Fatalf("identityref bases = %v, want [child]", namesOf(bases))
	}
}

func TestDerivedRequireInstanceRestrictionOverridesTypedef(t *testing.T) {
	source := `module cambium-derived-require-instance {
    namespace "urn:cambium:derived-require-instance";
    prefix cdri;
    yang-version 1.1;

    typedef base-iid {
        type instance-identifier;
    }

    typedef base-leafref {
        type leafref {
            path "../target";
        }
    }

    leaf target {
        type string;
    }

    leaf iid {
        type base-iid {
            require-instance false;
        }
    }

    leaf ref {
        type base-leafref {
            require-instance false;
        }
    }
}`
	builder, err := cambium.NewContextBuilder(cambium.ContextFlags{})
	if err != nil {
		t.Fatal(err)
	}
	if err := builder.LoadModuleStr(source); err != nil {
		t.Fatalf("LoadModuleStr: %v", err)
	}
	ctx, err := builder.Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	defer ctx.Close()
	mod, err := ctx.Schema("cambium-derived-require-instance")
	if err != nil {
		t.Fatalf("Schema: %v", err)
	}
	iid, err := mod.FindPath("/cdri:iid")
	if err != nil {
		t.Fatalf("Find iid: %v", err)
	}
	iidInfo, ok := iid.LeafType()
	if !ok {
		t.Fatal("expected iid leaf type")
	}
	inst, ok := iidInfo.Resolved().(cambium.ResolvedInstanceIdentifier)
	if !ok {
		t.Fatalf("expected instance-identifier, got %T", iidInfo.Resolved())
	}
	if inst.RequireInstance {
		t.Fatal("instance-identifier require-instance = true, want false")
	}

	ref, err := mod.FindPath("/cdri:ref")
	if err != nil {
		t.Fatalf("Find ref: %v", err)
	}
	refInfo, ok := ref.LeafType()
	if !ok {
		t.Fatal("expected ref leaf type")
	}
	leafref, ok := refInfo.Resolved().(cambium.ResolvedLeafRef)
	if !ok {
		t.Fatalf("expected leafref, got %T", refInfo.Resolved())
	}
	if leafref.RequireInstance() {
		t.Fatal("leafref require-instance = true, want false")
	}
}

func TestLeafTypeUnionMembersRecursive(t *testing.T) {
	ctx, cleanup := introspectionContext(t)
	defer cleanup()

	top := topNode(t, ctx)
	uni := childByName(t, top.Children(), "uni")
	info, ok := uni.LeafType()
	if !ok {
		t.Fatal("expected leaf type")
	}
	r, ok := info.Resolved().(cambium.ResolvedUnion)
	if !ok {
		t.Fatalf("expected union, got %T", info.Resolved())
	}
	members := r.Members()
	if len(members) != 2 {
		t.Fatalf("union members = %d, want 2", len(members))
	}
	if members[0].Base() != cambium.BaseTypeString {
		t.Fatalf("union[0] base = %v, want String", members[0].Base())
	}
	if members[1].Base() != cambium.BaseTypeInt32 {
		t.Fatalf("union[1] base = %v, want Int32", members[1].Base())
	}
}

func TestImportedTypedefUnionMembersResolveInDefiningModule(t *testing.T) {
	ctx, err := cambium.NewContext()
	if err != nil {
		t.Fatal(err)
	}
	defer ctx.Close()

	moduleDir := filepath.Join("..", "..", "conformance", "fixtures", "rfc6991-inet-yang-types-roundtrip", "module")
	if err := ctx.SetSearchPath(moduleDir); err != nil {
		t.Fatal(err)
	}
	if err := ctx.LoadModule("rfc6991-inet-yang-types-roundtrip"); err != nil {
		t.Fatal(err)
	}
	mod, err := ctx.Schema("rfc6991-inet-yang-types-roundtrip")
	if err != nil {
		t.Fatal(err)
	}
	leaf, err := mod.FindPath("/riyt:top/riyt:addr")
	if err != nil {
		t.Fatal(err)
	}
	info, ok := leaf.LeafType()
	if !ok {
		t.Fatal("expected leaf type")
	}
	union, ok := info.Resolved().(cambium.ResolvedUnion)
	if !ok {
		t.Fatalf("expected union, got %T", info.Resolved())
	}
	members := union.Members()
	if len(members) != 2 {
		t.Fatalf("union members = %d, want 2", len(members))
	}
	wantTypedefs := []string{"ipv4-address", "ipv6-address"}
	for i, member := range members {
		if member.Base() != cambium.BaseTypeString {
			t.Fatalf("union[%d] base = %v, want String", i, member.Base())
		}
		name, ok := member.TypedefName()
		if !ok || name != wantTypedefs[i] {
			t.Fatalf("union[%d] typedef = %q,%v, want %q,true", i, name, ok, wantTypedefs[i])
		}
	}
}

func TestLeafTypeIntRange(t *testing.T) {
	ctx, cleanup := introspectionContext(t)
	defer cleanup()

	top := topNode(t, ctx)
	leaf := childByName(t, top.Children(), "ranged-int")
	info, ok := leaf.LeafType()
	if !ok {
		t.Fatal("expected leaf type")
	}
	r, ok := info.Resolved().(cambium.ResolvedInt)
	if !ok {
		t.Fatalf("expected int, got %T", info.Resolved())
	}
	if r.Kind != cambium.IntKindI32 {
		t.Fatalf("kind = %v, want I32", r.Kind)
	}
	parts := r.Range
	if len(parts) != 2 {
		t.Fatalf("range parts = %d, want 2", len(parts))
	}
	if parts[0].Min() != "1" || parts[0].Max() != "10" {
		t.Fatalf("range[0] = %s..%s, want 1..10", parts[0].Min(), parts[0].Max())
	}
	if parts[1].Min() != "20" || parts[1].Max() != "2147483647" {
		t.Fatalf("range[1] = %s..%s, want 20..2147483647", parts[1].Min(), parts[1].Max())
	}
	if tag, ok := parts[0].ErrorAppTag(); !ok || tag != "range-tag" {
		t.Fatalf("range[0].error-app-tag = (%q,%v), want range-tag,true", tag, ok)
	}
	if ref, ok := parts[1].Reference(); !ok || ref != "range-ref" {
		t.Fatalf("range[1].reference = (%q,%v), want range-ref,true", ref, ok)
	}
}

func TestLeafTypeDecimal64Range(t *testing.T) {
	ctx, cleanup := introspectionContext(t)
	defer cleanup()

	top := topNode(t, ctx)
	leaf := childByName(t, top.Children(), "ranged-dec64")
	info, ok := leaf.LeafType()
	if !ok {
		t.Fatal("expected leaf type")
	}
	r, ok := info.Resolved().(cambium.ResolvedDecimal64)
	if !ok {
		t.Fatalf("expected decimal64, got %T", info.Resolved())
	}
	if r.FractionDigits().Value() != 2 {
		t.Fatalf("fraction-digits = %d, want 2", r.FractionDigits().Value())
	}
	parts := r.Range
	if len(parts) != 1 {
		t.Fatalf("range parts = %d, want 1", len(parts))
	}
	if parts[0].Min() != "0.00" || parts[0].Max() != "100.00" {
		t.Fatalf("range[0] = %s..%s, want 0.00..100.00", parts[0].Min(), parts[0].Max())
	}
}

func TestLeafTypeStringLength(t *testing.T) {
	ctx, cleanup := introspectionContext(t)
	defer cleanup()

	top := topNode(t, ctx)
	leaf := childByName(t, top.Children(), "constrained-string")
	info, ok := leaf.LeafType()
	if !ok {
		t.Fatal("expected leaf type")
	}
	r, ok := info.Resolved().(cambium.ResolvedString)
	if !ok {
		t.Fatalf("expected string, got %T", info.Resolved())
	}
	parts := r.Length
	if len(parts) != 1 {
		t.Fatalf("length parts = %d, want 1", len(parts))
	}
	if parts[0].Min() != "1" || parts[0].Max() != "255" {
		t.Fatalf("length[0] = %s..%s, want 1..255", parts[0].Min(), parts[0].Max())
	}
	if msg, ok := parts[0].ErrorMessage(); !ok || msg != "length failed" {
		t.Fatalf("length[0].error-message = (%q,%v), want length failed,true", msg, ok)
	}
	if desc, ok := parts[0].Description(); !ok || desc != "String length bounds." {
		t.Fatalf("length[0].description = (%q,%v), want String length bounds.,true", desc, ok)
	}
}

func TestRangeAndLengthBoundsAllowLeadingPlus(t *testing.T) {
	source := `module cambium-range-length-plus {
    namespace "urn:cambium:range-length-plus";
    prefix crlp;

    leaf ranged-uint {
        type uint8 {
            range "+1..2";
        }
    }
    leaf bounded-string {
        type string {
            length "+1..2";
        }
    }
}`
	builder, err := cambium.NewContextBuilder(cambium.ContextFlags{})
	if err != nil {
		t.Fatal(err)
	}
	if err := builder.LoadModuleStr(source); err != nil {
		t.Fatalf("LoadModuleStr: %v", err)
	}
	ctx, err := builder.Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	t.Cleanup(func() { ctx.Close() })
	mod, err := ctx.Schema("cambium-range-length-plus")
	if err != nil {
		t.Fatalf("Schema: %v", err)
	}

	ranged := resolvedTypeFor[cambium.ResolvedInt](t, mod, "/crlp:ranged-uint")
	if got, want := len(ranged.Range), 1; got != want {
		t.Fatalf("uint range parts = %d, want %d", got, want)
	}
	if gotMin, gotMax := ranged.Range[0].Min(), ranged.Range[0].Max(); gotMin != "1" || gotMax != "2" {
		t.Fatalf("uint range = %s..%s, want 1..2", gotMin, gotMax)
	}

	bounded := resolvedTypeFor[cambium.ResolvedString](t, mod, "/crlp:bounded-string")
	if got, want := len(bounded.Length), 1; got != want {
		t.Fatalf("string length parts = %d, want %d", got, want)
	}
	if gotMin, gotMax := bounded.Length[0].Min(), bounded.Length[0].Max(); gotMin != "1" || gotMax != "2" {
		t.Fatalf("string length = %s..%s, want 1..2", gotMin, gotMax)
	}
}

func TestLeafTypeStringPatterns(t *testing.T) {
	ctx, cleanup := introspectionContext(t)
	defer cleanup()

	top := topNode(t, ctx)
	leaf := childByName(t, top.Children(), "constrained-string")
	info, ok := leaf.LeafType()
	if !ok {
		t.Fatal("expected leaf type")
	}
	r, ok := info.Resolved().(cambium.ResolvedString)
	if !ok {
		t.Fatalf("expected string, got %T", info.Resolved())
	}
	patterns := r.Patterns
	if len(patterns) != 2 {
		t.Fatalf("patterns = %d, want 2", len(patterns))
	}
	if patterns[0].Regex() != "[a-zA-Z0-9_-]+" {
		t.Fatalf("pattern[0].regex = %q, want [a-zA-Z0-9_-]+", patterns[0].Regex())
	}
	if tag, ok := patterns[0].ErrorAppTag(); !ok || tag != "my-tag" {
		t.Fatalf("pattern[0].app-tag = %q, ok=%v, want my-tag", tag, ok)
	}
	if msg, ok := patterns[0].ErrorMessage(); !ok || msg != "pattern failed" {
		t.Fatalf("pattern[0].error-message = %q, ok=%v, want pattern failed", msg, ok)
	}
	if desc, ok := patterns[0].Description(); !ok || desc != "Allowed identifier characters." {
		t.Fatalf("pattern[0].description = %q, ok=%v, want Allowed identifier characters.", desc, ok)
	}
	if ref, ok := patterns[0].Reference(); !ok || ref != "pattern-ref" {
		t.Fatalf("pattern[0].reference = %q, ok=%v, want pattern-ref", ref, ok)
	}
	if patterns[0].IsInverted() {
		t.Fatal("pattern[0] should not be inverted")
	}
	if patterns[1].Regex() != "^foo.*" {
		t.Fatalf("pattern[1].regex = %q, want ^foo.*", patterns[1].Regex())
	}
	if !patterns[1].IsInverted() {
		t.Fatal("pattern[1] should be inverted")
	}
}

func TestLeafTypeStringPatternXSDUnicodeBlock(t *testing.T) {
	source := `module cambium-pattern-xsd-unicode-block-valid {
    namespace "urn:cambium:pattern-xsd-unicode-block-valid";
    prefix cpxubv;
    leaf value {
        type string {
            pattern "\\p{IsBasicLatin}+";
        }
        default "abc";
    }
}`
	builder, err := cambium.NewContextBuilder(cambium.ContextFlags{})
	if err != nil {
		t.Fatal(err)
	}
	if err := builder.LoadModuleStr(source); err != nil {
		t.Fatalf("LoadModuleStr: %v", err)
	}
	ctx, err := builder.Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	defer ctx.Close()
	mod, err := ctx.Schema("cambium-pattern-xsd-unicode-block-valid")
	if err != nil {
		t.Fatalf("Schema: %v", err)
	}
	leaf := schemaNodeAt(t, mod, "/cpxubv:value")
	info, ok := leaf.LeafType()
	if !ok {
		t.Fatal("expected leaf type")
	}
	resolved, ok := info.Resolved().(cambium.ResolvedString)
	if !ok {
		t.Fatalf("expected string, got %T", info.Resolved())
	}
	if len(resolved.Patterns) != 1 {
		t.Fatalf("patterns = %d, want 1", len(resolved.Patterns))
	}
	if got, want := resolved.Patterns[0].Regex(), `\p{IsBasicLatin}+`; got != want {
		t.Fatalf("pattern regex = %q, want %q", got, want)
	}
}

func TestLeafTypeStringPatternXSDNonASCIIUnicodeBlock(t *testing.T) {
	source := `module cambium-pattern-xsd-nonascii-unicode-block-valid {
    namespace "urn:cambium:pattern-xsd-nonascii-unicode-block-valid";
    prefix cpxnubv;
    leaf value {
        type string {
            pattern "\\p{IsGreek}+";
        }
        default "Ω";
    }
    leaf non_greek {
        type string {
            pattern "\\P{IsGreek}+";
        }
        default "abc";
    }
}`
	builder, err := cambium.NewContextBuilder(cambium.ContextFlags{})
	if err != nil {
		t.Fatal(err)
	}
	if err := builder.LoadModuleStr(source); err != nil {
		t.Fatalf("LoadModuleStr: %v", err)
	}
	ctx, err := builder.Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	defer ctx.Close()
	mod, err := ctx.Schema("cambium-pattern-xsd-nonascii-unicode-block-valid")
	if err != nil {
		t.Fatalf("Schema: %v", err)
	}
	for _, tt := range []struct {
		path string
		want string
	}{
		{path: "/cpxnubv:value", want: `\p{IsGreek}+`},
		{path: "/cpxnubv:non_greek", want: `\P{IsGreek}+`},
	} {
		leaf := schemaNodeAt(t, mod, tt.path)
		info, ok := leaf.LeafType()
		if !ok {
			t.Fatalf("%s expected leaf type", tt.path)
		}
		resolved, ok := info.Resolved().(cambium.ResolvedString)
		if !ok {
			t.Fatalf("%s expected string, got %T", tt.path, info.Resolved())
		}
		if len(resolved.Patterns) != 1 {
			t.Fatalf("%s patterns = %d, want 1", tt.path, len(resolved.Patterns))
		}
		if got := resolved.Patterns[0].Regex(); got != tt.want {
			t.Fatalf("%s pattern regex = %q, want %q", tt.path, got, tt.want)
		}
	}
}

func TestLeafTypeStringPatternXSDMultiCharacterEscapes(t *testing.T) {
	source := `module cambium-pattern-xsd-multichar-escapes-valid {
    namespace "urn:cambium:pattern-xsd-multichar-escapes-valid";
    prefix cpxmev;
    leaf unicode_digit {
        type string {
            pattern "\\d+";
        }
        default "١";
    }
    leaf word_symbol {
        type string {
            pattern "\\w+";
        }
        default "$";
    }
    leaf identifier {
        type string {
            pattern "\\i\\c*";
        }
        default "name-1";
    }
}`
	builder, err := cambium.NewContextBuilder(cambium.ContextFlags{})
	if err != nil {
		t.Fatal(err)
	}
	if err := builder.LoadModuleStr(source); err != nil {
		t.Fatalf("LoadModuleStr: %v", err)
	}
	ctx, err := builder.Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	defer ctx.Close()
	mod, err := ctx.Schema("cambium-pattern-xsd-multichar-escapes-valid")
	if err != nil {
		t.Fatalf("Schema: %v", err)
	}
	for _, tt := range []struct {
		path string
		want string
	}{
		{path: "/cpxmev:unicode_digit", want: `\d+`},
		{path: "/cpxmev:word_symbol", want: `\w+`},
		{path: "/cpxmev:identifier", want: `\i\c*`},
	} {
		leaf := schemaNodeAt(t, mod, tt.path)
		info, ok := leaf.LeafType()
		if !ok {
			t.Fatalf("%s expected leaf type", tt.path)
		}
		resolved, ok := info.Resolved().(cambium.ResolvedString)
		if !ok {
			t.Fatalf("%s expected string, got %T", tt.path, info.Resolved())
		}
		if len(resolved.Patterns) != 1 {
			t.Fatalf("%s patterns = %d, want 1", tt.path, len(resolved.Patterns))
		}
		if got := resolved.Patterns[0].Regex(); got != tt.want {
			t.Fatalf("%s pattern regex = %q, want %q", tt.path, got, tt.want)
		}
	}
}

func TestLeafTypeStringPatternXSDLiteralAnchors(t *testing.T) {
	source := `module cambium-pattern-xsd-literal-anchors-valid {
    namespace "urn:cambium:pattern-xsd-literal-anchors-valid";
    prefix cpxlav;
    leaf value {
        type string {
            pattern "^foo$";
        }
        default "^foo$";
    }
}`
	builder, err := cambium.NewContextBuilder(cambium.ContextFlags{})
	if err != nil {
		t.Fatal(err)
	}
	if err := builder.LoadModuleStr(source); err != nil {
		t.Fatalf("LoadModuleStr: %v", err)
	}
	ctx, err := builder.Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	defer ctx.Close()
	mod, err := ctx.Schema("cambium-pattern-xsd-literal-anchors-valid")
	if err != nil {
		t.Fatalf("Schema: %v", err)
	}
	leaf := schemaNodeAt(t, mod, "/cpxlav:value")
	info, ok := leaf.LeafType()
	if !ok {
		t.Fatal("expected leaf type")
	}
	resolved, ok := info.Resolved().(cambium.ResolvedString)
	if !ok {
		t.Fatalf("expected string, got %T", info.Resolved())
	}
	if len(resolved.Patterns) != 1 {
		t.Fatalf("patterns = %d, want 1", len(resolved.Patterns))
	}
	if got, want := resolved.Patterns[0].Regex(), "^foo$"; got != want {
		t.Fatalf("pattern regex = %q, want %q", got, want)
	}
}

func TestLeafTypeStringPatternXSDClassSubtraction(t *testing.T) {
	source := `module cambium-pattern-xsd-class-subtraction-valid {
    namespace "urn:cambium:pattern-xsd-class-subtraction-valid";
    prefix cpxcsv;
    leaf value {
        type string {
            pattern "[a-z-[aeiou]]+";
        }
        default "bcdf";
    }
}`
	builder, err := cambium.NewContextBuilder(cambium.ContextFlags{})
	if err != nil {
		t.Fatal(err)
	}
	if err := builder.LoadModuleStr(source); err != nil {
		t.Fatalf("LoadModuleStr: %v", err)
	}
	ctx, err := builder.Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	defer ctx.Close()
	mod, err := ctx.Schema("cambium-pattern-xsd-class-subtraction-valid")
	if err != nil {
		t.Fatalf("Schema: %v", err)
	}
	leaf := schemaNodeAt(t, mod, "/cpxcsv:value")
	info, ok := leaf.LeafType()
	if !ok {
		t.Fatal("expected leaf type")
	}
	resolved, ok := info.Resolved().(cambium.ResolvedString)
	if !ok {
		t.Fatalf("expected string, got %T", info.Resolved())
	}
	if len(resolved.Patterns) != 1 {
		t.Fatalf("patterns = %d, want 1", len(resolved.Patterns))
	}
	if got, want := resolved.Patterns[0].Regex(), "[a-z-[aeiou]]+"; got != want {
		t.Fatalf("pattern regex = %q, want %q", got, want)
	}
}

func TestLeafTypeStringPatternXSDCategoryClassSubtraction(t *testing.T) {
	source := `module cambium-pattern-xsd-category-class-subtraction-valid {
    namespace "urn:cambium:pattern-xsd-category-class-subtraction-valid";
    prefix cpxccsv;
    leaf value {
        type string {
            pattern "[\\p{IsGreek}-[\\p{Lu}]]+";
        }
        default "ω";
    }
}`
	builder, err := cambium.NewContextBuilder(cambium.ContextFlags{})
	if err != nil {
		t.Fatal(err)
	}
	if err := builder.LoadModuleStr(source); err != nil {
		t.Fatalf("LoadModuleStr: %v", err)
	}
	ctx, err := builder.Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	defer ctx.Close()
	mod, err := ctx.Schema("cambium-pattern-xsd-category-class-subtraction-valid")
	if err != nil {
		t.Fatalf("Schema: %v", err)
	}
	leaf := schemaNodeAt(t, mod, "/cpxccsv:value")
	info, ok := leaf.LeafType()
	if !ok {
		t.Fatal("expected leaf type")
	}
	resolved, ok := info.Resolved().(cambium.ResolvedString)
	if !ok {
		t.Fatalf("expected string, got %T", info.Resolved())
	}
	if len(resolved.Patterns) != 1 {
		t.Fatalf("patterns = %d, want 1", len(resolved.Patterns))
	}
	if got, want := resolved.Patterns[0].Regex(), `[\p{IsGreek}-[\p{Lu}]]+`; got != want {
		t.Fatalf("pattern regex = %q, want %q", got, want)
	}
}

func TestLeafTypeStringPatternXSDNestedClassSubtraction(t *testing.T) {
	source := `module cambium-pattern-xsd-nested-class-subtraction-valid {
    namespace "urn:cambium:pattern-xsd-nested-class-subtraction-valid";
    prefix cpxncsv;

    leaf value {
        type string {
            pattern "[a-z-[a-m-[aeiou]]]+";
        }
        default "aeiounz";
    }
    leaf vowels {
        type string {
            pattern "[a-z-[^aeiou]]+";
        }
        default "aeiou";
    }
}`
	builder, err := cambium.NewContextBuilder(cambium.ContextFlags{})
	if err != nil {
		t.Fatal(err)
	}
	if err := builder.LoadModuleStr(source); err != nil {
		t.Fatalf("LoadModuleStr: %v", err)
	}
	ctx, err := builder.Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	defer ctx.Close()
	mod, err := ctx.Schema("cambium-pattern-xsd-nested-class-subtraction-valid")
	if err != nil {
		t.Fatalf("Schema: %v", err)
	}
	for _, tt := range []struct {
		path string
		want string
	}{
		{path: "/cpxncsv:value", want: "[a-z-[a-m-[aeiou]]]+"},
		{path: "/cpxncsv:vowels", want: "[a-z-[^aeiou]]+"},
	} {
		leaf := schemaNodeAt(t, mod, tt.path)
		info, ok := leaf.LeafType()
		if !ok {
			t.Fatalf("%s should be a leaf", tt.path)
		}
		resolved, ok := info.Resolved().(cambium.ResolvedString)
		if !ok {
			t.Fatalf("%s resolved type = %T, want string", tt.path, info.Resolved())
		}
		if len(resolved.Patterns) != 1 {
			t.Fatalf("%s patterns = %d, want 1", tt.path, len(resolved.Patterns))
		}
		if got := resolved.Patterns[0].Regex(); got != tt.want {
			t.Fatalf("%s pattern regex = %q, want %q", tt.path, got, tt.want)
		}
	}
}

func TestLeafrefRealtypeResolves(t *testing.T) {
	ctx, cleanup := introspectionContext(t)
	defer cleanup()

	top := topNode(t, ctx)
	leaf := childByName(t, top.Children(), "ref-to-name")
	info, ok := leaf.LeafType()
	if !ok {
		t.Fatal("expected leaf type")
	}
	r, ok := info.Resolved().(cambium.ResolvedLeafRef)
	if !ok {
		t.Fatalf("expected leafref, got %T", info.Resolved())
	}
	if !r.RequireInstance() {
		t.Fatal("leafref should require instance")
	}
	realType, ok := r.Realtype()
	if !ok {
		t.Fatal("expected leafref realtype")
	}
	if realType.Base() != cambium.BaseTypeString {
		t.Fatalf("realtype base = %v, want String", realType.Base())
	}
}

func TestLeafrefRequireInstanceFalseMayTargetState(t *testing.T) {
	const source = `module cambium-leafref-config-target-state-valid {
    namespace "urn:cambium:leafref-config-target-state-valid";
    prefix clctsv;
    yang-version 1.1;
    container top {
        leaf state-id {
            config false;
            type string;
        }
        leaf ref {
            type leafref {
                path "../state-id";
                require-instance false;
            }
        }
    }
}`
	builder, err := cambium.NewContextBuilder(cambium.ContextFlags{})
	if err != nil {
		t.Fatal(err)
	}
	if err := builder.LoadModuleStr(source); err != nil {
		t.Fatalf("LoadModuleStr: %v", err)
	}
	ctx, err := builder.Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	defer ctx.Close()
	mod, err := ctx.Schema("cambium-leafref-config-target-state-valid")
	if err != nil {
		t.Fatalf("Schema: %v", err)
	}
	ref, err := mod.FindPath("/clctsv:top/clctsv:ref")
	if err != nil {
		t.Fatalf("FindPath ref: %v", err)
	}
	info, ok := ref.LeafType()
	if !ok {
		t.Fatal("expected leaf type")
	}
	leafref, ok := info.Resolved().(cambium.ResolvedLeafRef)
	if !ok {
		t.Fatalf("expected leafref, got %T", info.Resolved())
	}
	if leafref.RequireInstance() {
		t.Fatal("leafref require-instance = true, want false")
	}
	target, ok := leafref.Target()
	if !ok {
		t.Fatal("expected leafref target")
	}
	if !target.ReadOnly() {
		t.Fatal("leafref target should be read-only state data")
	}
}

func TestOperationPayloadLeafrefMayTargetState(t *testing.T) {
	const source = `module cambium-operation-leafref-state {
    namespace "urn:cambium:operation-leafref-state";
    prefix cols;
    yang-version 1.1;

    container top {
        leaf state-id {
            config false;
            type string;
        }
    }

    rpc collect {
        input {
            leaf ref {
                type leafref {
                    path "/cols:top/cols:state-id";
                }
            }
        }
    }

    notification changed {
        leaf ref {
            type leafref {
                path "/cols:top/cols:state-id";
            }
        }
    }
}`
	builder, err := cambium.NewContextBuilder(cambium.ContextFlags{})
	if err != nil {
		t.Fatal(err)
	}
	if err := builder.LoadModuleStr(source); err != nil {
		t.Fatalf("LoadModuleStr: %v", err)
	}
	ctx, err := builder.Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	defer ctx.Close()
	mod, err := ctx.Schema("cambium-operation-leafref-state")
	if err != nil {
		t.Fatalf("Schema: %v", err)
	}

	for _, path := range []string{
		"/cols:collect/input/ref",
		"/cols:changed/ref",
	} {
		ref, err := mod.FindPath(path)
		if err != nil {
			t.Fatalf("FindPath %s: %v", path, err)
		}
		info, ok := ref.LeafType()
		if !ok {
			t.Fatalf("%s LeafType missing", path)
		}
		leafref, ok := info.Resolved().(cambium.ResolvedLeafRef)
		if !ok {
			t.Fatalf("%s resolved type = %T, want leafref", path, info.Resolved())
		}
		if !leafref.RequireInstance() {
			t.Fatalf("%s RequireInstance = false, want true", path)
		}
		target, ok := leafref.Target()
		if !ok {
			t.Fatalf("%s Target missing", path)
		}
		if target.Config() != cambium.ConfigRo {
			t.Fatalf("%s target Config = %v, want ConfigRo", path, target.Config())
		}
	}
}

func TestLeafrefTargetResolves(t *testing.T) {
	ctx, cleanup := introspectionContext(t)
	defer cleanup()

	top := topNode(t, ctx)
	leaf := childByName(t, top.Children(), "ref-to-name")
	info, ok := leaf.LeafType()
	if !ok {
		t.Fatal("expected leaf type")
	}
	r, ok := info.Resolved().(cambium.ResolvedLeafRef)
	if !ok {
		t.Fatalf("expected leafref, got %T", info.Resolved())
	}
	target, ok := r.Target()
	if !ok {
		t.Fatal("expected leafref target")
	}
	if got, want := target.Path(), "/cambium-introspection-demo/top/keyed-list/name"; got != want {
		t.Fatalf("target.Path() = %q, want %q", got, want)
	}
	if !target.IsListKey() {
		t.Fatal("target should be a list key")
	}
}

func TestRelativeLeafrefTargetResolves(t *testing.T) {
	t.Helper()
	dir := schemaIntrospectionModuleDir(t)
	module := `module cambium-relative-leafref {
    namespace "urn:cambium:relative-leafref";
    prefix crl;
    yang-version 1.1;

    container top {
        leaf target {
            type string;
        }
        container refs {
            leaf ref {
                type leafref {
                    path "../../target";
                }
            }
        }
    }
}
`
	writeModuleFile(t, filepath.Join(dir, "cambium-relative-leafref.yang"), []byte(module))

	ctx, err := cambium.NewContext()
	if err != nil {
		t.Fatal(err)
	}
	defer ctx.Close()
	if err := ctx.SetSearchPath(dir); err != nil {
		t.Fatal(err)
	}
	if err := ctx.LoadModule("cambium-relative-leafref"); err != nil {
		t.Fatalf("LoadModule: %v", err)
	}
	mod, err := ctx.Schema("cambium-relative-leafref")
	if err != nil {
		t.Fatalf("Schema: %v", err)
	}
	ref, err := mod.FindPath("/crl:top/crl:refs/crl:ref")
	if err != nil {
		t.Fatalf("FindPath ref: %v", err)
	}
	info, ok := ref.LeafType()
	if !ok {
		t.Fatal("expected leaf type")
	}
	leafref, ok := info.Resolved().(cambium.ResolvedLeafRef)
	if !ok {
		t.Fatalf("expected leafref, got %T", info.Resolved())
	}
	target, ok := leafref.Target()
	if !ok {
		t.Fatal("expected relative leafref target")
	}
	if got, want := target.Path(), "/cambium-relative-leafref/top/target"; got != want {
		t.Fatalf("target.Path() = %q, want %q", got, want)
	}
	realType, ok := leafref.Realtype()
	if !ok {
		t.Fatal("expected relative leafref realtype")
	}
	if realType.Base() != cambium.BaseTypeString {
		t.Fatalf("realtype base = %v, want String", realType.Base())
	}
}

func TestLeafrefCurrentPredicateTargetResolves(t *testing.T) {
	mod := loadConformanceSchema(t, "types-leafref-current-context")
	leaf := schemaNodeAt(t, mod, "/proto-name")
	info, ok := leaf.LeafType()
	if !ok {
		t.Fatal("expected leaf type")
	}
	leafref, ok := info.Resolved().(cambium.ResolvedLeafRef)
	if !ok {
		t.Fatalf("expected leafref, got %T", info.Resolved())
	}
	if got, want := mustLeafrefPath(t, leafref), "/proto[name = current()/../selected]/name"; got != want {
		t.Fatalf("leafref path = %q, want %q", got, want)
	}
	target, ok := leafref.Target()
	if !ok {
		t.Fatal("expected current() predicate leafref target")
	}
	if got, want := target.Path(), "/types-leafref-current-context/proto/name"; got != want {
		t.Fatalf("target.Path() = %q, want %q", got, want)
	}
	realType, ok := leafref.Realtype()
	if !ok {
		t.Fatal("expected current() predicate leafref realtype")
	}
	if realType.Base() != cambium.BaseTypeString {
		t.Fatalf("realtype base = %v, want String", realType.Base())
	}
}

func TestLeafrefDerefTargetResolves(t *testing.T) {
	mod := loadConformanceSchema(t, "types-leafref-deref-function")
	leaf := schemaNodeAt(t, mod, "/home-dir")
	info, ok := leaf.LeafType()
	if !ok {
		t.Fatal("expected leaf type")
	}
	leafref, ok := info.Resolved().(cambium.ResolvedLeafRef)
	if !ok {
		t.Fatalf("expected leafref, got %T", info.Resolved())
	}
	if got, want := mustLeafrefPath(t, leafref), "deref(../logged-in-user)/../home"; got != want {
		t.Fatalf("leafref path = %q, want %q", got, want)
	}
	target, ok := leafref.Target()
	if !ok {
		t.Fatal("expected deref() leafref target")
	}
	if got, want := target.Path(), "/types-leafref-deref-function/user/home"; got != want {
		t.Fatalf("target.Path() = %q, want %q", got, want)
	}
	realType, ok := leafref.Realtype()
	if !ok {
		t.Fatal("expected deref() leafref realtype")
	}
	if realType.Base() != cambium.BaseTypeString {
		t.Fatalf("realtype base = %v, want String", realType.Base())
	}
}

func TestLeafrefCrossModuleTargetResolves(t *testing.T) {
	mod := loadConformanceSchema(t, "types-leafref-cross-module")
	leaf := schemaNodeAt(t, mod, "/bound-if")
	info, ok := leaf.LeafType()
	if !ok {
		t.Fatal("expected leaf type")
	}
	leafref, ok := info.Resolved().(cambium.ResolvedLeafRef)
	if !ok {
		t.Fatalf("expected leafref, got %T", info.Resolved())
	}
	if got, want := mustLeafrefPath(t, leafref), "/base:interface/base:name"; got != want {
		t.Fatalf("leafref path = %q, want %q", got, want)
	}
	target, ok := leafref.Target()
	if !ok {
		t.Fatal("expected cross-module leafref target")
	}
	if got, want := target.Path(), "/types-leafref-cross-module-base/interface/name"; got != want {
		t.Fatalf("target.Path() = %q, want %q", got, want)
	}
	if got, want := target.Module().Name(), "types-leafref-cross-module-base"; got != want {
		t.Fatalf("target.Module().Name() = %q, want %q", got, want)
	}
	realType, ok := leafref.Realtype()
	if !ok {
		t.Fatal("expected cross-module leafref realtype")
	}
	if realType.Base() != cambium.BaseTypeString {
		t.Fatalf("realtype base = %v, want String", realType.Base())
	}
}

func TestInstanceIdentifierRequireInstanceConformance(t *testing.T) {
	cases := []struct {
		fixture string
		path    string
		want    bool
	}{
		{"types-instance-identifier-require-default", "/ref", true},
		{"types-instance-identifier-no-require", "/any-path", false},
		{"types-instance-identifier-complex-path", "/selected-member", true},
	}
	for _, tc := range cases {
		t.Run(tc.fixture, func(t *testing.T) {
			mod := loadConformanceSchema(t, tc.fixture)
			leaf := schemaNodeAt(t, mod, tc.path)
			info, ok := leaf.LeafType()
			if !ok {
				t.Fatal("expected leaf type")
			}
			inst, ok := info.Resolved().(cambium.ResolvedInstanceIdentifier)
			if !ok {
				t.Fatalf("expected instance-identifier, got %T", info.Resolved())
			}
			if inst.RequireInstance != tc.want {
				t.Fatalf("RequireInstance = %v, want %v", inst.RequireInstance, tc.want)
			}
		})
	}
}

func loadConformanceSchema(t *testing.T, name string) cambium.Module {
	t.Helper()
	ctx, err := cambium.NewContext()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ctx.Close() })
	if err := ctx.SetSearchPath(filepath.Join(conformanceRoot(t), "fixtures", name, "module")); err != nil {
		t.Fatal(err)
	}
	if err := ctx.LoadModule(name); err != nil {
		t.Fatalf("LoadModule(%s): %v", name, err)
	}
	mod, err := ctx.Schema(name)
	if err != nil {
		t.Fatalf("Schema(%s): %v", name, err)
	}
	return mod
}

func schemaNodeAt(t *testing.T, mod cambium.Module, path string) cambium.SchemaNodeRef {
	t.Helper()
	node, err := mod.FindPath(path)
	if err != nil {
		t.Fatalf("FindPath(%s): %v", path, err)
	}
	return node
}

func mustLeafrefPath(t *testing.T, leafref cambium.ResolvedLeafRef) string {
	t.Helper()
	path, ok := leafref.Path()
	if !ok {
		t.Fatal("expected leafref path")
	}
	return path
}

func TestListUnboundedMaxElementsIsNone(t *testing.T) {
	ctx, cleanup := introspectionContext(t)
	defer cleanup()

	top := topNode(t, ctx)
	list := childByName(t, top.Children(), "keyed-list")
	if _, ok := list.MinElements(); ok {
		t.Fatal("unbounded list min-elements should be unset")
	}
	if _, ok := list.MaxElements(); ok {
		t.Fatal("unbounded list max-elements should be unset")
	}
}

func TestMinElementsZeroAccepted(t *testing.T) {
	builder, err := cambium.NewContextBuilder(cambium.ContextFlags{})
	if err != nil {
		t.Fatal(err)
	}
	source := `module cambium-min-elements-zero {
    namespace "urn:cambium:min-elements-zero";
    prefix cmez;

    leaf-list values {
        min-elements 0;
        type string;
    }
}`
	if err := builder.LoadModuleStr(source); err != nil {
		t.Fatalf("LoadModuleStr: %v", err)
	}
	ctx, err := builder.Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	mod, err := ctx.Schema("cambium-min-elements-zero")
	if err != nil {
		t.Fatalf("Schema: %v", err)
	}
	values, err := mod.FindPath("/cmez:values")
	if err != nil {
		t.Fatalf("FindPath values: %v", err)
	}
	got, ok := values.MinElements()
	if !ok || got != 0 {
		t.Fatalf("MinElements = (%d,%v), want (0,true)", got, ok)
	}
}

func TestSchemaNodeRefPath(t *testing.T) {
	assertSchemaNodeRefPathRoundTrip(t)
}

func TestFindPathRoundTrip(t *testing.T) {
	// Spec-named alias for TestSchemaNodeRefPath.
	assertSchemaNodeRefPathRoundTrip(t)
}

func assertSchemaNodeRefPathRoundTrip(t *testing.T) {
	t.Helper()
	ctx, cleanup := introspectionContext(t)
	defer cleanup()

	top := topNode(t, ctx)
	if got, want := top.Path(), "/cambium-introspection-demo/top"; got != want {
		t.Fatalf("top.Path() = %q, want %q", got, want)
	}
	roundTrip, err := top.Module().FindPath(top.Path())
	if err != nil {
		t.Fatalf("FindPath(top.Path()) failed for %q: %v", top.Path(), err)
	}
	if got, want := roundTrip.Path(), top.Path(); got != want {
		t.Fatalf("FindPath(top.Path()).Path() = %q, want %q", got, want)
	}
	inner := childByName(t, childByName(t, top.Children(), "presence-box").Children(), "inner")
	if got, want := inner.Path(), "/cambium-introspection-demo/top/presence-box/inner"; got != want {
		t.Fatalf("inner.Path() = %q, want %q", got, want)
	}
	roundTrip, err = inner.Module().FindPath(inner.Path())
	if err != nil {
		t.Fatalf("FindPath(inner.Path()) failed for %q: %v", inner.Path(), err)
	}
	if got, want := roundTrip.Path(), inner.Path(); got != want {
		t.Fatalf("FindPath(inner.Path()).Path() = %q, want %q", got, want)
	}

	for _, path := range []string{"/missing:top", "/cid:top/missing:rw-flag", "cambium-introspection-demo:top", "/cambium-introspection-demo:top/"} {
		_, err := top.Module().FindPath(path)
		if err == nil {
			t.Fatalf("FindPath(%q) should fail", path)
		}
		var ce *cambium.Error
		if !errors.As(err, &ce) || ce.RuleCode() != cambium.RuleCodeContext {
			t.Fatalf("FindPath(%q) error = %v, want RuleCodeContext", path, err)
		}
	}
}

func TestSchemaNodeRefParentAncestors(t *testing.T) {
	ctx, cleanup := introspectionContext(t)
	defer cleanup()

	top := topNode(t, ctx)
	presence := childByName(t, top.Children(), "presence-box")
	inner := childByName(t, presence.Children(), "inner")

	root, ok := top.Parent()
	if !ok {
		t.Fatal("top-level node should have synthetic module root parent")
	}
	if root.Kind() != cambium.SchemaNodeKindModule {
		t.Fatalf("top.Parent().Kind() = %v, want module", root.Kind())
	}
	if got, want := root.Path(), "/cambium-introspection-demo"; got != want {
		t.Fatalf("top.Parent().Path() = %q, want %q", got, want)
	}
	if _, ok := root.Parent(); ok {
		t.Fatal("synthetic module root should have no Parent")
	}
	if len(top.Ancestors()) != 0 {
		t.Fatalf("top-level Ancestors = %v, want empty", top.Ancestors())
	}

	parent, ok := inner.Parent()
	if !ok || parent.Name() != "presence-box" {
		t.Fatalf("inner.Parent() = (%q, %v), want (presence-box, true)", parent.Name(), ok)
	}
	ancestors := inner.Ancestors()
	want := []string{"top", "presence-box"}
	if len(ancestors) != len(want) {
		t.Fatalf("inner.Ancestors() len = %d, want %d", len(ancestors), len(want))
	}
	for i, name := range want {
		if ancestors[i].Name() != name {
			t.Fatalf("inner.Ancestors()[%d] = %q, want %q", i, ancestors[i].Name(), name)
		}
	}
}

func TestSchemaChildrenLookup(t *testing.T) {
	ctx, cleanup := introspectionContext(t)
	defer cleanup()

	top := topNode(t, ctx)
	children := top.Children()

	c, ok := children.Lookup("ro-counter")
	if !ok || c.Name() != "ro-counter" {
		t.Fatalf("Lookup(ro-counter) = (%q, %v), want (ro-counter, true)", c.Name(), ok)
	}
	if _, ok := children.Lookup("not-there"); ok {
		t.Fatal("Lookup(not-there) should be false")
	}
}

func TestSchemaNodeDefaultValues(t *testing.T) {
	ctx, cleanup := introspectionContext(t)
	defer cleanup()

	top := topNode(t, ctx)
	children := top.Children()

	rw := childByName(t, children, "rw-flag")
	vals := rw.DefaultValues()
	if len(vals) != 1 || vals[0] != "true" {
		t.Fatalf("rw-flag.DefaultValues() = %v, want [true]", vals)
	}
	if v, ok := rw.DefaultValue(); !ok || v != "true" {
		t.Fatalf("rw-flag.DefaultValue() = (%q, %v), want (true, true)", v, ok)
	}

	multi := childByName(t, children, "multi-defaults")
	mvals := multi.DefaultValues()
	want := []string{"alpha", "beta", "gamma"}
	if len(mvals) != len(want) {
		t.Fatalf("multi-defaults.DefaultValues() len = %d, want %d", len(mvals), len(want))
	}
	for i := range want {
		if mvals[i] != want[i] {
			t.Fatalf("multi-defaults.DefaultValues()[%d] = %q, want %q", i, mvals[i], want[i])
		}
	}

	inner := childByName(t, childByName(t, children, "presence-box").Children(), "inner")
	if len(inner.DefaultValues()) != 0 {
		t.Fatalf("inner.DefaultValues() = %v, want empty", inner.DefaultValues())
	}
	if _, ok := inner.DefaultValue(); ok {
		t.Fatal("inner.DefaultValue() should be false")
	}
}

func TestSchemaNodeTypeDefaultValue(t *testing.T) {
	source := `module cambium-type-default-demo {
    namespace "urn:cambium:type-default-demo";
    prefix ctdd;

    typedef inherited-string {
        type string;
        default "inherited";
    }

    container top {
        leaf inherited {
            type inherited-string;
        }
        leaf explicit {
            type string;
            default "explicit";
        }
    }
}`
	builder, err := cambium.NewContextBuilder(cambium.ContextFlags{})
	if err != nil {
		t.Fatalf("NewContextBuilder: %v", err)
	}
	if err := builder.LoadModuleStr(source); err != nil {
		t.Fatalf("LoadModuleStr: %v", err)
	}
	ctx, err := builder.Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	defer ctx.Close()

	mod, err := ctx.Schema("cambium-type-default-demo")
	if err != nil {
		t.Fatalf("Schema: %v", err)
	}
	inherited := schemaNodeAt(t, mod, "/ctdd:top/inherited")
	if got, ok := inherited.TypeDefaultValue(); !ok || got != "inherited" {
		t.Fatalf("inherited.TypeDefaultValue() = (%q,%v), want inherited,true", got, ok)
	}
	if got, ok := inherited.DefaultValue(); !ok || got != "inherited" {
		t.Fatalf("inherited.DefaultValue() = (%q,%v), want inherited,true", got, ok)
	}

	explicit := schemaNodeAt(t, mod, "/ctdd:top/explicit")
	if got, ok := explicit.TypeDefaultValue(); ok || got != "" {
		t.Fatalf("explicit.TypeDefaultValue() = (%q,%v), want empty,false", got, ok)
	}
	if got, ok := explicit.DefaultValue(); !ok || got != "explicit" {
		t.Fatalf("explicit.DefaultValue() = (%q,%v), want explicit,true", got, ok)
	}
}

func TestResolvedLeafRefPath(t *testing.T) {
	ctx, cleanup := introspectionContext(t)
	defer cleanup()

	top := topNode(t, ctx)
	leaf := childByName(t, top.Children(), "ref-to-name")
	info, ok := leaf.LeafType()
	if !ok {
		t.Fatal("expected leaf type")
	}
	r, ok := info.Resolved().(cambium.ResolvedLeafRef)
	if !ok {
		t.Fatalf("expected leafref, got %T", info.Resolved())
	}
	path, ok := r.Path()
	if !ok {
		t.Fatal("expected leafref path")
	}
	if want := "/cid:top/keyed-list/name"; path != want {
		t.Fatalf("leafref.Path() = %q, want %q", path, want)
	}
}

func TestSchemaNodeExtensions(t *testing.T) {
	ctx, cleanup := introspectionContext(t)
	defer cleanup()

	top := topNode(t, ctx)
	rw := childByName(t, top.Children(), "rw-flag")

	exts := rw.Extensions()
	if len(exts) != 2 {
		t.Fatalf("Extensions() len = %d, want 2", len(exts))
	}
	want := []string{"flag-foo", "camelcase-name"}
	for i, name := range want {
		if exts[i].Name() != name {
			t.Fatalf("Extensions()[%d].Name() = %q, want %q", i, exts[i].Name(), name)
		}
	}

	camel, ok := rw.Extension("camelcase-name")
	if !ok {
		t.Fatal("Extension(camelcase-name) not found")
	}
	if camel.Name() != "camelcase-name" {
		t.Fatalf("Extension name = %q, want camelcase-name", camel.Name())
	}
	if arg, ok := camel.Argument(); !ok || arg != "fooBar" {
		t.Fatalf("Extension argument = (%q, %v), want (fooBar, true)", arg, ok)
	}
	if camel.ModuleName() != introspectionModuleName {
		t.Fatalf("Extension module = %q, want %q", camel.ModuleName(), introspectionModuleName)
	}

	flag, ok := rw.Extension("flag-foo")
	if !ok {
		t.Fatal("Extension(flag-foo) not found")
	}
	if _, ok := flag.Argument(); ok {
		t.Fatal("flag-foo should have no argument")
	}

	if _, ok := rw.Extension("missing"); ok {
		t.Fatal("Extension(missing) should be false")
	}
}

func TestModuleExtensionDefinitionMetadataAccessors(t *testing.T) {
	ctx, cleanup := introspectionContext(t)
	defer cleanup()

	mod, err := ctx.Schema(introspectionModuleName)
	if err != nil {
		t.Fatalf("Schema: %v", err)
	}
	defs := mod.ExtensionDefinitions()
	if got, want := len(defs), 2; got != want {
		t.Fatalf("ExtensionDefinitions len = %d, want %d", got, want)
	}
	if defs[0].Name() != "camelcase-name" || defs[1].Name() != "flag-foo" {
		t.Fatalf("ExtensionDefinitions = [%q, %q], want [camelcase-name, flag-foo]", defs[0].Name(), defs[1].Name())
	}
	if got := defs[0].Module().Name(); got != introspectionModuleName {
		t.Fatalf("camelcase Module = %q, want %s", got, introspectionModuleName)
	}
	if arg, ok := defs[0].Argument(); !ok || arg != "value" {
		t.Fatalf("camelcase Argument = (%q,%v), want value,true", arg, ok)
	}
	if yin, ok := defs[0].YinElement(); !ok || !yin {
		t.Fatalf("camelcase YinElement = (%v,%v), want true,true", yin, ok)
	}
	if got := defs[0].Status(); got != cambium.StatusDeprecated {
		t.Fatalf("camelcase Status = %v, want deprecated", got)
	}
	if got, ok := defs[0].Description(); !ok || got != "Camelcase extension." {
		t.Fatalf("camelcase Description = (%q,%v), want Camelcase extension.,true", got, ok)
	}
	if got, ok := defs[0].Reference(); !ok || got != "camel-ref" {
		t.Fatalf("camelcase Reference = (%q,%v), want camel-ref,true", got, ok)
	}
	if _, ok := defs[1].Argument(); ok {
		t.Fatal("flag-foo Argument should be absent")
	}
	if _, ok := defs[1].YinElement(); ok {
		t.Fatal("flag-foo YinElement should be absent")
	}
	if got := defs[1].Status(); got != cambium.StatusCurrent {
		t.Fatalf("flag-foo Status = %v, want current", got)
	}
}

func TestModuleExtensionsExposeTopLevelInstances(t *testing.T) {
	source := `module cambium-module-extensions {
    namespace "urn:cambium:module-extensions";
    prefix cme;

    extension marker {
        argument value;
    }

    cme:marker "module-level";

    container top {
        leaf name { type string; }
    }
}`
	builder, err := cambium.NewContextBuilder(cambium.ContextFlags{})
	if err != nil {
		t.Fatal(err)
	}
	if err := builder.LoadModuleStr(source); err != nil {
		t.Fatalf("LoadModuleStr: %v", err)
	}
	ctx, err := builder.Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	t.Cleanup(func() { ctx.Close() })
	mod, err := ctx.Schema("cambium-module-extensions")
	if err != nil {
		t.Fatalf("Schema: %v", err)
	}
	exts := mod.Extensions()
	if len(exts) != 1 {
		t.Fatalf("Extensions() len = %d, want 1", len(exts))
	}
	if got := exts[0].Name(); got != "marker" {
		t.Fatalf("Extensions()[0].Name() = %q, want marker", got)
	}
	if got := exts[0].ModuleName(); got != "cambium-module-extensions" {
		t.Fatalf("Extensions()[0].ModuleName() = %q, want cambium-module-extensions", got)
	}
	if got, ok := exts[0].Argument(); !ok || got != "module-level" {
		t.Fatalf("Extensions()[0].Argument() = (%q,%v), want module-level,true", got, ok)
	}
}

func TestModuleTypedefDefinitionMetadataAccessors(t *testing.T) {
	ctx, cleanup := introspectionContext(t)
	defer cleanup()

	mod, err := ctx.Schema(introspectionModuleName)
	if err != nil {
		t.Fatalf("Schema: %v", err)
	}
	defs := mod.TypedefDefinitions()
	if got, want := len(defs), 2; got != want {
		t.Fatalf("TypedefDefinitions len = %d, want %d", got, want)
	}
	if defs[0].Name() != "small-number" || defs[1].Name() != "plain-text" {
		t.Fatalf("TypedefDefinitions = [%q, %q], want [small-number, plain-text]", defs[0].Name(), defs[1].Name())
	}
	if got := defs[0].Module().Name(); got != introspectionModuleName {
		t.Fatalf("small-number Module = %q, want %s", got, introspectionModuleName)
	}
	info, ok := defs[0].Type()
	if !ok {
		t.Fatal("small-number Type returned false")
	}
	if got := info.Base(); got != cambium.BaseTypeUint16 {
		t.Fatalf("small-number Type base = %v, want uint16", got)
	}
	if got, ok := defs[0].Units(); !ok || got != "widgets" {
		t.Fatalf("small-number Units = (%q,%v), want widgets,true", got, ok)
	}
	if got, ok := defs[0].Default(); !ok || got != "7" {
		t.Fatalf("small-number Default = (%q,%v), want 7,true", got, ok)
	}
	if got := defs[0].Status(); got != cambium.StatusDeprecated {
		t.Fatalf("small-number Status = %v, want deprecated", got)
	}
	if got, ok := defs[0].Description(); !ok || got != "Small number typedef." {
		t.Fatalf("small-number Description = (%q,%v), want Small number typedef.,true", got, ok)
	}
	if got, ok := defs[0].Reference(); !ok || got != "small-number-ref" {
		t.Fatalf("small-number Reference = (%q,%v), want small-number-ref,true", got, ok)
	}
	if _, ok := defs[1].Default(); ok {
		t.Fatal("plain-text Default should be absent")
	}
	if got := defs[1].Status(); got != cambium.StatusCurrent {
		t.Fatalf("plain-text Status = %v, want current", got)
	}
}

func TestExtensionPrefixedStatementMayContainTypeChild(t *testing.T) {
	source := `module cambium-extension-type-child {
    namespace "urn:cambium:extension-type-child";
    prefix cetc;

    extension annotation;

    leaf value {
        type string;
        cetc:annotation {
            type string;
        }
    }
}`
	builder, err := cambium.NewContextBuilder(cambium.ContextFlags{})
	if err != nil {
		t.Fatal(err)
	}
	if err := builder.LoadModuleStr(source); err != nil {
		t.Fatalf("LoadModuleStr: %v", err)
	}
	ctx, err := builder.Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	t.Cleanup(func() { ctx.Close() })
	mod, err := ctx.Schema("cambium-extension-type-child")
	if err != nil {
		t.Fatalf("Schema: %v", err)
	}
	value := schemaNodeAt(t, mod, "/cetc:value")
	exts := value.Extensions()
	if len(exts) != 1 || exts[0].Name() != "annotation" {
		t.Fatalf("Extensions() = %#v, want annotation", exts)
	}
}

func TestChoiceCaseFlattenHelpers(t *testing.T) {
	ctx, cleanup := introspectionContext(t)
	defer cleanup()

	top := topNode(t, ctx)
	choice := childByName(t, top.Children(), "preference")

	cases := choice.Children()
	if cases.Len() != 2 {
		t.Fatalf("choice.Children() len = %d, want 2", cases.Len())
	}
	primary, _ := cases.Get(0)
	if primary.Name() != "primary" {
		t.Fatalf("choice child[0] = %q, want primary", primary.Name())
	}

	// DataChildren(false) is the same as Children().
	direct := choice.DataChildren(false)
	if direct.Len() != cases.Len() {
		t.Fatalf("DataChildren(false) len = %d, want %d", direct.Len(), cases.Len())
	}

	// DataChildren(true) flattens cases into their non-choice/case descendants.
	flat := choice.DataChildren(true)
	if flat.Len() != 2 {
		t.Fatalf("DataChildren(true) len = %d, want 2", flat.Len())
	}
	pLeaf, _ := flat.Get(0)
	if pLeaf.Name() != "primary-name" {
		t.Fatalf("flat[0] = %q, want primary-name", pLeaf.Name())
	}
	sLeaf, _ := flat.Get(1)
	if sLeaf.Name() != "secondary-name" {
		t.Fatalf("flat[1] = %q, want secondary-name", sLeaf.Name())
	}
	if !pLeaf.IsChoiceDescendant() {
		t.Fatal("primary-name should be a choice descendant")
	}
	if !sLeaf.IsChoiceDescendant() {
		t.Fatal("secondary-name should be a choice descendant")
	}

	topData := top.DataChildren(false)
	if _, ok := topData.Lookup("rw-flag"); !ok {
		t.Fatal("top.DataChildren(false) should include data leaf rw-flag")
	}
	if _, ok := topData.Lookup("preference"); !ok {
		t.Fatal("top.DataChildren(false) should include choice preference")
	}
	if _, ok := topData.Lookup("reset"); ok {
		t.Fatal("top.DataChildren(false) should exclude nested action reset")
	}

	topFlatData := top.DataChildren(true)
	if _, ok := topFlatData.Lookup("primary-name"); !ok {
		t.Fatal("top.DataChildren(true) should include flattened choice leaf primary-name")
	}
	if _, ok := topFlatData.Lookup("preference"); ok {
		t.Fatal("top.DataChildren(true) should exclude choice preference")
	}
	if _, ok := topFlatData.Lookup("reset"); ok {
		t.Fatal("top.DataChildren(true) should exclude nested action reset")
	}

	// A sibling outside the choice is not a choice descendant.
	rw := childByName(t, top.Children(), "rw-flag")
	if rw.IsChoiceDescendant() {
		t.Fatal("rw-flag should not be a choice descendant")
	}
}

func TestIfFeatureOnChoiceShorthandCaseRecorded(t *testing.T) {
	t.Helper()
	source := `module cambium-if-feature-shorthand-case {
    namespace "urn:cambium:if-feature-shorthand-case";
    prefix cifsc;
    yang-version 1.1;

    feature advanced;

    container top {
        choice mode {
            leaf gated {
                if-feature advanced;
                type string;
            }
            leaf always {
                type string;
            }
        }
    }
}
`
	builder, err := cambium.NewContextBuilder(cambium.ContextFlags{})
	if err != nil {
		t.Fatal(err)
	}
	if err := builder.LoadModuleStr(source); err != nil {
		t.Fatalf("LoadModuleStr: %v", err)
	}
	ctx, err := builder.Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	defer ctx.Close()
	mod, err := ctx.Schema("cambium-if-feature-shorthand-case")
	if err != nil {
		t.Fatalf("Schema: %v", err)
	}
	choice, err := mod.FindPath("/cifsc:top/cifsc:mode")
	if err != nil {
		t.Fatalf("FindPath choice: %v", err)
	}
	if got, want := schemaChildNames(choice.Children()), []string{"always"}; len(got) != len(want) || got[0] != want[0] {
		t.Fatalf("disabled choice children = %v, want %v", got, want)
	}

	enabledBuilder, err := cambium.NewContextBuilder(cambium.ContextFlags{})
	if err != nil {
		t.Fatal(err)
	}
	if err := enabledBuilder.SetFeatures("cambium-if-feature-shorthand-case", []string{"advanced"}); err != nil {
		t.Fatal(err)
	}
	if err := enabledBuilder.LoadModuleStr(source); err != nil {
		t.Fatalf("enabled LoadModuleStr: %v", err)
	}
	enabledCtx, err := enabledBuilder.Build()
	if err != nil {
		t.Fatalf("enabled Build: %v", err)
	}
	defer enabledCtx.Close()
	enabledMod, err := enabledCtx.Schema("cambium-if-feature-shorthand-case")
	if err != nil {
		t.Fatalf("enabled Schema: %v", err)
	}
	enabledChoice, err := enabledMod.FindPath("/cifsc:top/cifsc:mode")
	if err != nil {
		t.Fatalf("enabled FindPath choice: %v", err)
	}
	gatedCase := childByName(t, enabledChoice.Children(), "gated")
	if gatedCase.Kind() != cambium.SchemaNodeKindCase {
		t.Fatalf("gated shorthand kind = %v, want Case", gatedCase.Kind())
	}
	if got, want := gatedCase.IfFeatures(), []string{"advanced"}; len(got) != len(want) || got[0] != want[0] {
		t.Fatalf("gated implicit case IfFeatures = %v, want %v", got, want)
	}
	gatedLeaf := childByName(t, gatedCase.Children(), "gated")
	if got, want := gatedLeaf.IfFeatures(), []string{"advanced"}; len(got) != len(want) || got[0] != want[0] {
		t.Fatalf("gated shorthand leaf IfFeatures = %v, want %v", got, want)
	}
}

func TestMustWhenIntrospection(t *testing.T) {
	ctx, cleanup := introspectionContext(t)
	defer cleanup()

	top := topNode(t, ctx)

	mand := childByName(t, top.Children(), "mandatory-leaf")
	musts := mand.Musts()
	if len(musts) != 1 {
		t.Fatalf("mandatory-leaf musts = %d, want 1", len(musts))
	}
	m := musts[0]
	if m.Expression() != "../rw-flag = 'true'" {
		t.Fatalf("must expression = %q, want ../rw-flag = 'true'", m.Expression())
	}
	if msg, ok := m.ErrorMessage(); !ok || msg != "rw-flag must be true" {
		t.Fatalf("must error-message = (%q, %v), want (rw-flag must be true, true)", msg, ok)
	}
	if tag, ok := m.ErrorAppTag(); !ok || tag != "must-rw-flag" {
		t.Fatalf("must error-app-tag = (%q, %v), want (must-rw-flag, true)", tag, ok)
	}

	pc := childByName(t, top.Children(), "presence-box")
	whens := pc.Whens()
	if len(whens) != 1 {
		t.Fatalf("presence-box whens = %d, want 1", len(whens))
	}
	w := whens[0]
	if w.Expression() != "../rw-flag = 'true'" {
		t.Fatalf("when expression = %q, want ../rw-flag = 'true'", w.Expression())
	}
	if dsc, ok := w.Description(); !ok || dsc != "Only present when rw-flag is true" {
		t.Fatalf("when description = (%q, %v), want (Only present when rw-flag is true, true)", dsc, ok)
	}
	if ref, ok := w.Reference(); !ok || ref != "RFC 6020 when statement" {
		t.Fatalf("when reference = (%q, %v), want (RFC 6020 when statement, true)", ref, ok)
	}

	// Siblings without must/when return empty slices.
	rw := childByName(t, top.Children(), "rw-flag")
	if len(rw.Musts()) != 0 {
		t.Fatalf("rw-flag musts = %d, want 0", len(rw.Musts()))
	}
	if len(rw.Whens()) != 0 {
		t.Fatalf("rw-flag whens = %d, want 0", len(rw.Whens()))
	}
}

func TestXPathStandardFunctionsAccepted(t *testing.T) {
	const source = `module cambium-xpath-standard-functions {
    namespace "urn:cambium:xpath-standard-functions";
    prefix cxsf;

    leaf-list sample {
        type uint8;
    }

    container top {
        must "sum(../sample) >= 0";
        must "floor(1.2) = 1";
        must "ceiling(1.2) = 2";
        must "round(1.2) = 1";
        must "count(../node()) >= 0";
        must "count(../text()) >= 0";
        leaf value { type string; }
    }
}`
	builder, err := cambium.NewContextBuilder(cambium.ContextFlags{})
	if err != nil {
		t.Fatal(err)
	}
	if err := builder.LoadModuleStr(source); err != nil {
		t.Fatalf("LoadModuleStr: %v", err)
	}
	ctx, err := builder.Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	defer ctx.Close()
	mod, err := ctx.Schema("cambium-xpath-standard-functions")
	if err != nil {
		t.Fatalf("Schema: %v", err)
	}
	top := schemaNodeAt(t, mod, "/cxsf:top")
	musts := top.Musts()
	if got, want := len(musts), 6; got != want {
		t.Fatalf("musts = %d, want %d", got, want)
	}
}

func TestXPathReMatchCompoundMustAccepted(t *testing.T) {
	const source = `module cambium-xpath-rematch-compound-must {
    yang-version 1.1;
    namespace "urn:cambium:xpath-rematch-compound-must";
    prefix cxrcm;

    container top {
        leaf format-type {
            type enumeration {
                enum uuid;
                enum slug;
                enum free;
            }
        }
        leaf formatted {
            must "(../format-type = 'uuid' and re-match(., '^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$')) or (../format-type = 'slug' and re-match(., '^[a-z0-9-]+$')) or (../format-type = 'free')";
            type string;
        }
    }
}`
	builder, err := cambium.NewContextBuilder(cambium.ContextFlags{})
	if err != nil {
		t.Fatal(err)
	}
	if err := builder.LoadModuleStr(source); err != nil {
		t.Fatalf("LoadModuleStr: %v", err)
	}
	ctx, err := builder.Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	defer ctx.Close()
	mod, err := ctx.Schema("cambium-xpath-rematch-compound-must")
	if err != nil {
		t.Fatalf("Schema: %v", err)
	}
	formatted := schemaNodeAt(t, mod, "/cxrcm:top/formatted")
	musts := formatted.Musts()
	if got, want := len(musts), 1; got != want {
		t.Fatalf("musts = %d, want %d", got, want)
	}
}

func TestListUniqueConstraints(t *testing.T) {
	ctx, cleanup := introspectionContext(t)
	defer cleanup()

	top := topNode(t, ctx)
	list := childByName(t, top.Children(), "keyed-list")
	ucs := list.UniqueConstraints()
	if len(ucs) != 1 {
		t.Fatalf("unique constraints = %d, want 1", len(ucs))
	}
	leafs := ucs[0].Leafs()
	if len(leafs) != 2 {
		t.Fatalf("unique leafs = %d, want 2", len(leafs))
	}
	want := []string{"name", "extra"}
	for i, name := range want {
		if leafs[i].Name() != name {
			t.Fatalf("unique leaf[%d] = %q, want %q", i, leafs[i].Name(), name)
		}
	}

	// Non-list nodes return nil.
	nameLeaf := childByName(t, list.Children(), "name")
	if nameLeaf.UniqueConstraints() != nil {
		t.Fatal("non-list node UniqueConstraints should be nil")
	}
}

func TestSchemaNodeKindPredicatesReadOnlyNamespace(t *testing.T) {
	ctx, cleanup := introspectionContext(t)
	defer cleanup()

	top := topNode(t, ctx)
	children := top.Children()

	rw := childByName(t, children, "rw-flag")
	if !rw.IsLeaf() {
		t.Fatalf("rw-flag IsLeaf() = false, want true")
	}
	if rw.IsLeafList() || rw.IsContainer() || rw.IsList() || rw.IsChoice() || rw.IsCase() {
		t.Fatalf("rw-flag kind predicates should only report leaf")
	}
	if rw.IsDir() {
		t.Fatal("leaf should not be IsDir")
	}
	if rw.ReadOnly() {
		t.Fatal("rw-flag should not be ReadOnly")
	}

	ll := childByName(t, children, "typed-leaf-list")
	if !ll.IsLeafList() || ll.IsLeaf() || ll.IsContainer() || ll.IsList() {
		t.Fatalf("typed-leaf-list kind predicates mismatch")
	}

	cont := childByName(t, children, "presence-box")
	if !cont.IsContainer() || cont.IsList() || cont.IsChoice() || cont.IsCase() {
		t.Fatalf("presence-box kind predicates mismatch")
	}
	if !cont.IsDir() {
		t.Fatal("container should be IsDir")
	}

	list := childByName(t, children, "keyed-list")
	if !list.IsList() || list.IsContainer() || list.IsLeaf() {
		t.Fatalf("keyed-list kind predicates mismatch")
	}
	if !list.IsDir() {
		t.Fatal("list should be IsDir")
	}

	choice := childByName(t, children, "preference")
	if !choice.IsChoice() || choice.IsCase() || choice.IsContainer() {
		t.Fatalf("preference kind predicates mismatch")
	}
	if !choice.IsDir() {
		t.Fatal("choice should be IsDir")
	}

	cases := choice.Children()
	primary := childByName(t, cases, "primary")
	if !primary.IsCase() || primary.IsChoice() || primary.IsLeaf() {
		t.Fatalf("primary kind predicates mismatch")
	}
	if !primary.IsDir() {
		t.Fatal("case should be IsDir")
	}
	pLeaf := childByName(t, primary.Children(), "primary-name")
	if !pLeaf.IsLeaf() || pLeaf.IsCase() || pLeaf.IsDir() {
		t.Fatalf("primary-name kind predicates mismatch")
	}

	actionNode := childByName(t, children, "reset")
	if actionNode.Kind() != cambium.SchemaNodeKindAction {
		t.Fatalf("reset kind = %v, want Action", actionNode.Kind())
	}
	if !actionNode.IsDir() {
		t.Fatal("action should be IsDir")
	}
	actionChildren := actionNode.Children()
	input := childByName(t, actionChildren, "input")
	if input.Kind() != cambium.SchemaNodeKindInput || !input.IsDir() {
		t.Fatalf("input kind predicates mismatch")
	}
	output := childByName(t, actionChildren, "output")
	if output.Kind() != cambium.SchemaNodeKindOutput || !output.IsDir() {
		t.Fatalf("output kind predicates mismatch")
	}

	ro := childByName(t, children, "ro-counter")
	if !ro.ReadOnly() {
		t.Fatal("ro-counter should be ReadOnly")
	}
	if ro.Config() != cambium.ConfigRo {
		t.Fatalf("ro-counter Config = %v, want Ro", ro.Config())
	}

	wantNs := "urn:cambium:introspection"
	if got := top.Namespace(); got != wantNs {
		t.Fatalf("top.Namespace() = %q, want %q", got, wantNs)
	}
	if got := rw.Namespace(); got != wantNs {
		t.Fatalf("rw-flag.Namespace() = %q, want %q", got, wantNs)
	}
}

func TestModuleOperations(t *testing.T) {
	ctx, cleanup := introspectionContext(t)
	defer cleanup()

	mod, err := ctx.Schema(introspectionModuleName)
	if err != nil {
		t.Fatalf("Schema: %v", err)
	}

	rpcs := mod.RPCs()
	if rpcs.Len() != 1 {
		t.Fatalf("RPCs() len = %d, want 1", rpcs.Len())
	}
	rpc, _ := rpcs.Get(0)
	if rpc.Name() != "reboot" {
		t.Fatalf("RPC name = %q, want reboot", rpc.Name())
	}
	if !rpc.IsRPC() {
		t.Fatal("reboot should be IsRPC")
	}
	if rpc.IsAction() || rpc.IsNotification() {
		t.Fatal("reboot should not be action or notification")
	}
	rpcParent, ok := rpc.Parent()
	if !ok {
		t.Fatal("module-level RPC should have synthetic module root parent")
	}
	if rpcParent.Kind() != cambium.SchemaNodeKindModule {
		t.Fatalf("RPC parent kind = %v, want module", rpcParent.Kind())
	}
	rpcChildren := rpc.Children()
	if rpcChildren.Len() != 2 {
		t.Fatalf("reboot children len = %d, want 2", rpcChildren.Len())
	}
	inNode, _ := rpcChildren.Get(0)
	if inNode.Kind() != cambium.SchemaNodeKindInput {
		t.Fatalf("reboot child[0] kind = %v, want Input", inNode.Kind())
	}
	rpcInput, ok := rpc.Input()
	if !ok {
		t.Fatal("reboot Input() returned false")
	}
	if rpcInput.Kind() != cambium.SchemaNodeKindInput {
		t.Fatalf("reboot Input() kind = %v, want Input", rpcInput.Kind())
	}
	if got := schemaChildNames(rpcInput.Children()); strings.Join(got, ",") != "delay" {
		t.Fatalf("reboot Input() children = %v, want [delay]", got)
	}
	rpcOutput, ok := rpc.Output()
	if !ok {
		t.Fatal("reboot Output() returned false")
	}
	if rpcOutput.Kind() != cambium.SchemaNodeKindOutput {
		t.Fatalf("reboot Output() kind = %v, want Output", rpcOutput.Kind())
	}
	if got := schemaChildNames(rpcOutput.Children()); strings.Join(got, ",") != "result" {
		t.Fatalf("reboot Output() children = %v, want [result]", got)
	}

	notifs := mod.Notifications()
	if notifs.Len() != 1 {
		t.Fatalf("Notifications() len = %d, want 1", notifs.Len())
	}
	notif, _ := notifs.Get(0)
	if notif.Name() != "event" {
		t.Fatalf("Notification name = %q, want event", notif.Name())
	}
	if !notif.IsNotification() {
		t.Fatal("event should be IsNotification")
	}
	if notif.IsRPC() || notif.IsAction() {
		t.Fatal("event should not be rpc or action")
	}
	notifParent, ok := notif.Parent()
	if !ok {
		t.Fatal("module-level notification should have synthetic module root parent")
	}
	if notifParent.Kind() != cambium.SchemaNodeKindModule {
		t.Fatalf("notification parent kind = %v, want module", notifParent.Kind())
	}
	notifChildren := notif.Children()
	if notifChildren.Len() != 1 {
		t.Fatalf("event children len = %d, want 1", notifChildren.Len())
	}
	sev, _ := notifChildren.Get(0)
	if sev.Name() != "severity" {
		t.Fatalf("event child[0] = %q, want severity", sev.Name())
	}
	if _, ok := notif.Input(); ok {
		t.Fatal("notification Input() returned true")
	}
	if _, ok := notif.Output(); ok {
		t.Fatal("notification Output() returned true")
	}

	actions := mod.Actions()
	if actions.Len() != 0 {
		t.Fatalf("Actions() len = %d, want 0 (actions are nested, not module-level)", actions.Len())
	}

	// The action nested under top is reachable through the data tree.
	top := topNode(t, ctx)
	reset := childByName(t, top.Children(), "reset")
	if !reset.IsAction() {
		t.Fatal("reset should be IsAction")
	}
	if reset.IsRPC() || reset.IsNotification() {
		t.Fatal("reset should not be rpc or notification")
	}
	resetInput, ok := reset.Input()
	if !ok {
		t.Fatal("reset Input() returned false")
	}
	if got := schemaChildNames(resetInput.Children()); strings.Join(got, ",") != "in-val" {
		t.Fatalf("reset Input() children = %v, want [in-val]", got)
	}
	resetOutput, ok := reset.Output()
	if !ok {
		t.Fatal("reset Output() returned false")
	}
	if got := schemaChildNames(resetOutput.Children()); strings.Join(got, ",") != "out-val" {
		t.Fatalf("reset Output() children = %v, want [out-val]", got)
	}
	if _, ok := top.Input(); ok {
		t.Fatal("ordinary container Input() returned true")
	}
	if _, ok := top.Output(); ok {
		t.Fatal("ordinary container Output() returned true")
	}
}

func TestOperationPayloadListsMayBeKeyless(t *testing.T) {
	t.Helper()
	dir := schemaIntrospectionModuleDir(t)
	module := `module cambium-operation-keyless-list {
    namespace "urn:cambium:operation-keyless-list";
    prefix cokl;
    yang-version 1.1;

    rpc collect {
        input {
            list filter {
                leaf expression {
                    type string;
                }
            }
        }
        output {
            list sample {
                leaf value {
                    type string;
                }
            }
        }
    }

    container top {
        action run {
            input {
                list argument {
                    leaf value {
                        type string;
                    }
                }
            }
        }
    }

    notification changed {
        list event {
            leaf message {
                type string;
            }
        }
    }
}
`
	writeModuleFile(t, filepath.Join(dir, "cambium-operation-keyless-list.yang"), []byte(module))

	ctx, err := cambium.NewContext()
	if err != nil {
		t.Fatal(err)
	}
	defer ctx.Close()
	if err := ctx.SetSearchPath(dir); err != nil {
		t.Fatal(err)
	}
	if err := ctx.LoadModule("cambium-operation-keyless-list"); err != nil {
		t.Fatalf("LoadModule: %v", err)
	}
	mod, err := ctx.Schema("cambium-operation-keyless-list")
	if err != nil {
		t.Fatalf("Schema: %v", err)
	}

	for _, path := range []string{
		"/cokl:collect/input/filter",
		"/cokl:collect/output/sample",
		"/cokl:top/run/input/argument",
		"/cokl:changed/event",
	} {
		node, err := mod.FindPath(path)
		if err != nil {
			t.Fatalf("FindPath %s: %v", path, err)
		}
		if !node.IsList() {
			t.Fatalf("%s kind = %v, want list", path, node.Kind())
		}
		if keys := node.ListKeys(); keys.Len() != 0 {
			t.Fatalf("%s ListKeys len = %d, want 0", path, keys.Len())
		}
		if node.RepresentsConfigurationData() {
			t.Fatalf("%s RepresentsConfigurationData = true, want false", path)
		}
	}
}

func TestOperationPayloadUniqueAllowsRawConfigStateMix(t *testing.T) {
	const source = `module cambium-operation-unique-config-mix {
    namespace "urn:cambium:operation-unique-config-mix";
    prefix coucm;
    yang-version 1.1;

    rpc collect {
        input {
            list sample {
                unique "name state";
                leaf name {
                    type string;
                }
                leaf state {
                    config false;
                    type string;
                }
            }
        }
    }
}`
	builder, err := cambium.NewContextBuilder(cambium.ContextFlags{})
	if err != nil {
		t.Fatal(err)
	}
	if err := builder.LoadModuleStr(source); err != nil {
		t.Fatalf("LoadModuleStr: %v", err)
	}
	ctx, err := builder.Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	defer ctx.Close()

	mod, err := ctx.Schema("cambium-operation-unique-config-mix")
	if err != nil {
		t.Fatalf("Schema: %v", err)
	}
	sample, err := mod.FindPath("/coucm:collect/input/sample")
	if err != nil {
		t.Fatalf("FindPath sample: %v", err)
	}
	ucs := sample.UniqueConstraints()
	if len(ucs) != 1 {
		t.Fatalf("unique constraints = %d, want 1", len(ucs))
	}
	leafs := ucs[0].Leafs()
	want := []string{"name", "state"}
	if len(leafs) != len(want) {
		t.Fatalf("unique leafs = %d, want %d", len(leafs), len(want))
	}
	for i, name := range want {
		if leafs[i].Name() != name {
			t.Fatalf("unique leaf[%d] = %q, want %q", i, leafs[i].Name(), name)
		}
		if leafs[i].RepresentsConfigurationData() {
			t.Fatalf("unique leaf[%d] RepresentsConfigurationData = true, want false", i)
		}
	}
}

func TestModuleRootChildrenPreserveOperationInterleaving(t *testing.T) {
	dir := schemaIntrospectionModuleDir(t)

	const moduleName = "cambium-operation-interleave"
	module := `module cambium-operation-interleave {
    namespace "urn:cambium:operation-interleave";
    prefix coi;
    yang-version 1.1;

    container first {
        leaf value {
            type string;
        }
    }

    rpc do-it {
        input {
            leaf request {
                type string;
            }
        }
    }

    container middle {
        leaf value {
            type string;
        }
    }

    notification alarm {
        leaf severity {
            type string;
        }
    }

    container last {
        leaf value {
            type string;
        }
    }
}
`
	writeModuleFile(t, filepath.Join(dir, moduleName+".yang"), []byte(module))

	ctx, err := cambium.NewContext()
	if err != nil {
		t.Fatal(err)
	}
	defer ctx.Close()
	if err := ctx.SetSearchPath(dir); err != nil {
		t.Fatal(err)
	}
	if err := ctx.LoadModule(moduleName); err != nil {
		t.Fatalf("LoadModule: %v", err)
	}
	mod, err := ctx.Schema(moduleName)
	if err != nil {
		t.Fatalf("Schema: %v", err)
	}
	root, err := mod.FindPath("/" + moduleName)
	if err != nil {
		t.Fatalf("FindPath module root: %v", err)
	}

	gotChildren := schemaChildNames(root.Children())
	wantChildren := []string{"first", "do-it", "middle", "alarm", "last"}
	if strings.Join(gotChildren, ",") != strings.Join(wantChildren, ",") {
		t.Fatalf("root Children() = %v, want %v", gotChildren, wantChildren)
	}

	gotTop := schemaChildNames(mod.TopLevel())
	wantTop := []string{"first", "middle", "last"}
	if strings.Join(gotTop, ",") != strings.Join(wantTop, ",") {
		t.Fatalf("TopLevel() = %v, want %v", gotTop, wantTop)
	}

	gotRPCs := schemaChildNames(mod.RPCs())
	wantRPCs := []string{"do-it"}
	if strings.Join(gotRPCs, ",") != strings.Join(wantRPCs, ",") {
		t.Fatalf("RPCs() = %v, want %v", gotRPCs, wantRPCs)
	}

	gotNotifs := schemaChildNames(mod.Notifications())
	wantNotifs := []string{"alarm"}
	if strings.Join(gotNotifs, ",") != strings.Join(wantNotifs, ",") {
		t.Fatalf("Notifications() = %v, want %v", gotNotifs, wantNotifs)
	}
}

func TestSubmoduleChildrenExpandAtIncludeSite(t *testing.T) {
	dir := t.TempDir()

	const moduleName = "cambium-submodule-include-order"
	module := `module cambium-submodule-include-order {
    namespace "urn:cambium:submodule-include-order";
    prefix csio;
    yang-version 1.1;

    include cambium-submodule-include-order-part;

    container first {
        leaf value {
            type string;
        }
    }

    container last {
        leaf value {
            type string;
        }
    }
}
`
	writeModuleFile(t, filepath.Join(dir, moduleName+".yang"), []byte(module))

	submodule := `submodule cambium-submodule-include-order-part {
    belongs-to cambium-submodule-include-order {
        prefix csio;
    }
    yang-version 1.1;

    container middle {
        leaf value {
            type string;
        }
    }

    rpc do-it {
        input {
            leaf request {
                type string;
            }
        }
    }

    notification alarm {
        leaf severity {
            type string;
        }
    }
}
`
	writeModuleFile(t, filepath.Join(dir, "cambium-submodule-include-order-part.yang"), []byte(submodule))

	ctx, err := cambium.NewContext()
	if err != nil {
		t.Fatal(err)
	}
	defer ctx.Close()
	if err := ctx.SetSearchPath(dir); err != nil {
		t.Fatal(err)
	}
	if err := ctx.LoadModule(moduleName); err != nil {
		t.Fatalf("LoadModule: %v", err)
	}
	mod, err := ctx.Schema(moduleName)
	if err != nil {
		t.Fatalf("Schema: %v", err)
	}
	root, err := mod.FindPath("/" + moduleName)
	if err != nil {
		t.Fatalf("FindPath module root: %v", err)
	}

	gotChildren := schemaChildNames(root.Children())
	wantChildren := []string{"middle", "do-it", "alarm", "first", "last"}
	if strings.Join(gotChildren, ",") != strings.Join(wantChildren, ",") {
		t.Fatalf("root Children() = %v, want %v", gotChildren, wantChildren)
	}
	gotTop := schemaChildNames(mod.TopLevel())
	wantTop := []string{"middle", "first", "last"}
	if strings.Join(gotTop, ",") != strings.Join(wantTop, ",") {
		t.Fatalf("TopLevel() = %v, want %v", gotTop, wantTop)
	}
	if gotRPCs := schemaChildNames(mod.RPCs()); strings.Join(gotRPCs, ",") != "do-it" {
		t.Fatalf("RPCs() = %v, want [do-it]", gotRPCs)
	}
	if gotNotifs := schemaChildNames(mod.Notifications()); strings.Join(gotNotifs, ",") != "alarm" {
		t.Fatalf("Notifications() = %v, want [alarm]", gotNotifs)
	}
}

func TestNestedSubmoduleIncludesExpandInOrder(t *testing.T) {
	dir := t.TempDir()

	const moduleName = "cambium-nested-submodule-include"
	module := `module cambium-nested-submodule-include {
    namespace "urn:cambium:nested-submodule-include";
    prefix cnsi;

    include cambium-nested-submodule-include-a;

    container first {
        leaf value {
            type string;
        }
    }

    container last {
        leaf value {
            type string;
        }
    }
}
`
	writeModuleFile(t, filepath.Join(dir, moduleName+".yang"), []byte(module))

	partA := `submodule cambium-nested-submodule-include-a {
    belongs-to cambium-nested-submodule-include {
        prefix cnsi;
    }

    include cambium-nested-submodule-include-b;

    container from-a {
        leaf value {
            type string;
        }
    }
}
`
	writeModuleFile(t, filepath.Join(dir, "cambium-nested-submodule-include-a.yang"), []byte(partA))

	partB := `submodule cambium-nested-submodule-include-b {
    belongs-to cambium-nested-submodule-include {
        prefix cnsi;
    }

    container from-b {
        leaf value {
            type string;
        }
    }
}
`
	writeModuleFile(t, filepath.Join(dir, "cambium-nested-submodule-include-b.yang"), []byte(partB))

	ctx, err := cambium.NewContext()
	if err != nil {
		t.Fatal(err)
	}
	defer ctx.Close()
	if err := ctx.SetSearchPath(dir); err != nil {
		t.Fatal(err)
	}
	if err := ctx.LoadModule(moduleName); err != nil {
		t.Fatalf("LoadModule: %v", err)
	}
	mod, err := ctx.Schema(moduleName)
	if err != nil {
		t.Fatalf("Schema: %v", err)
	}

	gotTop := schemaChildNames(mod.TopLevel())
	wantTop := []string{"from-b", "from-a", "first", "last"}
	if strings.Join(gotTop, ",") != strings.Join(wantTop, ",") {
		t.Fatalf("TopLevel() = %v, want %v", gotTop, wantTop)
	}
	if _, err := mod.FindPath("/cnsi:from-b"); err != nil {
		t.Fatalf("FindPath nested submodule child: %v", err)
	}
}

func TestYang11NestedSubmoduleIncludeMustBeInParent(t *testing.T) {
	dir := t.TempDir()

	const moduleName = "cambium-yang11-submodule-nested-include"
	module := `module cambium-yang11-submodule-nested-include {
    yang-version 1.1;
    namespace "urn:cambium:yang11-submodule-nested-include";
    prefix cysni;

    include cambium-yang11-submodule-nested-include-a;
}
`
	writeModuleFile(t, filepath.Join(dir, moduleName+".yang"), []byte(module))

	partA := `submodule cambium-yang11-submodule-nested-include-a {
    yang-version 1.1;
    belongs-to cambium-yang11-submodule-nested-include {
        prefix cysni;
    }

    include cambium-yang11-submodule-nested-include-b;

    container from-a {
        leaf value {
            type string;
        }
    }
}
`
	writeModuleFile(t, filepath.Join(dir, "cambium-yang11-submodule-nested-include-a.yang"), []byte(partA))

	partB := `submodule cambium-yang11-submodule-nested-include-b {
    yang-version 1.1;
    belongs-to cambium-yang11-submodule-nested-include {
        prefix cysni;
    }

    container from-b {
        leaf value {
            type string;
        }
    }
}
`
	writeModuleFile(t, filepath.Join(dir, "cambium-yang11-submodule-nested-include-b.yang"), []byte(partB))

	ctx, err := cambium.NewContext()
	if err != nil {
		t.Fatal(err)
	}
	defer ctx.Close()
	if err := ctx.SetSearchPath(dir); err != nil {
		t.Fatal(err)
	}
	err = ctx.LoadModule(moduleName)
	if err == nil {
		t.Fatal("LoadModule accepted YANG 1.1 nested submodule include missing from parent")
	}
	var ce *cambium.Error
	if !errors.As(err, &ce) || ce.RuleCode() != cambium.RuleCodeContext {
		t.Fatalf("LoadModule error = %v, want RuleCodeContext", err)
	}
	if !strings.Contains(err.Error(), `YANG 1.1 submodule "cambium-yang11-submodule-nested-include-a" includes "cambium-yang11-submodule-nested-include-b" but parent module "cambium-yang11-submodule-nested-include" does not include it`) {
		t.Fatalf("LoadModule error = %q, want nested include message", err.Error())
	}
}

func TestYang11NestedSubmoduleIncludeRevisionMustMatchParent(t *testing.T) {
	dir := t.TempDir()

	const moduleName = "cambium-yang11-submodule-nested-revision"
	module := `module cambium-yang11-submodule-nested-revision {
    yang-version 1.1;
    namespace "urn:cambium:yang11-submodule-nested-revision";
    prefix cysnr;

    include cambium-yang11-submodule-nested-revision-a;
    include cambium-yang11-submodule-nested-revision-b;
}
`
	writeModuleFile(t, filepath.Join(dir, moduleName+".yang"), []byte(module))

	partA := `submodule cambium-yang11-submodule-nested-revision-a {
    yang-version 1.1;
    belongs-to cambium-yang11-submodule-nested-revision {
        prefix cysnr;
    }

    include cambium-yang11-submodule-nested-revision-b {
        revision-date 2024-01-01;
    }

    container from-a {
        leaf value {
            type string;
        }
    }
}
`
	writeModuleFile(t, filepath.Join(dir, "cambium-yang11-submodule-nested-revision-a.yang"), []byte(partA))

	partB := `submodule cambium-yang11-submodule-nested-revision-b {
    yang-version 1.1;
    belongs-to cambium-yang11-submodule-nested-revision {
        prefix cysnr;
    }
    revision 2024-01-01;

    container from-b {
        leaf value {
            type string;
        }
    }
}
`
	writeModuleFile(t, filepath.Join(dir, "cambium-yang11-submodule-nested-revision-b@2024-01-01.yang"), []byte(partB))

	ctx, err := cambium.NewContext()
	if err != nil {
		t.Fatal(err)
	}
	defer ctx.Close()
	if err := ctx.SetSearchPath(dir); err != nil {
		t.Fatal(err)
	}
	err = ctx.LoadModule(moduleName)
	if err == nil {
		t.Fatal("LoadModule accepted YANG 1.1 nested submodule include revision not pinned by parent")
	}
	var ce *cambium.Error
	if !errors.As(err, &ce) || ce.RuleCode() != cambium.RuleCodeContext {
		t.Fatalf("LoadModule error = %v, want RuleCodeContext", err)
	}
	if !strings.Contains(err.Error(), `YANG 1.1 submodule "cambium-yang11-submodule-nested-revision-a" includes "cambium-yang11-submodule-nested-revision-b" revision "2024-01-01" but parent module "cambium-yang11-submodule-nested-revision" does not include the same revision`) {
		t.Fatalf("LoadModule error = %q, want nested include revision message", err.Error())
	}
}

func TestYang11SubmoduleCanReferenceSiblingDefinitions(t *testing.T) {
	dir := t.TempDir()

	const moduleName = "cambium-submodule-sibling-yang11"
	module := `module cambium-submodule-sibling-yang11 {
    yang-version 1.1;
    namespace "urn:cambium:submodule-sibling-yang11";
    prefix cssy;

    include cambium-submodule-sibling-yang11-a;
    include cambium-submodule-sibling-yang11-b;
}
`
	writeModuleFile(t, filepath.Join(dir, moduleName+".yang"), []byte(module))

	partA := `submodule cambium-submodule-sibling-yang11-a {
    yang-version 1.1;
    belongs-to cambium-submodule-sibling-yang11 {
        prefix cssy;
    }

    container top {
        uses shared-fields;
        leaf typed {
            type shared-type;
        }
    }
}
`
	writeModuleFile(t, filepath.Join(dir, "cambium-submodule-sibling-yang11-a.yang"), []byte(partA))

	partB := `submodule cambium-submodule-sibling-yang11-b {
    yang-version 1.1;
    belongs-to cambium-submodule-sibling-yang11 {
        prefix cssy;
    }

    typedef shared-type {
        type string;
    }

    grouping shared-fields {
        leaf shared {
            type string;
        }
    }
}
`
	writeModuleFile(t, filepath.Join(dir, "cambium-submodule-sibling-yang11-b.yang"), []byte(partB))

	ctx, err := cambium.NewContext()
	if err != nil {
		t.Fatal(err)
	}
	defer ctx.Close()
	if err := ctx.SetSearchPath(dir); err != nil {
		t.Fatal(err)
	}
	if err := ctx.LoadModule(moduleName); err != nil {
		t.Fatalf("LoadModule: %v", err)
	}
	mod, err := ctx.Schema(moduleName)
	if err != nil {
		t.Fatalf("Schema: %v", err)
	}
	top, err := mod.FindPath("/cssy:top")
	if err != nil {
		t.Fatalf("FindPath top: %v", err)
	}
	if got := schemaChildNames(top.Children()); strings.Join(got, ",") != "shared,typed" {
		t.Fatalf("top children = %v, want [shared typed]", got)
	}
	typed := childByName(t, top.Children(), "typed")
	info, ok := typed.LeafType()
	if !ok {
		t.Fatal("typed leaf has no type")
	}
	if name, ok := info.TypedefName(); !ok || name != "shared-type" {
		t.Fatalf("typed typedef = (%q,%v), want (shared-type,true)", name, ok)
	}
}

func TestYang10SubmoduleRequiresIncludeForSiblingDefinitions(t *testing.T) {
	dir := t.TempDir()

	const moduleName = "cambium-submodule-sibling-yang10"
	module := `module cambium-submodule-sibling-yang10 {
    namespace "urn:cambium:submodule-sibling-yang10";
    prefix cssyt;

    include cambium-submodule-sibling-yang10-a;
    include cambium-submodule-sibling-yang10-b;
}
`
	writeModuleFile(t, filepath.Join(dir, moduleName+".yang"), []byte(module))

	partA := `submodule cambium-submodule-sibling-yang10-a {
    belongs-to cambium-submodule-sibling-yang10 {
        prefix cssyt;
    }

    container top {
        uses shared-fields;
    }
}
`
	writeModuleFile(t, filepath.Join(dir, "cambium-submodule-sibling-yang10-a.yang"), []byte(partA))

	partB := `submodule cambium-submodule-sibling-yang10-b {
    belongs-to cambium-submodule-sibling-yang10 {
        prefix cssyt;
    }

    grouping shared-fields {
        leaf shared {
            type string;
        }
    }
}
`
	writeModuleFile(t, filepath.Join(dir, "cambium-submodule-sibling-yang10-b.yang"), []byte(partB))

	ctx, err := cambium.NewContext()
	if err != nil {
		t.Fatal(err)
	}
	defer ctx.Close()
	if err := ctx.SetSearchPath(dir); err != nil {
		t.Fatal(err)
	}
	if err := ctx.LoadModule(moduleName); err != nil {
		t.Fatalf("LoadModule: %v", err)
	}
	_, err = ctx.Schema(moduleName)
	if err == nil {
		t.Fatal("LoadModule accepted YANG 1.0 sibling submodule grouping reference without include")
	}
	if !strings.Contains(err.Error(), `unknown grouping "shared-fields"`) {
		t.Fatalf("Schema error = %q, want unknown grouping", err.Error())
	}
}

func TestYang10SubmoduleRequiresIncludeForSiblingTypedefs(t *testing.T) {
	dir := t.TempDir()

	const moduleName = "cambium-submodule-sibling-typedef-yang10"
	module := `module cambium-submodule-sibling-typedef-yang10 {
    namespace "urn:cambium:submodule-sibling-typedef-yang10";
    prefix cssytt;

    include cambium-submodule-sibling-typedef-yang10-a;
    include cambium-submodule-sibling-typedef-yang10-b;
}
`
	writeModuleFile(t, filepath.Join(dir, moduleName+".yang"), []byte(module))

	partA := `submodule cambium-submodule-sibling-typedef-yang10-a {
    belongs-to cambium-submodule-sibling-typedef-yang10 {
        prefix cssytt;
    }

    leaf typed {
        type shared-type;
    }
}
`
	writeModuleFile(t, filepath.Join(dir, "cambium-submodule-sibling-typedef-yang10-a.yang"), []byte(partA))

	partB := `submodule cambium-submodule-sibling-typedef-yang10-b {
    belongs-to cambium-submodule-sibling-typedef-yang10 {
        prefix cssytt;
    }

    typedef shared-type {
        type string;
    }
}
`
	writeModuleFile(t, filepath.Join(dir, "cambium-submodule-sibling-typedef-yang10-b.yang"), []byte(partB))

	ctx, err := cambium.NewContext()
	if err != nil {
		t.Fatal(err)
	}
	defer ctx.Close()
	if err := ctx.SetSearchPath(dir); err != nil {
		t.Fatal(err)
	}
	if err := ctx.LoadModule(moduleName); err != nil {
		t.Fatalf("LoadModule: %v", err)
	}
	_, err = ctx.Schema(moduleName)
	if err == nil {
		t.Fatal("LoadModule accepted YANG 1.0 sibling submodule typedef reference without include")
	}
	if !strings.Contains(err.Error(), `unknown type "shared-type"`) {
		t.Fatalf("Schema error = %q, want unknown type", err.Error())
	}
}

func TestYang10SubmoduleIncludeMakesSiblingDefinitionsVisible(t *testing.T) {
	dir := t.TempDir()

	const moduleName = "cambium-submodule-sibling-include-yang10"
	module := `module cambium-submodule-sibling-include-yang10 {
    namespace "urn:cambium:submodule-sibling-include-yang10";
    prefix cssiyt;

    include cambium-submodule-sibling-include-yang10-a;
}
`
	writeModuleFile(t, filepath.Join(dir, moduleName+".yang"), []byte(module))

	partA := `submodule cambium-submodule-sibling-include-yang10-a {
    belongs-to cambium-submodule-sibling-include-yang10 {
        prefix cssiyt;
    }

    include cambium-submodule-sibling-include-yang10-b;

    container top {
        uses shared-fields;
        leaf typed {
            type shared-type;
        }
    }
}
`
	writeModuleFile(t, filepath.Join(dir, "cambium-submodule-sibling-include-yang10-a.yang"), []byte(partA))

	partB := `submodule cambium-submodule-sibling-include-yang10-b {
    belongs-to cambium-submodule-sibling-include-yang10 {
        prefix cssiyt;
    }

    typedef shared-type {
        type string;
    }

    grouping shared-fields {
        leaf shared {
            type string;
        }
    }
}
`
	writeModuleFile(t, filepath.Join(dir, "cambium-submodule-sibling-include-yang10-b.yang"), []byte(partB))

	ctx, err := cambium.NewContext()
	if err != nil {
		t.Fatal(err)
	}
	defer ctx.Close()
	if err := ctx.SetSearchPath(dir); err != nil {
		t.Fatal(err)
	}
	if err := ctx.LoadModule(moduleName); err != nil {
		t.Fatalf("LoadModule: %v", err)
	}
	mod, err := ctx.Schema(moduleName)
	if err != nil {
		t.Fatalf("Schema: %v", err)
	}
	top, err := mod.FindPath("/cssiyt:top")
	if err != nil {
		t.Fatalf("FindPath top: %v", err)
	}
	if got := schemaChildNames(top.Children()); strings.Join(got, ",") != "shared,typed" {
		t.Fatalf("top children = %v, want [shared typed]", got)
	}
}

func TestYang10SubmoduleRequiresIncludeForSiblingDefinitionReferences(t *testing.T) {
	cases := []struct {
		name      string
		partABody string
		partBBody string
		wantErr   string
	}{
		{
			name: "identity",
			partABody: `    leaf kind {
        type identityref {
            base sibling-base;
        }
    }
`,
			partBBody: `    identity sibling-base;
`,
			wantErr: `unknown identity base "sibling-base"`,
		},
		{
			name: "feature",
			partABody: `    container gated {
        if-feature sibling-feature;
        leaf value {
            type string;
        }
    }
`,
			partBBody: `    feature sibling-feature;
`,
			wantErr: `invalid or unresolved if-feature expression "sibling-feature"`,
		},
		{
			name: "extension",
			partABody: `    container marked {
        cssd:mark "x";
        leaf value {
            type string;
        }
    }
`,
			partBBody: `    extension mark {
        argument value;
    }
`,
			wantErr: `unknown extension "cssd:mark"`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			moduleName := "cambium-submodule-sibling-def-" + tc.name + "-yang10"
			module := fmt.Sprintf(`module %s {
    namespace "urn:%s";
    prefix cssd;

    include %s-a;
    include %s-b;
}
`, moduleName, moduleName, moduleName, moduleName)
			writeModuleFile(t, filepath.Join(dir, moduleName+".yang"), []byte(module))

			partA := fmt.Sprintf(`submodule %s-a {
    belongs-to %s {
        prefix cssd;
    }

%s}
`, moduleName, moduleName, tc.partABody)
			writeModuleFile(t, filepath.Join(dir, moduleName+"-a.yang"), []byte(partA))

			partB := fmt.Sprintf(`submodule %s-b {
    belongs-to %s {
        prefix cssd;
    }

%s}
`, moduleName, moduleName, tc.partBBody)
			writeModuleFile(t, filepath.Join(dir, moduleName+"-b.yang"), []byte(partB))

			ctx, err := cambium.NewContext()
			if err != nil {
				t.Fatal(err)
			}
			defer ctx.Close()
			if err := ctx.SetSearchPath(dir); err != nil {
				t.Fatal(err)
			}
			if err := ctx.LoadModule(moduleName); err != nil {
				t.Fatalf("LoadModule: %v", err)
			}
			_, err = ctx.Schema(moduleName)
			if err == nil {
				t.Fatalf("Schema accepted YANG 1.0 sibling submodule %s reference without include", tc.name)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("Schema error = %q, want to contain %q", err.Error(), tc.wantErr)
			}
		})
	}
}

func TestYang11SubmoduleCanReferenceSiblingDefinitionReferences(t *testing.T) {
	dir := t.TempDir()

	const moduleName = "cambium-submodule-sibling-def-yang11"
	module := `module cambium-submodule-sibling-def-yang11 {
    yang-version 1.1;
    namespace "urn:cambium:submodule-sibling-def-yang11";
    prefix cssdy;

    include cambium-submodule-sibling-def-yang11-a;
    include cambium-submodule-sibling-def-yang11-b;
}
`
	writeModuleFile(t, filepath.Join(dir, moduleName+".yang"), []byte(module))

	partA := `submodule cambium-submodule-sibling-def-yang11-a {
    yang-version 1.1;
    belongs-to cambium-submodule-sibling-def-yang11 {
        prefix cssdy;
    }

    leaf kind {
        type identityref {
            base sibling-base;
        }
    }

    container gated {
        if-feature sibling-feature;
        leaf value {
            type string;
        }
    }

    container marked {
        cssdy:mark "x";
        leaf value {
            type string;
        }
    }
}
`
	writeModuleFile(t, filepath.Join(dir, "cambium-submodule-sibling-def-yang11-a.yang"), []byte(partA))

	partB := `submodule cambium-submodule-sibling-def-yang11-b {
    yang-version 1.1;
    belongs-to cambium-submodule-sibling-def-yang11 {
        prefix cssdy;
    }

    identity sibling-base;
    feature sibling-feature;
    extension mark {
        argument value;
    }
}
`
	writeModuleFile(t, filepath.Join(dir, "cambium-submodule-sibling-def-yang11-b.yang"), []byte(partB))

	ctx, err := cambium.NewContext()
	if err != nil {
		t.Fatal(err)
	}
	defer ctx.Close()
	if err := ctx.SetSearchPath(dir); err != nil {
		t.Fatal(err)
	}
	if err := ctx.LoadModule(moduleName); err != nil {
		t.Fatalf("LoadModule: %v", err)
	}
	mod, err := ctx.Schema(moduleName)
	if err != nil {
		t.Fatalf("Schema: %v", err)
	}
	marked, err := mod.FindPath("/cssdy:marked")
	if err != nil {
		t.Fatalf("FindPath marked: %v", err)
	}
	ext, ok := marked.Extension("mark")
	if !ok {
		t.Fatalf("marked extensions = %v, want mark", marked.Extensions())
	}
	if arg, ok := ext.Argument(); !ok || arg != "x" {
		t.Fatalf("extension argument = (%q,%v), want (x,true)", arg, ok)
	}
}

func importContext(t *testing.T) (ctx *cambium.Context, cleanup func()) {
	t.Helper()
	dir := schemaIntrospectionModuleDir(t)

	target := `module cambium-import-target {
    namespace "urn:cambium:target";
    prefix cit;
    yang-version 1.1;
    revision 2026-06-14;

    identity target-base;
}
`
	writeModuleFile(t, filepath.Join(dir, "cambium-import-target.yang"), []byte(target))

	user := `module cambium-import-user {
    namespace "urn:cambium:user";
    prefix ciu;
    yang-version 1.1;

    import cambium-import-target {
        prefix tgt;
        revision-date 2026-06-14;
        description "Target import.";
        reference "target-import-ref";
    }

    revision 2026-06-14;

    identity user-derived {
        base tgt:target-base;
    }

    leaf dummy {
        type string;
    }

    leaf cross-idref {
        type identityref {
            base tgt:target-base;
        }
    }
}
`
	writeModuleFile(t, filepath.Join(dir, "cambium-import-user.yang"), []byte(user))

	ctx, err := cambium.NewContext()
	if err != nil {
		t.Fatal(err)
	}
	if err := ctx.SetSearchPath(dir); err != nil {
		t.Fatal(err)
	}
	if err := ctx.LoadModule("cambium-import-user"); err != nil {
		t.Fatalf("LoadModule: %v", err)
	}
	return ctx, func() { ctx.Close() }
}

func TestModuleImportsAndResolvePrefix(t *testing.T) {
	assertModuleImportsAndResolvePrefix(t)
}

func TestModuleImportsResolvePrefix(t *testing.T) {
	// Spec-named alias for TestModuleImportsAndResolvePrefix.
	assertModuleImportsAndResolvePrefix(t)
}

func TestImportRevisionDatePinsLoadedModule(t *testing.T) {
	t.Helper()
	dir := schemaIntrospectionModuleDir(t)

	oldTarget := `module cambium-import-revision-target {
    namespace "urn:cambium:import-revision-target";
    prefix cirt;
    yang-version 1.1;
    revision 2024-01-01;

    typedef pinned-type {
        type string;
    }
}
`
	writeModuleFile(t, filepath.Join(dir, "cambium-import-revision-target@2024-01-01.yang"), []byte(oldTarget))

	newTarget := `module cambium-import-revision-target {
    namespace "urn:cambium:import-revision-target";
    prefix cirt;
    yang-version 1.1;
    revision 2025-01-01;

    typedef newer-type {
        type uint32;
    }
}
`
	writeModuleFile(t, filepath.Join(dir, "cambium-import-revision-target@2025-01-01.yang"), []byte(newTarget))

	user := `module cambium-import-revision-user {
    namespace "urn:cambium:import-revision-user";
    prefix ciru;
    yang-version 1.1;

    import cambium-import-revision-target {
        prefix target;
        revision-date 2024-01-01;
    }

    leaf pinned {
        type target:pinned-type;
    }
}
`
	writeModuleFile(t, filepath.Join(dir, "cambium-import-revision-user.yang"), []byte(user))

	ctx, err := cambium.NewContext()
	if err != nil {
		t.Fatal(err)
	}
	defer ctx.Close()
	if err := ctx.SetSearchPath(dir); err != nil {
		t.Fatal(err)
	}
	if err := ctx.LoadModule("cambium-import-revision-user"); err != nil {
		t.Fatalf("LoadModule: %v", err)
	}

	userMod, err := ctx.Schema("cambium-import-revision-user")
	if err != nil {
		t.Fatalf("Schema user: %v", err)
	}
	targetMod, ok := userMod.ResolvePrefix("target")
	if !ok {
		t.Fatal("ResolvePrefix(target) returned false")
	}
	if got, ok := targetMod.Revision(); !ok || got != "2024-01-01" {
		t.Fatalf("target revision = (%q,%v), want 2024-01-01,true", got, ok)
	}
	leaf, err := userMod.FindPath("/ciru:pinned")
	if err != nil {
		t.Fatalf("FindPath pinned: %v", err)
	}
	info, ok := leaf.LeafType()
	if !ok {
		t.Fatal("pinned leaf should have type info")
	}
	if got := info.Base(); got != cambium.BaseTypeString {
		t.Fatalf("pinned base = %v, want BaseTypeString from pinned old revision", got)
	}
	if got, ok := info.TypedefName(); !ok || got != "pinned-type" {
		t.Fatalf("typedef = (%q,%v), want pinned-type,true", got, ok)
	}
}

func TestImportsCanUseDifferentRevisionsOfSameModule(t *testing.T) {
	t.Helper()
	dir := schemaIntrospectionModuleDir(t)

	oldTarget := `module cambium-multi-revision-target {
    namespace "urn:cambium:multi-revision-target";
    prefix cmrt;
    yang-version 1.1;
    revision 2024-01-01;

    typedef old-type {
        type string;
    }
}
`
	writeModuleFile(t, filepath.Join(dir, "cambium-multi-revision-target@2024-01-01.yang"), []byte(oldTarget))

	newTarget := `module cambium-multi-revision-target {
    namespace "urn:cambium:multi-revision-target";
    prefix cmrt;
    yang-version 1.1;
    revision 2025-01-01;

    typedef new-type {
        type uint32;
    }
}
`
	writeModuleFile(t, filepath.Join(dir, "cambium-multi-revision-target@2025-01-01.yang"), []byte(newTarget))

	oldUser := `module cambium-multi-revision-old-user {
    namespace "urn:cambium:multi-revision-old-user";
    prefix cmrou;
    yang-version 1.1;

    import cambium-multi-revision-target {
        prefix target;
        revision-date 2024-01-01;
    }

    leaf old-leaf {
        type target:old-type;
    }
}
`
	writeModuleFile(t, filepath.Join(dir, "cambium-multi-revision-old-user.yang"), []byte(oldUser))

	newUser := `module cambium-multi-revision-new-user {
    namespace "urn:cambium:multi-revision-new-user";
    prefix cmrnu;
    yang-version 1.1;

    import cambium-multi-revision-target {
        prefix target;
        revision-date 2025-01-01;
    }

    leaf new-leaf {
        type target:new-type;
    }
}
`
	writeModuleFile(t, filepath.Join(dir, "cambium-multi-revision-new-user.yang"), []byte(newUser))

	ctx, err := cambium.NewContext()
	if err != nil {
		t.Fatal(err)
	}
	defer ctx.Close()
	if err := ctx.SetSearchPath(dir); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"cambium-multi-revision-old-user", "cambium-multi-revision-new-user"} {
		if err := ctx.LoadModule(name); err != nil {
			t.Fatalf("LoadModule(%s): %v", name, err)
		}
	}

	oldMod, err := ctx.Schema("cambium-multi-revision-old-user")
	if err != nil {
		t.Fatalf("Schema old user: %v", err)
	}
	oldTargetMod, ok := oldMod.ResolvePrefix("target")
	if !ok {
		t.Fatal("old user ResolvePrefix(target) returned false")
	}
	if got, ok := oldTargetMod.Revision(); !ok || got != "2024-01-01" {
		t.Fatalf("old user target revision = (%q,%v), want 2024-01-01,true", got, ok)
	}
	oldLeaf, err := oldMod.FindPath("/cmrou:old-leaf")
	if err != nil {
		t.Fatalf("FindPath old-leaf: %v", err)
	}
	oldInfo, ok := oldLeaf.LeafType()
	if !ok {
		t.Fatal("old-leaf should have type info")
	}
	if got := oldInfo.Base(); got != cambium.BaseTypeString {
		t.Fatalf("old-leaf base = %v, want BaseTypeString", got)
	}
	if got, ok := oldInfo.TypedefName(); !ok || got != "old-type" {
		t.Fatalf("old-leaf typedef = (%q,%v), want old-type,true", got, ok)
	}

	newMod, err := ctx.Schema("cambium-multi-revision-new-user")
	if err != nil {
		t.Fatalf("Schema new user: %v", err)
	}
	newTargetMod, ok := newMod.ResolvePrefix("target")
	if !ok {
		t.Fatal("new user ResolvePrefix(target) returned false")
	}
	if got, ok := newTargetMod.Revision(); !ok || got != "2025-01-01" {
		t.Fatalf("new user target revision = (%q,%v), want 2025-01-01,true", got, ok)
	}
	newLeaf, err := newMod.FindPath("/cmrnu:new-leaf")
	if err != nil {
		t.Fatalf("FindPath new-leaf: %v", err)
	}
	newInfo, ok := newLeaf.LeafType()
	if !ok {
		t.Fatal("new-leaf should have type info")
	}
	if got := newInfo.Base(); got != cambium.BaseTypeUint32 {
		t.Fatalf("new-leaf base = %v, want BaseTypeUint32", got)
	}
	if got, ok := newInfo.TypedefName(); !ok || got != "new-type" {
		t.Fatalf("new-leaf typedef = (%q,%v), want new-type,true", got, ok)
	}
}

func TestIncludeRevisionAttachesToExactParentRevision(t *testing.T) {
	t.Helper()
	dir := schemaIntrospectionModuleDir(t)

	oldParent := `module cambium-include-revision-parent {
    namespace "urn:cambium:include-revision-parent";
    prefix cirp;
    yang-version 1.1;

    include cambium-include-revision-part {
        revision-date 2024-01-01;
    }

    revision 2024-01-01;
}
`
	writeModuleFile(t, filepath.Join(dir, "cambium-include-revision-parent@2024-01-01.yang"), []byte(oldParent))

	newParent := `module cambium-include-revision-parent {
    namespace "urn:cambium:include-revision-parent";
    prefix cirp;
    yang-version 1.1;

    include cambium-include-revision-part {
        revision-date 2025-01-01;
    }

    revision 2025-01-01;
}
`
	writeModuleFile(t, filepath.Join(dir, "cambium-include-revision-parent@2025-01-01.yang"), []byte(newParent))

	oldSubmodule := `submodule cambium-include-revision-part {
    yang-version 1.1;
    belongs-to cambium-include-revision-parent {
        prefix cirp;
    }
    revision 2024-01-01;

    leaf from-old-submodule {
        type string;
    }
}
`
	writeModuleFile(t, filepath.Join(dir, "cambium-include-revision-part@2024-01-01.yang"), []byte(oldSubmodule))

	newSubmodule := `submodule cambium-include-revision-part {
    yang-version 1.1;
    belongs-to cambium-include-revision-parent {
        prefix cirp;
    }
    revision 2025-01-01;

    leaf from-new-submodule {
        type uint32;
    }
}
`
	writeModuleFile(t, filepath.Join(dir, "cambium-include-revision-part@2025-01-01.yang"), []byte(newSubmodule))

	builder, err := cambium.NewContextBuilder(cambium.ContextFlags{})
	if err != nil {
		t.Fatal(err)
	}
	if err := builder.SearchPath(dir); err != nil {
		t.Fatal(err)
	}
	newRevision := "2025-01-01"
	oldRevision := "2024-01-01"
	if err := builder.LoadModule("cambium-include-revision-parent", &newRevision, nil); err != nil {
		t.Fatalf("LoadModule new revision: %v", err)
	}
	if err := builder.LoadModule("cambium-include-revision-parent", &oldRevision, nil); err != nil {
		t.Fatalf("LoadModule old revision: %v", err)
	}
	ctx, err := builder.Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	defer ctx.Close()

	oldMod, err := ctx.SchemaRevision("cambium-include-revision-parent", oldRevision)
	if err != nil {
		t.Fatalf("SchemaRevision old parent: %v", err)
	}
	newMod, err := ctx.SchemaRevision("cambium-include-revision-parent", newRevision)
	if err != nil {
		t.Fatalf("SchemaRevision new parent: %v", err)
	}
	defaultMod, err := ctx.Schema("cambium-include-revision-parent")
	if err != nil {
		t.Fatalf("Schema default parent: %v", err)
	}
	if got, ok := defaultMod.Revision(); !ok || got != newRevision {
		t.Fatalf("Schema default revision = (%q,%v), want %s,true", got, ok, newRevision)
	}
	defaultByNamespace, err := ctx.Schema("urn:cambium:include-revision-parent")
	if err != nil {
		t.Fatalf("Schema default parent by namespace: %v", err)
	}
	if got, ok := defaultByNamespace.Revision(); !ok || got != newRevision {
		t.Fatalf("Schema namespace default revision = (%q,%v), want %s,true", got, ok, newRevision)
	}
	oldByNamespace, err := ctx.SchemaRevision("urn:cambium:include-revision-parent", oldRevision)
	if err != nil {
		t.Fatalf("SchemaRevision old parent by namespace: %v", err)
	}
	if got, ok := oldByNamespace.Revision(); !ok || got != oldRevision {
		t.Fatalf("SchemaRevision namespace old revision = (%q,%v), want %s,true", got, ok, oldRevision)
	}

	if _, ok := oldMod.TopLevel().Lookup("from-old-submodule"); !ok {
		t.Fatal("old parent should include old submodule leaf")
	}
	if _, ok := oldMod.TopLevel().Lookup("from-new-submodule"); ok {
		t.Fatal("old parent should not include new submodule leaf")
	}
	if _, ok := newMod.TopLevel().Lookup("from-new-submodule"); !ok {
		t.Fatal("new parent should include new submodule leaf")
	}
	if _, ok := newMod.TopLevel().Lookup("from-old-submodule"); ok {
		t.Fatal("new parent should not include old submodule leaf")
	}
}

func assertModuleImportsAndResolvePrefix(t *testing.T) {
	t.Helper()
	ctx, cleanup := importContext(t)
	defer cleanup()

	mod, err := ctx.Schema("cambium-import-user")
	if err != nil {
		t.Fatalf("Schema: %v", err)
	}

	imports := mod.Imports()
	if len(imports) != 1 {
		t.Fatalf("imports = %d, want 1", len(imports))
	}
	imp := imports[0]
	if imp.Prefix != "tgt" {
		t.Fatalf("import prefix = %q, want tgt", imp.Prefix)
	}
	if imp.Name != "cambium-import-target" {
		t.Fatalf("import name = %q, want cambium-import-target", imp.Name)
	}
	if imp.Revision != "2026-06-14" {
		t.Fatalf("import revision = %q, want 2026-06-14", imp.Revision)
	}
	if got, ok := imp.Description(); !ok || got != "Target import." {
		t.Fatalf("import description = (%q,%v), want Target import.,true", got, ok)
	}
	if got, ok := imp.Reference(); !ok || got != "target-import-ref" {
		t.Fatalf("import reference = (%q,%v), want target-import-ref,true", got, ok)
	}

	self, ok := mod.ResolvePrefix("")
	if !ok || self.Name() != "cambium-import-user" {
		t.Fatalf("ResolvePrefix(\"\") = (%q, %v), want (cambium-import-user, true)", self.Name(), ok)
	}

	ownPrefix, ok := mod.ResolvePrefix("ciu")
	if !ok {
		t.Fatal("ResolvePrefix(ciu) returned false, want self module")
	}
	if ownPrefix.Name() != "cambium-import-user" {
		t.Fatalf("ResolvePrefix(ciu).Name() = %q, want cambium-import-user", ownPrefix.Name())
	}

	target, ok := mod.ResolvePrefix("tgt")
	if !ok {
		t.Fatal("ResolvePrefix(tgt) returned false, want target module")
	}
	if target.Name() != "cambium-import-target" {
		t.Fatalf("ResolvePrefix(tgt).Name() = %q, want cambium-import-target", target.Name())
	}
	if target.IsImplemented() {
		t.Fatal("import-only target module should not be implemented")
	}
	if got, want := target.Namespace(), "urn:cambium:target"; got != want {
		t.Fatalf("import-only target namespace = %q, want %q", got, want)
	}

	cross, err := mod.FindPath("/cambium-import-user:cross-idref")
	if err != nil {
		t.Fatalf("FindPath cross-idref: %v", err)
	}
	info, ok := cross.LeafType()
	if !ok {
		t.Fatal("cross-idref should have leaf type")
	}
	idref, ok := info.Resolved().(cambium.ResolvedIdentityRef)
	if !ok {
		t.Fatalf("cross-idref resolved type = %T, want ResolvedIdentityRef", info.Resolved())
	}
	bases := idref.Bases()
	if len(bases) != 1 {
		t.Fatalf("cross-idref bases = %d, want 1", len(bases))
	}
	if got, want := bases[0].Name(), "target-base"; got != want {
		t.Fatalf("cross-idref base name = %q, want %q", got, want)
	}
	if got, want := bases[0].Module().Name(), "cambium-import-target"; got != want {
		t.Fatalf("cross-idref base module = %q, want %q", got, want)
	}

	var userDerived cambium.Identity
	for id := range mod.Identities() {
		if id.Name() == "user-derived" {
			userDerived = id
			break
		}
	}
	if userDerived.Name() == "" {
		t.Fatal("user-derived identity not found")
	}
	userBases := userDerived.Bases()
	if len(userBases) != 1 {
		t.Fatalf("user-derived bases = %d, want 1", len(userBases))
	}
	if got, want := userBases[0].Name(), "target-base"; got != want {
		t.Fatalf("user-derived base name = %q, want %q", got, want)
	}
	if got, want := userBases[0].Module().Name(), "cambium-import-target"; got != want {
		t.Fatalf("user-derived base module = %q, want %q", got, want)
	}

	if _, ok := mod.ResolvePrefix("missing"); ok {
		t.Fatal("ResolvePrefix(missing) should be false")
	}
}

func augmentDeviationContext(t *testing.T) (ctx *cambium.Context, cleanup func()) {
	t.Helper()
	dir := schemaIntrospectionModuleDir(t)

	target := `module cambium-provenance-target {
    namespace "urn:cambium:provtarget";
    prefix cpt;
    yang-version 1.1;
    revision 2026-06-14;

    container top {
        leaf inner {
            type string;
        }
    }

    leaf a {
        type string;
    }
}
`
	writeModuleFile(t, filepath.Join(dir, "cambium-provenance-target.yang"), []byte(target))

	augment := `module cambium-provenance-augment {
    namespace "urn:cambium:provaug";
    prefix cpa;
    yang-version 1.1;

    import cambium-provenance-target {
        prefix cpt;
        revision-date 2026-06-14;
    }

    revision 2026-06-14;

    augment "/cpt:top" {
        leaf b {
            type string;
        }
    }
}
`
	writeModuleFile(t, filepath.Join(dir, "cambium-provenance-augment.yang"), []byte(augment))

	deviate := `module cambium-provenance-deviate {
    namespace "urn:cambium:provdev";
    prefix cpd;
    yang-version 1.1;

    import cambium-provenance-target {
        prefix cpt;
        revision-date 2026-06-14;
    }

    revision 2026-06-14;

    deviation "/cpt:a" {
        deviate replace {
            type uint8;
        }
    }
}
`
	writeModuleFile(t, filepath.Join(dir, "cambium-provenance-deviate.yang"), []byte(deviate))

	ctx, err := cambium.NewContext()
	if err != nil {
		t.Fatal(err)
	}
	if err := ctx.SetSearchPath(dir); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"cambium-provenance-target", "cambium-provenance-augment", "cambium-provenance-deviate"} {
		if err := ctx.LoadModule(name); err != nil {
			t.Fatalf("LoadModule %s: %v", name, err)
		}
	}
	return ctx, func() { ctx.Close() }
}

func TestModuleAugmentDeviationProvenance(t *testing.T) {
	ctx, cleanup := augmentDeviationContext(t)
	defer cleanup()

	target, err := ctx.Schema("cambium-provenance-target")
	if err != nil {
		t.Fatalf("Schema: %v", err)
	}

	aug := target.AugmentedBy()
	if len(aug) != 1 || aug[0] != "cambium-provenance-augment" {
		t.Fatalf("AugmentedBy() = %v, want [cambium-provenance-augment]", aug)
	}

	dev := target.DeviatedBy()
	if len(dev) != 1 || dev[0] != "cambium-provenance-deviate" {
		t.Fatalf("DeviatedBy() = %v, want [cambium-provenance-deviate]", dev)
	}

	top, err := target.FindPath("/cambium-provenance-target:top")
	if err != nil {
		t.Fatalf("FindPath top: %v", err)
	}
	augLeaf := childByName(t, top.Children(), "b")
	if got, want := augLeaf.Module().Name(), "cambium-provenance-augment"; got != want {
		t.Fatalf("augmented leaf Module().Name() = %q, want %q", got, want)
	}
	if got, want := augLeaf.Namespace(), "urn:cambium:provaug"; got != want {
		t.Fatalf("augmented leaf Namespace() = %q, want %q", got, want)
	}
	for _, path := range []string{
		"/cpt:top/cpa:b",
		"/cambium-provenance-target:top/cambium-provenance-augment:b",
	} {
		found, err := target.FindPath(path)
		if err != nil {
			t.Fatalf("FindPath augmented child by owning module qualifier %q: %v", path, err)
		}
		if got, want := found.Module().Name(), "cambium-provenance-augment"; got != want {
			t.Fatalf("FindPath(%q).Module().Name() = %q, want %q", path, got, want)
		}
	}

	// A module that neither augments nor deviates returns empty slices.
	augMod, err := ctx.Schema("cambium-provenance-augment")
	if err != nil {
		t.Fatalf("Schema augment: %v", err)
	}
	if len(augMod.AugmentedBy()) != 0 {
		t.Fatalf("augment module AugmentedBy() = %v, want empty", augMod.AugmentedBy())
	}
	if len(augMod.DeviatedBy()) != 0 {
		t.Fatalf("augment module DeviatedBy() = %v, want empty", augMod.DeviatedBy())
	}
}

const groupingModuleName = "cambium-grouping-demo"

func writeGroupingModule(t *testing.T) (dir, groupingPath string) {
	t.Helper()
	dir = schemaIntrospectionModuleDir(t)

	groupingPath = filepath.Join(dir, groupingModuleName+".yang")
	groupingYang := `module cambium-grouping-demo {
    namespace "urn:cambium:grouping";
    prefix cgd;
    yang-version 1.1;
    revision 2026-06-16;

    grouping common-grouping {
        status deprecated;
        description "Common grouping.";
        reference "common-group-ref";
        leaf grouped-leaf {
            type string;
        }
        container grouped-container {
            leaf nested-leaf {
                type uint32;
            }
        }
    }
    grouping unused-grouping {
        leaf unused-leaf {
            type string;
        }
    }

    container top {
        leaf direct-leaf {
            type string;
        }
        uses common-grouping;
    }
}
`
	writeModuleFile(t, groupingPath, []byte(groupingYang))
	return dir, groupingPath
}

func groupingContext(t *testing.T) (ctx *cambium.Context, cleanup func()) {
	t.Helper()
	_, groupingPath := writeGroupingModule(t)

	ctx, err := cambium.NewContext()
	if err != nil {
		t.Fatal(err)
	}
	if err := ctx.LoadModuleFromPath(groupingPath); err != nil {
		t.Fatalf("LoadModuleFromPath: %v", err)
	}
	return ctx, func() { ctx.Close() }
}

func TestLoadModuleFromPath(t *testing.T) {
	ctx, cleanup := groupingContext(t)
	defer cleanup()

	mod, err := ctx.Schema(groupingModuleName)
	if err != nil {
		t.Fatalf("Schema: %v", err)
	}
	if mod.Name() != groupingModuleName {
		t.Fatalf("module name = %q, want %q", mod.Name(), groupingModuleName)
	}
	if mod.Namespace() != "urn:cambium:grouping" {
		t.Fatalf("module namespace = %q, want urn:cambium:grouping", mod.Namespace())
	}
}

func TestLoadModuleFromPathInvalidatesSchemaForest(t *testing.T) {
	dir, path := writeGroupingModule(t)

	ctx, err := cambium.NewContext()
	if err != nil {
		t.Fatal(err)
	}
	defer ctx.Close()
	if err := ctx.SetSearchPath(dir); err != nil {
		t.Fatal(err)
	}
	if _, err := ctx.Schema(groupingModuleName); err == nil {
		t.Fatal("Schema should fail before loading grouping module")
	}
	if err := ctx.LoadModuleFromPath(path); err != nil {
		t.Fatalf("LoadModuleFromPath: %v", err)
	}
	if _, err := ctx.Schema(groupingModuleName); err != nil {
		t.Fatalf("Schema after LoadModuleFromPath: %v", err)
	}
}

func TestLoadModuleInvalidatesSchemaForest(t *testing.T) {
	dir, _ := writeGroupingModule(t)

	ctx, err := cambium.NewContext()
	if err != nil {
		t.Fatal(err)
	}
	defer ctx.Close()
	if err := ctx.SetSearchPath(dir); err != nil {
		t.Fatal(err)
	}
	if _, err := ctx.Schema(groupingModuleName); err == nil {
		t.Fatal("Schema should fail before loading grouping module")
	}
	if err := ctx.LoadModule(groupingModuleName); err != nil {
		t.Fatalf("LoadModule: %v", err)
	}
	if _, err := ctx.Schema(groupingModuleName); err != nil {
		t.Fatalf("Schema after LoadModule: %v", err)
	}
}

func TestContextModules(t *testing.T) {
	ctx, cleanup := groupingContext(t)
	defer cleanup()

	mods := ctx.Modules()
	found := false
	for _, m := range mods {
		if m.Name() == groupingModuleName {
			found = true
			break
		}
	}
	if !found {
		names := make([]string, len(mods))
		for i, m := range mods {
			names[i] = m.Name()
		}
		t.Fatalf("Modules() = %v, want to contain %q", names, groupingModuleName)
	}
}

func TestContextModulesExcludesImportOnlyDependencies(t *testing.T) {
	ctx, cleanup := importContext(t)
	defer cleanup()

	mods := ctx.Modules()
	for _, mod := range mods {
		if !mod.IsImplemented() {
			t.Fatalf("Modules() included non-implemented module %q", mod.Name())
		}
		if mod.Name() == "cambium-import-target" {
			t.Fatal("Modules() should not include import-only dependency cambium-import-target")
		}
	}
	if _, err := ctx.Schema("cambium-import-target"); err != nil {
		t.Fatalf("Schema should still expose retained import-only module: %v", err)
	}
}

func TestGroupingOrigin(t *testing.T) {
	ctx, cleanup := groupingContext(t)
	defer cleanup()

	mod, err := ctx.Schema(groupingModuleName)
	if err != nil {
		t.Fatalf("Schema: %v", err)
	}
	top, err := mod.FindPath("/cambium-grouping-demo:top")
	if err != nil {
		t.Fatalf("FindPath top: %v", err)
	}

	direct := childByName(t, top.Children(), "direct-leaf")
	if _, ok := direct.GroupingOrigin(); ok {
		t.Fatal("direct-leaf should have no GroupingOrigin")
	}

	grouped := childByName(t, top.Children(), "grouped-leaf")
	origin, ok := grouped.GroupingOrigin()
	if !ok {
		t.Fatal("grouped-leaf should have a GroupingOrigin")
	}
	if origin != "common-grouping" {
		t.Fatalf("grouped-leaf GroupingOrigin() = %q, want common-grouping", origin)
	}

	container := childByName(t, top.Children(), "grouped-container")
	origin, ok = container.GroupingOrigin()
	if !ok {
		t.Fatal("grouped-container should have a GroupingOrigin")
	}
	if origin != "common-grouping" {
		t.Fatalf("grouped-container GroupingOrigin() = %q, want common-grouping", origin)
	}

	nested := childByName(t, container.Children(), "nested-leaf")
	origin, ok = nested.GroupingOrigin()
	if !ok {
		t.Fatal("nested-leaf should have a GroupingOrigin")
	}
	if origin != "common-grouping" {
		t.Fatalf("nested-leaf GroupingOrigin() = %q, want common-grouping", origin)
	}
}

func TestModuleGroupingDefinitionMetadataAccessors(t *testing.T) {
	ctx, cleanup := groupingContext(t)
	defer cleanup()

	mod, err := ctx.Schema(groupingModuleName)
	if err != nil {
		t.Fatalf("Schema: %v", err)
	}
	defs := mod.GroupingDefinitions()
	if got, want := len(defs), 2; got != want {
		t.Fatalf("GroupingDefinitions len = %d, want %d", got, want)
	}
	if defs[0].Name() != "common-grouping" || defs[1].Name() != "unused-grouping" {
		t.Fatalf("GroupingDefinitions = [%q, %q], want [common-grouping, unused-grouping]", defs[0].Name(), defs[1].Name())
	}
	if got := defs[0].Module().Name(); got != groupingModuleName {
		t.Fatalf("common-grouping Module = %q, want %s", got, groupingModuleName)
	}
	if got := defs[0].Status(); got != cambium.StatusDeprecated {
		t.Fatalf("common-grouping Status = %v, want deprecated", got)
	}
	if got, ok := defs[0].Description(); !ok || got != "Common grouping." {
		t.Fatalf("common-grouping Description = (%q,%v), want Common grouping.,true", got, ok)
	}
	if got, ok := defs[0].Reference(); !ok || got != "common-group-ref" {
		t.Fatalf("common-grouping Reference = (%q,%v), want common-group-ref,true", got, ok)
	}
	children := defs[0].ChildNames()
	wantChildren := []string{"grouped-leaf", "grouped-container"}
	if len(children) != len(wantChildren) {
		t.Fatalf("common-grouping ChildNames = %v, want %v", children, wantChildren)
	}
	for i, want := range wantChildren {
		if children[i] != want {
			t.Fatalf("common-grouping ChildNames = %v, want %v", children, wantChildren)
		}
	}
	if got := defs[1].Status(); got != cambium.StatusCurrent {
		t.Fatalf("unused-grouping Status = %v, want current", got)
	}
}

const deviationTargetModuleName = "cambium-deviation-target"
const deviationSourceModuleName = "cambium-deviation-source"

func deviationContext(t *testing.T) (ctx *cambium.Context, cleanup func()) {
	t.Helper()
	dir := schemaIntrospectionModuleDir(t)

	target := `module cambium-deviation-target {
    namespace "urn:cambium:devtarget";
    prefix cdt;
    yang-version 1.1;
    revision 2026-06-16;

    container top {
        leaf metric {
            type string;
        }
        leaf enabled {
            type string;
        }
        leaf label {
            type string;
        }
        leaf defaulted {
            type string;
            default "old";
        }
        leaf-list samples {
            type string;
            min-elements 1;
            max-elements 8;
        }
        list records {
            key "id";
            max-elements 8;
            leaf id {
                type string;
            }
        }
    }
}
`
	writeModuleFile(t, filepath.Join(dir, deviationTargetModuleName+".yang"), []byte(target))

	source := `module cambium-deviation-source {
    namespace "urn:cambium:devsource";
    prefix cds;
    yang-version 1.1;

    import cambium-deviation-target {
        prefix cdt;
        revision-date 2026-06-16;
    }

    revision 2026-06-16;

    container top {
        leaf metric {
            type string;
        }
    }

    deviation "/cdt:top/cdt:metric" {
        description "Make metric numeric.";
        reference "RFC 6020 deviation";
        deviate replace {
            type uint32;
        }
    }

    deviation "/cdt:top/cdt:enabled" {
        deviate not-supported;
    }

    deviation "/cdt:top/cdt:label" {
        deviate add {
            units "meters";
        }
    }

    deviation "/cdt:top/cdt:defaulted" {
        deviate replace {
            default "";
        }
    }

    deviation "/cdt:top/cdt:samples" {
        deviate replace {
            min-elements 2;
            max-elements 4;
        }
    }

    deviation "/cdt:top/cdt:records" {
        deviate replace {
            max-elements unbounded;
        }
    }
}
`
	writeModuleFile(t, filepath.Join(dir, deviationSourceModuleName+".yang"), []byte(source))

	ctx, err := cambium.NewContext()
	if err != nil {
		t.Fatal(err)
	}
	if err := ctx.SetSearchPath(dir); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{deviationTargetModuleName, deviationSourceModuleName} {
		if err := ctx.LoadModule(name); err != nil {
			t.Fatalf("LoadModule %s: %v", name, err)
		}
	}
	return ctx, func() { ctx.Close() }
}

func TestModuleDeviations(t *testing.T) {
	ctx, cleanup := deviationContext(t)
	defer cleanup()

	source, err := ctx.Schema(deviationSourceModuleName)
	if err != nil {
		t.Fatalf("Schema: %v", err)
	}

	devs := source.Deviations()
	if len(devs) != 7 {
		t.Fatalf("Deviations() len = %d, want 7: %+v", len(devs), devs)
	}

	if got, want := devs[0].TargetPath(), "/cdt:top/cdt:metric"; got != want {
		t.Fatalf("deviation[0].TargetPath = %q, want %q", got, want)
	}
	if got, want := devs[0].Type(), "replace"; got != want {
		t.Fatalf("deviation[0].Type = %q, want %q", got, want)
	}
	if got, want := devs[0].Property(), "type"; got != want {
		t.Fatalf("deviation[0].Property = %q, want %q", got, want)
	}
	if got, want := devs[0].NewValue(), "uint32"; got != want {
		t.Fatalf("deviation[0].NewValue = %q, want %q", got, want)
	}
	if got, want := devs[0].SourceModule(), deviationSourceModuleName; got != want {
		t.Fatalf("deviation[0].SourceModule = %q, want %q", got, want)
	}
	if dsc, ok := devs[0].Description(); !ok || dsc != "Make metric numeric." {
		t.Fatalf("deviation[0].Description() = (%q, %v), want (Make metric numeric., true)", dsc, ok)
	}
	if ref, ok := devs[0].Reference(); !ok || ref != "RFC 6020 deviation" {
		t.Fatalf("deviation[0].Reference() = (%q, %v), want (RFC 6020 deviation, true)", ref, ok)
	}

	if got, want := devs[1].TargetPath(), "/cdt:top/cdt:enabled"; got != want {
		t.Fatalf("deviation[1].TargetPath = %q, want %q", got, want)
	}
	if got, want := devs[1].Type(), "not-supported"; got != want {
		t.Fatalf("deviation[1].Type = %q, want %q", got, want)
	}
	if devs[1].Property() != "" || devs[1].NewValue() != "" {
		t.Fatalf("deviation[1] property/value should be empty, got %q/%q", devs[1].Property(), devs[1].NewValue())
	}

	if got, want := devs[2].TargetPath(), "/cdt:top/cdt:label"; got != want {
		t.Fatalf("deviation[2].TargetPath = %q, want %q", got, want)
	}
	if got, want := devs[2].Type(), "add"; got != want {
		t.Fatalf("deviation[2].Type = %q, want %q", got, want)
	}
	if got, want := devs[2].Property(), "units"; got != want {
		t.Fatalf("deviation[2].Property = %q, want %q", got, want)
	}
	if got, want := devs[2].NewValue(), "meters"; got != want {
		t.Fatalf("deviation[2].NewValue = %q, want %q", got, want)
	}

	if got, want := devs[3].TargetPath(), "/cdt:top/cdt:defaulted"; got != want {
		t.Fatalf("deviation[3].TargetPath = %q, want %q", got, want)
	}
	if got, want := devs[3].Type(), "replace"; got != want {
		t.Fatalf("deviation[3].Type = %q, want %q", got, want)
	}
	if got, want := devs[3].Property(), "default"; got != want {
		t.Fatalf("deviation[3].Property = %q, want %q", got, want)
	}
	if got, want := devs[3].NewValue(), ""; got != want {
		t.Fatalf("deviation[3].NewValue = %q, want %q", got, want)
	}

	if got, want := devs[4].TargetPath(), "/cdt:top/cdt:samples"; got != want {
		t.Fatalf("deviation[4].TargetPath = %q, want %q", got, want)
	}
	if got, want := devs[4].Type(), "replace"; got != want {
		t.Fatalf("deviation[4].Type = %q, want %q", got, want)
	}
	if got, want := devs[4].Property(), "min-elements"; got != want {
		t.Fatalf("deviation[4].Property = %q, want %q", got, want)
	}
	if got, want := devs[4].NewValue(), "2"; got != want {
		t.Fatalf("deviation[4].NewValue = %q, want %q", got, want)
	}

	if got, want := devs[5].TargetPath(), "/cdt:top/cdt:samples"; got != want {
		t.Fatalf("deviation[5].TargetPath = %q, want %q", got, want)
	}
	if got, want := devs[5].Type(), "replace"; got != want {
		t.Fatalf("deviation[5].Type = %q, want %q", got, want)
	}
	if got, want := devs[5].Property(), "max-elements"; got != want {
		t.Fatalf("deviation[5].Property = %q, want %q", got, want)
	}
	if got, want := devs[5].NewValue(), "4"; got != want {
		t.Fatalf("deviation[5].NewValue = %q, want %q", got, want)
	}

	if got, want := devs[6].TargetPath(), "/cdt:top/cdt:records"; got != want {
		t.Fatalf("deviation[6].TargetPath = %q, want %q", got, want)
	}
	if got, want := devs[6].Type(), "replace"; got != want {
		t.Fatalf("deviation[6].Type = %q, want %q", got, want)
	}
	if got, want := devs[6].Property(), "max-elements"; got != want {
		t.Fatalf("deviation[6].Property = %q, want %q", got, want)
	}
	if got, want := devs[6].NewValue(), "unbounded"; got != want {
		t.Fatalf("deviation[6].NewValue = %q, want %q", got, want)
	}
}

func TestDeviationExtensionInstanceDoesNotBecomeDeviationProperty(t *testing.T) {
	target := `module cambium-deviation-extension-target {
    namespace "urn:cambium:deviation-extension-target";
    prefix cdet;

    leaf value {
        type uint8;
    }

    leaf obsolete {
        type string;
    }
}`
	source := `module cambium-deviation-extension-source {
    namespace "urn:cambium:deviation-extension-source";
    prefix cdes;

    import cambium-deviation-extension-target {
        prefix target;
    }

    extension marker {
        argument note;
    }

    deviation "/target:value" {
        deviate replace {
            type string;
            cdes:marker "sidecar";
        }
    }

    deviation "/target:obsolete" {
        deviate not-supported {
            cdes:marker "remove";
        }
    }
}`
	builder, err := cambium.NewContextBuilder(cambium.ContextFlags{})
	if err != nil {
		t.Fatal(err)
	}
	if err := builder.LoadModuleStr(target); err != nil {
		t.Fatalf("Load target: %v", err)
	}
	if err := builder.LoadModuleStr(source); err != nil {
		t.Fatalf("Load source: %v", err)
	}
	ctx, err := builder.Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	defer ctx.Close()

	sourceMod, err := ctx.Schema("cambium-deviation-extension-source")
	if err != nil {
		t.Fatalf("Schema source: %v", err)
	}
	devs := sourceMod.Deviations()
	if len(devs) != 2 {
		t.Fatalf("Deviations() len = %d, want 2: %+v", len(devs), devs)
	}
	if got, want := devs[0].Property(), "type"; got != want {
		t.Fatalf("deviation property = %q, want %q", got, want)
	}
	if got, want := devs[0].NewValue(), "string"; got != want {
		t.Fatalf("deviation value = %q, want %q", got, want)
	}
	if got, want := devs[1].Type(), "not-supported"; got != want {
		t.Fatalf("deviation type = %q, want %q", got, want)
	}
	if devs[1].Property() != "" || devs[1].NewValue() != "" {
		t.Fatalf("not-supported property/value = %q/%q, want empty", devs[1].Property(), devs[1].NewValue())
	}

	targetMod, err := ctx.Schema("cambium-deviation-extension-target")
	if err != nil {
		t.Fatalf("Schema target: %v", err)
	}
	if _, err := targetMod.FindPath("/cdet:obsolete"); err == nil {
		t.Fatal("not-supported deviation with extension sidecar left obsolete leaf addressable")
	}
}

func TestSchemaNodeRefDeviationProvenance(t *testing.T) {
	ctx, cleanup := deviationContext(t)
	defer cleanup()

	target, err := ctx.Schema(deviationTargetModuleName)
	if err != nil {
		t.Fatalf("Schema: %v", err)
	}

	top, err := target.FindPath("/cambium-deviation-target:top")
	if err != nil {
		t.Fatalf("FindPath top: %v", err)
	}

	metric := childByName(t, top.Children(), "metric")
	provs := metric.DeviationProvenance()
	if len(provs) != 1 {
		t.Fatalf("metric.DeviationProvenance() len = %d, want 1: %+v", len(provs), provs)
	}
	if provs[0].Type() != "replace" || provs[0].Property() != "type" || provs[0].NewValue() != "uint32" {
		t.Fatalf("metric provenance = %+v, want replace/type/uint32", provs[0])
	}

	label := childByName(t, top.Children(), "label")
	provs = label.DeviationProvenance()
	if len(provs) != 1 {
		t.Fatalf("label.DeviationProvenance() len = %d, want 1: %+v", len(provs), provs)
	}
	if provs[0].Type() != "add" || provs[0].Property() != "units" || provs[0].NewValue() != "meters" {
		t.Fatalf("label provenance = %+v, want add/units/meters", provs[0])
	}

	// A sibling without a deviation returns an empty slice.
	if provs := top.DeviationProvenance(); len(provs) != 0 {
		t.Fatalf("top.DeviationProvenance() = %v, want empty", provs)
	}

	other, err := ctx.Schema(deviationSourceModuleName)
	if err != nil {
		t.Fatalf("Schema source: %v", err)
	}
	otherA, err := other.FindPath("/cds:top/cds:metric")
	if err != nil {
		t.Fatalf("FindPath source metric: %v", err)
	}
	if provs := otherA.DeviationProvenance(); len(provs) != 0 {
		t.Fatalf("source metric DeviationProvenance() = %v, want empty", provs)
	}
}

func TestDeviationEffectsApplyToSchemaIR(t *testing.T) {
	ctx, cleanup := deviationContext(t)
	defer cleanup()

	target, err := ctx.Schema(deviationTargetModuleName)
	if err != nil {
		t.Fatalf("Schema: %v", err)
	}
	top, err := target.FindPath("/cdt:top")
	if err != nil {
		t.Fatalf("FindPath top: %v", err)
	}

	gotChildren := schemaChildNames(top.Children())
	wantChildren := []string{"metric", "label", "defaulted", "samples", "records"}
	if strings.Join(gotChildren, ",") != strings.Join(wantChildren, ",") {
		t.Fatalf("top children after deviations = %v, want %v", gotChildren, wantChildren)
	}
	if _, err := target.FindPath("/cdt:top/cdt:enabled"); err == nil {
		t.Fatal("not-supported deviation left /top/enabled addressable")
	}

	metric := childByName(t, top.Children(), "metric")
	info, ok := metric.LeafType()
	if !ok {
		t.Fatal("metric LeafType missing")
	}
	if got, want := info.Base(), cambium.BaseTypeUint32; got != want {
		t.Fatalf("metric base after replace type = %v, want %v", got, want)
	}

	label := childByName(t, top.Children(), "label")
	if got, ok := label.Units(); !ok || got != "meters" {
		t.Fatalf("label units after add = (%q,%v), want (meters,true)", got, ok)
	}

	defaulted := childByName(t, top.Children(), "defaulted")
	if got, ok := defaulted.DefaultValue(); !ok || got != "" {
		t.Fatalf("defaulted default after replace = (%q,%v), want empty string,true", got, ok)
	}

	samples := childByName(t, top.Children(), "samples")
	if got, ok := samples.MinElements(); !ok || got != 2 {
		t.Fatalf("samples min-elements after replace = (%d,%v), want 2,true", got, ok)
	}
	if got, ok := samples.MaxElements(); !ok || got != 4 {
		t.Fatalf("samples max-elements after replace = (%d,%v), want 4,true", got, ok)
	}

	records := childByName(t, top.Children(), "records")
	if _, ok := records.MaxElements(); ok {
		t.Fatal("records max-elements after replace unbounded = ok true, want false")
	}
}

func TestDeviationConfigFalseInheritedByDescendants(t *testing.T) {
	t.Helper()
	dir := schemaIntrospectionModuleDir(t)

	target := `module cambium-deviation-config-inheritance-target {
    namespace "urn:cambium:deviation-config-inheritance-target";
    prefix tgt;
    yang-version 1.1;

    container system {
        container state {
            config true;
            leaf uptime {
                type uint64;
            }
            container counters {
                leaf packets {
                    type uint64;
                }
            }
        }
    }
}
`
	writeModuleFile(t, filepath.Join(dir, "cambium-deviation-config-inheritance-target.yang"), []byte(target))

	source := `module cambium-deviation-config-inheritance-source {
    namespace "urn:cambium:deviation-config-inheritance-source";
    prefix src;
    yang-version 1.1;

    import cambium-deviation-config-inheritance-target {
        prefix tgt;
    }

    deviation "/tgt:system/tgt:state" {
        deviate replace {
            config false;
        }
    }
}
`
	writeModuleFile(t, filepath.Join(dir, "cambium-deviation-config-inheritance-source.yang"), []byte(source))

	ctx, err := cambium.NewContext()
	if err != nil {
		t.Fatal(err)
	}
	defer ctx.Close()
	if err := ctx.SetSearchPath(dir); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{
		"cambium-deviation-config-inheritance-target",
		"cambium-deviation-config-inheritance-source",
	} {
		if err := ctx.LoadModule(name); err != nil {
			t.Fatalf("LoadModule %s: %v", name, err)
		}
	}

	mod, err := ctx.Schema("cambium-deviation-config-inheritance-target")
	if err != nil {
		t.Fatalf("Schema: %v", err)
	}
	system, err := mod.FindPath("/tgt:system")
	if err != nil {
		t.Fatalf("FindPath system: %v", err)
	}
	if system.Config() != cambium.ConfigRw || system.ReadOnly() {
		t.Fatalf("system config = %v/readOnly=%v, want ConfigRw/false", system.Config(), system.ReadOnly())
	}
	for _, path := range []string{
		"/tgt:system/tgt:state",
		"/tgt:system/tgt:state/tgt:uptime",
		"/tgt:system/tgt:state/tgt:counters",
		"/tgt:system/tgt:state/tgt:counters/tgt:packets",
	} {
		node, err := mod.FindPath(path)
		if err != nil {
			t.Fatalf("FindPath %s: %v", path, err)
		}
		if node.Config() != cambium.ConfigRo || !node.ReadOnly() {
			t.Fatalf("%s config = %v/readOnly=%v, want ConfigRo/true", path, node.Config(), node.ReadOnly())
		}
	}
}

func ifFeatureContext(t *testing.T) (ctx *cambium.Context, cleanup func()) {
	t.Helper()
	dir := schemaIntrospectionModuleDir(t)

	module := `module cambium-if-feature-demo {
    namespace "urn:cambium:if-feature";
    prefix cif;
    yang-version 1.1;

    feature advanced;

    container top {
        leaf always { type string; }
        leaf gated {
            if-feature advanced;
            type string;
        }
        uses gated-grouping;
    }

    grouping gated-grouping {
        leaf group-always { type string; }
        leaf group-gated {
            if-feature advanced;
            type string;
        }
    }
}
`
	path := filepath.Join(dir, "cambium-if-feature-demo.yang")
	writeModuleFile(t, path, []byte(module))

	ctx, err := cambium.NewContext()
	if err != nil {
		t.Fatal(err)
	}
	if err := ctx.SetSearchPath(dir); err != nil {
		t.Fatal(err)
	}
	if err := ctx.LoadModule("cambium-if-feature-demo"); err != nil {
		t.Fatalf("LoadModule: %v", err)
	}
	return ctx, func() { ctx.Close() }
}

func TestIfFeatureDirectFeatureNameFiltered(t *testing.T) {
	ctx, cleanup := ifFeatureContext(t)
	defer cleanup()

	mod, err := ctx.Schema("cambium-if-feature-demo")
	if err != nil {
		t.Fatalf("Schema: %v", err)
	}
	top, err := mod.FindPath("/cif:top")
	if err != nil {
		t.Fatalf("FindPath top: %v", err)
	}

	got := schemaChildNames(top.Children())
	want := []string{"always", "group-always"}
	if len(got) != len(want) {
		t.Fatalf("top children = %v, want %v", got, want)
	}
	for i, name := range want {
		if got[i] != name {
			t.Fatalf("top children = %v, want %v", got, want)
		}
	}

	if _, err := mod.FindPath("/cif:top/cif:gated"); err == nil {
		t.Fatal("gated leaf should be absent from IR when feature is disabled")
	}
	if _, err := mod.FindPath("/cif:top/cif:group-gated"); err == nil {
		t.Fatal("group-gated leaf should be absent from IR when feature is disabled")
	}
}

func TestIfFeatureEnabledDirectFeatureNameIncluded(t *testing.T) {
	ctx, cleanup := ifFeatureContext(t)
	defer cleanup()

	if err := ctx.SetFeatures("cambium-if-feature-demo", []string{"advanced"}); err != nil {
		t.Fatalf("SetFeatures: %v", err)
	}

	mod, err := ctx.Schema("cambium-if-feature-demo")
	if err != nil {
		t.Fatalf("Schema: %v", err)
	}
	top, err := mod.FindPath("/cif:top")
	if err != nil {
		t.Fatalf("FindPath top: %v", err)
	}

	got := schemaChildNames(top.Children())
	want := []string{"always", "gated", "group-always", "group-gated"}
	if len(got) != len(want) {
		t.Fatalf("top children = %v, want %v", got, want)
	}
	for i, name := range want {
		if got[i] != name {
			t.Fatalf("top children = %v, want %v", got, want)
		}
	}
	gated := childByName(t, top.Children(), "gated")
	if got, want := gated.IfFeatures(), []string{"advanced"}; len(got) != len(want) || got[0] != want[0] {
		t.Fatalf("gated IfFeatures = %v, want %v", got, want)
	}
	always := childByName(t, top.Children(), "always")
	if got := always.IfFeatures(); len(got) != 0 {
		t.Fatalf("always IfFeatures = %v, want empty", got)
	}
}

func TestModuleFeaturesAndFeatureValue(t *testing.T) {
	t.Helper()
	dir := schemaIntrospectionModuleDir(t)

	module := `module cambium-feature-query {
    namespace "urn:cambium:feature-query";
    prefix cfq;
    yang-version 1.1;

    feature basic {
        status deprecated;
        description "Basic capability.";
        reference "basic-ref";
    }
    feature advanced {
        description "Advanced capability.";
    }
}
`
	writeModuleFile(t, filepath.Join(dir, "cambium-feature-query.yang"), []byte(module))

	builder, err := cambium.NewContextBuilder(cambium.ContextFlags{})
	if err != nil {
		t.Fatalf("NewContextBuilder: %v", err)
	}
	if err := builder.SearchPath(dir); err != nil {
		t.Fatalf("SearchPath: %v", err)
	}
	if err := builder.LoadModule("cambium-feature-query", nil, []string{"advanced"}); err != nil {
		t.Fatalf("LoadModule: %v", err)
	}
	ctx, err := builder.Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	defer ctx.Close()

	mod, err := ctx.Schema("cambium-feature-query")
	if err != nil {
		t.Fatalf("Schema: %v", err)
	}
	features := mod.Features()
	if got, want := len(features), 2; got != want {
		t.Fatalf("Features len = %d, want %d", got, want)
	}
	if features[0].Name() != "basic" || features[1].Name() != "advanced" {
		t.Fatalf("Features = [%q, %q], want [basic, advanced]", features[0].Name(), features[1].Name())
	}
	if got, ok := features[0].Description(); !ok || got != "Basic capability." {
		t.Fatalf("basic Description = (%q,%v), want Basic capability.,true", got, ok)
	}
	if got, ok := features[0].Reference(); !ok || got != "basic-ref" {
		t.Fatalf("basic Reference = (%q,%v), want basic-ref,true", got, ok)
	}
	if got := features[0].Status(); got != cambium.StatusDeprecated {
		t.Fatalf("basic Status = %v, want deprecated", got)
	}
	if got := features[1].Status(); got != cambium.StatusCurrent {
		t.Fatalf("advanced Status = %v, want current", got)
	}
	if got := features[1].Module().Name(); got != "cambium-feature-query" {
		t.Fatalf("advanced Module = %q, want cambium-feature-query", got)
	}
	if got, ok := mod.FeatureValue("advanced"); !ok || !got {
		t.Fatalf("FeatureValue(advanced) = (%v,%v), want true,true", got, ok)
	}
	if got, ok := mod.FeatureValue("basic"); !ok || got {
		t.Fatalf("FeatureValue(basic) = (%v,%v), want false,true", got, ok)
	}
	if got, ok := mod.FeatureValue("missing"); ok || got {
		t.Fatalf("FeatureValue(missing) = (%v,%v), want false,false", got, ok)
	}
}

func TestModuleHeaderMetadataAccessors(t *testing.T) {
	source := `module cambium-module-header-metadata {
    namespace "urn:cambium:module-header-metadata";
    prefix cmhm;
    organization "Signalbreak Labs";
    contact "ops@example.invalid";
    description "Module description.";
    reference "Module reference.";
    leaf value { type string; }
}`

	builder, err := cambium.NewContextBuilder(cambium.ContextFlags{})
	if err != nil {
		t.Fatalf("NewContextBuilder: %v", err)
	}
	if err := builder.LoadModuleStr(source); err != nil {
		t.Fatalf("LoadModuleStr: %v", err)
	}
	ctx, err := builder.Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	defer ctx.Close()

	mod, err := ctx.Schema("cambium-module-header-metadata")
	if err != nil {
		t.Fatalf("Schema: %v", err)
	}
	if got, ok := mod.Organization(); !ok || got != "Signalbreak Labs" {
		t.Fatalf("Organization = (%q,%v), want Signalbreak Labs,true", got, ok)
	}
	if got, ok := mod.Contact(); !ok || got != "ops@example.invalid" {
		t.Fatalf("Contact = (%q,%v), want ops@example.invalid,true", got, ok)
	}
	if got, ok := mod.Description(); !ok || got != "Module description." {
		t.Fatalf("Description = (%q,%v), want Module description.,true", got, ok)
	}
	if got, ok := mod.Reference(); !ok || got != "Module reference." {
		t.Fatalf("Reference = (%q,%v), want Module reference.,true", got, ok)
	}

	emptySource := `module cambium-module-header-empty {
    namespace "urn:cambium:module-header-empty";
    prefix cmhe;
    leaf value { type string; }
}`
	builder, err = cambium.NewContextBuilder(cambium.ContextFlags{})
	if err != nil {
		t.Fatalf("NewContextBuilder empty: %v", err)
	}
	if err := builder.LoadModuleStr(emptySource); err != nil {
		t.Fatalf("LoadModuleStr empty: %v", err)
	}
	emptyCtx, err := builder.Build()
	if err != nil {
		t.Fatalf("Build empty: %v", err)
	}
	defer emptyCtx.Close()
	emptyMod, err := emptyCtx.Schema("cambium-module-header-empty")
	if err != nil {
		t.Fatalf("Schema empty: %v", err)
	}
	if got, ok := emptyMod.Organization(); ok || got != "" {
		t.Fatalf("empty Organization = (%q,%v), want empty,false", got, ok)
	}
	if got, ok := emptyMod.Contact(); ok || got != "" {
		t.Fatalf("empty Contact = (%q,%v), want empty,false", got, ok)
	}
	if got, ok := emptyMod.Description(); ok || got != "" {
		t.Fatalf("empty Description = (%q,%v), want empty,false", got, ok)
	}
	if got, ok := emptyMod.Reference(); ok || got != "" {
		t.Fatalf("empty Reference = (%q,%v), want empty,false", got, ok)
	}
}

func TestModuleRevisionMetadataAccessors(t *testing.T) {
	source := `module cambium-module-revision-metadata {
    namespace "urn:cambium:module-revision-metadata";
    prefix cmrm;
    revision 2026-06-01 {
        description "Current revision.";
        reference "current-ref";
    }
    revision 2025-01-01 {
        description "Previous revision.";
    }
    leaf value { type string; }
}`

	builder, err := cambium.NewContextBuilder(cambium.ContextFlags{})
	if err != nil {
		t.Fatalf("NewContextBuilder: %v", err)
	}
	if err := builder.LoadModuleStr(source); err != nil {
		t.Fatalf("LoadModuleStr: %v", err)
	}
	ctx, err := builder.Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	defer ctx.Close()

	mod, err := ctx.Schema("cambium-module-revision-metadata")
	if err != nil {
		t.Fatalf("Schema: %v", err)
	}
	if got, ok := mod.Revision(); !ok || got != "2026-06-01" {
		t.Fatalf("Revision = (%q,%v), want 2026-06-01,true", got, ok)
	}
	revisions := mod.Revisions()
	if got, want := len(revisions), 2; got != want {
		t.Fatalf("Revisions len = %d, want %d", got, want)
	}
	if got := revisions[0].Date(); got != "2026-06-01" {
		t.Fatalf("revisions[0].Date = %q, want 2026-06-01", got)
	}
	if got, ok := revisions[0].Description(); !ok || got != "Current revision." {
		t.Fatalf("revisions[0].Description = (%q,%v), want Current revision.,true", got, ok)
	}
	if got, ok := revisions[0].Reference(); !ok || got != "current-ref" {
		t.Fatalf("revisions[0].Reference = (%q,%v), want current-ref,true", got, ok)
	}
	if got := revisions[1].Date(); got != "2025-01-01" {
		t.Fatalf("revisions[1].Date = %q, want 2025-01-01", got)
	}
	if got, ok := revisions[1].Description(); !ok || got != "Previous revision." {
		t.Fatalf("revisions[1].Description = (%q,%v), want Previous revision.,true", got, ok)
	}
	if got, ok := revisions[1].Reference(); ok || got != "" {
		t.Fatalf("revisions[1].Reference = (%q,%v), want empty,false", got, ok)
	}
}

func TestModuleIncludesMetadataAccessors(t *testing.T) {
	dir := schemaIntrospectionModuleDir(t)

	module := `module cambium-include-query {
    namespace "urn:cambium:include-query";
    prefix ciq;
    yang-version 1.1;
    include cambium-include-query-part {
        revision-date 2026-06-01;
        description "Part include.";
        reference "include-ref";
    }
}
`
	writeModuleFile(t, filepath.Join(dir, "cambium-include-query.yang"), []byte(module))

	submodule := `submodule cambium-include-query-part {
    yang-version 1.1;
    belongs-to cambium-include-query {
        prefix ciq;
    }
    revision 2026-06-01;
    leaf from-part { type string; }
}
`
	writeModuleFile(t, filepath.Join(dir, "cambium-include-query-part@2026-06-01.yang"), []byte(submodule))

	ctx, err := cambium.NewContext()
	if err != nil {
		t.Fatal(err)
	}
	defer ctx.Close()
	if err := ctx.SetSearchPath(dir); err != nil {
		t.Fatal(err)
	}
	if err := ctx.LoadModule("cambium-include-query"); err != nil {
		t.Fatalf("LoadModule: %v", err)
	}
	mod, err := ctx.Schema("cambium-include-query")
	if err != nil {
		t.Fatalf("Schema: %v", err)
	}
	includes := mod.Includes()
	if got, want := len(includes), 1; got != want {
		t.Fatalf("Includes len = %d, want %d", got, want)
	}
	if got := includes[0].Name; got != "cambium-include-query-part" {
		t.Fatalf("include Name = %q, want cambium-include-query-part", got)
	}
	if got := includes[0].Revision; got != "2026-06-01" {
		t.Fatalf("include Revision = %q, want 2026-06-01", got)
	}
	if got, ok := includes[0].Description(); !ok || got != "Part include." {
		t.Fatalf("include Description = (%q,%v), want Part include.,true", got, ok)
	}
	if got, ok := includes[0].Reference(); !ok || got != "include-ref" {
		t.Fatalf("include Reference = (%q,%v), want include-ref,true", got, ok)
	}
}

func TestIfFeatureAugmentFiltered(t *testing.T) {
	t.Helper()
	dir := schemaIntrospectionModuleDir(t)

	baseModule := `module cambium-if-feature-base {
    namespace "urn:cambium:if-feature-base";
    prefix cifb;
    yang-version 1.1;

    container top {
        leaf always { type string; }
    }
}
`
	writeModuleFile(t, filepath.Join(dir, "cambium-if-feature-base.yang"), []byte(baseModule))

	augmentModule := `module cambium-if-feature-augment {
    namespace "urn:cambium:if-feature-augment";
    prefix cifa;
    yang-version 1.1;

    import cambium-if-feature-base {
        prefix base;
    }

    feature advanced;

    augment "/base:top" {
        if-feature advanced;
        leaf augmented-gated { type string; }
    }

    augment "/base:top" {
        leaf augmented-always { type string; }
    }
}
`
	writeModuleFile(t, filepath.Join(dir, "cambium-if-feature-augment.yang"), []byte(augmentModule))

	ctx, err := cambium.NewContext()
	if err != nil {
		t.Fatal(err)
	}
	defer ctx.Close()
	if err := ctx.SetSearchPath(dir); err != nil {
		t.Fatal(err)
	}
	if err := ctx.LoadModule("cambium-if-feature-augment"); err != nil {
		t.Fatalf("LoadModule: %v", err)
	}

	mod, err := ctx.Schema("cambium-if-feature-base")
	if err != nil {
		t.Fatalf("Schema: %v", err)
	}
	top, err := mod.FindPath("/cifb:top")
	if err != nil {
		t.Fatalf("FindPath top: %v", err)
	}

	got := schemaChildNames(top.Children())
	want := []string{"always", "augmented-always"}
	if len(got) != len(want) {
		t.Fatalf("top children = %v, want %v", got, want)
	}
	for i, name := range want {
		if got[i] != name {
			t.Fatalf("top children = %v, want %v", got, want)
		}
	}

	if _, err := mod.FindPath("/cifb:top/cifb:augmented-gated"); err == nil {
		t.Fatal("augmented-gated leaf should be absent when augment if-feature is disabled")
	}

	enabledCtx, err := cambium.NewContext()
	if err != nil {
		t.Fatal(err)
	}
	defer enabledCtx.Close()
	if err := enabledCtx.SetSearchPath(dir); err != nil {
		t.Fatal(err)
	}
	if err := enabledCtx.SetFeatures("cambium-if-feature-augment", []string{"advanced"}); err != nil {
		t.Fatal(err)
	}
	if err := enabledCtx.LoadModule("cambium-if-feature-augment"); err != nil {
		t.Fatalf("enabled LoadModule: %v", err)
	}
	enabledMod, err := enabledCtx.Schema("cambium-if-feature-base")
	if err != nil {
		t.Fatalf("enabled Schema: %v", err)
	}
	enabledTop, err := enabledMod.FindPath("/cifb:top")
	if err != nil {
		t.Fatalf("enabled FindPath top: %v", err)
	}
	gated := childByName(t, enabledTop.Children(), "augmented-gated")
	if got, want := gated.IfFeatures(), []string{"advanced"}; len(got) != len(want) || got[0] != want[0] {
		t.Fatalf("augmented-gated IfFeatures = %v, want %v", got, want)
	}
	always := childByName(t, enabledTop.Children(), "augmented-always")
	if got := always.IfFeatures(); len(got) != 0 {
		t.Fatalf("augmented-always IfFeatures = %v, want empty", got)
	}
}

func TestIfFeatureOnUsesRecordedOnExpandedNodes(t *testing.T) {
	t.Helper()
	dir := schemaIntrospectionModuleDir(t)

	module := `module cambium-if-feature-uses {
    namespace "urn:cambium:if-feature-uses";
    prefix cifu;
    yang-version 1.1;

    feature advanced;

    grouping gated-group {
        leaf grouped {
            type string;
        }
        container nested {
            leaf inner {
                type string;
            }
        }
    }

    container top {
        uses gated-group {
            if-feature advanced;
        }
    }
}
`
	writeModuleFile(t, filepath.Join(dir, "cambium-if-feature-uses.yang"), []byte(module))

	ctx, err := cambium.NewContext()
	if err != nil {
		t.Fatal(err)
	}
	defer ctx.Close()
	if err := ctx.SetSearchPath(dir); err != nil {
		t.Fatal(err)
	}
	if err := ctx.LoadModule("cambium-if-feature-uses"); err != nil {
		t.Fatalf("LoadModule: %v", err)
	}
	mod, err := ctx.Schema("cambium-if-feature-uses")
	if err != nil {
		t.Fatalf("Schema: %v", err)
	}
	top, err := mod.FindPath("/cifu:top")
	if err != nil {
		t.Fatalf("FindPath top: %v", err)
	}
	if got := schemaChildNames(top.Children()); len(got) != 0 {
		t.Fatalf("top children = %v, want empty when uses if-feature is disabled", got)
	}

	enabledCtx, err := cambium.NewContext()
	if err != nil {
		t.Fatal(err)
	}
	defer enabledCtx.Close()
	if err := enabledCtx.SetSearchPath(dir); err != nil {
		t.Fatal(err)
	}
	if err := enabledCtx.SetFeatures("cambium-if-feature-uses", []string{"advanced"}); err != nil {
		t.Fatal(err)
	}
	if err := enabledCtx.LoadModule("cambium-if-feature-uses"); err != nil {
		t.Fatalf("enabled LoadModule: %v", err)
	}
	enabledMod, err := enabledCtx.Schema("cambium-if-feature-uses")
	if err != nil {
		t.Fatalf("enabled Schema: %v", err)
	}
	grouped, err := enabledMod.FindPath("/cifu:top/cifu:grouped")
	if err != nil {
		t.Fatalf("enabled FindPath grouped: %v", err)
	}
	if got, want := grouped.IfFeatures(), []string{"advanced"}; len(got) != len(want) || got[0] != want[0] {
		t.Fatalf("grouped IfFeatures = %v, want %v", got, want)
	}
	inner, err := enabledMod.FindPath("/cifu:top/cifu:nested/cifu:inner")
	if err != nil {
		t.Fatalf("enabled FindPath inner: %v", err)
	}
	if got, want := inner.IfFeatures(), []string{"advanced"}; len(got) != len(want) || got[0] != want[0] {
		t.Fatalf("inner IfFeatures = %v, want %v", got, want)
	}
}

func TestCrossModuleMandatoryAugmentVersionRules(t *testing.T) {
	base := `module cambium-mandatory-augment-base {
    namespace "urn:cambium:mandatory-augment-base";
    prefix cmab;

    container top {
        leaf mode { type string; }
    }
}`
	cases := []struct {
		name        string
		augment     string
		wantErr     string
		wantPresent bool
		wantWhen    bool
	}{
		{
			name: "yang 1 mandatory leaf rejected",
			augment: `module cambium-mandatory-augment-yang1 {
    namespace "urn:cambium:mandatory-augment-yang1";
    prefix cmay1;

    import cambium-mandatory-augment-base {
        prefix base;
    }

    augment "/base:top" {
        leaf required-name {
            mandatory true;
            type string;
        }
    }
}`,
			wantErr: `augment "/base:top" adds mandatory config node "required-name" to another module and requires yang-version 1.1`,
		},
		{
			name: "yang 1 mandatory leaf with when rejected",
			augment: `module cambium-mandatory-augment-yang1-when {
    namespace "urn:cambium:mandatory-augment-yang1-when";
    prefix cmay1w;

    import cambium-mandatory-augment-base {
        prefix base;
    }

    augment "/base:top" {
        when "base:mode = 'enabled'";
        leaf required-name {
            mandatory true;
            type string;
        }
    }
}`,
			wantErr: `augment "/base:top" adds mandatory config node "required-name" to another module and requires yang-version 1.1`,
		},
		{
			name: "yang 1.1 mandatory leaf without when rejected",
			augment: `module cambium-mandatory-augment-yang11-no-when {
    yang-version 1.1;
    namespace "urn:cambium:mandatory-augment-yang11-no-when";
    prefix cmay11n;

    import cambium-mandatory-augment-base {
        prefix base;
    }

    augment "/base:top" {
        leaf required-name {
            mandatory true;
            type string;
        }
    }
}`,
			wantErr: `augment "/base:top" adds mandatory config node "required-name" to another module without a when statement`,
		},
		{
			name: "yang 1.1 conditional mandatory leaf accepted",
			augment: `module cambium-mandatory-augment-yang11-when {
    yang-version 1.1;
    namespace "urn:cambium:mandatory-augment-yang11-when";
    prefix cmay11w;

    import cambium-mandatory-augment-base {
        prefix base;
    }

    augment "/base:top" {
        when "base:mode = 'enabled'";
        leaf required-name {
            mandatory true;
            type string;
        }
    }
}`,
			wantPresent: true,
			wantWhen:    true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			builder, err := cambium.NewContextBuilder(cambium.ContextFlags{})
			if err != nil {
				t.Fatal(err)
			}
			if err := builder.LoadModuleStr(base); err != nil {
				t.Fatalf("LoadModuleStr base: %v", err)
			}
			if err := builder.LoadModuleStr(tc.augment); err != nil {
				t.Fatalf("LoadModuleStr augment: %v", err)
			}
			ctx, err := builder.Build()
			if tc.wantErr != "" {
				if err == nil {
					ctx.Close()
					t.Fatal("Build accepted invalid mandatory augment")
				}
				var ce *cambium.Error
				if !errors.As(err, &ce) || ce.RuleCode() != cambium.RuleCodeContext {
					t.Fatalf("Build error = %v, want RuleCodeContext", err)
				}
				if !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("Build error = %q, want to contain %q", err.Error(), tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("Build: %v", err)
			}
			defer ctx.Close()
			mod, err := ctx.Schema("cambium-mandatory-augment-base")
			if err != nil {
				t.Fatalf("Schema base: %v", err)
			}
			top, err := mod.FindPath("/cmab:top")
			if err != nil {
				t.Fatalf("FindPath top: %v", err)
			}
			required, ok := top.Children().Lookup("required-name")
			if ok != tc.wantPresent {
				t.Fatalf("required-name present = %v, want %v", ok, tc.wantPresent)
			}
			if tc.wantPresent && !required.IsMandatory() {
				t.Fatal("required-name should be mandatory")
			}
			if tc.wantWhen && len(required.Whens()) != 1 {
				t.Fatalf("required-name whens = %d, want 1", len(required.Whens()))
			}
		})
	}
}

func TestCrossModuleMandatoryAugmentIntoRPCInputIsNotConfig(t *testing.T) {
	base := `module cambium-mandatory-rpc-augment-base {
    namespace "urn:cambium:mandatory-rpc-augment-base";
    prefix cmrab;

    rpc reset {
        input {
            container params {
                leaf name { type string; }
            }
        }
    }
}`
	augment := `module cambium-mandatory-rpc-augment-source {
    namespace "urn:cambium:mandatory-rpc-augment-source";
    prefix cmras;

    import cambium-mandatory-rpc-augment-base {
        prefix base;
    }

    augment "/base:reset/base:input/base:params" {
        leaf required-name {
            mandatory true;
            type string;
        }
    }
}`

	builder, err := cambium.NewContextBuilder(cambium.ContextFlags{})
	if err != nil {
		t.Fatal(err)
	}
	if err := builder.LoadModuleStr(base); err != nil {
		t.Fatalf("LoadModuleStr base: %v", err)
	}
	if err := builder.LoadModuleStr(augment); err != nil {
		t.Fatalf("LoadModuleStr augment: %v", err)
	}
	ctx, err := builder.Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	defer ctx.Close()

	mod, err := ctx.Schema("cambium-mandatory-rpc-augment-base")
	if err != nil {
		t.Fatalf("Schema base: %v", err)
	}
	rpcs := mod.RPCs()
	if rpcs.Len() != 1 {
		t.Fatalf("RPCs() len = %d, want 1", rpcs.Len())
	}
	rpc, _ := rpcs.Get(0)
	input, ok := rpc.Input()
	if !ok {
		t.Fatal("reset Input() returned false")
	}
	params, ok := input.Children().Lookup("params")
	if !ok {
		t.Fatal("input params not found")
	}
	required, ok := params.Children().Lookup("required-name")
	if !ok {
		t.Fatal("augmented required-name not found")
	}
	if !required.IsMandatory() {
		t.Fatal("required-name should be mandatory")
	}
	if required.RepresentsConfigurationData() {
		t.Fatal("RPC input payload leaf should not represent configuration data")
	}
}

func TestIfFeatureExpressionDisabledByDefault(t *testing.T) {
	t.Helper()
	dir := schemaIntrospectionModuleDir(t)

	module := `module cambium-if-feature-expr {
    namespace "urn:cambium:if-feature-expr";
    prefix cife;
    yang-version 1.1;

    feature alpha;
    feature beta;

    container top {
        leaf always { type string; }
        leaf complex-gated {
            if-feature "alpha and beta";
            type string;
        }
    }
}
`
	path := filepath.Join(dir, "cambium-if-feature-expr.yang")
	writeModuleFile(t, path, []byte(module))

	ctx, err := cambium.NewContext()
	if err != nil {
		t.Fatal(err)
	}
	defer ctx.Close()
	if err := ctx.SetSearchPath(dir); err != nil {
		t.Fatal(err)
	}
	if err := ctx.LoadModule("cambium-if-feature-expr"); err != nil {
		t.Fatalf("LoadModule: %v", err)
	}

	mod, err := ctx.Schema("cambium-if-feature-expr")
	if err != nil {
		t.Fatalf("Schema: %v", err)
	}
	top, err := mod.FindPath("/cife:top")
	if err != nil {
		t.Fatalf("FindPath top: %v", err)
	}

	got := schemaChildNames(top.Children())
	want := []string{"always"}
	if len(got) != len(want) {
		t.Fatalf("top children = %v, want %v", got, want)
	}

	if _, err := mod.FindPath("/cife:top/cife:complex-gated"); err == nil {
		t.Fatal("if-feature expression should be filtered when its features are disabled")
	}
}

func TestIfFeatureExpressionEvaluated(t *testing.T) {
	t.Helper()
	dir := schemaIntrospectionModuleDir(t)

	module := `module cambium-if-feature-expr-enabled {
    namespace "urn:cambium:if-feature-expr-enabled";
    prefix cifee;
    yang-version 1.1;

    feature alpha;
    feature beta;
    feature gamma;

    container top {
        leaf always { type string; }
        leaf and-gated {
            if-feature "alpha and beta";
            type string;
        }
        leaf not-gated {
            if-feature "not gamma";
            type string;
        }
        leaf paren-gated {
            if-feature "(alpha or gamma) and beta";
            type string;
        }
        leaf false-gated {
            if-feature "alpha and gamma";
            type string;
        }
        leaf multi-gated {
            if-feature alpha;
            if-feature "not gamma";
            type string;
        }
    }
}
`
	path := filepath.Join(dir, "cambium-if-feature-expr-enabled.yang")
	writeModuleFile(t, path, []byte(module))

	ctx, err := cambium.NewContext()
	if err != nil {
		t.Fatal(err)
	}
	defer ctx.Close()
	if err := ctx.SetSearchPath(dir); err != nil {
		t.Fatal(err)
	}
	if err := ctx.SetFeatures("cambium-if-feature-expr-enabled", []string{"alpha", "beta"}); err != nil {
		t.Fatalf("SetFeatures: %v", err)
	}
	if err := ctx.LoadModule("cambium-if-feature-expr-enabled"); err != nil {
		t.Fatalf("LoadModule: %v", err)
	}

	mod, err := ctx.Schema("cambium-if-feature-expr-enabled")
	if err != nil {
		t.Fatalf("Schema: %v", err)
	}
	top, err := mod.FindPath("/cifee:top")
	if err != nil {
		t.Fatalf("FindPath top: %v", err)
	}

	got := schemaChildNames(top.Children())
	want := []string{"always", "and-gated", "not-gated", "paren-gated", "multi-gated"}
	if len(got) != len(want) {
		t.Fatalf("top children = %v, want %v", got, want)
	}
	for i, name := range want {
		if got[i] != name {
			t.Fatalf("top children = %v, want %v", got, want)
		}
	}
	andGated := childByName(t, top.Children(), "and-gated")
	if got, want := andGated.IfFeatures(), []string{"alpha and beta"}; len(got) != len(want) || got[0] != want[0] {
		t.Fatalf("and-gated IfFeatures = %v, want %v", got, want)
	}
	multiGated := childByName(t, top.Children(), "multi-gated")
	if got, want := multiGated.IfFeatures(), []string{"alpha", "not gamma"}; len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("multi-gated IfFeatures = %v, want %v", got, want)
	}
	always := childByName(t, top.Children(), "always")
	if got := always.IfFeatures(); len(got) != 0 {
		t.Fatalf("always IfFeatures = %v, want empty", got)
	}
	if _, err := mod.FindPath("/cifee:top/cifee:false-gated"); err == nil {
		t.Fatal("false-gated leaf should be absent when gamma is disabled")
	}
}

func TestIfFeatureDependencyChain(t *testing.T) {
	t.Helper()
	dir := schemaIntrospectionModuleDir(t)
	module := `module cambium-if-feature-dependency {
    namespace "urn:cambium:if-feature-dependency";
    prefix cifd;
    yang-version 1.1;

    feature base;
    feature dependent {
        if-feature base;
    }

    container top {
        leaf gated {
            if-feature dependent;
            type string;
        }
    }
}
`
	writeModuleFile(t, filepath.Join(dir, "cambium-if-feature-dependency.yang"), []byte(module))

	load := func(t *testing.T, features []string) cambium.Module {
		t.Helper()
		builder, err := cambium.NewContextBuilder(cambium.ContextFlags{})
		if err != nil {
			t.Fatal(err)
		}
		if err := builder.SearchPath(dir); err != nil {
			t.Fatal(err)
		}
		if err := builder.LoadModule("cambium-if-feature-dependency", nil, features); err != nil {
			t.Fatalf("LoadModule features %v: %v", features, err)
		}
		ctx, err := builder.Build()
		if err != nil {
			t.Fatalf("Build features %v: %v", features, err)
		}
		t.Cleanup(ctx.Close)
		mod, err := ctx.Schema("cambium-if-feature-dependency")
		if err != nil {
			t.Fatalf("Schema features %v: %v", features, err)
		}
		return mod
	}

	onlyDependent := load(t, []string{"dependent"})
	if got, ok := onlyDependent.FeatureValue("dependent"); !ok || got {
		t.Fatalf("FeatureValue(dependent) with only dependent = (%v,%v), want false,true", got, ok)
	}
	top, err := onlyDependent.FindPath("/cifd:top")
	if err != nil {
		t.Fatalf("FindPath top only dependent: %v", err)
	}
	if _, ok := top.Children().Lookup("gated"); ok {
		t.Fatal("gated leaf should be absent when dependent feature's base dependency is disabled")
	}

	baseAndDependent := load(t, []string{"base", "dependent"})
	if got, ok := baseAndDependent.FeatureValue("dependent"); !ok || !got {
		t.Fatalf("FeatureValue(dependent) with base+dependent = (%v,%v), want true,true", got, ok)
	}
	features := baseAndDependent.Features()
	if got, want := len(features), 2; got != want {
		t.Fatalf("Features len = %d, want %d", got, want)
	}
	if got, want := features[1].IfFeatures(), []string{"base"}; len(got) != len(want) || got[0] != want[0] {
		t.Fatalf("dependent IfFeatures = %v, want %v", got, want)
	}
	top, err = baseAndDependent.FindPath("/cifd:top")
	if err != nil {
		t.Fatalf("FindPath top base+dependent: %v", err)
	}
	if _, ok := top.Children().Lookup("gated"); !ok {
		t.Fatal("gated leaf should be present when dependent feature and its base dependency are enabled")
	}
}

func TestIfFeaturePrefixedFeatureEvaluated(t *testing.T) {
	t.Helper()
	dir := schemaIntrospectionModuleDir(t)

	features := `module cambium-if-feature-source {
    namespace "urn:cambium:if-feature-source";
    prefix ciffs;
    yang-version 1.1;

    feature remote;
}
`
	writeModuleFile(t, filepath.Join(dir, "cambium-if-feature-source.yang"), []byte(features))

	user := `module cambium-if-feature-user {
    namespace "urn:cambium:if-feature-user";
    prefix cifu;
    yang-version 1.1;

    import cambium-if-feature-source {
        prefix src;
    }

    container top {
        leaf always { type string; }
        leaf remote-gated {
            if-feature src:remote;
            type string;
        }
    }
}
`
	writeModuleFile(t, filepath.Join(dir, "cambium-if-feature-user.yang"), []byte(user))

	ctx, err := cambium.NewContext()
	if err != nil {
		t.Fatal(err)
	}
	defer ctx.Close()
	if err := ctx.SetSearchPath(dir); err != nil {
		t.Fatal(err)
	}
	if err := ctx.SetFeatures("cambium-if-feature-source", []string{"remote"}); err != nil {
		t.Fatalf("SetFeatures: %v", err)
	}
	if err := ctx.LoadModule("cambium-if-feature-user"); err != nil {
		t.Fatalf("LoadModule: %v", err)
	}

	mod, err := ctx.Schema("cambium-if-feature-user")
	if err != nil {
		t.Fatalf("Schema: %v", err)
	}
	top, err := mod.FindPath("/cifu:top")
	if err != nil {
		t.Fatalf("FindPath top: %v", err)
	}

	got := schemaChildNames(top.Children())
	want := []string{"always", "remote-gated"}
	if len(got) != len(want) {
		t.Fatalf("top children = %v, want %v", got, want)
	}
	for i, name := range want {
		if got[i] != name {
			t.Fatalf("top children = %v, want %v", got, want)
		}
	}
}

func TestInvalidIfFeatureExpressionsReturnContextRuleCode(t *testing.T) {
	cases := []struct {
		name    string
		sources []string
		message string
	}{
		{
			name: "invalid syntax",
			sources: []string{`module cambium-if-feature-invalid-syntax {
    namespace "urn:cambium:if-feature-invalid-syntax";
    prefix cifis;
    yang-version 1.1;

    feature alpha;

    container top {
        if-feature "alpha and";
        leaf value { type string; }
    }
}`},
			message: `invalid or unresolved if-feature expression "alpha and"`,
		},
		{
			name: "unknown local feature",
			sources: []string{`module cambium-if-feature-unknown-local {
    namespace "urn:cambium:if-feature-unknown-local";
    prefix ciful;
    yang-version 1.1;

    container top {
        if-feature missing;
        leaf value { type string; }
    }
}`},
			message: `invalid or unresolved if-feature expression "missing"`,
		},
		{
			name: "unicode padded feature",
			sources: []string{`module cambium-if-feature-unicode-padded {
    namespace "urn:cambium:if-feature-unicode-padded";
    prefix cifupad;
    yang-version 1.1;

    feature alpha;

    container top {
        if-feature "` + "\u2003" + `alpha` + "\u2003" + `";
        leaf value { type string; }
    }
}`},
			message: `invalid or unresolved if-feature expression`,
		},
		{
			name: "unknown prefixed feature",
			sources: []string{
				`module cambium-if-feature-empty-source {
    namespace "urn:cambium:if-feature-empty-source";
    prefix src;
    yang-version 1.1;
}`,
				`module cambium-if-feature-unknown-prefixed {
    namespace "urn:cambium:if-feature-unknown-prefixed";
    prefix cifup;
    yang-version 1.1;

    import cambium-if-feature-empty-source {
        prefix src;
    }

    container top {
        if-feature src:missing;
        leaf value { type string; }
    }
}`},
			message: `invalid or unresolved if-feature expression "src:missing"`,
		},
		{
			name: "feature dependency cycle",
			sources: []string{`module cambium-if-feature-cycle {
    namespace "urn:cambium:if-feature-cycle";
    prefix cifc;
    yang-version 1.1;

    feature a {
        if-feature b;
    }

    feature b {
        if-feature a;
    }

    container top {
        if-feature a;
        leaf value { type string; }
    }
}`},
			message: `invalid or unresolved if-feature expression "b"`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			builder, err := cambium.NewContextBuilder(cambium.ContextFlags{})
			if err != nil {
				t.Fatal(err)
			}
			for _, source := range tc.sources {
				if err := builder.LoadModuleStr(source); err != nil {
					t.Fatalf("LoadModuleStr: %v", err)
				}
			}
			_, err = builder.Build()
			if err == nil {
				t.Fatal("Build accepted invalid if-feature expression")
			}
			var ce *cambium.Error
			if !errors.As(err, &ce) || ce.RuleCode() != cambium.RuleCodeContext {
				t.Fatalf("Build error = %v, want RuleCodeContext", err)
			}
			if !strings.Contains(err.Error(), tc.message) {
				t.Fatalf("Build error = %q, want to contain %q", err.Error(), tc.message)
			}
		})
	}
}

func TestUnknownEnabledFeatureReturnsContextRuleCode(t *testing.T) {
	source := `module cambium-unknown-enabled-feature {
    namespace "urn:cambium:unknown-enabled-feature";
    prefix cuef;
    yang-version 1.1;

    feature declared;

    container top {
        leaf value { type string; }
    }
}`
	builder, err := cambium.NewContextBuilder(cambium.ContextFlags{})
	if err != nil {
		t.Fatal(err)
	}
	if err := builder.SetFeatures("cambium-unknown-enabled-feature", []string{"missing"}); err != nil {
		t.Fatalf("SetFeatures: %v", err)
	}
	if err := builder.LoadModuleStr(source); err != nil {
		t.Fatalf("LoadModuleStr: %v", err)
	}
	_, err = builder.Build()
	if err == nil {
		t.Fatal("Build accepted unknown enabled feature")
	}
	var ce *cambium.Error
	if !errors.As(err, &ce) || ce.RuleCode() != cambium.RuleCodeContext {
		t.Fatalf("Build error = %v, want RuleCodeContext", err)
	}
	if !strings.Contains(err.Error(), `unknown feature "missing" for module "cambium-unknown-enabled-feature"`) {
		t.Fatalf("Build error = %q, want unknown feature message", err.Error())
	}
}

func TestIfFeatureNameWithOperatorSubstringFiltered(t *testing.T) {
	t.Helper()
	dir := schemaIntrospectionModuleDir(t)

	module := `module cambium-if-feature-oper-substr {
    namespace "urn:cambium:if-feature-oper-substr";
    prefix cifos;
    yang-version 1.1;

    feature android;
    feature notable;
    feature orange;

    container top {
        leaf always { type string; }
        leaf android-gated {
            if-feature android;
            type string;
        }
        leaf notable-gated {
            if-feature notable;
            type string;
        }
        leaf orange-gated {
            if-feature orange;
            type string;
        }
    }
}
`
	path := filepath.Join(dir, "cambium-if-feature-oper-substr.yang")
	writeModuleFile(t, path, []byte(module))

	ctx, err := cambium.NewContext()
	if err != nil {
		t.Fatal(err)
	}
	defer ctx.Close()
	if err := ctx.SetSearchPath(dir); err != nil {
		t.Fatal(err)
	}
	if err := ctx.LoadModule("cambium-if-feature-oper-substr"); err != nil {
		t.Fatalf("LoadModule: %v", err)
	}

	mod, err := ctx.Schema("cambium-if-feature-oper-substr")
	if err != nil {
		t.Fatalf("Schema: %v", err)
	}
	top, err := mod.FindPath("/cifos:top")
	if err != nil {
		t.Fatalf("FindPath top: %v", err)
	}

	got := schemaChildNames(top.Children())
	want := []string{"always"}
	if len(got) != len(want) {
		t.Fatalf("top children = %v, want %v", got, want)
	}

	for _, name := range []string{"android-gated", "notable-gated", "orange-gated"} {
		if _, err := mod.FindPath("/cifos:top/cifos:" + name); err == nil {
			t.Fatalf("%s leaf should be absent when feature is disabled", name)
		}
	}
}

func TestIfFeatureYang10OperatorNamesAreFeatureRefs(t *testing.T) {
	t.Helper()
	dir := schemaIntrospectionModuleDir(t)

	module := `module cambium-if-feature-yang10-operator-name {
    namespace "urn:cambium:if-feature-yang10-operator-name";
    prefix cifyon;

    feature not;

    container top {
        leaf always { type string; }
        leaf gated {
            if-feature not;
            type string;
        }
    }
}
`
	writeModuleFile(t, filepath.Join(dir, "cambium-if-feature-yang10-operator-name.yang"), []byte(module))

	disabledCtx, err := cambium.NewContext()
	if err != nil {
		t.Fatal(err)
	}
	defer disabledCtx.Close()
	if err := disabledCtx.SetSearchPath(dir); err != nil {
		t.Fatal(err)
	}
	if err := disabledCtx.LoadModule("cambium-if-feature-yang10-operator-name"); err != nil {
		t.Fatalf("LoadModule disabled: %v", err)
	}
	disabledMod, err := disabledCtx.Schema("cambium-if-feature-yang10-operator-name")
	if err != nil {
		t.Fatalf("Schema disabled: %v", err)
	}
	disabledTop, err := disabledMod.FindPath("/cifyon:top")
	if err != nil {
		t.Fatalf("FindPath disabled top: %v", err)
	}
	if got, want := schemaChildNames(disabledTop.Children()), []string{"always"}; strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("disabled top children = %v, want %v", got, want)
	}

	enabledCtx, err := cambium.NewContext()
	if err != nil {
		t.Fatal(err)
	}
	defer enabledCtx.Close()
	if err := enabledCtx.SetSearchPath(dir); err != nil {
		t.Fatal(err)
	}
	if err := enabledCtx.SetFeatures("cambium-if-feature-yang10-operator-name", []string{"not"}); err != nil {
		t.Fatalf("SetFeatures: %v", err)
	}
	if err := enabledCtx.LoadModule("cambium-if-feature-yang10-operator-name"); err != nil {
		t.Fatalf("LoadModule enabled: %v", err)
	}
	enabledMod, err := enabledCtx.Schema("cambium-if-feature-yang10-operator-name")
	if err != nil {
		t.Fatalf("Schema enabled: %v", err)
	}
	enabledTop, err := enabledMod.FindPath("/cifyon:top")
	if err != nil {
		t.Fatalf("FindPath enabled top: %v", err)
	}
	if got, want := schemaChildNames(enabledTop.Children()), []string{"always", "gated"}; strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("enabled top children = %v, want %v", got, want)
	}
}

func TestIfFeatureOnDeviationNotAppliedWhenDisabled(t *testing.T) {
	t.Helper()
	dir := schemaIntrospectionModuleDir(t)

	target := `module cambium-if-feature-deviation-target {
    namespace "urn:cambium:if-feature-deviation-target";
    prefix cifdvt;
    yang-version 1.1;

    container top {
        leaf config-true {
            config true;
            type string;
        }
    }
}
`
	writeModuleFile(t, filepath.Join(dir, "cambium-if-feature-deviation-target.yang"), []byte(target))

	source := `module cambium-if-feature-deviation-source {
    namespace "urn:cambium:if-feature-deviation-source";
    prefix cifdvs;
    yang-version 1.1;

    import cambium-if-feature-deviation-target {
        prefix tgt;
    }

    feature advanced;

    deviation "/tgt:top/tgt:config-true" {
        if-feature advanced;
        deviate replace {
            config false;
        }
    }
}
`
	writeModuleFile(t, filepath.Join(dir, "cambium-if-feature-deviation-source.yang"), []byte(source))

	ctx, err := cambium.NewContext()
	if err != nil {
		t.Fatal(err)
	}
	defer ctx.Close()
	if err := ctx.SetSearchPath(dir); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"cambium-if-feature-deviation-target", "cambium-if-feature-deviation-source"} {
		if err := ctx.LoadModule(name); err != nil {
			t.Fatalf("LoadModule %s: %v", name, err)
		}
	}

	targetMod, err := ctx.Schema("cambium-if-feature-deviation-target")
	if err != nil {
		t.Fatalf("Schema: %v", err)
	}
	leaf, err := targetMod.FindPath("/cifdvt:top/cifdvt:config-true")
	if err != nil {
		t.Fatalf("FindPath config-true: %v", err)
	}
	if leaf.Config() != cambium.ConfigRw {
		t.Fatalf("config-true Config() = %v, want ConfigRw (deviation with disabled if-feature should not apply)", leaf.Config())
	}

	enabledCtx, err := cambium.NewContext()
	if err != nil {
		t.Fatal(err)
	}
	defer enabledCtx.Close()
	if err := enabledCtx.SetSearchPath(dir); err != nil {
		t.Fatal(err)
	}
	if err := enabledCtx.SetFeatures("cambium-if-feature-deviation-source", []string{"advanced"}); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"cambium-if-feature-deviation-target", "cambium-if-feature-deviation-source"} {
		if err := enabledCtx.LoadModule(name); err != nil {
			t.Fatalf("enabled LoadModule %s: %v", name, err)
		}
	}
	sourceMod, err := enabledCtx.Schema("cambium-if-feature-deviation-source")
	if err != nil {
		t.Fatalf("enabled source Schema: %v", err)
	}
	var found cambium.Deviation
	for _, dev := range sourceMod.Deviations() {
		if dev.TargetPath() == "/tgt:top/tgt:config-true" && dev.Property() == "config" {
			found = dev
			break
		}
	}
	if found.TargetPath() == "" {
		t.Fatal("enabled deviation config property not found")
	}
	if got, want := found.IfFeatures(), []string{"advanced"}; len(got) != len(want) || got[0] != want[0] {
		t.Fatalf("deviation IfFeatures = %v, want %v", got, want)
	}
}

func TestIfFeatureOnRefineNotApplied(t *testing.T) {
	t.Helper()
	dir := schemaIntrospectionModuleDir(t)

	module := `module cambium-if-feature-refine {
    namespace "urn:cambium:if-feature-refine";
    prefix cifr;
    yang-version 1.1;

    feature advanced;

    grouping gated-group {
        leaf config-true {
            type string;
        }
    }

    container top {
        uses gated-group {
            refine config-true {
                if-feature advanced;
                config false;
            }
        }
    }
}
`
	writeModuleFile(t, filepath.Join(dir, "cambium-if-feature-refine.yang"), []byte(module))

	ctx, err := cambium.NewContext()
	if err != nil {
		t.Fatal(err)
	}
	defer ctx.Close()
	if err := ctx.SetSearchPath(dir); err != nil {
		t.Fatal(err)
	}
	if err := ctx.LoadModule("cambium-if-feature-refine"); err != nil {
		t.Fatalf("LoadModule: %v", err)
	}

	mod, err := ctx.Schema("cambium-if-feature-refine")
	if err != nil {
		t.Fatalf("Schema: %v", err)
	}
	leaf, err := mod.FindPath("/cifr:top/cifr:config-true")
	if err != nil {
		t.Fatalf("FindPath config-true: %v", err)
	}
	if leaf.Config() != cambium.ConfigRw {
		t.Fatalf("config-true Config() = %v, want ConfigRw (refine with disabled if-feature should not apply)", leaf.Config())
	}

	enabledCtx, err := cambium.NewContext()
	if err != nil {
		t.Fatal(err)
	}
	defer enabledCtx.Close()
	if err := enabledCtx.SetSearchPath(dir); err != nil {
		t.Fatal(err)
	}
	if err := enabledCtx.SetFeatures("cambium-if-feature-refine", []string{"advanced"}); err != nil {
		t.Fatal(err)
	}
	if err := enabledCtx.LoadModule("cambium-if-feature-refine"); err != nil {
		t.Fatalf("enabled LoadModule: %v", err)
	}
	enabledMod, err := enabledCtx.Schema("cambium-if-feature-refine")
	if err != nil {
		t.Fatalf("enabled Schema: %v", err)
	}
	enabledLeaf, err := enabledMod.FindPath("/cifr:top/cifr:config-true")
	if err != nil {
		t.Fatalf("enabled FindPath config-true: %v", err)
	}
	if enabledLeaf.Config() != cambium.ConfigRo {
		t.Fatalf("enabled config-true Config() = %v, want ConfigRo", enabledLeaf.Config())
	}
	if got, want := enabledLeaf.IfFeatures(), []string{"advanced"}; len(got) != len(want) || got[0] != want[0] {
		t.Fatalf("enabled config-true IfFeatures = %v, want %v", got, want)
	}
}

func TestIfFeatureOnEnumValueExcluded(t *testing.T) {
	t.Helper()
	dir := schemaIntrospectionModuleDir(t)

	module := `module cambium-if-feature-enum {
    namespace "urn:cambium:if-feature-enum";
    prefix cifev;
    yang-version 1.1;

    feature advanced;

    container top {
        leaf status {
            type enumeration {
                enum alpha;
                enum beta {
                    if-feature advanced;
                }
                enum gamma;
            }
        }
    }
}
`
	writeModuleFile(t, filepath.Join(dir, "cambium-if-feature-enum.yang"), []byte(module))

	ctx, err := cambium.NewContext()
	if err != nil {
		t.Fatal(err)
	}
	defer ctx.Close()
	if err := ctx.SetSearchPath(dir); err != nil {
		t.Fatal(err)
	}
	if err := ctx.LoadModule("cambium-if-feature-enum"); err != nil {
		t.Fatalf("LoadModule: %v", err)
	}

	mod, err := ctx.Schema("cambium-if-feature-enum")
	if err != nil {
		t.Fatalf("Schema: %v", err)
	}
	leaf, err := mod.FindPath("/cifev:top/cifev:status")
	if err != nil {
		t.Fatalf("FindPath status: %v", err)
	}
	info, ok := leaf.LeafType()
	if !ok {
		t.Fatal("expected leaf type")
	}
	r, ok := info.Resolved().(cambium.ResolvedEnumeration)
	if !ok {
		t.Fatalf("expected enumeration, got %T", info.Resolved())
	}
	vals := r.Values()
	if len(vals) != 2 {
		t.Fatalf("enum values = %d, want 2", len(vals))
	}
	want := []struct {
		name  string
		value int64
	}{
		{"alpha", 0},
		{"gamma", 2},
	}
	for i, tc := range want {
		if vals[i].Name() != tc.name || vals[i].Value() != tc.value {
			t.Fatalf("enum[%d] = (%q, %d), want (%q, %d)", i, vals[i].Name(), vals[i].Value(), tc.name, tc.value)
		}
	}

	enabledCtx, err := cambium.NewContext()
	if err != nil {
		t.Fatal(err)
	}
	defer enabledCtx.Close()
	if err := enabledCtx.SetSearchPath(dir); err != nil {
		t.Fatal(err)
	}
	if err := enabledCtx.SetFeatures("cambium-if-feature-enum", []string{"advanced"}); err != nil {
		t.Fatalf("SetFeatures enabled: %v", err)
	}
	if err := enabledCtx.LoadModule("cambium-if-feature-enum"); err != nil {
		t.Fatalf("LoadModule enabled: %v", err)
	}
	enabledMod, err := enabledCtx.Schema("cambium-if-feature-enum")
	if err != nil {
		t.Fatalf("Schema enabled: %v", err)
	}
	enabledLeaf, err := enabledMod.FindPath("/cifev:top/cifev:status")
	if err != nil {
		t.Fatalf("FindPath status enabled: %v", err)
	}
	enabledInfo, ok := enabledLeaf.LeafType()
	if !ok {
		t.Fatal("expected enabled leaf type")
	}
	enabledEnum, ok := enabledInfo.Resolved().(cambium.ResolvedEnumeration)
	if !ok {
		t.Fatalf("expected enabled enumeration, got %T", enabledInfo.Resolved())
	}
	enabledVals := enabledEnum.Values()
	if len(enabledVals) != 3 {
		t.Fatalf("enabled enum values = %d, want 3", len(enabledVals))
	}
	if got, want := enabledVals[1].IfFeatures(), []string{"advanced"}; len(got) != len(want) || got[0] != want[0] {
		t.Fatalf("enabled enum beta IfFeatures = %v, want %v", got, want)
	}
}

func TestIfFeatureOnBitsValueExcluded(t *testing.T) {
	t.Helper()
	dir := schemaIntrospectionModuleDir(t)

	module := `module cambium-if-feature-bits {
    namespace "urn:cambium:if-feature-bits";
    prefix cifb;
    yang-version 1.1;

    feature advanced;

    container top {
        leaf flags {
            type bits {
                bit one;
                bit two {
                    if-feature advanced;
                }
                bit four;
            }
        }
    }
}
`
	writeModuleFile(t, filepath.Join(dir, "cambium-if-feature-bits.yang"), []byte(module))

	ctx, err := cambium.NewContext()
	if err != nil {
		t.Fatal(err)
	}
	defer ctx.Close()
	if err := ctx.SetSearchPath(dir); err != nil {
		t.Fatal(err)
	}
	if err := ctx.LoadModule("cambium-if-feature-bits"); err != nil {
		t.Fatalf("LoadModule: %v", err)
	}

	mod, err := ctx.Schema("cambium-if-feature-bits")
	if err != nil {
		t.Fatalf("Schema: %v", err)
	}
	leaf, err := mod.FindPath("/cifb:top/cifb:flags")
	if err != nil {
		t.Fatalf("FindPath flags: %v", err)
	}
	info, ok := leaf.LeafType()
	if !ok {
		t.Fatal("expected leaf type")
	}
	r, ok := info.Resolved().(cambium.ResolvedBits)
	if !ok {
		t.Fatalf("expected bits, got %T", info.Resolved())
	}
	vals := r.Values()
	if len(vals) != 2 {
		t.Fatalf("bits values = %d, want 2", len(vals))
	}
	want := []struct {
		name  string
		value int64
	}{
		{"one", 0},
		{"four", 2},
	}
	for i, tc := range want {
		if vals[i].Name() != tc.name || vals[i].Value() != tc.value {
			t.Fatalf("bits[%d] = (%q, %d), want (%q, %d)", i, vals[i].Name(), vals[i].Value(), tc.name, tc.value)
		}
	}

	enabledCtx, err := cambium.NewContext()
	if err != nil {
		t.Fatal(err)
	}
	defer enabledCtx.Close()
	if err := enabledCtx.SetSearchPath(dir); err != nil {
		t.Fatal(err)
	}
	if err := enabledCtx.SetFeatures("cambium-if-feature-bits", []string{"advanced"}); err != nil {
		t.Fatalf("SetFeatures enabled: %v", err)
	}
	if err := enabledCtx.LoadModule("cambium-if-feature-bits"); err != nil {
		t.Fatalf("LoadModule enabled: %v", err)
	}
	enabledMod, err := enabledCtx.Schema("cambium-if-feature-bits")
	if err != nil {
		t.Fatalf("Schema enabled: %v", err)
	}
	enabledLeaf, err := enabledMod.FindPath("/cifb:top/cifb:flags")
	if err != nil {
		t.Fatalf("FindPath flags enabled: %v", err)
	}
	enabledInfo, ok := enabledLeaf.LeafType()
	if !ok {
		t.Fatal("expected enabled leaf type")
	}
	enabledBits, ok := enabledInfo.Resolved().(cambium.ResolvedBits)
	if !ok {
		t.Fatalf("expected enabled bits, got %T", enabledInfo.Resolved())
	}
	enabledVals := enabledBits.Values()
	if len(enabledVals) != 3 {
		t.Fatalf("enabled bits values = %d, want 3", len(enabledVals))
	}
	if got, want := enabledVals[1].IfFeatures(), []string{"advanced"}; len(got) != len(want) || got[0] != want[0] {
		t.Fatalf("enabled bit two IfFeatures = %v, want %v", got, want)
	}
}

func TestIfFeatureCanDisableAllEnumValues(t *testing.T) {
	source := `module cambium-if-feature-all-enum {
    namespace "urn:cambium:if-feature-all-enum";
    prefix cifae;
    yang-version 1.1;

    feature advanced;

    leaf state {
        type enumeration {
            enum enabled {
                if-feature advanced;
            }
        }
    }
}`
	builder, err := cambium.NewContextBuilder(cambium.ContextFlags{})
	if err != nil {
		t.Fatal(err)
	}
	if err := builder.LoadModuleStr(source); err != nil {
		t.Fatalf("LoadModuleStr: %v", err)
	}
	ctx, err := builder.Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	defer ctx.Close()
	mod, err := ctx.Schema("cambium-if-feature-all-enum")
	if err != nil {
		t.Fatalf("Schema: %v", err)
	}
	enum := resolvedTypeFor[cambium.ResolvedEnumeration](t, mod, "/cifae:state")
	if got := len(enum.Values()); got != 0 {
		t.Fatalf("disabled enum values = %d, want 0", got)
	}
}

func TestIfFeatureCanDisableAllBitValues(t *testing.T) {
	source := `module cambium-if-feature-all-bits {
    namespace "urn:cambium:if-feature-all-bits";
    prefix cifab;
    yang-version 1.1;

    feature advanced;

    leaf flags {
        type bits {
            bit enabled {
                if-feature advanced;
            }
        }
    }
}`
	builder, err := cambium.NewContextBuilder(cambium.ContextFlags{})
	if err != nil {
		t.Fatal(err)
	}
	if err := builder.LoadModuleStr(source); err != nil {
		t.Fatalf("LoadModuleStr: %v", err)
	}
	ctx, err := builder.Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	defer ctx.Close()
	mod, err := ctx.Schema("cambium-if-feature-all-bits")
	if err != nil {
		t.Fatalf("Schema: %v", err)
	}
	bits := resolvedTypeFor[cambium.ResolvedBits](t, mod, "/cifab:flags")
	if got := len(bits.Values()); got != 0 {
		t.Fatalf("disabled bit values = %d, want 0", got)
	}
}

func TestIfFeatureOnIdentityExcluded(t *testing.T) {
	t.Helper()
	dir := schemaIntrospectionModuleDir(t)

	module := `module cambium-if-feature-identity {
    namespace "urn:cambium:if-feature-identity";
    prefix cifid;
    yang-version 1.1;

    feature advanced;

    identity base-id;
    identity derived-id {
        base base-id;
        if-feature advanced;
    }
}
`
	writeModuleFile(t, filepath.Join(dir, "cambium-if-feature-identity.yang"), []byte(module))

	ctx, err := cambium.NewContext()
	if err != nil {
		t.Fatal(err)
	}
	defer ctx.Close()
	if err := ctx.SetSearchPath(dir); err != nil {
		t.Fatal(err)
	}
	if err := ctx.LoadModule("cambium-if-feature-identity"); err != nil {
		t.Fatalf("LoadModule: %v", err)
	}

	mod, err := ctx.Schema("cambium-if-feature-identity")
	if err != nil {
		t.Fatalf("Schema: %v", err)
	}

	var base cambium.Identity
	for id := range mod.Identities() {
		switch id.Name() {
		case "base-id":
			base = id
		case "derived-id":
			t.Fatal("derived-id identity should be absent when feature is disabled")
		}
	}
	if base.Name() == "" {
		t.Fatal("base-id identity not found")
	}
	if derived := base.Derived(); len(derived) != 0 {
		t.Fatalf("base-id derived = %v, want empty", namesOf(derived))
	}

	enabledCtx, err := cambium.NewContext()
	if err != nil {
		t.Fatal(err)
	}
	defer enabledCtx.Close()
	if err := enabledCtx.SetSearchPath(dir); err != nil {
		t.Fatal(err)
	}
	if err := enabledCtx.SetFeatures("cambium-if-feature-identity", []string{"advanced"}); err != nil {
		t.Fatalf("SetFeatures enabled: %v", err)
	}
	if err := enabledCtx.LoadModule("cambium-if-feature-identity"); err != nil {
		t.Fatalf("LoadModule enabled: %v", err)
	}
	enabledMod, err := enabledCtx.Schema("cambium-if-feature-identity")
	if err != nil {
		t.Fatalf("Schema enabled: %v", err)
	}
	var derived cambium.Identity
	for id := range enabledMod.Identities() {
		if id.Name() == "derived-id" {
			derived = id
			break
		}
	}
	if derived.Name() == "" {
		t.Fatal("derived-id identity not found when feature is enabled")
	}
	if got, want := derived.IfFeatures(), []string{"advanced"}; len(got) != len(want) || got[0] != want[0] {
		t.Fatalf("derived-id IfFeatures = %v, want %v", got, want)
	}
}

func TestIfFeatureCanDisableIdentityBase(t *testing.T) {
	source := `module cambium-if-feature-identity-base {
    namespace "urn:cambium:if-feature-identity-base";
    prefix cifib;
    yang-version 1.1;

    feature advanced;

    identity base-id {
        if-feature advanced;
    }
    identity child-id {
        base base-id;
    }
}`
	builder, err := cambium.NewContextBuilder(cambium.ContextFlags{})
	if err != nil {
		t.Fatal(err)
	}
	if err := builder.LoadModuleStr(source); err != nil {
		t.Fatalf("LoadModuleStr: %v", err)
	}
	ctx, err := builder.Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	defer ctx.Close()
	if _, err := ctx.Schema("cambium-if-feature-identity-base"); err != nil {
		t.Fatalf("Schema: %v", err)
	}
}

func TestIfFeatureOnExtensionInstanceExcluded(t *testing.T) {
	t.Helper()
	dir := schemaIntrospectionModuleDir(t)

	module := `module cambium-if-feature-extension {
    namespace "urn:cambium:if-feature-extension";
    prefix cife;
    yang-version 1.1;

    feature advanced;

    extension my-ext;

    container top {
        cife:my-ext {
            if-feature advanced;
        }
    }
}
`
	writeModuleFile(t, filepath.Join(dir, "cambium-if-feature-extension.yang"), []byte(module))

	ctx, err := cambium.NewContext()
	if err != nil {
		t.Fatal(err)
	}
	defer ctx.Close()
	if err := ctx.SetSearchPath(dir); err != nil {
		t.Fatal(err)
	}
	if err := ctx.LoadModule("cambium-if-feature-extension"); err != nil {
		t.Fatalf("LoadModule: %v", err)
	}

	mod, err := ctx.Schema("cambium-if-feature-extension")
	if err != nil {
		t.Fatalf("Schema: %v", err)
	}
	top, err := mod.FindPath("/cife:top")
	if err != nil {
		t.Fatalf("FindPath top: %v", err)
	}
	if exts := top.Extensions(); len(exts) != 0 {
		t.Fatalf("Extensions() len = %d, want 0 when extension instance is gated by disabled feature", len(exts))
	}

	enabledCtx, err := cambium.NewContext()
	if err != nil {
		t.Fatal(err)
	}
	defer enabledCtx.Close()
	if err := enabledCtx.SetSearchPath(dir); err != nil {
		t.Fatal(err)
	}
	if err := enabledCtx.SetFeatures("cambium-if-feature-extension", []string{"advanced"}); err != nil {
		t.Fatal(err)
	}
	if err := enabledCtx.LoadModule("cambium-if-feature-extension"); err != nil {
		t.Fatalf("enabled LoadModule: %v", err)
	}
	enabledMod, err := enabledCtx.Schema("cambium-if-feature-extension")
	if err != nil {
		t.Fatalf("enabled Schema: %v", err)
	}
	enabledTop, err := enabledMod.FindPath("/cife:top")
	if err != nil {
		t.Fatalf("enabled FindPath top: %v", err)
	}
	exts := enabledTop.Extensions()
	if len(exts) != 1 {
		t.Fatalf("enabled Extensions() len = %d, want 1", len(exts))
	}
	if got, want := exts[0].IfFeatures(), []string{"advanced"}; len(got) != len(want) || got[0] != want[0] {
		t.Fatalf("extension IfFeatures = %v, want %v", got, want)
	}
}
