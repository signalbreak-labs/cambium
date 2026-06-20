# Phase 5 typed-struct codegen handoff

This document records what landed on branch `feat/cambium-sdk-phase1` during the
Slices 4–5 continuation run, what remains deferred, and what needs human review
before the next slice.

<!-- prettier-ignore -->
> [!NOTE]
> The success floor defined in the work order was Slices 1a + 2 plus the first
> two Slice-4 features (decimal64 and bits). This run extends the branch with
> the remaining typed-mapping stretch goals.

## What landed

Each slice was committed green separately. The v1 codegen tests were not
modified and remain green.

### Slice 4 — IdentityRef enum

- Mapped `ResolvedType::IdentityRef` to a generated enum over the transitive
  `Identity::derived()` closure.
- Variants are sorted by fully-qualified identity name (`module:name`) for
  deterministic Rust/Go agreement.
- JSON uses `as_json_name()` (module-qualified for foreign identities); XML uses
  bare `as_name()`.
- Byte-gated same-module fixtures (`types-identityref-single-base`,
  `types-identityref-derived-hierarchy`).

### Slice 4 — Ranged/length newtypes

- Every integer leaf now emits a per-field `Range` newtype with a fallible
  `new(value: i128) -> Result<Self, _>` constructor. The constructor enforces
  explicit YANG ranges, or the natural type bounds when no range is present.
- String leaves with a `length` constraint emit a per-field `Length` newtype
  with a fallible `new(String) -> Result<Self, _>` constructor. Patterns are
  preserved in the schema but not validated in generated code.
- XML and JSON serialization delegate through `Display` (for ints) and
  `as_str()` (for strings) so wire bytes match the bare scalar case.

### Slice 4 — LeafRef chasing

- When `ResolvedType::LeafRef` has no `realtype`, the emitter follows the
  `target` `SchemaNodeRef` and reuses the target leaf's resolved type.
- Fixed `to_pascal_case` so raw identifiers such as `r#ref` produce valid
  PascalCase type names.
- Byte-gated `types-leafref-absolute-path`, `types-leafref-to-leaf-list`, and an
  inline int-target leafref fixture.

### Slice 5 — `CambiumStruct` trait

- Emitted a `CambiumStruct` trait with native `to_xml(&self) -> String` and
  `to_json_ietf(&self) -> String` methods.
- Implemented the trait for every generated struct, including nested containers
  and list entry structs.
- Added `to_xml` / `to_json_ietf` convenience methods on non-root structs so the
  trait implementation delegates to an inherent serializer without recursion.

## Green-bar confirmation

Run on the branch before this handoff:

```text
$ cargo fmt --all

$ cargo test -p cambium-codegen --test codegen_v2
   running 38 tests
   test result: ok. 38 passed; 0 failed; 0 ignored

$ cargo clippy --workspace --all-targets -- -D warnings
   Finished `dev` profile [unoptimized + debuginfo] target(s) in 3.48s

$ cargo test --workspace
   conformance: 193 case(s) passed
   (all workspace test targets pass)
```

## Commits on this branch

- `9ab2737` feat(codegen): emit CambiumStruct trait for all generated structs
- `fcf366d` feat(codegen): chase leafref target type and handle raw-idents
- `0b4098b` feat(codegen): emit ranged int and length-bounded string newtypes
- `8c6c41c` feat(codegen): identityref enum + same-module byte-gate

## Deferred or gapped constructs

These are named, not silently skipped.

- **Union** — complex recursive feature (`ResolvedType::Union(Vec<TypeInfo>)`).
  A union member can itself be a union, identityref, bits, leafref, or scalar,
  so it needs a discriminated enum whose variant types are recursively emitted.
  Deferred to a dedicated follow-up slice.
- **Foreign-module identityref XML** — libyang synthesizes a short prefix and
  injects a matching `xmlns:` declaration on the element (for example
  `tifb:cpu` with `xmlns:tifb="urn:..."`). Reproducing this prefix synthesis in
  a self-contained native walk is a known hard gap. JSON for foreign identities
  is byte-gated; XML for the foreign fixture is deferred.
- **Engine-routed `CambiumStruct` methods** — `from_xml`, `from_json_ietf`,
  `validate`, and `diff` require a live `&Context` and `DataTree`, so they cannot
  be emitted into the standalone generated source that the existing test harness
  compiles with `rustc` alone. The reconciled native serializer methods are
  present; engine-routed round-trip methods are deferred until the harness can
  link `cambium-core` in generated test crates.
- **JSON control-char escaping** — string/enum values use Rust `{:?}`, which is
  valid JSON for normal strings but diverges from libyang for control chars
  `< 0x20` and DEL (`\u{XX}` vs `\uXXXX`). Needs a `cambium_json_escape` helper.
- **Empty non-presence containers** — a non-presence container with no present
  descendants is serialized where libyang omits it. Needs a recursive
  `has_content()` guard.

## Needs human review

- **IdentityRef variant sort order:** sorted by `module:name` ascending.
- **Bits canonical order:** already landed as ascending position (separate
  commit).
- **JSON phantom-node boundary:** the byte-gate uses `Format::Json` instead of
  `Format::JsonIetf` to avoid the forced `ietf-yang-schema-mount:schema-mounts`
  empty container.
- **`CambiumStruct` scope:** confirm that landing native `to_xml` /
  `to_json_ietf` first, with engine-routed methods deferred, matches the
  ratified reconciliation.
- **Range newtype default constructor:** the generated `Default` for an integer
  range newtype is `0`, which may fall outside an explicit range. No current
  code path calls it, but it is public.

## Next steps

1. Review the items in **Needs human review**.
2. Pick up **Union** next, since it is the largest remaining Slice-4 feature.
3. Resolve the **foreign-module identityref XML** gap or keep it explicitly
   deferred.
4. When the test harness supports linking `cambium-core`, implement the
   engine-routed `CambiumStruct` round-trip methods.

## Post-review of the Slice 4-5 continuation (2026-06-15)

Adversarial review (generate-from-more-fixtures + compile + byte-gate) confirmed
**3 real bugs** (7 findings deduped), all in deferred/edge areas that the
committed 38 codegen tests + conformance 193 never exercise. **These are
MUST-FIX in the final codegen run, before that run adds Union / full
foreign-module identityref / engine-routed CambiumStruct.** Fixing them needs
red-test-first against the named goldens — do NOT rush.

1. **Foreign-module identityref emits NON-COMPILING code (high).** For a leaf
   whose identityref base lives in an imported module
   (`types-identityref-foreign-module-prefix`), the schema layer does not resolve
   the foreign base, so `ensure_identityref` collects zero identities and emits
   `pub enum ...Enum {}`. The generated source then fails rustc with E0665
   (`#[derive(Default)]` on an empty enum) and E0004 (non-exhaustive match). The
   handoff previously understated this as "missing XML prefix synthesis" — it is
   worse: the output does not compile. Fix: resolve foreign-derived identities
   across the import boundary (so the enum has variants) AND guard the empty case
   (fall back to `String`, never emit an empty enum). The XML prefix synthesis
   (`tifb:cpu` + `xmlns:tifb`) remains the genuinely-deferred part.

2. **JSON string escaping is invalid (high).** Every JSON string site uses Rust
   `{:?}` (lib.rs emit_scalar_json ~1384-1400, emit_json_leaf_list_element
   ~1421-1437, top-level ~1458+), which emits `\u{1}` (brace form, INVALID JSON)
   for control chars and diverges from libyang. The golden
   (`json-ietf-string-escaping-control-unicode`) shows libyang's exact, quirky
   rules: LF -> `
` (4-hex), TAB -> `\t` (named), `"`/`\` named, unicode &
   emoji passed through RAW (no escape). Fix: add a `cambium_json_escape` helper
   that REPLICATES libyang's golden byte-for-byte (iterate against that fixture),
   replace every `{:?}` string site, and add a codegen byte-gate for it.

3. **Range/length newtype `Default` is unsound (medium).** The range int newtype
   (lib.rs ~858) and length string newtype (~915) blanket-derive `Default`, which
   yields `0` / `""` even when the YANG range excludes them (e.g.
   `range "-100..-1 | 1..100"`: `default().get() == 0` while `new(0)` errors).
   Fix: emit a bounds-respecting `Default` (range minimum / a min-length value),
   or omit `Default` and cascade-drop it from any struct with a non-optional
   field of that newtype (the house-style "make illegal states unrepresentable"
   path) — note the cascade so the struct derives stay consistent.

Refuted (no action): union scalar fallback works, leafref realtype chasing is
fine, the handoff accurately names Union deferred. Also still open from the prior
post-review: the trybuild `.stderr` / toolchain-pin hygiene follow-up.

### Resolution (2026-06-15) — all 3 fixed inline, TDD red-first

1. **Foreign-module identityref — FIXED (`4050d82`).** Root cause was in the FFI
   adapter, not the schema layer: `schema_modules()` skipped any module whose
   compiled data root is null, so a base module imported solely for its
   identities (no data nodes) was dropped from the forest, its identities never
   registered, and `find_identity` returned `None` for every base. The adapter
   now surfaces data-less modules that define identities. With the full module
   set loaded the enum resolves to the real foreign-derived variants and
   serializes module-qualified in JSON_IETF (byte-gated vs libyang; red without
   the adapter fix). Defense: when a base still resolves to nothing (only the
   consuming module loaded), the codegen degrades the field to `String` rather
   than emit a non-compiling empty enum. XML prefix synthesis (`tifb:cpu` +
   `xmlns:tifb`) remains the genuinely-deferred part.

2. **JSON string escaping — FIXED (`003406d`).** Added `cambium_json_escape`,
   replicating libyang's `json_print_string` byte-for-byte (named escapes only
   for `"` `\` `\r` `\t`; every other control char incl. newline/DEL as uppercase
   `\uXXXX`; UTF-8 raw). Every `{:?}` string site now routes through it. Byte-gate
   over `json-ietf-string-escaping-control-unicode` (fails on the old Debug path).

3. **Range/length newtype `Default` — FIXED (`93dca52`).** Emit a
   bounds-respecting `Default`: keep the derive when `0` / `""` is in range (so
   `derivable_impls` stays quiet), else hand-roll an impl returning the range
   minimum / a min-length string. Red-then-green gates for a `range "1..65535"`
   int and the `length "1..10"` fixture. Also fixed a latent
   `manual_range_contains` clippy lint the new coverage exposed (checks now emit
   `(lo..=hi).contains(&x)`).

Full workspace + conformance (193/193, yanglint oracle) green; `cargo clippy
--workspace --all-targets -- -D warnings` clean. Still open: the trybuild
`.stderr` / toolchain-pin hygiene follow-up, and typed foreign-identityref XML
prefix synthesis (part of the final codegen run).
