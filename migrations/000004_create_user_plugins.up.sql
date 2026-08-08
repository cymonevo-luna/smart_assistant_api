CREATE TABLE user_plugins (
    id           UUID PRIMARY KEY,
    user_id      TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    plugin_id    UUID NOT NULL REFERENCES plugins(id) ON DELETE CASCADE,
    enabled      BOOLEAN NOT NULL DEFAULT true,
    setup_status TEXT NOT NULL DEFAULT 'not_started',
    setup_error  TEXT,
    config       JSONB NOT NULL DEFAULT '{}',
    installed_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (user_id, plugin_id)
);

CREATE INDEX IF NOT EXISTS idx_user_plugins_user_id ON user_plugins (user_id);
CREATE INDEX IF NOT EXISTS idx_user_plugins_installed_at ON user_plugins (installed_at DESC);
