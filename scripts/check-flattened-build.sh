#!/usr/bin/env bash
# Build the flattened Go module IN ISOLATION to catch the "works in repo, breaks
# on publish" drift (`go get` does not clone submodules). This is the CI gate for
# the release-flatten step.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

echo "=== Checking flattened Go module in isolation ==="
GO_SRC="$ROOT/go"
GO_TMP="$TMP/go"
mkdir -p "$GO_TMP"
cp -R "$GO_SRC/"* "$GO_TMP/"
# Lifetime tests read from the shared conformance corpus.
cp -R "$ROOT/conformance" "$GO_TMP/../conformance"
# Ensure vendor/ is the only C source visible.
if [ ! -d "$GO_TMP/internal/libyang/vendor/libyang" ]; then
  echo "error: Go vendor/ not found" >&2
  exit 1
fi

cd "$GO_TMP"
bash internal/libyang/build.sh 2>&1 | tail -10
go vet ./...
go test -race ./internal/libyang

echo ""
echo "=== Flattened builds OK ==="
