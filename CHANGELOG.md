# Changelog

All notable changes to this project will be documented in this file.
The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project follows semantic versioning for the Go module release line.

## [Unreleased]

### Added

- 156 more existing conformance cases opt into the `datatree` differential
  lane (`datatree = true`).
- The `ietf-interfaces` conformance case opts into the `datatree`
  differential lane.
- `cambium.schema-ir.v2` bounded node table (`Context.SchemaIRTable`,
  `SchemaIRNodeID`, `SchemaIRNoParent`), `Context.SchemaIRStats`, and
  `Context.SchemaIRWithLimit`, which fails with a `resource_limit` diagnostic
  instead of materializing an oversized v1 projection (ADR 0006).
- `cmd/cambium-ir -format v2` and a v1 `-max-records` limit (default
  1,048,576).
- `DeviationPolicy` and `ContextBuilder.SetDeviationPolicy`
  (`IgnoreNotSupported` keeps not-supported targets, decided before reference
  validation); `Deviation.Applied`, `Deviation.SourceLocation`,
  `LoadReport.DeviationPolicy`, and `LoadReport.IgnoredDeviations`.
- `LoadReport.OmittedContent()` and the `omitted_schema_content` diagnostic
  kind for vendor relaxations that drop declared content.
- `DefaultOrigin` and `DefaultValue.Origin()` (node, typedef, refine, or
  deviation).
- `SchemaChildren.ConfigOnly()` and `ProjectionOptions.ConfigOnly`.
- A `LoadReport` warning when an explicitly enabled feature is disabled by
  its own `if-feature`.
- Consumer contract tests (traversal, identity, types, loading, lifetime), a
  compiled integration example, and a check that the v2 table matches the
  conformance goldens' ordering.
- `RangeBound.MinNumber`/`MaxNumber` (`Number`) and, for length restrictions,
  `RangeBound.MinLength`/`MaxLength` (`uint64`), with `min`/`max` resolved
  against the type being restricted; `Min`/`Max` stay lexical.
- SchemaIR JSON (v1 and v2) `type.range` / `type.length` arrays carrying the
  resolved bounds as canonical decimal strings (additive under ADR 0002).
- `datatree.ParseModules` parses documents whose top-level nodes come from
  several modules (JSON_IETF `{"a:x":…,"b:y":…}`, or XML siblings in several
  namespaces). Top-level nodes are grouped by module in bytewise module-name
  order, each module's in schema order, as libyang orders them; `Validate`
  and `ApplyDefaults` cover every bound module, including leafrefs and
  `must`/`when` that cross modules. `Parse` still binds one module.
- Conformance case `multi-module-top-level-order` (I2) pins libyang's order
  for top-level nodes of several modules; it and `types-leafref-cross-module`
  run in the `datatree` differential lane.
- Must-reject conformance cases: `expect = "reject"` in
  `conformance/manifest.toml` marks a data document that a strict, validating
  parse must refuse. `cmd/cambium` asserts that libyang (and the `yanglint`
  oracle, when configured) refuses it, and `datatree-diff` asserts that
  datatree does too for `datatree = true` cases; verdicts are compared, not
  error texts. 24 such cases cover choice and case, `unique`, mandatory nodes
  under an absent container, leafref leaf-lists, canonical leaf-list
  duplicates, the decimal64 range, and plain mandatory, cardinality, pattern,
  range, duplicate-key, and `must` violations. Two accept cases cover data
  datatree used to reject (a mandatory leaf in an unselected case, duplicate
  `config false` leaf-list values).
- Generated code for a module with `anydata`/`anyxml` nodes has
  `ToXMLChecked() (string, error)` on every struct. It returns an error for an
  anydata/anyxml value parsed from JSON_IETF, which has no XML form, instead of
  writing an empty element.
- A weekly `Fuzz` workflow runs each native fuzz target cgo-free for 5 minutes
  and keeps any failing input as an artifact.
- `libyangbackend.ErrContextFrozen`, the sentinel for a module load after the
  context created a data tree.

### Changed

- `Build` rejects leafref cycles, including cycles through leafref union
  members, with `CAMBIUM_E0001`, a `semantic_schema_error` diagnostic, and a
  message listing the path chain; the first cycle in load order and schema
  order is reported. Vendor-compatible mode reports each cycle as a
  `LoadReport` warning instead. libyang v5.4.9 also rejects these ("circular
  chain of leafrefs").
- An unresolved augment or deviation target fails in every validation mode
  unless the enabled feature set excluded the target (its own `if-feature`, an
  enclosing `uses`, or a disabled declaring `augment`, matched by module);
  vendor-compatible mode no longer skips typos, wrong prefixes, or missing
  dependencies.
- `compat` maps `IgnoreDeviateNotSupported` to the native deviation policy and
  projects one ordered schema; with the option set it no longer shows
  feature-gated nodes or reorders augments.
- Schema diffs no longer build a v1 SchemaIR projection, so `DiffContexts`
  scales with unique nodes.
- Only implemented modules contribute augments and deviations (RFC 7950
  §5.6.5). An import-only module's augments and deviations were applied, and
  declaring a deviation made a module implemented; load an augmenting or
  deviation module explicitly for its effects. Modules that an implemented
  module's augment, deviation, or leafref paths name become implemented, and
  their own augments and deviations then apply.
- Strict loading rejects more RFC 7950 compile-time errors (each fails with
  `CAMBIUM_E0001`): import and include cycles; a module and submodule with
  different `yang-version`s; a YANG 1.0 module importing a YANG 1.1 module by
  revision; malformed leafref path predicates (unknown or repeated key, a
  right-hand side that is not a `current()/..` path or resolves to no leaf, a
  predicate on a non-list); `deviate add config`/`mandatory` over an explicit
  statement; a mandatory node directly in a choice's default case; same-module
  references from current to deprecated/obsolete (or deprecated to obsolete)
  typedefs, groupings, identities, and features; derived enumeration/bits
  restrictions in YANG 1.0; enum names that are empty or have leading or
  trailing whitespace; and an identityref default equal to its base.
  Vendor-compatible mode reports the cycles, version mismatches, and status
  references as warnings.
- `compat`: `IgnoreSubmoduleCircularDependencies` now has goyang's effect. A
  circular submodule include fails `Process()` without it; with it, `Process()`
  uses vendor-compatible loading and reports the cycle in `LoadReport()`.
- `datatree` holds leaf values in canonical form (decimal64 `45.50` → `45.5`,
  `0` → `0.0`; integers without sign or leading zeros; bits in position
  order), so `LeafValue`/`LeafListValues` and both output formats return
  canonical values; a value invalid for its type is kept as written.
- `datatree` sorts `ordered-by system` configuration lists and leaf-lists at
  parse time, so `Entries`/`LeafListValues` return canonical order.
- `datatree` XML identityref values resolve prefixes only against in-scope
  `xmlns` declarations, not the schema's import prefixes.
- The `datatree` differential lane parses each case against every implemented
  module (`ParseModules`), as the backend does, instead of guessing one module
  from the input, and reports a schema build failure as such.
- The `datatree-diff` lane runs `Tree.Validate` on every accept case before
  comparing output, so a flagged case fails if datatree rejects valid data.
- Codegen names every package-level identifier from one allocator per file,
  seeded with the runtime helper names. A colliding type family gets the
  smallest free numeric suffix, chosen in a canonical order (own-module nodes
  in schema order, then other modules by module name), so names depend only on
  the schema, not module load order. Schemas without collisions keep their
  names. Generated struct fields can no longer be named `ToXMLChecked` or
  `ToJSONIETFWithDefaults` (they get a `_` suffix, like `ToXML`).
- Generated identityref types no longer accept the base identity itself, only
  identities derived from it (RFC 7950 §9.10.2).
- Generated identityref JSON_IETF prefixes are relative to the module that
  defines the leaf, not the generated module (RFC 7951 §6.8). An identity from
  the leaf's own module is also accepted in its `module:identity` form.
- Generated `Validate` rejects anydata/anyxml content that is not a
  well-formed XML fragment (no XML declaration or DTD) or a single JSON value.
- Anydata/anyxml JSON parsed by generated code and nested more than 16 levels
  is written back compact instead of indented.
- CI pins every GitHub Action to a commit SHA, reads the Go version from
  `go/go.mod`, defaults to read-only `contents` permission (only the
  `conformance-artifact` job that attaches the release asset can write),
  verifies the gitleaks download checksum, and shuffles cgo-free test order.
- Codegen tests that build generated code run in parallel, and the
  context-deadline test no longer idles 5 s on `WaitDelay`.
- `scripts/green-bar.sh` no longer repeats the cgo-free vet and tests that
  `scripts/check-go-default-pure.sh` already runs.

### Fixed

- `compat` augment order depended on map iteration when
  `IgnoreDeviateNotSupported` was set.
- An ignored `deviate not-supported` in `compat` could leave references to the
  kept node failing validation.
- Leafrefs into `choice`/`case` data failed with `target not found`.
- `unknown prefix` diagnostics were classified `unknown` instead of
  `invalid_identifier`.
- An augment whose target another augment creates failed with `target not
  found` unless the creating augment happened to be applied first (within a
  module, or depending on `LoadModule` order). Augment targets now resolve to
  a fixpoint; contributions to one target keep module-load order, then source
  order. In vendor-compatible mode the local-name path fallback no longer
  claims a node before an exact target has been created.
- A `refine` `if-feature` was treated as a condition on the refine: with the
  feature disabled the target stayed in the schema and the refine's other
  properties were dropped. It now adds the `if-feature` to the target (RFC 7950
  §7.13.2), removing it from the effective schema when disabled, and augment or
  deviation paths into it are reported as feature exclusions.
- A `refine` of a grouping node excluded by the enabled feature set failed with
  `target not found`.
- Data nodes in different cases of a choice, or in a case and outside the
  choice, could share a name (RFC 7950 §6.2.1); they now fail as duplicate
  schema children.
- `Build` rejected an augment into another module when a mandatory node sat
  below a presence container, a list, or a case of the augment (for example
  `ietf-ip@2014-06-16`, RFC 7277). Only the augment's own nodes, through
  non-presence containers, are mandatory nodes (RFC 7950 §3, §7.17), as in
  libyang.
- A derived `range` or `length` using `min`/`max` over a restricted typedef
  failed as "not within the base restriction"; the keywords now stand for the
  restricted type's bounds (RFC 7950 sections 9.2.4 and 9.4.4).
- decimal64 range bounds outside the value space for the type's
  `fraction-digits` were accepted.
- `datatree` emitted `ordered-by system` lists and leaf-lists in input order
  instead of libyang's canonical order (I2); `ordered-by user` data keeps its
  exact input order.
- `datatree` identified list keys by local name, so an augmented child sharing
  a key's name could be ordered or uniqueness-checked as the key.
- `datatree` XML output of a foreign identityref lacked its `xmlns` prefix
  declaration, and instance-identifiers were not converted between XML
  prefixes and JSON_IETF module names (nor resolved for `require-instance`).
- `datatree` spent superlinear CPU parsing very long integer or decimal64 text;
  values over 256 characters are now rejected as invalid before parsing.
- `datatree` XML output escaped quotes in text and JSON_IETF output from XML
  input HTML-escaped `<`, `>`, and `&`, unlike libyang.
- `datatree` validated choice data as if every case were present: a mandatory
  leaf or `min-elements` list in an unselected case was reported, data from
  two cases of one choice and a missing mandatory choice passed, and
  `ApplyDefaults` filled defaults into every case. Only the selected case (or,
  with no case data, the default case) now applies (RFC 7950 §7.9).
- `datatree` never checked `unique` statements; they are now enforced,
  including leaves with a default value in use (RFC 7950 §7.8.3).
- `datatree` did not report a mandatory leaf, choice, or `min-elements` list
  under an absent non-presence container.
- `datatree` never checked leafref instances of leaf-list values, and compared
  leafref values by spelling, so an identityref leafref from another module
  (`"m:one"` against `"one"`) was falsely rejected.
- `datatree` rejected duplicate values in `config false` leaf-lists, which
  RFC 7950 §7.7.1 allows in state data.
- `datatree` accepted decimal64 values outside the range fraction-digits
  implies (for example `100.0` with `fraction-digits 18`).
- `datatree` `ApplyDefaults` ignored leaf-list defaults.
- Generated `UserOrderedVec` mutators edited a backing array shared with
  struct copies, so `Remove`, `InsertBefore`, `MoveBefore` or `InsertLast` on
  a copy could reorder or overwrite the original (I1). Every mutator now
  builds a fresh slice (copy-on-write); each mutation is O(n).
- Codegen dropped a leaf augmented into a list from another module when it
  shared a key's local name: list keys are now matched by module and name, so
  the leaf keeps its own field and field-order manifest entry after the keys,
  and never stands in for the key in sorting or duplicate-key validation.
  `CambiumMetadata` keys such a leaf, like any child that shares an earlier
  sibling's local name, as `module:name`, which also fixes the duplicate
  `case` that made same-named cross-module siblings fail to compile.
- Generated `bits` values were written in declaration order; they now list set
  bits in position order (RFC 7950 §9.7.2, matching libyang), including
  schema defaults. Parsing still accepts any order.
- Codegen gave distinct leaves with the same generated name one shared
  restriction, enumeration, bits, identityref or union type, merging their
  constraints. Every such leaf now has its own type.
- Generated code failed to compile when names collided across levels (for
  example `a-b/c` and `a/b-c`, a list `foo` and a container `foo-entry`, a
  node and a helper such as `CambiumStruct`), or when an enum name was not a Go
  identifier (`a/b`, `x+y`, `$`).
- When two other modules augmented the same local name into a struct, the
  one loaded first kept the plain field name and `CambiumMetadata` key; they
  are now ordered by module name. Root-level XML/JSON serializers read
  metadata by the plain wire name, so an imported top-level node sharing a
  local name with the module's own node was written with the wrong
  annotations.
- Generated anydata/anyxml `Validate` did not check content, so raw content
  could inject sibling XML or JSON. A deeply nested anydata value parsed from
  JSON_IETF re-serialized with quadratic indentation (54 KB in, 162 MB out).
- `scripts/check-go-default-pure.sh` listed dependencies only with
  `CGO_ENABLED=0`, which hides cgo files, so a dependency with a pure-Go
  fallback passed. It now also lists them with `CGO_ENABLED=1` and fails on any
  non-standard-library package with cgo files.
- `PUBLISHING.md` described a `0.1.0` release candidate and a deleted readiness
  note, and `go/internal/libyang/build.sh` referenced a nonexistent Rust build
  script.
- `libyangbackend` `LoadModule`/`LoadModuleFromPath` after the context had
  created a data tree let libyang recompile the schema under live trees, so a
  later `Serialize` failed and `Validate` crashed (SIGSEGV). The first data
  tree (`Parse`, `ParseOp`, `NewData`) now freezes the context permanently, and
  later loads fail with `ErrContextFrozen` (`CAMBIUM_E0001`), leaving the
  schema unchanged. Load every module before creating data.
- `LoadModuleFromPath` after `Close` now returns `ErrContextClosed` instead of
  passing a destroyed context to libyang.

## [go/v0.4.0] - 2026-07-02

### Added

- `gnmi` helper package emitting `ordered-by user` subtrees as one atomic
  JSON_IETF payload value (invariant I6), gated by the un-deferred
  `gnmi-ordered-atomic` conformance fixture.
- `cmd/cambium-ir`: pure-Go CLI exporting the versioned SchemaIR as JSON for
  downstream schema consumers.
- `SchemaIR.Errors` diagnostics field surfacing schema-rebuild failures;
  `LoadReport` gains the same rebuild-failure warning.
- Datatree XPath functions `re-match`, `bit-is-set`, `derived-from`, and
  `derived-from-or-self` (`deref` still skips).
- Datatree↔libyang differential conformance lane (`cambium datatree-diff`,
  per-case `datatree = true` opt-in) and a CI yanglint-oracle lane.
- Conformance corpus packaged as a versioned artifact
  (`scripts/package-conformance.py`, `conformance-artifact` workflow).
- Native Go fuzz targets for the pure parsers, characterization benchmarks for
  the hot paths, and OSV scanning for the vendored C submodules.
- `libyangbackend.UserOrderedList` staleness guard (`RuleCodeStale` after
  external tree mutations) and `libyang.ErrContextClosed` for post-`Close`
  context operations.
- Documented concurrency contract on `Context`, `DataTree`, `NodeRef`, and
  `UserOrderedList`, with race-detector stress tests.
- CI execution of zig/musl static Go test binaries for every test-bearing
  package in the module.
- Architecture decision records 0002–0005 (versioned SchemaIR export, payload-only
  gNMI helper, conformance-corpus authority, fail-closed FFI lifecycle), and a
  status-docs refresh reflecting the landed surfaces.

### Changed

- Every libyang FFI operation now pins its OS thread across the operation and
  its error retrieval, so diagnostics are always read from the correct
  per-thread error list; a validation failure with no retrievable diagnostics
  is now an explicit error instead of silent success.
- Deviate-type application order in `compat` is deterministic (declaration
  order, not map order).
- govulncheck scans the full module including the cgo tier; golden
  regeneration refuses to run when the repo-built yanglint oracle does not
  match the `/VERSIONS` pin.
- `conformance-tool.py gen` and `add` now share the same golden-generation path
  as the authoring library, including op-type and with-defaults handling.

### Fixed

- `RawDataTree.DiffApply` rejects diffs from a different context (undefined
  behavior in libyang), matching the existing `Merge`/`Diff` guards.
- Post-`Close` and concurrent-with-`Close` context operations are fail-closed
  instead of reaching freed C memory.
- `Context.NewData` after `Close` returns a closable tree shell whose
  context-dependent operations fail with `ErrContextClosed`; `libyangbackend`
  now re-exports the sentinel for public `errors.Is` checks.
- gNMI JSON_IETF atomic updates now reject predicated data paths with
  `RuleCodeDataPath`; callers must pass the list or leaf-list path so I6
  ordered values remain atomic.

## [go/v0.3.8] - 2026-06-26

Initial tracked release. See git history for earlier changes.
