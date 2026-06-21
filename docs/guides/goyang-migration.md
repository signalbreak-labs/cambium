# Migrating from goyang

For new code, migrate to the native `cambium` package first. It exposes the
ordered schema IR directly through `Context`, `Module`, `SchemaNodeRef`, and
`SchemaChildren`, and it exposes a native raw statement parser through
`ParseStatements` / `ReadStatements`. It also provides native equivalents for
common helper-style calls such as `PathsWithModules`, `CamelCase`, source
locations, prefix/path resolution, extension matching, and YANG numeric/range
parsing. That path keeps goyang's historical AST and `Entry` model out of your
application.

Package `compat` remains a cgo-free, goyang-shaped schema projection in the
**Schema-IR tier (pure Go, `CGO_ENABLED=0`)**. Use it when you need to move
existing goyang call sites with minimal edits. `compat` mirrors the surface of
goyang's `pkg/yang` — the `Entry` tree, the `Modules` loader, `ToEntry`, the
`Node` AST, `YangType` — but the projection is **read-only**: a view into a
Cambium-built ordered schema, not a mutable tree builder. There is exactly one
behavioral difference to plan for, and it is the reason the package exists:
ordered traversal goes through `Entry.Children()` (schema declaration order)
rather than iterating the `Entry.Dir` map.

The godoc is the API authority. This guide explains the shape and the one
migration step; for the exact, current symbol set see
[pkg.go.dev/github.com/signalbreak-labs/cambium/go/compat](https://pkg.go.dev/github.com/signalbreak-labs/cambium/go/compat).

## Prefer the native Cambium API

The native replacement for common goyang read use cases is:

- `ContextBuilder` / `Context` for loading modules from paths or strings.
- `Context.Schema(module)` for the compiled module handle.
- `Module.TopLevel()`, `Children()`, `RPCs()`, `Actions()`, and
  `Notifications()` for ordered schema entry points.
- `SchemaNodeRef` for node metadata, types, defaults, keys, constraints,
  extensions, and operation I/O.
- `ParseStatements(input, name)` and `ReadStatements(path)` for raw YANG
  statement inspection without the goyang-shaped `compat.Parse`.
- `PathsWithModules(root)` for module search-path discovery.
- `CamelCase(name)` for goyang-compatible exported identifier spelling.
- `Module.Location()` / `SchemaNodeRef.Location()` for source diagnostics.
- `Module.ResolvePrefix(prefix)` for native prefix-to-module resolution.
- `SchemaNodeRef.Path()` and `SchemaNodeRef.FindPath(path)` for goyang-style
  node path inspection and relative traversal.
- `Module.MatchingExtensions(module, name)` and
  `SchemaNodeRef.MatchingExtensions(module, name)` for resolved extension
  filtering.
- `Number`, `YRange`, `YangRange`, `ParseInt`, `ParseDecimal`,
  `ParseRangesInt`, `ParseRangesDecimal`, `FromInt`, `FromUint`, `FromFloat`,
  and `Frac` for YANG numeric/range helper code.

For example, a raw statement walk can use Cambium's native parser directly:

```go
stmts, err := cambium.ParseStatements(src, "demo.yang")
if err != nil {
    return err
}
for _, st := range stmts {
    arg, _ := st.Argument()
    fmt.Println(st.Keyword(), arg)
}
```

The returned `Statement` handles are read-only and preserve child order through
`SubStatements()`. They are for source inspection; use `Context.Schema` when you
need semantic schema resolution.

## What `compat` is for

`compat` exists so you can keep familiar goyang call shapes while reading a
Cambium-built schema tree. It targets the migration use case: porting existing
analysis, lookup, and walk code that already speaks `*yang.Entry`. The package
sits alongside `cambium` and `codegen` in the pure-Go Schema-IR tier and imports no
cgo or libyang code, so a port does not pull a C toolchain into your build.

goyang is a mature, widely used library. It made a reasonable choice for its goals:
it stores a node's effective children in a Go map (`Entry.Dir map[string]*Entry`)
and returns them alphabetically, which is fine for lookup and analysis. Cambium
preserves **schema declaration order** instead, because its target use cases —
order-correct, NETCONF-facing serialization and faithful typed-struct codegen —
depend on that order. The difference is one of design targets, not of one library
being right and the other wrong.

If you are writing new code rather than porting, the native `cambium` ordered IR
(see [./schema-introspection.md](./schema-introspection.md)) is the primary
surface; `compat` is the bridge for code that already targets goyang's `Entry`.

## The loader: side-by-side

The `Modules` loader mirrors goyang's: construct it, add search paths, read or
parse modules, process, then fetch the `Entry`.

goyang (`github.com/openconfig/goyang/pkg/yang`):

```go
ms := yang.NewModules()
ms.AddPath("yang", "yang/deps")
if err := ms.Read("my-module"); err != nil {
    return err
}
if errs := ms.Process(); len(errs) > 0 {
    return errs[0]
}
entry := yang.ToEntry(ms.Modules["my-module"])
```

compat (`github.com/signalbreak-labs/cambium/go/compat`):

```go
ms := compat.NewModules()
ms.AddPath("yang", "yang/deps")
if err := ms.Read("my-module"); err != nil {
    return err
}
if errs := ms.Process(); len(errs) > 0 {
    return errs[0]
}
entry, errs := ms.GetModule("my-module")
if len(errs) > 0 {
    return errs[0]
}
```

The method set on `*compat.Modules` is `AddPath(paths ...string)`,
`Read(name string) error`, `Parse(data, name string) error`,
`Process() []error`, `GetModule(name string) (*Entry, []error)`,
`FindModule(n Node) *Module`, and
`FindModuleByNamespace(ns string) (*Module, error)`. The exported fields
`Modules`, `SubModules`, `ParseOptions`, and `Path` are present as well, mirroring
goyang's.

Two package-level conveniences round out the compat entry points:

- `compat.GetModule(name string, sources ...string) (*Entry, []error)` constructs
  the loader, reads each `source`, and returns the projected `Entry` for `name`.
- `compat.Parse(input, path string) ([]*Statement, error)` parses raw YANG text
  with Cambium's native parser, then returns the upstream-shaped `[]*Statement`
  AST, matching goyang's `yang.Parse` shape. Prefer `cambium.ParseStatements`
  unless you specifically need the goyang-shaped statement type.

## Traversal: the one behavioral difference

This is the single difference to account for during migration. Both libraries
expose a node's effective children two ways:

- `Entry.Dir map[string]*Entry` — a name-keyed map, present in both libraries.
- `Entry.Children() []*Entry` — an ordered slice.

In goyang, ordered iteration is commonly done over `Entry.Dir`, which yields
children in alphabetical (map-key) order. In `compat`, `Entry.Children()` returns
children in **effective schema declaration order** — invariant
[I2](../../spec/ordering-invariants.md) — because Cambium's target use cases
(NETCONF-facing serialization, typed-struct codegen) depend on that order.
`Entry.Dir` remains available in `compat` as a name-lookup cache, but it is
intentionally not the ordering contract. This mirrors Cambium's central rule that
order is a structural property of the tree, never map iteration; see
[../concepts/ordering.md](../concepts/ordering.md).

The mechanical migration step: where your goyang code iterates `Entry.Dir` to walk
children in order, iterate `Entry.Children()` instead.

goyang (alphabetical, via the map):

```go
// Ordered by map key (alphabetical).
for name, child := range entry.Dir {
    visit(name, child)
}
```

compat (schema declaration order, via `Children()`):

```go
// Schema declaration order (I2).
for _, child := range entry.Children() {
    visit(child.Name, child)
}
```

For point lookups by name, `Entry.Dir`, `Entry.Lookup(name)`, and
`Entry.Find(path)` all work as in goyang and carry no ordering implication.
Aggregated parse/build errors are read via `Entry.GetErrors()`, which returns the
errors on a node and its descendants in deterministic order.

## Worked example: porting an ordered walk

Putting the loader and the traversal change together, here is a complete,
self-contained port. It loads a module the familiar way and walks a container's
children in order. The only thing that differs from the equivalent goyang code is
the ordered walk: `range top.Children()` instead of `range top.Dir`. For a `top`
container declaring leaves `z`, `m`, `a`, goyang's map iteration prints `a, m, z`;
`compat` prints `z, m, a` — schema declaration order.

```go
// src holds: module order-demo { ... container top { leaf z; leaf m; leaf a; } }
ms := compat.NewModules()
if err := ms.Parse(src, "order-demo"); err != nil {
    panic(err)
}
if errs := ms.Process(); len(errs) > 0 {
    panic(errs[0])
}
entry, errs := ms.GetModule("order-demo")
if len(errs) > 0 {
    panic(errs[0])
}

top := entry.Dir["top"] // name lookup on Dir is fine
for _, child := range top.Children() {
    fmt.Println(child.Name) // z, m, a — declaration order, not Dir's a, m, z
}
```

This program runs as a doc-test in `go/compat/example_test.go`, so it stays in sync
with the API.

## Beyond the projection: when to use the native tier

`compat` is deliberately a focused, goyang-shaped *read* surface. Some metadata a
goyang user reaches for is exposed more directly — or only — on the native
`cambium` handles, and for those it is cleaner to use the native tier
([schema introspection](./schema-introspection.md)) than to navigate the
projection:

- **Module namespace and prefix resolution** — `Module.Namespace()` and
  `Module.ResolvePrefix(prefix)` on the native handle.
- **Resolved leaf types and the typedef chain** — `SchemaNodeRef.LeafType()`
  returns a `TypeInfo` with `TypedefChain()`, rather than walking `YangType` by hand.
- **RPC / action / notification I/O in order** — `Module.RPCs()`, `Actions()`,
  `Notifications()`, and a node's `Input()` / `Output()`, all in schema order (I4).
- **Identity closure** — `Identity.Derived()` for the transitive derived set.

A common pattern is to keep loading and walking through `compat` for existing
goyang code, and reach into a parallel native `Context` (or `FromModule(...)`) for
the richer metadata.

## What is mirrored

`compat` projects a focused, read-only subset of goyang's `pkg/yang`. The shapes
you are most likely to depend on:

- **Loader:** `Modules`, `NewModules`, and the `AddPath` / `Read` / `Parse` /
  `Process` / `GetModule` / `FindModule` / `FindModuleByNamespace` methods, plus
  the package-level `GetModule` and `Parse`.
- **Entry construction:** `ToEntry(n Node) *Entry` projects a goyang-style AST
  node; `FromModule(module cambium.Module) *Entry` projects a Cambium module handle
  directly, which is the bridge from the native IR into the goyang-shaped view.
- **`Entry`:** the familiar fields (`Name`, `Kind`, `Dir`, `Type *YangType`,
  `ListAttr`, `RPC`, `Identities`, `Augments`, `Config`, `Mandatory`, `Default`,
  `Prefix`, `Uses`, and others) and methods (`Children()` for ordered traversal,
  plus `Lookup`, `Find`, and `GetErrors`).
- **Type model:** `YangType` and its supporting types.
- **AST nodes:** `Node` and the common statement node types (`Container`, `Leaf`,
  `List`, `Grouping`, `Uses`, `RPC`, `Action`, `Notification`, and others) are
  Cambium-owned, goyang-shaped types. AST-shaped read code carries over, but raw
  goyang AST values should be loaded through `compat` instead of passed in directly.
- **Helpers:** `CamelCase`, matching goyang's identifier conversion.

This is a summary, not the contract. The complete, current set of fields, methods,
and aliases lives in the
[godoc](https://pkg.go.dev/github.com/signalbreak-labs/cambium/go/compat) — confirm
a symbol there before relying on it.

## What is not mirrored

- **The projection is read-only.** `compat` is a view into a Cambium-built schema.
  It does not provide goyang's mutation, deviation-authoring, or tree-building APIs
  as a way to construct or edit schema. To build a schema, use the `cambium`
  package; `compat` then projects it (via `FromModule`, or by loading through
  `Modules`).
- **`Entry.Dir` is not the ordering source.** As above, map iteration is not part
  of the ordering contract; use `Entry.Children()` for order-sensitive walks.
- **No data tier.** `compat` is schema-only. Parsing, validating, and serializing
  instance data is out of this package's scope; it belongs to the separate data
  tiers (see the glossary and overview for the tier split).
- **No NETCONF / Terraform / gNMI surface.** Those are out of scope for Cambium as
  a whole; `compat` does not add them.

## Practical migration path

1. Swap the import: `github.com/openconfig/goyang/pkg/yang` →
   `github.com/signalbreak-labs/cambium/go/compat`, and the `yang.` qualifier →
   `compat.`.
2. Adjust `GetModule` call sites for the `(*Entry, []error)` return shape.
3. Replace any `range entry.Dir` used for ordered traversal with
   `range entry.Children()`. Leave name lookups on `Dir` / `Lookup` / `Find`
   as-is.
4. Drop any code paths that mutated the `Entry` tree; build schema via `cambium`
   instead and project with `FromModule`.
5. Build the changed packages with `CGO_ENABLED=0` to confirm the pure-Go tier is
   intact.

When in doubt about a symbol, confirm it against the godoc or the source before
relying on it:

```bash
cd go && go doc ./compat
cd go && go doc ./compat Entry
cd go && go doc ./compat Modules
```

## See also

- [../overview.md](../overview.md) — the problem, the design rule, and the three tiers
- [../concepts/ordering.md](../concepts/ordering.md) — why order is a structural property of the tree
- [./schema-introspection.md](./schema-introspection.md) — the native `cambium` ordered IR
- [./quickstart.md](./quickstart.md) — the shortest load-walk-codegen path
- [../glossary.md](../glossary.md) — terms, including the tier split
- [../../spec/ordering-invariants.md](../../spec/ordering-invariants.md) — normative I1–I6 wording
- [../README.md](../README.md) — documentation index
