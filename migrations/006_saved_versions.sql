CREATE TABLE IF NOT EXISTS trip_saved_versions (
  id TEXT PRIMARY KEY,
  trip_id TEXT NOT NULL,
  source_revision INTEGER NOT NULL,
  title TEXT NOT NULL,
  start_date TEXT NOT NULL,
  end_date TEXT NOT NULL,
  label TEXT NOT NULL DEFAULT '',
  document_json TEXT NOT NULL,
  content_hash TEXT NOT NULL,
  created_at TEXT NOT NULL,
  UNIQUE (trip_id, content_hash),
  FOREIGN KEY (trip_id) REFERENCES trips(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_trip_saved_versions_trip_created
  ON trip_saved_versions(trip_id, created_at DESC, id DESC);

CREATE TABLE IF NOT EXISTS trip_saved_version_tombstones (
  trip_id TEXT NOT NULL,
  version_id TEXT NOT NULL,
  content_hash TEXT,
  deleted_at TEXT NOT NULL,
  PRIMARY KEY (trip_id, version_id),
  FOREIGN KEY (trip_id) REFERENCES trips(id) ON DELETE CASCADE
);
