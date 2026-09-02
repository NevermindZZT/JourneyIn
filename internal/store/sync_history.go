package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"journeyin/internal/domain"
	journeysync "journeyin/internal/sync"
)

type SavedTripVersionChange struct {
	HistoryID      string          `json:"history_id"`
	SourceRevision int             `json:"source_revision"`
	Label          string          `json:"label,omitempty"`
	Document       json.RawMessage `json:"document"`
	ContentHash    string          `json:"content_hash,omitempty"`
}

type DeletedTripVersionChange struct {
	HistoryID string `json:"history_id"`
}

func (s *Store) pushHistoryChange(ctx context.Context, change journeysync.Change) (journeysync.Change, error) {
	if change.ChangeID == "" || change.AggregateID == "" || change.DeviceID == "" {
		return journeysync.Change{}, errors.New("missing change identity")
	}
	if change.NewRevision != change.BaseRevision {
		return journeysync.Change{}, journeysync.ErrConflict
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return journeysync.Change{}, err
	}
	defer tx.Rollback()

	var oldHash, oldAggregate string
	err = tx.QueryRowContext(ctx, "SELECT hash, aggregate_id FROM sync_changes WHERE change_id = ?", change.ChangeID).Scan(&oldHash, &oldAggregate)
	if err == nil {
		if oldHash != change.Hash || oldAggregate != change.AggregateID {
			return journeysync.Change{}, journeysync.ErrIdempotencyConflict
		}
		return change, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return journeysync.Change{}, err
	}
	var tripExists int
	if err := tx.QueryRowContext(ctx, "SELECT 1 FROM trips WHERE id = ?", change.AggregateID).Scan(&tripExists); errors.Is(err, sql.ErrNoRows) {
		return journeysync.Change{}, journeysync.ErrConflict
	} else if err != nil {
		return journeysync.Change{}, err
	}

	switch change.Operation {
	case journeysync.OperationHistorySave:
		if err := applySavedTripVersionChange(ctx, tx, change); err != nil {
			return journeysync.Change{}, err
		}
	case journeysync.OperationHistoryDelete:
		if err := applyDeletedTripVersionChange(ctx, tx, change); err != nil {
			return journeysync.Change{}, err
		}
	default:
		return journeysync.Change{}, fmt.Errorf("unsupported history operation %q", change.Operation)
	}

	now := time.Now().UTC()
	result, err := tx.ExecContext(ctx, "INSERT INTO sync_changes(change_id, aggregate_id, device_id, operation, base_revision, new_revision, hash, payload, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)", change.ChangeID, change.AggregateID, change.DeviceID, change.Operation, change.BaseRevision, change.NewRevision, change.Hash, change.Payload, now.Format(time.RFC3339Nano))
	if err != nil {
		return journeysync.Change{}, fmt.Errorf("insert sync change: %w", err)
	}
	sequence, err := result.LastInsertId()
	if err != nil {
		return journeysync.Change{}, err
	}
	change.Sequence = journeysync.Cursor(sequence)
	change.CreatedAt = now
	if err := tx.Commit(); err != nil {
		return journeysync.Change{}, err
	}
	return change, nil
}

func applySavedTripVersionChange(ctx context.Context, tx *sql.Tx, change journeysync.Change) error {
	var input SavedTripVersionChange
	if err := json.Unmarshal(change.Payload, &input); err != nil {
		return fmt.Errorf("invalid history save payload: %w", err)
	}
	if input.HistoryID == "" || len(input.Document) == 0 || !json.Valid(input.Document) {
		return errors.New("history save payload requires history_id and document")
	}
	if input.SourceRevision < 1 {
		input.SourceRevision = change.BaseRevision
	}
	normalized, trip, issues, err := domain.NormalizeTripForID(input.Document, change.AggregateID)
	if err != nil {
		return err
	}
	if hasErrors(issues) {
		return fmt.Errorf("synced history validation failed: %s", issues[0].Message)
	}
	hash := domain.ContentHash(normalized)
	if input.ContentHash != "" && input.ContentHash != hash {
		return journeysync.ErrIdempotencyConflict
	}
	var deleted int
	if err := tx.QueryRowContext(ctx, "SELECT 1 FROM trip_saved_version_tombstones WHERE trip_id = ? AND version_id = ?", change.AggregateID, input.HistoryID).Scan(&deleted); err == nil {
		return journeysync.ErrConflict
	} else if !errors.Is(err, sql.ErrNoRows) {
		return err
	}

	var existingTrip, existingHash string
	err = tx.QueryRowContext(ctx, "SELECT trip_id, content_hash FROM trip_saved_versions WHERE id = ?", input.HistoryID).Scan(&existingTrip, &existingHash)
	if err == nil {
		if existingTrip != change.AggregateID || existingHash != hash {
			return journeysync.ErrIdempotencyConflict
		}
		return nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	var duplicateID string
	if err := tx.QueryRowContext(ctx, "SELECT id FROM trip_saved_versions WHERE trip_id = ? AND content_hash = ?", change.AggregateID, hash).Scan(&duplicateID); err == nil {
		return journeysync.ErrIdempotencyConflict
	} else if !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	_, err = tx.ExecContext(ctx, "INSERT INTO trip_saved_versions(id, trip_id, source_revision, title, start_date, end_date, label, document_json, content_hash, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)", input.HistoryID, change.AggregateID, input.SourceRevision, trip.Title, trip.DateRange.Start, trip.DateRange.End, input.Label, string(normalized), hash, time.Now().UTC().Format(time.RFC3339Nano))
	return err
}

func applyDeletedTripVersionChange(ctx context.Context, tx *sql.Tx, change journeysync.Change) error {
	var input DeletedTripVersionChange
	if err := json.Unmarshal(change.Payload, &input); err != nil {
		return fmt.Errorf("invalid history delete payload: %w", err)
	}
	if input.HistoryID == "" {
		return errors.New("history delete payload requires history_id")
	}
	var contentHash sql.NullString
	if err := tx.QueryRowContext(ctx, "SELECT content_hash FROM trip_saved_versions WHERE trip_id = ? AND id = ?", change.AggregateID, input.HistoryID).Scan(&contentHash); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if _, err := tx.ExecContext(ctx, "INSERT OR IGNORE INTO trip_saved_version_tombstones(trip_id, version_id, content_hash, deleted_at) VALUES (?, ?, ?, ?)", change.AggregateID, input.HistoryID, nullableString(contentHash), time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, "DELETE FROM trip_saved_versions WHERE trip_id = ? AND id = ?", change.AggregateID, input.HistoryID)
	return err
}

func nullableString(value sql.NullString) any {
	if !value.Valid {
		return nil
	}
	return value.String
}
