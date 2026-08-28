package application

import (
	"context"
	"io/fs"
	"path/filepath"
	"testing"

	journeyin "journeyin"
	"journeyin/internal/store"
)

func testService(t *testing.T) *TripService {
	t.Helper()
	migrations, err := fs.Sub(journeyin.MigrationFS, "migrations")
	if err != nil {
		t.Fatal(err)
	}
	db, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "journeyin.db"), migrations)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return NewTripService(db)
}

const validTrip = `{
  "schema_version": 1,
  "title": "测试行程",
  "status": "draft",
  "timezone": "Asia/Shanghai",
  "date_range": {"start":"2026-04-18","end":"2026-04-18"},
  "days": [{"id":"day-1","date":"2026-04-18","stops":[{"id":"stop-1","sequence":1,"title":"西湖"}]}]
}`

func TestPreviewCommitCreateIsIdempotent(t *testing.T) {
	service := testService(t)
	preview, err := service.PreviewSave(context.Background(), []byte(validTrip), "create", "", 0, "test")
	if err != nil {
		t.Fatal(err)
	}
	if !preview.RequiresConfirmation || preview.PreviewID == "" || preview.ConfirmationToken == "" {
		t.Fatalf("invalid preview: %+v", preview)
	}
	first, err := service.CommitSave(context.Background(), preview.PreviewID, preview.ConfirmationToken, "idem-1234567890", 0, "test")
	if err != nil {
		t.Fatal(err)
	}
	replay, err := service.CommitSave(context.Background(), preview.PreviewID, preview.ConfirmationToken, "idem-1234567890", 0, "test")
	if err != nil {
		t.Fatal(err)
	}
	if replay.Status != "already_applied" || replay.TripID != first.TripID {
		t.Fatalf("unexpected replay: first=%+v replay=%+v", first, replay)
	}
}

func TestCommitRejectsWrongConfirmation(t *testing.T) {
	service := testService(t)
	preview, err := service.PreviewSave(context.Background(), []byte(validTrip), "create", "", 0, "test")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.CommitSave(context.Background(), preview.PreviewID, "wrong", "idem-abcdefghijk", 0, "test"); err == nil {
		t.Fatal("expected wrong confirmation to fail")
	}
}
