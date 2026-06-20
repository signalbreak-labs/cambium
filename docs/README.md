# Cambium documentation

Cambium is an order-correct YANG toolkit and SDK for Go. It loads YANG, builds a
schema tree that remembers **effective schema declaration order**, generates typed
Go structs with order-correct serializers, and (through an optional libyang-backed
backend) parses, validates, and serializes generic data trees. Cambium exists for
a specific use case: order-sensitive, NETCONF-facing workflows where RFC 7950
declaration order is structurally meaningful and must survive parse, codegen, and
serialization. This page is the index to the rest of the documentation.

Cambium ships in two tiers. The **Schema-IR tier (pure Go, `CGO_ENABLED=0`)**
covers schema loading, introspection, static validation, and typed-struct codegen.
The **Backend/data tier (optional, requires cgo)** adds generic data-tree parsing,
full RFC-7950 semantic validation, diff/merge, and LYB over a vendored, statically
linked libyang. Start with "Why Cambium" for the rationale, or jump straight to the
quickstart.

## Start here

- [why-cambium.md](./why-cambium.md) — the domain problem (YANG/NETCONF order
  semantics), Cambium's design response, the trade-offs, and who it is for.
- [quickstart.md](./quickstart.md) — the fast path: load a module, walk the ordered
  tree, generate typed structs.

## Concepts

- [ordering-story.md](./ordering-story.md) — why order is modeled as a structural
  property of the tree rather than a sort key, sidecar, or map.
- [architecture.md](./architecture.md) — the hexagonal design, the two tiers, the
  machine-enforced cgo-free closure, and the language-neutral shared layer.

## Guides

- [guides/schema-introspection.md](./guides/schema-introspection.md) — package
  `cambium`: build a `Context`, load modules, and walk the ordered schema tree.
- [guides/codegen.md](./guides/codegen.md) — package `codegen`: `GenerateGo`, what
  it emits, and the ordering guarantees baked into generated structs and serializers.
- [guides/goyang-migration.md](./guides/goyang-migration.md) — package `compat`: the
  goyang-shaped read-only projection and the neutral compatibility notes.
- [guides/libyang-backend.md](./guides/libyang-backend.md) — package
  `libyangbackend`: build the engine, then parse, validate, serialize, diff, merge,
  and emit LYB over real data.

## Reference

- [conformance.md](./conformance.md) — the shared `/conformance` corpus and golden
  outputs, and how the ordering invariants are gated.
- [faq.md](./faq.md) — concise answers on use-case fit, the cgo-free core, non-goals,
  and `ordered-by system` device order.
- [glossary.md](./glossary.md) — YANG and Cambium terms (schema declaration order,
  `ordered-by user`/`system`, list key, leafref, IR, field-order manifest, LYB, tier).
- [../spec/api.md](../spec/api.md) — the language-neutral API shape every binding
  implements against.
- [../spec/ordering-invariants.md](../spec/ordering-invariants.md) — the normative
  text for invariants I1–I6.
- [../spec/rule-codes.md](../spec/rule-codes.md) — the `CAMBIUM_E####` rule-code
  catalog.

## Background (historical)

These documents capture earlier design intent and point-in-time audits. They are
useful for context but defer to the docs above and to `/spec` for current status.

- [cambium-kickoff.md](./cambium-kickoff.md) — the original design brief
  (architecture, invariants, roadmap). Predates the Rust removal; read for intent.
- [sdk-api-design.md](./sdk-api-design.md) — the early SDK API design exploration.
- [gaps-analysis-2026-06-20.md](./gaps-analysis-2026-06-20.md) — a gap analysis
  snapshot.
- [release-readiness-2026-06-20.md](./release-readiness-2026-06-20.md) — a
  release-readiness snapshot.
- [go-quality-followups.md](./go-quality-followups.md) — recorded Go quality
  follow-ups.

## See also

- [../README.md](../README.md) — repository overview and capability summary.
- [../spec/api.md](../spec/api.md) — the shared, language-neutral contract.
- [why-cambium.md](./why-cambium.md) — start with the rationale.
