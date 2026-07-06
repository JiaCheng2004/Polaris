# 5. Apache-2.0 license

Status: Accepted

## Context

Polaris is infrastructure meant to be embedded in other companies' stacks and
imported as a Go/TypeScript SDK. A copyleft license (the project's original
AGPL-3.0) silently makes dependents copyleft and is a hard procurement blocker
in most commercial settings — the opposite of what an adoption-focused gateway
needs.

## Decision

License Polaris under **Apache-2.0**. Apache-2.0 is permissive (safe to embed and
redistribute), includes an explicit patent grant, and is the license the
comparable 2026 gateways converged on. A `NOTICE` file plus a `REUSE.toml`
(REUSE 3.3, SPDX) declare copyright and third-party provenance — notably the
vendored ByteDance/Volcengine AST protobuf types. A `go-licenses` check in CI
keeps every dependency Apache-compatible.

## Consequences

- The Go and TypeScript SDKs are safe to import in commercial code without
  copyleft obligations.
- The relicense required confirming sole authorship and clean dependency
  provenance (a hard gate in the release runbook).
- CI enforces license hygiene continuously (`make license-check`,
  `make reuse-check`), so a future dependency with an incompatible license fails
  the build rather than shipping.
