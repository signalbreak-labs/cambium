//! Phase 3 Slice 3: deep copy via `DataTree::duplicate`.
#![allow(clippy::unwrap_used)]

use std::fs;
use std::path::PathBuf;

use cambium_core::{
    ContextBuilder, ContextFlags, Format, NewPathOpts, ParseMode, Result, SerializeFlags, Value,
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
    let dir =
        PathBuf::from(env!("CARGO_MANIFEST_DIR")).join("../../target/tests/duplicate/modules");
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

#[test]
fn duplicate_deep_copy_independent() -> Result<()> {
    let ctx = load_crud_context();
    let mut original = ctx.new_data();
    original.new_path("/cambium-data-crud-demo:top", None, NewPathOpts::default())?;
    original.new_path(
        "/cambium-data-crud-demo:top/counter",
        Some("1"),
        NewPathOpts::default(),
    )?;
    original.new_path(
        "/cambium-data-crud-demo:top/nested",
        None,
        NewPathOpts::default(),
    )?;
    original.new_path(
        "/cambium-data-crud-demo:top/nested/name",
        Some("inner"),
        NewPathOpts::default(),
    )?;

    let mut copy = original.duplicate()?;

    // Mutate copy; original must stay unchanged.
    copy.set_value("/cambium-data-crud-demo:top/counter", "99")?;
    copy.remove_path("/cambium-data-crud-demo:top/nested")?;
    copy.new_path(
        "/cambium-data-crud-demo:top/item[id='x']",
        None,
        NewPathOpts::default(),
    )?;

    assert_eq!(
        original
            .get("/cambium-data-crud-demo:top/counter")?
            .value()?
            .unwrap(),
        Value::Uint64(1)
    );
    assert!(original.exists("/cambium-data-crud-demo:top/nested"));
    assert!(!original.exists("/cambium-data-crud-demo:top/item[id='x']"));

    // Mutate original; copy must stay unchanged.
    original.set_value("/cambium-data-crud-demo:top/counter", "42")?;
    assert_eq!(
        copy.get("/cambium-data-crud-demo:top/counter")?
            .value()?
            .unwrap(),
        Value::Uint64(99)
    );
    assert!(!copy.exists("/cambium-data-crud-demo:top/nested"));
    assert!(copy.exists("/cambium-data-crud-demo:top/item[id='x']"));
    Ok(())
}

#[test]
fn duplicate_preserves_user_order() -> Result<()> {
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
    let original = ctx.parse(Format::Xml, ParseMode::data_only(), &input)?;
    let copy = original.duplicate()?;

    assert_eq!(
        original.serialize(Format::Json, SerializeFlags::default())?,
        copy.serialize(Format::Json, SerializeFlags::default())?
    );
    Ok(())
}

#[test]
fn duplicate_freed_once() -> Result<()> {
    let ctx = load_crud_context();
    let mut tree = ctx.new_data();
    tree.new_path("/cambium-data-crud-demo:top", None, NewPathOpts::default())?;
    tree.new_path(
        "/cambium-data-crud-demo:top/counter",
        Some("1"),
        NewPathOpts::default(),
    )?;

    for _ in 0..100 {
        let _ = tree.duplicate()?;
    }
    // If Drop double-freed or leaked, we would have crashed or hung by now.
    assert!(tree.exists("/cambium-data-crud-demo:top/counter"));
    Ok(())
}
