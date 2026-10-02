CREATE TABLE oauth_states (
    state_hash bytea PRIMARY KEY,
    expires_at timestamptz NOT NULL
);
CREATE INDEX oauth_states_expiry_idx ON oauth_states (expires_at);

CREATE TABLE audit_events (
    id bigserial PRIMARY KEY,
    actor_user_id bigint REFERENCES users(id) ON DELETE SET NULL,
    event_type text NOT NULL,
    guild_id bigint REFERENCES guilds(id) ON DELETE SET NULL,
    metadata jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at timestamptz NOT NULL DEFAULT now()
);
