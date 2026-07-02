#!/usr/bin/env bash
# Build a standalone yanglint binary from the pinned vendored libyang sources.
#
# This intentionally uses a separate CMake build/install directory from the Go
# cgo engine. The production engine flags remain governed by /VERSIONS and
# scripts/diff-engine-config.sh; this script enables libyang tools only for the
# independent conformance oracle.
set -euo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(cd "$HERE/.." && pwd)"
LIBYANG_BINDINGS="$ROOT/go/internal/libyang"
BUILD="${CAMBIUM_BUILD_DIR:-$LIBYANG_BINDINGS/.build}"
JOBS="$(getconf _NPROCESSORS_ONLN 2>/dev/null || echo 4)"

if [ -n "${CAMBIUM_CC:-}" ]; then
  if [[ "$CAMBIUM_CC" == *" "* ]]; then
    CC_WRAPPER="$BUILD/cambium-cc"
    mkdir -p "$BUILD"
    printf '#!/bin/sh\nexec %s "$@"\n' "$CAMBIUM_CC" > "$CC_WRAPPER"
    chmod +x "$CC_WRAPPER"
    export CC="$CC_WRAPPER"
  else
    export CC="$CAMBIUM_CC"
  fi
fi

if [ -d "$LIBYANG_BINDINGS/vendor/libyang" ] && [ -d "$LIBYANG_BINDINGS/vendor/pcre2" ]; then
  LIBYANG_SRC="$LIBYANG_BINDINGS/vendor/libyang"
elif [ -d "$ROOT/third_party/libyang" ] && [ -d "$ROOT/third_party/pcre2" ]; then
  LIBYANG_SRC="$ROOT/third_party/libyang"
else
  echo "error: no vendored libyang/pcre2 source found (looked in vendor/ and $ROOT/third_party/)" >&2
  exit 1
fi

PCRE2_INSTALL="$BUILD/pcre2-install"
ORACLE_INSTALL="$BUILD/yanglint-oracle-install"
ORACLE_BUILD="$BUILD/yanglint-oracle-build"

if [ -x "$ORACLE_INSTALL/bin/yanglint" ]; then
  echo "already built: $ORACLE_INSTALL/bin/yanglint"
  exit 0
fi

if [ ! -f "$PCRE2_INSTALL/lib/libpcre2-8.a" ]; then
  bash "$LIBYANG_BINDINGS/build.sh"
fi

echo "=== Building yanglint oracle (tools enabled, separate CMake build) ==="
cmake -S "$LIBYANG_SRC" -B "$ORACLE_BUILD" \
  -DBUILD_SHARED_LIBS=OFF \
  -DCMAKE_POSITION_INDEPENDENT_CODE=ON \
  -DENABLE_LYD_PRIV=OFF \
  -DENABLE_TESTS=OFF \
  -DENABLE_TOOLS=ON \
  -DENABLE_YANGLINT_INTERACTIVE=OFF \
  -DCMAKE_INSTALL_LIBDIR=lib \
  -DPCRE2_LIBRARIES="$PCRE2_INSTALL/lib/libpcre2-8.a" \
  -DPCRE2_INCLUDE_DIRS="$PCRE2_INSTALL/include" \
  -DPCRE2_LIBRARY="$PCRE2_INSTALL/lib/libpcre2-8.a" \
  -DPCRE2_INCLUDE_DIR="$PCRE2_INSTALL/include" \
  -DCMAKE_INSTALL_PREFIX="$ORACLE_INSTALL" \
  -DCMAKE_BUILD_TYPE=Release
cmake --build "$ORACLE_BUILD" --target install -j "$JOBS"

echo "=== Done ==="
ls -la "$ORACLE_INSTALL/bin/yanglint"
