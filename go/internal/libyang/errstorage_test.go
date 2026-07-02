// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

//go:build cgo

package libyang

import (
	"fmt"
	"runtime"
	"strings"
	"testing"
)

func TestLibyangErrorStorageIsPerOSThread(t *testing.T) {
	ctxCh := make(chan *RawContext)
	parsedCh := make(chan struct{})
	closeCh := make(chan struct{})
	g1ErrCh := make(chan error)
	g2ErrCh := make(chan error)

	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()

		c, err := NewContext()
		if err != nil {
			close(ctxCh)
			close(parsedCh)
			g1ErrCh <- fmt.Errorf("NewContext: %w", err)
			return
		}
		ctxCh <- c

		_, err = c.ParseData(FormatXML, 0, []byte("<unclosed"))
		close(parsedCh)
		if err == nil {
			g1ErrCh <- fmt.Errorf("ParseData returned nil error")
		} else if strings.HasPrefix(err.Error(), "parse data failed: rc=") {
			g1ErrCh <- fmt.Errorf("ParseData returned generic error: %v", err)
		} else {
			g1ErrCh <- nil
		}

		<-closeCh
		c.Close()
	}()

	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()

		c, ok := <-ctxCh
		if !ok {
			g2ErrCh <- fmt.Errorf("context was not created")
			return
		}
		<-parsedCh
		err := lyError(c.ctx, "probe", 0)
		if err == nil || err.Error() != "probe failed" {
			g2ErrCh <- fmt.Errorf("lyError on another OS thread = %v, want probe failed", err)
			return
		}
		g2ErrCh <- nil
	}()

	var g1Err, g2Err error
	for range 2 {
		select {
		case g1Err = <-g1ErrCh:
		case g2Err = <-g2ErrCh:
		}
	}
	close(closeCh)

	if g1Err != nil {
		t.Fatal(g1Err)
	}
	if g2Err != nil {
		t.Fatal(g2Err)
	}
}
