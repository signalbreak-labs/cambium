// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

package compat

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/signalbreak-labs/cambium/go/cambium"
)

// TypeKind is the goyang-compatible enumeration of YANG built-in base types.
type TypeKind uint

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

// IsDecimal reports whether n is a decimal64 value.
func (n Number) IsDecimal() bool {
	return n.FractionDigits != 0
}

// YRange is one inclusive range of consecutive numbers.
type YRange struct {
	Min Number
	Max Number
}

// YangRange is a goyang-style set of non-overlapping ranges.
type YangRange []YRange

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

// deviationType identifies one deviate substatement kind.
type deviationType int64

type statementTypedefScopes []map[string]*Statement

type resolvedTypedefStatement struct {
	stmt     *Statement
	resolver Node
	scopes   statementTypedefScopes
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

func (scopes statementTypedefScopes) findTypedef(name string) *Statement {
	_, local := splitPrefix(name)
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
	prefix, local := splitPrefix(name)
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
			YangType: &YangType{
				Name: stmt.Argument,
				Kind: kind,
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
			YangType: base,
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
			if typ.FractionDigits >= 0 && typ.FractionDigits <= math.MaxUint8 {
				if parsed, err := parseStatementRange(typ.Range, rng.Argument, true, uint8(typ.FractionDigits)); err == nil {
					typ.Range = parsed
				}
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
			hi := parent[len(parent)-1].Max
			hi.FractionDigits = fractionDigits
			return hi, nil
		case "min":
			if len(parent) == 0 {
				return Number{}, fmt.Errorf("cannot resolve min without parent range")
			}
			lo := parent[0].Min
			lo.FractionDigits = fractionDigits
			return lo, nil
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
		lo, err := parseNumber(strings.TrimSpace(bounds[0]))
		if err != nil {
			return nil, err
		}
		hi := lo
		if len(bounds) == 2 {
			hi, err = parseNumber(strings.TrimSpace(bounds[1]))
			if err != nil {
				return nil, err
			}
		}
		if hi.Less(lo) {
			return nil, fmt.Errorf("range boundaries out of order")
		}
		out = append(out, YRange{Min: lo, Max: hi})
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

func newBaseTypedefs() map[string]*Typedef {
	out := make(map[string]*Typedef, len(TypeKindFromName)-1)
	for name, kind := range TypeKindFromName {
		if kind == Ynone {
			continue
		}
		typ := &YangType{
			Name:  name,
			Kind:  kind,
			Range: defaultRangeForTypeKind(kind),
		}
		typ.Root = typ
		out[name] = &Typedef{
			Name:   name,
			Source: &Statement{},
			Type: &Type{
				Name:     name,
				Source:   &Statement{},
				YangType: typ,
			},
			YangType: typ,
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
	root := yangTypeRootForBase(base)
	nextName := baseName
	nextYang := resolvedYangTypeForBase(base, root)
	for i := len(chain) - 1; i >= 0; i-- {
		name := chain[i]
		nextYang = &YangType{
			Name: name,
			Kind: typeKindForBase(base),
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

func resolvedYangTypeForBase(base cambium.BaseType, root *YangType) *YangType {
	name := base.String()
	return &YangType{
		Name: name,
		Kind: typeKindForBase(base),
		Base: &Type{
			Name:     name,
			Source:   typeStatement(name),
			YangType: root,
		},
		Root: root,
	}
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
