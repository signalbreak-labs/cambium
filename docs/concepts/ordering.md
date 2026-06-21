# Ordering

Order correctness is Cambium's reason to exist. This page explains what "order"
means in YANG, why it is load-bearing rather than cosmetic, the single design rule
Cambium follows to preserve it, the four ordering facets and how each is handled,
and the six normative invariants (I1–I6) that pin the behavior down — including
which tier guarantees which. The normative text lives in
[`/spec/ordering-invariants.md`](../../spec/ordering-invariants.md); this page is
the explanation behind it.

## Why order is load-bearing

In YANG, the order in which sibling nodes are declared is part of the model's
meaning. RFC 7950 §7.8.5 specifies that the children of a container or list entry
appear in schema declaration order, and many NETCONF servers expect exactly that
order on the wire. A model that declares leaves `z`, `m`, `a` describes a tree
whose children are `z`, `m`, `a` — in that sequence.

Consider this module (it is a real conformance fixture,
`conformance/fixtures/scrambled-children/module/order-demo.yang`):

```yang
module order-demo {
  namespace "urn:order-demo";
  prefix od;
  container top {
    leaf z { type string; }
    leaf m { type string; }
    leaf a { type string; }
  }
}
```

The children of `top` are `z, m, a`. A toolkit that stores children in a hash map
and returns them by iteration order will hand you `a, m, z` (alphabetical) or some
arbitrary order — and serialized output built from that order is wrong in a way
that is hard to notice, because the data still "looks right." Only the sequence is
off, and a strict NETCONF server rejects it.

Two workflows depend directly on preserving declaration order:

- **Faithful serialization.** XML and JSON_IETF output a device accepts must
  present children in declared order.
- **Typed-struct codegen.** Generated structs, their field-order manifests, and
  their serializers all walk the schema in declared order so the bytes they emit
  match what the model — and the device — expect.

## The one rule

> Order is a **structural property of the tree** — never a sort key, sidecar, or
> map.

The ordered sibling sequence *is* the data structure. Any keyed index Cambium keeps
is a derived lookup cache that maps a key to a node's **identity**, never to its
position, and it is never consulted for traversal, codegen, or serialization.
Because the index records identity rather than position, moving a node positionally
does not invalidate it.

This is an engineering rule with teeth, not a slogan:

- **Traversal walks ordered slices.** `(Module).TopLevel()`,
  `(SchemaNodeRef).Children()`, and `DataChildren(...)` read from an ordered
  representation. A `childByName` cache may exist for lookup, but it is never the
  traversal source. The codebase forbids deriving child order from a Go map, a
  reflected struct order, a goyang `Entry.Dir`, or goyang's kind-separated typed
  slices (`Leaf []*Leaf`, `Container []*Container`).
- **Serialization is one ordered walk.** The libyang backend walks libyang's
  `lyd_node.next/prev` sibling chain; native codegen serializers walk Cambium's
  field-order manifest; the pure-Go `datatree` walks its ordered node slices. None
  iterates a native map or reflected struct order.

Here is the rule in action — walking `top`'s children yields declaration order:

```go
b, _ := cambium.NewContextBuilder(cambium.ContextFlags{})
_ = b.LoadModuleStr(orderDemo) // the module above
ctx, _ := b.Build()
mod, _ := ctx.Schema("order-demo")

top, _ := mod.TopLevel().Lookup("top")
for child := range top.Children().Iter() {
	fmt.Println(child.Name()) // z, m, a — declaration order, not a, m, z
}
```

## The four ordering facets

YANG order is not a single rule. Cambium handles four distinct facets explicitly,
plus `ordered-by system`, which it deliberately canonicalizes.

### 1. Child / sibling node order

The children of a container or list entry — leaves, nested containers, and the
children spliced in by `uses`/grouping expansion — follow **effective schema
declaration order**. This is the RFC-canonical form (RFC 7950 §7.8.5). "Effective"
means after grouping expansion, augment application, and deviation processing; the
placement rules are specified in
[`/spec/ordering-invariants.md` §1.1](../../spec/ordering-invariants.md). `uses`
expands at the exact position of the `uses` statement; augment children are inserted
after the target's already-declared children; deviations modify in place without
reordering unaffected siblings.

### 2. `ordered-by user` entries and values

List entries and leaf-list values declared `ordered-by user` carry caller insertion
order as **semantically significant data** (RFC 7950 §7.7.7). The server must
maintain that order, and Cambium preserves it byte-exactly across parse → tree →
serialize. In generated code, a user-ordered node is a positional-only type with
positional mutators (insert/move), so reordering a *system*-ordered node is a
compile-time impossibility rather than a runtime check you might forget.

### 3. List keys first, in key order

A list's key leaves are emitted **first**, in `key`-statement order (RFC 7950
§7.8.5), before any non-key child — even if the keys are declared after non-key
leaves in the source. `(SchemaNodeRef).ListKeys()` and `KeyNames()` expose them in
that order, and encoders emit them accordingly.

### 4. RPC / action / notification I/O

The input and output children of an RPC or action, and notification children,
follow schema declaration order (RFC 7950 §7.14–7.15). `Module.RPCs()`,
`Actions()`, `Notifications()`, and a node's `Input()`/`Output()` expose them in
that order.

### `ordered-by system`: canonical, not device-replay

libyang sorts system-ordered config lists and leaf-lists during parse, so a
device's arbitrary system order is gone before Cambium ever sees the tree. Rather
than pretend otherwise, Cambium exposes system-ordered output as **deterministic
canonical order** — stable across runs, processes, and the order a device happened
to return data in. This is RFC-correct and avoids perpetual, meaningless diffs on a
read path. The trade-off is explicit: there is **no byte-exact device-order replay
API** for system-ordered nodes.

## The invariants I1–I6

The behavior above is pinned by six normative invariants. Each is testable and
backed by `/conformance` fixtures or unit tests. The full normative text is in
[`/spec/ordering-invariants.md`](../../spec/ordering-invariants.md); summarized:

| Invariant | Statement |
|---|---|
| **I1** | `ordered-by user` entry/value order is preserved exactly across parse → tree → serialize. |
| **I2** | Schema children are exposed in effective declaration order, never from a map or reflected order; data output is canonical and deterministic. |
| **I3** | List keys are exposed and serialized first, in `key`-statement order. |
| **I4** | RPC, action, and notification children appear in schema order. |
| **I5** | Lists/leaf-lists are JSON arrays carrying I1/I2 order; JSON object bytes are deterministic under a fixed printer, never from map iteration. |
| **I6** | gNMI output for `ordered-by user` is one atomic JSON_IETF value, never decomposed into order-losing scalar updates. |

### Which tier guarantees which

The invariants are tier-scoped, because Cambium has more than one engine:

| Invariant | Schema-IR (pure) | Pure-Go data tree (experimental) | libyang backend (cgo) |
|---|---|---|---|
| I2, I3, I4 | ✅ at the schema level | ✅ over supported data | ✅ over data |
| I1 | — (schema has no data) | ✅ for supported constructs | ✅ |
| I5 | — | ✅ for supported constructs | ✅ |
| I6 | — | — | future work |

The Schema-IR tier guarantees the *schema-level* ordering (I2/I3/I4) with no cgo.
The libyang backend adds the *data-level* guarantees (I1/I5, full I2/I3/I4 over real
trees). The experimental `datatree` tier reproduces I1/I2/I3/I5 over the constructs
it supports, cgo-free — see the [pure-Go data tree guide](../guides/data-tree-pure-go.md)
for exactly what that scope is. I6 (gNMI) is not yet implemented in any tier.

## See also

- [Overview](../overview.md) — where ordering sits in the bigger picture.
- [Architecture](architecture.md) — how the ordered IR stays pure and self-contained.
- [Tiers & the cgo boundary](tiers-and-cgo.md) — which tier you need.
- [Ordering invariants (spec)](../../spec/ordering-invariants.md) — the normative I1–I6 text and §1.1 placement rules.
- [Glossary](../glossary.md) — `ordered-by user`/`system`, canonical order, field-order manifest.
