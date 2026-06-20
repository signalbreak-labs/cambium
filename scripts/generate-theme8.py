#!/usr/bin/env python3
"""Generate Theme 8: ordering-invariants fixtures (10)."""
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
from conformance_lib import add_fixture, add_enabled, run_rust_runner, run_go_runner

FIXTURES = [
    (
        "list-ordered-by-user-insertion",
        """module list-ordered-by-user-insertion {
  namespace "urn:list-ordered-by-user-insertion";
  prefix loui;
  revision 2026-06-15;

  container top {
    list rule {
      ordered-by user;
      key name;
      leaf name { type string; }
      leaf action { type string; }
    }
  }
}""",
        """<top xmlns="urn:list-ordered-by-user-insertion">
  <rule>
    <name>c</name>
    <action>drop</action>
  </rule>
  <rule>
    <name>a</name>
    <action>accept</action>
  </rule>
  <rule>
    <name>b</name>
    <action>reject</action>
  </rule>
</top>
""",
        ["xml", "json", "json_ietf"],
    ),
    (
        "list-ordered-by-system-canonical",
        """module list-ordered-by-system-canonical {
  namespace "urn:list-ordered-by-system-canonical";
  prefix losc;
  revision 2026-06-15;

  container top {
    list vlan {
      key vlan-id;
      leaf vlan-id { type uint16; }
      leaf name { type string; }
    }
  }
}""",
        """<top xmlns="urn:list-ordered-by-system-canonical">
  <vlan>
    <vlan-id>300</vlan-id>
    <name>lab</name>
  </vlan>
  <vlan>
    <vlan-id>100</vlan-id>
    <name>mgmt</name>
  </vlan>
  <vlan>
    <vlan-id>200</vlan-id>
    <name>prod</name>
  </vlan>
</top>
""",
        ["xml", "json", "json_ietf"],
    ),
    (
        "leaflist-ordered-by-user",
        """module leaflist-ordered-by-user {
  namespace "urn:leaflist-ordered-by-user";
  prefix llobu;
  revision 2026-06-15;

  container top {
    leaf-list actions {
      ordered-by user;
      type string;
    }
  }
}""",
        """<top xmlns="urn:leaflist-ordered-by-user">
  <actions>c</actions>
  <actions>a</actions>
  <actions>b</actions>
</top>
""",
        ["xml", "json", "json_ietf"],
    ),
    (
        "leaflist-ordered-by-system",
        """module leaflist-ordered-by-system {
  namespace "urn:leaflist-ordered-by-system";
  prefix llobs;
  revision 2026-06-15;

  container top {
    leaf-list ports {
      type uint16;
    }
  }
}""",
        """<top xmlns="urn:leaflist-ordered-by-system">
  <ports>30</ports>
  <ports>10</ports>
  <ports>20</ports>
</top>
""",
        ["xml", "json", "json_ietf"],
    ),
    (
        "leaflist-with-defaults",
        """module leaflist-with-defaults {
  yang-version 1.1;
  namespace "urn:leaflist-with-defaults";
  prefix lwd;
  revision 2026-06-15;

  container top {
    leaf-list servers {
      type string;
      default "8.8.8.8";
      default "8.8.4.4";
    }
  }
}""",
        """<top xmlns="urn:leaflist-with-defaults">
  <servers>8.8.8.8</servers>
  <servers>8.8.4.4</servers>
</top>
""",
        ["xml", "json", "json_ietf"],
    ),
    (
        "ordering-nested-user-cascading",
        """module ordering-nested-user-cascading {
  namespace "urn:ordering-nested-user-cascading";
  prefix onuc;
  revision 2026-06-15;

  container top {
    list statement {
      ordered-by user;
      key name;
      leaf name { type string; }
      leaf-list actions {
        ordered-by user;
        type string;
      }
    }
  }
}""",
        """<top xmlns="urn:ordering-nested-user-cascading">
  <statement>
    <name>s2</name>
    <actions>b</actions>
    <actions>a</actions>
  </statement>
  <statement>
    <name>s1</name>
    <actions>b</actions>
    <actions>a</actions>
  </statement>
</top>
""",
        ["xml", "json", "json_ietf"],
    ),
    (
        "ordered-user-config-false-state",
        """module ordered-user-config-false-state {
  namespace "urn:ordered-user-config-false-state";
  prefix oucfs;
  revision 2026-06-15;

  container state {
    config false;
    list event {
      leaf seq { type uint32; }
      leaf msg { type string; }
    }
  }
}""",
        """<state xmlns="urn:ordered-user-config-false-state">
  <event>
    <seq>3</seq>
    <msg>first</msg>
  </event>
  <event>
    <seq>1</seq>
    <msg>second</msg>
  </event>
  <event>
    <seq>2</seq>
    <msg>third</msg>
  </event>
</state>
""",
        ["xml", "json", "json_ietf"],
    ),
    (
        "declaration-order-out-of-alphabetical",
        """module declaration-order-out-of-alphabetical {
  namespace "urn:declaration-order-out-of-alphabetical";
  prefix dooa;
  revision 2026-06-15;

  container system {
    leaf zebra { type string; }
    leaf apple { type string; }
    leaf mango { type string; }
    leaf banana { type string; }
  }
}""",
        """<system xmlns="urn:declaration-order-out-of-alphabetical">
  <apple>1</apple>
  <banana>2</banana>
  <mango>3</mango>
  <zebra>4</zebra>
</system>
""",
        ["xml", "json", "json_ietf"],
    ),
    (
        "wide-heterogeneous-siblings-all-types",
        """module wide-heterogeneous-siblings-all-types {
  namespace "urn:wide-heterogeneous-siblings-all-types";
  prefix whsat;
  revision 2026-06-15;

  container platform {
    container hardware {
      leaf sku { type string; }
    }
    leaf name { type string; }
    container software {
      leaf version { type string; }
    }
    leaf-list dns-servers {
      type string;
    }
    container services {
      leaf enabled { type boolean; }
    }
    choice routing-protocol {
      case static {
        leaf static-route { type string; }
      }
      case ospf {
        leaf ospf-area { type uint32; }
      }
    }
    leaf timezone { type string; }
    container state {
      config false;
      leaf uptime { type uint32; }
    }
    leaf-list features {
      type string;
    }
  }
}""",
        """<platform xmlns="urn:wide-heterogeneous-siblings-all-types">
  <timezone>UTC</timezone>
  <features>vlan</features>
  <name>r1</name>
  <dns-servers>8.8.8.8</dns-servers>
  <state>
    <uptime>1234</uptime>
  </state>
  <hardware>
    <sku>ABC</sku>
  </hardware>
  <services>
    <enabled>true</enabled>
  </services>
  <features>mpls</features>
  <software>
    <version>1.0</version>
  </software>
  <static-route>0.0.0.0/0</static-route>
</platform>
""",
        ["xml", "json", "json_ietf"],
    ),
    (
        "json-object-determinism",
        """module json-object-determinism {
  namespace "urn:json-object-determinism";
  prefix jod;
  revision 2026-06-15;

  container top {
    leaf zebra { type string; }
    leaf alpha { type string; }
    container mid {
      leaf nine { type uint8; }
      leaf one { type uint8; }
    }
    leaf middle { type string; }
  }
}""",
        """<top xmlns="urn:json-object-determinism">
  <alpha>a</alpha>
  <middle>m</middle>
  <mid>
    <one>1</one>
    <nine>9</nine>
  </mid>
  <zebra>z</zebra>
</top>
""",
        ["xml", "json", "json_ietf"],
    ),
]


def main():
    for name, module, inp, fmts in FIXTURES:
        add_fixture(
            name=name,
            theme="ordering-invariants",
            module=module,
            module_name=name,
            input=inp,
            input_name="input.xml",
            input_format="xml",
            formats=fmts,
            oracle=True,
            op_type=None,
            skip_existing=True,
        )
        add_enabled(name)

    run_rust_runner()
    run_go_runner()


if __name__ == "__main__":
    main()
