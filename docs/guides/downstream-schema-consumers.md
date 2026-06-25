# Downstream schema consumers

Use the native `cambium` package for new schema consumers and external
generators. `compat` is a migration bridge for goyang-shaped code; it is not the
recommended API for new renderers.

The native surface is target-neutral. It has no Terraform, HCL, provider, NETCONF
transport, or renderer-specific concepts.

## Versioned schema IR

`ctx.SchemaIR()` returns a `cambium.SchemaIR` value tagged with
`cambium.SchemaIRVersion`. The projection contains loaded modules in context
order, including import-only modules, and marks each module with `Implemented`.
Nested `SchemaIRNode` values are in effective schema declaration order.

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
their parent module, skipped feature-disabled augment targets, mandatory config
augments, config false mandatory typedef defaults, and unambiguous local-name
path fallbacks. Duplicate `Module.Revisions()` entries are preserved in
declaration order.

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
Successful resolutions include a trace of each hop.

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
