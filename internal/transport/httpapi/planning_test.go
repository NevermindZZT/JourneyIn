package httpapi

import (
	"context"
	"encoding/json"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"path/filepath"
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

func (p *httpPlanningProvider) ID() journeymaps.ProviderID { return journeymaps.ProviderID("fake") }
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
	searchBody := strings.NewReader(`{"provider":"fake","query":"地点","region":"测试"}`)
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
	request, err := http.NewRequest(http.MethodPost, server.URL+"/api/v1/trips/"+id+"/plan", strings.NewReader(`{"provider":"fake","mode":"walking"}`))
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
