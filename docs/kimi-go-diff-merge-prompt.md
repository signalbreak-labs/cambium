# Work Order — Go Parity: `Diff` · `DiffApply` · `Merge` · `Duplicate`

You are implementing four data-tree operations in the **Go** binding of **Cambium**
(a YANG toolkit / SDK built on libyang), bringing them to parity with the existing
**Rust** implementation, which is the **oracle**. The Rust side is done, tested, and
correct — your job is to mirror its observable behavior in Go, including identical
error rule-codes and byte-identical serialized output, and to do it without
introducing a use-after-free or double-free across the cgo boundary.

This is a single-session work order. Read it in full before writing any code.

---

## 0. NON-GOALS — read first (anti-drift)

Do **not** do any of the following. They are out of scope and will get the PR
rejected:

- **No NETCONF / gNMI / gRPC / Terraform.** No `<edit-config>` envelopes, no
  transport clients, no device sinks, no `terraform-plugin-framework` emitters.
  Cambium is a library; downstream repos consume it. This task touches only the
  in-process data tree.
- **No LYB.** Do not add, wire, or test the LYB binary format for any of these four
  ops. Diff serialization is XML/JSON only. If you see `Format::Lyb` in the Rust
  serializer, ignore it — it is not part of this work order.
- **No new public Go API beyond the four ops below** and the minimal internal cgo
  primitives they require (Section 4). Do not invent helpers, options structs, or
  convenience wrappers not present in the Rust oracle.
- **No schema/codegen changes**, no context API changes, no parser changes.
- **Do not refactor** unrelated Go or Rust code. Do not "improve" the Rust oracle.
  If you believe the oracle is wrong, stop and flag it in the handoff — do not
  silently diverge.

---

## 1. Mandate (what "done" means)

Implement, on the public Go `DataTree` and a new public Go `DataDiff`:

1. `Duplicate` — deep, independent copy of a tree.
2. `Diff` — compute the diff from one tree to another, yielding a `DataDiff` that
   exposes ordered edits and can serialize itself (yang-patch-shaped).
3. `DiffApply` — apply a `DataDiff` to a tree in place.
4. `Merge` — merge a source tree into a target tree in place, with a **conflict
   pre-scan** that rejects differing-value collisions before any mutation.

"Done" = every item in the **SUCCESS FLOOR** (Section 11) holds, each op committed
separately, on the correct branch, with red-first TDD evidence, no push.

---

## 2. Ground truth — branch, HEAD, oracle locations

- **Branch:** `feat/cambium-sdk-phase1`. Work here. Do **not** branch off `main`.
- **HEAD at handoff:** `788543d`. Verify with `git rev-parse --short HEAD` before
  starting; if it differs, stop and reconcile — the line numbers below are pinned to
  this commit and may have moved.
- **Rust oracle (do not edit, only read):**
  - `rust/cambium-core/src/tree.rs` — public `DataTree` / `DataDiff` ops and **error
    rule-code mapping** (this is the authority for codes; see Section 8).
  - `rust/cambium-libyang-sys/src/adapter.rs` — the FFI layer: `duplicate`, `diff`,
    `diff_apply`, `merge`, and the diff-edit metadata walk (`edits` / `collect_edits`
    / `inherited_op` / `node_path` / `meta_value_str`).
  - `rust/cambium-libyang-sys/src/bindings.rs` — the **authoritative flag constant
    values** (see Section 4.4). Read them; do not hardcode from memory.
- **Rust tests to mirror (read each before writing its Go twin):**
  `rust/cambium-core/tests/diff.rs`, `tests/diff_apply_merge.rs`, `tests/duplicate.rs`.
- **Go code you will extend:**
  - `go/cambium/cambium.go` — public types and the `codeForOp` rule-code router.
  - `go/internal/libyang/libyang.go` — the cgo adapter (`RawDataTree`, lifetime
    discipline). New cgo primitives go here.

---

## 3. Engineering rules (non-negotiable — from AGENTS.md)

- **TDD, red-first.** Write the failing Go test, run it, watch it fail for the right
  reason, *then* write production code. No production line — not even a stub — ahead
  of a red test. Capture the red output in your working notes.
- **Hexagonal.** The domain core imports **zero** libyang/cgo types. FFI is an
  adapter. FFI is **coarse-grained**: whole-document dup/diff/apply/merge per call.
  No per-node C calls in hot paths, no C→Go callbacks.
- **`-race` always on** for these tests: `CGO_ENABLED=1 go test -race ./...`. (See
  Section 8.3 for exactly what `-race` does and does not prove — do not over-claim.)
- **Errors:** wrap with identifying values; the rule-code is derived from the op and
  must be **identical to Rust** for the same failure (Section 8). Compare with
  `errors.Is`/`errors.As`, never string-match.
- **Lint clean:** `go vet ./...` and `golangci-lint run` must pass.
- **`ly_ctx` is build-once-then-frozen; data trees are not concurrency-safe.** Do not
  add locking; do not share a tree across goroutines in tests.

---

## 4. The cgo adapter surface you must add

All new C calls live in `go/internal/libyang/libyang.go`. Mirror
`rust/cambium-libyang-sys/src/adapter.rs` exactly.

### 4.1 Core operation primitives

These five mirror the adapter's op methods:

| Go adapter method | libyang call | Notes |
|---|---|---|
| `duplicate` | `lyd_dup_siblings` | First sibling in, `LYD_DUP_RECURSIVE \| LYD_DUP_WITH_FLAGS`; re-anchor result to first sibling. |
| `diff` | `lyd_diff_siblings` | First-sibling of each side; out-param diff node; `LYD_DIFF_DEFAULTS` when defaults requested. |
| `diff_apply` | `lyd_diff_apply_all` | Applies in place to `&self.ptr`; re-anchor `self.ptr` to first sibling after. |
| `merge` | `lyd_merge_siblings` | **flags = `0`** (see 4.3). Source first-sibling; re-anchor `self.ptr` after. |
| (re-anchor helper) | `lyd_first_sibling` | Used after every mutating call so the Go tree pointer never dangles on a detached node. |

### 4.2 Diff-edit metadata-walk primitives (REQUIRED — do not stub)

`Diff` is **not** complete with just `lyd_diff_siblings`. Materializing
`DataDiff.Edits()` requires a second, separate subsystem: a pre-order walk of the
diff tree that reads libyang **metadata** to recover each edit's operation. This
mirrors `adapter.rs:1972-2098` (`edits` → `collect_edits` → `inherited_op` →
`node_path` → `meta_value_str`). You must add **all** of these cgo primitives:

- `ly_ctx_get_module_implemented(ctx, "yang")` — resolve the `yang` module **once**
  per `edits()` call. If it returns null, return an error
  (`"yang module is not implemented in context"`); do not proceed.
- `lyd_find_meta((*node).meta, yangMod, "operation")` — find the `yang:operation`
  metadata on a node. Walk **up** `(*node).parent` to inherit the op from an
  ancestor when the node itself has none (`inherited_op`); stop at the first match.
- **Read the metadata value** the same way the oracle does, in this order:
  1. Read `(*meta).value._canonical` directly; if non-null, use it.
  2. **Fallback:** `lyd_value_get_canonical(ctx, &(*meta).value)`.
  This is `meta_value_str` (`adapter.rs:2085-2098`).
- **Term-node detection + value:** mirror `node_is_term` and `node_value_str_direct`
  (read `(*term).value._canonical`) for the edit's value. A non-term edit has no
  value (`None`/`""`).
- `lyd_path(node, LYD_PATH_TYPE_STD, NULL, 0)` for the edit path; **`free()` the
  returned C string** after copying it into Go (`node_path`, `adapter.rs:2056-2064`).
- **Map the operation string** to an op enum exactly: `"create"`→Create,
  `"delete"`→Delete, `"replace"`→Replace, `"none"`→None (and unknown→None). Edits
  with op `None` are **not** emitted (`collect_edits` skips them).

**THERE IS NO `lyd_get_meta_value` BINDING. Do not call it. Do not invent it.** The
only sanctioned path to a metadata value is `(*meta).value._canonical` with the
`lyd_value_get_canonical` fallback, exactly as above. If you find yourself reaching
for a single-call meta-value accessor, you are off the oracle — stop.

The literal C strings `"yang"` and `"operation"` must be NUL-terminated and kept
alive for the duration of the call (mirror `cstr_yang` / `cstr_operation`).

### 4.3 Merge flags — exactly `0` (double-free defense; see Section 7)

`lyd_merge_siblings(&self.ptr, source_first, 0)`. **Flags = `0`.** Do **not** pass
`LYD_MERGE_DESTRUCT`. The oracle documents why (`adapter.rs:1426-1441`,
`tree_data.c:2744`): without `DESTRUCT`, libyang treats `source` as `const` and
dup-copies from it. The source tree is **borrowed and never modified or freed** by
merge; there is no dup for *you* to own and no node for *you* to free on any path.
Passing `DESTRUCT` would make libyang consume nodes out of `source`, and Go's GC
finalizer on the source `RawDataTree` would then double-free. This is load-bearing.

### 4.4 Flag constant values — verify against `bindings.rs`, do not trust this table

Read the values from `rust/cambium-libyang-sys/src/bindings.rs` (or the vendored
libyang headers) and use those. At HEAD `788543d` they are:

| Constant | Value | Used by |
|---|---|---|
| `LYD_DUP_RECURSIVE` | `1` | duplicate |
| `LYD_DUP_WITH_FLAGS` | `8` | duplicate |
| `LYD_DIFF_DEFAULTS` | `1` | diff (when defaults requested) |
| `LYS_KEY` | `256` (`0x0100`) | diff-edit key skip (4.5) |
| `LYS_ORDBY_USER` | `64` (`0x0040`) | diff-edit user-order flag, duplicate/merge order checks |

If any value disagrees with `bindings.rs`, **trust `bindings.rs`** and note the
discrepancy in your handoff.

### 4.5 DIFF KEY ARTIFACT — fix at the source, not post-hoc

List **keys are immutable** (RFC 7950 §7.8.2) and appear in a diff subtree only to
identify the list instance — never as an independent edit. On a user-ordered *move*,
libyang inherits the parent's `replace` onto the key leaf, which would otherwise
surface as a spurious extra edit. The oracle removes this artifact **at its source**
inside `collect_edits` (`adapter.rs:1996-2000`): for each node, if
`(*schema).flags & LYS_KEY != 0`, **skip emitting an edit for it** (still recurse
into children). Do the same. **Do not** filter keys out in a post-processing pass —
that is fragile and diverges from the oracle. Skip at the walk, so a user-ordered
move stays one atomic edit.

Also record `is_user_ordered` per edit by probing
`(*schema).flags & LYS_ORDBY_USER` (`adapter.rs:2007-2010`), so `DataDiff` can report
whether it carries user-ordered changes (the I6 atomicity property).

---

## 5. The Go public API (with each sanctioned shape difference)

Mirror the Rust signatures, adjusting only for idiomatic Go. The sanctioned
differences from Rust are:

- Rust returns `Result<T>`; Go returns `(T, error)` (or just `error` for in-place
  ops). The error must carry the **same rule-code** (Section 8).
- Rust borrows (`&self`, `&other`) with lifetimes enforcing context identity at
  compile time; Go cannot, so Go must enforce the same invariant at **runtime**
  (Section 5.3).
- Rust `DataDiff::edits()` returns a borrowing iterator; Go `(*DataDiff).Edits()`
  returns a slice of value structs (already materialized — see 4.2). Each edit
  exposes `Op()`, `Path()`, `Value() (string, bool)`, and `IsUserOrdered()`.

### 5.1 Signatures

```go
// Deep, independent copy. Rust: tree.rs duplicate().
func (t *DataTree) Duplicate() (*DataTree, error)

// Diff from t to other. Rust: tree.rs diff().
func (t *DataTree) Diff(other *DataTree, opts DiffOpts) (*DataDiff, error)

// Apply a diff to t in place. Rust: tree.rs diff_apply().
func (t *DataTree) DiffApply(diff *DataDiff) error

// Merge source into t in place, with conflict pre-scan. Rust: tree.rs merge().
func (t *DataTree) Merge(source *DataTree, opts MergeOpts) error
```

- `DiffOpts` mirrors Rust `DiffOpts` (the `defaults` toggle gating
  `LYD_DIFF_DEFAULTS`). `MergeOpts` mirrors Rust `MergeOpts`. **Note:** in the Rust
  oracle `MergeOpts` is currently inert (`let _ = opts;` at `tree.rs:421`) — mirror
  the type for API parity but do not invent behavior it does not have.

### 5.2 `DataDiff` (new public type)

```go
type DataDiff struct { /* owns the diff tree + materialized edits */ }

func (d *DataDiff) IsEmpty() bool
func (d *DataDiff) IsOrderedByUser() bool   // any edit IsUserOrdered() (I6)
func (d *DataDiff) Edits() []DiffEdit
func (d *DataDiff) Serialize(format Format) ([]byte, error)
```

- An **empty** diff (libyang returned no diff node) must serialize to **empty
  bytes** and report `IsEmpty() == true`, `Edits()` empty (mirror
  `DataDiff::empty()` and `tree.rs:158-166`).
- `DataDiff` **owns** its underlying diff tree and must free it exactly once via a GC
  finalizer (single-finalizer ownership — Section 7).

### 5.3 Cross-context guard (runtime — mirrors Rust's compile-time guard)

Rust guards context identity at the public `diff`/`merge` boundary with
`std::ptr::eq(self.ctx, other.ctx)` (`tree.rs:353`, `tree.rs:392`), returning
`RuleCode::DataPath` (E0006) on mismatch.

Go anchors for the equivalent check:

- `RawDataTree` (in `libyang.go`) carries both `owner *RawContext` and
  `ctx *C.struct_ly_ctx` fields.
- The public `DataTree` holds `owner *Context` + `raw`.

**Implement the guard once, at the public `Diff`/`Merge` boundary**, by comparing the
owning `*Context` identity (`t.owner == other.owner`) — the direct Go analog of
Rust's `ptr::eq` on context pointers. (Equivalently you may compare
`raw.ctx == other.raw.ctx` at the adapter; pick **one** and state which in a comment.
Do not do both.) On mismatch return an E0006 error with a message mirroring Rust
(`"diff requires both trees to share the same context"` /
`"merge requires both trees to share the same context"`).

**Test honesty:** the single-context test harness cannot construct two contexts to
*trigger* this guard, so it is **defensive / untested-by-construction**. Either (a)
add a two-context test that builds a second `Context` and asserts the E0006 error, or
(b) if that is impractical this session, explicitly mark the guard
"defensive, untested by construction" in the handoff. **Do not** claim a passing
gate for an assertion no test exercises.

---

## 6. The MERGE CONFLICT PRE-SCAN (load-bearing — ygot semantics)

libyang's `lyd_merge_siblings` silently overwrites a leaf that exists in both trees
with the source's value. Cambium rejects that instead (ygot merge semantics). The
oracle does a **pre-scan before any mutation** (`tree.rs:399-419`):

1. After the cross-context guard, compute `conflict_diff = self.Diff(source, default
   DiffOpts)`.
2. For each edit where `Op() == Replace` and the edit has a source value: read the
   **base** tree's current value at that path (`try_get(path).value_str()`). If the
   base has a value there and it **differs** from the source value, **return a
   conflict error and mutate nothing**.
3. Only if no conflict is found, call the adapter `merge`.

**Conflict error rule-code:** `RuleCode::Validate` = **E0003** (`tree.rs:414`;
`spec/rule-codes.md` documents "a `merge` conflict where both trees set the same leaf
to different values" → `CAMBIUM_E0003`). This is **distinct** from a `merge` *FFI*
failure, which is E0006 (Section 8). Wire the conflict path to E0003 explicitly — do
**not** let it inherit `codeForOp("merge")`'s E0006.

---

## 7. MERGE / DUPLICATE DOUBLE-FREE DEFENSE (first-class concern)

Every node these ops touch is owned by exactly one Go object whose finalizer frees it
exactly once. The hazards and the defenses:

- **Merge does not transfer ownership of `source`.** Flags `= 0` (Section 4.3) →
  libyang dup-copies from a `const` source. The source `RawDataTree` keeps full
  ownership and its finalizer is unaffected. **Do not** add any free of source nodes
  in the merge path. **Do not** pass `LYD_MERGE_DESTRUCT`.
- **`self.ptr` may move.** `lyd_merge_siblings` and `lyd_diff_apply_all` take
  `&self.ptr` and can re-root the tree. After each, **re-anchor**
  `self.ptr = lyd_first_sibling(self.ptr)` so the Go tree never holds a pointer to a
  detached interior node. Mirror the existing `reanchorBeforeDetach` discipline in
  `libyang.go`.
- **`DataDiff` owns its diff tree alone.** It frees via one finalizer
  (`lyd_free_all`, mirroring `RawDataDiff`'s `Drop`). It must never be freed by the
  trees it was computed from, and the trees must never free it. After `DiffApply`
  consumes a diff, the diff is **still owned by `DataDiff`** — `lyd_diff_apply_all`
  does not consume it; do not free it in apply.
- **`Duplicate` produces a fully independent tree** with its own finalizer. The
  source is untouched.
- **`runtime.KeepAlive` discipline:** every adapter method that dereferences C
  pointers must `defer runtime.KeepAlive(t)` **and** `defer runtime.KeepAlive(t.owner)`
  (and the *other* operand for `Diff`/`Merge`, and the `DataDiff` for `DiffApply`) so
  the GC cannot finalize a tree whose nodes are mid-call. Follow the existing pattern
  in `libyang.go` exactly.
- **Generation bump:** any in-place mutation (`DiffApply`, `Merge`) must call
  `incrementGen()` on the mutated tree so stale `NodeRef` handles are invalidated
  (E0007), exactly as the existing CRUD ops do.

---

## 8. ERROR RULE-CODE PARITY — exact, per-op (this is a hard gate)

The Go error rule-code **must equal** the Rust rule-code for the same failure. The
authority is `rust/cambium-core/src/tree.rs`. The mapping is **not uniform across the
four ops** — read this table carefully:

| Operation / failure | Rust mapping (authority) | Code | Go must produce |
|---|---|---|---|
| `Duplicate` FFI failure | `tree.rs:343` → `RuleCode::Serialize` | **E0004** | E0004 |
| `DataDiff.Serialize` failure | `tree.rs:163` → `RuleCode::Serialize` | **E0004** | E0004 |
| `Diff` FFI failure (incl. edit-walk) | `tree.rs:362,365` → `RuleCode::DataPath` | **E0006** | E0006 |
| `Diff` cross-context guard | `tree.rs:354` → `RuleCode::DataPath` | **E0006** | E0006 |
| `DiffApply` FFI failure | `tree.rs:382` → `RuleCode::DataPath` | **E0006** | E0006 |
| `Merge` FFI failure | `tree.rs:424` → `RuleCode::DataPath` | **E0006** | E0006 |
| `Merge` cross-context guard | `tree.rs:393` → `RuleCode::DataPath` | **E0006** | E0006 |
| `Merge` **conflict** (pre-scan) | `tree.rs:414` → `RuleCode::Validate` | **E0003** | E0003 |

### 8.1 `codeForOp` is currently WRONG for `duplicate` — FIX IT

At HEAD `788543d`, `go/cambium/cambium.go:192-193` routes **both** `"merge"` **and**
`"duplicate"` to `RuleCodeDataPath` (E0006):

```go
case "diff", "diff apply", "merge", "duplicate":
    return RuleCodeDataPath
```

This is correct for `diff` / `diff apply` / `merge` (their FFI failures are E0006),
but **wrong for `duplicate`**, whose FFI failure is **E0004 Serialize** in the
oracle. You must fix this so Go matches Rust. Two things to do:

1. **Remove `"duplicate"` from the E0006 case.** Route a `Duplicate` FFI failure to
   **E0004** — either map `"duplicate"` to `RuleCodeSerialize`, or have the duplicate
   adapter path wrap through the existing `"serialize"` op
   (`codeForOp("serialize")` already returns `RuleCodeSerialize`). Pick one and be
   consistent.
2. **`DataDiff.Serialize` failures** must produce **E0004** — route them through the
   `"serialize"` op (which already yields E0004), not through any diff op.
3. **Update the `codeForOp` coverage test** (`go/cambium/codeforop_internal_test.go`
   and/or `rulecode_test.go`) so it asserts `duplicate → E0004` and the merge-conflict
   path → E0003. The existing test currently encodes the wrong `duplicate → E0006`
   expectation; correcting it is part of this change, and the corrected test must be
   red-first against the old behavior.

**Self-check:** the prompt's two demands — "E0006 for duplicate" (old code) and
"same code as Rust" — are mutually exclusive. Rust wins. After your change,
`duplicate` FFI → E0004 in **both** languages. If any test still expects
`duplicate → E0006`, it is wrong; fix the test, do not weaken the gate.

### 8.2 Wiring summary

- Duplicate FFI error → **E0004** (Serialize-coded path; *not* `wrap("duplicate", …)`
  if that still routes E0006).
- DataDiff.Serialize error → **E0004** (serialize op).
- Diff / DiffApply / Merge FFI error and both cross-context guards → **E0006**.
- Merge conflict (pre-scan) → **E0003** (explicit, not inherited from the merge op).

### 8.3 What `-race` proves — and what it does NOT

Run all these tests under `CGO_ENABLED=1 go test -race`. But be precise about what
`-race` buys you:

- `-race` detects **data races only** (concurrent unsynchronized access). It does
  **not** detect double-free, use-after-free-after-the-fact, or memory leaks. A
  double-free you introduce will typically crash the process (possibly
  intermittently) or silently corrupt memory — `-race` will not flag it.
- Therefore the `*_freed_once` loops (Section 10) are **crash/hang regression
  guards**, not free-safety proofs. They mirror `diff.rs:239` and `duplicate.rs:154`:
  run a create/drop loop ~100×; the process must not abort or corrupt; and
  `duplicate_freed_once` ends with a **liveness assertion** on the still-live source
  (`duplicate.rs:154-180`).
- The **real** free-safety defense is the single-finalizer ownership invariant
  (Section 7), which must be **reviewed by a human** (Section 12). Do not claim
  `-race` proves freed-once.

Keep `-race` on regardless — it catches genuine GC/finalizer-timing
use-after-free under concurrent finalization, which is real value.

---

## 9. Cross-language conformance gates

The shared conformance corpus is **6 cases** (one set, two runners). Both the Rust
and Go runners must agree on all six:

`scrambled-children`, `keys-first`, `ordered-user`, `rpc-order`,
`system-list-canonical`, `ietf-interfaces`.

Your changes must not regress any of these. Where a diff/merge/duplicate result is
serialized, it must be **byte-identical** to the corresponding golden output (same
golden files the Rust tests use).

---

## 10. Test plan — mirror the Rust oracle 1:1

Write a Go test for each Rust oracle test below. Read the Rust body before writing
the Go twin; match its fixtures, golden files, and assertions exactly.

### 10.1 Diff (`rust/cambium-core/tests/diff.rs`)

| Go test | Mirrors | Assert |
|---|---|---|
| `diff_empty_when_equal` | `diff_empty_when_equal` | `IsEmpty()`, `Edits()` empty. |
| `diff_create_delete_replace` | `diff_create_delete_replace` | Specific edits with correct `Op()`/`Path()`/`Value()`; **no** edit has op `None`; **no** spurious key edit. |
| `diff_ordered_by_user_atomic` | `diff_ordered_by_user_atomic` | The user-ordered move is **one** atomic edit; `IsOrderedByUser()` true; key artifact absent (Section 4.5). |
| `datadiff_freed_once` | `datadiff_freed_once` (`diff.rs:239`) | Create+drop the diff in a `for i := 0; i < 100; i++` loop; process does not crash/hang/corrupt. (Crash-guard, not a leak check — Section 8.3.) |

### 10.2 DiffApply + Merge (`rust/cambium-core/tests/diff_apply_merge.rs`)

| Go test | Mirrors | Assert |
|---|---|---|
| `diff_apply_round_trip` | `diff_apply_round_trip` (`:130-147`) | After `base.DiffApply(diff)`, `serialize(base) == serialize(after)` **byte-identical for BOTH `Format::Xml` AND `Format::Json`** with default `SerializeFlags`. Both formats — not just one. |
| `merge_preserves_user_order` | `merge_preserves_user_order` (`:152-180`) | Target is **not empty**: first `target.NewPath("/ordered-user-demo:config", …)`, *then* `target.Merge(source)` where source is the parsed `conformance/fixtures/ordered-user/input.xml`. Assert `serialize(target, Json)` byte-equals `conformance/golden/ordered-user/output.json`. |
| `merge_conflict_errors` | `merge_conflict_errors` (`:184+`) | Differing-value collision → error with code **E0003** (Validate), and the target is **unmutated** (pre-scan fired before any change). |

> **Do not** read `merge_preserves_user_order` as "merge into a brand-new empty
> `ctx.NewData()`". The oracle creates the parent container on the target first.
> Merging into a truly empty/nil-root target hits a different libyang path and will
> not match the golden — do not "fix" a golden mismatch by changing the target shape;
> match the oracle's setup.

### 10.3 Duplicate (`rust/cambium-core/tests/duplicate.rs`)

| Go test | Mirrors | Assert |
|---|---|---|
| `duplicate_deep_copy_independent` | `duplicate_deep_copy_independent` (`:103-122`) | Copy and original are independent: mutating one (add/remove a node) does not affect the other; assert both `exists()` divergences as the oracle does. |
| `duplicate_preserves_user_order` | `duplicate_preserves_user_order` (`:127-146`) | User-ordered list order survives the copy byte-for-byte on serialize. |
| `duplicate_freed_once` | `duplicate_freed_once` (`:154-180`) | Create+drop the copy in a `for i := 0; i < 100; i++` loop; process does not crash/hang; **then assert the source is still live** (`tree.exists("/cambium-data-crud-demo:top/counter")`). |

---

## 11. SUCCESS FLOOR (sized for one session)

All of the following must hold. Anything below the floor is a fail, not a partial
pass:

1. All four ops implemented on the public Go `DataTree` / `DataDiff` with the
   signatures in Section 5.
2. Every Go test in Section 10 written **red-first** and now **green** under
   `CGO_ENABLED=1 go test -race ./...`.
3. Rule-code parity holds per Section 8: `duplicate` FFI → **E0004**,
   `DataDiff.Serialize` → **E0004**, diff/apply/merge FFI + both context guards →
   **E0006**, merge conflict → **E0003**; and `codeForOp` (+ its coverage test) is
   corrected to drop the wrong `duplicate → E0006`.
4. The diff-edit metadata walk is real (Section 4.2) — `Edits()` recovers ops from
   `yang:operation` metadata via `_canonical` + `lyd_value_get_canonical` fallback;
   **no `lyd_get_meta_value`**; the `LYS_KEY` edit is skipped at the source
   (Section 4.5).
5. Merge passes flags `0` (no `DESTRUCT`); merge conflict pre-scan fires before any
   mutation; `self.ptr` re-anchored after merge/apply; `incrementGen()` called on
   in-place mutation.
6. All 6 conformance cases still pass in both runners; serialized diff/merge/dup
   output byte-identical to the goldens.
7. `go vet ./...` and `golangci-lint run` clean.
8. **Four separate commits** (one logical op each — Duplicate, Diff, DiffApply,
   Merge — or a defensible equivalent grouping), Conventional Commits, imperative
   subject ≤50 chars, on `feat/cambium-sdk-phase1`. **Do not push.** Leave the branch
   local for review.

---

## 12. Human-review-critical zones (call these out explicitly in your handoff)

In your handoff notes, draw the reviewer's attention to these three zones — they are
where a subtle error is most costly and least likely to be caught by tests:

1. **The merge double-free / ownership boundary** (Section 7) — flags `= 0`, source
   never freed by merge, single-finalizer ownership of `DataDiff`, re-anchoring after
   mutation. `-race` does **not** prove this (Section 8.3); a human must read the
   ownership invariant.
2. **The rule-code routing change in `codeForOp`** (Section 8.1) — the
   `duplicate → E0004` correction and the merge-conflict → E0003 path are easy to get
   subtly wrong (e.g. leaving a vacuous test that passes while diverging from Rust).
3. **The cross-context guard** (Section 5.3) — defensive and likely
   untested-by-construction in a single-context harness; flag whether you added a
   two-context test or left it as documented-defensive.

End every handoff with: the four commit SHAs, the red-first evidence (the failing
test output you captured before each implementation), the `-race` + lint results, and
an explicit statement of which guards are tested vs. defensive-only.
