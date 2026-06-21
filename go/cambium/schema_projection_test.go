package cambium_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/signalbreak-labs/cambium/go/cambium"
)

func loadProjectionModule(t *testing.T, name string, sources ...string) cambium.Module {
	t.Helper()
	builder, err := cambium.NewContextBuilder(cambium.ContextFlags{})
	if err != nil {
		t.Fatal(err)
	}
	for _, source := range sources {
		if err := builder.LoadModuleStr(source); err != nil {
			t.Fatalf("LoadModuleStr: %v", err)
		}
	}
	ctx, err := builder.Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	t.Cleanup(func() { ctx.Close() })
	mod, err := ctx.Schema(name)
	if err != nil {
		t.Fatalf("Schema %s: %v", name, err)
	}
	return mod
}

func projectionLines(p cambium.Projection) []string {
	var out []string
	var walk func([]cambium.ProjectedNode, int)
	walk = func(nodes []cambium.ProjectedNode, depth int) {
		for _, node := range nodes {
			prefix := strings.Repeat("  ", depth)
			line := prefix + node.Node.Module().Name() + ":" + node.Node.Name()
			var roles []string
			if node.Role.Has(cambium.ProjectionRoleAncestor) {
				roles = append(roles, "ancestor")
			}
			if node.Role.Has(cambium.ProjectionRoleSelected) {
				roles = append(roles, "selected")
			}
			if node.Role.Has(cambium.ProjectionRoleDescendant) {
				roles = append(roles, "descendant")
			}
			if node.Role.Has(cambium.ProjectionRoleKey) {
				roles = append(roles, "key")
			}
			if node.Role.Has(cambium.ProjectionRoleMandatory) {
				roles = append(roles, "mandatory")
			}
			if node.Role.Has(cambium.ProjectionRoleDefault) {
				roles = append(roles, "default")
			}
			if len(roles) > 0 {
				line += " [" + strings.Join(roles, ",") + "]"
			}
			out = append(out, line)
			walk(node.Children, depth+1)
		}
	}
	walk(p.Roots, 0)
	return out
}

func assertProjectionLines(t *testing.T, got cambium.Projection, want []string) {
	t.Helper()
	lines := projectionLines(got)
	if strings.Join(lines, "\n") != strings.Join(want, "\n") {
		t.Fatalf("projection mismatch:\n got:\n%s\nwant:\n%s", strings.Join(lines, "\n"), strings.Join(want, "\n"))
	}
}

const projectionBase = `module projection-base {
    yang-version 1.1;
    namespace "urn:projection-base";
    prefix pb;

    container root {
        list iface {
            key "name";
            leaf description { type string; }
            leaf name { type string; }
            container config {
                leaf z { type string; }
                leaf a { type string; }
                leaf mtu { type uint16; }
            }
            container state {
                leaf oper-status { type string; }
            }
        }
        leaf tail { type string; }
    }
}`

func TestProjectSchemaPathsMergesAncestorSpinesInSchemaOrder(t *testing.T) {
	mod := loadProjectionModule(t, "projection-base", projectionBase)

	projection, err := cambium.ProjectSchemaPaths(mod, []string{
		"/pb:root/pb:iface/pb:config/pb:mtu",
		"/pb:root/pb:iface/pb:config/pb:a",
	}, cambium.ProjectionOptions{
		FlattenChoices:      true,
		IncludeListKeys:     true,
		ListKeysFirst:       true,
		IncludeDescendants:  false,
		IgnoreRelativePaths: nil,
		AllowRelativePaths:  nil,
	})
	if err != nil {
		t.Fatalf("ProjectSchemaPaths: %v", err)
	}

	assertProjectionLines(t, projection, []string{
		"projection-base:root [ancestor]",
		"  projection-base:iface [ancestor]",
		"    projection-base:name [key]",
		"    projection-base:config [ancestor]",
		"      projection-base:a [selected]",
		"      projection-base:mtu [selected]",
	})
}

func TestProjectSchemaPathsAppliesRelativeIgnoreFilters(t *testing.T) {
	mod := loadProjectionModule(t, "projection-base", projectionBase)

	projection, err := cambium.ProjectSchemaPaths(mod, []string{
		"/pb:root/pb:iface",
	}, cambium.ProjectionOptions{
		FlattenChoices:      true,
		IncludeListKeys:     true,
		ListKeysFirst:       true,
		IncludeDescendants:  true,
		IgnoreRelativePaths: []string{"state", "config/z", "name"},
		AllowRelativePaths:  nil,
	})
	if err != nil {
		t.Fatalf("ProjectSchemaPaths: %v", err)
	}

	assertProjectionLines(t, projection, []string{
		"projection-base:root [ancestor]",
		"  projection-base:iface [selected]",
		"    projection-base:name [key]",
		"    projection-base:description [descendant]",
		"    projection-base:config [descendant]",
		"      projection-base:a [descendant]",
		"      projection-base:mtu [descendant]",
	})
}

func TestProjectSchemaPathsAppliesRelativeAllowFilters(t *testing.T) {
	mod := loadProjectionModule(t, "projection-base", projectionBase)

	projection, err := cambium.ProjectSchemaPaths(mod, []string{
		"/pb:root/pb:iface",
	}, cambium.ProjectionOptions{
		FlattenChoices:     true,
		IncludeListKeys:    true,
		ListKeysFirst:      true,
		IncludeDescendants: true,
		AllowRelativePaths: []string{"config/mtu", "config/a"},
	})
	if err != nil {
		t.Fatalf("ProjectSchemaPaths: %v", err)
	}

	assertProjectionLines(t, projection, []string{
		"projection-base:root [ancestor]",
		"  projection-base:iface [selected]",
		"    projection-base:name [key]",
		"    projection-base:config [descendant]",
		"      projection-base:a [descendant]",
		"      projection-base:mtu [descendant]",
	})
}

func TestProjectSchemaPathsReflectsChoiceAugmentDeviationOrder(t *testing.T) {
	base := `module projection-rewrite-base {
    yang-version 1.1;
    namespace "urn:projection-rewrite-base";
    prefix base;

    container top {
        leaf a { type string; }
        choice mode {
            case primary {
                leaf p { type string; }
            }
            case secondary {
                leaf s { type string; }
            }
        }
        leaf b { type string; }
    }
}`
	aug := `module projection-rewrite-augment {
    yang-version 1.1;
    namespace "urn:projection-rewrite-augment";
    prefix aug;

    import projection-rewrite-base { prefix base; }

    augment "/base:top" {
        leaf c { type string; }
    }
}`
	dev := `module projection-rewrite-dev {
    yang-version 1.1;
    namespace "urn:projection-rewrite-dev";
    prefix dev;

    import projection-rewrite-base { prefix base; }

    deviation "/base:top/base:b" {
        deviate not-supported;
    }
}`
	mod := loadProjectionModule(t, "projection-rewrite-base", base, aug, dev)

	projection, err := cambium.ProjectSchemaPaths(mod, []string{
		"/base:top",
	}, cambium.DefaultProjectionOptions())
	if err != nil {
		t.Fatalf("ProjectSchemaPaths: %v", err)
	}

	assertProjectionLines(t, projection, []string{
		"projection-rewrite-base:top [selected]",
		"  projection-rewrite-base:a [descendant]",
		"  projection-rewrite-base:p [descendant]",
		"  projection-rewrite-base:s [descendant]",
		"  projection-rewrite-augment:c [descendant]",
	})
}

func TestProjectSchemaPathsSelectsAugmentedSubtreeWithRelativeFilters(t *testing.T) {
	base := `module projection-augment-select-base {
    yang-version 1.1;
    namespace "urn:projection-augment-select-base";
    prefix base;

    container top {
        container native {
            leaf before { type string; }
        }
        leaf tail { type string; }
    }
}`
	aug := `module projection-augment-select-feature {
    yang-version 1.1;
    namespace "urn:projection-augment-select-feature";
    prefix feat;

    import projection-augment-select-base { prefix base; }

    augment "/base:top/base:native" {
        when "../base:tail = 'enabled'";
        container feature {
            leaf required {
                mandatory true;
                type string;
            }
            leaf optional { type string; }
        }
        leaf aug-leaf {
            type string;
            default "on";
        }
    }
}`
	mod := loadProjectionModule(t, "projection-augment-select-base", base, aug)

	projection, err := cambium.ProjectSchemaPaths(mod, []string{
		"/base:top/base:native",
	}, cambium.ProjectionOptions{
		FlattenChoices:     true,
		IncludeDescendants: true,
		IncludeDefaults:    true,
		IncludeMandatory:   true,
		AllowRelativePaths: []string{
			"feat:feature/feat:required",
			"feat:aug-leaf",
		},
	})
	if err != nil {
		t.Fatalf("ProjectSchemaPaths: %v", err)
	}

	assertProjectionLines(t, projection, []string{
		"projection-augment-select-base:top [ancestor]",
		"  projection-augment-select-base:native [selected]",
		"    projection-augment-select-feature:feature [descendant]",
		"      projection-augment-select-feature:required [descendant,mandatory]",
		"    projection-augment-select-feature:aug-leaf [descendant,default]",
	})
}

func TestProjectSchemaPathsFlattensAugmentedChoiceCasesInOrder(t *testing.T) {
	base := `module projection-choice-augment-base {
    yang-version 1.1;
    namespace "urn:projection-choice-augment-base";
    prefix base;

    container top {
        choice transport {
            case ethernet {
                leaf speed { type string; }
            }
        }
        leaf tail { type string; }
    }
}`
	aug := `module projection-choice-augment-feature {
    yang-version 1.1;
    namespace "urn:projection-choice-augment-feature";
    prefix feat;

    import projection-choice-augment-base { prefix base; }

    augment "/base:top/base:transport" {
        case optical {
            leaf wavelength {
                type string;
                default "1310";
            }
        }
    }
}`
	mod := loadProjectionModule(t, "projection-choice-augment-base", base, aug)

	projection, err := cambium.ProjectSchemaPaths(mod, []string{
		"/base:top",
	}, cambium.ProjectionOptions{
		FlattenChoices:     true,
		IncludeDescendants: true,
		IncludeDefaults:    true,
	})
	if err != nil {
		t.Fatalf("ProjectSchemaPaths: %v", err)
	}

	assertProjectionLines(t, projection, []string{
		"projection-choice-augment-base:top [selected]",
		"  projection-choice-augment-base:speed [descendant]",
		"  projection-choice-augment-feature:wavelength [descendant,default]",
		"  projection-choice-augment-base:tail [descendant]",
	})
}

func TestProjectSchemaPathsRetainsChoiceCaseSpineWhenNotFlattened(t *testing.T) {
	mod := loadProjectionModule(t, "projection-choice-spine", `module projection-choice-spine {
    namespace "urn:projection-choice-spine";
    prefix pcs;

    container top {
        choice mode {
            case primary {
                container settings {
                    leaf value { type string; }
                }
            }
        }
    }
}`)

	projection, err := cambium.ProjectSchemaPaths(mod, []string{
		"/pcs:top/pcs:mode/pcs:primary/pcs:settings/pcs:value",
	}, cambium.ProjectionOptions{
		FlattenChoices:     false,
		IncludeDescendants: false,
	})
	if err != nil {
		t.Fatalf("ProjectSchemaPaths: %v", err)
	}

	assertProjectionLines(t, projection, []string{
		"projection-choice-spine:top [ancestor]",
		"  projection-choice-spine:mode [ancestor]",
		"    projection-choice-spine:primary [ancestor]",
		"      projection-choice-spine:settings [ancestor]",
		"        projection-choice-spine:value [selected]",
	})
}

func TestProjectSchemaPathsIncludesSubmoduleAndMultipleAugments(t *testing.T) {
	base := `module projection-submodule-augment-base {
    namespace "urn:projection-submodule-augment-base";
    prefix base;
    yang-version 1.1;

    include projection-submodule-augment-part;

    container system {
        leaf name { type string; }
    }
}`
	part := `submodule projection-submodule-augment-part {
    yang-version 1.1;

    belongs-to projection-submodule-augment-base {
        prefix base;
    }

    container protocols {
        list protocol {
            key "name";
            leaf name { type string; }
            container config {
                leaf enabled { type boolean; }
            }
        }
    }
}`
	ospf := `module projection-submodule-augment-ospf {
    namespace "urn:projection-submodule-augment-ospf";
    prefix ospf;
    yang-version 1.1;

    import projection-submodule-augment-base { prefix base; }

    grouping metric-policy {
        leaf metric-style { type string; }
    }

    augment "/base:protocols/base:protocol/base:config" {
        when "../base:name = 'ospf'";
        leaf area { type string; }
        uses metric-policy;
    }
}`
	isis := `module projection-submodule-augment-isis {
    namespace "urn:projection-submodule-augment-isis";
    prefix isis;
    yang-version 1.1;

    import projection-submodule-augment-base { prefix base; }

    augment "/base:protocols/base:protocol/base:config" {
        when "../base:name = 'isis'";
        leaf level { type uint8; }
        container timers {
            leaf hello-interval { type uint16; }
        }
    }
}`
	dir := t.TempDir()
	writeModuleFile(t, filepath.Join(dir, "projection-submodule-augment-base.yang"), []byte(base))
	writeModuleFile(t, filepath.Join(dir, "projection-submodule-augment-part.yang"), []byte(part))
	writeModuleFile(t, filepath.Join(dir, "projection-submodule-augment-ospf.yang"), []byte(ospf))
	writeModuleFile(t, filepath.Join(dir, "projection-submodule-augment-isis.yang"), []byte(isis))
	builder, err := cambium.NewContextBuilder(cambium.ContextFlags{})
	if err != nil {
		t.Fatal(err)
	}
	if err := builder.SearchPath(dir); err != nil {
		t.Fatalf("SearchPath: %v", err)
	}
	for _, name := range []string{
		"projection-submodule-augment-base",
		"projection-submodule-augment-ospf",
		"projection-submodule-augment-isis",
	} {
		if err := builder.LoadModule(name, nil, nil); err != nil {
			t.Fatalf("LoadModule %s: %v", name, err)
		}
	}
	ctx, err := builder.Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	t.Cleanup(func() { ctx.Close() })
	mod, err := ctx.Schema("projection-submodule-augment-base")
	if err != nil {
		t.Fatalf("Schema projection-submodule-augment-base: %v", err)
	}

	projection, err := cambium.ProjectSchemaPaths(mod, []string{
		"/base:protocols/base:protocol/base:config",
	}, cambium.DefaultProjectionOptions())
	if err != nil {
		t.Fatalf("ProjectSchemaPaths: %v", err)
	}

	assertProjectionLines(t, projection, []string{
		"projection-submodule-augment-base:protocols [ancestor]",
		"  projection-submodule-augment-base:protocol [ancestor]",
		"    projection-submodule-augment-base:name [key]",
		"    projection-submodule-augment-base:config [selected]",
		"      projection-submodule-augment-base:enabled [descendant]",
		"      projection-submodule-augment-ospf:area [descendant]",
		"      projection-submodule-augment-ospf:metric-style [descendant]",
		"      projection-submodule-augment-isis:level [descendant]",
		"      projection-submodule-augment-isis:timers [descendant]",
		"        projection-submodule-augment-isis:hello-interval [descendant]",
	})
}

func TestProjectSchemaPathsReflectsDeviationRoleChanges(t *testing.T) {
	base := `module projection-deviation-base {
    yang-version 1.1;
    namespace "urn:projection-deviation-base";
    prefix base;

    container top {
        leaf removable { type string; }
        leaf add-default { type string; }
        leaf replace-default {
            type string;
            default "old";
        }
        leaf delete-default {
            type string;
            default "gone";
        }
        leaf add-mandatory { type string; }
        leaf was-required {
            mandatory true;
            type string;
        }
        list add-min-list {
            key "name";
            leaf name { type string; }
            leaf value { type string; }
        }
        list delete-min-list {
            key "name";
            min-elements 1;
            leaf name { type string; }
        }
        leaf-list add-min-tag { type string; }
    }
}`
	dev := `module projection-deviation-source {
    yang-version 1.1;
    namespace "urn:projection-deviation-source";
    prefix dev;

    import projection-deviation-base { prefix base; }

    deviation "/base:top/base:removable" {
        deviate not-supported;
    }
    deviation "/base:top/base:add-default" {
        deviate add {
            default "added";
        }
    }
    deviation "/base:top/base:replace-default" {
        deviate replace {
            default "new";
        }
    }
    deviation "/base:top/base:delete-default" {
        deviate delete {
            default "gone";
        }
    }
    deviation "/base:top/base:add-mandatory" {
        deviate add {
            mandatory true;
        }
    }
    deviation "/base:top/base:was-required" {
        deviate replace {
            mandatory false;
        }
    }
    deviation "/base:top/base:add-min-list" {
        deviate add {
            min-elements 1;
        }
    }
    deviation "/base:top/base:delete-min-list" {
        deviate delete {
            min-elements 1;
        }
    }
    deviation "/base:top/base:add-min-tag" {
        deviate add {
            min-elements 1;
        }
    }
}`
	mod := loadProjectionModule(t, "projection-deviation-base", base, dev)

	projection, err := cambium.ProjectSchemaPaths(mod, []string{
		"/base:top",
	}, cambium.ProjectionOptions{
		FlattenChoices:     true,
		IncludeDefaults:    true,
		IncludeMandatory:   true,
		IncludeListKeys:    true,
		ListKeysFirst:      true,
		IncludeDescendants: false,
	})
	if err != nil {
		t.Fatalf("ProjectSchemaPaths: %v", err)
	}

	assertProjectionLines(t, projection, []string{
		"projection-deviation-base:top [selected]",
		"  projection-deviation-base:add-default [default]",
		"  projection-deviation-base:replace-default [default]",
		"  projection-deviation-base:add-mandatory [mandatory]",
		"  projection-deviation-base:add-min-list [mandatory]",
		"    projection-deviation-base:name [key]",
		"  projection-deviation-base:add-min-tag [mandatory]",
	})
}

func TestSchemaNodeRefDataAncestorsSkipChoiceCase(t *testing.T) {
	mod := loadProjectionModule(t, "projection-choice-parent", `module projection-choice-parent {
    namespace "urn:projection-choice-parent";
    prefix pcp;

    container top {
        choice mode {
            case primary {
                container settings {
                    leaf value { type string; }
                }
            }
        }
    }
}`)
	value, err := mod.FindPath("/pcp:top/pcp:mode/pcp:primary/pcp:settings/pcp:value")
	if err != nil {
		t.Fatalf("FindPath value: %v", err)
	}
	ancestors := value.DataAncestors()
	var got []string
	for _, ancestor := range ancestors {
		got = append(got, ancestor.Name())
	}
	if want := []string{"top", "settings"}; strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("DataAncestors = %v, want %v", got, want)
	}
}

func TestProjectSchemaPathsRejectsInvalidRelativeFilterByDefault(t *testing.T) {
	mod := loadProjectionModule(t, "projection-base", projectionBase)

	_, err := cambium.ProjectSchemaPaths(mod, []string{"/pb:root/pb:iface"}, cambium.ProjectionOptions{
		IncludeDescendants:  true,
		IgnoreRelativePaths: []string{"config/missing"},
	})
	if err == nil || !strings.Contains(err.Error(), "filter path") {
		t.Fatalf("ProjectSchemaPaths invalid filter error = %v, want filter path error", err)
	}
}

func TestProjectSubtreeAndWalkPreOrder(t *testing.T) {
	mod := loadProjectionModule(t, "projection-base", projectionBase)
	iface, err := mod.FindPath("/pb:root/pb:iface")
	if err != nil {
		t.Fatalf("FindPath iface: %v", err)
	}

	subtree, err := cambium.ProjectSubtree(iface, cambium.ProjectionOptions{
		FlattenChoices:      true,
		IncludeListKeys:     true,
		ListKeysFirst:       true,
		IncludeDescendants:  true,
		AllowRelativePaths:  []string{"config"},
		IgnoreRelativePaths: []string{"config/z"},
	})
	if err != nil {
		t.Fatalf("ProjectSubtree: %v", err)
	}
	projection := cambium.Projection{Roots: []cambium.ProjectedNode{subtree}}
	assertProjectionLines(t, projection, []string{
		"projection-base:iface [selected]",
		"  projection-base:name [key]",
		"  projection-base:config [descendant]",
		"    projection-base:a [descendant]",
		"    projection-base:mtu [descendant]",
	})

	var walked []string
	projection.WalkPreOrder(func(node cambium.ProjectedNode) bool {
		walked = append(walked, node.Node.Name())
		return true
	})
	if got, want := strings.Join(walked, ","), "iface,name,config,a,mtu"; got != want {
		t.Fatalf("WalkPreOrder = %s, want %s", got, want)
	}
}

const projectionRequirement = `module projection-requirement {
    yang-version 1.1;
    namespace "urn:projection-requirement";
    prefix pr;

    container top {
        container settings {
            leaf optional { type string; }
            leaf required {
                mandatory true;
                type string;
            }
            leaf fallback {
                type string;
                default "auto";
            }
            container nested {
                leaf nested-required {
                    mandatory true;
                    type string;
                }
                leaf nested-default {
                    type string;
                    default "nested";
                }
            }
        }
    }
}`

func TestProjectSchemaPathsIncludesMandatoryWhenRequested(t *testing.T) {
	mod := loadProjectionModule(t, "projection-requirement", projectionRequirement)

	projection, err := cambium.ProjectSchemaPaths(mod, []string{
		"/pr:top/pr:settings",
	}, cambium.ProjectionOptions{
		FlattenChoices:     true,
		IncludeMandatory:   true,
		IncludeDescendants: false,
	})
	if err != nil {
		t.Fatalf("ProjectSchemaPaths: %v", err)
	}

	assertProjectionLines(t, projection, []string{
		"projection-requirement:top [ancestor]",
		"  projection-requirement:settings [selected]",
		"    projection-requirement:required [mandatory]",
		"    projection-requirement:nested [descendant]",
		"      projection-requirement:nested-required [mandatory]",
	})
}

func TestProjectSchemaPathsIncludesDefaultsOnlyWhenRequested(t *testing.T) {
	mod := loadProjectionModule(t, "projection-requirement", projectionRequirement)

	withoutDefaults, err := cambium.ProjectSchemaPaths(mod, []string{
		"/pr:top/pr:settings",
	}, cambium.ProjectionOptions{
		FlattenChoices:     true,
		IncludeMandatory:   true,
		IncludeDescendants: false,
	})
	if err != nil {
		t.Fatalf("ProjectSchemaPaths without defaults: %v", err)
	}
	assertProjectionLines(t, withoutDefaults, []string{
		"projection-requirement:top [ancestor]",
		"  projection-requirement:settings [selected]",
		"    projection-requirement:required [mandatory]",
		"    projection-requirement:nested [descendant]",
		"      projection-requirement:nested-required [mandatory]",
	})

	withDefaults, err := cambium.ProjectSchemaPaths(mod, []string{
		"/pr:top/pr:settings",
	}, cambium.ProjectionOptions{
		FlattenChoices:     true,
		IncludeMandatory:   true,
		IncludeDefaults:    true,
		IncludeDescendants: false,
	})
	if err != nil {
		t.Fatalf("ProjectSchemaPaths with defaults: %v", err)
	}
	assertProjectionLines(t, withDefaults, []string{
		"projection-requirement:top [ancestor]",
		"  projection-requirement:settings [selected]",
		"    projection-requirement:required [mandatory]",
		"    projection-requirement:fallback [default]",
		"    projection-requirement:nested [descendant]",
		"      projection-requirement:nested-required [mandatory]",
		"      projection-requirement:nested-default [default]",
	})
}

func TestProjectSchemaPathsProtectsMandatoryFromFilters(t *testing.T) {
	mod := loadProjectionModule(t, "projection-requirement", projectionRequirement)

	projection, err := cambium.ProjectSchemaPaths(mod, []string{
		"/pr:top/pr:settings",
	}, cambium.ProjectionOptions{
		FlattenChoices:      true,
		IncludeMandatory:    true,
		ProtectMandatory:    true,
		IncludeDefaults:     true,
		IncludeDescendants:  false,
		IgnoreRelativePaths: []string{"required", "nested/nested-required", "fallback"},
	})
	if err != nil {
		t.Fatalf("ProjectSchemaPaths: %v", err)
	}

	assertProjectionLines(t, projection, []string{
		"projection-requirement:top [ancestor]",
		"  projection-requirement:settings [selected]",
		"    projection-requirement:required [mandatory]",
		"    projection-requirement:nested [descendant]",
		"      projection-requirement:nested-required [mandatory]",
		"      projection-requirement:nested-default [default]",
	})
}

func TestProjectSchemaPathsProtectsMandatoryFromAllowFilters(t *testing.T) {
	mod := loadProjectionModule(t, "projection-requirement", projectionRequirement)

	projection, err := cambium.ProjectSchemaPaths(mod, []string{
		"/pr:top/pr:settings",
	}, cambium.ProjectionOptions{
		FlattenChoices:     true,
		IncludeMandatory:   true,
		ProtectMandatory:   true,
		IncludeDefaults:    true,
		IncludeDescendants: false,
		AllowRelativePaths: []string{"fallback"},
	})
	if err != nil {
		t.Fatalf("ProjectSchemaPaths: %v", err)
	}

	assertProjectionLines(t, projection, []string{
		"projection-requirement:top [ancestor]",
		"  projection-requirement:settings [selected]",
		"    projection-requirement:required [mandatory]",
		"    projection-requirement:fallback [default]",
		"    projection-requirement:nested [descendant]",
		"      projection-requirement:nested-required [mandatory]",
	})
}

func TestProjectSchemaPathsMarksSelectedKey(t *testing.T) {
	mod := loadProjectionModule(t, "projection-base", projectionBase)

	projection, err := cambium.ProjectSchemaPaths(mod, []string{
		"/pb:root/pb:iface/pb:name",
	}, cambium.ProjectionOptions{
		FlattenChoices:     true,
		IncludeListKeys:    true,
		ListKeysFirst:      true,
		IncludeDescendants: false,
	})
	if err != nil {
		t.Fatalf("ProjectSchemaPaths: %v", err)
	}

	assertProjectionLines(t, projection, []string{
		"projection-base:root [ancestor]",
		"  projection-base:iface [ancestor]",
		"    projection-base:name [selected,key]",
	})
}

func TestProjectSchemaPathsTreatsPositiveMinElementsAsMandatory(t *testing.T) {
	mod := loadProjectionModule(t, "projection-cardinality", `module projection-cardinality {
    yang-version 1.1;
    namespace "urn:projection-cardinality";
    prefix pc;

    container top {
        list required-list {
            key "name";
            min-elements 1;
            leaf name { type string; }
            leaf value { type string; }
        }
        leaf-list required-tag {
            min-elements 1;
            type string;
        }
    }
}`)

	projection, err := cambium.ProjectSchemaPaths(mod, []string{
		"/pc:top",
	}, cambium.ProjectionOptions{
		FlattenChoices:     true,
		IncludeListKeys:    true,
		ListKeysFirst:      true,
		IncludeMandatory:   true,
		IncludeDescendants: false,
	})
	if err != nil {
		t.Fatalf("ProjectSchemaPaths: %v", err)
	}

	assertProjectionLines(t, projection, []string{
		"projection-cardinality:top [selected]",
		"  projection-cardinality:required-list [mandatory]",
		"    projection-cardinality:name [key]",
		"  projection-cardinality:required-tag [mandatory]",
	})
}
