# Pure-Go data tree (experimental)

> **Experimental.** The `datatree` package is under active development. Its public
> API and its internal value representation **will change** — a raw-JSON-token
> representation is being reworked into a neutral value model. Its feature scope is
> narrower than the [libyang backend](data-tree-libyang.md). **Do not depend on its
> API in production yet.** If you need stable, RFC-complete data handling today, use
> the [libyang backend](data-tree-libyang.md). Status and direction are tracked in
> the [roadmap](../contributing/roadmap.md).

The `datatree` package is a generic data tree for YANG instance data that needs
**no libyang and no cgo**. It parses a document against a Cambium schema into an
ordered tree and serializes it back in effective schema declaration order, so you
can round-trip and validate generic data while staying in the pure-Go,
`CGO_ENABLED=0` world the schema tier already lives in. It is the long-term goal —
a complete data tier with the same portability as the schema tier — under
construction. The API reference is the package godoc on
[pkg.go.dev](https://pkg.go.dev/github.com/signalbreak-labs/cambium/go/datatree).

## When to use it

Reach for `datatree` when you want generic data parse/serialize/validate without a
C toolchain, you can tolerate an unstable API, and your models stay inside its
supported scope (below). When you need production-grade, RFC-complete validation —
the full XPath function set, `anydata`/`anyxml`, operation data — use the
[libyang backend](data-tree-libyang.md) instead. The
[tiers & cgo boundary](../concepts/tiers-and-cgo.md) page lays out the trade-off.

## Parsing and serializing

`datatree.Parse(module, format, data)` decodes a document against a schema `Module`
(obtained from a frozen `Context`) into a `*Tree`. `(*Tree).Serialize(format)`
encodes it back. Both accept `FormatJSONIETF` (RFC 7951) and `FormatXML`. Output
order comes from the schema, not the input — members can arrive in any order and
come out in declaration order, keys first.

```go
const src = `module dt {
  namespace "urn:dt";
  prefix dt;
  container c {
    leaf z { type string; }
    leaf a { type string; }
  }
}`

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
mod, err := ctx.Schema("dt")
if err != nil {
	panic(err)
}

// Input members are out of schema order; the tree normalizes them to z, a.
tree, err := datatree.Parse(mod, datatree.FormatJSONIETF, []byte(`{"dt:c":{"a":"1","z":"2"}}`))
if err != nil {
	panic(err)
}

roots := tree.RootNodes()
for _, child := range roots[0].Children() {
	fmt.Println(child.Name())
}
// z
// a
```

This program runs as a doc-test in `go/datatree/example_test.go`, so it stays in
sync with the API. To cross formats, parse one format and serialize another —
`Parse(mod, FormatXML, ...)` then `Serialize(FormatJSONIETF)` round-trips XML to
JSON_IETF.

## Reading and navigating

A parsed `*Tree` exposes its data as ordered `Node` values:

- `RootNodes() []Node` — the top-level nodes in schema order.
- `Find(path) (Node, bool)` — a slash-path lookup.
- On a `Node`: `Name()`, `Module()`, the kind predicates (`IsLeaf()`,
  `IsLeafList()`, `IsContainer()`, `IsList()`), `LeafValue()` for a leaf's value,
  `Children()` for a container's ordered children, and `Entries()` for a list's
  entries (each with keys first).

## Validation and defaults

- `(*Tree).Validate() error` checks mandatory nodes, cardinality
  (`min`/`max-elements`), uniqueness and list-key uniqueness, leafref instance
  existence, and `must`/`when` constraints over a core XPath subset.
- `(*Tree).ApplyDefaults()` fills absent leaves with their schema defaults.

## Supported scope and limitations

This is the experimental part. `datatree` currently handles containers, leaves,
leaf-lists, and lists across JSON_IETF and XML, with the validation above, and it
preserves ordering invariants I1/I2/I3/I5 over what it supports. It does **not** yet
handle:

- `anydata` and `anyxml` nodes.
- RPC, action, and notification (operation) data.
- The full XPath function set — the engine implements a core subset and
  **skips** `derived-from`, `re-match`, `bit-is-set`, and `deref` rather than
  mis-evaluating them. Validation that depends on an unsupported construct is
  skipped, not failed.

In addition, leaf values are currently held as raw JSON tokens with XML conversion
layered on top; the planned neutral value-representation refactor will change the
internal model and the public surface that exposes leaf values. Treat any code
against `datatree` as needing revision when that lands.

## See also

- [libyang backend](data-tree-libyang.md) — the complete, stable data engine.
- [Tiers & the cgo boundary](../concepts/tiers-and-cgo.md) — choosing a tier.
- [Architecture](../concepts/architecture.md) — why two data-tree implementations exist.
- [Roadmap](../contributing/roadmap.md) — `datatree` status and direction.
