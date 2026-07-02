#!/usr/bin/env python3
"""Build a versioned, deterministic Cambium conformance corpus tarball."""

from __future__ import annotations

import argparse
import gzip
import hashlib
import io
import os
import pathlib
import subprocess
import tarfile


ROOT = pathlib.Path(__file__).resolve().parents[1]
DEFAULT_MTIME = int(os.environ.get("SOURCE_DATE_EPOCH", "0"))


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument(
        "--version",
        default=os.environ.get("CAMBIUM_CONFORMANCE_VERSION") or git_version(),
        help="artifact version label; defaults to CAMBIUM_CONFORMANCE_VERSION or git describe",
    )
    parser.add_argument(
        "--out-dir",
        default=str(ROOT / "dist"),
        help="directory for the .tar.gz and .sha256 files",
    )
    args = parser.parse_args()

    version = sanitize_version(args.version)
    out_dir = pathlib.Path(args.out_dir)
    out_dir.mkdir(parents=True, exist_ok=True)

    name = f"cambium-conformance-{version}"
    archive = out_dir / f"{name}.tar.gz"
    write_archive(archive, name, version)

    digest = hashlib.sha256(archive.read_bytes()).hexdigest()
    checksum = archive.with_suffix(archive.suffix + ".sha256")
    checksum.write_text(f"{digest}  {archive.name}\n", encoding="utf-8")
    print(archive)
    print(checksum)
    return 0


def git_version() -> str:
    try:
        out = subprocess.check_output(
            ["git", "describe", "--tags", "--always", "--dirty"],
            cwd=ROOT,
            stderr=subprocess.DEVNULL,
            text=True,
        )
        return out.strip()
    except (OSError, subprocess.CalledProcessError):
        return "unknown"


def sanitize_version(version: str) -> str:
    raw = version.strip()
    if not raw:
        raise SystemExit("version must not be empty")
    # Go subdirectory-module release tags are path-like (for example,
    # go/v0.4.0). Artifact names are filesystem entries, so normalize the tag
    # separator instead of rejecting the project's valid release-tag format.
    cleaned = raw.replace("/", "-")
    bad = {"\\", "\x00"}
    if any(ch in cleaned for ch in bad) or cleaned in {".", ".."}:
        raise SystemExit(f"unsafe version label: {version!r}")
    return cleaned


def write_archive(archive: pathlib.Path, root_name: str, version: str) -> None:
    with archive.open("wb") as raw:
        with gzip.GzipFile(filename="", mode="wb", fileobj=raw, mtime=DEFAULT_MTIME) as gz:
            with tarfile.open(fileobj=gz, mode="w", format=tarfile.PAX_FORMAT) as tf:
                add_bytes(tf, f"{root_name}/VERSION", (version + "\n").encode("utf-8"))
                for path in corpus_files():
                    rel = path.relative_to(ROOT).as_posix()
                    add_file(tf, path, f"{root_name}/{rel}")


def corpus_files() -> list[pathlib.Path]:
    files = [ROOT / "VERSIONS"]
    base = ROOT / "conformance"
    files.extend(path for path in base.rglob("*") if path.is_file() and not ignored(path))
    return sorted(files, key=lambda p: p.relative_to(ROOT).as_posix())


def ignored(path: pathlib.Path) -> bool:
    return any(part in {".DS_Store", "__pycache__"} for part in path.parts)


def add_file(tf: tarfile.TarFile, path: pathlib.Path, arcname: str) -> None:
    data = path.read_bytes()
    mode = 0o755 if os.access(path, os.X_OK) else 0o644
    add_bytes(tf, arcname, data, mode=mode)


def add_bytes(tf: tarfile.TarFile, arcname: str, data: bytes, mode: int = 0o644) -> None:
    info = tarfile.TarInfo(arcname)
    info.size = len(data)
    info.mode = mode
    info.mtime = DEFAULT_MTIME
    info.uid = 0
    info.gid = 0
    info.uname = ""
    info.gname = ""
    tf.addfile(info, io.BytesIO(data))


if __name__ == "__main__":
    raise SystemExit(main())
