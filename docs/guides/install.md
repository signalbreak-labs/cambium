# Install

Cambium's default surface is pure Go and installs with nothing but `go get`. The
optional libyang data backend is the only part that needs a C toolchain. This guide
covers both, plus how to verify the default surface is genuinely cgo-free.

## The default (cgo-free) packages

```bash
go get github.com/signalbreak-labs/cambium/go
```

That is all you need for the Schema-IR tier (`cambium`, `codegen`, `compat`) and
the experimental pure-Go data tier (`datatree`). No C compiler, no system libyang,
no build step. These packages build with `CGO_ENABLED=0` and cross-compile to any
target the Go toolchain supports.

```go
import (
	"github.com/signalbreak-labs/cambium/go/cambium"
	"github.com/signalbreak-labs/cambium/go/codegen"
)
```

## Verifying the default surface is cgo-free

The cgo-free guarantee is enforced, not assumed. From a checkout of the repository:

```bash
scripts/check-go-default-pure.sh
```

It exercises `cambium`, `codegen`, `compat`, and `datatree` with `CGO_ENABLED=0`,
then inspects their actual transitive dependency closure and fails if it contains
`runtime/cgo`, anything matching `libyang`, or any package carrying cgo source.
See [tiers & the cgo boundary](../concepts/tiers-and-cgo.md) for what this buys you.

## The optional libyang backend (cgo)

The [libyang backend](data-tree-libyang.md) (`libyangbackend`) is the full RFC-7950
data engine. It statically links a vendored libyang + PCRE2 — there is no
dependency on a system libyang — so it requires a one-time native build.

**Prerequisites:** a C toolchain (a C compiler and `make`) and CMake.

**Build the engine once:**

```bash
bash go/internal/libyang/build.sh
```

This is a two-stage static CMake build: PCRE2 first (static, position-independent),
then libyang against the staged PCRE2, both into a gitignored `.build/` directory.
The vendored sources and their pinned SHAs live in `/third_party` and `/VERSIONS`.

**Then build or test with cgo enabled:**

```bash
cd go
CGO_ENABLED=1 go test ./...
```

Because this tier uses cgo, cross-compiling it requires a cross C toolchain; the
pure tiers above do not.

## Running the full local gate

`scripts/green-bar.sh` runs the complete local release bar: the cgo-free purity
check first, then the cgo test suite, lint, and the conformance runner. See
[development](../contributing/development.md) and
[conformance](../contributing/conformance.md).

## Next

- [Quickstart](quickstart.md) — load a module and generate structs.
- [Tiers & the cgo boundary](../concepts/tiers-and-cgo.md) — which tier you need.
