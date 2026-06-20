//! v2 codegen acceptance: rich-API rewire, `generate` surface, field-order
//! manifest, typed ints/decimal64, mandatory vs optional, determinism, and
//! generated-source clippy cleanliness.

#![allow(clippy::unwrap_used, clippy::expect_used)]

use std::fs;
use std::path::{Path, PathBuf};
use std::process::Command;

use cambium_codegen::{CodegenOpts, Lang, generate, generate_rust};
use cambium_core::{ContextBuilder, ContextFlags, Format, ParseMode, SerializeFlags, ValidateMode};

fn fixture_dir(name: &str) -> PathBuf {
    PathBuf::from(env!("CARGO_MANIFEST_DIR"))
        .join("../../conformance/fixtures")
        .join(name)
}

fn generated_dir() -> PathBuf {
    PathBuf::from(env!("CARGO_MANIFEST_DIR")).join("../../target/generated")
}

fn temp_crate_dir(name: &str) -> PathBuf {
    let dir = generated_dir().join("clippy-crates").join(name);
    fs::create_dir_all(&dir).unwrap();
    dir
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

#[test]
fn codegen_generate_opts_signature() {
    let ctx = load_ctx_for_fixture("scrambled-children");
    let module = module_name_from_dir("scrambled-children");

    let generated = generate(&ctx, &module, CodegenOpts::default()).unwrap();
    assert!(!generated.source.is_empty());
    assert!(generated.field_order.contains_key(&to_pascal_case(&module)));

    let src = generate_rust(&ctx, &module).unwrap();
    assert!(!src.is_empty());
}

#[test]
fn codegen_field_order_manifest_keys_first() {
    let ctx = load_ctx_for_fixture("keys-first");
    let module = module_name_from_dir("keys-first");

    let generated = generate(&ctx, &module, CodegenOpts::default()).unwrap();

    let entry = "KeysFirstDemoServerEntry";
    assert!(generated.source.contains(&format!("pub struct {entry} {{")));
    assert!(
        generated
            .source
            .contains("pub const FIELD_ORDER: &[&str] = &[\"name\", \"class\", \"description\"];"),
        "FIELD_ORDER must list keys first"
    );

    let manifest = generated
        .field_order
        .get(entry)
        .expect("entry manifest present");
    assert_eq!(manifest, &vec!["name", "class", "description"]);
}

#[test]
fn codegen_round_trip_bytes_equal_libyang_xml_order_demo() {
    let ctx = load_ctx_for_fixture("scrambled-children");
    let module = module_name_from_dir("scrambled-children");

    let generated = generate(&ctx, &module, CodegenOpts::default()).unwrap();
    let expected = libyang_reference_xml(&fixture_dir("scrambled-children")).unwrap();
    let escaped = escape_for_rust_string(&expected);

    let root = to_pascal_case(&module);
    let test_wrapper = format!(
        "{}\n\
         #[test]\n\
         fn generated_serializer_matches_libyang() {{\n\
         \tlet od = {root} {{\n\
         \t\ttop: {root}Top {{ z: Some(\"z1\".into()), m: Some(\"m1\".into()), a: Some(\"a1\".into()) }},\n\
         \t}};\n\
         \tassert_eq!(od.to_xml(), \"{}\");\n\
         }}\n",
        generated.source, escaped
    );

    compile_and_run(&test_wrapper, "order_demo_v2.rs", "order_demo_v2_test");
}

#[test]
fn codegen_round_trip_bytes_equal_libyang_xml_keys_first() {
    let ctx = load_ctx_for_fixture("keys-first");
    let module = module_name_from_dir("keys-first");

    let generated = generate(&ctx, &module, CodegenOpts::default()).unwrap();
    let expected = libyang_reference_xml(&fixture_dir("keys-first")).unwrap();
    let escaped = escape_for_rust_string(&expected);

    let root = to_pascal_case(&module);
    let test_wrapper = format!(
        "{}\n\
         #[test]\n\
         fn generated_list_matches_libyang() {{\n\
         \tlet demo = {root} {{\n\
         \t\tserver: vec![{root}ServerEntry {{\n\
         \t\t\tname: \"s1\".into(),\n\
         \t\t\tclass: \"c1\".into(),\n\
         \t\t\tdescription: Some(\"main\".into()),\n\
         \t\t}}],\n\
         \t}};\n\
         \tassert_eq!(demo.to_xml(), \"{}\");\n\
         }}\n",
        generated.source, escaped
    );

    compile_and_run(&test_wrapper, "keys_first_v2.rs", "keys_first_v2_test");
}

#[test]
fn codegen_mandatory_vs_optional() {
    let dir = generated_dir().join("v2-mandatory-demo");
    fs::create_dir_all(&dir).unwrap();
    let yang_path = dir.join("v2-mandatory-demo.yang");
    fs::write(
        &yang_path,
        "module v2-mandatory-demo {\n\
         \tnamespace \"urn:v2-mandatory-demo\";\n\
         \tprefix v2m;\n\
         \trevision 2026-06-13;\n\
         \tcontainer top {\n\
         \t\tleaf required { type string; mandatory true; }\n\
         \t\tleaf optional { type string; }\n\
         \t}\n\
         }\n",
    )
    .unwrap();

    let ctx = ContextBuilder::new(ContextFlags {
        no_yang_library: true,
        ..Default::default()
    })
    .unwrap()
    .search_path(&dir)
    .unwrap()
    .load_module("v2-mandatory-demo", None, &[])
    .unwrap()
    .build()
    .unwrap();

    let src = generate(&ctx, "v2-mandatory-demo", CodegenOpts::default())
        .unwrap()
        .source;

    assert!(src.contains("pub required: String,"));
    assert!(src.contains("pub optional: Option<String>,"));
}

#[test]
fn codegen_deterministic_output() {
    let ctx = load_ctx_for_fixture("scrambled-children");
    let module = module_name_from_dir("scrambled-children");

    let gen1 = generate(&ctx, &module, CodegenOpts::default()).unwrap();
    let gen2 = generate(&ctx, &module, CodegenOpts::default()).unwrap();
    assert_eq!(gen1.source, gen2.source);
    assert_eq!(gen1.field_order, gen2.field_order);
}

#[test]
fn codegen_generated_code_clippy_clean() {
    let ctx = load_ctx_for_fixture("keys-first");
    let module = module_name_from_dir("keys-first");

    let generated = generate(&ctx, &module, CodegenOpts::default()).unwrap();
    let crate_dir = temp_crate_dir("keys-first");
    let src_dir = crate_dir.join("src");
    fs::create_dir_all(&src_dir).unwrap();

    fs::write(
        crate_dir.join("Cargo.toml"),
        "[package]\n\
         name = \"keys-first-gen\"\n\
         version = \"0.0.0\"\n\
         edition = \"2024\"\n\n\
         [workspace]\n",
    )
    .unwrap();
    fs::write(src_dir.join("lib.rs"), &generated.source).unwrap();

    let status = Command::new("cargo")
        .arg("clippy")
        .arg("--manifest-path")
        .arg(crate_dir.join("Cargo.toml"))
        .arg("--")
        .arg("-D")
        .arg("warnings")
        .status()
        .expect("cargo clippy must be available");

    assert!(status.success(), "generated source must be clippy-clean");
}

/// Regression for the Phase-5 review: the generator emitted NON-COMPILING code for
/// several common shapes that no earlier fixture exercised — a module-level leaf,
/// a leaf-list (was a scalar field, not `Vec<T>`), an empty presence container
/// (unused serializer vars vs `#![deny(warnings)]`), and two sibling YANG names
/// that collapse to one PascalCase enum type (silent clobber). Generating + running
/// `cargo clippy -- -D warnings` over the result proves all of those now compile clean.
#[test]
fn codegen_hard_shapes_compile_clippy_clean() {
    let dir = generated_dir().join("v2-hard-shapes");
    fs::create_dir_all(&dir).unwrap();
    fs::write(
        dir.join("v2-hard-shapes.yang"),
        "module v2-hard-shapes {\n\
         \tnamespace \"urn:v2-hard-shapes\";\n\
         \tprefix v2h;\n\
         \trevision 2026-06-14;\n\
         \tleaf top-leaf { type string; }\n\
         \tleaf-list top-tags { type string; }\n\
         \tcontainer presence-box { presence \"empty on purpose\"; }\n\
         \tcontainer data {\n\
         \t\tleaf-list tags { type string; }\n\
         \t\tleaf ratio { type decimal64 { fraction-digits 2; } }\n\
         \t\tleaf foo-bar { type enumeration { enum a; enum b; } }\n\
         \t\tleaf foo_bar { type enumeration { enum c; enum d; } }\n\
         \t\tleaf port { type uint16 { range \"1..65535\"; } }\n\
         \t\tleaf code { type string { length \"3..8\"; } }\n\
         \t}\n\
         }\n",
    )
    .unwrap();

    let ctx = ContextBuilder::new(ContextFlags {
        no_yang_library: true,
        ..Default::default()
    })
    .unwrap()
    .search_path(&dir)
    .unwrap()
    .load_module("v2-hard-shapes", None, &[])
    .unwrap()
    .build()
    .unwrap();

    let generated = generate(&ctx, "v2-hard-shapes", CodegenOpts::default()).unwrap();
    // Leaf-lists must be Vec, not bare scalars; the two colliding enums must be
    // two distinct types (the second is suffixed from the disambiguated field ident).
    assert!(
        generated.source.contains("Vec<String>"),
        "leaf-list must be Vec"
    );
    assert!(generated.source.contains("FooBarEnum"));
    assert!(generated.source.contains("FooBar2Enum"));

    let crate_dir = temp_crate_dir("v2-hard-shapes");
    let src_dir = crate_dir.join("src");
    fs::create_dir_all(&src_dir).unwrap();
    fs::write(
        crate_dir.join("Cargo.toml"),
        "[package]\n\
         name = \"hard-shapes-gen\"\n\
         version = \"0.0.0\"\n\
         edition = \"2024\"\n\n\
         [workspace]\n",
    )
    .unwrap();
    fs::write(src_dir.join("lib.rs"), &generated.source).unwrap();

    let status = Command::new("cargo")
        .arg("clippy")
        .arg("--manifest-path")
        .arg(crate_dir.join("Cargo.toml"))
        .arg("--")
        .arg("-D")
        .arg("warnings")
        .status()
        .expect("cargo clippy must be available");
    assert!(
        status.success(),
        "generated source for hard shapes must compile + be clippy-clean"
    );
}

#[test]
fn codegen_dedup_groupings_gated() {
    let ctx = load_ctx_for_fixture("scrambled-children");
    let module = module_name_from_dir("scrambled-children");

    let opts = CodegenOpts {
        dedup_groupings: true,
        ..Default::default()
    };
    let err = generate(&ctx, &module, opts).unwrap_err();
    assert!(err.to_string().contains("dedup_groupings"));
}

#[test]
fn codegen_lang_go_gated() {
    let ctx = load_ctx_for_fixture("scrambled-children");
    let module = module_name_from_dir("scrambled-children");

    let opts = CodegenOpts {
        lang: Lang::Go,
        ..Default::default()
    };
    let err = generate(&ctx, &module, opts).unwrap_err();
    assert!(err.to_string().contains("Lang::Rust"));
}

fn libyang_reference_xml(dir: &Path) -> Result<String, Box<dyn std::error::Error>> {
    let ctx = load_ctx_for_dir(dir);
    let input = fs::read_to_string(dir.join("input.xml"))?;
    libyang_reference_xml_from_input(&ctx, &input)
}

fn load_ctx_for_dir(dir: &Path) -> cambium_core::Context {
    let module_path = dir.join("module");
    let name = fs::read_dir(&module_path)
        .unwrap()
        .filter_map(|e| e.ok())
        .filter(|e| {
            e.path()
                .extension()
                .map(|ext| ext == "yang")
                .unwrap_or(false)
        })
        .map(|e| e.path().file_stem().unwrap().to_string_lossy().to_string())
        .next()
        .expect("no .yang module file found");

    ContextBuilder::new(ContextFlags {
        no_yang_library: true,
        ..Default::default()
    })
    .unwrap()
    .search_path(&module_path)
    .unwrap()
    .load_module(&name, None, &[])
    .unwrap()
    .build()
    .unwrap()
}

fn libyang_reference_xml_from_input(
    ctx: &cambium_core::Context,
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

fn escape_for_rust_string(s: &str) -> String {
    s.replace('\\', "\\\\")
        .replace('"', "\\\"")
        .replace('\n', "\\n")
}

fn compile_and_run(wrapper: &str, source_name: &str, binary_name: &str) {
    fs::create_dir_all(generated_dir()).unwrap();
    let source_path = generated_dir().join(source_name);
    let binary_path = generated_dir().join(binary_name);
    fs::write(&source_path, wrapper).unwrap();

    let rustc_status = Command::new("rustc")
        .arg("--test")
        .arg("-o")
        .arg(&binary_path)
        .arg(&source_path)
        .status()
        .expect("rustc must be available");
    assert!(rustc_status.success(), "generated code must compile");

    let run_status = Command::new(&binary_path)
        .status()
        .expect("generated binary must run");
    assert!(
        run_status.success(),
        "generated serializer must match libyang bytes"
    );
}

/// Build a detached temp crate that links `cambium-core`, re-feed generated XML
/// bytes through `Context::parse` + `validate` + `serialize`, and asserts the
/// re-serialized bytes match the libyang oracle. This is a serializer-acceptance
/// gate, NOT a struct round-trip (deserialization into generated structs is out
/// of scope).
fn engine_routed_xml_gate(ctx: &cambium_core::Context, fixture: &str, instance: &str, src: &str) {
    let input = fs::read_to_string(fixture_dir(fixture).join("input.xml")).unwrap();
    let expected = libyang_reference_xml_from_input(ctx, &input).unwrap();
    let escaped_expected = escape_for_rust_string(&expected);

    let crate_dir = temp_crate_dir(&format!("engine-routed-{fixture}"));
    let src_dir = crate_dir.join("src");
    fs::create_dir_all(&src_dir).unwrap();

    let core_path = PathBuf::from(env!("CARGO_MANIFEST_DIR"))
        .join("../cambium-core")
        .canonicalize()
        .unwrap();
    let module_path = fixture_dir(fixture).join("module").canonicalize().unwrap();
    let module_name = module_name_from_dir(fixture);

    let cargo_toml = format!(
        "[package]\n\
         name = \"engine-routed-{fixture}\"\n\
         version = \"0.0.0\"\n\
         edition = \"2024\"\n\n\
         [workspace]\n\n\
         [dependencies]\n\
         cambium-core = {{ path = \"{}\" }}\n",
        core_path.to_str().unwrap().replace('\\', "/")
    );
    fs::write(crate_dir.join("Cargo.toml"), cargo_toml).unwrap();

    let wrapper = format!(
        "{src}\n\
         #[cfg(test)]\n\
         mod engine_routed {{\n\
         \tuse crate::*;\n\
         \tuse cambium_core::{{ContextBuilder, ContextFlags, Format, ParseMode, SerializeFlags, ValidateMode}};\n\
         \t#[test]\n\
         \tfn generated_xml_round_trips_through_engine() {{\n\
         \t\tlet demo = {instance};\n\
         \t\tlet generated = demo.to_xml();\n\
         \t\tlet ctx = ContextBuilder::new(ContextFlags {{ no_yang_library: true, ..Default::default() }})\n\
         \t\t\t.unwrap()\n\
         \t\t\t.search_path(\"{}\")\n\
         \t\t\t.unwrap()\n\
         \t\t\t.load_module(\"{}\", None, &[])\n\
         \t\t\t.unwrap()\n\
         \t\t\t.build()\n\
         \t\t\t.unwrap();\n\
         \t\tlet mut tree = ctx.parse(Format::Xml, ParseMode::data_only(), generated.as_bytes()).unwrap();\n\
         \t\ttree.validate(ValidateMode::default()).unwrap();\n\
         \t\tlet bytes = tree.serialize(Format::Xml, SerializeFlags {{ siblings: true, ..Default::default() }}).unwrap();\n\
         \t\tlet re_serialized = String::from_utf8(bytes).unwrap();\n\
         \t\tassert_eq!(re_serialized, \"{}\");\n\
         \t}}\n\
         }}\n",
        module_path.to_str().unwrap().replace('\\', "/"),
        module_name,
        escaped_expected
    );
    fs::write(src_dir.join("lib.rs"), wrapper).unwrap();

    let status = Command::new("cargo")
        .arg("test")
        .arg("--manifest-path")
        .arg(crate_dir.join("Cargo.toml"))
        .status()
        .expect("cargo must be available");
    assert!(status.success(), "engine-routed serializer gate must pass");
}

fn to_pascal_case(s: &str) -> String {
    let mut out = String::new();
    let mut upper = true;
    for c in s.chars() {
        if c == '-' || c == '_' || c == '.' {
            upper = true;
        } else if upper {
            out.extend(c.to_uppercase());
            upper = false;
        } else {
            out.push(c);
        }
    }
    if out.is_empty() {
        out.push_str("Node");
    }
    out
}

#[test]
fn codegen_enum_typed() {
    let dir = generated_dir().join("v2-enum-demo");
    fs::create_dir_all(&dir).unwrap();
    let yang_path = dir.join("v2-enum-demo.yang");
    fs::write(
        &yang_path,
        "module v2-enum-demo {\n\
         \tnamespace \"urn:v2-enum-demo\";\n\
         \tprefix v2e;\n\
         \trevision 2026-06-13;\n\
         \tcontainer top {\n\
         \t\tleaf status-enum {\n\
         \t\t\ttype enumeration {\n\
         \t\t\t\tenum up { value 1; }\n\
         \t\t\t\tenum down { value 2; }\n\
         \t\t\t\tenum unknown { value 0; }\n\
         \t\t\t}\n\
         \t\t}\n\
         \t}\n\
         }\n",
    )
    .unwrap();

    let ctx = ContextBuilder::new(ContextFlags {
        no_yang_library: true,
        ..Default::default()
    })
    .unwrap()
    .search_path(&dir)
    .unwrap()
    .load_module("v2-enum-demo", None, &[])
    .unwrap()
    .build()
    .unwrap();

    let generated = generate(&ctx, "v2-enum-demo", CodegenOpts::default()).unwrap();
    let src = &generated.source;

    // Enum is emitted with explicit sparse discriminants in declaration order.
    assert!(
        src.contains("pub enum V2EnumDemoTopStatusEnumEnum {"),
        "enum not found in source:\n{src}"
    );
    assert!(src.contains("Up = 1,"));
    assert!(src.contains("Down = 2,"));
    assert!(src.contains("Unknown = 0,"));
    assert!(src.contains("pub fn as_name(&self) -> &'static str"));
    assert!(src.contains("pub fn from_name(name: &str) -> Option<Self>"));

    let input = "<top xmlns=\"urn:v2-enum-demo\">\n\
                 \t<status-enum>up</status-enum>\n\
                 </top>\n";
    let expected = libyang_reference_xml_from_input(&ctx, input).unwrap();
    let escaped = escape_for_rust_string(&expected);

    let test_wrapper = format!(
        "{}\n\
         #[test]\n\
         fn generated_enum_matches_libyang() {{\n\
         \tlet demo = V2EnumDemo {{\n\
         \t\ttop: V2EnumDemoTop {{\n\
         \t\t\tstatus_enum: Some(V2EnumDemoTopStatusEnumEnum::Up),\n\
         \t\t}},\n\
         \t}};\n\
         \tassert_eq!(demo.to_xml(), \"{}\");\n\
         }}\n",
        src, escaped
    );

    compile_and_run(&test_wrapper, "v2_enum_demo.rs", "v2_enum_demo_test");
}

#[test]
fn codegen_reserved_word_idents() {
    let dir = generated_dir().join("v2-ident-demo");
    fs::create_dir_all(&dir).unwrap();
    let yang_path = dir.join("v2-ident-demo.yang");
    fs::write(
        &yang_path,
        "module v2-ident-demo {\n\
         \tnamespace \"urn:v2-ident-demo\";\n\
         \tprefix v2i;\n\
         \trevision 2026-06-13;\n\
         \tcontainer top {\n\
         \t\tleaf type { type string; }\n\
         \t\tleaf match { type string; }\n\
         \t}\n\
         }\n",
    )
    .unwrap();

    let ctx = ContextBuilder::new(ContextFlags {
        no_yang_library: true,
        ..Default::default()
    })
    .unwrap()
    .search_path(&dir)
    .unwrap()
    .load_module("v2-ident-demo", None, &[])
    .unwrap()
    .build()
    .unwrap();

    let generated = generate(&ctx, "v2-ident-demo", CodegenOpts::default()).unwrap();
    let src = &generated.source;

    assert!(src.contains("pub r#type: Option<String>,"));
    assert!(src.contains("pub r#match: Option<String>,"));

    let input = "<top xmlns=\"urn:v2-ident-demo\">\n\
                 \t<type>tcp</type>\n\
                 \t<match>exact</match>\n\
                 </top>\n";
    let expected = libyang_reference_xml_from_input(&ctx, input).unwrap();
    let escaped = escape_for_rust_string(&expected);

    let test_wrapper = format!(
        "{}\n\
         #[test]\n\
         fn generated_reserved_idents_match_libyang() {{\n\
         \tlet demo = V2IdentDemo {{\n\
         \t\ttop: V2IdentDemoTop {{\n\
         \t\t\tr#type: Some(\"tcp\".into()),\n\
         \t\t\tr#match: Some(\"exact\".into()),\n\
         \t\t}},\n\
         \t}};\n\
         \tassert_eq!(demo.to_xml(), \"{}\");\n\
         }}\n",
        src, escaped
    );

    compile_and_run(&test_wrapper, "v2_ident_demo.rs", "v2_ident_demo_test");
}

#[test]
fn codegen_ident_collision_disambiguated() {
    let dir = generated_dir().join("v2-collision-demo");
    fs::create_dir_all(&dir).unwrap();
    let yang_path = dir.join("v2-collision-demo.yang");
    fs::write(
        &yang_path,
        "module v2-collision-demo {\n\
         \tnamespace \"urn:v2-collision-demo\";\n\
         \tprefix v2c;\n\
         \trevision 2026-06-13;\n\
         \tcontainer top {\n\
         \t\tleaf foo-bar { type string; }\n\
         \t\tleaf foo_bar { type string; }\n\
         \t}\n\
         }\n",
    )
    .unwrap();

    let ctx = ContextBuilder::new(ContextFlags {
        no_yang_library: true,
        ..Default::default()
    })
    .unwrap()
    .search_path(&dir)
    .unwrap()
    .load_module("v2-collision-demo", None, &[])
    .unwrap()
    .build()
    .unwrap();

    let generated = generate(&ctx, "v2-collision-demo", CodegenOpts::default()).unwrap();
    let src = &generated.source;

    assert!(src.contains("pub foo_bar: Option<String>,"));
    assert!(src.contains("pub foo_bar_2: Option<String>,"));
}

#[test]
fn codegen_enum_clippy_clean() {
    let dir = generated_dir().join("v2-enum-clippy");
    fs::create_dir_all(&dir).unwrap();
    let yang_path = dir.join("v2-enum-clippy.yang");
    fs::write(
        &yang_path,
        "module v2-enum-clippy {\n\
         \tnamespace \"urn:v2-enum-clippy\";\n\
         \tprefix v2ec;\n\
         \trevision 2026-06-13;\n\
         \tcontainer top {\n\
         \t\tleaf status {\n\
         \t\t\ttype enumeration {\n\
         \t\t\t\tenum up { value 1; }\n\
         \t\t\t\tenum down { value 0; }\n\
         \t\t\t}\n\
         \t\t}\n\
         \t}\n\
         }\n",
    )
    .unwrap();

    let ctx = ContextBuilder::new(ContextFlags {
        no_yang_library: true,
        ..Default::default()
    })
    .unwrap()
    .search_path(&dir)
    .unwrap()
    .load_module("v2-enum-clippy", None, &[])
    .unwrap()
    .build()
    .unwrap();

    let generated = generate(&ctx, "v2-enum-clippy", CodegenOpts::default()).unwrap();
    let crate_dir = temp_crate_dir("enum-clippy");
    let src_dir = crate_dir.join("src");
    fs::create_dir_all(&src_dir).unwrap();

    fs::write(
        crate_dir.join("Cargo.toml"),
        "[package]\n\
         name = \"enum-clippy\"\n\
         version = \"0.0.0\"\n\
         edition = \"2024\"\n\n\
         [workspace]\n",
    )
    .unwrap();
    fs::write(src_dir.join("lib.rs"), &generated.source).unwrap();

    let status = Command::new("cargo")
        .arg("clippy")
        .arg("--manifest-path")
        .arg(crate_dir.join("Cargo.toml"))
        .arg("--")
        .arg("-D")
        .arg("warnings")
        .status()
        .expect("cargo clippy must be available");

    assert!(
        status.success(),
        "generated enum source must be clippy-clean"
    );
}

fn libyang_reference_json_from_input(
    ctx: &cambium_core::Context,
    input: &str,
) -> Result<String, Box<dyn std::error::Error>> {
    let mut tree = ctx.parse(Format::Xml, ParseMode::data_only(), input.as_bytes())?;
    tree.validate(ValidateMode::default())?;
    let bytes = tree.serialize(
        Format::Json,
        SerializeFlags {
            siblings: true,
            ..Default::default()
        },
    )?;
    Ok(String::from_utf8(bytes)?)
}

#[test]
fn codegen_round_trip_bytes_equal_libyang_json_order_demo() {
    let ctx = load_ctx_for_fixture("scrambled-children");
    let module = module_name_from_dir("scrambled-children");

    let generated = generate(&ctx, &module, CodegenOpts::default()).unwrap();
    let input = "<top xmlns=\"urn:order-demo\">\n\
                 \t<a>a1</a>\n\
                 \t<z>z1</z>\n\
                 \t<m>m1</m>\n\
                 </top>\n";
    let expected = libyang_reference_json_from_input(&ctx, input).unwrap();
    let escaped = escape_for_rust_string(&expected);

    let root = to_pascal_case(&module);
    let test_wrapper = format!(
        "{}\n\
         #[test]\n\
         fn generated_json_matches_libyang() {{\n\
         \tlet od = {root} {{\n\
         \t\ttop: {root}Top {{\n\
         \t\t\tz: Some(\"z1\".into()),\n\
         \t\t\tm: Some(\"m1\".into()),\n\
         \t\t\ta: Some(\"a1\".into()),\n\
         \t\t}},\n\
         \t}};\n\
         \tassert_eq!(od.to_json_ietf(), \"{}\");\n\
         }}\n",
        generated.source, escaped
    );

    compile_and_run(&test_wrapper, "order_demo_json.rs", "order_demo_json_test");
}

#[test]
fn codegen_round_trip_bytes_equal_libyang_json_keys_first() {
    let ctx = load_ctx_for_fixture("keys-first");
    let module = module_name_from_dir("keys-first");

    let generated = generate(&ctx, &module, CodegenOpts::default()).unwrap();
    let input = "<server xmlns=\"urn:keys-first-demo\">\n\
                 \t<description>main</description>\n\
                 \t<name>s1</name>\n\
                 \t<class>c1</class>\n\
                 </server>\n";
    let expected = libyang_reference_json_from_input(&ctx, input).unwrap();
    let escaped = escape_for_rust_string(&expected);

    let root = to_pascal_case(&module);
    let test_wrapper = format!(
        "{}\n\
         #[test]\n\
         fn generated_json_list_matches_libyang() {{\n\
         \tlet demo = {root} {{\n\
         \t\tserver: vec![{root}ServerEntry {{\n\
         \t\t\tname: \"s1\".into(),\n\
         \t\t\tclass: \"c1\".into(),\n\
         \t\t\tdescription: Some(\"main\".into()),\n\
         \t\t}}],\n\
         \t}};\n\
         \tassert_eq!(demo.to_json_ietf(), \"{}\");\n\
         }}\n",
        generated.source, escaped
    );

    compile_and_run(&test_wrapper, "keys_first_json.rs", "keys_first_json_test");
}

#[test]
fn codegen_jsonietf_scalar_quoting() {
    let dir = generated_dir().join("v2-json-scalar-demo");
    fs::create_dir_all(&dir).unwrap();
    let yang_path = dir.join("v2-json-scalar-demo.yang");
    fs::write(
        &yang_path,
        "module v2-json-scalar-demo {\n\
         \tnamespace \"urn:v2-json-scalar-demo\";\n\
         \tprefix v2js;\n\
         \trevision 2026-06-13;\n\
         \tcontainer top {\n\
         \t\tleaf big { type int64; }\n\
         \t\tleaf small { type int32; }\n\
         \t\tleaf flag { type boolean; }\n\
         \t}\n\
         }\n",
    )
    .unwrap();

    let ctx = ContextBuilder::new(ContextFlags {
        no_yang_library: true,
        ..Default::default()
    })
    .unwrap()
    .search_path(&dir)
    .unwrap()
    .load_module("v2-json-scalar-demo", None, &[])
    .unwrap()
    .build()
    .unwrap();

    let generated = generate(&ctx, "v2-json-scalar-demo", CodegenOpts::default()).unwrap();
    let input = "<top xmlns=\"urn:v2-json-scalar-demo\">\n\
                 \t<big>9223372036854775807</big>\n\
                 \t<small>-42</small>\n\
                 \t<flag>true</flag>\n\
                 </top>\n";
    let expected = libyang_reference_json_from_input(&ctx, input).unwrap();
    let escaped = escape_for_rust_string(&expected);

    let test_wrapper = format!(
        "{}\n\
         #[test]\n\
         fn generated_json_scalar_quoting_matches_libyang() {{\n\
         \tlet demo = V2JsonScalarDemo {{\n\
         \t\ttop: V2JsonScalarDemoTop {{\n\
         \t\t\tbig: Some(V2JsonScalarDemoTopBigRange::new(9223372036854775807i128).unwrap()),\n\
         \t\t\tsmall: Some(V2JsonScalarDemoTopSmallRange::new(-42).unwrap()),\n\
         \t\t\tflag: Some(true),\n\
         \t\t}},\n\
         \t}};\n\
         \tassert_eq!(demo.to_json_ietf(), \"{}\");\n\
         }}\n",
        generated.source, escaped
    );

    compile_and_run(
        &test_wrapper,
        "v2_json_scalar_demo.rs",
        "v2_json_scalar_demo_test",
    );
}

/// String leaves must be JSON-escaped exactly as libyang's `json_print_string`:
/// named escapes only for `"` `\` `\r` `\t`; every other control char (e.g. a
/// newline becomes the six bytes backslash-u-0-0-0-A) is `\uXXXX`; multi-byte
/// UTF-8 passes through raw. The old emitter used Rust `{:?}` (Debug), which
/// renders the newline as `\n` — a byte mismatch with the libyang oracle — so
/// this gate fails on the pre-fix code.
#[test]
fn codegen_jsonietf_string_escaping_byte_gate() {
    let fixture = "json-ietf-string-escaping-control-unicode";
    let ctx = load_ctx_for_fixture(fixture);
    let module = module_name_from_dir(fixture);
    let generated = generate(&ctx, &module, CodegenOpts::default()).unwrap();

    let input = fs::read_to_string(fixture_dir(fixture).join("input.xml")).unwrap();
    let expected = libyang_reference_json_from_input(&ctx, &input).unwrap();
    let escaped = escape_for_rust_string(&expected);

    let test_wrapper = format!(
        "{src}\n\
         #[test]\n\
         fn generated_json_escaping_matches_libyang() {{\n\
         \tlet demo = JsonIetfStringEscapingControlUnicode {{\n\
         \t\ttop: JsonIetfStringEscapingControlUnicodeTop {{\n\
         \t\t\twith_control: Some(\"line1\\nline2\\tend\".into()),\n\
         \t\t\twith_quote: Some(\"say \\\"hello\\\"\".into()),\n\
         \t\t\twith_backslash: Some(\"C:\\\\path\".into()),\n\
         \t\t\tfrench: Some(\"café\".into()),\n\
         \t\t\temoji: Some(\"🚀\".into()),\n\
         \t\t\tgreek: Some(\"αβγ\".into()),\n\
         \t\t}},\n\
         \t}};\n\
         \tassert_eq!(demo.to_json_ietf(), \"{escaped}\");\n\
         }}\n",
        src = generated.source,
        escaped = escaped,
    );

    compile_and_run(
        &test_wrapper,
        "json_escaping_byte_gate.rs",
        "json_escaping_byte_gate_test",
    );
}

#[test]
fn codegen_user_ordered_vec_typed_and_xml_byte_gate() {
    let ctx = load_ctx_for_fixture("ordering-nested-user-cascading");
    let module = module_name_from_dir("ordering-nested-user-cascading");

    let generated = generate(&ctx, &module, CodegenOpts::default()).unwrap();
    let src = &generated.source;

    // Field-type assertion: user-ordered list and leaf-list must NOT fall back to Vec<T>.
    assert!(
        src.contains("pub struct UserOrderedVec<T>"),
        "UserOrderedVec helper missing"
    );
    assert!(
        src.contains(
            "pub statement: UserOrderedVec<OrderingNestedUserCascadingTopStatementEntry>,"
        ),
        "user-ordered list field must be UserOrderedVec, got:\n{src}"
    );
    assert!(
        src.contains("pub actions: UserOrderedVec<String>,"),
        "user-ordered leaf-list field must be UserOrderedVec, got:\n{src}"
    );

    let expected = libyang_reference_xml(&fixture_dir("ordering-nested-user-cascading")).unwrap();
    let escaped = escape_for_rust_string(&expected);

    let test_wrapper = format!(
        "{}\n\
         #[test]\n\
         fn generated_user_ordered_matches_libyang_xml() {{\n\
         \tlet demo = OrderingNestedUserCascading {{\n\
         \t\ttop: OrderingNestedUserCascadingTop {{\n\
         \t\t\tstatement: UserOrderedVec::from(vec![\n\
         \t\t\t\tOrderingNestedUserCascadingTopStatementEntry {{\n\
         \t\t\t\t\tname: \"s2\".into(),\n\
         \t\t\t\t\tactions: UserOrderedVec::from(vec![\"b\".into(), \"a\".into()]),\n\
         \t\t\t\t}},\n\
         \t\t\t\tOrderingNestedUserCascadingTopStatementEntry {{\n\
         \t\t\t\t\tname: \"s1\".into(),\n\
         \t\t\t\t\tactions: UserOrderedVec::from(vec![\"b\".into(), \"a\".into()]),\n\
         \t\t\t\t}},\n\
         \t\t\t]),\n\
         \t\t}},\n\
         \t}};\n\
         \tassert_eq!(demo.to_xml(), \"{}\");\n\
         }}\n",
        src, escaped
    );

    compile_and_run(
        &test_wrapper,
        "user_ordered_xml.rs",
        "user_ordered_xml_test",
    );
}

#[test]
fn codegen_user_ordered_vec_json_byte_gate() {
    let ctx = load_ctx_for_fixture("ordering-nested-user-cascading");
    let module = module_name_from_dir("ordering-nested-user-cascading");

    let generated = generate(&ctx, &module, CodegenOpts::default()).unwrap();
    let src = &generated.source;

    let input = fs::read_to_string(fixture_dir("ordering-nested-user-cascading").join("input.xml"))
        .unwrap();
    let expected = libyang_reference_json_from_input(&ctx, &input).unwrap();
    let escaped = escape_for_rust_string(&expected);

    let test_wrapper = format!(
        "{}\n\
         #[test]\n\
         fn generated_user_ordered_matches_libyang_json() {{\n\
         \tlet demo = OrderingNestedUserCascading {{\n\
         \t\ttop: OrderingNestedUserCascadingTop {{\n\
         \t\t\tstatement: UserOrderedVec::from(vec![\n\
         \t\t\t\tOrderingNestedUserCascadingTopStatementEntry {{\n\
         \t\t\t\t\tname: \"s2\".into(),\n\
         \t\t\t\t\tactions: UserOrderedVec::from(vec![\"b\".into(), \"a\".into()]),\n\
         \t\t\t\t}},\n\
         \t\t\t\tOrderingNestedUserCascadingTopStatementEntry {{\n\
         \t\t\t\t\tname: \"s1\".into(),\n\
         \t\t\t\t\tactions: UserOrderedVec::from(vec![\"b\".into(), \"a\".into()]),\n\
         \t\t\t\t}},\n\
         \t\t\t]),\n\
         \t\t}},\n\
         \t}};\n\
         \tassert_eq!(demo.to_json_ietf(), \"{}\");\n\
         }}\n",
        src, escaped
    );

    compile_and_run(
        &test_wrapper,
        "user_ordered_json.rs",
        "user_ordered_json_test",
    );
}

#[test]
fn codegen_user_ordered_vec_positive_control() {
    let ctx = load_ctx_for_fixture("ordering-nested-user-cascading");
    let module = module_name_from_dir("ordering-nested-user-cascading");
    let generated = generate(&ctx, &module, CodegenOpts::default()).unwrap();

    let test_wrapper = format!(
        "{}\n\
         #[test]\n\
         fn user_ordered_vec_positional_methods_work() {{\n\
         \tlet mut v = UserOrderedVec::new();\n\
         \tv.insert_last(\"a\".to_string());\n\
         \tv.insert_last(\"c\".to_string());\n\
         \tv.insert_before(1, \"b\".to_string());\n\
         \tv.insert_after(2, \"d\".to_string());\n\
         \tassert_eq!(v.iter().collect::<Vec<_>>(), vec![\"a\", \"b\", \"c\", \"d\"]);\n\
         \tv.move_before(3, 1);\n\
         \tassert_eq!(v.iter().collect::<Vec<_>>(), vec![\"a\", \"d\", \"b\", \"c\"]);\n\
         \tv.move_after(0, 2);\n\
         \tassert_eq!(v.iter().collect::<Vec<_>>(), vec![\"d\", \"b\", \"a\", \"c\"]);\n\
         \tassert_eq!(v.remove(2), \"a\");\n\
         \tassert_eq!(v.iter().collect::<Vec<_>>(), vec![\"d\", \"b\", \"c\"]);\n\
         \tassert_eq!(v.len(), 3);\n\
         \tassert!(!v.is_empty());\n\
         \tassert_eq!(v.get(1), Some(&\"b\".to_string()));\n\
         }}\n",
        generated.source
    );

    compile_and_run(
        &test_wrapper,
        "user_ordered_positive.rs",
        "user_ordered_positive_test",
    );
}

fn byte_gate_fixture_xml_json(fixture: &str, instance_expr: &str, src: &str) {
    let dir = fixture_dir(fixture);
    let input = fs::read_to_string(dir.join("input.xml")).unwrap();
    let ctx = load_ctx_for_dir(&dir);
    let expected_xml = libyang_reference_xml_from_input(&ctx, &input).unwrap();
    let expected_json = libyang_reference_json_from_input(&ctx, &input).unwrap();

    let escaped_xml = escape_for_rust_string(&expected_xml);
    let xml_wrapper = format!(
        "{}\n\
         #[test]\n\
         fn generated_xml_matches_libyang() {{\n\
         \tassert_eq!({}.to_xml(), \"{}\");\n\
         }}\n",
        src, instance_expr, escaped_xml
    );
    compile_and_run(
        &xml_wrapper,
        &format!("{fixture}_xml.rs"),
        &format!("{fixture}_xml_test"),
    );

    let escaped_json = escape_for_rust_string(&expected_json);
    let json_wrapper = format!(
        "{}\n\
         #[test]\n\
         fn generated_json_matches_libyang() {{\n\
         \tassert_eq!({}.to_json_ietf(), \"{}\");\n\
         }}\n",
        src, instance_expr, escaped_json
    );
    compile_and_run(
        &json_wrapper,
        &format!("{fixture}_json.rs"),
        &format!("{fixture}_json_test"),
    );
}

#[test]
fn codegen_decimal64_fraction1_range() {
    let fixture = "types-decimal64-fraction1-range";
    let ctx = load_ctx_for_fixture(fixture);
    let module = module_name_from_dir(fixture);
    let generated = generate(&ctx, &module, CodegenOpts::default()).unwrap();
    let src = &generated.source;

    assert!(
        src.contains("pub struct Decimal64"),
        "Decimal64 helper missing"
    );
    assert!(
        src.contains("pub temp: Option<Decimal64>,"),
        "decimal64 leaf must be typed Decimal64, not String:\n{src}"
    );

    let root = to_pascal_case(&module);
    byte_gate_fixture_xml_json(
        fixture,
        &format!("{root} {{ temp: Some(Decimal64::new(-5, 1)) }}"),
        src,
    );
}

#[test]
fn codegen_decimal64_fraction2_canonical_round() {
    let fixture = "types-decimal64-fraction2-canonical-round";
    let ctx = load_ctx_for_fixture(fixture);
    let module = module_name_from_dir(fixture);
    let generated = generate(&ctx, &module, CodegenOpts::default()).unwrap();
    let src = &generated.source;

    assert!(src.contains("pub struct Decimal64"));
    assert!(
        src.contains("pub rate: Option<Decimal64>,"),
        "decimal64 leaf must be typed Decimal64:\n{src}"
    );

    let root = to_pascal_case(&module);
    byte_gate_fixture_xml_json(
        fixture,
        &format!("{root} {{ rate: Some(Decimal64::new(314, 2)) }}"),
        src,
    );
}

#[test]
fn codegen_decimal64_fraction3_and_6() {
    let fixture = "types-decimal64-fraction3-and-6";
    let ctx = load_ctx_for_fixture(fixture);
    let module = module_name_from_dir(fixture);
    let generated = generate(&ctx, &module, CodegenOpts::default()).unwrap();
    let src = &generated.source;

    assert!(src.contains("pub struct Decimal64"));
    assert!(src.contains("pub milli: Option<Decimal64>,"));
    assert!(src.contains("pub micro: Option<Decimal64>,"));

    let root = to_pascal_case(&module);
    let top = format!("{root}Top");
    byte_gate_fixture_xml_json(
        fixture,
        &format!(
            "{root} {{ top: {top} {{ milli: Some(Decimal64::new(1500, 3)), micro: Some(Decimal64::new(1, 6)) }} }}"
        ),
        src,
    );
}

#[test]
fn codegen_decimal64_fraction9_negative() {
    let fixture = "types-decimal64-fraction9-negative";
    let ctx = load_ctx_for_fixture(fixture);
    let module = module_name_from_dir(fixture);
    let generated = generate(&ctx, &module, CodegenOpts::default()).unwrap();
    let src = &generated.source;

    assert!(src.contains("pub struct Decimal64"));
    assert!(src.contains("pub delay: Option<Decimal64>,"));

    let root = to_pascal_case(&module);
    byte_gate_fixture_xml_json(
        fixture,
        &format!("{root} {{ delay: Some(Decimal64::new(-1, 9)) }}"),
        src,
    );
}

#[test]
fn codegen_decimal64_fraction18_max_magnitude() {
    let fixture = "types-decimal64-fraction18-max-magnitude";
    let ctx = load_ctx_for_fixture(fixture);
    let module = module_name_from_dir(fixture);
    let generated = generate(&ctx, &module, CodegenOpts::default()).unwrap();
    let src = &generated.source;

    assert!(src.contains("pub struct Decimal64"));
    assert!(src.contains("pub precise: Option<Decimal64>,"));

    let root = to_pascal_case(&module);
    byte_gate_fixture_xml_json(
        fixture,
        &format!("{root} {{ precise: Some(Decimal64::new(9223372036854775807i64, 18)) }}"),
        src,
    );
}

#[test]
fn codegen_bits_explicit_positions_gaps() {
    let fixture = "types-bits-explicit-positions-gaps";
    let ctx = load_ctx_for_fixture(fixture);
    let module = module_name_from_dir(fixture);
    let generated = generate(&ctx, &module, CodegenOpts::default()).unwrap();
    let src = &generated.source;

    let bits_type = "TypesBitsExplicitPositionsGapsOpsBits";
    assert!(
        src.contains(&format!("pub struct {bits_type}")),
        "bits newtype must be emitted:\n{src}"
    );
    assert!(
        src.contains(&format!("pub ops: Option<{bits_type}>,")),
        "bits leaf must be typed with the newtype, not String:\n{src}"
    );

    let root = to_pascal_case(&module);
    let instance =
        format!("{root} {{ ops: Some({bits_type}::new(&[\"read\", \"delete\"]).unwrap()) }}");
    byte_gate_fixture_xml_json(fixture, &instance, src);
}

#[test]
fn codegen_bits_canonical_order_reorders_by_position() {
    let fixture = "types-bits-explicit-positions-gaps";
    let ctx = load_ctx_for_fixture(fixture);
    let module = module_name_from_dir(fixture);
    let generated = generate(&ctx, &module, CodegenOpts::default()).unwrap();

    let bits_type = "TypesBitsExplicitPositionsGapsOpsBits";
    let test_wrapper = format!(
        "{}\n\
         #[test]\n\
         fn bits_reorders_by_ascending_position() {{\n\
         \tlet bits = {bits_type}::new(&[\"delete\", \"read\"]).unwrap();\n\
         \tassert_eq!(bits.to_string(), \"read delete\");\n\
         }}\n",
        generated.source
    );
    compile_and_run(&test_wrapper, "bits_reorder.rs", "bits_reorder_test");
}

#[test]
fn codegen_identityref_single_base() {
    let fixture = "types-identityref-single-base";
    let ctx = load_ctx_for_fixture(fixture);
    let module = module_name_from_dir(fixture);
    let generated = generate(&ctx, &module, CodegenOpts::default()).unwrap();
    let src = &generated.source;

    let enum_type = "TypesIdentityrefSingleBaseProtoEnum";
    assert!(
        src.contains(&format!("pub enum {enum_type}")),
        "identityref enum missing:\n{src}"
    );
    assert!(
        src.contains(&format!("pub proto: Option<{enum_type}>,")),
        "identityref leaf must be typed enum:\n{src}"
    );

    let root = to_pascal_case(&module);
    let instance = format!("{root} {{ proto: Some({enum_type}::Tcp) }}");
    byte_gate_fixture_xml_json(fixture, &instance, src);
}

#[test]
fn codegen_identityref_multiple_bases() {
    let fixture = "types-identityref-multiple-bases";
    let ctx = load_ctx_for_fixture(fixture);
    let module = module_name_from_dir(fixture);
    let generated = generate(&ctx, &module, CodegenOpts::default()).unwrap();
    let src = &generated.source;

    let enum_type = "TypesIdentityrefMultipleBasesClassEnum";
    assert!(src.contains(&format!("pub enum {enum_type}")));
    assert!(src.contains(&format!("pub class: Option<{enum_type}>,")));

    let root = to_pascal_case(&module);
    let instance = format!("{root} {{ class: Some({enum_type}::Ethernet) }}");
    byte_gate_fixture_xml_json(fixture, &instance, src);
}

#[test]
fn codegen_identityref_derived_hierarchy() {
    let fixture = "types-identityref-derived-hierarchy";
    let ctx = load_ctx_for_fixture(fixture);
    let module = module_name_from_dir(fixture);
    let generated = generate(&ctx, &module, CodegenOpts::default()).unwrap();
    let src = &generated.source;

    let enum_type = "TypesIdentityrefDerivedHierarchyItemEnum";
    assert!(src.contains(&format!("pub enum {enum_type}")));
    assert!(src.contains(&format!("pub item: Option<{enum_type}>,")));

    let root = to_pascal_case(&module);
    let instance = format!("{root} {{ item: Some({enum_type}::Fpc) }}");
    byte_gate_fixture_xml_json(fixture, &instance, src);
}

/// Regression: a base module imported solely for its identities has no data
/// nodes, so the schema forest skipped it and the foreign identityref resolved
/// to zero bases -> an empty, non-compiling enum (`#[derive(Default)]` on an
/// empty enum is E0665). The base module's identities must register so the enum
/// carries the real foreign-derived variants and serializes module-qualified.
#[test]
fn codegen_identityref_foreign_module_resolves() {
    let fixture = "types-identityref-foreign-module-prefix";
    let main = "types-identityref-foreign-module-prefix";
    let dir = fixture_dir(fixture).join("module");
    // Load every module in the fixture (the realistic codegen setup: a consumer
    // implements its full module set, base modules included) so the imported
    // identity module is compiled and its identities are visible.
    let mut module_names: Vec<String> = fs::read_dir(&dir)
        .unwrap()
        .filter_map(|e| e.ok())
        .map(|e| e.path())
        .filter(|p| p.extension().map(|x| x == "yang").unwrap_or(false))
        .map(|p| p.file_stem().unwrap().to_string_lossy().to_string())
        .collect();
    module_names.sort();
    let mut builder = ContextBuilder::new(ContextFlags {
        no_yang_library: true,
        ..Default::default()
    })
    .unwrap()
    .search_path(&dir)
    .unwrap();
    for name in &module_names {
        builder = builder.load_module(name, None, &[]).unwrap();
    }
    let ctx = builder.build().unwrap();

    let generated = generate(&ctx, main, CodegenOpts::default()).unwrap();
    let src = &generated.source;

    let enum_ty = "TypesIdentityrefForeignModulePrefixComponentEnum";
    assert!(
        src.contains(&format!("pub enum {enum_ty} {{")),
        "identityref enum missing:\n{src}"
    );
    assert!(
        src.contains("\tCpu,"),
        "foreign-derived identity `cpu` must be an enum variant, not an empty enum:\n{src}"
    );

    // JSON_IETF byte-gate: a foreign identity serializes module-qualified
    // (`types-identityref-foreign-base:cpu`).
    let input = fs::read_to_string(fixture_dir(fixture).join("input.xml")).unwrap();
    let expected_json = libyang_reference_json_from_input(&ctx, &input).unwrap();
    let escaped_json = escape_for_rust_string(&expected_json);

    let json_wrapper = format!(
        "{src}\n\
         #[test]\n\
         fn generated_foreign_identityref_json_matches_libyang() {{\n\
         \tlet demo = TypesIdentityrefForeignModulePrefix {{\n\
         \t\tcomponent: Some({enum_ty}::Cpu),\n\
         \t}};\n\
         \tassert_eq!(demo.to_json_ietf(), \"{escaped}\");\n\
         }}\n",
        src = src,
        enum_ty = enum_ty,
        escaped = escaped_json,
    );
    compile_and_run(
        &json_wrapper,
        "foreign_identityref.rs",
        "foreign_identityref_test",
    );

    // XML byte-gate: a foreign identity value synthesizes the defining module's
    // own prefix on the value and adds a matching xmlns: declaration on the
    // carrying element.
    let expected_xml = libyang_reference_xml_from_input(&ctx, &input).unwrap();
    let escaped_xml = escape_for_rust_string(&expected_xml);

    let xml_wrapper = format!(
        "{src}\n\
         #[test]\n\
         fn generated_foreign_identityref_xml_matches_libyang() {{\n\
         \tlet demo = TypesIdentityrefForeignModulePrefix {{\n\
         \t\tcomponent: Some({enum_ty}::Cpu),\n\
         \t}};\n\
         \tassert_eq!(demo.to_xml(), \"{escaped}\");\n\
         }}\n",
        src = src,
        enum_ty = enum_ty,
        escaped = escaped_xml,
    );
    compile_and_run(
        &xml_wrapper,
        "foreign_identityref_xml.rs",
        "foreign_identityref_xml_test",
    );
}

/// Defense-in-depth for the same bug from the other side: when only the
/// consuming module is loaded, libyang auto-loads the imported base module but
/// does not implement it. The schema forest keeps it for prefix resolution, but
/// the identityref value is not data-valid, so codegen must degrade to String
/// rather than emit a typed enum for non-implemented-module identities.
#[test]
fn codegen_identityref_unimplemented_import_falls_back_to_string() {
    let fixture = "types-identityref-foreign-module-prefix";
    let main = "types-identityref-foreign-module-prefix";
    let dir = fixture_dir(fixture).join("module");
    // Load ONLY the consuming module; libyang auto-loads the imported base but
    // keeps it non-implemented.
    let ctx = ContextBuilder::new(ContextFlags {
        no_yang_library: true,
        ..Default::default()
    })
    .unwrap()
    .search_path(&dir)
    .unwrap()
    .load_module(main, None, &[])
    .unwrap()
    .build()
    .unwrap();

    let generated = generate(&ctx, main, CodegenOpts::default()).unwrap();
    let src = &generated.source;

    assert!(
        src.contains("pub component: Option<String>,"),
        "identityref from non-implemented import must fall back to String, got:\n{src}"
    );
    assert!(
        !src.contains("ComponentEnum"),
        "no identityref enum should be emitted for non-implemented-module identities:\n{src}"
    );

    let test_wrapper = format!(
        "{src}\n\
         #[test]\n\
         fn generated_identityref_string_fallback_serializes() {{\n\
         \tlet demo = TypesIdentityrefForeignModulePrefix {{\n\
         \t\tcomponent: Some(\"types-identityref-foreign-base:cpu\".into()),\n\
         \t}};\n\
         \tlet json = demo.to_json_ietf();\n\
         \tassert!(\n\
         \t\tjson.contains(\"\\\"types-identityref-foreign-module-prefix:component\\\": \\\"types-identityref-foreign-base:cpu\\\"\"),\n\
         \t\t\"unexpected json: {{}}\", json\n\
         \t);\n\
         }}\n",
        src = src,
    );
    compile_and_run(
        &test_wrapper,
        "identityref_string_fallback.rs",
        "identityref_string_fallback_test",
    );
}

#[test]
fn codegen_int_range_newtype() {
    let fixture = "types-int-int8-range";
    let ctx = load_ctx_for_fixture(fixture);
    let module = module_name_from_dir(fixture);
    let generated = generate(&ctx, &module, CodegenOpts::default()).unwrap();
    let src = &generated.source;

    let min_type = "TypesIntInt8RangeTopI8MinRange";
    assert!(
        src.contains(&format!("pub struct {min_type}")),
        "ranged int newtype missing:\n{src}"
    );
    assert!(
        src.contains(&format!("pub i8_min: Option<{min_type}>,")),
        "ranged int leaf must be typed newtype:\n{src}"
    );

    let root = to_pascal_case(&module);
    let top = format!("{root}Top");
    let instance = format!(
        "{root} {{ top: {top} {{ \
        i8_min: Some({min_type}::new(-128).unwrap()), \
        i8_zero: Some({top}I8ZeroRange::new(0).unwrap()), \
        i8_max: Some({top}I8MaxRange::new(127).unwrap()) }} }}"
    );
    byte_gate_fixture_xml_json(fixture, &instance, src);
}

#[test]
fn codegen_int_range_constructor_rejects_out_of_bounds() {
    let fixture = "types-int-int8-range";
    let ctx = load_ctx_for_fixture(fixture);
    let module = module_name_from_dir(fixture);
    let generated = generate(&ctx, &module, CodegenOpts::default()).unwrap();

    let test_wrapper = format!(
        "{}\n\
         #[test]\n\
         fn int_range_bounds_enforced() {{\n\
         \tassert!(TypesIntInt8RangeTopI8MinRange::new(-129).is_err());\n\
         \tassert!(TypesIntInt8RangeTopI8MinRange::new(128).is_err());\n\
         \tassert!(TypesIntInt8RangeTopI8MinRange::new(-128).is_ok());\n\
         \tassert!(TypesIntInt8RangeTopI8MinRange::new(127).is_ok());\n\
         }}\n",
        generated.source
    );
    compile_and_run(
        &test_wrapper,
        "int_range_bounds.rs",
        "int_range_bounds_test",
    );
}

#[test]
fn codegen_string_length_newtype() {
    let fixture = "types-string-length-pattern-anchor-posix";
    let ctx = load_ctx_for_fixture(fixture);
    let module = module_name_from_dir(fixture);
    let generated = generate(&ctx, &module, CodegenOpts::default()).unwrap();
    let src = &generated.source;

    let len_type = "TypesStringLengthPatternAnchorPosixCodeLength";
    assert!(
        src.contains(&format!("pub struct {len_type}")),
        "string length newtype missing:\n{src}"
    );
    assert!(
        src.contains(&format!("pub code: Option<{len_type}>,")),
        "constrained string leaf must be typed newtype:\n{src}"
    );

    let root = to_pascal_case(&module);
    let instance = format!("{root} {{ code: Some({len_type}::new(\"abc_123\".into()).unwrap()) }}");
    byte_gate_fixture_xml_json(fixture, &instance, src);
}

#[test]
fn codegen_string_length_constructor_rejects_out_of_bounds() {
    let fixture = "types-string-length-pattern-anchor-posix";
    let ctx = load_ctx_for_fixture(fixture);
    let module = module_name_from_dir(fixture);
    let generated = generate(&ctx, &module, CodegenOpts::default()).unwrap();

    let len_type = "TypesStringLengthPatternAnchorPosixCodeLength";
    let test_wrapper = format!(
        "{}\n\
         #[test]\n\
         fn string_length_bounds_enforced() {{\n\
         \tassert!({len_type}::new(\"\".into()).is_err());\n\
         \tassert!({len_type}::new(\"a\".repeat(16)).is_err());\n\
         \tassert!({len_type}::new(\"abc_123\".into()).is_ok());\n\
         }}\n",
        generated.source
    );
    compile_and_run(
        &test_wrapper,
        "string_length_bounds.rs",
        "string_length_bounds_test",
    );
}

/// A ranged-int newtype whose range excludes 0 must not let `Default` mint an
/// out-of-range value. Deriving `Default` yields the inner `0`, which `new()`
/// rejects — an illegal state. `Default` must instead return the range minimum.
#[test]
fn codegen_int_range_default_is_in_range() {
    let dir = generated_dir().join("v2-int-range-default");
    fs::create_dir_all(&dir).unwrap();
    let yang_path = dir.join("v2-int-range-default.yang");
    fs::write(
        &yang_path,
        "module v2-int-range-default {\n\
         \tnamespace \"urn:v2-int-range-default\";\n\
         \tprefix v2ird;\n\
         \trevision 2026-06-15;\n\
         \tcontainer top {\n\
         \t\tleaf port { type uint16 { range \"1..65535\"; } }\n\
         \t}\n\
         }\n",
    )
    .unwrap();

    let ctx = ContextBuilder::new(ContextFlags {
        no_yang_library: true,
        ..Default::default()
    })
    .unwrap()
    .search_path(&dir)
    .unwrap()
    .load_module("v2-int-range-default", None, &[])
    .unwrap()
    .build()
    .unwrap();

    let generated = generate(&ctx, "v2-int-range-default", CodegenOpts::default()).unwrap();

    let test_wrapper = format!(
        "{}\n\
         #[test]\n\
         fn int_range_default_round_trips() {{\n\
         \tlet d = V2IntRangeDefaultTopPortRange::default();\n\
         \tassert!(\n\
         \t\tV2IntRangeDefaultTopPortRange::new(d.get() as i128).is_ok(),\n\
         \t\t\"Default must yield an in-range value, got {{}}\",\n\
         \t\td.get()\n\
         \t);\n\
         }}\n",
        generated.source
    );
    compile_and_run(
        &test_wrapper,
        "int_range_default.rs",
        "int_range_default_test",
    );
}

// ----------------------------------------------------------------------------
// Item 1 — Union typed-struct codegen (red-test-first, golden-backed).
// ----------------------------------------------------------------------------

#[test]
fn codegen_union_enum_and_scalar() {
    let fixture = "types-union-enum-and-scalar";
    let ctx = load_ctx_for_fixture(fixture);
    let module = module_name_from_dir(fixture);
    let generated = generate(&ctx, &module, CodegenOpts::default()).unwrap();
    let src = &generated.source;

    let union_ty = "TypesUnionEnumAndScalarModeUnion";
    assert!(
        src.contains(&format!("pub enum {union_ty} {{")),
        "union enum must be emitted:\n{src}"
    );
    assert!(
        src.contains(&format!("pub mode: Option<{union_ty}>")),
        "union leaf must be typed enum, not String:\n{src}"
    );

    let root = to_pascal_case(&module);
    let instance = format!(
        "{root} {{ mode: Some({union_ty}::Enumeration(TypesUnionEnumAndScalarModeEnumerationEnum::Auto)) }}"
    );
    byte_gate_fixture_xml_json(fixture, &instance, src);
}

#[test]
fn codegen_union_scalar_all_members() {
    let fixture = "types-union-scalar-all-members";
    let ctx = load_ctx_for_fixture(fixture);
    let module = module_name_from_dir(fixture);
    let generated = generate(&ctx, &module, CodegenOpts::default()).unwrap();
    let src = &generated.source;

    let top = "TypesUnionScalarAllMembersTop";
    assert!(src.contains(&format!("pub struct {top} {{")));
    assert!(
        src.contains("pub s: Option<TypesUnionScalarAllMembersTopSUnion>"),
        "union leaf s must be typed:\n{src}"
    );
    assert!(
        src.contains("pub b: Option<TypesUnionScalarAllMembersTopBUnion>"),
        "union leaf b must be typed:\n{src}"
    );

    let root = to_pascal_case(&module);
    let instance = format!(
        "{root} {{ top: {top} {{ \
        s: Some(TypesUnionScalarAllMembersTopSUnion::String(\"str\".into())), \
        b: Some(TypesUnionScalarAllMembersTopBUnion::Boolean(false)), \
        i8: Some(TypesUnionScalarAllMembersTopI8Union::Int8(TypesUnionScalarAllMembersTopI8Int8Range::new(-128).unwrap())), \
        u16: Some(TypesUnionScalarAllMembersTopU16Union::Uint16(TypesUnionScalarAllMembersTopU16Uint16Range::new(65000).unwrap())), \
        i64: Some(TypesUnionScalarAllMembersTopI64Union::Int64(TypesUnionScalarAllMembersTopI64Int64Range::new(-9223372036854775808i128).unwrap())), \
        u64: Some(TypesUnionScalarAllMembersTopU64Union::Uint64(TypesUnionScalarAllMembersTopU64Uint64Range::new(18446744073709551615i128).unwrap())), \
        d: Some(TypesUnionScalarAllMembersTopDUnion::Decimal64(Decimal64::new(-150, 2))), \
        e: Some(TypesUnionScalarAllMembersTopEUnion::Enumeration(TypesUnionScalarAllMembersTopEEnumerationEnum::Beta)), \
        bits: Some(TypesUnionScalarAllMembersTopBitsUnion::Bits(TypesUnionScalarAllMembersTopBitsBitsBits::new(&[\"flag1\", \"flag2\"]).unwrap())), \
        bin: Some(TypesUnionScalarAllMembersTopBinUnion::Binary(\"SGVsbA==\".into())), \
        iid: Some(TypesUnionScalarAllMembersTopIidUnion::InstanceIdentifier(InstanceIdentifier::with_xmlns(\
            \"/typesunionscalarallmembers:top/typesunionscalarallmembers:s\", \
            \"/types-union-scalar-all-members:top/s\", \
            \"typesunionscalarallmembers\", \
            \"urn:types-union-scalar-all-members\", \
        ))) \
        }} }}"
    );
    byte_gate_fixture_xml_json(fixture, &instance, src);
}

#[test]
fn codegen_union_member_resolution_order_quoting_divergence() {
    let fixture = "types-union-member-resolution-order";
    let ctx = load_ctx_for_fixture(fixture);
    let module = module_name_from_dir(fixture);
    let generated = generate(&ctx, &module, CodegenOpts::default()).unwrap();
    let src = &generated.source;

    let union_ty = "TypesUnionMemberResolutionOrderCodeUnion";
    assert!(
        src.contains(&format!("pub code: Option<{union_ty}>")),
        "union leaf must be typed enum, not String:\n{src}"
    );

    let root = to_pascal_case(&module);

    // uint32 variant serializes bare 5.
    let instance_uint = format!(
        "{root} {{ code: Some({union_ty}::Uint32(TypesUnionMemberResolutionOrderCodeUint32Range::new(5).unwrap())) }}"
    );
    byte_gate_fixture_xml_json(fixture, &instance_uint, src);

    // enum variant serializes quoted "five" in JSON and bare "five" in XML.
    // Drive this gate through the libyang oracle with a custom input so the
    // expected bytes are generated, not hand-authored.
    let instance_enum = format!(
        "{root} {{ code: Some({union_ty}::Enumeration(TypesUnionMemberResolutionOrderCodeEnumerationEnum::Five)) }}"
    );
    let escaped_enum = escape_for_rust_string(&instance_enum);
    let input_enum = "<code xmlns=\"urn:types-union-member-resolution-order\">five</code>";
    let expected_xml = libyang_reference_xml_from_input(&ctx, input_enum).unwrap();
    let expected_json = libyang_reference_json_from_input(&ctx, input_enum).unwrap();
    let escaped_xml = escape_for_rust_string(&expected_xml);
    let escaped_json = escape_for_rust_string(&expected_json);
    let wrapper = format!(
        "{src}\n\
         #[test]\n\
         fn generated_union_enum_variant_quoting() {{\n\
         \tlet demo = {instance};\n\
         \tassert_eq!(demo.to_xml(), \"{expected_xml}\");\n\
         \tassert_eq!(demo.to_json_ietf(), \"{expected_json}\");\n\
         }}\n",
        src = src,
        instance = escaped_enum,
        expected_xml = escaped_xml,
        expected_json = escaped_json,
    );
    compile_and_run(
        &wrapper,
        "types_union_member_resolution_order_enum.rs",
        "types_union_member_resolution_order_enum_test",
    );
}

#[test]
fn codegen_union_identityref_member() {
    let fixture = "types-union-identityref-member";
    let ctx = load_ctx_for_fixture(fixture);
    let module = module_name_from_dir(fixture);
    let generated = generate(&ctx, &module, CodegenOpts::default()).unwrap();
    let src = &generated.source;

    let union_ty = "TypesUnionIdentityrefMemberTransportUnion";
    assert!(
        src.contains(&format!("pub transport: Option<{union_ty}>")),
        "union leaf must be typed enum, not String:\n{src}"
    );

    let root = to_pascal_case(&module);
    let instance = format!(
        "{root} {{ transport: Some({union_ty}::Identityref(TypesUnionIdentityrefMemberTransportIdentityrefEnum::Tcp)) }}"
    );
    byte_gate_fixture_xml_json(fixture, &instance, src);
}

#[test]
fn codegen_union_leafref_member() {
    let fixture = "types-union-leafref-member";
    let ctx = load_ctx_for_fixture(fixture);
    let module = module_name_from_dir(fixture);
    let generated = generate(&ctx, &module, CodegenOpts::default()).unwrap();
    let src = &generated.source;

    let union_ty = "TypesUnionLeafrefMemberPrimaryOrStaticUnion";
    assert!(
        src.contains(&format!("pub primary_or_static: Option<{union_ty}>")),
        "union leaf must be typed enum, not String:\n{src}"
    );

    let root = to_pascal_case(&module);
    let instance = format!(
        "{root} {{ interface: vec![TypesUnionLeafrefMemberInterfaceEntry {{ name: \"eth0\".into() }}], primary_or_static: Some({union_ty}::Leafref(\"eth0\".into())) }}"
    );
    byte_gate_fixture_xml_json(fixture, &instance, src);
}

#[test]
fn codegen_union_nested_typedef_chain_typed() {
    let fixture = "types-union-nested-typedef-chain";
    let ctx = load_ctx_for_fixture(fixture);
    let module = module_name_from_dir(fixture);
    let generated = generate(&ctx, &module, CodegenOpts::default()).unwrap();
    let src = &generated.source;

    assert!(
        src.contains("pub ext_comm: Option<TypesUnionNestedTypedefChainTopExtCommUnion>"),
        "typedef'd union leaf ext-comm must be typed enum:\n{src}"
    );
    assert!(
        src.contains("pub ext_comm_bin: Option<TypesUnionNestedTypedefChainTopExtCommBinUnion>"),
        "typedef'd union leaf ext-comm-bin must be typed enum:\n{src}"
    );

    let root = to_pascal_case(&module);
    let top = "TypesUnionNestedTypedefChainTop";
    let instance = format!(
        "{root} {{ top: {top} {{ \
        ext_comm: Some(TypesUnionNestedTypedefChainTopExtCommUnion::CommonString(\"65000:100\".into())), \
        ext_comm_bin: Some(TypesUnionNestedTypedefChainTopExtCommBinUnion::Binary(\"AQIDBAUGBwg=\".into())) \
        }} }}"
    );
    byte_gate_fixture_xml_json(fixture, &instance, src);
}

#[test]
fn codegen_union_typedef_union_composition_typed() {
    let fixture = "types-typedef-union-composition";
    let ctx = load_ctx_for_fixture(fixture);
    let module = module_name_from_dir(fixture);
    let generated = generate(&ctx, &module, CodegenOpts::default()).unwrap();
    let src = &generated.source;

    assert!(
        src.contains("pub server: Option<TypesTypedefUnionCompositionTopServerUnion>"),
        "composed union leaf server must be typed enum:\n{src}"
    );
    assert!(
        src.contains("pub mode: Option<TypesTypedefUnionCompositionTopModeUnion>"),
        "composed union leaf mode must be typed enum:\n{src}"
    );

    let root = to_pascal_case(&module);
    let top = "TypesTypedefUnionCompositionTop";
    let instance = format!(
        "{root} {{ top: {top} {{ \
        server: Some(TypesTypedefUnionCompositionTopServerUnion::String(\"example.com\".into())), \
        mode: Some(TypesTypedefUnionCompositionTopModeUnion::Enumeration(TypesTypedefUnionCompositionTopModeEnumerationEnum::Tcp)) \
        }} }}"
    );
    byte_gate_fixture_xml_json(fixture, &instance, src);
}

#[test]
fn codegen_union_two_identityrefs_distinct_bases() {
    let fixture = "types-union-two-identityrefs-distinct-bases";
    let ctx = load_ctx_for_fixture(fixture);
    let module = module_name_from_dir(fixture);
    let generated = generate(&ctx, &module, CodegenOpts::default()).unwrap();
    let src = &generated.source;

    let union_ty = "TypesUnionTwoIdentityrefsDistinctBasesComponentTypeUnion";
    assert!(
        src.contains(&format!("pub component_type: Option<{union_ty}>")),
        "union leaf must be typed enum, not String:\n{src}"
    );

    let root = to_pascal_case(&module);
    let instance = format!(
        "{root} {{ component_type: Some({union_ty}::Identityref(TypesUnionTwoIdentityrefsDistinctBasesComponentTypeIdentityrefEnum::Linecard)) }}"
    );
    byte_gate_fixture_xml_json(fixture, &instance, src);
}

#[test]
fn codegen_union_heterogeneous_members_quoting() {
    let fixture = "types-union-heterogeneous-members-quoting";
    let ctx = load_ctx_for_fixture(fixture);
    let module = module_name_from_dir(fixture);
    let generated = generate(&ctx, &module, CodegenOpts::default()).unwrap();
    let src = &generated.source;

    let top = "TypesUnionHeterogeneousMembersQuotingTop";
    assert!(src.contains(&format!("pub struct {top} {{")));
    assert!(
        src.contains("pub text: Option<TypesUnionHeterogeneousMembersQuotingTopTextUnion>"),
        "union leaf text must be typed:\n{src}"
    );

    let root = to_pascal_case(&module);
    let instance = format!(
        "{root} {{ top: {top} {{ \
        text: Some(TypesUnionHeterogeneousMembersQuotingTopTextUnion::String(\"text\".into())), \
        flag: Some(TypesUnionHeterogeneousMembersQuotingTopFlagUnion::Boolean(true)), \
        big: Some(TypesUnionHeterogeneousMembersQuotingTopBigUnion::Int64(TypesUnionHeterogeneousMembersQuotingTopBigInt64Range::new(9223372036854775807i128).unwrap())), \
        huge: Some(TypesUnionHeterogeneousMembersQuotingTopHugeUnion::Uint64(TypesUnionHeterogeneousMembersQuotingTopHugeUint64Range::new(18446744073709551615i128).unwrap())), \
        rate: Some(TypesUnionHeterogeneousMembersQuotingTopRateUnion::Decimal64(Decimal64::new(314, 2))) \
        }} }}"
    );
    byte_gate_fixture_xml_json(fixture, &instance, src);
}

// ----------------------------------------------------------------------------
// Item 3 — Empty non-presence container omission + self-closing empty containers.
// ----------------------------------------------------------------------------

#[test]
fn codegen_presence_container_empty_self_closing() {
    let fixture = "container-presence-empty";
    let ctx = load_ctx_for_fixture(fixture);
    let module = module_name_from_dir(fixture);
    let generated = generate(&ctx, &module, CodegenOpts::default()).unwrap();
    let src = &generated.source;

    assert!(
        src.contains("pub enable_ssh: Option<ContainerPresenceEmptyEnableSsh>"),
        "presence container must be Option<Struct>:\n{src}"
    );
    assert!(
        src.contains("pub non_presence: ContainerPresenceEmptyNonPresence"),
        "non-presence container must be bare Struct:\n{src}"
    );

    let root = to_pascal_case(&module);
    let instance = format!(
        "{root} {{ enable_ssh: Some({root}EnableSsh {{ port: None }}), non_presence: {root}NonPresence {{ status: None }} }}"
    );
    byte_gate_fixture_xml_json(fixture, &instance, src);
}

#[test]
fn codegen_nonpresence_container_omitted_when_empty() {
    let fixture = "json-ietf-presence-vs-nonpresence";
    let ctx = load_ctx_for_fixture(fixture);
    let module = module_name_from_dir(fixture);
    let generated = generate(&ctx, &module, CodegenOpts::default()).unwrap();
    let src = &generated.source;

    assert!(
        src.contains("pub ssh: Option<JsonIetfPresenceVsNonpresenceTopSsh>"),
        "presence container ssh must be Option<Struct>:\n{src}"
    );
    assert!(
        src.contains("pub empty_slot: JsonIetfPresenceVsNonpresenceTopEmptySlot"),
        "non-presence container empty-slot must be bare Struct:\n{src}"
    );

    let input = fs::read_to_string(fixture_dir(fixture).join("input.xml")).unwrap();
    let expected = libyang_reference_json_from_input(&ctx, &input).unwrap();
    let escaped = escape_for_rust_string(&expected);

    let instance = "JsonIetfPresenceVsNonpresence { top: JsonIetfPresenceVsNonpresenceTop { ssh: Some(JsonIetfPresenceVsNonpresenceTopSsh { port: None }), empty_slot: JsonIetfPresenceVsNonpresenceTopEmptySlot { name: None } } }";
    let wrapper = format!(
        "{src}\n\
         #[test]\n\
         fn generated_json_omits_empty_nonpresence_container() {{\n\
         \tlet demo = {instance};\n\
         \tassert_eq!(demo.to_json_ietf(), \"{escaped}\");\n\
         }}\n",
        src = src,
        instance = instance,
        escaped = escaped,
    );
    compile_and_run(
        &wrapper,
        "json_ietf_presence_vs_nonpresence.rs",
        "json_ietf_presence_vs_nonpresence_test",
    );
}

// ----------------------------------------------------------------------------
// Item 4 — Engine-routed serializer-acceptance gate (serializer only, no
// deserialization into generated structs).
// ----------------------------------------------------------------------------

#[test]
fn codegen_engine_routed_xml_serializer_acceptance() {
    let fixture = "container-presence-empty";
    let ctx = load_ctx_for_fixture(fixture);
    let module = module_name_from_dir(fixture);
    let generated = generate(&ctx, &module, CodegenOpts::default()).unwrap();
    let src = &generated.source;

    let root = to_pascal_case(&module);
    let instance = format!(
        "{root} {{ enable_ssh: Some({root}EnableSsh {{ port: None }}), non_presence: {root}NonPresence {{ status: None }} }}"
    );
    engine_routed_xml_gate(&ctx, fixture, &instance, src);
}

/// A length-bounded string newtype whose minimum length is > 0 must not let
/// `Default` mint the empty string, which `new()` rejects. `Default` must
/// instead return a string of the minimum valid length.
#[test]
fn codegen_string_length_default_is_valid() {
    let fixture = "types-string-length-pattern-anchor-posix";
    let ctx = load_ctx_for_fixture(fixture);
    let module = module_name_from_dir(fixture);
    let generated = generate(&ctx, &module, CodegenOpts::default()).unwrap();

    let len_type = "TypesStringLengthPatternAnchorPosixCodeLength";
    let test_wrapper = format!(
        "{src}\n\
         #[test]\n\
         fn string_length_default_round_trips() {{\n\
         \tlet d = {len_type}::default();\n\
         \tassert!(\n\
         \t\t{len_type}::new(d.as_str().to_string()).is_ok(),\n\
         \t\t\"Default must satisfy the length bound, got {{:?}}\",\n\
         \t\td.as_str()\n\
         \t);\n\
         }}\n",
        src = generated.source,
        len_type = len_type,
    );
    compile_and_run(
        &test_wrapper,
        "string_length_default.rs",
        "string_length_default_test",
    );
}

#[test]
fn codegen_leafref_chases_int_target() {
    let dir = generated_dir().join("v2-leafref-int");
    fs::create_dir_all(&dir).unwrap();
    let yang_path = dir.join("v2-leafref-int.yang");
    fs::write(
        &yang_path,
        "module v2-leafref-int {\n\
         \tnamespace \"urn:v2-leafref-int\";\n\
         \tprefix v2li;\n\
         \trevision 2026-06-15;\n\
         \tcontainer top {\n\
         \t\tleaf target { type int8; }\n\
         \t\tleaf ref { type leafref { path \"../target\"; } }\n\
         \t}\n\
         }\n",
    )
    .unwrap();

    let ctx = ContextBuilder::new(ContextFlags {
        no_yang_library: true,
        ..Default::default()
    })
    .unwrap()
    .search_path(&dir)
    .unwrap()
    .load_module("v2-leafref-int", None, &[])
    .unwrap()
    .build()
    .unwrap();

    let generated = generate(&ctx, "v2-leafref-int", CodegenOpts::default()).unwrap();
    let src = &generated.source;

    assert!(
        src.contains("pub target: Option<V2LeafrefIntTopTargetRange>"),
        "target leaf must be typed int range:\n{src}"
    );
    assert!(
        src.contains("pub r#ref: Option<V2LeafrefIntTopRefRange>"),
        "leafref leaf must chase target int range:\n{src}"
    );

    let input = "<top xmlns=\"urn:v2-leafref-int\">\n\
                 \t<target>42</target>\n\
                 \t<ref>42</ref>\n\
                 </top>\n";
    let expected_xml = libyang_reference_xml_from_input(&ctx, input).unwrap();
    let expected_json = libyang_reference_json_from_input(&ctx, input).unwrap();

    let instance = "V2LeafrefInt { top: V2LeafrefIntTop { target: Some(V2LeafrefIntTopTargetRange::new(42).unwrap()), r#ref: Some(V2LeafrefIntTopRefRange::new(42).unwrap()) } }";

    let escaped_xml = escape_for_rust_string(&expected_xml);
    let xml_wrapper = format!(
        "{}\n\
         #[test]\n\
         fn generated_xml_matches_libyang() {{\n\
         \tassert_eq!({}.to_xml(), \"{}\");\n\
         }}\n",
        src, instance, escaped_xml
    );
    compile_and_run(
        &xml_wrapper,
        "v2_leafref_int_xml.rs",
        "v2_leafref_int_xml_test",
    );

    let escaped_json = escape_for_rust_string(&expected_json);
    let json_wrapper = format!(
        "{}\n\
         #[test]\n\
         fn generated_json_matches_libyang() {{\n\
         \tassert_eq!({}.to_json_ietf(), \"{}\");\n\
         }}\n",
        src, instance, escaped_json
    );
    compile_and_run(
        &json_wrapper,
        "v2_leafref_int_json.rs",
        "v2_leafref_int_json_test",
    );
}

#[test]
fn codegen_leafref_to_string_absolute_path() {
    let fixture = "types-leafref-absolute-path";
    let ctx = load_ctx_for_fixture(fixture);
    let module = module_name_from_dir(fixture);
    let generated = generate(&ctx, &module, CodegenOpts::default()).unwrap();
    let src = &generated.source;

    assert!(
        src.contains("pub primary_iface: Option<String>"),
        "leafref to string must resolve to String:\n{src}"
    );

    let root = to_pascal_case(&module);
    let instance = format!(
        "{root} {{ top: {root}Top {{ iface: vec![{root}TopIfaceEntry {{ name: \"eth0\".to_string() }}] }}, primary_iface: Some(\"eth0\".to_string()) }}"
    );
    byte_gate_fixture_xml_json(fixture, &instance, src);
}

#[test]
fn codegen_leafref_to_leaf_list_element() {
    let fixture = "types-leafref-to-leaf-list";
    let ctx = load_ctx_for_fixture(fixture);
    let module = module_name_from_dir(fixture);
    let generated = generate(&ctx, &module, CodegenOpts::default()).unwrap();
    let src = &generated.source;

    assert!(
        src.contains("pub assigned_tag: Option<String>"),
        "leafref to leaf-list element must resolve to element String:\n{src}"
    );

    let root = to_pascal_case(&module);
    // System-ordered leaf-lists are serialized in sorted order by libyang.
    let instance = format!(
        "{root} {{ tag: vec![\"blue\".to_string(), \"red\".to_string()], assigned_tag: Some(\"red\".to_string()) }}"
    );
    byte_gate_fixture_xml_json(fixture, &instance, src);
}

#[test]
fn codegen_cambium_struct_trait() {
    let fixture = "types-leafref-absolute-path";
    let ctx = load_ctx_for_fixture(fixture);
    let module = module_name_from_dir(fixture);
    let generated = generate(&ctx, &module, CodegenOpts::default()).unwrap();
    let src = &generated.source;

    assert!(
        src.contains("pub trait CambiumStruct"),
        "CambiumStruct trait must be emitted:\n{src}"
    );

    let root = to_pascal_case(&module);
    assert!(
        src.contains(&format!("impl CambiumStruct for {root}")),
        "root struct must implement CambiumStruct:\n{src}"
    );
    assert!(
        src.contains(&format!("impl CambiumStruct for {root}Top")),
        "nested struct must implement CambiumStruct:\n{src}"
    );

    let wrapper = format!(
        "{}\n\
         #[test]\n\
         fn cambium_struct_trait_is_implemented() {{\n\
         \tfn assert_cs<T: CambiumStruct>(_: &T) {{}}\n\
         \tlet demo = {root} {{\n\
         \t\ttop: {root}Top {{ iface: vec![] }},\n\
         \t\tprimary_iface: None,\n\
         \t}};\n\
         \tassert_cs(&demo);\n\
         \tassert_cs(&demo.top);\n\
         }}\n",
        src
    );
    compile_and_run(
        &wrapper,
        "cambium_struct_trait.rs",
        "cambium_struct_trait_test",
    );
}
