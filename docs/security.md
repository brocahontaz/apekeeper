# Security and reliability controls

ApeKeeper assumes the browser and API are same-origin and PostgreSQL is a
trusted private service. OAuth state is random, stored as a SHA-256 digest in
PostgreSQL, expires after ten minutes, and is atomically deleted on callback,
so it works across restarts and backend instances.

State-changing requests use a strict double-submit CSRF token (`apekeeper_csrf`
and `X-CSRF-Token`). Session cookies are HttpOnly, signed, expiring, and
SameSite=Lax; logout revokes the database session.

CSRF is enabled by default. Authentication, sync, export, and mutation routes
have independent rate/body-limit buckets. Defaults are respectively 20/256 KiB,
30/256 KiB, 30/4 MiB, and 60/256 KiB per minute/client address. Configure the
`*_RATE_LIMIT_PER_MINUTE` and `*_MAX_BODY_BYTES` variables (limits are bounded
to 1..10000 and 1..16777216). The legacy global settings remain supported.
`/healthz` is a liveness/database check; `/readyz` separately checks database,
Blizzard, background, scheduler, notification, and shutdown state with bounded
checks. Expired OAuth states are removed by the daily sweeper.

Sensitive credentials, cookies, authorization headers, webhook URLs, and
upstream values are redacted centrally. Security-relevant mutations write
audit events without token or payload data. Never put secrets in fixtures,
URLs, or logs.
