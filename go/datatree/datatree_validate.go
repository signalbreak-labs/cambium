package datatree

import (
	"fmt"
	"strings"

	"github.com/signalbreak-labs/cambium/go/cambium"
)

// ValidationError aggregates the structural validation violations found by
// Tree.Validate, in document/schema order.
type ValidationError struct {
	Violations []string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("datatree: %d validation violation(s): %s",
		len(e.Violations), strings.Join(e.Violations, "; "))
}

// Validate checks constraints that need no XPath engine: mandatory leaves present,
// list / leaf-list min- and max-elements, leaf-list value uniqueness, list keys
// present in every entry, list key uniqueness, and per-leaf value types. It
// returns a *ValidationError listing every violation, or nil if the tree is valid.
//
// It does NOT yet check must/when XPath, leafref instance existence, or
// instance-identifier resolution, which remain the libyang backend's domain.
func (t *Tree) Validate() error {
	var violations []string
	validateLevel(flattenTopLevel(t.module), t.roots, "", &violations)
	if len(violations) == 0 {
		return nil
	}
	return &ValidationError{Violations: violations}
}

// validateLevel walks the schema children in declaration order and checks each
// against the data present at this level. Driving from the schema (not the data)
// is what surfaces absent-but-required nodes.
func validateLevel(schema []cambium.SchemaNodeRef, data []*node, path string, out *[]string) {
	present := make(map[nodeKey]*node, len(data))
	for _, d := range data {
		present[dataNodeKey(d)] = d
	}
	for _, sn := range schema {
		childPath := path + "/" + sn.Name()
		dn := present[schemaNodeKey(sn)]
		if dn == nil {
			if sn.IsMandatory() {
				*out = append(*out, fmt.Sprintf("missing mandatory node %s", childPath))
			}
			if min, ok := sn.MinElements(); ok && min > 0 {
				*out = append(*out, fmt.Sprintf("%s has 0 entries, fewer than min-elements %d", childPath, min))
			}
			continue
		}
		switch {
		case sn.IsList():
			checkElements(sn, len(dn.entries), childPath, out)
			checkListKeys(sn, dn, childPath, out)
			for i, entry := range dn.entries {
				validateLevel(childRefs(sn.DataChildren(true)), entry, fmt.Sprintf("%s[%d]", childPath, i), out)
			}
		case sn.IsLeafList():
			checkElements(sn, len(dn.values), childPath, out)
			checkLeafListUnique(dn, childPath, out)
			if ti, ok := sn.LeafType(); ok {
				for i, v := range dn.values {
					validateLeafValue(ti, v, fmt.Sprintf("%s[%d]", childPath, i), sn.Module().Name(), out)
				}
			}
		case sn.IsContainer():
			validateLevel(childRefs(sn.DataChildren(true)), dn.children, childPath, out)
		case sn.IsLeaf():
			if ti, ok := sn.LeafType(); ok {
				validateLeafValue(ti, dn.value, childPath, sn.Module().Name(), out)
			}
		}
	}
}

func checkElements(sn cambium.SchemaNodeRef, count int, path string, out *[]string) {
	if min, ok := sn.MinElements(); ok && uint32(count) < min {
		*out = append(*out, fmt.Sprintf("%s has %d entries, fewer than min-elements %d", path, count, min))
	}
	if max, ok := sn.MaxElements(); ok && uint32(count) > max {
		*out = append(*out, fmt.Sprintf("%s has %d entries, more than max-elements %d", path, count, max))
	}
}

// checkListKeys verifies every list entry carries all key leaves and that the
// key tuples are unique across entries (invariant-neutral: key order in the
// tuple follows key-statement order, and entries are not reordered).
func checkListKeys(sn cambium.SchemaNodeRef, dn *node, path string, out *[]string) {
	keys := sn.KeyNames()
	if len(keys) == 0 {
		return
	}
	seen := make(map[string]bool, len(dn.entries))
	for i, entry := range dn.entries {
		byName := make(map[string]*node, len(entry))
		for _, e := range entry {
			byName[e.name] = e
		}
		tuple := make([]string, 0, len(keys))
		complete := true
		for _, k := range keys {
			kn := byName[k]
			if kn == nil {
				*out = append(*out, fmt.Sprintf("%s[%d] is missing key leaf %q", path, i, k))
				complete = false
				continue
			}
			tuple = append(tuple, string(kn.value))
		}
		if !complete {
			continue
		}
		joined := strings.Join(tuple, "\x00")
		if seen[joined] {
			*out = append(*out, fmt.Sprintf("%s has a duplicate key %v", path, tuple))
		}
		seen[joined] = true
	}
}

func checkLeafListUnique(dn *node, path string, out *[]string) {
	seen := make(map[string]bool, len(dn.values))
	for i, v := range dn.values {
		value := string(v)
		if seen[value] {
			*out = append(*out, fmt.Sprintf("%s[%d] has a duplicate leaf-list value", path, i))
			continue
		}
		seen[value] = true
	}
}
