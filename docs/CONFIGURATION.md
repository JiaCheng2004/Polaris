# Polaris Configuration

`config/polaris.yaml` is the local-development default. `config/polaris.example.yaml` is the full reference file that documents the intended production topology and the full provider catalog described in `BLUEPRINT.md`.

## Precedence

Configuration is resolved in this order:

1. CLI flags
2. Environment variables
3. YAML config
4. Built-in defaults

Supported CLI flags from the blueprint:

- `--config <path>`
- `--port <port>`
- `--log-level <level>`

## File Roles

- `config/polaris.yaml`: single-binary development defaults, centered on SQLite and in-memory cache.
- `config/polaris.example.yaml`: full reference config for real deployments and future phases.

## Sections

- `server`: listener address and request/shutdown timeouts.
- `auth`: auth mode, static keys, external signed-claim auth, legacy multi-user compatibility, and virtual-key bootstrap settings.
- `store`: backing database driver, DSN, and async log writer tuning.
- `cache`: driver selection, Redis connection settings, rate limiting, and response-cache settings.
- `providers`: provider credentials, base URLs, retry policy, and model catalog.
- `routing`: static aliases, capability-driven selector aliases, and provider failover order.
- `control_plane`: project / virtual-key control-plane enablement.
- `tools`: local tool registration metadata for the tool runtime.
- `mcp`: MCP broker enablement.
- `observability`: metrics, structured logging, tracing, and audit-event settings.

The current runtime intentionally keeps `control_plane`, `tools`, and `mcp` YAML small. Detailed projects, virtual keys, policies, budgets, toolsets, and MCP bindings are managed through the database-backed control-plane API instead of being preloaded from YAML. Token-accounting provenance is always emitted by the runtime and does not currently have a separate `token_accounting` YAML section.

## Secret Handling

- Provider credentials must come from environment variables via `${VAR_NAME}` references.
- Do not commit plaintext API keys, admin keys, TLS material, or local `.env` files.
- Static and admin gateway keys must be stored as hashes, not plaintext values.
- `auth.external.shared_secret` must come from an environment variable such as `${POLARIS_EXTERNAL_AUTH_SECRET}` and must not be committed as a literal.

## Authentication Modes

Choose the smallest auth mode that matches the deployment:

- `auth.mode: none`: local-only development path. Polaris accepts every request and applies wildcard model access.
- `auth.mode: static`: simple fixed API keys defined in YAML. Good for private demos or one trusted service.
- `auth.mode: external`: bring-your-own-auth path. Your app owns Google OAuth, SMS OTP, SSO, sessions, and user lifecycle; Polaris verifies signed request claims.
- `auth.mode: virtual_keys`: Polaris-owned production key plane with projects, policies, budgets, toolsets, MCP bindings, and audit records.
- `auth.mode: multi-user`: compatibility path for older database-backed `api_keys` rows.

External auth uses the built-in `signed_headers` provider:

```yaml
auth:
  mode: external
  external:
    provider: signed_headers
    shared_secret: ${POLARIS_EXTERNAL_AUTH_SECRET}
    max_clock_skew: 60s
    cache_ttl: 60s
```

Equivalent environment overrides are:

- `POLARIS_AUTH_MODE=external`
- `POLARIS_EXTERNAL_AUTH_PROVIDER=signed_headers`
- `POLARIS_EXTERNAL_AUTH_SECRET=<shared-secret>`
- `POLARIS_EXTERNAL_AUTH_MAX_CLOCK_SKEW=60s`
- `POLARIS_EXTERNAL_AUTH_CACHE_TTL=60s`

The host platform sends:

- `X-Polaris-External-Auth`: base64url JSON claims
- `X-Polaris-External-Auth-Timestamp`: Unix seconds
- `X-Polaris-External-Auth-Signature`: `v1=<hex hmac-sha256>` over `timestamp + "\n" + encoded_claims`

Required claim:

- `sub`: stable external subject/user ID

Optional policy claims:

- `project_id`
- `key_id`
- `key_prefix`
- `is_admin`
- `rate_limit`
- `allowed_models`
- `allowed_modalities`
- `allowed_toolsets`
- `allowed_mcp_bindings`
- `policy_models`
- `policy_modalities`
- `policy_toolsets`
- `policy_mcp_bindings`
- `expires_at`

Use `external` when Polaris is embedded behind a product backend. Use `virtual_keys` when Polaris itself should be the API key and project boundary.

## Current Phase Guidance

Phase 5 music hardening is current. The shipped runtime supports:

- `providers.openai`
- `providers.anthropic`
- `providers.deepseek`
- `providers.google`
- `providers.google-vertex`
- `providers.minimax`
- `providers.elevenlabs`
- `providers.xai`
- `providers.openrouter`
- `providers.together`
- `providers.groq`
- `providers.fireworks`
- `providers.featherless`
- `providers.moonshot`
- `providers.glm`
- `providers.mistral`
- `providers.bedrock`
- `providers.nvidia`
- `providers.replicate`
- `providers.qwen`
- `providers.bytedance`
- `providers.ollama`
- `store.driver: sqlite|postgres`
- `cache.driver: memory|redis`
- `routing.aliases`, `routing.selectors`, and `routing.fallbacks`
- `control_plane.enabled`
- `tools.enabled`
- `mcp.enabled`
- `observability.metrics`
- `observability.traces`
- `observability.audit`
- `auth.mode: virtual_keys`
- `auth.mode: external`
- compatibility `auth.mode: multi-user`
- control-plane management via `/v1/projects`, `/v1/virtual_keys`, `/v1/policies`, `/v1/budgets`, `/v1/tools`, `/v1/toolsets`, and `/v1/mcp/bindings`
- legacy key management compatibility via `/v1/keys`

`config/polaris.yaml` remains the simplest local path: SQLite plus the in-memory cache. `config/polaris.example.yaml` documents the distributed runtime and the full shipped provider matrix across chat, embeddings, images, voice, video, audio sessions, and music.

`config/polaris.live-smoke.yaml` is the committed release-validation config. It is designed for env-driven credential injection plus the real-provider smoke matrix in `tests/e2e/live_smoke_test.go`.

The chat-first expansion families OpenRouter, Together, Groq, Fireworks, Featherless, Moonshot, GLM, and Mistral all use the shared provider-common OpenAI-compatible adapter base. Their Polaris config shape is intentionally uniform:

- `api_key`
- optional `base_url`
- `timeout`
- `retry`
- `models`

The provider model matrix lives in `internal/provider/catalog/models.yaml`, is embedded into the binary at build time, and adds `/v1/models` metadata for provider variants, canonical model families, human aliases, and routing hints such as `cost_tier` and `latency_tier` for configured models. Exact `provider/model` requests always execute directly; family IDs and family aliases resolve to one enabled provider variant by deterministic policy.

You can validate configured model metadata and routing coverage without provider credentials:

- `make verify-models`
- `make verify-models-json`

The verification report checks configured canonical models, aliases, and selectors against the embedded matrix and marks each configured route as `strict`, `opt_in`, or `skipped`.

`routing.selectors` adds intent-style aliases that resolve to the best currently enabled model matching a required modality plus selector capabilities. Selectors are deterministic and can filter by provider order, provider exclusions, release status, verification class, preferred models, `cost_tier`, and `latency_tier`.

Model-taking API requests may also include a request-level `routing` object. Polaris merges those per-request hints with the static selector/config policy when the caller uses a family ID, family alias, or selector alias. Exact `provider/model` requests still bypass routing.

Amazon Bedrock is intentionally separate from the OpenAI-compatible family. Configure:

- `providers.bedrock.access_key_id`
- `providers.bedrock.access_key_secret`
- optional `providers.bedrock.session_token`
- `providers.bedrock.location`
- optional `providers.bedrock.base_url`
- `models`

The Bedrock adapter uses the native runtime `Converse` / `ConverseStream` APIs for chat, the native `InvokeModel` path for Titan embeddings, signs every request with SigV4, and expects official Bedrock model IDs such as `amazon.nova-2-lite-v1:0` or `amazon.titan-embed-text-v2:0`.

NVIDIA NIM uses the shared OpenAI-compatible provider base. Configure:

- `providers.nvidia.api_key`
- optional `providers.nvidia.base_url`
- `models`

For official NVIDIA-hosted models, prefer the official wire model IDs in config, for example `nvidia/NVIDIA-Nemotron-Nano-9B-v2` for chat and `nvidia/llama-nemotron-embed-1b-v2` for embeddings. Polaris also treats the short form without the leading `nvidia/` as a local alias when you configure the model that way.

Replicate is currently a native async video provider family. Configure:

- `providers.replicate.api_key`
- optional `providers.replicate.base_url`
- `models`

Replicate models should use official `owner/model` identifiers in config, for example `minimax/video-01`. The first Polaris scope is the Predictions API for async video jobs, so Replicate models should currently be configured with `modality: video`.

For `v2.1.0`, MiniMax music is part of the release-blocking smoke set. Opt-in provider paths, including ElevenLabs music, are classified by the embedded provider model matrix and run only when `POLARIS_LIVE_SMOKE_INCLUDE_OPT_IN=1` is set or the matching provider-specific env such as `POLARIS_LIVE_SMOKE_PROVIDER_ELEVENLABS=1` is set.

`modality: audio` is now a runnable model type. Audio models are Polaris session definitions rather than a raw upstream endpoint, and they require:

- `capabilities: [audio_input, audio_output]`
- either:
- `audio_pipeline.chat_model`
- `audio_pipeline.stt_model`
- `audio_pipeline.tts_model`
- or:
- `realtime_session.transport`
- `realtime_session.model`
- optional `voices`
- optional `session_ttl`

Current runtime limits for audio sessions:

- only `pcm16` input/output audio is supported
- only `16000` Hz is supported
- OpenAI-backed audio sessions support `manual` and `server_vad`
- ByteDance native realtime audio sessions support `manual` and `server_vad`
- ByteDance native realtime audio sessions connect to `wss://openspeech.bytedance.com/api/v3/realtime/dialogue`, request `pcm_s16le` output from the provider, and Polaris down-samples the provider's `24000 Hz` PCM back to the shared `16000 Hz pcm16` contract
- `audio_pipeline` remains available for provider-backed cascaded compatibility models; ByteDance cascaded sessions still support `manual` only

`cache.response_cache` is also live in the runtime. Current cache boundaries are:

- semantic cache: non-streaming chat only
- exact-match cache: embeddings, images, TTS, STT, and synchronous music generation/edit/stems/lyrics/plan calls
- bypass: streaming chat, video endpoints, and audio sessions

Preferred auth mode for production is `virtual_keys`. In that mode:

- bearer tokens are Polaris virtual keys stored in the database
- `auth.bootstrap_admin_key_hash` is required when `auth.mode: virtual_keys`
- the bootstrap admin key can manage control-plane endpoints, but it is not valid for inference endpoints
- project policies gate models, modalities, toolsets, and MCP bindings
- hard budgets can block requests once the current budget window is already exceeded

Preferred auth mode for embedding Polaris behind another platform is `external`. In that mode:

- the platform handles OAuth, SMS OTP, SSO, users, sessions, and tenancy
- Polaris verifies signed headers from that trusted platform
- signed claims become the same request `AuthContext` used by rate limits, model policy, modality policy, tools, MCP, audit, and budgets
- `is_admin: true` in the signed claims is required for control-plane endpoints

`auth.mode: multi-user` remains as a compatibility path for older database-backed key rows and the legacy `/v1/keys` surface.

When `control_plane.enabled` is true, Polaris exposes:

- `/v1/projects`
- `/v1/virtual_keys`
- `/v1/policies`
- `/v1/budgets`
- `/v1/tools`
- `/v1/toolsets`
- `/v1/mcp/bindings`

`/v1/keys` remains implemented as a compatibility facade. In `virtual_keys` mode it issues Polaris virtual keys with the legacy response shape; in `multi-user` mode it still uses the older `api_keys` rows.

When `tools.enabled` is true, local tool implementations registered in `tools.local` can be attached to toolsets and exposed through MCP bindings. Polaris does not upload arbitrary code; it only executes implementations already registered in the runtime.

When `mcp.enabled` is true, Polaris exposes streamable HTTP MCP broker paths under `/mcp/:binding_id`. Current binding kinds are:

- `upstream_proxy`
- `local_toolset`

When `observability.traces.enabled` is true, Polaris exports OTLP traces using:

- `observability.traces.endpoint`
- `observability.traces.insecure`
- `observability.traces.service_name`
- `observability.traces.sample_ratio`

The emitted trace tree is deeper than a single HTTP span. Polaris adds child spans for auth lookup, rate-limit and budget checks, cache lookup/store, provider HTTP calls, fallback attempts, and MCP/tool execution, while keeping prompts, raw bodies, and secret material out of trace attributes.

When `observability.audit.enabled` is true, Polaris records audit events for project, virtual-key, policy, budget, tool, toolset, and MCP-binding changes. Audit writes are buffered off the hot path.

When `cache.driver` is `redis`, set `cache.url` to a `redis://` connection URL. The reference config does this through `${REDIS_URL}` so the production and dev stack files can inject Redis without committing environment-specific values.

When ByteDance TTS is enabled on the new Doubao Speech control plane, set `providers.bytedance.speech_api_key`. Polaris uses that field as the OpenSpeech `X-Api-Key` credential for the V3 Text-to-Speech 2.0 API and expects ByteDance 2.0 speaker IDs such as `zh_female_vv_uranus_bigtts`.

When provider-backed ByteDance voice catalog listing is enabled, set:

- `providers.bytedance.access_key_id`
- `providers.bytedance.access_key_secret`

Polaris uses those control-plane credentials to sign `speech_saas_prod` OpenAPI requests against `providers.bytedance.control_base_url`, which defaults to `https://open.volcengineapi.com`. The current provider-backed catalog path powers `GET /v1/voices?scope=provider&provider=bytedance` and returns built-in 2.0 voice metadata from `ListBigModelTTSTimbres`.

When ByteDance voice assets are enabled, set:

- `providers.bytedance.speech_api_key`
- `providers.bytedance.access_key_id`
- `providers.bytedance.access_key_secret`

Polaris uses `speech_api_key` for data-plane clone, design, and retrain requests, and uses the signed control-plane credentials for custom-voice status lookup. The current ByteDance voice-asset surface powers:

- `GET /v1/voices?scope=provider&type=custom|all`
- `GET /v1/voices/:id`
- `POST /v1/voices/:id/archive`
- `POST /v1/voices/:id/unarchive`
- `POST /v1/voices/clones`
- `POST /v1/voices/designs`
- `POST /v1/voices/:id/retrain`
- `POST /v1/voices/:id/activate`

`DELETE /v1/voices/:id` remains provider deletion semantics. For ByteDance, Polaris keeps that truthful and returns unsupported because the provider delete path is not available; use archive/unarchive for the local hide workflow.

When ByteDance STT is enabled, set `providers.bytedance.speech_api_key`. Polaris uses that field as the OpenSpeech `X-Api-Key` credential for the direct-upload file-recognition 2.0 flash endpoint and sends the current synchronous resource ID `volc.bigasr.auc_turbo`.

When ByteDance machine translation is enabled, set `providers.bytedance.speech_api_key`. Polaris uses that field as the OpenSpeech `X-Api-Key` credential for the new-console machine translation endpoint at `/api/v3/machine_translation/matx_translate` and sends `X-Api-Resource-Id: volc.speech.mt`. Configure translation models with `modality: translation`.

When ByteDance streaming transcription is enabled, set:

- `providers.bytedance.app_id`
- `providers.bytedance.speech_access_token`

The streaming model itself should declare `modality: voice`, `capabilities: [streaming]`, a `session_ttl`, and a websocket endpoint such as `wss://openspeech.bytedance.com/api/v3/sauc/bigmodel_async`. Polaris keeps the public contract provider-neutral on `/v1/audio/transcriptions/stream`, while the ByteDance adapter maps model aliases onto the current resource IDs:

- `doubao-streaming-asr-2.0` -> `volc.seedasr.sauc.duration`
- `doubao-streaming-asr-2.0-concurrent` -> `volc.seedasr.sauc.concurrent`

Current runtime limits remain `pcm16` at `16000 Hz`.

When ByteDance simultaneous interpretation is enabled, set:

- `providers.bytedance.app_id`
- `providers.bytedance.speech_access_token`

The interpreting model itself should declare `modality: interpreting`, `capabilities: [audio_input, audio_output]`, a `session_ttl`, and a websocket endpoint such as `wss://openspeech.bytedance.com/api/v4/ast/v2/translate`. Polaris keeps the public contract provider-neutral on `/v1/audio/interpreting/sessions`, while the ByteDance adapter maps that surface onto the current AST resource id:

- `doubao-interpreting-2.0` -> `volc.service_type.10053`

Current runtime limits are:

- input must be `16000 Hz`
- input format may be `pcm16` or `wav`
- Polaris normalizes ByteDance source-audio metadata to `wav + raw + 16000 Hz`
- `speech_to_text` sessions do not require target audio
- `speech_to_speech` sessions support `pcm16` output at `16000 Hz` or `ogg_opus` output at `48000 Hz`

When ByteDance notes are enabled, set:

- `providers.bytedance.app_id`
- `providers.bytedance.speech_access_token`

The notes model itself should declare `modality: notes`, `capabilities: [audio_notes]`, and an HTTP submit endpoint such as `https://openspeech.bytedance.com/api/v3/auc/lark/submit`. Polaris signs the provider task id into the public `/v1/audio/notes/:id` identifier and polls the matching query endpoint with the same app-scoped credentials.

When ByteDance podcast generation is enabled, set:

- `providers.bytedance.app_id`
- `providers.bytedance.speech_access_token`

The podcast model itself should declare `modality: podcast`, `capabilities: [podcast_generation]`, and a websocket endpoint such as `wss://openspeech.bytedance.com/api/v3/sami/podcasttts`. Polaris runs podcast generation as a cache-backed async job on `/v1/audio/podcasts`, then proxies the completed audio bytes through `/v1/audio/podcasts/:id/content`.

When ByteDance native realtime audio sessions are enabled through `realtime_session.transport: bytedance_dialog`, Polaris defaults to the current official realtime auth path:

- `providers.bytedance.app_id`
- `providers.bytedance.speech_access_token`

If your ByteDance account exposes a newer realtime API Key control plane, you can opt into it explicitly with:

- `realtime_session.auth: api_key`
- `providers.bytedance.speech_api_key`

The audio session model itself should define:

- `realtime_session.transport: bytedance_dialog`
- optional `realtime_session.auth` (`access_token` by default, `api_key` when the account supports realtime API-key auth)
- optional `realtime_session.url` (defaults to `wss://openspeech.bytedance.com/api/v3/realtime/dialogue`)
- optional `realtime_session.resource_id` (defaults to `volc.speech.dialog`)
- optional `realtime_session.app_key` (defaults to `PlgvMymc7f3tQnJ6`)
- `realtime_session.model` (`1.2.1.1` for O2.0 or `2.2.0.0` for SC2.0)

When ByteDance audio sessions use the legacy cascaded `audio_pipeline` path instead, all three fields are required together:

- `providers.bytedance.api_key`
- `providers.bytedance.app_id`
- `providers.bytedance.speech_api_key`

When `providers.google-vertex` is enabled, set `project_id`, `location`, and `secret_key` in YAML and provide Google ADC credentials in the runtime environment. `secret_key` is used for signed Polaris video job IDs; provider API auth still comes from ADC, not from a YAML API key.

When OpenAI native realtime audio sessions are enabled, the audio model should use `realtime_session.transport: openai_realtime`. Older cascaded compatibility sessions can still use `audio_pipeline` pointing at an OpenAI chat model plus the STT/TTS models you want Polaris to use for the session turns.

When MiniMax music is enabled, set `providers.minimax.base_url` explicitly to either `https://api.minimax.io` or `https://api.minimaxi.com`. Token Plan / global accounts typically use `https://api.minimax.io`; China mainland accounts use `https://api.minimaxi.com`. Real MiniMax generation can take minutes, so the release-facing configs use a `10m` timeout and production callers should prefer `mode=async` for long-running music jobs.

## Hot Reload Behavior

Phase 2 hot reload updates the runtime routing layer without restarting the HTTP server. Reloadable settings include:

- provider credentials, base URLs, retry policy, and model catalog
- `routing.aliases`
- `routing.fallbacks`
- auth mode and static-key configuration
- rate-limit settings
- log level

Hot reload intentionally rejects changes to non-runtime infrastructure settings. A restart is required for:

- `server.*`
- `store.*`
- cache connection settings such as `cache.driver` and `cache.url`
- `logging.format`
- `observability.metrics.path`

## Metrics

Set `observability.metrics.enabled: true` to expose Prometheus metrics. The path is configured by `observability.metrics.path` at startup and defaults to `/metrics`.

The stable metric labels are:

- `polaris_requests_total`: `interface_family`, `model`, `modality`, `status`, `provider`
- `polaris_request_duration_seconds`: `interface_family`, `model`, `modality`, `provider`
- `polaris_provider_latency_seconds`: `model`, `provider`
- `polaris_tokens_total`: `model`, `provider`, `direction`, `token_source`

## Example Runtime Shapes

Recommended production/auth baseline:

- `auth.mode: virtual_keys`
- `auth.bootstrap_admin_key_hash: ${POLARIS_BOOTSTRAP_ADMIN_KEY_HASH}`
- `control_plane.enabled: true`
- `tools.enabled: true`
- `mcp.enabled: true`
- `observability.audit.enabled: true`
- `observability.traces.enabled: true` when OTLP export is available

Recommended embedded-platform auth baseline:

- `auth.mode: external`
- `auth.external.provider: signed_headers`
- `auth.external.shared_secret: ${POLARIS_EXTERNAL_AUTH_SECRET}`
- `control_plane.enabled: true` only if the upstream platform signs `is_admin: true` for trusted operators

Compatibility/local options still exist:

- `auth.mode: none` for local development
- `auth.mode: static` for simple fixed-key deployments
- `auth.mode: multi-user` only when you still need the older key-row model

## Close-Out Commands

Use these during the `v2.1.0` release close-out:

- `make release-check`: repo-local validation gate
- `make stack-validate STACK=local|prod|dev`: validate Compose config without rendering interpolated values
- `make stack-config STACK=local|prod|dev`: render Compose config for local debugging
- `make live-smoke`: env-gated live provider smoke matrix
- `POLARIS_LIVE_SMOKE=1 POLARIS_LIVE_SMOKE_STRICT=1 make live-smoke`: strict release blocker for matrix-classified `strict` models
- `POLARIS_LIVE_SMOKE=1 POLARIS_LIVE_SMOKE_INCLUDE_OPT_IN=1 make live-smoke`: include matrix-classified `opt_in` models
- `POLARIS_LIVE_SMOKE=1 POLARIS_LIVE_SMOKE_PROVIDER_ELEVENLABS=1 make live-smoke`: include only the ElevenLabs opt-in slice
- `make load-check`: local load validation with SQLite and memory cache

`make release-check` uses `stack-validate`, not `stack-config`, so shared CI logs do not print environment-expanded Compose configuration. Run `stack-config` only when you intentionally need a local rendered config.

The live-smoke targets default `LIVE_SMOKE_TIMEOUT` to `45m`; override it only when running a deliberately smaller provider slice. Provider quota, billing, and entitlement failures are real validation results for release-blocking provider paths, but the default open-source contributor gate does not require a production Postgres/Redis load environment.
