# Migration diff: Cambium versus goyang/ygot on order

This page is a running checklist for teams moving from openconfig/goyang and
ygot to Cambium. The focus is order correctness, because that is the structural
defect that Cambium fixes.

## What the comparison means

- **goyang** stores schema children in `Entry.Dir`, a `map[string]*Entry`. Go
  maps iterate in random order, so goyang sorts keys alphabetically before
  emitting output. The result is alphabetical child order, not schema
  declaration order.
- **ygot** generates Go structs from YANG. Insertion-order preservation for
  `ordered-by user` was added late and is opt-in via
  `generate_ordered_maps`. Nested ordered lists and keyless lists have
  historically been unsupported.
- **Cambium** walks libyang's `lyd_node.next`/`prev` sibling chain, so child
  order is exactly the schema declaration order. `ordered-by user` is a
  positional-only type by default.

## Reproducing the diff locally

The fastest demonstration is the shared conformance corpus. Run Cambium:

```bash
cargo run -p conformance-runner
```

Compare its output with goyang or ygot on the same input. The
`scrambled-children` fixture is deliberately designed to show the difference:

- Input order: `a`, `z`, `m`
- Cambium output order: `z`, `m`, `a` (schema order)
- goyang output order: `a`, `m`, `z` (alphabetical)

## Planned automation

A future CI job will:

1. Parse each `/conformance` fixture with Cambium and with goyang/ygot.
2. Report every case where goyang/ygot output differs from the libyang oracle.
3. Fail only on regressions in Cambium's output, treating the goyang/ygot diff
   as informational.

That job is not yet wired. This page documents the intended behavior so the
comparison criteria stay consistent.

## References

- [docs/ordering-story.md](ordering-story.md)
- [docs/cambium-kickoff.md](cambium-kickoff.md) §1.4 and §5
