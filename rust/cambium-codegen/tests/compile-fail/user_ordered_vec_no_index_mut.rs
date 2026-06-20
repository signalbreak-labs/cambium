//! This file must fail to compile: `UserOrderedVec<T>` has no mutable indexing.

include!("_generated_user_ordered_vec.rs");

fn misuse(mut top: OrderingNestedUserCascadingTop) {
    // There is deliberately no `IndexMut` implementation for `UserOrderedVec<T>`.
    top.statement[0] = Default::default();
}

fn main() {}
