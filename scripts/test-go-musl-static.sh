#!/usr/bin/env bash
# Build linux/amd64 musl static Go test binaries with zig cc and execute them
# on a linux/amd64 host. Run each binary from its package directory so relative
# testdata and conformance fixture paths match normal `go test` execution.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
GO_DIR="$ROOT/go"
OUT_DIR="$ROOT/.build/go-musl-tests"

export GOOS="${GOOS:-linux}"
export GOARCH="${GOARCH:-amd64}"
export CGO_ENABLED="${CGO_ENABLED:-1}"
export CC="${CC:-zig cc -target x86_64-linux-musl}"

host_os="$(go env GOHOSTOS)"
host_arch="$(go env GOHOSTARCH)"
if [ "$host_os" != "$GOOS" ] || [ "$host_arch" != "$GOARCH" ]; then
  echo "error: cannot execute $GOOS/$GOARCH test binaries on $host_os/$host_arch" >&2
  exit 1
fi

if [ ! -f "$GO_DIR/internal/libyang/.build/libyang-install/lib/libyang.a" ] ||
  [ ! -f "$GO_DIR/internal/libyang/.build/pcre2-install/lib/libpcre2-8.a" ]; then
  echo "error: musl static engine is missing; run go/internal/libyang/build.sh with CAMBIUM_CC='$CC' first" >&2
  exit 1
fi

mkdir -p "$OUT_DIR"

cd "$GO_DIR"

# Every test-bearing package, derived from the live module so new packages
# cannot be silently excluded from the musl lane.
pkgs=()
while IFS= read -r pkg; do
  [ -n "$pkg" ] && pkgs+=("$pkg")
done < <(go list -f '{{if or .TestGoFiles .XTestGoFiles}}{{.ImportPath}}{{end}}' ./...)
if [ "${#pkgs[@]}" -eq 0 ]; then
  echo "error: no test-bearing packages found under $GO_DIR" >&2
  exit 1
fi

if [ "${CAMBIUM_MUSL_PROBE_RACE:-1}" != "0" ]; then
  echo "==> probing musl -race support on internal/libyang"
  probe="$OUT_DIR/race-probe.test"
  probe_log="$OUT_DIR/race-probe.log"
  if go test -race -c -ldflags '-extldflags "-static"' -o "$probe" ./internal/libyang >"$probe_log" 2>&1 &&
    (cd "$GO_DIR/internal/libyang" && "$probe" -test.timeout=10m) >>"$probe_log" 2>&1; then
    echo "musl -race probe compiled and executed; this lane still runs without -race because the glibc lane owns race coverage."
    rm -f "$probe"
  else
    echo "musl -race probe failed; running static musl tests without -race."
    sed 's/^/  /' "$probe_log"
  fi
fi

for pkg in "${pkgs[@]}"; do
  import_path="$(go list -f '{{.ImportPath}}' "$pkg")"
  pkg_dir="$(go list -f '{{.Dir}}' "$pkg")"
  binary_name="$(printf '%s' "$import_path" | sed 's#[^A-Za-z0-9._-]#_#g').test"
  binary="$OUT_DIR/$binary_name"

  echo "==> building $pkg as a static musl test binary"
  go test -c -ldflags '-extldflags "-static"' -o "$binary" "$pkg"

  echo "==> running $pkg from ${pkg_dir#$ROOT/}"
  (cd "$pkg_dir" && "$binary" -test.v -test.timeout=10m)
done

echo "static musl Go tests passed"
