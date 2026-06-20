//! Low-level, unsafe FFI to the vendored libyang engine.
//!
//! This crate is an internal adapter. The safe public API lives in
//! `cambium` / `cambium-core`.
#![allow(clippy::all)]
#![allow(non_upper_case_globals)]
#![allow(non_camel_case_types)]
#![allow(non_snake_case)]
#![allow(dead_code)]

#[cfg(docsrs)]
include!("bindings.rs");

#[cfg(not(docsrs))]
include!(concat!(env!("OUT_DIR"), "/bindings.rs"));

pub mod adapter;

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn ly_ctx_new_returns_non_null() {
        let mut ctx: *mut ly_ctx = std::ptr::null_mut();
        let rc = unsafe { ly_ctx_new(std::ptr::null(), 0, &mut ctx) };
        assert_eq!(rc, LY_ERR::LY_SUCCESS, "ly_ctx_new must return LY_SUCCESS");
        assert!(!ctx.is_null(), "ly_ctx_new must return a non-null context");
        unsafe { ly_ctx_destroy(ctx) };
    }
}
