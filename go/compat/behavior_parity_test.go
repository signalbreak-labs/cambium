package compat_test

import (
	"fmt"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/signalbreak-labs/cambium/go/compat"
	upstream "github.com/signalbreak-labs/cambium/go/internal/yangparse/upstream/yang"
)

func TestCompatDefaultValuesMatchGoyangTypedefFallback(t *testing.T) {
	source := `module compat-default-parity {
    namespace "urn:compat-default-parity";
    prefix cdp;

    typedef inherited-string {
        type string;
        default "inherited";
    }

    container top {
        leaf inherited {
            type inherited-string;
        }
        leaf explicit {
            type inherited-string;
            default "explicit";
        }
    }
}
`
	compatRoot := mustCompatEntry(t, "compat-default-parity", source)
	upstreamRoot := mustUpstreamEntry(t, "compat-default-parity", source)

	for _, name := range []string{"inherited", "explicit"} {
		t.Run(name, func(t *testing.T) {
			compatLeaf := compatRoot.Lookup("top").Lookup(name)
			upstreamLeaf := upstreamRoot.Dir["top"].Dir[name]
			if compatLeaf == nil || upstreamLeaf == nil {
				t.Fatalf("leaf %q missing: compat=%#v upstream=%#v", name, compatLeaf, upstreamLeaf)
			}
			if got, want := compatLeaf.DefaultValues(), upstreamLeaf.DefaultValues(); !reflect.DeepEqual(got, want) {
				t.Fatalf("%s DefaultValues() = %v, want goyang %v", name, got, want)
			}
			gotDefault, gotOK := compatLeaf.SingleDefaultValue()
			wantDefault, wantOK := upstreamLeaf.SingleDefaultValue()
			if gotDefault != wantDefault || gotOK != wantOK {
				t.Fatalf("%s SingleDefaultValue() = (%q,%v), want goyang (%q,%v)", name, gotDefault, gotOK, wantDefault, wantOK)
			}
		})
	}
}

func TestCompatDefaultValueHelpersMatchGoyangDirectEntries(t *testing.T) {
	tests := []struct {
		name     string
		compat   *compat.Entry
		upstream *upstream.Entry
	}{
		{
			name: "multiple explicit defaults",
			compat: &compat.Entry{
				Node:    &compat.Leaf{Name: "value"},
				Name:    "value",
				Kind:    compat.LeafEntry,
				Default: []string{"one", "two"},
			},
			upstream: &upstream.Entry{
				Node:    &upstream.Leaf{Name: "value"},
				Name:    "value",
				Kind:    upstream.LeafEntry,
				Default: []string{"one", "two"},
			},
		},
		{
			name: "leaf type default",
			compat: &compat.Entry{
				Node: &compat.Leaf{Name: "value"},
				Name: "value",
				Kind: compat.LeafEntry,
				Type: &compat.YangType{HasDefault: true, Default: "inherited"},
			},
			upstream: &upstream.Entry{
				Node: &upstream.Leaf{Name: "value"},
				Name: "value",
				Kind: upstream.LeafEntry,
				Type: &upstream.YangType{HasDefault: true, Default: "inherited"},
			},
		},
		{
			name: "mandatory leaf suppresses type default",
			compat: &compat.Entry{
				Node: &compat.Leaf{Name: "value", Mandatory: &compat.ASTValue{Name: "true"}},
				Name: "value",
				Kind: compat.LeafEntry,
				Type: &compat.YangType{HasDefault: true, Default: "inherited"},
			},
			upstream: &upstream.Entry{
				Node: &upstream.Leaf{Name: "value", Mandatory: &upstream.Value{Name: "true"}},
				Name: "value",
				Kind: upstream.LeafEntry,
				Type: &upstream.YangType{HasDefault: true, Default: "inherited"},
			},
		},
		{
			name: "leaf-list type default",
			compat: &compat.Entry{
				Node:     &compat.Leaf{Name: "values"},
				Name:     "values",
				Kind:     compat.LeafEntry,
				ListAttr: compat.NewDefaultListAttr(),
				Type:     &compat.YangType{HasDefault: true, Default: "inherited"},
			},
			upstream: &upstream.Entry{
				Node:     &upstream.Leaf{Name: "values"},
				Name:     "values",
				Kind:     upstream.LeafEntry,
				ListAttr: upstream.NewDefaultListAttr(),
				Type:     &upstream.YangType{HasDefault: true, Default: "inherited"},
			},
		},
		{
			name: "min-elements leaf-list suppresses type default",
			compat: &compat.Entry{
				Node:     &compat.Leaf{Name: "values"},
				Name:     "values",
				Kind:     compat.LeafEntry,
				ListAttr: &compat.ListAttr{MinElements: 1},
				Type:     &compat.YangType{HasDefault: true, Default: "inherited"},
			},
			upstream: &upstream.Entry{
				Node:     &upstream.Leaf{Name: "values"},
				Name:     "values",
				Kind:     upstream.LeafEntry,
				ListAttr: &upstream.ListAttr{MinElements: 1},
				Type:     &upstream.YangType{HasDefault: true, Default: "inherited"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got, want := tt.compat.DefaultValues(), tt.upstream.DefaultValues(); !reflect.DeepEqual(got, want) {
				t.Fatalf("DefaultValues() = %v, want goyang %v", got, want)
			}
			gotDefault, gotOK := tt.compat.SingleDefaultValue()
			wantDefault, wantOK := tt.upstream.SingleDefaultValue()
			if gotDefault != wantDefault || gotOK != wantOK {
				t.Fatalf("SingleDefaultValue() = (%q,%v), want goyang (%q,%v)", gotDefault, gotOK, wantDefault, wantOK)
			}
		})
	}
}

func TestCompatEntryClassifiersMatchGoyangDirectEntries(t *testing.T) {
	tests := []struct {
		name     string
		compat   *compat.Entry
		upstream *upstream.Entry
	}{
		{
			name: "leaf with dir is not leaf",
			compat: &compat.Entry{
				Name: "value",
				Kind: compat.LeafEntry,
				Dir:  map[string]*compat.Entry{},
			},
			upstream: &upstream.Entry{
				Name: "value",
				Kind: upstream.LeafEntry,
				Dir:  map[string]*upstream.Entry{},
			},
		},
		{
			name: "leaf-list with dir is list",
			compat: &compat.Entry{
				Name:     "values",
				Kind:     compat.LeafEntry,
				Dir:      map[string]*compat.Entry{},
				ListAttr: compat.NewDefaultListAttr(),
			},
			upstream: &upstream.Entry{
				Name:     "values",
				Kind:     upstream.LeafEntry,
				Dir:      map[string]*upstream.Entry{},
				ListAttr: upstream.NewDefaultListAttr(),
			},
		},
		{
			name: "directory list without dir is not list",
			compat: &compat.Entry{
				Name:     "items",
				Kind:     compat.DirectoryEntry,
				ListAttr: compat.NewDefaultListAttr(),
			},
			upstream: &upstream.Entry{
				Name:     "items",
				Kind:     upstream.DirectoryEntry,
				ListAttr: upstream.NewDefaultListAttr(),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := []bool{
				tt.compat.IsDir(),
				tt.compat.IsLeaf(),
				tt.compat.IsLeafList(),
				tt.compat.IsList(),
				tt.compat.IsContainer(),
				tt.compat.IsChoice(),
				tt.compat.IsCase(),
			}
			want := []bool{
				tt.upstream.IsDir(),
				tt.upstream.IsLeaf(),
				tt.upstream.IsLeafList(),
				tt.upstream.IsList(),
				tt.upstream.IsContainer(),
				tt.upstream.IsChoice(),
				tt.upstream.IsCase(),
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("classifiers = %v, want goyang %v", got, want)
			}
		})
	}
}

func TestCompatTypedefBaseChainMatchesGoyang(t *testing.T) {
	source := `module compat-typedef-chain {
    namespace "urn:compat-typedef-chain";
    prefix ctc;

    typedef inner {
        type uint8;
    }
    typedef middle {
        type inner;
    }
    typedef outer {
        type middle;
    }

    leaf value {
        type outer;
    }
}
`
	compatLeaf := mustCompatEntry(t, "compat-typedef-chain", source).Lookup("value")
	upstreamLeaf := mustUpstreamEntry(t, "compat-typedef-chain", source).Dir["value"]
	if compatLeaf == nil || upstreamLeaf == nil {
		t.Fatalf("leaf value missing: compat=%#v upstream=%#v", compatLeaf, upstreamLeaf)
	}
	got := compatYangTypeBaseChain(compatLeaf.Type)
	want := upstreamYangTypeBaseChain(upstreamLeaf.Type)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("typedef Base chain = %v, want goyang %v", got, want)
	}
}

func TestCompatEntryBehaviorMatchesGoyangCommonSchema(t *testing.T) {
	source := `module compat-behavior-parity {
    yang-version 1.1;
    namespace "urn:compat-behavior-parity";
    prefix cbp;

    typedef inherited-string {
        type string;
        default "inherited";
    }

    identity transport;
    identity tcp {
        base transport;
    }

    container z-top {
        description "Top container.";
        config false;

        leaf zed {
            type string;
            default "fallback";
            units "widgets";
        }
        leaf alpha {
            mandatory true;
            type uint32;
        }
        leaf inherited {
            type inherited-string;
        }
        leaf ref-child {
            type leafref {
                path "../zed";
                require-instance false;
            }
        }
        leaf union-child {
            type union {
                type string;
                type uint8;
            }
        }
        leaf decimal-child {
            type decimal64 {
                fraction-digits 2;
                range "0..10.00";
            }
        }
        leaf enum-child {
            type enumeration {
                enum down { value 2; }
                enum up { value 5; }
            }
        }
        leaf bits-child {
            type bits {
                bit read { position 0; }
                bit write { position 3; }
            }
        }
        leaf identity-child {
            type identityref {
                base transport;
            }
        }
        leaf-list tags {
            type string;
            min-elements 1;
            max-elements 3;
            ordered-by user;
        }
        list item {
            key "id";
            min-elements 1;
            max-elements 2;
            ordered-by user;
            leaf id { type string; }
            leaf enabled {
                type boolean;
                default "true";
            }
        }
        choice pick {
            case one {
                leaf selected { type string; }
            }
        }
    }

    rpc reset {
        input {
            leaf zeta { type string; }
            leaf alpha { type string; }
        }
        output {
            leaf result { type string; }
        }
    }

    container a-tail {
        leaf value { type string; }
    }
}
`
	compatRoot := mustCompatEntry(t, "compat-behavior-parity", source)
	upstreamRoot := mustUpstreamEntry(t, "compat-behavior-parity", source)

	if got, want := childNames(compatRoot.Children()), []string{"z-top", "reset", "a-tail"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("compat root Children() = %v, want declaration order %v", got, want)
	}
	if got, want := childNames(compatRoot.Lookup("z-top").Children()), []string{"zed", "alpha", "inherited", "ref-child", "union-child", "decimal-child", "enum-child", "bits-child", "identity-child", "tags", "item", "pick"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("compat z-top Children() = %v, want declaration order %v", got, want)
	}
	if got, want := childNames(compatRoot.Lookup("reset").RPC.Input.Children()), []string{"zeta", "alpha"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("compat RPC input Children() = %v, want declaration order %v", got, want)
	}

	compatSummaries := compatBehaviorSummaries(compatRoot)
	upstreamSummaries := upstreamBehaviorSummaries(upstreamRoot)
	if diff := firstBehaviorSummaryDiff(compatSummaries, upstreamSummaries); diff != "" {
		t.Fatal(diff)
	}
}

func mustCompatEntry(t *testing.T, moduleName, source string) *compat.Entry {
	t.Helper()
	ms := compat.NewModules()
	if err := ms.Parse(source, moduleName+".yang"); err != nil {
		t.Fatalf("compat Parse: %v", err)
	}
	entry, errs := ms.GetModule(moduleName)
	if len(errs) != 0 {
		t.Fatalf("compat GetModule errors: %v", errs)
	}
	if entry == nil {
		t.Fatal("compat GetModule returned nil entry")
	}
	return entry
}

type entryBehaviorSummary struct {
	Name             string
	Kind             string
	Description      string
	DefaultValues    []string
	SingleDefault    string
	HasSingleDefault bool
	Key              string
	MandatoryValue   bool
	ReadOnly         bool
	IsDir            bool
	IsLeaf           bool
	IsLeafList       bool
	IsList           bool
	IsContainer      bool
	IsChoice         bool
	IsCase           bool
	Namespace        string
	List             *listBehaviorSummary
	RPC              *rpcBehaviorSummary
	Type             *typeBehaviorSummary
}

type listBehaviorSummary struct {
	MinElements   uint64
	MaxElements   uint64
	OrderedBy     string
	OrderedByUser bool
}

type rpcBehaviorSummary struct {
	Input  string
	Output string
}

type typeBehaviorSummary struct {
	Kind             string
	HasDefault       bool
	Default          string
	FractionDigits   int
	Length           string
	OptionalInstance bool
	Path             string
	Pattern          []string
	POSIXPattern     []string
	Range            string
	MemberKinds      []string
	EnumNames        []string
	EnumValues       []int64
	BitNames         []string
	BitValues        []int64
	IdentityBase     string
}

func compatBehaviorSummaries(root *compat.Entry) map[string]entryBehaviorSummary {
	out := make(map[string]entryBehaviorSummary)
	var walk func(path string, entry *compat.Entry)
	walk = func(path string, entry *compat.Entry) {
		if entry == nil {
			return
		}
		out[path] = compatEntryBehaviorSummary(entry)
		for _, child := range entry.Children() {
			walk(path+"/"+child.Name, child)
		}
	}
	walk("/"+root.Name, root)
	return out
}

func compatEntryBehaviorSummary(entry *compat.Entry) entryBehaviorSummary {
	single, singleOK := entry.SingleDefaultValue()
	summary := entryBehaviorSummary{
		Name:             entry.Name,
		Kind:             entry.Kind.String(),
		Description:      entry.Description,
		DefaultValues:    entry.DefaultValues(),
		SingleDefault:    single,
		HasSingleDefault: singleOK,
		Key:              entry.Key,
		MandatoryValue:   entry.Mandatory.Value(),
		ReadOnly:         entry.ReadOnly(),
		IsDir:            entry.IsDir(),
		IsLeaf:           entry.IsLeaf(),
		IsLeafList:       entry.IsLeafList(),
		IsList:           entry.IsList(),
		IsContainer:      entry.IsContainer(),
		IsChoice:         entry.IsChoice(),
		IsCase:           entry.IsCase(),
		Namespace:        valueName(entry.Namespace()),
		List:             compatListBehaviorSummary(entry.ListAttr),
		RPC:              compatRPCBehaviorSummary(entry.RPC),
		Type:             compatTypeBehaviorSummary(entry.Type),
	}
	return summary
}

func compatListBehaviorSummary(attr *compat.ListAttr) *listBehaviorSummary {
	if attr == nil {
		return nil
	}
	return &listBehaviorSummary{
		MinElements:   attr.MinElements,
		MaxElements:   attr.MaxElements,
		OrderedBy:     valueName(attr.OrderedBy),
		OrderedByUser: attr.OrderedByUser,
	}
}

func compatRPCBehaviorSummary(rpc *compat.RPCEntry) *rpcBehaviorSummary {
	if rpc == nil {
		return nil
	}
	return &rpcBehaviorSummary{
		Input:  entryName(rpc.Input),
		Output: entryName(rpc.Output),
	}
}

func compatTypeBehaviorSummary(typ *compat.YangType) *typeBehaviorSummary {
	if typ == nil {
		return nil
	}
	summary := &typeBehaviorSummary{
		Kind:             typ.Kind.String(),
		HasDefault:       typ.HasDefault,
		Default:          typ.Default,
		FractionDigits:   typ.FractionDigits,
		Length:           typ.Length.String(),
		OptionalInstance: typ.OptionalInstance,
		Path:             typ.Path,
		Pattern:          append([]string(nil), typ.Pattern...),
		POSIXPattern:     append([]string(nil), typ.POSIXPattern...),
		Range:            typ.Range.String(),
		MemberKinds:      compatTypeKinds(typ.Type),
	}
	if typ.Enum != nil {
		summary.EnumNames = typ.Enum.Names()
		summary.EnumValues = typ.Enum.Values()
	}
	if typ.Bit != nil {
		summary.BitNames = typ.Bit.Names()
		summary.BitValues = typ.Bit.Values()
	}
	if typ.IdentityBase != nil {
		summary.IdentityBase = typ.IdentityBase.Name
	}
	return summary
}

func upstreamBehaviorSummaries(root *upstream.Entry) map[string]entryBehaviorSummary {
	out := make(map[string]entryBehaviorSummary)
	var walk func(path string, entry *upstream.Entry)
	walk = func(path string, entry *upstream.Entry) {
		if entry == nil {
			return
		}
		out[path] = upstreamEntryBehaviorSummary(entry)
		for _, name := range sortedUpstreamChildNames(entry) {
			walk(path+"/"+name, entry.Dir[name])
		}
		if entry.RPC != nil {
			if entry.RPC.Input != nil {
				walk(path+"/"+entry.RPC.Input.Name, entry.RPC.Input)
			}
			if entry.RPC.Output != nil {
				walk(path+"/"+entry.RPC.Output.Name, entry.RPC.Output)
			}
		}
	}
	walk("/"+root.Name, root)
	return out
}

func upstreamEntryBehaviorSummary(entry *upstream.Entry) entryBehaviorSummary {
	single, singleOK := entry.SingleDefaultValue()
	summary := entryBehaviorSummary{
		Name:             entry.Name,
		Kind:             entry.Kind.String(),
		Description:      entry.Description,
		DefaultValues:    entry.DefaultValues(),
		SingleDefault:    single,
		HasSingleDefault: singleOK,
		Key:              entry.Key,
		MandatoryValue:   entry.Mandatory.Value(),
		ReadOnly:         entry.ReadOnly(),
		IsDir:            entry.IsDir(),
		IsLeaf:           entry.IsLeaf(),
		IsLeafList:       entry.IsLeafList(),
		IsList:           entry.IsList(),
		IsContainer:      entry.IsContainer(),
		IsChoice:         entry.IsChoice(),
		IsCase:           entry.IsCase(),
		Namespace:        upstreamValueName(entry.Namespace()),
		List:             upstreamListBehaviorSummary(entry.ListAttr),
		RPC:              upstreamRPCBehaviorSummary(entry.RPC),
		Type:             upstreamTypeBehaviorSummary(entry.Type),
	}
	return summary
}

func upstreamListBehaviorSummary(attr *upstream.ListAttr) *listBehaviorSummary {
	if attr == nil {
		return nil
	}
	return &listBehaviorSummary{
		MinElements:   attr.MinElements,
		MaxElements:   attr.MaxElements,
		OrderedBy:     upstreamValueName(attr.OrderedBy),
		OrderedByUser: attr.OrderedByUser,
	}
}

func upstreamRPCBehaviorSummary(rpc *upstream.RPCEntry) *rpcBehaviorSummary {
	if rpc == nil {
		return nil
	}
	return &rpcBehaviorSummary{
		Input:  upstreamEntryName(rpc.Input),
		Output: upstreamEntryName(rpc.Output),
	}
}

func upstreamTypeBehaviorSummary(typ *upstream.YangType) *typeBehaviorSummary {
	if typ == nil {
		return nil
	}
	summary := &typeBehaviorSummary{
		Kind:             typ.Kind.String(),
		HasDefault:       typ.HasDefault,
		Default:          typ.Default,
		FractionDigits:   typ.FractionDigits,
		Length:           typ.Length.String(),
		OptionalInstance: typ.OptionalInstance,
		Path:             typ.Path,
		Pattern:          append([]string(nil), typ.Pattern...),
		POSIXPattern:     append([]string(nil), typ.POSIXPattern...),
		Range:            typ.Range.String(),
		MemberKinds:      upstreamTypeKinds(typ.Type),
	}
	if typ.Enum != nil {
		summary.EnumNames = typ.Enum.Names()
		summary.EnumValues = typ.Enum.Values()
	}
	if typ.Bit != nil {
		summary.BitNames = typ.Bit.Names()
		summary.BitValues = typ.Bit.Values()
	}
	if typ.IdentityBase != nil {
		summary.IdentityBase = typ.IdentityBase.Name
	}
	return summary
}

func firstBehaviorSummaryDiff(got, want map[string]entryBehaviorSummary) string {
	gotPaths := sortedSummaryPaths(got)
	wantPaths := sortedSummaryPaths(want)
	if !reflect.DeepEqual(gotPaths, wantPaths) {
		return "compat entry paths differ from goyang\ngot:  " + strings.Join(gotPaths, ", ") + "\nwant: " + strings.Join(wantPaths, ", ")
	}
	for _, path := range wantPaths {
		if !reflect.DeepEqual(got[path], want[path]) {
			return path + " summary differs from goyang\ngot:  " + formatEntryBehaviorSummary(got[path]) + "\nwant: " + formatEntryBehaviorSummary(want[path])
		}
	}
	return ""
}

func formatEntryBehaviorSummary(summary entryBehaviorSummary) string {
	return fmt.Sprintf("%#v", struct {
		Entry entryBehaviorSummary
		List  listBehaviorSummary
		RPC   rpcBehaviorSummary
		Type  typeBehaviorSummary
	}{
		Entry: summary,
		List:  derefListBehaviorSummary(summary.List),
		RPC:   derefRPCBehaviorSummary(summary.RPC),
		Type:  derefTypeBehaviorSummary(summary.Type),
	})
}

func derefListBehaviorSummary(summary *listBehaviorSummary) listBehaviorSummary {
	if summary == nil {
		return listBehaviorSummary{}
	}
	return *summary
}

func derefRPCBehaviorSummary(summary *rpcBehaviorSummary) rpcBehaviorSummary {
	if summary == nil {
		return rpcBehaviorSummary{}
	}
	return *summary
}

func derefTypeBehaviorSummary(summary *typeBehaviorSummary) typeBehaviorSummary {
	if summary == nil {
		return typeBehaviorSummary{}
	}
	return *summary
}

func sortedSummaryPaths(summaries map[string]entryBehaviorSummary) []string {
	paths := make([]string, 0, len(summaries))
	for path := range summaries {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	return paths
}

func sortedUpstreamChildNames(entry *upstream.Entry) []string {
	if entry == nil || entry.Dir == nil {
		return nil
	}
	names := make([]string, 0, len(entry.Dir))
	for name := range entry.Dir {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func compatTypeKinds(types []*compat.YangType) []string {
	out := make([]string, 0, len(types))
	for _, typ := range types {
		if typ == nil {
			out = append(out, "")
			continue
		}
		out = append(out, typ.Kind.String())
	}
	return out
}

func upstreamTypeKinds(types []*upstream.YangType) []string {
	out := make([]string, 0, len(types))
	for _, typ := range types {
		if typ == nil {
			out = append(out, "")
			continue
		}
		out = append(out, typ.Kind.String())
	}
	return out
}

func compatYangTypeBaseChain(typ *compat.YangType) []string {
	if typ == nil {
		return nil
	}
	out := []string{typ.Name}
	if typ.Base == nil {
		return out
	}
	out = append(out, typ.Base.Name)
	seen := map[*upstream.YangType]bool{}
	for cur := typ.Base.YangType; cur != nil && !seen[cur]; cur = cur.Base.YangType {
		seen[cur] = true
		if cur.Base == nil {
			break
		}
		out = append(out, cur.Base.Name)
	}
	return out
}

func upstreamYangTypeBaseChain(typ *upstream.YangType) []string {
	if typ == nil {
		return nil
	}
	out := []string{typ.Name}
	seen := map[*upstream.YangType]bool{}
	for cur := typ; cur != nil && !seen[cur]; cur = cur.Base.YangType {
		seen[cur] = true
		if cur.Base == nil {
			break
		}
		out = append(out, cur.Base.Name)
	}
	return out
}

func valueName(value *compat.Value) string {
	if value == nil {
		return ""
	}
	return value.Name
}

func upstreamValueName(value *upstream.Value) string {
	if value == nil {
		return ""
	}
	return value.Name
}

func entryName(entry *compat.Entry) string {
	if entry == nil {
		return ""
	}
	return entry.Name
}

func upstreamEntryName(entry *upstream.Entry) string {
	if entry == nil {
		return ""
	}
	return entry.Name
}

func mustUpstreamEntry(t *testing.T, moduleName, source string) *upstream.Entry {
	t.Helper()
	ms := upstream.NewModules()
	if err := ms.Parse(source, moduleName+".yang"); err != nil {
		t.Fatalf("upstream Parse: %v", err)
	}
	entry, errs := ms.GetModule(moduleName)
	if len(errs) != 0 {
		t.Fatalf("upstream GetModule errors: %v", errs)
	}
	if entry == nil {
		t.Fatal("upstream GetModule returned nil entry")
	}
	return entry
}
