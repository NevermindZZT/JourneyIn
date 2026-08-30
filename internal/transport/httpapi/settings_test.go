package httpapi

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"testing"
)

func TestDefaultMapProviderSettingControlsCapabilitiesAndPlanning(t *testing.T) {
	server := testPlanningServer(t)
	defer server.Close()

	settingsResponse, err := http.Get(server.URL + "/api/v1/settings")
	if err != nil {
		t.Fatal(err)
	}
	defer settingsResponse.Body.Close()
	var settings struct {
		Map struct {
			DefaultProvider string `json:"default_provider"`
		} `json:"map"`
	}
	if err := json.NewDecoder(settingsResponse.Body).Decode(&settings); err != nil {
		t.Fatal(err)
	}
	if settings.Map.DefaultProvider != "baidu" {
		t.Fatalf("initial default provider=%q, want baidu", settings.Map.DefaultProvider)
	}

	preferenceRequest, err := http.NewRequest(http.MethodPut, server.URL+"/api/v1/settings/map", strings.NewReader(`{"default_provider":"amap"}`))
	if err != nil {
		t.Fatal(err)
	}
	preferenceRequest.Header.Set("Content-Type", "application/json")
	preferenceResponse, err := http.DefaultClient.Do(preferenceRequest)
	if err != nil {
		t.Fatal(err)
	}
	defer preferenceResponse.Body.Close()
	if preferenceResponse.StatusCode != http.StatusOK {
		t.Fatalf("preference status=%d, want %d", preferenceResponse.StatusCode, http.StatusOK)
	}

	capabilitiesResponse, err := http.Get(server.URL + "/api/v1/capabilities")
	if err != nil {
		t.Fatal(err)
	}
	defer capabilitiesResponse.Body.Close()
	var capabilities struct {
		DefaultProvider string `json:"default_map_provider"`
	}
	if err := json.NewDecoder(capabilitiesResponse.Body).Decode(&capabilities); err != nil {
		t.Fatal(err)
	}
	if capabilities.DefaultProvider != "amap" {
		t.Fatalf("capabilities default provider=%q, want amap", capabilities.DefaultProvider)
	}

	trip := `{"schema_version":1,"title":"default provider planning","status":"draft","timezone":"Asia/Shanghai","date_range":{"start":"2026-04-18","end":"2026-04-18"},"map":{"enabled_providers":["amap"]},"days":[{"id":"day-1","date":"2026-04-18","stops":[{"id":"stop-a","sequence":1,"title":"A","location":{"preferred":"gcj02","coordinates":{"gcj02":{"lat":30.2,"lng":120.1,"crs":"gcj02"}}}},{"id":"stop-b","sequence":2,"title":"B","location":{"preferred":"gcj02","coordinates":{"gcj02":{"lat":30.21,"lng":120.11,"crs":"gcj02"}}}}]}]}`
	createResponse, err := http.Post(server.URL+"/api/v1/trips", "application/json", strings.NewReader(trip))
	if err != nil {
		t.Fatal(err)
	}
	defer createResponse.Body.Close()
	if createResponse.StatusCode != http.StatusCreated {
		t.Fatalf("create status=%d, want %d", createResponse.StatusCode, http.StatusCreated)
	}
	var created struct {
		ID       string `json:"id"`
		Revision int    `json:"revision"`
	}
	if err := json.NewDecoder(createResponse.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}

	planRequest, err := http.NewRequest(http.MethodPost, server.URL+"/api/v1/trips/"+created.ID+"/plan", strings.NewReader(`{"mode":"walking"}`))
	if err != nil {
		t.Fatal(err)
	}
	planRequest.Header.Set("Content-Type", "application/json")
	planRequest.Header.Set("If-Match", "revision-"+strconv.Itoa(created.Revision))
	planResponse, err := http.DefaultClient.Do(planRequest)
	if err != nil {
		t.Fatal(err)
	}
	defer planResponse.Body.Close()
	if planResponse.StatusCode != http.StatusOK {
		t.Fatalf("plan status=%d, want %d", planResponse.StatusCode, http.StatusOK)
	}
}
