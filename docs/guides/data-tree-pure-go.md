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
opaque XML `anydata`/`anyxml`, cross-format opaque conversion, operation data, or
the full XPath function set — use the [libyang backend](data-tree-libyang.md) instead. The
[tiers & cgo boundary](../concepts/tiers-and-cgo.md) page lays out the trade-off.

## Parsing and serializing

`datatree.Parse(module, format, data)` decodes a document against a schema `Module`
(obtained from a frozen `Context`) into a `*Tree`. `(*Tree).Serialize(format)`
encodes it back. Both accept `FormatJSONIETF` (RFC 7951) and `FormatXML`. Output
order comes from the schema, not the input — members can arrive in any order and
come out in declaration order, keys first. List entries and leaf-list values
follow `ordered-by`: `ordered-by user` keeps the input order exactly (I1), and
`ordered-by system` configuration data comes out in the canonical order the
libyang backend uses (I2) — see [Values and canonical order](#values-and-canonical-order).

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
  `LeafListValues()` for a leaf-list's ordered values, `Children()` for a
  container's ordered children, and `Entries()` for a list's entries (each with
  keys first).

## Values and canonical order

Leaf values are held in their canonical form, as libyang stores them, whichever
format and lexical form they arrive in: decimal64 `45.50` becomes `45.5` (and `0`
becomes `0.0`), integers lose signs and leading zeros (`"+007"` is `"7"`), bits are
listed in position order, and an identityref naming one of the leaf's own
module's identities drops its module qualifier. `LeafValue` and `LeafListValues`
return these canonical JSON_IETF tokens. A value that is invalid for its type is
kept exactly as written, so `Validate` reports it. Integer and decimal64 text
longer than 256 characters is rejected as invalid before any numeric parsing.

Values that name other modules convert at the format boundary:

- **identityref** — in XML, a prefix resolves against the `xmlns` declarations in
  scope on the element (an unprefixed value against the default namespace), never
  the schema's import prefixes. On output a foreign identity is written with its
  module's own prefix, declared on the element: `<type xmlns="urn:a"
  xmlns:ianaift="urn:…:iana-if-type">ianaift:ethernetCsmacd</type>`.
- **instance-identifier** — held in canonical JSON_IETF form
  (`/mod:top/list[key='v']`, module names only where the module changes). XML
  input prefixes are resolved in scope and output qualifies every node with its
  module's prefix, declared on the element. A path that does not resolve against
  the schema is kept as written.

`ordered-by system` lists and leaf-lists are sorted when parsed, so navigation,
validation, and both output formats see the same canonical order: leaf-list values
by value, list entries by their key values in `key` order, each compared by type
the way libyang does (numbers numerically, strings and identity names bytewise,
enums by assigned value, bits by position, binary by decoded length then bytes, and
in a union the later member types first). As in libyang, state data and keyless
lists keep their input order. Derived types that libyang orders or canonicalizes
through a dedicated plugin (such as the `ietf-inet-types` address types) are
treated here as their base type, so an IPv6 address, for example, is compared and
printed as the string it was written as.

## Validation and defaults

- `(*Tree).Validate() error` checks mandatory nodes, cardinality
  (`min`/`max-elements`), uniqueness and list-key uniqueness, leafref instance
  existence, and `must`/`when` constraints over a growing XPath subset.
- `(*Tree).ApplyDefaults()` fills absent leaves with their schema defaults.

## Supported scope and limitations

This is the experimental part. `datatree` currently handles containers, leaves,
leaf-lists, lists, and opaque `anydata`/`anyxml` values in JSON_IETF, with the
validation above, and it preserves ordering invariants I1/I2/I3/I5 over what it
supports, including the canonical order of `ordered-by system` data. It does
**not** yet handle:

- Opaque `anydata`/`anyxml` in XML, or cross-format conversion of opaque content.
- RPC, action, and notification (operation) data.
- The full XPath function set — the engine implements a growing XPath 1.0 subset
  plus the YANG functions `re-match`, `bit-is-set`, `derived-from`, and
  `derived-from-or-self`, and **skips** `deref()` (and any other unimplemented
  function) rather than mis-evaluating it. Validation that depends on an
  unsupported construct is skipped, not failed.

In addition, leaf values are currently held as raw JSON tokens with XML conversion
layered on top; the planned neutral value-representation refactor will change the
internal model and the public surface that exposes leaf values. Treat any code
against `datatree` as needing revision when that lands.

## See also

- [libyang backend](data-tree-libyang.md) — the complete, stable data engine.
- [Tiers & the cgo boundary](../concepts/tiers-and-cgo.md) — choosing a tier.
- [Architecture](../concepts/architecture.md) — why two data-tree implementations exist.
- [Roadmap](../contributing/roadmap.md) — `datatree` status and direction.
