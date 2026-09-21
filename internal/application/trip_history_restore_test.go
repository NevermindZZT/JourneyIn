package application

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"journeyin/internal/domain"
	"journeyin/internal/store"
)

func TestRestoreTripVersionReplacesWorkingTripAndKeepsSnapshot(t *testing.T) {
	service := testService(t)
	record := createTripForDetails(t, service, "历史初始", []domain.Day{{ID: "day-1", Date: "2026-04-18", Stops: []domain.Stop{}}})
	saved, alreadySaved, replayed, err := service.SaveTripVersionIdempotent(context.Background(), record.ID, record.Revision, "初始版本", "history-save-initial")
	if err != nil || alreadySaved || replayed {
		t.Fatalf("save history: version=%+v already=%v replayed=%v err=%v", saved, alreadySaved, replayed, err)
	}

	var current domain.Trip
	if err := json.Unmarshal(record.Document, &current); err != nil {
		t.Fatal(err)
	}
	current.Title = "当前修改"
	updatedDocument, err := json.Marshal(current)
	if err != nil {
		t.Fatal(err)
	}
	updated, err := service.Replace(context.Background(), record.ID, record.Revision, updatedDocument, "test:update")
	if err != nil {
		t.Fatal(err)
	}

	restored, replayed, err := service.RestoreTripVersionIdempotent(context.Background(), record.ID, saved.ID, updated.Revision, "history-restore-1")
	if err != nil {
		t.Fatal(err)
	}
	if replayed || restored.ID != record.ID || restored.Revision != 3 || restored.Title != "历史初始" {
		t.Fatalf("unexpected restored record: %+v replayed=%v", restored, replayed)
	}
	var restoredDocument domain.Trip
	if err := json.Unmarshal(restored.Document, &restoredDocument); err != nil {
		t.Fatal(err)
	}
	if restoredDocument.ID != record.ID || restoredDocument.Title != "历史初始" {
		t.Fatalf("restore did not retain target identity and snapshot content: %+v", restoredDocument)
	}

	replay, replayed, err := service.RestoreTripVersionIdempotent(context.Background(), record.ID, saved.ID, updated.Revision, "history-restore-1")
	if err != nil || !replayed || replay.Revision != restored.Revision || replay.Title != restored.Title {
		t.Fatalf("unexpected restore replay: record=%+v replayed=%v err=%v", replay, replayed, err)
	}
	if _, _, err := service.RestoreTripVersionIdempotent(context.Background(), record.ID, saved.ID, restored.Revision, "history-restore-1"); !errors.Is(err, store.ErrIdempotencyConflict) {
		t.Fatalf("altered restore replay error=%v, want idempotency conflict", err)
	}
	if _, _, err := service.RestoreTripVersionIdempotent(context.Background(), record.ID, saved.ID, updated.Revision, "history-restore-stale"); !errors.Is(err, store.ErrRevisionConflict) {
		t.Fatalf("stale restore error=%v, want revision conflict", err)
	}

	history, err := service.GetTripVersion(context.Background(), record.ID, saved.ID)
	if err != nil {
		t.Fatal(err)
	}
	var immutable domain.Trip
	if err := json.Unmarshal(history.Document, &immutable); err != nil {
		t.Fatal(err)
	}
	if immutable.Title != "历史初始" {
		t.Fatalf("restore mutated immutable history snapshot: %+v", immutable)
	}
}
