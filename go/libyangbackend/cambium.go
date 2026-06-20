//go:build cgo

// Package libyangbackend is Cambium's optional Go backend/data tier over
// vendored libyang. The default go/cambium package is pure Go; users import this
// package explicitly when they need libyang-backed data parsing, validation, or
// serialization.
package libyangbackend

import (
	"fmt"

	"github.com/signalbreak-labs/cambium/go/internal/libyang"
)

// Format is an on-wire encoding. Every format is produced by a single ordered
// walk of the libyang sibling chain — never a native map/struct serializer.
type Format int

const (
	// FormatXML is RFC 7950 XML.
	FormatXML Format = iota
	// FormatJSON is RFC 7951 JSON.
	FormatJSON
	// FormatJSONIETF is RFC 7951 JSON as used by gNMI JSON_IETF.
	FormatJSONIETF
	// FormatLYB is libyang's binary LYB format.
	FormatLYB
)

func (f Format) raw() libyang.Format {
	switch f {
	case FormatJSON:
		return libyang.FormatJSON
	case FormatJSONIETF:
		return libyang.FormatJSONIETF
	case FormatLYB:
		return libyang.FormatLYB
	default:
		return libyang.FormatXML
	}
}

// ParseMode maps libyang's separable parse-option bitmap.
type ParseMode struct {
	// Strict rejects unknown data nodes (LYD_PARSE_STRICT).
	Strict bool
	// Opaque parses unknown data as opaque nodes (LYD_PARSE_OPAQ).
	Opaque bool
	// ParseOnly parses without validating (LYD_PARSE_ONLY).
	ParseOnly bool
	// NoState ignores state data during parsing (LYD_PARSE_NO_STATE).
	NoState bool
	// LybModUpdate skips LYB module revision checks (LYD_PARSE_LYB_SKIP_MODULE_CHECK).
	LybModUpdate bool
}

// ParseModeDataOnly parses without validating. It is a package-level var so
// existing call sites compile while ParseMode is no longer an iota enum.
var ParseModeDataOnly = ParseMode{ParseOnly: true}

func (m ParseMode) raw() (uint32, error) {
	if m.Strict && m.Opaque {
		return 0, &Error{Code: RuleCodeParse, Op: "parse", Err: fmt.Errorf("strict and opaque parse modes are mutually exclusive")}
	}
	var opts uint32
	if m.Strict {
		opts |= libyang.ParseStrict
	}
	if m.Opaque {
		opts |= libyang.ParseOpaque
	}
	if m.ParseOnly {
		opts |= libyang.ParseOnly
	}
	if m.NoState {
		opts |= libyang.ParseNoState
	}
	if m.LybModUpdate {
		opts |= libyang.ParseLybSkipModuleCheck
	}
	return opts, nil
}

// OpType selects the kind of operation document for ParseOp.
type OpType int

const (
	// OpTypeRPC is a YANG RPC.
	OpTypeRPC OpType = iota
	// OpTypeNotification is a YANG notification.
	OpTypeNotification
	// OpTypeReply is a YANG RPC/action reply.
	OpTypeReply
)

func (o OpType) raw() libyang.OpType {
	switch o {
	case OpTypeNotification:
		return libyang.OpNotification
	case OpTypeReply:
		return libyang.OpReply
	default:
		return libyang.OpRPC
	}
}

// WithDefaults controls how default-valued nodes are serialized.
type WithDefaults int

const (
	// WithDefaultsExplicit prints explicit nodes only (value 0).
	WithDefaultsExplicit WithDefaults = iota
	// WithDefaultsTrim suppresses nodes whose value equals the schema default.
	WithDefaultsTrim
	// WithDefaultsAll prints every default node, materialized or not.
	WithDefaultsAll
	// WithDefaultsAllTagged tags defaults with ietf-netconf-with-defaults.
	WithDefaultsAllTagged
)

// SerializeFlags controls serialization.
type SerializeFlags struct {
	// Siblings includes all siblings of the root (LYD_PRINT_WITHSIBLINGS).
	Siblings bool
	// Shrink removes insignificant whitespace (LYD_PRINT_SHRINK).
	Shrink bool
	// KeepEmptyContainers preserves empty non-presence containers.
	KeepEmptyContainers bool
	// WithDefaults selects how default-valued nodes are printed.
	WithDefaults WithDefaults
}

// DefaultSerializeFlags returns the conformance-golden profile. Go's zero value
// for SerializeFlags is NOT this default (Siblings would be false), so callers
// must use this function or set Siblings explicitly.
func DefaultSerializeFlags() SerializeFlags {
	return SerializeFlags{
		Siblings:            true,
		Shrink:              false,
		KeepEmptyContainers: false,
		WithDefaults:        WithDefaultsExplicit,
	}
}

// ValidateMode maps libyang's validation-option bitmap.
type ValidateMode struct {
	// NoState rejects state data (LYD_VALIDATE_NO_STATE).
	NoState bool
	// Present validates only nodes that exist (LYD_VALIDATE_PRESENT).
	Present bool
	// MultiError accumulates every error (LYD_VALIDATE_MULTI_ERROR).
	MultiError bool
}

// DiffOpts controls DataTree.Diff.
type DiffOpts struct {
	// Defaults includes default nodes in the diff (LYD_DIFF_DEFAULTS).
	Defaults bool
}

// MergeOpts controls DataTree.Merge.
type MergeOpts struct {
	// Destruct is currently inert; source is always borrowed and unmodified.
	Destruct bool
}

// RuleCode is a stable Cambium diagnostic code (see spec/rule-codes.md). The
// same failure yields the same code in Rust and Go.
type RuleCode string

const (
	// RuleCodeUnknown is the unclassified fallback (CAMBIUM_E0000).
	RuleCodeUnknown RuleCode = "CAMBIUM_E0000"
	// RuleCodeContext is a context/schema setup error (CAMBIUM_E0001).
	RuleCodeContext RuleCode = "CAMBIUM_E0001"
	// RuleCodeParse is a data parse error (CAMBIUM_E0002).
	RuleCodeParse RuleCode = "CAMBIUM_E0002"
	// RuleCodeValidate is an RFC-7950 validation error (CAMBIUM_E0003).
	RuleCodeValidate RuleCode = "CAMBIUM_E0003"
	// RuleCodeSerialize is a serialization error (CAMBIUM_E0004).
	RuleCodeSerialize RuleCode = "CAMBIUM_E0004"
	// RuleCodeOrderedList is a path/positional list-operation error (CAMBIUM_E0005).
	RuleCodeOrderedList RuleCode = "CAMBIUM_E0005"
	// RuleCodeDataPath is a data-tree access/mutation by path or XPath (CAMBIUM_E0006).
	RuleCodeDataPath RuleCode = "CAMBIUM_E0006"
	// RuleCodeStale is a data handle used after an invalidating mutation (CAMBIUM_E0007).
	RuleCodeStale RuleCode = "CAMBIUM_E0007"
)

func codeForOp(op string) RuleCode {
	switch op {
	case "new context", "set search path", "load module", "schema tree":
		return RuleCodeContext
	case "parse", "parse op":
		return RuleCodeParse
	case "validate", "merge":
		return RuleCodeValidate
	case "serialize":
		return RuleCodeSerialize
	case "user ordered list", "user ordered leaf list", "user ordered view",
		"insert first", "insert last",
		"insert before", "insert after", "move before", "move after":
		return RuleCodeOrderedList
	case "get", "try get", "exists", "select", "new path", "set value",
		"remove path", "unlink path", "add defaults", "root nodes",
		"children", "siblings", "schema", "value":
		return RuleCodeDataPath
	case "diff", "diff apply":
		return RuleCodeDataPath
	case "duplicate":
		return RuleCodeSerialize
	default:
		return RuleCodeUnknown
	}
}

// Error wraps a failed Cambium operation. It supports errors.Is/As via Unwrap
// and carries a stable RuleCode.
type Error struct {
	// Code is the stable rule code (see spec/rule-codes.md).
	Code RuleCode
	// Op is the operation that failed (e.g. "parse", "serialize").
	Op string
	// Err is the underlying cause.
	Err error
}

func (e *Error) Error() string {
	return fmt.Sprintf("[%s] %s: %v", e.Code, e.Op, e.Err)
}

// RuleCode returns the stable diagnostic code for this error.
func (e *Error) RuleCode() RuleCode { return e.Code }

// Unwrap returns the underlying error so errors.Is/As work.
func (e *Error) Unwrap() error { return e.Err }

func wrap(op string, err error) error {
	if err == nil {
		return nil
	}
	return &Error{Code: codeForOp(op), Op: op, Err: err}
}

// Context is a compiled YANG context: build-once-then-frozen.
type Context struct {
	raw    *libyang.RawContext
	forest *schemaForest
}

// NewContext creates an empty context.
func NewContext() (*Context, error) {
	raw, err := libyang.NewContext()
	if err != nil {
		return nil, wrap("new context", err)
	}
	return &Context{raw: raw}, nil
}

// SetSearchPath appends a directory to the module search path.
func (c *Context) SetSearchPath(path string) error {
	return wrap("set search path", c.raw.SetSearchPath(path))
}

// LoadModule loads a YANG module (all features) into the context.
func (c *Context) LoadModule(name string) error {
	if err := c.raw.LoadModule(name); err != nil {
		return wrap("load module", err)
	}
	c.forest = nil
	return nil
}

// LoadModuleFromPath loads a YANG module from a file path into the context.
// The path may be absolute or relative to the current working directory.
func (c *Context) LoadModuleFromPath(path string) error {
	if err := c.raw.LoadModuleFromPath(path); err != nil {
		return wrap("load module", err)
	}
	c.forest = nil
	return nil
}

// Modules returns handles for every implemented module in the context. The
// returned handles are borrowed from the frozen context.
func (c *Context) Modules() []Module {
	f, err := c.schemaForest()
	if err != nil {
		return nil
	}
	rawMods := c.raw.Modules()
	out := make([]Module, 0, len(rawMods))
	for _, rm := range rawMods {
		revision := ""
		if rm.Revision != nil {
			revision = *rm.Revision
		}
		if idx, ok := f.moduleIndexForImport(rm.Name, revision); ok {
			out = append(out, Module{forest: f, moduleIndex: idx})
		}
	}
	return out
}

// schemaForest lazily builds and returns the compiled schema forest.
func (c *Context) schemaForest() (*schemaForest, error) {
	if c.forest == nil {
		if err := c.buildForest(); err != nil {
			return nil, err
		}
	}
	return c.forest, nil
}

// Parse parses a whole data document against this context.
func (c *Context) Parse(format Format, mode ParseMode, data []byte) (*DataTree, error) {
	opts, err := mode.raw()
	if err != nil {
		return nil, err
	}
	raw, err := c.raw.ParseData(format.raw(), opts, data)
	if err != nil {
		return nil, wrap("parse", err)
	}
	return &DataTree{owner: c, raw: raw}, nil
}

// ParseOp parses an RPC, action, or notification against this context.
func (c *Context) ParseOp(format Format, opType OpType, data []byte) (*DataTree, error) {
	raw, err := c.raw.ParseOp(format.raw(), opType.raw(), data)
	if err != nil {
		return nil, wrap("parse op", err)
	}
	return &DataTree{owner: c, raw: raw}, nil
}

// NewData creates an empty in-memory data tree against this context.
func (c *Context) NewData() *DataTree {
	return &DataTree{owner: c, raw: c.raw.NewData()}
}

// Close frees the underlying context.
func (c *Context) Close() { c.raw.Close() }

// DataTree is a YANG data tree. Child order is the source of truth; any keyed
// index is a derived lookup cache and is never iterated for serialization.
// It keeps its owning Context alive.
type DataTree struct {
	owner *Context
	raw   *libyang.RawDataTree
}

// Serialize serializes the tree to bytes in the requested format. Element order
// is structural (a single ordered walk of the libyang sibling chain).
func (t *DataTree) Serialize(format Format, flags SerializeFlags) ([]byte, error) {
	var opts uint32
	if flags.Siblings {
		opts |= libyang.PrintWithSiblings
	}
	if flags.Shrink {
		opts |= libyang.PrintShrink
	}
	if flags.KeepEmptyContainers {
		opts |= libyang.PrintEmptyCont
	}
	switch flags.WithDefaults {
	case WithDefaultsTrim:
		opts |= libyang.PrintWDTrim
	case WithDefaultsAll:
		opts |= libyang.PrintWDAll
	case WithDefaultsAllTagged:
		opts |= libyang.PrintWDAllTag
	default:
		opts |= libyang.PrintWDExplicit
	}
	b, err := t.raw.Serialize(format.raw(), opts)
	return b, wrap("serialize", err)
}

// UserOrderedListAt returns a positional handle to the `ordered-by user` list
// at path.
func (t *DataTree) UserOrderedListAt(path string) (*UserOrderedList, error) {
	// Reject non-user-ordered targets (the I1 mechanism). libyang only partially
	// guards: lyd_insert_before/after reject a system-ordered node, but
	// lyd_insert_child (InsertLast) silently succeeds, so a positional handle on
	// a system-ordered list would violate the positional-only contract. Mirror
	// the Rust oracle and fail at the boundary with E0005.
	ref, err := t.Get(path)
	if err != nil {
		return nil, wrap("user ordered list", err)
	}
	schema, err := ref.Schema()
	if err != nil {
		return nil, wrap("user ordered list", err)
	}
	if schema.OrderedBy() != OrderedByUser {
		return nil, wrap("user ordered list", fmt.Errorf("not an ordered-by-user list: %q", path))
	}
	raw, err := t.raw.UserOrderedListAt(path)
	if err != nil {
		return nil, wrap("user ordered list", err)
	}
	return &UserOrderedList{owner: t, raw: raw}, nil
}

// Close frees the underlying tree.
func (t *DataTree) Close() { t.raw.Close() }

// Duplicate creates a deep, independent copy of the tree.
func (t *DataTree) Duplicate() (*DataTree, error) {
	raw, err := t.raw.Duplicate()
	if err != nil {
		return nil, wrap("duplicate", err)
	}
	return &DataTree{owner: t.owner, raw: raw}, nil
}

// DiffApply applies a diff to this tree in place.
func (t *DataTree) DiffApply(diff *DataDiff) error {
	if diff == nil || diff.raw == nil {
		return nil
	}
	if err := t.raw.DiffApply(diff.raw); err != nil {
		return wrap("diff apply", err)
	}
	return nil
}

// Merge merges source into this tree in place.
// A leaf that exists in both trees with different values is rejected with
// CAMBIUM_E0003 before any mutation.
func (t *DataTree) Merge(source *DataTree, opts MergeOpts) error {
	if t.owner != source.owner {
		return &Error{
			Code: RuleCodeDataPath,
			Op:   "merge",
			Err:  fmt.Errorf("merge requires both trees to share the same context"),
		}
	}
	// Conflict pre-scan: any leaf value present in both trees but differing is a
	// Cambium error, not a silent libyang overwrite.
	conflictDiff, err := t.Diff(source, DiffOpts{})
	if err != nil {
		return err
	}
	defer conflictDiff.Close()
	for _, edit := range conflictDiff.Edits() {
		if edit.Op() != DiffOpReplace {
			continue
		}
		srcVal, srcOK := edit.Value()
		if !srcOK {
			continue
		}
		base, ok := t.TryGet(edit.Path())
		if !ok {
			continue
		}
		baseVal, baseOK, err := base.ValueStr()
		if err != nil {
			return wrap("merge", err)
		}
		if baseOK && baseVal != srcVal {
			return &Error{
				Code: RuleCodeValidate,
				Op:   "merge",
				Err:  fmt.Errorf("merge conflict at %s", edit.Path()),
			}
		}
	}
	// MergeOpts is currently inert; source is always borrowed.
	_ = opts
	if err := t.raw.Merge(source.raw); err != nil {
		return wrap("merge", err)
	}
	return nil
}

// Diff computes the diff from this tree to another.
func (t *DataTree) Diff(other *DataTree, opts DiffOpts) (*DataDiff, error) {
	if t.owner != other.owner {
		return nil, &Error{
			Code: RuleCodeDataPath,
			Op:   "diff",
			Err:  fmt.Errorf("diff requires both trees to share the same context"),
		}
	}
	rawDiff, err := t.raw.Diff(other.raw, opts.Defaults)
	if err != nil {
		return nil, wrap("diff", err)
	}
	if rawDiff == nil {
		return &DataDiff{}, nil
	}
	rawEdits, err := rawDiff.Edits()
	if err != nil {
		return nil, wrap("diff", err)
	}
	edits := make([]DiffEdit, len(rawEdits))
	ordered := false
	for i, e := range rawEdits {
		edits[i] = DiffEdit{
			op:            DiffOp(e.Op),
			path:          e.Path,
			value:         e.Value,
			valueOK:       e.ValueOK,
			isUserOrdered: e.IsUserOrdered,
		}
		if e.IsUserOrdered {
			ordered = true
		}
	}
	return &DataDiff{raw: rawDiff, edits: edits, isUserOrdered: ordered}, nil
}

// DiffOp is the operation on a diff edit.
type DiffOp int

const (
	// DiffOpCreate creates a node.
	DiffOpCreate DiffOp = iota
	// DiffOpDelete deletes a node.
	DiffOpDelete
	// DiffOpReplace replaces a leaf/leaf-list value.
	DiffOpReplace
	// DiffOpNone is excluded from Edits iteration.
	DiffOpNone
)

// DiffEdit is one materialized diff edit.
type DiffEdit struct {
	op            DiffOp
	path          string
	value         string
	valueOK       bool
	isUserOrdered bool
}

// Op returns the operation to apply.
func (e DiffEdit) Op() DiffOp { return e.op }

// Path returns the absolute data path of the edited node.
func (e DiffEdit) Path() string { return e.path }

// Value returns the canonical value for leaf/leaf-list edits, if any.
func (e DiffEdit) Value() (string, bool) { return e.value, e.valueOK }

// IsUserOrdered reports whether the edit is on an ordered-by user list/leaf-list.
func (e DiffEdit) IsUserOrdered() bool { return e.isUserOrdered }

// DataDiff is an owned, apply-safe diff between two data trees.
type DataDiff struct {
	raw           *libyang.RawDataDiff
	edits         []DiffEdit
	isUserOrdered bool
}

// IsEmpty reports whether the diff contains no edits.
func (d *DataDiff) IsEmpty() bool { return len(d.edits) == 0 }

// IsOrderedByUser reports whether any edit carries user-ordered changes.
func (d *DataDiff) IsOrderedByUser() bool { return d.isUserOrdered }

// Edits returns the diff edits in apply-safe document order.
func (d *DataDiff) Edits() []DiffEdit { return d.edits }

// Serialize serializes the yang-patch-shaped diff tree.
func (d *DataDiff) Serialize(format Format) ([]byte, error) {
	if d.raw == nil {
		return []byte{}, nil
	}
	b, err := d.raw.Serialize(format.raw(), libyang.PrintWithSiblings)
	return b, wrap("serialize", err)
}

// Close frees the underlying diff tree immediately.
func (d *DataDiff) Close() {
	if d.raw != nil {
		d.raw.Close()
		d.raw = nil
	}
}

// UserOrderedList is a positional-only handle to an `ordered-by user` list.
//
// It deliberately has NO order-agnostic mutator (no Set, no Upsert, no index
// assignment): reordering a system-ordered node by mistake is impossible to
// express, mirroring the Rust UserOrderedList type. It keeps its owning
// DataTree alive.
type UserOrderedList struct {
	owner *DataTree
	raw   *libyang.RawUserOrderedList
}

// InsertFirst inserts entry as the first entry.
func (l *UserOrderedList) InsertFirst(entry *DataTree) error {
	return wrap("insert first", l.raw.InsertFirst(entry.raw))
}

// InsertLast inserts entry as the last entry.
func (l *UserOrderedList) InsertLast(entry *DataTree) error {
	return wrap("insert last", l.raw.InsertLast(entry.raw))
}

// InsertBefore inserts entry before the entry at index.
func (l *UserOrderedList) InsertBefore(index int, entry *DataTree) error {
	return wrap("insert before", l.raw.InsertBefore(index, entry.raw))
}

// InsertAfter inserts entry after the entry at index.
func (l *UserOrderedList) InsertAfter(index int, entry *DataTree) error {
	return wrap("insert after", l.raw.InsertAfter(index, entry.raw))
}

// MoveBefore moves the entry at what before the entry at point.
func (l *UserOrderedList) MoveBefore(what, point int) error {
	return wrap("move before", l.raw.MoveBefore(what, point))
}

// MoveAfter moves the entry at what after the entry at point.
func (l *UserOrderedList) MoveAfter(what, point int) error {
	return wrap("move after", l.raw.MoveAfter(what, point))
}
