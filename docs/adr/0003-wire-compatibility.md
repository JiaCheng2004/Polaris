# 3. OpenAI-compatible wire contract

Status: Accepted

## Context

Adopters already have code, SDKs, and tooling built against the OpenAI HTTP API.
The cheapest possible migration to a gateway is "change the base URL." A bespoke
protocol would impose rewrite costs that no reliability feature can offset.

## Decision

Where a de-facto standard exists, Polaris is **byte-for-byte wire-compatible**
with it: OpenAI-shaped requests/responses, OpenAI-compatible SSE streaming
(`data:` frames, a final `usage` chunk, `data: [DONE]`), and an OpenAI-compatible
error envelope with a fixed set of `type` codes. Where no standard exists (files,
sessions, music, video), Polaris defines its own but keeps the same conventions.
Compatibility is protected by a golden-transcript suite that asserts recorded
request→response bytes (including SSE, frame by frame) never change, run in every
CI job.

## Consequences

- Migration is a base-URL change; existing OpenAI SDKs work unmodified.
- Every refactor — including the large transport/provider consolidation — is
  fenced by the golden suite, so internal change cannot leak into the wire.
- New capabilities must fit the OpenAI-compatible envelope or be added as
  clearly-namespaced extensions; the contract, not internal convenience, wins.
