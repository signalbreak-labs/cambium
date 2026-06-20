//go:build cgo

// Data-tree read / navigation API.
//
// This file adds NodeRef, NodeSet, DataSiblings, Value, Decimal64, and the
// read-only DataTree accessors. It mirrors rust/cambium-core/src/tree.rs and
// list.rs for the read side.

package libyangbackend

import (
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"

	"github.com/signalbreak-labs/cambium/go/internal/libyang"
)

// Decimal64 is a fixed-point decimal64 value.
type Decimal64 struct {
	raw            int64
	fractionDigits uint8
}

// NewDecimal64 creates a Decimal64 from its raw fixed-point integer and the
// number of fraction digits (1..=18).
func NewDecimal64(raw int64, fractionDigits uint8) Decimal64 {
	return Decimal64{raw: raw, fractionDigits: fractionDigits}
}

// Raw returns the fixed-point integer representation.
func (d Decimal64) Raw() int64 { return d.raw }

// FractionDigits returns the number of fractional digits.
func (d Decimal64) FractionDigits() uint8 { return d.fractionDigits }

// String returns the RFC-7950 canonical form (at least one fractional digit
// when fraction-digits > 0).
func (d Decimal64) String() string {
	if d.fractionDigits == 0 {
		return strconv.FormatInt(d.raw, 10)
	}
	divisor := int64(1)
	for i := uint8(0); i < d.fractionDigits; i++ {
		divisor *= 10
	}
	whole := d.raw / divisor
	frac := d.raw % divisor
	if frac < 0 {
		frac = -frac
	}
	padded := strconv.FormatInt(frac, 10)
	width := int(d.fractionDigits)
	if len(padded) < width {
		padded = strings.Repeat("0", width-len(padded)) + padded
	} else if len(padded) > width {
		padded = padded[:width]
	}
	trimmed := strings.TrimRight(padded, "0")
	if trimmed == "" {
		trimmed = "0"
	}
	if d.raw < 0 {
		return fmt.Sprintf("-%d.%s", -whole, trimmed)
	}
	return fmt.Sprintf("%d.%s", whole, trimmed)
}

// ValueKind classifies a typed YANG leaf/leaf-list value.
type ValueKind int

const (
	ValueKindNone ValueKind = iota
	ValueInt8
	ValueInt16
	ValueInt32
	ValueInt64
	ValueUint8
	ValueUint16
	ValueUint32
	ValueUint64
	ValueDecimal64
	ValueBool
	ValueEmpty
	ValueStr
	ValueBinary
	ValueEnum
	ValueBits
	ValueIdentityref
	ValueInstanceIdentifier
)

// Value is a typed YANG leaf/leaf-list value.
type Value struct {
	kind    ValueKind
	i64     int64
	u64     uint64
	str     string
	b       []byte
	bits    []string
	dec     Decimal64
	boolean bool
}

// NewPathOpts controls the behaviour of DataTree.NewPath.
type NewPathOpts struct {
	// Update replaces an existing leaf/list value instead of returning an error.
	Update bool
	// Output creates the path under an RPC/action output node.
	Output bool
	// Opaque creates opaque nodes for unknown data.
	Opaque bool
}

func (o NewPathOpts) raw() uint32 {
	var opts uint32
	if o.Update {
		opts |= libyang.NewPathUpdate
	}
	if o.Output {
		opts |= libyang.NewPathOutput
	}
	if o.Opaque {
		opts |= libyang.NewPathOpaque
	}
	return opts
}

// ImplicitOpts controls which implicit/default nodes AddDefaults materializes.
type ImplicitOpts struct {
	// NoState skips state data while adding defaults.
	NoState bool
	// Output targets RPC/action output instead of normal data tree.
	Output bool
}

func (o ImplicitOpts) raw() uint32 {
	var opts uint32
	if o.NoState {
		opts |= libyang.ImplicitNoState
	}
	if o.Output {
		opts |= libyang.ImplicitOutput
	}
	return opts
}

// NodeAddr is an opaque path-key handle returned by DataTree.NewPath. It
// identifies the created/updated node by its absolute data path and survives
// root re-anchoring.
type NodeAddr struct {
	path string
}

// Path returns the absolute data path of the node this handle addresses.
func (a NodeAddr) Path() string { return a.path }

// Kind returns the value kind.
func (v Value) Kind() ValueKind { return v.kind }

// Int8 returns the int8 value, if any.
func (v Value) Int8() (int8, bool) {
	if v.kind != ValueInt8 {
		return 0, false
	}
	return int8(v.i64), true
}

// Int16 returns the int16 value, if any.
func (v Value) Int16() (int16, bool) {
	if v.kind != ValueInt16 {
		return 0, false
	}
	return int16(v.i64), true
}

// Int32 returns the int32 value, if any.
func (v Value) Int32() (int32, bool) {
	if v.kind != ValueInt32 {
		return 0, false
	}
	return int32(v.i64), true
}

// Int64 returns the int64 value, if any.
func (v Value) Int64() (int64, bool) {
	if v.kind != ValueInt64 {
		return 0, false
	}
	return v.i64, true
}

// Uint8 returns the uint8 value, if any.
func (v Value) Uint8() (uint8, bool) {
	if v.kind != ValueUint8 {
		return 0, false
	}
	return uint8(v.u64), true
}

// Uint16 returns the uint16 value, if any.
func (v Value) Uint16() (uint16, bool) {
	if v.kind != ValueUint16 {
		return 0, false
	}
	return uint16(v.u64), true
}

// Uint32 returns the uint32 value, if any.
func (v Value) Uint32() (uint32, bool) {
	if v.kind != ValueUint32 {
		return 0, false
	}
	return uint32(v.u64), true
}

// Uint64 returns the uint64 value, if any.
func (v Value) Uint64() (uint64, bool) {
	if v.kind != ValueUint64 {
		return 0, false
	}
	return v.u64, true
}

// Decimal64 returns the decimal64 value, if any.
func (v Value) Decimal64() (Decimal64, bool) {
	if v.kind != ValueDecimal64 {
		return Decimal64{}, false
	}
	return v.dec, true
}

// Bool returns the boolean value, if any.
func (v Value) Bool() (bool, bool) {
	if v.kind != ValueBool {
		return false, false
	}
	return v.boolean, true
}

// Empty reports whether the value is the empty type.
func (v Value) Empty() bool { return v.kind == ValueEmpty }

// Str returns the string value, if this is a plain string.
func (v Value) Str() (string, bool) {
	if v.kind != ValueStr {
		return "", false
	}
	return v.str, true
}

// Binary returns the base64-decoded binary value, if any.
func (v Value) Binary() ([]byte, bool) {
	if v.kind != ValueBinary {
		return nil, false
	}
	return v.b, true
}

// Enum returns the enumeration name, if any.
func (v Value) Enum() (string, bool) {
	if v.kind != ValueEnum {
		return "", false
	}
	return v.str, true
}

// Bits returns the set of bit names, if any.
func (v Value) Bits() ([]string, bool) {
	if v.kind != ValueBits {
		return nil, false
	}
	return v.bits, true
}

// Identityref returns the identityref value, if any.
func (v Value) Identityref() (string, bool) {
	if v.kind != ValueIdentityref {
		return "", false
	}
	return v.str, true
}

// InstanceIdentifier returns the instance-identifier value, if any.
func (v Value) InstanceIdentifier() (string, bool) {
	if v.kind != ValueInstanceIdentifier {
		return "", false
	}
	return v.str, true
}

// NodeRef is a borrowed handle to one node in a DataTree.
type NodeRef struct {
	tree *DataTree
	path string
	gen  uint64
}

func (n NodeRef) assertValid(op string) error {
	if n.tree == nil {
		return &Error{Code: RuleCodeStale, Op: op, Err: fmt.Errorf("stale data handle")}
	}
	if n.tree.raw.Generation() != n.gen {
		return &Error{Code: RuleCodeStale, Op: op, Err: fmt.Errorf("stale data handle")}
	}
	return nil
}

// Path returns the absolute canonical path of this node.
func (n NodeRef) Path() (string, error) {
	if err := n.assertValid("path"); err != nil {
		return "", err
	}
	return n.path, nil
}

// Name returns the schema-local node name.
func (n NodeRef) Name() (string, error) {
	if err := n.assertValid("name"); err != nil {
		return "", err
	}
	return pathNodeName(n.path), nil
}

// ValueStr returns the canonical value string for a leaf or leaf-list.
// For non-term nodes it returns ("", false, nil).
func (n NodeRef) ValueStr() (string, bool, error) {
	if err := n.assertValid("value str"); err != nil {
		return "", false, err
	}
	return n.tree.raw.ValueStr(n.path)
}

// Value returns the typed value for a leaf or leaf-list.
// For non-term nodes it returns (Value{}, false, nil).
func (n NodeRef) Value() (Value, bool, error) {
	if err := n.assertValid("value"); err != nil {
		return Value{}, false, err
	}
	s, ok, err := n.tree.raw.ValueStr(n.path)
	if err != nil || !ok {
		return Value{}, false, err
	}
	schema, err := n.Schema()
	if err != nil {
		return Value{}, false, err
	}
	info, ok := schema.LeafType()
	if !ok {
		return Value{}, false, wrap("value", fmt.Errorf("node has no leaf type"))
	}
	v, err := parseValue(s, info)
	if err != nil {
		return Value{}, false, wrap("value", err)
	}
	return v, true, nil
}

// IsDefault reports whether this node was created from a default value.
func (n NodeRef) IsDefault() (bool, error) {
	if err := n.assertValid("is default"); err != nil {
		return false, err
	}
	return n.tree.raw.IsDefault(n.path)
}

// Schema returns the compiled schema node for this data node.
func (n NodeRef) Schema() (SchemaNodeRef, error) {
	if err := n.assertValid("schema"); err != nil {
		return SchemaNodeRef{}, err
	}
	ptr, ok, err := n.tree.raw.SchemaPtr(n.path)
	if err != nil {
		return SchemaNodeRef{}, wrap("schema", err)
	}
	if !ok {
		return SchemaNodeRef{}, wrap("schema", fmt.Errorf("schema not found"))
	}
	forest, err := n.tree.owner.schemaForest()
	if err != nil {
		return SchemaNodeRef{}, wrap("schema", err)
	}
	ref, ok := forest.schemaRefByPtr(ptr)
	if !ok {
		return SchemaNodeRef{}, wrap("schema", fmt.Errorf("schema not in forest"))
	}
	return ref, nil
}

// Children returns the immediate children in declaration order.
func (n NodeRef) Children() (DataSiblings, error) {
	if err := n.assertValid("children"); err != nil {
		return DataSiblings{}, err
	}
	infos, err := n.tree.raw.ChildrenOf(n.path)
	if err != nil {
		return DataSiblings{}, wrap("children", err)
	}
	return DataSiblings{tree: n.tree, paths: childPaths(infos), gen: n.tree.raw.Generation()}, nil
}

// Siblings returns this node and all its siblings in declaration order.
func (n NodeRef) Siblings() (DataSiblings, error) {
	if err := n.assertValid("siblings"); err != nil {
		return DataSiblings{}, err
	}
	infos, err := n.tree.raw.SiblingsOf(n.path)
	if err != nil {
		return DataSiblings{}, wrap("siblings", err)
	}
	return DataSiblings{tree: n.tree, paths: childPaths(infos), gen: n.tree.raw.Generation()}, nil
}

// AsUserOrdered returns a read-only positional view if this node is an
// ordered-by user list or leaf-list. For system-ordered targets it returns
// ok=false and no error.
func (n NodeRef) AsUserOrdered() (UserOrderedView, bool, error) {
	if err := n.assertValid("as user ordered"); err != nil {
		return UserOrderedView{}, false, err
	}
	schema, err := n.Schema()
	if err != nil {
		return UserOrderedView{}, false, err
	}
	if schema.OrderedBy() != OrderedByUser {
		return UserOrderedView{}, false, nil
	}
	return UserOrderedView{
		tree: n.tree,
		path: n.path,
		gen:  n.tree.raw.Generation(),
	}, true, nil
}

// NodeSet is an ordered set of nodes returned by an XPath selection.
type NodeSet struct {
	tree  *DataTree
	paths []string
	gen   uint64
}

// Len returns the number of matched nodes.
func (s NodeSet) Len() int { return len(s.paths) }

// IsEmpty reports whether the selection is empty.
func (s NodeSet) IsEmpty() bool { return len(s.paths) == 0 }

// Get returns the node at index.
func (s NodeSet) Get(i int) (NodeRef, bool) {
	if i < 0 || i >= len(s.paths) {
		return NodeRef{}, false
	}
	return NodeRef{tree: s.tree, path: s.paths[i], gen: s.gen}, true
}

// Iter returns all matched nodes in document order.
func (s NodeSet) Iter() []NodeRef {
	out := make([]NodeRef, len(s.paths))
	for i, p := range s.paths {
		out[i] = NodeRef{tree: s.tree, path: p, gen: s.gen}
	}
	return out
}

// DataSiblings is an ordered list of data-node siblings.
type DataSiblings struct {
	tree  *DataTree
	paths []string
	gen   uint64
}

// Len returns the number of siblings.
func (s DataSiblings) Len() int { return len(s.paths) }

// IsEmpty reports whether there are no siblings.
func (s DataSiblings) IsEmpty() bool { return len(s.paths) == 0 }

// Get returns the sibling at index.
func (s DataSiblings) Get(i int) (NodeRef, bool) {
	if i < 0 || i >= len(s.paths) {
		return NodeRef{}, false
	}
	return NodeRef{tree: s.tree, path: s.paths[i], gen: s.gen}, true
}

// Iter returns all siblings in order.
func (s DataSiblings) Iter() []NodeRef {
	out := make([]NodeRef, len(s.paths))
	for i, p := range s.paths {
		out[i] = NodeRef{tree: s.tree, path: p, gen: s.gen}
	}
	return out
}

// Get returns the node at the given absolute data path. A missing path yields
// E0006 (DataPath).
func (t *DataTree) Get(path string) (NodeRef, error) {
	_, ok, err := t.raw.FindNode(path)
	if err != nil {
		return NodeRef{}, wrap("get", err)
	}
	if !ok {
		return NodeRef{}, wrap("get", fmt.Errorf("path not found: %s", path))
	}
	return NodeRef{tree: t, path: path, gen: t.raw.Generation()}, nil
}

// TryGet returns the node at path, or ok=false for a missing path.
func (t *DataTree) TryGet(path string) (NodeRef, bool) {
	_, ok, err := t.raw.FindNode(path)
	if err != nil || !ok {
		return NodeRef{}, false
	}
	return NodeRef{tree: t, path: path, gen: t.raw.Generation()}, true
}

// Exists reports whether a node exists at path.
func (t *DataTree) Exists(path string) bool {
	_, ok, _ := t.raw.FindNode(path)
	return ok
}

// Select evaluates an XPath against the tree and returns the matched nodes in
// document order.
func (t *DataTree) Select(xpath string) (NodeSet, error) {
	paths, err := t.raw.XPathPaths(xpath)
	if err != nil {
		return NodeSet{}, wrap("select", err)
	}
	return NodeSet{tree: t, paths: paths, gen: t.raw.Generation()}, nil
}

// RootNodes returns the top-level data nodes in declaration order.
func (t *DataTree) RootNodes() (DataSiblings, error) {
	infos, err := t.raw.RootNodes()
	if err != nil {
		return DataSiblings{}, wrap("root nodes", err)
	}
	return DataSiblings{tree: t, paths: childPaths(infos), gen: t.raw.Generation()}, nil
}

// UserOrderedView is a read-only positional view of an ordered-by user list or
// leaf-list.
type UserOrderedView struct {
	tree *DataTree
	path string
	gen  uint64
}

func (v UserOrderedView) assertValid(op string) error {
	if v.tree == nil || v.tree.raw.Generation() != v.gen {
		return &Error{Code: RuleCodeStale, Op: op, Err: fmt.Errorf("stale data handle")}
	}
	return nil
}

func (v UserOrderedView) entryPaths() ([]string, error) {
	if err := v.assertValid("user ordered view"); err != nil {
		return nil, err
	}
	// The representative node is one entry; its siblings (including itself) are
	// the list entries in insertion order.
	infos, err := v.tree.raw.SiblingsOf(v.path)
	if err != nil {
		return nil, wrap("user ordered view", err)
	}
	return childPaths(infos), nil
}

// Len returns the number of entries.
func (v UserOrderedView) Len() (int, error) {
	paths, err := v.entryPaths()
	if err != nil {
		return 0, err
	}
	return len(paths), nil
}

// IsEmpty reports whether there are no entries.
func (v UserOrderedView) IsEmpty() (bool, error) {
	n, err := v.Len()
	if err != nil {
		return false, err
	}
	return n == 0, nil
}

// Get returns the entry at index.
func (v UserOrderedView) Get(i int) (NodeRef, bool, error) {
	paths, err := v.entryPaths()
	if err != nil {
		return NodeRef{}, false, err
	}
	if i < 0 || i >= len(paths) {
		return NodeRef{}, false, nil
	}
	return NodeRef{tree: v.tree, path: paths[i], gen: v.gen}, true, nil
}

// Iter returns all entries in insertion order.
func (v UserOrderedView) Iter() ([]NodeRef, error) {
	paths, err := v.entryPaths()
	if err != nil {
		return nil, err
	}
	out := make([]NodeRef, len(paths))
	for i, p := range paths {
		out[i] = NodeRef{tree: v.tree, path: p, gen: v.gen}
	}
	return out, nil
}

// FindByKey returns the positional index of the list entry whose key leaves
// match the supplied [{name,value}, ...] pairs.
func (v UserOrderedView) FindByKey(keys [][2]string) (int, bool, error) {
	entries, err := v.Iter()
	if err != nil {
		return 0, false, err
	}
	want := len(keys)
	for idx, entry := range entries {
		children, err := entry.Children()
		if err != nil {
			return 0, false, err
		}
		matched := 0
		for _, kv := range keys {
			wantName := kv[0]
			wantVal := kv[1]
			for i := 0; i < children.Len(); i++ {
				child, _ := children.Get(i)
				name, _ := child.Name()
				if name != wantName {
					continue
				}
				s, ok, err := child.ValueStr()
				if err != nil {
					return 0, false, err
				}
				if ok && s == wantVal {
					matched++
				}
				break
			}
		}
		if matched == want {
			return idx, true, nil
		}
	}
	return 0, false, nil
}

func childPaths(infos []libyang.RawChildInfo) []string {
	out := make([]string, len(infos))
	for i, info := range infos {
		out[i] = info.Path
	}
	return out
}

func pathNodeName(path string) string {
	last := path
	if i := strings.LastIndexByte(path, '/'); i >= 0 {
		last = path[i+1:]
	}
	if i := strings.IndexByte(last, '['); i >= 0 {
		last = last[:i]
	}
	if i := strings.IndexByte(last, ':'); i >= 0 {
		last = last[i+1:]
	}
	return last
}

func parseValue(s string, info TypeInfo) (Value, error) {
	base := info.Base()
	if base == BaseTypeLeafRef {
		if r, ok := info.Resolved().(ResolvedLeafRef); ok {
			if real, ok := r.Realtype(); ok {
				return parseValue(s, *real)
			}
		}
		return Value{kind: ValueStr, str: s}, nil
	}
	if base == BaseTypeUnion || base == BaseTypeUnknown {
		return Value{kind: ValueStr, str: s}, nil
	}
	switch base {
	case BaseTypeInt8:
		v, err := strconv.ParseInt(s, 10, 8)
		if err != nil {
			return Value{}, fmt.Errorf("invalid int8: %w", err)
		}
		return Value{kind: ValueInt8, i64: v}, nil
	case BaseTypeInt16:
		v, err := strconv.ParseInt(s, 10, 16)
		if err != nil {
			return Value{}, fmt.Errorf("invalid int16: %w", err)
		}
		return Value{kind: ValueInt16, i64: v}, nil
	case BaseTypeInt32:
		v, err := strconv.ParseInt(s, 10, 32)
		if err != nil {
			return Value{}, fmt.Errorf("invalid int32: %w", err)
		}
		return Value{kind: ValueInt32, i64: v}, nil
	case BaseTypeInt64:
		v, err := strconv.ParseInt(s, 10, 64)
		if err != nil {
			return Value{}, fmt.Errorf("invalid int64: %w", err)
		}
		return Value{kind: ValueInt64, i64: v}, nil
	case BaseTypeUint8:
		v, err := strconv.ParseUint(s, 10, 8)
		if err != nil {
			return Value{}, fmt.Errorf("invalid uint8: %w", err)
		}
		return Value{kind: ValueUint8, u64: v}, nil
	case BaseTypeUint16:
		v, err := strconv.ParseUint(s, 10, 16)
		if err != nil {
			return Value{}, fmt.Errorf("invalid uint16: %w", err)
		}
		return Value{kind: ValueUint16, u64: v}, nil
	case BaseTypeUint32:
		v, err := strconv.ParseUint(s, 10, 32)
		if err != nil {
			return Value{}, fmt.Errorf("invalid uint32: %w", err)
		}
		return Value{kind: ValueUint32, u64: v}, nil
	case BaseTypeUint64:
		v, err := strconv.ParseUint(s, 10, 64)
		if err != nil {
			return Value{}, fmt.Errorf("invalid uint64: %w", err)
		}
		return Value{kind: ValueUint64, u64: v}, nil
	case BaseTypeDecimal64:
		fd := uint8(1)
		if r, ok := info.Resolved().(ResolvedDecimal64); ok {
			fd = r.FractionDigits().Value()
		}
		dec, err := parseDecimal64(s, fd)
		if err != nil {
			return Value{}, err
		}
		return Value{kind: ValueDecimal64, dec: dec}, nil
	case BaseTypeString:
		return Value{kind: ValueStr, str: s}, nil
	case BaseTypeBoolean:
		switch s {
		case "true":
			return Value{kind: ValueBool, boolean: true}, nil
		case "false":
			return Value{kind: ValueBool, boolean: false}, nil
		default:
			return Value{}, fmt.Errorf("invalid boolean: %s", s)
		}
	case BaseTypeEmpty:
		return Value{kind: ValueEmpty}, nil
	case BaseTypeBinary:
		b, err := base64.StdEncoding.DecodeString(s)
		if err != nil {
			return Value{}, fmt.Errorf("invalid base64: %w", err)
		}
		return Value{kind: ValueBinary, b: b}, nil
	case BaseTypeEnumeration:
		return Value{kind: ValueEnum, str: s}, nil
	case BaseTypeBits:
		var bits []string
		if s != "" {
			bits = strings.Fields(s)
		}
		return Value{kind: ValueBits, bits: bits}, nil
	case BaseTypeIdentityRef:
		return Value{kind: ValueIdentityref, str: s}, nil
	case BaseTypeInstanceIdentifier:
		return Value{kind: ValueInstanceIdentifier, str: s}, nil
	default:
		return Value{kind: ValueStr, str: s}, nil
	}
}

// NewPath creates or updates a node at `path`.
// A nil `value` creates an inner node (container/list); a non-nil value sets a
// leaf or leaf-list. The returned NodeAddr is an opaque path-key handle.
func (t *DataTree) NewPath(path string, value *string, opts NewPathOpts) (NodeAddr, error) {
	if err := t.raw.NewPath(path, value, opts.raw()); err != nil {
		return NodeAddr{}, wrap("new path", err)
	}
	return NodeAddr{path: path}, nil
}

// SetValue changes the value of an existing leaf or leaf-list at `path`.
// It returns true if the value (or default flag) changed and false if the
// value was identical (a true no-op).
func (t *DataTree) SetValue(path, value string) (bool, error) {
	changed, err := t.raw.SetValue(path, value)
	return changed, wrap("set value", err)
}

// RemovePath removes and frees the subtree at `path`.
func (t *DataTree) RemovePath(path string) error {
	return wrap("remove path", t.raw.RemovePath(path))
}

// UnlinkPath detaches the subtree at `path` and returns it as an owned tree.
// The detached tree shares this tree's context.
func (t *DataTree) UnlinkPath(path string) (*DataTree, error) {
	raw, err := t.raw.UnlinkPath(path)
	if err != nil {
		return nil, wrap("unlink path", err)
	}
	return &DataTree{owner: t.owner, raw: raw}, nil
}

// AddDefaults materializes implicit/default nodes according to `opts`.
func (t *DataTree) AddDefaults(opts ImplicitOpts) error {
	return wrap("add defaults", t.raw.AddDefaults(opts.raw()))
}

func parseDecimal64(s string, fractionDigits uint8) (Decimal64, error) {
	if fractionDigits == 0 {
		raw, err := strconv.ParseInt(s, 10, 64)
		if err != nil {
			return Decimal64{}, fmt.Errorf("invalid decimal64: %w", err)
		}
		return Decimal64{raw: raw}, nil
	}
	negative := strings.HasPrefix(s, "-")
	s = strings.TrimPrefix(s, "-")
	s = strings.TrimPrefix(s, "+")
	wholeStr, fracStr, _ := strings.Cut(s, ".")
	if wholeStr == "" && fracStr == "" {
		return Decimal64{}, fmt.Errorf("empty decimal64 value")
	}
	var whole int64
	if wholeStr != "" {
		v, err := strconv.ParseInt(wholeStr, 10, 64)
		if err != nil {
			return Decimal64{}, fmt.Errorf("invalid decimal64: %w", err)
		}
		whole = v
	}
	fracDigits := strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' {
			return r
		}
		return -1
	}, fracStr)
	width := int(fractionDigits)
	if len(fracDigits) < width {
		fracDigits += strings.Repeat("0", width-len(fracDigits))
	} else if len(fracDigits) > width {
		fracDigits = fracDigits[:width]
	}
	fracVal, err := strconv.ParseInt(fracDigits, 10, 64)
	if err != nil {
		return Decimal64{}, fmt.Errorf("invalid decimal64: %w", err)
	}
	divisor := int64(1)
	for i := uint8(0); i < fractionDigits; i++ {
		divisor *= 10
	}
	raw := whole*divisor + fracVal
	if negative {
		raw = -raw
	}
	return Decimal64{raw: raw, fractionDigits: fractionDigits}, nil
}
