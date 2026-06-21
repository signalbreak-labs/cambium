# AGENTS.md

Agent + contributor guide for **Cambium** (signalbreak-labs) — a modern,
order-correct **YANG toolkit / SDK**. The **implemented stack today is Go**.
Successor to openconfig/goyang (the YANG parser/AST library — **not** ygot).
Cambium loads YANG, builds an order-correct schema tree, generates typed structs,
and serializes to generic ordered XML / JSON_IETF. Generic RFC-7950 data parsing,
validation, and serialization are provided two ways: an **experimental pure-Go
data tree** (`go/datatree`, cgo-free) and the mature **optional libyang-backed
backend** (`go/libyangbackend`, cgo). A downstream **YANG → Terraform-provider**
generator that emits NETCONF lives in a **separate repo that consumes Cambium** —
not a Cambium deliverable (see Non-goals).

`CLAUDE.md` is a symlink to this file; other tools read `AGENTS.md` natively.
**Edit this file — never `CLAUDE.md` directly.**

> **Language status.** Go is the sole shipping target. The shared contract
> (`/spec`, `/conformance`, `/VERSIONS`, `/third_party`) is deliberately kept
> **language-neutral** so an additional language binding can attach as a
> first-class peer under `/<lang>/` — see "Adding a language binding".

## Read first
- **`/spec/`** — the language-neutral contract (API shape, ordering invariants
  I1–I6, rule codes) every binding implements against. PRs that diverge fail review.
- **`docs/overview.md`** — the domain problem, the design rule, the three tiers,
  and the non-goals.
- **`docs/concepts/architecture.md`** — the hexagonal design, the cgo boundary, the
  machine-enforced cgo-free import closure, and the language-neutral shared layer.

## The one rule that defines this project
Order is a **structural property of the tree** — never a sort key, sidecar, or map.
- Container / sibling / `uses`-grouping children → **effective schema declaration
  order** (what NETCONF expects; goyang returns children alphabetically, since it
  stores them in a map). Never map order. Pure-Go schema/codegen walks ordered IR
  slices only; maps are lookup caches, not traversal sources.
- `ordered-by user` → byte-exact insertion order, modeled as a positional-only
  type so reordering a system-ordered node is a **compile error**.
- List keys first; RPC/action I/O in schema order; system-ordered → canonical.
- Every serializer is **one ordered walk**, never a native map/struct serializer:
  the libyang backend walks `lyd_node.next/prev`; native Go codegen serializers
  walk Cambium's ordered field-order manifest; the pure-Go `datatree` walks its
  ordered node slices. Full RFC-7950 data **validation** correctness is delegated
  to libyang; `datatree` is an emerging pure-Go validator (experimental, partial)
  and does not yet replace it.

## Tiers (what is and isn't cgo-free)
Three tiers, split by what they need to build and run:
- **Schema-IR tier — pure Go, `CGO_ENABLED=0`** (`go/cambium`, `go/codegen`,
  `go/compat`): YANG parse → ordered schema IR, introspection, schema-level static
  validation, and typed-struct **codegen** with native XML/JSON_IETF (de)serializers,
  `Validate()`, with-defaults, and RFC-7952 metadata.
- **Pure-Go data tier — pure Go, `CGO_ENABLED=0`, EXPERIMENTAL** (`go/datatree`):
  a generic data tree that parses/serializes JSON_IETF + XML and validates
  (mandatory, cardinality, uniqueness, leafref existence, `must`/`when` over a core
  XPath subset) and applies defaults, all cgo-free. In the default purity closure
  (`scripts/check-go-default-pure.sh`). **API + internal value representation are
  unstable** and its scope is narrower than the backend (no `anydata`/`anyxml`, no
  RPC/action/notification data, partial XPath). Not yet a stable public surface.
- **Backend/data tier — optional, requires cgo** (`go/libyangbackend`,
  `go/internal/libyang`): generic `DataTree` parse/validate/serialize/diff/merge/LYB
  over libyang — the complete, RFC-7950 data engine. Must stay **outside** the
  default (cgo-free) import closure.

## Non-goals (out of scope — downstream consumers, not Cambium)
Cambium is a library/SDK. It does **not**: send or model NETCONF transport (no
`<edit-config>` envelope builders, NETCONF clients, or device sinks/fakes); open
transports (gNMI/NETCONF/gRPC sessions); or generate Terraform providers
(`terraform-plugin-framework` resource/provider/model emitters). Those belong to a
separate downstream "generation system" repo that consumes Cambium's ordered trees
and typed-struct codegen. Generic ordered XML / JSON_IETF serialization and
typed-struct codegen (field-order manifest + deterministic serializer, zero
NETCONF/Terraform coupling) **are** in scope and stay.

## Layout
```
/VERSIONS              SHA + CMake-flag pins (libyang v5.x, pcre2) — single neutral source
/third_party/          vendored C engine: libyang, pcre2 (submodules, static build)
/spec/                 SHARED, language-neutral contract: api.md, ordering-invariants.md, rule-codes.md
/conformance/          SHARED corpus + golden outputs (fixtures, golden, manifest.toml)
/go/                   module github.com/signalbreak-labs/cambium/go
                         package cambium  — pure-Go schema IR (cgo-free default)
                         codegen          — pure-Go typed-struct generator (cgo-free)
                         compat           — goyang-compatible surface (cgo-free)
                         datatree         — experimental pure-Go data tree (cgo-free)
                         libyangbackend   — optional libyang data tier (cgo, outside default closure)
                         internal/libyang — cgo engine bindings + static build
                         cmd/cambium      — conformance runner (cgo)
/scripts/              green-bar, engine-config + release-flatten checks, conformance authoring
/docs/                 overview, concepts/, consumer guides/, contributing/, glossary, faq
/.planning/            gitignored agent scratchpad
```
No libyang major in any package name — absorb it internally.

## Build / test / lint
The optional backend statically links a **vendored** libyang + PCRE2 (no system
libyang). The default Go schema/codegen/datatree surface MUST build with cgo disabled.
- **Go default (cgo-free)** — `cd go && CGO_ENABLED=0 go test ./cambium ./codegen ./compat ./datatree` · `cd go && CGO_ENABLED=0 go vet ./cambium ./codegen ./compat ./datatree` · `scripts/check-go-default-pure.sh`
- **Go backend/full (cgo)** — `bash go/internal/libyang/build.sh` · `cd go && CGO_ENABLED=1 go test ./...` · `cd go && CGO_ENABLED=1 go vet ./...` · `cd go && golangci-lint run` · `cd go && go run ./cmd/cambium all`
- **One-shot** — `scripts/green-bar.sh` runs the full local gate.

## Engineering rules (non-negotiable)
- **TDD** — failing test first, always; no production code ahead of a red test.
  Every ordering invariant (I1–I6) has a conformance fixture; coverage is a floor.
- **Hexagonal** — the schema/codegen core imports ZERO libyang/cgo types; FFI is
  an optional backend adapter. The default Go public package, codegen, and the
  pure-Go datatree import closure must contain no cgo or libyang packages (enforced
  by `scripts/check-go-default-pure.sh`). FFI is **coarse-grained** (whole-document
  parse/serialize per call; no per-node calls, no C→Go callbacks in hot paths).
  Read values via `lyd_get_value()`.
- `ly_ctx` is build-once-then-frozen; data trees are not concurrency-safe.

## Adding a language binding
The shared layer is the contract; a new binding is a **peer**, not a bolt-on:
1. Implement under `/<lang>/`, mirroring `/go/`'s split (a
   cgo/FFI-free schema+codegen core + an optional engine-backed data tier).
2. Implement against `/spec` (API shape, I1–I6, rule codes) — do not fork it.
3. Run the **shared `/conformance` corpus** with a `/<lang>/` runner; reuse the
   same `golden/` outputs. Add the binding's jobs to `.github/workflows/ci.yml`.
4. Honor `/VERSIONS` (engine SHA + cmake_flags); `scripts/diff-engine-config.sh`
   asserts the build honors the pin and generalizes to multiple stacks.
No binding is "primary": parity is defined by `/spec` + `/conformance`, not by
which language landed first.

## Scratchpad / commits / safety
- Use `/.planning/` for scratch; promote durable decisions into `docs/` or `/spec/`.
- Conventional Commits; imperative subject ≤50 chars; one logical change per PR;
  CI green (lint + tests + conformance).
- Never commit secrets. `go.sum` is committed. Pin libyang/PCRE2 by SHA in
  `/VERSIONS`; any bump re-runs the full ordering + conformance suite.
