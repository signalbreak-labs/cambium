// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

package codegen

import (
	"fmt"
	"strings"

	"github.com/signalbreak-labs/cambium/go/cambium"
)

func (g *goEmitter) documentValidationSourceChildren() []cambium.SchemaNodeRef {
	children := schemaChildrenList(g.module.TopLevel())
	seen := map[string]bool{moduleIdentityKey(g.module): true}
	for _, imp := range g.module.Imports() {
		imported, ok := g.module.ResolvePrefix(imp.Prefix)
		if !ok {
			continue
		}
		key := moduleIdentityKey(imported)
		if seen[key] {
			continue
		}
		seen[key] = true
		children = append(children, schemaChildrenList(imported.TopLevel())...)
	}
	return children
}

func (g *goEmitter) emitPatternValidateHelper(out *strings.Builder) {
	out.WriteString(tmplPatternValidate)
}

func (g *goEmitter) emitValidationMethods(name string, fields []fieldInfo, sourceChildren []cambium.SchemaNodeRef, rootPath string, out *strings.Builder) {
	fmt.Fprintf(out, "func (n *%s) Validate() error {\n", name)
	out.WriteString("\tif n == nil { return nil }\n")
	fmt.Fprintf(out, "\treturn n.validate(%q)\n", rootPath)
	out.WriteString("}\n\n")

	fmt.Fprintf(out, "func (n *%s) validate(path string) error {\n", name)
	out.WriteString("\tif n == nil { return nil }\n")
	g.emitMetadataValidation(fields, out)
	g.emitChoiceValidation(fields, sourceChildren, out)
	for _, f := range fields {
		g.emitFieldValidation(f, out)
	}
	out.WriteString("\treturn nil\n")
	out.WriteString("}\n\n")
}

func (g *goEmitter) emitMetadataValidation(fields []fieldInfo, out *strings.Builder) {
	targets := metadataTargetFields(fields)
	if len(targets) == 0 {
		return
	}
	out.WriteString("\tfor metadataNode := range n.CambiumMetadata {\n")
	out.WriteString("\t\tswitch metadataNode {\n")
	for _, f := range targets {
		fmt.Fprintf(out, "\t\tcase %q:\n", f.wire)
	}
	out.WriteString("\t\tdefault:\n")
	out.WriteString("\t\t\treturn cambiumValidationError(cambiumJoinPath(path, metadataNode), \"metadata for unknown data node\")\n")
	out.WriteString("\t\t}\n")
	out.WriteString("\t}\n")
	for _, f := range targets {
		fmt.Fprintf(out, "\tif len(n.CambiumMetadata[%q]) > 0 {\n", f.wire)
		fmt.Fprintf(out, "\t\tif !(%s) { return cambiumValidationError(cambiumJoinPath(path, %q), %q) }\n", fieldPresentExpr("n", f), f.wire, "metadata for absent data node")
		fmt.Fprintf(out, "\t\tfor _, item := range n.CambiumMetadata[%q] {\n", f.wire)
		fmt.Fprintf(out, "\t\t\tif err := cambiumValidateMetadataAnnotation(item); err != nil { return cambiumValidationError(cambiumJoinPath(path, %q), err.Error()) }\n", f.wire)
		out.WriteString("\t\t}\n")
		out.WriteString("\t}\n")
	}
}

func metadataTargetFields(fields []fieldInfo) []fieldInfo {
	var out []fieldInfo
	for _, f := range fields {
		switch f.node.Kind() {
		case cambium.SchemaNodeKindLeaf, cambium.SchemaNodeKindLeafList:
			out = append(out, f)
		}
	}
	return out
}

func (g *goEmitter) emitStringValueValidationAt(f fieldInfo, owner, pathExpr string, out *strings.Builder, indent string) {
	if f.isUnion || !stringValueType(f.node) {
		return
	}
	base := fieldConcreteType(f)
	_, isLengthType := g.stringLengthTypes[base]
	ref := owner + "." + f.ident
	switch f.node.Kind() {
	case cambium.SchemaNodeKindLeaf:
		if f.optional {
			valueExpr := "*" + ref
			if isLengthType {
				valueExpr = ref + ".String()"
			}
			fmt.Fprintf(out, "%sif %s != nil {\n", indent, ref)
			fmt.Fprintf(out, "%s\tif err := cambiumValidateStringValue(%s); err != nil { return cambiumValidationError(%s, err.Error()) }\n", indent, valueExpr, pathExpr)
			fmt.Fprintf(out, "%s}\n", indent)
			return
		}
		valueExpr := ref
		if isLengthType {
			valueExpr = ref + ".String()"
		}
		fmt.Fprintf(out, "%sif err := cambiumValidateStringValue(%s); err != nil { return cambiumValidationError(%s, err.Error()) }\n", indent, valueExpr, pathExpr)
	case cambium.SchemaNodeKindLeafList:
		itemsExpr := fieldItemsExpr(owner, f.ident, f)
		valueExpr := "v"
		if isLengthType {
			valueExpr = "v.String()"
		}
		fmt.Fprintf(out, "%sfor _, v := range %s {\n", indent, itemsExpr)
		fmt.Fprintf(out, "%s\tif err := cambiumValidateStringValue(%s); err != nil { return cambiumValidationError(%s, err.Error()) }\n", indent, valueExpr, pathExpr)
		fmt.Fprintf(out, "%s}\n", indent)
	}
}

func (g *goEmitter) emitFieldValidation(f fieldInfo, out *strings.Builder) {
	wire := f.wire
	switch f.node.Kind() {
	case cambium.SchemaNodeKindLeafList, cambium.SchemaNodeKindList:
		lenExpr := validationLenExpr("n", f)
		if min, ok := f.node.MinElements(); ok && min > 0 {
			if !f.node.IsChoiceDescendant() {
				fmt.Fprintf(out, "\tif %s < %d {\n", lenExpr, min)
				fmt.Fprintf(out, "\t\treturn cambiumValidationError(cambiumJoinPath(path, %q), %q)\n", wire, "min-elements violation")
				out.WriteString("\t}\n")
			}
		}
		if max, ok := f.node.MaxElements(); ok {
			if !f.node.IsChoiceDescendant() {
				fmt.Fprintf(out, "\tif %s > %d {\n", lenExpr, max)
				fmt.Fprintf(out, "\t\treturn cambiumValidationError(cambiumJoinPath(path, %q), %q)\n", wire, "max-elements violation")
				out.WriteString("\t}\n")
			}
		}
	}

	base := fieldConcreteType(f)
	if _, ok := g.intRangeTypes[base]; ok {
		pathExpr := fmt.Sprintf("cambiumJoinPath(path, %q)", wire)
		switch f.node.Kind() {
		case cambium.SchemaNodeKindLeaf:
			if f.optional {
				fmt.Fprintf(out, "\tif n.%s != nil {\n", f.ident)
				fmt.Fprintf(out, "\t\tif _, err := New%s(n.%s.Get()); err != nil { return cambiumValidationError(%s, err.Error()) }\n", base, f.ident, pathExpr)
				out.WriteString("\t}\n")
			} else {
				fmt.Fprintf(out, "\tif _, err := New%s(n.%s.Get()); err != nil { return cambiumValidationError(%s, err.Error()) }\n", base, f.ident, pathExpr)
			}
		case cambium.SchemaNodeKindLeafList:
			itemsExpr := fieldItemsExpr("n", f.ident, f)
			fmt.Fprintf(out, "\tfor _, v := range %s {\n", itemsExpr)
			fmt.Fprintf(out, "\t\tif _, err := New%s(v.Get()); err != nil { return cambiumValidationError(%s, err.Error()) }\n", base, pathExpr)
			out.WriteString("\t}\n")
		}
	}
	if _, ok := g.stringLengthTypes[base]; ok {
		pathExpr := fmt.Sprintf("cambiumJoinPath(path, %q)", wire)
		switch f.node.Kind() {
		case cambium.SchemaNodeKindLeaf:
			if f.optional {
				fmt.Fprintf(out, "\tif n.%s != nil {\n", f.ident)
				fmt.Fprintf(out, "\t\tif _, err := New%s(n.%s.String()); err != nil { return cambiumValidationError(%s, err.Error()) }\n", base, f.ident, pathExpr)
				out.WriteString("\t}\n")
			} else {
				fmt.Fprintf(out, "\tif _, err := New%s(n.%s.String()); err != nil { return cambiumValidationError(%s, err.Error()) }\n", base, f.ident, pathExpr)
			}
		case cambium.SchemaNodeKindLeafList:
			itemsExpr := fieldItemsExpr("n", f.ident, f)
			fmt.Fprintf(out, "\tfor _, v := range %s {\n", itemsExpr)
			fmt.Fprintf(out, "\t\tif _, err := New%s(v.String()); err != nil { return cambiumValidationError(%s, err.Error()) }\n", base, pathExpr)
			out.WriteString("\t}\n")
		}
	}

	g.emitStringValueValidationAt(f, "n", fmt.Sprintf("cambiumJoinPath(path, %q)", wire), out, "\t")

	if info, ok := stringPatternInfo(f.node); ok {
		g.helpers["patternValidate"] = true
		patterns := stringPatternLiteral(info)
		pathExpr := fmt.Sprintf("cambiumJoinPath(path, %q)", wire)
		_, isLengthType := g.stringLengthTypes[base]
		switch f.node.Kind() {
		case cambium.SchemaNodeKindLeaf:
			if f.optional {
				valueExpr := fmt.Sprintf("*n.%s", f.ident)
				if isLengthType {
					valueExpr = fmt.Sprintf("n.%s.String()", f.ident)
				}
				fmt.Fprintf(out, "\tif n.%s != nil {\n", f.ident)
				fmt.Fprintf(out, "\t\tif err := cambiumValidateStringPatterns(%s, %s); err != nil { return cambiumValidationError(%s, err.Error()) }\n", valueExpr, patterns, pathExpr)
				out.WriteString("\t}\n")
			} else {
				valueExpr := fmt.Sprintf("n.%s", f.ident)
				if isLengthType {
					valueExpr = fmt.Sprintf("n.%s.String()", f.ident)
				}
				fmt.Fprintf(out, "\tif err := cambiumValidateStringPatterns(%s, %s); err != nil { return cambiumValidationError(%s, err.Error()) }\n", valueExpr, patterns, pathExpr)
			}
		case cambium.SchemaNodeKindLeafList:
			itemsExpr := fieldItemsExpr("n", f.ident, f)
			valueExpr := "v"
			if isLengthType {
				valueExpr = "v.String()"
			}
			fmt.Fprintf(out, "\tfor _, v := range %s {\n", itemsExpr)
			fmt.Fprintf(out, "\t\tif err := cambiumValidateStringPatterns(%s, %s); err != nil { return cambiumValidationError(%s, err.Error()) }\n", valueExpr, patterns, pathExpr)
			out.WriteString("\t}\n")
		}
	}

	if info, ok := binaryTypeInfo(f.node); ok {
		g.helpers["binaryParse"] = true
		pathExpr := fmt.Sprintf("cambiumJoinPath(path, %q)", wire)
		ranges := binaryLengthRangeLiteral(info)
		switch f.node.Kind() {
		case cambium.SchemaNodeKindLeaf:
			if f.optional {
				fmt.Fprintf(out, "\tif n.%s != nil {\n", f.ident)
				fmt.Fprintf(out, "\t\tif err := cambiumValidateBinaryString(*n.%s, %s); err != nil { return cambiumValidationError(%s, err.Error()) }\n", f.ident, ranges, pathExpr)
				out.WriteString("\t}\n")
			} else {
				fmt.Fprintf(out, "\tif err := cambiumValidateBinaryString(n.%s, %s); err != nil { return cambiumValidationError(%s, err.Error()) }\n", f.ident, ranges, pathExpr)
			}
		case cambium.SchemaNodeKindLeafList:
			itemsExpr := fieldItemsExpr("n", f.ident, f)
			fmt.Fprintf(out, "\tfor _, v := range %s {\n", itemsExpr)
			fmt.Fprintf(out, "\t\tif err := cambiumValidateBinaryString(v, %s); err != nil { return cambiumValidationError(%s, err.Error()) }\n", ranges, pathExpr)
			out.WriteString("\t}\n")
		}
	}

	if info, ok := decimal64TypeInfo(f.node); ok {
		fd, ranges := decimal64ValidationArgs(info)
		pathExpr := fmt.Sprintf("cambiumJoinPath(path, %q)", wire)
		switch f.node.Kind() {
		case cambium.SchemaNodeKindLeaf:
			if f.optional {
				fmt.Fprintf(out, "\tif n.%s != nil {\n", f.ident)
				fmt.Fprintf(out, "\t\tif err := cambiumValidateDecimal64Value(*n.%s, %d, %s); err != nil { return cambiumValidationError(%s, err.Error()) }\n", f.ident, fd, ranges, pathExpr)
				out.WriteString("\t}\n")
			} else {
				fmt.Fprintf(out, "\tif err := cambiumValidateDecimal64Value(n.%s, %d, %s); err != nil { return cambiumValidationError(%s, err.Error()) }\n", f.ident, fd, ranges, pathExpr)
			}
		case cambium.SchemaNodeKindLeafList:
			itemsExpr := fieldItemsExpr("n", f.ident, f)
			fmt.Fprintf(out, "\tfor _, v := range %s {\n", itemsExpr)
			fmt.Fprintf(out, "\t\tif err := cambiumValidateDecimal64Value(v, %d, %s); err != nil { return cambiumValidationError(%s, err.Error()) }\n", fd, ranges, pathExpr)
			out.WriteString("\t}\n")
		}
	}

	if f.jsonKind == "InstanceIdentifier" {
		pathExpr := fmt.Sprintf("cambiumJoinPath(path, %q)", wire)
		switch f.node.Kind() {
		case cambium.SchemaNodeKindLeaf:
			if f.optional {
				fmt.Fprintf(out, "\tif n.%s != nil {\n", f.ident)
				fmt.Fprintf(out, "\t\tif err := n.%s.validate(%s); err != nil { return err }\n", f.ident, pathExpr)
				out.WriteString("\t}\n")
			} else {
				fmt.Fprintf(out, "\tif err := n.%s.validate(%s); err != nil { return err }\n", f.ident, pathExpr)
			}
		case cambium.SchemaNodeKindLeafList:
			itemsExpr := fieldItemsExpr("n", f.ident, f)
			fmt.Fprintf(out, "\tfor _, v := range %s {\n", itemsExpr)
			fmt.Fprintf(out, "\t\tif err := v.validate(%s); err != nil { return err }\n", pathExpr)
			out.WriteString("\t}\n")
		}
	}

	if f.isEnum || f.isIdentityref {
		method := "AsName"
		message := "invalid enum value"
		if f.isIdentityref {
			method = "AsJSONName"
			message = "invalid identityref value"
		}
		pathExpr := fmt.Sprintf("cambiumJoinPath(path, %q)", wire)
		switch f.node.Kind() {
		case cambium.SchemaNodeKindLeaf:
			if f.optional {
				fmt.Fprintf(out, "\tif n.%s != nil && n.%s.%s() == \"\" { return cambiumValidationError(%s, %q) }\n", f.ident, f.ident, method, pathExpr, message)
			} else {
				fmt.Fprintf(out, "\tif n.%s.%s() == \"\" { return cambiumValidationError(%s, %q) }\n", f.ident, method, pathExpr, message)
			}
		case cambium.SchemaNodeKindLeafList:
			itemsExpr := fieldItemsExpr("n", f.ident, f)
			fmt.Fprintf(out, "\tfor _, v := range %s {\n", itemsExpr)
			fmt.Fprintf(out, "\t\tif v.%s() == \"\" { return cambiumValidationError(%s, %q) }\n", method, pathExpr, message)
			out.WriteString("\t}\n")
		}
	}

	if f.isUnion {
		pathExpr := fmt.Sprintf("cambiumJoinPath(path, %q)", wire)
		switch f.node.Kind() {
		case cambium.SchemaNodeKindLeaf:
			if f.optional {
				fmt.Fprintf(out, "\tif n.%s != nil {\n", f.ident)
				fmt.Fprintf(out, "\t\tif err := n.%s.validate(%s); err != nil { return err }\n", f.ident, pathExpr)
				out.WriteString("\t}\n")
			} else {
				fmt.Fprintf(out, "\tif err := n.%s.validate(%s); err != nil { return err }\n", f.ident, pathExpr)
			}
		case cambium.SchemaNodeKindLeafList:
			itemsExpr := fieldItemsExpr("n", f.ident, f)
			fmt.Fprintf(out, "\tfor _, v := range %s {\n", itemsExpr)
			fmt.Fprintf(out, "\t\tif err := v.validate(%s); err != nil { return err }\n", pathExpr)
			out.WriteString("\t}\n")
		}
	}

	if f.node.Kind() == cambium.SchemaNodeKindList {
		g.emitListKeyValidation(f, out)
		g.emitUniqueValidation(f, out)
		itemsExpr := fieldItemsExpr("n", f.ident, f)
		fmt.Fprintf(out, "\tfor i := range %s {\n", itemsExpr)
		fmt.Fprintf(out, "\t\tif err := %s[i].validate(cambiumJoinPath(path, %q)); err != nil { return err }\n", itemsExpr, wire)
		out.WriteString("\t}\n")
		return
	}
	if f.node.Kind() == cambium.SchemaNodeKindLeafList && requiresLeafListValueUniqueness(f.node) {
		g.emitLeafListUniqueValueValidation(f, out)
	}

	switch f.node.Kind() {
	case cambium.SchemaNodeKindContainer, cambium.SchemaNodeKindAction, cambium.SchemaNodeKindNotification:
		if f.node.IsChoiceDescendant() {
			return
		}
		if f.optional {
			fmt.Fprintf(out, "\tif n.%s != nil {\n", f.ident)
			fmt.Fprintf(out, "\t\tif err := n.%s.validate(cambiumJoinPath(path, %q)); err != nil { return err }\n", f.ident, wire)
			out.WriteString("\t}\n")
		} else {
			fmt.Fprintf(out, "\tif err := n.%s.validate(cambiumJoinPath(path, %q)); err != nil { return err }\n", f.ident, wire)
		}
	}
}

func requiresLeafListValueUniqueness(node cambium.SchemaNodeRef) bool {
	return node.RepresentsConfigurationData() || node.YangVersion() != "1.1"
}

func (g *goEmitter) emitLeafListUniqueValueValidation(f fieldInfo, out *strings.Builder) {
	g.emitLeafListUniqueValueValidationAt(f, "n", fmt.Sprintf("cambiumJoinPath(path, %q)", f.wire), out, "\t")
}

func (g *goEmitter) emitLeafListUniqueValueValidationAt(f fieldInfo, owner, pathExpr string, out *strings.Builder, indent string) {
	itemsExpr := fieldItemsExpr(owner, f.ident, f)
	seenName := fmt.Sprintf("seen%sValues", f.ident)
	fmt.Fprintf(out, "%s%s := make(map[string]struct{})\n", indent, seenName)
	fmt.Fprintf(out, "%sfor _, value := range %s {\n", indent, itemsExpr)
	keyExpr := g.uniqueStringExpr("value", f)
	fmt.Fprintf(out, "%s\tkey := %s\n", indent, keyExpr)
	fmt.Fprintf(out, "%s\tif _, ok := %s[key]; ok {\n", indent, seenName)
	fmt.Fprintf(out, "%s\t\treturn cambiumValidationError(%s, %q)\n", indent, pathExpr, "duplicate leaf-list value")
	fmt.Fprintf(out, "%s\t}\n", indent)
	fmt.Fprintf(out, "%s\t%s[key] = struct{}{}\n", indent, seenName)
	fmt.Fprintf(out, "%s}\n", indent)
}

func (g *goEmitter) emitListKeyValidation(f fieldInfo, out *strings.Builder) {
	g.emitListKeyValidationAt(f, "n", fmt.Sprintf("cambiumJoinPath(path, %q)", f.wire), out, "\t")
}

func (g *goEmitter) emitListKeyValidationAt(f fieldInfo, owner, pathExpr string, out *strings.Builder, indent string) {
	keys := g.listKeyFields(f)
	if len(keys) == 0 {
		return
	}
	itemsExpr := fieldItemsExpr(owner, f.ident, f)
	seenName := fmt.Sprintf("seen%sKeys", f.ident)
	fmt.Fprintf(out, "%s%s := make(map[string]struct{})\n", indent, seenName)
	parts := make([]string, 0, len(keys))
	usesEntry := false
	for _, key := range keys {
		if key.jsonKind == "Empty" {
			parts = append(parts, `"[null]"`)
			continue
		}
		usesEntry = true
		parts = append(parts, g.jsonLiteralExpr(uniqueFieldValueExpr("entry", key), key))
	}
	if usesEntry {
		fmt.Fprintf(out, "%sfor _, entry := range %s {\n", indent, itemsExpr)
	} else {
		fmt.Fprintf(out, "%sfor range %s {\n", indent, itemsExpr)
	}
	keyExpr := parts[0]
	if len(parts) > 1 {
		keyExpr = "strings.Join([]string{" + strings.Join(parts, ", ") + `}, "\x00")`
	}
	fmt.Fprintf(out, "%s\tkey := %s\n", indent, keyExpr)
	fmt.Fprintf(out, "%s\tif _, ok := %s[key]; ok {\n", indent, seenName)
	fmt.Fprintf(out, "%s\t\treturn cambiumValidationError(%s, %q)\n", indent, pathExpr, "duplicate key violation")
	fmt.Fprintf(out, "%s\t}\n", indent)
	fmt.Fprintf(out, "%s\t%s[key] = struct{}{}\n", indent, seenName)
	fmt.Fprintf(out, "%s}\n", indent)
}

func (g *goEmitter) emitChoiceValidation(fields []fieldInfo, sourceChildren []cambium.SchemaNodeRef, out *strings.Builder) {
	choices := validationChoices(sourceChildren)
	if len(choices) == 0 {
		return
	}
	byNode := make(map[cambium.SchemaNodeRef]fieldInfo, len(fields))
	for _, f := range fields {
		byNode[f.node] = f
	}
	choiceCounter := 0
	for _, choice := range choices {
		g.emitChoiceValidationForChoice(choice, "n", []string{choice.Name()}, byNode, out, "\t", &choiceCounter)
	}
}

func (g *goEmitter) emitChoiceValidationForChoice(choice cambium.SchemaNodeRef, owner string, path []string, byNode map[cambium.SchemaNodeRef]fieldInfo, out *strings.Builder, indent string, choiceCounter *int) {
	countName := fmt.Sprintf("choiceCount%d", *choiceCounter)
	*choiceCounter++
	type caseCheck struct {
		node cambium.SchemaNodeRef
		expr string
	}
	var caseChecks []caseCheck
	fmt.Fprintf(out, "%s%s := 0\n", indent, countName)
	for c := range choice.Children().Iter() {
		if c.Kind() != cambium.SchemaNodeKindCase {
			continue
		}
		expr := g.choiceCasePresenceExpr(owner, c, byNode)
		caseChecks = append(caseChecks, caseCheck{node: c, expr: expr})
		fmt.Fprintf(out, "%sif %s { %s++ }\n", indent, expr, countName)
	}
	fmt.Fprintf(out, "%sif %s > 1 {\n", indent, countName)
	fmt.Fprintf(out, "%s\treturn cambiumValidationError(%s, %q)\n", indent, choiceCasePathExpr(path), "choice has multiple cases selected")
	fmt.Fprintf(out, "%s}\n", indent)
	if choice.IsMandatory() {
		fmt.Fprintf(out, "%sif %s == 0 {\n", indent, countName)
		fmt.Fprintf(out, "%s\treturn cambiumValidationError(%s, %q)\n", indent, choiceCasePathExpr(path), "mandatory choice has no selected case")
		fmt.Fprintf(out, "%s}\n", indent)
	}
	for _, check := range caseChecks {
		casePath := append(append([]string(nil), path...), check.node.Name())
		caseExpr := check.expr
		if defaultCase, ok := choice.DefaultValue(); ok && defaultCase == check.node.Name() {
			caseExpr = fmt.Sprintf("(%s || %s == 0)", check.expr, countName)
		}
		g.emitChoiceCaseMandatoryValidation(check.node, caseExpr, casePath, owner, byNode, out, indent, choiceCounter)
	}
}

func (g *goEmitter) emitChoiceCaseMandatoryValidation(c cambium.SchemaNodeRef, caseExpr string, path []string, owner string, byNode map[cambium.SchemaNodeRef]fieldInfo, out *strings.Builder, indent string, choiceCounter *int) {
	var checks strings.Builder
	g.emitSelectedCaseDescendantValidation(c.Children(), owner, path, byNode, &checks, indent+"\t", choiceCounter)
	if checks.Len() == 0 {
		return
	}
	fmt.Fprintf(out, "%sif %s {\n", indent, caseExpr)
	out.WriteString(checks.String())
	fmt.Fprintf(out, "%s}\n", indent)
}

func (g *goEmitter) emitSelectedCaseDescendantValidation(children cambium.SchemaChildren, owner string, path []string, byNode map[cambium.SchemaNodeRef]fieldInfo, out *strings.Builder, indent string, choiceCounter *int) {
	for child := range children.Iter() {
		if len(child.Whens()) > 0 {
			continue
		}
		f, hasField := byNode[child]
		switch child.Kind() {
		case cambium.SchemaNodeKindChoice:
			choicePath := append(append([]string(nil), path...), child.Name())
			g.emitChoiceValidationForChoice(child, owner, choicePath, byNode, out, indent, choiceCounter)
		case cambium.SchemaNodeKindLeaf, cambium.SchemaNodeKindAnyData:
			if !hasField {
				continue
			}
			fieldPath := append(append([]string(nil), path...), f.wire)
			if child.Kind() == cambium.SchemaNodeKindLeaf {
				g.emitScalarFieldValidationAt(f, owner, choiceCasePathExpr(fieldPath), out, indent)
			}
			if !child.IsMandatory() {
				continue
			}
			missingExpr := mandatoryFieldMissingExpr(owner, f)
			if missingExpr == "" {
				continue
			}
			fmt.Fprintf(out, "%sif %s {\n", indent, missingExpr)
			fmt.Fprintf(out, "%s\treturn cambiumValidationError(%s, %q)\n", indent, choiceCasePathExpr(fieldPath), "missing mandatory field")
			fmt.Fprintf(out, "%s}\n", indent)
		case cambium.SchemaNodeKindLeafList, cambium.SchemaNodeKindList:
			if !hasField {
				continue
			}
			fieldPath := append(append([]string(nil), path...), f.wire)
			fieldPathExpr := choiceCasePathExpr(fieldPath)
			if min, ok := child.MinElements(); ok && min > 0 {
				fmt.Fprintf(out, "%sif %s < %d {\n", indent, validationLenExpr(owner, f), min)
				fmt.Fprintf(out, "%s\treturn cambiumValidationError(%s, %q)\n", indent, fieldPathExpr, "min-elements violation")
				fmt.Fprintf(out, "%s}\n", indent)
			}
			if max, ok := child.MaxElements(); ok {
				fmt.Fprintf(out, "%sif %s > %d {\n", indent, validationLenExpr(owner, f), max)
				fmt.Fprintf(out, "%s\treturn cambiumValidationError(%s, %q)\n", indent, fieldPathExpr, "max-elements violation")
				fmt.Fprintf(out, "%s}\n", indent)
			}
			if child.Kind() == cambium.SchemaNodeKindLeafList {
				g.emitScalarFieldValidationAt(f, owner, fieldPathExpr, out, indent)
				if requiresLeafListValueUniqueness(child) {
					g.emitLeafListUniqueValueValidationAt(f, owner, fieldPathExpr, out, indent)
				}
				continue
			}
			g.emitListKeyValidationAt(f, owner, fieldPathExpr, out, indent)
			g.emitUniqueValidationAt(f, owner, fieldPathExpr, out, indent)
			entryFields := g.collectFields(fieldConcreteType(f), orderedListChildren(child.DataChildren(true), child.ListKeys()))
			entryByNode := make(map[cambium.SchemaNodeRef]fieldInfo, len(entryFields))
			for _, ef := range entryFields {
				entryByNode[ef.node] = ef
			}
			itemsExpr := fieldItemsExpr(owner, f.ident, f)
			fmt.Fprintf(out, "%sfor i := range %s {\n", indent, itemsExpr)
			entryOwner := fmt.Sprintf("%s[i]", itemsExpr)
			fmt.Fprintf(out, "%s\tif err := %s.validate(%s); err != nil { return err }\n", indent, entryOwner, fieldPathExpr)
			g.emitSelectedCaseDescendantValidation(child.Children(), entryOwner, fieldPath, entryByNode, out, indent+"\t", choiceCounter)
			fmt.Fprintf(out, "%s}\n", indent)
		case cambium.SchemaNodeKindContainer:
			if !hasField {
				continue
			}
			nextOwner := owner + "." + f.ident
			nextPath := append(append([]string(nil), path...), f.wire)
			nextPathExpr := choiceCasePathExpr(nextPath)
			nestedFields := g.collectFields(fieldConcreteType(f), g.orderedChildrenList(child.DataChildren(true)))
			nestedByNode := make(map[cambium.SchemaNodeRef]fieldInfo, len(nestedFields))
			for _, nf := range nestedFields {
				nestedByNode[nf.node] = nf
			}
			if f.optional {
				fmt.Fprintf(out, "%sif %s != nil {\n", indent, nextOwner)
				fmt.Fprintf(out, "%s\tif err := %s.validate(%s); err != nil { return err }\n", indent, nextOwner, nextPathExpr)
				g.emitSelectedCaseDescendantValidation(child.Children(), nextOwner, nextPath, nestedByNode, out, indent+"\t", choiceCounter)
				fmt.Fprintf(out, "%s}\n", indent)
				continue
			}
			fmt.Fprintf(out, "%sif err := %s.validate(%s); err != nil { return err }\n", indent, nextOwner, nextPathExpr)
			g.emitSelectedCaseDescendantValidation(child.Children(), nextOwner, nextPath, nestedByNode, out, indent, choiceCounter)
		}
	}
}

func (g *goEmitter) emitScalarFieldValidationAt(f fieldInfo, owner, pathExpr string, out *strings.Builder, indent string) {
	base := fieldConcreteType(f)
	ref := owner + "." + f.ident
	if _, ok := g.intRangeTypes[base]; ok {
		switch f.node.Kind() {
		case cambium.SchemaNodeKindLeaf:
			if f.optional {
				fmt.Fprintf(out, "%sif %s != nil {\n", indent, ref)
				fmt.Fprintf(out, "%s\tif _, err := New%s(%s.Get()); err != nil { return cambiumValidationError(%s, err.Error()) }\n", indent, base, ref, pathExpr)
				fmt.Fprintf(out, "%s}\n", indent)
			} else {
				fmt.Fprintf(out, "%sif _, err := New%s(%s.Get()); err != nil { return cambiumValidationError(%s, err.Error()) }\n", indent, base, ref, pathExpr)
			}
		case cambium.SchemaNodeKindLeafList:
			itemsExpr := fieldItemsExpr(owner, f.ident, f)
			fmt.Fprintf(out, "%sfor _, v := range %s {\n", indent, itemsExpr)
			fmt.Fprintf(out, "%s\tif _, err := New%s(v.Get()); err != nil { return cambiumValidationError(%s, err.Error()) }\n", indent, base, pathExpr)
			fmt.Fprintf(out, "%s}\n", indent)
		}
	}
	if _, ok := g.stringLengthTypes[base]; ok {
		switch f.node.Kind() {
		case cambium.SchemaNodeKindLeaf:
			if f.optional {
				fmt.Fprintf(out, "%sif %s != nil {\n", indent, ref)
				fmt.Fprintf(out, "%s\tif _, err := New%s(%s.String()); err != nil { return cambiumValidationError(%s, err.Error()) }\n", indent, base, ref, pathExpr)
				fmt.Fprintf(out, "%s}\n", indent)
			} else {
				fmt.Fprintf(out, "%sif _, err := New%s(%s.String()); err != nil { return cambiumValidationError(%s, err.Error()) }\n", indent, base, ref, pathExpr)
			}
		case cambium.SchemaNodeKindLeafList:
			itemsExpr := fieldItemsExpr(owner, f.ident, f)
			fmt.Fprintf(out, "%sfor _, v := range %s {\n", indent, itemsExpr)
			fmt.Fprintf(out, "%s\tif _, err := New%s(v.String()); err != nil { return cambiumValidationError(%s, err.Error()) }\n", indent, base, pathExpr)
			fmt.Fprintf(out, "%s}\n", indent)
		}
	}

	g.emitStringValueValidationAt(f, owner, pathExpr, out, indent)

	if info, ok := stringPatternInfo(f.node); ok {
		g.helpers["patternValidate"] = true
		patterns := stringPatternLiteral(info)
		_, isLengthType := g.stringLengthTypes[base]
		switch f.node.Kind() {
		case cambium.SchemaNodeKindLeaf:
			if f.optional {
				valueExpr := "*" + ref
				if isLengthType {
					valueExpr = ref + ".String()"
				}
				fmt.Fprintf(out, "%sif %s != nil {\n", indent, ref)
				fmt.Fprintf(out, "%s\tif err := cambiumValidateStringPatterns(%s, %s); err != nil { return cambiumValidationError(%s, err.Error()) }\n", indent, valueExpr, patterns, pathExpr)
				fmt.Fprintf(out, "%s}\n", indent)
			} else {
				valueExpr := ref
				if isLengthType {
					valueExpr = ref + ".String()"
				}
				fmt.Fprintf(out, "%sif err := cambiumValidateStringPatterns(%s, %s); err != nil { return cambiumValidationError(%s, err.Error()) }\n", indent, valueExpr, patterns, pathExpr)
			}
		case cambium.SchemaNodeKindLeafList:
			itemsExpr := fieldItemsExpr(owner, f.ident, f)
			valueExpr := "v"
			if isLengthType {
				valueExpr = "v.String()"
			}
			fmt.Fprintf(out, "%sfor _, v := range %s {\n", indent, itemsExpr)
			fmt.Fprintf(out, "%s\tif err := cambiumValidateStringPatterns(%s, %s); err != nil { return cambiumValidationError(%s, err.Error()) }\n", indent, valueExpr, patterns, pathExpr)
			fmt.Fprintf(out, "%s}\n", indent)
		}
	}

	if info, ok := binaryTypeInfo(f.node); ok {
		g.helpers["binaryParse"] = true
		ranges := binaryLengthRangeLiteral(info)
		switch f.node.Kind() {
		case cambium.SchemaNodeKindLeaf:
			if f.optional {
				fmt.Fprintf(out, "%sif %s != nil {\n", indent, ref)
				fmt.Fprintf(out, "%s\tif err := cambiumValidateBinaryString(*%s, %s); err != nil { return cambiumValidationError(%s, err.Error()) }\n", indent, ref, ranges, pathExpr)
				fmt.Fprintf(out, "%s}\n", indent)
			} else {
				fmt.Fprintf(out, "%sif err := cambiumValidateBinaryString(%s, %s); err != nil { return cambiumValidationError(%s, err.Error()) }\n", indent, ref, ranges, pathExpr)
			}
		case cambium.SchemaNodeKindLeafList:
			itemsExpr := fieldItemsExpr(owner, f.ident, f)
			fmt.Fprintf(out, "%sfor _, v := range %s {\n", indent, itemsExpr)
			fmt.Fprintf(out, "%s\tif err := cambiumValidateBinaryString(v, %s); err != nil { return cambiumValidationError(%s, err.Error()) }\n", indent, ranges, pathExpr)
			fmt.Fprintf(out, "%s}\n", indent)
		}
	}

	if info, ok := decimal64TypeInfo(f.node); ok {
		fd, ranges := decimal64ValidationArgs(info)
		switch f.node.Kind() {
		case cambium.SchemaNodeKindLeaf:
			if f.optional {
				fmt.Fprintf(out, "%sif %s != nil {\n", indent, ref)
				fmt.Fprintf(out, "%s\tif err := cambiumValidateDecimal64Value(*%s, %d, %s); err != nil { return cambiumValidationError(%s, err.Error()) }\n", indent, ref, fd, ranges, pathExpr)
				fmt.Fprintf(out, "%s}\n", indent)
			} else {
				fmt.Fprintf(out, "%sif err := cambiumValidateDecimal64Value(%s, %d, %s); err != nil { return cambiumValidationError(%s, err.Error()) }\n", indent, ref, fd, ranges, pathExpr)
			}
		case cambium.SchemaNodeKindLeafList:
			itemsExpr := fieldItemsExpr(owner, f.ident, f)
			fmt.Fprintf(out, "%sfor _, v := range %s {\n", indent, itemsExpr)
			fmt.Fprintf(out, "%s\tif err := cambiumValidateDecimal64Value(v, %d, %s); err != nil { return cambiumValidationError(%s, err.Error()) }\n", indent, fd, ranges, pathExpr)
			fmt.Fprintf(out, "%s}\n", indent)
		}
	}

	if f.isEnum || f.isIdentityref {
		method := "AsName"
		message := "invalid enum value"
		if f.isIdentityref {
			method = "AsJSONName"
			message = "invalid identityref value"
		}
		switch f.node.Kind() {
		case cambium.SchemaNodeKindLeaf:
			if f.optional {
				fmt.Fprintf(out, "%sif %s != nil && %s.%s() == \"\" { return cambiumValidationError(%s, %q) }\n", indent, ref, ref, method, pathExpr, message)
			} else {
				fmt.Fprintf(out, "%sif %s.%s() == \"\" { return cambiumValidationError(%s, %q) }\n", indent, ref, method, pathExpr, message)
			}
		case cambium.SchemaNodeKindLeafList:
			itemsExpr := fieldItemsExpr(owner, f.ident, f)
			fmt.Fprintf(out, "%sfor _, v := range %s {\n", indent, itemsExpr)
			fmt.Fprintf(out, "%s\tif v.%s() == \"\" { return cambiumValidationError(%s, %q) }\n", indent, method, pathExpr, message)
			fmt.Fprintf(out, "%s}\n", indent)
		}
	}

	if f.isUnion {
		switch f.node.Kind() {
		case cambium.SchemaNodeKindLeaf:
			if f.optional {
				fmt.Fprintf(out, "%sif %s != nil {\n", indent, ref)
				fmt.Fprintf(out, "%s\tif err := %s.validate(%s); err != nil { return err }\n", indent, ref, pathExpr)
				fmt.Fprintf(out, "%s}\n", indent)
			} else {
				fmt.Fprintf(out, "%sif err := %s.validate(%s); err != nil { return err }\n", indent, ref, pathExpr)
			}
		case cambium.SchemaNodeKindLeafList:
			itemsExpr := fieldItemsExpr(owner, f.ident, f)
			fmt.Fprintf(out, "%sfor _, v := range %s {\n", indent, itemsExpr)
			fmt.Fprintf(out, "%s\tif err := v.validate(%s); err != nil { return err }\n", indent, pathExpr)
			fmt.Fprintf(out, "%s}\n", indent)
		}
	}
}

func mandatoryFieldMissingExpr(owner string, f fieldInfo) string {
	ref := owner + "." + f.ident
	switch f.node.Kind() {
	case cambium.SchemaNodeKindLeaf, cambium.SchemaNodeKindAnyData:
		if f.optional {
			return ref + " == nil"
		}
	}
	return ""
}

func choiceCasePathExpr(path []string) string {
	pathExpr := "path"
	for _, elem := range path {
		pathExpr = fmt.Sprintf("cambiumJoinPath(%s, %q)", pathExpr, elem)
	}
	return pathExpr
}

func validationChoices(children []cambium.SchemaNodeRef) []cambium.SchemaNodeRef {
	var out []cambium.SchemaNodeRef
	for _, child := range children {
		if child.Kind() == cambium.SchemaNodeKindChoice {
			out = append(out, child)
		}
	}
	return out
}

func (g *goEmitter) choiceCasePresenceExpr(owner string, c cambium.SchemaNodeRef, byNode map[cambium.SchemaNodeRef]fieldInfo) string {
	var exprs []string
	for child := range c.DataChildren(true).Iter() {
		if f, ok := byNode[child]; ok {
			exprs = append(exprs, fieldPresentExpr(owner, f))
		}
	}
	if len(exprs) == 0 {
		return "false"
	}
	return strings.Join(exprs, " || ")
}

func (g *goEmitter) choiceDefaultEmissionGuard(owner string, f fieldInfo, byNode map[cambium.SchemaNodeRef]fieldInfo) string {
	choice, caseNode, ok := nearestChoiceCase(f.node)
	if !ok {
		return ""
	}
	caseExpr := g.choiceCasePresenceExpr(owner, caseNode, byNode)
	defaultCase, hasDefaultCase := choice.DefaultValue()
	if !hasDefaultCase {
		return caseExpr
	}
	if defaultCase != caseNode.Name() {
		return caseExpr
	}
	anyCaseExpr := g.choiceAnyCasePresenceExpr(owner, choice, byNode)
	return "(" + caseExpr + " || !(" + anyCaseExpr + "))"
}

func nearestChoiceCase(node cambium.SchemaNodeRef) (cambium.SchemaNodeRef, cambium.SchemaNodeRef, bool) {
	ancestors := node.Ancestors()
	for i := len(ancestors) - 1; i >= 0; i-- {
		if ancestors[i].Kind() != cambium.SchemaNodeKindCase {
			continue
		}
		for j := i - 1; j >= 0; j-- {
			if ancestors[j].Kind() == cambium.SchemaNodeKindChoice {
				return ancestors[j], ancestors[i], true
			}
		}
	}
	return cambium.SchemaNodeRef{}, cambium.SchemaNodeRef{}, false
}

func (g *goEmitter) choiceAnyCasePresenceExpr(owner string, choice cambium.SchemaNodeRef, byNode map[cambium.SchemaNodeRef]fieldInfo) string {
	var exprs []string
	for c := range choice.Children().Iter() {
		if c.Kind() != cambium.SchemaNodeKindCase {
			continue
		}
		expr := g.choiceCasePresenceExpr(owner, c, byNode)
		if expr != "false" {
			exprs = append(exprs, expr)
		}
	}
	if len(exprs) == 0 {
		return "false"
	}
	return strings.Join(exprs, " || ")
}

func fieldPresentExpr(owner string, f fieldInfo) string {
	ref := owner + "." + f.ident
	switch f.node.Kind() {
	case cambium.SchemaNodeKindLeaf:
		if f.optional {
			return ref + " != nil"
		}
		return "true"
	case cambium.SchemaNodeKindLeafList, cambium.SchemaNodeKindList:
		return validationLenExpr(owner, f) + " > 0"
	case cambium.SchemaNodeKindAnyData:
		if f.optional {
			return ref + " != nil"
		}
		return "true"
	case cambium.SchemaNodeKindContainer:
		if f.optional {
			return ref + " != nil"
		}
		return ref + ".hasContent()"
	case cambium.SchemaNodeKindAction, cambium.SchemaNodeKindNotification:
		return ref + " != nil"
	default:
		return "false"
	}
}

func validationLenExpr(owner string, f fieldInfo) string {
	if strings.HasPrefix(f.goType, "UserOrderedVec[") {
		return owner + "." + f.ident + ".Len()"
	}
	return "len(" + owner + "." + f.ident + ")"
}

func (g *goEmitter) emitUniqueValidation(f fieldInfo, out *strings.Builder) {
	g.emitUniqueValidationAt(f, "n", fmt.Sprintf("cambiumJoinPath(path, %q)", f.wire), out, "\t")
}

func (g *goEmitter) emitUniqueValidationAt(f fieldInfo, owner, pathExpr string, out *strings.Builder, indent string) {
	uniques := f.node.UniqueConstraints()
	if len(uniques) == 0 {
		return
	}
	itemsExpr := fieldItemsExpr(owner, f.ident, f)
	for i, unique := range uniques {
		var accessors []uniqueLeafAccessor
		for _, leaf := range unique.Leafs() {
			accessor, ok := g.uniqueLeafAccessor(f, leaf)
			if !ok {
				g.recordEmitError(fmt.Errorf("unsupported unique leaf path %s", leaf.Path()))
				continue
			}
			accessors = append(accessors, accessor)
		}
		if len(accessors) == 0 {
			continue
		}
		seenName := fmt.Sprintf("seen%sUnique%d", f.ident, i)
		fmt.Fprintf(out, "%s%s := make(map[string]struct{})\n", indent, seenName)
		fmt.Fprintf(out, "%sfor _, entry := range %s {\n", indent, itemsExpr)
		conds := make([]string, 0, len(accessors))
		parts := make([]string, 0, len(accessors))
		for _, accessor := range accessors {
			if accessor.presentExpr != "true" {
				conds = append(conds, accessor.presentExpr)
			}
			parts = append(parts, accessor.keyExpr)
		}
		cond := "true"
		if len(conds) > 0 {
			cond = strings.Join(conds, " && ")
		}
		fmt.Fprintf(out, "%s\tif %s {\n", indent, cond)
		keyExpr := parts[0]
		if len(parts) > 1 {
			keyExpr = "strings.Join([]string{" + strings.Join(parts, ", ") + `}, "\x00")`
		}
		fmt.Fprintf(out, "%s\t\tkey := %s\n", indent, keyExpr)
		fmt.Fprintf(out, "%s\t\tif _, ok := %s[key]; ok {\n", indent, seenName)
		fmt.Fprintf(out, "%s\t\t\treturn cambiumValidationError(%s, %q)\n", indent, pathExpr, "unique violation")
		fmt.Fprintf(out, "%s\t\t}\n", indent)
		fmt.Fprintf(out, "%s\t\t%s[key] = struct{}{}\n", indent, seenName)
		fmt.Fprintf(out, "%s\t}\n", indent)
		fmt.Fprintf(out, "%s}\n", indent)
	}
}

type uniqueLeafAccessor struct {
	field       fieldInfo
	presentExpr string
	keyExpr     string
}

func (g *goEmitter) uniqueLeafAccessor(listField fieldInfo, leaf cambium.SchemaNodeRef) (uniqueLeafAccessor, bool) {
	path, ok := uniqueLeafPath(listField.node, leaf)
	if !ok || len(path) == 0 {
		return uniqueLeafAccessor{}, false
	}
	owner := "entry"
	presentConds := []string{}
	fields := g.collectFields(fieldConcreteType(listField), orderedListChildren(listField.node.DataChildren(true), listField.node.ListKeys()))
	for i, node := range path {
		if node.Kind() == cambium.SchemaNodeKindChoice || node.Kind() == cambium.SchemaNodeKindCase {
			continue
		}
		field, ok := findFieldForNode(fields, node)
		if !ok {
			return uniqueLeafAccessor{}, false
		}
		if i == len(path)-1 {
			present, keyExpr := g.uniqueFieldEffectivePresenceAndKey(owner, field, fields)
			if present != "true" {
				presentConds = append(presentConds, present)
			}
			presentExpr := "true"
			if len(presentConds) > 0 {
				presentExpr = strings.Join(presentConds, " && ")
			}
			return uniqueLeafAccessor{
				field:       field,
				presentExpr: presentExpr,
				keyExpr:     keyExpr,
			}, true
		}
		if node.Kind() != cambium.SchemaNodeKindContainer {
			return uniqueLeafAccessor{}, false
		}
		ref := owner + "." + field.ident
		if field.optional {
			presentConds = append(presentConds, ref+" != nil")
		}
		owner = ref
		fields = g.collectFields(fieldConcreteType(field), g.orderedChildrenList(node.DataChildren(true)))
	}
	return uniqueLeafAccessor{}, false
}

func (g *goEmitter) uniqueFieldEffectivePresenceAndKey(owner string, f fieldInfo, fields []fieldInfo) (string, string) {
	actualPresent := uniqueFieldPresentExpr(owner, f)
	actualKey := g.jsonLiteralExpr(uniqueFieldValueExpr(owner, f), f)
	defaultValue, hasDefault := f.node.DefaultEntry()
	if !hasDefault || !f.optional {
		return actualPresent, actualKey
	}
	defaultPresent := g.choiceDefaultEmissionGuard(owner, f, fieldsByNode(fields))
	if defaultPresent == "" {
		defaultPresent = "true"
	}
	effectivePresent := actualPresent
	if actualPresent != "true" {
		effectivePresent = "(" + actualPresent + " || " + defaultPresent + ")"
	}
	defaultKey := g.defaultJSONLiteralExpr(f, defaultValue)
	keyExpr := fmt.Sprintf("func() string { if %s { return %s }; return %s }()", actualPresent, actualKey, defaultKey)
	return effectivePresent, keyExpr
}

func uniqueLeafPath(list, leaf cambium.SchemaNodeRef) ([]cambium.SchemaNodeRef, bool) {
	ancestors := leaf.Ancestors()
	for i, ancestor := range ancestors {
		if ancestor == list {
			path := append([]cambium.SchemaNodeRef(nil), ancestors[i+1:]...)
			path = append(path, leaf)
			return path, true
		}
	}
	return nil, false
}

func uniqueFieldPresentExpr(owner string, f fieldInfo) string {
	if f.optional {
		return owner + "." + f.ident + " != nil"
	}
	return "true"
}

func uniqueFieldValueExpr(owner string, f fieldInfo) string {
	ref := owner + "." + f.ident
	if f.optional && !f.isUnion {
		return "(*" + ref + ")"
	}
	return ref
}

func (g *goEmitter) uniqueStringExpr(valueRef string, f fieldInfo) string {
	base := scalarBaseType(f)
	if f.isUnion {
		return valueRef + ".String()"
	}
	if f.isIdentityref {
		return valueRef + ".AsJSONName()"
	}
	switch f.jsonKind {
	case "InstanceIdentifier":
		return valueRef + ".String()"
	case "String":
		if base == "string" {
			return valueRef
		}
		return valueRef + ".String()"
	case "Bool":
		g.helpers["strconv"] = true
		return "strconv.FormatBool(" + valueRef + ")"
	case "BareNumber", "QuotedNumber":
		if f.isNewtype || base == "Decimal64" {
			return valueRef + ".String()"
		}
		g.helpers["strconv"] = true
		if f.intSigned {
			return "strconv.FormatInt(int64(" + valueRef + "), 10)"
		}
		return "strconv.FormatUint(uint64(" + valueRef + "), 10)"
	case "Empty":
		return `""`
	default:
		return valueRef + ".String()"
	}
}

func (g *goEmitter) operationSourceValidationChildren(source cambium.SchemaNodeRef) []cambium.SchemaNodeRef {
	if source.Kind() != cambium.SchemaNodeKindUnknown {
		return schemaChildrenList(source.Children())
	}
	return nil
}

func (g *goEmitter) validationSourceChildren(node cambium.SchemaNodeRef) []cambium.SchemaNodeRef {
	if node.Kind() == cambium.SchemaNodeKindRPC || node.Kind() == cambium.SchemaNodeKindAction {
		source := g.operationPayloadSource(node)
		if source.Name() != "" {
			return schemaChildrenList(source.Children())
		}
	}
	return schemaChildrenList(node.Children())
}

func (g *goEmitter) emitExplicitChoiceCaseParseValidation(owner string, f fieldInfo, byNode map[cambium.SchemaNodeRef]fieldInfo, out *strings.Builder, indent string) {
	if !f.node.IsChoiceDescendant() {
		return
	}
	_, caseNode, ok := nearestChoiceCase(f.node)
	if !ok {
		return
	}
	presenceExpr := fieldPresentExpr(owner, f)
	if presenceExpr == "true" {
		return
	}
	casePath := schemaPathElements(caseNode)
	if len(casePath) == 0 {
		return
	}
	var checks strings.Builder
	choiceCounter := 0
	g.emitSelectedCaseDescendantValidation(caseNode.Children(), owner, casePath, byNode, &checks, indent+"\t", &choiceCounter)
	if checks.Len() == 0 {
		return
	}
	fmt.Fprintf(out, "%sif !(%s) {\n", indent, presenceExpr)
	fmt.Fprintf(out, "%s\tpath := \"\"\n", indent)
	out.WriteString(checks.String())
	fmt.Fprintf(out, "%s}\n", indent)
}

func decimal64ValidationArgs(info cambium.TypeInfo) (uint8, string) {
	r, ok := info.Resolved().(cambium.ResolvedDecimal64)
	if !ok {
		return 0, "nil"
	}
	fd := r.FractionDigits().Value()
	if len(r.Range) == 0 {
		return fd, "nil"
	}
	parts := make([]string, 0, len(r.Range))
	for _, bound := range r.Range {
		parts = append(parts, fmt.Sprintf("{min: %d, max: %d}", parseDecimal64BoundRaw(bound.Min(), fd), parseDecimal64BoundRaw(bound.Max(), fd)))
	}
	return fd, "[]cambiumDecimal64Range{" + strings.Join(parts, ", ") + "}"
}
