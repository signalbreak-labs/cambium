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
  metadata. The tier carries no runtime dependency on `openconfig/goyang`: `compat`
  owns its goyang-shaped AST node types natively, and the only remaining goyang seam
  is a thin vendored raw-statement lexer, kept out of the default cgo-free closure.
  The IR is exportable as versioned JSON (`cambium.schema-ir.v1`) via the pure-Go
  `cmd/cambium-ir` CLI, with rebuild failures carried in-band
  ([ADR 0002](../adr/0002-versioned-schemair-export.md)).
- **libyang backend tier** (`libyangbackend`, `internal/libyang`) — the complete
  RFC-7950 data engine over a vendored, statically linked libyang: parse, full
  semantic validation, serialize, diff, merge, and LYB. The backend-tier `gnmi`
  helper emits `ordered-by user` data as one atomic JSON_IETF payload value
  (invariant I6), payload-only by design
  ([ADR 0003](../adr/0003-gnmi-payload-only-helper.md)).
- The shared [conformance corpus](conformance.md) passes, gating the ordering
  invariants across the tiers that implement them. Goldens regenerate only from
  the `/VERSIONS`-pinned, repo-built oracle, and the corpus ships as a versioned
  artifact ([ADR 0004](../adr/0004-conformance-corpus-authority.md)).

## Experimental / active work

- **Pure-Go data tree** (`datatree`) — a cgo-free generic data tree and the current
  development frontier. What works today: JSON_IETF and XML parse/serialize for
  containers, leaves, leaf-lists, and lists; structural and type validation;
  leaf-list/list uniqueness and list-key checks; leafref instance existence;
  `must`/`when` over a growing XPath subset; opaque `anydata`/`anyxml` in
  JSON_IETF; and apply-defaults. It preserves ordering invariants I1/I2/I3/I5
  over what it supports. See the
  [pure-Go data tree guide](../guides/data-tree-pure-go.md).

  It is **experimental** for concrete reasons, each of which is on the path to
  stable:

  - **Value-representation refactor.** Leaf values are currently held as raw JSON
    tokens with XML conversion layered on top. A neutral value model is planned; it
    will change the internal representation and the public surface that exposes leaf
    values.
  - **Scope gaps.** No opaque XML `anydata`/`anyxml`, no cross-format conversion
    for opaque content, and no RPC/action/notification (operation) data. The XPath
    engine now covers the YANG functions `re-match`, `bit-is-set`, `derived-from`,
    and `derived-from-or-self`; it still **skips** `deref()` rather than
    mis-evaluating it.

  The goal is a complete, stable pure-Go data tier so that the full
  parse → validate → serialize path can run with the same portability the schema
  tier already has — `go get`, cross-compile, no C toolchain. Graduation is
  machine-gated: every conformance case marked `datatree = true` runs through
  **both** engines in the differential lane (`go run ./cmd/cambium datatree-diff`),
  which byte-compares output after compact-only normalization — element and member
  order is never normalized away. Growing that flagged subset *is* the path to
  stable.

## Not built yet

- **An additional language binding.** The contract (`/spec`, `/conformance`,
  `/VERSIONS`) is kept language-neutral so another binding can attach as a peer;
  none exists today. The enabling step has landed: the corpus is published as a
  versioned, checksummed artifact a peer can consume without cloning this repo
  ([conformance artifact guide](../guides/conformance-artifact.md)). See
  [adding a binding](adding-a-binding.md).

## How status is tracked

- This page — the living narrative of stable / experimental / unbuilt.
- The [conformance corpus](conformance.md) — the machine-checkable floor; a
  capability is not "done" without passing fixtures. For `datatree`, the
  differential lane (`datatree = true` cases) is the graduation gate.
- [Architecture decision records](../adr/) — the one-way-door decisions and their
  reversal costs.
- Git history — past point-in-time audits and release-readiness snapshots remain
  there for anyone who needs the historical record.

## See also

- [Overview](../overview.md) — the three tiers and the design rule.
- [Pure-Go data tree guide](../guides/data-tree-pure-go.md) — the experimental tier in detail.
- [Conformance](conformance.md) — the shared corpus and gating.
- [Development](development.md) — build/test/lint and the TDD rule.
