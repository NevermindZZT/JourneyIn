package store

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"journeyin/internal/domain"
)

type SavedTripVersion struct {
	ID             string
	TripID         string
	SourceRevision int
	Title          string
	StartDate      string
	EndDate        string
	Label          string
	Document       []byte
	ContentHash    string
	CreatedAt      time.Time
}

var ErrSavedTripVersionNotFound = errors.New("saved trip version not found")

func (s *Store) CreateSavedTripVersion(ctx context.Context, tripID string, expectedRevision int, label string) (SavedTripVersion, bool, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return SavedTripVersion{}, false, err
	}
	defer tx.Rollback()

	var current SavedTripVersion
	var document []byte
	if err := tx.QueryRowContext(ctx, "SELECT revision, title, start_date, end_date, document_json, content_hash FROM trips WHERE id = ?", tripID).Scan(&current.SourceRevision, &current.Title, &current.StartDate, &current.EndDate, &document, &current.ContentHash); errors.Is(err, sql.ErrNoRows) {
		return SavedTripVersion{}, false, ErrNotFound
	} else if err != nil {
		return SavedTripVersion{}, false, err
	}
	if current.SourceRevision != expectedRevision {
		return SavedTripVersion{}, false, ErrRevisionConflict
	}

	var existing SavedTripVersion
	var existingCreated string
	err = tx.QueryRowContext(ctx, "SELECT id, trip_id, source_revision, title, start_date, end_date, label, content_hash, created_at FROM trip_saved_versions WHERE trip_id = ? AND content_hash = ?", tripID, current.ContentHash).Scan(&existing.ID, &existing.TripID, &existing.SourceRevision, &existing.Title, &existing.StartDate, &existing.EndDate, &existing.Label, &existing.ContentHash, &existingCreated)
	if err == nil {
		existing.CreatedAt, _ = time.Parse(time.RFC3339Nano, existingCreated)
		if err := tx.Commit(); err != nil {
			return SavedTripVersion{}, false, err
		}
		return existing, true, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return SavedTripVersion{}, false, err
	}

	id, err := domain.NewID("history")
	if err != nil {
		return SavedTripVersion{}, false, err
	}
	now := time.Now().UTC()
	if _, err := tx.ExecContext(ctx, "INSERT OR IGNORE INTO trip_saved_versions(id, trip_id, source_revision, title, start_date, end_date, label, document_json, content_hash, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)", id, tripID, current.SourceRevision, current.Title, current.StartDate, current.EndDate, strings.TrimSpace(label), string(document), current.ContentHash, now.Format(time.RFC3339Nano)); err != nil {
		return SavedTripVersion{}, false, err
	}

	var savedCreated string
	if err := tx.QueryRowContext(ctx, "SELECT id, trip_id, source_revision, title, start_date, end_date, label, content_hash, created_at FROM trip_saved_versions WHERE trip_id = ? AND content_hash = ?", tripID, current.ContentHash).Scan(&current.ID, &current.TripID, &current.SourceRevision, &current.Title, &current.StartDate, &current.EndDate, &current.Label, &current.ContentHash, &savedCreated); err != nil {
		return SavedTripVersion{}, false, err
	}
	current.CreatedAt, _ = time.Parse(time.RFC3339Nano, savedCreated)
	if err := tx.Commit(); err != nil {
		return SavedTripVersion{}, false, err
	}
	return current, false, nil
}

func (s *Store) ListSavedTripVersions(ctx context.Context, tripID string, limit int) ([]SavedTripVersion, error) {
	if err := s.ensureTripExists(ctx, tripID); err != nil {
		return nil, err
	}
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	rows, err := s.db.QueryContext(ctx, "SELECT id, trip_id, source_revision, title, start_date, end_date, label, content_hash, created_at FROM trip_saved_versions WHERE trip_id = ? ORDER BY created_at DESC, id DESC LIMIT ?", tripID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]SavedTripVersion, 0, limit)
	for rows.Next() {
		var version SavedTripVersion
		var created string
		if err := rows.Scan(&version.ID, &version.TripID, &version.SourceRevision, &version.Title, &version.StartDate, &version.EndDate, &version.Label, &version.ContentHash, &created); err != nil {
			return nil, err
		}
		version.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
		result = append(result, version)
	}
	return result, rows.Err()
}

func (s *Store) GetSavedTripVersion(ctx context.Context, tripID, versionID string) (SavedTripVersion, error) {
	var version SavedTripVersion
	var created string
	err := s.db.QueryRowContext(ctx, "SELECT id, trip_id, source_revision, title, start_date, end_date, label, document_json, content_hash, created_at FROM trip_saved_versions WHERE trip_id = ? AND id = ?", tripID, versionID).Scan(&version.ID, &version.TripID, &version.SourceRevision, &version.Title, &version.StartDate, &version.EndDate, &version.Label, &version.Document, &version.ContentHash, &created)
	if errors.Is(err, sql.ErrNoRows) {
		if tripErr := s.ensureTripExists(ctx, tripID); tripErr != nil {
			return SavedTripVersion{}, tripErr
		}
		return SavedTripVersion{}, ErrSavedTripVersionNotFound
	}
	if err != nil {
		return SavedTripVersion{}, err
	}
	version.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
	return version, nil
}

func (s *Store) DeleteSavedTripVersion(ctx context.Context, tripID, versionID string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var exists int
	if err := tx.QueryRowContext(ctx, "SELECT 1 FROM trips WHERE id = ?", tripID).Scan(&exists); errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	} else if err != nil {
		return err
	}
	var contentHash string
	if err := tx.QueryRowContext(ctx, "SELECT content_hash FROM trip_saved_versions WHERE trip_id = ? AND id = ?", tripID, versionID).Scan(&contentHash); errors.Is(err, sql.ErrNoRows) {
		var tombstone int
		if tombstoneErr := tx.QueryRowContext(ctx, "SELECT 1 FROM trip_saved_version_tombstones WHERE trip_id = ? AND version_id = ?", tripID, versionID).Scan(&tombstone); tombstoneErr == nil {
			if err := tx.Commit(); err != nil {
				return err
			}
			return nil
		} else if !errors.Is(tombstoneErr, sql.ErrNoRows) {
			return tombstoneErr
		}
		return ErrSavedTripVersionNotFound
	} else if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "INSERT OR IGNORE INTO trip_saved_version_tombstones(trip_id, version_id, content_hash, deleted_at) VALUES (?, ?, ?, ?)", tripID, versionID, contentHash, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM trip_saved_versions WHERE trip_id = ? AND id = ?", tripID, versionID); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	return nil
}

func (s *Store) ensureTripExists(ctx context.Context, tripID string) error {
	var exists int
	if err := s.db.QueryRowContext(ctx, "SELECT 1 FROM trips WHERE id = ?", tripID).Scan(&exists); errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	} else if err != nil {
		return err
	}
	return nil
}
