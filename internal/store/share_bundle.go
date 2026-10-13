package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"
)

type ShareBundleRecord struct {
	ID          string
	TokenHash   [32]byte
	ContentHash string
	ItemCount   int
	TripTitles  []string
	Snapshot    []byte
	CreatedBy   string
	ExpiresAt   time.Time
	RevokedAt   *time.Time
	CreatedAt   time.Time
}

func (s *Store) PutShareBundle(ctx context.Context, record ShareBundleRecord) error {
	titles, err := json.Marshal(record.TripTitles)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO share_bundles(id, token_hash, content_hash, item_count, trip_titles_json, snapshot_json, created_by, expires_at, revoked_at, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, record.ID, record.TokenHash[:], record.ContentHash, record.ItemCount, string(titles), string(record.Snapshot), record.CreatedBy, nullableStringTime(record.ExpiresAt), nullableTime(record.RevokedAt), record.CreatedAt.UTC().Format(time.RFC3339Nano))
	return err
}

func (s *Store) GetShareBundleByTokenHash(ctx context.Context, hash [32]byte) (ShareBundleRecord, error) {
	var record ShareBundleRecord
	var token []byte
	var titles string
	var expires, revoked, created sql.NullString
	err := s.db.QueryRowContext(ctx, `SELECT id, token_hash, content_hash, item_count, trip_titles_json, snapshot_json, created_by, expires_at, revoked_at, created_at FROM share_bundles WHERE token_hash = ?`, hash[:]).Scan(&record.ID, &token, &record.ContentHash, &record.ItemCount, &titles, &record.Snapshot, &record.CreatedBy, &expires, &revoked, &created)
	if errors.Is(err, sql.ErrNoRows) {
		return ShareBundleRecord{}, ErrNotFound
	}
	if err != nil {
		return ShareBundleRecord{}, err
	}
	copy(record.TokenHash[:], token)
	if err := json.Unmarshal([]byte(titles), &record.TripTitles); err != nil {
		return ShareBundleRecord{}, err
	}
	if expires.Valid && expires.String != "" {
		record.ExpiresAt, _ = time.Parse(time.RFC3339Nano, expires.String)
	}
	if revoked.Valid && revoked.String != "" {
		if value, parseErr := time.Parse(time.RFC3339Nano, revoked.String); parseErr == nil {
			record.RevokedAt = &value
		}
	}
	record.CreatedAt, _ = time.Parse(time.RFC3339Nano, created.String)
	return record, nil
}

func (s *Store) ListShareBundles(ctx context.Context, actorID string, admin bool) ([]ShareBundleRecord, error) {
	query := `SELECT id, content_hash, item_count, trip_titles_json, created_by, expires_at, revoked_at, created_at FROM share_bundles`
	args := []any{}
	if !admin {
		query += ` WHERE created_by = ?`
		args = append(args, actorID)
	}
	query += ` ORDER BY created_at DESC LIMIT 100`
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]ShareBundleRecord, 0)
	for rows.Next() {
		var record ShareBundleRecord
		var titles string
		var expires, revoked, created sql.NullString
		if err := rows.Scan(&record.ID, &record.ContentHash, &record.ItemCount, &titles, &record.CreatedBy, &expires, &revoked, &created); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(titles), &record.TripTitles); err != nil {
			return nil, err
		}
		if expires.Valid && expires.String != "" {
			record.ExpiresAt, _ = time.Parse(time.RFC3339Nano, expires.String)
		}
		if revoked.Valid && revoked.String != "" {
			if value, parseErr := time.Parse(time.RFC3339Nano, revoked.String); parseErr == nil {
				record.RevokedAt = &value
			}
		}
		record.CreatedAt, _ = time.Parse(time.RFC3339Nano, created.String)
		result = append(result, record)
	}
	return result, rows.Err()
}

func (s *Store) RevokeShareBundle(ctx context.Context, id, actorID string, admin bool, at time.Time) error {
	query := `DELETE FROM share_bundles WHERE id = ?`
	args := []any{id}
	if !admin {
		query += ` AND created_by = ?`
		args = append(args, actorID)
	}
	result, err := s.db.ExecContext(ctx, query, args...)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return ErrNotFound
	}
	return nil
}

func nullableStringTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Format(time.RFC3339Nano)
}
