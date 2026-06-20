#!/usr/bin/env python3
"""Generate Theme 10: identifier-codegen fixtures (8)."""
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
from conformance_lib import add_fixture, add_enabled, run_rust_runner, run_go_runner

FIXTURES = [
    (
        "idents-keywords-rust",
        """module idents-keywords-rust {
  namespace "urn:idents-keywords-rust";
  prefix ikrr;
  revision 2026-06-15;

  container top {
    leaf type { type string; }
    leaf match { type string; }
    leaf ref { type string; }
    leaf move { type string; }
    leaf struct { type string; }
    leaf fn { type string; }
    leaf impl { type string; }
    leaf loop { type string; }
    leaf async { type string; }
    leaf await { type string; }
    leaf box { type string; }
    leaf where { type string; }
    leaf self { type string; }
    leaf crate { type string; }
    leaf super { type string; }
  }
}""",
        """<top xmlns="urn:idents-keywords-rust">
  <type>t</type>
  <match>m</match>
  <ref>r</ref>
  <move>mv</move>
  <struct>s</struct>
  <fn>f</fn>
  <impl>i</impl>
  <loop>l</loop>
  <async>a</async>
  <await>aw</await>
  <box>b</box>
  <where>w</where>
  <self>se</self>
  <crate>c</crate>
  <super>su</super>
</top>
""",
    ),
    (
        "idents-keywords-go",
        """module idents-keywords-go {
  namespace "urn:idents-keywords-go";
  prefix ikg;
  revision 2026-06-15;

  container top {
    leaf func { type string; }
    leaf range { type string; }
    leaf chan { type string; }
    leaf map { type string; }
    leaf select { type string; }
    leaf go { type string; }
    leaf defer { type string; }
    container interface {
      leaf enabled { type empty; }
    }
    leaf package { type string; }
  }
}""",
        """<top xmlns="urn:idents-keywords-go">
  <func>f</func>
  <range>r</range>
  <chan>c</chan>
  <map>m</map>
  <select>s</select>
  <go>g</go>
  <defer>d</defer>
  <interface>
    <enabled/>
  </interface>
  <package>p</package>
</top>
""",
    ),
    (
        "idents-collision-hyphen-underscore",
        """module idents-collision-hyphen-underscore {
  namespace "urn:idents-collision-hyphen-underscore";
  prefix ichu;
  revision 2026-06-15;

  container top {
    leaf foo-bar { type string; }
    leaf foo_bar { type string; }
    container baz-qux {
      leaf enabled { type empty; }
    }
    container baz_qux {
      leaf status { type string; }
    }
  }
}""",
        """<top xmlns="urn:idents-collision-hyphen-underscore">
  <foo-bar>a</foo-bar>
  <foo_bar>b</foo_bar>
  <baz-qux>
    <enabled/>
  </baz-qux>
  <baz_qux>
    <status>ok</status>
  </baz_qux>
</top>
""",
    ),
    (
        "idents-enum-value-collision",
        """module idents-enum-value-collision {
  namespace "urn:idents-enum-value-collision";
  prefix ievc;
  revision 2026-06-15;

  typedef enable-mode {
    type enumeration {
      enum disabled { value 0; }
      enum enabled { value 1; }
      enum ENABLED { value 2; }
      enum enable-default { value 3; }
      enum enable_default { value 4; }
    }
  }

  container top {
    leaf mode { type enable-mode; }
  }
}""",
        """<top xmlns="urn:idents-enum-value-collision">
  <mode>enable-default</mode>
</top>
""",
    ),
    (
        "idents-container-leaf-collision",
        """module idents-container-leaf-collision {
  namespace "urn:idents-container-leaf-collision";
  prefix iclc;
  revision 2026-06-15;

  container top {
    container config {
      leaf enabled { type empty; }
    }
    leaf config-backup { type string; }
    container interface-state {
      leaf up { type boolean; }
    }
    leaf interface { type string; }
  }
}""",
        """<top xmlns="urn:idents-container-leaf-collision">
  <config>
    <enabled/>
  </config>
  <config-backup>bak</config-backup>
  <interface-state>
    <up>true</up>
  </interface-state>
  <interface>eth0</interface>
</top>
""",
    ),
    (
        "idents-long-name",
        """module idents-long-name {
  namespace "urn:idents-long-name";
  prefix iln;
  revision 2026-06-15;

  container top {
    leaf get-service-accounting-aggregation-source-destination-prefix-information { type string; }
    leaf very-long-interface-configuration-with-many-policy-and-route-settings-applied { type string; }
  }
}""",
        """<top xmlns="urn:idents-long-name">
  <get-service-accounting-aggregation-source-destination-prefix-information>one</get-service-accounting-aggregation-source-destination-prefix-information>
  <very-long-interface-configuration-with-many-policy-and-route-settings-applied>two</very-long-interface-configuration-with-many-policy-and-route-settings-applied>
</top>
""",
    ),
    (
        "idents-unicode-mixed-case",
        """module idents-unicode-mixed-case {
  namespace "urn:idents-unicode-mixed-case";
  prefix iumc;
  revision 2026-06-15;

  container top {
    leaf systemStatus { type string; }
    leaf ConfigUrl { type string; }
    leaf port-id { type string; }
    leaf PortNum { type string; }
  }
}""",
        """<top xmlns="urn:idents-unicode-mixed-case">
  <systemStatus>s</systemStatus>
  <ConfigUrl>c</ConfigUrl>
  <port-id>p</port-id>
  <PortNum>1</PortNum>
</top>
""",
    ),
]


def main():
    for name, module, inp in FIXTURES:
        add_fixture(
            name=name,
            theme="identifier-codegen",
            module=module,
            module_name=name,
            input=inp,
            input_name="input.xml",
            input_format="xml",
            formats=["xml", "json", "json_ietf"],
            oracle=True,
            op_type=None,
            skip_existing=True,
        )
        add_enabled(name)

    run_rust_runner()
    run_go_runner()


if __name__ == "__main__":
    main()
