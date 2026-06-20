#!/usr/bin/env bash
# Build the flattened Rust crate and Go module IN ISOLATION to catch the
# "works in repo, breaks on publish" drift. This is the CI gate for the
# release-flatten step.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

echo "=== Checking flattened Rust crate in isolation ==="
RUST_SRC="$ROOT/rust/cambium-libyang-sys"
RUST_TMP="$TMP/rust-cambium-libyang-sys"
mkdir -p "$RUST_TMP"
cp -R "$RUST_SRC/src" "$RUST_TMP/"
cp -R "$RUST_SRC/vendor" "$RUST_TMP/"
cp "$RUST_SRC/build.rs" "$RUST_TMP/"
cp "$RUST_SRC/Cargo.toml" "$RUST_TMP/"
# The copied Cargo.toml uses workspace inheritance; replace with concrete values
# so the crate builds outside the workspace.
sed -i.bak \
  -e 's/version\.workspace = true/version = "0.0.0"/' \
  -e 's/edition\.workspace = true/edition = "2024"/' \
  -e 's/license\.workspace = true/license = "BSD-3-Clause"/' \
  -e 's/repository\.workspace = true/repository = "https:\/\/github.com\/signalbreak-labs\/cambium"/' \
  -e 's/authors\.workspace = true/authors = ["signalbreak-labs"]/' \
  "$RUST_TMP/Cargo.toml"
rm "$RUST_TMP/Cargo.toml.bak"

cd "$RUST_TMP"
cargo build --features bundled 2>&1 | tail -20
cargo test --features bundled 2>&1 | tail -20

echo ""
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
