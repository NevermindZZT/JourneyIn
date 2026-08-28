package store

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

func (s *Store) GetSetting(ctx context.Context, key string) (string, bool, error) {
	var value string
	err := s.db.QueryRowContext(ctx, "SELECT value FROM app_settings WHERE key = ?", key).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return value, true, nil
}

func (s *Store) SetSetting(ctx context.Context, key, value string, secret bool) error {
	_, err := s.db.ExecContext(ctx, "INSERT INTO app_settings(key, value, is_secret, updated_at) VALUES (?, ?, ?, ?) ON CONFLICT(key) DO UPDATE SET value=excluded.value, is_secret=excluded.is_secret, updated_at=excluded.updated_at", key, value, boolInt(secret), time.Now().UTC().Format(time.RFC3339Nano))
	return err
}

func (s *Store) DeleteSetting(ctx context.Context, key string) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM app_settings WHERE key = ?", key)
	return err
}
func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}
