//! Phase 1 acceptance tests: rich schema node metadata + type constraints.
#![allow(clippy::unwrap_used, clippy::expect_used)]

use std::fs;
use std::path::{Path, PathBuf};
use std::sync::atomic::{AtomicU64, Ordering};

use cambium_core::{
    BaseType, Config, ContextBuilder, IntKind, OrderedBy, ResolvedType, SchemaNodeKind, Status,
};

fn temp_module_dir() -> PathBuf {
    static DIR_COUNTER: AtomicU64 = AtomicU64::new(0);
    let n = DIR_COUNTER.fetch_add(1, Ordering::Relaxed);
    let dir = PathBuf::from(env!("CARGO_MANIFEST_DIR"))
        .join("../../target/tests/schema-introspection/modules")
        .join(format!("case-{n}"));
    fs::create_dir_all(&dir).unwrap();
    dir
}

static WRITE_COUNTER: AtomicU64 = AtomicU64::new(0);

fn atomic_write(path: &std::path::Path, content: &str) {
    let dir = path.parent().unwrap();
    let n = WRITE_COUNTER.fetch_add(1, Ordering::Relaxed);
    let tmp = dir.join(format!(".tmp-{n}.yang"));
    fs::write(&tmp, content).unwrap();
    fs::rename(&tmp, path).unwrap();
}

fn write_module(name: &str, source: &str) -> PathBuf {
    let dir = temp_module_dir();
    write_module_to(&dir, name, source);
    dir
}

fn write_module_to(dir: &Path, name: &str, source: &str) {
    let path = dir.join(format!("{name}.yang"));
    // Atomic write keeps each generated module complete even if a helper grows
    // additional same-directory writers later.
    atomic_write(&path, source);
}

fn introspection_module() -> &'static str {
    "cambium-introspection-demo"
}

fn load_introspection_context() -> (cambium_core::Context, PathBuf) {
    let yang = r#"module cambium-introspection-demo {
    namespace "urn:cambium:introspection";
    prefix cid;
    yang-version 1.1;
    revision 2026-06-13;

    identity base-id;
    identity mid-id {
        base base-id;
    }
    identity leaf-id {
        base mid-id;
    }

    extension camelcase-name {
        argument value;
    }
    extension flag-foo;

    grouping common-grouping {
        leaf grouped-leaf {
            type string;
        }
        container grouped-container {
            leaf nested-leaf {
                type string;
            }
        }
    }

    container top {
        description "Top-level ordered container.";
        leaf direct-leaf {
            type string;
        }
        uses common-grouping;
        leaf rw-flag {
            cid:flag-foo;
            cid:camelcase-name "fooBar";
            type boolean;
            default "true";
        }
        leaf-list multi-defaults {
            type string;
            default "alpha";
            default "beta";
            default "gamma";
        }
        leaf ro-counter {
            config false;
            type uint64;
        }
        leaf deprecated-leaf {
            status deprecated;
            type string;
        }
        leaf mandatory-leaf {
            mandatory "true";
            must "../rw-flag = 'true'" {
                error-message "rw-flag must be true";
                error-app-tag "must-rw-flag";
            }
            type string;
        }
        container presence-box {
            when "../rw-flag = 'true'" {
                description "Only present when rw-flag is true";
                reference "RFC 6020 when statement";
            }
            presence "Explicit presence container.";
            leaf inner {
                type string;
                units "packets";
            }
        }
        leaf-list typed-leaf-list {
            ordered-by user;
            type string;
            min-elements 1;
            max-elements 10;
        }
        list keyed-list {
            key "name color";
            unique "name extra";
            leaf name {
                type string;
            }
            leaf color {
                type string;
            }
            leaf extra {
                type string;
            }
        }
        leaf all-builtins {
            type int64;
        }
        leaf dec64 {
            type decimal64 {
                fraction-digits 4;
                range "0..100";
            }
        }
        leaf status-enum {
            type enumeration {
                enum up { value 1; }
                enum down { value 2; }
                enum unknown { value 0; }
            }
        }
        leaf flags-bits {
            type bits {
                bit read;
                bit write;
                bit execute;
            }
        }
        leaf uni {
            type union {
                type string;
                type int32;
            }
        }
        leaf ranged-int {
            type int32 {
                range "1..10|20..max";
            }
        }
        leaf ranged-dec64 {
            type decimal64 {
                fraction-digits 2;
                range "0..100";
            }
        }
        leaf constrained-string {
            type string {
                length "1..255";
                pattern "[a-zA-Z0-9_-]+" {
                    error-app-tag "my-tag";
                }
                pattern "^foo.*" {
                    modifier invert-match;
                }
            }
        }
        leaf idref {
            type identityref {
                base base-id;
            }
        }
        leaf ref-to-name {
            type leafref {
                path "/cid:top/keyed-list/name";
            }
        }
        choice preference {
            case primary {
                leaf primary-name {
                    type string;
                }
            }
            case secondary {
                leaf secondary-name {
                    type string;
                }
            }
        }
        anyxml raw-data;
        action reset {
            input {
                leaf force {
                    type boolean;
                }
            }
            output {
                leaf accepted {
                    type boolean;
                }
            }
        }
        leaf post-action {
            type string;
        }
    }
    rpc reboot {
        input {
            leaf delay {
                type uint16;
            }
        }
        output {
            leaf status {
                type string;
            }
        }
    }
    notification event {
        leaf severity {
            type string;
        }
    }
}
"#;
    let dir = temp_module_dir();
    write_module_to(&dir, introspection_module(), yang);
    let ctx = ContextBuilder::new(Default::default())
        .unwrap()
        .search_path(&dir)
        .unwrap()
        .load_module(introspection_module(), None, &[])
        .unwrap()
        .build()
        .unwrap();
    (ctx, dir)
}

fn load_provenance_context() -> (cambium_core::Context, PathBuf) {
    let target = r#"module cambium-provenance-target {
    namespace "urn:cambium:provtarget";
    prefix cpt;
    yang-version 1.1;

    container top {
        leaf a {
            type uint32;
        }
    }
}
"#;
    let augment = r#"module cambium-provenance-augment {
    namespace "urn:cambium:provaugment";
    prefix cpa;
    yang-version 1.1;

    import cambium-provenance-target {
        prefix cpt;
    }

    augment "/cpt:top" {
        leaf b {
            type string;
        }
    }
}
"#;
    let deviate = r#"module cambium-provenance-deviate {
    namespace "urn:cambium:provdeviate";
    prefix cpd;
    yang-version 1.1;

    import cambium-provenance-target {
        prefix cpt;
    }

    deviation "/cpt:top/cpt:a" {
        deviate replace {
            type string;
        }
    }
}
"#;

    let dir = temp_module_dir();
    write_module_to(&dir, "cambium-provenance-target", target);
    write_module_to(&dir, "cambium-provenance-augment", augment);
    write_module_to(&dir, "cambium-provenance-deviate", deviate);
    let ctx = ContextBuilder::new(Default::default())
        .unwrap()
        .search_path(&dir)
        .unwrap()
        .load_module("cambium-provenance-target", None, &[])
        .unwrap()
        .load_module("cambium-provenance-augment", None, &[])
        .unwrap()
        .load_module("cambium-provenance-deviate", None, &[])
        .unwrap()
        .build()
        .unwrap();
    (ctx, dir)
}

fn deviation_target_module() -> &'static str {
    "cambium-deviation-target"
}

fn deviation_source_module() -> &'static str {
    "cambium-deviation-source"
}

fn load_deviation_context() -> (cambium_core::Context, PathBuf) {
    let target = r#"module cambium-deviation-target {
    namespace "urn:cambium:devtarget";
    prefix cdt;
    yang-version 1.1;
    revision 2026-06-16;

    container top {
        leaf metric {
            type string;
        }
        leaf enabled {
            type string;
        }
        leaf label {
            type string;
        }
        leaf defaulted {
            type string;
            default "old";
        }
        leaf-list samples {
            type string;
        }
        list records {
            key "id";
            leaf id {
                type string;
            }
        }
    }
}
"#;
    let source = r#"module cambium-deviation-source {
    namespace "urn:cambium:devsource";
    prefix cds;
    yang-version 1.1;

    import cambium-deviation-target {
        prefix cdt;
        revision-date 2026-06-16;
    }

    revision 2026-06-16;

    container top {
        leaf metric {
            type string;
        }
    }

    deviation "/cdt:top/cdt:metric" {
        description "Make metric numeric.";
        reference "RFC 6020 deviation";
        deviate replace {
            type uint32;
        }
    }

    deviation "/cdt:top/cdt:enabled" {
        deviate not-supported;
    }

    deviation "/cdt:top/cdt:label" {
        deviate add {
            units "meters";
        }
    }

    deviation "/cdt:top/cdt:defaulted" {
        deviate replace {
            default "";
        }
    }

    deviation "/cdt:top/cdt:samples" {
        deviate replace {
            min-elements 2;
            max-elements 4;
        }
    }

    deviation "/cdt:top/cdt:records" {
        deviate replace {
            max-elements unbounded;
        }
    }
}
"#;

    let dir = temp_module_dir();
    write_module_to(&dir, deviation_target_module(), target);
    write_module_to(&dir, deviation_source_module(), source);
    let ctx = ContextBuilder::new(Default::default())
        .unwrap()
        .search_path(&dir)
        .unwrap()
        .load_module(deviation_target_module(), None, &[])
        .unwrap()
        .load_module(deviation_source_module(), None, &[])
        .unwrap()
        .build()
        .unwrap();
    (ctx, dir)
}

#[test]
fn children_declaration_order() {
    let (ctx, _dir) = load_introspection_context();
    let module = ctx.schema(introspection_module()).unwrap();
    let children: Vec<_> = module.top_level().collect();
    let names: Vec<_> = children.iter().map(|n| n.name().to_string()).collect();
    assert_eq!(names, vec!["top"]);

    let top = module.find_path("/cambium-introspection-demo:top").unwrap();
    let top_children: Vec<_> = top.children().collect();
    let names: Vec<_> = top_children.iter().map(|n| n.name().to_string()).collect();
    assert_eq!(
        names,
        vec![
            "direct-leaf",
            "grouped-leaf",
            "grouped-container",
            "rw-flag",
            "multi-defaults",
            "ro-counter",
            "deprecated-leaf",
            "mandatory-leaf",
            "presence-box",
            "typed-leaf-list",
            "keyed-list",
            "all-builtins",
            "dec64",
            "status-enum",
            "flags-bits",
            "uni",
            "ranged-int",
            "ranged-dec64",
            "constrained-string",
            "idref",
            "ref-to-name",
            "preference",
            "raw-data",
            "reset",
            "post-action",
        ]
    );
}

#[test]
fn schema_node_config_status_mandatory() {
    let (ctx, _dir) = load_introspection_context();
    let module = ctx.schema(introspection_module()).unwrap();
    let top = module.find_path("/cid:top").unwrap();

    let rw = top
        .children()
        .find(|n| n.name() == "rw-flag")
        .expect("rw-flag");
    assert_eq!(rw.config(), Config::Rw);
    assert_eq!(rw.status(), Status::Current);
    assert!(!rw.is_mandatory());

    let ro = top
        .children()
        .find(|n| n.name() == "ro-counter")
        .expect("ro-counter");
    assert_eq!(ro.config(), Config::Ro);

    let dep = top
        .children()
        .find(|n| n.name() == "deprecated-leaf")
        .expect("deprecated-leaf");
    assert_eq!(dep.status(), Status::Deprecated);

    let mand = top
        .children()
        .find(|n| n.name() == "mandatory-leaf")
        .expect("mandatory-leaf");
    assert!(mand.is_mandatory());

    let pc = top
        .children()
        .find(|n| n.name() == "presence-box")
        .expect("presence-box");
    assert!(pc.is_presence_container());

    let inner = pc.children().find(|n| n.name() == "inner").expect("inner");
    assert_eq!(inner.units(), Some("packets"));

    let ll = top
        .children()
        .find(|n| n.name() == "typed-leaf-list")
        .expect("typed-leaf-list");
    assert_eq!(ll.ordered_by(), OrderedBy::User);
    assert_eq!(ll.min_elements(), Some(1));
    assert_eq!(ll.max_elements(), Some(10));

    // Regression: libyang sets LYS_ORDBY_SYSTEM (0x80) on every system-ordered
    // (the default) list/leaf-list in the compiled tree, and that bit aliases
    // LYS_PRESENCE (0x80). The presence read must be gated to containers, so a
    // system-ordered keyed-list must NOT report as a presence container.
    let kl = top
        .children()
        .find(|n| n.name() == "keyed-list")
        .expect("keyed-list");
    assert!(!kl.is_presence_container());
}

#[test]
fn schema_node_default_values() {
    let (ctx, _dir) = load_introspection_context();
    let module = ctx.schema(introspection_module()).unwrap();
    let top = module.find_path("/cid:top").unwrap();
    let children = top.children();

    let rw = children.lookup("rw-flag").expect("rw-flag");
    assert_eq!(rw.default_value(), Some("true"));
    assert_eq!(rw.default_values(), vec!["true"]);

    let multi = top
        .children()
        .lookup("multi-defaults")
        .expect("multi-defaults");
    assert_eq!(multi.default_value(), Some("alpha"));
    assert_eq!(
        multi.default_values(),
        vec!["alpha", "beta", "gamma"],
        "leaf-list defaults must stay in declaration order"
    );

    let inner = top
        .children()
        .lookup("presence-box")
        .expect("presence-box")
        .children()
        .lookup("inner")
        .expect("inner");
    assert_eq!(inner.default_value(), None);
    assert!(inner.default_values().is_empty());
}

#[test]
fn must_when_introspection() {
    let (ctx, _dir) = load_introspection_context();
    let module = ctx.schema(introspection_module()).unwrap();
    let top = module.find_path("/cid:top").unwrap();

    let mand = top
        .children()
        .lookup("mandatory-leaf")
        .expect("mandatory-leaf");
    let musts = mand.musts();
    assert_eq!(musts.len(), 1);
    let must = &musts[0];
    assert_eq!(must.expression(), "../rw-flag = 'true'");
    assert_eq!(must.error_message(), Some("rw-flag must be true"));
    assert_eq!(must.error_app_tag(), Some("must-rw-flag"));
    assert_eq!(must.description(), None);
    assert_eq!(must.reference(), None);

    let presence = top.children().lookup("presence-box").expect("presence-box");
    let whens = presence.whens();
    assert_eq!(whens.len(), 1);
    let when = &whens[0];
    assert_eq!(when.expression(), "../rw-flag = 'true'");
    assert_eq!(
        when.description(),
        Some("Only present when rw-flag is true")
    );
    assert_eq!(when.reference(), Some("RFC 6020 when statement"));

    let rw = top.children().lookup("rw-flag").expect("rw-flag");
    assert!(rw.musts().is_empty());
    assert!(rw.whens().is_empty());
}

#[test]
fn schema_node_extensions() {
    let (ctx, _dir) = load_introspection_context();
    let module = ctx.schema(introspection_module()).unwrap();
    let top = module.find_path("/cid:top").unwrap();
    let rw = top.children().lookup("rw-flag").expect("rw-flag");

    let extensions = rw.extensions();
    assert_eq!(extensions.len(), 2);
    let names: Vec<_> = extensions
        .iter()
        .map(|ext| ext.name().to_string())
        .collect();
    assert_eq!(names, vec!["flag-foo", "camelcase-name"]);

    let camel = rw.extension("camelcase-name").expect("camelcase-name");
    assert_eq!(camel.name(), "camelcase-name");
    assert_eq!(camel.argument(), Some("fooBar"));
    assert_eq!(camel.module_name(), introspection_module());

    let flag = rw.extension("flag-foo").expect("flag-foo");
    assert_eq!(flag.argument(), None);

    assert!(rw.extension("missing").is_none());
    assert!(top.extensions().is_empty());
}

#[test]
fn list_keys_as_nodes() {
    let (ctx, _dir) = load_introspection_context();
    let module = ctx.schema(introspection_module()).unwrap();
    let top = module.find_path("/cid:top").unwrap();
    let list = top
        .children()
        .find(|n| n.name() == "keyed-list")
        .expect("keyed-list");

    let keys: Vec<_> = list.list_keys().collect();
    let names: Vec<_> = keys.iter().map(|n| n.name().to_string()).collect();
    assert_eq!(names, vec!["name", "color"]);
    assert_eq!(list.key_names(), vec!["name", "color"]);
}

#[test]
fn list_unique_constraints() {
    let (ctx, _dir) = load_introspection_context();
    let module = ctx.schema(introspection_module()).unwrap();
    let top = module.find_path("/cid:top").unwrap();
    let list = top.children().lookup("keyed-list").expect("keyed-list");

    let uniques = list.unique_constraints();
    assert_eq!(uniques.len(), 1);
    let leafs = uniques[0].leafs();
    let names: Vec<_> = leafs.iter().map(|node| node.name().to_string()).collect();
    assert_eq!(names, vec!["name", "extra"]);

    let name_leaf = list.children().lookup("name").expect("name");
    assert!(name_leaf.unique_constraints().is_empty());
}

#[test]
fn schema_navigation_helpers() {
    let (ctx, _dir) = load_introspection_context();
    let module = ctx.schema(introspection_module()).unwrap();
    let top = module.find_path("/cid:top").unwrap();
    let presence = top.children().lookup("presence-box").expect("presence-box");
    let inner = presence.children().lookup("inner").expect("inner");

    let root = top.parent().expect("top parent");
    assert_eq!(root.kind(), SchemaNodeKind::Module);
    assert_eq!(root.path(), "/cambium-introspection-demo");
    assert!(root.parent().is_none());
    assert!(top.ancestors().is_empty());

    let parent = inner.parent().expect("inner parent");
    assert_eq!(parent.name(), "presence-box");
    let ancestor_names: Vec<_> = inner
        .ancestors()
        .iter()
        .map(|node| node.name().to_string())
        .collect();
    assert_eq!(ancestor_names, vec!["top", "presence-box"]);

    assert!(top.children().lookup("ro-counter").is_some());
    assert!(top.children().lookup("not-there").is_none());
}

#[test]
fn grouping_origin_introspection() {
    let (ctx, _dir) = load_introspection_context();
    let module = ctx.schema(introspection_module()).unwrap();
    let top = module.find_path("/cid:top").unwrap();

    let direct = top.children().lookup("direct-leaf").expect("direct-leaf");
    assert_eq!(direct.grouping_origin(), None);

    let grouped = top.children().lookup("grouped-leaf").expect("grouped-leaf");
    assert_eq!(grouped.grouping_origin(), Some("common-grouping"));

    let container = top
        .children()
        .lookup("grouped-container")
        .expect("grouped-container");
    assert_eq!(container.grouping_origin(), Some("common-grouping"));

    let nested = container
        .children()
        .lookup("nested-leaf")
        .expect("nested-leaf");
    assert_eq!(nested.grouping_origin(), Some("common-grouping"));
}

#[test]
fn schema_node_predicates_and_namespace() {
    let (ctx, _dir) = load_introspection_context();
    let module = ctx.schema(introspection_module()).unwrap();
    let top = module.find_path("/cid:top").unwrap();
    let children = top.children();

    let rw = children.lookup("rw-flag").expect("rw-flag");
    assert!(rw.is_leaf());
    assert!(!rw.is_leaf_list());
    assert!(!rw.is_dir());
    assert!(!rw.read_only());

    let ro = top.children().lookup("ro-counter").expect("ro-counter");
    assert!(ro.read_only());
    assert_eq!(ro.namespace(), "urn:cambium:introspection");

    let ll = top
        .children()
        .lookup("typed-leaf-list")
        .expect("typed-leaf-list");
    assert!(ll.is_leaf_list());
    assert!(!ll.is_leaf());

    let list = top.children().lookup("keyed-list").expect("keyed-list");
    assert!(list.is_list());
    assert!(list.is_dir());

    let choice = top.children().lookup("preference").expect("preference");
    assert!(choice.is_choice());
    assert!(choice.is_dir());
    assert!(!choice.is_choice_descendant());
    let primary = choice.children().lookup("primary").expect("primary");
    assert!(primary.is_case());
    assert!(primary.is_dir());
    assert!(primary.is_choice_descendant());
    let primary_name = primary
        .children()
        .lookup("primary-name")
        .expect("primary-name");
    assert!(primary_name.is_choice_descendant());

    let action = top.children().lookup("reset").expect("reset");
    assert!(action.is_action());
    assert!(action.is_dir());
    assert!(!action.is_choice_descendant());
    let input = action.children().lookup("input").expect("input");
    let output = action.children().lookup("output").expect("output");
    assert_eq!(input.kind(), SchemaNodeKind::Input);
    assert_eq!(output.kind(), SchemaNodeKind::Output);
    assert!(input.is_dir());
    assert!(output.is_dir());

    assert!(top.is_container());
    assert_eq!(top.namespace(), "urn:cambium:introspection");
}

#[test]
fn module_operation_views() {
    let (ctx, _dir) = load_introspection_context();
    let module = ctx.schema(introspection_module()).unwrap();

    let rpcs: Vec<_> = module.rpcs().collect();
    assert_eq!(rpcs.len(), 1);
    assert_eq!(rpcs[0].name(), "reboot");
    assert!(rpcs[0].is_rpc());
    assert!(!rpcs[0].is_action());
    assert!(!rpcs[0].is_notification());

    let notifications: Vec<_> = module.notifications().collect();
    assert_eq!(notifications.len(), 1);
    assert_eq!(notifications[0].name(), "event");
    assert!(notifications[0].is_notification());

    assert!(module.actions().is_empty());

    let top = module.find_path("/cid:top").unwrap();
    let reset = top.children().lookup("reset").expect("reset");
    assert!(reset.is_action());
}

#[test]
fn schema_node_kind_anyxml() {
    let (ctx, _dir) = load_introspection_context();
    let module = ctx.schema(introspection_module()).unwrap();
    let top = module.find_path("/cid:top").unwrap();
    // anyxml's nodetype (0x20) is a distinct value from anydata (0x60); matching
    // only 0x60 misclassifies anyxml as Unknown. Both surface as AnyData.
    let anyx = top
        .children()
        .find(|n| n.name() == "raw-data")
        .expect("raw-data");
    assert_eq!(anyx.kind(), SchemaNodeKind::AnyData);
}

#[test]
fn module_metadata() {
    let (ctx, _dir) = load_introspection_context();
    let module = ctx.schema(introspection_module()).unwrap();
    assert_eq!(module.name(), "cambium-introspection-demo");
    assert_eq!(module.namespace(), "urn:cambium:introspection");
    assert_eq!(module.prefix(), "cid");
    assert_eq!(module.revision(), Some("2026-06-13"));
    assert!(module.is_implemented());
}

#[test]
fn module_imports_and_prefix_resolution() {
    let dir = temp_module_dir();
    atomic_write(
        &dir.join("cambium-import-target@2026-06-14.yang"),
        r#"module cambium-import-target {
    yang-version 1.1;
    namespace "urn:cambium:import-target";
    prefix cit;
    revision 2026-06-14;

    leaf target { type string; }
}
"#,
    );
    atomic_write(
        &dir.join("cambium-import-user.yang"),
        r#"module cambium-import-user {
    yang-version 1.1;
    namespace "urn:cambium:import-user";
    prefix ciu;

    import cambium-import-target {
        prefix tgt;
        revision-date 2026-06-14;
    }

    leaf ref { type leafref { path "/tgt:target"; } }
}
"#,
    );

    let ctx = ContextBuilder::new(Default::default())
        .unwrap()
        .search_path(&dir)
        .unwrap()
        .load_module("cambium-import-user", None, &[])
        .unwrap()
        .build()
        .unwrap();

    let module = ctx.schema("cambium-import-user").unwrap();
    let imports = module.imports();
    assert_eq!(imports.len(), 1);
    assert_eq!(imports[0].prefix(), "tgt");
    assert_eq!(imports[0].name(), "cambium-import-target");
    assert_eq!(imports[0].revision(), Some("2026-06-14"));

    assert_eq!(
        module.resolve_prefix("").unwrap().name(),
        "cambium-import-user"
    );
    assert_eq!(
        module.resolve_prefix("ciu").unwrap().name(),
        "cambium-import-user"
    );
    assert_eq!(
        module.resolve_prefix("tgt").unwrap().name(),
        "cambium-import-target"
    );
    assert!(module.resolve_prefix("missing").is_none());
}

#[test]
fn module_augment_deviation_provenance() {
    let (ctx, _dir) = load_provenance_context();
    let target = ctx.schema("cambium-provenance-target").unwrap();
    assert_eq!(target.augmented_by(), vec!["cambium-provenance-augment"]);
    assert_eq!(target.deviated_by(), vec!["cambium-provenance-deviate"]);

    let augment = ctx.schema("cambium-provenance-augment").unwrap();
    assert!(augment.augmented_by().is_empty());
    assert!(augment.deviated_by().is_empty());

    let deviate = ctx.schema("cambium-provenance-deviate").unwrap();
    assert!(deviate.augmented_by().is_empty());
    assert!(deviate.deviated_by().is_empty());

    let top = target.find_path("/cpt:top").unwrap();
    let augmented = top.children().lookup("b").expect("augmented leaf");
    assert_eq!(augmented.module().name(), "cambium-provenance-augment");
    assert_eq!(augmented.namespace(), "urn:cambium:provaugment");
}

#[test]
fn module_deviations_metadata() {
    let (ctx, _dir) = load_deviation_context();
    let source = ctx.schema(deviation_source_module()).unwrap();

    let devs = source.deviations();
    assert_eq!(devs.len(), 7);

    assert_eq!(devs[0].target_path(), "/cdt:top/cdt:metric");
    assert_eq!(devs[0].source_module(), deviation_source_module());
    assert_eq!(devs[0].deviation_type(), "replace");
    assert_eq!(devs[0].property(), "type");
    assert_eq!(devs[0].new_value(), "uint32");
    assert_eq!(devs[0].description(), Some("Make metric numeric."));
    assert_eq!(devs[0].reference(), Some("RFC 6020 deviation"));

    assert_eq!(devs[1].target_path(), "/cdt:top/cdt:enabled");
    assert_eq!(devs[1].deviation_type(), "not-supported");
    assert_eq!(devs[1].property(), "");
    assert_eq!(devs[1].new_value(), "");

    assert_eq!(devs[2].target_path(), "/cdt:top/cdt:label");
    assert_eq!(devs[2].deviation_type(), "add");
    assert_eq!(devs[2].property(), "units");
    assert_eq!(devs[2].new_value(), "meters");

    assert_eq!(devs[3].target_path(), "/cdt:top/cdt:defaulted");
    assert_eq!(devs[3].deviation_type(), "replace");
    assert_eq!(devs[3].property(), "default");
    assert_eq!(devs[3].new_value(), "");

    assert_eq!(devs[4].target_path(), "/cdt:top/cdt:samples");
    assert_eq!(devs[4].deviation_type(), "replace");
    assert_eq!(devs[4].property(), "min-elements");
    assert_eq!(devs[4].new_value(), "2");

    assert_eq!(devs[5].target_path(), "/cdt:top/cdt:samples");
    assert_eq!(devs[5].deviation_type(), "replace");
    assert_eq!(devs[5].property(), "max-elements");
    assert_eq!(devs[5].new_value(), "4");

    assert_eq!(devs[6].target_path(), "/cdt:top/cdt:records");
    assert_eq!(devs[6].deviation_type(), "replace");
    assert_eq!(devs[6].property(), "max-elements");
    assert_eq!(devs[6].new_value(), "unbounded");
}

#[test]
fn schema_node_deviation_provenance() {
    let (ctx, _dir) = load_deviation_context();
    let target = ctx.schema(deviation_target_module()).unwrap();
    let top = target.find_path("/cdt:top").unwrap();

    let names: Vec<_> = top
        .children()
        .map(|child| child.name().to_string())
        .collect();
    assert_eq!(
        names,
        vec!["metric", "label", "defaulted", "samples", "records"]
    );
    assert!(target.find_path("/cdt:top/cdt:enabled").is_err());

    let metric = top.children().lookup("metric").unwrap();
    let provs = metric.deviation_provenance();
    assert_eq!(provs.len(), 1);
    assert_eq!(provs[0].deviation_type(), "replace");
    assert_eq!(provs[0].property(), "type");
    assert_eq!(provs[0].new_value(), "uint32");
    assert_eq!(metric.leaf_type().unwrap().base(), BaseType::Uint32);

    let label = top.children().lookup("label").unwrap();
    let provs = label.deviation_provenance();
    assert_eq!(provs.len(), 1);
    assert_eq!(provs[0].deviation_type(), "add");
    assert_eq!(provs[0].property(), "units");
    assert_eq!(provs[0].new_value(), "meters");
    assert_eq!(label.units(), Some("meters"));

    let defaulted = top.children().lookup("defaulted").unwrap();
    assert_eq!(defaulted.default_value(), Some(""));

    let samples = top.children().lookup("samples").unwrap();
    assert_eq!(samples.min_elements(), Some(2));
    assert_eq!(samples.max_elements(), Some(4));

    let records = top.children().lookup("records").unwrap();
    assert_eq!(records.max_elements(), None);

    assert!(top.deviation_provenance().is_empty());

    let source = ctx.schema(deviation_source_module()).unwrap();
    let source_metric = source.find_path("/cds:top/cds:metric").unwrap();
    assert!(source_metric.deviation_provenance().is_empty());
}

#[test]
fn data_children_flatten_choice_cases() {
    let dir = write_module(
        "cambium-choice-demo",
        r#"module cambium-choice-demo {
    yang-version 1.1;
    namespace "urn:cambium:choice-demo";
    prefix ccd;

    container root {
        leaf before { type string; }
        choice mode {
            case primary {
                leaf first { type string; }
            }
            case secondary {
                leaf second { type string; }
            }
        }
        leaf after { type string; }
    }
}
"#,
    );
    let ctx = ContextBuilder::new(Default::default())
        .unwrap()
        .search_path(&dir)
        .unwrap()
        .load_module("cambium-choice-demo", None, &[])
        .unwrap()
        .build()
        .unwrap();

    let module = ctx.schema("cambium-choice-demo").unwrap();
    let root = module.find_path("/ccd:root").unwrap();
    let children: Vec<_> = root.children().map(|n| n.name().to_string()).collect();
    assert_eq!(children, vec!["before", "mode", "after"]);

    let flat: Vec<_> = root
        .data_children(true)
        .map(|n| n.name().to_string())
        .collect();
    assert_eq!(flat, vec!["before", "first", "second", "after"]);
}

#[test]
fn leaf_type_base_kind_all_builtins() {
    let (ctx, _dir) = load_introspection_context();
    let module = ctx.schema(introspection_module()).unwrap();
    let top = module.find_path("/cid:top").unwrap();

    let rw = top.children().find(|n| n.name() == "rw-flag").unwrap();
    assert_eq!(rw.leaf_type().unwrap().base(), BaseType::Boolean);

    let ro = top.children().find(|n| n.name() == "ro-counter").unwrap();
    assert_eq!(ro.leaf_type().unwrap().base(), BaseType::Uint64);

    let all = top.children().find(|n| n.name() == "all-builtins").unwrap();
    assert_eq!(all.leaf_type().unwrap().base(), BaseType::Int64);

    let inner = top
        .children()
        .find(|n| n.name() == "presence-box")
        .unwrap()
        .children()
        .find(|n| n.name() == "inner")
        .unwrap();
    assert_eq!(inner.leaf_type().unwrap().base(), BaseType::String);
}

#[test]
fn leaf_type_decimal64_fraction_digits() {
    let (ctx, _dir) = load_introspection_context();
    let module = ctx.schema(introspection_module()).unwrap();
    let top = module.find_path("/cid:top").unwrap();
    let dec = top.children().find(|n| n.name() == "dec64").unwrap();
    let info = dec.leaf_type().unwrap();
    match info.resolved() {
        ResolvedType::Decimal64 {
            fraction_digits, ..
        } => {
            assert_eq!(fraction_digits.value(), 4);
        }
        other => panic!("expected decimal64, got {other:?}"),
    }
}

#[test]
fn leaf_type_enum_values_ordered() {
    let (ctx, _dir) = load_introspection_context();
    let module = ctx.schema(introspection_module()).unwrap();
    let top = module.find_path("/cid:top").unwrap();
    let enm = top.children().find(|n| n.name() == "status-enum").unwrap();
    let info = enm.leaf_type().unwrap();
    match info.resolved() {
        ResolvedType::Enumeration(def) => {
            let names: Vec<_> = def.values().iter().map(|v| v.name().to_string()).collect();
            let values: Vec<_> = def.values().iter().map(|v| v.value()).collect();
            assert_eq!(names, vec!["up", "down", "unknown"]);
            assert_eq!(values, vec![1, 2, 0]);
        }
        other => panic!("expected enumeration, got {other:?}"),
    }
}

#[test]
fn leaf_type_bits_positions_ordered() {
    let (ctx, _dir) = load_introspection_context();
    let module = ctx.schema(introspection_module()).unwrap();
    let top = module.find_path("/cid:top").unwrap();
    let bits = top.children().find(|n| n.name() == "flags-bits").unwrap();
    let info = bits.leaf_type().unwrap();
    match info.resolved() {
        ResolvedType::Bits(def) => {
            let names: Vec<_> = def.values().iter().map(|v| v.name().to_string()).collect();
            assert_eq!(names, vec!["read", "write", "execute"]);
        }
        other => panic!("expected bits, got {other:?}"),
    }
}

#[test]
fn leaf_type_union_members_recursive() {
    let (ctx, _dir) = load_introspection_context();
    let module = ctx.schema(introspection_module()).unwrap();
    let top = module.find_path("/cid:top").unwrap();
    let uni = top.children().find(|n| n.name() == "uni").unwrap();
    let info = uni.leaf_type().unwrap();
    match info.resolved() {
        ResolvedType::Union(members) => {
            let bases: Vec<_> = members.iter().map(|m| m.base()).collect();
            assert_eq!(bases, vec![BaseType::String, BaseType::Int32]);
        }
        other => panic!("expected union, got {other:?}"),
    }
}

#[test]
fn list_unbounded_max_elements_is_none() {
    // A list with no `max-elements` is unbounded. NOTE: although tree_schema.h
    // comments the field as "0 means unbounded", the *compiled* `lysc_node_list.max`
    // empirically stores `UINT32_MAX` for an unbounded list (verified against the
    // pinned libyang). The adapter maps that sentinel to `None`; this guard pins it
    // so a future libyang bump that changed the sentinel would fail loudly.
    let (ctx, _dir) = load_introspection_context();
    let module = ctx.schema(introspection_module()).unwrap();
    let top = module.find_path("/cid:top").unwrap();
    let list = top
        .children()
        .find(|n| n.name() == "keyed-list")
        .expect("keyed-list");
    assert_eq!(list.min_elements(), None);
    assert_eq!(list.max_elements(), None);
}

#[test]
fn leaf_type_int_range() {
    let (ctx, _dir) = load_introspection_context();
    let module = ctx.schema(introspection_module()).unwrap();
    let top = module.find_path("/cid:top").unwrap();
    let leaf = top.children().find(|n| n.name() == "ranged-int").unwrap();
    let info = leaf.leaf_type().unwrap();
    match info.resolved() {
        ResolvedType::Int { kind, range } => {
            assert_eq!(*kind, IntKind::I32);
            let parts = range.as_deref().expect("expected a range");
            assert_eq!(parts.len(), 2);
            assert_eq!(parts[0].min(), "1");
            assert_eq!(parts[0].max(), "10");
            assert_eq!(parts[1].min(), "20");
            assert_eq!(parts[1].max(), "2147483647");
        }
        other => panic!("expected int, got {other:?}"),
    }
}

#[test]
fn leaf_type_decimal64_range() {
    let (ctx, _dir) = load_introspection_context();
    let module = ctx.schema(introspection_module()).unwrap();
    let top = module.find_path("/cid:top").unwrap();
    let leaf = top.children().find(|n| n.name() == "ranged-dec64").unwrap();
    let info = leaf.leaf_type().unwrap();
    match info.resolved() {
        ResolvedType::Decimal64 {
            fraction_digits,
            range,
        } => {
            assert_eq!(fraction_digits.value(), 2);
            let parts = range.as_deref().expect("expected a range");
            assert_eq!(parts.len(), 1);
            assert_eq!(parts[0].min(), "0.00");
            assert_eq!(parts[0].max(), "100.00");
        }
        other => panic!("expected decimal64, got {other:?}"),
    }
}

#[test]
fn leaf_type_string_length() {
    let (ctx, _dir) = load_introspection_context();
    let module = ctx.schema(introspection_module()).unwrap();
    let top = module.find_path("/cid:top").unwrap();
    let leaf = top
        .children()
        .find(|n| n.name() == "constrained-string")
        .unwrap();
    let info = leaf.leaf_type().unwrap();
    match info.resolved() {
        ResolvedType::StringType { length, .. } => {
            let parts = length.as_deref().expect("expected a length");
            assert_eq!(parts.len(), 1);
            assert_eq!(parts[0].min(), "1");
            assert_eq!(parts[0].max(), "255");
        }
        other => panic!("expected string, got {other:?}"),
    }
}

#[test]
fn leaf_type_string_patterns() {
    let (ctx, _dir) = load_introspection_context();
    let module = ctx.schema(introspection_module()).unwrap();
    let top = module.find_path("/cid:top").unwrap();
    let leaf = top
        .children()
        .find(|n| n.name() == "constrained-string")
        .unwrap();
    let info = leaf.leaf_type().unwrap();
    match info.resolved() {
        ResolvedType::StringType { patterns, .. } => {
            assert_eq!(patterns.len(), 2);
            assert_eq!(patterns[0].regex(), "[a-zA-Z0-9_-]+");
            assert_eq!(patterns[0].error_app_tag(), Some("my-tag"));
            assert!(!patterns[0].is_inverted());
            assert_eq!(patterns[1].regex(), "^foo.*");
            assert!(patterns[1].is_inverted());
        }
        other => panic!("expected string, got {other:?}"),
    }
}

#[test]
fn identity_derived_closure() {
    let (ctx, _dir) = load_introspection_context();
    let module = ctx.schema(introspection_module()).unwrap();
    let base = module
        .identities()
        .find(|i| i.name() == "base-id")
        .expect("base-id identity");
    let derived_names: Vec<_> = base
        .derived()
        .iter()
        .map(|i| i.name().to_string())
        .collect();
    assert_eq!(derived_names, vec!["mid-id", "leaf-id"]);
}

#[test]
fn leafref_realtype_resolves() {
    let (ctx, _dir) = load_introspection_context();
    let module = ctx.schema(introspection_module()).unwrap();
    let top = module.find_path("/cid:top").unwrap();
    let leaf = top.children().find(|n| n.name() == "ref-to-name").unwrap();
    let info = leaf.leaf_type().unwrap();
    match info.resolved() {
        ResolvedType::LeafRef {
            path: Some(path),
            target: Some(target),
            realtype: Some(realtype),
            require_instance,
        } => {
            assert_eq!(path, "/cid:top/keyed-list/name");
            assert_eq!(
                target.path(),
                "/cambium-introspection-demo/top/keyed-list/name"
            );
            assert_eq!(realtype.base(), BaseType::String);
            assert!(require_instance);
        }
        other => panic!("expected leafref with realtype, got {other:?}"),
    }
}
