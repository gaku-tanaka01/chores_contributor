ALTER TABLE events
  DROP CONSTRAINT IF EXISTS events_source_msg_len;

ALTER TABLE events
  ALTER COLUMN source_msg_id DROP NOT NULL;

