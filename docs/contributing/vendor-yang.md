# Distilling fixtures from vendor YANG

Real device models — Nokia, OpenConfig, and other vendor releases — are where the
hard ordering and projection hazards actually occur: deep cross-module augments,
submodule meshes, deviation matrices, feature-gated constructs. Cambium needs
conformance coverage for those shapes, but it does **not** vendor proprietary
device models into the corpus. This page is the policy and workflow for the
middle path: mine vendor repositories as references, then reduce what you find to
small **owned** fixtures that preserve the construct shape that matters —
effective schema order, augment/deviation behavior, submodule/import topology,
list keys, defaults, mandatory nodes, and projection traversal — without copying
vendor text. For the corpus those fixtures land in, see
[conformance.md](./conformance.md).

## Source classes

Public upstreams are scratch inputs only — read for a construct census, never
copied into `/conformance`:

- Nokia SR OS — `https://github.com/nokia/7x50_YangModels`
- Nokia SR Linux — `https://github.com/nokia/srlinux-yang-models`
- OpenConfig — `https://github.com/openconfig/public`
- Cross-vendor survey inputs, when needed: public `YangModels/yang` vendor
  directories, read only for construct census and never copied into
  `/conformance`.

## Workflow

1. Clone or update the vendor repository under `/.planning/` (the gitignored
   agent scratchpad) or another ignored scratch directory. Vendor checkouts must
   never be tracked.
2. Run the corpus census script, `scripts/yang-corpus-census.py`, against the
   local checkout. It reads local paths only — it does not clone, modify, or copy
   vendor files into `/conformance`.
3. Read the aggregate statement counts and the example statement locations it
   reports, and pick the construct combination you want to cover.
4. Author a small owned fixture under `/conformance/fixtures/<case>/` (see
   [Fixture policy](#fixture-policy)).
5. Register the case in `/conformance/manifest.toml`.
6. Prefer `tier = "schema-ir"` for schema-shape and projection hazards — that is
   the cgo-free tier where ordering and projection live. Reach for a backend/data
   fixture only when you are asserting a payload or serialization byte invariant
   that requires the data tier.
7. Run the focused schema/projection tests, then the full local gate via
   `scripts/green-bar.sh`.

The census script takes one or more `paths` (files or directories) plus optional
`--json-out` and `--markdown-out` report destinations (and `--max-examples` to
cap example locations per keyword). For example, against a shallow SR OS checkout:

```sh
git clone -b sros_24.10 --depth 1 https://github.com/nokia/7x50_YangModels .planning/vendor/7x50
scripts/yang-corpus-census.py \
  .planning/vendor/7x50/YANG \
  --json-out .planning/vendor/7x50-census.json \
  --markdown-out .planning/vendor/7x50-census.md
```

And against OpenConfig:

```sh
git clone --depth 1 https://github.com/openconfig/public .planning/vendor/openconfig-public
scripts/yang-corpus-census.py \
  .planning/vendor/openconfig-public/release/models \
  --json-out .planning/vendor/openconfig-census.json \
  --markdown-out .planning/vendor/openconfig-census.md
```

## Fixture policy

Owned fixtures are minimal and synthetic. The construct combination is
vendor-inspired; the bytes are Cambium's own:

- Keep module names under a Cambium-owned namespace — `schema-vendor-*` or
  similar. Nothing should resolve to a vendor namespace.
- Preserve the vendor-inspired construct combination, **not** vendor text.
  Reproduce the shape (the augment topology, the deviation matrix, the submodule
  include graph), not the source.
- Use tiny representative paths and names.
- Include module-qualified paths when an augment can collide by local name across
  modules — that collision is often the thing under test.
- Avoid product names, proprietary descriptions, copied comments, and copied
  `typedef` bodies. If a `description` would identify a product, rewrite it.
- Get projection coverage for free by registering the case in the manifest:
  `TestProjectionConformanceManifestFixtures` runs every manifest fixture through
  `ProjectSubtree` and `ProjectSchemaPaths`, so a manifest-registered fixture is
  automatically exercised for projection traversal.

## Current distilled fixtures

- `schema-vendor-nokia-submodule-mesh` — Nokia SR OS-inspired multi-submodule
  topology with sibling definitions, a submodule-level include, defaults,
  mandatory leaves, list keys, and effective include-site ordering.
- `schema-vendor-nokia-feature-deviation` — Nokia SR OS / SR Linux-inspired
  feature-gated augment plus deviation matrix covering `not-supported`,
  `replace`, if-feature-controlled deviations, ordered lists and leaf-lists,
  min/max-elements, leafrefs, `must`, defaults, mandatory nodes, and config
  inheritance.
- `schema-vendor-openconfig-augment-refine` — OpenConfig-inspired config/state
  groupings, composite protocol keys, cross-module augment, `uses` plus `refine`,
  defaults, mandatory leaves, and augmented choice/case projection.

## Samsung corpus audit (2026-10-02)

A local audit covered 2,437 YANG file paths (325 unique file contents) in 31
release bundles across four Samsung common-core provider projects. The models
remained local; the regression tests use small synthetic modules.

The audit compared parsed statements, arguments, and child order with the
vendored goyang reference, then built every bundle and its compatibility
projection using the consumer's vendor-validation, feature-retention,
refinement, and deviation policies. All files parsed with matching ASTs, and
all bundles compiled and projected without omitted content. Structural and
flattened payload views were compared separately so implicit cases and
operation IO were not mistaken for payload changes.

The remaining payload differences were accounted for: effective units,
normalized key whitespace, list-key mandatory/default semantics, inherited IO
namespaces, and valid nested `uses` augment content omitted by the reference.
No payload nodes were missing and no value-constraint differences remained
unexplained. The only load warnings concerned a duplicate vendor revision date
already tolerated by vendor-compatible validation.

Three compiler defects were reproduced before fixing them:

| Construct | Regression coverage |
|---|---|
| Extensions in imported groupings must resolve in the declaration's import scope | [`grouping_extensions_test.go`](../../go/cambium/grouping_extensions_test.go) covers missing consumer imports, different aliases, alias shadowing, and transitive grouping expansion. |
| Augments can add the first parameters to an RPC or action | [`schema_operation_test.go`](../../go/cambium/schema_operation_test.go) covers implicit IO, explicit order, resource limits, and choice ancestry. |
| Choice augments require implicit cases for shorthand children | [`schema_choice_augment_test.go`](../../go/cambium/schema_choice_augment_test.go) covers module and local `uses` augments, all shorthand kinds, metadata, qualified structural paths, and flattened order. |

The choice behavior follows [RFC 7950 sections 7.9.2 and
7.17](https://www.rfc-editor.org/rfc/rfc7950.html#section-7.17). TerraYANG has
matching consumer regressions for all three shapes. Its provider tests also
exposed a separate sample-generation issue with patterned strings and repeated
list keys; those fixes belong to TerraYANG rather than the schema compiler.

For future consumer upgrades, run the synthetic regressions against both the
previous dependency and the proposed dependency, compile the available release
bundles, then regenerate, build, and test each provider from its normal config.
Generated-provider tests with a mock NETCONF server do not establish live-device
compatibility.

## High-risk patterns to keep mining

These are the construct families where order or projection most often breaks;
keep growing owned coverage for them:

- YANG 1.1 submodule meshes where submodules include or reference sibling
  submodules.
- Augments into deep list/config/state paths, especially with the same local
  names across modules.
- Augments under choices/cases, and relative augments inside `uses`.
- Deviation matrices combining `not-supported`, `add`, `replace`, and `delete`.
- Large leafref surfaces, especially cross-module and relative paths.
- `ordered-by user`, `min-elements`, defaults, and mandatory nodes under
  augment / refine / deviation.
- `if-feature` on augments, deviations, `uses`, identities, enums, and bits —
  with both enabled and disabled feature sets represented in owned fixtures.
- `must` / `when` XPath context under augments, choices, refines, and deviations,
  especially when source-module prefixes differ from target-module prefixes.
- Vendor extensions that must be ignored semantically but preserved or tolerated
  structurally.

## See also

- [conformance.md](./conformance.md) — the shared corpus, the manifest, and how
  the gates run.
- [development.md](./development.md) — build/test/lint and the green-bar gate.
- [../overview.md](../overview.md) — the tiers and why declaration order
  matters.
- [../glossary.md](../glossary.md) — YANG and Cambium terms.
- [../README.md](../README.md) — documentation index.
