//! Coarse-grained, safe-ish adapter over libyang.
//!
//! This module deliberately does **not** expose raw libyang types (`ly_ctx`,
//! `lyd_node`, etc.) in its public signatures. It exists so that `cambium-core`
//! can remain free of libyang/cgo types per the hexagonal rule. Calls are
//! whole-document only — no per-node FFI, no C→Rust callbacks.

use std::collections::HashMap;
use std::ffi::{CStr, CString};
use std::fs;
use std::path::Path;

use crate::*;

unsafe extern "C" {
    fn cam_lyd_get_value(node: *const lyd_node) -> *const ::std::os::raw::c_char;
    fn cam_lyd_get_meta_value(meta: *const lyd_meta) -> *const ::std::os::raw::c_char;
}

/// Number of elements in a libyang sized array.
///
/// libyang stores the element count in a `LY_ARRAY_COUNT_TYPE` (`uint64_t`)
/// immediately before the array pointer. The array pointer itself is never
/// null-terminated.
unsafe fn sized_array_count<T>(ptr: *const T) -> usize {
    if ptr.is_null() {
        0
    } else {
        unsafe { *((ptr as *const u64).offset(-1)) as usize }
    }
}

unsafe fn cstr_opt(ptr: *const ::std::os::raw::c_char) -> Option<String> {
    if ptr.is_null() {
        None
    } else {
        Some(unsafe { CStr::from_ptr(ptr).to_string_lossy().into_owned() })
    }
}

unsafe fn module_name_array(mods: *mut *mut lys_module) -> Vec<String> {
    let count = unsafe { sized_array_count(mods) };
    let mut out = Vec::with_capacity(count);
    for i in 0..count {
        let module = unsafe { *mods.add(i) };
        if !module.is_null() {
            if let Some(name) = unsafe { cstr_opt((*module).name) } {
                out.push(name);
            }
        }
    }
    out
}

unsafe fn lys_module_name(module: *const lys_module) -> String {
    if module.is_null() {
        String::new()
    } else {
        unsafe { cstr_opt((*module).name).unwrap_or_default() }
    }
}

unsafe fn lys_module_namespace(module: *const lys_module) -> String {
    if module.is_null() {
        String::new()
    } else {
        unsafe { cstr_opt((*module).ns).unwrap_or_default() }
    }
}

// -----------------------------------------------------------------------------
// Data-node helpers (reimplementing the `static inline` libyang accessors).
// -----------------------------------------------------------------------------

unsafe fn node_is_inner(node: *const lyd_node) -> bool {
    if node.is_null() {
        return false;
    }
    let schema = unsafe { (*node).schema };
    if schema.is_null() {
        return false;
    }
    let nt = unsafe { (*schema).nodetype } as u32;
    nt == LYS_CONTAINER || nt == LYS_LIST || nt == LYS_RPC || nt == LYS_ACTION || nt == LYS_NOTIF
}

unsafe fn node_is_term(node: *const lyd_node) -> bool {
    if node.is_null() {
        return false;
    }
    let schema = unsafe { (*node).schema };
    if schema.is_null() {
        return false;
    }
    let nt = unsafe { (*schema).nodetype } as u32;
    nt == LYS_LEAF || nt == LYS_LEAFLIST
}

unsafe fn node_child_first(node: *const lyd_node) -> Option<*mut lyd_node> {
    if !unsafe { node_is_inner(node) } {
        return None;
    }
    Some(unsafe { (*node.cast::<lyd_node_inner>()).child })
}

unsafe fn node_schema_name(node: *const lyd_node) -> Option<String> {
    let schema = unsafe { (*node).schema };
    if schema.is_null() {
        return None;
    }
    unsafe { cstr_opt((*schema).name) }
}

/// Read the canonical value string through libyang's public value accessor.
/// Returns `None` for non-term nodes or missing values.
unsafe fn node_value_str_direct(node: *const lyd_node) -> Option<String> {
    if !unsafe { node_is_term(node) } {
        return None;
    }
    unsafe { cstr_opt(cam_lyd_get_value(node)) }
}

/// Build an absolute data path for `node` by walking the data parent chain.
/// This uses only pointer dereferences plus `lyd_get_value` for keys; it does
/// **not** call `lyd_path`.
unsafe fn node_path(node: *const lyd_node) -> Option<String> {
    let schema = unsafe { (*node).schema };
    if schema.is_null() {
        return None;
    }
    let module = unsafe { (*schema).module };
    if module.is_null() {
        return None;
    }
    let module_name = unsafe { cstr_opt((*module).name) }?;

    let mut segments: Vec<String> = Vec::new();
    let mut cur = node;
    while !cur.is_null() {
        let cur_schema = unsafe { (*cur).schema };
        if cur_schema.is_null() {
            break;
        }
        let name = unsafe { cstr_opt((*cur_schema).name) }?;
        let nt = unsafe { (*cur_schema).nodetype } as u32;
        let segment = if nt == LYS_LIST {
            let mut preds = String::new();
            if let Some(first_child) = unsafe { node_child_first(cur) } {
                let mut key = first_child;
                while !key.is_null() {
                    let key_schema = unsafe { (*key).schema };
                    if !key_schema.is_null()
                        && (unsafe { (*key_schema).flags } as u32 & LYS_KEY) != 0
                    {
                        let key_name = unsafe { cstr_opt((*key_schema).name) }?;
                        if let Some(key_val) = unsafe { node_value_str_direct(key) } {
                            preds.push_str(&format!("[{key_name}='{key_val}']"));
                        }
                    }
                    key = unsafe { (*key).next };
                }
            }
            format!("{name}{preds}")
        } else {
            name
        };
        segments.push(segment);
        cur = unsafe { (*cur).parent };
    }
    segments.reverse();
    Some(format!("/{module_name}:{}", segments.join("/")))
}

unsafe fn node_name(node: *const lysc_node) -> String {
    if node.is_null() {
        return String::new();
    }
    let name = unsafe { (*node).name };
    if name.is_null() {
        return String::new();
    }
    unsafe { CStr::from_ptr(name).to_string_lossy().into_owned() }
}

unsafe fn node_description(node: *const lysc_node) -> Option<String> {
    unsafe { cstr_opt((*node).dsc) }
}

unsafe fn node_reference(node: *const lysc_node) -> Option<String> {
    unsafe { cstr_opt((*node).ref_) }
}

unsafe fn node_config(flags: u32) -> RawConfig {
    match flags & LYS_CONFIG_MASK {
        n if n == LYS_CONFIG_R => RawConfig::Ro,
        n if n == LYS_CONFIG_W => RawConfig::Rw,
        _ => RawConfig::Unset,
    }
}

unsafe fn node_status(flags: u32) -> RawStatus {
    match flags & LYS_STATUS_MASK {
        n if n == LYS_STATUS_DEPRC => RawStatus::Deprecated,
        n if n == LYS_STATUS_OBSLT => RawStatus::Obsolete,
        _ => RawStatus::Current,
    }
}

unsafe fn node_kind(node: *const lysc_node) -> &'static str {
    if node.is_null() {
        return "unknown";
    }
    let nt = unsafe { (*node).nodetype } as u32;
    match nt {
        n if n == LYS_CONTAINER => "container",
        n if n == LYS_LEAF => "leaf",
        n if n == LYS_LEAFLIST => "leaflist",
        n if n == LYS_LIST => "list",
        n if n == LYS_CHOICE => "choice",
        n if n == LYS_CASE => "case",
        // anyxml (0x20) and anydata (0x60) are distinct nodetype values; both
        // surface as the AnyData kind. Matching only 0x60 misclassifies anyxml.
        n if n == LYS_ANYXML => "anydata",
        n if n == LYS_ANYDATA => "anydata",
        n if n == LYS_RPC => "rpc",
        n if n == LYS_ACTION => "action",
        n if n == LYS_INPUT => "input",
        n if n == LYS_OUTPUT => "output",
        n if n == LYS_NOTIF => "notification",
        _ => "unknown",
    }
}

unsafe fn leaf_type_base(node: *const lysc_node) -> RawBaseType {
    if node.is_null() {
        return RawBaseType::Unknown;
    }
    let nt = unsafe { (*node).nodetype } as u32;
    let type_ptr = if nt == LYS_LEAF {
        unsafe { (*(node as *const lysc_node_leaf)).type_ }
    } else if nt == LYS_LEAFLIST {
        unsafe { (*(node as *const lysc_node_leaflist)).type_ }
    } else {
        std::ptr::null_mut()
    };
    if type_ptr.is_null() {
        return RawBaseType::Unknown;
    }
    let basetype = unsafe { (*type_ptr).basetype };
    match basetype {
        LY_DATA_TYPE::LY_TYPE_STRING => RawBaseType::String,
        LY_DATA_TYPE::LY_TYPE_BOOL => RawBaseType::Boolean,
        LY_DATA_TYPE::LY_TYPE_INT8 => RawBaseType::Int8,
        LY_DATA_TYPE::LY_TYPE_INT16 => RawBaseType::Int16,
        LY_DATA_TYPE::LY_TYPE_INT32 => RawBaseType::Int32,
        LY_DATA_TYPE::LY_TYPE_INT64 => RawBaseType::Int64,
        LY_DATA_TYPE::LY_TYPE_UINT8 => RawBaseType::Uint8,
        LY_DATA_TYPE::LY_TYPE_UINT16 => RawBaseType::Uint16,
        LY_DATA_TYPE::LY_TYPE_UINT32 => RawBaseType::Uint32,
        LY_DATA_TYPE::LY_TYPE_UINT64 => RawBaseType::Uint64,
        LY_DATA_TYPE::LY_TYPE_DEC64 => RawBaseType::Decimal64,
        LY_DATA_TYPE::LY_TYPE_EMPTY => RawBaseType::Empty,
        LY_DATA_TYPE::LY_TYPE_BINARY => RawBaseType::Binary,
        LY_DATA_TYPE::LY_TYPE_BITS => RawBaseType::Bits,
        LY_DATA_TYPE::LY_TYPE_ENUM => RawBaseType::Enumeration,
        LY_DATA_TYPE::LY_TYPE_IDENT => RawBaseType::IdentityRef,
        LY_DATA_TYPE::LY_TYPE_INST => RawBaseType::InstanceIdentifier,
        LY_DATA_TYPE::LY_TYPE_LEAFREF => RawBaseType::LeafRef,
        LY_DATA_TYPE::LY_TYPE_UNION => RawBaseType::Union,
        _ => RawBaseType::Unknown,
    }
}

unsafe fn leaf_typedef_name(node: *const lysc_node) -> Option<String> {
    if node.is_null() {
        return None;
    }
    let nt = unsafe { (*node).nodetype } as u32;
    let type_ptr = if nt == LYS_LEAF {
        unsafe { (*(node as *const lysc_node_leaf)).type_ }
    } else if nt == LYS_LEAFLIST {
        unsafe { (*(node as *const lysc_node_leaflist)).type_ }
    } else {
        std::ptr::null_mut()
    };
    if type_ptr.is_null() {
        return None;
    }
    let name = unsafe { cstr_opt((*type_ptr).name) };
    let basetype = unsafe { (*type_ptr).basetype };
    // If the type name is the built-in keyword, it is not a typedef.
    let builtin_name = match basetype {
        LY_DATA_TYPE::LY_TYPE_STRING => Some("string"),
        LY_DATA_TYPE::LY_TYPE_BOOL => Some("boolean"),
        LY_DATA_TYPE::LY_TYPE_INT8 => Some("int8"),
        LY_DATA_TYPE::LY_TYPE_INT16 => Some("int16"),
        LY_DATA_TYPE::LY_TYPE_INT32 => Some("int32"),
        LY_DATA_TYPE::LY_TYPE_INT64 => Some("int64"),
        LY_DATA_TYPE::LY_TYPE_UINT8 => Some("uint8"),
        LY_DATA_TYPE::LY_TYPE_UINT16 => Some("uint16"),
        LY_DATA_TYPE::LY_TYPE_UINT32 => Some("uint32"),
        LY_DATA_TYPE::LY_TYPE_UINT64 => Some("uint64"),
        LY_DATA_TYPE::LY_TYPE_DEC64 => Some("decimal64"),
        LY_DATA_TYPE::LY_TYPE_EMPTY => Some("empty"),
        LY_DATA_TYPE::LY_TYPE_BINARY => Some("binary"),
        LY_DATA_TYPE::LY_TYPE_BITS => Some("bits"),
        LY_DATA_TYPE::LY_TYPE_ENUM => Some("enumeration"),
        LY_DATA_TYPE::LY_TYPE_IDENT => Some("identityref"),
        LY_DATA_TYPE::LY_TYPE_INST => Some("instance-identifier"),
        LY_DATA_TYPE::LY_TYPE_LEAFREF => Some("leafref"),
        LY_DATA_TYPE::LY_TYPE_UNION => Some("union"),
        _ => None,
    };
    if name.as_deref() == builtin_name {
        None
    } else {
        name
    }
}

unsafe fn leaf_units(node: *const lysc_node) -> Option<String> {
    if node.is_null() {
        return None;
    }
    let nt = unsafe { (*node).nodetype } as u32;
    let units_ptr = if nt == LYS_LEAF {
        unsafe { (*(node as *const lysc_node_leaf)).units }
    } else if nt == LYS_LEAFLIST {
        unsafe { (*(node as *const lysc_node_leaflist)).units }
    } else {
        std::ptr::null()
    };
    unsafe { cstr_opt(units_ptr) }
}

unsafe fn leaf_defaults(node: *const lysc_node) -> Vec<String> {
    if node.is_null() {
        return Vec::new();
    }
    let nt = unsafe { (*node).nodetype } as u32;
    if nt == LYS_LEAF {
        let leaf = node as *const lysc_node_leaf;
        return unsafe { cstr_opt((*leaf).dflt.str_) }.into_iter().collect();
    }
    if nt == LYS_LEAFLIST {
        let leaflist = node as *const lysc_node_leaflist;
        let dflts = unsafe { (*leaflist).dflts };
        let count = unsafe { sized_array_count(dflts) };
        let mut out = Vec::with_capacity(count);
        for i in 0..count {
            let value = unsafe { dflts.add(i) };
            if value.is_null() {
                continue;
            }
            if let Some(default) = unsafe { cstr_opt((*value).str_) } {
                out.push(default);
            }
        }
        return out;
    }
    Vec::new()
}

unsafe fn list_min_max(node: *const lysc_node) -> (Option<u32>, Option<u32>) {
    if node.is_null() {
        return (None, None);
    }
    let nt = unsafe { (*node).nodetype } as u32;
    let (min, max) = if nt == LYS_LIST {
        let list = node as *const lysc_node_list;
        unsafe { ((*list).min, (*list).max) }
    } else if nt == LYS_LEAFLIST {
        let list = node as *const lysc_node_leaflist;
        unsafe { ((*list).min, (*list).max) }
    } else {
        return (None, None);
    };
    // libyang uses 0 for unset min and UINT32_MAX for unset max.
    let min_out = if min == 0 { None } else { Some(min) };
    let max_out = if max == u32::MAX { None } else { Some(max) };
    (min_out, max_out)
}

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
enum RangeKind {
    Signed,
    Unsigned,
    Decimal64(u8),
}

unsafe fn format_identity_name(ident: *const lysc_ident) -> String {
    if ident.is_null() {
        return String::new();
    }
    let name = unsafe { cstr_opt((*ident).name).unwrap_or_default() };
    let mod_name = unsafe {
        if (*ident).module.is_null() {
            String::new()
        } else {
            cstr_opt((*(*ident).module).name).unwrap_or_default()
        }
    };
    if mod_name.is_empty() {
        name
    } else {
        format!("{mod_name}:{name}")
    }
}

unsafe fn extract_range(range_ptr: *const lysc_range, kind: RangeKind) -> Vec<RawRangeBound> {
    if range_ptr.is_null() {
        return Vec::new();
    }
    let count = unsafe { sized_array_count((*range_ptr).parts) };
    let mut out = Vec::with_capacity(count);
    for i in 0..count {
        let part = unsafe { (*range_ptr).parts.add(i) };
        if part.is_null() {
            continue;
        }
        let (min, max) = match kind {
            RangeKind::Signed => {
                let min = unsafe { (*part).__bindgen_anon_1.min_64 };
                let max = unsafe { (*part).__bindgen_anon_2.max_64 };
                (min.to_string(), max.to_string())
            }
            RangeKind::Unsigned => {
                let min = unsafe { (*part).__bindgen_anon_1.min_u64 };
                let max = unsafe { (*part).__bindgen_anon_2.max_u64 };
                (min.to_string(), max.to_string())
            }
            RangeKind::Decimal64(fd) => {
                let min = unsafe { (*part).__bindgen_anon_1.min_64 };
                let max = unsafe { (*part).__bindgen_anon_2.max_64 };
                (format_decimal64(min, fd), format_decimal64(max, fd))
            }
        };
        out.push(RawRangeBound { min, max });
    }
    out
}

fn format_decimal64(value: i64, fraction_digits: u8) -> String {
    if fraction_digits == 0 {
        return value.to_string();
    }
    let divisor = 10i64.pow(u32::from(fraction_digits));
    let whole = value / divisor;
    let frac = (value % divisor).unsigned_abs() as u64;
    let frac_str = format!("{:0>width$}", frac, width = usize::from(fraction_digits));
    if value < 0 {
        format!("-{whole}.{frac_str}")
    } else {
        format!("{whole}.{frac_str}")
    }
}

unsafe fn extract_patterns(patterns_ptr: *mut *mut lysc_pattern) -> Vec<RawPattern> {
    let count = unsafe { sized_array_count(patterns_ptr) };
    let mut out = Vec::with_capacity(count);
    for i in 0..count {
        let pattern = unsafe { *patterns_ptr.add(i) };
        if pattern.is_null() {
            continue;
        }
        let regex = unsafe { cstr_opt((*pattern).expr).unwrap_or_default() };
        let error_app_tag = unsafe { cstr_opt((*pattern).eapptag) };
        let inverted = unsafe { (*pattern).inverted() != 0 };
        out.push(RawPattern {
            regex,
            error_app_tag,
            inverted,
        });
    }
    out
}

unsafe fn extract_musts(musts_ptr: *mut lysc_must) -> Vec<RawMustConstraint> {
    let count = unsafe { sized_array_count(musts_ptr) };
    let mut out = Vec::with_capacity(count);
    for i in 0..count {
        let must = unsafe { musts_ptr.add(i) };
        if must.is_null() {
            continue;
        }
        out.push(RawMustConstraint {
            expression: unsafe { cstr_opt(lyxp_get_expr((*must).cond)).unwrap_or_default() },
            error_message: unsafe { cstr_opt((*must).emsg) },
            error_app_tag: unsafe { cstr_opt((*must).eapptag) },
            description: unsafe { cstr_opt((*must).dsc) },
            reference: unsafe { cstr_opt((*must).ref_) },
        });
    }
    out
}

unsafe fn extract_whens(whens_ptr: *mut *mut lysc_when) -> Vec<RawWhenConstraint> {
    let count = unsafe { sized_array_count(whens_ptr) };
    let mut out = Vec::with_capacity(count);
    for i in 0..count {
        let when = unsafe { *whens_ptr.add(i) };
        if when.is_null() {
            continue;
        }
        out.push(RawWhenConstraint {
            expression: unsafe { cstr_opt(lyxp_get_expr((*when).cond)).unwrap_or_default() },
            description: unsafe { cstr_opt((*when).dsc) },
            reference: unsafe { cstr_opt((*when).ref_) },
        });
    }
    out
}

unsafe fn extract_extensions(exts_ptr: *mut lysc_ext_instance) -> Vec<RawExtension> {
    let count = unsafe { sized_array_count(exts_ptr) };
    let mut out = Vec::with_capacity(count);
    for i in 0..count {
        let ext = unsafe { exts_ptr.add(i) };
        let def = unsafe { (*ext).def };
        if def.is_null() {
            continue;
        }
        let module = unsafe {
            if !(*ext).module.is_null() {
                (*ext).module
            } else {
                (*def).module
            }
        };
        out.push(RawExtension {
            name: unsafe { cstr_opt((*def).name).unwrap_or_default() },
            argument: unsafe { cstr_opt((*ext).argument) },
            module_name: unsafe {
                if module.is_null() {
                    String::new()
                } else {
                    cstr_opt((*module).name).unwrap_or_default()
                }
            },
        });
    }
    out
}

unsafe fn grouping_origin(node: *const lysc_node) -> Option<String> {
    if node.is_null() {
        return None;
    }

    let mut parsed = unsafe { (*node).priv_ as *const lysp_node };
    while !parsed.is_null() {
        if unsafe { (*parsed).nodetype } as u32 == LYS_GROUPING {
            return unsafe { cstr_opt((*parsed).name) };
        }
        parsed = unsafe { (*parsed).parent };
    }
    None
}

unsafe fn list_unique_constraints(node: *const lysc_node) -> Vec<RawUniqueConstraint> {
    if node.is_null() || unsafe { (*node).nodetype } as u32 != LYS_LIST {
        return Vec::new();
    }

    let uniques = unsafe { (*(node as *const lysc_node_list)).uniques };
    let count = unsafe { sized_array_count(uniques) };
    let mut out = Vec::with_capacity(count);
    for i in 0..count {
        let unique = unsafe { *uniques.add(i) };
        let leaf_count = unsafe { sized_array_count(unique) };
        let mut leaf_schemas = Vec::with_capacity(leaf_count);
        for j in 0..leaf_count {
            let leaf = unsafe { *unique.add(j) };
            if !leaf.is_null() {
                leaf_schemas.push((leaf as *const lysc_node_leaf).cast());
            }
        }
        out.push(RawUniqueConstraint { leaf_schemas });
    }
    out
}

fn flag_set(flags: u16, flag: u32) -> bool {
    (flags as u32 & flag) != 0
}

fn deviate_type_name(deviate_type: u8) -> Option<&'static str> {
    match deviate_type as u32 {
        LYS_DEV_NOT_SUPPORTED => Some("not-supported"),
        LYS_DEV_ADD => Some("add"),
        LYS_DEV_DELETE => Some("delete"),
        LYS_DEV_REPLACE => Some("replace"),
        _ => None,
    }
}

unsafe fn qname_value(qname: *const lysp_qname) -> Option<String> {
    if qname.is_null() {
        None
    } else {
        unsafe { cstr_opt((*qname).str_) }
    }
}

fn push_deviation_property(
    out: &mut Vec<RawDeviation>,
    target_path: &str,
    source_module: &str,
    deviation_type: &str,
    property: &str,
    new_value: String,
    description: &Option<String>,
    reference: &Option<String>,
) {
    out.push(RawDeviation {
        target_path: target_path.to_string(),
        source_module: source_module.to_string(),
        deviation_type: deviation_type.to_string(),
        property: property.to_string(),
        new_value,
        description: description.clone(),
        reference: reference.clone(),
    });
}

unsafe fn push_qname_array_deviations(
    out: &mut Vec<RawDeviation>,
    qnames: *mut lysp_qname,
    target_path: &str,
    source_module: &str,
    deviation_type: &str,
    property: &str,
    description: &Option<String>,
    reference: &Option<String>,
) {
    let count = unsafe { sized_array_count(qnames) };
    for i in 0..count {
        if let Some(value) = unsafe { qname_value(qnames.add(i)) } {
            push_deviation_property(
                out,
                target_path,
                source_module,
                deviation_type,
                property,
                value,
                description,
                reference,
            );
        }
    }
}

unsafe fn push_restr_array_deviations(
    out: &mut Vec<RawDeviation>,
    restrictions: *mut lysp_restr,
    target_path: &str,
    source_module: &str,
    deviation_type: &str,
    property: &str,
    description: &Option<String>,
    reference: &Option<String>,
) {
    let count = unsafe { sized_array_count(restrictions) };
    for i in 0..count {
        let restriction = unsafe { restrictions.add(i) };
        if let Some(value) = unsafe { qname_value(&(*restriction).arg) } {
            push_deviation_property(
                out,
                target_path,
                source_module,
                deviation_type,
                property,
                value,
                description,
                reference,
            );
        }
    }
}

fn push_config_deviation(
    out: &mut Vec<RawDeviation>,
    flags: u16,
    target_path: &str,
    source_module: &str,
    deviation_type: &str,
    description: &Option<String>,
    reference: &Option<String>,
) {
    if !flag_set(flags, LYS_SET_CONFIG) {
        return;
    }
    let value = match flags as u32 & LYS_CONFIG_MASK {
        LYS_CONFIG_R => "false",
        LYS_CONFIG_W => "true",
        _ => "",
    };
    push_deviation_property(
        out,
        target_path,
        source_module,
        deviation_type,
        "config",
        value.to_string(),
        description,
        reference,
    );
}

fn push_mandatory_deviation(
    out: &mut Vec<RawDeviation>,
    flags: u16,
    target_path: &str,
    source_module: &str,
    deviation_type: &str,
    description: &Option<String>,
    reference: &Option<String>,
) {
    let value = match flags as u32 & LYS_MAND_MASK {
        LYS_MAND_TRUE => Some("true"),
        LYS_MAND_FALSE => Some("false"),
        _ => None,
    };
    if let Some(value) = value {
        push_deviation_property(
            out,
            target_path,
            source_module,
            deviation_type,
            "mandatory",
            value.to_string(),
            description,
            reference,
        );
    }
}

fn max_elements_value(max: u32) -> String {
    if max == 0 {
        "unbounded".to_string()
    } else {
        max.to_string()
    }
}

unsafe fn push_deviate_add_properties(
    out: &mut Vec<RawDeviation>,
    add: *const lysp_deviate_add,
    target_path: &str,
    source_module: &str,
    deviation_type: &str,
    description: &Option<String>,
    reference: &Option<String>,
) {
    if let Some(units) = unsafe { cstr_opt((*add).units) } {
        push_deviation_property(
            out,
            target_path,
            source_module,
            deviation_type,
            "units",
            units,
            description,
            reference,
        );
    }
    unsafe {
        push_restr_array_deviations(
            out,
            (*add).musts,
            target_path,
            source_module,
            deviation_type,
            "must",
            description,
            reference,
        );
        push_qname_array_deviations(
            out,
            (*add).uniques,
            target_path,
            source_module,
            deviation_type,
            "unique",
            description,
            reference,
        );
        push_qname_array_deviations(
            out,
            (*add).dflts,
            target_path,
            source_module,
            deviation_type,
            "default",
            description,
            reference,
        );
    }

    let flags = unsafe { (*add).flags };
    push_config_deviation(
        out,
        flags,
        target_path,
        source_module,
        deviation_type,
        description,
        reference,
    );
    push_mandatory_deviation(
        out,
        flags,
        target_path,
        source_module,
        deviation_type,
        description,
        reference,
    );
    if flag_set(flags, LYS_SET_MIN) && unsafe { (*add).dflts.is_null() } {
        push_deviation_property(
            out,
            target_path,
            source_module,
            deviation_type,
            "min-elements",
            unsafe { (*add).min }.to_string(),
            description,
            reference,
        );
    }
    if flag_set(flags, LYS_SET_MAX) && unsafe { (*add).units.is_null() } {
        push_deviation_property(
            out,
            target_path,
            source_module,
            deviation_type,
            "max-elements",
            max_elements_value(unsafe { (*add).max }),
            description,
            reference,
        );
    }
}

unsafe fn push_deviate_delete_properties(
    out: &mut Vec<RawDeviation>,
    del: *const lysp_deviate_del,
    target_path: &str,
    source_module: &str,
    deviation_type: &str,
    description: &Option<String>,
    reference: &Option<String>,
) {
    if let Some(units) = unsafe { cstr_opt((*del).units) } {
        push_deviation_property(
            out,
            target_path,
            source_module,
            deviation_type,
            "units",
            units,
            description,
            reference,
        );
    }
    unsafe {
        push_restr_array_deviations(
            out,
            (*del).musts,
            target_path,
            source_module,
            deviation_type,
            "must",
            description,
            reference,
        );
        push_qname_array_deviations(
            out,
            (*del).uniques,
            target_path,
            source_module,
            deviation_type,
            "unique",
            description,
            reference,
        );
        push_qname_array_deviations(
            out,
            (*del).dflts,
            target_path,
            source_module,
            deviation_type,
            "default",
            description,
            reference,
        );
    }
}

unsafe fn push_deviate_replace_properties(
    out: &mut Vec<RawDeviation>,
    replace: *const lysp_deviate_rpl,
    target_path: &str,
    source_module: &str,
    deviation_type: &str,
    description: &Option<String>,
    reference: &Option<String>,
) {
    let type_ptr = unsafe { (*replace).type_ };
    if !type_ptr.is_null() {
        if let Some(type_name) = unsafe { cstr_opt((*type_ptr).name) } {
            push_deviation_property(
                out,
                target_path,
                source_module,
                deviation_type,
                "type",
                type_name,
                description,
                reference,
            );
        }
    }
    if let Some(units) = unsafe { cstr_opt((*replace).units) } {
        push_deviation_property(
            out,
            target_path,
            source_module,
            deviation_type,
            "units",
            units,
            description,
            reference,
        );
    }

    let flags = unsafe { (*replace).flags };
    let default_set = !unsafe { (*replace).dflt.str_.is_null() };
    if default_set {
        push_deviation_property(
            out,
            target_path,
            source_module,
            deviation_type,
            "default",
            unsafe { qname_value(&(*replace).dflt) }.unwrap_or_default(),
            description,
            reference,
        );
    }
    push_config_deviation(
        out,
        flags,
        target_path,
        source_module,
        deviation_type,
        description,
        reference,
    );
    push_mandatory_deviation(
        out,
        flags,
        target_path,
        source_module,
        deviation_type,
        description,
        reference,
    );
    if flag_set(flags, LYS_SET_MIN) && !default_set {
        push_deviation_property(
            out,
            target_path,
            source_module,
            deviation_type,
            "min-elements",
            unsafe { (*replace).min }.to_string(),
            description,
            reference,
        );
    }
    if flag_set(flags, LYS_SET_MAX) && unsafe { (*replace).units.is_null() } {
        push_deviation_property(
            out,
            target_path,
            source_module,
            deviation_type,
            "max-elements",
            max_elements_value(unsafe { (*replace).max }),
            description,
            reference,
        );
    }
}

unsafe fn extract_deviations(
    deviations: *mut lysp_deviation,
    source_module: &str,
) -> Vec<RawDeviation> {
    let count = unsafe { sized_array_count(deviations) };
    let mut out = Vec::new();
    for i in 0..count {
        let deviation = unsafe { deviations.add(i) };
        let target_path = unsafe { cstr_opt((*deviation).nodeid).unwrap_or_default() };
        let description = unsafe { cstr_opt((*deviation).dsc) };
        let reference = unsafe { cstr_opt((*deviation).ref_) };
        let mut deviate = unsafe { (*deviation).deviates };
        while !deviate.is_null() {
            let Some(deviation_type) = (unsafe { deviate_type_name((*deviate).mod_) }) else {
                deviate = unsafe { (*deviate).next };
                continue;
            };
            match unsafe { (*deviate).mod_ as u32 } {
                LYS_DEV_NOT_SUPPORTED => push_deviation_property(
                    &mut out,
                    &target_path,
                    source_module,
                    deviation_type,
                    "",
                    String::new(),
                    &description,
                    &reference,
                ),
                LYS_DEV_ADD => unsafe {
                    push_deviate_add_properties(
                        &mut out,
                        deviate.cast::<lysp_deviate_add>(),
                        &target_path,
                        source_module,
                        deviation_type,
                        &description,
                        &reference,
                    );
                },
                LYS_DEV_DELETE => unsafe {
                    push_deviate_delete_properties(
                        &mut out,
                        deviate.cast::<lysp_deviate_del>(),
                        &target_path,
                        source_module,
                        deviation_type,
                        &description,
                        &reference,
                    );
                },
                LYS_DEV_REPLACE => unsafe {
                    push_deviate_replace_properties(
                        &mut out,
                        deviate.cast::<lysp_deviate_rpl>(),
                        &target_path,
                        source_module,
                        deviation_type,
                        &description,
                        &reference,
                    );
                },
                _ => {}
            }
            deviate = unsafe { (*deviate).next };
        }
    }
    out
}

unsafe fn extract_type_info(type_ptr: *const lysc_type) -> RawTypeInfo {
    if type_ptr.is_null() {
        return RawTypeInfo::unknown();
    }
    let base_type = unsafe { (*type_ptr).basetype };
    let base = match base_type {
        LY_DATA_TYPE::LY_TYPE_STRING => RawBaseType::String,
        LY_DATA_TYPE::LY_TYPE_BOOL => RawBaseType::Boolean,
        LY_DATA_TYPE::LY_TYPE_INT8 => RawBaseType::Int8,
        LY_DATA_TYPE::LY_TYPE_INT16 => RawBaseType::Int16,
        LY_DATA_TYPE::LY_TYPE_INT32 => RawBaseType::Int32,
        LY_DATA_TYPE::LY_TYPE_INT64 => RawBaseType::Int64,
        LY_DATA_TYPE::LY_TYPE_UINT8 => RawBaseType::Uint8,
        LY_DATA_TYPE::LY_TYPE_UINT16 => RawBaseType::Uint16,
        LY_DATA_TYPE::LY_TYPE_UINT32 => RawBaseType::Uint32,
        LY_DATA_TYPE::LY_TYPE_UINT64 => RawBaseType::Uint64,
        LY_DATA_TYPE::LY_TYPE_DEC64 => RawBaseType::Decimal64,
        LY_DATA_TYPE::LY_TYPE_EMPTY => RawBaseType::Empty,
        LY_DATA_TYPE::LY_TYPE_BINARY => RawBaseType::Binary,
        LY_DATA_TYPE::LY_TYPE_BITS => RawBaseType::Bits,
        LY_DATA_TYPE::LY_TYPE_ENUM => RawBaseType::Enumeration,
        LY_DATA_TYPE::LY_TYPE_IDENT => RawBaseType::IdentityRef,
        LY_DATA_TYPE::LY_TYPE_INST => RawBaseType::InstanceIdentifier,
        LY_DATA_TYPE::LY_TYPE_LEAFREF => RawBaseType::LeafRef,
        LY_DATA_TYPE::LY_TYPE_UNION => RawBaseType::Union,
        _ => RawBaseType::Unknown,
    };
    let mut info = RawTypeInfo::unknown();
    info.base_type = base;
    info.typedef_name = unsafe { leaf_typedef_name_from_type(type_ptr) };

    match base_type {
        LY_DATA_TYPE::LY_TYPE_INT8
        | LY_DATA_TYPE::LY_TYPE_INT16
        | LY_DATA_TYPE::LY_TYPE_INT32
        | LY_DATA_TYPE::LY_TYPE_INT64 => {
            let num = type_ptr as *const lysc_type_num;
            info.range = unsafe { extract_range((*num).range, RangeKind::Signed) };
        }
        LY_DATA_TYPE::LY_TYPE_UINT8
        | LY_DATA_TYPE::LY_TYPE_UINT16
        | LY_DATA_TYPE::LY_TYPE_UINT32
        | LY_DATA_TYPE::LY_TYPE_UINT64 => {
            let num = type_ptr as *const lysc_type_num;
            info.range = unsafe { extract_range((*num).range, RangeKind::Unsigned) };
        }
        LY_DATA_TYPE::LY_TYPE_DEC64 => {
            let dec = type_ptr as *const lysc_type_dec;
            let fraction_digits = unsafe { (*dec).fraction_digits };
            info.fraction_digits = Some(fraction_digits);
            info.range =
                unsafe { extract_range((*dec).range, RangeKind::Decimal64(fraction_digits)) };
        }
        LY_DATA_TYPE::LY_TYPE_STRING => {
            let s = type_ptr as *const lysc_type_str;
            info.length = unsafe { extract_range((*s).length, RangeKind::Unsigned) };
            info.patterns = unsafe { extract_patterns((*s).patterns) };
        }
        LY_DATA_TYPE::LY_TYPE_BINARY => {
            let b = type_ptr as *const lysc_type_bin;
            info.length = unsafe { extract_range((*b).length, RangeKind::Unsigned) };
        }
        LY_DATA_TYPE::LY_TYPE_ENUM => {
            let enm = type_ptr as *const lysc_type_enum;
            let count = unsafe { sized_array_count((*enm).enums) };
            let mut items = Vec::with_capacity(count);
            for i in 0..count {
                let cur = unsafe { (*enm).enums.add(i) };
                if cur.is_null() {
                    continue;
                }
                let name = unsafe { cstr_opt((*cur).name).unwrap_or_default() };
                let value = unsafe { (*cur).__bindgen_anon_1.value } as i64;
                items.push(RawEnumValue { name, value });
            }
            info.enum_values = items;
        }
        LY_DATA_TYPE::LY_TYPE_BITS => {
            let bits = type_ptr as *const lysc_type_bits;
            let count = unsafe { sized_array_count((*bits).bits) };
            let mut items = Vec::with_capacity(count);
            for i in 0..count {
                let cur = unsafe { (*bits).bits.add(i) };
                if cur.is_null() {
                    continue;
                }
                let name = unsafe { cstr_opt((*cur).name).unwrap_or_default() };
                let value = unsafe { (*cur).__bindgen_anon_1.position } as i64;
                items.push(RawEnumValue { name, value });
            }
            info.bit_values = items;
        }
        LY_DATA_TYPE::LY_TYPE_IDENT => {
            let idref = type_ptr as *const lysc_type_identityref;
            let count = unsafe { sized_array_count((*idref).bases) };
            let mut bases = Vec::with_capacity(count);
            for i in 0..count {
                let base_ptr = unsafe { *(*idref).bases.add(i) };
                if base_ptr.is_null() {
                    continue;
                }
                bases.push(unsafe { format_identity_name(base_ptr) });
            }
            info.identity_bases = bases;
        }
        LY_DATA_TYPE::LY_TYPE_INST => {
            let inst = type_ptr as *const lysc_type_instanceid;
            info.require_instance = Some(unsafe { (*inst).require_instance != 0 });
        }
        LY_DATA_TYPE::LY_TYPE_LEAFREF => {
            let lr = type_ptr as *const lysc_type_leafref;
            let path = unsafe { (*lr).path };
            if !path.is_null() {
                info.leafref_path = unsafe { cstr_opt(lyxp_get_expr(path)) };
            }
            info.require_instance = Some(unsafe { (*lr).require_instance != 0 });
            let realtype = unsafe { (*lr).realtype };
            if !realtype.is_null() {
                info.leafref_realtype = Some(Box::new(unsafe { extract_type_info(realtype) }));
            }
        }
        LY_DATA_TYPE::LY_TYPE_UNION => {
            let uni = type_ptr as *const lysc_type_union;
            let count = unsafe { sized_array_count((*uni).types) };
            let mut members = Vec::with_capacity(count);
            for i in 0..count {
                let member = unsafe { *(*uni).types.add(i) };
                if member.is_null() {
                    continue;
                }
                members.push(unsafe { extract_type_info(member) });
            }
            info.union_types = members;
        }
        _ => {}
    }

    info
}

unsafe fn leaf_typedef_name_from_type(type_ptr: *const lysc_type) -> Option<String> {
    if type_ptr.is_null() {
        return None;
    }
    let name = unsafe { cstr_opt((*type_ptr).name) };
    let basetype = unsafe { (*type_ptr).basetype };
    let builtin_name = match basetype {
        LY_DATA_TYPE::LY_TYPE_STRING => Some("string"),
        LY_DATA_TYPE::LY_TYPE_BOOL => Some("boolean"),
        LY_DATA_TYPE::LY_TYPE_INT8 => Some("int8"),
        LY_DATA_TYPE::LY_TYPE_INT16 => Some("int16"),
        LY_DATA_TYPE::LY_TYPE_INT32 => Some("int32"),
        LY_DATA_TYPE::LY_TYPE_INT64 => Some("int64"),
        LY_DATA_TYPE::LY_TYPE_UINT8 => Some("uint8"),
        LY_DATA_TYPE::LY_TYPE_UINT16 => Some("uint16"),
        LY_DATA_TYPE::LY_TYPE_UINT32 => Some("uint32"),
        LY_DATA_TYPE::LY_TYPE_UINT64 => Some("uint64"),
        LY_DATA_TYPE::LY_TYPE_DEC64 => Some("decimal64"),
        LY_DATA_TYPE::LY_TYPE_EMPTY => Some("empty"),
        LY_DATA_TYPE::LY_TYPE_BINARY => Some("binary"),
        LY_DATA_TYPE::LY_TYPE_BITS => Some("bits"),
        LY_DATA_TYPE::LY_TYPE_ENUM => Some("enumeration"),
        LY_DATA_TYPE::LY_TYPE_IDENT => Some("identityref"),
        LY_DATA_TYPE::LY_TYPE_INST => Some("instance-identifier"),
        LY_DATA_TYPE::LY_TYPE_LEAFREF => Some("leafref"),
        LY_DATA_TYPE::LY_TYPE_UNION => Some("union"),
        _ => None,
    };
    if name.as_deref() == builtin_name {
        None
    } else {
        name
    }
}

unsafe fn leaf_type_info(node: *const lysc_node) -> RawTypeInfo {
    if node.is_null() {
        return RawTypeInfo::unknown();
    }
    let nt = unsafe { (*node).nodetype } as u32;
    let type_ptr = if nt == LYS_LEAF {
        unsafe { (*(node as *const lysc_node_leaf)).type_ }
    } else if nt == LYS_LEAFLIST {
        unsafe { (*(node as *const lysc_node_leaflist)).type_ }
    } else {
        std::ptr::null_mut()
    };
    let mut info = unsafe { extract_type_info(type_ptr) };
    if matches!(info.base_type, RawBaseType::LeafRef) {
        let target = unsafe { lysc_node_lref_target(node) };
        if !target.is_null() {
            info.leafref_target_schema = target.cast();
        }
    }
    info
}

unsafe fn walk_siblings(first: *const lysc_node) -> Vec<RawSchemaNode> {
    let mut out = Vec::new();
    let mut cur = first;
    while !cur.is_null() {
        let children = unsafe { walk_node_children(cur) };
        let flags = unsafe { (*cur).flags } as u32;
        let nodetype = unsafe { (*cur).nodetype } as u32;
        let key_names: Vec<String> = if nodetype == LYS_LIST {
            children
                .iter()
                .filter(|c| c.is_key)
                .map(|c| c.name.clone())
                .collect()
        } else {
            Vec::new()
        };

        let (min_elements, max_elements) = unsafe { list_min_max(cur) };
        let owner_module = unsafe { (*cur).module };

        out.push(RawSchemaNode {
            name: unsafe { node_name(cur) },
            kind: unsafe { node_kind(cur) }.to_string(),
            config: unsafe { node_config(flags) },
            status: unsafe { node_status(flags) },
            mandatory: (flags & LYS_MAND_TRUE) != 0,
            // LYS_PRESENCE (0x80) aliases LYS_ORDBY_SYSTEM (0x80), which libyang
            // sets on every system-ordered list/leaf-list. Gate to containers
            // (mirrors libyang's lysc_is_np_cont) so only true presence
            // containers report presence.
            presence: nodetype == LYS_CONTAINER && (flags & LYS_PRESENCE) != 0,
            description: unsafe { node_description(cur) },
            reference: unsafe { node_reference(cur) },
            extensions: unsafe { extract_extensions((*cur).exts) },
            grouping_origin: unsafe { grouping_origin(cur) },
            units: unsafe { leaf_units(cur) },
            default_values: unsafe { leaf_defaults(cur) },
            musts: unsafe { extract_musts(lysc_node_musts(cur)) },
            whens: unsafe { extract_whens(lysc_node_when(cur)) },
            unique_constraints: unsafe { list_unique_constraints(cur) },
            min_elements,
            max_elements,
            ordered_by_user: (flags & LYS_ORDBY_USER) != 0,
            is_key: (flags & LYS_KEY) != 0,
            key_names,
            key_indices: Vec::new(),
            base_type: unsafe { leaf_type_base(cur) },
            typedef_name: unsafe { leaf_typedef_name(cur) },
            type_info: unsafe { leaf_type_info(cur) },
            children,
            module_ns: String::new(),
            owner_module_name: unsafe { lys_module_name(owner_module) },
            owner_module_namespace: unsafe { lys_module_namespace(owner_module) },
            schema: cur.cast(),
        });
        cur = unsafe { (*cur).next as *const lysc_node };
    }
    out
}

unsafe fn walk_node_children(node: *const lysc_node) -> Vec<RawSchemaNode> {
    let child_first = unsafe { lysc_node_child(node) };
    let mut children = if child_first.is_null() {
        Vec::new()
    } else {
        unsafe { walk_siblings(child_first) }
    };

    let actions = unsafe { lysc_node_actions(node) };
    if !actions.is_null() {
        children.extend(unsafe { walk_siblings(actions.cast::<lysc_node>()) });
    }
    let notifs = unsafe { lysc_node_notifs(node) };
    if !notifs.is_null() {
        children.extend(unsafe { walk_siblings(notifs.cast::<lysc_node>()) });
    }

    children
}

unsafe fn walk_compiled_module_children(compiled: *const lysc_module) -> Vec<RawSchemaNode> {
    if compiled.is_null() {
        return Vec::new();
    }

    let mut children = Vec::new();
    let data = unsafe { (*compiled).data };
    if !data.is_null() {
        children.extend(unsafe { walk_siblings(data) });
    }
    let rpcs = unsafe { (*compiled).rpcs };
    if !rpcs.is_null() {
        children.extend(unsafe { walk_siblings(rpcs.cast::<lysc_node>()) });
    }
    let notifs = unsafe { (*compiled).notifs };
    if !notifs.is_null() {
        children.extend(unsafe { walk_siblings(notifs.cast::<lysc_node>()) });
    }
    children
}

#[derive(Debug, Clone, PartialEq, Eq)]
enum YangToken {
    Word(String),
    LBrace,
    RBrace,
    Semi,
}

#[derive(Debug, Clone)]
struct YangStmt {
    keyword: String,
    arg: Option<String>,
    children: Vec<YangStmt>,
}

#[derive(Debug, Clone, PartialEq, Eq, Hash)]
struct SourceChildKey {
    kind: String,
    name: String,
}

type SourceOrderMap = HashMap<Vec<String>, Vec<SourceChildKey>>;

fn tokenize_yang(source: &str) -> Vec<YangToken> {
    let mut tokens = Vec::new();
    let mut chars = source.chars().peekable();
    while let Some(ch) = chars.next() {
        match ch {
            c if c.is_whitespace() => {}
            '/' if chars.peek() == Some(&'/') => {
                chars.next();
                for c in chars.by_ref() {
                    if c == '\n' {
                        break;
                    }
                }
            }
            '/' if chars.peek() == Some(&'*') => {
                chars.next();
                let mut prev = '\0';
                for c in chars.by_ref() {
                    if prev == '*' && c == '/' {
                        break;
                    }
                    prev = c;
                }
            }
            '{' => tokens.push(YangToken::LBrace),
            '}' => tokens.push(YangToken::RBrace),
            ';' => tokens.push(YangToken::Semi),
            '"' | '\'' => {
                let quote = ch;
                let mut value = String::new();
                while let Some(c) = chars.next() {
                    if c == quote {
                        break;
                    }
                    if quote == '"' && c == '\\' {
                        if let Some(next) = chars.next() {
                            value.push(next);
                        }
                    } else {
                        value.push(c);
                    }
                }
                tokens.push(YangToken::Word(value));
            }
            _ => {
                let mut word = String::from(ch);
                while let Some(&c) = chars.peek() {
                    if c.is_whitespace() || matches!(c, '{' | '}' | ';' | '"' | '\'') {
                        break;
                    }
                    if c == '/' {
                        let mut lookahead = chars.clone();
                        lookahead.next();
                        if matches!(lookahead.peek(), Some('/') | Some('*')) {
                            break;
                        }
                    }
                    word.push(c);
                    chars.next();
                }
                tokens.push(YangToken::Word(word));
            }
        }
    }
    tokens
}

fn parse_yang_statements(tokens: &[YangToken], pos: &mut usize) -> Vec<YangStmt> {
    let mut out = Vec::new();
    while *pos < tokens.len() {
        let keyword = match &tokens[*pos] {
            YangToken::Word(word) => word.clone(),
            YangToken::RBrace => {
                *pos += 1;
                break;
            }
            _ => {
                *pos += 1;
                continue;
            }
        };
        *pos += 1;

        let mut arg = None;
        loop {
            match tokens.get(*pos) {
                Some(YangToken::Word(word)) => {
                    if arg.is_none() && word != "+" {
                        arg = Some(word.clone());
                    }
                    *pos += 1;
                }
                Some(YangToken::LBrace) => {
                    *pos += 1;
                    let children = parse_yang_statements(tokens, pos);
                    out.push(YangStmt {
                        keyword,
                        arg,
                        children,
                    });
                    break;
                }
                Some(YangToken::Semi) => {
                    *pos += 1;
                    out.push(YangStmt {
                        keyword,
                        arg,
                        children: Vec::new(),
                    });
                    break;
                }
                Some(YangToken::RBrace) => {
                    out.push(YangStmt {
                        keyword,
                        arg,
                        children: Vec::new(),
                    });
                    break;
                }
                None => {
                    out.push(YangStmt {
                        keyword,
                        arg,
                        children: Vec::new(),
                    });
                    break;
                }
            }
        }
    }
    out
}

fn schema_child_key(stmt: &YangStmt) -> Option<SourceChildKey> {
    let keyword = stmt.keyword.rsplit(':').next().unwrap_or(&stmt.keyword);
    let (kind, name) = match keyword {
        "container" | "leaf" | "leaf-list" | "list" | "choice" | "case" | "action" | "rpc"
        | "notification" => (keyword, stmt.arg.as_deref()?),
        "anydata" | "anyxml" => ("anydata", stmt.arg.as_deref()?),
        "input" => ("input", "input"),
        "output" => ("output", "output"),
        _ => return None,
    };
    Some(SourceChildKey {
        kind: kind.to_string(),
        name: name.to_string(),
    })
}

fn collect_source_order(stmts: &[YangStmt], path: &mut Vec<String>, out: &mut SourceOrderMap) {
    let children: Vec<_> = stmts.iter().filter_map(schema_child_key).collect();
    if !children.is_empty() {
        out.insert(path.clone(), children);
    }

    for stmt in stmts {
        if let Some(child) = schema_child_key(stmt) {
            path.push(child.name);
            collect_source_order(&stmt.children, path, out);
            path.pop();
        } else if matches!(
            stmt.keyword.rsplit(':').next().unwrap_or(&stmt.keyword),
            "module" | "submodule"
        ) {
            collect_source_order(&stmt.children, path, out);
        }
    }
}

unsafe fn module_source_order(mod_ptr: *const lys_module) -> Option<SourceOrderMap> {
    let filepath = unsafe { cstr_opt((*mod_ptr).filepath)? };
    let source = fs::read_to_string(filepath).ok()?;
    let tokens = tokenize_yang(&source);
    let mut pos = 0;
    let stmts = parse_yang_statements(&tokens, &mut pos);
    let module_name = unsafe { cstr_opt((*mod_ptr).name).unwrap_or_default() };

    let mut out = SourceOrderMap::new();
    let mut path = Vec::new();
    for stmt in &stmts {
        let keyword = stmt.keyword.rsplit(':').next().unwrap_or(&stmt.keyword);
        if matches!(keyword, "module" | "submodule") && stmt.arg.as_deref() == Some(&module_name) {
            collect_source_order(&stmt.children, &mut path, &mut out);
        }
    }
    Some(out)
}

fn child_order_key(child: &RawSchemaNode) -> SourceChildKey {
    SourceChildKey {
        kind: child.kind.clone(),
        name: child.name.clone(),
    }
}

fn inferred_unmatched_key(index: usize, matched: &[Option<usize>]) -> i64 {
    let prev = matched[..index].iter().rev().flatten().next().copied();
    let next = matched[index + 1..].iter().flatten().next().copied();
    match (prev, next) {
        (Some(p), Some(n)) if p < n => (p as i64 * 2) + 1,
        (Some(p), _) => (p as i64 * 2) + 1,
        (_, Some(n)) => (n as i64 * 2) - 1,
        _ => i64::MAX / 2,
    }
}

fn reorder_children_from_source(children: &mut Vec<RawSchemaNode>, source: &[SourceChildKey]) {
    let source_index: HashMap<_, _> = source
        .iter()
        .enumerate()
        .map(|(idx, child)| (child.clone(), idx))
        .collect();
    let matched: Vec<Option<usize>> = children
        .iter()
        .map(|child| source_index.get(&child_order_key(child)).copied())
        .collect();
    if matched.iter().all(Option::is_none) {
        return;
    }

    let mut indexed: Vec<_> = std::mem::take(children).into_iter().enumerate().collect();
    indexed.sort_by_key(|(idx, _)| {
        let primary = matched[*idx]
            .map(|source_idx| source_idx as i64 * 2)
            .unwrap_or_else(|| inferred_unmatched_key(*idx, &matched));
        (primary, *idx)
    });
    children.extend(indexed.into_iter().map(|(_, child)| child));
}

fn apply_source_order(
    children: &mut Vec<RawSchemaNode>,
    path: &mut Vec<String>,
    map: &SourceOrderMap,
) {
    if let Some(source) = map.get(path) {
        reorder_children_from_source(children, source);
    }

    for child in children {
        path.push(child.name.clone());
        apply_source_order(&mut child.children, path, map);
        path.pop();
    }
}

/// Opaque owned libyang context.
#[derive(Debug)]
pub struct RawContext {
    ptr: *mut ly_ctx,
}

/// Opaque owned libyang data tree.
#[derive(Debug)]
pub struct RawDataTree {
    ptr: *mut lyd_node,
    ctx: *const ly_ctx,
}

/// One materialized child/sibling/root node returned by a single coarse walk.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct RawChildInfo {
    /// Absolute data path (e.g. `/module:container/leaf`).
    pub path: String,
    /// Schema-local node name.
    pub name: String,
    /// True if the node was created from a default value.
    pub is_default: bool,
}

/// One raw validation diagnostic copied from the context error list.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct RawDiagnostic {
    /// Human-readable message.
    pub message: String,
    /// Data instance path, if available.
    pub data_path: Option<String>,
    /// Schema path, if available.
    pub schema_path: Option<String>,
    /// Error-app-tag, if available.
    pub apptag: Option<String>,
    /// Human-readable description of the libyang validation code.
    pub vecode_str: Option<String>,
}

/// Structured validation failure extracted from `ly_err_item`.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct RawValidationError(pub Vec<RawDiagnostic>);

/// Module-level metadata extracted in one coarse walk.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct RawModuleInfo {
    /// Module name.
    pub name: String,
    /// XML namespace.
    pub namespace: String,
    /// Module prefix.
    pub prefix: String,
    /// Revision string (YYYY-MM-DD), if present.
    pub revision: Option<String>,
    /// Whether the module is implemented (not just imported).
    pub is_implemented: bool,
    /// Names of loaded modules that augment this module.
    pub augmented_by: Vec<String>,
    /// Names of loaded modules that deviate this module.
    pub deviated_by: Vec<String>,
    /// Deviations defined by this module in source declaration order.
    pub deviations: Vec<RawDeviation>,
    /// Imports in source declaration order.
    pub imports: Vec<RawImport>,
    /// Identities declared in this module.
    pub identities: Vec<RawIdentity>,
}

/// One parsed deviation property.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct RawDeviation {
    /// Target schema nodeid as written in the deviation statement.
    pub target_path: String,
    /// Module defining the deviation.
    pub source_module: String,
    /// Deviate operation: not-supported, add, delete, or replace.
    pub deviation_type: String,
    /// Affected property, or empty for not-supported.
    pub property: String,
    /// New or removed value, if represented by the parsed deviation.
    pub new_value: String,
    /// Deviation description.
    pub description: Option<String>,
    /// Deviation reference.
    pub reference: Option<String>,
}

/// Module import metadata extracted from the parsed module.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct RawImport {
    /// Import prefix.
    pub prefix: String,
    /// Imported module name.
    pub name: String,
    /// Requested revision-date, if present.
    pub revision: Option<String>,
}

/// Config flag for a schema node.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum RawConfig {
    /// Read-write (`config true`).
    Rw,
    /// Read-only (`config false`).
    Ro,
    /// No explicit config statement (inherits from parent).
    Unset,
}

/// Status flag for a schema node.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum RawStatus {
    /// `status current`.
    Current,
    /// `status deprecated`.
    Deprecated,
    /// `status obsolete`.
    Obsolete,
}

/// Precise built-in YANG base type.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
#[non_exhaustive]
pub enum RawBaseType {
    /// `int8`.
    Int8,
    /// `int16`.
    Int16,
    /// `int32`.
    Int32,
    /// `int64`.
    Int64,
    /// `uint8`.
    Uint8,
    /// `uint16`.
    Uint16,
    /// `uint32`.
    Uint32,
    /// `uint64`.
    Uint64,
    /// `decimal64`.
    Decimal64,
    /// `string`.
    String,
    /// `boolean`.
    Boolean,
    /// `empty`.
    Empty,
    /// `binary`.
    Binary,
    /// `bits`.
    Bits,
    /// `enumeration`.
    Enumeration,
    /// `identityref`.
    IdentityRef,
    /// `instance-identifier`.
    InstanceIdentifier,
    /// `leafref`.
    LeafRef,
    /// `union`.
    Union,
    /// Unmapped or absent.
    Unknown,
}

/// One enum or bit value in declaration order.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct RawEnumValue {
    /// Enum/bit name.
    pub name: String,
    /// Integer value (enum value or bit position).
    pub value: i64,
}

/// A textual pattern constraint.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct RawPattern {
    /// POSIX regular expression.
    pub regex: String,
    /// Error app-tag, if any.
    pub error_app_tag: Option<String>,
    /// Invert-match flag.
    pub inverted: bool,
}

/// One compiled `must` constraint.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct RawMustConstraint {
    /// XPath expression.
    pub expression: String,
    /// Optional error-message.
    pub error_message: Option<String>,
    /// Optional error-app-tag.
    pub error_app_tag: Option<String>,
    /// Optional description.
    pub description: Option<String>,
    /// Optional reference.
    pub reference: Option<String>,
}

/// One compiled `when` constraint.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct RawWhenConstraint {
    /// XPath expression.
    pub expression: String,
    /// Optional description.
    pub description: Option<String>,
    /// Optional reference.
    pub reference: Option<String>,
}

/// One compiled extension instance.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct RawExtension {
    /// Extension definition name.
    pub name: String,
    /// Optional argument string.
    pub argument: Option<String>,
    /// Defining module name.
    pub module_name: String,
}

/// One compiled list `unique` constraint.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct RawUniqueConstraint {
    /// Participating leaf schema node pointers in unique-statement order.
    pub leaf_schemas: Vec<*const ::std::os::raw::c_void>,
}

/// One bound of a `range` or `length` constraint.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct RawRangeBound {
    /// Canonical minimum string.
    pub min: String,
    /// Canonical maximum string.
    pub max: String,
}

/// Rich type constraints for a leaf or leaf-list type.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct RawTypeInfo {
    /// Built-in base type.
    pub base_type: RawBaseType,
    /// Typedef name, if not a built-in.
    pub typedef_name: Option<String>,
    /// `fraction-digits` for decimal64.
    pub fraction_digits: Option<u8>,
    /// `range` constraint parts for numeric types.
    pub range: Vec<RawRangeBound>,
    /// `length` constraint parts for string/binary types.
    pub length: Vec<RawRangeBound>,
    /// Pattern constraints for strings.
    pub patterns: Vec<RawPattern>,
    /// Enumeration values in declaration order.
    pub enum_values: Vec<RawEnumValue>,
    /// Bit values in declaration order.
    pub bit_values: Vec<RawEnumValue>,
    /// Identity base names for identityref (module:name or name).
    pub identity_bases: Vec<String>,
    /// `require-instance` for leafref/instance-identifier.
    pub require_instance: Option<bool>,
    /// Raw leafref path expression, if available.
    pub leafref_path: Option<String>,
    /// Resolved target schema pointer for leafref, if available.
    pub leafref_target_schema: *const ::std::os::raw::c_void,
    /// Resolved target type for leafref, if available.
    pub leafref_realtype: Option<Box<RawTypeInfo>>,
    /// Union member types, recursively extracted.
    pub union_types: Vec<RawTypeInfo>,
}

impl RawTypeInfo {
    /// A placeholder with no constraints.
    pub fn unknown() -> Self {
        Self {
            base_type: RawBaseType::Unknown,
            typedef_name: None,
            fraction_digits: None,
            range: Vec::new(),
            length: Vec::new(),
            patterns: Vec::new(),
            enum_values: Vec::new(),
            bit_values: Vec::new(),
            identity_bases: Vec::new(),
            require_instance: None,
            leafref_path: None,
            leafref_target_schema: std::ptr::null(),
            leafref_realtype: None,
            union_types: Vec::new(),
        }
    }
}

/// Identity metadata extracted from a compiled module.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct RawIdentity {
    /// Identity name.
    pub name: String,
    /// Owning module name.
    pub module_name: String,
    /// Direct base identity names (module:name or name if same module).
    pub bases: Vec<String>,
    /// Directly derived identity names (module:name or name).
    pub derived: Vec<String>,
}

/// Description of one schema node, returned by a single coarse schema walk.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct RawSchemaNode {
    /// Node name.
    pub name: String,
    /// Libyang node type as a stable string (container, leaf, list, leaflist,
    /// choice, case, anydata, rpc, action, input, output, notification, unknown).
    pub kind: String,
    /// Read-write vs read-only flag.
    pub config: RawConfig,
    /// Status flag.
    pub status: RawStatus,
    /// True for `mandatory true`.
    pub mandatory: bool,
    /// True for presence containers.
    pub presence: bool,
    /// `description` substatement.
    pub description: Option<String>,
    /// `reference` substatement.
    pub reference: Option<String>,
    /// Extension instances in declaration order.
    pub extensions: Vec<RawExtension>,
    /// Grouping name if this node was instantiated from a `uses`.
    pub grouping_origin: Option<String>,
    /// `units` substatement.
    pub units: Option<String>,
    /// `default` values as canonical strings in declaration order.
    pub default_values: Vec<String>,
    /// Compiled `must` constraints in declaration order.
    pub musts: Vec<RawMustConstraint>,
    /// Compiled `when` constraints in declaration order.
    pub whens: Vec<RawWhenConstraint>,
    /// Compiled list `unique` constraints in declaration order.
    pub unique_constraints: Vec<RawUniqueConstraint>,
    /// `min-elements` for lists/leaf-lists.
    pub min_elements: Option<u32>,
    /// `max-elements` for lists/leaf-lists.
    pub max_elements: Option<u32>,
    /// True for `ordered-by user` lists and leaf-lists.
    pub ordered_by_user: bool,
    /// True for list key leaves.
    pub is_key: bool,
    /// For lists, the names of key leaves in key-statement order.
    pub key_names: Vec<String>,
    /// For lists, indices into `children` of key leaves in key-statement order.
    pub key_indices: Vec<usize>,
    /// For leaf/leaf-list nodes, the precise built-in base type.
    pub base_type: RawBaseType,
    /// For leaf/leaf-list nodes, the typedef name if not a built-in.
    pub typedef_name: Option<String>,
    /// For leaf/leaf-list nodes, rich type constraints.
    pub type_info: RawTypeInfo,
    /// Child nodes in schema declaration order.
    pub children: Vec<RawSchemaNode>,
    /// Module namespace (set only on the synthetic module root).
    pub module_ns: String,
    /// Name of the module that owns this schema node.
    pub owner_module_name: String,
    /// XML namespace of the module that owns this schema node.
    pub owner_module_namespace: String,
    /// Raw compiled schema node pointer (NULL for the synthetic module root).
    pub schema: *const ::std::os::raw::c_void,
}

impl RawContext {
    /// Create a new context with no search path.
    pub fn new(flags: u32) -> Result<Self, String> {
        let mut ptr: *mut ly_ctx = std::ptr::null_mut();
        let rc = unsafe { ly_ctx_new(std::ptr::null(), flags, &mut ptr) };
        if rc != LY_ERR::LY_SUCCESS {
            return Err(format!("ly_ctx_new failed: {:?}", rc));
        }
        if ptr.is_null() {
            return Err("ly_ctx_new returned null".to_string());
        }
        Ok(Self { ptr })
    }

    /// Append a directory to the module search path.
    pub fn set_search_path<P: AsRef<Path>>(&mut self, path: P) -> Result<(), String> {
        let path = path_as_cstring(path)?;
        let rc = unsafe { ly_ctx_set_searchdir(self.ptr, path.as_ptr()) };
        if rc != LY_ERR::LY_SUCCESS {
            return Err(format!("ly_ctx_set_searchdir failed: {:?}", rc));
        }
        Ok(())
    }

    /// Load a YANG module into the context.
    pub fn load_module(
        &mut self,
        name: &str,
        revision: Option<&str>,
        features: &[&str],
    ) -> Result<(), String> {
        let name = CString::new(name).map_err(|e| e.to_string())?;
        let rev_cstr = revision
            .map(CString::new)
            .transpose()
            .map_err(|e| e.to_string())?;
        let mut features_ptrs: Vec<*const ::std::os::raw::c_char> = features
            .iter()
            .map(|f| {
                CString::new(*f)
                    .map(|c| c.into_raw() as *const ::std::os::raw::c_char)
                    .unwrap_or(std::ptr::null())
            })
            .collect();
        features_ptrs.push(std::ptr::null());
        let features_arg = if features.is_empty() {
            std::ptr::null_mut()
        } else {
            features_ptrs.as_mut_ptr()
        };
        let module = unsafe {
            ly_ctx_load_module(
                self.ptr,
                name.as_ptr(),
                rev_cstr
                    .as_ref()
                    .map(|c| c.as_ptr())
                    .unwrap_or(std::ptr::null()),
                features_arg,
            )
        };
        // Reclaim the leaked CString pointers so the strings are freed.
        for ptr in features_ptrs.iter().take(features.len()) {
            if !ptr.is_null() {
                unsafe {
                    let _ = CString::from_raw(*ptr as *mut ::std::os::raw::c_char);
                }
            }
        }
        if module.is_null() {
            return Err(format!("failed to load module {}", name.to_string_lossy()));
        }
        Ok(())
    }

    /// Load a YANG module from a filesystem path.
    pub fn load_module_path<P: AsRef<Path>>(&mut self, path: P) -> Result<(), String> {
        let path = path_as_cstring(path)?;
        let mut module: *mut lys_module = std::ptr::null_mut();
        let rc = unsafe {
            lys_parse_path(
                self.ptr,
                path.as_ptr(),
                LYS_INFORMAT::LYS_IN_YANG,
                &mut module,
            )
        };
        if rc != LY_ERR::LY_SUCCESS {
            return Err(format!("lys_parse_path failed: {:?}", rc));
        }
        if module.is_null() {
            return Err("lys_parse_path returned null".to_string());
        }
        Ok(())
    }

    /// Load a YANG module from an in-memory source string.
    pub fn load_module_str(&mut self, source: &str) -> Result<(), String> {
        let source = CString::new(source).map_err(|e| e.to_string())?;
        let mut module: *mut lys_module = std::ptr::null_mut();
        let rc = unsafe {
            lys_parse_mem(
                self.ptr,
                source.as_ptr(),
                LYS_INFORMAT::LYS_IN_YANG,
                &mut module,
            )
        };
        if rc != LY_ERR::LY_SUCCESS {
            return Err(format!("lys_parse_mem failed: {:?}", rc));
        }
        if module.is_null() {
            return Err("lys_parse_mem returned null".to_string());
        }
        Ok(())
    }

    /// Parse a whole data document.
    pub fn parse_data(
        &self,
        format: RawFormat,
        parse_options: u32,
        data: &[u8],
    ) -> Result<RawDataTree, String> {
        // libyang expects a NUL-terminated buffer, but LYB is binary and may
        // contain interior NULs. Append a sentinel NUL ourselves and pass the
        // slice pointer directly; text parsers still see termination, while LYB's
        // self-describing length keeps reads within the actual data.
        let mut buf = data.to_vec();
        buf.push(0);
        let mut ptr: *mut lyd_node = std::ptr::null_mut();
        let rc = unsafe {
            lyd_parse_data_mem(
                self.ptr,
                buf.as_ptr() as *const ::std::os::raw::c_char,
                format.into(),
                parse_options,
                0,
                &mut ptr,
            )
        };
        if rc != LY_ERR::LY_SUCCESS {
            return Err(format!("lyd_parse_data_mem failed: {:?}", rc));
        }
        if ptr.is_null() {
            return Err("lyd_parse_data_mem returned null".to_string());
        }
        Ok(RawDataTree { ptr, ctx: self.ptr })
    }

    /// Parse an RPC, action, or notification.
    pub fn parse_op(
        &self,
        format: RawFormat,
        op_type: RawOpType,
        data: &[u8],
    ) -> Result<RawDataTree, String> {
        let data = CString::new(data).map_err(|e| e.to_string())?;

        let mut input: *mut ly_in = std::ptr::null_mut();
        let rc = unsafe { ly_in_new_memory(data.as_ptr(), &mut input) };
        if rc != LY_ERR::LY_SUCCESS {
            return Err(format!("ly_in_new_memory failed: {:?}", rc));
        }

        let mut tree: *mut lyd_node = std::ptr::null_mut();
        let mut op: *mut lyd_node = std::ptr::null_mut();
        let rc = unsafe {
            lyd_parse_op(
                self.ptr,
                std::ptr::null_mut(),
                input,
                format.into(),
                op_type.into(),
                0,
                &mut tree,
                &mut op,
            )
        };
        unsafe { ly_in_free(input, 0) };

        if rc != LY_ERR::LY_SUCCESS {
            return Err(format!("lyd_parse_op failed: {:?}", rc));
        }

        // Return the full parsed `tree` so the ancestor data-tree context of a
        // nested action/notification (parent containers and list keys that
        // identify the target instance) is preserved on serialize, per RFC 7950
        // §7.15.2 and matching the yanglint oracle. `op` is the operation node
        // ONLY (no ancestors); fall back to it when there is no wrapping tree
        // (e.g. a top-level RPC where `tree` may be null).
        let ptr = if !tree.is_null() {
            tree
        } else if !op.is_null() {
            op
        } else {
            return Err("lyd_parse_op returned no tree".to_string());
        };
        Ok(RawDataTree { ptr, ctx: self.ptr })
    }

    /// Look up a loaded module and return its metadata.
    fn module_ptr(&self, module: &str) -> Result<*mut lys_module, String> {
        let name = CString::new(module).map_err(|e| e.to_string())?;
        let mod_ptr = unsafe { ly_ctx_get_module_implemented(self.ptr, name.as_ptr()) };
        if mod_ptr.is_null() {
            return Err(format!("module not found: {module}"));
        }
        Ok(mod_ptr)
    }

    unsafe fn module_info(mod_ptr: *const lys_module) -> RawModuleInfo {
        let name = unsafe { cstr_opt((*mod_ptr).name).unwrap_or_default() };
        let namespace = unsafe { cstr_opt((*mod_ptr).ns).unwrap_or_default() };
        let prefix = unsafe { cstr_opt((*mod_ptr).prefix).unwrap_or_default() };
        let revision = unsafe { cstr_opt((*mod_ptr).revision) };
        let is_implemented = unsafe { (*mod_ptr).implemented != 0 };
        let augmented_by = unsafe { module_name_array((*mod_ptr).augmented_by) };
        let deviated_by = unsafe { module_name_array((*mod_ptr).deviated_by) };
        let deviations = unsafe { Self::module_deviations(mod_ptr) };
        let imports = unsafe { Self::module_imports(mod_ptr) };
        let identities = unsafe { Self::module_identities(mod_ptr) };
        RawModuleInfo {
            name,
            namespace,
            prefix,
            revision,
            is_implemented,
            augmented_by,
            deviated_by,
            deviations,
            imports,
            identities,
        }
    }

    unsafe fn module_deviations(mod_ptr: *const lys_module) -> Vec<RawDeviation> {
        let parsed = unsafe { (*mod_ptr).parsed };
        if parsed.is_null() {
            return Vec::new();
        }
        let source_module = unsafe { lys_module_name(mod_ptr) };
        unsafe { extract_deviations((*parsed).deviations, &source_module) }
    }

    unsafe fn module_imports(mod_ptr: *const lys_module) -> Vec<RawImport> {
        let parsed = unsafe { (*mod_ptr).parsed };
        if parsed.is_null() {
            return Vec::new();
        }
        let imports = unsafe { (*parsed).imports };
        let count = unsafe { sized_array_count(imports) };
        let mut out = Vec::with_capacity(count);
        for i in 0..count {
            let import = unsafe { imports.add(i) };
            if import.is_null() {
                continue;
            }
            let name = unsafe { cstr_opt((*import).name).unwrap_or_default() };
            let prefix = unsafe { cstr_opt((*import).prefix).unwrap_or_default() };
            let revision = unsafe { cstr_opt((*import).rev.as_ptr()) }.filter(|s| !s.is_empty());
            out.push(RawImport {
                prefix,
                name,
                revision,
            });
        }
        out
    }

    unsafe fn module_identities(mod_ptr: *const lys_module) -> Vec<RawIdentity> {
        let count = unsafe { sized_array_count((*mod_ptr).identities) };
        let mut out = Vec::with_capacity(count);
        for i in 0..count {
            let ident = unsafe { (*mod_ptr).identities.add(i) };
            if ident.is_null() {
                continue;
            }
            let name = unsafe { cstr_opt((*ident).name).unwrap_or_default() };
            let module_name = unsafe {
                if (*ident).module.is_null() {
                    String::new()
                } else {
                    cstr_opt((*(*ident).module).name).unwrap_or_default()
                }
            };

            let derived_count = unsafe { sized_array_count((*ident).derived) };
            let mut derived = Vec::with_capacity(derived_count);
            for j in 0..derived_count {
                let derived_ptr = unsafe { *(*ident).derived.add(j) };
                if derived_ptr.is_null() {
                    continue;
                }
                derived.push(unsafe { format_identity_name(derived_ptr) });
            }

            out.push(RawIdentity {
                name,
                module_name,
                bases: Vec::new(),
                derived,
            });
        }
        out
    }

    /// Walk the compiled schema tree for `module` and return module metadata plus
    /// a coarse description of every node in declaration order.
    pub fn schema_module(&self, module: &str) -> Result<(RawModuleInfo, RawSchemaNode), String> {
        let mod_ptr = self.module_ptr(module)?;
        let info = unsafe { Self::module_info(mod_ptr) };
        let compiled = unsafe { (*mod_ptr).compiled };
        if compiled.is_null() {
            return Err(format!("module {module} has no compiled schema"));
        }
        let mut children = unsafe { walk_compiled_module_children(compiled) };
        if let Some(source_order) = unsafe { module_source_order(mod_ptr) } {
            apply_source_order(&mut children, &mut Vec::new(), &source_order);
        }
        Self::resolve_key_indices(&mut children);
        let module_ns = info.namespace.clone();
        let owner_module_name = info.name.clone();
        let owner_module_namespace = info.namespace.clone();
        Ok((
            info,
            RawSchemaNode {
                name: String::new(),
                kind: "module".to_string(),
                config: RawConfig::Unset,
                status: RawStatus::Current,
                mandatory: false,
                presence: false,
                description: None,
                reference: None,
                extensions: Vec::new(),
                grouping_origin: None,
                units: None,
                default_values: Vec::new(),
                musts: Vec::new(),
                whens: Vec::new(),
                unique_constraints: Vec::new(),
                min_elements: None,
                max_elements: None,
                ordered_by_user: false,
                is_key: false,
                key_names: Vec::new(),
                key_indices: Vec::new(),
                base_type: RawBaseType::Unknown,
                typedef_name: None,
                type_info: RawTypeInfo::unknown(),
                children,
                module_ns,
                owner_module_name,
                owner_module_namespace,
                schema: std::ptr::null(),
            },
        ))
    }

    fn resolve_key_indices(nodes: &mut [RawSchemaNode]) {
        for node in nodes.iter_mut() {
            if node.kind == "list" {
                node.key_indices = node
                    .key_names
                    .iter()
                    .filter_map(|name| {
                        node.children
                            .iter()
                            .position(|c| c.is_key && &c.name == name)
                    })
                    .collect();
            }
            Self::resolve_key_indices(&mut node.children);
        }
    }

    /// Walk the compiled schema tree for `module` in declaration order and return
    /// a coarse description of every node. This is the legacy v1 view; prefer
    /// `schema_module`.
    pub fn schema_tree(&self, module: &str) -> Result<RawSchemaNode, String> {
        self.schema_module(module).map(|(_, tree)| tree)
    }

    /// Enumerate every loaded module and its compiled schema tree.
    pub fn schema_modules(&self) -> Vec<(RawModuleInfo, RawSchemaNode)> {
        let mut out = Vec::new();
        let mut idx: u32 = 0;
        loop {
            let mod_ptr = unsafe { ly_ctx_get_module_iter(self.ptr, &mut idx) };
            if mod_ptr.is_null() {
                break;
            }
            let info = unsafe { Self::module_info(mod_ptr) };
            let compiled = unsafe { (*mod_ptr).compiled };
            // Keep every loaded module in the forest, even when it has no
            // compiled data root. Imported modules are needed for prefix
            // resolution and may carry identities used by implemented modules.
            let mut children = unsafe { walk_compiled_module_children(compiled) };
            if let Some(source_order) = unsafe { module_source_order(mod_ptr) } {
                apply_source_order(&mut children, &mut Vec::new(), &source_order);
            }
            Self::resolve_key_indices(&mut children);
            let module_ns = info.namespace.clone();
            let owner_module_name = info.name.clone();
            let owner_module_namespace = info.namespace.clone();
            out.push((
                info,
                RawSchemaNode {
                    name: String::new(),
                    kind: "module".to_string(),
                    config: RawConfig::Unset,
                    status: RawStatus::Current,
                    mandatory: false,
                    presence: false,
                    description: None,
                    reference: None,
                    extensions: Vec::new(),
                    grouping_origin: None,
                    units: None,
                    default_values: Vec::new(),
                    musts: Vec::new(),
                    whens: Vec::new(),
                    unique_constraints: Vec::new(),
                    min_elements: None,
                    max_elements: None,
                    ordered_by_user: false,
                    is_key: false,
                    key_names: Vec::new(),
                    key_indices: Vec::new(),
                    base_type: RawBaseType::Unknown,
                    typedef_name: None,
                    type_info: RawTypeInfo::unknown(),
                    children,
                    module_ns,
                    owner_module_name,
                    owner_module_namespace,
                    schema: std::ptr::null(),
                },
            ));
        }
        out
    }

    /// Create an empty in-memory data tree tied to this context.
    pub fn new_data(&self) -> RawDataTree {
        RawDataTree {
            ptr: std::ptr::null_mut(),
            ctx: self.ptr,
        }
    }
}

impl Default for RawContext {
    fn default() -> Self {
        Self::new(LY_CTX_NO_YANGLIBRARY | LY_CTX_LEAFREF_EXTENDED | LY_CTX_SET_PRIV_PARSED)
            .expect("default RawContext creation must succeed")
    }
}

impl Drop for RawContext {
    fn drop(&mut self) {
        unsafe {
            ly_ctx_destroy(self.ptr);
        }
    }
}

unsafe impl Send for RawContext {}
unsafe impl Sync for RawContext {}

impl RawDataTree {
    /// Serialize the whole tree to bytes.
    pub fn serialize(&self, format: RawFormat, options: u32) -> Result<Vec<u8>, String> {
        let mut out: *mut ::std::os::raw::c_char = std::ptr::null_mut();
        let options = if matches!(format, RawFormat::JsonIetf) {
            options | LYD_PRINT_EMPTY_CONT
        } else {
            options
        };
        let rc = unsafe { lyd_print_mem(&mut out, self.ptr, format.into(), options) };
        if rc != LY_ERR::LY_SUCCESS {
            return Err(format!("lyd_print_mem failed: {:?}", rc));
        }
        if out.is_null() {
            return Err("lyd_print_mem returned null".to_string());
        }
        let bytes = unsafe {
            let s = CStr::from_ptr(out).to_bytes();
            let v = s.to_vec();
            libc::free(out as *mut _);
            v
        };
        Ok(bytes)
    }

    /// Serialize the whole tree to LYB using the length-aware output handler.
    ///
    /// LYB is binary and may contain embedded NULs, so it cannot use `lyd_print_mem`
    /// (which reads the result via `CStr`). This path uses `ly_out_new_memory`,
    /// `lyd_print_all`, and `ly_out_printed` to copy exactly the printed bytes.
    pub fn serialize_lyb(&self, options: u32) -> Result<Vec<u8>, String> {
        // `lyd_print_all` auto-includes siblings and rejects the SIBLINGS flag.
        let options = options & !LYD_PRINT_SIBLINGS;

        let mut buf: *mut ::std::os::raw::c_char = std::ptr::null_mut();
        let mut out: *mut ly_out = std::ptr::null_mut();
        let rc = unsafe { ly_out_new_memory(&mut buf, 0, &mut out) };
        if rc != LY_ERR::LY_SUCCESS {
            return Err(format!("ly_out_new_memory failed: {:?}", rc));
        }

        let first = unsafe { lyd_first_sibling(self.ptr) };
        let rc = unsafe { lyd_print_all(out, first, LYD_FORMAT::LYD_LYB, options) };
        if rc != LY_ERR::LY_SUCCESS {
            unsafe { ly_out_free(out, None, 1) };
            return Err(format!("lyd_print_all failed: {:?}", rc));
        }

        let len = unsafe { ly_out_printed(out) };
        let bytes = if len == 0 {
            Vec::new()
        } else {
            unsafe { std::slice::from_raw_parts(buf as *const u8, len).to_vec() }
        };

        unsafe { ly_out_free(out, None, 1) };
        Ok(bytes)
    }

    /// Create a deep copy of the whole sibling chain.
    ///
    /// Uses `lyd_dup_siblings` with `LYD_DUP_RECURSIVE` so all roots and children
    /// survive; `LYD_DUP_WITH_FLAGS` preserves default/validated state.
    pub fn duplicate(&self) -> Result<RawDataTree, String> {
        let first = unsafe { lyd_first_sibling(self.ptr) };
        let mut out: *mut lyd_node = std::ptr::null_mut();
        let rc = unsafe {
            lyd_dup_siblings(
                first,
                std::ptr::null_mut(),
                LYD_DUP_RECURSIVE | LYD_DUP_WITH_FLAGS,
                &mut out,
            )
        };
        if rc != LY_ERR::LY_SUCCESS {
            return Err(format!("lyd_dup_siblings failed: {:?}", rc));
        }
        Ok(RawDataTree {
            ptr: out,
            ctx: self.ctx,
        })
    }

    /// Compute the yang-patch-shaped diff between two trees.
    ///
    /// Returns `Ok(None)` when the trees are identical. The caller must free the
    /// returned diff tree exactly once.
    pub fn diff(&self, other: &RawDataTree, defaults: bool) -> Result<Option<RawDataDiff>, String> {
        if self.ctx != other.ctx {
            return Err("diff requires both trees to share the same context".to_string());
        }
        let options: u16 = if defaults {
            LYD_DIFF_DEFAULTS as u16
        } else {
            0
        };
        let first = unsafe { lyd_first_sibling(self.ptr) };
        let second = unsafe { lyd_first_sibling(other.ptr) };
        let mut diff: *mut lyd_node = std::ptr::null_mut();
        let rc = unsafe { lyd_diff_siblings(first, second, options, &mut diff) };
        if rc != LY_ERR::LY_SUCCESS {
            return Err(format!("lyd_diff_siblings failed: {:?}", rc));
        }
        if diff.is_null() {
            Ok(None)
        } else {
            Ok(Some(RawDataDiff {
                ptr: diff,
                ctx: self.ctx,
            }))
        }
    }

    /// Apply a diff tree to `self` in place.
    ///
    /// The diff is borrowed, not consumed.
    pub fn diff_apply(&mut self, diff: &RawDataDiff) -> Result<(), String> {
        let rc = unsafe { lyd_diff_apply_all(&mut self.ptr, diff.ptr) };
        if rc != LY_ERR::LY_SUCCESS {
            return Err(format!("lyd_diff_apply_all failed: {:?}", rc));
        }
        self.ptr = unsafe { lyd_first_sibling(self.ptr) };
        Ok(())
    }

    /// Merge `source` into `self` in place.
    ///
    /// `source` is borrowed and never modified: `lyd_merge_siblings` is called
    /// WITHOUT `LYD_MERGE_DESTRUCT`, so libyang treats `source` as `const` and
    /// dup-copies from it (tree_data.c:2744). No duplication or ownership transfer
    /// is needed, and there is no dup to (double-)free on an error path.
    pub fn merge(&mut self, source: &RawDataTree) -> Result<(), String> {
        if self.ctx != source.ctx {
            return Err("merge requires both trees to share the same context".to_string());
        }
        let source_first = unsafe { lyd_first_sibling(source.ptr) };
        let rc = unsafe { lyd_merge_siblings(&mut self.ptr, source_first, 0) };
        if rc != LY_ERR::LY_SUCCESS {
            return Err(format!("lyd_merge_siblings failed: {:?}", rc));
        }
        self.ptr = unsafe { lyd_first_sibling(self.ptr) };
        Ok(())
    }

    /// Validate the whole tree.
    ///
    /// On `LY_EVALID`, walks the per-context `ly_err_item` list, copies out each
    /// diagnostic, and then cleans the context error list. The tree root pointer
    /// is re-read after validation because libyang may move it.
    pub fn validate(&mut self, options: u32) -> Result<(), RawValidationError> {
        // `ly_log_options` is a PROCESS-GLOBAL toggle. Serialize the whole
        // set-options -> validate -> read-errors -> restore critical section so
        // concurrent validators (e.g. parallel tests) cannot clobber each other's
        // LY_LOSTORE mid-validate and lose accumulated multi-errors.
        static VALIDATE_LOG_LOCK: std::sync::Mutex<()> = std::sync::Mutex::new(());
        let _log_guard = VALIDATE_LOG_LOCK
            .lock()
            .unwrap_or_else(std::sync::PoisonError::into_inner);

        // Store all validation errors so `LYD_VALIDATE_MULTI_ERROR` can return a
        // list. libyang defaults to `LY_LOSTORE_LAST`, which overwrites the
        // previous record on every log.
        let prev_opts = unsafe { ly_log_options(LY_LOLOG | LY_LOSTORE) };
        unsafe { ly_err_clean(self.ctx, std::ptr::null_mut()) };

        let rc = unsafe {
            lyd_validate_all(
                &mut self.ptr,
                std::ptr::null(),
                options,
                std::ptr::null_mut(),
            )
        };

        let result = if rc == LY_ERR::LY_SUCCESS {
            Ok(())
        } else if rc != LY_ERR::LY_EVALID {
            Err(RawValidationError(vec![RawDiagnostic {
                message: format!("lyd_validate_all failed: {:?}", rc),
                data_path: None,
                schema_path: None,
                apptag: None,
                vecode_str: None,
            }]))
        } else {
            let mut diagnostics = Vec::new();
            let mut item = unsafe { ly_err_first(self.ctx) };
            while !item.is_null() {
                let message = unsafe { cstr_opt((*item).msg) };
                let data_path = unsafe { cstr_opt((*item).data_path) };
                let schema_path = unsafe { cstr_opt((*item).schema_path) };
                let apptag = unsafe { cstr_opt((*item).apptag) };
                let vecode = unsafe { (*item).vecode };
                let vecode_str = unsafe { cstr_opt(ly_strvecode(vecode)) };
                if let Some(message) = message {
                    diagnostics.push(RawDiagnostic {
                        message,
                        data_path,
                        schema_path,
                        apptag,
                        vecode_str,
                    });
                }
                item = unsafe { (*item).next };
            }
            unsafe { ly_err_clean(self.ctx, std::ptr::null_mut()) };
            if diagnostics.is_empty() {
                diagnostics.push(RawDiagnostic {
                    message: "validation failed with no diagnostics".to_string(),
                    data_path: None,
                    schema_path: None,
                    apptag: None,
                    vecode_str: None,
                });
            }
            Err(RawValidationError(diagnostics))
        };

        unsafe { ly_log_options(prev_opts) };
        result
    }

    /// Find a node by path relative to this tree.
    pub fn find_path(&self, path: &str) -> Result<*mut ::std::os::raw::c_void, String> {
        let path = CString::new(path).map_err(|e| e.to_string())?;
        let mut node: *mut lyd_node = std::ptr::null_mut();
        let rc = unsafe {
            lyd_find_path(
                self.ptr,
                path.as_ptr(),
                0, // output=false: find a single node, not RPC output
                &mut node,
            )
        };
        if rc != LY_ERR::LY_SUCCESS {
            return Err(format!("lyd_find_path failed: {:?}", rc));
        }
        if node.is_null() {
            return Err(format!("path not found: {path:?}"));
        }
        Ok(node.cast())
    }

    /// Find a node by path, distinguishing "not found" from real errors.
    pub fn find_node(&self, path: &str) -> Result<Option<*mut ::std::os::raw::c_void>, String> {
        if self.ptr.is_null() {
            return Ok(None);
        }
        let path = CString::new(path).map_err(|e| e.to_string())?;
        let mut node: *mut lyd_node = std::ptr::null_mut();
        let rc = unsafe { lyd_find_path(self.ptr, path.as_ptr(), 0, &mut node) };
        match rc {
            LY_ERR::LY_SUCCESS => Ok(Some(node.cast())),
            LY_ERR::LY_ENOTFOUND | LY_ERR::LY_EINCOMPLETE => Ok(None),
            _ => Err(format!("lyd_find_path failed: {:?}", rc)),
        }
    }

    /// Return the canonical value string for a term node at `path`.
    pub fn value_str(&self, path: &str) -> Result<Option<String>, String> {
        let node = match self.find_node(path)? {
            Some(n) => n as *mut lyd_node,
            None => return Ok(None),
        };
        if !unsafe { node_is_term(node) } {
            return Ok(None);
        }
        Ok(unsafe { cstr_opt(cam_lyd_get_value(node)) })
    }

    /// Return true if the node at `path` carries the default flag.
    pub fn is_default(&self, path: &str) -> Result<bool, String> {
        let node = match self.find_node(path)? {
            Some(n) => n as *mut lyd_node,
            None => return Ok(false),
        };
        Ok(unsafe { (*node).flags } & LYD_DEFAULT != 0)
    }

    /// Return the compiled schema pointer for the node at `path`.
    pub fn schema_ptr(&self, path: &str) -> Result<Option<*const ::std::os::raw::c_void>, String> {
        let node = match self.find_node(path)? {
            Some(n) => n as *mut lyd_node,
            None => return Ok(None),
        };
        Ok(Some(unsafe {
            (*node).schema as *const ::std::os::raw::c_void
        }))
    }

    /// Evaluate an XPath against this tree and return the matched node pointers.
    pub fn find_xpath(&self, xpath: &str) -> Result<Vec<*mut ::std::os::raw::c_void>, String> {
        if self.ptr.is_null() {
            return Ok(Vec::new());
        }
        let xpath = CString::new(xpath).map_err(|e| e.to_string())?;
        let mut set: *mut ly_set = std::ptr::null_mut();
        let rc = unsafe { lyd_find_xpath(self.ptr, xpath.as_ptr(), &mut set) };
        if rc != LY_ERR::LY_SUCCESS {
            return Err(format!("lyd_find_xpath failed: {:?}", rc));
        }
        let mut out = Vec::new();
        if !set.is_null() {
            let count = unsafe { (*set).count } as usize;
            let dnodes = unsafe { (*set).__bindgen_anon_1.dnodes };
            for i in 0..count {
                let node = unsafe { *dnodes.add(i) };
                if !node.is_null() {
                    out.push(node.cast());
                }
            }
            unsafe { ly_set_free(set, None) };
        }
        Ok(out)
    }

    /// Evaluate an XPath and return absolute paths for each matched node.
    ///
    /// The `ly_set` is freed before returning; only owned paths cross the
    /// boundary.
    pub fn xpath_paths(&self, xpath: &str) -> Result<Vec<String>, String> {
        let ptrs = self.find_xpath(xpath)?;
        let mut out = Vec::with_capacity(ptrs.len());
        for ptr in ptrs {
            let node = ptr as *mut lyd_node;
            if let Some(path) = unsafe { node_path(node) } {
                out.push(path);
            }
        }
        Ok(out)
    }

    /// Create or update a node at `path`.
    ///
    /// `value` is the canonical string for a leaf/leaf-list; `None` creates an
    /// inner node. The tree root is re-anchored after creation because libyang
    /// may insert a new top-level sibling before the previous first sibling.
    pub fn new_path(
        &mut self,
        path: &str,
        value: Option<&str>,
        options: u32,
    ) -> Result<(), String> {
        let path = CString::new(path).map_err(|e| e.to_string())?;
        let value_cstr = value
            .map(CString::new)
            .transpose()
            .map_err(|e| e.to_string())?;
        let (value_ptr, value_size_bits) = match value_cstr {
            Some(ref v) => (v.as_ptr() as *const ::std::os::raw::c_void, 0u64),
            None => (std::ptr::null(), 0u64),
        };
        let mut new_parent: *mut lyd_node = std::ptr::null_mut();
        let mut new_node: *mut lyd_node = std::ptr::null_mut();
        let rc = unsafe {
            lyd_new_path2(
                self.ptr,
                self.ctx,
                path.as_ptr(),
                value_ptr,
                value_size_bits,
                0,
                options,
                &mut new_parent,
                &mut new_node,
            )
        };
        if rc != LY_ERR::LY_SUCCESS {
            return Err(format!("lyd_new_path2 failed: {:?}", rc));
        }
        // Re-anchor the canonical first sibling as our root.
        if self.ptr.is_null() {
            let anchor = if !new_parent.is_null() {
                new_parent
            } else {
                new_node
            };
            if !anchor.is_null() {
                self.ptr = unsafe { lyd_first_sibling(anchor) };
            }
        } else {
            self.ptr = unsafe { lyd_first_sibling(self.ptr) };
        }
        Ok(())
    }

    /// Change the value of an existing leaf/leaf-list.
    ///
    /// Returns `true` if the value (or default flag) changed, `false` if the
    /// value was identical.
    pub fn set_value(&mut self, path: &str, value: &str) -> Result<bool, String> {
        let node = self.find_path(path)? as *mut lyd_node;
        if !unsafe { node_is_term(node) } {
            return Err("set_value target is not a leaf/leaf-list".to_string());
        }
        let value = CString::new(value).map_err(|e| e.to_string())?;
        let rc = unsafe { lyd_change_term(node, value.as_ptr()) };
        match rc {
            LY_ERR::LY_SUCCESS => Ok(true),
            LY_ERR::LY_ENOT => Ok(false),
            // Same value but the default flag was cleared: a real state change.
            LY_ERR::LY_EEXIST => Ok(true),
            _ => Err(format!("lyd_change_term failed: {:?}", rc)),
        }
    }

    /// Remove and free the subtree at `path`.
    pub fn remove_path(&mut self, path: &str) -> Result<(), String> {
        let node = self.find_path(path)? as *mut lyd_node;
        // libyang's lyd_free_tree silently refuses to free a list key (it logs
        // and returns void), which would be a false success. Reject key leaves
        // deterministically — the caller must remove the list entry, not its key.
        let schema = unsafe { (*node).schema };
        if !schema.is_null()
            && unsafe { (*schema).nodetype } as u32 == LYS_LEAF
            && (unsafe { (*schema).flags } as u32 & LYS_KEY) != 0
            && !unsafe { (*node).parent }.is_null()
        {
            return Err(format!(
                "cannot remove a list key {path:?}; remove the list entry instead"
            ));
        }
        self.reanchor_before_detach(node);
        unsafe { lyd_free_tree(node) };
        Ok(())
    }

    /// Detach the subtree at `path` and return it as an owned tree.
    ///
    /// The detached tree shares the same context and may only be re-inserted
    /// into a tree from that same context.
    pub fn unlink_path(&mut self, path: &str) -> Result<RawDataTree, String> {
        let node = self.find_path(path)? as *mut lyd_node;
        self.reanchor_before_detach(node);
        let rc = unsafe { lyd_unlink_tree(node) };
        if rc != LY_ERR::LY_SUCCESS {
            return Err(format!("lyd_unlink_tree failed: {:?}", rc));
        }
        Ok(RawDataTree {
            ptr: node,
            ctx: self.ctx,
        })
    }

    /// Add implicit/default nodes to the tree.
    pub fn add_defaults(&mut self, options: u32) -> Result<(), String> {
        let rc =
            unsafe { lyd_new_implicit_all(&mut self.ptr, self.ctx, options, std::ptr::null_mut()) };
        if rc != LY_ERR::LY_SUCCESS {
            return Err(format!("lyd_new_implicit_all failed: {:?}", rc));
        }
        // An implicit node may be inserted before the current first sibling;
        // re-anchor self.ptr to the canonical root so serialize and Drop stay correct.
        if !self.ptr.is_null() {
            self.ptr = unsafe { lyd_first_sibling(self.ptr) };
        }
        Ok(())
    }

    fn reanchor_before_detach(&mut self, node: *mut lyd_node) {
        if self.ptr.is_null() {
            return;
        }
        let root = unsafe { lyd_first_sibling(self.ptr) };
        if root == node {
            // The node being removed is the current root; the new root is its
            // next sibling (may be null).
            self.ptr = unsafe { (*node).next };
        } else {
            self.ptr = root;
        }
    }

    /// Materialize the immediate children of `path` in one forward sibling walk.
    ///
    /// The returned `RawChildInfo` contains the absolute path and name for each
    /// child; no libyang function is called inside the loop body.
    pub fn children_of(&self, path: &str) -> Result<Vec<RawChildInfo>, String> {
        let parent = match self.find_node(path)? {
            Some(n) => n as *mut lyd_node,
            None => return Ok(Vec::new()),
        };
        let mut out = Vec::new();
        let mut child = unsafe { node_child_first(parent) }.unwrap_or(std::ptr::null_mut());
        while !child.is_null() {
            if let Some(path) = unsafe { node_path(child) } {
                if let Some(name) = unsafe { node_schema_name(child) } {
                    let is_default = unsafe { (*child).flags } & LYD_DEFAULT != 0;
                    out.push(RawChildInfo {
                        path,
                        name,
                        is_default,
                    });
                }
            }
            child = unsafe { (*child).next };
        }
        Ok(out)
    }

    /// Materialize all siblings of the node at `path` (including the node itself)
    /// in one forward sibling walk.
    pub fn siblings_of(&self, path: &str) -> Result<Vec<RawChildInfo>, String> {
        let node = match self.find_node(path)? {
            Some(n) => n as *mut lyd_node,
            None => return Ok(Vec::new()),
        };
        let first = unsafe { lyd_first_sibling(node) };
        let mut out = Vec::new();
        let mut cur = first;
        while !cur.is_null() {
            if let Some(path) = unsafe { node_path(cur) } {
                if let Some(name) = unsafe { node_schema_name(cur) } {
                    let is_default = unsafe { (*cur).flags } & LYD_DEFAULT != 0;
                    out.push(RawChildInfo {
                        path,
                        name,
                        is_default,
                    });
                }
            }
            cur = unsafe { (*cur).next };
            if cur == first {
                break;
            }
        }
        Ok(out)
    }

    /// Materialize the top-level sibling chain of this tree.
    pub fn root_nodes(&self) -> Result<Vec<RawChildInfo>, String> {
        if self.ptr.is_null() {
            return Ok(Vec::new());
        }
        let first = unsafe { lyd_first_sibling(self.ptr) };
        let mut out = Vec::new();
        let mut cur = first;
        while !cur.is_null() {
            if let Some(path) = unsafe { node_path(cur) } {
                if let Some(name) = unsafe { node_schema_name(cur) } {
                    let is_default = unsafe { (*cur).flags } & LYD_DEFAULT != 0;
                    out.push(RawChildInfo {
                        path,
                        name,
                        is_default,
                    });
                }
            }
            cur = unsafe { (*cur).next };
            if cur == first {
                break;
            }
        }
        Ok(out)
    }

    /// Consume this tree and return its root pointer without freeing it.
    pub fn into_raw(mut self) -> *mut lyd_node {
        let ptr = self.ptr;
        self.ptr = std::ptr::null_mut();
        ptr
    }
}

impl Drop for RawDataTree {
    fn drop(&mut self) {
        if !self.ptr.is_null() {
            unsafe {
                lyd_free_all(self.ptr);
            }
        }
    }
}

/// Diff operation carried on a diff-tree node.
#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub enum RawDiffOp {
    /// `create`.
    Create,
    /// `delete`.
    Delete,
    /// `replace`.
    Replace,
    /// `none`.
    None,
}

/// One materialized diff edit.
#[derive(Clone, Debug)]
pub struct RawDiffEdit {
    /// The `yang:operation` for this edit, inherited from the nearest parent.
    pub op: RawDiffOp,
    /// Absolute data path of the edited node.
    pub path: String,
    /// Canonical value for leaf/leaf-list edits, if any.
    pub value: Option<String>,
    /// True if the edited node is in an `ordered-by user` list/leaf-list.
    pub is_user_ordered: bool,
}

/// Owned yang-patch-shaped diff tree returned by `RawDataTree::diff`.
pub struct RawDataDiff {
    ptr: *mut lyd_node,
    ctx: *const ly_ctx,
}

impl RawDataDiff {
    /// Serialize the diff tree to bytes.
    pub fn serialize(&self, format: RawFormat, options: u32) -> Result<Vec<u8>, String> {
        if self.ptr.is_null() {
            return Ok(Vec::new());
        }
        if matches!(format, RawFormat::Lyb) {
            return self.serialize_lyb(options);
        }

        let mut out: *mut ::std::os::raw::c_char = std::ptr::null_mut();
        let options = if matches!(format, RawFormat::JsonIetf) {
            options | LYD_PRINT_EMPTY_CONT
        } else {
            options
        };
        let rc = unsafe { lyd_print_mem(&mut out, self.ptr, format.into(), options) };
        if rc != LY_ERR::LY_SUCCESS {
            return Err(format!("lyd_print_mem failed: {:?}", rc));
        }
        if out.is_null() {
            return Ok(Vec::new());
        }
        let bytes = unsafe {
            let s = CStr::from_ptr(out).to_bytes();
            let v = s.to_vec();
            libc::free(out as *mut _);
            v
        };
        Ok(bytes)
    }

    fn serialize_lyb(&self, options: u32) -> Result<Vec<u8>, String> {
        let options = options & !LYD_PRINT_SIBLINGS;

        let mut buf: *mut ::std::os::raw::c_char = std::ptr::null_mut();
        let mut out: *mut ly_out = std::ptr::null_mut();
        let rc = unsafe { ly_out_new_memory(&mut buf, 0, &mut out) };
        if rc != LY_ERR::LY_SUCCESS {
            return Err(format!("ly_out_new_memory failed: {:?}", rc));
        }

        let rc = unsafe { lyd_print_all(out, self.ptr, LYD_FORMAT::LYD_LYB, options) };
        if rc != LY_ERR::LY_SUCCESS {
            unsafe { ly_out_free(out, None, 1) };
            return Err(format!("lyd_print_all failed: {:?}", rc));
        }

        let len = unsafe { ly_out_printed(out) };
        let bytes = if len == 0 {
            Vec::new()
        } else {
            unsafe { std::slice::from_raw_parts(buf as *const u8, len).to_vec() }
        };

        unsafe { ly_out_free(out, None, 1) };
        Ok(bytes)
    }

    /// Materialize all diff edits in one pre-order walk of the diff tree.
    pub fn edits(&self) -> Result<Vec<RawDiffEdit>, String> {
        let mut out = Vec::new();
        let yang_mod = unsafe { ly_ctx_get_module_implemented(self.ctx, cstr_yang()) };
        if yang_mod.is_null() {
            return Err("yang module is not implemented in context".to_string());
        }
        self.collect_edits(self.ptr, yang_mod, &mut out)?;
        Ok(out)
    }

    fn collect_edits(
        &self,
        node: *const lyd_node,
        yang_mod: *const lys_module,
        out: &mut Vec<RawDiffEdit>,
    ) -> Result<(), String> {
        let mut cur = node;
        while !cur.is_null() {
            let op = self.inherited_op(cur, yang_mod)?;
            // List keys are immutable (RFC 7950 §7.8.2) and appear in a diff
            // subtree only to identify the list instance — never as an independent
            // edit. On a user-ordered move libyang inherits the parent's `replace`
            // onto the key leaf; skipping keys keeps the move one atomic edit and
            // removes the artifact at its source (no post-hoc filtering needed).
            let is_key = unsafe {
                let schema = (*cur).schema;
                !schema.is_null() && ((*schema).flags as u32 & LYS_KEY) != 0
            };
            if !matches!(op, RawDiffOp::None) && !is_key {
                let path = self.node_path(cur)?;
                let value = if unsafe { node_is_term(cur) } {
                    unsafe { node_value_str_direct(cur) }
                } else {
                    None
                };
                let is_user_ordered = unsafe {
                    let schema = (*cur).schema;
                    !schema.is_null() && ((*schema).flags as u32 & LYS_ORDBY_USER) != 0
                };
                out.push(RawDiffEdit {
                    op,
                    path,
                    value,
                    is_user_ordered,
                });
            }

            if let Some(child) = unsafe { node_child_first(cur) } {
                self.collect_edits(child, yang_mod, out)?;
            }

            cur = unsafe { (*cur).next };
            if cur == node {
                break;
            }
        }
        Ok(())
    }

    fn inherited_op(
        &self,
        node: *const lyd_node,
        yang_mod: *const lys_module,
    ) -> Result<RawDiffOp, String> {
        let mut cur = node;
        while !cur.is_null() {
            let meta = unsafe {
                lyd_find_meta((*cur).meta as *const lyd_meta, yang_mod, cstr_operation())
            };
            if !meta.is_null() {
                let val = unsafe { meta_value_str(meta, self.ctx) }?;
                return Ok(match val.as_deref() {
                    Some("create") => RawDiffOp::Create,
                    Some("delete") => RawDiffOp::Delete,
                    Some("replace") => RawDiffOp::Replace,
                    Some("none") => RawDiffOp::None,
                    _ => RawDiffOp::None,
                });
            }
            cur = unsafe { (*cur).parent };
        }
        Ok(RawDiffOp::None)
    }

    fn node_path(&self, node: *const lyd_node) -> Result<String, String> {
        let path = unsafe { lyd_path(node, LYD_PATH_TYPE::LYD_PATH_STD, std::ptr::null_mut(), 0) };
        if path.is_null() {
            return Err("lyd_path returned null".to_string());
        }
        let s = unsafe { cstr_opt(path) }.unwrap_or_default();
        unsafe { libc::free(path as *mut _) };
        Ok(s)
    }
}

impl Drop for RawDataDiff {
    fn drop(&mut self) {
        if !self.ptr.is_null() {
            unsafe {
                lyd_free_all(self.ptr);
            }
        }
    }
}

fn cstr_yang() -> *const ::std::os::raw::c_char {
    CStr::from_bytes_with_nul(b"yang\0").unwrap().as_ptr()
}

fn cstr_operation() -> *const ::std::os::raw::c_char {
    CStr::from_bytes_with_nul(b"operation\0").unwrap().as_ptr()
}

unsafe fn meta_value_str(
    meta: *const lyd_meta,
    _ctx: *const ly_ctx,
) -> Result<Option<String>, String> {
    if meta.is_null() {
        return Ok(None);
    }
    Ok(unsafe { cstr_opt(cam_lyd_get_meta_value(meta)) })
}

/// Operation type for `lyd_parse_op`.
#[derive(Clone, Copy, Debug)]
pub enum RawOpType {
    /// YANG RPC.
    RpcYang,
    /// YANG notification.
    NotifYang,
    /// YANG RPC/action reply.
    ReplyYang,
}

impl From<RawOpType> for lyd_type {
    fn from(t: RawOpType) -> Self {
        match t {
            RawOpType::RpcYang => lyd_type::LYD_TYPE_RPC_YANG,
            RawOpType::NotifYang => lyd_type::LYD_TYPE_NOTIF_YANG,
            RawOpType::ReplyYang => lyd_type::LYD_TYPE_REPLY_YANG,
        }
    }
}

/// Wire format understood by libyang.
#[derive(Clone, Copy, Debug)]
pub enum RawFormat {
    Xml,
    Json,
    /// RFC 7951 JSON as used by gNMI JSON_IETF (preserves empty containers).
    JsonIetf,
    /// Binary LYB format.
    Lyb,
}

impl From<RawFormat> for LYD_FORMAT {
    fn from(f: RawFormat) -> Self {
        match f {
            RawFormat::Xml => LYD_FORMAT::LYD_XML,
            RawFormat::Json | RawFormat::JsonIetf => LYD_FORMAT::LYD_JSON,
            RawFormat::Lyb => LYD_FORMAT::LYD_LYB,
        }
    }
}

fn path_as_cstring<P: AsRef<Path>>(path: P) -> Result<CString, String> {
    let s = path
        .as_ref()
        .to_str()
        .ok_or("path contains invalid UTF-8")?;
    CString::new(s).map_err(|e| e.to_string())
}

/// Opaque handle to a `ordered-by user` list instance.
///
/// All operations are positional. The handle is constructed from any entry of
/// the target list; operations affect all sibling entries under the same parent.
/// The caller is responsible for ensuring the parent `RawDataTree` outlives
/// this handle.
#[derive(Debug)]
pub struct RawUserOrderedList {
    parent: *mut lyd_node,
    schema_name: String,
}

impl RawUserOrderedList {
    /// Wrap the parent of an existing list entry. Unsafe because the pointer
    /// must remain valid.
    pub unsafe fn from_raw(entry: *mut ::std::os::raw::c_void) -> Self {
        let entry = entry.cast::<lyd_node>();
        let schema_name = unsafe { node_schema_name(entry) }.unwrap_or_default();
        Self {
            parent: unsafe { (*entry).parent },
            schema_name,
        }
    }

    fn first_child(&self) -> Option<*mut lyd_node> {
        let name = self.schema_name.as_str();
        let mut child = unsafe { lyd_child(self.parent) };
        while !child.is_null() {
            if unsafe { node_schema_name(child) }.as_deref() == Some(name) {
                return Some(child);
            }
            child = unsafe { (*child).next };
        }
        None
    }

    fn nth_child(&self, n: usize) -> Option<*mut lyd_node> {
        let name = self.schema_name.as_str();
        let mut child = self.first_child()?;
        for _ in 0..n {
            child = unsafe { (*child).next };
            while !child.is_null() && unsafe { node_schema_name(child) }.as_deref() != Some(name) {
                child = unsafe { (*child).next };
            }
            if child.is_null() {
                return None;
            }
        }
        Some(child)
    }

    /// Number of list entries.
    pub fn len(&self) -> usize {
        let name = self.schema_name.as_str();
        let mut count = 0;
        let mut child = unsafe { lyd_child(self.parent) };
        while !child.is_null() {
            if unsafe { node_schema_name(child) }.as_deref() == Some(name) {
                count += 1;
            }
            child = unsafe { (*child).next };
        }
        count
    }

    /// True if there are no list entries.
    pub fn is_empty(&self) -> bool {
        self.len() == 0
    }

    /// Absolute data path of the entry at `index`, if it exists.
    pub fn path_at(&self, index: usize) -> Option<String> {
        let node = self.nth_child(index)?;
        unsafe { node_path(node) }
    }

    /// Absolute data paths of all entries in sibling order.
    pub fn paths(&self) -> Vec<String> {
        let name = self.schema_name.as_str();
        let mut out = Vec::new();
        let mut child = unsafe { lyd_child(self.parent) };
        while !child.is_null() {
            if unsafe { node_schema_name(child) }.as_deref() == Some(name) {
                if let Some(path) = unsafe { node_path(child) } {
                    out.push(path);
                }
            }
            child = unsafe { (*child).next };
        }
        out
    }

    /// Remove the entry at `index`.
    pub fn remove(&mut self, index: usize) -> Result<(), String> {
        let node = self.nth_child(index).ok_or("remove index out of range")?;
        unsafe {
            lyd_unlink_tree(node);
            lyd_free_tree(node);
        }
        Ok(())
    }

    pub fn insert_first(&mut self, entry: RawDataTree) -> Result<(), String> {
        let first = unsafe { lyd_child(self.parent) };
        if first.is_null() {
            self.insert_last(entry)
        } else {
            self.insert_before_node(first, entry)
        }
    }

    pub fn insert_last(&mut self, entry: RawDataTree) -> Result<(), String> {
        let node = entry.into_raw();
        let rc = unsafe { lyd_insert_child(self.parent, node) };
        if rc != LY_ERR::LY_SUCCESS {
            // `into_raw` already took ownership; free the orphan so it cannot leak.
            unsafe { lyd_free_tree(node) };
            return Err(format!("lyd_insert_child failed: {:?}", rc));
        }
        Ok(())
    }

    pub fn insert_before(&mut self, index: usize, entry: RawDataTree) -> Result<(), String> {
        // Filtered `self.nth_child`: `index` addresses THIS list's entries, not
        // every sibling of a heterogeneous parent container.
        let sibling = self
            .nth_child(index)
            .ok_or("insert_before index out of range")?;
        self.insert_before_node(sibling, entry)
    }

    pub fn insert_after(&mut self, index: usize, entry: RawDataTree) -> Result<(), String> {
        let sibling = self
            .nth_child(index)
            .ok_or("insert_after index out of range")?;
        let node = entry.into_raw();
        let rc = unsafe { lyd_insert_after(sibling, node) };
        if rc != LY_ERR::LY_SUCCESS {
            unsafe { lyd_free_tree(node) };
            return Err(format!("lyd_insert_after failed: {:?}", rc));
        }
        Ok(())
    }

    pub fn move_before(&mut self, what: usize, point: usize) -> Result<(), String> {
        let what_node = self
            .nth_child(what)
            .ok_or("move_before source index out of range")?;
        let point_node = self
            .nth_child(point)
            .ok_or("move_before target index out of range")?;
        let rc = unsafe { lyd_insert_before(point_node, what_node) };
        if rc != LY_ERR::LY_SUCCESS {
            return Err(format!("lyd_insert_before (move) failed: {:?}", rc));
        }
        Ok(())
    }

    pub fn move_after(&mut self, what: usize, point: usize) -> Result<(), String> {
        let what_node = self
            .nth_child(what)
            .ok_or("move_after source index out of range")?;
        let point_node = self
            .nth_child(point)
            .ok_or("move_after target index out of range")?;
        let rc = unsafe { lyd_insert_after(point_node, what_node) };
        if rc != LY_ERR::LY_SUCCESS {
            return Err(format!("lyd_insert_after (move) failed: {:?}", rc));
        }
        Ok(())
    }

    fn insert_before_node(
        &mut self,
        sibling: *mut lyd_node,
        entry: RawDataTree,
    ) -> Result<(), String> {
        let node = entry.into_raw();
        let rc = unsafe { lyd_insert_before(sibling, node) };
        if rc != LY_ERR::LY_SUCCESS {
            unsafe { lyd_free_tree(node) };
            return Err(format!("lyd_insert_before failed: {:?}", rc));
        }
        Ok(())
    }
}

unsafe fn lyd_child(node: *mut lyd_node) -> *mut lyd_node {
    unsafe { node_child_first(node) }.unwrap_or(std::ptr::null_mut())
}

/// Positional-only handle over the instances of one `ordered-by user` leaf-list.
#[derive(Debug)]
pub struct RawUserOrderedLeafList {
    parent: *mut lyd_node,
    schema_name: String,
}

impl RawUserOrderedLeafList {
    /// Wrap the parent of an existing leaf-list instance. Unsafe because the
    /// pointer must remain valid.
    pub unsafe fn from_raw(entry: *mut ::std::os::raw::c_void) -> Self {
        let entry = entry.cast::<lyd_node>();
        let schema_name = unsafe { node_schema_name(entry) }.unwrap_or_default();
        Self {
            parent: unsafe { (*entry).parent },
            schema_name,
        }
    }

    fn name(&self) -> &str {
        self.schema_name.as_str()
    }

    fn matches(&self, node: *const lyd_node) -> bool {
        unsafe { node_schema_name(node) }.as_deref() == Some(self.name())
    }

    fn first_child(&self) -> Option<*mut lyd_node> {
        let mut child = unsafe { lyd_child(self.parent) };
        while !child.is_null() {
            if self.matches(child) {
                return Some(child);
            }
            child = unsafe { (*child).next };
        }
        None
    }

    fn last_child(&self) -> Option<*mut lyd_node> {
        let mut last = None;
        let mut child = unsafe { lyd_child(self.parent) };
        while !child.is_null() {
            if self.matches(child) {
                last = Some(child);
            }
            child = unsafe { (*child).next };
        }
        last
    }

    fn nth_child(&self, n: usize) -> Option<*mut lyd_node> {
        let mut child = self.first_child()?;
        for _ in 0..n {
            child = unsafe { (*child).next };
            while !child.is_null() && !self.matches(child) {
                child = unsafe { (*child).next };
            }
            if child.is_null() {
                return None;
            }
        }
        Some(child)
    }

    fn new_leaf(&self, value: &str) -> Result<*mut lyd_node, String> {
        let schema = unsafe { (*self.parent).schema };
        if schema.is_null() {
            return Err("leaf-list parent has no schema".to_string());
        }
        let module = unsafe { (*schema).module };
        if module.is_null() {
            return Err("leaf-list parent has no module".to_string());
        }
        let name = CString::new(self.schema_name.as_str()).map_err(|e| e.to_string())?;
        let value = CString::new(value).map_err(|e| e.to_string())?;
        let mut node: *mut lyd_node = std::ptr::null_mut();
        let rc = unsafe {
            lyd_new_term(
                self.parent,
                module,
                name.as_ptr(),
                value.as_ptr(),
                0,
                &mut node,
            )
        };
        if rc != LY_ERR::LY_SUCCESS {
            return Err(format!("lyd_new_term failed: {:?}", rc));
        }
        if node.is_null() {
            return Err("lyd_new_term returned null".to_string());
        }
        Ok(node)
    }

    /// Number of leaf-list values.
    pub fn len(&self) -> usize {
        let mut count = 0;
        let mut child = unsafe { lyd_child(self.parent) };
        while !child.is_null() {
            if self.matches(child) {
                count += 1;
            }
            child = unsafe { (*child).next };
        }
        count
    }

    /// True if there are no leaf-list values.
    pub fn is_empty(&self) -> bool {
        self.len() == 0
    }

    /// Canonical value string at `index`, if it exists.
    pub fn value_at(&self, index: usize) -> Option<String> {
        let node = self.nth_child(index)?;
        unsafe { node_value_str_direct(node) }
    }

    /// All values in sibling order.
    pub fn values(&self) -> Vec<String> {
        let mut out = Vec::new();
        let mut child = unsafe { lyd_child(self.parent) };
        while !child.is_null() {
            if self.matches(child) {
                if let Some(value) = unsafe { node_value_str_direct(child) } {
                    out.push(value);
                }
            }
            child = unsafe { (*child).next };
        }
        out
    }

    /// Insert `value` as the first leaf-list instance.
    pub fn insert_first(&mut self, value: &str) -> Result<(), String> {
        let node = self.new_leaf(value)?;
        if let Some(first) = self.first_child() {
            if first != node {
                let rc = unsafe { lyd_insert_before(first, node) };
                if rc != LY_ERR::LY_SUCCESS {
                    unsafe { lyd_free_tree(node) };
                    return Err(format!("lyd_insert_before failed: {:?}", rc));
                }
            }
        }
        Ok(())
    }

    /// Insert `value` as the last leaf-list instance.
    pub fn insert_last(&mut self, value: &str) -> Result<(), String> {
        let node = self.new_leaf(value)?;
        if let Some(last) = self.last_child() {
            if last != node {
                let rc = unsafe { lyd_insert_after(last, node) };
                if rc != LY_ERR::LY_SUCCESS {
                    unsafe { lyd_free_tree(node) };
                    return Err(format!("lyd_insert_after failed: {:?}", rc));
                }
            }
        }
        Ok(())
    }

    /// Insert `value` before the instance at `index`.
    pub fn insert_before(&mut self, index: usize, value: &str) -> Result<(), String> {
        let sibling = self
            .nth_child(index)
            .ok_or("insert_before index out of range")?;
        let node = self.new_leaf(value)?;
        if sibling != node {
            let rc = unsafe { lyd_insert_before(sibling, node) };
            if rc != LY_ERR::LY_SUCCESS {
                unsafe { lyd_free_tree(node) };
                return Err(format!("lyd_insert_before failed: {:?}", rc));
            }
        }
        Ok(())
    }

    /// Insert `value` after the instance at `index`.
    pub fn insert_after(&mut self, index: usize, value: &str) -> Result<(), String> {
        let sibling = self
            .nth_child(index)
            .ok_or("insert_after index out of range")?;
        let node = self.new_leaf(value)?;
        if sibling != node {
            let rc = unsafe { lyd_insert_after(sibling, node) };
            if rc != LY_ERR::LY_SUCCESS {
                unsafe { lyd_free_tree(node) };
                return Err(format!("lyd_insert_after failed: {:?}", rc));
            }
        }
        Ok(())
    }

    /// Move the instance at `what` before the instance at `point`.
    pub fn move_before(&mut self, what: usize, point: usize) -> Result<(), String> {
        let what_node = self
            .nth_child(what)
            .ok_or("move_before source index out of range")?;
        let point_node = self
            .nth_child(point)
            .ok_or("move_before target index out of range")?;
        let rc = unsafe { lyd_insert_before(point_node, what_node) };
        if rc != LY_ERR::LY_SUCCESS {
            return Err(format!("lyd_insert_before (move) failed: {:?}", rc));
        }
        Ok(())
    }

    /// Move the instance at `what` after the instance at `point`.
    pub fn move_after(&mut self, what: usize, point: usize) -> Result<(), String> {
        let what_node = self
            .nth_child(what)
            .ok_or("move_after source index out of range")?;
        let point_node = self
            .nth_child(point)
            .ok_or("move_after target index out of range")?;
        let rc = unsafe { lyd_insert_after(point_node, what_node) };
        if rc != LY_ERR::LY_SUCCESS {
            return Err(format!("lyd_insert_after (move) failed: {:?}", rc));
        }
        Ok(())
    }

    /// Remove the instance at `index`.
    pub fn remove(&mut self, index: usize) -> Result<(), String> {
        let node = self.nth_child(index).ok_or("remove index out of range")?;
        unsafe {
            lyd_unlink_tree(node);
            lyd_free_tree(node);
        }
        Ok(())
    }
}
