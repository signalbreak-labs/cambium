# Adding a language binding

Cambium ships Go today, but it was designed so another language binding can attach
as a **first-class peer**, not a bolt-on. This page explains how. The mechanism is
the language-neutral shared layer described in
[architecture](../concepts/architecture.md#the-language-neutral-shared-layer); this
is the contributor-facing "how to attach one" version.

## The shared, language-neutral layer

Four directories are kept outside any single binding and form the contract every
binding implements against:

- **[`/spec`](../../spec/)** — the contract: the [API shape](../../spec/api.md), the
  [ordering invariants I1–I6](../../spec/ordering-invariants.md), and the
  [`CAMBIUM_E####` rule codes](../../spec/rule-codes.md). A binding implements
  *against* this spec; it does not fork it. A PR that diverges from `/spec` fails
  review.
- **[`/conformance`](../../conformance/)** — a shared corpus of fixtures plus
  `golden/` outputs and `manifest.toml`. Every binding runs the same corpus through
  its own runner and reproduces the same golden bytes. Parity is defined by behavior
  on shared inputs, not by which language landed first. See
  [conformance](conformance.md).
- **[`/VERSIONS`](../../VERSIONS)** — the single source of truth for the pinned C
  engine: the libyang and PCRE2 SHAs and the engine-affecting `cmake_flags`. Every
  build stack must honor the same pins so each links a byte-identical engine.
- **[`/third_party`](../../third_party/)** — the vendored engine sources the data
  tier links statically.

## The four steps

1. **Implement under `/<lang>/`**, mirroring `/go/`'s split:
   a cgo/FFI-free schema-and-codegen core plus an optional engine-backed data tier.
   The pure core must not depend on the engine, exactly as the Go default surface
   does not.
2. **Implement against `/spec`** — the API shape, the I1–I6 ordering semantics, and
   the rule codes. Do not redefine them in the binding; the spec is the source of
   truth.
3. **Run the shared `/conformance` corpus** with a `/<lang>/` runner, reusing the
   same `golden/` outputs. Add the binding's jobs to `.github/workflows/ci.yml`.
4. **Honor `/VERSIONS`** — the engine SHA and `cmake_flags`.
   `scripts/diff-engine-config.sh` asserts a build honors the pin and is written to
   generalize across stacks.

## No binding is "primary"

Parity is defined by `/spec` and `/conformance`, not by which language landed first.
Go is the only shipping binding today, but the layer above is structured so that any
additional binding attaches as an equal peer.

## See also

- [Architecture](../concepts/architecture.md) — the hexagonal design and the shared layer.
- [Conformance](conformance.md) — the shared corpus every binding runs.
- [Roadmap](roadmap.md) — current status, including the absence of a non-Go binding today.
- [AGENTS.md](../../AGENTS.md) — the canonical project rules.
