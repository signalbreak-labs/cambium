# Quickstart

This is the fast path through Cambium's pure-Go tier: load a YANG module, walk the
ordered schema tree, and generate typed Go structs — all with `CGO_ENABLED=0`, no C
toolchain. By the end you will have seen the one property that defines Cambium:
children come back in schema declaration order, not map order.

For installation details (including the optional cgo data backend) see
[install](install.md). For the conceptual picture, see the [overview](../overview.md).

## Install

```bash
go get github.com/signalbreak-labs/cambium/go
```

The packages used here — `cambium` and `codegen` — are pure Go and need nothing
else.

## Load a module

A `Context` is built once and then frozen. Use `ContextBuilder` to load modules,
then `Build()` to get an immutable context. Modules can come from a search path
(`LoadModule`) or from an in-memory string (`LoadModuleStr`); the examples here use
a string so they are self-contained.

```go
b, err := cambium.NewContextBuilder(cambium.ContextFlags{})
if err != nil {
	panic(err)
}
if err := b.LoadModuleStr(src); err != nil {
	panic(err)
}
ctx, err := b.Build()
if err != nil {
	panic(err)
}
```

## Walk the ordered schema tree

`(*Context).Schema(module)` returns a `Module` handle. From there `TopLevel()`,
`Children()`, and friends return `SchemaChildren` — an ordered view you iterate with
`Iter()`. The children of `top` below are declared `z, m, a`, and that is exactly
the order you get back.

```go
const src = `module order-demo {
  namespace "urn:order-demo";
  prefix od;
  container top {
    leaf z { type string; }
    leaf m { type string; }
    leaf a { type string; }
  }
}`

mod, err := ctx.Schema("order-demo")
if err != nil {
	panic(err)
}
top, ok := mod.TopLevel().Lookup("top")
if !ok {
	panic("container top not found")
}
for child := range top.Children().Iter() {
	fmt.Println(child.Name())
}
// z
// m
// a
```

A map-backed library — goyang stores a node's children in a Go map — would hand you
`a, m, z`. Cambium preserves declaration order because a NETCONF server expects
children on the wire in schema order; emit them sorted or shuffled and a strict
server rejects the request. That is [invariant I2](../concepts/ordering.md), and
order-sensitive serialization and codegen depend on it. This program runs as a doc-test in
`go/cambium/example_test.go`, so it cannot silently drift from the API. The full
introspection surface — node kinds, list keys, leaf types, constraints — is in the
[schema introspection guide](schema-introspection.md).

## Generate typed structs

From the same ordered context, `codegen.GenerateGo(ctx, module)` emits typed Go
structs with a per-struct field-order manifest and native, order-correct XML and
JSON_IETF (de)serializers, plus `Validate()`, with-defaults, and RFC-7952 metadata.

```go
out, err := codegen.GenerateGo(ctx, "order-demo")
if err != nil {
	panic(err)
}
fmt.Println(out) // gofmt-clean Go source you write to a file
```

The generated serializers walk the field-order manifest, never native map or struct
iteration, so their output follows compiled YANG declaration order with keys first.
The generated structs are self-contained — they do not need a live `Context` at
runtime. See the [codegen guide](codegen.md) for what exactly is emitted and the
ordering guarantees baked in.

## Working with instance data

The quickstart so far is schema and codegen — the pure, stable tier. To parse,
validate, and serialize generic *instance data*, you have two options:

- The experimental, cgo-free [`datatree` tier](data-tree-pure-go.md) — generic data
  round-trip and validation without a C toolchain, with an unstable API and a
  narrower feature set.
- The [libyang backend](data-tree-libyang.md) — the complete RFC-7950 data engine
  (full `must`/`when`, leafref instance existence, diff/merge, LYB), which requires
  cgo and a one-time native build.

See [tiers & the cgo boundary](../concepts/tiers-and-cgo.md) to choose.

## Next

- [Schema introspection](schema-introspection.md) — the full `cambium` package.
- [Codegen](codegen.md) — everything `GenerateGo` emits.
- [Migrating from goyang](goyang-migration.md) — the `compat` package.
- [Ordering](../concepts/ordering.md) — why order is structural.
