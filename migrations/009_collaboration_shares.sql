CREATE TABLE IF NOT EXISTS collaboration_shares (
  id TEXT PRIMARY KEY,
  trip_id TEXT NOT NULL,
  token_hash BLOB NOT NULL UNIQUE CHECK (length(token_hash) = 32),
  expires_at TEXT NOT NULL,
  revoked_at TEXT,
  created_at TEXT NOT NULL,
  FOREIGN KEY (trip_id) REFERENCES trips(id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_collaboration_shares_trip_id ON collaboration_shares(trip_id);
CREATE INDEX IF NOT EXISTS idx_collaboration_shares_expires_at ON collaboration_shares(expires_at);
