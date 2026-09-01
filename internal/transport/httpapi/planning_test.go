package httpapi

import (
	"context"
	"encoding/json"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	journeyin "journeyin"
	"journeyin/internal/application"
	"journeyin/internal/domain"
	journeymaps "journeyin/internal/maps"
	journeyshare "journeyin/internal/share"
	"journeyin/internal/store"
)

type httpPlanningProvider struct{}

func (p *httpPlanningProvider) ID() journeymaps.ProviderID { return journeymaps.ProviderAMap }
func (p *httpPlanningProvider) Geocode(context.Context, string, string) ([]journeymaps.PlaceCandidate, error) {
	return nil, nil
}
func (p *httpPlanningProvider) ReverseGeocode(context.Context, journeymaps.GeoPoint) (string, error) {
	return "", nil
}
func (p *httpPlanningProvider) SearchPOI(context.Context, string, string, int, int) (journeymaps.POISearchResult, error) {
	return journeymaps.POISearchResult{Items: []journeymaps.PlaceCandidate{{ID: "uid-a", Name: "地点 A", Address: "测试地址 A", Location: journeymaps.GeoPoint{Lat: 30.2, Lng: 120.1, CRS: journeymaps.CRSBD09LL}, Provider: p.ID()}}, Total: 1, Page: 1, PageSize: 10}, nil
}
func (p *httpPlanningProvider) Route(context.Context, journeymaps.RouteRequest) (journeymaps.RouteSnapshot, error) {
	now := time.Now().UTC()
	return journeymaps.RouteSnapshot{Provider: p.ID(), CoordinateSystem: journeymaps.CRSBD09LL, Mode: journeymaps.ModeWalking, Geometry: []journeymaps.GeoPoint{{Lat: 30.2, Lng: 120.1, CRS: journeymaps.CRSBD09LL}, {Lat: 30.21, Lng: 120.11, CRS: journeymaps.CRSBD09LL}}, DistanceM: 1000, DurationS: 600, FetchedAt: now, ExpiresAt: now.Add(time.Hour)}, nil
}
func (p *httpPlanningProvider) Weather(context.Context, journeymaps.WeatherRequest) (journeymaps.WeatherSnapshot, error) {
	return journeymaps.WeatherSnapshot{}, nil
}
func (p *httpPlanningProvider) NavigationURL(journeymaps.NavTarget, journeymaps.TravelMode, journeymaps.Platform) (string, error) {
	return "https://example.test", nil
}

func testPlanningServer(t *testing.T) *httptest.Server {
	t.Helper()
	migrations, err := fs.Sub(journeyin.MigrationFS, "migrations")
	if err != nil {
		t.Fatal(err)
	}
	db, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "journeyin.db"), migrations)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	webFS, err := fs.Sub(journeyin.WebFS, "web/dist")
	if err != nil {
		t.Fatal(err)
	}
	schemaFS, err := fs.Sub(journeyin.SchemaFS, "schemas")
	if err != nil {
		t.Fatal(err)
	}
	provider := &httpPlanningProvider{}
	registry := journeymaps.NewRegistry(provider)
	mapService := application.NewMapService(db, registry, 2, 0)
	app := application.NewTripService(db)
	app.SetMapService(mapService)
	api := NewServer(app, webFS, schemaFS, "test", nil)
	api.SetMapRegistry(registry, "")
	api.SetMapService(mapService)
	api.SetShareService(journeyshare.NewService(journeyshare.NewSQLiteStore(db)), "http://example.test")
	api.SetSyncStore(db)
	api.SetSettingsStore(db)
	return httptest.NewServer(api.Handler())
}

func TestPlanningHTTPWorkflow(t *testing.T) {
	server := testPlanningServer(t)
	defer server.Close()
	trip := []byte(`{"schema_version":1,"title":"HTTP planning","status":"draft","timezone":"Asia/Shanghai","date_range":{"start":"2026-04-18","end":"2026-04-18"},"days":[{"id":"day-1","date":"2026-04-18","stops":[]}]}`)
	response, err := http.Post(server.URL+"/api/v1/trips", "application/json", strings.NewReader(string(trip)))
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("create status %d", response.StatusCode)
	}
	var created map[string]any
	if err := json.NewDecoder(response.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}
	id := created["id"].(string)
	searchBody := strings.NewReader(`{"provider":"amap","query":"地点","region":"测试"}`)
	search, err := http.Post(server.URL+"/api/v1/maps/pois/search", "application/json", searchBody)
	if err != nil {
		t.Fatal(err)
	}
	defer search.Body.Close()
	if search.StatusCode != http.StatusOK {
		t.Fatalf("search status %d", search.StatusCode)
	}
	locationA := `{"preferred":"bd09ll","coordinates":{"bd09ll":{"lat":30.2,"lng":120.1,"crs":"bd09ll"}},"source":"test"}`
	locationB := `{"preferred":"bd09ll","coordinates":{"bd09ll":{"lat":30.21,"lng":120.11,"crs":"bd09ll"}},"source":"test"}`
	for _, location := range []string{locationA, locationB} {
		body := strings.NewReader(`{"stop":{"title":"地点","location":` + location + `}}`)
		request, err := http.NewRequest(http.MethodPost, server.URL+"/api/v1/trips/"+id+"/days/day-1/stops", body)
		if err != nil {
			t.Fatal(err)
		}
		request.Header.Set("Content-Type", "application/json")
		revision := 1
		if location == locationB {
			revision = 2
		}
		request.Header.Set("If-Match", "revision-"+string(rune('0'+revision)))
		resp, err := http.DefaultClient.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("add stop status %d", resp.StatusCode)
		}
		_ = resp.Body.Close()
	}
	request, err := http.NewRequest(http.MethodPost, server.URL+"/api/v1/trips/"+id+"/plan", strings.NewReader(`{"provider":"amap","mode":"walking"}`))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("If-Match", "revision-3")
	planned, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer planned.Body.Close()
	if planned.StatusCode != http.StatusOK {
		t.Fatalf("plan status %d", planned.StatusCode)
	}
	var result struct {
		Document struct {
			Days []struct {
				Legs []domain.RouteLeg `json:"legs"`
			} `json:"days"`
		} `json:"document"`
	}
	if err := json.NewDecoder(planned.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	if len(result.Document.Days) != 1 || len(result.Document.Days[0].Legs) != 1 {
		t.Fatalf("unexpected planned document: %+v", result)
	}
}

func TestPOIPreferencesAndDirectoryClear(t *testing.T) {
	server := testPlanningServer(t)
	defer server.Close()
	search, err := http.Post(server.URL+"/api/v1/maps/pois/search", "application/json", strings.NewReader(`{"provider":"amap","query":"地点"}`))
	if err != nil {
		t.Fatal(err)
	}
	if search.StatusCode != http.StatusOK {
		t.Fatalf("search status %d", search.StatusCode)
	}
	_ = search.Body.Close()
	settingsResponse, err := http.Get(server.URL + "/api/v1/settings")
	if err != nil {
		t.Fatal(err)
	}
	defer settingsResponse.Body.Close()
	var settings struct {
		POI struct {
			ProviderPriority    string `json:"provider_priority"`
			LocalDirectoryCount int    `json:"local_directory_count"`
		} `json:"poi"`
	}
	if err := json.NewDecoder(settingsResponse.Body).Decode(&settings); err != nil {
		t.Fatal(err)
	}
	if settings.POI.LocalDirectoryCount != 1 {
		t.Fatalf("directory count=%d", settings.POI.LocalDirectoryCount)
	}
	preference, err := http.NewRequest(http.MethodPut, server.URL+"/api/v1/settings/poi", strings.NewReader(`{"provider_priority":"baidu"}`))
	if err != nil {
		t.Fatal(err)
	}
	preference.Header.Set("Content-Type", "application/json")
	preferenceResponse, err := http.DefaultClient.Do(preference)
	if err != nil {
		t.Fatal(err)
	}
	if preferenceResponse.StatusCode != http.StatusOK {
		t.Fatalf("preference status %d", preferenceResponse.StatusCode)
	}
	_ = preferenceResponse.Body.Close()
	clear, err := http.NewRequest(http.MethodDelete, server.URL+"/api/v1/settings/place-directory", nil)
	if err != nil {
		t.Fatal(err)
	}
	clearResponse, err := http.DefaultClient.Do(clear)
	if err != nil {
		t.Fatal(err)
	}
	if clearResponse.StatusCode != http.StatusOK {
		t.Fatalf("clear status %d", clearResponse.StatusCode)
	}
	_ = clearResponse.Body.Close()
}

func TestDeleteStopHTTPWorkflow(t *testing.T) {
	server := testPlanningServer(t)
	defer server.Close()
	trip := []byte(`{"schema_version":1,"title":"HTTP delete","status":"draft","timezone":"Asia/Shanghai","date_range":{"start":"2026-04-18","end":"2026-04-18"},"days":[{"id":"day-1","date":"2026-04-18","stops":[{"id":"stop-1","sequence":1,"title":"地点","location":{"preferred":"bd09ll","coordinates":{"bd09ll":{"lat":30.2,"lng":120.1,"crs":"bd09ll"}}}}]}]}`)
	response, err := http.Post(server.URL+"/api/v1/trips", "application/json", strings.NewReader(string(trip)))
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var created map[string]any
	if err := json.NewDecoder(response.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}
	id := created["id"].(string)
	request, err := http.NewRequest(http.MethodDelete, server.URL+"/api/v1/trips/"+id+"/days/day-1/stops/stop-1", nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("If-Match", "revision-1")
	deleted, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer deleted.Body.Close()
	if deleted.StatusCode != http.StatusOK {
		t.Fatalf("delete status %d", deleted.StatusCode)
	}
}

func TestMoveStopHTTPWorkflow(t *testing.T) {
	server := testPlanningServer(t)
	defer server.Close()
	trip := []byte(`{"schema_version":1,"title":"HTTP reorder","status":"draft","timezone":"Asia/Shanghai","date_range":{"start":"2026-04-18","end":"2026-04-18"},"days":[{"id":"day-1","date":"2026-04-18","stops":[{"id":"stop-a","sequence":1,"title":"A"},{"id":"stop-b","sequence":2,"title":"B"},{"id":"stop-c","sequence":3,"title":"C"}]}]}`)
	response, err := http.Post(server.URL+"/api/v1/trips", "application/json", strings.NewReader(string(trip)))
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var created struct {
		ID       string `json:"id"`
		Revision int    `json:"revision"`
	}
	if err := json.NewDecoder(response.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}
	request, err := http.NewRequest(http.MethodPost, server.URL+"/api/v1/trips/"+created.ID+"/days/day-1/stops/stop-c/move", strings.NewReader(`{"direction":"up"}`))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("If-Match", "revision-"+strconv.Itoa(created.Revision))
	moved, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer moved.Body.Close()
	if moved.StatusCode != http.StatusOK {
		t.Fatalf("move status %d", moved.StatusCode)
	}
	var payload struct {
		Revision int `json:"revision"`
		Document struct {
			Days []struct {
				Stops []struct {
					ID       string `json:"id"`
					Sequence int    `json:"sequence"`
				} `json:"stops"`
			} `json:"days"`
		} `json:"document"`
	}
	if err := json.NewDecoder(moved.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if payload.Revision != created.Revision+1 {
		t.Fatalf("revision=%d", payload.Revision)
	}
	if got := []string{payload.Document.Days[0].Stops[0].ID, payload.Document.Days[0].Stops[1].ID, payload.Document.Days[0].Stops[2].ID}; !reflect.DeepEqual(got, []string{"stop-a", "stop-c", "stop-b"}) {
		t.Fatalf("order=%v", got)
	}
	bad, err := http.NewRequest(http.MethodPost, server.URL+"/api/v1/trips/"+created.ID+"/days/day-1/stops/stop-c/move", strings.NewReader(`{"direction":"up"}`))
	if err != nil {
		t.Fatal(err)
	}
	bad.Header.Set("Content-Type", "application/json")
	bad.Header.Set("If-Match", "revision-"+strconv.Itoa(created.Revision))
	conflict, err := http.DefaultClient.Do(bad)
	if err != nil {
		t.Fatal(err)
	}
	defer conflict.Body.Close()
	if conflict.StatusCode != http.StatusConflict {
		t.Fatalf("stale move status %d", conflict.StatusCode)
	}
}

func TestMoveStopToDayHTTPWorkflow(t *testing.T) {
	server := testPlanningServer(t)
	defer server.Close()
	trip := []byte(`{"schema_version":1,"title":"HTTP move day","status":"draft","timezone":"Asia/Shanghai","date_range":{"start":"2026-04-18","end":"2026-04-19"},"days":[{"id":"day-1","date":"2026-04-18","stops":[{"id":"stop-a","sequence":1,"title":"A"},{"id":"stop-b","sequence":2,"title":"B"}]},{"id":"day-2","date":"2026-04-19","stops":[{"id":"stop-c","sequence":1,"title":"C"}]}]}`)
	response, err := http.Post(server.URL+"/api/v1/trips", "application/json", strings.NewReader(string(trip)))
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var created struct {
		ID       string `json:"id"`
		Revision int    `json:"revision"`
	}
	if err := json.NewDecoder(response.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}
	request, err := http.NewRequest(http.MethodPost, server.URL+"/api/v1/trips/"+created.ID+"/days/day-1/stops/stop-b/move", strings.NewReader(`{"target_day_id":"day-2","target_sequence":2}`))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("If-Match", "revision-"+strconv.Itoa(created.Revision))
	moved, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer moved.Body.Close()
	if moved.StatusCode != http.StatusOK {
		t.Fatalf("move day status %d", moved.StatusCode)
	}
	var payload struct {
		Revision int `json:"revision"`
		Document struct {
			Days []struct {
				Stops []struct {
					ID       string `json:"id"`
					Sequence int    `json:"sequence"`
				} `json:"stops"`
			} `json:"days"`
		} `json:"document"`
	}
	if err := json.NewDecoder(moved.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if payload.Revision != created.Revision+1 {
		t.Fatalf("revision=%d", payload.Revision)
	}
	if got := []string{payload.Document.Days[0].Stops[0].ID, payload.Document.Days[1].Stops[0].ID, payload.Document.Days[1].Stops[1].ID}; !reflect.DeepEqual(got, []string{"stop-a", "stop-c", "stop-b"}) {
		t.Fatalf("days=%v", got)
	}
	if payload.Document.Days[1].Stops[1].Sequence != 2 {
		t.Fatalf("target sequence=%d", payload.Document.Days[1].Stops[1].Sequence)
	}
}
