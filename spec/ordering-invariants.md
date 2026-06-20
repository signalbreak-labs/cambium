# Cambium - Ordering Invariants (I1-I6)

> Status: **normative draft v0.2** - Layer: `/spec` (language-neutral contract)
> - Applies to: Rust (`cambium-core`) and Go (`package cambium`) by the
> implementation tier named below.
>
> This file is the **single source of truth** for ordering behavior. A PR that
> changes observable ordering MUST change this file first.
>
> Key words **MUST**, **MUST NOT**, **SHOULD**, **MAY** are per RFC 2119/8174.

## 0. Why this exists

Cambium's reason to exist is order correctness. goyang stores effective children
in a Go map (`Entry.Dir map[string]*Entry`) and historically loses cross-kind
declaration order when callers build from convenience structures. Cambium makes
order a **structural property of the tree**: an ordered sibling sequence is the
source of truth, and any keyed index is a derived lookup that is never consulted
for traversal, code generation, or serialization.

This contract is tiered because Cambium now has more than one engine:

| Tier | Applies to | Order authority |
|---|---|---|
| **Schema IR tier** | Pure-Go schema/codegen, Rust schema introspection, optional backend schema introspection | Cambium's ordered effective-schema IR |
| **Backend/data tier** | Rust libyang stack and optional Go libyang backend | libyang data tree plus Cambium's schema-order contract |

The pure-Go default package is required to satisfy the Schema IR tier without
cgo. It does not, by itself, promise RFC-complete validation or serialized data
bytes. Those remain Backend/data-tier guarantees unless a dedicated pure-Go data
and validation engine is specified later.

## 1. Definitions

| Term | Meaning |
|---|---|
| **Source declaration order** | The lexical order of relevant YANG substatements as they appear in the parsed YANG statement stream, after `include`/`import` discovery. Implementations MUST read this from an ordered parse representation, not from maps or kind-separated typed slices. |
| **Effective schema declaration order** | The order of child schema nodes after grouping expansion, augment application, and deviation processing under Cambium's rules in section 1.1. This is the Schema IR tier authority. |
| **Sibling/child node order** | The relative order of distinct child nodes of a container, list, RPC/action input/output, notification, choice, or case. Governed by effective schema declaration order. |
| **List-entry order** | The relative order of multiple instances of the same list. Governed by `ordered-by`. |
| **Leaf-list value order** | The relative order of values in a leaf-list. Governed by `ordered-by`. |
| **`ordered-by user`** | Entry/value order is semantically significant; the server MUST maintain caller order. |
| **`ordered-by system`** | Entry/value order is implementation-determined and not semantically significant. Backend/data-tier implementations emit canonical order. |

### 1.1 Effective-schema placement rules

These rules are engine-neutral and are the oracle for pure-Go schema IR tests:

1. Direct child data-definition substatements keep source declaration order.
2. `uses` expands at the exact position of the `uses` statement. The expanded
   grouping children keep the grouping's source declaration order, after
   applying `refine` and local `augment` statements allowed by RFC 7950.
3. Augment children are inserted after the target node's directly declared and
   already-expanded children. Multiple augments for the same target are applied
   in deterministic module-load order, then augment statement source order, then
   child source order within the augment.
4. Deviations modify or remove target nodes without reordering unaffected
   siblings. A deviated replacement node occupies the original target position.
5. Backend/data-tier implementations MAY expose libyang's compiled order for
   backend introspection, but pure-Go golden fixtures MUST use the rules above as
   the oracle. Any intentional backend difference must be documented as a backend
   compatibility note, not hidden behind map iteration.

## 2. Ordering taxonomy

| What is being ordered | Rule | Tier |
|---|---|---|
| Schema child nodes in containers/lists/choices/cases | effective schema declaration order | Schema IR |
| RPC/action input/output and notifications | effective schema declaration order | Schema IR |
| List keys | `key` statement order, emitted first in data encodings | Schema IR + Backend/data |
| `ordered-by user` list entries / leaf-list values | caller insertion order | Backend/data |
| `ordered-by system` list entries / leaf-list values | canonical key/value order | Backend/data |
| JSON list/leaf-list arrays | array order follows list/leaf-list order | Backend/data |
| JSON object members | deterministic single-printer/ordered-IR order | Backend/data |
| gNMI ordered-by user values | atomic JSON_IETF subtree, not scalar updates | Backend/data |

## 3. The invariants

Each invariant is testable and backed by `/conformance` fixtures or Go/Rust unit
tests.

### I1 - `ordered-by user` order is preserved exactly

Backend/data-tier only. For any list or leaf-list declared `ordered-by user`, the
order of entries/values in serialized output MUST equal the order they were
inserted or parsed. Round-tripping parse -> tree -> serialize MUST preserve that
order for every supported backend format.

### I2 - Schema children are ordered; data output is canonical

Schema IR tier:

1. The child nodes of every schema parent MUST be exposed in effective schema
   declaration order.
2. The implementation MUST NOT derive child order from a Go map, Rust hash map,
   reflected struct order, goyang `Entry.Dir`, or goyang typed child slices such
   as `Leaf []*Leaf` / `Container []*Container`.
3. Lookup caches such as `childByName` are allowed only when traversal continues
   to come from the ordered slice.

Backend/data tier:

1. Serialized child nodes MUST follow effective schema declaration order.
2. `ordered-by system` list entries MUST be emitted in canonical key order;
   leaf-list values MUST be emitted in canonical value order.
3. Output MUST be deterministic across repeated runs and processes.

### I3 - List keys are first and in key-statement order

Schema IR tier exposes list key names in `key` statement order. Backend/data-tier
serialization MUST emit key leaves first within a list entry, in that order,
before any non-key child.

### I4 - RPC/action/notification children are in schema order

Schema IR tier MUST expose RPC/action input and output children, action nodes,
and notification children in effective schema declaration order. Backend/data-tier
serialization MUST emit request/response/notification payloads in that order.

### I5 - JSON arrays carry order; JSON object order is deterministic

Backend/data-tier only.

1. YANG lists and leaf-lists MUST be encoded as JSON arrays, and array element
   order MUST follow I1/I2.
2. JSON object member order carries no JSON-level meaning, but Cambium MUST emit
   deterministic bytes under a fixed printer/formatting profile.
3. Implementations MUST NOT build ordered JSON output from language-native map
   iteration or reflected struct field order.

### I6 - gNMI carries `ordered-by user` atomically

Backend/data-tier only. gNMI output for an `ordered-by user` list or leaf-list
MUST be carried as one JSON_IETF value or atomic subtree. It MUST NOT be
decomposed into scalar updates that cannot encode order.

## 4. Cross-language determinism

Cross-language determinism is tier-scoped:

1. **Schema IR parity:** Go pure-Go schema IR and Rust/backend schema
   introspection MUST agree on the ordered properties covered by the Schema IR
   tier for fixtures that both can parse. Assertions are property-level, not
   printer-byte-level.
2. **Backend/data byte parity:** Byte-identical XML/JSON/gNMI output is required
   only when both implementations run a comparable Backend/data tier over the
   same fixture, same pinned engine/config, and same formatting profile.
3. The pure-Go default Go package does not provide a data-tree serializer in the
   rebuild floor, so it is not part of Backend/data byte parity until such a
   pure-Go data engine is specified and implemented.

## 5. Explicitly not guaranteed

- Pure-Go default schema/codegen does not guarantee full RFC 7950 validation,
  XPath `must`/`when` evaluation, leafref instance checking, or serialized data
  bytes.
- Cambium does not promise byte-exact replay of arbitrary device order for
  `ordered-by system` data. Backend/data-tier output is canonical.
- Cosmetic whitespace/formatting is not part of any invariant except where a
  Backend/data-tier golden fixture explicitly fixes a printer profile.

## 6. Conformance fixture tiers

Every fixture declares a tier in `manifest.toml`.

```
/conformance/
  fixtures/<name>/
    module/                # YANG module(s)
    input.{json,xml}       # backend/data fixtures only
    expected-ir.json       # schema IR fixtures
    mode.toml              # parse/validate/serialize options
  golden/<name>/
    output.xml  output.json  output.gnmi.json
  manifest.toml            # name -> {invariants:[I2,I3], tier:"schema-ir|backend-data"}
```

Schema IR runner contract:

1. Load the module set.
2. Build the ordered schema IR.
3. Assert ordered properties against `expected-ir.json`.
4. Compare Go/Rust/backend properties only where both tiers expose the same
   property.

Backend/data runner contract:

1. Parse `input.*` with `mode.toml`.
2. Serialize to each listed format.
3. Assert bytes equal the golden output under the fixed formatting profile.
4. For backend differential fixtures, assert Rust/backend bytes equal Go/backend
   bytes.

## 7. Required edge-case fixtures

| Fixture | Tier | Invariant | Why |
|---|---|---|---|
| `schema-cross-kind-order` | Schema IR | I2 | interleaves leaf/container/leaf-list/uses/list/choice to prove the builder uses ordered statements, not typed slices |
| `schema-uses-site-order` | Schema IR | I2 | grouping expansion happens at the `uses` statement position |
| `schema-augment-order` | Schema IR | I2 | pure-Go augment placement follows section 1.1, independent of libyang printer placement |
| `keys-first` | Schema IR + Backend/data | I3 | multi-key list, keys declared after non-keys |
| `rpc-io-order` | Schema IR + Backend/data | I4 | RPC input/output schema order |
| `scrambled-children` | Backend/data | I2 | data input order is normalized to schema order |
| `ordered-user-list` | Backend/data | I1 | user-ordered round-trip |
| `nested-ordered-user` | Backend/data | I1 | nested user-ordered lists |
| `keyless-list` | Backend/data | I1/I2 | keyless list positional order |
| `system-list-canonical` | Backend/data | I2 | system-ordered entries canonicalize deterministically |
| `json-object-determinism` | Backend/data | I5 | deterministic object member output |
| `gnmi-ordered-atomic` | Backend/data | I6 | ordered list as atomic JSON_IETF |

## Appendix - RFC reference table

| Topic | RFC | Section (verify) |
|---|---|---|
| `ordered-by` statement | RFC 7950 | section 7.7.7 |
| list / `key` statement and encoding | RFC 7950 | section 7.8 |
| container XML encoding | RFC 7950 | section 7.5.7 |
| RPC / action | RFC 7950 | section 7.14 / section 7.15 |
| JSON encoding | RFC 7951 | section 4, section 5.4 |
| RESTCONF `insert` / `point` query params | RFC 8040 | section 4.8.5 / section 4.8.6 |
