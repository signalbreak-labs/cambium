# FAQ

Short answers to the questions that come up most when teams first evaluate
Cambium. For the long-form rationale and the design response see
[overview.md](./overview.md); to start coding, see [install](./guides/install.md) and
the [quickstart](./guides/quickstart.md). Unfamiliar YANG/NETCONF terms (JSON_IETF,
leafref, `ordered-by user`, LYB) are defined in the [glossary](./glossary.md).

## Why not just use goyang or ygot?

It depends on what you are building. [goyang](https://github.com/openconfig/goyang)
is a mature, widely used YANG parser/AST library, and ygot is a generation toolkit
in the same ecosystem; both are good fits for a large class of YANG work — lookup,
analysis, validation, OpenConfig-style modeling and codegen.

Cambium targets a narrower use case: workflows where **schema declaration order**
matters. RFC 7950 says a container's (§7.5.7) or list entry's (§7.8.5)
children appear in schema declaration order, and order-correct, NETCONF-facing serialization plus
typed-struct codegen need the tree to remember that order. goyang stores a node's
effective children in a Go map (`Entry.Dir map[string]*Entry`) and returns them
alphabetically — a reasonable choice for its goals, but not the declaration order
those order-sensitive workflows depend on. Cambium makes order a structural
property of the schema tree and walks ordered slices for traversal, codegen, and
serialization; see [concepts/ordering.md](./concepts/ordering.md). The difference
is one of design targets, not of one tool being right and another wrong: if you do
not need declaration-order fidelity, the existing tools may already be the better
fit; if you do, that is the gap Cambium is built for.

Coming from goyang specifically? The `compat` package offers a goyang-shaped,
read-only surface — see [guides/goyang-migration.md](./guides/goyang-migration.md).

## Is the core really cgo-free?

Yes. The **Schema-IR tier (pure Go, `CGO_ENABLED=0`)** — packages `cambium`,
`codegen`, and `compat` — builds with cgo disabled and imports no libyang or cgo
packages. This is not a convention you have to trust; it is machine-enforced by
`scripts/check-go-default-pure.sh`, which fails CI if a cgo or libyang import leaks
into the default closure.

```bash
cd go && CGO_ENABLED=0 go test ./cambium ./codegen ./compat ./datatree
```

That tier covers YANG parse into the ordered schema IR, introspection,
schema-level static validation, and typed-struct codegen (`codegen.GenerateGo`)
with native order-correct XML / JSON_IETF (de)serializers, `Validate()`,
with-defaults, and RFC-7952 metadata. None of it requires a C toolchain. The
tier boundaries are laid out in
[concepts/tiers-and-cgo.md](./concepts/tiers-and-cgo.md).

## Pure-Go data tree vs libyang backend — which should I use?

Both give you a generic instance-data tree; they trade scope and stability against
the cgo build requirement.

- The pure-Go **`datatree`** tier is cgo-free but **experimental**. Its public API
  and internal value representation **will change** (the current raw-JSON-token
  representation is being reworked), and its scope is narrower than the backend: no
  `anydata`/`anyxml`, no RPC / action / notification data, and an XPath engine that
  implements only a core subset of `must`/`when`. Reach for it only if you need a
  cgo-free data round-trip and can tolerate API churn. See
  [guides/data-tree-pure-go.md](./guides/data-tree-pure-go.md) for the exact
  supported surface.
- The **libyang backend** is the complete, stable RFC-7950 data engine — but it
  needs cgo and a one-time native build. Use it for production-grade data work.

So: cgo-free and you can live with churn → `datatree`; complete and stable →
the libyang backend. When in doubt, prefer the backend for its stability — accepting
the one-time cgo build that buys it. The next two questions cover the backend
specifically.

## What happens to `ordered-by system` device order?

It is canonicalized. Cambium exposes `ordered-by system` lists and leaf-lists in
**deterministic canonical order** that is stable across runs, processes, and the
order a device happened to return — there is no byte-exact device-order replay API.

This is RFC-correct and deliberate. For `ordered-by system` nodes, RFC 7950 §7.7.7
gives the order no semantic meaning: the server controls it and clients may not
rely on it. libyang sorts system-ordered config lists/leaf-lists during parse, so
a device's arbitrary system order is already gone before Cambium sees the tree.
Surfacing a canonical order is what a read path wants — it avoids perpetual,
meaningless diffs caused by a device reshuffling order between reads. Caller
insertion order *is* preserved exactly for `ordered-by user` nodes (RFC 7950
§7.7.7, §7.8.6), where the order is semantically significant — see invariant I1 in
[../spec/ordering-invariants.md](../spec/ordering-invariants.md). The
`OrderedBy()` accessor on a `SchemaNodeRef` tells you which regime a node falls
under, and `DataTree.UserOrderedListAt` gives positional access to a user-ordered
list in the data tier.

## Do I need the libyang backend?

Only if you need a complete, stable data tier. The **Backend/data tier (optional,
requires cgo)** — packages `libyangbackend` and `internal/libyang` — provides the
generic `DataTree` for parsing, validating, serializing, diffing, merging, and LYB
(de)serialization over real instance data, plus full RFC-7950 semantic validation:
`must`/`when` XPath constraints and leafref instance existence.

The pure Schema-IR tier's `Validate()` covers structural and type constraints
(cardinality, range, length, pattern, unique, mandatory, choice) but *not*
`must`/`when` or leafref existence — those are delegated to libyang. So you need
the backend when you want production-grade data trees or those semantic checks; you
do not need it for schema introspection or codegen. If you want a data tree without
a C toolchain and can tolerate an unstable API, the experimental `datatree` tier
(above) is the cgo-free alternative.

Building the backend is a one-time static build of vendored PCRE2 + libyang:

```bash
bash go/internal/libyang/build.sh
```

See [guides/data-tree-libyang.md](./guides/data-tree-libyang.md) for the full
workflow.

## Does Cambium do NETCONF / gNMI / Terraform?

No. Cambium is a library/SDK, and those are explicit non-goals. It does not send
or model NETCONF transport (no `<edit-config>` envelope builders, NETCONF clients,
or device sinks), open transports (gNMI/NETCONF/gRPC sessions), or generate
Terraform providers. Those belong to a separate downstream "generation system"
repo that consumes Cambium's ordered trees and typed-struct codegen.

What Cambium does provide and keep in scope is generic ordered XML / JSON_IETF
serialization and the typed-struct generator — the ordered substrate a downstream
generator builds on, with no NETCONF or Terraform coupling of its own.

## See also

- [overview.md](./overview.md) — the domain problem, the design response, the three tiers
- [concepts/ordering.md](./concepts/ordering.md) — how order is made structural
- [concepts/tiers-and-cgo.md](./concepts/tiers-and-cgo.md) — tier boundaries and the cgo-free closure
- [guides/data-tree-pure-go.md](./guides/data-tree-pure-go.md) — the experimental pure-Go data tier
- [guides/data-tree-libyang.md](./guides/data-tree-libyang.md) — the libyang data tier
- [guides/goyang-migration.md](./guides/goyang-migration.md) — the `compat` surface
- [glossary.md](./glossary.md) — terms used across the docs
- [../spec/ordering-invariants.md](../spec/ordering-invariants.md) — normative I1–I6
- [Documentation index](./README.md)
