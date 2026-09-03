package httpapi

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestUpdatePlanningPointHTTPUpdatesLocationAndReturnsInvalidation(t *testing.T) {
	server := testHTTPServer(t)
	defer server.Close()
	trip := `{"schema_version":1,"title":"HTTP 点编辑","status":"draft","timezone":"Asia/Shanghai","date_range":{"start":"2026-04-18","end":"2026-04-19"},"days":[{"id":"day-1","date":"2026-04-18","stops":[{"id":"stop-1","sequence":1,"title":"旧名","location":{"preferred":"bd09ll","coordinates":{"bd09ll":{"lat":30.1,"lng":120.1,"crs":"bd09ll"}}},"weather":{"source":"old"}},{"id":"stop-2","sequence":2,"title":"终点","location":{"preferred":"bd09ll","coordinates":{"bd09ll":{"lat":30.2,"lng":120.2,"crs":"bd09ll"}}}}],"legs":[{"id":"leg-1","from_stop_id":"stop-1","to_stop_id":"stop-2"}]},{"id":"day-2","date":"2026-04-19","stops":[],"legs":[{"id":"leg-2","from_stop_id":"x","to_stop_id":"y"}]}]}`
	createResponse, err := http.Post(server.URL+"/api/v1/trips", "application/json", strings.NewReader(trip))
	if err != nil {
		t.Fatal(err)
	}
	defer createResponse.Body.Close()
	if createResponse.StatusCode != http.StatusCreated {
		t.Fatalf("create status %d", createResponse.StatusCode)
	}
	var created struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(createResponse.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}

	requestBody := `{"title":"新名称","address":"新地址","location":{"preferred":"gcj02","coordinates":{"gcj02":{"lat":30.25,"lng":120.25,"crs":"gcj02"}},"source":"amap-place-search"}}`
	request, err := http.NewRequest(http.MethodPatch, server.URL+"/api/v1/trips/"+created.ID+"/days/day-1/stops/stop-1", strings.NewReader(requestBody))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("If-Match", "revision-1")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("update status %d", response.StatusCode)
	}
	var payload struct {
		Revision int `json:"revision"`
		Changes  struct {
			LocationChanged  bool `json:"location_changed"`
			RouteInvalidated bool `json:"route_invalidated"`
			WeatherCleared   bool `json:"weather_cleared"`
		} `json:"changes"`
		Document map[string]any `json:"document"`
	}
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if payload.Revision != 2 || !payload.Changes.LocationChanged || !payload.Changes.RouteInvalidated || !payload.Changes.WeatherCleared {
		t.Fatalf("unexpected response: %+v", payload)
	}
	days := payload.Document["days"].([]any)
	day1 := days[0].(map[string]any)
	if _, ok := day1["legs"]; ok {
		t.Fatal("HTTP update did not clear current route")
	}
	stop := day1["stops"].([]any)[0].(map[string]any)
	if stop["title"] != "新名称" || stop["address"] != "新地址" {
		t.Fatalf("stop was not updated: %+v", stop)
	}
	if _, ok := stop["weather"]; ok {
		t.Fatal("HTTP update did not clear old weather")
	}
}

func TestUpdatePlanningPointHTTPRequiresRevisionAndRejectsUnknownFields(t *testing.T) {
	server := testHTTPServer(t)
	defer server.Close()
	request, err := http.NewRequest(http.MethodPatch, server.URL+"/api/v1/trips/missing/days/day-1/stops/stop-1", strings.NewReader(`{"title":"新名"}`))
	if err != nil {
		t.Fatal(err)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusPreconditionRequired {
		t.Fatalf("missing revision status %d", response.StatusCode)
	}

	request, err = http.NewRequest(http.MethodPatch, server.URL+"/api/v1/trips/missing/days/day-1/stops/stop-1", strings.NewReader(`{"unsupported":true}`))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("If-Match", "revision-1")
	response, err = http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("unknown field status %d", response.StatusCode)
	}
}
