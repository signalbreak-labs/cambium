// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

package datatree

import (
	"bytes"
	"cmp"
	"encoding/json"
	"slices"
	"strconv"
	"strings"

	"github.com/signalbreak-labs/cambium/go/cambium"
)

// Canonical order for ordered-by system lists and leaf-lists (invariant I2).
//
// RFC 7950 §7.7.7 leaves the order of ordered-by system entries to the
// implementation. Cambium's Backend/data tier emits libyang's canonical order,
// and datatree reproduces it so both engines serialize the same bytes: a
// leaf-list is ordered by value and a keyed list by its key values in
// key-statement order, each value compared with its type's libyang sort
// callback (tree_data_sorted.c, plugins_types/*.c). libyang keeps state data,
// RPC/action output, notification content, and keyless lists in insertion
// order, and so does datatree. Values that compare equal keep input order.
//
// Derived types that libyang sorts through a dedicated plugin (for example the
// ietf-inet-types address types) are compared here by their canonical string,
// like any other string.

// systemOrdered reports whether sn's instances are kept in canonical order.
func systemOrdered(sn cambium.SchemaNodeRef) bool {
	if sn.OrderedBy() != cambium.OrderedBySystem || !sn.RepresentsConfigurationData() {
		return false
	}
	switch {
	case sn.IsLeafList():
		return true
	case sn.IsList():
		return len(sn.KeyNames()) > 0
	default:
		return false
	}
}

// sortSystemOrdered puts n's leaf-list values or list entries in canonical
// order when sn is ordered-by system; ordered-by user data is left untouched,
// in insertion order (I1).
func sortSystemOrdered(sn cambium.SchemaNodeRef, n *node) {
	if !systemOrdered(sn) {
		return
	}
	switch n.kind {
	case kindLeafList:
		ti, ok := sn.LeafType()
		if !ok || len(n.values) < 2 {
			return
		}
		type keyed struct {
			key   orderKey
			value json.RawMessage
		}
		items := make([]keyed, len(n.values))
		for i, v := range n.values {
			items[i] = keyed{key: valueOrderKey(ti, v, sn.Module().Name()), value: v}
		}
		slices.SortStableFunc(items, func(a, b keyed) int { return compareOrderKeys(a.key, b.key) })
		for i := range items {
			n.values[i] = items[i].value
		}
	case kindList:
		if len(n.entries) < 2 {
			return
		}
		keys := childRefs(sn.ListKeys())
		type keyed struct {
			keys  []orderKey
			entry []*node
		}
		items := make([]keyed, len(n.entries))
		for i, entry := range n.entries {
			items[i] = keyed{keys: entryOrderKeys(keys, entry), entry: entry}
		}
		slices.SortStableFunc(items, func(a, b keyed) int {
			for k := range a.keys {
				if c := compareOrderKeys(a.keys[k], b.keys[k]); c != 0 {
					return c
				}
			}
			return 0
		})
		for i := range items {
			n.entries[i] = items[i].entry
		}
	}
}

// entryOrderKeys returns the order keys of a list entry's key leaves, in
// key-statement order. A missing key (invalid data) sorts as an invalid value.
func entryOrderKeys(keys []cambium.SchemaNodeRef, entry []*node) []orderKey {
	out := make([]orderKey, len(keys))
	for i, k := range keys {
		want := schemaNodeKey(k)
		for _, c := range entry {
			if c.kind == kindLeaf && dataNodeKey(c) == want {
				if ti, ok := k.LeafType(); ok {
					out[i] = valueOrderKey(ti, c.value, k.Module().Name())
				}
				break
			}
		}
	}
	return out
}

type orderKind uint8

const (
	orderBytes      orderKind = iota // bytewise, like strcmp/memcmp
	orderSigned                      // signed integers, decimal64 (scaled), enum values, booleans
	orderUnsigned                    // unsigned integers
	orderSizedBytes                  // binary: decoded length first, then bytes
	orderBits                        // bits: the little-endian position bitmap, bytewise
)

// orderKey is one value's position in libyang's canonical order for its type,
// computed once per value so sorting never re-parses tokens.
type orderKey struct {
	valid  bool // false: not a valid value of the type; sorts after all valid values, by raw token
	member int  // index of the matching flattened union member (0 outside unions)
	kind   orderKind
	i      int64
	u      uint64
	b      []byte
	bits   []uint64 // orderBits: nonzero bitmap bytes as index<<8|byte, by ascending index
}

func compareOrderKeys(a, b orderKey) int {
	if a.valid != b.valid {
		if a.valid {
			return -1
		}
		return 1
	}
	if !a.valid {
		return bytes.Compare(a.b, b.b)
	}
	if a.member != b.member {
		// libyang's union sort: the value whose member type appears first in the
		// union's (flattened) member list is the greater one.
		return cmp.Compare(b.member, a.member)
	}
	switch a.kind {
	case orderSigned:
		return cmp.Compare(a.i, b.i)
	case orderUnsigned:
		return cmp.Compare(a.u, b.u)
	case orderSizedBytes:
		if c := cmp.Compare(len(a.b), len(b.b)); c != 0 {
			return c
		}
		return bytes.Compare(a.b, b.b)
	case orderBits:
		return compareSparseBitmaps(a.bits, b.bits)
	default:
		return bytes.Compare(a.b, b.b)
	}
}

// compareSparseBitmaps compares two bitmaps as memcmp would compare their dense
// little-endian forms, without allocating a byte per possible bit position.
func compareSparseBitmaps(a, b []uint64) int {
	for i := 0; ; i++ {
		switch {
		case i == len(a) && i == len(b):
			return 0
		case i == len(a):
			return -1 // b has a nonzero byte where a is zero
		case i == len(b):
			return 1
		}
		ai, bi := a[i]>>8, b[i]>>8
		if ai != bi {
			// At the lower index one side has a nonzero byte and the other zero.
			if ai < bi {
				return 1
			}
			return -1
		}
		if c := cmp.Compare(a[i]&0xff, b[i]&0xff); c != 0 {
			return c
		}
	}
}

// valueOrderKey computes the canonical-order key of a leaf value token of type
// ti. leafModule is the leaf's module name (bare identityref context).
func valueOrderKey(ti cambium.TypeInfo, raw json.RawMessage, leafModule string) orderKey {
	invalid := orderKey{b: raw}
	switch r := ti.Resolved().(type) {
	case cambium.ResolvedInt:
		text, ok := integerText(raw, r.Kind)
		if !ok || len(text) > maxNumericLexicalLen {
			return invalid
		}
		if unsignedIntKind(r.Kind) {
			u, err := strconv.ParseUint(text, 10, 64)
			if err != nil {
				return invalid
			}
			return orderKey{valid: true, kind: orderUnsigned, u: u}
		}
		i, err := strconv.ParseInt(text, 10, 64)
		if err != nil {
			return invalid
		}
		return orderKey{valid: true, kind: orderSigned, i: i}
	case cambium.ResolvedDecimal64:
		s, ok := jsonStringValue(raw)
		if !ok {
			return invalid
		}
		num, ok := parseDecimal64(s, int(r.FractionDigits().Value()))
		if !ok {
			return invalid
		}
		return orderKey{valid: true, kind: orderSigned, i: num}
	case cambium.ResolvedBoolean:
		var v bool
		if json.Unmarshal(raw, &v) != nil {
			return invalid
		}
		if v {
			return orderKey{valid: true, kind: orderSigned, i: 1}
		}
		return orderKey{valid: true, kind: orderSigned}
	case cambium.ResolvedEnumeration:
		s, ok := jsonStringValue(raw)
		if !ok {
			return invalid
		}
		for _, v := range r.Values() {
			if v.Name() == s {
				return orderKey{valid: true, kind: orderSigned, i: v.Value()}
			}
		}
		return invalid
	case cambium.ResolvedBits:
		bits, ok := bitsOrderBitmap(raw, r.Values())
		if !ok {
			return invalid
		}
		return orderKey{valid: true, kind: orderBits, bits: bits}
	case cambium.ResolvedBinary:
		s, ok := jsonStringValue(raw)
		if !ok {
			return invalid
		}
		data, err := decodeBinaryValue(s)
		if err != nil {
			return invalid
		}
		return orderKey{valid: true, kind: orderSizedBytes, b: data}
	case cambium.ResolvedIdentityRef:
		// libyang compares the identity names only, not their modules.
		s, ok := jsonStringValue(raw)
		if !ok {
			return invalid
		}
		if i := strings.LastIndexByte(s, ':'); i >= 0 {
			s = s[i+1:]
		}
		return orderKey{valid: true, kind: orderBytes, b: []byte(s)}
	case cambium.ResolvedString, cambium.ResolvedInstanceIdentifier:
		s, ok := jsonStringValue(raw)
		if !ok {
			return invalid
		}
		return orderKey{valid: true, kind: orderBytes, b: []byte(s)}
	case cambium.ResolvedEmpty:
		return orderKey{valid: true, kind: orderBytes}
	case cambium.ResolvedLeafRef:
		if rt, ok := r.Realtype(); ok && rt != nil {
			return valueOrderKey(*rt, raw, leafModule)
		}
	case cambium.ResolvedUnion:
		for i, member := range flattenUnionMembers(r) {
			var trial []string
			validateLeafValue(member, raw, "", leafModule, &trial)
			if len(trial) == 0 {
				k := valueOrderKey(member, raw, leafModule)
				k.member = i
				return k
			}
		}
		return invalid
	}
	return orderKey{valid: true, kind: orderBytes, b: raw}
}

// flattenUnionMembers returns a union's member types with nested unions
// expanded in place, as libyang compiles them.
func flattenUnionMembers(r cambium.ResolvedUnion) []cambium.TypeInfo {
	var out []cambium.TypeInfo
	for _, m := range r.Members() {
		if nested, ok := m.Resolved().(cambium.ResolvedUnion); ok {
			out = append(out, flattenUnionMembers(nested)...)
			continue
		}
		out = append(out, m)
	}
	return out
}

func unsignedIntKind(kind cambium.IntKind) bool {
	switch kind {
	case cambium.IntKindU8, cambium.IntKindU16, cambium.IntKindU32, cambium.IntKindU64:
		return true
	default:
		return false
	}
}

// bitsOrderBitmap returns the set bits of a bits value as a sparse
// little-endian bitmap (nonzero bytes as index<<8|byte, ascending), the layout
// libyang compares with memcmp.
func bitsOrderBitmap(raw json.RawMessage, values []cambium.EnumValue) ([]uint64, bool) {
	s, ok := jsonStringValue(raw)
	if !ok {
		return nil, false
	}
	byteOf := make(map[uint64]uint64)
	seen := make(map[string]bool)
	for name := range strings.SplitSeq(s, " ") {
		if name == "" {
			continue
		}
		pos, ok := bitPosition(values, name)
		if !ok || seen[name] {
			return nil, false
		}
		seen[name] = true
		byteOf[pos/8] |= 1 << (pos % 8)
	}
	idx := make([]uint64, 0, len(byteOf))
	for i := range byteOf {
		idx = append(idx, i)
	}
	slices.Sort(idx)
	out := make([]uint64, len(idx))
	for i, k := range idx {
		out[i] = k<<8 | byteOf[k]
	}
	return out, true
}

func bitPosition(values []cambium.EnumValue, name string) (uint64, bool) {
	for _, v := range values {
		if v.Name() == name {
			pos := v.Value()
			if pos < 0 {
				return 0, false
			}
			return uint64(pos), true
		}
	}
	return 0, false
}
