// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

//go:build cgo

package libyang

import (
	"errors"
	"testing"
)

// TestContextCloseConcurrentWithOpsIsDefined pins the T-002 acquire/release
// protocol under the race detector: a Close racing in-flight operations must
// never destroy the ly_ctx under a running cgo call, and post-Close operations
// must fail with ErrContextClosed rather than touch freed memory. Every
// operation outcome (success, parse error, ErrContextClosed) is acceptable;
// the failure modes this guards are a crash or a race report.
func TestContextCloseConcurrentWithOpsIsDefined(t *testing.T) {
	const (
		rounds     = 200
		goroutines = 4
	)
	for i := 0; i < rounds; i++ {
		c, err := NewContext()
		if err != nil {
			t.Fatalf("NewContext: %v", err)
		}
		start := make(chan struct{})
		done := make(chan struct{})
		for g := 0; g < goroutines; g++ {
			parse := g%2 == 0
			go func() {
				defer func() { done <- struct{}{} }()
				<-start
				if parse {
					tree, err := c.ParseData(FormatXML, 0, []byte(`<a xmlns="urn:closerace"/>`))
					if err == nil {
						tree.Close()
					}
					return
				}
				// NewData never errors: racing Close it must return either a
				// live tree or a fail-closed shell, never a dangling ctx.
				tree := c.NewData()
				if err := tree.NewPath("/x", nil, 0); err != nil && !errors.Is(err, ErrContextClosed) {
					_ = err // any libyang error is acceptable; UAF/race is the failure mode
				}
				tree.Close()
			}()
		}
		close(start)
		c.Close()
		for g := 0; g < goroutines; g++ {
			<-done
		}
	}
}

func TestContextOpsAfterCloseReturnErrContextClosed(t *testing.T) {
	c, err := NewContext()
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}
	c.Close()

	if err := c.SetSearchPath(t.TempDir()); !errors.Is(err, ErrContextClosed) {
		t.Fatalf("SetSearchPath after Close error = %v, want ErrContextClosed", err)
	}
	if err := c.LoadModule("ietf-inet-types"); !errors.Is(err, ErrContextClosed) {
		t.Fatalf("LoadModule after Close error = %v, want ErrContextClosed", err)
	}
	if _, err := c.ParseData(FormatXML, 0, []byte("<a/>")); !errors.Is(err, ErrContextClosed) {
		t.Fatalf("ParseData after Close error = %v, want ErrContextClosed", err)
	}
	if _, err := c.ParseOp(FormatXML, OpRPC, []byte("<a/>")); !errors.Is(err, ErrContextClosed) {
		t.Fatalf("ParseOp after Close error = %v, want ErrContextClosed", err)
	}
}

func TestNewDataAfterCloseRawFailsClosed(t *testing.T) {
	c, err := NewContext()
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}
	c.Close()

	tree := c.NewData()
	defer tree.Close()
	if tree == nil {
		t.Fatal("NewData after Close returned nil")
	}
	if tree.ctx != nil {
		t.Fatalf("NewData after Close ctx = %p, want nil", tree.ctx)
	}
	if _, err := tree.RootNodes(); !errors.Is(err, ErrContextClosed) {
		t.Fatalf("RootNodes after Close error = %v, want ErrContextClosed", err)
	}
	if err := tree.NewPath("/x", nil, 0); !errors.Is(err, ErrContextClosed) {
		t.Fatalf("NewPath after Close error = %v, want ErrContextClosed", err)
	}
	if _, err := tree.Serialize(FormatXML, 0); !errors.Is(err, ErrContextClosed) {
		t.Fatalf("Serialize after Close error = %v, want ErrContextClosed", err)
	}
	if err := tree.Validate(0); !errors.Is(err, ErrContextClosed) {
		t.Fatalf("Validate after Close error = %v, want ErrContextClosed", err)
	}
}
