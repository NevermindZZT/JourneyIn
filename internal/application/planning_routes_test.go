package application

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"journeyin/internal/domain"
	journeymaps "journeyin/internal/maps"
)

func TestMergeRouteLegsPreservesOtherProviders(t *testing.T) {
	fetched := time.Date(2026, 4, 18, 1, 0, 0, 0, time.UTC)
	existing := []domain.RouteLeg{{ID: "leg-stable", FromStopID: "stop-a", ToStopID: "stop-b", Mode: "walking", Snapshots: []domain.RouteSnapshot{{Provider: "baidu", CoordinateSystem: "bd09ll", Mode: "walking", Geometry: [][]float64{{120, 30}, {120.1, 30.1}}, FetchedAt: fetched.Format(time.RFC3339), ExpiresAt: fetched.Add(time.Hour).Format(time.RFC3339)}}}}
	planned := []plannedRoute{{fromID: "stop-a", toID: "stop-b", snapshot: journeymaps.RouteSnapshot{Provider: journeymaps.ProviderAMap, CoordinateSystem: journeymaps.CRSGCJ02, Mode: journeymaps.ModeWalking, Source: "fixture", Geometry: []journeymaps.GeoPoint{{Lng: 120, Lat: 30, CRS: journeymaps.CRSGCJ02}, {Lng: 120.1, Lat: 30.1, CRS: journeymaps.CRSGCJ02}}, FetchedAt: fetched, ExpiresAt: fetched.Add(time.Hour)}}}
	merged, err := mergeRouteLegs(existing, planned, journeymaps.ProviderAMap, journeymaps.ModeWalking)
	if err != nil {
		t.Fatal(err)
	}
	if len(merged) != 1 || merged[0].ID != "leg-stable" || len(merged[0].Snapshots) != 2 {
		t.Fatalf("merged=%+v", merged)
	}
	if merged[0].Snapshots[0].Provider != "baidu" || merged[0].Snapshots[1].Provider != "amap" || merged[0].Snapshots[1].Mode != "walking" {
		t.Fatalf("snapshots=%+v", merged[0].Snapshots)
	}
}

func TestParseSavedLocationRetainsGaodeTransitMetadata(t *testing.T) {
	location, err := parseSavedLocation([]byte(`{"preferred":"bd09ll","coordinates":{"bd09ll":{"lat":30,"lng":120,"crs":"bd09ll"}},"provider_refs":{"amap_uid":"A123"},"citycode":"0571","adcode":"330100"}`))
	if err != nil {
		t.Fatal(err)
	}
	if location.CityCode != "0571" || location.AdCode != "330100" || providerPOIID(location.ProviderRefs, journeymaps.ProviderAMap) != "A123" {
		t.Fatalf("location=%+v", location)
	}
}

func TestParseSavedLocationNormalizesLegacyBaiduCoordinates(t *testing.T) {
	location, err := parseSavedLocation([]byte(`{"preferred":"baidu","coordinates":{"baidu":{"lat":34.15,"lng":103.18}}}`))
	if err != nil {
		t.Fatal(err)
	}
	if location.Point.CRS != journeymaps.CRSBD09LL || savedPointForProvider(location, journeymaps.ProviderAMap).CRS != journeymaps.CRSBD09LL {
		t.Fatalf("location=%+v", location)
	}
}

func TestPlanTripSkipsIdenticalCoordinates(t *testing.T) {
	service := testService(t)
	fake := &fakePlanningProvider{}
	service.SetMapService(NewMapService(service.store, journeymaps.NewRegistry(fake), 2, 0))
	record, err := service.Create(context.Background(), []byte(`{"schema_version":1,"title":"same place","status":"draft","timezone":"Asia/Shanghai","date_range":{"start":"2026-04-18","end":"2026-04-18"},"days":[{"id":"day-1","date":"2026-04-18","stops":[]}] }`), "test")
	if err != nil {
		t.Fatal(err)
	}
	location := json.RawMessage(`{"preferred":"baidu","coordinates":{"baidu":{"lat":34.15,"lng":103.18}}}`)
	record, err = service.AddStop(context.Background(), record.ID, record.Revision, "day-1", AddStopInput{Title: "同一地点 A", Location: location}, "test")
	if err != nil {
		t.Fatal(err)
	}
	record, err = service.AddStop(context.Background(), record.ID, record.Revision, "day-1", AddStopInput{Title: "同一地点 B", Location: location}, "test")
	if err != nil {
		t.Fatal(err)
	}
	record, err = service.PlanTrip(context.Background(), record.ID, record.Revision, PlanInput{Provider: "fake", Mode: journeymaps.ModeDriving}, "test")
	if err != nil {
		t.Fatal(err)
	}
	if fake.routeCalls.Load() != 0 {
		t.Fatalf("same-place route calls=%d", fake.routeCalls.Load())
	}
	var trip domain.Trip
	if err := json.Unmarshal(record.Document, &trip); err != nil {
		t.Fatal(err)
	}
	if len(trip.Days[0].Legs) != 1 || len(trip.Days[0].Legs[0].Snapshots) != 1 || trip.Days[0].Legs[0].Snapshots[0].Source != "journeyin-same-location" {
		t.Fatalf("trip=%+v", trip)
	}
}
