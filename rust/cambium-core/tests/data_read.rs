//! Phase 2 Slice 1 acceptance tests: data-tree read side.
#![allow(clippy::unwrap_used, clippy::expect_used)]

use std::fs;
use std::path::PathBuf;
use std::sync::atomic::{AtomicU64, Ordering};

use cambium_core::{
    BaseType, ContextBuilder, DataTree, Decimal64, Format, ParseMode, Result, RuleCode, Value,
};

fn temp_module_dir() -> PathBuf {
    let dir =
        PathBuf::from(env!("CARGO_MANIFEST_DIR")).join("../../target/tests/data-read/modules");
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

fn load_context() -> (cambium_core::Context, PathBuf) {
    let yang = r#"module cambium-data-read-demo {
    namespace "urn:cambium:data-read";
    prefix cdr;
    yang-version 1.1;
    revision 2026-06-14;

    container top {
        leaf rw-flag {
            type boolean;
            default "true";
        }
        leaf ro-counter {
            config false;
            type uint64;
        }
        leaf deprecated-leaf {
            status deprecated;
            type string;
        }
        leaf mandatory-leaf {
            mandatory "true";
            type string;
        }
        leaf all-builtins {
            type int64;
        }
        leaf dec64 {
            type decimal64 {
                fraction-digits 4;
            }
        }
        leaf status-enum {
            type enumeration {
                enum up { value 1; }
                enum down { value 2; }
                enum unknown { value 0; }
            }
        }
    }
}
"#;
    let dir = write_module("cambium-data-read-demo", yang);
    let ctx = ContextBuilder::new(Default::default())
        .unwrap()
        .search_path(&dir)
        .unwrap()
        .load_module("cambium-data-read-demo", None, &[])
        .unwrap()
        .build()
        .unwrap();
    (ctx, dir)
}

fn parse_demo(ctx: &cambium_core::Context) -> Result<DataTree<'_>> {
    // Deliberately scrambled order: system-ordered container must canonicalize.
    let xml = r#"<top xmlns="urn:cambium:data-read">
  <mandatory-leaf>required</mandatory-leaf>
  <rw-flag>true</rw-flag>
  <status-enum>up</status-enum>
  <dec64>12.3400</dec64>
  <all-builtins>-7</all-builtins>
</top>"#;
    ctx.parse(Format::Xml, ParseMode::data_only(), xml.as_bytes())
}

#[test]
fn get_value_typed() -> Result<()> {
    let (ctx, _dir) = load_context();
    let tree = parse_demo(&ctx)?;

    let rw = tree.get("/cambium-data-read-demo:top/rw-flag")?;
    assert_eq!(rw.value()?.unwrap(), Value::Bool(true));

    let all = tree.get("/cambium-data-read-demo:top/all-builtins")?;
    assert_eq!(all.value()?.unwrap(), Value::Int64(-7));

    let dec = tree.get("/cambium-data-read-demo:top/dec64")?;
    assert_eq!(
        dec.value()?.unwrap(),
        Value::Decimal64(Decimal64::new(123_400, 4))
    );

    let enm = tree.get("/cambium-data-read-demo:top/status-enum")?;
    assert_eq!(enm.value()?.unwrap(), Value::Enum("up".to_string()));
    Ok(())
}

#[test]
fn get_value_str_canonical() -> Result<()> {
    let (ctx, _dir) = load_context();
    let tree = parse_demo(&ctx)?;

    assert_eq!(
        tree.get("/cambium-data-read-demo:top/rw-flag")?
            .value_str()?,
        Some("true".to_string())
    );
    assert_eq!(
        tree.get("/cambium-data-read-demo:top/all-builtins")?
            .value_str()?,
        Some("-7".to_string())
    );
    assert_eq!(
        tree.get("/cambium-data-read-demo:top/dec64")?.value_str()?,
        Some("12.34".to_string())
    );
    Ok(())
}

#[test]
fn get_try_get_exists() -> Result<()> {
    let (ctx, _dir) = load_context();
    let tree = parse_demo(&ctx)?;

    assert!(tree.exists("/cambium-data-read-demo:top/rw-flag"));
    assert!(
        tree.try_get("/cambium-data-read-demo:top/rw-flag")
            .is_some()
    );
    assert!(!tree.exists("/cambium-data-read-demo:top/no-such"));
    assert!(
        tree.try_get("/cambium-data-read-demo:top/no-such")
            .is_none()
    );

    let err = tree.get("/cambium-data-read-demo:top/no-such").unwrap_err();
    assert_eq!(err.rule_code(), RuleCode::DataPath);
    Ok(())
}

#[test]
fn select_document_order() -> Result<()> {
    let (ctx, _dir) = load_context();
    let tree = parse_demo(&ctx)?;

    let nodes: Vec<_> = tree
        .select("/cambium-data-read-demo:top/*")?
        .iter()
        .collect();
    let names: Vec<_> = nodes.iter().map(|n| n.name().to_string()).collect();
    // System-ordered container canonicalizes to schema order.
    assert_eq!(
        names,
        vec![
            "rw-flag",
            "mandatory-leaf",
            "all-builtins",
            "dec64",
            "status-enum"
        ]
    );
    Ok(())
}

#[test]
fn children_one_ffi_walk_ordered() -> Result<()> {
    let (ctx, _dir) = load_context();
    let tree = parse_demo(&ctx)?;

    let top = tree.get("/cambium-data-read-demo:top")?;
    let names: Vec<_> = top
        .children()?
        .iter()
        .map(|n| n.name().to_string())
        .collect();
    assert_eq!(
        names,
        vec![
            "rw-flag",
            "mandatory-leaf",
            "all-builtins",
            "dec64",
            "status-enum"
        ]
    );
    Ok(())
}

#[test]
fn node_schema_bridge() -> Result<()> {
    let (ctx, _dir) = load_context();
    let tree = parse_demo(&ctx)?;

    let rw = tree.get("/cambium-data-read-demo:top/rw-flag")?;
    let schema = rw.schema()?;
    assert_eq!(schema.name(), "rw-flag");
    assert_eq!(schema.leaf_type().unwrap().base(), BaseType::Boolean);

    let all = tree.get("/cambium-data-read-demo:top/all-builtins")?;
    assert_eq!(all.schema()?.leaf_type().unwrap().base(), BaseType::Int64);
    Ok(())
}
