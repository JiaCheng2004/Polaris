# Phase 3 Image + Voice + Embeddings

This folder is the ABZ package for Polaris Phase 3.

- `A`: the current state after Phase 2 completion
- `B`: the ordered implementation steps required to finish Phase 3
- `Z`: the fully completed Phase 3 image, voice, embeddings, auth, and SDK layer described by the architecture plan

`docs/ARCHITECTURE.md` remains the source of truth. These files break Phase 3 into concrete, decision-complete subplans with explicit dependencies and exit criteria.

## ABZ Map

### A: Historical Starting Baseline

Phase 2 was complete before `3A` through `3J`:

- full chat provider matrix for OpenAI, Anthropic, DeepSeek, Google, ByteDance, xAI, Qwen, and Ollama
- PostgreSQL and Redis runtime support alongside SQLite and the in-memory cache
- runtime aliases, failover, hot reload, and Prometheus metrics
- validated local, prod, and dev stack entrypoints
- placeholder modality contracts and handlers for image, voice, embeddings, and video
- placeholder public Go SDK package in `pkg/client`

That is the baseline for Phase 3. Phase 3 expands Polaris beyond chat and adds the first admin and public-client surfaces that depend on the now-stable gateway runtime.

### B: Implementation Steps

| Step | Name | Status | Purpose |
|---|---|---|---|
| `3A` | [Multi-User Auth and Admin Keys](./3A_multi_user_auth_and_admin_keys.md) | Implemented | Make DB-backed auth and admin key management real. |
| `3B` | [Multimodal Contracts and Registry](./3B_multimodal_contracts_and_registry.md) | Implemented | Turn image, voice, and embedding contracts plus registry routing into real runtime infrastructure. |
| `3C` | [Embeddings Surface: OpenAI and Google](./3C_embeddings_surface_openai_google.md) | Implemented | Deliver the first non-chat modality through the OpenAI-compatible embeddings endpoint. |
| `3D` | [Image Provider Wave: OpenAI and Google](./3D_image_provider_wave_openai_google.md) | Implemented | Activate image generation and editing with the first provider wave. |
| `3E` | [Image Provider Wave: ByteDance and Qwen](./3E_image_provider_wave_bytedance_qwen.md) | Implemented | Complete the Phase 3 image provider matrix. |
| `3F` | [Voice Provider Wave: OpenAI](./3F_voice_provider_wave_openai.md) | Implemented | Activate TTS and STT with the provider that supports both surfaces. |
| `3G` | [Voice Provider Wave: ByteDance](./3G_voice_provider_wave_bytedance.md) | Implemented | Complete ByteDance voice support in the Phase 3 surface. |
| `3H` | [Usage, Models, and Costing Reconciliation](./3H_usage_model_catalog_and_costing.md) | Implemented | Align usage, model metadata, and cost reporting across all Phase 3 modalities. |
| `3I` | [Public Go SDK](./3I_public_go_sdk.md) | Implemented | Ship the first real importable Go client for implemented Polaris endpoints. |
| `3J` | [Phase 3 Hardening and Acceptance](./3J_phase_3_hardening_and_acceptance.md) | Implemented | Close Phase 3 with docs, SDK, tests, and acceptance gates aligned. |

### Z: End Goal

Phase 3 is complete only when Polaris can do all of the following:

- serve embeddings through OpenAI and Google models
- serve image generation and editing through OpenAI, Google, ByteDance, and Qwen models
- serve TTS and STT through OpenAI and ByteDance models
- enforce `auth.mode: multi-user` with admin-only API key management endpoints
- keep `none` and `static` auth modes working without regression
- expose correct modality-specific metadata from `GET /v1/models`
- record and report usage for embeddings, images, and voice in the existing usage pipeline
- provide a working public Go SDK for implemented endpoints
- keep docs, runtime claims, and stack entrypoints aligned with the real Phase 3 implementation
- pass the Phase 3 provider, auth, integration, SDK, and race-test matrix

## Phase 3 Boundaries

The following items are explicitly out of scope for this Phase 3 package:

- video generation
- full-duplex audio
- response caching
- npm or pip SDK packages
- release packaging and load testing
- a public request contract for voice cloning

Those belong to Phase 4 or later follow-up work and should not be folded into this package.

## Sequence Rule

The intended implementation order is `3A` through `3J`.

- `3A` is the blocker-clearing step because admin key management and DB-backed auth must exist before the public admin surface is real.
- `3B` must land before modality handlers are implemented so provider waves share one stable contract layer.
- `3C` should land before image and voice because embeddings are the lowest-translation non-chat modality.
- `3D` and `3E` together complete the image surface.
- `3F` and `3G` together complete the Phase 3 voice surface.
- `3H` reconciles cross-cutting runtime behavior only after all Phase 3 modalities exist.
- `3I` ships after the endpoint contracts are real and stable.
- `3J` is the explicit close-out step. Do not call Phase 3 complete before it lands.
