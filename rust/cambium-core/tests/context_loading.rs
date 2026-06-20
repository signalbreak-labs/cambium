#![allow(clippy::unwrap_used)]

use std::fs;
use std::path::{Path, PathBuf};
use std::sync::atomic::{AtomicU64, Ordering};

use cambium_core::{Context, ContextBuilder};

static WRITE_COUNTER: AtomicU64 = AtomicU64::new(0);

fn temp_module_dir() -> PathBuf {
    let dir = PathBuf::from(env!("CARGO_MANIFEST_DIR"))
        .join("../../target/tests/context-loading/modules");
    fs::create_dir_all(&dir).unwrap();
    dir
}

fn atomic_write(path: &Path, content: &str) {
    let dir = path.parent().unwrap();
    let n = WRITE_COUNTER.fetch_add(1, Ordering::Relaxed);
    let tmp = dir.join(format!(".tmp-{n}.yang"));
    fs::write(&tmp, content).unwrap();
    fs::rename(&tmp, path).unwrap();
}

#[test]
fn builder_loads_module_from_path_and_lists_modules() {
    let dir = temp_module_dir();
    let path = dir.join("cambium-load-path.yang");
    atomic_write(
        &path,
        r#"module cambium-load-path {
    yang-version 1.1;
    namespace "urn:cambium:load-path";
    prefix clp;

    leaf root { type string; }
}
"#,
    );

    let ctx = ContextBuilder::new(Default::default())
        .unwrap()
        .load_module_path(&path)
        .unwrap()
        .build()
        .unwrap();

    let module = ctx.schema("cambium-load-path").unwrap();
    assert_eq!(module.namespace(), "urn:cambium:load-path");

    let names: Vec<_> = ctx
        .modules()
        .into_iter()
        .map(|module| module.name().to_string())
        .collect();
    assert!(names.iter().any(|name| name == "cambium-load-path"));
}

#[test]
fn builder_loads_module_from_string() {
    let ctx = ContextBuilder::new(Default::default())
        .unwrap()
        .load_module_str(
            r#"module cambium-load-string {
    yang-version 1.1;
    namespace "urn:cambium:load-string";
    prefix cls;

    leaf root { type string; }
}
"#,
        )
        .unwrap()
        .build()
        .unwrap();

    let module = ctx.schema("cambium-load-string").unwrap();
    assert_eq!(module.prefix(), "cls");
}

#[test]
fn legacy_context_load_module_refreshes_schema_forest() {
    let dir = temp_module_dir();
    atomic_write(
        &dir.join("cambium-legacy-load.yang"),
        r#"module cambium-legacy-load {
    yang-version 1.1;
    namespace "urn:cambium:legacy-load";
    prefix cll;

    leaf root { type string; }
}
"#,
    );

    let mut ctx = Context::new().unwrap();
    ctx.set_search_path(&dir).unwrap();
    ctx.load_module("cambium-legacy-load").unwrap();

    let module = ctx.schema("cambium-legacy-load").unwrap();
    assert_eq!(module.prefix(), "cll");
}
