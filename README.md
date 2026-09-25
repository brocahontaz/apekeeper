# ApeKeeper

ApeKeeper is the GM and officer control room for **Ape Enclosure**: a compact guild roster, progression, and sync ledger rather than a generic dashboard.

## Local development

This repository supports local development only. Production deployment, reverse proxies, TLS, and production Compose configuration are intentionally out of scope; they will live in a separate deployment repository.

Copy the example environment file and set the required values:

```sh
cp .env.example .env
```

`GUILD_REALM` is required (use the Blizzard realm slug, such as `area-52`). Blizzard OAuth requires `BLIZZARD_CLIENT_ID`, `BLIZZARD_CLIENT_SECRET`, and an OAuth application registration for the exact `OAUTH_REDIRECT_URL`. The default callback route is `http://localhost:5173/api/auth/callback` (`GET /api/auth/callback`): the browser authenticates on the frontend origin and the Vite dev server proxies `/api` to the backend, so after sign-in you land on the SPA at `http://localhost:5173` rather than the backend port.

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

Both workflows expose the frontend at http://localhost:5173, the backend at http://localhost:8080, and PostgreSQL at `localhost:5432`. In Compose, the Vite frontend proxies API requests to the separate backend service. The backend image does not package frontend assets; `STATIC_DIR` remains an optional native fallback only.

## Architecture

Blizzard APIs → sync service → PostgreSQL store → Go HTTP API → Svelte SPA.

```
backend/   Go API, authentication, sync engine, storage, migrations
frontend/  Svelte 5/Vite control-room SPA
docs/      operations and architecture notes
```

Sync runs nightly at 03:00 UTC and may be manually triggered by an admin. The first authenticated user becomes an admin; ApeKeeper roles are application roles, deliberately independent of Blizzard guild ranks. The platform-level `superadmin` role has at least every admin capability and is granted at sign-in to BattleTags listed in the optional `SUPER_ADMIN_BATTLETAGS` variable (comma-separated, trimmed); it is intended for the repository or platform owner rather than the guild's actual GM, who holds the `admin` ("Guild Master") role.

## Tests

```sh
make test
make check
cd frontend && npm run test && npm run build
```

For troubleshooting sync or API failures, set `LOG_LEVEL=debug` in the backend environment to enable structured sync and request debug logging on stdout.

The schema is guild-scoped and ready for multi-guild support; the UI currently presents Ape Enclosure.
