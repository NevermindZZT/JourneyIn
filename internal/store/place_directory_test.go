package store

import (
	"context"
	"io/fs"
	"path/filepath"
	"testing"
	"time"

	journeyin "journeyin"
)

func TestPlaceDirectoryExpiresAndClears(t *testing.T) {
	migrations, err := fs.Sub(journeyin.MigrationFS, "migrations")
	if err != nil {
		t.Fatal(err)
	}
	db, err := Open(context.Background(), filepath.Join(t.TempDir(), "journeyin.db"), migrations)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	now := time.Now().UTC()
	item := PlaceDirectoryRecord{Provider: "amap", ProviderID: "poi-1", Name: "白石崖", Address: "甘肃省", Region: "甘肃省", Category: "旅游景点", LocationJSON: []byte(`{"lat":35.4,"lng":102.5,"crs":"gcj02"}`), CreatedAt: now, LastSeenAt: now, ExpiresAt: now.Add(7 * 24 * time.Hour)}
	if err := db.UpsertPlaceDirectory(context.Background(), item); err != nil {
		t.Fatal(err)
	}
	items, err := db.FindPlaceDirectory(context.Background(), "白石", "甘肃", "旅游景点", 20)
	if err != nil || len(items) != 1 {
		t.Fatalf("items=%+v err=%v", items, err)
	}
	item.ExpiresAt = now.Add(-time.Minute)
	if err := db.UpsertPlaceDirectory(context.Background(), item); err != nil {
		t.Fatal(err)
	}
	if err := db.PurgeExpiredPlaceDirectory(context.Background(), now); err != nil {
		t.Fatal(err)
	}
	items, err = db.FindPlaceDirectory(context.Background(), "白石", "", "", 20)
	if err != nil || len(items) != 0 {
		t.Fatalf("expired items=%+v err=%v", items, err)
	}
	if err := db.UpsertPlaceDirectory(context.Background(), item); err != nil {
		t.Fatal(err)
	}
	if err := db.ClearPlaceDirectory(context.Background()); err != nil {
		t.Fatal(err)
	}
	count, err := db.PlaceDirectoryCount(context.Background())
	if err != nil || count != 0 {
		t.Fatalf("count=%d err=%v", count, err)
	}
}
