# ApeKeeper

ApeKeeper is the GM and officer control room for **Ape Enclosure**: a compact guild roster, progression, and sync ledger rather than a generic dashboard.

## Local development

This repository supports local development only. Production deployment, reverse proxies, TLS, and production Compose configuration are intentionally out of scope; they will live in a separate deployment repository.

Copy the example environment file and set the required values:

```sh
cp .env.example .env
```

`GUILD_REALM` is required for the initial Ape Enclosure bootstrap (use the Blizzard realm slug, such as `area-52`). Blizzard OAuth credentials are platform-global. After sign-in, `GET /api/guilds` lists memberships and `POST /api/guilds/select` safely switches the HttpOnly-selected guild. The optional `DISCORD_WEBHOOK_URL` remains the legacy bootstrap default; new guild settings are stored per guild.

### Native backend and frontend

Run PostgreSQL in Compose and the application services on the host. Use two terminals because the backend development server keeps the first terminal occupied.

In terminal 1, start the database, migrate it, and run the backend:

```sh
set -a; . ./.env; set +a
docker compose up -d db
make migrate
make run                 # backend on :8080
```

In terminal 2, run the SPA:

```sh
make frontend-dev        # SPA on :5173
```

### All services in Compose

Build and run the separate development images:

```sh
make dev-up
# equivalent to: docker compose up --build
```

Use `make dev-down` to stop the stack and `make dev-logs` to follow service logs.

Both workflows expose the frontend at http://localhost:5173, the backend at http://localhost:8080, and PostgreSQL at `localhost:5432`. In Compose, the Vite frontend proxies API requests to the separate backend service. The Compose `frontend` service builds the development image from `frontend/Dockerfile.dev`; the production image from `frontend/Dockerfile` is built by CI instead. The backend image does not package frontend assets; `STATIC_DIR` remains an optional native fallback only.

## Container images

CI builds and publishes `ghcr.io/brocahontaz/apekeeper-backend` and `ghcr.io/brocahontaz/apekeeper-frontend` on pushes to `main` and version tags (`v*`); pull requests build without publishing. Pull requests and pushes to `main` also run the `ci` workflow's quality gates: backend (gofmt, go vet, tests against a PostgreSQL service) and frontend (svelte-check, vitest, build). The frontend image serves the built SPA with nginx and proxies `/api` and `/healthz` to the backend origin set in `BACKEND_ORIGIN` (default `http://backend:8080`). After the first publish, flip each GitHub package to public in its package settings and optionally link it to this repository — GitHub provides no API or workflow mechanism for this.

## Architecture

Blizzard APIs → sync service → PostgreSQL store → Go HTTP API → Svelte SPA.

```
backend/   Go API, authentication, sync engine, storage, migrations
frontend/  Svelte 5/Vite control-room SPA
docs/      operations and architecture notes
```

Sync runs nightly at 03:00 UTC and may be manually triggered by an admin. When `DISCORD_WEBHOOK_URL` is set, each finished run's notification outcome (sent, skipped, or failed) is recorded on the run and shown in the officer sync-run history. Migration 0007 bootstraps existing users into the single Ape Enclosure guild; existing admins become owners. Membership roles are explicit and independent of Blizzard guild ranks. The platform-level `superadmin` role may select any guild and is granted at sign-in to configured BattleTags.

## Sync operations

Officers can poll `GET /api/sync/progress` and `GET /api/sync/runs` for a live, safe run summary. Administrators (and superadmins) may start `POST /api/sync/run`, validate without writes through `POST /api/sync/dry-run`, retry the recorded failed characters with `POST /api/sync/runs/{id}/retry`, and request cancellation through `POST /api/sync/cancel`. The SPA polls progress while a run is active. Only one run is admitted at a time; cancellation and process shutdown propagate a context to the engine, then wait for its work to settle. Dry runs remain in the audit ledger with the `dry-run` trigger but do not alter character, progression, or snapshot data.

The roster page exports the roster as a CSV download (Export CSV link) matching the current view: the active filters and sort ordering apply to the export, so what you see is what you get. The header toggle switches between light and dark themes; the choice is stored per browser and otherwise follows the system preference.

## Officer workflow

Officers, admins, and superadmins can use **Officer workflow** for guild-scoped review only; members cannot access its API or UI. It keeps local officer notes (with the persisted author identity), normalized tags, and optional lifecycle statuses (`applicant`, `trial`, `active`, `inactive`, `retired`) separate from Blizzard-synced character fields. Notes are capped at 2,000 characters; tags are lower-case ASCII names matching `[a-z0-9][a-z0-9_-]{0,31}`, with at most 20 tags; confirmed bulk tag and review operations are transactional and limited to 100 distinct targets. The deterministic queue lists stale characters, recent failed sync runs (with durable run identifiers that can be marked reviewed), characters without progression snapshots, and changes after the last officer review. Invalid or cross-guild bulk targets are returned as structured details and no requested target is changed. Activity records actor, action, target, timestamp, and safe metadata only: note contents and credentials are never recorded.

A daily cleanup sweeper deletes progression snapshots older than `SNAPSHOT_RETENTION_DAYS` (default 90) and expired sessions.

## Historical progression

`GET /api/characters/{id}/history` returns guild-scoped, oldest-first snapshots for a validated RFC3339 `from`/`to` range (at most 90 days) and a bounded `limit` (at most 500). It includes item level, Mythic+ rating, best key, raid payload, timestamps, and an optional comparison selected by `compareFrom` and `compareTo` snapshot IDs. The response reports the configured retention period so the UI can explain missing older records. `GET /api/dashboard` includes daily guild trend aggregates; these are computed with one set-based grouped snapshot query, never one history request per character. The existing `progression_snapshots(character_id,captured_at)` index already supports guild character joins and ordered history reads, so no duplicate index or migration is justified.

## Tests

```sh
make test
make check
cd frontend && npm run test && npm run build
cd frontend && npx playwright install chromium && npm run e2e
make migrate-check api-contract
```

`openapi/openapi.json` is the checked-in integration contract. It covers the
authenticated and public HTTP surface, role requirements, bounded query
parameters, CSV export, and compatibility fields. `make api-contract` performs
a dependency-free CI validation so the document cannot become malformed.
Backend errors retain the legacy `error` string and status code while also
returning stable `code`, `message`, and `requestId` fields. Request logs use the
same correlation ID and contain only structured, redacted diagnostics.

For deterministic local development, backend integration tests use the
PostgreSQL service from `docker compose up -d db` and frontend tests use Vitest
with fetch doubles. Browser E2E uses Playwright route doubles for session,
roster, export, theme, character, and sync flows; it never requires OAuth,
Blizzard, or Discord credentials.
Run `make migrate-check` before adding a migration, then `make migrate` against
the local database. CI runs formatting, vet, tests, migration checks, contract
validation, frontend checks, tests, and build.

For troubleshooting sync or API failures, set `LOG_LEVEL=debug` in the backend environment to enable structured sync and request debug logging on stdout.

Guild data and mutations are selected and authorized server-side. The UI shows a switcher for users with multiple memberships. See `docs/multi-guild.md` for bootstrap and operating policy.
