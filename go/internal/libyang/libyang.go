// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

// Package libyang is Cambium's internal cgo FFI over a vendored, statically
// linked libyang + PCRE2 (built by build.sh into .build/). It is the ONLY place
// in the Go tree that touches C; the public github.com/.../go/cambium package
// imports zero cgo so the hexagonal core stays free of libyang types.
//
// Calls are coarse-grained by mandate: a whole document is parsed / serialized /
// validated per cgo call. There is no per-node cgo and no C->Go callback in any
// hot path — exactly mirroring rust/cambium-libyang-sys/src/adapter.rs.
//
//go:build cgo

package libyang

/*
#cgo CFLAGS: -I${SRCDIR}/.build/libyang-install/include -I${SRCDIR}/.build/pcre2-install/include
#cgo LDFLAGS: ${SRCDIR}/.build/libyang-install/lib/libyang.a ${SRCDIR}/.build/pcre2-install/lib/libpcre2-8.a -lm -lpthread
#cgo linux LDFLAGS: -ldl

#include <stdlib.h>
#include <libyang/libyang.h>
#include <libyang/metadata.h>

// lyd_parent is a function-like macro in libyang, so it cannot be called from
// cgo directly. Wrap it (and normalize the return to a generic node pointer).
static struct lyd_node *cam_lyd_parent(struct lyd_node *node) {
	return node ? (struct lyd_node *)node->parent : NULL;
}

// cam_lyd_schema_name returns the schema-node name of a data node, used to
// filter a parent's children down to one list's entries.
static const char *cam_lyd_schema_name(const struct lyd_node *node) {
	return (node && node->schema) ? node->schema->name : NULL;
}

// lyd_child and lyd_get_value are static inline in tree_data.h; wrap them so
// Go calls a stable C function and so node-type gating lives in one place.
static struct lyd_node *cam_lyd_child(const struct lyd_node *node) {
	return lyd_child(node);
}

static const char *cam_lyd_get_value(const struct lyd_node *node) {
	return lyd_get_value(node);
}

static const char *cam_lyd_get_meta_value(const struct lyd_meta *meta) {
	return lyd_get_meta_value(meta);
}

// Canonical metadata name constants. Returned by stable C helpers so the
// NUL-terminated strings have process lifetime and are safe to pass into
// libyang metadata lookups.
static const char *cam_cstr_yang(void) { return "yang"; }
static const char *cam_cstr_operation(void) { return "operation"; }

// The ly_set dnodes member is a union; cgo sees it as opaque, so access it
// through a tiny helper. The set does not own the pointed-to nodes.
static struct lyd_node *cam_set_dnode(const struct ly_set *set, uint32_t i) {
	return set->dnodes[i];
}
*/
import "C" //nolint:gocritic // dupImport false positive: gocritic pairs the cgo "C" pseudo-import with "unsafe"

import (
	"fmt"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"unsafe" //nolint:gocritic // dupImport false positive: gocritic pairs "unsafe" with the cgo "C" pseudo-import
)

// validateLogMu protects the ctx-global libyang error-log options and the
// ly_err_item list during multi-error validation collection.
var validateLogMu sync.Mutex

// Parse / print / validate option bits, re-exported so the safe core can map
// its typed flags without importing C.
var (
	// Parse options.
	ParseOnly               = uint32(C.LYD_PARSE_ONLY)
	ParseStrict             = uint32(C.LYD_PARSE_STRICT)
	ParseOpaque             = uint32(C.LYD_PARSE_OPAQ)
	ParseNoState            = uint32(C.LYD_PARSE_NO_STATE)
	ParseLybSkipModuleCheck = uint32(C.LYD_PARSE_LYB_SKIP_MODULE_CHECK)

	// Print options.
	PrintWithSiblings = uint32(C.LYD_PRINT_SIBLINGS)
	PrintShrink       = uint32(C.LYD_PRINT_SHRINK)
	PrintEmptyCont    = uint32(C.LYD_PRINT_EMPTY_CONT)
	PrintWDExplicit   = uint32(C.LYD_PRINT_WD_EXPLICIT)
	PrintWDTrim       = uint32(C.LYD_PRINT_WD_TRIM)
	PrintWDAll        = uint32(C.LYD_PRINT_WD_ALL)
	PrintWDAllTag     = uint32(C.LYD_PRINT_WD_ALL_TAG)

	// Validate options.
	ValidateNoState    = uint32(C.LYD_VALIDATE_NO_STATE)
	ValidatePresent    = uint32(C.LYD_VALIDATE_PRESENT)
	ValidateMultiError = uint32(C.LYD_VALIDATE_MULTI_ERROR)

	// new_path options.
	NewPathUpdate = uint32(C.LYD_NEW_PATH_UPDATE)
	NewPathOutput = uint32(C.LYD_NEW_VAL_OUTPUT)
	NewPathOpaque = uint32(C.LYD_NEW_PATH_OPAQ)

	// add_defaults options.
	ImplicitNoState = uint32(C.LYD_IMPLICIT_NO_STATE)
	ImplicitOutput  = uint32(C.LYD_IMPLICIT_OUTPUT)
)

// Format is a libyang wire format.
type Format int

const (
	// FormatXML is RFC 7950 XML.
	FormatXML Format = iota
	// FormatJSON is RFC 7951 JSON.
	FormatJSON
	// FormatJSONIETF is RFC 7951 JSON as used by gNMI JSON_IETF.
	FormatJSONIETF
	// FormatLYB is libyang's binary LYB format.
	FormatLYB
)

func (f Format) c() C.LYD_FORMAT {
	switch f {
	case FormatJSON, FormatJSONIETF:
		return C.LYD_JSON
	case FormatLYB:
		return C.LYD_LYB
	default:
		return C.LYD_XML
	}
}

// ParseOptions is a bitset of LYD_PARSE_* flags.
type ParseOptions uint32

// OpType selects the kind of operation document for ParseOp.
type OpType int

const (
	// OpRPC is a YANG RPC.
	OpRPC OpType = iota
	// OpNotification is a YANG notification.
	OpNotification
	// OpReply is a YANG RPC/action reply.
	OpReply
)

func (t OpType) c() C.enum_lyd_type {
	switch t {
	case OpNotification:
		return C.LYD_TYPE_NOTIF_YANG
	case OpReply:
		return C.LYD_TYPE_REPLY_YANG
	default:
		return C.LYD_TYPE_RPC_YANG
	}
}

// RawContext owns a libyang ly_ctx. Build-once-then-frozen: set search paths and
// load modules during setup, then parse. Free with Close (a finalizer is a
// backstop).
//
// A data tree references the context's schema and dictionary, so the context
// must outlive every tree/diff created from it. To make that safe even when a
// caller Closes the context before its trees are collected, the context holds a
// live-tree refcount: ly_ctx_destroy is deferred until the last tree/diff is
// freed (retain/release). Without this, a tree finalizer running after an
// explicit Close would lyd_free_all over freed schema/dictionary memory.
type RawContext struct {
	ctx       *C.struct_ly_ctx
	live      int64 // outstanding RawDataTree/RawDataDiff that free ctx-owned data
	closeReq  int32 // Close() requested
	destroyed int32 // ly_ctx_destroy performed (exactly-once)
}

// NewContext creates an empty context with no search path.
func NewContext() (*RawContext, error) {
	var ctx *C.struct_ly_ctx
	rc := C.ly_ctx_new(nil, C.uint32_t(C.LY_CTX_NO_YANGLIBRARY|C.LY_CTX_LEAFREF_EXTENDED|C.LY_CTX_SET_PRIV_PARSED), &ctx) //nolint:gocritic // dupSubExpr false positive on cgo-rewritten call
	if rc != C.LY_SUCCESS {
		return nil, fmt.Errorf("ly_ctx_new failed: rc=%d", int(rc))
	}
	if ctx == nil {
		return nil, fmt.Errorf("ly_ctx_new returned null")
	}
	c := &RawContext{ctx: ctx}
	runtime.SetFinalizer(c, (*RawContext).finalize)
	return c, nil
}

// SetSearchPath appends a directory to the module search path.
func (c *RawContext) SetSearchPath(path string) error {
	cpath := C.CString(path)
	defer C.free(unsafe.Pointer(cpath))
	rc := C.ly_ctx_set_searchdir(c.ctx, cpath)
	if rc != C.LY_SUCCESS {
		return lyError(c.ctx, fmt.Sprintf("set search path %q", path), int(rc))
	}
	return nil
}

// LoadModule loads a YANG module (all features) into the context.
func (c *RawContext) LoadModule(name string) error {
	cname := C.CString(name)
	defer C.free(unsafe.Pointer(cname))
	mod := C.ly_ctx_load_module(c.ctx, cname, nil, nil)
	if mod == nil {
		return lyError(c.ctx, fmt.Sprintf("load module %q", name), 0)
	}
	return nil
}

// checkNul rejects interior NUL bytes to match Rust CString::new parity.
func checkNul(data []byte) error {
	for i, b := range data {
		if b == 0 {
			return fmt.Errorf("input contains interior NUL byte at offset %d", i)
		}
	}
	return nil
}

// ParseData parses a whole data document in a single cgo call.
func (c *RawContext) ParseData(format Format, parseOptions uint32, data []byte) (*RawDataTree, error) {
	var keepAlive []byte
	var cdata *C.char
	if format == FormatLYB {
		keepAlive = append(append([]byte(nil), data...), 0)
		cdata = (*C.char)(unsafe.Pointer(&keepAlive[0]))
	} else {
		if err := checkNul(data); err != nil { //nolint:gocritic // uncheckedInlineErr false positive on cgo-rewritten body
			return nil, err
		}
		cdata = C.CString(string(data))
		defer C.free(unsafe.Pointer(cdata))
	}
	var tree *C.struct_lyd_node
	rc := C.lyd_parse_data_mem(c.ctx, cdata, format.c(), C.uint32_t(parseOptions), 0, &tree) //nolint:gocritic // dupSubExpr false positive on cgo-rewritten call
	runtime.KeepAlive(keepAlive)
	if rc != C.LY_SUCCESS {
		return nil, lyError(c.ctx, "parse data", int(rc))
	}
	if tree == nil {
		return nil, fmt.Errorf("parse data returned a null tree")
	}
	return newRawDataTree(tree, c, c.ctx), nil
}

// ParseOp parses an RPC, action, or notification document.
func (c *RawContext) ParseOp(format Format, opType OpType, data []byte) (*RawDataTree, error) {
	if err := checkNul(data); err != nil { //nolint:gocritic // uncheckedInlineErr false positive on cgo-rewritten body
		return nil, err
	}
	cdata := C.CString(string(data))
	defer C.free(unsafe.Pointer(cdata))

	var in *C.struct_ly_in
	rc := C.ly_in_new_memory(cdata, &in) //nolint:gocritic // dupSubExpr false positive on cgo-rewritten call
	if rc != C.LY_SUCCESS {
		return nil, lyError(c.ctx, "ly_in_new_memory", int(rc))
	}
	defer C.ly_in_free(in, 0)

	var tree *C.struct_lyd_node
	var op *C.struct_lyd_node
	rc = C.lyd_parse_op(c.ctx, nil, in, format.c(), opType.c(), 0, &tree, &op) //nolint:gocritic // dupSubExpr false positive on cgo-rewritten call
	if rc != C.LY_SUCCESS {
		return nil, lyError(c.ctx, "parse op", int(rc))
	}
	// Return the full parsed tree so a nested action/notification keeps its
	// ancestor context (parent containers + list keys identifying the target),
	// per RFC 7950 §7.15.2 and matching the yanglint oracle. `op` is the
	// operation node only; fall back to it when there is no wrapping tree.
	node := tree
	if node == nil {
		node = op
	}
	if node == nil {
		return nil, fmt.Errorf("parse op returned no tree")
	}
	return newRawDataTree(node, c, c.ctx), nil
}

// NewData creates an empty in-memory data tree tied to this context.
func (c *RawContext) NewData() *RawDataTree {
	return newRawDataTree(nil, c, c.ctx)
}

// RawDataTree owns a libyang lyd_node tree. It keeps its owning RawContext
// alive so the context is never finalized while the tree is reachable.
// The ctx pointer is cached so mutation operations on an empty or detached
// tree can pass it to libyang without a public accessor on RawContext.
// Free with Close (finalizer backstop).
type RawDataTree struct {
	tree     *C.struct_lyd_node
	owner    *RawContext
	ctx      *C.struct_ly_ctx
	gen      uint64
	released int32 // free/release performed (exactly-once, atomic)
}

func newRawDataTree(tree *C.struct_lyd_node, owner *RawContext, ctx *C.struct_ly_ctx) *RawDataTree {
	owner.retain()
	t := &RawDataTree{tree: tree, owner: owner, ctx: ctx}
	runtime.SetFinalizer(t, (*RawDataTree).finalize)
	return t
}

// NewPath creates or updates a node at `path`. A nil `value` creates an inner
// node (container/list); a non-nil value sets a leaf or leaf-list. `options`
// is a bitmask of NewPathUpdate/NewPathOutput/NewPathOpaque.
func (t *RawDataTree) NewPath(path string, value *string, options uint32) error {
	defer runtime.KeepAlive(t)
	defer runtime.KeepAlive(t.owner)
	cpath := C.CString(path)
	defer C.free(unsafe.Pointer(cpath))

	var cvalue *C.char
	if value != nil {
		cvalue = C.CString(*value)
		defer C.free(unsafe.Pointer(cvalue))
	}

	var newParent, newNode *C.struct_lyd_node
	rc := C.lyd_new_path2(
		t.tree,
		t.ctx,
		cpath,
		unsafe.Pointer(cvalue),
		0, // value_size_bits: 0 for 0-terminated string values
		0, // any_hints
		C.uint32_t(options),
		&newParent,
		&newNode, //nolint:gocritic // dupSubExpr false positive on cgo-rewritten call
	)
	if rc != C.LY_SUCCESS {
		return lyError(t.ctx, "new path", int(rc))
	}

	// Re-anchor the canonical first sibling. libyang may insert a new top-level
	// node before the previous root, and an empty tree starts with t.tree == nil.
	if t.tree == nil {
		anchor := newParent
		if anchor == nil {
			anchor = newNode
		}
		if anchor != nil {
			t.tree = C.lyd_first_sibling(anchor)
		}
	} else {
		t.tree = C.lyd_first_sibling(t.tree)
	}

	t.incrementGen()
	return nil
}

// SetValue changes the value of an existing leaf or leaf-list.
// It returns true if the value (or default flag) changed, false if the value
// was identical. A non-leaf/leaf-list target or missing path returns an error.
func (t *RawDataTree) SetValue(path, value string) (bool, error) {
	defer runtime.KeepAlive(t)
	defer runtime.KeepAlive(t.owner)
	node, ok, err := t.FindNode(path)
	if err != nil {
		return false, err
	}
	if !ok || !nodeIsTerm(node) {
		return false, fmt.Errorf("set_value target is not a leaf/leaf-list: %q", path)
	}
	cvalue := C.CString(value)
	defer C.free(unsafe.Pointer(cvalue))

	rc := C.lyd_change_term(node, cvalue)
	switch rc {
	case C.LY_SUCCESS, C.LY_EEXIST:
		// LY_EEXIST means same value but default flag cleared: a real state change.
		t.incrementGen()
		return true, nil
	case C.LY_ENOT:
		// Identical value: deliberate no-op; do NOT bump the generation.
		return false, nil
	default:
		return false, lyError(t.ctx, "set value", int(rc))
	}
}

// RemovePath removes and frees the subtree at `path`.
func (t *RawDataTree) RemovePath(path string) error {
	defer runtime.KeepAlive(t)
	defer runtime.KeepAlive(t.owner)
	node, ok, err := t.FindNode(path)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("path not found: %q", path)
	}
	// libyang's lyd_free_tree silently refuses to free a list key (it logs and
	// returns void), which would make this a false success that still bumps the
	// generation. Reject key leaves deterministically before touching the tree —
	// the caller must remove the list entry, not its key.
	if node.schema != nil &&
		uint16(node.schema.nodetype) == C.LYS_LEAF &&
		(uint16(node.schema.flags)&uint16(C.LYS_KEY)) != 0 &&
		C.cam_lyd_parent(node) != nil {
		return fmt.Errorf("cannot remove a list key %q; remove the list entry instead", path)
	}
	t.reanchorBeforeDetach(node)
	C.lyd_free_tree(node)
	t.incrementGen()
	return nil
}

// reanchorBeforeDetach updates t.tree so it does not point at a node that is
// about to be freed or unlinked. Must run BEFORE the detach/free call.
func (t *RawDataTree) reanchorBeforeDetach(node *C.struct_lyd_node) {
	if t.tree == nil {
		return
	}
	root := C.lyd_first_sibling(t.tree)
	if root == node {
		// The target is the current root; the new root is its next sibling,
		// which may be nil (tree becomes empty).
		t.tree = node.next
	} else {
		t.tree = root
	}
}

// UnlinkPath detaches the subtree at `path` and returns it as an owned tree.
// The detached tree shares the source context and must be freed by the caller.
func (t *RawDataTree) UnlinkPath(path string) (*RawDataTree, error) {
	defer runtime.KeepAlive(t)
	defer runtime.KeepAlive(t.owner)
	node, ok, err := t.FindNode(path)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("path not found: %q", path)
	}
	t.reanchorBeforeDetach(node)
	if rc := C.lyd_unlink_tree(node); rc != C.LY_SUCCESS {
		return nil, lyError(t.ctx, "unlink path", int(rc))
	}
	t.incrementGen()
	// The detached node becomes a fresh tree with exactly one owner/finalizer.
	detached := newRawDataTree(node, t.owner, t.ctx)
	return detached, nil
}

// AddDefaults adds implicit/default nodes to the tree.
func (t *RawDataTree) AddDefaults(options uint32) error {
	defer runtime.KeepAlive(t)
	defer runtime.KeepAlive(t.owner)
	rc := C.lyd_new_implicit_all(&t.tree, t.ctx, C.uint32_t(options), nil) //nolint:gocritic // dupSubExpr false positive on cgo-rewritten call
	if rc != C.LY_SUCCESS {
		return lyError(t.ctx, "add defaults", int(rc))
	}
	// An implicit node may be inserted before the current first sibling;
	// re-anchor so serialize and Close see the canonical root.
	if t.tree != nil {
		t.tree = C.lyd_first_sibling(t.tree)
	}
	t.incrementGen()
	return nil
}

// retain records one more data tree/diff that must be freed before the libyang
// context can be destroyed.
func (c *RawContext) retain() {
	if c != nil {
		atomic.AddInt64(&c.live, 1)
	}
}

// release records that a data tree/diff has been freed. If Close was requested
// and this was the last outstanding tree, the context is destroyed now — never
// before its data, which would use-after-free the schema/dictionary.
func (c *RawContext) release() {
	if c == nil {
		return
	}
	if atomic.AddInt64(&c.live, -1) == 0 && atomic.LoadInt32(&c.closeReq) == 1 {
		c.destroyCtx()
	}
}

// destroyCtx calls ly_ctx_destroy exactly once. Sequentially-consistent atomics
// guarantee that exactly one of Close/release observes the (live==0, closeReq)
// condition, and the CAS makes the destroy itself idempotent.
func (c *RawContext) destroyCtx() {
	if atomic.CompareAndSwapInt32(&c.destroyed, 0, 1) {
		if c.ctx != nil {
			C.ly_ctx_destroy(c.ctx)
			c.ctx = nil
		}
	}
}

// Close requests destruction of the context and cancels the finalizer. The
// actual ly_ctx_destroy is deferred until every data tree/diff from this
// context has been freed, so a caller may Close the context before its trees
// are collected without a use-after-free. Idempotent.
func (c *RawContext) Close() {
	runtime.SetFinalizer(c, nil)
	atomic.StoreInt32(&c.closeReq, 1)
	if atomic.LoadInt64(&c.live) == 0 {
		c.destroyCtx()
	}
}

// finalize is the GC backstop. A live tree keeps its owner reachable and each
// tree finalizer KeepAlives the owner across its free, so by the time the
// context itself becomes finalizable all its trees are already freed.
func (c *RawContext) finalize() {
	c.destroyCtx()
}

// Generation returns the current mutation counter. Public NodeRef snapshots
// compare against this value to detect stale handles.
func (t *RawDataTree) Generation() uint64 {
	if t == nil {
		return 0
	}
	return t.gen
}

func (t *RawDataTree) incrementGen() {
	if t != nil {
		t.gen++
	}
}

// IncrementGen bumps the mutation counter. It is exported so the public
// DataTree can invalidate NodeRef handles after coarse-grained mutations that
// re-anchor the root.
func (t *RawDataTree) IncrementGen() {
	t.incrementGen()
}

// Duplicate creates a deep, independent copy of the whole sibling chain.
func (t *RawDataTree) Duplicate() (*RawDataTree, error) {
	defer runtime.KeepAlive(t)
	defer runtime.KeepAlive(t.owner)
	first := C.lyd_first_sibling(t.tree)
	var out *C.struct_lyd_node
	rc := C.lyd_dup_siblings(first, nil, C.uint32_t(C.LYD_DUP_RECURSIVE|C.LYD_DUP_WITH_FLAGS), &out) //nolint:gocritic // dupSubExpr false positive on cgo-rewritten call
	if rc != C.LY_SUCCESS {
		return nil, lyError(t.ctx, "duplicate", int(rc))
	}
	if out != nil {
		out = C.lyd_first_sibling(out)
	}
	return newRawDataTree(out, t.owner, t.ctx), nil
}

// lyError builds a Go error from libyang's last logged error (message +
// error-app-tag) for the given context, falling back to the raw return code.
func lyError(ctx *C.struct_ly_ctx, op string, rc int) error {
	if ctx != nil {
		if item := C.ly_err_first(ctx); item != nil && item.msg != nil {
			msg := C.GoString(item.msg)
			if item.apptag != nil {
				if tag := C.GoString(item.apptag); tag != "" {
					return fmt.Errorf("%s: %s [error-app-tag=%s]", op, msg, tag)
				}
			}
			return fmt.Errorf("%s: %s", op, msg)
		}
	}
	if rc != 0 {
		return fmt.Errorf("%s failed: rc=%d", op, rc)
	}
	return fmt.Errorf("%s failed", op)
}

// Serialize prints the whole tree to bytes in one cgo call. The libyang printer
// walks the sibling chain in order, so element order is structural.
// serializeNode prints a libyang node to a textual format. When emptyIsError, a
// null printer result is an error (the data-tree contract); otherwise it yields
// nil bytes (the diff contract). Callers handle a nil tree before delegating.
func serializeNode(ctx *C.struct_ly_ctx, tree *C.struct_lyd_node, format Format, options uint32, emptyIsError bool) ([]byte, error) {
	if format == FormatLYB {
		return serializeNodeLYB(ctx, tree, options)
	}
	if format == FormatJSONIETF {
		options |= PrintEmptyCont
	}
	var out *C.char
	rc := C.lyd_print_mem(&out, tree, format.c(), C.uint32_t(options)) //nolint:gocritic // dupSubExpr false positive on cgo-rewritten call
	if rc != C.LY_SUCCESS {
		return nil, lyError(ctx, "serialize", int(rc))
	}
	if out == nil {
		if emptyIsError {
			return nil, fmt.Errorf("lyd_print_mem returned null")
		}
		return nil, nil
	}
	defer C.free(unsafe.Pointer(out))
	return []byte(C.GoString(out)), nil
}

// serializeNodeLYB prints a libyang node to LYB bytes using the length-aware
// output handler. LYB may contain embedded NULs, so it cannot use lyd_print_mem.
func serializeNodeLYB(ctx *C.struct_ly_ctx, tree *C.struct_lyd_node, options uint32) ([]byte, error) {
	// lyd_print_all auto-includes siblings and rejects the SIBLINGS flag.
	options &^= PrintWithSiblings
	var buf *C.char
	var out *C.struct_ly_out
	rc := C.ly_out_new_memory(&buf, 0, &out) //nolint:gocritic // dupSubExpr false positive on cgo-rewritten call
	if rc != C.LY_SUCCESS {
		return nil, lyError(ctx, "ly_out_new_memory", int(rc))
	}
	first := C.lyd_first_sibling(tree)
	rc = C.lyd_print_all(out, first, C.LYD_LYB, C.uint32_t(options))
	if rc != C.LY_SUCCESS {
		C.ly_out_free(out, nil, 1)
		return nil, lyError(ctx, "lyd_print_all", int(rc))
	}
	length := C.ly_out_printed(out)
	var b []byte
	if length > 0 {
		b = C.GoBytes(unsafe.Pointer(buf), C.int(length))
	}
	C.ly_out_free(out, nil, 1)
	return b, nil
}

// Serialize prints the whole tree to bytes in the given format via lyd_print,
// treating an empty tree as an error.
func (t *RawDataTree) Serialize(format Format, options uint32) ([]byte, error) {
	defer runtime.KeepAlive(t)
	defer runtime.KeepAlive(t.owner)
	return serializeNode(t.owner.ctx, t.tree, format, options, true)
}

// SerializeLYB prints the whole tree to LYB bytes.
func (t *RawDataTree) SerializeLYB(options uint32) ([]byte, error) {
	defer runtime.KeepAlive(t)
	defer runtime.KeepAlive(t.owner)
	return serializeNodeLYB(t.owner.ctx, t.tree, options)
}

// Validate validates the whole tree.
func (t *RawDataTree) Validate(options uint32) error {
	defer runtime.KeepAlive(t)
	defer runtime.KeepAlive(t.owner)
	rc := C.lyd_validate_all(&t.tree, nil, C.uint32_t(options), nil) //nolint:gocritic // dupSubExpr false positive on cgo-rewritten call
	if rc != C.LY_SUCCESS {
		return lyError(t.owner.ctx, "validate", int(rc))
	}
	// Re-anchor the canonical first sibling; validation may insert defaults.
	if t.tree != nil {
		t.tree = C.lyd_first_sibling(t.tree)
	}
	// Validation may insert defaults/re-anchor; conservatively invalidate handles.
	t.incrementGen()
	return nil
}

// RawChildInfo is one materialized child/sibling in declaration/insertion order.
type RawChildInfo struct {
	Path      string
	Name      string
	IsDefault bool
}

// RawDiagnostic is one structured validation diagnostic from libyang.
type RawDiagnostic struct {
	Message    string
	DataPath   string // "" == absent
	SchemaPath string // "" == absent
	AppTag     string // "" == absent
	VecodeStr  string // "" == absent
}

// FindNode resolves a data path to a node. A soft miss returns ok=false and a nil error.
func (t *RawDataTree) FindNode(path string) (*C.struct_lyd_node, bool, error) {
	defer runtime.KeepAlive(t)
	defer runtime.KeepAlive(t.owner)
	cpath := C.CString(path)
	defer C.free(unsafe.Pointer(cpath))
	var node *C.struct_lyd_node
	rc := C.lyd_find_path(t.tree, cpath, C.ly_bool(0), &node) //nolint:gocritic // dupSubExpr false positive on cgo-rewritten call
	if rc == C.LY_ENOTFOUND || rc == C.LY_EINCOMPLETE {
		return nil, false, nil
	}
	if rc != C.LY_SUCCESS {
		return nil, false, lyError(t.owner.ctx, "find path", int(rc))
	}
	if node == nil {
		return nil, false, nil
	}
	return node, true, nil
}

// RootNodes returns the top-level sibling chain in declaration order.
func (t *RawDataTree) RootNodes() ([]RawChildInfo, error) {
	defer runtime.KeepAlive(t)
	defer runtime.KeepAlive(t.owner)
	first := C.lyd_first_sibling(t.tree)
	return t.collectSiblings(first)
}

// ChildrenOf returns the immediate children of the node at path in declaration order.
func (t *RawDataTree) ChildrenOf(path string) ([]RawChildInfo, error) {
	defer runtime.KeepAlive(t)
	defer runtime.KeepAlive(t.owner)
	node, ok, err := t.FindNode(path)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("path not found: %q", path)
	}
	return t.collectSiblings(C.cam_lyd_child(node))
}

// SiblingsOf returns the node at path and all its siblings in declaration order.
func (t *RawDataTree) SiblingsOf(path string) ([]RawChildInfo, error) {
	defer runtime.KeepAlive(t)
	defer runtime.KeepAlive(t.owner)
	node, ok, err := t.FindNode(path)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("path not found: %q", path)
	}
	first := C.lyd_first_sibling(node)
	return t.collectSiblings(first)
}

func (t *RawDataTree) collectSiblings(first *C.struct_lyd_node) ([]RawChildInfo, error) {
	var out []RawChildInfo
	for cur := first; cur != nil; cur = cur.next {
		// Skip opaque / schema-less nodes (produced only under Opaque parse mode)
		// to match the Rust adapter, which gates children/siblings/roots on the
		// node having a schema name. Opaque nodes also cannot round-trip through
		// lyd_find_path, so a NodeRef to one would be unusable.
		if cur.schema == nil {
			continue
		}
		path := lydPathStd(cur)
		if path == "" {
			continue
		}
		out = append(out, RawChildInfo{
			Path:      path,
			Name:      PathNodeName(path),
			IsDefault: (cur.flags & C.LYD_DEFAULT) != 0,
		})
	}
	return out, nil
}

// XPathPaths evaluates an XPath against the tree and returns the absolute path
// of every matched node in document order. The set is materialized in one cgo pass.
func (t *RawDataTree) XPathPaths(xpath string) ([]string, error) {
	defer runtime.KeepAlive(t)
	defer runtime.KeepAlive(t.owner)
	cxpath := C.CString(xpath)
	defer C.free(unsafe.Pointer(cxpath))
	var set *C.struct_ly_set
	rc := C.lyd_find_xpath(t.tree, cxpath, &set) //nolint:gocritic // dupSubExpr false positive on cgo-rewritten call
	if rc != C.LY_SUCCESS {
		return nil, lyError(t.owner.ctx, "xpath", int(rc))
	}
	if set == nil {
		return nil, nil
	}
	count := uint64(set.count)
	paths := make([]string, 0, count)
	for i := uint64(0); i < count; i++ {
		node := C.cam_set_dnode(set, C.uint32_t(i))
		if node == nil {
			continue
		}
		path := lydPathStd(node)
		if path != "" {
			paths = append(paths, path)
		}
	}
	C.ly_set_free(set, (*[0]byte)(nil))
	return paths, nil
}

// ValueStr returns the canonical value string of a leaf or leaf-list node.
// For non-term nodes it returns ("", false, nil).
func (t *RawDataTree) ValueStr(path string) (value string, ok bool, err error) {
	defer runtime.KeepAlive(t)
	defer runtime.KeepAlive(t.owner)
	node, ok, err := t.FindNode(path)
	if err != nil {
		return "", false, err
	}
	if !ok {
		return "", false, fmt.Errorf("path not found: %q", path)
	}
	if !nodeIsTerm(node) {
		return "", false, nil
	}
	v := C.cam_lyd_get_value(node)
	if v == nil {
		return "", false, nil
	}
	return C.GoString(v), true, nil
}

// IsDefault reports whether the node at path was created from a default value.
func (t *RawDataTree) IsDefault(path string) (bool, error) {
	defer runtime.KeepAlive(t)
	defer runtime.KeepAlive(t.owner)
	node, ok, err := t.FindNode(path)
	if err != nil {
		return false, err
	}
	if !ok {
		return false, fmt.Errorf("path not found: %q", path)
	}
	return (node.flags & C.LYD_DEFAULT) != 0, nil
}

// SchemaPtr returns the compiled schema pointer for the node at path, if any.
// Opaque/anydata nodes may have a nil schema pointer.
func (t *RawDataTree) SchemaPtr(path string) (ptr unsafe.Pointer, ok bool, err error) {
	defer runtime.KeepAlive(t)
	defer runtime.KeepAlive(t.owner)
	node, ok, err := t.FindNode(path)
	if err != nil {
		return nil, false, err
	}
	if !ok {
		return nil, false, nil
	}
	if node.schema == nil {
		return nil, false, nil
	}
	return unsafe.Pointer(node.schema), true, nil
}

// ValidateCollect validates the tree and returns every diagnostic libyang produced.
// It sets LY_LOSTORE so the full ly_err_item linked list is preserved.
func (t *RawDataTree) ValidateCollect(options uint32) ([]RawDiagnostic, error) {
	defer runtime.KeepAlive(t)
	defer runtime.KeepAlive(t.owner)

	validateLogMu.Lock()
	defer validateLogMu.Unlock()

	prev := C.ly_log_options(C.uint32_t(C.LY_LOLOG | C.LY_LOSTORE))
	defer C.ly_log_options(prev)

	C.ly_err_clean(t.owner.ctx, nil)
	rc := C.lyd_validate_all(&t.tree, nil, C.uint32_t(options), nil) //nolint:gocritic // dupSubExpr false positive on cgo-rewritten call
	if t.tree != nil {
		t.tree = C.lyd_first_sibling(t.tree)
	}
	t.incrementGen()

	if rc == C.LY_SUCCESS {
		C.ly_err_clean(t.owner.ctx, nil)
		return nil, nil
	}
	if rc != C.LY_EVALID {
		return nil, lyError(t.owner.ctx, "validate", int(rc))
	}

	var out []RawDiagnostic
	for item := C.ly_err_first(t.owner.ctx); item != nil; item = item.next {
		out = append(out, RawDiagnostic{
			Message:    cgoString(item.msg),
			DataPath:   cgoString(item.data_path),
			SchemaPath: cgoString(item.schema_path),
			AppTag:     cgoString(item.apptag),
			VecodeStr:  cgoString(C.ly_strvecode(item.vecode)),
		})
	}
	C.ly_err_clean(t.owner.ctx, nil)
	return out, nil
}

func nodeIsTerm(node *C.struct_lyd_node) bool {
	if node == nil || node.schema == nil {
		return false
	}
	nt := uint16(node.schema.nodetype)
	return nt == C.LYS_LEAF || nt == C.LYS_LEAFLIST
}

func lydPathStd(node *C.struct_lyd_node) string {
	if node == nil {
		return ""
	}
	p := C.lyd_path(node, C.LYD_PATH_STD, nil, 0)
	if p == nil {
		return ""
	}
	defer C.free(unsafe.Pointer(p))
	return C.GoString(p)
}

// PathNodeName returns the final node name from a data path, stripping any list
// predicate (`[...]`) and module prefix (`pfx:`).
func PathNodeName(path string) string {
	last := path
	if i := strings.LastIndexByte(path, '/'); i >= 0 {
		last = path[i+1:]
	}
	if i := strings.IndexByte(last, '['); i >= 0 {
		last = last[:i]
	}
	if i := strings.IndexByte(last, ':'); i >= 0 {
		last = last[i+1:]
	}
	return last
}

func cgoString(p *C.char) string {
	if p == nil {
		return ""
	}
	return C.GoString(p)
}

// UserOrderedListAt returns a positional handle to the `ordered-by user` list
// whose entries live under the parent of the node at `path`.
func (t *RawDataTree) UserOrderedListAt(path string) (*RawUserOrderedList, error) {
	defer runtime.KeepAlive(t)
	defer runtime.KeepAlive(t.owner)
	cpath := C.CString(path)
	defer C.free(unsafe.Pointer(cpath))
	var node *C.struct_lyd_node
	rc := C.lyd_find_path(t.tree, cpath, C.ly_bool(0), &node) //nolint:gocritic // dupSubExpr false positive on cgo-rewritten call
	if rc != C.LY_SUCCESS || node == nil {
		return nil, fmt.Errorf("path not found: %q", path)
	}
	// Capture the resolved node's schema name so index-based operations address
	// only this list's entries, not every sibling under the parent container.
	name := C.GoString(C.cam_lyd_schema_name(node))
	return &RawUserOrderedList{owner: t, parent: C.cam_lyd_parent(node), schemaName: name}, nil
}

// Close frees the tree immediately and cancels the finalizer.
func (t *RawDataTree) Close() {
	runtime.SetFinalizer(t, nil)
	t.incrementGen()
	t.finalize()
}

// intoRaw releases ownership of the underlying node without freeing it (used
// when the node is handed to libyang via an insert). The node is reparented
// into another tree of the same context, so this shell releases its context
// retain (its finalizer will not run).
func (t *RawDataTree) intoRaw() *C.struct_lyd_node {
	runtime.SetFinalizer(t, nil)
	node := t.tree
	t.tree = nil
	if atomic.CompareAndSwapInt32(&t.released, 0, 1) {
		t.owner.release()
	}
	return node
}

func (t *RawDataTree) finalize() {
	if !atomic.CompareAndSwapInt32(&t.released, 0, 1) {
		return
	}
	defer runtime.KeepAlive(t.owner)
	if t.tree != nil {
		C.lyd_free_all(t.tree)
		t.tree = nil
	}
	// Release after the free so the context (and its schema/dictionary) is still
	// alive while lyd_free_all runs; this may now destroy a Close-pending context.
	t.owner.release()
}

// Merge merges source into this tree in place.
// Flags are always 0; source is borrowed and never modified or freed by libyang.
func (t *RawDataTree) Merge(source *RawDataTree) error {
	defer runtime.KeepAlive(t)
	defer runtime.KeepAlive(t.owner)
	defer runtime.KeepAlive(source)
	defer runtime.KeepAlive(source.owner)
	// Passing two different ly_ctx to libyang is undefined behavior; guard at the
	// raw layer to match the Rust adapter (adapter.rs merge).
	if t.ctx != source.ctx {
		return fmt.Errorf("merge requires both trees to share the same context")
	}
	sourceFirst := C.lyd_first_sibling(source.tree)
	rc := C.lyd_merge_siblings(&t.tree, sourceFirst, 0) //nolint:gocritic // dupSubExpr false positive on cgo-rewritten call
	if rc != C.LY_SUCCESS {
		return lyError(t.ctx, "merge", int(rc))
	}
	if t.tree != nil {
		t.tree = C.lyd_first_sibling(t.tree)
	}
	t.incrementGen()
	return nil
}

// DiffApply applies a diff tree to this tree in place.
func (t *RawDataTree) DiffApply(diff *RawDataDiff) error {
	defer runtime.KeepAlive(t)
	defer runtime.KeepAlive(t.owner)
	defer runtime.KeepAlive(diff)
	defer runtime.KeepAlive(diff.owner)
	rc := C.lyd_diff_apply_all(&t.tree, diff.tree) //nolint:gocritic // dupSubExpr false positive on cgo-rewritten call
	if rc != C.LY_SUCCESS {
		return lyError(t.ctx, "diff apply", int(rc))
	}
	if t.tree != nil {
		t.tree = C.lyd_first_sibling(t.tree)
	}
	t.incrementGen()
	return nil
}

// Diff computes the yang-patch-shaped diff from this tree to another.
func (t *RawDataTree) Diff(other *RawDataTree, defaults bool) (*RawDataDiff, error) {
	defer runtime.KeepAlive(t)
	defer runtime.KeepAlive(t.owner)
	defer runtime.KeepAlive(other)
	defer runtime.KeepAlive(other.owner)
	// Passing two different ly_ctx to libyang is undefined behavior; guard at the
	// raw layer to match the Rust adapter (adapter.rs diff).
	if t.ctx != other.ctx {
		return nil, fmt.Errorf("diff requires both trees to share the same context")
	}
	var options C.uint16_t
	if defaults {
		options = C.uint16_t(C.LYD_DIFF_DEFAULTS)
	}
	first := C.lyd_first_sibling(t.tree)
	second := C.lyd_first_sibling(other.tree)
	var diff *C.struct_lyd_node
	rc := C.lyd_diff_siblings(first, second, options, &diff) //nolint:gocritic // dupSubExpr false positive on cgo-rewritten call
	if rc != C.LY_SUCCESS {
		return nil, lyError(t.ctx, "diff", int(rc))
	}
	if diff == nil {
		return nil, nil
	}
	return newRawDataDiff(diff, t.owner, t.ctx), nil
}

// RawDiffOp is the operation carried on a diff-tree node.
type RawDiffOp int

// Diff operations carried on a diff-tree node, mirroring libyang's
// yang:operation metadata values.
const (
	RawDiffOpCreate RawDiffOp = iota
	RawDiffOpDelete
	RawDiffOpReplace
	RawDiffOpNone
)

// RawDiffEdit is one materialized diff edit.
type RawDiffEdit struct {
	Op            RawDiffOp
	Path          string
	Value         string
	ValueOK       bool
	IsUserOrdered bool
}

// RawDataDiff owns a yang-patch-shaped diff tree returned by RawDataTree.Diff.
type RawDataDiff struct {
	tree     *C.struct_lyd_node
	owner    *RawContext
	ctx      *C.struct_ly_ctx
	released int32 // free/release performed (exactly-once, atomic)
}

func newRawDataDiff(tree *C.struct_lyd_node, owner *RawContext, ctx *C.struct_ly_ctx) *RawDataDiff {
	owner.retain()
	d := &RawDataDiff{tree: tree, owner: owner, ctx: ctx}
	runtime.SetFinalizer(d, (*RawDataDiff).finalize)
	return d
}

// Close frees the diff tree immediately and cancels the finalizer.
func (d *RawDataDiff) Close() {
	runtime.SetFinalizer(d, nil)
	d.finalize()
}

func (d *RawDataDiff) finalize() {
	if !atomic.CompareAndSwapInt32(&d.released, 0, 1) {
		return
	}
	defer runtime.KeepAlive(d.owner)
	if d.tree != nil {
		C.lyd_free_all(d.tree)
		d.tree = nil
	}
	d.owner.release()
}

// Serialize prints the diff tree to bytes.
func (d *RawDataDiff) Serialize(format Format, options uint32) ([]byte, error) {
	defer runtime.KeepAlive(d)
	defer runtime.KeepAlive(d.owner)
	if d.tree == nil {
		return nil, nil
	}
	return serializeNode(d.ctx, d.tree, format, options, false)
}

// SerializeLYB prints the diff tree to LYB bytes.
func (d *RawDataDiff) SerializeLYB(options uint32) ([]byte, error) {
	defer runtime.KeepAlive(d)
	defer runtime.KeepAlive(d.owner)
	if d.tree == nil {
		return nil, nil
	}
	return serializeNodeLYB(d.ctx, d.tree, options)
}

// Edits materializes all diff edits in one pre-order walk of the diff tree.
func (d *RawDataDiff) Edits() ([]RawDiffEdit, error) {
	defer runtime.KeepAlive(d)
	defer runtime.KeepAlive(d.owner)
	yangMod := C.ly_ctx_get_module_implemented(d.ctx, C.cam_cstr_yang())
	if yangMod == nil {
		return nil, fmt.Errorf("yang module is not implemented in context")
	}
	var out []RawDiffEdit
	if err := d.collectEdits(d.tree, yangMod, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (d *RawDataDiff) collectEdits(node *C.struct_lyd_node, yangMod *C.struct_lys_module, out *[]RawDiffEdit) error {
	cur := node
	for cur != nil {
		op, err := d.inheritedOp(cur, yangMod)
		if err != nil {
			return err
		}
		schema := cur.schema
		isKey := schema != nil && (uint32(schema.flags)&uint32(C.LYS_KEY)) != 0
		if op != RawDiffOpNone && !isKey {
			path := lydPathStd(cur)
			if path == "" {
				return fmt.Errorf("lyd_path returned null")
			}
			var value string
			valueOK := false
			if nodeIsTerm(cur) {
				if v, ok := nodeValueStrDirect(cur); ok {
					value = v
					valueOK = true
				}
			}
			isUserOrdered := schema != nil && (uint32(schema.flags)&uint32(C.LYS_ORDBY_USER)) != 0
			*out = append(*out, RawDiffEdit{
				Op:            op,
				Path:          path,
				Value:         value,
				ValueOK:       valueOK,
				IsUserOrdered: isUserOrdered,
			})
		}
		if child := C.cam_lyd_child(cur); child != nil {
			if err := d.collectEdits(child, yangMod, out); err != nil {
				return err
			}
		}
		cur = cur.next
		if cur == node {
			break
		}
	}
	return nil
}

func (d *RawDataDiff) inheritedOp(node *C.struct_lyd_node, yangMod *C.struct_lys_module) (RawDiffOp, error) {
	for cur := node; cur != nil; cur = cur.parent {
		meta := C.lyd_find_meta(cur.meta, yangMod, C.cam_cstr_operation())
		if meta != nil {
			val, err := d.metaValueStr(meta)
			if err != nil {
				return RawDiffOpNone, err
			}
			switch val {
			case "create":
				return RawDiffOpCreate, nil
			case "delete":
				return RawDiffOpDelete, nil
			case "replace":
				return RawDiffOpReplace, nil
			case "none":
				return RawDiffOpNone, nil
			default:
				return RawDiffOpNone, nil
			}
		}
	}
	return RawDiffOpNone, nil
}

func (d *RawDataDiff) metaValueStr(meta *C.struct_lyd_meta) (string, error) {
	if meta == nil {
		return "", nil
	}
	value := C.cam_lyd_get_meta_value(meta)
	if value != nil {
		return C.GoString(value), nil
	}
	return "", nil
}

// nodeValueStrDirect reads the canonical value string through libyang's public
// value accessor. It returns ("", false) for non-term nodes or missing values.
func nodeValueStrDirect(node *C.struct_lyd_node) (string, bool) {
	if !nodeIsTerm(node) {
		return "", false
	}
	value := C.cam_lyd_get_value(node)
	if value == nil {
		return "", false
	}
	return C.GoString(value), true
}

// RawUserOrderedList is a positional-only handle to an `ordered-by user` list.
// It maps 1:1 to libyang's caller-authoritative insert functions. There is no
// order-agnostic mutator. It keeps its owning RawDataTree alive.
type RawUserOrderedList struct {
	owner      *RawDataTree
	parent     *C.struct_lyd_node
	schemaName string
}

// InsertFirst inserts entry as the first sibling.
func (l *RawUserOrderedList) InsertFirst(entry *RawDataTree) error {
	defer runtime.KeepAlive(l.owner)
	// Insert before the first entry OF THIS LIST, not the parent's first child
	// (which may be a foreign sibling).
	first := l.nth(0)
	if first == nil {
		return l.InsertLast(entry)
	}
	return insertBeforeNode(l, first, entry)
}

// InsertLast inserts entry as the last child of the parent.
func (l *RawUserOrderedList) InsertLast(entry *RawDataTree) error {
	defer runtime.KeepAlive(l.owner)
	node := entry.intoRaw()
	if rc := C.lyd_insert_child(l.parent, node); rc != C.LY_SUCCESS {
		C.lyd_free_tree(node)
		return fmt.Errorf("lyd_insert_child failed: rc=%d", int(rc))
	}
	l.owner.incrementGen()
	return nil
}

// InsertBefore inserts entry before the entry at index.
func (l *RawUserOrderedList) InsertBefore(index int, entry *RawDataTree) error {
	defer runtime.KeepAlive(l.owner)
	sib := l.nth(index)
	if sib == nil {
		return fmt.Errorf("insert_before index %d out of range", index)
	}
	return insertBeforeNode(l, sib, entry)
}

// InsertAfter inserts entry after the entry at index.
func (l *RawUserOrderedList) InsertAfter(index int, entry *RawDataTree) error {
	defer runtime.KeepAlive(l.owner)
	sib := l.nth(index)
	if sib == nil {
		return fmt.Errorf("insert_after index %d out of range", index)
	}
	node := entry.intoRaw()
	if rc := C.lyd_insert_after(sib, node); rc != C.LY_SUCCESS {
		C.lyd_free_tree(node)
		return fmt.Errorf("lyd_insert_after failed: rc=%d", int(rc))
	}
	l.owner.incrementGen()
	return nil
}

// MoveBefore moves the entry at `what` before the entry at `point`.
func (l *RawUserOrderedList) MoveBefore(what, point int) error {
	defer runtime.KeepAlive(l.owner)
	w, p := l.nth(what), l.nth(point)
	if w == nil || p == nil {
		return fmt.Errorf("move_before index out of range")
	}
	if rc := C.lyd_insert_before(p, w); rc != C.LY_SUCCESS {
		return fmt.Errorf("lyd_insert_before (move) failed: rc=%d", int(rc))
	}
	l.owner.incrementGen()
	return nil
}

// MoveAfter moves the entry at `what` after the entry at `point`.
func (l *RawUserOrderedList) MoveAfter(what, point int) error {
	defer runtime.KeepAlive(l.owner)
	w, p := l.nth(what), l.nth(point)
	if w == nil || p == nil {
		return fmt.Errorf("move_after index out of range")
	}
	if rc := C.lyd_insert_after(p, w); rc != C.LY_SUCCESS {
		return fmt.Errorf("lyd_insert_after (move) failed: rc=%d", int(rc))
	}
	l.owner.incrementGen()
	return nil
}

func (l *RawUserOrderedList) nth(n int) *C.struct_lyd_node {
	defer runtime.KeepAlive(l.owner)
	// Index relative to THIS list's entries only. A parent container may hold
	// other siblings (other lists, leaves), so filter by schema name — mirrors
	// the Rust adapter's filtered nth_child. Without this, an index walks across
	// foreign siblings and inserts/moves relative to the wrong node, silently
	// corrupting the structural order this project exists to protect.
	i := 0
	for child := C.lyd_child(l.parent); child != nil; child = child.next {
		if C.GoString(C.cam_lyd_schema_name(child)) != l.schemaName {
			continue
		}
		if i == n {
			return child
		}
		i++
	}
	return nil
}

func insertBeforeNode(l *RawUserOrderedList, sibling *C.struct_lyd_node, entry *RawDataTree) error {
	defer runtime.KeepAlive(l.owner)
	node := entry.intoRaw()
	if rc := C.lyd_insert_before(sibling, node); rc != C.LY_SUCCESS {
		C.lyd_free_tree(node)
		return fmt.Errorf("lyd_insert_before failed: rc=%d", int(rc))
	}
	l.owner.incrementGen()
	return nil
}
