//! Headline golden test at the safe core: scrambled input emits in schema order.

use std::fs;
use std::path::PathBuf;

use cambium_core::{Context, Format, ParseMode, Result, SerializeFlags, ValidateMode};

fn fixture_dir() -> Result<PathBuf> {
    let dir = PathBuf::from(env!("CARGO_MANIFEST_DIR"))
        .ancestors()
        .nth(2)
        .ok_or_else(|| cambium_core::Error::from("failed to locate fixture dir".to_string()))?
        .join("conformance/fixtures/scrambled-children");
    Ok(dir)
}

#[test]
fn scrambled_children_emitted_in_schema_order() -> Result<()> {
    let mut ctx = Context::new()?;
    ctx.set_search_path(fixture_dir()?.join("module"))?;
    ctx.load_module("order-demo")?;

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

    let golden_dir = fixture_dir()?
        .ancestors()
        .nth(2)
        .ok_or_else(|| cambium_core::Error::from("failed to locate golden dir".to_string()))?
        .join("golden/scrambled-children/output.xml");
    let golden = fs::read_to_string(golden_dir)?;

    assert_eq!(
        output.trim(),
        golden.trim(),
        "scrambled input must serialize in schema declaration order"
    );

    Ok(())
}
