-- 014_attraction_monitors.sql: 景点时间窗口监控、法定节假日与机会推荐评估表
CREATE TABLE IF NOT EXISTS attraction_monitors (
  id TEXT PRIMARY KEY,
  title TEXT NOT NULL,
  place_name TEXT NOT NULL,
  lat REAL NOT NULL,
  lng REAL NOT NULL,
  crs TEXT NOT NULL DEFAULT 'gcj02',
  criteria_json TEXT NOT NULL,
  origin_city TEXT NOT NULL DEFAULT '',
  max_budget REAL NOT NULL DEFAULT 0,
  enabled INTEGER NOT NULL DEFAULT 1 CHECK (enabled IN (0, 1)),
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_attraction_monitors_enabled ON attraction_monitors(enabled);

CREATE TABLE IF NOT EXISTS holiday_calendar (
  calendar_date TEXT PRIMARY KEY,
  holiday_name TEXT NOT NULL,
  is_holiday INTEGER NOT NULL DEFAULT 0 CHECK (is_holiday IN (0, 1)),
  is_workday INTEGER NOT NULL DEFAULT 0 CHECK (is_workday IN (0, 1)),
  year INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_holiday_calendar_year ON holiday_calendar(year);

CREATE TABLE IF NOT EXISTS monitor_evaluations (
  id TEXT PRIMARY KEY,
  monitor_id TEXT NOT NULL,
  window_start TEXT NOT NULL,
  window_end TEXT NOT NULL,
  score REAL NOT NULL,
  level TEXT NOT NULL CHECK (level IN ('strongly_recommended', 'acceptable', 'waiting')),
  details_json TEXT NOT NULL,
  evaluated_at TEXT NOT NULL,
  FOREIGN KEY (monitor_id) REFERENCES attraction_monitors(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_monitor_evaluations_monitor ON monitor_evaluations(monitor_id);
CREATE INDEX IF NOT EXISTS idx_monitor_evaluations_level ON monitor_evaluations(level);
