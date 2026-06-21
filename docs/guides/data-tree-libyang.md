# The libyang data tree backend

This guide covers `libyangbackend`, Cambium's **backend/data tier — optional,
requires cgo**. Where the schema-IR tier (the pure-Go `cambium`, `codegen`, and
`compat` packages) works with the *ordered schema*, this package works with
*instance data*: it parses, validates, serializes, diffs, and merges generic
`DataTree`s over a vendored, statically linked libyang + PCRE2. It exists because
full RFC 7950 semantic validation (`must`/`when` XPath, `mandatory`, leafref
instance existence) and byte-faithful generic data serialization need a complete
data engine; Cambium delegates that work to libyang and exposes it behind a
small, order-correct Go surface. This tier stays **outside** the default cgo-free
import closure, so adopting it is an explicit choice, not a default cost — see
[Tiers & the cgo boundary](../concepts/tiers-and-cgo.md).

The package godoc is the authoritative API reference; this guide is the
orientation. Every snippet below mirrors the package's own `_test.go` files
against the shared conformance fixtures.

> **godoc:** <https://pkg.go.dev/github.com/signalbreak-labs/cambium/go/libyangbackend>

## When to use this tier

Cambium ships three tiers (the full boundary treatment is in
[the overview](../overview.md) and [tiers-and-cgo](../concepts/tiers-and-cgo.md)).
Two of them handle data:

- The **experimental pure-Go `datatree` tier** (`CGO_ENABLED=0`) parses,
  serializes, and validates generic trees without any C toolchain, but its scope
  is deliberately narrower (no `anydata`/`anyxml`, no RPC/action/notification
  data, a core-subset XPath engine) and its API is still moving. See
  [the pure-Go data tree guide](./data-tree-pure-go.md).
- This **libyang backend tier** is the mature, RFC-complete data engine.

Reach for `libyangbackend` when you have *data* to handle and need
production-grade correctness:

- Parse a NETCONF/RESTCONF payload (XML or JSON_IETF) into a tree.
- Run full RFC 7950 validation, including `must`/`when` XPath and leafref
  instance existence — constraints the schema-IR tier's generated `Validate()`
  (emitted by `codegen.GenerateGo`) does not cover.
- Serialize a tree back to XML, JSON, JSON_IETF, or libyang's binary LYB.
- Compute and apply diffs, or merge two trees.

If you only need the ordered schema tree, typed-struct codegen, or the
goyang-shaped projection, stay in the pure tier — it builds with
`CGO_ENABLED=0` and pulls in no C code. See
[schema introspection](./schema-introspection.md) and [codegen](./codegen.md).
For installing either tier, see [install](./install.md); for an end-to-end first
run, [quickstart](./quickstart.md).

## Build the engine first

`libyangbackend` links a vendored libyang + PCRE2 statically; there is no
dependency on a system libyang. Build the engine once before compiling or
testing against this package:

```bash
bash go/internal/libyang/build.sh
```

This runs a two-stage static CMake build of the vendored PCRE2 and libyang under
`go/internal/libyang/.build/` (gitignored). The engine SHA and CMake flags are
pinned in `/VERSIONS`; the build honors that pin so the linked engine is exactly
the one the conformance corpus was generated against.

Then build and test with cgo enabled:

```bash
cd go
CGO_ENABLED=1 go test ./...
CGO_ENABLED=1 go vet ./...
```

Every file in the package carries a `//go:build cgo` constraint, so with
`CGO_ENABLED=0` the package simply drops out of the build. That is what keeps the
default schema/codegen surface cgo-free — a property machine-enforced by
`scripts/check-go-default-pure.sh`.

> **Import alias.** The package name is `libyangbackend`, but tests and examples
> import it aliased as `cambium`, because the data-tier surface mirrors the pure
> tier's names. This guide uses that alias, so `cambium.FormatXML`,
> `cambium.ParseModeDataOnly`, and `cambium.DefaultSerializeFlags()` below all
> resolve to symbols declared in this package.

```go
import cambium "github.com/signalbreak-labs/cambium/go/libyangbackend"
```

## Lifecycle and concurrency

A `Context` is a compiled YANG context. It is **build-once-then-frozen**: load
every module you need up front, then use it read-only to parse and validate data.
`NewContext()` returns an empty context; `SetSearchPath` appends a directory to
the module search path; `LoadModule` loads by name (resolving imports/includes
from the search path); `LoadModuleFromPath` loads a specific `.yang` file;
`NewData()` returns an empty in-memory tree bound to the context.

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

The concurrency contract is explicit:

- A **frozen `Context` is safe to share for reads** across goroutines — load all
  modules before any concurrent use, then never mutate the schema again.
- A **`*DataTree` is not concurrency-safe.** Do not parse into or mutate one tree
  from multiple goroutines. Give each goroutine its own tree via `Duplicate()`
  (a deep, independent copy), or serialize all access behind your own lock.
- The **FFI boundary is coarse-grained**: each `Parse`, `Serialize`, `Validate`,
  `Diff`, or `Merge` is one whole-document call into libyang, never a per-node
  chatter of C calls.

Close what you open. `Context.Close`, `DataTree.Close`, and `DataDiff.Close` free
the underlying C resources. A `DataTree` keeps its owning `Context` alive, so
close trees before (or alongside) the context.

## Parsing data

`Parse` reads a full data document; `ParseOp` reads an operation document (an
RPC, action, or notification reply).

```go
tree, err := ctx.Parse(cambium.FormatXML, cambium.ParseModeDataOnly, input)
if err != nil {
    return err
}
defer tree.Close()
```

`Format` selects the wire encoding: `FormatXML` (RFC 7950 XML), `FormatJSON` and
`FormatJSONIETF` (RFC 7951 JSON / gNMI JSON_IETF), and `FormatLYB` (libyang's
binary format).

`ParseMode` is a struct of separable options, not an enum. The common choice is
the package-level var `ParseModeDataOnly`, which parses without validating
(equivalent to `ParseMode{ParseOnly: true}`) so you can validate explicitly in a
later step. Other fields are `Strict` (reject unknown data nodes), `Opaque`
(parse unknown data as opaque nodes), `NoState`, and `LybModUpdate`. `Strict`
and `Opaque` are mutually exclusive — combining them returns `CAMBIUM_E0002`.

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
if err := tree.Validate(cambium.ValidateMode{Present: true, MultiError: true}); err != nil {
    var ve *cambium.ValidationErrors
    if errors.As(err, &ve) {
        if primary, ok := ve.Primary(); ok {
            log.Printf("validation failed: %s (%s)", primary.Message, primary.DataPath)
        }
        for _, d := range ve.Diagnostics() {
            log.Printf("  %s [%d]: %s", d.DataPath, d.ValidationCode, d.Message)
        }
    }
    return err
}
```

`ValidateMode` carries `NoState` (reject state data), `Present` (validate only
the nodes that exist, as for a partial datastore), and `MultiError` (accumulate
every diagnostic rather than stopping at the first). Each `Diagnostic` carries
`Message`, `DataPath`, `SchemaPath`, `ErrorType`, `ErrorAppTag`, and a
fine-grained `ValidationCode` — `ValidationMust`, `ValidationWhen`,
`ValidationMandatory`, `ValidationLeafref`, or `ValidationInvalidValue`. The
top-level `Code` on a validation failure is always `CAMBIUM_E0003`
(`RuleCodeValidate`); see [rule-codes](../../spec/rule-codes.md).

## Serializing

`Serialize` writes one ordered walk of the tree. Element order is structural — a
single pass over libyang's `lyd_node.next/prev` sibling chain — never a map or
struct iteration, so `ordered-by user` insertion order and schema declaration
order both survive the round trip.

```go
out, err := tree.Serialize(cambium.FormatJSONIETF, cambium.DefaultSerializeFlags())
```

Use `DefaultSerializeFlags()` for the conformance-golden profile. The Go zero
value of `SerializeFlags` is **not** that default — in particular its `Siblings`
field is `false`, which serializes only the first root and its descendants. Call
`DefaultSerializeFlags()` (which sets `Siblings: true`) or set `Siblings`
explicitly. The other fields are `Shrink` (drop insignificant whitespace),
`KeepEmptyContainers`, and `WithDefaults` — one of `WithDefaultsExplicit`
(the default), `WithDefaultsTrim`, `WithDefaultsAll`, or `WithDefaultsAllTagged`.
You can also start from the default and adjust a field:

```go
flags := cambium.DefaultSerializeFlags()
flags.Shrink = true
flags.KeepEmptyContainers = true
out, err := tree.Serialize(cambium.FormatXML, flags)
```

`FormatLYB` serializes to libyang's compact binary format, which round-trips
losslessly back through `Parse(cambium.FormatLYB, ...)`.

## Reading and mutating by path

The tree exposes a path/XPath access surface. `Get` returns a `NodeRef` at an
absolute data path (a missing path is `CAMBIUM_E0006`, `RuleCodeDataPath`);
`TryGet` returns `ok=false` instead of an error; `Exists` is the boolean form;
`Select` evaluates an XPath and returns a `NodeSet` of matches in document order;
`RootNodes` returns the top-level data nodes in declaration order.

```go
node, err := tree.Get("/cambium-data-read-demo:top/rw-flag")
if err != nil {
    return err
}
v, ok, err := node.Value()       // typed value; (Value{}, false, nil) for non-term nodes
if err == nil && ok {
    if b, ok := v.Bool(); ok {
        _ = b
    }
}

set, err := tree.Select("/cambium-data-read-demo:top/*")
```

A `NodeRef` also offers `ValueStr` (canonical value string), `Schema` (the
compiled `SchemaNodeRef` for the node), `Children`/`Siblings` (in declaration
order), and `AsUserOrdered`.

For mutation: `NewPath` creates or updates a node (a nil `value` creates an inner
container/list node, a non-nil `*string` sets a leaf/leaf-list); `SetValue`
changes an existing leaf value and reports whether it actually changed;
`RemovePath` removes and frees a subtree; `UnlinkPath` detaches a subtree and
returns it as an owned tree sharing the same context; `AddDefaults` materializes
implicit/default nodes.

```go
if _, err := tree.NewPath("/cambium-data-crud-demo:top", nil, cambium.NewPathOpts{}); err != nil {
    return err
}
counter := "5"
if _, err := tree.NewPath("/cambium-data-crud-demo:top/counter", &counter, cambium.NewPathOpts{}); err != nil {
    return err
}
if err := tree.AddDefaults(cambium.ImplicitOpts{}); err != nil {
    return err
}
```

## Ordered-by user lists (invariant I1)

`ordered-by user` lists and leaf-lists carry caller insertion order as
significant data. `UserOrderedListAt` returns a **positional-only**
`UserOrderedList` handle. It deliberately has no order-agnostic mutator — only
`InsertFirst`, `InsertLast`, `InsertBefore`, `InsertAfter`, `MoveBefore`, and
`MoveAfter` — so reordering a system-ordered node by mistake cannot even be
expressed. Asking for a handle on a *system*-ordered list fails at the boundary
with `CAMBIUM_E0005` (`RuleCodeOrderedList`).

```go
list, err := tree.UserOrderedListAt("/ordered-user-demo:config/entry[name='c']")
if err != nil {
    return err
}
// indices: 0=c, 1=a, 2=b. Move b before c -> b, c, a.
if err := list.MoveBefore(2, 0); err != nil {
    return err
}
```

For a read-only view, `NodeRef.AsUserOrdered` returns a `UserOrderedView`
(positional `Len`/`Get`/`Iter`/`FindByKey`) for user-ordered targets, and
`ok=false` with no error for system-ordered ones. This is the mechanism behind
invariant **I1**: insertion order is byte-exact across parse → tree → serialize.
The full normative text is in [ordering-invariants](../../spec/ordering-invariants.md).

## Diff, apply, and merge

`Diff` computes a `*DataDiff` describing the edits that turn one tree into
another; `DiffApply` applies a diff to a tree in place; `Merge` merges a source
tree into a destination in place. Both trees must share the same context.

```go
diff, err := base.Diff(updated, cambium.DiffOpts{})
if err != nil {
    return err
}
defer diff.Close()
if !diff.IsEmpty() {
    if err := base.DiffApply(diff); err != nil {
        return err
    }
}
```

`DiffOpts` has a `Defaults` field (include default nodes in the diff). A
`*DataDiff` reports `IsEmpty`, `IsOrderedByUser`, and `Edits()` (apply-safe
document order); each `DiffEdit` exposes `Op()` (`DiffOpCreate`, `DiffOpDelete`,
`DiffOpReplace`), `Path()`, `Value()`, and `IsUserOrdered()`. A reorder of an
`ordered-by user` list is reported as a single atomic edit flagged
`IsUserOrdered()`, never decomposed into order-losing scalar replaces.

`Merge` pre-scans for conflicts: a leaf present in both trees with differing
values is **rejected before any mutation** with `CAMBIUM_E0003`
(`RuleCodeValidate`), rather than silently overwriting. Check for it with the
same `errors.As` pattern used for validation:

```go
if err := dst.Merge(src, cambium.MergeOpts{}); err != nil {
    var ce *cambium.Error
    if errors.As(err, &ce) && ce.RuleCode() == cambium.RuleCodeValidate {
        // conflicting leaf values; dst is unmodified
    }
    return err
}
```

## Round-trip worked example

This walks the full data path end to end: load a module, parse JSON, validate,
serialize to LYB, parse the LYB back, and confirm the JSON is identical. It uses
the shared `ordered-user` conformance fixture, whose `ordered-by user` list
exercises insertion-order preservation (invariant I1). The pattern mirrors
`go/libyangbackend/lyb_round_trip_test.go`.

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
    // conformance/fixtures/ordered-user/module + conformance/golden/ordered-user/output.json
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

## Invariants this tier guarantees

Over real instance data, the libyang backend guarantees:

- **I1** — `ordered-by user` entry/value order is byte-exact across
  parse → tree → serialize (the positional `UserOrderedList`/`UserOrderedView`
  mechanism above).
- **I2** — schema children are exposed and serialized in effective declaration
  order; data output is canonical and deterministic, never from a map.
- **I3** — list keys are exposed and serialized first, in `key`-statement order.
- **I4** — RPC, action, and notification children appear in schema order.
- **I5** — lists and leaf-lists serialize as JSON arrays carrying I1/I2 order.

Serialization realizes all of these as a single ordered walk of libyang's
`lyd_node.next/prev` chain. I6 (gNMI) is future work. The normative statements
are in [ordering-invariants](../../spec/ordering-invariants.md).

## See also

- [Overview](../overview.md) — the order-semantics problem, the design response, and the three tiers
- [Tiers & the cgo boundary](../concepts/tiers-and-cgo.md) · [Architecture](../concepts/architecture.md)
- [Install](./install.md) · [Quickstart](./quickstart.md)
- [Pure-Go data tree](./data-tree-pure-go.md) — the experimental cgo-free data tier
- [Glossary](../glossary.md) — LYB, tiers, ordered-by user/system, and more
- [Ordering invariants](../../spec/ordering-invariants.md) · [Rule codes](../../spec/rule-codes.md)
- [Documentation index](../README.md)
