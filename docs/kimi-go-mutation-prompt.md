# Kimi Work Order — Cambium Go Data-Tree Mutation CRUD Parity

You are an autonomous overnight coding agent. This document is your **entire brief**. Read it fully before touching code. Everything you need — the exact API surface, the cgo hazards, the test names to mirror, the gates that must stay green — is here. Do not improvise scope. If a fact here ever conflicts with what you observe in the code, **trust the code**, note the discrepancy in your commit body, and proceed conservatively (do not expand scope to "fix" it).

---

## 0. What this phase is (and is NOT)

**Cambium is a YANG toolkit / SDK — a library.** This phase adds the **data-tree write side** to the **Go library only** (`go/`), bringing it to parity with the already-shipped Rust `cambium-core` mutation surface.

### NON-GOALS — out of scope, do not touch
- **No NETCONF / transport.** No `<edit-config>` builders, no NETCONF clients, no device sinks.
- **No gNMI / gRPC.** No transport sessions of any kind.
- **No Terraform emitters.** No `terraform-plugin-framework` resource/provider/model code.
- **No `go/codegen` change.** This is the data-tree layer, not the typed-struct generator.
- **No `diff` / `merge` / `duplicate`.** Those are the **NEXT** phase. Do not implement them, do not add stubs for them, do not add tests for them. If you find yourself reaching for `lyd_diff_*` or `lyd_merge_*`, stop — you are out of scope.

### IN scope — the only deliverable
The empty-tree constructor `NewData`, plus the five **mutation CRUD** operations on `DataTree`, mirroring Rust 1:1: `NewPath`, `SetValue`, `RemovePath`, `UnlinkPath`, `AddDefaults` — plus the `ctx`-plumbing prerequisite they all depend on, and the generation-bump staleness contract.

---

## 1. Branch & commit discipline

- Work on branch **`feat/cambium-sdk-phase1`** (current HEAD **`9ed8a65`**). It already exists and is checked out. Do **not** create a new branch. Do **not** rebase or force-push.
- **Commit each green slice separately**, with a **Conventional Commit** (imperative subject ≤ 50 chars; body only when "why" is non-obvious). One logical change per commit.
- **DO NOT push.** Leave all commits local. A human reviews before anything leaves the machine.

Suggested commit subjects (adapt as needed):
- `feat(go): plumb ctx into RawDataTree`
- `feat(go): add Context.NewData empty tree`
- `feat(go): add DataTree.NewPath mutation`
- `feat(go): add DataTree.SetValue mutation`
- `feat(go): add DataTree.RemovePath mutation`
- `feat(go): add DataTree.UnlinkPath mutation`
- `feat(go): add DataTree.AddDefaults mutation`
- `test(go): mirror Rust data_crud.rs acceptance tests`

---

## 2. TDD — non-negotiable

- **Failing test first, every slice.** Write the test, run it, **watch it fail (red)** before any production line. No production code — not even a stub signature with a body — ahead of a red test.
- Run the Go suite with the race detector on every iteration: `CGO_ENABLED=1 go test -race ./...` (from `go/`).
- Coverage is a **floor**: it must not drop. Do not mute, skip, or `t.Skip` a failing test to go green — fix it or delete it with justification.
- The three currently-skipped tests in `go/cambium/serialize_flags_test.go` (`TestSerializeWithDefaultsAll`, `TestSerializeKeepEmptyContainer`, `TestSerializeShrinkRemovesWhitespace`) are **empty `t.Skip` stubs with no assertions** — see §6 for the bodies you must author. Removing the `t.Skip` alone is not enough; an empty passing test verifies nothing and lets buggy `NewPath`/`AddDefaults` through.

---

## 3. The exact Go API to add (mirrors Rust `cambium-core`)

The Rust surface is the source of truth. Rust source lives at `rust/cambium-core/src/context.rs` (`new_data` 154–155), `rust/cambium-core/src/tree.rs` (`new_path` 515–537, `set_value` 543–547, `remove_path` 550–554, `unlink_path` 560–566, `add_defaults` 569–580) and the FFI adapter at `rust/cambium-libyang-sys/src/adapter.rs` (`new_data` 1274, `new_path` 1643–1690, `set_value` 1696–1715, `remove_path` 1718–1722, `unlink_path` 1726–1742, `add_defaults` 1745–1758, `reanchor_before_detach` 1760–1772). **Read those before coding each slice** — mirror their logic, do not reinvent it.

The **sanctioned Rust ↔ Go shape differences** (apply each — do not "fix" them):

| Rust | Go (add to `go/cambium/tree.go` + `go/cambium/cambium.go` + `go/internal/libyang/libyang.go`) | Difference rationale |
|---|---|---|
| `fn new_data(&self) -> DataTree` (context.rs:154) | `func (c *Context) NewData() *DataTree` (+ internal `func (c *RawContext) NewData() *RawDataTree`) | Owned `DataTree` → `*DataTree`. Creates an **empty** tree (`tree = nil`) carrying the plumbed `ctx`. Every `new_path` test starts from this. |
| `fn new_path(&mut self, path: &str, value: Option<&str>, opts: NewPathOpts) -> Result<NodeAddr>` | `func (t *DataTree) NewPath(path string, value *string, opts NewPathOpts) (NodeAddr, error)` | Rust `Option<&str>` → Go `*string` (nil = inner node, no value). `Result<T>` → `(T, error)`. |
| `fn set_value(&mut self, path: &str, value: &str) -> Result<bool>` | `func (t *DataTree) SetValue(path, value string) (bool, error)` | `Result<bool>` → `(bool, error)`. |
| `fn remove_path(&mut self, path: &str) -> Result<()>` | `func (t *DataTree) RemovePath(path string) error` | `Result<()>` → `error`. |
| `fn unlink_path(&mut self, path: &str) -> Result<DataTree<'ctx>>` | `func (t *DataTree) UnlinkPath(path string) (*DataTree, error)` | Owned `DataTree` → `*DataTree` (its own finalizer/Close owns the detached tree). |
| `fn add_defaults(&mut self, opts: ImplicitOpts) -> Result<()>` | `func (t *DataTree) AddDefaults(opts ImplicitOpts) error` | `Result<()>` → `error`. |

Supporting types to add in `go/cambium`:
- `NewPathOpts struct { Update, Output, Opaque bool }` (mirrors Rust `NewPathOpts { update, output, opaque }`).
- `ImplicitOpts struct { NoState, Output bool }` (mirrors Rust `ImplicitOpts { no_state, output }`).
- `NodeAddr` — an **opaque** path-key handle: a struct **wrapping the path STRING only** (mirror Rust `NodeAddr { path: path.to_string() }`, tree.rs:534–536). **NOT a raw `*lyd_node` pointer, NOT an exported string field.** Construct it in the Go public layer from the same `path` string the caller passed — the adapter does not return a node for it (see §5). Go already treats addressing as opaque (HEAD `9ed8a65`) — follow that precedent.

**Do not invent API the Rust side does not have.** No `NewPathAt`, no batch variants, no convenience overloads. One empty-tree constructor, five mutation methods, two opts structs, one opaque addr type.

### CRITICAL: every mutator bumps the generation counter

`go/internal/libyang/libyang.go` already has the generation infrastructure: `RawDataTree.gen uint64`, `Generation()`, `incrementGen()` (~276–287). All user-ordered mutators call `l.owner.incrementGen()` **only on the success branch** (precedent: `InsertLast` 712, `InsertAfter` 738, `MoveBefore` 752, `MoveAfter` 766, `insertBeforeNode` 797 — note `InsertFirst` delegates, and `MoveBefore`/`MoveAfter` are moves, not inserts). **Every one of your five new mutators MUST do the same:**

> Call `incrementGen()` on the owning `RawDataTree` **after** the libyang C call returns success, **before** returning to the caller. A **failing** mutation must **NOT** bump the generation (so a stale handle never accidentally becomes valid again).

Specifics:
- For `NewPath`, `SetValue`, `RemovePath`, `AddDefaults`: bump the **receiver** tree's own counter (`t.incrementGen()`).
- For `UnlinkPath`: bump the **SOURCE** (receiver) tree's gen. The detached returned tree is a **fresh** `RawDataTree` starting at `gen = 0` — do not bump it.
- **Note:** `intoRaw()` (~666–673) only releases ownership (nulls `t.tree`, cancels the finalizer); it does **NOT** bump gen. If your code path uses `intoRaw()`, you must still call `incrementGen()` explicitly. (`Close()` does bump, but that is unrelated to mutation.)

This is what makes outstanding **`NodeRef` / `NodeSet` / `DataSiblings` / `UserOrderedView`** handles go stale and return **`RuleCodeStale` (CAMBIUM_E0007)** via `assertValid()` (`go/cambium/tree.go` ~245–253). Rust gets this for free from the borrow checker; Go must do it at runtime.

**Mandatory regression test (must land this phase):** for **each** of the five mutators, take a `NodeRef` **before** calling the mutator, call the mutator, then assert that any accessor on the pre-mutation `NodeRef` returns **E0007**. Mirror the existing precedent `TestNodeRefStaleAfterValidate` (`go/cambium/stale_handle_test.go` 9–34).

---

## 4. Slice 0 prerequisite — plumb `ctx` into `RawDataTree`

`new_path` on an empty tree, the canonical-value fallback, `add_defaults`, and `NewData` all need `*const ly_ctx`, but `RawDataTree` currently holds only `tree`, `owner`, `gen` — **no `ctx` field** (confirmed). **This is the first slice.** Mirror Rust, where `RawDataTree` stores `ctx`.

- Add field: `type RawDataTree struct { tree *C.struct_lyd_node; owner *RawContext; ctx *C.struct_ly_ctx; gen uint64 }`.
- **Route ALL `RawDataTree` construction through the single finalizer-setting constructor** `newRawDataTree(tree, owner, ctx)` so every tree gets the finalizer + gen counter **uniformly**: the existing `ParseData` (mem path) and `ParseOp` sites, **plus** the new `NewData` site, **plus** the `UnlinkPath`-detached tree. Change `newRawDataTree`'s signature to take `ctx` and store it in the new field. Do **not** build a bare struct literal for the empty/detached tree (that would skip `SetFinalizer` and leave no finalizer backstop). `finalize()` already guards `t.tree != nil`, so a `nil`-tree `NewData` result is safe.
- **Do not add a public accessor for `ctx` on `RawContext`.** `RawContext.ctx` stays private; read it only at these construction sites within the same package and store the pointer directly in `RawDataTree` (Slice-0 decision, mirrors Rust).
- TDD: a test that constructs a tree and exercises a path needing `ctx` (e.g. `NewData` then the first `NewPath` slice) is your red test driving this plumbing.

---

## 5. Mutation cgo hazards — checklist (each item is load-bearing)

These are the Phase-3 lessons. Tick every box; a human will audit exactly these.

- [ ] **`NewData` empty tree.** `RawContext.NewData()` returns `newRawDataTree(nil, c, c.ctx)` — a tree with `tree == nil` and the plumbed `ctx`, through the finalizer-setting constructor (mirror Rust adapter.rs:1274). The public `Context.NewData()` wraps it as `*DataTree` at the boundary.

- [ ] **`lyd_new_path2` call shape — ALL three option bits go in the `options` bitmask (7th arg).** The signature is `lyd_new_path2(parent, ctx, path, value, value_size_bits, any_hints, options, &new_parent, &new_node)`. For a **0-terminated string** value, pass `value_size_bits = 0` and `any_hints = 0` (mirror adapter.rs:1655–1656,1666). Build `options` by OR-ing the bits — exactly as Rust core does (tree.rs:521–530) and the adapter passes it (adapter.rs:1661–1671):
  - `LYD_NEW_PATH_UPDATE` (0x10) when `opts.Update`,
  - `LYD_NEW_VAL_OUTPUT` (0x01) when `opts.Output`,
  - `LYD_NEW_PATH_OPAQ` (0x20) when `opts.Opaque`.

  Use the existing Go consts `NewPathUpdate` / `NewPathOutput` / `NewPathOpaque` (`libyang.go` 89–91; `NewPathOutput = uint32(C.LYD_NEW_VAL_OUTPUT)` confirms it is a flag). `LYD_NEW_VAL_OUTPUT` is a bit in `options` (vendored header tree_data.h:1258 defines it `0x01`); it does **NOT** ride in `value_size_bits`. Passing the output flag in `value_size_bits` would corrupt the call and silently break the `Output` option — do not do it.

- [ ] **`NewPath` returned-node handling + re-anchor.** `lyd_new_path2` takes separate `&new_parent` and `&new_node` out-params. The adapter (adapter.rs:1643–1690) returns **unit** — it uses these out-params **ONLY for empty-tree re-anchoring**: if `t.tree` was `nil`, anchor to `new_parent` (or `new_node` if `new_parent` is nil) via `lyd_first_sibling`. If `t.tree` was non-nil, re-anchor `t.tree = C.lyd_first_sibling(t.tree)` (an absolute path may insert a new top-level sibling before the current root). **Never store a `*lyd_node` in `NodeAddr`** — build `NodeAddr` from the path string in the public layer (see §3). The Go read path already uses `lyd_first_sibling` (342, 414, 443, 574) — same function, no `cam_*` wrapper needed.

- [ ] **Ownership transfer.** `lyd_new_path2` builds nodes into the tree directly (no Go-owned handle handed in), so no `intoRaw()` is needed for `NewPath` itself. But if any code path hands a Go-owned node to libyang, use the existing `intoRaw()` pattern (`libyang.go` 666–673) **before** the C call and `C.lyd_free_tree(node)` on the error branch to avoid a leak (precedent: insert methods at 708/709, 734/735, 793/794).

- [ ] **No double-free on delete; re-anchor BEFORE detach.** `RemovePath` = `find_path` → `reanchor_before_detach` → `lyd_free_tree(node)` (frees the subtree only). `UnlinkPath` = `find_path` → `reanchor_before_detach` → `lyd_unlink_tree(node)` (detaches, does **NOT** free) → transfer the detached node into a **fresh** `RawDataTree` (via the finalizer-setting constructor, see §4) so **exactly one** Drop/Close owns it. **NEVER** call `lyd_free_all` on a still-linked interior node. Implement `reanchor_before_detach` exactly as Rust (adapter.rs:1760–1772):
  > If `t.tree` is nil, return. Let `root = lyd_first_sibling(t.tree)`. If `root == node` (the target IS the current root), set `t.tree = node.next` (**may be nil → the tree becomes empty**). Else set `t.tree = root`. Do this **BEFORE** `lyd_free_tree` (RemovePath) / `lyd_unlink_tree` (UnlinkPath).

  Re-anchoring AFTER the node is freed/detached is a use-after-free.

- [ ] **`AddDefaults` re-anchor.** `lyd_new_implicit_all(&t.tree, ctx, options, NULL)` may insert an implicit node before the current first sibling. After success, if `t.tree != nil`, set `t.tree = C.lyd_first_sibling(t.tree)` (mirror adapter.rs:1745–1758). `ImplicitOpts.NoState` → `LYD_IMPLICIT_NO_STATE`; `ImplicitOpts.Output` → `LYD_IMPLICIT_OUTPUT`.

- [ ] **`SetValue` rejects key / non-term nodes, then handles the tristate.** `lyd_change_term` is UB on a container/list/rpc/action/notification node. Gate **before** the call: resolve the path, require `schema != nil` **and** `nodetype ∈ {LYS_LEAF, LYS_LEAFLIST}`; otherwise return **E0006** (`RuleCodeDataPath`). **The nodetype gate already exists at `libyang.go` 604–605 (`nt == C.LYS_LEAF || nt == C.LYS_LEAFLIST`) — reuse it; do not write a new one.** Then handle the **tristate** return (mirror adapter.rs:1696–1715):
  - `LY_SUCCESS` → `(true, nil)` — bump gen.
  - `LY_EEXIST` → `(true, nil)` — same value but default-flag cleared, a **real** state change; bump gen. Encode this choice as a named const/comment.
  - `LY_ENOT` → `(false, nil)` — identical value, a true **no-op**. **DO NOT bump gen.** This is the ONE deliberate exception to "bump on every non-error return": handles taken before an identical-value `SetValue` stay valid (and `set_value_reports_change` expects this). Do not "normalize" it to bump on every non-error.
  - any other code → `(false, E0006)`.

- [ ] **`runtime.KeepAlive` on every cgo-touching method.** GC can run during a cgo call and fire a finalizer mid-call → use-after-free. Every new method that calls C must `defer runtime.KeepAlive(t)` **and** `defer runtime.KeepAlive(t.owner)` at the top, before the C calls (precedent in the read/insert path). For `UnlinkPath`, also keep the returned tree's owner alive.

---

## 6. Authoring the three `serialize_flags_test.go` bodies

These functions currently contain only `t.Skip(...)`. **Replace the `t.Skip` with full bodies and real assertions** (mirror the golden-bytes pattern already in the same file at ~55–60: `normalize()` + compare against `goldenXML`/`goldenJSON`):
- `TestSerializeWithDefaultsAll` — build a tree via `NewData` + `NewPath`, call `AddDefaults`, serialize with `WithDefaults(all)`, and assert the default leaves appear in the output.
- `TestSerializeKeepEmptyContainer` — `NewPath` an empty container, serialize with the keep-empty-container flag, and assert the empty container is present.
- `TestSerializeShrinkRemovesWhitespace` — `NewPath` content, serialize with the shrink flag, and assert the shrunk output (no insignificant whitespace).

If a Rust equivalent exists, mirror its observable; otherwise derive assertions from the existing serialize-flag golden helpers in this file (the `goldenXML`/`goldenJSON` + `normalize` pattern). Do **not** leave any of the three assertion-free.

---

## 7. Cross-language gates — all must stay green

- [ ] **Go conformance stays 6/6 byte-identical.** The six fixtures (`scrambled-children`, `keys-first`, `ordered-user`, `rpc-order`, `system-list-canonical`, `ietf-interfaces`) must still serialize byte-for-byte against the golden outputs in `conformance/golden/*/`. Mutation code must not perturb the parse/serialize-read-only conformance cases. Run the Go conformance harness and confirm 6/6.

- [ ] **RuleCode parity.** All five CRUD ops already map to **`RuleCodeDataPath` (CAMBIUM_E0006)** in `codeForOp` (`go/cambium/cambium.go` 188–191: "new path", "set value", "remove path", "unlink path", "add defaults"). Wrap every failure through the existing `wrap(op, err)` path so the code is stable. Stale-handle use returns **`RuleCodeStale` (CAMBIUM_E0007)** (`cambium.go` ~170). Match `spec/rule-codes.md` (E0006 = "data-tree access/mutation by path or XPath failed"; E0007 = stale handle). Same input → identical RuleCode in both languages.

- [ ] **The demo module is an INLINE literal, not a committed fixture.** `cambium-data-crud-demo` is an inline string in `rust/cambium-core/tests/data_crud.rs` (~36–72), written to a temp dir at test time — there is no `*.yang` on the search path for `LoadModule` to find. Replicate it **verbatim** as a Go test helper that `os.WriteFile`s the YANG to `t.TempDir()`, calls `ctx.SetSearchPath(dir)`, then `ctx.LoadModule("cambium-data-crud-demo")` — follow the existing precedent in `go/cambium/tree_test.go` (~177 and ~344). Keep namespace `urn:cambium:data-crud`, prefix `cdc`, yang-version 1.1, revision `2026-06-14`, and the `top` container (`enabled` boolean default "true", `counter` uint64, `nested/name` string, `item` list keyed `id` with `value` uint64, `tags` leaf-list) **identical** so paths match.

- [ ] **Same observable as `data_crud.rs` — mirror each named test.** Add a Go acceptance test for each of these from `rust/cambium-core/tests/data_crud.rs`, asserting the **same observable behavior** (same module/paths):
  - `new_path_then_serialize_declaration_order` (82–127) — nodes built in reverse declaration order serialize in **schema declaration order** (I2 invariant).
  - `new_path_creates_nodes` (130–145) — nested leaf "42" reads back as `Uint64(42)`.
  - `new_path_updates_existing_leaf` (148–174) — `Update:true` replaces 5→7, no E0006.
  - `new_path_list_entry` (309–334) — `/...:top/item[id='a']` then child leaf; 2 children.
  - `set_value_reports_change` (177–197) — `SetValue("3")`/`SetValue("4")` → true; repeat `SetValue("4")` → false.
  - `remove_path_removes_subtree` (200–226) — container + child gone; sibling counter stays.
  - `unlink_path_detaches_subtree` (229–257) — gone from source; detached tree still serializes the nested structure.
  - `add_defaults_adds_default_leaves` (260–272) — `enabled` materializes as `Bool(true)`, `IsDefault()` true.
  - `validate_after_mutation_passes` (275–290) — validate(present) passes after mutation.
  - `invalid_path_returns_data_path_rule_code` (293–306) — bad schema path → **E0006**.

  Plus the **five stale-handle regression tests** from §3 (one per mutator).

- [ ] **Assert RuleCode via the established idiom**, not `errors.Is` with a sentinel (the `cambium.Error` type may not implement `Is`, yielding a non-compiling or always-false check). Use the type-assert precedent from `stale_handle_test.go` 27–32: `e, ok := err.(*cambium.Error); ok && e.RuleCode() == cambium.RuleCodeDataPath`. For `invalid_path`, assert `RuleCodeDataPath` (E0006); for the five stale regressions, assert `RuleCodeStale` (E0007).

- [ ] **Run the Rust leg once to confirm no regression.** You are not changing Rust, but run `cargo test --workspace` once at the end and confirm green — the Rust side is the parity oracle and must remain passing.

Full gate command set before each commit:
- Go: `CGO_ENABLED=1 go build ./... && CGO_ENABLED=1 go test -race ./... && go vet ./... && golangci-lint run` (from `go/`)
- Rust (final, once): `cargo test --workspace`

---

## 8. Success floor vs. deferrable

### SUCCESS FLOOR — the minimum that MUST land (phase fails without all of it)
0. `ctx` plumbed into `RawDataTree`, and **`Context.NewData()` / `RawContext.NewData()`** added (empty tree with plumbed ctx, through the finalizer-setting constructor).
1. **All five mutators** implemented and exported: `NewPath`, `SetValue`, `RemovePath`, `UnlinkPath`, `AddDefaults`.
2. **Every mutator calls `incrementGen()` on success** (per the §3 rules, incl. the `SetValue` `LY_ENOT` exception and `UnlinkPath` bumping the source) — proven by **five** stale-handle regression tests (NodeRef taken before each mutator returns **E0007** after).
3. The ten `data_crud.rs` acceptance tests mirrored in Go and passing, with the demo module replicated inline.
4. The three `serialize_flags_test.go` stubs given **real bodies + assertions** (not merely un-skipped) and passing.
5. Conformance **6/6** byte-identical; Go `go test -race ./...`, `go vet`, `golangci-lint` green; Rust `cargo test --workspace` green.

### Explicitly DEFERRABLE (do NOT block the phase on these; leave a `// TODO(next-phase)` if touched)
- `diff` / `merge` / `duplicate` — **NEXT phase**, hard out of scope here.
- Any non-path addressing convenience. Path-based mutation only (spec D-2).

### Already present — do NOT re-add (re-adding risks a C redefinition / build break)
- `cam_lyd_get_value` already exists (`libyang.go` 40, used at 520). Read leaf values via the existing read-path pattern; do not add a duplicate static-inline wrapper.
- The leaf/leaf-list nodetype gate already exists (`libyang.go` 604–605) — reuse it for `SetValue` term-checking.

---

## 9. Human-review-critical zones (point the adversarial reviewer here)

When you finish, the post-run review will focus on exactly these zones — make them clean, commented (WHY, not WHAT), and easy to audit:

1. **Ownership / double-free on delete** — `RemovePath` (`reanchor_before_detach` → `lyd_free_tree`, subtree only) vs. `UnlinkPath` (`reanchor_before_detach` → `lyd_unlink_tree` + transfer into a fresh `RawDataTree` with exactly one owner). The reviewer will look for a missing/AFTER re-anchor (`new root = node.next` when target is root, may be nil), an accidental `lyd_free_all`, or two finalizers racing on the detached node.
2. **Generation-bump staleness on every mutator** — all five bump `incrementGen()` **only on success**, **after** the C call; `SetValue` does **not** bump on `LY_ENOT`; `UnlinkPath` bumps the source tree; `intoRaw()` does not bump so an explicit call is present where used. The five regression tests must actually prove E0007 on a pre-mutation `NodeRef`.
3. **`NewPath` call + node handling** — `lyd_new_path2` arg order with `value_size_bits = 0` / `any_hints = 0` for string values, ALL option bits in the `options` bitmask (no flag smuggled into `value_size_bits`), out-params used only for empty-tree re-anchor, non-empty re-anchor via `lyd_first_sibling`, and `NodeAddr` built from the path string (never a `*lyd_node`).

---

## 10. Working notes

- `go/internal/libyang/libyang.go` is the FFI adapter (cgo); `go/cambium/tree.go` + `go/cambium/cambium.go` are the public domain layer. Keep libyang/cgo types out of the public layer — map at the boundary, exactly as the read path and user-ordered layer already do.
- The user-ordered insert/move methods (`libyang.go` ~693–797, public wrappers `cambium.go` ~362–390) are your closest working precedent for the cgo + `intoRaw` + error-free + `incrementGen` + `KeepAlive` pattern. Copy their shape.
- Use `/.planning/` for scratch. Promote any durable decision into `docs/` or `/spec/`.
- If a fact here conflicts with the code, **trust the code**, note the discrepancy in your commit body, and proceed conservatively (do not expand scope to "fix" it).
