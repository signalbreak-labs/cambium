//! Phase 3 Slice 1: serialization flag completeness + byte-stability gate.
#![allow(clippy::expect_used, clippy::unwrap_used)]

use std::fs;
use std::path::PathBuf;

use cambium_core::{
    ContextBuilder, Format, ImplicitOpts, NewPathOpts, ParseMode, Result, SerializeFlags,
    WithDefaults,
};

fn project_root() -> PathBuf {
    PathBuf::from(env!("CARGO_MANIFEST_DIR"))
        .join("../..")
        .canonicalize()
        .unwrap()
}

fn load_ordered_user_context() -> cambium_core::Context {
    let dir = project_root().join("conformance/fixtures/ordered-user/module");
    ContextBuilder::new(Default::default())
        .unwrap()
        .search_path(&dir)
        .unwrap()
        .load_module("ordered-user-demo", None, &[])
        .unwrap()
        .build()
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
        .join("../../target/tests/serialize-flags/modules");
    fs::create_dir_all(&dir).unwrap();
    let path = dir.join("cambium-data-crud-demo.yang");
    fs::write(&path, yang).unwrap();
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
fn serialize_default_bytes_unchanged() -> Result<()> {
    let ctx = load_ordered_user_context();
    let input =
        fs::read(project_root().join("conformance/fixtures/ordered-user/input.xml")).unwrap();
    let tree = ctx.parse(Format::Xml, ParseMode::data_only(), &input)?;

    let golden_xml =
        fs::read(project_root().join("conformance/golden/ordered-user/output.xml")).unwrap();
    let golden_json =
        fs::read(project_root().join("conformance/golden/ordered-user/output.json")).unwrap();

    let actual_xml = tree.serialize(Format::Xml, SerializeFlags::default())?;
    let actual_json = tree.serialize(Format::Json, SerializeFlags::default())?;

    assert_eq!(
        actual_xml, golden_xml,
        "XML default flags changed golden bytes"
    );
    assert_eq!(
        actual_json, golden_json,
        "JSON default flags changed golden bytes"
    );
    Ok(())
}

#[test]
fn serialize_with_defaults_explicit_equals_today() -> Result<()> {
    let ctx = load_ordered_user_context();
    let input =
        fs::read(project_root().join("conformance/fixtures/ordered-user/input.xml")).unwrap();
    let tree = ctx.parse(Format::Xml, ParseMode::data_only(), &input)?;

    let explicit = SerializeFlags {
        with_defaults: WithDefaults::Explicit,
        ..SerializeFlags::default()
    };
    assert_eq!(
        tree.serialize(Format::Xml, SerializeFlags::default())?,
        tree.serialize(Format::Xml, explicit)?
    );
    assert_eq!(
        tree.serialize(Format::Json, SerializeFlags::default())?,
        tree.serialize(Format::Json, explicit)?
    );
    Ok(())
}

#[test]
fn serialize_with_defaults_all_shows_default_leaves() -> Result<()> {
    let ctx = load_crud_context();
    let mut tree = ctx.new_data();
    tree.new_path("/cambium-data-crud-demo:top", None, NewPathOpts::default())?;
    tree.new_path(
        "/cambium-data-crud-demo:top/counter",
        Some("7"),
        NewPathOpts::default(),
    )?;
    tree.add_defaults(ImplicitOpts::default())?;

    let xml = String::from_utf8(tree.serialize(
        Format::Xml,
        SerializeFlags {
            with_defaults: WithDefaults::All,
            ..SerializeFlags::default()
        },
    )?)
    .unwrap();
    let e = xml.find("<enabled").expect("enabled in All XML");
    let c = xml.find("<counter").expect("counter in All XML");
    assert!(
        e < c,
        "default leaf must appear in declaration order before counter"
    );

    let json = String::from_utf8(tree.serialize(
        Format::Json,
        SerializeFlags {
            with_defaults: WithDefaults::All,
            ..SerializeFlags::default()
        },
    )?)
    .unwrap();
    let je = json.find("\"enabled\"").expect("enabled in All JSON");
    let jc = json.find("\"counter\"").expect("counter in All JSON");
    assert!(
        je < jc,
        "default leaf must appear in declaration order before counter in JSON"
    );
    Ok(())
}

#[test]
fn serialize_with_defaults_trim_hides_default_leaves() -> Result<()> {
    let ctx = load_crud_context();
    let mut tree = ctx.new_data();
    tree.new_path("/cambium-data-crud-demo:top", None, NewPathOpts::default())?;
    tree.new_path(
        "/cambium-data-crud-demo:top/counter",
        Some("7"),
        NewPathOpts::default(),
    )?;
    tree.add_defaults(ImplicitOpts::default())?;

    let xml = String::from_utf8(tree.serialize(
        Format::Xml,
        SerializeFlags {
            with_defaults: WithDefaults::Trim,
            ..SerializeFlags::default()
        },
    )?)
    .unwrap();
    assert!(
        !xml.contains("enabled"),
        "Trim must hide default-valued enabled leaf"
    );
    assert!(
        xml.contains("counter"),
        "Trim must keep non-default counter"
    );

    let json = String::from_utf8(tree.serialize(
        Format::Json,
        SerializeFlags {
            with_defaults: WithDefaults::Trim,
            ..SerializeFlags::default()
        },
    )?)
    .unwrap();
    assert!(
        !json.contains("\"enabled\""),
        "Trim must hide default-valued enabled leaf in JSON"
    );
    Ok(())
}

#[test]
fn serialize_keep_empty_container() -> Result<()> {
    let ctx = load_crud_context();
    let mut tree = ctx.new_data();
    tree.new_path("/cambium-data-crud-demo:top", None, NewPathOpts::default())?;
    tree.new_path(
        "/cambium-data-crud-demo:top/counter",
        Some("7"),
        NewPathOpts::default(),
    )?;
    tree.new_path(
        "/cambium-data-crud-demo:top/nested",
        None,
        NewPathOpts::default(),
    )?;

    let xml_without =
        String::from_utf8(tree.serialize(Format::Xml, SerializeFlags::default())?).unwrap();
    assert!(xml_without.contains("counter"), "counter must be present");
    assert!(
        !xml_without.contains("nested"),
        "empty container should be pruned without flag"
    );

    let xml_with = String::from_utf8(tree.serialize(
        Format::Xml,
        SerializeFlags {
            keep_empty_containers: true,
            ..SerializeFlags::default()
        },
    )?)
    .unwrap();
    assert!(
        xml_with.contains("counter"),
        "counter must still be present"
    );
    assert!(
        xml_with.contains("nested"),
        "empty container should be kept with flag"
    );

    let json_without =
        String::from_utf8(tree.serialize(Format::Json, SerializeFlags::default())?).unwrap();
    assert!(
        json_without.contains("\"counter\""),
        "counter must be present in JSON"
    );
    assert!(
        !json_without.contains("\"nested\""),
        "empty container should be pruned in JSON without flag"
    );

    let json_with = String::from_utf8(tree.serialize(
        Format::Json,
        SerializeFlags {
            keep_empty_containers: true,
            ..SerializeFlags::default()
        },
    )?)
    .unwrap();
    assert!(
        json_with.contains("\"counter\""),
        "counter must still be present in JSON"
    );
    assert!(
        json_with.contains("\"nested\""),
        "empty container should be kept in JSON with flag"
    );
    Ok(())
}

#[test]
fn serialize_shrink_removes_insignificant_whitespace() -> Result<()> {
    let ctx = load_crud_context();
    let mut tree = ctx.new_data();
    tree.new_path("/cambium-data-crud-demo:top", None, NewPathOpts::default())?;
    tree.new_path(
        "/cambium-data-crud-demo:top/counter",
        Some("7"),
        NewPathOpts::default(),
    )?;

    let pretty =
        String::from_utf8(tree.serialize(Format::Xml, SerializeFlags::default())?).unwrap();
    let shrunk = String::from_utf8(tree.serialize(
        Format::Xml,
        SerializeFlags {
            shrink: true,
            ..SerializeFlags::default()
        },
    )?)
    .unwrap();

    assert!(pretty.contains("\n  "), "default XML should be indented");
    assert!(
        !shrunk.contains("\n  "),
        "shrink should remove indentation whitespace"
    );
    assert!(
        shrunk.contains("<counter>7</counter>"),
        "shrink must keep the value"
    );
    Ok(())
}
