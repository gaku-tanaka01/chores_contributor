-- カテゴリ名の正規化カラム（trim + 連続空白圧縮）
ALTER TABLE categories ADD COLUMN normalized_name text GENERATED ALWAYS AS (
  trim(regexp_replace(name, '\s+', ' ', 'g'))
) STORED;

-- 正規化後の名前で一意性を保証
CREATE UNIQUE INDEX IF NOT EXISTS uq_categories_house_normalized
  ON categories(house_id, normalized_name);

