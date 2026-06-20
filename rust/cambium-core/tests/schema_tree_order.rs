//! Schema-tree introspection: declaration order is preserved, not alphabetical.

use std::path::PathBuf;

use cambium_core::{Context, Result, SchemaNodeKind};

fn fixture_dir() -> Result<PathBuf> {
    let dir = PathBuf::from(env!("CARGO_MANIFEST_DIR"))
        .ancestors()
        .nth(2)
        .ok_or_else(|| cambium_core::Error::from("failed to locate fixture dir".to_string()))?
        .join("conformance/fixtures/scrambled-children");
    Ok(dir)
}

#[test]
fn schema_tree_walks_children_in_declaration_order() -> Result<()> {
    let mut ctx = Context::new()?;
    ctx.set_search_path(fixture_dir()?.join("module"))?;
    ctx.load_module("order-demo")?;

    let tree = ctx.schema_tree("order-demo")?;
    let top = tree
        .find(&["top"])
        .ok_or_else(|| cambium_core::Error::from("top container must exist".to_string()))?;
    assert_eq!(top.kind(), SchemaNodeKind::Container);

    let names: Vec<_> = top
        .children()
        .iter()
        .map(|n| n.name().to_string())
        .collect();
    assert_eq!(
        names,
        vec!["z", "m", "a"],
        "schema children must be in declaration order, not alphabetical"
    );

    Ok(())
}

#[test]
fn schema_tree_preorder_visits_every_node() -> Result<()> {
    let mut ctx = Context::new()?;
    ctx.set_search_path(fixture_dir()?.join("module"))?;
    ctx.load_module("order-demo")?;

    let tree = ctx.schema_tree("order-demo")?;
    let names: Vec<_> = tree.iter().map(|n| n.name().to_string()).collect();
    assert_eq!(names, vec!["top", "z", "m", "a"]);
    Ok(())
}
