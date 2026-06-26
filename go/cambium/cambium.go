// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

// Package cambium is the default pure-Go schema and codegen surface of Cambium.
//
// It builds an ordered schema IR from Cambium's internal YANG parser adapter.
// Backend data parsing, validation, and serialization live in optional backend
// packages.
package cambium

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/signalbreak-labs/cambium/go/internal/yangparse"
)

const memoryModuleSource = "<memory>"

// RuleCode is a stable Cambium diagnostic code (see spec/rule-codes.md). The
// same failure yields the same code in Rust and Go where the selected tier
// supports the operation.
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
	case "new context", "context builder", "set search path", "unset search path", "set features", "load module", "schema tree", "schema diff":
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
		"children", "siblings", "schema", "value", "diff", "diff apply":
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
	// Code is the stable rule code.
	Code RuleCode
	// Op is the operation that failed.
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

// ContextFlags controls context creation. The pure-Go default has no internal
// yang-library module to suppress, but the public shape mirrors the
// backend/Rust builder and the relevant loading flags are honored.
type ContextFlags struct {
	NoYangLibrary       bool
	AllImplemented      bool
	RefImplemented      bool
	DisableSearchdirCwd bool
}

// ValidationMode controls how strictly schema source defects are handled during
// context loading.
type ValidationMode uint8

const (
	// ValidationStrict enforces RFC-compatible source validation. It is the
	// default and rejects duplicate direct revision dates.
	ValidationStrict ValidationMode = iota
	// ValidationVendorCompatible permits selected real-world vendor source
	// defects and reports each relaxation as a LoadReport warning.
	ValidationVendorCompatible
)

func validValidationMode(mode ValidationMode) bool {
	return mode == ValidationStrict || mode == ValidationVendorCompatible
}

// ContextBuilder is the mutable phase for constructing a frozen Context.
type ContextBuilder struct {
	ctx   *Context
	built bool
}

// NewContextBuilder creates a mutable pure-Go context builder.
func NewContextBuilder(flags ContextFlags) (*ContextBuilder, error) {
	ctx, err := newContext(flags)
	if err != nil {
		return nil, err
	}
	return &ContextBuilder{ctx: ctx}, nil
}

func (b *ContextBuilder) ensureMutable() error {
	if b == nil || b.ctx == nil {
		return wrap("context builder", fmt.Errorf("nil context builder"))
	}
	if b.built {
		return wrap("context builder", fmt.Errorf("context builder has already built a context"))
	}
	return nil
}

// SearchPath appends a directory to the builder's module search path.
func (b *ContextBuilder) SearchPath(path string) error {
	if err := b.ensureMutable(); err != nil {
		return err
	}
	return b.ctx.SetSearchPath(path)
}

// UnsetSearchPath removes a directory from the builder's module search path.
func (b *ContextBuilder) UnsetSearchPath(path string) error {
	if err := b.ensureMutable(); err != nil {
		return err
	}
	return b.ctx.UnsetSearchPath(path)
}

// SearchPaths returns a stable snapshot of the builder's configured module
// search path. The implicit current working directory is not included.
func (b *ContextBuilder) SearchPaths() []string {
	if b == nil || b.ctx == nil {
		return nil
	}
	return b.ctx.SearchPaths()
}

// SetFeatures replaces the enabled feature set for module in the builder.
func (b *ContextBuilder) SetFeatures(module string, features []string) error {
	if err := b.ensureMutable(); err != nil {
		return err
	}
	return b.ctx.SetFeatures(module, features)
}

// SetValidationMode selects strict RFC validation or explicit vendor-compatible
// loading for this builder. The default is ValidationStrict.
func (b *ContextBuilder) SetValidationMode(mode ValidationMode) error {
	if err := b.ensureMutable(); err != nil {
		return err
	}
	if !validValidationMode(mode) {
		return wrap("context builder", fmt.Errorf("invalid validation mode %d", mode))
	}
	b.ctx.validationMode = mode
	return nil
}

// LoadModule loads an implemented module, optionally pinned to revision, and
// optionally enables features for that module before schema materialization.
func (b *ContextBuilder) LoadModule(name string, revision *string, features []string) error {
	if err := b.ensureMutable(); err != nil {
		return err
	}
	snap := b.ctx.snapshot()
	if features != nil {
		if err := b.ctx.SetFeatures(name, features); err != nil {
			b.ctx.restore(snap)
			return err
		}
	}
	if err := b.ctx.loadModuleByName(name, revision, true, true); err != nil {
		b.ctx.restore(snap)
		return err
	}
	return nil
}

// LoadModuleFromPath loads an implemented module directly from path.
func (b *ContextBuilder) LoadModuleFromPath(path string) error {
	if err := b.ensureMutable(); err != nil {
		return err
	}
	snap := b.ctx.snapshot()
	if _, err := b.ctx.loadModulePath(path, true, true); err != nil {
		b.ctx.restore(snap)
		return wrap("load module", err)
	}
	return nil
}

// LoadModuleStr loads an implemented module from an in-memory YANG source.
func (b *ContextBuilder) LoadModuleStr(source string) error {
	if err := b.ensureMutable(); err != nil {
		return err
	}
	snap := b.ctx.snapshot()
	if _, err := b.ctx.loadModuleSource(source, memoryModuleSource, true, true); err != nil {
		b.ctx.restore(snap)
		return wrap("load module", err)
	}
	return nil
}

// Build consumes the mutable phase and returns a frozen schema context.
func (b *ContextBuilder) Build() (*Context, error) {
	if err := b.ensureMutable(); err != nil {
		return nil, err
	}
	if err := b.ctx.rebuildIfDirty(); err != nil {
		return nil, wrap("context builder", err)
	}
	b.built = true
	b.ctx.frozen = true
	return b.ctx, nil
}

// Context is a pure-Go YANG schema loading context.
type Context struct {
	searchPaths     []string
	modules         map[string]*moduleData
	modulesByKey    map[string]*moduleData
	modulesByNS     map[string]*moduleData
	modulesByNSKey  map[string]*moduleData
	loadOrder       []*moduleData
	enabledFeatures map[string]map[string]struct{}
	allImplemented  bool
	refImplemented  bool
	searchCwd       bool
	validationMode  ValidationMode
	loadWarnings    []Diagnostic
	dirty           bool
	frozen          bool
	closed          bool
}

type contextSnapshot struct {
	searchPaths     []string
	modules         map[string]*moduleData
	modulesByKey    map[string]*moduleData
	modulesByNS     map[string]*moduleData
	modulesByNSKey  map[string]*moduleData
	loadOrder       []*moduleData
	enabledFeatures map[string]map[string]struct{}
	validationMode  ValidationMode
	loadWarnings    []Diagnostic
	dirty           bool
	modulesState    []moduleDataSnapshot
}

type moduleDataSnapshot struct {
	mod               *moduleData
	namespace         string
	prefix            string
	revision          string
	file              string
	stmt              *yangparse.Statement
	implemented       bool
	submodules        []*submoduleData
	imports           []Import
	importByPfx       map[string]*moduleData
	sourceImportByPfx map[*yangparse.Statement]map[string]*moduleData
	requested         bool
}

// NewContext creates an empty pure-Go schema context.
func NewContext() (*Context, error) {
	return newContext(ContextFlags{})
}

func newContext(flags ContextFlags) (*Context, error) {
	return &Context{
		modules:         make(map[string]*moduleData),
		modulesByKey:    make(map[string]*moduleData),
		modulesByNS:     make(map[string]*moduleData),
		modulesByNSKey:  make(map[string]*moduleData),
		enabledFeatures: make(map[string]map[string]struct{}),
		allImplemented:  flags.AllImplemented,
		refImplemented:  flags.RefImplemented,
		searchCwd:       !flags.DisableSearchdirCwd,
	}, nil
}

// Close releases context-owned schema state. It is idempotent.
func (c *Context) Close() {
	if c == nil {
		return
	}
	c.closed = true
	c.searchPaths = nil
	c.modules = nil
	c.modulesByKey = nil
	c.modulesByNS = nil
	c.modulesByNSKey = nil
	c.loadOrder = nil
	c.enabledFeatures = nil
	c.loadWarnings = nil
	c.dirty = false
}

func (c *Context) snapshot() contextSnapshot {
	snap := contextSnapshot{
		searchPaths:     append([]string(nil), c.searchPaths...),
		modules:         cloneModuleMap(c.modules),
		modulesByKey:    cloneModuleMap(c.modulesByKey),
		modulesByNS:     cloneModuleMap(c.modulesByNS),
		modulesByNSKey:  cloneModuleMap(c.modulesByNSKey),
		loadOrder:       append([]*moduleData(nil), c.loadOrder...),
		enabledFeatures: cloneFeatureMap(c.enabledFeatures),
		validationMode:  c.validationMode,
		loadWarnings:    append([]Diagnostic(nil), c.loadWarnings...),
		dirty:           c.dirty,
	}
	for _, mod := range c.loadOrder {
		if mod == nil {
			continue
		}
		snap.modulesState = append(snap.modulesState, moduleDataSnapshot{
			mod:               mod,
			namespace:         mod.namespace,
			prefix:            mod.prefix,
			revision:          mod.revision,
			file:              mod.file,
			stmt:              mod.stmt,
			implemented:       mod.implemented,
			submodules:        append([]*submoduleData(nil), mod.submodules...),
			imports:           append([]Import(nil), mod.imports...),
			importByPfx:       cloneModuleMap(mod.importByPfx),
			sourceImportByPfx: cloneSourceImportMap(mod.sourceImportByPfx),
			requested:         mod.requested,
		})
	}
	return snap
}

func (c *Context) restore(snap contextSnapshot) {
	c.searchPaths = snap.searchPaths
	c.modules = snap.modules
	c.modulesByKey = snap.modulesByKey
	c.modulesByNS = snap.modulesByNS
	c.modulesByNSKey = snap.modulesByNSKey
	c.loadOrder = snap.loadOrder
	c.enabledFeatures = snap.enabledFeatures
	c.validationMode = snap.validationMode
	c.loadWarnings = snap.loadWarnings
	c.dirty = snap.dirty
	for _, state := range snap.modulesState {
		mod := state.mod
		if mod == nil {
			continue
		}
		mod.namespace = state.namespace
		mod.prefix = state.prefix
		mod.revision = state.revision
		mod.file = state.file
		mod.stmt = state.stmt
		mod.implemented = state.implemented
		mod.submodules = state.submodules
		mod.imports = state.imports
		mod.importByPfx = state.importByPfx
		mod.sourceImportByPfx = state.sourceImportByPfx
		mod.requested = state.requested
	}
}

func (c *Context) addLoadWarnings(warnings []Diagnostic) {
	if len(warnings) == 0 {
		return
	}
	c.loadWarnings = append(c.loadWarnings, warnings...)
}

func cloneModuleMap(in map[string]*moduleData) map[string]*moduleData {
	if in == nil {
		return nil
	}
	out := make(map[string]*moduleData, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}

func cloneSourceImportMap(in map[*yangparse.Statement]map[string]*moduleData) map[*yangparse.Statement]map[string]*moduleData {
	if in == nil {
		return nil
	}
	out := make(map[*yangparse.Statement]map[string]*moduleData, len(in))
	for source, imports := range in {
		out[source] = cloneModuleMap(imports)
	}
	return out
}

func cloneFeatureMap(in map[string]map[string]struct{}) map[string]map[string]struct{} {
	if in == nil {
		return nil
	}
	out := make(map[string]map[string]struct{}, len(in))
	for module, features := range in {
		copied := make(map[string]struct{}, len(features))
		for feature := range features {
			copied[feature] = struct{}{}
		}
		out[module] = copied
	}
	return out
}

func (c *Context) ensureMutable(op string) error {
	if c == nil {
		return wrap(op, fmt.Errorf("nil context"))
	}
	if c.closed {
		return wrap(op, fmt.Errorf("context is closed"))
	}
	if c.frozen {
		return wrap(op, fmt.Errorf("context is frozen; use ContextBuilder before Build"))
	}
	return nil
}

// SetSearchPath appends a directory to the module search path.
func (c *Context) SetSearchPath(path string) error {
	if err := c.ensureMutable("set search path"); err != nil {
		return err
	}
	if path == "" {
		return wrap("set search path", fmt.Errorf("empty search path"))
	}
	c.searchPaths = append(c.searchPaths, path)
	return nil
}

// UnsetSearchPath removes a directory from the module search path.
func (c *Context) UnsetSearchPath(path string) error {
	if err := c.ensureMutable("unset search path"); err != nil {
		return err
	}
	if path == "" {
		return wrap("unset search path", fmt.Errorf("empty search path"))
	}
	for i, existing := range c.searchPaths {
		if existing == path {
			c.searchPaths = append(c.searchPaths[:i], c.searchPaths[i+1:]...)
			return nil
		}
	}
	return wrap("unset search path", fmt.Errorf("search path %q not configured", path))
}

// SearchPaths returns a stable snapshot of the configured module search path.
// The implicit current working directory is not included.
func (c *Context) SearchPaths() []string {
	if c == nil {
		return nil
	}
	return append([]string(nil), c.searchPaths...)
}

// SetFeatures replaces the enabled feature set for module. Features are
// disabled by default; pass an empty slice to disable all features for a
// module. Feature names are local to module and must not include a prefix.
func (c *Context) SetFeatures(module string, features []string) error {
	if err := c.ensureMutable("set features"); err != nil {
		return err
	}
	if !validYangIdentifier(module, true) {
		return wrap("set features", fmt.Errorf("invalid module name %q", module))
	}
	set := make(map[string]struct{}, len(features))
	for _, feature := range features {
		if !validYangIdentifier(feature, true) {
			return wrap("set features", fmt.Errorf("invalid feature name %q for module %q", feature, module))
		}
		set[feature] = struct{}{}
	}
	if c.enabledFeatures == nil {
		c.enabledFeatures = make(map[string]map[string]struct{})
	}
	if len(set) == 0 {
		delete(c.enabledFeatures, module)
	} else {
		c.enabledFeatures[module] = set
	}
	c.dirty = true
	return nil
}

// LoadModule loads an implemented module by name from the configured search
// paths. The loader looks for name.yang first, then any name@revision.yang file.
func (c *Context) LoadModule(name string) error {
	return c.loadModuleByName(name, nil, true, true)
}

// LoadModuleFromPath loads an implemented module directly from path.
func (c *Context) LoadModuleFromPath(path string) error {
	if err := c.ensureMutable("load module"); err != nil {
		return err
	}
	snap := c.snapshot()
	if _, err := c.loadModulePath(path, true, true); err != nil {
		c.restore(snap)
		return wrap("load module", err)
	}
	return nil
}

func (c *Context) loadModuleByName(name string, revision *string, implemented, requested bool) error {
	if err := c.ensureMutable("load module"); err != nil {
		return err
	}
	if !validYangIdentifier(name, true) {
		return wrap("load module", fmt.Errorf("invalid module name %q", name))
	}
	revisionValue := revisionString(revision)
	if revisionValue != "" && !validRevisionDate(revisionValue) {
		return wrap("load module", fmt.Errorf("invalid revision %q", revisionValue))
	}
	snap := c.snapshot()
	path, err := c.findModulePathRevision(name, revisionValue)
	if err != nil {
		c.restore(snap)
		return wrap("load module", err)
	}
	ok, err := moduleFileDeclaresModule(path, name)
	if err != nil {
		c.restore(snap)
		return wrap("load module", err)
	}
	if !ok {
		c.restore(snap)
		return wrap("load module", fmt.Errorf("YANG file %q does not declare module %q", path, name))
	}
	if _, err := c.loadModulePath(path, implemented, requested); err != nil {
		c.restore(snap)
		return wrap("load module", err)
	}
	return nil
}

func (c *Context) findModulePathRevision(name, revision string) (string, error) {
	for _, dir := range c.moduleSearchDirs() {
		path, ok, err := findModuleInSearchDir(dir, name, revision, c.validationMode)
		if err != nil {
			return "", err
		}
		if ok {
			return path, nil
		}
	}
	if revision != "" {
		return "", fmt.Errorf("module %q revision %q not found in search path", name, revision)
	}
	return "", fmt.Errorf("module %q not found in search path", name)
}

func findModuleInSearchDir(dir, name, revision string, mode ValidationMode) (file string, found bool, err error) {
	clean := filepath.Clean(dir)
	if filepath.Base(clean) == "..." {
		return findModuleRecursive(moduleSourceDirectory(clean), name, revision, mode)
	}
	return findModuleDirect(clean, name, revision, mode)
}

func findModuleDirect(dir, name, revision string, mode ValidationMode) (file string, found bool, err error) {
	if revision != "" {
		revisioned := filepath.Join(dir, name+"@"+revision+".yang")
		if ok, err := fileExists(revisioned); err != nil {
			return "", false, err
		} else if ok {
			if moduleFileMatchesRevision(revisioned, name, revision, mode) {
				return revisioned, true, nil
			}
			return "", false, fmt.Errorf("YANG file %q does not declare filename revision %q", revisioned, revision)
		}
		candidate := filepath.Join(dir, name+".yang")
		if ok, err := fileExists(candidate); err != nil {
			return "", false, err
		} else if ok && moduleFileMatchesRevision(candidate, name, revision, mode) {
			return candidate, true, nil
		}
		return "", false, nil
	}
	candidate := filepath.Join(dir, name+".yang")
	if _, err := os.Stat(candidate); err == nil {
		ok, err := moduleFileDeclaresName(candidate, name)
		if err != nil {
			return "", false, err
		}
		if !ok {
			return "", false, fmt.Errorf("YANG file %q does not declare module or submodule %q", candidate, name)
		}
		return candidate, true, nil
	} else if !os.IsNotExist(err) {
		// In the else of os.Stat's err==nil, err is necessarily non-nil.
		return "", false, err
	}
	matches, err := filepath.Glob(filepath.Join(dir, name+"@*.yang"))
	if err != nil {
		return "", false, err
	}
	var valid []string
	for _, match := range matches {
		revision, ok := moduleFilenameRevision(filepath.Base(match), name)
		if !ok {
			continue
		}
		if moduleFileMatchesRevision(match, name, revision, mode) {
			valid = append(valid, match)
			continue
		}
		return "", false, fmt.Errorf("YANG file %q does not declare filename revision %q", match, revision)
	}
	if len(valid) > 0 {
		return valid[len(valid)-1], true, nil
	}
	return "", false, nil
}

func findModuleRecursive(dir, name, revision string, mode ValidationMode) (file string, found bool, err error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return "", false, nil
		}
		return "", false, err
	}
	if revision != "" {
		moduleFile := name + ".yang"
		revisionedFile := name + "@" + revision + ".yang"
		var fallback string
		var dirs []string
		for _, entry := range entries {
			entryName := entry.Name()
			path := filepath.Join(dir, entryName)
			if entry.IsDir() {
				dirs = append(dirs, path)
				continue
			}
			switch entryName {
			case revisionedFile:
				if moduleFileMatchesRevision(path, name, revision, mode) {
					return path, true, nil
				}
				return "", false, fmt.Errorf("YANG file %q does not declare filename revision %q", path, revision)
			case moduleFile:
				if moduleFileMatchesRevision(path, name, revision, mode) {
					fallback = path
				}
			}
		}
		if fallback != "" {
			return fallback, true, nil
		}
		for _, dirPath := range dirs {
			subFile, ok, err := findModuleRecursive(dirPath, name, revision, mode)
			if err != nil || ok {
				return subFile, ok, err
			}
		}
		return "", false, nil
	}
	var revisions []string
	moduleFile := name + ".yang"
	var dirs []string
	for _, entry := range entries {
		entryName := entry.Name()
		path := filepath.Join(dir, entryName)
		if entry.IsDir() {
			dirs = append(dirs, path)
			continue
		}
		if entryName == moduleFile {
			ok, err := moduleFileDeclaresName(path, name)
			if err != nil {
				return "", false, err
			}
			if !ok {
				return "", false, fmt.Errorf("YANG file %q does not declare module or submodule %q", path, name)
			}
			return path, true, nil
		}
		if filenameRevision, ok := moduleFilenameRevision(entryName, name); ok {
			if moduleFileMatchesRevision(path, name, filenameRevision, mode) {
				revisions = append(revisions, path)
				continue
			}
			return "", false, fmt.Errorf("YANG file %q does not declare filename revision %q", path, filenameRevision)
		}
	}
	if len(revisions) > 0 {
		return revisions[len(revisions)-1], true, nil
	}
	for _, dirPath := range dirs {
		subFile, ok, err := findModuleRecursive(dirPath, name, revision, mode)
		if err != nil || ok {
			return subFile, ok, err
		}
	}
	return "", false, nil
}

func fileExists(path string) (bool, error) {
	if _, err := os.Stat(path); err == nil {
		return true, nil
	} else if !os.IsNotExist(err) {
		return false, err
	}
	return false, nil
}

func moduleFilenameRevision(filename, name string) (string, bool) {
	if !strings.HasPrefix(filename, name+"@") || !strings.HasSuffix(filename, ".yang") {
		return "", false
	}
	revision := strings.TrimSuffix(strings.TrimPrefix(filename, name+"@"), ".yang")
	if !validRevisionDate(revision) {
		return "", false
	}
	return revision, true
}

func (c *Context) moduleSearchDirs() []string {
	dirs := make([]string, 0, len(c.searchPaths)+1)
	if c.searchCwd {
		dirs = append(dirs, ".")
	}
	dirs = append(dirs, c.searchPaths...)
	return dirs
}

func revisionString(revision *string) string {
	if revision == nil {
		return ""
	}
	return *revision
}

func moduleFileDeclaresName(path, name string) (bool, error) {
	return moduleFileDeclaresKeywordName(path, name, "")
}

func moduleFileDeclaresModule(path, name string) (bool, error) {
	return moduleFileDeclaresKeywordName(path, name, "module")
}

func moduleFileDeclaresKeywordName(path, name, keyword string) (bool, error) {
	raw, err := yangparse.ReadFile(path)
	if err != nil {
		return false, err
	}
	stmts, err := yangparse.Parse(raw, path)
	if err != nil {
		return false, err
	}
	if len(stmts) != 1 {
		return false, fmt.Errorf("%s: expected one module or submodule statement, got %d", path, len(stmts))
	}
	stmt := stmts[0]
	if stmt.Keyword != "module" && stmt.Keyword != "submodule" {
		return false, nil
	}
	if keyword != "" && stmt.Keyword != keyword {
		return false, nil
	}
	return stmt.Argument == name, nil
}

func moduleFileMatchesRevision(path, name, revision string, mode ValidationMode) bool {
	raw, err := yangparse.ReadFile(path)
	if err != nil {
		return false
	}
	stmts, err := yangparse.Parse(raw, path)
	if err != nil || len(stmts) != 1 {
		return false
	}
	stmt := stmts[0]
	if (stmt.Keyword != "module" && stmt.Keyword != "submodule") || stmt.Argument != name {
		return false
	}
	effectiveRevision, _, err := moduleRevisionValidatedMode(stmt, mode)
	if err != nil {
		return false
	}
	if mode == ValidationVendorCompatible {
		return moduleDeclaresRevision(stmt, revision)
	}
	return effectiveRevision == revision
}

func moduleDeclaresRevision(stmt *yangparse.Statement, revision string) bool {
	for _, rev := range direct(stmt, "revision") {
		if rev.Argument == revision {
			return true
		}
	}
	return false
}

func (c *Context) loadModulePath(path string, implemented, requested bool) (*moduleData, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	raw, err := yangparse.ReadFile(abs)
	if err != nil {
		return nil, err
	}
	dir := moduleSourceDirectory(abs)
	addedSearchPath := c.addSearchPathIfMissing(dir)
	mod, err := c.loadModuleSource(raw, abs, implemented, requested)
	if err != nil && addedSearchPath {
		c.removeSearchPath(dir)
	}
	return mod, err
}

func moduleSourceDirectory(path string) string {
	dir, _ := filepath.Split(path)
	if dir == "" {
		return "."
	}
	return filepath.Clean(dir)
}

func (c *Context) addSearchPathIfMissing(path string) bool {
	if path == "" {
		return false
	}
	for _, existing := range c.searchPaths {
		if existing == path {
			return false
		}
	}
	c.searchPaths = append(c.searchPaths, path)
	return true
}

func (c *Context) removeSearchPath(path string) {
	for i, existing := range c.searchPaths {
		if existing == path {
			c.searchPaths = append(c.searchPaths[:i], c.searchPaths[i+1:]...)
			return
		}
	}
}

func (c *Context) loadModuleSource(source, sourceName string, implemented, requested bool) (*moduleData, error) {
	if c.allImplemented {
		implemented = true
	}
	stmts, err := yangparse.Parse(source, sourceName)
	if err != nil {
		return nil, err
	}
	if len(stmts) != 1 {
		return nil, fmt.Errorf("%s: expected one module or submodule statement, got %d", sourceName, len(stmts))
	}
	stmt := stmts[0]
	if stmt.Keyword == "submodule" {
		if c.validationMode == ValidationVendorCompatible {
			return c.loadDirectSubmoduleSource(sourceName, stmt, implemented, requested)
		}
		return nil, c.rejectDirectSubmoduleSource(sourceName, stmt)
	}
	if stmt.Keyword != "module" {
		return nil, fmt.Errorf("%s: top-level statement is %q, want module", sourceName, stmt.Keyword)
	}
	name := stmt.Argument
	allowXMLPrefix := childArg(stmt, "yang-version") == "1.1"
	if err := validateYangIdentifierArg("module", name, stmt, allowXMLPrefix); err != nil {
		return nil, err
	}
	if err := validateNoStandardChildStatements(stmt); err != nil {
		return nil, err
	}
	orderWarnings, err := validateTopLevelStatementOrderMode(stmt, c.validationMode)
	if err != nil {
		return nil, err
	}
	if err := validateYangVersion(stmt); err != nil {
		return nil, err
	}
	if err := validateModuleHeaderTextMetadata(stmt); err != nil {
		return nil, err
	}
	if belongs := first(stmt, "belongs-to"); belongs != nil {
		return nil, fmt.Errorf("belongs-to %q is not valid in module %q at %s", belongs.Argument, name, belongs.Location())
	}
	namespaceStmt, err := singletonChild(stmt, "namespace")
	if err != nil {
		return nil, err
	}
	if namespaceStmt == nil {
		return nil, fmt.Errorf("module %q has no namespace", name)
	}
	namespace := namespaceStmt.Argument
	prefixStmt, err := singletonChild(stmt, "prefix")
	if err != nil {
		return nil, err
	}
	prefix := ""
	if prefixStmt != nil {
		prefix = prefixStmt.Argument
	}
	if prefix == "" {
		return nil, fmt.Errorf("module %q has no prefix", name)
	}
	if err := validateYangIdentifierArg("prefix", prefix, prefixStmt, allowXMLPrefix); err != nil {
		return nil, err
	}
	if existing := c.namespaceOwner(namespace, name); existing != "" {
		return nil, fmt.Errorf("namespace %q already belongs to module %q", namespace, existing)
	}
	revision, revisionWarnings, err := moduleRevisionValidatedMode(stmt, c.validationMode)
	if err != nil {
		return nil, err
	}
	c.addLoadWarnings(append(orderWarnings, revisionWarnings...))
	key := moduleKey(name, revision)
	mod := c.modulesByKey[key]
	if mod == nil {
		mod = &moduleData{
			ctx:               c,
			name:              name,
			file:              sourceName,
			stmt:              stmt,
			importByPfx:       make(map[string]*moduleData),
			sourceImportByPfx: make(map[*yangparse.Statement]map[string]*moduleData),
		}
		c.modulesByKey[key] = mod
		c.loadOrder = append(c.loadOrder, mod)
	} else {
		if mod.stmt != nil && !equivalentModuleSource(mod.file, sourceName) {
			return nil, duplicateModuleIdentityError(name, revision)
		}
		mod.file = sourceName
		mod.stmt = stmt
		mod.submodules = nil
	}
	if implemented {
		mod.implemented = true
	}
	if requested {
		mod.requested = true
	}
	mod.loadMeta()
	c.registerNameDefault(mod)
	if err := c.loadIncludes(mod); err != nil {
		return nil, err
	}
	if err := c.loadImports(mod); err != nil {
		return nil, err
	}
	c.dirty = true
	return mod, nil
}

func equivalentModuleSource(existing, current string) bool {
	if existing == "" || current == "" {
		return false
	}
	if existing == memoryModuleSource || current == memoryModuleSource {
		return false
	}
	if existing == current {
		return true
	}
	if sameResolvedSourcePath(existing, current) {
		return true
	}
	return sameSourceContent(existing, current)
}

func sameResolvedSourcePath(a, b string) bool {
	aAbs, aErr := filepath.Abs(a)
	bAbs, bErr := filepath.Abs(b)
	if aErr != nil || bErr != nil {
		return false
	}
	aReal, aErr := filepath.EvalSymlinks(aAbs)
	bReal, bErr := filepath.EvalSymlinks(bAbs)
	if aErr == nil && bErr == nil && aReal == bReal {
		return true
	}
	aInfo, aErr := os.Stat(aAbs)
	bInfo, bErr := os.Stat(bAbs)
	return aErr == nil && bErr == nil && os.SameFile(aInfo, bInfo)
}

func sameSourceContent(a, b string) bool {
	aRaw, aErr := os.ReadFile(a)
	bRaw, bErr := os.ReadFile(b)
	return aErr == nil && bErr == nil && bytes.Equal(aRaw, bRaw)
}

func duplicateModuleIdentityError(name, revision string) error {
	if revision == "" {
		return fmt.Errorf("module %q with no revision already loaded", name)
	}
	return fmt.Errorf("module %q revision %q already loaded", name, revision)
}

func (c *Context) namespaceOwner(namespace, moduleName string) string {
	for _, mod := range c.loadOrder {
		if mod == nil || mod.stmt == nil || mod.namespace != namespace || mod.name == moduleName {
			continue
		}
		return mod.name
	}
	return ""
}

func moduleKey(name, revision string) string {
	return name + "\x00" + revision
}

func moduleRevision(stmt *yangparse.Statement) string {
	revision := ""
	for _, r := range direct(stmt, "revision") {
		if r.Argument > revision {
			revision = r.Argument
		}
	}
	return revision
}

func moduleRevisionValidatedMode(stmt *yangparse.Statement, mode ValidationMode) (string, []Diagnostic, error) {
	revision := ""
	seen := make(map[string]*yangparse.Statement)
	var warnings []Diagnostic
	for _, r := range direct(stmt, "revision") {
		if !validRevisionDate(r.Argument) {
			return "", nil, fmt.Errorf("invalid revision %q at %s", r.Argument, r.Location())
		}
		if err := validateRevisionMetadata(r); err != nil {
			return "", nil, err
		}
		if previous := seen[r.Argument]; previous != nil {
			if mode != ValidationVendorCompatible {
				return "", nil, diagnosticErrorf(
					r,
					[]*yangparse.Statement{previous},
					"duplicate revision %q at %s",
					r.Argument,
					r.Location(),
				)
			}
			warnings = append(warnings, duplicateRevisionWarning(stmt, previous, r))
		} else {
			seen[r.Argument] = r
		}
		if r.Argument > revision {
			revision = r.Argument
		}
	}
	return revision, warnings, nil
}

func duplicateRevisionWarning(owner, previous, duplicate *yangparse.Statement) Diagnostic {
	ownerKind := "module"
	ownerName := ""
	if owner != nil {
		ownerKind = owner.Keyword
		ownerName = owner.Argument
	}
	revision := ""
	if duplicate != nil {
		revision = duplicate.Argument
	}
	message := fmt.Sprintf(
		"%s %q has duplicate revision %q at %s; previous declaration at %s",
		ownerKind,
		ownerName,
		revision,
		locationText(duplicate),
		locationText(previous),
	)
	return Diagnostic{
		Kind:       DiagnosticSemanticSchemaError,
		Code:       RuleCodeContext,
		Message:    message,
		Module:     ownerName,
		Source:     sourceLocation(duplicate),
		Related:    sourceLocations([]*yangparse.Statement{previous}),
		Underlying: fmt.Errorf("%s", message),
	}
}

func locationText(stmt *yangparse.Statement) string {
	if stmt == nil {
		return "unknown"
	}
	return stmt.Location()
}

func validateRevisionMetadata(stmt *yangparse.Statement) error {
	if err := validateRevisionStatementChildren(stmt); err != nil {
		return err
	}
	return validateStatementTextMetadata(stmt)
}

func validateRevisionStatementChildren(stmt *yangparse.Statement) error {
	for _, child := range stmt.SubStatements() {
		if err := validateNestedChildPlacement(stmt, child); err != nil {
			return err
		}
	}
	return nil
}

func statementRevisionDate(stmt *yangparse.Statement) (string, error) {
	if err := validateStatementTextMetadata(stmt); err != nil {
		return "", err
	}
	dates := direct(stmt, "revision-date")
	if len(dates) == 0 {
		return "", nil
	}
	if len(dates) > 1 {
		return "", fmt.Errorf("duplicate revision-date in %s %q at %s", stmt.Keyword, stmt.Argument, dates[1].Location())
	}
	revision := dates[0].Argument
	if !validRevisionDate(revision) {
		return "", fmt.Errorf("invalid revision-date %q at %s", revision, dates[0].Location())
	}
	return revision, nil
}

func validateDependencyStatementChildren(stmt *yangparse.Statement) error {
	for _, child := range stmt.SubStatements() {
		if err := validateNestedChildPlacement(stmt, child); err != nil {
			return err
		}
	}
	return nil
}

func validateBelongsToStatementChildren(stmt *yangparse.Statement) error {
	for _, child := range stmt.SubStatements() {
		if err := validateNestedChildPlacement(stmt, child); err != nil {
			return err
		}
	}
	return nil
}

func validateNoStandardChildStatements(stmt *yangparse.Statement) error {
	for _, child := range stmt.SubStatements() {
		if err := validateNestedNoStandardChildrenStatementChildPlacement(stmt, child); err != nil {
			return err
		}
		if err := validateNoStandardChildStatements(child); err != nil {
			return err
		}
	}
	return nil
}

func validateStatementTextMetadata(stmt *yangparse.Statement) error {
	if _, err := singletonChild(stmt, "description"); err != nil {
		return err
	}
	if _, err := singletonChild(stmt, "reference"); err != nil {
		return err
	}
	return nil
}

func validateModuleHeaderTextMetadata(stmt *yangparse.Statement) error {
	for _, keyword := range []string{"contact", "organization", "description", "reference"} {
		if _, err := singletonChild(stmt, keyword); err != nil {
			return err
		}
	}
	return nil
}

func singletonChild(stmt *yangparse.Statement, keyword string) (*yangparse.Statement, error) {
	children := direct(stmt, keyword)
	if len(children) == 0 {
		return nil, nil
	}
	if len(children) > 1 {
		return nil, fmt.Errorf("duplicate %s in %s %q at %s", keyword, stmt.Keyword, stmt.Argument, children[1].Location())
	}
	return children[0], nil
}

func validateYangVersion(stmt *yangparse.Statement) error {
	version, err := singletonChild(stmt, "yang-version")
	if err != nil || version == nil {
		return err
	}
	if err := validateYangVersionStatementChildren(version); err != nil {
		return err
	}
	switch version.Argument {
	case "1", "1.1":
		return nil
	default:
		return fmt.Errorf("invalid yang-version %q in %s %q at %s", version.Argument, stmt.Keyword, stmt.Argument, version.Location())
	}
}

func validateYangVersionStatementChildren(stmt *yangparse.Statement) error {
	for _, child := range stmt.SubStatements() {
		if err := validateNestedChildPlacement(stmt, child); err != nil {
			return err
		}
	}
	return nil
}

func validRevisionDate(revision string) bool {
	if len(revision) != len("2006-01-02") {
		return false
	}
	for i, ch := range revision {
		switch i {
		case 4, 7:
			if ch != '-' {
				return false
			}
		default:
			if ch < '0' || ch > '9' {
				return false
			}
		}
	}
	t, err := time.Parse("2006-01-02", revision)
	return err == nil && t.Format("2006-01-02") == revision
}

func (c *Context) registerNameDefault(mod *moduleData) {
	if mod == nil {
		return
	}
	if preferModuleDefault(mod, c.modules[mod.name]) {
		c.modules[mod.name] = mod
	}
	c.modulesByNSKey[moduleKey(mod.namespace, mod.revision)] = mod
	if preferModuleDefault(mod, c.modulesByNS[mod.namespace]) {
		c.modulesByNS[mod.namespace] = mod
	}
}

func preferModuleDefault(candidate, current *moduleData) bool {
	if candidate == nil {
		return false
	}
	if current == nil {
		return true
	}
	if candidate.implemented && !current.implemented {
		return true
	}
	return candidate.implemented == current.implemented && candidate.revision >= current.revision
}

type submoduleValidation struct {
	name       string
	parentName string
	revision   string
}

func validateSubmoduleSource(path string, stmt *yangparse.Statement, parent *moduleData, mode ValidationMode) (submoduleValidation, []Diagnostic, error) {
	info := submoduleValidation{name: stmt.Argument}
	name := info.name
	allowXMLPrefix := childArg(stmt, "yang-version") == "1.1"
	if err := validateYangIdentifierArg("submodule", name, stmt, allowXMLPrefix); err != nil {
		return submoduleValidation{}, nil, err
	}
	if err := validateNoStandardChildStatements(stmt); err != nil {
		return submoduleValidation{}, nil, err
	}
	orderWarnings, err := validateTopLevelStatementOrderMode(stmt, mode)
	if err != nil {
		return submoduleValidation{}, nil, err
	}
	if err := validateYangVersion(stmt); err != nil {
		return submoduleValidation{}, nil, err
	}
	if err := validateModuleHeaderTextMetadata(stmt); err != nil {
		return submoduleValidation{}, nil, err
	}
	if namespace := first(stmt, "namespace"); namespace != nil {
		return submoduleValidation{}, nil, fmt.Errorf("namespace %q is not valid in submodule %q at %s", namespace.Argument, name, namespace.Location())
	}
	if prefix := first(stmt, "prefix"); prefix != nil {
		return submoduleValidation{}, nil, fmt.Errorf("prefix %q is not valid in submodule %q at %s", prefix.Argument, name, prefix.Location())
	}
	parentName := ""
	belongs, err := singletonChild(stmt, "belongs-to")
	if err != nil {
		return submoduleValidation{}, nil, err
	}
	if belongs != nil {
		parentName = belongs.Argument
		if err := validateYangIdentifierArg("belongs-to", parentName, belongs, allowXMLPrefix); err != nil {
			return submoduleValidation{}, nil, err
		}
		if err := validateBelongsToStatementChildren(belongs); err != nil {
			return submoduleValidation{}, nil, err
		}
		prefixStmt, err := singletonChild(belongs, "prefix")
		if err != nil {
			return submoduleValidation{}, nil, err
		}
		prefix := ""
		if prefixStmt != nil {
			prefix = prefixStmt.Argument
		}
		if prefix == "" {
			return submoduleValidation{}, nil, fmt.Errorf("%s: submodule belongs-to %q has no prefix", path, parentName)
		}
		if err := validateYangIdentifierArg("prefix", prefix, prefixStmt, allowXMLPrefix); err != nil {
			return submoduleValidation{}, nil, err
		}
	}
	if parentName == "" {
		return submoduleValidation{}, nil, fmt.Errorf("%s: submodule has no belongs-to", path)
	}
	if parent != nil && parent.name != parentName {
		return submoduleValidation{}, nil, fmt.Errorf("%s: submodule belongs to %q, included by %q", path, parentName, parent.name)
	}
	if err := validateSubmoduleIncludeStatements(stmt, allowXMLPrefix); err != nil {
		return submoduleValidation{}, nil, err
	}
	revision, revisionWarnings, err := moduleRevisionValidatedMode(stmt, mode)
	if err != nil {
		return submoduleValidation{}, nil, err
	}
	info.parentName = parentName
	info.revision = revision
	return info, append(orderWarnings, revisionWarnings...), nil
}

func (c *Context) rejectDirectSubmoduleSource(path string, stmt *yangparse.Statement) error {
	info, _, err := validateSubmoduleSource(path, stmt, nil, c.validationMode)
	if err != nil {
		return err
	}
	return fmt.Errorf("%s: direct submodule %q load is not allowed; load parent module %q", path, info.name, info.parentName)
}

func (c *Context) loadDirectSubmoduleSource(path string, stmt *yangparse.Statement, implemented, requested bool) (*moduleData, error) {
	info, warnings, err := validateSubmoduleSource(path, stmt, nil, c.validationMode)
	if err != nil {
		return nil, err
	}
	c.addLoadWarnings(warnings)
	parentPath, err := c.findModulePathRevision(info.parentName, "")
	if err != nil {
		return nil, fmt.Errorf("%s: direct submodule %q belongs to parent module %q but parent module was not found: %w", path, info.name, info.parentName, err)
	}
	message := fmt.Sprintf("%s: direct submodule %q load resolved to parent module %q in vendor-compatible mode", path, info.name, info.parentName)
	c.addLoadWarnings([]Diagnostic{{
		Kind:       DiagnosticSemanticSchemaError,
		Code:       RuleCodeContext,
		Message:    message,
		Module:     info.parentName,
		Source:     sourceLocation(stmt),
		Underlying: fmt.Errorf("%s", message),
	}})
	return c.loadModulePath(parentPath, implemented, requested)
}

func (c *Context) loadSubmodule(path string, stmt *yangparse.Statement, parent *moduleData) (*moduleData, error) {
	info, warnings, err := validateSubmoduleSource(path, stmt, parent, c.validationMode)
	if err != nil {
		return nil, err
	}
	c.addLoadWarnings(warnings)
	if parent == nil {
		parent = c.modules[info.parentName]
	}
	if parent == nil {
		parent = &moduleData{
			ctx:               c,
			name:              info.parentName,
			importByPfx:       make(map[string]*moduleData),
			sourceImportByPfx: make(map[*yangparse.Statement]map[string]*moduleData),
		}
		c.modules[info.parentName] = parent
		c.modulesByKey[moduleKey(info.parentName, "")] = parent
		c.loadOrder = append(c.loadOrder, parent)
	}
	for _, sub := range parent.submodules {
		if sub.stmt != nil && sub.stmt.Argument == stmt.Argument && moduleRevision(sub.stmt) == info.revision {
			return parent, nil
		}
	}
	parent.submodules = append(parent.submodules, &submoduleData{file: path, stmt: stmt})
	if err := c.loadSubmoduleIncludes(parent, stmt); err != nil {
		return nil, err
	}
	c.dirty = true
	return parent, nil
}

func (c *Context) loadIncludes(mod *moduleData) error {
	includes := direct(mod.stmt, "include")
	if err := validateDuplicateIncludes("module "+strconv.Quote(mod.name), includes); err != nil {
		return err
	}
	for _, inc := range includes {
		revision, err := validateIncludeStatement(inc, mod.yangVersionForStatement(inc) == "1.1")
		if err != nil {
			return err
		}
		path, err := c.findModulePathRevision(inc.Argument, revision)
		if err != nil {
			return err
		}
		if err := c.loadIncludedSubmodulePath(path, mod); err != nil {
			return err
		}
	}
	return nil
}

func (c *Context) loadSubmoduleIncludes(parent *moduleData, stmt *yangparse.Statement) error {
	parentYang11 := parent != nil && parent.yangVersionForStatement(parent.stmt) == "1.1"
	for _, inc := range direct(stmt, "include") {
		revision, err := validateIncludeStatement(inc, childArg(stmt, "yang-version") == "1.1")
		if err != nil {
			return err
		}
		if parentYang11 {
			if err := validateParentYang11NestedInclude(parent, stmt, inc, revision); err != nil {
				return err
			}
		}
		path, err := c.findModulePathRevision(inc.Argument, revision)
		if err != nil {
			return err
		}
		if err := c.loadIncludedSubmodulePath(path, parent); err != nil {
			return err
		}
	}
	return nil
}

func validateParentYang11NestedInclude(parent *moduleData, submodule, inc *yangparse.Statement, revision string) error {
	if parentDirectlyIncludesSubmoduleRevision(parent, inc.Argument, revision) {
		return nil
	}
	if revision != "" && parentDirectlyIncludesSubmoduleName(parent, inc.Argument) {
		return fmt.Errorf("YANG 1.1 submodule %q includes %q revision %q but parent module %q does not include the same revision at %s", submodule.Argument, inc.Argument, revision, parent.name, inc.Location())
	}
	return fmt.Errorf("YANG 1.1 submodule %q includes %q but parent module %q does not include it at %s", submodule.Argument, inc.Argument, parent.name, inc.Location())
}

func parentDirectlyIncludesSubmoduleRevision(parent *moduleData, name, revision string) bool {
	if parent == nil || parent.stmt == nil {
		return false
	}
	for _, inc := range direct(parent.stmt, "include") {
		if inc.Argument != name {
			continue
		}
		parentRevision, err := statementRevisionDate(inc)
		if err != nil {
			continue
		}
		if revision == "" || parentRevision == revision {
			return true
		}
	}
	return false
}

func parentDirectlyIncludesSubmoduleName(parent *moduleData, name string) bool {
	if parent == nil || parent.stmt == nil {
		return false
	}
	for _, inc := range direct(parent.stmt, "include") {
		if inc.Argument == name {
			return true
		}
	}
	return false
}

func validateSubmoduleIncludeStatements(stmt *yangparse.Statement, allowXMLPrefix bool) error {
	includes := direct(stmt, "include")
	if err := validateDuplicateIncludes("submodule "+strconv.Quote(stmt.Argument), includes); err != nil {
		return err
	}
	for _, inc := range includes {
		if _, err := validateIncludeStatement(inc, allowXMLPrefix); err != nil {
			return err
		}
	}
	return nil
}

func validateDuplicateIncludes(scope string, includes []*yangparse.Statement) error {
	seen := make(map[string]*yangparse.Statement, len(includes))
	for _, inc := range includes {
		if prev := seen[inc.Argument]; prev != nil {
			return fmt.Errorf("duplicate include %q in %s at %s; previous include at %s", inc.Argument, scope, inc.Location(), prev.Location())
		}
		seen[inc.Argument] = inc
	}
	return nil
}

func validateIncludeStatement(inc *yangparse.Statement, allowXMLPrefix bool) (string, error) {
	if err := validateYangIdentifierArg("include", inc.Argument, inc, allowXMLPrefix); err != nil {
		return "", err
	}
	if err := validateDependencyStatementChildren(inc); err != nil {
		return "", err
	}
	revision, err := statementRevisionDate(inc)
	if err != nil {
		return "", err
	}
	return revision, nil
}

func (c *Context) loadIncludedSubmodulePath(path string, parent *moduleData) error {
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	raw, err := yangparse.ReadFile(abs)
	if err != nil {
		return err
	}
	stmts, err := yangparse.Parse(raw, abs)
	if err != nil {
		return err
	}
	if len(stmts) != 1 {
		return fmt.Errorf("%s: expected one submodule statement, got %d", abs, len(stmts))
	}
	stmt := stmts[0]
	if stmt.Keyword != "submodule" {
		return fmt.Errorf("%s: included file top-level statement is %q, want submodule", abs, stmt.Keyword)
	}
	_, err = c.loadSubmodule(abs, stmt, parent)
	return err
}

func (c *Context) loadImports(mod *moduleData) error {
	mod.imports = mod.imports[:0]
	mod.importByPfx = make(map[string]*moduleData)
	mod.sourceImportByPfx = make(map[*yangparse.Statement]map[string]*moduleData)
	for _, scope := range mod.importScopes() {
		seenPrefixes := make(map[string]importPrefixSeen)
		scopeImports := make(map[string]*moduleData)
		mod.sourceImportByPfx[scope.root] = scopeImports
		for _, impStmt := range scope.imports {
			allowXMLPrefix := sourceRootYangVersion(scope.root) == "1.1"
			if err := validateYangIdentifierArg("import", impStmt.Argument, impStmt, allowXMLPrefix); err != nil {
				return err
			}
			if err := validateDependencyStatementChildren(impStmt); err != nil {
				return err
			}
			prefixStmt, err := singletonChild(impStmt, "prefix")
			if err != nil {
				return err
			}
			pfx := ""
			if prefixStmt != nil {
				pfx = prefixStmt.Argument
			}
			if pfx == "" {
				return fmt.Errorf("import %q in %s has no prefix", impStmt.Argument, scope.label)
			}
			if err := validateYangIdentifierArg("prefix", pfx, prefixStmt, allowXMLPrefix); err != nil {
				return err
			}
			if pfx == scope.localPrefix {
				return fmt.Errorf("import prefix %q in %s collides with %s prefix", pfx, scope.label, scope.localPrefixKind)
			}
			if pfx == mod.name {
				return fmt.Errorf("import prefix %q in %s collides with module name", pfx, scope.label)
			}
			targetRevision, err := statementRevisionDate(impStmt)
			if err != nil {
				return err
			}
			if prev := seenPrefixes[pfx]; prev.name != "" {
				if c.validationMode == ValidationVendorCompatible && prev.name == impStmt.Argument && prev.revision == targetRevision {
					mod.recordVendorCompatibleWarning(
						impStmt,
						[]*yangparse.Statement{prev.stmt},
						"duplicate import prefix %q in %s for equivalent import %q revision %q at %s; ignored in vendor-compatible mode",
						pfx,
						scope.label,
						impStmt.Argument,
						targetRevision,
						impStmt.Location(),
					)
					continue
				}
				return fmt.Errorf("duplicate import prefix %q in %s for imports %q at %s and %q at %s", pfx, scope.label, prev.name, prev.stmt.Location(), impStmt.Argument, impStmt.Location())
			}
			seenPrefixes[pfx] = importPrefixSeen{name: impStmt.Argument, revision: targetRevision, stmt: impStmt}
			if impStmt.Argument == mod.name {
				return fmt.Errorf("module %q imports itself at %s", mod.name, impStmt.Location())
			}
			mod.imports = append(mod.imports, Import{
				Prefix:      pfx,
				Name:        impStmt.Argument,
				Revision:    targetRevision,
				description: childArg(impStmt, "description"),
				reference:   childArg(impStmt, "reference"),
			})
			targetName := impStmt.Argument
			target := c.modulesByKey[moduleKey(targetName, targetRevision)]
			if target == nil && targetRevision == "" {
				target = c.modules[targetName]
			}
			if target == nil || target.stmt == nil || targetRevision != "" && target.revision != targetRevision {
				path, err := c.findModulePathRevision(targetName, targetRevision)
				if err != nil {
					return err
				}
				ok, err := moduleFileDeclaresModule(path, targetName)
				if err != nil {
					return err
				}
				if !ok {
					return fmt.Errorf("YANG file %q does not declare module %q", path, targetName)
				}
				var loadErr error
				target, loadErr = c.loadModulePath(path, c.allImplemented, false)
				if loadErr != nil {
					return loadErr
				}
			}
			if c.allImplemented && target != nil && !target.implemented {
				c.markImplemented(target)
				c.dirty = true
			}
			scopeImports[pfx] = target
			if _, ok := mod.importByPfx[pfx]; !ok {
				mod.importByPfx[pfx] = target
			}
		}
	}
	return nil
}

func (c *Context) markImplemented(mod *moduleData) {
	if mod == nil || mod.implemented {
		return
	}
	mod.implemented = true
	c.registerNameDefault(mod)
}

// Schema returns the loaded module's schema IR.
func (c *Context) Schema(module string) (Module, error) {
	if err := c.rebuildIfDirty(); err != nil {
		return Module{}, wrap("schema tree", err)
	}
	mod := c.modules[module]
	if mod == nil {
		mod = c.modulesByNS[module]
	}
	if mod == nil || mod.stmt == nil {
		return Module{}, wrap("schema tree", fmt.Errorf("module %q not loaded", module))
	}
	return Module{mod: mod}, nil
}

// SchemaRevision returns the loaded module revision's schema IR. An empty
// revision looks up a module that declares no revision; use Schema for the
// context's latest/default same-name module view.
func (c *Context) SchemaRevision(module, revision string) (Module, error) {
	if err := c.rebuildIfDirty(); err != nil {
		return Module{}, wrap("schema tree", err)
	}
	mod := c.modulesByKey[moduleKey(module, revision)]
	if mod == nil {
		mod = c.modulesByNSKey[moduleKey(module, revision)]
	}
	if mod == nil || mod.stmt == nil {
		if revision == "" {
			return Module{}, wrap("schema tree", fmt.Errorf("module %q with no revision not loaded", module))
		}
		return Module{}, wrap("schema tree", fmt.Errorf("module %q revision %q not loaded", module, revision))
	}
	return Module{mod: mod}, nil
}

// GetModule returns a loaded module by name. A nil revision selects the
// context's latest/default same-name module; a non-nil revision selects that
// exact loaded revision.
func (c *Context) GetModule(name string, revision *string) (Module, bool) {
	if err := c.rebuildIfDirty(); err != nil {
		return Module{}, false
	}
	var mod *moduleData
	if revision == nil {
		mod = c.modules[name]
	} else {
		mod = c.modulesByKey[moduleKey(name, revisionString(revision))]
	}
	if mod == nil || mod.stmt == nil {
		return Module{}, false
	}
	return Module{mod: mod}, true
}

// FindModuleByNamespace returns the context's latest/default loaded module for a
// namespace.
func (c *Context) FindModuleByNamespace(namespace string) (Module, bool) {
	if err := c.rebuildIfDirty(); err != nil {
		return Module{}, false
	}
	mod := c.modulesByNS[namespace]
	if mod == nil || mod.stmt == nil {
		return Module{}, false
	}
	return Module{mod: mod}, true
}

// FindModuleByNS returns the context's latest/default loaded module for a
// namespace.
//
// Deprecated: use FindModuleByNamespace.
func (c *Context) FindModuleByNS(namespace string) (Module, bool) {
	return c.FindModuleByNamespace(namespace)
}

// Modules returns a stable snapshot of modules known to the context.
func (c *Context) Modules() []Module {
	if err := c.rebuildIfDirty(); err != nil {
		return nil
	}
	out := make([]Module, 0, len(c.loadOrder))
	for _, mod := range c.loadOrder {
		if mod.stmt != nil && mod.implemented {
			out = append(out, Module{mod: mod})
		}
	}
	return out
}

func (c *Context) rebuildIfDirty() error {
	if c == nil || c.closed {
		return fmt.Errorf("context is closed")
	}
	if !c.dirty {
		return nil
	}
	for _, mod := range c.loadOrder {
		mod.resetIR()
		if err := mod.collectDefinitions(); err != nil {
			return err
		}
	}
	for _, mod := range c.loadOrder {
		if err := mod.validateExtensionInstances(); err != nil {
			return err
		}
	}
	if err := c.validateEnabledFeatures(); err != nil {
		return err
	}
	for _, mod := range c.loadOrder {
		mod.validateIfFeatureExpressions()
	}
	if err := c.firstSchemaError(); err != nil {
		return err
	}
	for _, mod := range c.loadOrder {
		if mod.stmt != nil {
			mod.buildIR()
		}
	}
	if err := c.firstSchemaError(); err != nil {
		return err
	}
	for _, mod := range c.loadOrder {
		for _, top := range mod.sourceTopStatements() {
			if err := mod.validateYangVersionSpecificStatements(top); err != nil {
				return err
			}
		}
	}
	for _, mod := range c.loadOrder {
		mod.applyAugments()
	}
	for _, mod := range c.loadOrder {
		mod.collectDeviations()
	}
	for _, mod := range c.loadOrder {
		mod.buildIndexes()
	}
	for _, mod := range c.loadOrder {
		mod.validateSiblingNames()
	}
	for _, mod := range c.loadOrder {
		mod.validateListConstraints()
	}
	for _, mod := range c.loadOrder {
		mod.validateDefaultRules()
	}
	if err := c.firstSchemaError(); err != nil {
		return err
	}
	for _, mod := range c.loadOrder {
		if err := mod.parseTypes(); err != nil {
			return err
		}
	}
	for _, mod := range c.loadOrder {
		if err := mod.validateGroupingBodyTypes(); err != nil {
			return err
		}
	}
	for _, mod := range c.loadOrder {
		if err := mod.validateTypedefTypes(); err != nil {
			return err
		}
	}
	for _, mod := range c.loadOrder {
		if err := mod.validateTypedefDefaultValues(); err != nil {
			return err
		}
	}
	for _, mod := range c.loadOrder {
		if err := mod.validateListConstraintTypes(); err != nil {
			return err
		}
	}
	for _, mod := range c.loadOrder {
		mod.resolveLeafRefs()
	}
	for _, mod := range c.loadOrder {
		if err := mod.validateDefaultValues(); err != nil {
			return err
		}
	}
	for _, mod := range c.loadOrder {
		mod.resolveIdentities()
	}
	if err := c.firstSchemaError(); err != nil {
		return err
	}
	for _, mod := range c.loadOrder {
		mod.applyRefImplementedPolicy()
	}
	c.dirty = false
	return nil
}

func (c *Context) validateEnabledFeatures() error {
	if c == nil {
		return nil
	}
	for moduleName, features := range c.enabledFeatures {
		mod := c.modules[moduleName]
		if mod == nil || mod.stmt == nil {
			continue
		}
		for feature := range features {
			if mod.featureMap[feature] == nil {
				return fmt.Errorf("unknown feature %q for module %q", feature, moduleName)
			}
		}
	}
	return nil
}

func (c *Context) firstSchemaError() error {
	for _, mod := range c.loadOrder {
		if mod.schemaErr != nil {
			return mod.schemaErr
		}
	}
	return nil
}
