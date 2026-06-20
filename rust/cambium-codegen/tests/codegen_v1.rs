//! v1 codegen acceptance: lists, list keys, leaf-lists, and typed leaves.

use std::fs;
use std::path::{Path, PathBuf};
use std::process::Command;

use cambium_codegen::generate_rust;
use cambium_core::{Context, Format, ParseMode, SerializeFlags, ValidateMode};

fn fixture_dir(name: &str) -> PathBuf {
    PathBuf::from(env!("CARGO_MANIFEST_DIR"))
        .join("../../conformance/fixtures")
        .join(name)
}

fn generated_dir() -> PathBuf {
    PathBuf::from(env!("CARGO_MANIFEST_DIR")).join("../../target/generated")
}

#[test]
fn generated_rust_list_keys_first_matches_libyang() -> Result<(), Box<dyn std::error::Error>> {
    let mut ctx = Context::new()?;
    ctx.set_search_path(fixture_dir("keys-first").join("module"))?;
    ctx.load_module("keys-first-demo")?;

    let src = generate_rust(&ctx, "keys-first-demo")?;

    // The list entry struct must place keys first.
    assert!(src.contains("pub struct KeysFirstDemoServerEntry {"));
    assert!(
        src.contains("pub const FIELD_ORDER: &[&str] = &[\"name\", \"class\", \"description\"];")
    );

    let expected = libyang_reference_xml(&fixture_dir("keys-first"))?;
    let escaped = escape_for_rust_string(&expected);
    let test_wrapper = format!(
        "{}\n\
         #[test]\n\
         fn generated_list_matches_libyang() {{\n\
         \tlet demo = KeysFirstDemo {{\n\
         \t\tserver: vec![KeysFirstDemoServerEntry {{\n\
         \t\t\tname: \"s1\".into(),\n\
         \t\t\tclass: \"c1\".into(),\n\
         \t\t\tdescription: \"main\".into(),\n\
         \t\t}}],\n\
         \t}};\n\
         \tassert_eq!(demo.to_xml(), \"{}\");\n\
         }}\n",
        src, escaped
    );

    compile_and_run(
        &test_wrapper,
        "keys_first_codegen.rs",
        "keys_first_codegen_test",
    )
}

#[test]
fn generated_rust_leaf_list_and_typed_leaves_match_libyang()
-> Result<(), Box<dyn std::error::Error>> {
    let dir = generated_dir().join("v1-demo-module");
    fs::create_dir_all(&dir)?;
    let yang_path = dir.join("v1-demo.yang");
    fs::write(
        &yang_path,
        "module v1-demo {\n\
         \tnamespace \"urn:v1-demo\";\n\
         \tprefix v1;\n\
         \trevision 2026-06-13;\n\
         \tcontainer top {\n\
         \t\tleaf flag { type boolean; }\n\
         \t\tleaf count { type int64; }\n\
         \t\tleaf-list tags { type string; }\n\
         \t}\n\
         }\n",
    )?;

    let mut ctx = Context::new()?;
    ctx.set_search_path(&dir)?;
    ctx.load_module("v1-demo")?;

    let src = generate_rust(&ctx, "v1-demo")?;

    // Typed fields and leaf-list vector.
    assert!(src.contains("pub flag: bool,"));
    assert!(src.contains("pub count: i64,"));
    assert!(src.contains("pub tags: Vec<String>,"));

    let input = "<top xmlns=\"urn:v1-demo\">\n\
                 \t<flag>true</flag>\n\
                 \t<count>42</count>\n\
                 \t<tags>alpha</tags>\n\
                 \t<tags>beta</tags>\n\
                 </top>\n";
    let expected = libyang_reference_xml_from_input(&ctx, input)?;
    let escaped = escape_for_rust_string(&expected);
    let test_wrapper = format!(
        "{}\n\
         #[test]\n\
         fn generated_typed_leaves_match_libyang() {{\n\
         \tlet demo = V1Demo {{\n\
         \t\ttop: V1DemoTop {{\n\
         \t\t\tflag: true,\n\
         \t\t\tcount: 42,\n\
         \t\t\ttags: vec![\"alpha\".into(), \"beta\".into()],\n\
         \t\t}},\n\
         \t}};\n\
         \tassert_eq!(demo.to_xml(), \"{}\");\n\
         }}\n",
        src, escaped
    );

    compile_and_run(&test_wrapper, "v1_demo_codegen.rs", "v1_demo_codegen_test")
}

fn libyang_reference_xml(dir: &Path) -> Result<String, Box<dyn std::error::Error>> {
    let mut ctx = Context::new()?;
    ctx.set_search_path(dir.join("module"))?;
    let module_name = module_name_from_dir(dir)?;
    ctx.load_module(&module_name)?;
    libyang_reference_xml_from_input(&ctx, &fs::read_to_string(dir.join("input.xml"))?)
}

fn libyang_reference_xml_from_input(
    ctx: &Context,
    input: &str,
) -> Result<String, Box<dyn std::error::Error>> {
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

fn module_name_from_dir(dir: &Path) -> Result<String, Box<dyn std::error::Error>> {
    for entry in fs::read_dir(dir.join("module"))? {
        let entry = entry?;
        let name = entry.file_name();
        if let Some(s) = name.to_str()
            && s.ends_with(".yang")
        {
            return Ok(s.trim_end_matches(".yang").to_string());
        }
    }
    Err("no .yang module file found".into())
}

fn escape_for_rust_string(s: &str) -> String {
    s.replace('\\', "\\\\")
        .replace('"', "\\\"")
        .replace('\n', "\\n")
}

fn compile_and_run(
    wrapper: &str,
    source_name: &str,
    binary_name: &str,
) -> Result<(), Box<dyn std::error::Error>> {
    fs::create_dir_all(generated_dir())?;
    let source_path = generated_dir().join(source_name);
    let binary_path = generated_dir().join(binary_name);
    fs::write(&source_path, wrapper)?;

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
