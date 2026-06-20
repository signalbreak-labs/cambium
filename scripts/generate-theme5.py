#!/usr/bin/env python3
"""Batch-generate theme 5: identity fixtures."""
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).parent))
from conformance_lib import (
    CONFORMANCE,
    add_case,
    add_enabled,
    add_fixture,
    case_exists,
    write_goldens,
)

THEME = "identity"


def add_two_module(name: str, base_name: str, base_module: str, derived_name: str,
                   derived_module: str, input_xml: str):
    if case_exists(name):
        print(f"Skipping existing {name}")
        return
    fixture_dir = CONFORMANCE / "fixtures" / name
    module_dir = fixture_dir / "module"
    module_dir.mkdir(parents=True, exist_ok=True)
    (module_dir / f"{base_name}.yang").write_text(base_module)
    (module_dir / f"{derived_name}.yang").write_text(derived_module)
    input_path = fixture_dir / "input.xml"
    input_path.write_text(input_xml)
    add_case(name, f"fixtures/{name}/module", f"fixtures/{name}/input.xml",
             "xml", ["xml", "json", "json_ietf"], True)
    print(f"Generating goldens for {name} ({THEME})...")
    write_goldens(name)
    add_enabled(name)


def add_three_module(name: str, modules: list, input_xml: str):
    if case_exists(name):
        print(f"Skipping existing {name}")
        return
    fixture_dir = CONFORMANCE / "fixtures" / name
    module_dir = fixture_dir / "module"
    module_dir.mkdir(parents=True, exist_ok=True)
    for fname, content in modules:
        (module_dir / fname).write_text(content)
    input_path = fixture_dir / "input.xml"
    input_path.write_text(input_xml)
    add_case(name, f"fixtures/{name}/module", f"fixtures/{name}/input.xml",
             "xml", ["xml", "json", "json_ietf"], True)
    print(f"Generating goldens for {name} ({THEME})...")
    write_goldens(name)
    add_enabled(name)


def main():
    # 1. standalone identity (no base)
    add_fixture(
        name="identity-standalone",
        theme=THEME,
        module_name="identity-standalone",
        module="""module identity-standalone {
  yang-version 1;
  namespace "urn:identity-standalone";
  prefix is;
  revision 2026-06-14;

  identity hardware-component {
    description "Any hardware component";
  }
  identity chassis { base hardware-component; }
  container inventory {
    leaf component-type {
      type identityref { base hardware-component; }
    }
  }
}
""",
        input_name="input.xml",
        input_format="xml",
        formats=["xml", "json", "json_ietf"],
        oracle=True,
        input="""<inventory xmlns="urn:identity-standalone">
  <component-type>chassis</component-type>
</inventory>
""",
    )

    # 2. cross-module derivation
    add_two_module(
        "identity-cross-module-derivation",
        "identity-cross-module-derivation-base",
        """module identity-cross-module-derivation-base {
  yang-version 1;
  namespace "urn:identity-cross-module-derivation-base";
  prefix icmdb;
  revision 2026-06-14;

  identity transport-protocol;
}
""",
        "identity-cross-module-derivation",
        """module identity-cross-module-derivation {
  yang-version 1;
  namespace "urn:identity-cross-module-derivation";
  prefix icmd;
  import identity-cross-module-derivation-base { prefix base; }
  revision 2026-06-14;

  identity tcp { base base:transport-protocol; }
  identity udp { base base:transport-protocol; }
  leaf proto { type identityref { base base:transport-protocol; } }
}
""",
        """<proto xmlns="urn:identity-cross-module-derivation">tcp</proto>
""",
    )

    # 3. multi-base cross-module (YANG 1.1)
    add_three_module(
        "identity-multi-base-cross-module",
        [
            ("identity-multi-base-cross-module-a.yang", """module identity-multi-base-cross-module-a {
  yang-version 1.1;
  namespace "urn:identity-multi-base-cross-module-a";
  prefix imbcma;
  revision 2026-06-14;

  identity interface-role;
}
"""),
            ("identity-multi-base-cross-module-b.yang", """module identity-multi-base-cross-module-b {
  yang-version 1.1;
  namespace "urn:identity-multi-base-cross-module-b";
  prefix imbcmb;
  revision 2026-06-14;

  identity hardware-feature;
}
"""),
            ("identity-multi-base-cross-module.yang", """module identity-multi-base-cross-module {
  yang-version 1.1;
  namespace "urn:identity-multi-base-cross-module";
  prefix imbc;
  import identity-multi-base-cross-module-a { prefix a; }
  import identity-multi-base-cross-module-b { prefix b; }
  revision 2026-06-14;

  identity high-speed-interface {
    base a:interface-role;
    base b:hardware-feature;
  }
  leaf type { type identityref { base a:interface-role; } }
}
"""),
        ],
        """<type xmlns="urn:identity-multi-base-cross-module">high-speed-interface</type>
""",
    )

    # 4. hierarchy with identityref base filtering
    add_fixture(
        name="identity-hierarchy-with-identityref",
        theme=THEME,
        module_name="identity-hierarchy-with-identityref",
        module="""module identity-hierarchy-with-identityref {
  yang-version 1;
  namespace "urn:identity-hierarchy-with-identityref";
  prefix ihi;
  revision 2026-06-14;

  identity tunnel-type;
  identity mpls-tunnel { base tunnel-type; }
  identity rsvp-tunnel { base mpls-tunnel; }
  identity ldp-tunnel { base mpls-tunnel; }
  leaf primary { type identityref { base mpls-tunnel; } }
  leaf backup { type identityref { base tunnel-type; } }
}
""",
        input_name="input.xml",
        input_format="xml",
        formats=["xml", "json", "json_ietf"],
        oracle=True,
        input="""<primary xmlns="urn:identity-hierarchy-with-identityref">rsvp-tunnel</primary>
<backup xmlns="urn:identity-hierarchy-with-identityref">rsvp-tunnel</backup>
""",
    )

    print("Theme 5 fixtures generated.")


if __name__ == "__main__":
    main()
