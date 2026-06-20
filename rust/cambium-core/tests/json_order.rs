//! I5 — JSON list/leaf-list arrays carry order; JSON object member order is
//! the single libyang printer order (deterministic, schema-declaration order).
//!
#![allow(clippy::unwrap_used)]

use std::fs;
use std::path::PathBuf;

use cambium_core::{Context, Format, OpType, ParseMode, Result, SerializeFlags, ValidateMode};

fn fixture_dir(name: &str) -> Result<PathBuf> {
    let dir = PathBuf::from(env!("CARGO_MANIFEST_DIR"))
        .ancestors()
        .nth(2)
        .ok_or_else(|| cambium_core::Error::from("failed to locate fixture dir".to_string()))?
        .join(format!("conformance/fixtures/{name}"));
    Ok(dir)
}

fn golden_path(name: &str) -> Result<PathBuf> {
    let path = fixture_dir(name)?
        .ancestors()
        .nth(2)
        .ok_or_else(|| cambium_core::Error::from("failed to locate golden dir".to_string()))?
        .join(format!("golden/{name}/output.json"));
    Ok(path)
}

fn assert_json_roundtrip(name: &str, is_rpc: bool) -> Result<()> {
    let mut ctx = Context::new()?;
    ctx.set_search_path(fixture_dir(name)?.join("module"))?;

    let module_path = fixture_dir(name)?.join("module");
    let module_name = fs::read_dir(&module_path)?
        .filter_map(|e| e.ok())
        .filter(|e| {
            e.path()
                .extension()
                .map(|ext| ext == "yang")
                .unwrap_or(false)
        })
        .map(|e| e.path().file_stem().unwrap().to_string_lossy().to_string())
        .next()
        .ok_or_else(|| cambium_core::Error::from("no YANG module found".to_string()))?;
    ctx.load_module(&module_name)?;

    let input = fs::read_to_string(fixture_dir(name)?.join("input.xml"))?;
    let tree = if is_rpc {
        ctx.parse_op(Format::Xml, OpType::Rpc, input.as_bytes())?
    } else {
        let mut tree = ctx.parse(Format::Xml, ParseMode::data_only(), input.as_bytes())?;
        tree.validate(ValidateMode::default())?;
        tree
    };

    let output = tree.serialize(
        Format::Json,
        SerializeFlags {
            siblings: true,
            ..Default::default()
        },
    )?;
    let output = String::from_utf8(output)?;
    let golden = fs::read_to_string(golden_path(name)?)?;

    assert_eq!(
        output.trim(),
        golden.trim(),
        "JSON output for {name} must match golden"
    );

    Ok(())
}

#[test]
fn json_object_members_are_schema_ordered() -> Result<()> {
    assert_json_roundtrip("scrambled-children", false)
}

#[test]
fn json_list_keys_are_first() -> Result<()> {
    assert_json_roundtrip("keys-first", false)
}

#[test]
fn json_user_ordered_list_preserves_order() -> Result<()> {
    assert_json_roundtrip("ordered-user", false)
}

#[test]
fn json_rpc_input_is_schema_ordered() -> Result<()> {
    assert_json_roundtrip("rpc-order", true)
}
