// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

package datatree

import "strings"

// Node is a read-only view of one data node in a Tree. Accessors preserve the
// tree's order (children and list entries are returned in serialization order:
// schema declaration order, keys first), so callers never see map iteration.
type Node struct {
	n *node
}

func wrapNodes(ns []*node) []Node {
	out := make([]Node, len(ns))
	for i, n := range ns {
		out[i] = Node{n: n}
	}
	return out
}

// RootNodes returns the top-level data nodes in schema declaration order.
func (t *Tree) RootNodes() []Node {
	return wrapNodes(t.roots)
}

// Name is the node's schema-local name.
func (nd Node) Name() string { return nd.n.name }

// Module is the name of the module that defines this node.
func (nd Node) Module() string { return nd.n.module }

// IsLeaf reports whether the node is a leaf.
func (nd Node) IsLeaf() bool { return nd.n.kind == kindLeaf }

// IsLeafList reports whether the node is a leaf-list.
func (nd Node) IsLeafList() bool { return nd.n.kind == kindLeafList }

// IsContainer reports whether the node is a container.
func (nd Node) IsContainer() bool { return nd.n.kind == kindContainer }

// IsList reports whether the node is a list.
func (nd Node) IsList() bool { return nd.n.kind == kindList }

// LeafValue returns a leaf's value as its raw encoded token (for the JSON_IETF
// source this is the JSON text, e.g. `"hi"`, `7`, or `true`), and false if the
// node is not a leaf.
func (nd Node) LeafValue() (string, bool) {
	if nd.n.kind != kindLeaf {
		return "", false
	}
	return string(nd.n.value), true
}

// LeafListValues returns a leaf-list's values in order, each as its raw encoded
// token; nil if the node is not a leaf-list.
func (nd Node) LeafListValues() []string {
	if nd.n.kind != kindLeafList {
		return nil
	}
	out := make([]string, len(nd.n.values))
	for i, v := range nd.n.values {
		out[i] = string(v)
	}
	return out
}

// Children returns a container's child nodes in declaration order; nil if the
// node is not a container.
func (nd Node) Children() []Node {
	if nd.n.kind != kindContainer {
		return nil
	}
	return wrapNodes(nd.n.children)
}

// Entries returns a list's entries in document order; each entry is its child
// nodes with keys first. nil if the node is not a list.
func (nd Node) Entries() [][]Node {
	if nd.n.kind != kindList {
		return nil
	}
	out := make([][]Node, len(nd.n.entries))
	for i, e := range nd.n.entries {
		out[i] = wrapNodes(e)
	}
	return out
}

// Find returns the node at a slash-separated path of container/leaf names, e.g.
// "/c/z" or "c/z" (a leading slash is optional). It descends containers only;
// lists are not indexed (a list segment yields false). The lookup is by name; a
// name index per level is built on demand and is never the traversal source.
func (t *Tree) Find(path string) (Node, bool) {
	segments := splitPath(path)
	if len(segments) == 0 {
		return Node{}, false
	}
	level := t.roots
	for i, seg := range segments {
		match := findByName(level, seg)
		if match == nil {
			return Node{}, false
		}
		if i == len(segments)-1 {
			return Node{n: match}, true
		}
		if match.kind != kindContainer {
			return Node{}, false // cannot descend into a non-container mid-path
		}
		level = match.children
	}
	return Node{}, false
}

func splitPath(path string) []string {
	var out []string
	for _, seg := range strings.Split(path, "/") {
		if seg != "" {
			out = append(out, seg)
		}
	}
	return out
}

func findByName(nodes []*node, name string) *node {
	for _, n := range nodes {
		if n.name == name {
			return n
		}
	}
	return nil
}
