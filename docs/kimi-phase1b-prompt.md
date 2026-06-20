You are a senior Rust systems engineer running an UNATTENDED session on **Cambium** (signalbreak-labs), an order-correct YANG **library/SDK** (the goyang successor — NOT ygot; it does NOT send NETCONF or generate Terraform providers; see `AGENTS.md` "Non-goals"). Work autonomously to completion; do not wait for approval between steps.

You are continuing on branch **`feat/cambium-sdk-phase1`** (current HEAD `4ae6be8`). Stay on it. Commit each green slice separately. **Do not push.**

## Context — what already landed tonight (do NOT redo)
Phase 1 Slice A + B/C are in: `ContextBuilder` → frozen `Context`, borrowed `Module<'ctx>` / `SchemaNodeRef<'ctx>` handles, rich node metadata, and `TypeInfo<'ctx>` → `ResolvedType<'ctx>` with **decimal64 fraction-digits, enumeration values, bits positions, recursive unions, and instance-identifier/leafref `require_instance`** already extracted and tested. The domain types `Identity`, `IdentityBases`, `IdentityDerived`, `RangeBound`, `Pattern`, `EnumDef`, `BitsDef` already exist in `cambium-core::schema` — several are currently populated with empty/placeholder data. Your job is to **fill the deferred extractions**.

## READ FIRST (ground yourself in the real code before any unsafe read)
- `rust/cambium-libyang-sys/src/adapter.rs` — `extract_type_info()` and the **verified** `sized_array_count()` helper (reads a `u64` count immediately before a sized array; matches libyang's `LY_ARRAY_COUNT`). Mirror its proven patterns.
- `rust/cambium-libyang-sys/src/bindings.rs` — the **authoritative struct layouts** (bindgen output). Confirm every field name/type here before reading it.
- `rust/cambium-libyang-sys/vendor/libyang/src/tree_schema.h` (and `tree.h`) — the C definitions + sized-array convention.
- `rust/cambium-core/src/schema.rs` — the domain types you will populate (`ResolvedType` variants, `Identity*`, `RangeBound`, `Pattern`).
- `rust/cambium-core/tests/schema_introspection.rs` — extend the existing `cambium-introspection-demo` module + add the new red tests here.
- `spec/api.md` §Phase 1 + `docs/handoff-overnight-phase1.md` (Deferred work + "exact next red tests").

## Goal — finish Phase 1 type richness (Rust only; Go untouched)
Implement the four deferred extractions, **easiest-confidence-first** so partial progress maximizes:

1. **Range / length / pattern constraints.** Integer/decimal64 `range`, string/binary `length` (`lysc_range` → its `parts` sized array of min/max bounds → `Vec<RangeBound>`), and string `patterns` (`lysc_type_str.patterns` → `Vec<Pattern>` with regex `expr`, `inverted`, and `error-app-tag` when present). Populate the matching `ResolvedType` variants (`Int(_, range)`, `Decimal64 { range }`, `StringType { length, patterns }`, `Binary { length }`).
2. **Identityref bases + transitive derived closure.** `lysc_type_identityref.bases` and `lysc_ident.derived`. Populate `ResolvedType::IdentityRef { bases }` and `Identity::derived()` (transitive). This is where the earlier attempt hit `0x1` — see the **critical gotcha** below.
3. **Leafref target.** `lysc_type_leafref` exposes a compiler-resolved **`realtype`** (`*lysc_type`, the resolved target type) — verify in `bindings.rs`, then recurse `extract_type_info` on it to surface the resolved target type. If resolving a full target **node handle** (or printing the path expression from the opaque `lyxp_expr`) cannot be done safely, **surface `realtype` + defer the node handle with a note** — that satisfies most codegen needs.
4. **Default-value canonical strings** (lowest priority). If reading the type-specific `lysc_value`/default union is not confidently safe, **defer it to the Phase 2 data slice** and say so. Do not guess.

## CRITICAL FFI gotcha — the cause of tonight's `0x1` misalignment
libyang sized arrays come in two shapes; **you must distinguish them or you read garbage:**
- **Array of structs** (e.g. `lysc_type_enum.enums`, `lysc_type_bits.bits` = `lysc_type_bitenum_item *`): iterate with `(*t).enums.add(i)` → `*lysc_type_bitenum_item`. (Already done correctly.)
- **Array of POINTERS** (e.g. `lysc_type_union.types` = `lysc_type **`, and almost certainly `lysc_type_identityref.bases` = `lysc_ident **`, `lysc_ident.derived` = `lysc_ident **`, and `lysc_type_str.patterns` = `lysc_pattern **`): you need the **extra deref** — `*(*t).types.add(i)`. Treating a pointer-array as a struct-array (or vice-versa) is exactly what produced address `0x1`.
**Verify each field's pointer depth in `bindings.rs` first.** `sized_array_count()` takes the array pointer (works for both shapes — the count lives before the array regardless). For the transitive identity closure, BFS/DFS over `derived` with a visited-set to guard against cycles.

## Non-negotiable gates (a slice that can't meet these is reverted with `git checkout` and logged deferred — never commit red, never weaken a test, never guess C layouts)
1. **TDD red-first.** Add the failing test FIRST, watch it fail, then implement. Extend the existing `cambium-introspection-demo` test module (add an `identity` hierarchy + identityref leaf, a `leafref` leaf, a string leaf with `length`+`pattern`, an int leaf with `range`).
2. **Trust the compiled runtime, not header comments.** Header comments can mislead (e.g. `max-elements` is documented "0 means unbounded" but the *compiled* value is `UINT32_MAX`). Confirm what libyang actually returns via a test before encoding an assumption.
3. **Green before every commit:** `cargo build --workspace` · `cargo clippy --workspace --all-targets -- -D warnings` · `cargo test --workspace` · `cargo run -p conformance-runner` · `(cd go && CGO_ENABLED=1 go build ./... && go vet ./... && go test ./...)`.
4. **Do NOT touch the serialization/ordering path.** The `/conformance` goldens MUST stay byte-identical. Never edit/regenerate a golden.
5. **Hexagonal + house style:** raw extraction stays in `cambium-libyang-sys`, mapped to domain types in `cambium-core` at the boundary; no libyang types in `cambium-core`'s public API; no `unwrap`/`expect` outside tests; `thiserror`/`Result`; enums over bool+Option; `#![deny(missing_docs)]`; `cargo fmt --all` before the final commit.
6. **Keep the existing public API working** (legacy `SchemaTree`/`LeafType` stay deprecated-but-present). Go is untouched this run.
7. **Flag every new FFI/`unsafe` diff** in the handoff under "NEEDS HUMAN REVIEW".

## Acceptance tests to land green (add to `schema_introspection.rs`)
`leaf_type_int_range`, `leaf_type_decimal64_range`, `leaf_type_string_length`, `leaf_type_string_patterns` (incl. `error-app-tag` if modeled), `identity_derived_closure`, `leafref_realtype_resolves` (and `leafref_target_resolves` if the node handle proves safe). Defer any whose FFI can't be made confidently green, with reasons.

## Deliverable
Run `cargo fmt --all`, make a final commit, and append a "Phase 1b" section to `docs/handoff-overnight-phase1.md`: what landed per item (with green-bar output), what's still deferred and why, the **NEEDS HUMAN REVIEW** list of every new FFI/`unsafe` diff (especially the identityref/pattern pointer-array reads), confirmation the `/conformance` goldens are byte-unchanged, and the exact next red test. **Do not push.**
