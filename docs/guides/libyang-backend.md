# The libyang data backend

This guide covers `libyangbackend`, Cambium's optional **Backend/data tier
(optional, requires cgo)**. Where the **Schema-IR tier (pure Go,
`CGO_ENABLED=0`)** works with the ordered schema, this package works with
*instance data*: it parses, validates, serializes, diffs, and merges generic
`DataTree`s over a vendored, statically linked libyang + PCRE2. It exists because
full RFC 7950 semantic validation (`must`/`when`, `mandatory`, leafref instance
existence) and byte-faithful generic data serialization need a complete data
engine; Cambium delegates that work to libyang and exposes it behind a small,
order-correct Go surface. This tier stays **outside** the default cgo-free import
closure, so adopting it is an explicit choice, not a default cost.

## When you need this tier

Reach for `libyangbackend` when you have *data* to handle, not just schema:

- Parse a NETCONF/RESTCONF payload (XML or JSON_IETF) into a tree.
- Run full RFC 7950 validation, including `must`/`when` XPath and leafref
  instance existence — constraints the pure tier's generated value validation
  (the `Validate()` method emitted by `codegen.GenerateGo`) does not cover.
- Serialize a tree back to XML, JSON, JSON_IETF, or libyang's binary LYB.
- Compute and apply diffs, or merge two trees.

If you only need the ordered schema tree, typed-struct codegen, or the
goyang-shaped projection, stay in the pure tier — see
[schema introspection](./schema-introspection.md) and [codegen](./codegen.md).
Those packages build with `CGO_ENABLED=0` and pull in no C code.

## Build the engine first

`libyangbackend` links a vendored libyang + PCRE2 statically; there is no
dependency on a system libyang. Build the engine once before compiling or
testing against this package:

```bash
bash go/internal/libyang/build.sh
```

This runs a two-stage static CMake build of the vendored PCRE2 and libyang under
`go/internal/libyang/.build/` (gitignored). The engine SHA and CMake flags are
pinned in `/VERSIONS`; the build honors that pin so every binding links the same
engine.

Then build and test with cgo enabled:

```bash
cd go
CGO_ENABLED=1 go test ./...
CGO_ENABLED=1 go vet ./...
```

Every file in the package carries a `//go:build cgo` constraint, so with
`CGO_ENABLED=0` the package simply drops out of the build — that is what keeps
the default schema/codegen surface cgo-free, a property machine-enforced by
`scripts/check-go-default-pure.sh`.

> **Import alias.** The package name is `libyangbackend`, but tests and examples
> commonly import it aliased as `cambium` because the data-tier surface mirrors
> the pure tier's names. This guide uses that alias.

```go
import cambium "github.com/signalbreak-labs/cambium/go/libyangbackend"
```

## Context lifecycle

A `Context` is a compiled YANG context. It is **build-once-then-frozen**: load
every module you need up front, then use it read-only to parse and validate data.

```go
ctx, err := cambium.NewContext()
if err != nil {
    return err
}
defer ctx.Close()

if err := ctx.SetSearchPath(moduleDir); err != nil {
    return err
}
if err := ctx.LoadModule("ordered-user-demo"); err != nil {
    return err
}
```

`SetSearchPath` appends a directory to the module search path; `LoadModule`
loads by name (resolving imports/includes from the search path), and
`LoadModuleFromPath` loads a specific `.yang` file. `NewData` returns an empty
in-memory tree against the context.

> **Concurrency.** The context is safe to share once frozen, but data trees are
> **not concurrency-safe**: do not parse into or mutate a `DataTree` from
> multiple goroutines without external synchronization.

## Parsing data

`Parse` reads a full data document; `ParseOp` reads an operation document (an
RPC, action, or notification).

```go
tree, err := ctx.Parse(cambium.FormatJSON, cambium.ParseModeDataOnly, inputJSON)
if err != nil {
    return err
}
defer tree.Close()
```

`Format` selects the wire encoding: `FormatXML` (RFC 7950 XML), `FormatJSON` and
`FormatJSONIETF` (RFC 7951 JSON / gNMI JSON_IETF), and `FormatLYB` (libyang's
binary format).

`ParseMode` is a struct of separable options, not an enum. The most common
choice is the package var `ParseModeDataOnly`, which parses without validating
(equivalent to `ParseMode{ParseOnly: true}`) so you can validate explicitly in a
later step. Other fields include `Strict` (reject unknown nodes), `Opaque`
(parse unknown data as opaque nodes), `NoState`, and `LybModUpdate`. `Strict`
and `Opaque` are mutually exclusive.

For operation documents, pass an `OpType` — `OpTypeRPC`, `OpTypeNotification`,
or `OpTypeReply`:

```go
op, err := ctx.ParseOp(cambium.FormatXML, cambium.OpTypeRPC, rpcXML)
```

## Validating

`Validate` runs full RFC 7950 semantic validation. On failure it returns an
error that wraps a `*ValidationErrors`; recover it with `errors.As` to inspect
the structured diagnostics.

```go
if err := tree.Validate(cambium.ValidateMode{}); err != nil {
    var ve *cambium.ValidationErrors
    if errors.As(err, &ve) {
        if primary, ok := ve.Primary(); ok {
            log.Printf("validation failed: %s (%s)", primary.Message, primary.DataPath)
        }
        for _, d := range ve.Diagnostics() {
            log.Printf("  %s [%s]: %s", d.DataPath, d.Code, d.Message)
        }
    }
    return err
}
```

`ValidateMode` carries `NoState` (reject state data), `Present` (validate only
nodes that exist), and `MultiError` (accumulate every diagnostic rather than
stopping at the first). Each `Diagnostic` carries `Message`, `DataPath`,
`SchemaPath`, `ErrorAppTag`, and a fine-grained `ValidationCode` (for example
`ValidationMust`, `ValidationWhen`, `ValidationMandatory`, `ValidationLeafref`,
`ValidationInvalidValue`). The top-level `Code` on a validation failure is
always `CAMBIUM_E0003` (`RuleCodeValidate`).

## Serializing

`Serialize` writes one ordered walk of the tree. Element order is structural — a
single pass over libyang's sibling chain — never a map or struct iteration, so
`ordered-by user` insertion order and schema declaration order survive the round
trip.

```go
out, err := tree.Serialize(cambium.FormatJSONIETF, cambium.DefaultSerializeFlags())
```

Use `DefaultSerializeFlags()` for the conformance-golden profile. The Go zero
value of `SerializeFlags` is **not** that default — in particular its `Siblings`
field is `false`, which serializes only the first root and its descendants. Call
`DefaultSerializeFlags()` (which sets `Siblings: true`) or set `Siblings`
explicitly. Other fields are `Shrink` (drop insignificant whitespace),
`KeepEmptyContainers`, and `WithDefaults` (one of `WithDefaultsExplicit`,
`WithDefaultsTrim`, `WithDefaultsAll`, `WithDefaultsAllTagged`).

`FormatLYB` serializes to libyang's compact binary format, which round-trips
losslessly back through `Parse(FormatLYB, ...)`.

## Diff, apply, and merge

`Diff` computes a `*DataDiff` describing the edits that turn one tree into
another; `DiffApply` applies a diff to a tree in place; `Merge` merges a source
tree into a destination in place.

```go
diff, err := base.Diff(updated, cambium.DiffOpts{})
if err != nil {
    return err
}
if !diff.IsEmpty() {
    if err := base.DiffApply(diff); err != nil {
        return err
    }
}
defer diff.Close()
```

`DiffOpts` has a `Defaults` field (include default nodes in the diff). Both trees
must share the same context.

`Merge` pre-scans for conflicts: a leaf present in both trees with differing
values is **rejected before any mutation** with rule code `CAMBIUM_E0003`
(`RuleCodeValidate`), rather than silently overwriting. Check for it with the
same `errors.As` pattern used for validation:

```go
if err := dst.Merge(src, cambium.MergeOpts{}); err != nil {
    var ce *cambium.Error
    if errors.As(err, &ce) && ce.RuleCode() == cambium.RuleCodeValidate {
        // conflicting leaf values
    }
    return err
}
```

## Round-trip worked example

This walks the full data path end to end: load a module, parse JSON, validate,
serialize to LYB, parse the LYB back, and confirm the JSON is identical. It uses
the `ordered-user-demo` fixture, whose `ordered-by user` list exercises
insertion-order preservation (invariant I1).

```go
package main

import (
    "bytes"
    "errors"
    "fmt"
    "os"

    cambium "github.com/signalbreak-labs/cambium/go/libyangbackend"
)

func run(moduleDir string, inputJSON []byte) error {
    ctx, err := cambium.NewContext()
    if err != nil {
        return err
    }
    defer ctx.Close()

    if err := ctx.SetSearchPath(moduleDir); err != nil {
        return err
    }
    if err := ctx.LoadModule("ordered-user-demo"); err != nil {
        return err
    }

    // Parse without validating, then validate explicitly.
    tree, err := ctx.Parse(cambium.FormatJSON, cambium.ParseModeDataOnly, inputJSON)
    if err != nil {
        return err
    }
    defer tree.Close()

    if err := tree.Validate(cambium.ValidateMode{}); err != nil {
        var ve *cambium.ValidationErrors
        if errors.As(err, &ve) {
            if p, ok := ve.Primary(); ok {
                return fmt.Errorf("validate: %s at %s", p.Message, p.DataPath)
            }
        }
        return err
    }

    // Serialize to LYB, then parse it back — the binary form round-trips.
    lyb, err := tree.Serialize(cambium.FormatLYB, cambium.DefaultSerializeFlags())
    if err != nil {
        return err
    }
    round, err := ctx.Parse(cambium.FormatLYB, cambium.ParseModeDataOnly, lyb)
    if err != nil {
        return err
    }
    defer round.Close()

    roundJSON, err := round.Serialize(cambium.FormatJSON, cambium.DefaultSerializeFlags())
    if err != nil {
        return err
    }
    if !bytes.Equal(bytes.TrimSpace(roundJSON), bytes.TrimSpace(inputJSON)) {
        return fmt.Errorf("round trip mismatch:\nwant %s\ngot  %s", inputJSON, roundJSON)
    }
    return nil
}

func main() {
    data, err := os.ReadFile("output.json")
    if err != nil {
        panic(err)
    }
    if err := run("module", data); err != nil {
        panic(err)
    }
}
```

Because both serializations use `DefaultSerializeFlags()` and order is a
structural property of the tree, the JSON before and after the LYB hop is
byte-for-byte identical, including the `ordered-by user` entries.

## Lifecycle and safety notes

- The `Context` is **build-once-then-frozen**: load all modules before parsing
  data; do not mutate the schema afterward.
- `DataTree`s are **not concurrency-safe**. Do not share a tree across goroutines
  without your own synchronization.
- Close what you open: `Context.Close`, `DataTree.Close`, and `DataDiff.Close`
  free the underlying C resources. A `DataTree` keeps its owning `Context` alive,
  so close trees before (or alongside) the context.
- This tier requires cgo and the prebuilt engine. Keep it out of any build that
  must remain pure — the pure tier is the default surface for that reason.

## See also

- [Documentation index](../README.md)
- [Why Cambium](../why-cambium.md) — the order-semantics problem and the design response
- [Architecture](../architecture.md) — the two tiers and where libyang sits
- [Schema introspection](./schema-introspection.md) — the pure-Go ordered schema tree
- [Codegen](./codegen.md) — typed-struct generation with native serializers
- [Conformance](../conformance.md) — the shared corpus and golden outputs
- [Ordering invariants](../../spec/ordering-invariants.md) — normative I1–I6
- [Glossary](../glossary.md) — LYB, tiers, ordered-by user/system, and more
