# Roster query and view semantics

`GET /api/roster` and `/api/roster/export` share the same server-side filters and
sort. `mythicSeason` matches the stored `mythic_plus.season_slug`; `raidTier`
matches stored `raid_progression.raid_slug` or `raid_name`; `raidDifficulty` and
`minRaidProgress` use the corresponding stored progression row. `activityAge`
is a maximum age in days for `characters.synced_at`. `officerStatus` and
`officerTag` use the stored officer lifecycle status and exact normalized tag. `role`
filters on `users.app_role` through `characters.user_id`; valid values are `member`,
`officer`, `admin`, and `superadmin`. The returned `role` is that stored account role,
not the Blizzard specialization. Characters without a linked user are treated as members.

Existing `search`, `class`, `spec`, `rank`, `minLevel`, `minRating`, `stale`,
`page`, `pageSize`, `sort`, and `direction` remain supported. Invalid or
excessive values return HTTP 400. Results have an `id` tie-breaker after every
sort key, and export omits pagination while retaining the selected order.

The roster's grouping modes only regroup the already fetched page and never
issue another roster request. Column visibility is stored in browser
`localStorage` under `apekeeper.roster.columns`; the mobile card view is a
responsive presentation of the same server result.

The existing guild, progression, stale, and officer indexes support these
queries. No additional index is required by this feature.
