# Guardrails

Polaris's guardrails engine runs operator-configured detectors over request and
response text and takes an action — **observe** (log only), **redact** (mask the
match), or **block** (reject). It operates on decoded, typed content (not wire
bytes), so detection sees the actual text a model would receive or produce.

Guardrails are **disabled by default**. A policy must opt a route in, so enabling
the engine never changes behavior until you write a policy.

## Configuration

```yaml
runtime:
  guardrails:
    enabled: true
    policies:
      - name: redact-response-pii
        phase: response          # request | response | both
        action: redact           # observe | redact | block
        match: { model: "openai/*" }   # optional model glob (default: all)
        detectors:
          pii: { types: [email, phone, credit_card] }
      - name: block-prompt-injection
        phase: request
        action: block
        detectors:
          prompt_injection: { threshold: 0.5 }
```

- **phase** — when the policy applies. Request-phase runs after validation and
  before routing/cache; response-phase runs before caching and writing (so the
  cache stores redacted text).
- **action** — the strongest matching action wins across policies (block >
  redact > observe).
- **match.model** — a glob on the resolved model id; omit to apply to all.

## Detectors

| Detector | Types / options | Notes |
|---|---|---|
| `pii` | `email`, `phone` (E.164), `ssn`, `credit_card` (Luhn), `iban` (mod-97), `ipv4`, `ipv6` | precompiled regex + validators; per-type toggles via `types` |
| `secrets` | `aws_access_key`, `github_pat`, `slack_token`, `jwt`, `pem`, `gcp_sa`, opt-in `high_entropy` | high-entropy tokens are opt-in (noisy) |
| `prompt_injection` | `threshold` (default 0.5) | scored heuristics: multilingual instruction-override, jailbreak markers, role-play, invisible/bidi unicode |
| `content` | `terms: [...]` | operator literal term list (case-insensitive) |

Detection is pure Go and fast: each detector evaluates a 4 KB payload in well
under 1 ms.

## Detection quality (labeled corpus)

Measured by `TestDetectorPrecisionRecall` over a labeled positive/negative
corpus (regenerate by running that test with `-v`):

| Detector | Precision | Recall |
|---|---|---|
| `pii.email` | 1.00 | 1.00 |
| `pii.credit_card` | 1.00 | 1.00 |
| `secrets.aws_access_key` | 1.00 | 1.00 |
| `prompt_injection` | 1.00 | 1.00 |

The `pii` and `secrets` detectors validate matches (Luhn, IBAN mod-97, key
prefixes), so structural false positives are rejected. `prompt_injection` is a
scored heuristic — tune `threshold` per your false-positive tolerance.

## Streaming

For streaming responses, redaction uses a hold-back window (the last 64 runes by
default) so a match that straddles token boundaries is caught before any part of
it is emitted. A `block` policy terminates the stream with an error frame. The
guarantee holds for patterns up to the hold-back length.

## Recommended rollout: observe first

1. **Observe.** Deploy every policy with `action: observe`. Nothing is altered;
   the `polaris_guardrail_evaluations_total{detector,action,phase}` metric shows
   what *would* fire and how often.
2. **Measure.** Watch the metric and `polaris_guardrail_latency_seconds`. Confirm
   the false-positive rate is acceptable for your traffic.
3. **Enforce.** Switch high-confidence policies (validated PII, secrets) to
   `redact` or `block`. Leave heuristic detectors (`prompt_injection`) in
   `observe` or `redact` longer.

## Privacy

Guardrails never log or store matched plaintext. Findings carry the detector,
type, and byte offsets only.

## v1 scope

Text modalities only (chat/completions, messages, responses). Images, audio, and
files receive filename/metadata screening only. Remote detectors (webhook,
llm_judge) and DB-backed policy CRUD extend the config-driven engine documented
here.
