# Changelog

This repository is preparing the Polaris `v2.1.0` release. Do not mark `v2.1.0` released until the strict live-smoke matrix for the release set passes. ElevenLabs music remains preview-only unless explicitly opted into smoke validation.

## [2.1.0] - Pending release

### Added

- Multi-provider video support across ByteDance Seedance, OpenAI Sora, and Google Vertex Veo.
- Full-duplex audio sessions over the Polaris session contract with OpenAI and ByteDance cascaded execution.
- Broad sync response caching with `X-Polaris-Cache` markers.
- First-class music support with unified generation, edit, stems, lyrics, plans, async jobs, content download, and Go SDK helpers.
- MiniMax music generation, cover edit, and lyrics adapters.
- ElevenLabs music generation, streaming generation, stems, and plan adapters in preview.
- Public Go SDK coverage for chat, embeddings, images, voice, video, audio sessions, usage, models, and admin keys.
- Committed live-smoke validation assets in `config/polaris.live-smoke.yaml` and `tests/e2e/live_smoke_test.go`.
- Phase 4 close-out record in `spec/phase_4_video_audio_polish/4E_phase_4_hardening_and_acceptance.md`.
- Phase 5 music close-out record in `spec/phase_5_music/5B_phase_5_hardening_and_acceptance.md`.

### Changed

- `GET /v1/usage` now accepts `modality=audio`, matching the shipped runtime logging path.
- Release readiness now includes `make release-check`, `make live-smoke`, and the updated load-validation checklist for music, video, and audio, with ElevenLabs music smoke gated behind explicit preview opt-in.
- Operator docs now describe the `v2.1.0` close-out, explicit MiniMax regional config, and async guidance for long-running music jobs in a single consistent way.

### Fixed

- OpenAI GPT Image inline output is normalized correctly back into Polaris `url|b64_json` responses.
- Google embedding responses correctly handle both `embedContent` and `batchEmbedContents`.
- Audio usage can now be queried the same way as the other shipped modalities.
- Provider HTTP and transport failures now normalize through one shared Polaris error translator with stable subcodes such as `quota_exceeded`.
- Music sync and async timeout paths now preserve `timeout_error / provider_timeout` while directing operators toward `mode=async` or a longer provider timeout for long-running jobs.
