CREATE TABLE assistant_sessions (
    id             UUID PRIMARY KEY,
    user_id        TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    status         TEXT NOT NULL DEFAULT 'active',
    pending_action JSONB,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_assistant_sessions_user_id ON assistant_sessions (user_id);
CREATE INDEX IF NOT EXISTS idx_assistant_sessions_created_at ON assistant_sessions (created_at DESC);
