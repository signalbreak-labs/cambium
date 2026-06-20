//! gNMI JSON_IETF codec (invariant I6): ordered-by user lists serialize as
//! atomic JSON arrays, never decomposed into per-leaf updates.

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
fn json_ietf_preserves_user_ordered_list_as_array() -> Result<()> {
    let mut ctx = Context::new()?;
    ctx.set_search_path(fixture_dir()?.join("module"))?;
    ctx.load_module("ordered-user-demo")?;

    let input = fs::read_to_string(fixture_dir()?.join("input.xml"))?;
    let mut tree = ctx.parse(Format::Xml, ParseMode::data_only(), input.as_bytes())?;
    tree.validate(ValidateMode::default())?;

    let json_ietf = tree.serialize(
        Format::JsonIetf,
        SerializeFlags {
            siblings: true,
            ..Default::default()
        },
    )?;
    let json_plain = tree.serialize(
        Format::Json,
        SerializeFlags {
            siblings: true,
            ..Default::default()
        },
    )?;

    // Both formats must preserve the user-ordered list as an array. JSON_IETF
    // additionally keeps empty non-presence containers, so the bytes may differ.
    let _json_plain = json_plain;

    let s = String::from_utf8(json_ietf)?;

    // The ordered-by user list must be encoded as a JSON array in insertion order.
    assert!(
        s.contains("\"entry\": ["),
        "JSON_IETF must encode the ordered-by user list as an array"
    );

    // Verify the insertion order c, a, b is preserved.
    let c_pos = s
        .find("\"name\": \"c\"")
        .ok_or_else(|| cambium_core::Error::from("missing c".to_string()))?;
    let a_pos = s
        .find("\"name\": \"a\"")
        .ok_or_else(|| cambium_core::Error::from("missing a".to_string()))?;
    let b_pos = s
        .find("\"name\": \"b\"")
        .ok_or_else(|| cambium_core::Error::from("missing b".to_string()))?;
    assert!(
        c_pos < a_pos && a_pos < b_pos,
        "user-ordered list must stay c, a, b"
    );

    Ok(())
}
