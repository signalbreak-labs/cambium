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

### Fixed

- `compat` augment order depended on map iteration when
  `IgnoreDeviateNotSupported` was set.
- An ignored `deviate not-supported` in `compat` could leave references to the
  kept node failing validation.
- Leafrefs into `choice`/`case` data failed with `target not found`.
- `unknown prefix` diagnostics were classified `unknown` instead of
  `invalid_identifier`.
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
