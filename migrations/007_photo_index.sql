-- 007_photo_index.sql: 照片地理信息与拍摄时间索引缓存
CREATE TABLE IF NOT EXISTS photo_index (
  id TEXT PRIMARY KEY,
  file_path TEXT UNIQUE NOT NULL,
  file_name TEXT NOT NULL,
  file_size INTEGER NOT NULL,
  mod_time INTEGER NOT NULL,
  taken_at TEXT NOT NULL,
  has_gps INTEGER NOT NULL DEFAULT 0,
  lat_wgs84 REAL,
  lng_wgs84 REAL,
  lat_gcj02 REAL,
  lng_gcj02 REAL,
  lat_bd09ll REAL,
  lng_bd09ll REAL,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_photo_index_has_gps ON photo_index(has_gps);
CREATE INDEX IF NOT EXISTS idx_photo_index_taken_at ON photo_index(taken_at);
