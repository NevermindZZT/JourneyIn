package share

import (
	"context"
	"errors"
	"time"

	"journeyin/internal/store"
)

type SQLiteStore struct{ db *store.Store }

func NewSQLiteStore(db *store.Store) *SQLiteStore { return &SQLiteStore{db: db} }
func (s *SQLiteStore) Put(record Record) error {
	return s.db.PutShare(context.Background(), store.ShareRecord{ID: record.ID, TripID: record.TripID, Revision: record.Revision, ContentHash: record.ContentHash, TokenHash: record.TokenHash, Snapshot: record.Content, ExpiresAt: record.ExpiresAt, RevokedAt: record.RevokedAt, CreatedAt: record.CreatedAt})
}
func (s *SQLiteStore) Get(hash [32]byte) (Record, error) {
	record, err := s.db.GetShareByTokenHash(context.Background(), hash)
	if err != nil {
		return Record{}, err
	}
	return Record{ID: record.ID, TripID: record.TripID, Revision: record.Revision, ContentHash: record.ContentHash, TokenHash: record.TokenHash, ExpiresAt: record.ExpiresAt, RevokedAt: record.RevokedAt, CreatedAt: record.CreatedAt, Content: record.Snapshot}, nil
}
func (s *SQLiteStore) UpdateSnapshot(expected, updated Record) error {
	err := s.db.UpdateShareSnapshot(context.Background(),
		store.ShareRecord{ID: expected.ID, TripID: expected.TripID, Revision: expected.Revision, ContentHash: expected.ContentHash, TokenHash: expected.TokenHash, Snapshot: expected.Content, ExpiresAt: expected.ExpiresAt},
		store.ShareRecord{ID: updated.ID, TripID: updated.TripID, Revision: updated.Revision, ContentHash: updated.ContentHash, TokenHash: updated.TokenHash, Snapshot: updated.Content, ExpiresAt: updated.ExpiresAt},
	)
	if errors.Is(err, store.ErrNotFound) {
		return ErrNotFound
	}
	return err
}
func (s *SQLiteStore) Revoke(id string, at time.Time) error {
	return s.db.RevokeShare(context.Background(), id, at)
}
