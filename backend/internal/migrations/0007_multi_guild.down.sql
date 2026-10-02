ALTER TABLE guilds DROP COLUMN IF EXISTS discord_webhook_url;
ALTER TABLE guilds DROP COLUMN IF EXISTS sync_schedule;
ALTER TABLE guild_memberships DROP CONSTRAINT IF EXISTS guild_memberships_role_check;
ALTER TABLE guild_memberships DROP COLUMN IF EXISTS role;
