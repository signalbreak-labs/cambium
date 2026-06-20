#!/usr/bin/env bash
# Release-flatten: copy the vendored libyang + PCRE2 C source into the
# publishable Go module so it builds without git submodules (`go get` does not
# clone them).
#
# Output:
#   go/internal/libyang/vendor/{libyang,pcre2}
#
# Tests/ are stripped; LICENSE/NOTICE files are preserved.
#
# Do NOT run this inside a CI build; it is a pre-publish step.
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
echo "  $GO_VENDOR"
echo ""
echo "Next: run scripts/check-flattened-build.sh to build the flattened module in isolation."
