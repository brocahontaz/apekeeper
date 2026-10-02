CREATE TABLE officer_character_state (
	character_id bigint PRIMARY KEY REFERENCES characters(id) ON DELETE CASCADE,
	note text NOT NULL DEFAULT '',
	note_author_id bigint REFERENCES users(id) ON DELETE SET NULL,
  lifecycle_status text CHECK (lifecycle_status IN ('applicant','trial','active','inactive','retired')),
  tags text[] NOT NULL DEFAULT '{}',
  reviewed_at timestamptz,
  updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE officer_activity (
  id bigserial PRIMARY KEY,
  guild_id bigint NOT NULL REFERENCES guilds(id) ON DELETE CASCADE,
  actor_id bigint NOT NULL REFERENCES users(id),
  action text NOT NULL,
  target_type text NOT NULL,
  target_id bigint,
  before_after jsonb NOT NULL DEFAULT '{}',
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX officer_activity_guild_created_idx ON officer_activity(guild_id,created_at DESC);
CREATE INDEX officer_state_review_idx ON officer_character_state(reviewed_at);
CREATE TABLE officer_sync_run_review (
  sync_run_id bigint PRIMARY KEY REFERENCES sync_runs(id) ON DELETE CASCADE,
  guild_id bigint NOT NULL REFERENCES guilds(id) ON DELETE CASCADE,
  reviewed_at timestamptz NOT NULL DEFAULT now()
);
