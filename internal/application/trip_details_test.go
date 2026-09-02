package application

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"journeyin/internal/domain"
	"journeyin/internal/store"
)

func createTripForDetails(t *testing.T, service *TripService, title string, days []domain.Day) store.TripRecord {
	t.Helper()
	trip := domain.Trip{
		SchemaVersion: 1,
		Title:         title,
		Status:        "draft",
		Timezone:      "Asia/Shanghai",
		DateRange:     domain.DateRange{Start: days[0].Date, End: days[len(days)-1].Date},
		Days:          days,
	}
	document, err := json.Marshal(trip)
	if err != nil {
		t.Fatal(err)
	}
	record, err := service.Create(context.Background(), document, "test")
	if err != nil {
		t.Fatal(err)
	}
	return record
}

func TestUpdateTripDetailsRenamesShiftsAddsDaysAndClearsWeather(t *testing.T) {
	service := testService(t)
	record := createTripForDetails(t, service, "旧行程", []domain.Day{
		{ID: "day-1", Date: "2026-04-18", Stops: []domain.Stop{{ID: "stop-1", Sequence: 1, Title: "西湖", Weather: json.RawMessage("{\"condition\":\"晴\"}"), Children: []domain.SubStop{{ID: "sub-1", Sequence: 1, Title: "断桥", Weather: json.RawMessage("{\"condition\":\"多云\"}")}}}}, Legs: []domain.RouteLeg{{ID: "leg-1", FromStopID: "stop-1", ToStopID: "stop-2"}}},
		{ID: "day-2", Date: "2026-04-19", Stops: []domain.Stop{{ID: "stop-2", Sequence: 1, Title: "苏堤"}}},
	})

	title := "杭州春日慢游"
	dateRange := domain.DateRange{Start: "2026-04-25", End: "2026-04-27"}
	updated, changes, err := service.UpdateTripDetails(context.Background(), record.ID, record.Revision, UpdateTripDetailsInput{Title: &title, DateRange: &dateRange}, "test:update_trip_details")
	if err != nil {
		t.Fatal(err)
	}
	if updated.Revision != 2 {
		t.Fatalf("revision=%d, want 2", updated.Revision)
	}
	if !changes.Changed || !changes.TitleChanged || !changes.DateRangeChanged || changes.AddedDays != 1 || changes.RemovedDays != 0 || changes.ClearedWeatherStops != 2 {
		t.Fatalf("unexpected changes: %+v", changes)
	}

	var trip domain.Trip
	if err := json.Unmarshal(updated.Document, &trip); err != nil {
		t.Fatal(err)
	}
	if trip.Title != title || trip.DateRange != dateRange || len(trip.Days) != 3 {
		t.Fatalf("unexpected trip basics: title=%q range=%+v days=%d", trip.Title, trip.DateRange, len(trip.Days))
	}
	if trip.Days[0].ID != "day-1" || trip.Days[1].ID != "day-2" || trip.Days[0].Date != "2026-04-25" || trip.Days[1].Date != "2026-04-26" || trip.Days[2].Date != "2026-04-27" {
		t.Fatalf("days were not rebased: %+v", trip.Days)
	}
	if len(trip.Days[0].Stops) != 1 || trip.Days[0].Stops[0].ID != "stop-1" || len(trip.Days[0].Stops[0].Children) != 1 {
		t.Fatalf("stop identity was not preserved: %+v", trip.Days[0].Stops)
	}
	if trip.Days[0].Stops[0].Weather != nil || trip.Days[0].Stops[0].Children[0].Weather != nil {
		t.Fatal("weather snapshots on shifted days should be cleared")
	}
	if len(trip.Days[0].Legs) != 1 || trip.Days[2].Stops == nil {
		t.Fatal("route or empty-day data was not preserved")
	}
}

func TestUpdateTripDetailsRejectsRemovingDaysWithStops(t *testing.T) {
	service := testService(t)
	record := createTripForDetails(t, service, "三日行程", []domain.Day{
		{ID: "day-1", Date: "2026-04-18", Stops: []domain.Stop{}},
		{ID: "day-2", Date: "2026-04-19", Stops: []domain.Stop{}},
		{ID: "day-3", Date: "2026-04-20", Stops: []domain.Stop{{ID: "stop-3", Sequence: 1, Title: "灵隐寺"}}},
	})
	rangeValue := domain.DateRange{Start: "2026-04-18", End: "2026-04-19"}
	_, _, err := service.UpdateTripDetails(context.Background(), record.ID, record.Revision, UpdateTripDetailsInput{DateRange: &rangeValue}, "test:update_trip_details")
	var conflict *DateRangeConflictError
	if !errors.As(err, &conflict) || len(conflict.Days) != 1 || conflict.Days[0].DayID != "day-3" || conflict.Days[0].StopCount != 1 {
		t.Fatalf("expected date range conflict, got %v", err)
	}
	current, err := service.Get(context.Background(), record.ID)
	if err != nil {
		t.Fatal(err)
	}
	if current.Revision != record.Revision {
		t.Fatalf("conflicting update changed revision: %d", current.Revision)
	}
}

func TestUpdateTripDetailsCanRemoveEmptyTailAndNoOps(t *testing.T) {
	service := testService(t)
	record := createTripForDetails(t, service, "三日行程", []domain.Day{
		{ID: "day-1", Date: "2026-04-18", Stops: []domain.Stop{}},
		{ID: "day-2", Date: "2026-04-19", Stops: []domain.Stop{}},
		{ID: "day-3", Date: "2026-04-20", Stops: []domain.Stop{}},
	})
	rangeValue := domain.DateRange{Start: "2026-04-18", End: "2026-04-19"}
	updated, changes, err := service.UpdateTripDetails(context.Background(), record.ID, record.Revision, UpdateTripDetailsInput{DateRange: &rangeValue}, "test:update_trip_details")
	if err != nil {
		t.Fatal(err)
	}
	if updated.Revision != 2 || changes.RemovedDays != 1 {
		t.Fatalf("unexpected tail removal: revision=%d changes=%+v", updated.Revision, changes)
	}
	title := "三日行程"
	noOp, noOpChanges, err := service.UpdateTripDetails(context.Background(), record.ID, updated.Revision, UpdateTripDetailsInput{Title: &title}, "test:update_trip_details")
	if err != nil {
		t.Fatal(err)
	}
	if noOp.Revision != updated.Revision || noOpChanges.Changed {
		t.Fatalf("no-op should not create revision: revision=%d changes=%+v", noOp.Revision, noOpChanges)
	}
}

func TestUpdateTripDetailsValidatesInputAndRevision(t *testing.T) {
	service := testService(t)
	record := createTripForDetails(t, service, "测试行程", []domain.Day{{ID: "day-1", Date: "2026-04-18", Stops: []domain.Stop{}}})
	empty := "   "
	if _, _, err := service.UpdateTripDetails(context.Background(), record.ID, record.Revision, UpdateTripDetailsInput{Title: &empty}, "test:update_trip_details"); err == nil {
		t.Fatal("expected empty title error")
	}
	badRange := domain.DateRange{Start: "2026-04-20", End: "2026-04-19"}
	if _, _, err := service.UpdateTripDetails(context.Background(), record.ID, record.Revision, UpdateTripDetailsInput{DateRange: &badRange}, "test:update_trip_details"); err == nil {
		t.Fatal("expected invalid date range error")
	}
	newTitle := "新名称"
	if _, _, err := service.UpdateTripDetails(context.Background(), record.ID, record.Revision+1, UpdateTripDetailsInput{Title: &newTitle}, "test:update_trip_details"); !errors.Is(err, store.ErrRevisionConflict) {
		t.Fatalf("expected revision conflict, got %v", err)
	}
}
