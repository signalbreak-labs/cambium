#!/usr/bin/env python3
"""Batch-generate theme 3: type-composition fixtures."""
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).parent))
from conformance_lib import add_fixture, add_enabled

THEME = "type-composition"


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


def add_multi_file(name: str, files: dict[str, str], input_xml: str,
                   input_format="xml", formats=None, oracle=True):
    sys.path.insert(0, str(Path(__file__).parent))
    from conformance_lib import case_exists, add_case, write_goldens
    if case_exists(name):
        print(f"Skipping existing {name}")
        return
    if formats is None:
        formats = ["xml", "json", "json_ietf"]
    fixture_dir = Path(__file__).resolve().parent.parent / "conformance" / "fixtures" / name
    module_dir = fixture_dir / "module"
    module_dir.mkdir(parents=True, exist_ok=True)
    for fname, content in files.items():
        (module_dir / fname).write_text(content)
    (fixture_dir / "input.xml").write_text(input_xml)
    add_case(name, f"fixtures/{name}/module", f"fixtures/{name}/input.xml",
             input_format, formats, oracle)
    print(f"Generating goldens for {name} ({THEME})...")
    write_goldens(name)
    add_enabled(name)


def main():
    # 1. typedef simple base
    add_scalar(
        "types-typedef-simple-base",
        """  typedef port-number {
    type uint16 { range "1..65535"; }
  }
  leaf port { type port-number; }""",
        """<port xmlns="urn:types-typedef-simple-base">443</port>
""",
    )

    # 2. typedef chain 2-deep
    add_scalar(
        "types-typedef-chain-2deep",
        """  typedef base-ip {
    type string { pattern '[0-9]+\\.[0-9]+\\.[0-9]+\\.[0-9]+'; }
  }
  typedef gateway-ip { type base-ip; }
  leaf gw { type gateway-ip; }""",
        """<gw xmlns="urn:types-typedef-chain-2deep">192.168.1.1</gw>
""",
    )

    # 3. typedef chain 3-deep
    add_scalar(
        "types-typedef-chain-3deep",
        """  typedef base-id { type uint32; }
  typedef session-id { type base-id; }
  typedef tunnel-session-id { type session-id; }
  leaf tsid { type tunnel-session-id; }""",
        """<tsid xmlns="urn:types-typedef-chain-3deep">12345</tsid>
""",
    )

    # 4. typedef restriction narrowing
    add_scalar(
        "types-typedef-restriction-narrowing",
        """  typedef port-base { type uint16; }
  typedef ssh-port { type port-base { range "1..1024"; } }
  leaf ssh { type ssh-port; }""",
        """<ssh xmlns="urn:types-typedef-restriction-narrowing">22</ssh>
""",
    )

    # 5. typedef default inheritance
    add_scalar(
        "types-typedef-default-inheritance",
        """  typedef pct {
    type uint8 { range "0..100"; }
    default 50;
  }
  container top {
    leaf a { type pct; }
    leaf b { type pct; default 75; }
  }""",
        """<top xmlns="urn:types-typedef-default-inheritance">
  <a>50</a>
  <b>75</b>
</top>
""",
    )

    # 6. typedef union composition
    add_scalar(
        "types-typedef-union-composition",
        """  typedef transport {
    type union {
      type enumeration { enum tcp; enum udp; }
      type uint8;
    }
  }
  typedef endpoint {
    type union {
      type string { pattern '[a-z0-9-\\.]+'; }
      type transport;
    }
  }
  container top {
    leaf server { type endpoint; }
    leaf mode { type transport; }
  }""",
        """<top xmlns="urn:types-typedef-union-composition">
  <server>example.com</server>
  <mode>tcp</mode>
</top>
""",
    )

    # 7. typedef submodule cross-file
    add_multi_file(
        "types-typedef-submodule-cross-file",
        {
            "main.yang": """module main {
  namespace "urn:types-typedef-submodule-cross-file";
  prefix tdsccf;
  include shared-types;
  revision 2026-06-14;
  leaf val { type shared-decimal; }
}
""",
            "shared-types.yang": """submodule shared-types {
  belongs-to main { prefix tdsccf; }
  typedef shared-decimal { type decimal64 { fraction-digits 3; } }
}
""",
        },
        """<val xmlns="urn:types-typedef-submodule-cross-file">3.141</val>
""",
    )

    # 8. union heterogeneous members quoting
    add_scalar(
        "types-union-heterogeneous-members-quoting",
        """  container top {
    leaf text { type union { type string; } }
    leaf flag { type union { type boolean; } }
    leaf big { type union { type int64; } }
    leaf huge { type union { type uint64; } }
    leaf rate { type union { type decimal64 { fraction-digits 2; } } }
  }""",
        """<top xmlns="urn:types-union-heterogeneous-members-quoting">
  <text>text</text>
  <flag>true</flag>
  <big>9223372036854775807</big>
  <huge>18446744073709551615</huge>
  <rate>3.14</rate>
</top>
""",
    )

    # 9. union scalar all members
    add_scalar(
        "types-union-scalar-all-members",
        """  container top {
    leaf s { type union { type string; } }
    leaf b { type union { type boolean; } }
    leaf i8 { type union { type int8; } }
    leaf u16 { type union { type uint16; } }
    leaf i64 { type union { type int64; } }
    leaf u64 { type union { type uint64; } }
    leaf d { type union { type decimal64 { fraction-digits 2; } } }
    leaf e { type union { type enumeration { enum alpha; enum beta; } } }
    leaf bits { type union { type bits { bit flag1 { position 0; } bit flag2 { position 1; } } } }
    leaf bin { type union { type binary { length "4"; } } }
  }""",
        """<top xmlns="urn:types-union-scalar-all-members">
  <s>str</s>
  <b>false</b>
  <i8>-128</i8>
  <u16>65000</u16>
  <i64>-9223372036854775808</i64>
  <u64>18446744073709551615</u64>
  <d>-1.50</d>
  <e>beta</e>
  <bits>flag1 flag2</bits>
  <bin>SGVsbA==</bin>
</top>
""",
    )

    # 10. union member resolution order
    add_scalar(
        "types-union-member-resolution-order",
        """  leaf code {
    type union {
      type enumeration { enum five { value 5; } }
      type uint32;
    }
  }""",
        """<code xmlns="urn:types-union-member-resolution-order">5</code>
""",
    )

    # 11. union nested typedef chain
    add_scalar(
        "types-union-nested-typedef-chain",
        """  typedef common-string {
    type string { pattern '[0-9]+:[0-9]+'; }
  }
  typedef community-recv {
    type union {
      type common-string;
      type binary { length "8"; }
    }
  }
  container top {
    leaf ext-comm { type community-recv; }
    leaf ext-comm-bin { type community-recv; }
  }""",
        """<top xmlns="urn:types-union-nested-typedef-chain">
  <ext-comm>65000:100</ext-comm>
  <ext-comm-bin>AQIDBAUGBwg=</ext-comm-bin>
</top>
""",
    )

    # 12. union leafref member
    add_scalar(
        "types-union-leafref-member",
        """  list interface {
    key name;
    leaf name { type string; }
  }
  leaf primary-or-static {
    type union {
      type leafref { path "/interface/name"; }
      type string { pattern '192\\.168\\.[0-9]+\\.[0-9]+'; }
    }
  }""",
        """<interface xmlns="urn:types-union-leafref-member">
  <name>eth0</name>
</interface>
<primary-or-static xmlns="urn:types-union-leafref-member">eth0</primary-or-static>
""",
    )

    # 13. union identityref member
    add_scalar(
        "types-union-identityref-member",
        """  identity protocol-base;
  identity tcp { base protocol-base; }
  identity udp { base protocol-base; }
  leaf transport {
    type union {
      type identityref { base protocol-base; }
      type uint16 { range "1..65535"; }
    }
  }""",
        """<transport xmlns="urn:types-union-identityref-member">tcp</transport>
""",
    )

    # 14. union two identityrefs distinct bases
    add_scalar(
        "types-union-two-identityrefs-distinct-bases",
        """  identity hw-component;
  identity linecard { base hw-component; }
  identity sw-component;
  identity driver { base sw-component; }
  leaf component-type {
    type union {
      type identityref { base hw-component; }
      type identityref { base sw-component; }
    }
  }""",
        """<component-type xmlns="urn:types-union-two-identityrefs-distinct-bases">linecard</component-type>
""",
    )

    # 15. union enum and scalar
    add_scalar(
        "types-union-enum-and-scalar",
        """  leaf mode {
    type union {
      type enumeration { enum auto; enum manual; }
      type uint8 { range "0..255"; }
    }
  }""",
        """<mode xmlns="urn:types-union-enum-and-scalar">auto</mode>
""",
    )

    print("Theme 3 fixtures generated.")


if __name__ == "__main__":
    main()
