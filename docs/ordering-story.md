# The ordering story

Cambium exists because order is not a cosmetic detail in YANG. RFC 7950 says
container children must appear in schema declaration order, and many NETCONF
device implementations expect that order on the wire. The openconfig/goyang
family alphabetizes children because it stores them in a Go map, so a model
with leaves `z`, `m`, `a` can come out as `a`, `m`, `z`.

Cambium fixes this by making order a structural property of the tree. It
delegates sibling ordering to libyang and reads children from the
`lyd_node.next`/`prev` chain, not from a keyed map.

## A concrete example

The demo module `conformance/fixtures/scrambled-children/module/order-demo.yang`
declares a container with leaves in this order:

```yang
container top {
  leaf z { type string; }
  leaf m { type string; }
  leaf a { type string; }
}
```

The input XML deliberately sends the leaves out of order:

```xml
<top xmlns="urn:order-demo">
  <a>a1</a>
  <z>z1</z>
  <m>m1</m>
</top>
```

Cambium serializes it back in schema declaration order:

```xml
<top xmlns="urn:order-demo">
  <z>z1</z>
  <m>m1</m>
  <a>a1</a>
</top>
```

This behavior is enforced by the `scrambled-children` case in
`/conformance/manifest.toml`. If a future change ever makes Cambium fall back to
map iteration, that case fails.

## `ordered-by user`

Schema order is not the only ordering rule. For `ordered-by user` lists and
leaf-lists, RFC 7950 says insertion order is semantically significant. Cambium
models these as `UserOrderedList<T>`, whose only mutators are positional:
`insert_first`, `insert_last`, `insert_before`, `insert_after`, and the
`move_*` family. There is no `set(key, value)` method, so using a positional
API on a system-ordered node is a compile error rather than a silent bug.

## System-ordered lists

For `ordered-by system` lists, libyang canonicalizes entries to key order on
parse. Cambium accepts this: a Terraform provider wants a stable, deterministic
read path rather than an arbitrary device-specific order. The
`system-list-canonical` fixture asserts that the same input always produces the
same output.

## What this means for you

- Emit NETCONF in the order the server expects, without hand-maintaining
  sort keys.
- Round-trip `ordered-by user` payloads byte-exact.
- Normalize `ordered-by system` reads to a stable canonical form.
- Trust one engine, pinned by SHA in `/VERSIONS`, for both Rust and Go.
