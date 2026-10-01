# Goal Prompt: Better Sync Operations

Improve ApeKeeper's synchronization workflow for guild officers without weakening authorization or changing the existing guild-scoped data model. Start by inspecting the Go sync engine, scheduler, sync-run store and API, Discord notification path, Svelte sync view, migrations, tests, and CI. Preserve the established expedition-log visual language and make the smallest complete vertical slices.

Deliver these outcomes:

1. Add live progress reporting for a running sync. Use polling or Server-Sent Events, whichever fits the existing architecture best, and expose safe progress fields such as status, total, updated, failed, and current phase. Do not expose secrets or raw upstream credentials.
2. Add a retry-failed workflow that starts a bounded retry for characters that failed in a completed run. Preserve the original run and record the retry as a separate run with an explicit trigger and accurate outcome.
3. Make manually triggered runs visibly transition from queued/running to completed, partial, or failed without requiring a page refresh. Handle already-running and unavailable-sync errors clearly.
4. Add a dry-run option that fetches and validates data but does not mutate character, progression, snapshot, or cleanup state. Clearly label dry-run results.
5. Make sync work cancellation-safe and ensure background work does not leak goroutines during shutdown. Notification delivery must remain bounded and must never change the sync result.

Acceptance criteria:

- Existing sync, scheduler, authorization, and notification behavior remains compatible.
- Only authorized administrators can trigger, retry, cancel, or dry-run a sync.
- A running sync is observable through the API and the UI, including partial failures.
- Retry behavior is idempotent enough to prevent accidental duplicate concurrent runs.
- Automated tests cover progress, dry-run non-mutation, retry failures, authorization, cancellation, already-running conflicts, and shutdown behavior.
- Run the relevant Go and frontend checks documented in the repository, including integration tests where database behavior changes.
- Update README or architecture documentation for new endpoints, configuration, and operational behavior.

Constraints:

- Do not add dependencies unless clearly necessary.
- Validate all user-controlled parameters on the server.
- Do not block HTTP requests on long-running sync work.
- Do not commit, push, or modify unrelated files.
