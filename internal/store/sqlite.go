package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"journeyin/internal/domain"
	_ "modernc.org/sqlite"
)

type Store struct{ db *sql.DB }

type TripRecord struct {
	ID          string
	Title       string
	Status      string
	StartDate   string
	EndDate     string
	Timezone    string
	Revision    int
	Document    []byte
	ContentHash string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

var ErrNotFound = errors.New("trip not found")
var ErrRevisionConflict = errors.New("trip revision conflict")

func Open(ctx context.Context, path string, migrations fs.FS) (*Store, error) {
	if path != ":memory:" {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return nil, fmt.Errorf("create database directory: %w", err)
		}
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	db.SetMaxOpenConns(4)
	db.SetMaxIdleConns(4)
	s := &Store{db: db}
	for _, pragma := range []string{"PRAGMA foreign_keys = ON", "PRAGMA journal_mode = WAL", "PRAGMA busy_timeout = 5000", "PRAGMA synchronous = NORMAL"} {
		if _, err := db.ExecContext(ctx, pragma); err != nil {
			_ = db.Close()
			return nil, fmt.Errorf("sqlite %s: %w", pragma, err)
		}
	}
	if err := s.applyMigrations(ctx, migrations); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) applyMigrations(ctx context.Context, migrations fs.FS) error {
	if _, err := s.db.ExecContext(ctx, "CREATE TABLE IF NOT EXISTS schema_migrations (version TEXT PRIMARY KEY, applied_at TEXT NOT NULL)"); err != nil {
		return fmt.Errorf("create migration table: %w", err)
	}
	entries, err := fs.ReadDir(migrations, ".")
	if err != nil {
		return fmt.Errorf("read migrations: %w", err)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		var exists int
		if err := s.db.QueryRowContext(ctx, "SELECT COUNT(1) FROM schema_migrations WHERE version = ?", entry.Name()).Scan(&exists); err != nil {
			return fmt.Errorf("check migration %s: %w", entry.Name(), err)
		}
		if exists > 0 {
			continue
		}
		contents, err := fs.ReadFile(migrations, entry.Name())
		if err != nil {
			return fmt.Errorf("read migration %s: %w", entry.Name(), err)
		}
		tx, err := s.db.BeginTx(ctx, nil)
		if err != nil {
			return fmt.Errorf("begin migration %s: %w", entry.Name(), err)
		}
		if _, err := tx.ExecContext(ctx, string(contents)); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("apply migration %s: %w", entry.Name(), err)
		}
		if _, err := tx.ExecContext(ctx, "INSERT INTO schema_migrations(version, applied_at) VALUES (?, ?)", entry.Name(), time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("record migration %s: %w", entry.Name(), err)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit migration %s: %w", entry.Name(), err)
		}
	}
	return nil
}

func (s *Store) CreateTrip(ctx context.Context, document []byte, source string) (TripRecord, error) {
	normalized, trip, issues, err := domain.NormalizeTrip(document)
	if err != nil {
		return TripRecord{}, err
	}
	if hasErrors(issues) {
		return TripRecord{}, fmt.Errorf("trip validation failed: %s", issues[0].Message)
	}
	now := time.Now().UTC()
	hash := domain.ContentHash(normalized)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return TripRecord{}, err
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(ctx, "INSERT INTO trips(id, title, status, start_date, end_date, timezone, revision, document_json, content_hash, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, 1, ?, ?, ?, ?)", trip.ID, trip.Title, trip.Status, trip.DateRange.Start, trip.DateRange.End, trip.Timezone, string(normalized), hash, now.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano))
	if err != nil {
		return TripRecord{}, fmt.Errorf("insert trip: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return TripRecord{}, err
	}
	return TripRecord{ID: trip.ID, Title: trip.Title, Status: trip.Status, StartDate: trip.DateRange.Start, EndDate: trip.DateRange.End, Timezone: trip.Timezone, Revision: 1, Document: normalized, ContentHash: hash, CreatedAt: now, UpdatedAt: now}, nil
}

func (s *Store) GetTrip(ctx context.Context, id string) (TripRecord, error) {
	var r TripRecord
	var created, updated string
	err := s.db.QueryRowContext(ctx, "SELECT id, title, status, start_date, end_date, timezone, revision, document_json, content_hash, created_at, updated_at FROM trips WHERE id = ?", id).Scan(&r.ID, &r.Title, &r.Status, &r.StartDate, &r.EndDate, &r.Timezone, &r.Revision, &r.Document, &r.ContentHash, &created, &updated)
	if errors.Is(err, sql.ErrNoRows) {
		return TripRecord{}, ErrNotFound
	}
	if err != nil {
		return TripRecord{}, err
	}
	r.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
	r.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updated)
	return r, nil
}

func (s *Store) ListTrips(ctx context.Context, limit int) ([]TripRecord, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	rows, err := s.db.QueryContext(ctx, "SELECT id, title, status, start_date, end_date, timezone, revision, document_json, content_hash, created_at, updated_at FROM trips ORDER BY updated_at DESC LIMIT ?", limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []TripRecord
	for rows.Next() {
		var r TripRecord
		var created, updated string
		if err := rows.Scan(&r.ID, &r.Title, &r.Status, &r.StartDate, &r.EndDate, &r.Timezone, &r.Revision, &r.Document, &r.ContentHash, &created, &updated); err != nil {
			return nil, err
		}
		r.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
		r.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updated)
		result = append(result, r)
	}
	return result, rows.Err()
}

func (s *Store) ReplaceTrip(ctx context.Context, id string, expectedRevision int, document []byte, source string) (TripRecord, error) {
	normalized, trip, issues, err := domain.NormalizeTripForID(document, id)
	if err != nil {
		return TripRecord{}, err
	}
	if hasErrors(issues) {
		return TripRecord{}, fmt.Errorf("trip validation failed: %s", issues[0].Message)
	}
	hash := domain.ContentHash(normalized)
	now := time.Now().UTC()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return TripRecord{}, err
	}
	defer tx.Rollback()
	var current int
	if err := tx.QueryRowContext(ctx, "SELECT revision FROM trips WHERE id = ?", id).Scan(&current); errors.Is(err, sql.ErrNoRows) {
		return TripRecord{}, ErrNotFound
	} else if err != nil {
		return TripRecord{}, err
	}
	if current != expectedRevision {
		return TripRecord{}, ErrRevisionConflict
	}
	if _, err := tx.ExecContext(ctx, "UPDATE trips SET title=?, status=?, start_date=?, end_date=?, timezone=?, revision=?, document_json=?, content_hash=?, updated_at=? WHERE id=? AND revision=?", trip.Title, trip.Status, trip.DateRange.Start, trip.DateRange.End, trip.Timezone, current+1, string(normalized), hash, now.Format(time.RFC3339Nano), id, current); err != nil {
		return TripRecord{}, err
	}
	if err := tx.Commit(); err != nil {
		return TripRecord{}, err
	}
	return TripRecord{ID: id, Title: trip.Title, Status: trip.Status, StartDate: trip.DateRange.Start, EndDate: trip.DateRange.End, Timezone: trip.Timezone, Revision: current + 1, Document: normalized, ContentHash: hash, UpdatedAt: now}, nil
}

func (s *Store) DeleteTrip(ctx context.Context, id string, expectedRevision int) error {
	result, err := s.db.ExecContext(ctx, "DELETE FROM trips WHERE id = ? AND revision = ?", id, expectedRevision)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return ErrRevisionConflict
	}
	return nil
}

func hasErrors(issues []domain.ValidationIssue) bool {
	for _, issue := range issues {
		if issue.Level == "error" {
			return true
		}
	}
	return false
}
