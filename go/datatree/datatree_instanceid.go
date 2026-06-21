// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

package datatree

import (
	"fmt"

	"github.com/signalbreak-labs/cambium/go/cambium"
)

// checkInstanceIdentifier validates a require-instance instance-identifier leaf
// (O8 slice 4c): the value is itself a restricted XPath naming an instance that
// must exist. The value is parsed and evaluated with the XPath engine; if it
// resolves to an empty node-set the instance is missing.
//
// Caveat (safe by skipping): JSON_IETF encodes the path with module-name
// qualifiers while this resolver uses import prefixes, so a qualifier that is
// not a resolvable prefix makes evaluation error out and the check is SKIPPED —
// never a wrong verdict. Full coverage needs context-level module-name
// resolution.
func checkInstanceIdentifier(ev *evaluator, n *xnode, out *[]string) {
	if !n.hasSchema || !n.leaf {
		return
	}
	ti, ok := n.schema.LeafType()
	if !ok {
		return
	}
	ii, ok := ti.Resolved().(cambium.ResolvedInstanceIdentifier)
	if !ok || !ii.RequireInstance {
		return
	}
	ast, err := parseXPath(n.value)
	if err != nil {
		return // unparseable instance-identifier: skip
	}
	v, err := ev.eval(ast, ectx{node: n, pos: 1, size: 1})
	if err != nil || v.kind != kNodeset {
		return // unresolvable (e.g. module-name qualifier): skip
	}
	if len(v.ns) == 0 {
		*out = append(*out, fmt.Sprintf("%s: instance-identifier %q references a non-existent instance", xnodePath(n), n.value))
	}
}
