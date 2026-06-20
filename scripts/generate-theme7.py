#!/usr/bin/env python3
"""Batch-generate theme 7: data-node-types fixtures."""
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).parent))
from conformance_lib import CONFORMANCE, add_fixture

THEME = "data-node-types"


def write_invalid(name: str, xml: str) -> None:
    (CONFORMANCE / "fixtures" / name / "input-invalid.xml").write_text(xml)


def main():
    # 1. presence container empty
    add_fixture(
        name="container-presence-empty",
        theme=THEME,
        module_name="container-presence-empty",
        module="""module container-presence-empty {
  yang-version 1;
  namespace "urn:container-presence-empty";
  prefix cpe;
  revision 2026-06-14;

  container enable-ssh {
    presence "ssh enabled";
    leaf port { type uint16; default 22; }
  }
  container non-presence {
    leaf status { type string; }
  }
}
""",
        input_name="input.xml",
        input_format="xml",
        formats=["xml", "json", "json_ietf"],
        oracle=True,
        input="""<enable-ssh xmlns="urn:container-presence-empty"/>
""",
    )

    # 2. nested container depth
    add_fixture(
        name="container-nested-depth",
        theme=THEME,
        module_name="container-nested-depth",
        module="""module container-nested-depth {
  yang-version 1;
  namespace "urn:container-nested-depth";
  prefix cnd;
  revision 2026-06-14;

  container level1 {
    leaf a { type string; }
    container level2 {
      leaf b { type string; }
      container level3 {
        leaf c { type string; }
      }
    }
  }
}
""",
        input_name="input.xml",
        input_format="xml",
        formats=["xml", "json", "json_ietf"],
        oracle=True,
        input="""<level1 xmlns="urn:container-nested-depth">
  <a>top</a>
  <level2>
    <b>middle</b>
    <level3>
      <c>bottom</c>
    </level3>
  </level2>
</level1>
""",
    )

    # 3. list single string key
    add_fixture(
        name="list-single-key-string",
        theme=THEME,
        module_name="list-single-key-string",
        module="""module list-single-key-string {
  yang-version 1;
  namespace "urn:list-single-key-string";
  prefix lsks;
  revision 2026-06-14;

  list interface {
    key name;
    leaf name { type string; }
    leaf mtu { type uint16; }
  }
}
""",
        input_name="input.xml",
        input_format="xml",
        formats=["xml", "json", "json_ietf"],
        oracle=True,
        input="""<interface xmlns="urn:list-single-key-string">
  <name>eth0</name>
  <mtu>1500</mtu>
</interface>
<interface xmlns="urn:list-single-key-string">
  <name>eth1</name>
  <mtu>9000</mtu>
</interface>
""",
    )

    # 4. list single numeric key
    add_fixture(
        name="list-single-key-numeric",
        theme=THEME,
        module_name="list-single-key-numeric",
        module="""module list-single-key-numeric {
  yang-version 1;
  namespace "urn:list-single-key-numeric";
  prefix lskn;
  revision 2026-06-14;

  list vlan {
    key vlan-id;
    leaf vlan-id { type uint16; }
    leaf description { type string; }
  }
}
""",
        input_name="input.xml",
        input_format="xml",
        formats=["xml", "json", "json_ietf"],
        oracle=True,
        input="""<vlan xmlns="urn:list-single-key-numeric">
  <vlan-id>100</vlan-id>
  <description>mgmt</description>
</vlan>
<vlan xmlns="urn:list-single-key-numeric">
  <vlan-id>200</vlan-id>
  <description>prod</description>
</vlan>
""",
    )

    # 5. composite key two
    add_fixture(
        name="list-composite-key-two",
        theme=THEME,
        module_name="list-composite-key-two",
        module="""module list-composite-key-two {
  yang-version 1;
  namespace "urn:list-composite-key-two";
  prefix lckt;
  revision 2026-06-14;

  list edge {
    key "src-ip dst-ip";
    leaf metric { type uint32; }
    leaf src-ip { type string; }
    leaf dst-ip { type string; }
  }
}
""",
        input_name="input.xml",
        input_format="xml",
        formats=["xml", "json", "json_ietf"],
        oracle=True,
        input="""<edge xmlns="urn:list-composite-key-two">
  <src-ip>10.0.0.1</src-ip>
  <dst-ip>10.0.0.2</dst-ip>
  <metric>10</metric>
</edge>
""",
    )

    # 6. composite key three
    add_fixture(
        name="list-composite-key-three",
        theme=THEME,
        module_name="list-composite-key-three",
        module="""module list-composite-key-three {
  yang-version 1;
  namespace "urn:list-composite-key-three";
  prefix lck3;
  revision 2026-06-14;

  list route {
    key "prefix nexthop-ip afi";
    leaf preference { type uint8; }
    leaf prefix { type string; }
    leaf nexthop-ip { type string; }
    leaf metric { type uint32; }
    leaf afi { type string; }
  }
}
""",
        input_name="input.xml",
        input_format="xml",
        formats=["xml", "json", "json_ietf"],
        oracle=True,
        input="""<route xmlns="urn:list-composite-key-three">
  <prefix>192.0.2.0/24</prefix>
  <nexthop-ip>198.51.100.1</nexthop-ip>
  <afi>ipv4</afi>
  <preference>5</preference>
  <metric>100</metric>
</route>
""",
    )

    # 7. wide composite key scrambled
    add_fixture(
        name="ordering-composite-key-wide",
        theme=THEME,
        module_name="ordering-composite-key-wide",
        module="""module ordering-composite-key-wide {
  yang-version 1;
  namespace "urn:ordering-composite-key-wide";
  prefix ockw;
  revision 2026-06-14;

  list edges {
    key "src-type src-slot src-pfe dst-type dst-slot dst-pfe";
    leaf weight { type uint32; }
    leaf src-type { type string; }
    leaf src-slot { type uint8; }
    leaf src-pfe { type uint8; }
    leaf dst-type { type string; }
    leaf dst-slot { type uint8; }
    leaf dst-pfe { type uint8; }
  }
}
""",
        input_name="input.xml",
        input_format="xml",
        formats=["xml", "json", "json_ietf"],
        oracle=True,
        input="""<edges xmlns="urn:ordering-composite-key-wide">
  <weight>100</weight>
  <src-type>fpc</src-type>
  <src-slot>1</src-slot>
  <src-pfe>0</src-pfe>
  <dst-type>fpc</dst-type>
  <dst-slot>2</dst-slot>
  <dst-pfe>1</dst-pfe>
</edges>
""",
    )

    # 8. composite key with interleaved containers
    add_fixture(
        name="composite-key-with-interleaved-containers",
        theme=THEME,
        module_name="composite-key-with-interleaved-containers",
        module="""module composite-key-with-interleaved-containers {
  yang-version 1;
  namespace "urn:composite-key-with-interleaved-containers";
  prefix ckwic;
  revision 2026-06-14;

  list route {
    key "dest-prefix next-hop-ip";
    leaf dest-prefix { type string; }
    container metrics {
      leaf distance { type uint8; }
    }
    leaf next-hop-ip { type string; }
  }
}
""",
        input_name="input.xml",
        input_format="xml",
        formats=["xml", "json", "json_ietf"],
        oracle=True,
        input="""<route xmlns="urn:composite-key-with-interleaved-containers">
  <metrics>
    <distance>10</distance>
  </metrics>
  <dest-prefix>10.0.0.0/8</dest-prefix>
  <next-hop-ip>192.0.2.1</next-hop-ip>
</route>
""",
    )

    # 9. keyless list positional
    add_fixture(
        name="list-keyless-positional",
        theme=THEME,
        module_name="list-keyless-positional",
        module="""module list-keyless-positional {
  yang-version 1;
  namespace "urn:list-keyless-positional";
  prefix lkp;
  revision 2026-06-14;

  container state {
    config false;
    list sample {
      max-elements 8;
      leaf reading { type uint64; }
    }
  }
}
""",
        input_name="input.xml",
        input_format="xml",
        formats=["xml", "json", "json_ietf"],
        oracle=True,
        input="""<state xmlns="urn:list-keyless-positional">
  <sample>
    <reading>300</reading>
  </sample>
  <sample>
    <reading>100</reading>
  </sample>
  <sample>
    <reading>200</reading>
  </sample>
</state>
""",
    )

    # 10. min-elements reject (valid input)
    add_fixture(
        name="min-elements-reject",
        theme=THEME,
        module_name="min-elements-reject",
        module="""module min-elements-reject {
  yang-version 1;
  namespace "urn:min-elements-reject";
  prefix mer;
  revision 2026-06-14;

  leaf-list servers {
    type string;
    min-elements 2;
    max-elements 4;
  }
}
""",
        input_name="input.xml",
        input_format="xml",
        formats=["xml", "json", "json_ietf"],
        oracle=True,
        input="""<servers xmlns="urn:min-elements-reject">10.0.0.1</servers>
<servers xmlns="urn:min-elements-reject">10.0.0.2</servers>
""",
    )
    write_invalid(
        "min-elements-reject",
        """<servers xmlns="urn:min-elements-reject">10.0.0.1</servers>
""",
    )

    # 11. max-elements reject (valid input)
    add_fixture(
        name="max-elements-reject",
        theme=THEME,
        module_name="max-elements-reject",
        module="""module max-elements-reject {
  yang-version 1;
  namespace "urn:max-elements-reject";
  prefix mxer;
  revision 2026-06-14;

  leaf-list servers {
    type string;
    min-elements 2;
    max-elements 4;
  }
}
""",
        input_name="input.xml",
        input_format="xml",
        formats=["xml", "json", "json_ietf"],
        oracle=True,
        input="""<servers xmlns="urn:max-elements-reject">a</servers>
<servers xmlns="urn:max-elements-reject">b</servers>
<servers xmlns="urn:max-elements-reject">c</servers>
<servers xmlns="urn:max-elements-reject">d</servers>
""",
    )
    write_invalid(
        "max-elements-reject",
        """<servers xmlns="urn:max-elements-reject">a</servers>
<servers xmlns="urn:max-elements-reject">b</servers>
<servers xmlns="urn:max-elements-reject">c</servers>
<servers xmlns="urn:max-elements-reject">d</servers>
<servers xmlns="urn:max-elements-reject">e</servers>
""",
    )

    # 12. choice multiple cases with default
    add_fixture(
        name="choice-multiple-cases-default",
        theme=THEME,
        module_name="choice-multiple-cases-default",
        module="""module choice-multiple-cases-default {
  yang-version 1;
  namespace "urn:choice-multiple-cases-default";
  prefix cmcd;
  revision 2026-06-14;

  choice auth-type {
    default password;
    case password {
      leaf pass-string { type string; }
    }
    case public-key {
      leaf key-data { type string; }
    }
    case token {
      leaf token-value { type string; }
    }
  }
}
""",
        input_name="input.xml",
        input_format="xml",
        formats=["xml", "json", "json_ietf"],
        oracle=True,
        input="""<token-value xmlns="urn:choice-multiple-cases-default">tok-123</token-value>
""",
    )

    # 13. choice mandatory reject
    add_fixture(
        name="choice-mandatory-reject",
        theme=THEME,
        module_name="choice-mandatory-reject",
        module="""module choice-mandatory-reject {
  yang-version 1;
  namespace "urn:choice-mandatory-reject";
  prefix cmr;
  revision 2026-06-14;

  choice tunnel-mode {
    mandatory true;
    case gre {
      leaf remote { type string; }
    }
    case mpls {
      leaf label { type uint32; }
    }
  }
}
""",
        input_name="input.xml",
        input_format="xml",
        formats=["xml", "json", "json_ietf"],
        oracle=True,
        input="""<remote xmlns="urn:choice-mandatory-reject">203.0.113.1</remote>
""",
    )
    write_invalid(
        "choice-mandatory-reject",
        """<config xmlns="urn:choice-mandatory-reject"/>
""",
    )

    # 14. choice single-node case
    add_fixture(
        name="choice-single-node-case",
        theme=THEME,
        module_name="choice-single-node-case",
        module="""module choice-single-node-case {
  yang-version 1;
  namespace "urn:choice-single-node-case";
  prefix csnc;
  revision 2026-06-14;

  choice operation {
    case create {
      leaf name { type string; }
    }
    case delete {
      leaf id { type uint32; }
    }
  }
}
""",
        input_name="input.xml",
        input_format="xml",
        formats=["xml", "json", "json_ietf"],
        oracle=True,
        input="""<name xmlns="urn:choice-single-node-case">new-entry</name>
""",
    )

    # 15. choice nested in case
    add_fixture(
        name="choice-nested-in-case",
        theme=THEME,
        module_name="choice-nested-in-case",
        module="""module choice-nested-in-case {
  yang-version 1;
  namespace "urn:choice-nested-in-case";
  prefix cnic;
  revision 2026-06-14;

  choice address-family {
    case ipv4 {
      choice routing {
        case rip {
          leaf metric { type uint8; }
        }
        case ospf {
          leaf area { type uint32; }
        }
      }
    }
    case ipv6 {
      leaf prefix { type string; }
    }
  }
}
""",
        input_name="input.xml",
        input_format="xml",
        formats=["xml", "json", "json_ietf"],
        oracle=True,
        input="""<metric xmlns="urn:choice-nested-in-case">2</metric>
""",
    )

    # 16. choice cases interleaved siblings
    add_fixture(
        name="choice-cases-interleaved-siblings",
        theme=THEME,
        module_name="choice-cases-interleaved-siblings",
        module="""module choice-cases-interleaved-siblings {
  yang-version 1;
  namespace "urn:choice-cases-interleaved-siblings";
  prefix ccis;
  revision 2026-06-14;

  container rule {
    leaf id { type string; }
    choice action-type {
      case accept {
        leaf priority { type uint8; }
      }
      case deny {
        leaf reason { type string; }
      }
    }
    leaf description { type string; }
  }
}
""",
        input_name="input.xml",
        input_format="xml",
        formats=["xml", "json", "json_ietf"],
        oracle=True,
        input="""<rule xmlns="urn:choice-cases-interleaved-siblings">
  <id>r1</id>
  <priority>10</priority>
  <description>allow</description>
</rule>
""",
    )

    # 17. choice with leaf-list branch
    add_fixture(
        name="choice-with-leaflist-branch",
        theme=THEME,
        module_name="choice-with-leaflist-branch",
        module="""module choice-with-leaflist-branch {
  yang-version 1;
  namespace "urn:choice-with-leaflist-branch";
  prefix cwlb;
  revision 2026-06-14;

  choice log-dest {
    case file {
      leaf-list paths {
        ordered-by user;
        type string;
      }
    }
    case syslog {
      leaf facility { type string; }
    }
  }
}
""",
        input_name="input.xml",
        input_format="xml",
        formats=["xml", "json", "json_ietf"],
        oracle=True,
        input="""<paths xmlns="urn:choice-with-leaflist-branch">/var/log/p3</paths>
<paths xmlns="urn:choice-with-leaflist-branch">/var/log/p1</paths>
<paths xmlns="urn:choice-with-leaflist-branch">/var/log/p2</paths>
""",
    )

    # 18. choice shorthand leaf-list/list
    add_fixture(
        name="choice-shorthand-leaflist-list",
        theme=THEME,
        module_name="choice-shorthand-leaflist-list",
        module="""module choice-shorthand-leaflist-list {
  yang-version 1;
  namespace "urn:choice-shorthand-leaflist-list";
  prefix csll;
  revision 2026-06-14;

  choice payload {
    leaf-list tags { type string; }
    list rows {
      key id;
      leaf id { type uint16; }
      leaf v { type string; }
    }
  }
}
""",
        input_name="input.xml",
        input_format="xml",
        formats=["xml", "json", "json_ietf"],
        oracle=True,
        input="""<tags xmlns="urn:choice-shorthand-leaflist-list">red</tags>
<tags xmlns="urn:choice-shorthand-leaflist-list">blue</tags>
""",
    )

    # 19. list entry with choice schema order
    add_fixture(
        name="list-entry-with-choice-schema-order",
        theme=THEME,
        module_name="list-entry-with-choice-schema-order",
        module="""module list-entry-with-choice-schema-order {
  yang-version 1;
  namespace "urn:list-entry-with-choice-schema-order";
  prefix lewcs;
  revision 2026-06-14;

  list vlan {
    key vlan-id;
    leaf vlan-id { type uint16; }
    choice native-option {
      case tagged {
        leaf tag { type empty; }
      }
      case untagged {
        leaf untag { type empty; }
      }
    }
  }
}
""",
        input_name="input.xml",
        input_format="xml",
        formats=["xml", "json", "json_ietf"],
        oracle=True,
        input="""<vlan xmlns="urn:list-entry-with-choice-schema-order">
  <vlan-id>10</vlan-id>
  <untag/>
</vlan>
""",
    )

    # 20. container within list schema order
    add_fixture(
        name="container-within-list-schema-order",
        theme=THEME,
        module_name="container-within-list-schema-order",
        module="""module container-within-list-schema-order {
  yang-version 1;
  namespace "urn:container-within-list-schema-order";
  prefix cwl;
  revision 2026-06-14;

  list device {
    key id;
    leaf id { type string; }
    container config {
      leaf enabled { type boolean; }
    }
  }
}
""",
        input_name="input.xml",
        input_format="xml",
        formats=["xml", "json", "json_ietf"],
        oracle=True,
        input="""<device xmlns="urn:container-within-list-schema-order">
  <id>d1</id>
  <config>
    <enabled>true</enabled>
  </config>
</device>
""",
    )

    # 21. leaf-list within list entry
    add_fixture(
        name="leaf-list-within-list-entry",
        theme=THEME,
        module_name="leaf-list-within-list-entry",
        module="""module leaf-list-within-list-entry {
  yang-version 1;
  namespace "urn:leaf-list-within-list-entry";
  prefix llwle;
  revision 2026-06-14;

  list policy {
    ordered-by user;
    key name;
    leaf name { type string; }
    leaf-list actions {
      ordered-by user;
      type string;
    }
  }
}
""",
        input_name="input.xml",
        input_format="xml",
        formats=["xml", "json", "json_ietf"],
        oracle=True,
        input="""<policy xmlns="urn:leaf-list-within-list-entry">
  <name>p2</name>
  <actions>b</actions>
  <actions>a</actions>
</policy>
<policy xmlns="urn:leaf-list-within-list-entry">
  <name>p1</name>
  <actions>b</actions>
  <actions>a</actions>
</policy>
""",
    )

    # 22. config true subtree
    add_fixture(
        name="config-true-subtree",
        theme=THEME,
        module_name="config-true-subtree",
        module="""module config-true-subtree {
  yang-version 1;
  namespace "urn:config-true-subtree";
  prefix cts;
  revision 2026-06-14;

  container settings {
    config true;
    leaf hostname { type string; }
    leaf debug-level { type uint8; }
  }
}
""",
        input_name="input.xml",
        input_format="xml",
        formats=["xml", "json", "json_ietf"],
        oracle=True,
        input="""<settings xmlns="urn:config-true-subtree">
  <hostname>core</hostname>
  <debug-level>3</debug-level>
</settings>
""",
    )

    # 23. config false state subtree
    add_fixture(
        name="config-false-state-subtree",
        theme=THEME,
        module_name="config-false-state-subtree",
        module="""module config-false-state-subtree {
  yang-version 1;
  namespace "urn:config-false-state-subtree";
  prefix cfs;
  revision 2026-06-14;

  container system {
    config false;
    leaf uptime { type uint64; }
    leaf load-avg { type string; }
  }
}
""",
        input_name="input.xml",
        input_format="xml",
        formats=["xml", "json", "json_ietf"],
        oracle=True,
        input="""<system xmlns="urn:config-false-state-subtree">
  <uptime>123456</uptime>
  <load-avg>0.42</load-avg>
</system>
""",
    )

    # 24. mixed config state nested
    add_fixture(
        name="mixed-config-state-nested",
        theme=THEME,
        module_name="mixed-config-state-nested",
        module="""module mixed-config-state-nested {
  yang-version 1;
  namespace "urn:mixed-config-state-nested";
  prefix mcsn;
  revision 2026-06-14;

  container system {
    leaf hostname { type string; }
    container state {
      config false;
      leaf uptime { type uint64; }
    }
  }
}
""",
        input_name="input.xml",
        input_format="xml",
        formats=["xml", "json", "json_ietf"],
        oracle=True,
        input="""<system xmlns="urn:mixed-config-state-nested">
  <hostname>core</hostname>
  <state>
    <uptime>123456</uptime>
  </state>
</system>
""",
    )

    # 25. status current/deprecated/obsolete
    add_fixture(
        name="status-current-deprecated-obsolete",
        theme=THEME,
        module_name="status-current-deprecated-obsolete",
        module="""module status-current-deprecated-obsolete {
  yang-version 1;
  namespace "urn:status-current-deprecated-obsolete";
  prefix scdo;
  revision 2026-06-14;

  leaf current-item { type string; status current; }
  leaf deprecated-item { type string; status deprecated; }
  leaf obsolete-item { type string; status obsolete; }
}
""",
        input_name="input.xml",
        input_format="xml",
        formats=["xml", "json", "json_ietf"],
        oracle=True,
        input="""<current-item xmlns="urn:status-current-deprecated-obsolete">a</current-item>
<deprecated-item xmlns="urn:status-current-deprecated-obsolete">b</deprecated-item>
""",
    )

    # 26. anydata untyped container
    add_fixture(
        name="anydata-untyped-container",
        theme=THEME,
        module_name="anydata-untyped-container",
        module="""module anydata-untyped-container {
  yang-version 1.1;
  namespace "urn:anydata-untyped-container";
  prefix auc;
  revision 2026-06-14;

  container top {
    anydata metrics;
  }
}
""",
        input_name="input.xml",
        input_format="xml",
        formats=["xml", "json", "json_ietf"],
        oracle=True,
        input="""<top xmlns="urn:anydata-untyped-container">
  <metrics>
    <custom>value</custom>
  </metrics>
</top>
""",
    )

    # 27. anyxml opaque passthrough
    add_fixture(
        name="anyxml-opaque-passthrough",
        theme=THEME,
        module_name="anyxml-opaque-passthrough",
        module="""module anyxml-opaque-passthrough {
  yang-version 1;
  namespace "urn:anyxml-opaque-passthrough";
  prefix aop;
  revision 2026-06-14;

  container rpc-reply {
    anyxml data;
  }
}
""",
        input_name="input.xml",
        input_format="xml",
        formats=["xml", "json", "json_ietf"],
        oracle=True,
        input="""<rpc-reply xmlns="urn:anyxml-opaque-passthrough">
  <data>
    <foo>
      <bar>x</bar>
    </foo>
  </data>
</rpc-reply>
""",
    )

    # 28. anyxml attributes namespaced
    add_fixture(
        name="anyxml-attributes-namespaced",
        theme=THEME,
        module_name="anyxml-attributes-namespaced",
        module="""module anyxml-attributes-namespaced {
  yang-version 1;
  namespace "urn:anyxml-attributes-namespaced";
  prefix aan;
  revision 2026-06-14;

  container top {
    anyxml payload;
  }
}
""",
        input_name="input.xml",
        input_format="xml",
        formats=["xml"],
        oracle=True,
        input="""<top xmlns="urn:anyxml-attributes-namespaced">
  <payload>
    <ns:foo xmlns:ns="urn:x" attr="1">
      <bar>v</bar>
    </ns:foo>
  </payload>
</top>
""",
    )

    print("Theme 7 fixtures generated.")


if __name__ == "__main__":
    main()
