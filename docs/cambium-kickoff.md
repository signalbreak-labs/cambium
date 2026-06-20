# Cambium — Kickoff & Design Brief

> Status: **draft v0.1 (kickoff)** · Owner: signalbreak-labs · Engine bet: **libyang (CESNET)** · Languages: **Rust (primary) + Go**
> This document is the pre-spec kickoff. It is researched, fact-checked, and adversarially reviewed, but two design questions (below) are deliberately left open to resolve **before** the white-room spec.

Cambium is a modern, fully-owned YANG toolkit for Rust and Go, built on libyang as the RFC-7950-complete engine, with near-identical idiomatic public APIs and **order correctness as a first-class, never-broken guarantee** — the exact place openconfig/goyang (the parser/AST library ygot is built on) falls down. Cambium is goyang's successor, not ygot's.

All version numbers, crate names, maintainers, and GitHub issue references in this document were independently re-verified against the live GitHub / crates.io APIs on **2026-06-13** unless marked otherwise.

---

## 0. TL;DR and the two decisions to make first

**The bet is sound.** libyang is healthy and the only RFC-7950-complete, order-correct YANG engine with real production users; the Rust prior art (Holo's `yang-rs`) proves the FFI approach; goyang/ygot's ordering defects are real, verified, and structural. Making order a *structural property of the tree* (ordered sequence authoritative, keyed map only a derived cache) and modeling `ordered-by user` as a positional-only type so misuse **won't compile** is the right design.

**But one headline feature is not implementable as first drafted, and it's exactly your ordering concern.** libyang **v3+ sorts `ordered-by system` config lists/leaf-lists by key/value *during parse*** ([transition manual](https://netopeer.liberouter.org/doc/libyang/devel/html/transition2_3.html)). So a device's *arbitrary* emitted order for system-ordered nodes is gone before Cambium ever sees the tree. The originally-proposed "fidelity mode" (replay original system-ordered order for byte-exact NETCONF diffing) cannot work through a single coarse FFI parse call.

This is fine for **correctness**, and it does **not** touch the order that matters most in practice. Two distinct things are easy to conflate:

- **Declaration order of child/sibling nodes** inside a container or list entry — the order leaves, nested containers, and `uses`/grouping-expanded nodes appear in the YANG module. **This is the order NETCONF expects on input and the RFC-canonical encoding.** libyang places these in **schema declaration order** on insert, so Cambium gets it for free and guarantees it (I2/I4). It is exactly what goyang breaks by alphabetizing through its `Entry.Dir` map. **Never at risk.**
- **Entry order of a *system-ordered* list** (or leaf-list values) — *which* device emitted `eth2,eth0,eth1`. libyang canonicalizes this to key/value order on parse. RFC 7950 says this order is *implementation-determined and not significant*, so canonicalizing is correct; the only casualty is *cosmetic byte-replay* of one device's arbitrary system-ordered entry order. **That, and only that, is what Decision 1 is about.**

### Decision 1 — system-ordered "fidelity" — **RESOLVED: canonical-only (A)**
**Driver:** A representative downstream consumer (a **YANG → Terraform-provider** generator living in a separate repo) must emit NETCONF in the required order — Cambium just has to give it order-correct trees. On the *write/apply* path, the order that matters (schema declaration order for containers/groups; `ordered-by user` preserved; keys-first; RPC I/O) is fully guaranteed (I1–I4). On the *read/refresh* path the device may return system-ordered entries in any order — and a TF provider *wants* a **stable canonical** representation to avoid perpetual diffs, which is exactly what libyang's canonicalization yields. (Today this is worked around with unordered TF sets; Cambium can normalize it properly instead.)
- **(A) Canonical-only — adopted.** `ordered-by user` byte-exact; system-ordered deterministic-canonical (stable across runs, languages, *and* device-return-order). RFC-correct, strictly better than goyang.
- **(B) Order-capture pre-pass — backlog/stretch.** Byte-exact replay of a device's *arbitrary* system-ordered entry order (device-diff use case) needs a real pre-pass + the unverified `LYD_PARSE_ONLY|LYD_PARSE_OPAQ` assumption. **Not needed for the TF use case;** revisit only if a byte-exact device-diff feature is later wanted.

### Decision 2 — sequencing — **RESOLVED: Rust-first, Go is a first-class parity target**
Build **Rust first** (the owner is deep in Rust now and can use it immediately); **Go is a first-class parity target** because downstream consumers (e.g. Terraform providers, which are Go / terraform-plugin-framework) will import the Go SDK. The first-class Cambium goals are the order-correct tree, validation, generic ordered serialization, and **typed-struct codegen** (field-order manifest + deterministic serializer, zero NETCONF/Terraform coupling). A YANG → Terraform-provider generator is a **downstream consumer in a separate repo**, not a Cambium milestone. Sequence: Rust core to completion → Go parity against the same `/conformance` golden → typed-struct codegen. Two FFI stacks + codegen + gNMI in one 8-week block for 1–2 engineers is not credible, so we sequence; the architecture keeps Go a clean mirror.

Everything below is structured around those two decisions.

---

## 1. Research summary

### 1.1 Verified facts (live GitHub / crates.io, 2026-06-13)

| Fact | Value | Source |
|---|---|---|
| libyang latest stable | **v5.4.9** (2026-04-01); devel `VERSION 5.8.5` (2026-06-02) | [releases](https://github.com/CESNET/libyang/releases) |
| libyang major cadence | v2.2.8 → v3.1.0 → v3.13.6 → v4.2.2 → v5.4.9 in ~2 yrs (aggressive SO churn) | [tags](https://github.com/CESNET/libyang/tags) |
| libyang license / maintainer | BSD-3-Clause; **Michal Vasko** (3869 commits; bus-factor ≈ 1) | [contributors](https://github.com/CESNET/libyang/graphs/contributors) |
| Coordinated CESNET release | libyang v5.4.9 + sysrepo v4.5.4 + Netopeer2 v2.8.2 (all 2026-04-01) | CESNET release APIs |
| Rust bindings (canonical) | **holo-routing/yang-rs** by Renato Westphal: `yang3` 0.19.0, `yang5` 0.2.0; `libyang{2,3,4,5}-sys`; one crate **per libyang major** | [crates.io/yang3](https://crates.io/crates/yang3), [yang-rs](https://github.com/holo-routing/yang-rs) |
| `yang5` engine pin | submodule pinned to libyang `v5.4.9` (commit `f302d86`); `bundled`/`bindgen` features; bindgen 0.72.0 | yang-rs `yang5` branch |
| Pure-Rust YANG parser | **none viable** (sjtakada/yang-rs, last 2022; abandoned, RFC-incomplete) | [crates.io/yang-rs](https://crates.io/crates/yang-rs) |
| Go libyang binding | **no reusable library**; only SONiC `cvl/yparser.go` (`#cgo LDFLAGS: -lyang`, system-linked, app-internal) | [sonic-mgmt-common](https://github.com/sonic-net/sonic-mgmt-common) |
| goyang / ygot | goyang 242★ (active commits, mostly CI bumps); ygot 325★, latest **v0.34.0 (2025-09-03)** | [goyang](https://github.com/openconfig/goyang), [ygot](https://github.com/openconfig/ygot) |
| pyang | 2.7.1 (2025-08-29) | [pyang](https://github.com/mbj4668/pyang) |

### 1.2 libyang as the engine — strengths, risks

**Strengths:** Full RFC 7950 (YANG 1.1) incl. deviations, augments, `must`/`when` XPath, `mandatory`, leafref require-instance, extensions/plugins, schema-mount (8528), structure (8791); first-class XML, RFC-7951 JSON (the gNMI JSON_IETF content layer), and LYB binary; separable parse/validate with rich option bitmaps (`LYD_PARSE_ONLY/STRICT/OPAQ`, `LYD_VALIDATE_NO_STATE/PRESENT/MULTI_ERROR`); small, well-triaged issue backlog.

**Risks (and mitigations):**
| Risk | Mitigation |
|---|---|
| **Aggressive SO-major churn** (≈ one major/6–9 mo; v2→v3 silently changed ordering semantics; LYB restricted in v4, restored v5) | **Never leak the major into a public crate/package name.** Pin by SHA in `/VERSIONS`; treat a major bump as an internal `build.rs`/cgo change + full conformance + ordering re-run. Budget recurring migration work. |
| **Bus-factor ≈ 1** (Michal Vasko / CESNET) | Vendor + pin exact source; keep Cambium's high-level layer engine-swappable; maintain a conformance corpus so the engine is replaceable in principle. |
| **System-ordered auto-sort on parse** (v3+) | See Decision 1. Frame Cambium's guarantee honestly: user-ordered byte-exact, system-ordered canonical-deterministic. |
| **`ly_ctx` thread-safety asserted, not proven** | Build-once-then-freeze model is right, but **the "concurrent parse into separate trees" / `Send+Sync` claim must be verified empirically** (race/TSan test) before it appears in docs. |

### 1.3 Prior art and lessons

| Project | Lesson for Cambium |
|---|---|
| **Holo `yang-rs`** (yang3/yang5) | Proves libyang-FFI-from-Rust at production quality; **already has ygot-equivalent codegen but buried inside the Holo daemon** (`holo-northbound/src/yang_codegen/`) — the exact reusable gap Cambium fills. Study its `build.rs` (dynamic default / `bundled` static / `bindgen`); do **not** build the high-level layer on top of it — own it. |
| **yangson** (Python) | Best pure-software ordering model: a persistent "zipper" tree where user-ordered order is *intrinsic to the structure*, no out-of-band metadata. The positive model to mirror. |
| **pyangbind** | Anti-pattern: dict/OrderedDict-backed lists force a non-standard `__yang_order` JSON sidecar. **Never make a keyed map the primary child container.** |
| **YDK** | Shared cross-language **core** under thin generated bindings — directly applicable to Rust+Go near-identical APIs. Drive codegen from a neutral IR with per-language emitters. |
| **OpenConfig models-ci** | Conformance model to copy: a matrix of independent validators with a **required vs informational** split, per-PR, fail-on-disagreement. |
| **gnmic / RFC 7951** | The serialization contract: strict JSON_IETF, module-prefix only at namespace boundaries — implement that prefix rule in **one shared place** so JSON, gNMI, and codegen agree. |
| **NSO (real-world)** | Order is silently dropped at boundaries (NSO JSON-RPC reorders `ordered-by user` payloads). Defend order at *every* boundary and regression-test that failure mode. |

### 1.4 goyang / ygot pain points Cambium must beat (the differentiator)

Order is the headline. The root causes are **structural and verified**:

| Pain | Evidence | Cambium's structural answer |
|---|---|---|
| **goyang loses YANG declaration order** | children stored in `Entry.Dir map[string]*Entry`, iterated via `sort.Strings` → lexicographic, not declaration order | Walk libyang's `lyd_node.next/prev` sibling chain; ordered `Vec`/slice authoritative, keyed map only a derived lookup, **never iterated for serialization** |
| **ygot: no insertion-order preservation** for `ordered-by user` (added late, opt-in) | [ygot #425](https://github.com/openconfig/ygot/issues/425) "insertion order for lists" — **OPEN since 2020-08-05**; gated behind opt-in `generate_ordered_maps` | Order **on by default, no flag**; `UserOrderedList<T>` positional-only type |
| **ygot: nested `ordered-by user` unsupported** | errors `detected nested ordered-by user list` | Native via the positional type at any depth |
| **ygot: keyless lists historically unsupported** | [ygot #716](https://github.com/openconfig/ygot/issues/716) (CLOSED; error string `keyless list cannot be output`) | Keyless lists carry positional order natively |
| **ygot: no `must`/`when` XPath, no `mandatory` enforcement** | [ygot #514](https://github.com/openconfig/ygot/issues/514) "Mandatory field not honoured" — **OPEN** | Delegated to libyang (full XPath 1.0, mandatory, leafref require-instance) |
| **goyang: YANG-1.1 gaps** (`ToEntry` largely 1.0) | [goyang #51](https://github.com/openconfig/goyang/issues/51), [#190](https://github.com/openconfig/goyang/issues/190) — **OPEN** | Full YANG 1.1 via libyang |
| **ygot codegen bloat** (groupings inlined → duplicate structs) | design docs | Grouping-aware dedup (post-1.0, `--dedup-groupings`) |
| **Low feature velocity** | ygot still 0.x (v0.34.0, 2025-09); goyang commits are mostly CI/dep bumps and it self-describes as a work-in-progress | "Actively owned, modern engine, full control" — **not** a deprecation claim |

> **Citation hygiene (must-fix before any public copy):** ygot **#167** is *"Duplicated struct generation"* (**CLOSED**) — do **not** cite it as the source of non-deterministic `Top_C`/`Top_C_` struct naming. Non-determinism from Go map iteration is a real *class* of issue, but state it without pinning it to #167. ygot **#716** is **CLOSED** (keyless lists since addressed) — say "historically" not "currently". The load-bearing, currently-open citations are **#425** and **#514** (ygot) and **#51 / #190** (goyang).

### 1.5 The ordering reference (RFC-grounded)

| Surface | Rule | RFC |
|---|---|---|
| **Child/sibling node order** in a container or list entry (leaves, nested containers, `uses`/grouping expansion) | encode in **schema declaration order** — the RFC-canonical form and what NETCONF servers expect on input. A *compliant* server must also *accept* any order, but real devices are order-sensitive, so always **emit schema order** (this is precisely what goyang breaks). | RFC 7950 §7.8.5 + canonical encoding |
| `ordered-by user` list/leaf-list **entry** order | **semantically significant**; server MUST maintain | RFC 7950 §7.7.7, §7.8.6 |
| `ordered-by system` list **entry** / leaf-list **value** order | implementation-determined, **not** significant; libyang canonicalizes to key/value order on parse | RFC 7950 §7.7.7 |
| Keys within a list entry | **keys first**, in key-statement order | RFC 7950 §7.8.5 |
| RPC/action input/output | children in **schema order** | RFC 7950 |
| NETCONF positioning | `yang:insert` + `yang:key`/`yang:value` (first/last/before/after); element order within one `<edit-config>` is processing-significant | RFC 6241, RFC 7950 §7.8.6 |
| RESTCONF positioning | `insert` + `point` query params (before/after require `point`) | RFC 8040 §4.8.5–4.8.6 |
| JSON | list/leaf-list are **arrays** (order carries); a JSON **object** is unordered — member order carries no meaning | RFC 7951 |
| gNMI | `SetRequest` ops applied delete→replace→update; **no native insert/move**; `ordered-by user` must ride as JSON_IETF arrays / atomic subtrees | gNMI spec |

---

## 2. Architecture

### 2.1 Canonical decisions (the team builds against these)

| Decision | Choice | Why |
|---|---|---|
| Engine | Vendored **libyang**, pinned by SHA | Only RFC-7950-complete, order-correct engine with production users |
| libyang major | **5.x** (track `v5.4.9`), absorbed internally | Where upstream + Holo momentum live; LYB restored |
| 2nd C dep | Vendored **PCRE2**, built static + `-fPIC` | libyang's only real external dep; `yang-rs` links *system* PCRE2 (breaks zero-dep promise) |
| Rust crates | facade **`cambium`** · safe core **`cambium-core`** · FFI **`cambium-libyang-sys`** (no major in name) · **`cambium-codegen`** + **`cambium-cli`** | Major never leaks into a public name — the `yang2/3/4/5` cautionary tale |
| Go module | **`github.com/signalbreak-labs/cambium/go`** · `package cambium` · internal `package libyang` (cgo) · `package codegen` · `cmd/cambium` | Mirror of the Rust surface; module rooted at `/go` for a clean polyglot root |
| Shared, language-neutral | **`/spec`** (API contract, naming rules, ordering invariants I1–I6, rule-code registry) · **`/conformance`** (pinned corpus, fixtures, golden outputs) · **`/VERSIONS`** (SHA + **exact CMake flag** pins) | One source of truth; cross-language drift = CI red |

### 2.2 Layered binding strategy

One **shared, pinned C engine** under two symmetric native stacks:

```
        RUST                                  GO
  cambium (facade) + codegen + cli      package cambium + codegen + cmd/cambium   ← idiomatic, per-language
  cambium-core (tree/order/validate)    (same, in package cambium)                ← safe core (spec-identical)
  cambium-libyang-sys (bindgen FFI)     internal/libyang (cgo, coarse calls)      ← thin unsafe FFI
                       └──────────── /third_party/{libyang,pcre2} ────────────┘   ← ONE pinned copy (shared)
```

- **Shared:** the C source (`/third_party`, submodules, SHA-pinned) and `/spec` + `/conformance`.
- **Not shared:** FFI bindings, safe core, facade, codegen — each language reimplements against `/spec`; behavior pinned by `/conformance` golden outputs.
- **FFI boundary is coarse-grained by mandate:** parse/validate/serialize a *whole document* per FFI call; walk the libyang sibling chain in C, return a complete ordered structure. One cgo call ≈ tens of ns; one-per-node would destroy tree-walk performance. C→Go callbacks (~150 ns) banned from hot paths.
- **Engine concurrency:** `ly_ctx` is build-once-then-frozen (mutation confined to a single-threaded `ContextBuilder`). **The frozen-context "concurrent parse" / `Send+Sync` claim is UNVERIFIED and must be proven with a race-detector test before it ships in docs or API guarantees.** Data trees (`lyd_node`) are mutable and not concurrency-safe; lifetimes via Rust `Drop` / Go `Close()`+finalizer. Values always read via `lyd_get_value()`, never `node->_canonical`.

### 2.3 The in-memory model — order preserved by construction

**The rule is mechanical: order is a structural property of the tree — never a sort key, never a sidecar, never a map.**

- Children are an **ordered sequence** (`Vec<DataNode>` / `[]*DataNode`) = source of truth. A keyed index (`IndexMap` / `map[Key]…`) is a **derived cache for lookup only**, never iterated for serialization.
  - **Index correctness fix (from review):** the index must map key → **node identity** (`&node` / `*DataNode`), **not** key → position. Positional moves shift positions and would invalidate a position-cache; identity is move-stable. CI must assert no serialization path ever reads the index.
- `ordered-by user` is a **distinct positional type** — `UserOrderedList<T>` / `UserOrderedLeafList<T>` — whose *only* mutators are positional (`insert_first/last/before/after`, `move_*`), mapping 1:1 to `lyd_insert_before/after` and NETCONF `yang:insert`. It has **no** order-agnostic setter, so **reordering a system-ordered node by mistake is a compile error** (the method doesn't exist). System-ordered uses `OrderedList<T>` with order-agnostic `insert`/`upsert` (engine places it).
- **Serialization determinism source (from review):** *all* structured output (XML/JSON/gNMI) is produced by the **single libyang printer walk** over the sibling chain — never by a language-native map/struct serializer (no `serde_json::Map` default ordering, no Go `encoding/json` map sorting). JSON object member order == libyang sibling order in *both* languages, asserted by the cross-language byte-diff gate.

#### Ordering invariants (I1–I6, CI-gated in `/spec`)
- **I1** round-trip preserves `ordered-by user` order byte-for-byte (modulo whitespace).
- **I2** every node is emitted in **schema declaration order** (containers, leaves, `uses`/grouping expansion) — the order NETCONF expects, never alphabetical; system-ordered list *entries* canonicalize to key order and leaf-list *values* to value order. Deterministic across runs *and* languages.
- **I3** within a list entry, keys are emitted first, in key-statement order.
- **I4** RPC/action input/output children are emitted in schema order.
- **I5** JSON list/leaf-list arrays carry order; JSON object member order is whatever the **single libyang printer** emits (deterministic), never a native map.
- **I6** gNMI `ordered-by user` nodes are emitted as JSON_IETF arrays / atomic subtrees, never decomposed into per-leaf `TypedValue` updates.

> **Descoped per Decision 1:** the original "I2 fidelity mode — replay original system-ordered order captured at parse" is **not implementable** through the coarse FFI (libyang sorts on parse). I2 is **canonical-determinism only** unless Decision 1(B)'s pre-pass is prototyped and proven in week 1.

### 2.4 Codegen, validation, serialization (summary)

- **Codegen** is driven from the **schema-tree IR** (carries libyang declaration order), shared analysis → per-language emitters (YDK-shaped). Generated structs carry an explicit **field-order manifest** the serializer walks — never reflection/map order. **Review fix:** because augments/deviations can change runtime order, either (a) make the libyang printer the single source of on-wire order for generated types too, or (b) derive the manifest from the *same compiled schema* the runtime uses, with a CI assertion that generated-serializer bytes == libyang's `LYD_JSON`/`LYD_XML` bytes. Two independent order sources may not exist without that gate.
- **Validation** maps libyang's separable bitmaps to typed modes (`ParseMode::{DataOnly,Strict,Opaque}`, `ValidateMode::{ConfigOnly,MultiError}`, whole-datastore `validate()`); diagnostics use clippy-style named **rule codes** (`CAMBIUM_xxxx` + doc URL).
- **Serialization** is one ordered sibling-chain walk per format (XML/`LYD_XML`, JSON/`LYD_JSON`, gNMI JSON_IETF over openconfig/gnmi protos via tonic/prost + Go stubs, LYB as same-version fast-path only) with libyang-mirrored flags (`WITH_SIBLINGS`, with-defaults `EXPLICIT/TRIM/ALL`).

### 2.5 Cross-language consistency guarantees

1. **Shared `/spec`** — identical type names; Rust `snake_case` ↔ Go `PascalCase` same-root methods; `<T>` ↔ `[T]`; identical option-struct field names. PRs that diverge fail review.
2. **Shared `/conformance` + golden outputs** — one corpus, two runners; **Rust bytes ≠ Go bytes for the same input+mode ⇒ CI red.** libyang/yanglint is the mandatory oracle; `pyang -f tree` informational; goyang/ygot the migration differential (assert Cambium ≥ goyang on order).
3. **Pinned single engine** — `/VERSIONS` pins libyang + PCRE2 by SHA **and the exact CMake configure flags** (review fix: a flag drift, e.g. XXHASH on one side, can change printer bytes and fail the differential for build reasons). A CI job diffs the two engine build-configs + a fixed-fixture print before running the semantic differential, and runs against the **release-flattened** artifacts (what users actually get).

### 2.6 Folder layout

```
cambium/
├── VERSIONS                 # SHA + exact CMake-flag pins (libyang, pcre2, corpus)
├── Cargo.toml               # Rust workspace root
├── AGENTS.md / CLAUDE.md
├── third_party/             # ONE pinned C engine, shared (submodules)
│   ├── libyang/   pcre2/
├── spec/                    # SHARED contract
│   ├── api.md  ordering-invariants.md  serialization.md  rule-codes.md
├── conformance/             # SHARED corpus + golden (one set, two runners)
│   ├── corpus/ (submodules) fixtures/ golden/ manifest.toml
├── rust/
│   ├── cambium/ cambium-core/ cambium-libyang-sys/ cambium-codegen/ cambium-cli/ conformance-runner/
├── go/                      # module github.com/signalbreak-labs/cambium/go
│   ├── go.mod cambium/ internal/libyang/ codegen/ cmd/cambium/ conformance/
├── docs/   ci/
```

> **Packaging (load-bearing):** crates.io excludes submodule contents and `go get` doesn't clone submodules. A CI-gated **release-flatten** step copies the pinned `third_party` C into `cambium-libyang-sys/` and `go/internal/libyang/` before publish (the mattn/go-sqlite3 model), strips libyang `tests/`, keeps BSD-3 + PCRE2 license/NOTICE headers, regenerates bindings against the flattened copy, and **builds the flattened unit in isolation** to catch "works in repo, breaks when published" drift.

---

## 3. Public API (side-by-side, abridged)

Full examples for all five flows (load, walk schema, parse+validate, codegen, serialize) are in `.planning/extract/api-parity.md`. The two load-bearing snippets:

**Parse + validate (separable, mirrors libyang bitmaps):**
```rust
let tree: DataTree = tree::parse(&ctx, Format::Json, ParseMode::DataOnly, &bytes)?;
tree.validate(ValidateMode::ConfigOnly | ValidateMode::MultiError)?; // libyang: must/when, mandatory, leafref…
```
```go
tree, err := cambium.Parse(ctx, cambium.FormatJSON, cambium.ParseModeDataOnly, bytes)
err = tree.Validate(cambium.ValidateModeConfigOnly | cambium.ValidateModeMultiError)
```

**The ordering type — misuse won't compile:**
```rust
let subifs = tree.user_ordered_list_at("/interfaces/interface[name='eth0']/subinterfaces")?;
subifs.insert_first(make_subif(0))?;          // -> lyd_insert_before / yang:insert
subifs.insert_after(p0, make_subif(5))?;
subifs.move_before(p5, p0)?;
// there is NO subifs.set(key, val): accidental reorder is a COMPILE error
let json = tree.serialize(Format::Json, SerializeFlags::WITH_SIBLINGS | WithDefaults::Explicit)?;
```
```go
subifs, _ := tree.UserOrderedListAt("/interfaces/interface[name='eth0']/subinterfaces")
subifs.InsertFirst(makeSubif(0))              // -> lyd_insert_before / yang:insert
subifs.InsertAfter(p0, makeSubif(5)); subifs.MoveBefore(p5, p0)
// no Set(key,val): positional-only, vet-able by design
jsonBytes, _ := tree.Serialize(cambium.FormatJSON, cambium.SerializeWithSiblings|cambium.WithDefaultsExplicit)
```

Parity rules: identical type names; the only sanctioned differences are `Result<T,E>`↔`(T,error)`, `Drop`↔`Close()`, iterator↔slice, `Option<T>`↔`(T,ok)`. Full naming table in `/spec/api.md`.

> Review note carried into the spec: the `.children()` lazy-iterator (Rust, borrows C memory) vs materialized `[]*DataNode` (Go) is a **lifetime asymmetry**, not just naming — `/spec` must state the borrow/ownership contract, and the "fidelity" flag must be removed from the serialize API until Decision 1 lands.

---

## 4. Roadmap (re-scoped: Rust-first, 8 weeks)

**Team:** 1–2 engineers + heavy AI assist. Anything touching the **ordering invariant** or **C build/link** is human-verified, not AI-trusted. Go parity, codegen, and gNMI move to a **second 8-week block** (§4.2) — the original "both languages + codegen + gNMI + fidelity in 8 weeks" was not credible.

### 4.1 Block 1 — Rust core, order-proven (weeks 1–8)

| M | Weeks | Goal | Definition of Done | Coverage |
|---|---|---|---|---|
| **M0** | 1–1.5 | **It builds, pinned.** Vendored libyang **+ two-stage PCRE2 CMake** static-linked into a trivial Rust bin on Linux x86_64/arm64 + macOS arm64. Decide & prototype **Decision 1(B)** order-capture or descope to 1(A). | `cargo test -p cambium-libyang-sys` green; `otool/ldd` show no system libyang/pcre2; DOCS_RS no-network/no-CMake doc build proven in the crates-build-env image; `/VERSIONS` pins SHAs **and CMake flags** | build gate only |
| **M1** | 2–4.5 | **Parse / validate / serialize, ORDER-PROVEN.** `DataTree` over `lyd_node.next/prev`; `Vec` authoritative, identity index; coarse FFI; I1/I3/I4 golden. | A deliberately scrambled-input fixture round-trips order-correct where goyang would alphabetize; I1/I3/I4 green; `must`/`when`/`mandatory` validation surfaced with rule codes | ≥85% parse/serialize line; 100% I1/I3/I4 fixtures |
| **M2** | 5–6.5 | **Ordering edge cases + canonical determinism.** `UserOrderedList<T>` positional-only (compile-fail test via `trybuild`); nested + keyless user-ordered; I2 canonical (+ I5/I6 JSON/gNMI shape); `CAMBIUM_xxxx` rule registry. | Positional-misuse fails to compile; nested/keyless round-trip (cases ygot fails); I2 deterministic across runs | ≥90% branch on ordering module |
| **M3** | 7–8 | **Conformance harness + v0.1 Rust release.** Tiered corpus (submodules), parse-all + round-trip + libyang-oracle; differential vs goyang on order; docs (quickstart, the ordering-story page); LICENSE/NOTICE for vendored C. | Tier-0 IETF core 100% parse+round-trip+oracle; published migration-diff page; crate publishes with flattened C and green docs.rs | conformance gates wired |

### 4.2 Block 2 — Go parity + codegen + gNMI (next 8 weeks, summary)
Go FFI (cgo, vendored C, zig-cc cross-compile) mirroring M0–M2 against the **same `/conformance` golden** (Rust bytes == Go bytes gate); then typed-struct codegen (schema-IR → typed structs, field-order manifest, byte-deterministic, order-on-by-default — the answer to ygot #425); then gNMI JSON_IETF codec (I6) + `cambium convert` + an ygot-struct interop adapter. A YANG → Terraform-provider generator that emits NETCONF on apply and canonical-normalizes on read is a **downstream consumer built in a separate repo** on top of this SDK — out of scope for Cambium itself.

### 4.3 Backlog (must vs stretch)

| Must (blocks v0.1) | Stretch / deferred |
|---|---|
| Vendored libyang+PCRE2 static build (M0) · `/VERSIONS` SHA+flag pin + bump-gate · sibling-chain ordered tree · parse/validate/serialize XML+JSON · I1–I4 golden + cross-lang gate · coarse FFI · `UserOrderedList<T>` · canonical determinism (I2) · `RuleCode` registry · conformance Tier-0 | Decision-1(B) order-capture pre-pass · Go parity (Block 2) · codegen · gNMI codec · Windows/MSVC (tier-2) · `--dedup-groupings` · goyang-shaped API shim · ygot-struct interop · LYB fast-path · `system_libyang` opt-out |

---

## 5. Migration & community

**Staged on-ramp (no big-bang):** (0) *validate-alongside* — run `cambium validate` for full RFC-7950 checks ygot lacks (`must`/`when`, `mandatory` — #514); (1) *diff-the-order* — report where goyang/ygot reorder; (2) *re-generate types* — `cambium generate --lang go`, order on by default; (3) *interop* — ygot-struct adapter marshals existing `GoStruct`s through Cambium's ordered serializer (fix order without regenerating); (4) *full adoption*.

**Compatibility ideas:** goyang-shaped `Entry` view backed by `SchemaTree` (post-M4, design-first; intentionally *not* reproducing the alphabetical-iteration bug); ygot-struct interop adapter (best early win); drop-in CLI flags (`--generate_ordered_maps` accepted as a no-op + deprecation note).

**Positioning (honest):** "goyang is YANG-1.0-biased and alphabetizes your children (`Entry.Dir` map + `sort.Strings`); Cambium makes order a structural property — byte-exact for `ordered-by user`, deterministic-canonical for system-ordered." · "Full RFC 7950 via the same engine Holo ships, pinned." · "Rust and Go produce identical bytes — enforced in CI." Avoid claiming goyang/ygot are deprecated (they aren't); the verifiable point is *low feature velocity* + *structural ordering defects*.

**First community signals:** the ordering-story page (scrambled input: goyang alphabetizes, Cambium preserves), worked `openconfig-interfaces` round-trip, a conformance badge driven by `/conformance`, a public `/VERSIONS` with the bump-gate, and a runnable migration-diff report.

---

## 6. Risks, open decisions, next actions

### 6.1 Risk register

| Risk | Sev | Likelihood | Mitigation |
|---|---|---|---|
| **System-ordered fidelity not implementable via coarse FFI** (libyang sorts on parse) | High | Certain (already true) | Decision 1: descope to canonical (A) or prototype pre-pass (B) **week 1** |
| **PCRE2 two-stage static CMake** (libyang `find_package(PCRE2)` expects a prebuilt lib; debug/release name `pcre2-8` mismatch is a known failure) | High | High | M0 must *build & test* stage-1 PCRE2 (static, `-fPIC`) → stage-2 libyang with `PCRE2_LIBRARY/INCLUDE_DIR`; budget >1 week |
| **docs.rs (no network, maybe no CMake) + bundled-by-default** | Med | High | Default to dynamic/system link, `bundled` opt-in (yang-rs convention); committed pre-gen bindings; doc-only feature that compiles no C; test in crates-build-env before first publish |
| **libyang major churn** changes ordering/LYB/ABI | High | Med (≈/6–9 mo) | major never in a public name; SHA+flag pin; full conformance + ordering re-run on bump; budget migration |
| **`ly_ctx` concurrency claim false** | Med | Unknown | **Verify with a race/TSan test before claiming `Send+Sync`/concurrent parse** |
| **cgo cross-compile + static link** (Block 2) | Med | High | zig cc + musl; pin a known-good zig; CI recipes; assume M0-Go slips |
| **Maintainer bandwidth vs scope** (conformance is unbounded) | High | High | Bound the *required* tier to Tier-0 IETF core; Rust-first sequencing; everything else informational/nightly |
| **bus-factor-1 upstream** | Med | Low-Med | engine-swappable high-level layer; vendored pin; conformance corpus |
| **Citation errors in public copy** (#167/#716) | Med | — | Use only #425/#514/#51/#190; fix before any marketing |

### 6.2 Open decisions
1. ~~System-ordered fidelity~~ — **RESOLVED: canonical-only** (a downstream TF provider wants canonical normalization; byte-exact device-diff is backlog).
2. ~~Sequencing~~ — **RESOLVED: Rust-first; Go is a first-class parity target** (downstream consumers such as Terraform providers are Go). First-class Cambium deliverable is typed-struct codegen; TF-provider generation is a downstream consumer in a separate repo.
3. **v0.1 platform matrix:** state explicitly — **Tier-1 Linux x86_64/arm64 + macOS arm64; Windows/MSVC tier-2, post-1.0.** Don't let crates.io/README imply broad support.
4. **Engine relationship to `yang-rs`:** independent vendored `-sys` (full control, recommended) vs depend-on/contribute-upstream. Document the rationale.
5. **Default link mode:** static-bundled (user ergonomics) vs dynamic-system (docs.rs/CI simplicity) as the *default* feature.

### 6.3 Immediate next 3 actions
1. **Prototype the build gate + the ordering reality (week 1).** Stand up `third_party/{libyang@v5.4.9, pcre2}` submodules + `/VERSIONS`; get a static two-stage CMake build linking into a trivial Rust binary on Linux+macOS; and in the same week **empirically test whether original system-ordered input order can be recovered** (a second `LYD_PARSE_ONLY|LYD_PARSE_OPAQ` pass) — this resolves Decision 1 with data, not assumption.
2. **Write `/spec/ordering-invariants.md` (I1–I6) and the first golden fixtures** — including the headline "scrambled input, goyang alphabetizes, Cambium preserves" case and a keys-first XML case. The spec + golden corpus *is* the white-room design artifact; everything implements against it.
3. **Lock the public contract decisions** — crate/module names (no major leak), platform matrix (action 6.2.3), default link mode, and the `yang-rs` relationship — and capture them in `/spec/api.md` so Rust and (later) Go cannot drift.

---

## 7. Starter files (ready to copy)

> Curated, corrected subset. The full agent-generated set (more Go files, CI YAML, conformance manifest) is in `.planning/extract/starter-files.md`. **Do not scaffold the real build yet** — resolve §0 decisions first.

### `/VERSIONS`
```toml
# Single source of truth for the pinned C engine. Bumping ANY value triggers the
# full conformance + ordering re-run in CI (both languages).
[libyang]
repo = "https://github.com/CESNET/libyang"
tag  = "v5.4.9"
sha  = "f302d86cd6083c2bfe16fc2122bc6d4be69ce7a2"   # verify before commit
cmake_flags = ["-DBUILD_SHARED_LIBS=OFF", "-DCMAKE_POSITION_INDEPENDENT_CODE=ON", "-DENABLE_LYD_PRIV=OFF"]

[pcre2]
repo = "https://github.com/PCRE2Project/pcre2"
tag  = "pcre2-10.44"        # verify latest before commit
cmake_flags = ["-DBUILD_SHARED_LIBS=OFF", "-DPCRE2_BUILD_PCRE2_8=ON", "-DCMAKE_POSITION_INDEPENDENT_CODE=ON"]
```

### `Cargo.toml` (workspace root)
```toml
[workspace]
resolver = "2"
members = [
    "rust/cambium", "rust/cambium-core", "rust/cambium-libyang-sys",
    "rust/cambium-codegen", "rust/cambium-cli", "rust/conformance-runner",
]

[workspace.package]
version = "0.0.0"
edition = "2024"
license = "BSD-3-Clause"
repository = "https://github.com/signalbreak-labs/cambium"
authors = ["signalbreak-labs"]

[workspace.lints.clippy]
unwrap_used = "deny"
expect_used = "deny"
```

### `rust/cambium/Cargo.toml` (facade — note: NO libyang major in any name)
```toml
[package]
name = "cambium"
description = "Modern, order-correct YANG toolkit (Rust) — libyang-backed."
version.workspace = true
edition.workspace = true
license.workspace = true
repository.workspace = true

[features]
default = ["bundled"]          # OPEN DECISION 6.2.5 — may flip to dynamic-default for docs.rs/CI
bundled = ["cambium-libyang-sys/bundled"]
system  = ["cambium-libyang-sys/system"]

[dependencies]
cambium-core = { path = "../cambium-core", version = "0.0.0" }

[package.metadata.docs.rs]
# Doc build compiles no C; relies on committed pre-generated bindings + DOCS_RS guard.
features = ["bundled"]
```

### `rust/cambium/src/lib.rs` (skeleton)
```rust
//! # Cambium — order-correct YANG for Rust
//!
//! Order is a **structural property of the tree**: children are an ordered
//! sequence (the source of truth); any keyed index is a derived lookup cache,
//! never iterated for serialization. `ordered-by user` lists are a distinct
//! positional type whose only mutators are positional — misusing them on a
//! system-ordered node is a compile error.
#![deny(missing_docs)]

mod context;
mod schema;
pub mod tree;
mod error;

pub use context::{Context, ContextBuilder};
pub use error::{Error, Result, RuleCode};

/// On-wire formats. Every format is produced by a single ordered walk of the
/// libyang sibling chain — never a language-native map/struct serializer.
#[non_exhaustive]
pub enum Format { Xml, Json, /* gNMI JSON_IETF: Block 2 */ }

/// Parse behaviour, mapping libyang's separable parse-option bitmap.
#[non_exhaustive]
pub enum ParseMode { DataOnly /*LYD_PARSE_ONLY*/, Strict /*LYD_PARSE_STRICT*/, Opaque /*LYD_PARSE_OPAQ*/ }
```

### `rust/cambium-core/src/data.rs` (the ordering-preserving tree — sketch)
```rust
/// A data node. The child SEQUENCE is authoritative for order; `index` is a
/// derived key -> node-IDENTITY lookup (NOT position) so positional moves never
/// invalidate it. No serialization path may read `index`.
pub struct DataNode {
    children: Vec<DataNode>,
    index: indexmap::IndexMap<Key, NodeId>, // identity, not position
    raw: LydNodePtr,                          // borrows the live libyang tree
}

/// `ordered-by user` list: ONLY positional mutators exist. There is deliberately
/// no order-agnostic `set`, so accidental reordering cannot compile.
pub struct UserOrderedList<T> { /* … */ }
impl<T> UserOrderedList<T> {
    pub fn insert_first(&mut self, v: T) -> crate::Result<()> { todo!() } // lyd_insert_before(head)
    pub fn insert_after(&mut self, point: &T, v: T) -> crate::Result<()> { todo!() }
    pub fn move_before(&mut self, what: &T, point: &T) -> crate::Result<()> { todo!() }
}
```

### `go/go.mod`
```
module github.com/signalbreak-labs/cambium/go

go 1.24
```

### `go/internal/libyang/build.go` (cgo build constraints — vendored, static)
```go
// Package libyang is the internal cgo FFI over a vendored, statically-compiled
// libyang + PCRE2. Coarse-grained by mandate: whole-document parse/serialize per
// call; no per-node FFI, no C->Go callbacks in hot paths. Not importable externally.
//
//go:build cgo

package libyang

/*
#cgo CFLAGS: -I${SRCDIR}/c/libyang/src -I${SRCDIR}/c/pcre2/src
#cgo LDFLAGS: -lm
#include <libyang/libyang.h>
*/
import "C"
```

### `README.md` (first draft)
```markdown
# Cambium

**Modern, order-correct YANG for Rust and Go.** libyang-backed, fully owned by signalbreak-labs.

[![CI](https://github.com/signalbreak-labs/cambium/actions/workflows/ci.yml/badge.svg)](https://github.com/signalbreak-labs/cambium/actions)
[![crates.io](https://img.shields.io/crates/v/cambium.svg)](https://crates.io/crates/cambium)
[![Go Reference](https://pkg.go.dev/badge/github.com/signalbreak-labs/cambium/go.svg)](https://pkg.go.dev/github.com/signalbreak-labs/cambium/go)
[![Conformance](https://img.shields.io/badge/conformance-libyang%20oracle-blue)](./conformance)
[![License](https://img.shields.io/badge/license-BSD--3--Clause-green)](./LICENSE)

> The YANG toolkit that **keeps your order**. `ordered-by user` lists round-trip
> byte-exact; system-ordered output is deterministic-canonical. Full RFC 7950 —
> `must`/`when`, `mandatory`, leafref, deviations, augments — delegated to libyang.
> Near-identical Rust and Go APIs, proven byte-identical in CI.

## Why

goyang stores children in a Go map and iterates them alphabetically; ygot bolted
on insertion order late and opt-in ([#425](https://github.com/openconfig/ygot/issues/425), open since 2020).
Cambium makes order a **structural property of the tree** and delegates RFC-7950
correctness to the same engine Holo ships.

| | Cambium | goyang/ygot |
|---|---|---|
| `ordered-by user` round-trip | byte-exact, default | opt-in, late retrofit |
| nested / keyless ordered lists | yes | unsupported / historically broken |
| `must`/`when` XPath, `mandatory` | yes (libyang) | no ([#514](https://github.com/openconfig/ygot/issues/514)) |
| YANG 1.1 | full | gaps ([#51](https://github.com/openconfig/goyang/issues/51), [#190](https://github.com/openconfig/goyang/issues/190)) |
| Rust + Go, identical bytes | yes (CI-gated) | Go only |

## Quickstart

```rust
use cambium::{ContextBuilder, Format, ParseMode, ValidateMode, tree};
let ctx = ContextBuilder::new().search_path("yang").load_module("openconfig-interfaces", None)?.freeze();
let t = tree::parse(&ctx, Format::Json, ParseMode::DataOnly, &bytes)?;
t.validate(ValidateMode::ConfigOnly | ValidateMode::MultiError)?;
println!("{}", String::from_utf8(t.serialize(Format::Xml, Default::default())?)?);
```
```go
ctx, _ := cambium.NewContextBuilder().SearchPath("yang").LoadModule("openconfig-interfaces", nil).Freeze()
t, _ := cambium.Parse(ctx, cambium.FormatJSON, cambium.ParseModeDataOnly, bytes)
_ = t.Validate(cambium.ValidateModeConfigOnly | cambium.ValidateModeMultiError)
xml, _ := t.Serialize(cambium.FormatXML, cambium.SerializeWithSiblings)
```

> **v0.1 platforms:** Linux x86_64/arm64, macOS arm64. Windows is post-1.0.
> Vendors libyang + PCRE2 (BSD-3-Clause); no system libyang required.
```
