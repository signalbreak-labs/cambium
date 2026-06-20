# Go data-tree read/navigation/validation phase — handoff

> Current status note (2026-06-20): this is a historical handoff. Its
> "Deferred" section is superseded by
> [`docs/handoff-codex-2026-06-20.md`](./handoff-codex-2026-06-20.md).

What landed on `feat/cambium-sdk-phase1` for the Go data-layer parity phase
(per `docs/kimi-go-data-layer-prompt.md`), plus the post-review corrections.

## What landed

Kimi implemented the read/navigation/validation surface mirroring the Rust
`cambium-core` read side (commits `879a329`, `6ce8124`, `511a75c`, `c36b518`):

- cgo read primitives + `ValidateCollect` in `go/internal/libyang/libyang.go`
  (`cam_lyd_child`/`cam_lyd_get_value`/`cam_set_dnode` wrappers, `FindNode`,
  `XPathPaths`, `RootNodes`/`ChildrenOf`/`SiblingsOf`, `ValueStr`, `IsDefault`,
  `SchemaPtr`, generation counter).
- Public `NodeRef`, `NodeSet`, `DataSiblings`, `UserOrderedView`,
  `Value`/`Decimal64`, and `DataTree.Get/TryGet/Exists/Select/RootNodes` in
  `go/cambium/tree.go`.
- Generation-tagged stale-handle checks (`CAMBIUM_E0007`) on every accessor.
- Structured `ValidationErrors`/`Diagnostic`/`ValidationCode`/`ErrorType` in
  `go/cambium/validation.go`; `Validate` wraps a recoverable `*ValidationErrors`.

Verified green independently: `go test -race ./...`, `go vet`, `golangci-lint`
(0 issues), conformance 6/6 byte-identical, Rust oracle legs
(`data_read`/`data_validation`/`user_ordered_read`) pass.

## Post-review corrections (2026-06-14)

An adversarial review (cgo safety / generation-lifetime / Rust↔Go parity /
spec+RuleCode, each finding refuted against the libyang source + Rust oracle)
confirmed **3 of 9** findings (6 refuted). Manual spot-checks beforehand
confirmed the validation classification table is an exact 1:1 port of the Rust
`validation_code_from_apptag`, `Decimal64.String()` is the fixed form (no
double-minus), and the stale-handle guard is universal. **Fixed:**

- **GO-001 (high) — `UserOrderedListAt` accepted system-ordered lists.** The I1
  positional-only contract was not enforced at the API boundary; libyang only
  partially guards (`lyd_insert_before/after` reject a system-ordered node, but
  `lyd_insert_child` / `InsertLast` *silently succeeds*). Now gates on
  `schema.OrderedBy() == OrderedByUser` and returns `E0005` otherwise, mirroring
  the Rust oracle. Red-test-first regression added.
- **GO-FII-002 (medium, parity) — opaque nodes surfaced in Go but not Rust.**
  `collectSiblings` returned schema-less/opaque nodes (reachable under
  `ParseOnly+Opaque`) in `Children`/`Siblings`/`RootNodes`, while the Rust
  adapter gates on the node having a schema name. Identical input + API yielded
  different node sets. Now skips `cur.schema == nil`. Regression added (verified
  discriminating: pre-fix the opaque `<mystery>` child surfaced).
- **GO-002 (medium) — `codeForOp` missing read-op cases.** `children`,
  `siblings`, `schema`, `value` fell through to `RuleCodeUnknown` (E0000)
  instead of `E0006 DataPath`; `user ordered view` now maps to `E0005`. Pinned
  by a whitebox `codeForOp` test.

All Go-only fixes (no Rust touched). Full suite green after.

### Refuted (no change — examples of the suite being right)

- `lyd_get_value` on a non-term node: libyang's own `lyd_get_value` is already
  defensive (returns NULL for non-`LYD_NODE_TERM`); double-gated by `nodeIsTerm`.
- NodeSet/DataSiblings `Len`/`IsEmpty` lacking a stale check: they read only Go
  slices and mint NodeRefs with the snapshot gen, which assert on use — and Rust
  `len()` is likewise infallible, so this is faithful parity, not a gap.
- `Decimal64` `i64::MIN` overflow: `divisor >= 10` bounds `whole`; Go and Rust
  produce byte-identical output (empirically checked).

## Historical deferred list (superseded 2026-06-20)

At the time of this handoff, mutation CRUD (`new_path`/`set_value`/
`remove_path`/`add_defaults`), `diff`/`merge`/`duplicate`, LYB read,
`NodeRef.Ancestors`/`Traverse`, and the pre-existing `Identity.Bases()`
parsed-tree gap were deferred. This list is historical; see
[`docs/handoff-codex-2026-06-20.md`](./handoff-codex-2026-06-20.md) for the
current state.
