package httpapi

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestUpdatePlanningPointKindHTTP(t *testing.T) {
	server := testHTTPServer(t)
	defer server.Close()
	trip := []byte(`{"schema_version":1,"title":"标签 PATCH","status":"draft","timezone":"Asia/Shanghai","date_range":{"start":"2026-04-18","end":"2026-04-18"},"days":[{"id":"day-1","date":"2026-04-18","stops":[{"id":"stop-1","sequence":1,"title":"住宿点"}]}]}`)
	create, err := http.Post(server.URL+"/api/v1/trips", "application/json", strings.NewReader(string(trip)))
	if err != nil {
		t.Fatal(err)
	}
	defer create.Body.Close()
	var created struct {
		ID string `json:"id"`
	}
	if create.StatusCode != http.StatusCreated || json.NewDecoder(create.Body).Decode(&created) != nil {
		t.Fatalf("create status=%d", create.StatusCode)
	}
	request, err := http.NewRequest(http.MethodPatch, server.URL+"/api/v1/trips/"+created.ID+"/days/day-1/stops/stop-1", strings.NewReader("{\"kind\":\"lodging\"}"))
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
	var payload struct {
		Changes struct {
			KindChanged bool `json:"kind_changed"`
		} `json:"changes"`
		Document struct {
			Days []struct {
				Stops []struct {
					Kind string `json:"kind"`
				} `json:"stops"`
			} `json:"days"`
		} `json:"document"`
	}
	if response.StatusCode != http.StatusOK {
		t.Fatalf("kind update status=%d", response.StatusCode)
	}
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if !payload.Changes.KindChanged || payload.Document.Days[0].Stops[0].Kind != "lodging" {
		t.Fatalf("unexpected kind update payload: %+v", payload)
	}
}
