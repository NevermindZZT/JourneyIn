package httpapi

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestPlanningPointMutationDeltaResponseWorkflow(t *testing.T) {
	server := testHTTPServer(t)
	defer server.Close()
	location := `{"preferred":"bd09ll","coordinates":{"bd09ll":{"lat":30.1,"lng":120.1,"crs":"bd09ll"}}}`
	trip := `{"schema_version":1,"title":"Delta","status":"draft","timezone":"Asia/Shanghai","date_range":{"start":"2026-04-18","end":"2026-04-18"},"days":[{"id":"day-1","date":"2026-04-18","stops":[{"id":"stop-a","sequence":1,"title":"A","location":` + location + `},{"id":"stop-b","sequence":2,"title":"B","location":` + location + `}]}]}`
	createResponse, err := http.Post(server.URL+"/api/v1/trips", "application/json", strings.NewReader(trip))
	if err != nil {
		t.Fatal(err)
	}
	defer createResponse.Body.Close()
	var created struct {
		ID string `json:"id"`
	}
	if createResponse.StatusCode != http.StatusCreated || json.NewDecoder(createResponse.Body).Decode(&created) != nil || created.ID == "" {
		t.Fatalf("create failed: status=%d id=%q", createResponse.StatusCode, created.ID)
	}

	mutate := func(method, suffix, body string, revision int) map[string]any {
		t.Helper()
		request := historyRequest(t, method, server.URL+"/api/v1/trips/"+created.ID+suffix, body, revision, "")
		request.Header.Set("Prefer", "return=delta")
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		var payload map[string]any
		if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		if response.StatusCode != http.StatusOK || response.Header.Get("Preference-Applied") != "return=delta" {
			t.Fatalf("delta mutation failed: status=%d headers=%v payload=%+v", response.StatusCode, response.Header, payload)
		}
		if _, exists := payload["document"]; exists {
			t.Fatalf("delta response unexpectedly contains full document: %+v", payload)
		}
		if _, ok := payload["delta"].(map[string]any); !ok {
			t.Fatalf("missing delta: %+v", payload)
		}
		return payload
	}
	dayStops := func(payload map[string]any) []any {
		t.Helper()
		days := payload["delta"].(map[string]any)["days"].([]any)
		if len(days) != 1 || days[0].(map[string]any)["id"] != "day-1" {
			t.Fatalf("unexpected delta days: %+v", days)
		}
		return days[0].(map[string]any)["stops"].([]any)
	}
	titles := func(stops []any) []string {
		result := make([]string, 0, len(stops))
		for _, raw := range stops {
			result = append(result, raw.(map[string]any)["title"].(string))
		}
		return result
	}

	updated := mutate(http.MethodPatch, "/days/day-1/stops/stop-a", `{"title":"A 更新"}`, 1)
	if got := titles(dayStops(updated)); len(got) != 2 || got[0] != "A 更新" || got[1] != "B" || updated["revision"] != float64(2) {
		t.Fatalf("unexpected update delta: titles=%v payload=%+v", got, updated)
	}
	added := mutate(http.MethodPost, "/days/day-1/stops", `{"stop":{"title":"C","location":`+location+`}}`, 2)
	if got := titles(dayStops(added)); len(got) != 3 || got[2] != "C" || added["revision"] != float64(3) {
		t.Fatalf("unexpected add delta: titles=%v payload=%+v", got, added)
	}
	stops := dayStops(added)
	stopCID := stops[2].(map[string]any)["id"].(string)
	moved := mutate(http.MethodPost, "/days/day-1/stops/"+stopCID+"/move", `{"target_sequence":1}`, 3)
	if got := titles(dayStops(moved)); len(got) != 3 || got[0] != "C" || got[1] != "A 更新" || moved["revision"] != float64(4) {
		t.Fatalf("unexpected move delta: titles=%v payload=%+v", got, moved)
	}
	deleted := mutate(http.MethodDelete, "/days/day-1/stops/stop-a", "", 4)
	if got := titles(dayStops(deleted)); len(got) != 2 || got[0] != "C" || got[1] != "B" || deleted["revision"] != float64(5) {
		t.Fatalf("unexpected delete delta: titles=%v payload=%+v", got, deleted)
	}
}
