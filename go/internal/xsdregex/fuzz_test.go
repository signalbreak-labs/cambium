// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

package xsdregex

import (
	"regexp"
	"testing"
)

func FuzzCompile(f *testing.F) {
	f.Add("a*")
	f.Add("[0-9]{1,3}")
	f.Add("(")

	f.Fuzz(func(t *testing.T, expr string) {
		_, _ = regexp.Compile("^(?:" + NativePattern(expr) + ")$")
	})
}
