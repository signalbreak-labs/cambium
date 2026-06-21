# Tiers & the cgo boundary

Cambium is layered into three tiers. Two are pure Go and build with cgo disabled;
one wraps libyang and requires cgo. This page is the decision-oriented companion to
the [architecture](architecture.md) doc: it tells you which tier your work needs,
what the cgo boundary costs in practice, and how to confirm the default surface
stays cgo-free. For the structural "why it's shaped this way" treatment, read
[architecture](architecture.md).

## The three tiers at a glance

| Tier | Packages | cgo? | What it does | Stability |
|---|---|---|---|---|
| **Schema-IR** | `cambium`, `codegen`, `compat` | no | parse YANG → ordered schema IR, introspect, schema-level static validation, typed-struct codegen | stable |
| **Pure-Go data tree** | `datatree` | no | generic data: parse/serialize JSON_IETF + XML, validate, defaults — without libyang | **experimental** |
| **libyang backend** | `libyangbackend` | yes | full RFC-7950 data engine: parse/validate/serialize/diff/merge/LYB | stable |

## Which tier do I need?

Start from what you are trying to do, not from a package name.

- **"I need ordered schema information, or to generate typed structs."** → **Schema-IR
  tier.** Load modules, walk the ordered tree, and/or call
  `codegen.GenerateGo(ctx, "module-name")` (the second argument is the implemented
  module's name). No C build, no cgo. The generated structs
  themselves carry order-correct XML/JSON_IETF serializers, so for many codegen
  workflows you never touch a data-tree tier at all. See the
  [schema introspection](../guides/schema-introspection.md) and
  [codegen](../guides/codegen.md) guides.

- **"I need to parse, validate, and serialize generic instance data, with
  production-grade RFC-7950 semantics."** → **libyang backend.** This is the
  complete data engine: full `must`/`when` XPath, leafref instance existence over
  the whole function set, diff/merge, and LYB. It costs a one-time native build and
  cgo. See the [libyang backend guide](../guides/data-tree-libyang.md).

- **"I need generic data round-trip and validation, but I want to stay cgo-free and
  I can tolerate an unstable API and a narrower feature set."** → the **experimental
  `datatree` tier.** It parses/serializes JSON_IETF and XML and validates without a
  C toolchain, but its API and value representation will change, and it does not yet
  cover `anydata`/`anyxml`, RPC/action/notification data, or the full XPath function
  set. See the [pure-Go data tree guide](../guides/data-tree-pure-go.md). If you
  need stability today, use the libyang backend instead.

- **"I'm migrating from openconfig/goyang."** → **Schema-IR tier, `compat` package.**
  It mirrors goyang's `pkg/yang` surface. See the
  [goyang migration guide](../guides/goyang-migration.md).

```text
                Do you need instance DATA, or just SCHEMA?
                          │
          ┌───────────────┴────────────────┐
        SCHEMA                            DATA
          │                                │
   Schema-IR tier              Need full RFC-7950 semantics
   (cambium/codegen/compat)    and a stable API?
   pure Go, no cgo                 │
                          ┌────────┴─────────┐
                        YES                  NO — and want cgo-free,
                         │                   tolerate experimental
                  libyang backend                  │
                  (cgo, complete)            datatree tier
                                             (pure Go, experimental)
```

## What the cgo boundary costs

The boundary between the two pure tiers and the libyang tier is exactly the cgo
boundary, and it is a real trade-off, not a formality.

**Staying in the pure tiers (`CGO_ENABLED=0`) buys you:**

- `go get` and nothing else — no C compiler, no system libyang, no build script.
- Clean cross-compilation to any `GOOS`/`GOARCH` the Go toolchain targets.
- A dependency closure with no `runtime/cgo` and no C source, verified mechanically
  (below).

**Opting into the libyang backend (`CGO_ENABLED=1`) costs you:**

- A one-time native build of the vendored engine:
  `bash go/internal/libyang/build.sh`, a two-stage static CMake build of PCRE2 then
  libyang. You need a C toolchain and CMake.
- cgo at build time, which complicates cross-compilation.
- The libyang concurrency contract: an `ly_ctx` is build-once-then-frozen (shareable
  for reads), and a `*DataTree` is **not** concurrency-safe — give each goroutine its
  own (`Duplicate()`) or serialize access.

In exchange you get the complete, mature RFC-7950 data engine. The pure tiers never
reach the backend: `//go:build cgo` tags and the import-closure check keep it
strictly separated.

## Verifying the default surface is cgo-free

The cgo-free guarantee is enforced, not assumed. `scripts/check-go-default-pure.sh`
exercises `cambium`, `codegen`, `compat`, and `datatree` with `CGO_ENABLED=0`, then
inspects their actual transitive dependency closure and fails if it contains
`runtime/cgo`, anything matching `libyang`, `internal/libyang`, `libyangbackend`,
`github.com/openconfig/goyang`, the vendored `internal/yangparse/upstream` lexer, or
any package carrying cgo source files.

```bash
# Prove the default packages have no path to C:
scripts/check-go-default-pure.sh
```

Because the check reads the resolved dependency graph, an accidental import of the
backend from a pure package fails the gate rather than silently dragging C into the
default build. `scripts/green-bar.sh` runs it first.

## See also

- [Architecture](architecture.md) — the structural reasoning behind the tiers.
- [Overview](../overview.md) — where the tiers sit in the bigger picture.
- [Install](../guides/install.md) — `go get` vs building the cgo engine.
- [Roadmap](../contributing/roadmap.md) — the path toward a stable pure-Go data tier.
