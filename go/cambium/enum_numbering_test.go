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

func TestEnumBitsImplicitNumberingUsesHighestValue(t *testing.T) {
	for _, tc := range []struct{ kind, item, value string }{
		{"enumeration", "enum", "value"},
		{"bits", "bit", "position"},
	} {
		for _, retain := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/retain=%v", tc.kind, retain), func(t *testing.T) {
				b := retentionBuilder(t, retain)
				source := fmt.Sprintf(`module numbering {
  yang-version 1.1; namespace "urn:numbering"; prefix n; feature available;
  leaf value { type %s {
    %s high { %s 100; if-feature available; }
    %s low { %s 0; }
    %s implicit;
    %s lower { %s 50; }
    %s next;
  } }
}`, tc.kind, tc.item, tc.value, tc.item, tc.value, tc.item, tc.item, tc.value, tc.item)
				if err := b.LoadModuleStr(source); err != nil {
					t.Fatal(err)
				}
				ctx, err := b.Build()
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(ctx.Close)
				mod, _ := ctx.Schema("numbering")
				var values []cambium.EnumValue
				switch resolved := leafType(t, mod, "/n:value").Resolved().(type) {
				case cambium.ResolvedEnumeration:
					values = resolved.Values()
				case cambium.ResolvedBits:
					values = resolved.Values()
				}
				want := []string{"low=0", "implicit=101", "lower=50", "next=102"}
				if retain {
					want = append([]string{"high=100"}, want...)
				}
				if got := enumValuesForLog(values); !slices.Equal(got, want) {
					t.Errorf("values = %v, want %v", got, want)
				}
			})
		}
	}
}

func TestEnumImplicitNumberingStartsAtFirstAssignedValue(t *testing.T) {
	for _, tc := range []struct {
		name, enums string
		want        []string
	}{
		{"negative", `enum first { value -7; } enum next; enum lower { value -10; } enum last;`, []string{"first=-7", "next=-6", "lower=-10", "last=-5"}},
		{"zero", `enum first; enum lower { value -7; } enum next;`, []string{"first=0", "lower=-7", "next=1"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mod := s3Module(t, `module numbering {
  namespace "urn:numbering"; prefix n;
  leaf value { type enumeration { `+tc.enums+` } }
}`)
			values := leafType(t, mod, "/n:value").Resolved().(cambium.ResolvedEnumeration).Values()
			if got := enumValuesForLog(values); !slices.Equal(got, tc.want) {
				t.Errorf("values = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestEnumBitsImplicitNumberingOverflowAfterLowerValue(t *testing.T) {
	for _, tc := range []struct{ kind, item, value, maximum, want string }{
		{"enumeration", "enum", "value", "2147483647", "auto value 2147483648 outside int32 range"},
		{"bits", "bit", "position", "4294967295", "auto position 4294967296 outside uint32 range"},
	} {
		for _, gate := range []string{"", "if-feature available;"} {
			for _, retain := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/gated=%v/retain=%v", tc.kind, gate != "", retain), func(t *testing.T) {
					b := retentionBuilder(t, retain)
					source := fmt.Sprintf(`module numbering {
  yang-version 1.1; namespace "urn:numbering"; prefix n; feature available;
  leaf value { type %s { %s high { %s %s; %s } %s low { %s 0; } %s implicit; } }
}`, tc.kind, tc.item, tc.value, tc.maximum, gate, tc.item, tc.value, tc.item)
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
	}
}
