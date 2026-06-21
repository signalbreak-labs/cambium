// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

package compat

import (
	"bytes"
	"io"
)

func newIndentWriter(w io.Writer, prefix string) io.Writer {
	if prefix == "" {
		return w
	}
	return &indentWriter{w: w, prefix: []byte(prefix), atLineStart: true}
}

type indentWriter struct {
	w           io.Writer
	prefix      []byte
	atLineStart bool
}

func (w *indentWriter) Write(buf []byte) (int, error) {
	written := 0
	for len(buf) > 0 {
		if w.atLineStart {
			if _, err := w.w.Write(w.prefix); err != nil {
				return written, err
			}
			w.atLineStart = false
		}

		next := bytes.IndexByte(buf, '\n')
		if next < 0 {
			n, err := w.w.Write(buf)
			written += n
			return written, err
		}

		line := buf[:next+1]
		n, err := w.w.Write(line)
		written += n
		if err != nil {
			return written, err
		}
		if n != len(line) {
			return written, io.ErrShortWrite
		}
		w.atLineStart = true
		buf = buf[next+1:]
	}
	return written, nil
}
