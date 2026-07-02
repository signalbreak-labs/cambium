# Security Policy

## Reporting a vulnerability

Please use GitHub private vulnerability reporting for `signalbreak-labs/cambium`:
open the repository's Security tab and choose "Report a vulnerability" to create a
GitHub Security Advisory.

Do not report undisclosed vulnerabilities in public issues. When possible, include
a minimal reproducer with the YANG module and input document that demonstrate the
problem.

## Supported versions

The latest `go/vX.Y.Z` release line receives security fixes.

## Vendored C dependencies

Cambium statically links vendored libyang and PCRE2. Their exact revisions are
pinned by SHA in `/VERSIONS`; bumps are manual and require the full conformance
and ordering suite to be re-run. The Dependabot rationale for these vendored
dependencies is documented in `.github/dependabot.yml`.

Upstream advisories for libyang and PCRE2 apply to Cambium binaries until the
pin is bumped.
