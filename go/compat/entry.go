// Package compat provides a small cgo-free, goyang-shaped schema projection.
//
// The projection is read-only. Entry.Dir is available for familiar name lookup,
// but ordered traversal must use Children; Go map iteration is intentionally
// not part of the ordering contract.
package compat

import (
	"fmt"
	"io"
	"math"
	"math/big"
	"reflect"
	"sort"
	"strconv"
	"strings"

	"github.com/signalbreak-labs/cambium/go/cambium"
	upstream "github.com/signalbreak-labs/cambium/go/internal/yangparse/upstream/yang"
)

// TriState may be true, false, or unset.
type TriState int

const (
	TSUnset TriState = iota
	TSTrue
	TSFalse
)

// Value returns the boolean value of t. Unset is false.
func (t TriState) Value() bool { return t == TSTrue }

// String returns the goyang-compatible TriState spelling.
func (t TriState) String() string {
	switch t {
	case TSUnset:
		return "unset"
	case TSTrue:
		return "true"
	case TSFalse:
		return "false"
	default:
		return fmt.Sprintf("ts-%d", t)
	}
}

// Value is a minimal goyang-style scalar wrapper used by compatibility fields.
type Value struct {
	Name       string       `yang:"Name,nomerge"`
	Source     *Statement   `yang:"Statement,nomerge" json:",omitempty"`
	Parent     Node         `yang:"Parent,nomerge" json:"-"`
	Extensions []*Statement `yang:"Ext" json:",omitempty"`

	Description *Value `yang:"description" json:",omitempty"`
}

func (Value) Kind() string             { return "string" }
func (v *Value) ParentNode() Node      { return v.Parent }
func (v *Value) NName() string         { return v.Name }
func (v *Value) Statement() *Statement { return v.Source }
func (v *Value) Exts() []*Statement    { return v.Extensions }

type entryNode struct {
	kind       string
	name       string
	parent     Node
	source     *Statement
	extensions []*Statement
}

func (n *entryNode) Kind() string {
	if n == nil {
		return ""
	}
	return n.kind
}
func (n *entryNode) ParentNode() Node {
	if n == nil {
		return nil
	}
	return n.parent
}
func (n *entryNode) NName() string {
	if n == nil {
		return ""
	}
	return n.name
}
func (n *entryNode) Statement() *Statement {
	if n == nil {
		return nil
	}
	return n.source
}
func (n *entryNode) Exts() []*Statement {
	if n == nil {
		return nil
	}
	return append([]*Statement(nil), n.extensions...)
}

// TypeKind is the goyang-compatible enumeration of YANG built-in base types.
type TypeKind uint

const (
	Ynone TypeKind = iota
	Yint8
	Yint16
	Yint32
	Yint64
	Yuint8
	Yuint16
	Yuint32
	Yuint64
	Ybinary
	Ybits
	Ybool
	Ydecimal64
	Yempty
	Yenum
	Yidentityref
	YinstanceIdentifier
	Yleafref
	Ystring
	Yunion
)

// TypeKindToName maps TypeKind values to YANG built-in type names.
var TypeKindToName = map[TypeKind]string{
	Ynone:               "none",
	Yint8:               "int8",
	Yint16:              "int16",
	Yint32:              "int32",
	Yint64:              "int64",
	Yuint8:              "uint8",
	Yuint16:             "uint16",
	Yuint32:             "uint32",
	Yuint64:             "uint64",
	Ybinary:             "binary",
	Ybits:               "bits",
	Ybool:               "boolean",
	Ydecimal64:          "decimal64",
	Yempty:              "empty",
	Yenum:               "enumeration",
	Yidentityref:        "identityref",
	YinstanceIdentifier: "instance-identifier",
	Yleafref:            "leafref",
	Ystring:             "string",
	Yunion:              "union",
}

// TypeKindFromName maps YANG built-in type names to TypeKind values.
var TypeKindFromName = map[string]TypeKind{
	"none":                Ynone,
	"int8":                Yint8,
	"int16":               Yint16,
	"int32":               Yint32,
	"int64":               Yint64,
	"uint8":               Yuint8,
	"uint16":              Yuint16,
	"uint32":              Yuint32,
	"uint64":              Yuint64,
	"binary":              Ybinary,
	"bits":                Ybits,
	"boolean":             Ybool,
	"decimal64":           Ydecimal64,
	"empty":               Yempty,
	"enumeration":         Yenum,
	"identityref":         Yidentityref,
	"instance-identifier": YinstanceIdentifier,
	"leafref":             Yleafref,
	"string":              Ystring,
	"union":               Yunion,
}

// String returns the goyang-compatible TypeKind spelling.
func (k TypeKind) String() string {
	if name := TypeKindToName[k]; name != "" {
		return name
	}
	return fmt.Sprintf("unknown-type-%d", k)
}

const (
	// MaxEnum is the maximum value of a YANG enumeration.
	MaxEnum = 1<<31 - 1
	// MinEnum is the minimum value of a YANG enumeration.
	MinEnum = -1 << 31
	// MaxBitfieldSize is the maximum number of bits in a YANG bits type.
	MaxBitfieldSize = 1 << 32
)

// EnumType is a goyang-style enum/bit name-value mapping.
type EnumType struct {
	last     int64
	min      int64
	max      int64
	unique   bool
	ToString map[int64]string `json:",omitempty"`
	ToInt    map[string]int64 `json:",omitempty"`
}

// NewEnumType returns an initialized enumeration mapping.
func NewEnumType() *EnumType {
	return &EnumType{
		last:     -1,
		min:      MinEnum,
		max:      MaxEnum,
		unique:   true,
		ToString: map[int64]string{},
		ToInt:    map[string]int64{},
	}
}

// NewBitfield returns an initialized bits mapping.
func NewBitfield() *EnumType {
	return &EnumType{
		last:     -1,
		min:      0,
		max:      MaxBitfieldSize - 1,
		ToString: map[int64]string{},
		ToInt:    map[string]int64{},
	}
}

// Set assigns name to value.
func (e *EnumType) Set(name string, value int64) error {
	if e == nil {
		return fmt.Errorf("nil EnumType")
	}
	if _, ok := e.ToInt[name]; ok {
		return fmt.Errorf("field %s already assigned", name)
	}
	if oname, ok := e.ToString[value]; e.unique && ok {
		return fmt.Errorf("fields %s and %s conflict on value %d", name, oname, value)
	}
	if value < e.min {
		return fmt.Errorf("value %d for %s too small (minimum is %d)", value, name, e.min)
	}
	if value > e.max {
		return fmt.Errorf("value %d for %s too large (maximum is %d)", value, name, e.max)
	}
	e.ToString[value] = name
	e.ToInt[name] = value
	if value >= e.last {
		e.last = value
	}
	return nil
}

// SetNext assigns name to the next value after all previous values.
func (e *EnumType) SetNext(name string) error {
	if e == nil {
		return fmt.Errorf("nil EnumType")
	}
	if e.last == MaxEnum {
		return fmt.Errorf("enum %q must specify a value since previous enum is the maximum value allowed", name)
	}
	return e.Set(name, e.last+1)
}

// Name returns the name associated with value, or an empty string.
func (e *EnumType) Name(value int64) string {
	if e == nil {
		return ""
	}
	return e.ToString[value]
}

// Value returns the value associated with name, or 0.
func (e *EnumType) Value(name string) int64 {
	if e == nil {
		return 0
	}
	return e.ToInt[name]
}

// IsDefined reports whether name exists in e.
func (e *EnumType) IsDefined(name string) bool {
	if e == nil {
		return false
	}
	_, ok := e.ToInt[name]
	return ok
}

// Names returns enum or bit names sorted lexicographically.
func (e *EnumType) Names() []string {
	if e == nil {
		return nil
	}
	names := make([]string, 0, len(e.ToInt))
	for name := range e.ToInt {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// Values returns enum or bit values sorted numerically.
func (e *EnumType) Values() []int64 {
	if e == nil {
		return nil
	}
	values := make([]int64, 0, len(e.ToInt))
	for _, value := range e.ToInt {
		values = append(values, value)
	}
	sort.Slice(values, func(i, j int) bool { return values[i] < values[j] })
	return values
}

// NameMap returns a defensive copy of the name-to-value map.
func (e *EnumType) NameMap() map[string]int64 {
	if e == nil {
		return nil
	}
	out := make(map[string]int64, len(e.ToInt))
	for name, value := range e.ToInt {
		out[name] = value
	}
	return out
}

// ValueMap returns a defensive copy of the value-to-name map.
func (e *EnumType) ValueMap() map[int64]string {
	if e == nil {
		return nil
	}
	out := make(map[int64]string, len(e.ToString))
	for value, name := range e.ToString {
		out[value] = name
	}
	return out
}

// Identity is a goyang-style read-only identity projection.
type Identity struct {
	Name       string       `yang:"Name,nomerge"`
	Source     *Statement   `yang:"Statement,nomerge" json:"-"`
	Parent     Node         `yang:"Parent,nomerge" json:"-"`
	Extensions []*Statement `yang:"Ext" json:"-"`

	Base        []*Value    `yang:"base" json:"-"`
	Description *Value      `yang:"description" json:"-"`
	IfFeature   []*Value    `yang:"if-feature" json:"-"`
	Reference   *Value      `yang:"reference" json:"-"`
	Status      *Value      `yang:"status" json:"-"`
	Values      []*Identity `json:",omitempty"`
}

func (Identity) Kind() string { return "identity" }
func (i *Identity) ParentNode() Node {
	if i == nil {
		return nil
	}
	return i.Parent
}
func (i *Identity) NName() string {
	if i == nil {
		return ""
	}
	return i.Name
}
func (i *Identity) Statement() *Statement {
	if i == nil {
		return nil
	}
	return i.Source
}
func (i *Identity) Exts() []*Statement {
	if i == nil {
		return nil
	}
	return append([]*Statement(nil), i.Extensions...)
}

// PrefixedName returns the prefix-qualified identity name.
func (i *Identity) PrefixedName() string {
	if i == nil {
		return ""
	}
	root := RootNode(i)
	if root == nil || root.GetPrefix() == "" {
		return i.Name
	}
	return root.GetPrefix() + ":" + i.Name
}

// IsDefined reports whether a derived identity with name exists.
func (i *Identity) IsDefined(name string) bool {
	return i.GetValue(name) != nil
}

// GetValue returns the derived identity named name.
func (i *Identity) GetValue(name string) *Identity {
	if i == nil {
		return nil
	}
	for _, value := range i.Values {
		if value != nil && value.Name == name {
			return value
		}
	}
	return nil
}

// Number is a goyang-style integer or decimal64 range bound.
type Number struct {
	Value          uint64
	FractionDigits uint8
	Negative       bool
}

// IsDecimal reports whether n is a decimal64 value.
func (n Number) IsDecimal() bool {
	return n.FractionDigits != 0
}

// String returns n in YANG lexical notation.
func (n Number) String() string {
	out := strconv.FormatUint(n.Value, 10)
	if n.IsDecimal() {
		fd := int(n.FractionDigits)
		if len(out) <= fd {
			out = strings.Repeat("0", fd-len(out)+1) + out
		}
		split := len(out) - fd
		out = out[:split] + "." + out[split:]
	}
	if n.Negative {
		out = "-" + out
	}
	return out
}

// Int returns n as an int64 if it is an integer in range.
func (n Number) Int() (int64, error) {
	if n.IsDecimal() {
		return 0, fmt.Errorf("called Int() on decimal64 value")
	}
	const maxInt64 = uint64(1<<63 - 1)
	const minInt64Abs = uint64(1 << 63)
	switch {
	case n.Negative && n.Value == minInt64Abs:
		return -1 << 63, nil
	case n.Negative && n.Value <= maxInt64:
		return -int64(n.Value), nil
	case !n.Negative && n.Value <= maxInt64:
		return int64(n.Value), nil
	default:
		return 0, fmt.Errorf("signed integer overflow")
	}
}

// Less reports whether n is less than m.
func (n Number) Less(m Number) bool {
	return n.scaledBigInt().Cmp(m.scaledBigInt()) < 0
}

// Equal reports whether n and m have the same numeric value.
func (n Number) Equal(m Number) bool {
	return n.scaledBigInt().Cmp(m.scaledBigInt()) == 0
}

func (n Number) scaledBigInt() *big.Int {
	out := new(big.Int).SetUint64(n.Value)
	scale := 18 - int64(n.FractionDigits)
	if scale > 0 {
		mul := new(big.Int).Exp(big.NewInt(10), big.NewInt(scale), nil)
		out.Mul(out, mul)
	}
	if n.Negative {
		out.Neg(out)
	}
	return out
}

// YRange is one inclusive range of consecutive numbers.
type YRange struct {
	Min Number
	Max Number
}

// Valid reports whether r has a min less than or equal to max.
func (r YRange) Valid() bool {
	return !r.Max.Less(r.Min)
}

// String returns r in YANG range notation.
func (r YRange) String() string {
	if r.Min.Equal(r.Max) {
		return r.Min.String()
	}
	return r.Min.String() + ".." + r.Max.String()
}

// Equal reports whether r and s have equal bounds.
func (r YRange) Equal(s YRange) bool {
	return r.Min.Equal(s.Min) && r.Max.Equal(s.Max)
}

// YangRange is a goyang-style set of non-overlapping ranges.
type YangRange []YRange

// String returns the ranges in YANG notation separated by '|'.
func (r YangRange) String() string {
	parts := make([]string, 0, len(r))
	for _, part := range r {
		parts = append(parts, part.String())
	}
	return strings.Join(parts, "|")
}

func (r YangRange) Len() int      { return len(r) }
func (r YangRange) Swap(i, j int) { r[i], r[j] = r[j], r[i] }
func (r YangRange) Less(i, j int) bool {
	switch {
	case r[i].Min.Less(r[j].Min):
		return true
	case r[j].Min.Less(r[i].Min):
		return false
	default:
		return r[i].Max.Less(r[j].Max)
	}
}

// Sort sorts r by min, then max.
func (r YangRange) Sort() {
	sort.Sort(r)
}

// Equal reports whether r and q contain the same ordered ranges.
func (r YangRange) Equal(q YangRange) bool {
	if len(r) != len(q) {
		return false
	}
	for i := range r {
		if !r[i].Equal(q[i]) {
			return false
		}
	}
	return true
}

// YangType is a goyang-style type projection.
type YangType struct {
	Name             string
	Kind             TypeKind
	Base             *Type       `json:"-"`
	IdentityBase     *Identity   `json:",omitempty"`
	IdentityBases    []*Identity `json:",omitempty"`
	Root             *YangType   `json:"-"`
	Bit              *EnumType   `json:",omitempty"`
	Enum             *EnumType   `json:",omitempty"`
	Units            string      `json:",omitempty"`
	Default          string      `json:",omitempty"`
	HasDefault       bool        `json:",omitempty"`
	FractionDigits   int         `json:",omitempty"`
	Length           YangRange   `json:",omitempty"`
	OptionalInstance bool        `json:",omitempty"`
	Path             string      `json:",omitempty"`
	Pattern          []string    `json:",omitempty"`
	POSIXPattern     []string    `json:",omitempty"`
	Range            YangRange   `json:",omitempty"`
	Type             []*YangType `json:",omitempty"`
}

// Equal reports whether y and t describe the same projected YANG type.
func (y *YangType) Equal(t *YangType) bool {
	switch {
	case y == t:
		return true
	case y == nil || t == nil:
		return false
	case y.Kind != t.Kind,
		y.Units != t.Units,
		y.Default != t.Default,
		y.HasDefault != t.HasDefault,
		y.FractionDigits != t.FractionDigits,
		!enumTypeEqual(y.Bit, t.Bit),
		!enumTypeEqual(y.Enum, t.Enum),
		!identityEqual(y.IdentityBase, t.IdentityBase),
		!identitySlicesEqual(y.IdentityBases, t.IdentityBases),
		!y.Length.Equal(t.Length),
		y.OptionalInstance != t.OptionalInstance,
		y.Path != t.Path,
		!stringSlicesEqual(y.Pattern, t.Pattern),
		!stringSlicesEqual(y.POSIXPattern, t.POSIXPattern),
		!y.Range.Equal(t.Range),
		!yangTypeSlicesEqual(y.Type, t.Type):
		return false
	default:
		return true
	}
}

func enumTypeEqual(a, b *EnumType) bool {
	switch {
	case a == b:
		return true
	case a == nil || b == nil:
		return false
	case a.unique != b.unique:
		return false
	default:
		return stringInt64MapsEqual(a.ToInt, b.ToInt) && int64StringMapsEqual(a.ToString, b.ToString)
	}
}

func stringInt64MapsEqual(a, b map[string]int64) bool {
	if len(a) != len(b) {
		return false
	}
	for key, value := range a {
		if b[key] != value {
			return false
		}
	}
	return true
}

func int64StringMapsEqual(a, b map[int64]string) bool {
	if len(a) != len(b) {
		return false
	}
	for key, value := range a {
		if b[key] != value {
			return false
		}
	}
	return true
}

func identityEqual(a, b *Identity) bool {
	switch {
	case a == b:
		return true
	case a == nil || b == nil:
		return false
	default:
		return a.PrefixedName() == b.PrefixedName()
	}
}

func identitySlicesEqual(a, b []*Identity) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !identityEqual(a[i], b[i]) {
			return false
		}
	}
	return true
}

func stringSlicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func yangTypeSlicesEqual(a, b []*YangType) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !a[i].Equal(b[i]) {
			return false
		}
	}
	return true
}

// EntryKind is the coarse goyang-style kind of an Entry.
type EntryKind int

const (
	LeafEntry EntryKind = iota
	DirectoryEntry
	AnyDataEntry
	AnyXMLEntry
	CaseEntry
	ChoiceEntry
	InputEntry
	NotificationEntry
	OutputEntry
	DeviateEntry
)

// EntryKindToName maps EntryKind values to goyang-compatible names.
var EntryKindToName = map[EntryKind]string{
	LeafEntry:         "Leaf",
	DirectoryEntry:    "Directory",
	AnyDataEntry:      "AnyData",
	AnyXMLEntry:       "AnyXML",
	CaseEntry:         "Case",
	ChoiceEntry:       "Choice",
	InputEntry:        "Input",
	NotificationEntry: "Notification",
	OutputEntry:       "Output",
	DeviateEntry:      "Deviate",
}

// String returns the goyang-compatible EntryKind spelling.
func (k EntryKind) String() string {
	if name := EntryKindToName[k]; name != "" {
		return name
	}
	return fmt.Sprintf("unknown-entry-%d", k)
}

// RPCEntry contains RPC/action input and output entries.
type RPCEntry struct {
	Input  *Entry
	Output *Entry
}

// ListAttr contains list or leaf-list metadata.
type ListAttr struct {
	MinElements   uint64
	MaxElements   uint64
	OrderedBy     *Value
	OrderedByUser bool
}

// NewDefaultListAttr returns a list attribute value with default min/max bounds.
func NewDefaultListAttr() *ListAttr {
	return &ListAttr{
		MinElements: 0,
		MaxElements: math.MaxUint64,
	}
}

// UsesStmt associates a uses statement with the grouping entry it references.
type UsesStmt struct {
	Uses     *Uses
	Grouping *Entry
}

type deviationPresence struct {
	hasMinElements bool
	hasMaxElements bool
}

// deviationType identifies one deviate substatement kind.
type deviationType int64

const (
	// DeviationUnset specifies that the argument was unset, which is invalid.
	DeviationUnset deviationType = iota
	// DeviationNotSupported corresponds to the not-supported deviate argument.
	DeviationNotSupported
	// DeviationAdd corresponds to the add deviate argument.
	DeviationAdd
	// DeviationReplace corresponds to the replace deviate argument.
	DeviationReplace
	// DeviationDelete corresponds to the delete deviate argument.
	DeviationDelete
)

var fromDeviation = map[deviationType]string{
	DeviationNotSupported: "not-supported",
	DeviationAdd:          "add",
	DeviationReplace:      "replace",
	DeviationDelete:       "delete",
	DeviationUnset:        "unknown",
}

var toDeviation = map[string]deviationType{
	"not-supported": DeviationNotSupported,
	"add":           DeviationAdd,
	"replace":       DeviationReplace,
	"delete":        DeviationDelete,
}

func (d deviationType) String() string {
	return fromDeviation[d]
}

// DeviatedEntry stores a wrapped Entry that corresponds to a deviation.
type DeviatedEntry struct {
	Type         deviationType
	DeviatedPath string
	*Entry
}

// Entry is a read-only projection of a Cambium schema node.
type Entry struct {
	Parent      *Entry `json:"-"`
	Node        Node   `json:"-"`
	Name        string
	Description string   `json:",omitempty"`
	Default     []string `json:",omitempty"`
	Units       string   `json:",omitempty"`
	Errors      []error  `json:"-"`
	Kind        EntryKind
	Config      TriState
	Prefix      *Value                     `json:",omitempty"`
	Mandatory   TriState                   `json:",omitempty"`
	Dir         map[string]*Entry          `json:",omitempty"`
	Key         string                     `json:",omitempty"`
	Type        *YangType                  `json:",omitempty"`
	Exts        []*Statement               `json:",omitempty"`
	ListAttr    *ListAttr                  `json:",omitempty"`
	RPC         *RPCEntry                  `json:",omitempty"`
	Identities  []*Identity                `json:",omitempty"`
	Augments    []*Entry                   `json:",omitempty"`
	Augmented   []*Entry                   `json:",omitempty"`
	Deviations  []*DeviatedEntry           `json:"-"`
	Deviate     map[deviationType][]*Entry `json:"-"`
	Uses        []*UsesStmt                `json:",omitempty"`
	Extra       map[string][]interface{}   `json:"extra-unstable,omitempty"`
	Annotation  map[string]interface{}     `json:",omitempty"`

	deviatePresence deviationPresence
	namespace       *Value
	module          cambium.Module
	modules         *Modules
	ordered         []*Entry
	schemaNode      cambium.SchemaNodeRef
}

// ToEntry projects a goyang-style AST node into an Entry tree.
func ToEntry(n Node) *Entry {
	switch value := n.(type) {
	case nil:
		return errorEntry(fmt.Errorf("ToEntry called on nil AST node"))
	case *Module:
		return entryFromCompatModule(value)
	case *ASTModule:
		return entryFromASTModule(value)
	case *Uses:
		return entryFromUsesNode(value)
	case *Grouping:
		return entryFromGroupingNode(value)
	case *Statement:
		return entryFromStatement(value, nil)
	default:
		return entryFromASTNode(value)
	}
}

func entryFromCambiumModule(module cambium.Module) *Entry {
	root := &Entry{
		Name:       module.Name(),
		Kind:       DirectoryEntry,
		Config:     TSTrue,
		Prefix:     valueOrNil(module.Prefix()),
		Dir:        make(map[string]*Entry),
		Node:       entryNodeForModule(module),
		Exts:       statementsForExtensions(module.Extensions()),
		Extra:      make(map[string][]interface{}),
		Annotation: make(map[string]interface{}),
		module:     module,
	}
	if desc, ok := module.Description(); ok {
		root.Description = desc
	}
	seenIdentities := make(map[string]*Identity)
	for identity := range module.Identities() {
		if projected := identityFromCambium(identity, seenIdentities); projected != nil {
			root.Identities = append(root.Identities, projected)
		}
	}
	for child := range module.Children().Iter() {
		root.add(projectNode(child, root))
	}
	return root
}

// FromModule projects a Cambium module handle into an Entry tree.
func FromModule(module cambium.Module) *Entry { return entryFromCambiumModule(module) }

func entryFromCompatModule(module *Module) *Entry {
	if module == nil {
		return errorEntry(fmt.Errorf("ToEntry called on nil module"))
	}
	if entry, ok := moduleEntry(module); ok {
		return entry
	}
	if schema, ok := moduleSchema(module); ok && schema.Name() != "" {
		entry := entryFromCambiumModule(schema)
		setModuleEntry(module, entry)
		return entry
	}
	entry := entryFromASTNode(module)
	if entry != nil {
		entry.Node = module
		entry.modules = module.Modules
	}
	return entry
}

func entryFromCompatModuleSource(module *Module) *Entry {
	if module == nil || module.Source == nil {
		return nil
	}
	entry := entryFromStatementWithScopes(module.Source, nil, nil, nil, module)
	if entry != nil {
		entry.Node = module
		entry.modules = module.Modules
		if module.Modules != nil && module.Modules.ParseOptions.StoreUses {
			attachStoredUsesFromNode(entry, module)
		}
	}
	return entry
}

func entryFromASTModule(module *ASTModule) *Entry {
	if module == nil {
		return errorEntry(fmt.Errorf("ToEntry called on nil AST module"))
	}
	if module.Source != nil {
		entry := entryFromStatementWithScopes(module.Source, nil, nil, nil, module)
		if entry != nil {
			entry.Node = module
			entry.modules = modulesFromASTModuleSet(module)
			if module.Modules != nil && module.Modules.ParseOptions.StoreUses {
				attachStoredUsesFromNode(entry, module)
			}
		}
		return entry
	}
	return errorEntry(fmt.Errorf("ToEntry AST module %q has no ordered source statement", module.Name))
}

func entryFromASTNode(node Node) *Entry {
	if node == nil {
		return errorEntry(fmt.Errorf("ToEntry called on nil AST node"))
	}
	if stmt := node.Statement(); stmt != nil {
		return entryFromStatementWithScopes(stmt, nil, nil, typedefScopesForNode(node), node)
	}
	return errorEntry(fmt.Errorf("ToEntry AST node %q has no ordered source statement", node.NName()))
}

func entryFromUsesNode(uses *Uses) *Entry {
	if uses == nil {
		return errorEntry(fmt.Errorf("ToEntry called on nil uses node"))
	}
	grouping := groupingFromResolver(uses, uses.Name)
	if grouping.stmt == nil {
		return errorEntry(fmt.Errorf("unknown group: %s", uses.Name))
	}
	entry := entryForGrouping(grouping, uses)
	metadata := metadataForUsesStatement(uses.Statement())
	metadata.apply(entry)
	entry.addGroupingChildren(grouping, nil, typedefScopesForNode(uses), map[*Statement]bool{}, uses, statementMetadata{})
	return entry
}

func attachStoredUsesFromNode(entry *Entry, node Node) {
	if entry == nil || node == nil {
		return
	}
	value := reflect.ValueOf(node)
	if value.Kind() != reflect.Pointer || value.IsNil() || value.Elem().Kind() != reflect.Struct {
		return
	}
	value = value.Elem()
	typ := value.Type()
	for i := 0; i < typ.NumField(); i++ {
		fieldType := typ.Field(i)
		yangTag := fieldType.Tag.Get("yang")
		if yangTag == "" {
			continue
		}
		tagParts := strings.Split(yangTag, ",")
		if hasYangTagOption(tagParts[1:], "nomerge") {
			continue
		}
		field := value.Field(i)
		if !field.IsValid() || field.IsNil() {
			continue
		}
		if tagParts[0] == "uses" {
			for _, uses := range usesNodesFromField(field) {
				grouping := entryFromUsesNode(uses)
				entry.Uses = append(entry.Uses, &UsesStmt{Uses: uses, Grouping: grouping.shallowDup()})
			}
			continue
		}
		for _, child := range childNodesFromField(field) {
			if childEntry := childEntryForASTNode(entry, tagParts[0], child); childEntry != nil {
				attachStoredUsesFromNode(childEntry, child)
			}
		}
	}
}

func hasYangTagOption(options []string, want string) bool {
	for _, option := range options {
		if option == want {
			return true
		}
	}
	return false
}

func usesNodesFromField(field reflect.Value) []*Uses {
	switch field.Kind() {
	case reflect.Pointer:
		if uses, ok := field.Interface().(*Uses); ok && uses != nil {
			return []*Uses{uses}
		}
	case reflect.Slice:
		out := make([]*Uses, 0, field.Len())
		for i := 0; i < field.Len(); i++ {
			if uses, ok := field.Index(i).Interface().(*Uses); ok && uses != nil {
				out = append(out, uses)
			}
		}
		return out
	}
	return nil
}

func childNodesFromField(field reflect.Value) []Node {
	switch field.Kind() {
	case reflect.Pointer:
		if child, ok := field.Interface().(Node); ok && child != nil {
			return []Node{child}
		}
	case reflect.Slice:
		out := make([]Node, 0, field.Len())
		for i := 0; i < field.Len(); i++ {
			if child, ok := field.Index(i).Interface().(Node); ok && child != nil {
				out = append(out, child)
			}
		}
		return out
	}
	return nil
}

func childEntryForASTNode(parent *Entry, tag string, child Node) *Entry {
	if parent == nil || child == nil {
		return nil
	}
	switch tag {
	case "input":
		if parent.RPC != nil && parent.RPC.Input != nil {
			return parent.RPC.Input
		}
		return parent.Lookup("input")
	case "output":
		if parent.RPC != nil && parent.RPC.Output != nil {
			return parent.RPC.Output
		}
		return parent.Lookup("output")
	}
	if name := child.NName(); name != "" {
		return parent.Lookup(name)
	}
	return parent.Lookup(tag)
}

func entryFromGroupingNode(grouping *Grouping) *Entry {
	if grouping == nil {
		return errorEntry(fmt.Errorf("ToEntry called on nil grouping node"))
	}
	stmt := grouping.Statement()
	if stmt == nil {
		return errorEntry(fmt.Errorf("ToEntry grouping %q has no ordered source statement", grouping.Name))
	}
	ref := statementGrouping{stmt: stmt, node: grouping}
	entry := entryForGrouping(ref, grouping)
	entry.addGroupingChildren(ref, nil, typedefScopesForNode(grouping), map[*Statement]bool{}, grouping, statementMetadata{})
	return entry
}

func entryForGrouping(grouping statementGrouping, fallback Node) *Entry {
	node := grouping.node
	if node == nil {
		node = fallback
	}
	name := ""
	if grouping.stmt != nil {
		name = grouping.stmt.Argument
	}
	entry := &Entry{
		Name:       name,
		Kind:       DirectoryEntry,
		Config:     TSUnset,
		Dir:        make(map[string]*Entry),
		Node:       node,
		Exts:       extensionStatements(grouping.stmt),
		Extra:      make(map[string][]interface{}),
		Annotation: make(map[string]interface{}),
	}
	if root := RootNode(fallback); root != nil {
		entry.Prefix = valueOrNil(root.GetPrefix())
	}
	return entry
}

type statementGrouping struct {
	stmt *Statement
	node Node
}

type statementGroupingScopes []map[string]statementGrouping

type statementTypedefScopes []map[string]*Statement

type resolvedTypedefStatement struct {
	stmt     *Statement
	resolver Node
	scopes   statementTypedefScopes
}

type statementMetadata struct {
	extra map[string][]interface{}
	exts  []*Statement
}

func errorEntry(err error) *Entry {
	return &Entry{
		Node:       &ErrorNode{Error: err},
		Errors:     []error{err},
		Extra:      make(map[string][]interface{}),
		Annotation: make(map[string]interface{}),
	}
}

func entryFromStatement(stmt *Statement, parent *Entry) *Entry {
	return entryFromStatementWithScopes(stmt, parent, nil, nil, nil)
}

func entryFromStatementWithScopes(stmt *Statement, parent *Entry, scopes statementGroupingScopes, typedefScopes statementTypedefScopes, resolver Node) *Entry {
	if stmt == nil {
		return errorEntry(fmt.Errorf("ToEntry called on nil statement"))
	}
	entry := entryForSchemaStatement(stmt, parent, typedefScopes, resolver)
	if entry == nil {
		return nil
	}
	scopes = append(scopes, groupingScopeForStatement(stmt))
	typedefScopes = append(typedefScopes, typedefScopeForStatement(stmt))
	for _, child := range stmt.SubStatements() {
		if child.Keyword == "uses" {
			grouping := scopes.findGrouping(child.Argument)
			if grouping.stmt == nil {
				grouping = groupingFromResolver(resolver, child.Argument)
			}
			if grouping.stmt == nil {
				entry.Errors = append(entry.Errors, fmt.Errorf("unknown group: %s", child.Argument))
				continue
			}
			entry.addGroupingChildren(grouping, scopes, typedefScopes, map[*Statement]bool{}, resolver, metadataForUsesStatement(child))
			continue
		}
		if child.Keyword == "augment" {
			augmentEntry := entryFromStatementWithScopes(child, entry, scopes, typedefScopes, resolver)
			if augmentEntry != nil {
				entry.Augments = append(entry.Augments, augmentEntry)
			}
			continue
		}
		if child.Keyword == "deviation" {
			deviationEntry := entryFromStatementWithScopes(child, entry, scopes, typedefScopes, resolver)
			if deviationEntry != nil {
				entry.Deviations = append(entry.Deviations, &DeviatedEntry{
					Entry:        deviationEntry,
					DeviatedPath: child.Argument,
				})
			}
			continue
		}
		if child.Keyword == "deviate" {
			deviateEntry := entryFromStatementWithScopes(child, entry, scopes, typedefScopes, resolver)
			if deviateEntry != nil {
				devType := toDeviation[child.Argument]
				if entry.Deviate == nil {
					entry.Deviate = make(map[deviationType][]*Entry)
				}
				entry.Deviate[devType] = append(entry.Deviate[devType], deviateEntry)
			}
			continue
		}
		childEntry := entryFromStatementWithScopes(child, entry, scopes, typedefScopes, resolver)
		if childEntry == nil {
			continue
		}
		if child.Keyword == "rpc" && childEntry.RPC == nil {
			childEntry.RPC = &RPCEntry{}
		}
		entry.add(childEntry)
		switch childEntry.Kind {
		case InputEntry:
			if entry.RPC == nil {
				entry.RPC = &RPCEntry{}
			}
			entry.RPC.Input = childEntry
		case OutputEntry:
			if entry.RPC == nil {
				entry.RPC = &RPCEntry{}
			}
			entry.RPC.Output = childEntry
		}
	}
	return entry
}

func (e *Entry) addGroupingChildren(grouping statementGrouping, scopes statementGroupingScopes, typedefScopes statementTypedefScopes, seen map[*Statement]bool, resolver Node, metadata statementMetadata) {
	if e == nil || grouping.stmt == nil {
		return
	}
	if grouping.node != nil {
		resolver = grouping.node
	}
	if seen[grouping.stmt] {
		e.Errors = append(e.Errors, fmt.Errorf("circular group: %s", grouping.stmt.Argument))
		return
	}
	seen[grouping.stmt] = true
	defer delete(seen, grouping.stmt)

	groupingScopes := append(scopes, groupingScopeForStatement(grouping.stmt))
	groupingTypedefScopes := typedefScopes
	if grouping.node != nil {
		groupingTypedefScopes = typedefScopesForNode(grouping.node)
	}
	groupingTypedefScopes = append(groupingTypedefScopes, typedefScopeForStatement(grouping.stmt))
	for _, child := range grouping.stmt.SubStatements() {
		if child.Keyword == "uses" {
			nested := groupingScopes.findGrouping(child.Argument)
			if nested.stmt == nil {
				nested = groupingFromResolver(resolver, child.Argument)
			}
			if nested.stmt == nil {
				e.Errors = append(e.Errors, fmt.Errorf("unknown group: %s", child.Argument))
				continue
			}
			e.addGroupingChildren(nested, groupingScopes, groupingTypedefScopes, seen, resolver, mergeStatementMetadata(metadataForUsesStatement(child), metadata))
			continue
		}
		childEntry := entryFromStatementWithScopes(child, e, groupingScopes, groupingTypedefScopes, resolver)
		if childEntry != nil {
			metadata.apply(childEntry)
			e.add(childEntry)
		}
	}
}

func metadataForUsesStatement(stmt *Statement) statementMetadata {
	metadata := statementMetadata{extra: make(map[string][]interface{})}
	if stmt == nil {
		return metadata
	}
	for _, child := range stmt.SubStatements() {
		switch child.Keyword {
		case "if-feature", "reference", "status", "when":
			metadata.extra[child.Keyword] = append(metadata.extra[child.Keyword], child)
		default:
			if strings.Contains(child.Keyword, ":") {
				metadata.exts = append(metadata.exts, child)
			}
		}
	}
	return metadata
}

func mergeStatementMetadata(first, second statementMetadata) statementMetadata {
	merged := statementMetadata{extra: make(map[string][]interface{})}
	for _, metadata := range []statementMetadata{first, second} {
		merged.exts = append(merged.exts, metadata.exts...)
		for key, values := range metadata.extra {
			merged.extra[key] = append(merged.extra[key], values...)
		}
	}
	return merged
}

func (m statementMetadata) apply(entry *Entry) {
	if entry == nil {
		return
	}
	entry.Exts = append(entry.Exts, m.exts...)
	if entry.Extra == nil {
		entry.Extra = make(map[string][]interface{})
	}
	for key, values := range m.extra {
		entry.Extra[key] = append(entry.Extra[key], values...)
	}
}

func groupingScopeForStatement(stmt *Statement) map[string]statementGrouping {
	scope := map[string]statementGrouping{}
	if stmt == nil {
		return scope
	}
	for _, child := range stmt.SubStatements() {
		if child.Keyword == "grouping" && child.Argument != "" {
			scope[child.Argument] = statementGrouping{stmt: child}
		}
	}
	return scope
}

func typedefScopeForStatement(stmt *Statement) map[string]*Statement {
	scope := map[string]*Statement{}
	if stmt == nil {
		return scope
	}
	for _, child := range stmt.SubStatements() {
		if child.Keyword == "typedef" && child.Argument != "" {
			scope[child.Argument] = child
		}
	}
	return scope
}

func (scopes statementGroupingScopes) findGrouping(name string) statementGrouping {
	_, local := prefixedPathPart(name)
	for i := len(scopes) - 1; i >= 0; i-- {
		if grouping := scopes[i][name]; grouping.stmt != nil {
			return grouping
		}
		if local != name {
			if grouping := scopes[i][local]; grouping.stmt != nil {
				return grouping
			}
		}
	}
	return statementGrouping{}
}

func (scopes statementTypedefScopes) findTypedef(name string) *Statement {
	_, local := prefixedPathPart(name)
	for i := len(scopes) - 1; i >= 0; i-- {
		if typedef := scopes[i][name]; typedef != nil {
			return typedef
		}
		if local != name {
			if typedef := scopes[i][local]; typedef != nil {
				return typedef
			}
		}
	}
	return nil
}

func findTypedefStatement(name string, scopes statementTypedefScopes, resolver Node) resolvedTypedefStatement {
	prefix, local := prefixedPathPart(name)
	if prefix == "" || prefix == typedefResolverPrefix(resolver) {
		if typedef := scopes.findTypedef(name); typedef != nil {
			return resolvedTypedefStatement{stmt: typedef, resolver: resolver, scopes: scopes}
		}
		if included := includedTypedefStatement(resolver, local); included.stmt != nil {
			return included
		}
		return resolvedTypedefStatement{}
	}
	if prefix == "" {
		return resolvedTypedefStatement{}
	}
	return importedTypedefStatement(resolver, prefix, local)
}

func importedTypedefStatement(resolver Node, prefix, name string) resolvedTypedefStatement {
	root := rootNode(resolver)
	for _, imp := range importNodes(root) {
		if imp == nil || imp.Prefix == nil || imp.Prefix.Name != prefix {
			continue
		}
		module := moduleForImport(resolver, imp)
		if module == nil {
			return resolvedTypedefStatement{}
		}
		scopes := typedefScopesForNode(module)
		if typedef := scopes.findTypedef(name); typedef != nil {
			return resolvedTypedefStatement{stmt: typedef, resolver: module, scopes: scopes}
		}
		return resolvedTypedefStatement{}
	}
	return resolvedTypedefStatement{}
}

func includedTypedefStatement(resolver Node, name string) resolvedTypedefStatement {
	root := rootNode(resolver)
	for _, inc := range includeNodes(root) {
		module := moduleForInclude(resolver, inc)
		if module == nil {
			continue
		}
		scopes := typedefScopesForNode(module)
		if typedef := scopes.findTypedef(name); typedef != nil {
			return resolvedTypedefStatement{stmt: typedef, resolver: module, scopes: scopes}
		}
	}
	return resolvedTypedefStatement{}
}

func typedefResolverPrefix(resolver Node) string {
	root := rootNode(resolver)
	if root == nil {
		return ""
	}
	if prefixer, ok := root.(interface{ GetPrefix() string }); ok {
		return prefixer.GetPrefix()
	}
	return ""
}

func typedefScopesForNode(node Node) statementTypedefScopes {
	if node == nil {
		return nil
	}
	var stack []*Statement
	for cur := node; cur != nil; cur = cur.ParentNode() {
		if stmt := cur.Statement(); stmt != nil {
			stack = append(stack, stmt)
		}
	}
	var scopes statementTypedefScopes
	if root := RootNode(node); root != nil {
		if stmt := root.Statement(); stmt != nil {
			scopes = append(scopes, typedefScopeForStatement(stmt))
		}
	}
	for i := len(stack) - 1; i >= 0; i-- {
		scopes = append(scopes, typedefScopeForStatement(stack[i]))
	}
	return scopes
}

func groupingFromResolver(resolver Node, name string) statementGrouping {
	if resolver == nil {
		return statementGrouping{}
	}
	grouping := FindGrouping(resolver, name, map[string]bool{})
	if grouping == nil || grouping.Statement() == nil {
		return statementGrouping{}
	}
	return statementGrouping{stmt: grouping.Statement(), node: grouping}
}

func entryForSchemaStatement(stmt *Statement, parent *Entry, typedefScopes statementTypedefScopes, resolver Node) *Entry {
	entry := &Entry{
		Name:       stmt.Argument,
		Parent:     parent,
		Node:       entryNodeForStatement(stmt, parent),
		Exts:       extensionStatements(stmt),
		Extra:      make(map[string][]interface{}),
		Annotation: make(map[string]interface{}),
	}
	switch stmt.Keyword {
	case "module", "submodule", "container":
		entry.Kind = DirectoryEntry
		entry.Dir = make(map[string]*Entry)
	case "augment", "deviation":
		entry.Kind = DirectoryEntry
		entry.Dir = make(map[string]*Entry)
	case "list":
		entry.Kind = DirectoryEntry
		entry.Dir = make(map[string]*Entry)
		entry.ListAttr = listAttrForStatement(stmt, entry)
		entry.Key = childArgument(stmt, "key")
	case "leaf":
		entry.Kind = LeafEntry
		entry.Type = yangTypeForTypeStatement(firstChild(stmt, "type"), typedefScopes, resolver, nil)
		entry.Default = statementArguments(stmt, "default")
	case "leaf-list":
		entry.Kind = LeafEntry
		entry.Type = yangTypeForTypeStatement(firstChild(stmt, "type"), typedefScopes, resolver, nil)
		entry.Default = statementArguments(stmt, "default")
		entry.ListAttr = listAttrForStatement(stmt, entry)
	case "choice":
		entry.Kind = ChoiceEntry
		entry.Dir = make(map[string]*Entry)
		entry.Default = statementArguments(stmt, "default")
	case "case":
		entry.Kind = CaseEntry
		entry.Dir = make(map[string]*Entry)
	case "anydata":
		entry.Kind = AnyDataEntry
		entry.Dir = make(map[string]*Entry)
	case "anyxml":
		entry.Kind = AnyXMLEntry
		entry.Dir = make(map[string]*Entry)
	case "rpc", "action":
		entry.Kind = DirectoryEntry
		entry.Dir = make(map[string]*Entry)
	case "input":
		entry.Kind = InputEntry
		entry.Dir = make(map[string]*Entry)
	case "output":
		entry.Kind = OutputEntry
		entry.Dir = make(map[string]*Entry)
	case "notification":
		entry.Kind = NotificationEntry
		entry.Dir = make(map[string]*Entry)
	case "deviate":
		entry.Kind = DeviateEntry
		entry.Dir = make(map[string]*Entry)
		entry.Type = yangTypeForTypeStatement(firstChild(stmt, "type"), typedefScopes, resolver, nil)
		entry.Default = statementArguments(stmt, "default")
		minElements := firstChild(stmt, "min-elements")
		maxElements := firstChild(stmt, "max-elements")
		entry.deviatePresence.hasMinElements = minElements != nil
		entry.deviatePresence.hasMaxElements = maxElements != nil
		if minElements != nil || maxElements != nil {
			entry.ListAttr = listAttrForStatement(stmt, entry)
		}
	default:
		return nil
	}
	if desc := childArgument(stmt, "description"); desc != "" {
		entry.Description = desc
	}
	if units := childArgument(stmt, "units"); units != "" {
		entry.Units = units
	}
	addStatementExtras(entry, stmt)
	entry.Config = triStateFromStatement(firstChild(stmt, "config"), entry)
	entry.Mandatory = triStateFromStatement(firstChild(stmt, "mandatory"), entry)
	if stmt.Keyword == "module" || stmt.Keyword == "submodule" {
		entry.Prefix = valueOrNil(childArgument(stmt, "prefix"))
	}
	return entry
}

func addStatementExtras(entry *Entry, stmt *Statement) {
	if entry == nil || stmt == nil {
		return
	}
	for _, child := range stmt.SubStatements() {
		switch child.Keyword {
		case "belongs-to",
			"contact",
			"extension",
			"feature",
			"if-feature",
			"must",
			"namespace",
			"ordered-by",
			"organization",
			"presence",
			"reference",
			"revision",
			"status",
			"unique",
			"when",
			"yang-version":
			entry.Extra[child.Keyword] = append(entry.Extra[child.Keyword], child)
		}
	}
}

func entryNodeForModule(module cambium.Module) Node {
	if module.Name() == "" {
		return nil
	}
	return &entryNode{
		kind:       "module",
		name:       module.Name(),
		extensions: statementsForExtensions(module.Extensions()),
	}
}

func entryNodeForStatement(stmt *Statement, parent *Entry) Node {
	if stmt == nil {
		return nil
	}
	var parentNode Node
	if parent != nil {
		parentNode = parent.Node
	}
	return &entryNode{
		kind:       stmt.Keyword,
		name:       stmt.Argument,
		parent:     parentNode,
		source:     stmt,
		extensions: extensionStatements(stmt),
	}
}

func entryNodeForSchemaNode(node cambium.SchemaNodeRef, parent *Entry) Node {
	if node.Name() == "" {
		return nil
	}
	var parentNode Node
	if parent != nil {
		parentNode = parent.Node
	}
	return &entryNode{
		kind:       schemaNodeKindKeyword(node.Kind()),
		name:       node.Name(),
		parent:     parentNode,
		extensions: statementsForExtensions(node.Extensions()),
	}
}

func schemaNodeKindKeyword(kind cambium.SchemaNodeKind) string {
	switch kind {
	case cambium.SchemaNodeKindModule:
		return "module"
	case cambium.SchemaNodeKindContainer:
		return "container"
	case cambium.SchemaNodeKindList:
		return "list"
	case cambium.SchemaNodeKindLeaf:
		return "leaf"
	case cambium.SchemaNodeKindLeafList:
		return "leaf-list"
	case cambium.SchemaNodeKindChoice:
		return "choice"
	case cambium.SchemaNodeKindCase:
		return "case"
	case cambium.SchemaNodeKindAnyData:
		return "anydata"
	case cambium.SchemaNodeKindRPC:
		return "rpc"
	case cambium.SchemaNodeKindAction:
		return "action"
	case cambium.SchemaNodeKindInput:
		return "input"
	case cambium.SchemaNodeKindOutput:
		return "output"
	case cambium.SchemaNodeKindNotification:
		return "notification"
	default:
		return ""
	}
}

func listAttrForStatement(stmt *Statement, entry *Entry) *ListAttr {
	attr := NewDefaultListAttr()
	if min := firstChild(stmt, "min-elements"); min != nil {
		if value, err := strconv.ParseUint(min.Argument, 10, 64); err == nil {
			attr.MinElements = value
		} else if entry != nil {
			entry.Errors = append(entry.Errors, fmt.Errorf(`%s: invalid min-elements value %q (expect a non-negative integer): %v`, min.Location(), min.Argument, err))
		}
	}
	if max := firstChild(stmt, "max-elements"); max != nil {
		if max.Argument == "unbounded" {
			attr.MaxElements = math.MaxUint64
		} else if value, err := strconv.ParseUint(max.Argument, 10, 64); err == nil {
			if value == 0 {
				if entry != nil {
					entry.Errors = append(entry.Errors, fmt.Errorf(`%s: invalid max-elements value 0 (expect "unbounded" or a positive integer)`, max.Location()))
				}
			} else {
				attr.MaxElements = value
			}
		} else if entry != nil {
			entry.Errors = append(entry.Errors, fmt.Errorf(`%s: invalid max-elements value %q (expect "unbounded" or a positive integer): %v`, max.Location(), max.Argument, err))
		}
	}
	if orderedBy := firstChild(stmt, "ordered-by"); orderedBy != nil {
		attr.OrderedBy = &Value{Name: orderedBy.Argument, Source: orderedBy}
		attr.OrderedByUser = orderedBy.Argument == "user"
	}
	return attr
}

func yangTypeForTypeStatement(stmt *Statement, typedefScopes statementTypedefScopes, resolver Node, seen map[string]bool) *YangType {
	if stmt == nil {
		return nil
	}
	kind := Ynone
	if parsed, ok := TypeKindFromName[stmt.Argument]; ok {
		kind = parsed
	}
	typ := &YangType{
		Name: stmt.Argument,
		Kind: kind,
	}
	if kind != Ynone {
		typ.Root = &YangType{Name: stmt.Argument, Kind: kind}
		typ.Root.Root = typ.Root
		typ.Range = defaultRangeForTypeKind(kind)
		typ.Root.Range = cloneYangRange(typ.Range)
		typ.Base = &Type{
			Name:   stmt.Argument,
			Source: typeStatement(stmt.Argument),
			YangType: &upstream.YangType{
				Name: stmt.Argument,
				Kind: upstream.TypeKind(kind),
			},
		}
	}
	switch kind {
	case Ydecimal64:
		if fd := firstChild(stmt, "fraction-digits"); fd != nil {
			if value, err := strconv.Atoi(fd.Argument); err == nil {
				typ.FractionDigits = value
			}
		}
	case Yleafref:
		if path := firstChild(stmt, "path"); path != nil {
			typ.Path = path.Argument
		}
		typ.OptionalInstance = true
		if require := firstChild(stmt, "require-instance"); require != nil {
			typ.OptionalInstance = require.Argument == "false"
		}
	case Yunion:
		for _, child := range stmt.SubStatements() {
			if child.Keyword == "type" {
				typ.Type = append(typ.Type, yangTypeForTypeStatement(child, typedefScopes, resolver, seen))
			}
		}
	}
	applyRawTypeStatementMetadata(typ, stmt, resolver)
	if kind != Ynone {
		return typ
	}
	typedef := findTypedefStatement(stmt.Argument, typedefScopes, resolver)
	if typedef.stmt == nil {
		return typ
	}
	if seen == nil {
		seen = make(map[string]bool)
	}
	seenKey := typedefSeenKey(typedef.stmt, typedef.resolver)
	if seen[seenKey] {
		return typ
	}
	seen[seenKey] = true
	defer delete(seen, seenKey)

	baseStmt := firstChild(typedef.stmt, "type")
	base := yangTypeForTypeStatement(baseStmt, typedef.scopes, typedef.resolver, seen)
	if base == nil {
		return typ
	}
	resolved := *base
	resolved.Name = typedef.stmt.Argument
	if baseStmt != nil {
		resolved.Base = &Type{
			Name:     baseStmt.Argument,
			Source:   typeStatement(baseStmt.Argument),
			YangType: upstreamYangTypeFromCompat(base),
		}
	}
	if units := childArgument(typedef.stmt, "units"); units != "" {
		resolved.Units = units
	}
	if def := childArgument(typedef.stmt, "default"); def != "" {
		resolved.HasDefault = true
		resolved.Default = def
	}
	applyRawTypeStatementMetadata(&resolved, stmt, resolver)
	if resolved.Root == base || resolved.Root == nil || !resolved.Equal(resolved.Root) {
		resolved.Root = &resolved
	}
	return &resolved
}

func applyRawTypeStatementMetadata(typ *YangType, stmt *Statement, resolver Node) {
	if typ == nil || stmt == nil {
		return
	}
	switch typ.Kind {
	case Yint8, Yint16, Yint32, Yint64, Yuint8, Yuint16, Yuint32, Yuint64:
		if rng := firstChild(stmt, "range"); rng != nil {
			if parsed, err := parseStatementRange(typ.Range, rng.Argument, false, 0); err == nil {
				typ.Range = parsed
			}
		}
	case Ydecimal64:
		if typ.FractionDigits == 0 {
			if fd := firstChild(stmt, "fraction-digits"); fd != nil {
				if value, err := strconv.Atoi(fd.Argument); err == nil {
					typ.FractionDigits = value
				}
			}
		}
		if typ.FractionDigits != 0 && typ.Range == nil {
			typ.Range = decimalDefaultRange(typ.FractionDigits)
		}
		if rng := firstChild(stmt, "range"); rng != nil {
			if parsed, err := parseStatementRange(typ.Range, rng.Argument, true, uint8(typ.FractionDigits)); err == nil {
				typ.Range = parsed
			}
		}
	case Ystring:
		if length := firstChild(stmt, "length"); length != nil {
			parent := typ.Length
			if parent == nil {
				parent = Uint64Range
			}
			if parsed, err := parseStatementRange(parent, length.Argument, false, 0); err == nil {
				typ.Length = parsed
			}
		}
		typ.Pattern = appendUniqueStrings(typ.Pattern, statementArguments(stmt, "pattern")...)
		typ.POSIXPattern = appendUniqueStrings(typ.POSIXPattern, posixPatternArguments(stmt)...)
	case Ybinary:
		if length := firstChild(stmt, "length"); length != nil {
			parent := typ.Length
			if parent == nil {
				parent = Uint64Range
			}
			if parsed, err := parseStatementRange(parent, length.Argument, false, 0); err == nil {
				typ.Length = parsed
			}
		}
	case Yenum:
		if enum := enumTypeForStatement(stmt, false); enum != nil {
			typ.Enum = enum
		}
	case Ybits:
		if bit := enumTypeForStatement(stmt, true); bit != nil {
			typ.Bit = bit
		}
	case Yidentityref:
		if identity := identityBaseForTypeStatement(stmt, resolver); identity != nil {
			typ.IdentityBase = identity
			typ.IdentityBases = []*Identity{identity}
		}
	}
}

func parseStatementRange(parent YangRange, expr string, decimal bool, fractionDigits uint8) (YangRange, error) {
	parseNumber := func(raw string) (Number, error) {
		switch raw {
		case "max":
			if len(parent) == 0 {
				return Number{}, fmt.Errorf("cannot resolve max without parent range")
			}
			max := parent[len(parent)-1].Max
			max.FractionDigits = fractionDigits
			return max, nil
		case "min":
			if len(parent) == 0 {
				return Number{}, fmt.Errorf("cannot resolve min without parent range")
			}
			min := parent[0].Min
			min.FractionDigits = fractionDigits
			return min, nil
		default:
			if decimal {
				return ParseDecimal(raw, fractionDigits)
			}
			return ParseInt(raw)
		}
	}

	parts := strings.Split(expr, "|")
	out := make(YangRange, 0, len(parts))
	for _, part := range parts {
		bounds := strings.Split(part, "..")
		if len(bounds) > 2 {
			return nil, fmt.Errorf("too many range separators in %q", part)
		}
		min, err := parseNumber(strings.TrimSpace(bounds[0]))
		if err != nil {
			return nil, err
		}
		max := min
		if len(bounds) == 2 {
			max, err = parseNumber(strings.TrimSpace(bounds[1]))
			if err != nil {
				return nil, err
			}
		}
		if max.Less(min) {
			return nil, fmt.Errorf("range boundaries out of order")
		}
		out = append(out, YRange{Min: min, Max: max})
	}
	out.Sort()
	out = coalesceStatementRange(out)
	if !parent.Contains(out) {
		return nil, fmt.Errorf("%s not within %s", expr, parent)
	}
	if err := out.Validate(); err != nil {
		return nil, err
	}
	return out, nil
}

func coalesceStatementRange(in YangRange) YangRange {
	if len(in) < 2 {
		return in
	}
	out := make(YangRange, 0, len(in))
	out = append(out, in[0])
	for _, next := range in[1:] {
		last := &out[len(out)-1]
		adjacent := last.Max.addQuantum(1).Equal(next.Min)
		switch {
		case !last.Max.Less(next.Min):
			if last.Max.Less(next.Max) {
				last.Max = next.Max
			}
		case adjacent:
			last.Max = next.Max
		default:
			out = append(out, next)
		}
	}
	return out
}

func decimalDefaultRange(fractionDigits int) YangRange {
	if fractionDigits < 1 || fractionDigits > int(MaxFractionDigits) {
		return nil
	}
	fd := uint8(fractionDigits)
	return YangRange{{
		Min: Number{Value: uint64(AbsMinInt64), FractionDigits: fd, Negative: true},
		Max: Number{Value: uint64(MaxInt64), FractionDigits: fd},
	}}
}

func appendUniqueStrings(out []string, values ...string) []string {
	seen := make(map[string]bool, len(out)+len(values))
	for _, value := range out {
		seen[value] = true
	}
	for _, value := range values {
		if seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}

func posixPatternArguments(stmt *Statement) []string {
	if stmt == nil {
		return nil
	}
	var out []string
	for _, child := range stmt.SubStatements() {
		if strings.HasSuffix(child.Keyword, ":posix-pattern") {
			out = append(out, child.Argument)
		}
	}
	return out
}

func enumTypeForStatement(stmt *Statement, bitfield bool) *EnumType {
	var out *EnumType
	keyword := "enum"
	valueKeyword := "value"
	if bitfield {
		out = NewBitfield()
		keyword = "bit"
		valueKeyword = "position"
	} else {
		out = NewEnumType()
	}
	found := false
	for _, child := range stmt.SubStatements() {
		if child.Keyword != keyword {
			continue
		}
		found = true
		value := firstChild(child, valueKeyword)
		if value == nil {
			_ = out.SetNext(child.Argument)
			continue
		}
		parsed, err := ParseInt(value.Argument)
		if err != nil {
			continue
		}
		intValue, err := parsed.Int()
		if err != nil {
			continue
		}
		_ = out.Set(child.Argument, intValue)
	}
	if !found {
		return nil
	}
	return out
}

func identityBaseForTypeStatement(stmt *Statement, resolver Node) *Identity {
	base := firstChild(stmt, "base")
	if base == nil {
		return nil
	}
	return identityForName(base.Argument, resolver)
}

func identityForName(name string, resolver Node) *Identity {
	prefix, local := prefixedPathPart(name)
	if prefix == "" || prefix == typedefResolverPrefix(resolver) {
		if identity := identityInModule(rootNode(resolver), local); identity != nil {
			return identity
		}
		for _, inc := range includeNodes(rootNode(resolver)) {
			if identity := identityInModule(moduleForInclude(resolver, inc), local); identity != nil {
				return identity
			}
		}
		return nil
	}
	module := importedModuleForPrefix(resolver, prefix)
	return identityInModule(module, local)
}

func importedModuleForPrefix(resolver Node, prefix string) Node {
	root := rootNode(resolver)
	for _, imp := range importNodes(root) {
		if imp == nil || imp.Prefix == nil || imp.Prefix.Name != prefix {
			continue
		}
		return moduleForImport(resolver, imp)
	}
	return nil
}

func identityInModule(module Node, name string) *Identity {
	switch mod := module.(type) {
	case *ASTModule:
		for _, identity := range mod.Identity {
			if identity != nil && identity.Name == name {
				return identitiesFromASTIdentities([]*ASTIdentity{identity}, module)[0]
			}
		}
	case *Module:
		for _, identity := range mod.Identity {
			if identity != nil && identity.Name == name {
				return cloneIdentities([]*Identity{identity}, module)[0]
			}
		}
	}
	return identityFromModuleSource(module, name, map[string]bool{})
}

func identityFromModuleSource(module Node, name string, seen map[string]bool) *Identity {
	if module == nil || module.Statement() == nil || name == "" {
		return nil
	}
	key := module.Kind() + ":" + module.NName() + ":" + name
	if seen[key] {
		return nil
	}
	seen[key] = true
	defer delete(seen, key)

	var identityStmt *Statement
	for _, child := range module.Statement().SubStatements() {
		if child.Keyword == "identity" && child.Argument == name {
			identityStmt = child
			break
		}
	}
	if identityStmt == nil {
		return nil
	}
	identity := astIdentityFromStatementWithParent(identityStmt, module)
	if identity == nil {
		return nil
	}
	for _, child := range module.Statement().SubStatements() {
		if child.Keyword != "identity" || child.Argument == name {
			continue
		}
		if !identityStatementDerivesFrom(module, child, name) {
			continue
		}
		derived := identityFromModuleSource(module, child.Argument, seen)
		if derived == nil {
			continue
		}
		derived.Parent = identity
		identity.Values = append(identity.Values, derived)
	}
	return identity
}

func identityStatementDerivesFrom(module Node, stmt *Statement, name string) bool {
	if stmt == nil {
		return false
	}
	modulePrefix := typedefResolverPrefix(module)
	for _, child := range stmt.SubStatements() {
		if child.Keyword != "base" {
			continue
		}
		prefix, local := prefixedPathPart(child.Argument)
		if local != name {
			continue
		}
		if prefix == "" || prefix == modulePrefix || prefix == module.NName() {
			return true
		}
	}
	return false
}

func typedefSeenKey(stmt *Statement, resolver Node) string {
	if stmt == nil {
		return ""
	}
	if loc := stmt.Location(); loc != "" {
		return loc + ":" + stmt.Keyword + ":" + stmt.Argument
	}
	if resolver != nil {
		return resolver.Kind() + ":" + resolver.NName() + ":" + stmt.Keyword + ":" + stmt.Argument
	}
	return stmt.Keyword + ":" + stmt.Argument
}

func upstreamYangTypeFromCompat(typ *YangType) *upstream.YangType {
	if typ == nil {
		return nil
	}
	out := &upstream.YangType{
		Name:           typ.Name,
		Kind:           upstream.TypeKind(typ.Kind),
		Default:        typ.Default,
		HasDefault:     typ.HasDefault,
		FractionDigits: typ.FractionDigits,
		Units:          typ.Units,
	}
	if typ.Base != nil {
		out.Base = &upstream.Type{
			Name:     typ.Base.Name,
			Source:   typ.Base.Source,
			YangType: typ.Base.YangType,
		}
	}
	return out
}

func defaultRangeForTypeKind(kind TypeKind) YangRange {
	switch kind {
	case Yint8:
		return cloneYangRange(Int8Range)
	case Yint16:
		return cloneYangRange(Int16Range)
	case Yint32:
		return cloneYangRange(Int32Range)
	case Yint64:
		return cloneYangRange(Int64Range)
	case Yuint8:
		return cloneYangRange(Uint8Range)
	case Yuint16:
		return cloneYangRange(Uint16Range)
	case Yuint32:
		return cloneYangRange(Uint32Range)
	case Yuint64:
		return cloneYangRange(Uint64Range)
	default:
		return nil
	}
}

func statementArguments(stmt *Statement, keyword string) []string {
	if stmt == nil {
		return nil
	}
	var out []string
	for _, child := range stmt.SubStatements() {
		if child.Keyword == keyword {
			out = append(out, child.Argument)
		}
	}
	return out
}

func triStateFromStatement(stmt *Statement, entry *Entry) TriState {
	if stmt == nil {
		return TSUnset
	}
	switch stmt.Argument {
	case "true":
		return TSTrue
	case "false":
		return TSFalse
	default:
		if entry != nil {
			entry.Errors = append(entry.Errors, fmt.Errorf("%s: invalid config value: %s", stmt.Location(), stmt.Argument))
		}
		return TSUnset
	}
}

// Modules returns the Modules facade that projected this Entry, if any.
func (e *Entry) Modules() *Modules {
	for e != nil && e.Parent != nil {
		e = e.Parent
	}
	if e == nil {
		return nil
	}
	return e.modules
}

// Children returns this entry's children in effective schema declaration order.
func (e *Entry) Children() []*Entry {
	if e == nil || len(e.ordered) == 0 {
		return nil
	}
	return append([]*Entry(nil), e.ordered...)
}

// Lookup returns the direct child named name, or nil if no such child exists.
func (e *Entry) Lookup(name string) *Entry {
	if e == nil || e.Dir == nil {
		return nil
	}
	return e.Dir[name]
}

// GetErrors returns errors found on e and descendants in deterministic order.
func (e *Entry) GetErrors() []error {
	if e == nil {
		return nil
	}
	var out []error
	e.collectErrors(&out)
	return sortEntryErrors(out)
}

func (e *Entry) collectErrors(out *[]error) {
	if e == nil {
		return
	}
	*out = append(*out, e.Errors...)
	for _, pair := range orderedEntryChildPairs(e) {
		if pair.entry != nil {
			pair.entry.collectErrors(out)
		}
	}
}

type sortableEntryError struct {
	text string
	err  error
}

type sortableEntryErrors []sortableEntryError

func (errs sortableEntryErrors) Len() int      { return len(errs) }
func (errs sortableEntryErrors) Swap(i, j int) { errs[i], errs[j] = errs[j], errs[i] }
func (errs sortableEntryErrors) Less(i, j int) bool {
	const parts = 4
	left := strings.SplitN(errs[i].text, ":", parts)
	right := strings.SplitN(errs[j].text, ":", parts)
	if left[0] < right[0] {
		return true
	}
	if left[0] > right[0] {
		return false
	}
	for idx := 1; idx < parts; idx++ {
		switch {
		case len(right) == idx:
			return false
		case len(left) == idx:
			return true
		}
		switch compareErrorPart(left[idx], right[idx]) {
		case -1:
			return true
		case 1:
			return false
		}
	}
	return false
}

func sortEntryErrors(errs []error) []error {
	switch len(errs) {
	case 0:
		return nil
	case 1:
		return errs
	}
	sortable := make(sortableEntryErrors, len(errs))
	for i, err := range errs {
		sortable[i] = sortableEntryError{text: err.Error(), err: err}
	}
	sort.Sort(sortable)
	out := make([]error, len(errs))
	count := 0
	for _, err := range sortable {
		if count > 0 && reflect.DeepEqual(err.err, out[count-1]) {
			continue
		}
		out[count] = err.err
		count++
	}
	return out[:count]
}

func compareErrorPart(left, right string) int {
	leftInt, leftErr := strconv.Atoi(left)
	rightInt, rightErr := strconv.Atoi(right)
	switch {
	case leftErr == nil && rightErr == nil:
		switch {
		case leftInt < rightInt:
			return -1
		case leftInt > rightInt:
			return 1
		default:
			return 0
		}
	case left < right:
		return -1
	case left > right:
		return 1
	default:
		return 0
	}
}

// GetWhenXPath returns the first effective when XPath expression, if present.
func (e *Entry) GetWhenXPath() (string, bool) {
	if e == nil {
		return "", false
	}
	whens := e.schemaNode.Whens()
	if len(whens) == 0 {
		return e.rawWhenXPath()
	}
	return whens[0].Expression(), true
}

func (e *Entry) rawWhenXPath() (string, bool) {
	if e == nil {
		return "", false
	}
	for _, raw := range e.Extra["when"] {
		switch value := raw.(type) {
		case *Statement:
			if value != nil {
				return value.Argument, true
			}
		case *Value:
			if value != nil {
				return value.Name, true
			}
		case interface{ Statement() *Statement }:
			if stmt := value.Statement(); stmt != nil {
				return stmt.Argument, true
			}
		}
	}
	return "", false
}

// Find returns the entry at path relative to this entry.
//
// It accepts goyang-style slash paths, "." and "..", absolute paths beginning
// with "/", and module/prefix-qualified path parts. Absolute paths start at the
// compat projection root; if their first part names that root module, it is
// skipped.
func (e *Entry) Find(path string) *Entry {
	if e == nil || path == "" {
		return nil
	}
	cur := e
	absolute := strings.HasPrefix(path, "/")
	if absolute {
		for cur.Parent != nil {
			cur = cur.Parent
		}
		path = strings.TrimPrefix(path, "/")
		if path == "" {
			return cur
		}
	}
	parts := strings.Split(path, "/")
	if absolute && len(parts) != 0 {
		if prefix, _ := prefixedPathPart(parts[0]); prefix != "" {
			if !cur.matchesModulePrefix(prefix) {
				resolved := cur.importedPrefixRoot(prefix)
				if resolved == nil {
					cur.Errors = append(cur.Errors, fmt.Errorf("cannot find module giving prefix %q within context entry %q", prefix, cur.Path()))
					return nil
				}
				cur = resolved
			}
		}
	}
	for i, raw := range parts {
		if raw == "" {
			return nil
		}
		part := localPart(raw)
		switch part {
		case ".":
			continue
		case "..":
			if cur.Parent == nil {
				return nil
			}
			cur = cur.Parent
			continue
		}
		if i == 0 && cur.Parent == nil && part == cur.Name {
			continue
		}
		next := cur.Lookup(part)
		if next == nil && cur.RPC != nil {
			switch part {
			case "input":
				if cur.RPC.Input == nil {
					cur.RPC.Input = syntheticOperationIOEntry(cur, "input", InputEntry)
				}
				next = cur.RPC.Input
			case "output":
				if cur.RPC.Output == nil {
					cur.RPC.Output = syntheticOperationIOEntry(cur, "output", OutputEntry)
				}
				next = cur.RPC.Output
			}
		}
		if next == nil {
			return nil
		}
		cur = next
	}
	return cur
}

func syntheticOperationIOEntry(parent *Entry, name string, kind EntryKind) *Entry {
	entry := &Entry{
		Name:       name,
		Kind:       kind,
		Dir:        make(map[string]*Entry),
		Parent:     parent,
		Extra:      make(map[string][]interface{}),
		Annotation: make(map[string]interface{}),
	}
	if parent != nil {
		entry.Node = &entryNode{
			kind:   name,
			name:   name,
			parent: parent.Node,
		}
		entry.module = parent.module
		entry.modules = parent.modules
	}
	return entry
}

func prefixedPathPart(part string) (string, string) {
	prefix, local, ok := strings.Cut(part, ":")
	if !ok {
		return "", part
	}
	return prefix, local
}

func (e *Entry) matchesModulePrefix(prefix string) bool {
	if e == nil || prefix == "" {
		return false
	}
	if e.Name == prefix {
		return true
	}
	if e.Prefix != nil && e.Prefix.Name == prefix {
		return true
	}
	if e.module.Name() == prefix || e.module.Prefix() == prefix {
		return true
	}
	if mod, ok := e.Node.(*Module); ok {
		return mod.Name == prefix || mod.GetPrefix() == prefix
	}
	return false
}

func (e *Entry) importedPrefixRoot(prefix string) *Entry {
	if e == nil || prefix == "" {
		return nil
	}
	if e.modules != nil && e.module.Name() != "" {
		for _, imp := range e.module.Imports() {
			if imp.Prefix != prefix && imp.Name != prefix {
				continue
			}
			if record := e.modules.moduleRecordForImport(imp.Name, imp.Revision); record != nil {
				return entryForModuleRecord(record, e.modules)
			}
		}
	}
	if mod, ok := e.Node.(*Module); ok && e.modules != nil {
		for _, imp := range mod.Import {
			if imp == nil || (imp.Prefix != nil && imp.Prefix.Name != prefix) {
				continue
			}
			if record := e.modules.FindModule(imp); record != nil {
				return entryForModuleRecord(record, e.modules)
			}
		}
	}
	if mod, ok := e.Node.(*ASTModule); ok {
		for _, imp := range mod.Import {
			if imp == nil || imp.Prefix == nil || imp.Prefix.Name != prefix {
				continue
			}
			imported := moduleForImport(mod, imp)
			if imported == nil {
				return nil
			}
			switch module := imported.(type) {
			case *ASTModule:
				return entryFromASTModule(module)
			case *Module:
				return entryFromCompatModule(module)
			default:
				return entryFromASTNode(imported)
			}
		}
	}
	return nil
}

func (ms *Modules) moduleRecord(name, revision string) *Module {
	if ms == nil || name == "" {
		return nil
	}
	if revision != "" {
		if record := ms.Modules[name+"@"+revision]; record != nil {
			return record
		}
	}
	return ms.Modules[name]
}

func (ms *Modules) moduleRecordForImport(name, revision string) *Module {
	record := ms.moduleRecord(name, revision)
	if record != nil {
		return record
	}
	if ms == nil || ms.ctx == nil || name == "" {
		return nil
	}
	var revisionPtr *string
	if revision != "" {
		revisionPtr = &revision
	}
	if mod, ok := ms.ctx.GetModule(name, revisionPtr); ok {
		return ms.recordModule(mod)
	}
	return nil
}

func entryForModuleRecord(record *Module, modules *Modules) *Entry {
	if record == nil {
		return nil
	}
	entry := entryFromCompatModule(record)
	if entry != nil && entry.modules == nil {
		entry.modules = modules
	}
	return entry
}

// IsDir reports whether this entry can contain children.
func (e *Entry) IsDir() bool {
	return e != nil && e.Dir != nil
}

// IsLeaf reports whether this entry is a scalar leaf.
func (e *Entry) IsLeaf() bool {
	return e != nil && !e.IsDir() && e.Kind == LeafEntry && e.ListAttr == nil
}

// IsLeafList reports whether this entry is a leaf-list.
func (e *Entry) IsLeafList() bool {
	return e != nil && !e.IsDir() && e.Kind == LeafEntry && e.ListAttr != nil
}

// IsList reports whether this entry is a list.
func (e *Entry) IsList() bool {
	return e != nil && e.IsDir() && e.ListAttr != nil
}

// IsContainer reports whether this entry is a container-like directory.
func (e *Entry) IsContainer() bool {
	return e != nil && e.Kind == DirectoryEntry && e.ListAttr == nil
}

// IsChoice reports whether this entry is a choice.
func (e *Entry) IsChoice() bool {
	return e != nil && e.Kind == ChoiceEntry
}

// IsCase reports whether this entry is a case.
func (e *Entry) IsCase() bool {
	return e != nil && e.Kind == CaseEntry
}

// Augment applies pending augment entries into their target entries.
func (e *Entry) Augment(addErrors bool) (processed, skipped int) {
	if e == nil {
		return 0, 0
	}
	var unapplied []*Entry
	for _, augment := range e.Augments {
		if augment == nil {
			continue
		}
		target := augment.Find(augment.Name)
		if target == nil {
			if addErrors {
				e.Errors = append(e.Errors, fmt.Errorf("%s: augment %s not found", Source(augment.Node), augment.Name))
			}
			skipped++
			unapplied = append(unapplied, augment)
			continue
		}
		processed++
		target.merge(nil, augment.Namespace(), augment)
		target.Augmented = append(target.Augmented, augment.shallowDup())
	}
	e.Augments = unapplied
	return processed, skipped
}

// ApplyDeviate walks the deviations within e and applies them to the projected
// schema, matching goyang's migration behavior for cgo-free compat callers.
func (e *Entry) ApplyDeviate(deviateOpts ...DeviateOpt) []error {
	var errs []error
	appendErr := func(err error) {
		if err != nil {
			errs = append(errs, err)
		}
	}
	for _, deviation := range e.Deviations {
		if deviation == nil {
			continue
		}
		deviatedNode := e.Find(deviation.DeviatedPath)
		if deviatedNode == nil {
			appendErr(fmt.Errorf("cannot find target node to deviate, %s", deviation.DeviatedPath))
			continue
		}
		for deviationType, deviateEntries := range deviation.Deviate {
			for _, devSpec := range deviateEntries {
				if devSpec == nil {
					continue
				}
				switch deviationType {
				case DeviationAdd, DeviationReplace:
					applyAddOrReplaceDeviation(deviatedNode, devSpec, deviationType, appendErr, Source(e.Node))
				case DeviationNotSupported:
					parent := deviatedNode.Parent
					if parent == nil {
						appendErr(fmt.Errorf("%s: node %s does not have a valid parent, but deviate not-supported references one", Source(e.Node), e.Name))
						continue
					}
					if !hasIgnoreDeviateNotSupported(deviateOpts) {
						parent.delete(deviatedNode.Name)
					}
				case DeviationDelete:
					applyDeleteDeviation(deviatedNode, devSpec, deviation.DeviatedPath, appendErr, Source(e.Node))
				default:
					appendErr(fmt.Errorf("invalid deviation type %s", deviationType))
				}
			}
		}
	}
	return errs
}

func applyAddOrReplaceDeviation(deviatedNode, devSpec *Entry, deviationType deviationType, appendErr func(error), source string) {
	if devSpec.Config != TSUnset {
		deviatedNode.Config = devSpec.Config
	}
	if len(devSpec.Default) > 0 {
		switch deviationType {
		case DeviationAdd:
			switch {
			case deviatedNode.IsLeafList():
				deviatedNode.Default = append(deviatedNode.Default, devSpec.Default...)
			case len(devSpec.Default) > 1:
				appendErr(fmt.Errorf("%s: tried to add more than one default to a non-leaflist entry at deviation", source))
			case len(deviatedNode.Default) != 0:
				appendErr(fmt.Errorf("%s: tried to add a default value to an entry that already has a default value", source))
			case len(devSpec.Default) == 1 && len(deviatedNode.Default) == 0:
				deviatedNode.Default = append([]string{}, devSpec.Default[0])
			}
		case DeviationReplace:
			deviatedNode.Default = append([]string{}, devSpec.Default...)
		}
	}
	if devSpec.Mandatory != TSUnset {
		deviatedNode.Mandatory = devSpec.Mandatory
	}
	if devSpec.deviatePresence.hasMinElements {
		if !deviatedNode.IsList() && !deviatedNode.IsLeafList() {
			appendErr(fmt.Errorf("tried to deviate min-elements on a non-list type %s", deviatedNode.Kind))
		} else if devSpec.ListAttr != nil && deviatedNode.ListAttr != nil {
			deviatedNode.ListAttr.MinElements = devSpec.ListAttr.MinElements
		}
	}
	if devSpec.deviatePresence.hasMaxElements {
		if !deviatedNode.IsList() && !deviatedNode.IsLeafList() {
			appendErr(fmt.Errorf("tried to deviate max-elements on a non-list type %s", deviatedNode.Kind))
		} else if devSpec.ListAttr != nil && deviatedNode.ListAttr != nil {
			deviatedNode.ListAttr.MaxElements = devSpec.ListAttr.MaxElements
		}
	}
	if devSpec.Units != "" {
		deviatedNode.Units = devSpec.Units
	}
	if devSpec.Type != nil {
		deviatedNode.Type = devSpec.Type
	}
}

func applyDeleteDeviation(deviatedNode, devSpec *Entry, deviatedPath string, appendErr func(error), source string) {
	if devSpec.Config != TSUnset {
		deviatedNode.Config = TSUnset
	}
	if len(devSpec.Default) > 0 {
		switch {
		case deviatedNode.IsLeafList():
			appendErr(fmt.Errorf("%s: deviate delete on default statements unsupported for leaf-lists, please use replace instead", source))
		case len(deviatedNode.Default) == 0:
			appendErr(fmt.Errorf("%s: tried to deviate delete a default statement that doesn't exist", source))
		case devSpec.Default[0] != deviatedNode.Default[0]:
			appendErr(fmt.Errorf("%s: tried to deviate delete a default statement with a non-matching keyword", source))
		default:
			deviatedNode.Default = nil
		}
	}
	if devSpec.Mandatory != TSUnset {
		deviatedNode.Mandatory = TSUnset
	}
	if devSpec.deviatePresence.hasMinElements {
		if !deviatedNode.IsList() && !deviatedNode.IsLeafList() {
			appendErr(fmt.Errorf("tried to deviate min-elements on a non-list type %s", deviatedNode.Kind))
		} else if devSpec.ListAttr != nil && deviatedNode.ListAttr != nil {
			if deviatedNode.ListAttr.MinElements != devSpec.ListAttr.MinElements {
				appendErr(fmt.Errorf("min-element value %d differs from deviation's min-element value %d for entry %v", devSpec.ListAttr.MinElements, deviatedNode.ListAttr.MinElements, deviatedPath))
			}
			deviatedNode.ListAttr.MinElements = 0
		}
	}
	if devSpec.deviatePresence.hasMaxElements {
		if !deviatedNode.IsList() && !deviatedNode.IsLeafList() {
			appendErr(fmt.Errorf("tried to deviate max-elements on a non-list type %s", deviatedNode.Kind))
		} else if devSpec.ListAttr != nil && deviatedNode.ListAttr != nil {
			if deviatedNode.ListAttr.MaxElements != devSpec.ListAttr.MaxElements {
				appendErr(fmt.Errorf("max-element value %d differs from deviation's max-element value %d for entry %v", devSpec.ListAttr.MaxElements, deviatedNode.ListAttr.MaxElements, deviatedPath))
			}
			deviatedNode.ListAttr.MaxElements = math.MaxUint64
		}
	}
}

func hasIgnoreDeviateNotSupported(opts []DeviateOpt) bool {
	for _, opt := range opts {
		if deviateOptions, ok := opt.(DeviateOptions); ok {
			return deviateOptions.IgnoreDeviateNotSupported
		}
	}
	return false
}

func (e *Entry) delete(key string) {
	if e == nil {
		return
	}
	if _, ok := e.Dir[key]; !ok {
		e.Errors = append(e.Errors, fmt.Errorf("%s: unknown child key %s", Source(e.Node), key))
	}
	delete(e.Dir, key)
	for i := 0; i < len(e.ordered); {
		if e.ordered[i] != nil && e.ordered[i].Name == key {
			e.ordered = append(e.ordered[:i], e.ordered[i+1:]...)
			continue
		}
		i++
	}
}

func (e *Entry) importErrors(other *Entry) {
	if e == nil || other == nil {
		return
	}
	e.Errors = append(e.Errors, other.GetErrors()...)
}

func (e *Entry) shallowDup() *Entry {
	if e == nil {
		return nil
	}
	duplicate := *e
	duplicate.Extra = cloneExtraMap(e.Extra)
	duplicate.Annotation = cloneAnnotationMap(e.Annotation)
	duplicate.Dir = nil
	duplicate.ordered = nil
	if e.Dir != nil {
		duplicate.Dir = make(map[string]*Entry, len(e.Dir))
		for _, pair := range orderedEntryChildPairs(e) {
			if pair.entry == nil {
				continue
			}
			child := *pair.entry
			child.Dir = nil
			child.ordered = nil
			child.Parent = &duplicate
			child.Extra = cloneExtraMap(pair.entry.Extra)
			child.Annotation = cloneAnnotationMap(pair.entry.Annotation)
			duplicate.Dir[pair.key] = &child
			duplicate.ordered = append(duplicate.ordered, &child)
		}
	}
	return &duplicate
}

func (e *Entry) dup() *Entry {
	if e == nil {
		return nil
	}
	duplicate := *e
	duplicate.Extra = cloneExtraMap(e.Extra)
	duplicate.Annotation = cloneAnnotationMap(e.Annotation)
	duplicate.Dir = nil
	duplicate.ordered = nil
	if e.Dir != nil {
		duplicate.Dir = make(map[string]*Entry, len(e.Dir))
		for _, pair := range orderedEntryChildPairs(e) {
			if pair.entry == nil {
				continue
			}
			child := pair.entry.dup()
			child.Parent = &duplicate
			duplicate.Dir[pair.key] = child
			duplicate.ordered = append(duplicate.ordered, child)
		}
	}
	return &duplicate
}

func (e *Entry) merge(prefix *Value, namespace *Value, other *Entry) {
	if e == nil || other == nil {
		return
	}
	e.importErrors(other)
	if e.Dir == nil {
		e.Dir = make(map[string]*Entry)
	}
	for _, pair := range orderedEntryChildPairs(other) {
		if pair.entry == nil {
			continue
		}
		child := pair.entry.dup()
		if prefix != nil {
			child.Prefix = prefix
		}
		if namespace != nil {
			child.namespace = namespace
		}
		if existing := e.Dir[pair.key]; existing != nil {
			e.Errors = append(e.Errors, fmt.Errorf(`duplicate node %q in %q from:
   %s: %s
   %s: %s`, pair.key, e.Name, Source(child.Node), child.Name, Source(existing.Node), existing.Name))
			continue
		}
		child.Parent = e
		child.Exts = append(child.Exts, other.Exts...)
		if child.Extra == nil {
			child.Extra = make(map[string][]interface{})
		}
		for key, values := range other.Extra {
			child.Extra[key] = append(child.Extra[key], values...)
		}
		e.Dir[pair.key] = child
		e.ordered = append(e.ordered, child)
	}
}

type entryChildPair struct {
	key   string
	entry *Entry
}

func orderedEntryChildPairs(entry *Entry) []entryChildPair {
	if entry == nil || len(entry.Dir) == 0 {
		return nil
	}
	var pairs []entryChildPair
	seenKeys := make(map[string]bool)
	for _, child := range entry.ordered {
		key, ok := entry.keyForChild(child)
		if !ok || seenKeys[key] {
			continue
		}
		pairs = append(pairs, entryChildPair{key: key, entry: child})
		seenKeys[key] = true
	}
	var remaining []string
	for key := range entry.Dir {
		if !seenKeys[key] {
			remaining = append(remaining, key)
		}
	}
	sort.Strings(remaining)
	for _, key := range remaining {
		pairs = append(pairs, entryChildPair{key: key, entry: entry.Dir[key]})
	}
	return pairs
}

func (e *Entry) keyForChild(child *Entry) (string, bool) {
	if e == nil || child == nil {
		return "", false
	}
	if e.Dir[child.Name] == child {
		return child.Name, true
	}
	for key, candidate := range e.Dir {
		if candidate == child {
			return key, true
		}
	}
	return "", false
}

func cloneExtraMap(in map[string][]interface{}) map[string][]interface{} {
	if in == nil {
		return nil
	}
	out := make(map[string][]interface{}, len(in))
	for key, values := range in {
		out[key] = append([]interface{}(nil), values...)
	}
	return out
}

func cloneAnnotationMap(in map[string]interface{}) map[string]interface{} {
	if in == nil {
		return nil
	}
	out := make(map[string]interface{}, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}

// FixChoice inserts missing Case entries for non-case children of choice
// entries, matching goyang's schema normalization while keeping Cambium's
// ordered child slice coherent.
func (e *Entry) FixChoice() {
	if e == nil {
		return
	}
	replacements := map[*Entry]*Entry{}
	if e.Kind == ChoiceEntry && len(e.Errors) == 0 {
		for key, child := range e.Dir {
			if child == nil || child.Kind == CaseEntry {
				continue
			}
			implicitCase := e.implicitCaseForChoiceChild(child)
			child.Parent = implicitCase
			e.Dir[key] = implicitCase
			replacements[child] = implicitCase
		}
		for i, child := range e.ordered {
			if replacement := replacements[child]; replacement != nil {
				e.ordered[i] = replacement
			}
		}
	}

	seen := map[*Entry]bool{}
	for _, child := range e.ordered {
		if child == nil || seen[child] {
			continue
		}
		seen[child] = true
		child.FixChoice()
	}
	for _, child := range e.Dir {
		if child == nil || seen[child] {
			continue
		}
		seen[child] = true
		child.FixChoice()
	}
}

func (e *Entry) implicitCaseForChoiceChild(child *Entry) *Entry {
	var (
		name       = child.Name
		parent     Node
		source     *Statement
		extensions []*Statement
	)
	if child.Node != nil {
		parent = child.Node.ParentNode()
		source = child.Node.Statement()
		extensions = child.Node.Exts()
		if nodeName := child.Node.NName(); nodeName != "" {
			name = nodeName
		}
	}
	return &Entry{
		Parent: e,
		Node: &Case{
			Parent:     parent,
			Name:       name,
			Source:     source,
			Extensions: extensions,
		},
		Name:    child.Name,
		Kind:    CaseEntry,
		Config:  child.Config,
		Prefix:  child.Prefix,
		Dir:     map[string]*Entry{child.Name: child},
		Extra:   map[string][]interface{}{},
		ordered: []*Entry{child},
	}
}

// ReadOnly reports whether this entry represents config false data.
func (e *Entry) ReadOnly() bool {
	switch {
	case e == nil:
		return false
	case e.Kind == OutputEntry:
		return true
	case e.Config == TSUnset:
		return e.Parent.ReadOnly()
	default:
		return !e.Config.Value()
	}
}

// Print prints e in a goyang-like human-readable form.
//
// Children are printed in Cambium effective declaration order via Children(),
// never by iterating Entry.Dir.
func (e *Entry) Print(w io.Writer) {
	printEntry(w, e, "")
}

// SingleDefaultValue returns the single default value, if one exists.
func (e *Entry) SingleDefaultValue() (string, bool) {
	if defaults := e.DefaultValues(); len(defaults) == 1 {
		return defaults[0], true
	}
	return "", false
}

// DefaultValues returns all default values.
func (e *Entry) DefaultValues() []string {
	if e == nil {
		return nil
	}
	if len(e.Default) > 0 {
		return append([]string(nil), e.Default...)
	}
	if e.Type == nil || !e.Type.HasDefault {
		return nil
	}
	if leaf, ok := e.Node.(*Leaf); ok && leaf != nil {
		switch {
		case e.IsLeaf() && (leaf.Mandatory == nil || leaf.Mandatory.Name == "false"):
			return []string{e.Type.Default}
		case e.IsLeafList() && e.ListAttr != nil && e.ListAttr.MinElements == 0:
			return []string{e.Type.Default}
		default:
			return nil
		}
	}
	if e.Node == nil {
		return nil
	}
	switch e.Node.Kind() {
	case "leaf":
		if e.IsLeaf() && e.Mandatory != TSTrue {
			return []string{e.Type.Default}
		}
	case "leaf-list":
		if e.IsLeafList() && e.ListAttr != nil && e.ListAttr.MinElements == 0 {
			return []string{e.Type.Default}
		}
	}
	return nil
}

func printEntry(w io.Writer, e *Entry, indent string) {
	if e == nil {
		return
	}
	if e.Description != "" {
		_, _ = fmt.Fprintln(w)
		for _, line := range strings.Split(e.Description, "\n") {
			_, _ = fmt.Fprintf(w, "%s// %s\n", indent, line)
		}
	}
	mode := "rw"
	if e.ReadOnly() {
		mode = "RO"
	}
	_, _ = fmt.Fprintf(w, "%s%s: ", indent, mode)
	if e.Type != nil && e.Type.Name != "" {
		_, _ = fmt.Fprintf(w, "%s ", e.Type.Name)
	}
	switch {
	case e.IsLeafList():
		_, _ = fmt.Fprintf(w, "[]%s\n", e.Name)
	case e.IsLeaf():
		_, _ = fmt.Fprintf(w, "%s\n", e.Name)
	case e.IsList():
		_, _ = fmt.Fprintf(w, "[%s]%s {\n", e.Key, e.Name)
		for _, child := range e.Children() {
			printEntry(w, child, indent+"  ")
		}
		_, _ = fmt.Fprintf(w, "%s}\n", indent)
	case e.IsDir():
		_, _ = fmt.Fprintf(w, "%s {\n", e.Name)
		for _, child := range e.Children() {
			printEntry(w, child, indent+"  ")
		}
		_, _ = fmt.Fprintf(w, "%s}\n", indent)
	default:
		_, _ = fmt.Fprintf(w, "%s\n", e.Name)
	}
}

// Namespace returns the XML/YANG namespace for this entry.
func (e *Entry) Namespace() *Value {
	if e == nil {
		return nil
	}
	if e.namespace != nil {
		return e.namespace
	}
	if e.schemaNode.Name() != "" {
		return valueOrNil(e.schemaNode.Namespace())
	}
	if ns := valueOrNil(e.module.Namespace()); ns != nil {
		return ns
	}
	if ns := valueOrNil(e.rawRootNamespace()); ns != nil {
		return ns
	}
	return &Value{}
}

// InstantiatingModule returns the YANG module name that instantiated this entry.
func (e *Entry) InstantiatingModule() (string, error) {
	if e == nil {
		return "", fmt.Errorf("nil entry")
	}
	if ns := e.Namespace(); ns == nil || ns.Name == "" {
		return "", fmt.Errorf("entry %s had nil namespace", e.Name)
	}
	if e.schemaNode.Name() != "" {
		name := e.schemaNode.Module().Name()
		if name == "" {
			return "", fmt.Errorf("could not find module for entry %s", e.Name)
		}
		return name, nil
	}
	name := e.module.Name()
	if name != "" {
		return name, nil
	}
	if name := e.rawRootModuleName(); name != "" {
		return name, nil
	}
	return "", fmt.Errorf("could not find module for entry %s", e.Name)
}

func (e *Entry) rawRootModuleName() string {
	root := e.rawRootEntry()
	if root == nil {
		return ""
	}
	switch node := root.Node.(type) {
	case *ASTModule:
		if node.BelongsTo != nil {
			return node.BelongsTo.Name
		}
		return node.Name
	case *Module:
		if node.BelongsTo != nil {
			return node.BelongsTo.Name
		}
		return node.Name
	}
	return root.Name
}

func (e *Entry) rawRootNamespace() string {
	root := e.rawRootEntry()
	if root == nil {
		return ""
	}
	switch node := root.Node.(type) {
	case *ASTModule:
		if node.BelongsTo != nil {
			if parent := upstreamParentModule(node); parent != nil && parent.Namespace != nil {
				return parent.Namespace.Name
			}
		}
		if node.Namespace != nil {
			return node.Namespace.Name
		}
	case *Module:
		if node.BelongsTo != nil {
			if node.Modules != nil {
				if parent := node.Modules.Modules[node.BelongsTo.Name]; parent != nil && parent.Namespace != nil {
					return parent.Namespace.Name
				}
			}
		}
		if node.Namespace != nil {
			return node.Namespace.Name
		}
	}
	return root.rawRootChildArgument("namespace")
}

func upstreamParentModule(node *ASTModule) *ASTModule {
	if node == nil || node.BelongsTo == nil || node.Modules == nil {
		return nil
	}
	return node.Modules.Modules[node.BelongsTo.Name]
}

func (e *Entry) rawRootChildArgument(keyword string) string {
	root := e.rawRootEntry()
	if root == nil || root.Node == nil {
		return ""
	}
	stmt := root.Node.Statement()
	if stmt == nil {
		return ""
	}
	return childArgument(stmt, keyword)
}

func (e *Entry) rawRootEntry() *Entry {
	for e != nil && e.Parent != nil {
		e = e.Parent
	}
	return e
}

// Path returns the Cambium schema path for this entry, or a module-root path.
func (e *Entry) Path() string {
	if e == nil {
		return ""
	}
	if e.schemaNode.Name() != "" {
		return e.schemaNode.Path()
	}
	if e.module.Name() != "" {
		return "/" + e.module.Name()
	}
	if e.Parent != nil {
		return e.Parent.Path() + "/" + e.Name
	}
	if e.Name != "" {
		return "/" + e.Name
	}
	return ""
}

func localPart(part string) string {
	if _, local, ok := strings.Cut(part, ":"); ok {
		return local
	}
	return part
}

func projectNode(node cambium.SchemaNodeRef, parent *Entry) *Entry {
	kind := kindForNode(node)
	config := triStateForConfig(node.Config())
	if config == TSTrue && parent != nil && parent.inOutputSubtree() {
		config = TSUnset
	}
	entry := &Entry{
		Name:       node.Name(),
		Kind:       kind,
		Config:     config,
		Prefix:     valueOrNil(node.Module().Prefix()),
		Mandatory:  triStateForBool(node.IsMandatory()),
		Parent:     parent,
		Exts:       statementsForExtensions(node.Extensions()),
		Extra:      make(map[string][]interface{}),
		Annotation: make(map[string]interface{}),
		Node:       entryNodeForSchemaNode(node, parent),
		module:     node.Module(),
		schemaNode: node,
	}
	if entryKindHasDir(kind) {
		entry.Dir = make(map[string]*Entry)
	}
	if desc, ok := node.Description(); ok {
		entry.Description = desc
	}
	entry.Default = node.DefaultValues()
	if units, ok := node.Units(); ok {
		entry.Units = units
	}
	if typ := typeForNode(node); typ != nil {
		entry.Type = typ
	}
	if attr := listAttrForNode(node); attr != nil {
		entry.ListAttr = attr
		if keys := node.KeyNames(); len(keys) > 0 {
			entry.Key = strings.Join(keys, " ")
		}
	}
	if node.IsRPC() || node.IsAction() {
		entry.RPC = &RPCEntry{}
	}
	for child := range node.Children().Iter() {
		childEntry := projectNode(child, entry)
		entry.add(childEntry)
		switch child.Kind() {
		case cambium.SchemaNodeKindInput:
			if entry.RPC == nil {
				entry.RPC = &RPCEntry{}
			}
			entry.RPC.Input = childEntry
		case cambium.SchemaNodeKindOutput:
			if entry.RPC == nil {
				entry.RPC = &RPCEntry{}
			}
			entry.RPC.Output = childEntry
		}
	}
	return entry
}

func (e *Entry) inOutputSubtree() bool {
	for cur := e; cur != nil; cur = cur.Parent {
		switch cur.Kind {
		case OutputEntry:
			return true
		case InputEntry:
			return false
		}
	}
	return false
}

func entryKindHasDir(kind EntryKind) bool {
	switch kind {
	case DirectoryEntry, ChoiceEntry, CaseEntry, AnyDataEntry, AnyXMLEntry, InputEntry, OutputEntry, NotificationEntry, DeviateEntry:
		return true
	default:
		return false
	}
}

func statementsForExtensions(exts []cambium.Extension) []*Statement {
	if len(exts) == 0 {
		return nil
	}
	out := make([]*Statement, 0, len(exts))
	for _, ext := range exts {
		argument, hasArgument := ext.Argument()
		out = append(out, &Statement{
			Keyword:     ext.ModuleName() + ":" + ext.Name(),
			HasArgument: hasArgument,
			Argument:    argument,
		})
	}
	return out
}

func (e *Entry) add(child *Entry) {
	if e == nil || child == nil {
		return
	}
	if e.Dir == nil {
		e.Dir = make(map[string]*Entry)
	}
	e.Dir[child.Name] = child
	e.ordered = append(e.ordered, child)
}

func kindForNode(node cambium.SchemaNodeRef) EntryKind {
	switch node.Kind() {
	case cambium.SchemaNodeKindLeaf, cambium.SchemaNodeKindLeafList:
		return LeafEntry
	case cambium.SchemaNodeKindChoice:
		return ChoiceEntry
	case cambium.SchemaNodeKindCase:
		return CaseEntry
	case cambium.SchemaNodeKindAnyData:
		return AnyDataEntry
	case cambium.SchemaNodeKindInput:
		return InputEntry
	case cambium.SchemaNodeKindOutput:
		return OutputEntry
	case cambium.SchemaNodeKindNotification:
		return NotificationEntry
	default:
		return DirectoryEntry
	}
}

func listAttrForNode(node cambium.SchemaNodeRef) *ListAttr {
	if !node.IsList() && !node.IsLeafList() {
		return nil
	}
	orderedBy := "system"
	if node.OrderedBy() == cambium.OrderedByUser {
		orderedBy = "user"
	}
	attr := &ListAttr{
		MaxElements:   math.MaxUint64,
		OrderedBy:     &Value{Name: orderedBy},
		OrderedByUser: node.OrderedBy() == cambium.OrderedByUser,
	}
	if min, ok := node.MinElements(); ok {
		attr.MinElements = uint64(min)
	}
	if max, ok := node.MaxElements(); ok {
		attr.MaxElements = uint64(max)
	}
	return attr
}

func triStateForBool(v bool) TriState {
	if v {
		return TSTrue
	}
	return TSFalse
}

func triStateForConfig(v cambium.Config) TriState {
	if v == cambium.ConfigRo {
		return TSFalse
	}
	return TSTrue
}

func valueOrNil(name string) *Value {
	if name == "" {
		return nil
	}
	return &Value{Name: name}
}

func typeForNode(node cambium.SchemaNodeRef) *YangType {
	info, ok := node.LeafType()
	if !ok {
		return nil
	}
	typ := typeForInfo(info)
	if typ == nil {
		return nil
	}
	if units, ok := node.Units(); ok {
		typ.Units = units
	}
	if def, ok := node.TypeDefaultValue(); ok {
		typ.Default = def
		typ.HasDefault = true
	}
	return typ
}

func typeForInfo(info cambium.TypeInfo) *YangType {
	name := info.Base().String()
	typedefChain := info.TypedefChain()
	if len(typedefChain) > 0 {
		name = typedefChain[0]
	} else if typedef, ok := info.TypedefName(); ok {
		name = typedef
	}
	typ := &YangType{
		Name: name,
		Kind: typeKindForBase(info.Base()),
		Root: yangTypeRootForBase(info.Base()),
	}
	switch {
	case len(typedefChain) > 0:
		typ.Base = typeBaseForTypedefChain(typedefChain[1:], info.Base())
	case name != info.Base().String():
		typ.Base = typeBaseForTypedefChain(nil, info.Base())
	}
	switch resolved := info.Resolved().(type) {
	case cambium.ResolvedInt:
		typ.Range = rangeFromBounds(resolved.Range, rangeBoundRange, info.Base(), 0)
		if typ.Range == nil {
			typ.Range = defaultIntegerRange(info.Base())
		}
	case cambium.ResolvedDecimal64:
		typ.FractionDigits = int(resolved.FractionDigits().Value())
		typ.Range = rangeFromBounds(resolved.Range, rangeBoundRange, cambium.BaseTypeDecimal64, typ.FractionDigits)
	case cambium.ResolvedString:
		typ.Length = rangeFromBounds(resolved.Length, rangeBoundLength, cambium.BaseTypeString, 0)
		for _, pattern := range resolved.Patterns {
			typ.Pattern = append(typ.Pattern, pattern.Regex())
		}
	case cambium.ResolvedBinary:
		typ.Length = rangeFromBounds(resolved.Length, rangeBoundLength, cambium.BaseTypeBinary, 0)
	case cambium.ResolvedEnumeration:
		typ.Enum = enumTypeFromValues(NewEnumType(), resolved.Values())
	case cambium.ResolvedBits:
		typ.Bit = enumTypeFromValues(NewBitfield(), resolved.Values())
	case cambium.ResolvedIdentityRef:
		for _, base := range resolved.Bases() {
			typ.IdentityBases = append(typ.IdentityBases, identityFromCambium(base, make(map[string]*Identity)))
		}
		if len(typ.IdentityBases) > 0 {
			typ.IdentityBase = typ.IdentityBases[0]
		}
	case cambium.ResolvedLeafRef:
		typ.Name = "leafref"
		typ.Kind = Yleafref
		if path, ok := resolved.Path(); ok {
			typ.Path = path
		}
		typ.OptionalInstance = !resolved.RequireInstance()
	case cambium.ResolvedUnion:
		typ.Name = "union"
		typ.Kind = Yunion
		for _, member := range resolved.Members() {
			typ.Type = append(typ.Type, typeForInfo(member))
		}
	}
	return typ
}

func typeBaseForTypedefChain(chain []string, base cambium.BaseType) *Type {
	baseName := base.String()
	if baseName == "" || baseName == "unknown" {
		return nil
	}
	root := upstreamYangTypeRootForBase(base)
	nextName := baseName
	nextYang := upstreamResolvedYangTypeForBase(base, root)
	for i := len(chain) - 1; i >= 0; i-- {
		name := chain[i]
		nextYang = &upstream.YangType{
			Name: name,
			Kind: upstreamTypeKindForBase(base),
			Base: &Type{
				Name:     nextName,
				Source:   typeStatement(nextName),
				YangType: nextYang,
			},
			Root: root,
		}
		nextName = name
	}
	return &Type{
		Name:     nextName,
		Source:   typeStatement(nextName),
		YangType: nextYang,
	}
}

func upstreamResolvedYangTypeForBase(base cambium.BaseType, root *upstream.YangType) *upstream.YangType {
	name := base.String()
	return &upstream.YangType{
		Name: name,
		Kind: upstreamTypeKindForBase(base),
		Base: &Type{
			Name:     name,
			Source:   typeStatement(name),
			YangType: root,
		},
		Root: root,
	}
}

func upstreamYangTypeRootForBase(base cambium.BaseType) *upstream.YangType {
	name := base.String()
	root := &upstream.YangType{
		Name: name,
		Kind: upstreamTypeKindForBase(base),
	}
	root.Root = root
	return root
}

func upstreamTypeKindForBase(base cambium.BaseType) upstream.TypeKind {
	return upstream.TypeKind(typeKindForBase(base))
}

func typeStatement(name string) *Statement {
	return &Statement{
		Keyword:     "type",
		HasArgument: true,
		Argument:    name,
	}
}

func defaultIntegerRange(base cambium.BaseType) YangRange {
	switch base {
	case cambium.BaseTypeInt8:
		return cloneYangRange(Int8Range)
	case cambium.BaseTypeInt16:
		return cloneYangRange(Int16Range)
	case cambium.BaseTypeInt32:
		return cloneYangRange(Int32Range)
	case cambium.BaseTypeInt64:
		return cloneYangRange(Int64Range)
	case cambium.BaseTypeUint8:
		return cloneYangRange(Uint8Range)
	case cambium.BaseTypeUint16:
		return cloneYangRange(Uint16Range)
	case cambium.BaseTypeUint32:
		return cloneYangRange(Uint32Range)
	case cambium.BaseTypeUint64:
		return cloneYangRange(Uint64Range)
	default:
		return nil
	}
}

func yangTypeRootForBase(base cambium.BaseType) *YangType {
	name := base.String()
	if name == "" || name == "unknown" {
		return nil
	}
	root := &YangType{
		Name: name,
		Kind: typeKindForBase(base),
	}
	root.Root = root
	switch base {
	case cambium.BaseTypeInt8:
		root.Range = cloneYangRange(Int8Range)
	case cambium.BaseTypeInt16:
		root.Range = cloneYangRange(Int16Range)
	case cambium.BaseTypeInt32:
		root.Range = cloneYangRange(Int32Range)
	case cambium.BaseTypeInt64:
		root.Range = cloneYangRange(Int64Range)
	case cambium.BaseTypeUint8:
		root.Range = cloneYangRange(Uint8Range)
	case cambium.BaseTypeUint16:
		root.Range = cloneYangRange(Uint16Range)
	case cambium.BaseTypeUint32:
		root.Range = cloneYangRange(Uint32Range)
	case cambium.BaseTypeUint64:
		root.Range = cloneYangRange(Uint64Range)
	}
	return root
}

func cloneYangRange(r YangRange) YangRange {
	if len(r) == 0 {
		return nil
	}
	out := make(YangRange, len(r))
	copy(out, r)
	return out
}

func identityFromCambium(id cambium.Identity, seen map[string]*Identity) *Identity {
	if id.Name() == "" {
		return nil
	}
	key := id.Module().Name() + ":" + id.Name()
	if existing := seen[key]; existing != nil {
		return existing
	}
	parent := &Module{Name: id.Module().Name(), Prefix: valueOrNil(id.Module().Prefix())}
	out := &Identity{
		Name:   id.Name(),
		Parent: parent,
		Status: &Value{Name: statusName(id.Status())},
	}
	seen[key] = out
	if desc, ok := id.Description(); ok {
		out.Description = &Value{Name: desc}
	}
	if ref, ok := id.Reference(); ok {
		out.Reference = &Value{Name: ref}
	}
	for _, feature := range id.IfFeatures() {
		out.IfFeature = append(out.IfFeature, &Value{Name: feature})
	}
	for _, base := range id.Bases() {
		if base.Name() != "" {
			out.Base = append(out.Base, &Value{Name: identityPrefixedName(base)})
		}
	}
	for _, derived := range id.Derived() {
		if child := identityFromCambium(derived, seen); child != nil {
			out.Values = append(out.Values, child)
		}
	}
	return out
}

func identityPrefixedName(id cambium.Identity) string {
	prefix := id.Module().Prefix()
	if prefix == "" {
		return id.Name()
	}
	return prefix + ":" + id.Name()
}

func statusName(status cambium.Status) string {
	switch status {
	case cambium.StatusDeprecated:
		return "deprecated"
	case cambium.StatusObsolete:
		return "obsolete"
	default:
		return "current"
	}
}

type rangeBoundKind int

const (
	rangeBoundRange rangeBoundKind = iota
	rangeBoundLength
)

func rangeFromBounds(bounds []cambium.RangeBound, kind rangeBoundKind, base cambium.BaseType, fractionDigits int) YangRange {
	if len(bounds) == 0 {
		return nil
	}
	out := make(YangRange, 0, len(bounds))
	for _, bound := range bounds {
		out = append(out, YRange{
			Min: numberFromString(bound.Min(), kind, base, fractionDigits),
			Max: numberFromString(bound.Max(), kind, base, fractionDigits),
		})
	}
	return out
}

func numberFromString(raw string, kind rangeBoundKind, base cambium.BaseType, fractionDigits int) Number {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return Number{}
	}
	if symbolic, ok := symbolicRangeNumber(raw, kind, base, fractionDigits); ok {
		return symbolic
	}
	negative := false
	unsigned := raw
	switch raw[0] {
	case '+':
		unsigned = raw[1:]
	case '-':
		negative = true
		unsigned = raw[1:]
	}
	if fractionDigits > 0 || strings.Contains(unsigned, ".") {
		return decimalNumberFromString(unsigned, negative, fractionDigits)
	}
	value, err := strconv.ParseUint(unsigned, 0, 64)
	if err != nil {
		return Number{}
	}
	return Number{Value: value, Negative: negative}
}

func symbolicRangeNumber(raw string, kind rangeBoundKind, base cambium.BaseType, fractionDigits int) (Number, bool) {
	switch raw {
	case "min":
		switch {
		case kind == rangeBoundLength:
			return Number{}, true
		case base == cambium.BaseTypeDecimal64 && fractionDigits >= 1 && fractionDigits <= int(MaxFractionDigits):
			return Number{Value: uint64(AbsMinInt64), FractionDigits: uint8(fractionDigits), Negative: true}, true
		}
	case "max":
		switch {
		case kind == rangeBoundLength:
			return Number{Value: math.MaxUint64}, true
		case base == cambium.BaseTypeDecimal64 && fractionDigits >= 1 && fractionDigits <= int(MaxFractionDigits):
			return Number{Value: uint64(MaxInt64), FractionDigits: uint8(fractionDigits)}, true
		}
	}
	return Number{}, false
}

func decimalNumberFromString(unsigned string, negative bool, fractionDigits int) Number {
	fd := fractionDigits
	if fd == 0 {
		if _, frac, ok := strings.Cut(unsigned, "."); ok {
			fd = len(frac)
		}
	}
	if fd < 0 || fd > 18 {
		return Number{}
	}
	whole, frac, hasFrac := strings.Cut(unsigned, ".")
	if !hasFrac {
		frac = ""
	}
	if len(frac) > fd {
		return Number{}
	}
	scaled := whole + frac + strings.Repeat("0", fd-len(frac))
	if scaled == "" {
		return Number{}
	}
	value, err := strconv.ParseUint(scaled, 10, 64)
	if err != nil {
		return Number{}
	}
	return Number{Value: value, FractionDigits: uint8(fd), Negative: negative}
}

func enumTypeFromValues(out *EnumType, values []cambium.EnumValue) *EnumType {
	if out == nil {
		return nil
	}
	for _, value := range values {
		name := value.Name()
		position := value.Value()
		out.ToInt[name] = position
		out.ToString[position] = name
		if position >= out.last {
			out.last = position
		}
	}
	return out
}

func typeKindForBase(base cambium.BaseType) TypeKind {
	switch base {
	case cambium.BaseTypeString:
		return Ystring
	case cambium.BaseTypeBoolean:
		return Ybool
	case cambium.BaseTypeInt8:
		return Yint8
	case cambium.BaseTypeInt16:
		return Yint16
	case cambium.BaseTypeInt32:
		return Yint32
	case cambium.BaseTypeInt64:
		return Yint64
	case cambium.BaseTypeUint8:
		return Yuint8
	case cambium.BaseTypeUint16:
		return Yuint16
	case cambium.BaseTypeUint32:
		return Yuint32
	case cambium.BaseTypeUint64:
		return Yuint64
	case cambium.BaseTypeDecimal64:
		return Ydecimal64
	case cambium.BaseTypeEmpty:
		return Yempty
	case cambium.BaseTypeBinary:
		return Ybinary
	case cambium.BaseTypeBits:
		return Ybits
	case cambium.BaseTypeEnumeration:
		return Yenum
	case cambium.BaseTypeIdentityRef:
		return Yidentityref
	case cambium.BaseTypeInstanceIdentifier:
		return YinstanceIdentifier
	case cambium.BaseTypeLeafRef:
		return Yleafref
	case cambium.BaseTypeUnion:
		return Yunion
	default:
		return Ynone
	}
}
