# Phase 2F: Hot Reload Runtime

**Status:** Implemented

## Summary

This milestone adds safe runtime config reload without restarting the HTTP server. Reload providers, models, aliases, fallbacks, and rate-limit settings in place, but do not restart the server and do not reinitialize the store connection.

The implementation should be concurrency-safe first. Convenience comes second.

## Key Changes

- **Reload triggers**
  - Implement `internal/config/watcher.go`.
  - Support both:
    - `SIGHUP`
    - file-change detection via `fsnotify`
  - Deduplicate or debounce rapid file-change events so one config save does not trigger multiple overlapping reloads.

- **Reloadable runtime state**
  - Introduce a reloadable runtime holder for:
    - provider registry
    - aliases
    - fallback rules
    - effective rate-limit configuration
  - Use atomic swap semantics for the reloadable state.
  - Do not mutate live maps in place while requests are reading them.

- **Non-reloadable runtime state**
  - Keep these fixed across reloads:
    - HTTP server instance
    - SQLite / PostgreSQL store connection
    - memory / Redis cache client
  - If a config reload attempts to change a non-reloadable connection target:
    - define the behavior explicitly
    - recommended default: reject the reload and keep the current runtime state unchanged

- **Error handling**
  - On invalid config during reload:
    - log the validation failure
    - keep the previous runtime state active
    - do not partially apply changes

## Public Interfaces Added Or Changed

- no new HTTP endpoints
- runtime behavior changes:
  - `SIGHUP` triggers config reload
  - file changes trigger config reload
  - new requests see the updated registry, aliases, fallbacks, and rate limits without server restart

## Test Plan

- reload tests:
  - valid config reload via direct reload function
  - invalid config reload leaves old state intact
  - alias and fallback changes become visible after reload
  - rate-limit changes become visible after reload
- concurrency tests:
  - reload during active request traffic
  - no data races under `go test -race ./...`
- signal / watcher tests:
  - `SIGHUP` path
  - file-change path

## Exit Criteria

`2F` is complete only when:

- config reload works without HTTP server restart
- in-flight requests complete successfully during reload
- invalid reloads leave the prior runtime untouched
- race tests pass for the reload path

## Assumptions

- non-reloadable connection changes should be rejected in Phase 2 rather than forcing partial reconnection logic into this milestone
- if that assumption changes, it should be treated as a design decision and re-planned explicitly
