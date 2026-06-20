//! I3 — keys within a list entry are emitted first, in key-statement order.

use std::fs;
use std::path::PathBuf;

use cambium_core::{Context, Format, ParseMode, Result, SerializeFlags, ValidateMode};

fn fixture_dir() -> Result<PathBuf> {
    let dir = PathBuf::from(env!("CARGO_MANIFEST_DIR"))
        .ancestors()
        .nth(2)
        .ok_or_else(|| cambium_core::Error::from("failed to locate fixture dir".to_string()))?
        .join("conformance/fixtures/keys-first");
    Ok(dir)
}

#[test]
fn keys_are_emitted_first_in_key_statement_order() -> Result<()> {
    let mut ctx = Context::new()?;
    ctx.set_search_path(fixture_dir()?.join("module"))?;
    ctx.load_module("keys-first-demo")?;

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
            .join("golden/keys-first/output.xml"),
    )?;

    assert_eq!(
        output.trim(),
        golden.trim(),
        "keys must appear first, in key-statement order"
    );

    Ok(())
}
