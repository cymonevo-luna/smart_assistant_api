UPDATE plugins
SET
    manifest = jsonb_set(
        manifest,
        '{arguments}',
        '[
            {
                "name": "message",
                "type": "string",
                "required": false,
                "prompt": "What should I remind you about?"
            },
            {
                "name": "remind_at",
                "type": "datetime",
                "required": false,
                "prompt": "When should I remind you?"
            }
        ]'::jsonb
    ),
    updated_at = now()
WHERE slug = 'reminder';
