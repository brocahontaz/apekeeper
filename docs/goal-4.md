# Goal Prompt: More Useful Roster Views

Improve the roster experience for officers managing larger or more diverse guilds. Inspect the current server-side filters and sorting, CSV export, URL state handling, table accessibility, API contracts, responsive CSS, and tests. Keep filtering authoritative on the server and preserve current export semantics.

Deliver these outcomes:

1. Add filters for Mythic+ season/rating, raid tier/difficulty/progress, activity age, and any stored role or officer status. Reject invalid values with clear API errors.
2. Add grouping or view modes for class, role, realm, guild rank, activity, and progression without duplicating roster data requests.
3. Add configurable column visibility and persist the preference per browser. Preserve useful defaults and accessible labels.
4. Add a mobile card view or equivalent responsive layout while retaining sortable table semantics on larger screens.
5. Improve pagination for large guilds using stable deterministic ordering and, if justified by measured query plans, keyset pagination. Ensure CSV export still returns all matching rows in the selected order.
6. Add a clear “view as filtered” summary and preserve filters through character navigation and browser back/forward actions.

Acceptance criteria:

- The API remains the source of truth for every filter and sort.
- Sorts have stable tie-breakers and deterministic output across repeated requests.
- Export matches the complete filtered result, not the current page.
- Invalid, excessive, or contradictory query parameters are handled safely.
- Desktop, tablet, and mobile layouts remain usable with keyboard and screen readers.
- Tests cover every new filter, combinations, ordering, pagination boundaries, CSV output, URL restoration, authorization, and empty results.
- Document new query parameters and any required database indexes.

Constraints:

- Do not weaken existing roster authorization or CSV escaping.
- Avoid loading the entire guild into the browser merely to implement a view.
- Do not add dependencies unless clearly necessary.
- Do not commit, push, or alter unrelated files.
