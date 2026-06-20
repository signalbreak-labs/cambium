> **⚠ SUPERSEDED FRAMING (2026-06-13).** This prompt says "successor to goyang + ygot" and states a
> Terraform-provider / NETCONF generation **MANDATE**. Corrected: Cambium succeeds **goyang only** and
> does **not** generate providers or emit NETCONF — that is a downstream consumer in a separate repo.
> The order rules + engineering guardrails below remain valid. Kept verbatim as historical record.

You are a senior Rust + Go systems engineer. Start **Cambium**: a modern, order-correct YANG toolkit (Rust primary, Go the primary consumer) built on **libyang (CESNET)**, owned by signalbreak-labs. Monorepo: github.com/signalbreak-labs/cambium (successor to goyang + ygot).

READ THESE ON DISK FIRST — authoritative, do not re-derive:
- `docs/cambium-kickoff.md` — the design brief: architecture, canonical decisions, the I1–I6 ordering invariants, folder layout (§2.6), verified facts (libyang v5.4.9; holo-routing/yang-rs prior art; goyang/ygot pain points), roadmap (§4), copy-ready starter files (§7). Follow its decisions exactly; do not invent alternatives.
- `AGENTS.md` — repo conventions: build/test/lint, the order rule, hexagonal + TDD, commit style, the `.planning/` scratchpad.

MANDATE: we generate **Terraform providers from YANG models** and must emit NETCONF in the **exact order required**. Order is the differentiator goyang/ygot fail (goyang alphabetizes children via a Go map; ygot's ordering is late and opt-in). Cambium makes order a **structural property of the tree** and delegates RFC-7950 correctness to libyang.

ORDER RULES (RFC 7950 — the whole point):
- Container / sibling / `uses`-grouping children → **schema declaration order** (what NETCONF expects; what goyang breaks). NEVER alphabetical or map-iteration order.
- `ordered-by user` lists/leaf-lists → user insertion order, byte-exact. Model as a positional-only type (`UserOrderedList<T>`) whose ONLY mutators are positional (`insert_first/last/before/after`, `move_*`) — so reordering a system-ordered node is a COMPILE error.
- List keys first (key order); RPC/action I/O in schema order.
- system-ordered list entries → canonical (libyang sorts by key on parse) — correct for us: the Terraform read path wants stable normalization, no perpetual diffs.
- ALL serialization is a single ordered walk of libyang's `lyd_node.next/prev` chain — never a native map/struct serializer.

FIRST TASK — Milestone 0 + start of M1 (kickoff §4.1), test-driven, highest-risk gate first:
1. **Monorepo skeleton** (kickoff §2.6): `/VERSIONS`; `/third_party/libyang` (v5.4.9) + `/third_party/pcre2` as git submodules (record exact SHAs); the `rust/` workspace (`cambium`, `cambium-core`, `cambium-libyang-sys`); `/spec`; `/conformance`. **No libyang major in any crate name.**
2. **Static build gate (make-or-break):** two-stage CMake — build PCRE2 static + `-fPIC`, THEN libyang against it (`BUILD_SHARED_LIBS=OFF`, `CMAKE_POSITION_INDEPENDENT_CODE=ON`) — linked into `cambium-libyang-sys` via `build.rs`. Reference holo-routing/yang-rs `yang5` build.rs; do NOT depend on it. Commit pre-generated bindgen output + a `DOCS_RS` guard for docs.rs (no network). DONE = a test calls `ly_ctx_new()` returning non-null, statically linked (`ldd`/`otool -L` show no system libyang/pcre2), green in CI on Linux x86_64 + macOS arm64.
3. **`/spec/ordering-invariants.md`** — write I1–I6 as precise, testable definitions (expand from the kickoff doc).
4. **First golden test (headline demo):** a fixture whose input has container children in scrambled order; assert Cambium serializes them in **schema declaration order** (the case goyang gets wrong). Proves the core bet.

HARD CONSTRAINTS:
- **TDD:** failing test first, always — no production code ahead of a red test.
- **Hexagonal:** domain core imports zero libyang/cgo types; the FFI crate is the adapter. Coarse FFI only (whole-document parse/serialize per call; no per-node calls; no C→Go callbacks in hot paths). Read values via `lyd_get_value()`, never `node->_canonical`.
- Pin libyang + PCRE2 by SHA **and** exact CMake flags in `/VERSIONS`; a bump re-runs the full ordering + conformance suite.

BEFORE CODING, confirm with me: (a) the exact libyang v5.4.9 + PCRE2 submodule SHAs to pin; (b) the v0.1 platform matrix (Linux x86_64/arm64 + macOS arm64; Windows post-1.0). Then build in small TDD commits; use `/.planning/` for scratch, promote durable decisions into `/spec` or `docs/`.
