package store

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

type CollaborationShareRecord struct {
	ID        string
	TripID    string
	TokenHash [32]byte
	ExpiresAt time.Time
	RevokedAt *time.Time
	CreatedAt time.Time
}

func (s *Store) CreateCollaborationShare(ctx context.Context, record CollaborationShareRecord) error {
	_, err := s.db.ExecContext(ctx, "INSERT INTO collaboration_shares(id, trip_id, token_hash, expires_at, revoked_at, created_at) VALUES (?, ?, ?, ?, ?, ?)", record.ID, record.TripID, record.TokenHash[:], record.ExpiresAt.UTC().Format(time.RFC3339Nano), nullableTime(record.RevokedAt), record.CreatedAt.UTC().Format(time.RFC3339Nano))
	return err
}

func (s *Store) GetCollaborationShareByTokenHash(ctx context.Context, hash [32]byte) (CollaborationShareRecord, error) {
	var record CollaborationShareRecord
	var token []byte
	var expires, revoked, created sql.NullString
	err := s.db.QueryRowContext(ctx, "SELECT id, trip_id, token_hash, expires_at, revoked_at, created_at FROM collaboration_shares WHERE token_hash = ?", hash[:]).Scan(&record.ID, &record.TripID, &token, &expires, &revoked, &created)
	if errors.Is(err, sql.ErrNoRows) {
		return CollaborationShareRecord{}, ErrNotFound
	}
	if err != nil {
		return CollaborationShareRecord{}, err
	}
	if len(token) != len(record.TokenHash) {
		return CollaborationShareRecord{}, errors.New("invalid collaboration token hash in store")
	}
	copy(record.TokenHash[:], token)
	if expires.Valid {
		record.ExpiresAt, err = time.Parse(time.RFC3339Nano, expires.String)
		if err != nil {
			return CollaborationShareRecord{}, err
		}
	}
	if created.Valid {
		record.CreatedAt, err = time.Parse(time.RFC3339Nano, created.String)
		if err != nil {
			return CollaborationShareRecord{}, err
		}
	}
	if revoked.Valid {
		value, parseErr := time.Parse(time.RFC3339Nano, revoked.String)
		if parseErr != nil {
			return CollaborationShareRecord{}, parseErr
		}
		record.RevokedAt = &value
	}
	return record, nil
}

func (s *Store) ListCollaborationShares(ctx context.Context, tripID string) ([]CollaborationShareRecord, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT id, trip_id, token_hash, expires_at, revoked_at, created_at FROM collaboration_shares WHERE trip_id = ? ORDER BY created_at DESC, id DESC", tripID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]CollaborationShareRecord, 0)
	for rows.Next() {
		var record CollaborationShareRecord
		var token []byte
		var expires, revoked, created sql.NullString
		if err := rows.Scan(&record.ID, &record.TripID, &token, &expires, &revoked, &created); err != nil {
			return nil, err
		}
		if len(token) != len(record.TokenHash) {
			return nil, errors.New("invalid collaboration token hash in store")
		}
		copy(record.TokenHash[:], token)
		if expires.Valid {
			record.ExpiresAt, err = time.Parse(time.RFC3339Nano, expires.String)
			if err != nil {
				return nil, err
			}
		}
		if created.Valid {
			record.CreatedAt, err = time.Parse(time.RFC3339Nano, created.String)
			if err != nil {
				return nil, err
			}
		}
		if revoked.Valid {
			value, parseErr := time.Parse(time.RFC3339Nano, revoked.String)
			if parseErr != nil {
				return nil, parseErr
			}
			record.RevokedAt = &value
		}
		items = append(items, record)
	}
	return items, rows.Err()
}

func (s *Store) RevokeCollaborationShare(ctx context.Context, tripID, id string, at time.Time) error {
	result, err := s.db.ExecContext(ctx, "UPDATE collaboration_shares SET revoked_at = ? WHERE trip_id = ? AND id = ? AND revoked_at IS NULL", at.UTC().Format(time.RFC3339Nano), tripID, id)
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
