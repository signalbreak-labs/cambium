# Generating typed Go from YANG (`codegen`)

This guide covers the pure-Go `codegen` package, which turns a loaded Cambium
schema context into deterministic, typed Go source. The generator exists so that
the ordering Cambium computes at the schema level (RFC 7950 §7.8.5 declaration
order, list keys first, `ordered-by user` insertion order) survives all the way
into the serializers your application ships — without a runtime schema, reflection,
or a cgo dependency. `codegen` lives in the **Schema-IR tier (pure Go,
`CGO_ENABLED=0`)**, so the code it produces, and the code that produces it, build
with cgo disabled.

## The single entry point

`codegen` exposes exactly one function:

```go
func GenerateGo(ctx *cambium.Context, module string) (string, error)
```

It takes a fully built `*cambium.Context` (see
[schema introspection](./schema-introspection.md) for how to load one) and the
name of an implemented module, and returns the generated module as a Go source
string. There is **no options struct** and no configuration surface: the input is
the loaded context plus a module name, and the output is a single, deterministic,
`gofmt`-stable file. Re-running the generator on the same context produces the same
bytes, which makes generated code reviewable in version control and safe to gate in
CI.

The returned string is the entire file, including its `package` declaration and
imports. Writing it to disk and running your normal `go build` is all that is
required.

## What it emits

For a module, `GenerateGo` produces:

- **A typed struct per schema node** — containers, list entries, RPC/action input
  and output, and the module root each become a Go struct. Leaves map to typed Go
  fields (for example a `uint8` leaf becomes `*uint8`; a mandatory string key
  becomes `string`). Nested containers and list entries are nested struct types.
- **A per-struct field-order manifest** — a `var <Type>FieldOrder = []string{...}`
  listing each child's wire name in the exact order the serializers must emit them.
  This is the field-order manifest the project's "one rule" refers to: serialization
  walks this slice, never Go struct field order or map iteration.
- **Native XML and JSON_IETF serializers** — every data struct implements
  `ToXML() string`, `ToJSONIETF() string`, and
  `ToJSONIETFWithDefaults(WithDefaultsMode) string`. The XML output follows
  declaration order; the JSON_IETF output follows RFC 7951.
- **A JSON_IETF deserializer** — a top-level `FromJSONIETF(data []byte)` and a
  per-type `From<Type>JSONIETF(data []byte)` parse JSON_IETF back into the typed
  structs. (XML is serialize-only; there is no generated `FromXML`.)
- **`Validate() error`** — every data struct implements value validation (see
  [What `Validate` covers](#what-validate-covers) below).
- **With-defaults handling** — `ToJSONIETFWithDefaults` takes a `WithDefaultsMode`
  (`WithDefaultsExplicit`, `WithDefaultsTrim`, `WithDefaultsAll`,
  `WithDefaultsAllTagged`) to control how schema-default values appear in output,
  per RFC 6243 semantics.
- **RFC 7952 metadata** — structs whose nodes can carry instance metadata get a
  `CambiumMetadata map[string][]MetadataAnnotation` field, keyed by child wire name,
  that the XML and JSON_IETF serializers emit and the JSON_IETF deserializer reads.

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

The generated code is where Cambium's ordering invariants become mechanical
guarantees rather than runtime conventions:

- **Keys first, then schema declaration order (I3, I2).** Each struct's
  `FieldOrder` manifest lists list-key leaves first, in `key`-statement order, then
  the remaining children in effective schema declaration order. The serializers
  walk that manifest, so the emitted order is fixed at generation time and cannot
  drift with Go field rearrangement or map iteration.
- **`ordered-by user` is positional-only.** A node marked `ordered-by user` is
  generated as a `UserOrderedVec[T]`, whose only mutators are positional
  (`InsertFirst`, `InsertLast`, `InsertBefore`, `InsertAfter`, `MoveBefore`,
  `MoveAfter`, `Remove`) and whose readers are `Len`, `IsEmpty`, `Get`, and `Iter`.
  Because there is no API that assigns an absolute sequence to a system-ordered
  node, treating a system-ordered node as if it were user-ordered is a compile-time
  impossibility rather than a runtime mistake.
- **RPC/action input and output in schema order (I4).** Generated I/O structs carry
  their children in effective schema order via the same field-order manifest.

Because the generated walk follows the compiled YANG declaration order (keys
first), serializer output is **byte-identical to libyang's** for the same data.
That is what lets you run the pure-Go generated serializers in the default
`CGO_ENABLED=0` build while still matching the conformance golden outputs produced
through the optional [libyang backend](./libyang-backend.md).

## What `Validate` covers

The generated `Validate()` performs **value/structural validation** at the schema
level:

- type constraints — integer/decimal `range`, string/binary `length`, string
  `pattern` (RFC 7950 regular expressions);
- cardinality and `mandatory` presence;
- `unique` constraints on list entries;
- `choice` selection rules.

What it does **not** do, by design, is anything that requires evaluating data
against the rest of an instance tree: there is **no `must`/`when` XPath evaluation
and no leafref instance-existence checking** in generated code. Those are
data-tier semantics. For full RFC 7950 semantic validation
(`must`/`when`/`mandatory` across the tree, leafref resolution), parse and validate
through the **Backend/data tier (optional, requires cgo)** —
see [the libyang backend guide](./libyang-backend.md). Keeping the generated
`Validate()` to value/structural checks is what lets the entire codegen output stay
inside the cgo-free import closure.

## Worked example: write generated source to a file

Given a YANG module that places a container *between* the two leaves of a composite
key:

```text
module composite-key-with-interleaved-containers {
  namespace "urn:composite-key-with-interleaved-containers";
  prefix ckwic;

  list route {
    key "dest-prefix next-hop-ip";
    leaf dest-prefix { type string; }
    container metrics {
      leaf distance { type uint8; }
    }
    leaf next-hop-ip { type string; }
  }
}
```

load it, generate Go, and write the result to disk:

```go
package main

import (
    "log"
    "os"

    "github.com/signalbreak-labs/cambium/go/cambium"
    "github.com/signalbreak-labs/cambium/go/codegen"
)

func main() {
    ctx, err := cambium.NewContext()
    if err != nil {
        log.Fatal(err)
    }
    defer ctx.Close()

    if err := ctx.SetSearchPath("./yang"); err != nil {
        log.Fatal(err)
    }
    if err := ctx.LoadModule("composite-key-with-interleaved-containers"); err != nil {
        log.Fatal(err)
    }

    src, err := codegen.GenerateGo(ctx, "composite-key-with-interleaved-containers")
    if err != nil {
        log.Fatal(err)
    }

    // src is gofmt-stable; writing it verbatim is sufficient.
    if err := os.WriteFile("generated.go", []byte(src), 0o644); err != nil {
        log.Fatal(err)
    }
}
```

The generated route-entry struct keeps `metrics` declared where YANG places it, but
the field-order manifest the serializers walk lists the **keys first**:

```go
type CompositeKeyWithInterleavedContainersRouteEntry struct {
    DestPrefix string
    NextHopIp  string
    Metrics    CompositeKeyWithInterleavedContainersRouteEntryMetrics
    CambiumMetadata map[string][]MetadataAnnotation
}

var CompositeKeyWithInterleavedContainersRouteEntryFieldOrder = []string{"dest-prefix", "next-hop-ip", "metrics"}
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

- [Documentation index](../README.md)
- [Why Cambium](../why-cambium.md) — the order-semantics problem this generator serves
- [Schema introspection](./schema-introspection.md) — building the `*cambium.Context` you pass to `GenerateGo`
- [Migrating from goyang](./goyang-migration.md) — the `compat` package and ordered `Children()`
- [The libyang backend](./libyang-backend.md) — full RFC 7950 data validation (`must`/`when`/leafref)
- [Ordering invariants (spec)](../../spec/ordering-invariants.md) — normative I1–I6
