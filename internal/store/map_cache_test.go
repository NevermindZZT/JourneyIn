package store

import (
	"context"
	"errors"
	"io/fs"
	"path/filepath"
	"testing"
	"time"

	journeyin "journeyin"
)

func TestMapCacheAndQuotaSurviveReopen(t *testing.T) {
	migrations, err := fs.Sub(journeyin.MigrationFS, "migrations")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "journeyin.db")
	database, err := Open(context.Background(), path, migrations)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	if err := database.PutMapCache(context.Background(), "baidu", "poi_search", "cache-key", []byte(`{"items":[]}`), now.Add(time.Hour), now); err != nil {
		t.Fatal(err)
	}
	if err := database.ReserveMapRequest(context.Background(), "baidu", "2026-04-18", 2); err != nil {
		t.Fatal(err)
	}
	if err := database.ReserveMapRequest(context.Background(), "baidu", "2026-04-18", 2); err != nil {
		t.Fatal(err)
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}
	database, err = Open(context.Background(), path, migrations)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	entry, ok, err := database.GetMapCache(context.Background(), "baidu", "poi_search", "cache-key")
	if err != nil || !ok || string(entry.ResponseJSON) != `{"items":[]}` {
		t.Fatalf("cache did not survive reopen: ok=%v err=%v entry=%s", ok, err, entry.ResponseJSON)
	}
	if err := database.ReserveMapRequest(context.Background(), "baidu", "2026-04-18", 2); !errors.Is(err, ErrMapQuotaExceeded) {
		t.Fatalf("expected quota error, got %v", err)
	}
}
