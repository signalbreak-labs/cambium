#!/usr/bin/env bash
# Verify CI keeps the independent yanglint oracle lane wired.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
CI="$ROOT/.github/workflows/ci.yml"
ORACLE_BUILD="$ROOT/scripts/build-yanglint-oracle.sh"

require() {
  local pattern="$1"
  local file="$2"
  local message="$3"
  if ! grep -qF -- "$pattern" "$file"; then
    echo "missing: $message" >&2
    echo "pattern: $pattern" >&2
    echo "file: $file" >&2
    exit 1
  fi
}

require "Yanglint oracle conformance" "$CI" "CI step name"
require "scripts/build-yanglint-oracle.sh" "$CI" "oracle tool build step"
require "CAMBIUM_YANGLINT:" "$CI" "oracle environment variable"
require "go run ./cmd/cambium all" "$CI" "oracle-backed conformance command"

if [ ! -x "$ORACLE_BUILD" ]; then
  echo "missing executable oracle build script: $ORACLE_BUILD" >&2
  exit 1
fi
require "-DENABLE_TOOLS=ON" "$ORACLE_BUILD" "yanglint tools-enabled CMake flag"
require "yanglint-oracle-install" "$ORACLE_BUILD" "separate oracle install directory"

echo "CI yanglint oracle lane is wired"
