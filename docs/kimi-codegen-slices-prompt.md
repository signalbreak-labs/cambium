# Kimi Work Order — Cambium Typed-Struct Codegen, Slices 3–5

You are an autonomous overnight coding agent. This document is your **entire brief**. Read it fully before writing a line. Do not improvise APIs — every type and method you depend on is named and located here. Line numbers are approximate (drift ±5 lines is normal); **stop and report only if a named symbol is absent within ~5 lines of where it is cited**, not on a one-line offset.

---

## 0. What Cambium is, and the one wall you must not cross

Cambium is a **YANG toolkit / SDK library** for Rust (primary) and Go, built on a vendored static libyang. It loads YANG, builds an order-correct schema/data tree, validates, **generates typed structs**, and serializes to ordered XML / JSON_IETF.

### NON-GOALS — hard anti-drift boundary (do not cross, do not "helpfully" add)

Cambium is a **library**. It does **NOT**, and you must **NOT** add:

- NETCONF transport, `<edit-config>` envelopes, NETCONF clients, device sinks/fakes.
- gNMI / gRPC / NETCONF session handling of any kind.
- Terraform provider generation (`terraform-plugin-framework` resources/models).

Those live in a **separate downstream repo** that consumes Cambium. If you find yourself reaching for any of them, you have drifted — stop and re-read this section.

### The ratified serialization decision (a one-way door — already decided, do not relitigate)

Generated typed structs serialize via a **NATIVE Rust/Go serializer** (a self-contained ordered walk over the struct's own fields), **NOT** by marshalling data back through the libyang engine at runtime. Correctness is enforced by a **CI byte-gate**: the bytes your native serializer emits must equal libyang's `LYD_XML` / `LYD_JSON` output **byte-for-byte**. This is ratified in `/spec/api.md` §D-4 and `/docs/sdk-api-design.md` §H, superseding an earlier `to_json_ietf(&self, ctx: &Context)` engine-routed sketch. Native serializer + byte-gate **wins**. Do not route `to_xml` / `to_json_ietf` through a `Context`.

---

## 1. Where you start

- **Repo**: `/Users/recursive/workspace/projects/github/signalbreak-labs/cambium`
- **Branch**: `feat/cambium-sdk-phase1` — already checked out, **HEAD = `f34f1ca`**. Confirm with `git rev-parse HEAD` before you begin.
- **Slices 1–2 are DONE and committed** (the codegen "floor"). You are extending it, not rewriting it. Do not break any existing test.

### The floor you are building on (read these first)

| File | Role |
|---|---|
| `rust/cambium-codegen/src/lib.rs` (~1270 lines) | The v2 emitter. Public API + all emission logic. |
| `rust/cambium-codegen/tests/codegen_v2.rs` (~823 lines) | The compile-gate + byte-gate + clippy-gate harness. **Copy its patterns; do not invent new ones.** |
| `rust/cambium-codegen/tests/codegen_v1.rs` | Legacy tests — must stay green. |
| `rust/cambium-core/src/schema.rs` | The rich schema introspection API you read from (`Module`, `SchemaNodeRef`, `TypeInfo`, `ResolvedType`, `Identity`). |
| `rust/cambium-core/src/list.rs` | `UserOrderedList` — the user-ordered tree contract. You mirror its **forbidden-method philosophy** in Slice 3, NOT its exact signatures (see §4.1). |
| `rust/cambium-core/tests/compile_fail.rs` + `tests/compile-fail/*.rs` | The trybuild compile-fail wiring you copy for Slice 3's I1 guard (one misuse per file). |
| `docs/handoff-codegen-phase5.md` | Deferred-slice spec + known gaps. |
| `/spec/api.md`, `/docs/sdk-api-design.md` | The ratified contract. `/spec/` is canonical; diverging from it fails review. |

### Public API you MUST NOT break (stable v2 surface)

```rust
pub enum Lang { Rust, Go }
pub struct CodegenOpts { pub lang: Lang, pub dedup_groupings: bool, pub with_validate: bool, pub with_path_builder: bool }
pub struct GeneratedModule { pub source: String, pub field_order: BTreeMap<String, Vec<String>> }
pub fn generate(ctx: &Context, module: &str, opts: CodegenOpts) -> Result<GeneratedModule>
pub fn generate_rust(ctx: &Context, module: &str) -> cambium_core::Result<String>  // v1 wrapper
```

Per-struct emitted surface (do not change signatures): `pub fn write_xml(&self, w: &mut String, depth: usize)`, `pub fn write_json(&self, w: &mut String, depth: usize)`, `pub fn to_xml(&self) -> String`, `pub fn to_json_ietf(&self) -> String`, and `pub const FIELD_ORDER: &[&str]`.

**`with_validate`** is currently a **no-op accepted-but-unimplemented flag** — `generate()` (lib.rs ~91) reads it but never acts on it. **Do NOT wire validation emission in Slices 3–4, and do NOT emit standalone constraint validators anywhere** (that drifts toward hand-rolled validation logic the corpus does not byte-gate). In Slice 5 only the engine-routed `CambiumStruct::validate` (through `&Context`) is in scope. Leave `with_validate` untouched.

---

## 2. The gates — every generated artifact passes ALL of these. Do not weaken any.

These are already wired in `codegen_v2.rs`. Reuse the harness functions; do not re-implement.

1. **`#![deny(missing_docs)]`** is **emitted into the generated source header** (by `Emitter::output` at lib.rs ~181), not a crate attribute on cambium-codegen itself. Every public item you emit needs a doc comment.
2. **`#![deny(warnings)]`** is likewise **emitted into the generated source header** (lib.rs ~182). No unused vars, no dead code. (Gotcha: empty container bodies must prefix `_w` / `_depth`.)
3. **Clippy `-D warnings`** on a throwaway crate: `codegen_generated_code_clippy_clean` writes `generated.source` to `target/generated/clippy-crates/<name>/src/lib.rs` and runs `cargo clippy --manifest-path ... -- -D warnings`. Your new shapes must pass this clean.
4. **libyang XML byte-gate**: native `to_xml()` output == libyang `Format::Xml` output, byte-for-byte. Oracle helper: `libyang_reference_xml_from_input` (codegen_v2.rs ~372).
5. **libyang JSON byte-gate**: native `to_json_ietf()`/`write_json` output == libyang `Format::Json` output (use `Format::Json`, **not** `Format::JsonIetf` — the latter forces a phantom `ietf-yang-schema-mount:schema-mounts` node). Oracle helper: `libyang_reference_json_from_input` (codegen_v2.rs ~677).
6. **All existing v1 + v2 tests remain green.**

### Generate FROM the corpus — do not invent modules

The conformance corpus at `conformance/fixtures/<name>/` (with `module/*.yang` + `input.xml`) and goldens at `conformance/golden/<name>/output.{xml,json,json_ietf}` are **yanglint-oracle-verified** (manifest.toml marks 193 cases `oracle = true`). **Reuse them.** For each typed feature, load the matching fixture via `ContextBuilder`, call `generate()`, wrap in the test harness, and byte-gate against the committed golden. The pattern to copy is in `codegen_v2.rs` (the `keys-first` example, ~lines 95–146): load fixture → `generate()` → format generated source + an instantiation expr into a `#[test]` wrapper → `compile_and_run()` (~lines 394–416) → assert `to_xml()` / `to_json_ietf()` equals the escaped golden bytes.

Fixtures confirmed present (use these exact names):
- **Union**: `types-union-enum-and-scalar`, `types-union-heterogeneous-members-quoting`, `types-union-identityref-member`, `types-union-leafref-member`, `types-union-member-resolution-order`, `types-union-nested-typedef-chain`, `types-union-scalar-all-members`, `types-union-two-identityrefs-distinct-bases`
- **IdentityRef**: `types-identityref-single-base`, `types-identityref-multiple-bases`, `types-identityref-derived-hierarchy`, `types-identityref-foreign-module-prefix`
- **Bits**: `types-bits-explicit-positions-gaps`
- **Decimal64**: `types-decimal64-fraction1-range`, `-fraction2-canonical-round`, `-fraction3-and-6`, `-fraction9-negative`, `-fraction18-max-magnitude`
- **LeafRef**: `types-leafref-absolute-path`, `-relative-parent-path`, `-current-context`, `-to-list-key`, `-to-leaf-list`, `-to-leafref-chain`, `-require-instance-false`, `-cross-module`, `-deref-function`
- **User-ordered**: `ordering-nested-user-cascading`

---

## 3. Method — TDD, slice order, and the re-tiered floor

- **TDD is non-negotiable.** For every slice, write the **failing test first** and watch it fail (red) before writing any production code — no production line, not even a stub, ahead of a red test.
- **Beware the String-fallback false-green.** The floor maps `Bits`/`IdentityRef`/`Union` → `String` with `JsonKind::String` (lib.rs ~361–364). For fixtures whose input value already equals the golden (e.g. bits `read delete`, union scalar `5`), the String-fallback native serializer may **already byte-match the golden**, so a pure byte-gate can start GREEN and silently defeat red-first. **Therefore every typed-mapping test must ALSO assert the generated FIELD TYPE** — e.g. that a Bits newtype / identityref enum / union enum appears in the generated source and the field is NOT typed `String`. The test is only truly red until both the type assertion and the byte-gate pass. Anchor your first red on a fixture that cannot false-green: cross-module identityref (`foreign:cpu` input vs synthesized golden) and the bits-reordering fixture start genuinely red.
- **Do the slices/tiers strictly in order.** Each tier must be **fully green** (all six gates + all prior tests) before the next.
- **Commit each green slice/feature separately** with a Conventional Commit (imperative subject ≤50 chars). **DO NOT push.** Leave commits on `feat/cambium-sdk-phase1`.
- Use `/.planning/` for scratch (it is gitignored). Promote durable decisions into `docs/`.

### Re-tiered success ladder — honor this exactly; never trade gate integrity for completion

- **REQUIRED FLOOR (this run must deliver this, fully green and committed):**
  1. **Slice 3** — `UserOrderedVec<T>` + the I1 trybuild compile-fail guard.
  2. **The first two self-contained Slice-4 features: Decimal64 (§5.5) and Bits (§5.3).** These are independent, single-fixture-class, and have no cross-module XML hazard. If you deliver Slice 3 + these two, fully green, the run is a success.
- **STRETCH (land incrementally, one fixture at a time, with named deferrals — never all-or-bust):** the remaining Slice-4 features — IdentityRef-JSON (§5.1), ranged/length newtypes (§5.4), Union (§5.2), LeafRef (§5.6), and the cross-module identityref **XML** byte-gate (§5.1, a known hard gap).
- **STRETCH BEYOND:** Slice 5 (CambiumStruct trait + path-builder gate). **Go v2 is OUT OF SCOPE — see §6.**

If you run out of time/context, that is acceptable — but you must **name every construct you did not generate and why** in your final handoff (no silent skips).

---

## 4. Slice 3 — `UserOrderedVec<T>` + the I1 compile-fail guard

**Goal**: emit an inline **owned** `UserOrderedVec<T>` with a positional-only contract, type user-ordered fields with it, and prove with trybuild that order-agnostic mutation does not compile.

### 4.1 The type (emit inline into generated source)

This is an **owned, self-contained generic** (no `'a`/`'ctx` borrows — codegen users own and mutate their own lists). The owned `T` is the generated **entry struct** (for lists) or the **scalar/typed element** (for leaf-lists). **Do NOT literally mirror `cambium_core::UserOrderedList`'s signatures** — its `get` returns a tree `NodeRef` and its `find_by_key` returns `Option<usize>` over live tree children; neither maps to an owned `Vec<T>`. Mirror only its *forbidden-method philosophy*. Define the owned contract directly:

- **Write (positional-only)**: `insert_first`, `insert_last`, `insert_before(index, …)`, `insert_after(index, …)`, `move_before(what, point)`, `move_after(what, point)`, `remove(index)`.
- **Read-side (owned, exact signatures)**: `len() -> usize`, `is_empty() -> bool`, `get(index) -> Option<&T>`, `iter() -> impl Iterator<Item = &T>`. **`find_by_key` is DROPPED from the required set** (the core's tree-keyed version does not map to owned `T`); emit it only if you also emit a key accessor on the entry struct, and mark it optional in the handoff.
- **FORBIDDEN — must not exist on the type**: no `set`, no `push`, no `IndexMut`, no `Index`/indexing that yields a mutable slot, no `swap`, no `Deref<Target = Vec<T>>`, no public `Vec` field. This forbidden set is load-bearing — keep it verbatim.

Emit `UserOrderedVec<T>` and its `impl` **once** as a helper (same `self.helpers` mechanism the floor uses for the `Decimal64` helper), only when at least one user-ordered field is present.

### 4.2 Field typing

When emitting a list or leaf-list field, read `SchemaNodeRef::ordered_by()` (schema.rs ~904, returns `OrderedBy::User | OrderedBy::System`):
- `OrderedBy::User` → field type `UserOrderedVec<T>`.
- `OrderedBy::System` → field type stays `Vec<T>` (unchanged from the floor).

### 4.3 The I1 compile-fail guard (LOAD-BEARING)

I1 (user-ordered byte-exact, enforced positionally) is enforced at compile time in user code by **trybuild**. trybuild is already a dev-dep in `rust/cambium-codegen/Cargo.toml`; the wiring pattern is `rust/cambium-core/tests/compile_fail.rs` + `tests/compile-fail/*.rs` (the `user_ordered_list_no_set.rs` / `.stderr` pair). Copy it into the **codegen** crate, with these hard requirements:

- Add `rust/cambium-codegen/tests/compile_fail.rs` calling `trybuild::TestCases::new().compile_fail("tests/compile-fail/*.rs")`.
- **One forbidden operation per `.rs` file** (mirror the one-method-per-file core pattern), so each independently asserts non-compilation. trybuild stops at the first error per file, so multiple misuses in one file would under-test.
  - `field[0] = x;` (no `IndexMut`) — expect an indexing/assign error in the **E0608 / E0596** family.
  - `field.push(x);` (no `push`) — expect **E0599** (no method named `push`).
  - `field.swap(0, 1);` (no `swap`) — expect **E0599** (no method named `swap`).
- **Assert the error-code category, not just "does not compile."** When trimming `.stderr`, **delete rustc's version-fragile `help: there is a method with a similar name` suggestion lines** — they break across rustc versions. Capture once with `TRYBUILD=overwrite`, then **review the diff** (do not blind-accept), and trim the suggestion noise.
- **Add a positive control** (a normal `#[test]`, not compile-fail) proving the **positional methods DO compile and run** — e.g. construct a `UserOrderedVec`, `insert_last`, `insert_after`, `move_before`, and assert order. This prevents a `UserOrderedVec` that fails to compile *entirely* from masquerading as a passing compile-fail guard.

### 4.4 Byte-gate

Use `ordering-nested-user-cascading` (nested user-ordered list with a nested user-ordered leaf-list; input has entries `s2, s1` with reordered actions `b, a`). Byte-gate generated XML and JSON against `conformance/golden/ordering-nested-user-cascading/output.{xml,json}`.

### 4.5 Slice 3 red targets

A failing trybuild compile-fail test (each misuse in its own file) + the positive-control compile test + a failing byte-gate on the user-ordered fixture, written **before** any production code. Then make them green. Commit: `feat(codegen): UserOrderedVec<T> + I1 compile guard`.

---

## 5. Slice 4 — typed mappings for the complex YANG types

Each feature gets its **own oracle-backed byte-gate** against its fixture(s) **plus a field-type assertion** (per §3, to defeat String-fallback false-green). Implement in the §3 tier order: **Decimal64 and Bits first (required floor)**, then the rest as stretch. You read constraints from `ResolvedType` (schema.rs ~590–648) and `TypeInfo` (schema.rs ~651–673).

### 5.5 Decimal64 → inline newtype, Display byte-identical to `cambium_core::Decimal64` *(REQUIRED FLOOR — do first)*

- Source: `ResolvedType::Decimal64 { fraction_digits, range }`. `FractionDigits` is 1..=18 (schema.rs ~365), default **1** (not 0) if missing.
- The floor already emits a `DECIMAL64_HELPER` (lib.rs ~1205–1253). Your newtype's `Display` must be **byte-for-byte identical** to `cambium_core::Decimal64` (RFC-7950 canonical form): `whole = raw / 10^fd`, `frac = raw % 10^fd`, pad frac with leading zeros to `fd` width, **strip trailing fractional zeros keeping at least one digit** (`12.3`, not `12.30`), and apply the sign once via `unsigned_abs()` so negatives render `-12.34`, never `--12.34`. v1 had subtle failures here (double-minus, missing trailing-zero strip) — do not hand-author; mirror the core impl exactly. If a range is present, format range bounds at `fraction_digits` precision.
- Fixtures: `types-decimal64-fraction{1,2,3,9,18}-*` (canonical rounding, negatives, max magnitude).

### 5.3 Bits → bitflags-style newtype *(REQUIRED FLOOR — do second)*

- Source: `ResolvedType::Bits(BitsDef)`; `BitsDef.values: Vec<EnumValue>` where `EnumValue.value` is the **bit position** (i64), read from libyang via `cam_bit_position` (`adapter.rs ~498–512`). **Position ≠ array index** — use the position field.
- Emit a newtype (bitflags style). Serialize **set bits, space-separated, in ascending position order**. Declaration order is the input order; ascending-position is the serialization order. (Canonical bits ordering is a "needs human review" item per the handoff — implement ascending-position, byte-gate it, and flag the decision.)
- Fixture: `types-bits-explicit-positions-gaps` (explicit positions with gaps). This fixture's reordering means a String fallback fails the byte-gate — a clean red anchor.

### 5.1 IdentityRef → enum over the sorted derived closure *(STRETCH — JSON gate first; XML is a known hard gap)*

- Source: `ResolvedType::IdentityRef { bases: Vec<Identity> }`. For the variant set, walk the **transitive `Identity::derived()` closure** (schema.rs ~513–538) — DFS with cycle detection (a `visited` set).
- **Determinism (binding, no ambiguity):** `derived()` is **stack-POP (LIFO) traversal — NOT sorted, NOT lexicographic**. Collect the full closure, then **sort variants by fully-qualified identity name (`module:name`) ascending. This sort is the canonical codegen order and MUST be applied identically on any future Go side.** Note for the handoff: enum-**variant** order does NOT affect serialized bytes (the wire value is the selected identity, not the variant list), so the byte-gate is independent of variant order — but determinism + Rust/Go agreement still require the explicit sort. Flag this as a "needs human review" determinism decision.
- **JSON cross-module qualification (tractable — gate this):** when a derived identity belongs to a different module than the leaf, JSON serialization module-qualifies it with the **full module name** (`types-identityref-foreign-base:cpu`); same-module identities are bare. Owning module via `Identity::module()` (schema.rs ~493) / `Identity::name()` (schema.rs ~488).
- **XML cross-module — KNOWN HARD GAP, byte-gate XML and JSON SEPARATELY:** the foreign-module fixture's libyang **XML** golden is `<component ... xmlns:tifb="urn:types-identityref-foreign-base">tifb:cpu</component>`. libyang **synthesizes a short prefix (`tifb`) from the foreign module name and injects a matching `xmlns:tifb` declaration on the element** — it uses neither the input prefix (`foreign`) nor the full module name. Reproducing libyang's prefix-synthesis + xmlns-injection in a self-contained native walk is genuinely hard. **Read the golden to discover libyang's prefix-derivation rule.** If you can reproduce it natively this run, gate XML too. **If not: generate the JSON form, gate JSON only, and EXPLICITLY flag the cross-module identityref XML byte-gate as DEFERRED in the handoff** (a named known-gap, not a silent skip). Do not let an unsatisfiable XML gate block the JSON win or the rest of the run.
- Fixtures: `types-identityref-{single-base, multiple-bases, derived-hierarchy, foreign-module-prefix}` (the last exercises cross-module).

### 5.4 Int{range} / String{length} → range-bounded newtype with fallible constructor *(STRETCH — no serialization-golden fixture)*

- Sources: `ResolvedType::Int { kind, range: Option<Vec<RangeBound>> }` (schema.rs ~592), `ResolvedType::StringType { length, patterns }`. `RangeBound { min, max }` (schema.rs ~457) stores canonical string bounds (numbers, or the `min`/`max` keywords).
- Emit a newtype wrapping the base scalar with a **fallible smart constructor** `pub fn new(v: Base) -> Result<Self, _>` that checks the range/length bounds. Read-side accessor exposes the inner value for serialization.
- **Patterns are read but NOT validated in this slice** (deferred). Preserve them in metadata if convenient; do not emit pattern validators.
- **This feature has NO dedicated serialization-golden fixture** — range/length constraints affect *validation*, not *wire bytes*, and the corpus's range cases (e.g. `constraints-range-length-reject`) are REJECT cases that never serialize. Its gate is therefore: **(a)** a unit test on `new() -> Result` with in-bounds `Ok` and out-of-bounds `Err` values; **(b)** clippy-clean compile; **(c)** a byte-gate proving the newtype serializes **identically to the bare scalar** on an in-bounds value (reuse any int/string fixture). Do not imply a reject-case fixture serves as the byte-gate.

### 5.2 Union → recursive enum of named member variants *(STRETCH)*

- Source: `ResolvedType::Union(Vec<TypeInfo>)` (schema.rs ~645). Emit a discriminated enum with **one named variant per member, in declaration order**. Each member is a full `TypeInfo` that may itself be a union, identityref, bits, leafref, etc. — recurse to emit the member's own type. Decide flatten-vs-nest for nested unions and document the choice.
- Serialization (XML and JSON) of a union-valued leaf must match libyang's chosen member encoding byte-for-byte, including RFC-7951 scalar quoting for the resolved member type.
- Fixtures: the eight `types-union-*` (enum+scalar, heterogeneous quoting, identityref member, leafref member, member-resolution-order, nested typedef chain, all-scalar, two distinct identityref bases). Note the union *member* that resolves to a cross-module identityref inherits the §5.1 XML known-gap — defer XML for that member if needed and flag it.

### 5.6 LeafRef → follow `realtype` recursively to the concrete base *(STRETCH)*

- Source: `ResolvedType::LeafRef { target, realtype, require_instance }` (schema.rs ~635). `realtype` is a fully resolved `Box<TypeInfo>` for the target type, which may itself be a leafref, union, identityref, etc. Follow `realtype` **recursively** until you hit the first concrete scalar/enum base; type/serialize the field as that base. (libyang rejects circular chains at compile time, but still stop at the first non-leafref base to avoid infinite codegen.) `target` is reserved (`None`) — drive off `realtype`. Honor `require_instance` as metadata only.
- Fixtures: the nine `types-leafref-*` (absolute/relative/current-context paths, to-list-key, to-leaf-list, leafref chain, require-instance-false, cross-module, deref).

### 5.7 Slice 4 red targets & commits

For each feature: a failing byte-gate (XML + JSON) **plus** a failing field-type assertion on its fixture(s), written first, then made green under all gates. Commit each feature as it goes green — e.g. `feat(codegen): typed decimal64 + bits newtypes`, then separate commits for identityref, union, leafref, ranged newtypes as each lands. **Required-floor met = Slice 3 + Decimal64 + Bits green and committed.**

---

## 6. Slice 5 — `CambiumStruct` trait + path-builder gate (STRETCH); Go v2 OUT OF SCOPE

Attempt only after the required floor and as much Slice-4 stretch as feasible are green and committed.

### 6.1 The reconciled `CambiumStruct` trait

Emit the trait with the **ratified** shape (native serialize, engine-routed parse/validate/diff):

- **Native, no `Context`**: `to_xml(&self) -> String` and `to_json_ietf(&self) -> String` — self-contained ordered walks, no live `Context` at runtime. CI byte-gates the bytes against libyang `LYD_XML`/`LYD_JSON`.
- **Engine-routed, through `&Context`**: `from_xml`, `from_json_ietf`, `validate`, `diff` — routed through `&Context` / libyang.

This reconciliation is **locked** (a one-way-door ADR): native serialize wins over the superseded `to_json_ietf(&self, ctx: &Context)` sketch. Do not re-introduce a `Context` parameter on the serialize side.

### 6.2 Path-builder

Keep `with_path_builder` **gated** with a documented deferred error (it already gates via `UnsupportedOption`). Path-builder is **not** a red target — leave it deferred, just keep the gate's message clear.

### 6.3 Go v2 — OUT OF SCOPE for this run (ground truth)

**Do NOT attempt Go v2 codegen this run.** The live Go side exposes only the v1 `GenerateGo(ctx *cambium.Context, module string) (string, error)` (`go/codegen/codegen.go:18`) on the **old** `cambium.SchemaNode` / `cambium.LeafType` surface. There is **NO** `Generate(ctx, module, opts)`, **NO** `CodegenOpts`, **NO** `Lang`, and the Go emitter does **NOT** consume `ResolvedType` / `TypeInfo` / `SchemaNodeRef` at all. Building a Go v2 emitter (the opts/Generate/Lang API plus all typed mappings plus a `UserOrderedVec` equivalent) is a from-scratch effort that exceeds this session and is therefore **explicitly excluded**. (The rich Go *schema* API in `go/cambium/schema.go` exists, but the Go *codegen* layer does not consume it — so there is nothing to "mirror" yet.)

### 6.4 Cross-language byte gate — NOT BINDING this run

The "Rust bytes == Go bytes == libyang bytes" cross-language gate (`/spec/api.md` lines ~44–54) **applies only once a Go v2 emitter exists**. Since Go v2 is out of scope (§6.3), **this gate is NOT a red target for this run.** Do not treat a non-existent gate as a blocker. When you build the canonical sort orders (identityref variants, bits) in Rust, **record them in the handoff** so a future Go v2 build can match them — that is the only cross-language obligation this run.

---

## 7. Invariant gates (apply throughout)

- **I1** (user-ordered byte-exact): enforced positionally via `UserOrderedVec<T>` + the trybuild compile-fail guard (Slice 3).
- **I2** (declaration order) / **I3** (keys-first): the field-order manifest is **keys-first, then declaration order**. For list keys use `SchemaNodeRef::list_keys()` (schema.rs ~914, key-statement order) — do **not** iterate children and filter by `is_key()`; key indices can be sparse/out-of-order in the children array.
- **I5** (JSON arrays carry order): leaf-lists/lists serialize in their list order.
- **No silent skips**: if any construct cannot be generated this run, your handoff names it **and why**.

---

## 8. Known gaps you may hit (documented — handle or flag, do not silently diverge)

- **Cross-module identityref XML prefix-synthesis** — the hard gap in §5.1. libyang synthesizes a short prefix + injects `xmlns:`. Reproduce natively if you can; otherwise gate JSON only and flag XML as deferred.
- **JSON control-char escaping**: floor uses Rust `{:?}` quoting, which diverges from libyang for control chars `< 0x20` and DEL (Rust emits `\u{XX}`, libyang `\uXXXX`). If a fixture triggers this, add a `cambium_json_escape` helper; otherwise flag it.
- **Empty non-presence containers**: floor serializes a container with no present descendants; libyang omits it. If a fixture surfaces this, add a recursive `has_content()` guard; otherwise flag it.
- **Enum/type naming clobber**: type names derive from the **disambiguated field ident**, not the raw YANG name (so `foo-bar` and `foo_bar` emit distinct types). Preserve this — do not regress to raw-name-derived type names.
- **String-fallback false-green** (§3): assert field type, not just bytes.
- **"Needs human review" decisions to flag in the handoff**: identityref derived-closure sort order (you applied `module:name` ascending), bits canonical ordering (ascending position vs declaration order), JSON phantom-node boundary (`Format::Json` vs `Format::JsonIetf`), cross-module identityref XML prefix-synthesis, and the `UserOrderedVec<T>` positional-only contract surface.

---

## 9. Definition of done & handoff

**Required floor (must land green, each committed separately, nothing pushed):** Slice 3 + Decimal64 (§5.5) + Bits (§5.3), each fully green under all six gates, all prior v1/v2 tests still green.

**Stretch (incremental, named deferrals allowed):** the remaining Slice-4 features and Slice 5 §6.1–6.2, as far as they go, green and committed. Go v2 (§6.3) is excluded.

**Final handoff** (write to `docs/`, not pushed): which slices/features landed green; every construct **not** generated this run **with the reason** (especially: cross-module identityref XML if deferred, any union member or leafref chain skipped, Go v2 as out-of-scope); the status of each "needs human review" decision and the choice you made; and the exact commit SHAs you created. No silent skips — if a fixture, a member type, or a whole slice was left out, name it and say why.

Begin with `git rev-parse HEAD` (expect `f34f1ca…`), then write the first failing Slice-3 test.
