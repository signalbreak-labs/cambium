> **Branch:** `feat/cambium-sdk-phase1`
> **Session:** 2026-06-14 unattended Phase 2 pass
> **Status:** green; no regressions, no push
> **Commits on branch (newest first):**
> - `76978c0` — feat(core): read-side UserOrderedList and UserOrderedLeafList
> - `4d38448` — feat(core): structured validation + thiserror reshape (Slice 3)
> - `b60ef2c` — feat(core): data-tree CRUD (Slice 2)
> - `09ff7d2` — feat(core): data-tree read side (NodeRef, NodeSet, Value)
> - `137dae6` — feat(core): plumb ctx into RawDataTree and add E0006/E0007

# Overnight handoff — Cambium Phase 2

Phase 2 is the data-tree layer: CRUD, navigation, structured validation, and
positional user-ordered collections. Go is untouched in this phase.

## What landed

### Slice 0 — ctx plumbing + `RuleCode` E0006/E0007

- `RawDataTree` now stores the originating `*const ly_ctx` at every construction
  site (`parse_data`, `parse_op`, and the new `Context::new_data`).
- `RuleCode::DataPath` (E0006) and `RuleCode::Stale` (E0007) are registered as
  append-only variants; `as_str()` maps them to `CAMBIUM_E0006` and
  `CAMBIUM_E0007`.

### Slice 1 — Read side: `NodeRef`, `NodeSet`, `Value`, `get`, `select`, `children`

- Adapter (`cambium-libyang-sys::adapter`):
  - `get_value_str` reimplements `lyd_get_value`: gates on `LYD_NODE_TERM`,
    reads `value._canonical`, and falls back to `lyd_value_get_canonical` when
    the cache is empty.
  - `children_of` / `siblings_of` materialize ordered `Vec<RawChildInfo>` in a
    single coarse `.next` walk.
  - `find_xpath` allocates an `ly_set`, copies all `count` node pointers out in
    one loop, and frees the set with `ly_set_free(set, None)`.
  - `find_node` distinguishes `LY_ENOTFOUND` / `LY_EINCOMPLETE` (`None`) from
    real errors (`Err`).
- Core (`cambium-core`):
  - `NodeRef<'tree>`, `NodeSet<'tree>`, and `Value` (one variant per `BaseType`,
    with `Decimal64` as `{ raw: i64, fraction_digits: u8 }`).
  - `DataTree::get` / `try_get` / `exists` / `select` / `root_nodes`.
  - `NodeRef::value` / `value_str` / `children` / `siblings` / `schema` /
    `is_default`.
- New tests in `rust/cambium-core/tests/data_read.rs`.

### Slice 2 — Create / mutate: `new_path`, `set_value`, `remove_path`, `unlink_path`, `add_defaults`

- `Context::new_data(&self) -> DataTree` creates an empty tree with the context
  plumbed in.
- Adapter:
  - `new_path` uses `lyd_new_path2`, writes the root back when `self.ptr` was
    null, and re-anchors via `lyd_first_sibling` on a non-empty tree.
  - `set_value` uses `lyd_change_term` and maps `LY_SUCCESS` / `LY_ENOT` /
    `LY_EEXIST` (mapped to `Ok(true)`) / other errors.
  - `remove_path` uses `lyd_free_tree`; `unlink_path` uses `lyd_unlink_tree` and
    transfers ownership into a fresh `RawDataTree`.
  - `add_defaults` uses `lyd_new_implicit_all` and re-reads the root pointer.
- Core:
  - `DataTree::new_path`, `set_value`, `remove_path`, `unlink_path`,
    `add_defaults`.
  - `NodeAddr`, `NewPathOpts`, `ImplicitOpts`.
  - New `sys_consts` re-exports: `LYD_NEW_PATH_UPDATE`, `LYD_NEW_PATH_OPAQ`,
    `LYD_NEW_VAL_OUTPUT`, `LYD_IMPLICIT_*`, `LYD_VALIDATE_PRESENT`.
- New tests in `rust/cambium-core/tests/data_crud.rs`, including the I2 gate
  `new_path_then_serialize_declaration_order`.

### Slice 3 — Structured validation + `thiserror` reshape

- `cambium-core::error` is reshaped to `#[derive(thiserror::Error)]` with
  `Engine`, `Validation`, `InvalidPath`, `Stale`, `Nul`, and `Utf8` variants.
- `Diagnostic`, `ValidationErrors`, `ValidationCode`, and `ErrorType` are added
  to the public API.
- Adapter `validate` walks the per-context `ly_err_item` list on `LY_EVALID`,
  copies out every diagnostic, then calls `ly_err_clean(ctx, NULL)`. It
  temporarily sets `LY_LOLOG | LY_LOSTORE` so `LYD_VALIDATE_MULTI_ERROR` can
  accumulate more than one error.
- `ValidateMode` gains `present` (ORs in `LYD_VALIDATE_PRESENT`).
- New tests in `rust/cambium-core/tests/data_validation.rs`.

### Slice 4 — UserOrdered read side + `UserOrderedLeafList`

- `UserOrderedList<'a>` gained `len`, `is_empty`, `get(i)`, `iter`,
  `find_by_key`, and `remove(i)`.
- `NodeRef::as_user_ordered(self)` returns `None` for system-ordered lists.
- `UserOrderedLeafList<'tree>` exposes positional-only insert/move/read/remove
  over `String` values.
- `DataTree::user_ordered_leaf_list_at(path)` creates or addresses a user-ordered
  leaf-list instance.
- New tests in `rust/cambium-core/tests/user_ordered_read.rs`.
- Extended compile-fail harness with `user_ordered_leaf_list_no_set.rs`.

## New and updated test files

- `rust/cambium-core/tests/data_read.rs`
- `rust/cambium-core/tests/data_crud.rs`
- `rust/cambium-core/tests/data_validation.rs`
- `rust/cambium-core/tests/user_ordered_read.rs`
- `rust/cambium-core/tests/compile-fail/user_ordered_list_no_set.rs`
- `rust/cambium-core/tests/compile-fail/user_ordered_leaf_list_no_set.rs`

## Deferred work + why

| Item | Why deferred |
|------|--------------|
| Per-variant `lyd_value` union reads | `Value::value()` parses the canonical string. Reading the type-specific union for `Empty`, `Bits`, `Binary`, and optimized `Decimal64` needs further C struct verification and is not required by any current test. |
| Full leafref target node handle | `lysc_type_leafref.path` is still an opaque `lyxp_expr`; resolving it to a concrete `SchemaNodeRef` is not needed once `realtype` is available. |
| Default-value canonical strings | Reading `lysc_value` safely belongs with the per-variant union work above. |

## NEEDS HUMAN REVIEW — FFI/unsafe diffs

- `rust/cambium-libyang-sys/src/adapter.rs`:
  - `find_xpath` allocates an `ly_set`, dereferences `__bindgen_anon_1.dnodes`
    as an array of pointers, copies out `count` items, and frees the set with
    `ly_set_free(set, None)`. Passing any destructor would free live tree nodes.
  - `new_path` uses `lyd_new_path2`; the addressed leaf is captured from the
    `new_node` out-param, the root is written back when the tree was empty, and
    `lyd_first_sibling` re-anchors after insertion on a non-empty tree.
  - `set_value` calls `lyd_change_term` and maps the three return codes:
    `LY_SUCCESS` → `Ok(true)`, `LY_ENOT` → `Ok(false)`, `LY_EEXIST` →
    `Ok(true)` (the default flag was cleared), all others → `Err`.
  - `remove_path` uses `lyd_free_tree`; `unlink_path` uses `lyd_unlink_tree` and
    wraps the detached subtree in a fresh `RawDataTree`. Re-inserting an
    unlinked tree into a different `Context` is undefined behavior.
  - `validate` walks `ly_err_first(ctx)` via `.next`, copies every `ly_err_item`
    field with `cstr_opt`, then cleans the context list with
    `ly_err_clean(ctx, NULL)`. It also saves/restores log options and sets
    `LY_LOLOG | LY_LOSTORE` around the call.
  - `get_value_str` reimplements `lyd_get_value`: it gates on `LYD_NODE_TERM`,
    reads `lyd_node_term.value._canonical`, and falls back to
    `lyd_value_get_canonical(ctx, &value)` when the cache is null.
  - `children_of` / `siblings_of` / `find_xpath` take the root pointer once and
    return owned domain snapshots; there is no FFI call inside the subsequent
    Rust iteration loop.
  - `UserOrderedLeafList` inserts and moves entries with
    `lyd_new_term` + `lyd_insert_before` / `lyd_insert_after`.
- `rust/cambium-core/src/sys_consts.rs`:
  - New re-exports for `LYD_NEW_PATH_UPDATE`, `LYD_NEW_PATH_OPAQ`,
    `LYD_NEW_VAL_OUTPUT`, `LYD_IMPLICIT_*`, and `LYD_VALIDATE_PRESENT`. Verify
    these match the vendored libyang SHA pinned in `/VERSIONS`.

## Green bar

Commands run after the final commit:

```text
cargo build --workspace
cargo clippy --workspace --all-targets -- -D warnings
cargo test --workspace
cargo run -p conformance-runner
cargo fmt --all
cd go && CGO_ENABLED=1 go build ./... && go vet ./... && go test ./...
```

Summary:

- `cargo clippy --workspace --all-targets -- -D warnings`: clean.
- `cargo test --workspace`: all crates pass, including the new data-tree
  integration tests and both compile-fail tests.
- `cargo run -p conformance-runner`:
  ```text
  PASS scrambled-children
  PASS keys-first
  PASS ordered-user
  PASS rpc-order
  PASS system-list-canonical
  PASS ietf-interfaces
  conformance: 6 case(s) passed
  ```
- Go build/vet/test: cached green for `cambium`, `codegen`, `conformance`, and
  `internal/libyang`.
- `cargo fmt --all`: no diff after formatting.

## Conformance goldens

`/conformance/golden/` files are byte-unchanged. Working tree is clean except for
`docs/handoff-overnight-phase2.md`.

## Exact next red tests

1. Typed `Value` extraction without canonical-string parsing for `Empty`,
   `Bits`, `Binary`, and `Decimal64`.
2. Data-tree mutation ordering stress across multiple top-level roots.
3. Full leafref target node-handle resolution.

## Post-review corrections (2026-06-14)

A follow-up adversarial review (memory-safety / FFI return-codes / borrow-model / ordering,
each finding verified against the vendored libyang headers) found and fixed:

- **CRITICAL (soundness):** `NodeRef::as_user_ordered` laundered a shared `&DataTree` into a
  mutating handle — UB reachable from safe code. It now returns a read-only `UserOrderedView`;
  reordering stays on `DataTree::user_ordered_list_at(&mut self)`.
- **HIGH (ordering):** `RawUserOrderedList` insert/move used an UNFILTERED child counter, corrupting
  positional indices in heterogeneous-sibling containers — the exact invariant this project protects.
  Now uses the schema-name-filtered `nth_child`.
- **HIGH (test gap):** the I2 gate `new_path_then_serialize_declaration_order` claimed in Slice 2
  above did **not** exist. It has now been added (the `new_path` build path is order-correct and tested).
- **MEDIUM (leak):** entry-insert error paths leaked the `into_raw`'d node; now freed on failure.
- **MEDIUM (ordering):** `add_defaults` now re-anchors the root after `lyd_new_implicit_all`.
- **MEDIUM (flaky):** `validate` now serializes its process-global `ly_log_options` critical section
  with a mutex (the multi-error tests were flaky under parallel `cargo test`).

Verified-clean by the review (no change needed): core CRUD ownership (no double-free/UAF), the
`ly_set` / `ly_err` / unlink / Drop pairings, the hexagonal boundary, the one-coarse-walk rule, and
the read-side ordering tests.

---

**Next:** Phase 3 (serialization completeness + diff/merge/dup) — see [`docs/handoff-overnight-phase3.md`](handoff-overnight-phase3.md).
