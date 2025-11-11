-- users
CREATE TABLE IF NOT EXISTS users(
  id BIGSERIAL PRIMARY KEY,
  line_user_id TEXT UNIQUE,          -- 後でLINE連携する前提。今はNULL可でもOKにしたいなら UNIQUE 削除
  ext_user_id  TEXT UNIQUE,          -- HTTP運用用の外部ID（LINE使わない期間の識別子）
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
  weight NUMERIC(4,1) NOT NULL DEFAULT 1.0,
  UNIQUE(house_id, name)
);

-- events（家事/購入の記録）
CREATE TABLE IF NOT EXISTS events(
  id BIGSERIAL PRIMARY KEY,
  house_id BIGINT NOT NULL REFERENCES houses(id) ON DELETE CASCADE,
  user_id  BIGINT NOT NULL REFERENCES users(id)  ON DELETE CASCADE,
  kind TEXT NOT NULL CHECK (kind IN ('chore','purchase')),
  category_id BIGINT NULL REFERENCES categories(id),
  minutes INT NULL CHECK (minutes IS NULL OR minutes > 0),
  amount_yen INT NULL CHECK (amount_yen IS NULL OR amount_yen > 0),
  points NUMERIC(10,1) NOT NULL,
  note TEXT,
  source_msg_id TEXT,                     -- 将来のLINE messageId/HTTPクライアント側IDで冪等化
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (house_id, source_msg_id)
);

-- house設定（円↔ptや日上限）
CREATE TABLE IF NOT EXISTS house_settings(
  house_id BIGINT PRIMARY KEY REFERENCES houses(id) ON DELETE CASCADE,
  yen_per_point NUMERIC(6,2) NOT NULL DEFAULT 10.0,
  daily_cap_minutes INT NOT NULL DEFAULT 60
);
