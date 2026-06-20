//! Schema-tree introspection.
//!
//! A `SchemaTree` is a coarse, ordered mirror of libyang's compiled schema
//! (`lysc_node`) sibling chain. The new `SchemaNodeRef<'ctx>` API provides
//! goyang-grade borrowed handles with rich metadata; the legacy owned tree is
//! retained as a deprecated thin view for v1 consumers.

use std::collections::HashMap;

use cambium_libyang_sys::adapter::{
    RawBaseType, RawConfig, RawDeviation, RawExtension, RawImport, RawModuleInfo,
    RawMustConstraint, RawSchemaNode, RawStatus, RawWhenConstraint,
};

/// Coarse classification of a leaf/leaf-list value type.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
#[non_exhaustive]
pub enum LeafType {
    /// `string` or any type mapped to a string in generated code.
    String,
    /// Signed or unsigned integer, or `decimal64`.
    Int,
    /// `boolean`.
    Bool,
    /// A type not mapped above.
    Unknown,
}

/// A node in a compiled YANG schema tree.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct SchemaNode {
    name: String,
    kind: SchemaNodeKind,
    ordered_by_user: bool,
    is_key: bool,
    key_names: Vec<String>,
    leaf_type: LeafType,
    children: Vec<SchemaNode>,
}

impl SchemaNode {
    /// Node name.
    pub fn name(&self) -> &str {
        &self.name
    }

    /// Node kind.
    pub fn kind(&self) -> SchemaNodeKind {
        self.kind
    }

    /// True for `ordered-by user` lists and leaf-lists.
    pub fn ordered_by_user(&self) -> bool {
        self.ordered_by_user
    }

    /// True for list key leaves.
    pub fn is_key(&self) -> bool {
        self.is_key
    }

    /// For a list, the names of its key leaves in key-statement order.
    pub fn key_names(&self) -> &[String] {
        &self.key_names
    }

    /// For a leaf or leaf-list, the coarse generated type.
    pub fn leaf_type(&self) -> LeafType {
        self.leaf_type
    }

    /// Child nodes in schema declaration order.
    pub fn children(&self) -> &[SchemaNode] {
        &self.children
    }
}

/// Stable classification of a schema node.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
#[non_exhaustive]
pub enum SchemaNodeKind {
    /// Synthetic root wrapping a module's top-level nodes.
    Module,
    /// `container`.
    Container,
    /// `leaf`.
    Leaf,
    /// `leaf-list`.
    LeafList,
    /// `list`.
    List,
    /// `choice`.
    Choice,
    /// `case`.
    Case,
    /// `anydata` / `anyxml`.
    AnyData,
    /// `rpc`.
    Rpc,
    /// `action`.
    Action,
    /// `input`.
    Input,
    /// `output`.
    Output,
    /// `notification`.
    Notification,
    /// Anything not mapped above.
    Unknown,
}

/// A compiled YANG schema tree for one module.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct SchemaTree {
    root: SchemaNode,
    module_ns: String,
}

impl SchemaTree {
    pub(crate) fn from_raw(raw: RawSchemaNode) -> Self {
        Self {
            module_ns: raw.module_ns.clone(),
            root: SchemaNode::from_raw(raw),
        }
    }

    /// Return the module namespace (empty if none was provided by libyang).
    pub fn module_ns(&self) -> &str {
        &self.module_ns
    }

    /// Return the synthetic module root.
    pub fn root(&self) -> &SchemaNode {
        &self.root
    }

    /// Walk the tree in pre-order, yielding every node.
    pub fn iter(&self) -> SchemaTreeIter<'_> {
        SchemaTreeIter {
            stack: vec![self.root.children.iter()],
        }
    }

    /// Find a node by path of names (e.g. `["top", "z"]`).
    pub fn find(&self, path: &[&str]) -> Option<&SchemaNode> {
        let mut cur = &self.root;
        for segment in path {
            cur = cur.children.iter().find(|c| c.name == *segment)?;
        }
        Some(cur)
    }
}

impl SchemaNode {
    fn from_raw(raw: RawSchemaNode) -> Self {
        Self {
            name: raw.name,
            kind: SchemaNodeKind::from_raw(&raw.kind),
            ordered_by_user: raw.ordered_by_user,
            is_key: raw.is_key,
            key_names: raw.key_names,
            leaf_type: LeafType::from_raw(&raw.base_type),
            children: raw.children.into_iter().map(SchemaNode::from_raw).collect(),
        }
    }
}

impl LeafType {
    fn from_raw(base: &RawBaseType) -> Self {
        match base {
            RawBaseType::String => LeafType::String,
            RawBaseType::Boolean => LeafType::Bool,
            RawBaseType::Int8
            | RawBaseType::Int16
            | RawBaseType::Int32
            | RawBaseType::Int64
            | RawBaseType::Uint8
            | RawBaseType::Uint16
            | RawBaseType::Uint32
            | RawBaseType::Uint64
            | RawBaseType::Decimal64 => LeafType::Int,
            _ => LeafType::Unknown,
        }
    }
}

impl SchemaNodeKind {
    fn from_raw(kind: &str) -> Self {
        match kind {
            "module" => SchemaNodeKind::Module,
            "container" => SchemaNodeKind::Container,
            "leaf" => SchemaNodeKind::Leaf,
            "leaflist" => SchemaNodeKind::LeafList,
            "list" => SchemaNodeKind::List,
            "choice" => SchemaNodeKind::Choice,
            "case" => SchemaNodeKind::Case,
            "anydata" => SchemaNodeKind::AnyData,
            "rpc" => SchemaNodeKind::Rpc,
            "action" => SchemaNodeKind::Action,
            "input" => SchemaNodeKind::Input,
            "output" => SchemaNodeKind::Output,
            "notification" => SchemaNodeKind::Notification,
            _ => SchemaNodeKind::Unknown,
        }
    }
}

/// Pre-order iterator over schema nodes.
pub struct SchemaTreeIter<'a> {
    stack: Vec<std::slice::Iter<'a, SchemaNode>>,
}

impl<'a> Iterator for SchemaTreeIter<'a> {
    type Item = &'a SchemaNode;

    fn next(&mut self) -> Option<Self::Item> {
        loop {
            let it = self.stack.last_mut()?;
            if let Some(node) = it.next() {
                self.stack.push(node.children.iter());
                return Some(node);
            }
            self.stack.pop();
        }
    }
}

// =============================================================================
// New goyang-grade schema introspection API (Phase 1).
// =============================================================================

/// Read-write vs read-only config flag.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum Config {
    /// `config true` (default for data nodes).
    Rw,
    /// `config false`.
    Ro,
}

/// Status substatement.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
#[non_exhaustive]
pub enum Status {
    /// `status current`.
    Current,
    /// `status deprecated`.
    Deprecated,
    /// `status obsolete`.
    Obsolete,
}

/// Ordering semantics for lists and leaf-lists.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum OrderedBy {
    /// `ordered-by system` (canonical, deterministic).
    System,
    /// `ordered-by user` (byte-exact insertion order).
    User,
}

/// The 19 RFC-7950 built-in YANG base types.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
#[non_exhaustive]
pub enum BaseType {
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
    /// Unknown or not a leaf/leaf-list.
    Unknown,
}

impl BaseType {
    fn from_raw(raw: RawBaseType) -> Self {
        match raw {
            RawBaseType::Int8 => BaseType::Int8,
            RawBaseType::Int16 => BaseType::Int16,
            RawBaseType::Int32 => BaseType::Int32,
            RawBaseType::Int64 => BaseType::Int64,
            RawBaseType::Uint8 => BaseType::Uint8,
            RawBaseType::Uint16 => BaseType::Uint16,
            RawBaseType::Uint32 => BaseType::Uint32,
            RawBaseType::Uint64 => BaseType::Uint64,
            RawBaseType::Decimal64 => BaseType::Decimal64,
            RawBaseType::String => BaseType::String,
            RawBaseType::Boolean => BaseType::Boolean,
            RawBaseType::Empty => BaseType::Empty,
            RawBaseType::Binary => BaseType::Binary,
            RawBaseType::Bits => BaseType::Bits,
            RawBaseType::Enumeration => BaseType::Enumeration,
            RawBaseType::IdentityRef => BaseType::IdentityRef,
            RawBaseType::InstanceIdentifier => BaseType::InstanceIdentifier,
            RawBaseType::LeafRef => BaseType::LeafRef,
            RawBaseType::Union => BaseType::Union,
            RawBaseType::Unknown | _ => BaseType::Unknown,
        }
    }
}

/// Signed integer kinds.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
#[non_exhaustive]
pub enum IntKind {
    /// `int8`.
    I8,
    /// `int16`.
    I16,
    /// `int32`.
    I32,
    /// `int64`.
    I64,
    /// `uint8`.
    U8,
    /// `uint16`.
    U16,
    /// `uint32`.
    U32,
    /// `uint64`.
    U64,
}

impl IntKind {
    fn from_base(base: BaseType) -> Option<Self> {
        match base {
            BaseType::Int8 => Some(IntKind::I8),
            BaseType::Int16 => Some(IntKind::I16),
            BaseType::Int32 => Some(IntKind::I32),
            BaseType::Int64 => Some(IntKind::I64),
            BaseType::Uint8 => Some(IntKind::U8),
            BaseType::Uint16 => Some(IntKind::U16),
            BaseType::Uint32 => Some(IntKind::U32),
            BaseType::Uint64 => Some(IntKind::U64),
            _ => None,
        }
    }
}

/// `fraction-digits` for `decimal64`.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub struct FractionDigits(u8);

impl FractionDigits {
    /// Create a new fraction-digits value (1..=18).
    pub fn new(value: u8) -> Option<Self> {
        if (1..=18).contains(&value) {
            Some(Self(value))
        } else {
            None
        }
    }

    /// The raw value.
    pub fn value(&self) -> u8 {
        self.0
    }
}

/// One enum or bit value in declaration order.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct EnumValue {
    /// Enum/bit name.
    name: String,
    /// Integer value (enum value or bit position).
    value: i64,
}

impl EnumValue {
    /// Enum/bit name.
    pub fn name(&self) -> &str {
        &self.name
    }

    /// Integer value (enum value or bit position).
    pub fn value(&self) -> i64 {
        self.value
    }
}

/// Definition of an `enumeration` type.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct EnumDef {
    values: Vec<EnumValue>,
}

impl EnumDef {
    /// Values in declaration order.
    pub fn values(&self) -> &[EnumValue] {
        &self.values
    }
}

/// Definition of a `bits` type.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct BitsDef {
    values: Vec<EnumValue>,
}

impl BitsDef {
    /// Bits in declaration order.
    pub fn values(&self) -> &[EnumValue] {
        &self.values
    }
}

/// A textual pattern constraint for strings.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct Pattern {
    regex: String,
    error_app_tag: Option<String>,
    inverted: bool,
}

/// One compiled `must` constraint.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct MustConstraint {
    expression: String,
    error_message: Option<String>,
    error_app_tag: Option<String>,
    description: Option<String>,
    reference: Option<String>,
}

impl MustConstraint {
    fn from_raw(raw: &RawMustConstraint) -> Self {
        Self {
            expression: raw.expression.clone(),
            error_message: raw.error_message.clone(),
            error_app_tag: raw.error_app_tag.clone(),
            description: raw.description.clone(),
            reference: raw.reference.clone(),
        }
    }

    /// XPath expression.
    pub fn expression(&self) -> &str {
        &self.expression
    }

    /// `error-message`, if present.
    pub fn error_message(&self) -> Option<&str> {
        self.error_message.as_deref()
    }

    /// `error-app-tag`, if present.
    pub fn error_app_tag(&self) -> Option<&str> {
        self.error_app_tag.as_deref()
    }

    /// `description`, if present.
    pub fn description(&self) -> Option<&str> {
        self.description.as_deref()
    }

    /// `reference`, if present.
    pub fn reference(&self) -> Option<&str> {
        self.reference.as_deref()
    }
}

/// One compiled `when` constraint.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct WhenConstraint {
    expression: String,
    description: Option<String>,
    reference: Option<String>,
}

impl WhenConstraint {
    fn from_raw(raw: &RawWhenConstraint) -> Self {
        Self {
            expression: raw.expression.clone(),
            description: raw.description.clone(),
            reference: raw.reference.clone(),
        }
    }

    /// XPath expression.
    pub fn expression(&self) -> &str {
        &self.expression
    }

    /// `description`, if present.
    pub fn description(&self) -> Option<&str> {
        self.description.as_deref()
    }

    /// `reference`, if present.
    pub fn reference(&self) -> Option<&str> {
        self.reference.as_deref()
    }
}

/// One YANG extension instance attached to a schema node.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct Extension {
    name: String,
    argument: Option<String>,
    module_name: String,
}

impl Extension {
    fn from_raw(raw: &RawExtension) -> Self {
        Self {
            name: raw.name.clone(),
            argument: raw.argument.clone(),
            module_name: raw.module_name.clone(),
        }
    }

    /// Extension definition name.
    pub fn name(&self) -> &str {
        &self.name
    }

    /// Optional extension argument string.
    pub fn argument(&self) -> Option<&str> {
        self.argument.as_deref()
    }

    /// Name of the module defining the extension.
    pub fn module_name(&self) -> &str {
        &self.module_name
    }
}

/// One list `unique` constraint.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct UniqueConstraint<'ctx> {
    leafs: Vec<SchemaNodeRef<'ctx>>,
}

impl<'ctx> UniqueConstraint<'ctx> {
    /// Participating leaf nodes in unique-statement order.
    pub fn leafs(&self) -> Vec<SchemaNodeRef<'ctx>> {
        self.leafs.clone()
    }
}

/// Metadata for one parsed deviation property.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct Deviation {
    target_path: String,
    source_module: String,
    deviation_type: String,
    property: String,
    new_value: String,
    description: Option<String>,
    reference: Option<String>,
}

impl Deviation {
    fn from_raw(raw: &RawDeviation) -> Self {
        Self {
            target_path: raw.target_path.clone(),
            source_module: raw.source_module.clone(),
            deviation_type: raw.deviation_type.clone(),
            property: raw.property.clone(),
            new_value: raw.new_value.clone(),
            description: raw.description.clone(),
            reference: raw.reference.clone(),
        }
    }

    /// Target schema nodeid as written in the deviation statement.
    pub fn target_path(&self) -> &str {
        &self.target_path
    }

    /// Module defining the deviation.
    pub fn source_module(&self) -> &str {
        &self.source_module
    }

    /// Deviate operation: `not-supported`, `add`, `delete`, or `replace`.
    pub fn deviation_type(&self) -> &str {
        &self.deviation_type
    }

    /// Affected property, or empty for `not-supported`.
    pub fn property(&self) -> &str {
        &self.property
    }

    /// New or removed property value, if represented by the parsed deviation.
    pub fn new_value(&self) -> &str {
        &self.new_value
    }

    /// `description`, if present.
    pub fn description(&self) -> Option<&str> {
        self.description.as_deref()
    }

    /// `reference`, if present.
    pub fn reference(&self) -> Option<&str> {
        self.reference.as_deref()
    }
}

impl Pattern {
    /// POSIX regular expression.
    pub fn regex(&self) -> &str {
        &self.regex
    }

    /// Error app-tag, if any.
    pub fn error_app_tag(&self) -> Option<&str> {
        self.error_app_tag.as_deref()
    }

    /// Invert-match flag (`modifier invert-match`).
    pub fn is_inverted(&self) -> bool {
        self.inverted
    }
}

/// A numeric/string range bound.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct RangeBound {
    min: String,
    max: String,
}

impl RangeBound {
    /// Canonical minimum string (may be a number or "min").
    pub fn min(&self) -> &str {
        &self.min
    }

    /// Canonical maximum string (may be a number or "max").
    pub fn max(&self) -> &str {
        &self.max
    }
}

/// An identity borrowed from a frozen context.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub struct Identity<'ctx> {
    forest: &'ctx SchemaForest,
    module_index: usize,
    identity_index: usize,
}

/// Module import metadata.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct Import {
    prefix: String,
    name: String,
    revision: Option<String>,
}

impl Import {
    fn from_raw(raw: &RawImport) -> Self {
        Self {
            prefix: raw.prefix.clone(),
            name: raw.name.clone(),
            revision: raw.revision.clone(),
        }
    }

    /// Import prefix.
    pub fn prefix(&self) -> &str {
        &self.prefix
    }

    /// Imported module name.
    pub fn name(&self) -> &str {
        &self.name
    }

    /// Requested revision-date, if present.
    pub fn revision(&self) -> Option<&str> {
        self.revision.as_deref()
    }
}

impl<'ctx> Identity<'ctx> {
    fn data(&self) -> &'ctx IdentityData {
        &self.forest.modules[self.module_index].identities[self.identity_index]
    }

    /// Identity name.
    pub fn name(&self) -> &str {
        &self.data().name
    }

    /// Owning module.
    pub fn module(&self) -> Module<'ctx> {
        Module {
            forest: self.forest,
            module_index: self.module_index,
        }
    }

    /// Direct base identities.
    pub fn bases(&self) -> IdentityBases<'ctx> {
        let data = self.data();
        let indices = data.base_indices.clone();
        IdentityBases {
            forest: self.forest,
            module_index: self.module_index,
            indices,
            pos: 0,
        }
    }

    /// Transitively derived identities (recursive closure).
    pub fn derived(&self) -> Vec<Identity<'ctx>> {
        let data = self.data();
        let mut out = Vec::new();
        let mut visited: std::collections::HashSet<(usize, usize)> =
            std::collections::HashSet::new();
        let mut stack: Vec<Identity<'ctx>> = data
            .derived_names
            .iter()
            .filter_map(|name| self.forest.find_identity(name))
            .collect();
        while let Some(derived) = stack.pop() {
            let key = (derived.module_index, derived.identity_index);
            if !visited.insert(key) {
                continue;
            }
            stack.extend(
                derived
                    .data()
                    .derived_names
                    .iter()
                    .filter_map(|name| self.forest.find_identity(name)),
            );
            out.push(derived);
        }
        out
    }
}

/// Iterator over direct base identities.
#[derive(Debug, Clone)]
pub struct IdentityBases<'ctx> {
    forest: &'ctx SchemaForest,
    module_index: usize,
    indices: Vec<usize>,
    pos: usize,
}

impl<'ctx> Iterator for IdentityBases<'ctx> {
    type Item = Identity<'ctx>;

    fn next(&mut self) -> Option<Self::Item> {
        let idx = *self.indices.get(self.pos)?;
        self.pos += 1;
        Some(Identity {
            forest: self.forest,
            module_index: self.module_index,
            identity_index: idx,
        })
    }
}

/// Iterator over module identities.
#[derive(Debug, Clone)]
pub struct Identities<'ctx> {
    forest: &'ctx SchemaForest,
    module_index: usize,
    indices: Vec<usize>,
    pos: usize,
}

impl<'ctx> Iterator for Identities<'ctx> {
    type Item = Identity<'ctx>;

    fn next(&mut self) -> Option<Self::Item> {
        let idx = *self.indices.get(self.pos)?;
        self.pos += 1;
        Some(Identity {
            forest: self.forest,
            module_index: self.module_index,
            identity_index: idx,
        })
    }
}

/// Resolved type constraints for a leaf or leaf-list.
#[derive(Debug, Clone, PartialEq, Eq)]
#[non_exhaustive]
pub enum ResolvedType<'ctx> {
    /// Integer types with optional range constraints.
    Int {
        /// Signed or unsigned integer kind.
        kind: IntKind,
        /// Optional range constraints.
        range: Option<Vec<RangeBound>>,
    },
    /// `decimal64`.
    Decimal64 {
        /// Number of fractional digits.
        fraction_digits: FractionDigits,
        /// Optional range constraints.
        range: Option<Vec<RangeBound>>,
    },
    /// `boolean`.
    Boolean,
    /// `empty`.
    Empty,
    /// `binary` with optional length constraint.
    Binary {
        /// Optional length constraints.
        length: Option<Vec<RangeBound>>,
    },
    /// `string` with optional length and patterns.
    StringType {
        /// Optional length constraints.
        length: Option<Vec<RangeBound>>,
        /// Pattern constraints.
        patterns: Vec<Pattern>,
    },
    /// `enumeration`.
    Enumeration(EnumDef),
    /// `bits`.
    Bits(BitsDef),
    /// `identityref`.
    IdentityRef {
        /// Direct base identities.
        bases: Vec<Identity<'ctx>>,
    },
    /// `instance-identifier`.
    InstanceIdentifier {
        /// Whether the instance must exist.
        require_instance: bool,
    },
    /// `leafref`.
    LeafRef {
        /// Raw leafref path expression.
        path: Option<String>,
        /// Resolved target schema node, if available.
        target: Option<Box<SchemaNodeRef<'ctx>>>,
        /// Resolved target type (the type of the leaf the path points to).
        realtype: Option<Box<TypeInfo<'ctx>>>,
        /// Whether the target instance must exist.
        require_instance: bool,
    },
    /// `union`.
    Union(Vec<TypeInfo<'ctx>>),
    /// Unknown or unmapped.
    Unknown,
}

/// Rich type information for a leaf or leaf-list.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct TypeInfo<'ctx> {
    base: BaseType,
    typedef_name: Option<String>,
    resolved: ResolvedType<'ctx>,
}

impl<'ctx> TypeInfo<'ctx> {
    /// The built-in base type.
    pub fn base(&self) -> BaseType {
        self.base
    }

    /// The typedef name if this type is a derived typedef, else `None`.
    pub fn typedef_name(&self) -> Option<&str> {
        self.typedef_name.as_deref()
    }

    /// Resolved constraints for this type.
    pub fn resolved(&self) -> &ResolvedType<'ctx> {
        &self.resolved
    }
}

/// Internal storage for one identity.
#[derive(Debug, Clone, PartialEq, Eq)]
struct IdentityData {
    name: String,
    module_name: String,
    base_indices: Vec<usize>,
    derived_names: Vec<String>,
}

/// Internal storage for one node in a compiled module forest.
#[derive(Debug, Clone, PartialEq, Eq)]
pub(crate) struct SchemaNodeData {
    /// Node name.
    pub name: String,
    /// Node kind.
    pub kind: SchemaNodeKind,
    /// Read-write vs read-only.
    pub config: Config,
    /// Status substatement.
    pub status: Status,
    /// True for `mandatory true`.
    pub mandatory: bool,
    /// True for presence containers.
    pub presence: bool,
    /// `description` substatement.
    pub description: Option<String>,
    /// `reference` substatement.
    pub reference: Option<String>,
    /// Extension instances in declaration order.
    pub extensions: Vec<Extension>,
    /// Name of the grouping this node originated from, if known.
    pub grouping_origin: Option<String>,
    /// `units` substatement.
    pub units: Option<String>,
    /// `default` values as canonical strings in declaration order.
    pub default_values: Vec<String>,
    /// Compiled `must` constraints in declaration order.
    pub musts: Vec<MustConstraint>,
    /// Compiled `when` constraints in declaration order.
    pub whens: Vec<WhenConstraint>,
    /// Compiled list `unique` constraints as leaf schema pointers.
    pub unique_leaf_schemas: Vec<Vec<*const ::std::os::raw::c_void>>,
    /// `min-elements` for lists/leaf-lists.
    pub min_elements: Option<u32>,
    /// `max-elements` for lists/leaf-lists.
    pub max_elements: Option<u32>,
    /// Ordering semantics.
    pub ordered_by: OrderedBy,
    /// True for list key leaves.
    pub is_key: bool,
    /// List key names in key-statement order.
    pub key_names: Vec<String>,
    /// Indices of key leaves in key-statement order.
    pub key_indices: Vec<usize>,
    /// Raw type information for leaves/leaf-lists.
    pub raw_type_info: cambium_libyang_sys::adapter::RawTypeInfo,
    /// Indices of children in declaration order.
    pub children: Vec<usize>,
    /// Name of the module that owns this schema node.
    pub owner_module_name: String,
    /// XML namespace of the module that owns this schema node.
    pub owner_module_namespace: String,
    /// Raw compiled schema pointer used for the data-to-schema bridge.
    pub schema_ptr: *const ::std::os::raw::c_void,
}

impl SchemaNodeData {
    fn from_raw(raw: RawSchemaNode) -> Self {
        Self {
            name: raw.name,
            kind: SchemaNodeKind::from_raw(&raw.kind),
            config: Config::from_raw(raw.config),
            status: Status::from_raw(raw.status),
            mandatory: raw.mandatory,
            presence: raw.presence,
            description: raw.description,
            reference: raw.reference,
            extensions: raw.extensions.iter().map(Extension::from_raw).collect(),
            grouping_origin: raw.grouping_origin,
            units: raw.units,
            default_values: raw.default_values,
            musts: raw.musts.iter().map(MustConstraint::from_raw).collect(),
            whens: raw.whens.iter().map(WhenConstraint::from_raw).collect(),
            unique_leaf_schemas: raw
                .unique_constraints
                .into_iter()
                .map(|unique| unique.leaf_schemas)
                .collect(),
            min_elements: raw.min_elements,
            max_elements: raw.max_elements,
            ordered_by: if raw.ordered_by_user {
                OrderedBy::User
            } else {
                OrderedBy::System
            },
            is_key: raw.is_key,
            key_names: raw.key_names,
            key_indices: raw.key_indices,
            raw_type_info: raw.type_info,
            children: Vec::new(),
            owner_module_name: raw.owner_module_name,
            owner_module_namespace: raw.owner_module_namespace,
            schema_ptr: raw.schema,
        }
    }
}

impl Config {
    fn from_raw(raw: RawConfig) -> Self {
        match raw {
            RawConfig::Ro => Config::Ro,
            _ => Config::Rw,
        }
    }
}

impl Status {
    fn from_raw(raw: RawStatus) -> Self {
        match raw {
            RawStatus::Current => Status::Current,
            RawStatus::Deprecated => Status::Deprecated,
            RawStatus::Obsolete => Status::Obsolete,
        }
    }
}

/// A compiled module handle borrowed from a frozen `Context`.
#[derive(Debug, Clone, Copy)]
pub struct Module<'ctx> {
    pub(crate) forest: &'ctx SchemaForest,
    pub(crate) module_index: usize,
}

impl<'ctx> Module<'ctx> {
    /// Module name.
    pub fn name(&self) -> &str {
        &self.forest.modules[self.module_index].info.name
    }

    /// XML namespace.
    pub fn namespace(&self) -> &str {
        &self.forest.modules[self.module_index].info.namespace
    }

    /// Module prefix.
    pub fn prefix(&self) -> &str {
        &self.forest.modules[self.module_index].info.prefix
    }

    /// Revision string, if the module declared one.
    pub fn revision(&self) -> Option<&str> {
        self.forest.modules[self.module_index]
            .info
            .revision
            .as_deref()
    }

    /// True if the module is implemented (rather than just imported).
    pub fn is_implemented(&self) -> bool {
        self.forest.modules[self.module_index].info.is_implemented
    }

    /// Import statements in source declaration order.
    pub fn imports(&self) -> Vec<Import> {
        self.forest.modules[self.module_index]
            .info
            .imports
            .iter()
            .map(Import::from_raw)
            .collect()
    }

    /// Names of loaded modules that augment this module.
    pub fn augmented_by(&self) -> Vec<String> {
        self.forest.modules[self.module_index]
            .info
            .augmented_by
            .clone()
    }

    /// Names of loaded modules that deviate this module.
    pub fn deviated_by(&self) -> Vec<String> {
        self.forest.modules[self.module_index]
            .info
            .deviated_by
            .clone()
    }

    /// Deviations defined by this module.
    pub fn deviations(&self) -> Vec<Deviation> {
        self.forest.modules[self.module_index]
            .info
            .deviations
            .iter()
            .map(Deviation::from_raw)
            .collect()
    }

    /// Resolve a data-model prefix to a module.
    ///
    /// An empty prefix, this module's declared prefix, and this module's name
    /// resolve to the receiver module.
    pub fn resolve_prefix(&self, prefix: &str) -> Option<Module<'ctx>> {
        let info = &self.forest.modules[self.module_index].info;
        if prefix.is_empty() || prefix == info.prefix || prefix == info.name {
            return Some(*self);
        }
        let import = info.imports.iter().find(|import| import.prefix == prefix)?;
        let module_index = self.forest.module_index(&import.name)?;
        Some(Module {
            forest: self.forest,
            module_index,
        })
    }

    /// Top-level data nodes in schema declaration order.
    pub fn top_level(&self) -> SchemaChildren<'ctx> {
        let root = self.forest.modules[self.module_index].root;
        let mut indices = Vec::new();
        self.forest
            .collect_data_children(self.module_index, root, false, &mut indices);
        SchemaChildren {
            forest: self.forest,
            module_index: self.module_index,
            indices,
            pos: 0,
        }
    }

    /// Module-level RPCs in schema declaration order.
    pub fn rpcs(&self) -> SchemaChildren<'ctx> {
        self.forest
            .module_children_by_kind(self.module_index, SchemaNodeKind::Rpc)
    }

    /// Module-level actions in schema declaration order.
    ///
    /// YANG actions are normally nested, so this is usually empty.
    pub fn actions(&self) -> SchemaChildren<'ctx> {
        self.forest
            .module_children_by_kind(self.module_index, SchemaNodeKind::Action)
    }

    /// Module-level notifications in schema declaration order.
    pub fn notifications(&self) -> SchemaChildren<'ctx> {
        self.forest
            .module_children_by_kind(self.module_index, SchemaNodeKind::Notification)
    }

    /// Find a schema node by schema path (e.g. `/module:container/leaf`).
    pub fn find_path(&self, path: &str) -> crate::Result<SchemaNodeRef<'ctx>> {
        self.forest
            .find_path(self.module_index, path)
            .ok_or_else(|| crate::Error::from(format!("schema path not found: {path}")))
    }

    /// Identities declared in this module.
    pub fn identities(&self) -> Identities<'ctx> {
        let indices: Vec<usize> =
            (0..self.forest.modules[self.module_index].identities.len()).collect();
        Identities {
            forest: self.forest,
            module_index: self.module_index,
            indices,
            pos: 0,
        }
    }
}

/// A borrowed handle to a compiled schema node.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub struct SchemaNodeRef<'ctx> {
    pub(crate) forest: &'ctx SchemaForest,
    pub(crate) module_index: usize,
    pub(crate) node_index: usize,
}

impl<'ctx> SchemaNodeRef<'ctx> {
    fn data(&self) -> &'ctx SchemaNodeData {
        &self.forest.modules[self.module_index].nodes[self.node_index]
    }

    /// Node name.
    pub fn name(&self) -> &str {
        &self.data().name
    }

    /// Node kind.
    pub fn kind(&self) -> SchemaNodeKind {
        self.data().kind
    }

    /// Owning module.
    pub fn module(&self) -> Module<'ctx> {
        Module {
            forest: self.forest,
            module_index: self.owner_module_index(),
        }
    }

    /// Immediate parent node, or `None` for the synthetic module root.
    pub fn parent(&self) -> Option<SchemaNodeRef<'ctx>> {
        self.forest
            .parent_index(self.module_index, self.node_index)
            .map(|node_index| SchemaNodeRef {
                forest: self.forest,
                module_index: self.module_index,
                node_index,
            })
    }

    /// Ancestors in root-to-leaf order, excluding the synthetic module root and self.
    pub fn ancestors(&self) -> Vec<SchemaNodeRef<'ctx>> {
        self.forest
            .ancestor_indices(self.module_index, self.node_index)
            .into_iter()
            .map(|node_index| SchemaNodeRef {
                forest: self.forest,
                module_index: self.module_index,
                node_index,
            })
            .collect()
    }

    /// Slash-separated schema path beginning with the module name.
    pub fn path(&self) -> String {
        let module = &self.forest.modules[self.module_index];
        let mut segments = self
            .forest
            .path_segments(self.module_index, module.root, self.node_index)
            .unwrap_or_default();
        segments.insert(0, module.info.name.clone());
        format!("/{}", segments.join("/"))
    }

    /// `description` substatement.
    pub fn description(&self) -> Option<&str> {
        self.data().description.as_deref()
    }

    /// `reference` substatement.
    pub fn reference(&self) -> Option<&str> {
        self.data().reference.as_deref()
    }

    /// Extension instances attached to this schema node in declaration order.
    pub fn extensions(&self) -> Vec<Extension> {
        self.data().extensions.clone()
    }

    /// First extension instance with the given definition name.
    pub fn extension(&self, name: &str) -> Option<Extension> {
        self.data()
            .extensions
            .iter()
            .find(|ext| ext.name == name)
            .cloned()
    }

    /// Grouping name if this node was instantiated from a `uses` expansion.
    pub fn grouping_origin(&self) -> Option<&str> {
        self.data().grouping_origin.as_deref()
    }

    /// Deviations from any loaded module whose target path matches this node.
    pub fn deviation_provenance(&self) -> Vec<Deviation> {
        self.forest
            .deviations_for_node(self.module_index, self.node_index)
    }

    /// Status flag.
    pub fn status(&self) -> Status {
        self.data().status
    }

    /// Config flag.
    pub fn config(&self) -> Config {
        self.data().config
    }

    /// True for `mandatory true`.
    pub fn is_mandatory(&self) -> bool {
        self.data().mandatory
    }

    /// True for presence containers.
    pub fn is_presence_container(&self) -> bool {
        self.data().presence
    }

    /// Ordering semantics.
    pub fn ordered_by(&self) -> OrderedBy {
        self.data().ordered_by
    }

    /// True for list key leaves.
    pub fn is_list_key(&self) -> bool {
        self.data().is_key
    }

    /// True for `leaf`.
    pub fn is_leaf(&self) -> bool {
        self.kind() == SchemaNodeKind::Leaf
    }

    /// True for `leaf-list`.
    pub fn is_leaf_list(&self) -> bool {
        self.kind() == SchemaNodeKind::LeafList
    }

    /// True for `container`.
    pub fn is_container(&self) -> bool {
        self.kind() == SchemaNodeKind::Container
    }

    /// True for `list`.
    pub fn is_list(&self) -> bool {
        self.kind() == SchemaNodeKind::List
    }

    /// True for `choice`.
    pub fn is_choice(&self) -> bool {
        self.kind() == SchemaNodeKind::Choice
    }

    /// True for `case`.
    pub fn is_case(&self) -> bool {
        self.kind() == SchemaNodeKind::Case
    }

    /// True when this node is directly or transitively under a `choice`.
    pub fn is_choice_descendant(&self) -> bool {
        self.ancestors()
            .iter()
            .any(|node| node.kind() == SchemaNodeKind::Choice)
    }

    /// True for `rpc`.
    pub fn is_rpc(&self) -> bool {
        self.kind() == SchemaNodeKind::Rpc
    }

    /// True for `action`.
    pub fn is_action(&self) -> bool {
        self.kind() == SchemaNodeKind::Action
    }

    /// True for `notification`.
    pub fn is_notification(&self) -> bool {
        self.kind() == SchemaNodeKind::Notification
    }

    /// True for nodes that can contain children in the goyang `Entry` model.
    pub fn is_dir(&self) -> bool {
        matches!(
            self.kind(),
            SchemaNodeKind::Module
                | SchemaNodeKind::Container
                | SchemaNodeKind::List
                | SchemaNodeKind::Choice
                | SchemaNodeKind::Case
                | SchemaNodeKind::Rpc
                | SchemaNodeKind::Action
                | SchemaNodeKind::Input
                | SchemaNodeKind::Output
                | SchemaNodeKind::Notification
        )
    }

    /// Convenience for `config() == Config::Ro`.
    pub fn read_only(&self) -> bool {
        self.config() == Config::Ro
    }

    /// XML namespace of the owning module.
    pub fn namespace(&self) -> &str {
        let data = self.data();
        if data.owner_module_namespace.is_empty() {
            &self.forest.modules[self.module_index].info.namespace
        } else {
            &data.owner_module_namespace
        }
    }

    /// For a list, its key leaf names in key-statement order.
    pub fn key_names(&self) -> Vec<String> {
        self.data().key_names.clone()
    }

    /// For a list, its key leaves in key-statement order.
    pub fn list_keys(&self) -> SchemaChildren<'ctx> {
        let data = self.data();
        let indices: Vec<usize> = data
            .key_indices
            .iter()
            .filter_map(|&i| data.children.get(i).copied())
            .collect();
        SchemaChildren {
            forest: self.forest,
            module_index: self.module_index,
            indices,
            pos: 0,
        }
    }

    /// For a list, its `unique` constraints in declaration order.
    pub fn unique_constraints(&self) -> Vec<UniqueConstraint<'ctx>> {
        if self.kind() != SchemaNodeKind::List {
            return Vec::new();
        }
        self.data()
            .unique_leaf_schemas
            .iter()
            .map(|leaf_schemas| UniqueConstraint {
                leafs: leaf_schemas
                    .iter()
                    .filter_map(|&ptr| self.forest.schema_ref_by_ptr(ptr))
                    .collect(),
            })
            .collect()
    }

    /// `min-elements` for lists/leaf-lists.
    pub fn min_elements(&self) -> Option<u32> {
        self.data().min_elements
    }

    /// `max-elements` for lists/leaf-lists.
    pub fn max_elements(&self) -> Option<u32> {
        self.data().max_elements
    }

    /// `units` substatement for leaves/leaf-lists.
    pub fn units(&self) -> Option<&str> {
        self.data().units.as_deref()
    }

    /// `default` value as a canonical string for leaves/leaf-lists.
    pub fn default_value(&self) -> Option<&str> {
        self.data().default_values.first().map(String::as_str)
    }

    /// `default` values as canonical strings for leaves/leaf-lists.
    pub fn default_values(&self) -> Vec<String> {
        self.data().default_values.clone()
    }

    /// Compiled `must` constraints in declaration order.
    pub fn musts(&self) -> Vec<MustConstraint> {
        self.data().musts.clone()
    }

    /// Compiled `when` constraints in declaration order.
    pub fn whens(&self) -> Vec<WhenConstraint> {
        self.data().whens.clone()
    }

    /// Child nodes in schema declaration order.
    pub fn children(&self) -> SchemaChildren<'ctx> {
        let indices = self.data().children.clone();
        SchemaChildren {
            forest: self.forest,
            module_index: self.module_index,
            indices,
            pos: 0,
        }
    }

    /// Data children in schema declaration order.
    ///
    /// RPCs, actions, notifications, and their input/output wrappers are
    /// excluded. When `flatten_choices` is true, choice and case nodes are
    /// skipped and their data children are spliced into the returned order.
    pub fn data_children(&self, flatten_choices: bool) -> SchemaChildren<'ctx> {
        let mut indices = Vec::new();
        self.forest.collect_data_children(
            self.module_index,
            self.node_index,
            flatten_choices,
            &mut indices,
        );
        SchemaChildren {
            forest: self.forest,
            module_index: self.module_index,
            indices,
            pos: 0,
        }
    }

    /// Rich type information for a leaf or leaf-list.
    pub fn leaf_type(&self) -> Option<TypeInfo<'ctx>> {
        let data = self.data();
        if matches!(data.kind, SchemaNodeKind::Leaf | SchemaNodeKind::LeafList) {
            Some(
                self.forest
                    .resolve_type(self.module_index, &data.raw_type_info),
            )
        } else {
            None
        }
    }

    fn owner_module_index(&self) -> usize {
        let owner = &self.data().owner_module_name;
        if owner.is_empty() {
            return self.module_index;
        }
        self.forest.module_index(owner).unwrap_or(self.module_index)
    }
}

/// Iterator over schema node children in declaration order.
#[derive(Debug, Clone)]
pub struct SchemaChildren<'ctx> {
    forest: &'ctx SchemaForest,
    module_index: usize,
    indices: Vec<usize>,
    pos: usize,
}

impl<'ctx> Iterator for SchemaChildren<'ctx> {
    type Item = SchemaNodeRef<'ctx>;

    fn next(&mut self) -> Option<Self::Item> {
        let idx = *self.indices.get(self.pos)?;
        self.pos += 1;
        Some(SchemaNodeRef {
            forest: self.forest,
            module_index: self.module_index,
            node_index: idx,
        })
    }
}

impl<'ctx> SchemaChildren<'ctx> {
    /// Number of children.
    pub fn len(&self) -> usize {
        self.indices.len()
    }

    /// True if there are no children.
    pub fn is_empty(&self) -> bool {
        self.indices.is_empty()
    }

    /// Get a child by position.
    pub fn get(&self, i: usize) -> Option<SchemaNodeRef<'ctx>> {
        let _ = self.indices.get(i)?;
        Some(SchemaNodeRef {
            forest: self.forest,
            module_index: self.module_index,
            node_index: self.indices[i],
        })
    }

    /// Find the first child with the given name in declaration order.
    pub fn lookup(&self, name: &str) -> Option<SchemaNodeRef<'ctx>> {
        self.indices
            .iter()
            .copied()
            .find(|&idx| self.forest.modules[self.module_index].nodes[idx].name == name)
            .map(|node_index| SchemaNodeRef {
                forest: self.forest,
                module_index: self.module_index,
                node_index,
            })
    }
}

/// Owned compiled module forest stored inside a frozen `Context`.
#[derive(Debug, Clone, Default, PartialEq, Eq)]
pub(crate) struct SchemaForest {
    modules: Vec<ModuleData>,
    identity_map: HashMap<String, (usize, usize)>,
    schema_ptr_map: HashMap<*const ::std::os::raw::c_void, (usize, usize)>,
}

#[derive(Debug, Clone, PartialEq, Eq)]
struct ModuleData {
    info: RawModuleInfo,
    root: usize,
    nodes: Vec<SchemaNodeData>,
    identities: Vec<IdentityData>,
}

impl SchemaForest {
    /// Find a compiled identity by its fully-qualified name (`module:name` or
    /// just `name` if unambiguous).
    fn find_identity(&self, full_name: &str) -> Option<Identity<'_>> {
        let (module_index, identity_index) = *self.identity_map.get(full_name)?;
        Some(Identity {
            forest: self,
            module_index,
            identity_index,
        })
    }

    fn deviations_for_node(&self, module_index: usize, node_index: usize) -> Vec<Deviation> {
        let root = self.modules[module_index].root;
        let Some(node_segments) = self.path_segments(module_index, root, node_index) else {
            return Vec::new();
        };
        if node_segments.is_empty() {
            return Vec::new();
        }

        let mut out = Vec::new();
        for (source_module_index, module) in self.modules.iter().enumerate() {
            for raw in &module.info.deviations {
                if self.deviation_matches_node(
                    source_module_index,
                    raw,
                    module_index,
                    &node_segments,
                ) {
                    out.push(Deviation::from_raw(raw));
                }
            }
        }
        out
    }

    fn deviation_matches_node(
        &self,
        source_module_index: usize,
        raw: &RawDeviation,
        module_index: usize,
        node_segments: &[String],
    ) -> bool {
        let Some((target_module_index, target_segments)) =
            self.resolve_deviation_target_path(source_module_index, &raw.target_path)
        else {
            return false;
        };
        target_module_index == module_index && target_segments == node_segments
    }

    fn resolve_deviation_target_path(
        &self,
        source_module_index: usize,
        path: &str,
    ) -> Option<(usize, Vec<String>)> {
        let path = path.strip_prefix('/').unwrap_or(path);
        let mut target_module_index = source_module_index;
        let mut segments = Vec::new();

        for (pos, segment) in path
            .split('/')
            .filter(|segment| !segment.is_empty())
            .enumerate()
        {
            let (prefix, local_name) = segment
                .split_once(':')
                .map(|(prefix, local)| (Some(prefix), local))
                .unwrap_or((None, segment));
            if local_name.is_empty() {
                continue;
            }

            if pos == 0 {
                if let Some(prefix) = prefix {
                    target_module_index = self.resolve_prefix_index(source_module_index, prefix)?;
                } else {
                    let source = &self.modules[source_module_index].info;
                    if local_name == source.name || local_name == source.prefix {
                        continue;
                    }
                }
            }

            segments.push(local_name.to_string());
        }

        Some((target_module_index, segments))
    }

    fn resolve_prefix_index(&self, source_module_index: usize, prefix: &str) -> Option<usize> {
        let source = &self.modules[source_module_index].info;
        if prefix.is_empty() || prefix == source.prefix || prefix == source.name {
            return Some(source_module_index);
        }
        let import = source
            .imports
            .iter()
            .find(|import| import.prefix == prefix || import.name == prefix)?;
        self.module_index(&import.name)
    }

    /// Add a module tree from the adapter into the forest, flattening children
    /// into index-based structures for borrowed-handle safety.
    pub(crate) fn add_module(&mut self, info: RawModuleInfo, root: RawSchemaNode) {
        let mut nodes = Vec::new();
        let root_idx = Self::add_node(&mut nodes, root);
        let module_index = self.modules.len();
        let identities: Vec<_> = info
            .identities
            .iter()
            .map(|raw| IdentityData {
                name: raw.name.clone(),
                module_name: raw.module_name.clone(),
                base_indices: Vec::new(),
                derived_names: raw.derived.clone(),
            })
            .collect();
        for (identity_index, id) in identities.iter().enumerate() {
            let full_name = if id.module_name.is_empty() {
                id.name.clone()
            } else {
                format!("{}:{}", id.module_name, id.name)
            };
            self.identity_map
                .insert(full_name, (module_index, identity_index));
        }
        for (node_index, node) in nodes.iter().enumerate() {
            if !node.schema_ptr.is_null() {
                self.schema_ptr_map
                    .insert(node.schema_ptr, (module_index, node_index));
            }
        }
        self.modules.push(ModuleData {
            info,
            root: root_idx,
            nodes,
            identities,
        });
    }

    fn resolve_type<'a>(
        &'a self,
        _module_index: usize,
        raw: &cambium_libyang_sys::adapter::RawTypeInfo,
    ) -> TypeInfo<'a> {
        let base = BaseType::from_raw(raw.base_type);
        let range = || {
            if raw.range.is_empty() {
                None
            } else {
                Some(
                    raw.range
                        .iter()
                        .map(|r| RangeBound {
                            min: r.min.clone(),
                            max: r.max.clone(),
                        })
                        .collect(),
                )
            }
        };
        let length = || {
            if raw.length.is_empty() {
                None
            } else {
                Some(
                    raw.length
                        .iter()
                        .map(|r| RangeBound {
                            min: r.min.clone(),
                            max: r.max.clone(),
                        })
                        .collect(),
                )
            }
        };
        let patterns = raw
            .patterns
            .iter()
            .map(|p| Pattern {
                regex: p.regex.clone(),
                error_app_tag: p.error_app_tag.clone(),
                inverted: p.inverted,
            })
            .collect();
        let resolved = match base {
            BaseType::Int8
            | BaseType::Int16
            | BaseType::Int32
            | BaseType::Int64
            | BaseType::Uint8
            | BaseType::Uint16
            | BaseType::Uint32
            | BaseType::Uint64 => ResolvedType::Int {
                kind: IntKind::from_base(base).unwrap_or(IntKind::I32),
                range: range(),
            },
            BaseType::Decimal64 => ResolvedType::Decimal64 {
                fraction_digits: raw
                    .fraction_digits
                    .and_then(FractionDigits::new)
                    .unwrap_or(FractionDigits(1)),
                range: range(),
            },
            BaseType::String => ResolvedType::StringType {
                length: length(),
                patterns,
            },
            BaseType::Binary => ResolvedType::Binary { length: length() },
            BaseType::Enumeration => ResolvedType::Enumeration(EnumDef {
                values: raw
                    .enum_values
                    .iter()
                    .map(|v| EnumValue {
                        name: v.name.clone(),
                        value: v.value,
                    })
                    .collect(),
            }),
            BaseType::Bits => ResolvedType::Bits(BitsDef {
                values: raw
                    .bit_values
                    .iter()
                    .map(|v| EnumValue {
                        name: v.name.clone(),
                        value: v.value,
                    })
                    .collect(),
            }),
            BaseType::IdentityRef => {
                let bases: Vec<_> = raw
                    .identity_bases
                    .iter()
                    .filter_map(|name| self.find_identity(name))
                    .collect();
                ResolvedType::IdentityRef { bases }
            }
            BaseType::InstanceIdentifier => ResolvedType::InstanceIdentifier {
                require_instance: raw.require_instance.unwrap_or(true),
            },
            BaseType::LeafRef => ResolvedType::LeafRef {
                path: raw.leafref_path.clone(),
                target: if raw.leafref_target_schema.is_null() {
                    None
                } else {
                    self.schema_ref_by_ptr(raw.leafref_target_schema)
                        .map(Box::new)
                },
                realtype: raw
                    .leafref_realtype
                    .as_ref()
                    .map(|t| Box::new(self.resolve_type(_module_index, t))),
                require_instance: raw.require_instance.unwrap_or(true),
            },
            BaseType::Union => ResolvedType::Union(
                raw.union_types
                    .iter()
                    .map(|t| self.resolve_type(_module_index, t))
                    .collect(),
            ),
            BaseType::Boolean => ResolvedType::Boolean,
            BaseType::Empty => ResolvedType::Empty,
            _ => ResolvedType::Unknown,
        };
        TypeInfo {
            base,
            typedef_name: raw.typedef_name.clone(),
            resolved,
        }
    }

    fn add_node(nodes: &mut Vec<SchemaNodeData>, mut raw: RawSchemaNode) -> usize {
        let raw_children = std::mem::take(&mut raw.children);
        let mut data = SchemaNodeData::from_raw(raw);
        let mut child_indices = Vec::with_capacity(raw_children.len());
        for child in raw_children {
            child_indices.push(Self::add_node(nodes, child));
        }
        data.children = child_indices;
        let idx = nodes.len();
        nodes.push(data);
        idx
    }

    pub(crate) fn module_index(&self, name: &str) -> Option<usize> {
        self.modules
            .iter()
            .position(|m| m.info.name == name || m.info.namespace == name)
    }

    pub(crate) fn modules(&self) -> Vec<Module<'_>> {
        self.modules
            .iter()
            .enumerate()
            .filter(|(_, module)| module.info.is_implemented)
            .map(|(module_index, _)| Module {
                forest: self,
                module_index,
            })
            .collect()
    }

    fn module_children_by_kind(
        &self,
        module_index: usize,
        kind: SchemaNodeKind,
    ) -> SchemaChildren<'_> {
        let root = self.modules[module_index].root;
        let indices = self.modules[module_index].nodes[root]
            .children
            .iter()
            .copied()
            .filter(|&idx| self.modules[module_index].nodes[idx].kind == kind)
            .collect();
        SchemaChildren {
            forest: self,
            module_index,
            indices,
            pos: 0,
        }
    }

    fn parent_index(&self, module_index: usize, target_index: usize) -> Option<usize> {
        let root = self.modules[module_index].root;
        if target_index == root {
            return None;
        }
        self.parent_index_from(module_index, root, target_index)
    }

    fn parent_index_from(
        &self,
        module_index: usize,
        current_index: usize,
        target_index: usize,
    ) -> Option<usize> {
        let children = &self.modules[module_index].nodes[current_index].children;
        if children.contains(&target_index) {
            return Some(current_index);
        }
        children
            .iter()
            .find_map(|&child| self.parent_index_from(module_index, child, target_index))
    }

    fn ancestor_indices(&self, module_index: usize, node_index: usize) -> Vec<usize> {
        let root = self.modules[module_index].root;
        let mut current = node_index;
        let mut ancestors = Vec::new();
        while let Some(parent) = self.parent_index(module_index, current) {
            if parent == root {
                break;
            }
            ancestors.push(parent);
            current = parent;
        }
        ancestors.reverse();
        ancestors
    }

    /// Look up a `SchemaNodeRef` from a raw compiled schema pointer.
    pub(crate) fn schema_ref_by_ptr(
        &self,
        ptr: *const ::std::os::raw::c_void,
    ) -> Option<SchemaNodeRef<'_>> {
        let (module_index, node_index) = *self.schema_ptr_map.get(&ptr)?;
        Some(SchemaNodeRef {
            forest: self,
            module_index,
            node_index,
        })
    }

    pub(crate) fn find_path(&self, module_index: usize, path: &str) -> Option<SchemaNodeRef<'_>> {
        let module = self.modules.get(module_index)?;
        let path = path.strip_prefix('/').unwrap_or(path);
        let mut cur = module.root;
        for (pos, segment) in path.split('/').enumerate() {
            if segment.is_empty() {
                continue;
            }
            let name = segment
                .split_once(':')
                .map(|(_prefix, local)| local)
                .unwrap_or(segment);
            if name.is_empty() {
                continue;
            }
            if pos == 0 && (name == module.info.name || name == module.info.prefix) {
                continue;
            }
            let idx = self.modules[module_index].nodes[cur]
                .children
                .iter()
                .copied()
                .find(|&i| self.modules[module_index].nodes[i].name == name)?;
            cur = idx;
        }
        Some(SchemaNodeRef {
            forest: self,
            module_index,
            node_index: cur,
        })
    }

    fn collect_data_children(
        &self,
        module_index: usize,
        parent_index: usize,
        flatten_choices: bool,
        out: &mut Vec<usize>,
    ) {
        for &child_index in &self.modules[module_index].nodes[parent_index].children {
            let kind = self.modules[module_index].nodes[child_index].kind;
            if is_operation_kind(kind) {
                continue;
            }
            if flatten_choices && matches!(kind, SchemaNodeKind::Choice | SchemaNodeKind::Case) {
                self.collect_data_children(module_index, child_index, true, out);
                continue;
            }
            out.push(child_index);
        }
    }

    fn path_segments(
        &self,
        module_index: usize,
        current_index: usize,
        target_index: usize,
    ) -> Option<Vec<String>> {
        if current_index == target_index {
            return Some(Vec::new());
        }
        for &child_index in &self.modules[module_index].nodes[current_index].children {
            if let Some(mut path) = self.path_segments(module_index, child_index, target_index) {
                path.insert(
                    0,
                    self.modules[module_index].nodes[child_index].name.clone(),
                );
                return Some(path);
            }
        }
        None
    }
}

fn is_operation_kind(kind: SchemaNodeKind) -> bool {
    matches!(
        kind,
        SchemaNodeKind::Rpc
            | SchemaNodeKind::Action
            | SchemaNodeKind::Input
            | SchemaNodeKind::Output
            | SchemaNodeKind::Notification
    )
}
