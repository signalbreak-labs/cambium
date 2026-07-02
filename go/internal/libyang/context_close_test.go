// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

//go:build cgo

package libyang

import (
	"errors"
	"testing"
)

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
