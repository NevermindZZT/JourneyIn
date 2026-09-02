package httpapi

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestUpdateTripDetailsHTTPWorkflow(t *testing.T) {
	server := testHTTPServer(t)
	defer server.Close()

	trip := "{\"schema_version\":1,\"title\":\"旧行程\",\"status\":\"draft\",\"timezone\":\"Asia/Shanghai\",\"date_range\":{\"start\":\"2026-04-18\",\"end\":\"2026-04-19\"},\"days\":[{\"id\":\"day-1\",\"date\":\"2026-04-18\",\"stops\":[{\"id\":\"stop-1\",\"sequence\":1,\"title\":\"西湖\"}]},{\"id\":\"day-2\",\"date\":\"2026-04-19\",\"stops\":[]}]}"
	createResponse, err := http.Post(server.URL+"/api/v1/trips", "application/json", strings.NewReader(trip))
	if err != nil {
		t.Fatal(err)
	}
	defer createResponse.Body.Close()
	if createResponse.StatusCode != http.StatusCreated {
		t.Fatalf("create status %d", createResponse.StatusCode)
	}
	var created map[string]any
	if err := json.NewDecoder(createResponse.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}
	tripID, _ := created["id"].(string)
	if tripID == "" {
		t.Fatal("missing trip id")
	}

	request, err := http.NewRequest(http.MethodPatch, server.URL+"/api/v1/trips/"+tripID, strings.NewReader("{\"title\":\"杭州春日慢游\",\"date_range\":{\"start\":\"2026-04-25\",\"end\":\"2026-04-27\"}}"))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("If-Match", "revision-1")
	request.Header.Set("Idempotency-Key", "details-update-1")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("update status %d", response.StatusCode)
	}
	var updated struct {
		Title     string `json:"title"`
		StartDate string `json:"start_date"`
		EndDate   string `json:"end_date"`
		Revision  int    `json:"revision"`
		Changes   struct {
			Changed          bool `json:"changed"`
			TitleChanged     bool `json:"title_changed"`
			DateRangeChanged bool `json:"date_range_changed"`
			AddedDays        int  `json:"added_days"`
		} `json:"changes"`
		Document struct {
			Title     string `json:"title"`
			DateRange struct {
				Start string `json:"start"`
				End   string `json:"end"`
			} `json:"date_range"`
			Days []struct {
				ID   string `json:"id"`
				Date string `json:"date"`
			} `json:"days"`
		} `json:"document"`
	}
	if err := json.NewDecoder(response.Body).Decode(&updated); err != nil {
		t.Fatal(err)
	}
	if updated.Title != "杭州春日慢游" || updated.StartDate != "2026-04-25" || updated.EndDate != "2026-04-27" || updated.Revision != 2 {
		t.Fatalf("unexpected response: %+v", updated)
	}
	if !updated.Changes.Changed || !updated.Changes.TitleChanged || !updated.Changes.DateRangeChanged || updated.Changes.AddedDays != 1 {
		t.Fatalf("unexpected changes: %+v", updated.Changes)
	}
	if updated.Document.Title != updated.Title || updated.Document.DateRange.Start != updated.StartDate || len(updated.Document.Days) != 3 || updated.Document.Days[0].ID != "day-1" || updated.Document.Days[0].Date != "2026-04-25" || updated.Document.Days[2].Date != "2026-04-27" {
		t.Fatalf("document was not updated atomically: %+v", updated.Document)
	}

	replay, err := http.NewRequest(http.MethodPatch, server.URL+"/api/v1/trips/"+tripID, strings.NewReader("{\"title\":\"杭州春日慢游\",\"date_range\":{\"start\":\"2026-04-25\",\"end\":\"2026-04-27\"}}"))
	if err != nil {
		t.Fatal(err)
	}
	replay.Header.Set("Content-Type", "application/json")
	replay.Header.Set("If-Match", "revision-1")
	replay.Header.Set("Idempotency-Key", "details-update-1")
	replayResponse, err := http.DefaultClient.Do(replay)
	if err != nil {
		t.Fatal(err)
	}
	defer replayResponse.Body.Close()
	var replayPayload map[string]any
	if err := json.NewDecoder(replayResponse.Body).Decode(&replayPayload); err != nil {
		t.Fatal(err)
	}
	if replayResponse.StatusCode != http.StatusOK || replayPayload["idempotency_replay"] != true || replayPayload["revision"] != float64(2) {
		t.Fatalf("unexpected idempotency replay: status=%d payload=%+v", replayResponse.StatusCode, replayPayload)
	}

	stale, err := http.NewRequest(http.MethodPatch, server.URL+"/api/v1/trips/"+tripID, strings.NewReader("{\"title\":\"过期修改\"}"))
	if err != nil {
		t.Fatal(err)
	}
	stale.Header.Set("Content-Type", "application/json")
	stale.Header.Set("If-Match", "revision-1")
	stale.Header.Set("Idempotency-Key", "details-stale")
	staleResponse, err := http.DefaultClient.Do(stale)
	if err != nil {
		t.Fatal(err)
	}
	defer staleResponse.Body.Close()
	if staleResponse.StatusCode != http.StatusConflict {
		t.Fatalf("stale update status %d", staleResponse.StatusCode)
	}
}

func TestUpdateTripDetailsHTTPRejectsNonEmptyTail(t *testing.T) {
	server := testHTTPServer(t)
	defer server.Close()

	trip := "{\"schema_version\":1,\"title\":\"三日行程\",\"status\":\"draft\",\"timezone\":\"Asia/Shanghai\",\"date_range\":{\"start\":\"2026-04-18\",\"end\":\"2026-04-20\"},\"days\":[{\"id\":\"day-1\",\"date\":\"2026-04-18\",\"stops\":[]},{\"id\":\"day-2\",\"date\":\"2026-04-19\",\"stops\":[]},{\"id\":\"day-3\",\"date\":\"2026-04-20\",\"stops\":[{\"id\":\"stop-3\",\"sequence\":1,\"title\":\"灵隐寺\"}]}]}"
	createResponse, err := http.Post(server.URL+"/api/v1/trips", "application/json", strings.NewReader(trip))
	if err != nil {
		t.Fatal(err)
	}
	defer createResponse.Body.Close()
	var created map[string]any
	if err := json.NewDecoder(createResponse.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}
	tripID, _ := created["id"].(string)
	request, err := http.NewRequest(http.MethodPatch, server.URL+"/api/v1/trips/"+tripID, strings.NewReader("{\"date_range\":{\"start\":\"2026-04-18\",\"end\":\"2026-04-19\"}}"))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("If-Match", "revision-1")
	request.Header.Set("Idempotency-Key", "details-conflict")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusConflict {
		t.Fatalf("conflict status %d", response.StatusCode)
	}
	var payload struct {
		Error struct {
			Code    string `json:"code"`
			Details struct {
				Days []struct {
					DayID string `json:"day_id"`
				} `json:"days"`
			} `json:"details"`
		} `json:"error"`
	}
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if payload.Error.Code != "date_range_conflict" || len(payload.Error.Details.Days) != 1 || payload.Error.Details.Days[0].DayID != "day-3" {
		t.Fatalf("unexpected conflict payload: %+v", payload)
	}
}

func TestUpdateTripDetailsHTTPRequiresRevision(t *testing.T) {
	server := testHTTPServer(t)
	defer server.Close()
	request, err := http.NewRequest(http.MethodPatch, server.URL+"/api/v1/trips/missing", strings.NewReader("{\"title\":\"新名称\"}"))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusPreconditionRequired {
		t.Fatalf("missing revision status %d", response.StatusCode)
	}
}
