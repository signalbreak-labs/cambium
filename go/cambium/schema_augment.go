package cambium

import (
	"fmt"
	"strings"

	"github.com/signalbreak-labs/cambium/go/internal/yangparse"
)

// Deviation is metadata for one parsed deviation property.
type Deviation struct {
	targetPath, sourceModule, devType, property, newValue, description, reference string
	ifFeatures                                                                    []string
}

func (m *moduleData) applyUsesAugment(aug *yangparse.Statement, roots []*schemaNodeData, owner *moduleData, groupingStack map[*yangparse.Statement]bool, groupOrigin string) bool {
	target := findRelativeSchemaNode(m, roots, strings.Split(aug.Argument, "/"))
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
		n.defaults = []DefaultValue{{value: defaults[0].Argument, sourceModule: source}}
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
	n.refreshAncestorListConstraints()
}

func (m *moduleData) applyAugments() {
	for _, aug := range m.sourceTopStatements() {
		if aug.Keyword != "augment" || !m.featureIncluded(aug) {
			continue
		}
		if err := validateAbsoluteSchemaNodeIDStatement("augment", aug, m.yangVersionForStatement(aug) == "1.1"); err != nil {
			m.recordSchemaError(err)
			continue
		}
		targetMod, target := m.ctx.findNodeBySourceSchemaPath(m, aug.Argument)
		if target == nil || targetMod == nil {
			m.recordSchemaError(fmt.Errorf("augment %q target not found at %s", aug.Argument, aug.Location()))
			continue
		}
		if m.implemented {
			m.ctx.markImplemented(targetMod)
		}
		children := m.buildChildren(aug, target, m, false, "")
		if targetMod != m {
			if mandatory := firstMandatoryConfigNode(children); mandatory != nil {
				if m.yangVersionForStatement(aug) != "1.1" {
					m.recordSchemaError(fmt.Errorf("augment %q adds mandatory config node %q to another module and requires yang-version 1.1 at %s", aug.Argument, mandatory.name, mandatory.stmt.Location()))
					continue
				}
				if len(direct(aug, "when")) == 0 {
					m.recordSchemaError(fmt.Errorf("augment %q adds mandatory config node %q to another module without a when statement at %s", aug.Argument, mandatory.name, mandatory.stmt.Location()))
					continue
				}
			}
		}
		m.applyAugmentWhen(aug, children)
		prependIfFeatures(children, ifFeatureArgs(aug))
		target.children = append(target.children, children...)
		target.resolveListKeys()
		target.resolveUniqueConstraints()
		appendUnique(&targetMod.augmentedBy, m.name)
	}
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
	case SchemaNodeKindContainer, SchemaNodeKindLeaf, SchemaNodeKindLeafList, SchemaNodeKindList, SchemaNodeKindAnyData:
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
	for _, dev := range m.sourceTopStatements() {
		if dev.Keyword != "deviation" || !m.featureIncluded(dev) {
			continue
		}
		if err := validateAbsoluteSchemaNodeIDStatement("deviation", dev, m.yangVersionForStatement(dev) == "1.1"); err != nil {
			m.recordSchemaError(err)
			continue
		}
		m.ctx.markImplemented(m)
		targetMod, target := m.ctx.findNodeBySourceSchemaPath(m, dev.Argument)
		if targetMod == nil || target == nil {
			m.recordSchemaError(fmt.Errorf("deviation %q target not found at %s", dev.Argument, dev.Location()))
			continue
		}
		if m.implemented {
			m.ctx.markImplemented(targetMod)
		}
		appendUnique(&targetMod.deviatedBy, m.name)
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
			props := nonExtensionSubStatements(d)
			if len(props) == 0 {
				if len(d.SubStatements()) != 0 && d.Argument != "not-supported" {
					continue
				}
				one := Deviation{targetPath: dev.Argument, sourceModule: m.name, devType: d.Argument, description: desc, reference: ref, ifFeatures: ifFeatureArgs(dev)}
				m.deviations = append(m.deviations, one)
				if target != nil {
					target.devs = append(target.devs, one)
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
				}
				m.deviations = append(m.deviations, one)
				if target != nil {
					target.devs = append(target.devs, one)
					m.applyDeviation(targetMod, target, d.Argument, prop)
				}
			}
		}
	}
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
		target.defaults = append(target.defaults, DefaultValue{value: prop.Argument, sourceModule: m})
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
	case "config", "mandatory", "type":
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
		target.defaults = []DefaultValue{{value: prop.Argument, sourceModule: m}}
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
		if target.kind == SchemaNodeKindList && len(target.uniqueNames) == 0 {
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
		before := len(target.uniqueNames)
		out := target.uniqueNames[:0]
		for _, names := range target.uniqueNames {
			if strings.Join(names, "\x00") != want {
				out = append(out, names)
			}
		}
		target.uniqueNames = out
		target.resolveUniqueConstraints()
		if len(target.uniqueNames) == before {
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

func (m Module) AugmentedBy() []string {
	if m.mod == nil {
		return nil
	}
	return append([]string(nil), m.mod.augmentedBy...)
}
func (m Module) Deviations() []Deviation {
	if m.mod == nil {
		return nil
	}
	return append([]Deviation(nil), m.mod.deviations...)
}
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
		n.uniqueNames = [][]string{names}
	} else {
		n.uniqueNames = append(n.uniqueNames, names)
	}
	n.resolveUniqueConstraints()
}
