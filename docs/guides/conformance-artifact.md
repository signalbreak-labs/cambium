# Conformance Corpus Artifact

Cambium release automation publishes the language-neutral conformance corpus as
`cambium-conformance-<version>.tar.gz` plus a `.sha256` file. The archive contains:

- `VERSION` — the artifact version label.
- `VERSIONS` — the pinned libyang/PCRE2 source and CMake flags.
- `conformance/` — `manifest.toml`, fixtures, golden outputs, and shared corpus
  modules.

Use this artifact when building another binding or an external runner that needs
the same inputs without cloning the full Cambium repository.

```sh
tar -xzf cambium-conformance-<version>.tar.gz
cd cambium-conformance-<version>
```

Run `conformance/manifest.toml` through your binding's runner and compare against
the files under `conformance/golden/`. Backend/data runners must also honor the
engine pin in `VERSIONS` when they use libyang.

To build the same package locally:

```sh
scripts/package-conformance.py --version dev --out-dir dist
scripts/check-conformance-package.sh
```
