# Quickstart

Cambium is an order-correct YANG toolkit for Rust and Go. It uses libyang as
its RFC 7950 engine, so parsing, validation, and serialization all preserve the
order that NETCONF servers expect.

This guide shows you how to parse a YANG data document, validate it, and print
it back out in XML or JSON.

## Before you start

You need a Rust toolchain and the vendored C engine checked out:

```bash
git submodule update --init --recursive
cargo build --workspace
```

No system libyang or PCRE2 is required. The build is fully static on Linux
x86_64/arm64 and macOS arm64.

## Parse a document

Create a context, tell it where to find YANG modules, and parse a data tree:

```rust
use cambium::{Context, Format, ParseMode, Result};

fn example() -> Result<()> {
    let mut ctx = Context::new()?;
    ctx.set_search_path("yang")?;
    ctx.load_module("order-demo")?;

    let xml = br#"<top xmlns="urn:order-demo"><a>a1</a><z>z1</z><m>m1</m></top>"#;
    let tree = ctx.parse(Format::Xml, ParseMode::DataOnly, xml)?;
    Ok(())
}
```

`ParseMode::DataOnly` mirrors libyang's `LYD_PARSE_ONLY`. Use
`ParseMode::Strict` to reject unknown nodes, or `ParseMode::Opaque` to keep
unknown nodes as opaque data.

## Validate

Validation is separate from parsing. Call `validate` when you are ready to check
`must`, `when`, `mandatory`, leafref require-instance, and other constraints:

```rust
use cambium::ValidateMode;

let mut tree = ctx.parse(Format::Xml, ParseMode::DataOnly, xml)?;
tree.validate(ValidateMode { no_state: false, multi_error: true })?;
```

## Serialize

Serialization is one ordered walk of the libyang sibling chain. It never comes
from a native Rust map or struct:

```rust
use cambium::SerializeFlags;

let bytes = tree.serialize(Format::Xml, SerializeFlags { siblings: true })?;
println!("{}", String::from_utf8_lossy(&bytes));
```

Set `siblings: true` when your tree root is one of several top-level siblings.
For a single root, use `SerializeFlags::default()`.

## Next steps

- Read [the ordering story](ordering-story.md) to see why order is a structural
  property in Cambium.
- Run the conformance suite with `cargo run -p conformance-runner`.
- Explore the shared corpus in `/conformance`.
