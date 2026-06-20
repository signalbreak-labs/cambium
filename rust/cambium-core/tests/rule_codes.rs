//! The rule code assigned to a failure must match the Go side (parity):
//! loading a missing module is CAMBIUM_E0001 (Context) in both languages.

#![allow(clippy::unwrap_used)]

use cambium_core::{Context, Format, ParseMode, RuleCode, ValidateMode};
use std::path::PathBuf;

fn conformance_dir() -> PathBuf {
    let mut dir = PathBuf::from(env!("CARGO_MANIFEST_DIR"));
    dir.pop(); // cambium-core -> rust
    dir.pop(); // rust -> workspace root
    dir.join("conformance")
}

#[test]
fn missing_module_is_context_code() {
    let mut ctx = match Context::new() {
        Ok(c) => c,
        Err(e) => panic!("new context: {e}"),
    };
    match ctx.load_module("definitely-not-a-real-module") {
        Ok(()) => panic!("expected error loading missing module"),
        Err(e) => assert_eq!(e.rule_code(), RuleCode::Context),
    }
}

#[test]
fn int32_multipart_range_gap_rejected() {
    let mut ctx = Context::new().unwrap();
    let module_dir = conformance_dir().join("fixtures/types-int-int32-range-multipart/module");
    ctx.set_search_path(&module_dir).unwrap();
    ctx.load_module("types-int-int32-range-multipart").unwrap();
    let xml = b"<priority xmlns=\"urn:types-int-int32-range-multipart\">0</priority>";
    let err = ctx
        .parse(Format::Xml, ParseMode::data_only(), xml)
        .unwrap_err();
    // Parse-time type restriction violations are mapped to E0002 (Parse) in this
    // implementation; the catalog annotates them as E0003 (Validate) but both
    // languages agree on E0002, satisfying rule-code parity.
    assert_eq!(err.rule_code(), RuleCode::Parse);
}

#[test]
fn uint16_port_zero_rejected() {
    let mut ctx = Context::new().unwrap();
    let module_dir = conformance_dir().join("fixtures/types-uint-uint16-range-port/module");
    ctx.set_search_path(&module_dir).unwrap();
    ctx.load_module("types-uint-uint16-range-port").unwrap();
    let xml = b"<port xmlns=\"urn:types-uint-uint16-range-port\">0</port>";
    let err = ctx
        .parse(Format::Xml, ParseMode::data_only(), xml)
        .unwrap_err();
    assert_eq!(err.rule_code(), RuleCode::Parse);
}

// Theme 2: type-restrictions rejection parity.
// Parse-time pattern/range/length violations are mapped to E0002 (Parse).

#[test]
fn invert_match_lowercase_rejected() {
    let mut ctx = Context::new().unwrap();
    let module_dir =
        conformance_dir().join("fixtures/types-string-pattern-modifier-invert-match/module");
    ctx.set_search_path(&module_dir).unwrap();
    ctx.load_module("types-string-pattern-modifier-invert-match")
        .unwrap();
    let xml = b"<name xmlns=\"urn:types-string-pattern-modifier-invert-match\">abc</name>";
    let err = ctx
        .parse(Format::Xml, ParseMode::data_only(), xml)
        .unwrap_err();
    assert_eq!(err.rule_code(), RuleCode::Parse);
}

#[test]
fn multiple_patterns_letter_only_rejected() {
    let mut ctx = Context::new().unwrap();
    let module_dir =
        conformance_dir().join("fixtures/types-string-multiple-patterns-conjunction/module");
    ctx.set_search_path(&module_dir).unwrap();
    ctx.load_module("types-string-multiple-patterns-conjunction")
        .unwrap();
    let xml = b"<token xmlns=\"urn:types-string-multiple-patterns-conjunction\">abc</token>";
    let err = ctx
        .parse(Format::Xml, ParseMode::data_only(), xml)
        .unwrap_err();
    assert_eq!(err.rule_code(), RuleCode::Parse);
}

#[test]
fn multiple_patterns_digit_only_rejected() {
    let mut ctx = Context::new().unwrap();
    let module_dir =
        conformance_dir().join("fixtures/types-string-multiple-patterns-conjunction/module");
    ctx.set_search_path(&module_dir).unwrap();
    ctx.load_module("types-string-multiple-patterns-conjunction")
        .unwrap();
    let xml = b"<token xmlns=\"urn:types-string-multiple-patterns-conjunction\">123</token>";
    let err = ctx
        .parse(Format::Xml, ParseMode::data_only(), xml)
        .unwrap_err();
    assert_eq!(err.rule_code(), RuleCode::Parse);
}

#[test]
fn length_pattern_toolong_rejected() {
    let mut ctx = Context::new().unwrap();
    let module_dir =
        conformance_dir().join("fixtures/types-string-length-pattern-anchor-posix/module");
    ctx.set_search_path(&module_dir).unwrap();
    ctx.load_module("types-string-length-pattern-anchor-posix")
        .unwrap();
    let xml = b"<code xmlns=\"urn:types-string-length-pattern-anchor-posix\">12345678901</code>";
    let err = ctx
        .parse(Format::Xml, ParseMode::data_only(), xml)
        .unwrap_err();
    assert_eq!(err.rule_code(), RuleCode::Parse);
}

#[test]
fn length_pattern_nonalnum_rejected() {
    let mut ctx = Context::new().unwrap();
    let module_dir =
        conformance_dir().join("fixtures/types-string-length-pattern-anchor-posix/module");
    ctx.set_search_path(&module_dir).unwrap();
    ctx.load_module("types-string-length-pattern-anchor-posix")
        .unwrap();
    let xml = b"<code xmlns=\"urn:types-string-length-pattern-anchor-posix\">abc-</code>";
    let err = ctx
        .parse(Format::Xml, ParseMode::data_only(), xml)
        .unwrap_err();
    assert_eq!(err.rule_code(), RuleCode::Parse);
}

#[test]
fn range_length_reject_underflow() {
    let mut ctx = Context::new().unwrap();
    let module_dir = conformance_dir().join("fixtures/constraints-range-length-reject/module");
    ctx.set_search_path(&module_dir).unwrap();
    ctx.load_module("constraints-range-length-reject").unwrap();
    let xml = b"<top xmlns=\"urn:constraints-range-length-reject\"><n>0</n><s>abc</s></top>";
    let err = ctx
        .parse(Format::Xml, ParseMode::data_only(), xml)
        .unwrap_err();
    assert_eq!(err.rule_code(), RuleCode::Parse);
}

#[test]
fn range_length_reject_overflow() {
    let mut ctx = Context::new().unwrap();
    let module_dir = conformance_dir().join("fixtures/constraints-range-length-reject/module");
    ctx.set_search_path(&module_dir).unwrap();
    ctx.load_module("constraints-range-length-reject").unwrap();
    let xml = b"<top xmlns=\"urn:constraints-range-length-reject\"><n>101</n><s>abc</s></top>";
    let err = ctx
        .parse(Format::Xml, ParseMode::data_only(), xml)
        .unwrap_err();
    assert_eq!(err.rule_code(), RuleCode::Parse);
}

#[test]
fn range_length_reject_string_too_long() {
    let mut ctx = Context::new().unwrap();
    let module_dir = conformance_dir().join("fixtures/constraints-range-length-reject/module");
    ctx.set_search_path(&module_dir).unwrap();
    ctx.load_module("constraints-range-length-reject").unwrap();
    let xml = b"<top xmlns=\"urn:constraints-range-length-reject\"><n>42</n><s>toolong</s></top>";
    let err = ctx
        .parse(Format::Xml, ParseMode::data_only(), xml)
        .unwrap_err();
    assert_eq!(err.rule_code(), RuleCode::Parse);
}

#[test]
fn decimal64_exponent_input_rejected() {
    let mut ctx = Context::new().unwrap();
    let module_dir = conformance_dir().join("fixtures/json-ietf-decimal64-no-exponent/module");
    ctx.set_search_path(&module_dir).unwrap();
    ctx.load_module("json-ietf-decimal64-no-exponent").unwrap();
    let xml = b"<rate xmlns=\"urn:json-ietf-decimal64-no-exponent\">1E-9</rate>";
    let err = ctx
        .parse(Format::Xml, ParseMode::data_only(), xml)
        .unwrap_err();
    assert_eq!(err.rule_code(), RuleCode::Parse);
}

#[test]
fn typedef_narrowing_overflow_rejected() {
    let mut ctx = Context::new().unwrap();
    let module_dir = conformance_dir().join("fixtures/types-typedef-restriction-narrowing/module");
    ctx.set_search_path(&module_dir).unwrap();
    ctx.load_module("types-typedef-restriction-narrowing")
        .unwrap();
    let xml = b"<ssh xmlns=\"urn:types-typedef-restriction-narrowing\">2000</ssh>";
    let err = ctx
        .parse(Format::Xml, ParseMode::data_only(), xml)
        .unwrap_err();
    assert_eq!(err.rule_code(), RuleCode::Parse);
}

// Theme 13: linkage rejection parity.

fn load_fixture_modules(ctx: &mut Context, name: &str) {
    let dir = conformance_dir().join("fixtures").join(name).join("module");
    ctx.set_search_path(&dir).unwrap();
    for entry in std::fs::read_dir(&dir).unwrap() {
        let path = entry.unwrap().path();
        if path.extension() != Some(std::ffi::OsStr::new("yang")) {
            continue;
        }
        let text = std::fs::read_to_string(&path).unwrap();
        if text.trim_start().starts_with("submodule ") {
            continue;
        }
        let stem = path.file_stem().unwrap().to_str().unwrap();
        let mod_name = stem.split('@').next().unwrap();
        ctx.load_module(mod_name).unwrap();
    }
}

#[test]
fn linkage_refine_presence_must_rejected() {
    let mut ctx = Context::new().unwrap();
    load_fixture_modules(&mut ctx, "linkage-refine-presence-must");
    let xml =
        b"<system xmlns=\"urn:linkage-refine-presence-must\"><opts><val>0</val></opts></system>";
    let mut tree = ctx.parse(Format::Xml, ParseMode::data_only(), xml).unwrap();
    let err = tree
        .validate(ValidateMode {
            present: true,
            ..Default::default()
        })
        .unwrap_err();
    assert_eq!(err.rule_code(), RuleCode::Validate);
}

#[test]
fn linkage_refine_min_max_empty_tags_rejected() {
    let mut ctx = Context::new().unwrap();
    load_fixture_modules(&mut ctx, "linkage-refine-min-max-iffeature");
    let xml = b"<policy xmlns=\"urn:linkage-refine-min-max-iffeature\"></policy><service xmlns=\"urn:linkage-refine-min-max-iffeature\"><mode>auto</mode></service>";
    let mut tree = ctx.parse(Format::Xml, ParseMode::data_only(), xml).unwrap();
    let err = tree
        .validate(ValidateMode {
            present: true,
            ..Default::default()
        })
        .unwrap_err();
    assert_eq!(err.rule_code(), RuleCode::Validate);
}

#[test]
fn linkage_augment_intra_module_when_false_rejected() {
    let mut ctx = Context::new().unwrap();
    load_fixture_modules(&mut ctx, "linkage-augment-intra-module");
    let xml = b"<interfaces xmlns=\"urn:linkage-augment-intra-module\"><interface><name>lag0</name><type>lag</type><speed>1000</speed></interface></interfaces>";
    let mut tree = ctx.parse(Format::Xml, ParseMode::data_only(), xml).unwrap();
    let err = tree
        .validate(ValidateMode {
            present: true,
            ..Default::default()
        })
        .unwrap_err();
    assert_eq!(err.rule_code(), RuleCode::Validate);
}

#[test]
fn linkage_augment_when_target_context_disabled_rejected() {
    let mut ctx = Context::new().unwrap();
    load_fixture_modules(&mut ctx, "augment-when-target-context");
    let xml = b"<system xmlns=\"urn:augment-when-target-context-base\" xmlns:awtca=\"urn:augment-when-target-context\"><mode>disabled</mode><ospf><router-id>1.1.1.1</router-id><awtca:area>0</awtca:area></ospf></system>";
    let mut tree = ctx.parse(Format::Xml, ParseMode::data_only(), xml).unwrap();
    let err = tree.validate(ValidateMode::default()).unwrap_err();
    assert_eq!(err.rule_code(), RuleCode::Validate);
}

#[test]
fn linkage_deviation_not_supported_unknown_rejected() {
    let mut ctx = Context::new().unwrap();
    load_fixture_modules(&mut ctx, "linkage-deviation-not-supported");
    let xml = b"<c xmlns=\"urn:linkage-deviation-not-supported-base\"><deprecated-field>x</deprecated-field><active-field>ok</active-field></c>";
    let err = ctx
        .parse(
            Format::Xml,
            ParseMode {
                strict: true,
                parse_only: true,
                ..Default::default()
            },
            xml,
        )
        .unwrap_err();
    assert_eq!(err.rule_code(), RuleCode::Parse);
}

#[test]
fn linkage_deviation_replace_type_overflow_rejected() {
    let mut ctx = Context::new().unwrap();
    load_fixture_modules(&mut ctx, "linkage-deviation-replace-type");
    let xml = b"<c xmlns=\"urn:linkage-deviation-replace-type-base\"><count>2000</count></c>";
    let err = ctx
        .parse(Format::Xml, ParseMode::data_only(), xml)
        .unwrap_err();
    assert_eq!(err.rule_code(), RuleCode::Parse);
}

#[test]
fn linkage_deviation_add_mandatory_missing_rejected() {
    let mut ctx = Context::new().unwrap();
    load_fixture_modules(&mut ctx, "linkage-deviation-add");
    let xml = b"<c xmlns=\"urn:linkage-deviation-add-base\"/>";
    let mut tree = ctx.parse(Format::Xml, ParseMode::data_only(), xml).unwrap();
    let err = tree
        .validate(ValidateMode {
            present: true,
            ..Default::default()
        })
        .unwrap_err();
    assert_eq!(err.rule_code(), RuleCode::Validate);
}

#[test]
fn linkage_deviation_multi_legacy_rejected() {
    let mut ctx = Context::new().unwrap();
    load_fixture_modules(&mut ctx, "linkage-deviation-multi");
    let xml = b"<c xmlns=\"urn:linkage-deviation-multi-base\"><legacy>old</legacy><name>x</name><maximum>500</maximum></c>";
    let err = ctx
        .parse(
            Format::Xml,
            ParseMode {
                strict: true,
                parse_only: true,
                ..Default::default()
            },
            xml,
        )
        .unwrap_err();
    assert_eq!(err.rule_code(), RuleCode::Parse);
}

#[test]
fn linkage_deviation_multi_maximum_overflow_rejected() {
    let mut ctx = Context::new().unwrap();
    load_fixture_modules(&mut ctx, "linkage-deviation-multi");
    let xml =
        b"<c xmlns=\"urn:linkage-deviation-multi-base\"><name>x</name><maximum>2000</maximum></c>";
    let err = ctx
        .parse(Format::Xml, ParseMode::data_only(), xml)
        .unwrap_err();
    assert_eq!(err.rule_code(), RuleCode::Parse);
}

#[test]
fn linkage_deviation_multi_name_missing_rejected() {
    let mut ctx = Context::new().unwrap();
    load_fixture_modules(&mut ctx, "linkage-deviation-multi");
    let xml = b"<c xmlns=\"urn:linkage-deviation-multi-base\"><maximum>500</maximum></c>";
    let mut tree = ctx.parse(Format::Xml, ParseMode::data_only(), xml).unwrap();
    let err = tree
        .validate(ValidateMode {
            present: true,
            ..Default::default()
        })
        .unwrap_err();
    assert_eq!(err.rule_code(), RuleCode::Validate);
}

#[test]
fn linkage_deviation_replace_default_config_must_rejected() {
    let mut ctx = Context::new().unwrap();
    load_fixture_modules(&mut ctx, "linkage-deviation-replace-default-config");
    let xml = b"<system xmlns=\"urn:linkage-deviation-replace-default-config-base\"><mode>disabled</mode><ospf><router-id>1.1.1.1</router-id></ospf></system>";
    let mut tree = ctx.parse(Format::Xml, ParseMode::data_only(), xml).unwrap();
    let err = tree
        .validate(ValidateMode {
            present: true,
            ..Default::default()
        })
        .unwrap_err();
    assert_eq!(err.rule_code(), RuleCode::Validate);
}

#[test]
fn linkage_import_non_transitive_a_fails() {
    let mut ctx = Context::new().unwrap();
    let dir = conformance_dir().join("fixtures/linkage-import-non-transitive/module");
    ctx.set_search_path(&dir).unwrap();
    let err = ctx
        .load_module("linkage-import-non-transitive-a")
        .unwrap_err();
    assert_eq!(err.rule_code(), RuleCode::Context);
}

#[test]
fn linkage_import_non_transitive_b_loads() {
    let mut ctx = Context::new().unwrap();
    let dir = conformance_dir().join("fixtures/linkage-import-non-transitive/module");
    ctx.set_search_path(&dir).unwrap();
    ctx.load_module("linkage-import-non-transitive-b").unwrap();
}

// Theme 15: edge-illegality rejection parity.

#[test]
fn parse_malformed_truncated_xml_rejected() {
    let mut ctx = Context::new().unwrap();
    let dir = conformance_dir().join("fixtures/parse-malformed-e0002/module");
    ctx.set_search_path(&dir).unwrap();
    ctx.load_module("parse-malformed-e0002").unwrap();
    let xml = b"<top xmlns=\"urn:parse-malformed-e0002\"><name>incomplete</name>";
    let err = ctx
        .parse(Format::Xml, ParseMode::data_only(), xml)
        .unwrap_err();
    assert_eq!(err.rule_code(), RuleCode::Parse);
}

#[test]
fn parse_malformed_interior_nul_rejected() {
    let mut ctx = Context::new().unwrap();
    let dir = conformance_dir().join("fixtures/parse-malformed-e0002/module");
    ctx.set_search_path(&dir).unwrap();
    ctx.load_module("parse-malformed-e0002").unwrap();
    let mut xml = b"<top xmlns=\"urn:parse-malformed-e0002\"><name>".to_vec();
    xml.push(0);
    xml.extend_from_slice(b"x</name></top>");
    let err = ctx
        .parse(Format::Xml, ParseMode::data_only(), &xml)
        .unwrap_err();
    assert_eq!(err.rule_code(), RuleCode::Parse);
}

#[test]
fn parse_malformed_strict_unknown_element_rejected() {
    let mut ctx = Context::new().unwrap();
    let dir = conformance_dir().join("fixtures/parse-malformed-e0002/module");
    ctx.set_search_path(&dir).unwrap();
    ctx.load_module("parse-malformed-e0002").unwrap();
    let xml = b"<top xmlns=\"urn:parse-malformed-e0002\"><name>ok</name><unknown>x</unknown></top>";
    let err = ctx
        .parse(
            Format::Xml,
            ParseMode {
                strict: true,
                parse_only: true,
                ..Default::default()
            },
            xml,
        )
        .unwrap_err();
    assert_eq!(err.rule_code(), RuleCode::Parse);
}

#[test]
fn empty_default_illegal_load_fails() {
    let mut ctx = Context::new().unwrap();
    let dir = conformance_dir().join("fixtures/types-empty-edge-illegality/module");
    ctx.set_search_path(&dir).unwrap();
    let err = ctx.load_module("empty-default-illegal").unwrap_err();
    assert_eq!(err.rule_code(), RuleCode::Context);
}

#[test]
fn empty_leaflist_1_0_illegal_load_fails() {
    let mut ctx = Context::new().unwrap();
    let dir = conformance_dir().join("fixtures/types-empty-edge-illegality/module");
    ctx.set_search_path(&dir).unwrap();
    let err = ctx.load_module("empty-leaflist-1_0-illegal").unwrap_err();
    assert_eq!(err.rule_code(), RuleCode::Context);
}

#[test]
fn empty_leaflist_1_1_legal_loads() {
    let mut ctx = Context::new().unwrap();
    let dir = conformance_dir().join("fixtures/types-empty-edge-illegality/module");
    ctx.set_search_path(&dir).unwrap();
    ctx.load_module("empty-leaflist-1_1-legal").unwrap();
}
