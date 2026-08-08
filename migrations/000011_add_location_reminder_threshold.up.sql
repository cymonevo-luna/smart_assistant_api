ALTER TABLE assistant_settings
  ADD COLUMN location_reminder_threshold_meters INT NOT NULL DEFAULT 100;
