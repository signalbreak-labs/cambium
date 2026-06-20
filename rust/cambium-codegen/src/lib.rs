//! YANG → typed Rust/Go struct code generator (field-order manifest + deterministic serializer).
//!
//! The v2 emitter is driven from the rich `cambium_core::schema` API
//! (`Module`/`SchemaNodeRef`/`TypeInfo`/`ResolvedType`). It emits typed structs,
//! per-struct field-order manifests, and a self-contained native XML serializer
//! that is byte-identical to libyang for the same data. `generate_rust` is kept
//! as a thin wrapper over the new `generate` entry point.

#![deny(missing_docs)]

use std::collections::{BTreeMap, HashSet};

use cambium_core::{
    Context, IntKind, Module, OrderedBy, RangeBound, ResolvedType, SchemaNodeKind, SchemaNodeRef,
    TypeInfo,
};

/// Target language for code generation.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum Lang {
    /// Rust.
    Rust,
    /// Go.
    Go,
}

/// Options controlling the generated output.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub struct CodegenOpts {
    /// Target language.
    pub lang: Lang,
    /// Deduplicate structs emitted from reusable groupings.
    ///
    /// This option is currently unsupported from the compiled-schema IR and
    /// returns an error when enabled.
    pub dedup_groupings: bool,
    /// Emit validation helpers on generated structs.
    pub with_validate: bool,
    /// Emit ygnmi-style fluent path builders.
    pub with_path_builder: bool,
}

impl Default for CodegenOpts {
    fn default() -> Self {
        Self {
            lang: Lang::Rust,
            dedup_groupings: false,
            with_validate: false,
            with_path_builder: false,
        }
    }
}

/// The result of a successful code-generation run.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct GeneratedModule {
    /// Generated source text.
    pub source: String,
    /// Per-struct field-order manifest: keys first, then declaration order.
    pub field_order: BTreeMap<String, Vec<String>>,
}

/// Errors raised by the code generator.
#[derive(Debug, thiserror::Error)]
pub enum Error {
    /// An underlying core operation failed.
    #[error("core error: {0}")]
    Core(#[from] cambium_core::Error),
    /// An option is not yet implemented.
    #[error("unsupported codegen option: {0}")]
    UnsupportedOption(&'static str),
    /// A generated identifier collision could not be resolved.
    #[error("identifier collision: yang names {a:?} and {b:?} both map to {ident:?}")]
    IdentCollision {
        /// First colliding YANG name.
        a: String,
        /// Second colliding YANG name.
        b: String,
        /// The shared generated identifier.
        ident: String,
    },
}

/// Codegen result type.
pub type Result<T> = std::result::Result<T, Error>;

/// Generate a source file for `module` according to `opts`.
///
/// The output is deterministic for a given schema. The returned
/// `GeneratedModule` includes both the source and the per-struct field-order
/// manifest that the native serializer walks.
pub fn generate(ctx: &Context, module: &str, opts: CodegenOpts) -> Result<GeneratedModule> {
    if opts.lang != Lang::Rust {
        return Err(Error::UnsupportedOption("only Lang::Rust is supported"));
    }
    if opts.dedup_groupings {
        return Err(Error::UnsupportedOption(
            "dedup_groupings is not supported from the compiled lysc IR (needs lysp grouping provenance)",
        ));
    }

    let module_handle = ctx.schema(module)?;
    let mut emitter = Emitter::new(module_handle, module);
    emitter.emit()?;
    let source = emitter.output();

    Ok(GeneratedModule {
        source,
        field_order: emitter.field_order,
    })
}

/// Generate a Rust source file for the implemented module named `module`.
///
/// This is the backwards-compatible v1 entry point. It delegates to
/// [`generate`] with `Lang::Rust` and returns only the generated source.
///
/// This compatibility wrapper keeps the original v1 `SchemaTree` output shape.
/// New callers should use [`generate`] to get the schema-IR-driven emitter and
/// field-order manifest.
pub fn generate_rust(ctx: &Context, module: &str) -> cambium_core::Result<String> {
    fallback::generate_rust(ctx, module)
}

mod fallback;

/// How a generated leaf type is encoded in JSON_IETF.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
enum JsonKind {
    /// A JSON string (strings, enums, identityrefs, bits, quoted numbers).
    String,
    /// A bare JSON number (int8..int32 / uint8..uint32).
    BareNumber,
    /// A number encoded as a quoted string (int64 / uint64 / decimal64).
    QuotedNumber,
    /// A bare JSON boolean.
    Bool,
}

type UnionVariantMeta = (String, String, bool, bool, bool, bool, bool, JsonKind);

struct Emitter<'ctx> {
    module: Module<'ctx>,
    module_name: String,
    module_pascal: String,
    output: String,
    helpers: Vec<String>,
    emitted_decimal64: bool,
    emitted_user_ordered_vec: bool,
    emitted_enums: HashSet<String>,
    emitted_bits: HashSet<String>,
    emitted_identityrefs: HashSet<String>,
    emitted_int_ranges: HashSet<String>,
    emitted_string_lengths: HashSet<String>,
    emitted_unions: HashSet<String>,
    emitted_instance_identifier: bool,
    needs_xml_escape: bool,
    needs_json_indent: bool,
    needs_json_escape: bool,
    /// Per-struct field-order manifest populated during emission.
    pub field_order: BTreeMap<String, Vec<String>>,
}

impl<'ctx> Emitter<'ctx> {
    fn new(module: Module<'ctx>, module_name: &str) -> Self {
        Self {
            module,
            module_name: module_name.to_string(),
            module_pascal: to_pascal_case(module_name),
            output: String::new(),
            helpers: Vec::new(),
            emitted_decimal64: false,
            emitted_user_ordered_vec: false,
            emitted_enums: HashSet::new(),
            emitted_bits: HashSet::new(),
            emitted_identityrefs: HashSet::new(),
            emitted_int_ranges: HashSet::new(),
            emitted_string_lengths: HashSet::new(),
            emitted_unions: HashSet::new(),
            emitted_instance_identifier: false,
            needs_xml_escape: false,
            needs_json_indent: false,
            needs_json_escape: false,
            field_order: BTreeMap::new(),
        }
    }

    fn output(&mut self) -> String {
        let mut out = String::new();
        out.push_str(&format!(
            "//! Generated data structures for YANG module `{}`.\n",
            self.module_name
        ));
        out.push_str("#![deny(missing_docs)]\n");
        out.push_str("#![deny(warnings)]\n\n");
        out.push_str(&format!(
            "// Generated by cambium-codegen for module {:?}.\n// Do not edit by hand.\n\n",
            self.module_name
        ));
        out.push_str(&self.output);
        if self.needs_xml_escape {
            out.push_str(XML_ESCAPE_HELPER);
            out.push('\n');
        }
        if self.needs_json_indent {
            out.push_str(JSON_INDENT_HELPER);
            out.push('\n');
        }
        if self.needs_json_escape {
            out.push_str(JSON_ESCAPE_HELPER);
            out.push('\n');
        }
        if !self.helpers.is_empty() {
            out.push_str(
                "// ----------------------------------------------------------------------------\n",
            );
            out.push_str("// Inline helper types used by the generated schema above.\n");
            out.push_str(
                "// ----------------------------------------------------------------------------\n",
            );
            for helper in &self.helpers {
                out.push_str(helper);
                out.push('\n');
            }
        }
        out
    }

    fn emit(&mut self) -> Result<()> {
        let module_pascal = self.module_pascal.clone();
        let root_children: Vec<_> = data_children(self.module.top_level());

        // Emit the shared CambiumStruct trait once per generated module.
        self.push("/// Trait implemented by every generated YANG data struct.");
        self.push("pub trait CambiumStruct {");
        self.push("\t/// Serialize this node and its descendants to XML.");
        self.push("\tfn to_xml(&self) -> String;");
        self.push("\t/// Serialize this node and its descendants to JSON_IETF.");
        self.push("\tfn to_json_ietf(&self) -> String;");
        self.push("}\n");

        // Recursively emit structs for nested containers and list entries.
        for child in &root_children {
            self.emit_struct_rec(*child, &module_pascal)?;
        }

        // Emit the synthetic module struct that owns the top-level data nodes.
        let root_fields = self.collect_fields(&module_pascal, &root_children)?;
        self.emit_struct_definition(&module_pascal, &root_fields);
        self.emit_field_order(&module_pascal, &root_fields);
        self.push("}\n");
        self.emit_root_serializer(&root_fields);
        self.emit_cambium_struct_impl(&module_pascal);

        Ok(())
    }

    fn emit_struct_rec(&mut self, node: SchemaNodeRef<'ctx>, prefix: &str) -> Result<()> {
        if !is_struct_kind(node.kind()) {
            return Ok(());
        }

        // Lists emit their entry struct; containers emit their own struct.
        let struct_name = if node.kind() == SchemaNodeKind::List {
            self.entry_name_for(prefix, node)
        } else {
            self.struct_name_for(prefix, node)
        };
        let ordered = ordered_children(node);
        let fields = self.collect_fields(&struct_name, &ordered)?;

        self.emit_struct_definition(&struct_name, &fields);
        self.emit_field_order(&struct_name, &fields);
        self.emit_struct_serializer(&struct_name, &fields);
        self.emit_cambium_struct_impl(&struct_name);

        // Recurse into nested containers and lists.
        for child in &ordered {
            if is_struct_kind(child.kind()) {
                self.emit_struct_rec(*child, &struct_name)?;
            }
        }

        Ok(())
    }

    fn collect_fields(
        &mut self,
        prefix: &str,
        children: &[SchemaNodeRef<'ctx>],
    ) -> Result<Vec<Field<'ctx>>> {
        let mut fields = Vec::with_capacity(children.len());
        let mut used_idents: HashSet<String> = HashSet::new();

        for child in children {
            let ident = safe_field_ident(child.name(), &mut used_idents)?;
            let (ty, optional, is_enum, is_bits, is_identityref, is_union, json_kind) =
                self.field_type(prefix, *child, &ident)?;
            let description = child.description().map(|s| s.to_string());
            fields.push(Field {
                node: *child,
                ident,
                ty,
                optional,
                is_enum,
                is_bits,
                is_identityref,
                is_union,
                json_kind,
                description,
            });
        }

        Ok(fields)
    }

    fn field_type(
        &mut self,
        prefix: &str,
        node: SchemaNodeRef<'ctx>,
        field_ident: &str,
    ) -> Result<(String, bool, bool, bool, bool, bool, JsonKind)> {
        match node.kind() {
            SchemaNodeKind::Leaf | SchemaNodeKind::LeafList => {
                let (base, is_enum, is_bits, is_identityref, is_union, json_kind) =
                    if let Some(info) = node.leaf_type() {
                        self.rust_type_for(prefix, node.name(), &info, field_ident)
                    } else {
                        (
                            "String".to_string(),
                            false,
                            false,
                            false,
                            false,
                            JsonKind::String,
                        )
                    };
                if node.kind() == SchemaNodeKind::LeafList {
                    // A leaf-list is multi-valued: a `Vec` (system-ordered) or
                    // `UserOrderedVec` (user-ordered) of the element type, never `Option`.
                    // `is_enum`/`is_bits`/`is_identityref`/`json_kind` keep describing
                    // the ELEMENT.
                    let container = if node.ordered_by() == OrderedBy::User {
                        self.ensure_user_ordered_vec();
                        "UserOrderedVec"
                    } else {
                        "Vec"
                    };
                    return Ok((
                        format!("{container}<{base}>"),
                        false,
                        is_enum,
                        is_bits,
                        is_identityref,
                        is_union,
                        json_kind,
                    ));
                }
                let optional = !node.is_mandatory() && !node.is_list_key();
                Ok((
                    base,
                    optional,
                    is_enum,
                    is_bits,
                    is_identityref,
                    is_union,
                    json_kind,
                ))
            }
            SchemaNodeKind::Container => {
                let struct_name = self.struct_name_for(prefix, node);
                let optional = node.is_presence_container();
                Ok((
                    struct_name,
                    optional,
                    false,
                    false,
                    false,
                    false,
                    JsonKind::String,
                ))
            }
            SchemaNodeKind::List => {
                let entry_name = self.entry_name_for(prefix, node);
                let container = if node.ordered_by() == OrderedBy::User {
                    self.ensure_user_ordered_vec();
                    "UserOrderedVec"
                } else {
                    "Vec"
                };
                Ok((
                    format!("{container}<{entry_name}>"),
                    false,
                    false,
                    false,
                    false,
                    false,
                    JsonKind::String,
                ))
            }
            _ => Ok((
                "String".to_string(),
                false,
                false,
                false,
                false,
                false,
                JsonKind::String,
            )),
        }
    }

    fn rust_type_for(
        &mut self,
        prefix: &str,
        yang_name: &str,
        info: &TypeInfo<'ctx>,
        field_ident: &str,
    ) -> (String, bool, bool, bool, bool, JsonKind) {
        match info.resolved() {
            ResolvedType::LeafRef {
                realtype: Some(rt), ..
            } => self.rust_type_for(prefix, yang_name, rt, field_ident),
            ResolvedType::LeafRef {
                realtype: None,
                target: Some(t),
                ..
            } => {
                if let Some(target_info) = t.leaf_type() {
                    self.rust_type_for(prefix, yang_name, &target_info, field_ident)
                } else {
                    (
                        "String".to_string(),
                        false,
                        false,
                        false,
                        false,
                        JsonKind::String,
                    )
                }
            }
            ResolvedType::LeafRef { realtype: None, .. } => (
                "String".to_string(),
                false,
                false,
                false,
                false,
                JsonKind::String,
            ),
            ResolvedType::Boolean => (
                "bool".to_string(),
                false,
                false,
                false,
                false,
                JsonKind::Bool,
            ),
            ResolvedType::Int { kind, range } => {
                let json_kind = match *kind {
                    IntKind::I64 | IntKind::U64 => JsonKind::QuotedNumber,
                    _ => JsonKind::BareNumber,
                };
                let rname = format!("{}{}Range", prefix, to_pascal_case(field_ident));
                self.ensure_int_range(&rname, *kind, range.as_deref());
                (rname, false, false, false, false, json_kind)
            }
            ResolvedType::Decimal64 { .. } => {
                self.ensure_decimal64();
                (
                    "Decimal64".to_string(),
                    false,
                    false,
                    false,
                    false,
                    JsonKind::QuotedNumber,
                )
            }
            ResolvedType::Enumeration(def) => {
                // Derive the enum TYPE name from the already-disambiguated field
                // ident, not the raw YANG name: two YANG names that collapse to the
                // same PascalCase (e.g. `foo-bar` and `foo_bar`) would otherwise
                // produce one enum name and silently clobber the second.
                let ename = format!("{}{}Enum", prefix, to_pascal_case(field_ident));
                self.ensure_enum(&ename, yang_name, def);
                (ename, true, false, false, false, JsonKind::String)
            }
            ResolvedType::Bits(def) => {
                let bname = format!("{}{}Bits", prefix, to_pascal_case(field_ident));
                self.ensure_bits(&bname, yang_name, def);
                (bname, false, true, false, false, JsonKind::String)
            }
            ResolvedType::IdentityRef { bases } => {
                let members = collect_identityref_members(bases);
                if members.is_empty() {
                    // No data-valid identity from an implemented module resolved.
                    // An empty enum is uninhabited and cannot derive Default, so
                    // degrade to a plain String rather than emit code that will
                    // not compile.
                    (
                        "String".to_string(),
                        false,
                        false,
                        false,
                        false,
                        JsonKind::String,
                    )
                } else {
                    let iname = format!("{}{}Enum", prefix, to_pascal_case(field_ident));
                    self.ensure_identityref(&iname, yang_name, &members);
                    (iname, false, false, true, false, JsonKind::String)
                }
            }
            ResolvedType::StringType { length, .. } => {
                if let Some(bounds) = length {
                    let lname = format!("{}{}Length", prefix, to_pascal_case(field_ident));
                    self.ensure_string_length(&lname, bounds);
                    // The length-bounded newtype exposes `.as_str()` just like a
                    // plain `String`, so JSON/XML emission needs no extra flag.
                    return (lname, false, false, false, false, JsonKind::String);
                }
                (
                    "String".to_string(),
                    false,
                    false,
                    false,
                    false,
                    JsonKind::String,
                )
            }
            ResolvedType::InstanceIdentifier { .. } => {
                self.ensure_instance_identifier();
                (
                    "InstanceIdentifier".to_string(),
                    false,
                    false,
                    false,
                    false,
                    JsonKind::String,
                )
            }
            ResolvedType::Binary { .. } | ResolvedType::Empty | ResolvedType::Unknown => (
                "String".to_string(),
                false,
                false,
                false,
                false,
                JsonKind::String,
            ),
            ResolvedType::Union(members) => {
                let uname = format!("{}{}Union", prefix, to_pascal_case(field_ident));
                self.ensure_union(&uname, yang_name, members, prefix, field_ident);
                (uname, false, false, false, true, JsonKind::String)
            }
            // Fallback for any future ResolvedType variants.
            _ => (
                "String".to_string(),
                false,
                false,
                false,
                false,
                JsonKind::String,
            ),
        }
    }

    fn ensure_decimal64(&mut self) {
        if self.emitted_decimal64 {
            return;
        }
        self.emitted_decimal64 = true;
        self.helpers.push(DECIMAL64_HELPER.to_string());
    }

    fn ensure_user_ordered_vec(&mut self) {
        if self.emitted_user_ordered_vec {
            return;
        }
        self.emitted_user_ordered_vec = true;
        self.helpers.push(USER_ORDERED_VEC_HELPER.to_string());
    }

    fn ensure_enum(&mut self, ename: &str, yang_name: &str, def: &cambium_core::EnumDef) {
        if !self.emitted_enums.insert(ename.to_string()) {
            return;
        }
        let mut used_variants: HashSet<String> = HashSet::new();
        let variants: Vec<_> = def
            .values()
            .iter()
            .map(|ev| {
                let vname = safe_variant_name(ev.name(), &mut used_variants);
                (ev.name().to_string(), vname, ev.value())
            })
            .collect();

        let mut variant_defs = Vec::new();
        for (idx, (yang_name, vname, value)) in variants.iter().enumerate() {
            let doc = format!("/// YANG enum `{yang_name}` (= {value}).");
            let default_attr = if idx == 0 { "\t#[default]\n" } else { "" };
            variant_defs.push(format!("{doc}\n{default_attr}\t{vname} = {value},"));
        }
        let variants_block = variant_defs.join("\n");

        let mut arms_as_name = Vec::new();
        let mut arms_from_name = Vec::new();
        for (yang_name, vname, _value) in &variants {
            arms_as_name.push(format!("\t\t\tSelf::{vname} => \"{yang_name}\","));
            arms_from_name.push(format!("\t\t\t\"{yang_name}\" => Some(Self::{vname}),"));
        }
        let as_name = arms_as_name.join("\n");
        let from_name = arms_from_name.join("\n");

        let helper = format!(
            "/// Generated YANG enumeration `{yang_name}`.\n\
             #[derive(Debug, Clone, Copy, Default, PartialEq, Eq)]\n\
             #[repr(i64)]\n\
             pub enum {ename} {{\n\
             {variants_block}\n\
             }}\n\n\
             impl {ename} {{\n\
             \t/// Return the canonical YANG name for this value.\n\
             \tpub fn as_name(&self) -> &'static str {{\n\
             \t\tmatch self {{\n\
             {as_name}\n\
             \t\t}}\n\
             \t}}\n\n\
             \t/// Parse a YANG enum name into the typed value.\n\
             \tpub fn from_name(name: &str) -> Option<Self> {{\n\
             \t\tmatch name {{\n\
             {from_name}\n\
             \t\t\t_ => None,\n\
             \t\t}}\n\
             \t}}\n\
             }}\n\n\
             impl std::fmt::Display for {ename} {{\n\
             \tfn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {{\n\
             \t\twrite!(f, \"{{}}\", self.as_name())\n\
             \t}}\n\
             }}"
        );
        self.helpers.push(helper);
    }

    fn ensure_bits(&mut self, bname: &str, yang_name: &str, def: &cambium_core::BitsDef) {
        if !self.emitted_bits.insert(bname.to_string()) {
            return;
        }

        let mut used_names: HashSet<String> = HashSet::new();
        let entries: Vec<_> = def
            .values()
            .iter()
            .map(|ev| {
                let name = ev.name().to_string();
                used_names.insert(name.clone());
                (name, ev.value())
            })
            .collect();

        let positions: Vec<_> = entries
            .iter()
            .map(|(name, value)| format!("\t(\"{name}\", {value}),"))
            .collect();
        let positions_block = positions.join("\n");

        let helper = format!(
            "/// Generated YANG bits type `{yang_name}`.
\
             #[derive(Debug, Clone, Default, PartialEq, Eq)]
\
             pub struct {bname} {{
\
             \tset: Vec<(&'static str, i64)>,
\
             }}

\
             impl {bname} {{
\
             \t/// Bit names and their positions, in schema declaration order.
\
             \tconst BIT_POSITIONS: &'static [(&'static str, i64)] = &[
\
             {positions_block}
\
             \t];

\
             \t/// Create a bits value from the names of the bits to set.
\
             \tpub fn new(names: &[&str]) -> Result<Self, String> {{
\
             \t\tlet mut set = Vec::with_capacity(names.len());
\
             \t\tfor name in names {{
\
             \t\t\tlet (static_name, pos) = Self::BIT_POSITIONS
\
             \t\t\t\t.iter()
\
             \t\t\t\t.find(|(n, _)| n == name)
\
             \t\t\t\t.map(|(n, p)| (*n, *p))
\
             \t\t\t\t.ok_or_else(|| format!(\"invalid bit name: {{}}\", name))?;
\
             \t\t\tif !set.iter().any(|(n, _)| *n == static_name) {{
\
             \t\t\t\tset.push((static_name, pos));
\
             \t\t\t}}
\
             \t\t}}
\
             \t\tOk(Self {{ set }})
\
             \t}}
\
             }}

\
             impl std::fmt::Display for {bname} {{
\
             \tfn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {{
\
             \t\tlet mut ordered: Vec<&(&'static str, i64)> = self.set.iter().collect();
\
             \t\tordered.sort_by_key(|item| item.1);
\
             \t\tfor (i, item) in ordered.iter().enumerate() {{
\
             \t\t\tif i > 0 {{
\
             \t\t\t\tf.write_str(\" \")?;
\
             \t\t\t}}
\
             \t\t\tf.write_str(item.0)?;
\
             \t\t}}
\
             \t\tOk(())
\
             \t}}
\
             }}"
        );
        self.helpers.push(helper);
    }

    fn ensure_identityref(
        &mut self,
        iname: &str,
        yang_name: &str,
        identities: &[(String, String, String, String, String)],
    ) {
        if !self.emitted_identityrefs.insert(iname.to_string()) {
            return;
        }

        let leaf_module = self.module_name.clone();

        let variants_block = identities
            .iter()
            .enumerate()
            .map(|(idx, (module, name, variant, _, _))| {
                let doc = format!("/// YANG identity `{name}` from module `{module}`.");
                let default_attr = if idx == 0 { "\t#[default]\n" } else { "" };
                format!("{doc}\n{default_attr}\t{variant},")
            })
            .collect::<Vec<_>>()
            .join("\n");

        let as_name_block = identities
            .iter()
            .map(|(_, name, variant, _, _)| format!("\t\t\tSelf::{variant} => \"{name}\","))
            .collect::<Vec<_>>()
            .join("\n");

        let as_json_name_block = identities
            .iter()
            .map(|(module, name, variant, _, _)| {
                if *module == leaf_module {
                    format!("\t\t\tSelf::{variant} => \"{name}\",")
                } else {
                    format!("\t\t\tSelf::{variant} => \"{module}:{name}\",")
                }
            })
            .collect::<Vec<_>>()
            .join("\n");

        let from_name_block = identities
            .iter()
            .flat_map(|(module, name, variant, _, _)| {
                let mut arms = Vec::new();
                arms.push(format!("\t\t\t\"{name}\" => Some(Self::{variant}),"));
                if *module != leaf_module {
                    arms.push(format!(
                        "\t\t\t\"{module}:{name}\" => Some(Self::{variant}),"
                    ));
                }
                arms
            })
            .collect::<Vec<_>>()
            .join("\n");

        let xml_prefix_ns_block = identities
            .iter()
            .map(|(module, _, variant, prefix, namespace)| {
                if *module == leaf_module {
                    format!("\t\t\tSelf::{variant} => None,")
                } else {
                    format!("\t\t\tSelf::{variant} => Some((\"{prefix}\", \"{namespace}\")),")
                }
            })
            .collect::<Vec<_>>()
            .join("\n");

        let xml_value_block = identities
            .iter()
            .map(|(module, name, variant, prefix, _)| {
                if *module == leaf_module {
                    format!("\t\t\tSelf::{variant} => cambium_xml_escape_text(\"{name}\"),")
                } else {
                    format!(
                        "\t\t\tSelf::{variant} => format!(\"{prefix}:{{}}\", cambium_xml_escape_text(\"{name}\")),"
                    )
                }
            })
            .collect::<Vec<_>>()
            .join("\n");

        let helper = format!(
            "/// Generated YANG identityref type `{yang_name}`.\n\
             #[derive(Debug, Clone, Copy, Default, PartialEq, Eq)]\n\
             pub enum {iname} {{\n\
             {variants_block}\n\
             }}\n\n\
             impl {iname} {{\n\
             \t/// Return the bare identity name.\n\
             \tpub fn as_name(&self) -> &'static str {{\n\
             \t\tmatch self {{\n\
             {as_name_block}\n\
             \t\t}}\n\
             \t}}\n\n\
             \t/// Return the JSON_IETF name (module-qualified when foreign).\n\
             \tpub fn as_json_name(&self) -> &'static str {{\n\
             \t\tmatch self {{\n\
             {as_json_name_block}\n\
             \t\t}}\n\
             \t}}\n\n\
             \t/// Return the optional `(synth_prefix, namespace)` pair when the\n\
             \t/// active identity is defined in a foreign module.\n\
             \tpub fn xml_prefix_ns(&self) -> Option<(&'static str, &'static str)> {{\n\
             \t\tmatch self {{\n\
             {xml_prefix_ns_block}\n\
             \t\t}}\n\
             \t}}\n\n\
             \t/// Return the XML value for the active identity (prefixed when foreign).\n\
             \tpub fn write_xml_value(&self) -> String {{\n\
             \t\tmatch self {{\n\
             {xml_value_block}\n\
             \t\t}}\n\
             \t}}\n\n\
             \t/// Parse a bare or module-qualified identity name.\n\
             \tpub fn from_name(name: &str) -> Option<Self> {{\n\
             \t\tmatch name {{\n\
             {from_name_block}\n\
             \t\t\t_ => None,\n\
             \t\t}}\n\
             \t}}\n\
             }}\n\n\
             impl std::fmt::Display for {iname} {{\n\
             \tfn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {{\n\
             \t\twrite!(f, \"{{}}\", self.as_name())\n\
             \t}}\n\
             }}"
        );
        self.helpers.push(helper);
        self.needs_xml_escape = true;
    }

    fn ensure_instance_identifier(&mut self) {
        if self.emitted_instance_identifier {
            return;
        }
        self.emitted_instance_identifier = true;
        self.helpers.push(INSTANCE_IDENTIFIER_HELPER.to_string());
        self.needs_xml_escape = true;
    }

    fn ensure_union(
        &mut self,
        uname: &str,
        yang_name: &str,
        members: &[TypeInfo<'ctx>],
        prefix: &str,
        field_ident: &str,
    ) {
        if !self.emitted_unions.insert(uname.to_string()) {
            return;
        }

        // Per-member metadata: (variant_name, payload_type, is_enum, is_bits,
        // is_identityref, is_instance_identifier, is_union, json_kind).
        let mut used_variants: HashSet<String> = HashSet::new();
        let mut variants: Vec<UnionVariantMeta> = Vec::new();
        for member in members {
            let label = union_member_label(member);
            let variant = safe_union_variant_name(&label, &mut used_variants);
            let member_field_ident = format!("{field_ident}{variant}");
            let (payload, is_enum, is_bits, is_identityref, is_union, json_kind) =
                self.rust_type_for(prefix, yang_name, member, &member_field_ident);
            let is_instance_identifier = type_is_instance_identifier(member);
            variants.push((
                variant,
                payload,
                is_enum,
                is_bits,
                is_identityref,
                is_instance_identifier,
                is_union,
                json_kind,
            ));
        }

        // Union helpers reference cambium_xml_escape_text for any string-like member.
        if variants.iter().any(
            |(
                _,
                _,
                is_enum,
                is_bits,
                is_identityref,
                is_instance_identifier,
                is_union,
                json_kind,
            )| {
                *is_enum
                    || *is_bits
                    || *is_identityref
                    || *is_instance_identifier
                    || *json_kind == JsonKind::String
                    || *is_union
            },
        ) {
            self.needs_xml_escape = true;
        }

        if variants.is_empty() {
            // Degenerate empty union — fall back would already have happened; this
            // guard keeps the emitted helper compilable.
            self.needs_json_escape = true;
            self.needs_xml_escape = true;
            self.helpers.push(format!(
                "/// Generated YANG union type `{yang_name}` (empty fallback).\n\
                 #[derive(Debug, Clone, PartialEq, Eq)]\n\
                 pub enum {uname} {{\n\
                 \t/// Placeholder variant.\n\
                 \tPlaceholder(String),\n\
                 }}\n\n\
                 impl {uname} {{\n\
                 \t/// Write the JSON_IETF value for the active member.\n\
                 \tpub fn write_json_value(&self, w: &mut String) {{\n\
                 \t\tif let Self::Placeholder(v) = self {{ w.push_str(&cambium_json_escape(v.as_str())); }}\n\
                 \t}}\n\n\
                 \t/// Return the optional XML namespace binding required by the active value.\n\
                 \tpub fn xml_prefix_ns(&self) -> Option<(&str, &str)> {{\n\
                 \t\tNone\n\
                 \t}}\n\n\
                 \t/// Write the XML value for the active member.\n\
                 \tpub fn write_xml_value(&self) -> String {{\n\
                 \t\tlet mut s = String::new();\n\
                 \t\tif let Self::Placeholder(v) = self {{ s.push_str(&cambium_xml_escape_text(&v.to_string())); }}\n\
                 \t\ts\n\
                 \t}}\n\
                 }}\n\n\
                 impl std::fmt::Display for {uname} {{\n\
                 \tfn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {{\n\
                 \t\tmatch self {{\n\
                 \t\t\tSelf::Placeholder(v) => write!(f, \"{{}}\", v),\n\
                 \t\t}}\n\
                 \t}}\n\
                 }}\n\n\
                 impl Default for {uname} {{\n\
                 \tfn default() -> Self {{\n\
                 \t\tSelf::Placeholder(String::new())\n\
                 \t}}\n\
                 }}"
            ));
            return;
        }

        let variants_block = variants
            .iter()
            .map(|(variant, payload, ..)| {
                let doc = format!("/// YANG union member `{variant}`.");
                format!("{doc}\n\t{variant}({payload}),")
            })
            .collect::<Vec<_>>()
            .join("\n");

        // All union payloads implement Default (primitives + emitted helpers), so
        // we manually impl Default for the first variant to avoid the Rust 2024
        // restriction on `#[default]` for non-unit enum variants.
        let (first_variant, first_payload, ..) = &variants[0];
        let default_impl = format!(
            "\n\nimpl Default for {uname} {{\n\
             \tfn default() -> Self {{\n\
             \t\tSelf::{first_variant}({first_payload}::default())\n\
             \t}}\n\
             }}"
        );

        let json_arms = variants
            .iter()
            .map(
                |(variant, _payload, is_enum, is_bits, is_identityref, is_instance_identifier, is_union, json_kind)| {
                    let expr = if *is_union {
                        "v.write_json_value(w)".to_string()
                    } else {
                        match json_kind {
                            JsonKind::String => {
                                let value_expr = if *is_enum {
                                    "v.as_name()"
                                } else if *is_identityref {
                                    "v.as_json_name()"
                                } else if *is_instance_identifier {
                                    "v.as_json_str()"
                                } else if *is_bits {
                                    "v.to_string()"
                                } else {
                                    "v.as_str()"
                                };
                                format!("w.push_str(&cambium_json_escape({value_expr}))")
                            }
                            JsonKind::BareNumber | JsonKind::Bool => {
                                "w.push_str(&format!(\"{}\", v))".to_string()
                            }
                            JsonKind::QuotedNumber => {
                                "{\n\t\t\t\tw.push(\'\"\');\n\t\t\t\tw.push_str(&format!(\"{}\", v));\n\t\t\t\tw.push(\'\"\')\n\t\t\t}".to_string()
                            }
                        }
                    };
                    format!("\t\t\tSelf::{variant}(v) => {expr},")
                },
            )
            .collect::<Vec<_>>()
            .join("\n");

        let xml_arms = variants
            .iter()
            .map(
                |(
                    variant,
                    payload,
                    is_enum,
                    is_bits,
                    is_identityref,
                    is_instance_identifier,
                    is_union,
                    json_kind,
                )| {
                    let expr = if *is_union || *is_instance_identifier {
                        "w.push_str(&v.write_xml_value())".to_string()
                    } else if *is_enum || *is_identityref {
                        "w.push_str(&cambium_xml_escape_text(v.as_name()))".to_string()
                    } else if *is_bits {
                        "w.push_str(&cambium_xml_escape_text(&v.to_string()))".to_string()
                    } else if *json_kind == JsonKind::String {
                        if payload == "String" {
                            "w.push_str(&cambium_xml_escape_text(&v.to_string()))".to_string()
                        } else {
                            "w.push_str(&cambium_xml_escape_text(v.as_str()))".to_string()
                        }
                    } else {
                        "w.push_str(&format!(\"{}\", v))".to_string()
                    };
                    format!("\t\t\tSelf::{variant}(v) => {expr},")
                },
            )
            .collect::<Vec<_>>()
            .join("\n");

        let xml_prefix_ns_arms = variants
            .iter()
            .map(
                |(
                    variant,
                    _payload,
                    _is_enum,
                    _is_bits,
                    is_identityref,
                    is_instance_identifier,
                    is_union,
                    _json_kind,
                )| {
                    if *is_union || *is_identityref || *is_instance_identifier {
                        format!("\t\t\tSelf::{variant}(v) => v.xml_prefix_ns(),")
                    } else {
                        format!("\t\t\tSelf::{variant}(_) => None,")
                    }
                },
            )
            .collect::<Vec<_>>()
            .join("\n");

        let display_arms = variants
            .iter()
            .map(|(variant, ..)| format!("\t\t\tSelf::{variant}(v) => write!(f, \"{{}}\", v),"))
            .collect::<Vec<_>>()
            .join("\n");

        let helper = format!(
            "/// Generated YANG union type `{yang_name}`.\n\
             #[derive(Debug, Clone, PartialEq, Eq)]\n\
             pub enum {uname} {{\n\
             {variants_block}\n\
             }}\n\n\
             impl {uname} {{\n\
             \t/// Write the JSON_IETF value for the active member.\n\
             \tpub fn write_json_value(&self, w: &mut String) {{\n\
             \t\tmatch self {{\n\
             {json_arms}\n\
             \t\t}}\n\
             \t}}\n\n\
             \t/// Return the optional XML namespace binding required by the active value.\n\
             \tpub fn xml_prefix_ns(&self) -> Option<(&str, &str)> {{\n\
             \t\tmatch self {{\n\
             {xml_prefix_ns_arms}\n\
             \t\t}}\n\
             \t}}\n\n\
             \t/// Write the XML value for the active member.\n\
             \tpub fn write_xml_value(&self) -> String {{\n\
             \t\tlet mut w = String::new();\n\
             \t\tmatch self {{\n\
             {xml_arms}\n\
             \t\t}}\n\
             \t\tw\n\
             \t}}\n\
             }}\n\n\
             impl std::fmt::Display for {uname} {{\n\
             \tfn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {{\n\
             \t\tmatch self {{\n\
             {display_arms}\n\
             \t\t}}\n\
             \t}}\n\
             }}{default_impl}"
        );
        self.helpers.push(helper);
    }

    fn ensure_int_range(&mut self, rname: &str, kind: IntKind, bounds: Option<&[RangeBound]>) {
        if !self.emitted_int_ranges.insert(rname.to_string()) {
            return;
        }
        let inner = int_kind_rust_type(kind);
        let (type_min, type_max) = int_type_limits(kind);
        // Parsed `(lo, hi)` sub-ranges; an absent `range` means the full type span.
        let mut ranges: Vec<(i128, i128)> = bounds
            .unwrap_or_default()
            .iter()
            .map(|b| {
                let lo = parse_int_bound(b.min(), type_min, type_max);
                let hi = parse_int_bound(b.max(), type_min, type_max);
                (lo, hi)
            })
            .collect();
        if ranges.is_empty() {
            ranges.push((type_min, type_max));
        }
        let mut checks: Vec<String> = ranges
            .iter()
            .map(|(lo, hi)| format!("({lo}i128..={hi}i128).contains(&value)"))
            .collect();
        let check = if checks.len() == 1 {
            checks.swap_remove(0)
        } else {
            checks.join(" || ")
        };
        // `Default` must yield an in-range value. When 0 is permitted, the derived
        // `Self(0)` is valid (and keeping the derive avoids clippy::derivable_impls);
        // otherwise derive without `Default` and return the range minimum.
        let zero_in_range = ranges.iter().any(|(lo, hi)| *lo <= 0 && 0 <= *hi);
        let (derive_default, default_impl) = if zero_in_range {
            ("Default, ", String::new())
        } else {
            let min_lo = ranges.iter().map(|(lo, _)| *lo).min().unwrap_or(type_min);
            (
                "",
                format!(
                    "\n\nimpl Default for {rname} {{\n\
                     \tfn default() -> Self {{\n\
                     \t\tSelf({min_lo}i128 as {inner})\n\
                     \t}}\n\
                     }}"
                ),
            )
        };
        let helper = format!(
            "/// Generated YANG integer range type `{rname}`.\n\
             #[derive(Debug, Clone, Copy, {derive_default}PartialEq, Eq, PartialOrd, Ord, Hash)]\n\
             #[repr(transparent)]\n\
             pub struct {rname}(pub(crate) {inner});\n\n\
             impl {rname} {{\n\
             \t/// Create a new `{rname}` if `value` is within range.\n\
             \tpub fn new(value: i128) -> Result<Self, {rname}Error> {{\n\
             \t\tif {check} {{\n\
             \t\t\tOk(Self(value as {inner}))\n\
             \t\t}} else {{\n\
             \t\t\tErr({rname}Error {{ value }})\n\
             \t\t}}\n\
             \t}}\n\n\
             \t/// Return the wrapped value.\n\
             \tpub fn get(&self) -> {inner} {{\n\
             \t\tself.0\n\
             \t}}\n\
             }}\n\n\
             /// Error returned when a value is outside the allowed range.\n\
             #[derive(Debug, Clone, PartialEq, Eq)]\n\
             pub struct {rname}Error {{\n\
             \t/// The rejected value.\n\
             \tpub value: i128,\n\
             }}\n\n\
             impl std::fmt::Display for {rname}Error {{\n\
             \tfn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {{\n\
             \t\twrite!(f, \"value {{}} out of range for {rname}\", self.value)\n\
             \t}}\n\
             }}\n\n\
             impl std::error::Error for {rname}Error {{}}\n\n\
             impl std::fmt::Display for {rname} {{\n\
             \tfn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {{\n\
             \t\twrite!(f, \"{{}}\", self.0)\n\
             \t}}\n\
             }}{default_impl}"
        );
        self.helpers.push(helper);
    }

    fn ensure_string_length(&mut self, lname: &str, bounds: &[RangeBound]) {
        if !self.emitted_string_lengths.insert(lname.to_string()) {
            return;
        }
        let ranges: Vec<(usize, usize)> = bounds
            .iter()
            .map(|b| (parse_length_bound(b.min()), parse_length_bound(b.max())))
            .collect();
        let mut checks: Vec<String> = ranges
            .iter()
            .map(|(lo, hi)| format!("({lo}usize..={hi}usize).contains(&len)"))
            .collect();
        let check = if checks.len() == 1 {
            checks.swap_remove(0)
        } else {
            checks.join(" || ")
        };
        // `Default` must satisfy the length bound. The empty string (the derived
        // default) is only valid when a minimum length of 0 is permitted; keeping
        // the derive then avoids clippy::derivable_impls. Otherwise return a
        // space-filled string of the minimum valid length (this newtype enforces
        // length only, never `pattern`).
        let min_len = ranges.iter().map(|(lo, _)| *lo).min().unwrap_or(0);
        let (derive_default, default_impl) = if min_len == 0 {
            ("Default, ", String::new())
        } else {
            (
                "",
                format!(
                    "\n\nimpl Default for {lname} {{\n\
                     \tfn default() -> Self {{\n\
                     \t\tSelf(\" \".repeat({min_len}))\n\
                     \t}}\n\
                     }}"
                ),
            )
        };
        let helper = format!(
            "/// Generated YANG string length type `{lname}`.\n\
             #[derive(Debug, Clone, {derive_default}PartialEq, Eq, PartialOrd, Ord, Hash)]\n\
             #[repr(transparent)]\n\
             pub struct {lname}(pub(crate) String);\n\n\
             impl {lname} {{\n\
             \t/// Create a new `{lname}` if `value`'s length is within bounds.\n\
             \tpub fn new(value: String) -> Result<Self, {lname}Error> {{\n\
             \t\tlet len = value.len();\n\
             \t\tif {check} {{\n\
             \t\t\tOk(Self(value))\n\
             \t\t}} else {{\n\
             \t\t\tErr({lname}Error {{ value }})\n\
             \t\t}}\n\
             \t}}\n\n\
             \t/// Return a borrowed view of the wrapped string.\n\
             \tpub fn as_str(&self) -> &str {{\n\
             \t\t&self.0\n\
             \t}}\n\
             }}\n\n\
             /// Error returned when a string length is outside the allowed bounds.\n\
             #[derive(Debug, Clone, PartialEq, Eq)]\n\
             pub struct {lname}Error {{\n\
             \t/// The rejected string.\n\
             \tpub value: String,\n\
             }}\n\n\
             impl std::fmt::Display for {lname}Error {{\n\
             \tfn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {{\n\
             \t\twrite!(f, \"length {{}} out of bounds for {lname}\", self.value.len())\n\
             \t}}\n\
             }}\n\n\
             impl std::error::Error for {lname}Error {{}}\n\n\
             impl std::fmt::Display for {lname} {{\n\
             \tfn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {{\n\
             \t\twrite!(f, \"{{}}\", self.0)\n\
             \t}}\n\
             }}{default_impl}"
        );
        self.helpers.push(helper);
    }

    fn struct_name_for(&self, prefix: &str, node: SchemaNodeRef<'ctx>) -> String {
        format!("{}{}", prefix, to_pascal_case(node.name()))
    }

    fn entry_name_for(&self, prefix: &str, node: SchemaNodeRef<'ctx>) -> String {
        format!("{}{}Entry", prefix, to_pascal_case(node.name()))
    }

    fn emit_struct_definition(&mut self, name: &str, fields: &[Field<'ctx>]) {
        self.push(&format!(
            "/// Generated data node container for YANG structure `{name}`."
        ));
        self.push("#[derive(Debug, Clone, Default, PartialEq, Eq)]");
        self.push(&format!("pub struct {name} {{"));
        for field in fields {
            let doc = field.doc_text();
            self.push(&doc);
            if field.optional {
                self.push(&format!("\tpub {}: Option<{}>,", field.ident, field.ty));
            } else {
                self.push(&format!("\tpub {}: {},", field.ident, field.ty));
            }
        }
        self.push("}\n");
    }

    fn emit_field_order(&mut self, name: &str, fields: &[Field<'ctx>]) {
        let wire_names: Vec<_> = fields.iter().map(|f| f.node.name().to_string()).collect();
        let literals: Vec<_> = wire_names.iter().map(|n| format!("{n:?}")).collect();
        self.push(&format!(
            "impl {name} {{\n\t/// Field-order manifest: keys first, then schema declaration order.\n\tpub const FIELD_ORDER: &[&str] = &[{}];",
            literals.join(", ")
        ));
        self.field_order.insert(name.to_string(), wire_names);
    }

    fn emit_struct_serializer(&mut self, _name: &str, fields: &[Field<'ctx>]) {
        let has_fields = !fields.is_empty();
        self.push("\n\t/// Serialize this node and its descendants to XML.");
        if has_fields {
            self.push("\tpub fn write_xml(&self, w: &mut String, depth: usize) {");
            self.push("\t\tlet indent = \" \".repeat(depth * 2);");
            for field in fields {
                self.emit_field_xml(field);
            }
        } else {
            // An empty struct (e.g. an empty presence container) has no children;
            // the params are unused, so prefix them to satisfy `#![deny(warnings)]`.
            self.push("\tpub fn write_xml(&self, _w: &mut String, _depth: usize) {");
        }
        self.push("\t}\n");

        self.push("\t/// Serialize this node and its descendants to JSON_IETF.");
        if has_fields {
            self.needs_json_indent = true;
            self.push("\tpub fn write_json(&self, w: &mut String, depth: usize) {");
            self.push("\t\tif !self.has_content() {");
            self.push("\t\t\tw.push_str(\"{}\");");
            self.push("\t\t\treturn;");
            self.push("\t\t}");
            self.push("\t\tw.push('{');");
            self.push("\t\tlet mut first = true;");
            for field in fields {
                self.emit_field_json(field);
            }
            self.push("\t\tcambium_json_indent(w, depth);");
            self.push("\t\tw.push('}');");
        } else {
            self.push("\tpub fn write_json(&self, w: &mut String, _depth: usize) {");
            self.push("\t\tw.push_str(\"{}\");");
        }
        self.push("\t}\n");
        self.push("\t/// Serialize this node and its descendants to XML.");
        self.push("\tpub fn to_xml(&self) -> String {");
        self.push("\t\tlet mut s = String::new();");
        self.push("\t\tself.write_xml(&mut s, 0);");
        self.push("\t\ts");
        self.push("\t}\n");
        self.push("\t/// Serialize this node and its descendants to JSON_IETF.");
        self.push("\tpub fn to_json_ietf(&self) -> String {");
        self.push("\t\tlet mut s = String::new();");
        self.push("\t\tself.write_json(&mut s, 0);");
        self.push("\t\ts");
        self.push("\t}\n");
        self.emit_has_content(fields);
        self.push("}\n");
    }

    fn emit_has_content(&mut self, fields: &[Field<'ctx>]) {
        self.push("\t/// Returns true if this node carries any descendant content.");
        self.push("\tpub fn has_content(&self) -> bool {");
        let mut mandatory_leaf = false;
        let mut conds: Vec<String> = Vec::new();
        for field in fields {
            let ident = &field.ident;
            match field.node.kind() {
                SchemaNodeKind::Leaf => {
                    if field.optional {
                        conds.push(format!("self.{ident}.is_some()"));
                    } else {
                        mandatory_leaf = true;
                    }
                }
                SchemaNodeKind::LeafList => {
                    conds.push(format!("!self.{ident}.is_empty()"));
                }
                SchemaNodeKind::Container => {
                    if field.optional {
                        conds.push(format!("self.{ident}.is_some()"));
                    } else {
                        conds.push(format!("self.{ident}.has_content()"));
                    }
                }
                SchemaNodeKind::List => {
                    conds.push(format!("!self.{ident}.is_empty()"));
                }
                _ => {}
            }
        }
        if mandatory_leaf {
            self.push("\t\ttrue");
        } else if conds.is_empty() {
            self.push("\t\tfalse");
        } else {
            self.push(&format!("\t\t{}", conds.join(" || ")));
        }
        self.push("\t}\n");
    }

    fn emit_root_serializer(&mut self, fields: &[Field<'ctx>]) {
        let name = &self.module_pascal;
        let ns = self.module.namespace().to_string();

        self.push(&format!("impl {name} {{"));
        self.push(&format!(
            "\t/// XML namespace for module {:?}.",
            self.module_name
        ));
        self.push(&format!("\tpub const MODULE_NS: &str = {ns:?};"));
        self.push("\n\t/// Serialize the top-level data nodes to XML.");
        self.push("\tpub fn to_xml(&self) -> String {");
        self.push("\t\tlet mut s = String::new();");
        for field in fields {
            self.emit_top_level_xml(field);
        }
        self.push("\t\ts");
        self.push("\t}\n");

        self.push(&format!(
            "\t/// Module name for JSON_IETF namespace boundaries.\n\tpub const MODULE_NAME: &str = {:?};",
            self.module_name
        ));
        self.needs_json_indent = true;
        self.push("\n\t/// Serialize the top-level data nodes to JSON_IETF.");
        self.push("\tpub fn to_json_ietf(&self) -> String {");
        self.push("\t\tlet mut s = String::new();");
        self.push("\t\ts.push('{');");
        self.push("\t\tlet mut first = true;");
        for field in fields {
            self.emit_top_level_json(field);
        }
        self.push("\t\tcambium_json_indent(&mut s, 0);");
        self.push("\t\ts.push('}');");
        self.push("\t\ts.push('\\n');");
        self.push("\t\ts");
        self.push("\t}\n}\n");
    }

    fn emit_cambium_struct_impl(&mut self, name: &str) {
        self.push(&format!("impl CambiumStruct for {name} {{"));
        self.push("\tfn to_xml(&self) -> String { self.to_xml() }");
        self.push("\tfn to_json_ietf(&self) -> String { self.to_json_ietf() }");
        self.push("}\n");
    }

    fn emit_field_xml(&mut self, field: &Field<'ctx>) {
        let wire = field.node.name();
        let ident = &field.ident;

        match field.node.kind() {
            SchemaNodeKind::Leaf => {
                if field.is_identityref {
                    if field.optional {
                        self.push(&format!("\t\tif let Some(ref v) = self.{ident} {{"));
                        self.emit_identityref_leaf_value(wire, "v");
                        self.push("\t\t}");
                    } else {
                        self.emit_identityref_leaf_value(wire, &format!("self.{ident}"));
                    }
                } else if field.optional {
                    self.push(&format!("\t\tif let Some(ref v) = self.{ident} {{"));
                    self.emit_leaf_value(wire, "v", field);
                    self.push("\t\t}");
                } else {
                    self.emit_leaf_value(wire, &format!("self.{ident}"), field);
                }
            }
            SchemaNodeKind::LeafList => {
                if field.is_identityref {
                    self.push(&format!("\t\tfor v in self.{ident}.iter() {{"));
                    self.emit_identityref_leaf_list_value(wire);
                    self.push("\t\t}");
                } else {
                    self.push(&format!("\t\tfor v in self.{ident}.iter() {{"));
                    self.emit_leaf_list_value(wire, field);
                    self.push("\t\t}");
                }
            }
            SchemaNodeKind::Container => {
                if field.optional {
                    self.push(&format!("\t\tif let Some(ref v) = self.{ident} {{"));
                    self.emit_container_value(wire, "v", "depth");
                    self.push("\t\t}");
                } else {
                    self.push(&format!("\t\tif self.{ident}.has_content() {{"));
                    self.emit_container_value(wire, &format!("self.{ident}"), "depth");
                    self.push("\t\t}");
                }
            }
            SchemaNodeKind::List => {
                self.push(&format!("\t\tfor entry in self.{ident}.iter() {{"));
                self.push(&format!(
                    "\t\t\tw.push_str(&format!(\"{{}}<{wire}>\\n\", &indent));"
                ));
                self.push("\t\t\tentry.write_xml(w, depth + 1);");
                self.push(&format!(
                    "\t\t\tw.push_str(&format!(\"{{}}</{wire}>\\n\", &indent));"
                ));
                self.push("\t\t}");
            }
            _ => {}
        }
    }

    fn emit_top_level_xml(&mut self, field: &Field<'ctx>) {
        let wire = field.node.name();
        let ident = &field.ident;
        let ns = self.module.namespace();
        let ns_attr = if ns.is_empty() {
            String::new()
        } else {
            format!(" xmlns=\\\"{ns}\\\"")
        };

        match field.node.kind() {
            SchemaNodeKind::Leaf => {
                if field.is_identityref {
                    if field.optional {
                        self.push(&format!("\t\tif let Some(ref v) = self.{ident} {{"));
                        self.emit_top_identityref_leaf_value(wire, &ns_attr, "v");
                        self.push("\t\t}");
                    } else {
                        self.emit_top_identityref_leaf_value(
                            wire,
                            &ns_attr,
                            &format!("self.{ident}"),
                        );
                    }
                } else if field.optional {
                    self.push(&format!("\t\tif let Some(ref v) = self.{ident} {{"));
                    self.emit_top_leaf_value(wire, &ns_attr, "v", field);
                    self.push("\t\t}");
                } else {
                    self.emit_top_leaf_value(wire, &ns_attr, &format!("self.{ident}"), field);
                }
            }
            SchemaNodeKind::LeafList => {
                if field.is_identityref {
                    self.push(&format!("\t\tfor v in self.{ident}.iter() {{"));
                    self.emit_top_identityref_leaf_list_value(wire, &ns_attr);
                    self.push("\t\t}");
                } else {
                    self.push(&format!("\t\tfor v in self.{ident}.iter() {{"));
                    self.emit_top_leaf_list_value(wire, &ns_attr, field);
                    self.push("\t\t}");
                }
            }
            SchemaNodeKind::Container => {
                if field.optional {
                    self.push(&format!("\t\tif let Some(ref v) = self.{ident} {{"));
                    self.emit_top_container_value(wire, &ns_attr, "v");
                    self.push("\t\t}");
                } else {
                    self.push(&format!("\t\tif self.{ident}.has_content() {{"));
                    self.emit_top_container_value(wire, &ns_attr, &format!("self.{ident}"));
                    self.push("\t\t}");
                }
            }
            SchemaNodeKind::List => {
                self.push(&format!("\t\tfor entry in self.{ident}.iter() {{"));
                self.push(&format!("\t\t\ts.push_str(\"<{wire}{ns_attr}>\\n\");"));
                self.push("\t\t\tentry.write_xml(&mut s, 1);");
                self.push(&format!("\t\t\ts.push_str(\"</{wire}>\\n\");"));
                self.push("\t\t}");
            }
            _ => {}
        }
    }

    fn emit_leaf_value(&mut self, wire: &str, value_ref: &str, field: &Field<'ctx>) {
        let (expr, needs_escape) = xml_value_expr(value_ref, field);
        if needs_escape {
            self.needs_xml_escape = true;
        }
        if field.is_union || field_is_instance_identifier(field) {
            self.emit_xmlns_leaf_value(wire, value_ref, &expr);
            return;
        }
        self.push(&format!(
            "\t\t\tw.push_str(&format!(\"{{}}<{wire}>{{}}</{wire}>\\n\", &indent, {expr}));"
        ));
    }

    fn emit_leaf_list_value(&mut self, wire: &str, field: &Field<'ctx>) {
        let (expr, needs_escape) = xml_value_expr("v", field);
        if needs_escape {
            self.needs_xml_escape = true;
        }
        if field.is_union || field_is_instance_identifier(field) {
            self.emit_xmlns_leaf_list_value(wire, &expr);
            return;
        }
        self.push(&format!(
            "\t\t\tw.push_str(&format!(\"{{}}<{wire}>{{}}</{wire}>\\n\", &indent, {expr}));"
        ));
    }

    fn emit_top_leaf_value(
        &mut self,
        wire: &str,
        ns_attr: &str,
        value_ref: &str,
        field: &Field<'ctx>,
    ) {
        let (expr, needs_escape) = xml_value_expr(value_ref, field);
        if needs_escape {
            self.needs_xml_escape = true;
        }
        if field.is_union || field_is_instance_identifier(field) {
            self.emit_top_xmlns_leaf_value(wire, ns_attr, value_ref, &expr);
            return;
        }
        self.push(&format!(
            "\t\t\ts.push_str(&format!(\"<{wire}{ns_attr}>{{}}</{wire}>\\n\", {expr}));"
        ));
    }

    fn emit_top_leaf_list_value(&mut self, wire: &str, ns_attr: &str, field: &Field<'ctx>) {
        let (expr, needs_escape) = xml_value_expr("v", field);
        if needs_escape {
            self.needs_xml_escape = true;
        }
        if field.is_union || field_is_instance_identifier(field) {
            self.emit_top_xmlns_leaf_list_value(wire, ns_attr, &expr);
            return;
        }
        self.push(&format!(
            "\t\t\ts.push_str(&format!(\"<{wire}{ns_attr}>{{}}</{wire}>\\n\", {expr}));"
        ));
    }

    /// Emit a nested scalar leaf whose value may require a namespace binding.
    fn emit_xmlns_leaf_value(&mut self, wire: &str, value_ref: &str, expr: &str) {
        self.push(&format!(
            "\t\t\tif let Some((__pfx, __ns)) = {value_ref}.xml_prefix_ns() {{"
        ));
        self.push(&format!(
            "\t\t\t\tw.push_str(&format!(\"{{}}<{wire} xmlns:{{}}=\\\"{{}}\\\">{{}}</{wire}>\\n\", &indent, __pfx, __ns, {expr}));"
        ));
        self.push("\t\t\t} else {");
        self.push(&format!(
            "\t\t\t\tw.push_str(&format!(\"{{}}<{wire}>{{}}</{wire}>\\n\", &indent, {expr}));"
        ));
        self.push("\t\t\t}");
    }

    /// Emit a nested scalar leaf-list entry whose value may require a namespace binding.
    fn emit_xmlns_leaf_list_value(&mut self, wire: &str, expr: &str) {
        self.push("\t\t\tif let Some((__pfx, __ns)) = v.xml_prefix_ns() {");
        self.push(&format!(
            "\t\t\t\tw.push_str(&format!(\"{{}}<{wire} xmlns:{{}}=\\\"{{}}\\\">{{}}</{wire}>\\n\", &indent, __pfx, __ns, {expr}));"
        ));
        self.push("\t\t\t} else {");
        self.push(&format!(
            "\t\t\t\tw.push_str(&format!(\"{{}}<{wire}>{{}}</{wire}>\\n\", &indent, {expr}));"
        ));
        self.push("\t\t\t}");
    }

    /// Emit a top-level scalar leaf whose value may require a namespace binding.
    fn emit_top_xmlns_leaf_value(
        &mut self,
        wire: &str,
        ns_attr: &str,
        value_ref: &str,
        expr: &str,
    ) {
        self.push(&format!(
            "\t\t\tif let Some((__pfx, __ns)) = {value_ref}.xml_prefix_ns() {{"
        ));
        self.push(&format!(
            "\t\t\t\ts.push_str(&format!(\"<{wire}{ns_attr} xmlns:{{}}=\\\"{{}}\\\">{{}}</{wire}>\\n\", __pfx, __ns, {expr}));"
        ));
        self.push("\t\t\t} else {");
        self.push(&format!(
            "\t\t\t\ts.push_str(&format!(\"<{wire}{ns_attr}>{{}}</{wire}>\\n\", {expr}));"
        ));
        self.push("\t\t\t}");
    }

    /// Emit a top-level scalar leaf-list entry whose value may require a namespace binding.
    fn emit_top_xmlns_leaf_list_value(&mut self, wire: &str, ns_attr: &str, expr: &str) {
        self.push("\t\t\tif let Some((__pfx, __ns)) = v.xml_prefix_ns() {");
        self.push(&format!(
            "\t\t\t\ts.push_str(&format!(\"<{wire}{ns_attr} xmlns:{{}}=\\\"{{}}\\\">{{}}</{wire}>\\n\", __pfx, __ns, {expr}));"
        ));
        self.push("\t\t\t} else {");
        self.push(&format!(
            "\t\t\t\ts.push_str(&format!(\"<{wire}{ns_attr}>{{}}</{wire}>\\n\", {expr}));"
        ));
        self.push("\t\t\t}");
    }

    /// Emit a nested identityref leaf value, synthesizing `xmlns:<prefix>` and a
    /// prefixed value when the active identity is foreign.
    fn emit_identityref_leaf_value(&mut self, wire: &str, value_ref: &str) {
        self.push(&format!(
            "\t\t\tif let Some((__pfx, __ns)) = {value_ref}.xml_prefix_ns() {{"
        ));
        self.push(&format!(
            "\t\t\t\tw.push_str(&format!(\"{{}}<{wire} xmlns:{{}}=\\\"{{}}\\\">{{}}</{wire}>\\n\", &indent, __pfx, __ns, {value_ref}.write_xml_value()));"
        ));
        self.push("\t\t\t} else {");
        self.push(&format!(
            "\t\t\t\tw.push_str(&format!(\"{{}}<{wire}>{{}}</{wire}>\\n\", &indent, {value_ref}.write_xml_value()));"
        ));
        self.push("\t\t\t}");
    }

    /// Emit a nested identityref leaf-list element value.
    fn emit_identityref_leaf_list_value(&mut self, wire: &str) {
        self.push("\t\t\tif let Some((__pfx, __ns)) = v.xml_prefix_ns() {");
        self.push(&format!(
            "\t\t\t\tw.push_str(&format!(\"{{}}<{wire} xmlns:{{}}=\\\"{{}}\\\">{{}}</{wire}>\\n\", &indent, __pfx, __ns, v.write_xml_value()));"
        ));
        self.push("\t\t\t} else {");
        self.push(&format!(
            "\t\t\t\tw.push_str(&format!(\"{{}}<{wire}>{{}}</{wire}>\\n\", &indent, v.write_xml_value()));"
        ));
        self.push("\t\t\t}");
    }

    /// Emit a top-level identityref leaf value.
    fn emit_top_identityref_leaf_value(&mut self, wire: &str, ns_attr: &str, value_ref: &str) {
        self.push(&format!(
            "\t\t\tif let Some((__pfx, __ns)) = {value_ref}.xml_prefix_ns() {{"
        ));
        self.push(&format!(
            "\t\t\t\ts.push_str(&format!(\"<{wire}{ns_attr} xmlns:{{}}=\\\"{{}}\\\">{{}}</{wire}>\\n\", __pfx, __ns, {value_ref}.write_xml_value()));"
        ));
        self.push("\t\t\t} else {");
        self.push(&format!(
            "\t\t\t\ts.push_str(&format!(\"<{wire}{ns_attr}>{{}}</{wire}>\\n\", {value_ref}.write_xml_value()));"
        ));
        self.push("\t\t\t}");
    }

    /// Emit a top-level identityref leaf-list element value.
    fn emit_top_identityref_leaf_list_value(&mut self, wire: &str, ns_attr: &str) {
        self.push("\t\t\tif let Some((__pfx, __ns)) = v.xml_prefix_ns() {");
        self.push(&format!(
            "\t\t\t\ts.push_str(&format!(\"<{wire}{ns_attr} xmlns:{{}}=\\\"{{}}\\\">{{}}</{wire}>\\n\", __pfx, __ns, v.write_xml_value()));"
        ));
        self.push("\t\t\t} else {");
        self.push(&format!(
            "\t\t\t\ts.push_str(&format!(\"<{wire}{ns_attr}>{{}}</{wire}>\\n\", v.write_xml_value()));"
        ));
        self.push("\t\t\t}");
    }

    fn emit_container_value(&mut self, wire: &str, value_ref: &str, depth_var: &str) {
        self.push(&format!("\t\t\tif {value_ref}.has_content() {{"));
        self.push(&format!(
            "\t\t\t\tw.push_str(&format!(\"{{}}<{wire}>\\n\", &indent));"
        ));
        self.push(&format!(
            "\t\t\t\t{value_ref}.write_xml(w, {depth_var} + 1);"
        ));
        self.push(&format!(
            "\t\t\t\tw.push_str(&format!(\"{{}}</{wire}>\\n\", &indent));"
        ));
        self.push("\t\t\t} else {");
        self.push(&format!(
            "\t\t\t\tw.push_str(&format!(\"{{}}<{wire}/>\\n\", &indent));"
        ));
        self.push("\t\t\t}");
    }

    fn emit_top_container_value(&mut self, wire: &str, ns_attr: &str, value_ref: &str) {
        self.push(&format!("\t\t\tif {value_ref}.has_content() {{"));
        self.push(&format!("\t\t\t\ts.push_str(\"<{wire}{ns_attr}>\\n\");"));
        self.push(&format!("\t\t\t\t{value_ref}.write_xml(&mut s, 1);"));
        self.push(&format!("\t\t\t\ts.push_str(\"</{wire}>\\n\");"));
        self.push("\t\t\t} else {");
        self.push(&format!("\t\t\t\ts.push_str(\"<{wire}{ns_attr}/>\\n\");"));
        self.push("\t\t\t}");
    }

    fn emit_field_json(&mut self, field: &Field<'ctx>) {
        let wire = field.node.name();
        let ident = &field.ident;

        match field.node.kind() {
            SchemaNodeKind::Leaf => {
                if field.optional {
                    self.push(&format!("\t\tif let Some(ref v) = self.{ident} {{"));
                    self.emit_scalar_json(wire, "w", "v", field);
                    self.push("\t\t}");
                } else {
                    self.emit_scalar_json(wire, "w", &format!("self.{ident}"), field);
                }
            }
            SchemaNodeKind::LeafList => {
                self.push("\t\tif std::mem::take(&mut first) {} else { w.push(','); }");
                self.push("\t\tcambium_json_indent(w, depth + 1);");
                self.push(&format!("\t\tw.push_str(\"\\\"{wire}\\\": \");"));
                self.push("\t\tw.push('[');");
                self.push(&format!(
                    "\t\tfor (i, v) in self.{ident}.iter().enumerate() {{"
                ));
                self.push("\t\t\tif i > 0 { w.push(','); }");
                self.push("\t\t\tcambium_json_indent(w, depth + 2);");
                self.emit_json_leaf_list_element("v", field);
                self.push("\t\t}");
                self.push("\t\tcambium_json_indent(w, depth + 1);");
                self.push("\t\tw.push(']');");
            }
            SchemaNodeKind::Container => {
                if field.optional {
                    self.push(&format!("\t\tif let Some(ref v) = self.{ident} {{"));
                    self.push("\t\t\tif std::mem::take(&mut first) {} else { w.push(','); }");
                    self.push("\t\t\tcambium_json_indent(w, depth + 1);");
                    self.push(&format!("\t\t\tw.push_str(\"\\\"{wire}\\\": \");"));
                    self.push("\t\t\tv.write_json(w, depth + 1);");
                    self.push("\t\t}");
                } else {
                    self.push(&format!("\t\tif self.{ident}.has_content() {{"));
                    self.push("\t\t\tif std::mem::take(&mut first) {} else { w.push(','); }");
                    self.push("\t\t\tcambium_json_indent(w, depth + 1);");
                    self.push(&format!("\t\tw.push_str(\"\\\"{wire}\\\": \");"));
                    self.push(&format!("\t\tself.{ident}.write_json(w, depth + 1);"));
                    self.push("\t\t}");
                }
            }
            SchemaNodeKind::List => {
                self.push("\t\tif std::mem::take(&mut first) {} else { w.push(','); }");
                self.push("\t\tcambium_json_indent(w, depth + 1);");
                self.push(&format!("\t\tw.push_str(\"\\\"{wire}\\\": \");"));
                self.push("\t\tw.push('[');");
                self.push(&format!(
                    "\t\tfor (i, entry) in self.{ident}.iter().enumerate() {{"
                ));
                self.push("\t\t\tif i > 0 { w.push(','); }");
                self.push("\t\t\tcambium_json_indent(w, depth + 2);");
                self.push("\t\t\tentry.write_json(w, depth + 2);");
                self.push("\t\t}");
                self.push("\t\tcambium_json_indent(w, depth + 1);");
                self.push("\t\tw.push(']');");
            }
            _ => {}
        }
    }

    fn emit_top_level_json(&mut self, field: &Field<'ctx>) {
        let wire = field.node.name();
        let ident = &field.ident;
        let module = self.module.name().to_string();

        match field.node.kind() {
            SchemaNodeKind::Leaf => {
                if field.optional {
                    self.push(&format!("\t\tif let Some(ref v) = self.{ident} {{"));
                    self.emit_scalar_json(&format!("{module}:{wire}"), "s", "v", field);
                    self.push("\t\t}");
                } else {
                    self.emit_scalar_json(
                        &format!("{module}:{wire}"),
                        "s",
                        &format!("self.{ident}"),
                        field,
                    );
                }
            }
            SchemaNodeKind::LeafList => {
                self.push("\t\tif std::mem::take(&mut first) {} else { s.push(','); }");
                self.push("\t\tcambium_json_indent(&mut s, 1);");
                self.push(&format!("\t\ts.push_str(\"\\\"{module}:{wire}\\\": \");"));
                self.push("\t\ts.push('[');");
                self.push(&format!(
                    "\t\tfor (i, v) in self.{ident}.iter().enumerate() {{"
                ));
                self.push("\t\t\tif i > 0 { s.push(','); }");
                self.push("\t\t\tcambium_json_indent(&mut s, 2);");
                self.emit_json_leaf_list_element_top("v", field);
                self.push("\t\t}");
                self.push("\t\tcambium_json_indent(&mut s, 1);");
                self.push("\t\ts.push(']');");
            }
            SchemaNodeKind::Container => {
                if field.optional {
                    self.push(&format!("\t\tif let Some(ref v) = self.{ident} {{"));
                    self.push("\t\t\tif std::mem::take(&mut first) {} else { s.push(','); }");
                    self.push("\t\t\tcambium_json_indent(&mut s, 1);");
                    self.push(&format!("\t\t\ts.push_str(\"\\\"{module}:{wire}\\\": \");"));
                    self.push("\t\t\tv.write_json(&mut s, 1);");
                    self.push("\t\t}");
                } else {
                    self.push(&format!("\t\tif self.{ident}.has_content() {{"));
                    self.push("\t\t\tif std::mem::take(&mut first) {} else { s.push(','); }");
                    self.push("\t\t\tcambium_json_indent(&mut s, 1);");
                    self.push(&format!("\t\ts.push_str(\"\\\"{module}:{wire}\\\": \");"));
                    self.push(&format!("\t\tself.{ident}.write_json(&mut s, 1);"));
                    self.push("\t\t}");
                }
            }
            SchemaNodeKind::List => {
                self.push("\t\tif std::mem::take(&mut first) {} else { s.push(','); }");
                self.push("\t\tcambium_json_indent(&mut s, 1);");
                self.push(&format!("\t\ts.push_str(\"\\\"{module}:{wire}\\\": \");"));
                self.push("\t\ts.push('[');");
                self.push(&format!(
                    "\t\tfor (i, entry) in self.{ident}.iter().enumerate() {{"
                ));
                self.push("\t\t\tif i > 0 { s.push(','); }");
                self.push("\t\t\tcambium_json_indent(&mut s, 2);");
                self.push("\t\t\tentry.write_json(&mut s, 2);");
                self.push("\t\t}");
                self.push("\t\tcambium_json_indent(&mut s, 1);");
                self.push("\t\ts.push(']');");
            }
            _ => {}
        }
    }

    /// Build the `&str`/`String` expression feeding `cambium_json_escape` for a
    /// JSON string leaf. Every arm yields a borrow or an owned `String` (never a
    /// move out of a shared reference), so the same expression is valid whether
    /// `value_ref` is `self.field` or a `&T` loop variable.
    fn json_string_value_expr(value_ref: &str, field: &Field<'ctx>) -> String {
        if field.is_union {
            // Union leaves own their value serialization; this helper is not used.
            format!("{value_ref}.write_json_value(...)")
        } else if field.is_enum {
            format!("{value_ref}.as_name()")
        } else if field.is_identityref {
            format!("{value_ref}.as_json_name()")
        } else if field_is_instance_identifier(field) {
            format!("{value_ref}.as_json_str()")
        } else if field.is_bits {
            format!("{value_ref}.to_string()")
        } else {
            // Plain and ranged strings: `.as_str()` borrows uniformly.
            format!("{value_ref}.as_str()")
        }
    }

    fn emit_scalar_json(&mut self, wire: &str, writer: &str, value_ref: &str, field: &Field<'ctx>) {
        self.push(&format!(
            "\t\tif std::mem::take(&mut first) {{}} else {{ {writer}.push(','); }}"
        ));
        // The nested writer `w` is a `&mut String` with a `depth` in scope; the
        // top-level writer `s` is owned (needs `&mut`) and has no `depth` (indent 1).
        let (indent_writer, indent_depth) = if writer == "w" {
            (writer.to_string(), "depth + 1".to_string())
        } else {
            (format!("&mut {writer}"), "1".to_string())
        };
        self.push(&format!(
            "\t\tcambium_json_indent({indent_writer}, {indent_depth});"
        ));
        if field.is_union {
            self.needs_json_escape = true;
            self.push(&format!(
                "\t\t{writer}.push_str(&format!(\"\\\"{wire}\\\": \"));"
            ));
            let writer_arg = if writer == "s" {
                "&mut s".to_string()
            } else {
                writer.to_string()
            };
            self.push(&format!("\t\t{value_ref}.write_json_value({writer_arg});"));
            return;
        }
        match field.json_kind {
            JsonKind::String => {
                self.needs_json_escape = true;
                let value_expr = Self::json_string_value_expr(value_ref, field);
                self.push(&format!(
                    "\t\t{writer}.push_str(&format!(\"\\\"{wire}\\\": {{}}\", cambium_json_escape({value_expr})));"
                ));
            }
            JsonKind::BareNumber => self.push(&format!(
                "\t\t{writer}.push_str(&format!(\"\\\"{wire}\\\": {{}}\", {value_ref}));"
            )),
            JsonKind::QuotedNumber => self.push(&format!(
                "\t\t{writer}.push_str(&format!(\"\\\"{wire}\\\": \\\"{{}}\\\"\", {value_ref}));"
            )),
            JsonKind::Bool => self.push(&format!(
                "\t\t{writer}.push_str(&format!(\"\\\"{wire}\\\": {{}}\", {value_ref}));"
            )),
        }
    }

    fn emit_json_leaf_list_element(&mut self, value_ref: &str, field: &Field<'ctx>) {
        if field.is_union {
            self.needs_json_escape = true;
            self.push(&format!("\t\t\t{value_ref}.write_json_value(w);"));
            return;
        }
        match field.json_kind {
            JsonKind::String => {
                self.needs_json_escape = true;
                let value_expr = Self::json_string_value_expr(value_ref, field);
                self.push(&format!(
                    "\t\t\tw.push_str(&cambium_json_escape({value_expr}));"
                ));
            }
            JsonKind::BareNumber => self.push(&format!(
                "\t\t\tw.push_str(&format!(\"{{}}\", {value_ref}));"
            )),
            JsonKind::QuotedNumber => self.push(&format!(
                "\t\t\tw.push_str(&format!(\"\\\"{{}}\\\"\", {value_ref}));"
            )),
            JsonKind::Bool => self.push(&format!(
                "\t\t\tw.push_str(&format!(\"{{}}\", {value_ref}));"
            )),
        }
    }

    fn emit_json_leaf_list_element_top(&mut self, value_ref: &str, field: &Field<'ctx>) {
        if field.is_union {
            self.needs_json_escape = true;
            self.push(&format!("\t\t\t{value_ref}.write_json_value(s);"));
            return;
        }
        match field.json_kind {
            JsonKind::String => {
                self.needs_json_escape = true;
                let value_expr = Self::json_string_value_expr(value_ref, field);
                self.push(&format!(
                    "\t\t\ts.push_str(&cambium_json_escape({value_expr}));"
                ));
            }
            JsonKind::BareNumber => self.push(&format!(
                "\t\t\ts.push_str(&format!(\"{{}}\", {value_ref}));"
            )),
            JsonKind::QuotedNumber => self.push(&format!(
                "\t\t\ts.push_str(&format!(\"\\\"{{}}\\\"\", {value_ref}));"
            )),
            JsonKind::Bool => self.push(&format!(
                "\t\t\ts.push_str(&format!(\"{{}}\", {value_ref}));"
            )),
        }
    }

    fn push(&mut self, line: &str) {
        self.output.push_str(line);
        self.output.push('\n');
    }
}

struct Field<'ctx> {
    node: SchemaNodeRef<'ctx>,
    ident: String,
    ty: String,
    optional: bool,
    is_enum: bool,
    is_bits: bool,
    is_identityref: bool,
    is_union: bool,
    json_kind: JsonKind,
    description: Option<String>,
}

impl<'ctx> Field<'ctx> {
    fn doc_text(&self) -> String {
        if let Some(desc) = &self.description {
            let escaped = desc.replace('\n', "\n\t/// ");
            format!("/// {escaped}")
        } else {
            format!(
                "/// Generated YANG {kind} `{name}`.",
                kind = kind_name(self.node.kind()),
                name = self.node.name()
            )
        }
    }
}

fn xml_value_expr(value_ref: &str, field: &Field<'_>) -> (String, bool) {
    if field.is_union {
        // The union enum owns its XML value serialization; namespace bindings are
        // emitted by the surrounding leaf writer when the active variant needs one.
        return (format!("{value_ref}.write_xml_value()"), false);
    }
    if field.is_enum || field.is_identityref {
        (
            format!("cambium_xml_escape_text({value_ref}.as_name())"),
            true,
        )
    } else if field_is_instance_identifier(field) {
        (format!("{value_ref}.write_xml_value()"), false)
    } else if is_string_like(field) {
        (
            format!("cambium_xml_escape_text(&{value_ref}.to_string())"),
            true,
        )
    } else {
        // Numbers/bool/decimal64 implement Display: emit the value directly so the
        // generated `format!` arg has no `.to_string()` (clippy::to_string_in_format_args).
        (value_ref.to_string(), false)
    }
}

fn field_is_instance_identifier(field: &Field<'_>) -> bool {
    field
        .node
        .leaf_type()
        .is_some_and(|info| type_is_instance_identifier(&info))
}

fn type_is_instance_identifier(info: &TypeInfo<'_>) -> bool {
    match info.resolved() {
        ResolvedType::LeafRef {
            realtype: Some(rt), ..
        } => type_is_instance_identifier(rt),
        ResolvedType::InstanceIdentifier { .. } => true,
        _ => false,
    }
}

/// Collect the `(module, name, variant, prefix, namespace)` tuples for an
/// identityref enum: the base identity plus its transitive derived closure,
/// deduplicated and sorted by `module:name`. Returns empty when the schema
/// layer exposes no data-valid identities from implemented modules, which the
/// caller treats as a String fallback.
fn collect_identityref_members(
    bases: &[cambium_core::Identity<'_>],
) -> Vec<(String, String, String, String, String)> {
    let mut used_variants: HashSet<String> = HashSet::new();
    let mut seen: HashSet<(String, String)> = HashSet::new();
    let mut identities: Vec<(String, String, String, String, String)> = Vec::new();

    for base in bases {
        let mut add = |id: &cambium_core::Identity<'_>| {
            if !id.module().is_implemented() {
                return;
            }
            let module = id.module().name().to_string();
            let name = id.name().to_string();
            if seen.insert((module.clone(), name.clone())) {
                let variant = safe_variant_name(&name, &mut used_variants);
                let prefix = id.module().prefix().to_string();
                let namespace = id.module().namespace().to_string();
                identities.push((module, name, variant, prefix, namespace));
            }
        };
        add(base);
        for derived in base.derived() {
            add(&derived);
        }
    }

    identities.sort_by(|a, b| {
        let a_key = format!("{}:{}", a.0, a.1);
        let b_key = format!("{}:{}", b.0, b.1);
        a_key.cmp(&b_key)
    });
    identities
}

fn is_string_like(field: &Field<'_>) -> bool {
    let info = match field.node.leaf_type() {
        Some(i) => i,
        None => return true,
    };
    matches!(
        info.resolved(),
        ResolvedType::StringType { .. }
            | ResolvedType::Binary { .. }
            | ResolvedType::InstanceIdentifier { .. }
            | ResolvedType::Empty
            | ResolvedType::Unknown
            | ResolvedType::Bits(_)
            | ResolvedType::IdentityRef { .. }
            | ResolvedType::Union(_)
            | ResolvedType::LeafRef { .. }
    )
}

/// Derive a PascalCase label for one union member, used for both the variant
/// name and the nested helper-type name suffix. Typedef'd members are named by
/// their typedef; built-in members are named by their base type.
fn union_member_label(info: &TypeInfo<'_>) -> String {
    if let Some(name) = info.typedef_name() {
        return to_pascal_case(name);
    }
    match info.resolved() {
        ResolvedType::Boolean => "Boolean".to_string(),
        ResolvedType::Int { kind, .. } => int_kind_label(*kind).to_string(),
        ResolvedType::Decimal64 { .. } => "Decimal64".to_string(),
        ResolvedType::Enumeration(_) => "Enumeration".to_string(),
        ResolvedType::Bits(_) => "Bits".to_string(),
        ResolvedType::IdentityRef { .. } => "Identityref".to_string(),
        ResolvedType::StringType { .. } => "String".to_string(),
        ResolvedType::Binary { .. } => "Binary".to_string(),
        ResolvedType::InstanceIdentifier { .. } => "InstanceIdentifier".to_string(),
        ResolvedType::Empty => "Empty".to_string(),
        ResolvedType::LeafRef { .. } => "Leafref".to_string(),
        ResolvedType::Union(_) => "Union".to_string(),
        ResolvedType::Unknown => "Unknown".to_string(),
        _ => "Unknown".to_string(),
    }
}

fn int_kind_label(kind: IntKind) -> &'static str {
    match kind {
        IntKind::I8 => "Int8",
        IntKind::I16 => "Int16",
        IntKind::I32 => "Int32",
        IntKind::I64 => "Int64",
        IntKind::U8 => "Uint8",
        IntKind::U16 => "Uint16",
        IntKind::U32 => "Uint32",
        IntKind::U64 => "Uint64",
        _ => "Int",
    }
}

fn int_kind_rust_type(kind: IntKind) -> String {
    match kind {
        IntKind::I8 => "i8",
        IntKind::I16 => "i16",
        IntKind::I32 => "i32",
        IntKind::I64 => "i64",
        IntKind::U8 => "u8",
        IntKind::U16 => "u16",
        IntKind::U32 => "u32",
        IntKind::U64 => "u64",
        _ => "i64",
    }
    .to_string()
}

fn int_type_limits(kind: IntKind) -> (i128, i128) {
    match kind {
        IntKind::I8 => (i128::from(i8::MIN), i128::from(i8::MAX)),
        IntKind::I16 => (i128::from(i16::MIN), i128::from(i16::MAX)),
        IntKind::I32 => (i128::from(i32::MIN), i128::from(i32::MAX)),
        IntKind::I64 => (i128::from(i64::MIN), i128::from(i64::MAX)),
        IntKind::U8 => (i128::from(u8::MIN), i128::from(u8::MAX)),
        IntKind::U16 => (i128::from(u16::MIN), i128::from(u16::MAX)),
        IntKind::U32 => (i128::from(u32::MIN), i128::from(u32::MAX)),
        IntKind::U64 => (i128::from(u64::MIN), i128::from(u64::MAX)),
        _ => (i128::from(i64::MIN), i128::from(i64::MAX)),
    }
}

fn parse_int_bound(s: &str, type_min: i128, type_max: i128) -> i128 {
    if s == "min" {
        type_min
    } else if s == "max" {
        type_max
    } else {
        s.parse::<i128>().unwrap_or(type_min)
    }
}

fn parse_length_bound(s: &str) -> usize {
    if s == "min" {
        0
    } else if s == "max" {
        usize::MAX
    } else {
        s.parse::<usize>().unwrap_or(0)
    }
}

fn data_children<'a>(children: impl Iterator<Item = SchemaNodeRef<'a>>) -> Vec<SchemaNodeRef<'a>> {
    children.filter(|c| is_data_node(c.kind())).collect()
}

fn is_data_node(kind: SchemaNodeKind) -> bool {
    matches!(
        kind,
        SchemaNodeKind::Container
            | SchemaNodeKind::List
            | SchemaNodeKind::Leaf
            | SchemaNodeKind::LeafList
    )
}

fn is_struct_kind(kind: SchemaNodeKind) -> bool {
    matches!(kind, SchemaNodeKind::Container | SchemaNodeKind::List)
}

fn ordered_children(node: SchemaNodeRef<'_>) -> Vec<SchemaNodeRef<'_>> {
    let all = data_children(node.children());
    if node.kind() != SchemaNodeKind::List {
        return all;
    }

    let key_nodes: Vec<_> = node.list_keys().collect();
    let mut keys: Vec<_> = all
        .iter()
        .copied()
        .filter(|c| key_nodes.contains(c))
        .collect();
    let others: Vec<_> = all
        .iter()
        .copied()
        .filter(|c| !key_nodes.contains(c))
        .collect();

    let mut ordered_keys = Vec::with_capacity(keys.len());
    for key in key_nodes {
        if let Some(pos) = keys.iter().position(|k| *k == key) {
            ordered_keys.push(keys.remove(pos));
        }
    }
    ordered_keys.extend(keys); // defensive
    ordered_keys.extend(others);
    ordered_keys
}

fn safe_field_ident(name: &str, used: &mut HashSet<String>) -> Result<String> {
    let mut base = name.to_string();
    if base.is_empty() {
        base = "_empty".to_string();
    }
    if base.as_bytes().first().is_some_and(|b| b.is_ascii_digit()) {
        base = format!("_{base}");
    }
    base = base.replace(['-', '.'], "_");

    if is_rust_keyword(&base) {
        if is_raw_forbidden(&base) {
            base.push('_');
        } else {
            base = format!("r#{base}");
        }
    }

    let mut candidate = base.clone();
    let mut suffix = 2u32;
    while !used.insert(candidate.clone()) {
        candidate = format!("{base}_{suffix}");
        suffix += 1;
        if suffix > 100_000 {
            return Err(Error::IdentCollision {
                a: name.to_string(),
                b: name.to_string(),
                ident: candidate,
            });
        }
    }

    Ok(candidate)
}

fn safe_variant_name(name: &str, used: &mut HashSet<String>) -> String {
    let mut base = to_pascal_case(name);
    if base == "Self" {
        base.push('_');
    }

    let mut candidate = base.clone();
    let mut suffix = 2u32;
    while !used.insert(candidate.clone()) {
        candidate = format!("{base}_{suffix}");
        suffix += 1;
    }
    candidate
}

/// Like [`safe_variant_name`] but keeps the name camel-case clean for clippy
/// (`Identityref2` instead of `Identityref_2`).
fn safe_union_variant_name(name: &str, used: &mut HashSet<String>) -> String {
    let mut base = to_pascal_case(name);
    if base == "Self" {
        base.push('_');
    }

    let mut candidate = base.clone();
    let mut suffix = 2u32;
    while !used.insert(candidate.clone()) {
        candidate = format!("{base}{suffix}");
        suffix += 1;
    }
    candidate
}

fn is_rust_keyword(s: &str) -> bool {
    matches!(
        s,
        "as" | "break"
            | "const"
            | "continue"
            | "crate"
            | "else"
            | "enum"
            | "extern"
            | "false"
            | "fn"
            | "for"
            | "if"
            | "impl"
            | "in"
            | "let"
            | "loop"
            | "match"
            | "mod"
            | "move"
            | "mut"
            | "pub"
            | "ref"
            | "return"
            | "self"
            | "Self"
            | "static"
            | "struct"
            | "super"
            | "trait"
            | "true"
            | "type"
            | "unsafe"
            | "use"
            | "where"
            | "while"
            | "async"
            | "await"
            | "dyn"
            | "abstract"
            | "become"
            | "box"
            | "do"
            | "final"
            | "macro"
            | "override"
            | "priv"
            | "typeof"
            | "unsized"
            | "virtual"
            | "yield"
    )
}

fn is_raw_forbidden(s: &str) -> bool {
    matches!(s, "crate" | "self" | "super" | "Self")
}

fn to_pascal_case(s: &str) -> String {
    // Strip a leading raw-identifier prefix so keyword fields like `r#ref`
    // produce valid PascalCase type names.
    let s = s.strip_prefix("r#").unwrap_or(s);
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
    if out.as_bytes().first().is_some_and(|b| b.is_ascii_digit()) {
        out = format!("_{out}");
    }
    if out == "Self" {
        out.push('_');
    }
    out
}

const XML_ESCAPE_HELPER: &str = r#"/// Escape `&`, `<`, and `>` for XML text nodes.
fn cambium_xml_escape_text(s: &str) -> String {
    let mut out = String::with_capacity(s.len());
    for c in s.chars() {
        match c {
            '&' => out.push_str("&amp;"),
            '<' => out.push_str("&lt;"),
            '>' => out.push_str("&gt;"),
            _ => out.push(c),
        }
    }
    out
}
"#;

const JSON_INDENT_HELPER: &str = r#"/// Write a newline followed by `depth` two-space indents.
fn cambium_json_indent(w: &mut String, depth: usize) {
    w.push('\n');
    for _ in 0..depth {
        w.push_str("  ");
    }
}
"#;

const JSON_ESCAPE_HELPER: &str = r#"/// Render a string as a JSON string literal, including the surrounding quotes.
///
/// Matches libyang's `json_print_string` byte-for-byte: only `"`, `\`, `\r`, and
/// `\t` use named escapes; every other control character (including `\n`, `\b`,
/// `\f`, and DEL `0x7F`) is emitted as `\uXXXX` with uppercase hex; all other
/// characters (including multi-byte UTF-8) pass through unchanged.
fn cambium_json_escape<S: AsRef<str>>(s: S) -> String {
    let s = s.as_ref();
    let mut out = String::with_capacity(s.len() + 2);
    out.push('"');
    for c in s.chars() {
        match c {
            '"' => out.push_str("\\\""),
            '\\' => out.push_str("\\\\"),
            '\r' => out.push_str("\\r"),
            '\t' => out.push_str("\\t"),
            c if c.is_ascii_control() => out.push_str(&format!("\\u{:04X}", c as u32)),
            c => out.push(c),
        }
    }
    out.push('"');
    out
}
"#;

const INSTANCE_IDENTIFIER_HELPER: &str = r#"/// `instance-identifier` lexical value used by generated structs.
///
/// XML and JSON_IETF use different prefix rules, so generated values keep both
/// spellings instead of treating the value as a plain string.
#[derive(Debug, Clone, Default, PartialEq, Eq)]
pub struct InstanceIdentifier {
    xml: String,
    json: String,
    xml_prefix: Option<String>,
    xml_namespace: Option<String>,
}

impl InstanceIdentifier {
    /// Create an instance-identifier with separate XML and JSON_IETF lexical forms.
    pub fn new(xml: impl Into<String>, json: impl Into<String>) -> Self {
        Self {
            xml: xml.into(),
            json: json.into(),
            xml_prefix: None,
            xml_namespace: None,
        }
    }

    /// Create an instance-identifier with the XML namespace binding used by its value.
    pub fn with_xmlns(
        xml: impl Into<String>,
        json: impl Into<String>,
        prefix: impl Into<String>,
        namespace: impl Into<String>,
    ) -> Self {
        Self {
            xml: xml.into(),
            json: json.into(),
            xml_prefix: Some(prefix.into()),
            xml_namespace: Some(namespace.into()),
        }
    }

    /// Create an instance-identifier whose XML and JSON_IETF forms are identical.
    pub fn same(value: impl Into<String>) -> Self {
        let value = value.into();
        Self {
            xml: value.clone(),
            json: value,
            xml_prefix: None,
            xml_namespace: None,
        }
    }

    /// Return the XML lexical form.
    pub fn as_xml_str(&self) -> &str {
        &self.xml
    }

    /// Return the JSON_IETF lexical form.
    pub fn as_json_str(&self) -> &str {
        &self.json
    }

    /// Return the optional XML namespace binding required by the XML lexical form.
    pub fn xml_prefix_ns(&self) -> Option<(&str, &str)> {
        match (&self.xml_prefix, &self.xml_namespace) {
            (Some(prefix), Some(namespace)) => Some((prefix.as_str(), namespace.as_str())),
            _ => None,
        }
    }

    /// Write the escaped XML value.
    pub fn write_xml_value(&self) -> String {
        cambium_xml_escape_text(&self.xml)
    }
}

impl From<String> for InstanceIdentifier {
    fn from(value: String) -> Self {
        Self::same(value)
    }
}

impl From<&str> for InstanceIdentifier {
    fn from(value: &str) -> Self {
        Self::same(value)
    }
}

impl std::fmt::Display for InstanceIdentifier {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        write!(f, "{}", self.xml)
    }
}
"#;

const DECIMAL64_HELPER: &str = r#"/// Fixed-point `decimal64` value used by generated structs.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub struct Decimal64 {
    raw: i64,
    fraction_digits: u8,
}

impl Decimal64 {
    /// Create a new fixed-point decimal64 value.
    pub fn new(raw: i64, fraction_digits: u8) -> Self {
        Self { raw, fraction_digits }
    }

    /// The raw fixed-point integer.
    pub fn raw(&self) -> i64 {
        self.raw
    }

    /// The number of fractional digits (1..=18).
    pub fn fraction_digits(&self) -> u8 {
        self.fraction_digits
    }
}

impl Default for Decimal64 {
    fn default() -> Self {
        Self::new(0, 1)
    }
}

impl std::fmt::Display for Decimal64 {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        if self.fraction_digits == 0 {
            return write!(f, "{}", self.raw);
        }
        let divisor = 10i64.pow(u32::from(self.fraction_digits));
        let whole = (self.raw / divisor).unsigned_abs();
        let frac = (self.raw % divisor).unsigned_abs();
        let padded = format!("{:0>width$}", frac, width = usize::from(self.fraction_digits));
        // RFC-7950 canonical form strips trailing fractional zeros, keeping >= 1 digit.
        let trimmed = padded.trim_end_matches('0');
        let frac_str = if trimmed.is_empty() { "0" } else { trimmed };
        if self.raw < 0 {
            write!(f, "-{whole}.{frac_str}")
        } else {
            write!(f, "{whole}.{frac_str}")
        }
    }
}"#;

const USER_ORDERED_VEC_HELPER: &str = r#"/// Positional-only ordered container for `ordered-by user` data nodes.
#[derive(Debug, Clone, Default, PartialEq, Eq)]
pub struct UserOrderedVec<T> {
    items: Vec<T>,
}

impl<T> UserOrderedVec<T> {
    /// Create an empty container.
    pub fn new() -> Self {
        Self { items: Vec::new() }
    }

    /// Insert `value` at the front of the list.
    pub fn insert_first(&mut self, value: T) {
        self.items.insert(0, value);
    }

    /// Insert `value` at the back of the list.
    pub fn insert_last(&mut self, value: T) {
        self.items.push(value);
    }

    /// Insert `value` immediately before `index`.
    pub fn insert_before(&mut self, index: usize, value: T) {
        self.items.insert(index, value);
    }

    /// Insert `value` immediately after `index`.
    pub fn insert_after(&mut self, index: usize, value: T) {
        self.items.insert(index + 1, value);
    }

    /// Move the element at `what` to immediately before `point`.
    pub fn move_before(&mut self, what: usize, point: usize) {
        if what == point {
            return;
        }
        let item = self.items.remove(what);
        let insert_at = if what < point { point - 1 } else { point };
        self.items.insert(insert_at, item);
    }

    /// Move the element at `what` to immediately after `point`.
    pub fn move_after(&mut self, what: usize, point: usize) {
        if what == point {
            return;
        }
        let item = self.items.remove(what);
        let insert_at = if what < point { point } else { point + 1 };
        self.items.insert(insert_at, item);
    }

    /// Remove and return the element at `index`.
    pub fn remove(&mut self, index: usize) -> T {
        self.items.remove(index)
    }

    /// Return the number of elements.
    pub fn len(&self) -> usize {
        self.items.len()
    }

    /// Return true if there are no elements.
    pub fn is_empty(&self) -> bool {
        self.items.is_empty()
    }

    /// Borrow the element at `index`, if present.
    pub fn get(&self, index: usize) -> Option<&T> {
        self.items.get(index)
    }

    /// Iterate over the elements in their current order.
    pub fn iter(&self) -> impl Iterator<Item = &T> {
        self.items.iter()
    }
}

/// Convert a `Vec<T>` into a `UserOrderedVec<T>`.
impl<T> From<Vec<T>> for UserOrderedVec<T> {
    fn from(items: Vec<T>) -> Self {
        Self { items }
    }
}
"#;

fn kind_name(kind: SchemaNodeKind) -> &'static str {
    match kind {
        SchemaNodeKind::Container => "container",
        SchemaNodeKind::List => "list",
        SchemaNodeKind::Leaf => "leaf",
        SchemaNodeKind::LeafList => "leaf-list",
        SchemaNodeKind::Choice => "choice",
        SchemaNodeKind::Case => "case",
        SchemaNodeKind::AnyData => "anydata",
        SchemaNodeKind::Rpc => "rpc",
        SchemaNodeKind::Notification => "notification",
        SchemaNodeKind::Module => "module",
        _ => "node",
    }
}
