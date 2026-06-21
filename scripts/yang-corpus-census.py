#!/usr/bin/env python3
"""Census external YANG corpora for owned fixture distillation.

This tool intentionally reads local checkout paths only. It does not clone,
modify, or copy vendor files into /conformance; use the aggregate report to
author small owned fixtures instead.
"""

from __future__ import annotations

import argparse
import json
import re
from collections import Counter
from dataclasses import asdict, dataclass, field
from pathlib import Path
from typing import Iterable


KEYWORDS = {
    "action",
    "anydata",
    "anyxml",
    "augment",
    "belongs-to",
    "case",
    "choice",
    "config",
    "container",
    "default",
    "deviation",
    "deviate",
    "extension",
    "feature",
    "grouping",
    "identity",
    "if-feature",
    "import",
    "include",
    "key",
    "leaf",
    "leaf-list",
    "list",
    "mandatory",
    "max-elements",
    "min-elements",
    "module",
    "must",
    "notification",
    "ordered-by",
    "presence",
    "refine",
    "rpc",
    "submodule",
    "typedef",
    "type",
    "unique",
    "uses",
    "when",
    "yang-version",
}

STATEMENT_RE = re.compile(
    r"^\s*(?P<keyword>[A-Za-z_][A-Za-z0-9_.-]*)(?:\s+(?P<arg>\"[^\"]*\"|'[^']*'|[^\s;{]+))?\s*(?P<term>[;{])",
)
COMMENT_RE = re.compile(r"/\*.*?\*/", re.DOTALL)
LINE_COMMENT_RE = re.compile(r"//.*")


@dataclass
class StatementExample:
    file: str
    line: int
    keyword: str
    argument: str = ""


@dataclass
class CorpusReport:
    roots: list[str]
    files: int = 0
    skipped_files: int = 0
    bytes_read: int = 0
    keyword_counts: dict[str, int] = field(default_factory=dict)
    type_counts: dict[str, int] = field(default_factory=dict)
    examples: dict[str, list[StatementExample]] = field(default_factory=dict)
    submodule_includes: list[StatementExample] = field(default_factory=list)
    module_names: list[str] = field(default_factory=list)
    submodule_names: list[str] = field(default_factory=list)


def iter_yang_files(roots: Iterable[Path]) -> Iterable[Path]:
    for root in roots:
        if root.is_file() and root.suffix == ".yang":
            yield root
            continue
        if not root.is_dir():
            continue
        for path in root.rglob("*.yang"):
            if any(part in {".git", ".hg", ".svn"} for part in path.parts):
                continue
            yield path


def strip_comments(text: str) -> str:
    text = COMMENT_RE.sub("", text)
    return "\n".join(LINE_COMMENT_RE.sub("", line) for line in text.splitlines())


def unquote(arg: str | None) -> str:
    if not arg:
        return ""
    arg = arg.strip()
    if len(arg) >= 2 and arg[0] == arg[-1] and arg[0] in {"'", '"'}:
        return arg[1:-1]
    return arg


def add_example(bucket: dict[str, list[StatementExample]], key: str, example: StatementExample, limit: int) -> None:
    items = bucket.setdefault(key, [])
    if len(items) < limit:
        items.append(example)


def scan_file(path: Path, roots: list[Path], report: CorpusReport, max_examples: int, max_file_bytes: int) -> None:
    try:
        size = path.stat().st_size
    except OSError:
        report.skipped_files += 1
        return
    if size > max_file_bytes:
        report.skipped_files += 1
        return
    try:
        text = path.read_text(encoding="utf-8", errors="replace")
    except OSError:
        report.skipped_files += 1
        return

    report.files += 1
    report.bytes_read += size
    rel = relative_to_any(path, roots)
    stripped = strip_comments(text)
    is_submodule = False

    for lineno, line in enumerate(stripped.splitlines(), 1):
        match = STATEMENT_RE.match(line)
        if not match:
            continue
        keyword = match.group("keyword")
        if keyword not in KEYWORDS:
            continue
        arg = unquote(match.group("arg"))
        report.keyword_counts[keyword] = report.keyword_counts.get(keyword, 0) + 1
        example = StatementExample(file=rel, line=lineno, keyword=keyword, argument=arg)
        if keyword in {
            "augment",
            "deviation",
            "deviate",
            "if-feature",
            "include",
            "key",
            "mandatory",
            "max-elements",
            "min-elements",
            "ordered-by",
            "refine",
            "type",
            "uses",
            "when",
        }:
            add_example(report.examples, keyword, example, max_examples)
        if keyword == "module":
            report.module_names.append(arg)
        elif keyword == "submodule":
            is_submodule = True
            report.submodule_names.append(arg)
        elif keyword == "type":
            report.type_counts[arg] = report.type_counts.get(arg, 0) + 1
        elif keyword == "include" and is_submodule and len(report.submodule_includes) < max_examples:
            report.submodule_includes.append(example)


def relative_to_any(path: Path, roots: list[Path]) -> str:
    for root in roots:
        try:
            return path.relative_to(root).as_posix()
        except ValueError:
            continue
    return path.as_posix()


def build_report(paths: list[Path], max_examples: int, max_file_bytes: int) -> CorpusReport:
    roots = [p.resolve() for p in paths]
    report = CorpusReport(roots=[p.as_posix() for p in roots])
    for path in sorted(iter_yang_files(roots)):
        scan_file(path, roots, report, max_examples, max_file_bytes)
    report.keyword_counts = dict(sorted(report.keyword_counts.items()))
    report.type_counts = dict(Counter(report.type_counts).most_common())
    report.module_names = sorted(set(report.module_names))
    report.submodule_names = sorted(set(report.submodule_names))
    return report


def write_markdown(report: CorpusReport, path: Path) -> None:
    lines = [
        "# YANG Corpus Census",
        "",
        "## Roots",
        "",
    ]
    lines.extend(f"- `{root}`" for root in report.roots)
    lines.extend(
        [
            "",
            "## Summary",
            "",
            f"- Files read: {report.files}",
            f"- Files skipped: {report.skipped_files}",
            f"- Bytes read: {report.bytes_read}",
            f"- Modules: {len(report.module_names)}",
            f"- Submodules: {len(report.submodule_names)}",
            "",
            "## Statement Counts",
            "",
            "| Statement | Count |",
            "|---|---:|",
        ]
    )
    for keyword, count in sorted(report.keyword_counts.items(), key=lambda item: (-item[1], item[0])):
        lines.append(f"| `{keyword}` | {count} |")
    lines.extend(["", "## Type Counts", "", "| Type | Count |", "|---|---:|"])
    for typ, count in list(report.type_counts.items())[:60]:
        lines.append(f"| `{typ}` | {count} |")
    lines.extend(["", "## High-Risk Examples", ""])
    for key in sorted(report.examples):
        lines.extend([f"### `{key}`", ""])
        for ex in report.examples[key]:
            arg = f" `{ex.argument}`" if ex.argument else ""
            lines.append(f"- `{ex.file}:{ex.line}`{arg}")
        lines.append("")
    if report.submodule_includes:
        lines.extend(["### submodule-level includes", ""])
        for ex in report.submodule_includes:
            lines.append(f"- `{ex.file}:{ex.line}` `{ex.argument}`")
        lines.append("")
    path.write_text("\n".join(lines), encoding="utf-8")


def report_to_json(report: CorpusReport) -> dict:
    out = asdict(report)
    out["examples"] = {
        key: [asdict(example) for example in examples]
        for key, examples in report.examples.items()
    }
    out["submodule_includes"] = [asdict(example) for example in report.submodule_includes]
    return out


def main() -> int:
    parser = argparse.ArgumentParser(description="Census local YANG corpus checkouts for owned fixture distillation")
    parser.add_argument("paths", nargs="+", type=Path, help="YANG file or directory paths to scan")
    parser.add_argument("--json-out", type=Path, help="write JSON report")
    parser.add_argument("--markdown-out", type=Path, help="write Markdown report")
    parser.add_argument("--max-examples", type=int, default=8)
    parser.add_argument("--max-file-bytes", type=int, default=2_000_000)
    args = parser.parse_args()

    report = build_report(args.paths, args.max_examples, args.max_file_bytes)
    if args.json_out:
        args.json_out.parent.mkdir(parents=True, exist_ok=True)
        args.json_out.write_text(json.dumps(report_to_json(report), indent=2) + "\n", encoding="utf-8")
    if args.markdown_out:
        args.markdown_out.parent.mkdir(parents=True, exist_ok=True)
        write_markdown(report, args.markdown_out)
    if not args.json_out and not args.markdown_out:
        print(json.dumps(report_to_json(report), indent=2))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
