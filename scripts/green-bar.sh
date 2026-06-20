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

# Default (cgo-free) Go surface: schema + codegen.
run "$ROOT/scripts/check-go-default-pure.sh"
run_in "$ROOT/go" env CGO_ENABLED=0 go vet ./cambium ./codegen ./compat
run_in "$ROOT/go" env CGO_ENABLED=0 go test ./cambium ./codegen ./compat

# Optional libyang backend (cgo) + conformance.
run bash "$ROOT/go/internal/libyang/build.sh"
run_in "$ROOT/go" env CGO_ENABLED=1 go vet ./...
run_in "$ROOT/go" env CGO_ENABLED=1 go test -race ./...
run_in "$ROOT/go" golangci-lint run
run_in "$ROOT/go" env CGO_ENABLED=1 go run ./cmd/cambium all

# Engine build flags must match the pinned /VERSIONS cmake_flags.
run "$ROOT/scripts/diff-engine-config.sh"

echo ""
echo "green bar passed"
