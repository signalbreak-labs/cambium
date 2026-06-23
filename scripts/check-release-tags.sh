#!/usr/bin/env bash
# Release-tag hygiene guard.
#
# Cambium's Go module lives in the /go subdirectory (module path
# github.com/signalbreak-labs/cambium/go) and the repo ROOT is NOT a module.
# Go's rule for a subdirectory module is absolute: every version tag MUST be
# prefixed with the module's subdirectory, e.g. `go/v0.2.0`.
#
# A bare `v0.2.0` tag is NOT associated with the subdir module at all. Instead
# the module proxy synthesizes a phantom, go.mod-less module at the repo-root
# path (github.com/signalbreak-labs/cambium) from it -- and that mistake is then
# cached on proxy.golang.org *forever* (proxy entries are immutable). This is
# exactly how the legacy `v0.1.0` tag leaked. See PUBLISHING.md.
#
# This guard enforces what PUBLISHING.md prescribes:
#   - forbid bare semver tags (vX.Y.Z) while the repo root has no go.mod
#   - require version tags to be <module-subdir>/vX.Y.Z for a known module dir
#   - require valid semver (vMAJOR.MINOR.PATCH[-pre][+build])
#
# Usage:
#   scripts/check-release-tags.sh                # audit every existing tag
#   scripts/check-release-tags.sh go/v0.3.0      # pre-flight a tag before cutting it
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

# --- Derive valid module-subdir tag prefixes from go.mod locations. Today this
# is just `go`; a future /<lang>/ Go binding is picked up automatically, and a
# root go.mod (module at the repo root) would make bare tags legal.
root_is_module=0
prefixes=()
while IFS= read -r modfile; do
  [ -n "$modfile" ] || continue
  case "$modfile" in
    */vendor/*|*/testdata/*|third_party/*) continue ;;
  esac
  dir="$(dirname "$modfile")"
  if [ "$dir" = "." ]; then
    root_is_module=1
  else
    prefixes+=("$dir")
  fi
done < <(git ls-files '*go.mod')

if [ "${#prefixes[@]}" -eq 0 ] && [ "$root_is_module" -eq 0 ]; then
  echo "error: no go.mod found in the repository; cannot derive valid tag prefixes" >&2
  exit 1
fi

prefix_ok() {
  local p="$1" known
  for known in "${prefixes[@]}"; do
    [ "$p" = "$known" ] && return 0
  done
  return 1
}

# Stored in a variable so the regex is treated literally by [[ =~ ]].
SEMVER='^v[0-9]+\.[0-9]+\.[0-9]+([-+][0-9A-Za-z.-]+)?$'

errors=0
check_tag() {
  local tag="$1"
  local base="${tag##*/}"            # last path segment

  # Only version-looking tags are subject to the rule; arbitrary tags pass.
  [[ "$base" =~ ^v[0-9] ]] || return 0

  local prefix=""
  [ "$tag" != "$base" ] && prefix="${tag%/*}"

  if ! [[ "$base" =~ $SEMVER ]]; then
    printf 'tag %-22s INVALID: %s is not valid semver (want vMAJOR.MINOR.PATCH)\n' "$tag" "$base" >&2
    errors=$((errors + 1)); return 0
  fi

  if [ -z "$prefix" ]; then
    if [ "$root_is_module" -eq 1 ]; then
      printf 'tag %-22s ok\n' "$tag"; return 0
    fi
    local suggest="" p
    for p in "${prefixes[@]}"; do suggest+=" ${p}/${base}"; done
    printf 'tag %-22s FORBIDDEN: bare semver tag, but the repo root has no go.mod.\n' "$tag" >&2
    printf '    A bare tag leaks a phantom go.mod-less module at the repo-root path and\n' >&2
    printf '    is cached on proxy.golang.org forever. Use one of:%s\n' "$suggest" >&2
    errors=$((errors + 1)); return 0
  fi

  if ! prefix_ok "$prefix"; then
    printf 'tag %-22s INVALID: prefix %q is not a known module subdir (have: %s)\n' \
      "$tag" "$prefix" "${prefixes[*]}" >&2
    errors=$((errors + 1)); return 0
  fi

  printf 'tag %-22s ok\n' "$tag"
}

echo "=== Release-tag hygiene ==="
echo "module subdir prefixes: ${prefixes[*]:-<none>}    root-is-module: $root_is_module"
echo

if [ "$#" -gt 0 ]; then
  for t in "$@"; do check_tag "$t"; done
else
  any=0
  while IFS= read -r t; do
    [ -n "$t" ] || continue
    any=1
    check_tag "$t"
  done < <(git tag)
  [ "$any" -eq 1 ] || echo "(no tags)"
fi

echo
if [ "$errors" -gt 0 ]; then
  echo "release-tag check FAILED ($errors bad tag(s))" >&2
  exit 1
fi
echo "release-tag check passed"
