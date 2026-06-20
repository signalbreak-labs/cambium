#!/usr/bin/env python3
"""Batch-generate theme 6: constraints-conditionals fixtures."""
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).parent))
from conformance_lib import CONFORMANCE, add_fixture

THEME = "constraints-conditionals"


def write_invalid(name: str, xml: str) -> None:
    (CONFORMANCE / "fixtures" / name / "input-invalid.xml").write_text(xml)


def main():
    # 1. when + must + error-message/app-tag
    add_fixture(
        name="constraints-when-must",
        theme=THEME,
        module_name="constraints-when-must",
        module="""module constraints-when-must {
  yang-version 1;
  namespace "urn:constraints-when-must";
  prefix cwm;
  revision 2026-06-14;

  leaf kind { type string; }
  container detail {
    when "../kind = 'a' or ../kind = 'b'";
    leaf info {
      type string;
      must "string-length(.) > 0" {
        error-message "info required";
        error-app-tag "must-violation";
      }
    }
  }
}
""",
        input_name="input.xml",
        input_format="xml",
        formats=["xml", "json", "json_ietf"],
        oracle=True,
        input="""<kind xmlns="urn:constraints-when-must">a</kind>
<detail xmlns="urn:constraints-when-must">
  <info>x</info>
</detail>
""",
    )
    write_invalid(
        "constraints-when-must",
        """<kind xmlns="urn:constraints-when-must">a</kind>
<detail xmlns="urn:constraints-when-must">
  <info></info>
</detail>
""",
    )

    # 2. when with XPath functions
    add_fixture(
        name="constraints-when-xpath-functions",
        theme=THEME,
        module_name="constraints-when-xpath-functions",
        module="""module constraints-when-xpath-functions {
  yang-version 1.1;
  namespace "urn:constraints-when-xpath-functions";
  prefix cwxf;
  revision 2026-06-14;

  identity service-type;
  identity firewall { base service-type; }
  identity next-gen-firewall { base firewall; }
  identity routing { base service-type; }

  leaf service-id { type identityref { base service-type; } }
  container fw-rules {
    when "derived-from(../service-id, 'firewall')";
    leaf rule-count { type uint32; }
  }

  leaf hostname { type string; }
  container regex-cfg {
    when "re-match(../hostname, 'prod-.*')";
    leaf region { type string; }
  }

  leaf-list tags { type string; }
  container tag-cfg {
    when "count(../tags) > 0";
    leaf tag-count { type uint32; }
  }
}
""",
        input_name="input.xml",
        input_format="xml",
        formats=["xml", "json", "json_ietf"],
        oracle=True,
        input="""<service-id xmlns="urn:constraints-when-xpath-functions">next-gen-firewall</service-id>
<fw-rules xmlns="urn:constraints-when-xpath-functions">
  <rule-count>10</rule-count>
</fw-rules>
<hostname xmlns="urn:constraints-when-xpath-functions">prod-router-1</hostname>
<regex-cfg xmlns="urn:constraints-when-xpath-functions">
  <region>us-east</region>
</regex-cfg>
<tags xmlns="urn:constraints-when-xpath-functions">red</tags>
<tag-cfg xmlns="urn:constraints-when-xpath-functions">
  <tag-count>1</tag-count>
</tag-cfg>
""",
    )

    # 3. multiple must constraints
    add_fixture(
        name="constraints-must-multiple",
        theme=THEME,
        module_name="constraints-must-multiple",
        module="""module constraints-must-multiple {
  yang-version 1;
  namespace "urn:constraints-must-multiple";
  prefix cmm;
  revision 2026-06-14;

  container network {
    leaf mtu {
      type uint16 { range "512..65535"; }
    }
    leaf payload { type uint16; }
    must "payload < mtu" {
      error-app-tag "payload-check";
    }
    must "mtu <= 9000" {
      error-app-tag "mtu-limit";
    }
    leaf qos { type string; }
    must "qos != 'strict' or mtu >= 1000" {
      error-app-tag "qos-rule";
    }
  }
}
""",
        input_name="input.xml",
        input_format="xml",
        formats=["xml", "json", "json_ietf"],
        oracle=True,
        input="""<network xmlns="urn:constraints-must-multiple">
  <mtu>1500</mtu>
  <payload>1000</payload>
  <qos>best-effort</qos>
</network>
""",
    )

    # 4. unique single/composite/descendant
    add_fixture(
        name="constraints-unique-composite",
        theme=THEME,
        module_name="constraints-unique-composite",
        module="""module constraints-unique-composite {
  yang-version 1;
  namespace "urn:constraints-unique-composite";
  prefix cuc;
  revision 2026-06-14;

  list policy {
    key id;
    leaf id { type uint32; }
    leaf name { type string; }
    leaf priority { type uint8; }
    unique "name";
    unique "priority name";
    container rules {
      list rule {
        key rule-id;
        leaf rule-id { type uint32; }
        leaf src-ip { type string; }
        unique "rule-id";
      }
    }
  }
}
""",
        input_name="input.xml",
        input_format="xml",
        formats=["xml", "json", "json_ietf"],
        oracle=True,
        input="""<policy xmlns="urn:constraints-unique-composite">
  <id>1</id>
  <name>allow</name>
  <priority>10</priority>
  <rules>
    <rule>
      <rule-id>100</rule-id>
      <src-ip>10.0.0.0/8</src-ip>
    </rule>
  </rules>
</policy>
<policy xmlns="urn:constraints-unique-composite">
  <id>2</id>
  <name>deny</name>
  <priority>20</priority>
  <rules>
    <rule>
      <rule-id>200</rule-id>
      <src-ip>192.168.0.0/16</src-ip>
    </rule>
  </rules>
</policy>
""",
    )

    # 5. unique violation reject
    add_fixture(
        name="constraints-unique-violation-reject",
        theme=THEME,
        module_name="constraints-unique-violation-reject",
        module="""module constraints-unique-violation-reject {
  yang-version 1;
  namespace "urn:constraints-unique-violation-reject";
  prefix cuvr;
  revision 2026-06-14;

  list entry {
    key id;
    leaf id { type uint16; }
    leaf a { type string; }
    leaf b { type string; }
    unique "a b";
  }
}
""",
        input_name="input.xml",
        input_format="xml",
        formats=["xml", "json", "json_ietf"],
        oracle=True,
        input="""<entry xmlns="urn:constraints-unique-violation-reject">
  <id>1</id>
  <a>x</a>
  <b>p</b>
</entry>
<entry xmlns="urn:constraints-unique-violation-reject">
  <id>2</id>
  <a>y</a>
  <b>q</b>
</entry>
""",
    )
    write_invalid(
        "constraints-unique-violation-reject",
        """<entry xmlns="urn:constraints-unique-violation-reject">
  <id>1</id>
  <a>x</a>
  <b>p</b>
</entry>
<entry xmlns="urn:constraints-unique-violation-reject">
  <id>2</id>
  <a>x</a>
  <b>p</b>
</entry>
""",
    )

    # 6. mandatory + default + when + refine
    add_fixture(
        name="constraints-mandatory-interaction",
        theme=THEME,
        module_name="constraints-mandatory-interaction",
        module="""module constraints-mandatory-interaction {
  yang-version 1;
  namespace "urn:constraints-mandatory-interaction";
  prefix cmi;
  revision 2026-06-14;

  leaf hostname { type string; mandatory true; }
  leaf domain { type string; default "example.com"; }
  leaf mode { type string; }
  container services {
    when "../mode = 'operational'";
    leaf primary-server { type string; mandatory true; }
  }

  grouping auth-config {
    leaf username { type string; }
    leaf password { type string; }
  }
  container auth {
    uses auth-config {
      refine username { mandatory true; }
      refine password { default "unset"; }
    }
  }
}
""",
        input_name="input.xml",
        input_format="xml",
        formats=["xml", "json", "json_ietf"],
        oracle=True,
        input="""<hostname xmlns="urn:constraints-mandatory-interaction">router1</hostname>
<domain xmlns="urn:constraints-mandatory-interaction">example.com</domain>
<mode xmlns="urn:constraints-mandatory-interaction">operational</mode>
<services xmlns="urn:constraints-mandatory-interaction">
  <primary-server>10.0.0.1</primary-server>
</services>
<auth xmlns="urn:constraints-mandatory-interaction">
  <username>admin</username>
  <password>unset</password>
</auth>
""",
    )

    # 7. defaults on leaf/leaf-list/choice
    add_fixture(
        name="constraints-default-types",
        theme=THEME,
        module_name="constraints-default-types",
        module="""module constraints-default-types {
  yang-version 1.1;
  namespace "urn:constraints-default-types";
  prefix cdt;
  revision 2026-06-14;

  leaf hostname { type string; default "localhost"; }
  leaf port { type uint16; default 8080; }
  leaf enabled { type boolean; default true; }
  leaf-list nameserver { type string; default "8.8.8.8"; default "8.8.4.4"; }
  choice log-backend {
    default syslog;
    case syslog {
      leaf facility { type string; default "local0"; }
    }
    case file {
      leaf path { type string; }
    }
    case remote {
      leaf server { type string; }
    }
  }
}
""",
        input_name="input.xml",
        input_format="xml",
        formats=["xml", "json", "json_ietf"],
        oracle=True,
        input="""<hostname xmlns="urn:constraints-default-types">localhost</hostname>
<port xmlns="urn:constraints-default-types">8080</port>
<enabled xmlns="urn:constraints-default-types">true</enabled>
<nameserver xmlns="urn:constraints-default-types">8.8.8.8</nameserver>
<nameserver xmlns="urn:constraints-default-types">8.8.4.4</nameserver>
<facility xmlns="urn:constraints-default-types">local0</facility>
""",
    )

    # 8. feature + if-feature boolean expressions
    add_fixture(
        name="constraints-feature-iffeature",
        theme=THEME,
        module_name="constraints-feature-iffeature",
        module="""module constraints-feature-iffeature {
  yang-version 1.1;
  namespace "urn:constraints-feature-iffeature";
  prefix cfif;
  revision 2026-06-14;

  feature high-security;

  container stats {
    if-feature "not high-security";
    leaf packet-count { type uint64; }
    leaf dropped-packets {
      if-feature high-security;
      type uint64;
    }
  }
}
""",
        input_name="input.xml",
        input_format="xml",
        formats=["xml", "json", "json_ietf"],
        oracle=True,
        input="""<stats xmlns="urn:constraints-feature-iffeature">
  <packet-count>42</packet-count>
</stats>
""",
    )

    # 9. feature-on-feature dependency
    add_fixture(
        name="constraints-feature-dependency",
        theme=THEME,
        module_name="constraints-feature-dependency",
        module="""module constraints-feature-dependency {
  yang-version 1.1;
  namespace "urn:constraints-feature-dependency";
  prefix cfd;
  revision 2026-06-14;

  feature a;
  feature b {
    if-feature a;
  }
  container top {
    leaf gated {
      if-feature "not b";
      type string;
    }
  }
}
""",
        input_name="input.xml",
        input_format="xml",
        formats=["xml", "json", "json_ietf"],
        oracle=True,
        input="""<top xmlns="urn:constraints-feature-dependency">
  <gated>enabled</gated>
</top>
""",
    )

    # 10. presence container + if-feature
    add_fixture(
        name="constraints-feature-presence",
        theme=THEME,
        module_name="constraints-feature-presence",
        module="""module constraints-feature-presence {
  yang-version 1.1;
  namespace "urn:constraints-feature-presence";
  prefix cfp;
  revision 2026-06-14;

  feature candidate;

  container ssh {
    presence "enable ssh";
    leaf port { type uint16; default 22; }
  }
  container candidate-store {
    if-feature candidate;
    presence "enable candidate";
    leaf capacity { type uint32; }
  }
}
""",
        input_name="input.xml",
        input_format="xml",
        formats=["xml", "json", "json_ietf"],
        oracle=True,
        input="""<ssh xmlns="urn:constraints-feature-presence"/>
""",
    )

    print("Theme 6 fixtures generated.")


if __name__ == "__main__":
    main()
