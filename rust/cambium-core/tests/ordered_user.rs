//! I1 — `ordered-by user` list entry order is preserved exactly.

use std::fs;
use std::path::PathBuf;

use cambium_core::{Context, Format, ParseMode, Result, SerializeFlags, ValidateMode};

fn fixture_dir() -> Result<PathBuf> {
    let dir = PathBuf::from(env!("CARGO_MANIFEST_DIR"))
        .ancestors()
        .nth(2)
        .ok_or_else(|| cambium_core::Error::from("failed to locate fixture dir".to_string()))?
        .join("conformance/fixtures/ordered-user");
    Ok(dir)
}

#[test]
fn ordered_by_user_list_preserves_insertion_order() -> Result<()> {
    let mut ctx = Context::new()?;
    ctx.set_search_path(fixture_dir()?.join("module"))?;
    ctx.load_module("ordered-user-demo")?;

    let input = fs::read_to_string(fixture_dir()?.join("input.xml"))?;
    let mut tree = ctx.parse(Format::Xml, ParseMode::data_only(), input.as_bytes())?;
    tree.validate(ValidateMode::default())?;

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
            .join("golden/ordered-user/output.xml"),
    )?;

    assert_eq!(
        output.trim(),
        golden.trim(),
        "ordered-by user list must preserve insertion order (c, a, b)"
    );

    Ok(())
}
