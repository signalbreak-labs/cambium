# Cambium — Diagnostic Rule Codes

> Status: **normative draft v0.1** · Layer: `/spec` (shared contract) · Implemented identically in Rust (`cambium-core`) and Go (`package cambium`).

Every Cambium error carries a stable `CAMBIUM_E####` rule code. Codes are assigned by the **operation that failed**, so the same failure yields the same code in *both* languages — clippy-style diagnostics: a stable id callers can match on, plus a human message (and, when libyang supplies one, the YANG `error-app-tag`).

## Format
`CAMBIUM_E####` — `E` + a 4-digit number. The registry is **append-only**; a published code is never reused or renumbered.

## Registry (v0.1)
| Code | Name | Raised by (operation) | Meaning |
|---|---|---|---|
| `CAMBIUM_E0000` | Unknown | (fallback) | Unclassified internal error. |
| `CAMBIUM_E0001` | Context | new context · set search path · load module · schema tree / schema path lookup | Context/schema setup failed (e.g. module not found, bad search dir, missing schema path). |
| `CAMBIUM_E0002` | Parse | parse · parse op | Input could not be parsed: malformed, unknown schema (strict mode), invalid UTF-8, or illegal source characters including interior NUL. |
| `CAMBIUM_E0003` | Validate | validate · merge | RFC-7950 validation failed (`must`/`when`, `mandatory`, leafref, type restriction); also a `merge` conflict where both trees set the same leaf to different values. |
| `CAMBIUM_E0004` | Serialize | serialize | Serialization to XML/JSON failed. |
| `CAMBIUM_E0005` | OrderedList | user-ordered list lookup + positional ops | A path/positional list op failed (path not found, index out of range, non-user-ordered target). |
| `CAMBIUM_E0006` | DataPath | new path · get · set value · remove path · select (xpath) | A data-tree access/mutation by path or XPath failed: path not found or not creatable, invalid path/XPath expression, or a type-invalid value on `set_value`/`new_path`. |
| `CAMBIUM_E0007` | Stale | any data handle (`NodeRef`/`NodeSet`) accessor after an invalidating mutation | A data handle was used after a mutation advanced the owning tree's generation. Primarily a **Go** runtime safety net; in Rust the borrowed `NodeRef<'tree>` lifetime makes this a compile error, so it is rarely observed there. |

## Mapping rule (identical in both languages)
- **Go**: `cambium.Error.RuleCode() RuleCode`; the error string is `[CAMBIUM_E####] <op>: <cause>`.
- **Rust**: `cambium_core::Error::rule_code() -> RuleCode`; `Display` is `[CAMBIUM_E####] <cause>`.
- The code is derived from the operation and is **identical across languages** for the same failure. A conformance check should assert Go and Rust assign the same code to the same input (e.g. loading a missing module → `CAMBIUM_E0001` in both).

## Future (tracked, not v0.1)
- ~~Refine `E0002`/`E0003` into sub-codes~~ — **resolved by design**: `LY_VECODE` and the
  `error-app-tag` (RFC 8040 §7.1) are surfaced as informational sub-fields (`validation_code`,
  `error_app_tag`, `data_path`, `schema_path`) on the structured `Diagnostic` carried under the
  **stable** `E0003`, not as new top-level codes (preserves the append-only contract). See
  `spec/api.md` §Error contract.
- Per-code doc URLs (clippy-style `--explain`) once the docs site exists.
- A `RuleCode`-aware diff in the migration tooling so goyang/ygot users get mapped diagnostics.
