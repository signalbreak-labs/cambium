#!/usr/bin/env python3
"""Generate Theme 14: operations fixtures (18)."""
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
from conformance_lib import add_fixture, add_enabled, run_rust_runner, run_go_runner


# ---------------------------------------------------------------------------
# 1. rpc-input-only
# ---------------------------------------------------------------------------
add_fixture(
    name="rpc-input-only",
    theme="operations",
    module="""module operations-rpc-input-only {
  yang-version 1.1;
  namespace "urn:operations-rpc-input-only";
  prefix orio;
  revision 2026-06-15;

  rpc request-status {
    input {
      leaf interface-name {
        type string;
      }
      leaf check-type {
        type enumeration {
          enum arp;
          enum icmp;
        }
      }
      leaf timeout {
        type uint32;
      }
    }
  }
}""",
    module_name="operations-rpc-input-only",
    input="""<request-status xmlns="urn:operations-rpc-input-only">
  <check-type>icmp</check-type>
  <timeout>30</timeout>
  <interface-name>eth0</interface-name>
</request-status>
""",
    input_name="input.xml",
    input_format="xml",
    formats=["xml", "json", "json_ietf"],
    oracle=True,
    op_type="rpc",
)
add_enabled("rpc-input-only")

# ---------------------------------------------------------------------------
# 2. rpc-output-only
# ---------------------------------------------------------------------------
add_fixture(
    name="rpc-output-only",
    theme="operations",
    module="""module operations-rpc-output-only {
  yang-version 1.1;
  namespace "urn:operations-rpc-output-only";
  prefix oroo;
  revision 2026-06-15;

  rpc get-version {
    output {
      leaf version-string {
        type string;
      }
      leaf build-date {
        type string;
      }
      leaf revision-id {
        type string;
      }
    }
  }
}""",
    module_name="operations-rpc-output-only",
    input="""<get-version xmlns="urn:operations-rpc-output-only">
  <revision-id>r42</revision-id>
  <build-date>2026-06-15</build-date>
  <version-string>1.2.3</version-string>
</get-version>
""",
    input_name="input.xml",
    input_format="xml",
    formats=["xml", "json", "json_ietf"],
    oracle=True,
    op_type="reply",
)
add_enabled("rpc-output-only")

# ---------------------------------------------------------------------------
# 3. rpc-input-output-interleaved
# ---------------------------------------------------------------------------
add_fixture(
    name="rpc-input-output-interleaved",
    theme="operations",
    module="""module operations-rpc-input-output-interleaved {
  yang-version 1.1;
  namespace "urn:operations-rpc-input-output-interleaved";
  prefix oriioi;
  revision 2026-06-15;

  rpc configure-interface {
    input {
      leaf name {
        type string;
      }
      leaf mode {
        type enumeration {
          enum auto;
          enum manual;
        }
      }
      leaf mtu {
        type uint32;
      }
    }
    output {
      leaf status {
        type enumeration {
          enum ok;
          enum fail;
        }
      }
      leaf error-message {
        type string;
      }
      container stats {
        leaf packets-sent {
          type uint64;
        }
      }
    }
  }
}""",
    module_name="operations-rpc-input-output-interleaved",
    input="""<configure-interface xmlns="urn:operations-rpc-input-output-interleaved">
  <mtu>1500</mtu>
  <name>eth0</name>
  <mode>auto</mode>
</configure-interface>
""",
    input_name="input.xml",
    input_format="xml",
    formats=["xml", "json", "json_ietf"],
    oracle=True,
    op_type="rpc",
)
add_enabled("rpc-input-output-interleaved")

# ---------------------------------------------------------------------------
# 4. rpc-io-heterogeneous-nodes
# ---------------------------------------------------------------------------
add_fixture(
    name="rpc-io-heterogeneous-nodes",
    theme="operations",
    module="""module operations-rpc-io-heterogeneous-nodes {
  yang-version 1.1;
  namespace "urn:operations-rpc-io-heterogeneous-nodes";
  prefix oriohn;
  revision 2026-06-15;

  rpc query-metrics {
    input {
      leaf query-id {
        type string;
      }
      leaf-list metrics {
        type string;
      }
      container filter {
        leaf pattern {
          type string;
        }
      }
      choice scope {
        case node {
          leaf node-id {
            type string;
          }
        }
        case network {
          leaf network-id {
            type string;
          }
        }
      }
    }
    output {
      container results {
        leaf count {
          type uint32;
        }
        leaf-list values {
          type string;
        }
      }
      leaf response-time {
        type decimal64 {
          fraction-digits 3;
        }
      }
      leaf-list warnings {
        type string;
      }
    }
  }
}""",
    module_name="operations-rpc-io-heterogeneous-nodes",
    input="""<query-metrics xmlns="urn:operations-rpc-io-heterogeneous-nodes">
  <node-id>node-1</node-id>
  <metrics>cpu</metrics>
  <metrics>memory</metrics>
  <query-id>q1</query-id>
  <filter>
    <pattern>.*</pattern>
  </filter>
</query-metrics>
""",
    input_name="input.xml",
    input_format="xml",
    formats=["xml", "json", "json_ietf"],
    oracle=True,
    op_type="rpc",
)
add_enabled("rpc-io-heterogeneous-nodes")

# ---------------------------------------------------------------------------
# 5. rpc-io-nested-containers
# ---------------------------------------------------------------------------
add_fixture(
    name="rpc-io-nested-containers",
    theme="operations",
    module="""module operations-rpc-io-nested-containers {
  yang-version 1.1;
  namespace "urn:operations-rpc-io-nested-containers";
  prefix orionc;
  revision 2026-06-15;

  rpc deploy-service {
    input {
      leaf service-name {
        type string;
      }
      container config {
        leaf replica-count {
          type uint16;
        }
        container resource-limits {
          leaf cpu {
            type string;
          }
          leaf memory {
            type string;
          }
        }
        leaf timeout {
          type uint32;
        }
      }
    }
    output {
      container deployment-info {
        leaf status {
          type string;
        }
        leaf message {
          type string;
        }
      }
    }
  }
}""",
    module_name="operations-rpc-io-nested-containers",
    input="""<deploy-service xmlns="urn:operations-rpc-io-nested-containers">
  <config>
    <timeout>30</timeout>
    <resource-limits>
      <memory>512M</memory>
      <cpu>100m</cpu>
    </resource-limits>
    <replica-count>3</replica-count>
  </config>
  <service-name>svc</service-name>
</deploy-service>
""",
    input_name="input.xml",
    input_format="xml",
    formats=["xml", "json", "json_ietf"],
    oracle=True,
    op_type="rpc",
)
add_enabled("rpc-io-nested-containers")

# ---------------------------------------------------------------------------
# 6. rpc-io-with-anyxml
# ---------------------------------------------------------------------------
add_fixture(
    name="rpc-io-with-anyxml",
    theme="operations",
    module="""module operations-rpc-io-with-anyxml {
  yang-version 1.1;
  namespace "urn:operations-rpc-io-with-anyxml";
  prefix orioax;
  revision 2026-06-15;

  rpc execute-script {
    input {
      leaf script-name {
        type string;
      }
      anyxml parameters;
      leaf timeout {
        type uint32;
      }
    }
    output {
      anyxml result;
      leaf completion-time {
        type string;
      }
    }
  }
}""",
    module_name="operations-rpc-io-with-anyxml",
    input="""<execute-script xmlns="urn:operations-rpc-io-with-anyxml">
  <timeout>60</timeout>
  <parameters><args><arg>a</arg><arg>b</arg></args></parameters>
  <script-name>backup.sh</script-name>
</execute-script>
""",
    input_name="input.xml",
    input_format="xml",
    formats=["xml"],
    oracle=True,
    op_type="rpc",
)
add_enabled("rpc-io-with-anyxml")

# ---------------------------------------------------------------------------
# 7. rpc-io-decimal64-numeric-types
# ---------------------------------------------------------------------------
add_fixture(
    name="rpc-io-decimal64-numeric-types",
    theme="operations",
    module="""module operations-rpc-io-decimal64-numeric-types {
  yang-version 1.1;
  namespace "urn:operations-rpc-io-decimal64-numeric-types";
  prefix oriodnt;
  revision 2026-06-15;

  rpc compute-aggregate {
    input {
      leaf samples-count {
        type uint64;
      }
      leaf offset {
        type int64;
      }
      leaf scale-factor {
        type decimal64 {
          fraction-digits 2;
        }
      }
    }
    output {
      leaf sum {
        type int64;
      }
      leaf average {
        type decimal64 {
          fraction-digits 3;
        }
      }
      leaf max-value {
        type uint64;
      }
    }
  }
}""",
    module_name="operations-rpc-io-decimal64-numeric-types",
    input="""<compute-aggregate xmlns="urn:operations-rpc-io-decimal64-numeric-types">
  <scale-factor>1.50</scale-factor>
  <samples-count>10</samples-count>
  <offset>-5</offset>
</compute-aggregate>
""",
    input_name="input.xml",
    input_format="xml",
    formats=["xml", "json", "json_ietf"],
    oracle=True,
    op_type="rpc",
)
add_enabled("rpc-io-decimal64-numeric-types")

# ---------------------------------------------------------------------------
# 8. action-container-simple
# ---------------------------------------------------------------------------
add_fixture(
    name="action-container-simple",
    theme="operations",
    module="""module operations-action-container-simple {
  yang-version 1.1;
  namespace "urn:operations-action-container-simple";
  prefix oroacs;
  revision 2026-06-15;

  container device {
    leaf name {
      type string;
    }
    action restart {
      input {
        leaf delay-seconds {
          type uint32;
        }
        leaf force {
          type boolean;
        }
      }
      output {
        leaf status {
          type enumeration {
            enum ok;
            enum fail;
          }
        }
        leaf message {
          type string;
        }
      }
    }
  }
}""",
    module_name="operations-action-container-simple",
    input="""<action xmlns="urn:ietf:params:xml:ns:yang:1">
  <device xmlns="urn:operations-action-container-simple">
    <name>d1</name>
    <restart>
      <force>true</force>
      <delay-seconds>5</delay-seconds>
    </restart>
  </device>
</action>
""",
    input_name="input.xml",
    input_format="xml",
    formats=["xml", "json", "json_ietf"],
    oracle=True,
    op_type="rpc",
)
add_enabled("action-container-simple")

# ---------------------------------------------------------------------------
# 9. action-list-keys-context
# ---------------------------------------------------------------------------
add_fixture(
    name="action-list-keys-context",
    theme="operations",
    module="""module operations-action-list-keys-context {
  yang-version 1.1;
  namespace "urn:operations-action-list-keys-context";
  prefix oroalkc;
  revision 2026-06-15;

  list service {
    key "name";
    leaf name {
      type string;
    }
    leaf enabled {
      type boolean;
    }
    action reset {
      input {
        leaf reset-type {
          type enumeration {
            enum soft;
            enum hard;
          }
        }
        leaf preserve-state {
          type boolean;
        }
      }
      output {
        leaf result {
          type enumeration {
            enum success;
            enum failure;
          }
        }
        leaf details {
          type string;
        }
      }
    }
  }
}""",
    module_name="operations-action-list-keys-context",
    input="""<action xmlns="urn:ietf:params:xml:ns:yang:1">
  <service xmlns="urn:operations-action-list-keys-context">
    <name>svc1</name>
    <reset>
      <preserve-state>true</preserve-state>
      <reset-type>soft</reset-type>
    </reset>
  </service>
</action>
""",
    input_name="input.xml",
    input_format="xml",
    formats=["xml", "json", "json_ietf"],
    oracle=True,
    op_type="rpc",
)
add_enabled("action-list-keys-context")

# ---------------------------------------------------------------------------
# 10. action-nested-containers
# ---------------------------------------------------------------------------
add_fixture(
    name="action-nested-containers",
    theme="operations",
    module="""module operations-action-nested-containers {
  yang-version 1.1;
  namespace "urn:operations-action-nested-containers";
  prefix oroanc;
  revision 2026-06-15;

  container system {
    container management {
      leaf enabled {
        type boolean;
      }
      action audit-log {
        input {
          leaf duration-hours {
            type uint16;
          }
          leaf format {
            type enumeration {
              enum json;
              enum csv;
            }
          }
        }
        output {
          leaf entries-count {
            type uint64;
          }
          leaf log-file {
            type string;
          }
        }
      }
      leaf retention-days {
        type uint16;
      }
    }
  }
}""",
    module_name="operations-action-nested-containers",
    input="""<action xmlns="urn:ietf:params:xml:ns:yang:1">
  <system xmlns="urn:operations-action-nested-containers">
    <management>
      <enabled>true</enabled>
      <audit-log>
        <format>json</format>
        <duration-hours>24</duration-hours>
      </audit-log>
      <retention-days>30</retention-days>
    </management>
  </system>
</action>
""",
    input_name="input.xml",
    input_format="xml",
    formats=["xml", "json", "json_ietf"],
    oracle=True,
    op_type="rpc",
)
add_enabled("action-nested-containers")

# ---------------------------------------------------------------------------
# 11. action-io-heterogeneous
# ---------------------------------------------------------------------------
add_fixture(
    name="action-io-heterogeneous",
    theme="operations",
    module="""module operations-action-io-heterogeneous {
  yang-version 1.1;
  namespace "urn:operations-action-io-heterogeneous";
  prefix oroaih;
  revision 2026-06-15;

  container interface {
    action diagnostic {
      input {
        leaf test-type {
          type string;
        }
        leaf-list parameters {
          type string;
        }
        container config {
          leaf timeout {
            type uint16;
          }
        }
      }
      output {
        container results {
          leaf-list tests-passed {
            type string;
          }
          leaf summary {
            type string;
          }
        }
        leaf duration {
          type decimal64 {
            fraction-digits 2;
          }
        }
      }
    }
  }
}""",
    module_name="operations-action-io-heterogeneous",
    input="""<action xmlns="urn:ietf:params:xml:ns:yang:1">
  <interface xmlns="urn:operations-action-io-heterogeneous">
    <diagnostic>
      <config>
        <timeout>30</timeout>
      </config>
      <parameters>p1</parameters>
      <parameters>p2</parameters>
      <test-type>ping</test-type>
    </diagnostic>
  </interface>
</action>
""",
    input_name="input.xml",
    input_format="xml",
    formats=["xml", "json", "json_ietf"],
    oracle=True,
    op_type="rpc",
)
add_enabled("action-io-heterogeneous")

# ---------------------------------------------------------------------------
# 12. action-container-wide-siblings
# ---------------------------------------------------------------------------
add_fixture(
    name="action-container-wide-siblings",
    theme="operations",
    module="""module operations-action-container-wide-siblings {
  yang-version 1.1;
  namespace "urn:operations-action-container-wide-siblings";
  prefix oroacws;
  revision 2026-06-15;

  container config {
    leaf setting-a {
      type string;
    }
    leaf setting-b {
      type string;
    }
    action validate {
      input {
        leaf strict-mode {
          type boolean;
        }
      }
      output {
        leaf is-valid {
          type boolean;
        }
        leaf errors {
          type string;
        }
      }
    }
    leaf setting-c {
      type string;
    }
    leaf setting-d {
      type string;
    }
  }
}""",
    module_name="operations-action-container-wide-siblings",
    input="""<action xmlns="urn:ietf:params:xml:ns:yang:1">
  <config xmlns="urn:operations-action-container-wide-siblings">
    <setting-b>b</setting-b>
    <setting-d>d</setting-d>
    <validate>
      <strict-mode>true</strict-mode>
    </validate>
    <setting-a>a</setting-a>
    <setting-c>c</setting-c>
  </config>
</action>
""",
    input_name="input.xml",
    input_format="xml",
    formats=["xml", "json", "json_ietf"],
    oracle=True,
    op_type="rpc",
)
add_enabled("action-container-wide-siblings")

# ---------------------------------------------------------------------------
# 13. notification-top-level
# ---------------------------------------------------------------------------
add_fixture(
    name="notification-top-level",
    theme="operations",
    module="""module operations-notification-top-level {
  yang-version 1.1;
  namespace "urn:operations-notification-top-level";
  prefix orontl;
  revision 2026-06-15;

  notification link-state-change {
    leaf timestamp {
      type string;
    }
    leaf interface-name {
      type string;
    }
    leaf link-state {
      type enumeration {
        enum up;
        enum down;
      }
    }
    leaf reason {
      type string;
    }
  }
}""",
    module_name="operations-notification-top-level",
    input="""<link-state-change xmlns="urn:operations-notification-top-level">
  <reason>port-up</reason>
  <link-state>up</link-state>
  <interface-name>eth0</interface-name>
  <timestamp>2026-06-15T12:00:00Z</timestamp>
</link-state-change>
""",
    input_name="input.xml",
    input_format="xml",
    formats=["xml", "json", "json_ietf"],
    oracle=True,
    op_type="notification",
)
add_enabled("notification-top-level")

# ---------------------------------------------------------------------------
# 14. notification-nested-container
# ---------------------------------------------------------------------------
add_fixture(
    name="notification-nested-container",
    theme="operations",
    module="""module operations-notification-nested-container {
  yang-version 1.1;
  namespace "urn:operations-notification-nested-container";
  prefix oronnc;
  revision 2026-06-15;

  container system {
    leaf hostname {
      type string;
    }
    notification error-alarm {
      leaf severity {
        type enumeration {
          enum critical;
          enum major;
          enum minor;
        }
      }
      leaf error-code {
        type uint32;
      }
      leaf description {
        type string;
      }
      leaf timestamp {
        type string;
      }
    }
  }
}""",
    module_name="operations-notification-nested-container",
    input="""<system xmlns="urn:operations-notification-nested-container">
  <error-alarm>
    <timestamp>2026-06-15T12:00:00Z</timestamp>
    <description>overheating</description>
    <severity>critical</severity>
    <error-code>42</error-code>
  </error-alarm>
  <hostname>core-1</hostname>
</system>
""",
    input_name="input.xml",
    input_format="xml",
    formats=["xml", "json", "json_ietf"],
    oracle=True,
    op_type="notification",
)
add_enabled("notification-nested-container")

# ---------------------------------------------------------------------------
# 15. notification-nested-list
# ---------------------------------------------------------------------------
add_fixture(
    name="notification-nested-list",
    theme="operations",
    module="""module operations-notification-nested-list {
  yang-version 1.1;
  namespace "urn:operations-notification-nested-list";
  prefix oronnl;
  revision 2026-06-15;

  list interface {
    key "name";
    leaf name {
      type string;
    }
    leaf mtu {
      type uint32;
    }
    notification mtu-exceeded {
      leaf packet-size {
        type uint32;
      }
      leaf action-taken {
        type enumeration {
          enum drop;
          enum fragment;
        }
      }
    }
  }
}""",
    module_name="operations-notification-nested-list",
    input="""<interface xmlns="urn:operations-notification-nested-list">
  <mtu>1500</mtu>
  <name>eth0</name>
  <mtu-exceeded>
    <action-taken>drop</action-taken>
    <packet-size>2000</packet-size>
  </mtu-exceeded>
</interface>
""",
    input_name="input.xml",
    input_format="xml",
    formats=["xml", "json", "json_ietf"],
    oracle=True,
    op_type="notification",
)
add_enabled("notification-nested-list")

# ---------------------------------------------------------------------------
# 16. notification-with-container-leaflist
# ---------------------------------------------------------------------------
add_fixture(
    name="notification-with-container-leaflist",
    theme="operations",
    module="""module operations-notification-with-container-leaflist {
  yang-version 1.1;
  namespace "urn:operations-notification-with-container-leaflist";
  prefix oronwcl;
  revision 2026-06-15;

  notification backup-complete {
    leaf backup-id {
      type string;
    }
    container summary {
      leaf duration {
        type string;
      }
      leaf files-backed {
        type uint64;
      }
      leaf status {
        type enumeration {
          enum success;
          enum failure;
        }
      }
    }
    leaf timestamp {
      type string;
    }
  }

  notification log-rotation {
    leaf rotation-time {
      type string;
    }
    leaf-list archived-files {
      type string;
    }
    leaf total-size {
      type uint64;
    }
  }
}""",
    module_name="operations-notification-with-container-leaflist",
    input="""<backup-complete xmlns="urn:operations-notification-with-container-leaflist">
  <timestamp>2026-06-15T12:00:00Z</timestamp>
  <backup-id>bk-1</backup-id>
  <summary>
    <status>success</status>
    <files-backed>42</files-backed>
    <duration>PT5M</duration>
  </summary>
</backup-complete>
""",
    input_name="input.xml",
    input_format="xml",
    formats=["xml", "json", "json_ietf"],
    oracle=True,
    op_type="notification",
)
add_enabled("notification-with-container-leaflist")

# ---------------------------------------------------------------------------
# 17. notification-interleaved-siblings
# ---------------------------------------------------------------------------
add_fixture(
    name="notification-interleaved-siblings",
    theme="operations",
    module="""module operations-notification-interleaved-siblings {
  yang-version 1.1;
  namespace "urn:operations-notification-interleaved-siblings";
  prefix oronis;
  revision 2026-06-15;

  container alarms {
    leaf count {
      type uint32;
    }
    notification raised {
      leaf severity {
        type string;
      }
      leaf resource {
        type string;
      }
    }
    leaf threshold {
      type uint32;
    }
    container summary {
      leaf last {
        type string;
      }
    }
  }
}""",
    module_name="operations-notification-interleaved-siblings",
    input="""<alarms xmlns="urn:operations-notification-interleaved-siblings">
  <summary>
    <last>2026-06-15T12:00:00Z</last>
  </summary>
  <count>5</count>
  <threshold>10</threshold>
</alarms>
""",
    input_name="input.xml",
    input_format="xml",
    formats=["xml", "json", "json_ietf"],
    oracle=True,
)
add_enabled("notification-interleaved-siblings")

# ---------------------------------------------------------------------------
# 18. rpc-action-notification-coexistence
# ---------------------------------------------------------------------------
add_fixture(
    name="rpc-action-notification-coexistence",
    theme="operations",
    module="""module operations-rpc-action-notification-coexistence {
  yang-version 1.1;
  namespace "urn:operations-rpc-action-notification-coexistence";
  prefix ororanc;
  revision 2026-06-15;

  rpc global-operation {
    input {
      leaf param-a {
        type string;
      }
      leaf param-b {
        type string;
      }
    }
    output {
      leaf result {
        type string;
      }
    }
  }

  container device {
    leaf id {
      type string;
    }
    action device-action {
      input {
        leaf value {
          type uint32;
        }
      }
      output {
        leaf ack {
          type boolean;
        }
      }
    }
    notification device-event {
      leaf event-type {
        type string;
      }
      leaf timestamp {
        type string;
      }
    }
  }
}""",
    module_name="operations-rpc-action-notification-coexistence",
    input="""<global-operation xmlns="urn:operations-rpc-action-notification-coexistence">
  <param-b>b</param-b>
  <param-a>a</param-a>
</global-operation>
""",
    input_name="input.xml",
    input_format="xml",
    formats=["xml", "json", "json_ietf"],
    oracle=True,
    op_type="rpc",
)
add_enabled("rpc-action-notification-coexistence")

run_rust_runner()
run_go_runner()
