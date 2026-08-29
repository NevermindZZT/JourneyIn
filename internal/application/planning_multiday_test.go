package application

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	"journeyin/internal/domain"
	journeymaps "journeyin/internal/maps"
)

func multiDayStop(id, title string, sequence int, lat, lng float64) domain.Stop {
	location, _ := json.Marshal(map[string]any{
		"preferred": "bd09ll",
		"coordinates": map[string]journeymaps.GeoPoint{
			"bd09ll": {Lat: lat, Lng: lng, CRS: journeymaps.CRSBD09LL},
		},
	})
	return domain.Stop{ID: id, Sequence: sequence, Title: title, Location: location}
}

func createMultiDayTrip(t *testing.T, service *TripService, days []domain.Day) (string, int) {
	t.Helper()
	document, err := json.Marshal(domain.Trip{
		SchemaVersion: 1,
		Title:         "多天路线测试",
		Status:        "draft",
		Timezone:      "Asia/Shanghai",
		DateRange:     domain.DateRange{Start: "2026-04-18", End: "2026-04-20"},
		Days:          days,
	})
	if err != nil {
		t.Fatal(err)
	}
	record, err := service.Create(context.Background(), document, "test")
	if err != nil {
		t.Fatal(err)
	}
	return record.ID, record.Revision
}

func decodeTripDocument(t *testing.T, document []byte) domain.Trip {
	t.Helper()
	var trip domain.Trip
	if err := json.Unmarshal(document, &trip); err != nil {
		t.Fatal(err)
	}
	return trip
}

func TestPlanTripAddsCrossDayBoundaryLeg(t *testing.T) {
	service := testService(t)
	fake := &fakePlanningProvider{}
	service.SetMapService(NewMapService(service.store, journeymaps.NewRegistry(fake), 2, 0))
	tripID, revision := createMultiDayTrip(t, service, []domain.Day{
		{ID: "day-1", Date: "2026-04-18", Stops: []domain.Stop{
			multiDayStop("b", "第一天终点", 2, 30.21, 120.11),
			multiDayStop("a", "第一天起点", 1, 30.20, 120.10),
		}},
		{ID: "day-2", Date: "2026-04-19", Stops: []domain.Stop{
			multiDayStop("d", "第二天终点", 2, 30.23, 120.13),
			multiDayStop("c", "第二天起点", 1, 30.22, 120.12),
		}},
	})

	record, err := service.PlanTrip(context.Background(), tripID, revision, PlanInput{Provider: "fake", Mode: journeymaps.ModeWalking}, "test")
	if err != nil {
		t.Fatal(err)
	}
	trip := decodeTripDocument(t, record.Document)
	if got := fake.routeCalls.Load(); got != 3 {
		t.Fatalf("route calls=%d, want 3 including cross-day boundary", got)
	}
	if got := len(trip.Days[0].Legs); got != 1 {
		t.Fatalf("day 1 legs=%d, want 1", got)
	}
	if got := len(trip.Days[1].Legs); got != 2 {
		t.Fatalf("day 2 legs=%d, want 2 including boundary", got)
	}
	if got := [][]string{
		{trip.Days[0].Legs[0].FromStopID, trip.Days[0].Legs[0].ToStopID},
		{trip.Days[1].Legs[0].FromStopID, trip.Days[1].Legs[0].ToStopID},
		{trip.Days[1].Legs[1].FromStopID, trip.Days[1].Legs[1].ToStopID},
	}; !reflect.DeepEqual(got, [][]string{{"a", "b"}, {"b", "c"}, {"c", "d"}}) {
		t.Fatalf("route legs=%v", got)
	}

	record, err = service.AddStop(context.Background(), record.ID, record.Revision, "day-1", AddStopInput{ID: "e", Sequence: 3, Title: "新增点", Location: multiDayStop("e", "新增点", 3, 30.24, 120.14).Location}, "test")
	if err != nil {
		t.Fatal(err)
	}
	trip = decodeTripDocument(t, record.Document)
	if len(trip.Days[0].Legs) != 0 || len(trip.Days[1].Legs) != 0 {
		t.Fatalf("adding a stop must clear current and following cross-day legs: %+v", trip.Days)
	}
}

func TestPlanTripSingleDayUsesPreviousDayLastStop(t *testing.T) {
	service := testService(t)
	fake := &fakePlanningProvider{}
	service.SetMapService(NewMapService(service.store, journeymaps.NewRegistry(fake), 2, 0))
	tripID, revision := createMultiDayTrip(t, service, []domain.Day{
		{ID: "day-1", Date: "2026-04-18", Stops: []domain.Stop{
			multiDayStop("b", "第一天终点", 2, 30.21, 120.11),
			multiDayStop("a", "第一天起点", 1, 30.20, 120.10),
		}},
		{ID: "day-2", Date: "2026-04-19", Stops: []domain.Stop{
			multiDayStop("c", "第二天唯一点", 1, 30.22, 120.12),
		}},
	})

	record, err := service.PlanTrip(context.Background(), tripID, revision, PlanInput{Provider: "fake", Mode: journeymaps.ModeWalking, DayID: "day-2"}, "test")
	if err != nil {
		t.Fatal(err)
	}
	trip := decodeTripDocument(t, record.Document)
	if got := len(trip.Days[1].Legs); got != 1 {
		t.Fatalf("day 2 legs=%d, want 1", got)
	}
	leg := trip.Days[1].Legs[0]
	if leg.FromStopID != "b" || leg.ToStopID != "c" {
		t.Fatalf("day 2 boundary leg=%s -> %s, want b -> c", leg.FromStopID, leg.ToStopID)
	}
	if fake.routeCalls.Load() != 1 {
		t.Fatalf("route calls=%d, want 1", fake.routeCalls.Load())
	}
}
