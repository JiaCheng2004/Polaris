#!/usr/bin/env bash
#
# build-launch-history.sh — construct a fresh, curated orphan `v1` branch that
# tells the architecture story in 16 Conventional Commits, dropping internal
# planning artifacts (spec/phase_*; BLUEPRINT/AGENTS/CLAUDE are already
# gitignored). The result is a clean public history whose final tree is
# byte-identical to the source branch minus the dropped files.
#
# This script is LOCAL and reversible: it never pushes, tags, or deletes the
# source branch. The maintainer inspects `v1`, then force-pushes it as `main`
# per docs/RELEASE_RUNBOOK.md. Undo with: git checkout overhaul/v1 && git branch -D v1
#
# Usage:
#   scripts/build-launch-history.sh [--source <branch>] [--target <branch>]
# Defaults: --source overhaul/v1  --target v1
set -euo pipefail

SOURCE="overhaul/v1"
TARGET="v1"
while [ $# -gt 0 ]; do
  case "$1" in
    --source) SOURCE="$2"; shift 2 ;;
    --target) TARGET="$2"; shift 2 ;;
    *) echo "unknown flag: $1" >&2; exit 2 ;;
  esac
done

# --- preconditions ---------------------------------------------------------
[ -n "$(git rev-parse --show-toplevel 2>/dev/null)" ] || { echo "not a git repo" >&2; exit 1; }
cd "$(git rev-parse --show-toplevel)"
[ "$(git rev-parse --abbrev-ref HEAD)" = "$SOURCE" ] || { echo "checkout $SOURCE first (on $(git rev-parse --abbrev-ref HEAD))" >&2; exit 1; }
[ -z "$(git status --porcelain)" ] || { echo "working tree not clean; commit or stash first" >&2; exit 1; }
if git show-ref --verify --quiet "refs/heads/$TARGET"; then
  echo "branch $TARGET already exists; delete it first: git branch -D $TARGET" >&2; exit 1
fi

# (message, space-separated paths) — order follows the dependency layering.
# Every tracked path (minus spec/phase_*) MUST appear in exactly one group; the
# coverage check at the end fails the run if anything is missed.
# NB: named COMMIT_GROUPS, not GROUPS — the latter is a reserved bash array
# (the current user's group IDs) and assigning to it is a no-op.
COMMIT_GROUPS=(
  "chore: project foundation, license, and community files|go.mod go.sum Makefile LICENSE LICENSES NOTICE REUSE.toml README.md README.zh-CN.md CHANGELOG.md SECURITY.md SUPPORT.md GOVERNANCE.md CODE_OF_CONDUCT.md .gitignore .dockerignore .editorconfig .env.example"
  "feat(modality): unified multi-modality contracts|internal/modality"
  "feat(core): apierror, observability, and the unified transport core|internal/apierror internal/obs internal/transport"
  "feat(config): configuration loader, validation, and schemas|internal/config schema"
  "feat(store): sqlite, postgres, cache, and blob stores|internal/store"
  "feat(provider): adapter kit, provider integrations, catalog, and pricing|internal/provider internal/pricing internal/understanding internal/tooling internal/files"
  "feat(reliability): circuit breakers, load shedding, hedging, and idempotency|internal/reliability"
  "feat(routing): adaptive multi-strategy routing and failover|internal/routing"
  "feat(guardrails): PII, secrets, and prompt-injection guardrail engine|internal/guardrails"
  "feat(semcache): embedding semantic cache|internal/semcache"
  "feat(mcp): stateless streamable-HTTP MCP gateway with OAuth 2.1|internal/mcp"
  "feat(gateway): HTTP gateway, handlers, middleware, and entrypoint|internal/gateway cmd"
  "feat(sdk): public Go client SDK|pkg"
  "feat(config): deployment configuration and provider definitions|config"
  "docs: API reference, guides, examples, OpenAPI, and docs site|docs examples spec mkdocs.yml requirements-docs.txt"
  "build: CI, release automation, containers, and test suites|.github deployments scripts tests .golangci.yml .goreleaser.yaml .vacuum.yaml .revive-doc.toml"
)

echo "==> creating orphan branch $TARGET from $SOURCE"
git checkout --quiet --orphan "$TARGET"
git reset --quiet                       # unstage all; working tree preserved
rm -rf spec/phase_*                     # drop internal planning specs

echo "==> committing curated history (${#COMMIT_GROUPS[@]} commits)"
for entry in "${COMMIT_GROUPS[@]}"; do
  msg="${entry%%|*}"
  paths="${entry#*|}"
  # shellcheck disable=SC2086
  git add -- $paths 2>/dev/null || true
  if git diff --cached --quiet; then
    echo "   (skip, no files) $msg" >&2
    continue
  fi
  GIT_COMMITTER_NAME="$(git config user.name)" GIT_COMMITTER_EMAIL="$(git config user.email)" \
    git commit --quiet -m "$msg"
  echo "   ✓ $msg"
done

# --- coverage check: nothing tracked may be left uncommitted ---------------
leftover="$(git status --porcelain)"
if [ -n "$leftover" ]; then
  echo "ERROR: files not covered by any commit group:" >&2
  echo "$leftover" >&2
  echo "Fix the COMMIT_GROUPS mapping and re-run (git checkout $SOURCE && git branch -D $TARGET)." >&2
  exit 1
fi

# --- integrity check: final tree == source tree minus dropped files --------
diff="$(git diff "$SOURCE" "$TARGET" -- . ':(exclude)spec/phase_*' || true)"
if [ -n "$diff" ]; then
  echo "ERROR: $TARGET tree differs from $SOURCE (excluding spec/phase_*):" >&2
  echo "$diff" | head -40 >&2
  exit 1
fi

echo
echo "==> $TARGET built: $(git rev-list --count "$TARGET") commits, tree matches $SOURCE minus spec/phase_*."
echo "    Inspect:  git log --oneline $TARGET"
echo "    Verify:   go build ./... && go test -race ./..."
echo "    Then follow docs/RELEASE_RUNBOOK.md to force-push as main and tag v1.0.0."
echo "    Undo:     git checkout $SOURCE && git branch -D $TARGET"
