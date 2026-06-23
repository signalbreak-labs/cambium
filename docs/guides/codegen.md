# Generating typed Go from YANG (`codegen`)

The pure-Go `codegen` package turns a loaded Cambium schema context into a single,
deterministic Go source file: typed structs for the module's data nodes, plus the
serializers, parser, and validator those structs need. It exists so that the
ordering Cambium computes at the schema level — RFC 7950 declaration order (§7.5.7),
list keys first (§7.8.5), `ordered-by user` insertion order — survives all the way into the
code your application ships, without a runtime schema, reflection, or a cgo
dependency.

`codegen` lives in the **Schema-IR tier (pure Go, `CGO_ENABLED=0`)**: both the
generator and the code it emits build with cgo disabled. The generated structs are
**self-contained** — they carry their own field-order manifest, serializers, and
validation logic, so nothing they do at runtime needs a live `*cambium.Context` or
the libyang backend. For where this tier sits relative to the data tiers, see
[Tiers and the cgo boundary](../concepts/tiers-and-cgo.md).

## Entry points

`GenerateGo` renders the current Go target:

```go
func GenerateGo(ctx *cambium.Context, module string) (string, error)
```

It takes a fully built `*cambium.Context` and the name of an implemented module,
and returns the generated module as a Go source string. There is no options struct
and no configuration surface — the input is the loaded context plus a module name,
and the generator is otherwise a black box. The complete, authoritative API is on
[pkg.go.dev](https://pkg.go.dev/github.com/signalbreak-labs/cambium/go/codegen).

The output is one `gofmt`-stable file, including its `package` declaration and
imports. Internally `GenerateGo` runs the assembled source through `go/format`, so
writing the returned string verbatim and running your normal `go build` is all that
is required. Re-running the generator on the same context produces the same bytes,
which makes generated code reviewable in version control and safe to gate in CI.

`Plan` exposes the ordered model before rendering:

```go
func Plan(ctx *cambium.Context, module string) (*ModulePlan, error)
```

`ModulePlan` is tagged with `PlanVersion` and contains ordered records, fields,
type summaries, identity metadata, serializer field order, and validation
metadata. It is generic enough for external renderers to consume the same ordered
schema/codegen planning model without using generated Go. Because Go is the only
shipping renderer today, the plan still includes Go type expressions; downstream
renderers can ignore those and use the embedded `cambium.SchemaNodeRef`,
`BaseType`, resolved type, defaults, constraints, and field-order data instead.

## What it emits

For a module, `GenerateGo` produces:

- **A typed struct per schema node.** Containers, list entries, RPC/action input
  and output, notifications, and the module root each become a Go struct; leaves
  map to typed Go fields, and nested containers and lists are nested struct types.
- **A per-struct field-order manifest** — a `var <Type>FieldOrder = []string{...}`
  listing each child's wire name in the exact order the serializers must emit it.
  This is the field-order manifest the project's one rule refers to: serialization
  walks this slice, never Go struct field order or map iteration.
- **Native XML and JSON_IETF serializers.** Every data struct implements
  `ToXML() string`, `ToJSONIETF() string`, and
  `ToJSONIETFWithDefaults(WithDefaultsMode) string`.
- **A JSON_IETF deserializer** — a top-level `FromJSONIETF(data []byte)` (and a
  per-operation `From<Type>JSONIETF(data []byte)` for RPC/notification documents)
  parses JSON_IETF back into the typed structs, then validates the result. XML is
  serialize-only; there is no generated `FromXML`.
- **`Validate() error`** on every data struct (see
  [What `Validate` covers](#what-validate-covers)).
- **With-defaults handling.** `ToJSONIETFWithDefaults` takes a `WithDefaultsMode`
  (`WithDefaultsExplicit`, `WithDefaultsTrim`, `WithDefaultsAll`,
  `WithDefaultsAllTagged`) controlling how schema-default values appear in output.
- **RFC 7952 metadata.** A struct whose leaf or leaf-list children can carry
  instance metadata gets a `CambiumMetadata map[string][]MetadataAnnotation` field,
  keyed by child wire name, that the serializers emit and the deserializer reads.

Every generated data struct satisfies a generated `CambiumStruct` interface:

```go
type CambiumStruct interface {
    ToXML() string
    ToJSONIETF() string
    ToJSONIETFWithDefaults(WithDefaultsMode) string
    Validate() error
}
```

## Ordering guarantees baked into the generated code

Generated code is where Cambium's ordering invariants stop being runtime
conventions and become mechanical properties of the emitted bytes. See
[Ordering](../concepts/ordering.md) for the model, and
[`/spec/ordering-invariants.md`](../../spec/ordering-invariants.md) for the
normative I1–I6 text.

- **Keys first, then schema declaration order (I3, I2).** Each struct's
  `FieldOrder` manifest lists list-key leaves first, in `key`-statement order, then
  the remaining children in effective schema declaration order. The serializers
  walk that manifest, so the emitted order is fixed at generation time and cannot
  drift with Go field rearrangement or map iteration.
- **`ordered-by user` is a positional-only type (I1).** A node marked
  `ordered-by user` is generated as a `UserOrderedVec[T]`, whose only mutators are
  positional — `InsertFirst`, `InsertLast`, `InsertBefore`, `InsertAfter`,
  `MoveBefore`, `MoveAfter`, `Remove` — and whose readers are `Len`, `IsEmpty`,
  `Get`, and `Iter`. There is no API that assigns an absolute sequence to a
  system-ordered node, so treating a system-ordered node as if it were
  user-ordered is a **compile error**, not a runtime check.
- **RPC/action/notification I/O in schema order (I4).** Generated I/O structs
  carry their children in effective schema order through the same field-order
  manifest.

Because the generated walk follows the compiled YANG declaration order (keys
first), serializer output is byte-identical to libyang's for the same data. That
is what lets the pure-Go generated serializers run in the default `CGO_ENABLED=0`
build while still matching the conformance golden outputs produced through the
optional [libyang backend](./data-tree-libyang.md).

## What `Validate` covers

The generated `Validate()` performs **value and structural validation** at the
schema level:

- type constraints — integer/decimal `range`, string/binary `length`, string
  `pattern` (RFC 7950 regular expressions);
- `mandatory` presence and list/leaf-list cardinality;
- `unique` constraints on list entries;
- `choice` selection rules, including mandatory-choice and mandatory-within-case.

What it does **not** do, by design, is anything that requires evaluating data
against the rest of an instance tree: there is **no `must`/`when` XPath evaluation
and no leafref instance-existence checking** in generated code. Those are
data-tier semantics. Keeping `Validate()` to value and structural checks is part of
what lets the entire codegen output stay inside the cgo-free import closure. For
full RFC 7950 semantic validation (`must`/`when` across the tree, leafref
resolution), parse and validate through the data tier — see the
[libyang backend guide](./data-tree-libyang.md).

## Worked example

The example is hermetic: it builds a context from an in-memory YANG string, with no
files on disk and no search path. Consider a module that places a container
*between* the two leaves of a composite key:

```go
package main

import (
    "fmt"
    "log"

    "github.com/signalbreak-labs/cambium/go/cambium"
    "github.com/signalbreak-labs/cambium/go/codegen"
)

const routesYANG = `
module routes {
  namespace "urn:example:routes";
  prefix rt;

  list route {
    key "dest-prefix next-hop-ip";
    leaf dest-prefix { type string; }
    container metrics {
      leaf distance { type uint8; }
    }
    leaf next-hop-ip { type string; }
  }
}
`

func main() {
    b, err := cambium.NewContextBuilder(cambium.ContextFlags{})
    if err != nil {
        log.Fatal(err)
    }
    if err := b.LoadModuleStr(routesYANG); err != nil {
        log.Fatal(err)
    }
    ctx, err := b.Build()
    if err != nil {
        log.Fatal(err)
    }

    src, err := codegen.GenerateGo(ctx, "routes")
    if err != nil {
        log.Fatal(err)
    }

    // src is one gofmt-stable file; write it verbatim and `go build`.
    fmt.Print(src)
}
```

The generated route-entry struct keeps `metrics` declared where YANG places it, but
the field-order manifest the serializers walk lists the **keys first**:

```go
type RoutesRouteEntry struct {
    DestPrefix      string
    NextHopIp       string
    Metrics         RoutesRouteEntryMetrics
    CambiumMetadata map[string][]MetadataAnnotation
}

var RoutesRouteEntryFieldOrder = []string{"dest-prefix", "next-hop-ip", "metrics"}
```

So even though `metrics` appears between `dest-prefix` and `next-hop-ip` in the
source, `ToXML()` and `ToJSONIETF()` emit `dest-prefix`, then `next-hop-ip`, then
`metrics` — list keys first, in `key`-statement order (I3), then the remaining
child in schema declaration order (I2), matching what NETCONF servers expect.

Run the generator with cgo disabled to confirm it stays in the pure tier:

```bash
CGO_ENABLED=0 go run .
```

## See also

- [Overview](../overview.md) — the order-semantics problem this generator serves
- [Ordering](../concepts/ordering.md) · [Tiers and the cgo boundary](../concepts/tiers-and-cgo.md)
- [Downstream schema consumers](./downstream-schema-consumers.md) — versioned schema IR and codegen planning API for external renderers
- [Schema introspection](./schema-introspection.md) — building the `*cambium.Context` you pass to `GenerateGo`
- [Quickstart](./quickstart.md) — load a module, walk the ordered tree, generate structs
- [The libyang backend](./data-tree-libyang.md) — full RFC 7950 data validation (`must`/`when`/leafref)
- [Glossary](../glossary.md)
- [Ordering invariants (spec)](../../spec/ordering-invariants.md) — normative I1–I6
- [Documentation index](../README.md)
