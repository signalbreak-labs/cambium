// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

package cambium

import (
	"fmt"
	"slices"
	"strings"

	"github.com/signalbreak-labs/cambium/go/internal/yangparse"
)

// Deviation is metadata for one parsed deviation property.
type Deviation struct {
	targetPath, sourceModule, devType, property, newValue, description, reference string
	ifFeatures                                                                    []string
	source                                                                        *yangparse.Statement
	ignored                                                                       bool
}

func (m *moduleData) applyUsesAugment(aug *yangparse.Statement, roots []*schemaNodeData, owner *moduleData, groupingStack map[*yangparse.Statement]bool, groupOrigin string) bool {
	target := findRelativeSchemaNode(m, roots, strings.Split(aug.Argument, "/"), aug)
	if target == nil {
		return false
	}
	children := m.buildChildrenSeen(aug, target, owner, target.choiceDesc, groupOrigin, groupingStack)
	prependIfFeatures(children, ifFeatureArgs(aug))
	m.applyAugmentWhen(aug, children)
	target.children = append(target.children, children...)
	return true
}

func applyRefine(source *moduleData, n *schemaNodeData, refine *yangparse.Statement) {
	defaults := direct(refine, "default")
	if len(defaults) > 1 {
		n.recordSchemaError(fmt.Errorf("refine %q has multiple default statements at %s", refine.Argument, defaults[1].Location()))
		return
	}
	if len(defaults) == 1 {
		n.defaults = []DefaultValue{{value: defaults[0].Argument, sourceModule: source, origin: DefaultOriginRefine}}
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
	n.musts = append(n.musts, n.mustsFrom(source, refine)...)
}

// augmentJob is one top-level augment statement. rank is its position in
// module-load order, then augment statement source order; it fixes where the
// augment's children land among other augments of the same target, whichever
// resolution pass applies it.
type augmentJob struct {
	mod  *moduleData
	stmt *yangparse.Statement
	rank int
	// disabled marks an augment whose if-feature the enabled feature set
	// rejects: it only records its content as feature-excluded on its target.
	disabled bool
	// targetMod is the module owning the target once the augment is applied.
	targetMod *moduleData
}

// implementAmendmentTargets implements every module that a prefix in the
// target path of an implemented module's augment or deviation names, until no
// further module is implemented (RFC 7950 §5.6.5). A module that is only
// imported contributes no augments or deviations, so its paths name nothing.
func (c *Context) implementAmendmentTargets() {
	for changed := true; changed; {
		changed = false
		for _, m := range c.loadOrder {
			if m.stmt == nil || !m.implemented {
				continue
			}
			for _, st := range m.sourceTopStatements() {
				if (st.Keyword != "augment" && st.Keyword != "deviation") || !m.featureIncluded(st) {
					continue
				}
				if st.Keyword == "deviation" && c.deviationPolicy.IgnoreAll {
					continue
				}
				for _, step := range strings.Split(strings.TrimPrefix(st.Argument, "/"), "/") {
					if !hasPrefix(step) {
						continue
					}
					if target := m.resolveSourceQNameModuleFrom(step, st); target != nil && !target.implemented {
						c.markImplemented(target)
						changed = true
					}
				}
			}
		}
	}
}

// implementedAmendingModules counts the implemented modules that declare a
// top-level augment or deviation.
func (c *Context) implementedAmendingModules() int {
	count := 0
	for _, m := range c.loadOrder {
		if !m.implemented {
			continue
		}
		for _, st := range m.sourceTopStatements() {
			if st.Keyword == "augment" || st.Keyword == "deviation" {
				count++
				break
			}
		}
	}
	return count
}

// applyAugments applies every implemented module's top-level augments. An augment
// may target a node that another augment creates, so resolution repeats until
// no pending augment's target resolves: whether a target resolves does not
// depend on the order augments are declared in or modules are loaded, and each
// augment's children are placed by rank whichever pass applies it
// (spec/ordering-invariants.md §1.1 rule 3). Exact schema paths are resolved
// to a fixpoint before the vendor-compatible local-name fallback is tried, one
// augment at a time, so the fallback never claims a node in place of a target
// another augment has yet to create. Augments whose target never resolves are
// reported in rank order.
func (c *Context) applyAugments() {
	var jobs []*augmentJob
	for _, m := range c.loadOrder {
		if !m.implemented {
			// A module that is only imported contributes no augments
			// (RFC 7950 §5.6.5).
			continue
		}
		for _, aug := range m.sourceTopStatements() {
			if aug.Keyword != "augment" {
				continue
			}
			job := &augmentJob{mod: m, stmt: aug, rank: len(jobs) + 1, disabled: !m.featureIncluded(aug)}
			if !job.disabled {
				if err := validateAbsoluteSchemaNodeIDStatement("augment", aug, m.yangVersionForStatement(aug) == "1.1"); err != nil {
					m.recordSchemaError(err)
					continue
				}
			}
			jobs = append(jobs, job)
		}
	}
	pending := jobs
	for len(pending) > 0 {
		var progress bool
		if pending, progress = c.applyResolvableAugments(pending, false); progress {
			continue
		}
		if c.validationMode != ValidationVendorCompatible {
			break
		}
		if pending, progress = c.applyResolvableAugments(pending, true); !progress {
			break
		}
	}
	for _, job := range pending {
		if !job.disabled {
			job.mod.reportUnresolvedAugment(job.stmt)
		}
	}
	for _, job := range jobs {
		if job.targetMod != nil {
			appendUnique(&job.targetMod.augmentedBy, job.mod.name)
		}
	}
}

// applyResolvableAugments applies, in rank order, each pending augment whose
// target resolves and returns the augments still pending. With fallback it
// resolves through the vendor-compatible local-name fallback instead and stops
// after the first augment it resolves, so exact resolution runs again first.
func (c *Context) applyResolvableAugments(pending []*augmentJob, fallback bool) ([]*augmentJob, bool) {
	progress := false
	var rest []*augmentJob
	for i, job := range pending {
		if fallback && progress {
			rest = append(rest, pending[i:]...)
			break
		}
		m, aug := job.mod, job.stmt
		var targetMod *moduleData
		var target *schemaNodeData
		var reason string
		if fallback {
			targetMod, target, reason = c.findNodeByVendorCompatibleSchemaPath(m, aug.Argument, aug)
		} else {
			targetMod, target, _, _ = c.findNodeBySchemaPathDetail(m, aug.Argument, true, aug)
		}
		if target == nil || targetMod == nil {
			rest = append(rest, job)
			continue
		}
		progress = true
		if job.disabled {
			// A disabled augment contributes nothing, so its path adds no
			// warnings to the load report.
			m.recordFeatureExcludedAugment(target, aug, m)
			continue
		}
		if fallback {
			m.recordVendorCompatibleWarning(aug, nil, "schema path %q resolved by %s in vendor-compatible mode", aug.Argument, reason)
		}
		if m.applyAugment(aug, targetMod, target, job.rank) {
			job.targetMod = targetMod
		}
	}
	return rest, progress
}

func (m *moduleData) reportUnresolvedAugment(aug *yangparse.Statement) {
	excluded, ok := m.ctx.featureExcludedSchemaPathStep(m, aug.Argument, aug)
	if !ok {
		// A typo or missing dependency is never relaxed: the schema would
		// silently lose the augment's content.
		m.recordSchemaError(fmt.Errorf("augment %q target not found at %s", aug.Argument, aug.Location()))
		return
	}
	if m.ctx.validationMode == ValidationVendorCompatible {
		m.recordOmittedContentWarning(aug, "augment %q target not found at %s: target node %q is excluded by feature policy; augment skipped in vendor-compatible mode", aug.Argument, aug.Location(), excluded)
		return
	}
	m.recordSchemaError(fmt.Errorf("augment %q target not found at %s: target node %q is excluded by feature policy", aug.Argument, aug.Location(), excluded))
}

// applyAugment adds aug's children to its resolved target and reports whether
// the augment was applied.
func (m *moduleData) applyAugment(aug *yangparse.Statement, targetMod *moduleData, target *schemaNodeData, rank int) bool {
	if m.implemented {
		m.ctx.markImplemented(targetMod)
	}
	children := m.buildChildren(aug, target, m, false, "")
	if targetMod != m {
		if mandatory := firstMandatoryConfigNode(children); mandatory != nil {
			if m.yangVersionForStatement(aug) != "1.1" {
				if m.ctx != nil && m.ctx.validationMode == ValidationVendorCompatible {
					m.recordVendorCompatibleWarning(mandatory.stmt, []*yangparse.Statement{aug}, "augment %q adds mandatory config node %q to another module and requires yang-version 1.1 at %s; allowed in vendor-compatible mode", aug.Argument, mandatory.name, mandatory.stmt.Location())
				} else {
					m.recordSchemaError(fmt.Errorf("augment %q adds mandatory config node %q to another module and requires yang-version 1.1 at %s", aug.Argument, mandatory.name, mandatory.stmt.Location()))
					return false
				}
			}
			if len(direct(aug, "when")) == 0 {
				if m.ctx != nil && m.ctx.validationMode == ValidationVendorCompatible {
					m.recordVendorCompatibleWarning(mandatory.stmt, []*yangparse.Statement{aug}, "augment %q adds mandatory config node %q to another module without a when statement at %s; allowed in vendor-compatible mode", aug.Argument, mandatory.name, mandatory.stmt.Location())
				} else {
					m.recordSchemaError(fmt.Errorf("augment %q adds mandatory config node %q to another module without a when statement at %s", aug.Argument, mandatory.name, mandatory.stmt.Location()))
					return false
				}
			}
		}
	}
	m.applyAugmentWhen(aug, children)
	prependIfFeatures(children, ifFeatureArgs(aug))
	insertAugmentChildren(target, children, rank)
	target.resolveListKeys()
	target.resolveUniqueConstraints()
	return true
}

// insertAugmentChildren places one augment's children on target after the
// target's declared and already-expanded children and after the children of
// every augment of lower rank, so their position does not depend on which
// resolution pass applied the augment.
func insertAugmentChildren(target *schemaNodeData, children []*schemaNodeData, rank int) {
	for _, child := range children {
		child.augmentRank = rank
	}
	at := len(target.children)
	for at > 0 && target.children[at-1] != nil && target.children[at-1].augmentRank > rank {
		at--
	}
	target.children = slices.Insert(target.children, at, children...)
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
	var walk func(*schemaNodeData, int)
	walk = func(n *schemaNodeData, depth int) {
		if n == nil {
			return
		}
		appendInheritedWhen(n, when.withSourceModule(m).withContextAncestorDepth(depth).withExcludedSubtrees(roots))
		childDepth := depth
		if dataTreeContextNode(n) {
			childDepth++
		}
		for _, child := range n.children {
			walk(child, childDepth)
		}
	}
	for _, root := range roots {
		walk(root, 1)
	}
}

func dataTreeContextNode(n *schemaNodeData) bool {
	switch n.kind {
	case SchemaNodeKindContainer, SchemaNodeKindLeaf, SchemaNodeKindLeafList, SchemaNodeKindList, SchemaNodeKindAnyData, SchemaNodeKindAnyXML:
		return true
	default:
		return false
	}
}

func propagateChoiceCaseWhens(n *schemaNodeData) {
	if n == nil {
		return
	}
	for _, when := range n.whens {
		propagateWhenToDataDescendants(n.children, when.withExcludedSubtrees(n.children), 1)
	}
	for _, child := range n.children {
		if child == nil || child.kind != SchemaNodeKindCase {
			continue
		}
		for _, when := range child.whens {
			propagateWhenToDataDescendants(child.children, when.withExcludedSubtrees([]*schemaNodeData{child}), 1)
		}
	}
}

func propagateWhenToDataDescendants(nodes []*schemaNodeData, when WhenConstraint, depth int) {
	for _, n := range nodes {
		if n == nil {
			continue
		}
		childDepth := depth
		if dataTreeContextNode(n) {
			appendInheritedWhen(n, when.withContextAncestorDepth(depth))
			childDepth++
		}
		propagateWhenToDataDescendants(n.children, when, childDepth)
	}
}

func (w WhenConstraint) withContextAncestorDepth(depth int) WhenConstraint {
	w.contextAncestorDepth = depth
	return w
}

func (w WhenConstraint) withExcludedSubtrees(roots []*schemaNodeData) WhenConstraint {
	w.excludedSubtrees = append([]*schemaNodeData(nil), roots...)
	return w
}

func (m MustConstraint) withSourceModule(source *moduleData) MustConstraint {
	m.sourceModule = source
	return m
}

func (w WhenConstraint) withSourceModule(source *moduleData) WhenConstraint {
	w.sourceModule = source
	return w
}

func appendInheritedWhen(n *schemaNodeData, when WhenConstraint) {
	if n == nil || n.listKey {
		return
	}
	n.whens = append(n.whens, when)
}

func (m *moduleData) collectDeviations() {
	if !m.implemented {
		// A module that is only imported contributes no deviations
		// (RFC 7950 §5.6.5).
		return
	}
	for _, dev := range m.sourceTopStatements() {
		if dev.Keyword != "deviation" || !m.featureIncluded(dev) {
			continue
		}
		if err := validateAbsoluteSchemaNodeIDStatement("deviation", dev, m.yangVersionForStatement(dev) == "1.1"); err != nil {
			m.recordSchemaError(err)
			continue
		}
		targetMod, target := m.ctx.findNodeBySourceSchemaPathFrom(m, dev.Argument, dev)
		ignoreAll := m.ctx.deviationPolicy.IgnoreAll
		if !ignoreAll && (targetMod == nil || target == nil) {
			excluded, ok := m.ctx.featureExcludedSchemaPathStep(m, dev.Argument, dev)
			if !ok {
				m.recordSchemaError(fmt.Errorf("deviation %q target not found at %s", dev.Argument, dev.Location()))
				continue
			}
			if m.ctx.validationMode == ValidationVendorCompatible {
				// The target is absent from the effective schema, so the
				// deviation has nothing to change; no declared content is lost.
				m.recordVendorCompatibleWarning(dev, nil, "deviation %q target not found at %s: target node %q is excluded by feature policy; deviation skipped in vendor-compatible mode", dev.Argument, dev.Location(), excluded)
				continue
			}
			m.recordSchemaError(fmt.Errorf("deviation %q target not found at %s: target node %q is excluded by feature policy", dev.Argument, dev.Location(), excluded))
			continue
		}
		if !ignoreAll && m.implemented {
			m.ctx.markImplemented(targetMod)
		}
		if targetMod != nil {
			appendUnique(&targetMod.deviatedBy, m.name)
		}
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
			if ignoreAll {
				if err := m.validateIgnoredDeviate(d); err != nil {
					m.recordSchemaError(err)
					continue
				}
			}
			props := nonExtensionSubStatements(d)
			if len(props) == 0 {
				if len(d.SubStatements()) != 0 && d.Argument != "not-supported" {
					continue
				}
				one := Deviation{targetPath: dev.Argument, sourceModule: m.name, devType: d.Argument, description: desc, reference: ref, ifFeatures: ifFeatureArgs(dev), source: d}
				// Policy is decided here, before any static reference is
				// validated, so an ignored removal never breaks the schema.
				if ignoreAll || d.Argument == "not-supported" && m.ctx.deviationPolicy.IgnoreNotSupported {
					one.ignored = true
				}
				m.deviations = append(m.deviations, one)
				if target != nil {
					target.devs = append(target.devs, one)
				}
				if !one.ignored {
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
					source:       prop,
					ignored:      ignoreAll,
				}
				m.deviations = append(m.deviations, one)
				if target != nil {
					target.devs = append(target.devs, one)
				}
				if !one.ignored {
					m.applyDeviation(targetMod, target, d.Argument, prop)
				}
			}
		}
	}
}

func (m *moduleData) validateIgnoredDeviate(deviate *yangparse.Statement) error {
	for _, keyword := range []string{"type", "units", "config", "mandatory", "min-elements", "max-elements"} {
		if _, err := singletonChild(deviate, keyword); err != nil {
			return err
		}
	}
	if deviate.Argument == "replace" || m.yangVersionForStatement(deviate) != "1.1" {
		if _, err := singletonChild(deviate, "default"); err != nil {
			return err
		}
	}
	for _, prop := range nonExtensionSubStatements(deviate) {
		if prop.Keyword == "type" {
			// Type declarations can be checked in their source scope without
			// resolving or modifying the ignored deviation's target node.
			if _, err := m.parseType(prop); err != nil {
				return err
			}
		} else if err := validateIgnoredAmendmentProperty(prop); err != nil {
			return err
		}
	}
	return nil
}

// Ignored properties still have to be valid declarations. Target-dependent
// checks (including type/default applicability) belong to applying an amendment.
func validateIgnoredAmendmentProperty(prop *yangparse.Statement) error {
	switch prop.Keyword {
	case "must":
		_, err := mustFromValidated(prop)
		return err
	case "config", "mandatory":
		if _, ok := parseYangBool(prop); !ok {
			return fmt.Errorf("invalid %s %q at %s", prop.Keyword, prop.Argument, prop.Location())
		}
	case "unique":
		if names, ok := parseYANGIdentifierListFields(prop.Argument); !ok || len(names) == 0 {
			return fmt.Errorf("invalid unique %q at %s", prop.Argument, prop.Location())
		}
	case "min-elements", "max-elements":
		if prop.Keyword == "max-elements" && prop.Argument == "unbounded" {
			return nil
		}
		if value, ok := parseUint32(prop.Argument); !ok || prop.Keyword == "max-elements" && value == 0 {
			return fmt.Errorf("invalid %s %q at %s", prop.Keyword, prop.Argument, prop.Location())
		}
	}
	return nil
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

func (m *moduleData) addDeviationProperty(target *schemaNodeData, prop *yangparse.Statement) {
	switch prop.Keyword {
	case "default":
		if containsDefaultValue(target.defaults, prop.Argument) {
			target.recordSchemaError(fmt.Errorf("deviate add default %q for %q already exists at %s", prop.Argument, target.name, prop.Location()))
			return
		}
		target.defaults = append(target.defaults, DefaultValue{value: prop.Argument, sourceModule: m, origin: DefaultOriginDeviation})
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
		target.musts = append(target.musts, constraint.withSourceModule(m))
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
	case "config", "mandatory":
		// RFC 7950 §7.20.3.2: an explicit config or mandatory statement (not
		// a value inherited from an ancestor or a default) already exists.
		if (prop.Keyword == "config" && target.configProp != nil) || (prop.Keyword == "mandatory" && target.mandatoryProp != nil) {
			target.recordSchemaError(fmt.Errorf("deviate add %s for %q already exists at %s", prop.Keyword, target.name, prop.Location()))
			return
		}
		m.replaceDeviationProperty(target, prop)
	case "type":
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
		target.defaults = []DefaultValue{{value: prop.Argument, sourceModule: m, origin: DefaultOriginDeviation}}
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
		target.musts = []MustConstraint{constraint.withSourceModule(m)}
	case "unique":
		if target.kind == SchemaNodeKindList && len(target.uniqueSpecs) == 0 {
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
		target.defaults = removeDefaultValue(target.defaults, prop.Argument)
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
		before := len(target.uniqueSpecs)
		out := target.uniqueSpecs[:0]
		for _, spec := range target.uniqueSpecs {
			if strings.Join(spec.names, "\x00") != want {
				out = append(out, spec)
			}
		}
		target.uniqueSpecs = out
		target.resolveUniqueConstraints()
		if len(target.uniqueSpecs) == before {
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

// AugmentedBy returns the names of modules that augment this module.
func (m Module) AugmentedBy() []string {
	if m.mod == nil {
		return nil
	}
	return append([]string(nil), m.mod.augmentedBy...)
}

// Deviations returns the deviations declared by this module.
func (m Module) Deviations() []Deviation {
	if m.mod == nil {
		return nil
	}
	return append([]Deviation(nil), m.mod.deviations...)
}

// DeviationProvenance returns the deviations targeting this node, in apply
// order. Deviations suppressed by DeviationPolicy are listed with Applied()
// == false when their targets exist in the effective schema.
func (n SchemaNodeRef) DeviationProvenance() []Deviation {
	if n.node == nil {
		return nil
	}
	return append([]Deviation(nil), n.node.devs...)
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
		n.uniqueSpecs = []uniqueSpec{{expression: prop.Argument, names: names}}
	} else {
		n.uniqueSpecs = append(n.uniqueSpecs, uniqueSpec{expression: prop.Argument, names: names})
	}
	n.resolveUniqueConstraints()
}
