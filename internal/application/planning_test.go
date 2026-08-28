package application

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"sync/atomic"
	"testing"
	"time"

	"journeyin/internal/domain"
	journeymaps "journeyin/internal/maps"
	"journeyin/internal/store"
)

type fakePlanningProvider struct {
	searchCalls atomic.Int32
	routeCalls  atomic.Int32
	emptySearch bool
}

func (p *fakePlanningProvider) ID() journeymaps.ProviderID { return journeymaps.ProviderID("fake") }
func (p *fakePlanningProvider) Geocode(_ context.Context, address, _ string) ([]journeymaps.PlaceCandidate, error) {
	return []journeymaps.PlaceCandidate{{Name: address, Address: address, Location: journeymaps.GeoPoint{Lat: 30.2, Lng: 120.1, CRS: journeymaps.CRSBD09LL}, Provider: p.ID()}}, nil
}
func (p *fakePlanningProvider) ReverseGeocode(context.Context, journeymaps.GeoPoint) (string, error) {
	return "", nil
}
func (p *fakePlanningProvider) SearchPOI(context.Context, string, string, int, int) (journeymaps.POISearchResult, error) {
	p.searchCalls.Add(1)
	if p.emptySearch {
		return journeymaps.POISearchResult{Page: 1, PageSize: 10}, nil
	}
	return journeymaps.POISearchResult{Items: []journeymaps.PlaceCandidate{{ID: "poi-1", Name: "测试地点", Location: journeymaps.GeoPoint{Lat: 30.2, Lng: 120.1, CRS: journeymaps.CRSBD09LL}, Provider: p.ID()}}, Total: 1, Page: 1, PageSize: 10}, nil
}
func (p *fakePlanningProvider) Route(context.Context, journeymaps.RouteRequest) (journeymaps.RouteSnapshot, error) {
	p.routeCalls.Add(1)
	now := time.Now().UTC()
	return journeymaps.RouteSnapshot{Provider: p.ID(), CoordinateSystem: journeymaps.CRSBD09LL, Mode: journeymaps.ModeWalking, Geometry: []journeymaps.GeoPoint{{Lat: 30.2, Lng: 120.1, CRS: journeymaps.CRSBD09LL}, {Lat: 30.21, Lng: 120.11, CRS: journeymaps.CRSBD09LL}}, DistanceM: 1000, DurationS: 600, FetchedAt: now, ExpiresAt: now.Add(time.Hour)}, nil
}
func (p *fakePlanningProvider) Weather(context.Context, journeymaps.WeatherRequest) (journeymaps.WeatherSnapshot, error) {
	temperature := 18.5
	now := time.Now().UTC()
	return journeymaps.WeatherSnapshot{Provider: p.ID(), LocalDate: "2026-04-18", Condition: "晴", TemperatureC: &temperature, FetchedAt: now, ExpiresAt: now.Add(6 * time.Hour), Available: true}, nil
}
func (p *fakePlanningProvider) NavigationURL(journeymaps.NavTarget, journeymaps.TravelMode, journeymaps.Platform) (string, error) {
	return "https://example.test", nil
}

func TestAddStopPersistsLocationAndPlanUsesCachedSegments(t *testing.T) {
	service := testService(t)
	fake := &fakePlanningProvider{}
	service.SetMapService(NewMapService(service.store, journeymaps.NewRegistry(fake), 2, 0))
	tripJSON := []byte(`{"schema_version":1,"title":"规划测试","status":"draft","timezone":"Asia/Shanghai","date_range":{"start":"2026-04-18","end":"2026-04-18"},"days":[{"id":"day-1","date":"2026-04-18","stops":[]}]}`)
	record, err := service.Create(context.Background(), tripJSON, "test")
	if err != nil {
		t.Fatal(err)
	}
	location1 := json.RawMessage(`{"preferred":"bd09ll","coordinates":{"bd09ll":{"lat":30.200000,"lng":120.100000,"crs":"bd09ll"}},"source":"test"}`)
	record, err = service.AddStop(context.Background(), record.ID, record.Revision, "day-1", AddStopInput{Title: "地点 A", Location: location1}, "test")
	if err != nil {
		t.Fatal(err)
	}
	location2 := json.RawMessage(`{"preferred":"bd09ll","coordinates":{"bd09ll":{"lat":30.210000,"lng":120.110000,"crs":"bd09ll"}},"source":"test"}`)
	record, err = service.AddStop(context.Background(), record.ID, record.Revision, "day-1", AddStopInput{Title: "地点 B", Location: location2}, "test")
	if err != nil {
		t.Fatal(err)
	}
	record, err = service.PlanTrip(context.Background(), record.ID, record.Revision, PlanInput{Provider: "fake", Mode: journeymaps.ModeWalking}, "test")
	if err != nil {
		t.Fatal(err)
	}
	if fake.routeCalls.Load() != 1 {
		t.Fatalf("route calls after first plan = %d", fake.routeCalls.Load())
	}
	var trip domain.Trip
	if err := json.Unmarshal(record.Document, &trip); err != nil {
		t.Fatal(err)
	}
	if len(trip.Days[0].Stops) != 2 || len(trip.Days[0].Legs) != 1 || len(trip.Days[0].Legs[0].Snapshots) != 1 {
		t.Fatalf("unexpected planned trip: %+v", trip)
	}
	record, err = service.PlanTrip(context.Background(), record.ID, record.Revision, PlanInput{Provider: "fake", Mode: journeymaps.ModeWalking}, "test")
	if err != nil {
		t.Fatal(err)
	}
	if fake.routeCalls.Load() != 1 {
		t.Fatalf("cached route was called again: %d", fake.routeCalls.Load())
	}
}

func TestMapServiceCachesPOISearch(t *testing.T) {
	service := testService(t)
	fake := &fakePlanningProvider{}
	mapService := NewMapService(service.store, journeymaps.NewRegistry(fake), 2, 0)
	for i := 0; i < 2; i++ {
		result, err := mapService.SearchPOI(context.Background(), "fake", "景点", "杭州市", 1, 10)
		if err != nil {
			t.Fatal(err)
		}
		if len(result.Items) != 1 {
			t.Fatalf("unexpected result: %+v", result)
		}
	}
	if fake.searchCalls.Load() != 1 {
		t.Fatalf("search calls = %d", fake.searchCalls.Load())
	}
}

func TestMapServiceHonorsDailyQuotaAfterCacheMiss(t *testing.T) {
	service := testService(t)
	fake := &fakePlanningProvider{}
	mapService := NewMapService(service.store, journeymaps.NewRegistry(fake), 2, 1)
	if _, err := mapService.SearchPOI(context.Background(), "fake", "第一个", "杭州市", 1, 10); err != nil {
		t.Fatal(err)
	}
	if _, err := mapService.SearchPOI(context.Background(), "fake", "第二个", "杭州市", 1, 10); err == nil {
		t.Fatal("expected daily quota error")
	}
	if fake.searchCalls.Load() != 1 {
		t.Fatalf("search calls after quota = %d", fake.searchCalls.Load())
	}
}

func TestAddSubStopAndRefreshWeatherPersistSnapshots(t *testing.T) {
	service := testService(t)
	fake := &fakePlanningProvider{}
	service.SetMapService(NewMapService(service.store, journeymaps.NewRegistry(fake), 2, 0))
	tripJSON := []byte(`{"schema_version":1,"title":"子点天气测试","status":"draft","timezone":"Asia/Shanghai","date_range":{"start":"2026-04-18","end":"2026-04-18"},"days":[{"id":"day-1","date":"2026-04-18","stops":[]}]}`)
	record, err := service.Create(context.Background(), tripJSON, "test")
	if err != nil {
		t.Fatal(err)
	}
	location := json.RawMessage(`{"preferred":"bd09ll","coordinates":{"bd09ll":{"lat":30.2,"lng":120.1,"crs":"bd09ll"}}}`)
	record, err = service.AddStop(context.Background(), record.ID, record.Revision, "day-1", AddStopInput{Title: "主规划点", Location: location}, "test")
	if err != nil {
		t.Fatal(err)
	}
	var trip domain.Trip
	if err := json.Unmarshal(record.Document, &trip); err != nil {
		t.Fatal(err)
	}
	parentID := trip.Days[0].Stops[0].ID
	record, err = service.AddSubStop(context.Background(), record.ID, record.Revision, "day-1", parentID, AddStopInput{Title: "子规划点", Location: location}, "test")
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(record.Document, &trip); err != nil {
		t.Fatal(err)
	}
	if len(trip.Days[0].Stops[0].Children) != 1 {
		t.Fatalf("children not persisted: %+v", trip.Days[0].Stops[0])
	}
	record, err = service.RefreshWeather(context.Background(), record.ID, record.Revision, "day-1", parentID, WeatherInput{Provider: "fake"}, "test")
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(record.Document, &trip); err != nil {
		t.Fatal(err)
	}
	if len(trip.Days[0].Stops[0].Weather) == 0 {
		t.Fatal("weather snapshot not persisted")
	}
}

func TestMapServiceFallsBackToGeocodeForMissingPOI(t *testing.T) {
	service := testService(t)
	fake := &fakePlanningProvider{emptySearch: true}
	mapService := NewMapService(service.store, journeymaps.NewRegistry(fake), 2, 0)
	result, err := mapService.SearchPOI(context.Background(), "fake", "甘加", "青海省", 1, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Items) != 1 || result.Items[0].Name != "甘加" {
		t.Fatalf("expected geocode fallback: %+v", result)
	}
}

func TestDeleteStopAndChildUsesRevision(t *testing.T) {
	service := testService(t)
	tripJSON := []byte(`{"schema_version":1,"title":"删除测试","status":"draft","timezone":"Asia/Shanghai","date_range":{"start":"2026-04-18","end":"2026-04-18"},"days":[{"id":"day-1","date":"2026-04-18","stops":[]}] }`)
	record, err := service.Create(context.Background(), tripJSON, "test")
	if err != nil {
		t.Fatal(err)
	}
	location := json.RawMessage(`{"preferred":"bd09ll","coordinates":{"bd09ll":{"lat":30.2,"lng":120.1,"crs":"bd09ll"}}}`)
	record, err = service.AddStop(context.Background(), record.ID, record.Revision, "day-1", AddStopInput{Title: "主点", Location: location}, "test")
	if err != nil {
		t.Fatal(err)
	}
	var trip domain.Trip
	if err := json.Unmarshal(record.Document, &trip); err != nil {
		t.Fatal(err)
	}
	parentID := trip.Days[0].Stops[0].ID
	record, err = service.AddSubStop(context.Background(), record.ID, record.Revision, "day-1", parentID, AddStopInput{Title: "子点", Location: location}, "test")
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(record.Document, &trip); err != nil {
		t.Fatal(err)
	}
	childID := trip.Days[0].Stops[0].Children[0].ID
	record, err = service.DeleteStop(context.Background(), record.ID, record.Revision, "day-1", childID, "test")
	if err != nil {
		t.Fatal(err)
	}
	trip = domain.Trip{}
	if err := json.Unmarshal(record.Document, &trip); err != nil {
		t.Fatal(err)
	}
	if len(trip.Days[0].Stops[0].Children) != 0 {
		t.Fatal("child was not deleted")
	}
	if _, err = service.DeleteStop(context.Background(), record.ID, record.Revision-1, "day-1", parentID, "test"); !errors.Is(err, store.ErrRevisionConflict) {
		t.Fatalf("expected revision conflict, got %v", err)
	}
	record, err = service.DeleteStop(context.Background(), record.ID, record.Revision, "day-1", parentID, "test")
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(record.Document, &trip); err != nil {
		t.Fatal(err)
	}
	if len(trip.Days[0].Stops) != 0 {
		t.Fatal("parent was not deleted")
	}
}

func TestMoveStopAndChildUsesRevision(t *testing.T) {
	service := testService(t)
	tripJSON := []byte(`{"schema_version":1,"title":"顺序测试","status":"draft","timezone":"Asia/Shanghai","date_range":{"start":"2026-04-18","end":"2026-04-18"},"days":[{"id":"day-1","date":"2026-04-18","stops":[{"id":"stop-a","sequence":1,"title":"A","location":{"preferred":"bd09ll","coordinates":{"bd09ll":{"lat":30.2,"lng":120.1,"crs":"bd09ll"}}}},{"id":"stop-b","sequence":2,"title":"B","location":{"preferred":"bd09ll","coordinates":{"bd09ll":{"lat":30.21,"lng":120.11,"crs":"bd09ll"}}}},{"id":"stop-c","sequence":3,"title":"C","location":{"preferred":"bd09ll","coordinates":{"bd09ll":{"lat":30.22,"lng":120.12,"crs":"bd09ll"}}}}],"legs":[{"id":"leg-1","from_stop_id":"stop-a","to_stop_id":"stop-b"}]}]}`)
	record, err := service.Create(context.Background(), tripJSON, "test")
	if err != nil {
		t.Fatal(err)
	}
	record, err = service.MoveStop(context.Background(), record.ID, record.Revision, "day-1", "stop-c", "up", "test")
	if err != nil {
		t.Fatal(err)
	}
	var trip domain.Trip
	if err := json.Unmarshal(record.Document, &trip); err != nil {
		t.Fatal(err)
	}
	if got := []string{trip.Days[0].Stops[0].ID, trip.Days[0].Stops[1].ID, trip.Days[0].Stops[2].ID}; !reflect.DeepEqual(got, []string{"stop-a", "stop-c", "stop-b"}) {
		t.Fatalf("main order=%v", got)
	}
	if trip.Days[0].Stops[0].Sequence != 1 || trip.Days[0].Stops[1].Sequence != 2 || trip.Days[0].Stops[2].Sequence != 3 {
		t.Fatalf("main sequences not normalized: %+v", trip.Days[0].Stops)
	}
	if len(trip.Days[0].Legs) != 0 {
		t.Fatal("main reorder must clear route legs")
	}
	if _, err := service.MoveStop(context.Background(), record.ID, record.Revision-1, "day-1", "stop-c", "up", "test"); !errors.Is(err, store.ErrRevisionConflict) {
		t.Fatalf("expected revision conflict, got %v", err)
	}
	record, err = service.AddSubStop(context.Background(), record.ID, record.Revision, "day-1", "stop-a", AddStopInput{Title: "child-a", Location: json.RawMessage(`{"preferred":"bd09ll","coordinates":{"bd09ll":{"lat":30.2,"lng":120.1,"crs":"bd09ll"}}}`)}, "test")
	if err != nil {
		t.Fatal(err)
	}
	record, err = service.AddSubStop(context.Background(), record.ID, record.Revision, "day-1", "stop-a", AddStopInput{Title: "child-b", Location: json.RawMessage(`{"preferred":"bd09ll","coordinates":{"bd09ll":{"lat":30.2,"lng":120.1,"crs":"bd09ll"}}}`)}, "test")
	if err != nil {
		t.Fatal(err)
	}
	trip = domain.Trip{}
	if err := json.Unmarshal(record.Document, &trip); err != nil {
		t.Fatal(err)
	}
	children := trip.Days[0].Stops[0].Children
	childB := children[1].ID
	record, err = service.MoveStop(context.Background(), record.ID, record.Revision, "day-1", childB, "up", "test")
	if err != nil {
		t.Fatal(err)
	}
	trip = domain.Trip{}
	if err := json.Unmarshal(record.Document, &trip); err != nil {
		t.Fatal(err)
	}
	if got := []string{trip.Days[0].Stops[0].Children[0].Title, trip.Days[0].Stops[0].Children[1].Title}; !reflect.DeepEqual(got, []string{"child-b", "child-a"}) {
		t.Fatalf("child order=%v", got)
	}
	if trip.Days[0].Stops[0].Children[0].Sequence != 1 || trip.Days[0].Stops[0].Children[1].Sequence != 2 {
		t.Fatalf("child sequences not normalized: %+v", trip.Days[0].Stops[0].Children)
	}
}

func TestReorderStopToTargetSequence(t *testing.T) {
	service := testService(t)
	tripJSON := []byte(`{"schema_version":1,"title":"精确排序测试","status":"draft","timezone":"Asia/Shanghai","date_range":{"start":"2026-04-18","end":"2026-04-18"},"days":[{"id":"day-1","date":"2026-04-18","stops":[{"id":"a","sequence":1,"title":"A"},{"id":"b","sequence":2,"title":"B"},{"id":"c","sequence":3,"title":"C"}]}]}`)
	record, err := service.Create(context.Background(), tripJSON, "test")
	if err != nil {
		t.Fatal(err)
	}
	record, err = service.ReorderStop(context.Background(), record.ID, record.Revision, "day-1", "c", 1, "test")
	if err != nil {
		t.Fatal(err)
	}
	var trip domain.Trip
	if err := json.Unmarshal(record.Document, &trip); err != nil {
		t.Fatal(err)
	}
	if got := []string{trip.Days[0].Stops[0].ID, trip.Days[0].Stops[1].ID, trip.Days[0].Stops[2].ID}; !reflect.DeepEqual(got, []string{"c", "a", "b"}) {
		t.Fatalf("order=%v", got)
	}
	if _, err := service.ReorderStop(context.Background(), record.ID, record.Revision-1, "day-1", "a", 3, "test"); !errors.Is(err, store.ErrRevisionConflict) {
		t.Fatalf("expected revision conflict, got %v", err)
	}
}
