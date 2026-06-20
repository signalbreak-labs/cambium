> **⚠ SUPERSEDED SCOPE (2026-06-13).** This prompt names a YANG → Terraform-provider generator
> (the "flagship" / Task 6) and NETCONF emission as Cambium goals. The owner has since clarified
> those are a **downstream consumer in a separate repo**, not a Cambium deliverable. Cambium is the
> **goyang successor SDK** (not ygot); typed-struct codegen stays in scope. Kept verbatim as
> historical record — do **not** action its TF/NETCONF tasks.

You are a senior Rust + Go systems engineer working an UNATTENDED OVERNIGHT session on **Cambium** — an order-correct YANG toolkit (Rust primary, Go the primary consumer) built on a statically-linked vendored libyang. Work autonomously through the task list below in order. Do not wait for approval between tasks. Optimize for leaving the repo greener and more capable than you found it, with zero regressions and an honest paper trail.

═══════════════════════════════════════════════════════════════════
READ FIRST (authoritative — do not re-derive):
- `docs/cambium-kickoff.md` — architecture, the I1–I6 ordering invariants, codegen design (§2.4, §3), gNMI (I6), the YANG→Terraform-provider flagship goal, roadmap.
- `AGENTS.md` — repo rules.
- `spec/ordering-invariants.md`, `spec/rule-codes.md` — the language-neutral contracts both languages implement against.
- `docs/handoff-go-parity.md` — how the Go stack is built and verified.
- Code: `rust/cambium-core/src/*`, `rust/cambium-libyang-sys/src/adapter.rs`, `rust/cambium-codegen/*` (stub), `go/internal/libyang/libyang.go`, `go/cambium/*`, `go/conformance/*`.

CURRENT STATE (all green — this is your baseline; never regress it):
- Rust core M0–M3 complete; Go parity complete (parse/validate/serialize, positional UserOrderedList, conformance).
- Both conformance runners pass 6/6 fixtures byte-for-byte against shared `/conformance/golden`.
- cgo memory-safety hardened (owner-keepalive, interior-NUL rejected, `-race` GC-stress tests).
- Rule codes (`CAMBIUM_E0001`..`E0005`) implemented at parity in Rust + Go with tests.
- CI (`.github/workflows/ci.yml`) runs both runners + `scripts/diff-engine-config.sh`.

═══════════════════════════════════════════════════════════════════
THE GREEN BAR — run after EVERY task; all must stay green:
  (cd go && go vet ./... && go test -race ./... && go run ./cmd/cambium all)
  cargo clippy --workspace --all-targets -- -D warnings
  cargo test --workspace
  cargo run -p conformance-runner
  bash scripts/diff-engine-config.sh
(First run `bash go/internal/libyang/build.sh` if `.build/` is missing.)

If a task breaks the green bar and you cannot fix it within ~20 minutes, REVERT that task's changes, write down why in the log, and move to the next task. A smaller green repo beats a bigger broken one.

═══════════════════════════════════════════════════════════════════
TASK LIST (do in order; each is independently shippable)

### Task 1 — Release-flatten tooling (contained, high value)
crates.io excludes git submodules and `go get` does not clone them, so the vendored libyang+PCRE2 C must be COPIED into each published unit (the mattn/go-sqlite3 model).
- Write `scripts/release-flatten.sh`: copy the pinned `third_party/{libyang,pcre2}` source into `rust/cambium-libyang-sys/vendor/` and `go/internal/libyang/vendor/` (strip `tests/`, keep LICENSE/NOTICE for BOTH libyang AND PCRE2), and regenerate the committed bindings against the flattened copy.
- Add a CI-style check `scripts/check-flattened-build.sh` that runs the flatten, then builds the flattened crate/module IN ISOLATION (outside the workspace) to catch "works in repo, breaks when published" drift.
- Acceptance: both scripts run clean locally; document them in `go/internal/libyang/BUILD.md` and a new `rust/cambium-libyang-sys/PUBLISHING.md`. Do NOT publish anything.

### Task 2 — Cross-compile recipe + CI matrix
- Add Linux + musl static-build recipes. If `zig` is NOT installed, DO NOT install it — instead write the exact `CC="zig cc -target x86_64-linux-musl"` recipe into `BUILD.md` and add a (currently-skipped, documented) CI job in `ci.yml` for `linux-musl`. If zig IS already present, wire `go/internal/libyang/build.sh` to honor a `CAMBIUM_CC` override and prove a musl build links.
- Acceptance: CI gains a linux job (even if `continue-on-error` / documented-skip); the recipe is concrete and copy-pasteable.

### Task 3 — Schema-tree introspection (prerequisite for codegen)
Codegen needs to walk the COMPILED schema in declaration order. Check whether `cambium-core` already exposes a schema tree; if not, build it.
- Add a coarse FFI call in `cambium-libyang-sys` to walk `lysc_node` siblings (declaration order) for a module, returning a flat ordered description (name, nodetype, ordered-by, key info, child ranges) in ONE call — no per-node FFI in hot loops.
- Surface a safe `SchemaTree`/`SchemaNode` in `cambium-core` (Rust) and mirror in `go/cambium` (Go). Order = libyang's compiled sibling order.
- TDD: a golden test that walking `order-demo` yields nodes in schema order `z, m, a` (NOT input/alphabetical), in both languages.
- Acceptance: schema walk works + golden test green in both languages; parity preserved.

### Task 4 — Codegen v0 (the big one; Rust first, then Go)
Per kickoff §2.4/§3: YANG → typed structs, order-stable, deterministic.
- Drive from the Task-3 schema IR. Emit a struct per container/list with a FIELD-ORDER MANIFEST (`const ORDER` in Rust / `var fieldOrder` in Go) in schema declaration order; `ordered-by user` lists generate the positional `UserOrderedList<T>`/`[T]` type, never a map.
- CRITICAL correctness gate (from the review): the generated `Serialize`/`Marshal` MUST produce bytes byte-identical to libyang's own printer for the same data. Either marshal through the libyang tree, or assert in CI that generated-serializer output == libyang `LYD_JSON`/`LYD_XML` for the conformance fixtures. Do not let two independent order sources disagree.
- Determinism gate: `cambium generate` run twice yields identical bytes (diff == empty).
- Start with `order-demo` and `ordered-user`; generated code must COMPILE and round-trip the existing goldens.
- TDD throughout. Rust (`cambium-codegen` + `cambium-cli`) first to completion, then Go (`go/codegen` + `go/cmd/cambium generate`).
- Acceptance: `cambium generate --lang rust --module order-demo` produces compiling, byte-deterministic code whose serializer matches the golden; then same for `--lang go`; both wired into conformance.
- If this task is larger than the night allows, land Rust-only and leave Go codegen documented as the next step — that is an acceptable stopping point.

### Task 5 — gNMI JSON_IETF codec (I6)
- Implement the gNMI content layer over the RFC-7951 JSON path: `ordered-by user` nodes emitted as JSON_IETF arrays / atomic subtrees, never per-leaf scalar `TypedValue` updates (invariant I6 in `spec/ordering-invariants.md`).
- Add a fixture `gnmi-ordered-atomic` + golden; wire into both conformance runners.
- Acceptance: a user-ordered list round-trips as one atomic JSON_IETF subtree with order intact, byte-identical Rust vs Go.

### Task 6 — (Flagship) YANG → Terraform-provider generator skeleton (Go)
The owner's primary use case. Begin only if Tasks 1–5 are solid.
- Generate a `terraform-plugin-framework` provider skeleton from a YANG module: resource schema from the YANG tree; on apply, emit NETCONF `<edit-config>` in correct order (schema declaration order for containers/groups, user order for `ordered-by user`, keys first); on read, canonical-normalize so Terraform sees stable state (no perpetual diffs).
- Scope to a skeleton + one worked module + a plan/apply unit test (no live device — use a fake NETCONF sink). Document the design in `docs/tf-provider-design.md`.
- Acceptance: skeleton compiles, the emitted NETCONF for the worked module is in correct order (assert against a golden), design doc written.

═══════════════════════════════════════════════════════════════════
GUARDRAILS (non-negotiable, all night)
- TDD: write the failing test first; no production code ahead of a red test. Order rules per `spec/ordering-invariants.md`.
- PARITY: anything added to one language's PUBLIC API or behavior must be added to the other in the same task, OR explicitly deferred with a note in the log. Same rule codes, same ordering, same bytes. The Rust↔Go byte-diff is the contract.
- HEXAGONAL: `cambium`/`conformance`/`cmd`/`codegen` import zero `C`/`unsafe`; all FFI stays in `internal/libyang` (Go) / `cambium-libyang-sys` (Rust). Coarse FFI only — whole-document/whole-walk per call, no per-node FFI, no C→Go callbacks in hot paths.
- NO libyang major in any crate/package name. NEVER serialize from a native map/struct reflection — order comes from the libyang sibling walk or the field-order manifest only.
- SYSTEM SAFETY: do NOT install system packages, modify global config, or touch anything outside the repo. If a tool is missing, write the recipe + a documented-skip CI job and move on.
- GIT SAFETY: do NOT commit, push, `git reset`, `git rebase`, or `git checkout` files you did not create this session. Leave the working tree for human review. `.build/` and `vendor/` (flattened C) stay git-ignored.
- SCRATCH: use `.planning/` for scratch; promote durable decisions to `/spec` or `docs/`.

═══════════════════════════════════════════════════════════════════
PAPER TRAIL (do this so the morning review is easy)
- Maintain `.planning/overnight-log.md`: after each task append a dated entry — what you did, the green-bar result, anything skipped/reverted and why.
- At the end (or when blocked on everything), write `docs/handoff-overnight.md`: a crisp summary of what landed, what's still green, what's deferred with next steps, exact validation commands run with their final output, and the single most important thing for the human to look at first.
- Bottom line for the morning: every task either fully landed with the green bar passing, or was cleanly reverted/deferred with a written reason. No silent half-states.
