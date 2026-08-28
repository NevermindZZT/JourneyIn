package store

import (
	"context"
	"io/fs"
	"path/filepath"
	"testing"

	journeyin "journeyin"
	journeysync "journeyin/internal/sync"
)

func TestPushChangeAppliesTripPayloadAndPullsCursor(t *testing.T) {
	migrations, err := fs.Sub(journeyin.MigrationFS, "migrations")
	if err != nil {
		t.Fatal(err)
	}
	database, err := Open(context.Background(), filepath.Join(t.TempDir(), "sync.db"), migrations)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	payload := []byte(`{
      "schema_version": 1,
      "title": "同步行程",
      "status": "draft",
      "timezone": "Asia/Shanghai",
      "date_range": {"start":"2026-04-18","end":"2026-04-18"},
      "days": [{"id":"day-1","date":"2026-04-18","stops":[{"id":"stop-1","sequence":1,"title":"西湖"}]}]
    }`)
	change := journeysync.Change{ChangeID: "change-1", AggregateID: "trip-sync", DeviceID: "device-a", Operation: "upsert", BaseRevision: 0, NewRevision: 1, Hash: "hash-1", Payload: payload}
	accepted, err := database.PushChange(context.Background(), change)
	if err != nil {
		t.Fatal(err)
	}
	if accepted.Sequence != 1 {
		t.Fatalf("unexpected sequence: %d", accepted.Sequence)
	}
	record, err := database.GetTrip(context.Background(), "trip-sync")
	if err != nil {
		t.Fatal(err)
	}
	if record.Revision != 1 || record.Title != "同步行程" {
		t.Fatalf("trip was not applied: %+v", record)
	}
	changes, next, err := database.PullChanges(context.Background(), "trip-sync", 0, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 1 || next != 1 {
		t.Fatalf("unexpected pull: changes=%+v next=%d", changes, next)
	}
}
