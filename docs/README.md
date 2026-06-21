# Cambium documentation

Cambium is an order-correct YANG toolkit and SDK for Go. It loads YANG, builds a
schema tree that remembers **effective schema declaration order**, generates typed
Go structs with order-correct serializers, and parses/validates/serializes generic
data — either with the experimental pure-Go `datatree` tier or the optional
libyang-backed backend. It exists for one use case: order-sensitive,
NETCONF-facing workflows where RFC 7950 declaration order is structurally
meaningful and must survive parse, codegen, and serialization.

New here? Read the [overview](overview.md), then the [quickstart](guides/quickstart.md).

## Documentation map

This suite has three authorities, with no overlap between them:

- **godoc** — the API reference (signatures, per-symbol docs). Browse it on
  [pkg.go.dev](https://pkg.go.dev/github.com/signalbreak-labs/cambium/go). The
  pages here link into it; they do not restate signatures.
- **[`/spec`](../spec/)** — the normative, language-neutral contract: behavior,
  ordering invariants, rule codes.
- **`docs/`** (this tree) — narrative: concepts, task-oriented guides, and
  contributor material.

## Orientation

- [overview.md](overview.md) — the domain problem, the design rule, the three
  tiers, who it is for, and the non-goals.
- [glossary.md](glossary.md) — YANG and Cambium terms (declaration order,
  `ordered-by user`/`system`, list key, leafref, IR, field-order manifest, LYB,
  tier).
- [faq.md](faq.md) — use-case fit, the cgo-free core, pure-Go vs libyang data,
  `ordered-by system` device order.

## Concepts

- [concepts/ordering.md](concepts/ordering.md) — why order is modeled as a
  structural property of the tree, the four ordering facets, and a worked example.
- [concepts/tiers-and-cgo.md](concepts/tiers-and-cgo.md) — the three tiers, the cgo
  boundary, and how to choose.
- [concepts/architecture.md](concepts/architecture.md) — the hexagonal design, the
  machine-enforced cgo-free import closure, and the language-neutral shared layer.

## Using Cambium (consumer guides)

- [guides/install.md](guides/install.md) — `go get`, building the optional cgo
  engine, and verifying the default surface is cgo-free.
- [guides/quickstart.md](guides/quickstart.md) — load a module, walk the ordered
  tree, generate typed structs.
- [guides/schema-introspection.md](guides/schema-introspection.md) — package
  `cambium`: build a `Context`, load modules, walk the ordered schema tree.
- [guides/codegen.md](guides/codegen.md) — package `codegen`: `GenerateGo`, what it
  emits, and the ordering guarantees baked into generated structs and serializers.
- [guides/data-tree-pure-go.md](guides/data-tree-pure-go.md) — package `datatree`
  (**experimental**): parse, validate, and serialize generic data without cgo.
- [guides/data-tree-libyang.md](guides/data-tree-libyang.md) — package
  `libyangbackend`: build the engine, then parse, validate, serialize, diff, merge,
  and emit LYB over real data.
- [guides/goyang-migration.md](guides/goyang-migration.md) — package `compat`: the
  goyang-shaped read-only projection and the migration notes.

## Contributing & contract

- [contributing/development.md](contributing/development.md) — build/test/lint, the
  green-bar gate, the cgo-free purity check, and the TDD rule.
- [contributing/conformance.md](contributing/conformance.md) — the shared
  `/conformance` corpus, the manifest, the tiers, and how ordering is gated.
- [contributing/roadmap.md](contributing/roadmap.md) — current work, what is
  experimental, and known gaps.
- [contributing/adding-a-binding.md](contributing/adding-a-binding.md) — how a new
  language binding attaches as a peer against `/spec` and `/conformance`.
- [contributing/vendor-yang.md](contributing/vendor-yang.md) — policy for distilling
  owned conformance fixtures from vendor YANG without vendoring proprietary models.
- [../spec/api.md](../spec/api.md) · [../spec/ordering-invariants.md](../spec/ordering-invariants.md) · [../spec/rule-codes.md](../spec/rule-codes.md) — the normative contract.

## See also

- [Repository README](../README.md) — overview and capability summary.
- [AGENTS.md](../AGENTS.md) — the contributor + agent guide and project rules.
