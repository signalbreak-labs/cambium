# Cambium overview

Cambium is an order-correct YANG toolkit and SDK for Go. This page explains the
problem it solves — that schema declaration order is a load-bearing property of a
YANG model — the design rule that follows from taking that seriously, the three
implementation tiers, how Cambium relates to openconfig/goyang, who it is for, and
what it deliberately leaves out. Deeper treatments live in the `concepts/` docs,
which this page links to.

New to YANG or NETCONF? The [glossary](glossary.md) defines the domain terms used
here (JSON_IETF, `ordered-by user`, leafref, …), and the
[quickstart](guides/quickstart.md) is the shortest runnable path — `go get`, load a
module, walk the tree, generate structs.

## The domain problem: order is part of the schema

In YANG, the order in which sibling nodes are declared is not cosmetic. RFC 7950
specifies that the children of a container (§7.5.7) or list entry (§7.8.5) appear in
schema declaration order, and many NETCONF servers — NETCONF is the standard
protocol for configuring network devices — expect that order on the wire. A model that declares leaves `z`, `m`, `a` describes a tree whose
children are `z`, `m`, `a` — in that order. Two workflows depend directly on
preserving it:

- **Faithful serialization.** XML and JSON_IETF output that a NETCONF-facing
  device accepts must present children in the declared order, not an arbitrary or
  alphabetical one.
- **Typed-struct code generation.** Generated structs, their field-order
  manifests, and their serializers all walk the schema in declared order so that
  the bytes they emit match what the model — and the device — expect.

If the schema tree forgets declaration order somewhere between parse and use, both
break in ways that are hard to debug: the model still "looks right" — only the
sequence is wrong.

YANG order is not a single rule. Cambium handles four distinct facets explicitly:

1. **Child / sibling order** in a container or list entry — including children
   spliced in by `uses`/grouping expansion — follows effective schema declaration
   order, the RFC-canonical form NETCONF servers expect.
2. **`ordered-by user`** list entries and leaf-list values carry caller insertion
   order as semantically significant data (RFC 7950 §7.7.7, §7.8.6).
3. **List keys** are emitted first, in `key`-statement order (RFC 7950 §7.8.5).
4. **RPC / action / notification** input and output children follow schema order
   (RFC 7950 §7.14–7.15).

A fifth category, **`ordered-by system`**, is handled deliberately differently:
libyang sorts system-ordered config lists during parse, so a device's arbitrary
system order is gone before Cambium sees the tree. Rather than pretend otherwise,
Cambium exposes system-ordered output as **deterministic canonical order** —
stable across runs and processes — which is RFC-correct and avoids perpetual,
meaningless diffs on a read path. The trade-off is explicit: there is **no
byte-exact device-order replay** for system-ordered nodes.

## The design response: order is a structural property of the tree

Cambium's central rule is one sentence:

> Order is a **structural property of the tree** — never a sort key, sidecar, or
> map.

The ordered sibling sequence is the source of truth. Any keyed index Cambium keeps
is a *derived lookup cache* that maps a key to a node's **identity**, never to its
position, and it is never consulted for traversal, codegen, or serialization.
Concretely: traversal walks ordered slices (`(Module).TopLevel()`,
`(SchemaNodeRef).Children()`, `DataChildren(...)`), never map iteration; list keys
come first via `ListKeys()`/`KeyNames()`; `ordered-by user` nodes are modeled as
positional-only types so reordering a system-ordered node is a compile error, not
a runtime check; and serialization is one ordered walk. The full mechanism is in
[concepts/ordering.md](concepts/ordering.md).

### Ordering invariants I1–I6

The behavior is pinned by six normative invariants; the full text is
[`/spec/ordering-invariants.md`](../spec/ordering-invariants.md). At a user level:

- **I1** — `ordered-by user` entry/value order is preserved exactly across
  parse → tree → serialize.
- **I2** — schema children are exposed in effective declaration order, never from a
  map or reflected struct order; data output is canonical and deterministic.
- **I3** — list keys are exposed and serialized first, in `key`-statement order.
- **I4** — RPC, action, and notification children appear in schema order.
- **I5** — lists/leaf-lists are JSON arrays carrying I1/I2 order; JSON object bytes
  are deterministic under a fixed printer, never built from map iteration.
- **I6** — gNMI output for `ordered-by user` is one atomic JSON_IETF value, never
  decomposed into order-losing scalar updates.

Which tier guarantees which invariant is covered next.

## Three tiers

Order correctness is the goal, but RFC-complete YANG data validation
(`must`/`when` XPath, leafref instance existence) is a large, well-solved problem
Cambium does not blindly reimplement. The work splits into three tiers with clean
boundaries; the tier you pick is a real trade-off. See
[concepts/tiers-and-cgo.md](concepts/tiers-and-cgo.md) for the full boundary
treatment.

### Schema-IR tier — pure Go, `CGO_ENABLED=0`

Packages `cambium`, `codegen`, and `compat`. Parse YANG into an ordered schema IR,
introspect it, run schema-level static validation (cardinality, range, length,
pattern, unique, mandatory, choice — **not** `must`/`when` or leafref instance
existence), and generate typed structs with native order-correct XML/JSON_IETF
(de)serializers, `Validate()`, with-defaults, and RFC-7952 metadata. No C
toolchain, no cgo build. Guarantees **I2/I3/I4** at the schema level. The cgo-free
import closure is machine-enforced by `scripts/check-go-default-pure.sh`.

### Pure-Go data tree tier — experimental, `CGO_ENABLED=0`

Package `datatree`. A generic, cgo-free data tree: parse and serialize JSON_IETF
and XML, validate (mandatory, cardinality, uniqueness, leafref existence,
`must`/`when` over a core XPath subset), and apply defaults — all without libyang.
For its supported constructs it preserves **I1/I2/I3/I5**.

> **Experimental.** `datatree`'s public API and internal value representation **will
> change**, and its scope is narrower than the libyang backend (no `anydata`/`anyxml`,
> no RPC/action/notification data, partial XPath). Do not depend on its API yet — the
> [pure-Go data tree guide](guides/data-tree-pure-go.md) has the exact supported
> surface and [the roadmap](contributing/roadmap.md) tracks status.

### libyang data backend tier — optional, requires cgo

Packages `libyangbackend` and `internal/libyang`. A generic `DataTree` over a
vendored, statically linked libyang + PCRE2: parse, full RFC-7950 semantic
validation (`must`, `when`, `mandatory`, leafref), serialize, diff, merge, and LYB.
This is the mature, complete data engine. The cost is real and explicit: it
requires cgo and a one-time native build (`bash go/internal/libyang/build.sh`),
stays strictly outside the default cgo-free closure, uses a coarse-grained
whole-document FFI boundary, treats `ly_ctx` as build-once-then-frozen, and its
data trees are not concurrency-safe. Guarantees **I1/I2/I3/I4/I5** over real data;
gNMI output (I6) is not wired yet — its atomic-JSON_IETF mechanism is specified, but
no tier emits gNMI today.

**Choosing a tier.** Schema and codegen only → Schema-IR tier, no C build. Generic
data round-trip and validation without a C toolchain, and you can tolerate an
unstable API → the experimental `datatree` tier. Production-grade, RFC-complete
data validation → the libyang backend.

## Relationship to goyang

Cambium is the successor to the openconfig/goyang YANG parser/AST library (not
ygot). goyang is a mature, widely used library that made a reasonable choice for
its goals: it stores a node's effective children in a Go map
(`Entry.Dir map[string]*Entry`) and returns them alphabetically, which is fine for
lookup and analysis. It does not preserve the **schema declaration order** that
order-sensitive, NETCONF-facing serialization and faithful typed-struct codegen
depend on — and that specific use case is exactly what Cambium targets. The
difference is one of design targets, not of one tool being right and the other
wrong.

For teams already on goyang, Cambium ships a `compat` package that mirrors goyang's
`pkg/yang` surface (the `Entry` tree, `Modules` loader, `ToEntry`, the `Node` AST,
`YangType`). The one behavioral note to carry over: ordered traversal must go
through `Entry.Children()` (schema declaration order) rather than iterating the
`Entry.Dir` map. See the [goyang migration guide](guides/goyang-migration.md).

## Who Cambium is for

Cambium is for Go engineers building YANG tooling where declaration order is
load-bearing: order-correct serialization to generic XML / JSON_IETF, and
typed-struct codegen whose output a NETCONF-facing device accepts as-is. The
motivating downstream consumer is a YANG → Terraform-provider generator that emits
NETCONF; that generator lives in a *separate* repository and consumes Cambium's
ordered schema trees and typed-struct codegen — it is **not** a Cambium
deliverable. Cambium's job is to be the order-correct library underneath it.

## Non-goals

Cambium is a library and SDK, not a transport or a provider generator. It does
**not** send or model NETCONF transport (no `<edit-config>` builders, NETCONF
clients, or device sinks), open transports (no gNMI/NETCONF/gRPC sessions),
generate Terraform providers, or replay a device's arbitrary `ordered-by system`
order byte-for-byte. Those concerns belong to the separate downstream "generation
system" repo that consumes Cambium. What stays in scope is the ordered schema IR,
generic order-correct XML / JSON_IETF serialization, and typed-struct codegen with
a field-order manifest and deterministic serializer — with zero NETCONF or
Terraform coupling.

## Next

- [Quickstart](guides/quickstart.md) — load a module, walk the ordered tree,
  generate structs.
- [Ordering](concepts/ordering.md) · [Tiers & the cgo boundary](concepts/tiers-and-cgo.md) · [Architecture](concepts/architecture.md)
- [Glossary](glossary.md) · [FAQ](faq.md)
- [Documentation index](README.md)
