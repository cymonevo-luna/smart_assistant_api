CREATE TABLE reminders (
    id             UUID PRIMARY KEY,
    user_id        TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    user_plugin_id UUID,
    message        TEXT NOT NULL,
    remind_at        TIMESTAMPTZ NOT NULL,
    status           TEXT NOT NULL,
    notified_at      TIMESTAMPTZ,
    delivered_at     TIMESTAMPTZ,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_reminders_user_id ON reminders (user_id);
CREATE INDEX IF NOT EXISTS idx_reminders_remind_at ON reminders (remind_at);
CREATE INDEX IF NOT EXISTS idx_reminders_user_id_remind_at ON reminders (user_id, remind_at);
CREATE INDEX IF NOT EXISTS idx_reminders_status_remind_at ON reminders (status, remind_at);
