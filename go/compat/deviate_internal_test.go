// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

package compat

import (
	"reflect"
	"testing"

	upstream "github.com/signalbreak-labs/cambium/go/internal/yangparse/upstream/yang"
)

func TestApplyDeviateReplaceMatchesGoyang(t *testing.T) {
	const source = `module compat-apply-deviate-replace {
  yang-version 1.1;
  namespace "urn:compat-apply-deviate-replace";
  prefix cadr;

  container top {
    leaf value {
      type uint32;
      default "7";
      units "old-units";
      mandatory false;
    }
  }

  deviation "/cadr:top/cadr:value" {
    deviate replace {
      config false;
      default "new";
      mandatory true;
      units "widgets";
      type string;
    }
  }
}
`
	compatRoot, upstreamRoot := rawCompatAndUpstreamEntries(t, "compat-apply-deviate-replace", source)
	compatValue := compatRoot.Find("/cadr:top/cadr:value")
	upstreamValue := upstreamRoot.Find("/cadr:top/cadr:value")
	if compatValue == nil || upstreamValue == nil {
		t.Fatalf("fixture value entries = (%#v,%#v), want both non-nil", compatValue, upstreamValue)
	}

	compatErrs := compatRoot.ApplyDeviate()
	upstreamErrs := upstreamRoot.ApplyDeviate()
	if len(compatErrs) != len(upstreamErrs) {
		t.Fatalf("ApplyDeviate errors = %v, want goyang %v", compatErrs, upstreamErrs)
	}
	if got, want := compatValue.Config.String(), upstreamValue.Config.String(); got != want {
		t.Fatalf("Config = %q, want goyang %q", got, want)
	}
	if !reflect.DeepEqual(compatValue.Default, upstreamValue.Default) {
		t.Fatalf("Default = %v, want goyang %v", compatValue.Default, upstreamValue.Default)
	}
	if got, want := compatValue.Mandatory.String(), upstreamValue.Mandatory.String(); got != want {
		t.Fatalf("Mandatory = %q, want goyang %q", got, want)
	}
	if got, want := compatValue.Units, upstreamValue.Units; got != want {
		t.Fatalf("Units = %q, want goyang %q", got, want)
	}
	if compatValue.Type == nil || upstreamValue.Type == nil || compatValue.Type.Name != upstreamValue.Type.Name || compatValue.Type.Kind.String() != upstreamValue.Type.Kind.String() {
		t.Fatalf("Type = %#v, want goyang %#v", compatValue.Type, upstreamValue.Type)
	}
}

func TestApplyDeviateDeleteMatchesGoyang(t *testing.T) {
	const source = `module compat-apply-deviate-delete {
  yang-version 1.1;
  namespace "urn:compat-apply-deviate-delete";
  prefix cadd;

  container top {
    leaf value {
      type string;
      config false;
      mandatory true;
      default "old";
    }
    list item {
      key "name";
      min-elements 2;
      max-elements 4;
      leaf name { type string; }
    }
    leaf-list tags {
      type string;
      default "a";
    }
  }

  deviation "/cadd:top/cadd:value" {
    deviate delete {
      config false;
      mandatory true;
      default "old";
    }
  }

  deviation "/cadd:top/cadd:item" {
    deviate delete {
      min-elements 2;
      max-elements 4;
    }
  }

  deviation "/cadd:top/cadd:tags" {
    deviate delete {
      default "a";
    }
  }
}
`
	compatRoot, upstreamRoot := rawCompatAndUpstreamEntries(t, "compat-apply-deviate-delete", source)
	compatValue := compatRoot.Find("/cadd:top/cadd:value")
	upstreamValue := upstreamRoot.Find("/cadd:top/cadd:value")
	compatItem := compatRoot.Find("/cadd:top/cadd:item")
	upstreamItem := upstreamRoot.Find("/cadd:top/cadd:item")
	if compatValue == nil || upstreamValue == nil || compatItem == nil || upstreamItem == nil {
		t.Fatalf("fixture entries = (%#v,%#v,%#v,%#v), want all non-nil", compatValue, upstreamValue, compatItem, upstreamItem)
	}

	compatErrs := deviationErrorStrings(compatRoot.ApplyDeviate())
	upstreamErrs := deviationErrorStrings(upstreamRoot.ApplyDeviate())
	if !reflect.DeepEqual(compatErrs, upstreamErrs) {
		t.Fatalf("ApplyDeviate delete errors = %v, want goyang %v", compatErrs, upstreamErrs)
	}
	if !reflect.DeepEqual(compatValue.Default, upstreamValue.Default) {
		t.Fatalf("deleted default = %v, want goyang %v", compatValue.Default, upstreamValue.Default)
	}
	if got, want := compatValue.Config.String(), upstreamValue.Config.String(); got != want {
		t.Fatalf("deleted config = %q, want goyang %q", got, want)
	}
	if got, want := compatValue.Mandatory.String(), upstreamValue.Mandatory.String(); got != want {
		t.Fatalf("deleted mandatory = %q, want goyang %q", got, want)
	}
	if compatItem.ListAttr == nil || upstreamItem.ListAttr == nil {
		t.Fatalf("list attrs = (%#v,%#v), want both non-nil", compatItem.ListAttr, upstreamItem.ListAttr)
	}
	if got, want := compatItem.ListAttr.MinElements, upstreamItem.ListAttr.MinElements; got != want {
		t.Fatalf("deleted min-elements = %d, want goyang %d", got, want)
	}
	if got, want := compatItem.ListAttr.MaxElements, upstreamItem.ListAttr.MaxElements; got != want {
		t.Fatalf("deleted max-elements = %d, want goyang %d", got, want)
	}
}

func TestApplyDeviateNotSupportedMatchesGoyangAndMaintainsOrder(t *testing.T) {
	const source = `module compat-apply-deviate-not-supported {
  yang-version 1.1;
  namespace "urn:compat-apply-deviate-not-supported";
  prefix cadn;

  container top {
    leaf before { type string; }
    leaf value { type string; }
    leaf after { type string; }
  }

  deviation "/cadn:top/cadn:value" {
    deviate not-supported;
  }
}
`
	compatRoot, upstreamRoot := rawCompatAndUpstreamEntries(t, "compat-apply-deviate-not-supported", source)

	compatErrs := compatRoot.ApplyDeviate()
	upstreamErrs := upstreamRoot.ApplyDeviate()
	if len(compatErrs) != len(upstreamErrs) {
		t.Fatalf("ApplyDeviate errors = %v, want goyang %v", compatErrs, upstreamErrs)
	}
	compatTop := compatRoot.Find("/cadn:top")
	upstreamTop := upstreamRoot.Find("/cadn:top")
	if compatTop == nil || upstreamTop == nil {
		t.Fatalf("fixture top entries = (%#v,%#v), want both non-nil", compatTop, upstreamTop)
	}
	if (compatTop.Lookup("value") == nil) != (upstreamTop.Dir["value"] == nil) {
		t.Fatalf("deleted child present = %v, want goyang %v", compatTop.Lookup("value") != nil, upstreamTop.Dir["value"] != nil)
	}
	var orderedNames []string
	for _, child := range compatTop.Children() {
		orderedNames = append(orderedNames, child.Name)
	}
	if !reflect.DeepEqual(orderedNames, []string{"before", "after"}) {
		t.Fatalf("ordered children after not-supported = %v, want [before after]", orderedNames)
	}
}

func rawCompatAndUpstreamEntries(t *testing.T, moduleName, source string) (*Entry, *upstream.Entry) {
	t.Helper()
	compatModules := NewModules()
	if err := compatModules.Parse(source, moduleName+".yang"); err != nil {
		t.Fatalf("compat Parse: %v", err)
	}
	compatModule := compatModules.Modules[moduleName]
	if compatModule == nil {
		t.Fatalf("compat parsed module %q not found", moduleName)
	}

	upstreamModules := upstream.NewModules()
	if err := upstreamModules.Parse(source, moduleName+".yang"); err != nil {
		t.Fatalf("upstream Parse: %v", err)
	}
	rawModule := upstreamModules.Modules[moduleName]
	if rawModule == nil {
		t.Fatalf("upstream parsed module %q not found", moduleName)
	}
	return ToEntry(compatModule), upstream.ToEntry(rawModule)
}

func deviationErrorStrings(errs []error) []string {
	out := make([]string, 0, len(errs))
	for _, err := range errs {
		if err == nil {
			out = append(out, "")
			continue
		}
		out = append(out, err.Error())
	}
	return out
}
