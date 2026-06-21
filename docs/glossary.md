# Cambium glossary

This page defines the YANG and Cambium terms used throughout the documentation,
in one to three sentences each. Definitions are alphabetical; YANG terms cite
RFC 7950 (YANG 1.1) where the standard fixes the meaning, and Cambium-specific
terms point to the design they describe. For the normative wording of the
ordering invariants and rule codes, defer to the shared `/spec` contract.

## Augment

A YANG `augment` statement adds nodes to a schema tree defined elsewhere — in
another module or in a `grouping` — at a target node identified by a schema path
(RFC 7950 §7.17). Cambium resolves augments into the effective schema tree, so
augmented children appear in the ordered traversal at their effective declared
position rather than as a separate sidecar.

## choice / case

A YANG `choice` is a schema node whose children are mutually exclusive
alternatives, each alternative grouped under a `case` (RFC 7950 §7.9). In
Cambium, `choice` and `case` are ordinary ordered schema nodes; the
`SchemaNodeRef.DataChildren(flattenChoices bool)` cursor can splice the data
children of cases in at the `choice`'s declared position when `flattenChoices`
is true.

## Conformance corpus

The shared, language-neutral set of fixtures and golden outputs under
`/conformance` (fixtures, golden, `manifest.toml`) that every binding runs to
prove it honors the ordering invariants. Reusing the same corpus and golden files
is how parity is defined across bindings — not by which language landed first.

## Deviation

A YANG `deviation` statement records how a server's implementation diverges from
a published module — for example marking a node `not-supported` or adjusting a
constraint — at a target identified by a schema path (RFC 7950 §7.20.3). Cambium
applies deviations when building the effective schema tree it exposes.

## Field-order manifest

A per-struct table that Cambium's generated code carries to record the compiled
YANG declaration order of a node's fields (keys first). Generated XML and
JSON_IETF serializers walk this manifest rather than native Go map or struct
iteration order, which is how the typed-struct output stays order-correct and
deterministic.

## grouping / uses

A YANG `grouping` is a named, reusable set of schema nodes; a `uses` statement
expands a grouping's contents into the tree at the point of use (RFC 7950 §7.12,
§7.13). Cambium expands `uses` into the effective schema declaration order, so
the grouping's children appear inline at the `uses` position in ordered
traversal.

## instance-identifier

The YANG `instance-identifier` built-in type holds a value that uniquely
references a single node instance in a data tree, written as an XPath-like
absolute path (RFC 7950 §9.13). In Cambium's type model it surfaces as the
`ResolvedInstanceIdentifier` resolved-type variant.

## JSON_IETF

The IETF JSON encoding of YANG-modeled data defined by RFC 7951, used for the
NETCONF/RESTCONF data model (namespace-qualified member names, type-specific
value encodings). Cambium emits JSON_IETF from generated typed-struct serializers
in the Schema-IR tier, from the experimental pure-Go `datatree` tier, and from the
libyang backend (`Format` value `FormatJSONIETF`).

## leafref

The YANG `leafref` built-in type constrains a leaf's value to equal the value of
another leaf identified by an XPath `path` expression (RFC 7950 §9.9). In Cambium
it surfaces as the `ResolvedLeafRef` resolved-type variant. Verifying that the
referenced instance actually exists is RFC 7950 semantic validation: it is done by
the libyang backend (completely) and by the experimental `datatree` tier (over its
supported scope), but not by the Schema-IR tier's `Validate()`.

## libyang backend tier

The optional Cambium layer that operates on real data trees and **requires cgo**
(packages `libyangbackend` and `internal/libyang`). It provides a generic
`DataTree` for parse, validate, serialize, diff, merge, and LYB over a vendored,
statically linked libyang + PCRE2, including full RFC 7950 semantic validation
(`must`/`when`/`mandatory`/leafref); it stays outside the default cgo-free import
closure. (Sometimes called the Backend/data tier.)

## list key

A `key` statement names the leaf or leaves whose values uniquely identify each
entry of a YANG `list` (RFC 7950 §7.8.2). Keys are emitted first, in
`key`-statement order, before any non-key child (RFC 7950 §7.8.5); Cambium
exposes them through `SchemaNodeRef.ListKeys()` and `SchemaNodeRef.KeyNames()`,
satisfying invariant I3.

## LYB

LYB (libyang binary) is libyang's compact binary serialization format for data
trees. Cambium produces it from the libyang backend tier via
`DataTree.Serialize(FormatLYB, ...)`; it is one of the `Format` values alongside
XML, JSON, and JSON_IETF.

## Ordered-by system

The default ordering for YANG `list` and `leaf-list` nodes, where entry order is
not user-significant and the system MAY arrange entries as it sees fit (RFC 7950
§7.7.7). libyang sorts system-ordered config entries during parse, so Cambium
exposes them in a **deterministic canonical order** — stable across runs,
processes, and device return order — rather than replaying a device's arbitrary
order; the `OrderedBy` value is `OrderedBySystem`.

## Ordered-by user

The `ordered-by user` substatement on a YANG `list` or `leaf-list` declares that
entry order is set by the user and is semantically significant; the server MUST
maintain that insertion order (RFC 7950 §7.7.7, §7.8.6). Cambium preserves this
byte-exact order through parse, tree, and serialize (invariant I1) and models it
as a positional-only type so reordering is an explicit positional operation; the
`OrderedBy` value is `OrderedByUser`.

## Ordering invariant (I1–I6)

The six normative ordering guarantees Cambium implements, defined in
`/spec/ordering-invariants.md`. The Schema-IR tier guarantees I2 (schema children
in effective declaration order), I3 (list keys first, in `key` order), and I4
(RPC/action/notification children in schema order). The libyang backend
additionally guarantees I1 (`ordered-by user` preserved across round-trip), I5
(YANG lists/leaf-lists as JSON arrays carrying I1/I2 order), and I6 (gNMI
`ordered-by user` output as one atomic JSON_IETF subtree, currently future work).
The experimental `datatree` tier reproduces I1/I2/I3/I5 over the constructs it
supports.

## Pure-Go data tree tier

The **experimental**, cgo-free Cambium layer (package `datatree`) that parses,
serializes, and validates generic instance data without libyang. It handles
JSON_IETF and XML round-trip, structural and type validation, leafref instance
existence, `must`/`when` over a core XPath subset, and apply-defaults — but its
API and internal value representation are unstable and its scope is narrower than
the libyang backend (no `anydata`/`anyxml`, no RPC/action/notification data,
partial XPath). It is in the default cgo-free import closure.

## Rule code (CAMBIUM_E####)

A stable, append-only diagnostic identifier of the form `CAMBIUM_E####` carried
by every Cambium error and assigned by the operation that failed, so the same
failure yields the same code across bindings (see `/spec/rule-codes.md`). The
registry runs `CAMBIUM_E0000` (unknown) through `CAMBIUM_E0007` (stale data
handle); in Go it is surfaced via `Error.RuleCode()`.

## Schema declaration order

The effective order in which a container's or list entry's children are declared
in the YANG schema, after `uses`/grouping expansion and augmentation (RFC 7950
§7.8.5). This is the RFC-canonical child order that NETCONF servers expect on the
wire; goyang returns children alphabetically from its `Entry.Dir` map, whereas
Cambium preserves declaration order because its target use cases
(NETCONF-facing serialization, typed-struct codegen) depend on it (invariant I2).

## Schema IR

Cambium's ordered, in-memory intermediate representation of a parsed YANG schema:
an ordered tree of schema nodes that records effective declaration order
structurally rather than as a sort key, sidecar, or map. The pure-Go
introspection (`cambium`), codegen (`codegen`), and compatibility (`compat`)
packages walk this IR's ordered slices; any keyed map is a lookup cache, never a
traversal source.

## Schema-IR tier

The default Cambium layer that is **pure Go and builds with `CGO_ENABLED=0`**
(packages `cambium`, `codegen`, `compat`). It covers YANG parse to ordered schema
IR, introspection, schema-level static validation, and typed-struct codegen with
native XML/JSON_IETF (de)serializers, `Validate()`, with-defaults, and RFC 7952
metadata; its public and codegen import closure contains no cgo or libyang
(machine-enforced by `scripts/check-go-default-pure.sh`).

## The one rule (order as a structural property)

Cambium's defining design principle: order is a **structural property of the
tree** — never a sort key, sidecar, or map. The ordered sibling sequence is the
source of truth; any keyed index maps key to node identity (not key to position),
and serialization is one ordered walk of that structure (the libyang backend
walks the `lyd_node` next/prev chain; native codegen walks the field-order
manifest; `datatree` walks its ordered node slices), never native map or struct
iteration.

## Tier

One of Cambium's three implementation layers, split by what they need to build and
run: the **Schema-IR tier** (pure Go), the experimental **pure-Go data tree tier**
(pure Go), and the **libyang backend tier** (cgo). See
[Tiers & the cgo boundary](concepts/tiers-and-cgo.md).

## See also

- [Documentation index](README.md)
- [Overview](overview.md)
- [Ordering](concepts/ordering.md)
- [Architecture](concepts/architecture.md)
- [Tiers & the cgo boundary](concepts/tiers-and-cgo.md)
- [Ordering invariants (spec)](../spec/ordering-invariants.md)
- [Rule codes (spec)](../spec/rule-codes.md)
