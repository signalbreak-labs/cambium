package datatree

import "github.com/signalbreak-labs/cambium/go/cambium"

// ApplyDefaults fills in absent leaves that carry a schema default with that
// default value, in effective schema declaration order (RFC 6243 report-all,
// scoped to subtrees that are present). It recurses into present containers and
// list entries; absent containers and lists are not materialized. Existing
// values are never overwritten, and it is idempotent.
//
// Order is preserved: each level is rebuilt in schema declaration order, and
// list entries keep their keys-first ordering (I3).
func (t *Tree) ApplyDefaults() {
	t.roots = applyDefaultsLevel(flattenTopLevel(t.module), t.roots)
}

func applyDefaultsLevel(schema []cambium.SchemaNodeRef, data []*node) []*node {
	present := make(map[nodeKey]*node, len(data))
	for _, d := range data {
		present[dataNodeKey(d)] = d
	}
	var out []*node
	for _, sn := range schema {
		dn := present[schemaNodeKey(sn)]
		switch {
		case sn.IsLeaf():
			if dn != nil {
				out = append(out, dn)
			} else if def, ok := sn.DefaultValue(); ok {
				out = append(out, defaultLeafNode(sn, def))
			}
		case sn.IsContainer():
			if dn != nil {
				dn.children = applyDefaultsLevel(childRefs(sn.DataChildren(true)), dn.children)
				out = append(out, dn)
			}
		case sn.IsList():
			if dn != nil {
				for i := range dn.entries {
					filled := applyDefaultsLevel(childRefs(sn.DataChildren(true)), dn.entries[i])
					dn.entries[i] = keysFirst(sn, filled)
				}
				out = append(out, dn)
			}
		default:
			if dn != nil {
				out = append(out, dn)
			}
		}
	}
	return out
}

func defaultLeafNode(sn cambium.SchemaNodeRef, def string) *node {
	ti, _ := sn.LeafType()
	return &node{
		name:      sn.Name(),
		module:    sn.Module().Name(),
		namespace: sn.Namespace(),
		kind:      kindLeaf,
		value:     jsonTokenFromText(ti, def, sn.Module().Name()),
	}
}
