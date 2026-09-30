// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

package compat_test

import (
	"testing"

	"github.com/signalbreak-labs/cambium/go/compat"
)

func TestNativeProjectionInstanceIdentifierRequireInstance(t *testing.T) {
	ctx := nativeProjectionContext(t, false, map[string]string{
		"instances": `module instances {
  yang-version 1.1; namespace "urn:instances"; prefix i;
  typedef optional { type instance-identifier { require-instance false; } }
  typedef required { type instance-identifier; }
  leaf direct { type instance-identifier { require-instance false; } }
  leaf inherited { type optional; }
  leaf refined { type required { require-instance false; } }
  leaf mandatory-instance { type optional { require-instance true; } }
  leaf implicit { type instance-identifier; }
  leaf nested { type union { type union { type optional; type required; } type uint8; } }
}`,
	}, "instances")
	roots, err := compat.FromContext(ctx)
	if err != nil {
		t.Fatal(err)
	}
	mod, _ := ctx.Schema("instances")
	for _, root := range []*compat.Entry{projectionRoot(t, roots, "instances"), compat.FromModule(mod)} {
		for name, want := range map[string]bool{
			"direct": true, "inherited": true, "refined": true,
			"mandatory-instance": false, "implicit": false,
		} {
			if typ := root.Lookup(name).Type; typ.Kind != compat.YinstanceIdentifier || typ.OptionalInstance != want {
				t.Errorf("%s type = %+v, want instance-identifier OptionalInstance=%v", name, typ, want)
			}
		}
		members := root.Lookup("nested").Type.Type[0].Type
		for i, want := range []bool{true, false} {
			if typ := members[i]; typ.Kind != compat.YinstanceIdentifier || typ.OptionalInstance != want {
				t.Errorf("nested member %d type = %+v, want OptionalInstance=%v", i, typ, want)
			}
		}
	}
}
