CREATE TABLE plugin_credentials (
    id                UUID PRIMARY KEY,
    user_plugin_id    UUID NOT NULL UNIQUE REFERENCES user_plugins(id) ON DELETE CASCADE,
    provider          TEXT NOT NULL,
    encrypted_payload TEXT NOT NULL,
    expires_at        TIMESTAMPTZ,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_plugin_credentials_user_plugin_id ON plugin_credentials (user_plugin_id);
