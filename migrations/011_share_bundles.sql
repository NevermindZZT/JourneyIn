CREATE TABLE IF NOT EXISTS share_bundles (
  id TEXT PRIMARY KEY,
  token_hash BLOB NOT NULL UNIQUE,
  content_hash TEXT NOT NULL,
  item_count INTEGER NOT NULL,
  trip_titles_json TEXT NOT NULL,
  snapshot_json TEXT NOT NULL,
  created_by TEXT NOT NULL,
  expires_at TEXT NOT NULL,
  revoked_at TEXT,
  created_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_share_bundles_created_by ON share_bundles(created_by, created_at);
CREATE INDEX IF NOT EXISTS idx_share_bundles_expires_at ON share_bundles(expires_at);

CREATE TABLE IF NOT EXISTS imported_share_sources (
  source_origin TEXT NOT NULL,
  source_trip_id TEXT NOT NULL,
  content_hash TEXT NOT NULL,
  local_trip_id TEXT NOT NULL,
  imported_at TEXT NOT NULL,
  PRIMARY KEY (source_origin, source_trip_id, content_hash),
  FOREIGN KEY (local_trip_id) REFERENCES trips(id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_imported_share_local_trip ON imported_share_sources(local_trip_id);
