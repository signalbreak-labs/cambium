# ADR 0001: Unify `SchemaNodeKind` as a single canonical type in the pure-Go core

- Status: Accepted
- Date: 2026-06-23

## Context

The schema-node kind taxonomy (`SchemaNodeKind`: `container`, `leaf`, `list`,
`anydata`, `anyxml`, `rpc`, ...) was defined **twice** as independent Go types
with parallel constant blocks and parallel `String()` methods:

- `go/cambium/schema.go` - the pure-Go schema-IR core (`CGO_ENABLED=0`).
- `go/libyangbackend/schema.go` - the cgo backend adapter over libyang.

The two were kept in sync only by convention. Splitting the collapsed
`anydata`/`anyxml` kind into two distinct kinds required editing **both** enums,
both `kindFor*` mappers, and (after peer review) **both** `String()` methods in
lockstep - a concrete demonstration of the drift hazard. A future divergence
(an added kind, a renumbered discriminant, a differing `String()`) would be a
silent correctness bug at the tier boundary, and the discriminant order is also
intended as a language-neutral, cross-binding contract.

Cambium's hexagonal rule (`AGENTS.md` Hexagonal) is that the domain core owns
the domain types and adapters depend inward on the core - never the reverse. The
backend adapter importing the core is the architecturally correct direction;
`go/cambium` already imports zero cgo/libyang packages and must stay that way
(`scripts/check-go-default-pure.sh`).

## Decision

The canonical `SchemaNodeKind` lives in the pure-Go core (`go/cambium`). The cgo
backend **aliases** it instead of redefining it:

```go
// go/libyangbackend/schema.go
type SchemaNodeKind = cambium.SchemaNodeKind

const (
    SchemaNodeKindModule = cambium.SchemaNodeKindModule
    // ... all kinds re-exported ...
)
```

- The type alias makes `libyangbackend.SchemaNodeKind` and
  `cambium.SchemaNodeKind` the **same type**; cross-tier values interoperate
  without conversion and there is exactly one `String()` implementation.
- Constants are re-exported so existing `libyangbackend.SchemaNodeKind*`
  references and `switch` arms keep compiling unchanged.
- The discriminant order is documented on the core type as the cross-binding
  contract: **append new kinds before `SchemaNodeKindUnknown`; never renumber.**
- A compile-time guard test
  (`libyangbackend.TestSchemaNodeKindUnifiedWithCore`) asserts the alias
  relationship; it fails to compile if the backend ever reintroduces a distinct
  local type.

## Consequences

- Single source of truth for the kind taxonomy; the two tiers can no longer
  drift in values, ordering, or `String()`.
- The backend adapter now imports the pure-Go core - the correct hexagonal
  direction. The core remains cgo-free (purity check still passes); there is no
  import cycle (the core does not import the backend).
- The backend inherits the core's `String()`; `%v`/`%s` formatting is identical
  across tiers.

## Reversal cost

Moderate and mechanical: re-inline the `type SchemaNodeKind int` definition, the
constant block, and the `String()` method in `go/libyangbackend/schema.go`, drop
the `cambium` import and the alias, and delete the guard test. No data or wire
migration is involved. Per `AGENTS.md`, this ADR would be **superseded**, not
edited, if reversed.

## Scope and follow-up

This ADR covers `SchemaNodeKind` only - the type the `anydata`/`anyxml` split
proved drift-prone. Other value enums duplicated across the same two tiers
(`Config`, `Status`, `OrderedBy`, `BaseType`) are **not** unified here to keep
the change focused and low-risk. They are candidates for the same alias pattern
as a separate follow-up; `BaseType` carries behavior and more values and needs
its own verification before aliasing.

## Alternatives considered

- **A shared `internal/` package owning the enum, imported by both tiers** -
  rejected: adds indirection for no benefit; the pure-Go core is the natural,
  already-public home for the domain taxonomy.
- **Keep both definitions, add a runtime value-equality test** - rejected: it
  catches value drift but not type-identity drift, and still leaves two
  `String()` methods to maintain. The alias makes drift unrepresentable.
