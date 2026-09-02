package httpapi

import (
	"context"
	"encoding/json"
	"io"
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
	sharedContent, err := io.ReadAll(shared.Body)
	if err != nil {
		t.Fatal(err)
	}
	csp := shared.Header.Get("Content-Security-Policy")
	if shared.StatusCode != http.StatusOK || shared.Header.Get("X-Robots-Tag") == "" || shared.Header.Get("Referrer-Policy") != "origin" || !strings.Contains(csp, "script-src") || !strings.Contains(csp, "https://*.bdimg.com") || !strings.Contains(csp, "https://*.bdstatic.com") || !strings.Contains(csp, "worker-src") || !strings.Contains(csp, "frame-src") || !strings.Contains(csp, "base-uri 'self'") || strings.Contains(csp, "default-src 'none'") || !strings.Contains(string(sharedContent), "__JOURNEYIN_SHARE__") || strings.Contains(string(sharedContent), "<pre>") {
		t.Fatalf("invalid rendered share response: %d", shared.StatusCode)
	}
	sharedJSON, err := http.Get(server.URL + shareURL.Path + ".json")
	if err != nil {
		t.Fatal(err)
	}
	defer sharedJSON.Body.Close()
	if sharedJSON.StatusCode != http.StatusOK {
		t.Fatalf("share JSON status %d", sharedJSON.StatusCode)
	}
	revokeRequest, err := http.NewRequest(http.MethodPost, server.URL+"/api/v1/shares/"+shareResult["id"].(string)+"/revoke", nil)
	if err != nil {
		t.Fatal(err)
	}
	revoked, err := http.DefaultClient.Do(revokeRequest)
	if err != nil {
		t.Fatal(err)
	}
	defer revoked.Body.Close()
	if revoked.StatusCode != http.StatusNoContent {
		t.Fatalf("revoke status %d", revoked.StatusCode)
	}
	revokedShared, err := http.Get(server.URL + shareURL.Path)
	if err != nil {
		t.Fatal(err)
	}
	defer revokedShared.Body.Close()
	if revokedShared.StatusCode != http.StatusNotFound {
		t.Fatalf("revoked share status %d", revokedShared.StatusCode)
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

func TestValidateAndExportHTTPWorkflow(t *testing.T) {
	server := testHTTPServer(t)
	defer server.Close()
	trip := []byte(`{
      "schema_version": 1,
      "title": "Import export smoke",
      "status": "draft",
      "timezone": "Asia/Shanghai",
      "date_range": {"start":"2026-04-18","end":"2026-04-18"},
      "days": [{"id":"day-1","date":"2026-04-18","stops":[]}]
    }`)
	response, err := http.Post(server.URL+"/api/v1/validate", "application/json", strings.NewReader(string(trip)))
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("validate status %d", response.StatusCode)
	}
	response, err = http.Post(server.URL+"/api/v1/import", "application/json", strings.NewReader(string(trip)))
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("import status %d", response.StatusCode)
	}
	var created map[string]any
	if err := json.NewDecoder(response.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}
	tripID := created["id"].(string)
	exported, err := http.Get(server.URL + "/api/v1/trips/" + tripID + "/export.json")
	if err != nil {
		t.Fatal(err)
	}
	defer exported.Body.Close()
	if exported.StatusCode != http.StatusOK || !strings.Contains(exported.Header.Get("Content-Disposition"), "attachment") {
		t.Fatalf("export response: %d %s", exported.StatusCode, exported.Header.Get("Content-Disposition"))
	}
	var document map[string]any
	if err := json.NewDecoder(exported.Body).Decode(&document); err != nil {
		t.Fatal(err)
	}
	if document["title"] != "Import export smoke" {
		t.Fatalf("export title=%v", document["title"])
	}
	encodedDocument, err := json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	roundTrip, err := http.Post(server.URL+"/api/v1/import", "application/json", strings.NewReader(string(encodedDocument)))
	if err != nil {
		t.Fatal(err)
	}
	defer roundTrip.Body.Close()
	if roundTrip.StatusCode != http.StatusCreated {
		t.Fatalf("round-trip import status %d", roundTrip.StatusCode)
	}
}
func TestShareLinkReusesAndKeepsSnapshotAfterTripUpdates(t *testing.T) {
	server := testHTTPServer(t)
	defer server.Close()
	trip := []byte(`{"schema_version":1,"title":"Share source","status":"draft","timezone":"Asia/Shanghai","date_range":{"start":"2026-04-18","end":"2026-04-18"},"days":[{"id":"day-1","date":"2026-04-18","stops":[]}]}`)
	response, err := http.Post(server.URL+"/api/v1/trips", "application/json", strings.NewReader(string(trip)))
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var created map[string]any
	if err := json.NewDecoder(response.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}
	tripID := created["id"].(string)
	shareResponse, err := http.Post(server.URL+"/api/v1/shares", "application/json", strings.NewReader(`{"trip_id":"`+tripID+`"}`))
	if err != nil {
		t.Fatal(err)
	}
	defer shareResponse.Body.Close()
	var share map[string]any
	if err := json.NewDecoder(shareResponse.Body).Decode(&share); err != nil {
		t.Fatal(err)
	}
	shareURL, _ := url.Parse(share["url"].(string))
	token := strings.TrimPrefix(shareURL.Path, "/s/")
	reusedResponse, err := http.Post(server.URL+"/api/v1/shares", "application/json", strings.NewReader(`{"trip_id":"`+tripID+`","existing_token":"`+token+`"}`))
	if err != nil {
		t.Fatal(err)
	}
	defer reusedResponse.Body.Close()
	if reusedResponse.StatusCode != http.StatusOK {
		t.Fatalf("reuse status %d", reusedResponse.StatusCode)
	}
	var reused map[string]any
	if err := json.NewDecoder(reusedResponse.Body).Decode(&reused); err != nil {
		t.Fatal(err)
	}
	if reused["url"] != share["url"] || reused["reused"] != true {
		t.Fatalf("share was not reused: %+v", reused)
	}
	updated := []byte(`{"schema_version":1,"title":"Share updated","status":"draft","timezone":"Asia/Shanghai","date_range":{"start":"2026-04-18","end":"2026-04-18"},"days":[{"id":"day-1","date":"2026-04-18","stops":[]}]}`)
	updateRequest, err := http.NewRequest(http.MethodPut, server.URL+"/api/v1/trips/"+tripID, strings.NewReader(string(updated)))
	if err != nil {
		t.Fatal(err)
	}
	updateRequest.Header.Set("Content-Type", "application/json")
	updateRequest.Header.Set("If-Match", "revision-1")
	updatedResponse, err := http.DefaultClient.Do(updateRequest)
	if err != nil {
		t.Fatal(err)
	}
	defer updatedResponse.Body.Close()
	if updatedResponse.StatusCode != http.StatusOK {
		t.Fatalf("update status %d", updatedResponse.StatusCode)
	}
	shared, err := http.Get(server.URL + shareURL.Path)
	if err != nil {
		t.Fatal(err)
	}
	defer shared.Body.Close()
	sharedBody, err := io.ReadAll(shared.Body)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(sharedBody), "Share source") || !strings.Contains(string(sharedBody), "\"revision\":1") || strings.Contains(string(sharedBody), "Share updated") {
		t.Fatalf("share snapshot changed after update: %s", string(sharedBody))
	}
}
func TestMapKeysArePersistedWithoutReturningSecretValues(t *testing.T) {
	server := testHTTPServer(t)
	defer server.Close()
	body := strings.NewReader(
		`{"baidu_browser_key":"browser-test","baidu_server_key":"server-test","amap_js_key":"amap-js-test","amap_server_key":"amap-server-test","amap_security_js_code":"amap-security-test"}`,
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
	if strings.Contains(string(encoded), "server-test") || strings.Contains(string(encoded), "browser-test") || strings.Contains(string(encoded), "amap-server-test") || strings.Contains(string(encoded), "amap-security-test") {
		t.Fatal("map key value was returned")
	}
	mapData := result["map"].(map[string]any)
	baidu := mapData["baidu"].(map[string]any)
	if baidu["browser_key_configured"] != true || baidu["server_key_configured"] != true {
		t.Fatalf("unexpected Baidu settings: %+v", result)
	}
	amap := mapData["amap"].(map[string]any)
	if amap["js_key_configured"] != true || amap["server_key_configured"] != true || amap["security_js_code_configured"] != true {
		t.Fatalf("unexpected AMap settings: %+v", result)
	}
}
