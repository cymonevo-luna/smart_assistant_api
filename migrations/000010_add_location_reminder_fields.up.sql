ALTER TABLE reminders
    ADD COLUMN trigger_type  TEXT NOT NULL DEFAULT 'time',
    ADD COLUMN title         TEXT,
    ADD COLUMN location_mode TEXT,
    ADD COLUMN place_query   TEXT,
    ADD COLUMN latitude      DOUBLE PRECISION,
    ADD COLUMN longitude     DOUBLE PRECISION,
    ADD COLUMN place_keyword TEXT,
    ADD COLUMN radius_meters INT NOT NULL DEFAULT 0,
    ADD COLUMN triggered_at  TIMESTAMPTZ;

ALTER TABLE reminders ALTER COLUMN message DROP NOT NULL;
ALTER TABLE reminders ALTER COLUMN remind_at DROP NOT NULL;

CREATE INDEX IF NOT EXISTS idx_reminders_user_id_status ON reminders (user_id, status);
