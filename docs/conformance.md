# Conformance

This document explains Cambium's shared conformance corpus: the fixtures and
golden outputs under `/conformance`, how each case is tagged with the ordering
invariants (I1–I6) it exercises, how cases are split between the Schema-IR tier
and the Backend/data tier, and how the gates are run. Because the corpus is the
contract — not any single binding's source — its golden outputs are reusable by
any future language binding that runs the same cases.

## Why a shared corpus

Cambium's reason to exist is order correctness: schema child order, list-key
order, RPC/action I/O order, and `ordered-by user` insertion order all carry
meaning (RFC 7950 §7.7.7, §7.8.5, §7.8.6, §7.14–7.15). Ordering bugs are easy to
introduce and hard to catch by reading code, because the wrong order is still
*valid* YANG — it just no longer matches what a NETCONF server expects on the
wire. The corpus pins that behavior down as data: a fixture in, a byte-exact
golden out. A regression that reorders a single sibling is then a failing test,
not a silent change.

Keeping the corpus in `/conformance` — separate from `/go` — is deliberate.
`/spec` and `/conformance` form a language-neutral contract: parity is defined by
the spec plus the shared cases and golden outputs, not by which language landed
first. A future binding under `/<lang>/` reuses the exact same fixtures and
`golden/` files through its own runner.

## Layout

```text
/conformance/
  manifest.toml        # the case list: name, tier, invariants, inputs, expected outputs
  fixtures/<case>/     # per-case YANG modules + input data (or expected-ir.json)
  golden/<case>/       # per-case expected serialized outputs (output.xml, .json, .json_ietf)
  corpus/              # shared real-world YANG (IETF/IANA modules) used by several cases
```

A typical Backend/data fixture is a module directory plus an input document:

```text
fixtures/declaration-order-out-of-alphabetical/
  module/            # the YANG module(s) for the case
  input.xml          # the data to parse
golden/declaration-order-out-of-alphabetical/
  output.xml
  output.json
  output.json_ietf
```

The golden output for that case shows the point directly — the fixture declares
leaves out of alphabetical order, and the serialized result preserves *schema
declaration order*, not a sorted order:

```xml
<system xmlns="urn:declaration-order-out-of-alphabetical">
  <zebra>4</zebra>
  <apple>1</apple>
  <mango>3</mango>
  <banana>2</banana>
</system>
```

## The manifest

`manifest.toml` is the single index of cases. Each `[[case]]` entry names the
case, declares its tier, lists the ordering invariants it exercises, points at
its fixture inputs, and lists the expected outputs to compare against.

A Backend/data case (the default tier) names an input plus a `[case.expected]`
table of `format -> golden path`:

```toml
[[case]]
name = "scrambled-children"
invariants = ["I2"]
module = "fixtures/scrambled-children/module"
input = "fixtures/scrambled-children/input.xml"
input-format = "xml"
oracle = true
[case.expected]
xml = "golden/scrambled-children/output.xml"
json = "golden/scrambled-children/output.json"
```

A Schema-IR case has no data input and no serialized golden. Instead it sets
`tier = "schema-ir"` and points at an `expected-ir.json` describing the ordered
schema structure to assert:

```toml
[[case]]
name = "schema-cross-kind-order"
invariants = ["I2"]
tier = "schema-ir"
module = "fixtures/schema-cross-kind-order/module"
expected-ir = "fixtures/schema-cross-kind-order/expected-ir.json"
```

The `expected-ir.json` records the ordered effective-schema facts a fixture is
meant to lock in — for example the children of a node in declaration order and a
list's keys in key-statement order:

```json
{
  "module": "schema-cross-kind-order",
  "children": {
    "/schema-cross-kind-order/root": ["a", "b", "c", "g1", "g2", "d", "e"]
  },
  "keys": {
    "/schema-cross-kind-order/root/d": ["name"]
  }
}
```

`oracle = true` marks a case whose serialized output can be cross-checked against
an independent `yanglint` run when one is available (see "Running the gates").

## Cases are tagged by invariant

Each case carries an `invariants` array naming the ordering invariants (I1–I6) it
exercises, so the corpus maps directly onto the normative contract in
[`/spec/ordering-invariants.md`](../spec/ordering-invariants.md). For example:

- **I2** — schema children in effective schema declaration order — is the tag on
  `scrambled-children`, `declaration-order-out-of-alphabetical`, and the
  `schema-*` Schema-IR cases.
- **I3** — list keys serialized first, in key-statement order.
- **I4** — RPC/action/notification children in schema order.
- **I1** — `ordered-by user` entry order preserved across parse → tree →
  serialize (carried by the `ordered-user` cases; some cases tag `["I1", "I2"]`).
- **I5** — JSON arrays carry I1/I2 order under a deterministic printer profile.

The invariant tags are how coverage is read: the normative wording for I1–I6
lives in the spec, and the corpus is the executable proof that each tier upholds
its share of them. Treat coverage as a floor — every observable ordering
invariant has at least one fixture, and new ordering behavior adds a fixture
before the code.

## Schema-IR cases vs Backend/data cases

The two tiers (see [`architecture.md`](./architecture.md)) are gated by two
different kinds of case, distinguished by `tier`:

- **Schema-IR cases** (`tier = "schema-ir"`) run in the
  **Schema-IR tier (pure Go, `CGO_ENABLED=0`)**. They assert ordered schema
  structure from `expected-ir.json` — children, data children, keys, imports,
  prefixes, derived identities, leafref targets — with no data parsing and no
  serialized bytes. They guarantee the schema-level invariants **I2/I3/I4**.
  Backend runners skip them.

- **Backend/data cases** (the default, `tier = "backend-data"`) run in the
  **Backend/data tier (optional, requires cgo)**. They parse `input` through the
  libyang-backed engine and assert byte-for-byte equality against every golden
  format in `[case.expected]`. They additionally guarantee the data-tier
  invariants **I1/I5/I6** over real data.

The manifest is required to contain both kinds: a Schema-IR case must declare
`expected-ir` and carry no backend-data fields, and a Backend/data case must
declare `input`, `input-format`, and at least one expected output. That shape
contract is itself checked by a cgo-free fitness test so the corpus can never
drift into a tier-ambiguous state.

The same fixture style is what a future `/<lang>/` binding consumes: its
schema-introspection layer runs the Schema-IR cases against the same
`expected-ir.json`, and its engine-backed data tier runs the Backend/data cases
against the same `golden/` bytes.

## Golden outputs are reused across bindings

The `golden/<case>/` files are the canonical expected outputs for the corpus, not
artifacts of the Go runner. The byte-for-byte comparison normalizes only trailing
whitespace before comparing (LYB binary output is compared exactly). That keeps
the goldens portable: any binding's runner reads the same `output.xml`,
`output.json`, and `output.json_ietf` and asserts the same bytes. This is what
makes parity a property of the contract rather than of one implementation — the
goal called out in the project's "Adding a language binding" guidance.

The Go typed-struct generator participates in the same discipline: generated
serializers walk Cambium's ordered field-order manifest (keys first, declaration
order after), producing output byte-identical to libyang's — which is exactly
what the shared goldens encode. See [`guides/codegen.md`](./guides/codegen.md).

## Running the gates

The full local gate is one script:

```bash
scripts/green-bar.sh
```

It runs both tiers in sequence. The two halves can also be run on their own.

### The cgo-free pure gate (Schema-IR tier)

This half needs no C toolchain and no libyang build:

```bash
scripts/check-go-default-pure.sh
cd go && CGO_ENABLED=0 go vet  ./cambium ./codegen ./compat
cd go && CGO_ENABLED=0 go test ./cambium ./codegen ./compat
```

`check-go-default-pure.sh` does two jobs: it runs the cgo-free (`!cgo`) tests —
including the Schema-IR conformance test that drives the `schema-ir` cases and
the manifest-shape fitness test — and it asserts the public `cambium`/`codegen`/
`compat` import closure contains no cgo, no `internal/libyang`, no
`libyangbackend`, and no `goyang` packages. That is the machine enforcement of
the Schema-IR tier's cgo-free guarantee.

### The cgo backend gate (Backend/data tier)

This half builds the vendored engine, then runs the Backend/data cases through
the cgo runner:

```bash
bash go/internal/libyang/build.sh        # two-stage static build of PCRE2 + libyang
cd go && CGO_ENABLED=1 go vet  ./...
cd go && CGO_ENABLED=1 go test -race ./...
cd go && golangci-lint run
cd go && CGO_ENABLED=1 go run ./cmd/cambium all
```

`go/internal/libyang/build.sh` statically links a vendored libyang + PCRE2 pinned
by SHA and CMake flags in `/VERSIONS`; `scripts/diff-engine-config.sh` (also run
by `green-bar.sh`) asserts the build honors that pin.

### The `cmd/cambium` runner

`cmd/cambium` is the Go conformance runner for the Backend/data tier (it is
`//go:build cgo`). It locates `conformance/manifest.toml`, parses each enabled
fixture through the libyang backend, serializes to every expected format, and
exits non-zero on any byte mismatch. Schema-IR cases are skipped here — they run
in the pure gate.

```bash
cd go
go run ./cmd/cambium                       # curated enabled set
go run ./cmd/cambium all                    # every Backend/data case in the manifest
go run ./cmd/cambium scrambled-children ordered-user   # named cases only
```

With no arguments the runner runs a curated enabled set; `all` runs every
Backend/data case; otherwise it runs exactly the named cases. When the
`CAMBIUM_YANGLINT` environment variable points at a `yanglint` binary, cases
marked `oracle = true` are additionally checked against an independent `yanglint`
invocation, so the goldens are validated against a second implementation rather
than trusted on their own.

## Adding a case

1. Add `fixtures/<name>/` with the YANG module(s) and either an `input.<fmt>`
   (Backend/data) or an `expected-ir.json` (Schema-IR).
2. For a Backend/data case, add `golden/<name>/output.{xml,json,json_ietf}`.
3. Add the `[[case]]` entry to `manifest.toml` with `invariants = [...]`, the
   correct `tier`, and the input/expected pointers.
4. Run the relevant gate. New ordering behavior follows TDD: the fixture (the red
   test) lands before the production change.

## See also

- [Documentation index](./README.md)
- [Why Cambium](./why-cambium.md)
- [Architecture](./architecture.md)
- [Codegen guide](./guides/codegen.md)
- [libyang backend guide](./guides/libyang-backend.md)
- [Ordering invariants (normative)](../spec/ordering-invariants.md)
- [Diagnostic rule codes](../spec/rule-codes.md)
