//! Rust conformance runner for the shared `/conformance` corpus.
//!
//! Reads `conformance/manifest.toml`, loads the advertised YANG modules, parses
//! each input fixture, and asserts that Cambium serializes it to the golden
//! outputs byte-for-byte. When `CAMBIUM_YANGLINT` is available the runner also
//! compares Cambium's output against an independent `yanglint` invocation.

use std::collections::HashMap;
use std::ffi::OsStr;
use std::fs;
use std::path::{Path, PathBuf};
use std::process::Command;

use cambium::{
    Context, ContextBuilder, Format, OpType, ParseMode, ResolvedType, Result, SchemaNodeRef,
    SerializeFlags, WithDefaults,
};
use serde::Deserialize;

/// Path to the `yanglint` binary built alongside libyang, if any.
const YANGLINT: Option<&str> = option_env!("CAMBIUM_YANGLINT");

#[derive(Debug, Deserialize)]
struct Manifest {
    case: Vec<Case>,
}

#[derive(Debug, Deserialize)]
struct Case {
    name: String,
    #[serde(default)]
    tier: Option<String>,
    module: PathBuf,
    #[serde(default)]
    input: Option<PathBuf>,
    #[serde(rename = "input-format", default)]
    input_format: Option<String>,
    #[serde(rename = "op-type")]
    op_type: Option<String>,
    #[serde(rename = "serialize-defaults")]
    serialize_defaults: Option<String>,
    #[serde(default)]
    expected: HashMap<String, PathBuf>,
    #[serde(rename = "expected-ir", default)]
    expected_ir: Option<PathBuf>,
    #[serde(default)]
    oracle: bool,
}

impl Case {
    fn is_schema_ir(&self) -> bool {
        self.tier.as_deref() == Some("schema-ir")
    }

    fn validate_tier(&self) -> Result<()> {
        match self.tier.as_deref() {
            None | Some("backend-data") | Some("schema-ir") => Ok(()),
            Some(other) => Err(format!("case {}: invalid tier {:?}", self.name, other).into()),
        }
    }
}

fn main() {
    if let Err(e) = run() {
        eprintln!("conformance runner failed: {e}");
        std::process::exit(1);
    }
}

fn run() -> Result<()> {
    let root = project_root()?;
    let conformance = root.join("conformance");
    let manifest_path = conformance.join("manifest.toml");
    let manifest_src =
        fs::read_to_string(&manifest_path).map_err(|e| format!("read manifest: {e}"))?;
    let manifest: Manifest =
        toml::from_str(&manifest_src).map_err(|e| format!("parse manifest: {e}"))?;

    for case in &manifest.case {
        case.validate_tier()?;
    }

    let mut failures = Vec::new();
    let mut passed = 0;

    for case in &manifest.case {
        let result = if case.is_schema_ir() {
            run_schema_ir_case(&conformance, case)
        } else {
            run_case(&conformance, case)
        };
        if let Err(e) = result {
            failures.push(format!("{}: {e}", case.name));
        } else {
            passed += 1;
        }
    }

    if failures.is_empty() {
        println!("conformance: {passed} case(s) passed");
        Ok(())
    } else {
        for f in &failures {
            eprintln!("FAIL {f}");
        }
        Err(format!("{} conformance case(s) failed", failures.len()).into())
    }
}

#[derive(Debug, Deserialize)]
#[serde(rename_all = "camelCase")]
struct SchemaIrExpected {
    module: String,
    #[serde(default)]
    load: Vec<String>,
    #[serde(default)]
    children: HashMap<String, Vec<String>>,
    #[serde(default)]
    data_children_flatten: HashMap<String, Vec<String>>,
    #[serde(default)]
    keys: HashMap<String, Vec<String>>,
    #[serde(default)]
    imports: HashMap<String, Vec<SchemaIrImport>>,
    #[serde(default)]
    prefixes: HashMap<String, HashMap<String, String>>,
    #[serde(default)]
    identity_derived: HashMap<String, Vec<String>>,
    #[serde(default)]
    leafrefs: HashMap<String, SchemaIrLeafref>,
}

#[derive(Debug, Deserialize)]
struct SchemaIrImport {
    prefix: String,
    name: String,
    #[serde(default)]
    revision: Option<String>,
}

#[derive(Debug, Deserialize)]
struct SchemaIrLeafref {
    path: String,
    target: String,
}

fn run_schema_ir_case(conformance: &Path, case: &Case) -> Result<()> {
    let module_dir = conformance.join(&case.module);
    let expected_ir = case
        .expected_ir
        .as_ref()
        .ok_or_else(|| format!("schema-ir case {} has no expected-ir", case.name))?;
    let expected_path = conformance.join(expected_ir);
    let expected_src = fs::read_to_string(&expected_path)
        .map_err(|e| format!("read expected-ir {}: {e}", expected_path.display()))?;
    let expected: SchemaIrExpected = serde_json::from_str(&expected_src)
        .map_err(|e| format!("parse expected-ir {}: {e}", expected_path.display()))?;

    let mut builder = ContextBuilder::new(Default::default())?.search_path(&module_dir)?;
    let load = if expected.load.is_empty() {
        vec![expected.module.as_str()]
    } else {
        expected.load.iter().map(String::as_str).collect()
    };
    for module in load {
        builder = builder.load_module(module, None, &[])?;
    }
    let ctx = builder.build()?;
    let module = ctx.schema(&expected.module)?;

    for (path, want) in &expected.children {
        let got = child_names(schema_node_or_root(&module, path)?.children());
        assert_names(path, &got, want)?;
    }
    for (path, want) in &expected.data_children_flatten {
        let got = child_names(schema_node_or_root(&module, path)?.data_children(true));
        assert_names(path, &got, want)?;
    }
    for (path, want) in &expected.keys {
        let got = child_names(schema_node_or_root(&module, path)?.list_keys());
        assert_names(path, &got, want)?;
    }
    for (module_name, want) in &expected.imports {
        let got_module = ctx.schema(module_name)?;
        let got = got_module.imports();
        if got.len() != want.len() {
            return Err(format!("{module_name} imports = {got:?}, want {want:?}").into());
        }
        for (got, want) in got.iter().zip(want) {
            if got.prefix() != want.prefix
                || got.name() != want.name
                || got.revision() != want.revision.as_deref()
            {
                return Err(format!("{module_name} imports = {got:?}, want {want:?}").into());
            }
        }
    }
    for (module_name, checks) in &expected.prefixes {
        let got_module = ctx.schema(module_name)?;
        for (prefix, want) in checks {
            let resolved = got_module
                .resolve_prefix(prefix)
                .ok_or_else(|| format!("{module_name} prefix {prefix:?} did not resolve"))?;
            if resolved.name() != want {
                return Err(format!(
                    "{module_name} prefix {prefix:?} resolved to {}, want {want}",
                    resolved.name()
                )
                .into());
            }
        }
    }
    for (identity, want) in &expected.identity_derived {
        let (module_name, identity_name) = identity
            .split_once(':')
            .ok_or_else(|| format!("identity key {identity:?} must be module:identity"))?;
        let got_module = ctx.schema(module_name)?;
        let ident = got_module
            .identities()
            .find(|id| id.name() == identity_name)
            .ok_or_else(|| format!("identity {identity} not found"))?;
        let got: Vec<_> = ident
            .derived()
            .iter()
            .map(|id| id.name().to_string())
            .collect();
        assert_names(identity, &got, want)?;
    }
    for (path, want) in &expected.leafrefs {
        let node = schema_node_or_root(&module, path)?;
        let info = node
            .leaf_type()
            .ok_or_else(|| format!("{path} has no leaf type"))?;
        let ResolvedType::LeafRef {
            path: got_path,
            target,
            ..
        } = info.resolved()
        else {
            return Err(format!("{path} is not a leafref").into());
        };
        if got_path.as_deref() != Some(want.path.as_str()) {
            return Err(format!("{path} leafref path = {got_path:?}, want {:?}", want.path).into());
        }
        let got_target = target
            .as_ref()
            .ok_or_else(|| format!("{path} leafref target did not resolve"))?
            .path();
        if got_target != want.target {
            return Err(
                format!("{path} leafref target = {got_target}, want {}", want.target).into(),
            );
        }
    }

    println!("PASS schema-ir {}", case.name);
    Ok(())
}

fn schema_node_or_root<'a>(module: &cambium::Module<'a>, path: &str) -> Result<SchemaNodeRef<'a>> {
    module.find_path(path)
}

fn child_names<'a>(children: impl Iterator<Item = SchemaNodeRef<'a>>) -> Vec<String> {
    children.map(|child| child.name().to_string()).collect()
}

fn assert_names(label: &str, got: &[String], want: &[String]) -> Result<()> {
    if got == want {
        Ok(())
    } else {
        Err(format!("{label} = {got:?}, want {want:?}").into())
    }
}

fn run_case(conformance: &Path, case: &Case) -> Result<()> {
    let module_dir = conformance.join(&case.module);
    let input_path = case
        .input
        .as_ref()
        .ok_or_else(|| format!("case {} has no input", case.name))?;
    let input_path = conformance.join(input_path);
    let input_format = case
        .input_format
        .as_ref()
        .ok_or_else(|| format!("case {} has no input-format", case.name))?;

    let mut ctx = Context::new()?;
    ctx.set_search_path(&module_dir)?;
    load_modules_in_dir(&mut ctx, &module_dir)?;

    let input_bytes = fs::read(&input_path).map_err(|e| format!("read input: {e}"))?;
    let input_format = parse_format(input_format)?;
    let tree = if let Some(op) = &case.op_type {
        ctx.parse_op(input_format, parse_op_type(op)?, &input_bytes)?
    } else {
        ctx.parse(input_format, ParseMode::data_only(), &input_bytes)?
    };

    for (fmt_name, golden_rel) in &case.expected {
        let fmt = parse_format(fmt_name)?;
        let golden_path = conformance.join(golden_rel);
        let expected = fs::read(&golden_path)
            .map_err(|e| format!("read golden {}: {e}", golden_path.display()))?;

        let mut flags = SerializeFlags {
            siblings: true,
            ..Default::default()
        };
        if let Some(mode) = &case.serialize_defaults {
            flags.with_defaults = parse_with_defaults(mode)?;
        }
        let actual = tree.serialize(fmt, flags)?;

        if normalize(&expected) != normalize(&actual) {
            return Err(format!(
                "{} output does not match golden {}\n--- expected (first 512 bytes) ---\n{}\n--- actual (first 512 bytes) ---\n{}",
                fmt_name,
                golden_path.display(),
                String::from_utf8_lossy(&expected[..expected.len().min(512)]),
                String::from_utf8_lossy(&actual[..actual.len().min(512)])
            )
            .into());
        }

        if case.oracle
            && let Some(yanglint) = YANGLINT
        {
            let oracle = run_yanglint_oracle(
                yanglint,
                &module_dir,
                &input_path,
                fmt,
                flags.with_defaults,
                case.op_type.as_deref(),
            )?;
            if normalize(&oracle) != normalize(&actual) {
                return Err(format!(
                    "{} output differs from yanglint oracle\n--- yanglint (first 512 bytes) ---\n{}\n--- cambium (first 512 bytes) ---\n{}",
                    fmt_name,
                    String::from_utf8_lossy(&oracle[..oracle.len().min(512)]),
                    String::from_utf8_lossy(&actual[..actual.len().min(512)])
                )
                .into());
            }
        }
    }

    println!("PASS {}", case.name);
    Ok(())
}

fn load_modules_in_dir(ctx: &mut Context, dir: &Path) -> Result<()> {
    let mut entries: Vec<_> = fs::read_dir(dir)
        .map_err(|e| format!("read module dir: {e}"))?
        .filter_map(|e| e.ok())
        .collect();
    entries.sort_by_key(|e| e.file_name());

    for entry in entries {
        let path = entry.path();
        if path.extension() != Some(OsStr::new("yang")) {
            continue;
        }
        // Skip submodule files; they are resolved via include from their main module.
        if is_submodule(&path) {
            continue;
        }
        let stem = path
            .file_stem()
            .and_then(|s| s.to_str())
            .ok_or_else(|| format!("invalid module file name: {}", path.display()))?;
        // Strip a revision suffix such as `ietf-inet-types@2021-02-22`.
        let name = stem.split('@').next().unwrap_or(stem);
        ctx.load_module(name)?;
    }
    Ok(())
}

fn is_submodule(path: &Path) -> bool {
    let Ok(text) = fs::read_to_string(path) else {
        return false;
    };
    text.trim_start().starts_with("submodule ")
}

fn run_yanglint_oracle(
    yanglint: &str,
    module_dir: &Path,
    input: &Path,
    fmt: Format,
    wd: WithDefaults,
    op_type: Option<&str>,
) -> Result<Vec<u8>> {
    let mut schemas: Vec<PathBuf> = fs::read_dir(module_dir)
        .map_err(|e| format!("read module dir: {e}"))?
        .filter_map(|e| e.ok().map(|e| e.path()))
        .filter(|p| p.extension() == Some(OsStr::new("yang")) && !is_submodule(p))
        .collect();
    schemas.sort();

    let format_arg = match fmt {
        Format::Xml => "xml",
        Format::Json | Format::JsonIetf => "json",
        _ => return Err(format!("unsupported oracle format: {fmt:?}").into()),
    };

    let wd_arg = match wd {
        WithDefaults::Explicit => None,
        WithDefaults::Trim => Some("trim"),
        WithDefaults::All => Some("all"),
        WithDefaults::AllTagged => Some("all-tagged"),
        _ => None,
    };

    // Mirror the generator (scripts/conformance_lib.py run_yanglint) EXACTLY so
    // the oracle's bytes can match Cambium's:
    //   yanglint -X -p <dir> [-d wd] [-t rpc|notif|reply] -f <fmt> -F <mod>: ... <schemas> <input>
    let mut cmd = Command::new(yanglint);
    cmd.arg("-X").arg("-p").arg(module_dir);
    if let Some(mode) = wd_arg {
        cmd.arg("-d").arg(mode);
    }
    if let Some(t) = op_type {
        let yt = match t {
            "rpc" => "rpc",
            "reply" => "reply",
            "notification" => "notif",
            other => return Err(format!("unknown op-type {other:?} for yanglint oracle").into()),
        };
        cmd.arg("-t").arg(yt);
    }
    cmd.arg("-f").arg(format_arg);
    // Disable all features per module to match Cambium's load (no features
    // enabled), as the generator does.
    for s in &schemas {
        let stem = s.file_stem().and_then(|x| x.to_str()).unwrap_or_default();
        let name = stem.split('@').next().unwrap_or(stem);
        cmd.arg("-F").arg(format!("{name}:"));
    }
    for s in &schemas {
        cmd.arg(s);
    }
    cmd.arg(input);

    let output = cmd.output().map_err(|e| format!("yanglint: {e}"))?;
    if !output.status.success() {
        return Err(format!(
            "yanglint failed: {}",
            String::from_utf8_lossy(&output.stderr)
        )
        .into());
    }
    Ok(output.stdout)
}

fn parse_format(s: &str) -> Result<Format> {
    match s.to_lowercase().as_str() {
        "xml" => Ok(Format::Xml),
        "json" => Ok(Format::Json),
        "json-ietf" | "json_ietf" => Ok(Format::JsonIetf),
        other => Err(format!("unknown format: {other}").into()),
    }
}

fn parse_with_defaults(s: &str) -> Result<WithDefaults> {
    match s.to_lowercase().as_str() {
        "explicit" => Ok(WithDefaults::Explicit),
        "trim" => Ok(WithDefaults::Trim),
        "all" | "report-all" => Ok(WithDefaults::All),
        "all-tagged" | "report-all-tagged" => Ok(WithDefaults::AllTagged),
        other => Err(format!("unknown serialize-defaults: {other}").into()),
    }
}

fn parse_op_type(s: &str) -> Result<OpType> {
    match s.to_lowercase().as_str() {
        "rpc" => Ok(OpType::Rpc),
        "notification" => Ok(OpType::Notification),
        "reply" => Ok(OpType::Reply),
        other => Err(format!("unknown op-type: {other}").into()),
    }
}

fn normalize(bytes: &[u8]) -> Vec<u8> {
    // Ignore trailing whitespace/newline differences between golden files and
    // printer output.
    let mut v = bytes.to_vec();
    while v.last().map(|b| b.is_ascii_whitespace()).unwrap_or(false) {
        v.pop();
    }
    v
}

fn project_root() -> Result<PathBuf> {
    // The runner may be executed from the workspace root, a package directory,
    // or `target/debug/deps`. Walk up until we find the workspace `Cargo.toml`
    // (the one that declares `[workspace]`).
    let mut dir = std::env::current_dir().map_err(|e| format!("current dir: {e}"))?;
    loop {
        let cargo_toml = dir.join("Cargo.toml");
        if cargo_toml.exists()
            && let Ok(contents) = fs::read_to_string(&cargo_toml)
            && contents.contains("[workspace]")
        {
            return Ok(dir);
        }
        if !dir.pop() {
            // Fallback: assume the executable lives under target/ in the project.
            let mut fallback = PathBuf::from(std::env!("CARGO_MANIFEST_DIR"));
            while fallback.join("Cargo.toml").exists() {
                if let Ok(contents) = fs::read_to_string(fallback.join("Cargo.toml"))
                    && contents.contains("[workspace]")
                {
                    return Ok(fallback);
                }
                if !fallback.pop() {
                    break;
                }
            }
            return Err("could not locate workspace root".to_string().into());
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn invalid_tier_rejected() {
        let case = Case {
            name: "bad-tier".to_string(),
            tier: Some("schema-irx".to_string()),
            module: PathBuf::new(),
            input: None,
            input_format: None,
            op_type: None,
            serialize_defaults: None,
            expected: HashMap::new(),
            expected_ir: None,
            oracle: false,
        };

        let err = match case.validate_tier() {
            Err(e) => e.to_string(),
            Ok(()) => panic!("expected invalid tier to be rejected"),
        };
        assert!(
            err.contains("bad-tier"),
            "error should contain case name: {err}"
        );
        assert!(
            err.contains("schema-irx"),
            "error should contain tier value: {err}"
        );
    }

    #[test]
    fn valid_tiers_accepted() {
        for tier in [
            None,
            Some("backend-data".to_string()),
            Some("schema-ir".to_string()),
        ] {
            let case = Case {
                name: "ok".to_string(),
                tier,
                module: PathBuf::new(),
                input: None,
                input_format: None,
                op_type: None,
                serialize_defaults: None,
                expected: HashMap::new(),
                expected_ir: None,
                oracle: false,
            };
            assert!(
                case.validate_tier().is_ok(),
                "valid tier should be accepted"
            );
        }
    }
}
