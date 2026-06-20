#include <libyang/libyang.h>
#include <libyang/metadata.h>

const char *cam_lyd_get_value(const struct lyd_node *node) {
    return lyd_get_value(node);
}

const char *cam_lyd_get_meta_value(const struct lyd_meta *meta) {
    return lyd_get_meta_value(meta);
}
