CREATE TABLE IF NOT EXISTS place_directory (
  provider TEXT NOT NULL,
  provider_id TEXT NOT NULL,
  name TEXT NOT NULL,
  address TEXT NOT NULL DEFAULT '',
  region TEXT NOT NULL DEFAULT '',
  category TEXT NOT NULL DEFAULT '',
  location_json BLOB NOT NULL,
  created_at TEXT NOT NULL,
  last_seen_at TEXT NOT NULL,
  expires_at TEXT NOT NULL,
  PRIMARY KEY(provider, provider_id)
);
CREATE INDEX IF NOT EXISTS idx_place_directory_expiry ON place_directory(expires_at);
CREATE INDEX IF NOT EXISTS idx_place_directory_name ON place_directory(name);
