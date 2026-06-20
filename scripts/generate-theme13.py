#!/usr/bin/env python3
"""Generate Theme 13: linkage fixtures (28)."""
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
from conformance_lib import add_fixture, add_enabled, run_rust_runner, run_go_runner


def write_extra_modules(name: str, modules: dict) -> None:
    """Write additional .yang files into fixtures/<name>/module/ before add_fixture."""
    module_dir = Path(__file__).resolve().parent.parent / "conformance" / "fixtures" / name / "module"
    module_dir.mkdir(parents=True, exist_ok=True)
    for filename, text in modules.items():
        (module_dir / filename).write_text(text)


# ---------------------------------------------------------------------------
# 1. grouping simple
# ---------------------------------------------------------------------------
add_fixture(
    name="linkage-grouping-simple",
    theme="linkage",
    module="""module linkage-grouping-simple {
  yang-version 1.1;
  namespace "urn:linkage-grouping-simple";
  prefix lgs;
  revision 2026-06-15;

  grouping addr {
    leaf ip {
      type string;
    }
    leaf mask {
      type uint8;
    }
  }

  container intf {
    leaf name {
      type string;
    }
    uses addr;
    leaf mtu {
      type uint16;
    }
  }
}""",
    module_name="linkage-grouping-simple",
    input="""<intf xmlns="urn:linkage-grouping-simple">
  <mtu>1500</mtu>
  <name>eth0</name>
  <mask>24</mask>
  <ip>192.0.2.1</ip>
</intf>
""",
    input_name="input.xml",
    input_format="xml",
    formats=["xml", "json", "json_ietf"],
    oracle=True,
)
add_enabled("linkage-grouping-simple")

# ---------------------------------------------------------------------------
# 2. grouping nested uses
# ---------------------------------------------------------------------------
add_fixture(
    name="linkage-grouping-nested-uses",
    theme="linkage",
    module="""module linkage-grouping-nested-uses {
  yang-version 1.1;
  namespace "urn:linkage-grouping-nested-uses";
  prefix lgnu;
  revision 2026-06-15;

  grouping base-addr {
    leaf ip {
      type string;
    }
    leaf mask {
      type uint8;
    }
  }

  grouping extended-addr {
    uses base-addr;
    leaf gateway {
      type string;
    }
  }

  container intf {
    leaf name {
      type string;
    }
    uses extended-addr;
  }
}""",
    module_name="linkage-grouping-nested-uses",
    input="""<intf xmlns="urn:linkage-grouping-nested-uses">
  <gateway>192.0.2.254</gateway>
  <name>eth0</name>
  <mask>24</mask>
  <ip>192.0.2.1</ip>
</intf>
""",
    input_name="input.xml",
    input_format="xml",
    formats=["xml", "json", "json_ietf"],
    oracle=True,
)
add_enabled("linkage-grouping-nested-uses")

# ---------------------------------------------------------------------------
# 3. grouping config + state reuse
# ---------------------------------------------------------------------------
add_fixture(
    name="linkage-grouping-config-state",
    theme="linkage",
    module="""module linkage-grouping-config-state {
  yang-version 1.1;
  namespace "urn:linkage-grouping-config-state";
  prefix lgcs;
  revision 2026-06-15;

  grouping addr-params {
    leaf ip {
      type string;
    }
    leaf mask {
      type uint8;
    }
  }

  container intf {
    leaf name {
      type string;
    }
    container config {
      uses addr-params;
    }
    container state {
      config false;
      uses addr-params;
    }
  }
}""",
    module_name="linkage-grouping-config-state",
    input="""<intf xmlns="urn:linkage-grouping-config-state">
  <state>
    <mask>24</mask>
    <ip>192.0.2.1</ip>
  </state>
  <name>eth0</name>
  <config>
    <mask>24</mask>
    <ip>192.0.2.1</ip>
  </config>
</intf>
""",
    input_name="input.xml",
    input_format="xml",
    formats=["xml", "json", "json_ietf"],
    oracle=True,
)
add_enabled("linkage-grouping-config-state")

# ---------------------------------------------------------------------------
# 4. grouping cross-module
# ---------------------------------------------------------------------------
write_extra_modules(
    "linkage-grouping-cross-module",
    {
        "linkage-grouping-cross-module-base.yang": """module linkage-grouping-cross-module-base {
  yang-version 1.1;
  namespace "urn:linkage-grouping-cross-module-base";
  prefix lgcb;
  revision 2026-06-15;

  grouping shared-addr {
    leaf ip {
      type string;
    }
    leaf mask {
      type uint8;
    }
  }
}""",
    },
)
add_fixture(
    name="linkage-grouping-cross-module",
    theme="linkage",
    module="""module linkage-grouping-cross-module {
  yang-version 1.1;
  namespace "urn:linkage-grouping-cross-module";
  prefix lgcm;

  import linkage-grouping-cross-module-base {
    prefix lgcmb;
  }

  revision 2026-06-15;

  container intf {
    uses lgcmb:shared-addr;
  }
}""",
    module_name="linkage-grouping-cross-module",
    input="""<intf xmlns="urn:linkage-grouping-cross-module">
  <mask>24</mask>
  <ip>192.0.2.1</ip>
</intf>
""",
    input_name="input.xml",
    input_format="xml",
    formats=["xml", "json", "json_ietf"],
    oracle=True,
)
add_enabled("linkage-grouping-cross-module")

# ---------------------------------------------------------------------------
# 5. refine default
# ---------------------------------------------------------------------------
add_fixture(
    name="linkage-refine-default",
    theme="linkage",
    module="""module linkage-refine-default {
  yang-version 1.1;
  namespace "urn:linkage-refine-default";
  prefix lrd;
  revision 2026-06-15;

  grouping auth {
    leaf alg {
      type string;
    }
    leaf key {
      type string;
    }
  }

  container ntp {
    presence "enable ntp";
    uses auth {
      refine alg {
        default "sha256";
      }
      refine key {
        default "secret";
      }
    }
  }
}""",
    module_name="linkage-refine-default",
    input="""<ntp xmlns="urn:linkage-refine-default">
  <key>secret</key>
  <alg>sha256</alg>
</ntp>
""",
    input_name="input.xml",
    input_format="xml",
    formats=["xml", "json", "json_ietf"],
    oracle=True,
    serialize_defaults="all",
)
add_enabled("linkage-refine-default")

# ---------------------------------------------------------------------------
# 6. refine mandatory + config false
# ---------------------------------------------------------------------------
add_fixture(
    name="linkage-refine-mandatory-config",
    theme="linkage",
    module="""module linkage-refine-mandatory-config {
  yang-version 1.1;
  namespace "urn:linkage-refine-mandatory-config";
  prefix lrmc;
  revision 2026-06-15;

  grouping params {
    leaf name {
      type string;
    }
    leaf secret {
      type string;
    }
  }

  container service {
    uses params {
      refine name {
        mandatory true;
      }
      refine secret {
        config false;
      }
    }
  }
}""",
    module_name="linkage-refine-mandatory-config",
    input="""<service xmlns="urn:linkage-refine-mandatory-config">
  <name>demo</name>
</service>
""",
    input_name="input.xml",
    input_format="xml",
    formats=["xml", "json", "json_ietf"],
    oracle=True,
)
add_enabled("linkage-refine-mandatory-config")

# ---------------------------------------------------------------------------
# 7. refine presence + must
# ---------------------------------------------------------------------------
add_fixture(
    name="linkage-refine-presence-must",
    theme="linkage",
    module="""module linkage-refine-presence-must {
  yang-version 1.1;
  namespace "urn:linkage-refine-presence-must";
  prefix lrpm;
  revision 2026-06-15;

  grouping shared-container {
    container opts {
      leaf val {
        type uint8;
      }
    }
  }

  container system {
    uses shared-container {
      refine opts {
        presence "enable opts";
      }
      refine opts/val {
        must "current() > 0" {
          error-message "val > 0";
        }
      }
    }
  }
}""",
    module_name="linkage-refine-presence-must",
    input="""<system xmlns="urn:linkage-refine-presence-must">
  <opts>
    <val>5</val>
  </opts>
</system>
""",
    input_name="input.xml",
    input_format="xml",
    formats=["xml", "json", "json_ietf"],
    oracle=True,
)
add_enabled("linkage-refine-presence-must")

# ---------------------------------------------------------------------------
# 8. refine min/max + if-feature
# ---------------------------------------------------------------------------
add_fixture(
    name="linkage-refine-min-max-iffeature",
    theme="linkage",
    module="""module linkage-refine-min-max-iffeature {
  yang-version 1.1;
  namespace "urn:linkage-refine-min-max-iffeature";
  prefix lrmmi;
  revision 2026-06-15;

  feature advanced;

  grouping tags-group {
    leaf-list tags {
      type string;
    }
  }

  grouping basic {
    leaf mode {
      type string;
    }
    leaf advanced-opt {
      type string;
    }
  }

  container policy {
    uses tags-group {
      refine tags {
        min-elements 1;
        max-elements 5;
      }
    }
  }

  container service {
    uses basic {
      refine advanced-opt {
        if-feature advanced;
      }
    }
  }
}""",
    module_name="linkage-refine-min-max-iffeature",
    input="""<lrmmi:policy xmlns:lrmmi="urn:linkage-refine-min-max-iffeature">
  <lrmmi:tags>a</lrmmi:tags>
  <lrmmi:tags>b</lrmmi:tags>
</lrmmi:policy>
<lrmmi:service xmlns:lrmmi="urn:linkage-refine-min-max-iffeature">
  <lrmmi:mode>auto</lrmmi:mode>
</lrmmi:service>
""",
    input_name="input.xml",
    input_format="xml",
    formats=["xml", "json", "json_ietf"],
    oracle=True,
)
add_enabled("linkage-refine-min-max-iffeature")

# ---------------------------------------------------------------------------
# 9. augment intra-module with when
# ---------------------------------------------------------------------------
add_fixture(
    name="linkage-augment-intra-module",
    theme="linkage",
    module="""module linkage-augment-intra-module {
  yang-version 1.1;
  namespace "urn:linkage-augment-intra-module";
  prefix laim;
  revision 2026-06-15;

  container interfaces {
    list interface {
      key "name";
      leaf name {
        type string;
      }
      leaf type {
        type string;
      }
    }
  }

  augment "/laim:interfaces/laim:interface" {
    when "type = 'eth'";
    leaf speed {
      type uint32;
    }
  }
}""",
    module_name="linkage-augment-intra-module",
    input="""<interfaces xmlns="urn:linkage-augment-intra-module">
  <interface>
    <speed>1000</speed>
    <type>eth</type>
    <name>eth0</name>
  </interface>
</interfaces>
""",
    input_name="input.xml",
    input_format="xml",
    formats=["xml", "json", "json_ietf"],
    oracle=True,
)
add_enabled("linkage-augment-intra-module")

# ---------------------------------------------------------------------------
# 10. augment container + leaf + leaf-list
# ---------------------------------------------------------------------------
add_fixture(
    name="linkage-augment-container-leaf-list",
    theme="linkage",
    module="""module linkage-augment-container-leaf-list {
  yang-version 1.1;
  namespace "urn:linkage-augment-container-leaf-list";
  prefix lacll;
  revision 2026-06-15;

  container top {
    leaf id {
      type string;
    }
  }

  augment "/lacll:top" {
    container stats {
      leaf packets {
        type uint64;
      }
      leaf bytes {
        type uint64;
      }
    }
    leaf mtu {
      type uint16;
    }
    leaf-list tags {
      type string;
    }
  }
}""",
    module_name="linkage-augment-container-leaf-list",
    input="""<top xmlns="urn:linkage-augment-container-leaf-list">
  <tags>red</tags>
  <tags>blue</tags>
  <mtu>9000</mtu>
  <id>node-1</id>
  <stats>
    <bytes>4096</bytes>
    <packets>64</packets>
  </stats>
</top>
""",
    input_name="input.xml",
    input_format="xml",
    formats=["xml", "json", "json_ietf"],
    oracle=True,
)
add_enabled("linkage-augment-container-leaf-list")

# ---------------------------------------------------------------------------
# 11. augment choice/case
# ---------------------------------------------------------------------------
add_fixture(
    name="linkage-augment-choice-case",
    theme="linkage",
    module="""module linkage-augment-choice-case {
  yang-version 1.1;
  namespace "urn:linkage-augment-choice-case";
  prefix lacc;
  revision 2026-06-15;

  container config {
    choice transport {
      case tcp {
        leaf tcp-port {
          type uint16;
        }
      }
      case udp {
        leaf udp-port {
          type uint16;
        }
      }
    }
  }

  augment "/lacc:config/lacc:transport" {
    case quic {
      leaf quic-port {
        type uint16;
      }
    }
  }

  augment "/lacc:config/lacc:transport/lacc:udp" {
    leaf timeout {
      type uint32;
    }
  }
}""",
    module_name="linkage-augment-choice-case",
    input="""<config xmlns="urn:linkage-augment-choice-case">
  <timeout>30</timeout>
  <udp-port>5000</udp-port>
</config>
""",
    input_name="input.xml",
    input_format="xml",
    formats=["xml", "json", "json_ietf"],
    oracle=True,
)
add_enabled("linkage-augment-choice-case")

# ---------------------------------------------------------------------------
# 12. augment nested
# ---------------------------------------------------------------------------
add_fixture(
    name="linkage-augment-nested",
    theme="linkage",
    module="""module linkage-augment-nested {
  yang-version 1.1;
  namespace "urn:linkage-augment-nested";
  prefix lan;
  revision 2026-06-15;

  container dev {
    leaf id {
      type string;
    }
  }

  augment "/lan:dev" {
    container status {
      leaf ready {
        type boolean;
      }
    }
  }

  augment "/lan:dev/lan:status" {
    leaf timestamp {
      type string;
    }
  }
}""",
    module_name="linkage-augment-nested",
    input="""<dev xmlns="urn:linkage-augment-nested">
  <status>
    <timestamp>2026-06-15T00:00:00Z</timestamp>
    <ready>true</ready>
  </status>
  <id>dev-1</id>
</dev>
""",
    input_name="input.xml",
    input_format="xml",
    formats=["xml", "json", "json_ietf"],
    oracle=True,
)
add_enabled("linkage-augment-nested")

# ---------------------------------------------------------------------------
# 13. augment inter-module
# ---------------------------------------------------------------------------
write_extra_modules(
    "linkage-augment-inter-module",
    {
        "linkage-augment-inter-module-base.yang": """module linkage-augment-inter-module-base {
  yang-version 1.1;
  namespace "urn:linkage-augment-inter-module-base";
  prefix liaim-b;
  revision 2026-06-15;

  container interfaces {
    list interface {
      key "name";
      leaf name {
        type string;
      }
      leaf type {
        type string;
      }
    }
  }
}""",
    },
)
add_fixture(
    name="linkage-augment-inter-module",
    theme="linkage",
    module="""module linkage-augment-inter-module {
  yang-version 1.1;
  namespace "urn:linkage-augment-inter-module";
  prefix liaim;

  import linkage-augment-inter-module-base {
    prefix liaim-b;
  }

  revision 2026-06-15;

  augment "/liaim-b:interfaces/liaim-b:interface" {
    leaf speed {
      type uint32;
    }
  }
}""",
    module_name="linkage-augment-inter-module",
    input="""<interfaces xmlns="urn:linkage-augment-inter-module-base" xmlns:liaim="urn:linkage-augment-inter-module">
  <interface>
    <liaim:speed>1000</liaim:speed>
    <name>eth0</name>
    <type>eth</type>
  </interface>
</interfaces>
""",
    input_name="input.xml",
    input_format="xml",
    formats=["xml", "json", "json_ietf"],
    oracle=True,
)
add_enabled("linkage-augment-inter-module")

# ---------------------------------------------------------------------------
# 14. cross-module ident collision
# ---------------------------------------------------------------------------
write_extra_modules(
    "augment-cross-module-ident-collision",
    {
        "augment-cross-module-ident-collision-base.yang": """module augment-cross-module-ident-collision-base {
  yang-version 1.1;
  namespace "urn:augment-cross-module-ident-collision-base";
  prefix acmib;
  revision 2026-06-15;

  container top {
    container config {
      leaf v {
        type string;
      }
    }
    leaf status {
      type string;
    }
  }
}""",
    },
)
add_fixture(
    name="augment-cross-module-ident-collision",
    theme="linkage",
    module="""module augment-cross-module-ident-collision {
  yang-version 1.1;
  namespace "urn:augment-cross-module-ident-collision";
  prefix acmic;

  import augment-cross-module-ident-collision-base {
    prefix acmib;
  }

  revision 2026-06-15;

  augment "/acmib:top" {
    container config-extra {
      leaf w {
        type string;
      }
    }
    leaf config-status {
      type string;
    }
  }
}""",
    module_name="augment-cross-module-ident-collision",
    input="""<top xmlns="urn:augment-cross-module-ident-collision-base" xmlns:acmic="urn:augment-cross-module-ident-collision">
  <acmic:config-status>ok</acmic:config-status>
  <config>
    <v>cfg</v>
  </config>
  <acmic:config-extra>
    <acmic:w>extra</acmic:w>
  </acmic:config-extra>
  <status>up</status>
</top>
""",
    input_name="input.xml",
    input_format="xml",
    formats=["xml", "json", "json_ietf"],
    oracle=True,
)
add_enabled("augment-cross-module-ident-collision")

# ---------------------------------------------------------------------------
# 15. augment when with target context
# ---------------------------------------------------------------------------
write_extra_modules(
    "augment-when-target-context",
    {
        "augment-when-target-context-base.yang": """module augment-when-target-context-base {
  yang-version 1.1;
  namespace "urn:augment-when-target-context-base";
  prefix awtc;
  revision 2026-06-15;

  container system {
    leaf mode {
      type string;
    }
    container ospf {
      leaf router-id {
        type string;
      }
    }
  }
}""",
    },
)
add_fixture(
    name="augment-when-target-context",
    theme="linkage",
    module="""module augment-when-target-context {
  yang-version 1.1;
  namespace "urn:augment-when-target-context";
  prefix awtca;

  import augment-when-target-context-base {
    prefix awtc;
  }

  revision 2026-06-15;

  augment "/awtc:system/awtc:ospf" {
    when "../awtc:mode = 'enabled'";
    leaf area {
      type string;
    }
  }
}""",
    module_name="augment-when-target-context",
    input="""<system xmlns="urn:augment-when-target-context-base" xmlns:awtca="urn:augment-when-target-context">
  <mode>enabled</mode>
  <ospf>
    <awtca:area>0</awtca:area>
    <router-id>1.1.1.1</router-id>
  </ospf>
</system>
""",
    input_name="input.xml",
    input_format="xml",
    formats=["xml", "json", "json_ietf"],
    oracle=True,
)
add_enabled("augment-when-target-context")

# ---------------------------------------------------------------------------
# 16. deviation not-supported
# ---------------------------------------------------------------------------
write_extra_modules(
    "linkage-deviation-not-supported",
    {
        "linkage-deviation-not-supported-base.yang": """module linkage-deviation-not-supported-base {
  yang-version 1.1;
  namespace "urn:linkage-deviation-not-supported-base";
  prefix ldnsb;
  revision 2026-06-15;

  container c {
    leaf deprecated-field {
      type string;
    }
    leaf active-field {
      type string;
    }
  }
}""",
        "linkage-deviation-not-supported-dev.yang": """module linkage-deviation-not-supported-dev {
  yang-version 1.1;
  namespace "urn:linkage-deviation-not-supported-dev";
  prefix ldnsd;

  import linkage-deviation-not-supported-base {
    prefix ldnsb;
  }

  revision 2026-06-15;

  deviation "/ldnsb:c/ldnsb:deprecated-field" {
    deviate not-supported;
  }
}""",
    },
)
add_fixture(
    name="linkage-deviation-not-supported",
    theme="linkage",
    module="""module linkage-deviation-not-supported-base {
  yang-version 1.1;
  namespace "urn:linkage-deviation-not-supported-base";
  prefix ldnsb;
  revision 2026-06-15;

  container c {
    leaf deprecated-field {
      type string;
    }
    leaf active-field {
      type string;
    }
  }
}""",
    module_name="linkage-deviation-not-supported-base",
    input="""<c xmlns="urn:linkage-deviation-not-supported-base">
  <active-field>ok</active-field>
</c>
""",
    input_name="input.xml",
    input_format="xml",
    formats=["xml", "json", "json_ietf"],
    oracle=True,
)
add_enabled("linkage-deviation-not-supported")

# ---------------------------------------------------------------------------
# 17. deviation replace type
# ---------------------------------------------------------------------------
write_extra_modules(
    "linkage-deviation-replace-type",
    {
        "linkage-deviation-replace-type-base.yang": """module linkage-deviation-replace-type-base {
  yang-version 1.1;
  namespace "urn:linkage-deviation-replace-type-base";
  prefix ldrb;
  revision 2026-06-15;

  container c {
    leaf count {
      type uint64;
    }
  }
}""",
        "linkage-deviation-replace-type-dev.yang": """module linkage-deviation-replace-type-dev {
  yang-version 1.1;
  namespace "urn:linkage-deviation-replace-type-dev";
  prefix ldrd;

  import linkage-deviation-replace-type-base {
    prefix ldrb;
  }

  revision 2026-06-15;

  deviation "/ldrb:c/ldrb:count" {
    deviate replace {
      type uint32 {
        range "1..1000";
      }
    }
  }
}""",
    },
)
add_fixture(
    name="linkage-deviation-replace-type",
    theme="linkage",
    module="""module linkage-deviation-replace-type-base {
  yang-version 1.1;
  namespace "urn:linkage-deviation-replace-type-base";
  prefix ldrb;
  revision 2026-06-15;

  container c {
    leaf count {
      type uint64;
    }
  }
}""",
    module_name="linkage-deviation-replace-type-base",
    input="""<c xmlns="urn:linkage-deviation-replace-type-base">
  <count>500</count>
</c>
""",
    input_name="input.xml",
    input_format="xml",
    formats=["xml", "json", "json_ietf"],
    oracle=True,
)
add_enabled("linkage-deviation-replace-type")

# ---------------------------------------------------------------------------
# 18. deviation add mandatory
# ---------------------------------------------------------------------------
write_extra_modules(
    "linkage-deviation-add",
    {
        "linkage-deviation-add-base.yang": """module linkage-deviation-add-base {
  yang-version 1.1;
  namespace "urn:linkage-deviation-add-base";
  prefix ldab;
  revision 2026-06-15;

  container c {
    leaf optional-name {
      type string;
    }
  }
}""",
        "linkage-deviation-add-dev.yang": """module linkage-deviation-add-dev {
  yang-version 1.1;
  namespace "urn:linkage-deviation-add-dev";
  prefix ldad;

  import linkage-deviation-add-base {
    prefix ldab;
  }

  revision 2026-06-15;

  deviation "/ldab:c/ldab:optional-name" {
    deviate add {
      mandatory true;
    }
  }
}""",
    },
)
add_fixture(
    name="linkage-deviation-add",
    theme="linkage",
    module="""module linkage-deviation-add-base {
  yang-version 1.1;
  namespace "urn:linkage-deviation-add-base";
  prefix ldab;
  revision 2026-06-15;

  container c {
    leaf optional-name {
      type string;
    }
  }
}""",
    module_name="linkage-deviation-add-base",
    input="""<c xmlns="urn:linkage-deviation-add-base">
  <optional-name>present</optional-name>
</c>
""",
    input_name="input.xml",
    input_format="xml",
    formats=["xml", "json", "json_ietf"],
    oracle=True,
)
add_enabled("linkage-deviation-add")

# ---------------------------------------------------------------------------
# 19. deviation delete default
# ---------------------------------------------------------------------------
write_extra_modules(
    "linkage-deviation-delete",
    {
        "linkage-deviation-delete-base.yang": """module linkage-deviation-delete-base {
  yang-version 1.1;
  namespace "urn:linkage-deviation-delete-base";
  prefix lddb;
  revision 2026-06-15;

  container c {
    presence "enable c";
    leaf flag {
      type boolean;
      default true;
    }
  }
}""",
        "linkage-deviation-delete-dev.yang": """module linkage-deviation-delete-dev {
  yang-version 1.1;
  namespace "urn:linkage-deviation-delete-dev";
  prefix lddd;

  import linkage-deviation-delete-base {
    prefix lddb;
  }

  revision 2026-06-15;

  deviation "/lddb:c/lddb:flag" {
    deviate delete {
      default true;
    }
  }
}""",
    },
)
add_fixture(
    name="linkage-deviation-delete",
    theme="linkage",
    module="""module linkage-deviation-delete-base {
  yang-version 1.1;
  namespace "urn:linkage-deviation-delete-base";
  prefix lddb;
  revision 2026-06-15;

  container c {
    presence "enable c";
    leaf flag {
      type boolean;
      default true;
    }
  }
}""",
    module_name="linkage-deviation-delete-base",
    input="""<c xmlns="urn:linkage-deviation-delete-base"/>
""",
    input_name="input.xml",
    input_format="xml",
    formats=["xml", "json", "json_ietf"],
    oracle=True,
    serialize_defaults="all",
)
add_enabled("linkage-deviation-delete")

# ---------------------------------------------------------------------------
# 20. deviation multi
# ---------------------------------------------------------------------------
write_extra_modules(
    "linkage-deviation-multi",
    {
        "linkage-deviation-multi-base.yang": """module linkage-deviation-multi-base {
  yang-version 1.1;
  namespace "urn:linkage-deviation-multi-base";
  prefix ldmb;
  revision 2026-06-15;

  container c {
    leaf legacy {
      type string;
    }
    leaf maximum {
      type uint64;
    }
    leaf name {
      type string;
    }
  }
}""",
        "linkage-deviation-multi-dev.yang": """module linkage-deviation-multi-dev {
  yang-version 1.1;
  namespace "urn:linkage-deviation-multi-dev";
  prefix ldmd;

  import linkage-deviation-multi-base {
    prefix ldmb;
  }

  revision 2026-06-15;

  deviation "/ldmb:c/ldmb:legacy" {
    deviate not-supported;
  }

  deviation "/ldmb:c/ldmb:maximum" {
    deviate replace {
      type uint32 {
        range "1..1000";
      }
    }
  }

  deviation "/ldmb:c/ldmb:name" {
    deviate add {
      mandatory true;
    }
  }
}""",
    },
)
add_fixture(
    name="linkage-deviation-multi",
    theme="linkage",
    module="""module linkage-deviation-multi-base {
  yang-version 1.1;
  namespace "urn:linkage-deviation-multi-base";
  prefix ldmb;
  revision 2026-06-15;

  container c {
    leaf legacy {
      type string;
    }
    leaf maximum {
      type uint64;
    }
    leaf name {
      type string;
    }
  }
}""",
    module_name="linkage-deviation-multi-base",
    input="""<c xmlns="urn:linkage-deviation-multi-base">
  <maximum>500</maximum>
  <name>valid</name>
</c>
""",
    input_name="input.xml",
    input_format="xml",
    formats=["xml", "json", "json_ietf"],
    oracle=True,
)
add_enabled("linkage-deviation-multi")

# ---------------------------------------------------------------------------
# 21. deviation replace default + config + add must
# ---------------------------------------------------------------------------
write_extra_modules(
    "linkage-deviation-replace-default-config",
    {
        "linkage-deviation-replace-default-config-base.yang": """module linkage-deviation-replace-default-config-base {
  yang-version 1.1;
  namespace "urn:linkage-deviation-replace-default-config-base";
  prefix ldrdc;
  revision 2026-06-15;

  container system {
    presence "enable system";
    leaf mode {
      type string;
      default "disabled";
    }
    container ospf {
      leaf router-id {
        type string;
      }
    }
  }
}""",
        "linkage-deviation-replace-default-config-dev.yang": """module linkage-deviation-replace-default-config-dev {
  yang-version 1.1;
  namespace "urn:linkage-deviation-replace-default-config-dev";
  prefix ldrdd;

  import linkage-deviation-replace-default-config-base {
    prefix ldrdc;
  }

  revision 2026-06-15;

  deviation "/ldrdc:system/ldrdc:mode" {
    deviate replace {
      default "enabled";
    }
  }

  deviation "/ldrdc:system/ldrdc:ospf/ldrdc:router-id" {
    deviate replace {
      config false;
    }
  }

  deviation "/ldrdc:system/ldrdc:ospf" {
    deviate add {
      must "../ldrdc:mode = 'enabled'";
    }
  }
}""",
    },
)
add_fixture(
    name="linkage-deviation-replace-default-config",
    theme="linkage",
    module="""module linkage-deviation-replace-default-config-base {
  yang-version 1.1;
  namespace "urn:linkage-deviation-replace-default-config-base";
  prefix ldrdc;
  revision 2026-06-15;

  container system {
    presence "enable system";
    leaf mode {
      type string;
      default "disabled";
    }
    container ospf {
      leaf router-id {
        type string;
      }
    }
  }
}""",
    module_name="linkage-deviation-replace-default-config-base",
    input="""<system xmlns="urn:linkage-deviation-replace-default-config-base">
  <ospf>
    <router-id>1.1.1.1</router-id>
  </ospf>
  <mode>enabled</mode>
</system>
""",
    input_name="input.xml",
    input_format="xml",
    formats=["xml", "json", "json_ietf"],
    oracle=True,
    serialize_defaults="all",
)
add_enabled("linkage-deviation-replace-default-config")

# ---------------------------------------------------------------------------
# 22. import prefix
# ---------------------------------------------------------------------------
write_extra_modules(
    "linkage-import-prefix",
    {
        "linkage-import-prefix-base.yang": """module linkage-import-prefix-base {
  yang-version 1.1;
  namespace "urn:linkage-import-prefix-base";
  prefix lipb;
  revision 2026-06-15;

  typedef port-number {
    type uint16 {
      range "1..65535";
    }
  }
}""",
    },
)
add_fixture(
    name="linkage-import-prefix",
    theme="linkage",
    module="""module linkage-import-prefix {
  yang-version 1.1;
  namespace "urn:linkage-import-prefix";
  prefix lipp;

  import linkage-import-prefix-base {
    prefix remote;
  }

  revision 2026-06-15;

  leaf port {
    type remote:port-number;
  }
}""",
    module_name="linkage-import-prefix",
    input="""<port xmlns="urn:linkage-import-prefix">8080</port>
""",
    input_name="input.xml",
    input_format="xml",
    formats=["xml", "json", "json_ietf"],
    oracle=True,
)
add_enabled("linkage-import-prefix")

# ---------------------------------------------------------------------------
# 23. import revision-date
# ---------------------------------------------------------------------------
write_extra_modules(
    "linkage-import-revision-date",
    {
        "linkage-import-revision-date-base.yang": """module linkage-import-revision-date-base {
  yang-version 1.1;
  namespace "urn:linkage-import-revision-date-base";
  prefix lirdb;
  revision 2026-06-14 {
    description "Pinned revision for import-by-revision test.";
  }

  typedef old-field-type {
    type string;
  }

  container c {
    leaf old-field {
      type old-field-type;
    }
  }
}""",
    },
)
add_fixture(
    name="linkage-import-revision-date",
    theme="linkage",
    module="""module linkage-import-revision-date {
  yang-version 1.1;
  namespace "urn:linkage-import-revision-date";
  prefix lirdm;

  import linkage-import-revision-date-base {
    prefix lirdb;
    revision-date 2026-06-14;
  }

  revision 2026-06-15;

  leaf old-field {
    type lirdb:old-field-type;
  }
}""",
    module_name="linkage-import-revision-date",
    input="""<old-field xmlns="urn:linkage-import-revision-date">legacy-value</old-field>
""",
    input_name="input.xml",
    input_format="xml",
    formats=["xml", "json", "json_ietf"],
    oracle=True,
)
add_enabled("linkage-import-revision-date")

# ---------------------------------------------------------------------------
# 24. import multiple
# ---------------------------------------------------------------------------
write_extra_modules(
    "linkage-import-multiple",
    {
        "linkage-import-multiple-types1.yang": """module linkage-import-multiple-types1 {
  yang-version 1.1;
  namespace "urn:linkage-import-multiple-types1";
  prefix lim1;
  revision 2026-06-15;

  typedef id {
    type string;
  }
}""",
        "linkage-import-multiple-types2.yang": """module linkage-import-multiple-types2 {
  yang-version 1.1;
  namespace "urn:linkage-import-multiple-types2";
  prefix lim2;
  revision 2026-06-15;

  typedef name {
    type string;
  }
}""",
        "linkage-import-multiple-types3.yang": """module linkage-import-multiple-types3 {
  yang-version 1.1;
  namespace "urn:linkage-import-multiple-types3";
  prefix lim3;
  revision 2026-06-15;

  typedef mode {
    type string;
  }
}""",
    },
)
add_fixture(
    name="linkage-import-multiple",
    theme="linkage",
    module="""module linkage-import-multiple {
  yang-version 1.1;
  namespace "urn:linkage-import-multiple";
  prefix limm;

  import linkage-import-multiple-types1 {
    prefix m1;
  }
  import linkage-import-multiple-types2 {
    prefix m2;
  }
  import linkage-import-multiple-types3 {
    prefix m3;
  }

  revision 2026-06-15;

  container obj {
    leaf id {
      type m1:id;
    }
    leaf name {
      type m2:name;
    }
    leaf mode {
      type m3:mode;
    }
  }
}""",
    module_name="linkage-import-multiple",
    input="""<obj xmlns="urn:linkage-import-multiple">
  <mode>active</mode>
  <id>42</id>
  <name>test</name>
</obj>
""",
    input_name="input.xml",
    input_format="xml",
    formats=["xml", "json", "json_ietf"],
    oracle=True,
)
add_enabled("linkage-import-multiple")

# ---------------------------------------------------------------------------
# 25. import non-transitive (pure rejection, no manifest case)
# ---------------------------------------------------------------------------
write_extra_modules(
    "linkage-import-non-transitive",
    {
        "linkage-import-non-transitive-c.yang": """module linkage-import-non-transitive-c {
  yang-version 1.1;
  namespace "urn:linkage-import-non-transitive-c";
  prefix lintc;
  revision 2026-06-15;

  typedef color {
    type string;
  }
}""",
        "linkage-import-non-transitive-b.yang": """module linkage-import-non-transitive-b {
  yang-version 1.1;
  namespace "urn:linkage-import-non-transitive-b";
  prefix lintb;

  import linkage-import-non-transitive-c {
    prefix lintc;
  }

  revision 2026-06-15;

  leaf x {
    type lintc:color;
  }
}""",
        "linkage-import-non-transitive-a.yang": """module linkage-import-non-transitive-a {
  yang-version 1.1;
  namespace "urn:linkage-import-non-transitive-a";
  prefix linta;

  import linkage-import-non-transitive-b {
    prefix lintb;
  }

  revision 2026-06-15;

  leaf bad {
    type c:color;
  }
}""",
    },
)

# ---------------------------------------------------------------------------
# 26. submodule simple
# ---------------------------------------------------------------------------
write_extra_modules(
    "linkage-submodule-simple",
    {
        "linkage-submodule-simple-auth.yang": """submodule linkage-submodule-simple-auth {
  yang-version 1.1;
  belongs-to linkage-submodule-simple {
    prefix lss;
  }
  revision 2026-06-15;

  grouping auth-params {
    leaf username {
      type string;
    }
    leaf password {
      type string;
    }
  }
}""",
    },
)
add_fixture(
    name="linkage-submodule-simple",
    theme="linkage",
    module="""module linkage-submodule-simple {
  yang-version 1.1;
  namespace "urn:linkage-submodule-simple";
  prefix lss;

  include linkage-submodule-simple-auth;

  revision 2026-06-15;

  container aaa {
    uses auth-params;
  }
}""",
    module_name="linkage-submodule-simple",
    input="""<aaa xmlns="urn:linkage-submodule-simple">
  <password>hunter2</password>
  <username>admin</username>
</aaa>
""",
    input_name="input.xml",
    input_format="xml",
    formats=["xml", "json", "json_ietf"],
    oracle=True,
)
add_enabled("linkage-submodule-simple")

# ---------------------------------------------------------------------------
# 27. submodule multi
# ---------------------------------------------------------------------------
write_extra_modules(
    "linkage-submodule-multi",
    {
        "linkage-submodule-multi-types.yang": """submodule linkage-submodule-multi-types {
  yang-version 1.1;
  belongs-to linkage-submodule-multi {
    prefix lsmm;
  }
  revision 2026-06-15;

  typedef device-id {
    type string;
  }
}""",
        "linkage-submodule-multi-state.yang": """submodule linkage-submodule-multi-state {
  yang-version 1.1;
  belongs-to linkage-submodule-multi {
    prefix lsmm;
  }

  include linkage-submodule-multi-types;

  revision 2026-06-15;

  grouping device-state {
    leaf id {
      type device-id;
    }
    leaf online {
      type boolean;
    }
  }
}""",
    },
)
add_fixture(
    name="linkage-submodule-multi",
    theme="linkage",
    module="""module linkage-submodule-multi {
  yang-version 1.1;
  namespace "urn:linkage-submodule-multi";
  prefix lsmm;

  include linkage-submodule-multi-types;
  include linkage-submodule-multi-state;

  revision 2026-06-15;

  container device {
    uses device-state;
  }
}""",
    module_name="linkage-submodule-multi",
    input="""<device xmlns="urn:linkage-submodule-multi">
  <online>true</online>
  <id>dev-1</id>
</device>
""",
    input_name="input.xml",
    input_format="xml",
    formats=["xml", "json", "json_ietf"],
    oracle=True,
)
add_enabled("linkage-submodule-multi")

# ---------------------------------------------------------------------------
# 28. submodule imports foreign
# ---------------------------------------------------------------------------
write_extra_modules(
    "linkage-submodule-imports-foreign",
    {
        "owned-inet-types.yang": """module owned-inet-types {
  yang-version 1.1;
  namespace "urn:linkage-submodule-imports-foreign-inet";
  prefix inet;
  revision 2026-06-15;

  typedef ip-address {
    type string {
      pattern '[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}';
    }
  }
}""",
        "linkage-submodule-imports-foreign-part.yang": """submodule linkage-submodule-imports-foreign-part {
  yang-version 1.1;
  belongs-to linkage-submodule-imports-foreign {
    prefix lsif;
  }

  import owned-inet-types {
    prefix inet;
  }

  revision 2026-06-15;

  grouping radius-cfg {
    leaf server {
      type inet:ip-address;
    }
    leaf secret {
      type string;
    }
  }
}""",
    },
)
add_fixture(
    name="linkage-submodule-imports-foreign",
    theme="linkage",
    module="""module linkage-submodule-imports-foreign {
  yang-version 1.1;
  namespace "urn:linkage-submodule-imports-foreign";
  prefix lsif;

  include linkage-submodule-imports-foreign-part;

  revision 2026-06-15;

  container aaa {
    uses radius-cfg;
  }
}""",
    module_name="linkage-submodule-imports-foreign",
    input="""<aaa xmlns="urn:linkage-submodule-imports-foreign">
  <secret>shared</secret>
  <server>192.0.2.1</server>
</aaa>
""",
    input_name="input.xml",
    input_format="xml",
    formats=["xml", "json", "json_ietf"],
    oracle=True,
)
add_enabled("linkage-submodule-imports-foreign")

run_rust_runner()
run_go_runner()
