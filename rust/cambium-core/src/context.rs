//! Safe, owned YANG context.

use std::path::Path;

use cambium_libyang_sys::adapter::{RawContext, RawFormat};

use crate::Module;
use crate::error::{Error, Result, RuleCode};
use crate::schema::{SchemaForest, SchemaTree};
use crate::tree::{DataTree, Format, OpType, ParseMode};

/// Flags controlling context creation.
#[derive(Clone, Copy, Debug, Default)]
pub struct ContextFlags {
    /// Do not load `ietf-yang-library` automatically.
    pub no_yang_library: bool,
    /// Implement all imported modules.
    pub all_implemented: bool,
    /// Implement modules referenced by `reference`.
    pub ref_implemented: bool,
    /// Do not automatically search the current working directory.
    pub disable_searchdir_cwd: bool,
}

impl ContextFlags {
    fn to_raw(self) -> u32 {
        let mut flags = 0u32;
        // Enable deref() in leafref paths (YANG 1.1, RFC 7950 §10.3.1).
        flags |= cambium_libyang_sys::LY_CTX_LEAFREF_EXTENDED;
        // Keep a parsed-node pointer on compiled schema nodes so the adapter can
        // expose provenance such as grouping origin without leaking libyang.
        flags |= cambium_libyang_sys::LY_CTX_SET_PRIV_PARSED;
        if self.no_yang_library {
            flags |= cambium_libyang_sys::LY_CTX_NO_YANGLIBRARY;
        }
        if self.all_implemented {
            flags |= cambium_libyang_sys::LY_CTX_ALL_IMPLEMENTED;
        }
        if self.ref_implemented {
            flags |= cambium_libyang_sys::LY_CTX_REF_IMPLEMENTED;
        }
        if self.disable_searchdir_cwd {
            flags |= cambium_libyang_sys::LY_CTX_DISABLE_SEARCHDIR_CWD;
        }
        flags
    }
}

/// Mutable builder for a frozen `Context`.
///
/// Only the builder may load modules, set search paths, or toggle features.
/// `build()` consumes the builder and returns an immutable, `Send + Sync`
/// `Context`.
#[derive(Debug)]
pub struct ContextBuilder {
    raw: RawContext,
}

impl ContextBuilder {
    /// Create a new context builder.
    pub fn new(flags: ContextFlags) -> Result<Self> {
        let raw = RawContext::new(flags.to_raw()).map_err(|e| Error::ffi(RuleCode::Context, e))?;
        Ok(Self { raw })
    }

    /// Append a directory to the module search path.
    pub fn search_path<P: AsRef<Path>>(mut self, path: P) -> Result<Self> {
        self.raw
            .set_search_path(path)
            .map_err(|e| Error::ffi(RuleCode::Context, e))?;
        Ok(self)
    }

    /// Load a YANG module into the context.
    pub fn load_module(
        mut self,
        name: &str,
        revision: Option<&str>,
        features: &[&str],
    ) -> Result<Self> {
        self.raw
            .load_module(name, revision, features)
            .map_err(|e| Error::ffi(RuleCode::Context, e))?;
        Ok(self)
    }

    /// Load a YANG module from a filesystem path.
    pub fn load_module_path<P: AsRef<Path>>(mut self, path: P) -> Result<Self> {
        self.raw
            .load_module_path(path)
            .map_err(|e| Error::ffi(RuleCode::Context, e))?;
        Ok(self)
    }

    /// Load a YANG module from an in-memory source string.
    pub fn load_module_str(mut self, source: &str) -> Result<Self> {
        self.raw
            .load_module_str(source)
            .map_err(|e| Error::ffi(RuleCode::Context, e))?;
        Ok(self)
    }

    /// Consume the builder and produce a frozen `Context`.
    pub fn build(mut self) -> Result<Context> {
        let forest = Self::compile_forest(&mut self.raw)?;
        Ok(Context {
            raw: self.raw,
            forest,
        })
    }

    fn compile_forest(raw: &mut RawContext) -> Result<SchemaForest> {
        let mut forest = SchemaForest::default();
        for (info, root) in raw.schema_modules() {
            forest.add_module(info, root);
        }
        Ok(forest)
    }
}

/// A compiled YANG context.
///
/// The context is build-once-then-frozen in spirit: callers create it, set
/// search paths, load modules, and then use it to parse data trees. Mutating
/// the context after data trees have been created against it is not supported.
#[derive(Debug)]
pub struct Context {
    raw: RawContext,
    pub(crate) forest: SchemaForest,
}

impl Context {
    /// Create a new empty context (deprecated; prefer `ContextBuilder`).
    pub fn new() -> Result<Self> {
        let raw = RawContext::new(
            cambium_libyang_sys::LY_CTX_NO_YANGLIBRARY
                | cambium_libyang_sys::LY_CTX_LEAFREF_EXTENDED
                | cambium_libyang_sys::LY_CTX_SET_PRIV_PARSED,
        )
        .map_err(|e| Error::ffi(RuleCode::Context, e))?;
        Ok(Self {
            raw,
            forest: SchemaForest::default(),
        })
    }

    /// Append a directory to the module search path (deprecated; prefer
    /// `ContextBuilder::search_path`).
    pub fn set_search_path<P: AsRef<Path>>(&mut self, path: P) -> Result<()> {
        self.raw
            .set_search_path(path)
            .map_err(|e| Error::ffi(RuleCode::Context, e))
    }

    /// Load a YANG module into the context (deprecated; prefer
    /// `ContextBuilder::load_module`).
    pub fn load_module(&mut self, name: &str) -> Result<()> {
        self.raw
            .load_module(name, None, &[])
            .map_err(|e| Error::ffi(RuleCode::Context, e))?;
        self.forest = ContextBuilder::compile_forest(&mut self.raw)?;
        Ok(())
    }

    /// Parse a whole data document against this context.
    pub fn parse(&self, format: Format, mode: ParseMode, data: &[u8]) -> Result<DataTree<'_>> {
        if mode.strict && mode.opaque {
            return Err(Error::ffi(
                RuleCode::Parse,
                "strict and opaque parse modes are mutually exclusive",
            ));
        }
        let raw = self
            .raw
            .parse_data(format.into(), mode.into(), data)
            .map_err(|e| Error::ffi(RuleCode::Parse, e))?;
        Ok(DataTree::with_raw(self, raw))
    }

    /// Create an empty in-memory data tree against this context.
    pub fn new_data(&self) -> DataTree<'_> {
        DataTree::with_raw(self, self.raw.new_data())
    }

    /// Parse an RPC, action, or notification against this context.
    pub fn parse_op(&self, format: Format, op_type: OpType, data: &[u8]) -> Result<DataTree<'_>> {
        let raw = self
            .raw
            .parse_op(format.into(), op_type.into(), data)
            .map_err(|e| Error::ffi(RuleCode::Parse, e))?;
        Ok(DataTree::with_raw(self, raw))
    }

    /// Return a borrowed module handle.
    pub fn schema(&self, module: &str) -> Result<Module<'_>> {
        let idx = self
            .forest
            .module_index(module)
            .ok_or_else(|| Error::ffi(RuleCode::Context, format!("module not found: {module}")))?;
        Ok(Module {
            forest: &self.forest,
            module_index: idx,
        })
    }

    /// Return borrowed handles for every implemented module in this context.
    pub fn modules(&self) -> Vec<Module<'_>> {
        self.forest.modules()
    }

    /// Return the compiled schema tree for a loaded module (deprecated; prefer
    /// `Context::schema`).
    pub fn schema_tree(&self, module: &str) -> Result<SchemaTree> {
        let raw = self
            .raw
            .schema_tree(module)
            .map_err(|e| Error::ffi(RuleCode::Context, e))?;
        Ok(SchemaTree::from_raw(raw))
    }
}

impl From<RawFormat> for Format {
    fn from(f: RawFormat) -> Self {
        match f {
            RawFormat::Xml => Format::Xml,
            RawFormat::Json => Format::Json,
            RawFormat::JsonIetf => Format::JsonIetf,
            RawFormat::Lyb => Format::Lyb,
        }
    }
}
