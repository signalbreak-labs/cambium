// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

package cambium

import (
	"fmt"
	"maps"
	"path/filepath"

	"github.com/signalbreak-labs/cambium/go/internal/yangparse"
)

type sourceIdentity struct {
	kind, name, revision string
}

// RegisterSourcePaths catalogs YANG files by declared module/submodule name and
// newest declared revision, independently of their filenames. Register all
// explicit paths before loading any modules. Registration does not load,
// implement, or semantically compile the sources. Dependencies use registered
// sources first, then the ordinary search paths; an unpinned lookup selects the
// newest registered revision. Keep source files unchanged until Build completes.
// The batch is atomic; re-registering the same path is harmless, but conflicting
// files declaring one kind/name/revision are rejected.
func (b *ContextBuilder) RegisterSourcePaths(paths ...string) error {
	if err := b.ensureMutable(); err != nil {
		return err
	}
	if len(b.ctx.loadOrder) != 0 {
		return wrap("context builder", fmt.Errorf("register source paths before loading modules"))
	}
	catalog := maps.Clone(b.ctx.sourceCatalog)
	if catalog == nil {
		catalog = make(map[sourceIdentity]string)
	}
	for _, path := range paths {
		abs, err := filepath.Abs(path)
		if err != nil {
			return wrap("context builder", err)
		}
		identity, err := readSourceIdentity(abs, b.ctx.validationMode)
		if err != nil {
			return wrap("context builder", err)
		}
		if existing := catalog[identity]; existing != "" && existing != abs && !sameResolvedSourcePath(existing, abs) {
			return wrap("context builder", fmt.Errorf("conflicting registered %s %q revision %q: %s and %s", identity.kind, identity.name, identity.revision, existing, abs))
		}
		catalog[identity] = abs
	}
	b.ctx.sourceCatalog = catalog
	return nil
}

func readSourceIdentity(path string, mode ValidationMode) (sourceIdentity, error) {
	raw, err := yangparse.ReadFile(path)
	if err != nil {
		return sourceIdentity{}, err
	}
	statements, err := yangparse.Parse(raw, path)
	if err != nil {
		return sourceIdentity{}, err
	}
	if len(statements) != 1 || statements[0].Keyword != "module" && statements[0].Keyword != "submodule" {
		return sourceIdentity{}, fmt.Errorf("%s: expected one module or submodule declaration", path)
	}
	stmt := statements[0]
	if err := validateYangVersion(stmt); err != nil {
		return sourceIdentity{}, err
	}
	if err := validateYangIdentifierArg(stmt.Keyword, stmt.Argument, stmt, sourceRootYangVersion(stmt) == "1.1"); err != nil {
		return sourceIdentity{}, err
	}
	revision, _, err := moduleRevisionValidatedMode(stmt, mode)
	if err != nil {
		return sourceIdentity{}, err
	}
	return sourceIdentity{kind: stmt.Keyword, name: stmt.Argument, revision: revision}, nil
}

func (c *Context) registeredSource(kind, name, revision string) (selected sourceIdentity, path string) {
	if revision != "" {
		identity := sourceIdentity{kind: kind, name: name, revision: revision}
		return identity, c.sourceCatalog[identity]
	}
	for identity, candidate := range c.sourceCatalog {
		if identity.kind == kind && identity.name == name && (path == "" || identity.revision > selected.revision) {
			selected, path = identity, candidate
		}
	}
	return selected, path
}
