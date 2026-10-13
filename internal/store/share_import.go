package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"journeyin/internal/domain"
)

var ErrImportedShareAlreadyExists = errors.New("share item was already imported")

type ImportedShareTrip struct {
	Document          []byte
	SourceOrigin      string
	SourceTripID      string
	SourceContentHash string
}

type ImportedTripResult struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Status    string `json:"status"`
	StartDate string `json:"start_date"`
	EndDate   string `json:"end_date"`
	Timezone  string `json:"timezone"`
	Revision  int    `json:"revision"`
}

func (s *Store) HasImportedShareSource(ctx context.Context, origin, sourceTripID, contentHash string) (bool, error) {
	var exists int
	err := s.db.QueryRowContext(ctx, `SELECT 1 FROM imported_share_sources WHERE source_origin = ? AND source_trip_id = ? AND content_hash = ?`, origin, sourceTripID, contentHash).Scan(&exists)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return err == nil, err
}

func (s *Store) CreateShareImportedTrips(ctx context.Context, items []ImportedShareTrip, source, scope, idempotencyKey, requestHash string) ([]ImportedTripResult, bool, error) {
	if len(items) == 0 || idempotencyKey == "" || requestHash == "" {
		return nil, false, errors.New("share import batch is incomplete")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, false, err
	}
	defer tx.Rollback()
	var storedHash, response string
	err = tx.QueryRowContext(ctx, `SELECT request_hash, response_json FROM idempotency_keys WHERE scope = ? AND key = ?`, scope, idempotencyKey).Scan(&storedHash, &response)
	if err == nil {
		if storedHash != requestHash {
			return nil, false, ErrIdempotencyConflict
		}
		var result []ImportedTripResult
		if err := json.Unmarshal([]byte(response), &result); err != nil {
			return nil, false, err
		}
		return result, true, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, false, err
	}
	for _, item := range items {
		var exists int
		err := tx.QueryRowContext(ctx, `SELECT 1 FROM imported_share_sources WHERE source_origin = ? AND source_trip_id = ? AND content_hash = ?`, item.SourceOrigin, item.SourceTripID, item.SourceContentHash).Scan(&exists)
		if err == nil {
			return nil, false, ErrImportedShareAlreadyExists
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return nil, false, err
		}
	}
	now := time.Now().UTC()
	result := make([]ImportedTripResult, 0, len(items))
	for _, item := range items {
		normalized, trip, issues, err := domain.NormalizeTrip(item.Document)
		if err != nil {
			return nil, false, err
		}
		if hasErrors(issues) {
			return nil, false, fmt.Errorf("trip validation failed: %s", issues[0].Message)
		}
		hash := domain.ContentHash(normalized)
		nowText := now.Format(time.RFC3339Nano)
		if _, err := tx.ExecContext(ctx, `INSERT INTO trips(id, title, status, start_date, end_date, timezone, revision, document_json, content_hash, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, 1, ?, ?, ?, ?)`, trip.ID, trip.Title, trip.Status, trip.DateRange.Start, trip.DateRange.End, trip.Timezone, string(normalized), hash, nowText, nowText); err != nil {
			return nil, false, fmt.Errorf("insert imported trip: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO imported_share_sources(source_origin, source_trip_id, content_hash, local_trip_id, imported_at) VALUES (?, ?, ?, ?, ?)`, item.SourceOrigin, item.SourceTripID, item.SourceContentHash, trip.ID, nowText); err != nil {
			return nil, false, fmt.Errorf("record imported share source: %w", err)
		}
		result = append(result, ImportedTripResult{ID: trip.ID, Title: trip.Title, Status: trip.Status, StartDate: trip.DateRange.Start, EndDate: trip.DateRange.End, Timezone: trip.Timezone, Revision: 1})
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		return nil, false, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO idempotency_keys(scope, key, request_hash, response_json, created_at) VALUES (?, ?, ?, ?, ?)`, scope, idempotencyKey, requestHash, string(encoded), now.Format(time.RFC3339Nano)); err != nil {
		return nil, false, err
	}
	if err := tx.Commit(); err != nil {
		return nil, false, err
	}
	return result, false, nil
}
