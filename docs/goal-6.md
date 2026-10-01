# Goal Prompt: Security and Reliability Hardening

Harden ApeKeeper's authentication, authorization, request handling, and operational safety. Inspect OAuth flow, session storage, cookies, middleware, state-changing endpoints, logging, configuration validation, webhook handling, scheduler lifecycle, database access, CI, and existing security tests. Preserve the current Battle.net login experience.

Deliver these outcomes:

1. Persist or otherwise safely distribute OAuth state so login remains secure across backend restarts and multiple instances. Expire and consume state exactly once.
2. Add CSRF protection for state-changing requests, including logout, sync triggers, bulk actions, and future mutations, while keeping same-origin SPA requests functional.
3. Add configurable rate limits and bounded request bodies for authentication, sync, export, and mutation endpoints. Return safe, consistent errors.
4. Review session cookies and session lifecycle: secure defaults, explicit SameSite behavior, expiration, revocation, and rotation when appropriate.
5. Ensure logs and errors redact access tokens, refresh tokens, webhook URLs, cookies, authorization headers, and sensitive upstream payloads.
6. Improve health/readiness checks and graceful shutdown for PostgreSQL, Blizzard API access, scheduler jobs, notification delivery, and background goroutines.
7. Add security-focused audit events for sign-in failures, role changes, sync triggers, exports, and administrative mutations.

Acceptance criteria:

- Threat-model assumptions and configuration changes are documented.
- Tests cover invalid/replayed/expired OAuth state, CSRF failures, rate-limit boundaries, authorization bypass attempts, cookie attributes, redaction, and shutdown.
- Security controls fail closed without breaking legitimate same-origin behavior.
- Timeouts and limits are bounded and configurable with safe defaults.
- No secrets appear in test fixtures, generated files, logs, or API responses.
- Run backend tests, vet, formatting, frontend tests/build, and any security/static checks available in CI.

Constraints:

- Do not invent cryptography; use standard-library or established project mechanisms.
- Do not weaken authorization to simplify frontend behavior.
- Do not add external infrastructure requirements without documenting local development impact.
- Do not commit, push, or alter unrelated files.
