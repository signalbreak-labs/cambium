# Why Cambium

Cambium is an order-correct YANG toolkit and SDK for Go. This document explains
the problem it is built to solve — that schema declaration order is a meaningful,
load-bearing property of a YANG model — and the design choices that follow from
taking that problem seriously: treating order as a structural property of the
tree, splitting the work into a pure-Go schema tier and an optional libyang-backed
data tier, and exposing system-ordered data in canonical form. It also states,
neutrally, how Cambium relates to openconfig/goyang, who Cambium is for, and what
it deliberately leaves out of scope.

## The domain problem: order is part of the schema

In YANG, the order in which sibling nodes are declared is not cosmetic. RFC 7950
§7.8.5 specifies that the children of a container or list entry appear in schema
declaration order, and many NETCONF server implementations expect that order on
the wire. A model that declares leaves `z`, `m`, `a` describes a tree whose
children are `z`, `m`, `a` — in that order. Two workflows depend directly on
preserving it:

- **Faithful serialization.** XML and JSON_IETF output that a NETCONF-facing
  device accepts must present children in the declared order, not an arbitrary or
  alphabetical one.
- **Typed-struct code generation.** Generated structs, their field-order
  manifests, and their serializers all have to walk the schema in declared order
  so that the bytes they emit match what the model — and the device — expects.

If the schema tree forgets declaration order somewhere between parse and use, both
of these break in ways that are hard to debug, because the model still "looks
right" — only the sequence is wrong.

### Four ordering facets

YANG order is not a single rule. Cambium identifies four distinct facets, and
preserves or handles each one explicitly:

1. **Child / sibling node order** in a container or list entry — leaves, nested
   containers, and the children spliced in by `uses`/grouping expansion — follows
   effective **schema declaration order**. This is the RFC-canonical form NETCONF
   servers expect.
2. **`ordered-by user`** list entries and leaf-list values carry caller insertion
   order as semantically significant data; the server must maintain it
   (RFC 7950 §7.7.7, §7.8.6).
3. **List keys** are emitted first, in `key`-statement order (RFC 7950 §7.8.5),
   before any non-key child.
4. **RPC / action / notification input and output** children follow schema order
   (RFC 7950 §7.14–7.15).

### `ordered-by system`: canonical, not device-replay

The fourth ordering category in YANG, `ordered-by system`, is intentionally
handled differently. libyang v3+ sorts system-ordered config lists and leaf-lists
during parse, so a device's arbitrary system order is already gone before Cambium
ever sees the tree. Rather than pretend otherwise, Cambium exposes system-ordered
output as **deterministic canonical order** — stable across runs, processes, and
the order a device happened to return data in. This is RFC-correct, and it is what
a downstream read path wants: a canonical form that does not produce perpetual,
meaningless diffs. The trade-off is explicit and stated up front: there is **no
byte-exact device-order replay API** for system-ordered nodes. If you need to
reproduce a specific device's arbitrary system order verbatim, that is outside
what Cambium provides.

## The design response: order is a structural property of the tree

Cambium's central design rule is one sentence:

> Order is a **structural property of the tree** — never a sort key, sidecar, or
> map.

Everything else follows from it. The ordered sibling sequence is the source of
truth. Any keyed index Cambium keeps is a *derived lookup cache* — it maps a key
to a node's **identity**, never to its position — and it is never consulted for
traversal, code generation, or serialization. Because the index records identity
rather than position, moving a node positionally does not invalidate it.

Concretely:

- **Traversal walks ordered slices.** The schema-tier accessors return children in
  effective schema declaration order. `(Module).TopLevel()`,
  `(SchemaNodeRef).Children()`, and `(SchemaNodeRef).DataChildren(flattenChoices)`
  all read from an ordered representation, never from map iteration or reflected
  struct order.
- **List keys come first.** `(SchemaNodeRef).ListKeys()` and `KeyNames()` return a
  list's key leaves in `key`-statement order, so encoders emit them before any
  non-key child.
- **`ordered-by user` is modeled positionally.** In generated code, a user-ordered
  node is a positional-only type whose mutators are positional (insert/move).
  Reordering a system-ordered node is a compile-time impossibility, not a runtime
  check you might forget.
- **Serialization is one ordered walk.** The optional libyang backend walks the
  `lyd_node.next`/`prev` chain; native Go codegen serializers walk Cambium's
  ordered field-order manifest. Neither path iterates a native map or struct.

### Ordering invariants I1–I6

The behavior above is pinned down by six normative invariants. The full text lives
in [`/spec/ordering-invariants.md`](../spec/ordering-invariants.md); at a user
level they read as:

- **I1** — `ordered-by user` entry and value order is preserved exactly across
  parse → tree → serialize, for every backend format.
- **I2** — schema children are exposed in effective schema declaration order,
  never derived from a map, reflected struct order, or a goyang `Entry.Dir`. Data
  output is canonical (system-ordered nodes in canonical key/value order),
  deterministic across runs.
- **I3** — list keys are exposed and serialized first, in `key`-statement order.
- **I4** — RPC, action, and notification children appear in effective schema
  order.
- **I5** — YANG lists and leaf-lists are JSON arrays carrying I1/I2 order; JSON
  object member order carries no JSON-level meaning but is deterministic bytes
  under a fixed printer profile, never built from map iteration.
- **I6** — gNMI output for `ordered-by user` is one atomic JSON_IETF value or
  subtree, never decomposed into order-losing scalar updates.

The split between the two tiers (below) determines which invariants each can
guarantee: the Schema-IR tier guarantees I2/I3/I4 at the schema level; the
Backend/data tier additionally guarantees I1/I5/I6 over real data.

## The trade-offs: two tiers

Order correctness is the goal, but RFC-complete YANG data validation
(`must`/`when` XPath, leafref instance existence) is a large, well-solved problem.
Cambium does not reimplement it where a mature engine already exists. Instead it
splits into two tiers with a clean boundary, and the choice you make is a real
trade-off.

### Schema-IR tier — pure Go, `CGO_ENABLED=0`

Packages `cambium`, `codegen`, and `compat` are pure Go and build with cgo
disabled. This tier covers:

- YANG parse into an ordered schema IR, plus introspection over it.
- Schema-level static validation. `Validate()` on generated code here covers
  structural and type constraints — cardinality, range, length, pattern, unique,
  mandatory, choice — but **not** `must`/`when` XPath or leafref instance
  existence.
- Typed-struct codegen via `codegen.GenerateGo(ctx, module)`, emitting native
  order-correct XML and JSON_IETF (de)serializers, `Validate()`, with-defaults,
  and RFC-7952 metadata.

The benefit of staying pure Go is that this tier has no C toolchain dependency, no
cgo build step, and a public and codegen import closure that contains no cgo or
libyang packages at all. That closure property is machine-enforced by
`scripts/check-go-default-pure.sh`, so it cannot silently regress. If your work is
schema introspection and code generation, this is everything you need, and you
never compile a line of C.

### Backend/data tier — optional, requires cgo

Packages `libyangbackend` and `internal/libyang` provide a generic `DataTree`
over a vendored, statically linked libyang + PCRE2: parse, validate, serialize,
diff, merge, and LYB. This is where full RFC-7950 semantic validation lives —
`must`, `when`, `mandatory`, and leafref. The cost is real: this tier requires
cgo and a one-time native build (`bash go/internal/libyang/build.sh`, a two-stage
static CMake build of vendored PCRE2 + libyang). It stays strictly outside the
default cgo-free import closure, the FFI boundary is coarse-grained
(whole-document per call), an `ly_ctx` is build-once-then-frozen, and data trees
are not concurrency-safe.

The trade-off, stated plainly: you pay a C build and cgo to get RFC-complete data
validation and a full data-tree engine. If you only need ordered schema and
codegen, you do not pay it.

## Relationship to goyang

Cambium is the successor to the openconfig/goyang YANG parser/AST library (not
ygot). goyang is a mature, widely used library that made a reasonable design
choice for its goals. The relevant technical fact is mechanical: goyang stores a
node's effective children in a Go map (`Entry.Dir map[string]*Entry`) and returns
them in alphabetical order. For lookup and analysis, that is perfectly serviceable.
It does not, however, preserve the **schema declaration order** that
order-sensitive, NETCONF-facing serialization and faithful typed-struct codegen
depend on — and that specific use case is exactly what Cambium targets.

So the difference is one of design targets, not of one tool being right and the
other wrong. goyang returns children alphabetically; Cambium preserves schema
declaration order because its target use cases require it. For teams already on
goyang, Cambium ships a `compat` package that mirrors goyang's `pkg/yang` surface
(the `Entry` tree, `Modules` loader, `ToEntry`, the `Node` AST, `YangType`). The
one behavioral note to carry over: ordered traversal must go through
`Entry.Children()` (which returns schema declaration order) rather than iterating
the `Entry.Dir` map. See the [goyang migration guide](./guides/goyang-migration.md)
for details.

## Who Cambium is for

Cambium is for Go engineers building YANG tooling where declaration order is
load-bearing: order-correct serialization to generic XML / JSON_IETF, and
typed-struct codegen whose output a NETCONF-facing device accepts as-is.

The motivating downstream consumer is a YANG → Terraform-provider generator that
emits NETCONF. That generator lives in a *separate* repository and consumes
Cambium's ordered schema trees and typed-struct codegen — it is **not** a Cambium
deliverable. Cambium's job is to be the order-correct library underneath it.

## Non-goals

Cambium is a library and SDK, not a transport or a provider generator. It does
**not**:

- Send or model NETCONF transport — no `<edit-config>` envelope builders, NETCONF
  clients, or device sinks.
- Open transports — no gNMI, NETCONF, or gRPC sessions.
- Generate Terraform providers — no `terraform-plugin-framework` resource,
  provider, or model emitters.
- Replay a device's arbitrary `ordered-by system` order byte-for-byte; system
  order is exposed in deterministic canonical form.

Those transport and provider concerns belong to the separate downstream
"generation system" repo that consumes Cambium. What stays in scope is what
Cambium is built for: ordered schema IR, generic order-correct XML / JSON_IETF
serialization, and typed-struct codegen with a field-order manifest and a
deterministic serializer — with zero NETCONF or Terraform coupling.

## See also

- [Documentation index](./README.md)
- [Architecture](./architecture.md)
- [The ordering story](./ordering-story.md)
- [Schema introspection guide](./guides/schema-introspection.md)
- [Codegen guide](./guides/codegen.md)
- [goyang migration guide](./guides/goyang-migration.md)
- [libyang backend guide](./guides/libyang-backend.md)
- [FAQ](./faq.md)
- [Glossary](./glossary.md)
- [Ordering invariants (spec)](../spec/ordering-invariants.md)
- [Rule codes (spec)](../spec/rule-codes.md)
