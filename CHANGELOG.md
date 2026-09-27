# Changelog

All notable changes to this project will be documented in this file.
The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project follows semantic versioning for the Go module release line.

## [Unreleased]

### Added

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
- `datatree.ParseModules` parses documents whose top-level nodes come from
  several modules (JSON_IETF `{"a:x":…,"b:y":…}`, or XML siblings in several
  namespaces). Top-level nodes are grouped by module in bytewise module-name
  order, each module's in schema order, as libyang orders them; `Validate`
  and `ApplyDefaults` cover every bound module, including leafrefs and
  `must`/`when` that cross modules. `Parse` still binds one module.
- Conformance case `multi-module-top-level-order` (I2) pins libyang's order
  for top-level nodes of several modules; it and `types-leafref-cross-module`
  run in the `datatree` differential lane.

### Changed

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

### Fixed

- `compat` augment order depended on map iteration when
  `IgnoreDeviateNotSupported` was set.
- An ignored `deviate not-supported` in `compat` could leave references to the
  kept node failing validation.
- Leafrefs into `choice`/`case` data failed with `target not found`.
- `unknown prefix` diagnostics were classified `unknown` instead of
  `invalid_identifier`.
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
