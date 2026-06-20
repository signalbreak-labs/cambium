package compat

import (
	"testing"

	upstream "github.com/signalbreak-labs/cambium/go/internal/yangparse/upstream/yang"
)

func TestReadOnlyMatchesGoyangForOutputDescendantConfig(t *testing.T) {
	compatOutput := &Entry{Name: "output", Kind: OutputEntry, Config: TSUnset}
	compatExplicitConfig := &Entry{Name: "value", Kind: LeafEntry, Config: TSTrue, Parent: compatOutput}
	compatInheritedConfig := &Entry{Name: "inherited", Kind: LeafEntry, Config: TSUnset, Parent: compatOutput}

	upstreamOutput := &upstream.Entry{Name: "output", Kind: upstream.OutputEntry, Config: upstream.TSUnset}
	upstreamExplicitConfig := &upstream.Entry{Name: "value", Kind: upstream.LeafEntry, Config: upstream.TSTrue, Parent: upstreamOutput}
	upstreamInheritedConfig := &upstream.Entry{Name: "inherited", Kind: upstream.LeafEntry, Config: upstream.TSUnset, Parent: upstreamOutput}

	cases := []struct {
		name     string
		compat   *Entry
		upstream *upstream.Entry
	}{
		{name: "output", compat: compatOutput, upstream: upstreamOutput},
		{name: "explicit config true child", compat: compatExplicitConfig, upstream: upstreamExplicitConfig},
		{name: "inherited output child", compat: compatInheritedConfig, upstream: upstreamInheritedConfig},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got, want := tc.compat.ReadOnly(), tc.upstream.ReadOnly(); got != want {
				t.Fatalf("ReadOnly() = %v, want goyang %v", got, want)
			}
		})
	}
}
