# Goal Prompt: Officer Workflow Tools

Extend ApeKeeper from a read-only roster and progression ledger into a focused officer workflow tool. Inspect the existing user roles and authorization middleware, character and guild stores, migrations, roster/detail routes, API tests, and frontend conventions. Preserve the distinction between platform superadmin, guild admin, officer, and member.

Deliver these outcomes:

1. Add officer notes, tags, and a small set of validated custom statuses to characters. Notes must be scoped to the guild and record author and timestamps.
2. Add a review queue for stale characters, recent sync failures, missing progression, and unreviewed changes. Provide filters and deterministic ordering.
3. Add bulk actions for authorized officers, such as applying/removing tags and marking review items complete. Require explicit confirmation and make operations auditable.
4. Add an optional recruitment/member workflow with statuses such as applicant, trial, active, inactive, and retired, without overwriting Blizzard profile data.
5. Add a compact activity history for officer actions, including actor, action, target, timestamp, and relevant before/after values. Avoid recording secrets or unnecessary personal data.

Acceptance criteria:

- Every read and write is guild-scoped and enforced on the backend.
- Members cannot access officer-only notes, queues, bulk actions, or audit data unless explicitly authorized.
- Input lengths, allowed statuses, tag names, batch sizes, and identifiers are validated server-side.
- Bulk operations are transactional, bounded, idempotent where practical, and report partial validation failures clearly.
- UI supports loading, error, empty, confirmation, and success states with accessible controls.
- Tests cover authorization by role, cross-guild access attempts, validation, transaction behavior, audit records, empty queues, and bulk boundaries.
- Add migrations and user-facing documentation for new data and permissions.

Constraints:

- Do not alter Blizzard-sourced fields to store officer metadata.
- Do not expose private officer notes through existing member-facing character responses.
- Avoid speculative workflow complexity; implement a small extensible model.
- Do not commit, push, or modify unrelated files.
