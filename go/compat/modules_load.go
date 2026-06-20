package compat

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/signalbreak-labs/cambium/go/cambium"
	"github.com/signalbreak-labs/cambium/go/internal/yangparse"
)

func findInDir(dir, name string, recurse bool) (string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}

	var dirs []string
	var revisions []string
	moduleName := strings.TrimSuffix(name, ".yang")
	for _, entry := range entries {
		entryName := entry.Name()
		path := filepath.Join(dir, entryName)
		if entry.IsDir() {
			if recurse {
				dirs = append(dirs, path)
			}
			continue
		}
		if entryName == name {
			return path, nil
		}
		if strings.HasPrefix(entryName, moduleName) &&
			revisionDateSuffixRegex.MatchString(strings.TrimPrefix(entryName, moduleName)) {
			revisions = append(revisions, entryName)
		}
	}
	if len(revisions) != 0 {
		sort.Strings(revisions)
		return filepath.Join(dir, revisions[len(revisions)-1]), nil
	}
	for _, dir := range dirs {
		path, err := findInDir(dir, name, true)
		if err != nil || path != "" {
			return path, err
		}
	}
	return "", nil
}

// Parse parses YANG source and registers module/submodule names.
func (ms *Modules) Parse(data, name string) error {
	if ms == nil {
		return fmt.Errorf("nil Modules")
	}
	stmts, err := yangparse.Parse(data, name)
	if err != nil {
		return err
	}
	for _, stmt := range stmts {
		switch stmt.Keyword {
		case "module":
			decl := moduleDeclFromStatement(stmt, name, false)
			if err := ms.duplicateModuleDeclError(decl); err != nil {
				return err
			}
			ms.addModuleDecl(decl)
		case "submodule":
			decl := moduleDeclFromStatement(stmt, name, true)
			if err := ms.duplicateModuleDeclError(decl); err != nil {
				return err
			}
			ms.addModuleDecl(decl)
		default:
			return fmt.Errorf("not a module or submodule: %s is of type %s", stmt.Argument, stmt.Keyword)
		}
	}
	ms.sources = append(ms.sources, moduleSource{data: data, name: name})
	ms.closeContext()
	ms.resetNamespaceCache()
	ms.built = false
	return nil
}

// Read reads a YANG source file or module name.
func (ms *Modules) Read(name string) error {
	if ms == nil {
		return fmt.Errorf("nil Modules")
	}
	path, err := ms.findFile(name)
	if err != nil {
		return err
	}
	data, err := yangparse.ReadFile(path)
	if err != nil {
		return err
	}
	if err := ms.Parse(data, path); err != nil {
		return err
	}
	ms.sources[len(ms.sources)-1].path = path
	return nil
}

// GetModule returns the Entry projection for module name.
func (ms *Modules) GetModule(name string) (*Entry, []error) {
	if ms == nil {
		return nil, []error{fmt.Errorf("nil Modules")}
	}
	if ms.Modules[name] == nil {
		if err := ms.Read(name); err != nil {
			return nil, []error{err}
		}
		if ms.Modules[name] == nil {
			return nil, []error{fmt.Errorf("module not found: %s", name)}
		}
	}
	if errs := ms.Process(); len(errs) != 0 {
		return nil, errs
	}
	if record := ms.Modules[name]; record != nil {
		if entry, ok := moduleEntry(record); ok {
			return entry, nil
		}
	}
	if ms.ctx != nil {
		mod, err := ms.ctx.Schema(name)
		if err == nil {
			record := ms.recordModule(mod)
			if entry, ok := moduleEntry(record); ok {
				return entry, nil
			}
		}
	}
	return nil, []error{fmt.Errorf("module not found: %s", name)}
}

// FindModule returns the module or submodule referenced by a goyang-style import or include node.
func (ms *Modules) FindModule(n Node) *Module {
	if ms == nil || n == nil {
		return nil
	}
	name := n.NName()
	if strings.TrimSpace(name) == "" {
		return nil
	}

	var (
		target       map[string]*Module
		revisionDate string
	)
	switch node := n.(type) {
	case *Include:
		target = ms.SubModules
		if node.RevisionDate != nil {
			revisionDate = node.RevisionDate.Name
		}
	case *Import:
		target = ms.Modules
		if node.RevisionDate != nil {
			revisionDate = node.RevisionDate.Name
		}
	default:
		return nil
	}

	if revisionDate != "" {
		fullName := name + "@" + revisionDate
		if record := target[fullName]; record != nil {
			return record
		}
		if err := ms.Read(fullName); err == nil {
			if record := target[fullName]; record != nil {
				return record
			}
		}
		if record := target[name]; record != nil {
			return record
		}
		if err := ms.Read(name); err != nil {
			return nil
		}
		if record := target[fullName]; record != nil {
			return record
		}
		return target[name]
	}

	if record := target[name]; record != nil {
		return record
	}
	if err := ms.Read(name); err != nil {
		return nil
	}
	return target[name]
}

// FindModuleByNamespace returns the module record with the specified namespace.
func (ms *Modules) FindModuleByNamespace(ns string) (*Module, error) {
	if ms == nil {
		return nil, fmt.Errorf("nil Modules")
	}

	ms.nsMu.Lock()
	if record := ms.byNS[ns]; record != nil {
		ms.nsMu.Unlock()
		return record, nil
	}
	ms.nsMu.Unlock()

	if found, err := ms.findRecordedModuleByNamespace(ns); err != nil {
		return nil, err
	} else if found != nil {
		ms.cacheNamespace(ns, found)
		return found, nil
	}

	if ms.ctx != nil {
		if mod, ok := ms.ctx.FindModuleByNS(ns); ok {
			found := ms.recordModule(mod)
			ms.cacheNamespace(ns, found)
			return found, nil
		}
	}
	return nil, fmt.Errorf("%q: no such namespace", ns)
}

func (ms *Modules) referencesInclude(name string) bool {
	for _, record := range ms.moduleRecords() {
		for _, include := range record.Include {
			if include != nil && include.Name == name {
				return true
			}
		}
	}
	return false
}

func (ms *Modules) referencesImport(name string) bool {
	for _, record := range ms.moduleRecords() {
		for _, imp := range record.Import {
			if imp != nil && imp.Name == name {
				return true
			}
		}
	}
	return false
}

// GetModule reads optional sources and returns the named module Entry.
func GetModule(name string, sources ...string) (*Entry, []error) {
	ms := NewModules()
	var errs []error
	for _, source := range sources {
		if err := ms.Read(source); err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) != 0 {
		return nil, errs
	}
	return ms.GetModule(name)
}

func (ms *Modules) findRecordedModuleByNamespace(ns string) (*Module, error) {
	var found *Module
	for _, record := range ms.Modules {
		if record == nil || record.Namespace == nil || record.Namespace.Name != ns {
			continue
		}
		switch found {
		case nil:
			found = record
		case record:
		default:
			return nil, fmt.Errorf("namespace %s matches two or more modules (%s, %s)", ns, found.Name, record.Name)
		}
	}
	return found, nil
}

func importFromStatement(stmt *Statement) *Import {
	if stmt == nil {
		return nil
	}
	imp := &Import{
		Name:       stmt.Argument,
		Source:     stmt,
		Extensions: extensionStatements(stmt),
	}
	imp.Prefix = astValueChild(stmt, "prefix", imp)
	imp.RevisionDate = astValueChild(stmt, "revision-date", imp)
	imp.Description = astValueChild(stmt, "description", imp)
	imp.Reference = astValueChild(stmt, "reference", imp)
	return imp
}

func includeFromStatement(stmt *Statement) *Include {
	if stmt == nil {
		return nil
	}
	inc := &Include{
		Name:       stmt.Argument,
		Source:     stmt,
		Extensions: extensionStatements(stmt),
	}
	inc.RevisionDate = astValueChild(stmt, "revision-date", inc)
	return inc
}

func importsFromCambium(imports []cambium.Import) []*Import {
	if len(imports) == 0 {
		return nil
	}
	out := make([]*Import, 0, len(imports))
	for _, imp := range imports {
		out = append(out, &Import{
			Name:         imp.Name,
			Prefix:       astValueOrNil(imp.Prefix),
			RevisionDate: astValueOrNil(imp.Revision),
			Description:  astValueFromOptional(imp.Description()),
			Reference:    astValueFromOptional(imp.Reference()),
		})
	}
	return out
}

func includesFromCambium(includes []cambium.Include) []*Include {
	if len(includes) == 0 {
		return nil
	}
	out := make([]*Include, 0, len(includes))
	for _, inc := range includes {
		out = append(out, &Include{
			Name:         inc.Name,
			RevisionDate: astValueOrNil(inc.Revision),
		})
	}
	return out
}

func cloneImports(in []*Import, parent Node) []*Import {
	if len(in) == 0 {
		return nil
	}
	out := make([]*Import, 0, len(in))
	for _, imp := range in {
		if imp == nil {
			continue
		}
		cloned := &Import{
			Name:         imp.Name,
			Source:       imp.Source,
			Parent:       parent,
			Extensions:   append([]*Statement(nil), imp.Extensions...),
			Module:       imp.Module,
			Prefix:       cloneASTValueWithParent(imp.Prefix, nil),
			RevisionDate: cloneASTValueWithParent(imp.RevisionDate, nil),
			Description:  cloneASTValueWithParent(imp.Description, nil),
			Reference:    cloneASTValueWithParent(imp.Reference, nil),
		}
		setASTValueParent(cloned.Prefix, cloned)
		setASTValueParent(cloned.RevisionDate, cloned)
		setASTValueParent(cloned.Description, cloned)
		setASTValueParent(cloned.Reference, cloned)
		out = append(out, cloned)
	}
	return out
}

func cloneIncludes(in []*Include, parent Node) []*Include {
	if len(in) == 0 {
		return nil
	}
	out := make([]*Include, 0, len(in))
	for _, inc := range in {
		if inc == nil {
			continue
		}
		cloned := &Include{
			Name:         inc.Name,
			Source:       inc.Source,
			Parent:       parent,
			Extensions:   append([]*Statement(nil), inc.Extensions...),
			Module:       inc.Module,
			RevisionDate: cloneASTValueWithParent(inc.RevisionDate, nil),
		}
		setASTValueParent(cloned.RevisionDate, cloned)
		out = append(out, cloned)
	}
	return out
}

func (ms *Modules) findFile(name string) (string, error) {
	lookup := name
	hasSeparator := strings.ContainsRune(name, os.PathSeparator)
	if !hasSeparator && !strings.HasSuffix(lookup, ".yang") {
		lookup += ".yang"
		if path, err := findInDir(".", lookup, false); err != nil {
			return "", err
		} else if path != "" {
			lookup = path
		}
	}
	if info, err := os.Stat(lookup); err == nil {
		if !info.IsDir() {
			ms.AddPath(filepath.Dir(lookup))
			return lookup, nil
		}
	} else if !os.IsNotExist(err) {
		return "", err
	}
	if hasSeparator {
		return "", fmt.Errorf("no such file: %s", lookup)
	}

	for _, dir := range ms.Path {
		clean := filepath.Clean(dir)
		recurse := filepath.Base(clean) == "..."
		if recurse {
			clean = filepath.Dir(clean)
		}
		path, err := findInDir(clean, lookup, recurse)
		if err != nil {
			return "", err
		}
		if path != "" {
			return path, nil
		}
	}
	return "", fmt.Errorf("no such file: %s", lookup)
}
