package mcptransport

import (
	"context"
	"io/fs"
	"path/filepath"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	journeyinroot "journeyin"
	"journeyin/internal/application"
	"journeyin/internal/monitor"
	"journeyin/internal/weather"
	"journeyin/internal/store"
)

func TestReadTripToolsDeclareDocumentAsObject(t *testing.T) {
	ctx := context.Background()
	server := NewServer(nil, "test", nil)
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	serverSession, err := server.mcp.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatalf("connect server: %v", err)
	}
	defer serverSession.Close()

	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "test"}, nil)
	clientSession, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("connect client: %v", err)
	}
	defer clientSession.Close()

	result, err := clientSession.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}

	for _, name := range []string{"journeyin.get_trip", "journeyin.get_trip_history"} {
		t.Run(name, func(t *testing.T) {
			var tool *mcp.Tool
			for _, candidate := range result.Tools {
				if candidate.Name == name {
					tool = candidate
					break
				}
			}
			if tool == nil {
				t.Fatalf("tool %q not found", name)
			}

			schema, ok := tool.OutputSchema.(map[string]any)
			if !ok {
				t.Fatalf("output schema has type %T, want object schema", tool.OutputSchema)
			}
			if got, _ := schema["type"].(string); got != "object" {
				t.Fatalf("output schema type = %q, want object", got)
			}
			properties, ok := schema["properties"].(map[string]any)
			if !ok {
				t.Fatalf("output schema properties has type %T, want object", schema["properties"])
			}
			document, ok := properties["document"].(map[string]any)
			if !ok {
				t.Fatalf("document schema has type %T, want object schema", properties["document"])
			}
			if got, _ := document["type"].(string); got != "object" {
				t.Fatalf("document schema type = %q, want object", got)
			}
		})
	}
}

func TestDecodeTripDocumentPreservesCompleteObject(t *testing.T) {
	document, err := decodeTripDocument([]byte(`{"schema_version":1,"description_markdown":"# Overview","days":[{"id":"day-1","stops":[{"id":"stop-1","description_markdown":"## Details"}]}]}`))
	if err != nil {
		t.Fatalf("decode document: %v", err)
	}
	if got := document["description_markdown"]; got != "# Overview" {
		t.Fatalf("description_markdown = %#v, want complete Markdown", got)
	}
	days, ok := document["days"].([]any)
	if !ok || len(days) != 1 {
		t.Fatalf("days = %#v, want one day", document["days"])
	}
	day, ok := days[0].(map[string]any)
	if !ok {
		t.Fatalf("day has type %T, want object", days[0])
	}
	stops, ok := day["stops"].([]any)
	if !ok || len(stops) != 1 {
		t.Fatalf("stops = %#v, want one stop", day["stops"])
	}
	stop, ok := stops[0].(map[string]any)
	if !ok || stop["description_markdown"] != "## Details" {
		t.Fatalf("stop = %#v, want nested Markdown", stops[0])
	}
}

func TestDecodeTripDocumentRejectsNonObject(t *testing.T) {
	if _, err := decodeTripDocument([]byte(`[]`)); err == nil {
		t.Fatal("decodeTripDocument accepted an array")
	}
	if _, err := decodeTripDocument([]byte(`null`)); err == nil {
		t.Fatal("decodeTripDocument accepted null")
	}
}

func TestReadTripToolsReturnCompleteDocument(t *testing.T) {
	ctx := context.Background()
	migrations, err := fs.Sub(journeyinroot.MigrationFS, "migrations")
	if err != nil {
		t.Fatalf("sub migrations: %v", err)
	}
	db, err := store.Open(ctx, filepath.Join(t.TempDir(), "journeyin.db"), migrations)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer db.Close()

	app := application.NewTripService(db)
	record, err := app.Create(ctx, []byte(`{"schema_version":1,"title":"MCP完整行程","status":"draft","timezone":"Asia/Shanghai","date_range":{"start":"2026-04-18","end":"2026-04-18"},"description_markdown":"# 总体说明\n\n## 来源\n[攻略](https://example.com/source)","days":[{"id":"day-1","date":"2026-04-18","title":"第1天","notes_markdown":"## 行程\n\n- 上午：城市漫步","stops":[{"id":"stop-1","sequence":1,"kind":"poi","title":"示例地点","description_markdown":"### 详情\n\n完整说明和来源：[来源](https://example.com/source)"}]}]}`), "test")
	if err != nil {
		t.Fatalf("create trip: %v", err)
	}
	history, _, err := app.SaveTripVersion(ctx, record.ID, record.Revision, "initial")
	if err != nil {
		t.Fatalf("save history: %v", err)
	}

	server := NewServer(app, "test", nil)
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	serverSession, err := server.mcp.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatalf("connect server: %v", err)
	}
	defer serverSession.Close()
	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "test"}, nil)
	clientSession, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("connect client: %v", err)
	}
	defer clientSession.Close()

	assertDocument := func(t *testing.T, result *mcp.CallToolResult) {
		t.Helper()
		if result.IsError {
			t.Fatalf("MCP tool returned error: %+v", result.Content)
		}
		payload, ok := result.StructuredContent.(map[string]any)
		if !ok {
			t.Fatalf("structured content has type %T, want object", result.StructuredContent)
		}
		document, ok := payload["document"].(map[string]any)
		if !ok {
			t.Fatalf("document has type %T, want complete object", payload["document"])
		}
		if document["description_markdown"] != "# 总体说明\n\n## 来源\n[攻略](https://example.com/source)" {
			t.Fatalf("description_markdown was not preserved: %#v", document["description_markdown"])
		}
		days, ok := document["days"].([]any)
		if !ok || len(days) != 1 {
			t.Fatalf("days = %#v, want complete day array", document["days"])
		}
	}

	tripResult, err := clientSession.CallTool(ctx, &mcp.CallToolParams{Name: "journeyin.get_trip", Arguments: map[string]any{"trip_id": record.ID}})
	if err != nil {
		t.Fatalf("get trip: %v", err)
	}
	assertDocument(t, tripResult)

	historyResult, err := clientSession.CallTool(ctx, &mcp.CallToolParams{Name: "journeyin.get_trip_history", Arguments: map[string]any{"trip_id": record.ID, "history_id": history.ID}})
	if err != nil {
		t.Fatalf("get trip history: %v", err)
	}
	assertDocument(t, historyResult)
}

func TestAttractionMonitorMCPTools(t *testing.T) {
	ctx := context.Background()
	migrations, err := fs.Sub(journeyinroot.MigrationFS, "migrations")
	if err != nil {
		t.Fatalf("sub migrations: %v", err)
	}
	db, err := store.Open(ctx, filepath.Join(t.TempDir(), "test.db"), migrations)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer db.Close()

	app := application.NewTripService(db)
	weatherReg := weather.NewRegistry()
	weatherSvc := weather.NewService(weatherReg, nil, nil, nil, nil, weather.ProviderAuto)
	monSvc := monitor.NewService(db, weatherSvc, nil)

	server := NewServer(app, "test", nil)
	server.SetMonitorService(monSvc)

	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	serverSession, err := server.mcp.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatalf("connect server: %v", err)
	}
	defer serverSession.Close()

	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "test"}, nil)
	clientSession, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("connect client: %v", err)
	}
	defer clientSession.Close()

	// 1. Verify tools are declared
	toolsResult, err := clientSession.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}

	expectedTools := []string{
		"journeyin.create_attraction_monitor",
		"journeyin.list_attraction_monitors",
		"journeyin.get_attraction_monitor",
		"journeyin.get_radar_opportunities",
		"journeyin.convert_monitor_to_trip",
	}

	for _, expected := range expectedTools {
		found := false
		for _, tool := range toolsResult.Tools {
			if tool.Name == expected {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("expected tool %s to be registered in MCP server", expected)
		}
	}

	// 2. Call journeyin.create_attraction_monitor
	createRes, err := clientSession.CallTool(ctx, &mcp.CallToolParams{
		Name: "journeyin.create_attraction_monitor",
		Arguments: map[string]any{
			"place_name":       "冷嘎措",
			"title":            "冷嘎措 · 贡嘎倒影与银河",
			"lat":               29.58,
			"lng":               101.78,
			"crs":               "gcj02",
			"months":            []int{6, 7, 8, 9, 10},
			"stargazing_mode":   true,
			"max_leave_days":    1,
			"min_duration_days": 2,
			"max_duration_days": 3,
			"origin_city":       "成都",
			"max_budget":        1200,
		},
	})
	if err != nil {
		t.Fatalf("call create_attraction_monitor: %v", err)
	}
	if createRes.IsError {
		t.Fatalf("create_attraction_monitor returned error: %+v", createRes.Content)
	}

	createPayload, ok := createRes.StructuredContent.(map[string]any)
	if !ok {
		t.Fatalf("expected structured content map, got %T", createRes.StructuredContent)
	}
	monitorID, _ := createPayload["monitor_id"].(string)
	if monitorID == "" {
		t.Fatalf("expected non-empty monitor_id")
	}

	// 3. Call journeyin.list_attraction_monitors
	listRes, err := clientSession.CallTool(ctx, &mcp.CallToolParams{
		Name: "journeyin.list_attraction_monitors",
		Arguments: map[string]any{},
	})
	if err != nil {
		t.Fatalf("call list_attraction_monitors: %v", err)
	}
	if listRes.IsError {
		t.Fatalf("list_attraction_monitors returned error: %+v", listRes.Content)
	}

	// 4. Call journeyin.get_attraction_monitor
	getRes, err := clientSession.CallTool(ctx, &mcp.CallToolParams{
		Name: "journeyin.get_attraction_monitor",
		Arguments: map[string]any{
			"monitor_id": monitorID,
		},
	})
	if err != nil {
		t.Fatalf("call get_attraction_monitor: %v", err)
	}
	if getRes.IsError {
		t.Fatalf("get_attraction_monitor returned error: %+v", getRes.Content)
	}

	// 5. Call journeyin.get_radar_opportunities
	radarRes, err := clientSession.CallTool(ctx, &mcp.CallToolParams{
		Name: "journeyin.get_radar_opportunities",
		Arguments: map[string]any{},
	})
	if err != nil {
		t.Fatalf("call get_radar_opportunities: %v", err)
	}
	if radarRes.IsError {
		t.Fatalf("get_radar_opportunities returned error: %+v", radarRes.Content)
	}

	// 6. Call journeyin.convert_monitor_to_trip
	convRes, err := clientSession.CallTool(ctx, &mcp.CallToolParams{
		Name: "journeyin.convert_monitor_to_trip",
		Arguments: map[string]any{
			"monitor_id":   monitorID,
			"window_start": "2026-06-19",
			"window_end":   "2026-06-21",
		},
	})
	if err != nil {
		t.Fatalf("call convert_monitor_to_trip: %v", err)
	}
	if convRes.IsError {
		t.Fatalf("convert_monitor_to_trip returned error: %+v", convRes.Content)
	}
	convPayload, _ := convRes.StructuredContent.(map[string]any)
	tripID, _ := convPayload["trip_id"].(string)
	if tripID == "" {
		t.Fatalf("expected non-empty trip_id from convert_monitor_to_trip")
	}

	t.Logf("Successfully executed all 5 attraction monitor MCP tools. Created Trip ID: %s", tripID)
}
