# Changelog

All notable changes to this project will be documented in this file.
The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project follows semantic versioning for the Go module release line.

## [Unreleased]

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
