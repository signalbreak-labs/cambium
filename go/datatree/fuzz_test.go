// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

package datatree_test

import (
	"testing"

	"github.com/signalbreak-labs/cambium/go/cambium"
	"github.com/signalbreak-labs/cambium/go/datatree"
)

func FuzzParseJSONIETF(f *testing.F) {
	b, err := cambium.NewContextBuilder(cambium.ContextFlags{})
	if err != nil {
		f.Fatal(err)
	}
	if err := b.LoadModuleStr(dtSchema); err != nil {
		f.Fatalf("LoadModuleStr: %v", err)
	}
	ctx, err := b.Build()
	if err != nil {
		f.Fatalf("Build: %v", err)
	}
	mod, err := ctx.Schema("dt")
	if err != nil {
		f.Fatalf("Schema: %v", err)
	}

	f.Add(`{"dt:c":{"a":"1","z":"2"}}`)
	f.Add(`{}`)
	f.Add(`[`)

	f.Fuzz(func(t *testing.T, doc string) {
		_, _ = datatree.Parse(mod, datatree.FormatJSONIETF, []byte(doc))
	})
}
