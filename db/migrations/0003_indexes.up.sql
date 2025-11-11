CREATE INDEX IF NOT EXISTS idx_events_house_created_at ON events(house_id, created_at);

CREATE INDEX IF NOT EXISTS idx_events_house_source ON events(house_id, source_msg_id);

CREATE INDEX IF NOT EXISTS idx_categories_house_name ON categories(house_id, name);

CREATE INDEX IF NOT EXISTS idx_memberships_house_user ON memberships(house_id, user_id);

-- カバリングインデックス（週次集計クエリの最適化）
CREATE INDEX IF NOT EXISTS idx_events_house_created_cover
ON events(house_id, created_at)
INCLUDE (points, user_id);

