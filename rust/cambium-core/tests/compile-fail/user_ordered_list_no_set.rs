//! This file must fail to compile: `UserOrderedList` has no order-agnostic setter.

use cambium_core::{Context, DataTree, UserOrderedList};

fn misuse(ctx: &Context, mut list: UserOrderedList<'_, '_>) {
    // There is deliberately no `set` method on `UserOrderedList`.
    let entry: DataTree<'_> = ctx.parse(cambium_core::Format::Xml, cambium_core::ParseMode::data_only(), b"<x/>").unwrap();
    list.set(entry);
}

fn main() {}
