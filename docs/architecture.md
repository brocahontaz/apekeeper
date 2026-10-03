# Architecture

The backend flows from Blizzard clients through the sync engine into the PostgreSQL store, then exposes authenticated aggregates through `httpapi`. The Svelte SPA consumes that API and is optionally served by the Go process after build.

Finished sync runs fan out to optional external channels through `internal/notify`: a `DiscordNotifier` posts a summary of every run to that guild's configured Discord webhook in its own goroutine, so delivery can neither delay nor affect the sync result. The legacy `DISCORD_WEBHOOK_URL` is only the bootstrap default.

Every roster and progression query is scoped by `guild_id`. `guild_memberships`
contains explicit per-guild roles, while `users.app_role=superadmin` remains a
platform role. The API resolves the HttpOnly selected-guild cookie through the
authenticated membership table on every request; URL and query identifiers do
not grant access. Migration 0007 bootstraps the existing single guild and
assigns existing admins as owners.

Each sync records character progression and snapshots for historical comparisons. Snapshot retention is owned by the sync/store layer. Future incremental sync can reuse the existing guild scope and update only changed character records while preserving the same snapshot contract.

The HTTP route table and OpenAPI document are checked in together under `openapi/`; CI compares every registered method/path and its role requirement against the contract. Shared response shape metadata is kept in `contracts/api-contract.json` and is consumed by backend and frontend contract tests. Sync services publish only aggregate counters (`started`, `completed`, `errors`, and duration) at the admin-only diagnostics endpoint; no payloads, names, URLs, or secrets are included.
