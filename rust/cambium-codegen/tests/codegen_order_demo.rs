//! v0 codegen acceptance: generated Rust source compiles, is deterministic,
//! and its serializer is byte-identical to libyang for the order-demo fixture.

use std::fs;
use std::path::PathBuf;
use std::process::Command;

use cambium_codegen::generate_rust;
use cambium_core::{Context, Format, ParseMode, SerializeFlags, ValidateMode};

fn fixture_dir() -> PathBuf {
    PathBuf::from(env!("CARGO_MANIFEST_DIR")).join("../../conformance/fixtures/scrambled-children")
}

fn generated_dir() -> PathBuf {
    PathBuf::from(env!("CARGO_MANIFEST_DIR")).join("../../target/generated")
}

#[test]
fn generated_rust_is_deterministic_and_matches_libyang() -> Result<(), Box<dyn std::error::Error>> {
    let mut ctx = Context::new()?;
    ctx.set_search_path(fixture_dir().join("module"))?;
    ctx.load_module("order-demo")?;

    let src = generate_rust(&ctx, "order-demo")?;
    let src2 = generate_rust(&ctx, "order-demo")?;
    assert_eq!(src, src2, "codegen output must be deterministic");

    // The generated code should contain the field-order manifest and typed structs.
    assert!(src.contains("pub const FIELD_ORDER: &[&str] = &[\"z\", \"m\", \"a\"];"));
    assert!(src.contains("pub struct OrderDemoTop {"));
    assert!(src.contains("pub z: String,"));
    assert!(src.contains("pub m: String,"));
    assert!(src.contains("pub a: String,"));

    // Compile and run the generated serializer as a standalone test binary.
    fs::create_dir_all(generated_dir())?;
    let source_path = generated_dir().join("order_demo_codegen.rs");
    let binary_path = generated_dir().join("order_demo_codegen_test");

    let expected = libyang_reference_xml()?;
    let escaped = expected
        .replace('\\', "\\\\")
        .replace('"', "\\\"")
        .replace('\n', "\\n");
    let test_wrapper = format!(
        "{}\n\
         #[test]\n\
         fn generated_serializer_matches_libyang() {{\n\
         \tlet top = OrderDemoTop {{\n\
         \t\tz: \"z1\".into(),\n\
         \t\tm: \"m1\".into(),\n\
         \t\ta: \"a1\".into(),\n\
         \t}};\n\
         \tlet od = OrderDemo {{ top }};\n\
         \tlet xml = od.to_xml();\n\
         \tassert_eq!(xml, \"{}\");\n\
         }}\n",
        src, escaped
    );
    fs::write(&source_path, test_wrapper)?;

    let rustc_status = Command::new("rustc")
        .arg("--test")
        .arg("-o")
        .arg(&binary_path)
        .arg(&source_path)
        .status()?;
    assert!(rustc_status.success(), "generated code must compile");

    let run_status = Command::new(&binary_path).status()?;
    assert!(
        run_status.success(),
        "generated serializer must match libyang bytes"
    );

    Ok(())
}

fn libyang_reference_xml() -> Result<String, Box<dyn std::error::Error>> {
    let mut ctx = Context::new()?;
    ctx.set_search_path(fixture_dir().join("module"))?;
    ctx.load_module("order-demo")?;

    let input = fs::read_to_string(fixture_dir().join("input.xml"))?;
    let mut tree = ctx.parse(Format::Xml, ParseMode::data_only(), input.as_bytes())?;
    tree.validate(ValidateMode::default())?;
    let bytes = tree.serialize(
        Format::Xml,
        SerializeFlags {
            siblings: true,
            ..Default::default()
        },
    )?;
    Ok(String::from_utf8(bytes)?)
}
