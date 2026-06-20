# Codex Handoff - 2026-06-17

> Historical checkpoint only. For current finish-line status, proof commands,
> and remaining parity risks, use `docs/handoff-codex-2026-06-20.md`.

## Context

The active objective was to audit the large overnight Kimi/Codex work, fix issues, and continue toward Cambium's north star: an order-correct, safer, up-to-date goyang successor for Rust first and Go parity.

This handoff is committed with the reviewed work so another machine can continue from a coherent checkpoint.

## What Changed

- CI now separates Go default no-cgo tests from cgo/libyang backend tests and uses Go 1.25 plus golangci-lint v9 config.
- Go cgo-only entrypoints/tests now have `//go:build cgo`; the default Go `cambium` and `codegen` packages remain cgo-free.
- The upstream vendored goyang dynamic-format vet warning is fixed and documented.
- Rust conformance runner now validates `schema-ir` `expected-ir` fixtures; all 200 conformance cases pass.
- Go conformance runner now mirrors the optional yanglint oracle check when
  `CAMBIUM_YANGLINT` is set; it remains skipped when the environment variable
  is absent.
- Rust schema introspection now covers:
  - imports and prefix resolution;
  - operation nodes and module RPC/action/notification views;
  - leafref raw path and target schema node;
  - defaults, must/when, extensions, unique constraints, grouping origin;
  - schema/data navigation helpers and kind predicates;
  - module `augmented_by`, `deviated_by`, and parsed deviation metadata;
  - node owner module/namespace for augmented children;
  - node-level deviation provenance;
  - context module enumeration plus builder `load_module_path` / `load_module_str`.
- Deprecated Rust `Context::load_module` now refreshes the materialized schema forest, so legacy users can call `schema()` after loading.
- Rust codegen now falls back to `String` for identityrefs whose imported identity module is not implemented.
- README now documents the pure-Go default and optional Go libyang backend boundary.

## Verification

Latest checks before commit:

- `cargo fmt --all`
- `cargo fmt --all -- --check`
- `cargo test --workspace`
- `cargo clippy --workspace --all-targets -- -D warnings`
- `CGO_ENABLED=0 go test ./cambium ./codegen`
- `CGO_ENABLED=0 go vet ./cambium ./codegen`
- `cargo test -p cambium-core --test context_loading`
- `cargo test -p cambium-core --test schema_introspection`
- `cargo test -p conformance-runner` (`conformance: 200 case(s) passed`)
- `git diff --check`

Earlier in this same checkpoint, these also passed:

- `CGO_ENABLED=1 go test ./...`
- `CGO_ENABLED=1 go vet ./...`
- `golangci-lint run`
- `scripts/diff-engine-config.sh`
- `go run ./cmd/cambium all` (`193 passed`)

## Continue From Here

- Re-run full Rust and Go gates on the target machine after checkout, especially if local generated `target/` artifacts differ.
- Continue spec/API parity review from `spec/api.md`, using Go schema tests as the parity oracle where Rust is still catching up.
- Follow-up progress after this checkpoint: the pure-Go default now has
  `Context.SetFeatures(module, features)` and evaluates YANG 1.1 `if-feature`
  expressions (`not`/`and`/`or`/parentheses, including prefixed feature refs)
  against the enabled feature set, including `if-feature` dependencies on
  feature declarations themselves. Invalid syntax, unknown local/prefixed
  feature references, and feature dependency cycles now fail context construction
  with `CAMBIUM_E0001`; boolean expressions now also fail closed in YANG 1.0
  sources unless the source declares `yang-version 1.1`. It also has
  `ContextBuilder` /
  `ContextFlags`, revision-pinned builder module loading, and builder-produced
  frozen contexts that reject post-build mutation with `CAMBIUM_E0001`;
  `ContextBuilder.LoadModuleStr` covers in-memory YANG source loading.
  `DisableSearchdirCwd` disables implicit `.` module lookup, `AllImplemented`
  marks implicit imports implemented for `Modules()`, default promotion covers
  leafref/augment/deviation targets, `RefImplemented` covers explicit-prefix
  `must`/`when`/default references, and `LoadModuleFromPath` registers the
  source file's directory for sibling imports/includes. Search paths ending in
  `...` recurse using goyang-compatible filename precedence, applying
  same-directory `name.yang`, then `name@revision.yang`, then child-directory
  precedence at each scanned directory; `SearchPaths` returns a copy and
  `UnsetSearchPath` is mutable-phase only. Public methods on
  a nil Go `*Context` now avoid panics: mutating methods and schema lookups
  return `CAMBIUM_E0001`, and snapshot lookups return empty results. Module
  feature introspection is exposed through `Module.Features()` and
  `Module.FeatureValue(name)`. The synthetic module root now preserves
  declaration-order interleaving across top-level data nodes, RPCs, and
  notifications in `Children()`, while `TopLevel()`, `RPCs()`, and
  `Notifications()` remain filtered ordered views; operation nodes expose direct
  `Input()` / `Output()` accessors over the same ordered IR. Included submodule
  top-level schema statements expand at the parent module's `include` statement
  site, including interleaved data, RPC, and notification statements; nested
  submodule-to-submodule includes are loaded and expanded at their own include
  site. Imports declared inside included submodules are retained in
  `Module.Imports()` and prefix resolution using that include-expanded order.
  YANG 1.0 submodules can reference sibling submodule definitions only through
  their own `include` chain, while YANG 1.1 submodules can reference
  definitions in all submodules included by the parent module without a direct
  sibling include.
  Pure-Go import/include
  resolution now honors `revision-date` pins instead of binding the latest
  same-name module; multiple
  revisions of the same module can coexist in one context, and each importer
  keeps its exact revision-bound prefix target. The parser adapter rejects
  oversized or excessively nested YANG input, rejects invalid UTF-8 and illegal
  RFC 7950 source characters before syntax parsing, and recovers upstream
  parser panics as ordinary load errors; file-based module loads check file size
  before reading full contents into memory. Included submodules are attached
  to the exact parent module revision that issued the include, not the context's
  latest same-name module. `Context.SchemaRevision(module, revision)` exposes an
  exact loaded-revision lookup while `Context.Schema(module)` keeps the
  latest/default lookup behavior; both `Schema` and `SchemaRevision` accept a
  module name or namespace. `Context.GetModule(name, revision)` and
  `Context.FindModuleByNS(namespace)` expose bool-returning lookup helpers that
  can return import-only dependency modules. `Context.Modules()` now matches the
  implemented-only enumeration contract; import-only dependencies remain retained for
  `Schema`, prefix resolution, and type/identity resolution. The deprecated
  legacy `SchemaTree` view now treats nil or zero-value `Find`/`PreOrder`
  receivers as empty instead of panicking. Schema node
  `config` now inherits from parent nodes, so descendants of `config false`
  state containers report `ConfigRo`/`ReadOnly()` unless explicitly changed by
  schema metadata. Leafref target/realtype resolution now uses a context-wide
  two-pass type-resolution walk and covers relative schema paths with `..`
  parent segments, cross-module absolute paths, predicates containing
  `current()`, and simple leading `deref()` schema-target paths in addition to
  same-module absolute schema paths; Go codegen has generated XML/JSON byte
  gates for absolute leafrefs, leaf-list targets, `require-instance false`,
  relative-parent leafrefs, `current()` predicate leafrefs, `deref()` leafrefs,
  and leafref-to-leafref chains. Go codegen also has generated XML/JSON byte gates
  for same-module identityref single-base, multiple-base, derived-hierarchy,
  foreign-module identityrefs, cross-module identity derivation, and
  cross-module multi-base identity derivation, including the
  `identityref-iana-if-type-foreign` corpus fixture; identity derived closures
  are now fully transitive and covered across import boundaries.
  Instance-identifier schema introspection preserves `require-instance`, and Go
  codegen now emits a generated `InstanceIdentifier` helper with separate
  XML/JSON lexical forms plus XML prefix namespace metadata; the three
  instance-identifier fixtures have generated XML/JSON byte gates, and the
  require-default fixture has generated JSON parse-and-reemit coverage. Empty
  leaf and binary fixtures now also parse-and-reemit their JSON goldens. Union codegen
  has generated XML/JSON byte gates for enum/scalar, member-resolution order,
  heterogeneous scalar quoting, leafref-member, identityref-member, nested
  typedef-chain, typedef-union composition, and two-distinct-identityref
  fixtures, with typed-union source assertions to prevent string-fallback false
  greens. Typedef codegen now has generated XML/JSON byte gates for simple base
  typedefs, two- and three-deep typedef chains, narrowed typedef restrictions,
  typedef default-inheritance data, and submodule typedefs referenced from the
  parent module. Go codegen now consumes flattened choice/case data children
  instead of dropping choice descendants, while still keeping list keys first.
  Generated XML/JSON byte gates cover top-level choice, nested choice-in-case shapes,
  shorthand leaf-list/list choice branches, user-ordered leaf-list choice
  branches, default choices with explicit non-default data, interleaved choice
  siblings, and choice leaves inside list entries. Go codegen also has generated
  XML/JSON byte gates for same-module, nested, and cross-module grouping
  expansion, refine mandatory/config effects, intra-module augment placement,
  and inter-module augment placement; augmented children owned by another
  module now emit the child XML namespace and JSON_IETF module-qualified member
  name. Generated serializers now canonicalize `ordered-by system` leaf-lists
  by scalar value and keyed system lists by key tuple while preserving
  `ordered-by user` vectors exactly; XML/JSON byte gates cover system leaf-list
  canonicalization, mixed user/system leaf-list ordering, numeric- and
  string-keyed system list canonicalization, nested user-ordered list/leaf-list
  data, and composite key-first field ordering. Empty, binary, uint8 range,
  enum/bits auto-position, and zero-valued enum fixtures now have generated
  XML/JSON byte gates as well. Deviation-aware codegen now has XML/JSON byte
  gates for `not-supported`, `add mandatory`, `delete default`, `replace type`,
  `replace default/config`, and combined deviation fixtures, generated from the
  deviated base module after loading the deviation source module. Import/include
  codegen has XML/JSON byte gates for prefixed imported typedefs, multiple
  imports, revision-pinned imports, simple and nested submodule grouping/typedef
  expansion, and submodule-owned foreign imports. Additional augment byte gates
  now cover container/leaf/leaf-list augments, nested augments, augmenting
  choice/case nodes, cross-module identifier-collision augments, augment `when`
  target-context placement, and a combined cross-module augment plus deviation
  fixture. Identifier safety gates now cover Go keywords, hyphen/underscore
  collisions, container/leaf naming edges, enum variant Go-identifier
  collisions, long names, and mixed-case YANG identifiers. JSON serializer
  gates now cover control/unicode string escaping, int64/uint64 quoting,
  decimal64 no-exponent canonical output, nested container objects, deterministic
  object member order, leafref/union resolved JSON forms, and
  instance-identifier XML-vs-JSON lexical strings. Representative schema-shape
  byte gates now cover empty presence containers, non-alphabetical declaration
  order, config-false state subtrees with uint64 JSON quoting, metadata/units
  leaves, nested opt-in action/notification fields, and wide heterogeneous
  sibling order with flattened choices and system leaf-list canonicalization.
  Additional valid data-codegen gates cover leaf-list defaults as explicit
  values, grouping expansion under config/state containers, refine default and
  refine presence/must metadata, keyless state-list positional preservation,
  standalone plus hierarchical identityref fixtures, owned RFC6991-style typedef
  unions, extension passthrough, valid constraint-shaped data, nested action
  documents, nested notification documents, generated parse-and-reemit for both
  nested operation shapes plus valid constraint-shaped data, and the real `ietf-interfaces`
  corpus fixture. Top-level RPC and notification codegen now emits standalone
  operation document structs with XML/JSON byte gates across rpc-order, RPC
  input/output/nested/heterogeneous/numeric/anyxml/coexistence, and notification
  fixtures; typed parser parse-and-reemit gates cover the JSON operation
  document fixture matrix. Anydata and anyxml codegen now emits a raw `AnyData` helper with
  separate XML/JSON forms; byte gates cover untyped anydata, opaque anyxml,
  namespaced anyxml XML payloads, and RPC anyxml XML payloads. RFC 7952 instance
  metadata codegen now emits a `MetadataAnnotation` helper plus per-struct
  `CambiumMetadata` maps for scalar annotations; the
  `metadata-annotation-rfc7952` generated byte gate covers XML attributes and
  JSON `@node` siblings. Generated Go structs now implement
  `ToJSONIETFWithDefaults(WithDefaultsMode)`, preserving existing explicit
  `ToJSONIETF()` bytes while covering RFC 6243 trim/report-all/report-all-tagged
  scalar default behavior; explicit leaf-list values equal to schema defaults
  are omitted under trim, and absent defaulted leaf-lists are materialized in
  report-all/report-all-tagged mode using the same `ordered-by` policy as
  explicit leaf-list values: default-statement order for user-ordered lists and
  canonical scalar order for system-ordered lists. Defaults under `choice`/`case`
  materialize only when their case is selected, or when that case is the choice
  default and no other case is selected. The
  `leaflist-with-defaults` generated gate covers the system-ordered path. The
  `json-ietf-with-defaults-modes` fixture has a generated byte gate across all
  four committed goldens. Schema defaults now resolve through effective type
  information before generated JSON_IETF with-defaults emission/comparison, so
  scalar defaults use canonical wire forms and ambiguous union defaults use the
  selected member's JSON wire form instead of string fallback quoting. Generated root document structs now
  include selected-module top-level data first, then direct imported modules'
  top-level data in import-statement order; `types-leafref-cross-module` has a
  generated XML/JSON byte gate proving a leafref target in the imported module
  and the referring leaf in the selected module serialize together. Imports
  declared in included submodules also contribute imported top-level document
  fields in that order. Root `Validate()` uses the same
  selected-plus-direct-import document source for flattened choice/case checks,
  so imported top-level choices are enforced on
  generated document structs. Generated Go structs now implement
  `Validate() error` with cgo-free typed-struct safety
  checks for list/leaf-list `min-elements` and `max-elements`, duplicate
  leaf-list values where YANG requires uniqueness, duplicate list key tuples,
  direct list `unique` tuples, recursive child validation, and
  flattened choice/case single-case plus mandatory-choice constraints, including
  nested choice constraints, scalar constraints on selected case descendants,
  mandatory descendants, and `min-elements`/`max-elements` collections only
  when their case is selected; a choice default case is treated as selected
  when no case data is present;
  generated reject gates cover `min-elements-reject`, `max-elements-reject`,
  duplicate single-key and composite-key lists,
  `constraints-unique-violation-reject`, and `choice-mandatory-reject`. The
  `constraints-range-length-reject` generated gate now ties valid XML/JSON bytes
  to range/length constructor rejections for out-of-bounds values. Generated root
  packages now expose `FromJSONIETF([]byte) (*Root, error)`, with a native JSON
  decode path for containers, lists, leaf-lists, empty, string, bool, integer,
  decimal64, numeric range newtypes, enum/identityref, bits,
  instance-identifier, ordered scalar union member trials, and JSON anydata
  values. The `json-ietf-parse-roundtrip` generated gate parses the committed
  input JSON and reserializes it to the XML/JSON goldens for int64 quoting,
  empty, identityref, and leaf-list arrays; user-ordered list/leaf-list fixtures
  now parse and re-emit with caller order preserved, while non-canonical
  system-ordered list/leaf-list inputs parse and re-emit in canonical order;
  cross-module generated root documents parse and re-emit imported top-level
  data in contract order;
  standalone/hierarchical/foreign
  identityref fixtures, including the real `ietf-interfaces` corpus fixture,
  now parse their JSON goldens back into generated structs. Generated
  identityref parsers now reject bare foreign-module identity names and require
  the JSON_IETF `module:name` spelling for identities outside the generated
  module. Decimal64, integer range, enum/bits, broad scalar, valid
  constraint-data, and typedef-chain union fixtures
  also parse their JSON goldens back into generated structs. Nested
  action/notification fixtures now parse generated root JSON back into typed
  structs and re-emit matching JSON. Generated JSON parsers enforce RFC7951
  numeric wire forms (`int64`/`uint64` quoted, smaller integers bare), including
  ordered union member trials. Union parsing now follows leafref realtypes when
  the target type is a union, and union members can carry `instance-identifier`
  values with XML namespace bindings. RFC 7952 metadata JSON siblings are parsed back into
  `CambiumMetadata`; the metadata fixture now proves JSON `@node` input
  reserializes to XML attributes and JSON metadata, and rejects `@node`
  siblings when the annotated data node is absent or the annotation name is not
  module-qualified/resolvable. Generated parsers reject
  oversized JSON_IETF input before decoding, excessive JSON nesting, unknown
  top-level or nested JSON members, duplicate object members, missing ordinary
  mandatory leaf fields, and list entries missing required key fields instead
  of silently dropping, overwriting, or zero-filling them. They also reject
  omitted unconditional non-presence containers that carry mandatory scalar
  descendants, and validate binary values as base64 with decoded-octet length
  constraints. Parsed anydata JSON is stored in the generator's
  deterministic pretty form so JSON
  parse-and-reemit matches the valid anydata goldens. Successful generated
  JSON parsers run `Validate()` before returning, so min/max-elements, duplicate
  list keys, direct `unique` constraints, recursive child checks, and flattened
  choice constraints, including nested choice constraints, scalar constraints on
  selected case descendants, mandatory descendants, and `min-elements`/`max-elements`
  collections of selected cases, are enforced at the parse boundary; direct generated
  `Validate()` also rejects invalid enum/identityref values, invalid base64
  binary values, decoded-length violations, decimal64 fraction/range violations,
  zero-value or otherwise invalid generated integer range/string length wrappers,
  string pattern violations including invert-match, duplicate generated bits
  constructor/parse input, invalid union
  enum/identityref/binary/decimal64 variants, restricted union string/integer
  range or length violations, union string pattern violations, and invalid
  direct `CambiumMetadata` maps that target unknown/absent data nodes or
  undeclared/unresolvable metadata annotations.
- Generated JSON_IETF decimal64 parsing now rejects malformed lexical strings
  such as empty values, bare decimal points, missing integer digits, or a
  trailing decimal point, and rejects raw fixed-point overflows before applying
  fraction/range checks.
- Generated string length wrappers now count Unicode characters rather than
  UTF-8 bytes, so cgo-free constructors, `Validate()`, and `FromJSONIETF` match
  YANG string `length` semantics for non-ASCII values.
- Generated RFC 7952 XML metadata attributes now use XML attribute escaping,
  including `"` to `&quot;`, rather than element-text escaping.
- Generated dynamic XML namespace attribute values for identityref and
  instance-identifier helpers now use the same XML attribute escaping.
- Generated schema-derived static XML namespace attribute values now also use
  XML attribute escaping and safe Go string-literal escaping.
  Restricted string/integer union members now use the generated length/range
  constructors before a JSON union trial can match; decimal64 generated parsers
  now enforce schema fraction-digits and range before returning; generated
  string parsers enforce full-string regexp pattern checks before returning.
  Top-level RPC and notification document structs now
  expose typed `From<Operation>JSONIETF` constructors with the same strict root
  and nested field checks. Union parse failures return an explicit error when no
  ordered member trial matches. Explicit JSON members under `choice`/`case`
  now validate their selected case before a parsed zero value can erase the
  presence signal; this rejects shapes such as an explicit empty non-presence
  container whose selected case has missing mandatory descendants.
- Imported typedef parsing now resolves the typedef body relative to the
  typedef's defining module, so imported union members retain their real base
  types and typedef names instead of degrading to `unknown`; schema and codegen
  regression gates cover the RFC6991-style `ip-address` union case.
- Pure-Go schema resolution now indexes scoped typedefs/groupings by lexical
  parent and resolves from the actual `type`/`uses` statement context, including
  grouped nodes whose type definitions come from the grouping's defining module.
  Scoped `typedef`/`grouping` declarations are indexed only under legal
  definition scopes (`grouping`, `container`, `list`, `rpc`, `action`, `input`,
  `output`, and `notification`); invalid parents such as `leaf`, `leaf-list`,
  `type`, `choice`, and `augment` now fail closed instead of leaking into
  lexical resolver maps.
  Direct known non-extension children of `grouping` are now limited to data
  nodes, `action`, `notification`, `uses`, nested `typedef`/`grouping`, and
  supported grouping metadata. Direct known non-extension children of `typedef`
  are now limited to `type`, `default`, `units`, `description`, `reference`,
  and `status`.
  Absent `yang-version` now defaults to YANG 1.0 for version-gated statements:
  `anydata`, `action`, notifications nested under data nodes, direct nested
  `choice` shorthand cases, `must` statements directly under operation
  `input`, operation `output`, or `notification`, boolean `if-feature`
  expressions, `if-feature` under `enum`, `bit`, `identity`, or `refine`,
  `pattern` `modifier` substatements, and multiple `base` statements on
  `identity` definitions or `identityref` type bodies now fail closed unless
  their source declares `yang-version 1.1`; `empty`/`leafref` union members,
  list key leaves with type `empty`, leaf-list default statements, explicit
  `require-instance` statements on `leafref` types, identifiers starting with
  `xml` in any case, and cross-module augments that add mandatory config nodes
  now have the same version gate. YANG 1.1 cross-module augments that add
  mandatory config nodes must have a direct `when`.
  `uses` substatement augments now apply to that expanded grouping instance
  only, appending children after the target's existing expanded children, while
  `refine` and `augment` nested anywhere other than directly under `uses` now
  fail closed; missing `refine`/`augment` target arguments fail closed early.
  Direct known non-extension children of `augment` are now limited to data
  definition nodes, `case`, `uses`, `action`, `notification`, `when`,
  `if-feature`, `status`, `description`, and `reference`. Direct `when`
  statements under `augment` are retained as effective conditions on augmented
  schema nodes.
  Direct known non-extension children of `refine` are now limited to the
  supported refine properties plus `if-feature`, and refine `description` /
  `reference` now replace the effective text metadata exposed by
  `SchemaNodeRef`.
  Top-level-only definition statements (`identity`, `feature`, and `extension`)
  now also fail closed when nested under schema/grouping statements instead of
  being silently ignored.
  Module/submodule-body-only statements (`belongs-to`, `contact`, `deviation`,
  `import`, `include`, `namespace`, `organization`, `revision`, and
  `yang-version`) now fail closed when nested under schema/grouping statements
  instead of being ignored by top-level loaders.
  YANG statement keywords, required standard statement arguments,
  identifier-valued arguments, and simple identifier-ref/QName arguments
  (`type`, `base`, `uses`) now fail closed before resolver/index construction
  if malformed, including quoted names with spaces and identifiers starting with
  digits. Identifiers starting with `xml` in any case now fail closed in YANG
  1.0 sources and are accepted in YANG 1.1 sources. Every known unprefixed
  standard statement except `input` and `output` now fails closed when its
  explicit argument is absent, while `input` and `output` fail closed if they
  carry arguments.
  Unknown unprefixed statement keywords now fail closed at any depth instead of
  being ignored; prefixed extension statements still resolve through the
  extension validation pass.
  Duplicate typedef/grouping/identity/feature/extension definitions and scoped
  shadowing collisions now fail the builder/schema path with `CAMBIUM_E0001`
  instead of silently overwriting resolver maps. Extension definition
  `description`, `reference`, `argument`, and nested `yin-element` metadata now
  fail closed on duplicates; `argument` is now valid only under `extension`,
  `yin-element` is valid only under `argument`, and `yin-element` accepts only
  `true` or `false`.
  Unknown prefixed extension statements anywhere in loaded module/submodule
  source now fail closed instead of being silently ignored or omitted from
  schema-node extension metadata. Extension instances now also fail closed when
  they omit a required extension argument or provide an argument to an extension
  definition that does not declare one.
- Resolver safety now also fails closed for unresolved typedefs, missing
  groupings, unmatched `uses`/`refine` target paths, unknown identity bases from
  both `identityref` and `identity` declarations, duplicate identity bases,
  YANG 1.0 multi-base identity/identityref declarations, typedef cycles, and
  identity cycles; these all return
  `CAMBIUM_E0001` through the pure-Go builder/schema path instead of partial
  unknown schema IR or recursive resolution.
  Public `Module.FindPath` misses now also return coded `CAMBIUM_E0001`
  context/schema errors instead of raw `fmt.Errorf` values. `Module.FindPath`
  also resolves already-materialized augmented children by their owning module
  prefix or module name when the receiver module has no import binding for that
  qualifier.
- List key and `unique` metadata now resolve fail-closed as well: duplicate or
  missing keys, keys that do not name direct child leaves, duplicate unique
  paths, and unique paths that do not resolve to descendant leaves return
  `CAMBIUM_E0001`. Key leaves with type `empty` now require `yang-version 1.1`;
  unique leaves with type `empty` still fail closed. Empty or misplaced
  `key`/`unique` statements fail closed too, including deviation-added/replaced
  `unique` properties. Legal descendant unique paths are resolved into ordered
  `UniqueConstraint` leaf handles. Config true lists
  now fail closed without a non-empty `key`; keyless lists remain valid only
  when their effective `config` is false. Key leaves now fail closed if they
  carry `if-feature` or effective `when` statements, or if their effective
  `config` differs from the list. `unique` constraints now fail closed if they
  mix config and state leaves.
- Built-in type bodies now fail closed for malformed required substatements:
  decimal64 requires exactly one valid `fraction-digits` in 1..18, identityref
  requires at least one non-duplicate base, leafref requires one non-empty path,
  union requires at least one member and permits `empty`/`leafref` members only
  in YANG 1.1, and enumeration/bits require at least one
  effective value. Enum/bit names and numeric values/positions are now checked
  for duplicates, malformed assignments, misplaced `value`/`position`
  substatements, and duplicate `value`/`position` substatements on one enum/bit
  before schema/codegen can proceed; enum/bit
  `description`, `reference`, and `status` metadata now fails closed on
  duplicates, and `status` accepts only `current`, `deprecated`, or `obsolete`;
  direct known non-extension children of `enum` and `bit` now fail closed unless
  they are the value/position statement, `if-feature`, or supported metadata;
  typedef-derived enum and bits restrictions now narrow the ordered value set
  while preserving base values/positions, and typedef-derived identityref
  restrictions now narrow the ordered base set to declared derived base
  identities;
  known type restriction substatements such as `range`, `length`, and
  `fraction-digits` now fail closed when attached to incompatible effective base
  types, and now fail closed when misplaced outside `type` bodies. `base` now
  fails closed unless it is under `identity` or `type`. Direct known
  non-extension children of `type` now fail closed unless they are valid
  type-body substatements or nested union member `type` statements.
  Typedef-derived `range`/`length` restrictions now fail closed when they widen
  the typedef base restriction;
  `range`/`length` `error-message`, `error-app-tag`, `description`, and
  `reference` metadata now fails closed on duplicates and is exposed through
  public `RangeBound` values;
  `leafref` and `instance-identifier` `require-instance` values now accept only
  absent, `true`, or `false`; explicit `require-instance` statements on
  `leafref` types now require YANG 1.1, while instance-identifier
  `require-instance` remains valid in YANG 1.0. Typedef-derived leafref and
  instance-identifier restrictions now preserve explicit `require-instance`
  overrides; config leafrefs with effective `require-instance true` now fail
  closed if they target state data.
- Leaf and leaf-list nodes now require exactly one `type` statement, and every
  typedef definition requires exactly one `type` statement whether or not it is
  referenced. Typedef type resolution now fails closed for unknown types, cycles,
  and invalid type-body restrictions even when the typedef is unused. Missing
  or duplicate type statements fail closed with `CAMBIUM_E0001`. Built-in YANG
  type names now resolve only in unprefixed form, so prefixed names such as
  `missing:string` and imported-prefix `ctpt:string` must resolve to typedefs
  or fail closed. `type` statements on non-leaf schema nodes now fail closed too,
  including deviation-introduced type replacements. Under known non-extension
  parents outside schema nodes, direct `type` statements now fail closed unless
  they are under `typedef`, `type` (union member), or `deviate`.
  Unused grouping bodies are now type-validated through a scratch tree, so
  missing/duplicate/unknown types and invalid type restrictions inside grouping
  definitions fail closed without changing public traversal order. Default
  placement/cardinality, duplicate sibling-name, and list `unique` leaf type
  rules now run over the same scratch grouping tree.
	- `range` and `length` restrictions now fail closed for empty/malformed
	  segments, repeated range/length statements, integer bounds outside the target
	  base type, negative/malformed length bounds, and invalid decimal64 lexical
	  forms such as exponents. Reversed segments whose lower bound is greater than
	  the upper bound now fail closed as well, and multi-segment restrictions must
	  be ordered and non-overlapping. Direct known non-extension children of
	  `range` and `length` now fail closed unless they are supported metadata.
	  String pattern `modifier` substatements now
	  fail closed unless they are singleton `invert-match` values in YANG 1.1
	  sources, and pattern expressions now fail closed unless they compile for
	  the native full-string regexp checks used by generated code; pattern
	  metadata children (`error-message`, `error-app-tag`, `description`,
	  `reference`) now fail
	  closed when duplicated. Direct known non-extension children of `pattern` now
	  fail closed unless they are supported metadata or `modifier`. Constraint
	  error metadata (`error-message`, `error-app-tag`) now fails closed outside
	  `must`, `range`, `length`, and `pattern`; `modifier` now fails closed
	  outside `pattern`. Pattern error-message, error-app-tag, description, and
	  reference metadata are exposed through the public `Pattern` handle.
- `min-elements`/`max-elements` metadata now fails closed for malformed
  `min-elements` values, malformed or zero numeric `max-elements` values, and
  finite `min-elements > max-elements`, including values introduced through
  `refine` or `deviation`. `min-elements 0` is valid. Deviation cardinality
  operations now enforce existence semantics: `add` rejects existing target statements, `replace`
  requires an existing target statement, and `delete` requires an exact matching
  value. Explicit `max-elements unbounded` is retained internally as an
  existing statement while the public accessor still reports no finite maximum.
- Scalar schema metadata now fails closed for malformed enum/bool values:
  `status`, `config`, `mandatory`, and `ordered-by` are validated instead of
  being coerced to defaults. Duplicate singleton schema metadata statements
  (`status`, `config`, `mandatory`, `ordered-by`, `presence`, `description`,
  `reference`, `units`, list `key`, and node `when`) now fail closed instead of
  using the first value; `refine` now applies the same singleton rule for
  `mandatory`, `config`, and `presence`. `units` now fails closed on non-leaf
  schema nodes, including when introduced by deviations; `mandatory` now fails
  closed unless it is on leaf, choice, anydata, or anyxml nodes, including when
  introduced by `refine` or deviations. `config` now fails closed unless it is
  on a data node, including when introduced by `refine` or deviations, and
  descendants of a `config false` node now fail closed if they set
  `config true`. `presence` now fails closed unless it is on a container, including when
  introduced by `refine`. `status` now fails closed unless it is on a data,
  choice, case, rpc, action, or notification node. Schema/operation
  `description` and `reference` now follow the same placement rule. Operation
  `input` and `output` nodes now fail closed unless they are under rpc or
  action nodes, `action` nodes now fail closed unless they are under container
  or list nodes, `notification` nodes now fail closed unless they are at module
  top level or under container/list nodes, `action` and `notification` nodes
  now fail closed with keyless list ancestors or `rpc`/`action`/`notification`
  ancestors, and nested `rpc` nodes now fail closed. RPC/action nodes now fail
  closed on duplicate `input` or `output`
  bodies. Direct known non-extension children of `rpc` and `action` are now
  limited to `input`, `output`, nested `typedef`/`grouping`, `if-feature`,
  `status`, `description`, and `reference`. Direct known non-extension children
  of operation `input` and `output` are now limited to data nodes, `must` in
  YANG 1.1 sources, `uses`, and nested `typedef`/`grouping`. Direct known
  non-extension children of `notification` are now limited to data nodes,
  `must` in YANG 1.1 sources, `uses`, nested `typedef`/`grouping`,
  `if-feature`, `status`, `description`, and `reference`.
  `case` nodes now fail closed unless they are under `choice` nodes.
  Direct known non-extension children of `choice` are now limited to shorthand
  data nodes, direct nested `choice` shorthand in YANG 1.1 sources, `case`,
  supported choice metadata, and `uses` only so the dedicated direct-`uses`
  rejection path can report the placement error.
  Direct known non-extension children of `case` are now limited to data nodes,
  `uses`, `when`, and supported case metadata.
  Data definition nodes now fail closed unless they are at module top level or
  under `container`, `list`, `choice`, `case`, `input`, `output`, or
  `notification` nodes, so scalar nodes and raw RPC/action operation bodies
  cannot acquire illegal ordered children.
  `uses` now follows the same fail-closed parent validation for expansion
  sites, while direct `uses` under `choice` is rejected rather than silently
  ignored by shorthand-case handling. Recursive grouping expansion through
  `uses` cycles now fails closed with `CAMBIUM_E0001` instead of overflowing the
  Go stack. Direct known non-extension children of `uses` are now limited to
  `augment`, `description`, `if-feature`, `reference`, `refine`, `status`, and
  singleton `when`; direct `when` under `uses` is retained as an effective
  condition on the expanded grouping instance's schema nodes.
  Known YANG statements outside the module/submodule top-level allowlist now
  fail closed instead of being ignored; extension-prefixed statements are left
  for extension-aware handling. `module` and `submodule` now fail closed if they
  appear anywhere other than as the parsed source root.
  Duplicate `must` metadata children (`error-message`, `error-app-tag`,
  `description`, `reference`) and `when` metadata children (`description`,
  `reference`) also fail closed; deviation-added/replaced `must` statements now
  use the same validated path. Direct known non-extension children of `must` now
  fail closed unless they are supported metadata; direct known non-extension
  children of `when` now fail closed unless they are `description` or
  `reference`. `must` now fails closed unless it is on a container, leaf,
  leaf-list, list, anydata, or anyxml node, including when introduced by
  `refine` or deviations. `when` now fails closed unless it is on a data, choice,
  or case node.
- Illegal defaults now fail closed after final schema materialization: leaves
  cannot carry multiple defaults, leaf-list default values cannot be duplicated,
  leaf-list defaults require `yang-version 1.1`, leaf-lists with
  `min-elements` greater than zero cannot default, mandatory leaves cannot
  default, and list key leaves cannot default; choice defaults must name an
  existing case and mandatory choices cannot default. Defaults on
  node kinds other than leaf, leaf-list, or choice now fail closed too. Typedef
  definitions with multiple defaults fail closed before default inheritance
  whether or not the typedef is referenced. Typedef default values are now
  validated against the typedef's effective type even when the typedef
  is unused. `refine` defaults are now singleton statements and preserve
  empty-string defaults exactly. Boolean defaults now fail closed unless they
  are `true` or `false`; integer defaults now fail closed unless they parse
  within the effective base type and range restriction; decimal64 defaults now
  fail closed unless they satisfy the effective `fraction-digits` and range
  restriction; enumeration defaults now fail closed unless they name an
  effective enum value not marked with `if-feature`; bits defaults now fail
  closed unless they name effective bit values not marked with `if-feature`,
  without duplicate tokens; string defaults now fail closed unless they satisfy
  effective length restrictions; binary defaults now fail
  closed unless they are base64 and satisfy effective decoded length
  restrictions; identityref defaults now fail closed unless they resolve to an
  identity derived from the effective base set; `empty` types now fail closed if
  a default is present; union defaults now fail closed unless at least one
  effective member type accepts the value; leafref defaults now validate
  against the resolved target real type when resolvable.
- Duplicate same-module sibling schema nodes now fail closed after
  augment/deviation materialization. The check keys by owner module plus local
  name, so cross-module augment collision cases with the same local name remain
  representable in ordered IR.
- Included augment/deviation statements whose target paths cannot be resolved
  now fail closed with `CAMBIUM_E0001` instead of silently producing partial IR.
- Deviation statements now require at least one `deviate`, and unknown
  `deviate` operation names fail closed with `CAMBIUM_E0001`; only
  `not-supported`, `add`, `replace`, and `delete` are accepted. Deviation
  `deviate` statements are now valid only directly under `deviation`, so
  misplaced `deviate` statements fail closed instead of being ignored.
  Direct known non-extension children of `deviation` are now limited to
  `deviate`, `if-feature`, `description`, and `reference`.
  `description` and `reference` metadata now fail closed when duplicated.
  Direct known non-extension children of `deviate` now fail closed according to
  the operation: `not-supported` has none, `add` accepts additive properties,
  `replace` accepts replacement properties including `type`, and `delete`
  accepts removable properties only. Valid extension-prefixed substatements
  under `deviate` stay sidecar extension instances and no longer appear as
  synthetic `Deviation.Property()` entries.
  `deviate add units` now fails closed if the target already has `units` instead
  of silently doing nothing, and `deviate replace units` now fails closed when
  the target has no existing `units`. `deviate delete units` now requires a
  matching target `units` value. `deviate add default` now rejects an
  already-present target default value, `deviate replace default` now requires
  an existing target default, and `deviate delete default` now requires a
  matching target default value. `deviate replace must` and `deviate replace
  unique` now require existing target constraints; `deviate delete must` and
  `deviate delete unique` now require matching target constraints.
- Import tables now fail closed when an import has no prefix or when two imports
  in the module/include source set use the same prefix, preventing silent prefix
  map overwrite during QName resolution. Import prefixes that collide with the
  module's own prefix or self-resolving module-name alias now fail closed as
  ambiguous. Duplicate `prefix` children inside one `import` statement also
  fail closed before dependency lookup. Nested `prefix` statements are now valid
  only under `import` or `belongs-to`, so misplaced nested prefixes fail closed
  instead of being ignored. `revision-date` is now valid only under `import` or
  `include`; misplaced `revision-date` statements fail closed. Direct known
  non-extension children of `import` are now limited to `prefix`,
  `revision-date`, `description`, and `reference`; direct known non-extension
  children of `include` are limited to `revision-date`, `description`, and
  `reference`, so invalid dependency children fail before dependency lookup.
- Module loads now fail closed when a `module` statement is missing required
  `namespace` or `prefix` metadata, and when a namespace is reused by a
  different module name in one context.
- Conflicting duplicate module identity loads with the same module name and
  revision now fail closed instead of overwriting the prior module.
- Submodule loads now fail closed when `belongs-to` is missing its required
  `prefix` child; direct known non-extension children of `belongs-to` are
  limited to `prefix`.
- Module and submodule `yang-version` statements now fail closed when they have
  standard non-extension children.
- Scalar statements with no standard substatements, including module header
  fields such as `namespace`, `prefix`, `contact`, `organization`, and common
  metadata/value statements such as `description`, `reference`, `default`,
  `status`, and `units`, now reject known non-extension children.
- Module and submodule `revision` statements now fail closed for malformed
  calendar dates and duplicate revision values before the source enters the
  context. Import/include `revision-date` pins also fail closed for malformed
  dates and duplicate pin statements before dependency lookup. Revision
  `description` and `reference` metadata now fail closed when duplicated, and
  direct known non-extension children of `revision` are limited to those
  statements. Module revision statements are exposed in declaration order
  through `Module.Revisions()`. Import/include `description` and `reference`
  metadata also fail closed when duplicated, and import description/reference
  metadata is exposed through the public `Import` value returned by
  `Module.Imports()`; direct module include metadata is exposed through the
  public `Include` value returned by `Module.Includes()`.
- Module names passed to `ContextBuilder.LoadModule` / `Context.LoadModule`
  now fail closed for malformed identifiers before search-path lookup, so path
  traversal and explicit path loads stay confined to `LoadModuleFromPath`.
  Files found through module-name lookup or import resolution must declare the
  requested `module` name before they can enter the context; submodules load
  only through `include` resolution. Revision-suffixed filenames are selected
  only when the source's effective/latest declared revision matches the filename
  date; recursive `...` lookup honors direct filename precedence within each
  directory before descending into child directories; explicit revision pins now
  also fail closed on a mismatched
  revision-suffixed candidate instead of falling back to `name.yang`. Failed
  module loads now roll back newly visible module state at the context boundary,
  so dependency/import/include errors do not leave the requesting module
  available through later schema lookups; builder loads that set features inline
  also roll back those feature mutations if the module load fails.
  Direct path/string loads now also reject valid submodule sources after
  metadata validation, so they cannot populate parent placeholders or bypass
  the parent's `include` chain. Failed direct path loads roll back any
  search-path directory added only for that load. `SetFeatures` now rejects
  malformed module and feature names before mutating feature state.
  Explicit revision strings passed to `ContextBuilder.LoadModule` now fail
  closed for malformed calendar dates before search-path lookup.
- Module/submodule header metadata now fails closed for duplicate singleton
  statements: module `namespace`/`prefix`, module or submodule `yang-version`,
  submodule `belongs-to`, `belongs-to` `prefix`, and module/submodule top-level
  `contact`, `organization`, `description`, and `reference`. `yang-version`
  accepts only `1` or `1.1`. Direct `belongs-to` now fails closed in modules,
  and direct `namespace`/`prefix` now fail closed in submodules.
- `if-feature` expressions now fail closed for invalid syntax, unknown
  local/prefixed feature references, and feature dependency cycles before schema
  materialization; declared-but-disabled features still filter guarded statements
  without producing diagnostics. Enabled feature names now also fail closed if
  they do not resolve to declared module-local features once the module is
  loaded. `SchemaNodeRef.IfFeatures()` now reports direct node plus applied
  `uses`, `augment`, and `refine` `if-feature` expression strings after
  feature filtering, and generated implicit cases for choice shorthand children
  inherit the shorthand child feature expressions. Direct
  `if-feature` expression strings are exposed through `Feature`, `Identity`,
  `EnumValue`, schema-node `Extension`, and `Deviation` handles after feature
  filtering. Feature, identity, typedef, grouping, and extension `description`
  and `reference` metadata now fail closed when duplicated. Feature, identity,
  typedef, grouping, and extension `status` metadata now fails closed on
  duplicates or values other than `current`, `deprecated`, and `obsolete`.
  Feature `status` metadata is exposed through the public `Feature` handle.
  Extension definition `argument`, `yin-element`, `description`, `reference`,
  and `status` metadata are exposed in declaration order through
  `Module.ExtensionDefinitions()`.
  Module-level typedef definition `type`, `units`, `default`, `description`,
  `reference`, and `status` metadata are exposed in declaration order through
  `Module.TypedefDefinitions()`. Module-level grouping definition direct child
  names plus `description`, `reference`, and `status` metadata are exposed
  through `Module.GroupingDefinitions()`.
  Enum and bit `description`, `reference`, `if-feature`, and `status` metadata
  are exposed through the public `EnumValue` handle. Public resolved type slice
  metadata now returns defensive copies, including range/length/pattern,
  enum/bit value, identityref base, and union member slices.
  Direct known non-extension children of `identity`, `feature`, and `extension`
  are now limited to their supported definition-body statements. Identity
  `description`, `reference`, and `status` metadata are exposed through the
  public `Identity` handle. Module header `organization`, `contact`,
  `description`, and `reference` metadata are exposed through the public
  `Module` handle.
  Direct known non-extension children of `argument` are now limited to
  `yin-element`.
- Keep TDD discipline: add a failing test first for the next missing contract, then implement narrowly.
- Follow-up progress after the static namespace escaping slice: generated Go
  validation now rejects malformed or reserved dynamic XML namespace prefixes
  before they can be emitted as `xmlns:` attribute names. The guard covers
  instance-identifier helpers, instance-identifier union variants, and RFC 7952
  metadata annotations; namespace values continue to use XML attribute escaping.
- Generated Go JSON bits parsing now rejects tab/newline-style separators
  instead of accepting every `strings.Fields` delimiter; ordinary bits and
  union-bits JSON parser branches share the same lexical helper.
- Generated Go `FromJSONIETF` parsers now reject invalid UTF-8 byte input before
  JSON decoding, avoiding `encoding/json` replacement-character normalization
  for malformed string contents.
- Generated Go JSON parsing now also rejects escaped/decoded string code points
  libyang rejects for YANG strings, including invalid C0 controls, surrogate
  code points, and Unicode noncharacters.
- Generated Go binary validation now rejects whitespace inside base64 lexical
  values before decoding, so direct `Validate()` and `FromJSONIETF` do not
  accept newline-split binary values.
- Follow-up correction: generated `FromJSONIETF` now matches libyang's binary
  parser tolerance for PEM newlines every 64 base64 characters and canonicalizes
  those values before storing/re-emitting; direct typed-string validation remains
  canonical-only.
- Generated `FromJSONIETF` quoted integer parsing now matches libyang's numeric
  string tolerance for surrounding JSON whitespace; unsigned integer strings
  also canonicalize libyang-compatible `+N` and `-0` forms before re-emitting.
- The pure-Go YANG lexer now terminates unquoted arguments before immediately
  adjacent `//` and `/* */` comments, fixing an upstream goyang parser TODO that
  previously tokenized comment text as part of the argument. It also rejects a
  standalone `*/` sequence inside unquoted arguments instead of accepting
  invalid RFC 7950 source in free-text statements such as `description`.
- Pure-Go `if-feature` evaluation now treats YANG 1.0 single references named
  `not`, `and`, or `or` as ordinary feature names instead of boolean operators;
  YANG 1.1 boolean expressions continue to use the operator grammar.
- Module-level pure-Go `augment` and `deviation` targets now fail closed when
  their arguments are not valid absolute schema-nodeids, so relative-looking
  paths such as `target:top` and normalizable trailing slashes are rejected
  before target resolution.
- Pure-Go `uses` substatement `augment`/`refine` targets now validate descendant
  schema-nodeid syntax before matching, and `uses` augment/refine target
  matching now honors prefixes instead of stripping prefixes for augment targets
  or comparing raw prefixed refine targets against local names.
- Pure-Go `refine` now refreshes ancestor list key/unique validation after
  mutating a target node, so refine-introduced key config mismatches and related
  list constraint changes fail closed instead of leaking through the initially
  built grouping tree.
- Pure-Go `refine`/deviation config changes now recompute descendant effective
  `config` values from the final parent value, so a grouping subtree refined to
  `config false` exposes read-only descendants instead of retaining stale
  writable build-time inheritance.
- Pure-Go list keylessness is now validated against final effective `config`
  after `refine`/deviation processing, so a keyless list inside a grouping
  subtree refined to state data is accepted while direct config-true keyless
  lists still fail closed.
- Pure-Go keyless lists are now accepted in RPC/action input/output and
  notification payload subtrees, matching the YANG rule that only
  configuration-data lists require keys; top-level config true keyless lists
  still fail closed.
- Pure-Go list key `if-feature` validation now distinguishes direct or
  `refine`-applied key conditions from inherited `uses`/augment gating, so a
  feature-gated grouping that contains a valid keyed list remains valid while
  keys that carry their own `if-feature` still fail closed.
- Pure-Go `unique` config/state classification is now evaluated against final
  effective `config`, so a grouping list that initially mixes config/state but
  is refined into an all-state subtree is accepted while final mixed
  config/state unique constraints still fail closed.
- Pure-Go list key config matching is now evaluated against final effective
  `config`, so grouping-provided state key leaves become valid when an ancestor
  `refine config false` turns the list into state data; final config/key
  mismatches still fail closed.
- Pure-Go leafref config/state validation now uses the same configuration-data
  classification as list key requirements, so RPC/action input/output and
  notification payload leafrefs may target state data while ordinary config
  leafrefs with `require-instance true` still fail closed.
- Go codegen now uses `SchemaNodeRef.RepresentsConfigurationData()` for
  generated leaf-list duplicate-value validation, so YANG 1.1 RPC/action and
  notification payload leaf-lists may preserve duplicate values while config
  leaf-lists still enforce uniqueness.
- Pure-Go cross-module augment mandatory-node checks now use semantic
  configuration-data classification, so mandatory RPC/action input/output and
  notification payload augments are not rejected as mandatory config augments.
- Pure-Go `unique` config/state classification now also uses semantic
  configuration-data classification, so RPC/action input/output and notification
  payload unique constraints do not fail merely because one payload leaf has raw
  `ConfigRw` inheritance and another has `config false`.
- Go codegen now emits response-shaped `RPCOutput` document structs for
  module-level RPCs that define both `input` and `output`, while preserving the
  existing request-shaped `RPC` struct and the legacy `RPC` name for output-only
  RPCs.
- Public pure-Go `Module.FindPath` now rejects malformed or relative-looking
  schema paths before resolution, so inputs such as `module:top` or trailing
  slash paths cannot be normalized into valid schema nodes.
- Generated Go JSON_IETF parsers now sort object keys before unknown-field
  checks, so malformed inputs with multiple unknown members produce stable
  lexicographic diagnostics instead of Go map-order dependent errors.
- Added a cgo-free `go/compat` read-only Entry projection for goyang migration
  code. A cgo-free `Modules` facade now provides goyang-style `NewModules`,
  `AddPath`, `Parse`, `Read`, `Process`, `GetModule`, `FindModule`, and
  `FindModuleByNamespace` helpers over Cambium's pure-Go builder. `FindModule`
  resolves goyang-style import/include AST nodes and honors exact
  `revision-date` pins before falling back to bare latest module lookup when
  the requested revision is absent. `FindModuleByNamespace` now mirrors
  goyang's parsed-record lookup behavior before `Process`, so migration code
  can inspect a known namespace even when other imports/includes are not yet
  loadable. `Process` now supports goyang-style in-memory `Parse` sets
  containing modules and submodules by internally staging those parsed sources
  for the builder's ordinary include/import search path; Cambium's public
  direct-submodule load rejection remains unchanged.
  `AddPath` accepts path-list strings, suppresses duplicate paths, and supports
  `dir/...` recursive lookup with direct-directory filename precedence before
  descending. Compat module records expose `Namespace`, `Prefix`, `Contact`,
  `Description`, `Organization`, `Reference`, `YangVersion`, `Revision`,
  `Import`, `Include`, top-level parser-backed AST slices (`Anydata`,
  `Anyxml`, `Augment`, `Choice`, `Container`, `Deviation`, `Extension`,
  `Feature`, `Leaf`, `LeafList`, `List`, `Notification`, `RPC`, and `Uses`),
  statement-backed `Source`, `Kind()`, `ParentNode()`, `NName()`,
  `Statement()`, `Exts()`, `Groupings()`, `Typedefs()`, `Identity`,
  `Identities()`,
  `Current()`, `FullName()`, and `GetPrefix()` values populated from parser
  metadata or Cambium's module handle. `Module.Import` and `Module.Include`
  nodes now also carry goyang-shaped parser `Statement()` / `ParentNode()` /
  `Source()` metadata. Module scalar `Value` nodes (`Namespace`, `Prefix`,
  `YangVersion`, `Organization`, `Contact`, `Description`, and `Reference`)
  now also retain parser `Statement()` / `ParentNode()` / `Source()`
  metadata before and after `Process`, and `Module.Revision` nodes retain their
  parser statement metadata with `Description` / `Reference` value children
  pointing back to the revision. Top-level `Module.Anydata`, `Module.Anyxml`,
  `Module.Leaf`, `Module.LeafList`, and `Module.List` AST nodes now also
  expose parser-backed `Type`, `if-feature` and `must` children, defaults,
  keys, cardinality, unique constraints, `must` statements, nested list leaf
  type metadata, and list-scoped action / notification children, and nested
  `Container`, `Choice` / `Case`, `Augment`, `Notification`, `Grouping`,
  `Typedef`, `Uses` / `Refine`, `Action`, and RPC `Input` / `Output` child
  slices now carry parser-backed children and parent/source metadata. Definition
  and deviation metadata under `Module.Extension`, `Module.Feature`,
  `Module.Deviation`, `Module.Identity`, `Deviation.Deviate`, and submodule
  `BelongsTo.Prefix` now also carry parser-backed parent/source metadata.
  `Modules.ParseOptions` plus
  `Options`/`DeviateOptions`/`DeviateOpt` preserve goyang call shapes.
  Package-level parser/AST helpers `Parse`, `Statement`, `Node`, `ErrorNode`,
  `Source`, `Typedefer`, `RootNode`, `FindModuleByPrefix`,
  `MatchingExtensions`, `MatchingEntryExtensions`, `FindGrouping`, `NodePath`,
  `FindNode`, `ChildNode`, `PrintNode`, `PathsWithModules`, and `CamelCase`
  are now exposed for cgo-free
  goyang migration code; `Parse` uses Cambium's bounded parser wrapper instead
  of raw upstream parsing. `RootNode` and `FindModuleByPrefix` return the
  compat `Module` facade while still accepting parser AST nodes, including
  imported prefixes already linked by an upstream-style AST `Modules.Process()`
  call. `ToEntry`
  now accepts compat module nodes and source-backed parser AST
  module/statement nodes; `FromModule` accepts Cambium module handles.
  `FindGrouping` now resolves local, imported, and included grouping
  definitions through the compat module facade instead of depending on
  upstream-only `Import.Module` / `Include.Module` links.
  Loader-backed compat modules return the full ordered Cambium projection;
  AST-only conversion walks ordered `Statement.SubStatements()` and fails closed
  when no ordered source statement is available rather than reconstructing order
  from kind-separated typed AST slices. Non-conflicting parser AST node structs are exposed
  as aliases; names that collide with the compat projection use `ASTValue`,
  `ASTModule`, and `ASTIdentity`; `ASTModule` is for explicitly constructing
  raw parser nodes, not the helper return type.
  `Value` carries goyang-style `Parent`, `Source`, `Extensions`, and
  `Description` fields and implements the parser `Node` shape.
  Projected `Identity` values carry goyang-style `Parent`, `Source`, and
  `Extensions` fields and also implement the parser `Node` shape.
  `Modules.ClearEntryCache()` clears compat-owned cached `Entry` projections
  so repeated `ToEntry` calls match goyang's cache lifecycle without making the
  cache a traversal authority.
  `Entry.Exts` is populated from Cambium schema extension instances in
  declaration order, and `MatchingEntryExtensions` supports the goyang
  entry-extension helper shape, including imported extension modules whose
  projected keywords use the defining module name instead of the source prefix,
  without using extension metadata as a traversal source.
  `Number`/`YangRange` now expose goyang-style constructors, parsers,
  validation/containment helpers, numeric constants, and built-in integer
  ranges (`FromInt`, `FromUint`, `FromFloat`, `ParseInt`, `ParseDecimal`,
  `ParseRangesInt`, `ParseRangesDecimal`, `Frac`, `Int8Range` through
  `Uint64Range`) for migration code.
  `Entry.Modules()` returns the owning compat facade for loader-projected
  entries, while `Entry.Augment()`, `Entry.ApplyDeviate()`, and
  `Entry.FixChoice()` are no-op compatibility hooks because Cambium applies
  augments, deviations, and choice materialization before projection.
  `Entry.Dir` is a lookup cache only; ordered traversal uses `Entry.Children()`
  backed by Cambium's module-root/schema-node ordered IR.
  `Entry.Node` is now exposed as a goyang-style parser `Node`; Cambium-specific
  schema handles used by helpers such as `Namespace()`, `Path()`, and
  `GetWhenXPath()` remain internal so migrated code can compile against the
  expected field type without making parser nodes a traversal authority.
  `Entry.Find` supports goyang-style slash paths, absolute paths, `"."`/`".."`
  and prefix-qualified parts through direct lookup without map-order traversal.
  The projection now also carries common goyang-style read fields including
  description, defaults, units, config/mandatory tristates, key/list metadata,
  scalar type names, RPC input/output handles, augment/deviation/uses metadata
  fields (`Augments`, `Augmented`, `Deviations`, `Deviate`, and `Uses`), and
  `Entry.Namespace()`. `UsesStmt`, `DeviatedEntry`, and the `Deviation*`
  constants are present for goyang source-shape compatibility; these fields
  are metadata surfaces and are not traversal authorities.
  Module-root `Identities`, `Entry.GetErrors()`, `Entry.Annotation`, and
  `NewDefaultListAttr()` are present for migration code, and
  `Entry.InstantiatingModule()` resolves projected entries back to their module
  name while ordered traversal remains exclusively `Entry.Children()`.
  `Entry.GetWhenXPath()` exposes the first effective `when` expression as a
  goyang-style convenience over Cambium's validated `Whens()` metadata.
  `Entry.Print(w)` renders through `Entry.Children()`, so human-readable compat
  output preserves Cambium declaration order rather than sorting `Entry.Dir`.
  `TriState.String()` and `EntryKind.String()` now match goyang spellings.
  `YangType` now projects goyang-compatible `TypeKind` values and common
  type metadata including defaults, units, range/length restrictions, string
  patterns, enum/bit name-value maps, identityref base identities, leafref
  path/optional-instance, and union member types from Cambium `TypeInfo`.
  Compat now also exposes the parser fork's goyang-shaped `BaseTypedefs` table,
  and projected `YangType` values carry goyang-compatible `Base` and `Root`
  fields. `TypeInfo.TypedefChain()` exposes typedef-derived names from the
  outer referenced typedef to the innermost typedef before the final built-in
  base, and compat `YangType.Base` projects that intermediate chain in
  goyang-compatible parser type shape. `YangType.Root` reflects the final
  built-in base known through `TypeInfo`.
  `YangType.Equal` is implemented for name-insensitive comparison of projected
  type metadata and ignores `Base`/`Root`, matching goyang's current comparison
  behavior.
  `SchemaNodeRef.TypeDefaultValue()` now exposes inherited typedef defaults
  separately from explicit schema node defaults so compat `YangType.Default` /
  `HasDefault` match goyang's type-default semantics.
  `go/compat` now includes an executable API parity guard against the vendored
  goyang v1.6.3 package for exported top-level names, exported struct field
  names/types/tags, non-struct type declarations, const/var declaration shape,
  public function signatures, and public method sets.
  Compat-owned `Module`, `Value`, and `Identity` nodes now retain goyang
  `yang` tags, and a companion behavioral differential checks reflection-based
  AST helper traversal, module-loader lookup behavior, common `Entry` metadata,
  and `YangType` projection semantics while preserving Cambium-only
  improvements such as declaration-order `Children()` traversal and richer
  units metadata.
