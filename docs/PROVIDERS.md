# Polaris Provider Notes

This file is the operator-facing companion to `BLUEPRINT.md` §7 and §11. It records provider-specific authentication rules, compatibility quirks, and the implementation phase each provider belongs to.

Provider/model metadata is sourced from the embedded matrix in `internal/provider/catalog/models.yaml`. Docs describe that matrix-driven implementation; they are not the source of truth for model IDs, family IDs, aliases, or verification classes.

Release rule for `v2.1.0`: every provider path described here as shipped and release-blocking must pass the strict live-smoke matrix before the release tag is cut when credentials are available, and the repo-local open-source gate must pass before release completion is claimed. Production Postgres/Redis load testing is optional operator proof for service deployments. The live-smoke harness now derives `strict`, `opt_in`, and `skipped` coverage from the embedded provider model matrix. Opt-in provider paths do not block the tag unless they are explicitly included with `POLARIS_LIVE_SMOKE_INCLUDE_OPT_IN=1` or a provider-specific opt-in env such as `POLARIS_LIVE_SMOKE_PROVIDER_ELEVENLABS=1`.

## Phase 1

### OpenAI

- Auth: `Authorization: Bearer <key>`
- Scope: chat in Phase 1, embeddings/image/voice in Phase 3, video in Phase 4
- Status: chat implemented in Phase 1; embeddings implemented in Phase 3C; image generation/editing implemented in Phase 3D; TTS and STT implemented in Phase 3F; Sora video implemented in Phase 4C
- Notes: standard SSE streaming and the closest OpenAI wire compatibility baseline; the current embedded catalog includes `gpt-5.5` as the frontier OpenAI chat family. Embeddings use the native `/embeddings` endpoint and support both `float` and `base64` output encodings through the OpenAI-compatible Polaris surface; image generation uses `/images/generations` and image editing uses multipart `/images/edits`. OpenAI GPT Image models, including `gpt-image-2`, return inline image bytes, so Polaris preserves the shared `url|b64_json` contract by normalizing default `url` responses into `data:` URLs and returning inline base64 when callers request `b64_json`; older DALL·E-style models continue using the legacy Image API response-format passthrough. Voice uses `/audio/speech` for TTS and multipart `/audio/transcriptions` for STT, with Polaris requesting `verbose_json` for `whisper-1` JSON transcripts so segment metadata survives the shared response shape. Sora video uses `/videos`, `/videos/{id}`, and `/videos/{id}/content`; Polaris currently exposes the truthful subset `prompt`, `first_frame`, `duration`, `aspect_ratio`, `resolution`, and `with_audio`. Sora models are treated as non-cancelable in Polaris, so `DELETE /v1/video/generations/:id` returns `job_not_cancelable` for those jobs. OpenAI audio sessions now use the native Realtime websocket transport under the shared Polaris session contract and support both `manual` and `server_vad`; explicit cascaded `audio_pipeline` models remain available only as a compatibility path.

### Anthropic

- Auth: `x-api-key` plus `anthropic-version`
- Primary scope: chat in Phase 1
- Notes: adapter must translate Anthropic system/message structure into Polaris/OpenAI-compatible shapes

## Phase 2

### DeepSeek

- Auth: OpenAI-compatible bearer token
- Scope: chat
- Status: implemented in Phase 2B
- Notes: OpenAI-compatible chat completions; `reasoning_content` is currently stripped from the normalized Polaris response surface

### Google

- Auth: `x-goog-api-key` header
- Scope: chat in Phase 2, image and embeddings in Phase 3
- Status: chat implemented in Phase 2C; embeddings implemented in Phase 3C; image generation/editing implemented in Phase 3D
- Notes: chat adapter uses native `generateContent` / `streamGenerateContent`, translates `assistant` to Gemini `model`, and normalizes Gemini SSE responses back into OpenAI-style chat chunks; embeddings use Gemini `embedContent` for single-input requests and `batchEmbedContents` for multi-input requests, with Polaris-side normalization for `base64` output. Current embedding token usage is recorded when the provider returns it; otherwise Polaris reports `0` for the Google embedding usage fields in this phase. Image generation/editing also use native `generateContent`; when Polaris callers request `response_format: url`, Gemini inline image bytes are normalized into `data:` URLs rather than hosted URLs. Google voice is not a shipped Polaris provider surface in this release.

### Google Vertex

- Auth: Google ADC / OAuth bearer token for `https://www.googleapis.com/auth/cloud-platform`
- Scope: video in Phase 4
- Status: Veo video implemented in Phase 4C under the separate `google-vertex` provider name
- Notes: Polaris keeps Vertex video isolated from the API-key-based Gemini provider. The adapter uses publisher-model `predictLongRunning` plus `fetchPredictOperation`, and cancels with the standard long-running operation `:cancel` path. Polaris currently exposes the truthful Veo subset `prompt`, `first_frame`, `last_frame`, `duration`, `aspect_ratio`, `resolution`, and `with_audio`. The implementation expects ADC at runtime, uses `project_id` and `location` from config, and serves completed video bytes through the Polaris `/content` endpoint from inline base64 output when Vertex returns it.

### xAI

- Auth: bearer token
- Scope: chat
- Status: implemented in Phase 2B
- Notes: uses the legacy chat completions surface at `https://api.x.ai/v1/chat/completions`

### OpenRouter

- Auth: bearer token
- Scope: chat-first in the current provider-expansion wave
- Status: implemented through the shared OpenAI-compatible provider base
- Notes: Polaris uses the OpenRouter OpenAI-compatible `chat/completions` surface at `https://openrouter.ai/api/v1`. Provider-specific OpenRouter extensions are intentionally not normalized into the shared contract yet; this adapter is for standard chat/function-calling/streaming flows.

### Together

- Auth: bearer token
- Scope: chat-first in the current provider-expansion wave
- Status: implemented through the shared OpenAI-compatible provider base
- Notes: Polaris uses the Together OpenAI-compatible `chat/completions` surface at `https://api.together.xyz/v1`. The first shipped Polaris scope is text chat only; image and serverless-specific Together surfaces remain out of scope for this wave.

### Groq

- Auth: bearer token
- Scope: chat-first in the current provider-expansion wave
- Status: implemented through the shared OpenAI-compatible provider base
- Notes: Polaris uses the Groq OpenAI-compatible endpoint at `https://api.groq.com/openai/v1`. Groq-specific Responses support and non-text endpoints are intentionally left out of the first Polaris provider-family wave.

### Fireworks

- Auth: bearer token
- Scope: chat-first in the current provider-expansion wave
- Status: implemented through the shared OpenAI-compatible provider base
- Notes: Polaris uses the Fireworks OpenAI-compatible inference endpoint at `https://api.fireworks.ai/inference/v1`. Fireworks-native deployments, fine-tuning, and Anthropic-compatible surfaces are intentionally not part of the first Polaris adapter.

### Featherless

- Auth: bearer token
- Scope: chat-first in the current provider-expansion wave
- Status: implemented through the shared OpenAI-compatible provider base
- Notes: Polaris uses the Featherless OpenAI-compatible endpoint at `https://api.featherless.ai/v1`. Featherless recommends `HTTP-Referer` and `X-Title` for application attribution, but Polaris does not require them for the shared gateway path.

### Moonshot / Kimi

- Auth: bearer token
- Scope: chat-first in the current provider-expansion wave
- Status: implemented through a chat-first adapter on the Moonshot OpenAI-compatible base
- Notes: Polaris uses `https://api.moonshot.ai/v1` and the provider `chat/completions` contract. The shipped first-cut scope is text chat and function calling; official Moonshot tools and formula APIs remain out of scope for this phase.

### GLM

- Auth: bearer token
- Scope: chat-first in the current provider-expansion wave
- Status: implemented through a chat-first adapter on the GLM REST base
- Notes: Polaris uses the current GLM chat completion endpoint family rooted at `https://open.bigmodel.cn/api/paas/v4`. The first-cut Polaris scope uses the standard `chat/completions` path and intentionally excludes the coding-plan-specific endpoint family.

### Mistral

- Auth: bearer token
- Scope: chat-first in the current provider-expansion wave
- Status: implemented through a chat-first adapter on the Mistral API base
- Notes: Polaris uses the Mistral `chat/completions` surface at `https://api.mistral.ai/v1`. Native Mistral-specific OCR, agents, and FIM endpoints are outside the first Polaris adapter wave.

### Amazon Bedrock

- Auth: AWS SigV4 using access key id, secret access key, optional session token, and region
- Scope: native chat plus Titan text embeddings
- Status: implemented through a native Bedrock adapter
- Notes: Polaris uses Bedrock Runtime `Converse` and `ConverseStream` for chat plus `InvokeModel` for Titan embeddings, not an OpenAI-compatibility shim. Configure official Bedrock model IDs such as `amazon.nova-2-lite-v1:0` and `amazon.titan-embed-text-v2:0`, and let Polaris sign each request against the configured region.

### NVIDIA

- Auth: bearer token
- Scope: chat plus embeddings
- Status: implemented through the shared OpenAI-compatible provider base
- Notes: Polaris uses the NVIDIA-hosted OpenAI-compatible endpoint at `https://integrate.api.nvidia.com/v1` for both chat and embeddings. Configure official NVIDIA model IDs such as `nvidia/NVIDIA-Nemotron-Nano-9B-v2` for chat and `nvidia/llama-nemotron-embed-1b-v2` for embeddings; Polaris preserves that official wire model ID internally and also treats the shortened configured form without the leading `nvidia/` as a local alias.

### Replicate

- Auth: bearer token
- Scope: async video-first in the current provider-expansion wave
- Status: implemented through a native Predictions adapter on the Replicate HTTP API
- Notes: Polaris uses the Replicate Predictions API at `https://api.replicate.com/v1` and creates official-model predictions through `/models/{owner}/{model}/predictions`. The first shipped Polaris scope is async video generation only, mapped onto the existing Polaris video job/status/content contract. Configure official Replicate model IDs such as `minimax/video-01`, and expect Replicate outputs to be short-lived downloadable URLs that still require the bearer token when Polaris fetches the completed asset.

### Qwen

- Auth: DashScope compatible-mode bearer token
- Scope: chat in Phase 2, image in Phase 3
- Status: chat implemented in Phase 2C; image generation/editing implemented in Phase 3E
- Notes: chat adapter uses the DashScope OpenAI-compatible endpoint and enables stream usage reporting with `stream_options.include_usage`. Image generation/editing use the native DashScope multimodal-generation endpoint. When callers request `response_format: b64_json`, Polaris downloads the provider-returned image URL and re-encodes it as base64 to preserve the shared image contract.

### ByteDance

- Auth: `Authorization: Bearer <ARK_API_KEY>`
- Scope: chat in Phase 2, image/voice in Phase 3, video in Phase 4, translation and simultaneous interpretation in the current speech wave
- Status: chat implemented in Phase 2D; image generation/editing implemented in Phase 3E; one-shot TTS and synchronous STT implemented in the Phase 3 voice wave; async Seedance video submit/poll/cancel implemented in Phase 4A, expanded to the full planned request parity surface in Phase 4B, and hardened with Polaris-owned content download in Phase 4C; machine translation and simultaneous interpretation implemented on the current OpenSpeech speech stack
- Notes: chat adapter targets the ModelArk `chat/completions` data plane and uses the configured model endpoint as the request base URL. Current recommended chat/vision models in Polaris are `doubao-seed-2.0-pro` and `doubao-seed-1.6-vision`; legacy `doubao-pro-256k` remains supported for compatibility. Seedream image generation/editing use the native Ark image generation surface, map Polaris `reference_images` into provider image inputs, and use the same endpoint for both text-to-image and image-guided editing. Polaris now defaults its ByteDance image alias to `doubao-seedream-5.0-lite` while keeping `seedream-4.5` available for compatibility and broader legacy edit behavior. Seedance video uses the Ark async task surface under `contents/generations/tasks`; Polaris maps `first_frame`, `last_frame`, `reference_images`, `reference_videos`, and synced input `audio` onto the provider-native `content` roles, returns an opaque signed `job_id` to callers, and proxies completed video bytes through `/v1/video/generations/:id/content`. The provider treats first-frame scenes, first+last-frame scenes, and multimodal reference scenes as mutually exclusive, so Polaris validates those combinations before the request is sent. Current recommended video aliases point to `doubao-seedance-2.0` / `doubao-seedance-2.0-fast`; legacy `seedance-2.0` aliases remain supported. ByteDance TTS now targets the new-console OpenSpeech V3 TTS 2.0 surface under `/api/v3/tts/unidirectional/sse`, authenticates with `X-Api-Key` from `providers.bytedance.speech_api_key`, selects the provider family through `X-Api-Resource-Id: seed-tts-2.0`, and decodes the streamed base64 audio chunks before replying. The current default ByteDance TTS alias in Polaris is `doubao-tts-2.0`, and the shipped example voices are 2.0 speaker IDs such as `zh_female_vv_uranus_bigtts`. ByteDance provider-backed voice catalog and voice-asset lifecycle are now exposed through `/v1/voices`, `/v1/voices/:id`, `/v1/voices/clones`, `/v1/voices/designs`, `/v1/voices/:id/retrain`, `/v1/voices/:id/activate`, `/v1/voices/:id/archive`, and `/v1/voices/:id/unarchive`. Polaris signs the `speech_saas_prod` control-plane action `ListBigModelTTSTimbres` with `providers.bytedance.access_key_id` / `providers.bytedance.access_key_secret` against `providers.bytedance.control_base_url`, uses `providers.bytedance.speech_api_key` for clone/design/retrain data-plane calls, and normalizes both built-in and custom voice state back into the shared voice resource shape. ByteDance provider deletion remains truthfully unsupported; use the Polaris-local archive/unarchive endpoints when you need a hidden voice without pretending the provider deleted it. ByteDance STT uses the synchronous direct-upload file-recognition 2.0 compatibility path at `/api/v3/auc/bigmodel/recognize/flash`, authenticates with `X-Api-Key` from `providers.bytedance.speech_api_key`, uses `X-Api-Resource-Id: volc.bigasr.auc_turbo`, and normalizes the returned utterance timestamps into the shared transcript surface. ByteDance machine translation is now exposed through `/v1/translations`: Polaris calls `/api/v3/machine_translation/matx_translate`, authenticates with `X-Api-Key` from `providers.bytedance.speech_api_key`, sends `X-Api-Resource-Id: volc.speech.mt`, and aggregates the provider-reported prompt and completion tokens back into the shared `usage` envelope. ByteDance streaming STT 2.0 is now exposed through `/v1/audio/transcriptions/stream`: Polaris creates a signed session over HTTP, then bridges websocket client events onto the provider `wss://openspeech.bytedance.com/api/v3/sauc/bigmodel_async` protocol using `providers.bytedance.app_id`, `providers.bytedance.speech_access_token`, and the current `volc.seedasr.sauc.duration` / `volc.seedasr.sauc.concurrent` resource IDs. The public wire contract stays provider-neutral with `input_audio.append`, `input_audio.commit`, `transcript.delta`, `transcript.segment`, and `transcript.completed` events while the adapter handles the provider binary frame protocol internally. ByteDance simultaneous interpretation 2.0 is now exposed through `/v1/audio/interpreting/sessions`: Polaris creates a signed session over HTTP, then bridges websocket client events onto `wss://openspeech.bytedance.com/api/v4/ast/v2/translate` using `providers.bytedance.app_id`, `providers.bytedance.speech_access_token`, and resource id `volc.service_type.10053`. The adapter follows the current AST requirements for `wav + raw + 16000 Hz` source-audio metadata, streams 80 ms chunks in the live validation path, and normalizes source transcript, translation, translated audio, and usage events back into the shared JSON websocket contract. ByteDance notes are now exposed through `/v1/audio/notes`: Polaris submits jobs to the current notes endpoint, signs the provider task id into a Polaris note id, and returns structured transcript, summary, chapters, action items, QA pairs, and optional translation on poll. ByteDance podcast generation is now exposed through `/v1/audio/podcasts`: Polaris runs the provider websocket generation flow asynchronously, stores the completed asset in the shared async-job cache, and proxies the downloaded audio bytes through `/v1/audio/podcasts/:id/content`. ByteDance audio sessions now support the native OpenSpeech realtime dialogue transport under `wss://openspeech.bytedance.com/api/v3/realtime/dialogue` through `realtime_session.transport: bytedance_dialog`. The default native realtime auth path follows the current official realtime docs and uses `providers.bytedance.app_id` plus `providers.bytedance.speech_access_token`, together with the fixed realtime `resource_id`/`app_key` settings. Polaris also supports explicit `realtime_session.auth: api_key` for accounts that expose realtime API-key auth, but that is not the default assumption. The native path supports `manual` (`push_to_talk`) and `server_vad` (`keep_alive`), sends binary dialogue events instead of the old HTTP cascade, and keeps the public Polaris session contract normalized to `pcm16` / `16000 Hz` by down-sampling provider `24000 Hz pcm_s16le` audio on the gateway side. The legacy cascaded `audio_pipeline` path is still available for compatibility and continues to require `api_key`, `app_id`, and `speech_api_key`.

## Phase 5

### MiniMax

- Auth: bearer token
- Scope: music
- Status: music generation, cover edit, and lyrics implemented in Phase 5A; release-blocking for `v2.1.0`
- Notes: Polaris uses MiniMax `/v1/music_generation` for synchronous music generation and cover workflows, always requests provider-native hex audio, and normalizes the decoded bytes back into the shared binary music response contract. Lyrics generation uses `/v1/lyrics_generation`. Token Plan / global keys must point at `https://api.minimax.io`, while China mainland accounts use `https://api.minimaxi.com`; set `providers.minimax.base_url` explicitly instead of assuming one endpoint fits both. MiniMax can also return business errors inside HTTP `200` responses via `base_resp.status_code`, so Polaris treats non-zero `base_resp.status_code` values as provider failures instead of false successes. Real MiniMax generation can take minutes, so `mode=async` is the recommended Polaris path for long-running jobs and the release-facing configs use a generous provider timeout. MiniMax streaming, stems, and composition-plan helpers are intentionally not exposed through the current adapter because those provider paths are not part of the shipped MiniMax music surface in Polaris.

### ElevenLabs

- Auth: `xi-api-key`
- Scope: music
- Status: music generation, streaming generation, composition plans, and stems implemented in Phase 5A; preview for `v2.1.0` until explicitly opted into live smoke
- Notes: Polaris uses `/v1/music` for synchronous generation, `/v1/music/stream` for streamed generation, `/v1/music/plan` for composition-plan helpers, and `/v1/music/stem-separation` for stems ZIP output. The current ElevenLabs adapter does not expose editing or lyrics helpers through Polaris, so those capabilities are omitted from the model metadata and enforced through capability gating at request time. For `v2.1.0`, ElevenLabs remains in the reference config and codebase, but its live-smoke path follows the matrix-driven `opt_in` policy and runs only when opt-in coverage is enabled globally or for the `elevenlabs` provider.

### Ollama

- Auth: none
- Scope: chat
- Status: implemented in Phase 2B
- Notes: uses native `/api/chat`, not the OpenAI-compatible shim; first request may be slow while models load

## Documentation Rule

Every provider implementation PR should update this file with:

- auth setup
- modality support added in that PR
- any request or response translation quirks
- any operational gotchas operators need to know
