INSERT INTO plugins (id, slug, name, description, version, manifest, created_at, updated_at)
VALUES (
    'a1b2c3d4-e5f6-4789-a012-3456789abcde',
    'google-calendar-meet',
    'Google Meet Scheduler',
    'Schedule Google Calendar events with Meet links for your contacts.',
    '1.0.0',
    '{
        "triggers": [
            "schedule a meeting",
            "set up a meeting",
            "create a calendar event"
        ],
        "required_setup": true,
        "setup_type": "oauth_google",
        "arguments": [
            {
                "name": "attendee_name",
                "type": "string",
                "required": true,
                "description": "Name of the person to meet with",
                "prompt": "Who would you like to meet with?"
            },
            {
                "name": "attendee_email",
                "type": "email",
                "required": true,
                "description": "Email address of the attendee",
                "prompt": "What is {attendee_name}''s email address?"
            },
            {
                "name": "start_time",
                "type": "datetime",
                "required": true,
                "description": "When the meeting should start",
                "prompt": "When should the meeting start?"
            },
            {
                "name": "title",
                "type": "string",
                "required": false,
                "description": "Calendar event title",
                "prompt": "What should the meeting be called?"
            }
        ],
        "confirmation_required": true,
        "executor": {
            "type": "composio",
            "config": {
                "tool_slug": "GOOGLECALENDAR_CREATE_EVENT",
                "builtin_adapter": "google_calendar_meet"
            }
        }
    }'::jsonb,
    now(),
    now()
)
ON CONFLICT (slug) DO NOTHING;
