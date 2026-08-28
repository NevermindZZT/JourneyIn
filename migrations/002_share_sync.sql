CREATE TABLE IF NOT EXISTS shares (
  id TEXT PRIMARY KEY,
  trip_id TEXT NOT NULL,
  revision INTEGER NOT NULL,
  content_hash TEXT NOT NULL,
  token_hash BLOB NOT NULL UNIQUE,
  snapshot_json TEXT NOT NULL,
  expires_at TEXT NOT NULL,
  revoked_at TEXT,
  created_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_shares_trip_id ON shares(trip_id);
CREATE INDEX IF NOT EXISTS idx_shares_expires_at ON shares(expires_at);

CREATE TABLE IF NOT EXISTS sync_changes (
  sequence INTEGER PRIMARY KEY AUTOINCREMENT,
  change_id TEXT NOT NULL UNIQUE,
  aggregate_id TEXT NOT NULL,
  device_id TEXT NOT NULL,
  operation TEXT NOT NULL,
  base_revision INTEGER NOT NULL,
  new_revision INTEGER NOT NULL,
  hash TEXT NOT NULL,
  payload BLOB,
  created_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_sync_changes_aggregate_sequence ON sync_changes(aggregate_id, sequence);
