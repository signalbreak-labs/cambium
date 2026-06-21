# Cambium

Modern, order-correct YANG for Go.

Cambium is a YANG toolkit and SDK that treats declaration order as a **structural
property of the schema tree** rather than a sort key, a sidecar, or a map.
Container children are exposed and serialized in effective schema declaration
order — the order NETCONF servers expect — `ordered-by user` lists round-trip
byte-exact, and `ordered-by system` output is deterministic and canonical. It is
the successor to [openconfig/goyang](https://github.com/openconfig/goyang) (the
YANG parser/AST library), **not** ygot.

## The one rule

Order is a structural property of the tree — never a sort key, sidecar, or map.
An ordered sibling sequence is the source of truth; any keyed index is a derived
lookup that is never consulted for traversal, codegen, or serialization. goyang
stores effective children in a Go map (`Entry.Dir`) and returns them
alphabetically; Cambium keeps the declaration order RFC 7950 §7.8.5 makes
semantically load-bearing. This single rule is what the whole toolkit is built to
protect — see [docs/concepts/ordering.md](docs/concepts/ordering.md).

## Capability tiers

Cambium is layered into three tiers by what they need to build and run. The
**default surface is pure Go** (`CGO_ENABLED=0`); only the full libyang data
engine requires cgo.

| Tier | Packages | cgo? | What you get |
|---|---|---|---|
| **Schema + codegen** (default) | `cambium`, `codegen`, `compat` | no | parse YANG → ordered schema IR; introspect it; generate typed Go structs that serialize/parse/validate themselves in declaration order |
| **Pure-Go data tree** (experimental) | `datatree` | no | generic data tree: parse/serialize/validate JSON_IETF + XML, leafrefs, `must`/`when`, defaults — without libyang. **API not yet stable** ([scope](docs/guides/data-tree-pure-go.md)) |
| **libyang data backend** (optional) | `libyangbackend` | yes | full RFC-7950 data engine: parse/validate/serialize/diff/merge/LYB over a vendored, statically linked libyang |

The first two tiers are in the cgo-free default import closure, verified
mechanically by `scripts/check-go-default-pure.sh`. The libyang backend stays
strictly outside it.

## Install

```bash
go get github.com/signalbreak-labs/cambium/go
```

The default packages need no cgo and no system libraries. The optional libyang
backend statically links a vendored libyang + PCRE2; build the engine first with
`bash go/internal/libyang/build.sh`. See [docs/guides/install.md](docs/guides/install.md).

## Quickstart

Load a module and walk a container's children. They come back in **declaration
order** — `z, m, a` — not the alphabetical `a, m, z` a map would give you.

```go
package main

import (
	"fmt"

	"github.com/signalbreak-labs/cambium/go/cambium"
)

const orderDemo = `module order-demo {
  namespace "urn:order-demo";
  prefix od;
  container top {
    leaf z { type string; }
    leaf m { type string; }
    leaf a { type string; }
  }
}`

func main() {
	b, err := cambium.NewContextBuilder(cambium.ContextFlags{})
	if err != nil {
		panic(err)
	}
	if err := b.LoadModuleStr(orderDemo); err != nil {
		panic(err)
	}
	ctx, err := b.Build()
	if err != nil {
		panic(err)
	}

	mod, err := ctx.Schema("order-demo")
	if err != nil {
		panic(err)
	}
	top, _ := mod.TopLevel().Lookup("top")
	for child := range top.Children().Iter() {
		fmt.Println(child.Name()) // z, m, a — declaration order, not alphabetical
	}
}
```

From the same ordered IR, `codegen.GenerateGo(ctx, "order-demo")` emits typed Go
structs with keys-first, declaration-order XML/JSON_IETF serializers. To parse and
validate generic instance data, use the pure-Go `datatree` tier (experimental) or
the libyang backend. See the [quickstart](docs/guides/quickstart.md).

## Documentation

Full docs live in [`docs/`](docs/README.md). Start with the
[overview](docs/overview.md).

**Using Cambium**
- [Install](docs/guides/install.md) · [Quickstart](docs/guides/quickstart.md)
- [Schema introspection](docs/guides/schema-introspection.md) — package `cambium`
- [Codegen](docs/guides/codegen.md) — package `codegen`
- [Pure-Go data tree](docs/guides/data-tree-pure-go.md) — package `datatree` (experimental)
- [libyang backend](docs/guides/data-tree-libyang.md) — package `libyangbackend`
- [Migrating from goyang](docs/guides/goyang-migration.md) — package `compat`

**Concepts**
- [Ordering](docs/concepts/ordering.md) · [Tiers & the cgo boundary](docs/concepts/tiers-and-cgo.md) · [Architecture](docs/concepts/architecture.md)

**Contributing & contract**
- [Development](docs/contributing/development.md) · [Conformance](docs/contributing/conformance.md) · [Roadmap](docs/contributing/roadmap.md)
- [`/spec`](spec/) — the normative, language-neutral contract: [API shape](spec/api.md), [ordering invariants I1–I6](spec/ordering-invariants.md), [rule codes](spec/rule-codes.md)

API reference is the package godoc, browsable on
[pkg.go.dev](https://pkg.go.dev/github.com/signalbreak-labs/cambium/go).

## Project status

The Schema-IR tier (schema + codegen) and the optional libyang backend are in
place and the shared conformance corpus passes. The pure-Go `datatree` tier is
under active development and **experimental** — its API and value representation
will change. Current work and known gaps are tracked in
[docs/contributing/roadmap.md](docs/contributing/roadmap.md). gNMI support remains
future work.

## License

BSD-3-Clause. See [LICENSE](LICENSE) and [NOTICE](NOTICE) for vendored third-party
components (libyang, PCRE2).
