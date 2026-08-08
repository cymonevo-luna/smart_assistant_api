INSERT INTO plugins (id, slug, name, description, version, manifest, created_at, updated_at)
VALUES (
    'b2c3d4e5-f6a7-4890-b123-456789abcdef',
    'set-reminder',
    'Location Reminder',
    'Set reminders triggered when you arrive at a place.',
    '1.0.0',
    '{
        "triggers": [
            "remind me when I arrive",
            "remind me when I get",
            "remind me once I",
            "set a location reminder"
        ],
        "required_setup": false,
        "setup_type": "none",
        "arguments": [
            {
                "name": "title",
                "type": "string",
                "required": true,
                "description": "What to remind the user about",
                "prompt": "What should I remind you about?"
            },
            {
                "name": "location_mode",
                "type": "string",
                "required": true,
                "description": "exact address or nearby place keyword",
                "prompt": "Is this for a specific address or any nearby place?"
            },
            {
                "name": "place_query",
                "type": "string",
                "required": true,
                "description": "Address or place keyword",
                "prompt": "What is the address?"
            }
        ],
        "confirmation_required": true,
        "executor": {
            "type": "builtin",
            "config": {
                "builtin_adapter": "location_reminder"
            }
        }
    }'::jsonb,
    now(),
    now()
)
ON CONFLICT (slug) DO NOTHING;
