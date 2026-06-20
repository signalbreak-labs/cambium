//! I4 — RPC/action input/output children are emitted in schema order.

use std::fs;
use std::path::PathBuf;

use cambium_core::{Context, Format, OpType, Result, SerializeFlags};

fn fixture_dir() -> Result<PathBuf> {
    let dir = PathBuf::from(env!("CARGO_MANIFEST_DIR"))
        .ancestors()
        .nth(2)
        .ok_or_else(|| cambium_core::Error::from("failed to locate fixture dir".to_string()))?
        .join("conformance/fixtures/rpc-order");
    Ok(dir)
}

#[test]
fn rpc_input_children_emitted_in_schema_order() -> Result<()> {
    let mut ctx = Context::new()?;
    ctx.set_search_path(fixture_dir()?.join("module"))?;
    ctx.load_module("rpc-order-demo")?;

    let input = fs::read_to_string(fixture_dir()?.join("input.xml"))?;
    let tree = ctx.parse_op(Format::Xml, OpType::Rpc, input.as_bytes())?;

    let output = tree.serialize(
        Format::Xml,
        SerializeFlags {
            siblings: true,
            ..Default::default()
        },
    )?;
    let output = String::from_utf8(output)?;

    let golden = fs::read_to_string(
        fixture_dir()?
            .ancestors()
            .nth(2)
            .ok_or_else(|| cambium_core::Error::from("failed to locate golden dir".to_string()))?
            .join("golden/rpc-order/output.xml"),
    )?;

    assert_eq!(
        output.trim(),
        golden.trim(),
        "RPC input children must serialize in schema declaration order"
    );

    Ok(())
}
