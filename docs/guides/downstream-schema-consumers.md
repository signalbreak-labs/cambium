# Downstream schema consumers

Use the native `cambium` package for new schema consumers and external
generators. `compat` is a migration bridge for goyang-shaped code; it is not the
recommended API for new renderers.

The native surface is target-neutral. It has no Terraform, HCL, provider, NETCONF
transport, or renderer-specific concepts.

## Requirements and packaging

The schema surface (`cambium`, `compat`, `codegen`, `cmd/cambium-ir`) is pure Go
and builds with `CGO_ENABLED=0`; `scripts/check-go-default-pure.sh` enforces that
its import closure contains no cgo or libyang package. Import
`github.com/signalbreak-labs/cambium/go/cambium` and pin a released `go/v*` tag.
The optional libyang backend is only needed for RFC 7950 data validation.

## Integration recipe

`Example_schemaConsumer` in `go/cambium/example_consumer_test.go` is the
compiled, output-checked recipe. It sets every loading policy explicitly,
checks completeness, walks config-only list entries with keys first, and reads
qualified identity, defaults with their origin, and resolved types.

Caller responsibilities:

- Set every policy explicitly: `ContextFlags.DisableSearchdirCwd`, ordered
  `SearchPath` calls, root modules (with revisions when it matters), features
  per module, `SetDeviationPolicy`, and `SetValidationMode`. Defaults are
  strict validation, no features, and every loaded deviation applied.
- Treat a `Build` error as no schema. `DiagnosticFromError(err)` gives the
  kind, rule code, and source location.
- Treat a non-empty `LoadReport().OmittedContent()` as an incomplete schema.
  Other `LoadReport.Warnings` do not drop declared content.
- Check `LoadReport.RequestedModules` when revisions matter: a context may
  implement more than one explicitly requested revision of a module.
- Filter state explicitly. No traversal profile drops `config false` nodes;
  use `SchemaChildren.ConfigOnly()` or `ProjectionOptions.ConfigOnly`.
- Key on qualified identity (`QualifiedName`, `QualifiedPath`,
  `NamespaceQualifiedPath`, `LookupQualified`), not local names. `Lookup` by
  local name returns the first match in schema order.
- A built `Context` is frozen and safe for concurrent reads. Read handles only
  before `Close`, and keep your own annotations in your own maps keyed by
  qualified path.

`Build` rejects leafref cycles, including cycles through leafref union
members, with a diagnostic that lists the path chain. Vendor-compatible mode
reports each cycle as a `LoadReport` warning instead, and `ResolveLeafrefChain`
still reports a cycle at query time. For range and length restrictions,
`RangeBound.Min()`/`Max()` keep the lexical form; use `MinNumber()`/`MaxNumber()`
(and `MinLength()`/`MaxLength()` for lengths) for the numeric bounds with
`min`/`max` resolved against the type being restricted.

Known limit: `must` / `when` constraints carry no source location.

## Versioned schema IR

`ctx.SchemaIR()` returns a `cambium.SchemaIR` value tagged with
`cambium.SchemaIRVersion`. The projection contains loaded modules in context
order, including import-only modules, and marks each module with `Implemented`.
Nested `SchemaIRNode` values are in effective schema declaration order. If schema
materialization fails during a dirty rebuild, `SchemaIR.Errors` carries
structured diagnostics instead of silently dropping the failure.

The current version string is `cambium.schema-ir.v1` (defined in
`go/cambium/schema_ir.go` and emitted by `cmd/cambium-ir`). For v1, consumers can
depend on the presence and meaning of the documented top-level fields:
`version`, `modules`, and optional `errors`; module identity/import/include
fields; node path/name/kind/order fields; type/default/config/constraint fields;
source location; and provenance. Future v1 releases may add fields to objects or
new enum/string values where the existing meaning is unchanged. Removing or
renaming fields, changing path semantics, or changing ordering semantics requires
a new version string.

Each node carries:

- local, module-qualified, and namespace-expanded paths (`LocalPath`,
  `QualifiedPath`, `NamespaceQualifiedPath`);
- structural children (`Children`) preserving `choice`/`case`;
- flattened data children (`DataChildren`);
- list keys in key-statement order (`ListKeys`, `KeyNames`);
- kind, type metadata, defaults, config/read-only state, constraints;
- structured source location and provenance;
- deviation provenance.

The projection is a value snapshot over Cambium's ordered handles. Ordering still
comes from Cambium's ordered IR slices, never from map iteration.

v1 nests full copies of each node's `Children`, `DataChildren`, and `ListKeys`
subtrees, so its size can grow exponentially with schema depth (a chain of
nested containers doubles per level). `ctx.SchemaIRStats()` measures that size
without building it, and `ctx.SchemaIRWithLimit(max)` returns a
`resource_limit` diagnostic instead of materializing an oversized projection.

### Bounded node table (`cambium.schema-ir.v2`)

`ctx.SchemaIRTable()` returns the same facts as a node table
([ADR 0006](../adr/0006-bounded-schemair-table.md)). Every schema node appears
once in `Nodes`, in pre-order of the structural walk over modules in context
load order; `ID` is its index and `Parent` is its structural parent
(`SchemaIRNoParent` at module top level). `Children`, `DataChildren`, and
`ListKeys` are ordered node ids with the same ordering as v1, so record count
equals unique nodes for any depth. Prefer v2 for deep or large schemas.

### What the JSON export does and does not carry

Both JSON versions are deliberately narrow. They carry node names, kinds,
paths, the base type name, range and length bounds, defaults and `must`/`when`
as expression strings, config state, source location, provenance, and
deviation records without description or source location. Range and length
bounds appear as `type.range` (integer and decimal64 types) or `type.length`
(string and binary types): ordered `{"min", "max"}` segments with `min`/`max`
resolved, as canonical decimal strings so 64-bit and decimal64 values keep full
precision. Typedef chains, patterns, union members, enum and bit values,
presence, ordering, cardinality, description, units, status, extensions,
constraint error metadata, and XPath prefix context are available only through
the native Go handles (`SchemaIRNode.Ref`, `SchemaIRTableNode.Ref`). A consumer
that needs those facts should use the Go API rather than the JSON export.

For command-line consumers, `cmd/cambium-ir` exports the pure-Go SchemaIR as JSON
without importing the cgo backend. Its output is the JSON form of the same
versioned projection, not a separate schema:

```bash
cd go
CGO_ENABLED=0 go run ./cmd/cambium-ir -search ../conformance/fixtures/scrambled-children/module order-demo
CGO_ENABLED=0 go run ./cmd/cambium-ir -format v2 -search ../conformance/fixtures/scrambled-children/module order-demo
```

The default output is v1, bounded by `-max-records` (default 1,048,576); an
oversized v1 document exits 1 with a `resource_limit` diagnostic. `-format v2`
emits the node table.

For handle-oriented code, use `Context.Schema`, `Module`, `SchemaNodeRef`, and
`SchemaChildren` directly. `SchemaNodeRef.LocalPath()` returns a local module-root
path, while `Path()` preserves the existing goyang-shaped path that begins with
the module name. `SchemaNodeRef.NamespaceQualifiedPath()` returns expanded-name
path segments such as `/{urn:example}top/{urn:vendor}state`, which stays
unambiguous even when namespaces contain colons.

## Traversal profiles

`SchemaNodeRef.Traverse(profile)` and `Module.Traverse(profile)` expose named
schema traversal profiles:

- `TraversalStructuralChildren` preserves structural `choice` and `case` nodes.
- `TraversalDataChildren` flattens `choice`/`case` to payload data nodes.
- `TraversalSerializationOrder` returns the direct serializer shape, including
  keys first for list entries.
- `TraversalSchemaDeclarationOrder` preserves effective schema declaration order.
- `TraversalListEntryOrder` returns data children with list keys first in
  key-statement order.

These profiles are named in YANG/schema terms and are independent of any target
generator.

## Provenance and diagnostics

`Module.SourceLocation()`, `SchemaNodeRef.SourceLocation()`, and
`Identity.SourceLocation()` expose structured file, line, and column components
alongside the established human-readable location string.

`SchemaIRNode.Provenance` reports defining module, instantiating module,
augmenting module when Cambium can identify it, grouping origin, and deviations
applied to the node. Cambium does not invent provenance it has not tracked; absent
fields mean the detail is not known for that node.

Errors remain inspectable with `errors.As` against `*cambium.Error` and more
specific causes such as `*cambium.SchemaPathError`,
`*cambium.LeafrefResolutionError`, or `*cambium.DiagnosticError`.
`cambium.DiagnosticFromError(err)` converts an error into a structured diagnostic
with a stable rule code and category such as invalid identifier, missing module,
unresolved path, invalid deviation, semantic schema error, unsupported construct,
or syntax error. When Cambium has structured secondary source statements, such as
a previous duplicate definition, `Diagnostic.Related` carries those locations.

## Load reports

`ctx.LoadReport()` returns observability data for a built context:

- explicitly requested modules;
- loaded transitive imports;
- included submodules;
- deviation modules;
- enabled and disabled declared features;
- participating source files;
- diagnostics/warnings when available.

This is a reporting API only; it does not change module loading, validation, or
feature semantics. The default schema loader is strict. If a builder opts into
`cambium.ValidationVendorCompatible`, selected vendor compatibility relaxations
are reported here as warnings while the schema still loads. This includes
duplicate or out-of-order revisions, direct submodule entrypoints resolved to
their parent module, augment and deviation targets excluded by the enabled
feature set, mandatory config augments, config false mandatory typedef defaults,
unambiguous local-name path fallbacks, and leafref cycles. Duplicate `Module.Revisions()`
entries are preserved in declaration order.

An augment or deviation target that does not resolve fails in every mode unless
the path stops at a node the enabled feature set excluded (by its own
`if-feature`, one a `refine` added, an enclosing `uses`, or a disabled `augment`
that declares it); a typo, a wrong prefix, or a missing dependency is never relaxed. Strict mode rejects the feature-excluded case too,
naming the excluded node. Vendor mode skips it with a warning, and a skipped
augment's warning has kind `omitted_schema_content` so
`LoadReport.OmittedContent()` lists every relaxation that dropped declared
content.

`ContextBuilder.SetDeviationPolicy(DeviationPolicy{IgnoreNotSupported: true})`
keeps nodes targeted by `deviate not-supported` while still applying other
deviations, decided before references are validated.
`LoadReport.IgnoredDeviations` lists what the policy kept, and each
`Deviation` reports `Applied()` and `SourceLocation()`. To apply none of a
deviation module's effects, do not load it.

## Schema diffs

`cambium.DiffModules(oldModule, newModule)` and
`cambium.DiffContexts(oldCtx, newCtx)` compare loaded schema models and return a
`cambium.SchemaDiff` tagged with `cambium.SchemaDiffVersion`.

Diff changes are generic schema facts:

- added and removed nodes;
- node kind, type, list key, default, config/read-only, and constraint changes;
- augment provenance changes;
- deviation provenance/effect changes.

Each `SchemaDiffChange` includes a local path, module-qualified path,
namespace-expanded path where a node reference is available, old/new
`SchemaNodeRef` handles when present, and old/new value summaries. Ordering is
deterministic and produced by walking Cambium's ordered schema IR; maps are used
only as lookup indexes. Same-local augmented siblings are matched using qualified
identity so a change to one augmenting module's `state` leaf is not confused with
another module's same-local-name sibling.

## Leafref and identity helpers

Use `cambium.ResolveLeafref(node)` for a single leafref hop and
`cambium.ResolveLeafrefChain(node)` to follow a chain to its terminal target.
Failures return `*cambium.LeafrefResolutionError`, including a structured reason.
Successful resolutions include a trace of each hop. Leafref paths are data
paths: `..` moves to the data parent and steps look through `choice` and
`case`, so a leafref into a case's leaf resolves.

For identities, `Module.Identity(name)` returns a resolved identity handle and
`Identity.DerivedClosure()` returns the transitive derived set in Cambium's
deterministic schema resolution order.

## Codegen planning

`codegen.Plan(ctx, module)` returns a `*codegen.ModulePlan` tagged with
`codegen.PlanVersion`. The plan exposes ordered records, fields, types,
identities, serializer field order, and validation metadata before rendering Go.

The current Go package still computes Go type names because the only shipping
renderer is the Go emitter, but the plan also carries native `cambium` schema
handles and target-neutral type/validation metadata so other renderers can build
from the same ordered model without using generated Go.
