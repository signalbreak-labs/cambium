#!/usr/bin/env bash
# Ensure the Rust and Go libyang/PCRE2 CMake configure flags that affect the
# engine (and therefore printer bytes) are identical. Install-path / archive
# variables are intentionally different because each build stage writes to its
# own prefix.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"

# Relevant flags that must match byte-for-byte engine behavior.
KEYS=(
  BUILD_SHARED_LIBS
  CMAKE_POSITION_INDEPENDENT_CODE
  ENABLE_LYD_PRIV
  ENABLE_TESTS
  PCRE2_BUILD_PCRE2_8
  PCRE2_BUILD_PCRE2_16
  PCRE2_BUILD_PCRE2_32
  PCRE2_BUILD_PCRE2GREP
  PCRE2_BUILD_TESTS
)

# Extract -DKEY=VALUE or -DKEY flags from build.sh for the keys above.
go_flags() {
  for key in "${KEYS[@]}"; do
    grep -oE -- "-D${key}(=[^[:space:]]+)?" "$ROOT/go/internal/libyang/build.sh"
  done | sort -u
}

# Extract .define("KEY", "VALUE") from build.rs for the keys above.
rust_flags() {
  for key in "${KEYS[@]}"; do
    # .define("KEY", "VALUE")
    sed -n "s/.*\.define(\"${key}\", *\"\([^\"]*\)\".*/-D${key}=\1/p" \
      "$ROOT/rust/cambium-libyang-sys/build.rs"
  done | sort -u
}

echo "=== Go build.sh engine flags ==="
go_flags
echo ""
echo "=== Rust build.rs engine flags ==="
rust_flags
echo ""

diff -u <(go_flags) <(rust_flags) && echo "engine flags match"
