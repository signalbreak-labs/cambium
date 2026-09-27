// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

// Command cambium-ir exports Cambium's pure-Go SchemaIR as JSON.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/signalbreak-labs/cambium/go/cambium"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	var searches multiFlag
	var features multiFlag
	fs := flag.NewFlagSet("cambium-ir", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Var(&searches, "search", "module search path; may be repeated")
	fs.Var(&features, "feature", "enable features as module=feature[,feature]; may be repeated")
	format := fs.String("format", "v1", "projection to emit: v1 (nested "+cambium.SchemaIRVersion+") or v2 (bounded node table "+cambium.SchemaIRTableVersion+")")
	maxRecords := fs.Uint64("max-records", defaultMaxV1Records, "v1 only: refuse to emit when the nested projection would exceed this many node records")
	fs.Usage = func() {
		writef(stderr, "usage: cambium-ir [-search DIR] [-feature module=a,b] [-format v1|v2] [-max-records N] MODULE [MODULE...]\n")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *format != "v1" && *format != "v2" {
		writef(stderr, "error: -format must be v1 or v2, got %q\n", *format)
		return 2
	}
	modules := fs.Args()
	if len(modules) == 0 {
		writef(stderr, "error: at least one module name is required\n")
		fs.Usage()
		return 2
	}

	builder, err := cambium.NewContextBuilder(cambium.ContextFlags{})
	if err != nil {
		writef(stderr, "error: %v\n", err)
		return 1
	}
	for _, search := range searches {
		if err := builder.SearchPath(search); err != nil {
			writef(stderr, "error: %v\n", err)
			return 1
		}
	}
	for _, spec := range features {
		module, names, err := parseFeatureSpec(spec)
		if err != nil {
			writef(stderr, "error: %v\n", err)
			return 2
		}
		if err := builder.SetFeatures(module, names); err != nil {
			writef(stderr, "error: %v\n", err)
			return 1
		}
	}
	for _, module := range modules {
		if err := builder.LoadModule(module, nil, nil); err != nil {
			writef(stderr, "error: %v\n", err)
			return 1
		}
	}
	ctx, err := builder.Build()
	if err != nil {
		writef(stderr, "error: %v\n", err)
		return 1
	}
	defer ctx.Close()

	var doc any
	if *format == "v2" {
		doc = exportSchemaIRTable(ctx.SchemaIRTable())
	} else {
		ir, err := ctx.SchemaIRWithLimit(*maxRecords)
		if err != nil {
			writef(stderr, "error: %v\n", err)
			return 1
		}
		doc = exportSchemaIR(ir)
	}
	enc := json.NewEncoder(stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(doc); err != nil {
		writef(stderr, "error: %v\n", err)
		return 1
	}
	return 0
}

// defaultMaxV1Records bounds the nested v1 projection, whose size can grow
// exponentially with schema depth.
const defaultMaxV1Records = 1 << 20

func writef(w io.Writer, format string, args ...any) {
	_, _ = fmt.Fprintf(w, format, args...)
}

type multiFlag []string

func (m *multiFlag) String() string {
	if m == nil {
		return ""
	}
	return strings.Join(*m, ",")
}

func (m *multiFlag) Set(value string) error {
	*m = append(*m, value)
	return nil
}

func parseFeatureSpec(spec string) (module string, features []string, err error) {
	module, raw, ok := strings.Cut(spec, "=")
	if !ok || module == "" {
		return "", nil, fmt.Errorf("feature spec %q must be module=feature[,feature]", spec)
	}
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part != "" {
			features = append(features, part)
		}
	}
	return module, features, nil
}

type exportIR struct {
	Version string             `json:"version"`
	Modules []exportModule     `json:"modules"`
	Errors  []exportDiagnostic `json:"errors,omitempty"`
}

type exportModule struct {
	Name        string          `json:"name"`
	Namespace   string          `json:"namespace"`
	Prefix      string          `json:"prefix"`
	Revision    string          `json:"revision,omitempty"`
	Implemented bool            `json:"implemented"`
	Source      exportSource    `json:"source"`
	Imports     []exportImport  `json:"imports,omitempty"`
	Includes    []exportInclude `json:"includes,omitempty"`
	Children    []exportNode    `json:"children,omitempty"`
}

type exportNode struct {
	Name                   string              `json:"name"`
	Kind                   string              `json:"kind"`
	LocalPath              string              `json:"local_path"`
	QualifiedPath          string              `json:"qualified_path"`
	NamespaceQualifiedPath string              `json:"namespace_qualified_path"`
	QualifiedName          exportQualifiedName `json:"qualified_name"`
	Children               []exportNode        `json:"children,omitempty"`
	DataChildren           []exportNode        `json:"data_children,omitempty"`
	ListKeys               []exportNode        `json:"list_keys,omitempty"`
	KeyNames               []string            `json:"key_names,omitempty"`
	Type                   *exportType         `json:"type,omitempty"`
	Defaults               []string            `json:"defaults,omitempty"`
	Config                 string              `json:"config"`
	Mandatory              bool                `json:"mandatory,omitempty"`
	ReadOnly               bool                `json:"read_only,omitempty"`
	Musts                  []string            `json:"musts,omitempty"`
	Whens                  []string            `json:"whens,omitempty"`
	Uniques                [][]string          `json:"uniques,omitempty"`
	Source                 exportSource        `json:"source"`
	Provenance             exportProvenance    `json:"provenance"`
}

type exportType struct {
	Base   string        `json:"base"`
	Range  []exportBound `json:"range,omitempty"`
	Length []exportBound `json:"length,omitempty"`
}

// exportBound is one range or length segment with min and max resolved to
// numbers, encoded as canonical decimal strings so 64-bit and decimal64 values
// keep full precision.
type exportBound struct {
	Min string `json:"min"`
	Max string `json:"max"`
}

type exportProvenance struct {
	Source              exportSource      `json:"source"`
	DefiningModule      string            `json:"defining_module,omitempty"`
	InstantiatingModule string            `json:"instantiating_module,omitempty"`
	AugmentingModule    string            `json:"augmenting_module,omitempty"`
	Grouping            string            `json:"grouping,omitempty"`
	Deviations          []exportDeviation `json:"deviations,omitempty"`
}

type exportDeviation struct {
	TargetPath   string   `json:"target_path"`
	SourceModule string   `json:"source_module"`
	Type         string   `json:"type"`
	Property     string   `json:"property,omitempty"`
	NewValue     string   `json:"new_value,omitempty"`
	IfFeatures   []string `json:"if_features,omitempty"`
}

type exportSource struct {
	File   string `json:"file,omitempty"`
	Line   int    `json:"line,omitempty"`
	Column int    `json:"column,omitempty"`
	Text   string `json:"text"`
}

type exportQualifiedName struct {
	Module    string `json:"module"`
	Prefix    string `json:"prefix,omitempty"`
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
}

type exportImport struct {
	Prefix   string `json:"prefix"`
	Name     string `json:"name"`
	Revision string `json:"revision,omitempty"`
}

type exportInclude struct {
	Name     string `json:"name"`
	Revision string `json:"revision,omitempty"`
}

type exportDiagnostic struct {
	Kind    string         `json:"kind"`
	Code    string         `json:"code"`
	Message string         `json:"message"`
	Module  string         `json:"module,omitempty"`
	Path    string         `json:"path,omitempty"`
	Source  exportSource   `json:"source"`
	Related []exportSource `json:"related,omitempty"`
}

func exportSchemaIR(ir cambium.SchemaIR) exportIR {
	out := exportIR{
		Version: ir.Version,
		Errors:  exportDiagnostics(ir.Errors),
		Modules: make([]exportModule, 0, len(ir.Modules)),
	}
	for _, module := range ir.Modules {
		out.Modules = append(out.Modules, exportSchemaIRModule(module))
	}
	return out
}

func exportSchemaIRModule(module cambium.SchemaIRModule) exportModule {
	return exportModule{
		Name:        module.Name,
		Namespace:   module.Namespace,
		Prefix:      module.Prefix,
		Revision:    module.Revision,
		Implemented: module.Implemented,
		Source:      exportLocation(module.Source),
		Imports:     exportImports(module.Imports),
		Includes:    exportIncludes(module.Includes),
		Children:    exportSchemaIRNodes(module.Children),
	}
}

func exportSchemaIRNodes(nodes []cambium.SchemaIRNode) []exportNode {
	if len(nodes) == 0 {
		return nil
	}
	out := make([]exportNode, 0, len(nodes))
	for _, node := range nodes {
		out = append(out, exportSchemaIRNode(node))
	}
	return out
}

type exportTable struct {
	Version string              `json:"version"`
	Modules []exportTableModule `json:"modules"`
	Nodes   []exportTableNode   `json:"nodes"`
	Errors  []exportDiagnostic  `json:"errors,omitempty"`
}

type exportTableModule struct {
	Name        string          `json:"name"`
	Namespace   string          `json:"namespace"`
	Prefix      string          `json:"prefix"`
	Revision    string          `json:"revision,omitempty"`
	Implemented bool            `json:"implemented"`
	Source      exportSource    `json:"source"`
	Imports     []exportImport  `json:"imports,omitempty"`
	Includes    []exportInclude `json:"includes,omitempty"`
	Children    []int           `json:"children,omitempty"`
}

// exportTableNode carries the v1 node fields with views as node IDs. parent is
// -1 at module top level.
type exportTableNode struct {
	ID                     int                 `json:"id"`
	Parent                 int                 `json:"parent"`
	Name                   string              `json:"name"`
	Kind                   string              `json:"kind"`
	LocalPath              string              `json:"local_path"`
	QualifiedPath          string              `json:"qualified_path"`
	NamespaceQualifiedPath string              `json:"namespace_qualified_path"`
	QualifiedName          exportQualifiedName `json:"qualified_name"`
	Children               []int               `json:"children,omitempty"`
	DataChildren           []int               `json:"data_children,omitempty"`
	ListKeys               []int               `json:"list_keys,omitempty"`
	KeyNames               []string            `json:"key_names,omitempty"`
	Type                   *exportType         `json:"type,omitempty"`
	Defaults               []string            `json:"defaults,omitempty"`
	Config                 string              `json:"config"`
	Mandatory              bool                `json:"mandatory,omitempty"`
	ReadOnly               bool                `json:"read_only,omitempty"`
	Musts                  []string            `json:"musts,omitempty"`
	Whens                  []string            `json:"whens,omitempty"`
	Uniques                [][]string          `json:"uniques,omitempty"`
	Source                 exportSource        `json:"source"`
	Provenance             exportProvenance    `json:"provenance"`
}

func exportSchemaIRTable(table cambium.SchemaIRTable) exportTable {
	out := exportTable{
		Version: table.Version,
		Errors:  exportDiagnostics(table.Errors),
		Modules: make([]exportTableModule, 0, len(table.Modules)),
		Nodes:   make([]exportTableNode, 0, len(table.Nodes)),
	}
	for _, module := range table.Modules {
		out.Modules = append(out.Modules, exportTableModule{
			Name:        module.Name,
			Namespace:   module.Namespace,
			Prefix:      module.Prefix,
			Revision:    module.Revision,
			Implemented: module.Implemented,
			Source:      exportLocation(module.Source),
			Imports:     exportImports(module.Imports),
			Includes:    exportIncludes(module.Includes),
			Children:    exportNodeIDs(module.Children),
		})
	}
	for _, node := range table.Nodes {
		exported := exportTableNode{
			ID:                     int(node.ID),
			Parent:                 int(node.Parent),
			Name:                   node.Name,
			Kind:                   node.Kind.String(),
			LocalPath:              node.LocalPath,
			QualifiedPath:          node.QualifiedPath,
			NamespaceQualifiedPath: node.NamespaceQualifiedPath,
			QualifiedName:          exportQName(node.QualifiedName),
			Children:               exportNodeIDs(node.Children),
			DataChildren:           exportNodeIDs(node.DataChildren),
			ListKeys:               exportNodeIDs(node.ListKeys),
			KeyNames:               append([]string(nil), node.KeyNames...),
			Config:                 exportConfig(node.Config),
			Mandatory:              node.Mandatory,
			ReadOnly:               node.ReadOnly,
			Defaults:               exportDefaults(node.Defaults),
			Musts:                  exportMusts(node.Musts),
			Whens:                  exportWhens(node.Whens),
			Uniques:                exportUniques(node.Uniques),
			Source:                 exportLocation(node.Source),
			Provenance:             exportNodeProvenance(node.Provenance),
		}
		exported.Type = exportTypeInfo(node.Type)
		out.Nodes = append(out.Nodes, exported)
	}
	return out
}

func exportNodeIDs(ids []cambium.SchemaIRNodeID) []int {
	if len(ids) == 0 {
		return nil
	}
	out := make([]int, len(ids))
	for i, id := range ids {
		out[i] = int(id)
	}
	return out
}

func exportSchemaIRNode(node cambium.SchemaIRNode) exportNode {
	out := exportNode{
		Name:                   node.Name,
		Kind:                   node.Kind.String(),
		LocalPath:              node.LocalPath,
		QualifiedPath:          node.QualifiedPath,
		NamespaceQualifiedPath: node.NamespaceQualifiedPath,
		QualifiedName:          exportQName(node.QualifiedName),
		Children:               exportSchemaIRNodes(node.Children),
		DataChildren:           exportSchemaIRNodes(node.DataChildren),
		ListKeys:               exportSchemaIRNodes(node.ListKeys),
		KeyNames:               append([]string(nil), node.KeyNames...),
		Config:                 exportConfig(node.Config),
		Mandatory:              node.Mandatory,
		ReadOnly:               node.ReadOnly,
		Defaults:               exportDefaults(node.Defaults),
		Musts:                  exportMusts(node.Musts),
		Whens:                  exportWhens(node.Whens),
		Uniques:                exportUniques(node.Uniques),
		Source:                 exportLocation(node.Source),
		Provenance:             exportNodeProvenance(node.Provenance),
	}
	out.Type = exportTypeInfo(node.Type)
	return out
}

// exportTypeInfo exports the base type name and the type's effective range or
// length restriction with resolved numeric bounds.
func exportTypeInfo(info *cambium.TypeInfo) *exportType {
	if info == nil {
		return nil
	}
	out := &exportType{Base: info.Base().String()}
	switch resolved := info.Resolved().(type) {
	case cambium.ResolvedInt:
		out.Range = exportBounds(resolved.Range)
	case cambium.ResolvedDecimal64:
		out.Range = exportBounds(resolved.Range)
	case cambium.ResolvedString:
		out.Length = exportBounds(resolved.Length)
	case cambium.ResolvedBinary:
		out.Length = exportBounds(resolved.Length)
	}
	return out
}

func exportBounds(bounds []cambium.RangeBound) []exportBound {
	if len(bounds) == 0 {
		return nil
	}
	out := make([]exportBound, 0, len(bounds))
	for _, bound := range bounds {
		exported := exportBound{Min: bound.Min(), Max: bound.Max()}
		if value, ok := bound.MinNumber(); ok {
			exported.Min = value.String()
		}
		if value, ok := bound.MaxNumber(); ok {
			exported.Max = value.String()
		}
		out = append(out, exported)
	}
	return out
}

func exportConfig(config cambium.Config) string {
	if config == cambium.ConfigRo {
		return "ro"
	}
	return "rw"
}

func exportDefaults(defaults []cambium.DefaultValue) []string {
	if len(defaults) == 0 {
		return nil
	}
	out := make([]string, 0, len(defaults))
	for _, def := range defaults {
		out = append(out, def.Value())
	}
	return out
}

func exportMusts(musts []cambium.MustConstraint) []string {
	if len(musts) == 0 {
		return nil
	}
	out := make([]string, 0, len(musts))
	for _, must := range musts {
		out = append(out, must.Expression())
	}
	return out
}

func exportWhens(whens []cambium.WhenConstraint) []string {
	if len(whens) == 0 {
		return nil
	}
	out := make([]string, 0, len(whens))
	for _, when := range whens {
		out = append(out, when.Expression())
	}
	return out
}

func exportUniques(uniques []cambium.UniqueConstraint) [][]string {
	if len(uniques) == 0 {
		return nil
	}
	out := make([][]string, 0, len(uniques))
	for _, unique := range uniques {
		var paths []string
		for _, leaf := range unique.Leafs() {
			paths = append(paths, leaf.LocalPath())
		}
		out = append(out, paths)
	}
	return out
}

func exportNodeProvenance(prov cambium.SchemaProvenance) exportProvenance {
	return exportProvenance{
		Source:              exportLocation(prov.Source),
		DefiningModule:      prov.DefiningModule,
		InstantiatingModule: prov.InstantiatingModule,
		AugmentingModule:    prov.AugmentingModule,
		Grouping:            prov.Grouping,
		Deviations:          exportDeviations(prov.Deviations),
	}
}

func exportLocation(loc cambium.SourceLocation) exportSource {
	return exportSource{
		File:   loc.File,
		Line:   loc.Line,
		Column: loc.Column,
		Text:   loc.Text,
	}
}

func exportQName(q cambium.QualifiedName) exportQualifiedName {
	return exportQualifiedName{
		Module:    q.Module,
		Prefix:    q.Prefix,
		Namespace: q.Namespace,
		Name:      q.Name,
	}
}

func exportImports(imports []cambium.Import) []exportImport {
	if len(imports) == 0 {
		return nil
	}
	out := make([]exportImport, 0, len(imports))
	for _, imp := range imports {
		out = append(out, exportImport{
			Prefix:   imp.Prefix,
			Name:     imp.Name,
			Revision: imp.Revision,
		})
	}
	return out
}

func exportIncludes(includes []cambium.Include) []exportInclude {
	if len(includes) == 0 {
		return nil
	}
	out := make([]exportInclude, 0, len(includes))
	for _, include := range includes {
		out = append(out, exportInclude{
			Name:     include.Name,
			Revision: include.Revision,
		})
	}
	return out
}

func exportDiagnostics(diags []cambium.Diagnostic) []exportDiagnostic {
	if len(diags) == 0 {
		return nil
	}
	out := make([]exportDiagnostic, 0, len(diags))
	for _, diag := range diags {
		related := make([]exportSource, 0, len(diag.Related))
		for _, loc := range diag.Related {
			related = append(related, exportLocation(loc))
		}
		out = append(out, exportDiagnostic{
			Kind:    string(diag.Kind),
			Code:    string(diag.Code),
			Message: diag.Message,
			Module:  diag.Module,
			Path:    diag.Path,
			Source:  exportLocation(diag.Source),
			Related: related,
		})
	}
	return out
}

func exportDeviations(deviations []cambium.Deviation) []exportDeviation {
	if len(deviations) == 0 {
		return nil
	}
	out := make([]exportDeviation, 0, len(deviations))
	for _, deviation := range deviations {
		out = append(out, exportDeviation{
			TargetPath:   deviation.TargetPath(),
			SourceModule: deviation.SourceModule(),
			Type:         deviation.Type(),
			Property:     deviation.Property(),
			NewValue:     deviation.NewValue(),
			IfFeatures:   deviation.IfFeatures(),
		})
	}
	return out
}
