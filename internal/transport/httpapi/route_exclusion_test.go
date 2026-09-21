package httpapi

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestUpdatePlanningPointHTTPExcludesRoutePoint(t *testing.T) {
	server := testHTTPServer(t)
	defer server.Close()
	trip := `{"schema_version":1,"title":"HTTP 路线排除","status":"draft","timezone":"Asia/Shanghai","date_range":{"start":"2026-04-18","end":"2026-04-18"},"days":[{"id":"day-1","date":"2026-04-18","stops":[{"id":"stop-1","sequence":1,"title":"备选点","location":{"preferred":"bd09ll","coordinates":{"bd09ll":{"lat":30.1,"lng":120.1,"crs":"bd09ll"}}}},{"id":"stop-2","sequence":2,"title":"终点","location":{"preferred":"bd09ll","coordinates":{"bd09ll":{"lat":30.2,"lng":120.2,"crs":"bd09ll"}}}}],"legs":[{"id":"leg-1","from_stop_id":"stop-1","to_stop_id":"stop-2"}]}]}`
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

	request, err := http.NewRequest(http.MethodPatch, server.URL+"/api/v1/trips/"+created.ID+"/days/day-1/stops/stop-1", strings.NewReader(`{"exclude_from_route":true}`))
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
		Changes struct {
			RouteExcludedChanged bool `json:"route_excluded_changed"`
			RouteInvalidated     bool `json:"route_invalidated"`
		} `json:"changes"`
		Document map[string]any `json:"document"`
	}
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if !payload.Changes.RouteExcludedChanged || !payload.Changes.RouteInvalidated {
		t.Fatalf("unexpected route-exclusion changes: %+v", payload.Changes)
	}
	day := payload.Document["days"].([]any)[0].(map[string]any)
	if _, exists := day["legs"]; exists {
		t.Fatal("existing routes were not cleared")
	}
	stop := day["stops"].([]any)[0].(map[string]any)
	if stop["exclude_from_route"] != true {
		t.Fatalf("route exclusion was not returned: %+v", stop)
	}
}
