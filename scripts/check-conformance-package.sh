#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

"$ROOT/scripts/package-conformance.py" --version test-version --out-dir "$tmp"

archive="$tmp/cambium-conformance-test-version.tar.gz"
checksum="$archive.sha256"
test -s "$archive"
test -s "$checksum"

tar -tzf "$archive" > "$tmp/contents.txt"
grep -Fx "cambium-conformance-test-version/VERSION" "$tmp/contents.txt" >/dev/null
grep -Fx "cambium-conformance-test-version/VERSIONS" "$tmp/contents.txt" >/dev/null
grep -Fx "cambium-conformance-test-version/conformance/manifest.toml" "$tmp/contents.txt" >/dev/null

python3 - "$archive" "$checksum" <<'PY'
import hashlib
import pathlib
import sys

archive = pathlib.Path(sys.argv[1])
checksum = pathlib.Path(sys.argv[2])
line = checksum.read_text(encoding="utf-8").strip()
want, name = line.split(maxsplit=1)
if name != archive.name:
    raise SystemExit(f"checksum filename {name!r} != {archive.name!r}")
got = hashlib.sha256(archive.read_bytes()).hexdigest()
if got != want:
    raise SystemExit(f"checksum {got} != {want}")
PY

echo "conformance package check passed"
