//! # Cambium — order-correct YANG for Rust
//!
//! Order is a **structural property of the tree**: children are an ordered
//! sequence (the source of truth); any keyed index is a derived lookup cache,
//! never iterated for serialization. `ordered-by user` lists are a distinct
//! positional type whose only mutators are positional — misusing them on a
//! system-ordered node is a compile error.
#![deny(missing_docs)]

pub use cambium_core::tree;
pub use cambium_core::{
    BaseType, BitsDef, Config, Context, ContextBuilder, ContextFlags, DataDiff, DataSiblings,
    DataTree, Decimal64, Deviation, DiffEdit, DiffOp, DiffOpts, EnumDef, EnumValue, Error,
    Extension, Format, FractionDigits, Identity, Import, IntKind, MergeOpts, Module,
    MustConstraint, NodeRef, NodeSet, OpType, OrderedBy, ParseMode, Pattern, RangeBound,
    ResolvedType, Result, RuleCode, SchemaNode, SchemaNodeKind, SchemaNodeRef, SerializeFlags,
    Status, TypeInfo, UniqueConstraint, ValidateMode, Value, WhenConstraint, WithDefaults,
};

/// Commonly used types.
pub mod prelude {
    pub use cambium_core::{
        BaseType, BitsDef, Config, Context, ContextBuilder, ContextFlags, DataDiff, DataSiblings,
        DataTree, Decimal64, Deviation, DiffEdit, DiffOp, DiffOpts, EnumDef, EnumValue, Error,
        Extension, Format, FractionDigits, Identity, Import, IntKind, MergeOpts, Module,
        MustConstraint, NodeRef, NodeSet, OpType, OrderedBy, ParseMode, Pattern, RangeBound,
        ResolvedType, Result, RuleCode, SchemaNode, SchemaNodeKind, SchemaNodeRef, SerializeFlags,
        Status, TypeInfo, UniqueConstraint, ValidateMode, Value, WhenConstraint, WithDefaults,
    };
}
