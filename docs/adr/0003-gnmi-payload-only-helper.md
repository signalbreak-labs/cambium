# ADR 0003: The gNMI surface is a payload-only helper; predicated paths fail closed

- Status: Accepted
- Date: 2026-07-02

## Context

Ordering invariant **I6** ([spec/ordering-invariants.md](../../spec/ordering-invariants.md))
requires `ordered-by user` data to travel as **one atomic JSON_IETF value** —
decomposing an ordered list into scalar updates loses the order that is the
project's product. The fleet consumer for this surface (a gNMI-speaking
control plane) needs the payload; Cambium's declared non-goals
([overview](../../docs/overview.md#non-goals)) exclude owning transports,
sessions, or clients.

Two shape questions had to be answered before the API stabilized:

1. How much gNMI does Cambium own — payload, envelope, RPC, transport?
2. What happens when a caller passes a *predicated* path
   (`/top/rule[name='a']`)? The first implementation silently stripped
   predicates, so a path naming one entry returned the **whole list** — data
   of a different shape than the path asked for.

## Decision

- `go/gnmi` is a **payload-only helper**. `JSONIETFAtomicUpdate` returns an
  `Update{path, encoding, value}` envelope whose `value` is the JSON_IETF
  subtree extracted as a verbatim byte-slice of the serialized document —
  ordering is preserved because the bytes are never re-marshaled. Cambium
  defines **no** gNMI client, RPC, session, transport, or protobuf envelope.
- **Predicated paths are rejected** with `RuleCodeDataPath` and an
  instructive message; callers pass the list or leaf-list path itself for an
  atomic I6 update. Rejection replaces the earlier silent stripping.
- The stable envelope fields are documented in
  [spec/api.md](../../spec/api.md) "Helper JSON Surfaces"; the behavior is
  machine-gated by the `gnmi-ordered-atomic` conformance fixture and the
  manifest's `gnmi-path` key (which must be unpredicated).

## Consequences

- I6 has a concrete, gated implementation instead of a paper claim.
- Downstream consumers own transport concerns; the boundary between library
  and control plane stays crisp.
- A caller asking for one list entry gets a loud, typed error today. If
  honoring predicates is ever wanted, it can be **added** as an explicit
  widening driven by a real consumer — it cannot drift in.

## Reversal cost

- Adding transport/client layers later: additive (new packages), does not
  touch this contract.
- Honoring predicates later: additive (currently-rejected inputs start
  succeeding).
- Returning to **silent predicate stripping** is the one forbidden reversal:
  it reintroduces answer-differs-from-question semantics. That change would
  supersede this ADR and break the documented `RuleCodeDataPath` contract.

## Alternatives considered

- **Honor predicates by extracting the single entry** — rejected: an entry
  extracted alone loses its position among siblings; serving it as an
  "atomic" update contradicts I6's purpose, and no consumer asked for it.
- **Keep silent stripping** — rejected: returns the whole list for an
  entry-shaped path; silent wrongness.
- **Own a gNMI client/transport** — rejected: declared non-goal; belongs to
  downstream consumers.
