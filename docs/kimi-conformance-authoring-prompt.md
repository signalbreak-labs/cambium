# Work Order — Author the COMPLETE Cambium Conformance Corpus (all 189 fixtures)

You are Kimi, an autonomous overnight coding agent. This document is your entire
brief. Read it fully before touching the repo, then execute it end to end without
asking for further input.

Repo: `/Users/recursive/workspace/projects/github/signalbreak-labs/cambium`
(`signalbreak-labs/cambium` — an order-correct YANG toolkit/SDK for Rust + Go,
built on a vendored libyang).

---

## 0. THE GOAL (read this twice)

Deliver the **FULL** owned conformance fixture corpus — **all 189 fixtures** listed
in `docs/conformance-coverage-catalog.json` — into `/conformance`. Every fixture is
authored from scratch, libyang-golden-backed, yanglint-oracle-confirmed where the
construct supports it, and **byte-gated GREEN in BOTH the Rust and Go runners**.

This is **not** a subset, a floor, a "representative sample", or an MVP. The target
is the entire catalog. **Stopping at a representative subset is explicitly
forbidden.** Proceed theme-by-theme; each theme fully complete and green before the
next; commit per theme. If the session ends, the handoff names exactly which
themes/fixtures are done and which remain — but no fixture is ever left
half-authored, and the standing goal is all 189.

The committed corpus has **ZERO dependency on any external vendor repo**. Once you
are done, deleting any external YANG directory must not affect a single fixture.

---

## 1. ANTI-DRIFT / NON-GOALS (hard boundaries — violating these fails the work order)

- **No external corpus.** Do **NOT** reference, read, copy, or `searchdir` into
  `~/Downloads/yang` or any external/vendor YANG path. It may be deleted at any
  time. Work **ONLY** from `docs/conformance-coverage-catalog.json` (and its `.md`
  companion guide) plus `/spec`. Every `.yang` you ship is authored fresh and owned
  in-repo under `conformance/fixtures/<name>/` or `conformance/corpus/<name>/`.
- **No vendor `.yang` copies.** Do not paste a vendored module. RFC modules
  (e.g. RFC 6991 inet/yang-types for the single `real-typedef-imports` fixture) must
  be authored as a minimal, owned, self-contained module that exercises the needed
  types — not lifted wholesale from a vendor tree. If you must reproduce RFC type
  semantics, transcribe the minimal type definitions you need from the RFC text into
  a fresh owned module; do not copy a vendor file.
- **Cambium is a library/SDK.** No NETCONF (no `<edit-config>` envelopes, clients,
  device sinks), no gNMI/gRPC transport, no Terraform provider emitters. Those are
  downstream consumers, explicitly out of scope (see `AGENTS.md` § Non-goals).
  Generic ordered XML / JSON / JSON_IETF serialization **is** in scope and is the
  whole point.
- **No `~/Downloads` or vendor paths in any commit.**

---

## 2. BRANCH + COMMIT DISCIPLINE

- Work on branch **`feat/cambium-sdk-phase1`** (current HEAD `5a87690`). It is
  already checked out. Confirm with `git rev-parse --abbrev-ref HEAD` and
  `git rev-parse HEAD` before starting.
- **Commit per theme** using Conventional Commits, imperative subject ≤ 50 chars,
  e.g. `test(conformance): add builtin-scalar-types fixtures`. One theme per commit.
- **DO NOT push.** No `git push`, no PR. Local commits only.
- Never commit secrets, scratch, or `~/Downloads` artifacts. Use `.planning/`
  (gitignored) for scratch.

---

## 3. THE AUTHORITATIVE SOURCE & THE CONTRACT

### 3.1 The catalog (your worklist)
`docs/conformance-coverage-catalog.json` — **189 fixtures**, confirmed. Each entry:
```json
{
  "name": "types-int-int8-range",
  "theme": "builtin-scalar-types",
  "covers": "int8 (RFC 7950 §4.2.5) with range restriction ... -128/0/+127",
  "invariant": "validate + serialize-bytes(JsonIetf,Xml); pin i8 at -128, 0, 127",
  "yang_sketch": "container top { leaf i8-min { type int8; } ... }",
  "juniper_exemplar": "ietf-yang-types counter-style int typedefs"
}
```
- `name` → the fixture directory name (`conformance/fixtures/<name>/`).
- `theme` → grouping/commit key.
- `covers` → the RFC/construct under test.
- `invariant` → **what the fixture must demonstrably assert** (see §6).
- `yang_sketch` → **reference pseudocode, NOT binding.** Expand it into a real,
  complete, loadable module. Sketches use `...` ellipses as intentional compression
  (e.g. `ordering-composite-key-wide` stubs 5 of 6 keys; `wide-heterogeneous-
  siblings-all-types` stubs the 9+ children). **You expand them fully.**
- `juniper_exemplar` → context only. Do **not** go fetch the exemplar; author fresh.

The companion `docs/conformance-coverage-catalog.md` is a human-readable guide to the
same 189; read it for theme intent and the "fidelity traps" notes.

> **Stale-note guard:** the catalog JSON may carry an internal `completeness_note`
> prose string that says "175 de-duplicated fixtures across 14 themes". That note is
> **stale** and contradicts the catalog's own data. The authoritative target is
> **189 fixtures across 15 themes** (the per-theme counts below total exactly 189 —
> verified). Trust the fixture list and the counts below, not the prose note.

### Confirmed theme counts (must total 189):
```
 20  builtin-scalar-types      10  constraints-conditionals
 28  data-node-types            2  edge-illegality
  7  extensions-metadata        8  identifier-codegen
  4  identity                  15  json-ietf-serialization
 28  linkage                   18  operations
 10  ordering-invariants        1  real-typedef-imports
 17  reference-types           15  type-composition
  6  type-restrictions
```

### 3.2 The shared contract (`/spec`) — single source of truth, both languages
- `spec/api.md` — API + serializer + byte-gate contract.
- `spec/ordering-invariants.md` — **I1–I6**. Every ordering fixture must assert its
  invariant per this file (see §6).
- `spec/rule-codes.md` — the **CAMBIUM_E####** registry. Rejection fixtures assert
  these codes (see §5). Relevant codes:
  - `CAMBIUM_E0001` Context — module-not-found / bad searchdir / schema-load failure.
  - `CAMBIUM_E0002` Parse — malformed input, unknown schema (strict), bad encoding
    incl. interior NUL.
  - `CAMBIUM_E0003` Validate — RFC-7950 validation (`must`/`when`, `mandatory`,
    leafref, type restriction, min/max-elements, unique, range/length/pattern). The
    structured `Diagnostic` under the stable `E0003` carries `error_app_tag`,
    `data_path`, `schema_path`, `validation_code` as informational sub-fields.

PRs/fixtures that diverge from `/spec` fail review. If `/spec` is silent or ambiguous
on a needed detail, **the yanglint oracle is the tiebreaker** (§4).

---

## 4. THE YANGLINT ORACLE IS TRUTH (the single most important rule)

The vendored `yanglint` binary (built from `third_party/libyang`, exposed to the
Rust runner via the `CAMBIUM_YANGLINT` compile-time env from
`rust/cambium-libyang-sys/build.rs`) is the **independent engine authority**.

For every `oracle = true` fixture: **the committed golden MUST equal what yanglint
independently produces** for that module + input + format.

- If Cambium serializes differently from yanglint, **that is a CAMBIUM BUG.**
  - Do **NOT** edit the golden to match Cambium's (wrong) output.
  - Do **NOT** reorder/soften the fixture to hide the mismatch.
  - Do **NOT** silently skip the fixture.
  - **RECORD it** in the findings report (§10): fixture name, format, the exact
    Cambium-vs-yanglint diff, the YANG construct, and your root-cause hypothesis.
  - Then either (a) author the golden from yanglint's truth and leave the fixture
    failing-with-a-recorded-bug if Cambium can't match it yet, or (b) if you can
    safely fix the engine to match yanglint, do so under TDD (red test first) and
    note the fix in findings. **Prefer recording over fixing** unless the fix is
    small and obviously correct — do not go on an engine-refactor tangent that
    starves the corpus delivery. When in doubt: record, mark the fixture's status in
    the handoff, keep authoring the rest.
- Authoring this corpus is therefore **also a comprehensive engine bug-finding
  pass.** Every divergence you find is a deliverable, not a nuisance.
- The "fidelity traps" already known (replicate libyang exactly, these are NOT bugs):
  anyxml namespace-rewrite, import-before-revision statement order, and
  deviate-replace-default requiring a pre-existing default.

There is **no `--bless` / `--update-golden` mode** anywhere in the toolchain (the
`cambium-cli` is unimplemented). Goldens are authored by hand from the runner's
actual output cross-checked against yanglint, reviewed, then **frozen**. Review every
golden diff yourself before committing; never blind-accept. Once committed and
reviewed, a golden is immutable.

**How to capture a golden in practice.** The runner READS the golden file *before*
comparing — verified: `rust/conformance-runner/src/main.rs` does
`fs::read(&golden_path).map_err(|e| "read golden {}: {e}")` first. A **missing**
golden therefore produces a hard `read golden ...: No such file` error, **NOT** a
diff. So you must create the file first:
1. Author `module/` + `input.xml` (or `input.json`).
2. Add the `[[case]]` to `manifest.toml` with the `expected` golden paths pointing at
   files you are about to create. Create those golden files as **empty placeholders**
   first (`touch golden/<name>/output.xml` etc.) so the runner can read them.
3. Run the Rust runner (it has the oracle). With an empty placeholder golden the
   compare is `normalize(empty) != normalize(actual)`, which fails and prints the
   diff: the `--- actual (first 512 bytes) ---` half is Cambium's real serialized
   output. **Independently** run `yanglint -f <fmt> <schemas-sorted...> <input>` (the
   runner sorts schema files by name and passes the input last — mirror that
   exactly). The runner's `actual` bytes and the yanglint bytes **must agree** (modulo
   the normalize step: trailing-whitespace stripped). If they DISAGREE, that is a
   Cambium bug — §4 applies: record it, do not write Cambium's bytes into the golden
   to make the case pass.
4. Write the agreed bytes into the golden file. Re-run: Rust green + oracle green.
   (An empty golden NEVER silently passes — `empty != actual`, so a forgotten or
   blank golden always fails loudly. There is no path where a blank golden is
   accepted, and no out-of-band capture step that bypasses the yanglint cross-check.)
5. Run the Go runner on the same fixture: Go must produce **byte-identical** output
   (the I5 Rust==Go gate). Green in both = the golden is correct and frozen.

---

## 5. REJECTION FIXTURES = RULE-CODE PARITY UNITS (NOT byte-goldens)

The catalog's rejection cases (the E0001/E0002/E0003 fixtures — ~29 of the 189, the
ones whose `invariant` says `validate-reject(... -> E000x)` or
`schema-load-fails`/`parse-fails`) are **NOT** byte-golden manifest cases. They are
**per-language unit tests** that assert Rust and Go return the **same rule code** for
the same malformed input, per `spec/rule-codes.md`.

- Rust: add cases to `rust/cambium-core/tests/rule_codes.rs` (existing shape:
  build a `Context`, attempt the invalid op, assert
  `err.rule_code() == RuleCode::Context | RuleCode::Parse | RuleCode::Validate`).
- Go: add cases to `go/cambium/rulecode_test.go` (existing shape: attempt the op,
  `errors.As(err, &ce)`, assert
  `ce.RuleCode() == cambium.RuleCodeContext | RuleCodeParse | RuleCodeValidate`).
  (Verified constant names — do not invent variants.)
- Each rejection fixture's owned `module/` YANG still lives under
  `conformance/fixtures/<name>/` (the schema being loaded), and the **invalid data**
  (or invalid schema) is what triggers the code. The unit tests reference those owned
  modules; do not invent throwaway inline modules unless the schema itself is the
  thing being rejected (then inline is fine).
- **Parity is the assertion:** both languages emit the same `CAMBIUM_E000x` and,
  where libyang supplies one, the same `error_app_tag` and `data_path` (e.g.
  min-elements → "Too few", max-elements → "Too many"). Where the catalog names an
  app-tag, assert it in both languages.
- Do **not** create `golden/<name>/` byte files for pure rejection cases.

The 3 codes in play: E0001 (schema-load/context — e.g. `empty` with `default`,
non-transitive import of an unimported type), E0002 (parse — truncated/NUL XML,
strict-unknown, `json-ietf-decimal64-no-exponent` rejecting `1E-9` at parse),
E0003 (validation — range/length/pattern, leafref dangling, min/max-elements,
unique violation, mandatory missing, must/when false, choice-mandatory).

---

## 6. EVERY FIXTURE MUST DEMONSTRABLY ASSERT ITS INVARIANT

A golden is a **proof artifact**: a regression must change the bytes, or the fixture
is worthless. Design each fixture so a plausible bug flips the output.

The catalog's `invariant` strings already specify the scramble (e.g.
`pin rules c,a,b -> [c,a,b] not alphabetized`; `input zebra,apple,mango,banana ->
NOT alphabetized`). **You must actually implement that scramble in the `input`
file** — the order of elements in `input.xml`/`input.json` must differ from both the
schema declaration order AND alphabetical order. A fixture whose input is already in
schema order (or already alphabetical) proves nothing: a sort-by-name or
preserve-input bug would leave the bytes unchanged and pass silently. Before
committing any ordering fixture, confirm **by eye** that input order ≠ golden order ≠
alphabetical. If a fixture's invariant does not name a scramble but the construct has
an order (siblings, list entries, I/O nodes), scramble it anyway by default.

- **Ordering fixtures must be authored OUT of declaration order.** The input data
  must present children/entries in a *different* order than the schema (scramble:
  alphabetical, reverse, or random) so that a sort-by-name or preserve-input-order
  bug produces different bytes. Model on the existing `scrambled-children`:
  schema order `z, m, a`, input order `a, z, m`, golden order `z, m, a`.
- **I1 (`ordered-by user`)** — input the user-ordered list/leaf-list entries in a
  scrambled insertion order; the golden preserves that **exact insertion order**
  byte-for-byte (e.g. input `[c, a, b]` → golden `[c, a, b]`, never sorted).
- **I2 (system order / canonicalization)** — `ordered-by system` numeric keys/values
  input out of order; golden shows libyang's **canonical** sort, proving the engine
  re-canonicalizes on parse.
- **I3 (keys first)** — declare non-key leaves *before* the keys in schema text and
  make the `key` statement reorder them (model on existing `keys-first`: schema
  `description, name, class`, `key "name class"`, golden emits `name, class,
  description`). For wide composite keys (`ordering-composite-key-wide`, 6-leaf,
  non-contiguous) prove every key precedes every non-key.
- **I4 (RPC/action/notification I/O order)** — interleaved heterogeneous I/O nodes
  input scrambled; golden emits them in **schema declaration order**. Set the
  manifest `op-type` so the runner calls `ParseOp`. **The ONLY valid `op-type`
  values are `rpc`, `notification`, `reply`** (verified: `OpType` in
  `rust/cambium-core/src/tree.rs` has exactly `Rpc`/`Notification`/`Reply`, and
  `parseOpType()`/`parse_op_type()` in both runners accept only those three).
  **There is NO `action` op-type** — `op-type = "action"` errors
  `unknown op-type: action` and stalls the fixture. A YANG 1.1 `action` is parsed
  through the RPC path: use **`op-type = "rpc"`** for every `action-*` fixture
  (libyang parses an action's input/output via the same RPC-YANG parse type; the
  action is rooted at its parent data node). If an action fixture refuses to parse via
  `op-type = "rpc"`, that is a finding to RECORD (§10), **not** a reason to invent a
  new op-type or manifest field.
  I4 is the cross-cutting invariant of the `operations` theme.
- **I5 (JSON object/array determinism, Rust==Go)** — every byte-golden inherits this:
  the Go runner must produce **byte-identical** output to the Rust runner (both must
  use libyang's printer order, never language-native map/reflection order). The
  dedicated `json-object-determinism` fixture exists to make this explicit, but the
  gate applies to all byte-goldens transitively. A fixture is not done until both
  runners agree on the same bytes — this holds even for the `oracle = false`
  JsonIetf empty-container cases of §7.4.

Review every golden diff against the fixture's stated `invariant` from the catalog
before committing. Never blind-accept.

---

## 7. PER-FIXTURE DELIVERABLES & DONE-DEFINITION

### 7.1 What each positive (byte-golden) fixture ships
Under `conformance/fixtures/<name>/`:
- `module/` — a directory containing **one** owned `.yang` for self-contained
  fixtures, or **multiple** owned `.yang` files for
  augment/deviation/submodule/import-revision/cross-module fixtures (~31 fixtures
  need 2+ modules). For multi-module fixtures the manifest `module` field points at
  the **directory** and the runner's `load_modules_in_dir()` loads every `.yang`
  (sorted by name, stem-before-`@revision`); libyang resolves imports/includes from
  the searchdir. (The existing `ietf-interfaces` case uses `corpus/<name>/` for its
  module dir — you may mirror that layout for the larger multi-module fixtures, but
  `fixtures/<name>/module/` is the default and is preferred for owned single-purpose
  modules.)
- `input.xml` **or** `input.json` — the data instance, authored **out of order**
  per §6 where ordering is under test.
  **Path note (load-bearing):** the manifest `input` path is resolved from the
  `/conformance` root and is **independent** of the `module` path. The existing
  `ietf-interfaces` case proves the split — `module = "corpus/ietf-interfaces"` but
  `input = "fixtures/ietf-interfaces/input.xml"`. The runner does **not** look for
  `input` relative to `module`. So even when a multi-module fixture's modules live
  under `corpus/<name>/`, its `input.*` still belongs under `fixtures/<name>/`. Keep
  `input` under `fixtures/<name>/` and goldens under `golden/<name>/` **regardless**
  of where the modules live.
- Goldens under `conformance/golden/<name>/`: `output.xml`, `output.json`, and
  `output.json_ietf` **where applicable** (JSON_IETF for the
  `json-ietf-serialization` theme and any fixture whose `invariant` names JsonIetf).
- A `[[case]]` entry in `conformance/manifest.toml`:
  ```toml
  [[case]]
  name = "<name>"
  module = "fixtures/<name>/module"     # or "corpus/<name>" for big multi-module
  input = "fixtures/<name>/input.xml"   # ALWAYS under fixtures/<name>/, never under module/
  input-format = "xml"                   # or "json" / "json-ietf"
  op-type = "rpc"                        # ONLY rpc/notification/reply; actions use "rpc"; NO "action" value exists
  oracle = true                          # true wherever yanglint can cross-check
  [case.expected]
  xml = "golden/<name>/output.xml"
  json = "golden/<name>/output.json"
  json_ietf = "golden/<name>/output.json_ietf"   # where applicable
  ```
  Set `oracle = true` for every fixture where the construct supports a yanglint
  cross-check (the overwhelming majority). The **only** legitimate `oracle = false`
  cases are the JsonIetf empty-container fixtures of §7.4 — never use `oracle = false`
  to dodge a recorded mismatch, and always state the reason in the handoff.

### 7.2 PER-FIXTURE DONE (all must hold — nothing is "done" until both languages pass)
1. Module(s) **load** in both the Rust and Go runners.
2. Input **parses** in both.
3. Cambium serializes **byte-identically** to the committed golden in the **Rust**
   runner **AND** the **Go** runner (the I5 Rust==Go differential).
4. `oracle = true` ⇒ yanglint independently confirms the golden (Rust runner's oracle
   arm green).
5. The fixture **demonstrably asserts its invariant** per §6 (authored out of order;
   keys-first; ordered-by user; I/O order; JSON array/object order).

For rejection fixtures, "done" = both Rust and Go unit tests assert the **same**
`CAMBIUM_E000x` (and app-tag/data-path where named) per §5.

### 7.3 Existing fixtures are frozen
The 6 existing cases — `scrambled-children`, `keys-first`, `ordered-user`,
`rpc-order`, `system-list-canonical`, `ietf-interfaces` — **stay green and
untouched**. Do not modify their modules, inputs, goldens, or manifest entries. They
are your reference templates; read them, copy the *pattern*, never the *files*.

### 7.4 PREREQ: wire the JSON_IETF runner arm + the JSON_IETF oracle arm
`Format::JsonIetf` exists in `rust/cambium-core/src/tree.rs` (variant `JsonIetf`) and
in Go the constant is **`cambium.FormatJSONIETF`** — verify the exact identifier
in `go/cambium/cambium.go` before typing it. **It is `FormatJSONIETF`, NOT
`FormatJSON_IETF`** (no underscore): `FormatJSON_IETF` does not exist and the Go build
will fail to compile. Neither runner's `parse_format()` maps the `"json-ietf"` /
`"json_ietf"` string yet (verified: both `parse_format` arms are only `xml`/`json`).
Before authoring the `json-ietf-serialization` theme, add **three** arms (verify each
exact identifier in the file first):
- Rust `rust/conformance-runner/src/main.rs` `parse_format()`: add
  `"json-ietf" | "json_ietf" => Ok(Format::JsonIetf),`.
- Go `go/conformance/runner.go` `parseFormat()`: add the matching
  `case "json-ietf", "json_ietf": return cambium.FormatJSONIETF, nil`.
- Rust `run_yanglint_oracle()` format match. **This is the trap:** that function
  currently matches `Format::Xml => "xml", Format::Json => "json", _ => Err("unsupported
  oracle format")` (verified `main.rs` ~lines 170–172). The catch-all `_` arm means
  any `oracle = true` json-ietf fixture hits `unsupported oracle format` and the whole
  case **ERRORS** (it does not skip). You must add a **mandatory third arm**
  `Format::JsonIetf => "json",`. libyang's JSON printer (`yanglint -f json`,
  `LYD_FORMAT::LYD_JSON`) IS RFC 7951 / JSON_IETF, so `-f json` is the correct oracle
  for JsonIetf goldens. Wiring only `parse_format()` without this arm leaves every
  json-ietf oracle fixture dead on arrival.

**Caveat — the JsonIetf empty-container blind spot yanglint cannot oracle:**
`Format::JsonIetf` differs from `Format::Json` only by `LYD_PRINT_EMPTY_CONT`
(verified `rust/cambium-libyang-sys/src/adapter.rs` ~lines 1303-1304, 2136: JsonIetf
== `LYD_JSON | LYD_PRINT_EMPTY_CONT`). It preserves empty non-presence containers
that bare `-f json` drops. A bare `yanglint -f json` does NOT pass `LYD_PRINT_EMPTY_CONT`,
so for any json-ietf fixture that **emits an empty non-presence container** (notably
`json-ietf-presence-vs-nonpresence`, and any fixture whose golden shows an empty `{}`
container), Cambium's JsonIetf output will **legitimately** differ from the bare
yanglint oracle for a **non-bug** reason. For those specific fixtures only:
- set `oracle = false`,
- still byte-gate Rust==Go (§6 I5 still applies),
- and **state the reason verbatim in the handoff**:
  `"JsonIetf empty-container preservation (LYD_PRINT_EMPTY_CONT) not expressible via
  bare yanglint -f json"`.

This is the *only* legitimate `oracle = false`. It is explicitly **not** a way to hide
a real mismatch — do not record it as a Cambium bug (it is expected), and do not use
the same `oracle = false` reason for any other fixture. Every other json-ietf fixture
keeps `oracle = true`.

This whole §7.4 is a prerequisite, not a feature expansion. Commit it as part of (or
just before) the `json-ietf-serialization` theme.

### 7.5 Register fixtures in the Go `enabled` set
The Go runner runs a curated `enabled []string` declared in
`go/cmd/cambium/main.go` (`var enabled = []string{...}`); the bare
`go run ./cmd/cambium` runs that curated set, while `go run ./cmd/cambium all` runs
every manifest case. As you complete each fixture, add its name to that `enabled`
slice so the curated run covers it. (CI may run `all`; keeping `enabled` complete
keeps the default run honest.)

---

## 8. THEME-BY-THEME DELIVERY ORDER (self-contained first, linkage last)

Author in this order. **Each theme fully complete + green in both runners before the
next. Commit per theme.** Resume from the exact fixture name if interrupted.

1. **builtin-scalar-types (20)** — int/uint 8–64, decimal64 fd1–18, bool, empty,
   enum, bits, binary; JSON quoting/canonical boundaries (int64 quoted, int32 bare).
2. **type-restrictions (6)** — invert-match patterns (YANG 1.1), multiple-patterns
   AND, length+pattern+POSIX, min/max keywords, range/length rejection.
3. **type-composition (15)** — typedef (1/2/3-deep, narrowing, default-inheritance,
   submodule cross-file, union composition); union (heterogeneous, all-scalars,
   resolution-order, nested-typedef, leafref/identityref member, two-bases,
   enum+scalar).
4. **reference-types (17)** — leafref (absolute/relative/current()/to-key/to-leaf-
   list/deref/require-instance-false/cross-module/chaining); identityref
   (single/multiple-base, derived-hierarchy, foreign-module, iana-if-type scale);
   instance-identifier (require-default/no-require/complex-path).
5. **identity (4)** — standalone, cross-module-derivation, multi-base-cross-module,
   hierarchy + identityref-filtering.
6. **constraints-conditionals (10)** — when (OR, XPath fns derived-from/re-match/
   count/boolean/descendant), must (conjunction, error-message/app-tag), unique
   (single/composite/descendant + violation), mandatory (default/when/refine
   interaction), default (leaf/leaf-list/choice + with-defaults modes), feature/
   if-feature (and/or/not, transitive, presence-gating), + E0003 rejections.
7. **data-node-types (28)** — presence container, nested depth (I1), list (single/
   composite/keyless), choice (cases+default, mandatory-reject, nested, interleaved,
   leaf-list branch, shorthand auto-wrap), config true/false/mixed, status, anydata/
   anyxml fidelity, min/max-elements reject, declaration-order non-alphabetical (I1),
   wide heterogeneous siblings (I1).
8. **ordering-invariants (10)** — the explicit I1/I2/I3/I5 fixtures, authored maximally
   scrambled so a sort bug regresses every one.
9. **json-ietf-serialization (15)** — **(do §7.4 first)** namespace qualification,
   int-span quoting, decimal64 canonical/trailing-zeros, string escape/unicode,
   leaf-list arrays, list arrays+keys-first, nested containers, choice
   case-transparency, presence vs non-presence, anydata/anyxml, with-defaults modes,
   instance-identifier as string, leafref/union resolved-form, cross-module augment/
   deviation/when, JSON_IETF parse-roundtrip (`input-format = "json"` /
   `"json-ietf"`).
10. **identifier-codegen (8)** — Rust keyword `r#` prefix, Go keyword mangle, hyphen/
    underscore collision, enum-value collision, container-vs-leaf collision,
    leading-digit, >60-char name, unicode/CJK mixed-case. (These assert codegen
    naming; where a fixture has a data instance, byte-gate it; where it asserts only
    a generated name, assert via the codegen path per `spec/api.md`.)
11. **extensions-metadata (7)** — extension def+usage, yin-element modes, vendor
    passthrough (opaque), unknown-opaque preservation, extension-typedef collision,
    RFC 7952 metadata annotations, yang-version/units/status/description/reference
    metadata. (Author owned vendor-style extensions; do NOT copy Juniper modules —
    invent a fresh extension namespace.)
12. **real-typedef-imports (1)** — RFC 6991 inet/yang-types round-trip (ip-address
    union/zone, ipv6-prefix, mac-address, date-and-time, counter64). Author a
    **minimal owned** module reproducing only the RFC type definitions you exercise;
    do not copy a vendor `ietf-inet-types.yang`.
13. **linkage (28)** — grouping (simple/nested-uses/config+state/cross-module),
    refine (default/mandatory+config/presence+must/min-max+if-feature), augment
    (intra/inter-module, choice new-case, nested, when-reach-up, cross-module ident
    collision), deviation (not-supported/replace-type/add/delete/multi/replace-
    default+config), import (prefix/revision-date/multiple/non-transitive E0001),
    submodule (simple/multi/imports-foreign), identity cross-module. **Most
    multi-module fixtures live here.**
14. **operations (18)** — RPC (input-only/output-only/interleaved/heterogeneous/
    nested/anyxml/decimal64, I4), action (YANG 1.1 simple/in-list/nested/
    heterogeneous/wide-siblings, I4 — **all use `op-type = "rpc"`** per §6),
    notification (top-level/nested-container/nested-list/container+leaf-list/
    interleaved, I4 — `op-type = "notification"`), rpc-action-notification
    coexistence (I4).
15. **edge-illegality (2)** — `parse-malformed` (E0002: truncated XML, NUL, strict-
    unknown) and `schema-load` (E0001: empty+default, 1.0 leaf-list-of-empty) — these
    are rejection units (§5), not byte-goldens.

(The rejection cases scattered through themes 2, 6, 7, 13, 14 are authored as
rule-code parity units per §5 alongside their theme; commit them with their theme.)

### Empirical libyang flags you will need (per the catalog's flag notes)
- `LYD_PARSE_OPAQ` — anydata/anyxml opaque passthrough, anyxml namespaced attrs,
  unknown-extension opaque preservation, JSON_IETF parse-roundtrip, rpc-io-with-anyxml.
- `LYD_VALIDATE_NO_STATE` — `ordered-user-config-false-state` (config-false state
  data, user-order preserved across parse).
- with-defaults / RFC 6243 modes — `constraints-default-*`, `leaflist-with-defaults`,
  `types-boolean-default-false`, `json-ietf-with-defaults-modes` (explicit / trim /
  report-all / report-all-tagged).
- strict mode (no OPAQ) — `parse-malformed-e0002` unknown-element rejection.
- feature bitmask context — `constraints-feature-*` (enable/disable feature sets).
Drive these through Cambium's existing parse/validate/serialize API
(`spec/api.md`); do not add new public API for them unless `spec/api.md` already
defines it. If a flag is needed but not exposed, record it in findings (do not invent
a manifest field for it).

---

## 9. BUILD / TEST / LINT GATES (run before every theme commit)

Both languages must be green. Run from the repo root.

**Rust**
```
cargo build --workspace
cargo test --workspace
cargo run -p conformance-runner        # byte-gate + oracle, all manifest cases
cargo clippy --workspace --all-targets -- -D warnings
cargo fmt --all
```
**Go**
```
CGO_ENABLED=1 go build ./...
go test ./...
go run ./cmd/cambium all               # byte-gate, all manifest cases
go vet ./...
golangci-lint run
```
A theme is committable only when: all its fixtures are green in **both** runners
(byte-identical, oracle-confirmed where `oracle = true`), all rejection units pass in
both languages, clippy/vet/lint clean, fmt applied, and the existing 6 cases still
green.

Forbidden in this codebase per house style: `unwrap()`/`expect()` outside tests
(Rust); string-matching `err.Error()` (Go) — assert on `RuleCode()`. The conformance
runners only **read and assert** goldens; they never write them.

---

## 10. HANDOFF / FINDINGS REPORT (your final output)

At session end, emit a structured handoff (in your final message and/or
`.planning/conformance-handoff.md`) containing:

1. **Themes delivered** — per theme, `N/N` fixtures green in both runners, with the
   commit SHA. E.g. `builtin-scalar-types [20/20] ✓ (commit abc1234)`.
2. **In-progress** — the theme and the **exact current fixture name** so the next
   session resumes precisely (never restarts a theme). No fixture left half-authored:
   a fixture is either fully delivered or not started.
3. **Cambium-vs-yanglint mismatches (the bug list)** — every divergence found, with:
   fixture name, format (xml/json/json_ietf), the YANG construct, the exact byte
   diff (or a tight snippet), and your root-cause hypothesis. This is a primary
   deliverable, not an appendix. **Do not list the JsonIetf empty-container
   `oracle = false` cases here** — those are expected (§7.4), not bugs.
4. **Deferred fixtures** — any fixture not delivered, **named individually with the
   reason** (blocker, missing engine support, oracle limitation). **No silent
   skips.** "Representative subset" is not an acceptable reason.
5. **Legitimate `oracle = false` list** — the JsonIetf empty-container fixtures, each
   with the mandated reason string from §7.4, kept separate from the bug list and the
   deferred list.
6. Confirmation the existing 6 fixtures are still green and untouched.

Target restated: **all 189**. The bug list, the deferred list, and the legitimate
`oracle = false` list together must account for every fixture in the catalog that is
not a plain green `oracle = true` byte-gate.

---

## 11. QUICK START CHECKLIST

1. `git rev-parse --abbrev-ref HEAD` ⇒ `feat/cambium-sdk-phase1`; `git rev-parse HEAD`
   ⇒ `5a87690…`.
2. Read `docs/conformance-coverage-catalog.json` (+ `.md`), `spec/ordering-invariants.md`,
   `spec/rule-codes.md`, `spec/api.md`. (Ignore the stale `14/175` completeness note —
   the target is 189/15 per §3.1.)
3. Read the 6 existing fixtures + goldens + `conformance/manifest.toml` as templates.
4. Build both runners green (baseline) before authoring anything.
5. Author theme 1 (`builtin-scalar-types`) fixture by fixture: module → input
   (out-of-order where ordering matters) → manifest entry → `touch` placeholder
   golden → capture golden from runner + yanglint agreement → write agreed bytes →
   Rust green + oracle → Go byte-identical → add to Go `enabled` → mark done.
6. When all 20 are green in both runners: `cargo fmt`, clippy, vet, lint; commit
   `test(conformance): add builtin-scalar-types fixtures`.
7. Repeat for themes 2–15 in order. Do §7.4 (JSON_IETF + oracle arms) before theme 9.
8. Record every oracle mismatch as you go. Never bend a golden, never silent-skip,
   never set `oracle = false` except the §7.4 empty-container case.
9. End: emit the handoff (§10). Do **not** push.

Go deliver the whole catalog.
