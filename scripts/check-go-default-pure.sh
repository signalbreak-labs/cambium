#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT/go"

# Public cgo-free surface — subject to the forbidden-dependency closure check.
pkgs=(./cambium ./codegen ./compat ./datatree)

# Packages whose cgo-free (!cgo) tests must also run under CGO_ENABLED=0 — these
# include the nocgo architecture-fitness tests (e.g. conformance/nocgo_test.go,
# pure_go_boundary_test.go) that the CGO_ENABLED=1 lane silently excludes.
nocgoTestPkgs=(./cambium ./codegen ./compat ./datatree ./conformance ./internal/...)

CGO_ENABLED=0 go vet "${pkgs[@]}"
CGO_ENABLED=0 go test "${nocgoTestPkgs[@]}"

deps="$(CGO_ENABLED=0 go list -deps "${pkgs[@]}")"
bad_deps="$(
  printf '%s\n' "$deps" |
    grep -E '(^runtime/cgo$|libyang|internal/libyang|libyangbackend|cambium-libyang|go-libyang|^github\.com/openconfig/goyang(/|$)|github\.com/signalbreak-labs/cambium/go/internal/yangparse/upstream)' || true
)"
if [ -n "$bad_deps" ]; then
  printf 'default Go dependency closure contains forbidden packages:\n%s\n' "$bad_deps" >&2
  exit 1
fi

cgo_files="$(
  CGO_ENABLED=0 go list -deps -f '{{if .CgoFiles}}{{.ImportPath}} {{.CgoFiles}}{{end}}' "${pkgs[@]}"
)"
if [ -n "$cgo_files" ]; then
  printf 'default Go dependency closure contains packages with cgo files:\n%s\n' "$cgo_files" >&2
  exit 1
fi
