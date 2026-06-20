#!/usr/bin/env python3
"""Batch-generate theme 4: reference-types fixtures."""
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).parent))
from conformance_lib import add_fixture, add_enabled, add_case, write_goldens, case_exists

THEME = "reference-types"
CONFORMANCE = Path(__file__).resolve().parent.parent / "conformance"
CORPUS = CONFORMANCE / "corpus"
VENDOR = CORPUS / "ietf-interfaces"


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


def add_two_module(name: str, module_a_name: str, module_a: str,
                   module_b_name: str, module_b: str, input_xml: str,
                   formats=None):
    if case_exists(name):
        print(f"Skipping existing {name}")
        return
    if formats is None:
        formats = ["xml", "json", "json_ietf"]
    fixture_dir = CONFORMANCE / "fixtures" / name
    module_dir = fixture_dir / "module"
    module_dir.mkdir(parents=True, exist_ok=True)
    (module_dir / f"{module_a_name}.yang").write_text(module_a)
    (module_dir / f"{module_b_name}.yang").write_text(module_b)
    (fixture_dir / "input.xml").write_text(input_xml)
    add_case(name, f"fixtures/{name}/module", f"fixtures/{name}/input.xml",
             "xml", formats, True)
    print(f"Generating goldens for {name} ({THEME})...")
    write_goldens(name)
    add_enabled(name)


def main():
    # 1. leafref absolute path
    add_scalar(
        "types-leafref-absolute-path",
        """  container top {
    list iface {
      key name;
      leaf name { type string; }
    }
  }
  leaf primary-iface { type leafref { path "/top/iface/name"; } }""",
        """<top xmlns="urn:types-leafref-absolute-path">
  <iface>
    <name>eth0</name>
  </iface>
</top>
<primary-iface xmlns="urn:types-leafref-absolute-path">eth0</primary-iface>
""",
    )

    # 2. leafref relative parent path
    add_scalar(
        "types-leafref-relative-parent-path",
        """  list iface {
    key name;
    leaf name { type string; }
    container config {
      leaf route-id { type leafref { path "../../name"; } }
    }
  }""",
        """<iface xmlns="urn:types-leafref-relative-parent-path">
  <name>eth0</name>
  <config>
    <route-id>eth0</route-id>
  </config>
</iface>
""",
    )

    # 3. leafref current context
    add_scalar(
        "types-leafref-current-context",
        """  list proto {
    key name;
    leaf name { type string; }
    leaf state { type string; }
  }
  leaf proto-name { type leafref { path "/proto[name = current()/../selected]/name"; } }
  leaf selected { type string; }""",
        """<proto xmlns="urn:types-leafref-current-context">
  <name>ospf</name>
  <state>up</state>
</proto>
<selected xmlns="urn:types-leafref-current-context">ospf</selected>
<proto-name xmlns="urn:types-leafref-current-context">ospf</proto-name>
""",
    )

    # 4. leafref to list key
    add_scalar(
        "types-leafref-to-list-key",
        """  list device {
    key hostname;
    leaf hostname { type string; }
    leaf version { type string; }
  }
  leaf primary-device { type leafref { path "/device/hostname"; } }""",
        """<device xmlns="urn:types-leafref-to-list-key">
  <hostname>router1</hostname>
  <version>1.0</version>
</device>
<primary-device xmlns="urn:types-leafref-to-list-key">router1</primary-device>
""",
    )

    # 5. leafref to leaf-list
    add_scalar(
        "types-leafref-to-leaf-list",
        """  leaf-list tag { type string; }
  leaf assigned-tag { type leafref { path "/tag"; } }""",
        """<tag xmlns="urn:types-leafref-to-leaf-list">red</tag>
<tag xmlns="urn:types-leafref-to-leaf-list">blue</tag>
<assigned-tag xmlns="urn:types-leafref-to-leaf-list">red</assigned-tag>
""",
    )

    # 6. leafref to leafref chain
    add_scalar(
        "types-leafref-to-leafref-chain",
        """  list device {
    key name;
    leaf name { type string; }
  }
  leaf primary-device { type leafref { path "/device/name"; } }
  leaf active-alias { type leafref { path "/primary-device"; } }""",
        """<device xmlns="urn:types-leafref-to-leafref-chain">
  <name>core1</name>
</device>
<primary-device xmlns="urn:types-leafref-to-leafref-chain">core1</primary-device>
<active-alias xmlns="urn:types-leafref-to-leafref-chain">core1</active-alias>
""",
    )

    # 7. leafref deref() function (YANG 1.1)
    add_scalar(
        "types-leafref-deref-function",
        """  list user {
    key uid;
    leaf uid { type string; }
    leaf home { type string; }
  }
  leaf logged-in-user { type leafref { path "/user/uid"; } }
  leaf home-dir { type leafref { path "deref(../logged-in-user)/../home"; } }""",
        """<user xmlns="urn:types-leafref-deref-function">
  <uid>1001</uid>
  <home>/home/alice</home>
</user>
<logged-in-user xmlns="urn:types-leafref-deref-function">1001</logged-in-user>
<home-dir xmlns="urn:types-leafref-deref-function">/home/alice</home-dir>
""",
        yang_version="1.1",
    )

    # 8. leafref require-instance false (YANG 1.1)
    add_scalar(
        "types-leafref-require-instance-false",
        """  list item {
    key id;
    leaf id { type string; }
  }
  leaf future-target { type leafref { path "/item/id"; require-instance false; } }""",
        """<future-target xmlns="urn:types-leafref-require-instance-false">missing</future-target>
""",
        yang_version="1.1",
    )

    # 9. leafref cross-module
    add_two_module(
        "types-leafref-cross-module",
        "types-leafref-cross-module-base",
        """module types-leafref-cross-module-base {
  namespace "urn:types-leafref-cross-module-base";
  prefix tlcb;
  revision 2026-06-14;
  list interface {
    key name;
    leaf name { type string; }
  }
}
""",
        "types-leafref-cross-module",
        """module types-leafref-cross-module {
  namespace "urn:types-leafref-cross-module";
  prefix tlcm;
  import types-leafref-cross-module-base { prefix base; }
  revision 2026-06-14;
  leaf bound-if { type leafref { path "/base:interface/base:name"; } }
}
""",
        """<interface xmlns="urn:types-leafref-cross-module-base">
  <name>eth0</name>
</interface>
<bound-if xmlns="urn:types-leafref-cross-module">eth0</bound-if>
""",
    )

    # 10. identityref single base
    add_scalar(
        "types-identityref-single-base",
        """  identity transport;
  identity tcp { base transport; }
  identity udp { base transport; }
  leaf proto { type identityref { base transport; } }""",
        """<proto xmlns="urn:types-identityref-single-base">tcp</proto>
""",
    )

    # 11. identityref multiple bases (YANG 1.1)
    add_scalar(
        "types-identityref-multiple-bases",
        """  identity interface-type;
  identity data-plane;
  identity ethernet { base interface-type; base data-plane; }
  leaf class { type identityref { base interface-type; } }""",
        """<class xmlns="urn:types-identityref-multiple-bases">ethernet</class>
""",
        yang_version="1.1",
    )

    # 12. identityref derived hierarchy
    add_scalar(
        "types-identityref-derived-hierarchy",
        """  identity component;
  identity hardware-component { base component; }
  identity linecard { base hardware-component; }
  identity fpc { base linecard; }
  leaf item { type identityref { base component; } }""",
        """<item xmlns="urn:types-identityref-derived-hierarchy">fpc</item>
""",
    )

    # 13. identityref foreign module prefix
    add_two_module(
        "types-identityref-foreign-module-prefix",
        "types-identityref-foreign-base",
        """module types-identityref-foreign-base {
  namespace "urn:types-identityref-foreign-base";
  prefix tifb;
  revision 2026-06-14;
  identity component-type;
  identity cpu { base component-type; }
}
""",
        "types-identityref-foreign-module-prefix",
        """module types-identityref-foreign-module-prefix {
  namespace "urn:types-identityref-foreign-module-prefix";
  prefix tifmp;
  import types-identityref-foreign-base { prefix foreign; }
  revision 2026-06-14;
  leaf component { type identityref { base foreign:component-type; } }
}
""",
        """<component xmlns="urn:types-identityref-foreign-module-prefix"
             xmlns:foreign="urn:types-identityref-foreign-base">foreign:cpu</component>
""",
    )

    # 14. identityref iana-if-type foreign
    name = "identityref-iana-if-type-foreign"
    corpus_dir = CORPUS / name
    corpus_dir.mkdir(parents=True, exist_ok=True)
    my_module = """module identityref-iana-if-type-foreign {
  namespace "urn:identityref-iana-if-type-foreign";
  prefix iiftf;
  import ietf-interfaces { prefix if; }
  import iana-if-type { prefix ianaift; }
  revision 2026-06-14;
  leaf type { type identityref { base if:interface-type; } }
}
"""
    (corpus_dir / "identityref-iana-if-type-foreign.yang").write_text(my_module)
    for src in ["ietf-interfaces@2014-05-08.yang", "iana-if-type@2014-05-08.yang"]:
        dst = corpus_dir / src.replace("@2014-05-08", "")
        if not dst.exists():
            dst.symlink_to(VENDOR / src)
    fixture_dir = CONFORMANCE / "fixtures" / name
    fixture_dir.mkdir(parents=True, exist_ok=True)
    input_xml = """<type xmlns="urn:identityref-iana-if-type-foreign"
      xmlns:ianaift="urn:ietf:params:xml:ns:yang:iana-if-type">ianaift:ethernetCsmacd</type>
"""
    (fixture_dir / "input.xml").write_text(input_xml)
    if case_exists(name):
        print(f"Skipping existing {name}")
    else:
        add_case(name,
                 "corpus/identityref-iana-if-type-foreign",
                 "fixtures/identityref-iana-if-type-foreign/input.xml",
                 "xml", ["xml", "json", "json_ietf"], True)
        print(f"Generating goldens for {name} (reference-types)...")
        write_goldens(name)
        add_enabled(name)

    # 15. instance-identifier require default
    add_scalar(
        "types-instance-identifier-require-default",
        """  list node {
    key id;
    leaf id { type string; }
  }
  leaf ref { type instance-identifier; }""",
        """<node xmlns="urn:types-instance-identifier-require-default">
  <id>x</id>
</node>
<ref xmlns="urn:types-instance-identifier-require-default"
  xmlns:t="urn:types-instance-identifier-require-default">/t:node[t:id='x']</ref>
""",
    )

    # 16. instance-identifier no require
    add_scalar(
        "types-instance-identifier-no-require",
        """  container arbitrary {
    list path {
      key key;
      leaf key { type string; }
    }
  }
  leaf any-path { type instance-identifier { require-instance false; } }""",
        """<any-path xmlns="urn:types-instance-identifier-no-require"
  xmlns:t="urn:types-instance-identifier-no-require">/t:arbitrary/t:path[t:key='value']</any-path>
""",
    )

    # 17. instance-identifier complex path
    add_scalar(
        "types-instance-identifier-complex-path",
        """  list vlan {
    key "id name";
    leaf id { type uint16; }
    leaf name { type string; }
    list member {
      key port;
      leaf port { type string; }
    }
  }
  leaf selected-member { type instance-identifier; }""",
        """<vlan xmlns="urn:types-instance-identifier-complex-path">
  <id>100</id>
  <name>mgmt</name>
  <member>
    <port>eth0</port>
  </member>
</vlan>
<selected-member xmlns="urn:types-instance-identifier-complex-path"
  xmlns:t="urn:types-instance-identifier-complex-path">/t:vlan[t:id='100'][t:name='mgmt']/t:member[t:port='eth0']</selected-member>
""",
    )

    print("Theme 4 fixtures generated.")


if __name__ == "__main__":
    main()
