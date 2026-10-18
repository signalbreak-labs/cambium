// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs
package cambium

import (
	"regexp"
	"testing"
)

func TestPatternGoRegexpLeadingZeroCounts(t *testing.T) {
	for _, tc := range []struct {
		source, value string
		want          bool
	}{
		{"a{0001}", "a", true}, {"a{0001}", "a{0001}", false}, {"a{0000}", "", true},
		{"a{00,0000}", "", true}, {"a{0001,}", "aaa", true}, {"(a{0002}){0002}", "aaaa", true},
	} {
		t.Run(tc.source+"/"+tc.value, func(t *testing.T) {
			p := Pattern{regex: tc.source}
			source, err := p.GoRegexp()
			if err != nil {
				t.Fatal(err)
			}
			re, err := regexp.Compile(source)
			if err != nil {
				t.Fatal(err)
			}
			if got := re.MatchString(tc.value); got != tc.want {
				t.Fatalf("GoRegexp(%q)=%q MatchString(%q)=%v want %v", tc.source, source, tc.value, got, tc.want)
			}
		})
	}
}
