// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

package cambium_test

import (
	"testing"

	"github.com/signalbreak-labs/cambium/go/cambium"
	upstream "github.com/signalbreak-labs/cambium/go/internal/yangparse/upstream/yang"
)

func TestCamelCaseMatchesGoyang(t *testing.T) {
	tests := []string{
		"",
		"ietf-interfaces.foo",
		"my_field-name_2",
		"_hidden",
		"-leading-dash",
		".leading-dot",
		"alreadyCamel",
		"ip-address",
		"foo1bar",
		"name_1",
		"XML-name",
	}
	for _, input := range tests {
		if got, want := cambium.CamelCase(input), upstream.CamelCase(input); got != want {
			t.Fatalf("CamelCase(%q) = %q, want goyang %q", input, got, want)
		}
	}
}
