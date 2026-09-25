# Battle.net setup

Create an OAuth client in the [Battle.net Developer Portal](https://develop.battle.net/), then copy its client ID and secret to `BLIZZARD_CLIENT_ID` and `BLIZZARD_CLIENT_SECRET` in `.env`.

Register the redirect URI configured by `OAUTH_REDIRECT_URL`. For local development with the supplied environment this is **`http://localhost:5173/api/auth/callback`**. The backend API callback endpoint is `GET /api/auth/callback`; the browser reaches it through the frontend origin, where the Vite dev server proxies `/api` to the backend. Use that exact URL unless you change the backend’s configured redirect URL.

Set region (`us`, `eu`, `kr`, or `tw`) and a matching locale. Guild realm and slug spelling must match Blizzard’s data. Blizzard APIs are rate limited: keep scheduled syncs nightly and avoid repeated manual triggers.

### Troubleshooting

* **Invalid redirect URI:** the portal value and `OAUTH_REDIRECT_URL` must match exactly.
* **No guild data:** check region, realm, guild name/slug, and connected-realm spelling.
* **Authorization works but data fails:** verify client credentials and selected API region.
