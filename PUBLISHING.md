# Publishing Cambium

Cambium is published as a single Go module rooted at `/go/`. The optional
libyang backend builds against the vendored libyang + PCRE2 C engine under
`/third_party/`, but `go get` does not clone git submodules. A release-flatten
step copies the C source into the module so end users can build without
submodule setup.

The shared layer (`/spec/`, `/conformance/`, `/VERSIONS`, `/third_party/`) is
language-neutral. A future `/<lang>/` binding would publish on its own track —
see AGENTS.md "Adding a language binding".

Released versions and the pending `[Unreleased]` changes are tracked in
`CHANGELOG.md`; each release is a `go/vX.Y.Z` tag (the latest is the newest
dated `## [go/vX.Y.Z]` heading there).

## Prerequisites

```bash
git submodule update --init --recursive
```

Local release checks expect the normal Go toolchain, `cmake`, `golangci-lint`,
and `zig` when verifying musl/static builds.

## Release-flatten tooling

Run:

```bash
bash scripts/release-flatten.sh
```

This copies `third_party/libyang` and `third_party/pcre2` into
`go/internal/libyang/vendor/{libyang,pcre2}`.

## Verify the flattened unit builds in isolation

```bash
bash scripts/check-flattened-build.sh
```

This creates a temporary copy of the Go module (with only the flattened
`vendor/` C source visible) and builds it. It catches the classic "works in the
workspace, breaks on publish" drift.

## What stays, what is stripped

- Kept: source, headers, CMake files, LICENSE/NOTICE/AUTHORS.
- Stripped: `libyang/tests/`, `pcre2/tests/` (build-only; not needed at publish
time).

## Publishing workflow

1. Pick the next version from `CHANGELOG.md`: bump the newest released
   `go/vX.Y.Z` per semver for what `[Unreleased]` carries, then rename
   `[Unreleased]` to `[go/vX.Y.Z] - YYYY-MM-DD`. The module is rooted at `/go/`,
   so the release tag must use the subdirectory prefix `go/vX.Y.Z`.
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
7. Commit the updated `vendor/` directory.
8. Pre-flight the tag name, then tag the Go module:

   ```bash
   scripts/check-release-tags.sh go/vX.Y.Z   # must pass before you tag
   git tag -s go/vX.Y.Z -m "Cambium Go vX.Y.Z"   # annotated + signed, like go/v0.4.0
   git push origin go/vX.Y.Z
   ```

9. Publish a GitHub release for the tag. That runs
   `.github/workflows/conformance-artifact.yml`, which attaches the
   `cambium-conformance-*.tar.gz` corpus package (and its `.sha256`) to the
   release.

> Do not publish from a tree where `vendor/` is missing or stale. The flattened
> build is the publish gate.

## Tag hygiene (read before you tag)

The module is rooted at `/go/`, so **every** version tag MUST carry the `go/`
subdirectory prefix: `go/vX.Y.Z`. The repo root is not a module.

A **bare** `vX.Y.Z` tag is not just ignored — it actively breaks. Go's module
proxy does not associate it with `github.com/signalbreak-labs/cambium/go`;
instead it synthesizes a phantom, `go.mod`-less module at the repo-root path
`github.com/signalbreak-labs/cambium` and caches it on proxy.golang.org. Proxy
entries are **immutable**, so a mistaken bare tag cannot be recalled even after
the git tag is deleted. This is precisely what the legacy `v0.1.0` tag did.

`scripts/check-release-tags.sh` enforces this — it runs in CI (the `release-tags`
job) and in `scripts/green-bar.sh`, and accepts a tag argument so you can
pre-flight a name before cutting it. Consumers always install the prefixed
module path:

```bash
go get github.com/signalbreak-labs/cambium/go@latest   # resolves go/vX.Y.Z
```
