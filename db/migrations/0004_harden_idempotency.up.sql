-- 既存データにNULLがある場合の対応（一時IDで埋める）
UPDATE events SET source_msg_id = 'migrated-' || id::text WHERE source_msg_id IS NULL;

-- NOT NULL制約の追加
ALTER TABLE events
  ALTER COLUMN source_msg_id SET NOT NULL;

-- 長さ制限（1〜64文字）
ALTER TABLE events
  ADD CONSTRAINT events_source_msg_len CHECK (char_length(source_msg_id) BETWEEN 1 AND 64);

