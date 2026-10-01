# Goal Prompt: Frontend Resilience

Make the Svelte SPA resilient, accessible, and comfortable on narrow screens. Inspect the existing routes, shared CSS, router, stores, theme behavior, API wrapper, component tests, and available browser tooling. Preserve the expedition-log visual language, stored theme preference, system preference fallback, and current URL-driven roster behavior.

Deliver these outcomes:

1. Add consistent loading, retryable error, empty, and stale-data states for dashboard, roster, character detail, login, and sync history. Avoid losing already loaded data unnecessarily during refreshes.
2. Ensure asynchronous requests cannot overwrite newer state after navigation, filter changes, or component teardown. Cancel or ignore stale requests safely.
3. Improve narrow-screen header/navigation behavior, table overflow, controls, pagination, charts, and character detail layouts at mobile and tablet widths.
4. Make theme control state explicit and accessible, including current mode, keyboard operation, focus styles, and system-preference changes.
5. Improve announcements and focus management for route changes, errors, sync completion, empty results, and validation messages. Keep visible status text for users who do not use assistive technology.
6. Make roster URL state fully round-trip through direct links and browser back/forward navigation, including filters, sort, direction, and page.

Acceptance criteria:

- No uncaught rejected promise or stale response corrupts visible state.
- Loading, error, and empty states have appropriate semantic roles and usable retry actions.
- Keyboard-only and screen-reader-oriented flows can navigate the header, filters, table, dialogs, and theme control.
- Verify desktop and mobile layouts with browser tooling or an equivalent repeatable check.
- Component/unit tests cover theme behavior, accessible states, retries, stale requests, URL restoration, and responsive-specific behavior where testable.
- Run frontend check, tests, and build; update documentation if user-visible behavior or browser support changes.

Constraints:

- Do not introduce a UI framework or large dependency for small interaction improvements.
- Preserve existing authentication redirects and API error semantics.
- Keep visual changes focused and avoid unrelated redesign.
- Do not commit, push, or modify unrelated files.
