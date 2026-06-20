//! Phase 2 Slice 3 acceptance tests: structured validation diagnostics.
#![allow(clippy::unwrap_used, clippy::expect_used)]

use std::fs;
use std::path::PathBuf;
use std::sync::atomic::{AtomicU64, Ordering};

use cambium_core::{
    ContextBuilder, ErrorType, NewPathOpts, Result, RuleCode, ValidateMode, ValidationCode,
    ValidationErrors,
};

fn temp_module_dir() -> PathBuf {
    let dir = PathBuf::from(env!("CARGO_MANIFEST_DIR"))
        .join("../../target/tests/data-validation/modules");
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
    let yang = r#"module cambium-validation-demo {
    namespace "urn:cambium:validation";
    prefix cvd;
    yang-version 1.1;
    revision 2026-06-14;

    container top {
        leaf name {
            type string;
        }
        leaf ref {
            type leafref {
                path "../name";
            }
        }
        container c {
            leaf x {
                type uint8;
                must "../../name = 'open'" {
                    error-app-tag "must-violation";
                }
            }
        }
        leaf y {
            when "../name = 'enable'";
            type string;
        }
        leaf z {
            mandatory "true";
            type string;
        }
    }
}
"#;
    let dir = write_module("cambium-validation-demo", yang);
    ContextBuilder::new(Default::default())
        .unwrap()
        .search_path(&dir)
        .unwrap()
        .load_module("cambium-validation-demo", None, &[])
        .unwrap()
        .build()
        .unwrap()
}

fn validation_errors(err: &cambium_core::Error) -> Option<&ValidationErrors> {
    match err {
        cambium_core::Error::Validation(errors) => Some(errors),
        _ => None,
    }
}

#[test]
fn validate_must_when_fails_with_path() -> Result<()> {
    let ctx = load_context();
    let mut tree = ctx.new_data();

    tree.new_path("/cambium-validation-demo:top", None, NewPathOpts::default())?;
    tree.new_path(
        "/cambium-validation-demo:top/name",
        Some("closed"),
        NewPathOpts::default(),
    )?;
    tree.new_path(
        "/cambium-validation-demo:top/c",
        None,
        NewPathOpts::default(),
    )?;
    tree.new_path(
        "/cambium-validation-demo:top/c/x",
        Some("1"),
        NewPathOpts::default(),
    )?;
    tree.new_path(
        "/cambium-validation-demo:top/y",
        Some("foo"),
        NewPathOpts::default(),
    )?;
    tree.new_path(
        "/cambium-validation-demo:top/z",
        Some("ok"),
        NewPathOpts::default(),
    )?;

    let err = tree
        .validate(ValidateMode {
            present: true,
            multi_error: true,
            ..Default::default()
        })
        .unwrap_err();
    assert_eq!(err.rule_code(), RuleCode::Validate);

    let errors = validation_errors(&err).expect("expected Validation errors");
    let codes: Vec<_> = errors.iter().filter_map(|d| d.validation_code).collect();
    assert!(codes.contains(&ValidationCode::Must));
    assert!(codes.contains(&ValidationCode::When));

    let must = errors
        .iter()
        .find(|d| d.validation_code == Some(ValidationCode::Must))
        .unwrap();
    assert_eq!(must.error_app_tag.as_deref(), Some("must-violation"));
    assert_eq!(must.error_type, ErrorType::Application);
    assert!(must.data_path.as_deref().unwrap_or("").contains("/c"));
    Ok(())
}

#[test]
fn validate_mandatory_missing_app_tag() -> Result<()> {
    let ctx = load_context();
    let mut tree = ctx.new_data();

    tree.new_path("/cambium-validation-demo:top", None, NewPathOpts::default())?;
    tree.new_path(
        "/cambium-validation-demo:top/name",
        Some("anything"),
        NewPathOpts::default(),
    )?;

    let err = tree
        .validate(ValidateMode {
            present: true,
            ..Default::default()
        })
        .unwrap_err();
    let errors = validation_errors(&err).expect("expected Validation errors");
    let diag = errors.primary().unwrap();
    assert_eq!(diag.validation_code, Some(ValidationCode::Mandatory));
    assert!(diag.error_app_tag.is_none());
    assert!(diag.message.to_lowercase().contains("mandatory"));
    Ok(())
}

#[test]
fn validate_leafref_unresolved() -> Result<()> {
    let ctx = load_context();
    let mut tree = ctx.new_data();

    tree.new_path("/cambium-validation-demo:top", None, NewPathOpts::default())?;
    tree.new_path(
        "/cambium-validation-demo:top/name",
        Some("a"),
        NewPathOpts::default(),
    )?;
    tree.new_path(
        "/cambium-validation-demo:top/ref",
        Some("no-such"),
        NewPathOpts::default(),
    )?;
    tree.new_path(
        "/cambium-validation-demo:top/z",
        Some("ok"),
        NewPathOpts::default(),
    )?;

    let err = tree
        .validate(ValidateMode {
            present: true,
            ..Default::default()
        })
        .unwrap_err();
    let errors = validation_errors(&err).expect("expected Validation errors");
    let diag = errors.primary().unwrap();
    assert_eq!(diag.validation_code, Some(ValidationCode::Leafref));
    assert_eq!(diag.error_app_tag.as_deref(), Some("instance-required"));
    assert!(diag.data_path.as_deref().unwrap_or("").contains("/ref"));
    Ok(())
}

#[test]
fn validate_multi_error_returns_list() -> Result<()> {
    let ctx = load_context();
    let mut tree = ctx.new_data();

    tree.new_path("/cambium-validation-demo:top", None, NewPathOpts::default())?;
    tree.new_path(
        "/cambium-validation-demo:top/name",
        Some("a"),
        NewPathOpts::default(),
    )?;
    tree.new_path(
        "/cambium-validation-demo:top/ref",
        Some("no-such"),
        NewPathOpts::default(),
    )?;

    let err = tree
        .validate(ValidateMode {
            present: true,
            multi_error: true,
            ..Default::default()
        })
        .unwrap_err();
    let errors = validation_errors(&err).expect("expected Validation errors");
    assert!(
        errors.len() >= 2,
        "expected at least two diagnostics, got {}",
        errors.len()
    );
    let codes: Vec<_> = errors.iter().filter_map(|d| d.validation_code).collect();
    assert!(codes.contains(&ValidationCode::Mandatory));
    assert!(codes.contains(&ValidationCode::Leafref));
    Ok(())
}

#[test]
fn error_source_chain_preserved() -> Result<()> {
    let ctx = load_context();
    let mut tree = ctx.new_data();

    tree.new_path("/cambium-validation-demo:top", None, NewPathOpts::default())?;

    let err = tree
        .validate(ValidateMode {
            present: true,
            ..Default::default()
        })
        .unwrap_err();
    assert_eq!(err.rule_code(), RuleCode::Validate);
    assert!(std::error::Error::source(&err).is_some());

    let errors = validation_errors(&err).unwrap();
    for diag in errors.iter() {
        assert_eq!(diag.code, RuleCode::Validate);
    }
    Ok(())
}

#[test]
fn validate_present_running_datastore_passes() -> Result<()> {
    let ctx = load_context();
    let mut tree = ctx.new_data();

    tree.new_path("/cambium-validation-demo:top", None, NewPathOpts::default())?;
    tree.new_path(
        "/cambium-validation-demo:top/name",
        Some("open"),
        NewPathOpts::default(),
    )?;
    tree.new_path(
        "/cambium-validation-demo:top/z",
        Some("ok"),
        NewPathOpts::default(),
    )?;

    tree.validate(ValidateMode {
        present: true,
        ..Default::default()
    })?;
    Ok(())
}

#[test]
fn diagnostic_is_cloneable_and_stable() -> Result<()> {
    let ctx = load_context();
    let mut tree = ctx.new_data();

    tree.new_path("/cambium-validation-demo:top", None, NewPathOpts::default())?;
    let err = tree
        .validate(ValidateMode {
            present: true,
            ..Default::default()
        })
        .unwrap_err();
    let errors = validation_errors(&err).unwrap();
    let first = errors.primary().unwrap().clone();
    let cloned = first.clone();
    assert_eq!(first, cloned);
    Ok(())
}
