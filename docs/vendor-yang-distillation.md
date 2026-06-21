# Vendor YANG Distillation

Cambium does not vendor proprietary device model packets into conformance.
Vendor repositories are mined as references, then reduced to small owned
fixtures that preserve the shape that matters to Cambium: effective schema
order, augment/deviation behavior, submodule/import topology, list keys,
defaults, mandatory nodes, and projection traversal.

## Source Classes

Use public upstreams as scratch inputs only:

- Nokia SR OS: `https://github.com/nokia/7x50_YangModels`
- Nokia SR Linux: `https://github.com/nokia/srlinux-yang-models`
- OpenConfig: `https://github.com/openconfig/public`
- Cross-vendor survey inputs, if needed: public `YangModels/yang` vendor
  directories, read only for construct census and never copied into
  `/conformance`.

## Workflow

1. Clone or update vendor repositories under `/.planning/` or another ignored
   scratch directory.
2. Run `scripts/yang-corpus-census.py` against the local checkout.
3. Read the aggregate counts and example statement locations.
4. Author a small owned fixture under `/conformance/fixtures`.
5. Add it to `/conformance/manifest.toml`.
6. Prefer `tier = "schema-ir"` for schema-shape and projection hazards. Use
   backend/data fixtures only when a payload or serialization byte invariant is
   being asserted.
7. Run the focused schema/projection tests and then `scripts/green-bar.sh`.

Example:

```sh
git clone -b sros_24.10 --depth 1 https://github.com/nokia/7x50_YangModels .planning/vendor/7x50
scripts/yang-corpus-census.py \
  .planning/vendor/7x50/YANG \
  --json-out .planning/vendor/7x50-census.json \
  --markdown-out .planning/vendor/7x50-census.md
```

For OpenConfig:

```sh
git clone --depth 1 https://github.com/openconfig/public .planning/vendor/openconfig-public
scripts/yang-corpus-census.py \
  .planning/vendor/openconfig-public/release/models \
  --json-out .planning/vendor/openconfig-census.json \
  --markdown-out .planning/vendor/openconfig-census.md
```

## Fixture Policy

Owned fixtures should be minimal and synthetic:

- Keep module names under `schema-vendor-*` or another Cambium-owned namespace.
- Preserve the vendor-inspired construct combination, not vendor text.
- Use tiny representative paths and names.
- Include module-qualified paths when an augment can collide by local name.
- Avoid product names, proprietary descriptions, copied comments, and copied
  typedef bodies.
- Add projection coverage by relying on
  `TestProjectionConformanceManifestFixtures`, which runs every manifest case
  through `ProjectSubtree` and `ProjectSchemaPaths`.

## Current Distilled Fixtures

- `schema-vendor-nokia-submodule-mesh`: Nokia SR OS-inspired multi-submodule
  topology with sibling definitions, a submodule-level include, defaults,
  mandatory leaves, list keys, and effective include-site ordering.
- `schema-vendor-nokia-feature-deviation`: Nokia SR OS / SR Linux-inspired
  feature-gated augment plus deviation matrix covering `not-supported`,
  `replace`, if-feature-controlled deviations, ordered lists and leaf-lists,
  min/max-elements, leafrefs, `must`, defaults, mandatory nodes, and config
  inheritance.
- `schema-vendor-openconfig-augment-refine`: OpenConfig-inspired
  config/state groupings, composite protocol keys, cross-module augment,
  `uses` plus `refine`, defaults, mandatory leaves, and augmented choice/case
  projection.

## High-Risk Patterns To Keep Mining

- YANG 1.1 submodule meshes where submodules include or reference sibling
  submodules.
- Augments into deep list/config/state paths, especially with same local names
  across modules.
- Augments under choices/cases and relative augments inside `uses`.
- Deviation matrices combining `not-supported`, `add`, `replace`, and `delete`.
- Large leafref surfaces, especially cross-module and relative paths.
- `ordered-by user`, `min-elements`, defaults, and mandatory nodes under
  augment/refine/deviation.
- `if-feature` on augments, deviations, `uses`, identities, enums, and bits,
  with both enabled and disabled feature sets represented in owned fixtures.
- `must` / `when` XPath context under augments, choices, refines, and
  deviations, especially when source-module prefixes differ from target-module
  prefixes.
- Vendor extensions that must be ignored semantically but preserved or tolerated
  structurally.
