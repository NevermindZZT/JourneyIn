package store

import (
	"context"
	"encoding/json"
	"errors"
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

func TestPushHistoryChangesKeepsWorkingRevisionAndUsesTombstones(t *testing.T) {
	migrations, err := fs.Sub(journeyin.MigrationFS, "migrations")
	if err != nil {
		t.Fatal(err)
	}
	database, err := Open(context.Background(), filepath.Join(t.TempDir(), "sync-history.db"), migrations)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	trip := `{"schema_version":1,"title":"同步历史","status":"draft","timezone":"Asia/Shanghai","date_range":{"start":"2026-04-18","end":"2026-04-18"},"days":[{"id":"day-1","date":"2026-04-18","stops":[]}]}`
	created, err := database.CreateTrip(context.Background(), []byte(trip), "test")
	if err != nil {
		t.Fatal(err)
	}
	historyDocument := json.RawMessage(`{"schema_version":1,"title":"同步历史旧版","status":"draft","timezone":"Asia/Shanghai","date_range":{"start":"2026-04-18","end":"2026-04-18"},"days":[{"id":"day-1","date":"2026-04-18","stops":[]}]}`)
	payload, err := json.Marshal(SavedTripVersionChange{HistoryID: "history-sync-1", SourceRevision: created.Revision, Label: "同步保存", Document: historyDocument})
	if err != nil {
		t.Fatal(err)
	}
	change := journeysync.Change{ChangeID: "history-change-1", AggregateID: created.ID, DeviceID: "device-a", Operation: journeysync.OperationHistorySave, BaseRevision: created.Revision, NewRevision: created.Revision, Hash: "history-hash-1", Payload: payload}
	if _, err := database.PushChange(context.Background(), change); err != nil {
		t.Fatal(err)
	}
	if _, err := database.PushChange(context.Background(), change); err != nil {
		t.Fatal(err)
	}
	version, err := database.GetSavedTripVersion(context.Background(), created.ID, "history-sync-1")
	if err != nil {
		t.Fatal(err)
	}
	if version.Title != "同步历史旧版" || version.SourceRevision != created.Revision {
		t.Fatalf("unexpected synced history: %+v", version)
	}
	current, err := database.GetTrip(context.Background(), created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if current.Revision != created.Revision {
		t.Fatalf("history save changed working revision: %d", current.Revision)
	}
	deletePayload, err := json.Marshal(DeletedTripVersionChange{HistoryID: "history-sync-1"})
	if err != nil {
		t.Fatal(err)
	}
	deleteChange := journeysync.Change{ChangeID: "history-delete-1", AggregateID: created.ID, DeviceID: "device-b", Operation: journeysync.OperationHistoryDelete, BaseRevision: created.Revision, NewRevision: created.Revision, Hash: "history-delete-hash", Payload: deletePayload}
	if _, err := database.PushChange(context.Background(), deleteChange); err != nil {
		t.Fatal(err)
	}
	if _, err := database.GetSavedTripVersion(context.Background(), created.ID, "history-sync-1"); !errors.Is(err, ErrSavedTripVersionNotFound) {
		t.Fatalf("expected deleted history, got %v", err)
	}
	resurrect, err := database.PushChange(context.Background(), journeysync.Change{ChangeID: "history-change-2", AggregateID: created.ID, DeviceID: "device-c", Operation: journeysync.OperationHistorySave, BaseRevision: created.Revision, NewRevision: created.Revision, Hash: "history-hash-2", Payload: payload})
	if err == nil || !errors.Is(err, journeysync.ErrConflict) || resurrect.ChangeID != "" {
		t.Fatalf("expected tombstone conflict, change=%+v err=%v", resurrect, err)
	}
}
