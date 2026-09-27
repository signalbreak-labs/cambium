#!/usr/bin/env bash
# Local release green bar. This mirrors the CI/release proof gates that should
# stay green before tagging or publishing.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"

run() {
  echo ""
  echo "==> $*"
  "$@"
}

run_in() {
  local dir="$1"
  shift
  echo ""
  echo "==> (cd ${dir#$ROOT/} && $*)"
  (cd "$dir" && "$@")
}

# Default (cgo-free) Go surface: the pure gate runs CGO_ENABLED=0 go vet + go
# test over the default packages (plus the cgo-free fitness tests) and checks
# their dependency closure.
run "$ROOT/scripts/check-go-default-pure.sh"

# Optional libyang backend (cgo) + conformance.
run bash "$ROOT/go/internal/libyang/build.sh"
run_in "$ROOT/go" env CGO_ENABLED=1 go vet ./...
run_in "$ROOT/go" env CGO_ENABLED=1 go test -race ./...
run_in "$ROOT/go" golangci-lint run
run_in "$ROOT/go" env CGO_ENABLED=1 go run ./cmd/cambium all
run_in "$ROOT/go" env CGO_ENABLED=1 go run ./cmd/cambium datatree-diff
run "$ROOT/scripts/build-yanglint-oracle.sh"
run_in "$ROOT/go" env CAMBIUM_YANGLINT="$ROOT/go/internal/libyang/.build/yanglint-oracle-install/bin/yanglint" go run ./cmd/cambium all

# Engine build flags must match the pinned /VERSIONS cmake_flags.
run "$ROOT/scripts/diff-engine-config.sh"
run "$ROOT/scripts/check-ci-yanglint-oracle.sh"

# Every version tag must be go/vX.Y.Z, never a bare vX.Y.Z (see PUBLISHING.md).
run "$ROOT/scripts/check-release-tags.sh"

# Shared corpus artifact must be buildable for downstream language bindings.
run "$ROOT/scripts/check-conformance-package.sh"

echo ""
echo "green bar passed"
