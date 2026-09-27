// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

//go:build cgo

package libyang

import (
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

// freezeRawContext returns a context with freeze-a loaded and the path of
// freeze-b, which augments freeze-a and is not loaded yet.
func freezeRawContext(t *testing.T) (c *RawContext, augmentPath string) {
	t.Helper()
	dir := t.TempDir()
	modules := []struct{ name, src string }{
		{"freeze-a.yang", `module freeze-a {
  namespace "urn:freeze-a";
  prefix fa;
  container top { leaf x { type string; } }
}
`},
		{"freeze-b.yang", `module freeze-b {
  namespace "urn:freeze-b";
  prefix fb;
  import freeze-a { prefix fa; }
  augment "/fa:top" { leaf y { type string; } }
}
`},
	}
	for _, m := range modules {
		if err := os.WriteFile(filepath.Join(dir, m.name), []byte(m.src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	c, err := NewContext()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(c.Close)
	if err := c.SetSearchPath(dir); err != nil {
		t.Fatal(err)
	}
	if err := c.LoadModule("freeze-a"); err != nil {
		t.Fatal(err)
	}
	return c, filepath.Join(dir, "freeze-b.yang")
}

// TestLoadModuleAfterDataTreeReturnsErrContextFrozen pins the raw-layer
// freeze: once a data tree exists, a module load would recompile and free the
// schema the tree points at, so it must fail before reaching libyang.
func TestLoadModuleAfterDataTreeReturnsErrContextFrozen(t *testing.T) {
	for _, create := range []struct {
		name string
		fn   func(*RawContext) (*RawDataTree, error)
	}{
		{"ParseData", func(c *RawContext) (*RawDataTree, error) {
			return c.ParseData(FormatXML, ParseOnly, []byte(`<top xmlns="urn:freeze-a"><x>1</x></top>`))
		}},
		{"NewData", func(c *RawContext) (*RawDataTree, error) { return c.NewData(), nil }},
	} {
		t.Run(create.name, func(t *testing.T) {
			c, bPath := freezeRawContext(t)
			tree, err := create.fn(c)
			if err != nil {
				t.Fatal(err)
			}
			defer tree.Close()
			if err := c.LoadModule("freeze-b"); !errors.Is(err, ErrContextFrozen) {
				t.Fatalf("LoadModule after %s error = %v, want ErrContextFrozen", create.name, err)
			}
			if err := c.LoadModuleFromPath(bPath); !errors.Is(err, ErrContextFrozen) {
				t.Fatalf("LoadModuleFromPath after %s error = %v, want ErrContextFrozen", create.name, err)
			}
		})
	}
}

// TestLoadModuleFromPathAfterCloseReturnsErrContextClosed: LoadModuleFromPath
// skipped the acquire gate every other context entry point uses, so after
// Close it handed a nil ly_ctx to libyang instead of failing closed.
func TestLoadModuleFromPathAfterCloseReturnsErrContextClosed(t *testing.T) {
	c, bPath := freezeRawContext(t)
	c.Close()
	if err := c.LoadModuleFromPath(bPath); !errors.Is(err, ErrContextClosed) {
		t.Fatalf("LoadModuleFromPath after Close error = %v, want ErrContextClosed", err)
	}
}

// TestLoadModuleRacingFirstParseIsDefined runs a module load against the
// context's first parses under the race detector. Either the load wins (the
// parses then see the recompiled schema) or a parse wins (the load then fails
// with ErrContextFrozen); a load must never recompile under a live tree.
func TestLoadModuleRacingFirstParseIsDefined(t *testing.T) {
	const (
		rounds  = 20
		parsers = 3
	)
	doc := []byte(`<top xmlns="urn:freeze-a"><x>1</x></top>`)
	for i := 0; i < rounds; i++ {
		c, _ := freezeRawContext(t)
		start := make(chan struct{})
		var wg sync.WaitGroup
		trees := make([]*RawDataTree, parsers)
		for g := 0; g < parsers; g++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start
				tree, err := c.ParseData(FormatXML, ParseOnly, doc)
				if err != nil {
					t.Errorf("ParseData: %v", err)
					return
				}
				trees[g] = tree
			}()
		}
		var loadErr error
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			loadErr = c.LoadModule("freeze-b")
		}()
		close(start)
		wg.Wait()
		if loadErr != nil && !errors.Is(loadErr, ErrContextFrozen) {
			t.Fatalf("round %d: LoadModule error = %v, want nil or ErrContextFrozen", i, loadErr)
		}
		for _, tree := range trees {
			if tree == nil {
				continue
			}
			if err := tree.Validate(0); err != nil {
				t.Fatalf("round %d: Validate: %v", i, err)
			}
			if _, err := tree.Serialize(FormatXML, PrintWithSiblings); err != nil {
				t.Fatalf("round %d: Serialize: %v", i, err)
			}
			tree.Close()
		}
	}
}
