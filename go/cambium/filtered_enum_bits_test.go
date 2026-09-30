// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

package cambium_test

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/signalbreak-labs/cambium/go/cambium"
)

func TestDerivedEnumBitsFeatureFiltering(t *testing.T) {
	for _, tc := range []struct {
		name    string
		retain  bool
		enabled bool
	}{
		{name: "disabled"},
		{name: "retained", retain: true},
		{name: "enabled", enabled: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b := retentionBuilder(t, tc.retain)
			if err := b.LoadModuleStr(`module filtered-types {
  yang-version 1.1; namespace "urn:filtered-types"; prefix f;
  feature available;
  typedef enum-base { type enumeration { enum allowed { value -7; } enum forbidden { value 99; } } }
  typedef bit-base { type bits { bit allowed { position 17; } bit forbidden { position 99; } } }
  typedef enum-filtered { type enum-base { enum allowed { if-feature available; } } }
  typedef bit-filtered { type bit-base { bit allowed { if-feature available; } } }
  typedef enum-alias { type enum-filtered; }
  typedef bit-alias { type bit-filtered; }
  leaf enums { type enum-alias; }
  leaf bits { type bit-alias; }
  leaf unioned { type union { type union { type enum-filtered; type bit-filtered; } type uint8; } }
}`); err != nil {
				t.Fatal(err)
			}
			if tc.enabled {
				if err := b.SetFeatures("filtered-types", []string{"available"}); err != nil {
					t.Fatal(err)
				}
			}
			ctx, err := b.Build()
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(ctx.Close)
			mod, _ := ctx.Schema("filtered-types")
			union := leafType(t, mod, "/f:unioned").Resolved().(cambium.ResolvedUnion).Members()[0].Resolved().(cambium.ResolvedUnion)
			members := union.Members()
			for i, values := range [][]cambium.EnumValue{
				leafType(t, mod, "/f:enums").Resolved().(cambium.ResolvedEnumeration).Values(),
				leafType(t, mod, "/f:bits").Resolved().(cambium.ResolvedBits).Values(),
				members[0].Resolved().(cambium.ResolvedEnumeration).Values(),
				members[1].Resolved().(cambium.ResolvedBits).Values(),
			} {
				var want []string
				if tc.retain || tc.enabled {
					want = []string{"allowed=-7"}
					if i%2 == 1 {
						want = []string{"allowed=17"}
					}
				}
				if got := enumValuesForLog(values); !slices.Equal(got, want) {
					t.Errorf("type %d values = %v, want %v", i, got, want)
				}
			}
		})
	}
}

func TestDerivedEnumBitsKeepDisabledParentRestrictions(t *testing.T) {
	mod := s3Module(t, `module filtered-parent {
  yang-version 1.1; namespace "urn:filtered-parent"; prefix f;
  feature available;
  typedef enum-base { type enumeration { enum allowed { if-feature available; value -7; } } }
  typedef bit-base { type bits { bit allowed { if-feature available; position 17; } } }
  typedef enum-filtered { type enum-base { enum allowed { value -7; } } }
  typedef bit-filtered { type bit-base { bit allowed { position 17; } } }
  leaf enums { type enum-filtered { enum allowed; } }
  leaf bits { type bit-filtered { bit allowed; } }
}`)
	if got := leafType(t, mod, "/f:enums").Resolved().(cambium.ResolvedEnumeration).Values(); len(got) != 0 {
		t.Errorf("disabled parent enum restored: %v", enumValuesForLog(got))
	}
	if got := leafType(t, mod, "/f:bits").Resolved().(cambium.ResolvedBits).Values(); len(got) != 0 {
		t.Errorf("disabled parent bit restored: %v", enumValuesForLog(got))
	}
}

func TestDerivedEnumBitsValidateDisabledRestrictions(t *testing.T) {
	for _, tc := range []struct {
		name, base, restriction, want string
	}{
		{"enum value", "enumeration { enum allowed { value -7; } }", "enum allowed { if-feature available; value 3; }", "want base value -7"},
		{"bit position", "bits { bit allowed { position 17; } }", "bit allowed { if-feature available; position 3; }", "want base value 17"},
		{"disabled parent value", "enumeration { enum allowed { if-feature available; value -7; } }", "enum allowed { if-feature available; value 3; }", "want base value -7"},
		{"enum name", "enumeration { enum allowed; }", "enum missing { if-feature available; }", "does not exist in base type"},
		{"bit name", "bits { bit allowed; }", "bit missing { if-feature available; }", "does not exist in base type"},
		{"duplicate enum", "enumeration { enum allowed; }", "enum allowed { if-feature available; } enum allowed { if-feature available; }", "duplicate enum name"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b := retentionBuilder(t, false)
			source := fmt.Sprintf(`module invalid-filter {
  yang-version 1.1; namespace "urn:invalid-filter"; prefix f; feature available;
  typedef parent { type %s }
  typedef child { type parent { %s } }
  leaf unioned { type union { type child; type uint8; } }
}`, tc.base, tc.restriction)
			if err := b.LoadModuleStr(source); err != nil {
				t.Fatal(err)
			}
			ctx, err := b.Build()
			if ctx != nil {
				t.Cleanup(ctx.Close)
			}
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("Build = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestDerivedEnumBitsRejectFilteredDefaults(t *testing.T) {
	for _, tc := range []struct {
		name, definition string
	}{
		{"enum leaf", `leaf value { type enum-filtered; default forbidden; }`},
		{"bits leaf", `leaf value { type bit-filtered; default forbidden; }`},
		{"enum leaf-list", `leaf-list value { type enum-filtered; default forbidden; }`},
		{"enum typedef", `typedef bad { type enum-filtered; default forbidden; }`},
		{"enum inherited", `typedef bad { type enum-base; default forbidden; } leaf value { type bad { enum allowed { if-feature available; } } }`},
		{"enum union", `leaf value { type union { type enum-filtered; type uint8; } default forbidden; }`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b := retentionBuilder(t, false)
			source := `module filtered-defaults {
  yang-version 1.1; namespace "urn:filtered-defaults"; prefix f; feature available;
  typedef enum-base { type enumeration { enum allowed; enum forbidden; } }
  typedef bit-base { type bits { bit allowed; bit forbidden; } }
  typedef enum-filtered { type enum-base { enum allowed { if-feature available; } } }
  typedef bit-filtered { type bit-base { bit allowed { if-feature available; } } }
` + tc.definition + `}`
			if err := b.LoadModuleStr(source); err != nil {
				t.Fatal(err)
			}
			ctx, err := b.Build()
			if ctx != nil {
				t.Cleanup(ctx.Close)
			}
			if err == nil || !strings.Contains(err.Error(), `default "forbidden"`) {
				t.Errorf("Build = %v, want forbidden default rejection", err)
			}
		})
	}
	mod := s3Module(t, `module empty-bits-default {
  yang-version 1.1; namespace "urn:empty-bits-default"; prefix f; feature available;
  typedef parent { type bits { bit allowed; bit forbidden; } }
  leaf value { type parent { bit allowed { if-feature available; } } default ""; }
}`)
	if got := schemaNodeAt(t, mod, "/f:value").DefaultValues(); !slices.Equal(got, []string{""}) {
		t.Fatalf("empty bits default = %v, want one empty value", got)
	}
}
