// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

package datatree_test

import (
	"strings"
	"testing"

	"github.com/signalbreak-labs/cambium/go/datatree"
)

// TestUnknownMemberErrorNamesFirstInDocumentOrder guards against map
// iteration reaching the error text: with several unknown members, Parse
// reports the first one in document order, every time, at the root and in a
// nested object.
func TestUnknownMemberErrorNamesFirstInDocumentOrder(t *testing.T) {
	mod := loadDT(t)
	cases := []struct {
		name, in, want string
	}{
		{
			name: "root",
			in:   `{"dt:u3":1,"dt:c":{"z":"hi"},"dt:u1":2,"dt:u2":3,"dt:u0":4}`,
			want: `unknown member "dt:u3"`,
		},
		{
			name: "container",
			in:   `{"dt:c":{"z":"hi","u3":1,"u1":2,"m":5,"u2":3,"u0":4}}`,
			want: `unknown member "u3"`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for i := 0; i < 50; i++ {
				_, err := datatree.Parse(mod, datatree.FormatJSONIETF, []byte(tc.in))
				if err == nil {
					t.Fatal("Parse accepted unknown members")
				}
				if !strings.Contains(err.Error(), tc.want) {
					t.Fatalf("run %d: error = %q, want it to name %s", i, err, tc.want)
				}
			}
		})
	}
}
