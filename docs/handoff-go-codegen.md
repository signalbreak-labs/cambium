# Handoff — Cambium Go Codegen Parity (Foundation + Typed Scalars)

**Branch:** `feat/go-codegen-parity` (branched from `feat/cambium-sdk-phase1`)
**Last verified commit:** `ffc8f21`
**Status:** Slices 1 and 2 landed, byte-clean. All deferred features scoped for next agent.

---

## Public-signature decision

`GenerateGo(ctx *cambium.Context, module string) (string, error)` is kept as the Go entry point.
The manifest is exposed only as in-source `var <Struct>FieldOrder = []string{...}` per generated
struct. No `GeneratedModule` struct is returned.

**Why:** `spec/api.md:42-54` sanctions only runtime-shape differences between Rust and Go
(`Result`→`(T,error)`, `Option`→pointer, etc.). The Rust `GeneratedModule { source, field_order }`
shape is an emitter-internal convenience; Go callers consuming generated code only need the source
string, and the field-order list is already embedded in that source. Returning a wrapper struct
would add an API surface with no proven user benefit.

No symmetric `CodegenOpts` is added. `dedup_groupings` and `with_path_builder` are documented as
unsupported on both sides (`kimi-go-codegen-prompt.md` Section 8).

---

## Slices landed

### Slice 1 — Foundation

Rewired `GenerateGo` from the deprecated `ctx.SchemaTree` API to the rich
`ctx.Schema(module) → Module → SchemaNodeRef → TypeInfo` API.

Landed in earlier commits on `feat/go-codegen-parity` (see `git log`).

Gated fixtures (Go `ToXML()`/`ToJSONIETF()` `bytes.Equal` the libyang golden):

- `scrambled-children` (XML + JSON)
- `keys-first` (XML + JSON)
- `json-ietf-module-namespace-qualification` (JSON)

Harness assertions:
- `go/format.Source(src) == src` (gofmt-stable)
- generated module compiles and passes `go test -v ./...`
- generated module passes `go vet ./...`
- determinism (`TestGenerateGoIsDeterministic`)

### Slice 2 — Typed Scalars

Added typed scalar emission:

- `bool` → `bool`, `JsonKind::Bool`
- `int8..int64` / `uint8..uint64` → exact-width Go types; ranged ints emit
  `<Prefix><Field>Range` newtypes with `New`, `Get`, `String`, multi-range OR-check,
  and bounds-respecting `Default` (min when 0 is out of range)
- `decimal64` → generated `Decimal64` helper (`raw int64`, `fractionDigits uint8`)
  with RFC-7950 canonical `String()` and `JsonKind::QuotedNumber`
- `string` with `length` → `<Prefix><Field>Length` newtype with constructor validation;
  plain `string` otherwise

Gated fixtures (all `PASS` in `go/codegen`):

| Fixture | XML | JSON | Notes |
|---|---|---|---|
| `types-boolean-default-false` | ✓ | ✓ | default-bearing |
| `types-int-int8-range` | ✓ | ✓ | |
| `types-int-int16-range` | ✓ | ✓ | |
| `types-int-int32-range-multipart` | ✓ | ✓ | multi-range `\|`, signed |
| `types-int-int64-range-quoted` | — | ✓ | JSON quotes `int64` per RFC-7951 |
| `types-uint-uint16-range-port` | — | ✓ | `Default` exercised |
| `types-uint-uint32-range-multi` | ✓ | ✓ | |
| `types-uint-uint64-range-quoted` | — | ✓ | JSON quotes `uint64` per RFC-7951 |
| `types-decimal64-fraction1-range` | ✓ | ✓ | negative fractional |
| `types-decimal64-fraction2-canonical-round` | ✓ | ✓ | trailing-zero trim |
| `types-decimal64-fraction3-and-6` | ✓ | ✓ | small fractions |
| `types-decimal64-fraction9-negative` | ✓ | ✓ | very small negative |
| `types-decimal64-fraction18-max-magnitude` | ✓ | ✓ | max `int64` magnitude |
| `json-ietf-decimal64-canonical-quoting` | — | ✓ | quoting + canonical `0.0` |
| `types-string-length-pattern-anchor-posix` | ✓ | ✓ | length + `Default` |
| `types-range-length-min-max-keywords` | ✓ | ✓ | `min`/`max` keywords |
| `json-ietf-list-array-keys-first` | ✓ | ✓ | typed `uint16` `mtu`, keys-first |

Rust `codegen_v2.rs` coverage for the Slice-2 fixtures is **partial**:
- Covered: `types-decimal64-fraction{1,2,3-and-6,9-negative,18-max-magnitude}`,
  `types-int-int8-range`, `types-string-length-pattern-anchor-posix`
- **Golden-only for this run:** all other Slice-2 fixtures listed above.
  They are gated against `conformance/golden/<fixture>/output.{xml,json}` (the libyang oracle),
  so Go output is byte-correct, but transitivity "Go == Rust" is not proven for them.

---

## Deferred features (hard stop — do NOT half-land)

If any of these does not reach byte-clean green in a single focused run, revert its changes and
extend this handoff with the failed red test.

### 1. `has_content` + empty-container handling

- **Rust refs:** `rust/cambium-codegen/src/lib.rs:1335-1374` (`has_content`), `1649-1675`
  (XML self-closing), `1300-1318` (JSON `{}` omission).
- **What it does:** mandatory leaf ⇒ true; optional leaf / optional container / presence container
  ⇒ present (non-nil); non-presence container ⇒ recurse `hasContent()`; list/leaf-list ⇒ `len > 0`.
- **Target fixtures:** `json-ietf-string-escaping-control-unicode` (also hard-gates JSON escaping),
  plus any `container-presence-empty` / `json-ietf-presence-vs-nonpresence` fixtures.
- **Next red test:** generate `json-ietf-string-escaping-control-unicode`, construct a struct with
  control/unicode string values, and assert `ToJSONIETF()` equals the golden.

### 2. enum + bits + identifier safety

- **Rust refs:** `lib.rs:480-488, 577-637` (enum), `489-493, 639-744` (bits), `2180-2213`
  (`safe_field_ident`).
- **Key work:** port `safe_field_ident` (digit-leading fix, hyphen/dot→underscore, Go keyword
  avoidance, collision disambiguation); emit `<Prefix><Field>Enum` and bits newtype with
  declaration-order `BIT_POSITIONS`.
- **Target fixtures:** `types-enumeration-explicit-values-sparse`,
  `types-enum-bits-auto-position`, `types-enumeration-zero-value-disabled`,
  `idents-enum-value-collision`, `types-bits-explicit-positions-gaps`,
  `idents-collision-hyphen-underscore`, `idents-keywords-go`,
  `idents-container-leaf-collision`, `idents-long-name`, `idents-unicode-mixed-case`.
- **Next red test:** `types-enumeration-explicit-values-sparse` — generate struct, construct with
  enum value, assert XML/JSON equals golden.

### 3. leafref realtype chasing

- **Rust refs:** `lib.rs:423-451`.
- **Rule:** chase `ResolvedLeafRef.Realtype()` only; if unset, degrade to `string`.
  **Do NOT use `ResolvedLeafRef.Target()` — it returns nil today.**
- **Target fixtures:** `types-leafref-to-list-key`, `types-leafref-to-leafref-chain`,
  `types-leafref-cross-module`.
- **Next red test:** `types-leafref-to-list-key` byte gate.

### 4. ordered-by user positional type

- **Rust refs:** `lib.rs:2433-2517`.
- **Rule:** branch on `OrderedBy() == OrderedByUser`; system-ordered stays `[]T`.
  Expose only positional mutators (`InsertFirst/Last/Before/After`, `Move*`, `Remove`, `Len`,
  `IsEmpty`, `Get`, `Iter`, `From([]T)`). No append, no index-assign.
- **Target fixtures:** `leaflist-ordered-by-user`, `list-ordered-by-user-insertion`,
  `ordered-user`, `ordering-nested-user-cascading`, `system-list-canonical`,
  `json-ietf-leaflist-array-user-system`.
- **Next red test:** `list-ordered-by-user-insertion` byte gate.

### 5. identityref incl. foreign-module XML prefix/xmlns

- **Rust refs:** `lib.rs:494-515, 746-876, 1592-1647, 1994-2024`.
- **Key work:** enum with `AsName`/`AsJSONName`/`FromName`; transitive `Derived()` closure sorted
  by `module:name`; synthesize `<wire xmlns:PFX="NS">PFX:name</wire>` for foreign identities;
  degrade to `string` on no base.
- **Target fixtures:** `types-identityref-single-base`, `-multiple-bases`,
  `-derived-hierarchy`, `-foreign-module-prefix`, `identityref-iana-if-type-foreign`.
- **Next red test:** `types-identityref-single-base` byte gate.

### 6. typed unions

- **Rust refs:** `lib.rs:544-548, 878-1080, 2048-2068`.
- **Key work:** discriminated type with per-member quoting; member naming via typedef name or
  base-type label; empty-union fallback.
- **Target fixtures:** `types-union-enum-and-scalar`,
  `types-union-heterogeneous-members-quoting`, `types-union-identityref-member`,
  `types-union-leafref-member`, `types-union-scalar-all-members`,
  `types-union-two-identityrefs-distinct-bases`, `json-ietf-leafref-union-resolved-form`.
- **Next red test:** `types-union-scalar-all-members` byte gate.

---

## No new `go/cambium` accessors added

All required schema introspection (`Module`, `SchemaNodeRef`, `TypeInfo.Resolved()`,
`RangeBound`, `Identity.Derived()`) was already present. The Go codegen remains libyang-free:
`go/codegen` imports only `github.com/signalbreak-labs/cambium/go/cambium`; no `import "C"`,
`unsafe`, or `internal/libyang`.

---

## Golden resolution contract

The Go codegen gate uses the **same** manifest lookup as the conformance runner:

- `conformance/manifest.toml` `[case.expected]` keys → format
- `xml` → `cambium.FormatXML`
- `json` → `cambium.FormatJSON`
- `json_ietf` / `json-ietf` → `cambium.FormatJSONIETF`

The JSON oracle for codegen is `cambium.FormatJSON` (matching Rust `libyang_reference_json_from_input`
which serializes with `Format::Json`); the Go method is named `ToJSONIETF` for API symmetry only.

Comparison uses `bytes.Equal` with **no** `TrimRight`; the generated serializers reproduce the
single deterministic trailing newline in the libyang goldens.

---

## Verified floor (run before handing off)

```bash
cd /Users/recursive/workspace/projects/github/signalbreak-labs/cambium/go
CGO_ENABLED=1 go build ./...
CGO_ENABLED=1 go vet ./...
CGO_ENABLED=1 go test -race ./...
golangci-lint run
gofmt -l .                      # empty
go run ./cmd/cambium all        # conformance: 193 passed, 0 failed

cd /Users/recursive/workspace/projects/github/signalbreak-labs/cambium
cargo test --workspace
cargo clippy --workspace --all-targets -- -D warnings
```

All commands green on commit `ffc8f21`.

---

## Resume checklist for next agent

1. Read this handoff and `docs/kimi-go-codegen-prompt.md` Sections 6–9.
2. Pick the next deferred slice (recommended order: `has_content`, then enum/bits/idents, then
   leafref, then user-ordered, then identityref, then unions).
3. Write the failing Go byte-gate test against the golden oracle first.
4. Implement the emitter arm, keeping generated code stdlib-only and gofmt/vet/lint clean.
5. Run the verified-floor commands; do not commit if any are red.
