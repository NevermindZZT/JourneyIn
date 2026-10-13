package share

import (
	"context"
	"errors"
	"time"

	"journeyin/internal/store"
)

type SQLiteBundleStore struct{ db *store.Store }

func NewSQLiteBundleStore(db *store.Store) *SQLiteBundleStore { return &SQLiteBundleStore{db: db} }

func (s *SQLiteBundleStore) PutBundle(record BundleRecord) error {
	return s.db.PutShareBundle(context.Background(), store.ShareBundleRecord{ID: record.ID, TokenHash: record.TokenHash, ContentHash: record.ContentHash, ItemCount: record.ItemCount, TripTitles: record.TripTitles, Snapshot: record.Snapshot, CreatedBy: record.CreatedBy, ExpiresAt: record.ExpiresAt, RevokedAt: record.RevokedAt, CreatedAt: record.CreatedAt})
}

func (s *SQLiteBundleStore) GetBundleByTokenHash(hash [32]byte) (BundleRecord, error) {
	record, err := s.db.GetShareBundleByTokenHash(context.Background(), hash)
	if errors.Is(err, store.ErrNotFound) {
		return BundleRecord{}, ErrNotFound
	}
	if err != nil {
		return BundleRecord{}, err
	}
	return BundleRecord{ID: record.ID, TokenHash: record.TokenHash, ContentHash: record.ContentHash, ItemCount: record.ItemCount, TripTitles: record.TripTitles, Snapshot: record.Snapshot, CreatedBy: record.CreatedBy, ExpiresAt: record.ExpiresAt, RevokedAt: record.RevokedAt, CreatedAt: record.CreatedAt}, nil
}

func (s *SQLiteBundleStore) ListBundles(actorID string, admin bool) ([]BundleRecord, error) {
	records, err := s.db.ListShareBundles(context.Background(), actorID, admin)
	if err != nil {
		return nil, err
	}
	result := make([]BundleRecord, 0, len(records))
	for _, record := range records {
		result = append(result, BundleRecord{ID: record.ID, ContentHash: record.ContentHash, ItemCount: record.ItemCount, TripTitles: record.TripTitles, Snapshot: record.Snapshot, CreatedBy: record.CreatedBy, ExpiresAt: record.ExpiresAt, RevokedAt: record.RevokedAt, CreatedAt: record.CreatedAt})
	}
	return result, nil
}

func (s *SQLiteBundleStore) RevokeBundle(id, actorID string, admin bool, at time.Time) error {
	err := s.db.RevokeShareBundle(context.Background(), id, actorID, admin, at)
	if errors.Is(err, store.ErrNotFound) {
		return ErrNotFound
	}
	return err
}
