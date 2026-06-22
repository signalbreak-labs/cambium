// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

package cambium

import (
	"fmt"
	"strings"
)

// ProjectionRole is a bitmask describing why a projected schema node is present.
type ProjectionRole uint8

const (
	// ProjectionRoleAncestor marks nodes included to connect a selected target
	// back to a top-level data node.
	ProjectionRoleAncestor ProjectionRole = 1 << iota
	// ProjectionRoleSelected marks nodes that directly matched an input path.
	ProjectionRoleSelected
	// ProjectionRoleDescendant marks nodes included by a selected subtree walk.
	ProjectionRoleDescendant
	// ProjectionRoleKey marks list keys retained for payload construction.
	ProjectionRoleKey
	// ProjectionRoleMandatory marks nodes required by schema semantics such as
	// mandatory true or positive min-elements.
	ProjectionRoleMandatory
	// ProjectionRoleDefault marks nodes carrying schema defaults when defaults
	// are requested in the projection.
	ProjectionRoleDefault
)

// Has reports whether role contains flag.
func (r ProjectionRole) Has(flag ProjectionRole) bool { return r&flag != 0 }

// ProjectionOptions controls ordered schema projection.
type ProjectionOptions struct {
	// IncludeDescendants walks downward from each selected target.
	IncludeDescendants bool
	// IncludeListKeys retains list keys under every included list node.
	IncludeListKeys bool
	// IncludeMandatory retains mandatory descendants under each selected target.
	IncludeMandatory bool
	// ProtectMandatory keeps mandatory descendants even when allow/ignore filters
	// would otherwise remove them. It applies to nodes included by
	// IncludeMandatory or IncludeDescendants.
	ProtectMandatory bool
	// IncludeDefaults retains defaulted descendants under each selected target.
	IncludeDefaults bool
	// ListKeysFirst orders retained key children before non-key list children.
	ListKeysFirst bool
	// FlattenChoices omits schema-only choice/case nodes and splices their data
	// descendants into the projected payload shape.
	FlattenChoices bool

	// AllowRelativePaths keeps only these relative subtrees below each selected
	// target, plus required ancestors and retained list keys. Paths are schema
	// paths, not runtime XPath predicates.
	AllowRelativePaths []string
	// IgnoreRelativePaths prunes these relative subtrees below each selected
	// target. Ignore wins over allow; retained list keys are protected.
	IgnoreRelativePaths []string

	// IgnoreInvalidFilters skips unresolved allow/ignore paths. The default is
	// strict: invalid filters are returned as errors.
	IgnoreInvalidFilters bool
}

// DefaultProjectionOptions returns the payload-oriented defaults most callers
// want for generation: full selected subtrees, list keys retained/key-first, and
// choice/case flattened to data nodes.
func DefaultProjectionOptions() ProjectionOptions {
	return ProjectionOptions{
		IncludeDescendants: true,
		IncludeListKeys:    true,
		ListKeysFirst:      true,
		FlattenChoices:     true,
	}
}

// Projection is an ordered schema projection rooted at top-level data nodes.
type Projection struct {
	Roots []ProjectedNode
}

// WalkPreOrder visits projected nodes in emitted order until fn returns false.
func (p Projection) WalkPreOrder(fn func(ProjectedNode) bool) {
	for _, root := range p.Roots {
		if !root.WalkPreOrder(fn) {
			return
		}
	}
}

// ProjectedNode is one node in an ordered schema projection.
type ProjectedNode struct {
	Node     SchemaNodeRef
	Role     ProjectionRole
	Children []ProjectedNode
}

// WalkPreOrder visits this node and descendants in emitted order until fn
// returns false. It returns false when traversal was stopped early.
func (n ProjectedNode) WalkPreOrder(fn func(ProjectedNode) bool) bool {
	if !fn(n) {
		return false
	}
	for _, child := range n.Children {
		if !child.WalkPreOrder(fn) {
			return false
		}
	}
	return true
}

// DataParent returns this node's nearest data-node parent, skipping schema-only
// choice/case nodes.
func (n SchemaNodeRef) DataParent() (SchemaNodeRef, bool) {
	if n.node == nil {
		return SchemaNodeRef{}, false
	}
	for p := n.node.parent; p != nil && p.kind != SchemaNodeKindModule; p = p.parent {
		ref := SchemaNodeRef{node: p}
		if ref.IsDataNode() {
			return ref, true
		}
	}
	return SchemaNodeRef{}, false
}

// DataAncestors returns data-node ancestors from root to parent, skipping
// schema-only choice/case nodes.
func (n SchemaNodeRef) DataAncestors() []SchemaNodeRef {
	if n.node == nil {
		return nil
	}
	var rev []SchemaNodeRef
	for p := n.node.parent; p != nil && p.kind != SchemaNodeKindModule; p = p.parent {
		ref := SchemaNodeRef{node: p}
		if ref.IsDataNode() {
			rev = append(rev, ref)
		}
	}
	out := make([]SchemaNodeRef, len(rev))
	for i := range rev {
		out[len(rev)-1-i] = rev[i]
	}
	return out
}

// IsDataNode reports whether this schema node can appear in a data payload.
func (n SchemaNodeRef) IsDataNode() bool {
	switch n.Kind() {
	case SchemaNodeKindContainer, SchemaNodeKindLeaf, SchemaNodeKindLeafList, SchemaNodeKindList, SchemaNodeKindAnyData:
		return true
	default:
		return false
	}
}

// ProjectSchemaPaths resolves absolute schema paths and returns a merged,
// ordered projection. Final ordering is driven by the effective schema tree, not
// input path order.
func ProjectSchemaPaths(mod Module, paths []string, opts ProjectionOptions) (Projection, error) {
	if mod.mod == nil {
		return Projection{}, wrap("schema projection", fmt.Errorf("nil module"))
	}
	builder := newProjectionBuilder(mod, opts)
	for _, path := range paths {
		target, err := mod.FindPath(path)
		if err != nil {
			return Projection{}, wrap("schema projection", fmt.Errorf("select %q: %w", path, err))
		}
		if !target.IsDataNode() {
			return Projection{}, wrap("schema projection", fmt.Errorf("selected path %q is not a data node", path))
		}
		filters, err := projectionFiltersFor(target, opts)
		if err != nil {
			return Projection{}, wrap("schema projection", err)
		}
		builder.addSelection(target, filters)
	}
	return builder.projection(), nil
}

// ProjectSubtree returns one ordered projected subtree rooted at node.
func ProjectSubtree(node SchemaNodeRef, opts ProjectionOptions) (ProjectedNode, error) {
	if node.node == nil {
		return ProjectedNode{}, wrap("schema projection", fmt.Errorf("nil schema node"))
	}
	if !node.IsDataNode() {
		return ProjectedNode{}, wrap("schema projection", fmt.Errorf("selected node %q is not a data node", node.Name()))
	}
	filters, err := projectionFiltersFor(node, opts)
	if err != nil {
		return ProjectedNode{}, wrap("schema projection", err)
	}
	root := newProjectionBuildNode(node, projectionSelectedRole(node, opts))
	addProjectionListKeys(root, opts)
	includeProjectionDescendants(root, node, filters, opts)
	return root.project(opts), nil
}

type projectionBuilder struct {
	module    Module
	opts      ProjectionOptions
	roots     map[SchemaNodeRef]*projectionBuildNode
	rootOrder []SchemaNodeRef
}

func newProjectionBuilder(module Module, opts ProjectionOptions) *projectionBuilder {
	return &projectionBuilder{
		module: module,
		opts:   opts,
		roots:  make(map[SchemaNodeRef]*projectionBuildNode),
	}
}

func (b *projectionBuilder) addSelection(target SchemaNodeRef, filters projectionFilters) {
	ancestors := target.DataAncestors()
	if !b.opts.FlattenChoices {
		ancestors = target.Ancestors()
	}
	var parent *projectionBuildNode
	for _, ancestor := range ancestors {
		if !ancestor.IsDataNode() && b.opts.FlattenChoices {
			continue
		}
		if parent == nil {
			parent = b.ensureRoot(ancestor, ProjectionRoleAncestor)
		} else {
			parent = parent.ensureChild(ancestor, ProjectionRoleAncestor)
		}
		addProjectionListKeys(parent, b.opts)
	}
	targetRole := projectionSelectedRole(target, b.opts)
	if parent == nil {
		parent = b.ensureRoot(target, targetRole)
	} else {
		parent = parent.ensureChild(target, targetRole)
	}
	addProjectionListKeys(parent, b.opts)
	includeProjectionDescendants(parent, target, filters, b.opts)
}

func (b *projectionBuilder) ensureRoot(node SchemaNodeRef, role ProjectionRole) *projectionBuildNode {
	if existing := b.roots[node]; existing != nil {
		existing.role |= role
		return existing
	}
	created := newProjectionBuildNode(node, role)
	b.roots[node] = created
	b.rootOrder = append(b.rootOrder, node)
	return created
}

func (b *projectionBuilder) projection() Projection {
	emitted := make(map[SchemaNodeRef]bool, len(b.roots))
	var roots []ProjectedNode
	for top := range b.module.TopLevel().Iter() {
		if root := b.roots[top]; root != nil {
			roots = append(roots, root.project(b.opts))
			emitted[top] = true
		}
	}
	for _, ref := range b.rootOrder {
		if emitted[ref] {
			continue
		}
		if root := b.roots[ref]; root != nil {
			roots = append(roots, root.project(b.opts))
		}
	}
	return Projection{Roots: roots}
}

type projectionBuildNode struct {
	node       SchemaNodeRef
	role       ProjectionRole
	children   map[SchemaNodeRef]*projectionBuildNode
	childOrder []SchemaNodeRef
}

func newProjectionBuildNode(node SchemaNodeRef, role ProjectionRole) *projectionBuildNode {
	return &projectionBuildNode{
		node:     node,
		role:     role,
		children: make(map[SchemaNodeRef]*projectionBuildNode),
	}
}

func (n *projectionBuildNode) ensureChild(node SchemaNodeRef, role ProjectionRole) *projectionBuildNode {
	if existing := n.children[node]; existing != nil {
		existing.role |= role
		return existing
	}
	created := newProjectionBuildNode(node, role)
	n.children[node] = created
	n.childOrder = append(n.childOrder, node)
	return created
}

func (n *projectionBuildNode) project(opts ProjectionOptions) ProjectedNode {
	out := ProjectedNode{Node: n.node, Role: n.role}
	for _, childRef := range orderedProjectionChildren(n, opts) {
		if child := n.children[childRef]; child != nil {
			out.Children = append(out.Children, child.project(opts))
		}
	}
	return out
}

func orderedProjectionChildren(n *projectionBuildNode, opts ProjectionOptions) []SchemaNodeRef {
	if n == nil || len(n.children) == 0 {
		return nil
	}
	emitted := make(map[SchemaNodeRef]bool, len(n.children))
	out := make([]SchemaNodeRef, 0, len(n.children))
	appendIfPresent := func(ref SchemaNodeRef) {
		if emitted[ref] {
			return
		}
		if n.children[ref] == nil {
			return
		}
		out = append(out, ref)
		emitted[ref] = true
	}
	if opts.ListKeysFirst && n.node.IsList() {
		for key := range n.node.ListKeys().Iter() {
			appendIfPresent(key)
		}
	}
	for child := range n.node.DataChildren(opts.FlattenChoices).Iter() {
		appendIfPresent(child)
	}
	for _, child := range n.childOrder {
		appendIfPresent(child)
	}
	return out
}

func addProjectionListKeys(n *projectionBuildNode, opts ProjectionOptions) {
	if n == nil || !opts.IncludeListKeys || !n.node.IsList() {
		return
	}
	for key := range n.node.ListKeys().Iter() {
		n.ensureChild(key, ProjectionRoleKey)
	}
}

func includeProjectionDescendants(parent *projectionBuildNode, current SchemaNodeRef, filters projectionFilters, opts ProjectionOptions) {
	for child := range current.DataChildren(opts.FlattenChoices).Iter() {
		include, role := projectionChildInclusion(child, filters, opts)
		if !include && projectionHasIncludedDescendant(child, filters, opts) {
			include = true
			role = ProjectionRoleDescendant
		}
		if !include {
			continue
		}
		childNode := parent.ensureChild(child, role)
		addProjectionListKeys(childNode, opts)
		includeProjectionDescendants(childNode, child, filters, opts)
	}
}

func projectionChildInclusion(node SchemaNodeRef, filters projectionFilters, opts ProjectionOptions) (bool, ProjectionRole) {
	ignored := filters.ignored(node)
	allowed := filters.allowed(node)
	protectedMandatory := projectionProtectsMandatory(node, filters, opts)
	if ignored && !protectedMandatory {
		return false, 0
	}

	if opts.IncludeDescendants && (allowed || protectedMandatory) {
		role := ProjectionRoleDescendant | projectionSupplementalRole(node, opts, protectedMandatory)
		return true, role
	}

	if opts.IncludeMandatory && projectionMandatoryNode(node) && (allowed || protectedMandatory) {
		return true, projectionSupplementalRole(node, opts, protectedMandatory)
	}

	if opts.IncludeDefaults && projectionDefaultNode(node) && allowed {
		return true, projectionSupplementalRole(node, opts, false)
	}

	return false, 0
}

func projectionHasIncludedDescendant(node SchemaNodeRef, filters projectionFilters, opts ProjectionOptions) bool {
	for child := range node.DataChildren(opts.FlattenChoices).Iter() {
		if include, _ := projectionChildInclusion(child, filters, opts); include {
			return true
		}
		if projectionHasIncludedDescendant(child, filters, opts) {
			return true
		}
	}
	return false
}

func projectionSelectedRole(node SchemaNodeRef, opts ProjectionOptions) ProjectionRole {
	return ProjectionRoleSelected | projectionSupplementalRole(node, opts, false)
}

func projectionSupplementalRole(node SchemaNodeRef, opts ProjectionOptions, protectedMandatory bool) ProjectionRole {
	var role ProjectionRole
	if (opts.IncludeMandatory || protectedMandatory) && projectionMandatoryNode(node) {
		role |= ProjectionRoleMandatory
	}
	if opts.IncludeDefaults && projectionDefaultNode(node) {
		role |= ProjectionRoleDefault
	}
	return role
}

func projectionProtectsMandatory(node SchemaNodeRef, filters projectionFilters, opts ProjectionOptions) bool {
	if !opts.ProtectMandatory || (!opts.IncludeMandatory && !opts.IncludeDescendants) {
		return false
	}
	if !projectionMandatoryNode(node) {
		return false
	}
	return filters.ignored(node) || !filters.allowed(node)
}

func projectionMandatoryNode(node SchemaNodeRef) bool {
	if node.IsMandatory() {
		return true
	}
	if node.IsList() || node.IsLeafList() {
		if minElems, ok := node.MinElements(); ok && minElems > 0 {
			return true
		}
	}
	return false
}

func projectionDefaultNode(node SchemaNodeRef) bool {
	_, ok := node.DefaultEntry()
	return ok
}

type projectionFilters struct {
	allow  []SchemaNodeRef
	ignore []SchemaNodeRef
}

func projectionFiltersFor(root SchemaNodeRef, opts ProjectionOptions) (projectionFilters, error) {
	var filters projectionFilters
	for _, rel := range opts.AllowRelativePaths {
		node, err := resolveProjectionRelativePath(root, rel, opts.FlattenChoices)
		if err != nil {
			if opts.IgnoreInvalidFilters {
				continue
			}
			return projectionFilters{}, fmt.Errorf("filter path %q: %w", rel, err)
		}
		filters.allow = append(filters.allow, node)
	}
	for _, rel := range opts.IgnoreRelativePaths {
		node, err := resolveProjectionRelativePath(root, rel, opts.FlattenChoices)
		if err != nil {
			if opts.IgnoreInvalidFilters {
				continue
			}
			return projectionFilters{}, fmt.Errorf("filter path %q: %w", rel, err)
		}
		filters.ignore = append(filters.ignore, node)
	}
	return filters, nil
}

func (f projectionFilters) ignored(node SchemaNodeRef) bool {
	for _, root := range f.ignore {
		if schemaAncestorOrSelf(root, node) {
			return true
		}
	}
	return false
}

func (f projectionFilters) allowed(node SchemaNodeRef) bool {
	if len(f.allow) == 0 {
		return true
	}
	for _, root := range f.allow {
		if schemaAncestorOrSelf(node, root) || schemaAncestorOrSelf(root, node) {
			return true
		}
	}
	return false
}

func resolveProjectionRelativePath(root SchemaNodeRef, rel string, flattenChoices bool) (SchemaNodeRef, error) {
	if root.node == nil {
		return SchemaNodeRef{}, fmt.Errorf("nil root")
	}
	rel = strings.TrimSpace(rel)
	if rel == "" {
		return SchemaNodeRef{}, fmt.Errorf("empty relative path")
	}
	if rel == "." {
		return root, nil
	}
	if strings.HasPrefix(rel, "/") {
		return SchemaNodeRef{}, fmt.Errorf("absolute paths are not relative")
	}
	source := root.node.ownerModule()
	cur := root
	for _, part := range strings.Split(rel, "/") {
		if part == "" || part == "." || part == ".." || strings.TrimSpace(part) != part {
			return SchemaNodeRef{}, fmt.Errorf("invalid relative path segment %q", part)
		}
		next, err := projectionChildByQName(cur, source, part, flattenChoices)
		if err != nil {
			return SchemaNodeRef{}, err
		}
		cur = next
	}
	return cur, nil
}

func projectionChildByQName(parent SchemaNodeRef, source *moduleData, qname string, flattenChoices bool) (SchemaNodeRef, error) {
	name := localName(qname)
	if name == "" {
		return SchemaNodeRef{}, fmt.Errorf("empty path segment")
	}
	prefix := ""
	var wantModule *moduleData
	if hasPrefix(qname) {
		prefix, _, _ = strings.Cut(qname, ":")
		if source != nil {
			wantModule = source.resolveSourceQNameModule(qname)
		}
	}
	var matches []SchemaNodeRef
	for child := range parent.DataChildren(flattenChoices).Iter() {
		if child.Name() != name {
			continue
		}
		switch {
		case wantModule != nil:
			if child.node != nil && child.node.module == wantModule {
				matches = append(matches, child)
			}
		case prefix != "":
			if child.Module().Name() == prefix || child.Module().Prefix() == prefix {
				matches = append(matches, child)
			}
		default:
			matches = append(matches, child)
		}
	}
	switch len(matches) {
	case 0:
		return SchemaNodeRef{}, fmt.Errorf("relative schema path segment %q not found under %s", qname, parent.Path())
	case 1:
		return matches[0], nil
	default:
		return SchemaNodeRef{}, fmt.Errorf("relative schema path segment %q is ambiguous under %s", qname, parent.Path())
	}
}

func schemaAncestorOrSelf(ancestor, node SchemaNodeRef) bool {
	if ancestor.node == nil || node.node == nil {
		return false
	}
	for cur := node.node; cur != nil; cur = cur.parent {
		if cur == ancestor.node {
			return true
		}
	}
	return false
}
