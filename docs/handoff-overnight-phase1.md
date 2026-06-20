> **Branch:** `feat/cambium-sdk-phase1`  
> **Session:** 2026-06-14 unattended Phase 1 + 1b pass  
> **Status:** green; no regressions, no push  
> **Commits on branch:**
> - `df15627` — feat(rust): Phase 1 Slice A — rich schema metadata + ContextBuilder + Module handles
> - `9ff6750` — feat(core): rich TypeInfo/ResolvedType introspection
> - `00975c8` — feat(core): Phase 1b deferred type extractions

# Overnight handoff — Cambium Phase 1

Phase 1 replaces the coarse `SchemaTree`/`SchemaNode`/`LeafType` surface with
goyang-grade schema introspection. Go is untouched in this phase.

## What landed

### Slice A — rich schema-node metadata + `ContextBuilder` + `Module` handles

- `ContextBuilder` → frozen `Context` typestate (`build()` consumes the builder).
- Borrowed `Module<'ctx>` and `SchemaNodeRef<'ctx>` handles (the spec literal
  `SchemaNode<'ctx>` name collides with the legacy owned type, so the new handle
  is `SchemaNodeRef`; the legacy type stays deprecated-but-present).
- Node metadata: `config`, `status`, `mandatory`, `presence`, `description`,
  `reference`, `units`, `default_value` (placeholder), `min_elements`,
  `max_elements`, `ordered_by_user`, `is_key`, `list_keys()` in key-statement
  order.
- `SchemaNodeRef::children()` walks libyang's compiled `next` chain in schema
  declaration order (I2).
- `cambium::prelude` re-exports the new public types.
- Legacy `SchemaTree`/`SchemaNode`/`LeafType` and `Context::new`/`schema_tree`
  remain present and deprecated so M0–M3 + v1 codegen keep compiling.

### Slice B/C — `TypeInfo` → `ResolvedType` sum type

- New domain types in `cambium-core::schema`:
  - `IntKind`, `FractionDigits`, `EnumDef`, `BitsDef`, `EnumValue`, `Pattern`,
    `RangeBound`, `Identity`, `IdentityBases`, `IdentityDerived`.
  - `ResolvedType<'ctx>` non-exhaustive enum covering ints, `decimal64`,
    `boolean`, `empty`, `binary`, `string`, `enumeration`, `bits`,
    `identityref` (bases empty), `instance-identifier`, `leafref` (target
    unresolved), and recursive `union`.
  - `TypeInfo<'ctx>` with `base()`, `typedef_name()`, `resolved()`.
- Adapter coarse extraction in `cambium-libyang-sys::adapter`:
  - Reads `lysc_type` for leaf/leaf-list nodes.
  - Extracts `decimal64` `fraction_digits`.
  - Extracts `enumeration` values and `bits` positions in declaration order
    using libyang sized-array semantics (`LY_ARRAY_COUNT_TYPE` stored before the
    array pointer).
  - Recursively extracts `union` member types.
  - Records `require_instance` for `instance-identifier` and `leafref`.
  - Defers ranges, patterns, `identityref` bases, and `leafref` path/target
    resolution (see below).
- New acceptance tests in `rust/cambium-core/tests/schema_introspection.rs`:
  - `children_declaration_order`
  - `schema_node_config_status_mandatory`
  - `list_keys_as_nodes`
  - `module_metadata`
  - `leaf_type_base_kind_all_builtins`
  - `leaf_type_decimal64_fraction_digits`
  - `leaf_type_enum_values_ordered`
  - `leaf_type_bits_positions_ordered`
  - `leaf_type_union_members_recursive`

## Deferred work + why

| Item | Why deferred |
|------|--------------|
| `identityref` direct bases + transitive derived closure | `lysc_type_identityref.bases` is a sized array; the first attempt read past the count and produced misaligned addresses (`0x1`). Safe iteration is now understood, but `lysc_ident` `next`/`bases` layout and module identity indexing need a verified binding walk before public `Identity::derived()` can return real data. |
| `leafref` target schema-node resolution | `lysc_type_leafref` carries a path expression (`lyxp_expr`) and optionally a cached target pointer; resolving it to a `SchemaNodeRef` needs a safe path-to-node lookup and verification against the compiled tree. |
| `range` / `length` / `pattern` constraints | `lysc_range`, `lysc_pattern`, and `lysc_restr` formatting are not yet extracted; they are stored as `None`/`Vec::new()` in the matching `ResolvedType` variants. |
| Default value canonical strings | `lysc_value` union reading is deferred to the data-layer Phase 2 slice. |

## NEEDS HUMAN REVIEW — FFI/unsafe diffs

- `rust/cambium-libyang-sys/src/adapter.rs`:
  - `sized_array_count()` reads a `uint64_t` immediately before the array
    pointer, matching libyang's `LY_ARRAY_COUNT` / `LY_ARRAY_COUNT_TYPE`
    convention. Verify this matches the vendored libyang SHA pinned in
    `/VERSIONS`.
  - `extract_type_info()` casts `lysc_type` pointers to type-specific structs
    (`lysc_type_dec`, `lysc_type_enum`, `lysc_type_bits`, `lysc_type_union`,
    `lysc_type_instanceid`, `lysc_type_leafref`) and reads enum/bit names via
    pointer arithmetic. The sized-array fix should be reviewed for off-by-one
    and alignment assumptions.
  - `extract_type_info()` identityref branch is currently a no-op comment; the
    deferred identity extraction must be re-written with the same sized-array
    discipline.
  - `list_min_max()` casts `lysc_node` to `lysc_node_list` / `lysc_node_leaflist`
    based on `nodetype`.

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
- `cargo test --workspace`: all crates pass, including the 9 new schema
  introspection tests and the compile-fail `user_ordered_list_no_set` test.
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
- Go build/vet/test: cached green (`ok` for `cambium`, `codegen`, `conformance`,
  `internal/libyang`).
- `cargo fmt --all`: no diff after formatting.

## Conformance goldens

`/conformance/golden/` files are byte-unchanged. Working tree is clean except for
`docs/handoff-overnight-phase1.md`.

## Exact next red tests

From `spec/api.md` Phase 1 acceptance list, the remaining failing targets are:

1. `leafref_target_resolves` — implement `lysc_type_leafref` path/target
   resolution and surface `ResolvedType::LeafRef { target: Some(...) }`.
2. `identity_derived_closure` — implement safe `lysc_type_identityref.bases`
   extraction and populate `Identity::derived()` via module identity tables.

Both require a verified libyang struct-layout pass before re-enabling the
extractor branches.

---

# Phase 1b addendum — deferred type extractions

## What landed in 1b

### Range / length / pattern constraints

- Adapter: `extract_range()` reads `lysc_range.parts` (sized array of
  `lysc_range_part`) and formats signed, unsigned, and decimal64 bounds.
  `extract_patterns()` reads `lysc_type_str.patterns` (pointer array) and
  captures `expr`, `error-app-tag`, and the `inverted` bitfield flag.
- Core: `ResolvedType::Int { range }`, `Decimal64 { range }`, `StringType {
  length, patterns }`, and `Binary { length }` are now populated.
- `Pattern::is_inverted()` exposes the invert-match flag.
- New tests: `leaf_type_int_range`, `leaf_type_decimal64_range`,
  `leaf_type_string_length`, `leaf_type_string_patterns`.

### Identityref bases + derived closure

- Adapter: `lysc_type_identityref.bases` (pointer array) is read to produce
  fully-qualified base identity names. Module-level `lysc_ident` identities are
  enumerated from `lys_module.identities` (struct array), and each identity's
  `derived` pointer array is captured.
- Core: a global `identity_map` (`module:name` → `(module_index,
  identity_index)`) resolves identity handles across modules.
  `ResolvedType::IdentityRef { bases }` now contains `Identity` handles.
  `Identity::derived()` performs a BFS/DFS over the transitive `derived`
  closure with a visited set to guard cycles.
- New test: `identity_derived_closure`.

### Leafref realtype resolution

- Adapter: `lysc_type_leafref.realtype` is recursed through `extract_type_info`
  when the compiler has resolved the target type.
- Core: `ResolvedType::LeafRef` gained `realtype: Option<Box<TypeInfo<'ctx>>>`;
  the unresolved node handle field `target` remains `None`.
- New test: `leafref_realtype_resolves`.

## Still deferred

| Item | Why deferred |
|------|--------------|
| Default-value canonical strings | Reading the type-specific `lysc_value` union safely requires more C struct verification and belongs to the Phase 2 data-layer slice. |
| Full leafref target **node handle** | `lysc_type_leafref.path` is an opaque `lyxp_expr`; resolving it to a concrete `SchemaNodeRef` needs a verified path-to-node walk and is not required for codegen once `realtype` is available. |

## NEEDS HUMAN REVIEW — new FFI/unsafe diffs in 1b

- `rust/cambium-libyang-sys/src/adapter.rs`:
  - `extract_range()` casts `lysc_range.parts` as a sized array of structs and
    reads the `min_64`/`max_64` or `min_u64`/`max_u64` union fields based on the
    type kind. Review that signed vs unsigned vs decimal64 selection matches the
    vendored libyang comment convention (`>= LY_TYPE_DEC64` uses signed 64-bit).
  - `format_decimal64()` formats scaled integers by `fraction_digits`; confirm
    the canonical representation matches downstream expectations.
  - `extract_patterns()` dereferences `lysc_type_str.patterns` as a pointer array
    (`*mut *mut lysc_pattern`) and calls the bindgen-generated `inverted()`
    bitfield accessor. Verify the accessor correctly masks the low bit.
  - `extract_type_info()` identityref branch now dereferences
    `lysc_type_identityref.bases` as a pointer array (`*mut *mut lysc_ident`).
  - `module_identities()` dereferences `lys_module.identities` as a struct array
    and each identity's `derived` as a pointer array.
  - `extract_type_info()` leafref branch recurses on `lysc_type_leafref.realtype`;
    guard against unusually deep/cyclic type graphs is by recursion depth only.

## Green bar after 1b commit `00975c8`

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
- `cargo test --workspace`: all crates pass, including 16/16 schema
  introspection tests and the compile-fail `user_ordered_list_no_set` test.
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
- Go build/vet/test: cached green.
- `cargo fmt --all`: no diff after formatting.

## Conformance goldens

`/conformance/golden/` files remain byte-unchanged after Phase 1b.

## Exact next red tests

With the Phase 1 acceptance list now complete except for the explicitly
out-of-scope node-handle variant, the next red tests move to Phase 2 data
layer:

1. `new_path_builds_intermediates` — data-tree CRUD begins.
2. `validate_must_when_fails_with_path` — structured validation diagnostics.

Default-value canonical strings may be picked up as part of either the data
layer or a small follow-up once `lysc_value` layout is verified.
