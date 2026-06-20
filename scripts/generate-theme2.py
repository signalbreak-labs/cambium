#!/usr/bin/env python3
"""Batch-generate theme 2: type-restrictions fixtures."""
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).parent))
from conformance_lib import add_fixture, run_rust_runner, run_go_runner, add_enabled

THEME = "type-restrictions"


def make_module(name: str, body: str, yang_version: str = "1") -> str:
    return f"""module {name} {{
  namespace "urn:{name}";
  prefix {name.replace('-', '')};

  yang-version {yang_version};
  revision 2026-06-14;

{body}
}}
"""


def add_scalar(name: str, module_body: str, input_xml: str, formats=None, yang_version="1"):
    if formats is None:
        formats = ["xml", "json", "json_ietf"]
    add_fixture(
        name=name,
        theme=THEME,
        module=make_module(name, module_body, yang_version),
        module_name=name,
        input=input_xml,
        input_name="input.xml",
        input_format="xml",
        formats=formats,
        oracle=True,
    )
    add_enabled(name)


def main():
    # 1. pattern modifier invert-match (YANG 1.1)
    add_scalar(
        "types-string-pattern-modifier-invert-match",
        """  leaf name { type string { pattern '[a-z]+' { modifier invert-match; } } }""",
        """<name xmlns="urn:types-string-pattern-modifier-invert-match">ABC9</name>
""",
        yang_version="1.1",
    )

    # 2. multiple patterns conjunction
    add_scalar(
        "types-string-multiple-patterns-conjunction",
        """  leaf token { type string { pattern '.*[a-z].*'; pattern '.*[0-9].*'; } }""",
        """<token xmlns="urn:types-string-multiple-patterns-conjunction">abc123</token>
""",
    )

    # 3. length + pattern + POSIX (patterns are implicitly anchored by libyang)
    add_scalar(
        "types-string-length-pattern-anchor-posix",
        """  leaf code { type string { length "1..10"; pattern '[[:alnum:]_]*'; } }""",
        """<code xmlns="urn:types-string-length-pattern-anchor-posix">abc_123</code>
""",
    )

    # 4. range/length reject (valid byte-golden; reject cases in unit tests)
    add_scalar(
        "constraints-range-length-reject",
        """  container top {
    leaf n { type int32 { range "1..100"; } }
    leaf s { type string { length "2..5"; } }
  }""",
        """<top xmlns="urn:constraints-range-length-reject">
  <n>42</n>
  <s>abc</s>
</top>
""",
    )

    # 5. min/max keywords
    add_scalar(
        "types-range-length-min-max-keywords",
        """  container top {
    leaf code { type int32 { range "min..-1 | 1..max"; } }
    leaf label { type string { length "min..max"; } }
  }""",
        """<top xmlns="urn:types-range-length-min-max-keywords">
  <code>-2147483648</code>
  <label>max</label>
</top>
""",
    )

    # 6. decimal64 no exponent (valid byte-golden; reject case in unit tests)
    add_scalar(
        "json-ietf-decimal64-no-exponent",
        """  leaf rate { type decimal64 { fraction-digits 9; } }""",
        """<rate xmlns="urn:json-ietf-decimal64-no-exponent">0.000000001</rate>
""",
        formats=["xml", "json", "json_ietf"],
    )

    print("Theme 2 fixtures generated. Running verification...")
    run_rust_runner()
    run_go_runner()


if __name__ == "__main__":
    main()
