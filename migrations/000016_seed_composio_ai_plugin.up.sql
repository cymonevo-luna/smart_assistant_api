INSERT INTO plugins (id, slug, name, description, version, manifest, created_at, updated_at)
VALUES (
    'c3d4e5f6-a7b8-4901-c234-56789abcdef0',
    'composio-ai',
    'Composio AI',
    'Connects your Composio account so the assistant can act across all your integrated apps.',
    '1.0.0',
    '{
        "triggers": [
            "help me",
            "can you",
            "please",
            "send",
            "create",
            "update",
            "post",
            "schedule",
            "find",
            "list",
            "delete",
            "sync",
            "automate",
            "integrate",
            "use my",
            "in my app"
        ],
        "required_setup": true,
        "setup_type": "form",
        "arguments": [
            {
                "name": "task",
                "type": "string",
                "required": true,
                "description": "The task to perform across integrated apps",
                "prompt": "What would you like me to do?"
            }
        ],
        "confirmation_required": false,
        "executor": {
            "type": "composio_mcp",
            "config": {}
        }
    }'::jsonb,
    now(),
    now()
)
ON CONFLICT (slug) DO NOTHING;
