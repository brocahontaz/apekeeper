# Goal Prompt: Operational Roster Improvements

Improve ApeKeeper's officer workflow without changing its guild-scoped data model or weakening authorization. Start by inspecting the existing Go backend, PostgreSQL store layer, Svelte 5 frontend, tests, CI, and recent dashboard/CSV/Discord work. Keep the established expedition-log visual language and make the smallest complete vertical slices.

Deliver these outcomes:

1. Fix the dashboard data path so its aggregates scale with roster size. The current single SQL statement joins raid rows before correlated Mythic+, snapshot count, first-snapshot, and last-snapshot subqueries; those calculations can repeat for every raid row. Use set-based grouped/CTE aggregates (or an equivalently demonstrable plan) so each character's progression aggregates are calculated once, preserve the existing dashboard JSON contract, deterministic ordering, and 30-day snapshot behavior. Add integration coverage for a character with multiple raid rows and multiple Mythic+ seasons, and document any useful indexes or migration required by the final query.
2. Make CSV export match the officer's current roster view. Export all matching rows, not merely the current page, honoring validated roster filters and sort/direction from the URL. Keep it authenticated, stream CSV safely, retain a useful filename, and ensure the frontend export link includes the active query state. Add backend tests for filters, ordering, CSV escaping, unauthorized access, and empty results.
3. Improve sync operability. Surface the most recent Discord-notification delivery result in structured logs and the existing sync-run view without allowing notification failure to change a completed sync result. Bound delivery with a timeout, avoid goroutine leaks during shutdown, redact webhook URLs/tokens from logs and errors, and add tests for success, non-2xx responses, network failure, timeout, and disabled notification configuration.
4. Improve UI resilience and accessibility. Make the header usable on narrow screens, give the theme control an explicit visible state, and ensure loading/error/empty states are announced appropriately. Preserve system-preference and stored-theme behavior. Add focused component/unit tests and verify desktop and mobile layouts manually or with the available browser tooling.

Constraints:

- Do not introduce new third-party dependencies unless clearly necessary.
- Preserve existing API compatibility except for additive fields needed by the sync view.
- Keep dashboard responses deterministic and avoid N+1 queries.
- Validate user-controlled query parameters on the server; never trust frontend-only filtering.
- Add or update user-facing docs for behavior/configuration changes.
- Do not commit, push, or alter unrelated files.

Quality gates:

- Run `make test`, `make check`, `cd frontend && npm run test`, and `cd frontend && npm run build`.
- Inspect `git status`, `git diff`, and `git diff --cached` before completion.
- Obtain an independent review and resolve valid BLOCKER/IMPORTANT findings before reporting completion.
