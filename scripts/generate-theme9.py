#!/usr/bin/env python3
"""Generate Theme 9: json-ietf-serialization fixtures (15)."""
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
from conformance_lib import (
    add_fixture, add_enabled, add_case, write_goldens, case_exists, run_rust_runner, run_go_runner,
    CONFORMANCE, YANGLINT,
)

FIXTURES = [
    (
        "json-ietf-module-namespace-qualification",
        """module json-ietf-module-namespace-qualification {
  namespace "urn:json-ietf-module-namespace-qualification";
  prefix jimnq;
  revision 2026-06-15;

  container top {
    leaf local-leaf { type string; }
    container nested {
      leaf inner { type string; }
    }
  }
}""",
        """<top xmlns="urn:json-ietf-module-namespace-qualification">
  <local-leaf>one</local-leaf>
  <nested>
    <inner>two</inner>
  </nested>
</top>
""",
        ["xml", "json", "json_ietf"],
        True,
    ),
    (
        "json-ietf-scalar-quoting-int-spans",
        """module json-ietf-scalar-quoting-int-spans {
  namespace "urn:json-ietf-scalar-quoting-int-spans";
  prefix jisqis;
  revision 2026-06-15;

  container top {
    leaf i8 { type int8; }
    leaf i16 { type int16; }
    leaf i32 { type int32; }
    leaf i64 { type int64; }
    leaf u8 { type uint8; }
    leaf u16 { type uint16; }
    leaf u32 { type uint32; }
    leaf u64 { type uint64; }
  }
}""",
        """<top xmlns="urn:json-ietf-scalar-quoting-int-spans">
  <i8>127</i8>
  <i16>32767</i16>
  <i32>2147483647</i32>
  <i64>9223372036854775807</i64>
  <u8>255</u8>
  <u16>65535</u16>
  <u32>4294967295</u32>
  <u64>18446744073709551615</u64>
</top>
""",
        ["xml", "json", "json_ietf"],
        True,
    ),
    (
        "json-ietf-decimal64-canonical-quoting",
        """module json-ietf-decimal64-canonical-quoting {
  namespace "urn:json-ietf-decimal64-canonical-quoting";
  prefix jidccq;
  revision 2026-06-15;

  container top {
    leaf negative-value { type decimal64 { fraction-digits 2; } }
    leaf positive-value { type decimal64 { fraction-digits 2; } }
    leaf zero-value { type decimal64 { fraction-digits 2; } }
  }
}""",
        """<top xmlns="urn:json-ietf-decimal64-canonical-quoting">
  <negative-value>-3.14</negative-value>
  <positive-value>2.5</positive-value>
  <zero-value>0</zero-value>
</top>
""",
        ["xml", "json", "json_ietf"],
        True,
    ),
    (
        "json-ietf-string-escaping-control-unicode",
        """module json-ietf-string-escaping-control-unicode {
  namespace "urn:json-ietf-string-escaping-control-unicode";
  prefix jisecu;
  revision 2026-06-15;

  container top {
    leaf with-control { type string; }
    leaf with-quote { type string; }
    leaf with-backslash { type string; }
    leaf french { type string; }
    leaf emoji { type string; }
    leaf greek { type string; }
  }
}""",
        """<top xmlns="urn:json-ietf-string-escaping-control-unicode">
  <with-control>line1
line2\tend</with-control>
  <with-quote>say "hello"</with-quote>
  <with-backslash>C:\\path</with-backslash>
  <french>café</french>
  <emoji>🚀</emoji>
  <greek>αβγ</greek>
</top>
""",
        ["xml", "json", "json_ietf"],
        True,
    ),
    (
        "json-ietf-leaflist-array-user-system",
        """module json-ietf-leaflist-array-user-system {
  namespace "urn:json-ietf-leaflist-array-user-system";
  prefix jillasu;
  revision 2026-06-15;

  container top {
    leaf-list priorities {
      ordered-by user;
      type uint8;
    }
    leaf-list ports {
      type uint16;
    }
  }
}""",
        """<top xmlns="urn:json-ietf-leaflist-array-user-system">
  <priorities>100</priorities>
  <priorities>50</priorities>
  <priorities>75</priorities>
  <ports>8080</ports>
  <ports>22</ports>
  <ports>443</ports>
  <ports>80</ports>
</top>
""",
        ["xml", "json", "json_ietf"],
        True,
    ),
    (
        "json-ietf-list-array-keys-first",
        """module json-ietf-list-array-keys-first {
  namespace "urn:json-ietf-list-array-keys-first";
  prefix jilakf;
  revision 2026-06-15;

  container top {
    list interface {
      key "name protocol";
      leaf protocol { type string; }
      leaf mtu { type uint16; }
      leaf name { type string; }
    }
  }
}""",
        """<top xmlns="urn:json-ietf-list-array-keys-first">
  <interface>
    <mtu>1500</mtu>
    <name>eth0</name>
    <protocol>ip</protocol>
  </interface>
</top>
""",
        ["xml", "json", "json_ietf"],
        True,
    ),
    (
        "json-ietf-nested-container-object",
        """module json-ietf-nested-container-object {
  namespace "urn:json-ietf-nested-container-object";
  prefix jinco;
  revision 2026-06-15;

  container top {
    container middle {
      container deep {
        leaf value { type string; }
      }
    }
  }
}""",
        """<top xmlns="urn:json-ietf-nested-container-object">
  <middle>
    <deep>
      <value>bottom</value>
    </deep>
  </middle>
</top>
""",
        ["xml", "json", "json_ietf"],
        True,
    ),
    (
        "json-ietf-choice-case-transparency",
        """module json-ietf-choice-case-transparency {
  namespace "urn:json-ietf-choice-case-transparency";
  prefix jicct;
  revision 2026-06-15;

  container top {
    leaf priority { type uint8; }
    choice action-type {
      case deny {
        leaf reason { type string; }
        choice log-target {
          case local {
            leaf local-level { type uint8; }
          }
        }
      }
      case allow {
        leaf log { type boolean; }
      }
    }
  }
}""",
        """<top xmlns="urn:json-ietf-choice-case-transparency">
  <priority>7</priority>
  <reason>policy</reason>
  <local-level>3</local-level>
</top>
""",
        ["xml", "json", "json_ietf"],
        True,
    ),
    (
        "json-ietf-presence-vs-nonpresence",
        """module json-ietf-presence-vs-nonpresence {
  namespace "urn:json-ietf-presence-vs-nonpresence";
  prefix jipvn;
  revision 2026-06-15;

  container top {
    container ssh {
      presence "enable ssh";
      leaf port { type uint16; default 22; }
    }
    container empty-slot {
      leaf name { type string; }
    }
  }
}""",
        """<top xmlns="urn:json-ietf-presence-vs-nonpresence">
  <ssh/>
</top>
""",
        ["json_ietf"],
        False,
    ),
    (
        "json-ietf-instance-identifier-string",
        """module json-ietf-instance-identifier-string {
  namespace "urn:json-ietf-instance-identifier-string";
  prefix jiiis;
  revision 2026-06-15;

  container top {
    list device {
      key name;
      leaf name { type string; }
    }
    leaf active-device { type instance-identifier; }
  }
}""",
        """<top xmlns="urn:json-ietf-instance-identifier-string" xmlns:jiiis="urn:json-ietf-instance-identifier-string">
  <device>
    <name>eth0</name>
  </device>
  <active-device>/jiiis:top/jiiis:device[jiiis:name='eth0']</active-device>
</top>
""",
        ["xml", "json", "json_ietf"],
        True,
    ),
    (
        "json-ietf-leafref-union-resolved-form",
        """module json-ietf-leafref-union-resolved-form {
  namespace "urn:json-ietf-leafref-union-resolved-form";
  prefix jilurf;
  revision 2026-06-15;

  container top {
    list iface {
      key name;
      leaf name { type string; }
    }
    leaf primary-iface {
      type leafref { path "/top/iface/name"; }
    }
    leaf value {
      type union {
        type int64;
        type boolean;
      }
    }
  }
}""",
        """<top xmlns="urn:json-ietf-leafref-union-resolved-form">
  <iface>
    <name>eth0</name>
  </iface>
  <primary-iface>eth0</primary-iface>
  <value>9223372036854775807</value>
</top>
""",
        ["xml", "json", "json_ietf"],
        True,
    ),
]


def main():
    for name, module, inp, fmts, oracle in FIXTURES:
        add_fixture(
            name=name,
            theme="json-ietf-serialization",
            module=module,
            module_name=name,
            input=inp,
            input_name="input.xml",
            input_format="xml",
            formats=fmts,
            oracle=oracle,
            op_type=None,
            skip_existing=True,
        )
        add_enabled(name)

    # anydata/anyxml representation (YANG 1.1 for anydata)
    name = "json-ietf-anydata-anyxml-representation"
    module = """module json-ietf-anydata-anyxml-representation {
  yang-version 1.1;
  namespace "urn:json-ietf-anydata-anyxml-representation";
  prefix jiaar;
  revision 2026-06-15;

  container top {
    container config {
      anydata metadata;
    }
    container payload {
      anyxml data;
    }
  }
}"""
    inp = """<top xmlns="urn:json-ietf-anydata-anyxml-representation">
  <config>
    <metadata>
      <custom>1</custom>
      <custom>2</custom>
      <custom>
        <nested>true</nested>
      </custom>
    </metadata>
  </config>
  <payload>
    <data>
      <foo>
        <bar>test</bar>
      </foo>
    </data>
  </payload>
</top>
"""
    add_fixture(
        name=name,
        theme="json-ietf-serialization",
        module=module,
        module_name=name,
        input=inp,
        input_name="input.xml",
        input_format="xml",
        formats=["xml", "json", "json_ietf"],
        oracle=True,
        skip_existing=True,
    )
    add_enabled(name)

    # parse-roundtrip from JSON input
    name = "json-ietf-parse-roundtrip"
    module = """module json-ietf-parse-roundtrip {
  yang-version 1.1;
  namespace "urn:json-ietf-parse-roundtrip";
  prefix jiprt;
  revision 2026-06-15;

  identity base-id;
  identity derived-id { base base-id; }

  container top {
    leaf big { type int64; }
    leaf flag { type empty; }
    leaf kind { type identityref { base base-id; } }
    leaf-list tags { type string; }
  }
}"""
    inp = """{
  "json-ietf-parse-roundtrip:top": {
    "big": "9223372036854775807",
    "flag": [null],
    "kind": "derived-id",
    "tags": ["a", "b"]
  }
}
"""
    add_fixture(
        name=name,
        theme="json-ietf-serialization",
        module=module,
        module_name=name,
        input=inp,
        input_name="input.json",
        input_format="json",
        formats=["xml", "json", "json_ietf"],
        oracle=True,
        skip_existing=True,
    )
    add_enabled(name)

    # with-defaults modes: one directory, four manifest cases
    wd_name = "json-ietf-with-defaults-modes"
    wd_module = """module json-ietf-with-defaults-modes {
  namespace "urn:json-ietf-with-defaults-modes";
  prefix jidwm;
  revision 2026-06-15;

  container settings {
    leaf timeout { type uint32; default 30; }
    leaf retries { type uint8; default 3; }
    leaf name { type string; }
  }
}"""
    wd_input = """<settings xmlns="urn:json-ietf-with-defaults-modes">
  <timeout>30</timeout>
  <retries>3</retries>
  <name>primary</name>
</settings>
"""
    fixture_dir = CONFORMANCE / "fixtures" / wd_name
    module_dir = fixture_dir / "module"
    module_dir.mkdir(parents=True, exist_ok=True)
    (module_dir / f"{wd_name}.yang").write_text(wd_module)
    (fixture_dir / "input.xml").write_text(wd_input)
    module_rel = f"fixtures/{wd_name}/module"
    input_rel = f"fixtures/{wd_name}/input.xml"

    for mode in ["explicit", "trim", "all", "all-tagged"]:
        case_name = f"{wd_name}-{mode}"
        if case_exists(case_name):
            print(f"Skipping existing {case_name}")
            continue
        add_case(
            name=case_name,
            module_rel=module_rel,
            input_rel=input_rel,
            input_format="xml",
            formats=["json_ietf"],
            oracle=True,
            serialize_defaults=mode,
        )
        print(f"Generating goldens for {case_name} (mode={mode})...")
        write_goldens(case_name, wd_mode=mode if mode != "explicit" else None)
        add_enabled(case_name)

    # cross-module augment/deviation/when + submodule
    cm_name = "json-ietf-cross-module-augment-deviation-when"
    base_module = """module json-ietf-cross-module-base {
  yang-version 1.1;
  namespace "urn:json-ietf-cross-module-base";
  prefix jicmb;

  include json-ietf-cross-module-base-types;

  revision 2026-06-15;

  container interfaces {
    list interface {
      key name;
      leaf name { type string; }
      leaf counter { type uint64; }
    }
  }

  container system {
    leaf mode { type string; }
    leaf debug {
      when "../mode = 'advanced'";
      type string;
    }
    uses secret-group;
  }
}"""
    submodule = """submodule json-ietf-cross-module-base-types {
  yang-version 1.1;
  belongs-to json-ietf-cross-module-base {
    prefix jicmb;
  }
  revision 2026-06-15;

  grouping secret-group {
    leaf secret { type string; }
  }
}"""
    aug_module = """module json-ietf-cross-module-aug {
  yang-version 1.1;
  namespace "urn:json-ietf-cross-module-aug";
  prefix jicma;

  import json-ietf-cross-module-base {
    prefix b;
  }

  revision 2026-06-15;

  augment "/b:interfaces/b:interface" {
    leaf custom-field { type string; }
  }

  deviation "/b:interfaces/b:interface/b:counter" {
    deviate replace {
      type uint32 {
        range "0..1000";
      }
    }
  }
}"""
    cm_input = """<interfaces xmlns="urn:json-ietf-cross-module-base" xmlns:jicma="urn:json-ietf-cross-module-aug">
  <interface>
    <name>eth0</name>
    <counter>500</counter>
    <jicma:custom-field>abc</jicma:custom-field>
  </interface>
</interfaces>
<system xmlns="urn:json-ietf-cross-module-base">
  <mode>basic</mode>
  <secret>shh</secret>
</system>
"""
    fixture_dir = CONFORMANCE / "fixtures" / cm_name
    module_dir = fixture_dir / "module"
    module_dir.mkdir(parents=True, exist_ok=True)
    (module_dir / "json-ietf-cross-module-base.yang").write_text(base_module)
    (module_dir / "json-ietf-cross-module-base-types.yang").write_text(submodule)
    (module_dir / "json-ietf-cross-module-aug.yang").write_text(aug_module)
    (fixture_dir / "input.xml").write_text(cm_input)
    add_case(
        name=cm_name,
        module_rel=f"fixtures/{cm_name}/module",
        input_rel=f"fixtures/{cm_name}/input.xml",
        input_format="xml",
        formats=["xml", "json", "json_ietf"],
        oracle=True,
    )
    print(f"Generating goldens for {cm_name}...")
    write_goldens(cm_name)
    add_enabled(cm_name)

    run_rust_runner()
    run_go_runner()


if __name__ == "__main__":
    main()
