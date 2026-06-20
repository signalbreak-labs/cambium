# Handoff — Cambium Go parity Phase 4 (Slice 1 + Slice 2 success floor)

> Current status note (2026-06-20): this is a historical handoff. Its
> deferred Slice 3/4/5 and `Identity.Bases()` notes are superseded by
> [`docs/handoff-codex-2026-06-20.md`](./handoff-codex-2026-06-20.md).

This handoff covers the Go-side parity work done on `feat/cambium-sdk-phase1`.
It landed the **success floor** defined in `docs/kimi-phase4-prompt.md`:
Slice 1 (reshaped `ParseMode`/`SerializeFlags`/`ValidateMode` + byte-stability
gate) and Slice 2a/2b (rich `Module`/`SchemaNodeRef`/`TypeInfo` → `ResolvedType`).

## Where we are (all green)

- `cd go && CGO_ENABLED=1 go build ./...` → clean.
- `cd go && CGO_ENABLED=1 go vet ./...` → clean.
- `cd go && CGO_ENABLED=1 go test ./...` → all packages pass.
- `cd go && CGO_ENABLED=1 go run ./cmd/cambium all` → **6/6 fixtures pass**
  against the shared `/conformance/golden` corpus.
- `cargo test --workspace` → Rust stack is green against the same goldens.
- Rust-bytes == golden == Go-bytes transitively holds.

## Slices landed

### Slice 1 — Reshaped flags + byte-stability gate

- `ParseMode` is now a composable flags struct (`Strict`, `Opaque`, `ParseOnly`,
  `NoState`, `LybModUpdate`) with the strict/opaque mutual-exclusion guard
  returning `E0002`.
- `SerializeFlags` gained `Shrink`, `KeepEmptyContainers`, and `WithDefaults`
  (`Explicit`/`Trim`/`All`/`AllTagged`). `DefaultSerializeFlags()` returns the
  Rust-equivalent default profile (`Siblings: true`).
- `ValidateMode` gained `Present`.
- Migrated every Go call site (`go/conformance/runner.go`, `go/codegen`, tests)
  to the new shapes.
- Added red-first tests mirroring Rust:
  `TestSerializeDefaultBytesUnchanged`, `TestSerializeShrinkRemovesWhitespace`,
  `TestSerializeKeepEmptyContainer`, `TestParseModeComposeStrictNoState`,
  `TestParseModeStrictOpaqueRejected`.
- Commit: `feat/cambium-sdk-phase1` (Slice 1).

### Slice 2a — Module + rich SchemaNodeRef metadata

- Public `Module` handle (`Name`, `Namespace`, `Prefix`, `Revision`,
  `IsImplemented`, `Identities`, `TopLevel`, `FindPath`).
- Public `SchemaNodeRef` value type with full metadata methods
  (`Name`, `Kind`, `Config`, `Status`, `IsMandatory`, `IsPresenceContainer`,
  `OrderedBy`, `IsListKey`, `ListKeys`, `MinElements`, `MaxElements`, `Units`,
  `DefaultValue`, `Children`).
- `SchemaChildren` iteration helper (`Len`, `IsEmpty`, `Get`, `Iter`).
- `Identity` handle with transitive `Derived()` closure.
- Internal compiled-module forest built once over the frozen context, with a
  `schemaPtrMap` for later data-to-schema bridging.
- Tests mirroring `rust/cambium-core/tests/schema_introspection.rs`:
  `TestModuleMetadata`, `TestChildrenDeclarationOrder`,
  `TestSchemaNodeConfigStatusMandatory`, `TestListKeysStatementOrder`,
  `TestIdentityDerivedClosure`, `TestListUnboundedMaxElementsIsNone`.
- Commit: `feat/cambium-sdk-phase1` (Slice 2a).

### Slice 2b — TypeInfo → ResolvedType

- Public `BaseType`, `IntKind`, `FractionDigits`, `EnumValue`, `EnumDef`,
  `BitsDef`, `Pattern`, `RangeBound`, `TypeInfo`, and `ResolvedType` sealed
  interface with concrete variants (`ResolvedInt`, `ResolvedDecimal64`,
  `ResolvedString`, `ResolvedBinary`, `ResolvedEnumeration`, `ResolvedBits`,
  `ResolvedIdentityRef`, `ResolvedInstanceIdentifier`, `ResolvedLeafRef`,
  `ResolvedUnion`, `ResolvedBoolean`, `ResolvedEmpty`, `ResolvedUnknown`).
- Internal cgo helpers for every anonymous-union / sized-array / pointer-depth
  read required by `TypeInfo`:
  - `cam_range_parts` / `cam_range_part_at` / `cam_range_part_min64` /
    `cam_range_part_max64` / `cam_range_part_minu64` / `cam_range_part_maxu64`
  - `cam_pattern_at` / `cam_pattern_expr` / `cam_pattern_eapptag` /
    `cam_pattern_inverted`
  - `cam_bitenum_at` / `cam_bitenum_name` / `cam_enum_value` /
    `cam_bit_position`
  - `cam_type_ident_bases` / `cam_ident_base_at`
  - `cam_type_union_types` / `cam_union_type_at`
  - `cam_leafref_realtype` / `cam_leafref_require_instance` /
    `cam_instanceid_require_instance`
  - `cam_node_leaf_type` / `cam_node_leaflist_type`
- Corrected the `LY_DATA_TYPE` → `RawBaseType` numeric mapping against the
  vendored libyang enum order (this was the only bug hit after the helpers were
  in place).
- Tests mirroring Rust:
  `TestLeafTypeBaseKindAllBuiltins`, `TestLeafTypeDecimal64FractionDigits`,
  `TestLeafTypeEnumValuesOrdered`, `TestLeafTypeBitsPositionsOrdered`,
  `TestLeafTypeUnionMembersRecursive`, `TestLeafTypeIntRange`,
  `TestLeafTypeDecimal64Range`, `TestLeafTypeStringLength`,
  `TestLeafTypeStringPatterns`, `TestLeafrefRealtypeResolves`.
- Commit: `feat/cambium-sdk-phase1` (Slice 2b).

## Documentation update

- `spec/api.md` now records the sixth sanctioned Rust/Go shape difference:
  Go's `SerializeFlags{}` zero value has `Siblings: false`, so callers must use
  `DefaultSerializeFlags()` for the Rust-equivalent default profile.
- Commit: `feat/cambium-sdk-phase1` (docs/spec).

## Historical deferred list (superseded 2026-06-20)

Per `docs/kimi-phase4-prompt.md`, the success floor is **Slice 1 + Slice 2**.
The following slices were intentionally deferred at the time of this historical
handoff. This list is not current; see
[`docs/handoff-codex-2026-06-20.md`](./handoff-codex-2026-06-20.md).

- **Slice 3 — Data read/nav parity:** `NodeRef`/`NodeSet`/`Value`,
  `get`/`try_get`/`exists`/`select`/`root_nodes`, generation-tagged stale-handle
  contract (`E0007`), `Decimal64` value type, coarse one-FFI children/siblings
  materializer.
- **Slice 4 — Data CRUD + structured validation:** `new_path`/`set_value`/
  `remove_path`/`unlink_path`/`add_defaults`, `NewPathOpts`/`ImplicitOpts`/
  `NodeAddr`, user-ordered read side (`UserOrderedView`,
  `UserOrderedLeafList`), structured `ValidationErrors`/`Diagnostic` with the
  `ly_log` `sync.Mutex` + `LY_LOSTORE` + full `ly_err_first` walk.
- **Slice 5 — Diff/merge/dup + LYB:** `Diff`/`DiffApply`/`Merge`/`Duplicate`,
  `DataDiff`/`DiffEdit`, the inherited `yang:operation` pre-order walk,
  merge conflict pre-scan, and the length-aware LYB serialize/parse primitive.

These slices are scoped and ready; the Rust oracle tests and `adapter.rs`
reference implementations exist and should be mirrored flag-for-flag.

## Needs human review

The following new cgo/`unsafe` diffs were introduced in this block and should be
reviewed before they are considered production-grade:

1. **New C union/bitfield helpers in `go/internal/libyang/schema.go`:**
   `cam_range_parts`, `cam_range_part_*`, `cam_pattern_*`, `cam_bitenum_*`,
   `cam_enum_value`, `cam_bit_position`, `cam_pattern_inverted`,
   `cam_type_ident_bases`, `cam_ident_base_at`, `cam_type_union_types`,
   `cam_union_type_at`, `cam_node_leaf_type`, `cam_node_leaflist_type`.
2. **Sized-array pointer-depth assumptions:** `range.parts` is an array-of-structs
   counted via `cam_range_parts`; `patterns`/`idref.bases`/`union.types` are
   arrays-of-pointers accessed through `*_at` helpers; `enums`/`bits` are
   arrays-of-structs. Count is read one `uint64` before the array base.
3. **`LY_DATA_TYPE` numeric mapping** for `baseTypeFromRaw` was derived from the
   vendored `tree_schema.h`/`schema_compile_node.c` array order and verified by
   runtime tests; a libyang major bump could change it.
4. **Decimal64 range formatting** in `formatDecimal64` keeps all fraction digits
   (matches the Rust adapter), not the trailing-zero-stripping `Display` used
   for data values.
5. **Go `SerializeFlags` zero value ≠ Default** — the resolution is
   `DefaultSerializeFlags()`, now documented in `spec/api.md`.
6. **`Identity.Bases()`** was present but returned an empty slice at the time of
   this historical handoff because the underlying `lysc_ident` struct does not
   store base pointers and the Rust adapter also left `RawIdentity.bases` empty.
   The current implementation has since moved past this gap.

## Green-bar output (recorded)

```text
$ cd go && CGO_ENABLED=1 go test ./...
ok  	github.com/signalbreak-labs/cambium/go/cambium	0.249s
?   	github.com/signalbreak-labs/cambium/go/cmd/cambium	[no test files]
ok  	github.com/signalbreak-labs/cambium/go/codegen	2.237s
ok  	github.com/signalbreak-labs/cambium/go/conformance	0.646s
ok  	github.com/signalbreak-labs/cambium/go/internal/libyang	0.660s

$ cd go && CGO_ENABLED=1 go run ./cmd/cambium all
PASS scrambled-children
PASS keys-first
PASS ordered-user
PASS rpc-order
PASS system-list-canonical
PASS ietf-interfaces
conformance: 6 passed, 0 failed

$ cargo test --workspace
... (full workspace green, including schema_introspection.rs 16/16)
```

## Next steps

Continue with Slice 3. Start by reading `rust/cambium-core/tests/data_read.rs`
and `rust/cambium-libyang-sys/src/adapter.rs` for the value-read and
`find_xpath` primitives, then add the Go `NodeRef`/`NodeSet`/`Value` surface and
the generation/KeepAlive discipline.

## Post-review corrections (2026-06-14)

An adversarial review (cgo memory-safety / flag semantics / Rust↔Go observable
parity / spec conformance, each finding refuted against the vendored libyang
headers + source) confirmed 6 findings out of 19; 13 were refuted. The green
suite missed all of them because no fixture exercised the shape. **Fixed**
(red-test-first where the bug was directly assertable):

- **Presence flag overload (HIGH, both languages).** libyang sets
  `LYS_ORDBY_SYSTEM` (0x80) on every system-ordered (the default) list/leaf-list
  in the **compiled** tree (`schema_compile_node.c:2570`), and that bit aliases
  `LYS_PRESENCE` (0x80). The ungated read `presence = (flags & LYS_PRESENCE)`
  therefore reported `presence: true` for every default config list — in **both**
  `adapter.rs:633` (Rust) and `schema.go` `walkSchemaSiblings` (Go), so the
  cross-language gate never caught it (identically wrong on both sides). Gated
  `presence` to `nodetype == LYS_CONTAINER` in both, mirroring libyang's own
  `lysc_is_np_cont`. `LYS_MAND_FALSE` is never set in the compiled tree, so the
  0x40 (`MAND_FALSE`/`ORDBY_USER`) overload is **not** active — `mandatory` and
  `ordered_by_user` were left unchanged (verified safe).
- **anyxml misclassified as `unknown` (medium, both).** `node_kind`/`schemaKindName`
  exact-matched `LYS_ANYDATA` (0x60); a compiled `anyxml` keeps its distinct
  nodetype (0x20) and fell through to `unknown` instead of the public `AnyData`
  kind. Added the `LYS_ANYXML` arm in both. (The Junos 25.4R1 census found 325
  anyxml nodes, so this is a real-world shape.)
- **`nth()` unfiltered sibling walk (medium, Go — parity).** Go
  `RawUserOrderedList.nth()` walked all of the parent container's children by
  `.next`, while Rust filters by schema name. On a heterogeneous parent (a
  user-ordered list plus any other sibling — common in real YANG) the index
  addressed the wrong node, silently corrupting structural order. Brought Go to
  parity: added a `schemaName` field + a `cam_lyd_schema_name` C accessor,
  populated it in `UserOrderedListAt`, and filtered `nth()` + `InsertFirst`.
- **Missing `runtime.KeepAlive` over the schema walk (medium, Go — latent UAF).**
  `SchemaTree`/`SchemaModule`/`SchemaModules` read `c.ctx` then walked
  ctx-owned C memory without pinning the finalizer-bearing `*RawContext`; a GC
  during the walk could free the schema tree mid-traversal. Added
  `defer runtime.KeepAlive(c)` to all three, matching the data-tree methods.
- **Backwards comment (cosmetic, Go).** The `max-elements` unbounded comment in
  `schema.go` claimed libyang uses 0; the compiled tree uses `UINT32_MAX` (the
  guard was already correct — only the comment was wrong).

Regression tests added: `schema_introspection` (Go + Rust) assert a
system-ordered keyed-list reports `presence == false` and an `anyxml` node
reports kind `AnyData`; `ordered_test.go` adds a heterogeneous-parent
user-ordered move test. Full workspace green both languages (Go `go test -race`,
conformance 6/6 byte-identical, Rust 17/17 introspection + clippy/fmt clean).

### Historical deferred finding (superseded 2026-06-20)

- **Identity `bases()` always returned `[]` at this historical checkpoint
  (medium, both languages).** Both `Identity.Bases()` (Go) and
  `Identity::bases()` (Rust) returned empty because the **compiled**
  `lysc_ident` struct has no `bases` field — only the forward `derived` set.
  The base identifiers needed to be read from the **parsed** tree
  (`lys_module.parsed->identities[i].bases`, a `const char **` of possibly
  prefix-qualified names), then resolved to indices via the existing identity
  map. This is a pre-existing both-empty gap (no drift today), is unused by the
  ordered-serialization core, and needs its own red-test-first slice (a new Go C
  accessor over `lysp_ident.bases`, the Rust `lysp_ident.bases` read, and
  cross-module prefix resolution). Not rushed into this commit.
