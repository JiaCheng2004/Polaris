# Phase 3A: Multi-User Auth and Admin Keys

**Status:** Implemented

## Summary

This milestone makes `auth.mode: multi-user` real. Phase 1 and Phase 2 proved the gateway with `none` and `static` auth. Phase 3A adds the database-backed key path, the admin authorization layer, and the `/v1/keys` management API from the architecture.

After this step, Polaris must support all three auth modes:

- local mode: `none`
- single-tenant mode: `static`
- managed mode: `multi-user`

## Key Changes

- **DB-backed auth**
  - Implement the `multi-user` branch in `middleware/auth.go`.
  - Lookup keys by SHA-256 hash through the existing store contract.
  - Cache successful key lookups in memory for 60 seconds, keyed by hash.
  - Reject revoked or expired keys before the request reaches handlers.
  - Store key metadata in request context with the same shape already used by the static-auth path.

- **Admin key management**
  - Implement:
    - `POST /v1/keys`
    - `GET /v1/keys`
    - `DELETE /v1/keys/:id`
  - Require `auth.mode: multi-user` plus `is_admin: true` for all three endpoints.
  - Generate plaintext keys only on creation, return them once, hash before persistence, and never log them.
  - Revocation must be soft-delete style: retain rows for audit, set `is_revoked: true`, and block future auth.

- **Key lifecycle behavior**
  - Support optional `owner_id`, `rate_limit`, `allowed_models`, `expires_at`, and `is_admin` fields as already documented in `docs/API_REFERENCE.md`.
  - Update `last_used_at` asynchronously or at the end of the request path without blocking normal request handling.
  - Invalidate the auth cache on key create, revoke, or expiry-sensitive lookups so revoked keys do not remain usable for the cache TTL.

- **Bootstrap and docs**
  - Make `auth.mode: multi-user` a real runtime option in config validation and startup guidance.
  - Update `docs/CONFIGURATION.md`, `docs/API_REFERENCE.md`, and `README.md` so the admin-key surface is described as implemented only after this step lands.

## Public Interfaces Added Or Changed

- New live endpoints:
  - `POST /v1/keys`
  - `GET /v1/keys`
  - `DELETE /v1/keys/:id`
- Existing auth behavior now fully supports:
  - `auth.mode: multi-user`
- Error behavior must match the documented auth/admin errors:
  - `admin_required`
  - `key_revoked`
  - `key_expired`
  - `invalid_api_key`

## Test Plan

- Auth tests:
  - valid multi-user key
  - revoked key
  - expired key
  - cache hit and cache invalidation behavior
  - model permission enforcement under `multi-user`
- Admin endpoint tests:
  - create key returns plaintext once
  - list keys omits plaintext
  - revoke key blocks future auth
  - non-admin key gets `403 admin_required`
  - admin endpoints reject non-`multi-user` runtimes
- Integration tests:
  - PostgreSQL-backed key lifecycle
  - Redis and in-memory cache behavior for auth lookups
  - `last_used_at` update path

## Exit Criteria

`3A` is complete only when:

- `auth.mode: multi-user` works end to end
- admin keys can create, list, and revoke downstream keys
- revoked and expired keys are enforced correctly
- the full key is never persisted or returned after creation
- auth and admin tests pass under `go test -race ./...`

## Non-Goals

`3A` must not add:

- embeddings
- images
- voice
- SDK code
- video or audio work

Those belong to later Phase 3 steps.
