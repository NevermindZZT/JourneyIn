package httpapi

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestRefreshTripWeatherBatchHTTPReportsPointFailures(t *testing.T) {
	server := testPlanningServer(t)
	defer server.Close()
	trip := "{\"schema_version\":1,\"title\":\"批量天气\",\"status\":\"draft\",\"timezone\":\"Asia/Shanghai\",\"date_range\":{\"start\":\"2026-04-18\",\"end\":\"2026-04-18\"},\"days\":[{\"id\":\"day-1\",\"date\":\"2026-04-18\",\"stops\":[{\"id\":\"stop-a\",\"sequence\":1,\"title\":\"待定位\"},{\"id\":\"stop-b\",\"sequence\":2,\"title\":\"已定位\",\"location\":{\"preferred\":\"bd09ll\",\"coordinates\":{\"bd09ll\":{\"lat\":30.2,\"lng\":120.1,\"crs\":\"bd09ll\"}}}}]}]}"
	create, err := http.Post(server.URL+"/api/v1/trips", "application/json", strings.NewReader(trip))
	if err != nil {
		t.Fatal(err)
	}
	defer create.Body.Close()
	var created struct {
		ID       string `json:"id"`
		Revision int    `json:"revision"`
	}
	if create.StatusCode != http.StatusCreated {
		t.Fatalf("create status=%d", create.StatusCode)
	}
	if err := json.NewDecoder(create.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}
	endpoint := server.URL + "/api/v1/trips/" + created.ID + "/weather/refresh"
	request, err := http.NewRequest(http.MethodPost, endpoint, strings.NewReader("{\"provider\":\"amap\",\"offset\":0}"))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("If-Match", "revision-1")
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var payload struct {
		Revision int `json:"revision"`
		Progress struct {
			Total      int `json:"total"`
			NextOffset int `json:"next_offset"`
			Skipped    int `json:"skipped"`
			Failed     int `json:"failed"`
		} `json:"progress"`
	}
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusOK || payload.Progress.Total != 2 || payload.Progress.NextOffset != 2 || payload.Progress.Skipped != 1 || payload.Progress.Failed != 1 || payload.Revision != created.Revision {
		t.Fatalf("unexpected response: status=%d payload=%+v", response.StatusCode, payload)
	}
	missing, err := http.Post(endpoint, "application/json", strings.NewReader("{}"))
	if err != nil {
		t.Fatal(err)
	}
	defer missing.Body.Close()
	if missing.StatusCode != http.StatusPreconditionRequired {
		t.Fatalf("missing If-Match status=%d", missing.StatusCode)
	}
}
