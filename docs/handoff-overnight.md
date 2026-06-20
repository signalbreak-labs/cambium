> **⚠ SCOPE CORRECTION (2026-06-13).** Task 6 below (Terraform provider generator) and the
> NETCONF sink are **out of scope** for Cambium — they were moved to the
> `../cambium-generation-system-seed/` holding dir for a separate downstream repo. Tasks 3–5
> (schema tree, typed-struct codegen, gNMI JSON_IETF) stay. Cambium is the **goyang successor
> SDK**, not ygot. Kept verbatim as historical record.

# Overnight handoff — Cambium

**Session:** 2026-06-13 unattended overnight pass  
**Status:** green; no regressions

## What landed

### Task 3 — Schema-tree introspection (Rust + Go)
- Coarse FFI walk of `lysc_node` in declaration order in both stacks.
- Safe public types: `SchemaTree`, `SchemaNode`, `SchemaNodeKind` (Rust) and
  equivalents in `go/cambium`.
- Module namespace surfaced through `SchemaTree::module_ns()` /
  `SchemaTree.ModuleNs()`.
- Golden tests in both languages prove `order-demo` children under `top` walk as
  `z, m, a` (declaration order, not alphabetical).

### Task 4 — Codegen v0 (Rust + Go)
- **Rust:** `cambium_codegen::generate_rust` emits typed structs, a per-struct
  `FIELD_ORDER` manifest, and a deterministic XML serializer. The generated code
  is compiled and executed by `rust/cambium-codegen/tests/codegen_order_demo.rs`;
  output is byte-identical to libyang for the order-demo fixture.
- **Go:** `codegen.GenerateGo` mirrors the emitter shape (structs +
  `FieldOrder` manifest + deterministic XML serializer skeleton). Source
  determinism and structure are tested; execution of generated Go code is
  deferred to the next pass.

### Task 5 — gNMI JSON_IETF codec (I6)
- Added `Format::JsonIetf` / `FormatJSONIETF` in both languages, mapping to
  libyang's JSON printer with `LYD_PRINT_EMPTY_CONT` to preserve empty
  non-presence containers.
- `rust/cambium-core/tests/json_ietf.rs` and `go/cambium/json_ietf_test.go`
  prove the `ordered-user` list serializes as a JSON array in insertion order
  (`c, a, b`), never decomposed into per-leaf updates.

### Task 6 — Terraform provider generator skeleton (Go)
- Added `go/providergen` to emit a `terraform-plugin-framework` provider from a
  compiled `cambium.SchemaTree`.
- Added `go/internal/netconfsink`, a fake NETCONF client that records
  `<edit-config>` payloads for tests.
- Generated package layout: `provider.go`, `netconf_client.go`, `models.go`,
  `xml.go`, `resource_<container>.go`, and `provider_test.go`.
- Generated `Create`/`Update` call order-correct `Apply*` helpers that serialize
  container children in schema declaration order.
- `go/providergen/generate_test.go` generates a provider for `order-demo`,
  writes a temporary `go.mod` with a `replace` to the local Cambium Go module,
  runs `go mod tidy`, and runs the generated tests. The test asserts that the
  emitted XML contains `z`, `m`, `a` in schema order.
- Wired `cambium generate-provider <module-dir> <module-name> <out-dir>` into
  `go/cmd/cambium/main.go`.

### Task 7 — Codegen v1: lists, leaf-lists, typed leaves (Rust + Go)
- Rust: updated `cambium_codegen::generate_rust` to emit `Vec<T>` for lists and
  leaf-lists, move list key fields to the front of generated structs and the
  field-order manifest, and choose `String`, `i64`, or `bool` from the schema
  leaf type.
- Added `rust/cambium-codegen/tests/codegen_v1.rs`: proves the `keys-first`
  fixture serializes keys before non-key leaves, and proves a temporary module
  with `boolean`, `int64`, and `leaf-list` produces byte-identical XML.
- Go: updated `codegen.GenerateGo` with the same v1 features and a generated
  `valueString` helper for typed fields.
- Extended `go/codegen/codegen_test.go` to compile and run generated Go code for
  `order-demo`, `keys-first`, and the temporary typed-leaf/leaf-list module,
  asserting byte parity with libyang golden output.

## Green bar (run after every task)

```text
bash go/internal/libyang/build.sh      # already built
(cd go && go vet ./... && go test -race ./... && go run ./cmd/cambium all)
cargo clippy --workspace --all-targets -- -D warnings
cargo test --workspace
cargo run -p conformance-runner
bash scripts/diff-engine-config.sh
```

All pass:
- Go: 6/6 conformance fixtures byte-for-byte.
- Rust: 6/6 conformance fixtures byte-for-byte.
- Engine build config diff: flags match.
- Generated `order-demo` provider compiles and passes its schema-order apply
  test against the fake NETCONF sink.

## Deferred work

- **Go codegen execution test:** the Go emitter is source-deterministic and
  structurally correct, but generated Go code is not yet compiled/executed in
  CI. Extend `go/codegen/codegen_test.go` to write generated source to
  `target/generated` and run `go test`/`go run` against it, mirroring the Rust
  rustc execution test.
- **Provider generator v1:** add lists, leaf-lists, typed framework attributes,
  and a real `Read` implementation that parses device replies.

## Files to review first

- `rust/cambium-core/src/schema.rs` + `context.rs` — safe schema-tree API.
- `rust/cambium-libyang-sys/src/adapter.rs` — coarse FFI schema walk.
- `rust/cambium-codegen/src/lib.rs` + `tests/codegen_order_demo.rs` +
  `tests/codegen_v1.rs` — Rust v1 codegen and acceptance tests.
- `go/internal/libyang/schema.go` + `go/cambium/schema.go` — Go schema-tree
  parity.
- `go/codegen/codegen.go` + `codegen_test.go` — Go v1 codegen parity with
  compiled/executed generated tests.
- `go/providergen/generate.go` + `generate_test.go` — Terraform provider
  generator and its acceptance test.
- `go/internal/netconfsink/sink.go` — fake NETCONF client.
- `go/cmd/cambium/main.go` — `generate-provider` CLI subcommand.
- `docs/terraform-provider-design.md` — current generator design.
- `.planning/overnight-log.md` — dated entries for every task.
