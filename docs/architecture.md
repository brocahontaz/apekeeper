# Architecture

The backend flows from Blizzard clients through the sync engine into the PostgreSQL store, then exposes authenticated aggregates through `httpapi`. The Svelte SPA consumes that API and is optionally served by the Go process after build.

Every roster and progression query is scoped by `guild_id`; `EnsureGuild` establishes the configured guild. This makes the schema multi-guild ready even though ApeKeeper currently operates Ape Enclosure.

Each sync records character progression and snapshots for historical comparisons. Snapshot retention is owned by the sync/store layer. Future incremental sync can reuse the existing guild scope and update only changed character records while preserving the same snapshot contract.
