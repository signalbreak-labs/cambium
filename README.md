# Cambium

Modern, order-correct YANG for Go.

Cambium is a YANG toolkit that treats order as a structural property of the
tree. `ordered-by user` lists round-trip byte-exact, system-ordered output is
deterministic-canonical, and container children are emitted in schema
declaration order — the order NETCONF servers expect. It is the successor to
openconfig/goyang (the parser/AST library), **not** ygot.

The **default Go package is pure Go** (`CGO_ENABLED=0`): schema loading,
introspection, and typed-struct **codegen** with native, order-correct
XML/JSON_IETF (de)serializers. Generic data-tree parse/validate/serialize is the
**optional** libyang-backed backend (`go/libyangbackend`, requires cgo).

> Cambium was originally Rust-primary + Go. The Rust stack was removed
> (2026-06-20) to focus on Go. The shared contract (`/spec`, `/conformance`,
> `/VERSIONS`) stays language-neutral so a Rust (or other) binding can return as
> a first-class peer — see `AGENTS.md` → "Adding a language binding".

## Why Cambium

Cambium targets order-sensitive, NETCONF-facing workflows and faithful
serialization, where the schema tree must remember declaration order. What it
provides:

| Capability | What Cambium provides |
|---|---|
| Container child order | effective schema declaration order — the order NETCONF servers expect |
| `ordered-by user` round-trip | byte-exact insertion order preserved by default |
| YANG 1.1 | full RFC-7950 support |
| `must`/`when`/`mandatory` (data) | enforced via the optional libyang backend |
| Typed-struct codegen | native, order-correct, cgo-free |

For the design rationale and how this fits the use case, read
[docs/why-cambium.md](docs/why-cambium.md) and the docs index at
[docs/README.md](docs/README.md). For background, see
[docs/cambium-kickoff.md](docs/cambium-kickoff.md) (design brief) and
[docs/ordering-story.md](docs/ordering-story.md).

## Capability tiers

| Tier | Packages | cgo? | What you get |
|---|---|---|---|
| Schema + codegen (default) | `go/cambium`, `go/codegen`, `go/compat` | no | parse YANG → ordered schema IR; generate typed Go structs that serialize/parse/validate themselves |
| Data backend (optional) | `go/libyangbackend` | yes | generic `DataTree`: parse/validate/serialize/diff/merge/LYB over libyang |

## Install

```bash
go get github.com/signalbreak-labs/cambium/go
```

The default packages need no cgo and no system libraries. The optional backend
statically links a vendored libyang + PCRE2; build the engine first with
`bash go/internal/libyang/build.sh`.

## Quickstart (pure Go: schema + codegen)

```go
package main

import (
	"fmt"

	"github.com/signalbreak-labs/cambium/go/cambium"
	"github.com/signalbreak-labs/cambium/go/codegen"
)

func main() {
	b, err := cambium.NewContextBuilder(cambium.ContextFlags{})
	if err != nil {
		panic(err)
	}
	if err := b.SearchPath("yang"); err != nil {
		panic(err)
	}
	if err := b.LoadModule("order-demo", nil, nil); err != nil {
		panic(err)
	}
	ctx, err := b.Build()
	if err != nil {
		panic(err)
	}

	// Order-correct typed Go structs (keys-first, schema declaration order),
	// each with ToXML/ToJSONIETF/FromJSONIETF/Validate — all cgo-free.
	src, err := codegen.GenerateGo(ctx, "order-demo")
	if err != nil {
		panic(err)
	}
	fmt.Println(src)
}
```

For generic data trees (parse an arbitrary document, RFC-7950 `must`/`when`
validation), import the optional `go/libyangbackend` (cgo). See
[docs/quickstart.md](docs/quickstart.md).

## Test

```bash
# Default cgo-free surface.
scripts/check-go-default-pure.sh
cd go && CGO_ENABLED=0 go test ./cambium ./codegen ./compat
cd go && CGO_ENABLED=0 go vet ./cambium ./codegen ./compat

# Optional libyang backend (cgo) + conformance.
bash go/internal/libyang/build.sh
cd go && CGO_ENABLED=1 go test ./...
cd go && go run ./cmd/cambium all   # conformance corpus
```

`scripts/green-bar.sh` runs the full local gate. The conformance runner reads
`/conformance/manifest.toml` and verifies every fixture against its golden
output.

## Project status

Go schema/codegen and the optional Go libyang backend are in place; the shared
conformance corpus passes. Known gaps are tracked in
[docs/gaps-analysis-2026-06-20.md](docs/gaps-analysis-2026-06-20.md). gNMI
support remains future work.

## License

BSD-3-Clause. See [LICENSE](LICENSE) and [NOTICE](NOTICE) for vendored
third-party components.
