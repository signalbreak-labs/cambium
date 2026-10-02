// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

package cambium

import "slices"

// RPCs and actions have input/output schema nodes even when their optional
// statements are absent. Materialize these before resolving augments, which
// may supply their first parameters. Synthetic IO has no source statement.
func (n *schemaNodeData) addImplicitOperationIO() {
	if n.kind != SchemaNodeKindRPC && n.kind != SchemaNodeKindAction {
		return
	}
	for _, name := range []string{"input", "output"} {
		if n.directChild(name) != nil {
			continue
		}
		if !n.sourceModule.admitSchemaNode(n.stmt) {
			return
		}
		child := &schemaNodeData{
			name:                name,
			kind:                kindForKeyword(name),
			module:              n.module,
			sourceModule:        n.sourceModule,
			instantiatingModule: n.instantiatingModule,
			parent:              n,
			status:              StatusCurrent,
			config:              n.config,
			orderedBy:           OrderedBySystem,
			choiceDesc:          n.choiceDesc,
			groupOrigin:         n.groupOrigin,
		}
		if name == "input" {
			n.children = slices.Insert(n.children, 0, child)
		} else {
			n.children = append(n.children, child)
		}
	}
}
