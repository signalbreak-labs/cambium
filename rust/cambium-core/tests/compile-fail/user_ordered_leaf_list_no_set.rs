//! This file must fail to compile: `UserOrderedLeafList` has no order-agnostic setter.

use cambium_core::{Context, UserOrderedLeafList};

fn misuse(_ctx: &Context, mut list: UserOrderedLeafList<'_, '_>) {
    // There is deliberately no `set` method on `UserOrderedLeafList`.
    list.set("x");
}

fn main() {}
