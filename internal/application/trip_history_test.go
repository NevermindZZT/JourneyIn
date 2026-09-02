package application

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"journeyin/internal/domain"
	"journeyin/internal/store"
)

func TestTripHistoryRequiresExplicitSaveAndKeepsSnapshotImmutable(t *testing.T) {
	service := testService(t)
	record := createTripForDetails(t, service, "初始行程", []domain.Day{{ID: "day-1", Date: "2026-04-18", Stops: []domain.Stop{{ID: "stop-1", Sequence: 1, Title: "西湖"}}}})

	versions, err := service.ListTripVersions(context.Background(), record.ID, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(versions) != 0 {
		t.Fatalf("ordinary trip creation should not create saved history: %+v", versions)
	}

	var trip domain.Trip
	if err := json.Unmarshal(record.Document, &trip); err != nil {
		t.Fatal(err)
	}
	trip.Title = "第一次修改"
	updatedDocument, err := json.Marshal(trip)
	if err != nil {
		t.Fatal(err)
	}
	updated, err := service.Replace(context.Background(), record.ID, record.Revision, updatedDocument, "test:update")
	if err != nil {
		t.Fatal(err)
	}
	if updated.Revision != 2 {
		t.Fatalf("revision=%d, want 2", updated.Revision)
	}
	versions, err = service.ListTripVersions(context.Background(), record.ID, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(versions) != 0 {
		t.Fatalf("ordinary trip edit should not create saved history: %+v", versions)
	}

	first, alreadySaved, replayed, err := service.SaveTripVersionIdempotent(context.Background(), record.ID, updated.Revision, "编辑后版本", "history-save-1")
	if err != nil {
		t.Fatal(err)
	}
	if alreadySaved || replayed || first.ID == "" || first.SourceRevision != 2 || first.Title != "第一次修改" {
		t.Fatalf("unexpected saved version: version=%+v already=%v replayed=%v", first, alreadySaved, replayed)
	}

	replay, alreadySaved, replayed, err := service.SaveTripVersionIdempotent(context.Background(), record.ID, updated.Revision, "编辑后版本", "history-save-1")
	if err != nil {
		t.Fatal(err)
	}
	if !replayed || alreadySaved || replay.ID != first.ID {
		t.Fatalf("unexpected history replay: first=%+v replay=%+v already=%v replayed=%v", first, replay, alreadySaved, replayed)
	}

	duplicate, alreadySaved, replayed, err := service.SaveTripVersionIdempotent(context.Background(), record.ID, updated.Revision, "重复点击", "history-save-2")
	if err != nil {
		t.Fatal(err)
	}
	if !alreadySaved || replayed || duplicate.ID != first.ID {
		t.Fatalf("same content should reuse history: first=%+v duplicate=%+v already=%v replayed=%v", first, duplicate, alreadySaved, replayed)
	}

	history, err := service.GetTripVersion(context.Background(), record.ID, first.ID)
	if err != nil {
		t.Fatal(err)
	}
	if string(history.Document) == "" {
		t.Fatal("history document is empty")
	}
	var savedTrip domain.Trip
	if err := json.Unmarshal(history.Document, &savedTrip); err != nil {
		t.Fatal(err)
	}
	if savedTrip.Title != "第一次修改" {
		t.Fatalf("saved history title=%q", savedTrip.Title)
	}

	savedTrip.Title = "当前第三次修改"
	currentDocument, err := json.Marshal(savedTrip)
	if err != nil {
		t.Fatal(err)
	}
	current, err := service.Replace(context.Background(), record.ID, updated.Revision, currentDocument, "test:update-again")
	if err != nil {
		t.Fatal(err)
	}
	if current.Revision != 3 {
		t.Fatalf("revision=%d, want 3", current.Revision)
	}
	unchanged, err := service.GetTripVersion(context.Background(), record.ID, first.ID)
	if err != nil {
		t.Fatal(err)
	}
	var unchangedTrip domain.Trip
	if err := json.Unmarshal(unchanged.Document, &unchangedTrip); err != nil {
		t.Fatal(err)
	}
	if unchangedTrip.Title != "第一次修改" {
		t.Fatalf("history was mutated by current edit: %q", unchangedTrip.Title)
	}

	if _, _, _, err := service.SaveTripVersionIdempotent(context.Background(), record.ID, 1, "过期版本", "history-stale"); !errors.Is(err, store.ErrRevisionConflict) {
		t.Fatalf("expected stale history save conflict, got %v", err)
	}
	if replayed, err := service.DeleteTripVersionIdempotent(context.Background(), record.ID, first.ID, "history-delete-1"); err != nil || replayed {
		t.Fatalf("unexpected first history delete: replayed=%v err=%v", replayed, err)
	}
	if replayed, err := service.DeleteTripVersionIdempotent(context.Background(), record.ID, first.ID, "history-delete-1"); err != nil || !replayed {
		t.Fatalf("unexpected history delete replay: replayed=%v err=%v", replayed, err)
	}
	versions, err = service.ListTripVersions(context.Background(), record.ID, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(versions) != 0 {
		t.Fatalf("history should be deleted: %+v", versions)
	}
	final, err := service.Get(context.Background(), record.ID)
	if err != nil {
		t.Fatal(err)
	}
	if final.Revision != 3 || final.Title != "当前第三次修改" {
		t.Fatalf("deleting history changed current trip: %+v", final)
	}
}

func TestTripHistoryLabelLimit(t *testing.T) {
	service := testService(t)
	record := createTripForDetails(t, service, "标签限制", []domain.Day{{ID: "day-1", Date: "2026-04-18", Stops: []domain.Stop{}}})
	longLabel := "一"
	for i := 0; i < 120; i++ {
		longLabel += "一"
	}
	if _, _, _, err := service.SaveTripVersionIdempotent(context.Background(), record.ID, record.Revision, longLabel, "history-label-too-long"); !errors.Is(err, ErrSavedTripVersionLabelTooLong) {
		t.Fatalf("expected label limit error, got %v", err)
	}
}
