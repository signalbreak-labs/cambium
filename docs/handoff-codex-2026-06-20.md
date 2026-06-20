# Codex Handoff - 2026-06-20

## Current checkpoint

This file records the current state on `feat/pure-go-rebuild-spec` after the
latest Go parity and safety-net work. It supersedes the stale deferred sections
in older historical handoffs for status purposes only:

- `docs/handoff-go-data-layer.md`
- `docs/handoff-go-parity-phase4.md`
- `docs/handoff-go-parity.md`

Do not rewrite those files as history; use this file for the current finish-line
audit.

## Recent committed slices

- `c053186 feat(libyangbackend): add LYB format`
- `7ef9597 test(conformance): handle LYB bytes`
- `2fe74e4 chore(go): satisfy golangci lint`
- `5b05399 test(cambium): guard import transitivity`
- `a4e8d79 test(cambium): mirror empty type edge fixture`
- `1220e00 fix(libyang): use value accessor`
- `3d08fb8 fix(libyang): use metadata value accessor`
- `97c9953 ci: gate default go purity`
- `42663ca docs: update purity gate checkpoint`
- `e3d084c docs: mark old handoff historical`
- `a675295 docs: document go purity gate`
- `0476960 docs: label historical design status`
- `085147d test(compat): cover range safety edges`
- `d9599ed test(compat): cover number int edges`

## Proven current state

- The default Go packages remain cgo-free. This was rechecked with:

  ```sh
  cd go
  CGO_ENABLED=0 go list -deps ./cambium ./codegen ./compat \
    | perl -ne 'our $hit; if (/(^runtime\/cgo$|libyang|cambium-libyang|\/backend|\/cgo)/) { print; $hit=1 } END { exit($hit ? 1 : 0) }'
  ```

- Go default tests, Go backend tests, vet, lint, and the cgo-free dependency
  closure were green at the latest checkpoint:
  - `scripts/check-go-default-pure.sh`
  - `CGO_ENABLED=0 go test ./cambium ./codegen ./compat -count=1 -timeout=30m`
  - `CGO_ENABLED=1 go test ./... -count=1 -timeout=30m`
  - `CGO_ENABLED=0 go vet ./cambium ./codegen ./compat`
  - `CGO_ENABLED=1 go vet ./...`
  - `golangci-lint run` (`0 issues`)
  - `CGO_ENABLED=0 go list -deps ./cambium ./codegen ./compat` with the
    `runtime/cgo|libyang|backend|cgo` guard

- Rust was green at the same checkpoint:
  - `cargo fmt --all -- --check`
  - `cargo test --workspace` (`conformance: 200 case(s) passed`)
  - `cargo clippy --workspace --all-targets -- -D warnings`

- Go conformance runner was green:
  - `CGO_ENABLED=1 go run ./cmd/cambium all`
  - Output: `conformance: 193 passed, 0 failed`

- The libyang-backed value reads now use the public libyang accessors:
  - Go data-node values call `lyd_get_value()`.
  - Go metadata values call `lyd_get_meta_value()`.
  - Rust data-node values call `lyd_get_value()`.
  - Rust metadata values call `lyd_get_meta_value()`.
  - Source guards in both languages reject direct `lyd_value._canonical` reads
    in these paths.

- The libyang-backed Go data layer now has the work older handoffs listed as
  deferred: mutation CRUD, stale-handle generation bumps, diff/apply, merge,
  duplicate, and LYB round-trip coverage.

- The pure-Go schema side now has working `Identity.Bases()` and
  `SchemaNodeRef.Ancestors()` implementations with tests. The same identity-base
  gap called out in older Phase 4 handoffs is not current.

- The live `unsupported` strings in `go/compat` mirror vendored goyang behavior
  for known upstream cases. Codegen `unsupported` strings are fail-fast emitter
  errors; tests assert generated source does not contain runtime unsupported
  parser fallbacks for the covered fixture set.

- The pure-Go boundary guard is now CI-facing via
  `scripts/check-go-default-pure.sh`, which vets/tests `./cambium`, `./codegen`,
  and `./compat` with `CGO_ENABLED=0`, then rejects forbidden cgo/libyang/backend
  dependencies and any cgo files in the default dependency closure.

## Finish-line status

There are no known blocking implementation gaps for the cgo-free Go default
surface after the latest proof pass. The current evidence includes:

- API-shape parity guards compare exported `go/compat` declarations, struct
  fields, values, function signatures, methods, and runtime method sets against
  the vendored goyang surface.
- Behavior parity coverage exercises the goyang-shaped parser/module facade,
  AST helpers, `ToEntry`, defaults, typedefs, identities, augment/deviation,
  choice normalization, path lookup, namespace resolution, RPC/action I/O, and
  source-order preservation.
- Cambium-specific safety-net coverage now also guards stricter range validation
  and `Number.Int` overflow rejection where goyang's historical helpers accept
  invalid constructed values.

Remaining risk before a release tag is release engineering and review:

- Cross-platform static build/release flattening should still be checked on the
  actual release targets.
- Old historical handoffs are intentionally preserved as history. New work should
  use this checkpoint and the normative `/spec` files, not stale deferred notes.
- Any future public API additions need the same parity/safety proof pattern before
  they are called complete.

## Next useful commands

```sh
git status --short --branch --ignored=no

cd go
../scripts/check-go-default-pure.sh
CGO_ENABLED=0 GOTOOLCHAIN=local go test ./... -count=1 -timeout=30m
CGO_ENABLED=1 GOTOOLCHAIN=local go test ./... -count=1 -timeout=30m
CGO_ENABLED=0 GOTOOLCHAIN=local go vet ./cambium ./codegen ./compat
CGO_ENABLED=1 GOTOOLCHAIN=local go vet ./...
golangci-lint run
CGO_ENABLED=1 GOTOOLCHAIN=local go run ./cmd/cambium all

cd ..
cargo fmt --all -- --check
cargo test --workspace
cargo clippy --workspace --all-targets -- -D warnings
```
