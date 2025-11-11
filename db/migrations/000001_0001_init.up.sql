-- users
CREATE TABLE IF NOT EXISTS users(
  id BIGSERIAL PRIMARY KEY,
  line_user_id TEXT UNIQUE,          -- LINE連携用
  ext_user_id  TEXT UNIQUE,          -- HTTP運用用の外部ID
  display_name TEXT
);

-- houses (LINEグループ/シェアハウス単位)
CREATE TABLE IF NOT EXISTS houses(
  id BIGSERIAL PRIMARY KEY,
  line_group_id TEXT UNIQUE,         -- LINE用
  ext_group_id  TEXT UNIQUE,         -- HTTP運用用
  name TEXT
);

-- memberships
CREATE TABLE IF NOT EXISTS memberships(
  house_id BIGINT REFERENCES houses(id) ON DELETE CASCADE,
  user_id  BIGINT REFERENCES users(id)  ON DELETE CASCADE,
  role TEXT DEFAULT 'member',
  PRIMARY KEY (house_id, user_id)
);

-- categories（家ごとの重み）
CREATE TABLE IF NOT EXISTS categories(
  id BIGSERIAL PRIMARY KEY,
  house_id BIGINT REFERENCES houses(id) ON DELETE CASCADE,
  name TEXT NOT NULL,
  normalized_name TEXT GENERATED ALWAYS AS (
    trim(regexp_replace(name, '\s+', ' ', 'g'))
  ) STORED,
  weight NUMERIC(4,1) NOT NULL DEFAULT 1.0,
  UNIQUE(house_id, name)
);

-- events（家事の記録）
CREATE TABLE IF NOT EXISTS events(
  id BIGSERIAL PRIMARY KEY,
  house_id BIGINT NOT NULL REFERENCES houses(id) ON DELETE CASCADE,
  user_id  BIGINT NOT NULL REFERENCES users(id)  ON DELETE CASCADE,
  kind TEXT NOT NULL DEFAULT 'chore' CHECK (kind = 'chore'),
  category_id BIGINT NULL REFERENCES categories(id),
  points NUMERIC(10,1) NOT NULL,
  note TEXT,
  source_msg_id TEXT NOT NULL,                     -- LINE messageId/HTTPクライアント側IDで冪等化
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (house_id, source_msg_id),
  CONSTRAINT events_source_msg_len CHECK (char_length(source_msg_id) BETWEEN 1 AND 64)
);

-- house設定（円↔ptや日上限）
CREATE TABLE IF NOT EXISTS house_settings(
  house_id BIGINT PRIMARY KEY REFERENCES houses(id) ON DELETE CASCADE,
  yen_per_point NUMERIC(6,2) NOT NULL DEFAULT 10.0,
  daily_cap_minutes INT NOT NULL DEFAULT 60
);

-- インデックス
CREATE INDEX IF NOT EXISTS idx_events_house_created_at ON events(house_id, created_at);
CREATE INDEX IF NOT EXISTS idx_events_house_source ON events(house_id, source_msg_id);
CREATE INDEX IF NOT EXISTS idx_categories_house_name ON categories(house_id, name);
CREATE INDEX IF NOT EXISTS idx_memberships_house_user ON memberships(house_id, user_id);
CREATE INDEX IF NOT EXISTS idx_events_house_created_cover
  ON events(house_id, created_at)
  INCLUDE (points, user_id);
CREATE UNIQUE INDEX IF NOT EXISTS uq_categories_house_normalized
  ON categories(house_id, normalized_name);

-- 初期データ
INSERT INTO houses (ext_group_id, name) VALUES ('default-house', 'default')
ON CONFLICT DO NOTHING;

WITH h AS (SELECT id FROM houses WHERE ext_group_id='default-house')
INSERT INTO categories (house_id, name, weight)
SELECT h.id, v.name, v.weight
FROM h, (VALUES
 ('皿洗い',1.0),
 ('ゴミ出し',1.2),
 ('トイレ',1.6),
 ('風呂',1.8),
 ('掃除機',1.1),
 ('買い出し',1.0)
) AS v(name,weight)
ON CONFLICT DO NOTHING;
