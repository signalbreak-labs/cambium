//! Headline golden test: scrambled input is emitted in schema declaration order.
//!
//! This is the core Cambium bet — the order goyang loses by alphabetizing its
//! `Entry.Dir` map. Here we prove the engine (libyang) places children in the
//! compiled-schema order even when the input XML presents them scrambled.
#![allow(clippy::unwrap_used)]
#![allow(clippy::expect_used)]

use std::ffi::{CStr, CString};
use std::fs;
use std::path::PathBuf;

use cambium_libyang_sys::*;

fn fixture_dir() -> PathBuf {
    PathBuf::from(env!("CARGO_MANIFEST_DIR"))
        .ancestors()
        .nth(2)
        .unwrap()
        .join("conformance/fixtures/scrambled-children")
}

#[test]
fn scrambled_children_emitted_in_schema_order() {
    let mut ctx: *mut ly_ctx = std::ptr::null_mut();
    let search_dir = CString::new(fixture_dir().join("module").to_str().unwrap()).unwrap();
    let rc = unsafe { ly_ctx_new(search_dir.as_ptr(), LY_CTX_NO_YANGLIBRARY, &mut ctx) };
    assert_eq!(rc, LY_ERR::LY_SUCCESS);
    assert!(!ctx.is_null());

    let module_name = CString::new("order-demo").unwrap();
    let module = unsafe {
        ly_ctx_load_module(
            ctx,
            module_name.as_ptr(),
            std::ptr::null(),
            std::ptr::null_mut(),
        )
    };
    assert!(!module.is_null(), "module must load");

    let input = fs::read_to_string(fixture_dir().join("input.xml")).unwrap();
    let input_c = CString::new(input.as_bytes()).unwrap();

    let mut tree: *mut lyd_node = std::ptr::null_mut();
    let rc =
        unsafe { lyd_parse_data_mem(ctx, input_c.as_ptr(), LYD_FORMAT::LYD_XML, 0, 0, &mut tree) };
    assert_eq!(rc, LY_ERR::LY_SUCCESS, "parse must succeed");
    assert!(!tree.is_null());

    let mut out: *mut ::std::os::raw::c_char = std::ptr::null_mut();
    let rc = unsafe { lyd_print_mem(&mut out, tree, LYD_FORMAT::LYD_XML, LYD_PRINT_SIBLINGS) };
    assert_eq!(rc, LY_ERR::LY_SUCCESS, "print must succeed");
    assert!(!out.is_null());

    let output = unsafe {
        CStr::from_ptr(out)
            .to_str()
            .expect("XML output must be UTF-8")
            .to_string()
    };

    unsafe {
        libc::free(out as *mut _);
        lyd_free_all(tree);
        ly_ctx_destroy(ctx);
    }

    let golden = fs::read_to_string(
        fixture_dir()
            .ancestors()
            .nth(2)
            .unwrap()
            .join("golden/scrambled-children/output.xml"),
    )
    .unwrap();

    assert_eq!(
        output.trim(),
        golden.trim(),
        "scrambled input must serialize in schema declaration order"
    );
}
