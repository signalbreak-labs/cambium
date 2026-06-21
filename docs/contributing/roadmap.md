# Roadmap & status

This is a **living** status document — the single place that says what is stable,
what is experimental, and what is not built yet. It deliberately replaces
date-stamped snapshot files: point-in-time audits and release-readiness notes rot,
and they live in git history when you need them. Keep this page current as work
lands rather than adding another dated file.

## Stable today

- **Schema-IR tier** (`cambium`, `codegen`, `compat`) — pure Go, `CGO_ENABLED=0`.
  YANG parse into an ordered schema IR, introspection, schema-level static
  validation, the goyang-shaped `compat` projection, and typed-struct codegen with
  native XML/JSON_IETF serializers, `Validate()`, with-defaults, and RFC-7952
  metadata.
- **libyang backend tier** (`libyangbackend`, `internal/libyang`) — the complete
  RFC-7950 data engine over a vendored, statically linked libyang: parse, full
  semantic validation, serialize, diff, merge, and LYB.
- The shared [conformance corpus](conformance.md) passes, gating the ordering
  invariants across the tiers that implement them.

## Experimental / active work

- **Pure-Go data tree** (`datatree`) — a cgo-free generic data tree and the current
  development frontier. What works today: JSON_IETF and XML parse/serialize for
  containers, leaves, leaf-lists, and lists; structural and type validation;
  leaf-list/list uniqueness and list-key checks; leafref instance existence;
  `must`/`when` over a core XPath subset; and apply-defaults. It preserves ordering
  invariants I1/I2/I3/I5 over what it supports. See the
  [pure-Go data tree guide](../guides/data-tree-pure-go.md).

  It is **experimental** for concrete reasons, each of which is on the path to
  stable:

  - **Value-representation refactor.** Leaf values are currently held as raw JSON
    tokens with XML conversion layered on top. A neutral value model is planned; it
    will change the internal representation and the public surface that exposes leaf
    values.
  - **Scope gaps.** No `anydata`/`anyxml`, no RPC/action/notification (operation)
    data, and the XPath engine implements a core subset — it skips `derived-from`,
    `re-match`, `bit-is-set`, and `deref` rather than mis-evaluating them.

  The goal is a complete, stable pure-Go data tier so that the full
  parse → validate → serialize path can run with the same portability the schema
  tier already has — `go get`, cross-compile, no C toolchain.

## Not built yet

- **gNMI (invariant I6).** No tier emits gNMI today; the `gnmi-ordered-atomic`
  conformance fixture is deferred (see
  [`/spec/ordering-invariants.md`](../../spec/ordering-invariants.md) §7). I6's
  mechanism (carrying `ordered-by user` as one atomic JSON_IETF value) is specified
  but not wired to a gNMI output path.
- **A returning non-Go binding.** The contract (`/spec`, `/conformance`,
  `/VERSIONS`) is kept language-neutral so a Rust (or other) binding can attach as a
  peer; none exists today. See [adding a binding](adding-a-binding.md).

## How status is tracked

- This page — the living narrative of stable / experimental / unbuilt.
- The [conformance corpus](conformance.md) — the machine-checkable floor; a
  capability is not "done" without passing fixtures.
- Git history — past point-in-time audits and release-readiness snapshots remain
  there for anyone who needs the historical record.

## See also

- [Overview](../overview.md) — the three tiers and the design rule.
- [Pure-Go data tree guide](../guides/data-tree-pure-go.md) — the experimental tier in detail.
- [Conformance](conformance.md) — the shared corpus and gating.
- [Development](development.md) — build/test/lint and the TDD rule.
