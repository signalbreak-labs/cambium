# KIMI WORK-ORDER — Cambium Go Codegen Parity (Foundation + Typed Scalars)

You are an autonomous overnight coding agent working in the **Cambium** repo
(`signalbreak-labs/cambium`), a YANG toolkit/SDK for Rust (primary) + Go built on a
vendored static libyang. Your job: bring the **Go typed-struct codegen** (`go/codegen/`)
toward parity with the now-complete, **frozen** **Rust codegen**
(`rust/cambium-codegen/src/lib.rs`, ~2533 lines), replicating it *idiomatically in Go*
with **byte-for-byte identical serializer output**.

**This is a believable FIRST run, not the whole port.** The Rust codegen was built in
~10 byte-gated slices. You will land exactly **two** of them clean — FOUNDATION and
TYPED SCALARS — and DEFER everything harder behind a written handoff. A clean 2-slice run
beats a half-landed 5-slice run.

---

## 1. Objective + WHY

Cambium's headline differentiator over `openconfig/goyang` is **order-correctness**: a
map-based toolkit alphabetizes children and breaks NETCONF; Cambium emits typed structs
whose serializer walks the **compiled YANG declaration order** (keys-first within list
entries), byte-identical to what libyang itself prints. The codegen *is* that feature.

The Rust codegen is **complete and frozen** — typed structs, a per-struct `FIELD_ORDER`
manifest, typed scalars/enums/bits/identityref/union, `UserOrderedVec<T>` for
`ordered-by user`, ordered XML **and** JSON_IETF serializers, gated byte-exact against the
libyang/yanglint oracle. The **Go codegen lags badly** (~305 lines): it consumes the
*deprecated* coarse `SchemaTree` API, emits XML only, collapses every leaf to
`int64/bool/string`, has no JSON, no typed scalars, no enums/bits/union/identityref, no
`has_content`, no empty-container handling, no user-ordered positional type, and — a latent
bug — emits an XML-escape helper it never calls.

**The contract is cross-language byte-identity:**
> **Rust-bytes == golden == Go-bytes**, byte-for-byte, for the same input + serialization
> mode + format; **FIELD_ORDER identical in both languages**. Divergence is a CI failure.
> (spec/api.md:42-54, spec/ordering-invariants.md:86-92.)

There is no separate "Go golden." The committed `conformance/golden/<fixture>/` outputs
(the libyang/yanglint oracle) and the Rust emitter's output are the oracle. Asserting
**Go-bytes == golden** transitively proves **Go == Rust** — *provided* the targeted fixture
is actually gated green in Rust's `codegen_v2.rs` (see Section 5 pre-flight). No direct
Rust↔Go binary diff is needed.

---

## 2. NON-NEGOTIABLES (read twice — these gate the whole run)

1. **TDD, red-first, ALWAYS.** Write the failing Go test that byte-gates generated output
   against the libyang/golden oracle, watch it fail, *then* write the emitter line. No
   production code — not even a stub — ahead of a red test.
2. **A red test must fail on a BYTE MISMATCH, not merely a compile error.** Because the
   harness compiles the generated source, a test for a not-yet-emitted type (e.g.
   `NewFooRange`) first fails to *compile* — that is an insufficient red. The correct
   red-first loop: (a) emit the struct/field so generated code compiles; (b) observe
   `ToXML()`/`ToJSONIETF() != oracle` (the actual byte diff); (c) implement the serializer
   arm until bytes match. A test red only because generated code does not compile does not
   prove the serializer is wrong.
3. **Hexagonal / FFI boundary.** `go/codegen` consumes **only** the public Go cambium
   schema API (`go/cambium`). It imports **zero** `go/internal/libyang`, `import "C"`,
   `unsafe`, or cgo. The **generated** Go must also be self-contained pure Go — stdlib
   only (`fmt`, `strings`, `strconv`, `sort`, `encoding/base64` as needed), **no live
   Context, no libyang at runtime** (spec D-4, spec/api.md:29-33). Confirm with `grep` that
   neither `go/codegen` nor generated output imports `C`/`unsafe`/`internal/libyang`.
4. **Serialization is ONE ordered walk.** Every serializer (XML and JSON) is a single walk
   over the per-struct field-order in declaration order (keys-first for lists). **Never** a
   Go `map`, **never `encoding/json`**, struct reflection, or sorted-key printer for wire
   bytes. `ToXML()`/`ToJSONIETF()` must delegate to the same `writeXML`/`writeJSON` walk —
   never a second code path. The returned string is the exact wire bytes; callers compare
   with `bytes.Equal`, not fuzzy match.
5. **Ordering invariants I1–I6** (spec/ordering-invariants.md) must hold in generated
   output: I2 declaration order; I3 keys-first; I1 user-ordered byte-exact insertion order
   via a positional-only type (DEFERRED here — see Section 6); I4 RPC I/O order; I5 JSON
   arrays carry order + object members in single-printer (declaration) order; I6 gNMI
   atomic ("user-ordered rides as one JSON_IETF array", not transport).
6. **FIELD_ORDER identical to Rust.** Wire names byte-identical (keys-first in key-statement
   order, then schema declaration order). Go idiom is `var <Struct>FieldOrder = []string{...}`;
   Rust is `pub const FIELD_ORDER`. Only the language idiom differs; the wire-name list must
   match exactly.
7. **Generated-Go quality.** `gofmt`-stable (assert `go/format.Source(src) == src`),
   `go vet`-clean, `golangci-lint`-clean, and **godoc on every exported generated symbol**
   (the Go analogue of Rust's `#![deny(missing_docs)]`/`#![deny(warnings)]` + `clippy -D
   warnings` gate). The generated source must compile under the standard Go toolchain.
8. **Determinism.** `GenerateGo` must be byte-stable across runs (sort any map iteration
   before emitting; never iterate a Go `map` in an emission-order-sensitive path).
9. **Exact byte equality in the codegen gate.** Use `bytes.Equal` with NO `TrimRight` for
   codegen XML/JSON gates — the libyang goldens carry a single deterministic trailing
   newline the generated serializer MUST reproduce (Rust pushes exactly one trailing `\n`;
   verified `scrambled-children/output.json` ends with `}\n`). `TrimRight` is only the
   conformance-runner's cross-tool tolerance and would silently pass a missing/extra newline.
10. **No NETCONF, no Terraform, no gNMI/gRPC transport.** Generated typed structs + ordered
    XML / JSON_IETF serializers only. (spec Non-goals.)
11. **Conventional Commits; one logical change per commit; CI green per slice; NEVER push.**
    Imperative subject ≤50 chars. You are on `main` — **branch first** before committing.
12. **No Rust changes** except a genuine shared-spec fix (and only if unavoidable, with its
    own red test, documented). This is a Go-side port; the Rust emitter is frozen reference.

---

## 3. Go idiom, NOT transliterated Rust

Mirror the Rust emitter's *output shape and semantics*, expressed in idiomatic Go, using
the **sanctioned cross-language shape differences** (spec/api.md:42-54). Do not transliterate
Rust syntax:

| Rust | Go |
|---|---|
| `Result<T, E>` | `(T, error)` — bounded-newtype constructors return `error`, never panic |
| `Drop` / RAII | `Close()` |
| `impl Iterator<Item=T>` | `[]T` or `iter.Seq[T]` |
| `Option<T>` | pointer `*T` **or** `(T, ok bool)` — optional leaf ⇒ pointer field, skip-if-nil in the walk |
| consuming `self` | method taking owner, returning `(owner, error)` |
| `SerializeFlags::default()` siblings:true | Go `SerializeFlags{}` is `Siblings:false`; **callers use `DefaultSerializeFlags()`** |
| `i128` range newtype, `r#raw` idents | Go numeric types `int8..uint64`; Go-keyword avoidance, exported PascalCase fields |
| `CambiumStruct` trait | a Go `interface { ToXML() string; ToJSONIETF() string }` implemented by every struct |
| `pub const FIELD_ORDER: &[&str]` | `var <Struct>FieldOrder = []string{...}` |
| `UserOrderedVec<T>` (trybuild compile-fail) | (DEFERRED) a positional-only Go wrapper with only `InsertFirst/Last/Before/After`, `Move*`, `Remove`, `Len`, `Get`, `Iter` — no append, no index-assign |

The **type surface must match in spirit**: one newtype/enum/bits per shape, a bounds-checked
constructor `New<Type>(v) (<Type>, error)`, a bounds-respecting zero/Default. Everything else
(type/method names where semantically shared, ordering, serialized bytes) is 1:1.

**Decide and pin the public signature first** by reading `spec/api.md` (Area H ~96-101 and
the cross-language contract ~42-54):
- Whether `GenerateGo` returns idiomatic `(string, error)` (today) and exposes the manifest
  only as in-source `var <Struct>FieldOrder`, **or** returns a `GeneratedModule`-style struct
  (`Source string; FieldOrder map[string][]string`) mirroring Rust's
  `GeneratedModule { source, field_order: BTreeMap }`.
- Whether a symmetric `CodegenOpts` (lang/dedup/validate/path-builder) is required for parity.
  (`dedup_groupings` is unsupported on **both** sides — it needs lysp grouping provenance
  absent from the compiled lysc IR.)

Record the decision and *why* in `docs/handoff-go-codegen.md`. Default recommendation: keep
`(string, error)` plus the in-source manifest (the sanctioned asymmetry), and add a
`CodegenOpts` only if `spec/api.md` requires symmetric options. Do not over-build.

---

## 4. Grounding — what already exists (do NOT rebuild)

**The rich Go schema API is COMPLETE and mirrors Rust 1:1.** This is the critical de-risk:
Slice 1 is a *rewire onto an existing API*, not new core work. Confirmed in the repo:

- `Context.Schema(module) (Module, error)` — `go/cambium/schema.go:622`.
- `Module`: `Name() / Namespace() / Prefix() / Revision() (string,bool) / IsImplemented() /
  TopLevel() SchemaChildren / Identities() iter.Seq[Identity] / FindPath(string) (SchemaNodeRef,error)`.
- `SchemaNodeRef`: `Name / Kind / Module / Description (string,bool) / Status / Config /
  IsMandatory / IsPresenceContainer / OrderedBy / IsListKey / ListKeys() SchemaChildren /
  MinElements (uint32,bool) / MaxElements (uint32,bool) / LeafType() (TypeInfo,bool) /
  Units (string,bool) / DefaultValue (string,bool) / Children() SchemaChildren`.
- `SchemaChildren`: `Len / IsEmpty / Get(i) (SchemaNodeRef,bool) / Iter() iter.Seq[SchemaNodeRef]`.
- `TypeInfo.Resolved() ResolvedType` (sum-type interface, `go/cambium/schema.go:217-369`).
  Concrete variants (switch on these, exactly as Rust matches `ResolvedType`):
  - `ResolvedInt{ Kind, Range }` (`Kind` = I8..U64; `Range []RangeBound`)
  - `ResolvedDecimal64{ FractionDigits(), Range }`
  - `ResolvedString{ Length, Patterns }` (`Patterns []Pattern` with `Regex/ErrorAppTag/IsInverted`)
  - `ResolvedBinary{ Length }`
  - `ResolvedEnumeration` → `Values() []EnumValue`
  - `ResolvedBits` → `Values() []EnumValue`
  - `ResolvedIdentityRef{ Bases }` → `Bases() []Identity` *(populated — what codegen needs)*
  - `ResolvedLeafRef` → `Realtype() (*TypeInfo, bool)` *(populated)*, `RequireInstance() bool`,
    `Target() (SchemaNodeRef, bool)` *(returns nil today — DO NOT use)*
  - `ResolvedUnion` → `Members() []TypeInfo` (recursive)
  - `ResolvedBoolean{}`, `ResolvedEmpty{}`, `ResolvedInstanceIdentifier{ ... }`, `ResolvedUnknown{}`
- `RangeBound`: `Min()/Max()` canonical strings (`schema.go:206`).
- `Identity`: `Name() / Module() / Bases() / Derived()` (transitive closure).
  `Identity.Module()` exposes `Namespace()`/`Prefix()`.
- `cambium.Decimal64` (`go/cambium/tree.go:19`) already provides RFC-7950 canonical `String()`
  — **reuse it** as the emitted decimal64 field type/formatter; do not write a new canonicalizer.

**Documented gaps (do NOT block on these):** `Identity.Bases()` and `ResolvedLeafRef.Target()`
are empty/nil today by design. The populated paths codegen needs — `ResolvedIdentityRef.Bases()`
and `ResolvedLeafRef.Realtype()` — work. **No new Go schema-API accessor is required for the
in-scope work below.** If you hit a genuine missing accessor, add it to `go/cambium` mirroring
the Rust shape, keep the domain core libyang-free, write its red test first, and note it in the
handoff — but expect not to.

**What `go/codegen` has today** (the v1/fallback-grade emitter you are replacing):
`GenerateGo(ctx, module) (string, error)` driving off `ctx.SchemaTree(module)`; container/list
(`[]XxxEntry`)/leaf/leaf-list structs; the `var <Struct>FieldOrder = []string{...}` manifest
with keys-first via `orderedChildren()` (consults `KeyNames()`/`IsKey()`); an XML-only
`ToXML()`/`writeXML(b, depth)`; `toPascalCase`; `cambiumXMLEscapeText` (emitted **at
codegen.go:44 but never called** — latent bug); `valueString = fmt.Sprint`. Reuse the keys-first
ordering logic and `toPascalCase` skeleton; replace the coarse `SchemaTree` walk and `leafGoType`.

---

## 5. The byte-gate harness (cite + extend — do not reinvent)

The Go codegen byte-gate **already exists** at `go/codegen/codegen_test.go:200`,
`runGeneratedGoTest(t, generatedSrc, testBody)`:
1. writes a temp Go module — `go.mod` (`module generated` / `go 1.25.0`, **no cambium dep**)
   + `gen.go` (the `GenerateGo` output) + **`gen_test.go` hardcoded as exactly**
   `"package generated\n\nimport \"testing\"\n" + testBody`;
2. runs `go test -v ./...` via `exec.Command` with `cmd.Dir = t.TempDir()`;
3. asserts `PASS` in output.

The generated Go is **pure Go (no cgo)** — building/running it needs no `CGO_ENABLED`. Only
the engine layer that *produces the `want` oracle* needs cgo.

**MANDATORY harness changes (the current template will NOT support the JSON/vet/gofmt work):**
- The hardcoded `import "testing"` line means a `testBody` that needs any extra import
  (`strconv`, etc.) cannot add one. **Change `runGeneratedGoTest` to accept the full
  `gen_test.go` contents (or an explicit imports list)** instead of hardcoding the import set.
- **Before** writing `gen.go`, assert gofmt-stability: `go/format.Source(generatedSrc)` must
  equal `generatedSrc` (fail the test otherwise).
- **After** the `go test` run, run `exec.Command("go", "vet", "./...")` with `cmd.Dir = dir`
  and fail on non-zero.

**The `want` oracle.** Resolve the golden path and format from the manifest — never glob the
directory:
- The golden filename + format for each fixture is declared in `conformance/manifest.toml`
  `[case.expected]` (keys `xml` / `json` / `json_ietf`), the **same** lookup
  `go/conformance/runner.go:169` uses via `c.Expected[fmtName]`. There is **no inconsistent
  naming to discover**: `output.json` and `output.json_ietf` are two *declared formats*, not
  two names for one artifact. Where both exist they are byte-identical (verified 180/180); 6
  fixtures (incl. all 6 headline: `scrambled-children`, `keys-first`, `ordered-user`,
  `rpc-order`, `system-list-canonical`, `ietf-interfaces`) are `output.json`-only.
- **Format→manifest-key table (runner.go:265-268):** `xml` → `cambium.FormatXML`;
  `json` → `cambium.FormatJSON`; `json_ietf`/`json-ietf` → `cambium.FormatJSONIETF`.
- **The JSON oracle for the codegen gate uses `cambium.FormatJSON`, matching the frozen Rust
  reference.** Rust's `libyang_reference_json_from_input` (`codegen_v2.rs:752-759`) serializes
  with `Format::Json` and asserts `to_json_ietf() == that Format::Json output`. The Go method
  is named `ToJSONIETF` for API symmetry, but its bytes MUST equal libyang `FormatJSON`,
  exactly as Rust `to_json_ietf` does. **Do NOT silently switch the oracle to
  `FormatJSONIETF`.** (Today the two formats are byte-identical on the corpus, so a slip would
  not immediately fail — but it is a latent divergence from the frozen reference.)

Produce the oracle either by reading the manifest-declared golden file, or inline:
```go
tree, _ := ctx.Parse(cambium.FormatXML, cambium.ParseModeDataOnly, []byte(input)) // []byte, not string
tree.Validate(cambium.ValidateMode{})                                             // same profile the runner uses for the fixture
want, _    := tree.Serialize(cambium.FormatXML,  cambium.DefaultSerializeFlags()) // []byte — XML oracle
wantJSON, _ := tree.Serialize(cambium.FormatJSON, cambium.DefaultSerializeFlags()) // []byte — JSON oracle (FormatJSON, matches Rust)
```
`Serialize` returns `([]byte, error)`; goldens read via `os.ReadFile` are `[]byte` — compare
with `bytes.Equal` (NO `TrimRight` — see Non-negotiable 9). **Always use
`DefaultSerializeFlags()`** (Siblings:true); `SerializeFlags{}` defaults to `Siblings:false`
and will diverge. The existing harness uses `cambium.ValidateMode{}` (codegen_test.go:178);
the oracle must be produced with the **same** validate profile the conformance runner uses for
that fixture, so default-bearing fixtures (e.g. `types-boolean-default-false`) materialize
identically — if a byte mismatch appears on a defaults fixture, check the `ValidateMode`
profile before suspecting the emitter.

**Pre-flight before claiming transitivity:** before relying on "Go==golden ⇒ Go==Rust" for a
fixture, `grep rust/cambium-codegen/tests/codegen_v2.rs` for that fixture name and confirm it is
gated green. If a Go-targeted fixture has **no** Rust `codegen_v2` coverage, the golden alone is
the oracle — note it in the handoff and do not claim Rust==Go for it.

State the transitive-proof chain explicitly in each gated test's doc comment so a reader knows
the oracle provenance.

---

## 6. SCOPE — exactly two must-land slices, then DEFER

Land Slices 1 and 2 byte-clean. **Defer the rest** under the stop-and-hand-off protocol
(Section 9). A clean 2-slice run beats a half-landed 5-slice run.

Rust reference lines below are *navigation aids* into `rust/cambium-codegen/src/lib.rs` — read
the cited spans before porting each arm.

### SLICE 1 — FOUNDATION (rewire + structs + XML + JSON_IETF + manifest) — **must land**

Rewire `GenerateGo` from `ctx.SchemaTree(module)` to `ctx.Schema(module) → Module →
SchemaNodeRef → TypeInfo`. Walk `Module.TopLevel()` then `SchemaNodeRef.Children().Iter()`.
Mirror Rust `Emitter::emit` / `emit_struct_rec` / `collect_fields` (lib.rs:231-258, 1255-1481).

Emit, for the module:
- One struct per container / list (lists → `[]Entry`, entry struct `<Prefix><Name>Entry`) /
  leaf-list (`[]T`) / leaf, plus a synthetic module-root struct owning `MODULE_NS` and
  `MODULE_NAME` consts.
- Per-struct `var <Struct>FieldOrder = []string{...}` (keys-first via the existing
  `orderedChildren` logic, then declaration order). **Identical wire names to Rust.**
- A `CambiumStruct`-equivalent Go interface `{ ToXML() string; ToJSONIETF() string }`
  implemented by every struct (mirror Rust trait lib.rs:236-242 + impl 1415-1420). Both methods
  delegate to the single FIELD_ORDER-ordered `writeXML`/`writeJSON` walk — never a second path.
- **XML** serializer: `writeXML(b *strings.Builder, depth int)` (two-space indent) + root
  `ToXML()` adding `xmlns="<MODULE_NS>"` on the first element of each subtree; lists/leaf-lists
  loop. **Apply `cambiumXMLEscapeText` to every value** (fix the latent un-applied-escape bug).
  XML escaping: only `&` `<` `>` in text (lib.rs cambium_xml_escape_text).
- **JSON_IETF** serializer: `writeJSON(b, depth)` + root `ToJSONIETF()`. Top-level keys
  **module-qualified** (`module:wire`); nested keys **bare wire names**; trailing newline
  (`s.push('\n')` → append `"\n"`). Port `cambiumJSONIndent` (`\n` + depth*2 spaces) and
  `cambiumJSONEscape` **byte-for-byte from libyang's `json_print_string`**: named escapes
  **only** for `"` `\` `\r` `\t`; every other ASCII control incl. `\n` `\b` `\f` DEL ⇒ `\uXXXX`
  **uppercase** hex; multi-byte UTF-8 passes raw (lib.rs:2349-2381, 1742-1811). **Do NOT use
  `encoding/json` for any wire bytes or escaping.** Write the JSON walk **has_content-ready from
  the start** (a guard hook callable when has_content lands in Slice 2 / DEFER) and pick Slice-1
  JSON fixtures with **no empty containers** so `{}` omission is not yet exercised; `{}` omission
  itself lands with `has_content` (DEFER). This avoids a Slice-1-green / later-regression seam.
- Mandatory-vs-optional leaf: optional (`!IsMandatory && !IsListKey`) ⇒ pointer field `*T`,
  skipped in the walk when nil; mandatory/key ⇒ `T` (Rust `Option<T>` lib.rs:361, 1264-1268,
  1426-1442).
- Godoc on every exported symbol (field doc from YANG `Description()` or synthetic
  `"Generated YANG <kind> <name>"`; file banner "Do not edit by hand").

**Objective acceptance gates (red-first, byte-exact via the harness):**
- `codegen_field_order_manifest_keys_first`: `scrambled-children` →
  `var OrderDemoTopFieldOrder = []string{"z", "m", "a"}`; `keys-first` →
  `var KeysFirstDemoServerEntryFieldOrder = []string{"name", "class", "description"}`.
- `codegen_round_trip_bytes_equal_libyang` (XML): generated `ToXML()` `bytes.Equal` golden for
  `scrambled-children`, `keys-first`. Headline: `scrambled-children` reproduces the order a
  map-based toolkit (goyang) would alphabetize.
- JSON gate (`FormatJSON` oracle): generated `ToJSONIETF()` `bytes.Equal`
  `golden/<fx>/output.json` for `scrambled-children`, `keys-first` (also
  `json-ietf-list-array-keys-first`, `json-ietf-module-namespace-qualification` — confirm each
  has a `json` key + Rust codegen_v2 coverage in pre-flight).
- `TestGenerateGoIsDeterministic` stays green; new gofmt-stable + `go vet` harness assertions
  pass on generated source.

### SLICE 2 — TYPED SCALARS (bool + ranged-int + decimal64 + length-string) — **must land**

Per-JsonKind quoting (mirror Rust `JsonKind`, lib.rs:133-144, 460-468): `String` (quoted,
escaped) for strings; `BareNumber` for `int8..int32/uint8..uint32`; `QuotedNumber` for
`int64/uint64/decimal64` (RFC-7951 — emit as quoted string); `Bool` bare `true`/`false`. Switch
on `ResolvedType`:
- **boolean** → `bool`, `JsonKind::Bool` (lib.rs:452-459).
- **ranged int** → bounds-checked newtype `<Prefix><Field>Range` over the exact Go width
  (`int8..uint64` from `ResolvedInt.Kind`), constructor `New<Type>(v) (<Type>, error)`, `Get()`,
  `String()`, multi-range OR-check, **bounds-respecting zero/Default** (min when 0 out of range)
  (lib.rs:460-468, 1082-1167).
- **decimal64** → reuse `cambium.Decimal64` shape: a Go helper carrying `(raw int64,
  fractionDigits uint8)` with canonical RFC-7950 `String()` (trailing-zero trim, ≥1 frac digit,
  sign); `JsonKind::QuotedNumber` (lib.rs:469-479, 2383-2431). Prefer reusing
  `cambium.Decimal64`'s `String()` logic so canonicalization matches exactly.
- **length-bounded string** → newtype `<Prefix><Field>Length` over `string`, constructor
  returning `error`, `String()`, bounds-respecting Default (empty if min=0 else min-length fill)
  (lib.rs:516-523, 1169-1245); no length ⇒ plain `string`.

**Objective acceptance gates** (XML + JSON `bytes.Equal` oracle, after pre-flight confirms Rust
codegen_v2 coverage): `types-boolean-default-false`; `types-int-int8-range`,
`types-int-int16-range`, `types-int-int32-range-multipart`, `types-int-int64-range-quoted`,
`types-uint-uint16-range-port`, `types-uint-uint32-range-multi`, `types-uint-uint64-range-quoted`;
`types-decimal64-fraction1-range`, `types-decimal64-fraction2-canonical-round`,
`types-decimal64-fraction3-and-6`, `types-decimal64-fraction9-negative`,
`types-decimal64-fraction18-max-magnitude`, `json-ietf-decimal64-canonical-quoting`;
`types-string-length-pattern-anchor-posix`, `types-range-length-min-max-keywords`. Plus:
generated constructors return `error` (never panic), and a bounds-respecting Default test
(`codegen_string_length_default_is_valid` analogue: the zero value of every newtype is itself a
valid value).

---

## 6a. Missing Go schema-API accessor (add first, only if a real gap blocks a slice)

No new accessor is expected (Section 4). If — and only if — a slice is genuinely blocked by a
missing `go/cambium` accessor: add it to `go/cambium` mirroring the Rust shape, keep the domain
core **libyang-free** (no `import "C"`/`unsafe` outside `internal/libyang`), write its red test
first, wrap errors with `%w` + a `CAMBIUM_E####` rule code, and document the addition (with
rationale) in the handoff. Do not pre-emptively add accessors.

---

## 7. DEFERRED features — hard stop-and-hand-off (do NOT half-land)

These are the harder, mostly recursive features. **Only land one if it goes cleanly byte-green
within the run.** If a deferred slice does not complete byte-clean, **stop, revert that slice's
incomplete changes** so the suite + conformance 6/6 + Rust workspace are all green, commit the
green state, and write the remaining work into `docs/handoff-go-codegen.md` as a ready-to-resume
red-test plan (Rust reference spans, target fixtures, the precise next red test).

- **`has_content` + empty-container handling** — `has_content()` recursion (lib.rs:1335-1374):
  mandatory leaf ⇒ true; optional leaf / optional container / presence container ⇒ present
  (non-nil); non-presence container ⇒ recurse `child.hasContent()`; list/leaf-list ⇒ `len > 0`.
  Drives self-closing `<x/>` for empty containers (XML, lib.rs:1649-1675) and `{}` omission for
  empty JSON objects (lib.rs:1300-1318). `IsPresenceContainer()` is on the rich API. This is a
  known byte-divergence in current Go (always emits open/close) and is the natural next slice —
  land it if clean. Gates: `codegen_jsonietf_string_escaping_byte_gate`
  (`json-ietf-string-escaping-control-unicode` — also hard-gates `cambiumJSONEscape`),
  `container-presence-empty`, `json-ietf-presence-vs-nonpresence`.
- **enum + bits + identifier safety** — enum type with sparse explicit declaration-order
  discriminants, `AsName`/`FromName`/`String`, type name `<Prefix><Field>Enum` from the
  **disambiguated** field ident (lib.rs:480-488, 577-637); bits newtype with declaration-order
  `BIT_POSITIONS`, `New([]string)(T,error)`, `String()` sorting by **ascending position**
  (lib.rs:489-493, 639-744); `safe_field_ident` port (lib.rs:2180-2213): digit-leading fix,
  hyphen/dot→underscore, **Go keyword** avoidance, collision disambiguation (`foo-bar` vs
  `foo_bar` → `FooBar`/`FooBar2`), helper TYPE names from the disambiguated ident (lib.rs:480-486).
  Fixtures: `types-enumeration-explicit-values-sparse`, `types-enum-bits-auto-position`,
  `types-enumeration-zero-value-disabled`, `idents-enum-value-collision`,
  `types-bits-explicit-positions-gaps`, `idents-collision-hyphen-underscore`, `idents-keywords-go`,
  `idents-container-leaf-collision`, `idents-long-name`, `idents-unicode-mixed-case`.
- **leafref realtype chasing** — chase `ResolvedLeafRef.Realtype()` **only**, recursively, before
  mapping; if `Realtype()` is unset, degrade to `string`. **`ResolvedLeafRef.Target()` returns nil
  today (documented gap) — do NOT write a `Target()`-based path; it cannot be tested**
  (lib.rs:423-451). Fixtures: `types-leafref-to-list-key`, `types-leafref-to-leafref-chain`,
  `types-leafref-cross-module`.
- **ordered-by user positional type** — the `UserOrderedVec<T>` analogue (lib.rs:2433-2517) for
  `ordered-by user` lists/leaf-lists (branch on `OrderedBy() == OrderedByUser`); system-ordered
  stays `[]T`. Expose only positional mutators (`InsertFirst/Last/Before/After`, `Move*`, `Remove`,
  `Len`, `IsEmpty`, `Get`, `Iter`, `From([]T)`) — no append, no index-assign. Go has no trybuild
  compile-fail; enforce via the API surface + a test asserting the type has no order-agnostic
  setter (mirror the `UserOrderedList`/`GO-001` runtime guard in `go/cambium`). Fixtures:
  `leaflist-ordered-by-user`, `list-ordered-by-user-insertion`, `ordered-user`,
  `ordering-nested-user-cascading`, `system-list-canonical`, `json-ietf-leaflist-array-user-system`.
- **identityref incl. foreign-module XML prefix/xmlns** — enum with
  `AsName`/`AsJSONName`(`module:name` foreign)/`FromName`/`xmlPrefixNS`/`writeXMLValue`; base +
  transitive `Derived()` closure sorted by `module:name`; synthesize
  `<wire xmlns:PFX="NS">PFX:name</wire>` for foreign identities; degrade to `string` on no base
  (lib.rs:494-515, 746-876, 1592-1647, 1994-2024). Fixtures: `types-identityref-single-base`,
  `-multiple-bases`, `-derived-hierarchy`, `-foreign-module-prefix`, `identityref-iana-if-type-foreign`.
- **typed unions** — discriminated type owning `WriteJSONValue`/`WriteXMLValue` with per-member
  quoting, delegating to nested enum/bits/identityref helpers; member naming via typedef name or
  base-type label; empty-union fallback (lib.rs:544-548, 878-1080, 2048-2068). Fixtures:
  `types-union-enum-and-scalar`, `-heterogeneous-members-quoting`, `-identityref-member`,
  `-leafref-member`, `-scalar-all-members`, `-two-identityrefs-distinct-bases`,
  `json-ietf-leafref-union-resolved-form`.

---

## 8. Out of scope

- Wiring `Lang::Go` into the Rust `generate()` entry point — Rust `generate()` rejects `Lang::Go`
  by design; Go codegen is a **separate Go-native emitter** in `go/codegen`. Do not touch the Rust
  crate to "enable Go."
- `dedup_groupings`, `with_path_builder` — unsupported on both sides (need lysp provenance).
- Any NETCONF (`edit-config`, transport, clients), Terraform-provider emitters, or gNMI/gRPC
  sessions. These belong to a downstream consumer repo, never Cambium.
- Rust emitter changes (frozen reference). Touch Rust only for a genuine shared-spec fix, with its
  own red test, documented.
- New `go/cambium` core accessors — expected unnecessary; add only if a real gap blocks a slice,
  libyang-free, red-test-first, noted in the handoff (Section 6a).

---

## 9. Verification commands (run before claiming any slice done; full sweep before final)

```bash
# Go (codegen package + generated-code gates) — from repo root:
cd go && CGO_ENABLED=1 go build ./...
cd go && CGO_ENABLED=1 go vet ./...
cd go && CGO_ENABLED=1 go test -race ./...
cd go && golangci-lint run
cd go && gofmt -l .                      # must print nothing (incl. the codegen pkg)

# Conformance floor (libyang oracle byte-parity) — must stay green:
cd go && go run ./cmd/cambium all        # exits non-zero on any byte mismatch; headline 6/6 must hold

# Rust must NOT regress (parity proven transitively):
cargo test --workspace
cargo clippy --workspace --all-targets -- -D warnings
```

A slice is **done** only when: its red tests now pass byte-clean (XML and, where the fixture has
a `json` key, JSON, via `bytes.Equal`); `go vet`/`golangci-lint`/`gofmt -l` clean on `go/codegen`;
the harness asserts the generated source is gofmt-stable + vet-clean; determinism test green;
conformance headline 6/6 still byte-identical; and `cargo test --workspace` + `cargo clippy -D
warnings` stay green.

---

## 10. Deliverables, commit discipline & STOP-AND-HAND-OFF

**Branch first** (you start on `main`): e.g. `feat/go-codegen-parity`. Conventional Commits, one
logical change per commit, CI green per slice. **Never push.**

**Per-slice commit checklist:**
- [ ] Red test written and observed failing on a **byte mismatch** (not a bare compile error)
      before the serializer arm.
- [ ] Generated XML (and JSON where the fixture declares a `json` key) `bytes.Equal` the
      libyang/golden oracle for the slice's named fixtures (no `TrimRight`).
- [ ] Pre-flight done: each gated fixture confirmed green in Rust `codegen_v2.rs`, or noted in
      handoff as golden-only.
- [ ] FIELD_ORDER wire names identical to Rust for every gated fixture.
- [ ] Generated source: compiles, gofmt-stable, `go vet`/`golangci-lint` clean, godoc on exported
      symbols, stdlib-only (no `encoding/json` for wire bytes, no cgo/`C`/`unsafe`/`internal/libyang`).
- [ ] `go/codegen` package itself: gofmt/goimports/vet/golangci-lint clean; errors wrapped with
      `%w` + `errors.Is/As` (no string-matching); exported symbols documented.
- [ ] Determinism test green; conformance 6/6 green; `cargo test --workspace` + `clippy -D warnings`
      green (no Rust regression).
- [ ] Conventional Commit, imperative subject ≤50 chars, one logical change.

**Write `docs/handoff-go-codegen.md`** capturing: the public-signature decision and *why* (per
`spec/api.md`); each slice landed (green) with its gated fixtures; the exact state of every
DEFERRED feature — what's done, what remains, the Rust reference spans (lib.rs line ranges), the
target fixtures, and the precise next red test to write; any new `go/cambium` accessor added (with
rationale + confirmation the core stays libyang-free); any Go-targeted fixture lacking Rust
codegen_v2 coverage (golden-only); the manifest-driven golden resolution (`[case.expected]` →
`FormatXML`/`FormatJSON`/`FormatJSONIETF`) and the `FormatJSON`-as-JSON-oracle decision; and the
verified floor (commands + results) so the next agent can resume without re-deriving context.

**STOP-AND-HAND-OFF (protect the foundation):**
- The foundation (Slices 1–2) is the value of this run. **Never** sacrifice a green, byte-clean
  foundation to chase a deferred hard feature.
- If a DEFERRED feature does not reach byte-clean green: **revert that slice's incomplete changes**
  so the test suite, conformance 6/6, and Rust workspace are all green, **commit the green state**,
  and write the remaining work into `docs/handoff-go-codegen.md` as a ready-to-resume red-test plan.
  A half-landed emitter that breaks the build or diverges from the oracle is **worse** than a clean
  handoff.
- Leave the tree green and `gofmt`-clean. Do not push. Do not leave the suite red.
