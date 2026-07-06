# Release Runbook — v1.0.0

This runbook covers the **irreversible and outward-facing** launch steps. Every
file/config the automated pipeline needs (Apache-2.0 `LICENSE`, `NOTICE`,
`.goreleaser.yaml`, `release.yml`, `scorecard.yml`, version renumber) is already
committed on `overhaul/v1`. The steps below are performed by a human maintainer,
in order. Do not skip the audit gate.

## 0. Preconditions

- `overhaul/v1` is green: `make release-check` passes locally and in CI.
- You have push rights, GitHub Releases permission, npm publish rights (for the
  TypeScript SDK), and provider API keys for the live smoke test.

## 1. License & authorship audit (hard gate)

```bash
go install github.com/google/go-licenses@latest
go-licenses report ./... > /tmp/licenses.txt   # all deps must be Apache-compatible
```

- Review `/tmp/licenses.txt`: every dependency must be Apache-2.0-compatible
  (Apache/MIT/BSD/ISC). Reciprocal licenses (GPL/AGPL/LGPL) are blockers.
- Verify embedded-asset provenance: `internal/provider/catalog/models.yaml`,
  `internal/pricing/data/*.yaml`, and the astpb generated types (documented in
  `NOTICE`).
- Confirm sole authorship of all commits (the relicense prerequisite).

**If the audit fails, stop.** Reconcile findings into `NOTICE` or remove the
offending dependency before proceeding. The relicense is only sound once this
passes.

## 2. Hygiene sweep

```bash
git grep -iE "phase [0-9]"   # must be empty over tracked files (excl. spec/phase_*)
git grep -in "AGPL"          # must be empty
git ls-files | grep -E "BLUEPRINT.md|AGENTS.md|CLAUDE.md|spec/phase_" # dropped in step 3
```

## 3. Fresh curated history

```bash
git checkout --orphan v1 overhaul/v1
git rm -r --cached BLUEPRINT.md AGENTS.md CLAUDE.md spec/phase_* 2>/dev/null || true
rm -rf spec/phase_*        # keep spec/openapi
# Stage the tree in a curated Conventional-Commit sequence (10-15 commits):
#   contracts -> apierror/obs/transport -> provider kit + providers -> store ->
#   gateway/middleware -> reliability -> routing -> guardrails -> semcache ->
#   mcp -> SDKs -> docs -> CI/release/community.
# No AI attribution anywhere in messages or trailers.
```

- Archive the full original history to a **private mirror** before any force-push.
- Force-push `v1` as the new `main`; set it as the default branch; delete stale
  branches (`dev`, `overhaul/v1`).

## 4. Tag & release

```bash
git tag -a v1.0.0 -m "Polaris v1.0.0"
git push origin v1.0.0        # triggers release.yml -> goreleaser
```

`release.yml` runs `make release-check` then `goreleaser release --clean`,
producing signed multi-arch binaries + `checksums.txt` + SBOMs attached to the
GitHub Release. `publish-image.yml` fires on the same tag for the signed
multi-arch container image.

## 5. Smoke test the release

- Clean macOS + Linux: download the binary, run the Quickstart, confirm a first
  request succeeds.
- Container: `docker run --rm -p 8080:8080 ghcr.io/<owner>/polaris:v1.0.0` +
  a curl request.

## 6. Live-key confirmation ("tested with the api keys")

```bash
make live-smoke   # requires real provider API keys in the environment
```

This is the end-state validation against real providers.

## 7. Publish & register

- npm: `cd sdk/typescript && npm publish` (once the TS SDK ships).
- pkg.go.dev auto-indexes `github.com/JiaCheng2004/Polaris/pkg/client` on tag.
- Apply for the OpenSSF Best Practices badge; the Scorecard workflow runs weekly.
- Submit to relevant awesome-lists.

## Rollback

The container and binaries are immutable once published. To correct a bad
release, cut a new patch tag (`v1.0.1`); do not delete a published tag.
