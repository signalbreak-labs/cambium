# Cambium SDK API — Historical Design Proposal

> **Current-status warning (2026-06-20):** this is a historical proposal and
> design-rationale record, not the current implementation-status source. For
> the normative contract use [`/spec/api.md`](../spec/api.md); for the current
> finish-line audit use [`docs/handoff-codex-2026-06-20.md`](handoff-codex-2026-06-20.md).
> Sections below that say "Cambium today", "gap", "open decision", or phase
> ordering reflect the proposal checkpoint when written and may be superseded.

> **Status:** proposal v1 (not ratified). Produced 2026-06-13 by a 3-lens design panel
> — goyang-migrant ergonomics · libyang data-power · Rust-idiomatic safety — synthesized
> into one surface, then reviewed. The eventual ratified home is `/spec/api.md` (Phase 0).
>
> Cambium is the **goyang successor SDK**: this is the surface that matches goyang's schema
> introspection and goes much richer with a real data layer, validation, diff/merge, and
> order-correct typed-struct codegen. The SDK does **not** send NETCONF or generate Terraform
> providers — those are downstream consumers (see `AGENTS.md` Non-goals).

## Ratified decisions (2026-06-13) — supersede the open questions below

The four one-way doors were decided; the normative record now lives in [`/spec/api.md`](../spec/api.md):

- **Handle model → BORROWED `NodeRef<'tree>`** (not `{gen,path}`). Open Question 1 is **resolved**.
  Where §0 *D-1* below says "`{ gen, path }` adopted," read it as superseded: Rust uses a borrowed
  lifetime (no stale possible in safe Rust); Go uses a generation-tagged value type that returns
  `CAMBIUM_E0007` on stale use.
- **Codegen serialization → NATIVE serializer + CI byte-equality gate** (not marshal-through-engine).
  Open Question 6 is **resolved**.
- **Validation → runtime `validate(&mut self)`**, no `Validated/Unvalidated` typestate. Open
  Question 2 is **resolved**.
- **Next step → Phase 0** (this change): `/spec/api.md` authored, `E0006`/`E0007` appended to
  `spec/rule-codes.md`, acceptance tests pinned as the red targets.

# Cambium Public SDK API — Unified Design

> Status: **proposal v1** for `docs/sdk-api-design.md`. Layer: design contract feeding `/spec/api.md`.
> Lens synthesis: goyang-recognizability (D1) + libyang data-power (D2) + Rust-idiomatic safety & typed codegen (D3).
> Rust is primary; every type has a 1:1 Go mirror under the four sanctioned shape differences: `Result<T,E>`↔`(T, error)`, `Drop`↔`Close()`, `impl Iterator`↔slice/`iter.Seq`, `Option<T>`↔`(T, ok bool)`, plus consuming-`self`↔value-receiver-taking-the-owner.
> "EXISTS" marks today's surface; "NEW" marks additions. Every NEW capability maps to an obvious failing-test-first unit (noted inline).

## 0. Governing decisions (resolve before any signature is final)

These are baked into the design below; they are restated as forks in Open Questions only where genuinely contested.

- **D-1 (handle model — adopted from the data-power lens).** `NodeRef`/`SchemaNode` are **domain handles, never `*mut lyd_node`**. A data handle is `{ gen: u64, path: PathKey }`; a schema handle is `{ ctx-id, schema-path }`. Iterators (`children()`, `traverse()`, `select()`) materialize an **ordered `Vec` of handles in one coarse FFI walk** (honors "no per-node FFI"). Mutating a `DataTree` bumps its `gen`; a stale handle yields `RuleCode::Stale` instead of UB. This is what makes the Go mirror a clean **value type** with no cgo pointer escape.
- **D-2 (borrow discipline — adopted from D2/D3).** Reads borrow the tree **immutably** and return short-lived handles; **all mutation is path-addressed on `&mut DataTree`** (`new_path`/`set_value`/`remove_path`). No long-lived `&mut NodeRef`. The borrow checker therefore forbids read-while-mutate aliasing; Go documents the same invalidation contract since it has no borrow checker.
- **D-3 (frozen context as typestate — adopted from D3).** A mutable `ContextBuilder` is the only thing that loads modules/sets features; `build()` consumes it into an immutable, `Send + Sync` `Context`. Data trees can only be created from a frozen `Context`, so "no mutation after freeze" is structural.
- **D-4 (codegen-first per kickoff Decision 2).** Typed-struct codegen with a field-order manifest is a first-class deliverable; the YANG→Terraform generator is a downstream consumer in a separate repo. Diff/merge are P1 (high value, but after the core read/CRUD/validate/serialize round-trip).
- **D-5 (ordering scope per kickoff Decision 1).** `ordered-by user` is byte-exact (`UserOrderedList`/`UserOrderedLeafList`); `ordered-by system` is canonical-deterministic only. No fidelity-replay API.

## Crate / package map

| Crate | Role | Public? |
|---|---|---|
| `cambium` | facade re-export + `prelude` + version | yes, `#![deny(missing_docs)]` |
| `cambium-core` | Context, Schema*, Data*, ordered types, errors | yes, `#![deny(missing_docs)]` |
| `cambium-codegen` | typed-struct + path-builder generator | yes |
| `cambium-libyang-sys` | FFI adapter — the **only** place `*mut lyd_node` exists | internal |
| `cambium-compat` (P2) | optional read-only goyang-shaped projection | yes |

Go: `go/cambium` (facade + core), `go/codegen`, `go/compat` (P2), `go/internal/libyang` (cgo, unexported).

---

## A. Context / loading  *(EXISTS thin → typestate redesign)*

```rust
// cambium-core::context
#[derive(Clone, Copy, Debug, Default)]
pub struct ContextFlags {            // NEW — newtype over LY_CTX_*, never a raw u16
    pub no_yang_library: bool,
    pub all_implemented: bool,
    pub ref_implemented: bool,
    pub disable_searchdir_cwd: bool,
}

pub struct ContextBuilder { /* private; the only mutable phase */ }      // NEW
impl ContextBuilder {
    pub fn new(flags: ContextFlags) -> Result<Self>;
    pub fn search_path<P: AsRef<Path>>(&mut self, dir: P) -> Result<&mut Self>;     // EXISTS (set_search_path)
    pub fn unset_search_path<P: AsRef<Path>>(&mut self, dir: P) -> Result<&mut Self>; // NEW
    pub fn load_module(&mut self, name: &str, revision: Option<&str>,
                       features: &[&str]) -> Result<&mut Self>;          // EXTENDS load_module(name)
    pub fn load_module_str(&mut self, text: &str, format: SchemaFormat) -> Result<&mut Self>; // NEW
    pub fn set_features(&mut self, module: &str, features: &[&str]) -> Result<&mut Self>;      // NEW
    pub fn from_yang_library_str(text: &str, fmt: Format, dir: &Path,
                                 flags: ContextFlags) -> Result<Self>;   // NEW (P3)
    pub fn build(self) -> Result<Context>;                              // NEW — freezes
}

pub struct Context { /* private; Send + Sync (frozen ly_ctx) */ }        // EXISTS, reshaped
impl Context {
    pub fn search_paths(&self) -> impl Iterator<Item = &Path>;           // NEW
    pub fn modules(&self, skip_internal: bool) -> impl Iterator<Item = Module<'_>>; // NEW
    pub fn get_module(&self, name: &str, revision: Option<&str>) -> Option<Module<'_>>; // NEW
    pub fn find_module_by_ns(&self, ns: &str) -> Option<Module<'_>>;     // NEW
    pub fn schema(&self, module: &str) -> Result<Module<'_>>;           // REPLACES schema_tree
    pub fn schema_tree(&self, module: &str) -> Result<SchemaTree>;       // EXISTS — kept, deprecated thin view
    pub fn new_data(&self) -> DataTree;                                  // NEW — empty in-mem tree
    pub fn parse(&self, format: Format, parse: ParseMode,
                 validate: ValidateMode, data: &[u8]) -> Result<DataTree>; // EXTENDS parse
    pub fn parse_op(&self, format: Format, op: OpType, data: &[u8]) -> Result<DataTree>; // EXISTS
}
```

**Invariants/house rules.** Frozen-via-typestate (D-3); `ContextFlags`/`ParseMode`/`ValidateMode` are flag structs (composable — today's `ParseMode` enum cannot express `strict + no_state`); zero libyang types (`Module<'_>` is a domain handle). `Context: Send + Sync`; `DataTree: !Sync`.

**Go mirror.** `ContextBuilder` with chainable `SearchPath(dir) error`, `LoadModule(name string, revision *string, features []string) error`, `Build() (*Context, error)`. `Context.Close()` frees. `Modules(skipInternal bool) iter.Seq[Module]`. Keep `NewContext()`/`SchemaTree()` as the deprecated thin path during migration.

**TDD units.** `builder_freeze_then_parse`, `load_module_revision_pin`, `load_module_features_prune`, `modules_enumerate_implemented`, `parse_mode_strict_plus_no_state_compose`.

---

## B. Schema introspection + rich type info  *(EXISTS coarse → biggest goyang-parity redesign)*

Today: `SchemaTree`/`SchemaNode` is a deep-copied `Vec` of owned `String`s carrying only `name/kind/ordered_by_user/is_key/key_names/leaf_type/children`; `LeafType` is `{String,Int,Bool,Unknown}`. We keep the **single-coarse-walk materialization** (satisfies no-per-node-FFI) but expose **borrowed handles** with goyang-grade richness.

### B.1 Module + node

```rust
pub struct Module<'ctx> { /* private handle */ }                          // NEW
impl<'ctx> Module<'ctx> {
    pub fn name(&self) -> &str;
    pub fn namespace(&self) -> &str;
    pub fn prefix(&self) -> &str;
    pub fn revision(&self) -> Option<&str>;
    pub fn is_implemented(&self) -> bool;
    pub fn features(&self) -> impl Iterator<Item = Feature<'ctx>>;
    pub fn feature_value(&self, name: &str) -> Result<bool>;
    pub fn identities(&self) -> impl Iterator<Item = Identity<'ctx>>;
    pub fn top_level(&self) -> SchemaChildren<'ctx>;   // DECLARATION ORDER (I2)
    pub fn find_path(&self, schema_path: &str) -> Result<SchemaNode<'ctx>>;
}

pub struct SchemaNode<'ctx> { /* private handle */ }                      // EXISTS, reshaped to borrowed
impl<'ctx> SchemaNode<'ctx> {
    pub fn name(&self) -> &str;                                           // EXISTS
    pub fn kind(&self) -> SchemaNodeKind;                                 // EXISTS
    pub fn module(&self) -> Module<'ctx>;                                 // NEW
    pub fn description(&self) -> Option<&str>;                            // NEW
    pub fn reference(&self) -> Option<&str>;                             // NEW
    pub fn status(&self) -> Status;                                      // NEW — enum, not bool+Option
    pub fn config(&self) -> Config;                                     // NEW — enum Rw|Ro
    pub fn is_mandatory(&self) -> bool;                                  // NEW
    pub fn is_presence_container(&self) -> bool;                         // NEW
    // ordering-critical predicates (gate codegen field types)
    pub fn ordered_by(&self) -> OrderedBy;            // NEW — enum System|User (replaces ordered_by_user bool)
    pub fn is_list_key(&self) -> bool;               // EXISTS (is_key)
    pub fn is_keyless_list(&self) -> bool;           // NEW
    pub fn list_keys(&self) -> impl Iterator<Item = SchemaNode<'ctx>>; // EXTENDS key_names — KEY-STMT ORDER (I3)
    pub fn min_elements(&self) -> Option<u32>;       // NEW
    pub fn max_elements(&self) -> Option<u32>;       // NEW
    pub fn uniques(&self) -> impl Iterator<Item = Vec<SchemaNode<'ctx>>>; // NEW
    // navigation — ALL DECLARATION ORDER
    pub fn parent(&self) -> Option<SchemaNode<'ctx>>;                    // NEW
    pub fn children(&self) -> SchemaChildren<'ctx>;  // EXTENDS children() slice — ordered iterator, never a map
    pub fn ancestors(&self) -> SchemaAncestors<'ctx>;                    // NEW
    pub fn siblings(&self) -> SchemaChildren<'ctx>;                      // NEW
    pub fn traverse(&self) -> SchemaTraverse<'ctx>;  // NEW — pre-order DFS (replaces SchemaTree::iter)
    pub fn find_path(&self, path: &str) -> Result<SchemaNode<'ctx>>;     // EXTENDS find(&[names])
    // rpc/action/notif (I4)
    pub fn input(&self) -> Option<SchemaChildren<'ctx>>;                 // NEW
    pub fn output(&self) -> Option<SchemaChildren<'ctx>>;               // NEW
    pub fn actions(&self) -> impl Iterator<Item = SchemaNode<'ctx>>;     // NEW
    pub fn notifications(&self) -> impl Iterator<Item = SchemaNode<'ctx>>; // NEW
    // type + constraints + extensions
    pub fn leaf_type(&self) -> Option<TypeInfo<'ctx>>; // REPLACES leaf_type()->LeafType
    pub fn units(&self) -> Option<&str>;                                 // NEW
    pub fn default_value(&self) -> Option<&str>;                        // NEW
    pub fn whens(&self) -> impl Iterator<Item = WhenStmt<'ctx>>;         // NEW (P2)
    pub fn musts(&self) -> impl Iterator<Item = MustStmt<'ctx>>;         // NEW (P2)
    pub fn extensions(&self) -> impl Iterator<Item = ExtInstance<'ctx>>; // NEW (P2)
}

#[non_exhaustive]
pub enum SchemaNodeKind {            // EXTENDS — split rpc/action, input/output
    Container, Leaf, LeafList, List, Choice, Case,
    AnyData, AnyXml, Rpc, Action, Input, Output, Notification, Unknown,
}
#[non_exhaustive] pub enum Status { Current, Deprecated, Obsolete } // NEW
pub enum Config { Rw, Ro }          // NEW
pub enum OrderedBy { System, User } // NEW
```

### B.2 Rich type info — the real `YangType` replacement (sum type)

The constraint set is made illegal-state-proof by carrying only valid constraints per variant (range cannot exist on `String`; patterns cannot exist on `Int`).

```rust
pub struct TypeInfo<'ctx> { /* private handle */ }                        // NEW (replaces LeafType)
impl<'ctx> TypeInfo<'ctx> {
    pub fn base(&self) -> BaseType;                  // the 19 RFC-7950 builtins
    pub fn typedef_name(&self) -> Option<&str>;
    pub fn resolved(&self) -> ResolvedType<'ctx>;
}

#[non_exhaustive]
pub enum BaseType {                  // member-for-member parity with goyang TypeKind
    Int8, Int16, Int32, Int64, Uint8, Uint16, Uint32, Uint64,
    Decimal64, String, Boolean, Empty, Binary, Bits, Enumeration,
    IdentityRef, InstanceIdentifier, LeafRef, Union,
}

#[non_exhaustive]
pub enum ResolvedType<'ctx> {        // illegal field combos unrepresentable
    Int(IntKind, Option<Vec<RangeBound>>),       // IntKind discriminates i8..u64
    Decimal64 { fraction_digits: FractionDigits, range: Option<Vec<RangeBound>> },
    Boolean,
    Empty,
    Binary { length: Option<Vec<RangeBound>> },
    StringType { length: Option<Vec<RangeBound>>, patterns: Vec<Pattern> },
    Enumeration(EnumDef),            // ordered (name,value) pairs, two-way lookup
    Bits(BitsDef),                   // ordered (name,position) pairs
    IdentityRef { bases: Vec<Identity<'ctx>> },
    InstanceIdentifier { require_instance: bool },
    LeafRef { target: Box<SchemaNode<'ctx>>, require_instance: bool },
    Union(Vec<TypeInfo<'ctx>>),      // recursive
}

pub struct RangeBound { pub min: Decimal, pub max: Decimal } // Decimal = smart-ctor newtype
pub struct Pattern { pub regex: String, pub inverted: bool,
                     pub error_message: Option<String>, pub error_app_tag: Option<String> }
pub struct FractionDigits(u8);       // smart ctor 1..=18
pub struct EnumDef { /* ordered (name,value) + two-way lookup */ }
pub struct BitsDef { /* ordered (name,position) */ }

pub struct Identity<'ctx> { /* handle */ }                               // NEW
impl<'ctx> Identity<'ctx> {
    pub fn name(&self) -> &str;
    pub fn module(&self) -> Module<'ctx>;
    pub fn bases(&self) -> impl Iterator<Item = Identity<'ctx>>;
    pub fn derived(&self) -> impl Iterator<Item = Identity<'ctx>>; // transitive closure
}
```

**Invariants/house rules.** `top_level()`/`children()`/`input()`/`output()` walk the compiled sibling chain → declaration order (I2/I4); `list_keys()` is key-statement order (I3); `ordered_by()` gates `UserOrderedList<T>` codegen (I1 mechanism at schema level). `ResolvedType` makes range-on-string unrepresentable. `EnumDef`/`BitsDef` preserve declaration order (matters for codegen). `'ctx` ties handles to the frozen context; zero libyang types leak. The legacy flat `SchemaTree`/`LeafType` stays as a deprecated view so the v1 codegen and M0–M3 consumers keep compiling.

**Go mirror.** `Module`/`SchemaNode`/`TypeInfo` interfaces; iterators → `[]SchemaNode` or `iter.Seq`. `ResolvedType` is a sealed interface + concrete struct per variant + type switch (Go's sum-type idiom). `Status`/`Config`/`OrderedBy`/`BaseType` are typed int consts. `Option<&str>` → `(string, bool)`.

**TDD units.** `children_declaration_order` (scrambled-children fixture), `list_keys_statement_order`, `rpc_io_schema_order`, `leaf_type_decimal64_fraction_digits`, `leaf_type_enum_values_ordered`, `leaf_type_union_members_recursive`, `leafref_target_resolves`, `identity_derived_closure`, `pattern_carries_app_tag`.

---

## C. Data tree CRUD + navigation  *(NEW — where Cambium out-classes goyang)*

Realized through coarse whole-subtree FFI; no live pointer crosses the boundary (D-1). Mutation by path on `&mut DataTree`; reads return short-lived borrowed handles (D-2).

```rust
#[derive(Clone, Copy, Debug, Default)]
pub struct NewPathOpts { pub update: bool, pub output: bool, pub opaque: bool }
#[derive(Clone, Copy, Debug, Default)]
pub struct ImplicitOpts { pub no_state: bool, pub output: bool }

impl DataTree {
    // CREATE
    pub fn new_path(&mut self, path: &str, value: Option<&str>, opts: NewPathOpts)
        -> Result<NodeAddr>;                                            // NEW — lyd_new_path
    // READ
    pub fn get(&self, path: &str) -> Result<NodeRef<'_>>;               // NEW (single)
    pub fn try_get(&self, path: &str) -> Option<NodeRef<'_>>;           // NEW
    pub fn select(&self, xpath: &str) -> Result<NodeSet<'_>>;          // NEW — find_xpath, document order (I5)
    pub fn exists(&self, path: &str) -> bool;                          // NEW
    pub fn root_nodes(&self) -> DataSiblings<'_>;                      // NEW — ordered top-level
    // UPDATE / DELETE by path
    pub fn set_value(&mut self, path: &str, value: &str) -> Result<bool>; // NEW — Ok(true)=changed
    pub fn remove_path(&mut self, path: &str) -> Result<()>;           // NEW
    pub fn unlink_path(&mut self, path: &str) -> Result<DataTree>;     // NEW — detach, re-insertable
    // implicit defaults
    pub fn add_defaults(&mut self, opts: ImplicitOpts) -> Result<()>; // NEW
    // ordered-by-user handles (EXISTS + new leaf-list variant)
    pub fn user_ordered_list_at(&mut self, path: &str) -> Result<UserOrderedList<'_>>;      // EXISTS
    pub fn user_ordered_leaf_list_at(&mut self, path: &str) -> Result<UserOrderedLeafList<'_>>; // NEW
}

pub struct NodeRef<'tree> { /* private {gen,path} handle, never *mut lyd_node */ } // NEW
impl<'tree> NodeRef<'tree> {
    pub fn schema(&self) -> SchemaNode<'_>;          // node→schema bridge
    pub fn path(&self) -> String;                     // canonical data path
    pub fn value(&self) -> Option<Value>;             // typed
    pub fn value_str(&self) -> Option<&str>;          // canonical (AGENTS-mandated lyd_get_value)
    pub fn is_default(&self) -> bool;
    pub fn parent(&self) -> Option<NodeRef<'tree>>;
    pub fn children(&self) -> DataSiblings<'tree>;    // ORDERED sibling-chain walk (one FFI walk)
    pub fn siblings(&self) -> DataSiblings<'tree>;
    pub fn ancestors(&self) -> DataAncestors<'tree>;
    pub fn traverse(&self) -> DataTraverse<'tree>;    // pre-order DFS, ordered
    pub fn list_keys(&self) -> impl Iterator<Item = NodeRef<'tree>>;
    /// Borrow a positional handle iff this is an ordered-by-user list/leaf-list;
    /// None for system-ordered — the only way to reorder.
    pub fn as_user_ordered(self) -> Option<UserOrdered<'tree>>;
}

#[non_exhaustive]
pub enum Value {                     // one variant per base type, never a stringly blob
    Int8(i8), Int16(i16), Int32(i32), Int64(i64),
    Uint8(u8), Uint16(u16), Uint32(u32), Uint64(u64),
    Decimal64(Decimal), Bool(bool), Empty,
    Str(String), Binary(Vec<u8>),
    Enum(String), Bits(Vec<String>),
    Identityref(String), InstanceIdentifier(String),
}

pub struct NodeSet<'tree> { /* domain newtype over ordered Vec<NodeRef> — never ly_set */ }
impl<'tree> NodeSet<'tree> {
    pub fn len(&self) -> usize; pub fn is_empty(&self) -> bool;
    pub fn iter(&self) -> impl Iterator<Item = NodeRef<'tree>>; // document order (I5)
    pub fn get(&self, i: usize) -> Option<NodeRef<'tree>>;
}
```

**Invariants/house rules.** `children()`/`siblings()`/`root_nodes()`/`traverse()` walk `lyd_node.next` → declaration/ordered order, materialized in ONE FFI walk per call (I2; no `.next()`-per-call across the boundary). `select()` returns document order (I5). `new_path` inserts at the schema-correct position so a freshly built tree already serializes in declaration order. `NodeAddr` is an opaque domain path-key. `set_value` returns changed/same for idempotency/diff. `as_user_ordered()` returning `None` for system-ordered is the compile-time gate to reordering. New `RuleCode::DataPath` (E0006) and `RuleCode::Stale` (E0007).

**Go mirror.** Path mutators on `*DataTree` (`NewPath(path string, value *string, opts NewPathOpts) (NodeAddr, error)`, `SetValue(path, value string) (changed bool, err error)`). `NodeRef` is a value type; `Children() []NodeRef` materialized in one walk; `Value` → sealed interface + type switch. No `context.Context` arg (pure CPU — ctx is for I/O, which lives in downstream transport adapters).

**TDD units.** `new_path_builds_intermediates`, `new_path_then_serialize_declaration_order`, `set_value_reports_changed`, `remove_path_round_trip`, `get_value_typed`, `select_document_order`, `children_one_ffi_walk_ordered`, `stale_handle_after_mutation_errors`.

---

## D. Ordered nodes  *(EXISTS — keep positional-only, complete the read side)*

The crown jewel stays positional-only (no order-agnostic setter); gains a read side and a leaf-list sibling.

```rust
pub struct UserOrdered<'tree> { /* private; PhantomData<&'tree mut DataTree> */ }
// (UserOrderedList is the public alias kept for back-compat; UserOrdered is the unified name)
impl<'tree> UserOrdered<'tree> {
    // EXISTING positional-only mutators — KEEP unchanged
    pub fn insert_first(&mut self, entry: DataTree) -> Result<()>;       // EXISTS
    pub fn insert_last(&mut self, entry: DataTree) -> Result<()>;        // EXISTS
    pub fn insert_before(&mut self, index: usize, entry: DataTree) -> Result<()>; // EXISTS
    pub fn insert_after(&mut self, index: usize, entry: DataTree) -> Result<()>;  // EXISTS
    pub fn move_before(&mut self, what: usize, point: usize) -> Result<()>; // EXISTS
    pub fn move_after(&mut self, what: usize, point: usize) -> Result<()>;  // EXISTS
    // NEW read side (today the list is write-only) — still NO set/push/index-assign
    pub fn len(&self) -> usize;                                          // NEW
    pub fn is_empty(&self) -> bool;                                      // NEW
    pub fn get(&self, index: usize) -> Option<NodeRef<'_>>;              // NEW
    pub fn iter(&self) -> impl Iterator<Item = NodeRef<'_>> + '_;        // NEW — insertion order
    pub fn find_by_key(&self, keys: &[(&str, &str)]) -> Option<usize>;   // NEW
    pub fn remove(&mut self, index: usize) -> Result<()>;               // NEW
}

pub struct UserOrderedLeafList<'tree> { /* same positional-only discipline */ } // NEW
impl<'tree> UserOrderedLeafList<'tree> {
    pub fn insert_first(&mut self, value: &str) -> Result<()>;
    pub fn insert_last(&mut self, value: &str) -> Result<()>;
    pub fn insert_before(&mut self, index: usize, value: &str) -> Result<()>;
    pub fn insert_after(&mut self, index: usize, value: &str) -> Result<()>;
    pub fn move_before(&mut self, what: usize, point: usize) -> Result<()>;
    pub fn move_after(&mut self, what: usize, point: usize) -> Result<()>;
    pub fn len(&self) -> usize; pub fn is_empty(&self) -> bool;
    pub fn get(&self, index: usize) -> Option<String>;
    pub fn iter(&self) -> impl Iterator<Item = String> + '_;
}
```

**Invariant I1.** These methods exist ONLY on `UserOrdered`/`UserOrderedLeafList`, reachable only via `as_user_ordered()` (None for system-ordered) and `user_ordered_*_at` (errors `RuleCode::OrderedList` for a non-user-ordered target). Codegen types a user-ordered field as `UserOrderedVec<T>`, so `move_after` on an ordinary `Vec` field is a **compile error**. A `trybuild` compile-fail test (already in the M2 plan) guards this.

**Decision: keep `usize` indices, not an `EntryIndex` newtype.** The list-scoped newtype is elegant in Rust but has no clean Go mirror and adds friction without removing the only real failure (out-of-range), already covered by `Result` + `RuleCode::OrderedList`. (Flagged in Open Questions.)

**Go mirror.** Identical restricted method set (no `Set`): `Len()`, `Get(i int) (NodeRef, bool)`, `Iter() iter.Seq[NodeRef]`, `FindByKey([]KV) (int, bool)`, `Remove(i int) error`; `UserOrderedLeafList` mirrors with `string` values.

**TDD units.** `user_ordered_round_trip` (ordered-user-list fixture), `nested_ordered_user`, `keyless_list_positional`, `user_ordered_read_side`, `compile_fail_no_set_on_vec_field`.

---

## E. Validation  *(EXISTS thin → add modes + structured errors)*

```rust
#[derive(Clone, Copy, Debug, Default)]
pub struct ValidateMode {            // EXTENDS {no_state, multi_error}
    pub no_state: bool,
    pub present: bool,               // NEW — LYD_VALIDATE_PRESENT (:running case)
    pub multi_error: bool,
}

impl DataTree {
    pub fn validate(&mut self, mode: ValidateMode) -> Result<()>;        // EXISTS — now structured on failure
    pub fn revalidate_subtree(&mut self, path: &str, mode: ValidateMode) -> Result<()>; // NEW (P2)
}
```

**House rules.** On failure the returned `Error::Validation(RuleCode::Validate, ValidationErrors)` carries a **list** of typed `Diagnostic`s (data path, schema path, `LY_VECODE`-derived `ValidationCode`, RFC-7950 `error-app-tag`) — replacing today's single flattened string even under `multi_error`. `RuleCode::Validate` (E0003) stays the stable key; `ValidationCode` is a sub-field (append-only registry rule). `add_defaults` (§C) materializes implicit nodes so a subsequent `WithDefaults::All` serialize is order-correct.

**Go mirror.** `Validate(mode ValidateMode) error`; recover detail via `errors.As(err, &ve)` where `ve *ValidationErrors`.

**TDD units.** `validate_must_when_fails_with_path`, `validate_mandatory_missing_app_tag`, `validate_leafref_unresolved`, `validate_multi_error_returns_list`, `validate_present_running_datastore`.

---

## F. Serialization  *(EXISTS thin → full print modes, one sibling walk)*

```rust
#[non_exhaustive]
pub enum Format { Xml, Json, JsonIetf, Lyb }   // EXTENDS — adds Lyb (P3)

#[non_exhaustive]
pub enum WithDefaults { Explicit, Trim, All, AllTagged } // NEW — enum, not 4 bools

#[derive(Clone, Copy, Debug)]
pub struct SerializeFlags {           // EXTENDS {siblings}
    pub siblings: bool,
    pub shrink: bool,                 // NEW
    pub keep_empty_containers: bool,  // NEW
    pub with_defaults: WithDefaults,  // NEW
}
impl Default for SerializeFlags { /* siblings:true, shrink:false, keep_empty:false, WD:Explicit */ }

#[derive(Clone, Copy, Debug, Default)]
pub struct ParseMode {                // REPLACES the mutually-exclusive enum
    pub strict: bool, pub opaque: bool, pub parse_only: bool,
    pub no_state: bool, pub lyb_mod_update: bool,
}

impl DataTree {
    pub fn serialize(&self, format: Format, flags: SerializeFlags) -> Result<Vec<u8>>; // EXTENDS
}
```

**Invariants.** This is the load-bearing invariant carrier: ALL output is produced by **exactly one in-order walk of libyang's sibling chain** (spec §0; I2/I5) — `with_defaults`/`shrink` are libyang print flags, never a re-sort. The default is the conformance golden profile (SHRINK off, 2-space, LF, WITH_SIBLINGS) so golden bytes are the path of least resistance. JSON lists/leaf-lists → arrays carrying order (I5); JSON object member order is the single printer's order, byte-identical Rust↔Go (I5, §4). `JsonIetf` keeps empty containers (already in the adapter) for gNMI; user-ordered rides atomically (I6).

**Go mirror.** `Serialize(format Format, flags SerializeFlags) ([]byte, error)`; `WithDefaults` typed int const; `Format` gains `FormatLYB`. Byte-for-byte identical to Rust is a CI gate.

**TDD units.** `serialize_with_defaults_all` (after add_defaults), `serialize_keep_empty_container`, `json_object_determinism` (Rust==Go), `serialize_lyb_round_trip`.

---

## G. Diff / merge / dup  *(NEW — P1; the downstream plan/apply engine)*

```rust
#[derive(Clone, Copy, Debug, Default)] pub struct DiffOpts  { pub defaults: bool }
#[derive(Clone, Copy, Debug, Default)] pub struct MergeOpts { pub destruct: bool }
#[non_exhaustive] pub enum DiffOp { Create, Delete, Replace, None }

pub struct DataDiff { /* owned yang-patch-shaped diff, RAII Drop */ }
impl DataDiff {
    pub fn is_empty(&self) -> bool;
    pub fn edits(&self) -> impl Iterator<Item = DiffEdit<'_>>; // apply-safe ordered (I2/I6)
    pub fn serialize(&self, format: Format) -> Result<Vec<u8>>; // yang-patch shaped
}
pub struct DiffEdit<'d> { /* private */ }
impl<'d> DiffEdit<'d> {
    pub fn op(&self) -> DiffOp;
    pub fn path(&self) -> &str;
    pub fn value(&self) -> Option<&str>;
    /// True for an edit on an ordered-by-user list/leaf-list: the consumer MUST
    /// carry it atomically (yang:insert metadata), never as N scalar updates (I6).
    pub fn is_ordered_by_user(&self) -> bool;
}

impl DataTree {
    pub fn diff(&self, other: &DataTree, opts: DiffOpts) -> Result<DataDiff>;
    pub fn diff_apply(&mut self, diff: &DataDiff) -> Result<()>;      // round-trips with diff
    pub fn merge(&mut self, source: &DataTree, opts: MergeOpts) -> Result<()>;
    pub fn duplicate(&self) -> Result<DataTree>;
}
```

**Invariants.** `DataDiff` MUST flag `is_ordered_by_user` edits and preserve libyang's `yang:insert` metadata so a NETCONF/gNMI consumer carries them atomically (I6). `merge` preserves user-ordered order; conflicting leaf values → `Error` (ygot semantics). `edits()` yields apply-safe document order.

**Go mirror.** `Diff(other *DataTree, opts DiffOpts) (*DataDiff, error)`, `DiffApply(*DataDiff) error`, `Merge(source *DataTree, opts MergeOpts) error`, `Duplicate() (*DataTree, error)`. `DiffEdit` getters; `DiffOp` typed int const.

**TDD units.** `diff_create_delete_replace`, `diff_apply_round_trip`, `diff_ordered_by_user_atomic` (gnmi-ordered-atomic fixture), `merge_conflict_errors`, `merge_preserves_user_order`.

---

## H. Typed-struct codegen  *(EXISTS XML-only → first-class per Decision 2)*

```rust
// cambium-codegen
pub enum Lang { Rust, Go }
pub struct CodegenOpts {
    pub lang: Lang,
    pub dedup_groupings: bool,       // addresses ygot bloat
    pub with_validate: bool,
    pub with_path_builder: bool,     // ygnmi-style fluent paths (P3)
}
pub struct GeneratedModule {
    pub source: String,
    pub field_order: BTreeMap<String, Vec<String>>, // per-struct, keys-first
}
pub fn generate(ctx: &Context, module: &str, opts: CodegenOpts) -> Result<GeneratedModule>; // EXTENDS generate_rust

/// Every generated struct routes marshal/validate/diff THROUGH a libyang tree,
/// so emitted order matches the engine byte-for-byte (no reflection/map order).
pub trait CambiumStruct: Sized {
    fn to_data_tree(&self, ctx: &Context) -> Result<DataTree>;
    fn from_data_tree(tree: &DataTree) -> Result<Self>;
    fn to_json_ietf(&self, ctx: &Context) -> Result<String>;
    fn from_json_ietf(ctx: &Context, s: &str) -> Result<Self>;
    fn validate(&self, ctx: &Context) -> Result<()>;
    fn diff(&self, ctx: &Context, other: &Self) -> Result<DataDiff>;
}
```

**Type mapping (illegal-states-unrepresentable in generated code too):**
- YANG `enumeration` → generated `enum`; `identityref` → generated `enum` over the derived-identity closure; `union` → generated `enum` of member variants.
- `ordered-by user` list/leaf-list field → **`UserOrderedVec<T>`** (positional-only owned collection mirroring §D — no `Vec`-style index assignment); `ordered-by system` list → ordered `Vec<T>` with keys-first manifest.
- ranges → range-bounded newtypes with smart constructors; `mandatory` leaf → `T`, optional → `Option<T>`.

**Invariants.** The per-struct field-order manifest (keys-first, declaration order) + marshal-through-engine guarantees I2/I3/I5 cannot be violated by generated code; CI asserts generated bytes == libyang `LYD_JSON`/`LYD_XML` for the same data (spec §4.3). `UserOrderedVec<T>` extends positional-only into generated types (I1, compile-time in user code). `dedup_groupings` fixes ygot bloat.

**Go mirror.** `Generate(ctx *Context, module string, opts CodegenOpts) (*GeneratedModule, error)`; generated structs implement a `CambiumStruct` interface; user-ordered fields are a `UserOrderedSlice[T]` type (no exported `Set`); optional generated `<module>path` package.

**TDD units.** `codegen_field_order_manifest_keys_first`, `codegen_user_ordered_field_is_positional`, `codegen_enum_and_union_typed`, `codegen_round_trip_bytes_equal_libyang`, `codegen_dedup_groupings`.

---

## I. Errors  *(EXISTS thin → thiserror chain + structured diagnostics, codes stay stable)*

```rust
#[derive(thiserror::Error, Debug)]
#[non_exhaustive]
pub enum Error {
    #[error("[{code}] {message}")]
    Engine { code: RuleCode, message: String,
             #[source] source: Option<Box<dyn std::error::Error + Send + Sync>> },
    #[error("[{}] validation failed", RuleCode::Validate)]
    Validation(RuleCode, ValidationErrors),
    #[error("[{}] invalid path: {path}", RuleCode::DataPath)]
    InvalidPath { path: PathBuf },
    #[error("[{}] stale data handle", RuleCode::Stale)]
    Stale,
    #[error("[{}] interior NUL byte", RuleCode::Parse)]
    Nul(#[from] std::ffi::NulError),
    #[error("[{}] {0}", RuleCode::Parse)]
    Utf8(#[from] std::string::FromUtf8Error),
}
impl Error { pub fn rule_code(&self) -> RuleCode; }

pub struct ValidationErrors { /* Vec<Diagnostic> */ }
impl ValidationErrors {
    pub fn iter(&self) -> impl Iterator<Item = &Diagnostic>;
    pub fn primary(&self) -> &Diagnostic;
}
pub struct Diagnostic {
    pub code: RuleCode,                          // stable top-level (E0003)
    pub message: String,
    pub data_path: Option<String>,
    pub schema_path: Option<String>,
    pub error_type: ErrorType,                   // enum Application|Protocol
    pub error_app_tag: Option<String>,           // RFC 8040 §7.1
    pub validation_code: Option<ValidationCode>, // LY_VECODE-derived sub-code
}
#[non_exhaustive] pub enum ErrorType { Application, Protocol }
#[non_exhaustive] pub enum ValidationCode { Must, When, Mandatory, Leafref, InvalidValue /* ... */ }

#[non_exhaustive]
pub enum RuleCode {                  // EXTEND append-only: E0006, E0007
    Unknown, Context, Parse, Validate, Serialize, OrderedList, // EXISTS E0000..E0005
    DataPath /* E0006 */, Stale /* E0007 */,                   // NEW
}
```

**House rules.** Derive `thiserror` with `#[from]`/`#[source]` to preserve the chain (today's hand-rolled `Error` discards it). `rule_code()` stays the stable cross-language contract. `Diagnostic` adds machine-readable location + app-tag without renumbering (append-only registry; the rule-codes.md "Future" item). The adapter maps libyang `ly_err_item` → `Diagnostic` at the boundary; no raw libyang error bubbles up.

**Go mirror.** Keep `*cambium.Error{Code, Op, Err}` with `RuleCode()`/`Unwrap()` (errors.Is/As works). `ValidationErrors` implements `error`, recovered via `errors.As(err, &ve)`; `Diagnostic` fields exported; `ValidationCode`/`ErrorType` typed consts. Add `RuleCodeDataPath="CAMBIUM_E0006"`, `RuleCodeStale="CAMBIUM_E0007"`. The Rust↔Go same-code conformance check extends to the two new codes (and is a no-op for the sub-fields, which are informational).

**TDD units.** `error_source_chain_preserved`, `validation_errors_iterable`, `rule_code_stable_cross_lang_e0006_e0007`, `diagnostic_carries_data_path_and_app_tag`.

---

## J. goyang-compat projection  *(NEW — P2, optional ramp)*

A read-only `cambium-compat` crate exposing a goyang-recognizable `Entry`-like view whose `dir()` is an **ordered map** (insertion order, **never** alphabetized — the bug it exists to avoid). It projects from the `SchemaNode<'ctx>` IR. A one-import on-ramp for goyang migrants, explicitly a ramp, not a destination; sequenced last.

```rust
// cambium-compat (P2)
pub struct Entry<'ctx> { /* projects SchemaNode<'ctx> */ }
impl<'ctx> Entry<'ctx> {
    pub fn name(&self) -> &str;
    pub fn dir(&self) -> impl Iterator<Item = (&str, Entry<'ctx>)>; // INSERTION ORDER
    pub fn is_leaf(&self) -> bool; pub fn is_list(&self) -> bool; /* ...goyang Is* set */
    pub fn yang_type(&self) -> Option<TypeInfo<'ctx>>;
}
```

**TDD unit.** `compat_dir_insertion_order_not_alphabetical`.

---

## Cross-cutting honor matrices

| Invariant | Mechanism |
|---|---|
| **I1** user-ordered exact | `UserOrdered`/`UserOrderedLeafList`/`UserOrderedVec<T>` positional-only; no order-agnostic setter; `trybuild` compile-fail test |
| **I2** declaration order + canonical | every `children()`/`top_level()`/`root_nodes()` materializes libyang's sibling chain in one FFI walk; never a map; system-ordered inherits libyang canonicalization |
| **I3** keys first | `list_keys()` in key-statement order; codegen `field_order` manifest puts keys first |
| **I4** RPC/action I/O | `input()`/`output()` walk the compiled subtree in declaration order |
| **I5** JSON arrays carry order | single libyang printer; `select()`/`NodeSet` document order; Rust==Go byte gate |
| **I6** gNMI atomic | `DiffEdit::is_ordered_by_user()`; `JsonIetf` keeps empty containers; user-ordered serialized as one subtree |

| House rule | Mechanism |
|---|---|
| Hexagonal / zero libyang types | `Module`/`SchemaNode`/`TypeInfo`/`NodeRef`/`NodeSet`/`Value`/`DataDiff` are domain handles/newtypes; adapter maps `lyd_node`/`lysc_node`/`ly_err_item`/`ly_set` at the boundary; coarse FFI only |
| Rust style | `Result`+`thiserror` `#[source]`; no `unwrap`/`expect`; enums over bool+Option (`BaseType`/`Status`/`Config`/`OrderedBy`/`WithDefaults`/`Value`/`ResolvedType`); flag structs (`ContextFlags`/`ParseMode`/`ValidateMode`); `#![deny(missing_docs)]` |
| ordered-by-user positional-only | §D — defining type, unchanged mutators |
| frozen ctx / non-concurrent trees | `Context: Send+Sync` via typestate; `DataTree: !Sync`, mutation single-threaded by `&mut` |
| Go mirror (sanctioned diffs only) | `Result↔(T,error)`, `Drop↔Close()`, iterator↔slice, `Option↔(T,ok)`, consuming-self↔owner-taking method |

## Prelude
`cambium::prelude` re-exports: `ContextBuilder`, `Context`, `ContextFlags`, `Module`, `SchemaNode`, `SchemaNodeKind`, `TypeInfo`, `ResolvedType`, `BaseType`, `Status`, `Config`, `OrderedBy`, `Identity`, `DataTree`, `NodeRef`, `NodeSet`, `Value`, `UserOrdered`, `UserOrderedLeafList`, `Format`, `SerializeFlags`, `WithDefaults`, `ParseMode`, `ValidateMode`, `OpType`, `DataDiff`, `DiffEdit`, `DiffOp`, `Error`, `Result`, `RuleCode`, `Diagnostic`, `ValidationErrors`.

---

## Implementation roadmap

### Phase 0 — /spec/api.md + handle contract (pre-req, ~1 wk)

The kickoff explicitly flags /spec/api.md as referenced-but-absent and names the iterator-vs-slice lifetime asymmetry as a /spec gap that must close BEFORE Go work. The handle model gates every navigation/CRUD signature, so it is decided first, with red fixtures committed first per the TDD house rule.

- Author the missing /spec/api.md the kickoff references but lacks: language-neutral contract for every type, the four sanctioned Rust<->Go shape differences, and the consuming-self<->(owner,error) shape.
- Ratify D-1 (the {gen,path} NodeRef snapshot-handle model resolving the kickoff 'lifetime asymmetry' note) and D-2 (reads borrow &, mutation path-addressed on &mut) with worked examples and the stale-handle contract.
- Append E0006 (DataPath) and E0007 (Stale) to spec/rule-codes.md (append-only); add Rust==Go same-code conformance assertions; note ValidationCode/error-app-tag are informational sub-fields, not new top-level codes.
- Commit failing conformance fixtures for the new surface scaffolding (new_path round-trip, select document order) FIRST (TDD red).

### Phase 1 — Rich schema introspection + TypeInfo (Rust)

The single biggest goyang-parity gap (type info collapses to String/Int/Bool) AND the largest net-new FFI surface, so it is its own milestone. Pure read-side over the frozen context = least lifetime/borrow risk. It unblocks real codegen (enums, bitflags, identityref, leafref) and the data->schema bridge.

- Extend the adapter to extract lysc_node flags (config/status/mandatory/presence/min-max), list-key order, when/must/extensions AND lysc_type substructs (range/length/patterns/fraction_digits/enums/bits/identity-bases/leafref-target/union-members) in ONE coarse walk; map to domain types at the boundary.
- Introduce ContextBuilder->Context typestate (build-once-frozen as a type); ContextFlags + load_module(revision,features) + module enumeration.
- Replace flat SchemaTree/SchemaNode with borrowed Module<'ctx>/SchemaNode<'ctx>; add Status/Config/OrderedBy enums, declaration-order children()/traverse()/find_path, RPC input()/output().
- Replace LeafType{String,Int,Bool,Unknown} with TypeInfo + ResolvedType sum type + Identity derived-closure.
- Keep the legacy flat SchemaTree/LeafType as a deprecated thin view so v1 codegen + M0-M3 consumers keep compiling.
- Golden: pyang -f tree as the declaration-order oracle; assert children() == declaration order on the augmented-order fixture.

### Phase 2 — Data tree CRUD + navigation + validation (Rust)

Where Cambium massively out-classes goyang and the highest-value net-new layer. Depends on Phase 0 (handle model) and Phase 1 (rich schema for the data->schema bridge). Validation-with-structured-errors is the killer feature vs ygot (no must/when/mandatory).

- new_path/get/try_get/select/exists/set_value/remove_path/unlink_path/add_defaults on DataTree; NodeRef snapshot handle with value/value_str/schema/parent/children/traverse; Value typed enum; NodeSet newtype (document order).
- DataSiblings/DataTraverse iterators materialized in one FFI walk; stale-handle -> RuleCode::Stale.
- ValidateMode gains present; validation failure returns structured ValidationErrors (map ly_err_item -> Diagnostic with data_path/schema_path/app_tag/ValidationCode).
- Reshape Error to thiserror with #[source]; wire E0006/E0007.
- UserOrdered read side (len/get/iter/find_by_key/remove) + UserOrderedLeafList; trybuild compile-fail asserting move_after on a Vec field does not compile.

### Phase 3 — Serialization completeness + diff/merge/dup (Rust)

Completes the correct NETCONF/gNMI serialization story and ships diff/merge — the engine the downstream TF-provider plan/apply needs. P1 (after the core round-trip per Decision 2's codegen-first ordering) but all additive over Phase 2.

- SerializeFlags{shrink,keep_empty_containers,with_defaults:WithDefaults}; ParseMode as a composable flags struct; add Format::Lyb (verify the vendored build compiles it).
- diff/diff_apply/merge/duplicate on DataTree; DataDiff with DiffEdit.is_ordered_by_user() flag (I6 carrier); yang-patch-shaped DataDiff::serialize.
- Conformance: diff round-trips with diff_apply; user-ordered edits carried atomically (gnmi-ordered-atomic fixture); WithDefaults::All after add_defaults is order-correct; json-object-determinism Rust==Go.

### Phase 4 — Go parity (Block 2)

Kickoff Decision 2: Rust-first, Go a first-class parity target because the Terraform-provider consumer imports the Go SDK. The {gen,path} handle model is what makes the Go mirror clean (no cgo pointer escape) and keeps the cross-language determinism gate enforceable.

- Mirror Phases 1-3 in package cambium against the SAME /conformance golden; assert Rust bytes == Go bytes for every fixture+format.
- NodeRef as a Go value type {gen,path}; iterators->slices/iter.Seq; consuming-self->(owner,error); ResolvedType/Value as sealed interface + type switch; ValidationErrors via errors.As.
- ContextBuilder->Context; add E0006/E0007 string consts and the cross-language same-code conformance check.

### Phase 5 — Codegen v2 + path builder + compat shim

Codegen is the first-class Cambium deliverable (the ygot role) per Decision 2 and consumes every prior layer (rich types from Phase 1, data/diff from Phases 2-3). Routing through libyang guarantees the field-order manifest can never disagree with the engine, even under augments. UserOrderedVec<T> makes reorder-is-a-compile-error a developer-facing guarantee. The compat shim ships last as a migration aid.

- Unified generate(ctx,module,CodegenOpts)->GeneratedModule (Rust+Go) with per-struct field-order manifest (keys-first); typed enums for enumeration/identityref/union, bitflags for bits, UserOrderedVec<T> for user-ordered fields, range-bounded newtypes; mandatory->T / optional->Option<T>.
- CambiumStruct trait/interface (to/from_data_tree, to/from_json_ietf, validate, diff) routing through the libyang tree; CI asserts generated bytes == libyang LYD_JSON/LYD_XML; JSON_IETF + XML emit (not XML-only); deserialize codegen; dedup_groupings.
- Optional ygnmi-style fluent path builder (<module>path) resolving to DataPath+key map.
- Optional cambium-compat crate: read-only goyang-shaped Entry projection whose dir() is INSERTION order (never alphabetical) + migration guide.


---

## Open decisions (forks for the owner — lock in `/spec/api.md` before Phase 2)

1. Handle model — borrowed lifetime (NodeRef<'tree> over a materialized snapshot, Rust-idiomatic, must invalidate on every mutation) vs generation-keyed value handle ({gen, path}, trivially Go-mirrorable, costs an O(n) path re-resolve per call). The design adopts {gen,path} for cross-language cleanliness, but if Rust ergonomics or hot-path traversal cost dominate, a borrowed model + a documented Go invalidation contract is the alternative. Must be locked in /spec/api.md before Phase 2.

2. DataTree<Validated>/<Unvalidated> typestate (from the Rust-safety lens) — compile-time 'you can only diff/serialize-with-defaults a validated tree' is elegant in Rust but has NO clean Go equivalent (Go would need a distinct *ValidatedTree type, risking cross-language method-gating drift). Decide: adopt the typestate (and accept a documented Go asymmetry), or keep plain validate(&mut self) + runtime guarantees (the design's current default). This is the single biggest parity-vs-safety fork.

3. EntryIndex list-scoped newtype vs plain usize on UserOrdered — the newtype prevents cross-list index confusion in Rust but cannot be mirrored ergonomically in Go (plain int). The design keeps usize for symmetry; confirm that's acceptable or accept a sanctioned Rust-only stronger guarantee.

4. Vendored-libyang capability verification — Format::Lyb, from_yang_library_str (ietf-yang-library), and the no_yang_library ContextFlag all require specific libyang CMake features to be compiled into the static build pinned in /VERSIONS. Decide whether to enable+test these in the vendored build before exposing them, or gate them behind a cargo/build feature so the API never promises a capability the engine lacks.

5. Does the Module<'ctx> / borrowed-handle redesign justify a hard break of the existing flat SchemaTree/SchemaNode/LeafType now (pre-1.0, clean), or must the legacy view be maintained in parallel beyond Phase 1? The design keeps it deprecated-but-present; confirm the deprecation horizon.

6. Generated-struct serialization path — marshal-through-libyang (guarantees byte-identical-to-engine order even under augments, but forces a live Context on every to_json_ietf and adds runtime cost) vs a pure manifest-walking native serializer asserted byte-identical in CI (faster, no Context dependency, but reintroduces the risk that generated order disagrees under augments). This is a one-way-door ADR; the design defaults to marshal-through-engine.

7. Should diff/merge be promoted from P1 to P0? Decision 2 sequences codegen first and frames the TF-provider as downstream, but diff is the project's stated reason the data layer exists. Confirm diff stays after the core round-trip, or pull it forward if a consumer needs plan/apply sooner.


---

## Capability gap matrix

| Capability | goyang | Cambium today | Others | Priority |
|---|:--:|:--:|---|:--:|
| Build-once-frozen context (ContextBuilder -> Context typestate) | ✅ | ✅ | libyang ly_ctx; yang3 Context | P0 |
| Context flags (LY_CTX_* as typed struct) | — | — | libyang ly_ctx_new; yang3 ContextFlags | P1 |
| Search-path set/unset/read-back | ✅ | ✅ | libyang ly_ctx_set/unset_searchdir; yang3 | P2 |
| load_module with revision pin + feature set | — | — | libyang ly_ctx_load_module; yang3 | P1 |
| load_module_str (from in-memory text) | ✅ | — | yang3; pyang | P2 |
| Context from ietf-yang-library | — | — | libyang ly_ctx_new_ylmem; yang3 | P3 |
| Module enumeration + lookup (by name/rev/ns) | ✅ | — | libyang ly_ctx_get_module*; yang3 | P1 |
| Module metadata (name/ns/prefix/revision/implemented) | ✅ | — | libyang lys_module; yang3 SchemaModule; goyang Module | P0 |
| Feature query/enable | — | — | libyang lys_feature_value; yang3 | P2 |
| SchemaNode rich metadata (config/status/mandatory/presence/desc) | ✅ | — | libyang lysc_node flags; yang3; goyang Entry | P0 |
| SchemaNode list/order semantics (min/max/unique/keys/user-ordered) | ✅ | ✅ | libyang; yang3; goyang ListAttr | P0 |
| Schema declaration-order navigation (children/ancestors/siblings/traverse/parent) | ✅ | ✅ | libyang lysc_node_child; yang3; goyang Find (but order lost) | P0 |
| RPC/action/notification I/O in schema order | ✅ | — | libyang; yang3; goyang RPCEntry (order map-dependent) | P0 |
| Rich resolved type info (ResolvedType sum type) | ✅ | — | libyang lysc_type*; yang3; goyang YangType | P0 |
| Identity derived-closure resolution | ✅ | — | libyang lysc_ident.derived; yang3; goyang Identity (no closure) | P1 |
| when/must constraint + error-app-tag access | ✅ | — | libyang lysc_when/must; yang3; goyang GetWhenXPath (one when, string-only) | P2 |
| Extension/metadata instance access | ✅ | — | libyang lysc_ext_instance; yang3; goyang Exts (uncompiled) | P2 |
| Data-tree create by path (new_path) | — | — | libyang lyd_new_path; yang3; YDK | P0 |
| Read by path / xpath (find_path/find_xpath/exists) | — | — | libyang lyd_find_path/find_xpath; yang3 | P0 |
| Update leaf value (change_value/set_value) | — | — | libyang lyd_change_term; yang3 | P0 |
| Delete / unlink / remove by path | — | — | libyang lyd_unlink_tree/free_tree; yang3 | P0 |
| Data-tree navigation + typed value read (value/value_str/schema/children) | — | — | libyang lyd_get_value/lyd_child; yang3 | P0 |
| Typed Value enum (one variant per base type) | — | — | libyang; yang3 value | P1 |
| add_implicit defaults | — | — | libyang lyd_new_implicit_all; yang3 | P1 |
| Full-tree validate with modes + structured errors | — | ✅ | libyang lyd_validate_all; yang3 | P0 |
| Subtree/partial validate | — | — | libyang lyd_validate_subtree | P2 |
| Serialize with full print modes (with-defaults/shrink/keep-empty) | — | ✅ | libyang lyd_print_mem; yang3 | P0 |
| LYB binary format | — | — | libyang LYD_LYB; yang3 | P3 |
| Composable parse modes (flags struct) | — | ✅ | libyang lyd_parse_data_mem | P1 |
| Positional-only ordered-by-user list (the defining type) | — | ✅ | libyang lyd_insert_before/after; ygot (late/opt-in) | P0 |
| Diff producing ordered, user-order-aware edit set | — | — | libyang lyd_diff_tree; yang3; ygot.Diff | P1 |
| diff_apply + merge + duplicate | — | — | libyang lyd_diff_apply/merge_tree/dup; yang3; ygot.MergeStructs | P1 |
| Structured Diagnostic / ValidationErrors set | — | — | libyang ly_err_item | P0 |
| thiserror Error with #[source] chain | — | — | house Rust rules | P0 |
| Order-correct typed-struct codegen (field-order manifest + engine round-trip) | — | ✅ | ygot generator | P0 |
| Generated-struct ergonomics trait (validate/emit/parse/diff through engine) | — | — | ygot GoStruct/ValidatedGoStruct | P1 |
| Generated fluent path-builder (ygnmi-parity) | — | — | ygot ypathgen/ygnmi | P3 |
| goyang-shaped compat projection (ordered dir()) | ✅ | — | n/a (Cambium-unique migration aid) | P2 |

---

## Reviewer's critique & goyang-parity notes

> The automated adversarial-critique pass hit a session limit and didn't run; this section is the review in its place, cross-checked against the **actual current source** (`cambium-core/src/{context,tree,list,schema,error}.rs`, the facade, and the Go mirror).

### The design is sound and correctly grounded
The gap analysis matches reality: today `DataTree` only does `validate`/`serialize`/`user_ordered_list_at`; `LeafType` collapses every type to `String/Int/Bool/Unknown`; `Error` is hand-rolled and discards the cause (only `Utf8` carries a `#[source]`); and the `cambium` facade doesn't even re-export `SchemaTree`/`DataTree`/`UserOrderedList`/`RuleCode`. The data layer (§C) and the rich `TypeInfo`/`ResolvedType` (§B.2) are exactly where Cambium overtakes goyang. The crown-jewel `UserOrdered` stays positional-only and extends into codegen as `UserOrderedVec<T>` — the I1 guarantee is preserved end to end.

### Three one-way doors — resolve before Phase 2
1. **Handle model (Open Q #1).** `{gen,path}` value handles mirror cleanly to Go but cost an O(depth) re-resolve per read and re-materialize `children()` each call. The borrowed `NodeRef<'tree>` model is what `yang3` uses **and what the existing `UserOrderedList<'a>` already does** (it holds a borrow over `&'a mut DataTree` today). So `{gen,path}` would make the new data layer inconsistent with the one ordered type we already shipped. **Lean: borrowed-for-Rust + a documented Go invalidation contract**, unless the cross-language byte-gate tooling specifically needs value handles.
2. **Codegen serialization (Open Q #6).** "Marshal-through-engine" makes every generated `to_json_ietf` require a live `Context` at runtime — heavy for the typed-struct ergonomics the SDK is selling. The native manifest-walk + CI byte-equality gate (what the *current* codegen and ygot both do) keeps generated code self-contained and fast. **Lean: native serializer + CI gate**; reserve engine round-trips for `validate()`/`diff()` that genuinely need libyang.
3. **Validated/Unvalidated typestate (Open Q #2).** Elegant in Rust, no clean Go mirror. **Lean: skip it**, keep runtime `validate(&mut self)`, to protect cross-language parity.

### goyang capabilities this design intentionally drops (compiled-only architecture)
- **No raw AST / parsed-statement view.** goyang exposes `yang.Node`/`Statement` (the un-compiled parse tree); Cambium works off libyang's *compiled* `lysc` tree, so the raw substatement view and unknown-statement access are gone. Defensible — the compiled view is what almost every SDK consumer wants — but it's a real difference. libyang's `lysp_*` parsed structs could back a P3 raw view if a linting/doc-gen use case ever needs it.
- **Grouping/typedef provenance.** Groupings are expanded and typedefs resolved in the compiled tree. `dedup_groupings` (the ygot-bloat fix in §H) needs to know "this struct came from grouping X" — but `lysc` may not retain that origin. **Flag:** verify libyang exposes enough provenance before promising `dedup_groupings`, or drop it from codegen scope.

### Smaller notes
- `Decimal` (in `Value::Decimal64` / `RangeBound`) isn't in std — model decimal64 as a newtype over `{ raw: i64, fraction_digits: u8 }` rather than pulling `rust_decimal` into the core.
- `NodeRef` needs a context handle to implement `schema()` (data→schema bridge) and typed `value()` — so the handle is really `{ctx, gen, path}`, not just `{gen, path}`.
- Reshaping `ParseMode` (enum → flags struct) and adding a `validate` param to `Context::parse` are **breaking changes** to the M0–M3 surface; Phase 1 must migrate the existing tests + v1 codegen in lockstep (fine pre-1.0, but budget it).
- The ordered-list insert API takes a whole parsed `DataTree` as an entry; once `new_path` lands, revisit so entries can be built in place rather than parsed separately.

### Verdict
Ready to become `docs/sdk-api-design.md` as the working proposal and, after the three forks above are locked, to seed `/spec/api.md`. The recommended build order (Phase 0 spec + red fixtures → Phase 1 rich schema/TypeInfo → Phase 2 data CRUD/validation → Phase 3 serialization+diff → Phase 4 Go parity → Phase 5 codegen v2) is correct: schema introspection is both the biggest goyang-parity gap and the prerequisite for real typed codegen.
