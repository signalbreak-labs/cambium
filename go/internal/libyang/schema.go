// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

// Coarse and rich schema-tree introspection over libyang's compiled schema nodes.
//
// This file is part of the internal cgo adapter; public callers use
// github.com/signalbreak-labs/cambium/go/cambium.
//
//go:build cgo

package libyang

/*
#cgo CFLAGS: -I${SRCDIR}/.build/libyang-install/include -I${SRCDIR}/.build/pcre2-install/include -I${SRCDIR}/vendor/libyang/src -I${SRCDIR}/../../../third_party/libyang/src
#include <stdlib.h>
#include <string.h>
#include <libyang/libyang.h>
#include <libyang/plugins_exts.h>
#include "path.h"

static uint32_t cam_lysc_node_basetype(const struct lysc_node *node) {
    uint16_t nt = node->nodetype;
    struct lysc_type *t = NULL;
    if (nt == LYS_LEAF) {
        t = ((struct lysc_node_leaf *)node)->type;
    } else if (nt == LYS_LEAFLIST) {
        t = ((struct lysc_node_leaflist *)node)->type;
    }
    return t ? t->basetype : LY_TYPE_UNKNOWN;
}

static const char *cam_lysc_node_dsc(const struct lysc_node *node) {
    return node ? node->dsc : NULL;
}

static const char *cam_lysc_node_ref(const struct lysc_node *node) {
    return node ? node->ref : NULL;
}

static const char *cam_lysc_node_units(const struct lysc_node *node) {
    uint16_t nt = node->nodetype;
    if (nt == LYS_LEAF) {
        return ((struct lysc_node_leaf *)node)->units;
    } else if (nt == LYS_LEAFLIST) {
        return ((struct lysc_node_leaflist *)node)->units;
    }
    return NULL;
}

static void cam_lysc_node_min_max(const struct lysc_node *node, uint32_t *min, uint32_t *max) {
    uint16_t nt = node->nodetype;
    if (nt == LYS_LIST) {
        *min = ((struct lysc_node_list *)node)->min;
        *max = ((struct lysc_node_list *)node)->max;
    } else if (nt == LYS_LEAFLIST) {
        *min = ((struct lysc_node_leaflist *)node)->min;
        *max = ((struct lysc_node_leaflist *)node)->max;
    } else {
        *min = 0;
        *max = 0;
    }
}

static struct lysc_type *cam_node_leaf_type(const struct lysc_node *node) {
    return ((struct lysc_node_leaf *)node)->type;
}

static struct lysc_type *cam_node_leaflist_type(const struct lysc_node *node) {
    return ((struct lysc_node_leaflist *)node)->type;
}

static uint64_t cam_sized_array_count(const void *arr) {
    return arr ? *((const uint64_t *)arr - 1) : 0;
}

static const struct lysc_ident *cam_module_identity_at(const struct lysc_ident *arr, uint64_t i) {
    return arr + i;
}

static const struct lysc_ident *cam_ident_at(const struct lysc_ident **arr, uint64_t i) {
    return arr[i];
}

static const char *cam_ident_name(const struct lysc_ident *ident) {
    return ident ? ident->name : NULL;
}

static const struct lys_module *cam_ident_module(const struct lysc_ident *ident) {
    return ident ? ident->module : NULL;
}

static const struct lysc_ident *cam_ident_derived_at(const struct lysc_ident **arr, uint64_t i) {
    return arr[i];
}

static const struct lysp_ident *cam_parsed_identity_array_by_name(const struct lysp_ident *idents, const char *name) {
    if (!idents || !name) {
        return NULL;
    }
    uint64_t count = cam_sized_array_count((void *)idents);
    for (uint64_t i = 0; i < count; i++) {
        const struct lysp_ident *ident = &idents[i];
        if (ident->name && !strcmp(ident->name, name)) {
            return ident;
        }
    }
    return NULL;
}

static const struct lysp_ident *cam_parsed_identity_by_name(const struct lys_module *mod, const char *name) {
    if (!mod || !mod->parsed || !name) {
        return NULL;
    }
    const struct lysp_ident *ident = cam_parsed_identity_array_by_name(mod->parsed->identities, name);
    if (ident) {
        return ident;
    }
    uint64_t count = cam_sized_array_count(mod->parsed->includes);
    for (uint64_t i = 0; i < count; i++) {
        const struct lysp_submodule *submod = mod->parsed->includes[i].submodule;
        if (!submod) {
            continue;
        }
        ident = cam_parsed_identity_array_by_name(submod->identities, name);
        if (ident) {
            return ident;
        }
    }
    return NULL;
}

static int cam_parsed_identity_array_contains(const struct lysp_ident *idents, const struct lysp_ident *ident) {
    if (!idents || !ident) {
        return 0;
    }
    uint64_t count = cam_sized_array_count((void *)idents);
    for (uint64_t i = 0; i < count; i++) {
        if (&idents[i] == ident) {
            return 1;
        }
    }
    return 0;
}

static int cam_prefix_eq(const char *candidate, const char *prefix, size_t prefix_len) {
    return candidate && strlen(candidate) == prefix_len && !strncmp(candidate, prefix, prefix_len);
}

static const char *cam_identity_base_module_name_in_scope(
    const struct lys_module *owner_mod,
    const struct lysp_import *imports,
    const char *local_prefix,
    const char *base
) {
    if (!owner_mod || !base) {
        return NULL;
    }
    const char *colon = strchr(base, ':');
    if (!colon) {
        return owner_mod->name;
    }
    size_t prefix_len = (size_t)(colon - base);
    if (cam_prefix_eq(owner_mod->name, base, prefix_len) ||
            cam_prefix_eq(owner_mod->prefix, base, prefix_len) ||
            cam_prefix_eq(local_prefix, base, prefix_len)) {
        return owner_mod->name;
    }
    uint64_t count = cam_sized_array_count((void *)imports);
    for (uint64_t i = 0; i < count; i++) {
        const struct lysp_import *imp = &imports[i];
        if (!cam_prefix_eq(imp->prefix, base, prefix_len) && !cam_prefix_eq(imp->name, base, prefix_len)) {
            continue;
        }
        return imp->module ? imp->module->name : imp->name;
    }
    return NULL;
}

static const char *cam_parsed_identity_base_module_name(
    const struct lys_module *mod,
    const struct lysp_ident *ident,
    const char *base
) {
    if (!mod || !mod->parsed || !ident || !base) {
        return NULL;
    }
    if (cam_parsed_identity_array_contains(mod->parsed->identities, ident)) {
        return cam_identity_base_module_name_in_scope(mod, mod->parsed->imports, mod->prefix, base);
    }
    uint64_t count = cam_sized_array_count(mod->parsed->includes);
    for (uint64_t i = 0; i < count; i++) {
        const struct lysp_submodule *submod = mod->parsed->includes[i].submodule;
        if (!submod || !cam_parsed_identity_array_contains(submod->identities, ident)) {
            continue;
        }
        return cam_identity_base_module_name_in_scope(mod, submod->imports, submod->prefix, base);
    }
    return NULL;
}

static const char **cam_parsed_identity_bases(const struct lysp_ident *ident) {
    return ident ? ident->bases : NULL;
}

static const char *cam_identity_base_at(const char **arr, uint64_t i) {
    return arr ? arr[i] : NULL;
}

static const char *cam_type_name(const struct lysc_type *type) {
    return type ? type->name : NULL;
}

static uint32_t cam_type_basetype(const struct lysc_type *type) {
    return type ? type->basetype : LY_TYPE_UNKNOWN;
}

static const struct lysc_type *cam_leafref_realtype(const struct lysc_type_leafref *lr) {
    return lr ? lr->realtype : NULL;
}

static int cam_leafref_require_instance(const struct lysc_type_leafref *lr) {
    return lr ? lr->require_instance : 1;
}

static int cam_instanceid_require_instance(const struct lysc_type_instanceid *inst) {
    return inst ? inst->require_instance : 1;
}

static const struct lysc_range *cam_type_num_range(const struct lysc_type *type) {
    return type ? ((const struct lysc_type_num *)type)->range : NULL;
}

static const struct lysc_range *cam_type_dec_range(const struct lysc_type *type) {
    return type ? ((const struct lysc_type_dec *)type)->range : NULL;
}

static uint8_t cam_type_dec_fraction_digits(const struct lysc_type *type) {
    return type ? ((const struct lysc_type_dec *)type)->fraction_digits : 1;
}

static const struct lysc_range *cam_type_str_length(const struct lysc_type *type) {
    return type ? ((const struct lysc_type_str *)type)->length : NULL;
}

static struct lysc_pattern **cam_type_str_patterns(const struct lysc_type *type) {
    return type ? ((struct lysc_type_str *)type)->patterns : NULL;
}

static const struct lysc_range *cam_type_bin_length(const struct lysc_type *type) {
    return type ? ((const struct lysc_type_bin *)type)->length : NULL;
}

static const struct lysc_type_bitenum_item *cam_type_enum_enums(const struct lysc_type *type) {
    return type ? ((const struct lysc_type_enum *)type)->enums : NULL;
}

static const struct lysc_type_bitenum_item *cam_type_bits_bits(const struct lysc_type *type) {
    return type ? ((const struct lysc_type_bits *)type)->bits : NULL;
}

static struct lysc_ident **cam_type_ident_bases(const struct lysc_type *type) {
    return type ? ((struct lysc_type_identityref *)type)->bases : NULL;
}

static struct lysc_type **cam_type_union_types(const struct lysc_type *type) {
    return type ? ((struct lysc_type_union *)type)->types : NULL;
}

static const struct lysc_range_part *cam_range_parts(const struct lysc_range *range) {
    return range ? range->parts : NULL;
}

static const struct lysc_range_part *cam_range_part_at(const struct lysc_range *range, uint64_t i) {
    return range->parts + i;
}

static int64_t cam_range_part_min64(const struct lysc_range_part *p) {
    return p->min_64;
}

static int64_t cam_range_part_max64(const struct lysc_range_part *p) {
    return p->max_64;
}

static uint64_t cam_range_part_minu64(const struct lysc_range_part *p) {
    return p->min_u64;
}

static uint64_t cam_range_part_maxu64(const struct lysc_range_part *p) {
    return p->max_u64;
}

static const struct lysc_type_bitenum_item *cam_bitenum_at(const struct lysc_type_bitenum_item *arr, uint64_t i) {
    return arr + i;
}

static const char *cam_bitenum_name(const struct lysc_type_bitenum_item *item) {
    return item ? item->name : NULL;
}

static int32_t cam_enum_value(const struct lysc_type_bitenum_item *item) {
    return item->value;
}

static uint32_t cam_bit_position(const struct lysc_type_bitenum_item *item) {
    return item->position;
}

static const struct lysc_pattern *cam_pattern_at(struct lysc_pattern **arr, uint64_t i) {
    return arr[i];
}

static const char *cam_pattern_expr(const struct lysc_pattern *p) {
    return p ? p->expr : NULL;
}

static const char *cam_pattern_eapptag(const struct lysc_pattern *p) {
    return p ? p->eapptag : NULL;
}

static int cam_pattern_inverted(const struct lysc_pattern *p) {
    return p ? p->inverted : 0;
}

static const struct lysc_ident *cam_ident_base_at(struct lysc_ident **arr, uint64_t i) {
    return arr[i];
}

static const struct lysc_type *cam_union_type_at(struct lysc_type **arr, uint64_t i) {
    return arr[i];
}

static const char *cam_leafref_path(const struct lysc_type *type) {
    struct lysc_type_leafref *lr = (struct lysc_type_leafref *)type;
    if (!lr || !lr->path) {
        return NULL;
    }
    return lyxp_get_expr(lr->path);
}

static const struct lysc_node *cam_leafref_target(const struct lysc_node *node) {
    uint16_t nt = node->nodetype;
    struct lysc_type *t = NULL;
    if (nt == LYS_LEAF) {
        t = ((struct lysc_node_leaf *)node)->type;
    } else if (nt == LYS_LEAFLIST) {
        t = ((struct lysc_node_leaflist *)node)->type;
    }
    if (!t || t->basetype != LY_TYPE_LEAFREF) {
        return NULL;
    }
    struct lysc_type_leafref *lref = (struct lysc_type_leafref *)t;
    struct ly_path *p = NULL;
    uint16_t oper = (node->flags & LYS_IS_OUTPUT) ? LY_PATH_OPER_OUTPUT : LY_PATH_OPER_INPUT;
    if (ly_path_compile_leafref(node->module->ctx, node, lref->path, oper, LY_PATH_TARGET_MANY,
            LY_VALUE_SCHEMA_RESOLVED, lref->prefixes, &p) != LY_SUCCESS) {
        return NULL;
    }
    const struct lysc_node *target = NULL;
    if (p) {
        uint64_t count = cam_sized_array_count((const void *)p);
        if (count > 0) {
            target = p[count - 1].node;
        }
        ly_path_free(p);
    }
    return target;
}

static const struct lys_module *cam_node_module(const struct lysc_node *node) {
    return node ? node->module : NULL;
}

static uint64_t cam_node_dflt_count(const struct lysc_node *node) {
    uint16_t nt = node->nodetype;
    if (nt == LYS_LEAF) {
        return ((struct lysc_node_leaf *)node)->dflt.str ? 1 : 0;
    } else if (nt == LYS_LEAFLIST) {
        return cam_sized_array_count(((struct lysc_node_leaflist *)node)->dflts);
    }
    return 0;
}

static const char *cam_node_dflt_at(const struct lysc_node *node, uint64_t i) {
    uint16_t nt = node->nodetype;
    if (nt == LYS_LEAF) {
        return ((struct lysc_node_leaf *)node)->dflt.str;
    } else if (nt == LYS_LEAFLIST) {
        const struct lysc_value *arr = ((struct lysc_node_leaflist *)node)->dflts;
        if (arr) {
            return (arr + i)->str;
        }
    }
    return NULL;
}

static struct lysc_ext_instance *cam_node_exts(const struct lysc_node *node) {
    return node ? node->exts : NULL;
}

static struct lysc_ext_instance *cam_ext_at(struct lysc_ext_instance *arr, uint64_t i) {
    return arr + i;
}

static const char *cam_ext_name(const struct lysc_ext_instance *e) {
    return (e && e->def) ? e->def->name : NULL;
}

static const char *cam_ext_argument(const struct lysc_ext_instance *e) {
    return e ? e->argument : NULL;
}

static const char *cam_ext_module_name(const struct lysc_ext_instance *e) {
    return (e && e->module) ? e->module->name : NULL;
}

static struct lysc_must *cam_node_musts(const struct lysc_node *node) {
    return lysc_node_musts(node);
}

static struct lysc_when **cam_node_whens(const struct lysc_node *node) {
    return lysc_node_when(node);
}

static struct lysc_must *cam_must_at(struct lysc_must *arr, uint64_t i) {
    return arr + i;
}

static struct lysc_when *cam_when_at(struct lysc_when **arr, uint64_t i) {
    return arr[i];
}

static const char *cam_must_cond(const struct lysc_must *m) {
    return (m && m->cond) ? lyxp_get_expr(m->cond) : NULL;
}

static const char *cam_must_emsg(const struct lysc_must *m) {
    return m ? m->emsg : NULL;
}

static const char *cam_must_eapptag(const struct lysc_must *m) {
    return m ? m->eapptag : NULL;
}

static const char *cam_must_dsc(const struct lysc_must *m) {
    return m ? m->dsc : NULL;
}

static const char *cam_must_ref(const struct lysc_must *m) {
    return m ? m->ref : NULL;
}

static const char *cam_when_cond(const struct lysc_when *w) {
    return (w && w->cond) ? lyxp_get_expr(w->cond) : NULL;
}

static const char *cam_when_dsc(const struct lysc_when *w) {
    return w ? w->dsc : NULL;
}

static const char *cam_when_ref(const struct lysc_when *w) {
    return w ? w->ref : NULL;
}

static struct lysc_node_leaf ***cam_node_uniques(const struct lysc_node *node) {
    if (node->nodetype != LYS_LIST) {
        return NULL;
    }
    return ((struct lysc_node_list *)node)->uniques;
}

static struct lysc_node_leaf **cam_unique_leaf_array_at(struct lysc_node_leaf ***arr, uint64_t i) {
    return arr[i];
}

static struct lysc_node_leaf *cam_unique_leaf_at(struct lysc_node_leaf **arr, uint64_t i) {
    return arr[i];
}

static struct lysp_import *cam_module_imports(const struct lys_module *mod) {
    return (mod && mod->parsed) ? mod->parsed->imports : NULL;
}

static struct lysp_import *cam_import_at(struct lysp_import *arr, uint64_t i) {
    return arr + i;
}

static const char *cam_import_prefix(const struct lysp_import *imp) {
    return imp ? imp->prefix : NULL;
}

static const char *cam_import_name(const struct lysp_import *imp) {
    return imp ? imp->name : NULL;
}

static const char *cam_import_revision(const struct lysp_import *imp) {
    return imp ? imp->rev : NULL;
}

static struct lys_module **cam_module_augmented_by(const struct lys_module *mod) {
    return mod ? mod->augmented_by : NULL;
}

static struct lys_module **cam_module_deviated_by(const struct lys_module *mod) {
    return mod ? mod->deviated_by : NULL;
}

static struct lysp_deviation *cam_module_deviations(const struct lys_module *mod) {
    return (mod && mod->parsed) ? mod->parsed->deviations : NULL;
}

static struct lysp_deviation *cam_deviation_at(struct lysp_deviation *arr, uint64_t i) {
    return arr + i;
}

static const char *cam_deviation_nodeid(const struct lysp_deviation *dev) {
    return dev ? dev->nodeid : NULL;
}

static const char *cam_deviation_dsc(const struct lysp_deviation *dev) {
    return dev ? dev->dsc : NULL;
}

static const char *cam_deviation_ref(const struct lysp_deviation *dev) {
    return dev ? dev->ref : NULL;
}

static struct lysp_deviate *cam_deviation_deviates(const struct lysp_deviation *dev) {
    return dev ? dev->deviates : NULL;
}

static struct lysp_deviate *cam_deviate_next(const struct lysp_deviate *d) {
    return d ? d->next : NULL;
}

static uint8_t cam_deviate_mod(const struct lysp_deviate *d) {
    return d ? d->mod : 0;
}

static const char *cam_deviate_add_units(const struct lysp_deviate *d) {
    return (d && d->mod == LYS_DEV_ADD) ? ((const struct lysp_deviate_add *)d)->units : NULL;
}

static struct lysp_restr *cam_deviate_add_musts(const struct lysp_deviate *d) {
    return (d && d->mod == LYS_DEV_ADD) ? ((const struct lysp_deviate_add *)d)->musts : NULL;
}

static struct lysp_qname *cam_deviate_add_uniques(const struct lysp_deviate *d) {
    return (d && d->mod == LYS_DEV_ADD) ? ((const struct lysp_deviate_add *)d)->uniques : NULL;
}

static struct lysp_qname *cam_deviate_add_dflts(const struct lysp_deviate *d) {
    return (d && d->mod == LYS_DEV_ADD) ? ((const struct lysp_deviate_add *)d)->dflts : NULL;
}

static uint16_t cam_deviate_add_flags(const struct lysp_deviate *d) {
    return (d && d->mod == LYS_DEV_ADD) ? ((const struct lysp_deviate_add *)d)->flags : 0;
}

static uint32_t cam_deviate_add_min(const struct lysp_deviate *d) {
    return (d && d->mod == LYS_DEV_ADD) ? ((const struct lysp_deviate_add *)d)->min : 0;
}

static uint32_t cam_deviate_add_max(const struct lysp_deviate *d) {
    return (d && d->mod == LYS_DEV_ADD) ? ((const struct lysp_deviate_add *)d)->max : 0;
}

static const char *cam_deviate_del_units(const struct lysp_deviate *d) {
    return (d && d->mod == LYS_DEV_DELETE) ? ((const struct lysp_deviate_del *)d)->units : NULL;
}

static struct lysp_restr *cam_deviate_del_musts(const struct lysp_deviate *d) {
    return (d && d->mod == LYS_DEV_DELETE) ? ((const struct lysp_deviate_del *)d)->musts : NULL;
}

static struct lysp_qname *cam_deviate_del_uniques(const struct lysp_deviate *d) {
    return (d && d->mod == LYS_DEV_DELETE) ? ((const struct lysp_deviate_del *)d)->uniques : NULL;
}

static struct lysp_qname *cam_deviate_del_dflts(const struct lysp_deviate *d) {
    return (d && d->mod == LYS_DEV_DELETE) ? ((const struct lysp_deviate_del *)d)->dflts : NULL;
}

static struct lysp_type *cam_deviate_rpl_type(const struct lysp_deviate *d) {
    return (d && d->mod == LYS_DEV_REPLACE) ? ((const struct lysp_deviate_rpl *)d)->type : NULL;
}

static const char *cam_deviate_rpl_units(const struct lysp_deviate *d) {
    return (d && d->mod == LYS_DEV_REPLACE) ? ((const struct lysp_deviate_rpl *)d)->units : NULL;
}

static const char *cam_deviate_rpl_dflt(const struct lysp_deviate *d) {
    return (d && d->mod == LYS_DEV_REPLACE) ? ((const struct lysp_deviate_rpl *)d)->dflt.str : NULL;
}

static uint16_t cam_deviate_rpl_flags(const struct lysp_deviate *d) {
    return (d && d->mod == LYS_DEV_REPLACE) ? ((const struct lysp_deviate_rpl *)d)->flags : 0;
}

static uint32_t cam_deviate_rpl_min(const struct lysp_deviate *d) {
    return (d && d->mod == LYS_DEV_REPLACE) ? ((const struct lysp_deviate_rpl *)d)->min : 0;
}

static uint32_t cam_deviate_rpl_max(const struct lysp_deviate *d) {
    return (d && d->mod == LYS_DEV_REPLACE) ? ((const struct lysp_deviate_rpl *)d)->max : 0;
}

static const char *cam_lysp_type_name(const struct lysp_type *type) {
    return type ? type->name : NULL;
}

static struct lysp_restr *cam_deviate_must_at(struct lysp_restr *arr, uint64_t i) {
    return arr + i;
}

static const char *cam_restr_cond(const struct lysp_restr *r) {
    return r ? r->arg.str : NULL;
}

static const char *cam_qname_str(const struct lysp_qname *q) {
    return q ? q->str : NULL;
}

static struct lys_module *cam_module_ptr_at(struct lys_module **arr, uint64_t i) {
    return arr[i];
}

static const char *cam_module_name(const struct lys_module *mod) {
    return mod ? mod->name : NULL;
}

static const char *cam_module_ns(const struct lys_module *mod) {
    return mod ? mod->ns : NULL;
}

static const char *cam_module_revision(const struct lys_module *mod) {
    return mod ? mod->revision : NULL;
}

static int cam_module_has_parsed(const struct lys_module *mod) {
    return (mod && mod->parsed) ? 1 : 0;
}

static struct lys_module *cam_load_module_path(struct ly_ctx *ctx, const char *path) {
    struct lys_module *mod = NULL;
    if (lys_parse_path(ctx, path, LYS_IN_YANG, &mod) != LY_SUCCESS) {
        return NULL;
    }
    return mod;
}

static const char *cam_grouping_origin(const struct lysc_node *node) {
    if (!node || !node->priv) {
        return NULL;
    }
    struct lysp_node *pnode = (struct lysp_node *)node->priv;
    while (pnode) {
        if (pnode->nodetype == LYS_GROUPING) {
            return pnode->name;
        }
        pnode = pnode->parent;
    }
    return NULL;
}

static int cam_ctx_has_priv_parsed_option(void) {
#ifdef LY_CTX_SET_PRIV_PARSED
    return 1;
#else
    return 0;
#endif
}

static struct lysc_node_action *cam_module_rpcs(const struct lys_module *mod) {
    return (mod && mod->compiled) ? mod->compiled->rpcs : NULL;
}

static struct lysc_node_notif *cam_module_notifs(const struct lys_module *mod) {
    return (mod && mod->compiled) ? mod->compiled->notifs : NULL;
}
*/
import "C" //nolint:gocritic // dupImport false positive: gocritic pairs the cgo "C" pseudo-import with "unsafe"

import (
	"fmt"
	"runtime"
	"strconv"
	"strings"
	"unsafe" //nolint:gocritic // dupImport false positive: gocritic pairs "unsafe" with the cgo "C" pseudo-import
)

// Schema node type constants from libyang's tree_schema.h. They are duplicated
// here as Go constants because cgo cannot see #define macros directly.
const (
	lysContainer = 0x0001
	lysChoice    = 0x0002
	lysLeaf      = 0x0004
	lysLeafList  = 0x0008
	lysList      = 0x0010
	lysAnyXml    = 0x0020
	lysAnyData   = 0x0060
	lysCase      = 0x0080
	lysRPC       = 0x0100
	lysAction    = 0x0200
	lysInput     = 0x1000
	lysOutput    = 0x2000
	lysNotif     = 0x0400

	lysConfigW    = 0x01
	lysConfigR    = 0x02
	lysConfigMask = 0x03

	lysStatusCurr  = 0x04
	lysStatusDeprc = 0x08
	lysStatusObslt = 0x10
	lysStatusMask  = 0x1C

	lysMandTrue  = 0x20
	lysMandFalse = 0x40
	lysPresence  = 0x80
	lysOrdByUser = 0x0040
	lysKey       = 0x0100

	lysSetMin = 0x0200
	lysSetMax = 0x0400

	lysDevNotSupported = 1
	lysDevAdd          = 2
	lysDevDelete       = 3
	lysDevReplace      = 4
)

// RawConfig is the compiled config flag for a schema node.
type RawConfig int

const (
	// RawConfigUnset means no explicit config statement.
	RawConfigUnset RawConfig = iota
	// RawConfigRw is config true.
	RawConfigRw
	// RawConfigRo is config false.
	RawConfigRo
)

// RawStatus is the compiled status substatement.
type RawStatus int

const (
	// RawStatusCurrent is status current.
	RawStatusCurrent RawStatus = iota
	// RawStatusDeprecated is status deprecated.
	RawStatusDeprecated
	// RawStatusObsolete is status obsolete.
	RawStatusObsolete
)

// RawBaseType is the precise built-in YANG base type.
type RawBaseType int

// Built-in YANG base types, mirroring libyang's LY_DATA_TYPE enum.
const (
	RawBaseTypeUnknown RawBaseType = iota
	RawBaseTypeString
	RawBaseTypeBoolean
	RawBaseTypeInt8
	RawBaseTypeInt16
	RawBaseTypeInt32
	RawBaseTypeInt64
	RawBaseTypeUint8
	RawBaseTypeUint16
	RawBaseTypeUint32
	RawBaseTypeUint64
	RawBaseTypeDecimal64
	RawBaseTypeEmpty
	RawBaseTypeBinary
	RawBaseTypeBits
	RawBaseTypeEnumeration
	RawBaseTypeIdentityRef
	RawBaseTypeInstanceIdentifier
	RawBaseTypeLeafRef
	RawBaseTypeUnion
)

// RawEnumValue is one enum or bit value in declaration order.
type RawEnumValue struct {
	Name  string
	Value int64
}

// RawPattern is a textual pattern constraint for strings.
type RawPattern struct {
	Regex       string
	ErrorAppTag *string
	Inverted    bool
}

// RawRangeBound is one bound of a range or length constraint.
type RawRangeBound struct {
	Min string
	Max string
}

// RawTypeInfo carries rich type constraints for a leaf or leaf-list.
type RawTypeInfo struct {
	BaseType         RawBaseType
	TypedefName      *string
	FractionDigits   *uint8
	Range            []RawRangeBound
	Length           []RawRangeBound
	Patterns         []RawPattern
	EnumValues       []RawEnumValue
	BitValues        []RawEnumValue
	IdentityBases    []string
	RequireInstance  *bool
	LeafrefPath      string
	LeafrefRealtype  *RawTypeInfo
	LeafrefTargetPtr unsafe.Pointer
	UnionTypes       []RawTypeInfo
}

// RawExtension is one compiled extension instance attached to a schema node.
type RawExtension struct {
	Name       string
	Argument   *string
	ModuleName string
}

// RawMust is a compiled must restriction.
type RawMust struct {
	Cond         string
	ErrorMessage *string
	ErrorAppTag  *string
	Description  *string
	Reference    *string
}

// RawWhen is a compiled when restriction.
type RawWhen struct {
	Cond        string
	Description *string
	Reference   *string
}

// RawSchemaNode is a rich, libyang-free description of one compiled schema node.
type RawSchemaNode struct {
	Name        string
	Kind        string
	Config      RawConfig
	Status      RawStatus
	Mandatory   bool
	Presence    bool
	Description *string
	Reference   *string
	Units       *string
	// DefaultValue is kept for legacy SchemaTree consumers; it holds the first
	// default value, if any. Prefer DefaultValues for leaf-list multi-defaults.
	DefaultValue  *string
	DefaultValues []string
	MinElements   *uint32
	MaxElements   *uint32
	OrderedByUser bool
	IsKey         bool
	// KeyNames is set only on list nodes; key leaves in key-statement order.
	KeyNames []string
	// KeyIndices is set only on list nodes; indices into Children of key leaves.
	KeyIndices []int
	// BaseType is set only on leaf/leaf-list nodes.
	BaseType RawBaseType
	// TypedefName is set only on leaf/leaf-list nodes.
	TypedefName *string
	// TypeInfo is set only on leaf/leaf-list nodes.
	TypeInfo RawTypeInfo
	Children []RawSchemaNode
	// Extensions are the compiled extension instances in declaration order.
	Extensions []RawExtension
	// Musts are the compiled must restrictions in declaration order.
	Musts []RawMust
	// Whens are the compiled when restrictions in declaration order.
	Whens []RawWhen
	// UniqueConstraints is set only on list nodes. Each inner slice is one
	// unique specification (a list of leaf pointers) in declaration order.
	UniqueConstraints [][]unsafe.Pointer
	// ModuleNs is set only on the synthetic module root.
	ModuleNs string
	// SchemaPtr is the raw compiled schema node pointer; NULL for the synthetic root.
	SchemaPtr unsafe.Pointer
	// OwnerModuleName is the module that owns this schema node. Augmented nodes may
	// live under another module's tree but still belong to the augmenting module.
	OwnerModuleName     string
	OwnerModuleRevision string
	OwnerModuleNs       string
	// LeafType is the legacy coarse classification kept for SchemaTree consumers.
	LeafType string
	// GroupingOrigin is set only when the node was instantiated from a uses of a
	// grouping; it holds the grouping name. Empty for directly defined nodes or
	// when LY_CTX_SET_PRIV_PARSED is unavailable.
	GroupingOrigin string
}

// RawDeviation is one parsed deviation statement from a module that is the
// deviation source. It exposes the parsed statement only; post-deviation
// compiled values are not reconstructed.
type RawDeviation struct {
	TargetPath   string
	SourceModule string
	Type         string // "not-supported", "add", "replace", "delete"
	Property     string
	NewValue     string
	Description  *string
	Reference    *string
}

// RawModuleInfo is module-level metadata extracted in one coarse walk.
type RawModuleInfo struct {
	Name          string
	Namespace     string
	Prefix        string
	Revision      *string
	HasParsed     bool
	IsImplemented bool
	Identities    []RawIdentity
	Imports       []RawImport
	AugmentedBy   []string
	DeviatedBy    []string
	Deviations    []RawDeviation
	// RPCs, Actions, and Notifications are the module-level operations. Actions
	// are stored alongside RPCs in libyang's compiled module; they are separated
	// here so the public API can distinguish top-level RPCs from top-level
	// actions. In YANG 1.1 actions are not valid at module top-level, so Actions
	// is normally empty, but it is exposed for completeness.
	RPCs          []RawSchemaNode
	Actions       []RawSchemaNode
	Notifications []RawSchemaNode
}

// RawImport is metadata for one import statement.
type RawImport struct {
	Prefix   string
	Name     string
	Revision string
}

// RawIdentity is metadata for one compiled identity.
type RawIdentity struct {
	Name       string
	ModuleName string
	Bases      []string
	Derived    []string
}

// RawModule pairs a module's metadata with its compiled schema tree root.
type RawModule struct {
	Info RawModuleInfo
	Root RawSchemaNode
}

func schemaKindName(nodetype uint16) string {
	switch nodetype {
	case lysContainer:
		return "container"
	case lysLeaf:
		return "leaf"
	case lysLeafList:
		return "leaflist"
	case lysList:
		return "list"
	case lysChoice:
		return "choice"
	case lysCase:
		return "case"
	case lysAnyXml:
		// anyxml (0x20) and anydata (0x60) are distinct libyang nodetypes and
		// surface as distinct public kinds (RFC 7950 section 7.11 anyxml,
		// section 7.10 anydata).
		return "anyxml"
	case lysAnyData:
		return "anydata"
	case lysRPC:
		return "rpc"
	case lysAction:
		return "action"
	case lysInput:
		return "input"
	case lysOutput:
		return "output"
	case lysNotif:
		return "notification"
	default:
		return "unknown"
	}
}

func leafBaseTypeName(node *C.struct_lysc_node) RawBaseType {
	return baseTypeFromRaw(uint32(C.cam_lysc_node_basetype(node)))
}

func leafTypeNameCoarse(base RawBaseType) string {
	switch base {
	case RawBaseTypeString:
		return "string"
	case RawBaseTypeBoolean:
		return "bool"
	case RawBaseTypeInt8, RawBaseTypeInt16, RawBaseTypeInt32, RawBaseTypeInt64,
		RawBaseTypeUint8, RawBaseTypeUint16, RawBaseTypeUint32, RawBaseTypeUint64,
		RawBaseTypeDecimal64:
		return "int"
	default:
		return "unknown"
	}
}

type rangeKind int

const (
	rangeSigned rangeKind = iota
	rangeUnsigned
	rangeDecimal64
)

func extractRange(rangePtr *C.struct_lysc_range, kind rangeKind, fractionDigits uint8) []RawRangeBound {
	count := C.cam_sized_array_count(unsafe.Pointer(C.cam_range_parts(rangePtr)))
	if count == 0 {
		return nil
	}
	out := make([]RawRangeBound, 0, count)
	for i := C.uint64_t(0); i < count; i++ {
		part := C.cam_range_part_at(rangePtr, i)
		var minStr, maxStr string
		switch kind {
		case rangeSigned:
			minStr = strconv.FormatInt(int64(C.cam_range_part_min64(part)), 10)
			maxStr = strconv.FormatInt(int64(C.cam_range_part_max64(part)), 10)
		case rangeUnsigned:
			minStr = strconv.FormatUint(uint64(C.cam_range_part_minu64(part)), 10)
			maxStr = strconv.FormatUint(uint64(C.cam_range_part_maxu64(part)), 10)
		case rangeDecimal64:
			minStr = FormatDecimal64(int64(C.cam_range_part_min64(part)), fractionDigits, false)
			maxStr = FormatDecimal64(int64(C.cam_range_part_max64(part)), fractionDigits, false)
		}
		out = append(out, RawRangeBound{Min: minStr, Max: maxStr})
	}
	return out
}

// FormatDecimal64 renders a fixed-point decimal64 value to its canonical string.
// When trimTrailingZeros is set, fractional trailing zeros are dropped (keeping a
// lone "0"), matching the RFC-7950 canonical leaf form; otherwise the full
// fraction width is preserved (used for range-bound display).
func FormatDecimal64(raw int64, fractionDigits uint8, trimTrailingZeros bool) string {
	if fractionDigits == 0 {
		return strconv.FormatInt(raw, 10)
	}
	divisor := int64(1)
	for i := uint8(0); i < fractionDigits; i++ {
		divisor *= 10
	}
	whole := raw / divisor
	frac := raw % divisor
	if frac < 0 {
		frac = -frac
	}
	padded := strconv.FormatInt(frac, 10)
	width := int(fractionDigits)
	if len(padded) < width {
		padded = strings.Repeat("0", width-len(padded)) + padded
	} else if len(padded) > width {
		padded = padded[:width]
	}
	if trimTrailingZeros {
		padded = strings.TrimRight(padded, "0")
		if padded == "" {
			padded = "0"
		}
	}
	if raw < 0 {
		return fmt.Sprintf("-%d.%s", -whole, padded)
	}
	return fmt.Sprintf("%d.%s", whole, padded)
}

func extractPatterns(patternsPtr **C.struct_lysc_pattern) []RawPattern {
	count := C.cam_sized_array_count(unsafe.Pointer(patternsPtr))
	if count == 0 {
		return nil
	}
	out := make([]RawPattern, 0, count)
	for i := C.uint64_t(0); i < count; i++ {
		p := C.cam_pattern_at(patternsPtr, i)
		if p == nil {
			continue
		}
		out = append(out, RawPattern{
			Regex:       C.GoString(C.cam_pattern_expr(p)),
			ErrorAppTag: cstrOpt(C.cam_pattern_eapptag(p)),
			Inverted:    C.cam_pattern_inverted(p) != 0,
		})
	}
	return out
}

func extractDefaults(node *C.struct_lysc_node) []string {
	count := C.cam_node_dflt_count(node)
	if count == 0 {
		return nil
	}
	out := make([]string, 0, count)
	for i := C.uint64_t(0); i < count; i++ {
		out = append(out, cstrValue(C.cam_node_dflt_at(node, i)))
	}
	return out
}

func extractExtensions(node *C.struct_lysc_node) []RawExtension {
	exts := C.cam_node_exts(node)
	count := C.cam_sized_array_count(unsafe.Pointer(exts))
	if count == 0 {
		return nil
	}
	out := make([]RawExtension, 0, count)
	for i := C.uint64_t(0); i < count; i++ {
		e := C.cam_ext_at(exts, i)
		if e == nil {
			continue
		}
		out = append(out, RawExtension{
			Name:       cstrValue(C.cam_ext_name(e)),
			Argument:   cstrOpt(C.cam_ext_argument(e)),
			ModuleName: cstrValue(C.cam_ext_module_name(e)),
		})
	}
	return out
}

func extractMusts(node *C.struct_lysc_node) []RawMust {
	musts := C.cam_node_musts(node)
	count := C.cam_sized_array_count(unsafe.Pointer(musts))
	if count == 0 {
		return nil
	}
	out := make([]RawMust, 0, count)
	for i := C.uint64_t(0); i < count; i++ {
		m := C.cam_must_at(musts, i)
		if m == nil {
			continue
		}
		out = append(out, RawMust{
			Cond:         cstrValue(C.cam_must_cond(m)),
			ErrorMessage: cstrOpt(C.cam_must_emsg(m)),
			ErrorAppTag:  cstrOpt(C.cam_must_eapptag(m)),
			Description:  cstrOpt(C.cam_must_dsc(m)),
			Reference:    cstrOpt(C.cam_must_ref(m)),
		})
	}
	return out
}

func extractWhens(node *C.struct_lysc_node) []RawWhen {
	whens := C.cam_node_whens(node)
	count := C.cam_sized_array_count(unsafe.Pointer(whens))
	if count == 0 {
		return nil
	}
	out := make([]RawWhen, 0, count)
	for i := C.uint64_t(0); i < count; i++ {
		w := C.cam_when_at(whens, i)
		if w == nil {
			continue
		}
		out = append(out, RawWhen{
			Cond:        cstrValue(C.cam_when_cond(w)),
			Description: cstrOpt(C.cam_when_dsc(w)),
			Reference:   cstrOpt(C.cam_when_ref(w)),
		})
	}
	return out
}

func extractUniqueConstraints(node *C.struct_lysc_node) [][]unsafe.Pointer {
	arr := C.cam_node_uniques(node)
	count := C.cam_sized_array_count(unsafe.Pointer(arr))
	if count == 0 {
		return nil
	}
	out := make([][]unsafe.Pointer, 0, count)
	for i := C.uint64_t(0); i < count; i++ {
		inner := C.cam_unique_leaf_array_at(arr, i)
		innerCount := C.cam_sized_array_count(unsafe.Pointer(inner))
		ptrs := make([]unsafe.Pointer, 0, innerCount)
		for j := C.uint64_t(0); j < innerCount; j++ {
			leaf := C.cam_unique_leaf_at(inner, j)
			if leaf != nil {
				ptrs = append(ptrs, unsafe.Pointer(leaf))
			}
		}
		out = append(out, ptrs)
	}
	return out
}

func builtinTypeName(base RawBaseType) string {
	switch base {
	case RawBaseTypeString:
		return "string"
	case RawBaseTypeBoolean:
		return "boolean"
	case RawBaseTypeInt8:
		return "int8"
	case RawBaseTypeInt16:
		return "int16"
	case RawBaseTypeInt32:
		return "int32"
	case RawBaseTypeInt64:
		return "int64"
	case RawBaseTypeUint8:
		return "uint8"
	case RawBaseTypeUint16:
		return "uint16"
	case RawBaseTypeUint32:
		return "uint32"
	case RawBaseTypeUint64:
		return "uint64"
	case RawBaseTypeDecimal64:
		return "decimal64"
	case RawBaseTypeEmpty:
		return "empty"
	case RawBaseTypeBinary:
		return "binary"
	case RawBaseTypeBits:
		return "bits"
	case RawBaseTypeEnumeration:
		return "enumeration"
	case RawBaseTypeIdentityRef:
		return "identityref"
	case RawBaseTypeInstanceIdentifier:
		return "instance-identifier"
	case RawBaseTypeLeafRef:
		return "leafref"
	case RawBaseTypeUnion:
		return "union"
	default:
		return ""
	}
}

func typedefNameFromType(typePtr *C.struct_lysc_type) *string {
	if typePtr == nil {
		return nil
	}
	name := C.GoString(C.cam_type_name(typePtr))
	builtin := builtinTypeName(baseTypeFromRaw(uint32(C.cam_type_basetype(typePtr))))
	if name == "" || name == builtin {
		return nil
	}
	s := name
	return &s
}

func baseTypeFromRaw(bt uint32) RawBaseType {
	// Values match libyang's enum LY_DATA_TYPE declaration order.
	switch bt {
	case 1:
		return RawBaseTypeBinary
	case 2:
		return RawBaseTypeUint8
	case 3:
		return RawBaseTypeUint16
	case 4:
		return RawBaseTypeUint32
	case 5:
		return RawBaseTypeUint64
	case 6:
		return RawBaseTypeString
	case 7:
		return RawBaseTypeBits
	case 8:
		return RawBaseTypeBoolean
	case 9:
		return RawBaseTypeDecimal64
	case 10:
		return RawBaseTypeEmpty
	case 11:
		return RawBaseTypeEnumeration
	case 12:
		return RawBaseTypeIdentityRef
	case 13:
		return RawBaseTypeInstanceIdentifier
	case 14:
		return RawBaseTypeLeafRef
	case 15:
		return RawBaseTypeUnion
	case 16:
		return RawBaseTypeInt8
	case 17:
		return RawBaseTypeInt16
	case 18:
		return RawBaseTypeInt32
	case 19:
		return RawBaseTypeInt64
	default:
		return RawBaseTypeUnknown
	}
}

func extractTypeInfo(typePtr *C.struct_lysc_type) RawTypeInfo {
	if typePtr == nil {
		return RawTypeInfo{BaseType: RawBaseTypeUnknown}
	}
	base := baseTypeFromRaw(uint32(C.cam_type_basetype(typePtr)))
	info := RawTypeInfo{
		BaseType:    base,
		TypedefName: typedefNameFromType(typePtr),
	}
	switch base {
	case RawBaseTypeInt8, RawBaseTypeInt16, RawBaseTypeInt32, RawBaseTypeInt64:
		info.Range = extractRange(C.cam_type_num_range(typePtr), rangeSigned, 0)
	case RawBaseTypeUint8, RawBaseTypeUint16, RawBaseTypeUint32, RawBaseTypeUint64:
		info.Range = extractRange(C.cam_type_num_range(typePtr), rangeUnsigned, 0)
	case RawBaseTypeDecimal64:
		fd := uint8(C.cam_type_dec_fraction_digits(typePtr))
		info.FractionDigits = &fd
		info.Range = extractRange(C.cam_type_dec_range(typePtr), rangeDecimal64, fd)
	case RawBaseTypeString:
		info.Length = extractRange(C.cam_type_str_length(typePtr), rangeUnsigned, 0)
		info.Patterns = extractPatterns(C.cam_type_str_patterns(typePtr))
	case RawBaseTypeBinary:
		info.Length = extractRange(C.cam_type_bin_length(typePtr), rangeUnsigned, 0)
	case RawBaseTypeEnumeration:
		enums := C.cam_type_enum_enums(typePtr)
		count := C.cam_sized_array_count(unsafe.Pointer(enums))
		for i := C.uint64_t(0); i < count; i++ {
			item := C.cam_bitenum_at(enums, i)
			info.EnumValues = append(info.EnumValues, RawEnumValue{
				Name:  C.GoString(C.cam_bitenum_name(item)),
				Value: int64(C.cam_enum_value(item)),
			})
		}
	case RawBaseTypeBits:
		bits := C.cam_type_bits_bits(typePtr)
		count := C.cam_sized_array_count(unsafe.Pointer(bits))
		for i := C.uint64_t(0); i < count; i++ {
			item := C.cam_bitenum_at(bits, i)
			info.BitValues = append(info.BitValues, RawEnumValue{
				Name:  C.GoString(C.cam_bitenum_name(item)),
				Value: int64(C.cam_bit_position(item)),
			})
		}
	case RawBaseTypeIdentityRef:
		bases := C.cam_type_ident_bases(typePtr)
		count := C.cam_sized_array_count(unsafe.Pointer(bases))
		for i := C.uint64_t(0); i < count; i++ {
			baseIdent := C.cam_ident_base_at(bases, i)
			info.IdentityBases = append(info.IdentityBases, formatIdentityName(baseIdent))
		}
	case RawBaseTypeInstanceIdentifier:
		req := C.cam_instanceid_require_instance((*C.struct_lysc_type_instanceid)(unsafe.Pointer(typePtr))) != 0
		info.RequireInstance = &req
	case RawBaseTypeLeafRef:
		lr := (*C.struct_lysc_type_leafref)(unsafe.Pointer(typePtr))
		req := C.cam_leafref_require_instance(lr) != 0
		info.RequireInstance = &req
		if path := C.cam_leafref_path(typePtr); path != nil {
			info.LeafrefPath = cstrValue(path)
		}
		if realType := C.cam_leafref_realtype(lr); realType != nil {
			rt := extractTypeInfo(realType)
			info.LeafrefRealtype = &rt
		}
	case RawBaseTypeUnion:
		types := C.cam_type_union_types(typePtr)
		count := C.cam_sized_array_count(unsafe.Pointer(types))
		for i := C.uint64_t(0); i < count; i++ {
			member := C.cam_union_type_at(types, i)
			info.UnionTypes = append(info.UnionTypes, extractTypeInfo(member))
		}
	}
	return info
}

func leafTypeInfo(node *C.struct_lysc_node) RawTypeInfo {
	if node == nil {
		return RawTypeInfo{BaseType: RawBaseTypeUnknown}
	}
	nt := uint16(node.nodetype)
	if nt != lysLeaf && nt != lysLeafList {
		return RawTypeInfo{BaseType: RawBaseTypeUnknown}
	}
	var typePtr *C.struct_lysc_type
	if nt == lysLeaf {
		typePtr = C.cam_node_leaf_type(node)
	} else {
		typePtr = C.cam_node_leaflist_type(node)
	}
	info := extractTypeInfo(typePtr)
	if info.BaseType == RawBaseTypeLeafRef {
		if target := C.cam_leafref_target(node); target != nil {
			info.LeafrefTargetPtr = unsafe.Pointer(target)
		}
	}
	return info
}

func cstrOpt(ptr *C.char) *string {
	if ptr == nil {
		return nil
	}
	s := C.GoString(ptr)
	return &s
}

func cstrValue(ptr *C.char) string {
	if ptr == nil {
		return ""
	}
	return C.GoString(ptr)
}

func formatIdentityName(ident *C.struct_lysc_ident) string {
	if ident == nil {
		return ""
	}
	name := cstrValue(C.cam_ident_name(ident))
	mod := C.cam_ident_module(ident)
	if mod == nil {
		return name
	}
	modName := cstrValue(mod.name)
	if modName == "" {
		return name
	}
	return fmt.Sprintf("%s:%s", modName, name)
}

func walkSchemaSiblings(first, parent *C.struct_lysc_node) []RawSchemaNode {
	var out []RawSchemaNode
	cur := first
	for cur != nil && cur.parent == parent {
		flags := uint32(cur.flags)
		nt := uint16(cur.nodetype)

		var cfg RawConfig
		switch flags & lysConfigMask {
		case lysConfigR:
			cfg = RawConfigRo
		case lysConfigW:
			cfg = RawConfigRw
		default:
			cfg = RawConfigUnset
		}

		var status RawStatus
		switch flags & lysStatusMask {
		case lysStatusDeprc:
			status = RawStatusDeprecated
		case lysStatusObslt:
			status = RawStatusObsolete
		default:
			status = RawStatusCurrent
		}

		var minVal, maxVal uint32
		C.cam_lysc_node_min_max(cur, (*C.uint32_t)(unsafe.Pointer(&minVal)), (*C.uint32_t)(unsafe.Pointer(&maxVal)))
		var minElements, maxElements *uint32
		if nt == lysList || nt == lysLeafList {
			if minVal != 0 {
				minElements = &minVal
			}
			if maxVal != 0 {
				// libyang's compiled tree stores UINT32_MAX for an unbounded
				// max-elements (only the parsed tree uses 0); treat it as no bound.
				if maxVal != ^uint32(0) {
					maxElements = &maxVal
				}
			}
		}

		var childFirst *C.struct_lysc_node
		if child := C.lysc_node_child(cur); child != nil {
			childFirst = child
		}
		children := walkSchemaSiblings(childFirst, cur)
		if actions := C.lysc_node_actions(cur); actions != nil {
			children = append(children, walkSchemaSiblings((*C.struct_lysc_node)(unsafe.Pointer(actions)), cur)...)
		}
		if notifs := C.lysc_node_notifs(cur); notifs != nil {
			children = append(children, walkSchemaSiblings((*C.struct_lysc_node)(unsafe.Pointer(notifs)), cur)...)
		}

		var keyNames []string
		var keyIndices []int
		if nt == lysList {
			for i, k := range children {
				if k.IsKey {
					keyNames = append(keyNames, k.Name)
					keyIndices = append(keyIndices, i)
				}
			}
		}

		baseType := leafBaseTypeName(cur)
		typeInfo := leafTypeInfo(cur)
		defaults := extractDefaults(cur)
		var defaultValue *string
		if len(defaults) > 0 {
			defaultValue = &defaults[0]
		}
		ownerMod := C.cam_node_module(cur)
		ownerModRevision := cstrValue(C.cam_module_revision(ownerMod))

		var groupingOrigin string
		if C.cam_ctx_has_priv_parsed_option() != 0 {
			if g := C.cam_grouping_origin(cur); g != nil {
				groupingOrigin = C.GoString(g)
			}
		}

		out = append(out, RawSchemaNode{
			Name:      C.GoString(cur.name),
			Kind:      schemaKindName(nt),
			Config:    cfg,
			Status:    status,
			Mandatory: (flags & lysMandTrue) != 0,
			// LYS_PRESENCE (0x80) aliases LYS_ORDBY_SYSTEM (0x80), which libyang
			// sets on every system-ordered list/leaf-list. Gate to containers so
			// only true presence containers report presence (mirrors libyang's
			// own lysc_is_np_cont, which checks nodetype == LYS_CONTAINER).
			Presence:            nt == lysContainer && (flags&lysPresence) != 0,
			Description:         cstrOpt(C.cam_lysc_node_dsc(cur)),
			Reference:           cstrOpt(C.cam_lysc_node_ref(cur)),
			Units:               cstrOpt(C.cam_lysc_node_units(cur)),
			DefaultValue:        defaultValue,
			DefaultValues:       defaults,
			MinElements:         minElements,
			MaxElements:         maxElements,
			OrderedByUser:       (flags & lysOrdByUser) != 0,
			IsKey:               (flags & lysKey) != 0,
			KeyNames:            keyNames,
			KeyIndices:          keyIndices,
			BaseType:            baseType,
			TypedefName:         typeInfo.TypedefName,
			TypeInfo:            typeInfo,
			Children:            children,
			Extensions:          extractExtensions(cur),
			Musts:               extractMusts(cur),
			Whens:               extractWhens(cur),
			UniqueConstraints:   extractUniqueConstraints(cur),
			SchemaPtr:           unsafe.Pointer(cur),
			OwnerModuleName:     cstrValue(C.cam_module_name(ownerMod)),
			OwnerModuleRevision: ownerModRevision,
			OwnerModuleNs:       cstrValue(C.cam_module_ns(ownerMod)),
			LeafType:            leafTypeNameCoarse(baseType),
			GroupingOrigin:      groupingOrigin,
		})
		cur = cur.next
	}
	return out
}

func moduleIdentities(mod *C.struct_lys_module) []RawIdentity {
	count := C.cam_sized_array_count(unsafe.Pointer(mod.identities))
	if count == 0 {
		return nil
	}
	out := make([]RawIdentity, 0, count)
	for i := C.uint64_t(0); i < count; i++ {
		ident := C.cam_module_identity_at(mod.identities, i)
		if ident == nil {
			continue
		}
		name := cstrValue(C.cam_ident_name(ident))
		modName := ""
		if imod := C.cam_ident_module(ident); imod != nil {
			modName = cstrValue(imod.name)
		}
		var bases []string
		if pident := C.cam_parsed_identity_by_name(mod, C.cam_ident_name(ident)); pident != nil {
			baseArr := C.cam_parsed_identity_bases(pident)
			baseCount := C.cam_sized_array_count(unsafe.Pointer(baseArr))
			for j := C.uint64_t(0); j < baseCount; j++ {
				basePtr := C.cam_identity_base_at(baseArr, j)
				base := cstrValue(basePtr)
				if base != "" {
					if baseModule := cstrValue(C.cam_parsed_identity_base_module_name(mod, pident, basePtr)); baseModule != "" {
						local := base
						if _, after, ok := strings.Cut(base, ":"); ok {
							local = after
						}
						base = baseModule + ":" + local
					}
					bases = append(bases, base)
				}
			}
		}
		var derived []string
		derivedCount := C.cam_sized_array_count(unsafe.Pointer(ident.derived))
		for j := C.uint64_t(0); j < derivedCount; j++ {
			d := C.cam_ident_derived_at(ident.derived, j)
			if d == nil {
				continue
			}
			derived = append(derived, formatIdentityName(d))
		}
		out = append(out, RawIdentity{
			Name:       name,
			ModuleName: modName,
			Bases:      bases,
			Derived:    derived,
		})
	}
	return out
}

func moduleImports(mod *C.struct_lys_module) []RawImport {
	imports := C.cam_module_imports(mod)
	count := C.cam_sized_array_count(unsafe.Pointer(imports))
	if count == 0 {
		return nil
	}
	out := make([]RawImport, 0, count)
	for i := C.uint64_t(0); i < count; i++ {
		imp := C.cam_import_at(imports, i)
		if imp == nil {
			continue
		}
		out = append(out, RawImport{
			Prefix:   cstrValue(C.cam_import_prefix(imp)),
			Name:     cstrValue(C.cam_import_name(imp)),
			Revision: cstrValue(C.cam_import_revision(imp)),
		})
	}
	return out
}

func moduleProvenance(arr **C.struct_lys_module) []string {
	count := C.cam_sized_array_count(unsafe.Pointer(arr))
	if count == 0 {
		return nil
	}
	out := make([]string, 0, count)
	for i := C.uint64_t(0); i < count; i++ {
		m := C.cam_module_ptr_at(arr, i)
		if m == nil {
			continue
		}
		out = append(out, cstrValue(C.cam_module_name(m)))
	}
	return out
}

func moduleDeviations(mod *C.struct_lys_module) []RawDeviation {
	devs := C.cam_module_deviations(mod)
	count := C.cam_sized_array_count(unsafe.Pointer(devs))
	if count == 0 {
		return nil
	}
	sourceModule := cstrValue(C.cam_module_name(mod))
	out := make([]RawDeviation, 0, count)
	for i := C.uint64_t(0); i < count; i++ {
		dev := C.cam_deviation_at(devs, i)
		if dev == nil {
			continue
		}
		targetPath := cstrValue(C.cam_deviation_nodeid(dev))
		dsc := cstrOpt(C.cam_deviation_dsc(dev))
		ref := cstrOpt(C.cam_deviation_ref(dev))
		for d := C.cam_deviation_deviates(dev); d != nil; d = C.cam_deviate_next(d) {
			modType := uint8(C.cam_deviate_mod(d))
			switch modType {
			case lysDevNotSupported:
				out = append(out, RawDeviation{
					TargetPath:   targetPath,
					SourceModule: sourceModule,
					Type:         "not-supported",
					Description:  dsc,
					Reference:    ref,
				})
			case lysDevAdd, lysDevDelete:
				devType := "add"
				if modType == lysDevDelete {
					devType = "delete"
				}
				out = append(out, deviateAddDelEntries(d, devType, targetPath, sourceModule, dsc, ref)...)
			case lysDevReplace:
				out = append(out, deviateReplaceEntries(d, targetPath, sourceModule, dsc, ref)...)
			}
		}
	}
	return out
}

func deviateAddDelEntries(d *C.struct_lysp_deviate, devType, targetPath, sourceModule string, dsc, ref *string) []RawDeviation {
	if devType == "delete" {
		return deviateDeleteEntries(d, targetPath, sourceModule, dsc, ref)
	}
	return deviateAddEntries(d, targetPath, sourceModule, dsc, ref)
}

func deviateAddEntries(d *C.struct_lysp_deviate, targetPath, sourceModule string, dsc, ref *string) []RawDeviation {
	var out []RawDeviation
	base := RawDeviation{
		TargetPath:   targetPath,
		SourceModule: sourceModule,
		Type:         "add",
		Description:  dsc,
		Reference:    ref,
	}
	if units := cstrValue(C.cam_deviate_add_units(d)); units != "" {
		out = append(out, baseWith(base, "units", units))
	}
	if musts := C.cam_deviate_add_musts(d); musts != nil {
		mustCount := C.cam_sized_array_count(unsafe.Pointer(musts))
		for j := C.uint64_t(0); j < mustCount; j++ {
			m := C.cam_deviate_must_at(musts, j)
			if m == nil {
				continue
			}
			out = append(out, baseWith(base, "must", cstrValue(C.cam_restr_cond(m))))
		}
	}
	if uniques := C.cam_deviate_add_uniques(d); uniques != nil {
		uniqueCount := C.cam_sized_array_count(unsafe.Pointer(uniques))
		for j := C.uint64_t(0); j < uniqueCount; j++ {
			q := qnameAt(uniques, j)
			if q == nil {
				continue
			}
			out = append(out, baseWith(base, "unique", cstrValue(C.cam_qname_str(q))))
		}
	}
	if dflts := C.cam_deviate_add_dflts(d); dflts != nil {
		dfltCount := C.cam_sized_array_count(unsafe.Pointer(dflts))
		for j := C.uint64_t(0); j < dfltCount; j++ {
			q := qnameAt(dflts, j)
			if q == nil {
				continue
			}
			out = append(out, baseWith(base, "default", cstrValue(C.cam_qname_str(q))))
		}
	}
	flags := uint16(C.cam_deviate_add_flags(d))
	out = append(out, deviateFlagEntries(base, flags)...)
	minVal := uint32(C.cam_deviate_add_min(d))
	maxVal := uint32(C.cam_deviate_add_max(d))
	if flags&lysSetMin != 0 {
		out = append(out, baseWith(base, "min-elements", strconv.FormatUint(uint64(minVal), 10)))
	}
	if flags&lysSetMax != 0 {
		if maxVal == 0 {
			out = append(out, baseWith(base, "max-elements", "unbounded"))
		} else {
			out = append(out, baseWith(base, "max-elements", strconv.FormatUint(uint64(maxVal), 10)))
		}
	}
	return out
}

func deviateDeleteEntries(d *C.struct_lysp_deviate, targetPath, sourceModule string, dsc, ref *string) []RawDeviation {
	var out []RawDeviation
	base := RawDeviation{
		TargetPath:   targetPath,
		SourceModule: sourceModule,
		Type:         "delete",
		Description:  dsc,
		Reference:    ref,
	}
	if units := cstrValue(C.cam_deviate_del_units(d)); units != "" {
		out = append(out, baseWith(base, "units", units))
	}
	if musts := C.cam_deviate_del_musts(d); musts != nil {
		mustCount := C.cam_sized_array_count(unsafe.Pointer(musts))
		for j := C.uint64_t(0); j < mustCount; j++ {
			m := C.cam_deviate_must_at(musts, j)
			if m == nil {
				continue
			}
			out = append(out, baseWith(base, "must", cstrValue(C.cam_restr_cond(m))))
		}
	}
	if uniques := C.cam_deviate_del_uniques(d); uniques != nil {
		uniqueCount := C.cam_sized_array_count(unsafe.Pointer(uniques))
		for j := C.uint64_t(0); j < uniqueCount; j++ {
			q := qnameAt(uniques, j)
			if q == nil {
				continue
			}
			out = append(out, baseWith(base, "unique", cstrValue(C.cam_qname_str(q))))
		}
	}
	if dflts := C.cam_deviate_del_dflts(d); dflts != nil {
		dfltCount := C.cam_sized_array_count(unsafe.Pointer(dflts))
		for j := C.uint64_t(0); j < dfltCount; j++ {
			q := qnameAt(dflts, j)
			if q == nil {
				continue
			}
			out = append(out, baseWith(base, "default", cstrValue(C.cam_qname_str(q))))
		}
	}
	return out
}

func deviateReplaceEntries(d *C.struct_lysp_deviate, targetPath, sourceModule string, dsc, ref *string) []RawDeviation {
	var out []RawDeviation
	base := RawDeviation{
		TargetPath:   targetPath,
		SourceModule: sourceModule,
		Type:         "replace",
		Description:  dsc,
		Reference:    ref,
	}
	if typ := C.cam_deviate_rpl_type(d); typ != nil {
		out = append(out, baseWith(base, "type", cstrValue(C.cam_lysp_type_name(typ))))
	}
	if units := cstrValue(C.cam_deviate_rpl_units(d)); units != "" {
		out = append(out, baseWith(base, "units", units))
	}
	flags := uint16(C.cam_deviate_rpl_flags(d))
	dflt := C.cam_deviate_rpl_dflt(d)
	if dflt != nil {
		out = append(out, baseWith(base, "default", cstrValue(dflt)))
	}
	out = append(out, deviateFlagEntries(base, flags)...)
	minVal := uint32(C.cam_deviate_rpl_min(d))
	maxVal := uint32(C.cam_deviate_rpl_max(d))
	if flags&lysSetMin != 0 {
		out = append(out, baseWith(base, "min-elements", strconv.FormatUint(uint64(minVal), 10)))
	}
	if flags&lysSetMax != 0 {
		if maxVal == 0 {
			out = append(out, baseWith(base, "max-elements", "unbounded"))
		} else {
			out = append(out, baseWith(base, "max-elements", strconv.FormatUint(uint64(maxVal), 10)))
		}
	}
	return out
}

func deviateFlagEntries(base RawDeviation, flags uint16) []RawDeviation {
	var out []RawDeviation
	switch flags & lysConfigMask {
	case lysConfigW:
		out = append(out, baseWith(base, "config", "true"))
	case lysConfigR:
		out = append(out, baseWith(base, "config", "false"))
	}
	if flags&lysMandTrue != 0 {
		out = append(out, baseWith(base, "mandatory", "true"))
	} else if flags&lysMandFalse != 0 {
		out = append(out, baseWith(base, "mandatory", "false"))
	}
	return out
}

func qnameAt(arr *C.struct_lysp_qname, i C.uint64_t) *C.struct_lysp_qname {
	return (*C.struct_lysp_qname)(unsafe.Pointer(uintptr(unsafe.Pointer(arr)) + uintptr(i)*unsafe.Sizeof(*arr)))
}

func baseWith(base RawDeviation, property, value string) RawDeviation {
	base.Property = property
	base.NewValue = value
	return base
}

func moduleInfo(mod *C.struct_lys_module) RawModuleInfo {
	rev := cstrOpt(mod.revision)
	var rpcs, actions []RawSchemaNode
	for _, op := range walkSchemaSiblings((*C.struct_lysc_node)(unsafe.Pointer(C.cam_module_rpcs(mod))), nil) {
		switch op.Kind {
		case "rpc":
			rpcs = append(rpcs, op)
		case "action":
			actions = append(actions, op)
		}
	}
	return RawModuleInfo{
		Name:          cstrValue(mod.name),
		Namespace:     cstrValue(mod.ns),
		Prefix:        cstrValue(mod.prefix),
		Revision:      rev,
		HasParsed:     C.cam_module_has_parsed(mod) != 0,
		IsImplemented: mod.implemented != 0,
		Identities:    moduleIdentities(mod),
		Imports:       moduleImports(mod),
		AugmentedBy:   moduleProvenance(C.cam_module_augmented_by(mod)),
		DeviatedBy:    moduleProvenance(C.cam_module_deviated_by(mod)),
		Deviations:    moduleDeviations(mod),
		RPCs:          rpcs,
		Actions:       actions,
		Notifications: walkSchemaSiblings((*C.struct_lysc_node)(unsafe.Pointer(C.cam_module_notifs(mod))), nil),
	}
}

// SchemaTree walks the compiled schema tree for the implemented module named
// `module` and returns it under a synthetic module root. The walk chases
// lysc_node.next/lysc_node_child pointers in C memory and only builds Go
// values after the traversal is complete. This is the legacy v1 view.
func (c *RawContext) SchemaTree(module string) (*RawSchemaNode, error) {
	// Pin the finalizer-bearing context across the whole C-pointer walk; the
	// schema memory we chase is owned by c.ctx and freed by its finalizer.
	defer runtime.KeepAlive(c)
	cname := C.CString(module)
	defer C.free(unsafe.Pointer(cname))

	mod := C.ly_ctx_get_module_implemented(c.ctx, cname)
	if mod == nil {
		return nil, fmt.Errorf("module not found: %s", module)
	}
	ns := ""
	if mod.ns != nil {
		ns = C.GoString(mod.ns)
	}
	compiled := mod.compiled
	if compiled == nil {
		return nil, fmt.Errorf("module %s is not implemented (no compiled schema)", module)
	}
	root := compiled.data
	if root == nil {
		return nil, fmt.Errorf("module %s has no data nodes", module)
	}
	return &RawSchemaNode{
		Name:     "",
		Kind:     "module",
		Children: walkSchemaSiblings(root, nil),
		ModuleNs: ns,
	}, nil
}

// SchemaModule walks the compiled schema tree for the implemented module named
// `module` and returns rich module metadata plus the synthetic schema root.
func (c *RawContext) SchemaModule(module string) (RawModuleInfo, *RawSchemaNode, error) {
	defer runtime.KeepAlive(c)
	cname := C.CString(module)
	defer C.free(unsafe.Pointer(cname))

	mod := C.ly_ctx_get_module_implemented(c.ctx, cname)
	if mod == nil {
		return RawModuleInfo{}, nil, fmt.Errorf("module not found: %s", module)
	}
	info := moduleInfo(mod)
	compiled := mod.compiled
	if compiled == nil {
		return RawModuleInfo{}, nil, fmt.Errorf("module %s is not implemented (no compiled schema)", module)
	}
	root := compiled.data
	if root == nil {
		return RawModuleInfo{}, nil, fmt.Errorf("module %s has no data nodes", module)
	}
	children := walkSchemaSiblings(root, nil)
	return info, &RawSchemaNode{
		Name:     "",
		Kind:     "module",
		Children: children,
		ModuleNs: info.Namespace,
	}, nil
}

// SchemaModules enumerates every loaded module with its compiled schema tree.
// Modules that define identities but no data nodes are included because
// identityrefs in implemented modules can depend on them.
func (c *RawContext) SchemaModules() ([]RawModule, error) {
	defer runtime.KeepAlive(c)
	var out []RawModule
	var idx C.uint32_t
	for {
		mod := C.ly_ctx_get_module_iter(c.ctx, &idx)
		if mod == nil {
			break
		}
		info := moduleInfo(mod)
		if mod.compiled == nil && !info.HasParsed && len(info.Identities) == 0 && len(info.Imports) == 0 {
			continue
		}
		var children []RawSchemaNode
		if mod.compiled != nil && mod.compiled.data != nil {
			children = walkSchemaSiblings(mod.compiled.data, nil)
		}
		out = append(out, RawModule{
			Info: info,
			Root: RawSchemaNode{
				Name:     "",
				Kind:     "module",
				Children: children,
				ModuleNs: info.Namespace,
			},
		})
	}
	return out, nil
}

// LoadModuleFromPath loads a YANG module from a file path into the context.
// The file is parsed as YANG format.
func (c *RawContext) LoadModuleFromPath(path string) error {
	defer runtime.KeepAlive(c)
	cpath := C.CString(path)
	defer C.free(unsafe.Pointer(cpath))
	mod := C.cam_load_module_path(c.ctx, cpath)
	if mod == nil {
		return lyError(c.ctx, fmt.Sprintf("load module from path %q", path), 0)
	}
	return nil
}

// Modules returns metadata for every implemented module in the context. The
// iteration is one coarse FFI walk of ly_ctx_get_module_iter.
func (c *RawContext) Modules() []RawModuleInfo {
	defer runtime.KeepAlive(c)
	var out []RawModuleInfo
	var idx C.uint32_t
	for {
		mod := C.ly_ctx_get_module_iter(c.ctx, &idx)
		if mod == nil {
			break
		}
		if mod.implemented == 0 {
			continue
		}
		out = append(out, moduleInfo(mod))
	}
	return out
}
