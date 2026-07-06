<!-- Thanks for contributing to Polaris! -->

## Summary

<!-- What does this change do, and why? -->

## Type of change

- [ ] Bug fix
- [ ] New provider (one provider per PR)
- [ ] New feature
- [ ] Docs
- [ ] Refactor / internal

## Checklist

- [ ] `make test` (with `-race`) passes
- [ ] `make lint` and `make check-layering` pass
- [ ] `make contract-check` / golden wire-compat pass (no unintended `/v1` change)
- [ ] Endpoint changes update `docs/API_REFERENCE.md` in this PR
- [ ] Config changes update `docs/CONFIGURATION.md` and the schemas
- [ ] New provider includes `httptest`-based unit tests and updates `docs/PROVIDERS.md`
- [ ] No secrets committed; provider keys stay in env vars

## Related issues

<!-- Fixes #123 -->
