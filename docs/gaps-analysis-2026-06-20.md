# Cambium — Gap Analysis (2026-06-20)

> Independent deep-dive audit of the `feat/pure-go-rebuild-spec` branch, prompted by a
> "project is complete" claim. This document records what Cambium **is**, what it
> **actually does today**, and every **gap** between the shipped code and the
> project's own contract (`/spec`), design brief (`docs/cambium-kickoff.md`), and
> onboarding docs.

> **Status update (2026-06-20): Rust stack removed; Cambium is now Go-only.**
> As a result, the Rust-specific and Rust↔Go-parity gaps in this document are now
> **MOOT (resolved by removal)**: G-02, G-03, G-05, G-10, G-13, G-16, G-17, G-18, and
> G-11 (cross-language parity).
> The remaining **LIVE** gaps are all Go-side: G-06 (pure-Go vs full-SDK doc framing —
> partly addressed), G-07 (gNMI/I6 fixture — deferred in spec), G-08 (manifest invariants
> mapping), G-09 (codegen tier in shared runner), G-14 (Go validation sub-code
> wording-pinning), G-19 (Go `FindByKey` duplicate-key guard), G-22 (goyang compat
> snapshot drift). G-01 (CI/billing) is **deferred** by the maintainer (private repo).
> The body below is preserved as the original audit for historical reference.

## 0. How this was produced

- **Local gate re-run** — every gate the release docs claim was executed from a clean
  tree (results in §2). This answers "does the code work": where it is exercised, it
  does.
- **Multi-agent code audit** — 7 parallel deep-readers mapped each subsystem (Rust core,
  Rust codegen/FFI, pure-Go default, Go codegen, Go compat/backend, tests/conformance)
  and enumerated every promise in `/spec` + design docs. A synthesis pass cross-referenced
  promise-vs-implementation into 22 candidate gaps. Each candidate was then handed to an
  **adversarial verifier** instructed to *refute* it against the source. 19 survived
  refutation; **3 were refuted and are listed in §6 so they are not re-investigated.**
- Every gap below carries file:line evidence and a severity **corrected by the verifier**
  (not the first-pass guess).

**Second-pass review corrections folded in (2026-06-20):** this pass re-checked the surviving
claims against source and corrected the manifest split to **7 schema-ir + 193
backend-data** cases, narrowed the Rust-codegen test-coverage wording, noted that Go
codegen does consume many shared fixture/golden files outside the canonical manifest, and
reclassified G-14/G-19 as cross-backend/cross-language rather than single-stack issues.

## 1. What Cambium is (one screen)

An order-correct YANG toolkit/SDK, successor to openconfig/goyang, in two language stacks
that share one contract (`/spec`), one conformance corpus (`/conformance`, 200 manifest
cases), and one engine pin (`/VERSIONS`: libyang v5.4.9 + PCRE2 10.44, vendored static).

| Stack | Surface | Engine | Tier |
|---|---|---|---|
| **Rust** (`cambium-core`, `cambium`, `cambium-codegen`, `cambium-libyang-sys`, `cambium-cli`) | full SDK: schema IR, ordered data tree, parse/validate/serialize/diff/merge, codegen | vendored **libyang** via coarse FFI | Schema-IR **+** Backend/data |
| **Go default** (`go/cambium`, `go/codegen`, `go/compat`) | schema IR, introspection, schema-level static validation, typed-struct codegen with native serializers | **pure Go, cgo-free** | Schema-IR **only** |
| **Go backend** (`go/libyangbackend`, `go/internal/libyang`) | data tree: parse/validate/serialize/diff/merge/LYB | vendored **libyang** via cgo | Backend/data (optional) |

The headline guarantee — order as a *structural* property (effective schema declaration
order for children; byte-exact insertion order for `ordered-by user`; keys-first; RPC/action
I/O in schema order; canonical system order) — is real and golden-tested for the
**libyang-backed** paths. The positional-only `ordered-by user` type is genuinely enforced
at compile time in both languages (trybuild / Go build-tag tests), which is the project's
strongest single piece of engineering.

## 2. Verification evidence (all local gates green)

Run from a clean checkout on this host:

| Gate | Result |
|---|---|
| `cargo build --workspace` | OK |
| `cargo test --workspace` (incl. conformance-runner) | **200 conformance cases pass**; all unit/golden green |
| `cargo clippy --workspace --all-targets -- -D warnings` | clean |
| Go default `CGO_ENABLED=0 go vet ./cambium ./codegen ./compat` | clean |
| Go default `CGO_ENABLED=0 go test ./cambium ./codegen ./compat` | pass (codegen suite ~115s) |
| Go default import-closure purity | no cgo/libyang in closure |
| `bash go/internal/libyang/build.sh` (vendored static engine) | builds `libyang.a` |
| Go backend `CGO_ENABLED=1 go run ./cmd/cambium all` | **193 conformance pass, 0 fail** |
| `golangci-lint run` | **0 issues** |

**Conclusion on the "complete" claim:** the code that exists is real and passes its own
suite locally. "Complete" is **not** false-on-its-face — it is **over-stated**. The gaps
are concentrated in (a) the Rust codegen, which silently loses whole node classes; (b)
release engineering that was *asserted green but never actually ran in CI*; and (c)
scope/parity/coverage seams the suite cannot see. None of the local gates exercises the
Rust codegen emitter or any CGO_ENABLED=0 *binary*, so the green bar does not cover the
biggest gap.

## 3. Gap register (by corrected severity)

| ID | Severity | Area | Gap |
|---|---|---|---|
| G-02 | **Critical** | rust-codegen | Rust codegen silently drops choice/case-nested leaves, RPC/action/notification I/O, and anydata/anyxml — order-correctness (I4) violation, invisible to the suite |
| G-01 | **High** | release | CI has **never run** on this branch (GitHub billing blocks job start); every "release-green" claim is local-only |
| G-03 | **High** | rust-codegen | Rust codegen is serialize-only: no generated parser, `Validate()`, `WithDefaults`, or RFC-7952 metadata — materially behind Go, inverting "Rust primary" |
| G-05 | **High** | rust-core | `cambium` facade omits `UserOrderedList`/`…View`/`ValidationErrors`/`Diagnostic` — the I1 mechanism and structured diagnostics aren't reachable without dropping to `cambium-core` |
| G-13 | **Medium** | rust-core | Documented `Context: Send + Sync` guarantee is **actually violated** (raw `*const c_void` in `SchemaForest`) and unguarded by any test |
| G-10 | **Medium** | rust-codegen | `bindgen` CLI absence silently falls back to committed bindings → stale-layout UB risk on a libyang SHA bump |
| G-08 | **Medium** | tests | Manifest lacks the spec-prescribed `invariants` key; I1–I6→fixture mapping is prose-only and not machine-checkable; spec §7 fixture names are stale |
| G-09 | **Medium** | tests | No canonical shared-manifest codegen tier; Go codegen has broad package-local fixture/golden coverage, but conformance runners cannot see codegen regressions and Rust codegen misses choice/ops/anydata coverage |
| G-06 | **Medium** | go-pure | No pure-Go data-tier (parse/validate/serialize); onboarding docs still present `t.Validate`/`t.Serialize` as core API |
| G-12 | **Medium** | docs | Catalog counts (189/175) diverge from the real 200-case manifest; "release readiness/green bar" docs committed against unrun CI |
| G-04 | **Medium** | cli | `cambium-cli` is a `println!` stub; no `generate`/`parse`/`validate` CLI in either language |
| G-07 | **Low** | spec | I6/gNMI implemented + unit-tested, but `gnmi-ordered-atomic` golden fixture is absent while spec lists it *required* with no deferral marker |
| G-11 | **Low** | tests | Cross-language parity is per-runner-vs-golden; no direct live Go-output-vs-Rust-output diff, though spec §6 mandates a shared-property cross-check |
| G-14 | **Low** | cross-backend | `When`/`Mandatory` validation sub-codes classified by English message-prefix matching in Rust and Go — brittle to libyang wording/locale |
| G-16 | **Low** | cross-cutting | Inert options advertise absent behavior: `with_validate`/`with_path_builder` ignored (`dedup_groupings` hard-fails; `MergeOpts.destruct` no-op — both self-documented) |
| G-17 | **Low** | rust-core | Dead `Error::InvalidPath`; un-`#[deprecated]` legacy `Context::new`/`SchemaTree` dual path; 11 core + 2 codegen tests still on the legacy path |
| G-18 | **Low** | codegen | No `FromXML` (by design, JSON_IETF-only); Rust drops `Empty`/`Binary`→`String`, losing type fidelity Go keeps |
| G-19 | **Low** | cross-language | `UserOrdered*.find_by_key` / `FindByKey` lack an exact-key/duplicate-query-key guard; adversarial composite keys untested |
| G-22 | **Low** | go-compat | goyang parity is diffed against the vendored v1.6.3 fork snapshot only; no periodic re-diff vs upstream HEAD |

## 4. Detailed findings

### Theme A — Rust codegen is the real weak spot (the "primary" stack trails)

**G-02 — Rust codegen silently drops whole node classes. [Critical]**
`emit_struct_rec` recurses only `Container`/`List`, and the codegen-local `is_data_node`
matches only `Container`/`List`/`Leaf`/`LeafList`. `Choice`/`Case`/`AnyData`/`AnyXml`/
`Rpc`/`Action`/`Notification` are excluded. The core already exposes
`data_children(flatten_choices=true)` (`cambium-core/src/schema.rs:1480`) and
`rpcs()`/`actions()`/`notifications()` (`schema.rs:1137`), but codegen never calls them.
- A leaf nested in a `choice`/`case` is **omitted entirely** from generated structs,
  serializers, and `FIELD_ORDER`.
- RPC/action/notification produce **no** I/O document structs — a direct violation of
  normative **I4** (`spec/ordering-invariants.md:114`, H-CODEGEN `spec/api.md:940`).
- `anydata`/`anyxml` produce no field (`spec/api.md:937`).
Evidence: `rust/cambium-codegen/src/lib.rs:259, 278-281, 2306-2321`; root walk
`lib.rs:231` via `top_level()` (`flatten_choices=false`, `schema.rs:1127`).
Important entrypoint split: `generate()` is the v2 emitter, but `generate_rust()` still
delegates to fallback v1; both call paths need coverage or one needs an explicit
deprecation/removal decision.
**Why it's invisible:** the conformance-runner exercises only the libyang serialize path;
Rust codegen v2 tests cover many scalar/type/user-order golden-backed shapes, but still no
`choice`/`case`, `rpc`/`action`/`notification`, or `anydata`/`anyxml` codegen cases (the
v1 tests cover only the older keys/scrambled/demo path). **Go codegen handles all of
these** (`go/codegen/codegen.go`: `IsChoiceDescendant`, operation docs), so this is
Rust-specific.
**Fix:** flatten choices via the existing core API and emit operation documents; wire the
existing choice/rpc/anydata fixtures into a Rust codegen test.

**G-03 — Rust codegen is serialize-only and behind Go. [High]**
The generated `CambiumStruct` impl exposes only `to_xml()`/`to_json_ietf()`. No generated
`FromJSONIETF` parser, no `Validate()`, no `WithDefaults` modes, no RFC-7952 metadata. Go
emits all of them (`FromJSONIETF` `codegen.go:4166`, `Validate()` `:3109`,
`WithDefaultsMode {Explicit,Trim,All,AllTagged}` `:1549`, `CambiumMetadata` `:1633`,
operation docs `:4196`). `docs/sdk-api-design.md:486-492,659` specs the richer Rust trait
(`to/from_data_tree`, `from_json_ietf`, `validate`, `diff`) that the emitter does not
implement. *Note:* Rust **does** enforce int-range/string-length at construction via
generated newtype constructors (`lib.rs:1153-1320`) — the missing pieces are the aggregate
`Validate()`, parser, `WithDefaults`, metadata, and operation-doc parsers.
**Fix:** close the Rust codegen parity gap, **or** re-scope H-CODEGEN-GO-SERDE as Go-only
in `/spec` so the asymmetry is contractual rather than silent.

**G-16 / G-18 (Rust side).** `with_validate`/`with_path_builder` are accepted but never
read — the only genuinely *uncaveated* inert options (`dedup_groupings` and `*.destruct`
are honestly self-documented as unsupported). Rust maps `Empty`/`Binary`/`Unknown`→`String`
where Go uses `struct{}` and base64-validated `string`.

### Theme B — Release engineering asserted but unproven

**G-01 — CI has never executed. [High]**
The entire CI history is three runs (`27871319283/167429/085858`), all `failure` in 2–5s,
every job annotated *"The job was not started because recent account payments have failed
or your spending limit needs to be increased."* `gh run view --log-failed` →
`log not found` (jobs never started). The workflow itself is real and comprehensive
(`.github/workflows/ci.yml`: rust-core on macOS, go-parity, go-linux-glibc, go-linux-musl
cross-build). So `cargo fmt/clippy/test`, the Rust conformance-runner, the Go pure gate,
cgo race tests, golangci-lint, the Go conformance runner, **and cross-platform/musl
proof** have never run on a runner. Commits `0fa51bd`/`c280e86`/`83e9b0d` assert
release-green; only `scripts/green-bar.sh` (single-host, local) backs them — and the
project's own `docs/release-readiness-2026-06-20.md:44` already flags this as a blocker.
**Fix:** resolve billing, re-trigger the workflow, keep the PR a draft and treat every
green-bar/readiness doc as provisional until a passing run exists. **Do not tag a release
on unrun CI.** *(Also: no Windows job; the Rust FFI build used committed pre-generated
bindings because the `bindgen` CLI was absent on this host — see G-10.)*

**G-10 — Silent stale-binding fallback → UB risk. [Medium]**
`rust/cambium-libyang-sys/build.rs:209` copies committed `src/bindings.rs` on `bindgen`
`NotFound`, emitting only a `cargo:warning`. `bindgen` is not a build-dependency. If
`/VERSIONS` bumps the libyang SHA on a host without `bindgen`, new C compiles against stale
Rust struct layouts; the adapter walks libyang structs by raw pointer deref (~205 sites),
so a layout mismatch is **undefined behavior, not a clean error**. No freshness assertion
ties `bindings.rs` to the pinned SHA. **Fix:** hard-fail (or assert binding-vs-header
freshness) when `bindgen` is unavailable instead of silently using possibly-stale layouts.

**G-12 — Doc counts / readiness drift. [Medium]**
`docs/conformance-coverage-catalog.md` says "189 fixtures / 175 de-duplicated / 193
manifest cases"; the real `conformance/manifest.toml` has **200** cases (7 schema-ir + 193
backend-data). Release-readiness/green-bar docs were committed against the unrun CI of
G-01. **Fix:** regenerate catalog counts from the manifest; gate "readiness" docs on a
passing CI run.

### Theme C — Public API surface incomplete

**G-05 — Facade hides the headline types. [High]**
`rust/cambium/src/lib.rs:11-29` re-exports a curated list that **omits** `UserOrderedList`,
`UserOrderedLeafList`, `UserOrderedView` (the I1 mechanism), and `ValidationErrors`,
`Diagnostic`, `ErrorType`, `ValidationCode` (the structured-diagnostics layer not present in
ygot), plus `LeafType`/legacy `SchemaTree`. A downstream crate on the facade can *call*
`user_ordered_list_at` but cannot **name** its return type or pattern-match a `Diagnostic`
without adding a direct `cambium-core` dependency — defeating the facade. (`SchemaNode`/
`SchemaNodeKind` *are* exported; the claim about those was corrected.) **Fix:** re-export
the `list` and `error` types from the facade and prelude.

**G-04 — No usable CLI. [Medium]**
`rust/cambium-cli/src/main.rs` is `fn main(){ println!("cambium — not yet implemented"); }`
(a versioned 0.1.0 workspace member named as a deliverable in the kickoff).
`go/cmd/cambium` is a `//go:build cgo` conformance runner, not a `generate`/`validate`
command. `GenerateGo`/`generate()` are library-only — no end user can invoke codegen from a
shell. *Severity moderated:* the kickoff roadmap files `cambium generate`/`convert` under
"Later (deliberately deferred)," so this is doc-vs-reality + an unwired UX surface, not a
missing core capability. **Fix:** implement a real CLI **or** drop `cambium-cli` from
shipped-deliverable claims.

**G-16 — Inert options.** See Theme A.

### Theme D — Conformance / test architecture cannot see its own gaps

**G-08 — Invariant→fixture mapping is not machine-checkable. [Medium]**
`spec/ordering-invariants.md:175` prescribes each manifest case carry an
`invariants:[I2,I3]` key; the real manifest has **none** (both runner `Case` structs omit
it). So no CI assertion proves every I1–I6 has a passing fixture. Spec §7's required-fixture
*names* are also stale (`rpc-io-order`, `ordered-user-list`, `nested-ordered-user`,
`keyless-list` were renamed; `gnmi-ordered-atomic` is genuinely absent). **Fix:** add the
`invariants` field + a CI check that every invariant has a passing case; sync §7 names.

**G-09 — No shared-manifest codegen tier. [Medium]**
The canonical Rust/Go conformance runners drive only `schema-ir` and `backend-data`. Go
codegen has broad package-local tests that read many shared fixture/golden files directly,
and Rust codegen has narrower package-local golden gates, but neither flows through a
manifest `codegen` tier. So codegen coverage is not shared, not exhaustively mapped to
I1–I6, and the canonical runner cannot catch G-02-class regressions. Rust codegen
specifically has no `choice`/`rpc`/`action`/`notification`/`anydata`/`anyxml` coverage.
**Fix:** add a codegen tier to the shared runner (compile generated structs → serialize →
byte-compare goldens).

**G-11 — No direct cross-language diff. [Low]**
Both languages assert independently against the same goldens; `scripts/conformance-tool.py`
only OR-combines exit codes. Spec §6 (`ordering-invariants.md:183,191`) mandates a
shared-property Rust↔Go / backend-differential cross-check that does not exist. Identical
drift against a stale golden would pass. **Fix:** add a differential harness.

**G-07 — gNMI fixture bookkeeping. [Low]**
I6 *is* implemented and unit-tested in both languages (`Format::JsonIetf` as the gNMI
content layer; `DataDiff::is_ordered_by_user` yields one atomic edit, tested in
`tests/diff.rs` / `diff_test.go`). But the `gnmi-ordered-atomic` golden fixture is absent
while `spec/ordering-invariants.md:209` lists it *required* with **no deferral marker**,
whereas `docs/conformance-coverage-catalog.md` openly documents the Block-2 deferral.
**Fix:** mark I6/`gnmi-ordered-atomic` explicitly deferred in the normative spec (or add
the fixture). A bookkeeping divergence, not a behavioral gap.

### Theme E — Scope & doc clarity

**G-06 — Pure-Go default ≠ full SDK; onboarding docs blur it. [Medium]**
By design (`spec/ordering-invariants.md:22-31,148-150`; `CLAUDE.md`), the cgo-free default
provides Schema-IR + codegen only — **no** `DataTree`, generic data parse, RFC-7950 data
validation, or generic ordered serialization; those are `//go:build cgo` in
`go/libyangbackend`, and a static test proves there is no faked `!cgo` stub. This split is
correct and enforced. The gap is **doc framing**: `docs/cambium-kickoff.md:477-482` and
`docs/quickstart.md` show `cambium.Parse`/`t.Validate`/`t.Serialize` as core API with no
cgo caveat (those symbols exist only in `libyangbackend`). `quickstart.md` predates the
pure-Go rebuild and was never updated. **Fix:** mark every data-tier example as
cgo-backend-only so docs match the default surface.

**G-22 — Compat parity is snapshot-scoped. [Low]**
`go/compat` parity tests diff against the vendored goyang **v1.6.3 fork**
(`internal/yangparse/upstream/yang`, pinned in `UPSTREAM.md`), not live
openconfig/goyang. Drift vs upstream HEAD is uncaught. The pin *is* documented; the only
missing piece is a periodic upstream re-diff.

### Theme F — Correctness robustness nits

**G-13 — `Send + Sync` contract is actually violated. [Medium]**
`SchemaForest` keys a `HashMap` on raw `*const c_void` (`schema.rs:1581`) for the
data→schema bridge, making `SchemaForest` (hence `Context`) `!Send + !Sync`. This
**contradicts** the documented guarantee (`spec/api.md:79` "frozen Context (Send + Sync)";
`context.rs:52` doc). A compile probe (`assert_sync::<Context>()`) fails to build. No
`assert_send/assert_sync` test guards it, so the regression is silent. (The verifier
refuted the original "UB on move/rebuild" framing — the lifetime ties data tree and forest
to one frozen ctx — but the Send+Sync violation is real and provable.) Also `cambium-core`
lib.rs:2 claims "imports zero libyang/cgo types," yet several modules import
`cambium_libyang_sys::adapter` Raw* DTOs (pure data shapes, so the *public* D-1 boundary
still holds, but the absolute doc statement is false). **Fix:** either restore Send+Sync
(e.g. key the bridge on an index/newtype, not a raw pointer) or correct the spec/docs, and
add a compile-time `assert_send/assert_sync` guard.

**G-14 — Brittle validation sub-code classification. [Low]**
`go/libyangbackend/validation.go:159-182` and `rust/cambium-core/src/tree.rs` both classify
some `When`/`Mandatory` sub-codes by prefix-matching libyang's English message (these
errors carry no app-tag; `ly_strvecode` is too coarse — all `LYVE_DATA`). A wording/locale
change degrades them to `ValidationNone`/generic diagnostics. `min`/`max-elements` and
several other cases do go through stable app-tags, so the brittle surface is narrower than
the first-pass claim. Existing must/when/mandatory tests partly pin current wording.
**Fix:** add wording-pinning tests in both languages and alert on unexpected
`ValidationNone` regressions; prefer stable upstream codes if libyang exposes them.

**G-19 — `find_by_key` / `FindByKey` have no exact/duplicate-key guard. [Low]**
`rust/cambium-core/src/list.rs:73-90,199-216` and `go/libyangbackend/tree.go` count a key
matched if *any* child matches and return the first entry with `matched == len(keys)`. With
a duplicate key-name in the input slice (e.g. composite key `a b` queried as
`[("a","x"),("a","x")]`), both passes match the single `a` child → false match; `b` is
never checked. Only single-key lookups are tested; composite-key fixtures exist but never
drive these lookup helpers. (A non-key child *cannot* collide with a key name in valid
YANG, so that vector was overstated.) **Fix:** require distinct key names / exact key-set
matching in both languages and add adversarial composite-key tests.

**G-17 — Dead/duplicated surface. [Low]**
`Error::InvalidPath` is never constructed (all path failures route through
`Error::ffi(DataPath)`). The legacy `Context::new`/`SchemaTree` path coexists with
`ContextBuilder`, is **not** `#[deprecated]` (only doc-commented), and 11 core test files
plus 2 codegen test files still use it — diverging from the single-builder story (D-3).
(`Error::Stale` being unconstructed in Rust is intentional cross-language parity, not dead
code.)

## 5. Prioritized remediation

1. **G-02** — fix the Rust codegen node-drop (choice/RPC/action/notification/anydata) and
   wire the existing fixtures into a Rust codegen test. This is a correctness bug in the
   *primary* stack and the single most important item.
2. **G-01** — unblock billing, get one fully green CI run before any release tag; add a
   Windows job if Windows is a target.
3. **G-09 + G-08** — put codegen on the shared conformance corpus and make the I1–I6→fixture
   mapping machine-checkable, so G-02-class regressions become CI-visible.
4. **G-03 / G-05** — decide Rust codegen scope (close the parity gap or contract it in
   `/spec`) and complete the facade re-export surface.
5. **G-13 / G-10** — restore (or correct the docs on) `Context: Send + Sync` with a
   compile-time guard; hard-fail stale bindings on a SHA bump.
6. **G-06 / G-12 / G-07** — reconcile docs with reality (data-tier caveats, fixture counts,
   gNMI deferral marker).
7. Low-severity robustness/cleanup: G-16, G-17, G-18, G-19, G-14, G-11, G-22.

## 6. Investigated and NOT a gap (do not re-open)

These were candidate gaps the adversarial pass **refuted**:

- **G-15** — "pure-Go has no data byte-gate / no shippable binary under CGO=0." Refuted:
  `go test ./codegen` runs under `CGO_ENABLED=0` and byte-compares 149 generated outputs
  to the conformance goldens (D-4 as designed); the 7 schema-ir cases run pure-Go; the 193
  backend-data cases are cgo-by-spec. The only true residual (no pure-Go *binary*) is a doc
  note, required by no spec.
- **G-20** — "DiffEdit lacks a positional target / merge pre-scan is Replace-only."
  Refuted: spec routes I6 atomicity through `is_ordered_by_user` + preserved libyang
  `yang:insert` metadata in the serialized subtree (not a per-edit field), proven by
  `TestDiffOrderedByUserAtomic`; the spec's *only* merge-conflict definition is a
  same-leaf value clash, which the pre-scan catches, and a shared frozen `ly_ctx` makes a
  node-kind conflict at one path impossible.
- **G-21** — "cgo build silently needs prebuilt static libs." Refuted: CI builds the engine
  on every platform via an explicit `build.sh` step; `green-bar.sh`/`check-flattened-build.sh`
  do too; the prerequisite is documented and `build.sh` errors clearly. Only a minor "no
  `go:generate` preflight" ergonomics nit remains.

## 7. Bottom line

Cambium is a genuinely substantial, mostly-working implementation whose headline
order-correctness mechanics (positional `ordered-by user` types, keys-first, schema-order
children, canonical system order) are real and golden-tested on the libyang-backed paths,
and whose pure-Go schema/codegen default is a legitimate cgo-free engine — not a goyang
wrapper. "Complete" is over-stated, not fabricated. The work remaining is: **one real
correctness bug in the Rust codegen (G-02)**, a **release process that was declared green
without ever running (G-01)**, and a cluster of **parity/coverage/scope/doc seams** the
current test suite structurally cannot detect. Close G-02, G-01, and G-09/G-08 and the
"complete" claim becomes defensible.
