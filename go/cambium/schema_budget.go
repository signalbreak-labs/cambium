// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

package cambium

import (
	"fmt"

	"github.com/signalbreak-labs/cambium/go/internal/yangparse"
)

// DefaultMaxSchemaNodes is the schema node budget Build uses unless
// ContextBuilder.SetMaxSchemaNodes selects another. The largest schemas
// measured stay well below it: the full Junos configuration schema (35
// modules, 1.1 million unique nodes) needs about 3.3 million instantiations
// and the OpenConfig release about 60,000. At roughly 1 KB per retained node
// the default still allows several GB, so services loading untrusted YANG
// should set a lower limit.
const DefaultMaxSchemaNodes uint64 = 1 << 23

// buildState is the context state that lives for one schema rebuild.
type buildState struct {
	// maxSchemaNodes is the caller-selected budget; 0 selects
	// DefaultMaxSchemaNodes.
	maxSchemaNodes uint64
	// schemaNodes counts the schema nodes the current rebuild instantiated.
	schemaNodes uint64
	// limitErr is set once the budget is exhausted. No schema node is
	// expanded after that, and it becomes the rebuild's error.
	limitErr error

	// The memos below hold results that do not depend on where a definition
	// is referenced from, so each definition in a chain is resolved once per
	// rebuild instead of once per reference. They are nil outside a rebuild;
	// lookups then miss and nothing is stored, so a frozen context is never
	// written by readers.

	// typedefTypes holds each successfully resolved typedef's type, keyed by
	// the typedef statement and its module. Entries are handed out as copies.
	typedefTypes map[typedefKey]TypeInfo
	// validFeatures holds features whose if-feature expressions resolved.
	validFeatures map[*featureData]bool
	// featureEnabled holds each evaluated enabled feature's if-feature result.
	featureEnabled map[*featureData]bool
}

type typedefKey struct {
	module *moduleData
	stmt   *yangparse.Statement
}

// SetMaxSchemaNodes limits how many schema nodes Build may instantiate. Each
// schema node statement counts once per instantiation: once per uses of its
// grouping, once per augment, and once more for the standalone check of each
// grouping body. Build fails with a DiagnosticResourceLimit error as soon as
// the limit is exceeded, before the expansion completes. 0 selects
// DefaultMaxSchemaNodes.
func (b *ContextBuilder) SetMaxSchemaNodes(limit uint64) error {
	if err := b.ensureMutable(); err != nil {
		return err
	}
	b.ctx.build.maxSchemaNodes = limit
	return nil
}

// beginRebuild resets the per-rebuild budget and memos.
func (c *Context) beginRebuild() {
	c.build.schemaNodes = 0
	c.build.limitErr = nil
	c.build.typedefTypes = make(map[typedefKey]TypeInfo)
	c.build.validFeatures = make(map[*featureData]bool)
	c.build.featureEnabled = make(map[*featureData]bool)
}

// endRebuild drops the memos and makes an exhausted budget the rebuild's
// error: once expansion stopped, any other error may be an artifact of the
// truncated schema. The context stays dirty so a later access does not see
// the truncated schema.
func (c *Context) endRebuild(err error) error {
	c.build.typedefTypes = nil
	c.build.validFeatures = nil
	c.build.featureEnabled = nil
	if c.build.limitErr != nil {
		c.dirty = true
		return c.build.limitErr
	}
	return err
}

// admitSchemaNode charges one schema node for st against the rebuild budget.
// It reports false once the budget is exhausted, recording the
// resource-limit error on the first refusal; callers then stop expanding.
func (m *moduleData) admitSchemaNode(st *yangparse.Statement) bool {
	if m == nil || m.ctx == nil {
		return true
	}
	budget := &m.ctx.build
	if budget.limitErr != nil {
		return false
	}
	limit := budget.maxSchemaNodes
	if limit == 0 {
		limit = DefaultMaxSchemaNodes
	}
	if budget.schemaNodes < limit {
		budget.schemaNodes++
		return true
	}
	budget.limitErr = &DiagnosticError{
		Kind:   DiagnosticResourceLimit,
		Module: m.name,
		Source: sourceLocation(st),
		Err: fmt.Errorf("schema exceeds the limit of %d schema nodes at %s: grouping and augment expansion instantiates too many nodes (raise the limit with ContextBuilder.SetMaxSchemaNodes)",
			limit, st.Location()),
	}
	m.recordSchemaError(budget.limitErr)
	return false
}
