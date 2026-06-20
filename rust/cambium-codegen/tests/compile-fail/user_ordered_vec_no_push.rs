//! This file must fail to compile: `UserOrderedVec<T>` has no order-agnostic `push`.

include!("_generated_user_ordered_vec.rs");

fn misuse(mut top: OrderingNestedUserCascadingTop) {
    // There is deliberately no `push` method on `UserOrderedVec<T>`.
    top.statement.push(Default::default());
}

fn main() {}
