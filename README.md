# Cambium

Modern, order-correct YANG for Rust and Go.

Cambium is a YANG toolkit that treats order as a structural property of the
tree. `ordered-by user` lists round-trip byte-exact, system-ordered output is
deterministic-canonical, and container children are emitted in schema
declaration order — the order NETCONF servers expect.

Rust's data/validation stack and the optional Go backend are libyang-backed.
The default Go package is pure Go for schema loading, introspection, and
codegen input; it must build with `CGO_ENABLED=0`.

## Why Cambium

| Capability | Cambium | goyang/ygot |
|---|---|---|
| Container child order | schema declaration order | alphabetical via `Entry.Dir` |
| `ordered-by user` round-trip | byte-exact, default | opt-in, late retrofit |
| `must`/`when`/`mandatory` | delegated to libyang | not enforced |
| YANG 1.1 | full | gaps |
| Rust + Go byte parity | CI-gated | Go only |

For the full background, read [docs/cambium-kickoff.md](docs/cambium-kickoff.md).
For a worked example of the ordering guarantee, read
[docs/ordering-story.md](docs/ordering-story.md).

## Build

```bash
git submodule update --init --recursive
cargo build --workspace
cd go && CGO_ENABLED=0 go test ./cambium ./codegen ./compat
```

The Rust backend statically links vendored libyang and PCRE2. No system libyang
is required on Linux x86_64/arm64 or macOS arm64. Go users only need cgo when
they explicitly use the optional `go/libyangbackend` package.

## Test

```bash
cargo test --workspace
cargo clippy --workspace --all-targets -- -D warnings
cargo run -p conformance-runner
scripts/check-go-default-pure.sh
cd go && CGO_ENABLED=0 go test ./cambium ./codegen ./compat
cd go && CGO_ENABLED=0 go vet ./cambium ./codegen ./compat
```

The conformance runner reads `/conformance/manifest.toml` and verifies every
fixture against its golden output. Tier-0 IETF core cases are included.

For the optional Go libyang backend, build the static engine first:

```bash
bash go/internal/libyang/build.sh
cd go && CGO_ENABLED=1 go test ./...
cd go && CGO_ENABLED=1 go vet ./...
```

## Quickstart

```rust
use cambium::{Context, Format, ParseMode, Result, SerializeFlags};

fn example() -> Result<()> {
    let mut ctx = Context::new()?;
    ctx.set_search_path("yang")?;
    ctx.load_module("order-demo")?;

    let xml = br#"<top xmlns="urn:order-demo"><a>a1</a><z>z1</z></top>"#;
    let tree = ctx.parse(Format::Xml, ParseMode::DataOnly, xml)?;

    let out = tree.serialize(Format::Xml, SerializeFlags { siblings: true })?;
    println!("{}", String::from_utf8_lossy(&out));
    Ok(())
}
```

See [docs/quickstart.md](docs/quickstart.md) for more.

## Project status

This is early work toward v0.1. The Rust core, ordering invariants, shared
conformance harness, pure-Go schema/codegen path, and optional Go libyang
backend are in place. gNMI support remains future work.

## License

BSD-3-Clause. See [LICENSE](LICENSE) and [NOTICE](NOTICE) for vendored
third-party components.
