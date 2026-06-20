# Handoff — Cambium Go parity (Block 2), for Kimi to continue

> See also the **Phase 4 handoff**: [`docs/handoff-go-parity-phase4.md`](./handoff-go-parity-phase4.md)
> for the Slice 1 + Slice 2 success floor.

## Where we are (all green)
The Go stack exists and **passes the full conformance corpus byte-for-byte**.

- `go test ./...` → all packages pass.
- `go vet ./...` → clean.
- `go run ./go/cmd/cambium all` → **6/6 fixtures pass** (scrambled-children, keys-first, ordered-user, rpc-order, system-list-canonical, ietf-interfaces), exit 0.
- The Rust runner (`./target/debug/conformance-runner`) also passes 6/6 against the **same** golden corpus → Rust and Go produce identical bytes (cross-language parity proven via the shared goldens).
- Static link verified (`otool -L`: no dynamic libyang/pcre2). Hexagonal boundary verified (`cambium`/`conformance`/`cmd` import no `C`/`unsafe`).
- Positional `UserOrderedList` mutation works (`TestUserOrderedListMove`: parse `c,a,b` → move → `b,c,a`).

## Build (do this first on a fresh checkout)
```sh
bash go/internal/libyang/build.sh        # two-stage static PCRE2 -> libyang into .build/ (gitignored)
cd go && go test ./... && go run ./cmd/cambium all
```
`build.sh` mirrors `rust/cambium-libyang-sys/build.rs` flag-for-flag (byte-identical engine). See `go/internal/libyang/BUILD.md`.

## Files created
```
go/go.mod
go/internal/libyang/{build.sh, .gitignore, BUILD.md, libyang.go, libyang_test.go}
go/cambium/{cambium.go, cambium_test.go, ordered_test.go}
go/conformance/{runner.go, runner_test.go}
go/cmd/cambium/main.go
```

## NEXT TASKS (priority order)

### 1. Fix cgo lifetime safety (DONE)
The adversarial review findings have been addressed and are now regression-tested:
- **`RawDataTree` stores its owning `*RawContext`** so the GC cannot collect the context while any tree is alive. `cambium.DataTree` also keeps its `*Context` owner.
- **`RawUserOrderedList` stores its owning `*RawDataTree`** so the tree cannot be collected while a list handle is alive. `cambium.UserOrderedList` also keeps its `*DataTree` owner.
- **`runtime.KeepAlive`** is called after every cgo path that dereferences a Go-owned C pointer.
- **Interior-NUL rejection:** `ParseData`/`ParseOp` now check `bytes.IndexByte(data, 0)` and return an error, matching Rust `CString::new` parity.
- **Leak-on-insert-error:** on `lyd_insert_*` failure the orphaned node is freed with `lyd_free_tree`.
- **Regression tests:** `go/internal/libyang/lifetime_test.go` includes `TestTreeSurvivesContextGC`, `TestUserOrderedListSurvivesTreeGC`, and `TestInteriorNulRejected`, run under `-race`.

### 2. Linux static-link robustness
`go/internal/libyang/libyang.go` now includes `// #cgo linux LDFLAGS: -ldl`. The static-archive link order is `libyang.a` before `libpcre2-8.a` before `-lm -lpthread -ldl`. `BUILD.md` documents this. A real Linux x86_64 build still needs CI verification.

### 3. Wire the live Rust↔Go byte-diff into CI
`.github/workflows/ci.yml` runs the Rust and Go conformance runners against the shared `/conformance` golden corpus and reports pass counts. `scripts/diff-engine-config.sh` diffs the CMake configure flags used by the Rust and Go engine builds so they cannot drift. Both runners are asserted against the same golden bytes, which is the transitive byte-diff gate (Rust bytes == golden == Go bytes).

### 4. Richer FFI diagnostics
`lyError` now surfaces libyang's `msg` + `error-app-tag`. Extend to emit the `CAMBIUM_xxxx` rule codes per `spec/rule-codes.md` (not yet authored).

### 5. Release-flatten + cross-compile (deferred)
- Release-flatten tooling: copy `third_party/{libyang,pcre2}` into the published Go module (mattn/go-sqlite3 model), CI-gated. `go get` does not clone submodules.
- musl/zig static cross-compile: recipe is in `BUILD.md`, untested.

## Adversarial review (COMPLETE — 4 dimensions)
Verdict: the Rust stack enforces every libyang lifetime invariant at compile time (ownership + `UserOrderedList<'a>` borrow); the Go port reproduces the call sequence but **discards those guarantees**, relying on hand-written `Close()` + unordered GC finalizers. Confirmed **2 critical** (tree-outlives-context UAF/double-free; UserOrderedList dangling parent), **2 high** (interior-NUL truncation divergence; missing `runtime.KeepAlive`), **1 medium** (leak-on-insert-error). All are captured in Task 1/2 above. None are hit by the 6/6 fixtures (no GC pressure in the tests).

Full findings (file:line + fixes, incl. parity/hexagonal/build dimensions):
```
jq '.result.reviews[].findings[]' \
  /private/tmp/claude-501/-Users-recursive-workspace-projects-github-signalbreak-labs-cambium/b2cfb05f-2fd4-4c13-bbb4-ec984c015ad8/tasks/wrw6jps43.output
```
Build/parity dimensions were largely clean (option mappings match Rust; `normalize` equivalent; module load order equivalent) — main build flag to add is Linux `-ldl` (Task 2). **Start with Task 1: it is confirmed UB, not theoretical.**

## Non-negotiable constraints (unchanged)
- No libyang major in any package name (`internal/libyang`, not `libyang5`).
- Hexagonal: `cambium`/`conformance`/`cmd` import zero `C`/`unsafe`; all FFI in `internal/libyang`.
- Coarse FFI only (whole-document per cgo call; no per-node cgo; no C→Go callbacks in hot paths).
- TDD: red test first. Order rules per `spec/ordering-invariants.md`.
- Do **not** commit/push. `.build/` is gitignored.

## libyang gotchas already discovered (save you the rediscovery)
- Print-siblings flag is `LYD_PRINT_SIBLINGS` (0x01), **not** `LYD_PRINT_WITHSIBLINGS` (that name is doc-only).
- `lyd_parent` is a **function-like macro** — cgo can't call it; wrapped as `cam_lyd_parent` in the preamble. `lyd_child` is a real function.
- `ly_ctx_new` options arg is `uint32_t` in this libyang (5.x), not `uint16_t`.
- `lyd_parse_op` is the 8-arg form `(ctx, parent, in, format, type, 0, &tree, &op)`; prefer the returned `op` node, fall back to `tree`.
