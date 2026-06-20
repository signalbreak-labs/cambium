You are a senior Rust + Go systems engineer continuing **Cambium** — an order-correct YANG toolkit whose Go stack is a cgo FFI over a statically-linked vendored libyang. Block 2 (Go parity) is scaffolded and green; your job is to make it memory-safe and production-ready WITHOUT breaking the passing corpus.

READ FIRST (authoritative — do not re-derive):
- `docs/handoff-go-parity.md` — current state, exact build/run commands, the prioritized task list, the confirmed adversarial-review findings, and libyang gotchas already discovered. THIS IS YOUR SPEC.
- `AGENTS.md` — repo rules (hexagonal, coarse FFI, TDD, no libyang major in names, conventional commits).
- `spec/ordering-invariants.md` — the language-neutral ordering contract.
- Code you are hardening: `go/internal/libyang/libyang.go` (cgo), `go/cambium/cambium.go`, `go/conformance/runner.go`. Compare to the Rust originals `rust/cambium-libyang-sys/src/adapter.rs` and `rust/cambium-core/src/{tree,list,context}.rs`.

BUILD/RUN (fresh checkout): `bash go/internal/libyang/build.sh`, then `cd go && go test ./... && go run ./cmd/cambium all` (expect 6/6 fixtures pass byte-for-byte). The static engine builds into `.build/` (gitignored).

CURRENT STATE: compiles, `go vet` clean, passes 6/6 conformance fixtures; both the Go and Rust runners agree against the shared `/conformance/golden`. BUT an adversarial review CONFIRMED real cgo lifetime UB that the tests never trip (they hold refs via `defer Close()`, no GC pressure). Fix it.

GOAL — make the Go FFI memory-safe, test-driven, keeping 6/6 green:

1. **Fix the confirmed lifetime bugs (CRITICAL — do first).** Rust enforces these at compile time (`Context` ownership; `UserOrderedList<'a>` borrows `&'a mut DataTree`); the Go port dropped them. Re-create the ties at runtime:
   - `RawDataTree` MUST keep its owning `*RawContext` alive — a data tree must never outlive its libyang context, or `lyd_free_all`/`lyd_validate_all` hit a destroyed context (UAF/double-free). Change `newRawDataTree` to take `*RawContext`, store it, and `runtime.KeepAlive(owner)` at the end of every method that touches C (Serialize, Validate, finalize, UserOrderedListAt). Mirror in `cambium.DataTree`.
   - `RawUserOrderedList` MUST hold the owning `*RawDataTree` (not just a raw `parent` pointer), with `runtime.KeepAlive(owner)` in every op. Mirror in `cambium.UserOrderedList`.
   - Reject interior-NUL input in `ParseData`/`ParseOp` (`bytes.IndexByte(data, 0) >= 0` → error) to match Rust's `CString::new`, instead of silently truncating (a real parity divergence).
   - Add `runtime.KeepAlive` after the `lyd_insert_*` cgo calls; on insert error, free or restore the orphaned node (no leak).
   - **TDD:** write the failing test FIRST — create many trees, drop the `*Context` reference, `runtime.GC()`, then use the trees/lists; it should crash/UAF before the fix and pass after. Run the whole suite with `-race`.

2. **Linux static-link robustness.** Add `// #cgo linux LDFLAGS: -ldl` to `libyang.go`; verify a real Linux x86_64 static build links (archive order: libyang.a before pcre2-8.a). Update `BUILD.md`.

3. **Live cross-language byte-diff in CI.** Add a job that builds + runs BOTH runners and asserts identical output (spec/ordering-invariants.md §4), plus an engine-build-config diff so the two libyang builds can't drift.

4. (Stretch) `CAMBIUM_xxxx` rule codes in `lyError`; release-flatten tooling (vendored C into the published module, mattn/go-sqlite3 style); musl/zig cross-compile.

HARD CONSTRAINTS:
- Keep `go test ./...`, `go vet ./...`, and `go run ./cmd/cambium all` (6/6) green after every change.
- Hexagonal: `cambium`/`conformance`/`cmd` import zero `C`/`unsafe`; all FFI stays in `internal/libyang`. Coarse FFI only (whole-document per cgo call).
- No libyang major in any package name. TDD: red test first. Do NOT commit or push.

ACCEPTANCE: lifetime bugs fixed and proven by a `-race` GC-stress test; interior-NUL rejected (with a test); 6/6 fixtures still byte-identical to golden AND to the Rust runner; `go vet` clean. The full confirmed findings are summarized in `docs/handoff-go-parity.md`.
