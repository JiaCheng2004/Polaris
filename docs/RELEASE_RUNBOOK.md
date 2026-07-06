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
make license-check   # go-licenses v2: every dep must be Apache-compatible
make reuse-check     # REUSE 3.3 (SPDX) compliance across the tree
go run github.com/google/go-licenses/v2@latest report ./cmd/polaris ./pkg/client > /tmp/licenses.txt
```

- `make license-check` fails on any reciprocal (GPL/AGPL/LGPL) dependency; the
  current tree is all Apache/MIT/BSD. Review `/tmp/licenses.txt` for the full
  inventory that seeds `NOTICE`.
- Verify embedded-asset provenance: `internal/provider/catalog/models.yaml`,
  `internal/pricing/data/*.yaml`, and the astpb generated types (declared in
  `NOTICE` + `REUSE.toml`).
- Confirm sole authorship of all commits (the relicense prerequisite).

**If the audit fails, stop.** Reconcile findings into `NOTICE`/`REUSE.toml` or
remove the offending dependency before proceeding. The relicense is only sound
once this passes.

## 2. Hygiene sweep

```bash
git grep -iE "phase [0-9]"   # must be empty over tracked files (excl. spec/phase_*)
git grep -in "AGPL"          # must be empty
git ls-files | grep -E "BLUEPRINT.md|AGENTS.md|CLAUDE.md|spec/phase_" # dropped in step 3
```

## 3. Fresh curated history

```bash
scripts/build-launch-history.sh          # builds a local orphan `v1` (16 commits)
git log --oneline v1                      # inspect the curated narrative
git checkout v1 && go build ./... && go test -race ./...   # verify the tree
```

The script drops `spec/phase_*` and `docs/internal/` (BLUEPRINT/AGENTS/CLAUDE are already gitignored),
lays down 16 Conventional Commits along the dependency layering, and self-checks
that the final tree is byte-identical to `overhaul/v1` minus the dropped files.
It never pushes. No AI attribution appears in any message or trailer.

- Archive the full original history to a **private mirror** before any force-push.
- Force-push `v1` as the new `main`; set it as the default branch; delete stale
  branches (`dev`, `overhaul/v1`).

## 4. Tag & release

```bash
# Signed annotated tag — GitHub verifies GPG/SSH signatures (configure signing first).
git tag -s v1.0.0 -m "Polaris v1.0.0"
git push origin v1.0.0        # triggers release.yml -> goreleaser
```

`release.yml` runs `make release-check` then `goreleaser release --clean`,
producing multi-arch binaries + `checksums.txt` + **both SPDX and CycloneDX
SBOMs** + **cosign keyless signatures**, and then attaches a **SLSA build
provenance attestation** (`actions/attest-build-provenance`) — all attached to
the GitHub Release. `publish-image.yml` fires on the same tag for the signed,
attested multi-arch container image.

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

- Docs: `mike deploy --push --update-aliases 1.0 latest` publishes the versioned
  MkDocs site to GitHub Pages.
- npm (TS SDK): publish via **npm OIDC Trusted Publishing** from CI (npm CLI
  ≥ 11.5.1) — configure the trusted publisher once; provenance is then generated
  automatically (no `--provenance` flag, no long-lived npm token).
- pkg.go.dev auto-indexes `github.com/JiaCheng2004/Polaris/pkg/client` on tag.
- Apply for the OpenSSF Best Practices badge at bestpractices.dev; the Scorecard
  workflow already publishes results weekly.
- Submit to relevant awesome-lists.

## Rollback

The container and binaries are immutable once published. To correct a bad
release, cut a new patch tag (`v1.0.1`); do not delete a published tag.
