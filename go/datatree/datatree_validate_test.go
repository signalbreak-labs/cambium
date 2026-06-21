// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

package datatree_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/signalbreak-labs/cambium/go/cambium"
	"github.com/signalbreak-labs/cambium/go/datatree"
)

const dvSchema = `module dv {
    namespace "urn:dv";
    prefix dv;

    leaf req { type string; mandatory true; }
    leaf-list tags { type string; min-elements 1; }
    list item {
        key "id";
        leaf id { type string; }
        leaf name { type string; }
    }
}`

func loadDV(t *testing.T) cambium.Module {
	t.Helper()
	b, err := cambium.NewContextBuilder(cambium.ContextFlags{})
	if err != nil {
		t.Fatal(err)
	}
	if err := b.LoadModuleStr(dvSchema); err != nil {
		t.Fatalf("LoadModuleStr: %v", err)
	}
	ctx, err := b.Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	t.Cleanup(func() { ctx.Close() })
	mod, err := ctx.Schema("dv")
	if err != nil {
		t.Fatalf("Schema: %v", err)
	}
	return mod
}

func validateInput(t *testing.T, mod cambium.Module, in string) error {
	t.Helper()
	tree, err := datatree.Parse(mod, datatree.FormatJSONIETF, []byte(in))
	if err != nil {
		t.Fatalf("Parse(%s): %v", in, err)
	}
	return tree.Validate()
}

func TestValidateAcceptsWellFormed(t *testing.T) {
	mod := loadDV(t)
	in := `{"dv:req":"x","dv:tags":["a","b"],"dv:item":[{"id":"1"},{"id":"2"}]}`
	if err := validateInput(t, mod, in); err != nil {
		t.Fatalf("expected valid, got %v", err)
	}
}

func TestValidateViolations(t *testing.T) {
	mod := loadDV(t)
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"missing mandatory", `{"dv:tags":["a"]}`, "missing mandatory node /req"},
		{"min-elements", `{"dv:req":"x","dv:tags":[]}`, "min-elements"},
		{"duplicate key", `{"dv:req":"x","dv:tags":["a"],"dv:item":[{"id":"1"},{"id":"1"}]}`, "duplicate key"},
		{"missing key leaf", `{"dv:req":"x","dv:tags":["a"],"dv:item":[{"name":"n"}]}`, `missing key leaf "id"`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateInput(t, mod, tc.in)
			if err == nil {
				t.Fatalf("expected a violation, got nil")
			}
			var ve *datatree.ValidationError
			if !errors.As(err, &ve) {
				t.Fatalf("expected *datatree.ValidationError, got %T", err)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("violation %q not found in: %s", tc.want, err.Error())
			}
		})
	}
}
