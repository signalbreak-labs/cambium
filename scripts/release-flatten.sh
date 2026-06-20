#!/usr/bin/env bash
# Release-flatten: copy the vendored libyang + PCRE2 C source into each
# publishable unit so the published crates/module work without submodules
# (crates.io excludes submodules; `go get` does not clone them).
#
# Output:
#   rust/cambium-libyang-sys/vendor/{libyang,pcre2}
#   go/internal/libyang/vendor/{libyang,pcre2}
#
# Tests/ are stripped; LICENSE/NOTICE files are preserved. Rust bindings are
# regenerated against the flattened libyang headers.
#
# Do NOT run this inside a docs.rs/CI build; it is a pre-publish step.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
THIRD_PARTY="$ROOT/third_party"

echo "=== Release flatten ==="

for src in libyang pcre2; do
  if [ ! -d "$THIRD_PARTY/$src" ]; then
    echo "error: missing $THIRD_PARTY/$src (run git submodule update --init)" >&2
    exit 1
  fi
done

# ---- Rust crate ----------------------------------------------------------------
RUST_VENDOR="$ROOT/rust/cambium-libyang-sys/vendor"
mkdir -p "$RUST_VENDOR"
rm -rf "$RUST_VENDOR/libyang" "$RUST_VENDOR/pcre2"

echo "Copying C source into rust/cambium-libyang-sys/vendor ..."
cp -R "$THIRD_PARTY/libyang" "$RUST_VENDOR/libyang"
cp -R "$THIRD_PARTY/pcre2" "$RUST_VENDOR/pcre2"

# Strip large test directories that are not needed at build/publish time.
rm -rf "$RUST_VENDOR/libyang/tests"
rm -rf "$RUST_VENDOR/pcre2/tests" 2>/dev/null || true

# Regenerate the committed Rust bindings against the installed libyang header.
# The source header references generated ly_config.h, so we build the engine
# once (using the workspace third_party source) and bindgen the result.
GO_BUILD="$ROOT/go/internal/libyang/build.sh"
GO_BUILD_DIR="$ROOT/go/internal/libyang/.build"
INSTALL_INCLUDE="$GO_BUILD_DIR/libyang-install/include/libyang"
BINDINGS="$ROOT/rust/cambium-libyang-sys/src/bindings.rs"

if ! command -v bindgen >/dev/null 2>&1; then
  echo "error: bindgen CLI is required to regenerate bindings" >&2
  exit 1
fi

echo "Building engine to produce installed headers for bindgen ..."
bash "$GO_BUILD"

HEADER="$INSTALL_INCLUDE/libyang.h"
if [ ! -f "$HEADER" ]; then
  echo "error: installed header not found at $HEADER" >&2
  exit 1
fi

echo "Regenerating src/bindings.rs against installed header ..."
bindgen "$HEADER" \
  --output "$BINDINGS" \
  --allowlist-function 'ly.*' \
  --allowlist-type 'ly.*' \
  --allowlist-var 'LY.*' \
  --default-enum-style rust \
  --no-doc-comments \
  --generate functions,types,vars \
  -- -I "$INSTALL_INCLUDE"

# ---- Go module -----------------------------------------------------------------
GO_VENDOR="$ROOT/go/internal/libyang/vendor"
mkdir -p "$GO_VENDOR"
rm -rf "$GO_VENDOR/libyang" "$GO_VENDOR/pcre2"

echo "Copying C source into go/internal/libyang/vendor ..."
cp -R "$THIRD_PARTY/libyang" "$GO_VENDOR/libyang"
cp -R "$THIRD_PARTY/pcre2" "$GO_VENDOR/pcre2"

rm -rf "$GO_VENDOR/libyang/tests"
rm -rf "$GO_VENDOR/pcre2/tests" 2>/dev/null || true

echo "=== Done ==="
echo "Flattened vendored C is in:"
echo "  $RUST_VENDOR"
echo "  $GO_VENDOR"
echo ""
echo "Next: run scripts/check-flattened-build.sh to build the flattened units in isolation."
