package collaboration

import (
	"context"
	"errors"
	"time"

	"journeyin/internal/store"
)

type SQLiteStore struct{ db *store.Store }

func NewSQLiteStore(db *store.Store) *SQLiteStore { return &SQLiteStore{db: db} }

func toGrant(record store.CollaborationShareRecord) Grant {
	return Grant{ID: record.ID, TripID: record.TripID, TokenHash: record.TokenHash, ExpiresAt: record.ExpiresAt, RevokedAt: record.RevokedAt, CreatedAt: record.CreatedAt}
}

func toRecord(grant Grant) store.CollaborationShareRecord {
	return store.CollaborationShareRecord{ID: grant.ID, TripID: grant.TripID, TokenHash: grant.TokenHash, ExpiresAt: grant.ExpiresAt, RevokedAt: grant.RevokedAt, CreatedAt: grant.CreatedAt}
}

func (s *SQLiteStore) Put(grant Grant) error {
	return s.db.CreateCollaborationShare(context.Background(), toRecord(grant))
}

func (s *SQLiteStore) Get(hash [32]byte) (Grant, error) {
	record, err := s.db.GetCollaborationShareByTokenHash(context.Background(), hash)
	if errors.Is(err, store.ErrNotFound) {
		return Grant{}, ErrNotFound
	}
	if err != nil {
		return Grant{}, err
	}
	return toGrant(record), nil
}

func (s *SQLiteStore) List(tripID string) ([]Grant, error) {
	records, err := s.db.ListCollaborationShares(context.Background(), tripID)
	if err != nil {
		return nil, err
	}
	grants := make([]Grant, 0, len(records))
	for _, record := range records {
		grants = append(grants, toGrant(record))
	}
	return grants, nil
}

func (s *SQLiteStore) Revoke(tripID, id string, at time.Time) error {
	err := s.db.RevokeCollaborationShare(context.Background(), tripID, id, at)
	if errors.Is(err, store.ErrNotFound) {
		return ErrNotFound
	}
	return err
}
