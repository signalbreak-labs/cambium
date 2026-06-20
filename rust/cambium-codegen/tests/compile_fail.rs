//! Compile-fail assertions for the generated `UserOrderedVec<T>` surface.
#![allow(clippy::unwrap_used, clippy::expect_used)]

use std::fs;
use std::path::PathBuf;

use cambium_codegen::{CodegenOpts, generate};
use cambium_core::{ContextBuilder, ContextFlags};

fn fixture_dir(name: &str) -> PathBuf {
    PathBuf::from(env!("CARGO_MANIFEST_DIR"))
        .join("../../conformance/fixtures")
        .join(name)
}

fn module_name_from_dir(name: &str) -> String {
    let dir = fixture_dir(name).join("module");
    for entry in fs::read_dir(&dir).unwrap() {
        let entry = entry.unwrap();
        let fname = entry.file_name();
        let s = fname.to_str().unwrap();
        if s.ends_with(".yang") {
            return s.trim_end_matches(".yang").to_string();
        }
    }
    panic!("no .yang module file found in {dir:?}");
}

fn load_ctx_for_fixture(name: &str) -> cambium_core::Context {
    let dir = fixture_dir(name).join("module");
    ContextBuilder::new(ContextFlags {
        no_yang_library: true,
        ..Default::default()
    })
    .unwrap()
    .search_path(&dir)
    .unwrap()
    .load_module(&module_name_from_dir(name), None, &[])
    .unwrap()
    .build()
    .unwrap()
}

#[test]
fn compile_fail_user_ordered_vec() {
    let ctx = load_ctx_for_fixture("ordering-nested-user-cascading");
    let module = module_name_from_dir("ordering-nested-user-cascading");
    let generated = generate(&ctx, &module, CodegenOpts::default()).unwrap();

    // Strip crate-level doc comments and attributes so the source can be `include!`-d
    // into a test file.
    let lines: Vec<_> = generated.source.lines().collect();
    let skip = lines
        .iter()
        .take_while(|l| l.starts_with("#!") || l.starts_with("//!"))
        .count();
    let module_src = lines[skip..].join("\n");

    let path = PathBuf::from(env!("CARGO_MANIFEST_DIR"))
        .join("tests/compile-fail/_generated_user_ordered_vec.rs");

    // The fixture is a committed snapshot of the live emitter output that the
    // trybuild cases below `include!`. Verify it matches the current emitter
    // rather than blindly overwriting it on every run — overwriting dirties the
    // working tree and would silently absorb a wrong emitter change. Refresh
    // deliberately with `BLESS_FIXTURES=1 cargo test -p cambium-codegen --test compile_fail`.
    if std::env::var_os("BLESS_FIXTURES").is_some() {
        fs::write(&path, &module_src).unwrap();
    } else {
        let on_disk = fs::read_to_string(&path).unwrap_or_default();
        assert_eq!(
            on_disk, module_src,
            "compile-fail fixture is stale vs the emitter; refresh with \
             BLESS_FIXTURES=1 cargo test -p cambium-codegen --test compile_fail"
        );
    }

    let t = trybuild::TestCases::new();
    t.compile_fail("tests/compile-fail/user_ordered_vec_*.rs");
}
