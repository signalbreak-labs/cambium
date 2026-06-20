//! Ordered data tree types and parse/serialize operations.

use cambium_libyang_sys::adapter::{
    RawChildInfo, RawDataDiff, RawDataTree, RawDiffEdit, RawDiffOp, RawFormat,
};

use crate::context::Context;
use crate::error::{
    Diagnostic, Error, ErrorType, Result, RuleCode, ValidationCode, ValidationErrors,
};
use crate::list::{UserOrderedLeafList, UserOrderedList, UserOrderedView};
use crate::schema::{BaseType, OrderedBy, ResolvedType, SchemaNodeRef, TypeInfo};

/// On-wire formats. Every format is produced by a single ordered walk of the
/// libyang sibling chain — never a language-native map/struct serializer.
#[derive(Clone, Copy, Debug, PartialEq, Eq)]
#[non_exhaustive]
pub enum Format {
    /// XML encoding (RFC 7950 canonical).
    Xml,
    /// RFC 7951 JSON encoding.
    Json,
    /// RFC 7951 JSON encoding as used by gNMI JSON_IETF.
    ///
    /// Empty non-presence containers are preserved because gNMI treats them as
    /// intentional configuration subtrees.
    JsonIetf,
    /// Binary LYB format.
    Lyb,
}

/// How to handle default-valued nodes during serialization.
#[derive(Clone, Copy, Debug, Default, PartialEq, Eq)]
#[non_exhaustive]
pub enum WithDefaults {
    /// Print nodes exactly as they appear in the tree; default nodes are only
    /// printed when explicitly present (`LYD_PRINT_WD_EXPLICIT`, value 0).
    #[default]
    Explicit,
    /// Suppress nodes whose value equals their schema default.
    Trim,
    /// Print every default node, materialized or not (combine with
    /// `DataTree::add_defaults` to see them).
    All,
    /// Print all defaults and tag them with `ietf-netconf-with-defaults` metadata.
    AllTagged,
}

/// Serialization flags.
#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub struct SerializeFlags {
    /// Include all siblings of the root (`LYD_PRINT_SIBLINGS`).
    pub siblings: bool,
    /// Remove insignificant whitespace from XML/JSON output
    /// (`LYD_PRINT_SHRINK`).
    pub shrink: bool,
    /// Preserve empty non-presence containers (`LYD_PRINT_EMPTY_CONT`).
    pub keep_empty_containers: bool,
    /// How to handle default-valued nodes.
    pub with_defaults: WithDefaults,
}

impl Default for SerializeFlags {
    fn default() -> Self {
        Self {
            siblings: true,
            shrink: false,
            keep_empty_containers: false,
            with_defaults: WithDefaults::Explicit,
        }
    }
}

/// Parse behaviour, mapping libyang's separable parse-option bitmap.
#[derive(Clone, Copy, Debug, Default, PartialEq, Eq)]
pub struct ParseMode {
    /// Reject unknown data nodes (`LYD_PARSE_STRICT`).
    pub strict: bool,
    /// Parse unknown data as opaque nodes (`LYD_PARSE_OPAQ`).
    pub opaque: bool,
    /// Parse only; do not validate (`LYD_PARSE_ONLY`).
    pub parse_only: bool,
    /// Ignore state data during parsing (`LYD_PARSE_NO_STATE`).
    pub no_state: bool,
    /// Skip LYB module revision checks when parsing LYB.
    ///
    /// Maps to `LYD_PARSE_LYB_SKIP_MODULE_CHECK`; the design-doc name
    /// `lyb_mod_update` does not exist as a libyang flag in this pinned version.
    pub lyb_mod_update: bool,
}

impl ParseMode {
    /// Shorthand for the old `ParseMode::DataOnly`: parse without validating.
    pub fn data_only() -> Self {
        Self {
            parse_only: true,
            ..Default::default()
        }
    }
}

/// Options for `DataTree::diff`.
#[derive(Clone, Copy, Debug, Default, PartialEq, Eq)]
pub struct DiffOpts {
    /// Include default nodes in the diff (`LYD_DIFF_DEFAULTS`).
    pub defaults: bool,
}

/// Options for `DataTree::merge`.
#[derive(Clone, Copy, Debug, Default, PartialEq, Eq)]
pub struct MergeOpts {
    /// Currently inert: `source` is always borrowed and left unmodified, so there
    /// is nothing to destruct. Reserved for a future by-value (consuming) merge.
    pub destruct: bool,
}

/// Operation on a diff edit.
#[derive(Clone, Copy, Debug, PartialEq, Eq)]
#[non_exhaustive]
pub enum DiffOp {
    /// Create a node.
    Create,
    /// Delete a node.
    Delete,
    /// Replace a leaf/leaf-list value.
    Replace,
    /// No operation (excluded from `DiffEdit` iteration).
    None,
}

/// An owned, apply-safe diff between two data trees.
pub struct DataDiff<'ctx> {
    raw: Option<RawDataDiff>,
    edits: Vec<RawDiffEdit>,
    _ctx: std::marker::PhantomData<&'ctx ()>,
}

impl<'ctx> DataDiff<'ctx> {
    fn empty() -> Self {
        Self {
            raw: None,
            edits: Vec::new(),
            _ctx: std::marker::PhantomData,
        }
    }

    /// True if the diff contains no edits.
    pub fn is_empty(&self) -> bool {
        self.edits.is_empty()
    }

    /// Iterate over the diff edits in apply-safe document order.
    pub fn edits(&self) -> impl Iterator<Item = DiffEdit<'_>> {
        self.edits.iter().map(DiffEdit)
    }

    /// Serialize the yang-patch-shaped diff tree.
    pub fn serialize(&self, format: Format) -> Result<Vec<u8>> {
        let options = crate::sys_consts::LYD_PRINT_SIBLINGS;
        match &self.raw {
            Some(raw) => raw
                .serialize(format.into(), options)
                .map_err(|e| Error::ffi(RuleCode::Serialize, e)),
            None => Ok(Vec::new()),
        }
    }
}

/// A borrowed view of one diff edit.
#[derive(Debug)]
pub struct DiffEdit<'d>(&'d RawDiffEdit);

impl<'d> DiffEdit<'d> {
    /// The operation to apply.
    pub fn op(&self) -> DiffOp {
        match self.0.op {
            RawDiffOp::Create => DiffOp::Create,
            RawDiffOp::Delete => DiffOp::Delete,
            RawDiffOp::Replace => DiffOp::Replace,
            RawDiffOp::None => DiffOp::None,
        }
    }

    /// Absolute data path of the edited node.
    pub fn path(&self) -> &str {
        &self.0.path
    }

    /// Canonical value for leaf/leaf-list edits, if any.
    pub fn value(&self) -> Option<&str> {
        self.0.value.as_deref()
    }

    /// True for edits on an `ordered-by user` list/leaf-list.
    pub fn is_ordered_by_user(&self) -> bool {
        self.0.is_user_ordered
    }
}

/// Operation type for parsing RPCs, actions, and notifications.
#[derive(Clone, Copy, Debug, PartialEq, Eq)]
#[non_exhaustive]
pub enum OpType {
    /// YANG RPC.
    Rpc,
    /// YANG notification.
    Notification,
    /// YANG RPC/action reply.
    Reply,
}

/// Validation behaviour, mapping libyang's validation-option bitmap.
#[derive(Clone, Copy, Debug, Default, PartialEq, Eq)]
pub struct ValidateMode {
    /// Ignore state data during validation (`LYD_VALIDATE_NO_STATE`).
    pub no_state: bool,
    /// Validate only nodes that exist in the data tree (`LYD_VALIDATE_PRESENT`).
    pub present: bool,
    /// Report all validation errors, not just the first (`LYD_VALIDATE_MULTI_ERROR`).
    pub multi_error: bool,
}

/// Options for `DataTree::new_path`.
#[derive(Clone, Copy, Debug, Default, PartialEq, Eq)]
pub struct NewPathOpts {
    /// Update an existing leaf instead of returning `E0006` (`LYD_NEW_PATH_UPDATE`).
    pub update: bool,
    /// Create output nodes for an RPC/action (`LYD_NEW_VAL_OUTPUT`).
    pub output: bool,
    /// Allow creation of opaque nodes (`LYD_NEW_PATH_OPAQ`).
    pub opaque: bool,
}

/// Options for `DataTree::add_defaults`.
#[derive(Clone, Copy, Debug, Default, PartialEq, Eq)]
pub struct ImplicitOpts {
    /// Do not add implicit state nodes (`LYD_IMPLICIT_NO_STATE`).
    pub no_state: bool,
    /// Add output implicit nodes for RPC/action nodes (`LYD_IMPLICIT_OUTPUT`).
    pub output: bool,
}

/// An opaque domain key returned by `DataTree::new_path`.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct NodeAddr {
    path: String,
}

impl NodeAddr {
    /// The absolute data path of the created/updated node.
    pub fn path(&self) -> &str {
        &self.path
    }
}

/// A YANG data tree.
///
/// Child order is the source of truth; any keyed index is a derived lookup
/// cache and must never be iterated for serialization.
#[derive(Debug)]
pub struct DataTree<'ctx> {
    pub(crate) raw: RawDataTree,
    ctx: &'ctx Context,
}

impl<'ctx> DataTree<'ctx> {
    pub(crate) fn with_raw(ctx: &'ctx Context, raw: RawDataTree) -> Self {
        Self { raw, ctx }
    }

    /// Obtain a positional handle to an `ordered-by user` list at `path`.
    pub fn user_ordered_list_at(&mut self, path: &str) -> Result<UserOrderedList<'_, 'ctx>> {
        let list = self
            .raw
            .find_path(path)
            .map_err(|e| Error::ffi(RuleCode::OrderedList, e))?;
        if let Some(schema) = self.schema_ptr_at(path).ok().flatten()
            && !matches!(schema.ordered_by(), OrderedBy::User)
        {
            return Err(Error::ffi(
                RuleCode::OrderedList,
                "not an ordered-by-user list",
            ));
        }
        Ok(unsafe { UserOrderedList::from_raw(self, list) })
    }

    /// Obtain a positional handle to an `ordered-by user` leaf-list at `path`.
    pub fn user_ordered_leaf_list_at(
        &mut self,
        path: &str,
    ) -> Result<UserOrderedLeafList<'_, 'ctx>> {
        let list = self
            .raw
            .find_path(path)
            .map_err(|e| Error::ffi(RuleCode::OrderedList, e))?;
        if let Some(schema) = self.schema_ptr_at(path).ok().flatten()
            && !matches!(schema.ordered_by(), OrderedBy::User)
        {
            return Err(Error::ffi(
                RuleCode::OrderedList,
                "not an ordered-by-user leaf-list",
            ));
        }
        Ok(unsafe { UserOrderedLeafList::from_raw(self, list) })
    }

    fn schema_ptr_at(&self, path: &str) -> Result<Option<SchemaNodeRef<'_>>> {
        let ptr = self
            .raw
            .schema_ptr(path)
            .map_err(|e| Error::ffi(RuleCode::DataPath, e))?;
        Ok(match ptr {
            Some(p) => self.ctx.forest.schema_ref_by_ptr(p),
            None => None,
        })
    }

    /// Validate the data tree.
    pub fn validate(&mut self, mode: ValidateMode) -> Result<()> {
        let mut options = 0;
        if mode.no_state {
            options |= crate::sys_consts::LYD_VALIDATE_NO_STATE;
        }
        if mode.present {
            options |= crate::sys_consts::LYD_VALIDATE_PRESENT;
        }
        if mode.multi_error {
            options |= crate::sys_consts::LYD_VALIDATE_MULTI_ERROR;
        }
        self.raw.validate(options).map_err(|e| {
            Error::Validation(ValidationErrors::new(
                e.0.into_iter().map(diagnostic_from_raw).collect(),
            ))
        })
    }

    /// Create a deep, independent copy of the tree.
    pub fn duplicate(&self) -> Result<DataTree<'ctx>> {
        let raw = self
            .raw
            .duplicate()
            .map_err(|e| Error::ffi(RuleCode::Serialize, e))?;
        Ok(DataTree::with_raw(self.ctx, raw))
    }

    /// Compute the diff from `self` to `other`.
    ///
    /// The returned `DataDiff` owns a yang-patch-shaped tree and exposes
    /// `is_ordered_by_user()` so a consumer can carry user-ordered changes
    /// atomically (I6).
    pub fn diff(&self, other: &DataTree<'ctx>, opts: DiffOpts) -> Result<DataDiff<'ctx>> {
        if !std::ptr::eq(self.ctx, other.ctx) {
            return Err(Error::ffi(
                RuleCode::DataPath,
                "diff requires both trees to share the same context",
            ));
        }
        let raw_diff = self
            .raw
            .diff(&other.raw, opts.defaults)
            .map_err(|e| Error::ffi(RuleCode::DataPath, e))?;
        match raw_diff {
            Some(raw) => {
                let edits = raw.edits().map_err(|e| Error::ffi(RuleCode::DataPath, e))?;
                Ok(DataDiff {
                    raw: Some(raw),
                    edits,
                    _ctx: std::marker::PhantomData,
                })
            }
            None => Ok(DataDiff::empty()),
        }
    }

    /// Apply a diff to `self` in place.
    pub fn diff_apply(&mut self, diff: &DataDiff<'ctx>) -> Result<()> {
        match &diff.raw {
            Some(raw) => self
                .raw
                .diff_apply(raw)
                .map_err(|e| Error::ffi(RuleCode::DataPath, e)),
            None => Ok(()),
        }
    }

    /// Merge `source` into `self` in place.
    ///
    /// A leaf that exists in both trees with a different value is rejected with
    /// `RuleCode::Validate` before any mutation (ygot semantics).
    pub fn merge(&mut self, source: &DataTree<'ctx>, opts: MergeOpts) -> Result<()> {
        if !std::ptr::eq(self.ctx, source.ctx) {
            return Err(Error::ffi(
                RuleCode::DataPath,
                "merge requires both trees to share the same context",
            ));
        }

        // Conflict pre-scan: any leaf value present in both trees but differing
        // is a Cambium error, not a silent libyang overwrite.
        let conflict_diff = self.diff(source, DiffOpts::default())?;
        for edit in conflict_diff.edits() {
            if edit.op() == DiffOp::Replace
                && let Some(src_val) = edit.value()
            {
                let base_opt = self
                    .try_get(edit.path())
                    .map(|n| n.value_str())
                    .transpose()?;
                if let Some(Some(base_val)) = base_opt
                    && base_val != src_val
                {
                    return Err(Error::ffi(
                        RuleCode::Validate,
                        format!("merge conflict at {}", edit.path()),
                    ));
                }
            }
        }

        let _ = opts;
        self.raw
            .merge(&source.raw)
            .map_err(|e| Error::ffi(RuleCode::DataPath, e))
    }

    /// Serialize the tree to bytes in the requested format.
    pub fn serialize(&self, format: Format, flags: SerializeFlags) -> Result<Vec<u8>> {
        let mut options: u32 = 0;
        if flags.siblings {
            options |= crate::sys_consts::LYD_PRINT_SIBLINGS;
        }
        if flags.shrink {
            options |= crate::sys_consts::LYD_PRINT_SHRINK;
        }
        if flags.keep_empty_containers {
            options |= crate::sys_consts::LYD_PRINT_EMPTY_CONT;
        }
        options |= match flags.with_defaults {
            WithDefaults::Explicit => crate::sys_consts::LYD_PRINT_WD_EXPLICIT,
            WithDefaults::Trim => crate::sys_consts::LYD_PRINT_WD_TRIM,
            WithDefaults::All => crate::sys_consts::LYD_PRINT_WD_ALL,
            WithDefaults::AllTagged => crate::sys_consts::LYD_PRINT_WD_ALL_TAG,
        };
        if matches!(format, Format::Lyb) {
            self.raw
                .serialize_lyb(options)
                .map_err(|e| Error::ffi(RuleCode::Serialize, e))
        } else {
            self.raw
                .serialize(format.into(), options)
                .map_err(|e| Error::ffi(RuleCode::Serialize, e))
        }
    }

    /// Return a node handle for `path`, or `E0006` if it does not exist.
    pub fn get(&self, path: &str) -> Result<NodeRef<'_>> {
        if self.exists(path) {
            Ok(NodeRef {
                tree: self,
                path: path.to_string(),
            })
        } else {
            Err(Error::ffi(
                RuleCode::DataPath,
                format!("path not found: {path}"),
            ))
        }
    }

    /// Return a node handle for `path` if it exists.
    pub fn try_get(&self, path: &str) -> Option<NodeRef<'_>> {
        if self.exists(path) {
            Some(NodeRef {
                tree: self,
                path: path.to_string(),
            })
        } else {
            None
        }
    }

    /// True if `path` addresses an existing node.
    pub fn exists(&self, path: &str) -> bool {
        self.raw
            .find_node(path)
            .map(|opt| opt.is_some())
            .unwrap_or(false)
    }

    /// Evaluate an XPath over the tree and return the matched nodes in document
    /// order.
    pub fn select(&self, xpath: &str) -> Result<NodeSet<'_>> {
        let paths = self
            .raw
            .xpath_paths(xpath)
            .map_err(|e| Error::ffi(RuleCode::DataPath, e))?;
        Ok(NodeSet { tree: self, paths })
    }

    /// Return the top-level sibling chain in declaration order.
    pub fn root_nodes(&self) -> Result<DataSiblings<'_>> {
        let children = self
            .raw
            .root_nodes()
            .map_err(|e| Error::ffi(RuleCode::DataPath, e))?;
        Ok(DataSiblings::new(self, children))
    }

    /// Create or update a node at `path`.
    ///
    /// `value` is the canonical string for a leaf or leaf-list; `None` creates
    /// an inner node (container/list). The returned `NodeAddr` can be used as a
    /// stable handle even after later mutations that re-anchor the root.
    pub fn new_path(
        &mut self,
        path: &str,
        value: Option<&str>,
        opts: NewPathOpts,
    ) -> Result<NodeAddr> {
        let mut options: u32 = 0;
        if opts.update {
            options |= crate::sys_consts::LYD_NEW_PATH_UPDATE;
        }
        if opts.output {
            options |= crate::sys_consts::LYD_NEW_VAL_OUTPUT;
        }
        if opts.opaque {
            options |= crate::sys_consts::LYD_NEW_PATH_OPAQ;
        }
        self.raw
            .new_path(path, value, options)
            .map_err(|e| Error::ffi(RuleCode::DataPath, e))?;
        Ok(NodeAddr {
            path: path.to_string(),
        })
    }

    /// Change the value of an existing leaf or leaf-list.
    ///
    /// Returns `true` if the value (or default flag) changed, `false` if the
    /// value was identical.
    pub fn set_value(&mut self, path: &str, value: &str) -> Result<bool> {
        self.raw
            .set_value(path, value)
            .map_err(|e| Error::ffi(RuleCode::DataPath, e))
    }

    /// Remove and free the subtree at `path`.
    pub fn remove_path(&mut self, path: &str) -> Result<()> {
        self.raw
            .remove_path(path)
            .map_err(|e| Error::ffi(RuleCode::DataPath, e))
    }

    /// Detach the subtree at `path` and return it as an owned tree.
    ///
    /// The detached tree shares the same context and may only be re-inserted
    /// into a tree from that same context.
    pub fn unlink_path(&mut self, path: &str) -> Result<DataTree<'ctx>> {
        let raw = self
            .raw
            .unlink_path(path)
            .map_err(|e| Error::ffi(RuleCode::DataPath, e))?;
        Ok(DataTree::with_raw(self.ctx, raw))
    }

    /// Add implicit/default nodes to the tree.
    pub fn add_defaults(&mut self, opts: ImplicitOpts) -> Result<()> {
        let mut options: u32 = 0;
        if opts.no_state {
            options |= crate::sys_consts::LYD_IMPLICIT_NO_STATE;
        }
        if opts.output {
            options |= crate::sys_consts::LYD_IMPLICIT_OUTPUT;
        }
        self.raw
            .add_defaults(options)
            .map_err(|e| Error::ffi(RuleCode::DataPath, e))
    }
}

fn diagnostic_from_raw(raw: cambium_libyang_sys::adapter::RawDiagnostic) -> Diagnostic {
    let validation_code = validation_code_from_apptag(&raw);
    Diagnostic {
        code: RuleCode::Validate,
        message: raw.message,
        data_path: raw.data_path,
        schema_path: raw.schema_path,
        error_type: ErrorType::Application,
        error_app_tag: raw.apptag,
        validation_code,
    }
}

fn validation_code_from_apptag(
    raw: &cambium_libyang_sys::adapter::RawDiagnostic,
) -> Option<ValidationCode> {
    let msg = raw.message.as_str();
    match raw.apptag.as_deref() {
        Some("must-violation") => Some(ValidationCode::Must),
        Some("instance-required") => Some(ValidationCode::Leafref),
        Some("too-few-elements") | Some("too-many-elements") | Some("missing-choice") => {
            Some(ValidationCode::Mandatory)
        }
        _ if msg.starts_with("When condition") => Some(ValidationCode::When),
        _ if msg.starts_with("Mandatory node") || msg.starts_with("Mandatory choice") => {
            Some(ValidationCode::Mandatory)
        }
        _ if msg.contains("min-elements") || msg.contains("max-elements") => {
            Some(ValidationCode::Mandatory)
        }
        _ => raw.vecode_str.as_deref().and_then(|s| {
            if s.eq_ignore_ascii_case("data") {
                Some(ValidationCode::InvalidValue)
            } else {
                None
            }
        }),
    }
}

impl From<Format> for RawFormat {
    fn from(f: Format) -> Self {
        match f {
            Format::Xml => RawFormat::Xml,
            Format::Json => RawFormat::Json,
            Format::JsonIetf => RawFormat::JsonIetf,
            Format::Lyb => RawFormat::Lyb,
        }
    }
}

impl From<OpType> for cambium_libyang_sys::adapter::RawOpType {
    fn from(t: OpType) -> Self {
        match t {
            OpType::Rpc => Self::RpcYang,
            OpType::Notification => Self::NotifYang,
            OpType::Reply => Self::ReplyYang,
        }
    }
}

impl From<ParseMode> for u32 {
    fn from(m: ParseMode) -> Self {
        let mut options: u32 = 0;
        if m.strict {
            options |= crate::sys_consts::LYD_PARSE_STRICT;
        }
        if m.opaque {
            options |= crate::sys_consts::LYD_PARSE_OPAQ;
        }
        if m.parse_only {
            options |= crate::sys_consts::LYD_PARSE_ONLY;
        }
        if m.no_state {
            options |= crate::sys_consts::LYD_PARSE_NO_STATE;
        }
        if m.lyb_mod_update {
            options |= crate::sys_consts::LYD_PARSE_LYB_SKIP_MODULE_CHECK;
        }
        options
    }
}

/// A borrowed handle to one node in a `DataTree`.
#[derive(Debug, Clone)]
pub struct NodeRef<'tree> {
    tree: &'tree DataTree<'tree>,
    path: String,
}

impl<'tree> NodeRef<'tree> {
    pub(crate) fn new(tree: &'tree DataTree<'tree>, path: String) -> Self {
        Self { tree, path }
    }

    /// If this node is an `ordered-by user` list, return a **read-only**
    /// positional view (`len`/`get`/`iter`/`find_by_key`).
    ///
    /// Returns `None` for system-ordered lists. Reordering requires a
    /// `&mut DataTree` handle via [`DataTree::user_ordered_list_at`]; a shared
    /// `NodeRef` deliberately cannot mutate the tree behind its borrow.
    pub fn as_user_ordered(self) -> Result<Option<UserOrderedView<'tree, 'tree>>> {
        let schema = self.schema()?;
        if !matches!(schema.ordered_by(), OrderedBy::User) {
            return Ok(None);
        }
        let list_ptr = self
            .tree
            .raw
            .find_path(&self.path)
            .map_err(|e| Error::ffi(RuleCode::OrderedList, e))?;
        // SAFETY: `list_ptr` addresses a user-ordered list owned by `self.tree`;
        // the view borrows `self.tree` immutably and exposes no mutators, so it
        // cannot alias a `&mut` borrow of the tree.
        Ok(Some(unsafe {
            UserOrderedView::from_raw(self.tree, list_ptr)
        }))
    }

    /// Canonical data path of this node.
    pub fn path(&self) -> &str {
        &self.path
    }

    /// Schema-local node name.
    pub fn name(&self) -> &str {
        path_node_name(&self.path)
    }

    /// Canonical value string, if this is a leaf or leaf-list.
    pub fn value_str(&self) -> Result<Option<String>> {
        self.tree
            .raw
            .value_str(&self.path)
            .map_err(|e| Error::ffi(RuleCode::DataPath, e))
    }

    /// Typed value, if this is a leaf or leaf-list.
    pub fn value(&self) -> Result<Option<Value>> {
        let s = match self.value_str()? {
            Some(s) => s,
            None => return Ok(None),
        };
        let schema = self.schema()?;
        let type_info = schema
            .leaf_type()
            .ok_or_else(|| Error::ffi(RuleCode::DataPath, "node has no leaf type"))?;
        Ok(Some(parse_value(&s, &type_info)?))
    }

    /// True if this node was created from a default value.
    pub fn is_default(&self) -> Result<bool> {
        self.tree
            .raw
            .is_default(&self.path)
            .map_err(|e| Error::ffi(RuleCode::DataPath, e))
    }

    /// Bridge to the compiled schema node.
    pub fn schema(&self) -> Result<SchemaNodeRef<'tree>> {
        let ptr = self
            .tree
            .raw
            .schema_ptr(&self.path)
            .map_err(|e| Error::ffi(RuleCode::DataPath, e))?
            .ok_or_else(|| Error::ffi(RuleCode::DataPath, "path not found"))?;
        self.tree
            .ctx
            .forest
            .schema_ref_by_ptr(ptr)
            .ok_or_else(|| Error::ffi(RuleCode::DataPath, "schema not in forest"))
    }

    /// Immediate children in declaration order.
    pub fn children(&self) -> Result<DataSiblings<'tree>> {
        let children = self
            .tree
            .raw
            .children_of(&self.path)
            .map_err(|e| Error::ffi(RuleCode::DataPath, e))?;
        Ok(DataSiblings::new(self.tree, children))
    }

    /// All siblings of this node (including this node) in declaration order.
    pub fn siblings(&self) -> Result<DataSiblings<'tree>> {
        let children = self
            .tree
            .raw
            .siblings_of(&self.path)
            .map_err(|e| Error::ffi(RuleCode::DataPath, e))?;
        Ok(DataSiblings::new(self.tree, children))
    }
}

/// An ordered set of nodes returned by an XPath selection.
#[derive(Debug, Clone)]
pub struct NodeSet<'tree> {
    tree: &'tree DataTree<'tree>,
    paths: Vec<String>,
}

impl<'tree> NodeSet<'tree> {
    /// Number of matched nodes.
    pub fn len(&self) -> usize {
        self.paths.len()
    }

    /// True if the selection is empty.
    pub fn is_empty(&self) -> bool {
        self.paths.is_empty()
    }

    /// Iterate in document order.
    pub fn iter(&self) -> impl Iterator<Item = NodeRef<'tree>> {
        self.paths.iter().cloned().map(|path| NodeRef {
            tree: self.tree,
            path,
        })
    }

    /// Get the node at `index`.
    pub fn get(&self, index: usize) -> Option<NodeRef<'tree>> {
        self.paths.get(index).cloned().map(|path| NodeRef {
            tree: self.tree,
            path,
        })
    }
}

/// Iterator over materialized data-node siblings in declaration order.
#[derive(Debug, Clone)]
pub struct DataSiblings<'tree> {
    tree: &'tree DataTree<'tree>,
    children: Vec<RawChildInfo>,
    pos: usize,
}

impl<'tree> DataSiblings<'tree> {
    fn new(tree: &'tree DataTree<'tree>, children: Vec<RawChildInfo>) -> Self {
        Self {
            tree,
            children,
            pos: 0,
        }
    }

    /// Number of nodes.
    pub fn len(&self) -> usize {
        self.children.len()
    }

    /// True if there are no nodes.
    pub fn is_empty(&self) -> bool {
        self.children.is_empty()
    }

    /// Get the node at `index`.
    pub fn get(&self, index: usize) -> Option<NodeRef<'tree>> {
        self.children.get(index).map(|c| NodeRef {
            tree: self.tree,
            path: c.path.clone(),
        })
    }

    /// Iterate in declaration order.
    pub fn iter(&self) -> impl Iterator<Item = NodeRef<'tree>> {
        self.children.clone().into_iter().map(|c| NodeRef {
            tree: self.tree,
            path: c.path,
        })
    }
}

impl<'tree> Iterator for DataSiblings<'tree> {
    type Item = NodeRef<'tree>;

    fn next(&mut self) -> Option<Self::Item> {
        let info = self.children.get(self.pos)?;
        self.pos += 1;
        Some(NodeRef {
            tree: self.tree,
            path: info.path.clone(),
        })
    }
}

fn path_node_name(path: &str) -> &str {
    let last = path.rsplit_once('/').map(|(_, s)| s).unwrap_or(path);
    let without_preds = last.split_once('[').map(|(s, _)| s).unwrap_or(last);
    without_preds
        .split_once(':')
        .map(|(_, n)| n)
        .unwrap_or(without_preds)
}

/// A typed YANG leaf/leaf-list value.
#[derive(Debug, Clone, PartialEq, Eq)]
#[non_exhaustive]
pub enum Value {
    /// `int8`.
    Int8(i8),
    /// `int16`.
    Int16(i16),
    /// `int32`.
    Int32(i32),
    /// `int64`.
    Int64(i64),
    /// `uint8`.
    Uint8(u8),
    /// `uint16`.
    Uint16(u16),
    /// `uint32`.
    Uint32(u32),
    /// `uint64`.
    Uint64(u64),
    /// `decimal64`.
    Decimal64(Decimal64),
    /// `boolean`.
    Bool(bool),
    /// `empty`.
    Empty,
    /// `string`.
    Str(String),
    /// `binary` (base64-decoded).
    Binary(Vec<u8>),
    /// `enumeration`.
    Enum(String),
    /// `bits`.
    Bits(Vec<String>),
    /// `identityref`.
    Identityref(String),
    /// `instance-identifier`.
    InstanceIdentifier(String),
}

/// A `decimal64` value stored as a fixed-point integer.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub struct Decimal64 {
    raw: i64,
    fraction_digits: u8,
}

impl Decimal64 {
    /// Create from a raw fixed-point integer and the number of fraction digits.
    pub fn new(raw: i64, fraction_digits: u8) -> Self {
        Self {
            raw,
            fraction_digits,
        }
    }

    /// The raw fixed-point integer.
    pub fn raw(&self) -> i64 {
        self.raw
    }

    /// Number of fractional digits (1..=18).
    pub fn fraction_digits(&self) -> u8 {
        self.fraction_digits
    }
}

impl std::fmt::Display for Decimal64 {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        if self.fraction_digits == 0 {
            return write!(f, "{}", self.raw);
        }
        let divisor = 10i64.pow(u32::from(self.fraction_digits));
        // `unsigned_abs` (not signed division) so a negative magnitude >= 1 does not
        // produce a double minus (e.g. -12.34, not --12.34), and is total at i64::MIN.
        let whole = (self.raw / divisor).unsigned_abs();
        let frac = (self.raw % divisor).unsigned_abs();
        let padded = format!(
            "{:0>width$}",
            frac,
            width = usize::from(self.fraction_digits)
        );
        // RFC-7950 canonical form strips trailing fractional zeros, keeping >= 1 digit.
        let trimmed = padded.trim_end_matches('0');
        let frac_str = if trimmed.is_empty() { "0" } else { trimmed };
        if self.raw < 0 {
            write!(f, "-{whole}.{frac_str}")
        } else {
            write!(f, "{whole}.{frac_str}")
        }
    }
}

fn parse_value(s: &str, type_info: &TypeInfo<'_>) -> Result<Value> {
    match type_info.base() {
        BaseType::LeafRef => {
            if let ResolvedType::LeafRef {
                realtype: Some(realtype),
                ..
            } = type_info.resolved()
            {
                return parse_value(s, realtype);
            }
            Ok(Value::Str(s.to_string()))
        }
        BaseType::Int8 => Ok(Value::Int8(parse_int(s)?)),
        BaseType::Int16 => Ok(Value::Int16(parse_int(s)?)),
        BaseType::Int32 => Ok(Value::Int32(parse_int(s)?)),
        BaseType::Int64 => Ok(Value::Int64(parse_int(s)?)),
        BaseType::Uint8 => Ok(Value::Uint8(parse_int(s)?)),
        BaseType::Uint16 => Ok(Value::Uint16(parse_int(s)?)),
        BaseType::Uint32 => Ok(Value::Uint32(parse_int(s)?)),
        BaseType::Uint64 => Ok(Value::Uint64(parse_int(s)?)),
        BaseType::Decimal64 => {
            let fd = match type_info.resolved() {
                ResolvedType::Decimal64 {
                    fraction_digits, ..
                } => fraction_digits.value(),
                _ => 1,
            };
            Ok(Value::Decimal64(parse_decimal64(s, fd)?))
        }
        BaseType::String => Ok(Value::Str(s.to_string())),
        BaseType::Boolean => parse_bool(s).map(Value::Bool),
        BaseType::Empty => Ok(Value::Empty),
        BaseType::Binary => Ok(Value::Binary(decode_base64(s)?)),
        BaseType::Enumeration => Ok(Value::Enum(s.to_string())),
        BaseType::Bits => Ok(Value::Bits(
            s.split_whitespace().map(String::from).collect(),
        )),
        BaseType::IdentityRef => Ok(Value::Identityref(s.to_string())),
        BaseType::InstanceIdentifier => Ok(Value::InstanceIdentifier(s.to_string())),
        BaseType::Union | BaseType::Unknown => Ok(Value::Str(s.to_string())),
    }
}

fn parse_int<T: std::str::FromStr<Err = std::num::ParseIntError>>(s: &str) -> Result<T> {
    s.parse().map_err(|e: std::num::ParseIntError| {
        Error::ffi(RuleCode::DataPath, format!("invalid integer: {e}"))
    })
}

fn parse_bool(s: &str) -> Result<bool> {
    match s {
        "true" => Ok(true),
        "false" => Ok(false),
        _ => Err(Error::ffi(
            RuleCode::DataPath,
            format!("invalid boolean: {s}"),
        )),
    }
}

fn parse_decimal64(s: &str, fraction_digits: u8) -> Result<Decimal64> {
    if fraction_digits == 0 {
        let raw = parse_int::<i64>(s)?;
        return Ok(Decimal64::new(raw, 0));
    }
    let negative = s.starts_with('-');
    let s = s.trim_start_matches('-').trim_start_matches('+');
    let (whole, frac) = s.split_once('.').unwrap_or((s, ""));
    if whole.is_empty() && frac.is_empty() {
        return Err(Error::ffi(RuleCode::DataPath, "empty decimal64 value"));
    }
    let whole = if whole.is_empty() {
        0i64
    } else {
        whole.parse::<i64>().map_err(|e: std::num::ParseIntError| {
            Error::ffi(RuleCode::DataPath, format!("invalid decimal64: {e}"))
        })?
    };
    let frac = frac
        .chars()
        .filter(|c| c.is_ascii_digit())
        .collect::<String>();
    let frac = format!("{:0<width$}", frac, width = usize::from(fraction_digits));
    let frac = &frac[..usize::from(fraction_digits)];
    let frac_val = frac.parse::<i64>().map_err(|e: std::num::ParseIntError| {
        Error::ffi(RuleCode::DataPath, format!("invalid decimal64: {e}"))
    })?;
    let divisor = 10i64.pow(u32::from(fraction_digits));
    let raw = whole * divisor + frac_val;
    Ok(Decimal64::new(
        if negative { -raw } else { raw },
        fraction_digits,
    ))
}

fn decode_base64(s: &str) -> Result<Vec<u8>> {
    // Minimal RFC 4648 base64 decoder (no padding handling beyond `=`).
    const TABLE: [u8; 256] = {
        let mut t = [255u8; 256];
        let mut i = 0;
        while i < 64 {
            let ch = match i {
                0..=25 => b'A' + i,
                26..=51 => b'a' + i - 26,
                52..=61 => b'0' + i - 52,
                62 => b'+',
                63 => b'/',
                _ => unreachable!(),
            };
            t[ch as usize] = i;
            i += 1;
        }
        t[b'=' as usize] = 0;
        t
    };

    let mut out = Vec::with_capacity(s.len() * 3 / 4);
    let mut buf = 0u32;
    let mut bits = 0u32;
    for ch in s.bytes() {
        if ch.is_ascii_whitespace() {
            continue;
        }
        let val = TABLE[ch as usize];
        if val == 255 {
            return Err(Error::ffi(RuleCode::DataPath, "invalid base64 character"));
        }
        if ch == b'=' {
            continue;
        }
        buf = (buf << 6) | u32::from(val);
        bits += 6;
        if bits >= 8 {
            bits -= 8;
            out.push((buf >> bits) as u8);
        }
    }
    Ok(out)
}
