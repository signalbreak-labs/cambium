# ADR 0006: Bounded node-table SchemaIR (`cambium.schema-ir.v2`)

- Status: Accepted
- Date: 2026-09-27

## Context

`cambium.schema-ir.v1` ([ADR 0002](0002-versioned-schemair-export.md)) nests
full copies of every node's `Children`, `DataChildren`, and `ListKeys`
subtrees. Ordinary schemas repeat subtrees across those views: a chain of
nested containers ending in one leaf is reachable from both `Children` and
`DataChildren` at every level, so record count doubles per level. Measured on
this repository at depth 4, 8, 12, and 16: 31, 511, 8,191, and 131,071 records
for 5, 9, 13, and 17 unique nodes; `cambium-ir` output grew from 55 KB to
24.8 MB between depth 4 and 12, and `SchemaIR()` allocated 480 MB at depth 16.
Memoizing construction would not fix serialized duplication, and removing the
nested arrays from v1 would change its field and path semantics, which ADR 0002
says requires a new version string.

## Decision

- Add `cambium.schema-ir.v2`, a node table: `Context.SchemaIRTable()` returns
  `SchemaIRTable{Version, Modules, Nodes, Errors}`. Each schema node appears
  once in `Nodes`, in pre-order of the structural walk over modules in context
  load order, with `ID` equal to its index and `Parent` its structural parent
  (`SchemaIRNoParent` at module top level). `Children`, `DataChildren`, and
  `ListKeys` are ordered `SchemaIRNodeID` references with v1 ordering
  semantics. Per-node facts are the v1 facts.
- Keep v1 unchanged. Add `Context.SchemaIRStats()`, which counts unique nodes,
  relationships, path bytes, and the records v1 would materialize without
  materializing it, and `Context.SchemaIRWithLimit(max)`, which returns a
  `resource_limit` diagnostic instead of building an oversized v1 projection.
- `cmd/cambium-ir` keeps v1 as its default output, bounded by `-max-records`
  (default 1,048,576), and emits v2 with `-format v2`.
- Schema diffs no longer build a v1 projection to enumerate modules.

## Consequences

- Records equal unique nodes and references equal relationships for any depth
  or view overlap; only absolute path strings grow with depth. At depth 12 the
  v2 JSON is 18 KB against v1's 24.8 MB.
- v1 consumers are unaffected until they hit the limit, which fails with a
  structured diagnostic pointing at v2 instead of exhausting memory.
- The v2 per-node metadata contract is as narrow as v1's; widening either is an
  additive change under the ADR 0002 policy, tracked separately from bounding.

## Reversal cost

Low for the Go API (additive), moderate once external tools consume v2 JSON;
the version string keeps any later change explicit.

## Alternatives considered

- **Drop nested arrays from v1** — rejected: silently changes v1 semantics.
- **Memoize v1 construction** — rejected: shares Go values but still
  serializes every copy.
- **Only a hard limit on v1** — rejected alone: it bounds failure but leaves no
  supported path for deep schemas.
