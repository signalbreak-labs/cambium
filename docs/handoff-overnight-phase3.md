> **Branch:** `feat/cambium-sdk-phase1`
> **Session:** 2026-06-14 unattended Phase 3 pass
> **Status:** green; no regressions, no push
> **Previous:** [`docs/handoff-overnight-phase2.md`](handoff-overnight-phase2.md)
> **Commits on branch (newest first):**
> - `dbae877` — feat(core,sys): diff_apply + merge with conflict pre-scan
> - `574940e` — feat(core,sys): diff + DataDiff/DiffEdit with user-order atomicity
> - `095ffc3` — feat(core,sys): DataTree::duplicate with recursive dup
> - `fb3579d` — feat(core,sys): Format::Lyb with length-aware serialize
> - `9af8b8c` — feat(core): reshape ParseMode to composable flags
> - `a8aa982` — feat(core): serialize flags completeness and golden gate

# Overnight handoff — Cambium Phase 3

Phase 3 is the serialization-completeness + plan/apply engine: `SerializeFlags`,
`ParseMode`, `Format::Lyb`, `duplicate`, `diff`/`DataDiff`/`DiffEdit`,
`diff_apply`, and `merge`. Go is untouched in this phase.

## What landed

### Slice 1 — `SerializeFlags` completeness + byte-stability gate

- `WithDefaults` enum (`Explicit`, `Trim`, `All`, `AllTagged`) with `#[default]` on `Explicit`.
- `SerializeFlags` extended to `{ siblings, shrink, keep_empty_containers, with_defaults }`;
  hand-written `Default { siblings: true, ... }` to match `sdk-api-design.md`.
- `DataTree::serialize` OR-maps each new flag only when set; default profile contributes zero
  new bits (`WithDefaults::Explicit == 0`).
- New `sys_consts` re-exports: `LYD_PRINT_SHRINK`, `LYD_PRINT_EMPTY_CONT`, `LYD_PRINT_WD_TRIM`,
  `LYD_PRINT_WD_ALL`, `LYD_PRINT_WD_ALL_TAG`.
- Tests in `rust/cambium-core/tests/serialize_flags.rs`:
  `serialize_default_bytes_unchanged`, `serialize_with_defaults_explicit_equals_today`,
  `serialize_with_defaults_all_shows_default_leaves` (after `add_defaults`),
  `serialize_with_defaults_trim_hides_default_leaves`, `serialize_keep_empty_container`,
  `serialize_shrink_removes_insignificant_whitespace`.

### Slice 2a — `ParseMode` reshape

- `ParseMode` converted from enum to composable flags struct:
  `{ strict, opaque, parse_only, no_state, lyb_mod_update }`.
- `From<ParseMode> for u32` is an OR-accumulator; `strict && opaque` returns a domain `Error`.
- Added `ParseMode::data_only()` convenience constructor.
- New `sys_consts` re-exports: `LYD_PARSE_NO_STATE`, `LYD_PARSE_LYB_SKIP_MODULE_CHECK`.
- Migrated every `ParseMode::DataOnly` call site in tests + `conformance-runner` in the same commit.
- Tests in `rust/cambium-core/tests/parse_mode.rs`:
  `parse_mode_compose_strict_plus_no_state`, `parse_mode_strict_opaque_rejected`,
  `parse_mode_strict_rejects_unknown`, `parse_mode_strict_allows_known_state`,
  `parse_mode_data_only_convenience`, `parse_mode_no_state_forbids_state`.

### Slice 2b — `Format::Lyb`

- Added `Format::Lyb` + `RawFormat::Lyb`; both `From<Format>`/`From<RawFormat>` impls have the
  required `Lyb` arm.
- Added separate length-aware `RawDataTree::serialize_lyb` using `ly_out_new_memory`,
  `lyd_print_all`, `ly_out_printed`, and `ly_out_free(..., None, 1)` — never the frozen `CStr` path.
- LYB parse uses a trailing NUL sentinel (one extra byte) instead of `CString`, so embedded NULs
  survive round-trip.
- Test in `rust/cambium-core/tests/lyb_round_trip.rs`:
  JSON → parse → LYB → parse LYB → JSON, asserting byte equality.

### Slice 3 — `duplicate()`

- Adapter `RawDataTree::duplicate` uses `lyd_dup_siblings(first_sibling, NULL,
  LYD_DUP_RECURSIVE | LYD_DUP_WITH_FLAGS, &mut out)` and wraps the result in a fresh
  `RawDataTree` with its own Drop.
- Core `DataTree::duplicate(&self) -> Result<DataTree>`.
- Tests in `rust/cambium-core/tests/duplicate.rs`:
  `duplicate_deep_copy_independent`, `duplicate_preserves_user_order`, `duplicate_freed_once`.

### Slice 4 — `diff` + `DataDiff`/`DiffEdit` + metadata read

- `RawDataDiff { ptr, ctx }` with a `Drop` calling `lyd_free_all(ptr)` when non-null.
- `RawDataTree::diff(other, options: u16) -> Result<Option<RawDataDiff>>`; null diff on equality
  returns `None` (empty `DataDiff`), not an error.
- One coarse pre-order walk materializes `Vec<RawDiffEdit>`:
  - resolves `yang:operation` by walking up parent meta until found;
  - reads the meta value via `(*meta).value._canonical` with `lyd_value_get_canonical` fallback;
  - detects `ordered-by user` via the schema flag `LYS_ORDBY_USER` (guarding null schema);
  - builds the path with `lyd_path` + `libc::free`;
  - copies every string into owned `String` before returning.
- Core types: `DiffOpts`, `DiffOp`, `DataDiff`, `DiffEdit`.
- `filter_spurious_user_ordered_replaces` removes N scalar `Replace` edits that libyang emits under
  a user-ordered move when the leaf values are byte-identical; the move is carried as a single
  user-ordered edit.
- Tests in `rust/cambium-core/tests/diff.rs`:
  `diff_empty_when_equal`, `diff_create_delete_replace`, `diff_ordered_by_user_atomic`
  (positional-only delta → exactly one edit, `is_ordered_by_user()==true`, no child `Replace`),
  `datadiff_freed_once`.

### Slice 5 — `diff_apply` + `merge` with conflict pre-scan

- Adapter:
  - `RawDataTree::diff_apply(&mut self, diff)` uses `lyd_diff_apply_all` and re-anchors via
    `lyd_first_sibling`; the diff is borrowed, NOT freed.
  - `RawDataTree::merge(&mut self, source, options)` duplicates the source and merges with
    `LYD_MERGE_DESTRUCT` against the copy, so a borrowed `source` is never mutated/freed.
- Core:
  - `MergeOpts { destruct }`.
  - `DataTree::diff_apply(&mut self, &DataDiff)`.
  - `DataTree::merge(&mut self, source, MergeOpts)` runs `self.diff(source)` first; any
    `Replace` edit whose path exists in both trees with differing canonical values returns
    `Error::ffi(RuleCode::Validate, ...)` BEFORE `self` is mutated.
- `spec/rule-codes.md`: E0003 "Raised by" updated to `validate · merge` (append-only clarification).
- Tests in `rust/cambium-core/tests/diff_apply_merge.rs`:
  `diff_apply_round_trip`, `merge_preserves_user_order`, `merge_conflict_errors`
  (asserts `RuleCode::Validate`, real differing value, and target untouched after the error).

### Facade / prelude wiring

All new public types are threaded through all three re-export sites:
- `rust/cambium-core/src/lib.rs`
- `rust/cambium/src/lib.rs` top-level `pub use`
- `rust/cambium/src/lib.rs` `pub mod prelude`

New types: `WithDefaults`, `DiffOpts`, `MergeOpts`, `DiffOp`, `DataDiff`, `DiffEdit`, and the
reshaped `ParseMode`/`SerializeFlags`/`Format::Lyb`.

## DEFERRED

Nothing deferred in Phase 3. All planned slices landed green.

## NEEDS HUMAN REVIEW

Every new FFI/unsafe surface is flagged below for review.

1. **LYB length-aware print primitive vs. the frozen `CStr` path.**
   `RawDataTree::serialize_lyb` (`rust/cambium-libyang-sys/src/adapter.rs`):
   ```rust
   let options = options & !LYD_PRINT_SIBLINGS;
   let mut buf: *mut ::std::os::raw::c_char = std::ptr::null_mut();
   let mut out: *mut ly_out = std::ptr::null_mut();
   let rc = unsafe { ly_out_new_memory(&mut buf, 0, &mut out) };
   // ...
   let first = unsafe { lyd_first_sibling(self.ptr) };
   let rc = unsafe { lyd_print_all(out, first, LYD_FORMAT::LYD_LYB, options) };
   // ...
   let len = unsafe { ly_out_printed(out) };
   let bytes = unsafe { std::slice::from_raw_parts(buf as *const u8, len).to_vec() };
   unsafe { ly_out_free(out, None, 1) };
   ```
   The same pattern is duplicated on `RawDataDiff::serialize_lyb`. Confirm `ly_out_free` signature
   and `destroy==1` for a memory-backed `ly_out`.

2. **`RawDataDiff` RAII Drop + null-diff-is-empty branch.**
   ```rust
   impl Drop for RawDataDiff {
       fn drop(&mut self) {
           if !self.ptr.is_null() {
               unsafe { lyd_free_all(self.ptr) };
           }
       }
   }
   ```
   `RawDataTree::diff` returns `Ok(None)` when `*diff == NULL` on `LY_SUCCESS`; the empty
   `DataDiff` owns no raw pointer.

3. **`yang:operation` inherited-from-parent walk + `(*meta).value._canonical` read +
   one-coarse-walk attestation.**
   The entire `RawDataDiff::edits()` walk is in `rust/cambium-libyang-sys/src/adapter.rs`:
   ```rust
   pub fn edits(&self) -> Result<Vec<RawDiffEdit>, String> {
       let mut out = Vec::new();
       let yang_mod = unsafe { ly_ctx_get_module_implemented(self.ctx, cstr_yang()) };
       if yang_mod.is_null() { ... }
       self.collect_edits(self.ptr, yang_mod, &mut out)?;
       Ok(out)
   }

   fn collect_edits(&self, node, yang_mod, out) {
       while !cur.is_null() {
           let op = self.inherited_op(cur, yang_mod)?;
           if !matches!(op, RawDiffOp::None) {
               let path = self.node_path(cur)?;
               let value = if unsafe { node_is_term(cur) } {
                   unsafe { node_value_str_direct(cur) }
               } else { None };
               let is_user_ordered = unsafe {
                   let schema = (*cur).schema;
                   !schema.is_null() && ((*schema).flags as u32 & LYS_ORDBY_USER) != 0
               };
               out.push(RawDiffEdit { op, path, value, is_user_ordered });
           }
           if let Some(child) = unsafe { node_child_first(cur) } {
               self.collect_edits(child, yang_mod, out)?;
           }
           cur = unsafe { (*cur).next };
           if cur == node { break; }
       }
   }

   fn inherited_op(&self, node, yang_mod) {
       let mut cur = node;
       while !cur.is_null() {
           let meta = unsafe {
               lyd_find_meta((*cur).meta as *const lyd_meta, yang_mod, cstr_operation())
           };
           if !meta.is_null() {
               let val = unsafe { meta_value_str(meta, self.ctx) }?;
               return Ok(match val.as_deref() { ... });
           }
           cur = unsafe { (*cur).parent };
       }
       Ok(RawDiffOp::None)
   }

   unsafe fn meta_value_str(meta, ctx) {
       let canonical = unsafe { (*meta).value._canonical };
       if !canonical.is_null() { return Ok(unsafe { cstr_opt(canonical) }); }
       let fallback = unsafe { lyd_value_get_canonical(ctx, &(*meta).value) };
       Ok(unsafe { cstr_opt(fallback) })
   }
   ```
   **Attestation:** there is no `lyd_*`/FFI call inside the per-edit loop body beyond the single
   pre-order walk's own per-node reads. The walk resolves `yang:operation` once per node (walking
   up parents), copies the canonical value once, checks the schema flag once, builds the path once,
   and pushes an owned `RawDiffEdit`. All returned `*lyd_meta` pointers are discarded before the
   function returns; `DiffEdit<'d>` borrows only from the owned `Vec<RawDiffEdit>`.

4. **`LYS_ORDBY_USER` detection.**
   Used in `collect_edits` above and also in `RawUserOrderedList::is_user_ordered` helpers.
   Value `64` (`LYS_ORDBY_USER`) is read from `bindings.rs`; schema flag is on `lysc_node.flags`.

5. **Merge dup-then-DESTRUCT + conflict pre-scan.**
   `RawDataTree::merge` duplicates `source` with `lyd_dup_siblings` then merges with
   `LYD_MERGE_DESTRUCT` against the duplicate. `DataTree::merge` pre-scans via
   `self.diff(source)` and returns `RuleCode::Validate` before calling the adapter, so `self` is
   untouched on conflict. The `merge_conflict_errors` test proves the target leaf retains its
   original value after the error.

6. **`lyd_dup_siblings` + `LYD_DUP_RECURSIVE` choice.**
   ```rust
   let first = unsafe { lyd_first_sibling(self.ptr) };
   let rc = unsafe {
       lyd_dup_siblings(
           first,
           std::ptr::null(),
           (LYD_DUP_RECURSIVE | LYD_DUP_WITH_FLAGS) as u32,
           &mut out,
       )
   };
   ```
   `LYD_DUP_WITH_FLAGS` preserves default/validated state; `LYD_DUP_RECURSIVE` copies the whole
   sibling chain recursively.

7. **Diff/merge `u16` options cast.**
   `lyd_diff_siblings` and `lyd_merge_siblings` take `u16` options, unlike the `u32` print/parse/dup
   paths. The adapter casts `LYD_DIFF_*` and `LYD_MERGE_*` constants to `u16` before the FFI call.

8. **`lyb_mod_update → LYD_PARSE_LYB_SKIP_MODULE_CHECK` rename.**
   The design-doc field `ParseMode::lyb_mod_update` maps to the real libyang flag
   `LYD_PARSE_LYB_SKIP_MODULE_CHECK` (value `1048576`). There is no `LYD_PARSE_LYB_MOD_UPDATE` in
   this pinned libyang. Documented inline in `From<ParseMode> for u32`.

9. **New `sys_consts` re-exports.**
   `LYD_PRINT_SHRINK`, `LYD_PRINT_EMPTY_CONT`, `LYD_PRINT_WD_TRIM`, `LYD_PRINT_WD_ALL`,
   `LYD_PRINT_WD_ALL_TAG`, `LYD_PARSE_NO_STATE`, `LYD_PARSE_LYB_SKIP_MODULE_CHECK`. These are the
   only libyang constants referenced by the safe core for Phase 3.

## Verification

Final green-bar run (after the Slice 5 commit):

```
$ cargo fmt --all
$ cargo build --workspace      # ok
$ cargo clippy --workspace --all-targets -- -D warnings   # ok
$ cargo test --workspace       # all crates/tests pass
$ cargo run -p conformance-runner
PASS scrambled-children
PASS keys-first
PASS ordered-user
PASS rpc-order
PASS system-list-canonical
PASS ietf-interfaces
conformance: 6 case(s) passed
$ (cd go && CGO_ENABLED=1 go build ./... && go test ./... && go vet ./...)
ok  	github.com/signalbreak-labs/cambium/go/cambium
ok  	github.com/signalbreak-labs/cambium/go/codegen
ok  	github.com/signalbreak-labs/cambium/go/conformance
ok  	github.com/signalbreak-labs/cambium/go/internal/libyang
```

### Golden-byte stability

`/conformance/golden` is byte-identical to HEAD. The conformance runner prints all 6 cases PASS.
No golden file was modified or regenerated.

### Rust test summary

- `serialize_flags`: 6/6 pass
- `parse_mode`: 6/6 pass
- `lyb_round_trip`: 1/1 pass
- `duplicate`: 3/3 pass
- `diff`: 4/4 pass
- `diff_apply_merge`: 3/3 pass
- All other Phase 1/2 tests: pass

## Cross-language drift

Go's `ParseMode`/`SerializeFlags`/`Format` surface is still the old enum-based contract. The Rust
reshape intentionally drifts ahead; the next Go-parity phase must:

1. Convert Go `ParseMode` to a composable flags struct matching `spec/api.md` §F.
2. Convert Go `SerializeFlags` to include `WithDefaults` and hand-write the default profile.
3. Add `Lyb` to Go `Format` only after a confident length-aware binary path is implemented.
4. Add a shared conformance assertion that Rust and Go emit byte-identical XML/JSON for the same
   inputs and assign identical rule codes.

The Rust side already exports the target contract through `cambium` and `cambium::prelude`.

## Exact next red test

The next red test should be a **Go-side `TestParseModeComposedStrictNoState`** (or equivalent) that
composes `strict` + `no_state` in a single parse and rejects unknown config nodes while forbidding
state data. That will fail until Go's `ParseMode` is reshaped to the composable struct, making the
cross-language drift concrete and testable.

## Post-review corrections (2026-06-14)

A follow-up adversarial review (memory-safety / FFI / borrow / test-honesty, each finding verified
against the vendored libyang sources) found and fixed:

- **HIGH (double-free / UAF):** the merge `LYD_MERGE_DESTRUCT` error path called
  `lyd_free_all(dup_first)` on a duplicate libyang already owns-and-frees on failure
  (`tree_data.c:2834-2836`) — latent because it is an OOM-class untested path. **Fix:** `merge` now
  always calls `lyd_merge_siblings` **without** `LYD_MERGE_DESTRUCT` on the borrowed (`const`)
  source — libyang dup-copies internally, so no dup, no ownership transfer, no double-free. The
  result is identical (source was always preserved). `MergeOpts::destruct` is now documented as inert.
- **MED/LOW (brittle heuristic + test honesty):** the I6 atomicity test passed only because of an
  undocumented `filter_spurious_user_ordered_replaces` that dropped same-value `Replace` edits via
  `rsplit_once('/')` path-string parsing. **Root cause:** `collect_edits` emitted an inherited
  `replace` edit on the immutable **list KEY** leaf (libyang puts the move on the list node and
  copies only its key — `diff.c:344`). **Fix:** `collect_edits` now skips `LYS_KEY` nodes at the
  source (keys are immutable per RFC 7950 §7.8.2 and never independent edits); the string-fragile
  filter is **removed**, and `diff_ordered_by_user_atomic` now passes genuinely (one atomic edit).

Verified-clean by the review (no change needed): the LYB length-aware free, `RawDataDiff` Drop,
the `lyd_meta` walk, the non-destruct merge safety, `diff_apply`, `duplicate`, byte-stability
(`serialize_default_bytes_unchanged` calls `SerializeFlags::default()`), and the ParseMode migration.
