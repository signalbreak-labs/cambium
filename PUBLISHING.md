# Publishing Cambium

Cambium is a polyglot Rust + Go monorepo. Both languages share the same
vendored libyang + PCRE2 C engine under `/third_party/`, but neither crates.io
nor `go get` clones git submodules. A release-flatten step copies the C source
into each publishable unit so end users can build without submodule setup.

Current release-candidate status is tracked in dated notes under `docs/`, such
as `docs/release-readiness-2026-06-20.md`.

## Prerequisites

```bash
git submodule update --init --recursive
```

Release flattening also requires the `bindgen` CLI because it regenerates the
committed Rust bindings from the installed libyang headers. Local release checks
expect the normal Rust and Go toolchains, `cmake`, `golangci-lint`, and `zig`
when verifying musl/static builds.

## Release-flatten tooling

Run:

```bash
bash scripts/release-flatten.sh
```

This copies `third_party/libyang` and `third_party/pcre2` into:

- `rust/cambium-libyang-sys/vendor/{libyang,pcre2}`
- `go/internal/libyang/vendor/{libyang,pcre2}`

It strips test directories and regenerates the committed Rust bindings from the
installed libyang headers.

## Verify the flattened units build in isolation

```bash
bash scripts/check-flattened-build.sh
```

This creates temporary copies of the Rust crate and the Go module (with only
the flattened `vendor/` C source visible) and builds them. It catches the
classic "works in the workspace, breaks on publish" drift.

## What stays, what is stripped

- Kept: source, headers, CMake files, LICENSE/NOTICE/AUTHORS.
- Stripped: `libyang/tests/`, `pcre2/tests/` (build-only; not needed at publish
time).

## Publishing workflow

1. Choose the release version. The workspace currently uses `0.0.0`; bump
   `[workspace.package].version` before publishing crates. The Go submodule tag
   must use the subdirectory prefix, for example `go/v0.1.0`.
2. Update `/VERSIONS` if the engine SHA or CMake flags changed.
3. Open or update a PR to `main` and require GitHub CI to pass. Branch pushes do
   not run the workflow by themselves; CI runs on `pull_request` and `main`
   pushes.
4. Run the local green bar:

   ```bash
   bash scripts/green-bar.sh
   ```

5. Run `bash scripts/release-flatten.sh`.
6. Run `bash scripts/check-flattened-build.sh`.
7. Commit the updated `vendor/` directories and regenerated `bindings.rs`.
8. Publish crates / tag the Go module.

> Do not publish from a tree where `vendor/` is missing or stale. The flattened
> build is the publish gate.
