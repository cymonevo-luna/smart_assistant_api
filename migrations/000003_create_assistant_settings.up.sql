CREATE TABLE assistant_settings (
  user_id TEXT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
  wake_word TEXT NOT NULL DEFAULT 'Jarvis',
  active_listening_enabled BOOLEAN NOT NULL DEFAULT false,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
