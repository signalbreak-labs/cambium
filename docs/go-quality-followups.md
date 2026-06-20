# Go Quality Follow-ups (2026-06-20)

Prioritized engineering-quality backlog from the Go code audit (hexagonal design,
maintainability, codegen templates). **Verdict: the architecture is sound** —
hexagonal purity is machine-enforced (the pure-Go core has a verified zero
cgo/libyang dependency closure; the libyang adapter is isolated behind
`//go:build cgo`; dependencies point strictly inward), the order-as-structure
invariant is honored (slice-authoritative traversal, maps only as caches), and
the byte-exact serializer is golden-gated. The open items are **maintainability
grade, not correctness or layering.**

## Done (this pass)

- **Dead code** — removed the `filepath` import-keep hack in `cambium/schema.go`
  and the colliding unused `lysSetType`/`lysSetValue` constants in
  `internal/libyang/schema.go`; deleted empty scratch dirs.
- **Conformance runner** — renamed the misleading `cambium`-aliased backend
  import to `backend`; de-Rusted its package doc.
- **CGO=0 fitness lane** — the pure gate now runs the `!cgo` fitness tests
  (`conformance/nocgo_test.go`, boundary tests) the cgo lane silently excluded.
- **Codegen templates** — the 8 fully-static generated-code helper blocks
  (~600 hand-escaped `WriteString` lines) are now `//go:embed` files emitted
  verbatim, regenerable via `CAMBIUM_REGEN_TEMPLATES=1 go test ./codegen
  -run TestRegenerateStaticTemplates`. Byte-output unchanged (compile+run gate).
- **Leak** — `compat` module⇄schema/entry association moved from a
  process-lifetime global `sync.Map` (never `Delete`d) onto unexported `*Module`
  fields; GC'd with the module, isolated per parse.
- **God-file split (R6)** — all four split by concern via pure same-package
  relocation (goimports-fixed imports, behavior-bit-identical, golden-gated):
  `codegen.go` 5612→2252 (+ `_serialize` 1425, `_validate` 1085, `_jsonparse`
  895); `cambium/schema.go` 8783→5921 (+ `_placement` 842, `_types` 659,
  `_augment` 501, `_pattern` 366, `_xpath` 357, `_iffeature` 233);
  `compat/entry.go` 3877→2479 (+ `_types` 820, `_build` 608); `compat/modules.go`
  3541→3112 (+ `_load` 439). (Further core-of-core decomposition — `schema.go`
  API vs IR-build vs validation passes — is left as a smaller follow-up; those
  are more entangled with the core types/accessors.)
- **Helper dedup (R13, partial)** — folded `compat`'s `prefixedPathPart` and
  `localPart` onto `splitPrefix`, and the duplicated `pathNodeName` onto an
  exported `internal/libyang.PathNodeName` reused by the backend. (The decimal64
  formatting triplication and the two integer-trait sets are left: they straddle
  the cgo/pure boundary / different enums for little gain.)
- **Doc comments (R15, partial)** — name-prefixed docs on the ordering-sensitive
  accessors (`TopLevel`/`Children`/`DataChildren`/`ListKeys`/`KeyNames`,
  why=I2/I3) and a provably-redundant `err != nil` removed. (Remaining gopls
  hints — the `if target != nil` guards flagged tautological, the error-dedup
  `reflect.DeepEqual`, and a test composite-literal — are left as defensive /
  behavior-risky / test-only; none are golangci-gated.)
- **Table-drive placement validators (R7)** — ~21 `validateNested*StatementChild
  Placement` funcs + ~24 `*ChildAllowed` predicates collapsed to one
  `nestedChildRules` table + one generic `validateNestedChildPlacement`
  (`schema_placement.go` 835→398). Allowed-sets + diagnostic string byte-identical
  (proven by a temporary table-vs-predicate equivalence test over the full
  keyword universe before deletion). Irregular validators (deviate's operation,
  input/output's format) kept.
- **Metadata helper to text/template (R10)** — `emitMetadataHelper` (~145 lines
  of hand-escaped WriteString) replaced by `templates/metadata.go.tmpl`: static
  RFC-7952 helpers read as plain Go, the two data-driven switches are `{{range}}`
  blocks over a `metadataView`. Byte output unchanged (gofmt-normalized; golden).
- **Share libyang serialize lifecycle (R14)** — `RawDataTree`/`RawDataDiff`
  `Serialize`/`SerializeLYB` folded onto shared `serializeNode`/`serializeNodeLYB`
  (diff keeps its nil-tree guards; `emptyIsError` preserves the null-result
  difference). Serializer goldens + conformance unchanged.

- **Type-helper templates (R11)** — `ensureEnum`/`ensureBits`/`ensureIdentityref`
  converted from Sprintf-assembly to `text/template`s (`enum`/`bits`/
  `identityref.go.tmpl`) over computed views via a shared `renderType`. Byte
  output unchanged (golden). `ensureUnion` and the small `emitStructDefinition`/
  `emitFieldOrder` shells are intentionally left as code: the union's nested
  `Resolved*` handling is value-encoding logic (per the do-not-templatize
  boundary), and the struct shells are already small structured `Fprintf`, not
  string-fmt walls.
- **Decimal64 formatting dedup (R13b)** — `internal/libyang.formatDecimal64` and
  the backend's `Decimal64.String` folded onto exported
  `FormatDecimal64(raw, fractionDigits, trim)`. The integer-trait dedup is also
  done: `intBitSize` now delegates through the existing `intKind(BaseType)`
  mapping to `intKindBitSize` (one width table). (`isSignedIntBase` stays a
  one-line range check — delegating it through `intKind` would flip non-integer
  bases from `false` to signed.)
- **Doc comments (R15b)** — the 12 `ResolvedType` implementations and the 53
  public enum constants (`SchemaNodeKind`/`Config`/`Status`/`OrderedBy`/
  `LeafType`/`BaseType`/`IntKind`) now carry name-prefixed godoc.
- **Codegen templating complete (R11)** — `emitStructDefinition`/`emitFieldOrder`
  and `ensureUnion` moved to text/templates (`structdef`/`fieldorder`/
  `union.go.tmpl`); the union keeps per-member payload/method computation in Go
  and renders the shell. Byte output unchanged (full codegen golden suite).

## Open

**None.** The audit backlog is complete. Two things are deliberately left as code
(not gaps): `isSignedIntBase` (a one-line range check) and the tight
value-encoding/validation/JSON-parse emitters (`emitFieldXML`/`emitFieldJSON`/
`emitScalarFieldValidationAt`/`unionVariantMethods`/parse-attempt chains) — nested
`switch`-on-`Resolved*` with indent threading where templating would obscure
control flow. compat's AST type aliases mirror upstream goyang (documented in
`internal/yangparse/upstream/UPSTREAM.md`).

## Notes
- `goimports` is installed (used for the R6 splits); keep it in the dev/CI
  toolchain so future file moves stay trivial and safe.
- Every change was behavior-preserving; the golden conformance corpus +
  `go test ./codegen` (compile+run) + the goyang parity suites are the gates.
