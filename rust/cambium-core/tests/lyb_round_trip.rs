//! Phase 3 Slice 2b: LYB binary format round-trip.
#![allow(clippy::unwrap_used)]

use std::fs;
use std::path::PathBuf;

use cambium_core::{ContextBuilder, ContextFlags, Format, ParseMode, Result, SerializeFlags};

fn project_root() -> PathBuf {
    PathBuf::from(env!("CARGO_MANIFEST_DIR"))
        .join("../..")
        .canonicalize()
        .unwrap()
}

#[test]
fn serialize_lyb_round_trip() -> Result<()> {
    let dir = project_root().join("conformance/fixtures/ordered-user/module");
    let ctx = ContextBuilder::new(ContextFlags {
        no_yang_library: true,
        ..Default::default()
    })
    .unwrap()
    .search_path(&dir)
    .unwrap()
    .load_module("ordered-user-demo", None, &[])
    .unwrap()
    .build()
    .unwrap();

    let input_json =
        fs::read(project_root().join("conformance/golden/ordered-user/output.json")).unwrap();
    let tree = ctx.parse(Format::Json, ParseMode::data_only(), &input_json)?;

    let lyb = tree.serialize(Format::Lyb, SerializeFlags::default())?;
    assert!(!lyb.is_empty(), "LYB output must not be empty");

    let round = ctx.parse(Format::Lyb, ParseMode::data_only(), &lyb)?;
    let round_json = round.serialize(Format::Json, SerializeFlags::default())?;
    let round_xml = round.serialize(Format::Xml, SerializeFlags::default())?;

    assert_eq!(
        String::from_utf8_lossy(&round_json).trim(),
        String::from_utf8_lossy(&input_json).trim(),
        "JSON round-trip via LYB must match"
    );

    let expected_xml =
        fs::read(project_root().join("conformance/golden/ordered-user/output.xml")).unwrap();
    assert_eq!(
        String::from_utf8_lossy(&round_xml).trim(),
        String::from_utf8_lossy(&expected_xml).trim(),
        "XML round-trip via LYB must match golden"
    );

    Ok(())
}
