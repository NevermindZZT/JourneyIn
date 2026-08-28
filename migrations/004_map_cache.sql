CREATE TABLE IF NOT EXISTS map_cache (
  provider TEXT NOT NULL,
  kind TEXT NOT NULL,
  cache_key TEXT NOT NULL,
  response_json BLOB NOT NULL,
  expires_at TEXT NOT NULL,
  created_at TEXT NOT NULL,
  PRIMARY KEY (provider, kind, cache_key)
);
CREATE INDEX IF NOT EXISTS idx_map_cache_expiry ON map_cache (expires_at);

CREATE TABLE IF NOT EXISTS map_request_usage (
  provider TEXT NOT NULL,
  usage_date TEXT NOT NULL,
  request_count INTEGER NOT NULL DEFAULT 0,
  updated_at TEXT NOT NULL,
  PRIMARY KEY (provider, usage_date)
);
