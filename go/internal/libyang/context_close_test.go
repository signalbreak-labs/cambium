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
			go func() {
				defer func() { done <- struct{}{} }()
				<-start
				tree, err := c.ParseData(FormatXML, 0, []byte(`<a xmlns="urn:closerace"/>`))
				if err == nil {
					tree.Close()
				}
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
