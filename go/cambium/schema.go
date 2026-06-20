package cambium

import (
	"encoding/base64"
	"errors"
	"fmt"
	"iter"
	"math/big"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/signalbreak-labs/cambium/go/internal/xsdregex"
	"github.com/signalbreak-labs/cambium/go/internal/yangparse"
)

// SchemaNodeKind classifies a YANG schema node.
type SchemaNodeKind int

const (
	SchemaNodeKindModule SchemaNodeKind = iota
	SchemaNodeKindContainer
	SchemaNodeKindLeaf
	SchemaNodeKindLeafList
	SchemaNodeKindList
	SchemaNodeKindChoice
	SchemaNodeKindCase
	SchemaNodeKindAnyData
	SchemaNodeKindRPC
	SchemaNodeKindAction
	SchemaNodeKindInput
	SchemaNodeKindOutput
	SchemaNodeKindNotification
	SchemaNodeKindUnknown
)

// Config is read-write vs read-only.
type Config int

const (
	ConfigRw Config = iota
	ConfigRo
)

// Status is the status substatement.
type Status int

const (
	StatusCurrent Status = iota
	StatusDeprecated
	StatusObsolete
)

// OrderedBy is the ordering semantics for lists and leaf-lists.
type OrderedBy int

const (
	OrderedBySystem OrderedBy = iota
	OrderedByUser
)

// LeafType is a coarse classification of a leaf/leaf-list value type.
type LeafType int

const (
	LeafTypeString LeafType = iota
	LeafTypeInt
	LeafTypeBool
	LeafTypeUnknown
)

// BaseType is the precise built-in YANG base type for a leaf or leaf-list.
type BaseType int

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
type FractionDigits struct{ value uint8 }

// NewFractionDigits creates a FractionDigits value.
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
	name        string
	value       int64
	description string
	reference   string
	status      Status
	ifFeatures  []string
	conditional bool
}

func (e EnumValue) Name() string   { return e.name }
func (e EnumValue) Value() int64   { return e.value }
func (e EnumValue) Status() Status { return e.status }
func (e EnumValue) IfFeatures() []string {
	return append([]string(nil), e.ifFeatures...)
}
func (e EnumValue) Description() (string, bool) {
	return optional(e.description)
}
func (e EnumValue) Reference() (string, bool) {
	return optional(e.reference)
}
func (d EnumDef) Values() []EnumValue { return append([]EnumValue(nil), d.values...) }

// EnumDef is the definition of an enumeration type.
type EnumDef struct{ values []EnumValue }

// BitsDef is the definition of a bits type.
type BitsDef struct{ values []EnumValue }

// Values returns the bit values in declaration order.
func (d BitsDef) Values() []EnumValue { return append([]EnumValue(nil), d.values...) }

// Pattern is a textual pattern constraint for strings.
type Pattern struct {
	regex                                string
	errorMessage, description, reference string
	appTag                               *string
	inverted                             bool
}

func (p Pattern) Regex() string                { return p.regex }
func (p Pattern) ErrorMessage() (string, bool) { return optional(p.errorMessage) }
func (p Pattern) ErrorAppTag() (string, bool) {
	if p.appTag == nil {
		return "", false
	}
	return *p.appTag, true
}
func (p Pattern) Description() (string, bool) { return optional(p.description) }
func (p Pattern) Reference() (string, bool)   { return optional(p.reference) }
func (p Pattern) IsInverted() bool            { return p.inverted }

// RangeBound is one bound of a numeric range or length constraint.
type RangeBound struct {
	min          string
	max          string
	errorMessage string
	errorAppTag  string
	description  string
	reference    string
}

func (r RangeBound) Min() string                  { return r.min }
func (r RangeBound) Max() string                  { return r.max }
func (r RangeBound) ErrorMessage() (string, bool) { return optional(r.errorMessage) }
func (r RangeBound) ErrorAppTag() (string, bool)  { return optional(r.errorAppTag) }
func (r RangeBound) Description() (string, bool)  { return optional(r.description) }
func (r RangeBound) Reference() (string, bool)    { return optional(r.reference) }

// ResolvedType is the sum-type interface for resolved leaf/leaf-list constraints.
type ResolvedType interface{ resolvedType() }

type ResolvedInt struct {
	Kind  IntKind
	Range []RangeBound
}

func (ResolvedInt) resolvedType() {}

type ResolvedDecimal64 struct {
	fractionDigits FractionDigits
	Range          []RangeBound
}

func (r ResolvedDecimal64) FractionDigits() FractionDigits { return r.fractionDigits }
func (ResolvedDecimal64) resolvedType()                    {}

type ResolvedBoolean struct{}

func (ResolvedBoolean) resolvedType() {}

type ResolvedEmpty struct{}

func (ResolvedEmpty) resolvedType() {}

type ResolvedBinary struct{ Length []RangeBound }

func (ResolvedBinary) resolvedType() {}

type ResolvedString struct {
	Length   []RangeBound
	Patterns []Pattern
}

func (ResolvedString) resolvedType() {}

type ResolvedEnumeration struct{ def EnumDef }

func (r ResolvedEnumeration) Values() []EnumValue { return r.def.Values() }
func (ResolvedEnumeration) resolvedType()         {}

type ResolvedBits struct{ def BitsDef }

func (r ResolvedBits) Values() []EnumValue { return r.def.Values() }
func (ResolvedBits) resolvedType()         {}

type ResolvedIdentityRef struct{ bases []Identity }

func (r ResolvedIdentityRef) Bases() []Identity { return append([]Identity(nil), r.bases...) }
func (ResolvedIdentityRef) resolvedType()       {}

type ResolvedInstanceIdentifier struct{ RequireInstance bool }

func (ResolvedInstanceIdentifier) resolvedType() {}

type ResolvedLeafRef struct {
	target          *SchemaNodeRef
	realtype        *TypeInfo
	requireInstance bool
	path            string
}

func (r ResolvedLeafRef) Target() (SchemaNodeRef, bool) {
	if r.target == nil {
		return SchemaNodeRef{}, false
	}
	return *r.target, true
}
func (r ResolvedLeafRef) Realtype() (*TypeInfo, bool) {
	if r.realtype == nil {
		return nil, false
	}
	clone := cloneTypeInfo(*r.realtype)
	return &clone, true
}
func (r ResolvedLeafRef) RequireInstance() bool { return r.requireInstance }
func (r ResolvedLeafRef) Path() (string, bool) {
	if r.path == "" {
		return "", false
	}
	return r.path, true
}
func (ResolvedLeafRef) resolvedType() {}

type ResolvedUnion struct{ members []TypeInfo }

func (r ResolvedUnion) Members() []TypeInfo { return cloneTypeInfos(r.members) }
func (ResolvedUnion) resolvedType()         {}

type ResolvedUnknown struct{}

func (ResolvedUnknown) resolvedType() {}

// TypeInfo is rich type information for a leaf or leaf-list.
type TypeInfo struct {
	base         BaseType
	typedefName  *string
	typedefChain []string
	resolved     ResolvedType
}

func (t TypeInfo) Base() BaseType { return t.base }
func (t TypeInfo) TypedefName() (string, bool) {
	if t.typedefName == nil {
		return "", false
	}
	return *t.typedefName, true
}
func (t TypeInfo) TypedefChain() []string { return append([]string(nil), t.typedefChain...) }
func (t TypeInfo) Resolved() ResolvedType {
	if t.resolved == nil {
		return ResolvedUnknown{}
	}
	return cloneResolvedType(t.resolved)
}

func cloneTypeInfo(t TypeInfo) TypeInfo {
	t.typedefChain = append([]string(nil), t.typedefChain...)
	if t.resolved != nil {
		t.resolved = cloneResolvedType(t.resolved)
	}
	return t
}

func cloneTypeInfos(in []TypeInfo) []TypeInfo {
	if len(in) == 0 {
		return nil
	}
	out := make([]TypeInfo, len(in))
	for i, info := range in {
		out[i] = cloneTypeInfo(info)
	}
	return out
}

func cloneResolvedType(resolved ResolvedType) ResolvedType {
	switch r := resolved.(type) {
	case ResolvedInt:
		r.Range = append([]RangeBound(nil), r.Range...)
		return r
	case ResolvedDecimal64:
		r.Range = append([]RangeBound(nil), r.Range...)
		return r
	case ResolvedBinary:
		r.Length = append([]RangeBound(nil), r.Length...)
		return r
	case ResolvedString:
		r.Length = append([]RangeBound(nil), r.Length...)
		r.Patterns = append([]Pattern(nil), r.Patterns...)
		return r
	case ResolvedEnumeration:
		r.def.values = append([]EnumValue(nil), r.def.values...)
		return r
	case ResolvedBits:
		r.def.values = append([]EnumValue(nil), r.def.values...)
		return r
	case ResolvedIdentityRef:
		r.bases = append([]Identity(nil), r.bases...)
		return r
	case ResolvedLeafRef:
		if r.target != nil {
			target := *r.target
			r.target = &target
		}
		if r.realtype != nil {
			realtype := *r.realtype
			r.realtype = &realtype
		}
		return r
	case ResolvedUnion:
		r.members = cloneTypeInfos(r.members)
		return r
	default:
		return resolved
	}
}

// Extension is a parsed YANG extension instance attached to a schema node.
type Extension struct {
	name       string
	argument   *string
	moduleName string
	ifFeatures []string
}

func (e Extension) Name() string { return e.name }
func (e Extension) Argument() (string, bool) {
	if e.argument == nil {
		return "", false
	}
	return *e.argument, true
}
func (e Extension) ModuleName() string { return e.moduleName }
func (e Extension) IfFeatures() []string {
	return append([]string(nil), e.ifFeatures...)
}

// ExtensionDefinition is a declared YANG extension on a module.
type ExtensionDefinition struct {
	module *moduleData
	stmt   *yangparse.Statement
}

func (e ExtensionDefinition) Name() string {
	if e.stmt == nil {
		return ""
	}
	return e.stmt.Argument
}
func (e ExtensionDefinition) Module() Module {
	if e.module == nil {
		return Module{}
	}
	return Module{mod: e.module}
}
func (e ExtensionDefinition) Argument() (string, bool) {
	arg := first(e.stmt, "argument")
	if arg == nil {
		return "", false
	}
	return arg.Argument, true
}
func (e ExtensionDefinition) YinElement() (bool, bool) {
	arg := first(e.stmt, "argument")
	if arg == nil {
		return false, false
	}
	yin := first(arg, "yin-element")
	if yin == nil {
		return false, false
	}
	return yin.Argument == "true", true
}
func (e ExtensionDefinition) Description() (string, bool) {
	return optional(childArg(e.stmt, "description"))
}
func (e ExtensionDefinition) Reference() (string, bool) {
	return optional(childArg(e.stmt, "reference"))
}
func (e ExtensionDefinition) Status() Status {
	if e.stmt == nil {
		return StatusCurrent
	}
	return statusFromStatement(e.stmt)
}

// TypedefDefinition is a declared top-level or included YANG typedef.
type TypedefDefinition struct {
	module *moduleData
	stmt   *yangparse.Statement
}

func (d TypedefDefinition) Name() string {
	if d.stmt == nil {
		return ""
	}
	return d.stmt.Argument
}
func (d TypedefDefinition) Module() Module {
	if d.module == nil {
		return Module{}
	}
	return Module{mod: d.module}
}
func (d TypedefDefinition) Type() (TypeInfo, bool) {
	if d.module == nil || d.stmt == nil {
		return TypeInfo{}, false
	}
	typ := first(d.stmt, "type")
	if typ == nil {
		return TypeInfo{}, false
	}
	info, err := d.module.parseTypeSeen(typ, make(map[*yangparse.Statement]bool))
	if err != nil {
		return TypeInfo{}, false
	}
	return info, true
}
func (d TypedefDefinition) Units() (string, bool)   { return optional(childArg(d.stmt, "units")) }
func (d TypedefDefinition) Default() (string, bool) { return optional(childArg(d.stmt, "default")) }
func (d TypedefDefinition) Description() (string, bool) {
	return optional(childArg(d.stmt, "description"))
}
func (d TypedefDefinition) Reference() (string, bool) { return optional(childArg(d.stmt, "reference")) }
func (d TypedefDefinition) Status() Status {
	if d.stmt == nil {
		return StatusCurrent
	}
	return statusFromStatement(d.stmt)
}

// GroupingDefinition is a declared top-level or included YANG grouping.
type GroupingDefinition struct {
	module *moduleData
	stmt   *yangparse.Statement
}

func (d GroupingDefinition) Name() string {
	if d.stmt == nil {
		return ""
	}
	return d.stmt.Argument
}
func (d GroupingDefinition) Module() Module {
	if d.module == nil {
		return Module{}
	}
	return Module{mod: d.module}
}
func (d GroupingDefinition) Description() (string, bool) {
	return optional(childArg(d.stmt, "description"))
}
func (d GroupingDefinition) Reference() (string, bool) {
	return optional(childArg(d.stmt, "reference"))
}
func (d GroupingDefinition) Status() Status {
	if d.stmt == nil {
		return StatusCurrent
	}
	return statusFromStatement(d.stmt)
}
func (d GroupingDefinition) ChildNames() []string {
	if d.stmt == nil {
		return nil
	}
	var out []string
	for _, child := range d.stmt.SubStatements() {
		if groupingDefinitionChildNameVisible(child.Keyword) {
			out = append(out, child.Argument)
		}
	}
	return out
}

// Feature is a declared YANG feature on a module.
type Feature struct{ feature *featureData }

func (f Feature) Name() string {
	if f.feature == nil {
		return ""
	}
	return f.feature.name
}
func (f Feature) Module() Module {
	if f.feature == nil {
		return Module{}
	}
	return Module{mod: f.feature.module}
}
func (f Feature) Description() (string, bool) {
	if f.feature == nil {
		return "", false
	}
	return optional(f.feature.description)
}
func (f Feature) Reference() (string, bool) {
	if f.feature == nil {
		return "", false
	}
	return optional(f.feature.reference)
}
func (f Feature) IfFeatures() []string {
	if f.feature == nil || f.feature.stmt == nil {
		return nil
	}
	return ifFeatureArgs(f.feature.stmt)
}
func (f Feature) Status() Status {
	if f.feature == nil || f.feature.stmt == nil {
		return StatusCurrent
	}
	return statusFromStatement(f.feature.stmt)
}

// MustConstraint is a parsed must expression plus optional metadata.
type MustConstraint struct {
	cond, errorMessage, errorAppTag, description, reference string
}

func (m MustConstraint) Expression() string           { return m.cond }
func (m MustConstraint) ErrorMessage() (string, bool) { return optional(m.errorMessage) }
func (m MustConstraint) ErrorAppTag() (string, bool)  { return optional(m.errorAppTag) }
func (m MustConstraint) Description() (string, bool)  { return optional(m.description) }
func (m MustConstraint) Reference() (string, bool)    { return optional(m.reference) }

// WhenConstraint is a parsed when expression plus optional metadata.
type WhenConstraint struct {
	cond, description, reference string
}

func (w WhenConstraint) Expression() string          { return w.cond }
func (w WhenConstraint) Description() (string, bool) { return optional(w.description) }
func (w WhenConstraint) Reference() (string, bool)   { return optional(w.reference) }

// UniqueConstraint is one unique statement on a list.
type UniqueConstraint struct{ leafs []SchemaNodeRef }

func (u UniqueConstraint) Leafs() []SchemaNodeRef { return append([]SchemaNodeRef(nil), u.leafs...) }

// Deviation is metadata for one parsed deviation property.
type Deviation struct {
	targetPath, sourceModule, devType, property, newValue, description, reference string
	ifFeatures                                                                    []string
}

func (d Deviation) TargetPath() string          { return d.targetPath }
func (d Deviation) SourceModule() string        { return d.sourceModule }
func (d Deviation) Type() string                { return d.devType }
func (d Deviation) Property() string            { return d.property }
func (d Deviation) NewValue() string            { return d.newValue }
func (d Deviation) Description() (string, bool) { return optional(d.description) }
func (d Deviation) Reference() (string, bool)   { return optional(d.reference) }
func (d Deviation) IfFeatures() []string {
	return append([]string(nil), d.ifFeatures...)
}

// Import is a value type for module import metadata.
type Import struct {
	Prefix   string
	Name     string
	Revision string

	description string
	reference   string
}

func (i Import) Description() (string, bool) { return optional(i.description) }
func (i Import) Reference() (string, bool)   { return optional(i.reference) }

// Include is a value type for module include metadata.
type Include struct {
	Name     string
	Revision string

	description string
	reference   string
}

func (i Include) Description() (string, bool) { return optional(i.description) }
func (i Include) Reference() (string, bool)   { return optional(i.reference) }

// Revision is a module revision statement plus optional metadata.
type Revision struct {
	date        string
	description string
	reference   string
}

func (r Revision) Date() string                { return r.date }
func (r Revision) Description() (string, bool) { return optional(r.description) }
func (r Revision) Reference() (string, bool)   { return optional(r.reference) }

type moduleData struct {
	ctx         *Context
	name        string
	namespace   string
	prefix      string
	revision    string
	file        string
	stmt        *yangparse.Statement
	implemented bool
	submodules  []*submoduleData

	imports          []Import
	importByPfx      map[string]*moduleData
	typedefs         map[string]*yangparse.Statement
	groupings        map[string]*yangparse.Statement
	typedefDefOrder  []*yangparse.Statement
	groupingDefOrder []*yangparse.Statement
	typedefsByScope  map[*yangparse.Statement]map[string]*yangparse.Statement
	groupingsByScope map[*yangparse.Statement]map[string]*yangparse.Statement
	statementParents map[*yangparse.Statement]*yangparse.Statement
	extDefs          map[string]*yangparse.Statement
	extDefOrder      []*yangparse.Statement
	features         []*featureData
	featureMap       map[string]*featureData

	root        *schemaNodeData
	top         []*schemaNodeData
	rpcs        []*schemaNodeData
	actions     []*schemaNodeData
	notifs      []*schemaNodeData
	nodesByPath map[string]*schemaNodeData
	schemaErr   error

	identities  []*identityData
	identityMap map[string]*identityData
	augmentedBy []string
	deviatedBy  []string
	deviations  []Deviation
}

type submoduleData struct {
	file string
	stmt *yangparse.Statement
}

type schemaNodeData struct {
	name           string
	kind           SchemaNodeKind
	module         *moduleData
	stmt           *yangparse.Statement
	parent         *schemaNodeData
	children       []*schemaNodeData
	path           string
	description    string
	reference      string
	ifFeatures     []string
	ownIfFeatures  []string
	status         Status
	config         Config
	configProp     *yangparse.Statement
	mandatory      bool
	presence       bool
	orderedBy      OrderedBy
	defaults       []string
	units          string
	minElements    *uint32
	maxElements    *uint32
	maxElementsSet bool
	typeInfo       *TypeInfo
	typeStmt       *yangparse.Statement
	typeModule     *moduleData
	listKey        bool
	keyNames       []string
	keys           []*schemaNodeData
	extensions     []Extension
	musts          []MustConstraint
	whens          []WhenConstraint
	uniques        []UniqueConstraint
	uniqueNames    [][]string
	choiceDesc     bool
	groupOrigin    string
	devs           []Deviation
}

type identityData struct {
	name      string
	module    *moduleData
	stmt      *yangparse.Statement
	baseNames []string
	bases     []*identityData
	derived   []*identityData
	resolving bool
	resolved  bool
}

type featureData struct {
	name        string
	module      *moduleData
	stmt        *yangparse.Statement
	description string
	reference   string
}

func (m *moduleData) loadMeta() {
	m.namespace = childArg(m.stmt, "namespace")
	m.prefix = childArg(m.stmt, "prefix")
	m.revision = ""
	for _, r := range direct(m.stmt, "revision") {
		if r.Argument > m.revision {
			m.revision = r.Argument
		}
	}
	m.imports = m.imports[:0]
	for _, imp := range direct(m.stmt, "import") {
		m.imports = append(m.imports, Import{
			Prefix:      childArg(imp, "prefix"),
			Name:        imp.Argument,
			Revision:    childArg(imp, "revision-date"),
			description: childArg(imp, "description"),
			reference:   childArg(imp, "reference"),
		})
	}
}

func (m *moduleData) sourceTopStatements() []*yangparse.Statement {
	if m.stmt == nil {
		return nil
	}
	subByName := make(map[string]*yangparse.Statement, len(m.submodules))
	for _, sub := range m.submodules {
		if sub.stmt != nil {
			subByName[sub.stmt.Argument] = sub.stmt
		}
	}
	seen := make(map[string]bool, len(subByName))
	var out []*yangparse.Statement
	var appendSubmodule func(name string)
	appendSubmodule = func(name string) {
		if seen[name] {
			return
		}
		sub := subByName[name]
		if sub == nil {
			return
		}
		seen[name] = true
		for _, st := range sub.SubStatements() {
			if st.Keyword == "include" {
				appendSubmodule(st.Argument)
				continue
			}
			out = append(out, st)
		}
	}
	for _, st := range m.stmt.SubStatements() {
		if st.Keyword == "include" {
			appendSubmodule(st.Argument)
			continue
		}
		out = append(out, st)
	}
	for _, sub := range m.submodules {
		if sub.stmt != nil {
			appendSubmodule(sub.stmt.Argument)
		}
	}
	return out
}

func (m *moduleData) sourceImports() []*yangparse.Statement {
	var imports []*yangparse.Statement
	for _, st := range m.sourceTopStatements() {
		if st.Keyword == "import" {
			imports = append(imports, st)
		}
	}
	return imports
}

func (m *moduleData) resetIR() {
	m.typedefs = make(map[string]*yangparse.Statement)
	m.groupings = make(map[string]*yangparse.Statement)
	m.typedefDefOrder = nil
	m.groupingDefOrder = nil
	m.typedefsByScope = make(map[*yangparse.Statement]map[string]*yangparse.Statement)
	m.groupingsByScope = make(map[*yangparse.Statement]map[string]*yangparse.Statement)
	m.statementParents = make(map[*yangparse.Statement]*yangparse.Statement)
	m.extDefs = make(map[string]*yangparse.Statement)
	m.extDefOrder = nil
	m.features = nil
	m.featureMap = make(map[string]*featureData)
	m.identityMap = make(map[string]*identityData)
	m.identities = nil
	m.root = &schemaNodeData{name: "", kind: SchemaNodeKindModule, module: m, config: ConfigRw, status: StatusCurrent}
	m.top = nil
	m.rpcs = nil
	m.actions = nil
	m.notifs = nil
	m.nodesByPath = make(map[string]*schemaNodeData)
	m.schemaErr = nil
	m.augmentedBy = nil
	m.deviatedBy = nil
	m.deviations = nil
}

func (m *moduleData) collectDefinitions() error {
	tops := m.sourceTopStatements()
	m.indexStatementParents(tops)
	for _, st := range tops {
		if err := validateTopLevelStatementPlacement(st); err != nil {
			return err
		}
	}
	for _, st := range tops {
		if err := m.validateStatementIdentifiers(st); err != nil {
			return err
		}
	}
	for _, st := range tops {
		if st.Keyword == "feature" {
			if err := m.addFeatureDefinition(st); err != nil {
				return err
			}
		}
	}
	for _, st := range tops {
		switch st.Keyword {
		case "typedef":
			if err := m.addTypedefDefinition(nil, st); err != nil {
				return err
			}
		case "grouping":
			if err := m.addGroupingDefinition(nil, st); err != nil {
				return err
			}
		case "extension":
			if err := m.addExtensionDefinition(st); err != nil {
				return err
			}
		case "identity":
			if err := m.addIdentityDefinition(st); err != nil {
				return err
			}
		}
	}
	for _, top := range tops {
		if err := m.collectScopedDefinitions(top); err != nil {
			return err
		}
	}
	return nil
}

func (m *moduleData) validateYangVersionSpecificStatements(st *yangparse.Statement) error {
	if st == nil {
		return nil
	}
	version := m.yangVersionForStatement(st)
	if version != "1.1" {
		switch st.Keyword {
		case "anydata", "action":
			return fmt.Errorf("%s %q requires yang-version 1.1 at %s", st.Keyword, st.Argument, st.Location())
		case "notification":
			if parent := m.statementParents[st]; parent != nil {
				return fmt.Errorf("notification %q requires yang-version 1.1 when nested under %s %q at %s", st.Argument, parent.Keyword, parent.Argument, st.Location())
			}
		case "if-feature":
			if m.ifFeaturePlacementRequiresYang11(st) {
				parent := m.statementParents[st]
				return fmt.Errorf("if-feature %q under %s %q requires yang-version 1.1 at %s", st.Argument, parent.Keyword, parent.Argument, st.Location())
			}
			if ifFeatureExprRequiresYang11(st.Argument) {
				return fmt.Errorf("if-feature expression %q requires yang-version 1.1 at %s", st.Argument, st.Location())
			}
		case "modifier":
			if m.patternModifierRequiresYang11(st) {
				return fmt.Errorf("pattern modifier %q requires yang-version 1.1 at %s", st.Argument, st.Location())
			}
		case "must":
			if parent := m.mustPlacementRequiresYang11(st); parent != nil {
				return fmt.Errorf("must %q under %s requires yang-version 1.1 at %s", st.Argument, statementLabel(parent), st.Location())
			}
		case "choice":
			if parent := m.choiceShorthandPlacementRequiresYang11(st); parent != nil {
				return fmt.Errorf("choice %q under %s requires yang-version 1.1 at %s", st.Argument, statementLabel(parent), st.Location())
			}
		}
	}
	for _, child := range st.SubStatements() {
		if err := m.validateYangVersionSpecificStatements(child); err != nil {
			return err
		}
	}
	return nil
}

func (m *moduleData) mustPlacementRequiresYang11(st *yangparse.Statement) *yangparse.Statement {
	if st == nil || st.Keyword != "must" {
		return nil
	}
	parent := m.statementParents[st]
	if parent == nil {
		return nil
	}
	switch parent.Keyword {
	case "input", "output", "notification":
		return parent
	default:
		return nil
	}
}

func (m *moduleData) choiceShorthandPlacementRequiresYang11(st *yangparse.Statement) *yangparse.Statement {
	if st == nil || st.Keyword != "choice" {
		return nil
	}
	parent := m.statementParents[st]
	if parent == nil || parent.Keyword != "choice" {
		return nil
	}
	return parent
}

func statementLabel(st *yangparse.Statement) string {
	if st == nil {
		return "unknown"
	}
	if st.Argument != "" {
		return fmt.Sprintf("%s %q", st.Keyword, st.Argument)
	}
	return st.Keyword
}

func (m *moduleData) ifFeaturePlacementRequiresYang11(st *yangparse.Statement) bool {
	if st == nil || st.Keyword != "if-feature" {
		return false
	}
	parent := m.statementParents[st]
	if parent == nil {
		return false
	}
	switch parent.Keyword {
	case "bit", "enum", "identity", "refine":
		return true
	default:
		return false
	}
}

func ifFeatureExprRequiresYang11(expr string) bool {
	if _, ok := singleIfFeatureRefArg(expr, false); ok {
		return false
	}
	tokens, ok := tokenizeIfFeatureExpr(expr)
	if !ok {
		return false
	}
	return len(tokens) != 1 || tokens[0].kind != ifFeatureTokenIdent
}

func (m *moduleData) patternModifierRequiresYang11(st *yangparse.Statement) bool {
	if st == nil || st.Keyword != "modifier" || st.Argument != "invert-match" {
		return false
	}
	parent := m.statementParents[st]
	if parent == nil || parent.Keyword != "pattern" {
		return false
	}
	modifiers := direct(parent, "modifier")
	return len(modifiers) == 1 && modifiers[0] == st
}

func (m *moduleData) yangVersionForStatement(st *yangparse.Statement) string {
	root := m.sourceRootForStatement(st)
	if root == nil {
		root = m.stmt
	}
	version := childArg(root, "yang-version")
	if version == "" {
		return "1"
	}
	return version
}

func (m *moduleData) sourceRootForStatement(st *yangparse.Statement) *yangparse.Statement {
	if st == nil {
		return nil
	}
	top := st
	for parent := m.statementParents[top]; parent != nil; parent = m.statementParents[top] {
		top = parent
	}
	if m.stmt != nil {
		for _, child := range m.stmt.SubStatements() {
			if child == top {
				return m.stmt
			}
		}
	}
	for _, sub := range m.submodules {
		if sub.stmt == nil {
			continue
		}
		for _, child := range sub.stmt.SubStatements() {
			if child == top {
				return sub.stmt
			}
		}
	}
	return nil
}

func (m *moduleData) definitionVisibleFrom(def, from *yangparse.Statement) bool {
	if m == nil || def == nil || from == nil {
		return true
	}
	defRoot := m.sourceRootForStatement(def)
	fromRoot := m.sourceRootForStatement(from)
	if defRoot == nil || fromRoot == nil || defRoot == fromRoot {
		return true
	}
	if fromRoot.Keyword != "submodule" {
		return true
	}
	if m.yangVersionForStatement(from) == "1.1" {
		return true
	}
	if defRoot.Keyword != "submodule" {
		return false
	}
	return m.submoduleIncludes(fromRoot, defRoot.Argument, make(map[*yangparse.Statement]bool))
}

func (m *moduleData) submoduleIncludes(source *yangparse.Statement, target string, seen map[*yangparse.Statement]bool) bool {
	if source == nil || seen[source] {
		return false
	}
	seen[source] = true
	for _, inc := range direct(source, "include") {
		if inc.Argument == target {
			return true
		}
		if m.submoduleIncludes(m.submoduleStatement(inc.Argument), target, seen) {
			return true
		}
	}
	return false
}

func (m *moduleData) submoduleStatement(name string) *yangparse.Statement {
	for _, sub := range m.submodules {
		if sub.stmt != nil && sub.stmt.Argument == name {
			return sub.stmt
		}
	}
	return nil
}

func (m *moduleData) validateStatementIdentifiers(st *yangparse.Statement) error {
	if st == nil {
		return nil
	}
	allowXMLPrefix := m != nil && m.yangVersionForStatement(st) == "1.1"
	if err := validateYangKeyword(st.Keyword, st.Location(), allowXMLPrefix); err != nil {
		return err
	}
	if err := validateKnownYangStatementKeyword(st); err != nil {
		return err
	}
	if err := validateStatementArgumentIdentifier(st, allowXMLPrefix); err != nil {
		return err
	}
	for _, child := range st.SubStatements() {
		if err := m.validateStatementIdentifiers(child); err != nil {
			return err
		}
	}
	return nil
}

func validateKnownYangStatementKeyword(st *yangparse.Statement) error {
	if st == nil || hasPrefix(st.Keyword) || knownYangStatementKeyword(st.Keyword) {
		return nil
	}
	return fmt.Errorf("unknown statement %q at %s", st.Keyword, st.Location())
}

func knownYangStatementKeyword(keyword string) bool {
	switch keyword {
	case "action", "anydata", "anyxml", "argument", "augment", "base", "belongs-to", "bit", "case", "choice", "config", "contact", "container", "default", "description", "deviation", "deviate", "enum", "error-app-tag", "error-message", "extension", "feature", "fraction-digits", "grouping", "identity", "if-feature", "import", "include", "input", "key", "leaf", "leaf-list", "length", "list", "mandatory", "max-elements", "min-elements", "modifier", "module", "must", "namespace", "notification", "ordered-by", "organization", "output", "path", "pattern", "position", "prefix", "presence", "range", "reference", "refine", "require-instance", "revision", "revision-date", "rpc", "status", "submodule", "type", "typedef", "unique", "units", "uses", "value", "when", "yang-version", "yin-element":
		return true
	default:
		return false
	}
}

func validateStatementArgumentIdentifier(st *yangparse.Statement, allowXMLPrefix bool) error {
	if st == nil {
		return nil
	}
	switch st.Keyword {
	case "action", "anydata", "anyxml", "argument", "bit", "case", "choice", "container", "enum", "extension", "feature", "grouping", "identity", "import", "include", "leaf", "leaf-list", "list", "module", "notification", "prefix", "rpc", "submodule", "typedef":
		if !st.HasArgument {
			return fmt.Errorf("%s statement requires an identifier argument at %s", st.Keyword, st.Location())
		}
		return validateYangIdentifierArg(st.Keyword, st.Argument, st, allowXMLPrefix)
	case "base", "type", "uses":
		if !st.HasArgument {
			return fmt.Errorf("%s statement requires an identifier-ref argument at %s", st.Keyword, st.Location())
		}
		return validateYangIdentifierRefArg(st.Keyword, st.Argument, st, allowXMLPrefix)
	case "augment":
		if !st.HasArgument {
			return fmt.Errorf("augment statement requires a schema-nodeid argument at %s", st.Location())
		}
	case "refine":
		if !st.HasArgument {
			return fmt.Errorf("refine statement requires a schema-nodeid argument at %s", st.Location())
		}
	case "deviate":
		if !st.HasArgument {
			return fmt.Errorf("deviate statement requires an operation argument at %s", st.Location())
		}
	case "input", "output":
		if st.HasArgument {
			return fmt.Errorf("%s statement must not have an argument at %s", st.Keyword, st.Location())
		}
	case "key":
		if !st.HasArgument {
			return fmt.Errorf("key statement requires an identifier argument at %s", st.Location())
		}
		for _, part := range strings.Fields(st.Argument) {
			if err := validateYangIdentifierArg(st.Keyword, part, st, allowXMLPrefix); err != nil {
				return err
			}
		}
	}
	if !hasPrefix(st.Keyword) && !st.HasArgument && standardStatementRequiresArgument(st.Keyword) {
		return fmt.Errorf("%s statement requires an argument at %s", st.Keyword, st.Location())
	}
	return nil
}

func standardStatementRequiresArgument(keyword string) bool {
	if !knownYangStatementKeyword(keyword) {
		return false
	}
	switch keyword {
	case "input", "output":
		return false
	default:
		return true
	}
}

func validateYangKeyword(keyword, location string, allowXMLPrefix bool) error {
	if keyword == "" {
		return fmt.Errorf("invalid empty statement keyword at %s", location)
	}
	if hasPrefix(keyword) {
		parts := strings.Split(keyword, ":")
		if len(parts) != 2 || !validYangIdentifier(parts[0], allowXMLPrefix) || !validYangIdentifier(parts[1], allowXMLPrefix) {
			return fmt.Errorf("invalid statement keyword %q at %s", keyword, location)
		}
		return nil
	}
	if !validYangIdentifier(keyword, allowXMLPrefix) {
		return fmt.Errorf("invalid statement keyword %q at %s", keyword, location)
	}
	return nil
}

func validateYangIdentifierArg(kind, value string, st *yangparse.Statement, allowXMLPrefix bool) error {
	location := "unknown"
	if st != nil {
		location = st.Location()
	}
	if !validYangIdentifier(value, allowXMLPrefix) {
		return fmt.Errorf("invalid identifier %q for %s at %s", value, kind, location)
	}
	return nil
}

func validateYangIdentifierRefArg(kind, value string, st *yangparse.Statement, allowXMLPrefix bool) error {
	location := "unknown"
	if st != nil {
		location = st.Location()
	}
	if !validYangIdentifierRef(value, allowXMLPrefix) {
		return fmt.Errorf("invalid identifier-ref %q for %s at %s", value, kind, location)
	}
	return nil
}

func validateAbsoluteSchemaNodeIDStatement(kind string, st *yangparse.Statement, allowXMLPrefix bool) error {
	if st == nil {
		return nil
	}
	if validAbsoluteSchemaNodeID(st.Argument, allowXMLPrefix) {
		return nil
	}
	return fmt.Errorf("invalid absolute schema-nodeid %q for %s at %s", st.Argument, kind, st.Location())
}

func validAbsoluteSchemaNodeID(value string, allowXMLPrefix bool) bool {
	if value == "" || value[0] != '/' || value == "/" || value[len(value)-1] == '/' {
		return false
	}
	return validDescendantSchemaNodeID(value[1:], allowXMLPrefix)
}

func validateDescendantSchemaNodeIDStatement(kind string, st *yangparse.Statement, allowXMLPrefix bool) error {
	if st == nil {
		return nil
	}
	if validDescendantSchemaNodeID(st.Argument, allowXMLPrefix) {
		return nil
	}
	return fmt.Errorf("invalid descendant schema-nodeid %q for %s at %s", st.Argument, kind, st.Location())
}

func validDescendantSchemaNodeID(value string, allowXMLPrefix bool) bool {
	if value == "" || value[0] == '/' || value[len(value)-1] == '/' {
		return false
	}
	start := 0
	for start < len(value) {
		end := strings.IndexByte(value[start:], '/')
		var step string
		if end < 0 {
			step = value[start:]
			start = len(value)
		} else {
			end += start
			step = value[start:end]
			start = end + 1
		}
		if step == "" || strings.TrimSpace(step) != step {
			return false
		}
		if !validYangIdentifierRef(step, allowXMLPrefix) {
			return false
		}
	}
	return true
}

func validYangIdentifierRef(value string, allowXMLPrefix bool) bool {
	if !hasPrefix(value) {
		return validYangIdentifier(value, allowXMLPrefix)
	}
	parts := strings.Split(value, ":")
	return len(parts) == 2 && validYangIdentifier(parts[0], allowXMLPrefix) && validYangIdentifier(parts[1], allowXMLPrefix)
}

func validYangIdentifier(value string, allowXMLPrefix bool) bool {
	if value == "" {
		return false
	}
	if !allowXMLPrefix && len(value) >= 3 && strings.EqualFold(value[:3], "xml") {
		return false
	}
	for i := 0; i < len(value); i++ {
		ch := value[i]
		if ch >= utf8.RuneSelf {
			return false
		}
		if i == 0 {
			if !isYangIdentifierStart(ch) {
				return false
			}
			continue
		}
		if !isYangIdentifierChar(ch) {
			return false
		}
	}
	return true
}

func isYangIdentifierStart(ch byte) bool {
	return ch == '_' || ch >= 'A' && ch <= 'Z' || ch >= 'a' && ch <= 'z'
}

func isYangIdentifierChar(ch byte) bool {
	return isYangIdentifierStart(ch) || ch >= '0' && ch <= '9' || ch == '-' || ch == '.'
}

func validateTopLevelStatementPlacement(st *yangparse.Statement) error {
	if st == nil || strings.Contains(st.Keyword, ":") || topLevelStatementAllowed(st.Keyword) {
		return nil
	}
	return fmt.Errorf("%s at %s is not valid at module or submodule top level", st.Keyword, st.Location())
}

func validateTopLevelStatementOrder(root *yangparse.Statement) error {
	if root == nil || root.Keyword != "module" && root.Keyword != "submodule" {
		return nil
	}
	phase := topLevelOrderHeader
	for _, st := range root.SubStatements() {
		next, ok := topLevelOrderPhase(st.Keyword)
		if !ok {
			continue
		}
		if next < phase {
			return fmt.Errorf("%s %q is out of order in %s %q at %s", st.Keyword, st.Argument, root.Keyword, root.Argument, st.Location())
		}
		if next > phase {
			phase = next
		}
	}
	return nil
}

type topLevelOrderPhaseValue int

const (
	topLevelOrderHeader topLevelOrderPhaseValue = iota
	topLevelOrderLinkage
	topLevelOrderMeta
	topLevelOrderRevision
	topLevelOrderBody
)

func topLevelOrderPhase(keyword string) (topLevelOrderPhaseValue, bool) {
	if hasPrefix(keyword) {
		return topLevelOrderBody, true
	}
	switch keyword {
	case "yang-version", "namespace", "prefix", "belongs-to":
		return topLevelOrderHeader, true
	case "import", "include":
		return topLevelOrderLinkage, true
	case "organization", "contact", "description", "reference":
		return topLevelOrderMeta, true
	case "revision":
		return topLevelOrderRevision, true
	case "anydata", "anyxml", "augment", "choice", "container", "deviation", "extension", "feature", "grouping", "identity", "leaf", "leaf-list", "list", "notification", "rpc", "typedef", "uses":
		return topLevelOrderBody, true
	default:
		return topLevelOrderHeader, false
	}
}

func topLevelStatementAllowed(keyword string) bool {
	switch keyword {
	case "anydata", "anyxml", "augment", "belongs-to", "choice", "contact", "container", "description", "deviation", "extension", "feature", "grouping", "identity", "import", "include", "leaf", "leaf-list", "list", "namespace", "notification", "organization", "prefix", "reference", "revision", "rpc", "typedef", "uses", "yang-version":
		return true
	default:
		return false
	}
}

func (m *moduleData) indexStatementParents(tops []*yangparse.Statement) {
	var walk func(st, parent *yangparse.Statement)
	walk = func(st, parent *yangparse.Statement) {
		if st == nil {
			return
		}
		if parent != nil {
			m.statementParents[st] = parent
		}
		for _, child := range st.SubStatements() {
			walk(child, st)
		}
	}
	for _, top := range tops {
		walk(top, nil)
	}
}

func (m *moduleData) collectScopedDefinitions(st *yangparse.Statement) error {
	if st == nil {
		return nil
	}
	for _, child := range st.SubStatements() {
		if err := validateNestedRootStatementPlacement(st, child); err != nil {
			return err
		}
		if err := validateNestedTypeStatementPlacement(st, child); err != nil {
			return err
		}
		if err := validateNestedModuleBodyStatementPlacement(st, child); err != nil {
			return err
		}
		if err := validateNestedTopLevelDefinitionPlacement(st, child); err != nil {
			return err
		}
		if err := validateNestedTopLevelDefinitionStatementChildPlacement(st, child); err != nil {
			return err
		}
		if err := validateNestedTypedefStatementChildPlacement(st, child); err != nil {
			return err
		}
		if err := validateNestedGroupingStatementChildPlacement(st, child); err != nil {
			return err
		}
		if err := validateNestedAugmentPlacement(st, child); err != nil {
			return err
		}
		if err := validateNestedAugmentStatementChildPlacement(st, child); err != nil {
			return err
		}
		if err := validateNestedRefinePlacement(st, child); err != nil {
			return err
		}
		if err := validateNestedRefineStatementChildPlacement(st, child); err != nil {
			return err
		}
		if err := validateNestedUsesStatementChildPlacement(st, child); err != nil {
			return err
		}
		if err := validateNestedChoiceStatementChildPlacement(st, child); err != nil {
			return err
		}
		if err := validateNestedCaseStatementChildPlacement(st, child); err != nil {
			return err
		}
		if err := validateNestedOperationStatementChildPlacement(st, child); err != nil {
			return err
		}
		if err := validateNestedOperationIOStatementChildPlacement(st, child); err != nil {
			return err
		}
		if err := validateNestedNotificationStatementChildPlacement(st, child); err != nil {
			return err
		}
		if err := validateNestedDeviatePlacement(st, child); err != nil {
			return err
		}
		if err := validateNestedDeviateStatementChildPlacement(st, child); err != nil {
			return err
		}
		if err := validateNestedDeviationStatementChildPlacement(st, child); err != nil {
			return err
		}
		if err := validateNestedRevisionDatePlacement(st, child); err != nil {
			return err
		}
		if err := validateNestedPrefixPlacement(st, child); err != nil {
			return err
		}
		if err := validateNestedDependencyStatementChildPlacement(st, child); err != nil {
			return err
		}
		if err := validateNestedRevisionStatementChildPlacement(st, child); err != nil {
			return err
		}
		if err := validateNestedBelongsToStatementChildPlacement(st, child); err != nil {
			return err
		}
		if err := validateNestedYangVersionStatementChildPlacement(st, child); err != nil {
			return err
		}
		if err := validateNestedNoStandardChildrenStatementChildPlacement(st, child); err != nil {
			return err
		}
		if err := validateNestedArgumentPlacement(st, child); err != nil {
			return err
		}
		if err := validateNestedArgumentStatementChildPlacement(st, child); err != nil {
			return err
		}
		if err := validateNestedYinElementPlacement(st, child); err != nil {
			return err
		}
		if err := validateNestedEnumBitValuePlacement(st, child); err != nil {
			return err
		}
		if err := validateNestedEnumBitStatementChildPlacement(st, child); err != nil {
			return err
		}
		if err := validateNestedTypeBodyStatementPlacement(st, child); err != nil {
			return err
		}
		if err := validateNestedTypeStatementChildPlacement(st, child); err != nil {
			return err
		}
		if err := validateNestedPatternStatementChildPlacement(st, child); err != nil {
			return err
		}
		if err := validateNestedRangeLengthStatementChildPlacement(st, child); err != nil {
			return err
		}
		if err := validateNestedMustWhenStatementChildPlacement(st, child); err != nil {
			return err
		}
		if err := validateNestedConstraintMetadataPlacement(st, child); err != nil {
			return err
		}
		if err := validateNestedModifierPlacement(st, child); err != nil {
			return err
		}
		switch child.Keyword {
		case "typedef":
			if err := validateScopedDefinitionPlacement(st, child); err != nil {
				return err
			}
			if err := m.addTypedefDefinition(st, child); err != nil {
				return err
			}
		case "grouping":
			if err := validateScopedDefinitionPlacement(st, child); err != nil {
				return err
			}
			if err := m.addGroupingDefinition(st, child); err != nil {
				return err
			}
		}
		if err := m.collectScopedDefinitions(child); err != nil {
			return err
		}
	}
	return nil
}

func validateNestedRootStatementPlacement(scope, st *yangparse.Statement) error {
	if scope == nil || st == nil {
		return nil
	}
	switch st.Keyword {
	case "module", "submodule":
	default:
		return nil
	}
	return fmt.Errorf("%s %q is not valid under %s %q at %s", st.Keyword, st.Argument, scope.Keyword, scope.Argument, st.Location())
}

func validateNestedTypeStatementPlacement(scope, st *yangparse.Statement) error {
	if scope == nil || st == nil || st.Keyword != "type" {
		return nil
	}
	if hasPrefix(scope.Keyword) {
		return nil
	}
	switch scope.Keyword {
	case "deviate", "type", "typedef":
		return nil
	}
	if kindForKeyword(scope.Keyword) != SchemaNodeKindUnknown {
		return nil
	}
	return fmt.Errorf("type %q is not valid under %s %q at %s", st.Argument, scope.Keyword, scope.Argument, st.Location())
}

func validateNestedAugmentPlacement(scope, st *yangparse.Statement) error {
	if scope == nil || st == nil || st.Keyword != "augment" || scope.Keyword == "uses" {
		return nil
	}
	return fmt.Errorf("augment %q is not valid under %s %q at %s", st.Argument, scope.Keyword, scope.Argument, st.Location())
}

func validateNestedAugmentStatementChildPlacement(scope, st *yangparse.Statement) error {
	if scope == nil || st == nil || scope.Keyword != "augment" || hasPrefix(st.Keyword) {
		return nil
	}
	if augmentStatementChildAllowed(st.Keyword) {
		return nil
	}
	return fmt.Errorf("%s %q is not valid under augment %q at %s", st.Keyword, st.Argument, scope.Argument, st.Location())
}

func augmentStatementChildAllowed(keyword string) bool {
	switch keyword {
	case "action", "anydata", "anyxml", "case", "choice", "container", "description", "if-feature", "leaf", "leaf-list", "list", "notification", "reference", "status", "uses", "when":
		return true
	default:
		return false
	}
}

func validateNestedRefinePlacement(scope, st *yangparse.Statement) error {
	if scope == nil || st == nil || st.Keyword != "refine" || scope.Keyword == "uses" {
		return nil
	}
	return fmt.Errorf("refine %q is not valid under %s %q at %s", st.Argument, scope.Keyword, scope.Argument, st.Location())
}

func validateNestedRefineStatementChildPlacement(scope, st *yangparse.Statement) error {
	if scope == nil || st == nil || scope.Keyword != "refine" || hasPrefix(st.Keyword) {
		return nil
	}
	if refineStatementChildAllowed(st.Keyword) {
		return nil
	}
	return fmt.Errorf("%s %q is not valid under refine %q at %s", st.Keyword, st.Argument, scope.Argument, st.Location())
}

func refineStatementChildAllowed(keyword string) bool {
	switch keyword {
	case "config", "default", "description", "if-feature", "mandatory", "max-elements", "min-elements", "must", "presence", "reference":
		return true
	default:
		return false
	}
}

func validateNestedUsesStatementChildPlacement(scope, st *yangparse.Statement) error {
	if scope == nil || st == nil || scope.Keyword != "uses" || hasPrefix(st.Keyword) {
		return nil
	}
	if usesStatementChildAllowed(st.Keyword) {
		return nil
	}
	return fmt.Errorf("%s %q is not valid under uses %q at %s", st.Keyword, st.Argument, scope.Argument, st.Location())
}

func usesStatementChildAllowed(keyword string) bool {
	switch keyword {
	case "augment", "description", "if-feature", "reference", "refine", "status", "when":
		return true
	default:
		return false
	}
}

func validateNestedChoiceStatementChildPlacement(scope, st *yangparse.Statement) error {
	if scope == nil || st == nil || scope.Keyword != "choice" || hasPrefix(st.Keyword) {
		return nil
	}
	if choiceStatementChildAllowed(st.Keyword) {
		return nil
	}
	return fmt.Errorf("%s %q is not valid under choice %q at %s", st.Keyword, st.Argument, scope.Argument, st.Location())
}

func choiceStatementChildAllowed(keyword string) bool {
	switch keyword {
	case "anydata", "anyxml", "case", "choice", "config", "container", "default", "description", "if-feature", "leaf", "leaf-list", "list", "mandatory", "reference", "status", "uses", "when":
		return true
	default:
		return false
	}
}

func validateNestedCaseStatementChildPlacement(scope, st *yangparse.Statement) error {
	if scope == nil || st == nil || scope.Keyword != "case" || hasPrefix(st.Keyword) {
		return nil
	}
	if caseStatementChildAllowed(st.Keyword) {
		return nil
	}
	return fmt.Errorf("%s %q is not valid under case %q at %s", st.Keyword, st.Argument, scope.Argument, st.Location())
}

func caseStatementChildAllowed(keyword string) bool {
	switch keyword {
	case "anydata", "anyxml", "choice", "container", "description", "if-feature", "leaf", "leaf-list", "list", "reference", "status", "uses", "when":
		return true
	default:
		return false
	}
}

func validateNestedOperationStatementChildPlacement(scope, st *yangparse.Statement) error {
	if scope == nil || st == nil || scope.Keyword != "rpc" && scope.Keyword != "action" || hasPrefix(st.Keyword) {
		return nil
	}
	if operationStatementChildAllowed(st.Keyword) {
		return nil
	}
	return fmt.Errorf("%s %q is not valid under %s %q at %s", st.Keyword, st.Argument, scope.Keyword, scope.Argument, st.Location())
}

func operationStatementChildAllowed(keyword string) bool {
	switch keyword {
	case "description", "grouping", "if-feature", "input", "output", "reference", "status", "typedef":
		return true
	default:
		return false
	}
}

func validateNestedDeviatePlacement(scope, st *yangparse.Statement) error {
	if scope == nil || st == nil || st.Keyword != "deviate" || scope.Keyword == "deviation" {
		return nil
	}
	return fmt.Errorf("deviate %q is not valid under %s %q at %s", st.Argument, scope.Keyword, scope.Argument, st.Location())
}

func validateNestedDeviationStatementChildPlacement(scope, st *yangparse.Statement) error {
	if scope == nil || st == nil || scope.Keyword != "deviation" || hasPrefix(st.Keyword) {
		return nil
	}
	switch st.Keyword {
	case "description", "deviate", "if-feature", "reference":
		return nil
	default:
		return fmt.Errorf("%s %q is not valid under deviation %q at %s", st.Keyword, st.Argument, scope.Argument, st.Location())
	}
}

func validateNestedDeviateStatementChildPlacement(scope, st *yangparse.Statement) error {
	if scope == nil || st == nil || scope.Keyword != "deviate" || hasPrefix(st.Keyword) {
		return nil
	}
	if !validDeviationType(scope.Argument) {
		return nil
	}
	if deviateStatementChildAllowed(scope.Argument, st.Keyword) {
		return nil
	}
	return fmt.Errorf("%s %q is not valid under deviate %q at %s", st.Keyword, st.Argument, scope.Argument, st.Location())
}

func deviateStatementChildAllowed(operation, keyword string) bool {
	switch operation {
	case "not-supported":
		return false
	case "add":
		switch keyword {
		case "config", "default", "mandatory", "max-elements", "min-elements", "must", "unique", "units":
			return true
		}
	case "replace":
		switch keyword {
		case "config", "default", "mandatory", "max-elements", "min-elements", "must", "type", "unique", "units":
			return true
		}
	case "delete":
		switch keyword {
		case "default", "max-elements", "min-elements", "must", "unique", "units":
			return true
		}
	}
	return false
}

func validateNestedRevisionDatePlacement(scope, st *yangparse.Statement) error {
	if scope == nil || st == nil || st.Keyword != "revision-date" || scope.Keyword == "import" || scope.Keyword == "include" {
		return nil
	}
	return fmt.Errorf("revision-date %q is not valid under %s %q at %s", st.Argument, scope.Keyword, scope.Argument, st.Location())
}

func validateNestedPrefixPlacement(scope, st *yangparse.Statement) error {
	if scope == nil || st == nil || st.Keyword != "prefix" || scope.Keyword == "import" || scope.Keyword == "belongs-to" {
		return nil
	}
	return fmt.Errorf("prefix %q is not valid under %s %q at %s", st.Argument, scope.Keyword, scope.Argument, st.Location())
}

func validateNestedDependencyStatementChildPlacement(scope, st *yangparse.Statement) error {
	if scope == nil || st == nil || hasPrefix(st.Keyword) {
		return nil
	}
	switch scope.Keyword {
	case "import":
		if importStatementChildAllowed(st.Keyword) {
			return nil
		}
	case "include":
		if includeStatementChildAllowed(st.Keyword) {
			return nil
		}
	default:
		return nil
	}
	return fmt.Errorf("%s %q is not valid under %s %q at %s", st.Keyword, st.Argument, scope.Keyword, scope.Argument, st.Location())
}

func importStatementChildAllowed(keyword string) bool {
	switch keyword {
	case "description", "prefix", "reference", "revision-date":
		return true
	default:
		return false
	}
}

func includeStatementChildAllowed(keyword string) bool {
	switch keyword {
	case "description", "reference", "revision-date":
		return true
	default:
		return false
	}
}

func validateNestedRevisionStatementChildPlacement(scope, st *yangparse.Statement) error {
	if scope == nil || st == nil || scope.Keyword != "revision" || hasPrefix(st.Keyword) {
		return nil
	}
	if revisionStatementChildAllowed(st.Keyword) {
		return nil
	}
	return fmt.Errorf("%s %q is not valid under revision %q at %s", st.Keyword, st.Argument, scope.Argument, st.Location())
}

func revisionStatementChildAllowed(keyword string) bool {
	switch keyword {
	case "description", "reference":
		return true
	default:
		return false
	}
}

func validateNestedBelongsToStatementChildPlacement(scope, st *yangparse.Statement) error {
	if scope == nil || st == nil || scope.Keyword != "belongs-to" || hasPrefix(st.Keyword) {
		return nil
	}
	if belongsToStatementChildAllowed(st.Keyword) {
		return nil
	}
	return fmt.Errorf("%s %q is not valid under belongs-to %q at %s", st.Keyword, st.Argument, scope.Argument, st.Location())
}

func belongsToStatementChildAllowed(keyword string) bool {
	return keyword == "prefix"
}

func validateNestedYangVersionStatementChildPlacement(scope, st *yangparse.Statement) error {
	if scope == nil || st == nil || scope.Keyword != "yang-version" || hasPrefix(st.Keyword) {
		return nil
	}
	return fmt.Errorf("%s %q is not valid under yang-version %q at %s", st.Keyword, st.Argument, scope.Argument, st.Location())
}

func validateNestedNoStandardChildrenStatementChildPlacement(scope, st *yangparse.Statement) error {
	if scope == nil || st == nil || hasPrefix(st.Keyword) {
		return nil
	}
	if !statementHasNoStandardChildren(scope.Keyword) {
		return nil
	}
	return fmt.Errorf("%s %q is not valid under %s %q at %s", st.Keyword, st.Argument, scope.Keyword, scope.Argument, st.Location())
}

func statementHasNoStandardChildren(keyword string) bool {
	switch keyword {
	case "base", "config", "contact", "default", "description", "error-app-tag", "error-message", "fraction-digits", "if-feature", "key", "mandatory", "max-elements", "min-elements", "modifier", "namespace", "ordered-by", "organization", "path", "position", "prefix", "presence", "reference", "require-instance", "revision-date", "status", "unique", "units", "value", "yin-element":
		return true
	default:
		return false
	}
}

func validateNestedArgumentPlacement(scope, st *yangparse.Statement) error {
	if scope == nil || st == nil || st.Keyword != "argument" || scope.Keyword == "extension" {
		return nil
	}
	return fmt.Errorf("argument %q is not valid under %s %q at %s", st.Argument, scope.Keyword, scope.Argument, st.Location())
}

func validateNestedArgumentStatementChildPlacement(scope, st *yangparse.Statement) error {
	if scope == nil || st == nil || scope.Keyword != "argument" || hasPrefix(st.Keyword) {
		return nil
	}
	if st.Keyword == "yin-element" {
		return nil
	}
	return fmt.Errorf("%s %q is not valid under argument %q at %s", st.Keyword, st.Argument, scope.Argument, st.Location())
}

func validateNestedYinElementPlacement(scope, st *yangparse.Statement) error {
	if scope == nil || st == nil || st.Keyword != "yin-element" || scope.Keyword == "argument" {
		return nil
	}
	return fmt.Errorf("yin-element %q is not valid under %s %q at %s", st.Argument, scope.Keyword, scope.Argument, st.Location())
}

func validateNestedEnumBitValuePlacement(scope, st *yangparse.Statement) error {
	if scope == nil || st == nil {
		return nil
	}
	switch st.Keyword {
	case "value":
		if scope.Keyword == "enum" {
			return nil
		}
	case "position":
		if scope.Keyword == "bit" {
			return nil
		}
	default:
		return nil
	}
	return fmt.Errorf("%s %q is not valid under %s %q at %s", st.Keyword, st.Argument, scope.Keyword, scope.Argument, st.Location())
}

func validateNestedEnumBitStatementChildPlacement(scope, st *yangparse.Statement) error {
	if scope == nil || st == nil || hasPrefix(st.Keyword) {
		return nil
	}
	switch scope.Keyword {
	case "enum":
		if enumStatementChildAllowed(st.Keyword) {
			return nil
		}
	case "bit":
		if bitStatementChildAllowed(st.Keyword) {
			return nil
		}
	default:
		return nil
	}
	return fmt.Errorf("%s %q is not valid under %s %q at %s", st.Keyword, st.Argument, scope.Keyword, scope.Argument, st.Location())
}

func enumStatementChildAllowed(keyword string) bool {
	switch keyword {
	case "description", "if-feature", "reference", "status", "value":
		return true
	default:
		return false
	}
}

func bitStatementChildAllowed(keyword string) bool {
	switch keyword {
	case "description", "if-feature", "position", "reference", "status":
		return true
	default:
		return false
	}
}

func validateNestedTypeBodyStatementPlacement(scope, st *yangparse.Statement) error {
	if scope == nil || st == nil {
		return nil
	}
	switch st.Keyword {
	case "base":
		if scope.Keyword == "identity" || scope.Keyword == "type" {
			return nil
		}
	case "bit", "enum", "fraction-digits", "length", "path", "pattern", "range", "require-instance":
		if scope.Keyword == "type" {
			return nil
		}
	default:
		return nil
	}
	return fmt.Errorf("%s %q is not valid under %s %q at %s", st.Keyword, st.Argument, scope.Keyword, scope.Argument, st.Location())
}

func validateNestedTypeStatementChildPlacement(scope, st *yangparse.Statement) error {
	if scope == nil || st == nil || scope.Keyword != "type" || hasPrefix(st.Keyword) {
		return nil
	}
	if typeStatementChildAllowed(st.Keyword) {
		return nil
	}
	return fmt.Errorf("%s %q is not valid under type %q at %s", st.Keyword, st.Argument, scope.Argument, st.Location())
}

func typeStatementChildAllowed(keyword string) bool {
	switch keyword {
	case "base", "bit", "enum", "fraction-digits", "length", "path", "pattern", "range", "require-instance", "type":
		return true
	default:
		return false
	}
}

func validateNestedPatternStatementChildPlacement(scope, st *yangparse.Statement) error {
	if scope == nil || st == nil || scope.Keyword != "pattern" || hasPrefix(st.Keyword) {
		return nil
	}
	if patternStatementChildAllowed(st.Keyword) {
		return nil
	}
	return fmt.Errorf("%s %q is not valid under pattern %q at %s", st.Keyword, st.Argument, scope.Argument, st.Location())
}

func patternStatementChildAllowed(keyword string) bool {
	switch keyword {
	case "description", "error-app-tag", "error-message", "modifier", "reference":
		return true
	default:
		return false
	}
}

func validateNestedRangeLengthStatementChildPlacement(scope, st *yangparse.Statement) error {
	if scope == nil || st == nil || hasPrefix(st.Keyword) {
		return nil
	}
	switch scope.Keyword {
	case "range", "length":
		if restrictionMetadataChildAllowed(st.Keyword) {
			return nil
		}
	default:
		return nil
	}
	return fmt.Errorf("%s %q is not valid under %s %q at %s", st.Keyword, st.Argument, scope.Keyword, scope.Argument, st.Location())
}

func restrictionMetadataChildAllowed(keyword string) bool {
	switch keyword {
	case "description", "error-app-tag", "error-message", "reference":
		return true
	default:
		return false
	}
}

func validateNestedMustWhenStatementChildPlacement(scope, st *yangparse.Statement) error {
	if scope == nil || st == nil || hasPrefix(st.Keyword) {
		return nil
	}
	switch scope.Keyword {
	case "must":
		if mustStatementChildAllowed(st.Keyword) {
			return nil
		}
	case "when":
		if whenStatementChildAllowed(st.Keyword) {
			return nil
		}
	default:
		return nil
	}
	return fmt.Errorf("%s %q is not valid under %s %q at %s", st.Keyword, st.Argument, scope.Keyword, scope.Argument, st.Location())
}

func mustStatementChildAllowed(keyword string) bool {
	switch keyword {
	case "description", "error-app-tag", "error-message", "reference":
		return true
	default:
		return false
	}
}

func whenStatementChildAllowed(keyword string) bool {
	switch keyword {
	case "description", "reference":
		return true
	default:
		return false
	}
}

func validateNestedConstraintMetadataPlacement(scope, st *yangparse.Statement) error {
	if scope == nil || st == nil {
		return nil
	}
	switch st.Keyword {
	case "error-message", "error-app-tag":
		if constraintMetadataScopeAllowed(scope.Keyword) {
			return nil
		}
	default:
		return nil
	}
	return fmt.Errorf("%s %q is not valid under %s %q at %s", st.Keyword, st.Argument, scope.Keyword, scope.Argument, st.Location())
}

func constraintMetadataScopeAllowed(keyword string) bool {
	switch keyword {
	case "length", "must", "pattern", "range":
		return true
	default:
		return false
	}
}

func validateNestedModifierPlacement(scope, st *yangparse.Statement) error {
	if scope == nil || st == nil || st.Keyword != "modifier" || scope.Keyword == "pattern" {
		return nil
	}
	return fmt.Errorf("modifier %q is not valid under %s %q at %s", st.Argument, scope.Keyword, scope.Argument, st.Location())
}

func validateNestedModuleBodyStatementPlacement(scope, st *yangparse.Statement) error {
	if scope == nil || st == nil || !moduleBodyOnlyKeyword(st.Keyword) {
		return nil
	}
	return fmt.Errorf("%s %q is not valid under %s %q at %s", st.Keyword, st.Argument, scope.Keyword, scope.Argument, st.Location())
}

func moduleBodyOnlyKeyword(keyword string) bool {
	switch keyword {
	case "belongs-to", "contact", "deviation", "import", "include", "namespace", "organization", "revision", "yang-version":
		return true
	default:
		return false
	}
}

func validateNestedTopLevelDefinitionPlacement(scope, st *yangparse.Statement) error {
	if scope == nil || st == nil || !topLevelDefinitionKeyword(st.Keyword) {
		return nil
	}
	return fmt.Errorf("%s %q is not valid under %s %q at %s", st.Keyword, st.Argument, scope.Keyword, scope.Argument, st.Location())
}

func topLevelDefinitionKeyword(keyword string) bool {
	switch keyword {
	case "identity", "feature", "extension":
		return true
	default:
		return false
	}
}

func validateNestedTopLevelDefinitionStatementChildPlacement(scope, st *yangparse.Statement) error {
	if scope == nil || st == nil || !topLevelDefinitionKeyword(scope.Keyword) || hasPrefix(st.Keyword) {
		return nil
	}
	if topLevelDefinitionStatementChildAllowed(scope.Keyword, st.Keyword) {
		return nil
	}
	return fmt.Errorf("%s %q is not valid under %s %q at %s", st.Keyword, st.Argument, scope.Keyword, scope.Argument, st.Location())
}

func topLevelDefinitionStatementChildAllowed(scopeKeyword, childKeyword string) bool {
	switch scopeKeyword {
	case "identity":
		switch childKeyword {
		case "base", "description", "if-feature", "reference", "status":
			return true
		}
	case "feature":
		switch childKeyword {
		case "description", "if-feature", "reference", "status":
			return true
		}
	case "extension":
		switch childKeyword {
		case "argument", "description", "reference", "status":
			return true
		}
	}
	return false
}

func validateNestedTypedefStatementChildPlacement(scope, st *yangparse.Statement) error {
	if scope == nil || st == nil || scope.Keyword != "typedef" || hasPrefix(st.Keyword) {
		return nil
	}
	if typedefStatementChildAllowed(st.Keyword) {
		return nil
	}
	return fmt.Errorf("%s %q is not valid under typedef %q at %s", st.Keyword, st.Argument, scope.Argument, st.Location())
}

func typedefStatementChildAllowed(keyword string) bool {
	switch keyword {
	case "default", "description", "reference", "status", "type", "units":
		return true
	default:
		return false
	}
}

func validateNestedGroupingStatementChildPlacement(scope, st *yangparse.Statement) error {
	if scope == nil || st == nil || scope.Keyword != "grouping" || hasPrefix(st.Keyword) {
		return nil
	}
	if groupingStatementChildAllowed(st.Keyword) {
		return nil
	}
	return fmt.Errorf("%s %q is not valid under grouping %q at %s", st.Keyword, st.Argument, scope.Argument, st.Location())
}

func groupingStatementChildAllowed(keyword string) bool {
	switch keyword {
	case "action", "anydata", "anyxml", "choice", "container", "description", "grouping", "leaf", "leaf-list", "list", "notification", "reference", "status", "typedef", "uses":
		return true
	default:
		return false
	}
}

func groupingDefinitionChildNameVisible(keyword string) bool {
	switch keyword {
	case "action", "anydata", "anyxml", "choice", "container", "leaf", "leaf-list", "list", "notification":
		return true
	default:
		return false
	}
}

func validateNestedOperationIOStatementChildPlacement(scope, st *yangparse.Statement) error {
	if scope == nil || st == nil || scope.Keyword != "input" && scope.Keyword != "output" || hasPrefix(st.Keyword) {
		return nil
	}
	if operationIOStatementChildAllowed(st.Keyword) {
		return nil
	}
	return fmt.Errorf("%s %q is not valid under %s at %s", st.Keyword, st.Argument, scope.Keyword, st.Location())
}

func operationIOStatementChildAllowed(keyword string) bool {
	switch keyword {
	case "anydata", "anyxml", "choice", "container", "grouping", "leaf", "leaf-list", "list", "must", "typedef", "uses":
		return true
	default:
		return false
	}
}

func validateNestedNotificationStatementChildPlacement(scope, st *yangparse.Statement) error {
	if scope == nil || st == nil || scope.Keyword != "notification" || hasPrefix(st.Keyword) {
		return nil
	}
	if notificationStatementChildAllowed(st.Keyword) {
		return nil
	}
	return fmt.Errorf("%s %q is not valid under notification %q at %s", st.Keyword, st.Argument, scope.Argument, st.Location())
}

func notificationStatementChildAllowed(keyword string) bool {
	switch keyword {
	case "anydata", "anyxml", "choice", "container", "description", "grouping", "if-feature", "leaf", "leaf-list", "list", "must", "reference", "status", "typedef", "uses":
		return true
	default:
		return false
	}
}

func validateScopedDefinitionPlacement(scope, st *yangparse.Statement) error {
	if scope == nil || st == nil {
		return nil
	}
	if scopedDefinitionScopeAllowed(scope.Keyword) {
		return nil
	}
	return fmt.Errorf("%s %q is not valid under %s %q at %s", st.Keyword, st.Argument, scope.Keyword, scope.Argument, st.Location())
}

func scopedDefinitionScopeAllowed(keyword string) bool {
	switch keyword {
	case "module", "submodule", "grouping", "container", "list", "rpc", "action", "input", "output", "notification":
		return true
	default:
		return false
	}
}

func (m *moduleData) addExtensionDefinition(st *yangparse.Statement) error {
	name := st.Argument
	if prev := m.extDefs[name]; prev != nil {
		return duplicateDefinitionError("extension", name, prev, st)
	}
	if err := validateDefinitionStatus("extension", name, st); err != nil {
		return err
	}
	if err := validateDefinitionTextMetadata("extension", name, st); err != nil {
		return err
	}
	args := direct(st, "argument")
	if len(args) > 1 {
		return fmt.Errorf("extension %q has multiple argument statements at %s", name, args[1].Location())
	}
	if len(args) == 1 {
		yins := direct(args[0], "yin-element")
		if len(yins) > 1 {
			return fmt.Errorf("argument %q has multiple yin-element statements at %s", args[0].Argument, yins[1].Location())
		}
		if len(yins) == 1 {
			switch yins[0].Argument {
			case "true", "false":
			default:
				return fmt.Errorf("invalid yin-element %q at %s", yins[0].Argument, yins[0].Location())
			}
		}
	}
	m.extDefs[name] = st
	m.extDefOrder = append(m.extDefOrder, st)
	return nil
}

func (m *moduleData) addTypedefDefinition(scope, st *yangparse.Statement) error {
	name := st.Argument
	if builtinBase(name) != BaseTypeUnknown {
		return fmt.Errorf("typedef %q at %s collides with built-in type", name, st.Location())
	}
	if err := validateTypedefTypeCardinality(name, st); err != nil {
		return err
	}
	if err := validateTypedefDefaultCardinality(name, st); err != nil {
		return err
	}
	if err := validateDefinitionStatus("typedef", name, st); err != nil {
		return err
	}
	if err := validateDefinitionTextMetadata("typedef", name, st); err != nil {
		return err
	}
	if scope == nil {
		if prev := m.typedefs[name]; prev != nil {
			return duplicateDefinitionError("typedef", name, prev, st)
		}
		m.typedefs[name] = st
		m.typedefDefOrder = append(m.typedefDefOrder, st)
		return nil
	}
	if prev := m.typedefs[name]; prev != nil {
		return definitionCollisionError("typedef", name, "collides with top-level typedef", prev, st)
	}
	for parent := m.statementParents[scope]; parent != nil; parent = m.statementParents[parent] {
		if prev := m.typedefsByScope[parent][name]; prev != nil {
			return definitionCollisionError("typedef", name, "collides with ancestor scoped typedef", prev, st)
		}
	}
	defs := m.typedefsByScope[scope]
	if defs == nil {
		defs = make(map[string]*yangparse.Statement)
		m.typedefsByScope[scope] = defs
	}
	if prev := defs[name]; prev != nil {
		return duplicateDefinitionError("typedef", name, prev, st)
	}
	defs[name] = st
	return nil
}

func validateTypedefTypeCardinality(name string, st *yangparse.Statement) error {
	types := direct(st, "type")
	if len(types) == 0 {
		return fmt.Errorf("typedef %q has no type at %s", name, st.Location())
	}
	if len(types) > 1 {
		return fmt.Errorf("duplicate type in typedef %q at %s", name, types[1].Location())
	}
	return nil
}

func validateTypedefDefaultCardinality(name string, st *yangparse.Statement) error {
	defaults := direct(st, "default")
	if len(defaults) > 1 {
		return fmt.Errorf("typedef %q has multiple default statements at %s", name, defaults[1].Location())
	}
	return nil
}

func (m *moduleData) addGroupingDefinition(scope, st *yangparse.Statement) error {
	name := st.Argument
	if err := validateDefinitionStatus("grouping", name, st); err != nil {
		return err
	}
	if err := validateDefinitionTextMetadata("grouping", name, st); err != nil {
		return err
	}
	if scope == nil {
		if prev := m.groupings[name]; prev != nil {
			return duplicateDefinitionError("grouping", name, prev, st)
		}
		m.groupings[name] = st
		m.groupingDefOrder = append(m.groupingDefOrder, st)
		return nil
	}
	if prev := m.groupings[name]; prev != nil {
		return definitionCollisionError("grouping", name, "collides with top-level grouping", prev, st)
	}
	for parent := m.statementParents[scope]; parent != nil; parent = m.statementParents[parent] {
		if prev := m.groupingsByScope[parent][name]; prev != nil {
			return definitionCollisionError("grouping", name, "collides with ancestor scoped grouping", prev, st)
		}
	}
	defs := m.groupingsByScope[scope]
	if defs == nil {
		defs = make(map[string]*yangparse.Statement)
		m.groupingsByScope[scope] = defs
	}
	if prev := defs[name]; prev != nil {
		return duplicateDefinitionError("grouping", name, prev, st)
	}
	defs[name] = st
	return nil
}

func (m *moduleData) addFeatureDefinition(st *yangparse.Statement) error {
	name := st.Argument
	if prev := m.featureMap[name]; prev != nil {
		return duplicateDefinitionError("feature", name, prev.stmt, st)
	}
	if err := validateDefinitionStatus("feature", name, st); err != nil {
		return err
	}
	description, err := singletonDefinitionArg("feature", name, st, "description")
	if err != nil {
		return err
	}
	reference, err := singletonDefinitionArg("feature", name, st, "reference")
	if err != nil {
		return err
	}
	feature := &featureData{
		name:        name,
		module:      m,
		stmt:        st,
		description: description,
		reference:   reference,
	}
	m.featureMap[feature.name] = feature
	m.features = append(m.features, feature)
	return nil
}

func (m *moduleData) addIdentityDefinition(st *yangparse.Statement) error {
	name := st.Argument
	if prev := m.identityMap[name]; prev != nil {
		return duplicateDefinitionError("identity", name, identityStatement(prev), st)
	}
	if err := validateDefinitionStatus("identity", name, st); err != nil {
		return err
	}
	if err := validateDefinitionTextMetadata("identity", name, st); err != nil {
		return err
	}
	id := &identityData{name: name, module: m, stmt: st}
	seenBases := make(map[string]*yangparse.Statement)
	for _, b := range direct(st, "base") {
		if prev := seenBases[b.Argument]; prev != nil {
			return fmt.Errorf("identity %q has duplicate base %q at %s; previous base at %s", name, b.Argument, b.Location(), prev.Location())
		}
		seenBases[b.Argument] = b
		id.baseNames = append(id.baseNames, b.Argument)
	}
	m.identityMap[id.name] = id
	m.identities = append(m.identities, id)
	return nil
}

func duplicateDefinitionError(kind, name string, prev, current *yangparse.Statement) error {
	return fmt.Errorf("duplicate %s %q at %s; previous definition at %s", kind, name, current.Location(), prev.Location())
}

func definitionCollisionError(kind, name, reason string, prev, current *yangparse.Statement) error {
	return fmt.Errorf("duplicate %s %q at %s: %s at %s", kind, name, current.Location(), reason, prev.Location())
}

func singletonDefinitionArg(kind, name string, st *yangparse.Statement, keyword string) (string, error) {
	props := direct(st, keyword)
	if len(props) == 0 {
		return "", nil
	}
	if len(props) > 1 {
		return "", fmt.Errorf("%s %q has multiple %s statements at %s", kind, name, keyword, props[1].Location())
	}
	return props[0].Argument, nil
}

func validateDefinitionTextMetadata(kind, name string, st *yangparse.Statement) error {
	if _, err := singletonDefinitionArg(kind, name, st, "description"); err != nil {
		return err
	}
	if _, err := singletonDefinitionArg(kind, name, st, "reference"); err != nil {
		return err
	}
	return nil
}

func validateDefinitionStatus(kind, name string, st *yangparse.Statement) error {
	status, err := singletonDefinitionArg(kind, name, st, "status")
	if err != nil || status == "" {
		return err
	}
	switch status {
	case "current", "deprecated", "obsolete":
		return nil
	default:
		return fmt.Errorf("invalid status %q at %s", status, first(st, "status").Location())
	}
}

func identityStatement(id *identityData) *yangparse.Statement {
	if id == nil {
		return nil
	}
	return id.stmt
}

func (m *moduleData) recordSchemaError(err error) {
	if m != nil && m.schemaErr == nil && err != nil {
		m.schemaErr = err
	}
}

func (m *moduleData) buildIR() {
	for _, st := range m.sourceTopStatements() {
		if !m.featureIncluded(st) {
			continue
		}
		switch {
		case st.Keyword == "rpc":
			node := m.buildNode(st, m.root, m, false, "")
			m.rpcs = append(m.rpcs, node)
			m.root.children = append(m.root.children, node)
		case st.Keyword == "notification":
			node := m.buildNode(st, m.root, m, false, "")
			m.notifs = append(m.notifs, node)
			m.root.children = append(m.root.children, node)
		case isSchemaChildKeyword(st.Keyword):
			node := m.buildNode(st, m.root, m, false, "")
			m.top = append(m.top, node)
			m.root.children = append(m.root.children, node)
		case st.Keyword == "uses":
			nodes := m.expandUses(st, m.root, m, false)
			m.top = append(m.top, nodes...)
			m.root.children = append(m.root.children, nodes...)
		}
	}
}

func (m *moduleData) buildNode(st *yangparse.Statement, parent *schemaNodeData, owner *moduleData, choiceDesc bool, groupOrigin string) *schemaNodeData {
	return m.buildNodeSeen(st, parent, owner, choiceDesc, groupOrigin, nil)
}

func (m *moduleData) buildNodeSeen(st *yangparse.Statement, parent *schemaNodeData, owner *moduleData, choiceDesc bool, groupOrigin string, groupingStack map[*yangparse.Statement]bool) *schemaNodeData {
	name := st.Argument
	if name == "" && (st.Keyword == "input" || st.Keyword == "output") {
		name = st.Keyword
	}
	config := ConfigRw
	if parent != nil {
		config = parent.config
	}
	ifFeatures := ifFeatureArgs(st)
	n := &schemaNodeData{
		name:          name,
		kind:          kindForKeyword(st.Keyword),
		module:        owner,
		stmt:          st,
		parent:        parent,
		ifFeatures:    append([]string(nil), ifFeatures...),
		ownIfFeatures: append([]string(nil), ifFeatures...),
		status:        StatusCurrent,
		config:        config,
		orderedBy:     OrderedBySystem,
		typeStmt:      first(st, "type"),
		typeModule:    m,
		choiceDesc:    choiceDesc || parent != nil && (parent.kind == SchemaNodeKindChoice || parent.kind == SchemaNodeKindCase || parent.choiceDesc),
		groupOrigin:   groupOrigin,
	}
	if description := n.singletonProperty(st, "description"); description != nil && n.textMetadataPropertyAllowed(description) {
		n.description = description.Argument
	}
	n.validateOperationIOParent()
	n.validateActionParent()
	n.validateNotificationParent()
	n.validateCaseParent()
	n.validateDataNodeParent()
	n.validateOperationIOCardinality()
	if reference := n.singletonProperty(st, "reference"); reference != nil && n.textMetadataPropertyAllowed(reference) {
		n.reference = reference.Argument
	}
	if units := n.singletonProperty(st, "units"); units != nil && n.unitsPropertyAllowed(units) {
		n.units = units.Argument
	}
	n.applyStatusProperty(n.singletonProperty(st, "status"))
	n.applyConfigProperty(n.singletonProperty(st, "config"))
	n.applyMandatoryProperty(n.singletonProperty(st, "mandatory"))
	n.applyPresenceProperty(n.singletonProperty(st, "presence"))
	n.applyOrderedByProperty(n.singletonProperty(st, "ordered-by"))
	n.applyCardinalityStatements(st, true)
	n.defaults = defaultValuesFor(owner, st)
	n.extensions = owner.extensionInstances(st)
	n.musts = n.mustsFrom(st)
	if when := n.singletonProperty(st, "when"); when != nil {
		if !n.whenPropertyAllowed(when) {
			// Error recorded by whenPropertyAllowed.
		} else if err := owner.validateXPathExpressionPrefixes("when", when); err != nil {
			n.recordSchemaError(err)
		} else if constraint, err := whenFromValidated(when); err != nil {
			n.recordSchemaError(err)
		} else {
			n.whens = []WhenConstraint{constraint}
		}
	}
	uniqueStatements := direct(st, "unique")
	if len(uniqueStatements) > 0 && n.kind != SchemaNodeKindList {
		n.recordSchemaError(fmt.Errorf("unique at %s is only valid on list nodes", uniqueStatements[0].Location()))
	} else {
		for _, u := range uniqueStatements {
			names, ok := parseYANGIdentifierListFields(u.Argument)
			if !ok {
				n.recordSchemaError(fmt.Errorf("list %q unique statement has invalid identifier list %q at %s", n.name, u.Argument, u.Location()))
				continue
			}
			if len(names) == 0 {
				n.recordSchemaError(fmt.Errorf("list %q unique statement is empty at %s", n.name, u.Location()))
				continue
			}
			n.uniqueNames = append(n.uniqueNames, names)
		}
	}
	if key := n.singletonProperty(st, "key"); key != nil {
		if n.kind != SchemaNodeKindList {
			n.recordSchemaError(fmt.Errorf("key at %s is only valid on list nodes", key.Location()))
		} else {
			var ok bool
			n.keyNames, ok = parseYANGIdentifierListFields(key.Argument)
			if !ok {
				n.recordSchemaError(fmt.Errorf("list %q key statement has invalid identifier list %q at %s", n.name, key.Argument, key.Location()))
			} else if len(n.keyNames) == 0 {
				n.recordSchemaError(fmt.Errorf("list %q key statement is empty at %s", n.name, key.Location()))
			} else if err := validateListKeyFeatureStatements(st, n.keyNames); err != nil {
				n.recordSchemaError(err)
			}
		}
	}
	n.children = m.buildChildrenSeen(st, n, owner, n.choiceDesc, groupOrigin, groupingStack)
	if n.kind == SchemaNodeKindList {
		n.resolveListKeys()
	}
	n.resolveUniqueConstraints()
	return n
}

func (m *moduleData) buildChildren(st *yangparse.Statement, parent *schemaNodeData, owner *moduleData, choiceDesc bool, groupOrigin string) []*schemaNodeData {
	return m.buildChildrenSeen(st, parent, owner, choiceDesc, groupOrigin, nil)
}

func (m *moduleData) buildChildrenSeen(st *yangparse.Statement, parent *schemaNodeData, owner *moduleData, choiceDesc bool, groupOrigin string, groupingStack map[*yangparse.Statement]bool) []*schemaNodeData {
	var out []*schemaNodeData
	for _, child := range st.SubStatements() {
		if !m.featureIncluded(child) {
			continue
		}
		switch {
		case child.Keyword == "uses":
			if err := validateUsesParent(child, parent); err != nil {
				m.recordSchemaError(err)
				continue
			}
			out = append(out, m.expandUsesSeen(child, parent, owner, choiceDesc, groupingStack)...)
		case child.Keyword == "choice":
			out = append(out, m.buildChoiceSeen(child, parent, owner, choiceDesc, groupOrigin, groupingStack))
		case isSchemaChildKeyword(child.Keyword), child.Keyword == "input", child.Keyword == "output":
			out = append(out, m.buildNodeSeen(child, parent, owner, choiceDesc, groupOrigin, groupingStack))
		case child.Keyword == "rpc":
			parent.recordSchemaError(fmt.Errorf("rpc at %s is only valid at module top level", child.Location()))
		}
	}
	return out
}

func (m *moduleData) buildChoiceSeen(st *yangparse.Statement, parent *schemaNodeData, owner *moduleData, choiceDesc bool, groupOrigin string, groupingStack map[*yangparse.Statement]bool) *schemaNodeData {
	n := m.buildNodeSeen(st, parent, owner, choiceDesc, groupOrigin, groupingStack)
	var children []*schemaNodeData
	for _, child := range st.SubStatements() {
		if !m.featureIncluded(child) {
			continue
		}
		switch {
		case child.Keyword == "case":
			children = append(children, m.buildNodeSeen(child, n, owner, true, groupOrigin, groupingStack))
		case child.Keyword == "uses":
			n.recordSchemaError(fmt.Errorf("uses at %s is not valid directly under choice nodes", child.Location()))
		case isSchemaChildKeyword(child.Keyword):
			ifFeatures := ifFeatureArgs(child)
			implicit := &schemaNodeData{
				name:          child.Argument,
				kind:          SchemaNodeKindCase,
				module:        owner,
				parent:        n,
				ifFeatures:    append([]string(nil), ifFeatures...),
				ownIfFeatures: append([]string(nil), ifFeatures...),
				status:        StatusCurrent,
				config:        n.config,
				orderedBy:     OrderedBySystem,
				choiceDesc:    true,
			}
			implicit.children = []*schemaNodeData{m.buildNodeSeen(child, implicit, owner, true, groupOrigin, groupingStack)}
			children = append(children, implicit)
		}
	}
	n.children = children
	return n
}

func validateUsesParent(uses *yangparse.Statement, parent *schemaNodeData) error {
	if uses == nil {
		return nil
	}
	if parent != nil {
		switch parent.kind {
		case SchemaNodeKindContainer, SchemaNodeKindList, SchemaNodeKindCase, SchemaNodeKindInput, SchemaNodeKindOutput, SchemaNodeKindNotification:
			return nil
		}
	}
	return fmt.Errorf("uses at %s is only valid at module top level or under container, list, case, input, output, or notification nodes", uses.Location())
}

func (m *moduleData) expandUses(uses *yangparse.Statement, parent *schemaNodeData, owner *moduleData, choiceDesc bool) []*schemaNodeData {
	return m.expandUsesSeen(uses, parent, owner, choiceDesc, nil)
}

func (m *moduleData) expandUsesSeen(uses *yangparse.Statement, parent *schemaNodeData, owner *moduleData, choiceDesc bool, groupingStack map[*yangparse.Statement]bool) []*schemaNodeData {
	if !m.featureIncluded(uses) {
		return nil
	}
	groupMod, group := m.findGroupingFrom(uses.Argument, uses)
	if group == nil {
		m.recordSchemaError(fmt.Errorf("unknown grouping %q at %s", uses.Argument, uses.Location()))
		return nil
	}
	if groupingStack == nil {
		groupingStack = make(map[*yangparse.Statement]bool)
	}
	if groupingStack[group] {
		m.recordSchemaError(fmt.Errorf("grouping cycle involving %q at %s", localName(uses.Argument), uses.Location()))
		return nil
	}
	groupingStack[group] = true
	defer delete(groupingStack, group)
	children := groupMod.buildChildrenSeen(group, parent, owner, choiceDesc, localName(uses.Argument), groupingStack)
	prependIfFeatures(children, ifFeatureArgs(uses))
	for _, refine := range direct(uses, "refine") {
		if !m.featureIncluded(refine) {
			continue
		}
		if err := validateDescendantSchemaNodeIDStatement("refine", refine, m.yangVersionForStatement(refine) == "1.1"); err != nil {
			m.recordSchemaError(err)
			continue
		}
		target := findRelativeSchemaNode(m, children, strings.Split(refine.Argument, "/"))
		if target == nil {
			m.recordSchemaError(fmt.Errorf("refine %q target not found at %s", refine.Argument, refine.Location()))
			continue
		}
		applyRefine(target, refine)
	}
	for _, aug := range direct(uses, "augment") {
		if !m.featureIncluded(aug) {
			continue
		}
		if err := validateDescendantSchemaNodeIDStatement("augment", aug, m.yangVersionForStatement(aug) == "1.1"); err != nil {
			m.recordSchemaError(err)
			continue
		}
		if !m.applyUsesAugment(aug, children, owner, groupingStack, localName(uses.Argument)) {
			m.recordSchemaError(fmt.Errorf("uses augment %q target not found at %s", aug.Argument, aug.Location()))
		}
	}
	m.applyUsesWhen(uses, children)
	return children
}

func (m *moduleData) applyUsesWhen(uses *yangparse.Statement, roots []*schemaNodeData) {
	whens := direct(uses, "when")
	switch len(whens) {
	case 0:
		return
	case 1:
	default:
		m.recordSchemaError(fmt.Errorf("uses %q has multiple when statements at %s", uses.Argument, whens[1].Location()))
		return
	}
	when, err := whenFromValidated(whens[0])
	if err != nil {
		m.recordSchemaError(err)
		return
	}
	if err := m.validateXPathExpressionPrefixes("when", whens[0]); err != nil {
		m.recordSchemaError(err)
		return
	}
	var walk func(*schemaNodeData)
	walk = func(n *schemaNodeData) {
		if n == nil {
			return
		}
		n.whens = append(n.whens, when)
		for _, child := range n.children {
			walk(child)
		}
	}
	for _, root := range roots {
		walk(root)
	}
}

func (m *moduleData) applyUsesAugment(aug *yangparse.Statement, roots []*schemaNodeData, owner *moduleData, groupingStack map[*yangparse.Statement]bool, groupOrigin string) bool {
	target := findRelativeSchemaNode(m, roots, strings.Split(aug.Argument, "/"))
	if target == nil {
		return false
	}
	children := m.buildChildrenSeen(aug, target, owner, target.choiceDesc, groupOrigin, groupingStack)
	prependIfFeatures(children, ifFeatureArgs(aug))
	m.applyAugmentWhen(aug, children)
	target.children = append(target.children, children...)
	return true
}

func findRelativeSchemaNode(source *moduleData, roots []*schemaNodeData, path []string) *schemaNodeData {
	if len(path) == 0 {
		return nil
	}
	head := path[0]
	if head == "" || strings.TrimSpace(head) != head {
		return nil
	}
	var wantModule *moduleData
	if hasPrefix(head) {
		if source == nil {
			return nil
		}
		wantModule = source.resolveSourceQNameModule(head)
		if wantModule == nil {
			return nil
		}
	}
	name := localName(head)
	for _, root := range roots {
		if root == nil || root.name != name {
			continue
		}
		if wantModule != nil && root.module != wantModule {
			continue
		}
		if len(path) == 1 {
			return root
		}
		return findRelativeSchemaNode(source, root.children, path[1:])
	}
	return nil
}

func (m *moduleData) findGroupingFrom(qname string, from *yangparse.Statement) (*moduleData, *yangparse.Statement) {
	mod := m.resolveSourceQNameModule(qname)
	if mod == nil {
		return nil, nil
	}
	local := localName(qname)
	if mod != m {
		return mod, mod.groupings[local]
	}
	if def := m.lookupScopedGrouping(local, from); def != nil {
		return m, def
	}
	def := m.groupings[local]
	if !m.definitionVisibleFrom(def, from) {
		return m, nil
	}
	return m, def
}

func applyRefine(n *schemaNodeData, refine *yangparse.Statement) {
	defaults := direct(refine, "default")
	if len(defaults) > 1 {
		n.recordSchemaError(fmt.Errorf("refine %q has multiple default statements at %s", refine.Argument, defaults[1].Location()))
		return
	}
	if len(defaults) == 1 {
		n.defaults = []string{defaults[0].Argument}
	}
	if description := n.singletonProperty(refine, "description"); description != nil && n.textMetadataPropertyAllowed(description) {
		n.description = description.Argument
	}
	if reference := n.singletonProperty(refine, "reference"); reference != nil && n.textMetadataPropertyAllowed(reference) {
		n.reference = reference.Argument
	}
	refineIfFeatures := ifFeatureArgs(refine)
	n.ifFeatures = append(n.ifFeatures, refineIfFeatures...)
	n.ownIfFeatures = append(n.ownIfFeatures, refineIfFeatures...)
	n.applyMandatoryProperty(n.singletonProperty(refine, "mandatory"))
	n.applyConfigProperty(n.singletonProperty(refine, "config"))
	n.applyPresenceProperty(n.singletonProperty(refine, "presence"))
	n.applyCardinalityStatements(refine, true)
	n.musts = append(n.musts, n.mustsFrom(refine)...)
	n.refreshAncestorListConstraints()
}

func (n *schemaNodeData) refreshAncestorListConstraints() {
	for cur := n; cur != nil; cur = cur.parent {
		if cur.kind != SchemaNodeKindList {
			continue
		}
		cur.resolveListKeys()
		cur.resolveUniqueConstraints()
	}
}

func (m *moduleData) applyAugments() {
	for _, aug := range m.sourceTopStatements() {
		if aug.Keyword != "augment" || !m.featureIncluded(aug) {
			continue
		}
		if err := validateAbsoluteSchemaNodeIDStatement("augment", aug, m.yangVersionForStatement(aug) == "1.1"); err != nil {
			m.recordSchemaError(err)
			continue
		}
		targetMod, target := m.ctx.findNodeBySourceSchemaPath(m, aug.Argument)
		if target == nil || targetMod == nil {
			m.recordSchemaError(fmt.Errorf("augment %q target not found at %s", aug.Argument, aug.Location()))
			continue
		}
		if m.implemented {
			m.ctx.markImplemented(targetMod)
		}
		children := m.buildChildren(aug, target, m, false, "")
		if targetMod != m {
			if mandatory := firstMandatoryConfigNode(children); mandatory != nil {
				if m.yangVersionForStatement(aug) != "1.1" {
					m.recordSchemaError(fmt.Errorf("augment %q adds mandatory config node %q to another module and requires yang-version 1.1 at %s", aug.Argument, mandatory.name, mandatory.stmt.Location()))
					continue
				}
				if len(direct(aug, "when")) == 0 {
					m.recordSchemaError(fmt.Errorf("augment %q adds mandatory config node %q to another module without a when statement at %s", aug.Argument, mandatory.name, mandatory.stmt.Location()))
					continue
				}
			}
		}
		m.applyAugmentWhen(aug, children)
		prependIfFeatures(children, ifFeatureArgs(aug))
		target.children = append(target.children, children...)
		target.resolveListKeys()
		target.resolveUniqueConstraints()
		appendUnique(&targetMod.augmentedBy, m.name)
	}
}

func (m *moduleData) applyAugmentWhen(aug *yangparse.Statement, roots []*schemaNodeData) {
	whens := direct(aug, "when")
	switch len(whens) {
	case 0:
		return
	case 1:
	default:
		m.recordSchemaError(fmt.Errorf("augment %q has multiple when statements at %s", aug.Argument, whens[1].Location()))
		return
	}
	when, err := whenFromValidated(whens[0])
	if err != nil {
		m.recordSchemaError(err)
		return
	}
	if err := m.validateXPathExpressionPrefixes("when", whens[0]); err != nil {
		m.recordSchemaError(err)
		return
	}
	var walk func(*schemaNodeData)
	walk = func(n *schemaNodeData) {
		if n == nil {
			return
		}
		n.whens = append(n.whens, when)
		for _, child := range n.children {
			walk(child)
		}
	}
	for _, root := range roots {
		walk(root)
	}
}

func prependIfFeatures(roots []*schemaNodeData, features []string) {
	if len(features) == 0 {
		return
	}
	var walk func(*schemaNodeData)
	walk = func(n *schemaNodeData) {
		if n == nil {
			return
		}
		prefix := append([]string(nil), features...)
		n.ifFeatures = append(prefix, n.ifFeatures...)
		for _, child := range n.children {
			walk(child)
		}
	}
	for _, root := range roots {
		walk(root)
	}
}

func firstMandatoryConfigNode(nodes []*schemaNodeData) *schemaNodeData {
	for _, node := range nodes {
		if mandatory := mandatoryConfigNode(node); mandatory != nil {
			return mandatory
		}
	}
	return nil
}

func mandatoryConfigNode(n *schemaNodeData) *schemaNodeData {
	if n == nil || !n.representsConfigurationData() {
		return nil
	}
	switch n.kind {
	case SchemaNodeKindLeaf, SchemaNodeKindChoice, SchemaNodeKindAnyData:
		if n.mandatory {
			return n
		}
	case SchemaNodeKindLeafList, SchemaNodeKindList:
		if n.minElements != nil && *n.minElements > 0 {
			return n
		}
	case SchemaNodeKindContainer:
		if !n.presence {
			for _, child := range n.children {
				if mandatoryConfigNode(child) != nil {
					return n
				}
			}
		}
	}
	for _, child := range n.children {
		if mandatory := mandatoryConfigNode(child); mandatory != nil {
			return mandatory
		}
	}
	return nil
}

func (m *moduleData) collectDeviations() {
	for _, dev := range m.sourceTopStatements() {
		if dev.Keyword != "deviation" || !m.featureIncluded(dev) {
			continue
		}
		if err := validateAbsoluteSchemaNodeIDStatement("deviation", dev, m.yangVersionForStatement(dev) == "1.1"); err != nil {
			m.recordSchemaError(err)
			continue
		}
		m.ctx.markImplemented(m)
		targetMod, target := m.ctx.findNodeBySourceSchemaPath(m, dev.Argument)
		if targetMod == nil || target == nil {
			m.recordSchemaError(fmt.Errorf("deviation %q target not found at %s", dev.Argument, dev.Location()))
			continue
		}
		if m.implemented {
			m.ctx.markImplemented(targetMod)
		}
		appendUnique(&targetMod.deviatedBy, m.name)
		desc, err := singletonDefinitionArg("deviation", dev.Argument, dev, "description")
		if err != nil {
			m.recordSchemaError(err)
			continue
		}
		ref, err := singletonDefinitionArg("deviation", dev.Argument, dev, "reference")
		if err != nil {
			m.recordSchemaError(err)
			continue
		}
		deviates := direct(dev, "deviate")
		if len(deviates) == 0 {
			m.recordSchemaError(fmt.Errorf("deviation %q has no deviate statements at %s", dev.Argument, dev.Location()))
			continue
		}
		for _, d := range deviates {
			if !validDeviationType(d.Argument) {
				m.recordSchemaError(fmt.Errorf("unsupported deviation type %q at %s", d.Argument, d.Location()))
				continue
			}
			props := nonExtensionSubStatements(d)
			if len(props) == 0 {
				if len(d.SubStatements()) != 0 && d.Argument != "not-supported" {
					continue
				}
				one := Deviation{targetPath: dev.Argument, sourceModule: m.name, devType: d.Argument, description: desc, reference: ref, ifFeatures: ifFeatureArgs(dev)}
				m.deviations = append(m.deviations, one)
				if target != nil {
					target.devs = append(target.devs, one)
					m.applyDeviation(targetMod, target, d.Argument, nil)
				}
				continue
			}
			for _, prop := range props {
				one := Deviation{
					targetPath:   dev.Argument,
					sourceModule: m.name,
					devType:      d.Argument,
					property:     prop.Keyword,
					newValue:     prop.Argument,
					description:  desc,
					reference:    ref,
					ifFeatures:   ifFeatureArgs(dev),
				}
				m.deviations = append(m.deviations, one)
				if target != nil {
					target.devs = append(target.devs, one)
					m.applyDeviation(targetMod, target, d.Argument, prop)
				}
			}
		}
	}
}

func validDeviationType(value string) bool {
	switch value {
	case "not-supported", "add", "replace", "delete":
		return true
	default:
		return false
	}
}

func (m *moduleData) applyDeviation(targetMod *moduleData, target *schemaNodeData, devType string, prop *yangparse.Statement) {
	if target == nil {
		return
	}
	if devType == "not-supported" {
		removeSchemaNode(targetMod, target)
		return
	}
	if prop == nil {
		return
	}
	switch devType {
	case "add":
		m.addDeviationProperty(target, prop)
	case "replace":
		m.replaceDeviationProperty(target, prop)
	case "delete":
		deleteDeviationProperty(target, prop)
	}
}

func removeSchemaNode(mod *moduleData, target *schemaNodeData) {
	if target == nil {
		return
	}
	if target.parent != nil {
		target.parent.children = removeNodePtr(target.parent.children, target)
	}
	if mod != nil {
		mod.top = removeNodePtr(mod.top, target)
		mod.rpcs = removeNodePtr(mod.rpcs, target)
		mod.actions = removeNodePtr(mod.actions, target)
		mod.notifs = removeNodePtr(mod.notifs, target)
	}
	target.parent = nil
}

func removeNodePtr(nodes []*schemaNodeData, target *schemaNodeData) []*schemaNodeData {
	out := nodes[:0]
	for _, n := range nodes {
		if n != target {
			out = append(out, n)
		}
	}
	return out
}

func (m *moduleData) addDeviationProperty(target *schemaNodeData, prop *yangparse.Statement) {
	switch prop.Keyword {
	case "default":
		if containsString(target.defaults, prop.Argument) {
			target.recordSchemaError(fmt.Errorf("deviate add default %q for %q already exists at %s", prop.Argument, target.name, prop.Location()))
			return
		}
		target.defaults = append(target.defaults, prop.Argument)
	case "units":
		if !target.unitsPropertyAllowed(prop) {
			return
		}
		if target.units != "" {
			target.recordSchemaError(fmt.Errorf("deviate add units for %q already exists at %s", target.name, prop.Location()))
			return
		}
		target.units = prop.Argument
	case "must":
		if !target.mustPropertyAllowed(prop) {
			return
		}
		if err := m.validateXPathExpressionPrefixes("must", prop); err != nil {
			target.recordSchemaError(err)
			return
		}
		constraint, err := mustFromValidated(prop)
		if err != nil {
			target.recordSchemaError(err)
			return
		}
		target.musts = append(target.musts, constraint)
	case "unique":
		target.applyDeviationUniqueProperty(prop, false)
	case "min-elements":
		if target.hasDeviationCardinalityProperty(prop.Keyword) {
			target.recordSchemaError(fmt.Errorf("deviate add %s for %q already exists at %s", prop.Keyword, target.name, prop.Location()))
			return
		}
		target.applyCardinalityProperty(prop, false)
	case "max-elements":
		if target.hasDeviationCardinalityProperty(prop.Keyword) {
			target.recordSchemaError(fmt.Errorf("deviate add %s for %q already exists at %s", prop.Keyword, target.name, prop.Location()))
			return
		}
		target.applyCardinalityProperty(prop, false)
	case "config", "mandatory", "type":
		m.replaceDeviationProperty(target, prop)
	}
}

func (m *moduleData) replaceDeviationProperty(target *schemaNodeData, prop *yangparse.Statement) {
	switch prop.Keyword {
	case "type":
		target.typeStmt = prop
		target.typeModule = m
		target.typeInfo = nil
	case "default":
		if len(target.defaults) == 0 {
			target.recordSchemaError(fmt.Errorf("deviate replace default for %q has no existing default at %s", target.name, prop.Location()))
			return
		}
		target.defaults = []string{prop.Argument}
	case "units":
		if !target.unitsPropertyAllowed(prop) {
			return
		}
		if target.units == "" {
			target.recordSchemaError(fmt.Errorf("deviate replace units for %q has no existing units at %s", target.name, prop.Location()))
			return
		}
		target.units = prop.Argument
	case "config":
		target.applyConfigProperty(prop)
	case "mandatory":
		target.applyMandatoryProperty(prop)
	case "min-elements":
		if !target.hasDeviationCardinalityProperty(prop.Keyword) {
			target.recordSchemaError(fmt.Errorf("deviate replace %s for %q has no existing %s at %s", prop.Keyword, target.name, prop.Keyword, prop.Location()))
			return
		}
		target.applyCardinalityProperty(prop, true)
	case "max-elements":
		if !target.hasDeviationCardinalityProperty(prop.Keyword) {
			target.recordSchemaError(fmt.Errorf("deviate replace %s for %q has no existing %s at %s", prop.Keyword, target.name, prop.Keyword, prop.Location()))
			return
		}
		target.applyCardinalityProperty(prop, true)
	case "must":
		if len(target.musts) == 0 {
			target.recordSchemaError(fmt.Errorf("deviate replace must for %q has no existing must at %s", target.name, prop.Location()))
			return
		}
		if !target.mustPropertyAllowed(prop) {
			return
		}
		if err := m.validateXPathExpressionPrefixes("must", prop); err != nil {
			target.recordSchemaError(err)
			return
		}
		constraint, err := mustFromValidated(prop)
		if err != nil {
			target.recordSchemaError(err)
			return
		}
		target.musts = []MustConstraint{constraint}
	case "unique":
		if target.kind == SchemaNodeKindList && len(target.uniqueNames) == 0 {
			target.recordSchemaError(fmt.Errorf("deviate replace unique for %q has no existing unique at %s", target.name, prop.Location()))
			return
		}
		target.applyDeviationUniqueProperty(prop, true)
	}
}

func deleteDeviationProperty(target *schemaNodeData, prop *yangparse.Statement) {
	switch prop.Keyword {
	case "default":
		before := len(target.defaults)
		target.defaults = removeString(target.defaults, prop.Argument)
		if len(target.defaults) == before {
			target.recordSchemaError(fmt.Errorf("deviate delete default %q for %q does not exist at %s", prop.Argument, target.name, prop.Location()))
		}
	case "units":
		if target.units == "" || target.units != prop.Argument {
			target.recordSchemaError(fmt.Errorf("deviate delete units %q for %q does not exist at %s", prop.Argument, target.name, prop.Location()))
			return
		}
		target.units = ""
	case "must":
		cond := prop.Argument
		before := len(target.musts)
		out := target.musts[:0]
		for _, m := range target.musts {
			if m.cond != cond {
				out = append(out, m)
			}
		}
		target.musts = out
		if len(target.musts) == before {
			target.recordSchemaError(fmt.Errorf("deviate delete must %q for %q does not exist at %s", prop.Argument, target.name, prop.Location()))
		}
	case "unique":
		names, ok := parseYANGIdentifierListFields(prop.Argument)
		if !ok || len(names) == 0 {
			target.recordSchemaError(fmt.Errorf("deviate delete unique %q has invalid identifier list at %s", prop.Argument, prop.Location()))
			return
		}
		want := strings.Join(names, "\x00")
		before := len(target.uniqueNames)
		out := target.uniqueNames[:0]
		for _, names := range target.uniqueNames {
			if strings.Join(names, "\x00") != want {
				out = append(out, names)
			}
		}
		target.uniqueNames = out
		target.resolveUniqueConstraints()
		if len(target.uniqueNames) == before {
			target.recordSchemaError(fmt.Errorf("deviate delete unique %q for %q does not exist at %s", prop.Argument, target.name, prop.Location()))
		}
	case "min-elements":
		if !target.deviationCardinalityPropertyMatches(prop) {
			target.recordSchemaError(deviationDeleteCardinalityError(target, prop))
			return
		}
		target.minElements = nil
	case "max-elements":
		if !target.deviationCardinalityPropertyMatches(prop) {
			target.recordSchemaError(deviationDeleteCardinalityError(target, prop))
			return
		}
		target.maxElements = nil
		target.maxElementsSet = false
	}
}

func deviationDeleteCardinalityError(target *schemaNodeData, prop *yangparse.Statement) error {
	if target.hasDeviationCardinalityProperty(prop.Keyword) {
		return fmt.Errorf("deviate delete %s %s for %q does not exist at %s", prop.Keyword, prop.Argument, target.name, prop.Location())
	}
	return fmt.Errorf("deviate delete %s for %q does not exist at %s", prop.Keyword, target.name, prop.Location())
}

func containsString(values []string, value string) bool {
	for _, v := range values {
		if v == value {
			return true
		}
	}
	return false
}

func (n *schemaNodeData) hasDeviationCardinalityProperty(keyword string) bool {
	if n == nil {
		return false
	}
	switch keyword {
	case "min-elements":
		return n.minElements != nil
	case "max-elements":
		return n.maxElementsSet
	default:
		return false
	}
}

func (n *schemaNodeData) deviationCardinalityPropertyMatches(prop *yangparse.Statement) bool {
	if n == nil || prop == nil {
		return false
	}
	switch prop.Keyword {
	case "min-elements":
		if n.minElements == nil {
			return false
		}
		v, ok := parseUint32(prop.Argument)
		return ok && *n.minElements == v
	case "max-elements":
		if !n.maxElementsSet {
			return false
		}
		if prop.Argument == "unbounded" {
			return n.maxElements == nil
		}
		v, ok := parseUint32(prop.Argument)
		return ok && n.maxElements != nil && *n.maxElements == v
	default:
		return false
	}
}

func removeString(values []string, value string) []string {
	out := values[:0]
	for _, v := range values {
		if v != value {
			out = append(out, v)
		}
	}
	return out
}

func (m *moduleData) buildIndexes() {
	m.nodesByPath = make(map[string]*schemaNodeData)
	m.root.path = "/" + m.name
	m.nodesByPath[m.root.path] = m.root
	var walk func(*schemaNodeData)
	walk = func(n *schemaNodeData) {
		for _, c := range n.children {
			if n.kind == SchemaNodeKindModule {
				c.path = "/" + m.name + "/" + c.name
			} else {
				c.path = n.path + "/" + c.name
			}
			if c.module == m {
				m.nodesByPath[c.path] = c
			}
			walk(c)
		}
	}
	walk(m.root)
}

func (m *moduleData) validateSiblingNames() {
	m.validateSiblingNamesFrom(m.root)
}

func (m *moduleData) validateSiblingNamesFrom(root *schemaNodeData) {
	var walk func(*schemaNodeData)
	walk = func(n *schemaNodeData) {
		if n == nil || m.schemaErr != nil {
			return
		}
		type siblingKey struct {
			module *moduleData
			name   string
		}
		seen := make(map[siblingKey]*schemaNodeData, len(n.children))
		for _, child := range n.children {
			key := siblingKey{module: child.module, name: child.name}
			if prev := seen[key]; prev != nil {
				m.recordSchemaError(fmt.Errorf("duplicate schema child %q from module %q under %s; previous child at %s", child.name, childModuleName(child), parentPath(n), prev.path))
				return
			}
			seen[key] = child
		}
		for _, child := range n.children {
			walk(child)
		}
	}
	walk(root)
}

func (m *moduleData) validateListConstraints() {
	m.validateListConstraintsFrom(m.root)
}

func (m *moduleData) validateListConstraintsFrom(root *schemaNodeData) {
	var walk func(*schemaNodeData)
	walk = func(n *schemaNodeData) {
		if n == nil || m.schemaErr != nil {
			return
		}
		if n.kind == SchemaNodeKindList {
			if n.requiresListKey() && len(n.keyNames) == 0 {
				n.recordSchemaError(fmt.Errorf("config true list %q must define a key", n.name))
				return
			}
			n.resolveListKeys()
			n.validateListKeyConfigConstraints()
			n.resolveUniqueConstraints()
			n.validateUniqueConfigConstraints()
		}
		for _, child := range n.children {
			walk(child)
		}
	}
	walk(root)
}

func (m *moduleData) validateDefaultRules() {
	m.validateDefaultRulesFrom(m.root)
}

func (m *moduleData) validateDefaultRulesFrom(root *schemaNodeData) {
	var walk func(*schemaNodeData)
	walk = func(n *schemaNodeData) {
		if n == nil || m.schemaErr != nil {
			return
		}
		if len(n.defaults) > 0 && n.kind != SchemaNodeKindLeaf && n.kind != SchemaNodeKindLeafList && n.kind != SchemaNodeKindChoice {
			keyword := "schema node"
			if n.stmt != nil {
				keyword = n.stmt.Keyword
			}
			n.recordSchemaError(fmt.Errorf("%s %q cannot have default statements", keyword, n.name))
			return
		}
		if n.kind == SchemaNodeKindLeaf {
			switch {
			case len(n.defaults) > 1:
				n.recordSchemaError(fmt.Errorf("leaf %q has multiple default statements", n.name))
				return
			case n.mandatory && len(n.defaults) > 0:
				n.recordSchemaError(fmt.Errorf("mandatory leaf %q cannot have a default", n.name))
				return
			case n.listKey && len(n.defaults) > 0:
				n.recordSchemaError(fmt.Errorf("key leaf %q cannot have a default", n.name))
				return
			}
		}
		if n.kind == SchemaNodeKindLeafList {
			if n.minElements != nil && *n.minElements > 0 && len(n.defaults) > 0 {
				n.recordSchemaError(fmt.Errorf("leaf-list %q with min-elements cannot have defaults", n.name))
				return
			}
			seen := make(map[string]bool, len(n.defaults))
			for _, value := range n.defaults {
				if seen[value] {
					n.recordSchemaError(fmt.Errorf("leaf-list %q has duplicate default %q", n.name, value))
					return
				}
				seen[value] = true
			}
			if len(n.defaults) > 0 && m.yangVersionForStatement(n.stmt) != "1.1" {
				n.recordSchemaError(fmt.Errorf("leaf-list %q default statements require yang-version 1.1", n.name))
				return
			}
		}
		if n.kind == SchemaNodeKindChoice {
			switch {
			case len(n.defaults) > 1:
				n.recordSchemaError(fmt.Errorf("choice %q has multiple default statements", n.name))
				return
			case n.mandatory && len(n.defaults) > 0:
				n.recordSchemaError(fmt.Errorf("mandatory choice %q cannot have a default", n.name))
				return
			case len(n.defaults) == 1 && n.directChild(n.defaults[0]) == nil:
				n.recordSchemaError(fmt.Errorf("choice %q default %q does not reference a case", n.name, n.defaults[0]))
				return
			}
		}
		for _, child := range n.children {
			walk(child)
		}
	}
	walk(root)
}

func childModuleName(n *schemaNodeData) string {
	if n == nil || n.module == nil {
		return ""
	}
	return n.module.name
}

func parentPath(n *schemaNodeData) string {
	if n == nil || n.path == "" {
		return "<schema-root>"
	}
	return n.path
}

func (m *moduleData) parseTypes() error {
	return m.parseNodeTypes(m.root)
}

func (m *moduleData) parseNodeTypes(root *schemaNodeData) error {
	var parse func(*schemaNodeData) error
	parse = func(n *schemaNodeData) error {
		if err := validateSchemaNodeTypeStatements(n); err != nil {
			return err
		}
		if n.typeStmt != nil {
			typeMod := n.typeModule
			if typeMod == nil {
				typeMod = n.module
			}
			ti, err := typeMod.parseType(n.typeStmt)
			if err != nil {
				return err
			}
			n.typeInfo = &ti
			if err := validateDefaultValuesForNode(n); err != nil {
				return err
			}
			if err := validateListKeyType(n); err != nil {
				return err
			}
		}
		for _, c := range n.children {
			if err := parse(c); err != nil {
				return err
			}
		}
		return nil
	}
	return parse(root)
}

func (m *moduleData) validateGroupingBodyTypes() error {
	for _, grouping := range m.groupingDefinitionsInOrder() {
		scratch := &schemaNodeData{
			name:       grouping.Argument,
			kind:       SchemaNodeKindContainer,
			module:     m,
			stmt:       grouping,
			config:     ConfigRw,
			status:     StatusCurrent,
			typeModule: m,
		}
		scratch.children = m.buildChildren(grouping, scratch, m, false, grouping.Argument)
		if m.schemaErr != nil {
			return m.schemaErr
		}
		m.validateSiblingNamesFrom(scratch)
		if m.schemaErr != nil {
			return m.schemaErr
		}
		m.validateDefaultRulesFrom(scratch)
		if m.schemaErr != nil {
			return m.schemaErr
		}
		if err := m.parseNodeTypes(scratch); err != nil {
			return err
		}
		if err := validateListConstraintTypesFrom(scratch); err != nil {
			return err
		}
	}
	return nil
}

func (m *moduleData) groupingDefinitionsInOrder() []*yangparse.Statement {
	var out []*yangparse.Statement
	var walk func(*yangparse.Statement)
	walk = func(st *yangparse.Statement) {
		if st == nil {
			return
		}
		if st.Keyword == "grouping" {
			out = append(out, st)
		}
		for _, child := range st.SubStatements() {
			walk(child)
		}
	}
	for _, st := range m.sourceTopStatements() {
		walk(st)
	}
	return out
}

func validateListKeyType(n *schemaNodeData) error {
	if n == nil || !n.listKey || n.typeInfo == nil {
		return nil
	}
	if n.typeInfo.base == BaseTypeEmpty {
		if n.module != nil && n.module.yangVersionForStatement(n.stmt) == "1.1" {
			return nil
		}
		return fmt.Errorf("key leaf %q with type empty requires yang-version 1.1", n.name)
	}
	return nil
}

func validateListKeyFeatureStatements(st *yangparse.Statement, keyNames []string) error {
	if st == nil || len(keyNames) == 0 {
		return nil
	}
	keys := make(map[string]bool, len(keyNames))
	for _, key := range keyNames {
		keys[key] = true
	}
	for _, child := range direct(st, "leaf") {
		if keys[child.Argument] && len(direct(child, "if-feature")) > 0 {
			return fmt.Errorf("key leaf %q cannot have if-feature statements", child.Argument)
		}
	}
	return nil
}

func (m *moduleData) validateListConstraintTypes() error {
	return validateListConstraintTypesFrom(m.root)
}

func validateListConstraintTypesFrom(root *schemaNodeData) error {
	var walk func(*schemaNodeData) error
	walk = func(n *schemaNodeData) error {
		if n == nil {
			return nil
		}
		if n.kind == SchemaNodeKindList {
			for _, unique := range n.uniques {
				for _, leaf := range unique.leafs {
					if leaf.node != nil && leaf.node.typeInfo != nil && leaf.node.typeInfo.base == BaseTypeEmpty {
						return fmt.Errorf("unique leaf %q cannot have type empty", leaf.node.name)
					}
				}
			}
		}
		for _, child := range n.children {
			if err := walk(child); err != nil {
				return err
			}
		}
		return nil
	}
	return walk(root)
}

func validateDefaultValuesForNode(n *schemaNodeData) error {
	if n == nil || n.typeInfo == nil || len(n.defaults) == 0 {
		return nil
	}
	for _, value := range n.defaults {
		if err := validateDefaultValueForType(n, value); err != nil {
			return err
		}
	}
	return nil
}

func (m *moduleData) validateDefaultValues() error {
	var walk func(*schemaNodeData) error
	walk = func(n *schemaNodeData) error {
		if err := validateDefaultValuesForNode(n); err != nil {
			return err
		}
		for _, child := range n.children {
			if err := walk(child); err != nil {
				return err
			}
		}
		return nil
	}
	return walk(m.root)
}

func (m *moduleData) validateTypedefTypes() error {
	for _, td := range m.typedefDefinitionsInOrder() {
		typ := first(td, "type")
		if typ == nil {
			continue
		}
		if _, err := m.parseTypeSeen(typ, make(map[*yangparse.Statement]bool)); err != nil {
			return err
		}
	}
	return nil
}

func (m *moduleData) validateTypedefDefaultValues() error {
	for _, td := range m.typedefDefinitionsInOrder() {
		defaults := direct(td, "default")
		if len(defaults) == 0 {
			continue
		}
		typ := first(td, "type")
		if typ == nil {
			continue
		}
		info, err := m.parseTypeSeen(typ, make(map[*yangparse.Statement]bool))
		if err != nil {
			return err
		}
		node := &schemaNodeData{
			name:       td.Argument,
			kind:       SchemaNodeKindUnknown,
			module:     m,
			stmt:       td,
			typeInfo:   &info,
			typeModule: m,
		}
		if err := validateDefaultValueForType(node, defaults[0].Argument); err != nil {
			return err
		}
	}
	return nil
}

func (m *moduleData) typedefDefinitionsInOrder() []*yangparse.Statement {
	var out []*yangparse.Statement
	var walk func(*yangparse.Statement)
	walk = func(st *yangparse.Statement) {
		if st == nil {
			return
		}
		if st.Keyword == "typedef" {
			out = append(out, st)
		}
		for _, child := range st.SubStatements() {
			walk(child)
		}
	}
	for _, st := range m.sourceTopStatements() {
		walk(st)
	}
	return out
}

func validateDefaultValueForType(n *schemaNodeData, value string) error {
	switch resolved := n.typeInfo.resolved.(type) {
	case ResolvedBoolean:
		if value != "true" && value != "false" {
			return fmt.Errorf("default %q is not valid for boolean %s %q", value, nodeStatementKeyword(n), n.name)
		}
	case ResolvedInt:
		if err := validateIntegerDefaultValue(n, value, resolved); err != nil {
			return err
		}
	case ResolvedDecimal64:
		if err := validateDecimal64DefaultValue(n, value, resolved); err != nil {
			return err
		}
	case ResolvedEnumeration:
		if !enumValueExists(resolved.Values(), value) {
			return fmt.Errorf("default %q is not valid for enumeration %s %q", value, nodeStatementKeyword(n), n.name)
		}
		if conditionalEnumDefault(resolved.Values(), value) {
			return fmt.Errorf("default %q references enum %q marked with if-feature", value, value)
		}
	case ResolvedBits:
		if !bitsValueValid(resolved.Values(), value) {
			return fmt.Errorf("default %q is not valid for bits %s %q", value, nodeStatementKeyword(n), n.name)
		}
		if bit, ok := conditionalBitsDefault(resolved.Values(), value); ok {
			return fmt.Errorf("default %q references bit %q marked with if-feature", value, bit)
		}
	case ResolvedString:
		if err := validateStringDefaultValue(n, value, resolved); err != nil {
			return err
		}
	case ResolvedBinary:
		if err := validateBinaryDefaultValue(n, value, resolved); err != nil {
			return err
		}
	case ResolvedIdentityRef:
		if err := validateIdentityRefDefaultValue(n, value, resolved); err != nil {
			return err
		}
	case ResolvedEmpty:
		return fmt.Errorf("empty %s %q cannot have a default", nodeStatementKeyword(n), n.name)
	case ResolvedUnion:
		if !unionDefaultValueValid(n, value, resolved) {
			return fmt.Errorf("default %q is not valid for union %s %q", value, nodeStatementKeyword(n), n.name)
		}
	case ResolvedLeafRef:
		if realtype, ok := resolved.Realtype(); ok && realtype != nil {
			memberNode := *n
			memberInfo := *realtype
			memberNode.typeInfo = &memberInfo
			return validateDefaultValueForType(&memberNode, value)
		}
	}
	return nil
}

func enumValueByName(values []EnumValue, name string) (EnumValue, bool) {
	for _, value := range values {
		if value.Name() == name {
			return value, true
		}
	}
	return EnumValue{}, false
}

func enumValueExists(values []EnumValue, name string) bool {
	_, ok := enumValueByName(values, name)
	return ok
}

func conditionalEnumDefault(values []EnumValue, name string) bool {
	value, ok := enumValueByName(values, name)
	return ok && value.conditional
}

func unionDefaultValueValid(n *schemaNodeData, value string, resolved ResolvedUnion) bool {
	for _, member := range resolved.Members() {
		memberNode := *n
		memberInfo := member
		memberNode.typeInfo = &memberInfo
		if validateDefaultValueForType(&memberNode, value) == nil {
			return true
		}
	}
	return false
}

func bitsValueValid(values []EnumValue, value string) bool {
	allowed := make(map[string]bool, len(values))
	for _, bit := range values {
		allowed[bit.Name()] = true
	}
	seen := make(map[string]bool)
	for _, token := range bitsASCIIFields(value) {
		if !allowed[token] || seen[token] {
			return false
		}
		seen[token] = true
	}
	return true
}

func conditionalBitsDefault(values []EnumValue, value string) (string, bool) {
	for _, token := range bitsASCIIFields(value) {
		bit, ok := enumValueByName(values, token)
		if ok && bit.conditional {
			return token, true
		}
	}
	return "", false
}

func bitsASCIIFields(value string) []string {
	var fields []string
	start := 0
	for start < len(value) {
		for start < len(value) && value[start] == ' ' {
			start++
		}
		end := start
		for end < len(value) && value[end] != ' ' {
			end++
		}
		if end > start {
			fields = append(fields, value[start:end])
		}
		start = end
	}
	return fields
}

func validateStringDefaultValue(n *schemaNodeData, value string, resolved ResolvedString) error {
	if len(resolved.Length) > 0 {
		length := strconv.FormatInt(int64(utf8.RuneCountInString(value)), 10)
		if !rangesWithin(resolved.Length, []RangeBound{{min: length, max: length}}, "length", BaseTypeString) {
			return fmt.Errorf("default %q does not satisfy length for string %s %q", value, nodeStatementKeyword(n), n.name)
		}
	}
	for _, pattern := range resolved.Patterns {
		matched := stringPatternMatchesDefault(pattern, value)
		if !matched {
			return fmt.Errorf("default %q does not satisfy pattern for string %s %q", value, nodeStatementKeyword(n), n.name)
		}
	}
	return nil
}

func stringPatternMatchesDefault(pattern Pattern, value string) bool {
	re, err := regexp.Compile("^(?:" + xsdregex.NativePattern(pattern.Regex()) + ")$")
	if err != nil {
		return false
	}
	matched := re.MatchString(value)
	if pattern.IsInverted() {
		matched = !matched
	}
	return matched
}

func validateBinaryDefaultValue(n *schemaNodeData, value string, resolved ResolvedBinary) error {
	if strings.ContainsAny(value, "\r\n") {
		return fmt.Errorf("default %q is not valid for binary %s %q", value, nodeStatementKeyword(n), n.name)
	}
	decoded, err := base64.StdEncoding.DecodeString(value)
	if err != nil {
		return fmt.Errorf("default %q is not valid for binary %s %q", value, nodeStatementKeyword(n), n.name)
	}
	if len(resolved.Length) == 0 {
		return nil
	}
	length := strconv.FormatInt(int64(len(decoded)), 10)
	if !rangesWithin(resolved.Length, []RangeBound{{min: length, max: length}}, "length", BaseTypeBinary) {
		return fmt.Errorf("default %q is not valid for binary %s %q", value, nodeStatementKeyword(n), n.name)
	}
	return nil
}

func validateIdentityRefDefaultValue(n *schemaNodeData, value string, resolved ResolvedIdentityRef) error {
	source := n.module
	if source == nil {
		source = n.typeModule
	}
	if source == nil {
		return fmt.Errorf("default %q is not valid for identityref %s %q", value, nodeStatementKeyword(n), n.name)
	}
	id := source.identityForQNameFrom(value, n.stmt)
	if id == nil || !identityDerivedFromAny(id, resolved.Bases(), nil) {
		return fmt.Errorf("default %q is not valid for identityref %s %q", value, nodeStatementKeyword(n), n.name)
	}
	idModule := id.module
	if idModule == nil {
		idModule = source
	}
	if idModule != nil && !idModule.implemented {
		if source.ctx != nil && source.ctx.refImplemented && source.implemented && source.resolveSourceQNameModule(value) == idModule {
			source.ctx.markImplemented(idModule)
		}
		if !idModule.implemented {
			return fmt.Errorf("default %q references identity %q from non-implemented module %q", value, id.name, idModule.name)
		}
	}
	return nil
}

func validateIntegerDefaultValue(n *schemaNodeData, value string, resolved ResolvedInt) error {
	base := n.typeInfo.base
	normalized := ""
	if isSignedIntKind(resolved.Kind) {
		parsed, err := strconv.ParseInt(value, 10, intKindBitSize(resolved.Kind))
		if err != nil {
			return fmt.Errorf("default %q is not valid for %s %s %q", value, base.String(), nodeStatementKeyword(n), n.name)
		}
		normalized = strconv.FormatInt(parsed, 10)
	} else {
		parsed, err := parseRangeUint(value, intKindBitSize(resolved.Kind))
		if err != nil {
			return fmt.Errorf("default %q is not valid for %s %s %q", value, base.String(), nodeStatementKeyword(n), n.name)
		}
		normalized = strconv.FormatUint(parsed, 10)
	}
	if len(resolved.Range) > 0 && !rangesWithin(resolved.Range, []RangeBound{{min: normalized, max: normalized}}, "range", base) {
		return fmt.Errorf("default %q is not valid for %s %s %q", value, base.String(), nodeStatementKeyword(n), n.name)
	}
	return nil
}

func validateDecimal64DefaultValue(n *schemaNodeData, value string, resolved ResolvedDecimal64) error {
	if !validDecimal64Bound(value, resolved.fractionDigits.Value()) {
		return fmt.Errorf("default %q is not valid for decimal64 %s %q", value, nodeStatementKeyword(n), n.name)
	}
	normalized := formatDecimalBound(value, resolved.fractionDigits.Value())
	if len(resolved.Range) > 0 && !rangesWithin(resolved.Range, []RangeBound{{min: normalized, max: normalized}}, "range", BaseTypeDecimal64) {
		return fmt.Errorf("default %q is not valid for decimal64 %s %q", value, nodeStatementKeyword(n), n.name)
	}
	return nil
}

func isSignedIntKind(kind IntKind) bool {
	switch kind {
	case IntKindI8, IntKindI16, IntKindI32, IntKindI64:
		return true
	default:
		return false
	}
}

func intKindBitSize(kind IntKind) int {
	switch kind {
	case IntKindI8, IntKindU8:
		return 8
	case IntKindI16, IntKindU16:
		return 16
	case IntKindI32, IntKindU32:
		return 32
	default:
		return 64
	}
}

func nodeStatementKeyword(n *schemaNodeData) string {
	if n == nil || n.stmt == nil {
		return "schema node"
	}
	return n.stmt.Keyword
}

func validateSchemaNodeTypeStatements(n *schemaNodeData) error {
	if n == nil || n.stmt == nil {
		return nil
	}
	types := direct(n.stmt, "type")
	if n.kind != SchemaNodeKindLeaf && n.kind != SchemaNodeKindLeafList {
		typeStmt := n.typeStmt
		if typeStmt == nil && len(types) > 0 {
			typeStmt = types[0]
		}
		if typeStmt != nil {
			return fmt.Errorf("type at %s is only valid on leaf or leaf-list nodes", typeStmt.Location())
		}
		return nil
	}
	if len(types) == 0 {
		return fmt.Errorf("%s %q has no type at %s", n.stmt.Keyword, n.name, n.stmt.Location())
	}
	if len(types) > 1 {
		return fmt.Errorf("%s %q has multiple type statements at %s", n.stmt.Keyword, n.name, types[1].Location())
	}
	return nil
}

func (m *moduleData) resolveLeafRefs() {
	var typed []*schemaNodeData
	var walk func(*schemaNodeData)
	walk = func(n *schemaNodeData) {
		if n.typeInfo != nil {
			typed = append(typed, n)
		}
		for _, c := range n.children {
			walk(c)
		}
	}
	walk(m.root)
	for _, n := range typed {
		if n.typeInfo == nil {
			continue
		}
		lr, ok := n.typeInfo.resolved.(ResolvedLeafRef)
		if !ok {
			continue
		}
		typeMod := n.typeModule
		if typeMod == nil {
			typeMod = n.module
		}
		resolveLeafRef(n, typeMod, &lr)
		if m.implemented && lr.target == nil {
			n.recordSchemaError(fmt.Errorf("leafref %q path %q target not found", n.name, lr.path))
		}
		if m.implemented && lr.target != nil && lr.target.node != nil {
			m.ctx.markImplemented(lr.target.node.module)
		}
		if n.representsConfigurationData() && lr.requireInstance && lr.target != nil && lr.target.node != nil && lr.target.node.config == ConfigRo {
			n.recordSchemaError(fmt.Errorf(
				"leafref %q with require-instance true cannot target config false %s %q",
				n.name,
				nodeStatementKeyword(lr.target.node),
				lr.target.node.name,
			))
		}
		n.typeInfo.resolved = lr
	}
}

func (m *moduleData) resolveIdentities() {
	for _, id := range m.identities {
		m.resolveIdentity(id)
	}
}

func (m *moduleData) resolveIdentity(id *identityData) {
	if id == nil || id.resolved {
		return
	}
	source := id.module
	if source == nil {
		source = m
	}
	if id.resolving {
		source.recordSchemaError(fmt.Errorf("identity cycle involving %q", id.name))
		return
	}
	id.resolving = true
	defer func() {
		id.resolving = false
	}()
	baseStmts := direct(id.stmt, "base")
	for i, q := range id.baseNames {
		var baseStmt *yangparse.Statement
		if i < len(baseStmts) {
			baseStmt = baseStmts[i]
		}
		baseMod := source.resolveSourceQNameModule(q)
		if baseMod == nil {
			source.recordSchemaError(fmt.Errorf("unknown identity base %q for identity %q", q, id.name))
			return
		}
		base := baseMod.identityMap[localName(q)]
		if baseMod == source && base != nil && !source.definitionVisibleFrom(base.stmt, baseStmt) {
			base = nil
		}
		if base == nil {
			source.recordSchemaError(fmt.Errorf("unknown identity base %q for identity %q", q, id.name))
			return
		}
		baseMod.resolveIdentity(base)
		if baseMod.schemaErr != nil {
			return
		}
		id.bases = append(id.bases, base)
		appendDerivedToIdentityAncestors(base, id, make(map[*identityData]bool))
	}
	if len(id.baseNames) > 1 && source.yangVersionForStatement(id.stmt) != "1.1" {
		source.recordSchemaError(fmt.Errorf("identity %q with multiple base statements requires yang-version 1.1 at %s", id.name, id.stmt.Location()))
		return
	}
	id.resolved = true
}

func (m *moduleData) applyRefImplementedPolicy() {
	if m == nil || m.ctx == nil || !m.ctx.refImplemented || !m.implemented {
		return
	}
	var walk func(*schemaNodeData)
	walk = func(n *schemaNodeData) {
		if n == nil {
			return
		}
		source := n.module
		if source == nil {
			source = m
		}
		for _, must := range n.musts {
			source.markImplementedPrefixes(must.Expression())
		}
		for _, when := range n.whens {
			source.markImplementedPrefixes(when.Expression())
		}
		for _, value := range n.defaults {
			source.markImplementedPrefixes(value)
		}
		for _, c := range n.children {
			walk(c)
		}
	}
	walk(m.root)
}

func (m *moduleData) markImplementedPrefixes(text string) {
	for _, pfx := range referencedPrefixes(text) {
		if target := m.importByPfx[pfx]; target != nil {
			m.ctx.markImplemented(target)
		}
	}
}

func referencedPrefixes(text string) []string {
	var out []string
	seen := make(map[string]bool)
	quote := byte(0)
	for i := 0; i < len(text); {
		ch := text[i]
		if quote != 0 {
			if ch == quote {
				quote = 0
			}
			i++
			continue
		}
		if ch == '\'' || ch == '"' {
			quote = ch
			i++
			continue
		}
		if !yangIdentStart(ch) {
			i++
			continue
		}
		start := i
		i++
		for i < len(text) && yangIdentContinue(text[i]) {
			i++
		}
		if i >= len(text) || text[i] != ':' {
			continue
		}
		if i+1 < len(text) && text[i+1] == ':' {
			continue
		}
		pfx := text[start:i]
		if !seen[pfx] {
			seen[pfx] = true
			out = append(out, pfx)
		}
		i++
	}
	return out
}

func yangIdentStart(ch byte) bool {
	return ch == '_' || ch >= 'A' && ch <= 'Z' || ch >= 'a' && ch <= 'z'
}

func yangIdentContinue(ch byte) bool {
	return yangIdentStart(ch) || ch >= '0' && ch <= '9' || ch == '-' || ch == '.'
}

func appendDerivedToIdentityAncestors(base, derived *identityData, seen map[*identityData]bool) {
	if base == nil || derived == nil || seen[base] {
		return
	}
	seen[base] = true
	appendIdentityUnique(&base.derived, derived)
	for _, ancestor := range base.bases {
		appendDerivedToIdentityAncestors(ancestor, derived, seen)
	}
}

func (m *moduleData) resolveQNameModule(qname string) *moduleData {
	pfx, _, has := strings.Cut(qname, ":")
	if !has {
		return m
	}
	if pfx == m.prefix || pfx == m.name {
		return m
	}
	return m.importByPfx[pfx]
}

func (m *moduleData) resolveSourceQNameModule(qname string) *moduleData {
	pfx, _, has := strings.Cut(qname, ":")
	if !has {
		return m
	}
	if pfx == m.prefix {
		return m
	}
	return m.importByPfx[pfx]
}

func moduleMatchesQNamePrefix(mod *moduleData, qname string) bool {
	pfx, _, has := strings.Cut(qname, ":")
	if !has || mod == nil {
		return false
	}
	return pfx == mod.prefix || pfx == mod.name
}

func moduleMatchesSourceQNamePrefix(mod *moduleData, qname string) bool {
	pfx, _, has := strings.Cut(qname, ":")
	if !has || mod == nil {
		return false
	}
	return pfx == mod.prefix
}

func (c *Context) findNodeBySchemaPath(source *moduleData, path string) (*moduleData, *schemaNodeData) {
	return c.findNodeBySchemaPathMode(source, path, false)
}

func (c *Context) findNodeBySourceSchemaPath(source *moduleData, path string) (*moduleData, *schemaNodeData) {
	return c.findNodeBySchemaPathMode(source, path, true)
}

func (c *Context) findNodeBySchemaPathMode(source *moduleData, path string, strictSource bool) (*moduleData, *schemaNodeData) {
	parts := splitPath(path)
	if len(parts) == 0 {
		return nil, nil
	}
	first := pathStepQName(parts[0])
	mod := source.resolveQNameModule(first)
	if strictSource {
		mod = source.resolveSourceQNameModule(first)
	}
	if mod == nil {
		if !strictSource && (first == source.name || first == source.prefix) {
			mod = source
			parts = parts[1:]
		} else {
			return nil, nil
		}
	} else if !strictSource && !hasPrefix(first) && (first == mod.prefix || first == mod.name) {
		parts = parts[1:]
	}
	cur := mod.root
	for _, part := range parts {
		qname := pathStepQName(part)
		name := localName(qname)
		var next *schemaNodeData
		for _, child := range cur.children {
			if child.name == name {
				if hasPrefix(qname) {
					pm := source.resolveQNameModule(qname)
					if strictSource {
						pm = source.resolveSourceQNameModule(qname)
					}
					if pm == nil {
						matches := moduleMatchesQNamePrefix(child.module, qname)
						if strictSource {
							matches = moduleMatchesSourceQNamePrefix(child.module, qname)
						}
						if !matches {
							continue
						}
					} else if child.module != pm {
						continue
					}
				}
				next = child
				break
			}
		}
		if next == nil {
			return mod, nil
		}
		cur = next
	}
	return mod, cur
}

// Module is a borrowed handle to a loaded module.
type Module struct{ mod *moduleData }

func (m Module) Name() string {
	if m.mod == nil {
		return ""
	}
	return m.mod.name
}
func (m Module) Namespace() string {
	if m.mod == nil {
		return ""
	}
	return m.mod.namespace
}
func (m Module) Prefix() string {
	if m.mod == nil {
		return ""
	}
	return m.mod.prefix
}
func (m Module) Revision() (string, bool) {
	if m.mod == nil || m.mod.revision == "" {
		return "", false
	}
	return m.mod.revision, true
}
func (m Module) Revisions() []Revision {
	if m.mod == nil || m.mod.stmt == nil {
		return nil
	}
	revisions := direct(m.mod.stmt, "revision")
	out := make([]Revision, 0, len(revisions))
	for _, rev := range revisions {
		out = append(out, Revision{
			date:        rev.Argument,
			description: childArg(rev, "description"),
			reference:   childArg(rev, "reference"),
		})
	}
	return out
}
func (m Module) Organization() (string, bool) { return m.headerMetadata("organization") }
func (m Module) Contact() (string, bool)      { return m.headerMetadata("contact") }
func (m Module) Description() (string, bool)  { return m.headerMetadata("description") }
func (m Module) Reference() (string, bool)    { return m.headerMetadata("reference") }
func (m Module) headerMetadata(keyword string) (string, bool) {
	if m.mod == nil || m.mod.stmt == nil {
		return "", false
	}
	return optional(childArg(m.mod.stmt, keyword))
}
func (m Module) YangVersion() string {
	if m.mod == nil {
		return "1"
	}
	return m.mod.yangVersionForStatement(m.mod.stmt)
}
func (m Module) IsImplemented() bool { return m.mod != nil && m.mod.implemented }
func (m Module) Features() []Feature {
	if m.mod == nil {
		return nil
	}
	out := make([]Feature, len(m.mod.features))
	for i, f := range m.mod.features {
		out[i] = Feature{feature: f}
	}
	return out
}
func (m Module) FeatureValue(name string) (bool, bool) {
	if m.mod == nil {
		return false, false
	}
	if hasPrefix(name) && m.mod.resolveQNameModule(name) != m.mod {
		return false, false
	}
	local := localName(name)
	if m.mod.featureMap[local] == nil {
		return false, false
	}
	return m.mod.featureEnabled(local), true
}
func (m Module) Imports() []Import {
	if m.mod == nil {
		return nil
	}
	return append([]Import(nil), m.mod.imports...)
}
func (m Module) Includes() []Include {
	if m.mod == nil || m.mod.stmt == nil {
		return nil
	}
	includes := direct(m.mod.stmt, "include")
	out := make([]Include, 0, len(includes))
	for _, inc := range includes {
		out = append(out, Include{
			Name:        inc.Argument,
			Revision:    childArg(inc, "revision-date"),
			description: childArg(inc, "description"),
			reference:   childArg(inc, "reference"),
		})
	}
	return out
}
func (m Module) Extensions() []Extension {
	if m.mod == nil {
		return nil
	}
	return m.mod.topLevelExtensionInstances()
}
func (m Module) ExtensionDefinitions() []ExtensionDefinition {
	if m.mod == nil {
		return nil
	}
	out := make([]ExtensionDefinition, 0, len(m.mod.extDefOrder))
	for _, st := range m.mod.extDefOrder {
		out = append(out, ExtensionDefinition{module: m.mod, stmt: st})
	}
	return out
}
func (m Module) TypedefDefinitions() []TypedefDefinition {
	if m.mod == nil {
		return nil
	}
	out := make([]TypedefDefinition, 0, len(m.mod.typedefDefOrder))
	for _, st := range m.mod.typedefDefOrder {
		out = append(out, TypedefDefinition{module: m.mod, stmt: st})
	}
	return out
}
func (m Module) GroupingDefinitions() []GroupingDefinition {
	if m.mod == nil {
		return nil
	}
	out := make([]GroupingDefinition, 0, len(m.mod.groupingDefOrder))
	for _, st := range m.mod.groupingDefOrder {
		out = append(out, GroupingDefinition{module: m.mod, stmt: st})
	}
	return out
}
func (m Module) ResolvePrefix(prefix string) (Module, bool) {
	if m.mod == nil {
		return Module{}, false
	}
	if prefix == "" || prefix == m.mod.prefix || prefix == m.mod.name {
		return m, true
	}
	mod := m.mod.importByPfx[prefix]
	if mod == nil {
		return Module{}, false
	}
	return Module{mod: mod}, true
}
func (m Module) AugmentedBy() []string {
	if m.mod == nil {
		return nil
	}
	return append([]string(nil), m.mod.augmentedBy...)
}
func (m Module) DeviatedBy() []string {
	if m.mod == nil {
		return nil
	}
	return append([]string(nil), m.mod.deviatedBy...)
}
func (m Module) Deviations() []Deviation {
	if m.mod == nil {
		return nil
	}
	return append([]Deviation(nil), m.mod.deviations...)
}
func (m Module) TopLevel() SchemaChildren {
	if m.mod == nil {
		return SchemaChildren{}
	}
	return refs(m.mod.top)
}
func (m Module) Children() SchemaChildren {
	if m.mod == nil || m.mod.root == nil {
		return SchemaChildren{}
	}
	return refs(m.mod.root.children)
}
func (m Module) RPCs() SchemaChildren {
	if m.mod == nil {
		return SchemaChildren{}
	}
	return refs(m.mod.rpcs)
}
func (m Module) Actions() SchemaChildren {
	if m.mod == nil {
		return SchemaChildren{}
	}
	return refs(m.mod.actions)
}
func (m Module) Notifications() SchemaChildren {
	if m.mod == nil {
		return SchemaChildren{}
	}
	return refs(m.mod.notifs)
}
func (m Module) Identities() iter.Seq[Identity] {
	return func(yield func(Identity) bool) {
		if m.mod == nil {
			return
		}
		for _, id := range m.mod.identities {
			if id.module != nil && !id.module.featureIncluded(id.stmt) {
				continue
			}
			if !yield(Identity{id: id}) {
				return
			}
		}
	}
}
func (m Module) FindPath(path string) (SchemaNodeRef, error) {
	if m.mod == nil {
		return SchemaNodeRef{}, wrap("schema tree", fmt.Errorf("nil module"))
	}
	if !validAbsoluteSchemaNodeID(path, true) {
		return SchemaNodeRef{}, wrap("schema tree", fmt.Errorf("invalid schema path: %s", path))
	}
	_, n := m.mod.ctx.findNodeBySchemaPath(m.mod, path)
	if n == nil {
		return SchemaNodeRef{}, wrap("schema tree", fmt.Errorf("schema path not found: %s", path))
	}
	return SchemaNodeRef{node: n}, nil
}

// Identity is a handle to a YANG identity.
type Identity struct{ id *identityData }

func (i Identity) Name() string {
	if i.id == nil {
		return ""
	}
	return i.id.name
}
func (i Identity) Module() Module {
	if i.id == nil {
		return Module{}
	}
	return Module{mod: i.id.module}
}
func (i Identity) Description() (string, bool) {
	if i.id == nil || i.id.stmt == nil {
		return "", false
	}
	return optional(childArg(i.id.stmt, "description"))
}
func (i Identity) Reference() (string, bool) {
	if i.id == nil || i.id.stmt == nil {
		return "", false
	}
	return optional(childArg(i.id.stmt, "reference"))
}
func (i Identity) IfFeatures() []string {
	if i.id == nil || i.id.stmt == nil {
		return nil
	}
	return ifFeatureArgs(i.id.stmt)
}
func (i Identity) Status() Status {
	if i.id == nil || i.id.stmt == nil {
		return StatusCurrent
	}
	return statusFromStatement(i.id.stmt)
}
func (i Identity) Bases() []Identity {
	if i.id == nil {
		return nil
	}
	out := make([]Identity, len(i.id.bases))
	for idx, b := range i.id.bases {
		out[idx] = Identity{id: b}
	}
	return out
}
func (i Identity) Derived() []Identity {
	if i.id == nil {
		return nil
	}
	out := make([]Identity, 0, len(i.id.derived))
	for _, d := range i.id.derived {
		if d.module != nil && !d.module.featureIncluded(d.stmt) {
			continue
		}
		out = append(out, Identity{id: d})
	}
	return out
}

// SchemaNodeRef is a handle to an ordered schema IR node.
type SchemaNodeRef struct{ node *schemaNodeData }

func (n SchemaNodeRef) Name() string {
	if n.node == nil {
		return ""
	}
	return n.node.name
}
func (n SchemaNodeRef) Kind() SchemaNodeKind {
	if n.node == nil {
		return SchemaNodeKindUnknown
	}
	return n.node.kind
}
func (n SchemaNodeRef) Module() Module {
	if n.node == nil {
		return Module{}
	}
	return Module{mod: n.node.module}
}
func (n SchemaNodeRef) Description() (string, bool) {
	if n.node == nil {
		return "", false
	}
	return optional(n.node.description)
}
func (n SchemaNodeRef) Reference() (string, bool) {
	if n.node == nil {
		return "", false
	}
	return optional(n.node.reference)
}
func (n SchemaNodeRef) Status() Status {
	if n.node == nil {
		return StatusCurrent
	}
	return n.node.status
}
func (n SchemaNodeRef) Config() Config {
	if n.node == nil {
		return ConfigRw
	}
	return n.node.config
}
func (n SchemaNodeRef) YangVersion() string {
	if n.node == nil || n.node.module == nil {
		return "1"
	}
	return n.node.module.yangVersionForStatement(n.node.stmt)
}
func (n SchemaNodeRef) IsMandatory() bool         { return n.node != nil && n.node.mandatory }
func (n SchemaNodeRef) IsPresenceContainer() bool { return n.node != nil && n.node.presence }
func (n SchemaNodeRef) OrderedBy() OrderedBy {
	if n.node == nil {
		return OrderedBySystem
	}
	return n.node.orderedBy
}
func (n SchemaNodeRef) IsListKey() bool      { return n.node != nil && n.node.listKey }
func (n SchemaNodeRef) IsLeaf() bool         { return n.Kind() == SchemaNodeKindLeaf }
func (n SchemaNodeRef) IsLeafList() bool     { return n.Kind() == SchemaNodeKindLeafList }
func (n SchemaNodeRef) IsContainer() bool    { return n.Kind() == SchemaNodeKindContainer }
func (n SchemaNodeRef) IsList() bool         { return n.Kind() == SchemaNodeKindList }
func (n SchemaNodeRef) IsChoice() bool       { return n.Kind() == SchemaNodeKindChoice }
func (n SchemaNodeRef) IsCase() bool         { return n.Kind() == SchemaNodeKindCase }
func (n SchemaNodeRef) IsRPC() bool          { return n.Kind() == SchemaNodeKindRPC }
func (n SchemaNodeRef) IsAction() bool       { return n.Kind() == SchemaNodeKindAction }
func (n SchemaNodeRef) IsNotification() bool { return n.Kind() == SchemaNodeKindNotification }
func (n SchemaNodeRef) RepresentsConfigurationData() bool {
	return n.node != nil && n.node.representsConfigurationData()
}
func (n SchemaNodeRef) IsDir() bool {
	switch n.Kind() {
	case SchemaNodeKindModule, SchemaNodeKindContainer, SchemaNodeKindList, SchemaNodeKindChoice, SchemaNodeKindCase, SchemaNodeKindRPC, SchemaNodeKindAction, SchemaNodeKindInput, SchemaNodeKindOutput, SchemaNodeKindNotification:
		return true
	default:
		return false
	}
}
func (n SchemaNodeRef) ReadOnly() bool { return n.Config() == ConfigRo }
func (n SchemaNodeRef) Namespace() string {
	if n.node == nil || n.node.module == nil {
		return ""
	}
	return n.node.module.namespace
}
func (n SchemaNodeRef) IfFeatures() []string {
	if n.node == nil {
		return nil
	}
	return append([]string(nil), n.node.ifFeatures...)
}
func (n SchemaNodeRef) ListKeys() SchemaChildren {
	if n.node == nil {
		return SchemaChildren{}
	}
	return refs(n.node.keys)
}
func (n SchemaNodeRef) KeyNames() []string {
	if n.node == nil {
		return nil
	}
	return append([]string(nil), n.node.keyNames...)
}
func (n SchemaNodeRef) MinElements() (uint32, bool) {
	if n.node == nil || n.node.minElements == nil {
		return 0, false
	}
	return *n.node.minElements, true
}
func (n SchemaNodeRef) MaxElements() (uint32, bool) {
	if n.node == nil || n.node.maxElements == nil {
		return 0, false
	}
	return *n.node.maxElements, true
}
func (n SchemaNodeRef) LeafType() (TypeInfo, bool) {
	if n.node == nil || n.node.typeInfo == nil {
		return TypeInfo{}, false
	}
	return *n.node.typeInfo, true
}
func (n SchemaNodeRef) Units() (string, bool) {
	if n.node == nil {
		return "", false
	}
	return optional(n.node.units)
}
func (n SchemaNodeRef) DataChildren(flattenChoices bool) SchemaChildren {
	if n.node == nil {
		return SchemaChildren{}
	}
	var out []SchemaNodeRef
	var add func(*schemaNodeData)
	add = func(c *schemaNodeData) {
		if c.kind == SchemaNodeKindAction || c.kind == SchemaNodeKindRPC || c.kind == SchemaNodeKindNotification {
			return
		}
		if flattenChoices && (c.kind == SchemaNodeKindChoice || c.kind == SchemaNodeKindCase) {
			for _, cc := range c.children {
				add(cc)
			}
			return
		}
		out = append(out, SchemaNodeRef{node: c})
	}
	for _, c := range n.node.children {
		add(c)
	}
	return SchemaChildren{nodes: out}
}
func (n SchemaNodeRef) IsChoiceDescendant() bool { return n.node != nil && n.node.choiceDesc }
func (n SchemaNodeRef) GroupingOrigin() (string, bool) {
	if n.node == nil {
		return "", false
	}
	return optional(n.node.groupOrigin)
}
func (n SchemaNodeRef) DeviationProvenance() []Deviation {
	if n.node == nil {
		return nil
	}
	return append([]Deviation(nil), n.node.devs...)
}
func (n SchemaNodeRef) Path() string {
	if n.node == nil {
		return ""
	}
	return n.node.path
}
func (n SchemaNodeRef) Parent() (SchemaNodeRef, bool) {
	if n.node == nil || n.node.parent == nil {
		return SchemaNodeRef{}, false
	}
	return SchemaNodeRef{node: n.node.parent}, true
}
func (n SchemaNodeRef) Ancestors() []SchemaNodeRef {
	if n.node == nil {
		return nil
	}
	var rev []SchemaNodeRef
	for p := n.node.parent; p != nil && p.kind != SchemaNodeKindModule; p = p.parent {
		rev = append(rev, SchemaNodeRef{node: p})
	}
	out := make([]SchemaNodeRef, len(rev))
	for i := range rev {
		out[len(rev)-1-i] = rev[i]
	}
	return out
}
func (n SchemaNodeRef) DefaultValue() (string, bool) {
	if n.node == nil || len(n.node.defaults) == 0 {
		return "", false
	}
	return n.node.defaults[0], true
}
func (n SchemaNodeRef) DefaultValues() []string {
	if n.node == nil {
		return nil
	}
	return append([]string(nil), n.node.defaults...)
}
func (n SchemaNodeRef) TypeDefaultValue() (string, bool) {
	if n.node == nil || n.node.typeStmt == nil {
		return "", false
	}
	typeModule := n.node.typeModule
	if typeModule == nil {
		typeModule = n.node.module
	}
	if typeModule == nil {
		return "", false
	}
	return typeModule.typedefDefaultFrom(n.node.typeStmt.Argument, n.node.typeStmt)
}
func (n SchemaNodeRef) Extensions() []Extension {
	if n.node == nil {
		return nil
	}
	return append([]Extension(nil), n.node.extensions...)
}
func (n SchemaNodeRef) Extension(name string) (Extension, bool) {
	if n.node == nil {
		return Extension{}, false
	}
	for _, e := range n.node.extensions {
		if e.name == name {
			return e, true
		}
	}
	return Extension{}, false
}
func (n SchemaNodeRef) Musts() []MustConstraint {
	if n.node == nil {
		return nil
	}
	return append([]MustConstraint(nil), n.node.musts...)
}
func (n SchemaNodeRef) Whens() []WhenConstraint {
	if n.node == nil {
		return nil
	}
	return append([]WhenConstraint(nil), n.node.whens...)
}
func (n SchemaNodeRef) UniqueConstraints() []UniqueConstraint {
	if n.node == nil || n.node.kind != SchemaNodeKindList {
		return nil
	}
	return append([]UniqueConstraint(nil), n.node.uniques...)
}
func (n SchemaNodeRef) Children() SchemaChildren {
	if n.node == nil {
		return SchemaChildren{}
	}
	return refs(n.node.children)
}
func (n SchemaNodeRef) Input() (SchemaNodeRef, bool) {
	return n.operationChild(SchemaNodeKindInput)
}
func (n SchemaNodeRef) Output() (SchemaNodeRef, bool) {
	return n.operationChild(SchemaNodeKindOutput)
}
func (n SchemaNodeRef) operationChild(kind SchemaNodeKind) (SchemaNodeRef, bool) {
	if n.node == nil || (n.node.kind != SchemaNodeKindRPC && n.node.kind != SchemaNodeKindAction) {
		return SchemaNodeRef{}, false
	}
	for _, c := range n.node.children {
		if c.kind == kind {
			return SchemaNodeRef{node: c}, true
		}
	}
	return SchemaNodeRef{}, false
}

// SchemaChildren is an ordered child slice.
type SchemaChildren struct{ nodes []SchemaNodeRef }

func (c SchemaChildren) Len() int      { return len(c.nodes) }
func (c SchemaChildren) IsEmpty() bool { return len(c.nodes) == 0 }
func (c SchemaChildren) Get(i int) (SchemaNodeRef, bool) {
	if i < 0 || i >= len(c.nodes) {
		return SchemaNodeRef{}, false
	}
	return c.nodes[i], true
}
func (c SchemaChildren) Lookup(name string) (SchemaNodeRef, bool) {
	for _, n := range c.nodes {
		if n.Name() == name {
			return n, true
		}
	}
	return SchemaNodeRef{}, false
}
func (c SchemaChildren) Iter() iter.Seq[SchemaNodeRef] {
	return func(yield func(SchemaNodeRef) bool) {
		for _, n := range c.nodes {
			if !yield(n) {
				return
			}
		}
	}
}

// SchemaNode is the legacy schema-tree node kept for compatibility.
type SchemaNode struct {
	ref      SchemaNodeRef
	children []*SchemaNode
}

func (n *SchemaNode) Name() string {
	if n == nil {
		return ""
	}
	return n.ref.Name()
}
func (n *SchemaNode) Kind() SchemaNodeKind {
	if n == nil {
		return SchemaNodeKindUnknown
	}
	return n.ref.Kind()
}
func (n *SchemaNode) Children() []*SchemaNode {
	if n == nil {
		return nil
	}
	return append([]*SchemaNode(nil), n.children...)
}

// SchemaTree is the legacy tree wrapper.
type SchemaTree struct{ root *SchemaNode }

func (c *Context) SchemaTree(module string) (*SchemaTree, error) {
	mod, err := c.Schema(module)
	if err != nil {
		return nil, err
	}
	root := &SchemaNode{ref: SchemaNodeRef{node: mod.mod.root}}
	var build func(*SchemaNode, SchemaNodeRef)
	build = func(dst *SchemaNode, ref SchemaNodeRef) {
		for child := range ref.Children().Iter() {
			cn := &SchemaNode{ref: child}
			dst.children = append(dst.children, cn)
			build(cn, child)
		}
	}
	build(root, root.ref)
	return &SchemaTree{root: root}, nil
}

func (t *SchemaTree) Find(path []string) *SchemaNode {
	if t == nil || t.root == nil {
		return nil
	}
	cur := t.root
	for _, name := range path {
		var next *SchemaNode
		for _, c := range cur.children {
			if c.Name() == name {
				next = c
				break
			}
		}
		if next == nil {
			return nil
		}
		cur = next
	}
	return cur
}

func (t *SchemaTree) PreOrder(fn func(*SchemaNode) bool) {
	if t == nil || t.root == nil || fn == nil {
		return
	}
	var walk func(*SchemaNode) bool
	walk = func(n *SchemaNode) bool {
		if n == nil {
			return true
		}
		if !fn(n) {
			return false
		}
		for _, c := range n.children {
			if !walk(c) {
				return false
			}
		}
		return true
	}
	walk(t.root)
}

func refs(nodes []*schemaNodeData) SchemaChildren {
	out := make([]SchemaNodeRef, len(nodes))
	for i, n := range nodes {
		out[i] = SchemaNodeRef{node: n}
	}
	return SchemaChildren{nodes: out}
}

func (n *schemaNodeData) resolveListKeys() {
	if n == nil || n.kind != SchemaNodeKindList {
		return
	}
	n.keys = nil
	seen := make(map[string]bool, len(n.keyNames))
	for _, key := range n.keyNames {
		if seen[key] {
			n.recordSchemaError(fmt.Errorf("list %q has duplicate key %q", n.name, key))
			return
		}
		seen[key] = true
		child := n.directChild(key)
		if child == nil || child.kind != SchemaNodeKindLeaf {
			n.recordSchemaError(fmt.Errorf("list %q key %q does not reference a child leaf", n.name, key))
			return
		}
		if len(child.ownIfFeatures) > 0 {
			n.recordSchemaError(fmt.Errorf("key leaf %q cannot have if-feature statements", child.name))
			return
		}
		if len(child.whens) > 0 {
			n.recordSchemaError(fmt.Errorf("key leaf %q cannot have when statements", child.name))
			return
		}
		child.listKey = true
		n.keys = append(n.keys, child)
	}
}

func (n *schemaNodeData) validateListKeyConfigConstraints() {
	if n == nil || n.kind != SchemaNodeKindList {
		return
	}
	for _, child := range n.keys {
		if child == nil {
			continue
		}
		if child.config != n.config {
			n.recordSchemaError(fmt.Errorf("key leaf %q config must match list %q", child.name, n.name))
			return
		}
	}
}

func (n *schemaNodeData) resolveUniqueConstraints() {
	if n == nil || n.kind != SchemaNodeKindList {
		return
	}
	n.uniques = nil
	seenConstraints := make(map[string]bool, len(n.uniqueNames))
	for _, names := range n.uniqueNames {
		var leafs []SchemaNodeRef
		seen := make(map[string]bool, len(names))
		for _, name := range names {
			if seen[name] {
				n.recordSchemaError(fmt.Errorf("list %q unique constraint has duplicate leaf %q", n.name, name))
				return
			}
			seen[name] = true
			leaf, nestedList := n.descendantUniqueLeaf(name)
			if nestedList != nil {
				n.recordSchemaError(fmt.Errorf("list %q unique path %q refers to a leaf in nested list %q", n.name, name, nestedList.name))
				return
			}
			if leaf == nil {
				n.recordSchemaError(fmt.Errorf("list %q unique path %q does not reference a descendant leaf", n.name, name))
				return
			}
			leafs = append(leafs, SchemaNodeRef{node: leaf})
		}
		key := strings.Join(names, "\x00")
		if seenConstraints[key] {
			n.recordSchemaError(fmt.Errorf("list %q has duplicate unique constraint %q", n.name, strings.Join(names, " ")))
			return
		}
		seenConstraints[key] = true
		n.uniques = append(n.uniques, UniqueConstraint{leafs: leafs})
	}
}

func (n *schemaNodeData) validateUniqueConfigConstraints() {
	if n == nil || n.kind != SchemaNodeKindList {
		return
	}
	for _, unique := range n.uniques {
		hasConfig := false
		hasState := false
		var names []string
		for _, leaf := range unique.leafs {
			if leaf.node == nil {
				continue
			}
			names = append(names, leaf.node.name)
			if leaf.node.representsConfigurationData() {
				hasConfig = true
			} else {
				hasState = true
			}
		}
		if hasConfig && hasState {
			n.recordSchemaError(fmt.Errorf("unique constraint %q mixes config and state leaves", strings.Join(names, " ")))
			return
		}
	}
}

func (n *schemaNodeData) applyDeviationUniqueProperty(prop *yangparse.Statement, replace bool) {
	if n == nil || prop == nil {
		return
	}
	if n.kind != SchemaNodeKindList {
		n.recordSchemaError(fmt.Errorf("unique at %s is only valid on list nodes", prop.Location()))
		return
	}
	names, ok := parseYANGIdentifierListFields(prop.Argument)
	if !ok {
		n.recordSchemaError(fmt.Errorf("list %q unique statement has invalid identifier list %q at %s", n.name, prop.Argument, prop.Location()))
		return
	}
	if len(names) == 0 {
		n.recordSchemaError(fmt.Errorf("list %q unique statement is empty at %s", n.name, prop.Location()))
		return
	}
	if replace {
		n.uniqueNames = [][]string{names}
	} else {
		n.uniqueNames = append(n.uniqueNames, names)
	}
	n.resolveUniqueConstraints()
}

func parseYANGIdentifierListFields(value string) ([]string, bool) {
	if value == "" {
		return nil, true
	}
	if isYANGSpace(value[0]) || isYANGSpace(value[len(value)-1]) {
		return nil, false
	}
	var fields []string
	start := 0
	for start < len(value) {
		for start < len(value) && isYANGSpace(value[start]) {
			start++
		}
		end := start
		for end < len(value) && !isYANGSpace(value[end]) {
			end++
		}
		if end > start {
			fields = append(fields, value[start:end])
		}
		start = end
	}
	return fields, true
}

func yangTrimSpace(value string) string {
	start := 0
	for start < len(value) && isYANGSpace(value[start]) {
		start++
	}
	end := len(value)
	for end > start && isYANGSpace(value[end-1]) {
		end--
	}
	return value[start:end]
}

func isYANGSpace(b byte) bool {
	return b == ' ' || b == '\t' || b == '\n' || b == '\r'
}

func (n *schemaNodeData) singletonProperty(st *yangparse.Statement, keyword string) *yangparse.Statement {
	props := direct(st, keyword)
	if len(props) == 0 {
		return nil
	}
	if len(props) > 1 {
		n.recordSchemaError(fmt.Errorf("schema node %q has multiple %s statements at %s", n.name, keyword, props[1].Location()))
		return nil
	}
	return props[0]
}

func (n *schemaNodeData) validateOperationIOParent() {
	if n == nil || n.stmt == nil || n.kind != SchemaNodeKindInput && n.kind != SchemaNodeKindOutput {
		return
	}
	if n.parent != nil && (n.parent.kind == SchemaNodeKindRPC || n.parent.kind == SchemaNodeKindAction) {
		return
	}
	n.recordSchemaError(fmt.Errorf("%s at %s is only valid under rpc or action nodes", n.stmt.Keyword, n.stmt.Location()))
}

func (n *schemaNodeData) validateOperationIOCardinality() {
	if n == nil || n.stmt == nil || n.kind != SchemaNodeKindRPC && n.kind != SchemaNodeKindAction {
		return
	}
	for _, keyword := range []string{"input", "output"} {
		children := direct(n.stmt, keyword)
		if len(children) > 1 {
			n.recordSchemaError(fmt.Errorf("%s %q has multiple %s statements at %s", n.stmt.Keyword, n.name, keyword, children[1].Location()))
			return
		}
	}
}

func (n *schemaNodeData) validateActionParent() {
	if n == nil || n.stmt == nil || n.kind != SchemaNodeKindAction {
		return
	}
	if n.parent != nil && (n.parent.kind == SchemaNodeKindContainer || n.parent.kind == SchemaNodeKindList) {
		if ancestor := n.operationAncestor(); ancestor != nil {
			n.recordSchemaError(fmt.Errorf("action %q cannot have %s ancestor %q", n.name, nodeStatementKeyword(ancestor), ancestor.name))
			return
		}
		if ancestor := n.keylessListAncestor(); ancestor != nil {
			n.recordSchemaError(fmt.Errorf("action %q cannot have keyless list ancestor %q", n.name, ancestor.name))
		}
		return
	}
	n.recordSchemaError(fmt.Errorf("action at %s is only valid under container or list nodes", n.stmt.Location()))
}

func (n *schemaNodeData) validateNotificationParent() {
	if n == nil || n.stmt == nil || n.kind != SchemaNodeKindNotification {
		return
	}
	if n.parent != nil && (n.parent.kind == SchemaNodeKindModule || n.parent.kind == SchemaNodeKindContainer || n.parent.kind == SchemaNodeKindList) {
		if ancestor := n.operationAncestor(); ancestor != nil {
			n.recordSchemaError(fmt.Errorf("notification %q cannot have %s ancestor %q", n.name, nodeStatementKeyword(ancestor), ancestor.name))
			return
		}
		if ancestor := n.keylessListAncestor(); ancestor != nil {
			n.recordSchemaError(fmt.Errorf("notification %q cannot have keyless list ancestor %q", n.name, ancestor.name))
		}
		return
	}
	n.recordSchemaError(fmt.Errorf("notification at %s is only valid at module top level or under container or list nodes", n.stmt.Location()))
}

func (n *schemaNodeData) keylessListAncestor() *schemaNodeData {
	for p := n.parent; p != nil; p = p.parent {
		if p.kind == SchemaNodeKindList && len(p.keyNames) == 0 {
			return p
		}
	}
	return nil
}

func (n *schemaNodeData) requiresListKey() bool {
	return n != nil && n.kind == SchemaNodeKindList && n.representsConfigurationData()
}

func (n *schemaNodeData) representsConfigurationData() bool {
	return n != nil && n.config == ConfigRw && n.operationAncestor() == nil
}

func (n *schemaNodeData) operationAncestor() *schemaNodeData {
	for p := n.parent; p != nil; p = p.parent {
		switch p.kind {
		case SchemaNodeKindRPC, SchemaNodeKindAction, SchemaNodeKindNotification:
			return p
		}
	}
	return nil
}

func (n *schemaNodeData) validateCaseParent() {
	if n == nil || n.stmt == nil || n.kind != SchemaNodeKindCase {
		return
	}
	if n.parent != nil && n.parent.kind == SchemaNodeKindChoice {
		return
	}
	n.recordSchemaError(fmt.Errorf("case at %s is only valid under choice nodes", n.stmt.Location()))
}

func (n *schemaNodeData) validateDataNodeParent() {
	if n == nil || n.stmt == nil || !n.isDataDefinitionNode() {
		return
	}
	if n.parent != nil {
		switch n.parent.kind {
		case SchemaNodeKindModule, SchemaNodeKindContainer, SchemaNodeKindList, SchemaNodeKindChoice, SchemaNodeKindCase, SchemaNodeKindInput, SchemaNodeKindOutput, SchemaNodeKindNotification:
			return
		}
	}
	n.recordSchemaError(fmt.Errorf("%s at %s is not valid under %s nodes", n.stmt.Keyword, n.stmt.Location(), parentKindLabel(n.parent)))
}

func (n *schemaNodeData) isDataDefinitionNode() bool {
	switch n.kind {
	case SchemaNodeKindContainer, SchemaNodeKindLeaf, SchemaNodeKindLeafList, SchemaNodeKindList, SchemaNodeKindChoice, SchemaNodeKindAnyData:
		return true
	default:
		return false
	}
}

func parentKindLabel(parent *schemaNodeData) string {
	if parent == nil || parent.stmt == nil {
		return "nil"
	}
	return parent.stmt.Keyword
}

func (n *schemaNodeData) mustsFrom(st *yangparse.Statement) []MustConstraint {
	var out []MustConstraint
	for _, m := range direct(st, "must") {
		if !n.mustPropertyAllowed(m) {
			continue
		}
		if n.module != nil {
			if err := n.module.validateXPathExpressionPrefixes("must", m); err != nil {
				n.recordSchemaError(err)
				continue
			}
		}
		constraint, err := mustFromValidated(m)
		if err != nil {
			n.recordSchemaError(err)
			continue
		}
		out = append(out, constraint)
	}
	return out
}

func (n *schemaNodeData) mustPropertyAllowed(prop *yangparse.Statement) bool {
	if n == nil || prop == nil {
		return false
	}
	switch n.kind {
	case SchemaNodeKindContainer, SchemaNodeKindLeaf, SchemaNodeKindLeafList, SchemaNodeKindList, SchemaNodeKindAnyData, SchemaNodeKindInput, SchemaNodeKindOutput, SchemaNodeKindNotification:
		return true
	default:
		n.recordSchemaError(fmt.Errorf("must at %s is only valid on container, leaf, leaf-list, list, anydata, anyxml, input, output, or notification nodes", prop.Location()))
		return false
	}
}

func (n *schemaNodeData) whenPropertyAllowed(prop *yangparse.Statement) bool {
	if n == nil || prop == nil {
		return false
	}
	switch n.kind {
	case SchemaNodeKindContainer, SchemaNodeKindLeaf, SchemaNodeKindLeafList, SchemaNodeKindList, SchemaNodeKindChoice, SchemaNodeKindCase, SchemaNodeKindAnyData:
		return true
	default:
		n.recordSchemaError(fmt.Errorf("when at %s is only valid on data, choice, or case nodes", prop.Location()))
		return false
	}
}

func (n *schemaNodeData) applyStatusProperty(prop *yangparse.Statement) {
	if n == nil || prop == nil {
		return
	}
	if !n.statusPropertyAllowed(prop) {
		return
	}
	switch prop.Argument {
	case "current":
		n.status = StatusCurrent
	case "deprecated":
		n.status = StatusDeprecated
	case "obsolete":
		n.status = StatusObsolete
	default:
		n.recordSchemaError(fmt.Errorf("invalid status %q at %s", prop.Argument, prop.Location()))
	}
}

func (n *schemaNodeData) statusPropertyAllowed(prop *yangparse.Statement) bool {
	switch n.kind {
	case SchemaNodeKindContainer, SchemaNodeKindLeaf, SchemaNodeKindLeafList, SchemaNodeKindList, SchemaNodeKindChoice, SchemaNodeKindCase, SchemaNodeKindAnyData, SchemaNodeKindRPC, SchemaNodeKindAction, SchemaNodeKindNotification:
		return true
	default:
		n.recordSchemaError(fmt.Errorf("status at %s is only valid on data, choice, case, rpc, action, or notification nodes", prop.Location()))
		return false
	}
}

func (n *schemaNodeData) textMetadataPropertyAllowed(prop *yangparse.Statement) bool {
	if n == nil || prop == nil {
		return false
	}
	switch n.kind {
	case SchemaNodeKindContainer, SchemaNodeKindLeaf, SchemaNodeKindLeafList, SchemaNodeKindList, SchemaNodeKindChoice, SchemaNodeKindCase, SchemaNodeKindAnyData, SchemaNodeKindRPC, SchemaNodeKindAction, SchemaNodeKindNotification:
		return true
	default:
		n.recordSchemaError(fmt.Errorf("%s at %s is only valid on data, choice, case, rpc, action, or notification nodes", prop.Keyword, prop.Location()))
		return false
	}
}

func (n *schemaNodeData) unitsPropertyAllowed(prop *yangparse.Statement) bool {
	if n == nil || prop == nil {
		return false
	}
	if n.kind != SchemaNodeKindLeaf && n.kind != SchemaNodeKindLeafList {
		n.recordSchemaError(fmt.Errorf("units at %s is only valid on leaf or leaf-list nodes", prop.Location()))
		return false
	}
	return true
}

func (n *schemaNodeData) applyConfigProperty(prop *yangparse.Statement) {
	if n == nil || prop == nil {
		return
	}
	if !n.configPropertyAllowed(prop) {
		return
	}
	value, ok := parseYangBool(prop)
	if !ok {
		n.recordSchemaError(fmt.Errorf("invalid config %q at %s", prop.Argument, prop.Location()))
		return
	}
	n.configProp = prop
	if value {
		if ancestor := n.configFalseAncestor(); ancestor != nil {
			n.config = ConfigRo
			n.recordSchemaError(fmt.Errorf("config true is not valid under config false ancestor %q", ancestor.name))
			n.refreshDescendantConfig()
			return
		}
		n.config = ConfigRw
	} else {
		n.config = ConfigRo
	}
	n.refreshDescendantConfig()
}

func (n *schemaNodeData) configFalseAncestor() *schemaNodeData {
	for p := n.parent; p != nil; p = p.parent {
		if p.config == ConfigRo {
			return p
		}
	}
	return nil
}

func (n *schemaNodeData) refreshDescendantConfig() {
	for _, child := range n.children {
		child.refreshConfigFromParent(n.config)
	}
}

func (n *schemaNodeData) refreshConfigFromParent(parentConfig Config) {
	if n == nil {
		return
	}
	if n.configProp == nil {
		n.config = parentConfig
	} else if value, ok := parseYangBool(n.configProp); ok {
		if value {
			if parentConfig == ConfigRo {
				n.config = ConfigRo
				if ancestor := n.configFalseAncestor(); ancestor != nil {
					n.recordSchemaError(fmt.Errorf("config true is not valid under config false ancestor %q", ancestor.name))
				}
			} else {
				n.config = ConfigRw
			}
		} else {
			n.config = ConfigRo
		}
	}
	for _, child := range n.children {
		child.refreshConfigFromParent(n.config)
	}
	if n.kind == SchemaNodeKindList {
		n.resolveListKeys()
		n.resolveUniqueConstraints()
	}
}

func (n *schemaNodeData) configPropertyAllowed(prop *yangparse.Statement) bool {
	switch n.kind {
	case SchemaNodeKindContainer, SchemaNodeKindLeaf, SchemaNodeKindLeafList, SchemaNodeKindList, SchemaNodeKindChoice, SchemaNodeKindAnyData:
		return true
	default:
		n.recordSchemaError(fmt.Errorf("config at %s is only valid on data nodes", prop.Location()))
		return false
	}
}

func (n *schemaNodeData) applyMandatoryProperty(prop *yangparse.Statement) {
	if n == nil || prop == nil {
		return
	}
	if n.kind != SchemaNodeKindLeaf && n.kind != SchemaNodeKindChoice && n.kind != SchemaNodeKindAnyData {
		n.recordSchemaError(fmt.Errorf("mandatory at %s is only valid on leaf, choice, anydata, or anyxml nodes", prop.Location()))
		return
	}
	value, ok := parseYangBool(prop)
	if !ok {
		n.recordSchemaError(fmt.Errorf("invalid mandatory %q at %s", prop.Argument, prop.Location()))
		return
	}
	n.mandatory = value
}

func (n *schemaNodeData) applyPresenceProperty(prop *yangparse.Statement) {
	if n == nil || prop == nil {
		return
	}
	if n.kind != SchemaNodeKindContainer {
		n.recordSchemaError(fmt.Errorf("presence at %s is only valid on container nodes", prop.Location()))
		return
	}
	n.presence = true
}

func (n *schemaNodeData) applyOrderedByProperty(prop *yangparse.Statement) {
	if n == nil || prop == nil {
		return
	}
	if n.kind != SchemaNodeKindList && n.kind != SchemaNodeKindLeafList {
		n.recordSchemaError(fmt.Errorf("ordered-by at %s is only valid on list or leaf-list nodes", prop.Location()))
		return
	}
	switch prop.Argument {
	case "system":
		n.orderedBy = OrderedBySystem
	case "user":
		n.orderedBy = OrderedByUser
	default:
		n.recordSchemaError(fmt.Errorf("invalid ordered-by %q at %s", prop.Argument, prop.Location()))
	}
}

func parseYangBool(st *yangparse.Statement) (bool, bool) {
	if st == nil {
		return false, false
	}
	switch st.Argument {
	case "true":
		return true, true
	case "false":
		return false, true
	default:
		return false, false
	}
}

func (n *schemaNodeData) applyCardinalityStatements(st *yangparse.Statement, replace bool) {
	if n == nil || st == nil {
		return
	}
	for _, keyword := range []string{"min-elements", "max-elements"} {
		props := direct(st, keyword)
		if len(props) > 1 {
			n.recordSchemaError(fmt.Errorf("schema node %q has multiple %s statements at %s", n.name, keyword, st.Location()))
			return
		}
		if len(props) == 1 {
			n.applyCardinalityPropertyNoValidate(props[0], replace)
		}
	}
	n.validateCardinality(st.Location())
}

func (n *schemaNodeData) applyCardinalityProperty(prop *yangparse.Statement, replace bool) {
	if n == nil || prop == nil {
		return
	}
	n.applyCardinalityPropertyNoValidate(prop, replace)
	n.validateCardinality(prop.Location())
}

func (n *schemaNodeData) applyCardinalityPropertyNoValidate(prop *yangparse.Statement, replace bool) {
	if n == nil || prop == nil {
		return
	}
	if prop.Keyword != "min-elements" && prop.Keyword != "max-elements" {
		return
	}
	if n.kind != SchemaNodeKindList && n.kind != SchemaNodeKindLeafList {
		n.recordSchemaError(fmt.Errorf("%s at %s is only valid on list or leaf-list nodes", prop.Keyword, prop.Location()))
		return
	}
	switch prop.Keyword {
	case "min-elements":
		v, ok := parseUint32(prop.Argument)
		if !ok {
			n.recordSchemaError(fmt.Errorf("invalid min-elements %q at %s", prop.Argument, prop.Location()))
			return
		}
		if replace || n.minElements == nil {
			n.minElements = &v
		}
	case "max-elements":
		if prop.Argument == "unbounded" {
			if replace || !n.maxElementsSet {
				n.maxElements = nil
				n.maxElementsSet = true
			}
			return
		}
		v, ok := parseUint32(prop.Argument)
		if !ok || v == 0 {
			n.recordSchemaError(fmt.Errorf("invalid max-elements %q at %s", prop.Argument, prop.Location()))
			return
		}
		if replace || !n.maxElementsSet {
			n.maxElements = &v
			n.maxElementsSet = true
		}
	}
}

func (n *schemaNodeData) validateCardinality(location string) {
	if n == nil || n.minElements == nil || n.maxElements == nil {
		return
	}
	if *n.minElements > *n.maxElements {
		n.recordSchemaError(fmt.Errorf("schema node %q at %s has min-elements %d exceeds max-elements %d", n.name, location, *n.minElements, *n.maxElements))
	}
}

func (n *schemaNodeData) recordSchemaError(err error) {
	mod := n.ownerModule()
	if mod != nil {
		mod.recordSchemaError(err)
	}
}

func (n *schemaNodeData) ownerModule() *moduleData {
	for cur := n; cur != nil; cur = cur.parent {
		if cur.module != nil {
			return cur.module
		}
	}
	return nil
}

func (n *schemaNodeData) directChild(name string) *schemaNodeData {
	if n == nil {
		return nil
	}
	for _, child := range n.children {
		if child.name == name {
			return child
		}
	}
	return nil
}

func (n *schemaNodeData) directChildByQName(source *moduleData, qname string) *schemaNodeData {
	if n == nil || qname == "" || strings.TrimSpace(qname) != qname {
		return nil
	}
	name := localName(qname)
	var wantModule *moduleData
	if hasPrefix(qname) {
		if source == nil {
			return nil
		}
		wantModule = source.resolveSourceQNameModule(qname)
		if wantModule == nil {
			return nil
		}
	}
	for _, child := range n.children {
		if child.name != name {
			continue
		}
		if wantModule != nil && child.module != wantModule {
			continue
		}
		return child
	}
	return nil
}

func (n *schemaNodeData) descendantUniqueLeaf(path string) (*schemaNodeData, *schemaNodeData) {
	if n == nil || path == "" {
		return nil, nil
	}
	source := n.ownerModule()
	cur := n
	parts := strings.Split(path, "/")
	for i, part := range parts {
		cur = cur.directChildByQName(source, part)
		if cur == nil {
			return nil, nil
		}
		if i < len(parts)-1 && cur.kind == SchemaNodeKindList {
			return nil, cur
		}
	}
	if cur.kind != SchemaNodeKindLeaf {
		return nil, nil
	}
	return cur, nil
}

func (m *moduleData) parseType(st *yangparse.Statement) (TypeInfo, error) {
	return m.parseTypeSeen(st, make(map[*yangparse.Statement]bool))
}

func (m *moduleData) parseTypeSeen(st *yangparse.Statement, seen map[*yangparse.Statement]bool) (TypeInfo, error) {
	name := st.Argument
	if tdMod, td := m.lookupTypedefModuleFrom(name, st); td != nil {
		if seen[td] {
			return TypeInfo{base: BaseTypeUnknown, resolved: ResolvedUnknown{}}, fmt.Errorf("typedef cycle involving %q at %s", localName(name), st.Location())
		}
		seen[td] = true
		defer delete(seen, td)
		typ, err := singletonChild(td, "type")
		if err != nil {
			return TypeInfo{base: BaseTypeUnknown, resolved: ResolvedUnknown{}}, err
		}
		if typ == nil {
			return TypeInfo{base: BaseTypeUnknown, resolved: ResolvedUnknown{}}, fmt.Errorf("typedef %q at %s has no type", td.Argument, td.Location())
		}
		defaults := direct(td, "default")
		if len(defaults) > 1 {
			return TypeInfo{base: BaseTypeUnknown, resolved: ResolvedUnknown{}}, fmt.Errorf("typedef %q has multiple default statements at %s", td.Argument, defaults[1].Location())
		}
		base, err := tdMod.parseTypeSeen(typ, seen)
		if err != nil {
			return TypeInfo{base: BaseTypeUnknown, resolved: ResolvedUnknown{}}, err
		}
		typedefName := localName(name)
		base.typedefName = ptr(typedefName)
		base.typedefChain = append([]string{typedefName}, base.typedefChain...)
		if err := validateTypeRestrictionPlacement(st, base.base); err != nil {
			return TypeInfo{base: BaseTypeUnknown, resolved: ResolvedUnknown{}}, err
		}
		restricted, err := m.applyTypeRestrictions(base.resolved, st, base.base)
		if err != nil {
			return TypeInfo{base: BaseTypeUnknown, resolved: ResolvedUnknown{}}, err
		}
		base.resolved = restricted
		return base, nil
	}
	if hasPrefix(name) {
		return TypeInfo{base: BaseTypeUnknown, resolved: ResolvedUnknown{}}, fmt.Errorf("unknown type %q at %s", name, st.Location())
	}
	base := builtinBase(name)
	if base == BaseTypeUnknown {
		return TypeInfo{base: BaseTypeUnknown, resolved: ResolvedUnknown{}}, fmt.Errorf("unknown type %q at %s", name, st.Location())
	}
	if err := validateTypeRestrictionPlacement(st, base); err != nil {
		return TypeInfo{base: BaseTypeUnknown, resolved: ResolvedUnknown{}}, err
	}
	ti := TypeInfo{base: base}
	switch base {
	case BaseTypeString:
		lengths, err := restrictionRanges(st, "length", base, 0)
		if err != nil {
			return TypeInfo{base: BaseTypeUnknown, resolved: ResolvedUnknown{}}, err
		}
		ps, err := patterns(st)
		if err != nil {
			return TypeInfo{base: BaseTypeUnknown, resolved: ResolvedUnknown{}}, err
		}
		ti.resolved = ResolvedString{Length: lengths, Patterns: ps}
	case BaseTypeBoolean:
		ti.resolved = ResolvedBoolean{}
	case BaseTypeInt8, BaseTypeInt16, BaseTypeInt32, BaseTypeInt64, BaseTypeUint8, BaseTypeUint16, BaseTypeUint32, BaseTypeUint64:
		rs, err := restrictionRanges(st, "range", base, 0)
		if err != nil {
			return TypeInfo{base: BaseTypeUnknown, resolved: ResolvedUnknown{}}, err
		}
		ti.resolved = ResolvedInt{Kind: intKind(base), Range: rs}
	case BaseTypeDecimal64:
		fractionDigits := direct(st, "fraction-digits")
		if len(fractionDigits) != 1 {
			return TypeInfo{base: BaseTypeUnknown, resolved: ResolvedUnknown{}}, fmt.Errorf("decimal64 type at %s must have exactly one fraction-digits statement", st.Location())
		}
		v, ok := parseUint32(fractionDigits[0].Argument)
		if !ok {
			return TypeInfo{base: BaseTypeUnknown, resolved: ResolvedUnknown{}}, fmt.Errorf("decimal64 type at %s has invalid fraction-digits %q", fractionDigits[0].Location(), fractionDigits[0].Argument)
		}
		if v < 1 || v > 18 {
			return TypeInfo{base: BaseTypeUnknown, resolved: ResolvedUnknown{}}, fmt.Errorf("decimal64 type at %s has fraction-digits %d outside 1..18", fractionDigits[0].Location(), v)
		}
		frac, _ := NewFractionDigits(uint8(v))
		fd := frac.Value()
		rs, err := restrictionRanges(st, "range", base, fd)
		if err != nil {
			return TypeInfo{base: BaseTypeUnknown, resolved: ResolvedUnknown{}}, err
		}
		ti.resolved = ResolvedDecimal64{fractionDigits: frac, Range: rs}
	case BaseTypeEmpty:
		ti.resolved = ResolvedEmpty{}
	case BaseTypeBinary:
		lengths, err := restrictionRanges(st, "length", base, 0)
		if err != nil {
			return TypeInfo{base: BaseTypeUnknown, resolved: ResolvedUnknown{}}, err
		}
		ti.resolved = ResolvedBinary{Length: lengths}
	case BaseTypeEnumeration:
		if len(direct(st, "enum")) == 0 {
			return TypeInfo{base: BaseTypeUnknown, resolved: ResolvedUnknown{}}, fmt.Errorf("enumeration type at %s must define at least one enum", st.Location())
		}
		values, err := m.enumValues(st, "enum", "value")
		if err != nil {
			return TypeInfo{base: BaseTypeUnknown, resolved: ResolvedUnknown{}}, err
		}
		ti.resolved = ResolvedEnumeration{def: EnumDef{values: values}}
	case BaseTypeBits:
		if len(direct(st, "bit")) == 0 {
			return TypeInfo{base: BaseTypeUnknown, resolved: ResolvedUnknown{}}, fmt.Errorf("bits type at %s must define at least one bit", st.Location())
		}
		values, err := m.enumValues(st, "bit", "position")
		if err != nil {
			return TypeInfo{base: BaseTypeUnknown, resolved: ResolvedUnknown{}}, err
		}
		ti.resolved = ResolvedBits{def: BitsDef{values: values}}
	case BaseTypeIdentityRef:
		var bases []Identity
		baseStmts := direct(st, "base")
		if len(baseStmts) == 0 {
			return TypeInfo{base: BaseTypeUnknown, resolved: ResolvedUnknown{}}, fmt.Errorf("identityref type at %s must define at least one base", st.Location())
		}
		seenBases := make(map[string]*yangparse.Statement, len(baseStmts))
		for _, b := range baseStmts {
			if prev := seenBases[b.Argument]; prev != nil {
				return TypeInfo{base: BaseTypeUnknown, resolved: ResolvedUnknown{}}, fmt.Errorf("identityref type has duplicate base %q at %s; previous base at %s", b.Argument, b.Location(), prev.Location())
			}
			seenBases[b.Argument] = b
			id := m.identityForQNameFrom(b.Argument, b)
			if id == nil {
				return TypeInfo{base: BaseTypeUnknown, resolved: ResolvedUnknown{}}, fmt.Errorf("unknown identity base %q at %s", b.Argument, b.Location())
			}
			bases = append(bases, Identity{id: id})
		}
		if len(baseStmts) > 1 && m.yangVersionForStatement(st) != "1.1" {
			return TypeInfo{base: BaseTypeUnknown, resolved: ResolvedUnknown{}}, fmt.Errorf("identityref type with multiple base statements requires yang-version 1.1 at %s", st.Location())
		}
		ti.resolved = ResolvedIdentityRef{bases: bases}
	case BaseTypeInstanceIdentifier:
		require, err := requireInstance(st)
		if err != nil {
			return TypeInfo{base: BaseTypeUnknown, resolved: ResolvedUnknown{}}, err
		}
		ti.resolved = ResolvedInstanceIdentifier{RequireInstance: require}
	case BaseTypeLeafRef:
		paths := direct(st, "path")
		if len(paths) != 1 || strings.TrimSpace(paths[0].Argument) == "" {
			return TypeInfo{base: BaseTypeUnknown, resolved: ResolvedUnknown{}}, fmt.Errorf("leafref type at %s must define exactly one non-empty path", st.Location())
		}
		if !validLeafRefPathArg(paths[0].Argument) {
			return TypeInfo{base: BaseTypeUnknown, resolved: ResolvedUnknown{}}, fmt.Errorf("invalid leafref path %q at %s", paths[0].Argument, paths[0].Location())
		}
		if err := m.validateLeafRefPathPrefixes(paths[0]); err != nil {
			return TypeInfo{base: BaseTypeUnknown, resolved: ResolvedUnknown{}}, err
		}
		require, err := requireInstance(st)
		if err != nil {
			return TypeInfo{base: BaseTypeUnknown, resolved: ResolvedUnknown{}}, err
		}
		if len(direct(st, "require-instance")) > 0 && m.yangVersionForStatement(st) != "1.1" {
			return TypeInfo{base: BaseTypeUnknown, resolved: ResolvedUnknown{}}, fmt.Errorf("leafref type require-instance statement requires yang-version 1.1 at %s", st.Location())
		}
		ti.resolved = ResolvedLeafRef{path: paths[0].Argument, requireInstance: require}
	case BaseTypeUnion:
		var members []TypeInfo
		memberTypes := direct(st, "type")
		if len(memberTypes) == 0 {
			return TypeInfo{base: BaseTypeUnknown, resolved: ResolvedUnknown{}}, fmt.Errorf("union type at %s must define at least one member type", st.Location())
		}
		for _, mt := range memberTypes {
			member, err := m.parseTypeSeen(mt, seen)
			if err != nil {
				return TypeInfo{base: BaseTypeUnknown, resolved: ResolvedUnknown{}}, err
			}
			if m.unionMemberRequiresYang11(st, member) {
				return TypeInfo{base: BaseTypeUnknown, resolved: ResolvedUnknown{}}, fmt.Errorf("union member type %q requires yang-version 1.1 at %s", member.Base().String(), mt.Location())
			}
			members = append(members, member)
		}
		ti.resolved = ResolvedUnion{members: members}
	default:
		ti.resolved = ResolvedUnknown{}
	}
	return ti, nil
}

func (m *moduleData) unionMemberRequiresYang11(union *yangparse.Statement, member TypeInfo) bool {
	if union == nil || union.Keyword != "type" || union.Argument != "union" || m.yangVersionForStatement(union) == "1.1" {
		return false
	}
	switch member.Base() {
	case BaseTypeEmpty, BaseTypeLeafRef:
		return true
	default:
		return false
	}
}

func validateTypeRestrictionPlacement(st *yangparse.Statement, base BaseType) error {
	for _, child := range st.SubStatements() {
		if hasPrefix(child.Keyword) {
			continue
		}
		if !isKnownTypeRestrictionKeyword(child.Keyword) {
			continue
		}
		if typeRestrictionAllowedForBase(child.Keyword, base) {
			continue
		}
		return fmt.Errorf("%s is not valid for type %s at %s", child.Keyword, base.String(), child.Location())
	}
	return nil
}

func isKnownTypeRestrictionKeyword(keyword string) bool {
	switch keyword {
	case "base", "bit", "enum", "fraction-digits", "length", "path", "pattern", "range", "require-instance", "type":
		return true
	default:
		return false
	}
}

func typeRestrictionAllowedForBase(keyword string, base BaseType) bool {
	switch keyword {
	case "base":
		return base == BaseTypeIdentityRef
	case "bit":
		return base == BaseTypeBits
	case "enum":
		return base == BaseTypeEnumeration
	case "fraction-digits":
		return base == BaseTypeDecimal64
	case "length":
		return base == BaseTypeString || base == BaseTypeBinary
	case "path":
		return base == BaseTypeLeafRef
	case "pattern":
		return base == BaseTypeString
	case "range":
		return isIntBase(base) || base == BaseTypeDecimal64
	case "require-instance":
		return base == BaseTypeLeafRef || base == BaseTypeInstanceIdentifier
	case "type":
		return base == BaseTypeUnion
	default:
		return true
	}
}

func (m *moduleData) lookupTypedefModuleFrom(qname string, from *yangparse.Statement) (*moduleData, *yangparse.Statement) {
	mod := m.resolveSourceQNameModule(qname)
	if mod == nil {
		return nil, nil
	}
	local := localName(qname)
	if mod != m {
		return mod, mod.typedefs[local]
	}
	if def := m.lookupScopedTypedef(local, from); def != nil {
		return m, def
	}
	def := m.typedefs[local]
	if !m.definitionVisibleFrom(def, from) {
		return m, nil
	}
	return m, def
}

func (m *moduleData) lookupScopedTypedef(name string, from *yangparse.Statement) *yangparse.Statement {
	for cur := from; cur != nil; cur = m.statementParents[cur] {
		if defs := m.typedefsByScope[cur]; defs != nil {
			if def := defs[name]; def != nil {
				return def
			}
		}
	}
	return nil
}

func (m *moduleData) lookupScopedGrouping(name string, from *yangparse.Statement) *yangparse.Statement {
	for cur := from; cur != nil; cur = m.statementParents[cur] {
		if defs := m.groupingsByScope[cur]; defs != nil {
			if def := defs[name]; def != nil {
				return def
			}
		}
	}
	return nil
}

func defaultValuesFor(m *moduleData, st *yangparse.Statement) []string {
	var out []string
	for _, d := range direct(st, "default") {
		out = append(out, d.Argument)
	}
	if len(out) > 0 {
		return out
	}
	if typ := first(st, "type"); typ != nil {
		if d, ok := m.typedefDefaultFrom(typ.Argument, typ); ok {
			return []string{d}
		}
	}
	return nil
}

func requireInstance(st *yangparse.Statement) (bool, error) {
	value, ok, err := requireInstanceOverride(st)
	if err != nil {
		return false, err
	}
	if ok {
		return value, nil
	}
	return true, nil
}

func requireInstanceOverride(st *yangparse.Statement) (bool, bool, error) {
	reqs := direct(st, "require-instance")
	if len(reqs) == 0 {
		return false, false, nil
	}
	if len(reqs) > 1 {
		return false, false, fmt.Errorf("type at %s has multiple require-instance statements", st.Location())
	}
	value, ok := parseYangBool(reqs[0])
	if !ok {
		return false, false, fmt.Errorf("invalid require-instance %q at %s", reqs[0].Argument, reqs[0].Location())
	}
	return value, true, nil
}

func (m *moduleData) typedefDefaultFrom(qname string, from *yangparse.Statement) (string, bool) {
	return m.typedefDefaultFromSeen(qname, from, make(map[*yangparse.Statement]bool))
}

func (m *moduleData) typedefDefaultFromSeen(qname string, from *yangparse.Statement, seen map[*yangparse.Statement]bool) (string, bool) {
	tdMod, td := m.lookupTypedefModuleFrom(qname, from)
	if td == nil {
		return "", false
	}
	if seen[td] {
		return "", false
	}
	seen[td] = true
	defer delete(seen, td)
	if d := childArg(td, "default"); d != "" {
		return d, true
	}
	if typ := first(td, "type"); typ != nil {
		return tdMod.typedefDefaultFromSeen(typ.Argument, typ, seen)
	}
	return "", false
}

func resolveLeafRef(n *schemaNodeData, source *moduleData, lr *ResolvedLeafRef) {
	resolveLeafRefWithSeen(n, source, lr, nil)
}

func resolveLeafRefWithSeen(n *schemaNodeData, source *moduleData, lr *ResolvedLeafRef, seen map[*schemaNodeData]bool) {
	if n == nil || lr == nil || lr.path == "" {
		return
	}
	if source == nil {
		source = n.module
	}
	var target *schemaNodeData
	if strings.HasPrefix(lr.path, "/") {
		_, target = source.ctx.findNodeBySourceSchemaPath(source, lr.path)
	} else {
		target = findRelativeSchemaPathWithSeen(n, source, lr.path, seen)
	}
	if target == nil || target.typeInfo == nil {
		return
	}
	ref := SchemaNodeRef{node: target}
	lr.target = &ref
	rt := *target.typeInfo
	lr.realtype = &rt
}

func findRelativeSchemaPathWithSeen(start *schemaNodeData, source *moduleData, path string, seen map[*schemaNodeData]bool) *schemaNodeData {
	if start == nil || source == nil {
		return nil
	}
	cur := start
	for _, part := range splitPath(path) {
		part = strings.TrimSpace(part)
		switch part {
		case "", ".":
			continue
		case "..":
			if cur.parent == nil {
				return nil
			}
			cur = cur.parent
			continue
		}
		if arg, ok := derefArgument(part); ok {
			refNode := findRelativeSchemaPathWithSeen(cur, source, arg, seen)
			target := leafRefTargetNode(refNode, seen)
			if target == nil {
				return nil
			}
			cur = target
			continue
		}
		qname := pathStepQName(part)
		name := localName(qname)
		var wantModule *moduleData
		if hasPrefix(qname) {
			wantModule = source.resolveSourceQNameModule(qname)
			if wantModule == nil {
				return nil
			}
		}
		var next *schemaNodeData
		for _, child := range cur.children {
			if child.name != name {
				continue
			}
			if wantModule != nil && child.module != wantModule {
				continue
			}
			next = child
			break
		}
		if next == nil {
			return nil
		}
		cur = next
	}
	return cur
}

func leafRefTargetNode(n *schemaNodeData, seen map[*schemaNodeData]bool) *schemaNodeData {
	if n == nil || n.typeInfo == nil {
		return nil
	}
	lr, ok := n.typeInfo.resolved.(ResolvedLeafRef)
	if !ok {
		return nil
	}
	if lr.target != nil {
		return lr.target.node
	}
	if seen == nil {
		seen = make(map[*schemaNodeData]bool)
	}
	if seen[n] {
		return nil
	}
	seen[n] = true
	source := n.typeModule
	if source == nil {
		source = n.module
	}
	resolveLeafRefWithSeen(n, source, &lr, seen)
	n.typeInfo.resolved = lr
	if lr.target == nil {
		return nil
	}
	return lr.target.node
}

func (m *moduleData) identityForQNameFrom(qname string, from *yangparse.Statement) *identityData {
	mod := m.resolveSourceQNameModule(qname)
	if mod == nil {
		return nil
	}
	id := mod.identityMap[localName(qname)]
	if mod == m && id != nil && !m.definitionVisibleFrom(id.stmt, from) {
		return nil
	}
	return id
}

func (m *moduleData) applyTypeRestrictions(r ResolvedType, st *yangparse.Statement, base BaseType) (ResolvedType, error) {
	switch v := r.(type) {
	case ResolvedInt:
		rs, err := restrictionRanges(st, "range", base, 0)
		if err != nil {
			return nil, err
		}
		if len(rs) > 0 {
			if err := validateDerivedRangeSubset(st, "range", base, v.Range, rs); err != nil {
				return nil, err
			}
			v.Range = rs
		}
		return v, nil
	case ResolvedDecimal64:
		rs, err := restrictionRanges(st, "range", base, v.fractionDigits.Value())
		if err != nil {
			return nil, err
		}
		if len(rs) > 0 {
			if err := validateDerivedRangeSubset(st, "range", base, v.Range, rs); err != nil {
				return nil, err
			}
			v.Range = rs
		}
		return v, nil
	case ResolvedString:
		rs, err := restrictionRanges(st, "length", base, 0)
		if err != nil {
			return nil, err
		}
		if len(rs) > 0 {
			if err := validateDerivedRangeSubset(st, "length", base, v.Length, rs); err != nil {
				return nil, err
			}
			v.Length = rs
		}
		ps, err := patterns(st)
		if err != nil {
			return nil, err
		}
		if len(ps) > 0 {
			v.Patterns = append(v.Patterns, ps...)
		}
		return v, nil
	case ResolvedBinary:
		rs, err := restrictionRanges(st, "length", base, 0)
		if err != nil {
			return nil, err
		}
		if len(rs) > 0 {
			if err := validateDerivedRangeSubset(st, "length", base, v.Length, rs); err != nil {
				return nil, err
			}
			v.Length = rs
		}
		return v, nil
	case ResolvedEnumeration:
		values, err := m.restrictedEnumBitValues(v.Values(), st, "enum", "value")
		if err != nil {
			return nil, err
		}
		if len(values) > 0 {
			v.def = EnumDef{values: values}
		}
		return v, nil
	case ResolvedBits:
		values, err := m.restrictedEnumBitValues(v.Values(), st, "bit", "position")
		if err != nil {
			return nil, err
		}
		if len(values) > 0 {
			v.def = BitsDef{values: values}
		}
		return v, nil
	case ResolvedIdentityRef:
		bases, err := m.restrictedIdentityBases(v.Bases(), st)
		if err != nil {
			return nil, err
		}
		if len(bases) > 0 {
			v.bases = bases
		}
		return v, nil
	case ResolvedInstanceIdentifier:
		if value, ok, err := requireInstanceOverride(st); err != nil {
			return nil, err
		} else if ok {
			v.RequireInstance = value
		}
		return v, nil
	case ResolvedLeafRef:
		if value, ok, err := requireInstanceOverride(st); err != nil {
			return nil, err
		} else if ok {
			v.requireInstance = value
		}
		return v, nil
	default:
		return r, nil
	}
}

func (m *moduleData) restrictedIdentityBases(base []Identity, st *yangparse.Statement) ([]Identity, error) {
	restrictions := direct(st, "base")
	if len(restrictions) == 0 {
		return nil, nil
	}
	var out []Identity
	seenBases := make(map[string]*yangparse.Statement, len(restrictions))
	for _, restriction := range restrictions {
		if prev := seenBases[restriction.Argument]; prev != nil {
			return nil, fmt.Errorf("identityref type has duplicate base %q at %s; previous base at %s", restriction.Argument, restriction.Location(), prev.Location())
		}
		seenBases[restriction.Argument] = restriction
		id := m.identityForQNameFrom(restriction.Argument, restriction)
		if id == nil {
			return nil, fmt.Errorf("unknown identity base %q at %s", restriction.Argument, restriction.Location())
		}
		if !identityDerivedFromAny(id, base, nil) {
			return nil, fmt.Errorf("identity base %q at %s is not derived from the typedef base set", restriction.Argument, restriction.Location())
		}
		out = append(out, Identity{id: id})
	}
	return out, nil
}

func identityDerivedFromAny(id *identityData, bases []Identity, seen map[*identityData]bool) bool {
	if id == nil {
		return false
	}
	for _, base := range bases {
		if base.id == id {
			return true
		}
	}
	if seen == nil {
		seen = make(map[*identityData]bool)
	}
	if seen[id] {
		return false
	}
	seen[id] = true
	source := id.module
	if source == nil {
		return false
	}
	for _, baseName := range id.baseNames {
		baseMod := source.resolveSourceQNameModule(baseName)
		if baseMod == nil {
			continue
		}
		if identityDerivedFromAny(baseMod.identityMap[localName(baseName)], bases, seen) {
			return true
		}
	}
	return false
}

func (m *moduleData) restrictedEnumBitValues(base []EnumValue, st *yangparse.Statement, keyword, valueKeyword string) ([]EnumValue, error) {
	restrictions := direct(st, keyword)
	if len(restrictions) == 0 {
		return nil, nil
	}
	baseByName := make(map[string]EnumValue, len(base))
	for _, value := range base {
		baseByName[value.Name()] = value
	}
	var out []EnumValue
	seenNames := make(map[string]*yangparse.Statement, len(restrictions))
	for _, restriction := range restrictions {
		if !m.featureIncluded(restriction) {
			continue
		}
		if err := validateEnumBitMetadata(keyword, restriction); err != nil {
			return nil, err
		}
		if prev := seenNames[restriction.Argument]; prev != nil {
			return nil, fmt.Errorf("duplicate %s name %q at %s; previous definition at %s", keyword, restriction.Argument, restriction.Location(), prev.Location())
		}
		seenNames[restriction.Argument] = restriction
		baseValue, ok := baseByName[restriction.Argument]
		if !ok {
			return nil, fmt.Errorf("%s %q at %s does not exist in base type", keyword, restriction.Argument, restriction.Location())
		}
		if err := validateRestrictedEnumBitValue(baseValue, restriction, keyword, valueKeyword); err != nil {
			return nil, err
		}
		if ifFeatures := ifFeatureArgs(restriction); len(ifFeatures) > 0 {
			baseValue.ifFeatures = ifFeatures
			baseValue.conditional = true
		}
		out = append(out, baseValue)
	}
	return out, nil
}

func validateRestrictedEnumBitValue(baseValue EnumValue, st *yangparse.Statement, keyword, valueKeyword string) error {
	values := direct(st, valueKeyword)
	if len(values) == 0 {
		return nil
	}
	if len(values) > 1 {
		return fmt.Errorf("%s %q has multiple %s statements at %s", keyword, st.Argument, valueKeyword, values[1].Location())
	}
	raw := values[0].Argument
	var parsed int64
	switch keyword {
	case "enum":
		value, ok := parseYANGInt32(raw)
		if !ok {
			return fmt.Errorf("enum %q at %s has invalid value %q", st.Argument, values[0].Location(), raw)
		}
		parsed = value
	case "bit":
		value, ok := parseUint32(raw)
		if !ok {
			return fmt.Errorf("bit %q at %s has invalid position %q", st.Argument, values[0].Location(), raw)
		}
		parsed = int64(value)
	default:
		value, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			return fmt.Errorf("%s %q at %s has invalid %s %q", keyword, st.Argument, values[0].Location(), valueKeyword, raw)
		}
		parsed = value
	}
	if parsed != baseValue.Value() {
		return fmt.Errorf("%s %q at %s has %s %d, want base value %d", keyword, st.Argument, values[0].Location(), valueKeyword, parsed, baseValue.Value())
	}
	return nil
}

func validateDerivedRangeSubset(st *yangparse.Statement, keyword string, base BaseType, baseRanges, derived []RangeBound) error {
	if len(baseRanges) == 0 || len(derived) == 0 {
		return nil
	}
	if rangesWithin(baseRanges, derived, keyword, base) {
		return nil
	}
	expr := ""
	location := st.Location()
	if restriction := first(st, keyword); restriction != nil {
		expr = restriction.Argument
		location = restriction.Location()
	}
	return fmt.Errorf("%s restriction %q is not within the base restriction at %s", keyword, expr, location)
}

func rangesWithin(baseRanges, derived []RangeBound, keyword string, base BaseType) bool {
	for _, child := range derived {
		within := false
		for _, parent := range baseRanges {
			minCmp, minOK := compareBounds(keyword, base, parent.Min(), child.Min())
			maxCmp, maxOK := compareBounds(keyword, base, child.Max(), parent.Max())
			if minOK && maxOK && minCmp <= 0 && maxCmp <= 0 {
				within = true
				break
			}
		}
		if !within {
			return false
		}
	}
	return true
}

func builtinBase(name string) BaseType {
	switch localName(name) {
	case "string":
		return BaseTypeString
	case "boolean":
		return BaseTypeBoolean
	case "int8":
		return BaseTypeInt8
	case "int16":
		return BaseTypeInt16
	case "int32":
		return BaseTypeInt32
	case "int64":
		return BaseTypeInt64
	case "uint8":
		return BaseTypeUint8
	case "uint16":
		return BaseTypeUint16
	case "uint32":
		return BaseTypeUint32
	case "uint64":
		return BaseTypeUint64
	case "decimal64":
		return BaseTypeDecimal64
	case "empty":
		return BaseTypeEmpty
	case "binary":
		return BaseTypeBinary
	case "bits":
		return BaseTypeBits
	case "enumeration":
		return BaseTypeEnumeration
	case "identityref":
		return BaseTypeIdentityRef
	case "instance-identifier":
		return BaseTypeInstanceIdentifier
	case "leafref":
		return BaseTypeLeafRef
	case "union":
		return BaseTypeUnion
	default:
		return BaseTypeUnknown
	}
}

func intKind(base BaseType) IntKind {
	switch base {
	case BaseTypeInt8:
		return IntKindI8
	case BaseTypeInt16:
		return IntKindI16
	case BaseTypeInt32:
		return IntKindI32
	case BaseTypeInt64:
		return IntKindI64
	case BaseTypeUint8:
		return IntKindU8
	case BaseTypeUint16:
		return IntKindU16
	case BaseTypeUint32:
		return IntKindU32
	case BaseTypeUint64:
		return IntKindU64
	default:
		return IntKindI64
	}
}

func restrictionRanges(st *yangparse.Statement, keyword string, base BaseType, fractionDigits uint8) ([]RangeBound, error) {
	statements := direct(st, keyword)
	if len(statements) == 0 {
		return nil, nil
	}
	if len(statements) > 1 {
		return nil, fmt.Errorf("type at %s has multiple %s statements", st.Location(), keyword)
	}
	metadata, err := restrictionMetadata(statements[0])
	if err != nil {
		return nil, err
	}
	out, err := ranges(statements[0].Argument, keyword, base, fractionDigits)
	if err != nil {
		return nil, err
	}
	for i := range out {
		out[i].errorMessage = metadata.errorMessage
		out[i].errorAppTag = metadata.errorAppTag
		out[i].description = metadata.description
		out[i].reference = metadata.reference
	}
	return out, nil
}

type restrictionMetadataData struct {
	errorMessage, errorAppTag, description, reference string
}

func restrictionMetadata(st *yangparse.Statement) (restrictionMetadataData, error) {
	var out restrictionMetadataData
	var err error
	if out.errorMessage, err = constraintMetadataArg(st, "error-message"); err != nil {
		return restrictionMetadataData{}, err
	}
	if out.errorAppTag, err = constraintMetadataArg(st, "error-app-tag"); err != nil {
		return restrictionMetadataData{}, err
	}
	if out.description, err = constraintMetadataArg(st, "description"); err != nil {
		return restrictionMetadataData{}, err
	}
	if out.reference, err = constraintMetadataArg(st, "reference"); err != nil {
		return restrictionMetadataData{}, err
	}
	return out, nil
}

func ranges(expr, keyword string, base BaseType, fractionDigits uint8) ([]RangeBound, error) {
	expr = yangTrimSpace(expr)
	if expr == "" {
		return nil, fmt.Errorf("%s expression is empty", keyword)
	}
	parts := strings.Split(expr, "|")
	out := make([]RangeBound, 0, len(parts))
	prevMax := ""
	for _, part := range parts {
		part = yangTrimSpace(part)
		if part == "" {
			return nil, fmt.Errorf("%s expression %q contains an empty segment", keyword, expr)
		}
		if strings.Count(part, "..") > 1 {
			return nil, fmt.Errorf("%s expression %q has malformed segment %q", keyword, expr, part)
		}
		lo, hi, ok := strings.Cut(part, "..")
		if !ok {
			lo, hi = part, part
		}
		lo = yangTrimSpace(lo)
		hi = yangTrimSpace(hi)
		if lo == "" || hi == "" {
			return nil, fmt.Errorf("%s expression %q has missing bound in segment %q", keyword, expr, part)
		}
		min, err := normalizeBound(lo, keyword, base, fractionDigits)
		if err != nil {
			return nil, err
		}
		max, err := normalizeBound(hi, keyword, base, fractionDigits)
		if err != nil {
			return nil, err
		}
		if !boundsOrdered(keyword, base, min, max) {
			return nil, fmt.Errorf("%s segment %q has lower bound greater than upper bound", keyword, part)
		}
		if prevMax != "" {
			cmp, ok := compareBounds(keyword, base, prevMax, min)
			if !ok {
				return nil, fmt.Errorf("%s expression %q has uncomparable segment %q", keyword, expr, part)
			}
			if cmp >= 0 {
				return nil, fmt.Errorf("%s expression %q has overlapping or unordered segment %q", keyword, expr, part)
			}
		}
		out = append(out, RangeBound{
			min: min,
			max: max,
		})
		prevMax = max
	}
	return out, nil
}

func normalizeBound(s, keyword string, base BaseType, fractionDigits uint8) (string, error) {
	if keyword == "length" {
		if s == "min" || s == "max" {
			return s, nil
		}
		parsed, err := parseRangeUint(s, 64)
		if err != nil {
			return "", fmt.Errorf("invalid length bound %q", s)
		}
		return strconv.FormatUint(parsed, 10), nil
	}
	if isIntBase(base) {
		if s == "min" {
			return intMin(base), nil
		}
		if s == "max" {
			return intMax(base), nil
		}
		if isSignedIntBase(base) {
			parsed, err := strconv.ParseInt(s, 10, intBitSize(base))
			if err != nil {
				return "", fmt.Errorf("invalid range bound %q for %s", s, base.String())
			}
			return strconv.FormatInt(parsed, 10), nil
		}
		parsed, err := parseRangeUint(s, intBitSize(base))
		if err != nil {
			return "", fmt.Errorf("invalid range bound %q for %s", s, base.String())
		}
		return strconv.FormatUint(parsed, 10), nil
	}
	if base == BaseTypeDecimal64 {
		if s == "min" || s == "max" {
			return s, nil
		}
		if !validDecimal64Bound(s, fractionDigits) {
			return "", fmt.Errorf("invalid decimal64 range bound %q", s)
		}
		return formatDecimalBound(s, fractionDigits), nil
	}
	return s, nil
}

func boundsOrdered(keyword string, base BaseType, min, max string) bool {
	cmp, ok := compareBounds(keyword, base, min, max)
	return ok && cmp <= 0
}

func compareBounds(keyword string, base BaseType, left, right string) (int, bool) {
	switch {
	case left == right:
		return 0, true
	case left == "min" || right == "max":
		return -1, true
	case left == "max" || right == "min":
		return 1, true
	}
	if keyword == "length" || isIntBase(base) {
		lo, ok := new(big.Int).SetString(left, 10)
		if !ok {
			return 0, false
		}
		hi, ok := new(big.Int).SetString(right, 10)
		if !ok {
			return 0, false
		}
		return lo.Cmp(hi), true
	}
	if base == BaseTypeDecimal64 {
		lo, ok := new(big.Rat).SetString(left)
		if !ok {
			return 0, false
		}
		hi, ok := new(big.Rat).SetString(right)
		if !ok {
			return 0, false
		}
		return lo.Cmp(hi), true
	}
	return 0, true
}

func isSignedIntBase(base BaseType) bool {
	return base >= BaseTypeInt8 && base <= BaseTypeInt64
}

func intBitSize(base BaseType) int {
	switch base {
	case BaseTypeInt8, BaseTypeUint8:
		return 8
	case BaseTypeInt16, BaseTypeUint16:
		return 16
	case BaseTypeInt32, BaseTypeUint32:
		return 32
	default:
		return 64
	}
}

func validDecimal64Bound(s string, fractionDigits uint8) bool {
	if s == "" {
		return false
	}
	if s[0] == '+' || s[0] == '-' {
		s = s[1:]
	}
	if s == "" {
		return false
	}
	digits := 0
	fraction := 0
	seenDot := false
	integerDigits := 0
	for i := 0; i < len(s); i++ {
		ch := s[i]
		if ch == '.' {
			if seenDot || integerDigits == 0 {
				return false
			}
			seenDot = true
			continue
		}
		if ch < '0' || ch > '9' {
			return false
		}
		digits++
		if seenDot {
			fraction++
		} else {
			integerDigits++
		}
	}
	return digits > 0 && integerDigits > 0 && (!seenDot || fraction > 0) && fraction <= int(fractionDigits)
}

func formatDecimalBound(s string, fractionDigits uint8) string {
	if fractionDigits == 0 || strings.Contains(s, ".") {
		return s
	}
	return s + "." + strings.Repeat("0", int(fractionDigits))
}

func isIntBase(base BaseType) bool {
	return base >= BaseTypeInt8 && base <= BaseTypeUint64
}

func intMin(base BaseType) string {
	switch base {
	case BaseTypeInt8:
		return "-128"
	case BaseTypeInt16:
		return "-32768"
	case BaseTypeInt32:
		return "-2147483648"
	case BaseTypeInt64:
		return "-9223372036854775808"
	default:
		return "0"
	}
}

func intMax(base BaseType) string {
	switch base {
	case BaseTypeInt8:
		return "127"
	case BaseTypeInt16:
		return "32767"
	case BaseTypeInt32:
		return "2147483647"
	case BaseTypeInt64:
		return "9223372036854775807"
	case BaseTypeUint8:
		return "255"
	case BaseTypeUint16:
		return "65535"
	case BaseTypeUint32:
		return "4294967295"
	case BaseTypeUint64:
		return "18446744073709551615"
	default:
		return "0"
	}
}

func patterns(st *yangparse.Statement) ([]Pattern, error) {
	var out []Pattern
	for _, p := range direct(st, "pattern") {
		if err := validateYANGPatternSyntax(p.Argument); err != nil {
			return nil, fmt.Errorf("invalid pattern %q at %s: %w", p.Argument, p.Location(), err)
		}
		if err := validateNativeStringPattern(p.Argument); err != nil {
			return nil, fmt.Errorf("pattern %q does not compile for native full-string regexp checks at %s: %w", p.Argument, p.Location(), err)
		}
		var tag *string
		errorMessage, err := constraintMetadataArg(p, "error-message")
		if err != nil {
			return nil, err
		}
		if s, err := constraintMetadataArg(p, "error-app-tag"); err != nil {
			return nil, err
		} else if s != "" {
			tag = ptr(s)
		}
		description, err := constraintMetadataArg(p, "description")
		if err != nil {
			return nil, err
		}
		reference, err := constraintMetadataArg(p, "reference")
		if err != nil {
			return nil, err
		}
		inverted := false
		modifiers := direct(p, "modifier")
		if len(modifiers) > 1 {
			return nil, fmt.Errorf("pattern %q has multiple modifier statements at %s", p.Argument, modifiers[1].Location())
		}
		if len(modifiers) == 1 {
			if modifiers[0].Argument != "invert-match" {
				return nil, fmt.Errorf("invalid pattern modifier %q at %s", modifiers[0].Argument, modifiers[0].Location())
			}
			inverted = true
		}
		out = append(out, Pattern{
			regex:        p.Argument,
			errorMessage: errorMessage,
			appTag:       tag,
			description:  description,
			reference:    reference,
			inverted:     inverted,
		})
	}
	return out, nil
}

func validateYANGPatternSyntax(pattern string) error {
	// YANG uses XML Schema regex syntax, not Go regexp syntax. This structural
	// sanity check runs before the native-regexp compatibility gate below so
	// malformed YANG patterns fail with parser-oriented diagnostics.
	runes := []rune(pattern)
	parenDepth := 0
	canRepeat := false
	lastWasQuantifier := false
	for i := 0; i < len(runes); i++ {
		switch r := runes[i]; r {
		case '\\':
			if i == len(runes)-1 {
				return fmt.Errorf("trailing escape")
			}
			end, err := validateYANGPatternEscape(runes, i+1)
			if err != nil {
				return err
			}
			i = end
			canRepeat = true
			lastWasQuantifier = false
		case '[':
			end, err := validateYANGPatternClass(runes, i)
			if err != nil {
				return err
			}
			i = end
			canRepeat = true
			lastWasQuantifier = false
		case '(':
			parenDepth++
			canRepeat = false
			lastWasQuantifier = false
		case ')':
			if parenDepth == 0 {
				return fmt.Errorf("unmatched ')'")
			}
			parenDepth--
			canRepeat = true
			lastWasQuantifier = false
		case '*', '+', '?':
			if !canRepeat || lastWasQuantifier {
				return fmt.Errorf("invalid repeated quantifier")
			}
			canRepeat = false
			lastWasQuantifier = true
		case '{':
			end, ok, err := parseYANGPatternQuantifier(runes, i)
			if err != nil {
				return err
			}
			if !ok {
				canRepeat = true
				lastWasQuantifier = false
				continue
			}
			if !canRepeat || lastWasQuantifier {
				return fmt.Errorf("invalid quantifier")
			}
			i = end
			canRepeat = false
			lastWasQuantifier = true
		case '|':
			canRepeat = false
			lastWasQuantifier = false
		default:
			canRepeat = true
			lastWasQuantifier = false
		}
	}
	if parenDepth != 0 {
		return fmt.Errorf("unterminated group")
	}
	return nil
}

func validateNativeStringPattern(pattern string) error {
	if unsupported := xsdregex.UnsupportedNativeSyntax(pattern); unsupported != "" {
		return errors.New(unsupported)
	}
	_, err := regexp.Compile("^(?:" + xsdregex.NativePattern(pattern) + ")$")
	return err
}

type yangPatternClassFrame struct {
	items    int
	atStart  bool
	prev     rune
	havePrev bool
}

func validateYANGPatternClass(runes []rune, start int) (int, error) {
	frames := []yangPatternClassFrame{{atStart: true}}
	for i := start + 1; i < len(runes); i++ {
		frame := &frames[len(frames)-1]
		switch r := runes[i]; r {
		case '\\':
			if i == len(runes)-1 {
				return 0, fmt.Errorf("trailing escape")
			}
			end, err := validateYANGPatternEscape(runes, i+1)
			if err != nil {
				return 0, err
			}
			i = end
			frame.items++
			frame.atStart = false
			frame.havePrev = false
		case '[':
			frame.items++
			frame.atStart = false
			frame.havePrev = false
			frames = append(frames, yangPatternClassFrame{atStart: true})
		case ']':
			if frame.items == 0 {
				return 0, fmt.Errorf("empty character class")
			}
			frames = frames[:len(frames)-1]
			if len(frames) == 0 {
				return i, nil
			}
			frames[len(frames)-1].havePrev = false
		case '^':
			if frame.atStart {
				frame.atStart = false
				frame.havePrev = false
				continue
			}
			recordYANGPatternClassRune(frame, runes, i)
		default:
			if frame.havePrev && r == '-' && i+1 < len(runes) {
				next := runes[i+1]
				if next != ']' && next != '[' && next != '\\' && frame.prev > next {
					return 0, fmt.Errorf("reversed character range")
				}
			}
			recordYANGPatternClassRune(frame, runes, i)
		}
	}
	return 0, fmt.Errorf("unterminated character class")
}

func validateYANGPatternEscape(runes []rune, start int) (int, error) {
	switch r := runes[start]; r {
	case 'p', 'P':
		if start+1 >= len(runes) || runes[start+1] != '{' {
			return 0, fmt.Errorf("invalid character category escape")
		}
		end := start + 2
		for end < len(runes) && runes[end] != '}' {
			end++
		}
		if end >= len(runes) || end == start+2 {
			return 0, fmt.Errorf("invalid character category escape")
		}
		if !validYANGPatternCategory(string(runes[start+2 : end])) {
			return 0, fmt.Errorf("unknown character category")
		}
		return end, nil
	case 'c', 'C', 'd', 'D', 'i', 'I', 'n', 'r', 's', 'S', 't', 'w', 'W':
		return start, nil
	default:
		if isYANGPatternEscapedChar(r) {
			return start, nil
		}
		return 0, fmt.Errorf("unrecognized escape")
	}
}

var yangPatternCategories = map[string]struct{}{
	"C":  {},
	"Cc": {},
	"Cf": {},
	"Cn": {},
	"Co": {},
	"Cs": {},
	"L":  {},
	"Ll": {},
	"Lm": {},
	"Lo": {},
	"Lt": {},
	"Lu": {},
	"M":  {},
	"Mc": {},
	"Me": {},
	"Mn": {},
	"N":  {},
	"Nd": {},
	"Nl": {},
	"No": {},
	"P":  {},
	"Pc": {},
	"Pd": {},
	"Pe": {},
	"Pf": {},
	"Pi": {},
	"Po": {},
	"Ps": {},
	"S":  {},
	"Sc": {},
	"Sk": {},
	"Sm": {},
	"So": {},
	"Z":  {},
	"Zl": {},
	"Zp": {},
	"Zs": {},
}

var yangPatternUnicodeBlocks = map[string]struct{}{
	"BasicLatin":                         {},
	"Latin-1Supplement":                  {},
	"LatinExtended-A":                    {},
	"LatinExtended-B":                    {},
	"IPAExtensions":                      {},
	"SpacingModifierLetters":             {},
	"CombiningDiacriticalMarks":          {},
	"Greek":                              {},
	"Cyrillic":                           {},
	"Armenian":                           {},
	"Hebrew":                             {},
	"Arabic":                             {},
	"Syriac":                             {},
	"Thaana":                             {},
	"Devanagari":                         {},
	"Bengali":                            {},
	"Gurmukhi":                           {},
	"Gujarati":                           {},
	"Oriya":                              {},
	"Tamil":                              {},
	"Telugu":                             {},
	"Kannada":                            {},
	"Malayalam":                          {},
	"Sinhala":                            {},
	"Thai":                               {},
	"Lao":                                {},
	"Tibetan":                            {},
	"Myanmar":                            {},
	"Georgian":                           {},
	"HangulJamo":                         {},
	"Ethiopic":                           {},
	"Cherokee":                           {},
	"UnifiedCanadianAboriginalSyllabics": {},
	"Ogham":                              {},
	"Runic":                              {},
	"Khmer":                              {},
	"Mongolian":                          {},
	"LatinExtendedAdditional":            {},
	"GreekExtended":                      {},
	"GeneralPunctuation":                 {},
	"SuperscriptsandSubscripts":          {},
	"CurrencySymbols":                    {},
	"CombiningMarksforSymbols":           {},
	"LetterlikeSymbols":                  {},
	"NumberForms":                        {},
	"Arrows":                             {},
	"MathematicalOperators":              {},
	"MiscellaneousTechnical":             {},
	"ControlPictures":                    {},
	"OpticalCharacterRecognition":        {},
	"EnclosedAlphanumerics":              {},
	"BoxDrawing":                         {},
	"BlockElements":                      {},
	"GeometricShapes":                    {},
	"MiscellaneousSymbols":               {},
	"Dingbats":                           {},
	"BraillePatterns":                    {},
	"CJKRadicalsSupplement":              {},
	"KangxiRadicals":                     {},
	"IdeographicDescriptionCharacters":   {},
	"CJKSymbolsandPunctuation":           {},
	"Hiragana":                           {},
	"Katakana":                           {},
	"Bopomofo":                           {},
	"HangulCompatibilityJamo":            {},
	"Kanbun":                             {},
	"BopomofoExtended":                   {},
	"EnclosedCJKLettersandMonths":        {},
	"CJKCompatibility":                   {},
	"CJKUnifiedIdeographsExtensionA":     {},
	"CJKUnifiedIdeographs":               {},
	"YiSyllables":                        {},
	"YiRadicals":                         {},
	"HangulSyllables":                    {},
	"PrivateUse":                         {},
	"CJKCompatibilityIdeographs":         {},
	"AlphabeticPresentationForms":        {},
	"ArabicPresentationForms-A":          {},
	"CombiningHalfMarks":                 {},
	"CJKCompatibilityForms":              {},
	"SmallFormVariants":                  {},
	"ArabicPresentationForms-B":          {},
	"HalfwidthandFullwidthForms":         {},
	"Specials":                           {},
}

func validYANGPatternCategory(category string) bool {
	if _, ok := yangPatternCategories[category]; ok {
		return true
	}
	if !strings.HasPrefix(category, "Is") {
		return false
	}
	_, ok := yangPatternUnicodeBlocks[category[len("Is"):]]
	return ok
}

func isYANGPatternEscapedChar(r rune) bool {
	switch r {
	case '\\', '|', '-', '.', '?', '*', '+', '{', '}', '(', ')', '[', ']', '^', '$':
		return true
	default:
		return false
	}
}

func recordYANGPatternClassRune(frame *yangPatternClassFrame, runes []rune, i int) {
	frame.items++
	frame.atStart = false
	frame.prev = runes[i]
	frame.havePrev = true
}

func parseYANGPatternQuantifier(runes []rune, start int) (int, bool, error) {
	i := start + 1
	if i >= len(runes) || !isYANGPatternDigit(runes[i]) {
		return 0, false, nil
	}
	minStart := i
	for i < len(runes) && isYANGPatternDigit(runes[i]) {
		i++
	}
	min := string(runes[minStart:i])
	if i >= len(runes) {
		return 0, false, nil
	}
	if runes[i] == '}' {
		return i, true, nil
	}
	if runes[i] != ',' {
		return 0, false, nil
	}
	i++
	if i >= len(runes) {
		return 0, false, nil
	}
	if runes[i] == '}' {
		return i, true, nil
	}
	maxStart := i
	if !isYANGPatternDigit(runes[i]) {
		return 0, false, nil
	}
	for i < len(runes) && isYANGPatternDigit(runes[i]) {
		i++
	}
	if i >= len(runes) || runes[i] != '}' {
		return 0, false, nil
	}
	maximum := string(runes[maxStart:i])
	if compareYANGPatternDecimal(min, maximum) > 0 {
		return 0, true, fmt.Errorf("reversed quantifier range")
	}
	return i, true, nil
}

func isYANGPatternDigit(r rune) bool {
	return r >= '0' && r <= '9'
}

func compareYANGPatternDecimal(a, b string) int {
	a = strings.TrimLeft(a, "0")
	b = strings.TrimLeft(b, "0")
	if a == "" {
		a = "0"
	}
	if b == "" {
		b = "0"
	}
	if len(a) < len(b) {
		return -1
	}
	if len(a) > len(b) {
		return 1
	}
	if a < b {
		return -1
	}
	if a > b {
		return 1
	}
	return 0
}

func (m *moduleData) enumValues(st *yangparse.Statement, keyword, valueKeyword string) ([]EnumValue, error) {
	const (
		enumMin = -1 << 31
		enumMax = 1<<31 - 1
		bitMax  = 1<<32 - 1
	)
	var out []EnumValue
	next := int64(0)
	seenNames := make(map[string]*yangparse.Statement)
	seenValues := make(map[int64]*yangparse.Statement)
	for _, ev := range direct(st, keyword) {
		if err := validateEnumBitMetadata(keyword, ev); err != nil {
			return nil, err
		}
		if prev := seenNames[ev.Argument]; prev != nil {
			return nil, fmt.Errorf("duplicate %s name %q at %s; previous definition at %s", keyword, ev.Argument, ev.Location(), prev.Location())
		}
		seenNames[ev.Argument] = ev
		val := next
		valueStatements := direct(ev, valueKeyword)
		if len(valueStatements) > 1 {
			return nil, fmt.Errorf("%s %q has multiple %s statements at %s", keyword, ev.Argument, valueKeyword, valueStatements[1].Location())
		}
		if len(valueStatements) == 1 {
			raw := valueStatements[0].Argument
			switch keyword {
			case "enum":
				value, ok := parseYANGInt32(raw)
				if !ok {
					return nil, fmt.Errorf("enum %q at %s has invalid value %q", ev.Argument, valueStatements[0].Location(), raw)
				}
				val = value
			case "bit":
				value, ok := parseUint32(raw)
				if !ok {
					return nil, fmt.Errorf("bit %q at %s has invalid position %q", ev.Argument, valueStatements[0].Location(), raw)
				}
				val = int64(value)
			default:
				parsed, err := strconv.ParseInt(raw, 10, 64)
				if err != nil {
					return nil, fmt.Errorf("%s %q at %s has invalid %s %q", keyword, ev.Argument, valueStatements[0].Location(), valueKeyword, raw)
				}
				val = parsed
			}
		} else {
			switch keyword {
			case "enum":
				if val < enumMin || val > enumMax {
					return nil, fmt.Errorf("enum %q at %s auto value %d outside int32 range", ev.Argument, ev.Location(), val)
				}
			case "bit":
				if val < 0 || val > bitMax {
					return nil, fmt.Errorf("bit %q at %s auto position %d outside uint32 range", ev.Argument, ev.Location(), val)
				}
			}
		}
		if prev := seenValues[val]; prev != nil {
			return nil, fmt.Errorf("duplicate %s %s %d at %s; previous definition at %s", keyword, valueKeyword, val, ev.Location(), prev.Location())
		}
		seenValues[val] = ev
		// Keep auto-assignment counters moving for gated values so the
		// positions of later ungated values match the schema declaration order.
		if m.featureIncluded(ev) {
			out = append(out, EnumValue{
				name:        ev.Argument,
				value:       val,
				description: childArg(ev, "description"),
				reference:   childArg(ev, "reference"),
				status:      statusFromStatement(ev),
				ifFeatures:  ifFeatureArgs(ev),
				conditional: len(direct(ev, "if-feature")) > 0,
			})
		}
		next = val + 1
	}
	return out, nil
}

func validateEnumBitMetadata(kind string, st *yangparse.Statement) error {
	name := st.Argument
	if _, err := singletonDefinitionArg(kind, name, st, "description"); err != nil {
		return err
	}
	if _, err := singletonDefinitionArg(kind, name, st, "reference"); err != nil {
		return err
	}
	return validateDefinitionStatus(kind, name, st)
}

func (m *moduleData) extensionInstances(st *yangparse.Statement) []Extension {
	var out []Extension
	for _, child := range st.SubStatements() {
		ext, ok := m.extensionInstance(child, true)
		if ok {
			out = append(out, ext)
		}
	}
	return out
}

func (m *moduleData) topLevelExtensionInstances() []Extension {
	var out []Extension
	for _, st := range m.sourceTopStatements() {
		ext, ok := m.extensionInstance(st, false)
		if ok {
			out = append(out, ext)
		}
	}
	return out
}

func (m *moduleData) extensionInstance(child *yangparse.Statement, recordError bool) (Extension, bool) {
	if child == nil {
		return Extension{}, false
	}
	pfx, local, ok := strings.Cut(child.Keyword, ":")
	if !ok {
		return Extension{}, false
	}
	mod := m.extensionModuleForPrefix(pfx)
	var def *yangparse.Statement
	if mod != nil {
		def = mod.extDefs[local]
	}
	if mod == m && def != nil && !m.definitionVisibleFrom(def, child) {
		def = nil
	}
	if def == nil {
		if recordError {
			m.recordSchemaError(fmt.Errorf("unknown extension %q at %s", child.Keyword, child.Location()))
		}
		return Extension{}, false
	}
	if !m.featureIncluded(child) {
		return Extension{}, false
	}
	var arg *string
	if child.HasArgument {
		arg = ptr(child.Argument)
	}
	return Extension{name: local, argument: arg, moduleName: mod.name, ifFeatures: ifFeatureArgs(child)}, true
}

func (m *moduleData) validateExtensionInstances() error {
	for _, top := range m.sourceTopStatements() {
		var err error
		walkStatements(top, func(st *yangparse.Statement) {
			if err != nil {
				return
			}
			pfx, local, ok := strings.Cut(st.Keyword, ":")
			if !ok {
				return
			}
			mod := m.extensionModuleForPrefix(pfx)
			def := (*yangparse.Statement)(nil)
			if mod != nil {
				def = mod.extDefs[local]
			}
			if mod == m && def != nil && !m.definitionVisibleFrom(def, st) {
				def = nil
			}
			if def == nil {
				err = fmt.Errorf("unknown extension %q at %s", st.Keyword, st.Location())
				return
			}
			if argErr := validateExtensionInstanceArgument(st, def); argErr != nil {
				err = argErr
			}
		})
		if err != nil {
			return err
		}
	}
	return nil
}

func validateExtensionInstanceArgument(instance, def *yangparse.Statement) error {
	if instance == nil || def == nil {
		return nil
	}
	arg := first(def, "argument")
	switch {
	case arg != nil && !instance.HasArgument:
		return fmt.Errorf("extension %q requires an argument at %s", instance.Keyword, instance.Location())
	case arg == nil && instance.HasArgument:
		return fmt.Errorf("extension %q does not accept an argument at %s", instance.Keyword, instance.Location())
	default:
		return nil
	}
}

func (m *moduleData) extensionModuleForPrefix(prefix string) *moduleData {
	if prefix == m.prefix {
		return m
	}
	return m.importByPfx[prefix]
}

func (m *moduleData) validateXPathExpressionPrefixes(kind string, st *yangparse.Statement) error {
	if m == nil || st == nil {
		return nil
	}
	for _, prefix := range referencedPrefixes(st.Argument) {
		if m.resolveSourceQNameModule(prefix+":_") == nil {
			return fmt.Errorf("unknown prefix %q in %s expression %q at %s", prefix, kind, st.Argument, st.Location())
		}
	}
	return nil
}

func mustFromValidated(st *yangparse.Statement) (MustConstraint, error) {
	if st == nil {
		return MustConstraint{}, nil
	}
	if err := validateXPathExpressionStatement("must", st); err != nil {
		return MustConstraint{}, err
	}
	errorMessage, err := constraintMetadataArg(st, "error-message")
	if err != nil {
		return MustConstraint{}, err
	}
	errorAppTag, err := constraintMetadataArg(st, "error-app-tag")
	if err != nil {
		return MustConstraint{}, err
	}
	description, err := constraintMetadataArg(st, "description")
	if err != nil {
		return MustConstraint{}, err
	}
	reference, err := constraintMetadataArg(st, "reference")
	if err != nil {
		return MustConstraint{}, err
	}
	return MustConstraint{
		cond:         st.Argument,
		errorMessage: errorMessage,
		errorAppTag:  errorAppTag,
		description:  description,
		reference:    reference,
	}, nil
}

func whenFromValidated(st *yangparse.Statement) (WhenConstraint, error) {
	if st == nil {
		return WhenConstraint{}, nil
	}
	if err := validateXPathExpressionStatement("when", st); err != nil {
		return WhenConstraint{}, err
	}
	description, err := constraintMetadataArg(st, "description")
	if err != nil {
		return WhenConstraint{}, err
	}
	reference, err := constraintMetadataArg(st, "reference")
	if err != nil {
		return WhenConstraint{}, err
	}
	return WhenConstraint{cond: st.Argument, description: description, reference: reference}, nil
}

func validateXPathExpressionStatement(keyword string, st *yangparse.Statement) error {
	if strings.TrimSpace(st.Argument) == "" {
		return fmt.Errorf("%s expression is empty at %s", keyword, st.Location())
	}
	if !structurallyValidXPathExpression(st.Argument) {
		return fmt.Errorf("invalid %s expression %q at %s", keyword, st.Argument, st.Location())
	}
	return nil
}

func structurallyValidXPathExpression(expr string) bool {
	brackets := 0
	parens := 0
	var quote rune
	for _, r := range expr {
		if quote != 0 {
			if r == quote {
				quote = 0
			}
			continue
		}
		switch r {
		case '\'', '"':
			quote = r
		case '[':
			brackets++
		case ']':
			if brackets == 0 {
				return false
			}
			brackets--
		case '(':
			parens++
		case ')':
			if parens == 0 {
				return false
			}
			parens--
		}
	}
	return quote == 0 && brackets == 0 && parens == 0 && lexicallyValidXPathExpression(expr)
}

func lexicallyValidXPathExpression(expr string) bool {
	return xpathAxesAreKnown(expr) && xpathFunctionsHaveKnownArity(expr) && xpathBinaryOperatorsHaveOperands(expr)
}

var xpathKnownAxes = map[string]struct{}{
	"ancestor":           {},
	"ancestor-or-self":   {},
	"attribute":          {},
	"child":              {},
	"descendant":         {},
	"descendant-or-self": {},
	"following":          {},
	"following-sibling":  {},
	"namespace":          {},
	"parent":             {},
	"preceding":          {},
	"preceding-sibling":  {},
	"self":               {},
}

func xpathAxesAreKnown(expr string) bool {
	for i := 0; i < len(expr); i++ {
		if expr[i] == '\'' || expr[i] == '"' {
			next, ok := skipXPathQuoted(expr, i)
			if !ok {
				return false
			}
			i = next
			continue
		}
		if i+1 >= len(expr) || expr[i] != ':' || expr[i+1] != ':' {
			continue
		}
		start := i - 1
		for start >= 0 && isXPathNameChar(expr[start]) {
			start--
		}
		axis := expr[start+1 : i]
		if axis == "" {
			return false
		}
		if _, ok := xpathKnownAxes[axis]; !ok {
			return false
		}
		i++
	}
	return true
}

type xpathFunctionArity struct {
	min int
	max int
}

const xpathVariadic = -1

var xpathFunctionArities = map[string]xpathFunctionArity{
	"last":                 {min: 0, max: 0},
	"position":             {min: 0, max: 0},
	"count":                {min: 1, max: 1},
	"id":                   {min: 1, max: 1},
	"local-name":           {min: 0, max: 1},
	"namespace-uri":        {min: 0, max: 1},
	"name":                 {min: 0, max: 1},
	"string":               {min: 0, max: 1},
	"concat":               {min: 2, max: xpathVariadic},
	"starts-with":          {min: 2, max: 2},
	"contains":             {min: 2, max: 2},
	"substring-before":     {min: 2, max: 2},
	"substring-after":      {min: 2, max: 2},
	"substring":            {min: 2, max: 3},
	"string-length":        {min: 0, max: 1},
	"normalize-space":      {min: 0, max: 1},
	"translate":            {min: 3, max: 3},
	"boolean":              {min: 1, max: 1},
	"not":                  {min: 1, max: 1},
	"true":                 {min: 0, max: 0},
	"false":                {min: 0, max: 0},
	"lang":                 {min: 1, max: 1},
	"number":               {min: 0, max: 1},
	"sum":                  {min: 1, max: 1},
	"floor":                {min: 1, max: 1},
	"ceiling":              {min: 1, max: 1},
	"round":                {min: 1, max: 1},
	"node":                 {min: 0, max: 0},
	"text":                 {min: 0, max: 0},
	"current":              {min: 0, max: 0},
	"deref":                {min: 1, max: 1},
	"derived-from":         {min: 2, max: 2},
	"derived-from-or-self": {min: 2, max: 2},
	"enum-value":           {min: 1, max: 1},
	"bit-is-set":           {min: 2, max: 2},
	"re-match":             {min: 2, max: 2},
}

func xpathFunctionsHaveKnownArity(expr string) bool {
	for i := 0; i < len(expr); {
		if expr[i] == '\'' || expr[i] == '"' {
			next, ok := skipXPathQuoted(expr, i)
			if !ok {
				return false
			}
			i = next + 1
			continue
		}
		if !isXPathNameStart(expr[i]) {
			i++
			continue
		}
		start := i
		i++
		for i < len(expr) && isXPathNameChar(expr[i]) {
			i++
		}
		name := expr[start:i]
		j := i
		for j < len(expr) && isYANGSpace(expr[j]) {
			j++
		}
		if j >= len(expr) || expr[j] != '(' {
			continue
		}
		arity, known := xpathFunctionArities[name]
		if !known {
			return false
		}
		end, ok := matchingXPathParen(expr, j)
		if !ok {
			return false
		}
		count, ok := countXPathFunctionArgs(expr[j+1 : end])
		if !ok {
			return false
		}
		if count < arity.min {
			return false
		}
		if arity.max != xpathVariadic && count > arity.max {
			return false
		}
	}
	return true
}

func countXPathFunctionArgs(args string) (int, bool) {
	if strings.TrimSpace(args) == "" {
		return 0, true
	}
	count := 1
	parens := 0
	brackets := 0
	segmentHasContent := false
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case '\'', '"':
			next, ok := skipXPathQuoted(args, i)
			if !ok {
				return 0, false
			}
			segmentHasContent = true
			i = next
		case '(':
			parens++
			segmentHasContent = true
		case ')':
			if parens == 0 {
				return 0, false
			}
			parens--
			segmentHasContent = true
		case '[':
			brackets++
			segmentHasContent = true
		case ']':
			if brackets == 0 {
				return 0, false
			}
			brackets--
			segmentHasContent = true
		case ',':
			if parens != 0 || brackets != 0 {
				segmentHasContent = true
				continue
			}
			if !segmentHasContent {
				return 0, false
			}
			count++
			segmentHasContent = false
		default:
			if !isYANGSpace(args[i]) {
				segmentHasContent = true
			}
		}
	}
	if parens != 0 || brackets != 0 || !segmentHasContent {
		return 0, false
	}
	return count, true
}

type xpathSyntaxTokenKind int

const (
	xpathSyntaxOperand xpathSyntaxTokenKind = iota
	xpathSyntaxOperator
)

type xpathSyntaxToken struct {
	kind xpathSyntaxTokenKind
}

func xpathBinaryOperatorsHaveOperands(expr string) bool {
	tokens, ok := xpathSyntaxTokens(expr)
	if !ok || len(tokens) == 0 {
		return false
	}
	expectOperand := true
	seenOperand := false
	for _, token := range tokens {
		switch token.kind {
		case xpathSyntaxOperand:
			expectOperand = false
			seenOperand = true
		case xpathSyntaxOperator:
			if expectOperand {
				return false
			}
			expectOperand = true
		}
	}
	return seenOperand && !expectOperand
}

func xpathSyntaxTokens(expr string) ([]xpathSyntaxToken, bool) {
	var tokens []xpathSyntaxToken
	for i := 0; i < len(expr); {
		ch := expr[i]
		switch {
		case isYANGSpace(ch):
			i++
		case ch == '\'' || ch == '"':
			next, ok := skipXPathQuoted(expr, i)
			if !ok {
				return nil, false
			}
			tokens = append(tokens, xpathSyntaxToken{kind: xpathSyntaxOperand})
			i = next + 1
		case ch == '(' || ch == ')' || ch == '[' || ch == ']' || ch == ',':
			i++
		case isXPathSymbolicOperatorStart(ch):
			if ch == '!' && (i+1 >= len(expr) || expr[i+1] != '=') {
				return nil, false
			}
			tokens = append(tokens, xpathSyntaxToken{kind: xpathSyntaxOperator})
			if i+1 < len(expr) && (ch == '!' || ch == '<' || ch == '>') && expr[i+1] == '=' {
				i += 2
			} else {
				i++
			}
		case isXPathNameStart(ch):
			start := i
			i++
			for i < len(expr) && isXPathNameChar(expr[i]) {
				i++
			}
			if isXPathWordOperator(expr[start:i]) {
				tokens = append(tokens, xpathSyntaxToken{kind: xpathSyntaxOperator})
			} else {
				tokens = append(tokens, xpathSyntaxToken{kind: xpathSyntaxOperand})
			}
		default:
			start := i
			for i < len(expr) && !isYANGSpace(expr[i]) && expr[i] != '\'' && expr[i] != '"' &&
				expr[i] != '(' && expr[i] != ')' && expr[i] != '[' && expr[i] != ']' &&
				expr[i] != ',' && !isXPathSymbolicOperatorStart(expr[i]) {
				i++
			}
			if i == start {
				return nil, false
			}
			tokens = append(tokens, xpathSyntaxToken{kind: xpathSyntaxOperand})
		}
	}
	return tokens, true
}

func skipXPathQuoted(expr string, start int) (int, bool) {
	quote := expr[start]
	for i := start + 1; i < len(expr); i++ {
		if expr[i] == quote {
			return i, true
		}
	}
	return 0, false
}

func matchingXPathParen(expr string, start int) (int, bool) {
	depth := 0
	for i := start; i < len(expr); i++ {
		switch expr[i] {
		case '\'', '"':
			next, ok := skipXPathQuoted(expr, i)
			if !ok {
				return 0, false
			}
			i = next
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return i, true
			}
			if depth < 0 {
				return 0, false
			}
		}
	}
	return 0, false
}

func isXPathSymbolicOperatorStart(ch byte) bool {
	switch ch {
	case '=', '<', '>', '!', '+', '|':
		return true
	default:
		return false
	}
}

func isXPathWordOperator(word string) bool {
	switch word {
	case "and", "or", "div", "mod":
		return true
	default:
		return false
	}
}

func isXPathNameStart(ch byte) bool {
	return ch >= 'A' && ch <= 'Z' || ch >= 'a' && ch <= 'z' || ch == '_'
}

func isXPathNameChar(ch byte) bool {
	return isXPathNameStart(ch) || ch >= '0' && ch <= '9' || ch == '-' || ch == '.' || ch == ':'
}

func constraintMetadataArg(st *yangparse.Statement, keyword string) (string, error) {
	children := direct(st, keyword)
	if len(children) == 0 {
		return "", nil
	}
	if len(children) > 1 {
		return "", fmt.Errorf("%s %q has multiple %s statements at %s", st.Keyword, st.Argument, keyword, children[1].Location())
	}
	return children[0].Argument, nil
}

func kindForKeyword(keyword string) SchemaNodeKind {
	switch keyword {
	case "container":
		return SchemaNodeKindContainer
	case "leaf":
		return SchemaNodeKindLeaf
	case "leaf-list":
		return SchemaNodeKindLeafList
	case "list":
		return SchemaNodeKindList
	case "choice":
		return SchemaNodeKindChoice
	case "case":
		return SchemaNodeKindCase
	case "anydata", "anyxml":
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

func isSchemaChildKeyword(keyword string) bool {
	switch keyword {
	case "container", "leaf", "leaf-list", "list", "choice", "case", "anydata", "anyxml", "action", "notification":
		return true
	default:
		return false
	}
}

func first(st *yangparse.Statement, keyword string) *yangparse.Statement {
	if st == nil {
		return nil
	}
	for _, c := range st.SubStatements() {
		if c.Keyword == keyword {
			return c
		}
	}
	return nil
}

func direct(st *yangparse.Statement, keyword string) []*yangparse.Statement {
	if st == nil {
		return nil
	}
	var out []*yangparse.Statement
	for _, c := range st.SubStatements() {
		if c.Keyword == keyword {
			out = append(out, c)
		}
	}
	return out
}

func nonExtensionSubStatements(st *yangparse.Statement) []*yangparse.Statement {
	if st == nil {
		return nil
	}
	var out []*yangparse.Statement
	for _, c := range st.SubStatements() {
		if !hasPrefix(c.Keyword) {
			out = append(out, c)
		}
	}
	return out
}

func childArg(st *yangparse.Statement, keyword string) string {
	if child := first(st, keyword); child != nil {
		return child.Argument
	}
	return ""
}

func ifFeatureArgs(st *yangparse.Statement) []string {
	children := direct(st, "if-feature")
	if len(children) == 0 {
		return nil
	}
	out := make([]string, len(children))
	for i, child := range children {
		out[i] = child.Argument
	}
	return out
}

// featureIncluded reports whether st should be included in the pure-Go schema
// IR for this module's enabled feature set. Features are disabled by default;
// multiple if-feature statements on the same target are ANDed together.
func (m *moduleData) featureIncluded(st *yangparse.Statement) bool {
	if st == nil {
		return true
	}
	for _, iff := range direct(st, "if-feature") {
		included, ok := m.evalIfFeatureExprFrom(iff.Argument, iff)
		if !ok {
			return false
		}
		if !included {
			return false
		}
	}
	return true
}

func (m *moduleData) validateIfFeatureExpressions() {
	if m == nil || m.schemaErr != nil {
		return
	}
	for _, top := range m.sourceTopStatements() {
		walkStatements(top, func(st *yangparse.Statement) {
			if m.schemaErr != nil {
				return
			}
			for _, iff := range direct(st, "if-feature") {
				if !m.validateIfFeatureExprFrom(iff.Argument, iff) {
					m.recordSchemaError(fmt.Errorf("invalid or unresolved if-feature expression %q at %s", iff.Argument, iff.Location()))
					return
				}
			}
		})
	}
}

func (m *moduleData) evalIfFeatureExpr(expr string) (bool, bool) {
	return m.evalIfFeatureExprFrom(expr, nil)
}

func (m *moduleData) evalIfFeatureExprFrom(expr string, from *yangparse.Statement) (bool, bool) {
	return m.evalIfFeatureExprSeen(expr, from, make(map[string]bool))
}

func (m *moduleData) evalIfFeatureExprSeen(expr string, from *yangparse.Statement, resolving map[string]bool) (bool, bool) {
	if ref, ok := m.yang10SingleIfFeatureRef(expr, from); ok {
		return m.featureEnabledSeen(ref, from, resolving)
	}
	tokens, ok := tokenizeIfFeatureExpr(expr)
	if !ok {
		return false, false
	}
	parser := ifFeatureParser{mod: m, from: from, tokens: tokens, resolving: resolving}
	value, ok := parser.parseOr()
	if !ok || parser.pos != len(tokens) {
		return false, false
	}
	return value, true
}

func (m *moduleData) validateIfFeatureExprFrom(expr string, from *yangparse.Statement) bool {
	if ref, ok := m.yang10SingleIfFeatureRef(expr, from); ok {
		return m.validateFeatureRefSeen(ref, from, make(map[string]bool))
	}
	tokens, ok := tokenizeIfFeatureExpr(expr)
	if !ok {
		return false
	}
	parser := ifFeatureParser{mod: m, from: from, tokens: tokens, resolving: make(map[string]bool), validateOnly: true}
	_, ok = parser.parseOr()
	return ok && parser.pos == len(tokens)
}

type ifFeatureTokenKind int

const (
	ifFeatureTokenIdent ifFeatureTokenKind = iota
	ifFeatureTokenNot
	ifFeatureTokenAnd
	ifFeatureTokenOr
	ifFeatureTokenLParen
	ifFeatureTokenRParen
)

type ifFeatureToken struct {
	kind ifFeatureTokenKind
	text string
}

func tokenizeIfFeatureExpr(expr string) ([]ifFeatureToken, bool) {
	expr = yangTrimSpace(expr)
	if expr == "" {
		return nil, false
	}
	var tokens []ifFeatureToken
	for i := 0; i < len(expr); {
		ch := expr[i]
		switch ch {
		case ' ', '\t', '\r', '\n':
			i++
		case '(':
			tokens = append(tokens, ifFeatureToken{kind: ifFeatureTokenLParen})
			i++
		case ')':
			tokens = append(tokens, ifFeatureToken{kind: ifFeatureTokenRParen})
			i++
		default:
			start := i
			for i < len(expr) && !isIfFeatureExprSeparator(expr[i]) {
				i++
			}
			word := expr[start:i]
			switch word {
			case "not":
				tokens = append(tokens, ifFeatureToken{kind: ifFeatureTokenNot})
			case "and":
				tokens = append(tokens, ifFeatureToken{kind: ifFeatureTokenAnd})
			case "or":
				tokens = append(tokens, ifFeatureToken{kind: ifFeatureTokenOr})
			default:
				if !validIfFeatureRef(word) {
					return nil, false
				}
				tokens = append(tokens, ifFeatureToken{kind: ifFeatureTokenIdent, text: word})
			}
		}
	}
	if len(tokens) == 0 {
		return nil, false
	}
	return tokens, true
}

func (m *moduleData) yang10SingleIfFeatureRef(expr string, from *yangparse.Statement) (string, bool) {
	if m == nil || from == nil || m.yangVersionForStatement(from) == "1.1" {
		return "", false
	}
	return singleIfFeatureRefArg(expr, false)
}

func singleIfFeatureRefArg(expr string, allowXMLPrefix bool) (string, bool) {
	ref := yangTrimSpace(expr)
	if !validYangIdentifierRef(ref, allowXMLPrefix) {
		return "", false
	}
	return ref, true
}

func isIfFeatureExprSeparator(ch byte) bool {
	return ch == ' ' || ch == '\t' || ch == '\r' || ch == '\n' || ch == '(' || ch == ')'
}

func validIfFeatureRef(ref string) bool {
	if ref == "" || strings.Count(ref, ":") > 1 {
		return false
	}
	parts := strings.Split(ref, ":")
	for _, part := range parts {
		if part == "" {
			return false
		}
		for i := 0; i < len(part); i++ {
			ch := part[i]
			if ch >= 'a' && ch <= 'z' || ch >= 'A' && ch <= 'Z' || ch >= '0' && ch <= '9' || ch == '_' || ch == '-' || ch == '.' {
				continue
			}
			return false
		}
	}
	return true
}

type ifFeatureParser struct {
	mod          *moduleData
	from         *yangparse.Statement
	tokens       []ifFeatureToken
	pos          int
	resolving    map[string]bool
	validateOnly bool
}

func (p *ifFeatureParser) parseOr() (bool, bool) {
	left, ok := p.parseAnd()
	if !ok {
		return false, false
	}
	for p.peek(ifFeatureTokenOr) {
		p.pos++
		right, ok := p.parseAnd()
		if !ok {
			return false, false
		}
		left = left || right
	}
	return left, true
}

func (p *ifFeatureParser) parseAnd() (bool, bool) {
	left, ok := p.parseNot()
	if !ok {
		return false, false
	}
	for p.peek(ifFeatureTokenAnd) {
		p.pos++
		right, ok := p.parseNot()
		if !ok {
			return false, false
		}
		left = left && right
	}
	return left, true
}

func (p *ifFeatureParser) parseNot() (bool, bool) {
	if p.peek(ifFeatureTokenNot) {
		p.pos++
		value, ok := p.parseNot()
		return !value, ok
	}
	return p.parsePrimary()
}

func (p *ifFeatureParser) parsePrimary() (bool, bool) {
	if p.pos >= len(p.tokens) {
		return false, false
	}
	tok := p.tokens[p.pos]
	switch tok.kind {
	case ifFeatureTokenIdent:
		p.pos++
		if p.validateOnly {
			return true, p.mod.validateFeatureRefSeen(tok.text, p.from, p.resolving)
		}
		return p.mod.featureEnabledSeen(tok.text, p.from, p.resolving)
	case ifFeatureTokenLParen:
		p.pos++
		value, ok := p.parseOr()
		if !ok || !p.peek(ifFeatureTokenRParen) {
			return false, false
		}
		p.pos++
		return value, true
	default:
		return false, false
	}
}

func (p *ifFeatureParser) peek(kind ifFeatureTokenKind) bool {
	return p.pos < len(p.tokens) && p.tokens[p.pos].kind == kind
}

func (m *moduleData) validateFeatureRefSeen(qname string, from *yangparse.Statement, resolving map[string]bool) bool {
	if m == nil {
		return false
	}
	mod := m.resolveSourceQNameModule(qname)
	if mod == nil {
		return false
	}
	local := localName(qname)
	feature := mod.featureMap[local]
	if feature == nil {
		return false
	}
	if mod == m && !m.definitionVisibleFrom(feature.stmt, from) {
		return false
	}
	key := moduleKey(mod.name, local)
	if resolving[key] {
		return false
	}
	resolving[key] = true
	defer delete(resolving, key)
	for _, iff := range direct(feature.stmt, "if-feature") {
		if !mod.validateIfFeatureExprSeen(iff.Argument, iff, resolving) {
			return false
		}
	}
	return true
}

func (m *moduleData) validateIfFeatureExprSeen(expr string, from *yangparse.Statement, resolving map[string]bool) bool {
	tokens, ok := tokenizeIfFeatureExpr(expr)
	if !ok {
		return false
	}
	parser := ifFeatureParser{mod: m, from: from, tokens: tokens, resolving: resolving, validateOnly: true}
	_, ok = parser.parseOr()
	return ok && parser.pos == len(tokens)
}

func (m *moduleData) featureEnabled(qname string) bool {
	enabled, ok := m.featureEnabledSeen(qname, nil, make(map[string]bool))
	return ok && enabled
}

func (m *moduleData) featureEnabledSeen(qname string, from *yangparse.Statement, resolving map[string]bool) (bool, bool) {
	if m == nil || m.ctx == nil {
		return false, false
	}
	mod := m.resolveSourceQNameModule(qname)
	if mod == nil {
		return false, false
	}
	local := localName(qname)
	feature := mod.featureMap[local]
	if feature == nil {
		return false, false
	}
	if mod == m && !m.definitionVisibleFrom(feature.stmt, from) {
		return false, false
	}
	enabled := m.ctx.enabledFeatures[mod.name]
	if len(enabled) == 0 {
		return false, true
	}
	if _, ok := enabled[local]; !ok {
		return false, true
	}
	key := moduleKey(mod.name, local)
	if resolving[key] {
		return false, false
	}
	resolving[key] = true
	defer delete(resolving, key)
	for _, iff := range direct(feature.stmt, "if-feature") {
		included, ok := mod.evalIfFeatureExprSeen(iff.Argument, iff, resolving)
		if !ok || !included {
			return false, ok
		}
	}
	return true, true
}

func walkStatements(st *yangparse.Statement, fn func(*yangparse.Statement)) {
	if st == nil {
		return
	}
	fn(st)
	for _, c := range st.SubStatements() {
		walkStatements(c, fn)
	}
}

func localName(qname string) string {
	if _, local, ok := strings.Cut(qname, ":"); ok {
		return local
	}
	return qname
}

func hasPrefix(qname string) bool {
	_, _, ok := strings.Cut(qname, ":")
	return ok
}

func splitPath(path string) []string {
	path = strings.Trim(path, "/")
	if path == "" {
		return nil
	}
	var parts []string
	start := 0
	brackets := 0
	parens := 0
	var quote rune
	for i, r := range path {
		if quote != 0 {
			if r == quote {
				quote = 0
			}
			continue
		}
		switch r {
		case '\'', '"':
			quote = r
		case '[':
			brackets++
		case ']':
			if brackets > 0 {
				brackets--
			}
		case '(':
			parens++
		case ')':
			if parens > 0 {
				parens--
			}
		case '/':
			if brackets == 0 && parens == 0 {
				parts = append(parts, strings.TrimSpace(path[start:i]))
				start = i + 1
			}
		}
	}
	parts = append(parts, strings.TrimSpace(path[start:]))
	return parts
}

func validLeafRefPathArg(path string) bool {
	if path == "" || strings.TrimSpace(path) != path || strings.HasSuffix(path, "/") {
		return false
	}
	parts, ok := splitPathRaw(path)
	if !ok {
		return false
	}
	if len(parts) == 0 {
		return false
	}
	start := 0
	if strings.HasPrefix(path, "/") {
		if len(parts) < 2 || parts[1] == "" {
			return false
		}
		start = 1
	}
	for _, part := range parts[start:] {
		if part == "" || strings.TrimSpace(part) != part {
			return false
		}
		if part == "." {
			return false
		}
		if part == ".." {
			continue
		}
		if strings.HasPrefix(part, "deref(") && strings.HasSuffix(part, ")") {
			continue
		}
		qname := pathStepQNameRaw(part)
		if qname == "" || strings.TrimSpace(qname) != qname || !validYangIdentifierRef(qname, true) {
			return false
		}
	}
	return true
}

func (m *moduleData) validateLeafRefPathPrefixes(pathStmt *yangparse.Statement) error {
	if m == nil || pathStmt == nil {
		return nil
	}
	path := pathStmt.Argument
	for _, prefix := range referencedPrefixes(path) {
		if m.resolveSourceQNameModule(prefix+":_") == nil {
			return fmt.Errorf("unknown prefix %q in leafref path %q at %s", prefix, path, pathStmt.Location())
		}
	}
	return nil
}

func splitPathRaw(path string) ([]string, bool) {
	if path == "" {
		return nil, false
	}
	var parts []string
	start := 0
	brackets := 0
	parens := 0
	var quote rune
	for i, r := range path {
		if quote != 0 {
			if r == quote {
				quote = 0
			}
			continue
		}
		switch r {
		case '\'', '"':
			quote = r
		case '[':
			brackets++
		case ']':
			if brackets == 0 {
				return nil, false
			}
			brackets--
		case '(':
			parens++
		case ')':
			if parens == 0 {
				return nil, false
			}
			parens--
		case '/':
			if brackets == 0 && parens == 0 {
				parts = append(parts, path[start:i])
				start = i + 1
			}
		}
	}
	if quote != 0 || brackets != 0 || parens != 0 {
		return nil, false
	}
	parts = append(parts, path[start:])
	return parts, true
}

func pathStepQNameRaw(step string) string {
	if i := strings.IndexByte(step, '['); i >= 0 {
		return step[:i]
	}
	return step
}

func pathStepQName(step string) string {
	step = strings.TrimSpace(step)
	if i := strings.IndexByte(step, '['); i >= 0 {
		step = step[:i]
	}
	return strings.TrimSpace(step)
}

func derefArgument(step string) (string, bool) {
	const prefix = "deref("
	step = strings.TrimSpace(step)
	if !strings.HasPrefix(step, prefix) || !strings.HasSuffix(step, ")") {
		return "", false
	}
	return strings.TrimSpace(step[len(prefix) : len(step)-1]), true
}

func optional(s string) (string, bool) {
	if s == "" {
		return "", false
	}
	return s, true
}

func statusFromStatement(st *yangparse.Statement) Status {
	switch childArg(st, "status") {
	case "deprecated":
		return StatusDeprecated
	case "obsolete":
		return StatusObsolete
	default:
		return StatusCurrent
	}
}

func parseUint32(s string) (uint32, bool) {
	if !validYANGUintLexical(s) {
		return 0, false
	}
	v, err := strconv.ParseUint(s, 10, 32)
	return uint32(v), err == nil
}

func parseRangeUint(s string, bitSize int) (uint64, error) {
	if strings.HasPrefix(s, "+") {
		s = strings.TrimPrefix(s, "+")
		if s == "" {
			return 0, strconv.ErrSyntax
		}
	}
	return strconv.ParseUint(s, 10, bitSize)
}

func parseYANGInt32(s string) (int64, bool) {
	if !validYANGIntLexical(s) {
		return 0, false
	}
	v, err := strconv.ParseInt(s, 10, 32)
	return v, err == nil
}

func validYANGIntLexical(s string) bool {
	if s == "" || s[0] == '+' {
		return false
	}
	if s[0] == '-' {
		if len(s) == 1 {
			return false
		}
		for i := 1; i < len(s); i++ {
			if !isYANGDecimalDigit(s[i]) {
				return false
			}
		}
		return true
	}
	return validYANGUintLexical(s)
}

func validYANGUintLexical(s string) bool {
	if s == "" {
		return false
	}
	if s == "0" {
		return true
	}
	if s[0] < '1' || s[0] > '9' {
		return false
	}
	for i := 1; i < len(s); i++ {
		if !isYANGDecimalDigit(s[i]) {
			return false
		}
	}
	return true
}

func isYANGDecimalDigit(ch byte) bool {
	return ch >= '0' && ch <= '9'
}

func appendUnique(dst *[]string, value string) {
	for _, existing := range *dst {
		if existing == value {
			return
		}
	}
	*dst = append(*dst, value)
}

func appendIdentityUnique(dst *[]*identityData, value *identityData) {
	for _, existing := range *dst {
		if existing == value {
			return
		}
	}
	*dst = append(*dst, value)
}

func ptr[T any](v T) *T { return &v }

// keep filepath imported for older callers that compare generated paths in
// stack traces; this also catches accidental removal during gofmt grouping.
var _ = filepath.Separator
