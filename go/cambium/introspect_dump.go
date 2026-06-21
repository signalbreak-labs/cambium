package cambium

import (
	"fmt"
	"io"
	"strconv"
	"strings"
)

// Schema introspection dumps. These render a loaded Module as human-readable
// text by walking the ordered schema IR — never map iteration — so the output
// is deterministic and reflects effective schema declaration order (invariants
// I2/I3/I4). They are read-only consumers of the IR and cannot reorder it.
//
// WriteTree renders an indented schema tree; WriteTypes renders a per-leaf
// resolved-type inventory. Both are pure Go (no cgo).

// dumpWriter coalesces write errors so the render helpers stay readable: once a
// write fails, subsequent printf calls are no-ops and the first error is kept.
type dumpWriter struct {
	w   io.Writer
	err error
}

func (d *dumpWriter) printf(format string, args ...any) {
	if d.err != nil {
		return
	}
	_, d.err = fmt.Fprintf(d.w, format, args...)
}

// WriteTree renders m as an indented schema tree in effective schema
// declaration order. Each line is "<kind> <name> [flags...] [: <type>]";
// siblings appear in declaration order, list keys are annotated in
// key-statement order, and ordered-by-user lists are marked. Output is
// deterministic.
func WriteTree(w io.Writer, m Module) error {
	d := &dumpWriter{w: w}
	d.printf("module: %s\n", m.Name())
	writeTreeChildren(d, m.TopLevel(), 1)
	writeTreeChildren(d, m.RPCs(), 1)
	writeTreeChildren(d, m.Notifications(), 1)
	return d.err
}

func writeTreeChildren(d *dumpWriter, children SchemaChildren, depth int) {
	for n := range children.Iter() {
		writeTreeNode(d, n, depth)
	}
}

func writeTreeNode(d *dumpWriter, n SchemaNodeRef, depth int) {
	line := strings.Repeat("  ", depth) + nodeKindWord(n) + " " + n.Name()
	if flags := nodeTreeFlags(n); flags != "" {
		line += " " + flags
	}
	if n.IsLeaf() || n.IsLeafList() {
		line += " : " + treeTypeName(n)
	}
	d.printf("%s\n", line)
	writeTreeChildren(d, n.Children(), depth+1)
}

// WriteTypes renders the resolved type of every leaf and leaf-list in m, in
// declaration order. Enumeration and bits values are listed in their YANG
// declaration order (the order encoders use), making the ordering observable;
// union members are listed in member order.
func WriteTypes(w io.Writer, m Module) error {
	d := &dumpWriter{w: w}
	d.printf("types: %s\n", m.Name())
	writeTypesChildren(d, m.TopLevel())
	writeTypesChildren(d, m.RPCs())
	writeTypesChildren(d, m.Notifications())
	return d.err
}

func writeTypesChildren(d *dumpWriter, children SchemaChildren) {
	for n := range children.Iter() {
		if n.IsLeaf() || n.IsLeafList() {
			if ti, ok := n.LeafType(); ok {
				d.printf("  %s : %s\n", n.Path(), describeType(ti))
			}
		}
		writeTypesChildren(d, n.Children())
	}
}

func nodeKindWord(n SchemaNodeRef) string {
	switch {
	case n.IsLeafList():
		return "leaf-list"
	case n.IsLeaf():
		return "leaf"
	case n.IsList():
		return "list"
	case n.IsContainer():
		return "container"
	case n.IsChoice():
		return "choice"
	case n.IsCase():
		return "case"
	case n.IsRPC():
		return "rpc"
	case n.IsAction():
		return "action"
	case n.IsNotification():
		return "notification"
	default:
		return "node"
	}
}

// nodeTreeFlags returns the bracketed annotations for a node in a stable order:
// config, list keys, ordered-by-user, presence, mandatory.
func nodeTreeFlags(n SchemaNodeRef) string {
	var flags []string
	if n.ReadOnly() {
		flags = append(flags, "ro")
	} else {
		flags = append(flags, "rw")
	}
	if n.IsList() {
		if keys := n.KeyNames(); len(keys) > 0 {
			flags = append(flags, "key: "+strings.Join(keys, " "))
		}
	}
	if (n.IsList() || n.IsLeafList()) && n.OrderedBy() == OrderedByUser {
		flags = append(flags, "ordered-by user")
	}
	if n.IsPresenceContainer() {
		flags = append(flags, "presence")
	}
	if n.IsMandatory() {
		flags = append(flags, "mandatory")
	}
	for i, f := range flags {
		flags[i] = "[" + f + "]"
	}
	return strings.Join(flags, " ")
}

func treeTypeName(n SchemaNodeRef) string {
	ti, ok := n.LeafType()
	if !ok {
		return "unknown"
	}
	if name, ok := ti.TypedefName(); ok && name != "" {
		return name
	}
	return ti.Base().String()
}

// describeType renders a resolved type for the types inventory: the typedef
// chain (if any) and base, plus the order-bearing detail for enums, bits,
// leafrefs, decimal64, and unions.
func describeType(ti TypeInfo) string {
	var sb strings.Builder
	if chain := ti.TypedefChain(); len(chain) > 0 {
		sb.WriteString(strings.Join(chain, "->"))
		sb.WriteString(" (" + ti.Base().String() + ")")
	} else {
		sb.WriteString(ti.Base().String())
	}
	switch r := ti.Resolved().(type) {
	case ResolvedEnumeration:
		sb.WriteString(" {" + enumValueList(r.Values()) + "}")
	case ResolvedBits:
		sb.WriteString(" {" + enumValueList(r.Values()) + "}")
	case ResolvedLeafRef:
		if path, ok := r.Path(); ok {
			sb.WriteString(" -> " + path)
		}
	case ResolvedDecimal64:
		sb.WriteString(" fraction-digits ")
		sb.WriteString(strconv.Itoa(int(r.FractionDigits().Value())))
	case ResolvedUnion:
		members := r.Members()
		rendered := make([]string, len(members))
		for i, m := range members {
			rendered[i] = describeType(m)
		}
		sb.WriteString(" union(" + strings.Join(rendered, " | ") + ")")
	}
	return sb.String()
}

func enumValueList(values []EnumValue) string {
	parts := make([]string, len(values))
	for i, v := range values {
		parts[i] = fmt.Sprintf("%s=%d", v.Name(), v.Value())
	}
	return strings.Join(parts, ", ")
}
