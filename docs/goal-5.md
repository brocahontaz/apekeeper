# Goal Prompt: Multi-Guild Support

Complete ApeKeeper's transition from a single configured guild to a usable multi-guild platform. The schema is already guild-scoped, so inspect existing guild creation/lookup, sync configuration, authentication, roles, scheduler, stores, API routes, frontend session state, migrations, and documentation before changing behavior.

Deliver these outcomes:

1. Add a guild membership model linking users to guilds with explicit per-guild roles. Preserve platform-level superadmin behavior and do not infer application permissions from Blizzard guild ranks.
2. Add a guild selection flow after sign-in and a visible current-guild switcher. Persist the selected guild safely and require the backend to resolve it from authenticated membership rather than trusting a frontend-only identifier.
3. Make sync configuration, scheduler ownership, Discord notification configuration, and Blizzard credentials explicit per guild or clearly separate platform-global settings from guild settings.
4. Scope every existing dashboard, roster, character, export, sync history, and mutation endpoint to the selected guild. Add an administration view for authorized users to manage memberships.
5. Provide a safe migration/bootstrap path for the existing Ape Enclosure deployment and document how the first administrator and guild owner are assigned.

Acceptance criteria:

- No user can read or mutate another guild's data by changing a URL, cookie, request body, or query parameter.
- Membership and role checks are centralized, tested, and applied consistently to all endpoints.
- Existing single-guild installations migrate without data loss or ambiguous ownership.
- Scheduled jobs cannot accidentally sync the wrong guild and concurrent runs remain isolated.
- Tests cover membership grants/revocation, role boundaries, guild switching, cross-guild ID/name access, migrations, and scheduler isolation.
- Add integration tests using at least two guilds with overlapping character names.
- Update README, architecture, environment documentation, and operational setup instructions.

Constraints:

- Do not duplicate guild-scoped tables unnecessarily.
- Do not trust a client-selected guild without rechecking authenticated membership.
- Keep current API behavior compatible for the default guild where practical.
- Do not commit, push, or modify unrelated files.
