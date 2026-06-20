//! Compile-fail assertions for the safe API surface.

#[test]
fn compile_fail_user_ordered_list() {
    let t = trybuild::TestCases::new();
    t.compile_fail("tests/compile-fail/*.rs");
}
