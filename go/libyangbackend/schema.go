// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

//go:build cgo

// Schema-tree introspection.
//
// This file exposes both the legacy coarse SchemaTree (deprecated) and the new
// goyang-grade Module/SchemaNodeRef API. The new API is a value-type forest
// built over the frozen context.

package libyangbackend

import (
	"fmt"
	"iter"
	"strings"
	"unsafe"

	"github.com/signalbreak-labs/cambium/go/internal/libyang"
)

// SchemaNodeKind classifies a compiled YANG schema node.
// The discriminant order matches rust/cambium-core/src/schema.rs.
type SchemaNodeKind int

const (
	// SchemaNodeKindModule is the synthetic root wrapping a module's top-level nodes.
	SchemaNodeKindModule SchemaNodeKind = iota
	// SchemaNodeKindContainer is a container statement.
	SchemaNodeKindContainer
	// SchemaNodeKindLeaf is a leaf statement.
	SchemaNodeKindLeaf
	// SchemaNodeKindLeafList is a leaf-list statement.
	SchemaNodeKindLeafList
	// SchemaNodeKindList is a list statement.
	SchemaNodeKindList
	// SchemaNodeKindChoice is a choice statement.
	SchemaNodeKindChoice
	// SchemaNodeKindCase is a case statement.
	SchemaNodeKindCase
	// SchemaNodeKindAnyData is an anydata/anyxml statement.
	SchemaNodeKindAnyData
	// SchemaNodeKindRPC is an RPC statement.
	SchemaNodeKindRPC
	// SchemaNodeKindAction is an action statement.
	SchemaNodeKindAction
	// SchemaNodeKindInput is RPC/action input.
	SchemaNodeKindInput
	// SchemaNodeKindOutput is RPC/action output.
	SchemaNodeKindOutput
	// SchemaNodeKindNotification is a notification statement.
	SchemaNodeKindNotification
	// SchemaNodeKindUnknown is anything not mapped above.
	SchemaNodeKindUnknown
)

// Config is read-write vs read-only.
type Config int

const (
	// ConfigRw is config true.
	ConfigRw Config = iota
	// ConfigRo is config false.
	ConfigRo
)

// Status is the status substatement.
type Status int

const (
	// StatusCurrent is status current.
	StatusCurrent Status = iota
	// StatusDeprecated is status deprecated.
	StatusDeprecated
	// StatusObsolete is status obsolete.
	StatusObsolete
)

// OrderedBy is the ordering semantics for lists and leaf-lists.
type OrderedBy int

const (
	// OrderedBySystem is canonical, deterministic order.
	OrderedBySystem OrderedBy = iota
	// OrderedByUser is byte-exact insertion order.
	OrderedByUser
)

// LeafType is a coarse classification of a leaf/leaf-list value type.
type LeafType int

const (
	// LeafTypeString is a string-valued leaf.
	LeafTypeString LeafType = iota
	// LeafTypeInt is an integer-valued leaf (signed/unsigned/decimal64).
	LeafTypeInt
	// LeafTypeBool is a boolean leaf.
	LeafTypeBool
	// LeafTypeUnknown is a type not mapped above.
	LeafTypeUnknown
)

// BaseType is the precise built-in YANG base type for a leaf or leaf-list.
type BaseType int

// Built-in YANG base types; BaseTypeUnknown is the zero value for an
// unresolved or non-leaf node.
const (
	BaseTypeUnknown BaseType = iota
	BaseTypeString
	BaseTypeBoolean
	BaseTypeInt8
	BaseTypeInt16
	BaseTypeInt32
	BaseTypeInt64
	BaseTypeUint8
	BaseTypeUint16
	BaseTypeUint32
	BaseTypeUint64
	BaseTypeDecimal64
	BaseTypeEmpty
	BaseTypeBinary
	BaseTypeBits
	BaseTypeEnumeration
	BaseTypeIdentityRef
	BaseTypeInstanceIdentifier
	BaseTypeLeafRef
	BaseTypeUnion
)

// String returns the canonical YANG builtin name for the base type.
func (b BaseType) String() string {
	switch b {
	case BaseTypeString:
		return "string"
	case BaseTypeBoolean:
		return "boolean"
	case BaseTypeInt8:
		return "int8"
	case BaseTypeInt16:
		return "int16"
	case BaseTypeInt32:
		return "int32"
	case BaseTypeInt64:
		return "int64"
	case BaseTypeUint8:
		return "uint8"
	case BaseTypeUint16:
		return "uint16"
	case BaseTypeUint32:
		return "uint32"
	case BaseTypeUint64:
		return "uint64"
	case BaseTypeDecimal64:
		return "decimal64"
	case BaseTypeEmpty:
		return "empty"
	case BaseTypeBinary:
		return "binary"
	case BaseTypeBits:
		return "bits"
	case BaseTypeEnumeration:
		return "enumeration"
	case BaseTypeIdentityRef:
		return "identityref"
	case BaseTypeInstanceIdentifier:
		return "instance-identifier"
	case BaseTypeLeafRef:
		return "leafref"
	case BaseTypeUnion:
		return "union"
	default:
		return "unknown"
	}
}

// IntKind classifies signed/unsigned integer types.
type IntKind int

// Integer kinds covering the eight signed and unsigned YANG integer widths.
const (
	IntKindI8 IntKind = iota
	IntKindI16
	IntKindI32
	IntKindI64
	IntKindU8
	IntKindU16
	IntKindU32
	IntKindU64
)

// FractionDigits is the number of fractional digits for a decimal64 type.
type FractionDigits struct {
	value uint8
}

// NewFractionDigits creates a FractionDigits value. It returns false for values
// outside the YANG-permitted 1..18 range.
func NewFractionDigits(value uint8) (FractionDigits, bool) {
	if value < 1 || value > 18 {
		return FractionDigits{}, false
	}
	return FractionDigits{value: value}, true
}

// Value returns the raw 1..18 fraction-digits value.
func (f FractionDigits) Value() uint8 { return f.value }

// EnumValue is one enum or bit value in declaration order.
type EnumValue struct {
	name  string
	value int64
}

// Name returns the enum or bit name.
func (e EnumValue) Name() string { return e.name }

// Value returns the integer value (enum value or bit position).
func (e EnumValue) Value() int64 { return e.value }

// EnumDef is the definition of an enumeration type.
type EnumDef struct {
	values []EnumValue
}

// Values returns the enum values in declaration order.
func (d EnumDef) Values() []EnumValue { return d.values }

// BitsDef is the definition of a bits type.
type BitsDef struct {
	values []EnumValue
}

// Values returns the bit values in declaration order.
func (d BitsDef) Values() []EnumValue { return d.values }

// Pattern is a textual pattern constraint for strings.
type Pattern struct {
	regex    string
	appTag   *string
	inverted bool
}

// Regex returns the POSIX regular expression.
func (p Pattern) Regex() string { return p.regex }

// ErrorAppTag returns the error app-tag, if any.
func (p Pattern) ErrorAppTag() (string, bool) {
	if p.appTag == nil {
		return "", false
	}
	return *p.appTag, true
}

// IsInverted reports whether the pattern uses invert-match.
func (p Pattern) IsInverted() bool { return p.inverted }

// RangeBound is one bound of a numeric range or string/binary length constraint.
type RangeBound struct {
	min string
	max string
}

// Min returns the canonical minimum string.
func (r RangeBound) Min() string { return r.min }

// Max returns the canonical maximum string.
func (r RangeBound) Max() string { return r.max }

// ResolvedType is the sum-type interface for resolved leaf/leaf-list constraints.
// Concrete variants are ResolvedInt, ResolvedDecimal64, ResolvedBoolean,
// ResolvedEmpty, ResolvedBinary, ResolvedString, ResolvedEnumeration,
// ResolvedBits, ResolvedIdentityRef, ResolvedInstanceIdentifier,
// ResolvedLeafRef, ResolvedUnion, and ResolvedUnknown.
type ResolvedType interface {
	resolvedType()
}

// ResolvedInt is an integer type with optional range constraints.
type ResolvedInt struct {
	Kind  IntKind
	Range []RangeBound
}

func (ResolvedInt) resolvedType() {}

// ResolvedDecimal64 is a decimal64 type.
type ResolvedDecimal64 struct {
	fractionDigits FractionDigits
	Range          []RangeBound
}

// FractionDigits returns the number of fractional digits.
func (r ResolvedDecimal64) FractionDigits() FractionDigits { return r.fractionDigits }

func (ResolvedDecimal64) resolvedType() {}

// ResolvedBoolean is a boolean type.
type ResolvedBoolean struct{}

func (ResolvedBoolean) resolvedType() {}

// ResolvedEmpty is an empty type.
type ResolvedEmpty struct{}

func (ResolvedEmpty) resolvedType() {}

// ResolvedBinary is a binary type with optional length constraints.
type ResolvedBinary struct {
	Length []RangeBound
}

func (ResolvedBinary) resolvedType() {}

// ResolvedString is a string type with optional length and pattern constraints.
type ResolvedString struct {
	Length   []RangeBound
	Patterns []Pattern
}

func (ResolvedString) resolvedType() {}

// ResolvedEnumeration is an enumeration type.
type ResolvedEnumeration struct {
	def EnumDef
}

// Values returns the enum values in declaration order.
func (r ResolvedEnumeration) Values() []EnumValue { return r.def.Values() }

func (ResolvedEnumeration) resolvedType() {}

// ResolvedBits is a bits type.
type ResolvedBits struct {
	def BitsDef
}

// Values returns the bit values in declaration order.
func (r ResolvedBits) Values() []EnumValue { return r.def.Values() }

func (ResolvedBits) resolvedType() {}

// ResolvedIdentityRef is an identityref type.
type ResolvedIdentityRef struct {
	bases []Identity
}

// Bases returns the direct base identities.
func (r ResolvedIdentityRef) Bases() []Identity { return r.bases }

func (ResolvedIdentityRef) resolvedType() {}

// ResolvedInstanceIdentifier is an instance-identifier type.
type ResolvedInstanceIdentifier struct {
	RequireInstance bool
}

func (ResolvedInstanceIdentifier) resolvedType() {}

// ResolvedLeafRef is a leafref type.
type ResolvedLeafRef struct {
	target          *SchemaNodeRef
	realtype        *TypeInfo
	requireInstance bool
	path            string
}

// Target returns the resolved target schema node, if available.
func (r ResolvedLeafRef) Target() (SchemaNodeRef, bool) {
	if r.target == nil {
		return SchemaNodeRef{}, false
	}
	return *r.target, true
}

// Realtype returns the resolved target type, if available.
func (r ResolvedLeafRef) Realtype() (*TypeInfo, bool) {
	if r.realtype == nil {
		return nil, false
	}
	return r.realtype, true
}

// RequireInstance reports whether the referenced instance must exist.
func (r ResolvedLeafRef) RequireInstance() bool { return r.requireInstance }

// Path returns the raw leafref path expression as it appears in the YANG
// module, including any prefixes.
func (r ResolvedLeafRef) Path() (string, bool) {
	if r.path == "" {
		return "", false
	}
	return r.path, true
}

func (ResolvedLeafRef) resolvedType() {}

// ResolvedUnion is a union type.
type ResolvedUnion struct {
	members []TypeInfo
}

// Members returns the union member types.
func (r ResolvedUnion) Members() []TypeInfo { return r.members }

func (ResolvedUnion) resolvedType() {}

// ResolvedUnknown is an unmapped or unsupported type.
type ResolvedUnknown struct{}

func (ResolvedUnknown) resolvedType() {}

// TypeInfo is rich type information for a leaf or leaf-list.
type TypeInfo struct {
	base        BaseType
	typedefName *string
	resolved    ResolvedType
}

// Base returns the built-in base type.
func (t TypeInfo) Base() BaseType { return t.base }

// TypedefName returns the typedef name if this type is a derived typedef.
func (t TypeInfo) TypedefName() (string, bool) {
	if t.typedefName == nil {
		return "", false
	}
	return *t.typedefName, true
}

// Resolved returns the resolved constraints for this type.
func (t TypeInfo) Resolved() ResolvedType { return t.resolved }

// Extension is a compiled YANG extension instance attached to a schema node.
type Extension struct {
	name       string
	argument   *string
	moduleName string
}

// Name returns the extension name (without prefix).
func (e Extension) Name() string { return e.name }

// Argument returns the extension's argument value, if it has one.
func (e Extension) Argument() (string, bool) {
	if e.argument == nil {
		return "", false
	}
	return *e.argument, true
}

// ModuleName returns the name of the module that defines the extension.
func (e Extension) ModuleName() string { return e.moduleName }

// MustConstraint is a compiled must restriction.
type MustConstraint struct {
	cond         string
	errorMessage *string
	errorAppTag  *string
	description  *string
	reference    *string
}

// Expression returns the must XPath expression string.
func (m MustConstraint) Expression() string { return m.cond }

// ErrorMessage returns the configured error-message, if any.
func (m MustConstraint) ErrorMessage() (string, bool) {
	if m.errorMessage == nil {
		return "", false
	}
	return *m.errorMessage, true
}

// ErrorAppTag returns the configured error-app-tag, if any.
func (m MustConstraint) ErrorAppTag() (string, bool) {
	if m.errorAppTag == nil {
		return "", false
	}
	return *m.errorAppTag, true
}

// Description returns the description substatement, if any.
func (m MustConstraint) Description() (string, bool) {
	if m.description == nil {
		return "", false
	}
	return *m.description, true
}

// Reference returns the reference substatement, if any.
func (m MustConstraint) Reference() (string, bool) {
	if m.reference == nil {
		return "", false
	}
	return *m.reference, true
}

// WhenConstraint is a compiled when restriction.
type WhenConstraint struct {
	cond        string
	description *string
	reference   *string
}

// Expression returns the when XPath expression string.
func (w WhenConstraint) Expression() string { return w.cond }

// Description returns the description substatement, if any.
func (w WhenConstraint) Description() (string, bool) {
	if w.description == nil {
		return "", false
	}
	return *w.description, true
}

// Reference returns the reference substatement, if any.
func (w WhenConstraint) Reference() (string, bool) {
	if w.reference == nil {
		return "", false
	}
	return *w.reference, true
}

// UniqueConstraint is one unique specification on a list node.
type UniqueConstraint struct {
	forest      *schemaForest
	moduleIndex int
	indices     []int
}

// Leafs returns the leaf nodes that make up this unique constraint, in the
// order they appear in the unique statement.
func (u UniqueConstraint) Leafs() []SchemaNodeRef {
	out := make([]SchemaNodeRef, len(u.indices))
	for i, idx := range u.indices {
		out[i] = SchemaNodeRef{
			forest:      u.forest,
			moduleIndex: u.moduleIndex,
			nodeIndex:   idx,
		}
	}
	return out
}

// Deviation is one parsed deviate statement from a module that is the deviation
// source. It exposes the parsed statement only; post-deviation compiled values
// are not reconstructed.
type Deviation struct {
	targetPath           string
	normalizedTargetPath string
	sourceModule         string
	devType              string
	property             string
	newValue             string
	description          *string
	reference            *string
}

// TargetPath returns the absolute schema nodeid that the deviation targets,
// including any prefixes as they appear in the source module.
func (d Deviation) TargetPath() string { return d.targetPath }

// SourceModule returns the name of the module that defines this deviation.
func (d Deviation) SourceModule() string { return d.sourceModule }

// Type returns the deviate operation: "not-supported", "add", "replace", or
// "delete".
func (d Deviation) Type() string { return d.devType }

// Property returns the affected property for add/replace/delete deviations
// (for example "type", "units", "must", "unique", "default", "config",
// "mandatory", "min-elements", or "max-elements"). For not-supported
// deviations it returns an empty string.
func (d Deviation) Property() string { return d.property }

// NewValue returns the new value for the affected property, if any. For
// config/mandatory it returns "true" or "false"; for max-elements it returns
// "unbounded" when the value is 0. For not-supported deviations it returns an
// empty string.
func (d Deviation) NewValue() string { return d.newValue }

// Description returns the description substatement of the deviation, if any.
func (d Deviation) Description() (string, bool) {
	if d.description == nil {
		return "", false
	}
	return *d.description, true
}

// Reference returns the reference substatement of the deviation, if any.
func (d Deviation) Reference() (string, bool) {
	if d.reference == nil {
		return "", false
	}
	return *d.reference, true
}

// SchemaNode is one node in the legacy coarse compiled YANG schema tree.
type SchemaNode struct {
	name          string
	kind          SchemaNodeKind
	orderedByUser bool
	isKey         bool
	keyNames      []string
	leafType      LeafType
	children      []SchemaNode
}

// Name returns the node's name.
func (n *SchemaNode) Name() string { return n.name }

// Kind returns the node's kind.
func (n *SchemaNode) Kind() SchemaNodeKind { return n.kind }

// OrderedByUser reports whether the node is an ordered-by user list or leaf-list.
func (n *SchemaNode) OrderedByUser() bool { return n.orderedByUser }

// IsKey reports whether the node is a list key leaf.
func (n *SchemaNode) IsKey() bool { return n.isKey }

// KeyNames returns the names of a list's key leaves in key-statement order.
func (n *SchemaNode) KeyNames() []string { return n.keyNames }

// LeafType returns the coarse generated type for a leaf or leaf-list.
func (n *SchemaNode) LeafType() LeafType { return n.leafType }

// Children returns the child nodes in schema declaration order.
func (n *SchemaNode) Children() []SchemaNode { return n.children }

// SchemaTree is a compiled YANG schema tree for one module.
type SchemaTree struct {
	root     SchemaNode
	moduleNs string
}

// ModuleNs returns the module namespace (empty if none was provided).
func (t *SchemaTree) ModuleNs() string { return t.moduleNs }

// Root returns the synthetic module root.
func (t *SchemaTree) Root() *SchemaNode { return &t.root }

// Find locates a node by descending a path of names (e.g. ["top", "z"]).
func (t *SchemaTree) Find(path []string) *SchemaNode {
	cur := &t.root
	for _, seg := range path {
		found := false
		for i := range cur.children {
			if cur.children[i].name == seg {
				cur = &cur.children[i]
				found = true
				break
			}
		}
		if !found {
			return nil
		}
	}
	return cur
}

// PreOrder visits every node in depth-first pre-order.
func (t *SchemaTree) PreOrder(yield func(*SchemaNode) bool) {
	var dfs func(n *SchemaNode) bool
	dfs = func(n *SchemaNode) bool {
		if !yield(n) {
			return false
		}
		for i := range n.children {
			if !dfs(&n.children[i]) {
				return false
			}
		}
		return true
	}
	dfs(&t.root)
}

func kindFromRaw(kind string) SchemaNodeKind {
	switch kind {
	case "module":
		return SchemaNodeKindModule
	case "container":
		return SchemaNodeKindContainer
	case "leaf":
		return SchemaNodeKindLeaf
	case "leaflist":
		return SchemaNodeKindLeafList
	case "list":
		return SchemaNodeKindList
	case "choice":
		return SchemaNodeKindChoice
	case "case":
		return SchemaNodeKindCase
	case "anydata":
		return SchemaNodeKindAnyData
	case "rpc":
		return SchemaNodeKindRPC
	case "action":
		return SchemaNodeKindAction
	case "input":
		return SchemaNodeKindInput
	case "output":
		return SchemaNodeKindOutput
	case "notification":
		return SchemaNodeKindNotification
	default:
		return SchemaNodeKindUnknown
	}
}

func nodeFromRaw(raw libyang.RawSchemaNode) SchemaNode {
	children := make([]SchemaNode, len(raw.Children))
	for i, c := range raw.Children {
		children[i] = nodeFromRaw(c)
	}
	return SchemaNode{
		name:          raw.Name,
		kind:          kindFromRaw(raw.Kind),
		orderedByUser: raw.OrderedByUser,
		isKey:         raw.IsKey,
		keyNames:      raw.KeyNames,
		leafType:      leafTypeFromRaw(raw.LeafType),
		children:      children,
	}
}

func leafTypeFromRaw(s string) LeafType {
	switch s {
	case "string":
		return LeafTypeString
	case "int":
		return LeafTypeInt
	case "bool":
		return LeafTypeBool
	default:
		return LeafTypeUnknown
	}
}

func baseTypeFromRaw(raw libyang.RawBaseType) BaseType {
	switch raw {
	case libyang.RawBaseTypeString:
		return BaseTypeString
	case libyang.RawBaseTypeBoolean:
		return BaseTypeBoolean
	case libyang.RawBaseTypeInt8:
		return BaseTypeInt8
	case libyang.RawBaseTypeInt16:
		return BaseTypeInt16
	case libyang.RawBaseTypeInt32:
		return BaseTypeInt32
	case libyang.RawBaseTypeInt64:
		return BaseTypeInt64
	case libyang.RawBaseTypeUint8:
		return BaseTypeUint8
	case libyang.RawBaseTypeUint16:
		return BaseTypeUint16
	case libyang.RawBaseTypeUint32:
		return BaseTypeUint32
	case libyang.RawBaseTypeUint64:
		return BaseTypeUint64
	case libyang.RawBaseTypeDecimal64:
		return BaseTypeDecimal64
	case libyang.RawBaseTypeEmpty:
		return BaseTypeEmpty
	case libyang.RawBaseTypeBinary:
		return BaseTypeBinary
	case libyang.RawBaseTypeBits:
		return BaseTypeBits
	case libyang.RawBaseTypeEnumeration:
		return BaseTypeEnumeration
	case libyang.RawBaseTypeIdentityRef:
		return BaseTypeIdentityRef
	case libyang.RawBaseTypeInstanceIdentifier:
		return BaseTypeInstanceIdentifier
	case libyang.RawBaseTypeLeafRef:
		return BaseTypeLeafRef
	case libyang.RawBaseTypeUnion:
		return BaseTypeUnion
	default:
		return BaseTypeUnknown
	}
}

func intKindFromBase(base BaseType) (IntKind, bool) {
	switch base {
	case BaseTypeInt8:
		return IntKindI8, true
	case BaseTypeInt16:
		return IntKindI16, true
	case BaseTypeInt32:
		return IntKindI32, true
	case BaseTypeInt64:
		return IntKindI64, true
	case BaseTypeUint8:
		return IntKindU8, true
	case BaseTypeUint16:
		return IntKindU16, true
	case BaseTypeUint32:
		return IntKindU32, true
	case BaseTypeUint64:
		return IntKindU64, true
	default:
		return 0, false
	}
}

func rangeFromRaw(raw []libyang.RawRangeBound) []RangeBound {
	if len(raw) == 0 {
		return nil
	}
	out := make([]RangeBound, len(raw))
	for i, r := range raw {
		out[i] = RangeBound{min: r.Min, max: r.Max}
	}
	return out
}

func patternsFromRaw(raw []libyang.RawPattern) []Pattern {
	out := make([]Pattern, len(raw))
	for i, p := range raw {
		out[i] = Pattern{
			regex:    p.Regex,
			appTag:   p.ErrorAppTag,
			inverted: p.Inverted,
		}
	}
	return out
}

func enumValuesFromRaw(raw []libyang.RawEnumValue) []EnumValue {
	out := make([]EnumValue, len(raw))
	for i, v := range raw {
		out[i] = EnumValue{name: v.Name, value: v.Value}
	}
	return out
}

// SchemaTree returns the compiled schema tree for the loaded module named
// `module`. This is the legacy coarse view; prefer Context.Schema.
func (c *Context) SchemaTree(module string) (*SchemaTree, error) {
	raw, err := c.raw.SchemaTree(module)
	if err != nil {
		return nil, wrap("schema tree", err)
	}
	return &SchemaTree{root: nodeFromRaw(*raw), moduleNs: raw.ModuleNs}, nil
}

// Schema returns a borrowed module handle for the loaded module named `module`.
func (c *Context) Schema(module string) (Module, error) {
	f, err := c.schemaForest()
	if err != nil {
		return Module{}, wrap("schema", err)
	}
	idx, ok := f.moduleIndex(module)
	if !ok {
		return Module{}, &Error{Code: RuleCodeContext, Op: "schema", Err: fmt.Errorf("module not found: %s", module)}
	}
	return Module{forest: f, moduleIndex: idx}, nil
}

func (c *Context) buildForest() error {
	rawMods, err := c.raw.SchemaModules()
	if err != nil {
		return err
	}
	f := &schemaForest{
		modules:      make([]moduleData, 0, len(rawMods)),
		identityMap:  make(map[string]identityLoc),
		schemaPtrMap: make(map[unsafe.Pointer]nodeLoc),
	}
	for _, rm := range rawMods {
		f.addModule(rm.Info, rm.Root)
	}
	c.forest = f
	return nil
}

// =============================================================================
// Rich schema introspection API.
// =============================================================================

// Module is a compiled module handle borrowed from a frozen Context.
type Module struct {
	forest      *schemaForest
	moduleIndex int
}

// Name returns the module name.
func (m Module) Name() string {
	return m.forest.modules[m.moduleIndex].info.Name
}

// Namespace returns the module XML namespace.
func (m Module) Namespace() string {
	return m.forest.modules[m.moduleIndex].info.Namespace
}

// Prefix returns the module prefix.
func (m Module) Prefix() string {
	return m.forest.modules[m.moduleIndex].info.Prefix
}

// Revision returns the declared revision (YYYY-MM-DD) and true if present.
func (m Module) Revision() (string, bool) {
	rev := m.forest.modules[m.moduleIndex].info.Revision
	if rev == nil {
		return "", false
	}
	return *rev, true
}

// IsImplemented reports whether the module is implemented (not just imported).
func (m Module) IsImplemented() bool {
	return m.forest.modules[m.moduleIndex].info.IsImplemented
}

// Import is one import statement of a module.
type Import struct {
	Prefix   string
	Name     string
	Revision string
}

// Imports returns the module's import statements in declaration order.
func (m Module) Imports() []Import {
	raw := m.forest.modules[m.moduleIndex].info.Imports
	out := make([]Import, len(raw))
	for i, r := range raw {
		out[i] = Import{
			Prefix:   r.Prefix,
			Name:     r.Name,
			Revision: r.Revision,
		}
	}
	return out
}

// ResolvePrefix maps a data-model prefix to the module it identifies. An empty
// prefix resolves to the receiver module itself. If the prefix is not declared
// by an import and is not the module's own empty prefix, the second result is
// false.
func (m Module) ResolvePrefix(prefix string) (Module, bool) {
	if prefix == "" || prefix == m.Prefix() {
		return m, true
	}
	mod := &m.forest.modules[m.moduleIndex]
	for _, imp := range mod.info.Imports {
		if imp.Prefix != prefix {
			continue
		}
		idx, ok := m.forest.moduleIndexForImport(imp.Name, imp.Revision)
		if !ok {
			return Module{}, false
		}
		return Module{forest: m.forest, moduleIndex: idx}, true
	}
	return Module{}, false
}

// AugmentedBy returns the names of modules that augment this module.
func (m Module) AugmentedBy() []string {
	return append([]string(nil), m.forest.modules[m.moduleIndex].info.AugmentedBy...)
}

// DeviatedBy returns the names of modules that deviate this module.
func (m Module) DeviatedBy() []string {
	return append([]string(nil), m.forest.modules[m.moduleIndex].info.DeviatedBy...)
}

// Deviations returns the deviations defined by this module (i.e., this module is
// the deviation source). The result is in declaration order; each deviate
// property produces one entry, so a single deviation statement may yield
// multiple entries.
func (m Module) Deviations() []Deviation {
	return append([]Deviation(nil), m.forest.modules[m.moduleIndex].info.Deviations...)
}

// TopLevel returns the top-level data nodes in schema declaration order.
func (m Module) TopLevel() SchemaChildren {
	mod := &m.forest.modules[m.moduleIndex]
	return SchemaChildren{
		forest:      m.forest,
		moduleIndex: m.moduleIndex,
		indices:     mod.nodes[mod.root].children,
	}
}

// RPCs returns the module-level RPCs in schema declaration order.
func (m Module) RPCs() SchemaChildren {
	mod := &m.forest.modules[m.moduleIndex]
	return SchemaChildren{
		forest:      m.forest,
		moduleIndex: m.moduleIndex,
		indices:     mod.rpcRootIndices,
	}
}

// Actions returns the module-level actions in schema declaration order. In
// YANG 1.1 actions are not valid at module top-level, so this is normally empty,
// but it is exposed for completeness.
func (m Module) Actions() SchemaChildren {
	mod := &m.forest.modules[m.moduleIndex]
	return SchemaChildren{
		forest:      m.forest,
		moduleIndex: m.moduleIndex,
		indices:     mod.actionRootIndices,
	}
}

// Notifications returns the module-level notifications in schema declaration
// order.
func (m Module) Notifications() SchemaChildren {
	mod := &m.forest.modules[m.moduleIndex]
	return SchemaChildren{
		forest:      m.forest,
		moduleIndex: m.moduleIndex,
		indices:     mod.notificationRootIndices,
	}
}

// Identities returns the identities declared in this module.
func (m Module) Identities() iter.Seq[Identity] {
	mod := &m.forest.modules[m.moduleIndex]
	return func(yield func(Identity) bool) {
		for i := range mod.identities {
			if !yield(Identity{forest: m.forest, moduleIndex: m.moduleIndex, identityIndex: i}) {
				return
			}
		}
	}
}

// FindPath locates a schema node by schema path (e.g. /module:container/leaf).
// Qualified segments must use a prefix or module name that resolves in this
// module's prefix context and must match the found node's owner module.
func (m Module) FindPath(path string) (SchemaNodeRef, error) {
	ref, ok := m.forest.findPath(m.moduleIndex, path)
	if !ok {
		return SchemaNodeRef{}, fmt.Errorf("schema path not found: %s", path)
	}
	return ref, nil
}

// Identity is a compiled YANG identity borrowed from the frozen context.
type Identity struct {
	forest        *schemaForest
	moduleIndex   int
	identityIndex int
}

func (id Identity) data() *identityData {
	return &id.forest.modules[id.moduleIndex].identities[id.identityIndex]
}

// Name returns the identity name.
func (id Identity) Name() string {
	return id.data().name
}

// Module returns the owning module.
func (id Identity) Module() Module {
	return Module{forest: id.forest, moduleIndex: id.moduleIndex}
}

// Bases returns the direct base identities.
func (id Identity) Bases() []Identity {
	data := id.data()
	out := make([]Identity, 0, len(data.baseNames))
	for _, name := range data.baseNames {
		if base, ok := id.forest.findIdentity(name); ok {
			out = append(out, base)
		}
	}
	return out
}

// Derived returns the transitive derived-identity closure in depth-first order.
func (id Identity) Derived() []Identity {
	data := id.data()
	var out []Identity
	visited := make(map[identityLoc]bool)
	var stack []Identity
	for _, name := range data.derivedNames {
		if derived, ok := id.forest.findIdentity(name); ok {
			stack = append(stack, derived)
		}
	}
	for len(stack) > 0 {
		cur := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		loc := identityLoc{moduleIndex: cur.moduleIndex, identityIndex: cur.identityIndex}
		if visited[loc] {
			continue
		}
		visited[loc] = true
		for _, name := range cur.data().derivedNames {
			if derived, ok := cur.forest.findIdentity(name); ok {
				stack = append(stack, derived)
			}
		}
		out = append(out, cur)
	}
	return out
}

// SchemaNodeRef is a borrowed handle to a compiled schema node.
type SchemaNodeRef struct {
	forest      *schemaForest
	moduleIndex int
	nodeIndex   int
}

func (n SchemaNodeRef) data() *nodeData {
	return &n.forest.modules[n.moduleIndex].nodes[n.nodeIndex]
}

// Name returns the node name.
func (n SchemaNodeRef) Name() string {
	return n.data().name
}

// Kind returns the node kind.
func (n SchemaNodeRef) Kind() SchemaNodeKind {
	return n.data().kind
}

// Module returns the owning module.
func (n SchemaNodeRef) Module() Module {
	data := n.data()
	if data.ownerModuleName != "" {
		if idx, ok := n.forest.moduleIndexForImport(data.ownerModuleName, data.ownerModuleRevision); ok {
			return Module{forest: n.forest, moduleIndex: idx}
		}
	}
	return Module{forest: n.forest, moduleIndex: n.moduleIndex}
}

// Description returns the description substatement, if any.
func (n SchemaNodeRef) Description() (string, bool) {
	if n.data().description == nil {
		return "", false
	}
	return *n.data().description, true
}

// Reference returns the reference substatement, if any.
func (n SchemaNodeRef) Reference() (string, bool) {
	if n.data().reference == nil {
		return "", false
	}
	return *n.data().reference, true
}

// Status returns the status flag.
func (n SchemaNodeRef) Status() Status {
	return n.data().status
}

// Config returns the config flag.
func (n SchemaNodeRef) Config() Config {
	return n.data().config
}

// IsMandatory reports whether the node is mandatory true.
func (n SchemaNodeRef) IsMandatory() bool {
	return n.data().mandatory
}

// IsPresenceContainer reports whether the node is a presence container.
func (n SchemaNodeRef) IsPresenceContainer() bool {
	return n.data().presence
}

// OrderedBy returns the ordering semantics.
func (n SchemaNodeRef) OrderedBy() OrderedBy {
	return n.data().orderedBy
}

// IsListKey reports whether the node is a list key leaf.
func (n SchemaNodeRef) IsListKey() bool {
	return n.data().isKey
}

// IsLeaf reports whether the node is a leaf.
func (n SchemaNodeRef) IsLeaf() bool {
	return n.Kind() == SchemaNodeKindLeaf
}

// IsLeafList reports whether the node is a leaf-list.
func (n SchemaNodeRef) IsLeafList() bool {
	return n.Kind() == SchemaNodeKindLeafList
}

// IsContainer reports whether the node is a container.
func (n SchemaNodeRef) IsContainer() bool {
	return n.Kind() == SchemaNodeKindContainer
}

// IsList reports whether the node is a list.
func (n SchemaNodeRef) IsList() bool {
	return n.Kind() == SchemaNodeKindList
}

// IsChoice reports whether the node is a choice.
func (n SchemaNodeRef) IsChoice() bool {
	return n.Kind() == SchemaNodeKindChoice
}

// IsCase reports whether the node is a case.
func (n SchemaNodeRef) IsCase() bool {
	return n.Kind() == SchemaNodeKindCase
}

// IsRPC reports whether the node is an RPC statement.
func (n SchemaNodeRef) IsRPC() bool {
	return n.Kind() == SchemaNodeKindRPC
}

// IsAction reports whether the node is an action statement.
func (n SchemaNodeRef) IsAction() bool {
	return n.Kind() == SchemaNodeKindAction
}

// IsNotification reports whether the node is a notification statement.
func (n SchemaNodeRef) IsNotification() bool {
	return n.Kind() == SchemaNodeKindNotification
}

// IsDir reports whether the node can contain children in the goyang Entry
// model: container, list, choice, case, rpc, action, input, output, and
// notification.
func (n SchemaNodeRef) IsDir() bool {
	switch n.Kind() {
	case SchemaNodeKindContainer, SchemaNodeKindList, SchemaNodeKindChoice,
		SchemaNodeKindCase, SchemaNodeKindRPC, SchemaNodeKindAction,
		SchemaNodeKindInput, SchemaNodeKindOutput, SchemaNodeKindNotification:
		return true
	}
	return false
}

// ReadOnly reports whether the node is config false.
func (n SchemaNodeRef) ReadOnly() bool {
	return n.Config() == ConfigRo
}

// Namespace returns the XML namespace of the owning module.
func (n SchemaNodeRef) Namespace() string {
	if ns := n.data().ownerNamespace; ns != "" {
		return ns
	}
	return n.Module().Namespace()
}

// ListKeys returns a list's key leaves in key-statement order.
func (n SchemaNodeRef) ListKeys() SchemaChildren {
	indices := n.data().keyIndices
	return SchemaChildren{
		forest:      n.forest,
		moduleIndex: n.moduleIndex,
		indices:     indices,
	}
}

// KeyNames returns the names of a list's key leaves in key-statement order.
// It is a convenience over ListKeys for callers that only need the names.
func (n SchemaNodeRef) KeyNames() []string {
	keys := n.ListKeys()
	out := make([]string, keys.Len())
	for i := 0; i < keys.Len(); i++ {
		k, _ := keys.Get(i)
		out[i] = k.Name()
	}
	return out
}

// MinElements returns min-elements and true if set.
func (n SchemaNodeRef) MinElements() (uint32, bool) {
	if n.data().minElements == nil {
		return 0, false
	}
	return *n.data().minElements, true
}

// MaxElements returns max-elements and true if set.
func (n SchemaNodeRef) MaxElements() (uint32, bool) {
	if n.data().maxElements == nil {
		return 0, false
	}
	return *n.data().maxElements, true
}

// LeafType returns rich type information for a leaf or leaf-list.
func (n SchemaNodeRef) LeafType() (TypeInfo, bool) {
	data := n.data()
	if data.kind != SchemaNodeKindLeaf && data.kind != SchemaNodeKindLeafList {
		return TypeInfo{}, false
	}
	return n.forest.resolveType(data.rawTypeInfo), true
}

// Units returns the units substatement, if any.
func (n SchemaNodeRef) Units() (string, bool) {
	if n.data().units == nil {
		return "", false
	}
	return *n.data().units, true
}

// DataChildren returns the children that can appear in a data tree.
//
// With flattenChoices false it returns direct data children, including choice
// and case nodes. With flattenChoices true, choice and case nodes are
// transparently skipped so the returned slice contains only non-choice/case data
// descendants in schema declaration order. Operation nodes are excluded.
func (n SchemaNodeRef) DataChildren(flattenChoices bool) SchemaChildren {
	var indices []int
	var walk func(idx int)
	walk = func(idx int) {
		node := &n.forest.modules[n.moduleIndex].nodes[idx]
		if !isDataChildKind(node.kind) {
			return
		}
		switch node.kind {
		case SchemaNodeKindChoice, SchemaNodeKindCase:
			if flattenChoices {
				for _, c := range node.children {
					walk(c)
				}
				return
			}
		}
		indices = append(indices, idx)
	}
	for _, c := range n.data().children {
		walk(c)
	}
	return SchemaChildren{
		forest:      n.forest,
		moduleIndex: n.moduleIndex,
		indices:     indices,
	}
}

func isDataChildKind(kind SchemaNodeKind) bool {
	switch kind {
	case SchemaNodeKindContainer, SchemaNodeKindList, SchemaNodeKindLeaf,
		SchemaNodeKindLeafList, SchemaNodeKindAnyData,
		SchemaNodeKindChoice, SchemaNodeKindCase:
		return true
	}
	return false
}

// IsChoiceDescendant reports whether the node lives under a choice (directly or
// transitively through cases).
func (n SchemaNodeRef) IsChoiceDescendant() bool {
	return n.data().isChoiceDescendant
}

// GroupingOrigin returns the grouping name if this node was instantiated from a
// uses of that grouping; otherwise the second result is false. Directly defined
// nodes and nodes compiled without LY_CTX_SET_PRIV_PARSED return false.
func (n SchemaNodeRef) GroupingOrigin() (string, bool) {
	if n.data().groupingOrigin == "" {
		return "", false
	}
	return n.data().groupingOrigin, true
}

// DeviationProvenance returns the deviations from any loaded module whose target
// path matches this node's path. Deviation target prefixes are resolved against
// the deviation source module before matching, so modules with the same local
// path do not collide. This is a convenience filter over the per-module
// Deviations() data; it scans every module in the context.
func (n SchemaNodeRef) DeviationProvenance() []Deviation {
	nodePath := n.Path()
	var out []Deviation
	for i := range n.forest.modules {
		for _, d := range n.forest.modules[i].info.Deviations {
			if d.normalizedTargetPath == nodePath {
				out = append(out, d)
			}
		}
	}
	return out
}

// Path returns a slash-separated schema path starting with the containing
// schema-tree module name, with no prefix colons (for example,
// "/module-name/container/leaf"). The synthetic module root resolves to
// "/module-name".
func (n SchemaNodeRef) Path() string {
	mod := &n.forest.modules[n.moduleIndex]
	rootIdx := mod.root
	if n.nodeIndex == rootIdx {
		return "/" + mod.info.Name
	}
	var names []string
	cur := n.nodeIndex
	for cur != rootIdx && cur >= 0 {
		names = append(names, mod.nodes[cur].name)
		cur = mod.nodes[cur].parentIndex
	}
	var b strings.Builder
	b.WriteString("/")
	b.WriteString(mod.info.Name)
	for i := len(names) - 1; i >= 0; i-- {
		b.WriteString("/")
		b.WriteString(names[i])
	}
	return b.String()
}

// Parent returns the parent schema node, or false for the synthetic module root
// or any malformed handle with no parent.
func (n SchemaNodeRef) Parent() (SchemaNodeRef, bool) {
	mod := &n.forest.modules[n.moduleIndex]
	if n.nodeIndex == mod.root {
		return SchemaNodeRef{}, false
	}
	p := n.data().parentIndex
	if p < 0 {
		return SchemaNodeRef{}, false
	}
	return SchemaNodeRef{forest: n.forest, moduleIndex: n.moduleIndex, nodeIndex: p}, true
}

// Ancestors returns the node's ancestors in root-to-leaf order, excluding the
// synthetic module root and the node itself.
func (n SchemaNodeRef) Ancestors() []SchemaNodeRef {
	mod := &n.forest.modules[n.moduleIndex]
	rootIdx := mod.root
	var rev []SchemaNodeRef
	cur := n.data().parentIndex
	for cur >= 0 && cur != rootIdx {
		rev = append(rev, SchemaNodeRef{forest: n.forest, moduleIndex: n.moduleIndex, nodeIndex: cur})
		cur = mod.nodes[cur].parentIndex
	}
	out := make([]SchemaNodeRef, len(rev))
	for i := range rev {
		out[i] = rev[len(rev)-1-i]
	}
	return out
}

// DefaultValue returns the first default value as a canonical string, if any.
// It is a convenience for DefaultValues()[0]; leaf-list nodes with multiple
// defaults should use DefaultValues.
func (n SchemaNodeRef) DefaultValue() (string, bool) {
	vals := n.data().defaultValues
	if len(vals) == 0 {
		return "", false
	}
	return vals[0], true
}

// DefaultValues returns all default values in declaration order. For leaf
// nodes this contains zero or one value; leaf-list nodes may contain multiple.
func (n SchemaNodeRef) DefaultValues() []string {
	return append([]string(nil), n.data().defaultValues...)
}

// Extensions returns the compiled extension instances in declaration order.
func (n SchemaNodeRef) Extensions() []Extension {
	return append([]Extension(nil), n.data().extensions...)
}

// Extension looks up a compiled extension instance by name (without prefix).
func (n SchemaNodeRef) Extension(name string) (Extension, bool) {
	for _, e := range n.data().extensions {
		if e.name == name {
			return e, true
		}
	}
	return Extension{}, false
}

// Musts returns the compiled must restrictions in declaration order.
func (n SchemaNodeRef) Musts() []MustConstraint {
	return append([]MustConstraint(nil), n.data().musts...)
}

// Whens returns the compiled when restrictions in declaration order.
func (n SchemaNodeRef) Whens() []WhenConstraint {
	return append([]WhenConstraint(nil), n.data().whens...)
}

// UniqueConstraints returns the list's unique specifications. It returns nil for
// non-list nodes.
func (n SchemaNodeRef) UniqueConstraints() []UniqueConstraint {
	ucs := n.data().uniqueConstraints
	if len(ucs) == 0 {
		return nil
	}
	out := make([]UniqueConstraint, len(ucs))
	for i, indices := range ucs {
		cp := make([]int, len(indices))
		copy(cp, indices)
		out[i] = UniqueConstraint{
			forest:      n.forest,
			moduleIndex: n.moduleIndex,
			indices:     cp,
		}
	}
	return out
}

// Children returns child nodes for navigation. Data children are in schema
// declaration order; libyang exposes data children, actions, and notifications
// as separate compiled lists, so Children exposes them grouped in that order
// rather than preserving cross-class lexical interleaving from source YANG.
func (n SchemaNodeRef) Children() SchemaChildren {
	return SchemaChildren{
		forest:      n.forest,
		moduleIndex: n.moduleIndex,
		indices:     n.data().children,
	}
}

// SchemaChildren iterates schema node children in declaration order.
type SchemaChildren struct {
	forest      *schemaForest
	moduleIndex int
	indices     []int
}

// Len returns the number of children.
func (c SchemaChildren) Len() int {
	return len(c.indices)
}

// IsEmpty reports whether there are no children.
func (c SchemaChildren) IsEmpty() bool {
	return len(c.indices) == 0
}

// Get returns the child at index.
func (c SchemaChildren) Get(i int) (SchemaNodeRef, bool) {
	if i < 0 || i >= len(c.indices) {
		return SchemaNodeRef{}, false
	}
	return SchemaNodeRef{
		forest:      c.forest,
		moduleIndex: c.moduleIndex,
		nodeIndex:   c.indices[i],
	}, true
}

// Lookup finds a child by name, preserving declaration order on iteration.
func (c SchemaChildren) Lookup(name string) (SchemaNodeRef, bool) {
	mod := &c.forest.modules[c.moduleIndex]
	for _, idx := range c.indices {
		if mod.nodes[idx].name == name {
			return SchemaNodeRef{forest: c.forest, moduleIndex: c.moduleIndex, nodeIndex: idx}, true
		}
	}
	return SchemaNodeRef{}, false
}

// Iter yields children in declaration order.
func (c SchemaChildren) Iter() iter.Seq[SchemaNodeRef] {
	return func(yield func(SchemaNodeRef) bool) {
		for _, idx := range c.indices {
			if !yield(SchemaNodeRef{
				forest:      c.forest,
				moduleIndex: c.moduleIndex,
				nodeIndex:   idx,
			}) {
				return
			}
		}
	}
}

// =============================================================================
// Internal compiled-module forest.
// =============================================================================

type schemaForest struct {
	modules      []moduleData
	identityMap  map[string]identityLoc
	schemaPtrMap map[unsafe.Pointer]nodeLoc
}

type moduleData struct {
	info                    moduleInfo
	root                    int
	nodes                   []nodeData
	identities              []identityData
	rpcRootIndices          []int
	actionRootIndices       []int
	notificationRootIndices []int
}

type nodeData struct {
	name                 string
	kind                 SchemaNodeKind
	config               Config
	status               Status
	mandatory            bool
	presence             bool
	description          *string
	reference            *string
	units                *string
	defaultValues        []string
	minElements          *uint32
	maxElements          *uint32
	orderedBy            OrderedBy
	isKey                bool
	isChoiceDescendant   bool
	keyIndices           []int
	rawTypeInfo          libyang.RawTypeInfo
	children             []int
	parentIndex          int
	schemaPtr            unsafe.Pointer
	ownerModuleName      string
	ownerModuleRevision  string
	ownerNamespace       string
	extensions           []Extension
	musts                []MustConstraint
	whens                []WhenConstraint
	rawUniqueConstraints [][]unsafe.Pointer
	uniqueConstraints    [][]int
	groupingOrigin       string
}

type identityData struct {
	name         string
	moduleName   string
	baseNames    []string
	derivedNames []string
}

type identityLoc struct {
	moduleIndex   int
	identityIndex int
}

type nodeLoc struct {
	moduleIndex int
	nodeIndex   int
}

func configFromRaw(raw libyang.RawConfig) Config {
	if raw == libyang.RawConfigRo {
		return ConfigRo
	}
	return ConfigRw
}

func statusFromRaw(raw libyang.RawStatus) Status {
	switch raw {
	case libyang.RawStatusDeprecated:
		return StatusDeprecated
	case libyang.RawStatusObsolete:
		return StatusObsolete
	default:
		return StatusCurrent
	}
}

func (f *schemaForest) addModule(info libyang.RawModuleInfo, root libyang.RawSchemaNode) {
	moduleIndex := len(f.modules)
	var nodes []nodeData
	rootIdx := f.addNode(&nodes, root, -1, false)

	var rpcRootIndices, actionRootIndices, notificationRootIndices []int
	for _, rpc := range info.RPCs {
		rpcRootIndices = append(rpcRootIndices, f.addNode(&nodes, rpc, rootIdx, false))
	}
	for _, action := range info.Actions {
		actionRootIndices = append(actionRootIndices, f.addNode(&nodes, action, rootIdx, false))
	}
	for _, notif := range info.Notifications {
		notificationRootIndices = append(notificationRootIndices, f.addNode(&nodes, notif, rootIdx, false))
	}

	identities := make([]identityData, len(info.Identities))
	for i, raw := range info.Identities {
		baseNames := make([]string, 0, len(raw.Bases))
		for _, base := range raw.Bases {
			baseNames = append(baseNames, normalizeIdentityName(info, base))
		}
		identities[i] = identityData{
			name:         raw.Name,
			moduleName:   raw.ModuleName,
			baseNames:    baseNames,
			derivedNames: raw.Derived,
		}
		key := raw.ModuleName
		if key != "" {
			key += ":" + raw.Name
		} else {
			key = raw.Name
		}
		f.identityMap[key] = identityLoc{moduleIndex: moduleIndex, identityIndex: i}
	}
	for i, n := range nodes {
		if n.schemaPtr != nil {
			f.schemaPtrMap[n.schemaPtr] = nodeLoc{moduleIndex: moduleIndex, nodeIndex: i}
		}
	}
	for i := range nodes {
		if len(nodes[i].rawUniqueConstraints) == 0 {
			continue
		}
		resolved := make([][]int, len(nodes[i].rawUniqueConstraints))
		for ui, spec := range nodes[i].rawUniqueConstraints {
			resolved[ui] = make([]int, 0, len(spec))
			for _, ptr := range spec {
				if loc, ok := f.schemaPtrMap[ptr]; ok {
					resolved[ui] = append(resolved[ui], loc.nodeIndex)
				}
			}
		}
		nodes[i].uniqueConstraints = resolved
	}
	deviations := make([]Deviation, len(info.Deviations))
	for i, rd := range info.Deviations {
		deviations[i] = Deviation{
			targetPath:           rd.TargetPath,
			normalizedTargetPath: normalizeSchemaNodeID(info, rd.TargetPath),
			sourceModule:         rd.SourceModule,
			devType:              rd.Type,
			property:             rd.Property,
			newValue:             rd.NewValue,
			description:          rd.Description,
			reference:            rd.Reference,
		}
	}
	f.modules = append(f.modules, moduleData{
		info: moduleInfo{
			Name:          info.Name,
			Namespace:     info.Namespace,
			Prefix:        info.Prefix,
			Revision:      info.Revision,
			HasParsed:     info.HasParsed,
			IsImplemented: info.IsImplemented,
			Imports:       info.Imports,
			AugmentedBy:   info.AugmentedBy,
			DeviatedBy:    info.DeviatedBy,
			Deviations:    deviations,
		},
		root:                    rootIdx,
		nodes:                   nodes,
		identities:              identities,
		rpcRootIndices:          rpcRootIndices,
		actionRootIndices:       actionRootIndices,
		notificationRootIndices: notificationRootIndices,
	})
}

func normalizeIdentityName(info libyang.RawModuleInfo, name string) string {
	if name == "" {
		return ""
	}
	if prefix, local, ok := strings.Cut(name, ":"); ok {
		if moduleName, ok := resolveModuleNameForPrefix(info, prefix); ok {
			return moduleName + ":" + local
		}
		return name
	}
	return info.Name + ":" + name
}

func normalizeSchemaNodeID(info libyang.RawModuleInfo, path string) string {
	parts := strings.Split(path, "/")
	out := make([]string, 0, len(parts)+1)
	moduleName := info.Name
	seenFirst := false
	for _, part := range parts {
		if part == "" {
			continue
		}
		local := part
		if prefix, rest, ok := strings.Cut(part, ":"); ok {
			local = rest
			if !seenFirst {
				resolved, ok := resolveModuleNameForPrefix(info, prefix)
				if !ok {
					return ""
				}
				moduleName = resolved
			}
		}
		if !seenFirst {
			out = append(out, moduleName)
			seenFirst = true
		}
		out = append(out, local)
	}
	if !seenFirst {
		return ""
	}
	return "/" + strings.Join(out, "/")
}

func resolveModuleNameForPrefix(info libyang.RawModuleInfo, prefix string) (string, bool) {
	if prefix == "" || prefix == info.Prefix || prefix == info.Name {
		return info.Name, true
	}
	for _, imp := range info.Imports {
		if imp.Prefix == prefix || imp.Name == prefix {
			return imp.Name, true
		}
	}
	return "", false
}

func (f *schemaForest) addNode(nodes *[]nodeData, raw libyang.RawSchemaNode, parentIndex int, isChoiceDescendant bool) int {
	idx := len(*nodes)
	kind := kindFromRaw(raw.Kind)
	// Reserve the slot first so children can record this index as their parent.
	*nodes = append(*nodes, nodeData{})

	rawChildren := raw.Children
	children := make([]int, len(rawChildren))
	childChoiceDesc := isChoiceDescendant || kind == SchemaNodeKindChoice || kind == SchemaNodeKindCase
	for i, child := range rawChildren {
		children[i] = f.addNode(nodes, child, idx, childChoiceDesc)
	}
	keyIndices := make([]int, len(raw.KeyIndices))
	for i, local := range raw.KeyIndices {
		keyIndices[i] = children[local]
	}
	orderedBy := OrderedBySystem
	if raw.OrderedByUser {
		orderedBy = OrderedByUser
	}
	exts := make([]Extension, len(raw.Extensions))
	for i, re := range raw.Extensions {
		exts[i] = Extension{
			name:       re.Name,
			argument:   re.Argument,
			moduleName: re.ModuleName,
		}
	}
	musts := make([]MustConstraint, len(raw.Musts))
	for i, rm := range raw.Musts {
		musts[i] = MustConstraint{
			cond:         rm.Cond,
			errorMessage: rm.ErrorMessage,
			errorAppTag:  rm.ErrorAppTag,
			description:  rm.Description,
			reference:    rm.Reference,
		}
	}
	whens := make([]WhenConstraint, len(raw.Whens))
	for i, rw := range raw.Whens {
		whens[i] = WhenConstraint{
			cond:        rw.Cond,
			description: rw.Description,
			reference:   rw.Reference,
		}
	}
	(*nodes)[idx] = nodeData{
		name:                 raw.Name,
		kind:                 kind,
		config:               configFromRaw(raw.Config),
		status:               statusFromRaw(raw.Status),
		mandatory:            raw.Mandatory,
		presence:             raw.Presence,
		description:          raw.Description,
		reference:            raw.Reference,
		units:                raw.Units,
		defaultValues:        raw.DefaultValues,
		minElements:          raw.MinElements,
		maxElements:          raw.MaxElements,
		orderedBy:            orderedBy,
		isKey:                raw.IsKey,
		isChoiceDescendant:   isChoiceDescendant,
		keyIndices:           keyIndices,
		rawTypeInfo:          raw.TypeInfo,
		children:             children,
		parentIndex:          parentIndex,
		schemaPtr:            raw.SchemaPtr,
		ownerModuleName:      raw.OwnerModuleName,
		ownerModuleRevision:  raw.OwnerModuleRevision,
		ownerNamespace:       raw.OwnerModuleNs,
		extensions:           exts,
		musts:                musts,
		whens:                whens,
		rawUniqueConstraints: raw.UniqueConstraints,
		groupingOrigin:       raw.GroupingOrigin,
	}
	return idx
}

func (f *schemaForest) moduleIndex(name string) (int, bool) {
	for i, m := range f.modules {
		if m.info.Name == name || m.info.Namespace == name {
			return i, true
		}
	}
	return 0, false
}

func (f *schemaForest) moduleIndexForImport(name, revision string) (int, bool) {
	for i, m := range f.modules {
		if m.info.Name != name {
			continue
		}
		if revision == "" {
			return i, true
		}
		if m.info.Revision != nil && *m.info.Revision == revision {
			return i, true
		}
	}
	return 0, false
}

func (f *schemaForest) findIdentity(name string) (Identity, bool) {
	loc, ok := f.identityMap[name]
	if !ok {
		return Identity{}, false
	}
	return Identity{forest: f, moduleIndex: loc.moduleIndex, identityIndex: loc.identityIndex}, true
}

func (f *schemaForest) schemaRefByPtr(ptr unsafe.Pointer) (SchemaNodeRef, bool) {
	loc, ok := f.schemaPtrMap[ptr]
	if !ok {
		return SchemaNodeRef{}, false
	}
	return SchemaNodeRef{forest: f, moduleIndex: loc.moduleIndex, nodeIndex: loc.nodeIndex}, true
}

func (f *schemaForest) findPath(moduleIndex int, path string) (SchemaNodeRef, bool) {
	path = strings.TrimPrefix(path, "/")
	segments := strings.Split(path, "/")
	mod := &f.modules[moduleIndex]
	for len(segments) > 0 && segments[0] == "" {
		segments = segments[1:]
	}
	if len(segments) > 0 && !strings.Contains(segments[0], ":") &&
		(segments[0] == mod.info.Name || segments[0] == mod.info.Namespace) {
		segments = segments[1:]
	}
	cur := f.modules[moduleIndex].root
	for _, segment := range segments {
		if segment == "" {
			continue
		}
		name := segment
		ownerModule := ""
		if prefix, local, ok := strings.Cut(segment, ":"); ok {
			resolved, ok := f.resolvePathPrefix(mod, prefix)
			if !ok {
				return SchemaNodeRef{}, false
			}
			name = local
			ownerModule = resolved
		}
		found := false
		for _, childIdx := range f.modules[moduleIndex].nodes[cur].children {
			child := &f.modules[moduleIndex].nodes[childIdx]
			if child.name != name {
				continue
			}
			if ownerModule != "" {
				childOwner := child.ownerModuleName
				if childOwner == "" {
					childOwner = mod.info.Name
				}
				if childOwner != ownerModule {
					continue
				}
			}
			cur = childIdx
			found = true
			break
		}
		if !found {
			return SchemaNodeRef{}, false
		}
	}
	return SchemaNodeRef{forest: f, moduleIndex: moduleIndex, nodeIndex: cur}, true
}

func (f *schemaForest) resolvePathPrefix(mod *moduleData, prefix string) (string, bool) {
	if prefix == "" || prefix == mod.info.Prefix || prefix == mod.info.Name {
		return mod.info.Name, true
	}
	for _, imp := range mod.info.Imports {
		if imp.Prefix == prefix || imp.Name == prefix {
			return imp.Name, true
		}
	}
	for _, loaded := range f.modules {
		if loaded.info.Name == prefix {
			return loaded.info.Name, true
		}
	}
	return "", false
}

func (f *schemaForest) resolveType(raw libyang.RawTypeInfo) TypeInfo {
	base := baseTypeFromRaw(raw.BaseType)
	info := TypeInfo{
		base:        base,
		typedefName: raw.TypedefName,
	}
	switch base {
	case BaseTypeInt8, BaseTypeInt16, BaseTypeInt32, BaseTypeInt64,
		BaseTypeUint8, BaseTypeUint16, BaseTypeUint32, BaseTypeUint64:
		kind, _ := intKindFromBase(base)
		info.resolved = ResolvedInt{Kind: kind, Range: rangeFromRaw(raw.Range)}
	case BaseTypeDecimal64:
		fd := uint8(1)
		if raw.FractionDigits != nil {
			fd = *raw.FractionDigits
		}
		info.resolved = ResolvedDecimal64{
			fractionDigits: FractionDigits{value: fd},
			Range:          rangeFromRaw(raw.Range),
		}
	case BaseTypeString:
		info.resolved = ResolvedString{
			Length:   rangeFromRaw(raw.Length),
			Patterns: patternsFromRaw(raw.Patterns),
		}
	case BaseTypeBinary:
		info.resolved = ResolvedBinary{Length: rangeFromRaw(raw.Length)}
	case BaseTypeEnumeration:
		info.resolved = ResolvedEnumeration{def: EnumDef{values: enumValuesFromRaw(raw.EnumValues)}}
	case BaseTypeBits:
		info.resolved = ResolvedBits{def: BitsDef{values: enumValuesFromRaw(raw.BitValues)}}
	case BaseTypeIdentityRef:
		var bases []Identity
		for _, name := range raw.IdentityBases {
			if baseIdent, ok := f.findIdentity(name); ok {
				bases = append(bases, baseIdent)
			}
		}
		info.resolved = ResolvedIdentityRef{bases: bases}
	case BaseTypeInstanceIdentifier:
		req := true
		if raw.RequireInstance != nil {
			req = *raw.RequireInstance
		}
		info.resolved = ResolvedInstanceIdentifier{RequireInstance: req}
	case BaseTypeLeafRef:
		req := true
		if raw.RequireInstance != nil {
			req = *raw.RequireInstance
		}
		var realtype *TypeInfo
		if raw.LeafrefRealtype != nil {
			rt := f.resolveType(*raw.LeafrefRealtype)
			realtype = &rt
		}
		var target *SchemaNodeRef
		if raw.LeafrefTargetPtr != nil {
			if ref, ok := f.schemaRefByPtr(raw.LeafrefTargetPtr); ok {
				target = &ref
			}
		}
		info.resolved = ResolvedLeafRef{
			target:          target,
			realtype:        realtype,
			requireInstance: req,
			path:            raw.LeafrefPath,
		}
	case BaseTypeUnion:
		members := make([]TypeInfo, len(raw.UnionTypes))
		for i, member := range raw.UnionTypes {
			members[i] = f.resolveType(member)
		}
		info.resolved = ResolvedUnion{members: members}
	case BaseTypeBoolean:
		info.resolved = ResolvedBoolean{}
	case BaseTypeEmpty:
		info.resolved = ResolvedEmpty{}
	default:
		info.resolved = ResolvedUnknown{}
	}
	return info
}

type moduleInfo struct {
	Name          string
	Namespace     string
	Prefix        string
	Revision      *string
	HasParsed     bool
	IsImplemented bool
	Imports       []libyang.RawImport
	AugmentedBy   []string
	DeviatedBy    []string
	Deviations    []Deviation
}
