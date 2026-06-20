package compat

import (
	"testing"

	upstream "github.com/signalbreak-labs/cambium/go/internal/yangparse/upstream/yang"
)

func TestFixChoiceWrapsImplicitCasesLikeGoyangAndMaintainsOrder(t *testing.T) {
	choiceStmt := &Statement{Keyword: "choice", Argument: "select", HasArgument: true}
	leafStmt := &Statement{Keyword: "leaf", Argument: "direct", HasArgument: true}

	upstreamChoiceNode := &upstream.Choice{Name: "select", Source: choiceStmt}
	upstreamLeafNode := &upstream.Leaf{Name: "direct", Source: leafStmt, Parent: upstreamChoiceNode}
	upstreamChoice := &upstream.Entry{
		Name:   "select",
		Kind:   upstream.ChoiceEntry,
		Config: upstream.TSTrue,
		Dir:    map[string]*upstream.Entry{},
	}
	upstreamLeaf := &upstream.Entry{
		Parent: upstreamChoice,
		Node:   upstreamLeafNode,
		Name:   "direct",
		Kind:   upstream.LeafEntry,
		Config: upstream.TSTrue,
	}
	upstreamChoice.Dir[upstreamLeaf.Name] = upstreamLeaf
	upstreamChoice.FixChoice()

	choiceNode := &Choice{Name: "select", Source: choiceStmt}
	leafNode := &Leaf{Name: "direct", Source: leafStmt, Parent: choiceNode}
	choice := &Entry{
		Name:   "select",
		Kind:   ChoiceEntry,
		Config: TSTrue,
		Dir:    map[string]*Entry{},
	}
	leaf := &Entry{
		Parent: choice,
		Node:   leafNode,
		Name:   "direct",
		Kind:   LeafEntry,
		Config: TSTrue,
	}
	choice.add(leaf)
	choice.FixChoice()

	gotCase := choice.Dir["direct"]
	wantCase := upstreamChoice.Dir["direct"]
	if gotCase == nil {
		t.Fatal("FixChoice did not create implicit case entry")
	}
	if gotCase.Name != wantCase.Name || gotCase.Kind.String() != wantCase.Kind.String() {
		t.Fatalf("implicit case = %s/%s, want goyang %s/%s", gotCase.Name, gotCase.Kind, wantCase.Name, wantCase.Kind)
	}
	if gotCase.Node == nil || wantCase.Node == nil || gotCase.Node.Kind() != wantCase.Node.Kind() || gotCase.Node.NName() != wantCase.Node.NName() {
		t.Fatalf("implicit case node = %#v, want goyang %#v", gotCase.Node, wantCase.Node)
	}
	if gotCase.Lookup("direct") != leaf {
		t.Fatalf("implicit case child = %#v, want original leaf", gotCase.Lookup("direct"))
	}
	if leaf.Parent != gotCase {
		t.Fatalf("leaf parent = %#v, want implicit case", leaf.Parent)
	}
	children := choice.Children()
	if len(children) != 1 || children[0] != gotCase {
		t.Fatalf("ordered choice children = %#v, want implicit case", children)
	}
	caseChildren := gotCase.Children()
	if len(caseChildren) != 1 || caseChildren[0] != leaf {
		t.Fatalf("ordered implicit case children = %#v, want original leaf", caseChildren)
	}
}
