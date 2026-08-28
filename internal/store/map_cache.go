package store

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

type MapCacheEntry struct {
	ResponseJSON []byte
	ExpiresAt    time.Time
	CreatedAt    time.Time
}

var ErrMapQuotaExceeded = errors.New("map provider daily quota exceeded")

func (s *Store) GetMapCache(ctx context.Context, provider, kind, cacheKey string) (MapCacheEntry, bool, error) {
	var response []byte
	var expiresRaw, createdRaw string
	err := s.db.QueryRowContext(ctx, "SELECT response_json, expires_at, created_at FROM map_cache WHERE provider = ? AND kind = ? AND cache_key = ?", provider, kind, cacheKey).Scan(&response, &expiresRaw, &createdRaw)
	if errors.Is(err, sql.ErrNoRows) {
		return MapCacheEntry{}, false, nil
	}
	if err != nil {
		return MapCacheEntry{}, false, err
	}
	expiresAt, err := time.Parse(time.RFC3339Nano, expiresRaw)
	if err != nil {
		return MapCacheEntry{}, false, err
	}
	createdAt, err := time.Parse(time.RFC3339Nano, createdRaw)
	if err != nil {
		return MapCacheEntry{}, false, err
	}
	if !time.Now().UTC().Before(expiresAt) {
		_, _ = s.db.ExecContext(ctx, "DELETE FROM map_cache WHERE provider = ? AND kind = ? AND cache_key = ?", provider, kind, cacheKey)
		return MapCacheEntry{}, false, nil
	}
	return MapCacheEntry{ResponseJSON: append([]byte(nil), response...), ExpiresAt: expiresAt, CreatedAt: createdAt}, true, nil
}

func (s *Store) PutMapCache(ctx context.Context, provider, kind, cacheKey string, response []byte, expiresAt, createdAt time.Time) error {
	_, err := s.db.ExecContext(ctx, "INSERT INTO map_cache(provider, kind, cache_key, response_json, expires_at, created_at) VALUES (?, ?, ?, ?, ?, ?) ON CONFLICT(provider, kind, cache_key) DO UPDATE SET response_json=excluded.response_json, expires_at=excluded.expires_at, created_at=excluded.created_at", provider, kind, cacheKey, response, expiresAt.UTC().Format(time.RFC3339Nano), createdAt.UTC().Format(time.RFC3339Nano))
	return err
}

func (s *Store) ReserveMapRequest(ctx context.Context, provider, usageDate string, dailyLimit int) error {
	if dailyLimit <= 0 {
		return nil
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()
	var count int
	queryErr := tx.QueryRowContext(ctx, "SELECT request_count FROM map_request_usage WHERE provider = ? AND usage_date = ?", provider, usageDate).Scan(&count)
	if errors.Is(queryErr, sql.ErrNoRows) {
		if _, err = tx.ExecContext(ctx, "INSERT INTO map_request_usage(provider, usage_date, request_count, updated_at) VALUES (?, ?, 1, ?)", provider, usageDate, now); err != nil {
			return err
		}
	} else if queryErr != nil {
		return queryErr
	} else {
		if count >= dailyLimit {
			_ = tx.Rollback()
			return ErrMapQuotaExceeded
		}
		if _, err = tx.ExecContext(ctx, "UPDATE map_request_usage SET request_count = request_count + 1, updated_at = ? WHERE provider = ? AND usage_date = ?", now, provider, usageDate); err != nil {
			return err
		}
	}
	err = tx.Commit()
	return err
}
