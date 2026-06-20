# Migrating from goyang with package `compat`

This guide covers package `compat`, the cgo-free, goyang-shaped projection in the
**Schema-IR tier (pure Go, `CGO_ENABLED=0`)**. `compat` mirrors the surface of
goyang's `pkg/yang` — the `Entry` tree, the `Modules` loader, `ToEntry`,
`FromModule`, `GetModule` — so code written against goyang can move with minimal
changes. The projection is **read-only**: it is a view into Cambium's ordered
schema, not a mutable builder. There is exactly one behavioral difference to plan
for, stated neutrally below: ordered traversal goes through `Entry.Children()`
(schema declaration order) rather than iterating the `Entry.Dir` map.

## What `compat` is for

`compat` exists so you can keep familiar goyang call shapes while reading a
Cambium-built schema tree. It targets the migration use case: porting existing
analysis, lookup, and walk code that already speaks `*yang.Entry`. The package
sits alongside `cambium` and `codegen` in the pure-Go tier and imports no cgo or
libyang code.

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
`Process() []error`, `GetModule(name string) (*Entry, []error)`, and
`FindModuleByNamespace(ns string) (*Module, error)`. The exported fields
`Modules`, `SubModules`, `ParseOptions`, and `Path` are present as well.

`compat.GetModule(name string, sources ...string) (*Entry, []error)` is also
available as a package-level convenience that constructs the loader for you.

## Traversal: the one behavioral difference

This is the single difference to account for during migration. Both libraries
expose a node's effective children two ways:

- `Entry.Dir map[string]*Entry` — a name-keyed map, present in both libraries.
- `Entry.Children()` — an ordered slice.

In goyang, ordered iteration is commonly done over `Entry.Dir`, which yields
children in alphabetical (map-key) order. In `compat`, `Entry.Children()` returns
children in **effective schema declaration order** — invariant I2 — because
Cambium's target use cases (NETCONF-facing serialization, typed-struct codegen)
depend on that order. `Entry.Dir` remains available in `compat` for familiar
name lookup, but it is intentionally not the ordering contract.

The mechanical migration step: where your goyang code iterates `Entry.Dir` to
walk children in order, iterate `Entry.Children()` instead.

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

## What is mirrored

The following goyang `pkg/yang` surface is present in `compat`:

- **Loader:** `Modules`, `NewModules`, and the `AddPath` / `Read` / `Parse` /
  `Process` / `GetModule` / `FindModuleByNamespace` methods.
- **Entry construction:** `ToEntry(n Node) *Entry` projects a goyang-style AST
  node; `FromModule(module cambium.Module) *Entry` projects a Cambium module
  handle; `GetModule(name string, sources ...string) (*Entry, []error)`.
- **`Entry` fields:** `Name`, `Kind`, `Dir`, `Type *YangType`, `ListAttr`,
  `RPC`, `Identities`, `Augments`, `Config`, `Mandatory`, `Default`, `Prefix`,
  `Uses`, and others (run `go doc ./compat Entry` for the full list).
- **`Entry` methods:** `Children()` (ordered), `Lookup`, `Find`, `Path`,
  `Namespace`, `ReadOnly`, and the predicates `IsLeaf`, `IsList`, `IsContainer`,
  `IsDir`, `IsLeafList`, `IsCase`, `IsChoice`.
- **Type model:** `YangType`, `EnumType`, `YangRange`, `Number`, `TypeKind`,
  `EntryKind`, `TriState`.
- **AST nodes:** `Node` and the statement node types (`Container`, `Leaf`,
  `List`, `Grouping`, `Uses`, `RPC`, `Action`, `Notification`, and others) are
  type aliases to the internal upstream parser, so AST-shaped code carries over.
  (`Module` is a concrete struct rather than an alias.)

## What is not mirrored

- **The projection is read-only.** `compat` is a view into a Cambium-built
  schema. It does not provide goyang's mutation, deviation-authoring, or
  tree-building APIs as a way to construct or edit schema. To build a schema,
  use the `cambium` package; `compat` then projects it.
- **`Entry.Dir` is not the ordering source.** As above, map iteration is not part
  of the ordering contract; use `Entry.Children()` for order-sensitive walks.
- **No data tier.** `compat` is schema-only. Parsing, validating, and serializing
  instance data (RFC 7950 / RFC 7951) live in the optional **Backend/data tier
  (optional, requires cgo)** — see [./libyang-backend.md](./libyang-backend.md).
- **No NETCONF/Terraform/gNMI surface.** Those are out of scope for Cambium as a
  whole; `compat` does not add them.

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
5. Build the changed packages with `CGO_ENABLED=0` to confirm the pure-Go tier
   is intact.

When in doubt about a symbol, confirm it against the source before relying on it:

```bash
cd go && go doc ./compat
cd go && go doc ./compat Entry
cd go && go doc ./compat Modules
```

## See also

- [../README.md](../README.md) — documentation index
- [./schema-introspection.md](./schema-introspection.md) — the native `cambium` ordered IR
- [./codegen.md](./codegen.md) — typed-struct generation
- [./libyang-backend.md](./libyang-backend.md) — the optional data tier
- [../why-cambium.md](../why-cambium.md) — the order-semantics rationale
- [../../spec/ordering-invariants.md](../../spec/ordering-invariants.md) — normative I1–I6 wording
