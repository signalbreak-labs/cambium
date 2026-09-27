// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

//go:build cgo

package libyangbackend_test

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"

	cambium "github.com/signalbreak-labs/cambium/go/libyangbackend"
)

const (
	freezeBaseModule = `module freeze-a {
  namespace "urn:freeze-a";
  prefix fa;
  revision 2026-09-27;
  container top { leaf x { type string; } }
  rpc ping { input { leaf msg { type string; } } }
}
`
	// freeze-b augments the base schema, so loading it recompiles freeze-a and
	// frees the compiled nodes that existing data trees point at.
	freezeAugmentModule = `module freeze-b {
  namespace "urn:freeze-b";
  prefix fb;
  import freeze-a { prefix fa; }
  revision 2026-09-27;
  augment "/fa:top" { leaf y { type string; } }
}
`
	freezeDoc = `<top xmlns="urn:freeze-a"><x>1</x></top>`
)

// freezeContext returns a context with freeze-a loaded and the directory that
// also holds the not-yet-loaded freeze-b.
func freezeContext(t *testing.T) (ctx *cambium.Context, dir string) {
	t.Helper()
	dir = t.TempDir()
	for name, src := range map[string]string{
		"freeze-a.yang": freezeBaseModule,
		"freeze-b.yang": freezeAugmentModule,
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	ctx, err := cambium.NewContext()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(ctx.Close)
	if err := ctx.SetSearchPath(dir); err != nil {
		t.Fatal(err)
	}
	if err := ctx.LoadModule("freeze-a"); err != nil {
		t.Fatal(err)
	}
	return ctx, dir
}

func assertFrozenLoad(t *testing.T, err error) {
	t.Helper()
	if !errors.Is(err, cambium.ErrContextFrozen) {
		t.Fatalf("load after data tree error = %v, want ErrContextFrozen", err)
	}
	var ce *cambium.Error
	if !errors.As(err, &ce) || ce.RuleCode() != cambium.RuleCodeContext {
		t.Fatalf("load after data tree error = %v, want *Error with %s", err, cambium.RuleCodeContext)
	}
}

// TestLoadModuleAfterDataTreeFailsClosed guards review item A1: loading a
// module that augments or deviates an existing schema makes libyang recompile
// it and free the compiled nodes live data trees point at, so the next
// Serialize failed with an internal error and Validate crashed (SIGSEGV). The
// context now freezes when its first data tree is created: later loads fail
// with ErrContextFrozen (CAMBIUM_E0001) and the schema is left untouched.
func TestLoadModuleAfterDataTreeFailsClosed(t *testing.T) {
	creators := []struct {
		name   string
		create func(*testing.T, *cambium.Context) *cambium.DataTree
	}{
		{"Parse", func(t *testing.T, ctx *cambium.Context) *cambium.DataTree {
			tree, err := ctx.Parse(cambium.FormatXML, cambium.ParseModeDataOnly, []byte(freezeDoc))
			if err != nil {
				t.Fatal(err)
			}
			return tree
		}},
		{"ParseOp", func(t *testing.T, ctx *cambium.Context) *cambium.DataTree {
			tree, err := ctx.ParseOp(cambium.FormatXML, cambium.OpTypeRPC, []byte(`<ping xmlns="urn:freeze-a"><msg>hi</msg></ping>`))
			if err != nil {
				t.Fatal(err)
			}
			return tree
		}},
		{"NewData", func(t *testing.T, ctx *cambium.Context) *cambium.DataTree {
			tree := ctx.NewData()
			x := "1"
			if _, err := tree.NewPath("/freeze-a:top/x", &x, cambium.NewPathOpts{}); err != nil {
				t.Fatal(err)
			}
			return tree
		}},
	}
	for _, tc := range creators {
		t.Run(tc.name, func(t *testing.T) {
			ctx, dir := freezeContext(t)
			tree := tc.create(t, ctx)
			before, err := tree.Serialize(cambium.FormatXML, cambium.DefaultSerializeFlags())
			if err != nil {
				t.Fatal(err)
			}

			assertFrozenLoad(t, ctx.LoadModule("freeze-b"))
			assertFrozenLoad(t, ctx.LoadModuleFromPath(filepath.Join(dir, "freeze-b.yang")))

			// The refused load must leave the compiled schema, and so the
			// tree, exactly as it was.
			after, err := tree.Serialize(cambium.FormatXML, cambium.DefaultSerializeFlags())
			if err != nil {
				t.Fatalf("Serialize after refused load: %v", err)
			}
			if !bytes.Equal(before, after) {
				t.Fatalf("Serialize after refused load = %q, want %q", after, before)
			}
			if tc.name != "ParseOp" {
				if err := tree.Validate(cambium.ValidateMode{}); err != nil {
					t.Fatalf("Validate after refused load: %v", err)
				}
			}
			if _, err := ctx.Schema("freeze-b"); err == nil {
				t.Fatal("freeze-b is in the schema after a refused load")
			}

			// The freeze is permanent: releasing every tree does not reopen
			// the build phase.
			tree.Close()
			assertFrozenLoad(t, ctx.LoadModule("freeze-b"))
		})
	}
}

// TestLoadModuleBeforeDataTreeStillAllowed pins what does NOT freeze the
// context: schema reads (their handles are Go-owned snapshots) and a parse
// that failed without producing a tree.
func TestLoadModuleBeforeDataTreeStillAllowed(t *testing.T) {
	ctx, _ := freezeContext(t)
	if _, err := ctx.Schema("freeze-a"); err != nil {
		t.Fatal(err)
	}
	if len(ctx.Modules()) == 0 {
		t.Fatal("no modules")
	}
	if _, err := ctx.Parse(cambium.FormatXML, cambium.ParseModeDataOnly, []byte(`<top xmlns="urn:freeze-b"/>`)); err == nil {
		t.Fatal("parse of data for an unloaded module succeeded")
	}
	if err := ctx.LoadModule("freeze-b"); err != nil {
		t.Fatalf("LoadModule before any data tree: %v", err)
	}
	base, err := ctx.Schema("freeze-a")
	if err != nil {
		t.Fatal(err)
	}
	if got := base.AugmentedBy(); len(got) != 1 || got[0] != "freeze-b" {
		t.Fatalf("freeze-a AugmentedBy = %v, want [freeze-b]", got)
	}
}
