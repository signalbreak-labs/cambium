//! Phase 3 Slice 5: diff_apply + merge (with conflict pre-scan).
#![allow(clippy::expect_used, clippy::unwrap_used)]

use std::fs;
use std::path::PathBuf;

use cambium_core::{
    ContextBuilder, ContextFlags, DiffOpts, Format, MergeOpts, NewPathOpts, ParseMode, Result,
    RuleCode, SerializeFlags,
};

fn project_root() -> PathBuf {
    PathBuf::from(env!("CARGO_MANIFEST_DIR"))
        .join("../..")
        .canonicalize()
        .unwrap()
}

fn load_crud_context() -> cambium_core::Context {
    let yang = r#"module cambium-data-crud-demo {
    namespace "urn:cambium:data-crud";
    prefix cdc;
    yang-version 1.1;
    revision 2026-06-14;

    container top {
        leaf enabled {
            type boolean;
            default "true";
        }
        leaf counter {
            type uint64;
        }
        container nested {
            leaf name {
                type string;
            }
        }
        list item {
            key "id";
            leaf id {
                type string;
            }
            leaf value {
                type uint64;
            }
        }
        leaf-list tags {
            type string;
        }
    }
}
"#;
    let dir = PathBuf::from(env!("CARGO_MANIFEST_DIR"))
        .join("../../target/tests/diff-apply-merge/modules");
    fs::create_dir_all(&dir).unwrap();
    let path = dir.join("cambium-data-crud-demo.yang");
    fs::write(&path, yang).unwrap();
    ContextBuilder::new(ContextFlags {
        no_yang_library: true,
        ..Default::default()
    })
    .unwrap()
    .search_path(&dir)
    .unwrap()
    .load_module("cambium-data-crud-demo", None, &[])
    .unwrap()
    .build()
    .unwrap()
}

fn build_base(ctx: &cambium_core::Context) -> Result<cambium_core::DataTree<'_>> {
    let mut tree = ctx.new_data();
    tree.new_path("/cambium-data-crud-demo:top", None, NewPathOpts::default())?;
    tree.new_path(
        "/cambium-data-crud-demo:top/counter",
        Some("1"),
        NewPathOpts::default(),
    )?;
    tree.new_path(
        "/cambium-data-crud-demo:top/nested",
        None,
        NewPathOpts::default(),
    )?;
    tree.new_path(
        "/cambium-data-crud-demo:top/nested/name",
        Some("x"),
        NewPathOpts::default(),
    )?;
    tree.new_path(
        "/cambium-data-crud-demo:top/item[id='a']",
        None,
        NewPathOpts::default(),
    )?;
    tree.new_path(
        "/cambium-data-crud-demo:top/item[id='a']/value",
        Some("10"),
        NewPathOpts::default(),
    )?;
    tree.new_path(
        "/cambium-data-crud-demo:top/tags",
        Some("red"),
        NewPathOpts::default(),
    )?;
    Ok(tree)
}

fn build_after(ctx: &cambium_core::Context) -> Result<cambium_core::DataTree<'_>> {
    let mut tree = ctx.new_data();
    tree.new_path("/cambium-data-crud-demo:top", None, NewPathOpts::default())?;
    tree.new_path(
        "/cambium-data-crud-demo:top/counter",
        Some("2"),
        NewPathOpts::default(),
    )?;
    tree.new_path(
        "/cambium-data-crud-demo:top/item[id='b']",
        None,
        NewPathOpts::default(),
    )?;
    tree.new_path(
        "/cambium-data-crud-demo:top/item[id='b']/value",
        Some("20"),
        NewPathOpts::default(),
    )?;
    Ok(tree)
}

#[test]
fn diff_apply_round_trip() -> Result<()> {
    let ctx = load_crud_context();
    let mut base = build_base(&ctx)?;
    let after = build_after(&ctx)?;

    let diff = base.diff(&after, DiffOpts::default())?;
    base.diff_apply(&diff)?;

    let expected_xml = after.serialize(Format::Xml, SerializeFlags::default())?;
    let expected_json = after.serialize(Format::Json, SerializeFlags::default())?;
    assert_eq!(
        base.serialize(Format::Xml, SerializeFlags::default())?,
        expected_xml
    );
    assert_eq!(
        base.serialize(Format::Json, SerializeFlags::default())?,
        expected_json
    );
    Ok(())
}

#[test]
fn merge_preserves_user_order() -> Result<()> {
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

    let input =
        fs::read(project_root().join("conformance/fixtures/ordered-user/input.xml")).unwrap();
    let source = ctx.parse(Format::Xml, ParseMode::data_only(), &input)?;

    let mut target = ctx.new_data();
    target.new_path("/ordered-user-demo:config", None, NewPathOpts::default())?;
    target.merge(&source, MergeOpts::default())?;

    let expected =
        fs::read(project_root().join("conformance/golden/ordered-user/output.json")).unwrap();
    assert_eq!(
        target.serialize(Format::Json, SerializeFlags::default())?,
        expected
    );
    Ok(())
}

#[test]
fn merge_conflict_errors() -> Result<()> {
    let ctx = load_crud_context();

    let mut target = ctx.new_data();
    target.new_path("/cambium-data-crud-demo:top", None, NewPathOpts::default())?;
    target.new_path(
        "/cambium-data-crud-demo:top/counter",
        Some("1"),
        NewPathOpts::default(),
    )?;

    let mut source = ctx.new_data();
    source.new_path("/cambium-data-crud-demo:top", None, NewPathOpts::default())?;
    source.new_path(
        "/cambium-data-crud-demo:top/counter",
        Some("2"),
        NewPathOpts::default(),
    )?;

    let err = target.merge(&source, MergeOpts::default()).unwrap_err();
    assert_eq!(err.rule_code(), RuleCode::Validate);

    // The pre-scan must have run before any mutation: target is untouched.
    assert_eq!(
        target
            .get("/cambium-data-crud-demo:top/counter")?
            .value()?
            .unwrap(),
        cambium_core::Value::Uint64(1)
    );
    Ok(())
}
