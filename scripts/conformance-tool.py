#!/usr/bin/env python3
"""Helper for authoring Cambium conformance fixtures.

Creates fixture files, captures goldens from yanglint, updates manifest.toml,
and verifies against the Go conformance runner.
"""
import argparse
import os
import re
import subprocess
import sys
from pathlib import Path
from typing import List, Optional

try:
    import tomllib
except ModuleNotFoundError:  # Python < 3.11
    tomllib = None

ROOT = Path(__file__).resolve().parent.parent
CONFORMANCE = ROOT / "conformance"
YANGLINT = ROOT / "go" / "internal" / "libyang" / ".build" / "libyang-install" / "bin" / "yanglint"


def normalize(bytes_data: bytes) -> bytes:
    while bytes_data and bytes_data[-1:].isspace():
        bytes_data = bytes_data[:-1]
    return bytes_data


def run_yanglint(module_dir: Path, input_path: Path, fmt: str) -> bytes:
    schemas = sorted(p for p in module_dir.iterdir() if p.suffix == ".yang")
    if not schemas:
        raise RuntimeError(f"no .yang files in {module_dir}")
    format_arg = {"xml": "xml", "json": "json", "json_ietf": "json"}[fmt]
    cmd = [str(YANGLINT), "-f", format_arg] + [str(s) for s in schemas] + [str(input_path)]
    result = subprocess.run(cmd, capture_output=True)
    if result.returncode != 0:
        raise RuntimeError(
            f"yanglint failed for {input_path.name} format={fmt}:\n"
            f"{result.stderr.decode('utf-8', errors='replace')}"
        )
    return normalize(result.stdout)


def load_manifest() -> str:
    return (CONFORMANCE / "manifest.toml").read_text()


def save_manifest(text: str) -> None:
    (CONFORMANCE / "manifest.toml").write_text(text)


def pinned_libyang_version(repo_root: Path) -> str:
    versions = repo_root / "VERSIONS"
    if tomllib is not None:
        with open(versions, "rb") as f:
            return tomllib.load(f)["libyang"]["tag"].lstrip("v")

    in_libyang = False
    for line in versions.read_text().splitlines():
        stripped = line.strip()
        if stripped == "[libyang]":
            in_libyang = True
            continue
        if stripped.startswith("[") and stripped.endswith("]"):
            in_libyang = False
            continue
        if in_libyang:
            m = re.match(r'tag\s*=\s*"v?([^"]+)"', stripped)
            if m:
                return m.group(1)
    raise RuntimeError("VERSIONS missing [libyang].tag")


def assert_yanglint_matches_pin(repo_root: Path) -> None:
    """Refuse to generate goldens with an oracle that differs from /VERSIONS.

    Probes the SAME binary gen_goldens invokes (the repo-built YANGLINT), not a
    PATH lookup: goldens are always written by that binary, and it is the one
    that can silently go stale when the /VERSIONS pin is bumped without a
    rebuild of go/internal/libyang/.build.
    """
    pinned = pinned_libyang_version(repo_root)
    try:
        out = subprocess.run(
            [str(YANGLINT), "--version"], capture_output=True, text=True, check=True
        ).stdout
    except (OSError, subprocess.CalledProcessError) as exc:
        sys.exit(f"cannot probe oracle version via {YANGLINT}: {exc}")
    m = re.search(r"(\d+\.\d+\.\d+)", out)
    if not m or m.group(1) != pinned:
        got = m.group(1) if m else out.strip()
        sys.exit(
            f"yanglint version {got!r} at {YANGLINT} does not match VERSIONS pin {pinned}; "
            "rebuild the pinned yanglint (bash go/internal/libyang/build.sh) before regenerating goldens"
        )


def case_exists(name: str) -> bool:
    text = load_manifest()
    return f'name = "{name}"' in text


def add_case(name: str, module_rel: str, input_rel: str, input_format: str,
             formats: List[str], oracle: bool, op_type: Optional[str] = None) -> None:
    if case_exists(name):
        raise RuntimeError(f"case {name} already exists in manifest")
    lines = [
        "",
        "[[case]]",
        f'name = "{name}"',
        f'module = "{module_rel}"',
        f'input = "{input_rel}"',
        f'input-format = "{input_format}"',
    ]
    if op_type:
        lines.append(f'op-type = "{op_type}"')
    lines.append(f'oracle = {"true" if oracle else "false"}')
    lines.append("[case.expected]")
    for fmt in formats:
        ext = {"xml": "xml", "json": "json", "json_ietf": "json_ietf"}[fmt]
        lines.append(f'{fmt} = "golden/{name}/output.{ext}"')
    text = load_manifest().rstrip() + "\n" + "\n".join(lines) + "\n"
    save_manifest(text)


def gen_goldens(name: str) -> None:
    text = load_manifest()
    # Parse minimal info for the case
    cases = {}
    current = None
    in_expected = False
    for line in text.splitlines():
        stripped = line.strip()
        if stripped == "[[case]]":
            current = {}
            in_expected = False
        elif stripped == "[case.expected]":
            in_expected = True
        elif current is not None and "=" in stripped and not stripped.startswith("["):
            k, v = stripped.split("=", 1)
            k = k.strip()
            v = v.strip().strip('"')
            if in_expected:
                current.setdefault("expected", {})[k] = v
            else:
                current[k] = v
        elif stripped == "" and current is not None and "name" in current:
            cases[current["name"]] = current
            current = None
    if current is not None and "name" in current:
        cases[current["name"]] = current

    if name not in cases:
        raise RuntimeError(f"case {name} not found in manifest")
    case = cases[name]
    module_dir = CONFORMANCE / case["module"]
    input_path = CONFORMANCE / case["input"]
    golden_dir = CONFORMANCE / "golden" / name
    golden_dir.mkdir(parents=True, exist_ok=True)
    for fmt, golden_rel in case.get("expected", {}).items():
        golden_path = CONFORMANCE / golden_rel
        if fmt not in {"xml", "json", "json_ietf"}:
            continue
        golden_bytes = run_yanglint(module_dir, input_path, fmt)
        golden_path.write_bytes(golden_bytes + b"\n")
        print(f"  wrote {golden_path}")


def verify(name: Optional[str] = None) -> int:
    print("--- Go runner ---")
    go_cmd = ["go", "run", "./cmd/cambium"]
    if name:
        go_cmd += [name]
    return subprocess.run(go_cmd, cwd=ROOT / "go").returncode


def cmd_add(args):
    assert_yanglint_matches_pin(ROOT)
    fixture_dir = CONFORMANCE / "fixtures" / args.name
    module_dir = fixture_dir / "module"
    module_dir.mkdir(parents=True, exist_ok=True)
    # Write module content
    module_path = module_dir / f"{args.module_name or args.name}.yang"
    module_path.write_text(args.module)
    # Write input
    input_path = fixture_dir / args.input_name
    input_path.write_text(args.input)
    # Add manifest entry
    module_rel = f"fixtures/{args.name}/module"
    input_rel = f"fixtures/{args.name}/{args.input_name}"
    add_case(args.name, module_rel, input_rel, args.input_format,
             args.formats, args.oracle, args.op_type)
    # Generate goldens
    print(f"Generating goldens for {args.name}...")
    gen_goldens(args.name)
    if not args.no_verify:
        return verify(args.name)
    return 0


def cmd_gen(args):
    assert_yanglint_matches_pin(ROOT)
    for name in args.names:
        print(f"Regenerating goldens for {name}...")
        gen_goldens(name)
    return 0


def cmd_verify(args):
    return verify(args.name)


def main():
    parser = argparse.ArgumentParser(description="Cambium conformance fixture helper")
    sub = parser.add_subparsers(dest="command", required=True)

    add = sub.add_parser("add", help="add a new fixture")
    add.add_argument("name")
    add.add_argument("--module", "-m", required=True, help="module YANG text")
    add.add_argument("--module-name", help="module file stem (default: fixture name)")
    add.add_argument("--input", "-i", required=True, help="input XML/JSON text")
    add.add_argument("--input-name", default="input.xml", help="input file name")
    add.add_argument("--input-format", default="xml", help="xml|json|json-ietf")
    add.add_argument("--formats", "-f", nargs="+", default=["xml", "json"],
                     help="golden formats: xml json json_ietf")
    add.add_argument("--oracle", action="store_true", default=True)
    add.add_argument("--no-oracle", dest="oracle", action="store_false")
    add.add_argument("--op-type", help="rpc|notification|reply")
    add.add_argument("--no-verify", action="store_true")

    gen = sub.add_parser("gen", help="regenerate goldens from yanglint")
    gen.add_argument("names", nargs="+")

    ver = sub.add_parser("verify", help="run Rust+Go runners")
    ver.add_argument("name", nargs="?")

    args = parser.parse_args()
    if args.command in {"add", "gen"} and not YANGLINT.exists():
        print(f"yanglint not found at {YANGLINT}", file=sys.stderr)
        return 1
    if args.command == "add":
        return cmd_add(args)
    if args.command == "gen":
        return cmd_gen(args)
    if args.command == "verify":
        return cmd_verify(args)
    return 1


if __name__ == "__main__":
    sys.exit(main())
