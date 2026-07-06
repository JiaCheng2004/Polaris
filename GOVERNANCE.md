# Governance

Polaris uses a lightweight, maintainer-led governance model.

## Roles

- **Maintainers** — review and merge changes, cut releases, and set technical
  direction. Listed in `.github/CODEOWNERS`.
- **Contributors** — anyone who opens an issue or a pull request.

## Decision making

- Routine changes are approved by at least one maintainer review.
- Architecturally significant changes (new modality interfaces, cross-cutting
  middleware, new provider integration patterns, the wire contract) require
  maintainer consensus and an ADR under `docs/adr/`.
- The public `/v1` HTTP API is **wire-compatible**; changes that would alter it
  require an explicit deprecation path and a version bump.

## Contribution flow

1. Open an issue to discuss non-trivial changes first.
2. Submit a pull request against the default branch. One provider per PR; any
   endpoint change updates `docs/API_REFERENCE.md` in the same PR.
3. CI must be green (build, `-race` tests, lint, layering, contract/golden,
   config, govulncheck).
4. A maintainer reviews and merges.

## Releases

Releases follow semantic versioning and are cut from tags by the automated
release pipeline (`.github/workflows/release.yml`). See `docs/RELEASE_RUNBOOK.md`.

## Changing this document

Amendments to governance are themselves pull requests requiring maintainer
consensus.
