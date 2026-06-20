# Schema introspection with package `cambium`

This guide is a hands-on walkthrough of the pure-Go `cambium` package: how to
build a loading context, get a `Module` handle, and walk the ordered schema tree
to read children, list keys, node kinds, and leaf types. Everything here lives in
the **Schema-IR tier (pure Go, `CGO_ENABLED=0`)** — no cgo, no libyang, no C
toolchain. The defining property of this tier is that the schema tree remembers
**effective schema declaration order** (invariant I2), exposes **list keys first
in `key`-statement order** (invariant I3), and keeps RPC/action/notification I/O
in schema order (invariant I4). Those orderings are what NETCONF-facing
serialization and typed-struct codegen depend on, so the API is built so that the
ordered sibling sequence — not a map — is the only thing you traverse.

## Building a Context

A `Context` is a loaded, frozen set of YANG modules. There are two ways to build
one, and they differ only in lifecycle ergonomics.

### Builder path (build once, then freeze)

`NewContextBuilder` gives you a mutable phase: load every module you need, then
call `Build()` to get an immutable `*Context`. This mirrors the "build-once,
then-frozen" contract used across Cambium's tiers.

```go
package main

import (
	"log"

	"github.com/signalbreak-labs/cambium/go/cambium"
)

func loadFromBuilder() (*cambium.Context, error) {
	b, err := cambium.NewContextBuilder(cambium.ContextFlags{})
	if err != nil {
		return nil, err
	}
	// Add a search directory, then load by module name (imports resolve
	// against the search path)...
	if err := b.SearchPath("./yang"); err != nil {
		return nil, err
	}
	if err := b.LoadModule("acme-vlans", nil, nil); err != nil {
		return nil, err
	}
	// ...or load a file directly, or from an in-memory string.
	if err := b.LoadModuleFromPath("./yang/acme-acls.yang"); err != nil {
		return nil, err
	}
	ctx, err := b.Build()
	if err != nil {
		return nil, err
	}
	return ctx, nil
}

func main() {
	ctx, err := loadFromBuilder()
	if err != nil {
		log.Fatal(err)
	}
	defer ctx.Close()
}
```

`LoadModule(name string, revision *string, features []string)` takes an optional
revision (pass `nil` for "newest available") and an optional `if-feature`
allow-list (pass `nil` to leave features at their defaults). `LoadModuleStr`
loads YANG from a source string, which is convenient in tests.

`ContextFlags` controls loading behavior. The zero value is the common case; the
fields are `NoYangLibrary`, `AllImplemented`, `RefImplemented`, and
`DisableSearchdirCwd`.

### Runtime path (load incrementally)

`NewContext` returns a `*Context` you can load into directly, without a separate
builder. This is the shorter path when you are loading interactively or driving
codegen.

```go
ctx, err := cambium.NewContext()
if err != nil {
	log.Fatal(err)
}
defer ctx.Close()

if err := ctx.SetSearchPath("./yang"); err != nil {
	log.Fatal(err)
}
if err := ctx.LoadModule("acme-vlans"); err != nil {
	log.Fatal(err)
}
// Or load a specific file:
// err = ctx.LoadModuleFromPath("./yang/acme-vlans.yang")
```

Always call `ctx.Close()` when you are done with a context.

## Getting a Module

`Schema` is the main handle into the ordered tree. It returns a `Module` — a
borrowed, read-only view of a loaded module.

```go
mod, err := ctx.Schema("acme-vlans")
if err != nil {
	log.Fatal(err)
}
```

If you need to pin a specific revision, `GetModule(name string, revision *string)
(Module, bool)` and `SchemaRevision(module, revision string)` are also available,
and `Modules()` returns every loaded module.

## Walking the ordered tree

A `Module` exposes several ordered entry points, each returning a
`SchemaChildren` cursor in declaration order:

- `TopLevel()` — top-level **data** nodes (the schema-declaration-order list of
  containers, lists, leaves, etc.) — I2.
- `Children()` — top-level children including `choice`/`case` structure.
- `RPCs()`, `Actions()`, `Notifications()` — operation roots in declaration
  order (I4).

`SchemaChildren` is a small cursor, not a slice you index by guessing. Its
methods are `Len()`, `IsEmpty()`, `Get(i int) (SchemaNodeRef, bool)`,
`Lookup(name string) (SchemaNodeRef, bool)`, and `Iter() iter.Seq[SchemaNodeRef]`
for `range`-over-func iteration. `Lookup` is a derived name → node-identity cache;
traversal and ordering always come from `Iter`/`Get`, never from the lookup index.

Each element is a `SchemaNodeRef` — a handle to one node. To descend, a node
gives you two ordered child views:

- `Children()` — all children, including `choice` and `case` wrapper nodes, in
  declaration order.
- `DataChildren(flattenChoices bool)` — only data nodes, in declaration order.
  When `flattenChoices` is `true`, the data children of `choice`/`case` are
  spliced in at the choice's position, so you see the flat data shape a NETCONF
  server expects without manually unwrapping cases.

> Use `DataChildren(true)` when you want the on-the-wire data shape; use
> `Children()` when you care about the `choice`/`case` structure itself.

## Reading list keys first (I3)

For a `list` node, the key leaves are emitted before any non-key child, in
`key`-statement order. Two methods expose this directly:

- `KeyNames() []string` — the key names, in order.
- `ListKeys() SchemaChildren` — the key leaves themselves, as an ordered cursor.

Reading keys from these methods (rather than filtering `DataChildren` yourself)
keeps your traversal aligned with how serialization and codegen emit list
entries.

## Classifying nodes

`SchemaNodeRef.Kind()` returns a `SchemaNodeKind`, one of `SchemaNodeKindModule`,
`Container`, `Leaf`, `LeafList`, `List`, `Choice`, `Case`, `AnyData`, `RPC`,
`Action`, `Input`, `Output`, `Notification`, or `Unknown`. There are also
boolean predicates for the common checks — `IsContainer()`, `IsList()`,
`IsLeaf()`, `IsLeafList()`, `IsChoice()`, `IsCase()`, `IsListKey()`,
`IsMandatory()`, and more — plus `Config()` (`ConfigRw`/`ConfigRo`), `Status()`,
`OrderedBy()` (`OrderedBySystem`/`OrderedByUser`), `Path()`, and `Parent()`.

For operation roots, `Input()` and `Output()` return the `input`/`output`
subtrees of an RPC or action; their children are themselves in schema order (I4).

## Reading leaf types

For a `leaf` or `leaf-list`, `LeafType() (TypeInfo, bool)` returns rich type
information (the second return is `false` for non-leaf nodes). `TypeInfo` gives
you two levels of detail:

- `Base() BaseType` — the coarse base type (`BaseTypeString`, `BaseTypeUint16`,
  `BaseTypeEnumeration`, ...). `BaseType` has a `String()` method, so it prints
  cleanly.
- `Resolved() ResolvedType` — the fully resolved type with its constraints,
  after the typedef chain is collapsed. `TypedefChain()` and `TypedefName()`
  report where it came from.

`ResolvedType` is a sum-type interface; switch on the concrete variant to read
type-specific detail. The variants are `ResolvedString` (`Length`, `Patterns`),
`ResolvedInt` (`Kind IntKind`, `Range []RangeBound`), `ResolvedDecimal64`
(`FractionDigits()`, `Range`), `ResolvedBoolean`, `ResolvedEmpty`,
`ResolvedBinary`, `ResolvedBits`, `ResolvedEnumeration` (`Values()`),
`ResolvedIdentityRef`, `ResolvedInstanceIdentifier`, `ResolvedLeafRef`,
`ResolvedUnion`, and `ResolvedUnknown`.

## A worked example

The following walks one module end to end: build a context from an in-memory
module, get the `Module`, descend into a `list`, read its keys first, then
classify and type each data child. Given this module:

```text
module acme-vlans {
  namespace "urn:acme:vlans";
  prefix av;

  container vlans {
    list vlan {
      key "id name";
      ordered-by user;

      leaf id   { type uint16 { range "1..4094"; } }
      leaf name { type string { length "1..32"; } }

      leaf admin-status {
        type enumeration { enum up; enum down; }
      }
    }
  }
}
```

the program below prints the keys before the non-key leaves, in declaration
order, and resolves each leaf's type:

```go
package main

import (
	"fmt"
	"log"

	"github.com/signalbreak-labs/cambium/go/cambium"
)

func main() {
	b, err := cambium.NewContextBuilder(cambium.ContextFlags{})
	if err != nil {
		log.Fatal(err)
	}
	if err := b.LoadModuleStr(moduleSource); err != nil {
		log.Fatal(err)
	}
	ctx, err := b.Build()
	if err != nil {
		log.Fatal(err)
	}
	defer ctx.Close()

	mod, err := ctx.Schema("acme-vlans")
	if err != nil {
		log.Fatal(err)
	}

	// Top-level data nodes, in schema declaration order (I2).
	for top := range mod.TopLevel().Iter() {
		fmt.Printf("%s\n", top.Path())

		vlan, ok := top.Children().Lookup("vlan")
		if !ok {
			continue
		}

		// I3: list keys first, in key-statement order.
		fmt.Printf("  list %q keys=%v ordered-by-user=%v\n",
			vlan.Name(), vlan.KeyNames(), vlan.OrderedBy() == cambium.OrderedByUser)

		// DataChildren(true) yields keys first, then the remaining data
		// children, all in declaration order.
		for child := range vlan.DataChildren(true).Iter() {
			describe(child)
		}
	}
}

func describe(n cambium.SchemaNodeRef) {
	role := "child"
	if n.IsListKey() {
		role = "key"
	}

	if n.Kind() != cambium.SchemaNodeKindLeaf && n.Kind() != cambium.SchemaNodeKindLeafList {
		fmt.Printf("    %-3s %s\n", role, n.Name())
		return
	}

	ti, _ := n.LeafType()
	fmt.Printf("    %-3s %s : %s\n", role, n.Name(), ti.Base())

	switch rt := ti.Resolved().(type) {
	case cambium.ResolvedString:
		for _, l := range rt.Length {
			fmt.Printf("        length %s..%s\n", l.Min(), l.Max())
		}
	case cambium.ResolvedInt:
		for _, r := range rt.Range {
			fmt.Printf("        range %s..%s\n", r.Min(), r.Max())
		}
	case cambium.ResolvedEnumeration:
		for _, v := range rt.Values() {
			fmt.Printf("        enum %s = %d\n", v.Name(), v.Value())
		}
	}
}
```

This prints:

```text
/acme-vlans/vlans
  list "vlan" keys=[id name] ordered-by-user=true
    key id : uint16
        range 1..4094
    key name : string
        length 1..32
    child admin-status : enumeration
        enum up = 0
        enum down = 1
```

Note what the API does for you: `id` and `name` come out before
`admin-status` because they are keys (I3), and the whole sequence is the module's
declaration order (I2) — read straight off the ordered tree, never sorted or
pulled from a map.

## Legacy surface

`Context.SchemaTree(module string) (*SchemaTree, error)` and the `SchemaTree` /
`SchemaNode` types are an earlier, retained read surface. New code should prefer
`Schema()` → `Module` → `SchemaNodeRef`, which is the supported ordered-tree API
this guide covers.

## See also

- [Documentation index](../README.md)
- [Why Cambium](../why-cambium.md)
- [Architecture](../architecture.md)
- [Typed-struct codegen guide](./codegen.md)
- [Migrating from goyang (`compat`)](./goyang-migration.md)
- [Ordering invariants (spec)](../../spec/ordering-invariants.md)
- [API contract (spec)](../../spec/api.md)
