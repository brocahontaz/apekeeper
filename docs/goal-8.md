# Goal Prompt: Developer and API Improvements

Improve ApeKeeper's maintainability and integration surface without changing behavior unnecessarily. Inspect Go handlers and stores, frontend API types, migrations, tests, CI workflows, Makefile commands, and documentation. Treat the existing JSON endpoints as a compatibility surface.

Deliver these outcomes:

1. Publish an accurate OpenAPI specification for authenticated and public endpoints, including schemas, role requirements, query validation, error responses, CSV export, and additive sync fields.
2. Add contract tests that verify important Go JSON responses match the frontend TypeScript models and that compatibility-sensitive fields remain present and correctly typed.
3. Add an API error envelope with stable machine-readable codes while preserving useful human-readable messages and existing status codes where practical.
4. Add focused integration and query-plan regression tests for dashboard aggregates, roster filters/sorts, exports, character history, sync history, and guild isolation.
5. Add end-to-end browser coverage for login/session handling, roster navigation, filtering/export, theme selection, character detail, and sync-run status using deterministic test doubles rather than live Blizzard services.
6. Improve local developer workflows with documented commands, seeded fixtures, migration checks, and CI jobs that run the authoritative backend/frontend gates consistently.
7. Add observability conventions: request IDs, structured endpoint logs, sync metrics, duration/error counters, and safe diagnostics for failed upstream calls.

Acceptance criteria:

- The API specification is generated or checked in CI so it cannot silently drift.
- Contract tests detect incompatible backend/frontend changes before merge.
- Test doubles never require real OAuth credentials, Blizzard access, or Discord webhooks.
- Query and end-to-end tests remain deterministic and have bounded runtime.
- Logs and metrics contain correlation data but no credentials or personal secrets.
- CI runs formatting, vet/static checks, backend tests, frontend checks, frontend tests, build, migrations, and contract validation.
- Update README and architecture documentation with the supported development and integration workflow.

Constraints:

- Do not rewrite stable APIs merely to fit a documentation generator.
- Do not weaken tests or hide failures behind retries.
- Avoid adding dependencies unless the maintenance benefit is clear and documented.
- Do not commit, push, or alter unrelated files.
