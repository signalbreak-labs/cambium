# Handoff — Cambium Typed-Struct Codegen Final Prompt

## Status

All four items from `docs/kimi-codegen-final-prompt.md` landed green on branch
`feat/cambium-sdk-phase1`. No work was deferred.

| Item | Commit | Status |
|------|--------|--------|
| 1 — Typed union enums | `feat(codegen): emit typed union enums` | Landed |
| 2 — Foreign identityref XML prefix/xmlns | `feat(codegen): synthesize foreign identityref xmlns` | Landed |
| 3 — Empty non-presence omission + self-closing | `feat(codegen): omit empty non-presence containers and self-close empty presence containers` | Landed |
| 4 — Engine-routed serializer acceptance | `test(codegen): add engine-routed serializer acceptance gate` | Landed |

## Verification floor (all green)

```text
cargo fmt --all
cargo clippy --workspace --all-targets -- -D warnings
cargo test -p cambium-codegen --test codegen_v2   # 55 tests
cargo test --workspace                              # conformance 193/193
```

## What changed

### `rust/cambium-codegen/src/lib.rs`

- **Union types (Item 1)**
  - Added `is_union: bool` to `Field` and threaded it through `collect_fields` / `field_type`.
  - Added `emitted_unions: HashSet<String>` guard.
  - Added `ensure_union` emitting a discriminated enum `{Prefix}{Field}Union` with per-variant `write_json_value`, `write_xml_value`, `Display`, and a conditional `Default`.
  - Routed JSON/XML scalar and leaf-list emitters through the union enum when `field.is_union`.
  - Added `ResolvedType::Union(members)` arm to `rust_type_for` before the `String` fallback.

- **Foreign identityref XML (Item 2)**
  - Extended `collect_identityref_members` to capture `(module_name, name, variant, prefix, namespace)`.
  - `ensure_identityref` emits `xml_prefix_ns()` and `write_xml_value()` on the identityref enum.
  - XML leaf/leaf-list emitters inject `xmlns:<prefix>="<ns>"` and a prefixed value when the active variant is foreign.

- **Empty container handling (Item 3)**
  - Added `emit_has_content` producing a recursive `has_content(&self) -> bool` method on every container/list-entry struct.
  - Non-presence containers are skipped in XML/JSON walks when `has_content()` is false.
  - Emitted containers with no content serialize as self-closing XML (`<tag .../>`) and compact JSON (`{}`).

### `rust/cambium-codegen/tests/codegen_v2.rs`

- Added byte-gated tests for all union fixtures, the foreign identityref XML fixture,
  the empty-container fixtures, and the engine-routed acceptance gate.
- Added `engine_routed_xml_gate` helper that builds a detached temp crate linking
  `cambium-core` and re-serializes generated XML through the engine.

## Fixtures covered

- `types-union-enum-and-scalar`
- `types-union-scalar-all-members`
- `types-union-member-resolution-order`
- `types-union-identityref-member`
- `types-union-leafref-member`
- `types-union-nested-typedef-chain`
- `types-typedef-union-composition`
- `types-union-two-identityrefs-distinct-bases`
- `types-union-heterogeneous-members-quoting`
- `types-identityref-foreign-module-prefix`
- `container-presence-empty`
- `json-ietf-presence-vs-nonpresence`

## Notes for the next agent

- **Go parity**: `Lang::Go` is still rejected by `generate()`; this run was Rust-only.
  A future pass can mirror the Rust emitter output to Go structs once the Rust
  surface is accepted as the canonical shape.
- **Engine-routed gate scope**: Item 4 deliberately does **not** add `from_xml`,
  `from_json_ietf`, `validate`, or `diff` to generated structs. It only proves
  generated XML is accepted by `cambium-core::Context::parse` + `validate` +
  `serialize` and round-trips to the oracle. A true deserialize-into-struct
  round-trip requires new codegen and is out of scope.
- **Temp-crate pattern**: `engine_routed_xml_gate` writes a `Cargo.toml` with an
  absolute `cambium-core` path dependency and runs `cargo test --manifest-path`.
  Reuse it for additional engine-routed fixtures by copying the
  `codegen_engine_routed_xml_serializer_acceptance` test shape.
