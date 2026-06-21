# Architecture

Cambium is organized as a hexagonal (ports-and-adapters) toolkit with a pure-Go
domain core at the center and libyang attached as an optional outbound adapter at
the edge. This document explains that layering, the three tiers and the cgo
boundary between them, the machine-enforced cgo-free import closure that keeps the
default surface portable, the lifecycle and concurrency contract of the libyang
backend, and the language-neutral shared layer that lets another language binding
return as a peer. The driving constraint throughout is the project's one rule:
order is a structural property of the schema tree (RFC 7950 §7.5.7), so the design
keeps the ordered IR pure and self-contained and treats every IO/FFI dependency as
something the core never imports.

## The hexagonal shape

The core domain is everything that can be computed from YANG without touching C,
IO, or a network: parsing YANG into an order-correct schema IR, introspecting that
tree, schema-level static validation, typed-struct codegen with native
serializers, and — newly, and still experimental — a pure-Go generic data tree.
That core is pure Go and depends on nothing outward.

libyang is not part of the domain — it is an *outbound adapter*. The domain never
imports it; the backend tier imports the domain's types and wraps libyang behind a
small, coarse-grained surface (`DataTree` parse/validate/serialize/diff/merge).
Dependencies point inward only: the data adapter knows about the schema tier, never
the reverse.

```text
                       inbound (callers)
        codegen.GenerateGo · cmd/cambium · your program
                              │
        ┌─────────────────────▼──────────────────────────┐
        │            DOMAIN CORE  (pure Go)               │
        │                                                 │
        │   package cambium    ordered schema IR          │
        │   package codegen    typed-struct generator     │
        │   package compat     goyang-shaped projection   │
        │   package datatree   pure-Go data tree (exp.)   │
        │                                                 │
        │   imports: stdlib only.  No cgo. No libyang.    │
        └─────────────────────┬───────────────────────────┘
                              │  (dependencies point inward only)
        ┌─────────────────────▼──────────────────────────┐
        │   OUTBOUND ADAPTER  (optional, cgo)             │
        │                                                 │
        │   package libyangbackend   generic DataTree     │
        │   package internal/libyang cgo engine bindings  │
        │                                                 │
        │   ───────────── cgo boundary ───────────────    │
        │   vendored libyang v5.x + PCRE2 (static link)   │
        └──────────────────────────────────────────────────┘
```

The payoff of this shape is that the import that matters most — the default,
schema-and-codegen surface — has no transitive path to C. It builds with
`CGO_ENABLED=0`, cross-compiles cleanly, and ships without a system libyang. The
libyang backend exists for the workflows that genuinely need a full RFC-7950 data
engine today (`must`/`when` XPath, leafref instance existence over the complete
function set), and those callers opt into cgo explicitly.

## The three tiers

### Schema-IR tier — pure Go, `CGO_ENABLED=0`

Packages `cambium`, `codegen`, and `compat`. This tier turns YANG text into an
ordered schema IR and computes everything derivable from it.

- **`cambium`** — the ordered schema IR and the entry point. Construct a context
  with `cambium.NewContextBuilder(ContextFlags{})`, load modules
  (`LoadModule` / `LoadModuleFromPath` / `LoadModuleStr`), and `Build()` a frozen
  `*Context`. From there `(*Context).Schema(module)` returns a `Module` whose
  `TopLevel()`, `Children()`, `RPCs()`, `Actions()`, and `Notifications()` expose
  nodes in effective schema declaration order (invariant I2).
  `(SchemaNodeRef).ListKeys()` and `KeyNames()` return list keys first in
  `key`-statement order (I3); RPC/action `Input()`/`Output()` children stay in
  schema order (I4). The ordered sibling sequence is the source of truth; any keyed
  lookup is a derived cache that maps key to node identity, never to position.
- **`codegen`** — the typed-struct generator. Its single exported entry point is
  `codegen.GenerateGo(ctx *cambium.Context, module string) (string, error)`. It
  emits typed structs, a per-struct field-order manifest, native XML and JSON_IETF
  (de)serializers, value `Validate`, with-defaults, and RFC-7952 metadata.
  Generated serializers walk the field-order manifest, never native map or struct
  iteration, so output follows compiled YANG declaration order with keys first.
- **`compat`** — a read-only, goyang-shaped projection (`Entry`, `Modules`,
  `ToEntry`, the `Node` AST) for callers migrating from `openconfig/goyang`. It
  reproduces the goyang-shaped Entry/AST with its own native node types and does
  **not** import `openconfig/goyang` at runtime — which is also why goyang is a
  forbidden import in the closure check below. The one behavioral difference:
  ordered traversal uses `Entry.Children()` (schema declaration order) rather than
  iterating the `Entry.Dir` map (alphabetical). `Dir` remains available as a lookup
  cache.

Static validation in this tier (`Validate()` on generated structs) covers
structural and type constraints — cardinality, range, length, pattern, unique,
mandatory, choice. It does **not** evaluate `must`/`when` XPath or check leafref
instance existence. This tier needs no C build; `go get` is enough.

### Pure-Go data tree tier — experimental, `CGO_ENABLED=0`

Package `datatree`. A generic data tree that parses a document against a Cambium
schema into an ordered tree and serializes it back in effective schema declaration
order, with **no libyang and no cgo**. `datatree.Parse(module, format, data)`
accepts JSON_IETF or XML; `(*Tree).Serialize`, `Validate`, and `ApplyDefaults`
round-trip and check it. For the constructs it supports it preserves I1/I2/I3/I5,
and it validates mandatory, cardinality, uniqueness, leafref instance existence,
and `must`/`when` over a core XPath subset.

This tier is **experimental and not a stable public surface.** Two facts shape how
to treat it:

- **Its value representation is being reworked.** Leaf values are currently stored
  as raw JSON tokens, with XML conversion layered on top; a neutral
  value-representation refactor is planned, and the public API will change with it.
- **Its scope is narrower than the libyang backend.** No `anydata`/`anyxml`, no
  RPC/action/notification data, and the XPath engine implements a core subset — it
  *skips* `derived-from`, `re-match`, `bit-is-set`, and `deref` rather than
  mis-evaluating them.

Why does a second, pure-Go data tree exist alongside the libyang backend? Because
the long-term goal is a complete data tier that honors the same cgo-free property
the schema tier already enjoys — `go get`, cross-compile, no C toolchain. `datatree`
is that goal under construction. Until it matures, the libyang backend remains the
reference data engine, and `datatree` is in the codebase (and the cgo-free purity
gate) so it can be built and tested in the open. See the
[pure-Go data tree guide](../guides/data-tree-pure-go.md) and the
[roadmap](../contributing/roadmap.md).

### libyang backend tier — optional, requires cgo

Packages `libyangbackend` and `internal/libyang`. A generic `DataTree` for real
RFC-7950 data: parse, full semantic validation, serialize, diff, merge, and LYB,
backed by a vendored, statically linked libyang + PCRE2.

- **`libyangbackend`** — the public data API, guarded by `//go:build cgo`. Build a
  context with `libyangbackend.NewContext()`, load schema, then
  `Parse(format, mode, data)` into a `*DataTree`. `(*DataTree).Validate` runs full
  RFC-7950 semantics including `must`/`when`/`mandatory`/leafref; `Serialize` emits
  XML / JSON / JSON_IETF / LYB in one ordered walk of libyang's `lyd_node.next/prev`
  sibling chain — never a native map or struct serializer. `ordered-by user` entry
  order is preserved byte-exactly across parse → tree → serialize (invariant I1).
- **`internal/libyang`** — the cgo engine bindings and the static build glue. This
  is the only package that contains C interop and references the vendored engine.
  Build the engine once with `bash go/internal/libyang/build.sh`, a two-stage static
  CMake build of vendored PCRE2 then libyang.

The libyang tier stays strictly outside the default import closure. Nothing in
`cambium`, `codegen`, `compat`, or `datatree` reaches it.

### The cgo boundary

The boundary between the pure tiers and the libyang tier is exactly the cgo
boundary. Above it is portable Go that builds anywhere; below it is C interop that
requires a toolchain and the vendored engine. `//go:build cgo` tags keep the
backend out of cgo-free builds, and the dependency-closure check (below) makes that
separation a property the build proves rather than a convention contributors must
remember. See [Tiers & the cgo boundary](tiers-and-cgo.md) for the consumer-facing
"which tier do I use" treatment.

## The machine-enforced cgo-free import closure

The split above is only meaningful if it cannot quietly erode. A single accidental
import of `libyangbackend` from the `cambium` or `datatree` package would drag C
into the default closure, and that kind of regression is easy to miss in review.
`scripts/check-go-default-pure.sh` enforces the boundary mechanically.

The script:

1. Runs `CGO_ENABLED=0 go vet` and `CGO_ENABLED=0 go test` over `./cambium`,
   `./codegen`, `./compat`, and `./datatree` (plus the cgo-free fitness tests under
   `./conformance` and `./internal/...`), so the pure surface is exercised with cgo
   genuinely disabled — the path the `CGO_ENABLED=1` lane would silently skip.
2. Lists the full transitive dependency closure of those packages with
   `CGO_ENABLED=0 go list -deps` and fails if it contains any forbidden package:
   `runtime/cgo`, anything matching `libyang`, `internal/libyang`, `libyangbackend`,
   `github.com/openconfig/goyang`, or the vendored `internal/yangparse/upstream`
   raw-statement lexer.
3. Fails if any package in that closure has cgo source files at all.

Because the check inspects the *actual resolved dependency graph*, the cgo-free
guarantee is verified, not asserted. `scripts/green-bar.sh` runs it as the first
gate of the local release bar, so the property is checked before anything else.

```bash
# Verify the default surface is genuinely cgo-free:
scripts/check-go-default-pure.sh
```

## libyang lifecycle and concurrency contract

Because libyang is an outbound adapter wrapped at a coarse grain, its lifecycle and
concurrency rules are part of the architecture, not an implementation detail.

- **Coarse-grained FFI.** Crossing the cgo boundary is expensive and error-prone, so
  the backend crosses it a whole document at a time: one call parses or serializes
  an entire document. There are no per-node FFI calls in hot paths and no C-to-Go
  callbacks during a walk; values are read via libyang's own accessors. This keeps
  the surface area of the C boundary small and auditable.
- **Build-once-then-frozen context.** A schema context is mutable while you load
  modules and frozen thereafter. In the pure tier this is explicit: `ContextBuilder`
  is the mutable phase and `Build()` returns a frozen `*Context`. The libyang
  `ly_ctx` follows the same discipline — assemble the schema, then treat it as
  read-only and shareable for reads.
- **Data trees are not concurrency-safe.** A `*DataTree` is mutable state with no
  internal locking. Do not share one across goroutines without external
  synchronization; give each goroutine its own (e.g. via `Duplicate()`), or
  serialize access. The frozen context is the shared, read-only part; the data tree
  is the per-operation, mutable part.

These rules follow directly from the hexagonal placement: the engine is an adapter
the core does not own, so the contract for using it safely is stated explicitly at
the boundary rather than hidden behind the domain.

## The language-neutral shared layer

Go is the sole shipping target. What remains deliberately language-neutral is the
contract layer, kept outside any single binding so an additional language binding can
attach as a first-class peer rather than a bolt-on.

- **`/spec`** — the contract: API shape (`api.md`), the ordering invariants I1–I6
  (`ordering-invariants.md`), and the `CAMBIUM_E####` rule codes (`rule-codes.md`).
  A binding implements *against* this spec; it does not fork it.
- **`/conformance`** — a shared corpus of fixtures plus `golden/` outputs and
  `manifest.toml`. Every binding runs the same corpus through its own runner and is
  expected to reproduce the same golden bytes, so parity is defined by behavior on
  shared inputs, not by which language landed first.
- **`/VERSIONS`** — the single source of truth for the pinned C engine: the libyang
  and PCRE2 SHAs and the engine-affecting `cmake_flags`. Every build stack must
  honor the same pins so each links a byte-identical engine.
  `scripts/diff-engine-config.sh` asserts the Go build's flags match `/VERSIONS`.
- **`/third_party`** — the vendored engine sources (libyang, PCRE2) the data tier
  links statically.

A new binding lives under `/<lang>/`, mirrors `/go/`'s split (a cgo/FFI-free
schema-and-codegen core plus an optional engine-backed data tier), implements
against `/spec`, and runs the shared `/conformance` corpus with its own runner. No
binding is "primary": conformance to `/spec` and `/conformance` defines parity. See
[adding a binding](../contributing/adding-a-binding.md).

## See also

- [Overview](../overview.md) — the domain problem and the design response.
- [Ordering](ordering.md) — how order is modeled as a structural property.
- [Tiers & the cgo boundary](tiers-and-cgo.md) — choosing a tier.
- [Conformance](../contributing/conformance.md) — the shared corpus and gating.
- [Ordering invariants (spec)](../../spec/ordering-invariants.md) — the normative I1–I6 text.
