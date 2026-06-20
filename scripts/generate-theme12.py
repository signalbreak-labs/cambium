#!/usr/bin/env python3
"""Generate Theme 12: real-typedef-imports fixture (1)."""
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
from conformance_lib import add_fixture, add_enabled, run_rust_runner, run_go_runner

NAME = "rfc6991-inet-yang-types-roundtrip"

INET_MOD = """module owned-inet-types {
  yang-version 1.1;
  namespace "urn:cambium:owned-inet-types";
  prefix oinet;
  revision 2026-06-15 {
    description "Owned minimal subset of RFC 6991 ietf-inet-types.";
  }

  typedef ipv4-address {
    type string {
      pattern '[0-9]{1,3}\\.[0-9]{1,3}\\.[0-9]{1,3}\\.[0-9]{1,3}';
    }
  }

  typedef ipv6-address {
    type string {
      pattern '[0-9a-fA-F:]+';
    }
  }

  typedef ip-address {
    type union {
      type ipv4-address;
      type ipv6-address;
    }
  }

  typedef ipv6-prefix {
    type string {
      pattern '[0-9a-fA-F:]+/[0-9]+';
    }
  }
}
"""

YANG_MOD = """module owned-yang-types {
  yang-version 1.1;
  namespace "urn:cambium:owned-yang-types";
  prefix oyng;
  revision 2026-06-15 {
    description "Owned minimal subset of RFC 6991 ietf-yang-types.";
  }

  typedef mac-address {
    type string {
      pattern '[0-9a-fA-F]{2}(:[0-9a-fA-F]{2}){5}';
    }
  }

  typedef date-and-time {
    type string {
      pattern '\\d{4}-\\d{2}-\\d{2}T\\d{2}:\\d{2}:\\d{2}(Z|[+-]\\d{2}:\\d{2})';
    }
  }

  typedef counter64 {
    type uint64;
  }
}
"""

MAIN_MOD = """module rfc6991-inet-yang-types-roundtrip {
  yang-version 1.1;
  namespace "urn:rfc6991-inet-yang-types-roundtrip";
  prefix riyt;

  import owned-inet-types {
    prefix oinet;
  }

  import owned-yang-types {
    prefix oyng;
  }

  revision 2026-06-15;

  container top {
    leaf addr {
      type oinet:ip-address;
    }
    leaf pfx {
      type oinet:ipv6-prefix;
    }
    leaf mac {
      type oyng:mac-address;
    }
    leaf ts {
      type oyng:date-and-time;
    }
    leaf ctr {
      type oyng:counter64;
    }
  }
}
"""

INPUT = """<top xmlns="urn:rfc6991-inet-yang-types-roundtrip">
  <ctr>18446744073709551615</ctr>
  <mac>00:1b:44:11:3a:b7</mac>
  <addr>192.0.2.1</addr>
  <ts>2026-06-15T00:00:00Z</ts>
  <pfx>2001:db8::/32</pfx>
</top>
"""


def main():
    fixture_dir = Path(__file__).resolve().parent.parent / "conformance" / "fixtures" / NAME / "module"
    fixture_dir.mkdir(parents=True, exist_ok=True)
    (fixture_dir / "owned-inet-types.yang").write_text(INET_MOD)
    (fixture_dir / "owned-yang-types.yang").write_text(YANG_MOD)

    add_fixture(
        name=NAME,
        theme="real-typedef-imports",
        module=MAIN_MOD,
        module_name=NAME,
        input=INPUT,
        input_name="input.xml",
        input_format="xml",
        formats=["xml", "json", "json_ietf"],
        oracle=True,
        skip_existing=True,
    )
    add_enabled(NAME)

    run_rust_runner()
    run_go_runner()


if __name__ == "__main__":
    main()
