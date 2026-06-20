You are a senior Go + Rust engineer continuing Cambium. This is a FOCUSED ~1-hour task: close the one open parity gap from the overnight session so the Rust↔Go "same bytes" contract holds for codegen too. Do exactly this one thing, well. Do not start gNMI or the Terraform provider.

READ FIRST:
- `docs/handoff-overnight.md` — the gap is the deferred "Go codegen execution test": Rust codegen is proven end-to-end (compiles + runs + byte-matches libyang) but the Go emitter only has source-determinism tested.
- `rust/cambium-codegen/tests/codegen_order_demo.rs` — the REFERENCE acceptance test. Mirror its shape for Go: generate source → assert determinism → compile + RUN the generated serializer → assert byte-identical to libyang.
- `go/codegen/codegen.go` + `go/codegen/codegen_test.go` — the current Go emitter.
- `AGENTS.md`, `spec/ordering-invariants.md`.

GREEN BAR (must stay green; run `bash go/internal/libyang/build.sh` first if `.build/` is missing):
  (cd go && go vet ./... && go test -race ./... && go run ./cmd/cambium all)
  cargo test --workspace
  bash scripts/diff-engine-config.sh

GOAL — prove the generated Go code end to end (TDD: write the failing test FIRST):
1. If the Go emitter only produces a serializer "skeleton", finish it so the generated type's XML serializer walks the `FieldOrder` manifest and emits real output. Keep the generated code self-contained (no cambium import needed for the order-demo case), exactly like the Rust generated serializer.
2. Add a Go execution test (extend `codegen_test.go`): generate Go source for the `order-demo` module, write it into a compilable location (a `t.TempDir()` package, or `go/codegen/internal/gen` behind a build tag), then `go build`/`go run` it so the generated serializer actually executes.
3. Assert: (a) generating twice yields byte-identical source (determinism); (b) the generated serializer's output for the order-demo input is BYTE-IDENTICAL to libyang — compare against `conformance/golden/scrambled-children/output.xml` (children emitted `z, m, a` in schema order), or against the bytes you get by parsing the same input through `cambium` and serializing. Use the same trailing-whitespace normalization the runners use.

ACCEPTANCE:
- A Go test compiles + runs generated code and asserts byte-match to libyang for order-demo; determinism asserted; this mirrors the Rust test so BOTH languages' codegen are proven byte-equal to libyang.
- The full green bar still passes.
- Move "Go codegen execution test" in `docs/handoff-overnight.md` from Deferred to Done, and append a dated entry to `.planning/overnight-log.md`.

GUARDRAILS:
- TDD, red first. Hexagonal: codegen imports zero C/unsafe; order comes only from the libyang walk or the `FieldOrder` manifest, never a native map/reflection. No libyang major in any name.
- Do NOT install system packages, commit, push, reset, rebase, or checkout files you did not create. Leave the tree for review; `.build/`, `vendor/`, and any generated dir stay git-ignored.
- If blocked for more than ~20 min, write down why in `.planning/overnight-log.md`, revert partial changes that break the green bar, and stop. Scope is this one parity gap only.
