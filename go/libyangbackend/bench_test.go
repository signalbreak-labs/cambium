// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

//go:build cgo

package libyangbackend_test

import (
	"os"
	"path/filepath"
	"testing"

	cambium "github.com/signalbreak-labs/cambium/go/libyangbackend"
)

func benchmarkValidationContext(b *testing.B) *cambium.Context {
	b.Helper()
	dir := b.TempDir()
	module := filepath.Join(dir, "cambium-validation-demo.yang")
	src := `module cambium-validation-demo {
  namespace "urn:cambium:validation";
  prefix cvd;
  yang-version 1.1;
  revision 2026-06-14;

  container top {
    leaf name { type string; }
    leaf ref { type leafref { path "../name"; } }
    container c {
      leaf x {
        type uint8;
        must "../../name = 'open'" { error-app-tag "must-violation"; }
      }
    }
    leaf y {
      when "../name = 'enable'";
      type string;
    }
    leaf z {
      mandatory "true";
      type string;
    }
  }
}`
	if err := os.WriteFile(module, []byte(src), 0o644); err != nil {
		b.Fatal(err)
	}
	ctx, err := cambium.NewContext()
	if err != nil {
		b.Fatalf("NewContext: %v", err)
	}
	b.Cleanup(func() { ctx.Close() })
	if err := ctx.SetSearchPath(dir); err != nil {
		b.Fatalf("SetSearchPath: %v", err)
	}
	if err := ctx.LoadModule("cambium-validation-demo"); err != nil {
		b.Fatalf("LoadModule: %v", err)
	}
	return ctx
}

func BenchmarkBackendParseValidateSerialize(b *testing.B) {
	ctx := benchmarkValidationContext(b)
	data := []byte(`<top xmlns="urn:cambium:validation"><name>open</name><z>ok</z></top>`)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tree, err := ctx.Parse(cambium.FormatXML, cambium.ParseMode{ParseOnly: true}, data)
		if err != nil {
			b.Fatalf("Parse: %v", err)
		}
		if err := tree.Validate(cambium.ValidateMode{Present: true}); err != nil {
			tree.Close()
			b.Fatalf("Validate: %v", err)
		}
		if _, err := tree.Serialize(cambium.FormatXML, cambium.DefaultSerializeFlags()); err != nil {
			tree.Close()
			b.Fatalf("Serialize: %v", err)
		}
		tree.Close()
	}
}
