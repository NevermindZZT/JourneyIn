package application

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"journeyin/internal/domain"
	journeymaps "journeyin/internal/maps"
	"journeyin/internal/store"
)

func weatherBatchTrip(t *testing.T, service *TripService) store.TripRecord {
	t.Helper()
	location := json.RawMessage("{\"preferred\":\"bd09ll\",\"coordinates\":{\"bd09ll\":{\"lat\":30.2,\"lng\":120.1,\"crs\":\"bd09ll\"}}}")
	stops := make([]domain.Stop, 7)
	for i := range stops {
		stops[i] = domain.Stop{ID: "stop-" + string(rune('a'+i)), Sequence: i + 1, Title: "地点", Location: location}
	}
	stops[0].Children = []domain.SubStop{{ID: "child-a", Sequence: 1, Title: "子点", Location: location}}
	stops[1].Location = nil
	stops[0].Weather = json.RawMessage(`{"provider":"fake","condition":"旧天气","available":true}`)
	return createTripForDetails(t, service, "天气批量", []domain.Day{{ID: "day-1", Date: "2026-04-18", Stops: stops}})
}

func TestRefreshTripWeatherBatchPagesAndSkipsMissingLocation(t *testing.T) {
	service := testService(t)
	service.SetMapService(NewMapService(service.store, journeymaps.NewRegistry(&fakePlanningProvider{}), 2, 0))
	record := weatherBatchTrip(t, service)
	first, progress, err := service.RefreshTripWeatherBatch(context.Background(), record.ID, record.Revision, 0, WeatherInput{Provider: "fake"}, "test")
	if err != nil {
		t.Fatal(err)
	}
	if progress.Total != 8 || progress.NextOffset != 5 || progress.Updated != 4 || progress.Skipped != 1 || progress.Failed != 0 || first.Revision != record.Revision+1 {
		t.Fatalf("unexpected first batch: %+v revision=%d", progress, first.Revision)
	}
	if progress.Items[1].StopID != "child-a" || progress.Items[1].Status != "updated" || progress.Items[2].Status != "skipped" {
		t.Fatalf("unexpected child/skip order: %+v", progress.Items)
	}
	second, rest, err := service.RefreshTripWeatherBatch(context.Background(), record.ID, first.Revision, progress.NextOffset, WeatherInput{Provider: "fake"}, "test")
	if err != nil {
		t.Fatal(err)
	}
	if rest.NextOffset != 8 || rest.Updated != 3 || second.Revision != first.Revision+1 {
		t.Fatalf("unexpected final batch: %+v revision=%d", rest, second.Revision)
	}
	var saved domain.Trip
	if err := json.Unmarshal(second.Document, &saved); err != nil {
		t.Fatal(err)
	}
	for i, point := range saved.Days[0].Stops {
		if i == 1 {
			if len(point.Weather) != 0 {
				t.Fatal("unlocated stop gained weather")
			}
			continue
		}
		if len(point.Weather) == 0 {
			t.Fatalf("missing weather on %s", point.ID)
		}
	}
	if len(saved.Days[0].Stops[0].Children[0].Weather) == 0 {
		t.Fatal("missing child weather")
	}
	if _, _, err := service.RefreshTripWeatherBatch(context.Background(), record.ID, record.Revision, 0, WeatherInput{Provider: "fake"}, "test"); !errors.Is(err, store.ErrRevisionConflict) {
		t.Fatalf("expected stale revision conflict, got %v", err)
	}
}

func TestRefreshTripWeatherBatchUnavailableKeepsRevision(t *testing.T) {
	service := testService(t)
	service.SetMapService(NewMapService(service.store, journeymaps.NewRegistry(&fakePlanningProvider{weatherUnavailable: true}), 2, 0))
	record := weatherBatchTrip(t, service)
	next, progress, err := service.RefreshTripWeatherBatch(context.Background(), record.ID, record.Revision, 0, WeatherInput{Provider: "fake"}, "test")
	if err != nil {
		t.Fatal(err)
	}
	if next.Revision != record.Revision || progress.Updated != 0 || progress.Failed != 4 || progress.Skipped != 1 {
		t.Fatalf("unexpected unavailable batch: %+v revision=%d", progress, next.Revision)
	}
	var saved domain.Trip
	if err := json.Unmarshal(next.Document, &saved); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(saved.Days[0].Stops[0].Weather), "旧天气") {
		t.Fatal("failed refresh discarded existing weather")
	}
	if _, _, err := service.RefreshTripWeatherBatch(context.Background(), record.ID, record.Revision, -1, WeatherInput{}, "test"); err == nil {
		t.Fatal("negative offset accepted")
	}
}
