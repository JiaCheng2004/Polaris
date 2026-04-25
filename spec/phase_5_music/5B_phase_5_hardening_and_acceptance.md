# Phase 5B: Music Hardening and Acceptance

This document is the canonical close-out record for the music wave that follows the shipped Phase 5A runtime. It does not add a new public surface. It hardens the existing music implementation for the `v2.1.0` release candidate.

## Scope

Phase 5B covers:

- MiniMax regional config hardening and timeout guidance
- normalized music timeout behavior across sync and async job paths
- live-smoke coverage for the release-blocking MiniMax music path plus an opt-in ElevenLabs preview path
- release-truth documentation updates for music

It does not add new music providers, new endpoints, or new music capabilities.

## Repo-Local Gate

The repo-local validation gate for this close-out is:

```bash
make release-check
```

Phase 5B is not locally complete until `make release-check` passes. The release gate uses quiet Docker Compose validation through `stack-validate`; use `stack-config` only for intentional local debugging because it renders environment-expanded Compose output.

## Live Release Gate

The strict live gate for `v2.1.0` includes the existing provider matrix plus the release-blocking music paths below:

- MiniMax lyrics
- MiniMax async music generation
- MiniMax music content download

If a provider path is documented as shipped and the corresponding credentials or account access are missing, that is a release blocker rather than an implicit skip when `POLARIS_LIVE_SMOKE_STRICT=1`.

## Preview Matrix

The ElevenLabs music path remains implemented in code and reference config, but it is preview-only for `v2.1.0`. Run it explicitly with:

```bash
POLARIS_LIVE_SMOKE=1 POLARIS_LIVE_SMOKE_STRICT=1 POLARIS_LIVE_SMOKE_ELEVENLABS=1 make live-smoke
```

Preview checks:

- ElevenLabs sync music generation
- ElevenLabs streamed music generation
- ElevenLabs composition plans
- ElevenLabs stems

## Load Validation

Run and record the executable load-validation outcomes from `docs/LOAD_TESTING.md` here before tagging `v2.1.0`, including:

- repeated music cache hit/miss behavior
- concurrent async music jobs
- concurrent music content downloads
- mixed-modality load with video and audio sessions still healthy
- `make load-check` with SQLite + memory cache

## Status

Current close-out state:

- repo-local gate: passed on 2026-04-24 with `make release-check`
- live release-blocking music matrix: passed on 2026-04-24 with `.env` loaded and `make live-smoke-strict`
- preview music matrix: optional / pending
- reduced local load-check: passed on 2026-04-24 with chat=2, sync=2, burst=1, video=1, audio=1, music=1
- reduced local load-check: passed on 2026-04-24 with chat=2, sync=1, burst=1, video=1, audio=1, music=1
- targeted OpenAI Realtime audio concurrency: passed on 2026-04-24 with `POLARIS_LOAD_AUDIO_SESSIONS=2` and only `TestLoadCheckMatrix/audio_session_concurrency`
- full local load-check: previously blocked on OpenAI Realtime quota at the default 5-session audio concurrency; a smaller 2-session targeted concurrency check now passes, but the default full load profile has not been rerun
- `v2.1.0` release tag: pending
