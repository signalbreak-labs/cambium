# Cambium Pure-Go Rebuild - Implementation Spec

> Status: implemented v0.4
> Branch: `feat/pure-go-rebuild-spec`
> Scope: ground-up Go rebuild plan for a pure-Go, order-correct Cambium schema/codegen library.
> Gate: do not start implementation until this document, `AGENTS.md`, and `/spec` agree.

## 1. Decision

Cambium's default Go library must be pure Go.

The current Go implementation proves useful API and conformance ideas, but it is
not the product users expect from a "better goyang": it requires cgo because it
is a Go facade over libyang. That tax is too high for broad Go adoption,
cross-compilation, static binaries, restricted build systems, and projects that
ban cgo.

Product lines after the rebuild:

- `go/cambium`: pure-Go schema loading, introspection, order-correct schema IR,
  and codegen support.
- `go/compat`: cgo-free, read-only goyang-shaped migration projection over
  Cambium's ordered schema IR.
- optional libyang backend: separate package or module, cgo-gated, used for full
  validation, data-tree serialization, and backend oracle work.
- shared `/spec` and `/conformance`: tiered contract that separates pure-Go
  schema IR guarantees from optional backend data/serialization guarantees.

This is not a rejection of libyang. It is a boundary correction: libyang remains
the Rust engine and an optional Go backend, not the default Go dependency.

### Implemented decisions

- **Gate A result:** Cambium's internal `yangparse.Statement` parser adapter
  preserves lexical substatement order through `SubStatements()` before any
  goyang-style `ToEntry` projection. The pure-Go builder reads schema children
  only from that ordered raw statement stream. The adapter rejects oversized or
  excessively nested YANG source before upstream recursion, rejects invalid
  UTF-8 and illegal RFC 7950 source characters before syntax parsing, and
  converts any upstream parser panic into a schema-loading error.
- **Unsafe goyang surfaces:** `Entry.Dir` and goyang's typed AST child slices
  (`Leaf`, `Container`, `LeafList`, `List`, `Choice`, etc.) are lookup or
  kind-separated conveniences, not Cambium traversal authorities. The default
  package has a static test guarding against their use for ordered traversal.
- **Import/fork decision:** Cambium vendors a narrow parser/AST fork from
  `github.com/openconfig/goyang v1.6.3` under
  `go/internal/yangparse/upstream`, with Apache-2.0 attribution, upstream tag
  SHA, and a patch log in `UPSTREAM.md`. The default `go/cambium`,
  `go/codegen`, and `go/compat` dependency closure contains no
  `github.com/openconfig/goyang` packages.
- **Backend boundary:** the cgo/libyang implementation lives under
  `github.com/signalbreak-labs/cambium/go/libyangbackend` with cgo build tags.
  The default `go/cambium`, `go/codegen`, and `go/compat` dependency closure
  does not include `internal/libyang`, `libyangbackend`, `runtime/cgo`, or
  `import "C"`.
- **Unsupported in pure-Go default:** data parsing, generic data-tree
  validation, diff/merge, and generic data serialization remain
  Backend/data-tier behavior. They are not faked by `//go:build !cgo` stubs in
  `go/cambium`; users opt into `go/libyangbackend` when they need
  libyang-grade data behavior. Generated typed structs may still expose narrow
  cgo-free structural validation for constraints expressible from typed fields.

## 2. Relationship To Rust And `/spec`

This rebuild is a Go-only default-track change.

- Rust remains libyang-backed until a separate pure-Rust parser/validator effort
  is specified.
- Go pure-Go default participates in the Schema IR tier only: ordered schema
  properties, type/constraint metadata, module/import/prefix APIs, and codegen
  input order.
- Rust/Go byte-for-byte XML/JSON/gNMI parity is not a pure-Go default guarantee.
  It applies only to Backend/data-tier runs where both sides use comparable
  serialization backends.
- Pure-Go and Rust/backend schema parity is property-level: ordered lists of
  schema nodes, keys, type data, defaults, imports, identity closure, and other
  schema IR facts.

The normative amendments for this decision live in:

- `AGENTS.md`: default Go build is pure Go; optional libyang backend is separate.
- `spec/ordering-invariants.md`: invariants are tiered into Schema IR and
  Backend/data guarantees.
- `spec/api.md`: public API contract is tiered; byte parity is backend-only.

`CLAUDE.md` is a symlink to `AGENTS.md`. Edit `AGENTS.md`, never `CLAUDE.md`
directly.

## 3. Goals

1. Provide a pure-Go successor to `openconfig/goyang` for schema parsing,
   linking, introspection, and codegen input.
2. Preserve YANG declaration order structurally, not by sorting, sidecars, or
   map iteration.
3. Reuse goyang source where it is useful and legally allowed, with Apache-2.0
   notices, attribution, and a clear import/fork decision.
4. Avoid goyang's `Entry.Dir map[string]*Entry` ordering failure by never making
   a map the authoritative child representation.
5. Avoid goyang's typed-slice ordering trap by building Cambium IR from an
   ordered statement stream, not from kind-separated typed slices.
6. Keep the public API free of cgo, `unsafe`, libyang types, and current
   `internal/libyang` concepts.
7. Keep validation claims honest: full RFC validation, XPath `must`/`when`,
   mandatory instance checks, and leafref instance checks remain optional-backend
   territory indefinitely unless a dedicated pure-Go validation engine is
   designed and tested.

## 4. Non-Goals

- No NETCONF client, transport, edit-config builder, gNMI session, Terraform
  provider generator, or device fake.
- No default cgo dependency in `go/cambium`, `go/codegen`, or `go/compat`.
- No goyang-compatible `Entry` clone as the primary API.
- No claim of full libyang-equivalent validation in the pure-Go default.
- No hand-rolled YANG source parser until the goyang parser/AST is proven unable
  to satisfy the ordering contract.
- No source-order sidecar that can disagree with the tree. Order must live in
  the tree structure.
- No hybrid public method that silently uses a pure-Go facade for some calls and
  a cgo/libyang fallback for others.

The optional `go/compat` compatibility projection is read-only and clearly
marked as a migration aid. It must not be used internally for serialization,
codegen field order, or schema traversal. Its cgo-free `Modules` facade
provides goyang-style `NewModules`, `AddPath`, `Parse`, `Read`, `Process`,
`GetModule`, `FindModule`, and `FindModuleByNamespace` helpers over Cambium's
pure-Go builder. `Process` supports goyang-style in-memory `Parse` sets
containing modules and submodules by internally staging those parsed sources
for the builder's ordinary include/import search path; Cambium's public
direct-submodule load rejection remains unchanged. `FindModule` resolves goyang-style import/include AST nodes
and honors exact `revision-date` pins before falling back to bare latest module
lookup when the requested revision is absent. `FindModuleByNamespace` first
looks through parsed module records, matching goyang's pre-`Process` lookup
behavior, before asking Cambium's builder to materialize additional records.
`AddPath` accepts path-list strings, suppresses duplicate paths, and
supports `dir/...` recursive lookup with direct-directory filename precedence
before descending. `Modules.ParseOptions`, `Options`, `DeviateOptions`, and
`DeviateOpt` exist for goyang call-shape compatibility. Package-level
parser/AST helpers `Parse`, `Statement`, `Node`, `ErrorNode`, `Source`,
`Typedefer`, `RootNode`, `FindModuleByPrefix`, `MatchingExtensions`,
`MatchingEntryExtensions`, `FindGrouping`, `NodePath`, `FindNode`,
`ChildNode`, `PrintNode`, `PathsWithModules`, and `CamelCase` are exposed for
cgo-free goyang migration code; `Parse` uses Cambium's bounded parser wrapper rather than the raw
upstream parser boundary. `RootNode` and `FindModuleByPrefix` return the
compat `Module` facade while still accepting parser AST nodes, including
imported prefixes already linked by an upstream-style AST `Modules.Process()`
call. `ToEntry`
accepts compat module nodes and source-backed parser AST module/statement
nodes; `FromModule` accepts Cambium module handles.
Compat-owned `Module`, `Value`, and `Identity` nodes retain goyang-compatible
`yang` struct tags so reflection-based helpers such as `ChildNode`, `FindNode`,
`NodePath`, and `MatchingExtensions` traverse loader-backed module facades as
they do upstream AST nodes. `FindGrouping` resolves local, imported, and
included grouping definitions through the compat module facade instead of
depending on upstream-only `Import.Module` / `Include.Module` links.
Loader-backed compat modules return the full ordered Cambium projection;
AST-only conversion walks ordered `Statement.SubStatements()` and fails closed
when no ordered source statement is available rather than reconstructing order
from kind-separated typed AST slices. Non-conflicting parser AST node structs are exposed
as aliases; names that collide with the compat projection use `ASTValue`,
`ASTModule`, and `ASTIdentity`; `ASTModule` is for explicitly constructing raw
parser nodes, not the helper return type. `Value` carries goyang-style `Parent`,
`Source`, `Extensions`, and `Description` fields and implements the parser
`Node` shape. Projected `Identity` values carry goyang-style `Parent`,
`Source`, and `Extensions` fields and also implement the parser `Node` shape.
`Modules.ClearEntryCache()` clears compat-owned cached `Entry` projections so
repeated `ToEntry` calls match goyang's cache lifecycle without making the cache
a traversal authority.
Its `Entry.Dir` map is a lookup cache only; ordered traversal goes through `Entry.Children()`. `Entry.Find`
uses direct lookup for slash path traversal and must not depend on map
iteration. `Entry.Node` is exposed as a goyang-style parser `Node`; Cambium-specific
schema handles used by helpers such as `Namespace()`, `Path()`, and
`GetWhenXPath()` remain internal so migrated code can compile against the
expected field type without making parser nodes a traversal authority.
Common goyang-style read fields (`Description`, `Default`, `Units`,
`Config`, `Mandatory`, `Key`, `Type`, `Exts`, `ListAttr`, `RPC`,
`Augments`, `Augmented`, `Deviations`, `Deviate`, and `Uses`) are populated
from Cambium's effective schema IR where that metadata is available.
`MatchingEntryExtensions` resolves extension instances whose keywords use
either a source prefix or Cambium's projected defining module name, including
imported extension modules.
`UsesStmt`, `DeviatedEntry`, and the `Deviation*` constants are present for
goyang source-shape compatibility; these fields are metadata surfaces, not
ordered traversal sources. Module-root `Identities`, `Entry.GetErrors()`,
`Entry.Annotation`, `NewDefaultListAttr()`, compat `Module.Namespace`/
`Module.Prefix`, `Module.Contact`, `Module.Description`,
`Module.Organization`, `Module.Reference`, `Module.YangVersion`,
`Module.Revision`, `Module.Import`, `Module.Include`, top-level
parser-backed AST slices (`Module.Anydata`, `Module.Anyxml`,
`Module.Augment`, `Module.Choice`, `Module.Container`, `Module.Deviation`,
`Module.Extension`, `Module.Feature`, `Module.Leaf`, `Module.LeafList`,
`Module.List`, `Module.Notification`, `Module.RPC`, and `Module.Uses`),
`Module.Source`, `Module.Kind()`, `Module.ParentNode()`, `Module.NName()`,
`Module.Statement()`, `Module.Exts()`, `Module.Groupings()`,
`Module.Typedefs()`, parser-backed top-level `Module.Leaf` /
`Module.Anydata` / `Module.Anyxml` / `Module.Leaf` / `Module.LeafList` /
`Module.List` child metadata such as `Type`, `if-feature` and `must`
children, defaults, keys, cardinality, unique constraints, `must` statements, nested list leaf
types, and list-scoped action / notification children, plus parser-backed nested
`Container`, `Choice` / `Case`, `Augment`, `Notification`, `Grouping`,
`Typedef`, `Uses` / `Refine`, `Action`, and RPC `Input` / `Output` child
slices,
parser-backed `Module.Extension` / `Module.Feature` / `Module.Deviation`
/ `Module.Identity` definition metadata, `Deviation.Deviate` `type` / `must`
/ `unique` children, and submodule `BelongsTo.Prefix` metadata,
`Module.Identity`, `Module.Identities()`, `Module.Current()`,
`Module.FullName()`, `Module.GetPrefix()`, goyang-shaped `Module.Import` /
`Module.Include` nodes with parser `Statement()` / `ParentNode()` / `Source()`
metadata, goyang-shaped module scalar `Value` nodes (`Namespace`, `Prefix`,
`YangVersion`, `Organization`, `Contact`, `Description`, and `Reference`) with
parser `Statement()` / `ParentNode()` / `Source()` metadata, and
goyang-shaped `Module.Revision` nodes whose `Description` / `Reference` value
children point back to the revision, `Entry.Namespace()`, `Entry.Modules()`,
`Entry.Augment()`, `Entry.ApplyDeviate()`, `Entry.FixChoice()`, and
`Entry.InstantiatingModule()` provide goyang-style migration conveniences
without becoming traversal sources. Mutating migration conveniences preserve
Cambium's ordered child slice when they add, remove, or wrap entries; they must
not rely on `Entry.Dir` map iteration for result order.
`Entry.GetWhenXPath()` returns the first
effective `when` expression as a goyang-style migration convenience over
Cambium's richer `Whens()` metadata. `Entry.Print(w)` walks `Entry.Children()`
so debug/migration output preserves declaration order instead of reproducing
goyang's sorted `Entry.Dir` output. `TriState.String()` and
`EntryKind.String()` preserve goyang-compatible spellings for existing
migration logging and diagnostics. `BaseTypedefs` exposes the parser fork's
goyang-shaped manufactured typedefs for built-in YANG types. `YangType` carries
goyang-compatible `TypeKind` constants plus common read fields such as `Base`,
`Root`, defaults, units, decimal64 fraction digits, range/length restrictions,
string patterns, enum/bit name-value maps, identityref base identities, leafref
path/optional-instance, and union member types, all sourced from Cambium
`TypeInfo`. `YangType.Default`/`HasDefault` use type-level typedef defaults
like goyang; explicit schema node defaults stay on `Entry.Default` and
`Entry.DefaultValues()`. `TypeInfo.TypedefChain()` exposes typedef-derived
names from the outer referenced typedef to the innermost typedef before the
final built-in base, and compat `YangType.Base` projects that intermediate
chain in goyang-compatible parser type shape. `YangType.Root` reflects the
final built-in base known through `TypeInfo`. `YangType.Equal` is
name-insensitive like goyang, ignores `Base`/`Root` like goyang's current
comparison behavior, and compares the projected metadata Cambium exposes.
`Number`/`YangRange` expose goyang-style
constructors, parsers, validation/containment helpers, numeric constants, and
built-in integer ranges (`FromInt`, `FromUint`, `FromFloat`, `ParseInt`,
`ParseDecimal`, `ParseRangesInt`, `ParseRangesDecimal`, `Frac`, `Int8Range`
through `Uint64Range`) for migration code. `go/compat` also carries an
executable API parity guard against the vendored goyang v1.6.3 package that
checks exported top-level names, exported struct field names/types/tags,
non-struct type declarations, const/var declaration shape, public function
signatures, and public method sets under the default cgo-free test gate.
Companion behavioral differentials compare compat AST helper
behavior, module-loader lookup behavior, common `Entry` metadata, and
`YangType` projection semantics with
goyang while keeping Cambium-only
improvements such as declaration-order `Children()` traversal and richer units
metadata as intentional non-exact-match behavior.

## 5. Current State To Preserve

The current Go branch contains useful work:

- a public schema API shape that exposed many goyang-grade needs;
- red-first tests for paths, defaults, extensions, leafrefs, identity bases,
  imports, deviations, actions, notifications, grouping origin, and constraints;
- useful `/spec/api.md` additions;
- practical lessons about libyang's limits, especially that backend-specific
  compiled lists are not the public ordering contract.

Preserve those lessons, but do not keep the cgo implementation as the default Go
path.

## 6. Source Reuse Policy

The initial spike imported `openconfig/goyang` as an ordinary Go module to prove
its raw statement representation preserved order. The implemented default path
now vendors the narrow parser/AST surface under `go/internal/yangparse/upstream`
so Cambium can be used as a goyang replacement without a runtime dependency on
the upstream module. Future fork updates follow the policy below.

If code is copied or forked from goyang:

- keep Apache-2.0 license headers where present;
- keep copied upstream files under their own Apache-2.0 license context;
- mark Cambium-modified files as modified, per Apache-2.0 section 4(c);
- carry goyang's NOTICE content if upstream includes one;
- update repository NOTICE/LICENSE material to declare the mixed license before
  copied Apache-2.0 source lands;
- record the imported upstream commit SHA in `go/third_party/goyang/UPSTREAM.md`
  or an equivalent file;
- keep a small patch log explaining what Cambium changed and why;
- prefer narrow, reviewable parser/resolver copies over a wholesale dump.

Initial internal target if vendoring becomes necessary:

```text
go/internal/yangparse/     pure-Go parser/linker adapter
go/internal/yangparse/upstream/
                           copied goyang files or narrow fork
go/cambium/                public pure-Go facade and IR
go/codegen/                codegen over the pure-Go IR
```

Imported upstream code must not live directly in `go/cambium`; keep it behind an
internal adapter so the public API remains Cambium's.

## 7. Architecture

```text
                 default pure-Go path

  go/cambium public API
          |
          v
  ordered schema IR
          |
          v
  internal/yangparse adapter
          |
          v
  goyang raw statement parser or narrow fork


                 optional backend path

  go/libyangbackend or separate module
          |
          v
  cgo adapter over vendored libyang
```

The pure-Go path owns schema shape, declaration order, and codegen metadata. The
optional backend may be used for deep validation, RFC 7951/XML printing, and
differential tests, but the default package must compile and test with:

```bash
cd go
CGO_ENABLED=0 go test ./cambium ./codegen ./compat
```

## 8. Package Layout

Target layout after the rebuild:

```text
go/
  cambium/
    context.go          pure-Go module loading context
    module.go           Module handle
    schema.go           SchemaNodeRef, SchemaChildren, type info
    types.go            BaseType, ResolvedType, constraints
    identity.go         identity graph
    errors.go           stable public errors
  internal/
    yangparse/
      parser.go         adapter over goyang parser pieces
      linker.go         module/import/include resolution
      ir_builder.go     ordered AST -> ordered Cambium IR
      upstream/         copied goyang files only if needed
  codegen/
    ...
  libyangbackend/       optional package or separate module; cgo build tags
```

Names can change during implementation, but the boundary cannot: public package
and codegen imports must not include cgo, `internal/libyang`, or backend handles.

## 9. Core Data Model

The central type is an ordered schema IR.

```go
type Module struct {
    name       string
    namespace  string
    prefix     string
    revision   string
    imports    []Import
    includes   []Include
    top        []*SchemaNode
    nodesByPath map[string]*SchemaNode // lookup cache only
    identities []*Identity
}

type SchemaNode struct {
    name     string
    kind     SchemaNodeKind
    module   *Module
    parent   *SchemaNode
    children []*SchemaNode // authoritative order

    // lookup caches only; never iterate for traversal order
    childByName map[string][]*SchemaNode

    typ      *TypeInfo
    config   Config
    status   Status
    defaults []string
    musts    []MustConstraint
    whens    []WhenConstraint
    uniques  []UniqueConstraint
    keys     []*SchemaNode
}
```

Rules:

- `children` is the authority for every walk.
- maps are lookup accelerators only.
- duplicate local names from different modules must be representable.
- augmented children must keep owner module metadata.
- grouping-expanded nodes must keep enough provenance for `GroupingOrigin()` if
  the parser/resolver exposes it cleanly.

## 10. Parser Strategy

Split parser validation into two gates.

### Gate A - raw parse order spike

This is a short proof, not a resolver milestone.

1. Import or depend on the smallest goyang parser surface needed to parse one
   module.
2. Prove the raw statement representation preserves substatement declaration
   order before `ToEntry`.
3. Prove the typed convenience representation is not safe for Cambium IR if it
   separates child statements by kind.
4. Document exactly which goyang type is safe to read for ordered substatements.

Required fixture shape:

```yang
container root {
  leaf a { type string; }
  container b { leaf x { type string; } }
  leaf-list c { type string; }
  uses g;
  list d { key "name"; leaf name { type string; } }
  choice e { case one { leaf f { type string; } } }
}
```

The all-leaf `z, a, m` fixture is insufficient because it would pass even if the
builder used kind-separated typed slices. The proof must catch cross-kind
interleaving.

### Gate B - resolver feasibility

This is real implementation work, not a spike.

- includes/imports without `Entry.Dir`;
- grouping/uses expansion at the use site;
- refine handling;
- augment placement under Cambium's pure-Go ordering rule;
- deviation metadata;
- identity derived closure;
- leafref target resolution where possible.

If the raw statement stream does not preserve order, stop and document why. At
that point the options are:

- deeper fork of goyang's lexer/parser to retain ordered substatements;
- a new parser focused on schema order and codegen metadata;
- abandon pure-Go default as infeasible for the desired product.

## 11. Resolver Strategy

The resolver is split into explicit passes.

1. **Load pass**
   - parse module/submodule files;
   - record module name, namespace, prefix, revision;
   - record imports/includes in declaration order.

2. **Link pass**
   - resolve include statements to submodules by name/revision-date;
   - attach each included submodule to the exact parent module revision that
     issued the include;
   - resolve imports to modules by name/revision-date;
   - retain exact revision-keyed module residency internally, so same-name
     module revisions can coexist while the public `Schema(name)` lookup remains
     the latest/default convenience view;
   - build prefix tables per module and submodule.

3. **Definition pass**
   - collect typedefs, groupings, identities, extensions, features;
   - index scoped typedefs and groupings by lexical parent so valid sibling
     local scopes resolve independently instead of overwriting each other;
   - keep source declaration order for iterators;
   - detect duplicate definitions and shadowing collisions with stable errors.

4. **Expansion pass**
   - build direct children from ordered substatements;
   - expand `uses` at the exact use-site position;
   - apply `refine` statements;
   - apply `augment` statements using the ordering rule in section 12;
   - apply deviations where implemented, or record unsupported deviation details
     explicitly.

5. **Type pass**
   - resolve built-in types;
   - resolve typedef chains;
   - resolve identityref bases;
   - assign node `TypeInfo` across the full context before resolving leafrefs,
     so forward references, chained leafrefs, and cross-module targets can see
     target type metadata;
   - resolve leafref paths where possible, including absolute paths, relative
     parent segments, predicates containing `current()`, and simple leading
     `deref()` schema-target paths that can be resolved without instance data;
   - preserve instance-identifier `require-instance` and keep codegen metadata
     rich enough to render XML and JSON_IETF lexical forms separately;
   - retain unresolved leafrefs as structured unresolved values, not strings
     pretending to be resolved.

6. **Index pass**
   - build path indexes;
   - build child lookup maps;
   - build identity derived closure;
   - build tests that verify maps are never used for ordered iteration.

## 12. Ordering Contract For Pure Go

Pure-Go Cambium must satisfy the Schema IR tier of
`spec/ordering-invariants.md` without libyang.

Required from M1:

- direct children under modules, containers, lists, choices, and cases in source
  declaration order;
- list keys in key-statement order;
- module imports in declaration order;
- module prefix resolution, including empty prefix to self;
- extension instances and leaf-list defaults in declaration order where parsed.

Required from M3:

- grouping-expanded children at the `uses` statement position;
- `refine` applied without reordering unaffected siblings;
- augment children appended after the target's directly declared and already
  expanded children;
- multiple augments applied in module-load order, then augment statement source
  order, then child source order;
- deviations applied without reordering unaffected siblings; replacement keeps
  the original target position;
- RPC/action input/output and notification children in effective schema order.

This pure-Go augment placement is the oracle for pure-Go fixtures. It may differ
from libyang's compiled placement for edge cases; that is documented divergence,
not a pure-Go bug. Do not assert pure-Go IR equals libyang for augment placement
until the expected property is explicitly shared.

### Feature / if-feature filtering

The pure-Go schema IR applies an explicit feature policy:

- All features are disabled by default.
- `ContextFlags.DisableSearchdirCwd` disables implicit current-working-directory
  lookup. By default, pure-Go `LoadModule` matches goyang/libyang behavior and
  searches `.` before configured search paths.
- Public methods on a nil Go `*Context` do not panic; mutating methods and
  schema lookups return `CAMBIUM_E0001`, and snapshot lookups return empty
  results.
- `LoadModule(name)` accepts a YANG module identifier, not a filesystem path;
  malformed names fail with `CAMBIUM_E0001` before search-path lookup. Use
  `LoadModuleFromPath` for explicit file paths. Files discovered by module-name
  lookup or import resolution must declare the requested `module` name before
  they enter the context; submodule files load only through `include`
  resolution. Revision-suffixed filenames (`name@YYYY-MM-DD.yang`) are eligible
  for unpinned lookup only when the source's effective/latest declared revision
  matches that same revision.
  Explicit revision pins also fail closed when the corresponding revisioned
  candidate exists but has a different effective revision instead of falling
  back to `name.yang`. Failed module loads are transactional at the context boundary:
  dependency/import/include errors roll back newly visible modules and inline
  `ContextBuilder.LoadModule` feature mutations instead of leaving partial
  schema or feature state behind.
- `SetFeatures(module, features)` applies the same local identifier validation
  to the module name and each enabled feature name before mutating feature
  state.
- `ContextFlags.AllImplemented` marks implicitly imported modules as
  implemented, so `Context.Modules()` includes them. `ContextFlags.NoYangLibrary`
  is a no-op in the pure-Go default because no internal yang-library module is
  auto-loaded.
- Without `AllImplemented`, imported modules targeted by leafrefs, augments, or
  deviations are still promoted to implemented status. `ContextFlags.RefImplemented`
  additionally promotes imported modules referenced by explicit prefixes in
  `must`, `when`, and default values.
- `LoadModuleFromPath` registers the loaded file's directory before resolving
  imports/includes, so sibling modules and submodules load without a separate
  `SearchPath` call. It rejects valid direct submodule sources, as does
  `LoadModuleStr`; submodules must enter through the parent module's `include`
  chain after ordinary submodule metadata validation. If the path load fails,
  a directory added only for that load is rolled back instead of remaining as a
  future search path.
- Search paths ending in `...` recursively scan the parent directory using
  goyang-compatible module filename precedence. Each scanned directory applies
  direct lookup precedence before descending: `name.yang` first, then valid
  same-directory `name@revision.yang` candidates, then child directories.
- The internal parser adapter fails closed for oversized or excessively nested
  YANG source, rejects invalid UTF-8 and illegal RFC 7950 source characters
  before syntax parsing, and recovers upstream parser panics as ordinary load
  errors.
  File-based module loads check file size before reading full contents into
  memory.
- Module loads fail closed when required module metadata is absent or duplicated;
  `module` statements must declare one `namespace` and one `prefix`, and cannot
  declare direct `belongs-to`. Module and submodule `yang-version`, when present,
  must be a singleton `1` or `1.1` and has no standard non-extension children.
  Absent `yang-version` defaults to YANG 1.0; `anydata`, `action`,
  notifications nested under data nodes, direct nested `choice` shorthand
  cases, `must` statements directly under operation `input`, operation
  `output`, or `notification`, boolean `if-feature` expressions, and `pattern`
  `modifier` substatements require `yang-version 1.1`. `if-feature`
  statements under `enum`, `bit`, `identity`, or `refine`, multiple `base`
  statements on `identity` definitions or `identityref` type bodies, `empty` or
  `leafref` union member types, list key leaves with type `empty`, default
  statements on `leaf-list` nodes, identifiers starting with `xml` in any case,
  and cross-module augments that add mandatory config nodes. Such augments
  require a direct `when` statement in YANG 1.1. Explicit
  `require-instance` statements on `leafref` types also require
  `yang-version 1.1`.
  Module and submodule top-level `contact`, `organization`, `description`, and
  `reference` header metadata are singleton statements.
  Namespaces cannot be reused by different module names in one context.
  Conflicting duplicate module identity loads with the same name and revision
  fail closed instead of overwriting the prior module. Module and submodule
  `revision` statements must
  be valid `YYYY-MM-DD` calendar dates and cannot repeat in one source file;
  direct known non-extension children of `revision` are limited to
  `description` and `reference`.
  `import`/`include` `revision-date` pins must also be valid dates and cannot be
  repeated on one dependency statement. Direct known non-extension children of
  `import` are limited to `prefix`, `revision-date`, `description`, and
  `reference`; direct known non-extension children of `include` are limited to
  `revision-date`, `description`, and `reference`, and other known children fail
  before dependency lookup. Loaded submodules must have a
  single `belongs-to` parent with a single `prefix` child; direct known
  non-extension children of `belongs-to` are limited to `prefix`. Submodules
  cannot declare direct `namespace` or direct `prefix`.
- Scalar statements with no standard substatements, including `namespace`,
  `prefix`, `contact`, `organization`, `description`, `reference`, `default`,
  `status`, `units`, and related metadata/value statements, reject known
  non-extension children with `CAMBIUM_E0001`.
- Explicit revision strings passed to `LoadModule` fail closed before
  search-path lookup unless they are valid `YYYY-MM-DD` calendar dates.
- `SearchPaths()` returns a copy of the configured search path list, excluding
  implicit current-working-directory lookup. `UnsetSearchPath(path)` removes a
  configured path during the mutable phase and returns `CAMBIUM_E0001` after
  `Build`.
- `Context.SetFeatures(module, features)` replaces the enabled feature set for a
  module. Calling it before or after module loading updates the next materialized
  schema IR. Once a module is loaded, enabled feature names must resolve to
  declared module-local features or context construction fails with
  `CAMBIUM_E0001`.
- Each `if-feature` expression is evaluated against the enabled feature set.
  Multiple `if-feature` statements on the same target are ANDed together.
- YANG 1.0 sources may use only a single feature reference per `if-feature`.
  YANG 1.1 `not`, `and`, `or`, and parentheses are supported, as are
  `if-feature` statements on `enum`, `bit`, `identity`, and `refine`.
  Prefix-qualified feature refs are resolved through the statement module's
  import table.
- Public schema-node handles expose direct node plus applied `uses`, `augment`,
  and `refine` `if-feature` expression strings in declaration/effective order
  through `IfFeatures()` after feature filtering. Feature, identity, enum/bit value,
  extension-instance, and deviation handles expose their direct source
  expressions. Generated implicit cases for choice shorthand children inherit
  the shorthand child `if-feature` expressions that controlled case
  materialization.
- Invalid syntax, unknown local/prefixed feature references, and feature
  dependency cycles fail context construction with `CAMBIUM_E0001`.
- Feature, identity, typedef, grouping, and extension `description` and
  `reference` metadata are singleton statements; duplicates fail context
  construction with `CAMBIUM_E0001`.
- Feature, identity, typedef, grouping, and extension `status` metadata is
  singleton and must be `current`, `deprecated`, or `obsolete`.

Explicit limitation:

- Full data-tree serialization order is not in the first schema-only pure-Go
  milestone.
- Existing data serialization fixtures such as scrambled data input,
  user-ordered lists, system-list canonicalization, JSON bytes, and gNMI atomic
  output are Backend/data-tier fixtures.
- If pure-Go data serialization is later added, it must use the same ordered IR
  and ordered data tree. It must not route through maps, reflection, or native
  struct field order.

## 13. Public API Floor

The first pure-Go public API is schema-only.

```go
type ContextFlags struct { ... }
type ContextBuilder struct { ... }

func NewContextBuilder(flags ContextFlags) (*ContextBuilder, error)
func (b *ContextBuilder) SearchPath(path string) error
func (b *ContextBuilder) UnsetSearchPath(path string) error
func (b *ContextBuilder) SearchPaths() []string
func (b *ContextBuilder) SetFeatures(module string, features []string) error
func (b *ContextBuilder) LoadModule(name string, revision *string, features []string) error
func (b *ContextBuilder) LoadModuleFromPath(path string) error
func (b *ContextBuilder) LoadModuleStr(source string) error
func (b *ContextBuilder) Build() (*Context, error)

type Context struct { ... }

func NewContext() (*Context, error)
func (c *Context) SetSearchPath(path string) error
func (c *Context) UnsetSearchPath(path string) error
func (c *Context) SearchPaths() []string
func (c *Context) SetFeatures(module string, features []string) error
func (c *Context) LoadModule(name string) error
func (c *Context) LoadModuleFromPath(path string) error
func (c *Context) Schema(moduleOrNamespace string) (Module, error)
func (c *Context) SchemaRevision(moduleOrNamespace, revision string) (Module, error)
func (c *Context) GetModule(name string, revision *string) (Module, bool)
func (c *Context) FindModuleByNS(namespace string) (Module, bool)
func (c *Context) Modules() []Module

type Module struct { ... }
func (m Module) Name() string
func (m Module) Namespace() string
func (m Module) Prefix() string
func (m Module) Revision() (string, bool)
func (m Module) Revisions() []Revision
func (m Module) Organization() (string, bool)
func (m Module) Contact() (string, bool)
func (m Module) Description() (string, bool)
func (m Module) Reference() (string, bool)
func (m Module) YangVersion() string
func (m Module) Features() []Feature
func (m Module) FeatureValue(name string) (bool, bool)
func (m Module) Imports() []Import
func (m Module) Includes() []Include
func (m Module) ExtensionDefinitions() []ExtensionDefinition
func (m Module) TypedefDefinitions() []TypedefDefinition
func (m Module) GroupingDefinitions() []GroupingDefinition
func (m Module) Deviations() []Deviation
func (m Module) ResolvePrefix(prefix string) (Module, bool)
func (m Module) TopLevel() SchemaChildren
func (m Module) Children() SchemaChildren
func (m Module) FindPath(path string) (SchemaNodeRef, error)
// Imported prefix bindings take precedence. If a qualifier is otherwise
// unresolved on an augmented child, the child's owning module prefix or module
// name is accepted. Missing or invalid schema paths return CAMBIUM_E0001.
func (m Module) Identities() iter.Seq[Identity]

type Revision struct { ... }
func (r Revision) Date() string
func (r Revision) Description() (string, bool)
func (r Revision) Reference() (string, bool)

type Import struct { ... }
func (i Import) Description() (string, bool)
func (i Import) Reference() (string, bool)

type Include struct { ... }
func (i Include) Description() (string, bool)
func (i Include) Reference() (string, bool)

type ExtensionDefinition struct { ... }
func (e ExtensionDefinition) Name() string
func (e ExtensionDefinition) Module() Module
func (e ExtensionDefinition) Argument() (string, bool)
func (e ExtensionDefinition) YinElement() (bool, bool)
func (e ExtensionDefinition) Description() (string, bool)
func (e ExtensionDefinition) Reference() (string, bool)
func (e ExtensionDefinition) Status() Status

type Extension struct { ... }
func (e Extension) Name() string
func (e Extension) Argument() (string, bool)
func (e Extension) ModuleName() string
func (e Extension) IfFeatures() []string

type TypedefDefinition struct { ... }
func (d TypedefDefinition) Name() string
func (d TypedefDefinition) Module() Module
func (d TypedefDefinition) Type() (TypeInfo, bool)
func (d TypedefDefinition) Units() (string, bool)
func (d TypedefDefinition) Default() (string, bool)
func (d TypedefDefinition) Description() (string, bool)
func (d TypedefDefinition) Reference() (string, bool)
func (d TypedefDefinition) Status() Status

type GroupingDefinition struct { ... }
func (d GroupingDefinition) Name() string
func (d GroupingDefinition) Module() Module
func (d GroupingDefinition) ChildNames() []string
func (d GroupingDefinition) Description() (string, bool)
func (d GroupingDefinition) Reference() (string, bool)
func (d GroupingDefinition) Status() Status

type Feature struct { ... }
func (f Feature) Name() string
func (f Feature) Module() Module
func (f Feature) Description() (string, bool)
func (f Feature) Reference() (string, bool)
func (f Feature) IfFeatures() []string
func (f Feature) Status() Status
func (i Identity) Name() string
func (i Identity) Module() Module
func (i Identity) Description() (string, bool)
func (i Identity) Reference() (string, bool)
func (i Identity) IfFeatures() []string
func (i Identity) Status() Status
func (i Identity) Bases() []Identity
func (i Identity) Derived() []Identity

type SchemaNodeRef struct { ... }
func (n SchemaNodeRef) Name() string
func (n SchemaNodeRef) Kind() SchemaNodeKind
func (n SchemaNodeRef) Module() Module
func (n SchemaNodeRef) Path() string
func (n SchemaNodeRef) Parent() (SchemaNodeRef, bool)
func (n SchemaNodeRef) Ancestors() []SchemaNodeRef
func (n SchemaNodeRef) YangVersion() string
func (n SchemaNodeRef) IfFeatures() []string
func (n SchemaNodeRef) Children() SchemaChildren
func (n SchemaNodeRef) DataChildren(flattenChoices bool) SchemaChildren
func (n SchemaNodeRef) Input() (SchemaNodeRef, bool)
func (n SchemaNodeRef) Output() (SchemaNodeRef, bool)
func (n SchemaNodeRef) ListKeys() SchemaChildren
func (n SchemaNodeRef) RepresentsConfigurationData() bool
func (n SchemaNodeRef) KeyNames() []string
func (n SchemaNodeRef) LeafType() (TypeInfo, bool)
func (n SchemaNodeRef) TypeDefaultValue() (string, bool)
func (n SchemaNodeRef) Extensions() []Extension
func (n SchemaNodeRef) Extension(name string) (Extension, bool)
func (n SchemaNodeRef) DeviationProvenance() []Deviation

type Deviation struct { ... }
func (d Deviation) TargetPath() string
func (d Deviation) SourceModule() string
func (d Deviation) Type() string
func (d Deviation) Property() string
func (d Deviation) NewValue() string
func (d Deviation) IfFeatures() []string
func (d Deviation) Description() (string, bool)
func (d Deviation) Reference() (string, bool)
```

Do not include data-tree CRUD, validation, diff/merge, or serialization in the
first pure-Go milestone. Those are Backend/data-tier APIs unless a pure-Go data
engine is separately specified.

## 14. Optional Libyang Backend

The existing cgo work moves behind an explicit optional boundary before the
pure-Go floor is expected to pass.

Possible names:

```text
github.com/signalbreak-labs/cambium/go/libyangbackend
github.com/signalbreak-labs/cambium/go-libyang
github.com/signalbreak-labs/cambium/libyang-go
```

The backend may provide:

- full RFC validation;
- XML/JSON_IETF/LYB serialization;
- deviation behavior oracle;
- Backend/data conformance in CI;
- migration comparison against pure-Go IR.

Rules:

- backend extraction is M0/M1 work, not a final cleanup milestone;
- cgo files must not be imported by default package tests;
- public pure-Go API must not mention backend types;
- backend failures must not block pure-Go schema users unless they opt in;
- no `//go:build !cgo` stub may silently degrade a public method by pretending
  backend-only behavior is available.

## 15. Conformance Plan

Create a pure-Go schema conformance tier.

```text
conformance/
  fixtures/
    schema-cross-kind-order/
    schema-uses-site-order/
    schema-augment-order/
    schema-choice-case-order/
    schema-import-prefix/
    schema-identity-derived/
    schema-leafref-path/
```

Implemented under `conformance/fixtures/*/expected-ir.json` for the seven
fixtures above, with a pure-Go runner in `go/cambium` that asserts ordered
children, flattened choice/case data children, key order, import/prefix
resolution, identity derived closure, and leafref path/target properties.

Each fixture asserts IR properties, not printer bytes:

- ordered list of top-level children;
- synthetic module root `Children()` preserves declaration interleaving across
  data nodes, module-level RPCs, and module-level notifications, while
  `TopLevel()`, `RPCs()`, and `Notifications()` remain filtered views in their
  own declaration order;
- included submodule top-level schema statements expand at the parent module's
  legal `include` declaration site, including interleaved data, RPC, and
  notification statements; submodule-to-submodule includes are followed
  recursively and expand at the nested include site, and under YANG 1.1 every
  nested submodule include must also be declared directly by the parent module
  with the same `revision-date` when the nested include is revision-pinned;
- YANG 1.0 submodules can reference sibling submodule definitions only through
  their own `include` chain; YANG 1.1 submodules can reference definitions in
  all submodules included by the parent module without a direct sibling include;
- imports declared inside included submodules are retained in `Module.Imports()`
  and prefix resolution using the same include-expanded declaration order;
- ordered list of nested children;
- key names;
- type info;
- defaults and extension order;
- identity bases and derived closure;
- resolved prefix tables;
- leafref target, if resolvable. Focused Go introspection/codegen tests also
  cover backend-data leafref fixtures with `current()` predicates and simple
  `deref()` schema targets.
- instance-identifier `require-instance` plus generated XML/JSON byte parity
  for values whose XML path prefixes and JSON_IETF module-name paths differ.
- codegen field collection from flattened data children, so choice/case
  descendants are emitted at the choice declaration site and list keys remain
  first.
- codegen native serializers preserve `ordered-by user` vectors exactly, but
  canonicalize `ordered-by system` leaf-lists by scalar value and keyed lists by
  key tuple before emitting XML/JSON bytes.
- codegen emits a raw `AnyData` helper for `anydata`/`anyxml`, preserving
  separate XML and JSON payload forms under the generated schema field.
- codegen emits a `MetadataAnnotation` helper and per-struct `CambiumMetadata`
  map for RFC 7952 instance metadata on scalar fields, preserving XML
  attributes and JSON `@node` siblings in node-local emission order.
- codegen emits `WithDefaultsMode` plus `ToJSONIETFWithDefaults`, preserving
  existing `ToJSONIETF()` explicit output while supporting RFC 6243 trim and
  report-all/report-all-tagged behavior for scalar schema defaults and
  defaulted leaf-lists. Explicit leaf-list values equal to schema defaults are
  omitted under trim, and absent leaf-list defaults follow the same
  `ordered-by` policy as explicit values: default-statement order for
  user-ordered lists and canonical scalar order for system-ordered lists. Defaults
  under `choice`/`case` materialize only when their case is selected, or when that
  case is the choice default and no other case is selected. Schema defaults
  resolve through effective type information before generated JSON_IETF
  emission/comparison, so default literals use canonical scalar wire forms and
  union defaults use the selected member's JSON wire form rather than falling
  back to string quoting.
- codegen emits a root `FromJSONIETF` parser for generated packages. The native
  path covers JSON objects, containers, lists, leaf-lists, empty, string, bool,
  integer, decimal64, numeric range newtypes, enum/identityref, bits,
  instance-identifier, ordered scalar union member trials, leafref-resolved
  union targets, union `instance-identifier` members, and JSON anydata values.
  Integer parsing enforces the RFC7951 JSON wire form: `int64`/`uint64` are
  quoted strings, while smaller integer widths are bare JSON numbers, including
  ordered union member trials. Quoted integer strings trim libyang-compatible
  surrounding JSON whitespace before parsing and unsigned integer strings
  canonicalize libyang-compatible `+N` and `-0` lexical forms back to unsigned
  decimal output. Restricted string and integer union members route
  through generated length/range constructors before a union trial can match.
  Decimal64 parsing enforces the schema fraction-digits and decimal64 range
  before returning generated structs. String pattern constraints, including
  invert-match, are enforced with full-string generated regexp checks for
  scalar and union string members. Generated identityref parsers accept bare
  names only for identities in the generated module; foreign-module identities
  must use their JSON_IETF `module:name` spelling. Bits parsing rejects duplicate bit names
  instead of silently collapsing them through the generated set representation.
  Generated parse-and-reemit coverage includes empty, binary,
  instance-identifier, standalone/hierarchical/foreign identityref, nested
  action/notification, user-ordered list/leaf-list arrays, anydata, metadata,
  enum/bits, union, and representative scalar and valid constraint-data fixtures.
  Non-canonical system-ordered list and leaf-list JSON inputs are parsed and
  re-emitted in deterministic canonical order. Cross-module generated root
  documents parse and re-emit selected-module plus imported top-level data in
  the generated field-order contract.
  RFC 7952 JSON
  metadata siblings are parsed into `CambiumMetadata`.
  Oversized JSON_IETF inputs are rejected before decoding and excessive JSON
  nesting is rejected during token walking. Unknown top-level and nested JSON
  members are rejected with deterministic lexicographic diagnostics, duplicate JSON object members are rejected before map
  decoding can overwrite them, missing ordinary mandatory leaf fields and list
  entries missing required key fields are rejected instead of zero-filled,
  omitted unconditional non-presence containers that carry mandatory scalar
  descendants are rejected, explicit choice/case JSON members whose parsed
  zero value would otherwise look absent are validated while the parser still
  has the member-presence signal, malformed decimal64 lexical values and
  decimal64 raw-range overflows are rejected before schema range validation,
  invalid UTF-8 input bytes are rejected before JSON decoding, escaped/decoded
  JSON string code points outside libyang's accepted YANG string character set
  are rejected, generated bits parsers reject non-space whitespace separators,
  generated binary parsers canonicalize libyang-compatible PEM newlines every
  64 base64 characters while rejecting malformed base64 whitespace, binary
  values are base64-decoded and checked against decoded-octet length constraints, and parsed anydata JSON is reserialized in
  deterministic generated formatting. RFC 7952 metadata siblings are rejected
  unless the corresponding data node is present in the same JSON object, and
  metadata annotation names must be module-qualified and declared by an
  RFC 7952 `md:annotation` statement in the generated module/import closure.
  Successful parses run generated
  `Validate()` before returning, so generated structural validation failures are
  parse-boundary failures. Top-level
  RPC and notification document structs expose typed `From<Operation>JSONIETF`
  parsers with the same strict root and nested field checks. Generated
  parse-and-reemit coverage spans the top-level RPC/notification JSON operation
  document fixture matrix. Union parse failures return an explicit parse error
  when no ordered member trial matches.
- generated root document structs include selected-module top-level data first,
  then direct imported modules' top-level data in import statement order. This
  keeps single-module behavior stable while allowing cross-module leafref
  documents to emit target and referring nodes together. Imports declared in
  included submodules participate in the same generated document boundary. Root
  validation uses the same document boundary for flattened choice/case checks.
- codegen emits standalone typed operation document structs for module-level
  RPCs and notifications. RPCs with both `input` and `output` preserve the
  existing request-shaped `<RpcName>RPC` struct and also emit a response-shaped
  `<RpcName>RPCOutput` struct over the output payload; output-only RPCs keep the
  `<RpcName>RPC` name. Nested actions and notifications are exposed as optional
  operation fields on containing data structs; nil fields preserve data-only
  output, while set fields serialize operation payload children after ordinary
  data children for libyang byte parity.
- generated Go structs implement `Validate() error` with a local
  `ValidationError` for typed-struct safety checks that do not require a data
  parser or XPath engine: recursive child validation, list/leaf-list
  `min-elements` and `max-elements`, duplicate leaf-list values where YANG
  requires uniqueness (configuration-data leaf-lists, plus YANG 1.0
  compatibility), duplicate list key tuples, direct list `unique` tuples,
  binary base64 plus decoded-octet length checks,
  decimal64 fraction-digits plus range checks, generated integer range and
  string length wrapper revalidation for zero-value/direct-assignment safety,
  with string length counted as Unicode characters rather than UTF-8 bytes,
  string pattern checks, duplicate bits constructor input,
  enum/identityref domain checks, union member validation for binary,
  decimal64, enum/identityref, restricted string/integer members, and string
  patterns, and flattened choice/case single-case plus
  mandatory-choice constraints, including nested choice constraints, scalar
  constraints on selected case descendants, mandatory descendants, and
  `min-elements`/`max-elements` collections only when their case is selected;
  a choice default case is treated as selected when no case data is present.
  Generated RFC 7952 XML metadata attributes use XML attribute escaping,
  including quote escaping, rather than element-text escaping. XML namespace
  attribute values, including schema-derived static namespaces and dynamic
  identityref/instance-identifier helper bindings, use the same attribute
  escaping. Dynamic XML namespace prefixes supplied through generated helper
  values are validated before generated `Validate()` succeeds, so malformed or
  reserved prefixes cannot be emitted as `xmlns:` attribute names through the
  normal parse/validate boundary.
  Generated validation also rejects `CambiumMetadata` entries for
  unknown/absent child nodes or metadata annotation names/XML bindings that
  cannot be resolved through a declared RFC 7952 annotation in the generated
  module/import closure.
  YANG 1.1 read-only leaf-lists retain repeated
  values. Full
  RFC validation, including XPath `must`/`when` and leafref instance existence,
  remains backend/data-engine scope.
- imported typedefs are parsed in the typedef defining module, so nested typedef
  references inside imported typedef bodies retain their real base type and
  typedef provenance.
- scoped typedefs/groupings are resolved from the `type` or `uses` statement's
  lexical parent chain, and duplicate typedef/grouping/identity/feature/extension
  definitions fail the pure-Go build path with `CAMBIUM_E0001`; scoped
  `typedef`/`grouping` declarations are indexed only under legal definition
  scopes (`grouping`, `container`, `list`, `rpc`, `action`, `input`, `output`,
  and `notification`) and fail closed elsewhere as invalid placement. Direct
  known non-extension children of `grouping` are limited to data nodes
  (`container`, `leaf`, `leaf-list`, `list`, `choice`, `anydata`, `anyxml`),
  `action`, `notification`, `uses`, nested `typedef`/`grouping`, and supported
  grouping metadata (`description`, `reference`, `status`). Direct known
  non-extension children of `typedef` are limited to `type`, `default`, `units`,
  `description`, `reference`, and `status`.
- `uses` substatement augments are applied to the expanded grouping tree for
  that use instance only, with augmented children appended after the target's
  existing expanded children. `refine` and `augment` are valid only directly
  under `uses`; missing target arguments or nesting anywhere else fails closed
  with `CAMBIUM_E0001`; their target arguments must be valid descendant
  schema-nodeids, and prefixed target steps must resolve instead of matching by
  local name only. Direct known non-extension children of `augment` are limited
  to data definition nodes, `case`, `uses`, `action`, `notification`, `when`,
  `if-feature`, `status`, `description`, and `reference`. A direct `when` under
  `augment` is retained as an effective condition on the augmented schema nodes.
  Module-level `augment` arguments must be valid absolute
  schema-nodeids; relative-looking or otherwise malformed paths fail closed with
  `CAMBIUM_E0001`. In YANG 1.0, cross-module augments cannot add mandatory
  nodes that represent configuration data; in YANG 1.1, cross-module augments
  that add such nodes must have a direct `when`. Mandatory RPC/action
  input/output and notification payload nodes are not subject to this
  config-data augment rule. Direct known non-extension children of
  `refine` are limited to `config`, `default`, `description`, `if-feature`,
  `mandatory`, `max-elements`, `min-elements`, `must`, `presence`, and
  `reference`.
  `description` and `reference` under `refine` replace the effective schema
  node text metadata exposed by `SchemaNodeRef`.
- Top-level-only definition statements (`identity`, `feature`, and `extension`)
  fail closed with `CAMBIUM_E0001` when they appear nested under schema or
  grouping statements. Direct known non-extension children of `identity` are
  limited to `base`, `description`, `if-feature`, `reference`, and `status`;
  direct known non-extension children of `feature` are limited to
  `description`, `if-feature`, `reference`, and `status`; direct known
  non-extension children of `extension` are limited to `argument`,
  `description`, `reference`, and `status`; direct known non-extension children
  of `argument` are limited to `yin-element`.
- Module/submodule-body-only statements (`belongs-to`, `contact`, `deviation`,
  `import`, `include`, `namespace`, `organization`, `revision`, and
  `yang-version`) fail closed with `CAMBIUM_E0001` when nested under schema or
  grouping statements.
- statement keywords, required standard statement arguments, identifier-valued
  arguments, and simple identifier-ref/QName arguments (`type`, `base`, `uses`)
  are validated before resolver/index construction. Malformed names such as
  quoted identifiers with spaces or identifiers starting with digits fail closed
  with `CAMBIUM_E0001`. Identifiers starting with `xml` in any case fail closed
  in YANG 1.0 sources and are accepted in YANG 1.1 sources. Every known
  unprefixed standard statement except `input` and `output` must carry an
  explicit argument, while `input` and `output` must not carry arguments.
- unknown unprefixed statement keywords fail closed at any depth instead of
  being ignored; prefixed extension statements continue through extension
  resolution and must resolve to declared extensions.
- Extension definition `description`, `reference`, `argument`, and nested
  `yin-element` metadata are singleton statements; `argument` is valid only
  under `extension`, `yin-element` is valid only under `argument`, and
  `yin-element` accepts only `true` or `false`. Unknown prefixed extension statements anywhere in loaded
  module/submodule source fail closed with `CAMBIUM_E0001` instead of being
  silently ignored or omitted from schema-node extension metadata. Extension
  instances must provide an argument exactly when the resolved extension
  definition declares one, and expose their direct `if-feature` expressions
  through `Extension.IfFeatures()`.
- unresolved typedefs, missing groupings, unmatched `uses`/`refine` target paths,
  unknown or duplicate identity bases, YANG 1.0 multi-base identity/identityref
  declarations, typedef cycles, and identity cycles fail closed with
  `CAMBIUM_E0001` instead of degrading to partial `unknown` schema IR.
- list key and `unique` references are resolved fail-closed: duplicate/missing
  keys, keys that do not name direct child leaves, duplicate unique paths, and
  unique paths that do not resolve to descendant leaves return `CAMBIUM_E0001`.
  Key leaves with type `empty` require `yang-version 1.1`; unique leaves cannot
  have type `empty`. Empty or misplaced `key`/`unique` statements also fail
  closed, including deviation-added/replaced `unique` properties; legal
  descendant unique paths such as `nested/code` are retained in the ordered
  schema IR. Configuration-data lists with final effective `config true` must
  define a non-empty `key`; keyless lists are allowed for state data and for
  RPC/action input/output or notification payload data. Final effective
  `config` is evaluated after `uses`, `refine`, `augment`, and deviation
  processing.
  Key leaves cannot carry direct or `refine`-applied `if-feature` statements
  and cannot carry effective `when` statements; inherited gating from a
  containing `uses` or augment remains visible through `IfFeatures()` but does
  not by itself invalidate the key. Key leaves' final effective `config` must
  match the list. A `unique` constraint cannot mix leaves that represent
  configuration data with leaves that do not; RPC/action input/output and
  notification payload leaves never represent configuration data even when
  their raw `Config()` is `ConfigRw`.
- malformed built-in type bodies fail closed with `CAMBIUM_E0001`: decimal64
  requires exactly one valid `fraction-digits` in 1..18, identityref requires a
  non-duplicate base and permits multiple bases only in YANG 1.1, leafref
  requires one non-empty path, union requires at least one member and permits
  `empty`/`leafref` member types only in YANG 1.1,
  and enumeration/bits require at least one effective value; enum/bit names and
  numeric values/positions are checked for duplicates, malformed assignments,
	  misplaced `value`/`position` substatements, and duplicate `value`/`position`
	  substatements on one enum/bit. Enum/bit
	  `description`, `reference`, and `status` metadata are singleton statements,
	  and `status` must be `current`, `deprecated`, or `obsolete`. Direct known
	  non-extension children of `enum` and `bit` are limited to their
	  value/position statement, `if-feature`, and supported metadata. `EnumValue`
	  exposes the ordered name, numeric value/position, optional `description` and
	  `reference`, direct `if-feature` expression strings, and `status` for both
	  enum and bit values.
	  Typedef-derived enum and bits restrictions narrow the ordered value set while
	  preserving base values/positions, and typedef-derived identityref
	  restrictions narrow the ordered base set to declared derived base identities.
  Known type restriction substatements such as `range`, `length`, and
  `fraction-digits` are valid only on compatible effective base types.
  Type-body substatements (`bit`, `enum`, `fraction-digits`, `length`, `path`,
  `pattern`, `range`, and `require-instance`) are valid only under a `type`
  statement; `base` is valid only under `identity` or `type`. Direct known
  non-extension children of `type` are limited to those statements plus nested
  union member `type` statements.
  Typedef-derived `range` and `length` restrictions must stay within the
  typedef base restriction.
  `range`/`length` `error-message`, `error-app-tag`, `description`, and
  `reference` metadata are singleton statements and are exposed through the
  public `RangeBound` values returned by resolved range/length introspection.
  Public resolved type slice metadata is defensive: mutating returned `Range`,
  `Length`, `Patterns`, `Values()`, `Bases()`, or `Members()` slices must not
  mutate the schema IR observed by later handle reads.
  `leafref` and `instance-identifier` `require-instance` values must be absent,
  `true`, or `false`; explicit `require-instance` statements on `leafref` types
  require `yang-version 1.1`, while instance-identifier `require-instance`
  remains valid in YANG 1.0. Typedef-derived leafref and instance-identifier
  restrictions preserve explicit `require-instance` overrides. Config leafrefs
  with effective `require-instance true` cannot target state data; RPC/action
  input/output and notification payload leafrefs are not treated as config
  leafrefs for this restriction.
- leaf and leaf-list schema nodes require exactly one `type` statement, and
  every typedef definition requires exactly one `type` statement whether or not
  it is referenced. Typedef type resolution fails closed for unknown types,
  cycles, and invalid type-body restrictions even when the typedef is unused.
  Built-in YANG type names are accepted only without prefixes; prefixed type
  names must resolve as typedef QNames or fail closed.
	  Missing or duplicate type statements fail closed instead of producing unknown
	  or first-type-wins IR. `type` statements on other schema node kinds fail
	  closed as invalid placement, including when introduced by deviations.
	  Under known non-extension parents outside schema nodes, direct `type`
	  statements are valid only under `typedef`, `type` (union member), or
	  `deviate`; other nested appearances fail closed.
- grouping bodies are type-validated through a scratch tree independently of
  `uses` expansion, so missing/duplicate/unknown types and invalid type
  restrictions inside unused grouping definitions fail closed without changing
  public traversal order. Default placement/cardinality, duplicate sibling-name,
  and list `unique` leaf type rules run over the same scratch grouping tree.
	- `range` and `length` restrictions fail closed for empty/malformed segments,
	  repeated range/length statements, integer bounds outside the target base type,
	  negative or malformed length bounds, and invalid decimal64 range lexical forms
	  such as exponents. Segments with lower bounds greater than upper bounds fail
	  closed, and multi-segment restrictions must be ordered and non-overlapping.
	  Direct known non-extension children of `range` and `length` are limited to
	  `error-message`, `error-app-tag`, `description`, and `reference`.
	  String pattern `modifier` substatements must be singleton `invert-match`
	  values and require `yang-version 1.1`; pattern expressions must compile
	  for the native full-string regexp checks before entering schema
	  IR/codegen; pattern metadata children (`error-message`, `error-app-tag`,
	  `description`, `reference`) are
	  singleton statements. Direct known non-extension children of `pattern` are
	  limited to those metadata statements plus `modifier`. Constraint error
	  metadata (`error-message`, `error-app-tag`) is valid only under `must`,
	  `range`, `length`, and `pattern`; `modifier` is valid only under
	  `pattern`. Pattern error-message, error-app-tag, description, and
	  reference metadata are exposed through the public `Pattern` handle.
- `min-elements`/`max-elements` cardinality metadata fails closed for malformed
  `min-elements` values, malformed or zero numeric `max-elements` values, and
  finite `min-elements > max-elements`, including values applied through
  `refine` or `deviation`. `min-elements 0` is valid. Deviation `add` requires
  the target cardinality statement to be absent, `replace` requires it to exist, and
  `delete` requires an exact matching value. Explicit `max-elements unbounded`
  is tracked internally as an existing statement even though the public
  `MaxElements()` accessor continues to report no finite bound.
- scalar schema metadata fails closed for malformed enum/bool values: `status`,
  `config`, `mandatory`, and `ordered-by` are validated before IR/codegen
  consumers can observe defaulted replacements. Singleton schema metadata
  statements (`status`, `config`, `mandatory`, `ordered-by`, `presence`,
  `description`, `reference`, `units`, list `key`, and node `when`) fail closed
  when duplicated on one schema node; `refine`
  statements apply the same singleton rule for `mandatory`, `config`, and
  `presence`. `units` is valid only on leaf and leaf-list nodes, including when
  introduced by deviations; `mandatory` is valid only on leaf, choice, anydata,
  and anyxml nodes, including when introduced by `refine` or `deviation`;
  `config` is valid only on data nodes, including when introduced by `refine` or
  `deviation`, and descendants of a `config false` node cannot set
  `config true`. Effective `config` inheritance is recomputed after `refine` or
  deviation changes, so descendants without their own `config` statement inherit
  the final parent value rather than the grouping build-time value; `presence`
  is valid only on container nodes, including when
  introduced by `refine`; `status` is valid only on data, choice, case, rpc,
  action, and notification nodes. Operation `input` and `output` nodes are valid
  only under rpc or action nodes, `action` nodes are valid only under container
  or list nodes, and `notification` nodes are valid only at module top level or
  under container/list nodes. `action` and `notification` nodes cannot have a
  keyless list ancestor or an `rpc`, `action`, or `notification` ancestor.
  `rpc` nodes are valid only at module top level.
  RPC/action nodes may contain at most one `input` and at most one `output`
  body. Direct known non-extension children of `rpc` and `action` are limited
  to `input`, `output`, nested `typedef`/`grouping`, `if-feature`, `status`,
  `description`, and `reference`. Direct known non-extension children of
  operation `input` and `output` are limited to data nodes (`container`, `leaf`,
  `leaf-list`, `list`, `choice`, `anydata`, `anyxml`), `must` in YANG 1.1
  sources, `uses`, and nested `typedef`/`grouping`. Direct known non-extension
  children of `notification` are limited to data nodes, `must` in YANG 1.1
  sources, `uses`, nested `typedef`/`grouping`, `if-feature`, `status`,
  `description`, and `reference`. `case` nodes are valid only under choice
  nodes. Direct known
  non-extension children of `choice` are limited to shorthand data
  nodes (`container`, `leaf`, `leaf-list`, `list`, `anydata`, `anyxml`),
  direct nested `choice` shorthand in YANG 1.1 sources, `case`, supported
  choice metadata (`config`, `default`, `description`, `if-feature`,
  `mandatory`, `reference`, `status`, `when`), and `uses` only so the
  dedicated direct-`uses` rejection path can report the placement error.
  Direct known non-extension children of `case` are limited to data nodes
  (`container`, `leaf`, `leaf-list`, `list`, `choice`, `anydata`, `anyxml`),
  `uses`, `when`, and supported case metadata (`description`, `if-feature`,
  `reference`, `status`).
  Schema/operation `description` and `reference` follow the same placement
  rule. `must`
	  metadata children (`error-message`, `error-app-tag`, `description`,
	  `reference`) and `when` metadata children (`description`, `reference`) are
	  singleton as well, including deviation-added/replaced `must` statements.
	  Direct known non-extension children of `must` are limited to those metadata
	  statements; direct known non-extension children of `when` are limited to
	  `description` and `reference`. `must` is valid only on container, leaf,
	  leaf-list, list, anydata, and anyxml nodes, including when introduced by
	  `refine` or `deviation`. `when` is valid only on data, choice, and case nodes.
- Data definition nodes (`container`, `leaf`, `leaf-list`, `list`, `choice`,
  `anydata`, and `anyxml`) are valid only at module top level or under
  `container`, `list`, `choice`, `case`, `input`, `output`, or `notification`
  nodes; direct data children under scalar nodes or directly under
  `rpc`/`action` bodies fail closed with `CAMBIUM_E0001`.
- `uses` statements are valid at module top level or under `container`, `list`,
  `case`, `input`, `output`, or `notification` nodes. Direct `uses` under
  scalar nodes, raw `rpc`/`action` bodies, or directly under `choice` nodes fail
  closed with `CAMBIUM_E0001`. Direct known non-extension children of `uses`
  are limited to `augment`, `description`, `if-feature`, `reference`, `refine`,
  `status`, and singleton `when`. A direct `when` under `uses` is retained as
  an effective condition on the expanded grouping instance's schema nodes.
  Recursive grouping expansion through `uses` cycles fails closed with
  `CAMBIUM_E0001` instead of recursing unboundedly.
- Known YANG statements outside the module/submodule top-level allowlist fail
  closed with `CAMBIUM_E0001` instead of being ignored. Extension-prefixed
  statements are not rejected by this generic allowlist. `module` and
  `submodule` statements are valid only as the parsed source root; nested
  appearances fail closed.
  Known non-extension children under scalar statements with no standard
  substatements also fail closed.
- illegal defaults fail closed after final schema materialization: leaves cannot
  carry multiple defaults, leaf-list default values cannot be duplicated,
  leaf-list defaults require `yang-version 1.1`, leaf-lists with
  `min-elements` greater than zero cannot default, mandatory leaves cannot
  default, list key leaves cannot default, choice defaults must name an existing
  case, mandatory choices cannot default, and defaults are
  rejected on node kinds other than leaf, leaf-list, or choice. Typedef
  definitions cannot carry multiple defaults before default inheritance whether
  or not the typedef is referenced. Typedef default values are validated against
  the typedef's effective type even when the typedef is unused. `refine`
  defaults are singleton statements and preserve exact default arguments,
  including `default "";`. Boolean defaults must be `true` or `false`; integer
  defaults must parse within the effective base type and range restriction;
  decimal64 defaults must satisfy the effective `fraction-digits` and range
  restriction; enumeration defaults must name an effective enum value not
  marked with `if-feature`; bits defaults must name effective bit values not
  marked with `if-feature`, without duplicate tokens; string defaults must
  satisfy effective length restrictions; binary defaults must be
  base64 and satisfy effective decoded length restrictions; identityref defaults
  must resolve to an identity derived from the effective base set; `empty` types
  cannot have defaults; union defaults must be accepted by at least one
  effective member type; leafref defaults are validated against the resolved
  target real type when resolvable.
- duplicate same-module sibling schema nodes fail closed after augment/deviation
  materialization, while same local names from different owner modules remain
  representable for cross-module augment collision cases.
- included augment/deviation statements with malformed absolute schema-nodeid
  target paths or unresolved targets fail closed with `CAMBIUM_E0001` instead
  of being skipped from the materialized schema IR.
- deviation statements require at least one `deviate`, and unknown `deviate`
  operation names fail closed with `CAMBIUM_E0001`; only `not-supported`,
  `add`, `replace`, and `delete` are accepted. `deviate` statements are valid
  only directly under `deviation`; misplaced `deviate` statements fail closed
  instead of being ignored. Direct known non-extension children of `deviation`
  are limited to `deviate`, `if-feature`, `description`, and `reference`. Deviation
  `description` and `reference` metadata are singleton statements. Deviation
  handles expose their direct `if-feature` expressions through
  `Deviation.IfFeatures()`.
  Direct known non-extension children of `deviate` are operation-specific:
  `not-supported` has none; `add` accepts `config`, `default`, `mandatory`,
  `max-elements`, `min-elements`, `must`, `unique`, and `units`; `replace`
  accepts those plus `type`; `delete` accepts `default`, `max-elements`,
  `min-elements`, `must`, `unique`, and `units`. Valid extension-prefixed
  substatements under `deviate` remain extension instances and are not emitted
  as public `Deviation.Property()` entries.
  `deviate add units` fails closed if the target already has `units` instead of silently becoming a no-op,
  and `deviate replace units` fails closed if there is no existing `units` to
  replace. `deviate delete units` requires a matching target `units` value.
  `deviate add default` rejects an already-present target default value,
  `deviate replace default` requires an existing target default, and `deviate
  delete default` requires a matching target default value. `deviate replace
  must` and `deviate replace unique` require existing target constraints;
  `deviate delete must` and `deviate delete unique` require matching target
  constraints.
- imports require one explicit `prefix` child, and duplicate import prefixes in
  a module or its included submodules fail closed before prefix tables can
  overwrite a prior target. Import prefixes that collide with the module's own
  prefix or self-resolving module-name alias fail closed as ambiguous.
  Direct known non-extension children of `import` are limited to `prefix`,
  `revision-date`, `description`, and `reference`; direct known non-extension
  children of `include` are limited to `revision-date`, `description`, and
  `reference`, and other known children fail before dependency lookup.
  Nested `prefix` statements are valid only under `import` or `belongs-to`;
  misplaced nested prefixes fail closed instead of being ignored.
  `revision-date` is valid only under `import` or `include`; misplaced
  `revision-date` statements fail closed.
- module and submodule `revision` metadata plus import/include `revision-date`
  pins fail closed for malformed calendar dates and duplicate pin statements
  before the source can enter the context or trigger dependency lookup; revision
  `description` and `reference` metadata are singleton statements, and direct
  known non-extension children of `revision` are limited to those statements.
  Import/include `description` and `reference` metadata are also singleton
  statements.
- module/submodule header singleton metadata now fails closed for duplicate
  `namespace`, `prefix`, `yang-version`, `belongs-to`, or `belongs-to` prefix
  statements, plus duplicate top-level `contact`, `organization`,
  `description`, and `reference` header metadata; `yang-version` only accepts
  `1` and `1.1` and has no standard non-extension children, and direct known
  non-extension children of `belongs-to` are limited to `prefix`.
- codegen namespace-aware child emission for augment nodes whose owner module
  differs from the containing schema node: XML gets the child namespace and
  JSON_IETF gets the child module-qualified member name.

Later tiers may compare pure-Go IR against libyang backend properties where the
property is shared. Do not compare pure-Go augment placement to libyang bytes
unless the expected placement rule has been explicitly made common.

## 16. Required Red Tests

First coding work starts by committing failing tests.

> Implementation note: the exact test names below are present in the codebase.
> `TestModuleImportsResolvePrefix`, `TestFindPathRoundTrip`,
> `TestNoEntryDirOrTypedSliceDependency`, `TestSchemaChildrenPreserveCrossKindDeclarationOrder`,
> `TestGroupingUsesExpansionOrder`, `TestAugmentPlacementPureGoGolden`, and
> `TestChoiceCaseOrder` are thin spec-traceability wrappers over existing helpers
> and fixtures.

### M0/M1 boundary tests

1. `TestPureGoNoCGO`
   - Run with `CGO_ENABLED=0`.
   - Imports `github.com/signalbreak-labs/cambium/go/cambium` and
     `github.com/signalbreak-labs/cambium/go/compat`.
   - Builds and runs a schema parse plus compat projection without cgo.

2. `TestPureGoNoCGODependencyClosure`
   - Runs `go list -deps ./cambium ./codegen ./compat`.
   - Fails if the closure contains any libyang-named import path,
     `libyangbackend`, any package with cgo files, or any `import "C"`
     dependency.

3. `TestNoSilentNoCGOBackendStub`
   - Fails if a `//go:build !cgo` file in the public package implements
     backend-only behavior by returning fake success.
   - Backend-only APIs must be absent from the pure-Go floor or return explicit
     unsupported errors from backend packages only.

4. `TestNoCGOConformanceManifestDeclaresSupportedTiers`
   - Runs in `go/conformance` with `CGO_ENABLED=0`.
   - Confirms schema-IR cases are declared with `expected-ir` and no backend
     input/output fields, while backend-data byte cases remain manifest-present
     for the optional cgo runner.

### M1 schema floor tests

4. `TestSchemaChildrenPreserveCrossKindDeclarationOrder`
   - Module interleaves `leaf`, `container`, `leaf-list`, `list`, and `choice`.
   - `Children()` returns that exact interleaving, never alphabetical and never
     kind-grouped.

5. `TestModuleRootChildrenPreserveOperationInterleaving`
   - Module interleaves data children, module-level RPCs, and module-level
     notifications.
   - Synthetic module root `Children()` returns that exact interleaving while
     `TopLevel()`, `RPCs()`, and `Notifications()` remain filtered and ordered.

6. `TestSchemaChildrenMapDoesNotDriveOrder`
   - Repeat loads in a loop.
   - Assert order is stable and declaration-order, not Go map order.

7. `TestListKeysStatementOrder`
   - Key statement `"name color"` returns that order even if leaves are declared
     elsewhere.

7. `TestModuleImportsResolvePrefix`
   - Imported module prefix resolves.
   - Empty prefix resolves to self.

8. `TestNoEntryDirOrTypedSliceDependency`
   - Static test or focused source scan fails if public traversal uses
     `Entry.Dir` or goyang typed child slices to assemble order.

### M3 effective schema tests

9. `TestGroupingUsesExpansionOrder`
   - Grouping has `g1`, `g2`; use site has sibling before/after.
   - Expanded order matches effective schema order at the `uses` statement.

10. `TestAugmentPlacementPureGoGolden`
    - Direct target children come first.
    - Augment children follow the pure-Go placement rule in section 12.
    - This is a pure-Go golden, not a libyang differential.

11. `TestChoiceCaseOrder`
    - Choice/cases and case children retain declaration order.

12. `TestIdentityDerivedClosure`
    - Same-module and cross-module identity bases work.

13. `TestFindPathRoundTrip`
    - `FindPath(node.Path())` returns same node.
    - Unknown prefixes fail.

## 17. Milestones

### M0 - Contract, Gate A, And Backend Boundary Plan

Deliverables:

- this document plus matching `AGENTS.md`, `spec/api.md`, and
  `spec/ordering-invariants.md` amendments;
- Gate A written result: which goyang raw statement type preserves declaration
  order, and which typed structures are unsafe;
- explicit package/module decision for the optional backend;
- license/NOTICE plan for goyang import/fork.

Exit criteria:

- no production pure-Go API beyond experiments;
- architecture decision recorded;
- reviewers agree the current normative contract no longer forbids pure-Go.

### M1 - Pure-Go Schema Floor And Cgo-Free Closure

Deliverables:

- extract current cgo/libyang code out of `go/cambium` and `go/codegen` import
  closures, or move it behind the chosen optional backend package/module first;
- `go/cambium` builds with `CGO_ENABLED=0`;
- load module from path/search path;
- expose ordered direct schema nodes;
- support modules/imports/includes enough for common models;
- type floor: builtins, typedef name, defaults, list keys;
- identity floor if it can be implemented without delaying the cgo-free floor.

Exit criteria:

```bash
cd go
CGO_ENABLED=0 go test ./cambium ./codegen ./compat
CGO_ENABLED=0 go vet ./cambium ./codegen ./compat
```

The dependency-closure test must prove `go/cambium`, `go/codegen`, and
`go/compat` do not import `internal/libyang`, `libyangbackend`, or any cgo
package.

### M2 - Type And Constraint Introspection

Deliverables:

- rich `TypeInfo`;
- enum/bits values;
- ranges, lengths, patterns;
- leaf-list defaults;
- must/when expression pass-through only, not XPath evaluation;
- extension instance pass-through.

Exit criteria:

- all schema introspection tests pass under `CGO_ENABLED=0`;
- no cgo package imported by `go/cambium`, `go/codegen`, or `go/compat`.

### M3 - Effective Schema Features

Deliverables:

- grouping/uses/refine support;
- augment support under the pure-Go placement rule;
- deviation metadata support;
- feature/if-feature filtering if practical;
- RPC/action/notification schema order.

Exit criteria:

- pure-Go schema IR fixtures pass;
- backend differential tests compare only shared properties;
- gaps documented if not cleanly implementable.

### M4 - Codegen Over Pure-Go IR

Deliverables:

- codegen no longer requires cgo;
- field-order manifest derives from pure-Go IR;
- generated serializers do not use map order.

Exit criteria:

```bash
cd go
CGO_ENABLED=0 go test ./codegen
```

### M5 - Optional Backend Hardening

Deliverables:

- backend package/module has clear docs and build tags;
- Backend/data conformance is separated from Schema IR conformance;
- docs explain when users need the backend;
- backend tests cover validation/serialization without leaking into default Go.

Exit criteria:

```bash
cd go
CGO_ENABLED=0 go test ./cambium ./codegen ./compat
CGO_ENABLED=1 go test ./...
```

M5 is not where cgo extraction begins. It is hardening after the default import
closure is already pure Go.

## 18. Migration From Current Main

The current `main` contains cgo-backed Go code. The pure-Go rebuild should avoid
a chaotic partial rewrite.

Recommended sequence:

1. Keep current main green until the backend boundary work starts.
2. Create the optional backend package/module and move current cgo code there, or
   otherwise remove it from the default public import closure.
3. Add the dependency-closure and `CGO_ENABLED=0` red tests.
4. Build the pure-Go schema floor in `go/cambium`.
5. Move data validation/serialization tests to Backend/data-tier tests.
6. Delete or rewrite tests that only prove cgo internals, unless they live in
   backend tests.
7. Update public docs to say pure-Go default is schema/codegen, backend is
   optional for validation/serialization.

Do not mix partial pure-Go code with cgo-backed behavior in the same public
methods. That creates the hybrid system this rebuild is meant to avoid.

## 19. Acceptance Bar

Before calling the rebuild successful:

- `CGO_ENABLED=0 go test ./cambium ./codegen ./compat` passes.
- `CGO_ENABLED=0 go vet ./cambium ./codegen ./compat` passes.
- dependency closure for `./cambium ./codegen ./compat` contains no
  `internal/libyang`, backend package, cgo package, or `import "C"`.
- no public type contains `unsafe.Pointer`, C pointer types, or backend handles.
- schema child order is held in slices and tested with cross-kind interleaving.
- no public traversal iterates a map.
- no builder assembles ordered children from goyang `Entry.Dir` or typed child
  slices.
- pure-Go augment placement has a golden fixture.
- existing data serialization goldens are clearly Backend/data-tier only.
- goyang-derived source has license notices, upstream SHA, NOTICE handling, and
  modified-file markings.
- every unsupported libyang-grade behavior is documented as unsupported, not
  faked.

## 20. Open Questions

Historical note: these were the open questions at planning time. Current status
for the implemented branch is tracked in
[`docs/handoff-codex-2026-06-20.md`](handoff-codex-2026-06-20.md); do not treat
this list as the active finish-line backlog without rechecking current source
and tests.

1. Does goyang's raw statement AST preserve declaration order in the exact API
   Cambium can depend on?
2. How much of goyang's resolver can be reused without inheriting map-order or
   kind-slice assumptions?
3. Should the optional backend live inside the same Go module or in a separate
   module?
4. Should `/spec/api.md` later split into `schema-api.md` and `backend-api.md`
   once implementation code lands? The tiering in current `/spec` is the
   prerequisite for starting.
5. Is pure-Go full validation worth a future dedicated engine, or should it stay
   backend-only permanently?

## 21. First Engineer Task List

1. Run Gate A against goyang's raw statement AST using the cross-kind fixture in
   section 10.
2. Record import-vs-fork and license handling before copying any upstream source.
3. Choose the optional backend package/module path.
4. Move or isolate existing cgo code so `go/cambium` can become cgo-free.
5. Commit the M0/M1 red tests, starting with dependency closure and
   `CGO_ENABLED=0`.
6. Implement only the M1 direct-schema floor.
7. Stop and update this document before broadening into grouping, augment,
   deviation, validation, or serialization.
