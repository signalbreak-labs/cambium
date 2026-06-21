#!/usr/bin/env bash
# Two-stage static build of the vendored PCRE2 + libyang for the Go cgo layer.
# Mirrors rust/cambium-libyang-sys/build.rs EXACTLY (same CMake flags) so the Go
# and Rust stacks link a byte-identical engine (see spec/ordering-invariants.md §4).
#
# Output: go/internal/libyang/.build/{pcre2-install,libyang-install} (gitignored).
# cgo references these via ${SRCDIR}/.build/... in build.go.
set -euo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(cd "$HERE/../../.." && pwd)"
BUILD="${CAMBIUM_BUILD_DIR:-$HERE/.build}"
JOBS="$(getconf _NPROCESSORS_ONLN 2>/dev/null || echo 4)"

# Optional cross-compiler override (e.g. CAMBIUM_CC="zig cc -target x86_64-linux-musl").
# If the override contains spaces, write a wrapper script so CMake treats it as a
# single compiler executable and does not inject host-specific flags.
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

# Release-flatten layout: vendor/ takes precedence over workspace third_party.
if [ -d "$HERE/vendor/libyang" ] && [ -d "$HERE/vendor/pcre2" ]; then
  PCRE2_SRC="$HERE/vendor/pcre2"
  LIBYANG_SRC="$HERE/vendor/libyang"
elif [ -d "$ROOT/third_party/pcre2" ] && [ -d "$ROOT/third_party/libyang" ]; then
  PCRE2_SRC="$ROOT/third_party/pcre2"
  LIBYANG_SRC="$ROOT/third_party/libyang"
else
  echo "error: no vendored libyang/pcre2 source found (looked in vendor/ and $ROOT/third_party/)" >&2
  exit 1
fi
PCRE2_INSTALL="$BUILD/pcre2-install"
LIBYANG_INSTALL="$BUILD/libyang-install"

mkdir -p "$BUILD"

if [ -f "$LIBYANG_INSTALL/lib/libyang.a" ] && [ -f "$PCRE2_INSTALL/lib/libpcre2-8.a" ]; then
  echo "already built: $LIBYANG_INSTALL/lib/libyang.a"
  exit 0
fi

echo "=== Stage 1: PCRE2 (static, -fPIC) ==="
cmake -S "$PCRE2_SRC" -B "$BUILD/pcre2-build" \
  -DBUILD_SHARED_LIBS=OFF \
  -DPCRE2_BUILD_PCRE2_8=ON -DPCRE2_BUILD_PCRE2_16=OFF -DPCRE2_BUILD_PCRE2_32=OFF \
  -DPCRE2_BUILD_PCRE2GREP=OFF -DPCRE2_BUILD_TESTS=OFF \
  -DCMAKE_POSITION_INDEPENDENT_CODE=ON \
  -DCMAKE_INSTALL_LIBDIR=lib \
  -DCMAKE_INSTALL_PREFIX="$PCRE2_INSTALL" \
  -DCMAKE_BUILD_TYPE=Release
cmake --build "$BUILD/pcre2-build" --target install -j "$JOBS"

echo "=== Stage 2: libyang (static) against staged PCRE2 ==="
# libyang's FindPCRE2 only short-circuits on the PLURAL PCRE2_LIBRARIES/_INCLUDE_DIRS;
# without them it falls through to find_library and links a system pcre2 (or fails
# outright under cross-toolchains like zig/musl, where no host pcre2 exists). Seed
# both spellings so it always uses our pinned static lib.
cmake -S "$LIBYANG_SRC" -B "$BUILD/libyang-build" \
  -DBUILD_SHARED_LIBS=OFF \
  -DCMAKE_POSITION_INDEPENDENT_CODE=ON \
  -DENABLE_LYD_PRIV=OFF \
  -DENABLE_TESTS=OFF \
  -DCMAKE_INSTALL_LIBDIR=lib \
  -DPCRE2_LIBRARIES="$PCRE2_INSTALL/lib/libpcre2-8.a" \
  -DPCRE2_INCLUDE_DIRS="$PCRE2_INSTALL/include" \
  -DPCRE2_LIBRARY="$PCRE2_INSTALL/lib/libpcre2-8.a" \
  -DPCRE2_INCLUDE_DIR="$PCRE2_INSTALL/include" \
  -DCMAKE_INSTALL_PREFIX="$LIBYANG_INSTALL" \
  -DCMAKE_BUILD_TYPE=Release
cmake --build "$BUILD/libyang-build" --target install -j "$JOBS"

echo "=== Done ==="
ls -la "$LIBYANG_INSTALL/lib/libyang.a" "$PCRE2_INSTALL/lib/libpcre2-8.a"
