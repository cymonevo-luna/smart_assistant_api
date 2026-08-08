DROP INDEX IF EXISTS idx_reminders_user_id_status;

ALTER TABLE reminders
    DROP COLUMN IF EXISTS triggered_at,
    DROP COLUMN IF EXISTS radius_meters,
    DROP COLUMN IF EXISTS place_keyword,
    DROP COLUMN IF EXISTS longitude,
    DROP COLUMN IF EXISTS latitude,
    DROP COLUMN IF EXISTS place_query,
    DROP COLUMN IF EXISTS location_mode,
    DROP COLUMN IF EXISTS title,
    DROP COLUMN IF EXISTS trigger_type;

ALTER TABLE reminders ALTER COLUMN message SET NOT NULL;
ALTER TABLE reminders ALTER COLUMN remind_at SET NOT NULL;
