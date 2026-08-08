CREATE TABLE reminders (
    id             UUID PRIMARY KEY,
    user_id        TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    title          TEXT NOT NULL,
    trigger_type   TEXT NOT NULL,
    location_mode  TEXT,
    place_query    TEXT,
    latitude       DOUBLE PRECISION,
    longitude      DOUBLE PRECISION,
    place_keyword  TEXT,
    radius_meters  INT NOT NULL,
    status         TEXT NOT NULL DEFAULT 'pending',
    triggered_at   TIMESTAMPTZ,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_reminders_user_id ON reminders (user_id);
CREATE INDEX IF NOT EXISTS idx_reminders_user_id_status ON reminders (user_id, status);
