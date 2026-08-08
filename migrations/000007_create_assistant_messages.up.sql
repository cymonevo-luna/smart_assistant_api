CREATE TABLE assistant_messages (
    id         UUID PRIMARY KEY,
    session_id UUID NOT NULL REFERENCES assistant_sessions(id) ON DELETE CASCADE,
    role       TEXT NOT NULL,
    content    TEXT NOT NULL,
    metadata   JSONB NOT NULL DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_assistant_messages_session_id ON assistant_messages (session_id);
CREATE INDEX IF NOT EXISTS idx_assistant_messages_created_at ON assistant_messages (session_id, created_at ASC);
