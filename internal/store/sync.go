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

func (s *Store) PushChange(ctx context.Context, change journeysync.Change) (journeysync.Change, error) {
	if journeysync.IsHistoryOperation(change.Operation) {
		return s.pushHistoryChange(ctx, change)
	}
	if change.ChangeID == "" || change.AggregateID == "" || change.DeviceID == "" {
		return journeysync.Change{}, errors.New("missing change identity")
	}
	if change.NewRevision != change.BaseRevision+1 {
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
	var current int
	tripExists := true
	err = tx.QueryRowContext(ctx, "SELECT revision FROM trips WHERE id = ?", change.AggregateID).Scan(&current)
	if errors.Is(err, sql.ErrNoRows) {
		tripExists = false
		err = tx.QueryRowContext(ctx, "SELECT COALESCE(MAX(new_revision), 0) FROM sync_changes WHERE aggregate_id = ?", change.AggregateID).Scan(&current)
	}
	if err != nil {
		return journeysync.Change{}, err
	}
	if current != change.BaseRevision {
		return journeysync.Change{}, journeysync.ErrConflict
	}
	now := time.Now().UTC()
	if change.Operation == "delete" {
		if tripExists {
			if _, err := tx.ExecContext(ctx, "DELETE FROM trips WHERE id = ?", change.AggregateID); err != nil {
				return journeysync.Change{}, err
			}
		}
	} else if json.Valid(change.Payload) {
		normalized, trip, issues, err := domain.NormalizeTripForID(change.Payload, change.AggregateID)
		if err != nil {
			return journeysync.Change{}, err
		}
		if hasErrors(issues) {
			return journeysync.Change{}, fmt.Errorf("synced trip validation failed: %s", issues[0].Message)
		}
		hash := domain.ContentHash(normalized)
		if tripExists {
			if _, err := tx.ExecContext(ctx, "UPDATE trips SET title=?, status=?, start_date=?, end_date=?, timezone=?, revision=?, document_json=?, content_hash=?, updated_at=? WHERE id=? AND revision=?", trip.Title, trip.Status, trip.DateRange.Start, trip.DateRange.End, trip.Timezone, change.NewRevision, string(normalized), hash, now.Format(time.RFC3339Nano), change.AggregateID, change.BaseRevision); err != nil {
				return journeysync.Change{}, err
			}
		} else {
			if _, err := tx.ExecContext(ctx, "INSERT INTO trips(id, title, status, start_date, end_date, timezone, revision, document_json, content_hash, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)", change.AggregateID, trip.Title, trip.Status, trip.DateRange.Start, trip.DateRange.End, trip.Timezone, change.NewRevision, string(normalized), hash, now.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano)); err != nil {
				return journeysync.Change{}, err
			}
		}
	}
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

func (s *Store) PullChanges(ctx context.Context, aggregate string, cursor journeysync.Cursor, limit int) ([]journeysync.Change, journeysync.Cursor, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	query := "SELECT sequence, change_id, aggregate_id, device_id, operation, base_revision, new_revision, hash, payload, created_at FROM sync_changes WHERE sequence > ?"
	args := []any{int64(cursor)}
	if aggregate != "" {
		query += " AND aggregate_id = ?"
		args = append(args, aggregate)
	}
	query += " ORDER BY sequence ASC LIMIT ?"
	args = append(args, limit)
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, cursor, err
	}
	defer rows.Close()
	result := make([]journeysync.Change, 0, limit)
	next := cursor
	for rows.Next() {
		var c journeysync.Change
		var sequence int64
		var created string
		if err := rows.Scan(&sequence, &c.ChangeID, &c.AggregateID, &c.DeviceID, &c.Operation, &c.BaseRevision, &c.NewRevision, &c.Hash, &c.Payload, &created); err != nil {
			return nil, cursor, err
		}
		c.Sequence = journeysync.Cursor(sequence)
		c.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
		result = append(result, c)
		next = c.Sequence
	}
	return result, next, rows.Err()
}
