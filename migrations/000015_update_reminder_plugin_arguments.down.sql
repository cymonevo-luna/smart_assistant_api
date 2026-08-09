UPDATE plugins
SET
    manifest = jsonb_set(
        manifest,
        '{arguments}',
        '[]'::jsonb
    ),
    updated_at = now()
WHERE slug = 'reminder';
