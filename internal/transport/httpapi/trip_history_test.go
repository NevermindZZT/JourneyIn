package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
)

func historyRequest(t *testing.T, method, url string, body string, revision int, idempotencyKey string) *http.Request {
	t.Helper()
	request, err := http.NewRequest(method, url, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	if revision > 0 {
		request.Header.Set("If-Match", fmt.Sprintf("revision-%d", revision))
	}
	if idempotencyKey != "" {
		request.Header.Set("Idempotency-Key", idempotencyKey)
	}
	return request
}

func TestTripHistoryHTTPWorkflow(t *testing.T) {
	server := testHTTPServer(t)
	defer server.Close()

	trip := `{"schema_version":1,"title":"历史初始","status":"draft","timezone":"Asia/Shanghai","date_range":{"start":"2026-04-18","end":"2026-04-18"},"days":[{"id":"day-1","date":"2026-04-18","stops":[]}] }`
	createResponse, err := http.Post(server.URL+"/api/v1/trips", "application/json", strings.NewReader(trip))
	if err != nil {
		t.Fatal(err)
	}
	defer createResponse.Body.Close()
	if createResponse.StatusCode != http.StatusCreated {
		t.Fatalf("create status %d", createResponse.StatusCode)
	}
	var created struct {
		ID       string `json:"id"`
		Revision int    `json:"revision"`
	}
	if err := json.NewDecoder(createResponse.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}
	if created.ID == "" || created.Revision != 1 {
		t.Fatalf("unexpected created trip: %+v", created)
	}

	save := func(revision int, key, label string) (int, map[string]any) {
		t.Helper()
		body, err := json.Marshal(map[string]string{"label": label})
		if err != nil {
			t.Fatal(err)
		}
		request := historyRequest(t, http.MethodPost, server.URL+"/api/v1/trips/"+created.ID+"/history", string(body), revision, key)
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		var payload map[string]any
		if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		return response.StatusCode, payload
	}

	status, history := save(1, "history-save-1", "起始版本")
	if status != http.StatusCreated || history["history_id"] == "" || history["source_revision"] != float64(1) || history["read_only"] != true {
		t.Fatalf("unexpected history response: status=%d payload=%+v", status, history)
	}
	historyID, _ := history["history_id"].(string)
	if historyID == "" {
		t.Fatal("missing history id")
	}

	status, replay := save(1, "history-save-1", "起始版本")
	if status != http.StatusOK || replay["idempotency_replay"] != true || replay["history_id"] != historyID {
		t.Fatalf("unexpected history replay: status=%d payload=%+v", status, replay)
	}

	listResponse, err := http.Get(server.URL + "/api/v1/trips/" + created.ID + "/history")
	if err != nil {
		t.Fatal(err)
	}
	defer listResponse.Body.Close()
	var listed struct {
		Items []map[string]any `json:"items"`
	}
	if err := json.NewDecoder(listResponse.Body).Decode(&listed); err != nil {
		t.Fatal(err)
	}
	if listResponse.StatusCode != http.StatusOK || len(listed.Items) != 1 || listed.Items[0]["history_id"] != historyID {
		t.Fatalf("unexpected history list: status=%d items=%+v", listResponse.StatusCode, listed.Items)
	}

	getHistory := func() map[string]any {
		t.Helper()
		response, err := http.Get(server.URL + "/api/v1/trips/" + created.ID + "/history/" + historyID)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		var payload map[string]any
		if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		if response.StatusCode != http.StatusOK {
			t.Fatalf("history get status %d: %+v", response.StatusCode, payload)
		}
		return payload
	}
	beforeUpdate := getHistory()
	if document, ok := beforeUpdate["document"].(map[string]any); !ok || document["title"] != "历史初始" {
		t.Fatalf("unexpected history document: %+v", beforeUpdate)
	}

	updated := `{"schema_version":1,"title":"当前修改","status":"draft","timezone":"Asia/Shanghai","date_range":{"start":"2026-04-18","end":"2026-04-18"},"days":[{"id":"day-1","date":"2026-04-18","stops":[]}]}`
	updateRequest := historyRequest(t, http.MethodPut, server.URL+"/api/v1/trips/"+created.ID, updated, 1, "")
	updateResponse, err := http.DefaultClient.Do(updateRequest)
	if err != nil {
		t.Fatal(err)
	}
	defer updateResponse.Body.Close()
	if updateResponse.StatusCode != http.StatusOK {
		t.Fatalf("update status %d", updateResponse.StatusCode)
	}
	if current := getHistory()["document"].(map[string]any)["title"]; current != "历史初始" {
		t.Fatalf("history changed after current edit: %v", current)
	}

	status, second := save(2, "history-save-2", "修改后版本")
	if status != http.StatusCreated || second["source_revision"] != float64(2) || second["history_id"] == historyID {
		t.Fatalf("unexpected second history: status=%d payload=%+v", status, second)
	}

	deleteRequest := historyRequest(t, http.MethodDelete, server.URL+"/api/v1/trips/"+created.ID+"/history/"+historyID, "", 0, "history-delete-1")
	deleteResponse, err := http.DefaultClient.Do(deleteRequest)
	if err != nil {
		t.Fatal(err)
	}
	deleteResponse.Body.Close()
	if deleteResponse.StatusCode != http.StatusNoContent {
		t.Fatalf("delete status %d", deleteResponse.StatusCode)
	}
	deleteReplay, err := http.DefaultClient.Do(deleteRequest)
	if err != nil {
		t.Fatal(err)
	}
	deleteReplay.Body.Close()
	if deleteReplay.StatusCode != http.StatusNoContent {
		t.Fatalf("delete replay status %d", deleteReplay.StatusCode)
	}

	remaining, err := http.Get(server.URL + "/api/v1/trips/" + created.ID + "/history")
	if err != nil {
		t.Fatal(err)
	}
	defer remaining.Body.Close()
	if remaining.StatusCode != http.StatusOK {
		t.Fatalf("remaining list status %d", remaining.StatusCode)
	}
	var remainingPayload struct {
		Items []map[string]any `json:"items"`
	}
	if err := json.NewDecoder(remaining.Body).Decode(&remainingPayload); err != nil {
		t.Fatal(err)
	}
	if len(remainingPayload.Items) != 1 {
		t.Fatalf("unexpected remaining history: %+v", remainingPayload.Items)
	}

	missingRevision := historyRequest(t, http.MethodPost, server.URL+"/api/v1/trips/"+created.ID+"/history", "{}", 0, "history-missing-revision")
	response, err := http.DefaultClient.Do(missingRevision)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusPreconditionRequired {
		t.Fatalf("missing revision status %d", response.StatusCode)
	}

	missingKey := historyRequest(t, http.MethodPost, server.URL+"/api/v1/trips/"+created.ID+"/history", "{}", 2, "")
	response, err = http.DefaultClient.Do(missingKey)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("missing idempotency status %d", response.StatusCode)
	}
}
