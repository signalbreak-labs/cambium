#!/usr/bin/env python3
"""Helper for authoring Cambium conformance fixtures.

Creates fixture files, captures goldens from yanglint, updates manifest.toml,
and verifies against the Go conformance runner.
"""
import argparse
import subprocess
import sys
from typing import Optional

from conformance_lib import (
    CONFORMANCE,
    ROOT,
    YANGLINT,
    add_case,
    assert_yanglint_matches_pin,
    write_goldens,
)


def gen_goldens(name: str) -> None:
    write_goldens(name)


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
             args.formats, args.oracle, args.op_type, args.serialize_defaults)
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
    add.add_argument("--serialize-defaults", help="with-defaults mode passed to yanglint -d")
    add.add_argument("--no-verify", action="store_true")

    gen = sub.add_parser("gen", help="regenerate goldens from yanglint")
    gen.add_argument("names", nargs="+")

    ver = sub.add_parser("verify", help="run the Go conformance runner")
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
