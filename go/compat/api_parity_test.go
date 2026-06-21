// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

package compat_test

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/signalbreak-labs/cambium/go/compat"
	upstream "github.com/signalbreak-labs/cambium/go/internal/yangparse/upstream/yang"
)

func TestCompatExportedTopLevelSurfaceTracksGoyang(t *testing.T) {
	root := repoRoot(t)
	upstreamSurface := exportedSurface(t, filepath.Join(root, "go", "internal", "yangparse", "upstream", "yang"))
	compatSurface := exportedSurface(t, filepath.Join(root, "go", "compat"))

	missing := missingNames(upstreamSurface.topLevel, compatSurface.topLevel)
	if len(missing) != 0 {
		t.Fatalf("compat missing upstream goyang exported top-level names: %v", missing)
	}
}

func TestCompatStructFieldsTrackGoyang(t *testing.T) {
	tests := []struct {
		name       string
		upstream   reflect.Type
		compat     reflect.Type
		extraField []string
		checkTags  bool
	}{
		{name: "Entry", upstream: reflect.TypeOf(upstream.Entry{}), compat: reflect.TypeOf(compat.Entry{}), checkTags: true},
		{name: "Module", upstream: reflect.TypeOf(upstream.Module{}), compat: reflect.TypeOf(compat.Module{}), checkTags: true},
		{name: "Modules", upstream: reflect.TypeOf(upstream.Modules{}), compat: reflect.TypeOf(compat.Modules{}), checkTags: true},
		{name: "Value", upstream: reflect.TypeOf(upstream.Value{}), compat: reflect.TypeOf(compat.Value{}), checkTags: true},
		{name: "Identity", upstream: reflect.TypeOf(upstream.Identity{}), compat: reflect.TypeOf(compat.Identity{}), checkTags: true},
		{name: "ListAttr", upstream: reflect.TypeOf(upstream.ListAttr{}), compat: reflect.TypeOf(compat.ListAttr{}), checkTags: true},
		{name: "Number", upstream: reflect.TypeOf(upstream.Number{}), compat: reflect.TypeOf(compat.Number{}), checkTags: true},
		{name: "Options", upstream: reflect.TypeOf(upstream.Options{}), compat: reflect.TypeOf(compat.Options{}), checkTags: true},
		{name: "DeviateOptions", upstream: reflect.TypeOf(upstream.DeviateOptions{}), compat: reflect.TypeOf(compat.DeviateOptions{}), checkTags: true},
		{name: "RPCEntry", upstream: reflect.TypeOf(upstream.RPCEntry{}), compat: reflect.TypeOf(compat.RPCEntry{}), checkTags: true},
		{name: "UsesStmt", upstream: reflect.TypeOf(upstream.UsesStmt{}), compat: reflect.TypeOf(compat.UsesStmt{}), checkTags: true},
		{name: "DeviatedEntry", upstream: reflect.TypeOf(upstream.DeviatedEntry{}), compat: reflect.TypeOf(compat.DeviatedEntry{}), checkTags: true},
		{name: "EnumType", upstream: reflect.TypeOf(upstream.EnumType{}), compat: reflect.TypeOf(compat.EnumType{}), checkTags: true},
		{name: "YRange", upstream: reflect.TypeOf(upstream.YRange{}), compat: reflect.TypeOf(compat.YRange{}), checkTags: true},
		{
			name:       "YangType",
			upstream:   reflect.TypeOf(upstream.YangType{}),
			compat:     reflect.TypeOf(compat.YangType{}),
			extraField: []string{"IdentityBases"},
			checkTags:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			want := exportedStructFields(tt.upstream)
			got := exportedStructFields(tt.compat)
			extra := map[string]bool{}
			for _, name := range tt.extraField {
				extra[name] = true
			}
			for _, name := range got {
				if extra[name] {
					got = deleteName(got, name)
				}
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("%s exported fields = %v, want goyang fields %v", tt.name, got, want)
			}
			wantTypes := exportedStructFieldTypes(tt.upstream)
			gotTypes := exportedStructFieldTypes(tt.compat)
			for name := range extra {
				delete(gotTypes, name)
			}
			for _, name := range want {
				if gotTypes[name] != wantTypes[name] {
					t.Fatalf("%s.%s type = %s, want goyang type %s", tt.name, name, gotTypes[name], wantTypes[name])
				}
			}
			if tt.checkTags {
				wantTags := exportedStructFieldTags(tt.upstream)
				gotTags := exportedStructFieldTags(tt.compat)
				for _, name := range want {
					if gotTags[name] != wantTags[name] {
						t.Fatalf("%s.%s tag = %q, want goyang tag %q", tt.name, name, gotTags[name], wantTags[name])
					}
				}
			}
		})
	}
}

func TestCompatExportedStructDeclarationsTrackGoyang(t *testing.T) {
	root := repoRoot(t)
	upstreamSurface := exportedSurface(t, filepath.Join(root, "go", "internal", "yangparse", "upstream", "yang"))
	compatSurface := exportedSurface(t, filepath.Join(root, "go", "compat"))
	extraFields := map[string]map[string]bool{
		"YangType": {"IdentityBases": true},
	}

	for typeName, wantFields := range upstreamSurface.structFields {
		if aliasCoversUpstreamType(typeName, compatSurface.aliases[typeName]) {
			continue
		}
		gotFields := compatSurface.structFields[typeName]
		gotOrder := deleteExtraFieldNames(gotFields.order, extraFields[typeName])
		if !reflect.DeepEqual(gotOrder, wantFields.order) {
			t.Fatalf("%s exported fields = %v, want goyang fields %v", typeName, gotOrder, wantFields.order)
		}
		for _, name := range wantFields.order {
			if gotFields.types[name] != wantFields.types[name] {
				t.Fatalf("%s.%s type = %s, want goyang type %s", typeName, name, gotFields.types[name], wantFields.types[name])
			}
			if gotFields.tags[name] != wantFields.tags[name] {
				t.Fatalf("%s.%s tag = %q, want goyang tag %q", typeName, name, gotFields.tags[name], wantFields.tags[name])
			}
		}
	}
}

func TestCompatExportedTypeDeclarationsTrackGoyang(t *testing.T) {
	root := repoRoot(t)
	upstreamSurface := exportedSurface(t, filepath.Join(root, "go", "internal", "yangparse", "upstream", "yang"))
	compatSurface := exportedSurface(t, filepath.Join(root, "go", "compat"))

	for typeName, want := range upstreamSurface.typeDecls {
		if aliasCoversUpstreamType(typeName, compatSurface.aliases[typeName]) {
			continue
		}
		if got := compatSurface.typeDecls[typeName]; got != want {
			t.Fatalf("%s type declaration = %s, want goyang type declaration %s", typeName, got, want)
		}
	}
}

func TestCompatExportedValueDeclarationsTrackGoyang(t *testing.T) {
	root := repoRoot(t)
	upstreamSurface := exportedSurface(t, filepath.Join(root, "go", "internal", "yangparse", "upstream", "yang"))
	compatSurface := exportedSurface(t, filepath.Join(root, "go", "compat"))

	for name, want := range upstreamSurface.values {
		got, ok := compatSurface.values[name]
		if !ok {
			t.Fatalf("compat missing upstream goyang exported value %s", name)
		}
		if valueAliasCoversUpstreamValue(name, got) {
			continue
		}
		if got.kind != want.kind {
			t.Fatalf("%s declaration kind = %s, want goyang kind %s", name, got.kind, want.kind)
		}
		if want.typ != "" && got.typ == "" {
			t.Fatalf("%s declaration type is empty, want goyang type %s", name, want.typ)
		}
		if want.typ != "" && got.typ != "" && got.typ != want.typ {
			t.Fatalf("%s declaration type = %s, want goyang type %s", name, got.typ, want.typ)
		}
	}
}

func TestCompatExportedRuntimeValuesTrackGoyang(t *testing.T) {
	if got, want := compat.TypeKindToName, convertUpstreamTypeKindToName(upstream.TypeKindToName); !reflect.DeepEqual(got, want) {
		t.Fatalf("TypeKindToName = %v, want goyang %v", got, want)
	}
	if got, want := compat.TypeKindFromName, convertUpstreamTypeKindFromName(upstream.TypeKindFromName); !reflect.DeepEqual(got, want) {
		t.Fatalf("TypeKindFromName = %v, want goyang %v", got, want)
	}
	if got, want := compat.EntryKindToName, convertUpstreamEntryKindToName(upstream.EntryKindToName); !reflect.DeepEqual(got, want) {
		t.Fatalf("EntryKindToName = %v, want goyang %v", got, want)
	}

	constants := []struct {
		name string
		got  string
		want string
	}{
		{name: "MaxInt64", got: fmt.Sprint(compat.MaxInt64), want: fmt.Sprint(upstream.MaxInt64)},
		{name: "MinInt64", got: fmt.Sprint(compat.MinInt64), want: fmt.Sprint(upstream.MinInt64)},
		{name: "MinDecimal64", got: fmt.Sprint(compat.MinDecimal64), want: fmt.Sprint(upstream.MinDecimal64)},
		{name: "MaxDecimal64", got: fmt.Sprint(compat.MaxDecimal64), want: fmt.Sprint(upstream.MaxDecimal64)},
		{name: "AbsMinInt64", got: fmt.Sprint(uint64(compat.AbsMinInt64)), want: fmt.Sprint(uint64(upstream.AbsMinInt64))},
		{name: "MaxEnum", got: fmt.Sprint(compat.MaxEnum), want: fmt.Sprint(upstream.MaxEnum)},
		{name: "MinEnum", got: fmt.Sprint(compat.MinEnum), want: fmt.Sprint(upstream.MinEnum)},
		{name: "MaxBitfieldSize", got: fmt.Sprint(uint64(compat.MaxBitfieldSize)), want: fmt.Sprint(uint64(upstream.MaxBitfieldSize))},
		{name: "MaxFractionDigits", got: fmt.Sprint(compat.MaxFractionDigits), want: fmt.Sprint(upstream.MaxFractionDigits)},
	}
	for _, tc := range constants {
		if tc.got != tc.want {
			t.Fatalf("%s = %v, want goyang %v", tc.name, tc.got, tc.want)
		}
	}

	ranges := []struct {
		name string
		got  compat.YangRange
		want upstream.YangRange
	}{
		{name: "Int8Range", got: compat.Int8Range, want: upstream.Int8Range},
		{name: "Int16Range", got: compat.Int16Range, want: upstream.Int16Range},
		{name: "Int32Range", got: compat.Int32Range, want: upstream.Int32Range},
		{name: "Int64Range", got: compat.Int64Range, want: upstream.Int64Range},
		{name: "Uint8Range", got: compat.Uint8Range, want: upstream.Uint8Range},
		{name: "Uint16Range", got: compat.Uint16Range, want: upstream.Uint16Range},
		{name: "Uint32Range", got: compat.Uint32Range, want: upstream.Uint32Range},
		{name: "Uint64Range", got: compat.Uint64Range, want: upstream.Uint64Range},
	}
	for _, tc := range ranges {
		if got, want := formatCompatRange(tc.got), formatUpstreamRange(tc.want); got != want {
			t.Fatalf("%s = %s, want goyang %s", tc.name, got, want)
		}
	}
}

func TestCompatFunctionSignaturesTrackGoyang(t *testing.T) {
	tests := []struct {
		name     string
		upstream any
		compat   any
	}{
		{name: "CamelCase", upstream: upstream.CamelCase, compat: compat.CamelCase},
		{name: "ChildNode", upstream: upstream.ChildNode, compat: compat.ChildNode},
		{name: "FindGrouping", upstream: upstream.FindGrouping, compat: compat.FindGrouping},
		{name: "FindModuleByPrefix", upstream: upstream.FindModuleByPrefix, compat: compat.FindModuleByPrefix},
		{name: "FindNode", upstream: upstream.FindNode, compat: compat.FindNode},
		{name: "Frac", upstream: upstream.Frac, compat: compat.Frac},
		{name: "FromFloat", upstream: upstream.FromFloat, compat: compat.FromFloat},
		{name: "FromInt", upstream: upstream.FromInt, compat: compat.FromInt},
		{name: "FromUint", upstream: upstream.FromUint, compat: compat.FromUint},
		{name: "GetModule", upstream: upstream.GetModule, compat: compat.GetModule},
		{name: "MatchingEntryExtensions", upstream: upstream.MatchingEntryExtensions, compat: compat.MatchingEntryExtensions},
		{name: "MatchingExtensions", upstream: upstream.MatchingExtensions, compat: compat.MatchingExtensions},
		{name: "NewBitfield", upstream: upstream.NewBitfield, compat: compat.NewBitfield},
		{name: "NewDefaultListAttr", upstream: upstream.NewDefaultListAttr, compat: compat.NewDefaultListAttr},
		{name: "NewEnumType", upstream: upstream.NewEnumType, compat: compat.NewEnumType},
		{name: "NewModules", upstream: upstream.NewModules, compat: compat.NewModules},
		{name: "NodePath", upstream: upstream.NodePath, compat: compat.NodePath},
		{name: "Parse", upstream: upstream.Parse, compat: compat.Parse},
		{name: "ParseDecimal", upstream: upstream.ParseDecimal, compat: compat.ParseDecimal},
		{name: "ParseInt", upstream: upstream.ParseInt, compat: compat.ParseInt},
		{name: "ParseRangesDecimal", upstream: upstream.ParseRangesDecimal, compat: compat.ParseRangesDecimal},
		{name: "ParseRangesInt", upstream: upstream.ParseRangesInt, compat: compat.ParseRangesInt},
		{name: "PathsWithModules", upstream: upstream.PathsWithModules, compat: compat.PathsWithModules},
		{name: "PrintNode", upstream: upstream.PrintNode, compat: compat.PrintNode},
		{name: "RootNode", upstream: upstream.RootNode, compat: compat.RootNode},
		{name: "Source", upstream: upstream.Source, compat: compat.Source},
		{name: "ToEntry", upstream: upstream.ToEntry, compat: compat.ToEntry},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			want := normalizedSignature(reflect.TypeOf(tt.upstream), false)
			got := normalizedSignature(reflect.TypeOf(tt.compat), false)
			if got != want {
				t.Fatalf("%s signature = %s, want goyang signature %s", tt.name, got, want)
			}
		})
	}
}

func TestCompatExportedFunctionDeclarationsTrackGoyang(t *testing.T) {
	root := repoRoot(t)
	upstreamSurface := exportedSurface(t, filepath.Join(root, "go", "internal", "yangparse", "upstream", "yang"))
	compatSurface := exportedSurface(t, filepath.Join(root, "go", "compat"))

	missing := missingNames(sortedMapKeys(upstreamSurface.funcs), sortedMapKeys(compatSurface.funcs))
	if len(missing) != 0 {
		t.Fatalf("compat missing upstream goyang exported functions: %v", missing)
	}
	for name, want := range upstreamSurface.funcs {
		if got := compatSurface.funcs[name]; got != want {
			t.Fatalf("%s declaration signature = %s, want goyang signature %s", name, got, want)
		}
	}
}

func TestCompatExportedMethodDeclarationsTrackGoyang(t *testing.T) {
	root := repoRoot(t)
	upstreamSurface := exportedSurface(t, filepath.Join(root, "go", "internal", "yangparse", "upstream", "yang"))
	compatSurface := exportedSurface(t, filepath.Join(root, "go", "compat"))

	for typeName, wantMethods := range upstreamSurface.methods {
		if aliasCoversUpstreamType(typeName, compatSurface.aliases[typeName]) {
			continue
		}
		gotMethods := compatSurface.methods[typeName]
		missing := missingNames(sortedMapKeys(wantMethods), sortedMapKeys(gotMethods))
		if len(missing) != 0 {
			t.Fatalf("%s missing goyang exported methods: %v", typeName, missing)
		}
		for name, want := range wantMethods {
			if got := gotMethods[name]; got != want {
				t.Fatalf("%s.%s declaration signature = %s, want goyang signature %s", typeName, name, got, want)
			}
		}
	}
}

func TestCompatMethodSetsTrackGoyang(t *testing.T) {
	tests := []struct {
		name     string
		upstream reflect.Type
		compat   reflect.Type
	}{
		{name: "Entry", upstream: reflect.TypeOf(&upstream.Entry{}), compat: reflect.TypeOf(&compat.Entry{})},
		{name: "Module", upstream: reflect.TypeOf(&upstream.Module{}), compat: reflect.TypeOf(&compat.Module{})},
		{name: "Modules", upstream: reflect.TypeOf(&upstream.Modules{}), compat: reflect.TypeOf(&compat.Modules{})},
		{name: "Value", upstream: reflect.TypeOf(&upstream.Value{}), compat: reflect.TypeOf(&compat.Value{})},
		{name: "Identity", upstream: reflect.TypeOf(&upstream.Identity{}), compat: reflect.TypeOf(&compat.Identity{})},
		{name: "Number", upstream: reflect.TypeOf(upstream.Number{}), compat: reflect.TypeOf(compat.Number{})},
		{name: "YRange", upstream: reflect.TypeOf(upstream.YRange{}), compat: reflect.TypeOf(compat.YRange{})},
		{name: "YangRange", upstream: reflect.TypeOf(upstream.YangRange{}), compat: reflect.TypeOf(compat.YangRange{})},
		{name: "YangType", upstream: reflect.TypeOf(&upstream.YangType{}), compat: reflect.TypeOf(&compat.YangType{})},
		{name: "EnumType", upstream: reflect.TypeOf(&upstream.EnumType{}), compat: reflect.TypeOf(&compat.EnumType{})},
		{name: "DeviateOptions", upstream: reflect.TypeOf(upstream.DeviateOptions{}), compat: reflect.TypeOf(compat.DeviateOptions{})},
		{name: "TriState", upstream: reflect.TypeOf(upstream.TriState(0)), compat: reflect.TypeOf(compat.TriState(0))},
		{name: "EntryKind", upstream: reflect.TypeOf(upstream.EntryKind(0)), compat: reflect.TypeOf(compat.EntryKind(0))},
		{name: "TypeKind", upstream: reflect.TypeOf(upstream.TypeKind(0)), compat: reflect.TypeOf(compat.TypeKind(0))},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			want := exportedMethodSignatures(tt.upstream)
			got := exportedMethodSignatures(tt.compat)
			missing := missingNames(sortedMapKeys(want), sortedMapKeys(got))
			if len(missing) != 0 {
				t.Fatalf("%s missing goyang exported methods: %v", tt.name, missing)
			}
			for name, wantSignature := range want {
				if gotSignature := got[name]; gotSignature != wantSignature {
					t.Fatalf("%s.%s signature = %s, want goyang signature %s", tt.name, name, gotSignature, wantSignature)
				}
			}
		})
	}
}

type packageSurface struct {
	topLevel     []string
	funcs        map[string]string
	methods      map[string]map[string]string
	aliases      map[string]string
	structFields map[string]structFieldSet
	typeDecls    map[string]string
	values       map[string]valueDecl
}

type structFieldSet struct {
	order []string
	types map[string]string
	tags  map[string]string
}

type valueDecl struct {
	kind string
	typ  string
	expr string
}

func exportedSurface(t *testing.T, dir string) packageSurface {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir(%s): %v", dir, err)
	}
	names := map[string]bool{}
	funcs := map[string]string{}
	methods := map[string]map[string]string{}
	aliases := map[string]string{}
	structFields := map[string]structFieldSet{}
	typeDecls := map[string]string{}
	values := map[string]valueDecl{}
	fset := token.NewFileSet()
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		file, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
		if err != nil {
			t.Fatalf("ParseFile(%s): %v", path, err)
		}
		for _, decl := range file.Decls {
			switch decl := decl.(type) {
			case *ast.GenDecl:
				collectExportedValueDecls(decl, values)
				for _, spec := range decl.Specs {
					switch spec := spec.(type) {
					case *ast.TypeSpec:
						if spec.Name.IsExported() {
							names[spec.Name.Name] = true
							if spec.Assign.IsValid() {
								aliases[spec.Name.Name] = normalizedASTType(spec.Type)
							}
							if structType, ok := spec.Type.(*ast.StructType); ok {
								structFields[spec.Name.Name] = exportedASTStructFields(structType)
							} else if !spec.Assign.IsValid() {
								typeDecls[spec.Name.Name] = normalizedASTType(spec.Type)
							}
						}
					case *ast.ValueSpec:
						for _, name := range spec.Names {
							if name.IsExported() {
								names[name.Name] = true
							}
						}
					}
				}
			case *ast.FuncDecl:
				if decl.Recv == nil && decl.Name.IsExported() {
					names[decl.Name.Name] = true
					funcs[decl.Name.Name] = normalizedFuncDeclSignature(decl.Type)
					continue
				}
				receiver := methodReceiverTypeName(decl)
				if receiver != "" && ast.IsExported(receiver) && decl.Name.IsExported() {
					if methods[receiver] == nil {
						methods[receiver] = map[string]string{}
					}
					methods[receiver][decl.Name.Name] = normalizedMethodDeclSignature(decl)
				}
			}
		}
	}
	return packageSurface{
		topLevel:     sortedNames(names),
		funcs:        funcs,
		methods:      methods,
		aliases:      aliases,
		structFields: structFields,
		typeDecls:    typeDecls,
		values:       values,
	}
}

func exportedStructFields(typ reflect.Type) []string {
	names := make([]string, 0, typ.NumField())
	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		if field.IsExported() {
			names = append(names, field.Name)
		}
	}
	return names
}

func exportedStructFieldTypes(typ reflect.Type) map[string]string {
	fields := make(map[string]string)
	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		if field.IsExported() {
			fields[field.Name] = normalizeType(field.Type)
		}
	}
	return fields
}

func exportedStructFieldTags(typ reflect.Type) map[string]string {
	fields := make(map[string]string)
	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		if field.IsExported() {
			fields[field.Name] = string(field.Tag)
		}
	}
	return fields
}

func exportedMethodSignatures(typ reflect.Type) map[string]string {
	methods := make(map[string]string)
	for i := 0; i < typ.NumMethod(); i++ {
		method := typ.Method(i)
		methods[method.Name] = normalizedSignature(method.Type, true)
	}
	return methods
}

func normalizedSignature(typ reflect.Type, skipReceiver bool) string {
	if typ.Kind() != reflect.Func {
		return normalizeType(typ)
	}
	var b strings.Builder
	b.WriteString("func(")
	start := 0
	if skipReceiver {
		start = 1
	}
	for i := start; i < typ.NumIn(); i++ {
		if i > start {
			b.WriteString(", ")
		}
		if typ.IsVariadic() && i == typ.NumIn()-1 {
			b.WriteString("...")
			b.WriteString(normalizeType(typ.In(i).Elem()))
			continue
		}
		b.WriteString(normalizeType(typ.In(i)))
	}
	b.WriteString(")")
	switch typ.NumOut() {
	case 0:
	case 1:
		b.WriteString(" ")
		b.WriteString(normalizeType(typ.Out(0)))
	default:
		b.WriteString(" (")
		for i := 0; i < typ.NumOut(); i++ {
			if i > 0 {
				b.WriteString(", ")
			}
			b.WriteString(normalizeType(typ.Out(i)))
		}
		b.WriteString(")")
	}
	return b.String()
}

func normalizeType(typ reflect.Type) string {
	switch typ.Kind() {
	case reflect.Pointer:
		return "*" + normalizeType(typ.Elem())
	case reflect.Slice:
		return "[]" + normalizeType(typ.Elem())
	case reflect.Array:
		return "[" + strconv.Itoa(typ.Len()) + "]" + normalizeType(typ.Elem())
	case reflect.Map:
		return "map[" + normalizeType(typ.Key()) + "]" + normalizeType(typ.Elem())
	case reflect.Interface:
		if typ.Name() != "" {
			return normalizeNamedType(typ)
		}
		return "interface{}"
	default:
		if typ.Name() != "" {
			return normalizeNamedType(typ)
		}
		return typ.String()
	}
}

func normalizeNamedType(typ reflect.Type) string {
	if typ.PkgPath() == "" {
		return typ.Name()
	}
	switch typ.PkgPath() {
	case "github.com/signalbreak-labs/cambium/go/compat",
		"github.com/signalbreak-labs/cambium/go/internal/yangparse",
		"github.com/signalbreak-labs/cambium/go/internal/yangparse/upstream/yang":
		return typ.Name()
	default:
		return typ.String()
	}
}

func normalizedFuncDeclSignature(typ *ast.FuncType) string {
	var b strings.Builder
	b.WriteString("func(")
	b.WriteString(normalizedASTFieldList(typ.Params))
	b.WriteString(")")
	results := normalizedASTFieldTypes(typ.Results)
	switch len(results) {
	case 0:
	case 1:
		b.WriteString(" ")
		b.WriteString(results[0])
	default:
		b.WriteString(" (")
		b.WriteString(strings.Join(results, ", "))
		b.WriteString(")")
	}
	return b.String()
}

func normalizedMethodDeclSignature(decl *ast.FuncDecl) string {
	return methodReceiverKind(decl) + " " + normalizedFuncDeclSignature(decl.Type)
}

func normalizedASTFieldList(fields *ast.FieldList) string {
	return strings.Join(normalizedASTFieldTypes(fields), ", ")
}

func normalizedASTFieldTypes(fields *ast.FieldList) []string {
	if fields == nil {
		return nil
	}
	var out []string
	for _, field := range fields.List {
		count := len(field.Names)
		if count == 0 {
			count = 1
		}
		typ := normalizedASTType(field.Type)
		for i := 0; i < count; i++ {
			out = append(out, typ)
		}
	}
	return out
}

func normalizedASTType(expr ast.Expr) string {
	if expr == nil {
		return ""
	}
	switch expr := expr.(type) {
	case *ast.Ident:
		if expr.Name == "any" {
			return "interface{}"
		}
		return expr.Name
	case *ast.StarExpr:
		return "*" + normalizedASTType(expr.X)
	case *ast.ArrayType:
		if expr.Len == nil {
			return "[]" + normalizedASTType(expr.Elt)
		}
		return "[" + normalizedASTType(expr.Len) + "]" + normalizedASTType(expr.Elt)
	case *ast.MapType:
		return "map[" + normalizedASTType(expr.Key) + "]" + normalizedASTType(expr.Value)
	case *ast.SelectorExpr:
		return normalizedASTType(expr.X) + "." + expr.Sel.Name
	case *ast.InterfaceType:
		if expr.Methods == nil || len(expr.Methods.List) == 0 {
			return "interface{}"
		}
		return "interface{" + normalizedASTInterfaceMethods(expr.Methods) + "}"
	case *ast.FuncType:
		return normalizedFuncDeclSignature(expr)
	case *ast.Ellipsis:
		return "..." + normalizedASTType(expr.Elt)
	case *ast.ParenExpr:
		return "(" + normalizedASTType(expr.X) + ")"
	case *ast.BasicLit:
		return expr.Value
	case *ast.ChanType:
		switch expr.Dir {
		case ast.SEND:
			return "chan<- " + normalizedASTType(expr.Value)
		case ast.RECV:
			return "<-chan " + normalizedASTType(expr.Value)
		default:
			return "chan " + normalizedASTType(expr.Value)
		}
	case *ast.IndexExpr:
		return normalizedASTType(expr.X) + "[" + normalizedASTType(expr.Index) + "]"
	case *ast.IndexListExpr:
		indices := make([]string, 0, len(expr.Indices))
		for _, index := range expr.Indices {
			indices = append(indices, normalizedASTType(index))
		}
		return normalizedASTType(expr.X) + "[" + strings.Join(indices, ", ") + "]"
	default:
		return printedASTType(expr)
	}
}

func normalizedASTInterfaceMethods(methods *ast.FieldList) string {
	if methods == nil {
		return ""
	}
	var out []string
	for _, method := range methods.List {
		if len(method.Names) == 0 {
			out = append(out, normalizedASTType(method.Type))
			continue
		}
		for _, name := range method.Names {
			out = append(out, name.Name+normalizedASTType(method.Type))
		}
	}
	return strings.Join(out, "; ")
}

func printedASTType(expr ast.Expr) string {
	var b bytes.Buffer
	if err := printer.Fprint(&b, token.NewFileSet(), expr); err != nil {
		return "<unprintable>"
	}
	return b.String()
}

func exportedASTStructFields(structType *ast.StructType) structFieldSet {
	fields := structFieldSet{
		types: map[string]string{},
		tags:  map[string]string{},
	}
	for _, field := range structType.Fields.List {
		for _, name := range exportedASTFieldNames(field) {
			fields.order = append(fields.order, name)
			fields.types[name] = normalizedASTType(field.Type)
			fields.tags[name] = normalizedASTTag(field.Tag)
		}
	}
	return fields
}

func exportedASTFieldNames(field *ast.Field) []string {
	if len(field.Names) == 0 {
		name := embeddedASTFieldName(field.Type)
		if ast.IsExported(name) {
			return []string{name}
		}
		return nil
	}
	var names []string
	for _, name := range field.Names {
		if name.IsExported() {
			names = append(names, name.Name)
		}
	}
	return names
}

func embeddedASTFieldName(expr ast.Expr) string {
	switch expr := expr.(type) {
	case *ast.Ident:
		return expr.Name
	case *ast.SelectorExpr:
		return expr.Sel.Name
	case *ast.StarExpr:
		return embeddedASTFieldName(expr.X)
	case *ast.IndexExpr:
		return embeddedASTFieldName(expr.X)
	case *ast.IndexListExpr:
		return embeddedASTFieldName(expr.X)
	default:
		return ""
	}
}

func normalizedASTTag(tag *ast.BasicLit) string {
	if tag == nil {
		return ""
	}
	value, err := strconv.Unquote(tag.Value)
	if err != nil {
		return tag.Value
	}
	return value
}

func collectExportedValueDecls(decl *ast.GenDecl, values map[string]valueDecl) {
	if decl.Tok != token.CONST && decl.Tok != token.VAR {
		return
	}
	groupType := ""
	for _, spec := range decl.Specs {
		spec, ok := spec.(*ast.ValueSpec)
		if !ok {
			continue
		}
		specType := normalizedASTType(spec.Type)
		if decl.Tok == token.CONST && specType == "" && len(spec.Values) > 0 {
			specType = constConversionType(spec.Values[0])
		}
		if decl.Tok == token.CONST {
			if specType == "" && len(spec.Values) == 0 {
				specType = groupType
			} else if specType != "" {
				groupType = specType
			}
		}
		for i, name := range spec.Names {
			if !name.IsExported() {
				continue
			}
			expr := valueSpecExpr(spec.Values, i)
			declType := specType
			if declType == "" {
				declType = inferredValueSpecType(expr)
			}
			values[name.Name] = valueDecl{
				kind: strings.ToLower(decl.Tok.String()),
				typ:  declType,
				expr: normalizedASTExpr(expr),
			}
		}
	}
}

func valueSpecExpr(values []ast.Expr, index int) ast.Expr {
	switch {
	case len(values) == 0:
		return nil
	case index < len(values):
		return values[index]
	case len(values) == 1:
		return values[0]
	default:
		return nil
	}
}

func constConversionType(expr ast.Expr) string {
	call, ok := expr.(*ast.CallExpr)
	if !ok || len(call.Args) != 1 {
		return ""
	}
	if ident, ok := call.Args[0].(*ast.Ident); !ok || ident.Name != "iota" {
		return ""
	}
	return normalizedASTType(call.Fun)
}

func inferredValueSpecType(expr ast.Expr) string {
	switch expr := expr.(type) {
	case *ast.CompositeLit:
		return normalizedASTType(expr.Type)
	default:
		return ""
	}
}

func normalizedASTExpr(expr ast.Expr) string {
	if expr == nil {
		return ""
	}
	return normalizedASTType(expr)
}

func methodReceiverTypeName(decl *ast.FuncDecl) string {
	if decl.Recv == nil || len(decl.Recv.List) == 0 {
		return ""
	}
	return receiverTypeName(decl.Recv.List[0].Type)
}

func methodReceiverKind(decl *ast.FuncDecl) string {
	if decl.Recv == nil || len(decl.Recv.List) == 0 {
		return ""
	}
	if receiverIsPointer(decl.Recv.List[0].Type) {
		return "pointer"
	}
	return "value"
}

func receiverTypeName(expr ast.Expr) string {
	switch expr := expr.(type) {
	case *ast.Ident:
		return expr.Name
	case *ast.StarExpr:
		return receiverTypeName(expr.X)
	case *ast.IndexExpr:
		return receiverTypeName(expr.X)
	case *ast.IndexListExpr:
		return receiverTypeName(expr.X)
	default:
		return ""
	}
}

func receiverIsPointer(expr ast.Expr) bool {
	switch expr := expr.(type) {
	case *ast.StarExpr:
		return true
	case *ast.IndexExpr:
		return receiverIsPointer(expr.X)
	case *ast.IndexListExpr:
		return receiverIsPointer(expr.X)
	default:
		return false
	}
}

func aliasCoversUpstreamType(typeName, aliasTarget string) bool {
	switch aliasTarget {
	case "upstream." + typeName:
		return true
	case "yangparse.Statement":
		return typeName == "Statement"
	default:
		return false
	}
}

func valueAliasCoversUpstreamValue(name string, got valueDecl) bool {
	return got.kind == "var" && got.expr == "upstream."+name
}

func convertUpstreamTypeKindToName(in map[upstream.TypeKind]string) map[compat.TypeKind]string {
	out := make(map[compat.TypeKind]string, len(in))
	for kind, name := range in {
		out[compat.TypeKind(kind)] = name
	}
	return out
}

func convertUpstreamTypeKindFromName(in map[string]upstream.TypeKind) map[string]compat.TypeKind {
	out := make(map[string]compat.TypeKind, len(in))
	for name, kind := range in {
		out[name] = compat.TypeKind(kind)
	}
	return out
}

func convertUpstreamEntryKindToName(in map[upstream.EntryKind]string) map[compat.EntryKind]string {
	out := make(map[compat.EntryKind]string, len(in))
	for kind, name := range in {
		out[compat.EntryKind(kind)] = name
	}
	return out
}

func formatCompatRange(r compat.YangRange) string {
	parts := make([]string, 0, len(r))
	for _, part := range r {
		parts = append(parts, part.Min.String()+".."+part.Max.String())
	}
	return strings.Join(parts, "|")
}

func formatUpstreamRange(r upstream.YangRange) string {
	parts := make([]string, 0, len(r))
	for _, part := range r {
		parts = append(parts, part.Min.String()+".."+part.Max.String())
	}
	return strings.Join(parts, "|")
}

func deleteExtraFieldNames(names []string, extra map[string]bool) []string {
	if len(extra) == 0 {
		return names
	}
	out := names[:0]
	for _, name := range names {
		if !extra[name] {
			out = append(out, name)
		}
	}
	return out
}

func missingNames(want, got []string) []string {
	have := map[string]bool{}
	for _, name := range got {
		have[name] = true
	}
	var missing []string
	for _, name := range want {
		if !have[name] {
			missing = append(missing, name)
		}
	}
	return missing
}

func sortedMapKeys[V any](values map[string]V) []string {
	out := make([]string, 0, len(values))
	for name := range values {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

func sortedNames(names map[string]bool) []string {
	out := make([]string, 0, len(names))
	for name := range names {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

func deleteName(names []string, target string) []string {
	out := names[:0]
	for _, name := range names {
		if name != target {
			out = append(out, name)
		}
	}
	return out
}

func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}
