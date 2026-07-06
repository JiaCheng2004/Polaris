# Security Policy

## Supported versions

Security fixes are provided for the latest minor release line.

| Version | Supported |
|---------|-----------|
| 1.0.x   | ✅        |
| < 1.0   | ❌        |

## Reporting a vulnerability

**Please do not open a public issue for security vulnerabilities.**

Report privately through GitHub's [private vulnerability reporting](https://docs.github.com/en/code-security/security-advisories/guidance-on-reporting-and-writing-information-about-vulnerabilities/privately-reporting-a-security-vulnerability)
("Report a vulnerability" under the repository's **Security** tab), or email the
maintainers if that is unavailable.

Please include:

- a description of the issue and its impact,
- steps to reproduce (a minimal proof-of-concept if possible),
- affected version(s) and configuration, and
- any suggested remediation.

## What to expect

- **Acknowledgement** within 3 business days.
- An initial **assessment** (severity, affected versions) within 10 business days.
- Coordinated disclosure: we will agree on a timeline and credit reporters who
  wish to be named. Fixes are released as patch versions with an advisory.

## Scope

In scope: the gateway (`cmd/polaris`, `internal/**`), the Go SDK (`pkg/client`),
the published container image, and release artifacts. Out of scope: issues in
third-party providers reached through the gateway, and misconfigurations that
contradict the documented security guidance (for example, disabling auth or
exposing admin endpoints publicly).

## Hardening references

See `docs/AUTHENTICATION.md` and `docs/CONFIGURATION.md` for auth modes, API-key
hashing, rate-limit cost controls, and CORS guidance.
