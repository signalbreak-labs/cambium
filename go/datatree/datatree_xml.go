package datatree

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
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
	text  string
	kids  []*xmlElem
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
		if err == io.EOF {
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
		}
	}
	return roots, nil
}

func decodeXMLElem(dec *xml.Decoder, start xml.StartElement) (*xmlElem, error) {
	el := &xmlElem{local: start.Name.Local}
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
			el.text = strings.TrimSpace(text.String())
			return el, nil
		}
	}
}

// bindXML matches parsed elements to schema children by local name (grouping
// repeats for lists/leaf-lists), in schema declaration order. Elements with no
// matching schema node are an error.
func bindXML(children []cambium.SchemaNodeRef, elems []*xmlElem) ([]*node, error) {
	byName := make(map[string][]*xmlElem, len(elems))
	for _, e := range elems {
		byName[e.local] = append(byName[e.local], e)
	}
	matched := make(map[*xmlElem]bool, len(elems))
	var out []*node
	for _, sn := range children {
		group := byName[sn.Name()]
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
			return nil, fmt.Errorf("datatree: unknown XML element %q (no matching schema node)", e.local)
		}
	}
	return out, nil
}

func bindXMLNode(sn cambium.SchemaNodeRef, group []*xmlElem) (*node, error) {
	n := &node{name: sn.Name(), module: sn.Module().Name(), namespace: sn.Namespace()}
	switch {
	case sn.IsLeaf():
		n.kind = kindLeaf
		ti, _ := sn.LeafType()
		n.value = jsonTokenFromText(ti, group[0].text)
	case sn.IsLeafList():
		n.kind = kindLeafList
		ti, _ := sn.LeafType()
		for _, e := range group {
			n.values = append(n.values, jsonTokenFromText(ti, e.text))
		}
	case sn.IsContainer():
		n.kind = kindContainer
		kids, err := bindXML(childRefs(sn.DataChildren(true)), group[0].kids)
		if err != nil {
			return nil, err
		}
		n.children = kids
	case sn.IsList():
		n.kind = kindList
		for _, e := range group {
			kids, err := bindXML(childRefs(sn.DataChildren(true)), e.kids)
			if err != nil {
				return nil, err
			}
			n.entries = append(n.entries, keysFirst(sn, kids))
		}
	default:
		return nil, fmt.Errorf("datatree: unsupported node kind for %q in this slice", sn.Name())
	}
	return n, nil
}

type jsonEnc int

const (
	encQuoted jsonEnc = iota // JSON string-encoded
	encBare                  // JSON number/boolean literal
	encEmpty                 // the empty type: [null]
)

// jsonEncoding reports how a leaf's value is JSON-encoded (RFC 7951 §6), which
// determines how XML element text maps to the internal JSON token.
func jsonEncoding(ti cambium.TypeInfo) jsonEnc {
	switch r := ti.Resolved().(type) {
	case cambium.ResolvedBoolean:
		return encBare
	case cambium.ResolvedEmpty:
		return encEmpty
	case cambium.ResolvedInt:
		// int64/uint64 are JSON strings; the narrower widths are JSON numbers.
		if r.Kind == cambium.IntKindI64 || r.Kind == cambium.IntKindU64 {
			return encQuoted
		}
		return encBare
	default:
		// string, enum, bits, identityref, instance-identifier, decimal64,
		// binary, union, leafref: JSON string-encoded.
		return encQuoted
	}
}

func jsonTokenFromText(ti cambium.TypeInfo, text string) json.RawMessage {
	switch jsonEncoding(ti) {
	case encEmpty:
		return json.RawMessage("[null]")
	case encBare:
		if text == "" {
			return json.RawMessage(`""`) // defensive: never emit an empty bare token
		}
		return json.RawMessage(text)
	default:
		b, _ := json.Marshal(text) // marshaling a string never errors
		return json.RawMessage(b)
	}
}

// --- serialize ---------------------------------------------------------------

func (t *Tree) serializeXML() ([]byte, error) {
	var b bytes.Buffer
	for _, n := range t.roots {
		writeXMLNode(&b, n, "")
	}
	return b.Bytes(), nil
}

func writeXMLNode(b *bytes.Buffer, n *node, parentNS string) {
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
			writeXMLNode(b, c, n.namespace)
		}
		writeXMLClose(b, n)
	case kindList:
		for _, entry := range n.entries {
			writeXMLOpen(b, n, parentNS)
			for _, c := range entry {
				writeXMLNode(b, c, n.namespace)
			}
			writeXMLClose(b, n)
		}
	}
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
