// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

package compat

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"github.com/signalbreak-labs/cambium/go/cambium"
	"github.com/signalbreak-labs/cambium/go/internal/yangparse"
)

var revisionDateSuffixRegex = regexp.MustCompile(`^@\d{4}-\d{2}-\d{2}\.yang$`)
var missingModuleInSearchPathRegex = regexp.MustCompile(`module "([^"]+)" not found in search path`)

// Module is a minimal goyang-style module record owned by Modules.
type Module struct {
	Name       string       `yang:"Name,nomerge"`
	Source     *Statement   `yang:"Statement,nomerge" json:"-"`
	Parent     Node         `yang:"Parent,nomerge" json:"-"`
	Extensions []*Statement `yang:"Ext"`

	Anydata      []*AnyData      `yang:"anydata"`
	Anyxml       []*AnyXML       `yang:"anyxml"`
	Augment      []*Augment      `yang:"augment"`
	BelongsTo    *BelongsTo      `yang:"belongs-to,required=submodule,nomerge"`
	Choice       []*Choice       `yang:"choice"`
	Contact      *Value          `yang:"contact,nomerge"`
	Container    []*Container    `yang:"container"`
	Description  *Value          `yang:"description,nomerge"`
	Deviation    []*Deviation    `yang:"deviation"`
	Extension    []*Extension    `yang:"extension"`
	Feature      []*Feature      `yang:"feature"`
	Grouping     []*Grouping     `yang:"grouping"`
	Identity     []*Identity     `yang:"identity"`
	Import       []*Import       `yang:"import"`
	Include      []*Include      `yang:"include"`
	Leaf         []*Leaf         `yang:"leaf"`
	LeafList     []*LeafList     `yang:"leaf-list"`
	List         []*List         `yang:"list"`
	Namespace    *Value          `yang:"namespace,required=module,nomerge"`
	Notification []*Notification `yang:"notification"`
	Organization *Value          `yang:"organization,nomerge"`
	Prefix       *Value          `yang:"prefix,required=module,nomerge"`
	Reference    *Value          `yang:"reference,nomerge"`
	Revision     []*Revision     `yang:"revision,nomerge"`
	RPC          []*RPC          `yang:"rpc"`
	Typedef      []*Typedef      `yang:"typedef"`
	Uses         []*Uses         `yang:"uses"`
	YangVersion  *Value          `yang:"yang-version,nomerge"`
	Modules      *Modules

	// schema/entry associate this *Module with its compiled Cambium schema and
	// goyang-style Entry. They live on the struct (not a process-global map) so
	// they are released with the Module and isolated per parse. Unexported, so
	// they stay off the goyang-compatible public surface.
	schema    cambium.Module
	schemaSet bool
	entry     *Entry
}

func setModuleSchema(m *Module, schema cambium.Module) {
	if m != nil {
		m.schema = schema
		m.schemaSet = true
	}
}

func moduleSchema(m *Module) (cambium.Module, bool) {
	if m == nil || !m.schemaSet {
		return cambium.Module{}, false
	}
	return m.schema, true
}

func setModuleEntry(m *Module, entry *Entry) {
	if m != nil && entry != nil {
		m.entry = entry
	}
}

func moduleEntry(m *Module) (*Entry, bool) {
	if m == nil || m.entry == nil {
		return nil, false
	}
	return m.entry, true
}

func clearModuleEntry(m *Module) {
	if m != nil {
		m.entry = nil
	}
}

// Kind returns "module" or "submodule".
func (m *Module) Kind() string {
	if m != nil && m.BelongsTo != nil {
		return "submodule"
	}
	return "module"
}

// ParentNode returns the parser parent node, if any.
func (m *Module) ParentNode() Node {
	if m == nil {
		return nil
	}
	return m.Parent
}

// NName returns the module name.
func (m *Module) NName() string {
	if m == nil {
		return ""
	}
	return m.Name
}

// Statement returns the parsed source statement for this module, if available.
func (m *Module) Statement() *Statement {
	if m == nil {
		return nil
	}
	return m.Source
}

// Exts returns extension statements attached to the module statement.
func (m *Module) Exts() []*Statement {
	if m == nil {
		return nil
	}
	return append([]*Statement(nil), m.Extensions...)
}

// Groupings returns top-level grouping AST nodes in source declaration order.
func (m *Module) Groupings() []*Grouping {
	if m == nil {
		return nil
	}
	return append([]*Grouping(nil), m.Grouping...)
}

// Typedefs returns top-level typedef AST nodes in source declaration order.
func (m *Module) Typedefs() []*Typedef {
	if m == nil {
		return nil
	}
	return append([]*Typedef(nil), m.Typedef...)
}

// Identities returns top-level identity AST nodes in source declaration order.
func (m *Module) Identities() []*Identity {
	if m == nil {
		return nil
	}
	return append([]*Identity(nil), m.Identity...)
}

// Current returns the most recent revision of this module, or "" if none is declared.
func (m *Module) Current() string {
	if m == nil {
		return ""
	}
	var current string
	for _, rev := range m.Revision {
		if rev != nil && rev.Name > current {
			current = rev.Name
		}
	}
	return current
}

// FullName returns the module name with its current revision suffix, if any.
func (m *Module) FullName() string {
	if m == nil {
		return ""
	}
	if rev := m.Current(); rev != "" {
		return m.Name + "@" + rev
	}
	return m.Name
}

// GetPrefix returns the module's local prefix.
func (m *Module) GetPrefix() string {
	if m == nil {
		return ""
	}
	if m.BelongsTo != nil {
		if m.BelongsTo.Prefix != nil {
			return m.BelongsTo.Prefix.Name
		}
		return ""
	}
	if m.Prefix == nil {
		return ""
	}
	return m.Prefix.Name
}

// Options defines parse/process options accepted for goyang API compatibility.
type Options struct {
	IgnoreSubmoduleCircularDependencies bool
	StoreUses                           bool
	DeviateOptions                      DeviateOptions
}

// DeviateOptions defines deviation handling options for API compatibility.
type DeviateOptions struct {
	IgnoreDeviateNotSupported bool
}

// IsDeviateOpt marks DeviateOptions as a valid ApplyDeviate option.
func (DeviateOptions) IsDeviateOpt() {}

// DeviateOpt is accepted by Entry.ApplyDeviate for goyang API compatibility.
type DeviateOpt interface {
	IsDeviateOpt()
}

type moduleSource struct {
	data string
	name string
	path string
}

type stagedInMemorySources struct {
	dir        string
	modulePath map[int][]string
}

type moduleDecl struct {
	name         string
	source       *Statement
	submodule    bool
	belongsTo    *BelongsTo
	extensions   []*Statement
	namespace    string
	prefix       string
	contact      string
	description  string
	organization string
	reference    string
	yangVersion  string
	revisions    []*Revision
	imports      []*Import
	includes     []*Include
	groupings    []*Grouping
	typedefs     []*Typedef
	identities   []*Identity
}

// Modules is a cgo-free goyang-style module loader facade.
type Modules struct {
	Modules    map[string]*Module
	SubModules map[string]*Module

	ParseOptions Options
	Path         []string

	nsMu    sync.Mutex
	byNS    map[string]*Module
	pathMap map[string]bool
	sources []moduleSource
	ctx     *cambium.Context
	built   bool
}

// NewModules returns an initialized Modules facade.
func NewModules() *Modules {
	return &Modules{
		Modules:    make(map[string]*Module),
		SubModules: make(map[string]*Module),
		byNS:       make(map[string]*Module),
		pathMap:    make(map[string]bool),
	}
}

// AddPath appends module search paths.
func (ms *Modules) AddPath(paths ...string) {
	if ms == nil {
		return
	}
	if ms.pathMap == nil {
		ms.pathMap = make(map[string]bool)
	}
	changed := false
	for _, pathList := range paths {
		for _, path := range strings.Split(pathList, ":") {
			if ms.pathMap[path] {
				continue
			}
			ms.pathMap[path] = true
			ms.Path = append(ms.Path, path)
			changed = true
		}
	}
	if changed {
		ms.built = false
		ms.resetNamespaceCache()
	}
}

// Process builds a Cambium context for all parsed/read modules.
func (ms *Modules) Process() []error {
	if ms == nil {
		return []error{fmt.Errorf("nil Modules")}
	}
	if ms.built {
		return nil
	}
	builder, err := cambium.NewContextBuilder(cambium.ContextFlags{})
	if err != nil {
		return []error{err}
	}
	staged, cleanup, err := ms.stageInMemorySources()
	if err != nil {
		return []error{err}
	}
	defer cleanup()
	if staged.dir != "" {
		if err := builder.SearchPath(staged.dir); err != nil {
			return []error{err}
		}
	}
	for _, path := range ms.Path {
		if err := builder.SearchPath(path); err != nil {
			return []error{err}
		}
	}
	for i, source := range ms.sources {
		if source.path != "" {
			if err := builder.LoadModuleFromPath(source.path); err != nil {
				return []error{ms.translateProcessError(err)}
			}
			continue
		}
		for _, path := range staged.modulePath[i] {
			if err := builder.LoadModuleFromPath(path); err != nil {
				return []error{ms.translateProcessError(err)}
			}
		}
	}
	ctx, err := builder.Build()
	if err != nil {
		return []error{ms.translateProcessError(err)}
	}
	ms.closeContext()
	ms.ctx = ctx
	ms.built = true
	ms.resetNamespaceCache()
	for _, mod := range ctx.Modules() {
		ms.recordModule(mod)
	}
	if ms.ParseOptions.DeviateOptions.IgnoreDeviateNotSupported {
		if errs := ms.rebuildSourceEntriesWithDeviateOptions(); len(errs) != 0 {
			return errs
		}
	}
	return nil
}

// ClearEntryCache clears cached Entry projections built for this module set.
func (ms *Modules) ClearEntryCache() {
	if ms == nil {
		return
	}
	for _, module := range ms.Modules {
		clearModuleEntry(module)
	}
	for _, module := range ms.SubModules {
		clearModuleEntry(module)
	}
}

func (ms *Modules) rebuildSourceEntriesWithDeviateOptions() []error {
	records := ms.moduleRecords()
	for _, record := range records {
		if record == nil || record.Source == nil {
			continue
		}
		clearModuleEntry(record)
		entry := entryFromCompatModuleSource(record)
		if entry == nil {
			continue
		}
		entry.modules = ms
		setModuleEntry(record, entry)
	}

	entries := make([]*Entry, 0, len(records))
	for _, record := range records {
		if entry, ok := moduleEntry(record); ok {
			entries = append(entries, entry)
		}
	}

	var errs []error
	for _, entry := range entries {
		errs = append(errs, entry.GetErrors()...)
	}
	if len(errs) != 0 {
		return errs
	}

	unresolved := append([]*Entry(nil), entries...)
	for len(unresolved) > 0 {
		processed := 0
		for i := 0; i < len(unresolved); {
			entry := unresolved[i]
			p, s := entry.Augment(false)
			processed += p
			if s == 0 {
				unresolved[i] = unresolved[len(unresolved)-1]
				unresolved = unresolved[:len(unresolved)-1]
				continue
			}
			i++
		}
		if processed == 0 {
			break
		}
	}

	for _, entry := range entries {
		entry.FixChoice()
	}
	for _, entry := range unresolved {
		entry.Augment(true)
		errs = append(errs, entry.GetErrors()...)
	}
	if len(errs) != 0 {
		return errs
	}

	applied := map[string]bool{}
	for _, entry := range entries {
		if applied[entry.Name] {
			continue
		}
		errs = append(errs, entry.ApplyDeviate(ms.ParseOptions.DeviateOptions)...)
		applied[entry.Name] = true
	}
	return errs
}

func (ms *Modules) moduleRecords() []*Module {
	if ms == nil {
		return nil
	}
	seen := map[*Module]bool{}
	var records []*Module
	for _, modules := range []map[string]*Module{ms.Modules, ms.SubModules} {
		for _, module := range modules {
			if module == nil || seen[module] {
				continue
			}
			seen[module] = true
			records = append(records, module)
		}
	}
	return records
}

func (ms *Modules) translateProcessError(err error) error {
	if err == nil {
		return nil
	}
	matches := missingModuleInSearchPathRegex.FindStringSubmatch(err.Error())
	if len(matches) != 2 {
		return err
	}
	name := matches[1]
	if ms.referencesInclude(name) {
		return fmt.Errorf("no such submodule: %s", name)
	}
	if ms.referencesImport(name) {
		return fmt.Errorf("no such module: %s", name)
	}
	return err
}

func (ms *Modules) cacheNamespace(ns string, record *Module) {
	if ms == nil || record == nil {
		return
	}
	ms.nsMu.Lock()
	if ms.byNS == nil {
		ms.byNS = make(map[string]*Module)
	}
	ms.byNS[ns] = record
	ms.nsMu.Unlock()
}

func (ms *Modules) stageInMemorySources() (stagedInMemorySources, func(), error) {
	staged := stagedInMemorySources{modulePath: make(map[int][]string)}
	if ms == nil {
		return staged, func() {}, nil
	}

	var dir string
	written := make(map[string]bool)
	cleanup := func() {
		if dir != "" {
			_ = os.RemoveAll(dir)
		}
	}
	for i, source := range ms.sources {
		if source.path != "" {
			continue
		}
		stmts, err := yangparse.Parse(source.data, source.name)
		if err != nil {
			cleanup()
			return stagedInMemorySources{}, func() {}, err
		}
		for _, stmt := range stmts {
			if stmt == nil || (stmt.Keyword != "module" && stmt.Keyword != "submodule") {
				continue
			}
			if dir == "" {
				dir, err = os.MkdirTemp("", "cambium-compat-yang-*")
				if err != nil {
					cleanup()
					return stagedInMemorySources{}, func() {}, err
				}
				staged.dir = dir
			}
			name, err := stagedSourceFilename(stmt)
			if err != nil {
				cleanup()
				return stagedInMemorySources{}, func() {}, err
			}
			path := filepath.Join(dir, name)
			if written[path] {
				cleanup()
				return stagedInMemorySources{}, func() {}, fmt.Errorf("duplicate in-memory YANG source %q", name)
			}
			var rendered strings.Builder
			if err := stmt.Write(&rendered, ""); err != nil {
				cleanup()
				return stagedInMemorySources{}, func() {}, err
			}
			if err := os.WriteFile(path, []byte(rendered.String()), 0o600); err != nil {
				cleanup()
				return stagedInMemorySources{}, func() {}, err
			}
			written[path] = true
			if stmt.Keyword == "module" {
				staged.modulePath[i] = append(staged.modulePath[i], path)
			}
		}
	}
	return staged, cleanup, nil
}

func stagedSourceFilename(stmt *Statement) (string, error) {
	if stmt == nil {
		return "", fmt.Errorf("nil YANG source statement")
	}
	if !safeYANGModuleFilenamePart(stmt.Argument) {
		return "", fmt.Errorf("unsafe %s name %q for in-memory YANG source", stmt.Keyword, stmt.Argument)
	}
	name := stmt.Argument
	if revision := statementLatestRevision(stmt); revision != "" {
		name += "@" + revision
	}
	return name + ".yang", nil
}

func safeYANGModuleFilenamePart(name string) bool {
	if name == "" || name == "." || name == ".." {
		return false
	}
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= 'A' && r <= 'Z':
		case r >= '0' && r <= '9':
		case r == '_' || r == '-' || r == '.':
		default:
			return false
		}
	}
	return true
}

func statementLatestRevision(stmt *Statement) string {
	var latest string
	for _, child := range stmt.SubStatements() {
		if child.Keyword == "revision" && child.Argument > latest {
			latest = child.Argument
		}
	}
	return latest
}

func (ms *Modules) recordModule(mod cambium.Module) *Module {
	decl := moduleDecl{
		name:        mod.Name(),
		namespace:   mod.Namespace(),
		prefix:      mod.Prefix(),
		yangVersion: mod.YangVersion(),
		imports:     importsFromCambium(mod.Imports()),
		includes:    includesFromCambium(mod.Includes()),
		revisions:   revisionsFromCambium(mod.Revisions()),
	}
	if organization, ok := mod.Organization(); ok {
		decl.organization = organization
	}
	if contact, ok := mod.Contact(); ok {
		decl.contact = contact
	}
	if description, ok := mod.Description(); ok {
		decl.description = description
	}
	if reference, ok := mod.Reference(); ok {
		decl.reference = reference
	}
	record := ms.addModuleDecl(decl)
	setModuleSchema(record, mod)
	entry := FromModule(mod)
	entry.modules = ms
	if ms.ParseOptions.StoreUses {
		attachStoredUsesFromNode(entry, record)
	}
	setModuleEntry(record, entry)
	return record
}

func (ms *Modules) addModuleDecl(decl moduleDecl) *Module {
	if ms.Modules == nil {
		ms.Modules = make(map[string]*Module)
	}
	if ms.SubModules == nil {
		ms.SubModules = make(map[string]*Module)
	}
	target := ms.Modules
	if decl.submodule {
		target = ms.SubModules
	}
	fullName := decl.fullName()
	record := target[fullName]
	if record == nil {
		record = &Module{Name: decl.name}
		target[fullName] = record
	}
	record.Modules = ms
	record.Name = decl.name
	if decl.source != nil {
		record.Source = decl.source
	}
	source := decl.source
	if source == nil {
		source = record.Source
	}
	if decl.belongsTo != nil {
		record.BelongsTo = cloneBelongsTo(decl.belongsTo)
		record.BelongsTo.Parent = record
	} else if decl.submodule && record.BelongsTo == nil {
		record.BelongsTo = &BelongsTo{Name: decl.name, Prefix: cloneASTValue(astValueOrNil(decl.prefix)), Parent: record}
	}
	if len(decl.extensions) != 0 {
		record.Extensions = append([]*Statement(nil), decl.extensions...)
	}
	if decl.namespace != "" {
		record.Namespace = moduleValueChild(source, "namespace", decl.namespace, record)
	} else if decl.namespace == "" && !decl.submodule {
		record.Namespace = nil
	}
	if decl.prefix != "" && !decl.submodule {
		record.Prefix = moduleValueChild(source, "prefix", decl.prefix, record)
	} else if decl.submodule {
		record.Prefix = nil
	}
	if decl.contact != "" {
		record.Contact = moduleValueChild(source, "contact", decl.contact, record)
	}
	if decl.description != "" {
		record.Description = moduleValueChild(source, "description", decl.description, record)
	}
	if decl.organization != "" {
		record.Organization = moduleValueChild(source, "organization", decl.organization, record)
	}
	if decl.reference != "" {
		record.Reference = moduleValueChild(source, "reference", decl.reference, record)
	}
	if decl.yangVersion != "" {
		record.YangVersion = moduleValueChild(source, "yang-version", decl.yangVersion, record)
	}
	if len(decl.revisions) != 0 {
		record.Revision = moduleRevisions(source, decl.revisions, record)
	}
	if len(decl.imports) != 0 {
		record.Import = cloneImports(decl.imports, record)
	}
	if len(decl.includes) != 0 {
		record.Include = cloneIncludes(decl.includes, record)
	}
	if len(decl.groupings) != 0 {
		record.Grouping = cloneGroupings(decl.groupings, record)
	}
	if len(decl.typedefs) != 0 {
		record.Typedef = cloneTypedefs(decl.typedefs, record)
	}
	if len(decl.identities) != 0 {
		record.Identity = cloneIdentities(decl.identities, record)
	}
	populateModuleTopLevelAST(record)

	bare := target[decl.name]
	if bare == nil || bare.FullName() < record.FullName() {
		target[decl.name] = record
	}
	return record
}

func (ms *Modules) duplicateModuleDeclError(decl moduleDecl) error {
	if ms == nil {
		return fmt.Errorf("nil Modules")
	}
	target := ms.Modules
	kind := "module"
	if decl.submodule {
		target = ms.SubModules
		kind = "submodule"
	}
	if target == nil {
		return nil
	}
	fullName := decl.fullName()
	if existing := target[fullName]; existing != nil {
		return fmt.Errorf("duplicate %s %s at %s and %s", kind, fullName, Source(existing), sourceOfModuleDecl(decl))
	}
	return nil
}

func sourceOfModuleDecl(decl moduleDecl) string {
	if decl.source == nil {
		return "unknown"
	}
	return decl.source.Location()
}

func moduleFromASTModule(ast *ASTModule) *Module {
	if ast == nil {
		return nil
	}
	if ast.Source != nil {
		modules := &Modules{}
		record := modules.addModuleDecl(moduleDeclFromStatement(ast.Source, "", ast.BelongsTo != nil))
		record.Parent = ast.Parent
		return record
	}

	record := &Module{
		Name:       ast.Name,
		Source:     ast.Source,
		Parent:     ast.Parent,
		Extensions: append([]*Statement(nil), ast.Extensions...),
	}
	record.Modules = &Modules{Modules: map[string]*Module{record.Name: record}}
	record.Revision = cloneRevisions(ast.Revision, record)
	record.YangVersion = valueFromASTValue(ast.YangVersion, record)
	record.Namespace = valueFromASTValue(ast.Namespace, record)
	record.Prefix = valueFromASTValue(ast.Prefix, record)
	record.Contact = valueFromASTValue(ast.Contact, record)
	record.Description = valueFromASTValue(ast.Description, record)
	record.Organization = valueFromASTValue(ast.Organization, record)
	record.Reference = valueFromASTValue(ast.Reference, record)
	record.Anydata = cloneAnyDataWithParent(ast.Anydata, record)
	record.Anyxml = cloneAnyXMLWithParent(ast.Anyxml, record)
	record.Augment = cloneAugmentsWithParent(ast.Augment, record)
	record.Choice = cloneChoicesWithParent(ast.Choice, record)
	record.Container = cloneContainersWithParent(ast.Container, record)
	record.Deviation = cloneDeviationsWithParent(ast.Deviation, record)
	record.Extension = cloneExtensionsWithParent(ast.Extension, record)
	record.Feature = cloneFeaturesWithParent(ast.Feature, record)
	record.Import = cloneImports(ast.Import, record)
	record.Include = cloneIncludes(ast.Include, record)
	record.Grouping = cloneGroupings(ast.Grouping, record)
	record.Typedef = cloneTypedefs(ast.Typedef, record)
	record.Identity = identitiesFromASTIdentities(ast.Identity, record)
	record.Leaf = cloneLeavesWithParent(ast.Leaf, record)
	record.LeafList = cloneLeafListsWithParent(ast.LeafList, record)
	record.List = cloneListsWithParent(ast.List, record)
	record.Notification = cloneNotificationsWithParent(ast.Notification, record)
	record.RPC = cloneRPCsWithParent(ast.RPC, record)
	record.Uses = cloneUsesWithParent(ast.Uses, record)
	record.BelongsTo = cloneBelongsTo(ast.BelongsTo)
	if record.BelongsTo != nil {
		record.BelongsTo.Parent = record
	}
	return record
}

func modulesFromASTModuleSet(ast *ASTModule) *Modules {
	if ast == nil {
		return nil
	}
	if ast.Modules == nil {
		if record := moduleFromASTModule(ast); record != nil {
			return record.Modules
		}
		return nil
	}
	modules := NewModules()
	for _, module := range ast.Modules.Modules {
		modules.addASTModuleRecord(module)
	}
	for _, submodule := range ast.Modules.SubModules {
		modules.addASTModuleRecord(submodule)
	}
	return modules
}

func (ms *Modules) addASTModuleRecord(ast *ASTModule) *Module {
	if ms == nil || ast == nil {
		return nil
	}
	if ast.Source != nil {
		record := ms.addModuleDecl(moduleDeclFromStatement(ast.Source, "", ast.BelongsTo != nil))
		record.Parent = ast.Parent
		return record
	}
	record := moduleFromASTModule(ast)
	if record == nil {
		return nil
	}
	record.Modules = ms
	target := ms.Modules
	if record.BelongsTo != nil {
		target = ms.SubModules
	}
	target[record.FullName()] = record
	if bare := target[record.Name]; bare == nil || bare.FullName() < record.FullName() {
		target[record.Name] = record
	}
	return record
}

func (decl moduleDecl) fullName() string {
	if rev := latestRevision(decl.revisions); rev != "" {
		return decl.name + "@" + rev
	}
	return decl.name
}

func moduleDeclFromStatement(stmt *Statement, _ string, submodule bool) moduleDecl {
	decl := moduleDecl{
		name:      stmt.Argument,
		source:    stmt,
		submodule: submodule,
	}
	for _, child := range stmt.SubStatements() {
		switch child.Keyword {
		case "namespace":
			decl.namespace = child.Argument
		case "prefix":
			decl.prefix = child.Argument
		case "contact":
			decl.contact = child.Argument
		case "description":
			decl.description = child.Argument
		case "organization":
			decl.organization = child.Argument
		case "reference":
			decl.reference = child.Argument
		case "yang-version":
			decl.yangVersion = child.Argument
		case "revision":
			decl.revisions = append(decl.revisions, revisionFromStatement(child))
		case "import":
			decl.imports = append(decl.imports, importFromStatement(child))
		case "include":
			decl.includes = append(decl.includes, includeFromStatement(child))
		case "grouping":
			decl.groupings = append(decl.groupings, groupingFromStatement(child))
		case "typedef":
			decl.typedefs = append(decl.typedefs, typedefFromStatement(child))
		case "identity":
			decl.identities = append(decl.identities, astIdentityFromStatement(child))
		case "belongs-to":
			decl.belongsTo = belongsToFromStatement(child)
			if submodule && decl.prefix == "" {
				decl.prefix = childArgument(child, "prefix")
			}
		default:
			if strings.Contains(child.Keyword, ":") {
				decl.extensions = append(decl.extensions, child)
			}
		}
	}
	return decl
}

func populateModuleTopLevelAST(module *Module) {
	if module == nil || module.Source == nil {
		return
	}
	module.Anydata = nil
	module.Anyxml = nil
	module.Augment = nil
	module.Choice = nil
	module.Container = nil
	module.Deviation = nil
	module.Extension = nil
	module.Feature = nil
	module.Leaf = nil
	module.LeafList = nil
	module.List = nil
	module.Notification = nil
	module.RPC = nil
	module.Uses = nil
	module.Grouping = nil
	module.Typedef = nil
	module.Identity = nil

	for _, child := range module.Source.SubStatements() {
		switch child.Keyword {
		case "anydata":
			module.Anydata = append(module.Anydata, anyDataFromStatement(child, module))
		case "anyxml":
			module.Anyxml = append(module.Anyxml, anyXMLFromStatement(child, module))
		case "augment":
			module.Augment = append(module.Augment, augmentFromStatement(child, module))
		case "choice":
			module.Choice = append(module.Choice, choiceFromStatement(child, module))
		case "container":
			module.Container = append(module.Container, containerFromStatement(child, module))
		case "deviation":
			module.Deviation = append(module.Deviation, deviationFromStatement(child, module))
		case "extension":
			module.Extension = append(module.Extension, extensionDefFromStatement(child, module))
		case "feature":
			module.Feature = append(module.Feature, featureFromStatement(child, module))
		case "grouping":
			module.Grouping = append(module.Grouping, groupingFromStatementWithParent(child, module))
		case "identity":
			module.Identity = append(module.Identity, astIdentityFromStatementWithParent(child, module))
		case "leaf":
			module.Leaf = append(module.Leaf, leafFromStatement(child, module))
		case "leaf-list":
			module.LeafList = append(module.LeafList, leafListFromStatement(child, module))
		case "list":
			module.List = append(module.List, listFromStatement(child, module))
		case "notification":
			module.Notification = append(module.Notification, notificationFromStatement(child, module))
		case "rpc":
			module.RPC = append(module.RPC, rpcFromStatement(child, module))
		case "typedef":
			module.Typedef = append(module.Typedef, typedefFromStatementWithParent(child, module))
		case "uses":
			module.Uses = append(module.Uses, usesFromStatement(child, module))
		}
	}
}

func childArgument(stmt *Statement, keyword string) string {
	for _, child := range stmt.SubStatements() {
		if child.Keyword == keyword {
			return child.Argument
		}
	}
	return ""
}

func latestRevision(revisions []*Revision) string {
	var latest string
	for _, revision := range revisions {
		if revision != nil && revision.Name > latest {
			latest = revision.Name
		}
	}
	return latest
}

func revisionFromStatement(stmt *Statement) *Revision {
	if stmt == nil || stmt.Argument == "" {
		return nil
	}
	revision := &Revision{
		Name:       stmt.Argument,
		Source:     stmt,
		Extensions: extensionStatements(stmt),
	}
	revision.Description = astValueChild(stmt, "description", revision)
	revision.Reference = astValueChild(stmt, "reference", revision)
	return revision
}

func anyDataFromStatement(stmt *Statement, parent Node) *AnyData {
	if stmt == nil {
		return nil
	}
	anydata := &AnyData{
		Name:       stmt.Argument,
		Source:     stmt,
		Parent:     parent,
		Extensions: extensionStatements(stmt),
	}
	anydata.Config = astValueChild(stmt, "config", anydata)
	anydata.Description = astValueChild(stmt, "description", anydata)
	anydata.Mandatory = astValueChild(stmt, "mandatory", anydata)
	anydata.Reference = astValueChild(stmt, "reference", anydata)
	anydata.Status = astValueChild(stmt, "status", anydata)
	anydata.When = astValueChild(stmt, "when", anydata)
	for _, child := range stmt.SubStatements() {
		switch child.Keyword {
		case "if-feature":
			anydata.IfFeature = append(anydata.IfFeature, &ASTValue{Name: child.Argument, Source: child, Parent: anydata, Extensions: extensionStatements(child)})
		case "must":
			anydata.Must = append(anydata.Must, mustFromStatement(child, anydata))
		}
	}
	return anydata
}

func anyXMLFromStatement(stmt *Statement, parent Node) *AnyXML {
	if stmt == nil {
		return nil
	}
	anyxml := &AnyXML{
		Name:       stmt.Argument,
		Source:     stmt,
		Parent:     parent,
		Extensions: extensionStatements(stmt),
	}
	anyxml.Config = astValueChild(stmt, "config", anyxml)
	anyxml.Description = astValueChild(stmt, "description", anyxml)
	anyxml.Mandatory = astValueChild(stmt, "mandatory", anyxml)
	anyxml.Reference = astValueChild(stmt, "reference", anyxml)
	anyxml.Status = astValueChild(stmt, "status", anyxml)
	anyxml.When = astValueChild(stmt, "when", anyxml)
	for _, child := range stmt.SubStatements() {
		switch child.Keyword {
		case "if-feature":
			anyxml.IfFeature = append(anyxml.IfFeature, &ASTValue{Name: child.Argument, Source: child, Parent: anyxml, Extensions: extensionStatements(child)})
		case "must":
			anyxml.Must = append(anyxml.Must, mustFromStatement(child, anyxml))
		}
	}
	return anyxml
}

func augmentFromStatement(stmt *Statement, parent Node) *Augment {
	if stmt == nil {
		return nil
	}
	augment := &Augment{
		Name:       stmt.Argument,
		Source:     stmt,
		Parent:     parent,
		Extensions: extensionStatements(stmt),
	}
	augment.Description = astValueChild(stmt, "description", augment)
	augment.Reference = astValueChild(stmt, "reference", augment)
	augment.Status = astValueChild(stmt, "status", augment)
	augment.When = astValueChild(stmt, "when", augment)
	for _, child := range stmt.SubStatements() {
		switch child.Keyword {
		case "action":
			augment.Action = append(augment.Action, actionFromStatement(child, augment))
		case "anydata":
			augment.Anydata = append(augment.Anydata, anyDataFromStatement(child, augment))
		case "anyxml":
			augment.Anyxml = append(augment.Anyxml, anyXMLFromStatement(child, augment))
		case "case":
			augment.Case = append(augment.Case, caseFromStatement(child, augment))
		case "choice":
			augment.Choice = append(augment.Choice, choiceFromStatement(child, augment))
		case "container":
			augment.Container = append(augment.Container, containerFromStatement(child, augment))
		case "if-feature":
			augment.IfFeature = append(augment.IfFeature, &ASTValue{Name: child.Argument, Source: child, Parent: augment, Extensions: extensionStatements(child)})
		case "leaf":
			augment.Leaf = append(augment.Leaf, leafFromStatement(child, augment))
		case "leaf-list":
			augment.LeafList = append(augment.LeafList, leafListFromStatement(child, augment))
		case "list":
			augment.List = append(augment.List, listFromStatement(child, augment))
		case "notification":
			augment.Notification = append(augment.Notification, notificationFromStatement(child, augment))
		case "uses":
			augment.Uses = append(augment.Uses, usesFromStatement(child, augment))
		}
	}
	return augment
}

func choiceFromStatement(stmt *Statement, parent Node) *Choice {
	if stmt == nil {
		return nil
	}
	choice := &Choice{
		Name:       stmt.Argument,
		Source:     stmt,
		Parent:     parent,
		Extensions: extensionStatements(stmt),
	}
	choice.Config = astValueChild(stmt, "config", choice)
	choice.Default = astValueChild(stmt, "default", choice)
	choice.Description = astValueChild(stmt, "description", choice)
	choice.Mandatory = astValueChild(stmt, "mandatory", choice)
	choice.Reference = astValueChild(stmt, "reference", choice)
	choice.Status = astValueChild(stmt, "status", choice)
	choice.When = astValueChild(stmt, "when", choice)
	for _, child := range stmt.SubStatements() {
		switch child.Keyword {
		case "anydata":
			choice.Anydata = append(choice.Anydata, anyDataFromStatement(child, choice))
		case "anyxml":
			choice.Anyxml = append(choice.Anyxml, anyXMLFromStatement(child, choice))
		case "case":
			choice.Case = append(choice.Case, caseFromStatement(child, choice))
		case "container":
			choice.Container = append(choice.Container, containerFromStatement(child, choice))
		case "if-feature":
			choice.IfFeature = append(choice.IfFeature, &ASTValue{Name: child.Argument, Source: child, Parent: choice, Extensions: extensionStatements(child)})
		case "leaf":
			choice.Leaf = append(choice.Leaf, leafFromStatement(child, choice))
		case "leaf-list":
			choice.LeafList = append(choice.LeafList, leafListFromStatement(child, choice))
		case "list":
			choice.List = append(choice.List, listFromStatement(child, choice))
		}
	}
	return choice
}

func caseFromStatement(stmt *Statement, parent Node) *Case {
	if stmt == nil {
		return nil
	}
	cas := &Case{
		Name:       stmt.Argument,
		Source:     stmt,
		Parent:     parent,
		Extensions: extensionStatements(stmt),
	}
	cas.Description = astValueChild(stmt, "description", cas)
	cas.Reference = astValueChild(stmt, "reference", cas)
	cas.Status = astValueChild(stmt, "status", cas)
	cas.When = astValueChild(stmt, "when", cas)
	for _, child := range stmt.SubStatements() {
		switch child.Keyword {
		case "anydata":
			cas.Anydata = append(cas.Anydata, anyDataFromStatement(child, cas))
		case "anyxml":
			cas.Anyxml = append(cas.Anyxml, anyXMLFromStatement(child, cas))
		case "choice":
			cas.Choice = append(cas.Choice, choiceFromStatement(child, cas))
		case "container":
			cas.Container = append(cas.Container, containerFromStatement(child, cas))
		case "if-feature":
			cas.IfFeature = append(cas.IfFeature, &ASTValue{Name: child.Argument, Source: child, Parent: cas, Extensions: extensionStatements(child)})
		case "leaf":
			cas.Leaf = append(cas.Leaf, leafFromStatement(child, cas))
		case "leaf-list":
			cas.LeafList = append(cas.LeafList, leafListFromStatement(child, cas))
		case "list":
			cas.List = append(cas.List, listFromStatement(child, cas))
		case "uses":
			cas.Uses = append(cas.Uses, usesFromStatement(child, cas))
		}
	}
	return cas
}

func containerFromStatement(stmt *Statement, parent Node) *Container {
	if stmt == nil {
		return nil
	}
	container := &Container{
		Name:       stmt.Argument,
		Source:     stmt,
		Parent:     parent,
		Extensions: extensionStatements(stmt),
	}
	container.Config = astValueChild(stmt, "config", container)
	container.Description = astValueChild(stmt, "description", container)
	container.Presence = astValueChild(stmt, "presence", container)
	container.Reference = astValueChild(stmt, "reference", container)
	container.Status = astValueChild(stmt, "status", container)
	container.When = astValueChild(stmt, "when", container)
	for _, child := range stmt.SubStatements() {
		switch child.Keyword {
		case "action":
			container.Action = append(container.Action, actionFromStatement(child, container))
		case "anydata":
			container.Anydata = append(container.Anydata, anyDataFromStatement(child, container))
		case "anyxml":
			container.Anyxml = append(container.Anyxml, anyXMLFromStatement(child, container))
		case "choice":
			container.Choice = append(container.Choice, choiceFromStatement(child, container))
		case "container":
			container.Container = append(container.Container, containerFromStatement(child, container))
		case "grouping":
			container.Grouping = append(container.Grouping, groupingFromStatementWithParent(child, container))
		case "if-feature":
			container.IfFeature = append(container.IfFeature, &ASTValue{Name: child.Argument, Source: child, Parent: container, Extensions: extensionStatements(child)})
		case "leaf":
			container.Leaf = append(container.Leaf, leafFromStatement(child, container))
		case "leaf-list":
			container.LeafList = append(container.LeafList, leafListFromStatement(child, container))
		case "list":
			container.List = append(container.List, listFromStatement(child, container))
		case "must":
			container.Must = append(container.Must, mustFromStatement(child, container))
		case "notification":
			container.Notification = append(container.Notification, notificationFromStatement(child, container))
		case "typedef":
			container.Typedef = append(container.Typedef, typedefFromStatementWithParent(child, container))
		case "uses":
			container.Uses = append(container.Uses, usesFromStatement(child, container))
		}
	}
	return container
}

func deviationFromStatement(stmt *Statement, parent Node) *Deviation {
	if stmt == nil {
		return nil
	}
	deviation := &Deviation{
		Name:       stmt.Argument,
		Source:     stmt,
		Parent:     parent,
		Extensions: extensionStatements(stmt),
	}
	deviation.Description = astValueChild(stmt, "description", deviation)
	deviation.Reference = astValueChild(stmt, "reference", deviation)
	for _, child := range stmt.SubStatements() {
		if child.Keyword == "deviate" {
			deviation.Deviate = append(deviation.Deviate, deviateFromStatement(child, deviation))
		}
	}
	return deviation
}

func mustFromStatement(stmt *Statement, parent Node) *Must {
	if stmt == nil {
		return nil
	}
	must := &Must{
		Name:       stmt.Argument,
		Source:     stmt,
		Parent:     parent,
		Extensions: extensionStatements(stmt),
	}
	must.Description = astValueChild(stmt, "description", must)
	must.ErrorAppTag = astValueChild(stmt, "error-app-tag", must)
	must.ErrorMessage = astValueChild(stmt, "error-message", must)
	must.Reference = astValueChild(stmt, "reference", must)
	return must
}

func deviateFromStatement(stmt *Statement, parent Node) *Deviate {
	if stmt == nil {
		return nil
	}
	deviate := &Deviate{
		Name:       stmt.Argument,
		Source:     stmt,
		Parent:     parent,
		Extensions: extensionStatements(stmt),
	}
	deviate.Config = astValueChild(stmt, "config", deviate)
	deviate.Default = astValueChild(stmt, "default", deviate)
	deviate.Mandatory = astValueChild(stmt, "mandatory", deviate)
	deviate.MaxElements = astValueChild(stmt, "max-elements", deviate)
	deviate.MinElements = astValueChild(stmt, "min-elements", deviate)
	deviate.Type = typeChild(stmt, deviate)
	deviate.Units = astValueChild(stmt, "units", deviate)
	for _, child := range stmt.SubStatements() {
		switch child.Keyword {
		case "must":
			deviate.Must = append(deviate.Must, mustFromStatement(child, deviate))
		case "unique":
			deviate.Unique = append(deviate.Unique, &ASTValue{Name: child.Argument, Source: child, Parent: deviate, Extensions: extensionStatements(child)})
		}
	}
	return deviate
}

func extensionDefFromStatement(stmt *Statement, parent Node) *Extension {
	if stmt == nil {
		return nil
	}
	ext := &Extension{
		Name:       stmt.Argument,
		Source:     stmt,
		Parent:     parent,
		Extensions: extensionStatements(stmt),
	}
	ext.Description = astValueChild(stmt, "description", ext)
	ext.Reference = astValueChild(stmt, "reference", ext)
	ext.Status = astValueChild(stmt, "status", ext)
	if arg := firstChild(stmt, "argument"); arg != nil {
		argument := &Argument{
			Name:       arg.Argument,
			Source:     arg,
			Parent:     ext,
			Extensions: extensionStatements(arg),
		}
		argument.YinElement = astValueChild(arg, "yin-element", argument)
		ext.Argument = argument
	}
	return ext
}

func featureFromStatement(stmt *Statement, parent Node) *Feature {
	if stmt == nil {
		return nil
	}
	feature := &Feature{
		Name:       stmt.Argument,
		Source:     stmt,
		Parent:     parent,
		Extensions: extensionStatements(stmt),
	}
	feature.Description = astValueChild(stmt, "description", feature)
	feature.Reference = astValueChild(stmt, "reference", feature)
	feature.Status = astValueChild(stmt, "status", feature)
	for _, child := range stmt.SubStatements() {
		if child.Keyword == "if-feature" {
			feature.IfFeature = append(feature.IfFeature, &ASTValue{Name: child.Argument, Source: child, Parent: feature, Extensions: extensionStatements(child)})
		}
	}
	return feature
}

func belongsToFromStatement(stmt *Statement) *BelongsTo {
	if stmt == nil {
		return nil
	}
	belongsTo := &BelongsTo{
		Name:       stmt.Argument,
		Source:     stmt,
		Extensions: extensionStatements(stmt),
	}
	belongsTo.Prefix = astValueChild(stmt, "prefix", belongsTo)
	return belongsTo
}

func groupingFromStatement(stmt *Statement) *Grouping {
	if stmt == nil {
		return nil
	}
	grouping := &Grouping{
		Name:       stmt.Argument,
		Source:     stmt,
		Extensions: extensionStatements(stmt),
	}
	grouping.Description = astValueChild(stmt, "description", grouping)
	grouping.Reference = astValueChild(stmt, "reference", grouping)
	grouping.Status = astValueChild(stmt, "status", grouping)
	for _, child := range stmt.SubStatements() {
		switch child.Keyword {
		case "action":
			grouping.Action = append(grouping.Action, actionFromStatement(child, grouping))
		case "anydata":
			grouping.Anydata = append(grouping.Anydata, anyDataFromStatement(child, grouping))
		case "anyxml":
			grouping.Anyxml = append(grouping.Anyxml, anyXMLFromStatement(child, grouping))
		case "choice":
			grouping.Choice = append(grouping.Choice, choiceFromStatement(child, grouping))
		case "container":
			grouping.Container = append(grouping.Container, containerFromStatement(child, grouping))
		case "grouping":
			grouping.Grouping = append(grouping.Grouping, groupingFromStatementWithParent(child, grouping))
		case "leaf":
			grouping.Leaf = append(grouping.Leaf, leafFromStatement(child, grouping))
		case "leaf-list":
			grouping.LeafList = append(grouping.LeafList, leafListFromStatement(child, grouping))
		case "list":
			grouping.List = append(grouping.List, listFromStatement(child, grouping))
		case "notification":
			grouping.Notification = append(grouping.Notification, notificationFromStatement(child, grouping))
		case "typedef":
			grouping.Typedef = append(grouping.Typedef, typedefFromStatementWithParent(child, grouping))
		case "uses":
			grouping.Uses = append(grouping.Uses, usesFromStatement(child, grouping))
		}
	}
	return grouping
}

func groupingFromStatementWithParent(stmt *Statement, parent Node) *Grouping {
	grouping := groupingFromStatement(stmt)
	if grouping != nil {
		grouping.Parent = parent
	}
	return grouping
}

func typedefFromStatement(stmt *Statement) *Typedef {
	if stmt == nil {
		return nil
	}
	typedef := &Typedef{
		Name:       stmt.Argument,
		Source:     stmt,
		Extensions: extensionStatements(stmt),
	}
	typedef.Default = astValueChild(stmt, "default", typedef)
	typedef.Description = astValueChild(stmt, "description", typedef)
	typedef.Reference = astValueChild(stmt, "reference", typedef)
	typedef.Status = astValueChild(stmt, "status", typedef)
	typedef.Type = typeChild(stmt, typedef)
	typedef.Units = astValueChild(stmt, "units", typedef)
	return typedef
}

func typedefFromStatementWithParent(stmt *Statement, parent Node) *Typedef {
	typedef := typedefFromStatement(stmt)
	if typedef != nil {
		typedef.Parent = parent
	}
	return typedef
}

func astIdentityFromStatement(stmt *Statement) *Identity {
	if stmt == nil {
		return nil
	}
	identity := &Identity{
		Name:       stmt.Argument,
		Source:     stmt,
		Extensions: extensionStatements(stmt),
	}
	identity.Description = compatValueChild(stmt, "description", identity)
	identity.Reference = compatValueChild(stmt, "reference", identity)
	identity.Status = compatValueChild(stmt, "status", identity)
	for _, child := range stmt.SubStatements() {
		switch child.Keyword {
		case "base":
			identity.Base = append(identity.Base, &Value{Name: child.Argument, Source: child, Parent: identity, Extensions: extensionStatements(child)})
		case "if-feature":
			identity.IfFeature = append(identity.IfFeature, &Value{Name: child.Argument, Source: child, Parent: identity, Extensions: extensionStatements(child)})
		}
	}
	return identity
}

func compatValueChild(stmt *Statement, keyword string, parent Node) *Value {
	child := firstChild(stmt, keyword)
	if child == nil {
		return nil
	}
	return &Value{Name: child.Argument, Source: child, Parent: parent, Extensions: extensionStatements(child)}
}

func moduleValueChild(stmt *Statement, keyword, fallback string, parent Node) *Value {
	if stmt != nil {
		if child := firstChild(stmt, keyword); child != nil {
			return &Value{
				Name:       child.Argument,
				Source:     child,
				Parent:     parent,
				Extensions: extensionStatements(child),
			}
		}
	}
	return &Value{Name: fallback, Parent: parent}
}

func moduleRevisions(stmt *Statement, revisions []*Revision, parent Node) []*Revision {
	if len(revisions) == 0 {
		return nil
	}
	sourceByName := make(map[string]*Revision)
	if stmt != nil {
		for _, child := range stmt.SubStatements() {
			if child.Keyword != "revision" {
				continue
			}
			if rev := revisionFromStatement(child); rev != nil {
				sourceByName[rev.Name] = rev
			}
		}
	}
	out := make([]*Revision, 0, len(revisions))
	for _, rev := range revisions {
		if rev == nil {
			continue
		}
		sourceRev := rev
		if rev.Source == nil {
			if candidate := sourceByName[rev.Name]; candidate != nil {
				sourceRev = candidate
			}
		}
		if cloned := cloneRevision(sourceRev, parent); cloned != nil {
			out = append(out, cloned)
		}
	}
	return out
}

func astIdentityFromStatementWithParent(stmt *Statement, parent Node) *Identity {
	identity := astIdentityFromStatement(stmt)
	if identity != nil {
		identity.Parent = parent
	}
	return identity
}

func leafFromStatement(stmt *Statement, parent Node) *Leaf {
	if stmt == nil {
		return nil
	}
	leaf := &Leaf{
		Name:       stmt.Argument,
		Source:     stmt,
		Parent:     parent,
		Extensions: extensionStatements(stmt),
	}
	leaf.Config = astValueChild(stmt, "config", leaf)
	leaf.Default = astValueChild(stmt, "default", leaf)
	leaf.Description = astValueChild(stmt, "description", leaf)
	leaf.Mandatory = astValueChild(stmt, "mandatory", leaf)
	leaf.Reference = astValueChild(stmt, "reference", leaf)
	leaf.Status = astValueChild(stmt, "status", leaf)
	leaf.Type = typeChild(stmt, leaf)
	leaf.Units = astValueChild(stmt, "units", leaf)
	leaf.When = astValueChild(stmt, "when", leaf)
	for _, child := range stmt.SubStatements() {
		switch child.Keyword {
		case "if-feature":
			leaf.IfFeature = append(leaf.IfFeature, &ASTValue{Name: child.Argument, Source: child, Parent: leaf, Extensions: extensionStatements(child)})
		case "must":
			leaf.Must = append(leaf.Must, mustFromStatement(child, leaf))
		}
	}
	return leaf
}

func leafListFromStatement(stmt *Statement, parent Node) *LeafList {
	if stmt == nil {
		return nil
	}
	leafList := &LeafList{
		Name:       stmt.Argument,
		Source:     stmt,
		Parent:     parent,
		Extensions: extensionStatements(stmt),
	}
	leafList.Config = astValueChild(stmt, "config", leafList)
	leafList.Description = astValueChild(stmt, "description", leafList)
	leafList.MaxElements = astValueChild(stmt, "max-elements", leafList)
	leafList.MinElements = astValueChild(stmt, "min-elements", leafList)
	leafList.OrderedBy = astValueChild(stmt, "ordered-by", leafList)
	leafList.Reference = astValueChild(stmt, "reference", leafList)
	leafList.Status = astValueChild(stmt, "status", leafList)
	leafList.Type = typeChild(stmt, leafList)
	leafList.Units = astValueChild(stmt, "units", leafList)
	leafList.When = astValueChild(stmt, "when", leafList)
	for _, child := range stmt.SubStatements() {
		switch child.Keyword {
		case "default":
			leafList.Default = append(leafList.Default, &ASTValue{Name: child.Argument, Source: child, Parent: leafList})
		case "if-feature":
			leafList.IfFeature = append(leafList.IfFeature, &ASTValue{Name: child.Argument, Source: child, Parent: leafList, Extensions: extensionStatements(child)})
		case "must":
			leafList.Must = append(leafList.Must, mustFromStatement(child, leafList))
		}
	}
	return leafList
}

func listFromStatement(stmt *Statement, parent Node) *List {
	if stmt == nil {
		return nil
	}
	list := &List{
		Name:       stmt.Argument,
		Source:     stmt,
		Parent:     parent,
		Extensions: extensionStatements(stmt),
	}
	list.Config = astValueChild(stmt, "config", list)
	list.Description = astValueChild(stmt, "description", list)
	list.Key = astValueChild(stmt, "key", list)
	list.MaxElements = astValueChild(stmt, "max-elements", list)
	list.MinElements = astValueChild(stmt, "min-elements", list)
	list.OrderedBy = astValueChild(stmt, "ordered-by", list)
	list.Reference = astValueChild(stmt, "reference", list)
	list.Status = astValueChild(stmt, "status", list)
	list.When = astValueChild(stmt, "when", list)
	for _, child := range stmt.SubStatements() {
		switch child.Keyword {
		case "action":
			list.Action = append(list.Action, actionFromStatement(child, list))
		case "anydata":
			list.Anydata = append(list.Anydata, anyDataFromStatement(child, list))
		case "anyxml":
			list.Anyxml = append(list.Anyxml, anyXMLFromStatement(child, list))
		case "choice":
			list.Choice = append(list.Choice, choiceFromStatement(child, list))
		case "container":
			list.Container = append(list.Container, containerFromStatement(child, list))
		case "leaf":
			list.Leaf = append(list.Leaf, leafFromStatement(child, list))
		case "leaf-list":
			list.LeafList = append(list.LeafList, leafListFromStatement(child, list))
		case "list":
			list.List = append(list.List, listFromStatement(child, list))
		case "grouping":
			list.Grouping = append(list.Grouping, groupingFromStatementWithParent(child, list))
		case "if-feature":
			list.IfFeature = append(list.IfFeature, &ASTValue{Name: child.Argument, Source: child, Parent: list, Extensions: extensionStatements(child)})
		case "must":
			list.Must = append(list.Must, mustFromStatement(child, list))
		case "notification":
			list.Notification = append(list.Notification, notificationFromStatement(child, list))
		case "typedef":
			list.Typedef = append(list.Typedef, typedefFromStatementWithParent(child, list))
		case "unique":
			list.Unique = append(list.Unique, &ASTValue{Name: child.Argument, Source: child, Parent: list, Extensions: extensionStatements(child)})
		case "uses":
			list.Uses = append(list.Uses, usesFromStatement(child, list))
		}
	}
	return list
}

func typeChild(stmt *Statement, parent Node) *Type {
	child := firstChild(stmt, "type")
	if child == nil {
		return nil
	}
	return typeFromStatement(child, parent)
}

func typeFromStatement(stmt *Statement, parent Node) *Type {
	if stmt == nil {
		return nil
	}
	typ := &Type{
		Name:       stmt.Argument,
		Source:     stmt,
		Parent:     parent,
		Extensions: extensionStatements(stmt),
	}
	typ.IdentityBase = astValueChild(stmt, "base", typ)
	typ.FractionDigits = astValueChild(stmt, "fraction-digits", typ)
	typ.Path = astValueChild(stmt, "path", typ)
	typ.RequireInstance = astValueChild(stmt, "require-instance", typ)
	if length := firstChild(stmt, "length"); length != nil {
		typ.Length = lengthFromStatement(length, typ)
	}
	if rng := firstChild(stmt, "range"); rng != nil {
		typ.Range = rangeFromStatement(rng, typ)
	}
	for _, child := range stmt.SubStatements() {
		switch child.Keyword {
		case "bit":
			typ.Bit = append(typ.Bit, bitFromStatement(child, typ))
		case "enum":
			typ.Enum = append(typ.Enum, enumFromStatement(child, typ))
		case "pattern":
			typ.Pattern = append(typ.Pattern, patternFromStatement(child, typ))
		case "type":
			typ.Type = append(typ.Type, typeFromStatement(child, typ))
		}
	}
	return typ
}

func rangeFromStatement(stmt *Statement, parent Node) *Range {
	if stmt == nil {
		return nil
	}
	rng := &Range{Name: stmt.Argument, Source: stmt, Parent: parent, Extensions: extensionStatements(stmt)}
	rng.Description = astValueChild(stmt, "description", rng)
	rng.ErrorAppTag = astValueChild(stmt, "error-app-tag", rng)
	rng.ErrorMessage = astValueChild(stmt, "error-message", rng)
	rng.Reference = astValueChild(stmt, "reference", rng)
	return rng
}

func lengthFromStatement(stmt *Statement, parent Node) *Length {
	if stmt == nil {
		return nil
	}
	length := &Length{Name: stmt.Argument, Source: stmt, Parent: parent, Extensions: extensionStatements(stmt)}
	length.Description = astValueChild(stmt, "description", length)
	length.ErrorAppTag = astValueChild(stmt, "error-app-tag", length)
	length.ErrorMessage = astValueChild(stmt, "error-message", length)
	length.Reference = astValueChild(stmt, "reference", length)
	return length
}

func patternFromStatement(stmt *Statement, parent Node) *Pattern {
	if stmt == nil {
		return nil
	}
	pattern := &Pattern{Name: stmt.Argument, Source: stmt, Parent: parent, Extensions: extensionStatements(stmt)}
	pattern.Description = astValueChild(stmt, "description", pattern)
	pattern.ErrorAppTag = astValueChild(stmt, "error-app-tag", pattern)
	pattern.ErrorMessage = astValueChild(stmt, "error-message", pattern)
	pattern.Modifier = astValueChild(stmt, "modifier", pattern)
	pattern.Reference = astValueChild(stmt, "reference", pattern)
	return pattern
}

func enumFromStatement(stmt *Statement, parent Node) *Enum {
	if stmt == nil {
		return nil
	}
	enum := &Enum{Name: stmt.Argument, Source: stmt, Parent: parent, Extensions: extensionStatements(stmt)}
	enum.Description = astValueChild(stmt, "description", enum)
	enum.Reference = astValueChild(stmt, "reference", enum)
	enum.Status = astValueChild(stmt, "status", enum)
	enum.Value = astValueChild(stmt, "value", enum)
	for _, child := range stmt.SubStatements() {
		if child.Keyword == "if-feature" {
			enum.IfFeature = append(enum.IfFeature, &ASTValue{Name: child.Argument, Source: child, Parent: enum, Extensions: extensionStatements(child)})
		}
	}
	return enum
}

func bitFromStatement(stmt *Statement, parent Node) *Bit {
	if stmt == nil {
		return nil
	}
	bit := &Bit{Name: stmt.Argument, Source: stmt, Parent: parent, Extensions: extensionStatements(stmt)}
	bit.Description = astValueChild(stmt, "description", bit)
	bit.Position = astValueChild(stmt, "position", bit)
	bit.Reference = astValueChild(stmt, "reference", bit)
	bit.Status = astValueChild(stmt, "status", bit)
	for _, child := range stmt.SubStatements() {
		if child.Keyword == "if-feature" {
			bit.IfFeature = append(bit.IfFeature, &ASTValue{Name: child.Argument, Source: child, Parent: bit, Extensions: extensionStatements(child)})
		}
	}
	return bit
}

func notificationFromStatement(stmt *Statement, parent Node) *Notification {
	if stmt == nil {
		return nil
	}
	notification := &Notification{
		Name:       stmt.Argument,
		Source:     stmt,
		Parent:     parent,
		Extensions: extensionStatements(stmt),
	}
	notification.Description = astValueChild(stmt, "description", notification)
	notification.Reference = astValueChild(stmt, "reference", notification)
	notification.Status = astValueChild(stmt, "status", notification)
	for _, child := range stmt.SubStatements() {
		switch child.Keyword {
		case "anydata":
			notification.Anydata = append(notification.Anydata, anyDataFromStatement(child, notification))
		case "anyxml":
			notification.Anyxml = append(notification.Anyxml, anyXMLFromStatement(child, notification))
		case "choice":
			notification.Choice = append(notification.Choice, choiceFromStatement(child, notification))
		case "container":
			notification.Container = append(notification.Container, containerFromStatement(child, notification))
		case "grouping":
			notification.Grouping = append(notification.Grouping, groupingFromStatementWithParent(child, notification))
		case "if-feature":
			notification.IfFeature = append(notification.IfFeature, &ASTValue{Name: child.Argument, Source: child, Parent: notification, Extensions: extensionStatements(child)})
		case "leaf":
			notification.Leaf = append(notification.Leaf, leafFromStatement(child, notification))
		case "leaf-list":
			notification.LeafList = append(notification.LeafList, leafListFromStatement(child, notification))
		case "list":
			notification.List = append(notification.List, listFromStatement(child, notification))
		case "typedef":
			notification.Typedef = append(notification.Typedef, typedefFromStatementWithParent(child, notification))
		case "uses":
			notification.Uses = append(notification.Uses, usesFromStatement(child, notification))
		}
	}
	return notification
}

func rpcFromStatement(stmt *Statement, parent Node) *RPC {
	if stmt == nil {
		return nil
	}
	rpc := &RPC{
		Name:       stmt.Argument,
		Source:     stmt,
		Parent:     parent,
		Extensions: extensionStatements(stmt),
	}
	rpc.Description = astValueChild(stmt, "description", rpc)
	rpc.Reference = astValueChild(stmt, "reference", rpc)
	rpc.Status = astValueChild(stmt, "status", rpc)
	if input := firstChild(stmt, "input"); input != nil {
		rpc.Input = inputFromStatement(input, rpc)
	}
	if output := firstChild(stmt, "output"); output != nil {
		rpc.Output = outputFromStatement(output, rpc)
	}
	for _, child := range stmt.SubStatements() {
		switch child.Keyword {
		case "grouping":
			rpc.Grouping = append(rpc.Grouping, groupingFromStatementWithParent(child, rpc))
		case "if-feature":
			rpc.IfFeature = append(rpc.IfFeature, &ASTValue{Name: child.Argument, Source: child, Parent: rpc, Extensions: extensionStatements(child)})
		case "typedef":
			rpc.Typedef = append(rpc.Typedef, typedefFromStatementWithParent(child, rpc))
		}
	}
	return rpc
}

func actionFromStatement(stmt *Statement, parent Node) *Action {
	if stmt == nil {
		return nil
	}
	action := &Action{
		Name:       stmt.Argument,
		Source:     stmt,
		Parent:     parent,
		Extensions: extensionStatements(stmt),
	}
	action.Description = astValueChild(stmt, "description", action)
	action.Reference = astValueChild(stmt, "reference", action)
	action.Status = astValueChild(stmt, "status", action)
	if input := firstChild(stmt, "input"); input != nil {
		action.Input = inputFromStatement(input, action)
	}
	if output := firstChild(stmt, "output"); output != nil {
		action.Output = outputFromStatement(output, action)
	}
	for _, child := range stmt.SubStatements() {
		switch child.Keyword {
		case "grouping":
			action.Grouping = append(action.Grouping, groupingFromStatementWithParent(child, action))
		case "if-feature":
			action.IfFeature = append(action.IfFeature, &ASTValue{Name: child.Argument, Source: child, Parent: action, Extensions: extensionStatements(child)})
		case "typedef":
			action.Typedef = append(action.Typedef, typedefFromStatementWithParent(child, action))
		}
	}
	return action
}

func inputFromStatement(stmt *Statement, parent Node) *Input {
	if stmt == nil {
		return nil
	}
	input := &Input{Name: stmt.Argument, Source: stmt, Parent: parent, Extensions: extensionStatements(stmt)}
	populateOperationPayload(stmt, input)
	return input
}

func outputFromStatement(stmt *Statement, parent Node) *Output {
	if stmt == nil {
		return nil
	}
	output := &Output{Name: stmt.Argument, Source: stmt, Parent: parent, Extensions: extensionStatements(stmt)}
	populateOperationPayload(stmt, output)
	return output
}

type operationPayload interface {
	Node
	addAnydata(*AnyData)
	addAnyxml(*AnyXML)
	addChoice(*Choice)
	addContainer(*Container)
	addGrouping(*Grouping)
	addLeaf(*Leaf)
	addLeafList(*LeafList)
	addList(*List)
	addTypedef(*Typedef)
	addUses(*Uses)
}

type inputPayload struct{ *Input }
type outputPayload struct{ *Output }

func (p inputPayload) addAnydata(v *AnyData)     { p.Anydata = append(p.Anydata, v) }
func (p inputPayload) addAnyxml(v *AnyXML)       { p.Anyxml = append(p.Anyxml, v) }
func (p inputPayload) addChoice(v *Choice)       { p.Choice = append(p.Choice, v) }
func (p inputPayload) addContainer(v *Container) { p.Container = append(p.Container, v) }
func (p inputPayload) addGrouping(v *Grouping)   { p.Grouping = append(p.Grouping, v) }
func (p inputPayload) addLeaf(v *Leaf)           { p.Leaf = append(p.Leaf, v) }
func (p inputPayload) addLeafList(v *LeafList)   { p.LeafList = append(p.LeafList, v) }
func (p inputPayload) addList(v *List)           { p.List = append(p.List, v) }
func (p inputPayload) addTypedef(v *Typedef)     { p.Typedef = append(p.Typedef, v) }
func (p inputPayload) addUses(v *Uses)           { p.Uses = append(p.Uses, v) }

func (p outputPayload) addAnydata(v *AnyData)     { p.Anydata = append(p.Anydata, v) }
func (p outputPayload) addAnyxml(v *AnyXML)       { p.Anyxml = append(p.Anyxml, v) }
func (p outputPayload) addChoice(v *Choice)       { p.Choice = append(p.Choice, v) }
func (p outputPayload) addContainer(v *Container) { p.Container = append(p.Container, v) }
func (p outputPayload) addGrouping(v *Grouping)   { p.Grouping = append(p.Grouping, v) }
func (p outputPayload) addLeaf(v *Leaf)           { p.Leaf = append(p.Leaf, v) }
func (p outputPayload) addLeafList(v *LeafList)   { p.LeafList = append(p.LeafList, v) }
func (p outputPayload) addList(v *List)           { p.List = append(p.List, v) }
func (p outputPayload) addTypedef(v *Typedef)     { p.Typedef = append(p.Typedef, v) }
func (p outputPayload) addUses(v *Uses)           { p.Uses = append(p.Uses, v) }

func populateOperationPayload(stmt *Statement, payload Node) {
	var out operationPayload
	switch node := payload.(type) {
	case *Input:
		out = inputPayload{node}
	case *Output:
		out = outputPayload{node}
	default:
		return
	}
	for _, child := range stmt.SubStatements() {
		switch child.Keyword {
		case "anydata":
			out.addAnydata(anyDataFromStatement(child, payload))
		case "anyxml":
			out.addAnyxml(anyXMLFromStatement(child, payload))
		case "choice":
			out.addChoice(choiceFromStatement(child, payload))
		case "container":
			out.addContainer(containerFromStatement(child, payload))
		case "grouping":
			out.addGrouping(groupingFromStatementWithParent(child, payload))
		case "leaf":
			out.addLeaf(leafFromStatement(child, payload))
		case "leaf-list":
			out.addLeafList(leafListFromStatement(child, payload))
		case "list":
			out.addList(listFromStatement(child, payload))
		case "typedef":
			out.addTypedef(typedefFromStatementWithParent(child, payload))
		case "uses":
			out.addUses(usesFromStatement(child, payload))
		}
	}
}

func usesFromStatement(stmt *Statement, parent Node) *Uses {
	if stmt == nil {
		return nil
	}
	uses := &Uses{
		Name:       stmt.Argument,
		Source:     stmt,
		Parent:     parent,
		Extensions: extensionStatements(stmt),
	}
	uses.Description = astValueChild(stmt, "description", uses)
	uses.Reference = astValueChild(stmt, "reference", uses)
	uses.Status = astValueChild(stmt, "status", uses)
	uses.When = astValueChild(stmt, "when", uses)
	for _, child := range stmt.SubStatements() {
		switch child.Keyword {
		case "augment":
			uses.Augment = augmentFromStatement(child, uses)
		case "if-feature":
			uses.IfFeature = append(uses.IfFeature, &ASTValue{Name: child.Argument, Source: child, Parent: uses, Extensions: extensionStatements(child)})
		case "refine":
			uses.Refine = append(uses.Refine, refineFromStatement(child, uses))
		}
	}
	return uses
}

func refineFromStatement(stmt *Statement, parent Node) *Refine {
	if stmt == nil {
		return nil
	}
	refine := &Refine{
		Name:       stmt.Argument,
		Source:     stmt,
		Parent:     parent,
		Extensions: extensionStatements(stmt),
	}
	refine.Config = astValueChild(stmt, "config", refine)
	refine.Default = astValueChild(stmt, "default", refine)
	refine.Description = astValueChild(stmt, "description", refine)
	refine.Mandatory = astValueChild(stmt, "mandatory", refine)
	refine.MaxElements = astValueChild(stmt, "max-elements", refine)
	refine.MinElements = astValueChild(stmt, "min-elements", refine)
	refine.Presence = astValueChild(stmt, "presence", refine)
	refine.Reference = astValueChild(stmt, "reference", refine)
	for _, child := range stmt.SubStatements() {
		switch child.Keyword {
		case "if-feature":
			refine.IfFeature = append(refine.IfFeature, &ASTValue{Name: child.Argument, Source: child, Parent: refine, Extensions: extensionStatements(child)})
		case "must":
			refine.Must = append(refine.Must, mustFromStatement(child, refine))
		}
	}
	return refine
}

func extensionStatements(stmt *Statement) []*Statement {
	if stmt == nil {
		return nil
	}
	var out []*Statement
	for _, child := range stmt.SubStatements() {
		if strings.Contains(child.Keyword, ":") {
			out = append(out, child)
		}
	}
	return out
}

func firstChild(stmt *Statement, keyword string) *Statement {
	for _, child := range stmt.SubStatements() {
		if child.Keyword == keyword {
			return child
		}
	}
	return nil
}

func astValueOrNil(value string) *ASTValue {
	if value == "" {
		return nil
	}
	return &ASTValue{Name: value}
}

func astValueChild(stmt *Statement, keyword string, parent Node) *ASTValue {
	child := firstChild(stmt, keyword)
	if child == nil {
		return nil
	}
	return &ASTValue{
		Name:       child.Argument,
		Source:     child,
		Parent:     parent,
		Extensions: extensionStatements(child),
	}
}

func revisionsFromCambium(revisions []cambium.Revision) []*Revision {
	if len(revisions) == 0 {
		return nil
	}
	out := make([]*Revision, 0, len(revisions))
	for _, rev := range revisions {
		out = append(out, &Revision{
			Name:        rev.Date(),
			Description: astValueFromOptional(rev.Description()),
			Reference:   astValueFromOptional(rev.Reference()),
		})
	}
	return out
}

func astValueFromOptional(value string, ok bool) *ASTValue {
	if !ok {
		return nil
	}
	return astValueOrNil(value)
}

func cloneRevisions(in []*Revision, parent Node) []*Revision {
	if len(in) == 0 {
		return nil
	}
	out := make([]*Revision, 0, len(in))
	for _, rev := range in {
		if cloned := cloneRevision(rev, parent); cloned != nil {
			out = append(out, cloned)
		}
	}
	return out
}

func cloneRevision(rev *Revision, parent Node) *Revision {
	if rev == nil {
		return nil
	}
	cloned := &Revision{
		Name:       rev.Name,
		Source:     rev.Source,
		Parent:     parent,
		Extensions: append([]*Statement(nil), rev.Extensions...),
	}
	cloned.Description = cloneASTValueWithParent(rev.Description, cloned)
	cloned.Reference = cloneASTValueWithParent(rev.Reference, cloned)
	return cloned
}

func cloneASTValueWithParent(value *ASTValue, parent Node) *ASTValue {
	if value == nil {
		return nil
	}
	cloned := &ASTValue{
		Name:       value.Name,
		Source:     value.Source,
		Parent:     parent,
		Extensions: append([]*Statement(nil), value.Extensions...),
	}
	cloned.Description = cloneASTValueWithParent(value.Description, cloned)
	return cloned
}

func setASTValueParent(value *ASTValue, parent Node) {
	if value != nil {
		value.Parent = parent
	}
}

func cloneBelongsTo(in *BelongsTo) *BelongsTo {
	if in == nil {
		return nil
	}
	out := &BelongsTo{
		Name:       in.Name,
		Source:     in.Source,
		Parent:     in.Parent,
		Extensions: append([]*Statement(nil), in.Extensions...),
	}
	out.Prefix = cloneASTValueWithParent(in.Prefix, out)
	return out
}

func cloneGroupings(in []*Grouping, parent Node) []*Grouping {
	if len(in) == 0 {
		return nil
	}
	out := make([]*Grouping, 0, len(in))
	for _, grouping := range in {
		if grouping == nil {
			continue
		}
		cloned := &Grouping{
			Name:       grouping.Name,
			Source:     grouping.Source,
			Parent:     parent,
			Extensions: append([]*Statement(nil), grouping.Extensions...),
		}
		cloned.Action = cloneActionsWithParent(grouping.Action, cloned)
		cloned.Anydata = cloneAnyDataWithParent(grouping.Anydata, cloned)
		cloned.Anyxml = cloneAnyXMLWithParent(grouping.Anyxml, cloned)
		cloned.Choice = cloneChoicesWithParent(grouping.Choice, cloned)
		cloned.Container = cloneContainersWithParent(grouping.Container, cloned)
		cloned.Description = cloneASTValueWithParent(grouping.Description, cloned)
		cloned.Grouping = cloneGroupings(grouping.Grouping, cloned)
		cloned.Leaf = cloneLeavesWithParent(grouping.Leaf, cloned)
		cloned.LeafList = cloneLeafListsWithParent(grouping.LeafList, cloned)
		cloned.List = cloneListsWithParent(grouping.List, cloned)
		cloned.Notification = cloneNotificationsWithParent(grouping.Notification, cloned)
		cloned.Reference = cloneASTValueWithParent(grouping.Reference, cloned)
		cloned.Status = cloneASTValueWithParent(grouping.Status, cloned)
		cloned.Typedef = cloneTypedefs(grouping.Typedef, cloned)
		cloned.Uses = cloneUsesWithParent(grouping.Uses, cloned)
		out = append(out, cloned)
	}
	return out
}

func cloneAnyDataWithParent(in []*AnyData, parent Node) []*AnyData {
	if len(in) == 0 {
		return nil
	}
	out := make([]*AnyData, 0, len(in))
	for _, anydata := range in {
		if anydata == nil {
			continue
		}
		cloned := &AnyData{
			Name:       anydata.Name,
			Source:     anydata.Source,
			Parent:     parent,
			Extensions: append([]*Statement(nil), anydata.Extensions...),
		}
		cloned.Config = cloneASTValueWithParent(anydata.Config, cloned)
		cloned.Description = cloneASTValueWithParent(anydata.Description, cloned)
		cloned.IfFeature = cloneASTValuesWithParent(anydata.IfFeature, cloned)
		cloned.Mandatory = cloneASTValueWithParent(anydata.Mandatory, cloned)
		cloned.Must = cloneMustsWithParent(anydata.Must, cloned)
		cloned.Reference = cloneASTValueWithParent(anydata.Reference, cloned)
		cloned.Status = cloneASTValueWithParent(anydata.Status, cloned)
		cloned.When = cloneASTValueWithParent(anydata.When, cloned)
		out = append(out, cloned)
	}
	return out
}

func cloneAnyXMLWithParent(in []*AnyXML, parent Node) []*AnyXML {
	if len(in) == 0 {
		return nil
	}
	out := make([]*AnyXML, 0, len(in))
	for _, anyxml := range in {
		if anyxml == nil {
			continue
		}
		cloned := &AnyXML{
			Name:       anyxml.Name,
			Source:     anyxml.Source,
			Parent:     parent,
			Extensions: append([]*Statement(nil), anyxml.Extensions...),
		}
		cloned.Config = cloneASTValueWithParent(anyxml.Config, cloned)
		cloned.Description = cloneASTValueWithParent(anyxml.Description, cloned)
		cloned.IfFeature = cloneASTValuesWithParent(anyxml.IfFeature, cloned)
		cloned.Mandatory = cloneASTValueWithParent(anyxml.Mandatory, cloned)
		cloned.Must = cloneMustsWithParent(anyxml.Must, cloned)
		cloned.Reference = cloneASTValueWithParent(anyxml.Reference, cloned)
		cloned.Status = cloneASTValueWithParent(anyxml.Status, cloned)
		cloned.When = cloneASTValueWithParent(anyxml.When, cloned)
		out = append(out, cloned)
	}
	return out
}

func cloneContainersWithParent(in []*Container, parent Node) []*Container {
	if len(in) == 0 {
		return nil
	}
	out := make([]*Container, 0, len(in))
	for _, container := range in {
		if container == nil {
			continue
		}
		cloned := &Container{
			Name:       container.Name,
			Source:     container.Source,
			Parent:     parent,
			Extensions: append([]*Statement(nil), container.Extensions...),
		}
		cloned.Action = cloneActionsWithParent(container.Action, cloned)
		cloned.Anydata = cloneAnyDataWithParent(container.Anydata, cloned)
		cloned.Anyxml = cloneAnyXMLWithParent(container.Anyxml, cloned)
		cloned.Choice = cloneChoicesWithParent(container.Choice, cloned)
		cloned.Config = cloneASTValueWithParent(container.Config, cloned)
		cloned.Container = cloneContainersWithParent(container.Container, cloned)
		cloned.Description = cloneASTValueWithParent(container.Description, cloned)
		cloned.Grouping = cloneGroupings(container.Grouping, cloned)
		cloned.IfFeature = cloneASTValuesWithParent(container.IfFeature, cloned)
		cloned.Leaf = cloneLeavesWithParent(container.Leaf, cloned)
		cloned.LeafList = cloneLeafListsWithParent(container.LeafList, cloned)
		cloned.List = cloneListsWithParent(container.List, cloned)
		cloned.Must = cloneMustsWithParent(container.Must, cloned)
		cloned.Notification = cloneNotificationsWithParent(container.Notification, cloned)
		cloned.Presence = cloneASTValueWithParent(container.Presence, cloned)
		cloned.Reference = cloneASTValueWithParent(container.Reference, cloned)
		cloned.Status = cloneASTValueWithParent(container.Status, cloned)
		cloned.Typedef = cloneTypedefs(container.Typedef, cloned)
		cloned.Uses = cloneUsesWithParent(container.Uses, cloned)
		cloned.When = cloneASTValueWithParent(container.When, cloned)
		out = append(out, cloned)
	}
	return out
}

func cloneChoicesWithParent(in []*Choice, parent Node) []*Choice {
	if len(in) == 0 {
		return nil
	}
	out := make([]*Choice, 0, len(in))
	for _, choice := range in {
		if choice == nil {
			continue
		}
		cloned := &Choice{
			Name:       choice.Name,
			Source:     choice.Source,
			Parent:     parent,
			Extensions: append([]*Statement(nil), choice.Extensions...),
		}
		cloned.Anydata = cloneAnyDataWithParent(choice.Anydata, cloned)
		cloned.Anyxml = cloneAnyXMLWithParent(choice.Anyxml, cloned)
		cloned.Case = cloneCasesWithParent(choice.Case, cloned)
		cloned.Config = cloneASTValueWithParent(choice.Config, cloned)
		cloned.Container = cloneContainersWithParent(choice.Container, cloned)
		cloned.Default = cloneASTValueWithParent(choice.Default, cloned)
		cloned.Description = cloneASTValueWithParent(choice.Description, cloned)
		cloned.IfFeature = cloneASTValuesWithParent(choice.IfFeature, cloned)
		cloned.Leaf = cloneLeavesWithParent(choice.Leaf, cloned)
		cloned.LeafList = cloneLeafListsWithParent(choice.LeafList, cloned)
		cloned.List = cloneListsWithParent(choice.List, cloned)
		cloned.Mandatory = cloneASTValueWithParent(choice.Mandatory, cloned)
		cloned.Reference = cloneASTValueWithParent(choice.Reference, cloned)
		cloned.Status = cloneASTValueWithParent(choice.Status, cloned)
		cloned.When = cloneASTValueWithParent(choice.When, cloned)
		out = append(out, cloned)
	}
	return out
}

func cloneCasesWithParent(in []*Case, parent Node) []*Case {
	if len(in) == 0 {
		return nil
	}
	out := make([]*Case, 0, len(in))
	for _, cas := range in {
		if cas == nil {
			continue
		}
		cloned := &Case{
			Name:       cas.Name,
			Source:     cas.Source,
			Parent:     parent,
			Extensions: append([]*Statement(nil), cas.Extensions...),
		}
		cloned.Anydata = cloneAnyDataWithParent(cas.Anydata, cloned)
		cloned.Anyxml = cloneAnyXMLWithParent(cas.Anyxml, cloned)
		cloned.Choice = cloneChoicesWithParent(cas.Choice, cloned)
		cloned.Container = cloneContainersWithParent(cas.Container, cloned)
		cloned.Description = cloneASTValueWithParent(cas.Description, cloned)
		cloned.IfFeature = cloneASTValuesWithParent(cas.IfFeature, cloned)
		cloned.Leaf = cloneLeavesWithParent(cas.Leaf, cloned)
		cloned.LeafList = cloneLeafListsWithParent(cas.LeafList, cloned)
		cloned.List = cloneListsWithParent(cas.List, cloned)
		cloned.Reference = cloneASTValueWithParent(cas.Reference, cloned)
		cloned.Status = cloneASTValueWithParent(cas.Status, cloned)
		cloned.Uses = cloneUsesWithParent(cas.Uses, cloned)
		cloned.When = cloneASTValueWithParent(cas.When, cloned)
		out = append(out, cloned)
	}
	return out
}

func cloneUsesWithParent(in []*Uses, parent Node) []*Uses {
	if len(in) == 0 {
		return nil
	}
	out := make([]*Uses, 0, len(in))
	for _, uses := range in {
		if uses == nil {
			continue
		}
		cloned := &Uses{
			Name:       uses.Name,
			Source:     uses.Source,
			Parent:     parent,
			Extensions: append([]*Statement(nil), uses.Extensions...),
		}
		cloned.Augment = cloneAugmentWithParent(uses.Augment, cloned)
		cloned.Description = cloneASTValueWithParent(uses.Description, cloned)
		cloned.IfFeature = cloneASTValuesWithParent(uses.IfFeature, cloned)
		cloned.Refine = cloneRefinesWithParent(uses.Refine, cloned)
		cloned.Reference = cloneASTValueWithParent(uses.Reference, cloned)
		cloned.Status = cloneASTValueWithParent(uses.Status, cloned)
		cloned.When = cloneASTValueWithParent(uses.When, cloned)
		out = append(out, cloned)
	}
	return out
}

func cloneRefinesWithParent(in []*Refine, parent Node) []*Refine {
	if len(in) == 0 {
		return nil
	}
	out := make([]*Refine, 0, len(in))
	for _, refine := range in {
		if refine == nil {
			continue
		}
		cloned := &Refine{
			Name:       refine.Name,
			Source:     refine.Source,
			Parent:     parent,
			Extensions: append([]*Statement(nil), refine.Extensions...),
		}
		cloned.Config = cloneASTValueWithParent(refine.Config, cloned)
		cloned.Default = cloneASTValueWithParent(refine.Default, cloned)
		cloned.Description = cloneASTValueWithParent(refine.Description, cloned)
		cloned.IfFeature = cloneASTValuesWithParent(refine.IfFeature, cloned)
		cloned.Mandatory = cloneASTValueWithParent(refine.Mandatory, cloned)
		cloned.MaxElements = cloneASTValueWithParent(refine.MaxElements, cloned)
		cloned.MinElements = cloneASTValueWithParent(refine.MinElements, cloned)
		cloned.Must = cloneMustsWithParent(refine.Must, cloned)
		cloned.Presence = cloneASTValueWithParent(refine.Presence, cloned)
		cloned.Reference = cloneASTValueWithParent(refine.Reference, cloned)
		out = append(out, cloned)
	}
	return out
}

func cloneAugmentsWithParent(in []*Augment, parent Node) []*Augment {
	if len(in) == 0 {
		return nil
	}
	out := make([]*Augment, 0, len(in))
	for _, augment := range in {
		if cloned := cloneAugmentWithParent(augment, parent); cloned != nil {
			out = append(out, cloned)
		}
	}
	return out
}

func cloneAugmentWithParent(in *Augment, parent Node) *Augment {
	if in == nil {
		return nil
	}
	cloned := &Augment{
		Name:       in.Name,
		Source:     in.Source,
		Parent:     parent,
		Extensions: append([]*Statement(nil), in.Extensions...),
	}
	cloned.Action = cloneActionsWithParent(in.Action, cloned)
	cloned.Anydata = cloneAnyDataWithParent(in.Anydata, cloned)
	cloned.Anyxml = cloneAnyXMLWithParent(in.Anyxml, cloned)
	cloned.Case = cloneCasesWithParent(in.Case, cloned)
	cloned.Choice = cloneChoicesWithParent(in.Choice, cloned)
	cloned.Container = cloneContainersWithParent(in.Container, cloned)
	cloned.Description = cloneASTValueWithParent(in.Description, cloned)
	cloned.IfFeature = cloneASTValuesWithParent(in.IfFeature, cloned)
	cloned.Leaf = cloneLeavesWithParent(in.Leaf, cloned)
	cloned.LeafList = cloneLeafListsWithParent(in.LeafList, cloned)
	cloned.List = cloneListsWithParent(in.List, cloned)
	cloned.Notification = cloneNotificationsWithParent(in.Notification, cloned)
	cloned.Reference = cloneASTValueWithParent(in.Reference, cloned)
	cloned.Status = cloneASTValueWithParent(in.Status, cloned)
	cloned.Uses = cloneUsesWithParent(in.Uses, cloned)
	cloned.When = cloneASTValueWithParent(in.When, cloned)
	return cloned
}

func cloneFeaturesWithParent(in []*Feature, parent Node) []*Feature {
	if len(in) == 0 {
		return nil
	}
	out := make([]*Feature, 0, len(in))
	for _, feature := range in {
		if feature == nil {
			continue
		}
		cloned := &Feature{
			Name:       feature.Name,
			Source:     feature.Source,
			Parent:     parent,
			Extensions: append([]*Statement(nil), feature.Extensions...),
		}
		cloned.Description = cloneASTValueWithParent(feature.Description, cloned)
		cloned.IfFeature = cloneASTValuesWithParent(feature.IfFeature, cloned)
		cloned.Reference = cloneASTValueWithParent(feature.Reference, cloned)
		cloned.Status = cloneASTValueWithParent(feature.Status, cloned)
		out = append(out, cloned)
	}
	return out
}

func cloneExtensionsWithParent(in []*Extension, parent Node) []*Extension {
	if len(in) == 0 {
		return nil
	}
	out := make([]*Extension, 0, len(in))
	for _, extension := range in {
		if extension == nil {
			continue
		}
		cloned := &Extension{
			Name:       extension.Name,
			Source:     extension.Source,
			Parent:     parent,
			Extensions: append([]*Statement(nil), extension.Extensions...),
		}
		cloned.Argument = cloneArgumentWithParent(extension.Argument, cloned)
		cloned.Description = cloneASTValueWithParent(extension.Description, cloned)
		cloned.Reference = cloneASTValueWithParent(extension.Reference, cloned)
		cloned.Status = cloneASTValueWithParent(extension.Status, cloned)
		out = append(out, cloned)
	}
	return out
}

func cloneArgumentWithParent(in *Argument, parent Node) *Argument {
	if in == nil {
		return nil
	}
	cloned := &Argument{
		Name:       in.Name,
		Source:     in.Source,
		Parent:     parent,
		Extensions: append([]*Statement(nil), in.Extensions...),
	}
	cloned.YinElement = cloneASTValueWithParent(in.YinElement, cloned)
	return cloned
}

func cloneDeviationsWithParent(in []*Deviation, parent Node) []*Deviation {
	if len(in) == 0 {
		return nil
	}
	out := make([]*Deviation, 0, len(in))
	for _, deviation := range in {
		if deviation == nil {
			continue
		}
		cloned := &Deviation{
			Name:       deviation.Name,
			Source:     deviation.Source,
			Parent:     parent,
			Extensions: append([]*Statement(nil), deviation.Extensions...),
		}
		cloned.Description = cloneASTValueWithParent(deviation.Description, cloned)
		cloned.Deviate = cloneDeviatesWithParent(deviation.Deviate, cloned)
		cloned.Reference = cloneASTValueWithParent(deviation.Reference, cloned)
		out = append(out, cloned)
	}
	return out
}

func cloneDeviatesWithParent(in []*Deviate, parent Node) []*Deviate {
	if len(in) == 0 {
		return nil
	}
	out := make([]*Deviate, 0, len(in))
	for _, deviate := range in {
		if deviate == nil {
			continue
		}
		cloned := &Deviate{
			Name:       deviate.Name,
			Source:     deviate.Source,
			Parent:     parent,
			Extensions: append([]*Statement(nil), deviate.Extensions...),
		}
		cloned.Config = cloneASTValueWithParent(deviate.Config, cloned)
		cloned.Default = cloneASTValueWithParent(deviate.Default, cloned)
		cloned.Mandatory = cloneASTValueWithParent(deviate.Mandatory, cloned)
		cloned.MaxElements = cloneASTValueWithParent(deviate.MaxElements, cloned)
		cloned.MinElements = cloneASTValueWithParent(deviate.MinElements, cloned)
		cloned.Must = cloneMustsWithParent(deviate.Must, cloned)
		cloned.Type = cloneTypeWithParent(deviate.Type, cloned)
		cloned.Unique = cloneASTValuesWithParent(deviate.Unique, cloned)
		cloned.Units = cloneASTValueWithParent(deviate.Units, cloned)
		out = append(out, cloned)
	}
	return out
}

func cloneRPCsWithParent(in []*RPC, parent Node) []*RPC {
	if len(in) == 0 {
		return nil
	}
	out := make([]*RPC, 0, len(in))
	for _, rpc := range in {
		if rpc == nil {
			continue
		}
		cloned := &RPC{
			Name:       rpc.Name,
			Source:     rpc.Source,
			Parent:     parent,
			Extensions: append([]*Statement(nil), rpc.Extensions...),
		}
		cloned.Description = cloneASTValueWithParent(rpc.Description, cloned)
		cloned.Grouping = cloneGroupings(rpc.Grouping, cloned)
		cloned.IfFeature = cloneASTValuesWithParent(rpc.IfFeature, cloned)
		cloned.Input = cloneInputWithParent(rpc.Input, cloned)
		cloned.Output = cloneOutputWithParent(rpc.Output, cloned)
		cloned.Reference = cloneASTValueWithParent(rpc.Reference, cloned)
		cloned.Status = cloneASTValueWithParent(rpc.Status, cloned)
		cloned.Typedef = cloneTypedefs(rpc.Typedef, cloned)
		out = append(out, cloned)
	}
	return out
}

func cloneActionsWithParent(in []*Action, parent Node) []*Action {
	if len(in) == 0 {
		return nil
	}
	out := make([]*Action, 0, len(in))
	for _, action := range in {
		if action == nil {
			continue
		}
		cloned := &Action{
			Name:       action.Name,
			Source:     action.Source,
			Parent:     parent,
			Extensions: append([]*Statement(nil), action.Extensions...),
		}
		cloned.Description = cloneASTValueWithParent(action.Description, cloned)
		cloned.Grouping = cloneGroupings(action.Grouping, cloned)
		cloned.IfFeature = cloneASTValuesWithParent(action.IfFeature, cloned)
		cloned.Input = cloneInputWithParent(action.Input, cloned)
		cloned.Output = cloneOutputWithParent(action.Output, cloned)
		cloned.Reference = cloneASTValueWithParent(action.Reference, cloned)
		cloned.Status = cloneASTValueWithParent(action.Status, cloned)
		cloned.Typedef = cloneTypedefs(action.Typedef, cloned)
		out = append(out, cloned)
	}
	return out
}

func cloneInputWithParent(in *Input, parent Node) *Input {
	if in == nil {
		return nil
	}
	cloned := &Input{
		Name:       in.Name,
		Source:     in.Source,
		Parent:     parent,
		Extensions: append([]*Statement(nil), in.Extensions...),
	}
	cloneOperationPayloadWithParent(cloned, in.Anydata, in.Anyxml, in.Choice, in.Container, in.Grouping, in.Leaf, in.LeafList, in.List, in.Typedef, in.Uses)
	return cloned
}

func cloneOutputWithParent(in *Output, parent Node) *Output {
	if in == nil {
		return nil
	}
	cloned := &Output{
		Name:       in.Name,
		Source:     in.Source,
		Parent:     parent,
		Extensions: append([]*Statement(nil), in.Extensions...),
	}
	cloneOperationPayloadWithParent(cloned, in.Anydata, in.Anyxml, in.Choice, in.Container, in.Grouping, in.Leaf, in.LeafList, in.List, in.Typedef, in.Uses)
	return cloned
}

func cloneOperationPayloadWithParent(
	parent Node,
	anydata []*AnyData,
	anyxml []*AnyXML,
	choices []*Choice,
	containers []*Container,
	groupings []*Grouping,
	leaves []*Leaf,
	leafLists []*LeafList,
	lists []*List,
	typedefs []*Typedef,
	uses []*Uses,
) {
	switch out := parent.(type) {
	case *Input:
		out.Anydata = cloneAnyDataWithParent(anydata, out)
		out.Anyxml = cloneAnyXMLWithParent(anyxml, out)
		out.Choice = cloneChoicesWithParent(choices, out)
		out.Container = cloneContainersWithParent(containers, out)
		out.Grouping = cloneGroupings(groupings, out)
		out.Leaf = cloneLeavesWithParent(leaves, out)
		out.LeafList = cloneLeafListsWithParent(leafLists, out)
		out.List = cloneListsWithParent(lists, out)
		out.Typedef = cloneTypedefs(typedefs, out)
		out.Uses = cloneUsesWithParent(uses, out)
	case *Output:
		out.Anydata = cloneAnyDataWithParent(anydata, out)
		out.Anyxml = cloneAnyXMLWithParent(anyxml, out)
		out.Choice = cloneChoicesWithParent(choices, out)
		out.Container = cloneContainersWithParent(containers, out)
		out.Grouping = cloneGroupings(groupings, out)
		out.Leaf = cloneLeavesWithParent(leaves, out)
		out.LeafList = cloneLeafListsWithParent(leafLists, out)
		out.List = cloneListsWithParent(lists, out)
		out.Typedef = cloneTypedefs(typedefs, out)
		out.Uses = cloneUsesWithParent(uses, out)
	}
}

func cloneNotificationsWithParent(in []*Notification, parent Node) []*Notification {
	if len(in) == 0 {
		return nil
	}
	out := make([]*Notification, 0, len(in))
	for _, notification := range in {
		if notification == nil {
			continue
		}
		cloned := &Notification{
			Name:       notification.Name,
			Source:     notification.Source,
			Parent:     parent,
			Extensions: append([]*Statement(nil), notification.Extensions...),
		}
		cloned.Anydata = cloneAnyDataWithParent(notification.Anydata, cloned)
		cloned.Anyxml = cloneAnyXMLWithParent(notification.Anyxml, cloned)
		cloned.Choice = cloneChoicesWithParent(notification.Choice, cloned)
		cloned.Container = cloneContainersWithParent(notification.Container, cloned)
		cloned.Description = cloneASTValueWithParent(notification.Description, cloned)
		cloned.Grouping = cloneGroupings(notification.Grouping, cloned)
		cloned.IfFeature = cloneASTValuesWithParent(notification.IfFeature, cloned)
		cloned.Leaf = cloneLeavesWithParent(notification.Leaf, cloned)
		cloned.LeafList = cloneLeafListsWithParent(notification.LeafList, cloned)
		cloned.List = cloneListsWithParent(notification.List, cloned)
		cloned.Reference = cloneASTValueWithParent(notification.Reference, cloned)
		cloned.Status = cloneASTValueWithParent(notification.Status, cloned)
		cloned.Typedef = cloneTypedefs(notification.Typedef, cloned)
		cloned.Uses = cloneUsesWithParent(notification.Uses, cloned)
		out = append(out, cloned)
	}
	return out
}

func cloneLeavesWithParent(in []*Leaf, parent Node) []*Leaf {
	if len(in) == 0 {
		return nil
	}
	out := make([]*Leaf, 0, len(in))
	for _, leaf := range in {
		if leaf == nil {
			continue
		}
		cloned := &Leaf{
			Name:       leaf.Name,
			Source:     leaf.Source,
			Parent:     parent,
			Extensions: append([]*Statement(nil), leaf.Extensions...),
		}
		cloned.Config = cloneASTValueWithParent(leaf.Config, cloned)
		cloned.Default = cloneASTValueWithParent(leaf.Default, cloned)
		cloned.Description = cloneASTValueWithParent(leaf.Description, cloned)
		cloned.IfFeature = cloneASTValuesWithParent(leaf.IfFeature, cloned)
		cloned.Mandatory = cloneASTValueWithParent(leaf.Mandatory, cloned)
		cloned.Must = cloneMustsWithParent(leaf.Must, cloned)
		cloned.Reference = cloneASTValueWithParent(leaf.Reference, cloned)
		cloned.Status = cloneASTValueWithParent(leaf.Status, cloned)
		cloned.Type = cloneTypeWithParent(leaf.Type, cloned)
		cloned.Units = cloneASTValueWithParent(leaf.Units, cloned)
		cloned.When = cloneASTValueWithParent(leaf.When, cloned)
		out = append(out, cloned)
	}
	return out
}

func cloneLeafListsWithParent(in []*LeafList, parent Node) []*LeafList {
	if len(in) == 0 {
		return nil
	}
	out := make([]*LeafList, 0, len(in))
	for _, leafList := range in {
		if leafList == nil {
			continue
		}
		cloned := &LeafList{
			Name:       leafList.Name,
			Source:     leafList.Source,
			Parent:     parent,
			Extensions: append([]*Statement(nil), leafList.Extensions...),
		}
		cloned.Config = cloneASTValueWithParent(leafList.Config, cloned)
		cloned.Default = cloneASTValuesWithParent(leafList.Default, cloned)
		cloned.Description = cloneASTValueWithParent(leafList.Description, cloned)
		cloned.IfFeature = cloneASTValuesWithParent(leafList.IfFeature, cloned)
		cloned.MaxElements = cloneASTValueWithParent(leafList.MaxElements, cloned)
		cloned.MinElements = cloneASTValueWithParent(leafList.MinElements, cloned)
		cloned.Must = cloneMustsWithParent(leafList.Must, cloned)
		cloned.OrderedBy = cloneASTValueWithParent(leafList.OrderedBy, cloned)
		cloned.Reference = cloneASTValueWithParent(leafList.Reference, cloned)
		cloned.Status = cloneASTValueWithParent(leafList.Status, cloned)
		cloned.Type = cloneTypeWithParent(leafList.Type, cloned)
		cloned.Units = cloneASTValueWithParent(leafList.Units, cloned)
		cloned.When = cloneASTValueWithParent(leafList.When, cloned)
		out = append(out, cloned)
	}
	return out
}

func cloneListsWithParent(in []*List, parent Node) []*List {
	if len(in) == 0 {
		return nil
	}
	out := make([]*List, 0, len(in))
	for _, list := range in {
		if list == nil {
			continue
		}
		cloned := &List{
			Name:       list.Name,
			Source:     list.Source,
			Parent:     parent,
			Extensions: append([]*Statement(nil), list.Extensions...),
		}
		cloned.Action = cloneActionsWithParent(list.Action, cloned)
		cloned.Anydata = cloneAnyDataWithParent(list.Anydata, cloned)
		cloned.Anyxml = cloneAnyXMLWithParent(list.Anyxml, cloned)
		cloned.Choice = cloneChoicesWithParent(list.Choice, cloned)
		cloned.Config = cloneASTValueWithParent(list.Config, cloned)
		cloned.Container = cloneContainersWithParent(list.Container, cloned)
		cloned.Description = cloneASTValueWithParent(list.Description, cloned)
		cloned.Grouping = cloneGroupings(list.Grouping, cloned)
		cloned.IfFeature = cloneASTValuesWithParent(list.IfFeature, cloned)
		cloned.Key = cloneASTValueWithParent(list.Key, cloned)
		cloned.Leaf = cloneLeavesWithParent(list.Leaf, cloned)
		cloned.LeafList = cloneLeafListsWithParent(list.LeafList, cloned)
		cloned.List = cloneListsWithParent(list.List, cloned)
		cloned.MaxElements = cloneASTValueWithParent(list.MaxElements, cloned)
		cloned.MinElements = cloneASTValueWithParent(list.MinElements, cloned)
		cloned.Must = cloneMustsWithParent(list.Must, cloned)
		cloned.Notification = cloneNotificationsWithParent(list.Notification, cloned)
		cloned.OrderedBy = cloneASTValueWithParent(list.OrderedBy, cloned)
		cloned.Reference = cloneASTValueWithParent(list.Reference, cloned)
		cloned.Status = cloneASTValueWithParent(list.Status, cloned)
		cloned.Typedef = cloneTypedefs(list.Typedef, cloned)
		cloned.Unique = cloneASTValuesWithParent(list.Unique, cloned)
		cloned.Uses = cloneUsesWithParent(list.Uses, cloned)
		cloned.When = cloneASTValueWithParent(list.When, cloned)
		out = append(out, cloned)
	}
	return out
}

func cloneMustsWithParent(in []*Must, parent Node) []*Must {
	if len(in) == 0 {
		return nil
	}
	out := make([]*Must, 0, len(in))
	for _, must := range in {
		if must == nil {
			continue
		}
		cloned := &Must{
			Name:       must.Name,
			Source:     must.Source,
			Parent:     parent,
			Extensions: append([]*Statement(nil), must.Extensions...),
		}
		cloned.Description = cloneASTValueWithParent(must.Description, cloned)
		cloned.ErrorAppTag = cloneASTValueWithParent(must.ErrorAppTag, cloned)
		cloned.ErrorMessage = cloneASTValueWithParent(must.ErrorMessage, cloned)
		cloned.Reference = cloneASTValueWithParent(must.Reference, cloned)
		out = append(out, cloned)
	}
	return out
}

func cloneTypedefs(in []*Typedef, parent Node) []*Typedef {
	if len(in) == 0 {
		return nil
	}
	out := make([]*Typedef, 0, len(in))
	for _, typedef := range in {
		if typedef == nil {
			continue
		}
		cloned := &Typedef{
			Name:       typedef.Name,
			Source:     typedef.Source,
			Parent:     parent,
			Extensions: append([]*Statement(nil), typedef.Extensions...),
			YangType:   typedef.YangType,
		}
		cloned.Default = cloneASTValueWithParent(typedef.Default, cloned)
		cloned.Description = cloneASTValueWithParent(typedef.Description, cloned)
		cloned.Reference = cloneASTValueWithParent(typedef.Reference, cloned)
		cloned.Status = cloneASTValueWithParent(typedef.Status, cloned)
		cloned.Type = cloneTypeWithParent(typedef.Type, cloned)
		cloned.Units = cloneASTValueWithParent(typedef.Units, cloned)
		out = append(out, cloned)
	}
	return out
}

func cloneTypeWithParent(in *Type, parent Node) *Type {
	if in == nil {
		return nil
	}
	cloned := &Type{
		Name:       in.Name,
		Source:     in.Source,
		Parent:     parent,
		Extensions: append([]*Statement(nil), in.Extensions...),
		YangType:   in.YangType,
	}
	cloned.IdentityBase = cloneASTValueWithParent(in.IdentityBase, cloned)
	cloned.Bit = cloneBitsWithParent(in.Bit, cloned)
	cloned.Enum = cloneEnumsWithParent(in.Enum, cloned)
	cloned.FractionDigits = cloneASTValueWithParent(in.FractionDigits, cloned)
	cloned.Length = cloneLengthWithParent(in.Length, cloned)
	cloned.Path = cloneASTValueWithParent(in.Path, cloned)
	cloned.Pattern = clonePatternsWithParent(in.Pattern, cloned)
	cloned.Range = cloneRangeWithParent(in.Range, cloned)
	cloned.RequireInstance = cloneASTValueWithParent(in.RequireInstance, cloned)
	if len(in.Type) != 0 {
		cloned.Type = make([]*Type, 0, len(in.Type))
		for _, nested := range in.Type {
			if nested != nil {
				cloned.Type = append(cloned.Type, cloneTypeWithParent(nested, cloned))
			}
		}
	}
	return cloned
}

func cloneRangeWithParent(in *Range, parent Node) *Range {
	if in == nil {
		return nil
	}
	cloned := &Range{Name: in.Name, Source: in.Source, Parent: parent, Extensions: append([]*Statement(nil), in.Extensions...)}
	cloned.Description = cloneASTValueWithParent(in.Description, cloned)
	cloned.ErrorAppTag = cloneASTValueWithParent(in.ErrorAppTag, cloned)
	cloned.ErrorMessage = cloneASTValueWithParent(in.ErrorMessage, cloned)
	cloned.Reference = cloneASTValueWithParent(in.Reference, cloned)
	return cloned
}

func cloneLengthWithParent(in *Length, parent Node) *Length {
	if in == nil {
		return nil
	}
	cloned := &Length{Name: in.Name, Source: in.Source, Parent: parent, Extensions: append([]*Statement(nil), in.Extensions...)}
	cloned.Description = cloneASTValueWithParent(in.Description, cloned)
	cloned.ErrorAppTag = cloneASTValueWithParent(in.ErrorAppTag, cloned)
	cloned.ErrorMessage = cloneASTValueWithParent(in.ErrorMessage, cloned)
	cloned.Reference = cloneASTValueWithParent(in.Reference, cloned)
	return cloned
}

func clonePatternsWithParent(in []*Pattern, parent Node) []*Pattern {
	if len(in) == 0 {
		return nil
	}
	out := make([]*Pattern, 0, len(in))
	for _, pattern := range in {
		if pattern == nil {
			continue
		}
		cloned := &Pattern{Name: pattern.Name, Source: pattern.Source, Parent: parent, Extensions: append([]*Statement(nil), pattern.Extensions...)}
		cloned.Description = cloneASTValueWithParent(pattern.Description, cloned)
		cloned.ErrorAppTag = cloneASTValueWithParent(pattern.ErrorAppTag, cloned)
		cloned.ErrorMessage = cloneASTValueWithParent(pattern.ErrorMessage, cloned)
		cloned.Modifier = cloneASTValueWithParent(pattern.Modifier, cloned)
		cloned.Reference = cloneASTValueWithParent(pattern.Reference, cloned)
		out = append(out, cloned)
	}
	return out
}

func cloneEnumsWithParent(in []*Enum, parent Node) []*Enum {
	if len(in) == 0 {
		return nil
	}
	out := make([]*Enum, 0, len(in))
	for _, enum := range in {
		if enum == nil {
			continue
		}
		cloned := &Enum{Name: enum.Name, Source: enum.Source, Parent: parent, Extensions: append([]*Statement(nil), enum.Extensions...)}
		cloned.Description = cloneASTValueWithParent(enum.Description, cloned)
		cloned.IfFeature = cloneASTValuesWithParent(enum.IfFeature, cloned)
		cloned.Reference = cloneASTValueWithParent(enum.Reference, cloned)
		cloned.Status = cloneASTValueWithParent(enum.Status, cloned)
		cloned.Value = cloneASTValueWithParent(enum.Value, cloned)
		out = append(out, cloned)
	}
	return out
}

func cloneBitsWithParent(in []*Bit, parent Node) []*Bit {
	if len(in) == 0 {
		return nil
	}
	out := make([]*Bit, 0, len(in))
	for _, bit := range in {
		if bit == nil {
			continue
		}
		cloned := &Bit{Name: bit.Name, Source: bit.Source, Parent: parent, Extensions: append([]*Statement(nil), bit.Extensions...)}
		cloned.Description = cloneASTValueWithParent(bit.Description, cloned)
		cloned.IfFeature = cloneASTValuesWithParent(bit.IfFeature, cloned)
		cloned.Position = cloneASTValueWithParent(bit.Position, cloned)
		cloned.Reference = cloneASTValueWithParent(bit.Reference, cloned)
		cloned.Status = cloneASTValueWithParent(bit.Status, cloned)
		out = append(out, cloned)
	}
	return out
}

func cloneIdentities(in []*Identity, parent Node) []*Identity {
	if len(in) == 0 {
		return nil
	}
	out := make([]*Identity, 0, len(in))
	for _, identity := range in {
		if identity == nil {
			continue
		}
		cloned := &Identity{
			Name:       identity.Name,
			Source:     identity.Source,
			Parent:     parent,
			Extensions: append([]*Statement(nil), identity.Extensions...),
		}
		cloned.Base = cloneValuesWithParent(identity.Base, cloned)
		cloned.Description = cloneValueWithParent(identity.Description, cloned)
		cloned.IfFeature = cloneValuesWithParent(identity.IfFeature, cloned)
		cloned.Reference = cloneValueWithParent(identity.Reference, cloned)
		cloned.Status = cloneValueWithParent(identity.Status, cloned)
		cloned.Values = cloneIdentities(identity.Values, cloned)
		out = append(out, cloned)
	}
	return out
}

func identitiesFromASTIdentities(in []*ASTIdentity, parent Node) []*Identity {
	if len(in) == 0 {
		return nil
	}
	out := make([]*Identity, 0, len(in))
	for _, identity := range in {
		if identity == nil {
			continue
		}
		cloned := &Identity{
			Name:       identity.Name,
			Source:     identity.Source,
			Parent:     parent,
			Extensions: append([]*Statement(nil), identity.Extensions...),
		}
		cloned.Base = valuesFromASTValues(identity.Base, cloned)
		cloned.Description = valueFromASTValue(identity.Description, cloned)
		cloned.IfFeature = valuesFromASTValues(identity.IfFeature, cloned)
		cloned.Reference = valueFromASTValue(identity.Reference, cloned)
		cloned.Status = valueFromASTValue(identity.Status, cloned)
		cloned.Values = identitiesFromASTIdentities(identity.Values, cloned)
		out = append(out, cloned)
	}
	return out
}

func cloneValuesWithParent(in []*Value, parent Node) []*Value {
	if len(in) == 0 {
		return nil
	}
	out := make([]*Value, 0, len(in))
	for _, value := range in {
		if cloned := cloneValueWithParent(value, parent); cloned != nil {
			out = append(out, cloned)
		}
	}
	return out
}

func valuesFromASTValues(in []*ASTValue, parent Node) []*Value {
	if len(in) == 0 {
		return nil
	}
	out := make([]*Value, 0, len(in))
	for _, value := range in {
		if cloned := valueFromASTValue(value, parent); cloned != nil {
			out = append(out, cloned)
		}
	}
	return out
}

func valueFromASTValue(in *ASTValue, parent Node) *Value {
	if in == nil {
		return nil
	}
	cloned := &Value{
		Name:       in.Name,
		Parent:     parent,
		Source:     in.Source,
		Extensions: append([]*Statement(nil), in.Extensions...),
	}
	cloned.Description = valueFromASTValue(in.Description, cloned)
	return cloned
}

func cloneValueWithParent(in *Value, parent Node) *Value {
	if in == nil {
		return nil
	}
	cloned := &Value{
		Name:       in.Name,
		Parent:     parent,
		Source:     in.Source,
		Extensions: append([]*Statement(nil), in.Extensions...),
	}
	cloned.Description = cloneValueWithParent(in.Description, cloned)
	return cloned
}

func cloneASTValuesWithParent(in []*ASTValue, parent Node) []*ASTValue {
	if len(in) == 0 {
		return nil
	}
	out := make([]*ASTValue, 0, len(in))
	for _, value := range in {
		if cloned := cloneASTValueWithParent(value, parent); cloned != nil {
			out = append(out, cloned)
		}
	}
	return out
}

func cloneASTValue(value *ASTValue) *ASTValue {
	if value == nil {
		return nil
	}
	return &ASTValue{
		Name:        value.Name,
		Source:      value.Source,
		Parent:      value.Parent,
		Extensions:  append([]*Statement(nil), value.Extensions...),
		Description: cloneASTValue(value.Description),
	}
}

func (ms *Modules) closeContext() {
	if ms != nil && ms.ctx != nil {
		ms.ctx.Close()
		ms.ctx = nil
	}
}

func (ms *Modules) resetNamespaceCache() {
	ms.nsMu.Lock()
	defer ms.nsMu.Unlock()
	ms.byNS = make(map[string]*Module)
}
