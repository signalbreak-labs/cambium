//! Phase 3 Slice 4: diff + DataDiff/DiffEdit.
#![allow(clippy::expect_used, clippy::unwrap_used)]

use std::fs;
use std::path::PathBuf;

use cambium_core::{
    ContextBuilder, ContextFlags, DataTree, DiffOp, DiffOpts, Format, NewPathOpts, ParseMode,
    Result,
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
    let dir = PathBuf::from(env!("CARGO_MANIFEST_DIR")).join("../../target/tests/diff/modules");
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

fn build_base(ctx: &cambium_core::Context) -> Result<DataTree<'_>> {
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

fn build_after(ctx: &cambium_core::Context) -> Result<DataTree<'_>> {
    let mut tree = ctx.new_data();
    tree.new_path("/cambium-data-crud-demo:top", None, NewPathOpts::default())?;
    // Replace counter.
    tree.new_path(
        "/cambium-data-crud-demo:top/counter",
        Some("2"),
        NewPathOpts::default(),
    )?;
    // Create item b.
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
    // Drop nested and the red tag.
    Ok(tree)
}

#[test]
fn diff_empty_when_equal() -> Result<()> {
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

    let diff = original.diff(&copy, DiffOpts::default())?;
    assert!(diff.is_empty());
    assert_eq!(diff.edits().count(), 0);
    Ok(())
}

#[test]
fn diff_create_delete_replace() -> Result<()> {
    let ctx = load_crud_context();
    let base = build_base(&ctx)?;
    let after = build_after(&ctx)?;

    let diff = base.diff(&after, DiffOpts::default())?;
    assert!(!diff.is_empty());

    let edits: Vec<_> = diff.edits().collect();

    let find = |path: &str| edits.iter().find(|e| e.path() == path);

    let counter = find("/cambium-data-crud-demo:top/counter").expect("counter edit");
    assert_eq!(counter.op(), DiffOp::Replace);
    assert_eq!(counter.value(), Some("2"));

    let item_b = find("/cambium-data-crud-demo:top/item[id='b']").expect("item b create");
    assert_eq!(item_b.op(), DiffOp::Create);

    let tag_red = find("/cambium-data-crud-demo:top/tags[.='red']").expect("tag red delete");
    assert_eq!(tag_red.op(), DiffOp::Delete);

    let nested = find("/cambium-data-crud-demo:top/nested").expect("nested delete");
    assert_eq!(nested.op(), DiffOp::Delete);

    // `None` edits must not be surfaced by the iterator.
    assert!(!edits.iter().any(|e| e.op() == DiffOp::None));
    Ok(())
}

#[test]
fn diff_ordered_by_user_atomic() -> Result<()> {
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
    let base = ctx.parse(Format::Xml, ParseMode::data_only(), &input)?;

    // The ONLY change is moving entry 'a' before 'c'.
    let mut after = base.duplicate()?;
    {
        let mut list = after.user_ordered_list_at("/ordered-user-demo:config/entry[name='c']")?;
        // indices: c=0, a=1, b=2. Move a (1) before c (0) -> a, c, b.
        list.move_before(1, 0)?;
    }

    let diff = base.diff(&after, DiffOpts::default())?;
    let edits: Vec<_> = diff.edits().collect();

    assert_eq!(
        edits.len(),
        1,
        "user-ordered positional change must be exactly one atomic edit, got {edits:?}"
    );
    assert!(edits[0].is_ordered_by_user());

    // No scalar leaf replace should appear.
    assert!(
        !edits
            .iter()
            .any(|e| { e.path().contains("/value") && e.op() == DiffOp::Replace })
    );

    // The diff tree must serialize (yang-patch shaped).
    let _ = diff.serialize(Format::Xml)?;
    let _ = diff.serialize(Format::Json)?;
    Ok(())
}

#[test]
fn datadiff_freed_once() -> Result<()> {
    let ctx = load_crud_context();
    let base = build_base(&ctx)?;
    let after = build_after(&ctx)?;

    for _ in 0..100 {
        let _ = base.diff(&after, DiffOpts::default())?;
    }
    Ok(())
}
