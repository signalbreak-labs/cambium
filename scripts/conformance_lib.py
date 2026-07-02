"""Shared helpers for conformance fixture batch generators."""
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
GO_MAIN = ROOT / "go" / "cmd" / "cambium" / "main.go"


def normalize(data: bytes) -> bytes:
    while data and data[-1:].isspace():
        data = data[:-1]
    return data


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

    Probes the SAME binary golden generation invokes (the repo-built YANGLINT),
    not a PATH lookup: goldens are always written by that binary, and it is the
    one that can silently go stale when the /VERSIONS pin is bumped without a
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


def is_module(path: Path) -> bool:
    # Skip submodule files; yanglint cannot load them as top-level schemas.
    try:
        text = path.read_text(encoding="utf-8", errors="ignore")
    except OSError:
        return False
    stripped = text.lstrip()
    return not stripped.startswith("submodule ")


def _module_name(path: Path) -> str:
    text = path.read_text(encoding="utf-8", errors="ignore")
    m = re.search(r"^\s*(?:submodule|module)\s+([A-Za-z0-9_-]+)", text)
    if not m:
        raise RuntimeError(f"cannot find module name in {path}")
    return m.group(1)


def run_yanglint(module_dir: Path, input_path: Path, fmt: str,
                 wd_mode: Optional[str] = None,
                 op_type: Optional[str] = None) -> bytes:
    schemas = sorted(p for p in module_dir.iterdir() if p.suffix == ".yang" and is_module(p))
    if not schemas:
        raise RuntimeError(f"no module .yang files in {module_dir}")
    format_arg = {"xml": "xml", "json": "json", "json_ietf": "json"}[fmt]
    # Match Cambium's default (all features disabled) by passing an empty
    # feature list for every implemented module.
    feature_args = []
    for s in schemas:
        feature_args.extend(["-F", f"{_module_name(s)}:"])
    cmd = [str(YANGLINT), "-X", "-p", str(module_dir)]
    if wd_mode and wd_mode != "explicit":
        cmd.extend(["-d", wd_mode])
    if op_type:
        yanglint_type = {"rpc": "rpc", "reply": "reply", "notification": "notif"}[op_type]
        cmd.extend(["-t", yanglint_type])
    cmd.extend(["-f", format_arg] + feature_args + [str(s) for s in schemas] + [str(input_path)])
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


def case_exists(name: str) -> bool:
    return f'name = "{name}"' in load_manifest()


def add_case(name: str, module_rel: str, input_rel: str, input_format: str,
             formats: List[str], oracle: bool, op_type: Optional[str] = None,
             serialize_defaults: Optional[str] = None) -> None:
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
    if serialize_defaults:
        lines.append(f'serialize-defaults = "{serialize_defaults}"')
    lines.append(f'oracle = {"true" if oracle else "false"}')
    lines.append("[case.expected]")
    for fmt in formats:
        ext = {"xml": "xml", "json": "json", "json_ietf": "json_ietf"}[fmt]
        lines.append(f'{fmt} = "golden/{name}/output.{ext}"')
    text = load_manifest().rstrip() + "\n" + "\n".join(lines) + "\n"
    save_manifest(text)


def write_goldens(name: str, wd_mode: Optional[str] = None,
                  op_type: Optional[str] = None) -> None:
    text = load_manifest()
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
    expected = {fmt: rel for fmt, rel in case.get("expected", {}).items()
                if fmt in {"xml", "json", "json_ietf"}}
    if "input" not in case or not expected:
        # Compile-only tiers (schema-ir) have no data input; helper-only cases
        # (gnmi-json-ietf) have no yanglint-comparable format. Neither golden
        # kind is produced by the oracle, so there is nothing to regenerate.
        tier = case.get("tier", "backend-data")
        print(f"  skipping {name}: no yanglint-regenerable goldens (tier={tier})")
        return
    if wd_mode is None:
        wd_mode = case.get("serialize-defaults")
    if op_type is None:
        op_type = case.get("op-type")
    module_dir = CONFORMANCE / case["module"]
    input_path = CONFORMANCE / case["input"]
    golden_dir = CONFORMANCE / "golden" / name
    golden_dir.mkdir(parents=True, exist_ok=True)
    for fmt, golden_rel in expected.items():
        golden_path = CONFORMANCE / golden_rel
        golden_bytes = run_yanglint(module_dir, input_path, fmt, wd_mode, op_type)
        golden_path.write_bytes(golden_bytes + b"\n")
        print(f"  wrote {golden_path}")


def add_fixture(name: str, theme: str, module: str, module_name: str, input: str,
                input_name: str, input_format: str, formats: List[str],
                oracle: bool, op_type: Optional[str] = None,
                serialize_defaults: Optional[str] = None, skip_existing: bool = True):
    if skip_existing and case_exists(name):
        print(f"Skipping existing {name}")
        return
    fixture_dir = CONFORMANCE / "fixtures" / name
    module_dir = fixture_dir / "module"
    module_dir.mkdir(parents=True, exist_ok=True)
    (module_dir / f"{module_name}.yang").write_text(module)
    input_path = fixture_dir / input_name
    input_path.write_text(input)
    module_rel = f"fixtures/{name}/module"
    input_rel = f"fixtures/{name}/{input_name}"
    add_case(name, module_rel, input_rel, input_format, formats, oracle, op_type,
             serialize_defaults)
    print(f"Generating goldens for {name} ({theme})...")
    write_goldens(name, serialize_defaults, op_type)


def add_enabled(name: str):
    text = GO_MAIN.read_text()
    if f'"{name}"' in text:
        return
    # Insert before closing brace of enabled slice
    idx = text.find("\n}\n")
    if idx < 0:
        raise RuntimeError("could not find enabled slice end")
    # Find the line before the closing brace
    insert_pos = text.rfind('"', 0, idx) + 1
    new_text = text[:insert_pos] + f',\n\t"{name}"' + text[insert_pos:]
    GO_MAIN.write_text(new_text)
    print(f"  added {name} to Go enabled")


def run_rust_runner():
    print("--- Rust runner ---")
    subprocess.run(["cargo", "run", "-p", "conformance-runner"], cwd=ROOT, check=True)


def run_go_runner():
    print("\n--- Go runner ---")
    subprocess.run(["go", "run", "./cmd/cambium", "all"], cwd=ROOT / "go", check=True)
