// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

package codegen

import "github.com/signalbreak-labs/cambium/go/cambium"

// PlanVersion is the stable version tag for the public codegen planning API.
const PlanVersion = "cambium.codegen.plan.v1"

// ModulePlan is the ordered planning model computed before rendering code.
type ModulePlan struct {
	Version    string
	Module     cambium.Module
	Records    []RecordPlan
	Identities []IdentityPlan
}

// RecordPlan is one ordered generated record/struct shape.
type RecordPlan struct {
	Name                 string
	Path                 string
	QualifiedPath        string
	Node                 cambium.SchemaNodeRef
	Fields               []FieldPlan
	SerializerFieldOrder []string
	Validation           ValidationPlan
}

// FieldPlan is one ordered field in a record.
type FieldPlan struct {
	Identifier    string
	WireName      string
	Path          string
	QualifiedPath string
	Node          cambium.SchemaNodeRef
	Type          TypePlan
	Optional      bool
	ListKey       bool
	Config        cambium.Config
	Defaults      []cambium.DefaultValue
	Validation    ValidationPlan
}

// TypePlan summarizes field type metadata. GoType is the current Go renderer's
// type expression; Base and Resolved expose target-neutral Cambium type data.
type TypePlan struct {
	GoType        string
	Base          cambium.BaseType
	Resolved      cambium.ResolvedType
	JSONKind      string
	IsNewtype     bool
	IsEnum        bool
	IsBits        bool
	IsIdentityRef bool
	IsUnion       bool
}

// ValidationPlan exposes validation metadata used by generated validators.
type ValidationPlan struct {
	Mandatory bool
	Min       *uint32
	Max       *uint32
	Musts     []cambium.MustConstraint
	Whens     []cambium.WhenConstraint
	Uniques   []cambium.UniqueConstraint
}

// IdentityPlan is one identity definition and its resolved graph metadata.
type IdentityPlan struct {
	Name       string
	Module     string
	Bases      []cambium.Identity
	Derived    []cambium.Identity
	Source     cambium.SourceLocation
	IfFeatures []string
}

// Plan returns the ordered model the code generator will use before rendering.
func Plan(ctx *cambium.Context, module string) (*ModulePlan, error) {
	mod, err := ctx.Schema(module)
	if err != nil {
		return nil, err
	}
	g := &goEmitter{
		module:       mod,
		moduleName:   mod.Name(),
		modulePascal: toPascalCase(mod.Name()),
		ns:           mod.Namespace(),
	}
	g.initPlanningState()

	plan := &ModulePlan{Version: PlanVersion, Module: mod}
	rootChildren := g.documentTopLevelChildren()
	rootFields := g.collectFields(g.modulePascal, rootChildren)
	plan.Records = append(plan.Records, g.recordPlan(g.modulePascal, cambium.SchemaNodeRef{}, rootFields))
	for _, field := range rootFields {
		g.appendRecordPlans(plan, field)
	}
	for identity := range mod.Identities() {
		plan.Identities = append(plan.Identities, identityPlan(identity))
	}
	return plan, nil
}

func (g *goEmitter) initPlanningState() {
	g.helpers = make(map[string]bool)
	g.typeSnippets = make(map[string]string)
	g.intRangeTypes = make(map[string]intRangeInfo)
	g.stringLengthTypes = make(map[string]stringLengthInfo)
	g.enumTypes = make(map[string]bool)
	g.bitsTypes = make(map[string]bool)
	g.identityrefTypes = make(map[string]bool)
	g.unionTypes = make(map[string]bool)
}

func (g *goEmitter) appendRecordPlans(plan *ModulePlan, field fieldInfo) {
	if !isStructKind(field.node.Kind()) {
		return
	}
	name := fieldConcreteType(field)
	fields := g.collectFields(name, g.recordChildren(field.node))
	plan.Records = append(plan.Records, g.recordPlan(name, field.node, fields))
	for _, child := range fields {
		g.appendRecordPlans(plan, child)
	}
}

func (g *goEmitter) recordChildren(node cambium.SchemaNodeRef) []cambium.SchemaNodeRef {
	switch node.Kind() {
	case cambium.SchemaNodeKindAction, cambium.SchemaNodeKindRPC:
		return g.operationPayloadChildren(node)
	case cambium.SchemaNodeKindNotification:
		return g.orderedChildrenList(node.DataChildren(true))
	case cambium.SchemaNodeKindList:
		ordered := orderedListChildren(node.DataChildren(true), node.ListKeys())
		return append(ordered, g.operationChildren(node)...)
	default:
		ordered := g.orderedChildrenList(node.DataChildren(true))
		return append(ordered, g.operationChildren(node)...)
	}
}

func (g *goEmitter) recordPlan(name string, node cambium.SchemaNodeRef, fields []fieldInfo) RecordPlan {
	record := RecordPlan{
		Name:          name,
		Node:          node,
		Path:          node.LocalPath(),
		QualifiedPath: node.QualifiedPath(),
		Fields:        make([]FieldPlan, 0, len(fields)),
		Validation:    validationPlanForNode(node),
	}
	for _, field := range fields {
		record.Fields = append(record.Fields, fieldPlan(field))
		record.SerializerFieldOrder = append(record.SerializerFieldOrder, field.wire)
	}
	return record
}

func fieldPlan(field fieldInfo) FieldPlan {
	return FieldPlan{
		Identifier:    field.ident,
		WireName:      field.wire,
		Path:          field.node.LocalPath(),
		QualifiedPath: field.node.QualifiedPath(),
		Node:          field.node,
		Type:          typePlan(field),
		Optional:      field.optional,
		ListKey:       field.node.IsListKey(),
		Config:        field.node.Config(),
		Defaults:      field.node.DefaultEntries(),
		Validation:    validationPlanForNode(field.node),
	}
}

func typePlan(field fieldInfo) TypePlan {
	plan := TypePlan{
		GoType:        field.goType,
		JSONKind:      field.jsonKind,
		IsNewtype:     field.isNewtype,
		IsEnum:        field.isEnum,
		IsBits:        field.isBits,
		IsIdentityRef: field.isIdentityref,
		IsUnion:       field.isUnion,
	}
	if info, ok := field.node.LeafType(); ok {
		plan.Base = info.Base()
		plan.Resolved = info.Resolved()
	}
	return plan
}

func validationPlanForNode(node cambium.SchemaNodeRef) ValidationPlan {
	plan := ValidationPlan{
		Mandatory: node.IsMandatory(),
		Musts:     node.Musts(),
		Whens:     node.Whens(),
		Uniques:   node.UniqueConstraints(),
	}
	if minElements, ok := node.MinElements(); ok {
		value := minElements
		plan.Min = &value
	}
	if maxElements, ok := node.MaxElements(); ok {
		value := maxElements
		plan.Max = &value
	}
	return plan
}

func identityPlan(identity cambium.Identity) IdentityPlan {
	return IdentityPlan{
		Name:       identity.Name(),
		Module:     identity.Module().Name(),
		Bases:      identity.Bases(),
		Derived:    identity.DerivedClosure(),
		Source:     identity.SourceLocation(),
		IfFeatures: identity.IfFeatures(),
	}
}
