# AGENTS.md

Agent + contributor guide for **Cambium** (signalbreak-labs) — a modern,
order-correct **YANG toolkit / SDK** for **Rust (primary) + Go**. The Rust stack and
optional Go backend are libyang-backed; the default Go schema/codegen stack is
pure Go and must not require cgo. Successor to openconfig/goyang (the YANG
parser/AST library — **not** ygot). Cambium loads YANG, builds an order-correct
schema/data tree, validates where the selected backend supports validation,
generates typed structs, and serializes to generic ordered XML / JSON_IETF. A downstream
**YANG → Terraform-provider** generator that emits NETCONF lives in a **separate repo
that consumes Cambium** — it is not a Cambium deliverable (see Non-goals).

`CLAUDE.md` is a symlink to this file; other tools read `AGENTS.md` natively.
**Edit this file — never `CLAUDE.md` directly.**

## Read first
- **`docs/cambium-kickoff.md`** — the design brief: architecture, the I1–I6
  ordering invariants, folder layout, verified facts, roadmap. Authoritative.
- **`/spec/`** — the language-neutral contract (API, ordering invariants, rule
  codes) both languages implement against. PRs that diverge from it fail review.

## The one rule that defines this project
Order is a **structural property of the tree** — never a sort key, sidecar, or map.
- Container / sibling / `uses`-grouping children → **effective schema declaration
  order** (what NETCONF expects; what goyang breaks by alphabetizing). Never map
  order. Pure-Go schema/codegen walks ordered IR slices only; maps are lookup
  caches, not traversal sources.
- `ordered-by user` → byte-exact insertion order, modeled as a positional-only
  type so reordering a system-ordered node is a **compile error**.
- List keys first; RPC/action I/O in schema order; system-ordered → canonical.
- Optional libyang-backed serialization is one ordered walk of libyang's
  `lyd_node.next/prev` chain — never a native map/struct serializer. Future pure-Go
  serialization must walk Cambium's ordered IR/data tree and must not use native
  map/struct order. RFC-7950 validation correctness is delegated to libyang unless
  and until a dedicated pure-Go validation engine exists.

## Non-goals (out of scope — downstream consumers, not Cambium)
Cambium is a library/SDK. It does **not**: send or model NETCONF transport (no
`<edit-config>` envelope builders, NETCONF clients, or device sinks/fakes); open
transports (gNMI/NETCONF/gRPC sessions); or generate Terraform providers
(`terraform-plugin-framework` resource/provider/model emitters). Those belong to a
separate downstream "generation system" repo that consumes Cambium's ordered trees and
typed-struct codegen. Generic ordered XML / JSON_IETF serialization and typed-struct
codegen (field-order manifest + deterministic serializer, zero NETCONF/Terraform
coupling) **are** in scope and stay.

## Layout
```
/VERSIONS              SHA + CMake-flag pins (libyang v5.x, pcre2) — single source
/third_party/          vendored optional C engine: libyang, pcre2 (submodules, static build)
/spec/                 SHARED contract: api.md, ordering-invariants.md, rule-codes.md
/conformance/          SHARED corpus + golden outputs (one set, two runners)
/Cargo.toml            Rust workspace root
/rust/                 cambium · cambium-core · cambium-libyang-sys · cambium-codegen · cambium-cli
/go/                   module github.com/signalbreak-labs/cambium/go (package cambium pure-Go default;
                       optional libyang backend kept outside the default import closure)
/.planning/            gitignored agent scratchpad
```
No libyang major in any crate/package name — absorb it internally.

## Build / test / lint
Default Rust/backend builds statically link a **vendored** libyang + PCRE2 (no
system libyang). Default Go schema/codegen builds must work with cgo disabled.
- **Rust** — `cargo build --workspace` · `cargo test --workspace` · `cargo clippy --workspace --all-targets -- -D warnings` · `cargo fmt --all`
- **Go default** — `cd go && CGO_ENABLED=0 go test ./cambium ./codegen ./compat` · `cd go && CGO_ENABLED=0 go vet ./cambium ./codegen ./compat`
- **Go backend/full** — `cd go && CGO_ENABLED=1 go test ./...` · `cd go && CGO_ENABLED=1 go vet ./...` · `cd go && golangci-lint run`

## Engineering rules (non-negotiable)
- **TDD** — failing test first, always; no production code ahead of a red test.
  Every ordering invariant (I1–I6) has a golden fixture; coverage is a floor.
- **Hexagonal** — the domain core imports ZERO libyang/cgo types; FFI is an
  optional backend adapter. The default Go public package and codegen import
  closure must contain no cgo or libyang packages. FFI is **coarse-grained**
  (whole-document parse/serialize per call; no per-node calls, no C→Go callbacks
  in hot paths). Read values via `lyd_get_value()`.
- `ly_ctx` is build-once-then-frozen; data trees are not concurrency-safe.

## Scratchpad / commits / safety
- Use `/.planning/` for scratch; promote durable decisions into `docs/` or `/spec/`.
- Conventional Commits; imperative subject ≤50 chars; one logical change per PR;
  CI green (fmt + clippy/lint + tests, both langs + conformance).
- Never commit secrets. `Cargo.lock` + `go.sum` are committed. Pin libyang/PCRE2 by
  SHA in `/VERSIONS`; any bump re-runs the full ordering + conformance suite.
