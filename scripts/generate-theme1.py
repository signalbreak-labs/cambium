#!/usr/bin/env python3
"""Batch-generate theme 1: builtin-scalar-types fixtures."""
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).parent))
from conformance_lib import (
    add_fixture, run_rust_runner, run_go_runner, add_enabled,
)

THEME = "builtin-scalar-types"

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
    # 1. int8 range
    add_scalar(
        "types-int-int8-range",
        """  container top {
    leaf i8-min { type int8; }
    leaf i8-zero { type int8; }
    leaf i8-max { type int8; }
  }""",
        """<top xmlns="urn:types-int-int8-range">
  <i8-min>-128</i8-min>
  <i8-zero>0</i8-zero>
  <i8-max>127</i8-max>
</top>
""",
    )

    # 2. int16 range
    add_scalar(
        "types-int-int16-range",
        """  container top {
    leaf i16-full { type int16; }
    leaf i16-range { type int16 { range "-1000..1000"; } }
  }""",
        """<top xmlns="urn:types-int-int16-range">
  <i16-full>-32768</i16-full>
  <i16-range>-500</i16-range>
</top>
""",
    )

    # 3. int32 multipart range + reject gap (unit test handled elsewhere)
    add_scalar(
        "types-int-int32-range-multipart",
        """  leaf priority { type int32 { range "-100..-1 | 1..100"; } }""",
        """<priority xmlns="urn:types-int-int32-range-multipart">-50</priority>
""",
        formats=["xml", "json", "json_ietf"],
    )

    # 4. int64 quoted
    add_scalar(
        "types-int-int64-range-quoted",
        """  leaf count { type int64; }""",
        """<count xmlns="urn:types-int-int64-range-quoted">-9223372036854775808</count>
""",
    )

    # 5. uint8 range
    add_scalar(
        "types-uint-uint8-range",
        """  container top {
    leaf u8-min { type uint8; }
    leaf u8-mid { type uint8; }
    leaf u8-max { type uint8; }
    leaf u8-dscp { type uint8 { range "0..63"; } }
  }""",
        """<top xmlns="urn:types-uint-uint8-range">
  <u8-min>0</u8-min>
  <u8-mid>128</u8-mid>
  <u8-max>255</u8-max>
  <u8-dscp>63</u8-dscp>
</top>
""",
    )

    # 6. uint16 port + reject 0
    add_scalar(
        "types-uint-uint16-range-port",
        """  leaf port { type uint16 { range "1..65535"; } }""",
        """<port xmlns="urn:types-uint-uint16-range-port">443</port>
""",
    )

    # 7. uint32 multi
    add_scalar(
        "types-uint-uint32-range-multi",
        """  leaf as-number { type uint32 { range "1..4199999999"; } }""",
        """<as-number xmlns="urn:types-uint-uint32-range-multi">65000</as-number>
""",
    )

    # 8. uint64 quoted
    add_scalar(
        "types-uint-uint64-range-quoted",
        """  leaf bytes-sent { type uint64; }""",
        """<bytes-sent xmlns="urn:types-uint-uint64-range-quoted">18446744073709551615</bytes-sent>
""",
    )

    # 9. decimal64 fd1
    add_scalar(
        "types-decimal64-fraction1-range",
        """  leaf temp { type decimal64 { fraction-digits 1; range "-50.0..100.0"; } }""",
        """<temp xmlns="urn:types-decimal64-fraction1-range">-0.5</temp>
""",
    )

    # 10. decimal64 fd2 canonical round
    add_scalar(
        "types-decimal64-fraction2-canonical-round",
        """  leaf rate { type decimal64 { fraction-digits 2; } }""",
        """<rate xmlns="urn:types-decimal64-fraction2-canonical-round">3.14</rate>
""",
    )

    # 11. decimal64 fd3 and fd6
    add_scalar(
        "types-decimal64-fraction3-and-6",
        """  container top {
    leaf milli { type decimal64 { fraction-digits 3; } }
    leaf micro { type decimal64 { fraction-digits 6; } }
  }""",
        """<top xmlns="urn:types-decimal64-fraction3-and-6">
  <milli>1.5</milli>
  <micro>0.000001</micro>
</top>
""",
    )

    # 12. decimal64 fd9 negative
    add_scalar(
        "types-decimal64-fraction9-negative",
        """  leaf delay { type decimal64 { fraction-digits 9; } }""",
        """<delay xmlns="urn:types-decimal64-fraction9-negative">-0.000000001</delay>
""",
    )

    # 13. decimal64 fd18 max magnitude
    add_scalar(
        "types-decimal64-fraction18-max-magnitude",
        """  leaf precise { type decimal64 { fraction-digits 18; range "0..9.223372036854775807"; } }""",
        """<precise xmlns="urn:types-decimal64-fraction18-max-magnitude">9.223372036854775807</precise>
""",
    )

    # 14. boolean default false
    add_scalar(
        "types-boolean-default-false",
        """  container top {
    leaf enabled { type boolean; default false; }
    leaf active { type boolean; default true; }
  }""",
        """<top xmlns="urn:types-boolean-default-false">
  <enabled>false</enabled>
  <active>true</active>
</top>
""",
        formats=["xml", "json", "json_ietf"],
    )

    # 15. empty leaf null json
    add_scalar(
        "types-empty-leaf-null-json",
        """  container top {
    leaf enabled { type empty; }
  }""",
        """<top xmlns="urn:types-empty-leaf-null-json">
  <enabled/>
</top>
""",
    )

    # 16. enumeration explicit sparse values
    add_scalar(
        "types-enumeration-explicit-values-sparse",
        """  leaf ip-version { type enumeration { enum unknown { value 0; } enum ipv4 { value 4; } enum ipv6 { value 6; } } }""",
        """<ip-version xmlns="urn:types-enumeration-explicit-values-sparse">ipv6</ip-version>
""",
    )

    # 17. enumeration zero value disabled
    add_scalar(
        "types-enumeration-zero-value-disabled",
        """  leaf status { type enumeration { enum disabled { value 0; } enum primary { value 1; } enum secondary { value 2; } } }""",
        """<status xmlns="urn:types-enumeration-zero-value-disabled">disabled</status>
""",
    )

    # 18. enum/bits auto position
    add_scalar(
        "types-enum-bits-auto-position",
        """  container top {
    leaf e { type enumeration { enum a { value 5; } enum b; enum c { value 1; } enum d; } }
    leaf f { type bits { bit x { position 3; } bit y; bit z { position 0; } } }
  }""",
        """<top xmlns="urn:types-enum-bits-auto-position">
  <e>b</e>
  <f>x y</f>
</top>
""",
    )

    # 19. bits explicit positions gaps
    add_scalar(
        "types-bits-explicit-positions-gaps",
        """  leaf ops { type bits { bit create { position 0; } bit read { position 1; } bit update { position 2; } bit delete { position 3; } } }""",
        """<ops xmlns="urn:types-bits-explicit-positions-gaps">read delete</ops>
""",
    )

    # 20. binary length base64
    add_scalar(
        "types-binary-length-base64",
        """  leaf ext-comm { type binary { length "8"; } }""",
        """<ext-comm xmlns="urn:types-binary-length-base64">AQIDBAUGBwg=</ext-comm>
""",
    )

    print("Theme 1 fixtures generated. Running verification...")
    run_rust_runner()
    run_go_runner()


if __name__ == "__main__":
    main()
