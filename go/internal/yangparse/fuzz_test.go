// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

package yangparse

import "testing"

func FuzzParse(f *testing.F) {
	f.Add(`module fuzz-one { namespace "urn:fuzz-one"; prefix fo; }`)
	f.Add(`module fuzz-two { namespace "urn:fuzz-two"; prefix ft; container c { leaf l { type string; } } }`)
	f.Add("module m {")

	f.Fuzz(func(t *testing.T, src string) {
		_, _ = Parse(src, "fuzz.yang")
	})
}
