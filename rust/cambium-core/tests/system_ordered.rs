//! I2 canonical — `ordered-by system` list entries are emitted in canonical key order.

use std::fs;
use std::path::PathBuf;

use cambium_core::{Context, Format, ParseMode, Result, SerializeFlags, ValidateMode};

fn fixture_dir() -> Result<PathBuf> {
    let dir = PathBuf::from(env!("CARGO_MANIFEST_DIR"))
        .ancestors()
        .nth(2)
        .ok_or_else(|| cambium_core::Error::from("failed to locate fixture dir".to_string()))?
        .join("conformance/fixtures/system-list-canonical");
    Ok(dir)
}

#[test]
fn system_ordered_list_is_canonicalized_by_key() -> Result<()> {
    let mut ctx = Context::new()?;
    ctx.set_search_path(fixture_dir()?.join("module"))?;
    ctx.load_module("system-list-demo")?;

    let input = fs::read_to_string(fixture_dir()?.join("input.xml"))?;
    let mut tree = ctx.parse(Format::Xml, ParseMode::data_only(), input.as_bytes())?;
    tree.validate(ValidateMode::default())?;

    let xml = tree.serialize(
        Format::Xml,
        SerializeFlags {
            siblings: true,
            ..Default::default()
        },
    )?;
    let xml = String::from_utf8(xml)?;

    let golden_xml = fs::read_to_string(
        fixture_dir()?
            .ancestors()
            .nth(2)
            .ok_or_else(|| cambium_core::Error::from("failed to locate golden dir".to_string()))?
            .join("golden/system-list-canonical/output.xml"),
    )?;

    assert_eq!(
        xml.trim(),
        golden_xml.trim(),
        "system-ordered list must be canonicalized by key (a, b, c)"
    );

    let json = tree.serialize(
        Format::Json,
        SerializeFlags {
            siblings: true,
            ..Default::default()
        },
    )?;
    let json = String::from_utf8(json)?;

    let golden_json = fs::read_to_string(
        fixture_dir()?
            .ancestors()
            .nth(2)
            .ok_or_else(|| cambium_core::Error::from("failed to locate golden dir".to_string()))?
            .join("golden/system-list-canonical/output.json"),
    )?;

    assert_eq!(
        json.trim(),
        golden_json.trim(),
        "system-ordered list JSON must be canonicalized by key (a, b, c)"
    );

    Ok(())
}
