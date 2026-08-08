INSERT INTO plugins (id, slug, name, description, version, manifest, created_at, updated_at)
VALUES (
    'b2c3d4e5-f6a7-4890-b123-456789cdef01',
    'reminder',
    'Reminder',
    'Set, list, and delete voice reminders. Get notified when they are due.',
    '1.0.0',
    '{
        "triggers": [
            "remind me",
            "set a reminder",
            "don''t let me forget",
            "list all reminders",
            "list my reminders",
            "show my reminders",
            "delete my reminder",
            "delete reminder",
            "remove my reminder"
        ],
        "required_setup": false,
        "setup_type": "none",
        "arguments": [],
        "confirmation_required": false,
        "executor": {
            "type": "builtin",
            "config": { "builtin_adapter": "reminder" }
        }
    }'::jsonb,
    now(),
    now()
)
ON CONFLICT (slug) DO NOTHING;
