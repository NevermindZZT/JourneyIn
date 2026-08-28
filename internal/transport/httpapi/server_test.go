package httpapi

import (
	"context"
	"encoding/json"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	journeyin "journeyin"
	"journeyin/internal/application"
	journeymaps "journeyin/internal/maps"
	journeyshare "journeyin/internal/share"
	"journeyin/internal/store"
)

func testHTTPServer(t *testing.T) *httptest.Server {
	t.Helper()
	migrations, err := fs.Sub(journeyin.MigrationFS, "migrations")
	if err != nil {
		t.Fatal(err)
	}
	database, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "journeyin.db"), migrations)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	webFS, err := fs.Sub(journeyin.WebFS, "web/dist")
	if err != nil {
		t.Fatal(err)
	}
	schemaFS, err := fs.Sub(journeyin.SchemaFS, "schemas")
	if err != nil {
		t.Fatal(err)
	}
	api := NewServer(application.NewTripService(database), webFS, schemaFS, "test", nil)
	mapRegistry := journeymaps.NewRegistry(journeymaps.NewBaiduProvider(journeymaps.BaiduConfig{}), journeymaps.NewAMapProvider("test"))
	mapService := application.NewMapService(database, mapRegistry, 2, 0)
	api.SetMapRegistry(mapRegistry, "")
	api.SetMapService(mapService)
	api.trips.SetMapService(mapService)
	api.SetShareService(journeyshare.NewService(journeyshare.NewSQLiteStore(database)), "http://example.test")
	api.SetSyncStore(database)
	api.SetSettingsStore(database)
	return httptest.NewServer(api.Handler())
}

func TestShareAndNavigationRoutes(t *testing.T) {
	server := testHTTPServer(t)
	defer server.Close()
	trip := []byte(`{
      "schema_version": 1,
      "title": "HTTP smoke",
      "status": "draft",
      "timezone": "Asia/Shanghai",
      "date_range": {"start":"2026-04-18","end":"2026-04-18"},
      "days": [{"id":"day-1","date":"2026-04-18","stops":[{"id":"stop-1","sequence":1,"title":"西湖"}]}]
    }`)
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
	tripID, _ := created["id"].(string)
	shareBody := `{"trip_id":"` + tripID + `","ttl_seconds":60}`
	response, err = http.Post(server.URL+"/api/v1/shares", "application/json", strings.NewReader(shareBody))
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("share status %d", response.StatusCode)
	}
	var shareResult map[string]any
	if err := json.NewDecoder(response.Body).Decode(&shareResult); err != nil {
		t.Fatal(err)
	}
	shareURL, _ := url.Parse(shareResult["url"].(string))
	shared, err := http.Get(server.URL + shareURL.Path)
	if err != nil {
		t.Fatal(err)
	}
	defer shared.Body.Close()
	if shared.StatusCode != http.StatusOK || shared.Header.Get("X-Robots-Tag") == "" {
		t.Fatalf("invalid share response: %d", shared.StatusCode)
	}
	sharedJSON, err := http.Get(server.URL + shareURL.Path + ".json")
	if err != nil {
		t.Fatal(err)
	}
	defer sharedJSON.Body.Close()
	if sharedJSON.StatusCode != http.StatusOK {
		t.Fatalf("share JSON status %d", sharedJSON.StatusCode)
	}
	navigation := `{"provider":"amap","target":{"name":"西湖","location":{"lat":30.25,"lng":120.15,"crs":"gcj02"}},"mode":"walking","platform":"web"}`
	navResponse, err := http.Post(server.URL+"/api/v1/maps/navigation", "application/json", strings.NewReader(navigation))
	if err != nil {
		t.Fatal(err)
	}
	defer navResponse.Body.Close()
	if navResponse.StatusCode != http.StatusOK {
		t.Fatalf("navigation status %d", navResponse.StatusCode)
	}
}

func TestMapKeysArePersistedWithoutReturningSecretValues(t *testing.T) {
	server := testHTTPServer(t)
	defer server.Close()
	body := strings.NewReader(
		`{"baidu_browser_key":"browser-test","baidu_server_key":"server-test","amap_js_key":"amap-js-test"}`,
	)
	request, err := http.NewRequest(http.MethodPut, server.URL+"/api/v1/settings/map-keys", body)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("settings update status %d", response.StatusCode)
	}
	settings, err := http.Get(server.URL + "/api/v1/settings")
	if err != nil {
		t.Fatal(err)
	}
	defer settings.Body.Close()
	if settings.StatusCode != http.StatusOK {
		t.Fatalf("settings read status %d", settings.StatusCode)
	}
	var result map[string]any
	if err := json.NewDecoder(settings.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(result)
	if strings.Contains(string(encoded), "server-test") || strings.Contains(string(encoded), "browser-test") {
		t.Fatal("map key value was returned")
	}
	mapData := result["map"].(map[string]any)
	baidu := mapData["baidu"].(map[string]any)
	if baidu["browser_key_configured"] != true || baidu["server_key_configured"] != true {
		t.Fatalf("unexpected settings: %+v", result)
	}
}
