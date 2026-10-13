package httpapi

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"journeyin/internal/application"
	journeyshare "journeyin/internal/share"
)

func TestShareBundlePublicReadAndAtomicImportFlow(t *testing.T) {
	server := testHTTPServer(t)
	defer server.Close()
	client := server.Client()
	postJSON := func(path string, value any) *http.Response {
		t.Helper()
		encoded, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		response, err := client.Post(server.URL+path, "application/json", bytes.NewReader(encoded))
		if err != nil {
			t.Fatal(err)
		}
		return response
	}
	tripIDs := make([]string, 0, 2)
	for index, title := range []string{"跨实例 <Trip A>", "Trip B"} {
		suffix := string(rune(97 + index))
		dayID := "day-source-" + suffix
		stopA := "stop-source-" + suffix
		stopB := "stop-destination-" + suffix
		document := map[string]any{
			"schema_version": 1, "title": title, "status": "draft", "timezone": "Asia/Shanghai",
			"date_range": map[string]string{"start": "2026-10-13", "end": "2026-10-13"},
			"days": []any{map[string]any{
				"id": dayID, "date": "2026-10-13",
				"stops": []any{map[string]any{"id": stopA, "sequence": 1, "title": "Place A", "location": map[string]any{"provider_refs": map[string]any{"amap": map[string]any{"api_key": "SHOULD_NOT_SHARE"}}}}, map[string]any{"id": stopB, "sequence": 2, "title": "Place B"}},
				"legs":  []any{map[string]any{"id": "leg-source-" + suffix, "from_stop_id": stopA, "to_stop_id": stopB}},
			}},
		}
		response := postJSON("/api/v1/trips", document)
		if response.StatusCode != http.StatusCreated {
			body, _ := io.ReadAll(response.Body)
			response.Body.Close()
			t.Fatalf("create trip status %d: %s", response.StatusCode, body)
		}
		var created map[string]any
		if err := json.NewDecoder(response.Body).Decode(&created); err != nil {
			response.Body.Close()
			t.Fatal(err)
		}
		response.Body.Close()
		tripIDs = append(tripIDs, created["id"].(string))
	}
	createResponse := postJSON("/api/v1/share-bundles", map[string]any{"trip_ids": tripIDs, "ttl_seconds": 3600})
	if createResponse.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(createResponse.Body)
		createResponse.Body.Close()
		t.Fatalf("create bundle status %d: %s", createResponse.StatusCode, body)
	}
	var createdBundle struct {
		ID        string `json:"id"`
		URL       string `json:"url"`
		ItemCount int    `json:"item_count"`
	}
	if err := json.NewDecoder(createResponse.Body).Decode(&createdBundle); err != nil {
		createResponse.Body.Close()
		t.Fatal(err)
	}
	createResponse.Body.Close()
	if createdBundle.ID == "" || createdBundle.ItemCount != 2 {
		t.Fatalf("unexpected bundle response: %+v", createdBundle)
	}
	shareURL, err := url.Parse(createdBundle.URL)
	if err != nil {
		t.Fatal(err)
	}
	manageResponse, err := client.Get(server.URL + "/api/v1/share-bundles")
	if err != nil {
		t.Fatal(err)
	}
	manageBody, _ := io.ReadAll(manageResponse.Body)
	manageResponse.Body.Close()
	var managed struct {
		Items []struct {
			ID         string   `json:"id"`
			Status     string   `json:"status"`
			ItemCount  int      `json:"item_count"`
			TripTitles []string `json:"trip_titles"`
		} `json:"items"`
	}
	if err := json.Unmarshal(manageBody, &managed); err != nil {
		t.Fatal(err)
	}
	if manageResponse.StatusCode != http.StatusOK || len(managed.Items) != 1 || managed.Items[0].ID != createdBundle.ID || managed.Items[0].Status != "active" || managed.Items[0].ItemCount != 2 || strings.Contains(string(manageBody), strings.TrimPrefix(shareURL.Path, "/s/")) {
		t.Fatalf("bundle management list leaked token or missed share: %s", manageBody)
	}
	page, err := client.Get(server.URL + shareURL.Path)
	if err != nil {
		t.Fatal(err)
	}
	pageBody, _ := io.ReadAll(page.Body)
	page.Body.Close()
	if page.StatusCode != http.StatusOK || page.Header.Get("X-Robots-Tag") == "" || page.Header.Get("Referrer-Policy") != "no-referrer" || !strings.Contains(string(pageBody), "&lt;Trip A&gt;") || !strings.Contains(string(pageBody), "下载分享包 JSON") {
		t.Fatalf("unexpected public bundle page: status=%d body=%s", page.StatusCode, pageBody)
	}
	jsonRequest, _ := http.NewRequest(http.MethodGet, server.URL+shareURL.Path+".json", nil)
	jsonRequest.Header.Set("Origin", "https://instance-b.example")
	shared, err := client.Do(jsonRequest)
	if err != nil {
		t.Fatal(err)
	}
	rawBundle, _ := io.ReadAll(shared.Body)
	shared.Body.Close()
	if shared.StatusCode != http.StatusOK || shared.Header.Get("Access-Control-Allow-Origin") != "*" {
		t.Fatalf("public bundle JSON status/CORS invalid: %d %q", shared.StatusCode, shared.Header.Get("Access-Control-Allow-Origin"))
	}
	if strings.Contains(string(rawBundle), "revision") || strings.Contains(string(rawBundle), "owner") || strings.Contains(string(rawBundle), "api_key") || strings.Contains(string(rawBundle), "SHOULD_NOT_SHARE") {
		t.Fatalf("private metadata leaked in public bundle: %s", rawBundle)
	}
	var bundle journeyshare.BundleDocument
	if err := json.Unmarshal(rawBundle, &bundle); err != nil {
		t.Fatal(err)
	}
	if bundle.SourceOrigin != "http://example.test" || len(bundle.Items) != 2 {
		t.Fatalf("unexpected bundle payload: %+v", bundle)
	}
	previewResponse := postJSON("/api/v1/share-import/previews", map[string]any{"source_origin": bundle.SourceOrigin, "items": bundle.Items})
	if previewResponse.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(previewResponse.Body)
		previewResponse.Body.Close()
		t.Fatalf("preview status %d: %s", previewResponse.StatusCode, body)
	}
	var preview application.ShareImportPreviewResult
	if err := json.NewDecoder(previewResponse.Body).Decode(&preview); err != nil {
		previewResponse.Body.Close()
		t.Fatal(err)
	}
	previewResponse.Body.Close()
	if !preview.RequiresConfirmation || preview.PreviewID == "" || len(preview.Items) != 2 {
		t.Fatalf("unexpected import preview: %+v", preview)
	}
	commitBody := map[string]any{"preview_id": preview.PreviewID, "confirmation_token": preview.ConfirmationToken, "idempotency_key": "bundle-import-commit-1"}
	commitResponse := postJSON("/api/v1/share-import/commits", commitBody)
	if commitResponse.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(commitResponse.Body)
		commitResponse.Body.Close()
		t.Fatalf("commit status %d: %s", commitResponse.StatusCode, body)
	}
	var committed application.ShareImportCommitResult
	if err := json.NewDecoder(commitResponse.Body).Decode(&committed); err != nil {
		commitResponse.Body.Close()
		t.Fatal(err)
	}
	commitResponse.Body.Close()
	if committed.Status != "created" || len(committed.Items) != 2 {
		t.Fatalf("unexpected commit result: %+v", committed)
	}
	type importedResponse struct {
		Document struct {
			ID   string `json:"id"`
			Days []struct {
				ID    string `json:"id"`
				Stops []struct {
					ID string `json:"id"`
				} `json:"stops"`
				Legs []struct {
					ID         string `json:"id"`
					FromStopID string `json:"from_stop_id"`
					ToStopID   string `json:"to_stop_id"`
				} `json:"legs"`
			} `json:"days"`
		} `json:"document"`
	}
	for index, item := range committed.Items {
		if item.ID == tripIDs[index] {
			t.Fatalf("import reused source trip id %s", item.ID)
		}
		response, err := client.Get(server.URL + "/api/v1/trips/" + url.PathEscape(item.ID))
		if err != nil {
			t.Fatal(err)
		}
		var imported importedResponse
		if err := json.NewDecoder(response.Body).Decode(&imported); err != nil {
			response.Body.Close()
			t.Fatal(err)
		}
		response.Body.Close()
		if response.StatusCode != http.StatusOK || imported.Document.ID == tripIDs[index] || len(imported.Document.Days) != 1 {
			t.Fatalf("imported copy missing or has source root ID: %+v", imported)
		}
		suffix := string(rune(97 + index))
		day := imported.Document.Days[0]
		if day.ID == "day-source-"+suffix || len(day.Stops) != 2 || len(day.Legs) != 1 {
			t.Fatalf("day/stop/leg IDs were not copied: %+v", day)
		}
		if day.Stops[0].ID == "stop-source-"+suffix || day.Stops[1].ID == "stop-destination-"+suffix || day.Legs[0].ID == "leg-source-"+suffix {
			t.Fatalf("nested entity IDs were not remapped: %+v", day)
		}
		if day.Legs[0].FromStopID != day.Stops[0].ID || day.Legs[0].ToStopID != day.Stops[1].ID {
			t.Fatalf("leg endpoints were not remapped with stops: %+v", day.Legs[0])
		}
	}
	replay := postJSON("/api/v1/share-import/commits", commitBody)
	var replayed application.ShareImportCommitResult
	if err := json.NewDecoder(replay.Body).Decode(&replayed); err != nil {
		replay.Body.Close()
		t.Fatal(err)
	}
	replay.Body.Close()
	if replay.StatusCode != http.StatusOK || replayed.Status != "already_applied" || len(replayed.Items) != 2 {
		t.Fatalf("idempotent replay failed: status=%d result=%+v", replay.StatusCode, replayed)
	}
	duplicatePreview := postJSON("/api/v1/share-import/previews", map[string]any{"source_origin": bundle.SourceOrigin, "items": bundle.Items})
	var duplicates application.ShareImportPreviewResult
	if err := json.NewDecoder(duplicatePreview.Body).Decode(&duplicates); err != nil {
		duplicatePreview.Body.Close()
		t.Fatal(err)
	}
	duplicatePreview.Body.Close()
	if duplicatePreview.StatusCode != http.StatusOK || duplicates.RequiresConfirmation {
		t.Fatalf("re-import was not recognized as duplicate: %+v", duplicates)
	}
	for _, item := range duplicates.Items {
		if !item.AlreadyImported {
			t.Fatalf("item %s should be marked imported", item.SourceTripID)
		}
	}
	revoke, err := client.Post(server.URL+"/api/v1/share-bundles/"+url.PathEscape(createdBundle.ID)+"/revoke", "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	revoke.Body.Close()
	if revoke.StatusCode != http.StatusNoContent {
		t.Fatalf("revoke status %d", revoke.StatusCode)
	}
	revoked, err := client.Get(server.URL + shareURL.Path + ".json")
	if err != nil {
		t.Fatal(err)
	}
	revoked.Body.Close()
	if revoked.StatusCode != http.StatusNotFound {
		t.Fatalf("revoked bundle remained readable: %d", revoked.StatusCode)
	}
	manageAfter, err := client.Get(server.URL + "/api/v1/share-bundles")
	if err != nil {
		t.Fatal(err)
	}
	var after struct {
		Items []struct {
			ID     string `json:"id"`
			Status string `json:"status"`
		} `json:"items"`
	}
	if err := json.NewDecoder(manageAfter.Body).Decode(&after); err != nil {
		manageAfter.Body.Close()
		t.Fatal(err)
	}
	manageAfter.Body.Close()
	if manageAfter.StatusCode != http.StatusOK || len(after.Items) != 0 {
		t.Fatalf("revoked bundle should be hidden from management list: %+v", after)
	}
}
