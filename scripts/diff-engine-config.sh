#!/usr/bin/env bash
# Ensure the libyang/PCRE2 CMake configure flags that affect engine behavior
# (and therefore printer bytes) match the single source of truth in /VERSIONS.
#
# /VERSIONS is language-neutral: every build stack (today the Go cgo backend;
# a future Rust stack) must honor the same pinned cmake_flags. This script
# asserts the Go engine build (go/internal/libyang/build.sh) contains every
# engine-affecting flag pinned in /VERSIONS.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"

# Flags that affect byte-for-byte engine behavior.
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

# Pinned flags from /VERSIONS (inside quoted TOML cmake_flags arrays).
versions_flags() {
  for key in "${KEYS[@]}"; do
    grep -oE -- "-D${key}(=[^\"[:space:]]+)?" "$ROOT/VERSIONS" || true
  done | sort -u
}

# Engine flags actually passed by the Go build.
go_flags() {
  for key in "${KEYS[@]}"; do
    grep -oE -- "-D${key}(=[^[:space:]]+)?" "$ROOT/go/internal/libyang/build.sh" || true
  done | sort -u
}

echo "=== /VERSIONS pinned engine flags ==="
versions_flags
echo ""
echo "=== Go build.sh engine flags ==="
go_flags
echo ""

missing=0
while IFS= read -r flag; do
  [ -z "$flag" ] && continue
  if ! go_flags | grep -qxF -- "$flag"; then
    echo "MISSING in go/internal/libyang/build.sh: $flag" >&2
    missing=1
  fi
done < <(versions_flags)

if [ "$missing" -ne 0 ]; then
  echo "engine flags drift from /VERSIONS" >&2
  exit 1
fi
echo "engine flags honor /VERSIONS"
