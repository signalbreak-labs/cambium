//! This file must fail to compile: `UserOrderedVec<T>` has no order-agnostic `swap`.

include!("_generated_user_ordered_vec.rs");

fn misuse(mut top: OrderingNestedUserCascadingTop) {
    // There is deliberately no `swap` method on `UserOrderedVec<T>`.
    top.statement.swap(0, 1);
}

fn main() {}
