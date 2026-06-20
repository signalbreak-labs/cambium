# Release Readiness - 2026-06-20

This note records the release-readiness state for the cgo-free Go parity
release candidate before the broad code-quality sweep.

## Branches and PR

- Source parity branch: `feat/pure-go-rebuild-spec`
- Main-history integration branch: `release/cgo-free-go-parity`
- Draft PR: <https://github.com/signalbreak-labs/cambium/pull/1>

The integration branch exists because `feat/pure-go-rebuild-spec` and `main`
have unrelated history, so GitHub cannot create a normal PR directly from the
feature branch.

## Local Proof

The following release gates passed locally:

```bash
scripts/green-bar.sh
scripts/check-flattened-build.sh
```

The musl/static Go build was also checked from a fresh clone:

```bash
CAMBIUM_CC="zig cc -target x86_64-linux-musl" bash go/internal/libyang/build.sh
cd go
CGO_ENABLED=1 CC="zig cc -target x86_64-linux-musl" \
  GOOS=linux GOARCH=amd64 \
  go build -ldflags '-extldflags "-static"' ./...
```

The local green bar includes Rust fmt/clippy/tests, Rust conformance, default
Go purity, Go vet, Go race tests, Go lint, Go conformance, and engine config
drift detection.

## Release Version

The release candidate version is `0.1.0` for Rust crates. The matching Go module
release tag should use the subdirectory prefix: `go/v0.1.0`.

## Current Blockers

GitHub Actions did not execute for PR #1. Each job failed before runner
   startup with:

   ```text
   The job was not started because recent account payments have failed or your
   spending limit needs to be increased. Please check the 'Billing & plans'
   section in your settings.
   ```

Fix billing/spending limits, then rerun PR CI.

## Release Checklist

- Rerun GitHub CI on PR #1 after the billing/spending-limit issue is fixed.
- Keep the PR draft until GitHub CI is green.
- Rerun `scripts/release-flatten.sh` if any engine sources or bindings change.
- Rerun `scripts/check-flattened-build.sh` before publishing.
