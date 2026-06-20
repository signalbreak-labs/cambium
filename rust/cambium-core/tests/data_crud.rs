//! Phase 2 Slice 2 acceptance tests: data-tree create / mutate.
#![allow(clippy::unwrap_used, clippy::expect_used)]

use std::fs;
use std::path::PathBuf;
use std::sync::atomic::{AtomicU64, Ordering};

use cambium_core::{
    ContextBuilder, Format, ImplicitOpts, NewPathOpts, Result, RuleCode, ValidateMode, Value,
};

fn temp_module_dir() -> PathBuf {
    let dir =
        PathBuf::from(env!("CARGO_MANIFEST_DIR")).join("../../target/tests/data-crud/modules");
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
    let dir = write_module("cambium-data-crud-demo", yang);
    ContextBuilder::new(Default::default())
        .unwrap()
        .search_path(&dir)
        .unwrap()
        .load_module("cambium-data-crud-demo", None, &[])
        .unwrap()
        .build()
        .unwrap()
}

#[test]
fn new_path_then_serialize_declaration_order() -> Result<()> {
    // I2 for BUILT trees: add `top`'s children via new_path in a SCRAMBLED order;
    // libyang must place each at its schema-declared position, so serialization is
    // in declaration order (enabled, counter, nested), NOT insertion order. Without
    // this, an out-of-order new_path could serialize in insertion order undetected.
    let ctx = load_context();
    let mut tree = ctx.new_data();
    tree.new_path("/cambium-data-crud-demo:top", None, NewPathOpts::default())?;
    // insertion order is the reverse of declaration order:
    tree.new_path(
        "/cambium-data-crud-demo:top/nested/name",
        Some("n"),
        NewPathOpts::default(),
    )?;
    tree.new_path(
        "/cambium-data-crud-demo:top/counter",
        Some("7"),
        NewPathOpts::default(),
    )?;
    tree.new_path(
        "/cambium-data-crud-demo:top/enabled",
        Some("false"),
        NewPathOpts::default(),
    )?;

    let xml = String::from_utf8(tree.serialize(Format::Xml, Default::default())?).unwrap();
    let xe = xml.find("<enabled").expect("enabled in xml");
    let xc = xml.find("<counter").expect("counter in xml");
    let xn = xml.find("<nested").expect("nested in xml");
    assert!(
        xe < xc && xc < xn,
        "XML must be schema declaration order enabled<counter<nested, got:\n{xml}"
    );

    // JSON (not JsonIetf, to avoid empty-container differences) must agree.
    let json = String::from_utf8(tree.serialize(Format::Json, Default::default())?).unwrap();
    let je = json.find("\"enabled\"").expect("enabled in json");
    let jc = json.find("\"counter\"").expect("counter in json");
    let jn = json.find("\"nested\"").expect("nested in json");
    assert!(
        je < jc && jc < jn,
        "JSON must be schema declaration order, got:\n{json}"
    );
    Ok(())
}

#[test]
fn new_path_creates_nodes() -> Result<()> {
    let ctx = load_context();
    let mut tree = ctx.new_data();

    tree.new_path("/cambium-data-crud-demo:top", None, NewPathOpts::default())?;
    let addr = tree.new_path(
        "/cambium-data-crud-demo:top/counter",
        Some("42"),
        NewPathOpts::default(),
    )?;
    assert_eq!(addr.path(), "/cambium-data-crud-demo:top/counter");

    let counter = tree.get("/cambium-data-crud-demo:top/counter")?;
    assert_eq!(counter.value()?.unwrap(), Value::Uint64(42));
    Ok(())
}

#[test]
fn new_path_updates_existing_leaf() -> Result<()> {
    let ctx = load_context();
    let mut tree = ctx.new_data();

    tree.new_path("/cambium-data-crud-demo:top", None, NewPathOpts::default())?;
    tree.new_path(
        "/cambium-data-crud-demo:top/counter",
        Some("5"),
        NewPathOpts::default(),
    )?;
    tree.new_path(
        "/cambium-data-crud-demo:top/counter",
        Some("7"),
        NewPathOpts {
            update: true,
            ..Default::default()
        },
    )?;

    assert_eq!(
        tree.get("/cambium-data-crud-demo:top/counter")?
            .value()?
            .unwrap(),
        Value::Uint64(7)
    );
    Ok(())
}

#[test]
fn set_value_reports_change() -> Result<()> {
    let ctx = load_context();
    let mut tree = ctx.new_data();

    tree.new_path("/cambium-data-crud-demo:top", None, NewPathOpts::default())?;
    tree.new_path(
        "/cambium-data-crud-demo:top/counter",
        Some("3"),
        NewPathOpts::default(),
    )?;

    assert!(tree.set_value("/cambium-data-crud-demo:top/counter", "4")?);
    assert!(!tree.set_value("/cambium-data-crud-demo:top/counter", "4")?);
    assert_eq!(
        tree.get("/cambium-data-crud-demo:top/counter")?
            .value()?
            .unwrap(),
        Value::Uint64(4)
    );
    Ok(())
}

#[test]
fn remove_path_removes_subtree() -> Result<()> {
    let ctx = load_context();
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
        Some("inner"),
        NewPathOpts::default(),
    )?;

    tree.remove_path("/cambium-data-crud-demo:top/nested")?;
    assert!(!tree.exists("/cambium-data-crud-demo:top/nested"));
    assert!(!tree.exists("/cambium-data-crud-demo:top/nested/name"));
    assert!(tree.exists("/cambium-data-crud-demo:top/counter"));
    Ok(())
}

#[test]
fn remove_path_rejects_list_key() -> Result<()> {
    let ctx = load_context();
    let mut tree = ctx.new_data();

    tree.new_path(
        "/cambium-data-crud-demo:top/item[id='a']/value",
        Some("1"),
        NewPathOpts::default(),
    )?;
    let key = "/cambium-data-crud-demo:top/item[id='a']/id";
    assert!(tree.exists(key));

    // libyang's lyd_free_tree silently refuses to free a list key (void return);
    // remove_path must surface that as DataPath, not a silent success.
    let err = tree.remove_path(key).unwrap_err();
    assert_eq!(err.rule_code(), RuleCode::DataPath);
    assert!(
        tree.exists(key),
        "key must remain after the rejected removal"
    );
    Ok(())
}

#[test]
fn unlink_path_detaches_subtree() -> Result<()> {
    let ctx = load_context();
    let mut tree = ctx.new_data();

    tree.new_path("/cambium-data-crud-demo:top", None, NewPathOpts::default())?;
    tree.new_path(
        "/cambium-data-crud-demo:top/counter",
        Some("9"),
        NewPathOpts::default(),
    )?;
    tree.new_path(
        "/cambium-data-crud-demo:top/nested",
        None,
        NewPathOpts::default(),
    )?;
    tree.new_path(
        "/cambium-data-crud-demo:top/nested/name",
        Some("detached"),
        NewPathOpts::default(),
    )?;

    let detached = tree.unlink_path("/cambium-data-crud-demo:top/nested")?;
    assert!(!tree.exists("/cambium-data-crud-demo:top/nested"));

    let xml = String::from_utf8(detached.serialize(Format::Xml, Default::default())?).unwrap();
    assert!(xml.contains("<nested"));
    assert!(xml.contains("<name>detached</name>"));
    Ok(())
}

#[test]
fn add_defaults_adds_default_leaves() -> Result<()> {
    let ctx = load_context();
    let mut tree = ctx.new_data();

    tree.new_path("/cambium-data-crud-demo:top", None, NewPathOpts::default())?;
    assert!(!tree.exists("/cambium-data-crud-demo:top/enabled"));

    tree.add_defaults(ImplicitOpts::default())?;
    let enabled = tree.get("/cambium-data-crud-demo:top/enabled")?;
    assert_eq!(enabled.value()?.unwrap(), Value::Bool(true));
    assert!(enabled.is_default()?);
    Ok(())
}

#[test]
fn validate_after_mutation_passes() -> Result<()> {
    let ctx = load_context();
    let mut tree = ctx.new_data();

    tree.new_path("/cambium-data-crud-demo:top", None, NewPathOpts::default())?;
    tree.new_path(
        "/cambium-data-crud-demo:top/counter",
        Some("99"),
        NewPathOpts::default(),
    )?;
    tree.validate(ValidateMode {
        present: true,
        ..Default::default()
    })?;
    Ok(())
}

#[test]
fn invalid_path_returns_data_path_rule_code() -> Result<()> {
    let ctx = load_context();
    let mut tree = ctx.new_data();

    let err = tree
        .new_path(
            "/cambium-data-crud-demo:top/no-such-leaf",
            Some("x"),
            NewPathOpts::default(),
        )
        .unwrap_err();
    assert_eq!(err.rule_code(), RuleCode::DataPath);
    Ok(())
}

#[test]
fn new_path_list_entry() -> Result<()> {
    let ctx = load_context();
    let mut tree = ctx.new_data();

    tree.new_path("/cambium-data-crud-demo:top", None, NewPathOpts::default())?;
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

    let item = tree.get("/cambium-data-crud-demo:top/item[id='a']")?;
    assert_eq!(item.children()?.len(), 2); // id, value
    assert_eq!(
        tree.get("/cambium-data-crud-demo:top/item[id='a']/value")?
            .value()?
            .unwrap(),
        Value::Uint64(10)
    );
    Ok(())
}
