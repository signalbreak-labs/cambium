#!/usr/bin/env python3
"""Generate Theme 11: extensions-metadata fixtures (7)."""
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
from conformance_lib import add_fixture, add_enabled, run_rust_runner, run_go_runner

FIXTURES = [
    (
        "metadata-yang-version-units",
        """module metadata-yang-version-units {
  yang-version 1.1;
  namespace "urn:metadata-yang-version-units";
  prefix myvu;
  organization "Cambium Conformance";
  contact "conformance@example.com";
  revision 2026-06-15 {
    description "Metadata round-trip fixture";
    reference "RFC 7950";
  }

  container performance {
    leaf cpu-usage {
      type decimal64 { fraction-digits 2; }
      units "percent";
      status current;
    }
    leaf memory-free {
      type uint64;
      units "bytes";
      status deprecated;
    }
    action reset-metrics {
      input { leaf force { type boolean; } }
      output { leaf result { type string; } }
    }
  }
}""",
        """<performance xmlns="urn:metadata-yang-version-units">
  <cpu-usage>45.50</cpu-usage>
  <memory-free>8589934592</memory-free>
</performance>
""",
    ),
    (
        "extension-definition-and-usage",
        """module extension-definition-and-usage {
  yang-version 1.1;
  namespace "urn:extension-definition-and-usage";
  prefix edu;
  revision 2026-06-15;

  extension metadata {
    argument key {
      yin-element true;
    }
  }

  container system {
    edu:metadata "version";
    leaf hostname { type string; }
    edu:metadata "owner";
    leaf domain { type string; }
  }
}""",
        """<system xmlns="urn:extension-definition-and-usage">
  <hostname>r1</hostname>
  <domain>example.com</domain>
</system>
""",
    ),
    (
        "extension-yin-element-modes",
        """module extension-yin-element-modes {
  yang-version 1.1;
  namespace "urn:extension-yin-element-modes";
  prefix eyem;
  revision 2026-06-15;

  extension attr-form {
    argument name {
      yin-element false;
    }
  }

  extension elem-form {
    argument body {
      yin-element true;
    }
  }

  container top {
    leaf x {
      eyem:attr-form "compact";
      eyem:elem-form "expanded";
      type string;
    }
  }
}""",
        """<top xmlns="urn:extension-yin-element-modes">
  <x>value</x>
</top>
""",
    ),
]


def main():
    for name, module, inp in FIXTURES:
        add_fixture(
            name=name,
            theme="extensions-metadata",
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

    # vendor extension pass-through (owned junos-style extension module)
    name = "vendor-extension-junos-passthrough"
    module_dir = Path(__file__).resolve().parent.parent / "conformance" / "fixtures" / name / "module"
    module_dir.mkdir(parents=True, exist_ok=True)
    (module_dir / "junos-extensions.yang").write_text("""module junos-extensions {
  yang-version 1.1;
  namespace "urn:example:junos-extensions";
  prefix jx;
  revision 2026-06-15;

  extension annotate {
    argument path;
  }

  extension secret;
}""")
    main_module = """module vendor-extension-junos-passthrough {
  yang-version 1.1;
  namespace "urn:vendor-extension-junos-passthrough";
  prefix vejp;

  import junos-extensions {
    prefix jx;
  }

  revision 2026-06-15;

  container config {
    jx:annotate "/vejp:config/users";
    container users {
      list user {
        key name;
        leaf name { type string; }
        leaf password {
          jx:secret;
          type string;
        }
      }
    }
  }
}"""
    inp = """<config xmlns="urn:vendor-extension-junos-passthrough">
  <users>
    <user>
      <name>admin</name>
      <password>hunter2</password>
    </user>
  </users>
</config>
"""
    add_fixture(
        name=name,
        theme="extensions-metadata",
        module=main_module,
        module_name=name,
        input=inp,
        input_name="input.xml",
        input_format="xml",
        formats=["xml", "json", "json_ietf"],
        oracle=True,
        skip_existing=True,
    )
    add_enabled(name)

    # extension/typedef collision (two modules)
    name = "extension-and-typedef-collision"
    module_dir = Path(__file__).resolve().parent.parent / "conformance" / "fixtures" / name / "module"
    module_dir.mkdir(parents=True, exist_ok=True)
    (module_dir / "ext-mod.yang").write_text("""module ext-mod {
  yang-version 1.1;
  namespace "urn:ext-mod";
  prefix em;
  revision 2026-06-15;

  extension Metadata {
    argument key {
      yin-element true;
    }
  }
}""")
    main_module = """module extension-and-typedef-collision {
  yang-version 1.1;
  namespace "urn:extension-and-typedef-collision";
  prefix eatc;

  import ext-mod {
    prefix ext;
  }

  revision 2026-06-15;

  typedef Metadata {
    type string {
      length "1..255";
    }
  }

  container top {
    ext:Metadata "version";
    leaf meta { type Metadata; }
  }
}"""
    inp = """<top xmlns="urn:extension-and-typedef-collision">
  <meta>data</meta>
</top>
"""
    add_fixture(
        name=name,
        theme="extensions-metadata",
        module=main_module,
        module_name=name,
        input=inp,
        input_name="input.xml",
        input_format="xml",
        formats=["xml", "json", "json_ietf"],
        oracle=True,
        skip_existing=True,
    )
    add_enabled(name)

    # RFC 7952 metadata annotation
    name = "metadata-annotation-rfc7952"
    module_dir = Path(__file__).resolve().parent.parent / "conformance" / "fixtures" / name / "module"
    module_dir.mkdir(parents=True, exist_ok=True)
    (module_dir / "ietf-yang-metadata.yang").write_text("""module ietf-yang-metadata {
  yang-version 1.1;
  namespace "urn:ietf:params:xml:ns:yang:ietf-yang-metadata";
  prefix md;
  revision 2016-08-05 {
    description "Initial revision";
    reference "RFC 7952";
  }

  extension annotation {
    argument name {
      yin-element true;
    }
  }
}""")
    main_module = """module metadata-annotation-rfc7952 {
  yang-version 1.1;
  namespace "urn:metadata-annotation-rfc7952";
  prefix mar;

  import ietf-yang-metadata {
    prefix md;
  }

  revision 2026-06-15;

  md:annotation last-modified {
    type string;
  }

  container top {
    leaf name { type string; }
  }
}"""
    inp = """<top xmlns="urn:metadata-annotation-rfc7952" xmlns:mar="urn:metadata-annotation-rfc7952">
  <name mar:last-modified="2026-06-15T00:00:00Z">primary</name>
</top>
"""
    add_fixture(
        name=name,
        theme="extensions-metadata",
        module=main_module,
        module_name=name,
        input=inp,
        input_name="input.xml",
        input_format="xml",
        formats=["xml", "json", "json_ietf"],
        oracle=True,
        skip_existing=True,
    )
    add_enabled(name)

    run_rust_runner()
    run_go_runner()


if __name__ == "__main__":
    main()
