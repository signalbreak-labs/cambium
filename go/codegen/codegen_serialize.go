package codegen

import (
	"encoding/base64"
	"fmt"
	"math/big"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/signalbreak-labs/cambium/go/cambium"
	"github.com/signalbreak-labs/cambium/go/internal/xsdregex"
)

func (g *goEmitter) emitFieldXML(f fieldInfo, currentNS string, out *strings.Builder) {
	wire := f.wire
	ident := f.ident
	nsAttr := nestedXMLNSAttr(f, currentNS)

	switch f.node.Kind() {
	case cambium.SchemaNodeKindLeaf:
		if f.jsonKind == "Empty" {
			if f.optional {
				fmt.Fprintf(out, "\tif n.%s != nil {\n", ident)
				fmt.Fprintf(out, "\t\tb.WriteString(indent + \"<%s%s\" + cambiumMetadataXMLAttrs(n.CambiumMetadata[%q]) + \"/>\\n\")\n", wire, nsAttr, wire)
				out.WriteString("\t}\n")
			} else {
				fmt.Fprintf(out, "\tb.WriteString(indent + \"<%s%s\" + cambiumMetadataXMLAttrs(n.CambiumMetadata[%q]) + \"/>\\n\")\n", wire, nsAttr, wire)
			}
			return
		}
		valueExpr := fmt.Sprintf("n.%s", ident)
		if f.optional {
			fmt.Fprintf(out, "\tif n.%s != nil {\n", ident)
			if f.isUnion {
				g.emitLeafValue(wire, f, "n."+ident, currentNS, out)
			} else {
				g.emitLeafValue(wire, f, "(*n."+ident+")", currentNS, out)
			}
			out.WriteString("\t}\n")
		} else {
			g.emitLeafValue(wire, f, valueExpr, currentNS, out)
		}
	case cambium.SchemaNodeKindLeafList:
		itemsExpr := g.emitCollectionItems("n", f, out, "\t")
		fmt.Fprintf(out, "\tfor _, v := range %s {\n", itemsExpr)
		g.emitLeafListValue(wire, f, currentNS, out)
		out.WriteString("\t}\n")
	case cambium.SchemaNodeKindAnyData:
		if f.optional {
			fmt.Fprintf(out, "\tif n.%s != nil {\n", ident)
			g.emitAnyDataXML(wire, f, "(*n."+ident+")", currentNS, out)
			out.WriteString("\t}\n")
		} else {
			g.emitAnyDataXML(wire, f, "n."+ident, currentNS, out)
		}
	case cambium.SchemaNodeKindContainer, cambium.SchemaNodeKindAction, cambium.SchemaNodeKindNotification:
		if f.optional {
			fmt.Fprintf(out, "\tif n.%s != nil {\n", ident)
			fmt.Fprintf(out, "\t\tif n.%s.hasContent() {\n", ident)
			fmt.Fprintf(out, "\t\t\tb.WriteString(indent + \"<%s%s>\\n\")\n", wire, nsAttr)
			fmt.Fprintf(out, "\t\t\tn.%s.writeXML(b, depth+1)\n", ident)
			fmt.Fprintf(out, "\t\t\tb.WriteString(indent + \"</%s>\\n\")\n", wire)
			out.WriteString("\t\t} else {\n")
			fmt.Fprintf(out, "\t\t\tb.WriteString(indent + \"<%s%s/>\\n\")\n", wire, nsAttr)
			out.WriteString("\t\t}\n")
			out.WriteString("\t}\n")
		} else {
			fmt.Fprintf(out, "\tif n.%s.hasContent() {\n", ident)
			fmt.Fprintf(out, "\t\tb.WriteString(indent + \"<%s%s>\\n\")\n", wire, nsAttr)
			fmt.Fprintf(out, "\t\tn.%s.writeXML(b, depth+1)\n", ident)
			fmt.Fprintf(out, "\t\tb.WriteString(indent + \"</%s>\\n\")\n", wire)
			out.WriteString("\t}\n")
		}
	case cambium.SchemaNodeKindList:
		itemsExpr := g.emitCollectionItems("n", f, out, "\t")
		fmt.Fprintf(out, "\tfor _, entry := range %s {\n", itemsExpr)
		fmt.Fprintf(out, "\t\tb.WriteString(indent + \"<%s%s>\\n\")\n", wire, nsAttr)
		out.WriteString("\t\tentry.writeXML(b, depth+1)\n")
		fmt.Fprintf(out, "\t\tb.WriteString(indent + \"</%s>\\n\")\n", wire)
		out.WriteString("\t}\n")
	}
}

func (g *goEmitter) emitLeafValue(wire string, f fieldInfo, valueExpr, currentNS string, out *strings.Builder) {
	nsAttr := nestedXMLNSAttr(f, currentNS)
	if f.isIdentityref {
		g.helpers["xmlEscape"] = true
		fmt.Fprintf(out, "\t\tif pfx, ns, ok := %s.XMLPrefixNS(); ok && cambiumValidateXMLPrefix(pfx) == nil {\n", valueExpr)
		fmt.Fprintf(out, "\t\t\tb.WriteString(indent + \"<%s%s\" + cambiumMetadataXMLAttrs(n.CambiumMetadata[%q]) + \" xmlns:\" + pfx + \"=\\\"\" + cambiumXMLEscapeAttr(ns) + \"\\\">\" + %s.XMLValue() + \"</%s>\\n\")\n", wire, nsAttr, wire, valueExpr, wire)
		out.WriteString("\t\t} else {\n")
		fmt.Fprintf(out, "\t\t\tb.WriteString(indent + \"<%s%s\" + cambiumMetadataXMLAttrs(n.CambiumMetadata[%q]) + \">\" + %s.XMLValue() + \"</%s>\\n\")\n", wire, nsAttr, wire, valueExpr, wire)
		out.WriteString("\t\t}\n")
		return
	}
	if f.jsonKind == "InstanceIdentifier" {
		g.emitInstanceIdentifierLeafValue(wire, valueExpr, nsAttr, out)
		return
	}
	if f.isUnion {
		g.helpers["xmlEscape"] = true
		fmt.Fprintf(out, "\t\tif pfx, ns, ok := %s.XMLPrefixNS(); ok && cambiumValidateXMLPrefix(pfx) == nil {\n", valueExpr)
		fmt.Fprintf(out, "\t\t\tb.WriteString(indent + \"<%s%s\" + cambiumMetadataXMLAttrs(n.CambiumMetadata[%q]) + \" xmlns:\" + pfx + \"=\\\"\" + cambiumXMLEscapeAttr(ns) + \"\\\">\" + %s.XMLValue() + \"</%s>\\n\")\n", wire, nsAttr, wire, valueExpr, wire)
		out.WriteString("\t\t} else {\n")
		fmt.Fprintf(out, "\t\t\tb.WriteString(indent + \"<%s%s\" + cambiumMetadataXMLAttrs(n.CambiumMetadata[%q]) + \">\" + %s.XMLValue() + \"</%s>\\n\")\n", wire, nsAttr, wire, valueExpr, wire)
		out.WriteString("\t\t}\n")
		return
	}
	expr := g.xmlValueExpr(valueExpr, f)
	if strings.Contains(expr, "cambiumXMLEscapeText") {
		g.helpers["xmlEscape"] = true
	}
	fmt.Fprintf(out, "\tb.WriteString(indent + \"<%s%s\" + cambiumMetadataXMLAttrs(n.CambiumMetadata[%q]) + \">\" + %s + \"</%s>\\n\")\n", wire, nsAttr, wire, expr, wire)
}

func (g *goEmitter) emitLeafListValue(wire string, f fieldInfo, currentNS string, out *strings.Builder) {
	nsAttr := nestedXMLNSAttr(f, currentNS)
	if f.isIdentityref {
		g.helpers["xmlEscape"] = true
		fmt.Fprintf(out, "\t\tif pfx, ns, ok := v.XMLPrefixNS(); ok && cambiumValidateXMLPrefix(pfx) == nil {\n")
		fmt.Fprintf(out, "\t\t\tb.WriteString(indent + \"<%s%s\" + cambiumMetadataXMLAttrs(n.CambiumMetadata[%q]) + \" xmlns:\" + pfx + \"=\\\"\" + cambiumXMLEscapeAttr(ns) + \"\\\">\" + v.XMLValue() + \"</%s>\\n\")\n", wire, nsAttr, wire, wire)
		out.WriteString("\t\t} else {\n")
		fmt.Fprintf(out, "\t\t\tb.WriteString(indent + \"<%s%s\" + cambiumMetadataXMLAttrs(n.CambiumMetadata[%q]) + \">\" + v.XMLValue() + \"</%s>\\n\")\n", wire, nsAttr, wire, wire)
		out.WriteString("\t\t}\n")
		return
	}
	if f.jsonKind == "InstanceIdentifier" {
		g.emitInstanceIdentifierLeafValue(wire, "v", nsAttr, out)
		return
	}
	if f.isUnion {
		g.helpers["xmlEscape"] = true
		fmt.Fprintf(out, "\t\tif pfx, ns, ok := v.XMLPrefixNS(); ok && cambiumValidateXMLPrefix(pfx) == nil {\n")
		fmt.Fprintf(out, "\t\t\tb.WriteString(indent + \"<%s%s\" + cambiumMetadataXMLAttrs(n.CambiumMetadata[%q]) + \" xmlns:\" + pfx + \"=\\\"\" + cambiumXMLEscapeAttr(ns) + \"\\\">\" + v.XMLValue() + \"</%s>\\n\")\n", wire, nsAttr, wire, wire)
		out.WriteString("\t\t} else {\n")
		fmt.Fprintf(out, "\t\t\tb.WriteString(indent + \"<%s%s\" + cambiumMetadataXMLAttrs(n.CambiumMetadata[%q]) + \">\" + v.XMLValue() + \"</%s>\\n\")\n", wire, nsAttr, wire, wire)
		out.WriteString("\t\t}\n")
		return
	}
	expr := g.xmlValueExpr("v", f)
	if strings.Contains(expr, "cambiumXMLEscapeText") {
		g.helpers["xmlEscape"] = true
	}
	fmt.Fprintf(out, "\t\tb.WriteString(indent + \"<%s%s\" + cambiumMetadataXMLAttrs(n.CambiumMetadata[%q]) + \">\" + %s + \"</%s>\\n\")\n", wire, nsAttr, wire, expr, wire)
}

func (g *goEmitter) emitInstanceIdentifierLeafValue(wire, valueExpr, nsAttr string, out *strings.Builder) {
	g.helpers["xmlEscape"] = true
	fmt.Fprintf(out, "\t\tif pfx, ns, ok := %s.XMLPrefixNS(); ok && cambiumValidateXMLPrefix(pfx) == nil {\n", valueExpr)
	fmt.Fprintf(out, "\t\t\tb.WriteString(indent + \"<%s%s xmlns:\" + pfx + \"=\\\"\" + cambiumXMLEscapeAttr(ns) + \"\\\">\" + %s.XMLValue() + \"</%s>\\n\")\n", wire, nsAttr, valueExpr, wire)
	out.WriteString("\t\t} else {\n")
	fmt.Fprintf(out, "\t\t\tb.WriteString(indent + \"<%s%s>\" + %s.XMLValue() + \"</%s>\\n\")\n", wire, nsAttr, valueExpr, wire)
	out.WriteString("\t\t}\n")
}

func (g *goEmitter) emitAnyDataXML(wire string, f fieldInfo, valueExpr, currentNS string, out *strings.Builder) {
	nsAttr := nestedXMLNSAttr(f, currentNS)
	fmt.Fprintf(out, "\tb.WriteString(indent + \"<%s%s>\\n\")\n", wire, nsAttr)
	fmt.Fprintf(out, "\tcambiumWriteRawXML(b, depth+1, %s.XMLValue())\n", valueExpr)
	fmt.Fprintf(out, "\tb.WriteString(indent + \"</%s>\\n\")\n", wire)
}

func (g *goEmitter) emitTopIdentityrefLeafValue(wire, nsAttr, valueExpr string, out *strings.Builder) {
	g.helpers["xmlEscape"] = true
	fmt.Fprintf(out, "\t\tif pfx, ns, ok := %s.XMLPrefixNS(); ok && cambiumValidateXMLPrefix(pfx) == nil {\n", valueExpr)
	fmt.Fprintf(out, "\t\t\tb.WriteString(\"<%s%s xmlns:\" + pfx + \"=\\\"\" + cambiumXMLEscapeAttr(ns) + \"\\\">\" + %s.XMLValue() + \"</%s>\\n\")\n", wire, nsAttr, valueExpr, wire)
	out.WriteString("\t\t} else {\n")
	fmt.Fprintf(out, "\t\t\tb.WriteString(\"<%s%s>\" + %s.XMLValue() + \"</%s>\\n\")\n", wire, nsAttr, valueExpr, wire)
	out.WriteString("\t\t}\n")
}

func (g *goEmitter) emitTopInstanceIdentifierLeafValue(wire, nsAttr, valueExpr string, out *strings.Builder) {
	g.helpers["xmlEscape"] = true
	fmt.Fprintf(out, "\t\tif pfx, ns, ok := %s.XMLPrefixNS(); ok && cambiumValidateXMLPrefix(pfx) == nil {\n", valueExpr)
	fmt.Fprintf(out, "\t\t\tb.WriteString(\"<%s%s xmlns:\" + pfx + \"=\\\"\" + cambiumXMLEscapeAttr(ns) + \"\\\">\" + %s.XMLValue() + \"</%s>\\n\")\n", wire, nsAttr, valueExpr, wire)
	out.WriteString("\t\t} else {\n")
	fmt.Fprintf(out, "\t\t\tb.WriteString(\"<%s%s>\" + %s.XMLValue() + \"</%s>\\n\")\n", wire, nsAttr, valueExpr, wire)
	out.WriteString("\t\t}\n")
}

func (g *goEmitter) emitTopUnionLeafValue(wire, nsAttr, valueExpr string, out *strings.Builder) {
	g.helpers["xmlEscape"] = true
	fmt.Fprintf(out, "\t\tif pfx, ns, ok := %s.XMLPrefixNS(); ok && cambiumValidateXMLPrefix(pfx) == nil {\n", valueExpr)
	fmt.Fprintf(out, "\t\t\tb.WriteString(\"<%s%s\" + cambiumMetadataXMLAttrs(m.CambiumMetadata[%q]) + \" xmlns:\" + pfx + \"=\\\"\" + cambiumXMLEscapeAttr(ns) + \"\\\">\" + %s.XMLValue() + \"</%s>\\n\")\n", wire, nsAttr, wire, valueExpr, wire)
	out.WriteString("\t\t} else {\n")
	fmt.Fprintf(out, "\t\t\tb.WriteString(\"<%s%s\" + cambiumMetadataXMLAttrs(m.CambiumMetadata[%q]) + \">\" + %s.XMLValue() + \"</%s>\\n\")\n", wire, nsAttr, wire, valueExpr, wire)
	out.WriteString("\t\t}\n")
}

func (g *goEmitter) emitTopAnyDataXML(wire string, f fieldInfo, valueExpr string, out *strings.Builder) {
	nsAttr := staticXMLNSAttr(f.namespace)
	fmt.Fprintf(out, "\tb.WriteString(\"<%s%s>\\n\")\n", wire, nsAttr)
	fmt.Fprintf(out, "\tcambiumWriteRawXML(&b, 1, %s.XMLValue())\n", valueExpr)
	fmt.Fprintf(out, "\tb.WriteString(\"</%s>\\n\")\n", wire)
}

func nestedXMLNSAttr(f fieldInfo, currentNS string) string {
	if f.namespace == "" || f.namespace == currentNS {
		return ""
	}
	return staticXMLNSAttr(f.namespace)
}

func staticXMLNSAttr(namespace string) string {
	if namespace == "" {
		return ""
	}
	return generatedStringLiteralContent(` xmlns="` + xmlAttributeEscape(namespace) + `"`)
}

func xmlAttributeEscape(value string) string {
	return strings.NewReplacer("&", "&amp;", `"`, "&quot;", "<", "&lt;", ">", "&gt;").Replace(value)
}

func generatedStringLiteralContent(value string) string {
	quoted := strconv.Quote(value)
	return quoted[1 : len(quoted)-1]
}

func jsonWireName(f fieldInfo, currentModule string) string {
	if f.moduleName == "" || f.moduleName == currentModule {
		return f.wire
	}
	return f.moduleName + ":" + f.wire
}

func scalarBaseType(f fieldInfo) string {
	s := strings.TrimPrefix(f.goType, "*")
	s = strings.TrimPrefix(s, "[]")
	if strings.HasPrefix(s, "UserOrderedVec[") && strings.HasSuffix(s, "]") {
		s = strings.TrimSuffix(strings.TrimPrefix(s, "UserOrderedVec["), "]")
	}
	return s
}

func fieldConcreteType(f fieldInfo) string {
	return scalarBaseType(f)
}

func fieldItemsExpr(owner, ident string, f fieldInfo) string {
	if strings.HasPrefix(f.goType, "UserOrderedVec[") {
		return owner + "." + ident + ".items"
	}
	return owner + "." + ident
}

func (g *goEmitter) emitCollectionItems(owner string, f fieldInfo, out *strings.Builder, indent string) string {
	if !g.shouldSortCollection(f) {
		return fieldItemsExpr(owner, f.ident, f)
	}
	g.needsSort = true
	items := "ordered" + f.ident
	fmt.Fprintf(out, "%s%s := append([]%s(nil), %s...)\n", indent, items, fieldConcreteType(f), fieldItemsExpr(owner, f.ident, f))
	g.emitSortSlice(items, f, out, indent)
	return items
}

func (g *goEmitter) shouldSortCollection(f fieldInfo) bool {
	if strings.HasPrefix(f.goType, "UserOrderedVec[") {
		return false
	}
	switch f.node.Kind() {
	case cambium.SchemaNodeKindLeafList:
		return f.jsonKind != "Empty"
	case cambium.SchemaNodeKindList:
		return len(g.sortableListKeyFields(f)) > 0
	default:
		return false
	}
}

func (g *goEmitter) emitSortSlice(items string, f fieldInfo, out *strings.Builder, indent string) {
	fmt.Fprintf(out, "%ssort.Slice(%s, func(i, j int) bool {\n", indent, items)
	switch f.node.Kind() {
	case cambium.SchemaNodeKindLeafList:
		fmt.Fprintf(out, "%s\treturn %s\n", indent, scalarLessExpr(items+"[i]", items+"[j]", f))
	case cambium.SchemaNodeKindList:
		for _, key := range g.sortableListKeyFields(f) {
			left := fmt.Sprintf("%s[i].%s", items, key.ident)
			right := fmt.Sprintf("%s[j].%s", items, key.ident)
			fmt.Fprintf(out, "%s\tif %s {\n", indent, scalarLessExpr(left, right, key))
			fmt.Fprintf(out, "%s\t\treturn true\n", indent)
			fmt.Fprintf(out, "%s\t}\n", indent)
			fmt.Fprintf(out, "%s\tif %s {\n", indent, scalarLessExpr(right, left, key))
			fmt.Fprintf(out, "%s\t\treturn false\n", indent)
			fmt.Fprintf(out, "%s\t}\n", indent)
		}
		out.WriteString(indent + "\treturn false\n")
	default:
		out.WriteString(indent + "\treturn false\n")
	}
	fmt.Fprintf(out, "%s})\n", indent)
}

func (g *goEmitter) listKeyFields(f fieldInfo) []fieldInfo {
	keyNames := make([]string, 0)
	for key := range f.node.ListKeys().Iter() {
		keyNames = append(keyNames, key.Name())
	}
	if len(keyNames) == 0 {
		return nil
	}
	ordered := orderedListChildren(f.node.DataChildren(true), f.node.ListKeys())
	fields := g.collectFields(fieldConcreteType(f), ordered)
	byName := make(map[string]fieldInfo, len(fields))
	for _, field := range fields {
		byName[field.wire] = field
	}
	keys := make([]fieldInfo, 0, len(keyNames))
	for _, name := range keyNames {
		if field, ok := byName[name]; ok {
			keys = append(keys, field)
		}
	}
	return keys
}

func (g *goEmitter) sortableListKeyFields(f fieldInfo) []fieldInfo {
	keys := g.listKeyFields(f)
	if len(keys) == 0 {
		return nil
	}
	out := keys[:0]
	for _, key := range keys {
		if key.jsonKind != "Empty" {
			out = append(out, key)
		}
	}
	return out
}

func scalarLessExpr(left, right string, f fieldInfo) string {
	if f.isUnion {
		return fmt.Sprintf("%s.String() < %s.String()", left, right)
	}
	if f.isIdentityref {
		return fmt.Sprintf("%s.AsJSONName() < %s.AsJSONName()", left, right)
	}
	base := scalarBaseType(f)
	switch f.jsonKind {
	case "Bool":
		return fmt.Sprintf("!%s && %s", left, right)
	case "BareNumber", "QuotedNumber":
		if base == "Decimal64" {
			return fmt.Sprintf("%s.Raw() < %s.Raw()", left, right)
		}
		if f.isNewtype {
			return fmt.Sprintf("%s.Get() < %s.Get()", left, right)
		}
		return fmt.Sprintf("%s < %s", left, right)
	case "String", "InstanceIdentifier":
		if base == "string" {
			return fmt.Sprintf("%s < %s", left, right)
		}
		return fmt.Sprintf("%s.String() < %s.String()", left, right)
	default:
		return fmt.Sprintf("%s.String() < %s.String()", left, right)
	}
}

func (g *goEmitter) xmlValueExpr(valueRef string, f fieldInfo) string {
	base := scalarBaseType(f)
	if f.isUnion {
		return fmt.Sprintf("%s.XMLValue()", valueRef)
	}
	if f.isIdentityref {
		return fmt.Sprintf("%s.XMLValue()", valueRef)
	}
	switch f.jsonKind {
	case "InstanceIdentifier":
		return fmt.Sprintf("%s.XMLValue()", valueRef)
	case "String":
		if f.isEnum || f.isBits {
			return fmt.Sprintf("cambiumXMLEscapeText(%s.String())", valueRef)
		}
		if base == "string" {
			return fmt.Sprintf("cambiumXMLEscapeText(%s)", valueRef)
		}
		return fmt.Sprintf("cambiumXMLEscapeText(%s.String())", valueRef)
	case "Bool":
		g.helpers["strconv"] = true
		return fmt.Sprintf("strconv.FormatBool(%s)", valueRef)
	case "BareNumber", "QuotedNumber":
		if f.isNewtype || base == "Decimal64" {
			return fmt.Sprintf("%s.String()", valueRef)
		}
		g.helpers["strconv"] = true
		if f.intSigned {
			return fmt.Sprintf("strconv.FormatInt(int64(%s), 10)", valueRef)
		}
		return fmt.Sprintf("strconv.FormatUint(uint64(%s), 10)", valueRef)
	}
	return fmt.Sprintf("%s.String()", valueRef)
}

func (g *goEmitter) emitFieldJSON(f fieldInfo, currentModule string, byNode map[cambium.SchemaNodeRef]fieldInfo, out *strings.Builder) {
	wire := jsonWireName(f, currentModule)
	ident := f.ident

	switch f.node.Kind() {
	case cambium.SchemaNodeKindLeaf:
		if f.jsonKind == "Empty" {
			if f.optional {
				fmt.Fprintf(out, "\tif n.%s != nil {\n", ident)
				out.WriteString("\t\tif !first { b.WriteByte(',') }\n")
				out.WriteString("\t\tfirst = false\n")
				out.WriteString("\t\tcambiumJSONIndent(b, depth+1)\n")
				fmt.Fprintf(out, "\t\tb.WriteString(\"\\\"%s\\\": [null]\")\n", wire)
				fmt.Fprintf(out, "\t\tcambiumWriteMetadataJSON(b, depth+1, %q, n.CambiumMetadata[%q], &first)\n", wire, f.wire)
				out.WriteString("\t}\n")
			} else {
				out.WriteString("\tif !first { b.WriteByte(',') }\n")
				out.WriteString("\tfirst = false\n")
				out.WriteString("\tcambiumJSONIndent(b, depth+1)\n")
				fmt.Fprintf(out, "\tb.WriteString(\"\\\"%s\\\": [null]\")\n", wire)
				fmt.Fprintf(out, "\tcambiumWriteMetadataJSON(b, depth+1, %q, n.CambiumMetadata[%q], &first)\n", wire, f.wire)
			}
			return
		}
		if f.optional {
			defaultValue, hasDefault := f.node.DefaultValue()
			valueRef := "(*n." + ident + ")"
			if f.isUnion {
				valueRef = "n." + ident
			}
			fmt.Fprintf(out, "\tif n.%s != nil {\n", ident)
			if hasDefault {
				fmt.Fprintf(out, "\t\tif mode != WithDefaultsTrim || %s != %s {\n", g.jsonLiteralExpr(valueRef, f), g.defaultJSONLiteralExpr(f, defaultValue))
				g.emitScalarJSON(wire, "b", "depth+1", valueRef, f, out)
				fmt.Fprintf(out, "\t\t\tcambiumWriteMetadataJSON(b, depth+1, %q, n.CambiumMetadata[%q], &first)\n", wire, f.wire)
				out.WriteString("\t\t}\n")
			} else {
				g.emitScalarJSON(wire, "b", "depth+1", valueRef, f, out)
				fmt.Fprintf(out, "\t\tcambiumWriteMetadataJSON(b, depth+1, %q, n.CambiumMetadata[%q], &first)\n", wire, f.wire)
			}
			out.WriteString("\t}\n")
			if hasDefault {
				defaultCond := "n." + ident + " == nil && (mode == WithDefaultsAll || mode == WithDefaultsAllTagged)"
				if guard := g.choiceDefaultEmissionGuard("n", f, byNode); guard != "" {
					defaultCond += " && (" + guard + ")"
				}
				fmt.Fprintf(out, "\tif %s {\n", defaultCond)
				g.emitScalarJSONLiteral(wire, "b", "depth+1", g.defaultJSONLiteralExpr(f, defaultValue), out)
				out.WriteString("\t}\n")
			}
		} else {
			if defaultValue, hasDefault := f.node.DefaultValue(); hasDefault {
				fmt.Fprintf(out, "\tif mode != WithDefaultsTrim || %s != %s {\n", g.jsonLiteralExpr("n."+ident, f), g.defaultJSONLiteralExpr(f, defaultValue))
				g.emitScalarJSON(wire, "b", "depth+1", "n."+ident, f, out)
				fmt.Fprintf(out, "\tcambiumWriteMetadataJSON(b, depth+1, %q, n.CambiumMetadata[%q], &first)\n", wire, f.wire)
				out.WriteString("\t}\n")
			} else {
				g.emitScalarJSON(wire, "b", "depth+1", "n."+ident, f, out)
				fmt.Fprintf(out, "\tcambiumWriteMetadataJSON(b, depth+1, %q, n.CambiumMetadata[%q], &first)\n", wire, f.wire)
			}
		}
	case cambium.SchemaNodeKindLeafList:
		defaults := f.node.DefaultValues()
		defaultVar := ""
		if len(defaults) > 0 {
			defaultVar = g.emitLeafListDefaultCheck("n", f, defaults, out, "\t")
		}
		if strings.HasPrefix(f.goType, "UserOrderedVec[") {
			fmt.Fprintf(out, "\tif n.%s.Len() > 0 {\n", ident)
		} else {
			fmt.Fprintf(out, "\tif len(n.%s) > 0 {\n", ident)
		}
		if defaultVar != "" {
			fmt.Fprintf(out, "\t\tif mode != WithDefaultsTrim || !%s {\n", defaultVar)
		}
		itemsExpr := g.emitCollectionItems("n", f, out, "\t\t")
		g.emitLeafListJSONArray(wire, "b", "depth+1", "depth+2", itemsExpr, f, out)
		fmt.Fprintf(out, "\t\tcambiumWriteMetadataJSON(b, depth+1, %q, n.CambiumMetadata[%q], &first)\n", wire, f.wire)
		if defaultVar != "" {
			out.WriteString("\t\t}\n")
		}
		out.WriteString("\t}\n")
		if len(defaults) > 0 {
			defaultCond := "mode == WithDefaultsAll || mode == WithDefaultsAllTagged"
			if guard := g.choiceDefaultEmissionGuard("n", f, byNode); guard != "" {
				defaultCond = "(" + defaultCond + ") && (" + guard + ")"
			}
			if strings.HasPrefix(f.goType, "UserOrderedVec[") {
				fmt.Fprintf(out, "\tif n.%s.Len() == 0 && %s {\n", ident, defaultCond)
			} else {
				fmt.Fprintf(out, "\tif len(n.%s) == 0 && %s {\n", ident, defaultCond)
			}
			g.emitDefaultLeafListJSONArray(wire, "b", "depth+1", "depth+2", defaults, f, out)
			out.WriteString("\t}\n")
		}
	case cambium.SchemaNodeKindAnyData:
		if f.optional {
			fmt.Fprintf(out, "\tif n.%s != nil {\n", ident)
			g.emitAnyDataJSON(wire, "b", "depth+1", "(*n."+ident+")", f, out)
			out.WriteString("\t}\n")
		} else {
			g.emitAnyDataJSON(wire, "b", "depth+1", "n."+ident, f, out)
		}
	case cambium.SchemaNodeKindContainer, cambium.SchemaNodeKindAction, cambium.SchemaNodeKindNotification:
		if f.optional {
			fmt.Fprintf(out, "\tif n.%s != nil {\n", ident)
			out.WriteString("\t\tif !first { b.WriteByte(',') }\n")
			out.WriteString("\t\tfirst = false\n")
			out.WriteString("\t\tcambiumJSONIndent(b, depth+1)\n")
			fmt.Fprintf(out, "\t\tb.WriteString(\"\\\"%s\\\": \")\n", wire)
			fmt.Fprintf(out, "\t\tn.%s.writeJSONWithDefaults(b, depth+1, mode)\n", ident)
			out.WriteString("\t}\n")
		} else {
			contentCond := "n." + ident + ".hasContentWithDefaults(mode)"
			if guard := g.choiceDefaultEmissionGuard("n", f, byNode); guard != "" {
				contentCond += " && (" + guard + ")"
			}
			fmt.Fprintf(out, "\tif %s {\n", contentCond)
			out.WriteString("\t\tif !first { b.WriteByte(',') }\n")
			out.WriteString("\t\tfirst = false\n")
			out.WriteString("\t\tcambiumJSONIndent(b, depth+1)\n")
			fmt.Fprintf(out, "\t\tb.WriteString(\"\\\"%s\\\": \")\n", wire)
			fmt.Fprintf(out, "\t\tn.%s.writeJSONWithDefaults(b, depth+1, mode)\n", ident)
			out.WriteString("\t}\n")
		}
	case cambium.SchemaNodeKindList:
		if strings.HasPrefix(f.goType, "UserOrderedVec[") {
			fmt.Fprintf(out, "\tif n.%s.Len() > 0 {\n", ident)
		} else {
			fmt.Fprintf(out, "\tif len(n.%s) > 0 {\n", ident)
		}
		itemsExpr := g.emitCollectionItems("n", f, out, "\t\t")
		out.WriteString("\t\tif !first { b.WriteByte(',') }\n")
		out.WriteString("\t\tfirst = false\n")
		out.WriteString("\t\tcambiumJSONIndent(b, depth+1)\n")
		fmt.Fprintf(out, "\t\tb.WriteString(\"\\\"%s\\\": [\")\n", wire)
		fmt.Fprintf(out, "\t\tfor i, entry := range %s {\n", itemsExpr)
		out.WriteString("\t\t\tif i > 0 { b.WriteByte(',') }\n")
		out.WriteString("\t\t\tcambiumJSONIndent(b, depth+2)\n")
		out.WriteString("\t\t\tentry.writeJSONWithDefaults(b, depth+2, mode)\n")
		out.WriteString("\t\t}\n")
		out.WriteString("\t\tcambiumJSONIndent(b, depth+1)\n")
		out.WriteString("\t\tb.WriteByte(']')\n")
		out.WriteString("\t}\n")
	}
}

func (g *goEmitter) emitScalarJSON(wire, writer, depthExpr, valueRef string, f fieldInfo, out *strings.Builder) {
	g.emitScalarJSONLiteral(wire, writer, depthExpr, g.jsonLiteralExpr(valueRef, f), out)
}

func (g *goEmitter) emitScalarJSONLiteral(wire, writer, depthExpr, literalExpr string, out *strings.Builder) {
	fmt.Fprintf(out, "\tif !first { %s.WriteByte(',') }\n", writer)
	out.WriteString("\tfirst = false\n")
	fmt.Fprintf(out, "\tcambiumJSONIndent(%s, %s)\n", writer, depthExpr)
	fmt.Fprintf(out, "\t%s.WriteString(\"\\\"%s\\\": \" + %s)\n", writer, wire, literalExpr)
}

func (g *goEmitter) jsonLiteralExpr(valueRef string, f fieldInfo) string {
	switch f.jsonKind {
	case "QuotedNumber":
		return "\"\\\"\" + " + g.scalarValueExpr(valueRef, f) + " + \"\\\"\""
	case "Empty":
		return `"[null]"`
	default:
		return g.scalarValueExpr(valueRef, f)
	}
}

func (g *goEmitter) defaultJSONLiteralExpr(f fieldInfo, value string) string {
	if info, ok := f.node.LeafType(); ok {
		if literal, ok := g.typeDefaultJSONLiteralExpr(info, value); ok {
			return literal
		}
		g.recordEmitError(fmt.Errorf("invalid default %q for %s", value, f.node.Path()))
	}
	switch f.jsonKind {
	case "String", "InstanceIdentifier":
		return g.jsonStringLiteralExpr(value)
	case "BareNumber", "Bool":
		return strconv.Quote(value)
	case "QuotedNumber":
		return strconv.Quote(`"` + value + `"`)
	case "Empty":
		return `"[null]"`
	default:
		return g.jsonStringLiteralExpr(value)
	}
}

func (g *goEmitter) typeDefaultJSONLiteralExpr(info cambium.TypeInfo, value string) (string, bool) {
	switch r := info.Resolved().(type) {
	case cambium.ResolvedLeafRef:
		if realtype, ok := r.Realtype(); ok {
			return g.typeDefaultJSONLiteralExpr(*realtype, value)
		}
		return g.jsonStringLiteralExpr(value), true
	case cambium.ResolvedUnion:
		for _, member := range r.Members() {
			if literal, ok := g.typeDefaultJSONLiteralExpr(member, value); ok {
				return literal, true
			}
		}
		return "", false
	case cambium.ResolvedBoolean:
		if value == "true" || value == "false" {
			return strconv.Quote(value), true
		}
		return "", false
	case cambium.ResolvedInt:
		normalized, ok := integerDefaultJSONValue(value, r)
		if !ok {
			return "", false
		}
		if jsonIntegerKindQuoted(r.Kind) {
			return strconv.Quote(`"` + normalized + `"`), true
		}
		return strconv.Quote(normalized), true
	case cambium.ResolvedDecimal64:
		canonical, ok := decimal64DefaultCanonical(value, r)
		if !ok {
			return "", false
		}
		return strconv.Quote(`"` + canonical + `"`), true
	case cambium.ResolvedString:
		if !stringDefaultMatches(value, r) {
			return "", false
		}
		return g.jsonStringLiteralExpr(value), true
	case cambium.ResolvedBinary:
		if !binaryDefaultMatches(value, r) {
			return "", false
		}
		return g.jsonStringLiteralExpr(value), true
	case cambium.ResolvedEnumeration:
		if !enumDefaultMatches(value, r.Values()) {
			return "", false
		}
		return g.jsonStringLiteralExpr(value), true
	case cambium.ResolvedBits:
		canonical, ok := bitsDefaultCanonical(value, r.Values())
		if !ok {
			return "", false
		}
		return g.jsonStringLiteralExpr(canonical), true
	case cambium.ResolvedIdentityRef:
		jsonName, ok := g.identityrefDefaultJSONName(value, r)
		if !ok {
			return "", false
		}
		return g.jsonStringLiteralExpr(jsonName), true
	case cambium.ResolvedInstanceIdentifier, cambium.ResolvedUnknown:
		return g.jsonStringLiteralExpr(value), true
	default:
		return "", false
	}
}

func (g *goEmitter) jsonStringLiteralExpr(value string) string {
	g.helpers["jsonEscape"] = true
	return "cambiumJSONEscape(" + strconv.Quote(value) + ")"
}

func integerDefaultJSONValue(value string, resolved cambium.ResolvedInt) (string, bool) {
	limits := intTypeLimits(resolved.Kind)
	var normalized string
	var numeric *big.Int
	if isSignedIntKind(resolved.Kind) {
		parsed, err := strconv.ParseInt(value, 10, intKindBitSize(resolved.Kind))
		if err != nil {
			return "", false
		}
		normalized = strconv.FormatInt(parsed, 10)
		numeric = big.NewInt(parsed)
	} else {
		parsed, err := strconv.ParseUint(value, 10, intKindBitSize(resolved.Kind))
		if err != nil {
			return "", false
		}
		normalized = strconv.FormatUint(parsed, 10)
		numeric = new(big.Int).SetUint64(parsed)
	}
	if !integerDefaultInRange(numeric, resolved.Range, limits) {
		return "", false
	}
	return normalized, true
}

func integerDefaultInRange(value *big.Int, bounds []cambium.RangeBound, limits intLimits) bool {
	if len(bounds) == 0 {
		return true
	}
	for _, bound := range bounds {
		lo, ok := intBoundBig(bound.Min(), limits)
		if !ok {
			return false
		}
		hi, ok := intBoundBig(bound.Max(), limits)
		if !ok {
			return false
		}
		if value.Cmp(lo) >= 0 && value.Cmp(hi) <= 0 {
			return true
		}
	}
	return false
}

func intBoundBig(bound string, limits intLimits) (*big.Int, bool) {
	switch bound {
	case "min":
		bound = limits.min
	case "max":
		bound = limits.max
	}
	value := new(big.Int)
	if _, ok := value.SetString(bound, 10); !ok {
		return nil, false
	}
	return value, true
}

func decimal64DefaultCanonical(value string, resolved cambium.ResolvedDecimal64) (string, bool) {
	fd := resolved.FractionDigits().Value()
	raw, ok := decimal64DefaultRaw(value, fd)
	if !ok {
		return "", false
	}
	if len(resolved.Range) > 0 {
		inRange := false
		for _, bound := range resolved.Range {
			lo := parseDecimal64BoundRaw(bound.Min(), fd)
			hi := parseDecimal64BoundRaw(bound.Max(), fd)
			if raw >= lo && raw <= hi {
				inRange = true
				break
			}
		}
		if !inRange {
			return "", false
		}
	}
	return formatDecimal64Raw(raw, fd), true
}

func decimal64DefaultRaw(value string, fractionDigits uint8) (int64, bool) {
	if value == "" {
		return 0, false
	}
	negative := strings.HasPrefix(value, "-")
	text := strings.TrimPrefix(value, "-")
	text = strings.TrimPrefix(text, "+")
	if text == "" {
		return 0, false
	}
	wholeText, fracText, hasFraction := strings.Cut(text, ".")
	if wholeText == "" {
		return 0, false
	}
	if hasFraction && fracText == "" {
		return 0, false
	}
	if len(fracText) > int(fractionDigits) {
		return 0, false
	}
	if !decimalDigitsOnly(wholeText) || (fracText != "" && !decimalDigitsOnly(fracText)) {
		return 0, false
	}
	for len(fracText) < int(fractionDigits) {
		fracText += "0"
	}
	whole := new(big.Int)
	whole.SetString(wholeText, 10)
	scale := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(fractionDigits)), nil)
	raw := new(big.Int).Mul(whole, scale)
	if fracText != "" {
		frac := new(big.Int)
		frac.SetString(fracText, 10)
		raw.Add(raw, frac)
	}
	if negative {
		raw.Neg(raw)
	}
	if !raw.IsInt64() {
		return 0, false
	}
	return raw.Int64(), true
}

func decimalDigitsOnly(value string) bool {
	for _, c := range value {
		if c < '0' || c > '9' {
			return false
		}
	}
	return value != ""
}

func formatDecimal64Raw(raw int64, fractionDigits uint8) string {
	if fractionDigits == 0 {
		return strconv.FormatInt(raw, 10)
	}
	divisor := int64(1)
	for i := uint8(0); i < fractionDigits; i++ {
		divisor *= 10
	}
	whole := raw / divisor
	frac := raw % divisor
	if frac < 0 {
		frac = -frac
	}
	padded := strconv.FormatInt(frac, 10)
	width := int(fractionDigits)
	if len(padded) < width {
		padded = strings.Repeat("0", width-len(padded)) + padded
	} else if len(padded) > width {
		padded = padded[:width]
	}
	trimmed := strings.TrimRight(padded, "0")
	if trimmed == "" {
		trimmed = "0"
	}
	if raw < 0 {
		return fmt.Sprintf("-%d.%s", -whole, trimmed)
	}
	return fmt.Sprintf("%d.%s", whole, trimmed)
}

func stringDefaultMatches(value string, resolved cambium.ResolvedString) bool {
	length := int64(len([]rune(value)))
	if !lengthDefaultMatches(length, resolved.Length) {
		return false
	}
	for _, pattern := range resolved.Patterns {
		matched, err := regexp.MatchString("^(?:"+xsdregex.NativePattern(pattern.Regex())+")$", value)
		if err != nil {
			return false
		}
		if pattern.IsInverted() {
			matched = !matched
		}
		if !matched {
			return false
		}
	}
	return true
}

func binaryDefaultMatches(value string, resolved cambium.ResolvedBinary) bool {
	decoded, err := base64.StdEncoding.DecodeString(value)
	if err != nil {
		return false
	}
	return lengthDefaultMatches(int64(len(decoded)), resolved.Length)
}

func lengthDefaultMatches(length int64, bounds []cambium.RangeBound) bool {
	if len(bounds) == 0 {
		return true
	}
	for _, bound := range bounds {
		lo := parseLengthBound(bound.Min())
		hi := parseLengthBound(bound.Max())
		if length >= lo && length <= hi {
			return true
		}
	}
	return false
}

func enumDefaultMatches(value string, values []cambium.EnumValue) bool {
	for _, enum := range values {
		if enum.Name() == value {
			return true
		}
	}
	return false
}

func bitsDefaultCanonical(value string, values []cambium.EnumValue) (string, bool) {
	allowed := make(map[string]bool, len(values))
	for _, bit := range values {
		allowed[bit.Name()] = true
	}
	seen := make(map[string]bool)
	for _, token := range bitsASCIIFields(value) {
		if !allowed[token] || seen[token] {
			return "", false
		}
		seen[token] = true
	}
	canonical := make([]string, 0, len(seen))
	for _, bit := range values {
		if seen[bit.Name()] {
			canonical = append(canonical, bit.Name())
		}
	}
	return strings.Join(canonical, " "), true
}

func bitsASCIIFields(value string) []string {
	var names []string
	start := 0
	for start < len(value) {
		for start < len(value) && value[start] == ' ' {
			start++
		}
		end := start
		for end < len(value) && value[end] != ' ' {
			end++
		}
		if end > start {
			names = append(names, value[start:end])
		}
		start = end
	}
	return names
}

func (g *goEmitter) identityrefDefaultJSONName(value string, resolved cambium.ResolvedIdentityRef) (string, bool) {
	for _, member := range collectIdentityrefMembers(resolved.Bases()) {
		jsonName := member.name
		if member.module != g.moduleName {
			jsonName = member.module + ":" + member.name
		}
		prefixedName := ""
		if member.prefix != "" {
			prefixedName = member.prefix + ":" + member.name
		}
		if value == jsonName || value == prefixedName || (member.module == g.moduleName && value == member.name) {
			return jsonName, true
		}
	}
	return "", false
}

func (g *goEmitter) emitAnyDataJSON(wire, writer, depthExpr, valueRef string, f fieldInfo, out *strings.Builder) {
	fmt.Fprintf(out, "\tif !first { %s.WriteByte(',') }\n", writer)
	out.WriteString("\tfirst = false\n")
	fmt.Fprintf(out, "\tcambiumJSONIndent(%s, %s)\n", writer, depthExpr)
	fmt.Fprintf(out, "\t%s.WriteString(\"\\\"%s\\\": \")\n", writer, wire)
	fmt.Fprintf(out, "\tcambiumWriteRawJSON(%s, %s, %s.JSONValue())\n", writer, depthExpr, valueRef)
}

func (g *goEmitter) emitLeafListJSONArray(wire, writer, depthExpr, elemDepthExpr, itemsExpr string, f fieldInfo, out *strings.Builder) {
	fmt.Fprintf(out, "\t\tif !first { %s.WriteByte(',') }\n", writer)
	out.WriteString("\t\tfirst = false\n")
	fmt.Fprintf(out, "\t\tcambiumJSONIndent(%s, %s)\n", writer, depthExpr)
	fmt.Fprintf(out, "\t\t%s.WriteString(\"\\\"%s\\\": [\")\n", writer, wire)
	fmt.Fprintf(out, "\t\tfor i, v := range %s {\n", itemsExpr)
	fmt.Fprintf(out, "\t\t\tif i > 0 { %s.WriteByte(',') }\n", writer)
	fmt.Fprintf(out, "\t\t\tcambiumJSONIndent(%s, %s)\n", writer, elemDepthExpr)
	g.emitJSONLeafListElement(writer, elemDepthExpr, "v", f, out)
	out.WriteString("\t\t}\n")
	fmt.Fprintf(out, "\t\tcambiumJSONIndent(%s, %s)\n", writer, depthExpr)
	fmt.Fprintf(out, "\t\t%s.WriteByte(']')\n", writer)
}

func (g *goEmitter) emitDefaultLeafListJSONArray(wire, writer, depthExpr, elemDepthExpr string, defaults []string, f fieldInfo, out *strings.Builder) {
	defaults = g.orderedDefaultValues(defaults, f)
	fmt.Fprintf(out, "\t\tif !first { %s.WriteByte(',') }\n", writer)
	out.WriteString("\t\tfirst = false\n")
	fmt.Fprintf(out, "\t\tcambiumJSONIndent(%s, %s)\n", writer, depthExpr)
	fmt.Fprintf(out, "\t\t%s.WriteString(\"\\\"%s\\\": [\")\n", writer, wire)
	for i, value := range defaults {
		if i > 0 {
			fmt.Fprintf(out, "\t\t%s.WriteByte(',')\n", writer)
		}
		fmt.Fprintf(out, "\t\tcambiumJSONIndent(%s, %s)\n", writer, elemDepthExpr)
		fmt.Fprintf(out, "\t\t%s.WriteString(%s)\n", writer, g.defaultJSONLiteralExpr(f, value))
	}
	fmt.Fprintf(out, "\t\tcambiumJSONIndent(%s, %s)\n", writer, depthExpr)
	fmt.Fprintf(out, "\t\t%s.WriteByte(']')\n", writer)
}

func (g *goEmitter) orderedDefaultValues(defaults []string, f fieldInfo) []string {
	out := append([]string(nil), defaults...)
	if len(out) < 2 || !g.shouldSortCollection(f) {
		return out
	}
	info, ok := f.node.LeafType()
	if !ok {
		sort.Strings(out)
		return out
	}
	sort.SliceStable(out, func(i, j int) bool {
		return g.defaultValueLess(info, out[i], out[j], f)
	})
	return out
}

func (g *goEmitter) defaultValueLess(info cambium.TypeInfo, left, right string, f fieldInfo) bool {
	if f.isUnion {
		leftText, leftOK := g.defaultTextForType(info, left)
		rightText, rightOK := g.defaultTextForType(info, right)
		if leftOK && rightOK {
			return leftText < rightText
		}
		return left < right
	}
	switch r := info.Resolved().(type) {
	case cambium.ResolvedLeafRef:
		if realtype, ok := r.Realtype(); ok {
			return g.defaultValueLess(*realtype, left, right, f)
		}
	case cambium.ResolvedBoolean:
		if (left == "true" || left == "false") && (right == "true" || right == "false") {
			return left == "false" && right == "true"
		}
	case cambium.ResolvedInt:
		leftValue, leftOK := integerDefaultSortValue(left, r)
		rightValue, rightOK := integerDefaultSortValue(right, r)
		if leftOK && rightOK {
			return leftValue.Cmp(rightValue) < 0
		}
	case cambium.ResolvedDecimal64:
		fd := r.FractionDigits().Value()
		leftRaw, leftOK := decimal64DefaultRaw(left, fd)
		rightRaw, rightOK := decimal64DefaultRaw(right, fd)
		if leftOK && rightOK {
			return leftRaw < rightRaw
		}
	case cambium.ResolvedIdentityRef:
		leftText, leftOK := g.identityrefDefaultJSONName(left, r)
		rightText, rightOK := g.identityrefDefaultJSONName(right, r)
		if leftOK && rightOK {
			return leftText < rightText
		}
	}
	leftText, leftOK := g.defaultTextForType(info, left)
	rightText, rightOK := g.defaultTextForType(info, right)
	if leftOK && rightOK {
		return leftText < rightText
	}
	return left < right
}

func (g *goEmitter) defaultTextForType(info cambium.TypeInfo, value string) (string, bool) {
	switch r := info.Resolved().(type) {
	case cambium.ResolvedLeafRef:
		if realtype, ok := r.Realtype(); ok {
			return g.defaultTextForType(*realtype, value)
		}
		return value, true
	case cambium.ResolvedUnion:
		for _, member := range r.Members() {
			if text, ok := g.defaultTextForType(member, value); ok {
				return text, true
			}
		}
		return "", false
	case cambium.ResolvedBoolean:
		if value == "true" || value == "false" {
			return value, true
		}
		return "", false
	case cambium.ResolvedInt:
		return integerDefaultJSONValue(value, r)
	case cambium.ResolvedDecimal64:
		return decimal64DefaultCanonical(value, r)
	case cambium.ResolvedString:
		if !stringDefaultMatches(value, r) {
			return "", false
		}
		return value, true
	case cambium.ResolvedBinary:
		if !binaryDefaultMatches(value, r) {
			return "", false
		}
		return value, true
	case cambium.ResolvedEnumeration:
		if enumDefaultMatches(value, r.Values()) {
			return value, true
		}
		return "", false
	case cambium.ResolvedBits:
		return bitsDefaultCanonical(value, r.Values())
	case cambium.ResolvedIdentityRef:
		return g.identityrefDefaultJSONName(value, r)
	case cambium.ResolvedInstanceIdentifier, cambium.ResolvedUnknown:
		return value, true
	default:
		return "", false
	}
}

func integerDefaultSortValue(value string, resolved cambium.ResolvedInt) (*big.Int, bool) {
	normalized, ok := integerDefaultJSONValue(value, resolved)
	if !ok {
		return nil, false
	}
	numeric := new(big.Int)
	if _, ok := numeric.SetString(normalized, 10); !ok {
		return nil, false
	}
	return numeric, true
}

func (g *goEmitter) emitJSONLeafListElement(writer, depthExpr, valueRef string, f fieldInfo, out *strings.Builder) {
	switch f.jsonKind {
	case "String", "InstanceIdentifier":
		g.helpers["jsonEscape"] = true
		expr := g.scalarValueExpr(valueRef, f)
		fmt.Fprintf(out, "\t\t%s.WriteString(%s)\n", writer, expr)
	case "BareNumber", "Bool":
		expr := g.scalarValueExpr(valueRef, f)
		fmt.Fprintf(out, "\t\t%s.WriteString(%s)\n", writer, expr)
	case "QuotedNumber":
		expr := g.scalarValueExpr(valueRef, f)
		fmt.Fprintf(out, "\t\t%s.WriteString(\"\\\"\" + %s + \"\\\"\")\n", writer, expr)
	}
}

func (g *goEmitter) scalarValueExpr(valueRef string, f fieldInfo) string {
	base := scalarBaseType(f)
	if f.isUnion {
		return fmt.Sprintf("%s.JSONValue()", valueRef)
	}
	if f.isIdentityref {
		return fmt.Sprintf("cambiumJSONEscape(%s.AsJSONName())", valueRef)
	}
	switch f.jsonKind {
	case "InstanceIdentifier":
		return fmt.Sprintf("%s.JSONValue()", valueRef)
	case "String":
		if f.isEnum || f.isBits {
			return fmt.Sprintf("cambiumJSONEscape(%s.String())", valueRef)
		}
		if base == "string" {
			return fmt.Sprintf("cambiumJSONEscape(%s)", valueRef)
		}
		return fmt.Sprintf("cambiumJSONEscape(%s.String())", valueRef)
	case "Bool":
		g.helpers["strconv"] = true
		return fmt.Sprintf("strconv.FormatBool(%s)", valueRef)
	case "BareNumber":
		if f.isNewtype {
			return fmt.Sprintf("%s.String()", valueRef)
		}
		g.helpers["strconv"] = true
		if f.intSigned {
			return fmt.Sprintf("strconv.FormatInt(int64(%s), 10)", valueRef)
		}
		return fmt.Sprintf("strconv.FormatUint(uint64(%s), 10)", valueRef)
	case "QuotedNumber":
		if f.isNewtype || base == "Decimal64" {
			return fmt.Sprintf("%s.String()", valueRef)
		}
		g.helpers["strconv"] = true
		if f.intSigned {
			return fmt.Sprintf("strconv.FormatInt(%s, 10)", valueRef)
		}
		return fmt.Sprintf("strconv.FormatUint(%s, 10)", valueRef)
	}
	return fmt.Sprintf("%s.String()", valueRef)
}

func (g *goEmitter) emitTopLevelXML(f fieldInfo, out *strings.Builder) {
	wire := f.wire
	ident := f.ident
	nsAttr := staticXMLNSAttr(f.namespace)

	switch f.node.Kind() {
	case cambium.SchemaNodeKindLeaf:
		if f.jsonKind == "Empty" {
			if f.optional {
				fmt.Fprintf(out, "\tif m.%s != nil {\n", ident)
				fmt.Fprintf(out, "\t\tb.WriteString(\"<%s%s\" + cambiumMetadataXMLAttrs(m.CambiumMetadata[%q]) + \"/>\\n\")\n", wire, nsAttr, wire)
				out.WriteString("\t}\n")
			} else {
				fmt.Fprintf(out, "\tb.WriteString(\"<%s%s\" + cambiumMetadataXMLAttrs(m.CambiumMetadata[%q]) + \"/>\\n\")\n", wire, nsAttr, wire)
			}
			return
		}
		if f.optional {
			fmt.Fprintf(out, "\tif m.%s != nil {\n", ident)
			if f.isIdentityref {
				g.emitTopIdentityrefLeafValue(wire, nsAttr, "(*m."+ident+")", out)
				out.WriteString("\t}\n")
				return
			}
			if f.jsonKind == "InstanceIdentifier" {
				g.emitTopInstanceIdentifierLeafValue(wire, nsAttr, "(*m."+ident+")", out)
				out.WriteString("\t}\n")
				return
			}
			valueRef := "(*m." + ident + ")"
			if f.isUnion {
				valueRef = "m." + ident
				g.emitTopUnionLeafValue(wire, nsAttr, valueRef, out)
				out.WriteString("\t}\n")
				return
			}
			expr := g.xmlValueExpr(valueRef, f)
			if strings.Contains(expr, "cambiumXMLEscapeText") {
				g.helpers["xmlEscape"] = true
			}
			fmt.Fprintf(out, "\t\tb.WriteString(\"<%s%s\" + cambiumMetadataXMLAttrs(m.CambiumMetadata[%q]) + \">\" + %s + \"</%s>\\n\")\n", wire, nsAttr, wire, expr, wire)
			out.WriteString("\t}\n")
		} else {
			if f.isIdentityref {
				g.emitTopIdentityrefLeafValue(wire, nsAttr, "m."+ident, out)
				return
			}
			if f.jsonKind == "InstanceIdentifier" {
				g.emitTopInstanceIdentifierLeafValue(wire, nsAttr, "m."+ident, out)
				return
			}
			if f.isUnion {
				g.emitTopUnionLeafValue(wire, nsAttr, "m."+ident, out)
				return
			}
			expr := g.xmlValueExpr("m."+ident, f)
			if strings.Contains(expr, "cambiumXMLEscapeText") {
				g.helpers["xmlEscape"] = true
			}
			fmt.Fprintf(out, "\tb.WriteString(\"<%s%s\" + cambiumMetadataXMLAttrs(m.CambiumMetadata[%q]) + \">\" + %s + \"</%s>\\n\")\n", wire, nsAttr, wire, expr, wire)
		}
	case cambium.SchemaNodeKindLeafList:
		itemsExpr := g.emitCollectionItems("m", f, out, "\t")
		fmt.Fprintf(out, "\tfor _, v := range %s {\n", itemsExpr)
		if f.isIdentityref {
			g.emitTopIdentityrefLeafValue(wire, nsAttr, "v", out)
			out.WriteString("\t}\n")
			return
		}
		if f.jsonKind == "InstanceIdentifier" {
			g.emitTopInstanceIdentifierLeafValue(wire, nsAttr, "v", out)
			out.WriteString("\t}\n")
			return
		}
		if f.isUnion {
			g.emitTopUnionLeafValue(wire, nsAttr, "v", out)
			out.WriteString("\t}\n")
			return
		}
		expr := g.xmlValueExpr("v", f)
		if strings.Contains(expr, "cambiumXMLEscapeText") {
			g.helpers["xmlEscape"] = true
		}
		fmt.Fprintf(out, "\t\tb.WriteString(\"<%s%s\" + cambiumMetadataXMLAttrs(m.CambiumMetadata[%q]) + \">\" + %s + \"</%s>\\n\")\n", wire, nsAttr, wire, expr, wire)
		out.WriteString("\t}\n")
	case cambium.SchemaNodeKindAnyData:
		if f.optional {
			fmt.Fprintf(out, "\tif m.%s != nil {\n", ident)
			g.emitTopAnyDataXML(wire, f, "(*m."+ident+")", out)
			out.WriteString("\t}\n")
		} else {
			g.emitTopAnyDataXML(wire, f, "m."+ident, out)
		}
	case cambium.SchemaNodeKindContainer:
		if f.optional {
			fmt.Fprintf(out, "\tif m.%s != nil {\n", ident)
			fmt.Fprintf(out, "\t\tif m.%s.hasContent() {\n", ident)
			fmt.Fprintf(out, "\t\t\tb.WriteString(\"<%s%s>\\n\")\n", wire, nsAttr)
			fmt.Fprintf(out, "\t\t\tm.%s.writeXML(&b, 1)\n", ident)
			fmt.Fprintf(out, "\t\t\tb.WriteString(\"</%s>\\n\")\n", wire)
			out.WriteString("\t\t} else {\n")
			fmt.Fprintf(out, "\t\t\tb.WriteString(\"<%s%s/>\\n\")\n", wire, nsAttr)
			out.WriteString("\t\t}\n")
			out.WriteString("\t}\n")
		} else {
			fmt.Fprintf(out, "\tif m.%s.hasContent() {\n", ident)
			fmt.Fprintf(out, "\t\tb.WriteString(\"<%s%s>\\n\")\n", wire, nsAttr)
			fmt.Fprintf(out, "\t\tm.%s.writeXML(&b, 1)\n", ident)
			fmt.Fprintf(out, "\t\tb.WriteString(\"</%s>\\n\")\n", wire)
			out.WriteString("\t}\n")
		}
	case cambium.SchemaNodeKindList:
		itemsExpr := g.emitCollectionItems("m", f, out, "\t")
		fmt.Fprintf(out, "\tfor _, entry := range %s {\n", itemsExpr)
		fmt.Fprintf(out, "\t\tb.WriteString(\"<%s%s>\\n\")\n", wire, nsAttr)
		out.WriteString("\t\tentry.writeXML(&b, 1)\n")
		fmt.Fprintf(out, "\t\tb.WriteString(\"</%s>\\n\")\n", wire)
		out.WriteString("\t}\n")
	}
}

func (g *goEmitter) emitTopLevelJSON(f fieldInfo, byNode map[cambium.SchemaNodeRef]fieldInfo, out *strings.Builder) {
	moduleName := f.moduleName
	if moduleName == "" {
		moduleName = g.moduleName
	}
	wire := fmt.Sprintf("%s:%s", moduleName, f.wire)
	ident := f.ident

	switch f.node.Kind() {
	case cambium.SchemaNodeKindLeaf:
		if f.jsonKind == "Empty" {
			if f.optional {
				fmt.Fprintf(out, "\tif m.%s != nil {\n", ident)
				out.WriteString("\t\tif !first { w.WriteByte(',') }\n")
				out.WriteString("\t\tfirst = false\n")
				out.WriteString("\t\tcambiumJSONIndent(w, 1)\n")
				fmt.Fprintf(out, "\t\tw.WriteString(\"\\\"%s\\\": [null]\")\n", wire)
				fmt.Fprintf(out, "\t\tcambiumWriteMetadataJSON(w, 1, %q, m.CambiumMetadata[%q], &first)\n", wire, f.wire)
				out.WriteString("\t}\n")
			} else {
				out.WriteString("\tif !first { w.WriteByte(',') }\n")
				out.WriteString("\tfirst = false\n")
				out.WriteString("\tcambiumJSONIndent(w, 1)\n")
				fmt.Fprintf(out, "\tw.WriteString(\"\\\"%s\\\": [null]\")\n", wire)
				fmt.Fprintf(out, "\tcambiumWriteMetadataJSON(w, 1, %q, m.CambiumMetadata[%q], &first)\n", wire, f.wire)
			}
			return
		}
		if f.optional {
			defaultValue, hasDefault := f.node.DefaultValue()
			valueRef := "(*m." + ident + ")"
			if f.isUnion {
				valueRef = "m." + ident
			}
			fmt.Fprintf(out, "\tif m.%s != nil {\n", ident)
			if hasDefault {
				fmt.Fprintf(out, "\t\tif mode != WithDefaultsTrim || %s != %s {\n", g.jsonLiteralExpr(valueRef, f), g.defaultJSONLiteralExpr(f, defaultValue))
				g.emitScalarJSON(wire, "w", "1", valueRef, f, out)
				fmt.Fprintf(out, "\t\t\tcambiumWriteMetadataJSON(w, 1, %q, m.CambiumMetadata[%q], &first)\n", wire, f.wire)
				out.WriteString("\t\t}\n")
			} else {
				g.emitScalarJSON(wire, "w", "1", valueRef, f, out)
				fmt.Fprintf(out, "\t\tcambiumWriteMetadataJSON(w, 1, %q, m.CambiumMetadata[%q], &first)\n", wire, f.wire)
			}
			out.WriteString("\t}\n")
			if hasDefault {
				defaultCond := "m." + ident + " == nil && (mode == WithDefaultsAll || mode == WithDefaultsAllTagged)"
				if guard := g.choiceDefaultEmissionGuard("m", f, byNode); guard != "" {
					defaultCond += " && (" + guard + ")"
				}
				fmt.Fprintf(out, "\tif %s {\n", defaultCond)
				g.emitScalarJSONLiteral(wire, "w", "1", g.defaultJSONLiteralExpr(f, defaultValue), out)
				out.WriteString("\t}\n")
			}
		} else {
			if defaultValue, hasDefault := f.node.DefaultValue(); hasDefault {
				fmt.Fprintf(out, "\tif mode != WithDefaultsTrim || %s != %s {\n", g.jsonLiteralExpr("m."+ident, f), g.defaultJSONLiteralExpr(f, defaultValue))
				g.emitScalarJSON(wire, "w", "1", "m."+ident, f, out)
				fmt.Fprintf(out, "\tcambiumWriteMetadataJSON(w, 1, %q, m.CambiumMetadata[%q], &first)\n", wire, f.wire)
				out.WriteString("\t}\n")
			} else {
				g.emitScalarJSON(wire, "w", "1", "m."+ident, f, out)
				fmt.Fprintf(out, "\tcambiumWriteMetadataJSON(w, 1, %q, m.CambiumMetadata[%q], &first)\n", wire, f.wire)
			}
		}
	case cambium.SchemaNodeKindLeafList:
		defaults := f.node.DefaultValues()
		defaultVar := ""
		if len(defaults) > 0 {
			defaultVar = g.emitLeafListDefaultCheck("m", f, defaults, out, "\t")
		}
		if strings.HasPrefix(f.goType, "UserOrderedVec[") {
			fmt.Fprintf(out, "\tif m.%s.Len() > 0 {\n", ident)
		} else {
			fmt.Fprintf(out, "\tif len(m.%s) > 0 {\n", ident)
		}
		if defaultVar != "" {
			fmt.Fprintf(out, "\t\tif mode != WithDefaultsTrim || !%s {\n", defaultVar)
		}
		itemsExpr := g.emitCollectionItems("m", f, out, "\t\t")
		g.emitLeafListJSONArray(wire, "w", "1", "2", itemsExpr, f, out)
		fmt.Fprintf(out, "\t\tcambiumWriteMetadataJSON(w, 1, %q, m.CambiumMetadata[%q], &first)\n", wire, f.wire)
		if defaultVar != "" {
			out.WriteString("\t\t}\n")
		}
		out.WriteString("\t}\n")
		if len(defaults) > 0 {
			defaultCond := "mode == WithDefaultsAll || mode == WithDefaultsAllTagged"
			if guard := g.choiceDefaultEmissionGuard("m", f, byNode); guard != "" {
				defaultCond = "(" + defaultCond + ") && (" + guard + ")"
			}
			if strings.HasPrefix(f.goType, "UserOrderedVec[") {
				fmt.Fprintf(out, "\tif m.%s.Len() == 0 && %s {\n", ident, defaultCond)
			} else {
				fmt.Fprintf(out, "\tif len(m.%s) == 0 && %s {\n", ident, defaultCond)
			}
			g.emitDefaultLeafListJSONArray(wire, "w", "1", "2", defaults, f, out)
			out.WriteString("\t}\n")
		}
	case cambium.SchemaNodeKindAnyData:
		if f.optional {
			fmt.Fprintf(out, "\tif m.%s != nil {\n", ident)
			g.emitAnyDataJSON(wire, "w", "1", "(*m."+ident+")", f, out)
			out.WriteString("\t}\n")
		} else {
			g.emitAnyDataJSON(wire, "w", "1", "m."+ident, f, out)
		}
	case cambium.SchemaNodeKindContainer:
		if f.optional {
			fmt.Fprintf(out, "\tif m.%s != nil {\n", ident)
			out.WriteString("\t\tif !first { w.WriteByte(',') }\n")
			out.WriteString("\t\tfirst = false\n")
			out.WriteString("\t\tcambiumJSONIndent(w, 1)\n")
			fmt.Fprintf(out, "\t\tw.WriteString(\"\\\"%s\\\": \")\n", wire)
			fmt.Fprintf(out, "\t\tm.%s.writeJSONWithDefaults(w, 1, mode)\n", ident)
			out.WriteString("\t}\n")
		} else {
			contentCond := "m." + ident + ".hasContentWithDefaults(mode)"
			if guard := g.choiceDefaultEmissionGuard("m", f, byNode); guard != "" {
				contentCond += " && (" + guard + ")"
			}
			fmt.Fprintf(out, "\tif %s {\n", contentCond)
			out.WriteString("\t\tif !first { w.WriteByte(',') }\n")
			out.WriteString("\t\tfirst = false\n")
			out.WriteString("\t\tcambiumJSONIndent(w, 1)\n")
			fmt.Fprintf(out, "\t\tw.WriteString(\"\\\"%s\\\": \")\n", wire)
			fmt.Fprintf(out, "\t\tm.%s.writeJSONWithDefaults(w, 1, mode)\n", ident)
			out.WriteString("\t}\n")
		}
	case cambium.SchemaNodeKindList:
		if strings.HasPrefix(f.goType, "UserOrderedVec[") {
			fmt.Fprintf(out, "\tif m.%s.Len() > 0 {\n", ident)
		} else {
			fmt.Fprintf(out, "\tif len(m.%s) > 0 {\n", ident)
		}
		itemsExpr := g.emitCollectionItems("m", f, out, "\t\t")
		out.WriteString("\t\tif !first { w.WriteByte(',') }\n")
		out.WriteString("\t\tfirst = false\n")
		out.WriteString("\t\tcambiumJSONIndent(w, 1)\n")
		fmt.Fprintf(out, "\t\tw.WriteString(\"\\\"%s\\\": [\")\n", wire)
		fmt.Fprintf(out, "\t\tfor i, entry := range %s {\n", itemsExpr)
		out.WriteString("\t\t\tif i > 0 { w.WriteByte(',') }\n")
		out.WriteString("\t\t\tcambiumJSONIndent(w, 2)\n")
		out.WriteString("\t\t\tentry.writeJSONWithDefaults(w, 2, mode)\n")
		out.WriteString("\t\t}\n")
		out.WriteString("\t\tcambiumJSONIndent(w, 1)\n")
		out.WriteString("\t\tw.WriteByte(']')\n")
		out.WriteString("\t}\n")
	}
}
