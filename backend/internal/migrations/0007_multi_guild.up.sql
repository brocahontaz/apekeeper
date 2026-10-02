ALTER TABLE guild_memberships ADD COLUMN role text NOT NULL DEFAULT 'member';
ALTER TABLE guild_memberships ADD CONSTRAINT guild_memberships_role_check CHECK (role IN ('owner','admin','officer','member'));
ALTER TABLE guilds ADD COLUMN sync_schedule text NOT NULL DEFAULT '03:00';
ALTER TABLE guilds ADD COLUMN discord_webhook_url text NOT NULL DEFAULT '';

-- Existing Ape Enclosure installations have one unambiguous guild. The
-- lowest-id admin/superadmin is the preserved bootstrap owner; all other
-- existing application admins remain admins. This is the explicit owner
-- assignment for the pre-multi-guild single-guild bootstrap.
INSERT INTO guild_memberships(guild_id, user_id, role)
SELECT g.id, u.id, CASE WHEN u.id = (SELECT min(id) FROM users WHERE app_role IN ('admin','superadmin')) THEN 'owner' WHEN u.app_role = 'admin' THEN 'admin' ELSE 'member' END
FROM guilds g CROSS JOIN users u
WHERE (SELECT count(*) FROM guilds) = 1
  AND NOT EXISTS (SELECT 1 FROM guild_memberships m WHERE m.guild_id = g.id AND m.user_id = u.id);

UPDATE guild_memberships m SET role = CASE WHEN u.id = (SELECT min(id) FROM users WHERE app_role IN ('admin','superadmin')) THEN 'owner' ELSE 'admin' END
FROM users u
WHERE m.user_id = u.id AND u.app_role = 'admin' AND (SELECT count(*) FROM guilds) = 1;
