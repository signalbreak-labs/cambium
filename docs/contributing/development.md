# Development

This guide covers building, testing, and linting Cambium, and the engineering
rules that gate every change. The architecture it all serves is in
[concepts/architecture.md](../concepts/architecture.md); the project rules in their
canonical, terse form are in [AGENTS.md](../../AGENTS.md). The driving constraint is
that the default surface stays pure Go and the cgo data backend stays optional and
isolated.

## Prerequisites

- **Go** (see `go/go.mod` for the version). Enough on its own for the pure tiers.
- **A C toolchain and CMake** — only if you build the optional libyang backend.

## The two workflows

### Default (cgo-free)

The Schema-IR tier (`cambium`, `codegen`, `compat`) and the experimental pure-Go
data tier (`datatree`) must build and test with cgo disabled:

```bash
cd go && CGO_ENABLED=0 go test ./cambium ./codegen ./compat ./datatree
cd go && CGO_ENABLED=0 go vet  ./cambium ./codegen ./compat ./datatree
scripts/check-go-default-pure.sh
```

`scripts/check-go-default-pure.sh` is the load-bearing one: it runs those packages —
plus the cgo-free fitness tests under `./conformance` and `./internal/...` — with
`CGO_ENABLED=0`, then inspects their actual transitive dependency closure and fails
if anything pulls in `runtime/cgo`, `libyang`, `internal/libyang`, `libyangbackend`,
`github.com/openconfig/goyang`, or the vendored `internal/yangparse/upstream` lexer. The cgo-free guarantee is
verified against the resolved dependency graph, not asserted — so an accidental
backend import fails the gate rather than silently dragging in C. See
[tiers & the cgo boundary](../concepts/tiers-and-cgo.md).

### Full (cgo + libyang backend)

Build the vendored engine once, then run the whole suite with cgo:

```bash
bash go/internal/libyang/build.sh        # two-stage static CMake: PCRE2, then libyang
cd go && CGO_ENABLED=1 go test ./...
cd go && CGO_ENABLED=1 go vet  ./...
cd go && golangci-lint run
cd go && go run ./cmd/cambium all        # the conformance corpus
```

### One-shot

```bash
scripts/green-bar.sh
```

`green-bar.sh` runs the full local release bar: the cgo-free purity check first,
then the cgo test suite (`go test -race`), lint, the conformance runner, and a final
engine-config check (`scripts/diff-engine-config.sh`) that the build flags match the
pinned `/VERSIONS` cmake_flags. Run it before declaring a change done.

## Test-driven development

TDD is a house rule, not a preference: **write the failing test first; no
production code lands ahead of a red test.** Ordering behavior in particular is
pinned by the conformance corpus — every ordering invariant (I1–I6) has at least
one fixture, and that coverage is a floor, not a ceiling. When you change observable
ordering, change [`/spec/ordering-invariants.md`](../../spec/ordering-invariants.md)
first, then the fixtures, then the code. See [conformance](conformance.md).

## Doc-tests

Runnable `Example` functions in `example_test.go` back the code samples in the
guides (for instance `go/cambium/example_test.go`,
`go/codegen/example_test.go`, `go/datatree/example_test.go`). They run under
`go test`, render in godoc, and — because the pure ones are in the cgo-free set —
fail the default gate if a documented API drifts. Keep guide snippets and their
`Example` in sync; that is the mechanism that keeps the docs honest.

## Where things live

The package layout and what each package owns is in
[AGENTS.md](../../AGENTS.md) (the `Layout` section) and explained in
[architecture](../concepts/architecture.md). In short: `cambium`/`codegen`/`compat`
are the pure schema tier, `datatree` is the experimental pure-Go data tier,
`libyangbackend` + `internal/libyang` are the cgo backend, and `cmd/cambium` is the
conformance runner.

## Commits and PRs

- **Conventional Commits**; imperative subject ≤ 50 characters; one logical change
  per PR.
- CI must be green: lint, tests, and conformance.
- Never commit secrets. `go.sum` is committed. The libyang/PCRE2 SHAs are pinned in
  [`/VERSIONS`](../../VERSIONS); any bump re-runs the full ordering + conformance
  suite.
- Use [`/.planning/`](../../.planning) for scratch; promote durable decisions into
  `docs/` or `/spec/`.

## See also

- [Architecture](../concepts/architecture.md) — the design these rules protect.
- [Conformance](conformance.md) — the shared corpus and how ordering is gated.
- [Adding a language binding](adding-a-binding.md) — the peer model.
- [Roadmap](roadmap.md) — current work and known gaps.
