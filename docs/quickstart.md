# Quickstart

Cambium is an order-correct YANG toolkit for Go.

There are two tiers:

- **Schema + codegen (default, pure Go, `CGO_ENABLED=0`)** — load YANG, walk the
  ordered schema, and generate typed Go structs that serialize, parse, and
  validate themselves in schema-declaration order.
- **Data backend (optional, requires cgo)** — `go/libyangbackend` provides a
  generic `DataTree` for parsing arbitrary documents and full RFC-7950
  validation (`must`/`when`/leafref) over libyang.

This guide covers the pure-Go path first, then the optional backend.

## Before you start

```bash
go get github.com/signalbreak-labs/cambium/go
```

The default packages need no cgo and no system libraries. For the optional
backend, build the vendored static engine once:

```bash
git submodule update --init --recursive
bash go/internal/libyang/build.sh
```

## Load a schema and generate typed structs (pure Go)

```go
import (
	"github.com/signalbreak-labs/cambium/go/cambium"
	"github.com/signalbreak-labs/cambium/go/codegen"
)

b, _ := cambium.NewContextBuilder(cambium.ContextFlags{})
_ = b.SearchPath("yang")
_ = b.LoadModule("order-demo", nil, nil) // (name, revision, features)
ctx, _ := b.Build()                      // frozen schema context

src, _ := codegen.GenerateGo(ctx, "order-demo")
// `src` is a self-contained Go file of typed structs for the module.
```

The generated structs are **order-correct by construction**: list keys come
first, then children in schema declaration order; `ordered-by user` lists are a
positional-only type (reordering a system-ordered field is a compile error).

## Use the generated structs

Each generated struct carries native, cgo-free methods:

```go
// Serialize (one ordered walk of the field-order manifest — never map order):
xml := root.ToXML()
js := root.ToJSONIETF()
trimmed := root.ToJSONIETFWithDefaults(WithDefaultsTrim)

// Parse JSON_IETF back into the typed structs:
root, err := FromJSONIETF(bytes)

// Validate cardinality / range / length / pattern / unique / mandatory / choice:
err = root.Validate()
```

`Validate()` covers structural and type constraints. Full RFC-7950 semantic
validation of `must`/`when` XPath and leafref *instance* existence requires the
optional backend (it needs a live data tree + XPath engine).

## Optional: generic data tree (libyang backend, cgo)

```go
import backend "github.com/signalbreak-labs/cambium/go/libyangbackend"

ctx, _ := backend.NewContextBuilder().SearchPath("yang").LoadModule("order-demo", nil).Freeze()
t, _ := backend.Parse(ctx, backend.FormatXML, backend.ParseModeDataOnly, xmlBytes)
_ = t.Validate(backend.ValidateModeConfigOnly | backend.ValidateModeMultiError)
out, _ := t.Serialize(backend.FormatXML, backend.SerializeWithSiblings)
```

Serialization is one ordered walk of libyang's sibling chain — never a native
map or struct serializer. Build with `CGO_ENABLED=1`.

## Next steps

- Read [the ordering story](ordering-story.md) for why order is structural.
- Walk the ordered schema in depth: [schema introspection](guides/schema-introspection.md).
- Generate typed structs: [codegen guide](guides/codegen.md).
- Migrating from goyang: [goyang migration](guides/goyang-migration.md).
- Use the optional data tier: [libyang backend](guides/libyang-backend.md).
- Run the conformance suite: `cd go && go run ./cmd/cambium all`.
- Explore the shared corpus in `/conformance` and the contract in `/spec`.
