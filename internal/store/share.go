package store

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

type ShareRecord struct {
	ID          string
	TripID      string
	Revision    int
	ContentHash string
	TokenHash   [32]byte
	Snapshot    []byte
	ExpiresAt   time.Time
	RevokedAt   *time.Time
	CreatedAt   time.Time
}

func (s *Store) PutShare(ctx context.Context, record ShareRecord) error {
	_, err := s.db.ExecContext(ctx, "INSERT INTO shares(id, trip_id, revision, content_hash, token_hash, snapshot_json, expires_at, revoked_at, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)", record.ID, record.TripID, record.Revision, record.ContentHash, record.TokenHash[:], string(record.Snapshot), record.ExpiresAt.UTC().Format(time.RFC3339Nano), nullableTime(record.RevokedAt), record.CreatedAt.UTC().Format(time.RFC3339Nano))
	return err
}

func (s *Store) GetShareByTokenHash(ctx context.Context, hash [32]byte) (ShareRecord, error) {
	var r ShareRecord
	var token []byte
	var expires, revoked, created sql.NullString
	err := s.db.QueryRowContext(ctx, "SELECT id, trip_id, revision, content_hash, token_hash, snapshot_json, expires_at, revoked_at, created_at FROM shares WHERE token_hash = ?", hash[:]).Scan(&r.ID, &r.TripID, &r.Revision, &r.ContentHash, &token, &r.Snapshot, &expires, &revoked, &created)
	if errors.Is(err, sql.ErrNoRows) {
		return ShareRecord{}, ErrNotFound
	}
	if err != nil {
		return ShareRecord{}, err
	}
	copy(r.TokenHash[:], token)
	r.ExpiresAt, _ = time.Parse(time.RFC3339Nano, expires.String)
	r.CreatedAt, _ = time.Parse(time.RFC3339Nano, created.String)
	if revoked.Valid {
		value, parseErr := time.Parse(time.RFC3339Nano, revoked.String)
		if parseErr == nil {
			r.RevokedAt = &value
		}
	}
	return r, nil
}

func (s *Store) RevokeShare(ctx context.Context, id string, at time.Time) error {
	result, err := s.db.ExecContext(ctx, "UPDATE shares SET revoked_at = ? WHERE id = ? AND revoked_at IS NULL", at.UTC().Format(time.RFC3339Nano), id)
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

func nullableTime(value *time.Time) any {
	if value == nil {
		return nil
	}
	return value.UTC().Format(time.RFC3339Nano)
}
