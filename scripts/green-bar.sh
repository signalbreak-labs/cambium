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

run cargo fmt --all -- --check
run cargo clippy --workspace --all-targets -- -D warnings
run cargo test --workspace
run cargo run -p conformance-runner

run "$ROOT/scripts/check-go-default-pure.sh"
run_in "$ROOT/go" env CGO_ENABLED=0 go vet ./cambium ./codegen ./compat

run bash "$ROOT/go/internal/libyang/build.sh"
run_in "$ROOT/go" env CGO_ENABLED=1 go vet ./...
run_in "$ROOT/go" env CGO_ENABLED=1 go test -race ./...
run_in "$ROOT/go" golangci-lint run
run_in "$ROOT/go" env CGO_ENABLED=1 go run ./cmd/cambium all

run "$ROOT/scripts/diff-engine-config.sh"

echo ""
echo "green bar passed"
