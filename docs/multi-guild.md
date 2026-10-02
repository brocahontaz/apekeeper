# Multi-guild operations

OAuth credentials remain platform-global. Guild identity, membership roles
(`owner`, `admin`, `officer`, `member`), sync schedule, and Discord webhook
settings are guild-local. Membership roles never derive from Blizzard ranks.

Migration `0007_multi_guild` bootstraps every existing user into the only
existing guild. Existing admins become owners; other users remain members. It
only runs for a single-guild database, avoiding ambiguous ownership. Owners,
admins, and superadmins manage memberships through `/api/guild/members`.
The membership page searches existing Battle.net identities and grants a
selected role by BattleTag, so administrators do not need numeric user IDs.

The backend validates the HttpOnly selected-guild cookie against authenticated
membership on every guild-scoped request. Superadmins may select any known
guild. Client-provided guild IDs are never authorization inputs.
