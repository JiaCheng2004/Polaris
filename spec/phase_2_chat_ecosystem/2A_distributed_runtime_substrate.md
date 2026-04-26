# Phase 2A: Distributed Runtime Substrate

**Status:** Implemented

## Summary

This milestone makes the distributed runtime path real. Phase 1 already proved the gateway with SQLite and the in-memory cache. Phase 2A adds the production-grade backends: PostgreSQL for storage and Redis for cache-backed rate limiting.

After this step, Polaris must support both:

- local mode: SQLite + in-memory cache
- distributed mode: PostgreSQL + Redis

## Key Changes

- **PostgreSQL store**
  - Implement `internal/store/postgres/postgres.go` against the existing `store.Store` interface.
  - Use `pgx`-backed SQL access as approved by the architecture.
  - Reuse the same logical schema and behavior as the SQLite store:
    - API key CRUD
    - request-log writes and batched writes
    - usage aggregation by day
    - usage aggregation by model
    - log retention purge
    - migration application
    - health checks
  - Keep SQL behavior aligned with the SQLite semantics so handler and middleware code do not branch by store type.

- **Redis cache**
  - Implement `internal/store/cache/redis.go` against the existing `cache.Cache` interface.
  - Support:
    - `Get`
    - `Set`
    - `Increment`
    - `Ping`
    - `Close`
  - Preserve the current rate-limit middleware contract so callers do not know whether the cache is memory or Redis.
  - TTL behavior must match the sliding-window rate-limit requirements already used by the middleware.

- **Bootstrap and runtime wiring**
  - Extend `cmd/polaris/main.go` bootstrap so:
    - `store.driver=postgres` is real
    - `cache.driver=redis` is real
  - Keep the current fallback behavior:
    - local config stays SQLite + memory
    - production-shaped stacks can switch to PostgreSQL + Redis entirely via config and environment
  - The bootstrap path must remain explicit and deterministic. Do not add hidden driver inference.

- **Deployment and command surface**
  - Make `STACK=prod` and `STACK=dev` operationally valid, not just renderable.
  - Keep `STACK=local` as the default developer path.
  - Keep the stable developer entrypoints unchanged:
    - `make dev`
    - `make build`
    - `make test`
    - `make stack-up STACK=...`

## Public Interfaces Added Or Changed

- no new HTTP endpoints
- runtime config now fully supports:
  - `store.driver=postgres`
  - `cache.driver=redis`
- `STACK=prod` and `STACK=dev` become real runtime targets rather than scaffolding-only Compose definitions

## Test Plan

- PostgreSQL store tests:
  - API key create/get/list/delete
  - request-log batch writes
  - `GetUsage`
  - `GetUsageByModel`
  - purge behavior
  - migration and `Ping`
- Redis cache tests:
  - `Get` / `Set`
  - `Increment`
  - TTL expiry behavior
  - concurrent correctness for rate-limit usage
- Bootstrap tests:
  - `postgres` store bootstraps cleanly
  - `redis` cache bootstraps cleanly
  - invalid DSN / Redis URL surfaces clear startup errors
- Stack validation:
  - `make stack-validate STACK=prod`
  - `make stack-validate STACK=dev`
  - smoke boot with the production-shaped stack

## Exit Criteria

`2A` is complete only when:

- PostgreSQL satisfies the existing `Store` contract
- Redis satisfies the existing `Cache` contract
- the runtime can boot successfully with both drivers
- the production and dev stacks are operational, not just syntactically present
- all store/cache tests pass under `go test -race ./...`

## Non-Goals

`2A` must not add:

- new providers
- failover logic
- hot reload
- metrics
- multi-user auth

Those are handled by later Phase 2 steps.
