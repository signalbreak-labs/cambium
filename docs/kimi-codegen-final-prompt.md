# KIMI WORK-ORDER — Complete the Cambium Typed-Struct Codegen Surface

## Objective

Make `cambium-codegen` produce typed, byte-correct Rust for **every** remaining conformance construct so the generated typed-struct surface is production-complete and demonstrably "much better than goyang." Four gaps remain. Close them, in priority order:

1. **Union types** — recursive discriminated Rust enum over typed members (scalar, union-of-union, identityref, leafref, bits, enum), where the generated union enum **owns its own per-variant serialization**; byte-exact vs the libyang oracle in **both** XML and JSON_IETF.
2. **Foreign-module identityref XML** — synthesize the short module prefix + matching `xmlns:` declaration on the value-carrying element (golden: `<component xmlns="urn:..." xmlns:tifb="urn:types-identityref-foreign-base">tifb:cpu</component>`), conditioned on the **active** variant being foreign, so the foreign fixture's **XML** byte-gate passes (JSON already lands).
3. **Empty non-presence container omission + self-closing empty containers** — a recursive `has_content()` guard that omits a non-presence container with no present descendants, AND a self-closing-tag XML path for a container that is emitted but contentless (the present-but-empty presence container).
4. **Engine-routed serializer-acceptance gate** — upgrade the harness to build a generated cargo test crate that links `cambium-core` (path dep) and re-feeds generated `to_xml()` bytes back through `Context::parse` + `DataTree::validate` + `DataTree::serialize`, comparing to the oracle. **This is a serializer-acceptance check, NOT a struct round-trip** (no deserialization codegen exists). Lowest priority and deferrable — see Stop-and-hand-off.

### WHY
The typed-struct codegen is the headline deliverable: load YANG → typed structs + per-struct `FIELD_ORDER` manifest + a native ordered serializer whose bytes equal libyang's. Union is the largest remaining feature; foreign-identityref XML and empty-container handling are the last byte-correctness divergences from libyang. After Items 1–3, every conformance construct is typed and on-wire-correct in both formats.

---

## NON-NEGOTIABLES (read before writing any code)

These are project law (from `/spec/` and `AGENTS.md`). Violating any one fails review.

- **TDD, red-test-first, ALWAYS.** Write the failing test and watch it fail before any production line — no code, not even a stub, ahead of a red test. Every construct gets a golden-backed test. Coverage is a floor, never to drop.
- **Golden bytes are GENERATED from libyang/yanglint and reviewed at implementation time — NEVER hand-authored.** The committed goldens under `conformance/golden/` and the in-test oracle helpers are the source of truth. Review every diff; never blind-accept.
- **Serialization is exactly ONE ordered walk.** All XML/JSON output is a single in-order walk reproducing schema declaration order (I2), keys-first (I3), and `ordered-by user` insertion order (I1). NEVER derive element/member order from a hash map, reflected struct field order, or per-language collection iteration. **A union's serialized form is the serialized form of its active member — there is no union-level reordering. Field/member order comes from the `FIELD_ORDER` manifest and the schema, not Rust enum/struct layout.**
- **Hexagonal, non-negotiable.** The domain core (`cambium-core`) imports ZERO libyang/cgo types; FFI is the only adapter and is coarse-grained (whole-document parse/serialize per call; no per-node FFI, no C→Go callbacks in hot paths). Read values via `lyd_get_value()`.
- **`ly_ctx` is build-once-then-frozen.** Do not touch that contract. Data trees are not concurrency-safe.
- **Generated code is fully self-contained for Items 1–3.** It compiles with `rustc --test` alone (no `cambium-core` link). Every emitted helper/struct/enum/method MUST carry a doc comment (`#![deny(missing_docs)]`) and produce zero warnings (`#![deny(warnings)]`) and pass `cargo clippy -- -D warnings` on a detached throwaway crate. Use `_`-prefixed params for unused; use **conditional** `Default` impls to dodge `clippy::derivable_impls` / uninhabited-default (mirror `ensure_int_range` / `ensure_string_length`).
- **Helper type names are derived from `to_pascal_case(field_ident)`**, never the raw YANG name (two YANG names can collapse to one PascalCase and clobber). Follow `{prefix}{Pascal(field_ident)}{Kind}` (Enum/Bits/Range/Length/**Union**).
- **Ordering invariants I1–I6 hold.** `ordered-by user` stays positional-only `UserOrderedVec<T>` — no order-agnostic setter; reordering an ordinary `Vec` field stays a compile error. Do not regress the trybuild compile-fail tests.
- **NO NETCONF / Terraform coupling.** No `<edit-config>` builders, no transports, no provider emitters. Only generic ordered XML / JSON_IETF serialization + typed-struct codegen.
- **Conventional Commits**, imperative subject ≤50 chars, **one logical change per commit**. `Cargo.lock` stays committed. **NEVER push.** Branch off `main`; do not open PRs unless asked.

---

## Starting state (green floor — do not regress)

- `cargo test -p cambium-codegen --test codegen_v2` — baseline green (38 tests as of the last handoff).
- `cargo clippy --workspace --all-targets -- -D warnings` — clean.
- `cargo test --workspace` conformance — **193/193** via the yanglint oracle.

**Do NOT re-raise the already-fixed items** (build on top, do not regress):
- JSON control-char escaping — FIXED (`cambium_json_escape`, commit 003406d).
- Range/length newtype bounds-respecting `Default` — FIXED (commit 93dca52).
- Foreign-module identityref **base resolution** — FIXED (commit 4050d82); JSON_IETF already byte-gated. Only the **XML prefix/xmlns synthesis** remains (Item 2).

---

## Harness facts you must rely on (verified against the live tree)

- `byte_gate_fixture_xml_json(fixture, instance_expr, src)` (`codegen_v2.rs:1007-1043`) emits BOTH an XML and a JSON byte-gate against the libyang oracle, where `instance_expr` is a **hand-written Rust root-struct literal** you supply. It internally calls `load_ctx_for_dir` (`:1010`), which is a **SINGLE-module loader** (`read_dir().next()` stem). **It is therefore usable ONLY for single-module fixtures.**
- The JSON oracle (`libyang_reference_json_from_input`, `:679-693`) serializes with **`Format::Json`**, NOT `Format::JsonIetf` — this avoids the phantom `ietf-yang-schema-mount:schema-mounts` container. The goldens were produced the same way. **Keep all new JSON byte-gates on the existing helper / `Format::Json`; do NOT introduce `Format::JsonIetf` serialization.**
- The XML oracle is `libyang_reference_xml_from_input(&ctx, &input)` (`:374-388`); output is COMPACT single-line per node and reproduces the committed `output.xml` byte-for-byte.
- Multi-module fixtures must use the **enumerate-and-sort-all-`.yang`-stems, load each, then `build()`** pattern shown in the existing test `codegen_identityref_foreign_module_resolves` (`codegen_v2.rs:1259`, loop at `:1267-1285`), then a **manual** `compile_and_run` wrapper. `module_name_from_dir`/`load_ctx_for_fixture`/`load_ctx_for_dir` are non-deterministic on multi-module dirs — do not use them there.
- The generated source is serialize-only (`to_xml`/`to_json_ietf` + the `CambiumStruct` trait). There is no parser/`from_xml`. So a byte-gate compares **a hand-picked instance literal's** serialized bytes to the oracle — the test author chooses the variant; the generator never resolves a wire string.

---

## ITEM 1 — UNION TYPES (highest priority)

### Scope
Replace the `ResolvedType::Union(_) | _ => String` catch-all at **`rust/cambium-codegen/src/lib.rs:477-481`** with a typed arm that emits a **discriminated Rust enum** whose variants are the recursively-emitted member types. The generated union enum **owns its own serialization** (see Design below). Keep a `String` catch-all for genuinely-unmapped variants — `ResolvedType` is `#[non_exhaustive]`.

Members to support (all already fully resolved at context-compile time; no core change needed):
- Scalar members (int8…uint64, decimal64, boolean, string with/without length, binary as base64 string).
- **Union-of-union** (a member whose `resolved()` is itself `ResolvedType::Union` — flatten by recursion).
- **Union-with-identityref** (member is `IdentityRef{bases}` — reuse identityref enum emission).
- **Union-with-leafref** (member is `LeafRef` — chase `realtype`, then `target.leaf_type()`, else String, exactly as `rust_type_for` already does at `lib.rs:395-411`).
- **Union-with-bits / enum** (reuse existing bits/enum emitters).
- Member declaration order is YANG order (`lysc_type_union.types`); typedef'd members carry `m.typedef_name()` for naming/dedup.

### Design — union enum owns its serialization (do NOT bake a single JsonKind on the Field)
A `Field` models ONE union leaf; its members have DIFFERING quoting decided by the **active variant at runtime**. There is NO single per-member JsonKind to store on the Field, and you cannot select a JsonKind at emit time (`emit_scalar_json`, `lib.rs:1353-1385`, bakes one format-template per field). Instead:

- Add a single carrier flag **`is_union: bool`** to the `Field` struct (`lib.rs:1435-1445`) and thread it through `collect_fields` (`lib.rs:287-314`) and the `field_type` return tuple (`lib.rs:316-385`). **Do NOT add a per-member JsonKind to Field.**
- The generated **union enum** exposes inherent value-serialization methods, e.g.:
  - `fn write_json_value(&self, w: &mut String)` — `match self { Variant(x) => ... }`, rendering each member with **that member's own quoting**: bare for int8..int32/uint8..uint32 and boolean; quoted-string for int64/uint64/decimal64 AND for enum/identityref/bits/string/**binary**/instance-identifier. Route string-typed members through `cambium_json_escape`.
  - An XML value method, e.g. `fn write_xml_value(&self, w: &mut String)` (or one returning `(String, needs_escape)`), rendering each member's XML lexical form with the correct escaping; numeric/bool unescaped, string-like escaped via `cambium_xml_escape_text`.
- At the **four serializer branch points**, when `field.is_union`, DELEGATE to the union enum's method instead of selecting a JsonKind / instead of the `is_string_like` `.to_string()` path:
  - `json_string_value_expr` (`lib.rs:1340-1351`),
  - `emit_scalar_json` (`lib.rs:1353-1385`) — emit `"wire": ` then call the enum's `write_json_value`,
  - `emit_json_leaf_list_element` / `_top` (`lib.rs:1387-1427`) — same for array elements,
  - `xml_value_expr` (`lib.rs:1462-1531`) — **branch on `field.is_union` BEFORE the `is_enum`/`is_identityref`/`is_string_like` checks.** Note: `is_string_like` (`lib.rs:1514-1531`) currently returns `true` for `ResolvedType::Union(_)`, so without an explicit earlier `is_union` branch a union value falls into the `.to_string()` path. Do NOT rely on the union enum's `Display` for XML when a member is a foreign identityref — that needs prefix/xmlns synthesis (Item 2), not just the lexical value; the `is_union` XML branch must delegate to a method that can carry it.

### Helper injection (follow the per-name template — Enum/Bits/Range/Length)
- Add a dedup guard `emitted_unions: HashSet<String>` to the `Emitter` struct (`lib.rs:146-186`).
- Add `ensure_union(&mut self, name, members)`: guard on `emitted_unions`, build the discriminated-enum helper String (variants, `write_json_value`/`write_xml_value`, `Display`, **conditional `Default`** over the first member to dodge `derivable_impls`/uninhabited-default), recursively emit member helper types via `rust_type_for`, and `self.helpers.push(...)`.
- Helper type name: `{prefix}{Pascal(field_ident)}Union` from `to_pascal_case(field_ident)`. Member variant names disambiguated like `collect_identityref_members` (`lib.rs:1484-1512`) does.
- Add the `ResolvedType::Union(members)` arm to `rust_type_for` BEFORE the wildcard; recurse `rust_type_for` per member to build the variant payload types.

### Per-member JSON quoting (the byte differences to reproduce)
From `types-union-scalar-all-members` golden (all members are siblings, but each is the resolved form of one union leaf — confirm quoting against it): `"b": false` and `"i8": -128` and `"u16": 65000` → **bare**; `"i64": "-9223372036854775808"`, `"u64": "18446744073709551615"`, `"d": "-1.5"` (decimal64 **canonicalizes** `-1.50`→`-1.5`) → **quoted numeric-as-string**; `"s": "str"`, `"e": "beta"`, `"bits": "flag1 flag2"`, `"bin": "SGVsbA=="` (binary base64) → **quoted JSON string**. The union enum's `write_json_value` must select quoting per active variant accordingly.

### HARD cases (call out so they are not missed)
- **`types-union-member-resolution-order`** — input `5` resolves to `uint32` (`5`, unquoted), NOT enum `five`. **Serialize-only codegen does NOT perform member resolution** (libyang resolves at parse time; the data tree stores a union as `Value::Str`, no type discrimination). This gate proves ONLY that the per-variant quoting is correct: construct the `Uint32(5)`-equivalent variant and assert it serializes bare `5`, AND construct the enum variant and assert it serializes quoted `"five"`, demonstrating the per-variant quoting divergence. **Do NOT claim this gate verifies resolution-order; do NOT build an RFC-7950 member-resolution algorithm** — the serializer never needs it. (The RFC fact that `5` is not a valid enum label is only why the oracle's golden is `5`.)
- **`types-union-nested-typedef-chain`** and **`types-typedef-union-composition`** — both members serialize as plain strings, so the byte-gate alone would PASS even with a `String` fallback. For these two fixtures the **generated-source TYPE assertion (field is the `{Pascal}Union` enum, not `String`) is the load-bearing typing gate** and must NOT be skipped.
- **Union member that is a foreign identityref** — out of scope this run; the named identityref-union fixtures are SAME-module (values serialize bare: `tcp`, `linecard`), so Item 1 needs no foreign-prefix synthesis. If a future union member is a foreign identityref, its XML depends on Item 2.

### Acceptance gates (OBJECTIVE — named fixtures + byte-gate vs oracle)
For each, FIRST assert the generated field TYPE (string-match on `generated.source`: the field is the `{Pascal}Union` enum, not `String`), THEN byte-gate. All are single-module → use `byte_gate_fixture_xml_json(fixture, instance_expr, &generated.source)`; supply only the Rust root-struct literal (e.g. `TypesUnionEnumAndScalar { mode: Some(<chosen-union-variant>) }`).
- **`types-union-enum-and-scalar`** — XML + JSON_IETF.
- **`types-union-scalar-all-members`** — per-member quoting (bare vs quoted) + decimal64 canonicalization + binary base64.
- **`types-union-member-resolution-order`** — both-variant quoting divergence (see HARD case; NOT a resolution gate).
- **`types-union-identityref-member`** — XML + JSON_IETF (same-module bare).
- **`types-union-leafref-member`** — XML + JSON_IETF (realtype chase).
- **`types-union-nested-typedef-chain`** + **`types-typedef-union-composition`** — TYPE assertion is the real gate; byte-gate as a floor.
- **`types-union-two-identityrefs-distinct-bases`** — two identityref members, distinct base modules (same-module values bare).
- **`types-union-heterogeneous-members-quoting`** — mixed quoting byte-gate.

### Core API (sufficient — no `cambium-core` change required)
`match info.resolved() { ResolvedType::Union(members) => for m in members { m.base(); m.resolved(); m.typedef_name(); } }`. Members (`schema.rs:645`, extracted `adapter.rs:538-549`) are in declaration order, fully depth-first resolved (`schema.rs:1194-1199`); recurse on members whose `resolved()` is itself `Union`. **This is purely an unimplemented codegen arm — do not add core methods.**

---

## ITEM 2 — FOREIGN-MODULE IDENTITYREF XML (prefix + xmlns synthesis)

### Scope
For an identityref **value** whose chosen identity is defined in a foreign module, libyang's XML synthesizes a short prefix on the value and injects a matching `xmlns:` decl on the carrying element. Verified golden for `types-identityref-foreign-module-prefix`:
```
<component xmlns="urn:types-identityref-foreign-module-prefix" xmlns:tifb="urn:types-identityref-foreign-base">tifb:cpu</component>
```
The synthesized prefix is the **foreign module's own declared prefix** (`prefix tifb;`). JSON already lands (`as_json_name`). Reproduce the XML so the foreign fixture's XML byte-gate passes.

### CRITICAL — the xmlns is RUNTIME-CONDITIONAL on the active variant
The `xmlns:tifb` decl appears ONLY because the active value (`cpu`) is foreign. A **same-module** identity value on the SAME leaf must emit NO `xmlns:` and a bare `as_name()` value. An identityref enum may mix local and foreign variants. Therefore:
- The generated identityref enum must expose a **runtime accessor** returning the optional `(synth_prefix, namespace)` for the ACTIVE variant — `None` for same-module variants, `Some((prefix, ns))` for foreign ones.
- The XML element emitter injects `xmlns:<prefix>="<ns>"` on the value-carrying element **only when that accessor returns `Some`**, and prefixes the value `<prefix>:<identity-name>`; otherwise bare value, no extra xmlns.

### Exact files / functions to touch
- **`collect_identityref_members(bases)`** (`lib.rs:1484-1512`) — currently captures `(module_name, name, variant)` and discards namespace/prefix. Extend to also capture `id.module().namespace()` and `id.module().prefix()` per variant (and the local/foreign flag, computed against `self.module_name`).
- **`ensure_identityref`** (`lib.rs:669-759`) — keep `as_name()` (bare/XML), `as_json_name()` (module-qualified/JSON), `from_name()`. Add the per-active-variant accessor returning `Option<(synth_prefix, namespace)>` and an XML-value method that prefixes the value when foreign.
- **`xml_value_expr`** (`lib.rs:1462-1531`) and the XML element emitters — `emit_top_level_xml` (`:1097-1140`), `emit_field_xml` (`:1054-1095`) — when the value is a (possibly mixed) identityref, query the accessor and inject `xmlns:<prefix>="<ns>"` onto the element conditionally, matching libyang's placement (alongside the default `xmlns`, on the value-carrying element).
- **Determinism (Rust==Go is a CI gate):** derive the prefix purely from the defining module's `prefix()`, never a counter that depends on traversal order.

### Acceptance gate
- **`types-identityref-foreign-module-prefix`** — add the **XML** byte-gate. This is a **2-module fixture** (`types-identityref-foreign-base.yang` + `types-identityref-foreign-module-prefix.yang`). **Do NOT use `byte_gate_fixture_xml_json`** — it routes through single-module `load_ctx_for_dir` and cannot load both modules, so the base identities won't resolve. Instead **extend the existing `codegen_identityref_foreign_module_resolves` test (`codegen_v2.rs:1259`)** which already enumerates+sorts all `.yang` and loads each (`:1267-1285`): add an XML byte-gate beside its existing JSON gate — compute the oracle via `libyang_reference_xml_from_input(&ctx, &input)` and emit a `compile_and_run` wrapper asserting `demo.to_xml() == <golden>`. Construct `demo = TypesIdentityrefForeignModulePrefix { component: Some(TypesIdentityrefForeignModulePrefixComponentEnum::Cpu) }`.

### Core API (sufficient)
`ResolvedType::IdentityRef { bases }` → each base/`derived()` `Identity` → `id.module()` → `.namespace()` (xmlns URI) and `.prefix()` (synth prefix). `Identity::name()` is bare local. All identity-defining modules (incl. import-only base/iana) are loaded since 4050d82. No core change required; if a base module is genuinely unloaded, the existing String-fallback degrade stands.

---

## ITEM 3 — EMPTY NON-PRESENCE CONTAINER OMISSION + SELF-CLOSING EMPTY CONTAINERS

### Scope (two coupled behaviors)
1. A **non-presence** container with no present descendants must be **omitted** from XML and JSON (libyang omits it). Today only `is_presence_container()` exists (`lib.rs:356`); there is no `has_content()` guard, so it is wrongly serialized.
2. A container that IS emitted but is contentless (the present-but-empty **presence** container) must serialize **self-closing** in XML (`<enable-ssh xmlns="..."/>`) and `{}` in JSON. The current XML emitter (`emit_top_container_value` `:1198-1202`, `emit_container_value` `:1188-1196`) UNCONDITIONALLY emits paired open+close tags with no self-closing path — it would emit `<enable-ssh ...>\n</enable-ssh>\n`, diverging from the golden `<enable-ssh ...prefix.../>`. **This DOES change presence-container XML for the empty case** — reconcile: behavior change is *only* the empty-container tag form (self-closing), nothing else about presence semantics.

### Exact files / functions to touch
- Emit a recursive, doc-commented **`has_content(&self) -> bool`** inherent method on each generated container / list-entry struct (true iff some descendant leaf/leaf-list/list/presence-container is present). Add it in `emit_struct_serializer` (`lib.rs:962-1006`) alongside `write_xml`/`write_json`.
- **Omission**: gate emission of each **non-presence** container child on `child.has_content()` in both walks — XML: `emit_top_level_xml` (`:1097-1140`), `emit_field_xml` (`:1054-1095`); JSON: `emit_top_level_json` (`:1204-1334`), `emit_field_json` (`:1204+`).
- **Self-closing**: in `emit_top_container_value`/`emit_container_value`, add an empty-content branch — when the container is emitted but `has_content()` is false, write the self-closing form `<wire .../>` (XML) and `{}` (JSON) instead of a paired open/close. A presence container that is `Some` but empty hits this self-closing branch; a non-presence empty container is dropped entirely by the omission gate.

### HARD cases (fixture roles — get these right; they are inverted in the naive reading)
- **`container-presence-empty`** (verified golden XML `<enable-ssh xmlns="urn:container-presence-empty"/>\n`, JSON `{ "container-presence-empty:enable-ssh": {} }`) — this fixture's input contains ONLY the **presence** container `enable-ssh`; its `non-presence` container is absent from input and never appears. So this fixture's job is to gate the **self-closing present-but-empty PRESENCE container** (`<enable-ssh.../>` / `{}`), NOT non-presence omission. Construct the presence container as `Some(<empty>)` and assert it renders self-closing in XML and `{}` in JSON. It must NOT be dropped by `has_content()`.
- **`json-ietf-presence-vs-nonpresence`** (ships ONLY `output.json_ietf`; verified golden `{ "json-ietf-presence-vs-nonpresence:top": { "ssh": {} } }`) — its module has a present **presence** container `ssh` AND a **non-presence** container `empty-slot`. The golden keeps `ssh` (`{}`) and **omits `empty-slot`**. This is the **PRIMARY non-presence-omission gate**: `has_content()` must drop `empty-slot` (all descendants `None`) while keeping the present `ssh`. Byte-gate **JSON only** (no XML/JSON goldens shipped) via the existing helper / `Format::Json`.

### Instance-literal requirement (so the guard is actually exercised)
A non-presence container maps to a **bare (non-Option)** struct field that is always present with default-empty contents. To exercise omission, the instance literal MUST construct the root with the non-presence child **defaulted (all descendants `None`)** so `has_content()` returns false and the child is OMITTED. To exercise self-closing, construct the presence container as `Some(<empty>)`. Assert both directions.

### Acceptance gates
- **`container-presence-empty`** — XML self-closing `<enable-ssh.../>` + JSON `{}` for the present-but-empty presence container; byte-gate both vs the oracle.
- **`json-ietf-presence-vs-nonpresence`** — JSON-only byte-gate proving `empty-slot` is omitted and `ssh` kept.

---

## ITEM 4 — ENGINE-ROUTED SERIALIZER-ACCEPTANCE GATE (lowest priority; deferrable)

### Honest scope (read carefully)
There is **no deserialization codegen** (`from_xml`/`from_json_ietf` are intentionally absent and must NOT be added). So "construct the typed struct" still means a hand-written instance literal — identical to the standalone gate. Linking `cambium-core` therefore does NOT buy a struct round-trip; it buys ONE new signal: **re-feed the generated `to_xml()` bytes back through `Context::parse` + `DataTree::validate` + `DataTree::serialize` and assert the re-serialized bytes equal the oracle** (a serializer-acceptance / self-consistency check). State this plainly in code comments and the handoff: a true deserialize-INTO-struct round-trip requires new codegen and is out of scope.

### Harness changes
- Follow the detached temp-crate pattern (`temp_crate_dir`, `codegen_v2.rs:24-28`) but ADD a `[dependencies]` section to the temp `Cargo.toml`:
  ```
  [package]
  name = "roundtrip-gen"
  version = "0.0.0"
  edition = "2024"

  [workspace]

  [dependencies]
  cambium-core = { path = "<ABSOLUTE_PATH>/rust/cambium-core" }
  ```
  Compute the absolute path from `env!("CARGO_MANIFEST_DIR")` (= `rust/cambium-codegen`) joined with `../cambium-core`. Keep the empty `[workspace]` table so the temp crate is self-contained but resolves the path dep.
- Build/test with **`cargo test --manifest-path <dir>/Cargo.toml`** — NOT `rustc --test` (which cannot resolve the dep).
- Engine surface (already imported in `codegen_v2.rs`): `cambium_core::{ContextBuilder, ContextFlags, Format, ParseMode, SerializeFlags, ValidateMode}`; `ctx.parse(Format, ParseMode, &[u8])` → `DataTree`; `tree.validate(ValidateMode)`; `tree.serialize(Format, SerializeFlags{ siblings: true, ..default })`.

### Important
- **Do NOT add `from_xml`/`from_json_ietf`/`validate`/`diff` to generated structs** — they would break the standalone `rustc --test` gate used by Items 1–3.
- TDD: red test first (a serializer-acceptance gate that fails until the harness links `cambium-core`).

---

## Stop-and-hand-off guidance (protect Items 1–3 from Item 4)

- Land Items 1, 2, 3 each as their own green commit FIRST. They are independent and self-contained (rustc-only harness).
- **If Item 4's harness work (path-dep temp crate + `cargo test --manifest-path` switch) does not land cleanly green, STOP.** Do NOT half-land it. Leave the green floor at Items 1–3 and write the handoff with the exact step-by-step for Item 4 (the temp-crate `Cargo.toml` shape above, the `env!("CARGO_MANIFEST_DIR")`-based absolute-path computation, the `cargo test --manifest-path` switch, and the precise `Context::parse`/`validate`/`serialize` calls). Item 4 must never block or revert Items 1–3.

---

## OUT OF SCOPE (do not touch)

- Do **NOT** re-raise the already-fixed JSON control-char escaping (003406d), range/length `Default` (93dca52), or identityref base resolution (4050d82). Build on top.
- No NETCONF (`<edit-config>`, clients, transports, device sinks), no gNMI/NETCONF/gRPC sessions, no Terraform-provider emitters.
- No Go this run (Rust only; `generate()` rejects `Lang::Go`). A one-line Go-parity note in the handoff is fine; no Go code.
- Do not add `from_xml`/`from_json_ietf`/`validate`/`diff` to generated structs (engine-routed/deferred).
- Do not touch the frozen `ly_ctx` build-once contract or weaken the hexagonal boundary (no libyang/cgo types in the domain core).
- `dedup_groupings` stays a hard `UnsupportedOption` error.
- Binary / InstanceIdentifier / Empty **standalone** typed newtypes are NOT in scope (remain String fallback) — except a union member that happens to be one (String value inside the union is acceptable; it still serializes as a quoted JSON string). `types-instance-identifier-*` and `types-empty-edge-illegality` are not required gates here.
- Do not regress the trybuild `UserOrderedVec` positional compile-fail tests.

---

## Verification commands (must all stay green before each commit)

```
cargo fmt --all
cargo test -p cambium-codegen --test codegen_v2
cargo clippy --workspace --all-targets -- -D warnings
cargo test --workspace            # conformance MUST stay 193/193 (yanglint oracle)
```

Formatting profile for byte assertions: SHRINK off, 2-space indent, LF, WITH_SIBLINGS. Cosmetic whitespace is not part of any invariant — assert on element/member/entry ORDER only. The libyang XML oracle output is COMPACT single-line per node; JSON uses `Format::Json` 2-space pretty — the in-test oracle helpers reproduce both exactly.

---

## Deliverables / commit discipline

Land each slice as its own commit, each independently green (fmt + clippy + codegen tests + conformance 193/193), Conventional Commits, imperative subject ≤50 chars. **Branch off `main`. NEVER push.**

- [ ] Branch created off `main`.
- [ ] **Item 1 — Union**: red test(s) first; `ResolvedType::Union(members)` arm in `rust_type_for` before the wildcard; `emitted_unions` guard + `ensure_union`; single `is_union` Field flag threaded through `Field`/`collect_fields`/`field_type`; union enum owns `write_json_value`/`write_xml_value` with per-variant quoting + conditional `Default`; all four serializer branch points delegate to the enum when `is_union` (and `xml_value_expr` checks `is_union` BEFORE `is_string_like`); doc comments + zero warnings. All named union fixtures green (TYPE assertion + byte-gate; nested-typedef/typedef-composition rely on the TYPE assertion). Commit: `feat(codegen): emit typed union enums`.
- [ ] **Item 2 — Foreign identityref XML**: red test first; `collect_identityref_members` captures namespace+prefix+local/foreign flag; identityref enum exposes per-active-variant `Option<(prefix, ns)>` accessor; XML emitters inject `xmlns:<prefix>` conditionally + prefixed value; gate via the existing multi-module `codegen_identityref_foreign_module_resolves` test (extend with an XML byte-gate), NOT `byte_gate_fixture_xml_json`. `types-identityref-foreign-module-prefix` XML green. Commit: `feat(codegen): synthesize foreign identityref xmlns`.
- [ ] **Item 3 — Empty non-presence + self-closing**: red test first; recursive `has_content()` emitted; non-presence containers omitted in both walks; self-closing empty-container XML branch added. `json-ietf-presence-vs-nonpresence` (JSON, omission) + `container-presence-empty` (self-closing presence) green. Commit: `feat(codegen): omit empty non-presence containers`.
- [ ] **Item 4 — Engine-routed serializer-acceptance** (only if it lands cleanly): red test first; detached temp crate with `cambium-core` path dep; `cargo test --manifest-path` gate; re-serialize-through-engine equals oracle. Commit: `test(codegen): add engine-routed serializer gate`. If it does not land green, STOP and write the handoff instead — no half-implementation.
- [ ] Full verification suite green after each commit; conformance stays 193/193; coverage did not drop.
- [ ] **Handoff doc** at `docs/handoff-codegen-final.md`: what landed per item (exact symbols/fixtures touched), what was deferred and WHY, concrete next-agent hints (full step-by-step for Item 4 if not reached; a one-line Go-parity note). Supersede prior handoff notes; do not silently edit them.
- [ ] `Cargo.lock` committed; no secrets; no push.

---

## Recommended sequencing

Union first (largest feature, most fixtures), then the empty/self-closing container slice (small, self-contained), then foreign-module identityref XML (multi-module harness, runtime-conditional xmlns), then — only if it lands cleanly green — the engine-routed serializer-acceptance gate. Each slice is red-test-first, golden-backed, and lands green and separately. Do not rush; byte-equality against the libyang/yanglint oracle is the bar.
