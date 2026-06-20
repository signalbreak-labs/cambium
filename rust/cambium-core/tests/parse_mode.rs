//! Phase 3 Slice 2a: ParseMode reshaped to composable flags.
#![allow(clippy::unwrap_used)]

use std::fs;
use std::path::PathBuf;

use cambium_core::{ContextBuilder, ContextFlags, Format, ParseMode, Result, RuleCode};

fn load_context() -> cambium_core::Context {
    let yang = r#"module parse-mode-demo {
    namespace "urn:parse-mode";
    prefix pmd;
    yang-version 1.1;
    revision 2026-06-14;

    leaf config-value {
        type string;
    }
    leaf state-value {
        config false;
        type string;
    }
}
"#;
    let dir =
        PathBuf::from(env!("CARGO_MANIFEST_DIR")).join("../../target/tests/parse-mode/modules");
    fs::create_dir_all(&dir).unwrap();
    let path = dir.join("parse-mode-demo.yang");
    fs::write(&path, yang).unwrap();
    ContextBuilder::new(ContextFlags {
        no_yang_library: true,
        ..Default::default()
    })
    .unwrap()
    .search_path(&dir)
    .unwrap()
    .load_module("parse-mode-demo", None, &[])
    .unwrap()
    .build()
    .unwrap()
}

#[test]
fn parse_mode_data_only_convenience() -> Result<()> {
    let ctx = load_context();
    let xml = br#"<config-value xmlns="urn:parse-mode">hello</config-value>"#;
    let tree = ctx.parse(Format::Xml, ParseMode::data_only(), xml)?;
    assert!(tree.exists("/parse-mode-demo:config-value"));
    Ok(())
}

#[test]
fn parse_mode_strict_rejects_unknown() -> Result<()> {
    let ctx = load_context();
    let xml = br#"<config-value xmlns="urn:parse-mode">cfg</config-value>
<unknown xmlns="urn:parse-mode">x</unknown>"#;
    let mode = ParseMode {
        strict: true,
        ..Default::default()
    };
    let err = ctx.parse(Format::Xml, mode, xml).unwrap_err();
    assert_eq!(err.rule_code(), RuleCode::Parse);
    Ok(())
}

#[test]
fn parse_mode_strict_allows_known_state() -> Result<()> {
    let ctx = load_context();
    let xml = br#"<config-value xmlns="urn:parse-mode">cfg</config-value>
<state-value xmlns="urn:parse-mode">st</state-value>"#;
    let mode = ParseMode {
        strict: true,
        ..Default::default()
    };
    let tree = ctx.parse(Format::Xml, mode, xml)?;
    assert!(tree.exists("/parse-mode-demo:config-value"));
    assert!(tree.exists("/parse-mode-demo:state-value"));
    Ok(())
}

#[test]
fn parse_mode_no_state_forbids_state() -> Result<()> {
    let ctx = load_context();
    let xml = br#"<config-value xmlns="urn:parse-mode">cfg</config-value>
<state-value xmlns="urn:parse-mode">st</state-value>"#;
    let mode = ParseMode {
        no_state: true,
        ..Default::default()
    };
    let err = ctx.parse(Format::Xml, mode, xml).unwrap_err();
    assert_eq!(err.rule_code(), RuleCode::Parse);
    Ok(())
}

#[test]
fn parse_mode_compose_strict_plus_no_state() -> Result<()> {
    let ctx = load_context();
    let xml = br#"<config-value xmlns="urn:parse-mode">cfg</config-value>
<state-value xmlns="urn:parse-mode">st</state-value>"#;
    let mode = ParseMode {
        strict: true,
        no_state: true,
        ..Default::default()
    };
    let err = ctx.parse(Format::Xml, mode, xml).unwrap_err();
    assert_eq!(err.rule_code(), RuleCode::Parse);
    Ok(())
}

#[test]
fn parse_mode_strict_opaque_rejected() -> Result<()> {
    let ctx = load_context();
    let xml = br#"<config-value xmlns="urn:parse-mode">cfg</config-value>"#;
    let mode = ParseMode {
        strict: true,
        opaque: true,
        ..Default::default()
    };
    let err = ctx.parse(Format::Xml, mode, xml).unwrap_err();
    assert_eq!(err.rule_code(), RuleCode::Parse);
    assert!(err.to_string().contains("mutually exclusive"));
    Ok(())
}
