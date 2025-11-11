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

-- events（家事の記録）
CREATE TABLE IF NOT EXISTS events(
  id BIGSERIAL PRIMARY KEY,
  house_id BIGINT NOT NULL REFERENCES houses(id) ON DELETE CASCADE,
  user_id  BIGINT NOT NULL REFERENCES users(id)  ON DELETE CASCADE,
  kind TEXT NOT NULL DEFAULT 'chore' CHECK (kind = 'chore'),
  task_key TEXT NOT NULL,
  task_option TEXT,
  points NUMERIC(10,1) NOT NULL,
  note TEXT,
  source_msg_id TEXT NOT NULL,                     -- LINE messageId/HTTPクライアント側IDで冪等化
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (house_id, source_msg_id),
  CONSTRAINT events_source_msg_len CHECK (char_length(source_msg_id) BETWEEN 1 AND 64)
);

-- インデックス
CREATE INDEX IF NOT EXISTS idx_events_house_created_at ON events(house_id, created_at);
CREATE INDEX IF NOT EXISTS idx_events_house_source ON events(house_id, source_msg_id);
CREATE INDEX IF NOT EXISTS idx_memberships_house_user ON memberships(house_id, user_id);
CREATE INDEX IF NOT EXISTS idx_events_house_created_cover
  ON events(house_id, created_at)
  INCLUDE (points, user_id);

-- 初期データ
INSERT INTO houses (ext_group_id, name) VALUES ('default-house', 'default')
ON CONFLICT DO NOTHING;
