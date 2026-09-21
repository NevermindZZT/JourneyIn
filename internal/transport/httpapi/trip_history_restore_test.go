package httpapi

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestRestoreTripHistoryHTTPWorkflow(t *testing.T) {
	server := testHTTPServer(t)
	defer server.Close()
	trip := `{"schema_version":1,"title":"历史初始","status":"draft","timezone":"Asia/Shanghai","date_range":{"start":"2026-04-18","end":"2026-04-18"},"days":[{"id":"day-1","date":"2026-04-18","stops":[]}]}`
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

	save := historyRequest(t, http.MethodPost, server.URL+"/api/v1/trips/"+created.ID+"/history", `{"label":"初始版本"}`, 1, "history-save-initial")
	saveResponse, err := http.DefaultClient.Do(save)
	if err != nil {
		t.Fatal(err)
	}
	defer saveResponse.Body.Close()
	var saved map[string]any
	if saveResponse.StatusCode != http.StatusCreated || json.NewDecoder(saveResponse.Body).Decode(&saved) != nil {
		t.Fatalf("save failed: status=%d", saveResponse.StatusCode)
	}
	historyID, _ := saved["history_id"].(string)
	if historyID == "" {
		t.Fatalf("missing history id: %+v", saved)
	}

	updated := `{"schema_version":1,"title":"当前修改","status":"draft","timezone":"Asia/Shanghai","date_range":{"start":"2026-04-18","end":"2026-04-18"},"days":[{"id":"day-1","date":"2026-04-18","stops":[]}]}`
	update := historyRequest(t, http.MethodPut, server.URL+"/api/v1/trips/"+created.ID, updated, 1, "")
	updateResponse, err := http.DefaultClient.Do(update)
	if err != nil {
		t.Fatal(err)
	}
	updateResponse.Body.Close()
	if updateResponse.StatusCode != http.StatusOK {
		t.Fatalf("update status=%d", updateResponse.StatusCode)
	}

	restore := historyRequest(t, http.MethodPost, server.URL+"/api/v1/trips/"+created.ID+"/history/"+historyID+"/restore", "", 2, "history-restore-1")
	restoreResponse, err := http.DefaultClient.Do(restore)
	if err != nil {
		t.Fatal(err)
	}
	defer restoreResponse.Body.Close()
	var payload map[string]any
	if restoreResponse.StatusCode != http.StatusOK || json.NewDecoder(restoreResponse.Body).Decode(&payload) != nil {
		t.Fatalf("restore failed: status=%d", restoreResponse.StatusCode)
	}
	if payload["revision"] != float64(3) || payload["restored_from_history_id"] != historyID || payload["idempotency_replay"] != false || payload["document"].(map[string]any)["title"] != "历史初始" {
		t.Fatalf("unexpected restore payload: %+v", payload)
	}

	replay := historyRequest(t, http.MethodPost, server.URL+"/api/v1/trips/"+created.ID+"/history/"+historyID+"/restore", "", 2, "history-restore-1")
	replayResponse, err := http.DefaultClient.Do(replay)
	if err != nil {
		t.Fatal(err)
	}
	defer replayResponse.Body.Close()
	var replayPayload map[string]any
	if replayResponse.StatusCode != http.StatusOK || json.NewDecoder(replayResponse.Body).Decode(&replayPayload) != nil || replayPayload["idempotency_replay"] != true || replayPayload["revision"] != float64(3) {
		t.Fatalf("unexpected restore replay: status=%d payload=%+v", replayResponse.StatusCode, replayPayload)
	}

	stale := historyRequest(t, http.MethodPost, server.URL+"/api/v1/trips/"+created.ID+"/history/"+historyID+"/restore", "", 2, "history-restore-stale")
	staleResponse, err := http.DefaultClient.Do(stale)
	if err != nil {
		t.Fatal(err)
	}
	staleResponse.Body.Close()
	if staleResponse.StatusCode != http.StatusConflict {
		t.Fatalf("stale restore status=%d, want %d", staleResponse.StatusCode, http.StatusConflict)
	}
}
