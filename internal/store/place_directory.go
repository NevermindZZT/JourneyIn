package store

import (
	"context"
	"time"
)

type PlaceDirectoryRecord struct {
	Provider     string
	ProviderID   string
	Name         string
	Address      string
	Region       string
	Category     string
	LocationJSON []byte
	CreatedAt    time.Time
	LastSeenAt   time.Time
	ExpiresAt    time.Time
}

func (s *Store) PurgeExpiredPlaceDirectory(ctx context.Context, now time.Time) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM place_directory WHERE expires_at <= ?", now.UTC().Format(time.RFC3339Nano))
	return err
}
func (s *Store) FindPlaceDirectory(ctx context.Context, query, region, category string, limit int) ([]PlaceDirectoryRecord, error) {
	if limit < 1 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	pattern := "%" + query + "%"
	rows, err := s.db.QueryContext(ctx, "SELECT provider, provider_id, name, address, region, category, location_json, created_at, last_seen_at, expires_at FROM place_directory WHERE expires_at > ? AND (name LIKE ? OR address LIKE ?) AND (? = '' OR region = ? OR region LIKE ?) AND (? = '' OR category = ?) ORDER BY last_seen_at DESC LIMIT ?", now, pattern, pattern, region, region, "%"+region+"%", category, category, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]PlaceDirectoryRecord, 0, limit)
	for rows.Next() {
		var item PlaceDirectoryRecord
		var created, seen, expires string
		if err := rows.Scan(&item.Provider, &item.ProviderID, &item.Name, &item.Address, &item.Region, &item.Category, &item.LocationJSON, &created, &seen, &expires); err != nil {
			return nil, err
		}
		item.CreatedAt, err = time.Parse(time.RFC3339Nano, created)
		if err != nil {
			return nil, err
		}
		item.LastSeenAt, err = time.Parse(time.RFC3339Nano, seen)
		if err != nil {
			return nil, err
		}
		item.ExpiresAt, err = time.Parse(time.RFC3339Nano, expires)
		if err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}
func (s *Store) UpsertPlaceDirectory(ctx context.Context, item PlaceDirectoryRecord) error {
	_, err := s.db.ExecContext(ctx, "INSERT INTO place_directory(provider, provider_id, name, address, region, category, location_json, created_at, last_seen_at, expires_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?) ON CONFLICT(provider, provider_id) DO UPDATE SET name=excluded.name, address=excluded.address, region=excluded.region, category=excluded.category, location_json=excluded.location_json, last_seen_at=excluded.last_seen_at, expires_at=excluded.expires_at", item.Provider, item.ProviderID, item.Name, item.Address, item.Region, item.Category, item.LocationJSON, item.CreatedAt.UTC().Format(time.RFC3339Nano), item.LastSeenAt.UTC().Format(time.RFC3339Nano), item.ExpiresAt.UTC().Format(time.RFC3339Nano))
	return err
}
func (s *Store) ClearPlaceDirectory(ctx context.Context) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, "DELETE FROM place_directory"); err != nil {
		_ = tx.Rollback()
		return err
	}
	if _, err = tx.ExecContext(ctx, "DELETE FROM map_cache WHERE kind = 'poi_search'"); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}
func (s *Store) PlaceDirectoryCount(ctx context.Context) (int, error) {
	var count int
	err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM place_directory WHERE expires_at > ?", time.Now().UTC().Format(time.RFC3339Nano)).Scan(&count)
	return count, err
}
