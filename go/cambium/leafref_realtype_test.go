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

func TestLeafrefRealtypeIndependentOfDeclarationOrder(t *testing.T) {
	for _, tc := range []struct{ name, leaves string }{
		{"target first", `leaf value { type combined; }
  leaf middle { type leafref { path "../value"; } }
  leaf ref { type leafref { path "../middle"; } }`},
		{"reference first", `leaf ref { type leafref { path "../middle"; } }
  leaf middle { type leafref { path "../value"; } }
  leaf value { type combined; }`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := buildSchemaContextFromSources(t, fmt.Sprintf(`module chain {
  yang-version 1.1; namespace "urn:chain"; prefix c;
  identity first; identity second;
  identity shared { base first; base second; }
  typedef combined { type identityref { base first; base second; } }
  %s
}`, tc.leaves))
			mod, err := ctx.Schema("chain")
			if err != nil {
				t.Fatal(err)
			}
			info := leafType(t, mod, "/c:ref")
			info = requireLeafrefRealtype(t, info, "/chain/middle")
			info = requireLeafrefRealtype(t, info, "/chain/value")
			identity := resolvedMemberTypeFor[cambium.ResolvedIdentityRef](t, info)
			var bases []string
			for _, base := range identity.Bases() {
				bases = append(bases, base.Name())
			}
			if !slices.Equal(bases, []string{"first", "second"}) {
				t.Fatalf("terminal identity bases = %v, want first, second", bases)
			}
			if alias, ok := info.TypedefName(); !ok || alias != "combined" {
				t.Fatalf("terminal typedef = %q, %v, want combined", alias, ok)
			}
		})
	}
}

func TestLeafrefRealtypePreservesUnionConstraints(t *testing.T) {
	ctx := buildSchemaContextFromSources(t, `module union-chain {
  yang-version 1.1; namespace "urn:union-chain"; prefix u;
  identity first; identity second;
  identity shared { base first; base second; }
  leaf selector { type union {
    type leafref { path "../middle"; }
    type boolean;
  } }
  leaf middle { type leafref { path "../value"; } }
  leaf value { type union {
    type leafref { path "../kind"; }
    type int16 { range "-4..7"; }
    type string { length "2..5"; pattern "[a-z]+"; }
  } }
  leaf kind { type identityref { base first; base second; } }
}`)
	mod, err := ctx.Schema("union-chain")
	if err != nil {
		t.Fatal(err)
	}
	for range 2 {
		outer := resolvedTypeFor[cambium.ResolvedUnion](t, mod, "/u:selector").Members()
		if len(outer) != 2 || outer[0].Base() != cambium.BaseTypeLeafRef || outer[1].Base() != cambium.BaseTypeBoolean {
			t.Fatalf("outer union members = %v, want leafref, boolean", outer)
		}
		info := requireLeafrefRealtype(t, outer[0], "/union-chain/middle")
		info = requireLeafrefRealtype(t, info, "/union-chain/value")
		members := resolvedMemberTypeFor[cambium.ResolvedUnion](t, info).Members()
		var bases []cambium.BaseType
		for _, member := range members {
			bases = append(bases, member.Base())
		}
		if !slices.Equal(bases, []cambium.BaseType{cambium.BaseTypeLeafRef, cambium.BaseTypeInt16, cambium.BaseTypeString}) {
			t.Fatalf("target union order = %v, want leafref, int16, string", bases)
		}
		identityInfo := requireLeafrefRealtype(t, members[0], "/union-chain/kind")
		identity := resolvedMemberTypeFor[cambium.ResolvedIdentityRef](t, identityInfo)
		if identityBases := identity.Bases(); len(identityBases) != 2 || identityBases[0].Name() != "first" || identityBases[1].Name() != "second" {
			t.Fatalf("nested identity bases = %v, want first, second", identityBases)
		}
		number := resolvedMemberTypeFor[cambium.ResolvedInt](t, members[1])
		if len(number.Range) != 1 || number.Range[0].Min() != "-4" || number.Range[0].Max() != "7" {
			t.Fatalf("nested numeric range = %v, want -4..7", number.Range)
		}
		str := resolvedMemberTypeFor[cambium.ResolvedString](t, members[2])
		if len(str.Length) != 1 || str.Length[0].Min() != "2" || str.Length[0].Max() != "5" || len(str.Patterns) != 1 || str.Patterns[0].Regex() != "[a-z]+" {
			t.Fatalf("nested string constraints = %+v, want length 2..5 and [a-z]+", str)
		}
		// Returned metadata remains a defensive copy at every depth.
		number.Range[0] = cambium.RangeBound{}
		str.Length[0] = cambium.RangeBound{}
		str.Patterns[0] = cambium.Pattern{}
		members[0] = cambium.TypeInfo{}
		outer[0] = cambium.TypeInfo{}
	}
}

func TestLeafrefRealtypeChecksForwardChainDefaults(t *testing.T) {
	_, err := buildModeContext(t, cambium.ValidationStrict, nil, `module default-chain {
  yang-version 1.1; namespace "urn:default-chain"; prefix d;
  leaf ref { type leafref { path "../middle"; } default 9; }
  leaf middle { type leafref { path "../value"; } }
  leaf value { type int8 { range "1..2"; } }
}`)
	if err == nil || !strings.Contains(err.Error(), `default "9"`) || !strings.Contains(err.Error(), "int8") {
		t.Fatalf("Build error = %v, want invalid default outside terminal range", err)
	}
}

func TestLeafrefRealtypeVendorCycleRemainsFinite(t *testing.T) {
	ctx, err := buildModeContext(t, cambium.ValidationVendorCompatible, nil, `module cyclic-types {
  yang-version 1.1; namespace "urn:cyclic-types"; prefix c;
  leaf ref { type leafref { path "../middle"; } default sample; }
  leaf middle { type leafref { path "../ref"; } }
}`)
	if err != nil {
		t.Fatal(err)
	}
	if len(ctx.LoadReport().Warnings) == 0 {
		t.Fatal("vendor cycle must retain its warning")
	}
	mod, err := ctx.Schema("cyclic-types")
	if err != nil {
		t.Fatal(err)
	}
	info := leafType(t, mod, "/c:ref")
	for range 8 {
		ref := resolvedMemberTypeFor[cambium.ResolvedLeafRef](t, info)
		if _, ok := ref.Target(); !ok {
			t.Fatal("cycle lost its resolved target")
		}
		realtype, ok := ref.Realtype()
		if !ok {
			return
		}
		info = *realtype
	}
	t.Fatal("vendor-cycle Realtype snapshots do not terminate")
}

func requireLeafrefRealtype(t *testing.T, info cambium.TypeInfo, wantTarget string) cambium.TypeInfo {
	t.Helper()
	ref := resolvedMemberTypeFor[cambium.ResolvedLeafRef](t, info)
	target, ok := ref.Target()
	if !ok || target.Path() != wantTarget {
		t.Fatalf("leafref target = %q, %v, want %q", target.Path(), ok, wantTarget)
	}
	realtype, ok := ref.Realtype()
	if !ok || realtype == nil {
		t.Fatalf("leafref to %s has no Realtype", wantTarget)
	}
	return *realtype
}
