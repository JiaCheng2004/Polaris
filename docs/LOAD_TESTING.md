# Polaris Load Testing

This document is the pre-release load-validation checklist for the `v2.1.0` close-out. It is intentionally opt-in because it calls live providers and can spend provider credits. It is not a default CI gate.

## Baseline

Run the repo-local gate first:

```bash
make release-check
```

The release gate validates Docker Compose files quietly. Use `make stack-config STACK=<name>` separately only when you intentionally need rendered Compose output for local debugging.

Before running live provider validation, you can inspect the configured verification surface without credentials:

```bash
make verify-models
```

If you are validating real providers, export credentials and run:

```bash
POLARIS_LIVE_SMOKE=1 POLARIS_LIVE_SMOKE_STRICT=1 make live-smoke
```

The Makefile live-smoke targets default `LIVE_SMOKE_TIMEOUT` to `45m` because the strict matrix includes provider-side async video, notes, podcast, and music polling.

To include opt-in provider paths in the same validation run, opt in explicitly:

```bash
POLARIS_LIVE_SMOKE=1 POLARIS_LIVE_SMOKE_INCLUDE_OPT_IN=1 make live-smoke
```

To enable one opt-in provider family without enabling the whole opt-in set, use the provider-specific env:

```bash
POLARIS_LIVE_SMOKE=1 POLARIS_LIVE_SMOKE_PROVIDER_ELEVENLABS=1 make live-smoke
```

The same pattern applies to the newer provider families, for example:

```bash
POLARIS_LIVE_SMOKE=1 \
POLARIS_LIVE_SMOKE_PROVIDER_OPENROUTER=1 \
POLARIS_LIVE_SMOKE_PROVIDER_TOGETHER=1 \
POLARIS_LIVE_SMOKE_PROVIDER_BEDROCK=1 \
make live-smoke
```

## Automated Load Check

Run the local backend load check:

```bash
make load-check
```

This starts an in-process Polaris gateway with SQLite and in-memory cache, forces response caching on, disables rate limiting to avoid false negatives, and runs:

- repeated cached and uncached synchronous traffic for chat, embeddings, image, TTS, STT, and MiniMax music
- a mixed chat, embeddings, image, TTS, and STT burst
- concurrent async video lifecycle polling across release and opted-in video providers
- concurrent OpenAI audio sessions, including one `server_vad` session
- MiniMax lyrics, sync music cache behavior, async music jobs, and content downloads
- `/ready`, `/metrics`, and `/v1/usage` checks after load

Optional knobs:

- `LOAD_CHECK_TIMEOUT=90m` changes the Go test timeout for the Makefile target.
- `POLARIS_LOAD_CHAT_REPEATS=20` controls repeated chat cache checks.
- `POLARIS_LOAD_SYNC_REPEATS=3` controls repeated sync cache checks.
- `POLARIS_LOAD_BURST_PARALLEL=5` controls mixed-modality burst concurrency.
- `POLARIS_LOAD_VIDEO_JOBS=5` controls async video job count.
- `POLARIS_LOAD_AUDIO_SESSIONS=5` controls audio session count.
- `POLARIS_LOAD_MUSIC_JOBS=3` controls async music job count.
- `POLARIS_LOAD_REPORT=/tmp/polaris-load.md` writes a Markdown report.
- `POLARIS_LOAD_ACCEPT_PROVIDER_TIMEOUTS=0` turns provider-side async polling timeouts into failures instead of accepted known-provider timeouts.
- `POLARIS_LOAD_INCLUDE_OPT_IN=1` includes opt-in provider families.
- `POLARIS_LOAD_PROVIDER_ELEVENLABS=1` includes only the ElevenLabs opt-in music plan path.

Provider authentication, entitlement, quota, and billing failures are treated as real load-check failures for release-blocking providers. If an account cannot open the required concurrent sessions or submit the required async jobs, record that as a provider/account blocker instead of weakening the harness.

## Pre-Release Scenarios

The automated load check covers the scenarios below. If you run them manually against a booted Polaris instance, use the same expected outcomes.

### 1. Cached and uncached sync traffic

- Repeat the same non-streaming chat request 20 times and confirm `X-Polaris-Cache` transitions from `miss` to `hit`.
- Repeat the same embeddings, image, TTS, and STT requests and confirm they do not regress under repetition.
- Repeat one streaming chat request and confirm it remains `bypass`.

### 2. Mixed-modality burst

- Send chat, embeddings, image, TTS, and STT requests concurrently with `xargs -P` or a similar shell-level runner.
- Confirm the server stays healthy and `/v1/usage` continues to aggregate requests correctly.
- Confirm `/metrics` remains scrapeable during the burst.

### 3. Video async lifecycle

- Submit at least 5 concurrent video jobs across the shipped providers you intend to release.
- Poll status concurrently until every job reaches a terminal state or a known provider-side timeout.
- If any provider returns completed output during the window, verify `/v1/video/generations/:id/content` downloads bytes successfully.

### 4. Audio session concurrency

- Open at least 5 concurrent audio sessions.
- For each session, send `input_text` plus `response.create` and confirm you receive `response.completed`.
- For OpenAI-backed audio models, also verify one `server_vad` session.
- Confirm `/v1/usage?group_by=model&modality=audio` reflects the completed sessions.

### 5. Music jobs and cache behavior

- Repeat the same synchronous lyrics and short music generation requests and confirm `X-Polaris-Cache` transitions from `miss` to `hit`.
- Repeat composition-plan requests only when the ElevenLabs opt-in provider slice is enabled; MiniMax does not expose composition plans.
- Submit at least 3 concurrent async music jobs and poll until each job reaches `completed`, `failed`, or a known provider-side timeout.
- Verify `/v1/music/jobs/:id/content` downloads bytes successfully for completed jobs.
- If MiniMax is in the release set, verify one long-running MiniMax request uses `mode=async` and completes without relying on the sync HTTP response path.
- If an opt-in provider slice is enabled, run the same scenario family for it and record it separately from the release-blocking matrix.

### 6. Store and cache behavior

- Run the same scenarios with SQLite + memory cache for the default open-source gate.
- Confirm `/ready` stays green during the run.
- Confirm request logs continue to flush without dropping usage rows.

## Recording

Record the outcome of `make load-check` in `spec/phase_5_music/5B_phase_5_hardening_and_acceptance.md` before cutting the `v2.1.0` tag, with MiniMax in the release-blocking matrix and ElevenLabs, when exercised, in the preview matrix.
