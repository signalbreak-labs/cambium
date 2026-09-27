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
	scope *xmlNSScope // namespace bindings in scope on the element
}

// xmlNSScope is one in-scope XML namespace binding, chained to the bindings of
// the enclosing elements, so a lookup walks outward as XML namespace scoping
// does. Leaf values that carry prefixes (identityref, instance-identifier)
// resolve them here: they are document bindings, unrelated to the schema's
// import prefixes (RFC 7950 §9.10.3, §9.13.2).
type xmlNSScope struct {
	parent *xmlNSScope
	prefix string // "" binds the default namespace
	uri    string
}

// namespace implements valueScope. An empty URI (xmlns="") unbinds.
func (s *xmlNSScope) namespace(prefix string) (string, bool) {
	for ; s != nil; s = s.parent {
		if s.prefix == prefix {
			return s.uri, s.uri != ""
		}
	}
	return "", false
}

func withNSDecls(parent *xmlNSScope, attrs []xml.Attr) *xmlNSScope {
	s := parent
	for _, a := range attrs {
		switch {
		case a.Name.Space == "xmlns":
			s = &xmlNSScope{parent: s, prefix: a.Name.Local, uri: a.Value}
		case a.Name.Space == "" && a.Name.Local == "xmlns":
			s = &xmlNSScope{parent: s, uri: a.Value}
		}
	}
	return s
}

// valueScope resolves the prefixes inside a lexical identityref or
// instance-identifier value to namespace URIs ("" is the default namespace).
// XML element text resolves against the element's in-scope xmlns bindings;
// schema default values against the defining module's import prefixes.
type valueScope interface {
	namespace(prefix string) (string, bool)
}

// schemaScope resolves prefixes as YANG does inside module m.
type schemaScope struct{ module cambium.Module }

func (s schemaScope) namespace(prefix string) (string, bool) {
	if m, ok := s.module.ResolvePrefix(prefix); ok {
		return m.Namespace(), true
	}
	// Tolerate an imported module's name in place of its prefix.
	for _, imp := range s.module.Imports() {
		if imp.Name == prefix {
			if m, ok := s.module.ResolvePrefix(imp.Prefix); ok {
				return m.Namespace(), true
			}
		}
	}
	return "", false
}

type xmlQName struct {
	local string
	ns    string
}

const maxXMLNestingDepth = 10000

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
			el, err := decodeXMLElem(dec, se, nil, 1)
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

func decodeXMLElem(dec *xml.Decoder, start xml.StartElement, parent *xmlNSScope, depth int) (*xmlElem, error) {
	if depth > maxXMLNestingDepth {
		return nil, fmt.Errorf("datatree: xml: nesting exceeds %d", maxXMLNestingDepth)
	}
	el := &xmlElem{local: start.Name.Local, ns: start.Name.Space, scope: withNSDecls(parent, start.Attr)}
	var text strings.Builder
	for {
		tok, err := dec.Token()
		if err != nil {
			return nil, fmt.Errorf("datatree: xml element %q: %w", el.local, err)
		}
		switch t := tok.(type) {
		case xml.StartElement:
			kid, err := decodeXMLElem(dec, t, el.scope, depth+1)
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
	n := newNode(sn)
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
		n.value = canonicalLeafToken(sn, jsonTokenFromText(ti, group[0].text, sn.Module(), group[0].scope))
	case sn.IsLeafList():
		n.kind = kindLeafList
		ti, _ := sn.LeafType()
		for _, e := range group {
			if len(e.kids) > 0 {
				return nil, fmt.Errorf("datatree: XML leaf-list %q contains child elements", sn.Name())
			}
			n.values = append(n.values, canonicalLeafToken(sn, jsonTokenFromText(ti, e.text, sn.Module(), e.scope)))
		}
		sortSystemOrdered(sn, n)
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
		sortSystemOrdered(sn, n)
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

// jsonTokenFromText converts a lexical value — XML element text or a schema
// default — to the internal JSON_IETF token for type ti. scope resolves the
// prefixes inside identityref and instance-identifier values.
func jsonTokenFromText(ti cambium.TypeInfo, text string, leafModule cambium.Module, scope valueScope) json.RawMessage {
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
			token := jsonTokenFromText(member, text, leafModule, scope)
			var trial []string
			validateLeafValue(member, token, "", leafModule.Name(), &trial)
			if len(trial) == 0 {
				return token
			}
		}
		return jsonStringToken(text)
	case cambium.ResolvedLeafRef:
		if rt, ok := r.Realtype(); ok && rt != nil {
			return jsonTokenFromText(*rt, text, leafModule, scope)
		}
		return jsonStringToken(text)
	case cambium.ResolvedIdentityRef:
		if jsonName, ok := identityrefJSONName(text, r, leafModule, scope); ok {
			return jsonStringToken(jsonName)
		}
		return jsonStringToken(text)
	case cambium.ResolvedInstanceIdentifier:
		if path, ok := instanceIdentifierFromLexical(text, leafModule, scope); ok {
			return jsonStringToken(path)
		}
		return jsonStringToken(text)
	default:
		return jsonStringToken(text)
	}
}

// identityrefJSONName maps a lexical "prefix:name" (or bare "name", in the
// scope's default namespace) identityref value to its JSON_IETF form: bare for
// an identity of the leaf's own module, module-qualified otherwise.
func identityrefJSONName(value string, resolved cambium.ResolvedIdentityRef, leafModule cambium.Module, scope valueScope) (string, bool) {
	prefix, local, prefixed := strings.Cut(value, ":")
	if !prefixed {
		prefix, local = "", value
	}
	ns, ok := scope.namespace(prefix)
	if !ok {
		return "", false
	}
	id, ok := findIdentity(resolved, func(id cambium.Identity) bool {
		return id.Name() == local && id.Module().Namespace() == ns
	})
	if !ok {
		return "", false
	}
	if id.Module().Name() == leafModule.Name() {
		return id.Name(), true
	}
	return id.Module().Name() + ":" + id.Name(), true
}

// identityFromJSON finds the identity a JSON_IETF identityref value names
// ("module:name", or a bare name of the leaf's module).
func identityFromJSON(raw json.RawMessage, resolved cambium.ResolvedIdentityRef, leafModule cambium.Module) (cambium.Identity, bool) {
	s, ok := jsonStringValue(raw)
	if !ok {
		return cambium.Identity{}, false
	}
	mod, local, qualified := strings.Cut(s, ":")
	if !qualified {
		mod, local = leafModule.Name(), s
	}
	return findIdentity(resolved, func(id cambium.Identity) bool {
		return id.Name() == local && id.Module().Name() == mod
	})
}

// findIdentity walks the identityref's bases and everything derived from them,
// in declaration order, for the first identity matching match.
func findIdentity(resolved cambium.ResolvedIdentityRef, match func(cambium.Identity) bool) (cambium.Identity, bool) {
	seen := make(map[string]bool)
	var visit func(cambium.Identity) (cambium.Identity, bool)
	visit = func(id cambium.Identity) (cambium.Identity, bool) {
		key := id.Module().Name() + ":" + id.Name()
		if seen[key] {
			return cambium.Identity{}, false
		}
		seen[key] = true
		if match(id) {
			return id, true
		}
		for _, derived := range id.Derived() {
			if found, ok := visit(derived); ok {
				return found, true
			}
		}
		return cambium.Identity{}, false
	}
	for _, base := range resolved.Bases() {
		if found, ok := visit(base); ok {
			return found, true
		}
	}
	return cambium.Identity{}, false
}

func jsonTokenFromIntegerText(text string, kind cambium.IntKind) json.RawMessage {
	s := strings.TrimSpace(text)
	if len(s) > maxNumericLexicalLen {
		// Too long to be any YANG integer: keep it unparsed (no math/big work)
		// in the shape Validate expects, so it is reported by length.
		if integerJSONQuoted(kind) || !jsonIntegerNumberLexical.MatchString(s) {
			return jsonStringToken(s)
		}
		return json.RawMessage(s)
	}
	v, ok := new(big.Int).SetString(s, 10)
	if !ok {
		return jsonStringToken(text)
	}
	if integerJSONQuoted(kind) {
		return jsonStringToken(v.String())
	}
	return json.RawMessage(v.String())
}

// jsonStringToken encodes s as a JSON string token. <, > and & stay literal
// (no HTML escaping), as in libyang's JSON output and in JSON_IETF input.
func jsonStringToken(s string) json.RawMessage {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(s); err != nil {
		return json.RawMessage(`""`)
	}
	return json.RawMessage(bytes.TrimSuffix(b.Bytes(), []byte("\n")))
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
		text, decls := xmlLeafText(n, n.value)
		writeXMLLeaf(b, n, parentNS, text, decls)
	case kindLeafList:
		for _, v := range n.values {
			text, decls := xmlLeafText(n, v)
			writeXMLLeaf(b, n, parentNS, text, decls)
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

func writeXMLLeaf(b *bytes.Buffer, n *node, parentNS, text string, decls []xmlNSDecl) {
	b.WriteByte('<')
	b.WriteString(n.name)
	writeXMLNS(b, n, parentNS)
	for _, d := range decls {
		b.WriteString(" xmlns:")
		b.WriteString(d.prefix)
		b.WriteString(`="`)
		b.WriteString(escapeXMLAttr(d.uri))
		b.WriteByte('"')
	}
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
		b.WriteString(escapeXMLAttr(n.namespace))
		b.WriteByte('"')
	}
}

// xmlNSDecl is a prefixed namespace declaration a leaf value needs on its own
// element.
type xmlNSDecl struct{ prefix, uri string }

// xmlLeafText renders the value token of leaf or leaf-list n as XML text.
func xmlLeafText(n *node, raw json.RawMessage) (string, []xmlNSDecl) {
	ti, ok := n.schema.LeafType()
	if !ok {
		return xmlTextFromToken(raw), nil
	}
	return xmlValueText(ti, raw, n.schema.Module())
}

// xmlValueText renders a value token of type ti as XML element text. Values
// that name identities or data nodes of another module are written with each
// such module's own prefix, declared on the value's element, as libyang
// prints them; an identity of the leaf's own module needs no prefix.
func xmlValueText(ti cambium.TypeInfo, raw json.RawMessage, leafModule cambium.Module) (string, []xmlNSDecl) {
	switch r := ti.Resolved().(type) {
	case cambium.ResolvedIdentityRef:
		if id, ok := identityFromJSON(raw, r, leafModule); ok && id.Module().Name() != leafModule.Name() {
			m := id.Module()
			return m.Prefix() + ":" + id.Name(), []xmlNSDecl{{prefix: m.Prefix(), uri: m.Namespace()}}
		}
	case cambium.ResolvedInstanceIdentifier:
		if s, ok := jsonStringValue(raw); ok {
			if path, decls, ok := instanceIdentifierXML(s, leafModule); ok {
				return path, decls
			}
		}
	case cambium.ResolvedLeafRef:
		if rt, ok := r.Realtype(); ok && rt != nil {
			return xmlValueText(*rt, raw, leafModule)
		}
	case cambium.ResolvedUnion:
		for _, member := range r.Members() {
			var trial []string
			validateLeafValue(member, raw, "", leafModule.Name(), &trial)
			if len(trial) == 0 {
				return xmlValueText(member, raw, leafModule) // first matching member wins
			}
		}
	}
	return xmlTextFromToken(raw), nil
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

// Escaping matches libyang's printer (lyxml_dump_text): element text escapes
// &, < and > (the last only for readability); attribute values, which are
// double-quoted, also escape ".
var (
	xmlTextEscaper = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;")
	xmlAttrEscaper = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;")
)

func escapeXMLText(s string) string { return xmlTextEscaper.Replace(s) }

func escapeXMLAttr(s string) string { return xmlAttrEscaper.Replace(s) }
