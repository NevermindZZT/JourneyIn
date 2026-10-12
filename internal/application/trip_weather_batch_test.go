package application

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"journeyin/internal/domain"
	journeymaps "journeyin/internal/maps"
	"journeyin/internal/store"
)

func weatherBatchTrip(t *testing.T, service *TripService) store.TripRecord {
	t.Helper()
	tripTimezone, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		t.Fatal(err)
	}
	date := time.Now().In(tripTimezone).AddDate(0, 0, 1).Format("2006-01-02")
	location := json.RawMessage("{\"preferred\":\"bd09ll\",\"coordinates\":{\"bd09ll\":{\"lat\":30.2,\"lng\":120.1,\"crs\":\"bd09ll\"}}}")
	stops := make([]domain.Stop, 7)
	for i := range stops {
		stops[i] = domain.Stop{ID: "stop-" + string(rune('a'+i)), Sequence: i + 1, Title: "地点", Location: location}
	}
	stops[0].Children = []domain.SubStop{{ID: "child-a", Sequence: 1, Title: "子点", Location: location}}
	stops[1].Location = nil
	stops[0].Weather = json.RawMessage(`{"provider":"fake","condition":"旧天气","available":true}`)
	return createTripForDetails(t, service, "天气批量", []domain.Day{{ID: "day-1", Date: date, Stops: stops}})
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

func TestRefreshTripWeatherBatchUsesCurrentWeatherForPastDays(t *testing.T) {
	service := testService(t)
	fake := &fakePlanningProvider{}
	service.SetMapService(NewMapService(service.store, journeymaps.NewRegistry(fake), 2, 0))
	location, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().In(location)
	past := now.AddDate(0, 0, -1).Format("2006-01-02")
	today := now.Format("2006-01-02")
	future := now.AddDate(0, 0, 1).Format("2006-01-02")
	pointLocation := json.RawMessage(`{"preferred":"bd09ll","coordinates":{"bd09ll":{"lat":30.2,"lng":120.1,"crs":"bd09ll"}}}`)
	record := createTripForDetails(t, service, "行程中天气刷新", []domain.Day{
		{ID: "past-day", Date: past, Stops: []domain.Stop{{ID: "past-stop", Sequence: 1, Title: "已完成地点", Location: pointLocation, Weather: json.RawMessage(`{"condition":"过去保存的天气"}`)}}},
		{ID: "today-day", Date: today, Stops: []domain.Stop{{ID: "today-stop", Sequence: 1, Title: "今天的地点", Location: pointLocation}}},
		{ID: "future-day", Date: future, Stops: []domain.Stop{{ID: "future-stop", Sequence: 1, Title: "之后的地点", Location: pointLocation}}},
	})
	updated, progress, err := service.RefreshTripWeatherBatch(context.Background(), record.ID, record.Revision, 0, WeatherInput{Provider: "fake"}, "test")
	if err != nil {
		t.Fatal(err)
	}
	if progress.Total != 3 || progress.NextOffset != 3 || progress.Updated != 3 || progress.Skipped != 0 || progress.Failed != 0 {
		t.Fatalf("unexpected progress: %+v", progress)
	}
	if progress.Items[0].StopID != "past-stop" || progress.Items[0].Status != "updated" || progress.Items[0].Reason != "行程日期已过，已改查当前天气" {
		t.Fatalf("past itinerary day was not refreshed with current weather: %+v", progress.Items[0])
	}
	if progress.Items[1].Status != "updated" || progress.Items[2].Status != "updated" || updated.Revision != record.Revision+1 {
		t.Fatalf("weather was not refreshed for all itinerary days: %+v revision=%d", progress.Items, updated.Revision)
	}
	var saved domain.Trip
	if err := json.Unmarshal(updated.Document, &saved); err != nil {
		t.Fatal(err)
	}
	var pastWeather journeymaps.WeatherSnapshot
	if err := json.Unmarshal(saved.Days[0].Stops[0].Weather, &pastWeather); err != nil {
		t.Fatal(err)
	}
	if pastWeather.LocalDate != today {
		t.Fatalf("past-day point fetched local date %q, want current date %q", pastWeather.LocalDate, today)
	}
	if len(saved.Days[1].Stops[0].Weather) == 0 || len(saved.Days[2].Stops[0].Weather) == 0 {
		t.Fatal("today or future itinerary weather was not saved")
	}
}

func TestRefreshWeatherForPastDayRequestsCurrentWeather(t *testing.T) {
	service := testService(t)
	service.SetMapService(NewMapService(service.store, journeymaps.NewRegistry(&fakePlanningProvider{}), 2, 0))
	location, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().In(location)
	today := now.Format("2006-01-02")
	past := now.AddDate(0, 0, -1).Format("2006-01-02")
	pointLocation := json.RawMessage(`{"preferred":"bd09ll","coordinates":{"bd09ll":{"lat":30.2,"lng":120.1,"crs":"bd09ll"}}}`)
	record := createTripForDetails(t, service, "已过日期单点天气", []domain.Day{{ID: "past-day", Date: past, Stops: []domain.Stop{{ID: "past-stop", Sequence: 1, Title: "已完成地点", Location: pointLocation}}}})
	updated, err := service.RefreshWeather(context.Background(), record.ID, record.Revision, "past-day", "past-stop", WeatherInput{Provider: "fake", LocalDate: past}, "test")
	if err != nil {
		t.Fatal(err)
	}
	var saved domain.Trip
	if err := json.Unmarshal(updated.Document, &saved); err != nil {
		t.Fatal(err)
	}
	var snapshot journeymaps.WeatherSnapshot
	if err := json.Unmarshal(saved.Days[0].Stops[0].Weather, &snapshot); err != nil {
		t.Fatal(err)
	}
	if snapshot.LocalDate != today {
		t.Fatalf("past-day refresh used date %q, want current date %q", snapshot.LocalDate, today)
	}
}

func TestTripLocalTodayUsesTripTimezone(t *testing.T) {
	now := time.Date(2026, 4, 17, 16, 30, 0, 0, time.UTC)
	today, err := tripLocalToday(now, "Asia/Shanghai")
	if err != nil {
		t.Fatal(err)
	}
	if today != "2026-04-18" {
		t.Fatalf("trip local date=%q, want 2026-04-18", today)
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
