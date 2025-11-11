DROP INDEX IF EXISTS uq_categories_house_normalized;

ALTER TABLE categories DROP COLUMN IF EXISTS normalized_name;

