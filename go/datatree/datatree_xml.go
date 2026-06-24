// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

package datatree

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"math/big"
	"strings"

	"github.com/signalbreak-labs/cambium/go/cambium"
)

// XML (de)serialization. The tree's internal leaf representation is the JSON_IETF
// token (see datatree.go), so XML parsing converts element text into that token
// using the leaf's resolved type, and XML serialization converts the token back
// to element text by inspecting the token. This keeps one canonical internal
// form, so a tree parsed from either format serializes to both and validates the
// same way. Element order, list keys-first, and leaf-list/list order follow the
// same ordered rules as JSON.

// --- parse -------------------------------------------------------------------

// xmlElem is a generic parsed XML element: its local name plus either leaf text
// or ordered child elements.
type xmlElem struct {
	local string
	ns    string
	text  string
	kids  []*xmlElem
}

type xmlQName struct {
	local string
	ns    string
}

func parseXML(m cambium.Module, data []byte) (*Tree, error) {
	roots, err := decodeXMLForest(data)
	if err != nil {
		return nil, err
	}
	nodes, err := bindXML(flattenTopLevel(m), roots)
	if err != nil {
		return nil, err
	}
	return &Tree{module: m, roots: nodes}, nil
}

// decodeXMLForest reads the (possibly multi-root) top-level elements; YANG data
// serializes top-level siblings as consecutive elements, which is a fragment
// rather than a single-rooted document.
func decodeXMLForest(data []byte) ([]*xmlElem, error) {
	dec := xml.NewDecoder(bytes.NewReader(data))
	var roots []*xmlElem
	for {
		tok, err := dec.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("datatree: xml: %w", err)
		}
		if se, ok := tok.(xml.StartElement); ok {
			el, err := decodeXMLElem(dec, se)
			if err != nil {
				return nil, err
			}
			roots = append(roots, el)
			continue
		}
		switch t := tok.(type) {
		case xml.CharData:
			if strings.TrimSpace(string(t)) != "" {
				return nil, fmt.Errorf("datatree: xml: non-whitespace text outside the root element")
			}
		case xml.Directive:
			return nil, fmt.Errorf("datatree: xml: directives are not supported")
		}
	}
	return roots, nil
}

func decodeXMLElem(dec *xml.Decoder, start xml.StartElement) (*xmlElem, error) {
	el := &xmlElem{local: start.Name.Local, ns: start.Name.Space}
	var text strings.Builder
	for {
		tok, err := dec.Token()
		if err != nil {
			return nil, fmt.Errorf("datatree: xml element %q: %w", el.local, err)
		}
		switch t := tok.(type) {
		case xml.StartElement:
			kid, err := decodeXMLElem(dec, t)
			if err != nil {
				return nil, err
			}
			el.kids = append(el.kids, kid)
		case xml.CharData:
			text.Write(t)
		case xml.EndElement:
			el.text = text.String()
			return el, nil
		case xml.Directive:
			return nil, fmt.Errorf("datatree: xml element %q: directives are not supported", el.local)
		}
	}
}

// bindXML matches parsed elements to schema children by namespace URI and local
// name (grouping repeats for lists/leaf-lists), in schema declaration order.
// Elements with no matching schema node are an error.
func bindXML(children []cambium.SchemaNodeRef, elems []*xmlElem) ([]*node, error) {
	byName := make(map[xmlQName][]*xmlElem, len(elems))
	for _, e := range elems {
		byName[xmlQName{local: e.local, ns: e.ns}] = append(byName[xmlQName{local: e.local, ns: e.ns}], e)
	}
	matched := make(map[*xmlElem]bool, len(elems))
	var out []*node
	for _, sn := range children {
		group := byName[schemaXMLName(sn)]
		if len(group) == 0 {
			continue
		}
		for _, e := range group {
			matched[e] = true
		}
		n, err := bindXMLNode(sn, group)
		if err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	for _, e := range elems {
		if !matched[e] {
			return nil, fmt.Errorf("datatree: unknown XML element %q in namespace %q (no matching schema node)", e.local, e.ns)
		}
	}
	return out, nil
}

func schemaXMLName(sn cambium.SchemaNodeRef) xmlQName {
	return xmlQName{local: sn.Name(), ns: sn.Namespace()}
}

func bindXMLNode(sn cambium.SchemaNodeRef, group []*xmlElem) (*node, error) {
	n := &node{name: sn.Name(), module: sn.Module().Name(), namespace: sn.Namespace()}
	switch {
	case sn.IsLeaf():
		if err := requireSingleXML(sn, group); err != nil {
			return nil, err
		}
		if len(group[0].kids) > 0 {
			return nil, fmt.Errorf("datatree: XML leaf %q contains child elements", sn.Name())
		}
		n.kind = kindLeaf
		ti, _ := sn.LeafType()
		n.value = jsonTokenFromText(ti, group[0].text, sn.Module(), sn.Module())
	case sn.IsLeafList():
		n.kind = kindLeafList
		ti, _ := sn.LeafType()
		for _, e := range group {
			if len(e.kids) > 0 {
				return nil, fmt.Errorf("datatree: XML leaf-list %q contains child elements", sn.Name())
			}
			n.values = append(n.values, jsonTokenFromText(ti, e.text, sn.Module(), sn.Module()))
		}
	case sn.IsContainer():
		if err := requireSingleXML(sn, group); err != nil {
			return nil, err
		}
		if strings.TrimSpace(group[0].text) != "" {
			return nil, fmt.Errorf("datatree: XML container %q contains non-whitespace text", sn.Name())
		}
		n.kind = kindContainer
		kids, err := bindXML(childRefs(sn.DataChildren(true)), group[0].kids)
		if err != nil {
			return nil, err
		}
		n.children = kids
	case sn.IsList():
		n.kind = kindList
		for _, e := range group {
			if strings.TrimSpace(e.text) != "" {
				return nil, fmt.Errorf("datatree: XML list %q entry contains non-whitespace text", sn.Name())
			}
			kids, err := bindXML(childRefs(sn.DataChildren(true)), e.kids)
			if err != nil {
				return nil, err
			}
			n.entries = append(n.entries, keysFirst(sn, kids))
		}
	case sn.IsAnyData(), sn.IsAnyXML():
		return nil, fmt.Errorf("datatree: anydata/anyxml %q in XML input is not supported "+
			"(the pure-Go XML reader cannot losslessly capture opaque content; use JSON_IETF "+
			"or the libyang backend)", sn.Name())
	default:
		return nil, fmt.Errorf("datatree: unsupported node kind for %q in this slice", sn.Name())
	}
	return n, nil
}

func requireSingleXML(sn cambium.SchemaNodeRef, group []*xmlElem) error {
	if len(group) <= 1 {
		return nil
	}
	return fmt.Errorf("datatree: duplicate XML element %q in namespace %q", sn.Name(), sn.Namespace())
}

func jsonTokenFromText(ti cambium.TypeInfo, text string, leafModule, sourceModule cambium.Module) json.RawMessage {
	if sourceModule.Name() == "" {
		sourceModule = leafModule
	}
	switch r := ti.Resolved().(type) {
	case cambium.ResolvedBoolean:
		s := strings.TrimSpace(text)
		if s == "true" || s == "false" {
			return json.RawMessage(s)
		}
		return jsonStringToken(text)
	case cambium.ResolvedEmpty:
		if strings.TrimSpace(text) == "" {
			return json.RawMessage("[null]")
		}
		return jsonStringToken(text)
	case cambium.ResolvedInt:
		return jsonTokenFromIntegerText(text, r.Kind)
	case cambium.ResolvedUnion:
		for _, member := range r.Members() {
			token := jsonTokenFromText(member, text, leafModule, sourceModule)
			var trial []string
			validateLeafValue(member, token, "", leafModule.Name(), &trial)
			if len(trial) == 0 {
				return token
			}
		}
		return jsonStringToken(text)
	case cambium.ResolvedLeafRef:
		if rt, ok := r.Realtype(); ok && rt != nil {
			return jsonTokenFromText(*rt, text, leafModule, sourceModule)
		}
		return jsonStringToken(text)
	case cambium.ResolvedIdentityRef:
		if jsonName, ok := identityrefJSONName(text, r, leafModule, sourceModule); ok {
			return jsonStringToken(jsonName)
		}
		return jsonStringToken(text)
	default:
		return jsonStringToken(text)
	}
}

func identityrefJSONName(value string, resolved cambium.ResolvedIdentityRef, leafModule, sourceModule cambium.Module) (string, bool) {
	if sourceModule.Name() == "" {
		sourceModule = leafModule
	}
	prefix, local, prefixed := strings.Cut(value, ":")
	if !prefixed {
		local = value
	}
	seen := make(map[string]bool)
	var visit func(cambium.Identity) (string, bool)
	visit = func(id cambium.Identity) (string, bool) {
		idModule := id.Module()
		key := idModule.Name() + ":" + id.Name()
		if seen[key] {
			return "", false
		}
		seen[key] = true
		if id.Name() == local && identityrefDefaultModuleMatches(prefix, prefixed, idModule, sourceModule) {
			jsonName := id.Name()
			if idModule.Name() != leafModule.Name() {
				jsonName = idModule.Name() + ":" + id.Name()
			}
			return jsonName, true
		}
		for _, derived := range id.Derived() {
			if jsonName, ok := visit(derived); ok {
				return jsonName, true
			}
		}
		return "", false
	}
	for _, base := range resolved.Bases() {
		if jsonName, ok := visit(base); ok {
			return jsonName, true
		}
	}
	return "", false
}

func identityrefDefaultModuleMatches(prefix string, prefixed bool, idModule, sourceModule cambium.Module) bool {
	if !prefixed {
		return sourceModule.Name() == idModule.Name()
	}
	if resolved, ok := sourceModule.ResolvePrefix(prefix); ok {
		return resolved.Name() == idModule.Name()
	}
	return prefix == idModule.Name()
}

func jsonTokenFromIntegerText(text string, kind cambium.IntKind) json.RawMessage {
	s := strings.TrimSpace(text)
	v, ok := new(big.Int).SetString(s, 10)
	if !ok {
		return jsonStringToken(text)
	}
	if integerJSONQuoted(kind) {
		return jsonStringToken(v.String())
	}
	return json.RawMessage(v.String())
}

func jsonStringToken(s string) json.RawMessage {
	b, err := json.Marshal(s)
	if err != nil {
		return json.RawMessage(`""`)
	}
	return json.RawMessage(b)
}

// --- serialize ---------------------------------------------------------------

func (t *Tree) serializeXML() ([]byte, error) {
	var b bytes.Buffer
	for _, n := range t.roots {
		if err := writeXMLNode(&b, n, ""); err != nil {
			return nil, err
		}
	}
	return b.Bytes(), nil
}

func writeXMLNode(b *bytes.Buffer, n *node, parentNS string) error {
	switch n.kind {
	case kindLeaf:
		writeXMLLeaf(b, n, parentNS, xmlTextFromToken(n.value))
	case kindLeafList:
		for _, v := range n.values {
			writeXMLLeaf(b, n, parentNS, xmlTextFromToken(v))
		}
	case kindContainer:
		writeXMLOpen(b, n, parentNS)
		for _, c := range n.children {
			if err := writeXMLNode(b, c, n.namespace); err != nil {
				return err
			}
		}
		writeXMLClose(b, n)
	case kindList:
		for _, entry := range n.entries {
			writeXMLOpen(b, n, parentNS)
			for _, c := range entry {
				if err := writeXMLNode(b, c, n.namespace); err != nil {
					return err
				}
			}
			writeXMLClose(b, n)
		}
	case kindAnyData, kindAnyXML:
		// anydata/anyxml parsed from JSON_IETF cannot be re-serialized as XML
		// (opaque content has no faithful cross-format conversion). The XML-format
		// branch is future-proofing for when opaque XML capture is supported.
		if n.anyFormat != FormatXML {
			return fmt.Errorf("datatree: %q: cross-format anydata/anyxml serialization unsupported (opaque content re-serializes only in its source format)", n.name)
		}
		writeXMLOpen(b, n, parentNS)
		b.Write(n.anyRaw)
		writeXMLClose(b, n)
	}
	return nil
}

func writeXMLLeaf(b *bytes.Buffer, n *node, parentNS, text string) {
	b.WriteByte('<')
	b.WriteString(n.name)
	writeXMLNS(b, n, parentNS)
	if text == "" {
		b.WriteString("/>")
		return
	}
	b.WriteByte('>')
	b.WriteString(escapeXMLText(text))
	writeXMLClose(b, n)
}

func writeXMLOpen(b *bytes.Buffer, n *node, parentNS string) {
	b.WriteByte('<')
	b.WriteString(n.name)
	writeXMLNS(b, n, parentNS)
	b.WriteByte('>')
}

func writeXMLClose(b *bytes.Buffer, n *node) {
	b.WriteString("</")
	b.WriteString(n.name)
	b.WriteByte('>')
}

// writeXMLNS emits an xmlns declaration when the node's namespace differs from
// the enclosing element's (always at the root, where parentNS is "").
func writeXMLNS(b *bytes.Buffer, n *node, parentNS string) {
	if n.namespace != "" && n.namespace != parentNS {
		b.WriteString(` xmlns="`)
		b.WriteString(escapeXMLText(n.namespace))
		b.WriteByte('"')
	}
}

// xmlTextFromToken converts an internal JSON token back to XML element text by
// inspecting the token (type-free): [null] and empty become an empty element,
// JSON strings are unquoted, and bare number/boolean literals pass through.
func xmlTextFromToken(raw json.RawMessage) string {
	s := strings.TrimSpace(string(raw))
	if s == "" || s == "[null]" {
		return ""
	}
	if s[0] == '"' {
		var str string
		if err := json.Unmarshal(raw, &str); err == nil {
			return str
		}
	}
	return s
}

func escapeXMLText(s string) string {
	return strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
		`"`, "&quot;",
		"'", "&apos;",
	).Replace(s)
}
