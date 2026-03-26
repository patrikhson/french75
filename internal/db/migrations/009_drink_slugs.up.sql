-- Add URL-friendly slug to drinks.
ALTER TABLE drinks ADD COLUMN slug TEXT;

-- Generate initial slugs: lowercase, non-alphanumeric runs become hyphens, trim edges.
UPDATE drinks
SET slug = LOWER(
    REGEXP_REPLACE(
        REGEXP_REPLACE(TRIM(name), '[^a-zA-Z0-9]+', '-', 'g'),
        '^-+|-+$', '', 'g'
    )
);

-- Fallback for any empty slug (all-special-char name).
UPDATE drinks SET slug = 'drink-' || LEFT(id::text, 8)
WHERE slug IS NULL OR slug = '';

-- Resolve any duplicate slugs by appending the first 8 chars of the drink's id.
WITH ranked AS (
    SELECT id, ROW_NUMBER() OVER (PARTITION BY slug ORDER BY created_at) AS rn
    FROM drinks
)
UPDATE drinks
SET slug = drinks.slug || '-' || LEFT(drinks.id::text, 8)
FROM ranked
WHERE drinks.id = ranked.id AND ranked.rn > 1;

ALTER TABLE drinks ALTER COLUMN slug SET NOT NULL;
CREATE UNIQUE INDEX drinks_slug_unique ON drinks(slug);
