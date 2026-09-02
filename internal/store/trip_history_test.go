package store

import (
	"context"
	"encoding/json"
	"io/fs"
	"path/filepath"
	"testing"

	journeyin "journeyin"
	"journeyin/internal/domain"
)

func TestWorkingTripWritesDoNotCreateAutomaticFullRevisionSnapshots(t *testing.T) {
	migrations, err := fs.Sub(journeyin.MigrationFS, "migrations")
	if err != nil {
		t.Fatal(err)
	}
	database, err := Open(context.Background(), filepath.Join(t.TempDir(), "history.db"), migrations)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	document, err := json.Marshal(domain.Trip{SchemaVersion: 1, Title: "自动版本测试", Status: "draft", Timezone: "Asia/Shanghai", DateRange: domain.DateRange{Start: "2026-04-18", End: "2026-04-18"}, Days: []domain.Day{{ID: "day-1", Date: "2026-04-18", Stops: []domain.Stop{}}}})
	if err != nil {
		t.Fatal(err)
	}
	created, err := database.CreateTrip(context.Background(), document, "test")
	if err != nil {
		t.Fatal(err)
	}
	var trip domain.Trip
	if err := json.Unmarshal(created.Document, &trip); err != nil {
		t.Fatal(err)
	}
	trip.Title = "修改后的行程"
	updated, err := json.Marshal(trip)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.ReplaceTrip(context.Background(), created.ID, created.Revision, updated, "test"); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := database.db.QueryRowContext(context.Background(), "SELECT COUNT(1) FROM trip_revisions WHERE trip_id = ?", created.ID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("automatic revision snapshots should be disabled, count=%d", count)
	}
}
