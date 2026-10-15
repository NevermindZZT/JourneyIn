package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	journeyin "journeyin"
	"journeyin/internal/application"
	"journeyin/internal/monitor"
	"journeyin/internal/store"
	"journeyin/internal/weather"
)

func testMonitorHTTPServer(t *testing.T) (*httptest.Server, *store.Store, *monitor.Service) {
	t.Helper()
	migrations, err := fs.Sub(journeyin.MigrationFS, "migrations")
	if err != nil {
		t.Fatal(err)
	}
	database, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "journeyin.db"), migrations)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })

	webFS, _ := fs.Sub(journeyin.WebFS, "web/dist")
	schemaFS, _ := fs.Sub(journeyin.SchemaFS, "schemas")

	app := application.NewTripService(database)
	api := NewServer(app, webFS, schemaFS, "test", nil)
	api.SetSettingsStore(database)

	weatherReg := weather.NewRegistry()
	weatherSvc := weather.NewService(weatherReg, nil, nil, nil, nil, weather.ProviderAuto)
	app.SetWeatherService(weatherSvc)
	api.SetWeatherService(weatherSvc)

	monSvc := monitor.NewService(database, weatherSvc, nil)
	api.SetMonitorService(monSvc)

	ts := httptest.NewServer(api.Handler())
	t.Cleanup(ts.Close)
	return ts, database, monSvc
}

func TestMonitorCRUDAndEvaluate(t *testing.T) {
	ts, _, _ := testMonitorHTTPServer(t)

	// 1. Create a monitor for Lenggacuo (冷嘎措)
	body := map[string]any{
		"title":      "冷嘎措 · 贡嘎倒影与银河",
		"place_name": "冷嘎措",
		"lat":        29.58,
		"lng":        101.78,
		"crs":        "gcj02",
		"criteria": map[string]any{
			"months":            []int{6, 7, 8, 9, 10},
			"stargazing_mode":   true,
			"max_leave_days":    1,
			"min_duration_days": 2,
			"max_duration_days": 3,
		},
		"origin_city": "成都",
		"max_budget":  1200,
	}
	bodyBytes, _ := json.Marshal(body)

	resp, err := http.Post(ts.URL+"/api/v1/monitors", "application/json", bytes.NewReader(bodyBytes))
	if err != nil {
		t.Fatalf("post monitor: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create monitor failed, code=%d", resp.StatusCode)
	}

	var created monitor.Monitor
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		t.Fatalf("decode created monitor: %v", err)
	}
	if created.ID == "" || created.PlaceName != "冷嘎措" {
		t.Fatalf("unexpected created monitor: %+v", created)
	}

	// 2. List monitors
	listResp, err := http.Get(ts.URL + "/api/v1/monitors")
	if err != nil {
		t.Fatalf("get monitors: %v", err)
	}
	defer listResp.Body.Close()

	if listResp.StatusCode != http.StatusOK {
		t.Fatalf("list monitors failed, code=%d", listResp.StatusCode)
	}
	var list []monitor.Monitor
	_ = json.NewDecoder(listResp.Body).Decode(&list)
	if len(list) != 1 {
		t.Fatalf("expected 1 monitor, got %d", len(list))
	}

	// 3. Trigger manual evaluate
	evalResp, err := http.Post(ts.URL+"/api/v1/monitors/"+created.ID+"/evaluate", "application/json", nil)
	if err != nil {
		t.Fatalf("post evaluate: %v", err)
	}
	defer evalResp.Body.Close()

	if evalResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(evalResp.Body)
		t.Fatalf("evaluate monitor failed, code=%d, body=%s", evalResp.StatusCode, string(body))
	}

	// 4. Query radar summary
	sumResp, err := http.Get(ts.URL + "/api/v1/monitors/radar-summary")
	if err != nil {
		t.Fatalf("get radar summary: %v", err)
	}
	defer sumResp.Body.Close()

	if sumResp.StatusCode != http.StatusOK {
		t.Fatalf("radar summary failed, code=%d", sumResp.StatusCode)
	}
	var sum monitor.RadarSummary
	_ = json.NewDecoder(sumResp.Body).Decode(&sum)
	if sum.TotalMonitors != 1 {
		t.Fatalf("expected 1 monitor in summary, got %d", sum.TotalMonitors)
	}

	// 5. Convert opportunity window to trip draft
	convBody := map[string]string{
		"window_start": "2026-06-19",
		"window_end":   "2026-06-21",
	}
	convBytes, _ := json.Marshal(convBody)
	convResp, err := http.Post(ts.URL+"/api/v1/monitors/"+created.ID+"/convert-trip", "application/json", bytes.NewReader(convBytes))
	if err != nil {
		t.Fatalf("post convert trip: %v", err)
	}
	defer convResp.Body.Close()

	if convResp.StatusCode != http.StatusOK {
		t.Fatalf("convert trip failed, code=%d", convResp.StatusCode)
	}

	var convData map[string]string
	_ = json.NewDecoder(convResp.Body).Decode(&convData)
	tripID := convData["trip_id"]
	if tripID == "" {
		t.Fatalf("expected non-empty trip_id from convert")
	}

	// 6. Verify converted trip in TripService
	getTripResp, err := http.Get(fmt.Sprintf("%s/api/v1/trips/%s", ts.URL, tripID))
	if err != nil {
		t.Fatalf("get converted trip: %v", err)
	}
	defer getTripResp.Body.Close()

	if getTripResp.StatusCode != http.StatusOK {
		t.Fatalf("get converted trip failed, code=%d", getTripResp.StatusCode)
	}
	t.Logf("Successfully created monitor, evaluated, and converted to Trip %s", tripID)
}
