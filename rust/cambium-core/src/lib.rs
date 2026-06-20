//! Safe core of Cambium: ordered YANG data trees and schema trees.
//!
//! This crate imports **zero** libyang/cgo types. The FFI adapter lives in
//! `cambium-libyang-sys`.
#![deny(missing_docs)]

pub mod context;
pub mod error;
pub mod list;
pub mod schema;
mod sys_consts;
pub mod tree;

pub use context::{Context, ContextBuilder, ContextFlags};
pub use error::{Diagnostic, Error, ErrorType, Result, RuleCode, ValidationCode, ValidationErrors};
pub use list::{UserOrderedLeafList, UserOrderedList, UserOrderedView};
pub use schema::{
    BaseType, BitsDef, Config, Deviation, EnumDef, EnumValue, Extension, FractionDigits, Identity,
    Import, IntKind, LeafType, Module, MustConstraint, OrderedBy, Pattern, RangeBound,
    ResolvedType, SchemaNode, SchemaNodeKind, SchemaNodeRef, SchemaTree, Status, TypeInfo,
    UniqueConstraint, WhenConstraint,
};
pub use tree::{
    DataDiff, DataSiblings, DataTree, Decimal64, DiffEdit, DiffOp, DiffOpts, Format, ImplicitOpts,
    MergeOpts, NewPathOpts, NodeAddr, NodeRef, NodeSet, OpType, ParseMode, SerializeFlags,
    ValidateMode, Value, WithDefaults,
};
