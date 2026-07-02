# ADR 0002: SchemaIR is exported as a versioned JSON projection (`cambium.schema-ir.v1`)

- Status: Accepted
- Date: 2026-07-02

## Context

The ordered schema IR is consumed outside this repository: the documented
downstream audience ([downstream-schema-consumers](../guides/downstream-schema-consumers.md))
includes schema renderers and a generator that lives in a separate repo, and the
conformance corpus already pinned an IR JSON shape as `expected-ir.json` goldens.
Two gaps forced a decision:

- There was no export surface at all — the only CLI was the cgo-gated
  conformance runner, so a non-Go consumer had no way to obtain the IR.
- `SchemaIR()` silently discarded schema-rebuild failures
  (GAP F-006 / §6-Q1), so a consumer could receive a stale or partial
  projection with no signal.

An unversioned ad-hoc JSON dump would make every field an implicit,
unbreakable contract; a wire-schema format (protobuf) would introduce a second
source of truth beside the existing goldens.

## Decision

The IR export is a **versioned value projection**:

- `go/cambium/schema_ir.go` defines the projection and its version string,
  `cambium.schema-ir.v1`, embedded in every emitted document as `version`.
- `go/cmd/cambium-ir` (pure Go, inside the cgo-free closure verified by
  `scripts/check-go-default-pure.sh`) emits the projection as JSON. It maps
  domain types through an explicit export DTO — JSON tags are never smeared
  onto domain types.
- Failures are **in-band**: `SchemaIR.Errors` (JSON `errors`) carries
  structured rebuild diagnostics instead of silently dropping them; the same
  failure also surfaces as a `LoadReport` warning.
- Ordering in the projection comes from Cambium's ordered IR slices, never
  from map iteration — the export inherits invariant I2 by construction.

**Compatibility policy for `v1`** (normative copy in
[spec/api.md](../../spec/api.md) "Helper JSON Surfaces"): future v1 output may
add fields and add enum/string values where existing meaning is unchanged.
Removing fields, renaming fields, or changing path/order semantics requires a
**new version string**. Only the fields named in the spec are stable.

## Consequences

- External consumers get a machine-checkable, versioned surface and can pin
  `version == "cambium.schema-ir.v1"` defensively.
- The conformance corpus's `expected-ir.json` goldens gate the projection's
  bytes; the spec names the stable fields; the guide carries the consumer
  narrative. Reference shape and prose cannot silently diverge from code
  without a golden diff.
- Additive evolution is cheap; breaking evolution is forced to be explicit
  and observable (a new version string), never silent.

## Reversal cost

High once consumed. Retiring or reshaping v1 means a deprecation cycle for
every downstream consumer plus a new version string; the version field is what
keeps that path orderly. Per `AGENTS.md`, reversal supersedes this ADR rather
than editing it.

## Alternatives considered

- **Go API only, no export** — rejected: the declared audience includes
  non-Go consumers and build pipelines that must not link Go.
- **Unversioned JSON dump** — rejected: every field becomes an implicit
  forever-contract; evolution breaks consumers silently.
- **Protobuf/IDL contract** — rejected for now: heavier toolchain for
  consumers and a second source of truth beside the committed
  `expected-ir.json` goldens. Can be revisited as a *new* versioned surface
  without breaking v1.
