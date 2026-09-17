package httpapi

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestAtlasSummaryIncludesRoutesAndHonorsShowInAtlas(t *testing.T) {
	server := testPlanningServer(t)
	defer server.Close()

	// 1. 创建第一条行程（包含路线，默认 show_in_atlas = true）
	trip1 := `{"schema_version":1,"title":"甘南大环线","status":"draft","timezone":"Asia/Shanghai","date_range":{"start":"2026-05-01","end":"2026-05-02"},"days":[{"id":"day-1","date":"2026-05-01","stops":[{"id":"stop-1","sequence":1,"title":"兰州中川机场","location":{"preferred":"gcj02","coordinates":{"gcj02":{"lat":36.5,"lng":103.6}}}},{"id":"stop-2","sequence":2,"title":"临夏八坊十三巷","location":{"preferred":"gcj02","coordinates":{"gcj02":{"lat":35.6,"lng":103.2}}}}],"legs":[{"id":"leg-1","from_stop_id":"stop-1","to_stop_id":"stop-2","mode":"driving","snapshots":[{"provider":"amap","coordinate_system":"gcj02","mode":"driving","geometry":[[103.6,36.5],[103.4,36.0],[103.2,35.6]],"distance_m":120000,"duration_s":5400}]}]}]}`
	resp1, err := http.Post(server.URL+"/api/v1/trips", "application/json", strings.NewReader(trip1))
	if err != nil || resp1.StatusCode != http.StatusCreated {
		t.Fatalf("create trip1 failed: %v", err)
	}
	var created1 struct {
		ID       string `json:"id"`
		Revision int    `json:"revision"`
	}
	_ = json.NewDecoder(resp1.Body).Decode(&created1)
	resp1.Body.Close()

	// 2. 创建第二条行程（显式排除：show_in_atlas = false）
	trip2 := `{"schema_version":1,"title":"测试草稿行程","status":"draft","timezone":"Asia/Shanghai","show_in_atlas":false,"date_range":{"start":"2026-06-01","end":"2026-06-01"},"days":[{"id":"day-1","date":"2026-06-01","stops":[{"id":"stop-a","sequence":1,"title":"点A","location":{"preferred":"gcj02","coordinates":{"gcj02":{"lat":30.1,"lng":120.1}}}}]}]}`
	resp2, err := http.Post(server.URL+"/api/v1/trips", "application/json", strings.NewReader(trip2))
	if err != nil || resp2.StatusCode != http.StatusCreated {
		t.Fatalf("create trip2 failed: %v", err)
	}
	resp2.Body.Close()

	// 3. 调用 GET /api/v1/atlas 验证汇总
	atlasResp, err := http.Get(server.URL + "/api/v1/atlas")
	if err != nil {
		t.Fatalf("get atlas error: %v", err)
	}
	defer atlasResp.Body.Close()

	var summary AtlasSummaryResponse
	if err := json.NewDecoder(atlasResp.Body).Decode(&summary); err != nil {
		t.Fatalf("decode atlas response: %v", err)
	}

	if summary.TotalTrips != 1 {
		t.Fatalf("expected 1 trip in atlas, got %d", summary.TotalTrips)
	}
	if summary.TotalDistanceM != 120000 {
		t.Fatalf("expected 120000m distance, got %d", summary.TotalDistanceM)
	}
	if len(summary.Trips) != 1 || summary.Trips[0].Title != "甘南大环线" {
		t.Fatalf("unexpected trip items: %+v", summary.Trips)
	}
	if len(summary.Trips[0].Legs) != 1 || len(summary.Trips[0].Legs[0].Geometry) != 3 {
		t.Fatalf("unexpected route geometry: %+v", summary.Trips[0].Legs)
	}

	// 4. 测试通过 PATCH /api/v1/trips/{id} 将 trip1 隐藏
	hidePayload := `{"show_in_atlas":false}`
	req, _ := http.NewRequest(http.MethodPatch, server.URL+"/api/v1/trips/"+created1.ID, strings.NewReader(hidePayload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("If-Match", "revision-1")
	req.Header.Set("Idempotency-Key", "test-hide")
	patchResp, err := http.DefaultClient.Do(req)
	if err != nil || patchResp.StatusCode != http.StatusOK {
		t.Fatalf("patch trip1 failed: %v", err)
	}
	patchResp.Body.Close()

	// 再次查询 atlas 验证已无行程
	atlasResp2, _ := http.Get(server.URL + "/api/v1/atlas")
	var summary2 AtlasSummaryResponse
	_ = json.NewDecoder(atlasResp2.Body).Decode(&summary2)
	atlasResp2.Body.Close()

	if summary2.TotalTrips != 0 {
		t.Fatalf("expected 0 trips in atlas after hide, got %d", summary2.TotalTrips)
	}
}
