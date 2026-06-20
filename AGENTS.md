# AGENTS.md

Agent + contributor guide for **Cambium** (signalbreak-labs) — a modern,
order-correct **YANG toolkit / SDK**. The **implemented stack today is Go**.
Successor to openconfig/goyang (the YANG parser/AST library — **not** ygot).
Cambium loads YANG, builds an order-correct schema tree, generates typed structs,
and serializes to generic ordered XML / JSON_IETF. RFC-7950 data parsing,
validation, and generic data-tree serialization are provided by the optional
libyang-backed backend. A downstream **YANG → Terraform-provider** generator that
emits NETCONF lives in a **separate repo that consumes Cambium** — not a Cambium
deliverable (see Non-goals).

`CLAUDE.md` is a symlink to this file; other tools read `AGENTS.md` natively.
**Edit this file — never `CLAUDE.md` directly.**

> **Language status.** Cambium began as a Rust-primary + Go project. The Rust
> stack was **removed** (2026-06-20) to focus on Go, the sole shipping target.
> The shared contract (`/spec`, `/conformance`, `/VERSIONS`, `/third_party`) is
> deliberately kept **language-neutral** so a Rust (or other) binding can return
> as a first-class peer under `/rust/` — see "Adding a language binding". The
> removed Rust code lives in git history.

## Read first
- **`/spec/`** — the language-neutral contract (API shape, ordering invariants
  I1–I6, rule codes) every binding implements against. PRs that diverge fail review.
- **`docs/cambium-kickoff.md`** — the original design brief (architecture, I1–I6,
  verified facts, roadmap). Historical: predates the Rust removal; read for intent,
  defer to `/spec` and this file for current status.

## The one rule that defines this project
Order is a **structural property of the tree** — never a sort key, sidecar, or map.
- Container / sibling / `uses`-grouping children → **effective schema declaration
  order** (what NETCONF expects; goyang returns children alphabetically, since it
  stores them in a map). Never map order. Pure-Go schema/codegen walks ordered IR
  slices only; maps are lookup caches, not traversal sources.
- `ordered-by user` → byte-exact insertion order, modeled as a positional-only
  type so reordering a system-ordered node is a **compile error**.
- List keys first; RPC/action I/O in schema order; system-ordered → canonical.
- Optional libyang-backed serialization is one ordered walk of libyang's
  `lyd_node.next/prev` chain — never a native map/struct serializer. Native Go
  codegen serializers walk Cambium's ordered field-order manifest, never native
  map/struct order. RFC-7950 data validation correctness is delegated to libyang
  unless and until a dedicated pure-Go validation engine exists.

## Tiers (what is and isn't cgo-free)
- **Schema-IR tier — pure Go, `CGO_ENABLED=0`** (`go/cambium`, `go/codegen`,
  `go/compat`): YANG parse → ordered schema IR, introspection, schema-level static
  validation, and typed-struct **codegen** with native XML/JSON_IETF (de)serializers,
  `Validate()`, with-defaults, and RFC-7952 metadata.
- **Backend/data tier — optional, requires cgo** (`go/libyangbackend`,
  `go/internal/libyang`): generic `DataTree` parse/validate/serialize/diff/merge/LYB
  over libyang. Must stay **outside** the default (cgo-free) import closure.

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
                         package cambium  — pure-Go schema (cgo-free default)
                         codegen          — pure-Go typed-struct generator (cgo-free)
                         compat           — goyang-compatible surface (cgo-free)
                         libyangbackend   — optional libyang data tier (cgo, outside default closure)
                         internal/libyang — cgo engine bindings + static build
                         cmd/cambium      — conformance runner (cgo)
/scripts/              green-bar, engine-config + release-flatten checks, conformance authoring
/docs/                 design brief, quickstart, gap analysis, SDK + ordering notes
/.planning/            gitignored agent scratchpad
```
No libyang major in any package name — absorb it internally.

## Build / test / lint
The optional backend statically links a **vendored** libyang + PCRE2 (no system
libyang). The default Go schema/codegen surface MUST build with cgo disabled.
- **Go default (cgo-free)** — `cd go && CGO_ENABLED=0 go test ./cambium ./codegen ./compat` · `cd go && CGO_ENABLED=0 go vet ./cambium ./codegen ./compat` · `scripts/check-go-default-pure.sh`
- **Go backend/full (cgo)** — `bash go/internal/libyang/build.sh` · `cd go && CGO_ENABLED=1 go test ./...` · `cd go && CGO_ENABLED=1 go vet ./...` · `cd go && golangci-lint run` · `cd go && go run ./cmd/cambium all`
- **One-shot** — `scripts/green-bar.sh` runs the full local gate.

## Engineering rules (non-negotiable)
- **TDD** — failing test first, always; no production code ahead of a red test.
  Every ordering invariant (I1–I6) has a conformance fixture; coverage is a floor.
- **Hexagonal** — the schema/codegen core imports ZERO libyang/cgo types; FFI is
  an optional backend adapter. The default Go public package and codegen import
  closure must contain no cgo or libyang packages (enforced by
  `scripts/check-go-default-pure.sh`). FFI is **coarse-grained** (whole-document
  parse/serialize per call; no per-node calls, no C→Go callbacks in hot paths).
  Read values via `lyd_get_value()`.
- `ly_ctx` is build-once-then-frozen; data trees are not concurrency-safe.

## Adding a language binding (e.g. bringing Rust back)
The shared layer is the contract; a new binding is a **peer**, not a bolt-on:
1. Implement under `/<lang>/` (e.g. `/rust/`), mirroring `/go/`'s split (a
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
