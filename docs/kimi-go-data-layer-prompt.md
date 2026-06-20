# Kimi Work-Order — Cambium Go SDK Phase 1: data-tree READ / NAVIGATION / VALIDATION parity

You are an autonomous overnight coding agent. This document is your entire brief. Read it
top to bottom before touching a file. Everything you need to mirror is in the existing Rust
crate and the shared `/spec` contract; **do not invent API the Rust side does not have.** When
in doubt, open the cited oracle file and mirror it observation-for-observation.

---

## 0. What Cambium is — and what this phase is NOT (anti-drift, read first)

Cambium is a **library/SDK**: an order-correct YANG toolkit built on a vendored, statically
linked libyang. It loads YANG, builds an order-correct data tree, validates, reads/navigates
that tree, and serializes via a single ordered libyang walk. It is a **foundation** for
higher-level tools, not those tools.

**NON-GOALS for this phase (and forever, in this repo). If you find yourself doing any of
these, STOP — you have drifted:**

1. **No NETCONF / transport.** No `<edit-config>` builders, no NETCONF client, no session,
   no device sink/fake.
2. **No gNMI / gRPC / streaming.** No service registration, no proto handlers. gNMI agents
   are *downstream consumers* of Cambium, not part of it.
3. **No Terraform provider emitters / codegen work.** Do not touch `go/codegen/`.
4. **No OpenConfig / vendor schema assumptions.** Cambium loads arbitrary user YANG.
5. **No libyang version-shimming.** The internal cgo layer reads struct fields/flags directly
   from the vendored headers; a libyang major bump is allowed to break it.

**This phase adds a data-tree READ / NAVIGATION / VALIDATION-diagnostics surface to the Go
library only**, bringing `go/cambium` to parity with the Rust `cambium-core` read side that
already exists in `rust/cambium-core/src/tree.rs`. Mutation CRUD (`new_path`/`set_value`/
`remove_path`) and diff/merge are out of THIS phase except where a generation counter must be
incremented (see §5). Stick to read + navigation + structured validation.

---

## 1. Branch, build, and commit discipline

- **Branch:** `feat/cambium-sdk-phase1`. It already exists and `HEAD` is `fcbc9b0`. Work
  there. **Do NOT create a new branch. Do NOT push. Do NOT open a PR.** Local commits only.
- **Build once on entry** (the static engine is gitignored and must be built):
  ```sh
  bash go/internal/libyang/build.sh           # two-stage static PCRE2 -> libyang into .build/
  go -C go build ./... && go -C go test ./... && go -C go vet ./...
  ```
  Use `go -C go ...` (or absolute paths) for every module command — do not rely on a persisted
  `cd`. Confirm green before you write a line. If the build is broken on entry, fix the build
  first and stop to report — do not pile new work on a red tree.
- **Commit each green slice separately**, Conventional Commit, imperative subject ≤50 chars.
  One logical change per commit. Example subjects:
  - `feat(go): cam_lyd_child/cam_lyd_get_value cgo wrappers`
  - `feat(go): RawNodeRef read primitives (value, path, children)`
  - `feat(go): NodeRef + Get/TryGet/Exists/Select public API`
  - `feat(go): generation-tagged stale-handle check (E0007)`
  - `feat(go): structured ValidationErrors diagnostics`
- A slice is "green" only when, for the whole `go` module: `go -C go build ./...`,
  `go -C go test ./... -race`, and `go -C go vet ./...` all pass, AND the conformance corpus is
  still 6/6 byte-identical (`go -C go run ./cmd/cambium all` exits 0). Never commit red.

---

## 2. TDD — non-negotiable

- **Failing test first, every slice.** Write the test, run it, watch it fail (red) for the
  right reason, then write the minimum production code to make it green. No production code —
  not even a stub type — ahead of a red test.
- Run the data-read and validation tests with `-race` (the generation/KeepAlive logic and the
  ctx-global error log are the whole point).
- **Mirror the Rust oracle observation-for-observation.** Each Go test asserts the *same
  observable* as its named Rust counterpart. The two oracle files are canonical — **open both
  before writing any test:**
  - `rust/cambium-core/tests/data_read.rs` — read/navigation oracle.
  - `rust/cambium-core/tests/data_validation.rs` — structured-validation oracle.
  - `rust/cambium-core/tests/user_ordered_read.rs` — read-only ordered-view oracle.
- **Read-side demo module** (mirror `data_read.rs`). Use the **same module and the same
  scrambled input XML** so Go assertions line up with Rust's: module `cambium-data-read-demo`
  (namespace `urn:cambium:data-read`, prefix `cdr`, yang-version 1.1) with `container top`
  holding `rw-flag` (boolean, default `true`), `ro-counter` (uint64, config false),
  `mandatory-leaf`, `all-builtins` (int64), `dec64` (decimal64 fraction-digits 4),
  `status-enum` (enumeration up/down/unknown). Input XML is deliberately scrambled
  (`mandatory-leaf, rw-flag, status-enum, dec64, all-builtins`); the system-ordered container
  must canonicalize to schema order on read.
- **IMPORTANT — Go validation tests build the failing tree by PARSE, not new_path.** The Rust
  validation oracle constructs its failing trees with `new_path(...)`, which is a Phase-2
  MUTATOR that is **out of scope** for this Go phase (§0, §8). Therefore each Go validation
  test must craft a failing instance document (XML or JSON) string and load it through the
  existing `DataTree` parse entrypoint with validation deferred (parse-only / no-validate
  mode), then call `Validate(...)` to trigger the failure. Mirror the *observable diagnostics*
  of each named Rust test, not its construction method. Use a `cambium-validation-demo`-shaped
  module (namespace `urn:cambium:validation-demo`) that reproduces the same constraints the
  Rust module uses: a `must`+`when` pair under `top/c` and `top/y`/`top/z` tagged
  `error-app-tag "must-violation"`, a `mandatory` leaf with no app-tag, and a `leafref`
  (`top/ref`) with `require-instance true` and `error-app-tag "instance-required"`.
- Golden bytes are **generated and reviewed at implementation time, never hand-authored**. Do
  not blind-accept any regenerated fixture; the existing 6 conformance goldens are frozen.
- Coverage is a floor. Every error/edge branch you add (miss, non-leaf value, stale handle,
  empty tree, interior-NUL, system-ordered view) gets a test. Do not mute or skip a failing
  test — fix or delete.

---

## 3. The exact Go API to add (mirror Rust; apply only the sanctioned shape differences)

Source of truth is `rust/cambium-core/src/tree.rs` (`NodeRef<'tree>`, `NodeSet<'tree>`,
`DataSiblings<'tree>`, `Value`, `Decimal64`, `DataTree::get/try_get/exists/select/root_nodes`,
and `NodeRef::as_user_ordered` at L684), `rust/cambium-core/src/list.rs` (`UserOrderedView` at
L145–220), and `rust/cambium-core/src/error.rs` (`Diagnostic`/`ValidationCode`/`ErrorType`/
`ValidationErrors` at L57–130). Names, ordering semantics, and rule codes MUST be 1:1. The
**only** sanctioned Rust↔Go shape differences (`spec/api.md` "Cross-language shape contract")
are:

| Rust | Go translation to apply |
|---|---|
| `Result<T, E>` | `(T, error)` |
| `Option<T>` | `(T, ok bool)` |
| `Drop` (RAII) | `Close()` |
| `impl Iterator<Item = T>` | `[]T` (return a materialized slice) |
| consuming `self` | method on the owner returning a value/`(owner, error)` |
| enum with data (`Value`) | tagged struct: a `Kind` field + typed accessors |
| `#[non_exhaustive]` enum | a Go enum **with a zero/None sentinel** so absent is representable; never assume the variant set is closed |
| `SerializeFlags::default()` siblings=true | Go `SerializeFlags{}` has `Siblings:false`; `DefaultSerializeFlags()` is the golden profile (already implemented — do not change) |
| **borrowed `NodeRef<'tree>`** | **Go `NodeRef` is a value type holding its owning `*DataTree` + a generation tag; invalidated on mutation/Close → `CAMBIUM_E0007` (Stale) at runtime** (Go has no lifetimes) |

Everything else is 1:1. Add to package `github.com/signalbreak-labs/cambium/go/cambium` (zero
cgo in this package — all C stays in `internal/libyang`):

### 3.1 `NodeRef` (value type, generation-tagged)

```go
// NodeRef is a borrowed handle to one node in a DataTree. It is a value type
// holding its owning *DataTree, the node's absolute canonical path, and the
// tree generation at which it was minted. Any tree mutation (or Close) advances
// the generation and invalidates every outstanding NodeRef: a subsequent read
// returns CAMBIUM_E0007 (Stale). This is the Go runtime equivalent of Rust's
// borrow-checked NodeRef<'tree>.
type NodeRef struct {
    tree *DataTree
    path string
    gen  uint64
}

func (n NodeRef) Path() (string, error)                    // absolute canonical path
func (n NodeRef) Name() (string, error)                    // schema-local name (strip preds + module prefix)
func (n NodeRef) ValueStr() (s string, ok bool, err error) // canonical value; ok=false for non-term nodes
func (n NodeRef) Value() (v Value, ok bool, err error)     // typed; ok=false for non-term nodes
func (n NodeRef) IsDefault() (bool, error)
func (n NodeRef) Schema() (SchemaNodeRef, error)           // bridge to compiled schema (reuse existing SchemaNodeRef)
func (n NodeRef) Children() (DataSiblings, error)          // immediate children, declaration order
func (n NodeRef) Siblings() (DataSiblings, error)          // self + siblings, declaration order
func (n NodeRef) AsUserOrdered() (v UserOrderedView, ok bool, err error) // read-only positional view; ok=false for system-ordered
```

- `Path()`/`Name()` return `error` (not pure values) because they assert the generation first —
  they can return `E0007`. `Name()` mirrors Rust `path_node_name`: take the segment after the
  last `/`, strip list predicates (`[...]`) and the `module:` prefix
  (`/mod:parent/list[key='x']/leaf` → `leaf`).
- `ValueStr`/`Value` return `ok=false` (NOT an error) for non-term nodes (containers/lists),
  mirroring Rust `Ok(None)`. Reserve errors for stale handle / FFI failure / a leaf whose
  schema type can't be resolved.
- `Value` dispatches on the leaf's base type (reuse the existing `TypeInfo`/`ResolvedType`/
  `BaseType` schema introspection already in `go/cambium/schema.go`). `Decimal64` is stored
  fixed-point: canonical `12.34` with 4 digits ↔ `raw=123400`.
- `Schema()` is gated by `node_schema_bridge` (§7.3); it resolves the node path → C
  `node.schema` pointer → the existing `SchemaNodeRef` over the compiled forest. Verify it
  returns the **schema node for that exact data node**, not a sibling.
- `AsUserOrdered` takes the owner by value and returns a **read-only** view. `ok=false` for a
  system-ordered list (do not error). Mirrors Rust's consuming
  `NodeRef::as_user_ordered(self) -> Result<Option<UserOrderedView>>` (tree.rs L684).

### 3.2 `UserOrderedView` (NEW Go type — read-only twin of the mutable list handle)

`UserOrderedView` does **not** exist in Go yet. Define it as NEW code — a generation-tagged
value type that is the **read-only** counterpart of the existing mutable `RawUserOrderedList`/
`DataTree.UserOrderedListAt` handle. It shares **no** mutators. Mirror `list.rs` L145–220:

```go
// UserOrderedView is a read-only positional view of a single `ordered-by user`
// list (or leaf-list) instance, in byte-exact insertion order (invariant I6).
// It is generation-tagged like NodeRef; reordering stays exclusively on the
// mutable DataTree.UserOrderedListAt handle.
type UserOrderedView struct {
    tree *DataTree
    path string
    gen  uint64
}

func (v UserOrderedView) Len() int
func (v UserOrderedView) IsEmpty() bool
func (v UserOrderedView) Get(i int) (NodeRef, bool)              // ok=false out of range
func (v UserOrderedView) Iter() []NodeRef                        // insertion order
func (v UserOrderedView) FindByKey(keys [][2]string) (idx int, ok bool) // [{name,value}...]; ok=false on no match
```

`FindByKey` mirrors `UserOrderedView::find_by_key(&self, keys: &[(&str,&str)]) -> Option<usize>`
(list.rs L199): match a list entry whose every `(name,value)` key pair matches, return its
positional index. The Go shape `[][2]string` carries `[predicate-name, predicate-value]` pairs.

### 3.3 `Value` enum (mirror Rust `tree.rs` `Value`, `#[non_exhaustive]`)

```go
type ValueKind int
const ( ValueInt8 ValueKind = iota; ValueInt16; ValueInt32; ValueInt64;
        ValueUint8; ValueUint16; ValueUint32; ValueUint64; ValueDecimal64;
        ValueBool; ValueEmpty; ValueStr; ValueBinary; ValueEnum; ValueBits;
        ValueIdentityref; ValueInstanceIdentifier )
```

Use a tagged struct (a `Kind` plus typed accessors, or one struct with the populated field).
`Bits` is a `[]string` split on whitespace; `Binary` is base64-decoded `[]byte`; `Enum`/
`Identityref`/`InstanceIdentifier` stay strings; union/leafref resolve to their realtype before
parsing (union default → `Str`). Add `Decimal64{ raw int64; fractionDigits uint8 }` with a
`String()` that reproduces RFC-7950 canonical form (left-pad fraction, strip trailing zeros
keeping ≥1 digit, preserve a single minus, total at `int64` min). Match Rust `Decimal64::Display`
exactly (`12.34`, not `12.3400`; use `unsigned_abs`-equivalent so there is no double-minus).

### 3.4 `DataTree` read methods (mirror Rust `DataTree::get/try_get/exists/select/root_nodes`)

```go
func (t *DataTree) Get(path string) (NodeRef, error)      // miss -> E0006 DataPath
func (t *DataTree) TryGet(path string) (NodeRef, bool)    // miss -> ok=false, no error
func (t *DataTree) Exists(path string) bool               // clean false on empty tree / miss
func (t *DataTree) Select(xpath string) (NodeSet, error)  // document order (I5)
func (t *DataTree) RootNodes() (DataSiblings, error)      // top-level sibling chain, declaration order
```

`Get` on a missing path returns `wrap("get", …)` → `RuleCodeDataPath` (E0006); `codeForOp`
(cambium.go L188–190) already maps `get`/`try get`/`exists`/`select`/`root nodes` to E0006 —
reuse it. `Exists` must soft-fail (false) on a missing path / empty tree, mirroring Rust
`find_node` catching `LY_ENOTFOUND`/`LY_EINCOMPLETE` → `Ok(None)` (never an error).

### 3.5 `NodeSet` and `DataSiblings` (iterator → slice)

```go
type NodeSet struct { tree *DataTree; paths []string; gen uint64 }
func (s NodeSet) Len() int
func (s NodeSet) IsEmpty() bool
func (s NodeSet) Get(i int) (NodeRef, bool)
func (s NodeSet) Iter() []NodeRef        // document order

type DataSiblings struct { tree *DataTree; children []rawChildInfo; gen uint64 }
func (s DataSiblings) Len() int
func (s DataSiblings) IsEmpty() bool
func (s DataSiblings) Get(i int) (NodeRef, bool)
func (s DataSiblings) Iter() []NodeRef   // declaration order
```

`Iter()` returns a materialized `[]NodeRef` (the sanctioned iterator→slice translation). The
NodeRefs it yields carry the set/siblings generation; if the tree mutated after the set was
built, every yielded NodeRef read returns `E0007`.

### 3.6 Structured validation diagnostics (area E) — mirror error.rs EXACTLY

`DataTree.Validate(mode)` already exists (cambium.go L312–324) but flattens to a single error.
Extend it so a validation failure returns a **list**. **The Go shapes below mirror
`rust/cambium-core/src/error.rs` L57–130 exactly — do not drift them:**

```go
// ErrorType mirrors Rust error.rs ErrorType (#[non_exhaustive]).
type ErrorType int
const ( ErrorTypeApplication ErrorType = iota // application-level validation/data error
        ErrorTypeProtocol )                    // transport/protocol-level error

// ValidationCode mirrors Rust error.rs ValidationCode (#[non_exhaustive]).
// ValidationCodeNone is the Go equivalent of Rust's `Option<ValidationCode>::None`
// (the variant could not be inferred). It MUST exist and be the zero value.
type ValidationCode int
const ( ValidationCodeNone ValidationCode = iota // unclassified == Rust None
        ValidationMust          // a `must` constraint was violated
        ValidationWhen          // a `when` condition was not satisfied
        ValidationMandatory     // mandatory / min-/max-elements / choice
        ValidationLeafref       // leafref or instance-identifier unresolved
        ValidationInvalidValue ) // type or semantic data restriction

// Diagnostic mirrors Rust error.rs Diagnostic. Optional Rust fields map to
// empty-string-means-absent (DataPath/SchemaPath/ErrorAppTag) and
// ValidationCodeNone-means-absent (ValidationCode == Rust Option::None).
type Diagnostic struct {
    Code           RuleCode       // always RuleCodeValidate (E0003)
    Message        string
    DataPath       string         // "" == Rust None
    SchemaPath     string         // "" == Rust None
    ErrorType      ErrorType      // typed enum, NOT a string
    ErrorAppTag    string         // "" == Rust None
    ValidationCode ValidationCode // ValidationCodeNone == Rust None
}

type ValidationErrors struct { diagnostics []Diagnostic }
func (v *ValidationErrors) Error() string
func (v *ValidationErrors) Diagnostics() []Diagnostic
func (v *ValidationErrors) Len() int
func (v *ValidationErrors) IsEmpty() bool
func (v *ValidationErrors) Primary() (Diagnostic, bool) // ok=false when empty (Rust primary() -> Option)
```

`Validate` returns an `error` that wraps `*ValidationErrors` so `errors.As(err, &target)` (with
`var target *ValidationErrors`) recovers the full list. The top-level rule code stays `E0003`
for every diagnostic; `error-app-tag`/`data_path`/`schema_path`/`validation_code`/`error_type`
are informational sub-fields under it (no renumbering).

**Classify `ValidationCode`/`ErrorType` byte-identically to the Rust adapter**
(`rust/cambium-core/src/tree.rs` `diagnostic_from_raw` + `validation_code_from_apptag`,
L583–622). `ErrorType` is always `Application` for validation diagnostics. `ValidationCode`
is derived in this exact priority order from each raw item's `apptag`, then `message`, then
`vecode_str`:

1. `apptag == "must-violation"` → `Must`
2. `apptag == "instance-required"` → `Leafref`
3. `apptag ∈ {"too-few-elements","too-many-elements","missing-choice"}` → `Mandatory`
4. else `message` starts with `"When condition"` → `When`
5. else `message` starts with `"Mandatory node"` or `"Mandatory choice"` → `Mandatory`
6. else `message` contains `"min-elements"` or `"max-elements"` → `Mandatory`
7. else `vecode_str` equals (case-insensitive) `"data"` → `InvalidValue`
8. else → `ValidationCodeNone` (absent)

---

## 4. cgo work in `internal/libyang` (the only place C is touched)

Add the internal primitives the public API needs. **Coarse-grained FFI is mandatory: every
multi-result read materializes ALL results in ONE cgo pass and returns Go-owned data (path
strings, value copies) before any loop. No per-node cgo, no C→Go callback.**

### 4.1 New `cam_*` C wrappers (in the cgo preamble alongside `cam_lyd_parent`/`cam_lyd_schema_name`)

`lyd_child(node)` and `lyd_get_value(node)` are **static inline** in `tree_data.h`. cgo *can*
compile static-inline (it builds a real C TU; the existing `nth()` at libyang.go L466 already
calls `C.lyd_child` directly). To match the Rust adapter's explicit-wrapper discipline and keep
call sites uniform/gated, add thin `cam_*` wrappers. Pin the bodies exactly:

```c
static struct lyd_node *cam_lyd_child(const struct lyd_node *node)   { return lyd_child(node); }      // NULL for non-inner
static const char     *cam_lyd_get_value(const struct lyd_node *node){ return lyd_get_value(node); }  // gate to LYS_LEAF/LEAFLIST
static struct lyd_node *cam_set_dnode(const struct ly_set *set, uint32_t i){ return set->dnodes[i]; } // union accessor
```

- The `ly_set` node array member is named **`dnodes`** (`struct lyd_node **dnodes` — verified
  in the vendored `libyang/set.h` L52), and the count is `set->count`. **Do NOT reference
  `__bindgen_anon_1`** — that is a Rust-bindgen artifact, irrelevant to cgo and will not
  compile.
- `lyd_find_path`, `lyd_find_xpath`, `ly_set_free` are `LIBYANG_API_DECL` (extern) — call from
  cgo directly, no wrapper needed.
- `lyd_path` is 4-arg: `char *lyd_path(const struct lyd_node *node, LYD_PATH_TYPE pathtype,
  char *buffer, size_t buflen)` (verified `tree_data.h` L2255). Call it in allocate-and-return
  mode: `C.lyd_path(node, C.LYD_PATH_STD, nil, 0)` returns a heap `char*` you must free with
  `C.free` (matches the Rust adapter's `lyd_path(node, LYD_PATH_STD)`).

### 4.2 New `RawDataTree` / `RawNodeRef` read methods

```go
type RawNodeRef struct { node *C.struct_lyd_node; owner *RawDataTree }

func (t *RawDataTree) FindNode(path string) (*RawNodeRef, bool, error)  // lyd_find_path; LY_ENOTFOUND/LY_EINCOMPLETE -> (nil,false,nil)
func (t *RawDataTree) XPathPaths(xpath string) ([]string, error)        // lyd_find_xpath + ly_set walk, document order
func (t *RawDataTree) RootNodes() ([]RawChildInfo, error)               // lyd_first_sibling + .next walk
func (t *RawDataTree) ChildrenOf(path string) ([]RawChildInfo, error)   // resolve path, then cam_lyd_child + .next walk
func (t *RawDataTree) SiblingsOf(path string) ([]RawChildInfo, error)
func (t *RawDataTree) ValueStr(path string) (string, bool, error)       // cam_lyd_get_value; (.,false,nil) for non-term
func (t *RawDataTree) IsDefault(path string) (bool, error)              // node.flags & LYD_DEFAULT
func (t *RawDataTree) SchemaPtr(path string) (unsafe.Pointer, bool, error) // node.schema (bridge to forest)
func (t *RawDataTree) Generation() uint64                               // current mutation counter
```

`RawChildInfo` mirrors Rust: `{ Path string; Name string; IsDefault bool }`. Build the absolute
path via `lyd_path(node, LYD_PATH_STD, nil, 0)`, freeing each returned `char*` with `C.free`.

- **`XPathPaths`** evaluates against a context node: `lyd_find_xpath`'s first arg is `ctx_node`,
  **not** the bare tree. Pass `t.tree` (the canonical first sibling that `Validate` re-anchors)
  as `ctx_node`. An absolute XPath is evaluated from the document root regardless, but the
  `ctx_node` must be a real node in the tree, or you get empty/partial results. Materialize every
  matched node's absolute path in one walk (via `cam_set_dnode(set, i)` for `i < set.count`,
  each `lyd_path` freed), THEN free the set **once** with `ly_set_free(set, nil)` (NULL
  destructor — the nodes are owned by the tree, not the set), and `runtime.KeepAlive` the tree
  across the whole thing.

### 4.3 New multi-error validation primitive (required for the §3.6 list)

The existing internal `lyError` (libyang.go L253–269) reads only `C.ly_err_first(ctx)` and
returns ONE formatted string. The structured-list path needs a NEW primitive — it will not work
by patching the single-error helper. Add:

```go
type RawDiagnostic struct {
    Message    string
    DataPath   string  // "" == absent
    SchemaPath string  // "" == absent
    AppTag     string  // "" == absent  (item.apptag)
    VecodeStr  string  // "" == absent  (ly_strvecode(item.vecode))
}

func (t *RawDataTree) ValidateCollect(opts uint32) ([]RawDiagnostic, error)
```

`ValidateCollect` must:
1. Acquire a **package-level `sync.Mutex`** around the entire critical section — the libyang
   error log is `ly_ctx`-global and this method must be `-race`-clean against concurrent callers.
2. Save current log options, then set `LY_LOSTORE` (store-all) so libyang retains **every**
   `ly_err_item`, not just the last. (Without store-all you get a single item and fail
   `validate_multi_error_returns_list`.) Pass the multi-error validate flag through `opts`.
3. Clear any prior errors on the context, then run `lyd_validate_all`.
4. On `LY_EVALID`, walk the linked list: `item := C.ly_err_first(ctx)`, then follow `item.next`
   to the end, copying `msg`/`data_path`/`schema_path`/`apptag` (via the existing `cstr_opt`-
   style NUL-safe copy) and `ly_strvecode(item.vecode)` into one `RawDiagnostic` per item.
5. Restore the saved log options; `runtime.KeepAlive(t)` and `runtime.KeepAlive(t.owner)` across
   the whole sequence.

The Go `cambium` layer maps each `RawDiagnostic` → `Diagnostic` using the §3.6 classification
table. (Mirror the field set of the Rust adapter's `RawDiagnostic`: `apptag`, `vecode_str`,
`data_path`, `schema_path`, `message`.)

---

## 5. The generation counter (the Go-only stale-handle net)

- `RawDataTree` gains an unexported `gen uint64`. Every method that takes a **path-mutating or
  re-anchoring** libyang call increments it: existing `Validate`/`ValidateCollect` (they insert
  defaults / re-anchor via `lyd_validate_all` + `lyd_first_sibling` — libyang.go L323–330), the
  existing `UserOrderedList` inserts/moves (via their owner), and `Close`/`finalize`. Future
  `new_path`/`set_value`/`remove_path`/`add_defaults`/`merge`/`diff_apply` must increment it too
  — add the increment hook now even though those mutators are out of scope, so the contract is
  correct when they land.
- **Validate bumps the generation UNCONDITIONALLY** at the end of the call. Do **not** attempt
  change-detection ("did the tree actually change"). This intentionally staleens every
  `NodeRef`/`NodeSet`/`DataSiblings`/`UserOrderedView` minted *before* a `Validate()` call: their
  next read returns `E0007`. This is the correct conservative behavior — a false "still valid"
  on a re-anchored tree is a UAF-shaped bug; a conservative E0007 is not. Rust has no analogue
  here (its borrow checker enforces `&mut` exclusivity), so there is no oracle — follow this rule.
- Public `NodeRef`/`NodeSet`/`DataSiblings`/`UserOrderedView` snapshot `tree.raw.Generation()` at
  construction. Every accessor first calls an unexported `assertValid()` comparing the stored gen
  to the current tree gen; mismatch returns a wrapped `RuleCodeStale` (E0007). The
  `RuleCodeStale` constant already exists (cambium.go L171); construct
  `&Error{Code: RuleCodeStale, Op: …, Err: …}` directly so the code is exact (`codeForOp` does
  not map a stale op string).
- **Red target (Go-only, no Rust oracle):** `go_stale_handle_after_mutation` — parse a tree, take
  a `NodeRef`, mutate the tree (e.g. `UserOrderedListAt(...).MoveBefore(...)`, or a `Validate()`
  that bumps gen, or `Close`), then call `ref.Value()` / `ref.Children()` and assert `E0007`.
  Named in `spec/api.md` Phase 2 acceptance list. Run under `-race`.

---

## 6. cgo gotchas — checklist Kimi MUST satisfy (tick every box)

- [ ] **`runtime.KeepAlive` on every cgo-touching method.** `defer runtime.KeepAlive(t)` and
      `defer runtime.KeepAlive(t.owner)` at entry of every new `RawDataTree`/`RawNodeRef` method
      (the established pattern, libyang.go L274–275). The GC can finalize the C tree
      (`lyd_free_all`) or context (`ly_ctx_destroy`) mid-call → UAF. Regression-test in
      `lifetime_test.go` style: build, drop Go refs, `runtime.GC();runtime.GC()`, then use —
      under `-race`.
- [ ] **Static-inline `lyd_get_value` via `cam_lyd_get_value`, gated.** It is THE way to read a
      leaf/leaf-list canonical value. Gate to `LYS_LEAF`/`LYS_LEAFLIST` (return `ok=false`, never
      garbage, for anything else). For `decimal64`, the canonical form is left-padded fraction,
      right-trimmed, minus preserved — assert Go output equals the Rust oracle string (`12.34`).
- [ ] **`find_xpath` `ly_set` freed exactly once + tree KeepAlive'd.** Walk `cam_set_dnode(set,i)`
      for `i < set.count`, materialize all paths (`lyd_path(node,LYD_PATH_STD,nil,0)`, free each
      `char*`), THEN `ly_set_free(set, nil)`. The node pointers inside are borrowed (owned by the
      tree); never free them. `KeepAlive` the tree across the entire walk.
- [ ] **Multi-error log walk under a mutex.** `ValidateCollect` sets `LY_LOSTORE`, walks the FULL
      `ly_err_item` linked list via `item.next` (not just `ly_err_first`), restores log options,
      and holds a package `sync.Mutex` for the whole ctx-global critical section. `-race`-clean.
- [ ] **Opaque / anydata / NULL-schema nodes.** A node may have `node.schema == nil` (opaque) or
      be `anydata`/`anyxml`. Gate every schema-dependent read on `node.schema != nil` first.
      `ValueStr`/`Value` return `ok=false` for non-term and for schema-less nodes; never deref a
      nil schema. (`anyxml`=`0x20`, `anydata`=`0x60` are already handled in `schema.go` — do not
      regress.)
- [ ] **Interior-NUL.** Never `C.GoString` a value/path that could contain an embedded NUL. For
      text paths/values from XML/JSON, `C.GoString` is fine; keep the existing `checkNul` guard
      on parse input (libyang.go L166–173).
- [ ] **Node-type gating before any typed cast.** Only `lyd_node_term` carries `.value`; only
      `lyd_node_inner` carries `.child`. Gate every cast on `node.schema.nodetype`. Use
      `cam_lyd_child` (NULL for non-inner) and `cam_lyd_get_value` (gated) rather than manual
      unsafe casts. Red target: `Value()` on a container returns `ok=false`, never a panic.
- [ ] **Coarse FFI walk.** `Children`/`Siblings`/`RootNodes`/`Select` each make ONE cgo call that
      returns a Go-owned slice; the Go loop runs on owned memory. Red target:
      `children_one_ffi_walk_ordered` (mirror Rust) — exactly one FFI walk per level.

---

## 7. Cross-language gates (all must hold before each commit)

1. **Conformance stays 6/6 byte-identical.** `go -C go run ./cmd/cambium all` exits 0 over all
   six fixtures (`scrambled-children, keys-first, ordered-user, rpc-order, system-list-canonical,
   ietf-interfaces`). Serialization is unchanged this phase; you must not perturb it.
2. **RuleCode parity.** The new surface emits the codes the spec assigns, identical to Rust:
   - `E0006 DataPath` — `Get` miss, `Select` on bad XPath, any path/XPath read failure.
   - `E0007 Stale` — a `NodeRef`/`NodeSet`/`DataSiblings`/`UserOrderedView` used after a tree
     mutation.
   - `E0005 OrderedList` — `AsUserOrdered`-side positional errors stay E0005 (distinct from
     E0006); do not collapse the two.
   - `E0003 Validate` — every `Diagnostic` carries E0003 at top level; sub-fields don't renumber.
   These are `spec/rule-codes.md` + `spec/api.md` "Error contract". `codeForOp` already maps the
   read/validate ops — reuse it; do not hand-roll codes.
3. **Same-observable-as-Rust per shipped API.** Each Go test asserts the same observable as the
   identically named Rust test. Mirror EVERY one below.

   **Read oracle — `rust/cambium-core/tests/data_read.rs`:**
   - `get_value_typed` (L103) — typed `Value` per leaf; `all-builtins` → `Int64(-7)` etc.
   - `get_value_str_canonical` (L125) — canonical strings; `dec64` → `"12.34"` (NOT `"12.3400"`).
   - `get_try_get_exists` (L147) — `Get` miss → `E0006`; `TryGet` miss → `ok=false`; `Exists`
     soft-false on empty/miss.
   - `select_document_order` (L168) — `Select("/cambium-data-read-demo:top/*")` yields
     `[rw-flag, mandatory-leaf, all-builtins, dec64, status-enum]` in canonical schema order.
   - `children_one_ffi_walk_ordered` (L192) — children in declaration order; exactly ONE FFI walk.
   - `node_schema_bridge` (L216) — `Get(".../rw-flag").Schema()`: `Name()=="rw-flag"`,
     `ResolvedType/Base()==BaseType::Boolean`; `Get(".../all-builtins").Schema()` base `Int64`.
     Reuse the existing `go/cambium/schema.go` `SchemaNodeRef` + `BaseType`.

   **Validation oracle — `rust/cambium-core/tests/data_validation.rs` (build the failing tree by
   PARSE, per §2):**
   - `validate_must_when_fails_with_path` (L91) — a tree failing BOTH `must` AND `when`, validated
     with multi-error on. `ValidationCode`s contain `Must` and `When`. The `Must` diagnostic has
     `ErrorAppTag == "must-violation"`, `ErrorType == Application`, and `DataPath` contains `/c`.
   - `validate_mandatory_missing_app_tag` (L145) — missing mandatory leaf; `Primary()` diagnostic
     has `ValidationCode == Mandatory`, `ErrorAppTag == ""` (absent), message contains
     "mandatory" (case-insensitive).
   - `validate_leafref_unresolved` (L172) — `top/ref="no-such"`; `Primary()` diagnostic
     `ValidationCode == Leafref`, `ErrorAppTag == "instance-required"`, `DataPath` contains `/ref`.
   - `validate_multi_error_returns_list` (L209) — a tree failing **Mandatory AND Leafref** (NOT
     must+when), validated with multi-error on; `Len() >= 2` and the `ValidationCode` set contains
     **both** `Mandatory` and `Leafref`.

   **User-ordered read oracle — `rust/cambium-core/tests/user_ordered_read.rs`:**
   - `user_ordered_read_side` — `AsUserOrdered` on a `user-list` instance: `Len()==3`,
     `Iter()` names all `"user-list"`, each entry has 2 children, `Get(0).Path()` contains
     `[id='a']`, `FindByKey([{"id","b"}])` → `idx=1`, `FindByKey([{"id","z"}])` → `ok=false`.
   - `as_user_ordered_none_for_system_ordered` — `AsUserOrdered` on a `system-list` instance
     returns `ok=false` (NOT an error), and that node's `Schema().OrderedBy()==System`.
4. **Run the Rust leg once to confirm no regression.** After your slices are green on Go, run
   `cargo test --workspace` (or at minimum `cargo test -p cambium-core data_read data_validation
   user_ordered_read`) once and confirm still green. You are not changing Rust; this guards that
   your reading of the oracle is faithful. Report the result.
5. **Hexagonal boundary intact.** `go/cambium`, `go/conformance`, `go/cmd` import zero `C`/
   `unsafe`; all C stays in `internal/libyang`. Verify before committing.

---

## 8. SUCCESS FLOOR (minimum that MUST land) vs deferrable

**SUCCESS FLOOR — this phase is not done until ALL of these are green, committed, and the §7
gates hold:**

- **S1.** `cam_lyd_child` / `cam_lyd_get_value` / `cam_set_dnode` cgo wrappers + node-type gating.
- **S2.** `RawNodeRef` + raw read primitives (`FindNode`, `ValueStr`, `ChildrenOf`/`SiblingsOf`,
      `RootNodes`, `XPathPaths`, `IsDefault`, `SchemaPtr`, `Generation`) + `ValidateCollect`
      multi-error primitive — each KeepAlive'd, coarse, race-clean.
- **S3.** Public `NodeRef` + `DataTree.Get/TryGet/Exists/Select/RootNodes` + `NodeSet` +
      `DataSiblings` + `Value`/`Decimal64`, mirroring Rust names/semantics.
- **S4.** `NodeRef.Value/ValueStr/Schema/Children/Siblings/Path/Name/IsDefault` + read-only
      `AsUserOrdered` + the NEW `UserOrderedView` type (§3.2). Gated by `user_ordered_read_side`
      and `as_user_ordered_none_for_system_ordered`.
- **S5.** Generation-tagged stale check → `E0007`, with `go_stale_handle_after_mutation` passing
      under `-race`.
- **S6.** Mirrored READ acceptance tests passing: `get_value_typed`, `get_value_str_canonical`,
      `get_try_get_exists`, `select_document_order`, `children_one_ffi_walk_ordered`,
      `node_schema_bridge`.
- **S7.** Structured `ValidationErrors`/`Diagnostic`/`ValidationCode`/`ErrorType` from `Validate`,
      recoverable via `errors.As`, with ALL FOUR validation oracle tests passing under `-race`:
      `validate_must_when_fails_with_path` (Must+When, app-tag/error-type/data-path asserts),
      `validate_mandatory_missing_app_tag` (Mandatory, no app-tag),
      `validate_leafref_unresolved` (Leafref, app-tag `instance-required`),
      `validate_multi_error_returns_list` (**Mandatory + Leafref**, `Len()>=2`).

**DEFERRABLE (do NOT do this phase; leave clean TODOs only if it costs nothing):**

- `NodeRef.Ancestors()` / `NodeRef.Traverse(order)` — defer unless S1–S7 are done with time
  to spare.
- Mutation CRUD: `new_path`, `set_value`, `remove_path`, `unlink_path`, `add_defaults` (Phase
  2/3) — out of scope; only wire their generation increment when they exist.
- `diff` / `diff_apply` / `merge` / `duplicate` (Phase 3) — out of scope entirely.
- LYB round-trip read, identity-base closure from the parsed tree — known deferred gaps; leave
  as-is.
- Any codegen change (`go/codegen/`) — forbidden this phase.

---

## 9. Human-review-critical zones (flag these for the post-run adversarial review)

When you finish, your handoff note must point the reviewer at these four load-bearing,
easy-to-get-wrong spots:

1. **The cgo value-read wrapper (`cam_lyd_get_value` + node-type gating).** Verify it is only
   ever dereferenced on `LYS_LEAF`/`LYS_LEAFLIST`, returns `ok=false` (never garbage/panic) for
   containers/lists/opaque/anydata, and that `decimal64` canonical output matches the Rust oracle
   string exactly.
2. **The `NodeRef`/`UserOrderedView` lifetime / generation-tag invalidation-on-mutation.** Verify
   every public accessor calls `assertValid()` first; verify EVERY mutation path (Validate/
   ValidateCollect — unconditional bump, ordered-list insert/move, Close, and the not-yet-added
   CRUD mutators) increments the generation; verify a stale handle yields exactly `E0007` and
   never a UAF/segfault under `-race`.
3. **The XPath `ly_set` ownership.** Verify the set is freed exactly once with a NULL destructor,
   the contained node pointers are never freed, each `lyd_path` `char*` is freed, and the tree is
   `KeepAlive`'d across the entire materialize-then-free sequence.
4. **The multi-error `ValidateCollect` log walk.** Verify `LY_LOSTORE` is set then restored, the
   FULL `ly_err_item` list is walked via `item.next` (not just `ly_err_first`), the ctx-global
   critical section is under a package mutex, and the `apptag`/`vecode_str`/`message` →
   `ValidationCode` mapping matches `tree.rs` `validation_code_from_apptag` (L596–622) exactly.

---

## 10. Final handoff (what to leave in your last message)

Report, concisely: which SUCCESS-FLOOR slices landed (commit SHAs + subjects), the
`go -C go test ./... -race` / `go -C go vet` / conformance `6/6` results, the `cargo test`
no-regression result, anything deferred, and the four human-review-critical zones above with
file:line pointers. Do not push. Do not open a PR.
