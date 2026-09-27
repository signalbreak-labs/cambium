// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

package datatree

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/signalbreak-labs/cambium/go/cambium"
)

// ParseModules decodes a document whose top-level nodes may belong to any of
// mods — a JSON_IETF object with members such as "a:x" and "b:y", or XML
// siblings in several namespaces — into one ordered Tree. A top-level node of
// a module not in mods is an error, as in Parse. Passing a context's
// implemented modules (cambium.Context.Modules) accepts what libyang accepts
// for a data tree parsed against that context.
//
// The tree's top-level nodes are grouped by module, modules in bytewise order
// of their names, each module's nodes in effective schema declaration order:
// the order libyang gives top-level siblings from several modules, whatever
// the input order or the order of mods. Below the top level nothing changes
// (I1-I6 hold as for Parse).
//
// The bound modules are the tree's schema from then on: Validate checks the
// top-level constraints (mandatory nodes, min-elements, choices) of every
// bound module, including one with no data in the document, and resolves
// leafref paths and must/when expressions across all of them; ApplyDefaults
// fills the top-level defaults of every bound module. mods must be non-empty
// and name each module once.
func ParseModules(mods []cambium.Module, f Format, data []byte) (*Tree, error) {
	bound, err := topLevelModules(mods)
	if err != nil {
		return nil, err
	}
	return parseTree(bound, f, data)
}

// topLevelModules returns a copy of mods in top-level order: bytewise by
// module name, the order in which libyang's lyd_insert_get_next_anchor keeps
// top-level siblings of different modules.
func topLevelModules(mods []cambium.Module) ([]cambium.Module, error) {
	if len(mods) == 0 {
		return nil, errors.New("datatree: ParseModules: no modules")
	}
	out := slices.Clone(mods)
	for _, m := range out {
		if m.Name() == "" {
			return nil, errors.New("datatree: ParseModules: invalid module (zero value)")
		}
	}
	slices.SortFunc(out, func(a, b cambium.Module) int { return strings.Compare(a.Name(), b.Name()) })
	for i := 1; i < len(out); i++ {
		if out[i].Name() == out[i-1].Name() {
			return nil, fmt.Errorf("datatree: ParseModules: module %q given more than once", out[i].Name())
		}
	}
	return out, nil
}
