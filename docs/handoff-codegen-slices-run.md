# Handoff — Cambium Typed-Struct Codegen Slices 3–5

Branch: `feat/cambium-sdk-phase1`  
Start SHA: `26f1f0e3e5cbddb927fd4429a029bb87b1186c6c`  
Run date: 2026-06-15

## What landed (required floor — fully green and committed)

All required-floor items passed the six codegen gates (`#![deny(missing_docs)]`, `#![deny(warnings)]`, clippy `-D warnings`, libyang XML byte-gate, libyang JSON byte-gate, and all prior v1/v2 tests).

### Slice 3 — `UserOrderedVec<T>` + I1 compile-fail guard
- Emits a positional-only owned `UserOrderedVec<T>` inline when any `ordered-by user` list/leaf-list is present.
- Field typing switches `Vec<T>` → `UserOrderedVec<T>` for user-ordered nodes.
- XML/JSON serializers use `.iter()` so they work for both containers.
- trybuild compile-fail guard covers `IndexMut`, `push`, and `swap` misuse (one per file).
- Positive-control runtime test exercises `insert_first/last/before/after`, `move_before/after`, `remove`, `len`, `is_empty`, `get`, `iter`.
- Byte-gated against `ordering-nested-user-cascading` golden XML and JSON.
- Commit: `4a9f047`

### Slice 4 — Decimal64
- The existing inline `Decimal64` newtype and `Display` implementation were already present in the floor.
- Added XML + JSON byte-gate tests for all five decimal64 fixtures:
  - `types-decimal64-fraction1-range`
  - `types-decimal64-fraction2-canonical-round`
  - `types-decimal64-fraction3-and-6`
  - `types-decimal64-fraction9-negative`
  - `types-decimal64-fraction18-max-magnitude`
- Field-type assertions prove generated leaves are `Option<Decimal64>` (not `String`).
- Commit: `c594815`

### Slice 4 — Bits
- Emits a per-field bits newtype (bitflags-style) named from the disambiguated field ident, e.g. `{Prefix}{Field}Bits`.
- Stores set bit names plus their positions; constructor `new(names: &[&str]) -> Result<Self, String>` validates against the static `BIT_POSITIONS` table.
- `Display` serializes set bits in **ascending position order**, space-separated.
- Byte-gated XML and JSON against `types-bits-explicit-positions-gaps`.
- Added a reordering unit test that constructs with `["delete", "read"]` and asserts output `"read delete"`, proving a String fallback would diverge.
- Commit: `1a2b78d`

## What was NOT generated this run (stretch, deferred, or out of scope)

These are named gaps, not silent skips.

### Slice 4 stretch — not attempted
- **IdentityRef enum** (`types-identityref-*` fixtures): deferred. JSON cross-module qualification and derived-closure sorting are tractable but were not started. XML cross-module prefix synthesis is the known hard gap.
- **Union recursive enum** (`types-union-*` fixtures): deferred. Needs recursive type emission plus correct RFC-7951 scalar quoting per resolved member.
- **LeafRef chasing** (`types-leafref-*` fixtures): deferred. Requires following `realtype` recursively to a concrete base.
- **Ranged/length-bounded newtypes** (`Int{range}`, `String{length}`): deferred. No serialization-golden fixture exists; gate would be a constructor unit test + clippy + bare-scalar byte-gate.

### Slice 5 — not attempted
- **`CambiumStruct` trait** with native `to_xml`/`to_json_ietf` and engine-routed `from_xml`/`from_json_ietf`/`validate`/`diff`: deferred.
- **Path-builder gate**: left as documented deferred error (`with_path_builder` still returns `UnsupportedOption`).

### Cross-language / Go
- **Go v2 codegen is explicitly out of scope** for this run per §6.3 of the prompt. The Rust-only gates above are the binding ones.

## "Needs human review" decisions recorded

| Decision | Choice made | Status |
|---|---|---|
| `UserOrderedVec<T>` positional-only contract | Implemented exactly as specified (no `set`/`push`/`IndexMut`/`swap`/`Deref<Target=Vec<T>>`). | Landed and compile-fail guarded. |
| Bits canonical serialization order | **Ascending bit position** (not declaration order). Byte-gated on `types-bits-explicit-positions-gaps`. | Landed. |
| IdentityRef derived-closure variant order | Not implemented, but future work should sort by fully-qualified name `module:name` ascending for Rust/Go parity. | Deferred. |
| Cross-module identityref XML prefix-synthesis | Not attempted. libyang synthesizes short prefixes + `xmlns:`; native reproduction is the hard gap. | Deferred; JSON-only gate should be attempted first. |

## Commit summary

```
7a7920b style(codegen): cargo fmt
5bc7fca docs: handoff for codegen Slices 3-5 run
1a2b78d feat(codegen): bits newtype with position-ordered Display
c594815 feat(codegen): byte-gate decimal64 across all fixtures
4a9f047 feat(codegen): UserOrderedVec<T> + I1 compile guard
26f1f0e docs: Kimi prompt — typed-struct codegen Slices 3-5  (start SHA)
```

Nothing was pushed.

## Post-review (2026-06-15)

Adversarial review of this run (byte-gate honesty, soundness, generated-code
quality): **1 confirmed finding (medium), 2 refuted** — the delivery is sound.

Confirmed clean:
- `UserOrderedVec<T>` is genuinely positional-only: inner `items: Vec<T>` is
  private; no `IndexMut`/`push`/`swap`/`Deref`. The I1 compile-fail guard is not a
  stale snapshot — `compile_fail.rs` regenerates `_generated_user_ordered_vec.rs`
  from the live emitter each run, so the trybuild misuse cases exercise the actual
  generated type and fail for the right reasons (E0608 / E0599).
- The bits and decimal64 byte-gates are real (generate-from-fixture, compile+run,
  compare to libyang); not vacuous.

Confirmed finding (medium) — **trybuild `.stderr` brittleness (project-wide,
pre-existing):** the compile-fail `.stderr` files pin rustc's exact output. The
prompt asked to trim the `help:` block, but trimming breaks trybuild's exact
full-file match, so the full `.stderr` is correct. The project builds on STABLE
(edition 2024, let-chains stable, no `#![feature]`) yet the 9 `.stderr` files
across the workspace are coupled to this machine's `nightly-1.96`, so they would
fail on a different toolchain. **Follow-up (toolchain-policy decision):** pin a
`rust-toolchain.toml` (recommend a recent stable), regenerate all 9 `.stderr` via
`TRYBUILD=overwrite` and review, so the compile-fail gate is deterministic.

Floor status: Slice 3 + Slice 4 (decimal64, bits) landed. **Deferred to the next
run:** the rest of Slice 4 (identityref, union, leafref, ranged/length newtypes)
and Slice 5 (CambiumStruct trait + path-builder + Go v2 parity).
