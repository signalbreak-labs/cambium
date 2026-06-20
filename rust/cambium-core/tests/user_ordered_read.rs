//! Phase 2 Slice 4 acceptance tests: UserOrdered read side + UserOrderedLeafList.
#![allow(clippy::unwrap_used, clippy::expect_used)]

use std::fs;
use std::path::PathBuf;
use std::sync::atomic::{AtomicU64, Ordering};

use cambium_core::{
    ContextBuilder, Format, NewPathOpts, OrderedBy, Result, RuleCode, SerializeFlags,
};

fn temp_module_dir() -> PathBuf {
    let dir = PathBuf::from(env!("CARGO_MANIFEST_DIR"))
        .join("../../target/tests/user-ordered-read/modules");
    fs::create_dir_all(&dir).unwrap();
    dir
}

static WRITE_COUNTER: AtomicU64 = AtomicU64::new(0);

fn atomic_write(path: &std::path::Path, content: &str) {
    let dir = path.parent().unwrap();
    let n = WRITE_COUNTER.fetch_add(1, Ordering::Relaxed);
    let tmp = dir.join(format!(".tmp-{n}.yang"));
    fs::write(&tmp, content).unwrap();
    fs::rename(&tmp, path).unwrap();
}

fn write_module(name: &str, source: &str) -> PathBuf {
    let dir = temp_module_dir();
    let path = dir.join(format!("{name}.yang"));
    atomic_write(&path, source);
    dir
}

fn load_context() -> cambium_core::Context {
    let yang = r#"module cambium-user-ordered-demo {
    namespace "urn:cambium:user-ordered";
    prefix cuod;
    yang-version 1.1;
    revision 2026-06-14;

    container lists {
        list user-list {
            ordered-by user;
            key "id";
            leaf id {
                type string;
            }
            leaf value {
                type uint64;
            }
        }
        list system-list {
            ordered-by system;
            key "id";
            leaf id {
                type string;
            }
            leaf value {
                type uint64;
            }
        }
        leaf-list user-tags {
            ordered-by user;
            type string;
        }
        leaf-list system-tags {
            ordered-by system;
            type string;
        }
    }
}
"#;
    let dir = write_module("cambium-user-ordered-demo", yang);
    ContextBuilder::new(Default::default())
        .unwrap()
        .search_path(&dir)
        .unwrap()
        .load_module("cambium-user-ordered-demo", None, &[])
        .unwrap()
        .build()
        .unwrap()
}

fn build_tree(ctx: &cambium_core::Context) -> Result<cambium_core::DataTree<'_>> {
    let mut tree = ctx.new_data();
    tree.new_path(
        "/cambium-user-ordered-demo:lists",
        None,
        NewPathOpts::default(),
    )?;
    tree.new_path(
        "/cambium-user-ordered-demo:lists/user-list[id='a']",
        None,
        NewPathOpts::default(),
    )?;
    tree.new_path(
        "/cambium-user-ordered-demo:lists/user-list[id='a']/value",
        Some("1"),
        NewPathOpts::default(),
    )?;
    tree.new_path(
        "/cambium-user-ordered-demo:lists/user-list[id='b']",
        None,
        NewPathOpts::default(),
    )?;
    tree.new_path(
        "/cambium-user-ordered-demo:lists/user-list[id='b']/value",
        Some("2"),
        NewPathOpts::default(),
    )?;
    tree.new_path(
        "/cambium-user-ordered-demo:lists/user-list[id='c']",
        None,
        NewPathOpts::default(),
    )?;
    tree.new_path(
        "/cambium-user-ordered-demo:lists/user-list[id='c']/value",
        Some("3"),
        NewPathOpts::default(),
    )?;
    Ok(tree)
}

#[test]
fn user_ordered_read_side() -> Result<()> {
    let ctx = load_context();
    let mut tree = build_tree(&ctx)?;

    {
        let list =
            tree.user_ordered_list_at("/cambium-user-ordered-demo:lists/user-list[id='a']")?;
        assert_eq!(list.len(), 3);
        assert!(!list.is_empty());

        let ids: Vec<_> = list.iter().map(|n| n.name().to_string()).collect();
        assert_eq!(ids, vec!["user-list", "user-list", "user-list"]);

        let values: Vec<_> = list.iter().map(|n| n.children().unwrap().len()).collect();
        assert_eq!(values, vec![2, 2, 2]);

        let first = list.get(0).unwrap();
        assert!(first.path().contains("[id='a']"));

        let idx = list.find_by_key(&[("id", "b")]).unwrap();
        assert_eq!(idx, 1);
        assert!(list.find_by_key(&[("id", "z")]).is_none());
    }

    // Mutation via the read handle must still work.
    {
        let mut list =
            tree.user_ordered_list_at("/cambium-user-ordered-demo:lists/user-list[id='a']")?;
        list.remove(1)?;
    }

    {
        let list =
            tree.user_ordered_list_at("/cambium-user-ordered-demo:lists/user-list[id='a']")?;
        assert_eq!(list.len(), 2);
        assert!(list.find_by_key(&[("id", "b")]).is_none());
    }
    Ok(())
}

#[test]
fn as_user_ordered_none_for_system_ordered() -> Result<()> {
    let ctx = load_context();
    let mut tree = build_tree(&ctx)?;
    tree.new_path(
        "/cambium-user-ordered-demo:lists/system-list[id='x']",
        None,
        NewPathOpts::default(),
    )?;
    tree.new_path(
        "/cambium-user-ordered-demo:lists/system-list[id='x']/value",
        Some("9"),
        NewPathOpts::default(),
    )?;

    let user = tree.get("/cambium-user-ordered-demo:lists/user-list[id='a']")?;
    assert!(user.clone().as_user_ordered()?.is_some());

    // The returned view is READ-ONLY (it borrows the tree immutably); reordering
    // would require DataTree::user_ordered_list_at(&mut self).
    {
        let list = user.as_user_ordered()?.unwrap();
        assert_eq!(list.len(), 3);
    }

    let system = tree.get("/cambium-user-ordered-demo:lists/system-list[id='x']")?;
    assert!(system.clone().as_user_ordered()?.is_none());
    assert_eq!(system.schema()?.ordered_by(), OrderedBy::System);
    Ok(())
}

#[test]
fn user_ordered_leaf_list_read_side() -> Result<()> {
    let ctx = load_context();
    let mut tree = ctx.new_data();
    tree.new_path(
        "/cambium-user-ordered-demo:lists",
        None,
        NewPathOpts::default(),
    )?;
    // Seed the leaf-list so the handle can be attached to an existing instance.
    tree.new_path(
        "/cambium-user-ordered-demo:lists/user-tags",
        Some("seed"),
        NewPathOpts::default(),
    )?;

    {
        let mut tags =
            tree.user_ordered_leaf_list_at("/cambium-user-ordered-demo:lists/user-tags[.='seed']")?;
        tags.insert_last("red")?;
        tags.insert_last("green")?;
        tags.insert_first("blue")?;
    }

    {
        let tags =
            tree.user_ordered_leaf_list_at("/cambium-user-ordered-demo:lists/user-tags[.='blue']")?;
        assert_eq!(tags.len(), 4);
        assert_eq!(tags.get(0), Some("blue".to_string()));
        assert_eq!(tags.get(1), Some("seed".to_string()));
        assert_eq!(tags.get(2), Some("red".to_string()));
        assert_eq!(tags.get(3), Some("green".to_string()));
        assert_eq!(
            tags.iter().collect::<Vec<_>>(),
            vec!["blue", "seed", "red", "green"]
        );
    }

    {
        let mut tags =
            tree.user_ordered_leaf_list_at("/cambium-user-ordered-demo:lists/user-tags[.='blue']")?;
        tags.remove(1)?;
    }

    {
        let tags =
            tree.user_ordered_leaf_list_at("/cambium-user-ordered-demo:lists/user-tags[.='blue']")?;
        assert_eq!(tags.len(), 3);
        assert_eq!(
            tags.iter().collect::<Vec<_>>(),
            vec!["blue", "red", "green"]
        );
    }
    Ok(())
}

#[test]
fn user_ordered_leaf_list_insert_positions() -> Result<()> {
    let ctx = load_context();
    let mut tree = ctx.new_data();
    tree.new_path(
        "/cambium-user-ordered-demo:lists",
        None,
        NewPathOpts::default(),
    )?;
    tree.new_path(
        "/cambium-user-ordered-demo:lists/user-tags",
        Some("seed"),
        NewPathOpts::default(),
    )?;

    {
        let mut tags =
            tree.user_ordered_leaf_list_at("/cambium-user-ordered-demo:lists/user-tags[.='seed']")?;
        tags.insert_last("one")?;
        tags.insert_last("three")?;
        tags.insert_before(1, "two")?;
        tags.insert_after(0, "half")?;
    }

    let tags =
        tree.user_ordered_leaf_list_at("/cambium-user-ordered-demo:lists/user-tags[.='half']")?;
    assert_eq!(
        tags.iter().collect::<Vec<_>>(),
        vec!["seed", "half", "two", "one", "three"]
    );
    Ok(())
}

#[test]
fn user_ordered_leaf_list_move() -> Result<()> {
    let ctx = load_context();
    let mut tree = ctx.new_data();
    tree.new_path(
        "/cambium-user-ordered-demo:lists",
        None,
        NewPathOpts::default(),
    )?;
    tree.new_path(
        "/cambium-user-ordered-demo:lists/user-tags",
        Some("seed"),
        NewPathOpts::default(),
    )?;

    {
        let mut tags =
            tree.user_ordered_leaf_list_at("/cambium-user-ordered-demo:lists/user-tags[.='seed']")?;
        tags.insert_last("a")?;
        tags.insert_last("b")?;
        tags.insert_last("c")?;
        tags.move_before(3, 0)?;
    }

    let tags =
        tree.user_ordered_leaf_list_at("/cambium-user-ordered-demo:lists/user-tags[.='c']")?;
    assert_eq!(tags.iter().collect::<Vec<_>>(), vec!["c", "seed", "a", "b"]);
    Ok(())
}

#[test]
fn non_user_ordered_list_errors() -> Result<()> {
    let ctx = load_context();
    let mut tree = ctx.new_data();
    tree.new_path(
        "/cambium-user-ordered-demo:lists",
        None,
        NewPathOpts::default(),
    )?;
    tree.new_path(
        "/cambium-user-ordered-demo:lists/system-list[id='x']",
        None,
        NewPathOpts::default(),
    )?;
    tree.new_path(
        "/cambium-user-ordered-demo:lists/system-list[id='x']/value",
        Some("9"),
        NewPathOpts::default(),
    )?;
    tree.new_path(
        "/cambium-user-ordered-demo:lists/system-tags",
        Some("seed"),
        NewPathOpts::default(),
    )?;

    let err = tree
        .user_ordered_list_at("/cambium-user-ordered-demo:lists/system-list[id='x']")
        .unwrap_err();
    assert_eq!(err.rule_code(), RuleCode::OrderedList);

    let err = tree
        .user_ordered_leaf_list_at("/cambium-user-ordered-demo:lists/system-tags[.='seed']")
        .unwrap_err();
    assert_eq!(err.rule_code(), RuleCode::OrderedList);
    Ok(())
}

#[test]
fn user_ordered_list_preserves_order_in_serialize() -> Result<()> {
    let ctx = load_context();
    let mut tree = ctx.new_data();
    tree.new_path(
        "/cambium-user-ordered-demo:lists",
        None,
        NewPathOpts::default(),
    )?;
    tree.new_path(
        "/cambium-user-ordered-demo:lists/user-tags",
        Some("seed"),
        NewPathOpts::default(),
    )?;

    {
        let mut tags =
            tree.user_ordered_leaf_list_at("/cambium-user-ordered-demo:lists/user-tags[.='seed']")?;
        tags.insert_last("third")?;
        tags.insert_first("first")?;
        tags.insert_after(0, "second")?;
    }

    let xml = String::from_utf8(tree.serialize(
        Format::Xml,
        SerializeFlags {
            siblings: true,
            ..Default::default()
        },
    )?)?;
    // Verify the user order is preserved (not alphabetical).
    let first_pos = xml.find("first").unwrap();
    let second_pos = xml.find("second").unwrap();
    let third_pos = xml.find("third").unwrap();
    assert!(first_pos < second_pos && second_pos < third_pos);
    Ok(())
}
