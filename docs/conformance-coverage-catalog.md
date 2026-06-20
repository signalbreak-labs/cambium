# Cambium conformance coverage catalog
The exhaustive, **owned** fixture build spec for `/conformance` — covering the full RFC 7950 + RFC 7951 (+ 7952, 6243, 6991) YANG surface. Derived by mining the Juniper Junos 25.4R1 corpus **once** as a reference, then authoring every fixture from scratch. The committed corpus has **zero runtime dependency** on the vendor repo.
- **189 fixtures** across **15 themes**; **195** construct→fixture coverage rows.
- Machine-readable spec (with YANG sketches): `docs/conformance-coverage-catalog.json`.

> **Authoritative count (updated 2026-06-20):** this catalog is the *authoring
> spec* (189 enumerated fixture descriptions). The **live source of truth is
> `/conformance/manifest.toml`**, which currently has **200 cases** — **7
> `schema-ir`** + **193 `backend-data`** (201 fixture directories). The "193
> manifest cases" figure later in this file is historical; regenerate counts
> from the manifest, not from this prose.

## Completeness & scope

MERGED CATALOG: 175 de-duplicated, owned fixtures across 14 themes (builtin-scalar-types, type-restrictions, reference-types, type-composition, data-node-types, ordering-invariants, linkage, identity, operations, constraints-conditionals, edge-illegality, json-ietf-serialization, extensions-metadata, identifier-codegen, real-typedef-imports). This is the union of the prior 31-fixture census, the 8 proposed slices, and the 27-fixture gap-fill, with every duplicate collapsed to ONE canonical fixture while preserving every distinct construct/edge.

DEDUP DECISIONS (no coverage dropped — duplicates folded into the richer single fixture):
- leafref/identityref/instance-identifier appeared in BOTH the builtin-types slice and the typedef-chains slice plus the base catalog. Collapsed to one fixture per distinct path/target/require-instance/base-cardinality variant (12 leafref+identityref+instance-id fixtures total) rather than the ~20 near-duplicates across sources.
- union: base catalog (types-union-breadth/-resolution/-typedef-chain) + builtin slice + typedef slice all proposed overlapping union fixtures. Collapsed to 8 union fixtures covering heterogeneous quoting, all-scalar-members, resolution-order, nested-typedef-chain+binary, leafref-member, identityref-member, two-distinct-base-identityrefs, enum+scalar. The base catalog's three are subsumed (noted in each fixture).
- identityref-single-base and types-identityref-same-module from two slices collapsed into types-identityref-single-base; the foreign/cross-module variants collapsed into types-identityref-foreign-module-prefix + identity-cross-module-derivation + identityref-iana-if-type-foreign (kept distinct because owned-tiny vs census-scale-foreign exercise different encoding scale).
- choice/case: data-node slice + base catalog (ordering-choice-case/-nested) + json-ietf slice overlapped. Schema-side ordering lives in data-node-types fixtures (8), JSON-side case-transparency in one json-ietf fixture; base catalog's two are subsumed.
- decimal64: base (fd1/2/9/18) + builtin (fd1/2/9/18) + gap (fd3/6) + json-ietf (canonical quoting) reconciled into 5 type fixtures + 1 json-ietf canonical-quoting fixture (no overlap: schema/range vs JSON-byte concerns).
- numeric quoting: base types-numeric-decimal64 quoting concern folded into json-ietf-scalar-quoting-int-spans (the 32->64-bit boundary) to avoid two fixtures asserting the same RFC 7951 §6.1 rule.
- augment/deviation/grouping/submodule/import/refine: linkage slice + base catalog + gap-fill heavily overlapped; the base catalog's linkage-augment/-deviation/-refine/-submodule/-grouping-uses are each subsumed by the finer-grained slice fixtures (noted inline). linkage-deviation's 5-in-one split into not-supported/replace-type/add/delete/multi + the gap's replace-default-config.
- idents: base idents-keywords-wide split into idents-keywords-rust + -go; idents-pathological split into idents-long-name + idents-container-leaf-collision; idents-augment-collision -> augment-cross-module-ident-collision.

HARNESS RECONCILIATION (carried from the prior census, re-verified): byte-goldens go in [case.expected] as xml/json/json_ietf (the json_ietf runner arm is a one-line prereq the prior catalog already assumed; Format::JsonIetf exists in tree.rs; the Go runner reads [case.expected] as a generic map already). Reject cases (CAMBIUM_E0001 schema-load, E0002 parse, E0003 validate + error-app-tag + data-path) and codegen-compile are NOT byte-goldens — they are per-language Rust+Go unit tests asserting rule-code PARITY per spec/rule-codes.md §Mapping rule. json-ietf-parse-roundtrip is the first input-format='json' fixture. Every byte-golden inherits the Rust==Go differential gate (I5) anchored by json-object-determinism. Goldens are GENERATED from the real libyang printer and reviewed, never hand-authored from RFC text (the gap-fill empirically verified each sketch + each rejection against the pinned yanglint, including the three fidelity traps: anyxml namespace-rewrite, import-before-revision statement order, and deviate-replace-default requiring a pre-existing default).

GENUINELY OUT OF SCOPE (with reasons):
1. gNMI / ordering-invariant I6 (atomic SetRequest vs N-scalar-updates, TypedValue): the gNMI codec is Block 2 per kickoff §4.2; no SetRequest/TypedValue path exists (Format only has Xml/Json/JsonIetf). The JSON_IETF array/atomic SHAPE is already locked by leaflist/nested-user fixtures; only the gNMI wire-envelope assertion is deferred. This is the one ordering invariant (of I1–I6) not covered, by design.
2. Vendor-extension SEMANTIC evaluation (junos:must / junos:posix-pattern / junos:cli-feature behavior): non-XPath, vendor-specific, downstream-consumer concern per AGENTS.md Non-goals. Cambium only PASSES THROUGH vendor extensions opaquely — that pass-through IS covered (vendor-extension-junos-passthrough, extension-unknown-opaque-passthrough); evaluating their semantics is out of scope.
3. NETCONF transport envelopes (<edit-config>, confirmed-commit, rollback), gNMI/NETCONF/gRPC session opening, and Terraform-provider emission: explicit AGENTS.md Non-goals — downstream generation-system repo, not a Cambium deliverable.
4. Fidelity-mode byte-exact replay of arbitrary ordered-by-system DEVICE order: descoped per spec §3 (system order is canonical, not replayed).
5. Unicode normalization (NFC/NFD) and identifier translation (snake_case/kebab-case in codegen) beyond RFC 7950 NameStartChar acceptance: Cambium emits identifiers as-declared; normalization is not a YANG-spec concern.
6. Reflection-based / ygot-style codegen patterns: Cambium codegen is YANG-source-driven, not runtime-reflection; not a conformance-observable surface.

Everything in RFC 7950 (data node types, all 14 builtin types + every restriction, typedef/grouping/uses/refine/augment/deviation/submodule/import, identity, rpc/action/notification, when/must/unique/mandatory/default/feature/if-feature/extension, status/config, yang-version 1.0+1.1), RFC 7951 (§5 structure, §6 every scalar JSON encoding, case-transparency, parse direction), RFC 7952 (instance metadata annotations), RFC 6243 (all four with-defaults modes), and RFC 6991 (real typedef round-trip) is covered by at least one fixture, and every ordering invariant I1–I5 has a dedicated fixture (I6 deferred per kickoff roadmap above).

## Themes

| Theme | Fixtures |
|---|---|
| data-node-types | 28 |
| linkage | 28 |
| builtin-scalar-types | 20 |
| operations | 18 |
| reference-types | 17 |
| type-composition | 15 |
| json-ietf-serialization | 15 |
| ordering-invariants | 10 |
| constraints-conditionals | 10 |
| identifier-codegen | 8 |
| extensions-metadata | 7 |
| type-restrictions | 6 |
| identity | 4 |
| edge-illegality | 2 |
| real-typedef-imports | 1 |

## Coverage matrix (construct → fixture)

| YANG construct | Fixture |
|---|---|
| int8 builtin type (RFC 7950 §4.2.5) | types-int-int8-range |
| int16 builtin type | types-int-int16-range |
| int32 builtin type + multi-part range + gap exclusion + JSON-bare (RFC 7951 §6.1) | types-int-int32-range-multipart |
| int64 builtin type + JSON string quoting (RFC 7951 §6.1) | types-int-int64-range-quoted |
| uint8 builtin type + range | types-uint-uint8-range |
| uint16 builtin type (port idiom 1..65535) | types-uint-uint16-range-port |
| uint32 builtin type (297K uses) + multi-part range + JSON-bare | types-uint-uint32-range-multi |
| uint64 builtin type + JSON string quoting | types-uint-uint64-range-quoted |
| decimal64 fraction-digits 1 (tenths) | types-decimal64-fraction1-range |
| decimal64 fraction-digits 2 + canonical rounding (RFC 7950 §9.3.4) | types-decimal64-fraction2-canonical-round |
| decimal64 fraction-digits 3..8 mid-range span | types-decimal64-fraction3-and-6 |
| decimal64 fraction-digits 9 (nanoseconds) + negative | types-decimal64-fraction9-negative |
| decimal64 fraction-digits 18 (max) + max-magnitude | types-decimal64-fraction18-max-magnitude |
| boolean builtin type + default true/false | types-boolean-default-false |
| empty builtin type + [null] JSON (RFC 7951 §6.9) + empty XML element | types-empty-leaf-null-json |
| enumeration with sparse explicit values 0/4/6 + name round-trip | types-enumeration-explicit-values-sparse |
| enumeration with explicit zero value (disabled idiom) | types-enumeration-zero-value-disabled |
| enum/bit AUTO-position continuation after explicit value (RFC 7950 §9.6.4/§9.7.4) | types-enum-bits-auto-position |
| bits builtin type + explicit positions/gaps + space-separated declaration-order serialization (RFC 7951 §6.5) | types-bits-explicit-positions-gaps |
| binary builtin type + length + base64 JSON (RFC 7951 §6.6) | types-binary-length-base64 |
| pattern modifier invert-match (CORE RFC 7950 1.1 §9.4.6) | types-string-pattern-modifier-invert-match |
| string multiple patterns AND-composed conjunction | types-string-multiple-patterns-conjunction |
| string length + pattern anchors (^,$) + POSIX classes | types-string-length-pattern-anchor-posix |
| range/length out-of-bounds rejection (CAMBIUM_E0003) | constraints-range-length-reject |
| range/length 'min'/'max' grammar keywords resolving to type extremes (RFC 7950 §9.2.4/§9.4.4) | types-range-length-min-max-keywords |
| decimal64 exponent-form input rejected on parse (RFC 7951 §6.1 no-exponent, CAMBIUM_E0002) | json-ietf-decimal64-no-exponent |
| leafref absolute path + unresolved rejection (RFC 7950 §9.9) | types-leafref-absolute-path |
| leafref relative ../ path (RFC 7950 §6.4.1) | types-leafref-relative-parent-path |
| leafref current() context function (RFC 7950 §10.1.1) | types-leafref-current-context |
| leafref targeting list key | types-leafref-to-list-key |
| leafref targeting leaf-list member | types-leafref-to-leaf-list |
| leafref-to-leafref chain | types-leafref-to-leafref-chain |
| leafref deref() XPath function (YANG 1.1) | types-leafref-deref-function |
| leafref require-instance false (forward reference) | types-leafref-require-instance-false |
| leafref cross-module prefixed path + dangling reject (RFC 7950 §9.9.2) | types-leafref-cross-module |
| identityref single base + same-module bare (RFC 7951 §6.8) | types-identityref-single-base |
| identityref multiple bases (YANG 1.1, RFC 7950 §6.4.16) | types-identityref-multiple-bases |
| identityref multi-level derived hierarchy | types-identityref-derived-hierarchy |
| identityref foreign-module prefixed JSON (RFC 7951 §6.8) | types-identityref-foreign-module-prefix |
| identityref to realistic large foreign base (iana-if-type) | identityref-iana-if-type-foreign |
| instance-identifier require-instance true (default) + validation (RFC 7950 §9.13) | types-instance-identifier-require-default |
| instance-identifier require-instance false | types-instance-identifier-no-require |
| instance-identifier complex path with composite key + predicates | types-instance-identifier-complex-path |
| typedef simple base (RFC 7950 §7.3) | types-typedef-simple-base |
| typedef chain 2-deep + restriction inheritance | types-typedef-chain-2deep |
| typedef chain 3+ deep | types-typedef-chain-3deep |
| typedef restriction narrowing beyond base | types-typedef-restriction-narrowing |
| typedef default substatement inheritance vs leaf override | types-typedef-default-inheritance |
| typedef that is a union (composition) | types-typedef-union-composition |
| typedef defined in submodule used cross-file | types-typedef-submodule-cross-file |
| union heterogeneous members + per-member JSON quoting (RFC 7950 §4.2.12, RFC 7951 §6.1/§6.3) | types-union-heterogeneous-members-quoting |
| union containing ALL scalar base kinds | types-union-scalar-all-members |
| union member resolution order (first-match, RFC 7950 §9.12) | types-union-member-resolution-order |
| union nesting (union-of-union typedef) + typedef-of-typedef + binary member base64 | types-union-nested-typedef-chain |
| union with leafref member | types-union-leafref-member |
| union with identityref member | types-union-identityref-member |
| union of two identityrefs with distinct bases | types-union-two-identityrefs-distinct-bases |
| union of enumeration + scalar | types-union-enum-and-scalar |
| presence container {} + non-presence distinction (RFC 7950 §7.5.1) | container-presence-empty |
| nested container hierarchy 3+ levels (I1) | container-nested-depth |
| list single string key | list-single-key-string |
| list single numeric key | list-single-key-numeric |
| list composite key 2 leaves (I3) | list-composite-key-two |
| list composite key 3 leaves interleaved (I3) | list-composite-key-three |
| list composite key 6 leaves non-contiguous (I3 wide) | ordering-composite-key-wide |
| composite key with interleaved non-key containers (I3) | composite-key-with-interleaved-containers |
| keyless list positional order + max-elements | list-keyless-positional |
| list ordered-by user insertion order (I1) | list-ordered-by-user-insertion |
| list ordered-by system key canonicalization (I2 first half) | list-ordered-by-system-canonical |
| leaf-list ordered-by user (I1, RFC 7951 §5.4) | leaflist-ordered-by-user |
| leaf-list ordered-by system value canonicalization (I2 second half) | leaflist-ordered-by-system |
| leaf-list multiple default values | leaflist-with-defaults |
| cascading nested ordered-by user (list + inner leaf-list) | ordering-nested-user-cascading |
| positional keyless list under config false (state user-order crossing) | ordered-user-config-false-state |
| min-elements count constraint rejection (RFC 7950 §7.7.5) | min-elements-reject |
| max-elements count constraint rejection (RFC 7950 §7.7.6) | max-elements-reject |
| choice multiple cases + default-case + JSON case-transparency (RFC 7950 §7.9, RFC 7951 §5.2) | choice-multiple-cases-default |
| mandatory choice rejection (RFC 7950 §7.9.4) | choice-mandatory-reject |
| choice single-node case branches | choice-single-node-case |
| nested choice inside case + generic case names + JSON transparency at depth | choice-nested-in-case |
| choice interleaved with plain siblings (I1) | choice-cases-interleaved-siblings |
| choice case containing ordered-by-user leaf-list | choice-with-leaflist-branch |
| choice shorthand case (direct leaf-list + list, implicit case synthesis, RFC 7950 §7.9.2) | choice-shorthand-leaflist-list |
| list entry containing a choice (I3 + choice ordering) | list-entry-with-choice-schema-order |
| container within list entry (I3) | container-within-list-schema-order |
| leaf-list within list entry (cascading I1/I2) | leaf-list-within-list-entry |
| config true subtree (RFC 7950 §7.21.1) | config-true-subtree |
| config false state subtree | config-false-state-subtree |
| mixed config/state nested (config inheritance) | mixed-config-state-nested |
| status current/deprecated/obsolete (RFC 7950 §7.21.2) | status-current-deprecated-obsolete |
| anydata untyped structured data (YANG 1.1, RFC 7950 §7.10, RFC 7951 §5.9) | anydata-untyped-container |
| anyxml opaque verbatim passthrough (RFC 7950 §7.11) | anyxml-opaque-passthrough |
| anyxml attributes + namespaced children fidelity (RFC 7951 §5.10) | anyxml-attributes-namespaced |
| non-alphabetical schema order preservation (I1, schema-declaration-order case) | declaration-order-out-of-alphabetical |
| wide heterogeneous sibling declaration order (I1) | wide-heterogeneous-siblings-all-types |
| I5 JSON object member determinism + Rust==Go byte-diff | json-object-determinism |
| grouping + uses expansion at schema position, inlined (RFC 7950 §7.12/§7.13) | linkage-grouping-simple |
| grouping containing nested uses (transitive reuse) | linkage-grouping-nested-uses |
| grouping reused by config+state (inline copy, two distinct structs) | linkage-grouping-config-state |
| grouping referenced cross-module with prefix | linkage-grouping-cross-module |
| refine altering default (RFC 7950 §7.13.2) | linkage-refine-default |
| refine altering mandatory + config | linkage-refine-mandatory-config |
| refine adding presence + must | linkage-refine-presence-must |
| refine altering min/max-elements + adding if-feature | linkage-refine-min-max-iffeature |
| augment intra-module + when guard + compiled placement (RFC 7950 §7.17) | linkage-augment-intra-module |
| augment carrying container + leaf + leaf-list (mixed kinds) | linkage-augment-container-leaf-list |
| augment of choice (new case) + of existing case (new leaf) | linkage-augment-choice-case |
| augment of an augmented node (nested augment) | linkage-augment-nested |
| augment inter-module with prefix | linkage-augment-inter-module |
| cross-module augment PascalCase ident collision (codegen) | augment-cross-module-ident-collision |
| when on augment with target XPath context reaching up cross-module (RFC 7950 §7.21.5) | augment-when-target-context |
| deviation not-supported cross-module (RFC 7950 §7.20.3) | linkage-deviation-not-supported |
| deviation replace type narrowing | linkage-deviation-replace-type |
| deviation add mandatory | linkage-deviation-add |
| deviation delete default substatement | linkage-deviation-delete |
| multiple deviations in one module + load order | linkage-deviation-multi |
| deviation replace default/config + add must/if-feature | linkage-deviation-replace-default-config |
| import by prefix (RFC 7950 §7.1.5) | linkage-import-prefix |
| import with revision-date | linkage-import-revision-date |
| multiple imports, mixed prefixes, no shadowing | linkage-import-multiple |
| import non-transitivity (RFC 7950 §5.1.1) | linkage-import-non-transitive |
| submodule belongs-to + include + parent-prefix serialization (RFC 7950 §5.3) | linkage-submodule-simple |
| multiple submodules + cross-submodule typedef/grouping reuse | linkage-submodule-multi |
| submodule importing a foreign top-level module (RFC 7950 §5.3.1) | linkage-submodule-imports-foreign |
| identity standalone (no base, RFC 7950 §6.4.16) | identity-standalone |
| identity cross-module derivation | identity-cross-module-derivation |
| identity multi-base from multiple imported modules | identity-multi-base-cross-module |
| identity hierarchy + identityref base filtering (mid vs root) | identity-hierarchy-with-identityref |
| rpc input only + I4 (RFC 7950 §7.14) | rpc-input-only |
| rpc output only + I4 | rpc-output-only |
| rpc input+output, input-before-output + I4 | rpc-input-output-interleaved |
| rpc I/O all node kinds + I4 | rpc-io-heterogeneous-nodes |
| rpc I/O nested containers + I4 at depth | rpc-io-nested-containers |
| rpc I/O with anyxml opaque + I4 | rpc-io-with-anyxml |
| rpc I/O numeric quoting (int64/uint64/decimal64) + I4 + I5 | rpc-io-decimal64-numeric-types |
| YANG 1.1 action in container + I4 | action-container-simple |
| action in list entry (key context) + I4 | action-list-keys-context |
| action at nested tree depth + I4 | action-nested-containers |
| action I/O heterogeneous node kinds + I4 | action-io-heterogeneous |
| action among container data siblings (I1+I4) | action-container-wide-siblings |
| top-level notification + I4 (RFC 7950 §7.16) | notification-top-level |
| YANG 1.1 notification nested in container + I4 | notification-nested-container |
| notification in list entry + I4 | notification-nested-list |
| notification with container + leaf-list children + I4 | notification-with-container-leaflist |
| notification interleaved among data siblings (sibling placement) | notification-interleaved-siblings |
| rpc/action/notification coexistence (no interference) | rpc-action-notification-coexistence |
| when (OR) + must + error-message/app-tag cross-language parity (RFC 7950 §7.5.3) | constraints-when-must |
| when with XPath functions (derived-from/re-match/count/boolean, RFC 7950 §10) | constraints-when-xpath-functions |
| multiple must conjunction + sibling/ancestor refs | constraints-must-multiple |
| unique single/composite/descendant (RFC 7950 §7.8.3) | constraints-unique-composite |
| unique duplicate-tuple rejection | constraints-unique-violation-reject |
| mandatory + default + when + refine interaction | constraints-mandatory-interaction |
| default on leaf/leaf-list/choice case (RFC 7950 §7.6.1/§7.9.3) | constraints-default-types |
| feature + if-feature boolean expressions + schema gating (RFC 7950 §7.20.1) | constraints-feature-iffeature |
| feature-on-feature transitive dependency | constraints-feature-dependency |
| presence container + if-feature gate + {}-vs-absent codegen Option<()> | constraints-feature-presence |
| raw parse-failure CAMBIUM_E0002 (malformed/NUL/strict unknown) | parse-malformed-e0002 |
| empty+default illegal; leaf-list-of-empty illegal in 1.0 (schema-load reject E0001) | types-empty-edge-illegality |
| RFC 7951 §5.1 module namespace qualification (module:name, bare same-module) | json-ietf-module-namespace-qualification |
| RFC 7951 §6.1 int8..uint32 bare vs int64/uint64 quoted (32->64 boundary) | json-ietf-scalar-quoting-int-spans |
| RFC 7951 §6.1 decimal64 always-quoted canonical + trailing zeros + sign | json-ietf-decimal64-canonical-quoting |
| RFC 7951 §6.1 string escaping control + quote + backslash + Unicode \uXXXX | json-ietf-string-escaping-control-unicode |
| RFC 7951 §5.4 leaf-list JSON array (user preserved / system sorted) | json-ietf-leaflist-array-user-system |
| RFC 7951 §5.3 list JSON array, keys-first in object (I3 in JSON) | json-ietf-list-array-keys-first |
| RFC 7951 §5.2 nested containers as nested JSON objects | json-ietf-nested-container-object |
| RFC 7951 §5.2 choice case-transparency (flat + nested) | json-ietf-choice-case-transparency |
| RFC 7951 presence {} vs non-presence absent in JSON | json-ietf-presence-vs-nonpresence |
| RFC 7951 §5.9/§5.10 anydata/anyxml JSON representation | json-ietf-anydata-anyxml-representation |
| RFC 6243 with-defaults modes explicit/trim/report-all/report-all-tagged | json-ietf-with-defaults-modes |
| RFC 7951 §6.13 instance-identifier as JSON XPath string | json-ietf-instance-identifier-string |
| RFC 7951 leafref resolved base-type value + union resolved-member quoting (§6.3) | json-ietf-leafref-union-resolved-form |
| RFC 7950+7951 JSON interactions: foreign augment qualification, deviation-narrowed quoting, when-absence, submodule parent prefix | json-ietf-cross-module-augment-deviation-when |
| RFC 7951 §4 JSON_IETF parse direction round-trip (input-format=json) | json-ietf-parse-roundtrip |
| yang-version 1.0/1.1 + units + status + description/reference + organization/contact metadata (RFC 7950 §7.1/§7.3.3) | metadata-yang-version-units |
| extension definition + usage + pass-through (RFC 7950 §7.19) | extension-definition-and-usage |
| extension argument yin-element true/false YIN forms (RFC 7950 §7.19.2.2) | extension-yin-element-modes |
| vendor extension (junos jx:annotate/jx:secret) pass-through, codegen-ignored | vendor-extension-junos-passthrough |
| unknown/undefined extension opaque preservation (LYD_PARSE_OPAQ) | extension-unknown-opaque-passthrough |
| extension/typedef PascalCase collision in unified codegen symbol table | extension-and-typedef-collision |
| RFC 7952 instance metadata annotation (md:annotation, @-member JSON / XML attribute) | metadata-annotation-rfc7952 |
| Rust keyword identifiers escaped via r# prefix | idents-keywords-rust |
| Go keyword identifiers escaped (suffix/mangling) | idents-keywords-go |
| hyphen/underscore PascalCase collision deterministic suffix | idents-collision-hyphen-underscore |
| enumeration value name collision (distinct discriminants) | idents-enum-value-collision |
| container-vs-leaf same-parent PascalCase collision | idents-container-leaf-collision |
| leading-digit identifiers (RFC 7950 §6.2) | idents-leading-digit |
| long identifiers >60 chars | idents-long-name |
| unicode/CJK + mixed-case identifiers | idents-unicode-mixed-case |
| RFC 6991 real typedefs (ip-address union/zone, ipv6-prefix, mac-address, date-and-time, counter64) | rfc6991-inet-yang-types-roundtrip |
| serialization preserves original YANG identifiers (no codegen artifacts) — cross-cutting | idents-keywords-rust |
| cross-language Rust==Go rule-code parity for reject cases — cross-cutting | constraints-range-length-reject |
| I4 RPC/action/notification I/O schema-order emission — cross-cutting invariant | rpc-input-output-interleaved |
| I3 keys-first emission — cross-cutting invariant | ordering-composite-key-wide |
| I1 declaration-order preservation — cross-cutting invariant | declaration-order-out-of-alphabetical |
| I2 system canonicalization (list-by-key + leaf-list-by-value) — cross-cutting invariant | leaflist-ordered-by-system |

## Fixture index

| Fixture | Theme | Covers | Invariant |
|---|---|---|---|
| `types-binary-length-base64` | builtin-scalar-types | binary type (RFC 7950 §4.2.9) with length restriction + base64 JSON encoding (RFC 7951 §6.6) | validate + serialize-bytes(JsonIetf,Xml); pin 8-octet value  |
| `types-bits-explicit-positions-gaps` | builtin-scalar-types | bits (RFC 7950 §4.2.9) explicit positions with gaps + space-separated token serialization in DECLARATION order not input order (RFC 7951 §6.5) | serialize-bytes(JsonIetf,Xml) + codegen-compile(bitflags); p |
| `types-boolean-default-false` | builtin-scalar-types | boolean (RFC 7950 §4.2.7) bare true/false + default true/false with with_defaults EXPLICIT emission | serialize-bytes(Json,JsonIetf,Xml) WithDefaults=Explicit for |
| `types-decimal64-fraction1-range` | builtin-scalar-types | decimal64 fraction-digits 1 (tenths) + range (RFC 7950 §4.2.8); JSON-quoted canonical | validate + serialize-bytes(JsonIetf); pin -0.5, 50.0, 99.9 |
| `types-decimal64-fraction18-max-magnitude` | builtin-scalar-types | decimal64 fraction-digits 18 (max per RFC) at maximum representable magnitude | validate + serialize-bytes(JsonIetf); pin 9223372036.8547758 |
| `types-decimal64-fraction2-canonical-round` | builtin-scalar-types | decimal64 fraction-digits 2 with canonical binary-to-decimal rounding (RFC 7950 §9.3.4) | validate + serialize-bytes(JsonIetf) + codegen-compile; pin  |
| `types-decimal64-fraction3-and-6` | builtin-scalar-types | decimal64 fraction-digits 3 and 6 (the explicit 3..8 mid-range span) with trailing-zero canonicalization | validate + serialize-bytes(JsonIetf) + codegen-compile; pin  |
| `types-decimal64-fraction9-negative` | builtin-scalar-types | decimal64 fraction-digits 9 (nanoseconds) + negative + high precision | validate + serialize-bytes(JsonIetf); pin -0.000000001 and + |
| `types-empty-leaf-null-json` | builtin-scalar-types | empty type (316K uses, config-flag idiom) with [null] JSON encoding (RFC 7951 §6.9) and <leaf/> XML | serialize-bytes(Json,JsonIetf,Xml); enabled present -> [null |
| `types-enum-bits-auto-position` | builtin-scalar-types | enum/bit AUTO-position continuation after an explicit value (RFC 7950 §9.6.4, §9.7.4): enum a{value 5;} enum b -> b=6; bit auto-increment (YANG 1.1) | codegen-compile(assert auto ordinals b=6, d=2; bit y=4) + se |
| `types-enumeration-explicit-values-sparse` | builtin-scalar-types | enumeration with explicit non-sequential values 0/4/6 (gaps, RFC 791/2460) + serialize BY NAME not ordinal (RFC 7950 §4.2.4) | validate + serialize-bytes(JsonIetf) + codegen-compile(discr |
| `types-enumeration-zero-value-disabled` | builtin-scalar-types | enumeration with explicit zero value (disabled/off idiom) + non-zero siblings, name round-trip | validate + serialize-bytes(JsonIetf); pin 'disabled'(0), 'pr |
| `types-int-int16-range` | builtin-scalar-types | int16 with full bounds -32768/+32767 and narrowed sub-range -1000..1000 | validate + serialize-bytes(JsonIetf); pin full bounds + in-r |
| `types-int-int32-range-multipart` | builtin-scalar-types | int32 with multi-part range (a..b \| c..d) and gap exclusion (RFC 7950 §9.2.4); int32 emitted JSON-bare (RFC 7951 §6.1) | validate + validate-reject(gap value 0 -> E0003) + serialize |
| `types-int-int64-range-quoted` | builtin-scalar-types | int64 (RFC 7950 §4.2.5) JSON-string-quoted at min/max boundaries (RFC 7951 §6.1) | validate + serialize-bytes(JsonIetf); pin -92233720368547758 |
| `types-int-int8-range` | builtin-scalar-types | int8 (RFC 7950 §4.2.5) with range restriction at signed boundaries -128/0/+127 | validate + serialize-bytes(JsonIetf,Xml); pin i8 at -128, 0, |
| `types-uint-uint16-range-port` | builtin-scalar-types | uint16 port-number idiom (1..65535 excluding 0) | validate + validate-reject(0 -> E0003) + serialize-bytes(Jso |
| `types-uint-uint32-range-multi` | builtin-scalar-types | uint32 (most common type, 297K uses) multi-part range, emitted JSON-bare (RFC 7951 §6.1) | validate + serialize-bytes(JsonIetf); pin 100, 65000 -> bare |
| `types-uint-uint64-range-quoted` | builtin-scalar-types | uint64 (70K uses) JSON-string-quoted at max boundary (RFC 7951 §6.1) | validate + serialize-bytes(JsonIetf); pin 184467440737095516 |
| `types-uint-uint8-range` | builtin-scalar-types | uint8 with range (0..255 full + 0..63 DSCP subset) | validate + serialize-bytes(JsonIetf,Xml); pin 0,128,255 and  |
| `constraints-default-types` | constraints-conditionals | default on leaf (string/numeric/boolean) + default on leaf-list (multiple) + default on a choice case (RFC 7950 §7.6.1, §7.9.3); round-trip | serialize-bytes(JsonIetf,Xml) WithDefaults=Explicit; omit al |
| `constraints-feature-dependency` | constraints-conditionals | feature-on-feature dependency chain (YANG 1.1, RFC 7950 §7.20.1): feature B{if-feature A;} node{if-feature B;} — transitive gate | validate across feature-sets {A}->absent, {A,B}->present, {B |
| `constraints-feature-iffeature` | constraints-conditionals | feature definition + if-feature on leaf/container + YANG 1.1 boolean expression (and/or/not/parens) + feature gating schema-node presence (RFC 7950 §7.20.1) | validate across feature-sets; node present/absent per enable |
| `constraints-feature-presence` | constraints-conditionals | presence container (enable idiom, 238 native) + if-feature-gated presence container; empty presence emits {} JSON while non-presence vanishes; presence -> Optio | validate + serialize-bytes(Json,JsonIetf,Xml)(Json/JsonIetf  |
| `constraints-mandatory-interaction` | constraints-conditionals | mandatory true on leaf + interaction with default (default applied before presence check) + with when (when gates requirement) + mandatory refined from grouping | validate + validate-reject(missing hostname/operational prim |
| `constraints-must-multiple` | constraints-conditionals | multiple must constraints on one node (conjunction) referencing siblings/ancestors, each with error-message/error-app-tag (RFC 7950 §7.5.3) | validate + validate-reject(each must independently -> E0003  |
| `constraints-unique-composite` | constraints-conditionals | unique on single leaf, composite (multiple leaves), and descendant-path on nested lists (RFC 7950 §7.8.3) | validate(distinct tuples byte-golden) + serialize-bytes(Xml) |
| `constraints-unique-violation-reject` | constraints-conditionals | unique-constraint duplicate-tuple REJECTION (RFC 7950 §7.8.3) with data-path/app-tag parity | validate(distinct byte-golden) + validate-reject(two entries |
| `constraints-when-must` | constraints-conditionals | when XPath conditional (OR multi-clause) + must assertion with error-message/error-app-tag; cross-language CAMBIUM_E0003 + app-tag parity (data_validation.rs is | validate(byte-golden) + validate-reject(must violation -> E0 |
| `constraints-when-xpath-functions` | constraints-conditionals | when with XPath functions: derived-from(), derived-from-or-self(), re-match(), boolean(), count(), descendant // (RFC 7950 §10) | validate + validate-reject; gate containers by derived-from( |
| `anydata-untyped-container` | data-node-types | anydata (YANG 1.1, RFC 7950 §7.10) untyped structured data; opaque round-trip (RFC 7951 §5.9 JSON value) | serialize-bytes(JsonIetf,Xml) + parse-opaque; pin opaque sub |
| `anyxml-attributes-namespaced` | data-node-types | anyxml fidelity — XML attributes + namespaced child elements (RFC 7950 §7.11; RFC 7951 §5.10). CRITICAL: libyang preserves attr but REWRITES prefixed ns to defa | serialize-bytes(Xml) + validate; golden captures libyang's A |
| `anyxml-opaque-passthrough` | data-node-types | anyxml (17,581 native uses, RFC 7950 §7.11) opaque XML subtree verbatim round-trip (LYD_PARSE_OPAQ) | serialize-bytes(Xml) + parse-opaque; pin <data><foo><bar>x</ |
| `choice-cases-interleaved-siblings` | data-node-types | choice interleaved with plain leaf/container siblings (not grouped at schema position), I1 | I1-decl-order + serialize-bytes(JsonIetf,Xml); order id, act |
| `choice-mandatory-reject` | data-node-types | mandatory choice (RFC 7950 §7.9.4) — must select a case or validation fails | validate(one case) + validate-reject(no case -> CAMBIUM_E000 |
| `choice-multiple-cases-default` | data-node-types | choice (1,836 corpus) with multiple named cases + default-case (RFC 7950 §7.9.3) + JSON case-transparency (no 'case' wrapper, RFC 7951 §5.2) | I1-decl-order + serialize-bytes(JsonIetf,Xml); activate one  |
| `choice-nested-in-case` | data-node-types | choice nested inside a case of an outer choice + JSON case-transparency at depth; generic case_N names where order is the only meaning (RFC 7950 §7.9.2) | I1-decl-order + serialize-bytes(JsonIetf,Xml); activate ipv4 |
| `choice-shorthand-leaflist-list` | data-node-types | SHORTHAND case — choice directly containing a leaf-list and a list with NO explicit 'case' wrapper (RFC 7950 §7.9.2 auto-wraps); implicit case synthesis | serialize-bytes(JsonIetf,Xml) + validate + codegen-compile(s |
| `choice-single-node-case` | data-node-types | choice with single-node case branches (one leaf per case) | serialize-bytes(JsonIetf,Xml); activate create or delete |
| `choice-with-leaflist-branch` | data-node-types | choice with a case containing an ordered-by-user leaf-list (ordering interaction) | I1-user-ordered + serialize-bytes(JsonIetf,Xml); file case a |
| `composite-key-with-interleaved-containers` | data-node-types | composite-key list with non-key CONTAINERS interleaved between key leaves (I3 enforcement past nested nodes) | I3-keys-first + serialize-bytes(JsonIetf,Xml); scrambled, ke |
| `config-false-state-subtree` | data-node-types | config false subtree, ro-state classification, config/state split | serialize-bytes(Xml) + state-node classification; pin uptime |
| `config-true-subtree` | data-node-types | config true subtree (default, RFC 7950 §7.21.1) rw-data classification | serialize-bytes(Xml) + config-node classification; pin hostn |
| `container-nested-depth` | data-node-types | nested container hierarchy 3+ levels with schema-order preservation at each level (I1) | I1-decl-order + serialize-bytes(JsonIetf,Xml); pin all leave |
| `container-presence-empty` | data-node-types | presence container (RFC 7950 §7.5.1) emitting {} JSON / empty element when present-but-childless; non-presence distinction (vanishes); presence -> Option<()> co | serialize-bytes(Json,JsonIetf,Xml) + codegen-compile; pin en |
| `container-within-list-schema-order` | data-node-types | container child within a list entry, schema order with key first (I3) | I3-keys-first + serialize-bytes(JsonIetf,Xml); key id then c |
| `leaf-list-within-list-entry` | data-node-types | leaf-list child within a list entry, cascading I1/I2 ordering both preserved | I1-user-ordered + serialize-bytes(JsonIetf,Xml); policy list |
| `list-composite-key-three` | data-node-types | composite key with 3 leaves interleaved with non-key siblings (I3) | I3-keys-first + serialize-bytes(JsonIetf,Xml); keys (prefix, |
| `list-composite-key-two` | data-node-types | composite key with 2 leaves, keys-first emission (I3) | I3-keys-first + serialize-bytes(JsonIetf,Xml); pin metric in |
| `list-entry-with-choice-schema-order` | data-node-types | list entry containing a choice at schema position (I3 keys-first + choice ordering together) | I3-keys-first + I1-decl-order + serialize-bytes(JsonIetf,Xml |
| `list-keyless-positional` | data-node-types | keyless list (no key, config false) positional order preservation + max-elements; ygot historically could not emit keyless lists | I1/I2 positional + json-array-order + serialize-bytes(JsonIe |
| `list-single-key-numeric` | data-node-types | list with single numeric (uint16) key + numeric uniqueness | validate + serialize-bytes(JsonIetf,Xml); pin vlan-id=100,20 |
| `list-single-key-string` | data-node-types | list with single string key + key-leaf uniqueness | validate + serialize-bytes(JsonIetf,Xml); pin entries name=' |
| `max-elements-reject` | data-node-types | max-elements constraint (RFC 7950 §7.7.6) rejection when count exceeds maximum | validate(<=4 byte-golden) + validate-reject(5 -> CAMBIUM_E00 |
| `min-elements-reject` | data-node-types | min-elements constraint (RFC 7950 §7.7.5) on list/leaf-list with COUNT-constraint rejection, order-agnostic | validate(2..4 byte-golden) + validate-reject(1 entry -> CAMB |
| `mixed-config-state-nested` | data-node-types | mixed config true + config false in nested siblings (config inheritance/override) | serialize-bytes(Xml) + config inheritance; hostname rw, stat |
| `ordering-composite-key-wide` | data-node-types | composite key with 6 leaves, non-contiguous in declaration order, keys-first at maximum width (I3) | I3-keys-first + serialize-bytes(JsonIetf,Xml) + validate; SC |
| `status-current-deprecated-obsolete` | data-node-types | status statement current/deprecated/obsolete (RFC 7950 §7.21.2) metadata preservation | serialize-bytes(Xml) + status-metadata preservation; set all |
| `parse-malformed-e0002` | edge-illegality | raw PARSE-failure code CAMBIUM_E0002 (rule-codes registry: malformed input, interior NUL, unknown element in strict mode) — distinct from schema-driven E0003 | parse-reject x3: truncated/unclosed XML, interior-NUL value, |
| `types-empty-edge-illegality` | edge-illegality | schema-validity illegality boundaries for type empty (RFC 7950 §7.6.1, §9.11): leaf{type empty; default x;} illegal; leaf-list of empty illegal in YANG 1.0 (all | schema-load-reject: (a) empty+default FAILS load, (b) 1.0 le |
| `extension-and-typedef-collision` | extensions-metadata | extension defined in module A; typedef in module B with PascalCase collision — codegen unified symbol table spans both RFC 7950 namespaces | codegen-compile(non-colliding symbols despite extension/type |
| `extension-definition-and-usage` | extensions-metadata | extension definition (extension + argument, RFC 7950 §7.19) + usage on schema nodes + pass-through on serialization (libyang opaque) | validate + serialize-bytes(Xml); custom extension annotation |
| `extension-unknown-opaque-passthrough` | extensions-metadata | unknown/undefined extension usage preserved as opaque via libyang LYD_PARSE_OPAQ; round-trip fidelity | serialize-bytes(Xml); undefined extension preserved unchange |
| `extension-yin-element-modes` | extensions-metadata | extension argument yin-element TRUE (nested-element form) vs FALSE (attribute form) serialization difference in YIN (RFC 7950 §7.19.2.2) | serialize-bytes(Xml/YIN) + schema-introspection; attr-form a |
| `metadata-annotation-rfc7952` | extensions-metadata | RFC 7952 instance-level metadata annotations (md:annotation extension; @-member in JSON, attribute in XML) — the mechanism behind default/origin tags | serialize-bytes(JsonIetf,Xml) + validate; import ietf-yang-m |
| `metadata-yang-version-units` | extensions-metadata | yang-version 1.0 vs 1.1 + units + status(current/deprecated/obsolete) + description/reference + organization/contact module metadata; round-trip preservation (R | validate + serialize-bytes(Xml); module + node metadata pars |
| `vendor-extension-junos-passthrough` | extensions-metadata | Juniper vendor extension usage (jx:annotate, jx:secret via import) — pass-through on serialization, IGNORED by codegen | validate + serialize-bytes(Xml) + codegen-compile(extensions |
| `idents-collision-hyphen-underscore` | identifier-codegen | PascalCase collision between hyphenated and underscored siblings (foo-bar + foo_bar -> FooBar) with deterministic suffix; serialization preserves both originals | codegen-compile(FooBar/FooBar2 distinct deterministic) + ser |
| `idents-container-leaf-collision` | identifier-codegen | PascalCase collision between a container and a sibling leaf within the same parent (struct vs field naming clash) | codegen-compile(non-colliding struct/field types) + serializ |
| `idents-enum-value-collision` | identifier-codegen | enumeration value name collisions (enabled / ENABLED / enable-default / enable_default) — codegen must emit distinct enum variant discriminants | codegen-compile(distinct variants despite case-only and hyph |
| `idents-keywords-go` | identifier-codegen | Go keyword identifiers (func, range, chan, map, select, go, defer, interface, package) escaped (suffix/mangling); serialization emits original YANG names | codegen-compile(valid Go idents, no reserved clash) + serial |
| `idents-keywords-rust` | identifier-codegen | Rust keyword identifiers (type, match, ref, move, struct, fn, impl, loop, async, await, box, where, self, crate, super) escaped via r# prefix; serialization emi | codegen-compile(r#type etc.) + serialize-bytes(Xml)(original |
| `idents-leading-digit` | identifier-codegen | identifiers beginning with digits (2g, 10Gbps, 0config) — valid YANG (RFC 7950 §6.2) but Rust/Go reject leading digits; codegen prefixes/mangles | codegen-compile(valid mangled idents) + serialize-bytes(Xml) |
| `idents-long-name` | identifier-codegen | identifiers >60 chars (72-char and 80+ char names); codegen truncates/hashes/keeps; serialization preserves full name | codegen-compile + serialize-bytes(Xml)(full YANG name preser |
| `idents-unicode-mixed-case` | identifier-codegen | non-ASCII/CJK and mixed-case identifiers (camelCase, PascalCase) where YANG permits (RFC 7950 NameStartChar); codegen handles/rejects; serialization unchanged | codegen-compile (behavior documented) + serialize-bytes(Xml) |
| `identity-cross-module-derivation` | identity | identity defined in module A, DERIVED in module B by importing A and basing on the prefixed identity; cross-module base resolution | validate + serialize-bytes(JsonIetf,Xml); two owned modules, |
| `identity-hierarchy-with-identityref` | identity | grandparent/parent/child identity hierarchy + identityref restricted to a mid-level base accepting descendants vs root base accepting any | validate + serialize-bytes(JsonIetf) + codegen-compile(ident |
| `identity-multi-base-cross-module` | identity | identity in module C deriving from bases in modules A AND B (multi-base cross-module, RFC 7950 §6.4.16 YANG 1.1) | validate + serialize-bytes(JsonIetf); three owned modules, h |
| `identity-standalone` | identity | identity (RFC 7950 §6.4.16) without base — standalone abstract identity as a type base point | validate; identity exists as identityref base; component-typ |
| `json-ietf-anydata-anyxml-representation` | json-ietf-serialization | RFC 7951 §5.9/§5.10 — anydata as untyped JSON value preserved verbatim; anyxml opaque XML-to-JSON conversion | serialize-bytes(JsonIetf); anydata {custom:[1,2,{nested:true |
| `json-ietf-choice-case-transparency` | json-ietf-serialization | RFC 7951 §5.2 — active case child at choice position WITHOUT 'case' wrapper; nested-choice transparency at depth | serialize-bytes(JsonIetf); deny case -> {priority,reason} no |
| `json-ietf-cross-module-augment-deviation-when` | json-ietf-serialization | RFC 7950+7951 JSON interactions: augmented foreign node module-qualified at top; deviation-replace-narrowed type changes JSON quoting (uint64->uint32 bare); whe | serialize-bytes(JsonIetf); augmented node qualified, narrowe |
| `json-ietf-decimal64-canonical-quoting` | json-ietf-serialization | RFC 7951 §6.1 — decimal64 ALWAYS quoted string in canonical form: trailing zeros per fraction-digits + sign preservation + zero canonicalization | serialize-bytes(JsonIetf); negative=-3.14, positive=2.50 (tr |
| `json-ietf-instance-identifier-string` | json-ietf-serialization | RFC 7951 §6.13 — instance-identifier serialized as a JSON string with fully-qualified XPath path + predicates | serialize-bytes(JsonIetf); active-device -> '/jiiis:devices/ |
| `json-ietf-leaflist-array-user-system` | json-ietf-serialization | RFC 7951 §5.4 — leaf-list as JSON array; ordered-by user preserves insertion order, ordered-by system sorted/canonicalized | serialize-bytes(JsonIetf); user [100,50,75] preserved, syste |
| `json-ietf-leafref-union-resolved-form` | json-ietf-serialization | RFC 7951 — leafref serializes as its RESOLVED BASE-TYPE value (not the XPath); union member serializes per its RESOLVED member type quoting (§6.3) | serialize-bytes(JsonIetf); leafref primary-iface->'eth0' (ba |
| `json-ietf-list-array-keys-first` | json-ietf-serialization | RFC 7951 §5.3 — list as JSON array of objects; within each object key leaves FIRST then non-keys (I3 in JSON) | serialize-bytes(JsonIetf); composite key, objects emit name, |
| `json-ietf-module-namespace-qualification` | json-ietf-serialization | RFC 7951 §5.1 — top-level member is module:name; child in different module re-qualifies; same-module children bare | serialize-bytes(JsonIetf); top-level key 'json-ietf-module-q |
| `json-ietf-nested-container-object` | json-ietf-serialization | RFC 7951 §5.2 — nested containers as nested JSON objects | serialize-bytes(JsonIetf); top/middle/deep/value -> nested o |
| `json-ietf-parse-roundtrip` | json-ietf-serialization | RFC 7951 §4 PARSE direction — reading a JSON_IETF document IN (quoting, [null], module-prefix, leaf-list array) then round-tripping; FIRST input-format='json' f | serialize-bytes(JsonIetf,Xml) from a json input; int64 quote |
| `json-ietf-presence-vs-nonpresence` | json-ietf-serialization | RFC 7951 — presence container present-but-childless emits {}; non-presence empty container ABSENT (not {}) | serialize-bytes(JsonIetf); ssh presence -> {} (or {port:22}  |
| `json-ietf-scalar-quoting-int-spans` | json-ietf-serialization | RFC 7951 §6.1 — int8..int32/uint8..uint32 BARE numbers vs int64/uint64 QUOTED strings; the 32->64-bit quoting boundary | serialize-bytes(JsonIetf); i8..u32 bare, i64="92233720368547 |
| `json-ietf-string-escaping-control-unicode` | json-ietf-serialization | RFC 7951 §6.1 JSON string escaping — control chars (\n,\t,\u00XX), quote (\"), backslash (\\), and non-ASCII Unicode (\uXXXX, surrogate pairs for emoji) | serialize-bytes(JsonIetf); pin newline/tab control, embedded |
| `json-ietf-with-defaults-modes` | json-ietf-serialization | RFC 6243 with-defaults modes: explicit (§2.1), trim (§2.2), report-all (§2.3), report-all-tagged (§2.4) — default emission/omission/tagging | serialize-bytes(JsonIetf) x4 modes over the same timeout/ret |
| `augment-cross-module-ident-collision` | linkage | cross-module augment injecting a sibling whose PascalCase type name collides with a native node in the target module (codegen disambiguation), serialization sti | codegen-compile(two distinct non-clobbering types, determini |
| `augment-when-target-context` | linkage | when on an AUGMENT evaluated with the augment-TARGET as XPath context, reaching UP the target tree cross-module (RFC 7950 §7.21.5) — the OpenConfig/Junos failur | validate + validate-reject(mode!='enabled' with area present |
| `linkage-augment-choice-case` | linkage | augment of a choice (injecting a new case) + augment of an existing case (adding a leaf to it) | serialize-bytes(JsonIetf,Xml) + validate; activate udp case  |
| `linkage-augment-container-leaf-list` | linkage | augment carrying multiple node kinds at once (container + leaf + leaf-list), declaration order among augmented peers preserved | serialize-bytes(JsonIetf,Xml); scrambled -> compiled order s |
| `linkage-augment-inter-module` | linkage | augment in module B targeting module A path with prefix (RFC 7950 §7.17 inter-module); injected children inherit augmenting-module context in JSON | serialize-bytes(JsonIetf,Xml); two owned modules, prefixed a |
| `linkage-augment-intra-module` | linkage | augment (RFC 7950 §7.17) in the same module targeting an earlier container + when guard; injected node at libyang-compiled position not appended | I2-decl-order + validate + validate-reject(when-false type=' |
| `linkage-augment-nested` | linkage | augment of an already-augmented node (second augment targets first augment's injected container) | serialize-bytes(JsonIetf,Xml) + validate; both augments' chi |
| `linkage-deviation-add` | linkage | cross-module deviation add inserting mandatory true on a previously-optional leaf | validate(optional-name present) + validate-reject(omitted -> |
| `linkage-deviation-delete` | linkage | cross-module deviation delete removing a default substatement; default cleared from compiled schema | serialize-bytes(Xml) WithDefaults=Explicit; omit flag -> NOT |
| `linkage-deviation-multi` | linkage | multiple deviations on different nodes in ONE module (not-supported + replace + add) with load-order resolution; each violation tested independently | validate(valid tree) + validate-reject x3: legacy present->E |
| `linkage-deviation-not-supported` | linkage | cross-module deviation not-supported (RFC 7950 §7.20.3) removing a node; using the removed node fails parse (CAMBIUM_E0002) | validate(deviated tree byte-golden) + validate-reject(deprec |
| `linkage-deviation-replace-default-config` | linkage | deviate REPLACE of default/config + deviate ADD of must/if-feature (RFC 7950 §7.20.3.2). CRITICAL: replace{default} requires the target to ALREADY carry a defau | validate(effective: mode default 'enabled', router-id ro) +  |
| `linkage-deviation-replace-type` | linkage | cross-module deviation replace narrowing a type (uint64->uint32+range); codegen uses the replaced type | validate(count=500) + validate-reject(count=2000 exceeds ran |
| `linkage-grouping-config-state` | linkage | one grouping reused by both config and state containers — INLINE copy, not a shared reference (two distinct codegen structs) | serialize-bytes(Xml) + codegen-compile(two distinct struct f |
| `linkage-grouping-cross-module` | linkage | grouping defined in module A, used (prefixed) in module B; children inherit base-module context in JSON | serialize-bytes(JsonIetf) + codegen-compile; two owned modul |
| `linkage-grouping-nested-uses` | linkage | grouping that itself contains a uses of another grouping (transitive reuse, flattens with no double wrapping) | codegen-compile(transitive expansion) + serialize-bytes(Json |
| `linkage-grouping-simple` | linkage | grouping definition (RFC 7950 §7.12) + uses expansion (§7.13) at schema position among siblings; codegen INLINES the grouping | I1-decl-order + serialize-bytes(JsonIetf,Xml) + codegen-comp |
| `linkage-import-multiple` | linkage | single module importing 3+ other modules with mixed prefixes; cross-imported typedefs compose without shadowing | validate + serialize-bytes(JsonIetf); four owned modules, ob |
| `linkage-import-non-transitive` | linkage | import NON-transitivity (RFC 7950 §5.1.1) — A imports B, B imports C; A canNOT use C's symbols without importing C | validate-reject(A references c:color unimported -> CAMBIUM_E |
| `linkage-import-prefix` | linkage | import (RFC 7950 §7.1.5) by module name + assigned prefix; remote typedef usage with prefix | validate + serialize-bytes(JsonIetf); two owned modules, por |
| `linkage-import-revision-date` | linkage | import with revision-date substatement pinning a specific module version | validate + serialize-bytes(JsonIetf); both modules pinned-re |
| `linkage-refine-default` | linkage | refine (RFC 7950 §7.13.2) altering default on inherited leaves | serialize-bytes WithDefaults=Explicit + codegen-compile; omi |
| `linkage-refine-mandatory-config` | linkage | refine changing mandatory true / config false on inherited leaves | validate + serialize-bytes(Xml) + codegen-compile(name non-O |
| `linkage-refine-min-max-iffeature` | linkage | refine changing min-elements/max-elements on an inherited leaf-list + refine adding if-feature to an inherited node | validate + validate-reject(0 tags -> E0003 min) + serialize- |
| `linkage-refine-presence-must` | linkage | refine adding presence to an inherited container + refine adding a must constraint | validate + validate-reject(must fail val=0 -> CAMBIUM_E0003) |
| `linkage-submodule-imports-foreign` | linkage | a submodule that itself IMPORTS a foreign top-level module and uses its typedef (RFC 7950 §5.3.1) — parent-prefix serialization | validate + serialize-bytes(JsonIetf,Xml); submodule imports  |
| `linkage-submodule-multi` | linkage | main module including TWO submodules; cross-submodule typedef/grouping reuse; all compile into parent namespace | validate + serialize-bytes(JsonIetf,Xml); main+2 submodules, |
| `linkage-submodule-simple` | linkage | submodule with belongs-to (RFC 7950 §5.3) + include (§7.1.5); submodule nodes compile into PARENT namespace and serialize with parent prefix in JSON (not submod | serialize-bytes(JsonIetf,Xml) + validate; main+submodule, gr |
| `action-container-simple` | operations | YANG 1.1 action (RFC 7950 §7.15) inside a container with input+output; action input/output schema order (I4); the project's yang-version 1.1 coverage | I4-rpc-io-order + serialize-bytes(Xml) + validate; action at |
| `action-container-wide-siblings` | operations | YANG 1.1 action positioned AMONG many data-leaf siblings in a container (action at schema position, I1+I4) | I4-rpc-io-order + serialize-bytes(Xml); schema order setting |
| `action-io-heterogeneous` | operations | YANG 1.1 action input/output with heterogeneous node kinds (leaf, leaf-list, container) | I4-rpc-io-order + serialize-bytes(Xml); scrambled -> full sc |
| `action-list-keys-context` | operations | YANG 1.1 action inside a list entry (list key provides implicit invocation context per entry) | I4-rpc-io-order + serialize-bytes(Xml) + validate; scrambled |
| `action-nested-containers` | operations | YANG 1.1 action at a nested position in the data tree (container>container>action) | I4-rpc-io-order + serialize-bytes(Xml) + validate; action at |
| `notification-interleaved-siblings` | operations | notification schema-node positioned AMONG ordinary data siblings (RFC 7950 §7.16; I4/I2 interaction) — must NOT perturb the container's DATA-instance declaratio | serialize-bytes(JsonIetf,Xml) + validate; DATA instance emit |
| `notification-nested-container` | operations | YANG 1.1 notification nested inside a data container; content schema order (I4) | I4-rpc-io-order + serialize-bytes(JsonIetf,Xml); scrambled - |
| `notification-nested-list` | operations | YANG 1.1 notification inside a list entry (scoped to entry context) | I4-rpc-io-order + serialize-bytes(JsonIetf,Xml); scrambled - |
| `notification-top-level` | operations | top-level notification (RFC 7950 §7.16, YANG 1.0) with content children in schema order (I4) | I4-rpc-io-order + serialize-bytes(JsonIetf,Xml); scrambled - |
| `notification-with-container-leaflist` | operations | notification containing nested container AND leaf-list children in schema order (I4) | I4-rpc-io-order + serialize-bytes(JsonIetf,Xml); scrambled - |
| `rpc-action-notification-coexistence` | operations | one module defining rpc + action + notification together — verify no cross-type ordering interference; each independently in schema order | I4-rpc-io-order + serialize-bytes(JsonIetf,Xml); all three p |
| `rpc-input-only` | operations | rpc (RFC 7950 §7.14) with input children only, no output (YANG 1.0); input schema-order emission (I4) | I4-rpc-io-order + serialize-bytes(JsonIetf,Xml); scrambled i |
| `rpc-input-output-interleaved` | operations | rpc with both input and output blocks; input children before output children, each in schema order (I4) | I4-rpc-io-order + serialize-bytes(JsonIetf,Xml); scrambled b |
| `rpc-io-decimal64-numeric-types` | operations | rpc input/output with int64/uint64/decimal64 requiring JSON string quoting (RFC 7951 §6.1) while preserving I4 order; inherits I5 Rust==Go gate | I4-rpc-io-order + serialize-bytes(JsonIetf)(int64/uint64/dec |
| `rpc-io-heterogeneous-nodes` | operations | rpc input/output containing all node kinds (leaf, leaf-list, container, choice) in schema order (I4) | I4-rpc-io-order + serialize-bytes(JsonIetf,Xml); scrambled - |
| `rpc-io-nested-containers` | operations | rpc input/output with nested container hierarchies; I4 ordering at depth | I4-rpc-io-order + serialize-bytes(JsonIetf,Xml); scrambled n |
| `rpc-io-with-anyxml` | operations | rpc input/output containing an anyxml opaque node at schema position; content verbatim | I4-rpc-io-order + serialize-bytes(Xml) + parse-opaque; scram |
| `rpc-output-only` | operations | rpc with output children only (reply-only); output schema-order emission (I4) | I4-rpc-io-order + serialize-bytes(JsonIetf,Xml); scrambled r |
| `declaration-order-out-of-alphabetical` | ordering-invariants | container with children in NON-alphabetical schema order (schema-declaration-order preservation, I1) | I1-decl-order + serialize-bytes(JsonIetf,Xml); input zebra,a |
| `json-object-determinism` | ordering-invariants | I5 — JSON object member order = single libyang printer order (schema declaration order), NOT language-native map/struct reflection; Rust==Go byte-identical | I5-json-object-determinism + serialize-bytes(Json,JsonIetf)  |
| `leaflist-ordered-by-system` | ordering-invariants | leaf-list ordered-by system (default) libyang value-canonicalization on parse (I2 second half) — the distinct value-canonicalization code path | I2-system + json-array-order + serialize-bytes(JsonIetf,Xml) |
| `leaflist-ordered-by-user` | ordering-invariants | leaf-list ordered-by user (~42K uses) byte-exact value insertion order, JSON array order (RFC 7951 §5.4, I1) | I1-user-ordered + json-array-order + serialize-bytes(JsonIet |
| `leaflist-with-defaults` | ordering-invariants | leaf-list with multiple default values (RFC 7950 §7.7.2) | serialize-bytes(JsonIetf,Xml) WithDefaults=Explicit; omit se |
| `list-ordered-by-system-canonical` | ordering-invariants | ordered-by system LIST (default) libyang key-canonicalization (I2 first half) | I2-system + serialize-bytes(JsonIetf,Xml); pin vlan-id 300,1 |
| `list-ordered-by-user-insertion` | ordering-invariants | ordered-by user LIST byte-exact insertion-order preservation (I1) | I1-user-ordered + serialize-bytes(JsonIetf,Xml); pin rules c |
| `ordered-user-config-false-state` | ordering-invariants | positional keyless list under config FALSE (state data) preserving parse order — the state-vs-config user-order crossing (RFC 7950 §7.7.7) | serialize-bytes(JsonIetf,Xml) + validate; pin event seq 3,1, |
| `ordering-nested-user-cascading` | ordering-invariants | ordered-by user list whose child leaf-list is ALSO ordered-by user (cascading double order); ygot historically errored | I1-user-ordered + json-array-order + serialize-bytes(JsonIet |
| `wide-heterogeneous-siblings-all-types` | ordering-invariants | container with 9+ mixed-kind children (containers, leaves, leaf-lists, choices, config-false state) in scrambled input — broadest declaration-order stress (I1) | I1-decl-order + serialize-bytes(JsonIetf,Xml); scrambled inp |
| `rfc6991-inet-yang-types-roundtrip` | real-typedef-imports | RFC 6991 real-typedef round-trip — imports vendored ietf-inet-types + ietf-yang-types; ip-address (ipv4/ipv6 union incl. %zone), ipv6-prefix, mac-address, date- | validate + serialize-bytes(JsonIetf,Xml) + codegen-compile;  |
| `identityref-iana-if-type-foreign` | reference-types | RFC 7951 §6.8 foreign-module identityref with a REALISTIC large foreign base — iana-if-type (hundreds of derived); imports the vendored iana-if-type + ietf-inte | validate + serialize-bytes(JsonIetf,Xml) + codegen-compile;  |
| `types-identityref-derived-hierarchy` | reference-types | multi-level derived identity hierarchy (grandparent<-parent<-child<-grandchild); identityref to ancestor accepts deep descendant | validate + serialize-bytes(JsonIetf bare) + codegen-compile( |
| `types-identityref-foreign-module-prefix` | reference-types | identityref to a foreign-module-derived identity emitted module:identity-prefixed in JSON (RFC 7951 §6.8); two owned modules | serialize-bytes(JsonIetf) with module: prefix on foreign ide |
| `types-identityref-multiple-bases` | reference-types | identityref derived from MULTIPLE base identities (YANG 1.1, union-like OR semantics, RFC 7950 §6.4.16) | validate + serialize-bytes(JsonIetf); pin component='etherne |
| `types-identityref-single-base` | reference-types | identityref (RFC 7950 §9.10) single base + same-module identity emitted BARE (RFC 7951 §6.8) | validate + serialize-bytes(JsonIetf bare) + codegen-compile; |
| `types-instance-identifier-complex-path` | reference-types | instance-identifier targeting nested list with COMPOSITE key + nested predicates | validate + serialize-bytes(JsonIetf); pin '/vlans/vlan[id="1 |
| `types-instance-identifier-no-require` | reference-types | instance-identifier with require-instance false (unvalidated reference, RFC 7950 §9.13) | validate(unresolved XPath accepted, no error); pin arbitrary |
| `types-instance-identifier-require-default` | reference-types | instance-identifier with require-instance true (default, RFC 7950 §9.13) + path validation + JSON string form (RFC 7951 §6.13) | validate + validate-reject(dangling -> CAMBIUM_E0003) + seri |
| `types-leafref-absolute-path` | reference-types | leafref (504 uses, RFC 7950 §9.9) with absolute path /top/.../name + unresolved-target rejection (§9.9.3) | validate + validate-reject(dangling -> CAMBIUM_E0003 both la |
| `types-leafref-cross-module` | reference-types | leafref whose TARGET is in a DIFFERENT module (RFC 7950 §9.9.2) — PREFIXED path steps /b:.../b:name | validate(byte-golden JsonIetf) + validate-reject(dangling -> |
| `types-leafref-current-context` | reference-types | leafref with current() context function (RFC 7950 §10.1.1) in a predicate; sibling-relative resolution | validate + validate-reject; pin proto-name resolving sibling |
| `types-leafref-deref-function` | reference-types | leafref using deref() XPath function (YANG 1.1, RFC 7950 §10.3.1) | validate + serialize-bytes(JsonIetf); pin home-dir resolving |
| `types-leafref-relative-parent-path` | reference-types | leafref with relative ../ path from nested context (RFC 7950 §6.4.1) | validate + validate-reject(unresolved); pin nested route-id  |
| `types-leafref-require-instance-false` | reference-types | leafref with require-instance false (forward-reference allowed; dangling not an error) | validate(dangling accepted) + serialize-bytes(JsonIetf); pin |
| `types-leafref-to-leaf-list` | reference-types | leafref targeting a leaf-list member (distinct mechanism from list key) | validate + serialize-bytes(JsonIetf); pin assigned-tag refer |
| `types-leafref-to-leafref-chain` | reference-types | leafref targeting another leafref (chained resolution, RFC 7950) | validate + serialize-bytes(JsonIetf); pin active-alias -> pr |
| `types-leafref-to-list-key` | reference-types | leafref targeting a list KEY leaf (the most common target) | validate + serialize-bytes(JsonIetf); pin primary-device ref |
| `types-typedef-chain-2deep` | type-composition | typedef of typedef (2 levels): B=A=base; restrictions inherited | validate + serialize-bytes(JsonIetf,Xml) + codegen-compile;  |
| `types-typedef-chain-3deep` | type-composition | typedef chained 3+ levels (C->B->A->base); transitive restriction inheritance | validate + serialize-bytes(JsonIetf) + codegen-compile; pin  |
| `types-typedef-default-inheritance` | type-composition | default applied THROUGH a typedef (RFC 7950 §7.3 typedef default substatement) vs leaf override | serialize-bytes WithDefaults=Explicit + codegen-compile(fiel |
| `types-typedef-restriction-narrowing` | type-composition | typedef adding a range/pattern restriction narrower than its base typedef | validate + validate-reject(out-of-narrowed -> E0003) + seria |
| `types-typedef-simple-base` | type-composition | simple typedef with a base type (RFC 7950 §7.3) | validate + serialize-bytes(JsonIetf,Xml) + codegen-compile;  |
| `types-typedef-submodule-cross-file` | type-composition | typedef defined in a submodule and used in the parent module (cross-file) | validate + serialize-bytes(JsonIetf) + codegen-compile; pin  |
| `types-typedef-union-composition` | type-composition | typedef that is itself a union, nested with a further outer union (RFC 7950 §9.12) | validate + serialize-bytes(JsonIetf) + codegen-compile; pin  |
| `types-union-enum-and-scalar` | type-composition | union combining an enumeration with a scalar base type | validate + serialize-bytes(JsonIetf); pin 'auto'(enum arm) a |
| `types-union-heterogeneous-members-quoting` | type-composition | union (193K+ uses, RFC 7950 §4.2.12) with 5+ heterogeneous members + per-member JSON quoting (string/bool bare, int64/uint64/decimal64 quoted, RFC 7951 §6.1/§6. | validate + serialize-bytes(JsonIetf) + codegen-compile(tagge |
| `types-union-identityref-member` | type-composition | union containing an identityref member | validate + serialize-bytes(JsonIetf); pin 'tcp'(identityref  |
| `types-union-leafref-member` | type-composition | union containing a leafref member (functional reference arm) | validate + serialize-bytes(JsonIetf); pin 'eth0'(leafref arm |
| `types-union-member-resolution-order` | type-composition | union member RESOLUTION ORDER — first-match wins on an ambiguous value (RFC 7950 §9.12) | validate + serialize-bytes(JsonIetf) + codegen(assert discri |
| `types-union-nested-typedef-chain` | type-composition | union nesting — a union member that is itself a typedef union (union-of-union), + typedef-of-typedef chain + binary union member (base64) | validate + serialize-bytes(JsonIetf) + codegen-compile(flatt |
| `types-union-scalar-all-members` | type-composition | union containing ALL scalar base kinds (string, bool, int8..64, uint8..64, decimal64, enum, bits, binary) — every member kind in one union | validate + serialize-bytes(JsonIetf) + codegen-compile; exer |
| `types-union-two-identityrefs-distinct-bases` | type-composition | union of TWO identityref members with DIFFERENT bases (union of two identity hierarchies) | validate + serialize-bytes(JsonIetf); pin 'linecard'(hw base |
| `constraints-range-length-reject` | type-restrictions | the bare numeric range / string length REJECTION path (CAMBIUM_E0003) — the core type-restriction reject that other type fixtures only comment | validate(in-range byte-golden) + validate-reject: n<min, n>m |
| `json-ietf-decimal64-no-exponent` | type-restrictions | RFC 7951 §6.1 no-exponent canonical decimal64 on the PARSE side — scientific/exponent input rejected | validate-reject: input rate='1E-9' REJECTED at PARSE -> CAMB |
| `types-range-length-min-max-keywords` | type-restrictions | range/length using literal 'min'/'max' grammar tokens (RFC 7950 §9.2.4, §9.4.4) resolving to type extremes | validate + serialize-bytes(JsonIetf,Xml); pin int32 'max'=21 |
| `types-string-length-pattern-anchor-posix` | type-restrictions | string length + pattern with anchors (^,$) + POSIX character classes ([[:alnum:]]) all AND-composed (RFC 7950 §4.2.6) | validate + validate-reject; pin 'abc_123' valid, '1234567890 |
| `types-string-multiple-patterns-conjunction` | type-restrictions | string with multiple pattern statements AND-composed (RFC 7950 §4.2.6); value must match ALL | validate + validate-reject(both langs, E0003); pin 'abc123'  |
| `types-string-pattern-modifier-invert-match` | type-restrictions | CORE RFC 7950 1.1 pattern 'modifier invert-match' (§9.4.6) — requires yang-version 1.1; negation semantics | validate + validate-reject(value matching base pattern REJEC |

## Deferred fixtures & known follow-ups (2026-06-15)

Reconciles the 189-fixture catalog with the 187 authored fixtures (+6 pre-existing
= 193 manifest cases). The conformance corpus is now genuinely **yanglint-oracle
verified at runtime** — the Rust runner runs yanglint independently over all 193
cases and they match Cambium byte-for-byte (the oracle was previously dead code;
fixed in `f46eb5c`).

**Deferred (legitimately blocked, not engine defects):**
- `idents-leading-digit` — RFC 7950 §6.2 forbids a YANG identifier starting with a
  digit; libyang rejects it at schema load (`Invalid identifier first character`).
  The catalog entry is over-specified; nothing to author.
- `extension-unknown-opaque-passthrough` — needs `LYD_PARSE_OPAQ` to preserve
  unknown-extension opaque nodes, which requires a per-case parse-mode field in the
  manifest that both runners honor. A valid future enhancement (add the field +
  wire both runners), not authorable under the current parse-mode-fixed runners.

**Known follow-ups:**
- The **Go** conformance runner now has the same optional yanglint cross-check as
  the Rust runner when `CAMBIUM_YANGLINT` points at a yanglint binary. It remains
  skipped when the environment variable is unset.
- `parse_op` returns the full parsed `tree` (ancestor context preserved) — not the
  bare operation node — so a nested action/notification round-trips with its
  instance keys per RFC 7950 §7.15.2. See `parse_op` in `adapter.rs` / `libyang.go`.
