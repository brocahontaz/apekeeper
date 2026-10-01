# Goal Prompt: Historical Progression Analytics

Turn ApeKeeper's existing progression snapshots into useful historical analytics for officers. Inspect the snapshot schema and retention sweeper, character detail and dashboard queries, Go API types, Svelte routes/components, existing formatting conventions, tests, and CI. Keep the dashboard deterministic and avoid N+1 queries.

Deliver these outcomes:

1. Add a typed history API for a character that returns item level, Mythic+ rating, best key, raid progression, and capture timestamps within a validated date range. Preserve the current character endpoint contract and add fields or endpoints additively.
2. Add character charts for item level and Mythic+ rating over time, with accessible text summaries for users who cannot use visual charts.
3. Add snapshot comparison showing changes since the previous snapshot and between two selected snapshots. Handle missing, identical, and incomplete snapshots gracefully.
4. Add guild-level trend summaries for roster average item level, average rating, stale count, and raid progress. Use set-based SQL aggregation or equivalent demonstrable queries, not per-character requests.
5. Add season and tier labels where the stored data supports them, and make retention boundaries visible so users understand why older history is unavailable.

Acceptance criteria:

- Historical data is scoped by guild and character authorization exactly like existing endpoints.
- Date ranges, limits, and comparison IDs are validated server-side and have safe bounds.
- Charts render loading, error, empty, and malformed-data states without breaking the page.
- Accessibility includes labels, keyboard usability, and a non-visual summary.
- Tests cover ordering, date boundaries, missing snapshots, comparisons, authorization, empty results, and aggregate correctness.
- Verify query performance and add only justified indexes or migrations, documenting them.
- Run `make test`, `make check`, frontend tests, and the frontend build.

Constraints:

- Do not rewrite existing snapshot data or weaken retention guarantees.
- Do not introduce a chart dependency unless the repository's existing stack cannot support the feature reasonably.
- Preserve existing API compatibility and the expedition-log visual language.
- Do not commit, push, or alter unrelated files.
