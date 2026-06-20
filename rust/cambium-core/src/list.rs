//! Positional-only `ordered-by user` list manipulation.
//!
//! `UserOrderedList` deliberately has **no** order-agnostic setter (no `set`,
//! no `push`, no index assignment). Reordering a system-ordered node by mistake
//! is therefore a compile error — the method does not exist.

use std::marker::PhantomData;

use cambium_libyang_sys::adapter::{RawUserOrderedLeafList, RawUserOrderedList};

use crate::error::{Error, Result, RuleCode};
use crate::tree::{DataTree, NodeRef};

/// A handle to an `ordered-by user` list.
///
/// The handle borrows the parent `DataTree` mutably; all positional operations
/// mutate that tree in place.
#[derive(Debug)]
pub struct UserOrderedList<'a, 'ctx: 'a> {
    raw: RawUserOrderedList,
    tree: *mut DataTree<'ctx>,
    _marker: PhantomData<&'a mut DataTree<'ctx>>,
}

impl<'a, 'ctx: 'a> UserOrderedList<'a, 'ctx> {
    /// Wrap a raw list pointer. The pointer must belong to the borrowed tree.
    pub(crate) unsafe fn from_raw(
        tree: &'a mut DataTree<'ctx>,
        list: *mut ::std::os::raw::c_void,
    ) -> Self {
        Self {
            raw: unsafe { RawUserOrderedList::from_raw(list.cast()) },
            tree: tree as *mut DataTree<'ctx>,
            _marker: PhantomData,
        }
    }

    fn tree_ref(&self) -> &'a DataTree<'ctx> {
        // SAFETY: the lifetime `'a` is exactly the mutable borrow of the tree
        // that this handle represents.
        unsafe { &*self.tree }
    }

    /// Number of list entries.
    pub fn len(&self) -> usize {
        self.raw.len()
    }

    /// True if there are no list entries.
    pub fn is_empty(&self) -> bool {
        self.raw.is_empty()
    }

    /// Get the entry at `index`.
    pub fn get(&self, index: usize) -> Option<NodeRef<'a>> {
        let path = self.raw.path_at(index)?;
        Some(NodeRef::new(self.tree_ref(), path))
    }

    /// Iterate over the entries in insertion order.
    pub fn iter(&self) -> impl Iterator<Item = NodeRef<'a>> {
        let tree = self.tree_ref();
        self.raw
            .paths()
            .into_iter()
            .map(move |path| NodeRef::new(tree, path))
    }

    /// Find the index of the entry whose composite key matches `keys`.
    ///
    /// `keys` is a slice of `(key-name, key-value)` pairs in any order; they
    /// are matched against the schema key leaves of each entry.
    pub fn find_by_key(&self, keys: &[(&str, &str)]) -> Option<usize> {
        for (i, path) in self.raw.paths().iter().enumerate() {
            let node = NodeRef::new(self.tree_ref(), path.clone());
            let children = node.children().ok()?;
            let mut matched = 0;
            for (k, v) in keys {
                if children.iter().any(|child| {
                    child.name() == *k && child.value_str().ok().flatten().as_deref() == Some(*v)
                }) {
                    matched += 1;
                }
            }
            if matched == keys.len() {
                return Some(i);
            }
        }
        None
    }

    /// Remove the entry at `index`.
    pub fn remove(&mut self, index: usize) -> Result<()> {
        self.raw
            .remove(index)
            .map_err(|e| Error::ffi(RuleCode::OrderedList, e))
    }

    /// Insert `entry` as the first entry.
    pub fn insert_first(&mut self, entry: DataTree<'ctx>) -> Result<()> {
        self.raw
            .insert_first(entry.raw)
            .map_err(|e| Error::ffi(RuleCode::OrderedList, e))
    }

    /// Insert `entry` as the last entry.
    pub fn insert_last(&mut self, entry: DataTree<'ctx>) -> Result<()> {
        self.raw
            .insert_last(entry.raw)
            .map_err(|e| Error::ffi(RuleCode::OrderedList, e))
    }

    /// Insert `entry` before the entry at `index`.
    pub fn insert_before(&mut self, index: usize, entry: DataTree<'ctx>) -> Result<()> {
        self.raw
            .insert_before(index, entry.raw)
            .map_err(|e| Error::ffi(RuleCode::OrderedList, e))
    }

    /// Insert `entry` after the entry at `index`.
    pub fn insert_after(&mut self, index: usize, entry: DataTree<'ctx>) -> Result<()> {
        self.raw
            .insert_after(index, entry.raw)
            .map_err(|e| Error::ffi(RuleCode::OrderedList, e))
    }

    /// Move the entry at `what` before the entry at `point`.
    pub fn move_before(&mut self, what: usize, point: usize) -> Result<()> {
        self.raw
            .move_before(what, point)
            .map_err(|e| Error::ffi(RuleCode::OrderedList, e))
    }

    /// Move the entry at `what` after the entry at `point`.
    pub fn move_after(&mut self, what: usize, point: usize) -> Result<()> {
        self.raw
            .move_after(what, point)
            .map_err(|e| Error::ffi(RuleCode::OrderedList, e))
    }
}

/// A **read-only** positional view of an `ordered-by user` list, obtained from a
/// shared [`NodeRef`] via [`NodeRef::as_user_ordered`](crate::tree::NodeRef::as_user_ordered).
///
/// It exposes only the read side (`len`/`get`/`iter`/`find_by_key`). Mutation
/// (insert/move/remove) requires a `&mut DataTree` handle from
/// [`DataTree::user_ordered_list_at`](crate::tree::DataTree::user_ordered_list_at),
/// so reordering can never happen behind a shared borrow.
#[derive(Debug)]
pub struct UserOrderedView<'tree, 'ctx: 'tree> {
    raw: RawUserOrderedList,
    tree: &'tree DataTree<'ctx>,
}

impl<'tree, 'ctx: 'tree> UserOrderedView<'tree, 'ctx> {
    /// Wrap a raw list pointer borrowed immutably from `tree`.
    ///
    /// # Safety
    ///
    /// `list` must address an `ordered-by user` list node owned by `tree`.
    pub(crate) unsafe fn from_raw(
        tree: &'tree DataTree<'ctx>,
        list: *mut ::std::os::raw::c_void,
    ) -> Self {
        Self {
            raw: unsafe { RawUserOrderedList::from_raw(list.cast()) },
            tree,
        }
    }

    /// Number of list entries.
    pub fn len(&self) -> usize {
        self.raw.len()
    }

    /// True if there are no list entries.
    pub fn is_empty(&self) -> bool {
        self.raw.is_empty()
    }

    /// Get the entry at `index`.
    pub fn get(&self, index: usize) -> Option<NodeRef<'tree>> {
        let path = self.raw.path_at(index)?;
        Some(NodeRef::new(self.tree, path))
    }

    /// Iterate over the entries in insertion order.
    pub fn iter(&self) -> impl Iterator<Item = NodeRef<'tree>> {
        let tree = self.tree;
        self.raw
            .paths()
            .into_iter()
            .map(move |path| NodeRef::new(tree, path))
    }

    /// Find the index of the entry whose composite key matches `keys`.
    ///
    /// `keys` is a slice of `(key-name, key-value)` pairs in any order.
    pub fn find_by_key(&self, keys: &[(&str, &str)]) -> Option<usize> {
        for (i, path) in self.raw.paths().iter().enumerate() {
            let node = NodeRef::new(self.tree, path.clone());
            let children = node.children().ok()?;
            let mut matched = 0;
            for (k, v) in keys {
                if children.iter().any(|child| {
                    child.name() == *k && child.value_str().ok().flatten().as_deref() == Some(*v)
                }) {
                    matched += 1;
                }
            }
            if matched == keys.len() {
                return Some(i);
            }
        }
        None
    }
}

/// A handle to an `ordered-by user` leaf-list.
///
/// Like `UserOrderedList`, this type is positional-only: there is no `set`,
/// no `push`, and no index assignment.
#[derive(Debug)]
pub struct UserOrderedLeafList<'a, 'ctx: 'a> {
    raw: RawUserOrderedLeafList,
    _marker: PhantomData<&'a mut DataTree<'ctx>>,
}

impl<'a, 'ctx: 'a> UserOrderedLeafList<'a, 'ctx> {
    /// Wrap a raw leaf-list pointer. The pointer must belong to the borrowed tree.
    pub(crate) unsafe fn from_raw(
        _tree: &'a mut DataTree<'ctx>,
        list: *mut ::std::os::raw::c_void,
    ) -> Self {
        Self {
            raw: unsafe { RawUserOrderedLeafList::from_raw(list.cast()) },
            _marker: PhantomData,
        }
    }

    /// Number of leaf-list values.
    pub fn len(&self) -> usize {
        self.raw.len()
    }

    /// True if there are no leaf-list values.
    pub fn is_empty(&self) -> bool {
        self.raw.is_empty()
    }

    /// Get the value at `index`.
    pub fn get(&self, index: usize) -> Option<String> {
        self.raw.value_at(index)
    }

    /// Iterate over the values in insertion order.
    pub fn iter(&self) -> impl Iterator<Item = String> + '_ {
        self.raw.values().into_iter()
    }

    /// Insert `value` as the first leaf-list instance.
    pub fn insert_first(&mut self, value: &str) -> Result<()> {
        self.raw
            .insert_first(value)
            .map_err(|e| Error::ffi(RuleCode::OrderedList, e))
    }

    /// Insert `value` as the last leaf-list instance.
    pub fn insert_last(&mut self, value: &str) -> Result<()> {
        self.raw
            .insert_last(value)
            .map_err(|e| Error::ffi(RuleCode::OrderedList, e))
    }

    /// Insert `value` before the instance at `index`.
    pub fn insert_before(&mut self, index: usize, value: &str) -> Result<()> {
        self.raw
            .insert_before(index, value)
            .map_err(|e| Error::ffi(RuleCode::OrderedList, e))
    }

    /// Insert `value` after the instance at `index`.
    pub fn insert_after(&mut self, index: usize, value: &str) -> Result<()> {
        self.raw
            .insert_after(index, value)
            .map_err(|e| Error::ffi(RuleCode::OrderedList, e))
    }

    /// Move the instance at `what` before the instance at `point`.
    pub fn move_before(&mut self, what: usize, point: usize) -> Result<()> {
        self.raw
            .move_before(what, point)
            .map_err(|e| Error::ffi(RuleCode::OrderedList, e))
    }

    /// Move the instance at `what` after the instance at `point`.
    pub fn move_after(&mut self, what: usize, point: usize) -> Result<()> {
        self.raw
            .move_after(what, point)
            .map_err(|e| Error::ffi(RuleCode::OrderedList, e))
    }

    /// Remove the instance at `index`.
    pub fn remove(&mut self, index: usize) -> Result<()> {
        self.raw
            .remove(index)
            .map_err(|e| Error::ffi(RuleCode::OrderedList, e))
    }
}
