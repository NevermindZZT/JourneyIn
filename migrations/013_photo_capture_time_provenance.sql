-- 013_photo_capture_time_provenance.sql: source and timezone trust for capture-time matching.
ALTER TABLE photo_index ADD COLUMN taken_at_source TEXT NOT NULL DEFAULT 'unknown';
ALTER TABLE photo_index ADD COLUMN taken_at_offset_known INTEGER NOT NULL DEFAULT 0;
CREATE INDEX IF NOT EXISTS idx_photo_index_gps_taken_at ON photo_index(has_gps, taken_at);
