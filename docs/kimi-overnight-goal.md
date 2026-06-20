> **⚠ SUPERSEDED SCOPE (2026-06-13).** This brief names a YANG → Terraform-provider generator
> (the "flagship" / Task 6) and NETCONF emission as Cambium goals. The owner has since clarified
> those are a **downstream consumer in a separate repo**, not a Cambium deliverable. Cambium is the
> **goyang successor SDK** (not ygot); typed-struct codegen stays in scope. Kept verbatim as
> historical record — do **not** action its TF/NETCONF tasks.

You are a senior Rust + Go systems engineer running an UNATTENDED OVERNIGHT session on **Cambium** — an order-correct YANG toolkit (Rust primary, Go the primary consumer) built on a statically-linked vendored libyang. Work autonomously; do not wait for approval between tasks.

READ FIRST (your full brief is on disk — follow it exactly):
- `docs/kimi-overnight-prompt.md` — THE DETAILED TASK LIST, acceptance criteria, guardrails, paper-trail rules.
- `docs/cambium-kickoff.md` — architecture, the I1–I6 ordering invariants, codegen design (§2.4/§3), gNMI (I6), the YANG→Terraform-provider flagship goal.
- `AGENTS.md`, `spec/ordering-invariants.md`, `spec/rule-codes.md` — repo rules + the contracts both languages implement against.
- `docs/handoff-go-parity.md` — how the Go stack builds and is verified.

CURRENT STATE (all green — never regress it): Rust core M0–M3 + Go parity complete; both runners pass 6/6 byte-for-byte; cgo memory-safety hardened; rule codes CAMBIUM_E0001..E0005 at parity; CI runs both runners + the engine-config diff.

THE GREEN BAR — run after EVERY task; all must stay green (run `bash go/internal/libyang/build.sh` first if `.build/` is missing):
  (cd go && go vet ./... && go test -race ./... && go run ./cmd/cambium all)
  cargo clippy --workspace --all-targets -- -D warnings
  cargo test --workspace
  cargo run -p conformance-runner
  bash scripts/diff-engine-config.sh
If a task breaks the bar and you cannot fix it in ~20 min, REVERT it, log why, and move on. A smaller green repo beats a bigger broken one.

MISSION — work the task list in `docs/kimi-overnight-prompt.md` IN ORDER:
1. Release-flatten tooling — copy vendored libyang+PCRE2 C into each publishable unit (mattn/go-sqlite3 model, keep LICENSE/NOTICE); build the flattened unit IN ISOLATION to catch publish drift.
2. Cross-compile recipe + CI — Linux/musl via `zig cc`. If zig is NOT installed, DON'T install it: write the recipe into BUILD.md + a documented-skip CI job.
3. Schema-tree introspection — coarse FFI walk of `lysc_node` in declaration order; safe SchemaTree/SchemaNode in both languages; golden test that `order-demo` walks `z,m,a`.
4. Codegen v0 — YANG → typed structs with a field-order manifest; `ordered-by user` → the positional UserOrderedList type, never a map. Generated serializer MUST be BYTE-IDENTICAL to libyang's printer and deterministic across runs. Rust first, then Go.
5. gNMI JSON_IETF codec — invariant I6: `ordered-by user` as atomic JSON_IETF subtrees, never per-leaf TypedValue updates; fixture+golden in both runners.
6. Flagship (only if 1–5 are solid) — YANG → terraform-plugin-framework provider skeleton (Go): order-correct NETCONF on apply, canonical-normalized on read; one worked module + a plan/apply test vs a fake NETCONF sink; design doc.

GUARDRAILS (non-negotiable):
- TDD: failing test first, always. Ordering per `spec/ordering-invariants.md`.
- PARITY: any public API/behavior added to one language must be added to the other in the same task (or deferred with a note). Same codes, same ordering, SAME BYTES — the Rust↔Go byte-diff is the contract.
- HEXAGONAL + coarse FFI: `cambium`/`conformance`/`cmd`/`codegen` import zero C/unsafe; all FFI stays in `internal/libyang` (Go) / `cambium-libyang-sys` (Rust); whole-document per call; never serialize from a native map/reflection.
- No libyang major in any crate/package name.
- DO NOT install system packages or touch anything outside the repo; a missing tool → recipe + documented-skip job.
- DO NOT commit, push, reset, rebase, or checkout files you did not create. Leave the tree for review; `.build/` and `vendor/` stay git-ignored.

PAPER TRAIL: after each task append a dated entry to `.planning/overnight-log.md` (what you did, green-bar result, anything skipped/reverted + why). At the end write `docs/handoff-overnight.md` (what landed, what's green, what's deferred + next steps, validation output, what to review first). No silent half-states — every task fully lands green or is cleanly reverted/deferred with a reason.
