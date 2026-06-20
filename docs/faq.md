# FAQ

Short answers to the questions that come up most when teams first evaluate
Cambium. For the long-form rationale see [why-cambium.md](./why-cambium.md);
for the design see [architecture.md](./architecture.md).

## Why not just use goyang or ygot?

It depends on what you are building. [goyang](https://github.com/openconfig/goyang)
is a mature, widely used YANG parser/AST library, and ygot is a generation toolkit
built around it; both are good fits for a large class of YANG work — lookup,
analysis, validation, OpenConfig-style modeling.

Cambium targets a narrower use case: workflows where **schema declaration order**
is load-bearing. RFC 7950 §7.8.5 says a container/list entry's children appear in
schema declaration order, and NETCONF-facing serialization and typed-struct codegen
need the tree to remember that order. goyang stores a node's effective children in a
Go map (`Entry.Dir map[string]*Entry`) and returns them alphabetically — a
reasonable choice for its goals, but not the declaration order those order-sensitive
workflows depend on. Cambium makes order a structural property of the schema tree
(see [the one rule](./why-cambium.md)) and walks ordered slices for traversal,
codegen, and serialization. If you do not need declaration-order fidelity, the
existing tools may already be the better fit. If you do, that is the gap Cambium is
built for.

If you are coming from goyang specifically, the `compat` package offers a
goyang-shaped read-only surface — see
[goyang-migration.md](./guides/goyang-migration.md).

## Is the core really cgo-free?

Yes. The **Schema-IR tier (pure Go, `CGO_ENABLED=0`)** — packages `cambium`,
`codegen`, and `compat` — builds with cgo disabled and imports no libyang or cgo
packages. This is not a convention you have to trust; it is machine-enforced by
`scripts/check-go-default-pure.sh`, which fails CI if a cgo or libyang import leaks
into the default closure.

```bash
cd go && CGO_ENABLED=0 go test ./cambium ./codegen ./compat
```

That tier covers YANG parse into the ordered schema IR, introspection,
schema-level static validation, and typed-struct codegen with native order-correct
XML / JSON_IETF (de)serializers, `Validate()`, with-defaults, and RFC-7952
metadata. None of it requires a C toolchain.

## Does Cambium do NETCONF / gNMI / Terraform?

No. Cambium is a library/SDK, and those are explicit non-goals. It does not send
or model NETCONF transport (no `<edit-config>` envelope builders, NETCONF clients,
or device sinks), open transports (gNMI/NETCONF/gRPC sessions), or generate
Terraform providers. Those belong to a separate downstream "generation system"
repo that consumes Cambium's ordered trees and typed-struct codegen.

What Cambium does provide and keep in scope is generic ordered XML / JSON_IETF
serialization and the typed-struct generator (`codegen.GenerateGo`) — the ordered
substrate a downstream generator builds on, with no NETCONF or Terraform coupling
of its own. See [codegen.md](./guides/codegen.md).

## What happens to `ordered-by system` device order?

It is canonicalized. Cambium exposes `ordered-by system` lists and leaf-lists in
**deterministic canonical order** that is stable across runs, processes, and the
order a device happened to return — there is no byte-exact device-order replay API.

This is RFC-correct and deliberate. For `ordered-by system` nodes, RFC 7950 §7.7.7
gives the order no semantic meaning: the server controls it and clients may not rely
on it. libyang v3+ sorts system-ordered config lists/leaf-lists during parse, so a
device's arbitrary system order is already gone before Cambium sees the tree.
Surfacing a canonical order is what a read path wants — it avoids perpetual,
meaningless diffs caused by a device reshuffling order between reads. Caller
insertion order *is* preserved exactly for `ordered-by user` nodes (RFC 7950
§7.7.7, §7.8.6), where the order is semantically significant — see invariant I1 in
[/spec/ordering-invariants.md](../spec/ordering-invariants.md). The
`OrderedBy()` accessor on a `SchemaNodeRef` tells you which regime a node falls
under, and `DataTree.UserOrderedListAt` gives positional access to a user-ordered
list in the data tier.

## Do I need the libyang backend?

Only if you need the data tier. The **Backend/data tier (optional, requires cgo)**
— packages `libyangbackend` and `internal/libyang` — provides the generic
`DataTree` for parsing, validating, serializing, diffing, merging, and LYB
(de)serialization over real instance data, plus full RFC-7950 semantic validation:
`must`/`when` XPath constraints and leafref instance existence.

The pure Schema-IR tier's `Validate()` covers structural and type constraints
(cardinality, range, length, pattern, unique, mandatory, choice) but *not*
`must`/`when` or leafref existence — those are delegated to libyang. So you need
the backend when you are working with data trees or need those semantic checks; you
do not need it for schema introspection or codegen.

Building it is a one-time static build of vendored PCRE2 + libyang:

```bash
bash go/internal/libyang/build.sh
```

See [libyang-backend.md](./guides/libyang-backend.md) for the full workflow.

## See also

- [why-cambium.md](./why-cambium.md) — the domain problem and design response
- [architecture.md](./architecture.md) — the two tiers and the pure cgo-free closure
- [guides/goyang-migration.md](./guides/goyang-migration.md) — the `compat` surface
- [guides/libyang-backend.md](./guides/libyang-backend.md) — the data tier
- [/spec/ordering-invariants.md](../spec/ordering-invariants.md) — normative I1–I6
