package application

import (
	"context"
	"encoding/json"
	"testing"

	"journeyin/internal/domain"
)

func TestUpdatePlanningPointKindForMainAndChildWithoutRouteInvalidation(t *testing.T) {
	service := testService(t)
	record := createTripForDetails(t, service, "规划点标签测试", []domain.Day{{
		ID: "day-1", Date: "2026-04-18",
		Stops: []domain.Stop{
			{ID: "stop-1", Sequence: 1, Title: "主点", Children: []domain.SubStop{{ID: "child-1", Sequence: 1, Title: "子点"}}},
			{ID: "stop-2", Sequence: 2, Title: "终点"},
		},
		Legs: []domain.RouteLeg{{ID: "leg-1", FromStopID: "stop-1", ToStopID: "stop-2"}},
	}})

	lodging := "lodging"
	record, changes, err := service.UpdatePlanningPoint(context.Background(), record.ID, record.Revision, "day-1", "stop-1", UpdatePlanningPointInput{Kind: &lodging}, "test:kind")
	if err != nil {
		t.Fatal(err)
	}
	if !changes.Changed || !changes.KindChanged || changes.RouteInvalidated {
		t.Fatalf("unexpected main category changes: %+v", changes)
	}
	var trip domain.Trip
	if err := json.Unmarshal(record.Document, &trip); err != nil {
		t.Fatal(err)
	}
	if trip.Days[0].Stops[0].Kind != lodging || len(trip.Days[0].Legs) != 1 {
		t.Fatalf("main kind/routes = %q/%d", trip.Days[0].Stops[0].Kind, len(trip.Days[0].Legs))
	}

	restaurant := "restaurant"
	record, changes, err = service.UpdatePlanningPoint(context.Background(), record.ID, record.Revision, "day-1", "child-1", UpdatePlanningPointInput{Kind: &restaurant}, "test:child-kind")
	if err != nil {
		t.Fatal(err)
	}
	if !changes.KindChanged || changes.RouteInvalidated {
		t.Fatalf("unexpected child category changes: %+v", changes)
	}
	if err := json.Unmarshal(record.Document, &trip); err != nil {
		t.Fatal(err)
	}
	if got := trip.Days[0].Stops[0].Children[0].Kind; got != restaurant {
		t.Fatalf("child kind=%q, want %q", got, restaurant)
	}
	if len(trip.Days[0].Legs) != 1 {
		t.Fatalf("category update cleared routes: %+v", trip.Days[0].Legs)
	}
}
