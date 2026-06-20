use std::fs;

#[test]
fn node_value_str_direct_uses_lyd_get_value() {
    let src = fs::read_to_string("src/adapter.rs").expect("read adapter.rs");
    let body = function_source(&src, "unsafe fn node_value_str_direct");
    assert!(
        body.contains("cam_lyd_get_value"),
        "node_value_str_direct must read values through lyd_get_value"
    );
    assert!(
        !body.contains("._canonical"),
        "node_value_str_direct must not read lyd_value._canonical directly"
    );
}

#[test]
fn meta_value_str_uses_lyd_get_meta_value() {
    let src = fs::read_to_string("src/adapter.rs").expect("read adapter.rs");
    let body = function_source(&src, "unsafe fn meta_value_str");
    assert!(
        body.contains("cam_lyd_get_meta_value"),
        "meta_value_str must read metadata through lyd_get_meta_value"
    );
    assert!(
        !body.contains("._canonical"),
        "meta_value_str must not read lyd_value._canonical directly"
    );
}

fn function_source<'a>(src: &'a str, signature: &str) -> &'a str {
    let start = src.find(signature).expect("function signature not found");
    let open = src[start..].find('{').expect("function body not found") + start;
    let mut depth = 0usize;
    for (offset, ch) in src[open..].char_indices() {
        match ch {
            '{' => depth += 1,
            '}' => {
                depth -= 1;
                if depth == 0 {
                    return &src[open..open + offset + 1];
                }
            }
            _ => {}
        }
    }
    panic!("function body did not terminate");
}
