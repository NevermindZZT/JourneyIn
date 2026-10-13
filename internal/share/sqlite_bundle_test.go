package share

import (
	"context"
	"errors"
	"io/fs"
	"path/filepath"
	"testing"
	"time"

	journeyin "journeyin"
	"journeyin/internal/store"
)

func TestSQLiteRevokeDeletesBundleRowAndSnapshot(t *testing.T) {
	migrations, err := fs.Sub(journeyin.MigrationFS, "migrations")
	if err != nil {
		t.Fatal(err)
	}
	database, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "share-bundle.db"), migrations)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	service := NewBundleService(NewSQLiteBundleStore(database))
	trip := []byte(`{"schema_version":1,"id":"trip_a","title":"A","status":"draft","timezone":"Asia/Shanghai","date_range":{"start":"2026-10-13","end":"2026-10-13"},"days":[{"id":"day_a","date":"2026-10-13","stops":[{"id":"stop_a","sequence":1,"title":"Place"}]}]}`)
	_, snapshot, err := NewBundleDocument([]BundleItem{{TripID: "trip_a", ContentHash: BundleContentHash(trip), Trip: trip}}, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	token, record, err := service.Create(snapshot, "owner", 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := service.Revoke(record.ID, "owner", false); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Resolve(token); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected share token to resolve as not found after delete, got %v", err)
	}
	if _, err := database.GetShareBundleByTokenHash(context.Background(), record.TokenHash); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("revoked bundle row and snapshot still exist: %v", err)
	}
}
